package command

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	updatepkg "github.com/ViceMe-AI/cli/internal/update"
	"github.com/spf13/cobra"
)

func TestBootstrapRecoveryKeepsBinarySkillsAndConfigOnOneGeneration(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name       string
		status     string
		wantBinary string
		wantSkill  string
		wantConfig string
	}{
		{name: "preparing rolls back everything", status: "PREPARING", wantBinary: "old-binary", wantSkill: "old-skill", wantConfig: "old-config"},
		{name: "committing rolls forward everything", status: "COMMITTING", wantBinary: "new-binary", wantSkill: "new-skill", wantConfig: "new-config"},
	} {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			configDir := filepath.Join(root, "config")
			if err := os.MkdirAll(configDir, 0o700); err != nil {
				t.Fatal(err)
			}
			destination := filepath.Join(root, "bin", "viceme")
			staged := filepath.Join(configDir, bootstrapActivationStagedFilename)
			backup := filepath.Join(configDir, bootstrapActivationBackupFilename)
			writeBootstrapTestFile(t, destination, "old-binary")
			writeBootstrapTestFile(t, staged, "new-binary")
			writeBootstrapTestFile(t, backup, "old-binary")
			targetHash, err := bootstrapFileHash(staged)
			if err != nil {
				t.Fatal(err)
			}
			previousHash, err := bootstrapFileHash(backup)
			if err != nil {
				t.Fatal(err)
			}
			journal := bootstrapActivationJournal{
				SchemaVersion: 1,
				Status:        scenario.status,
				Destination:   destination,
				Staged:        staged,
				Backup:        backup,
				HadExisting:   true,
				PreviousHash:  previousHash,
				TargetHash:    targetHash,
				TargetVersion: buildinfo.CompatibilityVersion(),
			}
			if err := writeBootstrapJournal(filepath.Join(configDir, bootstrapActivationJournalFilename), journal); err != nil {
				t.Fatal(err)
			}

			skillDestination := filepath.Join(root, ".agents", "skills", "viceme-publish")
			skillBackup := skillDestination + ".viceme-transaction-backup"
			configPath := filepath.Join(configDir, "config.json")
			configBackup := configPath + ".viceme-transaction-backup"
			writeBootstrapTestFile(t, filepath.Join(skillDestination, "state.txt"), "new-skill")
			writeBootstrapTestFile(t, filepath.Join(skillBackup, "state.txt"), "old-skill")
			writeBootstrapTestFile(t, configPath, "new-config")
			writeBootstrapTestFile(t, configBackup, "old-config")
			writeInstallRecoveryJournalForBootstrapTest(t, configDir, skillDestination, skillBackup, configPath, configBackup)

			environment := skillcontent.Environment{Home: root, ConfigDir: configDir}
			if err := recoverBootstrapActivation(configDir, environment); err != nil {
				t.Fatal(err)
			}
			assertBootstrapTestContent(t, destination, scenario.wantBinary)
			assertBootstrapTestContent(t, filepath.Join(skillDestination, "state.txt"), scenario.wantSkill)
			assertBootstrapTestContent(t, configPath, scenario.wantConfig)
			active, exists, err := updatepkg.ReadActiveGeneration(configDir)
			if scenario.status == "COMMITTING" {
				if err != nil || !exists || active.Version != buildinfo.CompatibilityVersion() || active.InstallMethod != "standalone" || active.Identity != targetHash {
					t.Fatalf("committed bootstrap did not publish its generation: active=%#v exists=%t err=%v", active, exists, err)
				}
			} else if err != nil || exists {
				t.Fatalf("rolled-back bootstrap published a generation: active=%#v exists=%t err=%v", active, exists, err)
			}
			for _, filename := range []string{
				filepath.Join(configDir, bootstrapActivationJournalFilename),
				filepath.Join(configDir, "install-transaction.json"),
				staged,
				backup,
			} {
				if _, err := os.Stat(filename); !os.IsNotExist(err) {
					t.Fatalf("recovery artifact was not retired: %s: %v", filename, err)
				}
			}
		})
	}
}

func TestBootstrapRejectsLateOlderGenerationInsideActivationLock(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	active, err := updatepkg.NewStandaloneGeneration("0.10.2", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	if err := updatepkg.CommitActiveGeneration(configDir, active); err != nil {
		t.Fatal(err)
	}
	runtime := &Runtime{
		configBase: configDir,
		region:     config.RegionCN,
		deps: Dependencies{
			Store:       securestore.NewMemory(),
			Environment: skillcontent.Environment{Home: root, ConfigDir: configDir},
		},
	}
	destination := filepath.Join(root, "bin", "viceme")
	_, err = activateBootstrap(&cobra.Command{}, runtime, destination, "agents", "cn")
	cliError := output.AsError(err)
	if cliError.Subtype != "BOOTSTRAP_DOWNGRADE_REFUSED" {
		t.Fatalf("late older standalone activation was not fenced: %#v", cliError)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("fenced standalone activation changed destination: %v", err)
	}
	current, exists, err := updatepkg.ReadActiveGeneration(configDir)
	if err != nil || !exists || current != active {
		t.Fatalf("fenced activation changed active generation: current=%#v exists=%t err=%v", current, exists, err)
	}
}

func TestBootstrapRejectsNPMToStandaloneMigrationBeforeMutation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	active, err := updatepkg.NewNPMGeneration(buildinfo.CompatibilityVersion())
	if err != nil {
		t.Fatal(err)
	}
	if err := updatepkg.CommitActiveGeneration(configDir, active); err != nil {
		t.Fatal(err)
	}
	runtime := &Runtime{
		configBase: configDir,
		region:     config.RegionCN,
		deps: Dependencies{
			Store:       securestore.NewMemory(),
			Environment: skillcontent.Environment{Home: root, ConfigDir: configDir},
		},
	}
	destination := filepath.Join(root, "bin", "viceme")
	_, err = activateBootstrap(&cobra.Command{}, runtime, destination, "agents", "cn")
	cliError := output.AsError(err)
	if cliError.Subtype != "BOOTSTRAP_INSTALL_METHOD_CHANGE_REFUSED" {
		t.Fatalf("cross-method bootstrap was not rejected: %#v", cliError)
	}
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected cross-method bootstrap mutated destination: %v", err)
	}
	current, exists, err := updatepkg.ReadActiveGeneration(configDir)
	if err != nil || !exists || current != active {
		t.Fatalf("rejected cross-method bootstrap changed active generation: active=%#v exists=%t err=%v", current, exists, err)
	}
}

func TestStandaloneRecoveryRequiresOldProcessToRestartAfterRollForward(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	destination := filepath.Join(root, "bin", "viceme")
	staged := filepath.Join(configDir, bootstrapActivationStagedFilename)
	backup := filepath.Join(configDir, bootstrapActivationBackupFilename)
	writeBootstrapTestFile(t, destination, "old-binary")
	writeBootstrapTestFile(t, staged, "new-binary")
	writeBootstrapTestFile(t, backup, "old-binary")
	targetHash, err := bootstrapFileHash(staged)
	if err != nil {
		t.Fatal(err)
	}
	previousHash, err := bootstrapFileHash(backup)
	if err != nil {
		t.Fatal(err)
	}
	journal := bootstrapActivationJournal{
		SchemaVersion: 1,
		Status:        "COMMITTING",
		Destination:   destination,
		Staged:        staged,
		Backup:        backup,
		HadExisting:   true,
		PreviousHash:  previousHash,
		TargetHash:    targetHash,
		TargetVersion: buildinfo.CompatibilityVersion(),
	}
	if err := writeBootstrapJournal(filepath.Join(configDir, bootstrapActivationJournalFilename), journal); err != nil {
		t.Fatal(err)
	}
	dependencies := Dependencies{
		Updater: updatepkg.NewReleaseService(buildinfo.Version, buildinfo.CompatibilityVersion()),
		Environment: skillcontent.Environment{
			Home: root, ConfigDir: configDir,
		},
	}
	if err := reconcileActivationAtStartup(context.Background(), configDir, &dependencies); !errors.Is(err, updatepkg.ErrActivationRestartNeeded) {
		t.Fatalf("old standalone process continued after recovery: %v", err)
	}
	assertBootstrapTestContent(t, destination, "new-binary")
	active, exists, err := updatepkg.ReadActiveGeneration(configDir)
	if err != nil || !exists || active.InstallMethod != "standalone" || active.Identity != targetHash {
		t.Fatalf("standalone target did not commit before restart fence: active=%#v exists=%t err=%v", active, exists, err)
	}
}

func writeInstallRecoveryJournalForBootstrapTest(t *testing.T, configDir, skillDestination, skillBackup, configPath, configBackup string) {
	t.Helper()
	contents := `{
  "schema_version": 1,
  "status": "COMMITTING",
  "target_cli_version": "dev",
  "entries": [
    {"destination": ` + quotedJSON(skillDestination) + `, "stage": "", "backup": ` + quotedJSON(skillBackup) + `, "had_existing": true, "activating": true},
    {"destination": ` + quotedJSON(configPath) + `, "stage": "", "backup": ` + quotedJSON(configBackup) + `, "had_existing": true, "activating": true}
  ]
}
`
	if err := os.WriteFile(filepath.Join(configDir, "install-transaction.json"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func quotedJSON(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func writeBootstrapTestFile(t *testing.T, filename, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func assertBootstrapTestContent(t *testing.T, filename, expected string) {
	t.Helper()
	content, err := os.ReadFile(filename)
	if err != nil || string(content) != expected {
		t.Fatalf("unexpected content at %s: content=%q err=%v", filename, content, err)
	}
}
