package publication

import (
	"bytes"
	"encoding/json"
	"io"
	"path"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

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
	if decoder.Decode(&manifest) != nil || decoder.Decode(new(any)) != io.EOF || manifest.Version != 1 || strings.TrimSpace(manifest.Purpose) == "" || utf16Length(manifest.Purpose) > 2000 || len(manifest.PrivateFiles) == 0 || len(manifest.PrivateFiles) > 40 || len(manifest.PublicFiles) > 1000 || manifest.PublicFiles == nil {
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
			normalized := strings.ToLower(norm.NFC.String(name))
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
				if (extension != ".md" && extension != ".txt") || !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 || utf16Length(strings.TrimSpace(string(data))) > 16000 {
					return invalid()
				}
				if name == "SKILL.md" {
					privateSkill = true
				}
			}
		}
	}
	workflow, exists := files[manifest.LocalWorkflow]
	publicWorkflow := false
	for _, name := range manifest.PublicFiles {
		publicWorkflow = publicWorkflow || name == manifest.LocalWorkflow
	}
	total := 0
	for _, name := range manifest.PrivateFiles {
		if len(bytes.TrimSpace(files[name])) == 0 {
			return invalid()
		}
		for _, r := range strings.TrimSpace(string(files[name])) {
			total++
			if r > 0xffff {
				total++
			}
		}
	}
	for name := range seen {
		parent := path.Dir(name)
		for parent != "." {
			if seen[parent] {
				return invalid()
			}
			parent = path.Dir(parent)
		}
	}
	if !privateSkill || len(seen) != len(files) || !exists || !publicWorkflow || !strings.EqualFold(path.Ext(manifest.LocalWorkflow), ".md") || !utf8.Valid(workflow) || len(bytes.TrimSpace(workflow)) == 0 || bytes.IndexByte(workflow, 0) >= 0 || utf16Length(string(workflow)) > 16000 || total > 120000 {
		return invalid()
	}
	manifest.Purpose = strings.TrimSpace(manifest.Purpose)
	return &manifest, nil
}

// Zod string limits count UTF-16 units. Preserve complete Unicode characters
// when producing public metadata for that shared contract.
func utf16Length(text string) int {
	length := 0
	for _, r := range text {
		length++
		if r > 0xffff {
			length++
		}
	}
	return length
}

func truncateUTF16(text string, limit int) string {
	length := 0
	for index, r := range text {
		length++
		if r > 0xffff {
			length++
		}
		if length > limit {
			return text[:index]
		}
	}
	return text
}
