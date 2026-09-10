package skillcontent_test

import (
	"io/fs"
	"path"
	"strings"
	"testing"

	cliembed "github.com/ViceMe-AI/cli"
)

func TestBonjourCardIsAReadOnlyAgentDrivenPreview(t *testing.T) {
	t.Parallel()

	const templateRoot = "customize-your-profile-page/templates/bonjour-card"
	for _, filename := range []string{
		"README.md",
		"DESIGN.md",
		"PRODUCT.md",
		"index.html",
		"package-lock.json",
		"package.json",
		"public/viceme-page.json",
		"src/App.jsx",
		"src/data.js",
		"src/main.jsx",
		"src/styles.css",
	} {
		info, err := fs.Stat(cliembed.EmbeddedSkills(), path.Join(templateRoot, filename))
		if err != nil {
			t.Fatalf("missing supplied prototype file %s: %v", filename, err)
		}
		if info.IsDir() {
			t.Fatalf("prototype file %s is a directory", filename)
		}
	}

	appBytes, err := fs.ReadFile(cliembed.EmbeddedSkills(), path.Join(templateRoot, "src/App.jsx"))
	if err != nil {
		t.Fatal(err)
	}
	app := string(appBytes)
	for _, required := range []string{
		"LinkPreviewModal",
		"打开链接",
		"window.viceme.navigation.openWork",
		"function validatedLink",
		"new URL",
		"'http:'",
		"'https:'",
		"'mailto:'",
		"url.protocol === 'mailto:'",
		"href={block.href}",
	} {
		if !strings.Contains(app, required) {
			t.Fatalf("read-only preview omitted retained action %q", required)
		}
	}
	for _, forbidden := range []string{
		"添加 Block",
		"编辑个人资料",
		"补充作品信息",
		"媒体与联系方式",
		"window.viceme.context.get()",
		"localStorage",
		"<Icon name=\"edit\"",
		"FileReader",
		"<input",
		"<textarea",
		"<form",
		"contentEditable",
		"onChange",
		"保存 Block",
		"删除",
	} {
		if strings.Contains(app, forbidden) {
			t.Fatalf("read-only preview retained editor surface %q", forbidden)
		}
	}

	dataBytes, err := fs.ReadFile(cliembed.EmbeddedSkills(), path.Join(templateRoot, "src/data.js"))
	if err != nil {
		t.Fatal(err)
	}
	data := string(dataBytes)
	for _, required := range []string{
		"type: 'work'",
		"type: 'contact'",
	} {
		if !strings.Contains(data, required) {
			t.Fatalf("prototype omitted scoped content %q", required)
		}
	}
	for _, excluded := range []string{
		"{ type: 'text'",
		"{ type: 'image'",
		"{ type: 'video'",
		"{ type: 'link'",
	} {
		if strings.Contains(data, excluded) {
			t.Fatalf("prototype retained out-of-scope Block %q", excluded)
		}
	}

	styleBytes, err := fs.ReadFile(cliembed.EmbeddedSkills(), path.Join(templateRoot, "src/styles.css"))
	if err != nil {
		t.Fatal(err)
	}
	style := string(styleBytes)
	for _, required := range []string{
		".workspace",
		".profile-rail",
		".rail-rule",
		".section-label",
		"grid-template-columns: minmax(255px, .78fr) minmax(0, 1.7fr)",
	} {
		if !strings.Contains(style, required) {
			t.Fatalf("read-only preview omitted supplied visual rule %q", required)
		}
	}
	for _, forbidden := range []string{".add-control", ".block-edit", ".block-picker", ".dialog-footer", ".delete-button"} {
		if strings.Contains(style, forbidden) {
			t.Fatalf("read-only preview retained editor style %q", forbidden)
		}
	}

	manifestBytes, err := fs.ReadFile(cliembed.EmbeddedSkills(), path.Join(templateRoot, "public/viceme-page.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := string(manifestBytes)
	if !strings.Contains(manifest, `"name": "Bonjour Card"`) {
		t.Fatal("read-only preview did not identify itself as Bonjour Card")
	}
	if strings.Contains(manifest, "context.read") {
		t.Fatal("read-only preview retained unused context.read capability")
	}
	if !strings.Contains(manifest, "navigation.open") {
		t.Fatal("read-only preview omitted used navigation.open capability")
	}

	for _, filename := range []string{"README.md", "DESIGN.md", "PRODUCT.md"} {
		body, err := fs.ReadFile(cliembed.EmbeddedSkills(), path.Join(templateRoot, filename))
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		if (!strings.Contains(text, "只读") && !strings.Contains(text, "read-only")) || !strings.Contains(text, "Agent") {
			t.Fatalf("template documentation %s omitted read-only Agent-driven contract", filename)
		}
		for _, forbidden := range []string{"添加 Block", "localStorage", "context.read", "编辑与删除"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("template documentation %s retained editor contract %q", filename, forbidden)
			}
		}
	}

	design, err := fs.ReadFile(cliembed.EmbeddedSkills(), path.Join(templateRoot, "DESIGN.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(design), "“个人主页”") {
		t.Fatal("template design did not match the personal-page heading in source")
	}

	index, err := fs.ReadFile(cliembed.EmbeddedSkills(), path.Join(templateRoot, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), "<title>Bonjour Card</title>") {
		t.Fatal("read-only preview did not use the Bonjour Card page title")
	}

	for _, filename := range []string{"package.json", "package-lock.json"} {
		body, err := fs.ReadFile(cliembed.EmbeddedSkills(), path.Join(templateRoot, filename))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), `"name": "bonjour-card-template"`) || strings.Contains(string(body), "profile-blocks-template") {
			t.Fatalf("Bonjour build metadata %s retained the legacy template name", filename)
		}
	}
}
