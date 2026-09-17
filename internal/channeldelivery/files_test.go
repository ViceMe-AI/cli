package channeldelivery

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteZipIsDeterministicAndCarriesModes(t *testing.T) {
	t.Parallel()
	files := map[string]File{
		"SKILL.md":                     {Data: []byte("entry"), Mode: 0o644},
		".viceme/scripts/trial.py":     {Data: []byte("#!/usr/bin/env python3\n"), Mode: 0o755},
		".viceme/environment.json":     {Data: []byte("{}"), Mode: 0o644},
	}
	first, err := WriteZip(files)
	if err != nil {
		t.Fatal(err)
	}
	shuffled := map[string]File{}
	shuffled[".viceme/environment.json"] = files[".viceme/environment.json"]
	shuffled["SKILL.md"] = files["SKILL.md"]
	shuffled[".viceme/scripts/trial.py"] = files[".viceme/scripts/trial.py"]
	second, err := WriteZip(shuffled)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("channel ZIP is not deterministic: %d vs %d bytes", len(first), len(second))
	}
	reader, err := zip.NewReader(bytes.NewReader(first), int64(len(first)))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, entry := range reader.File {
		mode := entry.Mode().Perm()
		seen[entry.Name] = mode.String()
	}
	if len(seen) != 3 || seen[".viceme/scripts/trial.py"] != "-rwxr-xr-x" || seen["SKILL.md"] != "-rw-r--r--" {
		t.Fatalf("channel ZIP entries or modes are wrong: %#v", seen)
	}
}

func digestOf(data []byte) string {
	sum := sha256.Sum256(data)
	return string(hexEncode(sum[:]))
}

func hexEncode(data []byte) []byte {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, len(data)*2)
	for _, value := range data {
		out = append(out, digits[value>>4], digits[value&0xf])
	}
	return out
}

func TestApplyWritesOwnsAndPreservesConflicts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	first := map[string]File{
		"SKILL.md":                 {Data: []byte("gate v1"), Mode: 0o644},
		".viceme/runtime.json":     {Data: []byte(`{"kind":"trial"}`), Mode: 0o644},
		".viceme/obsolete.json":    {Data: []byte("stale"), Mode: 0o644},
	}
	result, err := Apply(root, first, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Written) != 3 || len(result.Conflicts) != 0 {
		t.Fatalf("fresh apply failed: %#v", result)
	}
	owned := map[string][]string{}
	firstContent := map[string][]byte{
		"SKILL.md":              []byte("gate v1"),
		".viceme/runtime.json":  []byte(`{"kind":"trial"}`),
		".viceme/obsolete.json": []byte("stale"),
	}
	for name, data := range firstContent {
		owned[name] = []string{digestOf(data)}
	}

	// Author hand-edits a generated file between deliveries.
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("hand edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	second := map[string]File{
		"SKILL.md":             {Data: []byte("gate v2"), Mode: 0o644},
		".viceme/runtime.json": {Data: []byte(`{"kind":"trial","v":2}`), Mode: 0o644},
	}
	result, err = Apply(root, second, owned)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Conflicts) != 1 || result.Conflicts[0].Path != "SKILL.md" {
		t.Fatalf("hand edit was not reported as the only conflict: %#v", result)
	}
	if preserved, _ := os.ReadFile(filepath.Join(root, "SKILL.md")); string(preserved) != "hand edit" {
		t.Fatalf("hand edit was overwritten: %q", preserved)
	}
	if len(result.Written) != 1 || result.Written[0] != ".viceme/runtime.json" {
		t.Fatalf("owned file was not refreshed: %#v", result)
	}
	if len(result.Removed) != 1 || result.Removed[0] != ".viceme/obsolete.json" {
		t.Fatalf("stale owned file was not removed: %#v", result)
	}
	if _, err := os.Stat(filepath.Join(root, ".viceme", "obsolete.json")); !os.IsNotExist(err) {
		t.Fatalf("stale owned file still exists: %v", err)
	}

	// Unknown author files are never touched, and stale files with local
	// edits are kept and reported.
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("author note"), 0o644); err != nil {
		t.Fatal(err)
	}
	staleOwned := map[string][]string{".viceme/runtime.json": {digestOf([]byte(`{"kind":"trial"}`))}}
	third := map[string]File{"SKILL.md": {Data: []byte("gate v3"), Mode: 0o644}}
	result, err = Apply(root, third, staleOwned)
	if err != nil {
		t.Fatal(err)
	}
	if note, _ := os.ReadFile(filepath.Join(root, "notes.md")); string(note) != "author note" {
		t.Fatalf("unknown author file was touched: %q", note)
	}
	var staleConflict bool
	for _, conflict := range result.Conflicts {
		if conflict.Path == ".viceme/runtime.json" {
			staleConflict = true
		}
	}
	if !staleConflict {
		t.Fatalf("edited stale file was not preserved as a conflict: %#v", result)
	}

	// Symbolic links in generated paths are refused, never written through.
	linkRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(linkRoot, ".viceme"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(linkRoot, "outside.txt")
	if err := os.WriteFile(outside, []byte("precious"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(linkRoot, ".viceme", "runtime.json")); err != nil {
		t.Fatal(err)
	}
	result, err = Apply(linkRoot, map[string]File{".viceme/runtime.json": {Data: []byte("x"), Mode: 0o644}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Conflicts) != 1 || result.Conflicts[0].Path != ".viceme/runtime.json" {
		t.Fatalf("symlink was not refused: %#v", result)
	}
	if content, _ := os.ReadFile(outside); string(content) != "precious" {
		t.Fatalf("symlink target was modified: %q", content)
	}
}

func TestRecordRoundTripAndBranchNaming(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	now := func() time.Time { return time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC) }
	store := Store{Directory: directory, EndpointOrigin: "https://api.viceme.cn", Now: now}
	record := Record{
		APIVersion: RecordAPIVersion, EndpointOrigin: "https://api.viceme.cn", Market: "global",
		PublicationID: "dddddddd-dddd-4ddd-8ddd-dddddddddddd", SkillDir: "/repo/skills/demo",
		AppliedFiles: map[string]string{"SKILL.md": digestOf([]byte("gate"))},
	}
	if _, exists, err := store.Load(record.PublicationID, record.SkillDir); err != nil || exists {
		t.Fatalf("missing record must be a clean miss: %v %v", exists, err)
	}
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	loaded, exists, err := store.Load(record.PublicationID, record.SkillDir)
	if err != nil || !exists {
		t.Fatalf("record did not round-trip: %v %v", exists, err)
	}
	if loaded.AppliedFiles["SKILL.md"] != record.AppliedFiles["SKILL.md"] || loaded.UpdatedAt == "" {
		t.Fatalf("record content drifted: %#v", loaded)
	}
	// A different skill directory or endpoint must not collide.
	if _, exists, _ := store.Load(record.PublicationID, "/repo/skills/other"); exists {
		t.Fatal("record key does not include the skill directory")
	}
	isolation := Store{Directory: directory, EndpointOrigin: "https://api.viceme.global", Now: now}
	if _, exists, _ := isolation.Load(record.PublicationID, record.SkillDir); exists {
		t.Fatal("record store does not shard by endpoint")
	}
	raw, err := os.ReadFile(store.filename(store.Key(record.PublicationID, record.SkillDir)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), record.PublicationID) {
		t.Fatal("record file does not carry the publication identity")
	}
	var stored Record
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}

	if BranchName("Deliver Demo_V2") != "viceme-skill-deliver-demo-v2" {
		t.Fatalf("branch sanitization is wrong: %s", BranchName("Deliver Demo_V2"))
	}
	if BranchName("///") != "" {
		t.Fatalf("unusable slug must return an empty branch: %s", BranchName("///"))
	}
}
