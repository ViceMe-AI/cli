package command

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ViceMe-AI/cli/internal/skillcontent"
	updatepkg "github.com/ViceMe-AI/cli/internal/update"
)

func TestRootInstallBootstrapsSkillConfigAndLoginNextStep(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	configDirectory := filepath.Join(home, "config")
	environment := skillcontent.Environment{Home: home, ConfigDir: configDirectory}
	code, stdout, stderr, _ := runCLIWithDependencies(t, nil, nil, "", Dependencies{Environment: environment},
		"install", "--target", "codex", "--json")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	var envelope struct {
		Data struct {
			NextStep struct {
				Command string `json:"command"`
			} `json:"next_step"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.NextStep.Command != "viceme auth login --no-wait --json" {
		t.Fatalf("bootstrap did not return device-login next step: %s", stdout)
	}
	for _, filename := range []string{
		filepath.Join(home, ".codex", "skills", "viceme", "SKILL.md"),
		filepath.Join(home, ".codex", "skills", "viceme", ".viceme", "install-manifest.json"),
		filepath.Join(configDirectory, "viceme", "config.json"),
	} {
		if _, err := os.Stat(filename); err != nil {
			t.Fatalf("bootstrap did not create %s: %v", filename, err)
		}
	}
}

func TestSkillsDoctorReportsEveryReleaseContractAndFailsOnVersionDrift(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	environment := skillcontent.Environment{Home: home}
	code, _, stderr, _ := runCLIWithDependencies(t, nil, nil, "", Dependencies{Environment: environment},
		"skills", "install", "--target", "codex", "--json")
	if code != 0 || stderr != "" {
		t.Fatalf("install code=%d stderr=%s", code, stderr)
	}
	code, stdout, stderr, _ := runCLIWithDependencies(t, nil, nil, "", Dependencies{Environment: environment},
		"skills", "doctor", "--target", "codex", "--json")
	if code != 0 || stderr != "" {
		t.Fatalf("doctor code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	for _, required := range []string{"cli_version", "skill_version", "full_bundle_digest", "embedded_content_digest", "compatibility"} {
		if !containsJSONKey(stdout, required) {
			t.Fatalf("doctor output lacks %q check: %s", required, stdout)
		}
	}
	manifestPath := filepath.Join(home, ".codex", "skills", "viceme", ".viceme", "install-manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["cli_version"] = "9.9.9"
	data, _ = json.Marshal(manifest)
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr, _ = runCLIWithDependencies(t, nil, nil, "", Dependencies{Environment: environment},
		"skills", "doctor", "--target", "codex", "--json")
	if code != 5 || stdout != "" || !containsJSONKey(stderr, "skill_doctor_unhealthy") || !containsJSONKey(stderr, "installed Skill was written by a different CLI version") {
		t.Fatalf("doctor did not fail closed on version drift: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func containsJSONKey(document, value string) bool {
	return len(document) > 0 && stringContains(document, value)
}

func stringContains(document, value string) bool {
	for index := 0; index+len(value) <= len(document); index++ {
		if document[index:index+len(value)] == value {
			return true
		}
	}
	return false
}

type fakeUpdateService struct {
	check   updatepkg.CheckResult
	result  updatepkg.ApplyResult
	options updatepkg.ApplyOptions
	applied bool
}

func (service *fakeUpdateService) EnsureLauncher(context.Context) (updatepkg.TargetResult, error) {
	return updatepkg.TargetResult{Target: "npm_global", Status: "skipped"}, nil
}

func (service *fakeUpdateService) Check(context.Context) (updatepkg.CheckResult, error) {
	return service.check, nil
}

func (service *fakeUpdateService) Apply(_ context.Context, _ updatepkg.CheckResult, options updatepkg.ApplyOptions) (updatepkg.ApplyResult, error) {
	service.applied = true
	service.options = options
	return service.result, nil
}

func TestUpdateCommandChecksOrRefreshesPackageBinaryAndSkill(t *testing.T) {
	t.Parallel()
	service := &fakeUpdateService{
		check:  updatepkg.CheckResult{CurrentVersion: "0.1.0", AvailableVersion: "0.1.1", UpdateAvailable: true, Method: "npm"},
		result: updatepkg.ApplyResult{PreviousCLIVersion: "0.1.0", CLIVersion: "0.1.1"},
	}
	code, stdout, stderr, _ := runCLIWithDependencies(t, nil, nil, "", Dependencies{Updater: service}, "update", "--check", "--json")
	if code != 0 || stderr != "" || service.applied || !stringContains(stdout, `"update_available":true`) {
		t.Fatalf("check code=%d applied=%t stdout=%s stderr=%s", code, service.applied, stdout, stderr)
	}
	code, stdout, stderr, _ = runCLIWithDependencies(t, nil, nil, "", Dependencies{Updater: service}, "update", "--target", "claude", "--json")
	if code != 0 || stderr != "" || !service.applied || !service.options.RefreshSkills || service.options.SkillTarget != "claude" || !stringContains(stdout, `"cli_version":"0.1.1"`) {
		t.Fatalf("apply code=%d applied=%t options=%#v stdout=%s stderr=%s", code, service.applied, service.options, stdout, stderr)
	}
}
