package replicacontent

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/gofrs/flock"
)

const (
	LicenseFilePath      = ".viceme/replica-license.json"
	installJournalSuffix = ".viceme-replica-install.json"
	installStageSuffix   = ".viceme-replica-stage"
	installLockSuffix    = ".viceme-replica-install.lock"
)

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

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
	if err := requireMissingPath(absTarget, "target already exists"); err != nil {
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
	stageActive = false
	if err := os.Remove(journalPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return InstallResult{}, fmt.Errorf("complete Website Replica install journal: %w", err)
	}
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
	reader, err := zip.NewReader(file, info.Size())
	if err != nil {
		closeArchive()
		return nil, func() {}, fmt.Errorf("open Website Replica ZIP directory: %w", err)
	}
	return reader, closeArchive, nil
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
		if entry.UncompressedSize64 > 0 && (entry.CompressedSize64 == 0 || entry.UncompressedSize64 > entry.CompressedSize64*MaxCompressionRatio) {
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
	if raw == "" || !utf8.ValidString(raw) || strings.ContainsRune(raw, '\x00') || strings.Contains(raw, `\`) || strings.HasPrefix(raw, "/") {
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
	for index, segment := range segments {
		if segment == "" || segment == "." || segment == ".." || strings.Contains(segment, ":") ||
			(index == 0 && len(segment) >= 2 && ((segment[0] >= 'A' && segment[0] <= 'Z') || (segment[0] >= 'a' && segment[0] <= 'z')) && segment[1] == ':') {
			return "", false, errors.New("Website Replica ZIP contains an unsafe path segment")
		}
	}
	if raw == "" || path.IsAbs(raw) || path.Clean(raw) != raw || filepath.IsAbs(filepath.FromSlash(raw)) {
		return "", false, errors.New("Website Replica ZIP contains a non-canonical path")
	}
	return raw, isDirectory, nil
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
		closeOutputErr := output.Close()
		closeInputErr := input.Close()
		if copyErr != nil || closeOutputErr != nil || closeInputErr != nil {
			return fmt.Errorf("extract Website Replica file: %w", errors.Join(copyErr, closeOutputErr, closeInputErr))
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
	if err := os.WriteFile(filename, data, 0o644); err != nil {
		return fmt.Errorf("write Website Replica license: %w", err)
	}
	return nil
}

func recoverInstall(journalPath, target, stage string) error {
	data, err := os.ReadFile(journalPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read Website Replica install journal: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var journal installJournal
	if decoder.Decode(&journal) != nil || decoder.Decode(&struct{}{}) != io.EOF || journal.SchemaVersion != 1 ||
		journal.Target != target || journal.Stage != stage || (journal.Status != "PREPARING" && journal.Status != "ACTIVATING") {
		return errors.New("Website Replica install journal is invalid; refusing recovery")
	}
	if err := os.RemoveAll(stage); err != nil {
		return fmt.Errorf("clean interrupted Website Replica staging: %w", err)
	}
	if err := os.Remove(journalPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("complete Website Replica install recovery: %w", err)
	}
	return nil
}

func writeJournal(filename string, journal installJournal) error {
	data, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(filename), ".viceme-replica-journal-*.tmp")
	if err != nil {
		return fmt.Errorf("create Website Replica install journal: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, filename); err != nil {
		return fmt.Errorf("activate Website Replica install journal: %w", err)
	}
	return nil
}

func requireMissingPath(filename, reason string) error {
	if _, err := os.Lstat(filename); err == nil {
		return fmt.Errorf("refuse Website Replica installation: %s", reason)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("inspect Website Replica installation path: %w", err)
	}
	return nil
}
