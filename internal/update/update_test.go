package update

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestReleaseServiceDefaultBaseURLsUseStartBucket(t *testing.T) {
	t.Parallel()
	service := NewReleaseService("1.2.3", "1.2.3")
	if got := service.baseURL(); got != "https://s3.viceme.cn/start/cli/releases" {
		t.Fatalf("unexpected CN release base URL: %q", got)
	}
	service.SetRegion("global")
	if got := service.baseURL(); got != "https://s3.viceme.ai/start/cli/releases" {
		t.Fatalf("unexpected global release base URL: %q", got)
	}
}

func TestReleaseServiceAutomaticCheckUsesFreshRegionSpecificState(t *testing.T) {
	t.Setenv("CI", "")
	now := time.Date(2026, time.August, 13, 10, 0, 0, 0, time.UTC)
	var calls atomic.Int32
	latest := atomic.Value{}
	latest.Store("0.14.0")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/latest" {
			http.NotFound(writer, request)
			return
		}
		calls.Add(1)
		_, _ = writer.Write([]byte(latest.Load().(string)))
	}))
	defer server.Close()

	service := NewReleaseService("0.13.0", "0.13.0")
	service.ConfigDir = t.TempDir()
	service.ReleaseBaseURL = server.URL
	service.HTTPClient = server.Client()
	service.Now = func() time.Time { return now }

	first, err := service.CheckAutomatic(context.Background())
	if err != nil || !first.UpdateAvailable || first.AvailableVersion != "0.14.0" || first.Source != "official_s3" {
		t.Fatalf("initial automatic check=%#v err=%v", first, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("initial automatic check calls=%d", calls.Load())
	}
	cached, err := service.CheckAutomatic(context.Background())
	if err != nil || !cached.UpdateAvailable || cached.AvailableVersion != "0.14.0" || cached.Source != "cache" {
		t.Fatalf("cached automatic check=%#v err=%v", cached, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("fresh automatic state unexpectedly refreshed: calls=%d", calls.Load())
	}

	service.SetRegion("global")
	latest.Store("0.15.0")
	global, err := service.CheckAutomatic(context.Background())
	if err != nil || global.AvailableVersion != "0.15.0" || global.Source != "official_s3" {
		t.Fatalf("global automatic check=%#v err=%v", global, err)
	}
	if calls.Load() != 2 {
		t.Fatalf("global automatic check calls=%d", calls.Load())
	}

	service.SetRegion("cn")
	cnCached, err := service.CheckAutomatic(context.Background())
	if err != nil || cnCached.AvailableVersion != "0.14.0" || cnCached.Source != "cache" {
		t.Fatalf("CN cached automatic check=%#v err=%v", cnCached, err)
	}
	now = now.Add(updateCacheTTL + time.Second)
	latest.Store("0.14.1")
	refreshed, err := service.CheckAutomatic(context.Background())
	if err != nil || refreshed.AvailableVersion != "0.14.1" || refreshed.Source != "official_s3" {
		t.Fatalf("refreshed CN automatic check=%#v err=%v", refreshed, err)
	}
	if calls.Load() != 3 {
		t.Fatalf("stale CN automatic state refresh calls=%d", calls.Load())
	}
}

func TestReleaseServiceAutomaticCheckSkipsCI(t *testing.T) {
	t.Setenv("CI", "")
	service := NewReleaseService("0.13.0", "0.13.0")
	service.ConfigDir = t.TempDir()
	service.saveUpdateState("0.14.0")

	t.Setenv("CI", "true")
	if result, err := service.CheckAutomatic(context.Background()); err != nil || result.Source != "disabled" || result.UpdateAvailable {
		t.Fatalf("CI automatic check=%#v err=%v", result, err)
	}
}

func TestReleaseServiceChecksReplacesAndRefreshesMatchingSkills(t *testing.T) {
	t.Parallel()
	binary := []byte("new-viceme-binary")
	digest := sha256.Sum256(binary)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/latest":
			_, _ = writer.Write([]byte("1.2.3\n"))
		case "/v1.2.3/viceme_1.2.3_linux_amd64":
			_, _ = writer.Write(binary)
		case "/v1.2.3/viceme_1.2.3_linux_amd64.sha256":
			_, _ = fmt.Fprintf(writer, "%x  viceme_1.2.3_linux_amd64\n", digest)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	executable := filepath.Join(t.TempDir(), "viceme")
	if err := os.WriteFile(executable, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{hook: func(name string, args []string) error {
		if len(args) < 4 || args[0] != "bootstrap" || args[1] != "activate" || args[2] != "--destination" {
			return fmt.Errorf("unexpected bootstrap activation: %s %#v", name, args)
		}
		contents, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		return os.WriteFile(args[3], contents, 0o755)
	}}
	service := NewReleaseService("1.2.2", "1.2.2")
	service.ReleaseBaseURL = server.URL
	service.HTTPClient = server.Client()
	service.ExecutablePath = executable
	service.GOOS = "linux"
	service.GOARCH = "amd64"
	service.Runner = runner
	service.SetRegion("global")

	check, err := service.Check(context.Background())
	if err != nil || !check.UpdateAvailable || check.AvailableVersion != "1.2.3" {
		t.Fatalf("check=%#v err=%v", check, err)
	}
	result, err := service.Apply(context.Background(), check, ApplyOptions{RefreshSkills: true, SkillTarget: "workbuddy"})
	if err != nil || result.CLIVersion != "1.2.3" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	installed, err := os.ReadFile(executable)
	if err != nil || !reflect.DeepEqual(installed, binary) {
		t.Fatalf("installed=%q err=%v", installed, err)
	}
	if len(runner.calls) != 1 || !reflect.DeepEqual(runner.calls[0].args, []string{"bootstrap", "activate", "--destination", executable, "--agent", "workbuddy", "--region", "global"}) {
		t.Fatalf("matching Skill refresh did not use the activated new binary: %#v", runner.calls)
	}
}

func TestReleaseServiceRestoresCurrentBinaryWhenSkillActivationFails(t *testing.T) {
	t.Parallel()
	binary := []byte("new-viceme-binary")
	digest := sha256.Sum256(binary)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/latest":
			_, _ = writer.Write([]byte("1.2.3"))
		case "/v1.2.3/viceme_1.2.3_linux_amd64":
			_, _ = writer.Write(binary)
		case "/v1.2.3/viceme_1.2.3_linux_amd64.sha256":
			_, _ = fmt.Fprintf(writer, "%x", digest)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	executable := filepath.Join(t.TempDir(), "viceme")
	if err := os.WriteFile(executable, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	service := NewReleaseService("1.2.2", "1.2.2")
	service.ReleaseBaseURL = server.URL
	service.HTTPClient = server.Client()
	service.ExecutablePath = executable
	service.GOOS = "linux"
	service.GOARCH = "amd64"
	service.Runner = &fakeRunner{errors: []error{errors.New("doctor failed")}}
	check, err := service.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Apply(context.Background(), check, ApplyOptions{RefreshSkills: true}); ErrorKindOf(err) != ErrorReleaseSkillRefresh {
		t.Fatalf("unexpected error: %v", err)
	}
	installed, err := os.ReadFile(executable)
	if err != nil || string(installed) != "old" {
		t.Fatalf("previous binary was not restored: %q err=%v", installed, err)
	}
}

func TestReleaseServiceIntegrityFailurePreservesCurrentBinary(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/latest" {
			_, _ = writer.Write([]byte("1.2.3"))
			return
		}
		if strings.HasSuffix(request.URL.Path, ".sha256") {
			_, _ = writer.Write([]byte(strings.Repeat("0", 64)))
			return
		}
		_, _ = writer.Write([]byte("corrupt"))
	}))
	defer server.Close()
	executable := filepath.Join(t.TempDir(), "viceme")
	if err := os.WriteFile(executable, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	service := NewReleaseService("1.2.2", "1.2.2")
	service.ReleaseBaseURL = server.URL
	service.HTTPClient = server.Client()
	service.ExecutablePath = executable
	service.GOOS = "linux"
	service.GOARCH = "amd64"
	check, err := service.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Apply(context.Background(), check, ApplyOptions{})
	if ErrorKindOf(err) != ErrorReleaseIntegrity {
		t.Fatalf("unexpected error: %v", err)
	}
	installed, readErr := os.ReadFile(executable)
	if readErr != nil || string(installed) != "old" {
		t.Fatalf("current binary changed after integrity failure: %q err=%v", installed, readErr)
	}
}

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
	hook    func(string, []string) error
}

type blockingConcurrentRunner struct {
	mu           sync.Mutex
	calls        int
	firstStarted chan struct{}
	releaseFirst chan struct{}
	startOnce    sync.Once
}

func (runner *blockingConcurrentRunner) Run(context.Context, string, ...string) ([]byte, error) {
	runner.mu.Lock()
	runner.calls++
	call := runner.calls
	runner.mu.Unlock()
	if call == 1 {
		runner.startOnce.Do(func() { close(runner.firstStarted) })
		<-runner.releaseFirst
	}
	return nil, nil
}

func (runner *blockingConcurrentRunner) CallCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.calls
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
	if err == nil && runner.hook != nil {
		err = runner.hook(name, args)
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
	wantExecPrefix := []string{cacheArg, "exec", "--registry=https://registry.npmjs.org", "--@viceme-ai:registry=https://registry.npmjs.org", "--yes", "--package=@viceme-ai/cli@0.1.1", "--", "viceme", "install", "--agent", "codex", "--internal-skip-launcher-ensure"}
	gotExec := runner.calls[1].args
	if len(gotExec) != len(wantExecPrefix)+2 || !reflect.DeepEqual(gotExec[:len(wantExecPrefix)], wantExecPrefix) ||
		!strings.HasPrefix(gotExec[len(wantExecPrefix)], "--internal-activation-child=") ||
		gotExec[len(wantExecPrefix)+1] != "--internal-activation-target=0.1.1" {
		t.Fatalf("unexpected Skill refresh args: %#v", runner.calls[1])
	}
	if _, err := os.Stat(filepath.Join(configDir, updateStateFilename)); err != nil {
		t.Fatalf("version check did not persist update state: %v", err)
	}
}

func TestConcurrentNPMAutomaticUpdatesCoalesceIntoOneGenerationActivation(t *testing.T) {
	t.Parallel()
	configDir := t.TempDir()
	previous, err := NewNPMGeneration("0.15.2")
	if err != nil {
		t.Fatal(err)
	}
	if err := CommitActiveGeneration(configDir, previous); err != nil {
		t.Fatal(err)
	}
	runner := &blockingConcurrentRunner{
		firstStarted: make(chan struct{}),
		releaseFirst: make(chan struct{}),
	}
	newService := func() *NPMService {
		service := NewNPMService("0.15.2", "0.15.2", "npm")
		service.ConfigDir = configDir
		service.Runner = runner
		return service
	}
	check := CheckResult{CurrentVersion: "0.15.2", AvailableVersion: "0.16.0", UpdateAvailable: true}
	type outcome struct {
		result ApplyResult
		err    error
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	firstDone := make(chan outcome, 1)
	secondDone := make(chan outcome, 1)
	go func() {
		result, err := newService().Apply(ctx, check, ApplyOptions{RefreshSkills: true, SkillTarget: "auto"})
		firstDone <- outcome{result: result, err: err}
	}()
	<-runner.firstStarted
	go func() {
		result, err := newService().Apply(ctx, check, ApplyOptions{RefreshSkills: true, SkillTarget: "auto"})
		secondDone <- outcome{result: result, err: err}
	}()
	close(runner.releaseFirst)
	first := <-firstDone
	second := <-secondDone
	if first.err != nil || second.err != nil || first.result.CLIVersion != "0.16.0" || second.result.CLIVersion != "0.16.0" {
		t.Fatalf("concurrent outcomes: first=%#v second=%#v", first, second)
	}
	if runner.CallCount() != 2 {
		t.Fatalf("concurrent update repeated the npm and Skill activation: calls=%d", runner.CallCount())
	}
	active, exists, err := ReadActiveGeneration(configDir)
	if err != nil || !exists || active.Version != "0.16.0" {
		t.Fatalf("concurrent update did not commit one target generation: active=%#v exists=%t err=%v", active, exists, err)
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

func TestNPMServiceAutomaticCheckUsesFreshValidatedState(t *testing.T) {
	t.Setenv("CI", "")
	now := time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC)
	var calls atomic.Int32
	latest := atomic.Value{}
	latest.Store("0.8.3")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = writer.Write([]byte(`{"version":"` + latest.Load().(string) + `"}`))
	}))
	defer server.Close()

	service := NewNPMService("0.8.2", "0.8.2", "npm")
	service.ConfigDir = t.TempDir()
	service.RegistryEndpoint = server.URL
	service.HTTPClient = server.Client()
	service.Now = func() time.Time { return now }

	first, err := service.CheckAutomatic(context.Background())
	if err != nil || !first.UpdateAvailable || first.AvailableVersion != "0.8.3" || first.Source != "registry" {
		t.Fatalf("initial automatic check=%#v err=%v", first, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("initial automatic check calls=%d", calls.Load())
	}
	cached, err := service.CheckAutomatic(context.Background())
	if err != nil || cached.AvailableVersion != "0.8.3" || cached.Source != "cache" {
		t.Fatalf("cached automatic check=%#v err=%v", cached, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("fresh automatic state unexpectedly refreshed: calls=%d", calls.Load())
	}

	now = now.Add(updateCacheTTL + time.Second)
	latest.Store("0.8.4")
	refreshed, err := service.CheckAutomatic(context.Background())
	if err != nil || refreshed.AvailableVersion != "0.8.4" || refreshed.Source != "registry" {
		t.Fatalf("refreshed automatic check=%#v err=%v", refreshed, err)
	}
	if calls.Load() != 2 {
		t.Fatalf("stale automatic state refresh calls=%d", calls.Load())
	}
}

func TestNPMServiceAutomaticCheckSkipsNonNPMAndCI(t *testing.T) {
	t.Setenv("CI", "")
	service := NewNPMService("0.8.2", "0.8.2", "standalone")
	service.ConfigDir = t.TempDir()
	service.saveUpdateState("0.8.3")
	if result, err := service.CheckAutomatic(context.Background()); err != nil || result.Source != "disabled" || result.UpdateAvailable {
		t.Fatalf("non-npm automatic check=%#v err=%v", result, err)
	}

	service.InstallMethod = "npm"
	t.Setenv("CI", "true")
	if result, err := service.CheckAutomatic(context.Background()); err != nil || result.Source != "disabled" || result.UpdateAvailable {
		t.Fatalf("CI automatic check=%#v err=%v", result, err)
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
	runner := &fakeRunner{errors: []error{nil, errors.New("refresh failed")}}
	service := NewNPMService("0.1.0", "0.1.0", "npm")
	service.ConfigDir = t.TempDir()
	service.Runner = runner
	result, err := service.Apply(context.Background(), CheckResult{AvailableVersion: "0.1.0"}, ApplyOptions{RefreshSkills: true})
	if err == nil || len(result.Targets) != 2 || result.Targets[1].Status != "recovery_pending" {
		t.Fatalf("expected typed partial result, result=%#v err=%v", result, err)
	}
	if _, statErr := os.Stat(filepath.Join(service.ConfigDir, npmActivationFilename)); statErr != nil {
		t.Fatalf("failed activation did not retain a recovery journal: %v", statErr)
	}
}

func TestNPMServiceRollsForwardExactTargetAfterSkillRefreshFailure(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{errors: []error{nil, errors.New("refresh failed"), nil, nil, nil, nil}}
	service := NewNPMService("0.1.0", "0.1.0", "npm")
	service.ConfigDir = t.TempDir()
	service.Runner = runner
	result, err := service.Apply(
		context.Background(),
		CheckResult{AvailableVersion: "0.1.1", UpdateAvailable: true},
		ApplyOptions{RefreshSkills: true, SkillTarget: "codex"},
	)
	if err == nil {
		t.Fatal("failed Skill refresh unexpectedly committed the npm update")
	}
	if result.CLIVersion != "0.1.0" || len(result.Targets) != 2 || result.Targets[0].Status != "recovery_pending" {
		t.Fatalf("interrupted activation was not reported as recoverable: %#v", result)
	}
	result, err = service.Apply(
		context.Background(),
		CheckResult{AvailableVersion: "0.1.1", UpdateAvailable: true},
		ApplyOptions{RefreshSkills: true, SkillTarget: "codex"},
	)
	if err != nil || result.CLIVersion != "0.1.1" {
		t.Fatalf("retry did not recover and finish the exact target: result=%#v err=%v", result, err)
	}
	if len(runner.calls) != 4 {
		t.Fatalf("unexpected npm call count: %#v", runner.calls)
	}
	if got := runner.calls[0].args[len(runner.calls[0].args)-1]; got != "@viceme-ai/cli@0.1.1" {
		t.Fatalf("update did not install exact target version: %q", got)
	}
	if got := runner.calls[2].args[len(runner.calls[2].args)-1]; got != "@viceme-ai/cli@0.1.1" {
		t.Fatalf("recovery did not roll forward the exact target version: %q", got)
	}
}

func TestNPMServiceNeverDowngradesSkillWhenRegistryLatestIsOlder(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{}
	service := NewNPMService("0.2.0", "0.2.0", "npm")
	service.ConfigDir = t.TempDir()
	service.Runner = runner
	_, err := service.Apply(context.Background(), CheckResult{AvailableVersion: "0.1.9", UpdateAvailable: false}, ApplyOptions{RefreshSkills: true})
	if err != nil {
		t.Fatal(err)
	}
	want := "--package=@viceme-ai/cli@0.2.0"
	if len(runner.calls) != 2 || !slices.Contains(runner.calls[1].args, want) {
		t.Fatalf("update selected a downgrade package: %#v", runner.calls)
	}
}
