package pagepackage

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ViceMe-AI/cli/internal/output"
)

func TestBuildWebsiteWorkPagePackagesOnlyAgentSelectedDirectoryAndEntry(t *testing.T) {
	for _, selected := range []string{".", "public-site", "dist", "build", "out"} {
		t.Run(selected, func(t *testing.T) {
			project := t.TempDir()
			directory := filepath.Join(project, selected)
			files := map[string]string{
				"pages/home.html":             `<link rel="stylesheet" href="../assets/site.css"><script src="../app.js"></script>`,
				"assets/site.css":             "body { color: blue; }",
				"app.js":                      "console.log('static')",
				".well-known/assetlinks.json": "[]",
				"data.json":                   `{"public":true}`,
			}
			for name, content := range files {
				writeStaticTestFile(t, directory, name, content)
			}
			writeStaticTestFile(t, project, "index.html", "<h1>Another entry</h1>")
			writeStaticTestFile(t, project, "package.json", `{"scripts":{"build":"exit 99"}}`)
			pkg, err := BuildWebsiteWorkPage(directory, "pages/home.html", "Replica page")
			if err != nil {
				t.Fatal(err)
			}
			if pkg.Manifest.Kind != "WorkPage" || pkg.Manifest.Spec.Entry != "dist/pages/home.html" || pkg.Artifact.FileName != "page.zip" || pkg.Artifact.Digest == "" {
				t.Fatalf("unexpected hosted page package: %#v", pkg)
			}
			entries := staticTestArchive(t, pkg.Bytes)
			for name, content := range files {
				if entries["dist/"+name] != content {
					t.Fatalf("page file was moved or changed: %s", name)
				}
			}
			if _, leaked := entries["dist/package.json"]; leaked {
				t.Fatal("project metadata leaked into hosted page")
			}
			if selected != "." {
				if _, leaked := entries["dist/index.html"]; leaked {
					t.Fatal("unselected project files leaked into hosted page")
				}
			}
			if _, err := os.Stat(filepath.Join(directory, "viceme-page.json")); !os.IsNotExist(err) {
				t.Fatal("packaging modified the selected directory")
			}
		})
	}
}

func TestBuildWebsiteWorkPageDoesNotDiscoverAnEntryOrOutput(t *testing.T) {
	project := t.TempDir()
	writeStaticTestFile(t, project, "dist/index.html", "<h1>Do not select me</h1>")
	_, err := BuildWebsiteWorkPage(project, "index.html", "Site")
	if err == nil || output.AsError(err).Subtype != "PAGE_ENTRY_MISSING" {
		t.Fatalf("missing selected entry triggered output discovery: %v", err)
	}
}

func TestBuildWebsiteWorkPageExcludesToolingAndEnvironmentFiles(t *testing.T) {
	project := t.TempDir()
	writeStaticTestFile(t, project, "index.html", "<h1>Public</h1>")
	for _, name := range []string{
		".git", ".env", ".env.local", ".DS_Store", "._index.html", "_source.json", "package.json", "pnpm-lock.yaml", "VICEME-REPLICA.md",
		"nested/.viceme/publications/pending.json", ".workbuddy/state.json", ".codex/state.json", ".agents/skills/private.md",
		".vscode/settings.json", ".cache/private.json", "node_modules/dependency/index.js", "vendor/dependency.js",
	} {
		writeStaticTestFile(t, project, name, `<script src="https://private.invalid/state.js"></script>`)
	}
	pkg, err := BuildWebsiteWorkPage(project, "index.html", "Site")
	if err != nil {
		t.Fatal(err)
	}
	entries := staticTestArchive(t, pkg.Bytes)
	if len(entries) != 2 || entries["dist/index.html"] == "" || entries["viceme-page.json"] == "" {
		t.Fatalf("non-page files leaked into archive: %v", entries)
	}
}

func TestBuildWebsiteWorkPageRetainsResourceAndReplicaChecks(t *testing.T) {
	for name, document := range map[string]string{
		"quoted":            `<script src="https://cdn.example/app.js"></script>`,
		"unquoted":          `<script src=https://cdn.example/app.js></script>`,
		"protocol-relative": `<img src=//cdn.example/image.png>`,
		"replica":           `VICEME-REPLICA:VMR-ABCDEFGHIJKLMNOPQRST`,
	} {
		t.Run(name, func(t *testing.T) {
			project := t.TempDir()
			writeStaticTestFile(t, project, "home.html", document)
			_, err := BuildWebsiteWorkPage(project, "home.html", "Site")
			code := "PAGE_EXTERNAL_RESOURCE_UNVERIFIED"
			if name == "replica" {
				code = "PAGE_REPLICA_ENTRY_FORBIDDEN"
			}
			if err == nil || output.AsError(err).Subtype != code {
				t.Fatalf("unsafe content was not rejected: %v", err)
			}
		})
	}
}

func TestBuildWebsiteWorkPageRejectsUnsafeEntryPaths(t *testing.T) {
	for _, entry := range []string{"", "../index.html", "/index.html", "C:/index.html", `pages\index.html`, "pages/../index.html", "app.js"} {
		t.Run(entry, func(t *testing.T) {
			_, err := BuildWebsiteWorkPage(t.TempDir(), entry, "Site")
			if err == nil || output.AsError(err).Subtype != "PAGE_ENTRY_INVALID" {
				t.Fatalf("unsafe entry accepted: %v", err)
			}
		})
	}
}

func TestBuildWebsiteWorkPageRejectsSymlinks(t *testing.T) {
	for _, linked := range []string{"directory", "entry", "asset"} {
		t.Run(linked, func(t *testing.T) {
			project := t.TempDir()
			directory := filepath.Join(project, "page")
			writeStaticTestFile(t, directory, "home.html", "<h1>Public</h1>")
			target, link := filepath.Join(directory, "home.html"), filepath.Join(directory, "asset.js")
			entry := "home.html"
			if linked == "directory" {
				target, link = directory, filepath.Join(project, "linked-page")
				directory = link
			} else if linked == "entry" {
				entry = "linked.html"
				link = filepath.Join(directory, entry)
			}
			if err := os.Symlink(target, link); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			if _, err := BuildWebsiteWorkPage(directory, entry, "Site"); err == nil {
				t.Fatal("symlink accepted")
			}
		})
	}
}

func writeStaticTestFile(t *testing.T, directory, name, content string) {
	t.Helper()
	filename := filepath.Join(directory, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func staticTestArchive(t *testing.T, data []byte) map[string]string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]string{}
	for _, entry := range reader.File {
		opened, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(opened)
		_ = opened.Close()
		if err != nil {
			t.Fatal(err)
		}
		entries[entry.Name] = string(content)
	}
	return entries
}
