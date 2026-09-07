package templatecatalog

import (
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/ViceMe-AI/cli/internal/semver"
)

var (
	ErrInvalidSourceCatalog = errors.New("TEMPLATE_CATALOG_SOURCE_INVALID")
	idPattern               = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

type SourceCatalog struct {
	SchemaVersion int              `json:"schema_version"`
	Templates     []SourceTemplate `json:"templates"`
}

type SourceTemplate struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Version     string `json:"version"`
	Name        string `json:"name"`
	Scenario    string `json:"scenario"`
	Description string `json:"description"`
	SourceDir   string `json:"source_dir"`
	PreviewFile string `json:"preview_file"`
	License     string `json:"license"`
}

// DemoCatalog is used only when building a loopback catalog preview. Its
// entries are never signed into the production manifest and cannot be fetched.
type DemoCatalog struct {
	SchemaVersion int            `json:"schema_version"`
	Templates     []DemoTemplate `json:"templates"`
}

type DemoTemplate struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Scenario    string `json:"scenario"`
	Description string `json:"description"`
	PreviewFile string `json:"preview_file"`
}

func LoadSourceCatalog(reader io.Reader) (SourceCatalog, error) {
	var catalog SourceCatalog
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&catalog); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return SourceCatalog{}, ErrInvalidSourceCatalog
	}
	if err := validateSourceCatalog(catalog); err != nil {
		return SourceCatalog{}, err
	}
	return catalog, nil
}

func LoadDemoCatalog(reader io.Reader) (DemoCatalog, error) {
	var catalog DemoCatalog
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&catalog); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return DemoCatalog{}, ErrInvalidSourceCatalog
	}
	if catalog.SchemaVersion != 1 || len(catalog.Templates) == 0 {
		return DemoCatalog{}, ErrInvalidSourceCatalog
	}
	seen := make(map[string]struct{}, len(catalog.Templates))
	for _, entry := range catalog.Templates {
		if !idPattern.MatchString(entry.ID) || strings.TrimSpace(entry.Name) == "" ||
			strings.TrimSpace(entry.Scenario) == "" || strings.TrimSpace(entry.Description) == "" ||
			!safeRelativePath(entry.PreviewFile) {
			return DemoCatalog{}, ErrInvalidSourceCatalog
		}
		if _, duplicate := seen[entry.ID]; duplicate {
			return DemoCatalog{}, ErrInvalidSourceCatalog
		}
		seen[entry.ID] = struct{}{}
	}
	return catalog, nil
}

func validateSourceCatalog(catalog SourceCatalog) error {
	if catalog.SchemaVersion != 1 || len(catalog.Templates) == 0 {
		return ErrInvalidSourceCatalog
	}
	seen := make(map[string]struct{}, len(catalog.Templates))
	for _, template := range catalog.Templates {
		if !validSourceTemplate(template) {
			return ErrInvalidSourceCatalog
		}
		key := template.ID + "@" + template.Version
		if _, duplicate := seen[key]; duplicate {
			return ErrInvalidSourceCatalog
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validSourceTemplate(template SourceTemplate) bool {
	if template.Status != "production" || !idPattern.MatchString(template.ID) {
		return false
	}
	if _, err := semver.Parse(template.Version); err != nil {
		return false
	}
	if strings.TrimSpace(template.Name) == "" || strings.TrimSpace(template.Scenario) == "" ||
		strings.TrimSpace(template.Description) == "" || strings.TrimSpace(template.License) == "" {
		return false
	}
	return safeRelativePath(template.SourceDir) && safeRelativePath(template.PreviewFile)
}

func safeRelativePath(value string) bool {
	// filepath.FromSlash is used after validation. On Windows a backslash is a
	// path separator even though path.Clean treats it as an ordinary byte, so
	// reject it before a catalog field can escape the repository root.
	if value == "" || strings.Contains(value, "\\") || path.IsAbs(value) || path.Clean(value) != value || value == "." {
		return false
	}
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "" && parsed.Host == "" && parsed.User == nil && parsed.RawQuery == "" &&
		parsed.Fragment == "" && parsed.Path == value && !strings.HasPrefix(value, "../") && value != ".."
}
