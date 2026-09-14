package pagepackage

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/output"
)

// PAGE 必须保留自定义 HTML、样式和交互，原项目及制品摘要保持一致。
func TestBuildWebsiteWorkPagePreservesCreatorEntryAndOriginal(t *testing.T) {
	project := t.TempDir()
	files := map[string]string{
		"index.html":      "<main>共享办公计算器</main>\n<!-- VICEME_CREATOR_ENTRY_BEGIN -->\n<style>#viceme-entry{display:flex;position:fixed}</style>\n<button id=\"viceme-entry\">做同款</button>\n<script>document.querySelector('#viceme-entry').onclick = () => alert('口令已复制');</script>\n<!-- VICEME_CREATOR_ENTRY_END -->\n<footer>作者署名；做同款介绍</footer>\n",
		"assets/site.js":  "calculate();\n/* VICEME_CREATOR_ENTRY_BEGIN */\nshowCreatorEntry();\n/* VICEME_CREATOR_ENTRY_END */\nmountDanmaku();\n",
		"assets/site.css": "main { color: black; }\r\n/* VICEME_CREATOR_ENTRY_BEGIN */\r\n#viceme-entry { display: flex; }\r\n/* VICEME_CREATOR_ENTRY_END */\r\n",
		"assets/tip.js":   "mountTip();",
		"LICENSE":         "Copyright original creator",
	}
	for name, content := range files {
		writeStaticTestFile(t, project, name, content)
	}
	pkg, err := BuildWebsiteWorkPage(project, "index.html", "共享办公")
	if err != nil {
		t.Fatal(err)
	}
	entries := staticTestArchive(t, pkg.Bytes)
	for name, content := range files {
		if got := entries["dist/"+name]; got != content {
			t.Errorf("托管页面丢失自定义入口或改动无关内容 %s: %q", name, got)
		}
		original, err := os.ReadFile(filepath.Join(project, name))
		if err != nil || string(original) != content {
			t.Errorf("打包修改了创作者原项目 %s: %v", name, err)
		}
	}
	if pkg.Artifact.Digest != sha256Hex(pkg.Bytes) || pkg.Artifact.SizeBytes != int64(len(pkg.Bytes)) {
		t.Fatal("终审摘要未绑定实际 PAGE")
	}
	again, err := BuildWebsiteWorkPage(project, "index.html", "共享办公")
	if err != nil || again.Artifact != pkg.Artifact {
		t.Fatalf("相同输入未生成确定性制品: %v", err)
	}
}

func TestBuildWebsiteWorkPageRejectsAmbiguousCreatorEntryBoundaries(t *testing.T) {
	for _, block := range []string{
		"<!-- VICEME_CREATOR_ENTRY_BEGIN -->\n<button>做同款</button>\n",
		"<!-- VICEME_CREATOR_ENTRY_BEGIN -->\n<!-- VICEME_CREATOR_ENTRY_BEGIN -->\n<!-- VICEME_CREATOR_ENTRY_END -->\n",
		"/* VICEME_CREATOR_ENTRY_BEGIN */\n<button>做同款</button>\n<!-- VICEME_CREATOR_ENTRY_END -->\n",
		"<div><!-- VICEME_CREATOR_ENTRY_BEGIN --><button>做同款</button><!-- VICEME_CREATOR_ENTRY_END --></div>",
	} {
		project := t.TempDir()
		writeStaticTestFile(t, project, "index.html", "<main>网站</main>\n"+block)
		if _, err := BuildWebsiteWorkPage(project, "index.html", "Site"); err == nil || output.AsError(err).Subtype != "REPLICA_CREATOR_ENTRY_BOUNDARY_INVALID" {
			t.Errorf("不明确的入口边界不应进入托管页面: %q", block)
		}
	}
}

func TestBuildWebsiteWorkPageChecksCreatorEntryResources(t *testing.T) {
	project := t.TempDir()
	writeStaticTestFile(t, project, "index.html", "<main>网站</main>\n<!-- VICEME_CREATOR_ENTRY_BEGIN -->\n<script src=\"https://unverified.example/entry.js\"></script>\n<!-- VICEME_CREATOR_ENTRY_END -->\n")
	_, err := BuildWebsiteWorkPage(project, "index.html", "Site")
	if err == nil || output.AsError(err).Subtype != "PAGE_EXTERNAL_RESOURCE_UNVERIFIED" {
		t.Fatalf("入口移除不应绕过现有资源校验: %v", err)
	}
}

func TestInspectWebsiteWorkPagePreservesRepairZIPAndGenericImports(t *testing.T) {
	originalHTML := "<main>网站</main>\n<!-- VICEME_CREATOR_ENTRY_BEGIN -->\n<button>做同款</button><script>showCreatorEntry();</script>\n<!-- VICEME_CREATOR_ENTRY_END -->\n"
	archive := writePageZIP(t, map[string]string{
		"viceme-page.json": validManifest("WorkPage"),
		"dist/index.html":  originalHTML,
		"dist/tip.js":      "mountTip();",
	})
	original, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	generic, err := Inspect(archive)
	if err != nil || !bytes.Equal(generic.Bytes, original) {
		t.Fatalf("通用页面导入不应改写: %v", err)
	}
	pkg, err := InspectWebsiteWorkPage(archive)
	if err != nil {
		t.Fatal(err)
	}
	entries := staticTestArchive(t, pkg.Bytes)
	if entries["dist/index.html"] != originalHTML || entries["dist/tip.js"] != "mountTip();" {
		t.Fatal("托管补发丢失自定义入口或损坏共享功能")
	}
	if pkg.Artifact.Digest != generic.Artifact.Digest || pkg.Artifact.Digest != sha256Hex(pkg.Bytes) {
		t.Fatal("托管补发改写了原始摘要")
	}
	again, err := InspectWebsiteWorkPage(archive)
	if err != nil || !bytes.Equal(pkg.Bytes, again.Bytes) {
		t.Fatalf("托管补发结果不确定: %v", err)
	}
	current, err := os.ReadFile(archive)
	if err != nil || !bytes.Equal(current, original) {
		t.Fatal("原始页面 ZIP 被改写")
	}
}

func TestInspectWebsiteWorkPageKeepsUnmarkedZIPBytesAndRejectsOtherPageKinds(t *testing.T) {
	for _, kind := range []string{"WorkPage", "CreatorPage"} {
		archive := writePageZIP(t, map[string]string{
			"viceme-page.json": validManifest(kind),
			"dist/index.html":  "<main>未标记的做同款介绍和作者署名</main>",
		})
		original, err := os.ReadFile(archive)
		if err != nil {
			t.Fatal(err)
		}
		pkg, err := InspectWebsiteWorkPage(archive)
		if kind == "CreatorPage" {
			if err == nil || output.AsError(err).Subtype != "REPLICA_REPAIR_PAGE_REQUIRED" {
				t.Fatalf("做同款补发接受了其他页面类型: %v", err)
			}
		} else if err != nil || !bytes.Equal(pkg.Bytes, original) {
			t.Fatalf("未标记的 ZIP 字节应保持不变: %v", err)
		}
	}
}

func TestInspectWebsiteWorkPageValidatesPreservedEntries(t *testing.T) {
	for _, invalid := range []string{"../outside.html", "dist/index.html"} {
		files := map[string]string{
			"viceme-page.json": validManifest("WorkPage"),
			"dist/index.html":  "<main>网站</main>",
		}
		files[invalid] = "<!-- VICEME_CREATOR_ENTRY_BEGIN -->\n<button>做同款</button>"
		_, err := InspectWebsiteWorkPage(writePageZIP(t, files))
		code := "REPLICA_CREATOR_ENTRY_BOUNDARY_INVALID"
		if strings.HasPrefix(invalid, "../") {
			code = "PAGE_PACKAGE_PATH_INVALID"
		}
		if err == nil || output.AsError(err).Subtype != code {
			t.Fatalf("补发跳过了 %s 校验: %v", code, err)
		}
	}
}
