package replicacontent

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/ViceMe-AI/cli/internal/atomicfile"
	"github.com/gofrs/flock"
)

const (
	LicenseFilePath          = ".viceme/replica-license.json"
	installJournalSuffix     = ".viceme-replica-install.json"
	installJournalTempSuffix = ".tmp"
	installStageSuffix       = ".viceme-replica-stage"
	installLockSuffix        = ".viceme-replica-install.lock"
)

var (
	digestPattern            = regexp.MustCompile(`^[a-f0-9]{64}$`)
	errInstalledTreeMismatch = errors.New("installed Website Replica tree does not match the archive")
	installTestCrashHook     func(string)
)

type LicenseRecord struct {
	ReplicaID      string
	VersionID      string
	Version        int
	ArtifactDigest string
	License        json.RawMessage
}

type InstallResult struct {
	Target        string `json:"target"`
	LicensePath   string `json:"licensePath"`
	FileCount     int    `json:"fileCount"`
	ExpandedBytes uint64 `json:"expandedBytes"`
}

type archiveFile struct {
	entry *zip.File
	name  string
	mode  os.FileMode
}

type archivePlan struct {
	files         []archiveFile
	directories   []string
	expandedBytes uint64
}

type pathNode struct {
	display  string
	dir      bool
	explicit bool
}

type installJournal struct {
	SchemaVersion int    `json:"schemaVersion"`
	Status        string `json:"status"`
	Target        string `json:"target"`
	Stage         string `json:"stage"`
}

type persistedLicense struct {
	SchemaVersion  int             `json:"schemaVersion"`
	ReplicaID      string          `json:"replicaId"`
	VersionID      string          `json:"versionId"`
	Version        int             `json:"version"`
	ArtifactDigest string          `json:"artifactDigest"`
	License        json.RawMessage `json:"license"`
}

func InstallArchive(archivePath, target string, license LicenseRecord) (InstallResult, error) {
	if err := validateLicense(license); err != nil {
		return InstallResult{}, err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil || strings.TrimSpace(target) == "" {
		return InstallResult{}, errors.New("Website Replica target path is invalid")
	}
	absTarget = filepath.Clean(absTarget)
	parent := filepath.Dir(absTarget)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("create Website Replica target parent: %w", err)
	}
	installLock := flock.New(absTarget + installLockSuffix)
	if err := installLock.Lock(); err != nil {
		return InstallResult{}, fmt.Errorf("lock Website Replica target: %w", err)
	}
	defer installLock.Unlock()

	journalPath := absTarget + installJournalSuffix
	stage := absTarget + installStageSuffix
	if err := recoverInstall(journalPath, absTarget, stage); err != nil {
		return InstallResult{}, err
	}
	if err := requireMissingPath(stage, "unowned staging path already exists"); err != nil {
		return InstallResult{}, err
	}

	reader, closeArchive, err := openArchive(archivePath)
	if err != nil {
		return InstallResult{}, err
	}
	defer closeArchive()
	plan, err := inspectArchive(reader)
	if err != nil {
		return InstallResult{}, err
	}
	if _, err := os.Lstat(absTarget); err == nil {
		matches, matchErr := installedTreeMatches(absTarget, plan, license)
		if matchErr != nil {
			return InstallResult{}, matchErr
		}
		if !matches {
			return InstallResult{}, errors.New("refuse Website Replica installation: target already exists")
		}
		return InstallResult{
			Target: absTarget, LicensePath: filepath.Join(absTarget, filepath.FromSlash(LicenseFilePath)),
			FileCount: len(plan.files), ExpandedBytes: plan.expandedBytes,
		}, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return InstallResult{}, fmt.Errorf("inspect Website Replica installation path: %w", err)
	}

	journal := installJournal{SchemaVersion: 1, Status: "PREPARING", Target: absTarget, Stage: stage}
	if err := writeJournal(journalPath, journal); err != nil {
		return InstallResult{}, err
	}
	journalActive := true
	stageActive := false
	defer func() {
		if stageActive {
			_ = os.RemoveAll(stage)
		}
		if journalActive {
			_ = os.Remove(journalPath)
		}
	}()
	if err := os.Mkdir(stage, 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("create Website Replica staging directory: %w", err)
	}
	stageActive = true
	if err := extractArchive(plan, stage); err != nil {
		return InstallResult{}, err
	}
	licensePath := filepath.Join(stage, filepath.FromSlash(LicenseFilePath))
	if err := writeLicense(licensePath, license); err != nil {
		return InstallResult{}, err
	}
	if err := syncStagedDirectories(stage); err != nil {
		return InstallResult{}, err
	}

	journal.Status = "ACTIVATING"
	if err := writeJournal(journalPath, journal); err != nil {
		return InstallResult{}, err
	}
	if err := requireMissingPath(absTarget, "target appeared during installation"); err != nil {
		return InstallResult{}, err
	}
	if err := activateNoReplace(stage, absTarget); err != nil {
		return InstallResult{}, fmt.Errorf("activate Website Replica installation without overwrite: %w", err)
	}
	runInstallTestCrashHook("target-activated")
	stageActive = false
	if err := atomicfile.SyncDirectory(parent); err != nil {
		return InstallResult{}, fmt.Errorf("sync Website Replica target activation: %w", err)
	}
	runInstallTestCrashHook("target-directory-synced")
	if err := os.Remove(journalPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return InstallResult{}, fmt.Errorf("complete Website Replica install journal: %w", err)
	}
	runInstallTestCrashHook("journal-removed")
	if err := atomicfile.SyncDirectory(parent); err != nil {
		return InstallResult{}, fmt.Errorf("sync Website Replica install completion: %w", err)
	}
	runInstallTestCrashHook("completion-directory-synced")
	journalActive = false
	return InstallResult{
		Target: absTarget, LicensePath: filepath.Join(absTarget, filepath.FromSlash(LicenseFilePath)),
		FileCount: len(plan.files), ExpandedBytes: plan.expandedBytes,
	}, nil
}

func validateLicense(license LicenseRecord) error {
	if strings.TrimSpace(license.ReplicaID) == "" || strings.TrimSpace(license.VersionID) == "" || license.Version < 1 ||
		!digestPattern.MatchString(license.ArtifactDigest) || len(bytes.TrimSpace(license.License)) == 0 ||
		bytes.Equal(bytes.TrimSpace(license.License), []byte("null")) || !json.Valid(license.License) {
		return errors.New("Website Replica license metadata is invalid")
	}
	return nil
}

func installedTreeMatches(target string, plan archivePlan, expected LicenseRecord) (bool, error) {
	info, err := os.Lstat(target)
	if err != nil {
		return false, fmt.Errorf("inspect existing Website Replica target: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false, nil
	}
	expectedDirectories := map[string]struct{}{".": {}}
	expectedFiles := make(map[string]*zip.File, len(plan.files)+1)
	addParents := func(name string) {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			expectedDirectories[parent] = struct{}{}
		}
	}
	for _, directory := range plan.directories {
		expectedDirectories[directory] = struct{}{}
		addParents(directory)
	}
	for _, file := range plan.files {
		expectedFiles[file.name] = file.entry
		addParents(file.name)
	}
	expectedFiles[LicenseFilePath] = nil
	addParents(LicenseFilePath)

	seenDirectories := make(map[string]struct{}, len(expectedDirectories))
	seenFiles := make(map[string]struct{}, len(expectedFiles))
	err = filepath.WalkDir(target, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(target, filename)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		entryInfo, err := entry.Info()
		if err != nil {
			return err
		}
		if entryInfo.Mode()&os.ModeSymlink != 0 {
			return errInstalledTreeMismatch
		}
		if entry.IsDir() {
			if _, ok := expectedDirectories[relative]; !ok {
				return errInstalledTreeMismatch
			}
			seenDirectories[relative] = struct{}{}
			return nil
		}
		archiveEntry, ok := expectedFiles[relative]
		if !ok || !entryInfo.Mode().IsRegular() {
			return errInstalledTreeMismatch
		}
		if archiveEntry != nil {
			matches, err := installedFileMatches(filename, entryInfo, archiveEntry)
			if err != nil {
				return err
			}
			if !matches {
				return errInstalledTreeMismatch
			}
		}
		seenFiles[relative] = struct{}{}
		return nil
	})
	if errors.Is(err, errInstalledTreeMismatch) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect existing Website Replica tree: %w", err)
	}
	if len(seenDirectories) != len(expectedDirectories) || len(seenFiles) != len(expectedFiles) {
		return false, nil
	}

	licensePath := filepath.Join(target, filepath.FromSlash(LicenseFilePath))
	licenseInfo, err := os.Lstat(licensePath)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil || !licenseInfo.Mode().IsRegular() || licenseInfo.Mode()&os.ModeSymlink != 0 || licenseInfo.Size() > 64<<10 {
		return false, err
	}
	data, err := os.ReadFile(licensePath)
	if err != nil {
		return false, fmt.Errorf("read existing Website Replica license: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var existing persistedLicense
	if err := decoder.Decode(&existing); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return false, nil
	}
	if existing.SchemaVersion != 1 || existing.ReplicaID != expected.ReplicaID || existing.VersionID != expected.VersionID ||
		existing.Version != expected.Version || existing.ArtifactDigest != expected.ArtifactDigest {
		return false, nil
	}
	var existingDocument any
	var expectedDocument any
	if json.Unmarshal(existing.License, &existingDocument) != nil || json.Unmarshal(expected.License, &expectedDocument) != nil {
		return false, nil
	}
	return reflect.DeepEqual(existingDocument, expectedDocument), nil
}

func installedFileMatches(filename string, before fs.FileInfo, archiveEntry *zip.File) (bool, error) {
	if before.Size() != int64(archiveEntry.UncompressedSize64) {
		return false, nil
	}
	installed, err := os.Open(filename)
	if err != nil {
		return false, fmt.Errorf("open existing Website Replica file: %w", err)
	}
	defer installed.Close()
	after, err := installed.Stat()
	if err != nil {
		return false, fmt.Errorf("inspect open Website Replica file: %w", err)
	}
	if !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return false, nil
	}
	installedDigest := sha256.New()
	installedSize, err := io.Copy(installedDigest, installed)
	if err != nil {
		return false, fmt.Errorf("hash existing Website Replica file: %w", err)
	}
	archived, err := archiveEntry.Open()
	if err != nil {
		return false, fmt.Errorf("open Website Replica archive entry for recovery: %w", err)
	}
	archiveDigest := sha256.New()
	archiveSize, copyErr := io.Copy(archiveDigest, archived)
	closeErr := archived.Close()
	if copyErr != nil || closeErr != nil {
		return false, fmt.Errorf("hash Website Replica archive entry for recovery: %w", errors.Join(copyErr, closeErr))
	}
	return installedSize == archiveSize && installedSize == before.Size() &&
		bytes.Equal(installedDigest.Sum(nil), archiveDigest.Sum(nil)), nil
}

func openArchive(filename string) (*zip.Reader, func(), error) {
	pathInfo, err := os.Lstat(filename)
	if err != nil {
		return nil, func() {}, fmt.Errorf("inspect Website Replica ZIP: %w", err)
	}
	if !pathInfo.Mode().IsRegular() || pathInfo.Mode()&os.ModeSymlink != 0 || pathInfo.Size() <= 0 || pathInfo.Size() > MaxArchiveBytes {
		return nil, func() {}, errors.New("Website Replica ZIP is not a regular archive within the size limit")
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, func() {}, fmt.Errorf("open Website Replica ZIP: %w", err)
	}
	closeArchive := func() { _ = file.Close() }
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(pathInfo, info) {
		closeArchive()
		return nil, func() {}, errors.New("Website Replica ZIP changed while it was opened")
	}
	if err := validateCentralDirectory(file, info.Size()); err != nil {
		closeArchive()
		return nil, func() {}, err
	}
	reader, err := zip.NewReader(file, info.Size())
	if err != nil {
		closeArchive()
		return nil, func() {}, fmt.Errorf("open Website Replica ZIP directory: %w", err)
	}
	if len(reader.File) > MaxArchiveEntries {
		closeArchive()
		return nil, func() {}, errors.New("Website Replica ZIP contains too many entries")
	}
	return reader, closeArchive, nil
}

func validateCentralDirectory(file *os.File, size int64) error {
	const (
		endRecordSize  = 22
		maxCommentSize = 1<<16 - 1
		endSignature   = 0x06054b50
	)
	if size < endRecordSize {
		return errors.New("Website Replica ZIP end record is missing")
	}
	tailSize := int64(endRecordSize + maxCommentSize)
	if size < tailSize {
		tailSize = size
	}
	tail := make([]byte, tailSize)
	if _, err := file.ReadAt(tail, size-tailSize); err != nil {
		return fmt.Errorf("read Website Replica ZIP end record: %w", err)
	}
	for offset := len(tail) - endRecordSize; offset >= 0; offset-- {
		if binary.LittleEndian.Uint32(tail[offset:]) != endSignature {
			continue
		}
		commentSize := int(binary.LittleEndian.Uint16(tail[offset+20:]))
		if offset+endRecordSize+commentSize != len(tail) {
			continue
		}
		disk := binary.LittleEndian.Uint16(tail[offset+4:])
		centralDisk := binary.LittleEndian.Uint16(tail[offset+6:])
		diskEntries := binary.LittleEndian.Uint16(tail[offset+8:])
		totalEntries := binary.LittleEndian.Uint16(tail[offset+10:])
		centralSize := binary.LittleEndian.Uint32(tail[offset+12:])
		centralOffset := binary.LittleEndian.Uint32(tail[offset+16:])
		if disk != 0 || centralDisk != 0 || diskEntries != totalEntries {
			return errors.New("Website Replica ZIP uses unsupported multi-disk metadata")
		}
		if totalEntries == 0xffff || centralSize == 0xffffffff || centralOffset == 0xffffffff {
			return errors.New("Website Replica ZIP64 metadata is not supported")
		}
		if int(totalEntries) > MaxArchiveEntries {
			return errors.New("Website Replica ZIP contains too many entries")
		}
		return nil
	}
	return errors.New("Website Replica ZIP end record is invalid")
}

func inspectArchive(reader *zip.Reader) (archivePlan, error) {
	plan := archivePlan{}
	nodes := make(map[string]pathNode)
	for _, entry := range reader.File {
		name, isDirectory, err := validateArchivePath(entry)
		if err != nil {
			return archivePlan{}, err
		}
		if err := registerArchivePath(nodes, name, isDirectory); err != nil {
			return archivePlan{}, err
		}
		if strings.EqualFold(name, LicenseFilePath) {
			return archivePlan{}, errors.New("Website Replica ZIP contains the reserved license path")
		}
		mode := entry.Mode()
		if isDirectory {
			if mode.Type() != os.ModeDir || entry.UncompressedSize64 != 0 {
				return archivePlan{}, errors.New("Website Replica ZIP contains an invalid directory entry")
			}
			plan.directories = append(plan.directories, name)
			continue
		}
		if !mode.IsRegular() || mode.Type() != 0 || mode&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || entry.Flags&0x1 != 0 {
			return archivePlan{}, errors.New("Website Replica ZIP contains a link, encrypted entry, or special file")
		}
		if len(plan.files) >= MaxFileCount || entry.UncompressedSize64 > MaxFileBytes {
			return archivePlan{}, errors.New("Website Replica ZIP exceeds the file count or per-file size limit")
		}
		if entry.UncompressedSize64 > 0 &&
			(entry.CompressedSize64 == 0 ||
				(entry.UncompressedSize64-1)/entry.CompressedSize64+1 > MaxCompressionRatio) {
			return archivePlan{}, errors.New("Website Replica ZIP contains a file above the compression ratio limit")
		}
		if plan.expandedBytes > MaxExpandedBytes-entry.UncompressedSize64 {
			return archivePlan{}, errors.New("Website Replica ZIP exceeds the total expanded size limit")
		}
		plan.expandedBytes += entry.UncompressedSize64
		installMode := os.FileMode(0o644)
		if mode.Perm()&0o111 != 0 {
			installMode = 0o755
		}
		plan.files = append(plan.files, archiveFile{entry: entry, name: name, mode: installMode})
	}
	if len(plan.files) == 0 {
		return archivePlan{}, errors.New("Website Replica ZIP contains no regular files")
	}
	return plan, nil
}

func validateArchivePath(entry *zip.File) (string, bool, error) {
	raw := entry.Name
	if raw == "" || len(raw) > MaxArchivePathBytes || !utf8.ValidString(raw) || strings.ContainsRune(raw, '\x00') || strings.Contains(raw, `\`) || strings.HasPrefix(raw, "/") {
		return "", false, errors.New("Website Replica ZIP contains an unsafe path")
	}
	mode := entry.Mode()
	isDirectory := mode.IsDir()
	if isDirectory {
		raw = strings.TrimSuffix(raw, "/")
	} else if strings.HasSuffix(raw, "/") {
		return "", false, errors.New("Website Replica ZIP contains an invalid file path")
	}
	segments := strings.Split(raw, "/")
	if len(segments) > MaxArchivePathDepth {
		return "", false, errors.New("Website Replica ZIP path is too deep")
	}
	for index, segment := range segments {
		if segment == "" || len(segment) > MaxArchiveSegmentBytes || segment == "." || segment == ".." || strings.Contains(segment, ":") ||
			strings.HasSuffix(segment, ".") || strings.HasSuffix(segment, " ") || hasControlCharacter(segment) || isWindowsDeviceName(segment) ||
			(index == 0 && len(segment) >= 2 && ((segment[0] >= 'A' && segment[0] <= 'Z') || (segment[0] >= 'a' && segment[0] <= 'z')) && segment[1] == ':') {
			return "", false, errors.New("Website Replica ZIP contains an unsafe path segment")
		}
	}
	if raw == "" || path.IsAbs(raw) || path.Clean(raw) != raw || filepath.IsAbs(filepath.FromSlash(raw)) {
		return "", false, errors.New("Website Replica ZIP contains a non-canonical path")
	}
	return raw, isDirectory, nil
}

func hasControlCharacter(value string) bool {
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return true
		}
	}
	return false
}

func isWindowsDeviceName(segment string) bool {
	base := strings.ToUpper(strings.SplitN(segment, ".", 2)[0])
	switch base {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	return len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) &&
		base[3] >= '1' && base[3] <= '9'
}

func registerArchivePath(nodes map[string]pathNode, name string, isDirectory bool) error {
	segments := strings.Split(name, "/")
	for index := range segments {
		display := strings.Join(segments[:index+1], "/")
		key := strings.ToLower(display)
		final := index == len(segments)-1
		wantDirectory := !final || isDirectory
		existing, found := nodes[key]
		if found {
			if existing.display != display {
				return errors.New("Website Replica ZIP contains paths that conflict by letter case")
			}
			if existing.dir != wantDirectory {
				return errors.New("Website Replica ZIP contains a file and directory path conflict")
			}
			if final && existing.explicit {
				return errors.New("Website Replica ZIP contains a duplicate path")
			}
		} else {
			existing = pathNode{display: display, dir: wantDirectory}
		}
		if final {
			existing.explicit = true
		}
		nodes[key] = existing
	}
	return nil
}

func extractArchive(plan archivePlan, stage string) error {
	for _, directory := range plan.directories {
		if err := os.MkdirAll(filepath.Join(stage, filepath.FromSlash(directory)), 0o755); err != nil {
			return fmt.Errorf("create Website Replica directory: %w", err)
		}
	}
	for _, file := range plan.files {
		destination := filepath.Join(stage, filepath.FromSlash(file.name))
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return fmt.Errorf("create Website Replica file parent: %w", err)
		}
		input, err := file.entry.Open()
		if err != nil {
			return fmt.Errorf("open Website Replica ZIP entry: %w", err)
		}
		output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, file.mode)
		if err != nil {
			_ = input.Close()
			return fmt.Errorf("create staged Website Replica file: %w", err)
		}
		written, copyErr := io.Copy(output, io.LimitReader(input, int64(file.entry.UncompressedSize64)+1))
		syncOutputErr := output.Sync()
		if syncOutputErr == nil {
			runInstallTestCrashHook("source-file-synced")
		}
		closeOutputErr := output.Close()
		closeInputErr := input.Close()
		if copyErr != nil || syncOutputErr != nil || closeOutputErr != nil || closeInputErr != nil {
			return fmt.Errorf("extract Website Replica file: %w", errors.Join(copyErr, syncOutputErr, closeOutputErr, closeInputErr))
		}
		if uint64(written) != file.entry.UncompressedSize64 {
			return errors.New("Website Replica ZIP entry size changed during extraction")
		}
	}
	return nil
}

func writeLicense(filename string, record LicenseRecord) error {
	data, err := json.MarshalIndent(persistedLicense{
		SchemaVersion: 1, ReplicaID: record.ReplicaID, VersionID: record.VersionID,
		Version: record.Version, ArtifactDigest: record.ArtifactDigest, License: record.License,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Website Replica license: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return fmt.Errorf("create Website Replica license directory: %w", err)
	}
	output, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create Website Replica license: %w", err)
	}
	if _, err := output.Write(data); err != nil {
		_ = output.Close()
		return fmt.Errorf("write Website Replica license: %w", err)
	}
	if err := output.Sync(); err != nil {
		_ = output.Close()
		return fmt.Errorf("sync Website Replica license: %w", err)
	}
	runInstallTestCrashHook("license-file-synced")
	if err := output.Close(); err != nil {
		return fmt.Errorf("close Website Replica license: %w", err)
	}
	return nil
}

func syncStagedDirectories(stage string) error {
	var directories []string
	if err := filepath.WalkDir(stage, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			directories = append(directories, path)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("inspect staged Website Replica directories: %w", err)
	}
	sort.Slice(directories, func(left, right int) bool {
		return len(directories[left]) > len(directories[right])
	})
	for _, directory := range directories {
		if err := atomicfile.SyncDirectory(directory); err != nil {
			return fmt.Errorf("sync staged Website Replica directory: %w", err)
		}
		runInstallTestCrashHook("stage-directory-synced")
	}
	return nil
}

func recoverInstall(journalPath, target, stage string) error {
	temporaryPath := journalPath + installJournalTempSuffix
	removedTemporary, err := removeInterruptedJournalTemporary(temporaryPath, target, stage)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(journalPath)
	if errors.Is(err, fs.ErrNotExist) {
		if removedTemporary {
			if err := atomicfile.SyncDirectory(filepath.Dir(target)); err != nil {
				return fmt.Errorf("sync interrupted Website Replica journal cleanup: %w", err)
			}
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("read Website Replica install journal: %w", err)
	}
	_, err = decodeInstallJournal(data, target, stage)
	if err != nil {
		return errors.New("Website Replica install journal is invalid; refusing recovery")
	}
	if err := os.RemoveAll(stage); err != nil {
		return fmt.Errorf("clean interrupted Website Replica staging: %w", err)
	}
	if err := os.Remove(journalPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("complete Website Replica install recovery: %w", err)
	}
	if err := atomicfile.SyncDirectory(filepath.Dir(target)); err != nil {
		return fmt.Errorf("sync Website Replica install recovery: %w", err)
	}
	return nil
}

func removeInterruptedJournalTemporary(filename, target, stage string) (bool, error) {
	info, err := os.Lstat(filename)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect interrupted Website Replica journal: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 4096 {
		return false, errors.New("interrupted Website Replica journal temporary is invalid; refusing recovery")
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return false, fmt.Errorf("read interrupted Website Replica journal: %w", err)
	}
	if _, err := decodeInstallJournal(data, target, stage); err != nil {
		return false, errors.New("interrupted Website Replica journal temporary is invalid; refusing recovery")
	}
	if err := os.Remove(filename); err != nil {
		return false, fmt.Errorf("remove interrupted Website Replica journal: %w", err)
	}
	return true, nil
}

func decodeInstallJournal(data []byte, target, stage string) (installJournal, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var journal installJournal
	if decoder.Decode(&journal) != nil || decoder.Decode(&struct{}{}) != io.EOF || journal.SchemaVersion != 1 ||
		journal.Target != target || journal.Stage != stage || (journal.Status != "PREPARING" && journal.Status != "ACTIVATING") {
		return installJournal{}, errors.New("invalid Website Replica install journal")
	}
	return journal, nil
}

func writeJournal(filename string, journal installJournal) error {
	data, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporaryName := filename + installJournalTempSuffix
	temporary, err := os.OpenFile(temporaryName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create Website Replica install journal: %w", err)
	}
	defer os.Remove(temporaryName)
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	journalPointPrefix := strings.ToLower(journal.Status) + "-journal-"
	runInstallTestCrashHook(journalPointPrefix + "file-synced")
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := atomicfile.Replace(temporaryName, filename); err != nil {
		return fmt.Errorf("activate Website Replica install journal: %w", err)
	}
	runInstallTestCrashHook(journalPointPrefix + "renamed")
	if err := atomicfile.SyncDirectory(filepath.Dir(filename)); err != nil {
		return fmt.Errorf("sync Website Replica install journal: %w", err)
	}
	runInstallTestCrashHook(journalPointPrefix + "directory-synced")
	return nil
}

func runInstallTestCrashHook(point string) {
	if installTestCrashHook != nil {
		installTestCrashHook(point)
	}
}

func requireMissingPath(filename, reason string) error {
	if _, err := os.Lstat(filename); err == nil {
		return fmt.Errorf("refuse Website Replica installation: %s", reason)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("inspect Website Replica installation path: %w", err)
	}
	return nil
}
