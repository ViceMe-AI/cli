package publication

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"gopkg.in/yaml.v3"
)

const (
	MaxFiles = 1_000
	// The upload gate mirrors skillhub.cn: one hard size budget for the whole
	// package instead of separate per-file/zip budgets.
	MaxFileBytes         = 10 * 1024 * 1024
	MaxUncompressedBytes = 10 * 1024 * 1024
	MaxPackageBytes      = 10 * 1024 * 1024
	// Reserved for batch series publishing (one package carrying many skills).
	// A publication currently carries exactly one entry; the cap only becomes
	// observable once multi-entry packages are supported.
	MaxSkillEntries = 200
	// Keep two of the server's twelve media slots free for explicit replacement
	// uploads after the automatic package scan.
	MaxCandidates = 10
)

var (
	fixedZipTime       = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	sensitiveBaseNames = map[string]struct{}{
		".env": {}, ".npmrc": {}, ".pypirc": {},
		"desktop.ini": {}, "id_ed25519": {}, "id_rsa": {}, "thumbs.db": {},
	}
	forbiddenPathSegments = map[string]struct{}{
		".cache": {}, ".git": {}, ".hg": {}, ".idea": {}, ".mypy_cache": {},
		".next": {}, ".pytest_cache": {}, ".ruff_cache": {}, ".svn": {},
		".turbo": {}, ".venv": {}, ".viceme": {}, ".vscode": {},
		"__pycache__": {}, "coverage": {}, "node_modules": {}, "venv": {},
	}
	secretPatterns = []*regexp.Regexp{
		regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
		regexp.MustCompile(`(?i)sk-(?:proj-)?[A-Za-z0-9_-]{20,}`),
		regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{30,}`),
		regexp.MustCompile(`(?i)-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----`),
	}
	credentialAssignmentPattern = regexp.MustCompile(`(?i)(?:api[_-]?key|client[_-]?secret|access[_-]?token)\s*[:=]\s*["']?([A-Za-z0-9_./+=-]{16,})`)
	textExtensions              = map[string]struct{}{
		".css": {}, ".go": {}, ".html": {}, ".ini": {}, ".js": {}, ".json": {},
		".jsx": {}, ".md": {}, ".mjs": {}, ".py": {}, ".sh": {}, ".toml": {},
		".ts": {}, ".tsx": {}, ".txt": {}, ".yaml": {}, ".yml": {},
	}
)

type Candidate struct {
	RelativePath string `json:"relativePath"`
	FileName     string `json:"fileName"`
	ContentType  string `json:"contentType"`
	Digest       string `json:"digest"`
	SizeBytes    int64  `json:"sizeBytes"`
	Bytes        []byte `json:"-"`
}

func ReadCandidate(filename string) (Candidate, error) {
	abs, err := filepath.Abs(filename)
	if err != nil {
		return Candidate{}, output.Validation("MEDIA_PATH_INVALID", "could not resolve media path").WithCause(err)
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return Candidate{}, output.Validation("MEDIA_PATH_NOT_FOUND", "media file does not exist").WithCause(err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return Candidate{}, output.Validation("MEDIA_FILE_INVALID", "media must be a regular file")
	}
	if info.Size() <= 0 || info.Size() > MaxFileBytes {
		return Candidate{}, output.Validation("MEDIA_FILE_TOO_LARGE", "media must be between 1 byte and 10 MiB")
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return Candidate{}, output.Internal("MEDIA_READ_FAILED", "could not read media file", err)
	}
	contentType := imageContentType(abs, data)
	if contentType == "" {
		return Candidate{}, output.Validation("MEDIA_TYPE_UNSUPPORTED", "media must be PNG, JPEG, GIF, WebP, or AVIF")
	}
	return Candidate{FileName: filepath.Base(abs), ContentType: contentType, Digest: sha256Hex(data), SizeBytes: int64(len(data)), Bytes: data}, nil
}

type Package struct {
	SourcePath      string                       `json:"sourcePath"`
	BindingIdentity string                       `json:"bindingIdentity,omitempty"`
	Manifest        api.SkillPublicationManifest `json:"manifest"`
	Artifact        api.SkillPublicationFile     `json:"artifact"`
	Digest          string                       `json:"manifestDigest"`
	FileCount       int                          `json:"fileCount"`
	Candidates      []Candidate                  `json:"candidates"`
	Bytes           []byte                       `json:"-"`
}

type sourceEntry struct {
	name string
	mode fs.FileMode
	data []byte
}

type skillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// validationIssues accumulates every problem found in one package scan so the
// author fixes them all in a single round instead of hitting fail-fast errors
// one at a time. The first issue keeps its code as the error subtype so
// code-keyed guidance keeps working; later issues are appended with their own
// code prefixes in the message.
type validationIssues struct {
	issues []validationIssue
}

type validationIssue struct {
	subtype string
	message string
}

func (v *validationIssues) add(subtype, message string) {
	v.issues = append(v.issues, validationIssue{subtype: subtype, message: message})
}

func (v *validationIssues) addError(err error) {
	var cliErr *output.Error
	if errors.As(err, &cliErr) {
		v.add(cliErr.Subtype, cliErr.Message)
	}
}

func (v *validationIssues) empty() bool {
	return len(v.issues) == 0
}

func (v *validationIssues) err() error {
	if len(v.issues) == 0 {
		return nil
	}
	primary := v.issues[0]
	if len(v.issues) == 1 {
		return output.Validation(primary.subtype, primary.message)
	}
	message := primary.message + "\n\nAdditional problems in the same Skill package:"
	for _, issue := range v.issues[1:] {
		message += "\n" + issue.subtype + ": " + issue.message
	}
	return output.Validation(primary.subtype, message)
}

func Build(sourcePath string) (Package, error) {
	return build(sourcePath, "")
}

func BuildArchiveSubpath(sourcePath, subpath string) (Package, error) {
	return build(sourcePath, strings.TrimSpace(subpath))
}

// BuildRemoteArchive validates a server-normalized channel archive while
// preserving its exact bytes. The API receipt binds this byte digest, so the
// CLI must not rewrite the ZIP before publication creation.
func BuildRemoteArchive(sourcePath string) (Package, error) {
	abs, err := filepath.Abs(sourcePath)
	if err != nil {
		return Package{}, output.Validation("SKILL_PATH_INVALID", "could not resolve the Skill path").WithCause(err)
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return Package{}, output.Validation("SKILL_PATH_NOT_FOUND", "Skill path does not exist").WithCause(err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || !strings.EqualFold(filepath.Ext(abs), ".zip") {
		return Package{}, output.Validation("SKILL_SOURCE_RECEIPT_INVALID", "remote Skill source must be a regular ZIP file")
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return Package{}, output.Internal("SKILL_READ_FAILED", "failed to read remote Skill archive", err)
	}
	if len(raw) == 0 || len(raw) > MaxPackageBytes {
		return Package{}, output.Validation("SKILL_PACKAGE_TOO_LARGE", "remote Skill ZIP must be between 1 byte and 10 MB")
	}
	entries, err := readZip(abs)
	if err != nil {
		return Package{}, err
	}
	if err := validateEntryLimits(entries); err != nil {
		return Package{}, err
	}
	manifest, err := manifestFromEntries(entries, abs)
	if err != nil {
		return Package{}, err
	}
	manifestDigest, err := CanonicalDigest(manifest)
	if err != nil {
		return Package{}, err
	}
	return Package{
		SourcePath: abs,
		Manifest:   manifest,
		Artifact: api.SkillPublicationFile{
			Digest: sha256Hex(raw), SizeBytes: int64(len(raw)), FileName: safePackageName(manifest.Metadata.Title), ContentType: "application/zip",
		},
		Digest: manifestDigest, FileCount: len(entries), Candidates: listingCandidates(entries), Bytes: raw,
	}, nil
}

func build(sourcePath, archiveSubpath string) (Package, error) {
	abs, err := filepath.Abs(sourcePath)
	if err != nil {
		return Package{}, output.Validation("SKILL_PATH_INVALID", "could not resolve the Skill path").WithCause(err)
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return Package{}, output.Validation("SKILL_PATH_NOT_FOUND", "Skill path does not exist").WithCause(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return Package{}, output.Validation("SKILL_SYMLINK_REJECTED", "Skill source cannot be a symbolic link")
	}
	var entries []sourceEntry
	if info.IsDir() {
		entries, err = readDirectory(abs)
	} else if strings.EqualFold(filepath.Ext(abs), ".zip") {
		entries, err = readZip(abs)
	} else {
		return Package{}, output.Validation("SKILL_SOURCE_UNSUPPORTED", "Skill source must be a directory or ZIP file")
	}
	if err != nil {
		return Package{}, err
	}
	if archiveSubpath != "" {
		entries, err = selectArchiveSubpath(entries, archiveSubpath)
		if err != nil {
			return Package{}, err
		}
	}
	if err := validateEntryLimits(entries); err != nil {
		return Package{}, err
	}
	manifest, err := manifestFromEntries(entries, abs)
	if err != nil {
		return Package{}, err
	}
	if info.IsDir() {
		manifest.Spec.Source.Type = "WORKSPACE"
	} else {
		manifest.Spec.Source.Type = "ZIP"
	}
	archive, err := writeDeterministicZip(entries)
	if err != nil {
		return Package{}, err
	}
	if len(archive) > MaxPackageBytes {
		return Package{}, output.Validation("SKILL_PACKAGE_TOO_LARGE", "deterministic Skill ZIP exceeds 10 MB")
	}
	digest := sha256Hex(archive)
	manifestDigest, err := CanonicalDigest(manifest)
	if err != nil {
		return Package{}, err
	}
	candidates := listingCandidates(entries)
	return Package{
		SourcePath: abs,
		Manifest:   manifest,
		Artifact: api.SkillPublicationFile{
			Digest: digest, SizeBytes: int64(len(archive)), FileName: safePackageName(manifest.Metadata.Title), ContentType: "application/zip",
		},
		Digest: manifestDigest, FileCount: len(entries), Candidates: candidates, Bytes: archive,
	}, nil
}

func selectArchiveSubpath(entries []sourceEntry, subpath string) ([]sourceEntry, error) {
	normalized := strings.Trim(strings.ReplaceAll(subpath, "\\", "/"), "/")
	if normalized == "" || path.Clean(normalized) != normalized || strings.HasPrefix(normalized, "../") {
		return nil, output.Validation("GITHUB_PATH_INVALID", "--github-path must be a safe repository-relative directory")
	}
	selectPrefix := func(prefix string) []sourceEntry {
		selected := make([]sourceEntry, 0, len(entries))
		for _, entry := range entries {
			if !strings.HasPrefix(entry.name, prefix) {
				continue
			}
			entry.name = strings.TrimPrefix(entry.name, prefix)
			selected = append(selected, entry)
		}
		return selected
	}
	selected := selectPrefix(normalized + "/")
	if len(selected) == 0 {
		root := ""
		for _, entry := range entries {
			separator := strings.IndexByte(entry.name, '/')
			if separator <= 0 {
				root = ""
				break
			}
			candidate := entry.name[:separator]
			if root == "" {
				root = candidate
			} else if root != candidate {
				root = ""
				break
			}
		}
		if root != "" {
			selected = selectPrefix(root + "/" + normalized + "/")
		}
	}
	if len(selected) == 0 {
		return nil, output.Validation("GITHUB_PATH_NOT_FOUND", "the selected GitHub directory is empty or missing")
	}
	normalizedEntries, err := normalizeEntries(selected)
	if err != nil {
		return nil, err
	}
	if _, err := manifestFromEntries(normalizedEntries, subpath); err != nil {
		return nil, output.Validation("GITHUB_PATH_SKILL_MISSING", "the selected GitHub directory must contain SKILL.md at its root").WithCause(err)
	}
	return normalizedEntries, nil
}

func Customize(pkg Package, source api.SkillPublicationSource, edition api.SkillPublicationEdition) (Package, error) {
	pkg.Manifest.Spec.Source = source
	pkg.Manifest.Spec.Edition = edition
	digest, err := CanonicalDigest(pkg.Manifest)
	if err != nil {
		return Package{}, err
	}
	pkg.Digest = digest
	return pkg, nil
}

func CanonicalDigest(value any) (string, error) {
	encoded, err := marshalCanonical(value)
	if err != nil {
		return "", output.Internal("MANIFEST_DIGEST_FAILED", "failed to canonicalize publication manifest", err)
	}
	return sha256Hex(encoded), nil
}

func readDirectory(root string) ([]sourceEntry, error) {
	ignorePatterns, err := readViceMeIgnore(root)
	if err != nil {
		return nil, err
	}
	type pendingFile struct {
		relative string
		absolute string
		mode     fs.FileMode
	}
	var pending []pendingFile
	var issues validationIssues
	var total int64
	reportedTotalLimit, reportedFileLimit := false, false
	err = filepath.WalkDir(root, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if shouldIgnoreWorkspacePath(rel, entry.IsDir(), ignorePatterns) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			issues.add("SKILL_SYMLINK_REJECTED", "Skill package contains a symbolic link: "+rel)
			return nil
		}
		if !info.Mode().IsRegular() {
			issues.add("SKILL_SPECIAL_FILE_REJECTED", "Skill package contains a non-regular file: "+rel)
			return nil
		}
		if err := validatePath(rel); err != nil {
			issues.addError(err)
			return nil
		}
		if info.Size() > MaxFileBytes {
			issues.add("SKILL_FILE_TOO_LARGE", "Skill file exceeds 10 MiB: "+rel)
			return nil
		}
		total += info.Size()
		if !reportedTotalLimit && total > MaxUncompressedBytes {
			reportedTotalLimit = true
			issues.add("SKILL_PACKAGE_UNCOMPRESSED_TOO_LARGE", "Skill package exceeds 10 MB uncompressed")
		}
		pending = append(pending, pendingFile{relative: rel, absolute: filename, mode: info.Mode()})
		if !reportedFileLimit && len(pending) > MaxFiles {
			reportedFileLimit = true
			issues.add("SKILL_PACKAGE_TOO_MANY_FILES", "Skill package exceeds 1000 files")
		}
		return nil
	})
	if err != nil {
		var cliErr *output.Error
		if errors.As(err, &cliErr) {
			return nil, cliErr
		}
		return nil, output.Internal("SKILL_READ_FAILED", "failed to read Skill directory", err)
	}
	if !issues.empty() {
		return nil, issues.err()
	}
	entries := make([]sourceEntry, 0, len(pending))
	for _, file := range pending {
		data, err := os.ReadFile(file.absolute)
		if err != nil {
			return nil, output.Internal("SKILL_READ_FAILED", "failed to read Skill directory", err)
		}
		if err := validateContent(file.relative, data); err != nil {
			issues.addError(err)
			continue
		}
		entries = append(entries, sourceEntry{name: file.relative, mode: file.mode, data: data})
	}
	if !issues.empty() {
		return nil, issues.err()
	}
	return normalizeEntries(entries)
}

func readZip(filename string) ([]sourceEntry, error) {
	reader, err := zip.OpenReader(filename)
	if err != nil {
		return nil, output.Validation("SKILL_ZIP_INVALID", "Skill ZIP could not be opened").WithCause(err)
	}
	defer reader.Close()
	var issues validationIssues
	var total int64
	reportedTotalLimit, reportedFileLimit := false, false
	type pendingEntry struct {
		name string
		file *zip.File
	}
	var pending []pendingEntry
	seen := make(map[string]struct{})
	for _, file := range reader.File {
		name := strings.ReplaceAll(file.Name, "\\", "/")
		validationName := strings.TrimSuffix(name, "/")
		if shouldIgnorePackagedPath(validationName) {
			continue
		}
		if err := validatePath(validationName); err != nil {
			issues.addError(err)
			continue
		}
		mode := file.Mode()
		if mode&os.ModeSymlink != 0 || (!mode.IsRegular() && !mode.IsDir()) {
			issues.add("SKILL_SPECIAL_FILE_REJECTED", "Skill ZIP contains a link or special file: "+name)
			continue
		}
		if mode.IsDir() {
			continue
		}
		if _, exists := seen[name]; exists {
			issues.add("SKILL_ZIP_DUPLICATE_PATH", "Skill ZIP contains a duplicate path: "+name)
			continue
		}
		seen[name] = struct{}{}
		if file.UncompressedSize64 > MaxFileBytes {
			issues.add("SKILL_FILE_TOO_LARGE", "Skill file exceeds 10 MiB: "+name)
			continue
		}
		if file.UncompressedSize64 > 1024*1024 &&
			file.UncompressedSize64 > max(file.CompressedSize64, 1)*200 {
			issues.add("SKILL_ZIP_COMPRESSION_RATIO_EXCEEDED", "Skill ZIP entry has an unsafe compression ratio: "+name)
			continue
		}
		if file.Flags&0x1 != 0 {
			issues.add("SKILL_ZIP_ENCRYPTED", "Encrypted ZIP entries are not supported")
			continue
		}
		total += int64(file.UncompressedSize64)
		if !reportedTotalLimit && total > MaxUncompressedBytes {
			reportedTotalLimit = true
			issues.add("SKILL_PACKAGE_UNCOMPRESSED_TOO_LARGE", "Skill package exceeds 10 MB uncompressed")
		}
		pending = append(pending, pendingEntry{name: name, file: file})
		if !reportedFileLimit && len(pending) > MaxFiles {
			reportedFileLimit = true
			issues.add("SKILL_PACKAGE_TOO_MANY_FILES", "Skill package exceeds 1000 files")
		}
	}
	if !issues.empty() {
		return nil, issues.err()
	}
	entries := make([]sourceEntry, 0, len(pending))
	for _, item := range pending {
		data, err := readZipEntry(item.file)
		if err != nil {
			return nil, err
		}
		if len(data) > MaxFileBytes {
			issues.add("SKILL_FILE_TOO_LARGE", "Skill file exceeds 10 MiB: "+item.name)
			continue
		}
		if err := validateContent(item.name, data); err != nil {
			issues.addError(err)
			continue
		}
		entries = append(entries, sourceEntry{name: item.name, mode: item.file.Mode(), data: data})
	}
	if !issues.empty() {
		return nil, issues.err()
	}
	normalized, err := normalizeEntries(entries)
	if err != nil {
		return nil, err
	}
	return unwrapSingleZipRoot(normalized)
}

// readZipEntry streams one entry with the package file budget as a hard cap so
// a lying ZIP header cannot balloon memory during the content scan.
func readZipEntry(file *zip.File) ([]byte, error) {
	stream, err := file.Open()
	if err != nil {
		return nil, output.Validation("SKILL_ZIP_INVALID", "Skill ZIP entry could not be opened").WithCause(err)
	}
	data, readErr := io.ReadAll(io.LimitReader(stream, MaxFileBytes+1))
	closeErr := stream.Close()
	if readErr != nil || closeErr != nil {
		return nil, output.Validation("SKILL_ZIP_INVALID", "Skill ZIP entry could not be read").WithCause(errors.Join(readErr, closeErr))
	}
	return data, nil
}

// unwrapSingleZipRoot accepts the directory wrapper produced by common source
// archive downloads, for example repository-main/SKILL.md. It only unwraps
// when every file belongs to one top-level directory and SKILL.md is directly
// inside that directory. Ambiguous or deeper layouts remain invalid.
func unwrapSingleZipRoot(entries []sourceEntry) ([]sourceEntry, error) {
	for _, entry := range entries {
		if entry.name == "SKILL.md" {
			return entries, nil
		}
	}

	root := ""
	for _, entry := range entries {
		separator := strings.IndexByte(entry.name, '/')
		if separator <= 0 {
			return entries, nil
		}
		entryRoot := entry.name[:separator]
		if root == "" {
			root = entryRoot
			continue
		}
		if entryRoot != root {
			return entries, nil
		}
	}

	prefix := root + "/"
	manifestPath := prefix + "SKILL.md"
	hasManifest := false
	for _, entry := range entries {
		if entry.name == manifestPath {
			hasManifest = true
			break
		}
	}
	if !hasManifest {
		return entries, nil
	}

	unwrapped := make([]sourceEntry, 0, len(entries))
	for _, entry := range entries {
		name := strings.TrimPrefix(entry.name, prefix)
		if err := validatePath(name); err != nil {
			return nil, err
		}
		unwrapped = append(unwrapped, sourceEntry{name: name, mode: entry.mode, data: entry.data})
	}
	return normalizeEntries(unwrapped)
}

func normalizeEntries(entries []sourceEntry) ([]sourceEntry, error) {
	if len(entries) == 0 {
		return nil, output.Validation("SKILL_PACKAGE_EMPTY", "Skill package contains no files")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	return entries, nil
}

func writeDeterministicZip(entries []sourceEntry) ([]byte, error) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Deflate}
		header.SetModTime(fixedZipTime)
		if entry.mode&0o111 != 0 {
			header.SetMode(0o755)
		} else {
			header.SetMode(0o644)
		}
		out, err := writer.CreateHeader(header)
		if err != nil {
			return nil, output.Internal("SKILL_ZIP_WRITE_FAILED", "failed to create deterministic Skill ZIP", err)
		}
		if _, err := out.Write(entry.data); err != nil {
			return nil, output.Internal("SKILL_ZIP_WRITE_FAILED", "failed to write deterministic Skill ZIP", err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, output.Internal("SKILL_ZIP_WRITE_FAILED", "failed to finish deterministic Skill ZIP", err)
	}
	return buffer.Bytes(), nil
}

func manifestFromEntries(entries []sourceEntry, sourcePath string) (api.SkillPublicationManifest, error) {
	var skill []byte
	for _, entry := range entries {
		if entry.name == "SKILL.md" {
			skill = entry.data
			break
		}
	}
	if len(skill) == 0 {
		return api.SkillPublicationManifest{}, output.Validation("SKILL_MANIFEST_MISSING", "Skill package must contain SKILL.md at its root")
	}
	frontmatter, err := parseSkillFrontmatter(skill)
	if err != nil {
		return api.SkillPublicationManifest{}, err
	}
	if err := validateFrontmatterLimits(frontmatter); err != nil {
		return api.SkillPublicationManifest{}, err
	}
	if !skillNamePattern.MatchString(frontmatter.Name) {
		return api.SkillPublicationManifest{}, output.Validation("SKILL_PACKAGE_NAME_INVALID",
			"SKILL.md name is the install identifier and must use lowercase letters, digits, and single hyphens"+suggestedSkillNameSuffix(sourcePath))
	}
	// A missing frontmatter description no longer blocks the upload: derive a
	// deterministic fallback from the SKILL.md body so buyer-facing copy stays
	// non-empty. The author's own description always wins when present.
	summary := frontmatter.Description
	if summary == "" {
		summary = deriveSkillSummary(skill, frontmatter.Name)
	}
	return api.SkillPublicationManifest{
		APIVersion: "publication.viceme.ai/v1alpha1", Kind: "Skill",
		Metadata: api.SkillPublicationMetadata{Title: frontmatter.Name, Summary: summary},
		Spec: api.SkillPublicationSpec{
			PublishMode: "DOWNLOADABLE_SKILL",
			Source:      api.SkillPublicationSource{Type: "WORKSPACE", Entry: "SKILL.md"},
			Edition: api.SkillPublicationEdition{
				Key: "standard", Title: frontmatter.Name, SortOrder: 0, Highlights: []string{summary},
			},
			Sale: api.SkillPublicationSale{Currency: "CNY", PriceMinor: nil, Entitlement: "PERMANENT_DOWNLOAD"},
		},
	}, nil
}

// The frontmatter checks below mirror the server-side skill-package-validator
// and its single-sourced constants in packages/contracts/src/skill-publications.ts
// (SKILL_PACKAGE_NAME_PATTERN and the name/description lengths). The entry
// limits reuse the Max* constants above, which carry the same values. Keep the
// copies aligned when either side changes.

var skillNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

const (
	skillNameMaxRunes        = 64
	skillDescriptionMaxRunes = 500
)

func validateFrontmatterLimits(frontmatter skillFrontmatter) error {
	if utf8.RuneCountInString(frontmatter.Name) > skillNameMaxRunes {
		return output.Validation("SKILL_FRONTMATTER_INVALID", fmt.Sprintf("SKILL.md name must be at most %d characters", skillNameMaxRunes))
	}
	if utf8.RuneCountInString(frontmatter.Description) > skillDescriptionMaxRunes {
		return output.Validation("SKILL_FRONTMATTER_INVALID", fmt.Sprintf("SKILL.md description must be at most %d characters", skillDescriptionMaxRunes))
	}
	return nil
}

// suggestedSkillNameSuffix derives a valid install identifier from the source
// directory or archive name so the error itself carries a usable fix.
func suggestedSkillNameSuffix(sourcePath string) string {
	base := strings.ToLower(filepath.Base(sourcePath))
	base = strings.TrimSuffix(base, filepath.Ext(base))
	var builder strings.Builder
	for _, character := range base {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			builder.WriteRune(character)
		} else if builder.Len() > 0 && !strings.HasSuffix(builder.String(), "-") {
			builder.WriteByte('-')
		}
	}
	suggestion := strings.Trim(builder.String(), "-")
	if suggestion == "" || !skillNamePattern.MatchString(suggestion) {
		return ""
	}
	return fmt.Sprintf("; for example rename it to %q based on this source, and adjust the display title via the publication draft update instead", suggestion)
}

// validateEntryLimits enforces the uncompressed budgets locally so a highly
// compressible oversized file fails before any upload, mirroring the server's
// per-file and total uncompressed checks. The file count itself is already
// enforced while reading the source.
func validateEntryLimits(entries []sourceEntry) error {
	total := 0
	for _, entry := range entries {
		if len(entry.data) > MaxFileBytes {
			return output.Validation("SKILL_PACKAGE_FILE_TOO_LARGE", "Skill package file exceeds the size limit: "+entry.name)
		}
		total += len(entry.data)
	}
	if total > MaxUncompressedBytes {
		return output.Validation("SKILL_PACKAGE_TOO_LARGE", "Skill package exceeds the uncompressed size limit")
	}
	return nil
}

func parseSkillFrontmatter(data []byte) (skillFrontmatter, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return skillFrontmatter{}, output.Validation("SKILL_FRONTMATTER_INVALID", "SKILL.md must start with YAML frontmatter")
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		return skillFrontmatter{}, output.Validation("SKILL_FRONTMATTER_INVALID", "SKILL.md frontmatter is not closed")
	}
	var raw map[string]any
	block := []byte(text[4 : 4+end])
	if err := yaml.Unmarshal(block, &raw); err != nil {
		return skillFrontmatter{}, output.Validation("SKILL_FRONTMATTER_INVALID", "SKILL.md frontmatter is invalid YAML")
	}
	var result skillFrontmatter
	if err := yaml.Unmarshal(block, &result); err != nil {
		return skillFrontmatter{}, output.Validation("SKILL_FRONTMATTER_INVALID", "SKILL.md frontmatter could not be decoded")
	}
	result.Name = strings.TrimSpace(result.Name)
	result.Description = strings.TrimSpace(result.Description)
	// The name is the skill's identity for agents, install naming, and edition
	// keys, so it stays a hard upload requirement. The description is only
	// presentation and may be absent; manifestFromEntries derives a fallback.
	var issues validationIssues
	if result.Name == "" {
		issues.add("SKILL_TITLE_INVALID", "SKILL.md frontmatter name is required: add a name field with the skill's name (at most 64 characters)")
	} else if len([]rune(result.Name)) > 64 {
		issues.add("SKILL_TITLE_INVALID", "SKILL.md frontmatter name must be at most 64 characters")
	}
	if len([]rune(result.Description)) > 500 {
		issues.add("SKILL_SUMMARY_INVALID", "SKILL.md frontmatter description must be at most 500 characters")
	}
	if err := issues.err(); err != nil {
		return skillFrontmatter{}, err
	}
	return result, nil
}

// deriveSkillSummary extracts the fallback manifest summary from the SKILL.md
// body when the frontmatter has no description: the first prose line, then the
// first heading, then the skill name. The result is deterministic for a given
// package so the manifest digest never wobbles.
func deriveSkillSummary(skill []byte, fallback string) string {
	text := strings.ReplaceAll(string(skill), "\r\n", "\n")
	if start := strings.Index(text, "\n---\n"); start >= 0 {
		text = text[start+len("\n---\n"):]
	}
	heading := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if heading == "" {
				heading = strings.TrimSpace(strings.TrimLeft(line, "# "))
			}
			continue
		}
		return truncateRunes(line, 500)
	}
	if heading != "" {
		return truncateRunes(heading, 500)
	}
	return fallback
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func listingCandidates(entries []sourceEntry) []Candidate {
	result := make([]Candidate, 0, MaxCandidates)
	for _, entry := range entries {
		contentType := imageContentType(entry.name, entry.data)
		if contentType == "" {
			continue
		}
		copyOfBytes := append([]byte(nil), entry.data...)
		result = append(result, Candidate{
			RelativePath: entry.name, FileName: path.Base(entry.name), ContentType: contentType,
			Digest: sha256Hex(entry.data), SizeBytes: int64(len(entry.data)), Bytes: copyOfBytes,
		})
		if len(result) == MaxCandidates {
			break
		}
	}
	return result
}

func validatePath(name string) error {
	if name == "" || strings.HasPrefix(name, "/") || path.Clean(name) != name || name == "." || strings.HasPrefix(name, "../") || strings.Contains(name, "\\") || strings.ContainsRune(name, 0) {
		return output.Validation("SKILL_PATH_UNSAFE", "Skill package contains an unsafe path: "+name)
	}
	if len(name) > 512 {
		return output.Validation("SKILL_PATH_TOO_LONG", "Skill package path exceeds 512 characters")
	}
	base := strings.ToLower(path.Base(name))
	for _, segment := range strings.Split(strings.ToLower(name), "/") {
		if _, forbidden := forbiddenPathSegments[segment]; forbidden {
			return output.Validation("SKILL_FORBIDDEN_PATH", "Skill package contains a forbidden path: "+name)
		}
	}
	if _, sensitive := sensitiveBaseNames[base]; sensitive || strings.HasPrefix(base, ".env.") {
		return output.Validation("SKILL_SENSITIVE_FILE", "Skill package contains a sensitive file: "+name)
	}
	return nil
}

func validateContent(name string, data []byte) error {
	if hasSecret(data) {
		return output.Validation("SKILL_SECRET_DETECTED", "Skill package appears to contain a secret: "+name)
	}
	ext := strings.ToLower(path.Ext(name))
	if _, expectedText := textExtensions[ext]; expectedText && !validText(data) {
		return output.Validation("SKILL_TEXT_ENCODING_INVALID", "Text-like Skill file must be valid UTF-8: "+name)
	}
	return nil
}

func hasSecret(data []byte) bool {
	candidates := [][]byte{data}
	if decoded, ok := decodeUTF16(data, true); ok {
		candidates = append(candidates, decoded)
	}
	if decoded, ok := decodeUTF16(data, false); ok {
		candidates = append(candidates, decoded)
	}
	for _, candidate := range candidates {
		for _, pattern := range secretPatterns {
			if pattern.Match(candidate) {
				return true
			}
		}
		for _, match := range credentialAssignmentPattern.FindAllSubmatch(candidate, -1) {
			if len(match) == 2 && !isCredentialPlaceholder(string(match[1])) {
				return true
			}
		}
	}
	return false
}

func isCredentialPlaceholder(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "your-api-key-here", "your-client-secret-here", "your-access-token-here",
		"example-api-key", "example-client-secret", "example-access-token",
		"replace-with-api-key", "replace-with-client-secret", "replace-with-access-token":
		return true
	default:
		return false
	}
}

func validText(data []byte) bool {
	if bytes.IndexByte(data, 0) >= 0 {
		return false
	}
	return utf8.Valid(data)
}

func decodeUTF16(data []byte, littleEndian bool) ([]byte, bool) {
	if len(data) < 4 || len(data)%2 != 0 {
		return nil, false
	}
	units := make([]uint16, len(data)/2)
	for index := range units {
		if littleEndian {
			units[index] = uint16(data[index*2]) | uint16(data[index*2+1])<<8
		} else {
			units[index] = uint16(data[index*2])<<8 | uint16(data[index*2+1])
		}
	}
	decoded := string(utf16.Decode(units))
	if !utf8.ValidString(decoded) {
		return nil, false
	}
	return []byte(decoded), true
}

func ignoredDirectory(name string) bool {
	_, ignored := forbiddenPathSegments[strings.ToLower(name)]
	return ignored
}

func readViceMeIgnore(root string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(root, ".vicemeignore"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, output.Validation("SKILL_IGNORE_READ_FAILED", "could not read .vicemeignore").WithCause(err)
	}
	if !utf8.Valid(data) {
		return nil, output.Validation("SKILL_IGNORE_INVALID", ".vicemeignore must be UTF-8 text")
	}
	var patterns []string
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "!") {
			return nil, output.Validation("SKILL_IGNORE_INVALID", ".vicemeignore negation patterns are not supported")
		}
		line = strings.TrimPrefix(filepath.ToSlash(line), "/")
		if line == "" || strings.Contains(line, "\\") || strings.Contains(line, "..") {
			return nil, output.Validation("SKILL_IGNORE_INVALID", ".vicemeignore contains an unsafe pattern")
		}
		patterns = append(patterns, line)
	}
	return patterns, nil
}

func shouldIgnoreWorkspacePath(name string, directory bool, patterns []string) bool {
	if shouldIgnorePackagedPath(name) {
		return true
	}
	for _, pattern := range patterns {
		directoryPattern := strings.HasSuffix(pattern, "/")
		pattern = strings.TrimSuffix(pattern, "/")
		if prefix, ok := strings.CutSuffix(pattern, "/**"); ok && (name == prefix || strings.HasPrefix(name, prefix+"/")) {
			return true
		}
		if directoryPattern && !directory {
			if name == pattern || strings.HasPrefix(name, pattern+"/") {
				return true
			}
			continue
		}
		matched, err := path.Match(pattern, name)
		if err == nil && matched {
			return true
		}
		if !strings.Contains(pattern, "/") {
			matched, _ = path.Match(pattern, path.Base(name))
			if matched {
				return true
			}
		}
	}
	return false
}

func shouldIgnorePackagedPath(name string) bool {
	if name == "" {
		return false
	}
	base := strings.ToLower(path.Base(name))
	if base == ".vicemeignore" || base == ".ds_store" || strings.HasPrefix(base, "._") || base == ".env" || strings.HasPrefix(base, ".env.") {
		return true
	}
	for _, segment := range strings.Split(strings.ToLower(name), "/") {
		if segment == "__macosx" {
			return true
		}
		if ignoredDirectory(segment) {
			return true
		}
	}
	return false
}

func imageContentType(_ string, data []byte) string {
	detected := http.DetectContentType(data)
	switch detected {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return detected
	}
	if len(data) >= 12 && string(data[4:8]) == "ftyp" {
		brand := string(data[8:12])
		if brand == "avif" || brand == "avis" {
			return "image/avif"
		}
	}
	return ""
}

func safePackageName(title string) string {
	var builder strings.Builder
	for _, character := range strings.ToLower(title) {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			builder.WriteRune(character)
		} else if builder.Len() > 0 && !strings.HasSuffix(builder.String(), "-") {
			builder.WriteByte('-')
		}
	}
	value := strings.Trim(builder.String(), "-")
	if value == "" {
		value = "skill"
	}
	if len(value) > 80 {
		value = strings.Trim(value[:80], "-")
	}
	return value + ".zip"
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
