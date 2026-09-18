// Package channeldelivery owns the author-side channel delivery state: the
// local delivery record (recovery progress for the fixed-branch PR flow) and
// the safe in-place application of generated channel files onto the author's
// Skill directory. Consumer installation state (.viceme/package-files.json,
// install manifests) stays with the installer; a delivery record never
// impersonates an installation.
package channeldelivery

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/output"
)

// File is one channel package file. Modes follow the package convention:
// 0o755 for executable entries, 0o644 otherwise.
type File struct {
	Data []byte
	Mode os.FileMode
}

// Conflict describes one path whose local content was preserved instead of
// being overwritten because it carries uncommitted author work.
type Conflict struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// ApplyResult reports exactly what happened to the directory.
type ApplyResult struct {
	Written   []string   `json:"written"`
	Unchanged []string   `json:"unchanged"`
	Removed   []string   `json:"removed"`
	Conflicts []Conflict `json:"conflicts"`
}

// channelZipTime mirrors the publication packager's fixed timestamp so the
// same file set always produces the same ZIP bytes.
var channelZipTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

func digestHex(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

// WriteZip renders the channel package as a deterministic ZIP: entries sorted
// by path, Deflate, fixed timestamp, canonical 0o755/0o644 modes. The channel
// directory and the channel ZIP are built from this one byte set.
func WriteZip(files map[string]File) ([]byte, error) {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, name := range names {
		file := files[name]
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetModTime(channelZipTime)
		if file.Mode&0o111 != 0 {
			header.SetMode(0o755)
		} else {
			header.SetMode(0o644)
		}
		entry, err := writer.CreateHeader(header)
		if err != nil {
			return nil, output.Internal("SKILL_CHANNEL_ZIP_WRITE_FAILED", "could not create the channel ZIP", err)
		}
		if _, err := entry.Write(file.Data); err != nil {
			return nil, output.Internal("SKILL_CHANNEL_ZIP_WRITE_FAILED", "could not write the channel ZIP", err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, output.Internal("SKILL_CHANNEL_ZIP_WRITE_FAILED", "could not finish the channel ZIP", err)
	}
	return buffer.Bytes(), nil
}

// SaveZip writes channel ZIP bytes durably (temp file + rename).
func SaveZip(filename string, data []byte) error {
	directory := filepath.Dir(filename)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return output.Internal("SKILL_CHANNEL_ZIP_SAVE_FAILED", "could not create the channel ZIP directory", err)
	}
	temp, err := os.CreateTemp(directory, ".channel-*.zip.tmp")
	if err != nil {
		return output.Internal("SKILL_CHANNEL_ZIP_SAVE_FAILED", "could not stage the channel ZIP", err)
	}
	name := temp.Name()
	defer os.Remove(name)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return output.Internal("SKILL_CHANNEL_ZIP_SAVE_FAILED", "could not write the channel ZIP", err)
	}
	if err := temp.Close(); err != nil {
		return output.Internal("SKILL_CHANNEL_ZIP_SAVE_FAILED", "could not close the channel ZIP", err)
	}
	if err := os.Chmod(name, 0o644); err != nil {
		return output.Internal("SKILL_CHANNEL_ZIP_SAVE_FAILED", "could not finalize the channel ZIP mode", err)
	}
	if err := os.Rename(name, filename); err != nil {
		return output.Internal("SKILL_CHANNEL_ZIP_SAVE_FAILED", "could not publish the channel ZIP", err)
	}
	return nil
}

// Apply writes only the generated channel files into root. owned maps each
// path to the set of on-disk digests this process is allowed to replace (the
// recorded previous delivery, plus content provably equivalent to the new
// build, e.g. the ungated SKILL.md that matches the published trial body).
// Anything else on disk is treated as uncommitted author work: it is kept and
// reported as a conflict, never silently overwritten. Stale files from a
// previous delivery disappear only when their bytes still match the record;
// unknown author files are never removed. Every write and remove first walks
// the whole path chain from root: a symbolic link anywhere above the target
// (for example a linked subdirectory) is a conflict, never followed.
func Apply(root string, files map[string]File, owned map[string][]string) (*ApplyResult, error) {
	result := &ApplyResult{}
	allowed := make(map[string]map[string]struct{}, len(owned))
	for name, digests := range owned {
		set := make(map[string]struct{}, len(digests))
		for _, digest := range digests {
			set[digest] = struct{}{}
		}
		allowed[name] = set
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		file := files[name]
		next := digestHex(file.Data)
		if plain, current, err := resolveManagedPath(root, name); err != nil {
			return nil, err
		} else if !plain {
			result.Conflicts = append(result.Conflicts, Conflict{Path: name,
				Reason: "the local path chain contains a symbolic link or non-directory"})
			continue
		} else if current != nil {
			currentDigest := digestHex(current)
			if currentDigest == next {
				result.Unchanged = append(result.Unchanged, name)
				continue
			}
			if set, ok := allowed[name]; !ok || !containsDigest(set, currentDigest) {
				result.Conflicts = append(result.Conflicts, Conflict{Path: name,
					Reason: "local content differs from both the recorded delivery and the new build"})
				continue
			}
		}
		if err := writeFile(filepath.Join(root, filepath.FromSlash(name)), file); err != nil {
			return nil, err
		}
		result.Written = append(result.Written, name)
	}
	stale := make([]string, 0, len(owned))
	for name := range owned {
		if _, kept := files[name]; kept {
			continue
		}
		stale = append(stale, name)
	}
	sort.Strings(stale)
	for _, name := range stale {
		if plain, current, err := resolveManagedPath(root, name); err != nil {
			return nil, err
		} else if !plain {
			result.Conflicts = append(result.Conflicts, Conflict{Path: name,
				Reason: "the local path chain contains a symbolic link or non-directory"})
			continue
		} else if current == nil {
			continue
		} else if set, ok := allowed[name]; ok && containsDigest(set, digestHex(current)) {
			if err := os.Remove(filepath.Join(root, filepath.FromSlash(name))); err != nil {
				return nil, output.Internal("SKILL_CHANNEL_APPLY_FAILED", "could not remove a stale channel file", err).
					WithDetails(map[string]any{"path": name})
			}
			result.Removed = append(result.Removed, name)
			continue
		}
		result.Conflicts = append(result.Conflicts, Conflict{Path: name,
			Reason: "file came from an earlier delivery but has local changes"})
	}
	return result, nil
}

// resolveManagedPath walks every component of name from root. Each existing
// directory component must be a real directory, and an existing final component
// must be a regular file; missing trailing components are safe to create. It
// reports whether the path is plain and, when the file exists, its content.
func resolveManagedPath(root, name string) (bool, []byte, error) {
	components := strings.Split(name, "/")
	current := root
	for index, component := range components {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if index == len(components)-1 {
				return true, nil, nil
			}
			// A missing intermediate directory: everything below it does not
			// exist either, so the whole remaining chain is safe to create.
			return true, nil, nil
		}
		if err != nil {
			return false, nil, output.Internal("SKILL_CHANNEL_APPLY_FAILED", "could not inspect the channel path", err).
				WithDetails(map[string]any{"path": name})
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return false, nil, nil
		}
		if index < len(components)-1 && !info.IsDir() {
			return false, nil, nil
		}
		if index == len(components)-1 && !info.Mode().IsRegular() {
			return false, nil, nil
		}
	}
	data, err := os.ReadFile(current)
	if err != nil {
		return false, nil, output.Internal("SKILL_CHANNEL_APPLY_FAILED", "could not read the channel file", err).
			WithDetails(map[string]any{"path": name})
	}
	return true, data, nil
}

func containsDigest(set map[string]struct{}, digest string) bool {
	_, ok := set[digest]
	return ok
}

func writeFile(target string, file File) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return output.Internal("SKILL_CHANNEL_APPLY_FAILED", "could not create the channel directory", err).
			WithDetails(map[string]any{"path": target})
	}
	mode := os.FileMode(0o644)
	if file.Mode&0o111 != 0 {
		mode = 0o755
	}
	if err := os.WriteFile(target, file.Data, mode); err != nil {
		return output.Internal("SKILL_CHANNEL_APPLY_FAILED", "could not write the channel file", err).
			WithDetails(map[string]any{"path": target})
	}
	return nil
}
