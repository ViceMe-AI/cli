package skillcontent

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/buildinfo"
)

func TestClaudeInstallAlsoWritesAgentsFallbackAndRepairsDrift(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestSkill(t, root, "viceme-test")
	bundle := New(os.DirFS(root))
	home := t.TempDir()
	environment := Environment{Home: home, ClaudeConfigDir: filepath.Join(home, ".claude"), ConfigDir: filepath.Join(home, ".viceme-cli")}

	report := bundle.Install("viceme-test", "claude", environment)
	if !report.AllSucceeded || len(report.Results) != 2 {
		t.Fatalf("unexpected install report: %#v", report)
	}
	for _, directory := range []string{
		filepath.Join(home, ".claude", "skills", "viceme-test"),
		filepath.Join(home, ".agents", "skills", "viceme-test"),
	} {
		if _, err := os.Stat(filepath.Join(directory, "SKILL.md")); err != nil {
			t.Fatalf("missing installed Skill at %s: %v", directory, err)
		}
	}

	installed := filepath.Join(home, ".claude", "skills", "viceme-test", "SKILL.md")
	if err := os.WriteFile(installed, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if bundle.Doctor("viceme-test", "claude", environment).Healthy {
		t.Fatal("doctor accepted a modified Skill")
	}
	repaired := bundle.Install("viceme-test", "claude", environment)
	if !repaired.AllSucceeded || !bundle.Doctor("viceme-test", "claude", environment).Healthy {
		t.Fatalf("drift was not repaired: %#v", repaired)
	}
}

func TestCodexInstallHonorsIsolatedAgentsSkillsDirectory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestSkill(t, root, "viceme-test")
	home := t.TempDir()
	agentsSkillsDir := filepath.Join(t.TempDir(), "shared-agent-skills")
	environment := Environment{
		Home:            home,
		CodexHome:       filepath.Join(home, ".codex"),
		AgentsSkillsDir: agentsSkillsDir,
		ConfigDir:       filepath.Join(home, ".viceme-cli"),
	}
	bundle := New(os.DirFS(root))
	report := bundle.Install("viceme-test", "codex", environment)
	if !report.AllSucceeded || len(report.Results) != 1 {
		t.Fatalf("install failed: %#v", report)
	}
	for _, directory := range []string{
		filepath.Join(agentsSkillsDir, "viceme-test"),
	} {
		if _, err := os.Stat(filepath.Join(directory, "SKILL.md")); err != nil {
			t.Fatalf("missing installed Skill at %s: %v", directory, err)
		}
	}
	if _, err := os.Stat(environment.CodexHome); !os.IsNotExist(err) {
		t.Fatalf("legacy Codex directory was unexpectedly written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "viceme-test")); !os.IsNotExist(err) {
		t.Fatalf("default shared Agent directory was unexpectedly written: %v", err)
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
	if !report.AllSucceeded || len(report.Results) != 2 {
		t.Fatalf("auto targets did not include fallback and detected agents: %#v", report)
	}
	got := map[string]bool{}
	for _, result := range report.Results {
		got[result.Target] = true
	}
	if !got["agents"] || got["codex"] || !got["claude"] || got["workbuddy"] {
		t.Fatalf("unexpected auto target set: %#v", got)
	}
	doctor := bundle.Doctor("viceme-test", "auto", Environment{Home: home})
	if !doctor.Healthy || len(doctor.Results) != 2 {
		t.Fatalf("Doctor required an agent that was not detected: %#v", doctor)
	}
}

func TestInstallSetActivatesAllOfficialSkillsTogether(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestSkill(t, root, "creator-tools")
	writeTestSkill(t, root, "become-a-creator")
	writeTestSkill(t, root, "customize-your-page")
	writeTestSkill(t, root, "sell-a-skill")
	writeTestSkill(t, root, "use-a-skill")
	writeTestSkill(t, root, "charge-for-your-work")
	writeTestSkill(t, root, "let-people-interact")
	writeTestSkill(t, root, "let-others-make-a-copy")
	writeTestSkill(t, root, "let-me-make-a-copy")
	home := t.TempDir()
	environment := Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli")}
	bundle := New(os.DirFS(root))

	skillNames := []string{"creator-tools", "become-a-creator", "customize-your-page", "sell-a-skill", "use-a-skill", "charge-for-your-work", "let-people-interact", "let-others-make-a-copy", "let-me-make-a-copy"}
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

func TestLegacyRelativeInstallJournalStillRecovers(t *testing.T) {
	t.Parallel()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := tempDirOnWorkingVolume(t)
	destination := filepath.Join(root, "skills", "viceme-test")
	backup := destination + ".viceme-transaction-backup"
	for filename, content := range map[string]string{
		filepath.Join(destination, "state.txt"): "new",
		filepath.Join(backup, "state.txt"):      "old",
	} {
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	relativeDestination, err := filepath.Rel(workingDirectory, destination)
	if err != nil {
		t.Fatal(err)
	}
	relativeBackup, err := filepath.Rel(workingDirectory, backup)
	if err != nil {
		t.Fatal(err)
	}
	journal := installTransaction{SchemaVersion: 1, Status: "PREPARING", Entries: []installJournalEntry{{
		Destination: relativeDestination, Backup: relativeBackup, HadExisting: true, Activating: true,
	}}}
	journalPath := filepath.Join(root, installTransactionFilename)
	if err := writeInstallTransaction(journalPath, journal); err != nil {
		t.Fatalf("legacy schema 1 journal was rejected: %v", err)
	}
	if err := recoverInstallTransaction(journalPath); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(destination, "state.txt"))
	if err != nil || string(content) != "old" {
		t.Fatalf("legacy relative journal did not restore the prior generation: content=%q err=%v", content, err)
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

func TestInstallTransactionLockRejectsSameDestinationAcrossConfigDirectories(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestSkill(t, root, "viceme-test")
	bundle := New(os.DirFS(root))
	home := t.TempDir()
	provenance := SkillProvenance{ProductID: "product-1", ReleaseID: "release-1"}
	firstEnvironment := Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli-a")}
	secondEnvironment := Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli-b")}

	first, _, err := bundle.PrepareInstallSetWithProvenance([]string{"viceme-test"}, "agents", firstEnvironment, provenance)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Rollback()
	second, _, err := bundle.PrepareInstallSetWithProvenance([]string{"viceme-test"}, "agents", secondEnvironment, provenance)
	if second != nil {
		defer second.Rollback()
	}
	if err == nil || !strings.Contains(err.Error(), "another ViceMe install transaction") {
		t.Fatalf("same destination with a different config directory was not locked: %v", err)
	}
}

func TestIncompleteInstallBlocksSameDestinationAcrossConfigDirectories(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestSkill(t, root, "viceme-test")
	bundle := New(os.DirFS(root))
	home := t.TempDir()
	provenance := SkillProvenance{ProductID: "product-1", ReleaseID: "release-1"}
	firstEnvironment := Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli-a")}
	secondEnvironment := Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli-b")}

	first, _, err := bundle.PrepareInstallSetWithProvenance([]string{"viceme-test"}, "agents", firstEnvironment, provenance)
	if err != nil {
		t.Fatal(err)
	}
	first.Abandon()
	t.Cleanup(func() {
		_ = RecoverInstallTransactionAuto(firstEnvironment)
		_ = RecoverInstallTransactionAuto(secondEnvironment)
	})

	second, _, err := bundle.PrepareInstallSetWithProvenance([]string{"viceme-test"}, "agents", secondEnvironment, provenance)
	if second != nil {
		_ = second.Rollback()
	}
	if err == nil || !strings.Contains(err.Error(), "incomplete ViceMe install transaction") {
		t.Fatalf("incomplete transaction did not retain destination ownership across config directories: %v", err)
	}
	if err := RecoverInstallTransactionAuto(firstEnvironment); err != nil {
		t.Fatal(err)
	}
	third, _, err := bundle.PrepareInstallSetWithProvenance([]string{"viceme-test"}, "agents", secondEnvironment, provenance)
	if err != nil {
		t.Fatalf("recovered destination remained blocked: %v", err)
	}
	if err := third.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestInstallPreservesUnownedTransactionBackup(t *testing.T) {
	t.Parallel()
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	writeTestSkill(t, firstRoot, "viceme-test")
	writeTestSkill(t, secondRoot, "viceme-test")
	secondSkill := filepath.Join(secondRoot, "viceme-test", "SKILL.md")
	content, err := os.ReadFile(secondSkill)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondSkill, append(content, []byte("\n## Updated\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	environment := Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli")}
	destination := filepath.Join(home, ".agents", "skills", "viceme-test")
	first := New(os.DirFS(firstRoot))
	if report := first.Install("viceme-test", "agents", environment); !report.AllSucceeded {
		t.Fatalf("initial install failed: %#v", report)
	}
	previous, err := os.ReadFile(filepath.Join(destination, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	backupFile := filepath.Join(destination+".viceme-transaction-backup", "recovery.txt")
	if err := os.MkdirAll(filepath.Dir(backupFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backupFile, []byte("unowned recovery data\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	blocked := New(os.DirFS(secondRoot)).Install("viceme-test", "agents", environment)
	if blocked.AllSucceeded || !strings.Contains(blocked.Results[0].Error, "unowned transaction backup") {
		t.Fatalf("install did not refuse an unowned backup: %#v", blocked)
	}
	if actual, err := os.ReadFile(backupFile); err != nil || string(actual) != "unowned recovery data\n" {
		t.Fatalf("install changed the unowned backup: content=%q err=%v", actual, err)
	}
	if actual, err := os.ReadFile(filepath.Join(destination, "SKILL.md")); err != nil || !bytes.Equal(actual, previous) {
		t.Fatalf("blocked install changed the active Skill: content=%q err=%v", actual, err)
	}
}

func TestOfficialInstallPersistsManagedRegistryAndManifestV2(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestSkill(t, root, "viceme-test")
	bundle := New(os.DirFS(root))
	home := t.TempDir()
	environment := Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli")}
	destination := filepath.Join(home, ".agents", "skills", "viceme-test")

	report := bundle.Install("viceme-test", "agents", environment)
	if !report.AllSucceeded {
		t.Fatalf("official Skill did not install: %#v", report)
	}
	managedPath, err := normalizeManagedSkillPath(destination)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := readManagedSkillRegistry(filepath.Join(environment.ConfigDir, managedSkillRegistryFilename))
	if err != nil {
		t.Fatal(err)
	}
	record, exists := registry.Installs[managedPath]
	if !exists {
		t.Fatalf("managed registry omitted normalized path %s", managedPath)
	}
	manifest, err := readInstallManifest(destination)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := bundle.Digests("viceme-test")
	if err != nil {
		t.Fatal(err)
	}
	if record.SkillName != "viceme-test" || record.Target != "agents" || record.Digest != expected.Full || !validInstallID(record.InstallID) {
		t.Fatalf("managed registry contains an incomplete ownership record: %#v", record)
	}
	if manifest.SchemaVersion != 2 || manifest.InstallID != record.InstallID || manifest.FullBundleDigest != record.Digest || manifest.ProductID != "" || manifest.ReleaseID != "" {
		t.Fatalf("manifest v2 does not match managed registry ownership: manifest=%#v record=%#v", manifest, record)
	}

	unchanged := bundle.Install("viceme-test", "agents", environment)
	if !unchanged.AllSucceeded || unchanged.Results[0].Status != "unchanged" {
		t.Fatalf("matching registry reinstall was not idempotent: %#v", unchanged)
	}
	current, err := readManagedSkillRegistry(filepath.Join(environment.ConfigDir, managedSkillRegistryFilename))
	if err != nil {
		t.Fatal(err)
	}
	if current.Installs[managedPath].InstallID != record.InstallID {
		t.Fatal("idempotent reinstall replaced the managed install ID")
	}
}

func TestOfficialInstallClaimsLegacyV1Manifest(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestSkill(t, root, "viceme-test")
	bundle := New(os.DirFS(root))
	home := t.TempDir()
	environment := Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli")}
	destination := filepath.Join(home, ".agents", "skills", "viceme-test")
	installLegacyV1Fixture(t, bundle, "viceme-test", destination)

	report := bundle.Install("viceme-test", "agents", environment)
	if !report.AllSucceeded {
		t.Fatalf("official installer did not migrate a v1 manifest: %#v", report)
	}
	manifest, err := readInstallManifest(destination)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != 2 || !validInstallID(manifest.InstallID) {
		t.Fatalf("legacy official install did not receive manifest v2 ownership: %#v", manifest)
	}
	if !bundle.Doctor("viceme-test", "agents", environment).Healthy {
		t.Fatal("migrated legacy official install did not match the managed registry")
	}
}

func TestOfficialInstallRefusesMismatchedRegisteredManifest(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestSkill(t, root, "viceme-test")
	bundle := New(os.DirFS(root))
	home := t.TempDir()
	environment := Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli")}
	destination := filepath.Join(home, ".agents", "skills", "viceme-test")
	if report := bundle.Install("viceme-test", "agents", environment); !report.AllSucceeded {
		t.Fatalf("official install fixture failed: %#v", report)
	}
	manifest, err := readInstallManifest(destination)
	if err != nil {
		t.Fatal(err)
	}
	manifest.InstallID = "00000000-0000-4000-8000-000000000000"
	if err := writeInstallManifest(destination, manifest); err != nil {
		t.Fatal(err)
	}

	blocked := bundle.Install("viceme-test", "agents", environment)
	if blocked.AllSucceeded || !strings.Contains(blocked.Results[0].Error, "different install ID") {
		t.Fatalf("official install replaced conflicting managed ownership: %#v", blocked)
	}
	current, err := readInstallManifest(destination)
	if err != nil {
		t.Fatal(err)
	}
	if current.InstallID != manifest.InstallID {
		t.Fatal("blocked official install changed the conflicting manifest")
	}
}

func TestInstallTransactionRetiresOnlyManagedMatchingSkill(t *testing.T) {
	t.Parallel()
	activeRoot := t.TempDir()
	legacyRoot := t.TempDir()
	writeTestSkill(t, activeRoot, "viceme-current")
	writeTestSkill(t, legacyRoot, "viceme-access")
	activeBundle := New(os.DirFS(activeRoot))
	legacyBundle := New(os.DirFS(legacyRoot))
	home := t.TempDir()
	environment := Environment{
		Home:      home,
		CodexHome: filepath.Join(home, ".codex"),
		ConfigDir: filepath.Join(home, ".viceme-cli"),
	}
	if report := legacyBundle.Install("viceme-access", "agents", environment); !report.AllSucceeded {
		t.Fatalf("legacy Skill fixture did not install: %#v", report)
	}
	retired := []RetiredSkill{{Name: "viceme-access"}}
	managedPath := filepath.Join(home, ".agents", "skills", "viceme-access")
	userPath := filepath.Join(home, ".codex", "skills", "viceme-access")
	if err := os.MkdirAll(userPath, 0o755); err != nil {
		t.Fatal(err)
	}
	userContent := []byte("user-owned same-name Skill\n")
	if err := os.WriteFile(filepath.Join(userPath, "SKILL.md"), userContent, 0o644); err != nil {
		t.Fatal(err)
	}

	transaction, reports, err := activeBundle.PrepareInstallSetWithRetirements(
		[]string{"viceme-current"}, retired, "codex", environment,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || !reports[0].AllSucceeded {
		t.Fatalf("active Skill was not prepared: %#v", reports)
	}
	retirementStatuses := make(map[string]string)
	for _, result := range transaction.RetirementResults() {
		retirementStatuses[result.Target] = result.Status
	}
	if retirementStatuses["agents"] != "retired" || retirementStatuses["codex"] != "preserved_unmanaged" {
		_ = transaction.Rollback()
		t.Fatalf("retirement ownership statuses were not reported: %#v", transaction.RetirementResults())
	}
	if _, err := os.Stat(managedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("managed retired Skill was not staged for deletion: %v", err)
	}
	if actual, err := os.ReadFile(filepath.Join(userPath, "SKILL.md")); err != nil || !bytes.Equal(actual, userContent) {
		t.Fatalf("user-owned same-name directory was changed: content=%q err=%v", actual, err)
	}
	if err := transaction.Rollback(); err != nil {
		t.Fatal(err)
	}
	if !legacyBundle.Doctor("viceme-access", "agents", environment).Healthy {
		t.Fatal("rollback did not restore the managed retired Skill")
	}

	transaction, _, err = activeBundle.PrepareInstallSetWithRetirements(
		[]string{"viceme-current"}, retired, "codex", environment,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(managedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("committed installation retained the managed retired Skill: %v", err)
	}
	if actual, err := os.ReadFile(filepath.Join(userPath, "SKILL.md")); err != nil || !bytes.Equal(actual, userContent) {
		t.Fatalf("committed installation removed a user-owned same-name directory: content=%q err=%v", actual, err)
	}
	if !activeBundle.Doctor("viceme-current", "codex", environment).Healthy {
		t.Fatal("active official Skill was not committed")
	}
}

func TestRetirementPreservesOwnershipMismatch(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name       string
		mutation   string
		wantStatus string
	}{
		{name: "modified content", mutation: "content", wantStatus: "preserved_modified"},
		{name: "mismatched manifest install ID", mutation: "manifest", wantStatus: "preserved_modified"},
		{name: "mismatched registry digest", mutation: "registry-digest", wantStatus: "preserved_modified"},
		{name: "missing registry", mutation: "registry", wantStatus: "preserved_unmanaged"},
	} {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeTestSkill(t, root, "viceme-retired")
			bundle := New(os.DirFS(root))
			home := t.TempDir()
			environment := Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli")}
			destination := filepath.Join(home, ".agents", "skills", "viceme-retired")
			registryPath := filepath.Join(environment.ConfigDir, managedSkillRegistryFilename)
			if report := bundle.Install("viceme-retired", "agents", environment); !report.AllSucceeded {
				t.Fatalf("retired Skill fixture did not install: %#v", report)
			}
			switch scenario.mutation {
			case "content":
				if err := os.WriteFile(filepath.Join(destination, "SKILL.md"), []byte("user-modified\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			case "manifest":
				manifest, err := readInstallManifest(destination)
				if err != nil {
					t.Fatal(err)
				}
				manifest.InstallID = "00000000-0000-4000-8000-000000000000"
				if err := writeInstallManifest(destination, manifest); err != nil {
					t.Fatal(err)
				}
			case "registry-digest":
				registry, err := readManagedSkillRegistry(registryPath)
				if err != nil {
					t.Fatal(err)
				}
				managedPath, err := normalizeManagedSkillPath(destination)
				if err != nil {
					t.Fatal(err)
				}
				record := registry.Installs[managedPath]
				record.Digest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
				registry.Installs[managedPath] = record
				if err := writeManagedSkillRegistry(registryPath, registry); err != nil {
					t.Fatal(err)
				}
			case "registry":
				if err := os.Remove(registryPath); err != nil {
					t.Fatal(err)
				}
			default:
				t.Fatalf("unknown mutation %q", scenario.mutation)
			}

			transaction, _, err := bundle.PrepareInstallSetWithRetirements(nil, []RetiredSkill{{Name: "viceme-retired"}}, "agents", environment)
			if err != nil {
				t.Fatal(err)
			}
			results := transaction.RetirementResults()
			if len(results) != 1 || results[0].Status != scenario.wantStatus {
				_ = transaction.Rollback()
				t.Fatalf("unexpected retirement result: %#v", results)
			}
			if err := transaction.Commit(); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(destination); err != nil {
				t.Fatalf("retirement removed a preserved Skill: %v", err)
			}
		})
	}
}

func TestRetirementRecoveryKeepsRegistryAndDirectoryAtomic(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestSkill(t, root, "viceme-retired")
	bundle := New(os.DirFS(root))
	home := t.TempDir()
	environment := Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli")}
	destination := filepath.Join(home, ".agents", "skills", "viceme-retired")
	managedPath, err := normalizeManagedSkillPath(destination)
	if err != nil {
		t.Fatal(err)
	}
	registryPath := filepath.Join(environment.ConfigDir, managedSkillRegistryFilename)
	if report := bundle.Install("viceme-retired", "agents", environment); !report.AllSucceeded {
		t.Fatalf("retired Skill fixture did not install: %#v", report)
	}

	transaction, _, err := bundle.PrepareInstallSetWithRetirements(nil, []RetiredSkill{{Name: "viceme-retired"}}, "agents", environment)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		_ = transaction.Rollback()
		t.Fatalf("retirement was not staged before recovery: %v", err)
	}
	registry, err := readManagedSkillRegistry(registryPath)
	if err != nil {
		_ = transaction.Rollback()
		t.Fatal(err)
	}
	if _, exists := registry.Installs[managedPath]; exists {
		_ = transaction.Rollback()
		t.Fatal("staged retirement retained its registry record")
	}
	transaction.Abandon()
	if err := RecoverInstallTransactionAuto(environment); err != nil {
		t.Fatal(err)
	}
	if !bundle.Doctor("viceme-retired", "agents", environment).Healthy {
		t.Fatal("pre-commit crash recovery did not restore both the Skill and registry ownership")
	}

	transaction, _, err = bundle.PrepareInstallSetWithRetirements(nil, []RetiredSkill{{Name: "viceme-retired"}}, "agents", environment)
	if err != nil {
		t.Fatal(err)
	}
	if err := transaction.MarkCommitting(); err != nil {
		_ = transaction.Rollback()
		t.Fatal(err)
	}
	transaction.Abandon()
	if err := RecoverInstallTransactionAuto(environment); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("committing crash recovery restored a retired Skill: %v", err)
	}
	registry, err = readManagedSkillRegistry(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := registry.Installs[managedPath]; exists {
		t.Fatal("committing crash recovery restored a retired registry record")
	}
}

func TestRetirementUsesEveryRegisteredTarget(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestSkill(t, root, "viceme-current")
	writeTestSkill(t, root, "viceme-retired")
	bundle := New(os.DirFS(root))
	home := t.TempDir()
	environment := Environment{
		Home:            home,
		CodexHome:       filepath.Join(home, ".codex"),
		ClaudeConfigDir: filepath.Join(home, ".claude"),
		ConfigDir:       filepath.Join(home, ".viceme-cli"),
	}
	if report := bundle.Install("viceme-retired", "claude", environment); !report.AllSucceeded {
		t.Fatalf("retired Skill fixture did not install: %#v", report)
	}
	retiredPaths := map[string]string{
		"agents": filepath.Join(home, ".agents", "skills", "viceme-retired"),
		"claude": filepath.Join(environment.ClaudeConfigDir, "skills", "viceme-retired"),
	}

	transaction, reports, err := bundle.PrepareInstallSetWithRetirements(
		[]string{"viceme-current"}, []RetiredSkill{{Name: "viceme-retired"}}, "codex", environment,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || !reports[0].AllSucceeded {
		_ = transaction.Rollback()
		t.Fatalf("current Skill was not prepared: %#v", reports)
	}
	statuses := make(map[string]string)
	for _, result := range transaction.RetirementResults() {
		statuses[result.Target] = result.Status
	}
	for target := range retiredPaths {
		if statuses[target] != "retired" {
			_ = transaction.Rollback()
			t.Fatalf("registered %s retirement was not staged: %#v", target, transaction.RetirementResults())
		}
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
	for target, destination := range retiredPaths {
		if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("retired %s copy remains after an upgrade using another agent: %v", target, err)
		}
	}
	if !bundle.Doctor("viceme-current", "codex", environment).Healthy {
		t.Fatal("current Skill did not commit while retiring other registered targets")
	}
}

func TestLegacyRetirementMigrationRequiresExactV1Digest(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name       string
		modified   bool
		wantStatus string
		wantExists bool
	}{
		{name: "exact legacy install", wantStatus: "retired"},
		{name: "modified legacy install", modified: true, wantStatus: "preserved_modified", wantExists: true},
	} {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeTestSkill(t, root, "viceme-legacy")
			bundle := New(os.DirFS(root))
			home := t.TempDir()
			environment := Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli")}
			destination := filepath.Join(home, ".agents", "skills", "viceme-legacy")
			identity := installLegacyV1Fixture(t, bundle, "viceme-legacy", destination)
			if scenario.modified {
				if err := os.WriteFile(filepath.Join(destination, "SKILL.md"), []byte("user-modified\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			retired := RetiredSkill{Name: "viceme-legacy", LegacyMigrations: []LegacyRetiredSkillIdentity{identity}}
			transaction, _, err := bundle.PrepareInstallSetWithRetirements(nil, []RetiredSkill{retired}, "agents", environment)
			if err != nil {
				t.Fatal(err)
			}
			results := transaction.RetirementResults()
			if len(results) != 1 || results[0].Status != scenario.wantStatus {
				_ = transaction.Rollback()
				t.Fatalf("unexpected legacy retirement result: %#v", results)
			}
			if err := transaction.Commit(); err != nil {
				t.Fatal(err)
			}
			_, statErr := os.Stat(destination)
			if scenario.wantExists && statErr != nil {
				t.Fatalf("modified legacy Skill was not preserved: %v", statErr)
			}
			if !scenario.wantExists && !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("exact legacy Skill was not retired: %v", statErr)
			}
		})
	}
}

func TestLegacyRetirementMigrationScansAllTargetsOnlyOnce(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestSkill(t, root, "viceme-legacy")
	bundle := New(os.DirFS(root))
	home := t.TempDir()
	environment := Environment{
		Home:               home,
		ClaudeConfigDir:    filepath.Join(home, ".claude"),
		WorkBuddyConfigDir: filepath.Join(home, ".workbuddy"),
		ConfigDir:          filepath.Join(home, ".viceme-cli"),
	}
	claudePath := filepath.Join(environment.ClaudeConfigDir, "skills", "viceme-legacy")
	identity := installLegacyV1Fixture(t, bundle, "viceme-legacy", claudePath)
	retired := []RetiredSkill{{Name: "viceme-legacy", LegacyMigrations: []LegacyRetiredSkillIdentity{identity}}}

	transaction, _, err := bundle.PrepareInstallSetWithRetirements(nil, retired, "agents", environment)
	if err != nil {
		t.Fatal(err)
	}
	results := transaction.RetirementResults()
	if len(results) != 1 || results[0].Target != "claude" || results[0].Status != "retired" {
		_ = transaction.Rollback()
		t.Fatalf("one-time migration did not scan the non-selected Claude target: %#v", results)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
	registry, err := readManagedSkillRegistry(filepath.Join(environment.ConfigDir, managedSkillRegistryFilename))
	if err != nil {
		t.Fatal(err)
	}
	if registry.LegacyMigrationVersion != legacyRetiredSkillMigrationVersion {
		t.Fatalf("legacy migration completion was not persisted: %#v", registry)
	}

	workbuddyPath := filepath.Join(environment.WorkBuddyConfigDir, "skills", "viceme-legacy")
	installLegacyV1Fixture(t, bundle, "viceme-legacy", workbuddyPath)
	transaction, _, err = bundle.PrepareInstallSetWithRetirements(nil, retired, "workbuddy", environment)
	if err != nil {
		t.Fatal(err)
	}
	results = transaction.RetirementResults()
	if len(results) != 1 || results[0].Target != "workbuddy" || results[0].Status != "preserved_unmanaged" {
		_ = transaction.Rollback()
		t.Fatalf("completed migration reused the legacy hash catalog: %#v", results)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(workbuddyPath); err != nil {
		t.Fatalf("post-migration legacy-looking directory was not preserved: %v", err)
	}
}

func TestInstallJournalUsesCanonicalAbsolutePaths(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestSkill(t, root, "viceme-test")
	bundle := New(os.DirFS(root))
	home := tempDirOnWorkingVolume(t)
	actualAgents := filepath.Join(home, "actual-agents")
	if err := os.MkdirAll(actualAgents, 0o755); err != nil {
		t.Fatal(err)
	}
	agentsLink := filepath.Join(home, "agents-link")
	if err := os.Symlink(actualAgents, agentsLink); err != nil {
		t.Skipf("parent symlinks are unavailable on this platform: %v", err)
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relativeAgents, err := filepath.Rel(workingDirectory, agentsLink)
	if err != nil {
		t.Fatal(err)
	}
	environment := Environment{
		Home:            home,
		AgentsSkillsDir: relativeAgents,
		ConfigDir:       filepath.Join(home, ".viceme-cli"),
	}
	transaction, _, err := bundle.PrepareInstallSet([]string{"viceme-test"}, "agents", environment)
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()
	canonicalAgents, err := filepath.EvalSymlinks(actualAgents)
	if err != nil {
		t.Fatal(err)
	}
	wantDestination := filepath.Join(canonicalAgents, "viceme-test")
	if runtime.GOOS == "windows" {
		wantDestination = strings.ToLower(wantDestination)
	}
	foundSkill := false
	for _, entry := range transaction.journal.Entries {
		for _, filename := range []string{entry.Destination, entry.Backup, entry.Stage} {
			if filename != "" && !filepath.IsAbs(filename) {
				t.Fatalf("install journal retained a relative path: %#v", entry)
			}
		}
		if entry.Destination == wantDestination {
			foundSkill = true
		}
	}
	if !foundSkill {
		t.Fatalf("install journal did not canonicalize the target parent symlink: %#v", transaction.journal.Entries)
	}
	if !filepath.IsAbs(transaction.journalPath) {
		t.Fatalf("install journal path is relative: %s", transaction.journalPath)
	}
}

func TestInstallNormalizesRelativeConfigDirectory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestSkill(t, root, "viceme-test")
	bundle := New(os.DirFS(root))
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	absoluteConfig := filepath.Join(tempDirOnWorkingVolume(t), "config")
	relativeConfig, err := filepath.Rel(workingDirectory, absoluteConfig)
	if err != nil {
		t.Fatal(err)
	}
	environment := Environment{Home: t.TempDir(), ConfigDir: relativeConfig}
	transaction, _, err := bundle.PrepareInstallSet([]string{"viceme-test"}, "agents", environment)
	if err != nil {
		t.Fatalf("relative config directory was not normalized: %v", err)
	}
	defer transaction.Rollback()
	canonicalParent, err := filepath.EvalSymlinks(filepath.Dir(absoluteConfig))
	if err != nil {
		t.Fatal(err)
	}
	wantJournal := filepath.Join(canonicalParent, filepath.Base(absoluteConfig), installTransactionFilename)
	if transaction.journalPath != wantJournal || !filepath.IsAbs(transaction.journalPath) {
		t.Fatalf("relative config directory produced a non-canonical journal path: got %s want %s", transaction.journalPath, wantJournal)
	}
}

func installLegacyV1Fixture(t *testing.T, bundle *Bundle, name, destination string) LegacyRetiredSkillIdentity {
	t.Helper()
	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyTree(bundle.FS, name, destination); err != nil {
		t.Fatal(err)
	}
	manifest, err := bundle.installManifest(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeInstallManifest(destination, manifest); err != nil {
		t.Fatal(err)
	}
	return LegacyRetiredSkillIdentity{
		Name:                  name,
		SkillVersion:          manifest.SkillVersion,
		MinimumCLIVersion:     manifest.MinimumCLIVersion,
		CLICompatibility:      manifest.CLICompatibility,
		FullBundleDigest:      manifest.FullBundleDigest,
		EmbeddedContentDigest: manifest.EmbeddedContentDigest,
		Provenance:            "commit:0000000000000000000000000000000000000000",
	}
}

func tempDirOnWorkingVolume(t *testing.T) string {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	directory, err := os.MkdirTemp(workingDirectory, ".viceme-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(directory); err != nil {
			t.Errorf("remove same-volume test directory: %v", err)
		}
	})
	return directory
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

func TestCodexInstallUsesSharedDirectoryOnce(t *testing.T) {
	t.Parallel()
	for _, target := range []string{"codex", "agents", "auto"} {
		t.Run(target, func(t *testing.T) {
			root := t.TempDir()
			writeTestSkill(t, root, "viceme-test")
			bundle := New(os.DirFS(root))
			home := t.TempDir()
			environment := Environment{Home: home, CodexHome: filepath.Join(home, "custom-codex"), ConfigDir: filepath.Join(home, ".viceme-cli")}
			// Existing Codex installations must not add a second Skill destination.
			if err := os.MkdirAll(environment.CodexHome, 0o755); err != nil {
				t.Fatal(err)
			}
			for _, installTarget := range []string{target, "agents", "codex"} {
				report := bundle.Install("viceme-test", installTarget, environment)
				if !report.AllSucceeded || len(report.Results) != 1 || report.Results[0].Target != "agents" {
					t.Fatalf("expected one canonical destination: %#v", report)
				}
				// Compare filesystem identity: Windows folds managed paths to lower
				// case, while macOS temporary paths may pass through a symlink.
				expectedInfo, expectedErr := os.Stat(filepath.Join(home, ".agents", "skills", "viceme-test"))
				actualInfo, actualErr := os.Stat(report.Results[0].Path)
				if expectedErr != nil || actualErr != nil || !os.SameFile(expectedInfo, actualInfo) {
					t.Fatalf("unexpected destination: %#v (expected stat: %v; actual stat: %v)", report.Results[0], expectedErr, actualErr)
				}
				for _, doctorTarget := range []string{"codex", "agents", "auto"} {
					doctor := bundle.Doctor("viceme-test", doctorTarget, environment)
					if !doctor.Healthy || len(doctor.Results) != 1 {
						t.Fatalf("shared installation not healthy for %s: %#v", doctorTarget, doctor)
					}
				}
			}
			if _, err := os.Stat(filepath.Join(environment.CodexHome, "skills")); !os.IsNotExist(err) {
				t.Fatalf("legacy Codex skills directory was unexpectedly written: %v", err)
			}
		})
	}
}
