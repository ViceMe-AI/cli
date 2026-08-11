package skillcontent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExplicitAgentInstallAlsoWritesAgentsFallbackAndRepairsDrift(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestSkill(t, root, "viceme-test")
	bundle := New(os.DirFS(root))
	home := t.TempDir()
	environment := Environment{Home: home, CodexHome: filepath.Join(home, ".codex"), ConfigDir: filepath.Join(home, ".viceme-cli")}

	report := bundle.Install("viceme-test", "codex", environment)
	if !report.AllSucceeded || len(report.Results) != 2 {
		t.Fatalf("unexpected install report: %#v", report)
	}
	for _, directory := range []string{
		filepath.Join(home, ".codex", "skills", "viceme-test"),
		filepath.Join(home, ".agents", "skills", "viceme-test"),
	} {
		if _, err := os.Stat(filepath.Join(directory, "SKILL.md")); err != nil {
			t.Fatalf("missing installed Skill at %s: %v", directory, err)
		}
	}

	installed := filepath.Join(home, ".codex", "skills", "viceme-test", "SKILL.md")
	if err := os.WriteFile(installed, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if bundle.Doctor("viceme-test", "codex", environment).Healthy {
		t.Fatal("doctor accepted a modified Skill")
	}
	repaired := bundle.Install("viceme-test", "codex", environment)
	if !repaired.AllSucceeded || !bundle.Doctor("viceme-test", "codex", environment).Healthy {
		t.Fatalf("drift was not repaired: %#v", repaired)
	}
}

func TestAutoInstallsDetectedAgentsAndFallback(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestSkill(t, root, "viceme-test")
	home := t.TempDir()
	for _, directory := range []string{filepath.Join(home, ".codex"), filepath.Join(home, ".claude")} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	bundle := New(os.DirFS(root))
	report := bundle.Install("viceme-test", "auto", Environment{Home: home})
	if !report.AllSucceeded || len(report.Results) != 3 {
		t.Fatalf("auto targets did not include fallback and detected agents: %#v", report)
	}
	got := map[string]bool{}
	for _, result := range report.Results {
		got[result.Target] = true
	}
	if !got["agents"] || !got["codex"] || !got["claude"] || got["workbuddy"] {
		t.Fatalf("unexpected auto target set: %#v", got)
	}
}

func writeTestSkill(t *testing.T, root, name string) {
	t.Helper()
	files := map[string]string{
		"SKILL.md": `---
name: ` + name + `
description: Test ViceMe Skill.
---

# Test
`,
		"agents/openai.yaml": `interface:
  display_name: "ViceMe Test"
  short_description: "Test installer"
  default_prompt: "Use the test Skill."
`,
		"skill-package.json": `{
  "schema_version": 1,
  "skill_version": "0.10.1",
  "minimum_cli_version": "0.10.1",
  "cli_compatibility": ">=0.10.1 <0.11.0"
}
`,
	}
	for relative, content := range files {
		filename := filepath.Join(root, name, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
