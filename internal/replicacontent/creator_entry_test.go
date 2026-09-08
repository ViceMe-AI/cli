package replicacontent

import (
	"archive/zip"
	"bytes"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFreezeSourceArchiveRemovesCreatorEntryAndPreservesOriginal(t *testing.T) {
	root := t.TempDir()
	original := "<main>创作者的作品</main>\n<!-- VICEME_CREATOR_ENTRY_BEGIN -->\n<button>做同款</button><script>const invitation = '原作者的口令';</script>\n<!-- VICEME_CREATOR_ENTRY_END -->\n<footer>作者署名</footer>\n"
	writeSourceFile(t, root, "index.html", original, 0o644)
	archive, err := FreezeSourceArchive(root, FreezeSourceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Cleanup()
	contents, _ := readFrozenZIP(t, archive)
	if got := string(contents["index.html"]); got != "<main>创作者的作品</main>\n<footer>作者署名</footer>\n" {
		t.Fatalf("交付源码仍携带原作者入口或损坏作品: %q", got)
	}
	if !reflect.DeepEqual(archive.Summary.ExcludedPaths, []SourceArchiveExclusion{{Path: "index.html", Reason: "creator-entry-blocks"}}) {
		t.Fatalf("终审缺少入口移除回执: %+v", archive.Summary.ExcludedPaths)
	}
	current, err := os.ReadFile(filepath.Join(root, "index.html"))
	if err != nil || string(current) != original {
		t.Fatalf("冻结不应修改创作者原站: %v", err)
	}
}

func TestFreezeSourceArchiveRejectsAmbiguousCreatorEntryBoundaries(t *testing.T) {
	for _, entry := range []string{
		"<!-- VICEME_CREATOR_ENTRY_BEGIN -->\n<button>做同款</button>",
		"<!-- VICEME_CREATOR_ENTRY_BEGIN -->\n<!-- VICEME_CREATOR_ENTRY_BEGIN -->\n<!-- VICEME_CREATOR_ENTRY_END -->\n",
		"/* VICEME_CREATOR_ENTRY_BEGIN */\n<button/>\n<!-- VICEME_CREATOR_ENTRY_END -->\n",
		"<div><!-- VICEME_CREATOR_ENTRY_BEGIN --><button>做同款</button><!-- VICEME_CREATOR_ENTRY_END --></div>",
	} {
		root := t.TempDir()
		writeSourceFile(t, root, "index.html", entry, 0o644)
		archive, err := FreezeSourceArchive(root, FreezeSourceOptions{})
		if archive != nil {
			archive.Cleanup()
		}
		if !errors.Is(err, ErrCreatorEntryBoundary) {
			t.Fatalf("应拒绝不明确的移除边界，实际: %v", err)
		}
	}
}

func TestFreezeSourceArchiveCreatorEntryCannotHideSecrets(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, root, "index.html", "<!-- VICEME_CREATOR_ENTRY_BEGIN -->\nconst apiKey = \"sk-proj-abcdefghijklmnopqrstuvwxyz\";\n<!-- VICEME_CREATOR_ENTRY_END -->\n", 0o644)
	archive, err := FreezeSourceArchive(root, FreezeSourceOptions{})
	if archive != nil {
		archive.Cleanup()
	}
	if !errors.Is(err, ErrSensitiveContent) {
		t.Fatalf("入口移除不可绕过敏感内容校验: %v", err)
	}
}

func TestFreezeSourceArchiveRemovesCreatorEntryFromExistingZIP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "website.zip")
	writeArchive(t, path, []archiveEntry{
		{name: "index.html", content: []byte("<main>网站</main>\n<!-- VICEME_CREATOR_ENTRY_BEGIN -->\n<button>原作者做同款</button>\n<!-- VICEME_CREATOR_ENTRY_END -->\n")},
		{name: ProjectHandoffFile, content: []byte(testProjectHandoff("- None detected.", ""))},
	})
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := FreezeSourceArchive(path, FreezeSourceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Cleanup()
	contents, _ := readFrozenZIP(t, archive)
	if string(contents["index.html"]) != "<main>网站</main>\n" {
		t.Fatalf("ZIP 交付残留原作者入口: %q", contents["index.html"])
	}
	current, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(original, current) {
		t.Fatal("原始 ZIP 被改写")
	}
	second, err := FreezeSourceArchive(path, FreezeSourceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Cleanup()
	if archive.Summary.Digest != second.Summary.Digest || !reflect.DeepEqual(archive.Summary.ExcludedPaths, second.Summary.ExcludedPaths) {
		t.Fatal("同一源码包的移除结果必须可重复")
	}
}

func TestFreezeSourceArchiveRemovesFrameworkEntryWithoutBreakingSharedCode(t *testing.T) {
	root := t.TempDir()
	files := map[string][2]string{
		"src/app.tsx": {
			"import { Tip } from './tip';\r\n/* VICEME_CREATOR_ENTRY_BEGIN */\r\nimport { CreatorEntry } from './creator-entry';\r\n/* VICEME_CREATOR_ENTRY_END */\r\nexport const App = () => <main>\r\n  <Tip />\r\n  {/* VICEME_CREATOR_ENTRY_BEGIN */}\r\n  <CreatorEntry />\r\n  {/* VICEME_CREATOR_ENTRY_END */}\r\n</main>;\r\n",
			"import { Tip } from './tip';\r\nexport const App = () => <main>\r\n  <Tip />\r\n</main>;\r\n",
		},
		"src/creator-entry.tsx": {
			"/* VICEME_CREATOR_ENTRY_BEGIN */\nexport const CreatorEntry = () => <button>做同款</button>;\n/* VICEME_CREATOR_ENTRY_END */\n",
			"",
		},
		"src/app.css": {
			"main { color: black; }\n/* VICEME_CREATOR_ENTRY_BEGIN */\n.creator-entry { color: blue; }\n/* VICEME_CREATOR_ENTRY_END */",
			"main { color: black; }\n",
		},
		"LICENSE": {"Copyright the original creator\n", "Copyright the original creator\n"},
	}
	for name, pair := range files {
		writeSourceFile(t, root, name, pair[0], 0o644)
	}
	archive, err := FreezeSourceArchive(root, FreezeSourceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Cleanup()
	contents, _ := readFrozenZIP(t, archive)
	for name, pair := range files {
		if string(contents[name]) != pair[1] {
			t.Fatalf("交付文件 %s 与预期不符: %q", name, contents[name])
		}
	}
}

func TestFreezeSourceArchiveKeepsExistingZIPCompressedAfterEntryRemoval(t *testing.T) {
	block := make([]byte, 4096)
	if _, err := rand.New(rand.NewSource(42)).Read(block); err != nil {
		t.Fatal(err)
	}
	asset := bytes.Repeat(block, 16)
	path := filepath.Join(t.TempDir(), "compressed.zip")
	writeArchive(t, path, []archiveEntry{
		{name: "index.html", method: zip.Deflate, content: []byte("<main>网站</main>\n<!-- VICEME_CREATOR_ENTRY_BEGIN -->\n<button>做同款</button>\n<!-- VICEME_CREATOR_ENTRY_END -->\n")},
		{name: "assets/data.bin", method: zip.Deflate, content: asset},
		{name: ProjectHandoffFile, method: zip.Deflate, content: []byte(testProjectHandoff("- None detected.", ""))},
	})
	original, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := FreezeSourceArchive(path, FreezeSourceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Cleanup()
	if archive.Summary.SizeBytes > original.Size()*2 {
		t.Fatalf("移除入口不应把压缩资源展开进交付包: 原包 %d，交付包 %d", original.Size(), archive.Summary.SizeBytes)
	}
	contents, _ := readFrozenZIP(t, archive)
	if !bytes.Equal(contents["assets/data.bin"], asset) || string(contents["index.html"]) != "<main>网站</main>\n" {
		t.Fatal("压缩源码包的业务内容未保持完整")
	}
}
