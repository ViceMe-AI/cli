package cli

import (
	"io/fs"
	"strings"
	"testing"
)

func TestTipSkillRequiresProfileWebBaseURLAndForbidsProductionFallback(t *testing.T) {
	content, err := fs.ReadFile(EmbeddedSkills(), "viceme-tip/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, required := range []string{
		"webBaseUrl",
		"must not guess",
		"must not fall back to a production origin",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("viceme-tip Skill is missing %q", required)
		}
	}
}
