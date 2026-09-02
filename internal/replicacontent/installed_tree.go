package replicacontent

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ViceMe-AI/cli/internal/privatepath"
)

type InstalledTreeReport struct {
	FileCount     int
	ExpandedBytes uint64
	Digest        string
}

func InspectInstalledTree(target string) (InstalledTreeReport, error) {
	if err := privatepath.RequirePrivateDirectory(target); err != nil {
		return InstalledTreeReport{}, errors.New("Website Replica installed tree root is invalid")
	}
	hash := sha256.New()
	report := InstalledTreeReport{}
	pathNodes := 0
	licenseSeen := false
	err := filepath.WalkDir(target, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(target, filename)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if relative == "." {
			return nil
		}
		pathNodes++
		if pathNodes > MaxArchiveEntries+2 || len(relative) > MaxArchivePathBytes {
			return errors.New("Website Replica installed tree exceeds its path limits")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("Website Replica installed tree contains a symlink")
		}
		if relative == ".viceme" {
			if !entry.IsDir() {
				return errors.New("Website Replica installed metadata path is invalid")
			}
			return nil
		}
		if strings.HasPrefix(relative, ".viceme/") {
			if relative != LicenseFilePath || !info.Mode().IsRegular() {
				return errors.New("Website Replica installed metadata namespace contains an unexpected entry")
			}
			licenseSeen = true
			return nil
		}
		if entry.IsDir() {
			writeTreeDigestFrame(hash, 'd', relative, 0, nil)
			return nil
		}
		if !info.Mode().IsRegular() || report.FileCount >= MaxFileCount || info.Size() < 0 || uint64(info.Size()) > MaxFileBytes ||
			report.ExpandedBytes > MaxExpandedBytes-uint64(info.Size()) {
			return errors.New("Website Replica installed tree exceeds its file limits")
		}
		file, err := os.Open(filename)
		if err != nil {
			return err
		}
		after, statErr := file.Stat()
		contentHash := sha256.New()
		written, copyErr := io.Copy(contentHash, io.LimitReader(file, int64(MaxFileBytes)+1))
		closeErr := file.Close()
		if statErr != nil || copyErr != nil || closeErr != nil || !after.Mode().IsRegular() || !os.SameFile(info, after) || written != info.Size() {
			return errors.New("Website Replica installed file changed while it was inspected")
		}
		executable := uint32(0)
		if info.Mode().Perm()&0o111 != 0 {
			executable = 1
		}
		writeTreeDigestFrame(hash, 'f', relative, executable, contentHash.Sum(nil))
		report.FileCount++
		report.ExpandedBytes += uint64(info.Size())
		return nil
	})
	if err != nil {
		return InstalledTreeReport{}, fmt.Errorf("inspect Website Replica installed tree: %w", err)
	}
	if !licenseSeen || report.FileCount == 0 {
		return InstalledTreeReport{}, errors.New("Website Replica installed tree is incomplete")
	}
	report.Digest = hex.EncodeToString(hash.Sum(nil))
	return report, nil
}

func writeTreeDigestFrame(writer io.Writer, kind byte, name string, mode uint32, digest []byte) {
	frame := make([]byte, 1+4+len(name)+4+len(digest))
	frame[0] = kind
	binary.BigEndian.PutUint32(frame[1:], uint32(len(name)))
	copy(frame[5:], name)
	offset := 5 + len(name)
	binary.BigEndian.PutUint32(frame[offset:], mode)
	copy(frame[offset+4:], digest)
	_, _ = writer.Write(frame)
}
