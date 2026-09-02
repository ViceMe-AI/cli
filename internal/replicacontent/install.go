package replicacontent

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
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
	"github.com/ViceMe-AI/cli/internal/pathidentity"
	"github.com/ViceMe-AI/cli/internal/privatepath"
	"github.com/gofrs/flock"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"
)

const (
	LicenseFilePath      = ".viceme/replica-license.json"
	installOwnerPath     = ".viceme/replica-install-owner"
	installJournalSuffix = ".viceme-replica-install.json"
	installStageSuffix   = ".viceme-replica-stage"
	installLockSuffix    = ".viceme-replica-install.lock"
)

var (
	digestPattern            = regexp.MustCompile(`^[a-f0-9]{64}$`)
	windowsDevicePattern     = regexp.MustCompile(`(?i)^(?:CON|PRN|AUX|NUL|CONIN\$|CONOUT\$|(?:COM|LPT)[1-9¹²³])$`)
	pathLowerCaser           = cases.Lower(language.Und)
	pathUpperCaser           = cases.Upper(language.Und)
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

func AtomicInstallSupported() bool { return atomicInstallSupported() }

type installJournal struct {
	SchemaVersion int    `json:"schemaVersion"`
	Status        string `json:"status"`
	Target        string `json:"target"`
	Stage         string `json:"stage"`
	Nonce         string `json:"nonce"`
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
	return installArchive(archivePath, target, "", license)
}

func InstallArchiveAnchored(archivePath, target, targetParentID string, license LicenseRecord) (InstallResult, error) {
	if targetParentID == "" {
		return InstallResult{}, errors.New("Website Replica target parent identity is required")
	}
	return installArchive(archivePath, target, targetParentID, license)
}

func installArchive(archivePath, target, expectedParentID string, license LicenseRecord) (InstallResult, error) {
	if err := validateLicense(license); err != nil {
		return InstallResult{}, err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil || strings.TrimSpace(target) == "" {
		return InstallResult{}, errors.New("Website Replica target path is invalid")
	}
	absTarget = filepath.Clean(absTarget)
	parent := filepath.Dir(absTarget)
	parentInfo, err := os.Lstat(parent)
	if err != nil || !parentInfo.IsDir() || parentInfo.Mode()&os.ModeSymlink != 0 {
		return InstallResult{}, fmt.Errorf("Website Replica target parent is not a real existing directory: %w", err)
	}
	parentAnchor, err := pathidentity.OpenDirectory(parent)
	if err != nil {
		return InstallResult{}, fmt.Errorf("anchor Website Replica target parent: %w", err)
	}
	defer parentAnchor.Close()
	if expectedParentID != "" && parentAnchor.ID() != expectedParentID {
		return InstallResult{}, errors.New("Website Replica target parent changed before installation")
	}
	lockPath := absTarget + installLockSuffix
	lockCreated, err := privatepath.EnsureFile(lockPath)
	if err != nil {
		return InstallResult{}, fmt.Errorf("create private Website Replica target lock: %w", err)
	}
	if lockCreated {
		if err := atomicfile.SyncDirectory(parent); err != nil {
			return InstallResult{}, fmt.Errorf("sync Website Replica target lock creation: %w", err)
		}
	}
	installLock := flock.New(lockPath)
	if err := installLock.Lock(); err != nil {
		return InstallResult{}, fmt.Errorf("lock Website Replica target: %w", err)
	}
	defer installLock.Unlock()

	journalPath := absTarget + installJournalSuffix
	if err := recoverInstall(journalPath, absTarget); err != nil {
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

	stage, err := privatepath.CreateTempDirectory(parent, "."+filepath.Base(absTarget)+installStageSuffix+"-*")
	if err != nil {
		return InstallResult{}, fmt.Errorf("create private Website Replica staging directory: %w", err)
	}
	nonce, err := writeStageOwner(stage)
	if err != nil {
		_ = os.RemoveAll(stage)
		return InstallResult{}, err
	}
	journal := installJournal{SchemaVersion: 2, Status: "PREPARING", Target: absTarget, Stage: stage, Nonce: nonce}
	stageActive := true
	defer func() {
		if stageActive {
			cleanupPreparedInstall(journal, journalPath, parentAnchor)
		}
	}()
	if err := writeJournal(journalPath, journal); err != nil {
		return InstallResult{}, err
	}
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
	if err := parentAnchor.RenameNoReplace(filepath.Base(stage), filepath.Base(absTarget)); err != nil {
		return InstallResult{}, fmt.Errorf("activate Website Replica installation without overwrite: %w", err)
	}
	runInstallTestCrashHook("target-activated")
	stageActive = false
	if err := parentAnchor.Sync(); err != nil {
		return InstallResult{}, fmt.Errorf("sync Website Replica target activation: %w", err)
	}
	runInstallTestCrashHook("target-directory-synced")
	if err := removeStageOwner(absTarget, nonce); err != nil {
		return InstallResult{}, err
	}
	runInstallTestCrashHook("stage-owner-removed")
	if err := os.Remove(journalPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return InstallResult{}, fmt.Errorf("complete Website Replica install journal: %w", err)
	}
	runInstallTestCrashHook("journal-removed")
	if err := parentAnchor.Sync(); err != nil {
		return InstallResult{}, fmt.Errorf("sync Website Replica install completion: %w", err)
	}
	runInstallTestCrashHook("completion-directory-synced")
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

func ReadInstalledLicenseRecord(target string) (LicenseRecord, error) {
	info, err := os.Lstat(target)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return LicenseRecord{}, errors.New("Website Replica completion target is not a real directory")
	}
	filename := filepath.Join(target, filepath.FromSlash(LicenseFilePath))
	data, err := readBoundedRegularFile(filename, 64<<10)
	if err != nil {
		return LicenseRecord{}, fmt.Errorf("read installed Website Replica license: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var persisted persistedLicense
	if err := decoder.Decode(&persisted); err != nil || decoder.Decode(&struct{}{}) != io.EOF || persisted.SchemaVersion != 1 {
		return LicenseRecord{}, errors.New("installed Website Replica license is invalid")
	}
	record := LicenseRecord{
		ReplicaID:      persisted.ReplicaID,
		VersionID:      persisted.VersionID,
		Version:        persisted.Version,
		ArtifactDigest: persisted.ArtifactDigest,
		License:        persisted.License,
	}
	if err := validateLicense(record); err != nil {
		return LicenseRecord{}, err
	}
	return record, nil
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
	if reflect.DeepEqual(existingDocument, expectedDocument) {
		return true, nil
	}
	existingClaims, existingClaimsOK := licenseClaims(existingDocument)
	expectedClaims, expectedClaimsOK := licenseClaims(expectedDocument)
	if !existingClaimsOK || !expectedClaimsOK || !reflect.DeepEqual(existingClaims, expectedClaims) {
		return false, nil
	}
	if err := replaceInstalledLicense(licensePath, expected); err != nil {
		return false, err
	}
	return true, nil
}

func licenseClaims(document any) (any, bool) {
	envelope, ok := document.(map[string]any)
	if !ok {
		return nil, false
	}
	claims, ok := envelope["claims"]
	return claims, ok
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
	archiveChecksum := crc32.NewIEEE()
	archiveSize, copyErr := io.Copy(io.MultiWriter(archiveDigest, archiveChecksum), archived)
	closeErr := archived.Close()
	if copyErr != nil || closeErr != nil {
		return false, fmt.Errorf("hash Website Replica archive entry for recovery: %w", errors.Join(copyErr, closeErr))
	}
	return installedSize == archiveSize && installedSize == before.Size() && archiveChecksum.Sum32() == archiveEntry.CRC32 &&
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
	entries, err := validateArchiveStructure(file, info.Size())
	if err != nil {
		closeArchive()
		return nil, func() {}, err
	}
	reader, err := zip.NewReader(file, info.Size())
	if err != nil {
		closeArchive()
		return nil, func() {}, fmt.Errorf("open Website Replica ZIP directory: %w", err)
	}
	if err := validateZIPReader(reader, entries); err != nil {
		closeArchive()
		return nil, func() {}, err
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
		rootName := strings.SplitN(name, "/", 2)[0]
		if foldArchivePathSegment(rootName) == ".viceme" {
			return archivePlan{}, errors.New("Website Replica ZIP contains the reserved .viceme namespace")
		}
		mode := entry.Mode()
		if isDirectory {
			if mode.Type() != os.ModeDir || entry.UncompressedSize64 != 0 || entry.CompressedSize64 != 0 ||
				entry.Method != zip.Store || entry.CRC32 != 0 {
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
	unixMode := uint16(entry.ExternalAttrs >> 16)
	fileType := unixMode & 0o170000
	dosAttributes := entry.ExternalAttrs & 0xffff
	hasDirectorySuffix := strings.HasSuffix(raw, "/")
	isDirectory := hasDirectorySuffix || fileType == 0o040000
	if entry.Flags&0x1 != 0 || (entry.Method != zip.Store && entry.Method != zip.Deflate) || dosAttributes&(0x40|0x400) != 0 ||
		(dosAttributes&0x10 != 0 && !hasDirectorySuffix) ||
		(fileType != 0 && (fileType == 0o040000) != hasDirectorySuffix) ||
		(fileType == 0o100000 && hasDirectorySuffix) || (fileType != 0 && fileType != 0o040000 && fileType != 0o100000) ||
		unixMode&0o7000 != 0 {
		return "", false, errors.New("Website Replica ZIP contains a link, encrypted entry, or special file")
	}
	if isDirectory {
		raw = strings.TrimSuffix(raw, "/")
	} else if hasDirectorySuffix {
		return "", false, errors.New("Website Replica ZIP contains an invalid file path")
	}
	segments := strings.Split(raw, "/")
	if len(segments) > MaxArchivePathDepth {
		return "", false, errors.New("Website Replica ZIP path is too deep")
	}
	for index, segment := range segments {
		if segment == "" || len(segment) > MaxArchiveSegmentBytes || segment == "." || segment == ".." || strings.ContainsAny(segment, `<>:"|?*`) ||
			strings.HasSuffix(segment, ".") || strings.HasSuffix(segment, " ") || hasControlCharacter(segment) || isWindowsDeviceName(segment) ||
			!norm.NFC.IsNormalString(segment) ||
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
	base := strings.TrimRightFunc(strings.SplitN(segment, ".", 2)[0], func(character rune) bool {
		return isECMAScriptWhitespace(character)
	})
	return windowsDevicePattern.MatchString(base)
}

func isECMAScriptWhitespace(character rune) bool {
	return (character >= '\u0009' && character <= '\u000d') || character == '\u0020' || character == '\u00a0' ||
		character == '\u1680' || (character >= '\u2000' && character <= '\u200a') || character == '\u2028' ||
		character == '\u2029' || character == '\u202f' || character == '\u205f' || character == '\u3000' || character == '\ufeff'
}

func registerArchivePath(nodes map[string]pathNode, name string, isDirectory bool) error {
	segments := strings.Split(name, "/")
	display := ""
	key := ""
	keyBytes := 0
	for index := range segments {
		segment := segments[index]
		foldedSegment := foldArchivePathSegment(segment)
		if index == 0 {
			display = segment
			key = foldedSegment
		} else {
			display += "/" + segment
			key += "/" + foldedSegment
		}
		keyBytes += len(foldedSegment)
		if index > 0 {
			keyBytes++
		}
		if len(foldedSegment) > MaxArchiveSegmentBytes || keyBytes > MaxArchivePathBytes {
			return errors.New("Website Replica ZIP path expands beyond its portable collision-key budget")
		}
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
			if len(nodes) >= MaxArchiveEntries {
				return errors.New("Website Replica ZIP contains too many path nodes")
			}
			existing = pathNode{display: display, dir: wantDirectory}
		}
		if final {
			existing.explicit = true
		}
		nodes[key] = existing
	}
	return nil
}

func foldArchivePathSegment(segment string) string {
	folded := norm.NFKC.String(segment)
	folded = pathLowerCaser.String(folded)
	folded = pathUpperCaser.String(folded)
	return norm.NFKC.String(pathLowerCaser.String(folded))
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
		checksum := crc32.NewIEEE()
		written, copyErr := io.Copy(io.MultiWriter(output, checksum), io.LimitReader(input, int64(file.entry.UncompressedSize64)+1))
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
		if checksum.Sum32() != file.entry.CRC32 {
			return errors.New("Website Replica ZIP entry checksum changed during extraction")
		}
	}
	return nil
}

func writeLicense(filename string, record LicenseRecord) error {
	data, err := encodePersistedLicense(record)
	if err != nil {
		return err
	}
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

func encodePersistedLicense(record LicenseRecord) ([]byte, error) {
	data, err := json.MarshalIndent(persistedLicense{
		SchemaVersion: 1, ReplicaID: record.ReplicaID, VersionID: record.VersionID,
		Version: record.Version, ArtifactDigest: record.ArtifactDigest, License: record.License,
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode Website Replica license: %w", err)
	}
	return append(data, '\n'), nil
}

func replaceInstalledLicense(filename string, record LicenseRecord) error {
	data, err := encodePersistedLicense(record)
	if err != nil {
		return err
	}
	directory := filepath.Dir(filename)
	output, err := privatepath.CreateTempFile(directory, ".replica-license-*.tmp")
	if err != nil {
		return fmt.Errorf("create refreshed Website Replica license: %w", err)
	}
	temporaryName := output.Name()
	defer os.Remove(temporaryName)
	if _, err := output.Write(data); err != nil {
		_ = output.Close()
		return fmt.Errorf("write refreshed Website Replica license: %w", err)
	}
	if err := output.Sync(); err != nil {
		_ = output.Close()
		return fmt.Errorf("sync refreshed Website Replica license: %w", err)
	}
	if err := output.Close(); err != nil {
		return fmt.Errorf("close refreshed Website Replica license: %w", err)
	}
	if err := atomicfile.Replace(temporaryName, filename); err != nil {
		return fmt.Errorf("activate refreshed Website Replica license: %w", err)
	}
	if err := atomicfile.SyncDirectory(directory); err != nil {
		return fmt.Errorf("sync refreshed Website Replica license: %w", err)
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

func recoverInstall(journalPath, target string) error {
	if err := recoverJournalTemporaries(journalPath, target); err != nil {
		return err
	}
	data, err := readBoundedRegularFile(journalPath, 4096)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read Website Replica install journal: %w", err)
	}
	if err := privatepath.RequirePrivateFile(journalPath); err != nil {
		return fmt.Errorf("verify Website Replica install journal privacy: %w", err)
	}
	journal, err := decodeInstallJournal(data, target)
	if err != nil {
		return errors.New("Website Replica install journal is invalid; refusing recovery")
	}
	if err := cleanInterruptedInstall(journal); err != nil {
		return err
	}
	if err := os.Remove(journalPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("complete Website Replica install recovery: %w", err)
	}
	if err := atomicfile.SyncDirectory(filepath.Dir(target)); err != nil {
		return fmt.Errorf("sync Website Replica install recovery: %w", err)
	}
	return nil
}

func recoverJournalTemporaries(journalPath, target string) error {
	parent := filepath.Dir(journalPath)
	directory, err := os.Open(parent)
	if err != nil {
		return fmt.Errorf("open Website Replica journal directory: %w", err)
	}
	defer directory.Close()
	prefix := "." + filepath.Base(journalPath) + ".tmp-"
	removed := false
	for {
		entries, readErr := directory.ReadDir(128)
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), prefix) {
				continue
			}
			filename := filepath.Join(parent, entry.Name())
			data, readErr := readBoundedRegularFile(filename, 4096)
			if readErr != nil {
				continue
			}
			if err := privatepath.RequirePrivateFile(filename); err != nil {
				continue
			}
			journal, decodeErr := decodeInstallJournal(data, target)
			if decodeErr != nil {
				continue
			}
			if err := cleanInterruptedInstall(journal); err != nil {
				return err
			}
			if err := os.Remove(filename); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("remove interrupted Website Replica journal temporary: %w", err)
			}
			removed = true
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return fmt.Errorf("scan interrupted Website Replica journals: %w", readErr)
		}
	}
	if removed {
		if err := atomicfile.SyncDirectory(parent); err != nil {
			return fmt.Errorf("sync interrupted Website Replica journal cleanup: %w", err)
		}
	}
	return nil
}

func decodeInstallJournal(data []byte, target string) (installJournal, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var journal installJournal
	if decoder.Decode(&journal) != nil || decoder.Decode(&struct{}{}) != io.EOF || journal.SchemaVersion != 2 ||
		journal.Target != target || !validInstallStage(target, journal.Stage) || !validInstallNonce(journal.Nonce) ||
		(journal.Status != "PREPARING" && journal.Status != "ACTIVATING") {
		return installJournal{}, errors.New("invalid Website Replica install journal")
	}
	return journal, nil
}

func writeStageOwner(stage string) (string, error) {
	nonceBytes := make([]byte, 32)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", fmt.Errorf("create Website Replica staging ownership nonce: %w", err)
	}
	nonce := hex.EncodeToString(nonceBytes)
	directory := filepath.Join(stage, filepath.FromSlash(path.Dir(installOwnerPath)))
	if err := os.Mkdir(directory, 0o700); err != nil {
		return "", fmt.Errorf("create Website Replica staging ownership directory: %w", err)
	}
	filename := filepath.Join(stage, filepath.FromSlash(installOwnerPath))
	file, err := privatepath.CreateExclusiveFile(filename)
	if err != nil {
		return "", fmt.Errorf("create Website Replica staging ownership marker: %w", err)
	}
	writeErr := func() error {
		if _, err := file.WriteString(nonce + "\n"); err != nil {
			return err
		}
		if err := file.Sync(); err != nil {
			return err
		}
		return file.Close()
	}()
	if writeErr != nil {
		_ = file.Close()
		return "", fmt.Errorf("persist Website Replica staging ownership marker: %w", writeErr)
	}
	if err := atomicfile.SyncDirectory(directory); err != nil {
		return "", fmt.Errorf("sync Website Replica staging ownership marker: %w", err)
	}
	if err := atomicfile.SyncDirectory(stage); err != nil {
		return "", fmt.Errorf("sync Website Replica staging ownership directory: %w", err)
	}
	return nonce, nil
}

func validInstallNonce(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && value == strings.ToLower(value)
}

func verifyStageOwner(root, nonce string) error {
	info, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("Website Replica staging ownership root is not a real directory")
	}
	if err := privatepath.RequirePrivateDirectory(root); err != nil {
		return err
	}
	marker := filepath.Join(root, filepath.FromSlash(installOwnerPath))
	if err := privatepath.RequirePrivateFile(marker); err != nil {
		return err
	}
	data, err := readBoundedRegularFile(marker, 128)
	if err != nil || !bytes.Equal(data, []byte(nonce+"\n")) {
		return errors.New("Website Replica staging ownership marker is missing or invalid")
	}
	return nil
}

func removeStageOwner(root, nonce string) error {
	if err := verifyStageOwner(root, nonce); err != nil {
		return fmt.Errorf("verify activated Website Replica staging ownership: %w", err)
	}
	directory := filepath.Join(root, filepath.FromSlash(path.Dir(installOwnerPath)))
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(installOwnerPath))); err != nil {
		return fmt.Errorf("remove activated Website Replica staging ownership: %w", err)
	}
	if err := atomicfile.SyncDirectory(directory); err != nil {
		return fmt.Errorf("sync activated Website Replica staging ownership removal: %w", err)
	}
	return nil
}

func cleanInterruptedInstall(journal installJournal) error {
	stageInfo, stageErr := os.Lstat(journal.Stage)
	if stageErr == nil {
		if !stageInfo.IsDir() || stageInfo.Mode()&os.ModeSymlink != 0 {
			return errors.New("interrupted Website Replica staging path changed unexpectedly")
		}
		if err := verifyStageOwner(journal.Stage, journal.Nonce); err != nil {
			return fmt.Errorf("refuse unowned Website Replica staging cleanup: %w", err)
		}
		if err := os.RemoveAll(journal.Stage); err != nil {
			return fmt.Errorf("clean interrupted Website Replica staging: %w", err)
		}
		if err := atomicfile.SyncDirectory(filepath.Dir(journal.Target)); err != nil {
			return fmt.Errorf("sync interrupted Website Replica staging cleanup: %w", err)
		}
		return nil
	}
	if !errors.Is(stageErr, fs.ErrNotExist) {
		return fmt.Errorf("inspect interrupted Website Replica staging: %w", stageErr)
	}
	targetInfo, targetErr := os.Lstat(journal.Target)
	if errors.Is(targetErr, fs.ErrNotExist) {
		return nil
	}
	if targetErr != nil || journal.Status != "ACTIVATING" || !targetInfo.IsDir() || targetInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("interrupted Website Replica activation state is invalid")
	}
	if _, err := os.Lstat(filepath.Join(journal.Target, filepath.FromSlash(installOwnerPath))); errors.Is(err, fs.ErrNotExist) {
		// The marker can already be durably removed while the journal remains.
		// The caller still validates the complete installed tree before success.
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect activated Website Replica staging ownership: %w", err)
	}
	if err := removeStageOwner(journal.Target, journal.Nonce); err != nil {
		return fmt.Errorf("finish interrupted Website Replica activation: %w", err)
	}
	return nil
}

func writeJournal(filename string, journal installJournal) error {
	data, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary, err := privatepath.CreateTempFile(filepath.Dir(filename), "."+filepath.Base(filename)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create Website Replica install journal: %w", err)
	}
	temporaryName := temporary.Name()
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

func readBoundedRegularFile(filename string, maxBytes int64) ([]byte, error) {
	before, err := os.Lstat(filename)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 || before.Size() > maxBytes {
		return nil, errors.New("Website Replica install journal is not a bounded regular file")
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	after, statErr := file.Stat()
	data, readErr := io.ReadAll(io.LimitReader(file, maxBytes+1))
	closeErr := file.Close()
	if statErr != nil || readErr != nil || closeErr != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) || int64(len(data)) > maxBytes {
		return nil, errors.New("Website Replica install journal changed while reading")
	}
	return data, nil
}

func validInstallStage(target, stage string) bool {
	if !filepath.IsAbs(stage) || filepath.Clean(stage) != stage || filepath.Dir(stage) != filepath.Dir(target) {
		return false
	}
	prefix := "." + filepath.Base(target) + installStageSuffix + "-"
	suffix := strings.TrimPrefix(filepath.Base(stage), prefix)
	if suffix == "" || suffix == filepath.Base(stage) || len(suffix) > 32 {
		return false
	}
	for _, character := range suffix {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func cleanupPreparedInstall(journal installJournal, journalPath string, parent *pathidentity.Anchor) {
	if err := cleanInterruptedInstall(journal); err != nil {
		return
	}
	if err := parent.Sync(); err != nil {
		return
	}
	data, err := readBoundedRegularFile(journalPath, 4096)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil || privatepath.RequirePrivateFile(journalPath) != nil {
		return
	}
	persisted, err := decodeInstallJournal(data, journal.Target)
	if err != nil || persisted != journal {
		return
	}
	if err := os.Remove(journalPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return
	}
	_ = parent.Sync()
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
