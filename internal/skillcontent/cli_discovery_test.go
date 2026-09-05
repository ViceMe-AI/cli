package skillcontent_test

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	cliembed "github.com/ViceMe-AI/cli"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestInstalledCLIResolverWorksInFreshAgentShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX resolver; Windows has a PowerShell process test")
	}
	for _, scenario := range []string{"default", "custom", "existing launcher", "missing", "not executable"} {
		t.Run(scenario, func(t *testing.T) {
			parent, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(parent, "home with spaces")
			environment := skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")}
			bundle := skillcontent.New(cliembed.EmbeddedSkills())
			for _, report := range bundle.InstallSet([]string{"creator-tools"}, "workbuddy", environment) {
				if !report.AllSucceeded {
					t.Fatalf("install resolver through real Skill transaction: %#v", report)
				}
			}
			directory := filepath.Join(root, ".local", "bin")
			commandPath := "/usr/bin:/bin"
			custom := ""
			switch scenario {
			case "custom":
				directory = filepath.Join(root, "custom install")
				custom = directory
			case "existing launcher":
				directory = filepath.Join(root, "npm bin")
				commandPath = directory + ":" + commandPath
			}
			candidate := filepath.Join(directory, "viceme")
			if scenario != "missing" {
				if err := os.MkdirAll(directory, 0o755); err != nil {
					t.Fatal(err)
				}
				mode := fs.FileMode(0o755)
				if scenario == "not executable" {
					mode = 0o644
				}
				fixture := candidate
				if scenario == "existing launcher" {
					fixture = filepath.Join(directory, "npm-launcher")
					if err := os.Symlink(fixture, candidate); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.WriteFile(fixture, []byte("#!/bin/sh\nprintf '[%s]\\n' \"$@\"\n"), mode); err != nil {
					t.Fatal(err)
				}
			}
			resolver := filepath.Join(root, ".workbuddy", "skills", "creator-tools", "scripts", "resolve-cli.sh")
			if _, err := os.Stat(resolver); err != nil {
				t.Fatalf("installed bundle omitted the resolver: %v", err)
			}
			command := exec.Command("/bin/sh", resolver)
			command.Env = []string{"HOME=" + root, "PATH=" + commandPath, "VICEME_INSTALL_DIR=" + custom}
			output, err := command.CombinedOutput()
			if scenario == "missing" || scenario == "not executable" {
				want := 127
				if scenario == "not executable" {
					want = 6
				}
				failure, ok := err.(*exec.ExitError)
				if !ok || failure.ExitCode() != want {
					t.Fatalf("missing and inaccessible installation were conflated: err=%v output=%s", err, output)
				}
				return
			}
			resolved := strings.TrimSpace(string(output))
			if err != nil || resolved != candidate {
				t.Fatalf("installed CLI was not found in a fresh shell: got=%q want=%q err=%v", output, candidate, err)
			}
			// A second independent tool call has no PATH export from the first.
			// Preserve argument boundaries (URLs, spaces) through the resolved path.
			followup := exec.Command("/bin/sh", "-c", `exec "$1" skill detail "$2"`, "sh", resolved, "https://example.test/a skill?product=123&install=owned")
			followup.Env = []string{"HOME=" + root, "PATH=/usr/bin:/bin"}
			result, err := followup.CombinedOutput()
			if err != nil || string(result) != "[skill]\n[detail]\n[https://example.test/a skill?product=123&install=owned]\n" {
				t.Fatalf("fresh tool call did not reach the existing CLI: %v %s", err, result)
			}
		})
	}
}

func TestEveryOfficialSkillResolvesCLIThroughCreatorTools(t *testing.T) {
	for _, name := range officialSkillNames {
		content, err := fs.ReadFile(cliembed.EmbeddedSkills(), name+"/SKILL.md")
		if err != nil {
			t.Fatal(err)
		}
		if name == "creator-tools" {
			if !strings.Contains(string(content), "scripts/resolve-cli.sh") || !strings.Contains(string(content), "scripts/resolve-cli.ps1") {
				t.Fatal("creator-tools does not own both platform resolvers")
			}
		} else if !strings.Contains(string(content), "../creator-tools/SKILL.md#cli-定位") {
			t.Errorf("%s bypasses the common CLI discovery contract", name)
		}
	}
}
