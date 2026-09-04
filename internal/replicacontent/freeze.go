package replicacontent

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/privatepath"
)

var fixedArchiveTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

var excludedDirectoryReasons = map[string]string{
	".git": "version-control", ".hg": "version-control", ".svn": "version-control",
	".viceme": "viceme-state", ".idea": "editor-metadata", ".vscode": "editor-metadata",
	"node_modules": "dependency", "vendor": "dependency", ".venv": "dependency", "venv": "dependency",
	".cache": "cache", ".turbo": "cache", ".pytest_cache": "cache", ".mypy_cache": "cache", ".ruff_cache": "cache",
	"__pycache__": "cache", ".parcel-cache": "cache", ".vite": "cache", ".gradle": "cache",
	"dist": "build-output", "build": "build-output", "out": "build-output", ".next": "build-output", ".nuxt": "build-output",
	".svelte-kit": "build-output", ".output": "build-output", "coverage": "build-output", "target": "build-output",
	".vercel": "build-output", ".netlify": "build-output", ".serverless": "build-output",
}

var excludedFileReasons = map[string]string{
	".git": "version-control", ".hg": "version-control", ".svn": "version-control", ".viceme": "viceme-state",
}

type FreezeSourceOptions struct {
	Purpose, CreatorNotes string
	ExpiresAt             time.Time
}
type SourceArchiveExclusion struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}
type SourceArchiveSummary struct {
	Digest            string                   `json:"digest"`
	SizeBytes         int64                    `json:"sizeBytes"`
	IncludedFileCount int                      `json:"includedFileCount"`
	IncludedBytes     uint64                   `json:"includedBytes"`
	IncludedPaths     []string                 `json:"includedPaths"`
	ExcludedPaths     []SourceArchiveExclusion `json:"excludedPaths"`
}
type FrozenSourceArchive struct {
	directory, filename, digest string
	sizeBytes                   int64
	Summary                     SourceArchiveSummary
	ExpiresAt                   time.Time
}
type frozenSourceFile struct {
	name, snapshot string
	mode           fs.FileMode
	size           uint64
	data           []byte
}

// ValidateSourceWorktree performs the fail-closed source inspection before a
// preview is opened. The later freeze repeats the inspection while copying the
// exact bytes that the final confirmation binds.
func ValidateSourceWorktree(sourcePath string) (returnErr error) {
	if strings.TrimSpace(sourcePath) == "" {
		return errors.New("Website Replica source path is invalid")
	}
	source, err := filepath.Abs(sourcePath)
	if err != nil {
		return errors.New("Website Replica source path is invalid")
	}
	info, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("inspect Website Replica source path: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
		return errors.New("Website Replica source must be a real project directory or regular ZIP file")
	}
	if info.Mode().IsRegular() {
		if !strings.EqualFold(filepath.Ext(source), ".zip") {
			return errors.New("Website Replica source file must be a ZIP archive")
		}
		if info.Size() <= 0 || info.Size() > MaxArchiveBytes {
			return errors.New("Website Replica ZIP is outside the archive size limit")
		}
		file, err := os.Open(source)
		if err != nil {
			return fmt.Errorf("open Website Replica ZIP: %w", err)
		}
		defer func() { returnErr = errors.Join(returnErr, file.Close()) }()
		return ValidatePublishArchive(file, info.Size())
	}
	directory, err := privatepath.CreateTempDirectory(os.TempDir(), ".viceme-replica-check-*")
	if err != nil {
		return fmt.Errorf("create private Website Replica inspection directory: %w", err)
	}
	defer func() { returnErr = errors.Join(returnErr, os.RemoveAll(directory)) }()
	files, _, _, err := snapshotWorktree(source, directory)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return errors.New("Website Replica source must contain at least one publishable file")
	}
	return nil
}

// PrepareSourcePreview returns a directory suitable for the local preview
// runtime. Existing ZIP inputs are validated and extracted into an owner-only
// temporary directory; callers must invoke the returned cleanup function.
func PrepareSourcePreview(sourcePath string) (_ string, cleanup func() error, returnErr error) {
	source, err := filepath.Abs(sourcePath)
	if err != nil || strings.TrimSpace(sourcePath) == "" {
		return "", nil, errors.New("Website Replica source path is invalid")
	}
	info, err := os.Lstat(source)
	if err != nil {
		return "", nil, fmt.Errorf("inspect Website Replica source path: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", nil, errors.New("Website Replica source must not be a symbolic link")
	}
	if info.IsDir() {
		return source, func() error { return nil }, nil
	}
	if !info.Mode().IsRegular() || !strings.EqualFold(filepath.Ext(source), ".zip") || info.Size() <= 0 || info.Size() > MaxArchiveBytes {
		return "", nil, errors.New("Website Replica source must be a real project directory or regular ZIP file")
	}
	file, err := os.Open(source)
	if err != nil {
		return "", nil, fmt.Errorf("open Website Replica ZIP: %w", err)
	}
	plan, validationErr := validatePublishArchive(file, info.Size())
	if validationErr != nil {
		return "", nil, errors.Join(validationErr, file.Close())
	}
	directory, err := privatepath.CreateTempDirectory(os.TempDir(), ".viceme-replica-preview-*")
	if err != nil {
		return "", nil, errors.Join(fmt.Errorf("create private Website Replica preview directory: %w", err), file.Close())
	}
	if err := errors.Join(extractArchive(plan, directory), file.Close()); err != nil {
		return "", nil, errors.Join(err, os.RemoveAll(directory))
	}
	return directory, func() error { return os.RemoveAll(directory) }, nil
}

func FreezeSourceArchive(sourcePath string, options FreezeSourceOptions) (*FrozenSourceArchive, error) {
	if strings.TrimSpace(sourcePath) == "" {
		return nil, errors.New("Website Replica source path is invalid")
	}
	source, err := filepath.Abs(sourcePath)
	if err != nil {
		return nil, errors.New("Website Replica source path is invalid")
	}
	info, err := os.Lstat(source)
	if err != nil {
		return nil, fmt.Errorf("inspect Website Replica source path: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
		return nil, errors.New("Website Replica source must be a real directory or regular ZIP file")
	}
	if info.Mode().IsRegular() && !strings.EqualFold(filepath.Ext(source), ".zip") {
		return nil, errors.New("Website Replica source file must be a ZIP archive")
	}
	directory, err := privatepath.CreateTempDirectory(os.TempDir(), ".viceme-replica-freeze-*")
	if err != nil {
		return nil, fmt.Errorf("create private Website Replica freeze directory: %w", err)
	}
	result := &FrozenSourceArchive{directory: directory, filename: filepath.Join(directory, "source.zip"), ExpiresAt: options.ExpiresAt}
	succeeded := false
	defer func() {
		if !succeeded {
			_ = result.Cleanup()
		}
	}()
	excluded := make([]SourceArchiveExclusion, 0)
	if info.IsDir() {
		files, worktreeExclusions, envNames, err := snapshotWorktree(source, directory)
		if err != nil {
			return nil, err
		}
		handoff, err := generateProjectHandoff(files, envNames, options)
		if err != nil {
			return nil, err
		}
		files = append(files, frozenSourceFile{name: ProjectHandoffFile, mode: 0o644, size: uint64(len(handoff)), data: handoff})
		sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
		if err := writeDeterministicSourceZIP(result.filename, files); err != nil {
			return nil, err
		}
		for _, sourceFile := range files {
			if sourceFile.snapshot != "" {
				if err := os.Remove(sourceFile.snapshot); err != nil {
					return nil, err
				}
			}
		}
		excluded = worktreeExclusions
	} else {
		if info.Size() <= 0 || info.Size() > MaxArchiveBytes {
			return nil, errors.New("Website Replica ZIP is outside the archive size limit")
		}
		if err := copyWorkspaceFile(source, result.filename, info.Size()); err != nil {
			return nil, fmt.Errorf("freeze Website Replica ZIP: %w", err)
		}
	}
	if err := finalizeFrozenSourceArchive(result, excluded); err != nil {
		return nil, err
	}
	succeeded = true
	return result, nil
}

func finalizeFrozenSourceArchive(result *FrozenSourceArchive, excluded []SourceArchiveExclusion) error {
	file, err := os.Open(result.filename)
	if err != nil {
		return err
	}
	archiveInfo, err := file.Stat()
	if err != nil || !archiveInfo.Mode().IsRegular() || archiveInfo.Size() <= 0 || archiveInfo.Size() > MaxArchiveBytes {
		_ = file.Close()
		return errors.New("frozen Website Replica ZIP exceeds the archive limit")
	}
	plan, err := validatePublishArchive(file, archiveInfo.Size())
	if err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = file.Close()
		return err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	paths := make([]string, 0, len(plan.files))
	for _, sourceFile := range plan.files {
		paths = append(paths, sourceFile.name)
	}
	sort.Strings(paths)
	sort.Slice(excluded, func(i, j int) bool {
		if excluded[i].Path == excluded[j].Path {
			return excluded[i].Reason < excluded[j].Reason
		}
		return excluded[i].Path < excluded[j].Path
	})
	result.Summary = SourceArchiveSummary{
		Digest:            hex.EncodeToString(hash.Sum(nil)),
		SizeBytes:         archiveInfo.Size(),
		IncludedFileCount: len(plan.files),
		IncludedBytes:     plan.expandedBytes,
		IncludedPaths:     paths,
		ExcludedPaths:     excluded,
	}
	result.digest = result.Summary.Digest
	result.sizeBytes = result.Summary.SizeBytes
	return nil
}

func (archive *FrozenSourceArchive) Path() string {
	if archive == nil {
		return ""
	}
	return archive.filename
}

func (archive *FrozenSourceArchive) Open() (*os.File, fs.FileInfo, error) {
	if archive == nil || archive.directory == "" {
		return nil, nil, errors.New("frozen Website Replica archive is unavailable")
	}
	if err := privatepath.RequirePrivateDirectory(archive.directory); err != nil {
		return nil, nil, err
	}
	if err := privatepath.RequirePrivateFile(archive.filename); err != nil {
		return nil, nil, err
	}
	file, err := os.Open(archive.filename)
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != archive.sizeBytes {
		_ = file.Close()
		return nil, nil, errors.New("frozen Website Replica archive changed after creation")
	}
	hash := sha256.New()
	_, hashErr := io.Copy(hash, file)
	if hashErr != nil || hex.EncodeToString(hash.Sum(nil)) != archive.digest {
		_ = file.Close()
		return nil, nil, errors.New("frozen Website Replica archive digest changed after creation")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	return file, info, nil
}

func (archive *FrozenSourceArchive) Cleanup() error {
	if archive == nil || archive.directory == "" {
		return nil
	}
	directory := archive.directory
	if err := os.RemoveAll(directory); err != nil {
		return err
	}
	archive.directory, archive.filename = "", ""
	return nil
}

func (archive *FrozenSourceArchive) CleanupIfExpired(now time.Time) (bool, error) {
	if archive == nil || archive.ExpiresAt.IsZero() || now.Before(archive.ExpiresAt) {
		return false, nil
	}
	if err := archive.Cleanup(); err != nil {
		return false, err
	}
	return true, nil
}

func snapshotWorktree(root, freezeDirectory string) ([]frozenSourceFile, []SourceArchiveExclusion, map[string]struct{}, error) {
	type pendingFile struct {
		name, filename string
		mode           fs.FileMode
		size           int64
	}
	var pending []pendingFile
	var excluded []SourceArchiveExclusion
	envNames := make(map[string]struct{})
	var expanded uint64
	err := filepath.WalkDir(root, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filename == freezeDirectory {
			return filepath.SkipDir
		}
		relative, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			if reason, found := excludedDirectoryReasons[strings.ToLower(entry.Name())]; found {
				excluded = append(excluded, SourceArchiveExclusion{Path: relative, Reason: reason})
				return filepath.SkipDir
			}
			if err := validateSensitivePath(relative); err != nil {
				return err
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if relative == ProjectHandoffFile {
			excluded = append(excluded, SourceArchiveExclusion{Path: relative, Reason: "generated-handoff"})
			return nil
		}
		base := strings.ToLower(entry.Name())
		if reason, found := excludedFileReasons[base]; found {
			excluded = append(excluded, SourceArchiveExclusion{Path: relative, Reason: reason})
			return nil
		}
		if isEnvironmentFile(base) {
			data, err := readEnvironmentFile(filename, info)
			if err != nil {
				return &SensitiveContentError{Path: relative}
			}
			if err := validateExcludedEnvironmentFile(relative, data); err != nil {
				return err
			}
			collectEnvironmentFileNames(data, envNames)
			excluded = append(excluded, SourceArchiveExclusion{Path: relative, Reason: "environment"})
			return nil
		}
		if base == ".ds_store" || base == "thumbs.db" || strings.HasPrefix(base, "._") {
			excluded = append(excluded, SourceArchiveExclusion{Path: relative, Reason: "system-metadata"})
			return nil
		}
		if err := validateSensitivePath(relative); err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("Website Replica source contains a link or special file: %s", relative)
		}
		if info.Size() < 0 || uint64(info.Size()) > MaxFileBytes {
			return fmt.Errorf("Website Replica source file exceeds the per-file limit: %s", relative)
		}
		if len(pending) >= MaxFileCount-1 || expanded > MaxExpandedBytes-uint64(info.Size()) {
			return errors.New("Website Replica source exceeds the archive limits")
		}
		expanded += uint64(info.Size())
		pending = append(pending, pendingFile{name: relative, filename: filename, mode: info.Mode(), size: info.Size()})
		return nil
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("inspect Website Replica source worktree: %w", err)
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].name < pending[j].name })
	files := make([]frozenSourceFile, 0, len(pending))
	for index, source := range pending {
		snapshot := filepath.Join(freezeDirectory, fmt.Sprintf("source-%06d.snapshot", index))
		if err := copyWorkspaceFile(source.filename, snapshot, source.size); err != nil {
			return nil, nil, nil, fmt.Errorf("freeze Website Replica source file %s: %w", source.name, err)
		}
		data, err := os.ReadFile(snapshot)
		if err != nil {
			return nil, nil, nil, err
		}
		if err := validateSensitiveContent(source.name, data); err != nil {
			return nil, nil, nil, err
		}
		if err := validateForbiddenReplicaContent(source.name, data); err != nil {
			return nil, nil, nil, err
		}
		collectEnvironmentReferences(data, envNames)
		files = append(files, frozenSourceFile{name: source.name, snapshot: snapshot, mode: source.mode, size: uint64(len(data))})
	}
	return files, excluded, envNames, nil
}

func copyWorkspaceFile(sourceName, snapshotName string, expectedSize int64) error {
	before, err := os.Lstat(sourceName)
	if err != nil || !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 || before.Size() != expectedSize {
		return errors.New("source file changed before freezing")
	}
	input, err := os.Open(sourceName)
	if err != nil {
		return err
	}
	opened, err := input.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		_ = input.Close()
		return errors.New("source file changed while opening")
	}
	output, err := privatepath.CreateExclusiveFile(snapshotName)
	if err != nil {
		_ = input.Close()
		return err
	}
	written, copyErr := io.Copy(output, io.LimitReader(input, int64(MaxFileBytes)+1))
	syncErr := output.Sync()
	closeOutErr := output.Close()
	closeInErr := input.Close()
	if copyErr != nil || syncErr != nil || closeOutErr != nil || closeInErr != nil {
		return errors.Join(copyErr, syncErr, closeOutErr, closeInErr)
	}
	if written != expectedSize {
		return errors.New("source file changed while freezing")
	}
	return nil
}

func writeDeterministicSourceZIP(filename string, files []frozenSourceFile) error {
	output, err := privatepath.CreateExclusiveFile(filename)
	if err != nil {
		return err
	}
	writer := zip.NewWriter(output)
	failed := func(err error) error { _ = writer.Close(); _ = output.Close(); return err }
	for _, file := range files {
		header := &zip.FileHeader{Name: file.name, Method: zip.Store}
		header.SetModTime(fixedArchiveTime)
		if file.mode.Perm()&0o111 != 0 {
			header.SetMode(0o755)
		} else {
			header.SetMode(0o644)
		}
		entry, err := writer.CreateHeader(header)
		if err != nil {
			return failed(err)
		}
		if file.snapshot != "" {
			input, err := os.Open(file.snapshot)
			if err != nil {
				return failed(err)
			}
			written, copyErr := io.Copy(entry, input)
			closeErr := input.Close()
			if copyErr != nil || closeErr != nil {
				return failed(errors.Join(copyErr, closeErr))
			}
			if uint64(written) != file.size {
				return failed(errors.New("frozen Website Replica source file size changed"))
			}
		} else if _, err := entry.Write(file.data); err != nil {
			return failed(err)
		}
	}
	if err := writer.Close(); err != nil {
		_ = output.Close()
		return err
	}
	if err := output.Sync(); err != nil {
		_ = output.Close()
		return err
	}
	return output.Close()
}
