package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	cliembed "github.com/ViceMe-AI/cli"
	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	updatepkg "github.com/ViceMe-AI/cli/internal/update"
	"github.com/gofrs/flock"
)

type failingEntropyReader struct{}

type unavailableCredentialStore struct{}

func (unavailableCredentialStore) Get(string) (string, error) {
	return "", errors.New("credential backend unavailable")
}

func (unavailableCredentialStore) Set(string, string) error {
	return errors.New("credential backend unavailable")
}

func (unavailableCredentialStore) Delete(string) error {
	return errors.New("credential backend unavailable")
}

type startupRecoveryUpdater struct {
	called atomic.Bool
	err    error
}

type commandNPMRunner struct {
	errors []error
	calls  int
}

func (runner *commandNPMRunner) Run(context.Context, string, ...string) ([]byte, error) {
	runner.calls++
	if len(runner.errors) == 0 {
		return nil, nil
	}
	err := runner.errors[0]
	runner.errors = runner.errors[1:]
	return nil, err
}

func (updater *startupRecoveryUpdater) RecoverActivationWhileLocked(context.Context) error {
	updater.called.Store(true)
	return updater.err
}

func (*startupRecoveryUpdater) EnsureLauncher(context.Context) (updatepkg.TargetResult, error) {
	return updatepkg.TargetResult{}, nil
}

func (*startupRecoveryUpdater) Check(context.Context) (updatepkg.CheckResult, error) {
	return updatepkg.CheckResult{}, nil
}

func (*startupRecoveryUpdater) Apply(context.Context, updatepkg.CheckResult, updatepkg.ApplyOptions) (updatepkg.ApplyResult, error) {
	return updatepkg.ApplyResult{}, nil
}

func (failingEntropyReader) Read([]byte) (int, error) {
	return 0, errors.New("entropy unavailable")
}

func TestUUIDFallbackStillProducesAValidVersionFourUUID(t *testing.T) {
	t.Parallel()
	value := uuidFromEntropy(failingEntropyReader{}, time.Unix(1_700_000_000, 42), 123, 7)
	pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !pattern.MatchString(value) {
		t.Fatalf("fallback returned an invalid UUID: %q", value)
	}
}

func TestBareCommandKeepsMachineOutputAsJSON(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := Execute(nil, Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: securestore.NewMemory(),
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:      config.RegionCN,
	})
	if exit != 0 {
		t.Fatalf("bare command failed: exit=%d stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("bare command polluted machine output: result=%#v err=%v stdout=%q", result, err, stdout.String())
	}
	data, ok := result["data"].(map[string]any)
	if result["ok"] != true || !ok || data["command"] != "viceme" {
		t.Fatalf("bare command returned an unexpected envelope: %#v", result)
	}
	if !strings.Contains(stderr.String(), "Usage:") {
		t.Fatalf("human help was not isolated on stderr: %q", stderr.String())
	}
}

func TestOrdinaryCommandRunsOuterActivationRecoveryFirst(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	updater := &startupRecoveryUpdater{}
	var stdout bytes.Buffer
	exit := Execute([]string{"version"}, Dependencies{
		Out: &stdout, Store: securestore.NewMemory(), Updater: updater,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:      config.RegionCN,
	})
	if exit != 0 || !updater.called.Load() {
		t.Fatalf("ordinary command ran without startup recovery: exit=%d called=%t stdout=%q", exit, updater.called.Load(), stdout.String())
	}
}

func TestOrdinaryCommandRecoversNPMGenerationBeforeOutput(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	previous, err := updatepkg.NewNPMGeneration("0.10.0")
	if err != nil {
		t.Fatal(err)
	}
	if err := updatepkg.CommitActiveGeneration(configDir, previous); err != nil {
		t.Fatal(err)
	}
	runner := &commandNPMRunner{errors: []error{nil, errors.New("killed before Skill child started")}}
	oldUpdater := updatepkg.NewNPMService("0.10.0", "0.10.0", "npm")
	oldUpdater.ConfigDir = configDir
	oldUpdater.Runner = runner
	if _, err := oldUpdater.Apply(
		context.Background(),
		updatepkg.CheckResult{AvailableVersion: "0.10.1", UpdateAvailable: true},
		updatepkg.ApplyOptions{RefreshSkills: true, SkillTarget: "agents"},
	); err == nil {
		t.Fatal("interrupted npm activation unexpectedly committed")
	}
	updater := updatepkg.NewNPMService("0.10.1", "0.10.1", "npm")
	updater.ConfigDir = configDir
	updater.Runner = runner
	var stdout bytes.Buffer
	exit := Execute([]string{"version"}, Dependencies{
		Out: &stdout, Store: securestore.NewMemory(), Updater: updater,
		Environment: skillcontent.Environment{Home: root, ConfigDir: configDir},
		Region:      config.RegionCN,
	})
	if exit != 0 {
		t.Fatalf("ordinary command did not recover interrupted npm generation: exit=%d stdout=%q", exit, stdout.String())
	}
	active, exists, err := updatepkg.ReadActiveGeneration(configDir)
	if err != nil || !exists || active.Version != "0.10.1" || active.InstallMethod != "npm" {
		t.Fatalf("ordinary command ran before target generation committed: active=%#v exists=%t err=%v", active, exists, err)
	}
	if runner.calls != 4 {
		t.Fatalf("ordinary startup did not finish launcher and Skill recovery before output: calls=%d", runner.calls)
	}
}

func TestFailedOuterActivationRecoveryBlocksBusinessCommand(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	updater := &startupRecoveryUpdater{err: errors.New("recovery failed")}
	var stdout bytes.Buffer
	exit := Execute([]string{"version"}, Dependencies{
		Out: &stdout, Store: securestore.NewMemory(), Updater: updater,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:      config.RegionCN,
	})
	if exit == 0 || !updater.called.Load() || !strings.Contains(stdout.String(), "ACTIVATION_RECOVERY_FAILED") {
		t.Fatalf("business command was not blocked by recovery failure: exit=%d called=%t stdout=%q", exit, updater.called.Load(), stdout.String())
	}
}

func TestStandaloneEntryRecoversPendingNPMJournalThenRequiresRestart(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	previous, err := updatepkg.NewNPMGeneration("0.10.0")
	if err != nil {
		t.Fatal(err)
	}
	if err := updatepkg.CommitActiveGeneration(configDir, previous); err != nil {
		t.Fatal(err)
	}
	target, err := updatepkg.NewNPMGeneration(buildinfo.CompatibilityVersion())
	if err != nil {
		t.Fatal(err)
	}
	npmRunner := &commandNPMRunner{}
	npmRecoverer := updatepkg.NewNPMService(buildinfo.Version, buildinfo.CompatibilityVersion(), "npm")
	npmRecoverer.ConfigDir = configDir
	npmRecoverer.Runner = npmRunner
	journal, err := json.Marshal(map[string]any{
		"schemaVersion": 3,
		"status":        "COMMITTING",
		"nonce":         strings.Repeat("b", 64),
		"targetVersion": target.Version,
		"target":        target,
		"previous":      previous,
		"skillTarget":   "agents",
		"refreshSkills": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "npm-activation.json"), journal, 0o600); err != nil {
		t.Fatal(err)
	}
	release := updatepkg.NewReleaseService(buildinfo.Version, buildinfo.CompatibilityVersion())
	dependencies := Dependencies{
		Updater: release,
		Environment: skillcontent.Environment{
			Home: root, ConfigDir: configDir,
		},
		activationNPMRecoverer: npmRecoverer,
	}
	if err := reconcileActivationAtStartup(context.Background(), configDir, &dependencies); !errors.Is(err, updatepkg.ErrActivationRestartNeeded) {
		t.Fatalf("standalone entry continued after npm recovery: %v", err)
	}
	if npmRunner.calls != 2 {
		t.Fatalf("standalone entry did not recover npm launcher and Skills: calls=%d", npmRunner.calls)
	}
	active, exists, err := updatepkg.ReadActiveGeneration(configDir)
	if err != nil || !exists || active != target {
		t.Fatalf("npm target was not recovered: active=%#v exists=%t err=%v", active, exists, err)
	}
}

func TestNPMEntryCannotRunAgainstStandaloneActiveGeneration(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	active, err := updatepkg.NewStandaloneGeneration(buildinfo.CompatibilityVersion(), strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	if err := updatepkg.CommitActiveGeneration(configDir, active); err != nil {
		t.Fatal(err)
	}
	updater := updatepkg.NewNPMService(buildinfo.Version, buildinfo.CompatibilityVersion(), "npm")
	updater.ConfigDir = configDir
	dependencies := Dependencies{
		Updater: updater,
		Environment: skillcontent.Environment{
			Home: root, ConfigDir: configDir,
		},
	}
	if err := reconcileActivationAtStartup(context.Background(), configDir, &dependencies); !errors.Is(err, updatepkg.ErrActivationRestartNeeded) {
		t.Fatalf("npm entry continued against standalone active generation: %v", err)
	}
}

func TestBareInternalActivationFlagsCannotBypassRecovery(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	var stdout bytes.Buffer
	exit := Execute([]string{
		"install",
		"--agent", "codex",
		"--internal-skip-launcher-ensure",
		"--internal-activation-child=" + strings.Repeat("a", 64),
		"--internal-activation-target=" + buildinfo.CompatibilityVersion(),
	}, Dependencies{
		Out: &stdout, Store: securestore.NewMemory(),
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:      config.RegionCN,
	})
	if exit == 0 || !strings.Contains(stdout.String(), "ACTIVATION_CHILD_INVALID") {
		t.Fatalf("bare hidden flags bypassed the activation coordinator: exit=%d stdout=%q", exit, stdout.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", "skills", "viceme-publish")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unauthorized activation child mutated Skills: %v", err)
	}
}

func TestCoordinatedNPMChildRequiresCompleteBoundArguments(t *testing.T) {
	t.Parallel()
	request, err := parseNPMActivationChild([]string{
		"install",
		"--agent", "codex",
		"--internal-skip-launcher-ensure",
		"--internal-activation-child=" + strings.Repeat("a", 64),
		"--internal-activation-target=0.10.1",
	})
	if err != nil || !request.Requested || !request.SkipLauncher || request.SkillTarget != "codex" || request.TargetVersion != "0.10.1" {
		t.Fatalf("coordinated npm child was not parsed: request=%#v err=%v", request, err)
	}
	for _, args := range [][]string{
		{"version", "--internal-activation-child=fake", "--internal-activation-target=0.10.1"},
		{"install", "--internal-skip-launcher-ensure"},
		{"install", "--internal-activation-child=fake"},
	} {
		if _, err := parseNPMActivationChild(args); err == nil {
			t.Fatalf("incomplete internal flags were accepted: %#v", args)
		}
	}
}

func TestOrdinaryInstallCannotCommitAfterItsRunningGenerationWasReplaced(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	running, err := updatepkg.NewStandaloneGeneration("1.2.3", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	active, err := updatepkg.NewStandaloneGeneration("1.2.4", strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	if err := updatepkg.CommitActiveGeneration(configDir, active); err != nil {
		t.Fatal(err)
	}
	runtime := &Runtime{
		configBase: configDir,
		deps: Dependencies{
			Environment:                 skillcontent.Environment{Home: root, ConfigDir: configDir},
			runningActivationGeneration: &running,
		},
	}

	_, err = performOrdinaryInstall(context.Background(), runtime, "agents", "cn")
	if cliError := output.AsError(err); cliError.Subtype != "INSTALL_GENERATION_CHANGED" {
		t.Fatalf("stale install was not fenced by its captured generation: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", "skills", "viceme-publish")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale install mutated Skills after a newer generation committed: %v", err)
	}
	actual, exists, err := updatepkg.ReadActiveGeneration(configDir)
	if err != nil || !exists || actual != active {
		t.Fatalf("stale install changed the active generation: active=%#v exists=%t err=%v", actual, exists, err)
	}
}

func TestStartupRecoveryDoesNotPassAnActivationChildStillCommitting(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	member := flock.New(filepath.Join(configDir, updatepkg.ActivationMemberLockFilename))
	locked, err := member.TryLock()
	if err != nil || !locked {
		t.Fatalf("could not establish the child commit barrier: locked=%t err=%v", locked, err)
	}
	defer member.Unlock()
	updater := &startupRecoveryUpdater{}
	dependencies := Dependencies{
		Updater: updater,
		Environment: skillcontent.Environment{
			Home: root, ConfigDir: configDir,
		},
	}

	err = reconcileActivationAtStartup(context.Background(), configDir, &dependencies)
	if err == nil || !strings.Contains(err.Error(), "activation child") {
		t.Fatalf("startup recovery crossed an in-flight child commit: %v", err)
	}
	if updater.called.Load() {
		t.Fatal("outer recovery mutated generation state while the child still held commit authority")
	}
}

func TestStaleNPMChildRevalidatesItsJournalBeforeInstallingSkills(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	stale, err := updatepkg.NewNPMGeneration("1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	active, err := updatepkg.NewNPMGeneration("1.2.4")
	if err != nil {
		t.Fatal(err)
	}
	if err := updatepkg.CommitActiveGeneration(configDir, active); err != nil {
		t.Fatal(err)
	}
	journal, err := json.Marshal(map[string]any{
		"schemaVersion": 3,
		"status":        "COMMITTING",
		"nonce":         strings.Repeat("c", 64),
		"targetVersion": active.Version,
		"target":        active,
		"skillTarget":   "agents",
		"refreshSkills": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "npm-activation.json"), journal, 0o600); err != nil {
		t.Fatal(err)
	}
	runtime := &Runtime{
		configBase: configDir,
		deps: Dependencies{
			Environment:                 skillcontent.Environment{Home: root, ConfigDir: configDir},
			coordinatedActivationChild:  true,
			activationChildRequest:      npmActivationChildRequest{Nonce: strings.Repeat("b", 64), TargetVersion: stale.Version, SkillTarget: "agents"},
			runningActivationGeneration: &stale,
		},
	}

	_, err = performNPMChildInstall(context.Background(), runtime, "agents", "cn")
	if cliError := output.AsError(err); cliError.Subtype != "ACTIVATION_CHILD_INVALID" {
		t.Fatalf("stale child was not fenced by the replacement parent journal: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", "skills", "viceme-publish")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale npm child mutated Skills: %v", err)
	}
}

func TestDoctorIncludesUnauthenticatedNetworkReadiness(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	environment := skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")}
	skills := skillcontent.New(cliembed.EmbeddedSkills())
	reports := skills.InstallSet(officialSkillNames, "agents", environment)
	for _, report := range reports {
		if !report.AllSucceeded {
			t.Fatalf("test Skill install failed: %#v", reports)
		}
	}
	var readinessStatus atomic.Int32
	var leakedAuthorization atomic.Bool
	readinessStatus.Store(http.StatusNoContent)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/health/ready" {
			http.NotFound(writer, request)
			return
		}
		if request.Header.Get("Authorization") != "" {
			leakedAuthorization.Store(true)
		}
		writer.WriteHeader(int(readinessStatus.Load()))
	}))
	defer server.Close()
	store := securestore.NewMemory()
	run := func() (int, map[string]any) {
		t.Helper()
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		exit := Execute([]string{"doctor", "--agent", "agents"}, Dependencies{
			Out: &stdout, ErrOut: &stderr, Store: store, Skills: skills,
			Environment: environment, APIBaseURL: server.URL, Region: config.RegionCN,
		})
		var result map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			t.Fatalf("Doctor did not emit JSON: exit=%d stdout=%q stderr=%q err=%v", exit, stdout.String(), stderr.String(), err)
		}
		return exit, result
	}
	if exit, result := run(); exit != 0 {
		t.Fatalf("healthy Doctor failed: exit=%d result=%#v", exit, result)
	} else {
		data, ok := result["data"].(map[string]any)
		network, networkOK := data["network"].(map[string]any)
		if !ok || !networkOK || network["healthy"] != true {
			t.Fatalf("Doctor omitted network readiness: %#v", result)
		}
	}
	if leakedAuthorization.Load() {
		t.Fatal("Doctor readiness probe leaked a stored credential")
	}
	readinessStatus.Store(http.StatusServiceUnavailable)
	if exit, result := run(); exit == 0 || result["ok"] != false {
		t.Fatalf("Doctor accepted an unavailable API: exit=%d result=%#v", exit, result)
	}
}

func TestInstallTreatsCredentialStatusAsAdvisory(t *testing.T) {
	t.Parallel()
	runtime := &Runtime{
		region:  config.RegionCN,
		profile: config.Profile{ID: "profile-default", Name: "default", Region: config.RegionCN},
		deps:    Dependencies{Store: unavailableCredentialStore{}},
	}

	authenticated, known, warnings := installAuthenticationStatus(runtime)
	if authenticated || known {
		t.Fatalf("unavailable credential store was reported as a known login state: authenticated=%t known=%t", authenticated, known)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "could not be read") {
		t.Fatalf("install omitted the advisory credential warning: %#v", warnings)
	}
}
