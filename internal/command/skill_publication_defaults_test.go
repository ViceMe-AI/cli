package command

import (
	"strings"
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

func TestDefaultEditionHighlightTruncatesAtBoundary(t *testing.T) {
	if got := defaultEditionHighlight("短摘要"); got != "短摘要" {
		t.Errorf("short summary should stay unchanged, got %q", got)
	}
	if got := defaultEditionHighlight("   短摘要   "); got != "短摘要" {
		t.Errorf("summary should be trimmed, got %q", got)
	}

	long := strings.Repeat("生", 260)
	got := defaultEditionHighlight(long)
	if runeCount := len([]rune(got)); runeCount > 200 {
		t.Errorf("truncated highlight has %d runes, want <= 200", runeCount)
	}
	if got != strings.Repeat("生", 200) {
		t.Errorf("uninterrupted text should hard-cut at 200 runes, got %d runes", len([]rune(got)))
	}

	sentences := strings.Repeat("句。", 120) // 240 runes, sentence mark every 2nd rune
	got = defaultEditionHighlight(sentences)
	if runeCount := len([]rune(got)); runeCount > 200 {
		t.Errorf("sentence-cut highlight has %d runes, want <= 200", runeCount)
	}
	if !strings.HasSuffix(got, "。") {
		t.Errorf("sentence-cut highlight should end at a sentence mark, got %q", got)
	}
}
