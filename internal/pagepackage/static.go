package pagepackage

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
)

var (
	externalResourcePattern = regexp.MustCompile(`(?i)(?:<(?:script|img|source|video|audio|link)\b[^>]*(?:src|href)\s*=\s*["']?\s*(?:https?:)?//|(?:url|@import)\s*\(?\s*["']?\s*(?:https?:)?//)`)
	replicaEntryPattern     = regexp.MustCompile(`VICEME-REPLICA:VMR-[A-Z0-9]{20}|["']buyerEntry["']\s*:`)
)

// BuildWebsiteWorkPage packages an already-generated static directory. It is
// deliberately read-only: it never installs dependencies, runs scripts, or
// fetches remote assets.
func BuildWebsiteWorkPage(projectPath, name string) (Package, bool, error) {
	info, err := os.Lstat(projectPath)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return Package{}, false, nil
	}
	var outputDirectory string
	for _, candidate := range []string{"dist", "build", "out"} {
		directory := filepath.Join(projectPath, candidate)
		entry, statErr := os.Lstat(filepath.Join(directory, "index.html"))
		if statErr == nil && entry.Mode().IsRegular() && entry.Mode()&os.ModeSymlink == 0 {
			outputDirectory = directory
			break
		}
	}
	if outputDirectory == "" {
		return Package{}, false, nil
	}
	data, err := archiveStaticDirectory(outputDirectory, name)
	if err != nil {
		return Package{}, true, err
	}
	pkg, err := inspectBytes(outputDirectory, "page.zip", data)
	return pkg, true, err
}

func archiveStaticDirectory(directory, name string) ([]byte, error) {
	files := make([]string, 0)
	var total int64
	err := filepath.WalkDir(directory, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filename == directory {
			return nil
		}
		// Project recovery data is never a deployable static asset, even if a
		// user's build copied the entire project into its output directory.
		if strings.EqualFold(entry.Name(), ".viceme") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return output.Validation("PAGE_PACKAGE_SPECIAL_FILE", "static output contains a symlink or special file")
		}
		if info.IsDir() {
			return nil
		}
		if info.Size() > MaxFileBytes {
			return output.Validation("PAGE_PACKAGE_FILE_TOO_LARGE", "static output contains a file larger than 100 MiB")
		}
		total += info.Size()
		if total > MaxUncompressedBytes || len(files) >= MaxFiles-1 {
			return output.Validation("PAGE_PACKAGE_TOO_LARGE", "static output exceeds the page package budget")
		}
		files = append(files, filename)
		return nil
	})
	if err != nil {
		var cliErr *output.Error
		if errors.As(err, &cliErr) {
			return nil, err
		}
		return nil, output.Validation("PAGE_STATIC_OUTPUT_INVALID", "could not inspect the existing static output").WithCause(err)
	}
	sort.Strings(files)

	manifest := api.PageCustomizationManifest{
		APIVersion: "page.viceme.ai/v1alpha1",
		Kind:       "WorkPage",
		Metadata:   api.PageCustomizationManifestMetadata{Name: strings.TrimSpace(name)},
		Spec: api.PageCustomizationManifestSpec{
			Entry: "dist/index.html", SDKVersion: "1", Capabilities: []string{"context.read"},
		},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return nil, output.Internal("PAGE_PACKAGE_BUILD_FAILED", "could not encode the page manifest", err)
	}

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	if err := writeStaticZIPEntry(writer, "viceme-page.json", manifestBytes); err != nil {
		_ = writer.Close()
		return nil, err
	}
	for _, filename := range files {
		content, err := os.ReadFile(filename)
		if err != nil {
			_ = writer.Close()
			return nil, output.Validation("PAGE_STATIC_OUTPUT_INVALID", "could not read the existing static output").WithCause(err)
		}
		if externalResourcePattern.Match(content) {
			_ = writer.Close()
			return nil, output.Policy(
				"PAGE_EXTERNAL_RESOURCE_UNVERIFIED",
				"static output references an external resource whose source, license, and size were not proven",
			).WithHint("localize only resources with proven source, license, and size, or rerun with --replica-only")
		}
		if replicaEntryPattern.Match(content) {
			_ = writer.Close()
			return nil, output.Policy(
				"PAGE_REPLICA_ENTRY_FORBIDDEN",
				"static output must not contain a real Website Replica instruction or buyerEntry",
			)
		}
		relative, err := filepath.Rel(directory, filename)
		if err != nil {
			_ = writer.Close()
			return nil, output.Validation("PAGE_STATIC_OUTPUT_INVALID", "could not normalize the existing static output").WithCause(err)
		}
		entryName := filepath.ToSlash(filepath.Join("dist", relative))
		if err := writeStaticZIPEntry(writer, entryName, content); err != nil {
			_ = writer.Close()
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, output.Internal("PAGE_PACKAGE_BUILD_FAILED", "could not finalize the page package", err)
	}
	return buffer.Bytes(), nil
}

func writeStaticZIPEntry(writer *zip.Writer, name string, data []byte) error {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(0o600)
	entry, err := writer.CreateHeader(header)
	if err != nil {
		return output.Internal("PAGE_PACKAGE_BUILD_FAILED", "could not create the page package", err)
	}
	if _, err := entry.Write(data); err != nil {
		return output.Internal("PAGE_PACKAGE_BUILD_FAILED", "could not write the page package", err)
	}
	return nil
}
