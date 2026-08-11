package command

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ViceMe-AI/cli/internal/skillcontent"
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
