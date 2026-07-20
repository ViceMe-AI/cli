package command

import (
	"context"
	"encoding/json"
	"errors"
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
		"install", "--target", "codex", "--region", "global")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	var envelope struct {
		Data struct {
			Region   string `json:"region"`
			NextStep struct {
				Command string `json:"command"`
			} `json:"next_step"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.NextStep.Command != "viceme auth login --no-wait" {
		t.Fatalf("bootstrap did not return device-login next step: %s", stdout)
	}
	if envelope.Data.Region != "global" {
		t.Fatalf("bootstrap did not select the global region: %s", stdout)
	}
	for _, filename := range []string{
		filepath.Join(home, ".codex", "skills", "viceme", "SKILL.md"),
		filepath.Join(home, ".codex", "skills", "viceme", ".viceme", "install-manifest.json"),
		filepath.Join(configDirectory, "config.json"),
	} {
		if _, err := os.Stat(filename); err != nil {
			t.Fatalf("bootstrap did not create %s: %v", filename, err)
		}
	}
	configData, err := os.ReadFile(filepath.Join(configDirectory, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"currentProfile": "default"`, `"name": "default"`, `"region": "global"`} {
		if !stringContains(string(configData), expected) {
			t.Fatalf("config lacks %s: %s", expected, configData)
		}
	}
}

func TestSkillsDoctorReportsEveryReleaseContractAndFailsOnVersionDrift(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	environment := skillcontent.Environment{Home: home}
	code, _, stderr, _ := runCLIWithDependencies(t, nil, nil, "", Dependencies{Environment: environment},
		"skills", "install", "--target", "codex")
	if code != 0 || stderr != "" {
		t.Fatalf("install code=%d stderr=%s", code, stderr)
	}
	code, stdout, stderr, _ := runCLIWithDependencies(t, nil, nil, "", Dependencies{Environment: environment},
		"skills", "doctor", "--target", "codex")
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
		"skills", "doctor", "--target", "codex")
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
	check    updatepkg.CheckResult
	checkErr error
	result   updatepkg.ApplyResult
	applyErr error
	options  updatepkg.ApplyOptions
	applied  bool
}

func (service *fakeUpdateService) EnsureLauncher(context.Context) (updatepkg.TargetResult, error) {
	return updatepkg.TargetResult{Target: "npm_global", Status: "skipped"}, nil
}

func (service *fakeUpdateService) Check(context.Context) (updatepkg.CheckResult, error) {
	return service.check, service.checkErr
}

func (service *fakeUpdateService) Apply(_ context.Context, _ updatepkg.CheckResult, options updatepkg.ApplyOptions) (updatepkg.ApplyResult, error) {
	service.applied = true
	service.options = options
	return service.result, service.applyErr
}

func TestUpdateCommandReturnsSafeActionableFailures(t *testing.T) {
	t.Parallel()
	registryFailure := &fakeUpdateService{
		checkErr: &updatepkg.OperationError{Kind: updatepkg.ErrorRegistryNetwork, Cause: errors.New("dial failed")},
	}
	code, _, stderr, _ := runCLIWithDependencies(t, nil, nil, "", Dependencies{Updater: registryFailure}, "update", "--check")
	if code != 4 || !stringContains(stderr, `"subtype":"update_registry_unavailable"`) || !stringContains(stderr, `"retryable":true`) {
		t.Fatalf("registry error was not classified safely: code=%d stderr=%s", code, stderr)
	}
	permissionFailure := &fakeUpdateService{
		check:    updatepkg.CheckResult{CurrentVersion: "0.2.1", AvailableVersion: "0.3.0", UpdateAvailable: true, Method: "npm", Source: "registry"},
		result:   updatepkg.ApplyResult{PreviousCLIVersion: "0.2.1", CLIVersion: "0.2.1"},
		applyErr: &updatepkg.OperationError{Kind: updatepkg.ErrorNPMPermission, Cause: errors.New("safe permission failure")},
	}
	code, _, stderr, _ = runCLIWithDependencies(t, nil, nil, "", Dependencies{Updater: permissionFailure}, "update")
	if code != 5 || !stringContains(stderr, `"subtype":"update_npm_permission"`) || !stringContains(stderr, `"hint":"ensure ~/.viceme-cli`) {
		t.Fatalf("npm permission error was not actionable: code=%d stderr=%s", code, stderr)
	}
}

func TestUpdateCommandChecksOrRefreshesPackageBinaryAndSkill(t *testing.T) {
	t.Parallel()
	service := &fakeUpdateService{
		check:  updatepkg.CheckResult{CurrentVersion: "0.1.0", AvailableVersion: "0.1.1", UpdateAvailable: true, Method: "npm"},
		result: updatepkg.ApplyResult{PreviousCLIVersion: "0.1.0", CLIVersion: "0.1.1"},
	}
	code, stdout, stderr, _ := runCLIWithDependencies(t, nil, nil, "", Dependencies{Updater: service}, "update", "--check")
	if code != 0 || stderr != "" || service.applied || !stringContains(stdout, `"update_available":true`) {
		t.Fatalf("check code=%d applied=%t stdout=%s stderr=%s", code, service.applied, stdout, stderr)
	}
	code, stdout, stderr, _ = runCLIWithDependencies(t, nil, nil, "", Dependencies{Updater: service}, "update", "--target", "claude")
	if code != 0 || stderr != "" || !service.applied || !service.options.RefreshSkills || service.options.SkillTarget != "claude" || !stringContains(stdout, `"cli_version":"0.1.1"`) {
		t.Fatalf("apply code=%d applied=%t options=%#v stdout=%s stderr=%s", code, service.applied, service.options, stdout, stderr)
	}
}
