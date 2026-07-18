package archive

import (
	"archive/tar"
	"compress/gzip"
	"context"
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

	"github.com/ViceMe-AI/cli/internal/output"
)

const DefaultMaxBytes int64 = 100 << 20

var epoch = time.Unix(0, 0).UTC()

type Artifact struct {
	Path         string
	Filename     string
	ContentType  string
	Size         int64
	SHA256Digest string
	remove       bool
}

func (a Artifact) Open() (*os.File, error) {
	return os.Open(a.Path)
}

func (a Artifact) Cleanup() error {
	if !a.remove || a.Path == "" {
		return nil
	}
	return os.Remove(a.Path)
}

type entry struct {
	absPath string
	relPath string
	info    fs.FileInfo
}

func FromFile(path string, maxBytes int64) (Artifact, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	file, err := os.Open(path)
	if err != nil {
		return Artifact{}, output.Validation("file_open", fmt.Sprintf("cannot open %q: %v", path, err))
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Artifact{}, output.Validation("file_stat", fmt.Sprintf("cannot inspect %q: %v", path, err))
	}
	if !info.Mode().IsRegular() {
		return Artifact{}, output.Validation("file_type", "--file must reference a regular archive file")
	}
	if info.Size() > maxBytes {
		return Artifact{}, output.Policy("bundle_too_large", fmt.Sprintf("Skill archive exceeds the %d byte limit", maxBytes))
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return Artifact{}, output.Internal("file_hash", "failed to hash the Skill archive", err)
	}
	return Artifact{
		Path:         path,
		Filename:     filepath.Base(path),
		ContentType:  "application/octet-stream",
		Size:         info.Size(),
		SHA256Digest: "sha256:" + hex.EncodeToString(hash.Sum(nil)),
	}, nil
}

func BuildDirectory(ctx context.Context, root string, maxBytes int64) (Artifact, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Artifact{}, output.Validation("directory_path", "could not resolve the Skill directory")
	}
	rootInfo, err := os.Stat(absRoot)
	if err != nil {
		return Artifact{}, output.Validation("directory_open", fmt.Sprintf("cannot open %q: %v", root, err))
	}
	if !rootInfo.IsDir() {
		return Artifact{}, output.Validation("directory_type", "--dir must reference a directory")
	}
	entries, rawSize, err := collect(ctx, absRoot)
	if err != nil {
		return Artifact{}, err
	}
	if rawSize > maxBytes {
		return Artifact{}, output.Policy("bundle_too_large", fmt.Sprintf("Skill directory exceeds the %d byte limit", maxBytes))
	}
	temp, err := os.CreateTemp("", "viceme-skill-*.tar.gz")
	if err != nil {
		return Artifact{}, output.Internal("archive_temp", "failed to create a temporary archive", err)
	}
	tempPath := temp.Name()
	remove := true
	defer func() {
		_ = temp.Close()
		if remove {
			_ = os.Remove(tempPath)
		}
	}()

	hash := sha256.New()
	gzipWriter, err := gzip.NewWriterLevel(io.MultiWriter(temp, hash), gzip.BestCompression)
	if err != nil {
		return Artifact{}, output.Internal("archive_gzip", "failed to initialize the archive", err)
	}
	gzipWriter.Header.ModTime = epoch
	gzipWriter.Header.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)
	for _, item := range entries {
		if err := ctx.Err(); err != nil {
			return Artifact{}, err
		}
		if err := writeEntry(tarWriter, item); err != nil {
			return Artifact{}, err
		}
	}
	if err := tarWriter.Close(); err != nil {
		return Artifact{}, output.Internal("archive_tar_close", "failed to finalize the Skill archive", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return Artifact{}, output.Internal("archive_gzip_close", "failed to finalize the compressed Skill archive", err)
	}
	if err := temp.Close(); err != nil {
		return Artifact{}, output.Internal("archive_file_close", "failed to close the Skill archive", err)
	}
	info, err := os.Stat(tempPath)
	if err != nil {
		return Artifact{}, output.Internal("archive_stat", "failed to inspect the Skill archive", err)
	}
	if info.Size() > maxBytes {
		return Artifact{}, output.Policy("bundle_too_large", fmt.Sprintf("compressed Skill directory exceeds the %d byte limit", maxBytes))
	}
	remove = false
	return Artifact{
		Path:         tempPath,
		Filename:     filepath.Base(absRoot) + ".tar.gz",
		ContentType:  "application/gzip",
		Size:         info.Size(),
		SHA256Digest: "sha256:" + hex.EncodeToString(hash.Sum(nil)),
		remove:       true,
	}, nil
}

func collect(ctx context.Context, root string) ([]entry, int64, error) {
	var entries []entry
	var total int64
	err := filepath.WalkDir(root, func(path string, dirEntry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if ignored(rel, dirEntry.IsDir()) {
			if dirEntry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := dirEntry.Info()
		if err != nil {
			return err
		}
		item := entry{absPath: path, relPath: filepath.ToSlash(rel), info: info}
		switch {
		case info.Mode().IsRegular():
			total += info.Size()
		case info.IsDir():
		case info.Mode()&os.ModeSymlink != 0:
			return output.Policy("symlink_unsupported", fmt.Sprintf("symbolic links are not supported in Skill directories: %s", rel))
		default:
			return output.Policy("unsupported_file_type", fmt.Sprintf("unsupported file type in Skill directory: %s", rel))
		}
		entries = append(entries, item)
		return nil
	})
	if err != nil {
		var cliErr *output.Error
		if errors.As(err, &cliErr) {
			return nil, 0, cliErr
		}
		return nil, 0, output.Validation("directory_walk", fmt.Sprintf("failed to read the Skill directory: %v", err))
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].relPath < entries[j].relPath })
	return entries, total, nil
}

func ignored(rel string, isDir bool) bool {
	base := filepath.Base(rel)
	if !isDir {
		return base == ".DS_Store"
	}
	switch base {
	case ".git", "node_modules", ".cache", ".next", "dist", "build", "__pycache__":
		return true
	default:
		return false
	}
}

func writeEntry(writer *tar.Writer, item entry) error {
	header := &tar.Header{
		Name:       item.relPath,
		Mode:       normalizedMode(item.info),
		ModTime:    epoch,
		AccessTime: epoch,
		ChangeTime: epoch,
		Uid:        0,
		Gid:        0,
		Format:     tar.FormatPAX,
	}
	switch {
	case item.info.IsDir():
		header.Typeflag = tar.TypeDir
		header.Name = strings.TrimSuffix(header.Name, "/") + "/"
	case item.info.Mode().IsRegular():
		header.Typeflag = tar.TypeReg
		header.Size = item.info.Size()
	default:
		return output.Policy("unsupported_file_type", fmt.Sprintf("unsupported file type: %s", item.relPath))
	}
	if err := writer.WriteHeader(header); err != nil {
		return output.Internal("archive_header", fmt.Sprintf("failed to archive %s", item.relPath), err)
	}
	if !item.info.Mode().IsRegular() {
		return nil
	}
	file, err := os.Open(item.absPath)
	if err != nil {
		return output.Validation("archive_read", fmt.Sprintf("failed to read %s", item.relPath))
	}
	defer file.Close()
	if _, err := io.Copy(writer, file); err != nil {
		return output.Internal("archive_write", fmt.Sprintf("failed to archive %s", item.relPath), err)
	}
	return nil
}

func normalizedMode(info fs.FileInfo) int64 {
	if info.IsDir() {
		return 0o755
	}
	if info.Mode().Perm()&0o111 != 0 {
		return 0o755
	}
	return 0o644
}
