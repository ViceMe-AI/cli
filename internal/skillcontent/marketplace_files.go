package skillcontent

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// PackageFilesPath is reserved for the installer, not authored package content.
// Go and Python share this ownership record. Runtime readiness digests are a
// separate contract and do not identify all of the publisher's resources.
const PackageFilesPath = ".viceme/package-files.json"

// PackageIdentityTransitionMatches allows repair to recognize a Python
// installation interrupted between its two identity metadata writes. It does
// not make that uncommitted generation ready for use.
func PackageIdentityTransitionMatches(directory, productID, ownerReleaseID, runtimeReleaseID string) bool {
	inventory, valid := readPackageFiles(directory, SkillProvenance{ProductID: productID})
	if !valid || inventory.PreviousReleaseID == "" || inventory.ReleaseID == inventory.PreviousReleaseID {
		return false
	}
	return (ownerReleaseID == inventory.ReleaseID && runtimeReleaseID == inventory.PreviousReleaseID) ||
		(ownerReleaseID == inventory.PreviousReleaseID && runtimeReleaseID == inventory.ReleaseID)
}

// PackageInstallationComplete checks the publisher identity and the durable
// installation commit marker. Authored files remain editable after installation;
// runtime readiness separately checks its platform resources and trial body.
func PackageInstallationComplete(directory, productID string) bool {
	if _, err := os.Lstat(filepath.Join(directory, PackageFilesPath)); errors.Is(err, fs.ErrNotExist) {
		return true
	} else if err != nil {
		return false
	}
	inventory, valid := readPackageFiles(directory, SkillProvenance{ProductID: productID})
	return valid && !inventory.Installing && inventory.PreviousReleaseID == "" && inventory.RecoveryDirectory == ""
}

func readManagedFile(directory, relative string) ([]byte, error) {
	current := directory
	components := strings.Split(relative, "/")
	for index, component := range components {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || (index < len(components)-1 && !info.IsDir()) || (index == len(components)-1 && !info.Mode().IsRegular()) {
			return nil, errors.New("managed package path is not a plain file")
		}
	}
	return os.ReadFile(current)
}

type packageFiles struct {
	SchemaVersion     int               `json:"schemaVersion"`
	ProductID         string            `json:"productId"`
	ReleaseID         string            `json:"releaseId"`
	PreviousReleaseID string            `json:"previousReleaseId,omitempty"`
	RecoveryDirectory string            `json:"recoveryDirectory,omitempty"`
	Installing        bool              `json:"installing,omitempty"`
	Files             map[string]string `json:"files"`
}

// LocalRecovery describes a complete previous installation whose old format
// cannot distinguish publisher files from user output. It is outside the live
// Skill, remains after commit, and is removed if the install rolls back.
type LocalRecovery struct {
	SkillDirectory string   `json:"skillDirectory"`
	Directory      string   `json:"directory"`
	Files          []string `json:"files"`
}

type marketplacePreservation struct {
	Legacy   bool
	Recovery *LocalRecovery
}

type localRecoveryRecord struct {
	SchemaVersion  int                      `json:"schemaVersion"`
	SkillDirectory string                   `json:"skillDirectory"`
	ProductID      string                   `json:"productId"`
	ReleaseID      string                   `json:"releaseId"`
	Files          map[string]recoveryEntry `json:"files"`
}

func readLocalRecovery(directory, skillDirectory, home string, provenance SkillProvenance) (*LocalRecovery, error) {
	if home == "" {
		return nil, errors.New("legacy recovery requires an explicit home")
	}
	var err error
	home, err = filepath.Abs(home)
	if err != nil {
		return nil, err
	}
	home, err = filepath.EvalSymlinks(home)
	if err != nil {
		return nil, err
	}
	base := filepath.Join(home, ".viceme", "recovery", provenance.ProductID)
	if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory {
		return nil, errors.New("legacy recovery directory is outside the Product recovery state")
	}
	// System aliases above HOME (such as macOS /var -> /private/var) may
	// differ between runners. The four recovery-state entries themselves must
	// remain plain directories, regardless of their spelling above HOME.
	plainPaths := []string{base, filepath.Dir(base), filepath.Dir(filepath.Dir(base))}
	for current, depth := directory, 0; depth < 4; current, depth = filepath.Dir(current), depth+1 {
		plainPaths = append(plainPaths, current)
	}
	for _, current := range plainPaths {
		info, err := os.Lstat(current)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("legacy recovery path is not a plain state directory")
		}
	}
	parent, parentErr := os.Stat(filepath.Dir(directory))
	expectedParent, expectedErr := os.Stat(base)
	if parentErr != nil || expectedErr != nil || !os.SameFile(parent, expectedParent) {
		return nil, errors.New("legacy recovery directory is outside the Product recovery state")
	}
	info, err := os.Stat(directory)
	if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o700) {
		return nil, errors.New("legacy recovery directory is not private")
	}
	data, err := readManagedFile(directory, "recovery.json")
	var record localRecoveryRecord
	if err != nil || decodeStrictJSON(data, &record) != nil || record.SchemaVersion != 1 || record.ProductID != provenance.ProductID || record.ReleaseID == "" || !filepath.IsAbs(record.SkillDirectory) {
		return nil, errors.New("legacy recovery metadata does not match this installation")
	}
	// Existing paths can name the same directory with different casing on
	// macOS and Windows, even after resolving symbolic-link ancestors.
	recordedSkill, recordedErr := os.Stat(record.SkillDirectory)
	actualSkill, actualErr := os.Stat(skillDirectory)
	if recordedErr != nil || actualErr != nil || !recordedSkill.IsDir() || !actualSkill.IsDir() || !os.SameFile(recordedSkill, actualSkill) {
		return nil, errors.New("legacy recovery metadata does not match this installation")
	}
	actual, err := recoveryTree(filepath.Join(directory, "files"))
	if err != nil {
		return nil, err
	}
	wantJSON, _ := json.Marshal(record.Files)
	actualJSON, _ := json.Marshal(actual)
	if string(wantJSON) != string(actualJSON) {
		return nil, errors.New("legacy recovery files no longer match their recorded snapshot")
	}
	files := make([]string, 0, len(actual))
	for name := range actual {
		files = append(files, name)
	}
	sort.Strings(files)
	return &LocalRecovery{SkillDirectory: skillDirectory, Directory: directory, Files: files}, nil
}

func writePackageFiles(directory string, provenance SkillProvenance) error {
	if _, err := os.Lstat(filepath.Join(directory, PackageFilesPath)); !errors.Is(err, fs.ErrNotExist) {
		return errors.New("authored package occupies the managed package file inventory")
	}
	inventory := packageFiles{SchemaVersion: 1, ProductID: provenance.ProductID, ReleaseID: provenance.ReleaseID, Files: map[string]string{}}
	err := filepath.WalkDir(directory, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(directory, filename)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if relative == installManifestPath || relative == PackageFilesPath {
			return nil
		}
		data, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		inventory.Files[relative] = fmt.Sprintf("%x", sha256.Sum256(data))
		return nil
	})
	if err != nil {
		return err
	}
	data, err := json.Marshal(inventory)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(directory, ".viceme"), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, PackageFilesPath), data, 0o644)
}

// Invalid or missing records cannot prove ownership. Preserve that complete
// generation for recovery instead of guessing that an unknown file is authored.
func readPackageFiles(directory string, provenance SkillProvenance) (packageFiles, bool) {
	var inventory packageFiles
	owner, err := readInstallManifest(directory)
	if err != nil || owner.ProductID != provenance.ProductID {
		return inventory, false
	}
	filename := filepath.Join(directory, PackageFilesPath)
	info, err := os.Lstat(filename)
	if err != nil || !info.Mode().IsRegular() {
		return inventory, false
	}
	data, err := readManagedFile(directory, PackageFilesPath)
	if err != nil || decodeStrictJSON(data, &inventory) != nil || inventory.SchemaVersion != 1 || inventory.ProductID != owner.ProductID || inventory.ReleaseID == "" ||
		(owner.ReleaseID != inventory.ReleaseID && (inventory.PreviousReleaseID == "" || owner.ReleaseID != inventory.PreviousReleaseID)) || len(inventory.Files) == 0 {
		return inventory, false
	}
	for relative, digest := range inventory.Files {
		if !fs.ValidPath(relative) || relative == "." || strings.Contains(relative, "\\") || relative == PackageFilesPath || relative == installManifestPath || !validSHA256Digest("sha256:"+digest) {
			return inventory, false
		}
	}
	for relative := range inventory.Files {
		for parent := filepath.ToSlash(filepath.Dir(relative)); parent != "."; parent = filepath.ToSlash(filepath.Dir(parent)) {
			if _, exists := inventory.Files[parent]; exists {
				return inventory, false
			}
		}
	}
	return inventory, inventory.Files["SKILL.md"] != ""
}

// The product and destination locks are held. Copy only unowned paths, without
// dereferencing links, into the already verified package staging directory.
func preserveMarketplaceFiles(source, staged string, inventory packageFiles) error {
	managedDirectories := map[string]bool{}
	for relative := range inventory.Files {
		for parent := filepath.ToSlash(filepath.Dir(relative)); parent != "."; parent = filepath.ToSlash(filepath.Dir(parent)) {
			managedDirectories[parent] = true
		}
	}
	return filepath.WalkDir(source, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, filename)
		if err != nil || relative == "." {
			return err
		}
		relative = filepath.ToSlash(relative)
		if _, owned := inventory.Files[relative]; owned || relative == installManifestPath || relative == PackageFilesPath {
			return nil
		}
		target := filepath.Join(staged, filepath.FromSlash(relative))
		if info, err := os.Lstat(target); err == nil {
			if info.IsDir() != entry.IsDir() && !managedDirectories[relative] {
				return errors.New("formal package conflicts with a local file or directory")
			}
			if !entry.IsDir() {
				localInfo, err := entry.Info()
				if err != nil || !localInfo.Mode().IsRegular() || !info.Mode().IsRegular() {
					return errors.New("formal package conflicts with a local file or symbolic link")
				}
				local, err := os.ReadFile(filename)
				if err != nil {
					return err
				}
				published, err := os.ReadFile(target)
				if err != nil {
					return err
				}
				if !bytes.Equal(local, published) {
					return errors.New("formal package conflicts with an unowned local file")
				}
			}
			return nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if entry.IsDir() && managedDirectories[relative] {
			return nil
		}
		if err := plainDirectoryParents(staged, filepath.Dir(target)); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.Mkdir(target, info.Mode().Perm())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return copyLinkPlain(filename, target)
		}
		if !info.Mode().IsRegular() {
			return errors.New("local Skill files contain an unsupported file type")
		}
		return copyFilePlain(filename, target, info.Mode().Perm())
	})
}

func plainDirectoryParents(root, directory string) error {
	relative, err := filepath.Rel(root, directory)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("local file escapes installation staging")
	}
	current := root
	if relative == "." {
		return nil
	}
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			if err := os.Mkdir(current, 0o755); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("local file parent conflicts with a package file or symbolic link")
		}
	}
	return nil
}

func copyLinkPlain(source, destination string) error {
	link, err := os.Readlink(source)
	if err != nil {
		return err
	}
	if info, err := os.Lstat(destination); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			if current, err := os.Readlink(destination); err == nil && current == link {
				return nil
			}
		}
		if info.IsDir() {
			return errors.New("symbolic link conflicts with an existing directory")
		}
		if err := os.Remove(destination); err != nil {
			return err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return os.Symlink(link, destination)
}

// Preserve a legacy generation inside the existing transaction. TrackPath
// means a failed copy or later activation failure removes the recovery copy;
// a committed transaction retains it, independently of disposable backups.
func (transaction *InstallTransaction) preserveLegacyMarketplace(operation *installOperation, home string) (*LocalRecovery, error) {
	owner, err := readInstallManifest(operation.Backup)
	if err != nil {
		return nil, err
	}
	if home == "" || !fs.ValidPath(owner.ProductID) || owner.ProductID == "." || strings.ContainsAny(owner.ProductID, "/\\") {
		return nil, errors.New("legacy Skill recovery requires a home and safe Product identity")
	}
	base := home
	for _, component := range []string{".viceme", "recovery", owner.ProductID} {
		base = filepath.Join(base, component)
		if info, err := os.Lstat(base); errors.Is(err, fs.ErrNotExist) {
			if err := os.Mkdir(base, 0o700); err != nil {
				return nil, err
			}
		} else if err != nil {
			return nil, err
		} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("legacy recovery parent is not a plain state directory")
		}
	}
	identifier, err := newInstallID()
	if err != nil {
		return nil, err
	}
	directory := filepath.Join(base, identifier)
	if err := transaction.TrackPath(directory); err != nil {
		return nil, err
	}
	if err := os.Mkdir(directory, 0o700); err != nil {
		return nil, err
	}
	destination := filepath.Join(directory, "files")
	if err := copyTreeOnDisk(operation.Backup, destination); err != nil {
		return nil, err
	}
	want, err := recoveryTree(operation.Backup)
	if err != nil {
		return nil, err
	}
	got, err := recoveryTree(destination)
	if err != nil {
		return nil, err
	}
	wantJSON, _ := json.Marshal(want)
	gotJSON, _ := json.Marshal(got)
	if string(wantJSON) != string(gotJSON) {
		return nil, errors.New("legacy Skill recovery copy did not match its source")
	}
	files := make([]string, 0, len(want))
	for relative := range want {
		files = append(files, relative)
	}
	sort.Strings(files)
	result := &LocalRecovery{SkillDirectory: operation.Destination, Directory: directory, Files: files}
	record := localRecoveryRecord{1, operation.Destination, owner.ProductID, owner.ReleaseID, want}
	data, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(directory, "recovery.json"), data, 0o600); err != nil {
		return nil, err
	}
	return result, nil
}

type recoveryEntry struct {
	Kind   string `json:"kind"`
	Mode   uint32 `json:"mode"`
	Digest string `json:"digest,omitempty"`
	Link   string `json:"link,omitempty"`
}

func recoveryTree(directory string) (map[string]recoveryEntry, error) {
	files := map[string]recoveryEntry{}
	err := filepath.WalkDir(directory, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(directory, filename)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		record := recoveryEntry{Mode: uint32(info.Mode().Perm())}
		if runtime.GOOS == "windows" {
			// Unix mode bits are unavailable here. Both installers record zero
			// rather than claim POSIX permissions or ACL verification.
			record.Mode = 0
		}
		switch {
		case info.IsDir():
			record.Kind = "directory"
		case info.Mode()&os.ModeSymlink != 0:
			record.Kind = "symlink"
			record.Link, err = os.Readlink(filename)
		case info.Mode().IsRegular():
			record.Kind = "file"
			var data []byte
			data, err = os.ReadFile(filename)
			record.Digest = fmt.Sprintf("%x", sha256.Sum256(data))
		default:
			return errors.New("legacy Skill contains an unsupported file type")
		}
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = record
		return nil
	})
	return files, err
}

// Match the existing file digest algorithm for regular package files, but
// hash the link entry itself for preserved user links. Never read its target.
func digestInstalledTree(directory string, include func(string) bool) (string, error) {
	var names []string
	err := filepath.WalkDir(directory, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(directory, filename)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if include(relative) {
			names = append(names, relative)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(names)
	hash := sha256.New()
	for _, name := range names {
		filename := filepath.Join(directory, filepath.FromSlash(name))
		info, err := os.Lstat(filename)
		if err != nil {
			return "", err
		}
		var data []byte
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(filename)
			if err != nil {
				return "", err
			}
			data = []byte("symlink\x00" + link)
		} else if info.Mode().IsRegular() {
			data, err = os.ReadFile(filename)
			if err != nil {
				return "", err
			}
		} else {
			return "", errors.New("Skill contains an unsupported filesystem entry")
		}
		_, _ = hash.Write([]byte(name))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(data)
		_, _ = hash.Write([]byte{0})
	}
	return fmt.Sprintf("sha256:%x", hash.Sum(nil)), nil
}
