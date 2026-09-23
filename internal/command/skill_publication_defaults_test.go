package command

import (
	"testing"
)

func TestNormalizeGithubPathTreatsRootSpellingsAsRoot(t *testing.T) {
	cases := map[string]string{
		"":            "",
		" ":           "",
		".":           "",
		"./":          "",
		" . ":         "",
		"skills/":     "skills",
		"./skills":    "./skills",
		"/skills":     "/skills",
		"skills/core": "skills/core",
	}
	for input, expected := range cases {
		if got := normalizeGithubPath(input); got != expected {
			t.Errorf("normalizeGithubPath(%q) = %q, want %q", input, got, expected)
		}
	}
}
