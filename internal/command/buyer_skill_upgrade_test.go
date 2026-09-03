package command

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	cliembed "github.com/ViceMe-AI/cli"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestBuyerSkillUpgradeRetiresOnlyUnmodifiedManagedLegacyName(t *testing.T) {
	current := skillcontent.New(cliembed.EmbeddedSkills())
	legacyFiles := fstest.MapFS{}
	for _, relative := range []string{"SKILL.md", "agents/openai.yaml", "skill-package.json"} {
		data, err := fs.ReadFile(cliembed.EmbeddedSkills(), "use-a-skill/"+relative)
		if err != nil {
			t.Fatal(err)
		}
		legacyFiles["viceme-skill-use/"+relative] = &fstest.MapFile{
			Data: []byte(strings.ReplaceAll(string(data), "use-a-skill", "viceme-skill-use")),
		}
	}
	legacy := skillcontent.New(legacyFiles)
	for _, modified := range []bool{false, true} {
		name := "managed"
		if modified {
			name = "user-modified"
		}
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			environment := skillcontent.Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli")}
			if report := legacy.Install("viceme-skill-use", "workbuddy", environment); !report.AllSucceeded {
				t.Fatalf("legacy installation failed: %#v", report)
			}
			oldPath := filepath.Join(home, ".workbuddy", "skills", "viceme-skill-use")
			if modified {
				if err := os.WriteFile(filepath.Join(oldPath, "SKILL.md"), []byte("user customization\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			transaction, _, err := current.PrepareInstallSetWithRetirements(officialSkillNames, retiredOfficialSkills, "workbuddy", environment)
			if err != nil {
				t.Fatal(err)
			}
			defer transaction.Rollback()
			if err := transaction.Commit(); err != nil {
				t.Fatal(err)
			}
			if !current.Doctor("use-a-skill", "workbuddy", environment).Healthy {
				t.Fatal("the current buyer Skill was not activated")
			}
			if modified {
				data, err := os.ReadFile(filepath.Join(oldPath, "SKILL.md"))
				if err != nil || string(data) != "user customization\n" {
					t.Fatalf("modified Skill was not preserved: %q %v", data, err)
				}
			} else if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
				t.Fatalf("old buyer instructions remain discoverable after upgrade: %v", err)
			}
		})
	}
}
