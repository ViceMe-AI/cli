package update

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type runCall struct {
	name string
	args []string
}

type fakeRunner struct {
	outputs [][]byte
	errors  []error
	calls   []runCall
}

func (runner *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	runner.calls = append(runner.calls, runCall{name: name, args: append([]string(nil), args...)})
	index := len(runner.calls) - 1
	var output []byte
	var err error
	if index < len(runner.outputs) {
		output = runner.outputs[index]
	}
	if index < len(runner.errors) {
		err = runner.errors[index]
	}
	return output, err
}

func TestNPMServiceChecksAndAppliesExactVersion(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{outputs: [][]byte{[]byte(`"0.1.1"`), nil, []byte(`{"ok":true}`)}}
	service := NewNPMService("0.1.0", "0.1.0", "npm")
	service.Runner = runner
	check, err := service.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !check.UpdateAvailable || check.AvailableVersion != "0.1.1" {
		t.Fatalf("unexpected check: %#v", check)
	}
	result, err := service.Apply(context.Background(), check, ApplyOptions{RefreshSkills: true, SkillTarget: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if result.CLIVersion != "0.1.1" || len(result.Targets) != 2 {
		t.Fatalf("unexpected apply result: %#v", result)
	}
	wantCheck := []string{"view", "@viceme-ai/cli", "version", "--json", "--registry=https://registry.npmjs.org", "--@viceme-ai:registry=https://registry.npmjs.org"}
	if !reflect.DeepEqual(runner.calls[0].args, wantCheck) {
		t.Fatalf("npm check did not pin the canonical registry: %#v", runner.calls[0])
	}
	wantInstall := []string{"install", "--registry=https://registry.npmjs.org", "--@viceme-ai:registry=https://registry.npmjs.org", "--global", "--ignore-scripts", "--no-audit", "--no-fund", "@viceme-ai/cli@0.1.1"}
	if !reflect.DeepEqual(runner.calls[1].args, wantInstall) {
		t.Fatalf("unsafe or inexact npm install args: %#v", runner.calls[1])
	}
	wantExec := []string{"exec", "--registry=https://registry.npmjs.org", "--@viceme-ai:registry=https://registry.npmjs.org", "--yes", "--package=@viceme-ai/cli@0.1.1", "--", "viceme", "skills", "install", "--target", "codex", "--json"}
	if !reflect.DeepEqual(runner.calls[2].args, wantExec) {
		t.Fatalf("unexpected Skill refresh args: %#v", runner.calls[2])
	}
}

func TestNPMServiceBootstrapInstallsPersistentExactLauncher(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{}
	service := NewNPMService("0.1.0", "0.1.0", "npm")
	service.Runner = runner
	result, err := service.EnsureLauncher(context.Background())
	if err != nil || result.Status != "updated" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	want := []string{"install", "--registry=https://registry.npmjs.org", "--@viceme-ai:registry=https://registry.npmjs.org", "--global", "--ignore-scripts", "--no-audit", "--no-fund", "@viceme-ai/cli@0.1.0"}
	if len(runner.calls) != 1 || !reflect.DeepEqual(runner.calls[0].args, want) {
		t.Fatalf("bootstrap did not install exact persistent launcher: %#v", runner.calls)
	}
}

func TestNPMServiceRefusesMutationOutsideNPMLauncher(t *testing.T) {
	t.Parallel()
	service := NewNPMService("0.1.0", "0.1.0", "standalone")
	service.Runner = &fakeRunner{}
	_, err := service.Apply(context.Background(), CheckResult{AvailableVersion: "0.1.1", UpdateAvailable: true}, ApplyOptions{})
	if !errors.Is(err, ErrNPMInstallRequired) {
		t.Fatalf("expected npm install boundary, got %v", err)
	}
}

func TestNPMServiceReturnsPartialResultWhenSkillRefreshFails(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{errors: []error{errors.New("refresh failed")}}
	service := NewNPMService("0.1.0", "0.1.0", "npm")
	service.Runner = runner
	result, err := service.Apply(context.Background(), CheckResult{AvailableVersion: "0.1.0"}, ApplyOptions{RefreshSkills: true})
	if err == nil || len(result.Targets) != 2 || result.Targets[1].Status != "failed" {
		t.Fatalf("expected typed partial result, result=%#v err=%v", result, err)
	}
}

func TestNPMServiceNeverDowngradesSkillWhenRegistryLatestIsOlder(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{}
	service := NewNPMService("0.2.0", "0.2.0", "npm")
	service.Runner = runner
	_, err := service.Apply(context.Background(), CheckResult{AvailableVersion: "0.1.9", UpdateAvailable: false}, ApplyOptions{RefreshSkills: true})
	if err != nil {
		t.Fatal(err)
	}
	want := "--package=@viceme-ai/cli@0.2.0"
	if len(runner.calls) != 1 || len(runner.calls[0].args) < 5 || runner.calls[0].args[4] != want {
		t.Fatalf("update selected a downgrade package: %#v", runner.calls)
	}
}
