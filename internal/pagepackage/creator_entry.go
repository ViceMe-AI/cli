package pagepackage

import (
	"archive/zip"
	"bytes"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/replicacontent"
)

// InspectWebsiteWorkPage prepares a Replica hosting repair copy. Generic page
// imports continue to use Inspect and retain their original bytes.
func InspectWebsiteWorkPage(source string) (Package, error) {
	pkg, err := Inspect(source)
	if err != nil {
		return Package{}, err
	}
	if pkg.Manifest.Kind != "WorkPage" {
		return Package{}, output.Validation("REPLICA_REPAIR_PAGE_REQUIRED", "repair requires a valid static WorkPage artifact")
	}
	return stripWebsiteCreatorEntries(pkg)
}

// Both initial publication and hosting repair use the same creator-entry
// boundaries as SOURCE. Never rely on agent-authored iframe visibility checks
// to prevent a second button alongside the platform-owned entry.
func stripWebsiteCreatorEntries(pkg Package) (Package, error) {
	reader, err := zip.NewReader(bytes.NewReader(pkg.Bytes), int64(len(pkg.Bytes)))
	if err != nil {
		return Package{}, output.Validation("PAGE_PACKAGE_INVALID_ZIP", "page package is not a valid ZIP archive").WithCause(err)
	}
	var buffer bytes.Buffer
	var writer *zip.Writer
	for index, entry := range reader.File {
		var cleaned []byte
		removed := false
		if !entry.FileInfo().IsDir() && entry.Name != "viceme-page.json" {
			content, err := readEntry(entry)
			if err != nil {
				return Package{}, err
			}
			cleaned, removed, err = replicacontent.StripCreatorEntry(entry.Name, content)
			if err != nil {
				return Package{}, output.Validation("REPLICA_CREATOR_ENTRY_BOUNDARY_INVALID", "creator entry markers must be complete, non-nested standalone comment lines").WithHint("repair the marked creator entry in the selected page files before publishing")
			}
		}
		if removed && writer == nil {
			writer = zip.NewWriter(&buffer)
			// Preserve the compressed bytes of all untouched entries. A ZIP
			// without creator markers returns unchanged, including its digest.
			for _, previous := range reader.File[:index] {
				if err := writer.Copy(previous); err != nil {
					return Package{}, output.Internal("PAGE_PACKAGE_BUILD_FAILED", "could not copy the page package", err)
				}
			}
		}
		if writer == nil {
			continue
		}
		if removed {
			err = writeStaticZIPEntry(writer, entry.Name, cleaned)
		} else {
			err = writer.Copy(entry)
		}
		if err != nil {
			return Package{}, output.Internal("PAGE_PACKAGE_BUILD_FAILED", "could not prepare the hosted page copy", err)
		}
	}
	if writer == nil {
		return pkg, nil
	}
	if err := writer.Close(); err != nil {
		return Package{}, output.Internal("PAGE_PACKAGE_BUILD_FAILED", "could not finalize the page package", err)
	}
	// Review, confirmation, recovery and upload bind to the cleaned bytes.
	return inspectBytes(pkg.SourcePath, pkg.Artifact.FileName, buffer.Bytes())
}
