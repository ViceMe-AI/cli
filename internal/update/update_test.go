package update

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestUpdateChildProcessDoesNotInheritPublicationCredential(t *testing.T) {
	environment := []string{
		"PATH=/usr/bin",
		"VICEME_ACCESS_TOKEN=must-not-cross-exec",
		"VICEME_API_BASE_URL=https://api.viceme.ai",
	}
	filtered := withoutEnvironmentVariable(environment, "VICEME_ACCESS_TOKEN")
	joined := strings.Join(filtered, "\n")
	if strings.Contains(joined, "VICEME_ACCESS_TOKEN=") {
		t.Fatalf("process credential remained in child environment: %q", joined)
	}
	if !strings.Contains(joined, "PATH=/usr/bin") || !strings.Contains(joined, "VICEME_API_BASE_URL=https://api.viceme.ai") {
		t.Fatalf("unrelated environment was removed: %q", joined)
	}
}

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
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"version":"0.1.1"}`))
	}))
	defer server.Close()
	configDir := t.TempDir()
	runner := &fakeRunner{outputs: [][]byte{nil, []byte(`{"ok":true}`)}}
	service := NewNPMService("0.1.0", "0.1.0", "npm")
	service.ConfigDir = configDir
	service.RegistryEndpoint = server.URL
	service.HTTPClient = server.Client()
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
	if check.Source != "registry" || len(runner.calls) != 2 {
		t.Fatalf("version check unexpectedly invoked npm or lost its source: check=%#v calls=%#v", check, runner.calls)
	}
	cacheArg := "--cache=" + filepath.Join(configDir, npmCacheDirectory)
	wantInstall := []string{cacheArg, "install", "--registry=https://registry.npmjs.org", "--@viceme-ai:registry=https://registry.npmjs.org", "--global", "--ignore-scripts", "--no-audit", "--no-fund", "@viceme-ai/cli@0.1.1"}
	if !reflect.DeepEqual(runner.calls[0].args, wantInstall) {
		t.Fatalf("unsafe or inexact npm install args: %#v", runner.calls[0])
	}
	wantExec := []string{cacheArg, "exec", "--registry=https://registry.npmjs.org", "--@viceme-ai:registry=https://registry.npmjs.org", "--yes", "--package=@viceme-ai/cli@0.1.1", "--", "viceme", "skills", "install", "--target", "codex"}
	if !reflect.DeepEqual(runner.calls[1].args, wantExec) {
		t.Fatalf("unexpected Skill refresh args: %#v", runner.calls[1])
	}
	if _, err := os.Stat(filepath.Join(configDir, updateStateFilename)); err != nil {
		t.Fatalf("version check did not persist update state: %v", err)
	}
}

func TestNPMServiceBootstrapInstallsPersistentExactLauncher(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{}
	service := NewNPMService("0.1.0", "0.1.0", "npm")
	service.ConfigDir = t.TempDir()
	service.Runner = runner
	result, err := service.EnsureLauncher(context.Background())
	if err != nil || result.Status != "updated" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	want := []string{"--cache=" + filepath.Join(service.ConfigDir, npmCacheDirectory), "install", "--registry=https://registry.npmjs.org", "--@viceme-ai:registry=https://registry.npmjs.org", "--global", "--ignore-scripts", "--no-audit", "--no-fund", "@viceme-ai/cli@0.1.0"}
	if len(runner.calls) != 1 || !reflect.DeepEqual(runner.calls[0].args, want) {
		t.Fatalf("bootstrap did not install exact persistent launcher: %#v", runner.calls)
	}
}

func TestNPMServiceUsesOnlyFreshCacheWhenRegistryIsUnavailable(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	var failing atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if failing.Load() {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = writer.Write([]byte(`{"version":"0.3.0"}`))
	}))
	defer server.Close()
	service := NewNPMService("0.2.1", "0.2.1", "npm")
	service.ConfigDir = t.TempDir()
	service.RegistryEndpoint = server.URL
	service.HTTPClient = server.Client()
	service.Now = func() time.Time { return now }
	first, err := service.Check(context.Background())
	if err != nil || first.Source != "registry" {
		t.Fatalf("first check=%#v err=%v", first, err)
	}
	failing.Store(true)
	cached, err := service.Check(context.Background())
	if err != nil || cached.Source != "cache" || cached.AvailableVersion != "0.3.0" {
		t.Fatalf("cached check=%#v err=%v", cached, err)
	}
	now = now.Add(updateCacheTTL + time.Second)
	_, err = service.Check(context.Background())
	if ErrorKindOf(err) != ErrorRegistryNetwork {
		t.Fatalf("expired cache masked registry failure: %v", err)
	}
}

func TestNPMServiceClassifiesPermissionFailureWithoutLeakingOutput(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{
		outputs: [][]byte{[]byte("npm error code EPERM token=top-secret")},
		errors:  []error{errors.New("exit status 1")},
	}
	service := NewNPMService("0.2.1", "0.2.1", "npm")
	service.ConfigDir = t.TempDir()
	service.Runner = runner
	_, err := service.Apply(context.Background(), CheckResult{AvailableVersion: "0.3.0", UpdateAvailable: true}, ApplyOptions{})
	if ErrorKindOf(err) != ErrorNPMPermission {
		t.Fatalf("permission failure kind=%q err=%v", ErrorKindOf(err), err)
	}
	if strings.Contains(err.Error(), "top-secret") {
		t.Fatal("npm output leaked through the stable error")
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
