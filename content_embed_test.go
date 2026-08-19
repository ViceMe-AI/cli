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
		"creator-app create",
		"creator-app domain add",
		"creator-app domain verify",
		"creator-app show",
		"never ask the user to operate the Creator Center page",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("viceme-tip Skill is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"ask the user to click Verify",
		"guide the user to create",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("viceme-tip Skill still delegates manual work to the user: %q", forbidden)
		}
	}
}
