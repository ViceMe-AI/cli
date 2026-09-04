package skillcontent_test

import (
	"io/fs"
	"path"
	"strings"
	"testing"

	cliembed "github.com/ViceMe-AI/cli"
)

func TestBonjourCardKeepsTheSuppliedBlockEditorPrototype(t *testing.T) {
	t.Parallel()

	const templateRoot = "customize-your-page/templates/bonjour-card"
	for _, filename := range []string{
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
		"内容 Block",
		"添加 Block",
		"编辑个人资料",
		"补充作品信息",
		"作品封面",
		"媒体与联系方式",
		"飞书、X 和 GitHub",
		"window.viceme.context.get()",
		"window.viceme.navigation.openWork",
		"<Icon name=\"edit\"",
	} {
		if !strings.Contains(app, required) {
			t.Fatalf("prototype omitted interaction %q", required)
		}
	}

	dataBytes, err := fs.ReadFile(cliembed.EmbeddedSkills(), path.Join(templateRoot, "src/data.js"))
	if err != nil {
		t.Fatal(err)
	}
	data := string(dataBytes)
	for _, required := range []string{
		"{ type: 'work'",
		"{ type: 'contact'",
		"'飞书'",
		"'X / Twitter'",
		"'邮箱'",
		"'GitHub'",
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
		".primary-button",
		".block-edit",
		"grid-template-columns: minmax(255px, .78fr) minmax(0, 1.7fr)",
	} {
		if !strings.Contains(style, required) {
			t.Fatalf("prototype omitted supplied visual rule %q", required)
		}
	}
}
