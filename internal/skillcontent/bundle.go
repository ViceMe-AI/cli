package skillcontent

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/semver"
	"gopkg.in/yaml.v3"
)

type Bundle struct {
	FS fs.FS
}

type Info struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Digests struct {
	Full     string `json:"full_skill_bundle_digest"`
	Embedded string `json:"embedded_content_digest"`
}

type PackageMetadata struct {
	SchemaVersion     int    `json:"schema_version"`
	SkillVersion      string `json:"skill_version"`
	MinimumCLIVersion string `json:"minimum_cli_version"`
	CLICompatibility  string `json:"cli_compatibility"`
}

type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func New(fsys fs.FS) *Bundle { return &Bundle{FS: fsys} }

func (b *Bundle) List() ([]Info, error) {
	entries, err := fs.ReadDir(b.FS, ".")
	if err != nil {
		return nil, output.Internal("skill_list", "failed to read embedded Skills", err)
	}
	var result []Info
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		meta, err := b.metadata(entry.Name())
		if err != nil {
			return nil, err
		}
		result = append(result, Info{Name: meta.Name, Description: meta.Description})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (b *Bundle) Read(name, relative string) ([]byte, string, error) {
	if err := validName(name); err != nil {
		return nil, "", err
	}
	if relative == "" {
		relative = "SKILL.md"
	}
	clean := path.Clean(relative)
	if !fs.ValidPath(clean) || clean == "." || strings.HasPrefix(clean, "../") {
		return nil, "", output.Validation("skill_path", "Skill path must stay inside the selected Skill")
	}
	data, err := fs.ReadFile(b.FS, path.Join(name, clean))
	if err != nil {
		return nil, "", output.Validation("skill_file_not_found", fmt.Sprintf("Skill file %q was not found", clean))
	}
	return data, clean, nil
}

func (b *Bundle) Validate(name string) error {
	meta, err := b.metadata(name)
	if err != nil {
		return err
	}
	if meta.Name != name {
		return output.Validation("skill_name", fmt.Sprintf("Skill frontmatter name %q does not match directory %q", meta.Name, name))
	}
	if _, err := fs.Stat(b.FS, path.Join(name, "agents/openai.yaml")); err != nil {
		return output.Validation("skill_openai_metadata", "agents/openai.yaml is required in the install bundle")
	}
	data, err := fs.ReadFile(b.FS, path.Join(name, "agents/openai.yaml"))
	if err != nil {
		return output.Validation("skill_openai_metadata", "could not read agents/openai.yaml")
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return output.Validation("skill_openai_metadata", "agents/openai.yaml is not valid YAML")
	}
	if _, ok := doc["interface"]; !ok {
		return output.Validation("skill_openai_metadata", "agents/openai.yaml must define interface metadata")
	}
	if _, err := b.Package(name); err != nil {
		return err
	}
	return nil
}

func (b *Bundle) Package(name string) (PackageMetadata, error) {
	if err := validName(name); err != nil {
		return PackageMetadata{}, err
	}
	data, err := fs.ReadFile(b.FS, path.Join(name, "skill-package.json"))
	if err != nil {
		return PackageMetadata{}, output.Validation("skill_package_metadata", "skill-package.json is required in the install bundle")
	}
	var metadata PackageMetadata
	if err := decodeStrictJSON(data, &metadata); err != nil {
		return PackageMetadata{}, output.Validation("skill_package_metadata", "skill-package.json is not valid JSON")
	}
	if metadata.SchemaVersion != 1 {
		return PackageMetadata{}, output.Validation("skill_package_metadata", "skill-package.json schema_version must be 1")
	}
	if _, err := semver.Parse(metadata.SkillVersion); err != nil {
		return PackageMetadata{}, output.Validation("skill_package_metadata", "skill-package.json skill_version must be semantic version")
	}
	if _, err := semver.Parse(metadata.MinimumCLIVersion); err != nil {
		return PackageMetadata{}, output.Validation("skill_package_metadata", "skill-package.json minimum_cli_version must be semantic version")
	}
	compatible, err := semver.Satisfies(metadata.MinimumCLIVersion, metadata.CLICompatibility)
	if err != nil || !compatible {
		return PackageMetadata{}, output.Validation("skill_package_metadata", "skill-package.json cli_compatibility must include minimum_cli_version")
	}
	return metadata, nil
}

func decodeStrictJSON(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if len(bytes.TrimSpace(data[decoder.InputOffset():])) != 0 {
		return fmt.Errorf("unexpected trailing JSON value")
	}
	return nil
}

func (b *Bundle) Digests(name string) (Digests, error) {
	if err := b.Validate(name); err != nil {
		return Digests{}, err
	}
	full, err := digestFS(b.FS, name, func(string) bool { return true })
	if err != nil {
		return Digests{}, err
	}
	embedded, err := digestFS(b.FS, name, func(rel string) bool {
		return rel == "SKILL.md" || strings.HasPrefix(rel, "references/")
	})
	if err != nil {
		return Digests{}, err
	}
	return Digests{Full: full, Embedded: embedded}, nil
}

func (b *Bundle) metadata(name string) (frontmatter, error) {
	if err := validName(name); err != nil {
		return frontmatter{}, err
	}
	data, err := fs.ReadFile(b.FS, path.Join(name, "SKILL.md"))
	if err != nil {
		return frontmatter{}, output.Validation("skill_missing", fmt.Sprintf("%s/SKILL.md is missing", name))
	}
	meta, err := parseFrontmatter(data)
	if err != nil {
		return frontmatter{}, err
	}
	return meta, nil
}

func parseFrontmatter(data []byte) (frontmatter, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return frontmatter{}, output.Validation("skill_frontmatter", "SKILL.md must start with YAML frontmatter")
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		return frontmatter{}, output.Validation("skill_frontmatter", "SKILL.md frontmatter is not closed")
	}
	block := text[4 : 4+end]
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(block), &raw); err != nil {
		return frontmatter{}, output.Validation("skill_frontmatter", "SKILL.md frontmatter is invalid YAML")
	}
	for key := range raw {
		if key != "name" && key != "description" {
			return frontmatter{}, output.Validation("skill_frontmatter", fmt.Sprintf("unsupported SKILL.md frontmatter field %q", key))
		}
	}
	var meta frontmatter
	if err := yaml.Unmarshal([]byte(block), &meta); err != nil {
		return frontmatter{}, output.Validation("skill_frontmatter", "SKILL.md frontmatter is invalid")
	}
	if strings.TrimSpace(meta.Name) == "" || strings.TrimSpace(meta.Description) == "" {
		return frontmatter{}, output.Validation("skill_frontmatter", "SKILL.md frontmatter requires non-empty name and description")
	}
	return meta, nil
}

func validName(name string) error {
	if name == "" || !fs.ValidPath(name) || strings.Contains(name, "/") || name == "." || name == ".." {
		return output.Validation("skill_name", "Skill name must be a single safe path component")
	}
	return nil
}

func digestFS(fsys fs.FS, root string, include func(string) bool) (string, error) {
	var names []string
	err := fs.WalkDir(fsys, root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel := name
		if root != "." {
			rel = strings.TrimPrefix(name, strings.TrimSuffix(root, "/")+"/")
		}
		if include(rel) {
			names = append(names, rel)
		}
		return nil
	})
	if err != nil {
		return "", output.Internal("skill_digest", "failed to enumerate Skill content", err)
	}
	sort.Strings(names)
	hash := sha256.New()
	for _, name := range names {
		data, err := fs.ReadFile(fsys, path.Join(root, name))
		if err != nil {
			return "", output.Internal("skill_digest", "failed to read Skill content", err)
		}
		_, _ = hash.Write([]byte(name))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(data)
		_, _ = hash.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}
