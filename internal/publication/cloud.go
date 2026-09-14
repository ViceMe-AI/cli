package publication

import (
	"bytes"
	"encoding/json"
	"io"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
)

// The declaration is part of the immutable source archive and publication
// manifest. Only explicitly public images may become listing media.
func cloudManifestFromEntries(entries []sourceEntry) (*api.SkillCloudPackageManifest, error) {
	var raw []byte
	files := map[string][]byte{}
	for _, entry := range entries {
		files[entry.name] = entry.data
		if entry.name == "viceme-cloud.json" {
			raw = entry.data
		}
	}
	if raw == nil {
		return nil, nil
	}
	invalid := func() (*api.SkillCloudPackageManifest, error) {
		return nil, output.Validation("SKILL_CLOUD_MANIFEST_INVALID", "viceme-cloud.json must explicitly classify every source file, keep SKILL.md private, and declare only UTF-8 .md/.txt private sources of at most 16000 characters")
	}
	var manifest api.SkillCloudPackageManifest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&manifest) != nil || decoder.Decode(new(any)) != io.EOF || manifest.Version != 1 || strings.TrimSpace(manifest.Purpose) == "" || utf8.RuneCountInString(manifest.Purpose) > 2000 || len(manifest.PrivateFiles) == 0 || len(manifest.PrivateFiles) > 40 || len(manifest.PublicFiles) > 1000 || manifest.PublicFiles == nil {
		return invalid()
	}
	seen := map[string]bool{"viceme-cloud.json": true}
	privateSkill := false
	for _, private := range []bool{true, false} {
		names := manifest.PublicFiles
		if private {
			names = manifest.PrivateFiles
		}
		for _, name := range names {
			normalized := strings.ToLower(name)
			if validatePath(name) != nil || strings.Contains(name, ":") || seen[normalized] {
				return invalid()
			}
			for _, part := range strings.Split(normalized, "/") {
				if part == ".viceme" {
					return invalid()
				}
			}
			for _, r := range name {
				if r < 32 || r == 127 {
					return invalid()
				}
			}
			data, exists := files[name]
			if !exists {
				return invalid()
			}
			seen[normalized] = true
			if private {
				extension := strings.ToLower(path.Ext(name))
				if (extension != ".md" && extension != ".txt") || !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 || utf8.RuneCount(data) > 16000 {
					return invalid()
				}
				if name == "SKILL.md" {
					privateSkill = true
				}
			}
		}
	}
	if !privateSkill || len(seen) != len(files) {
		return invalid()
	}
	manifest.Purpose = strings.TrimSpace(manifest.Purpose)
	return &manifest, nil
}
