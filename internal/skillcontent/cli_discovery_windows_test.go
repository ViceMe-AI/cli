package skillcontent_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	cliembed "github.com/ViceMe-AI/cli"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestInstalledPowerShellCLIResolver(t *testing.T) {
	for _, scenario := range []string{"default", "custom", "launcher", "missing"} {
		t.Run(scenario, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "user space")
			environment := skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")}
			for _, report := range skillcontent.New(cliembed.EmbeddedSkills()).InstallSet([]string{"creator-tools"}, "workbuddy", environment) {
				if !report.AllSucceeded {
					t.Fatalf("install resolver: %#v", report)
				}
			}
			localAppData := filepath.Join(root, "Local")
			directory := filepath.Join(localAppData, "ViceMe", "bin")
			commandPath := filepath.Join(os.Getenv("SystemRoot"), "System32")
			custom := ""
			if scenario == "custom" {
				directory = filepath.Join(root, "custom bin")
				custom = directory
			}
			if scenario == "launcher" {
				directory = filepath.Join(root, "npm bin")
				commandPath = directory + ";" + commandPath
			}
			candidate := filepath.Join(directory, "viceme.exe")
			if scenario != "missing" {
				if err := os.MkdirAll(directory, 0o755); err != nil {
					t.Fatal(err)
				}
				binary, err := os.ReadFile(filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe"))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(candidate, binary, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			resolver := filepath.Join(root, ".workbuddy", "skills", "creator-tools", "scripts", "resolve-cli.ps1")
			command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", resolver)
			command.Env = append(os.Environ(), "PATH="+commandPath, "LOCALAPPDATA="+localAppData, "VICEME_INSTALL_DIR="+custom)
			output, err := command.CombinedOutput()
			if scenario == "missing" {
				failure, ok := err.(*exec.ExitError)
				if !ok || failure.ExitCode() != 127 {
					t.Fatalf("unexpected missing CLI result: err=%v output=%s", err, output)
				}
				return
			}
			resolved := strings.TrimSpace(string(output))
			if err != nil || !filepath.IsAbs(resolved) {
				t.Fatalf("existing CLI not resolved: err=%v got=%q want=%q", err, resolved, candidate)
			}
			// PowerShell expands a Windows 8.3 user-directory alias to its long
			// spelling. Compare file identity instead of path text.
			resolvedInfo, resolvedErr := os.Stat(resolved)
			candidateInfo, candidateErr := os.Stat(candidate)
			if resolvedErr != nil || candidateErr != nil || !os.SameFile(resolvedInfo, candidateInfo) {
				t.Fatalf("resolver selected a different executable: got=%q want=%q errors=%v/%v", resolved, candidate, resolvedErr, candidateErr)
			}
			followup := exec.Command(resolved, "/c", "echo", "resolved")
			followup.Env = append(os.Environ(), "PATH="+filepath.Join(os.Getenv("SystemRoot"), "System32"))
			if output, err := followup.CombinedOutput(); err != nil || strings.TrimSpace(string(output)) != "resolved" {
				t.Fatalf("independent call cannot use resolved executable: %v %s", err, output)
			}
		})
	}
}
