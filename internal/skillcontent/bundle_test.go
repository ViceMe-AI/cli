package skillcontent

import (
	"strings"
	"testing"
)

func TestFrontmatterAcceptsPublishedMetadata(t *testing.T) {
	for _, block := range []string{
		"name: latex-geometry\ntitle: latex-geometry\ndescription: LaTeX 几何绘图 Skill",
		"name: latex-geometry\nmetadata:\n  author: yee33\nallowed-tools: [Read, Bash]",
		"name: latex-geometry\ndescription: ''\nlicense: MIT",
	} {
		for _, ending := range []string{"\n---\n\n# Body\n", "\n---"} {
			for _, newline := range []string{"\n", "\r\n"} {
				content := strings.ReplaceAll("---\n"+block+ending, "\n", newline)
				meta, err := parseFrontmatter([]byte(content))
				if err != nil || meta.Name != "latex-geometry" {
					t.Fatalf("published frontmatter rejected: %q: %#v, %v", content, meta, err)
				}
			}
		}
	}
}

func TestFrontmatterStillRejectsInvalidCoreMetadata(t *testing.T) {
	for _, content := range []string{
		"# No frontmatter",
		"---\nname: demo\n",
		"---\nname: [broken\n---\n",
		"---\nname: demo\nname: duplicate\n---\n",
		"---\ntitle: demo\n---\n",
		"---\nname: ''\n---\n",
		"---\nname: 42\n---\n",
		"---\nname: demo\ndescription: null\n---\n",
		"---\nname: demo\ndescription: [not, text]\n---\n",
	} {
		if _, err := parseFrontmatter([]byte(content)); err == nil {
			t.Fatalf("invalid frontmatter was accepted: %q", content)
		}
	}
}
