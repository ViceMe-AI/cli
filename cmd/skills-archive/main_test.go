package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io/fs"
	"reflect"
	"testing"
	"testing/fstest"
	"time"

	cliembed "github.com/ViceMe-AI/cli"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func testSkillsFS() fs.FS {
	return fstest.MapFS{
		"demo-skill/SKILL.md":               &fstest.MapFile{Data: []byte("---\nname: demo-skill\ndescription: Demo.\n---\n\n正文。\n")},
		"demo-skill/references/workflow.md": &fstest.MapFile{Data: []byte("# 工作流\n")},
		"demo-skill/agents/openai.yaml": &fstest.MapFile{Data: []byte(
			"interface:\n  display_name: \"Demo\"\n  short_description: \"Demo skill\"\n  default_prompt: \"Run the demo.\"\n",
		)},
		"demo-skill/skill-package.json": &fstest.MapFile{Data: []byte(
			`{"schema_version":1,"skill_version":"0.1.0","minimum_cli_version":"0.1.0","cli_compatibility":">=0.1.0 <0.2.0"}`,
		)},
	}
}

func TestBuildHostingArchiveDeterministicZip(t *testing.T) {
	first, firstManifest, err := buildHostingArchive(testSkillsFS())
	if err != nil {
		t.Fatalf("first build: %v", err)
	}
	second, _, err := buildHostingArchive(testSkillsFS())
	if err != nil {
		t.Fatalf("second build: %v", err)
	}
	if len(first) != len(second) || len(first) != 2 {
		t.Fatalf("expected one zip plus the manifest, got %d files", len(first))
	}
	for name := range first {
		if !bytes.Equal(first[name], second[name]) {
			t.Fatalf("%s must be byte-identical across builds", name)
		}
	}
	if _, ok := first["manifest.json"]; !ok {
		t.Fatalf("manifest.json missing from the archive output")
	}
	if firstManifest.SchemaVersion != 1 {
		t.Fatalf("unexpected schema version %d", firstManifest.SchemaVersion)
	}
}

func TestArchiveSkillZipShape(t *testing.T) {
	skillsFS := testSkillsFS()
	archive, listing, err := archiveSkill(skillsFS, "demo-skill")
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	if len(reader.File) != len(listing) {
		t.Fatalf("zip entries %d do not match listing %d", len(reader.File), len(listing))
	}
	for index, file := range reader.File {
		if file.Name != "demo-skill/"+listing[index] {
			t.Fatalf("entry %q is not rooted at the skill directory", file.Name)
		}
		// 固定时间戳是字节确定性的关键;发布侧对已存在对象做字节比对。
		if !file.Modified.Equal(time.Unix(0, 0).UTC()) {
			t.Fatalf("entry %q must carry the fixed epoch timestamp, got %v", file.Name, file.Modified)
		}
		if file.Mode().Perm() != 0o644 {
			t.Fatalf("entry %q must carry fixed 0644 permissions", file.Name)
		}
	}
}

func TestManifestDigestsMatchBundle(t *testing.T) {
	skillsFS := testSkillsFS()
	files, manifest, err := buildHostingArchive(skillsFS)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	digests, err := skillcontent.New(skillsFS).Digests("demo-skill")
	if err != nil {
		t.Fatalf("bundle digests: %v", err)
	}
	hosted, ok := manifest.Skills["demo-skill"]
	if !ok {
		t.Fatalf("manifest missing demo-skill")
	}
	if hosted.FullBundleDigest != digests.Full || hosted.EmbeddedContentDigest != digests.Embedded {
		t.Fatalf("manifest digests drifted from the bundle source")
	}
	if hosted.SkillVersion != "0.1.0" || hosted.Zip != "demo-skill.zip" {
		t.Fatalf("unexpected hosted metadata: %+v", hosted)
	}
	if hosted.ZipSHA256 == "" {
		t.Fatalf("zip digest missing")
	}
	if len(hosted.Files) != 4 {
		t.Fatalf("expected 4 files in the listing, got %v", hosted.Files)
	}
	// manifest 必须能被严格 JSON 解析回同一形状(消费方按 schema 读)。
	var decoded hostingManifest
	if err := json.Unmarshal(files["manifest.json"], &decoded); err != nil {
		t.Fatalf("manifest is not valid JSON: %v", err)
	}
	if !reflect.DeepEqual(decoded.Skills["demo-skill"], hosted) {
		t.Fatalf("manifest round-trip drifted")
	}
}

func TestOfficialMakeCopyArchiveIncludesStandaloneRuntime(t *testing.T) {
	_, manifest, err := buildHostingArchive(cliembed.EmbeddedSkills())
	if err != nil {
		t.Fatalf("build official archive: %v", err)
	}
	hosted, ok := manifest.Skills["let-me-make-a-copy"]
	if !ok {
		t.Fatal("manifest missing let-me-make-a-copy")
	}
	want := map[string]bool{
		"scripts/make_copy.py": false,
	}
	for _, name := range hosted.Files {
		if _, required := want[name]; required {
			want[name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("hosted archive omitted %s", name)
		}
	}
}
