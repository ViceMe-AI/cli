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
	return validateWebsiteCreatorEntries(pkg)
}

// PAGE 保留创作者自定义入口；仅 SOURCE 移除入口。仍校验完整标记，
// 防止后续 SOURCE 冻结无法确定移除边界。终审与恢复绑定原始 PAGE 字节。
func validateWebsiteCreatorEntries(pkg Package) (Package, error) {
	reader, err := zip.NewReader(bytes.NewReader(pkg.Bytes), int64(len(pkg.Bytes)))
	if err != nil {
		return Package{}, output.Validation("PAGE_PACKAGE_INVALID_ZIP", "page package is not a valid ZIP archive").WithCause(err)
	}
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() || entry.Name == "viceme-page.json" {
			continue
		}
		content, err := readEntry(entry)
		if err != nil {
			return Package{}, err
		}
		if _, _, err := replicacontent.StripCreatorEntry(entry.Name, content); err != nil {
			return Package{}, output.Validation("REPLICA_CREATOR_ENTRY_BOUNDARY_INVALID", "creator entry markers must be complete, non-nested standalone comment lines").WithHint("repair the marked creator entry in the selected page files before publishing")
		}
	}
	return pkg, nil
}
