package skillcontent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/buildinfo"
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

func TestDoctorAcceptsPOCCLIForStableSkillCompatibilityContract(t *testing.T) {
	previousVersion := buildinfo.Version
	buildinfo.Version = "0.16.1-poc.12"
	t.Cleanup(func() { buildinfo.Version = previousVersion })

	root := t.TempDir()
	writeTestSkill(t, root, "viceme-test")
	home := t.TempDir()
	environment := Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli")}
	bundle := New(os.DirFS(root))

	if report := bundle.Install("viceme-test", "agents", environment); !report.AllSucceeded {
		t.Fatalf("POC CLI could not install the stable-line Skill: %#v", report)
	}
	report := bundle.Doctor("viceme-test", "agents", environment)
	if !report.Healthy {
		t.Fatalf("Doctor rejected a POC CLI for its stable-line Skill: %#v", report)
	}
	if got := report.Results[0].Checks.Compatibility.Actual; got != "0.16.1-poc.12" {
		t.Fatalf("Doctor compatibility report lost the actual CLI version: %q", got)
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
	doctor := bundle.Doctor("viceme-test", "auto", Environment{Home: home})
	if !doctor.Healthy || len(doctor.Results) != 3 {
		t.Fatalf("Doctor required an agent that was not detected: %#v", doctor)
	}
}

func TestInstallSetActivatesAllOfficialSkillsTogether(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestSkill(t, root, "viceme-shared")
	writeTestSkill(t, root, "viceme-publish")
	writeTestSkill(t, root, "viceme-danmaku")
	home := t.TempDir()
	environment := Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli")}
	bundle := New(os.DirFS(root))

	skillNames := []string{"viceme-shared", "viceme-publish", "viceme-danmaku"}
	reports := bundle.InstallSet(skillNames, "agents", environment)
	if len(reports) != len(skillNames) {
		t.Fatalf("transaction did not report every official Skill: %#v", reports)
	}
	for index, report := range reports {
		if !report.AllSucceeded {
			t.Fatalf("transaction did not activate %s: %#v", skillNames[index], report)
		}
	}
	for _, name := range skillNames {
		if !bundle.Doctor(name, "agents", environment).Healthy {
			t.Fatalf("installed Skill %s did not pass Doctor", name)
		}
	}
}

func TestIncompleteInstallJournalRollsBackBeforeNewWork(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	destination := filepath.Join(root, "skills", "viceme-test")
	backup := destination + ".viceme-transaction-backup"
	stageRoot := filepath.Join(root, ".viceme-stage-crash")
	stage := filepath.Join(stageRoot, "viceme-test")
	for filename, content := range map[string]string{
		filepath.Join(destination, "state.txt"): "new",
		filepath.Join(backup, "state.txt"):      "old",
		filepath.Join(stage, "state.txt"):       "staged",
	} {
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	journalPath := filepath.Join(root, installTransactionFilename)
	journal := installTransaction{SchemaVersion: 1, Status: "PREPARING", Entries: []installJournalEntry{{
		Destination: destination, Backup: backup, Stage: stage, HadExisting: true, Activating: true,
	}}}
	if err := writeInstallTransaction(journalPath, journal); err != nil {
		t.Fatal(err)
	}
	if err := recoverInstallTransaction(journalPath); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(destination, "state.txt"))
	if err != nil || string(data) != "old" {
		t.Fatalf("rollback did not restore the previous Skill: %q err=%v", data, err)
	}
	if _, err := os.Stat(journalPath); !os.IsNotExist(err) {
		t.Fatalf("recovery journal was not retired: %v", err)
	}
}

func TestBootstrapRecoveryDispositionOverridesMatchingInnerGeneration(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name        string
		commit      bool
		wantContent string
	}{
		{name: "outer preparing forces rollback", commit: false, wantContent: "old"},
		{name: "outer committing forces roll forward", commit: true, wantContent: "new"},
	} {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			destination := filepath.Join(root, "skills", "viceme-test", "state.txt")
			backup := filepath.Join(root, "skills", "viceme-test.viceme-transaction-backup", "state.txt")
			for filename, content := range map[string]string{destination: "new", backup: "old"} {
				if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			journalPath := filepath.Join(root, installTransactionFilename)
			journal := installTransaction{
				SchemaVersion: 1,
				Status:        "COMMITTING",
				// The inner generation deliberately matches the running CLI. The
				// outer bootstrap commit point must still be authoritative.
				TargetCLIVersion: buildinfo.Version,
				Entries: []installJournalEntry{{
					Destination: filepath.Dir(destination),
					Backup:      filepath.Dir(backup),
					HadExisting: true,
					Activating:  true,
				}},
			}
			if err := writeInstallTransaction(journalPath, journal); err != nil {
				t.Fatal(err)
			}
			if err := RecoverInstallTransaction(Environment{Home: root, ConfigDir: root}, scenario.commit); err != nil {
				t.Fatal(err)
			}
			content, err := os.ReadFile(destination)
			if err != nil || string(content) != scenario.wantContent {
				t.Fatalf("unexpected recovered generation: content=%q err=%v", content, err)
			}
			if _, err := os.Stat(journalPath); !os.IsNotExist(err) {
				t.Fatalf("recovery journal was not retired: %v", err)
			}
		})
	}
}

func TestAutomaticRecoveryUsesTargetCLIVersionAsCommitFence(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name          string
		targetVersion string
		wantContent   string
	}{
		{name: "matching generation rolls forward", targetVersion: buildinfo.Version, wantContent: "new"},
		{name: "different generation rolls back", targetVersion: "999.0.0", wantContent: "old"},
	} {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			destinationDirectory := filepath.Join(root, "skills", "viceme-test")
			backupDirectory := destinationDirectory + ".viceme-transaction-backup"
			for filename, content := range map[string]string{
				filepath.Join(destinationDirectory, "state.txt"): "new",
				filepath.Join(backupDirectory, "state.txt"):      "old",
			} {
				if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			journal := installTransaction{
				SchemaVersion:    1,
				Status:           "COMMITTING",
				TargetCLIVersion: scenario.targetVersion,
				Entries: []installJournalEntry{{
					Destination: destinationDirectory,
					Backup:      backupDirectory,
					HadExisting: true,
					Activating:  true,
				}},
			}
			if err := writeInstallTransaction(filepath.Join(root, installTransactionFilename), journal); err != nil {
				t.Fatal(err)
			}
			if err := RecoverInstallTransactionAuto(Environment{Home: root, ConfigDir: root}); err != nil {
				t.Fatal(err)
			}
			content, err := os.ReadFile(filepath.Join(destinationDirectory, "state.txt"))
			if err != nil || strings.TrimSpace(string(content)) != scenario.wantContent {
				t.Fatalf("unexpected automatic recovery: content=%q err=%v", content, err)
			}
		})
	}
}

func TestInstallTransactionLockRejectsConcurrentMutation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestSkill(t, root, "viceme-test")
	bundle := New(os.DirFS(root))
	environment := Environment{Home: root, ConfigDir: filepath.Join(root, ".viceme-cli")}
	transaction, _, err := bundle.PrepareInstallSet([]string{"viceme-test"}, "agents", environment)
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()
	if _, _, err := bundle.PrepareInstallSet([]string{"viceme-test"}, "agents", environment); err == nil || !strings.Contains(err.Error(), "another ViceMe install transaction") {
		t.Fatalf("concurrent install was not rejected: %v", err)
	}
}

func TestRetiredSkillRemovalCommitsAndRollsBackTransactionally(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		commit     bool
		wantExists bool
	}{
		{name: "commit removes retired Skill", commit: true, wantExists: false},
		{name: "rollback restores retired Skill", commit: false, wantExists: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := t.TempDir()
			writeTestSkill(t, root, "viceme-current")
			writeTestSkill(t, root, "viceme-retired")
			bundle := New(os.DirFS(root))
			home := t.TempDir()
			environment := Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli")}
			retiredPath := filepath.Join(home, ".agents", "skills", "viceme-retired")
			if err := installRetiredTestSkill(bundle, "viceme-retired", retiredPath); err != nil {
				t.Fatal(err)
			}
			digests, err := bundle.Digests("viceme-retired")
			if err != nil {
				t.Fatal(err)
			}
			retired := RetiredSkill{Name: "viceme-retired", Releases: []RetiredSkillRelease{
				{CLIVersions: []string{"0.0.0"}},
				{
					CLIVersions:  []string{buildinfo.Version},
					SkillVersion: buildinfo.SkillVersion, MinimumCLIVersion: buildinfo.MinimumCLIVersion,
					CLICompatibility: buildinfo.CLICompatibility, Digests: digests,
				},
			}}
			transaction, _, err := bundle.PrepareInstallSet([]string{"viceme-current"}, "agents", environment)
			if err != nil {
				t.Fatal(err)
			}
			if err := transaction.RetireSkill(retired, "agents", environment); err != nil {
				t.Fatal(err)
			}
			if scenario.commit {
				err = transaction.Commit()
			} else {
				err = transaction.Rollback()
			}
			if err != nil {
				t.Fatal(err)
			}
			_, statErr := os.Stat(retiredPath)
			if scenario.wantExists && statErr != nil {
				t.Fatalf("retired Skill was not restored: %v", statErr)
			}
			if !scenario.wantExists && !os.IsNotExist(statErr) {
				t.Fatalf("retired Skill still exists: %v", statErr)
			}
		})
	}
}

func TestRetiredSkillRemovalRejectsModifiedInstallation(t *testing.T) {
	root := t.TempDir()
	writeTestSkill(t, root, "viceme-current")
	writeTestSkill(t, root, "viceme-retired")
	bundle := New(os.DirFS(root))
	home := t.TempDir()
	environment := Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli")}
	retiredPath := filepath.Join(home, ".agents", "skills", "viceme-retired")
	if err := installRetiredTestSkill(bundle, "viceme-retired", retiredPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(retiredPath, "user-note.txt"), []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	digests, err := bundle.Digests("viceme-retired")
	if err != nil {
		t.Fatal(err)
	}
	transaction, _, err := bundle.PrepareInstallSet([]string{"viceme-current"}, "agents", environment)
	if err != nil {
		t.Fatal(err)
	}
	err = transaction.RetireSkill(RetiredSkill{Name: "viceme-retired", Releases: []RetiredSkillRelease{{
		CLIVersions:  []string{buildinfo.Version},
		SkillVersion: buildinfo.SkillVersion, MinimumCLIVersion: buildinfo.MinimumCLIVersion,
		CLICompatibility: buildinfo.CLICompatibility, Digests: digests,
	}}}, "agents", environment)
	if err == nil || !strings.Contains(err.Error(), "does not match the retired official release") {
		t.Fatalf("RetireSkill() error = %v", err)
	}
	if err := transaction.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(filepath.Join(retiredPath, "user-note.txt")); statErr != nil {
		t.Fatalf("modified retired Skill was removed: %v", statErr)
	}
}

func TestRetiredSkillRemovalPreservesReplacementCreatedAfterSnapshot(t *testing.T) {
	root := t.TempDir()
	writeTestSkill(t, root, "viceme-current")
	writeTestSkill(t, root, "viceme-retired")
	bundle := New(os.DirFS(root))
	home := t.TempDir()
	environment := Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli")}
	retiredPath := filepath.Join(home, ".agents", "skills", "viceme-retired")
	if err := installRetiredTestSkill(bundle, "viceme-retired", retiredPath); err != nil {
		t.Fatal(err)
	}
	digests, err := bundle.Digests("viceme-retired")
	if err != nil {
		t.Fatal(err)
	}
	transaction, _, err := bundle.PrepareInstallSet([]string{"viceme-current"}, "agents", environment)
	if err != nil {
		t.Fatal(err)
	}
	if err := transaction.RetireSkill(RetiredSkill{Name: "viceme-retired", Releases: []RetiredSkillRelease{{
		CLIVersions:  []string{buildinfo.Version},
		SkillVersion: buildinfo.SkillVersion, MinimumCLIVersion: buildinfo.MinimumCLIVersion,
		CLICompatibility: buildinfo.CLICompatibility, Digests: digests,
	}}}, "agents", environment); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(retiredPath, 0o755); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(retiredPath, "user-note.txt")
	if err := os.WriteFile(replacement, []byte("new user directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(replacement); err != nil || string(data) != "new user directory" {
		t.Fatalf("replacement directory was removed: data=%q err=%v", data, err)
	}
}

func installRetiredTestSkill(bundle *Bundle, name, destination string) error {
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	if err := copyTree(bundle.FS, name, destination); err != nil {
		return err
	}
	manifest, err := bundle.installManifest(name)
	if err != nil {
		return err
	}
	return writeInstallManifest(destination, manifest)
}

func writeTestSkill(t *testing.T, root, name string) {
	t.Helper()
	packageMetadata, err := json.MarshalIndent(struct {
		SchemaVersion     int    `json:"schema_version"`
		SkillVersion      string `json:"skill_version"`
		MinimumCLIVersion string `json:"minimum_cli_version"`
		CLICompatibility  string `json:"cli_compatibility"`
	}{
		SchemaVersion:     1,
		SkillVersion:      buildinfo.SkillVersion,
		MinimumCLIVersion: buildinfo.MinimumCLIVersion,
		CLICompatibility:  buildinfo.CLICompatibility,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
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
		"skill-package.json": string(packageMetadata) + "\n",
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
