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
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	cliembed "github.com/ViceMe-AI/cli"
	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
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

type automaticUpdater struct {
	startupRecoveryUpdater
	check      updatepkg.CheckResult
	checkErr   error
	apply      updatepkg.ApplyResult
	applyErr   error
	checkCalls atomic.Int32
	applyCalls atomic.Int32
}

type completedUpdateUpdater struct {
	startupRecoveryUpdater
}

func (*completedUpdateUpdater) Check(context.Context) (updatepkg.CheckResult, error) {
	return updatepkg.CheckResult{CurrentVersion: "0.14.2", AvailableVersion: "0.15.0", UpdateAvailable: true}, nil
}

func (*completedUpdateUpdater) Apply(context.Context, updatepkg.CheckResult, updatepkg.ApplyOptions) (updatepkg.ApplyResult, error) {
	return updatepkg.ApplyResult{PreviousCLIVersion: "0.14.2", CLIVersion: "0.15.0"}, nil
}

func (updater *automaticUpdater) CheckAutomatic(context.Context) (updatepkg.CheckResult, error) {
	updater.checkCalls.Add(1)
	return updater.check, updater.checkErr
}

func (updater *automaticUpdater) Check(context.Context) (updatepkg.CheckResult, error) {
	return updater.check, updater.checkErr
}

func (updater *automaticUpdater) Apply(context.Context, updatepkg.CheckResult, updatepkg.ApplyOptions) (updatepkg.ApplyResult, error) {
	updater.applyCalls.Add(1)
	return updater.apply, updater.applyErr
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

func TestOrdinaryCommandAutomaticallyUpdatesAndReexecutesWithTheNewGeneration(t *testing.T) {
	clearAutomaticUpdateReexecutionEnvironment(t)
	root := t.TempDir()
	updater := &automaticUpdater{
		check: updatepkg.CheckResult{CurrentVersion: "0.15.2", AvailableVersion: "0.16.0", UpdateAvailable: true},
		apply: updatepkg.ApplyResult{PreviousCLIVersion: "0.15.2", CLIVersion: "0.16.0", Targets: []updatepkg.TargetResult{
			{Target: "npm_global", Status: "updated"},
			{Target: "agent_skill:auto", Status: "updated"},
		}},
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var reexecuted atomic.Bool
	exit := Execute([]string{"version"}, Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: securestore.NewMemory(), Updater: updater,
		Environment:                skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:                     config.RegionCN,
		allowDevelopmentAutoUpdate: true,
		Reexecute: func(_ context.Context, args, environment []string) (int, error) {
			reexecuted.Store(true)
			if !reflect.DeepEqual(args, []string{"version"}) {
				t.Fatalf("re-executed args=%#v", args)
			}
			joined := strings.Join(environment, "\n")
			for _, expected := range []string{
				autoUpdateReexecEnvironment + "=1",
				autoUpdateFromEnvironment + "=0.15.2",
				autoUpdateToEnvironment + "=0.16.0",
			} {
				if !strings.Contains(joined, expected) {
					t.Fatalf("re-execution environment missing %q", expected)
				}
			}
			_, _ = stdout.WriteString("{\"ok\":true,\"data\":{\"version\":\"0.16.0\"}}\n")
			return 0, nil
		},
	})
	if exit != 0 || !reexecuted.Load() || updater.checkCalls.Load() != 1 || updater.applyCalls.Load() != 1 {
		t.Fatalf("automatic update did not re-execute exactly once: exit=%d reexecuted=%t checks=%d applies=%d stdout=%q stderr=%q", exit, reexecuted.Load(), updater.checkCalls.Load(), updater.applyCalls.Load(), stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "0.15.2 -> 0.16.0") {
		t.Fatalf("automatic update progress missing from stderr: %q", stderr.String())
	}
	if strings.Count(stdout.String(), "\n") != 1 {
		t.Fatalf("old and new processes both emitted protocol output: %q", stdout.String())
	}
}

func TestAutomaticUpdateDiscoveryFailureIsFailOpen(t *testing.T) {
	clearAutomaticUpdateReexecutionEnvironment(t)
	root := t.TempDir()
	updater := &automaticUpdater{checkErr: errors.New("offline")}
	var stdout bytes.Buffer
	var reexecuted atomic.Bool
	exit := Execute([]string{"version"}, Dependencies{
		Out: &stdout, Store: securestore.NewMemory(), Updater: updater,
		Environment:                skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:                     config.RegionCN,
		allowDevelopmentAutoUpdate: true,
		Reexecute: func(context.Context, []string, []string) (int, error) {
			reexecuted.Store(true)
			return 0, nil
		},
	})
	if exit != 0 || reexecuted.Load() || updater.checkCalls.Load() != 1 || updater.applyCalls.Load() != 0 {
		t.Fatalf("offline freshness check changed the business command: exit=%d reexecuted=%t checks=%d applies=%d stdout=%q", exit, reexecuted.Load(), updater.checkCalls.Load(), updater.applyCalls.Load(), stdout.String())
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || result["ok"] != true {
		t.Fatalf("offline command lost its normal result: result=%#v err=%v", result, err)
	}
}

func TestAutomaticUpdateApplyFailureBlocksOldBusinessCommand(t *testing.T) {
	clearAutomaticUpdateReexecutionEnvironment(t)
	root := t.TempDir()
	updater := &automaticUpdater{
		check:    updatepkg.CheckResult{CurrentVersion: "0.15.2", AvailableVersion: "0.16.0", UpdateAvailable: true},
		applyErr: &updatepkg.OperationError{Kind: updatepkg.ErrorNPMCommand, Cause: errors.New("activation failed")},
	}
	var stdout bytes.Buffer
	var reexecuted atomic.Bool
	exit := Execute([]string{"version"}, Dependencies{
		Out: &stdout, Store: securestore.NewMemory(), Updater: updater,
		Environment:                skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:                     config.RegionCN,
		allowDevelopmentAutoUpdate: true,
		Reexecute: func(context.Context, []string, []string) (int, error) {
			reexecuted.Store(true)
			return 0, nil
		},
	})
	if exit == 0 || reexecuted.Load() || updater.applyCalls.Load() != 1 || !strings.Contains(stdout.String(), "update_npm_failed") {
		t.Fatalf("failed activation allowed old business logic: exit=%d reexecuted=%t applies=%d stdout=%q", exit, reexecuted.Load(), updater.applyCalls.Load(), stdout.String())
	}
}

func TestScheduledAutomaticUpdateRequiresRetryWithoutRunningTheOldCommand(t *testing.T) {
	clearAutomaticUpdateReexecutionEnvironment(t)
	root := t.TempDir()
	updater := &automaticUpdater{
		check: updatepkg.CheckResult{CurrentVersion: "0.15.2", AvailableVersion: "0.16.0", UpdateAvailable: true},
		apply: updatepkg.ApplyResult{PreviousCLIVersion: "0.15.2", CLIVersion: "0.16.0", Targets: []updatepkg.TargetResult{
			{Target: "standalone_binary", Status: "scheduled"},
			{Target: "agent_skill:auto", Status: "scheduled"},
		}},
	}
	var stdout bytes.Buffer
	var reexecuted atomic.Bool
	exit := Execute([]string{"version"}, Dependencies{
		Out: &stdout, Store: securestore.NewMemory(), Updater: updater,
		Environment:                skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:                     config.RegionCN,
		allowDevelopmentAutoUpdate: true,
		Reexecute: func(context.Context, []string, []string) (int, error) {
			reexecuted.Store(true)
			return 0, nil
		},
	})
	if exit == 0 || reexecuted.Load() || !strings.Contains(stdout.String(), "AUTO_UPDATE_RESTART_REQUIRED") || !strings.Contains(stdout.String(), `"retryable": true`) {
		t.Fatalf("scheduled update ran the old command or lost retry semantics: exit=%d reexecuted=%t stdout=%q", exit, reexecuted.Load(), stdout.String())
	}
}

func TestReexecutedCommandReportsAutomaticGenerationMetadata(t *testing.T) {
	clearAutomaticUpdateReexecutionEnvironment(t)
	t.Setenv(autoUpdateReexecEnvironment, "1")
	t.Setenv(autoUpdateFromEnvironment, "0.15.2")
	t.Setenv(autoUpdateToEnvironment, "0.16.0")
	root := t.TempDir()
	var stdout bytes.Buffer
	exit := Execute([]string{"version"}, Dependencies{
		Out: &stdout, Store: securestore.NewMemory(), Updater: &startupRecoveryUpdater{},
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:      config.RegionCN,
	})
	if exit != 0 {
		t.Fatalf("re-executed command failed: exit=%d stdout=%q", exit, stdout.String())
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	meta := result["meta"].(map[string]any)
	autoUpdate := meta["autoUpdate"].(map[string]any)
	if autoUpdate["from"] != "0.15.2" || autoUpdate["to"] != "0.16.0" || autoUpdate["status"] != "updated" {
		t.Fatalf("automatic re-execution metadata=%#v", meta)
	}
}

func TestCommandWaitingOnAnotherActivationReexecutesTheCommittedGeneration(t *testing.T) {
	clearAutomaticUpdateReexecutionEnvironment(t)
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	activationLock := flock.New(filepath.Join(configDir, updatepkg.ActivationLockFilename))
	if err := activationLock.Lock(); err != nil {
		t.Fatal(err)
	}
	targetVersion := versionAfterCurrentRelease(t)
	target, err := updatepkg.NewNPMGeneration(targetVersion)
	if err != nil {
		t.Fatal(err)
	}
	updater := updatepkg.NewNPMService(buildinfo.Version, buildinfo.CompatibilityVersion(), "npm")
	updater.ConfigDir = configDir
	type reexecution struct {
		args        []string
		environment []string
	}
	reexecuted := make(chan reexecution, 1)
	var stdout bytes.Buffer
	exited := make(chan int, 1)
	go func() {
		exited <- Execute([]string{"version"}, Dependencies{
			Out: &stdout, Store: securestore.NewMemory(), Updater: updater,
			Environment: skillcontent.Environment{Home: root, ConfigDir: configDir},
			Region:      config.RegionCN,
			Reexecute: func(_ context.Context, args, environment []string) (int, error) {
				reexecuted <- reexecution{args: append([]string(nil), args...), environment: append([]string(nil), environment...)}
				_, _ = stdout.WriteString("{\"ok\":true,\"data\":{\"version\":\"" + targetVersion + "\"}}\n")
				return 0, nil
			},
		})
	}()
	time.Sleep(100 * time.Millisecond)
	if err := updatepkg.CommitActiveGeneration(configDir, target); err != nil {
		_ = activationLock.Unlock()
		t.Fatal(err)
	}
	if err := activationLock.Unlock(); err != nil {
		t.Fatal(err)
	}
	if exit := <-exited; exit != 0 {
		t.Fatalf("waiting command failed instead of re-executing: exit=%d stdout=%q", exit, stdout.String())
	}
	got := <-reexecuted
	if !reflect.DeepEqual(got.args, []string{"version"}) {
		t.Fatalf("re-executed args=%#v", got.args)
	}
	joined := strings.Join(got.environment, "\n")
	for _, expected := range []string{
		autoUpdateReexecEnvironment + "=1",
		autoUpdateFromEnvironment + "=" + buildinfo.CompatibilityVersion(),
		autoUpdateToEnvironment + "=" + targetVersion,
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("re-execution environment missing %q", expected)
		}
	}
	if strings.Count(stdout.String(), "\n") != 1 || strings.Contains(stdout.String(), "ACTIVATION_RECOVERY_FAILED") {
		t.Fatalf("old process emitted a second or generic recovery response: %q", stdout.String())
	}
}

func clearAutomaticUpdateReexecutionEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{autoUpdateReexecEnvironment, autoUpdateFromEnvironment, autoUpdateToEnvironment} {
		t.Setenv(name, "")
	}
}

func TestUpdateSeparatesExecutingAndInstalledCLIVersions(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	var stdout bytes.Buffer
	exit := Execute([]string{"update"}, Dependencies{
		Out: &stdout, Store: securestore.NewMemory(), Updater: &completedUpdateUpdater{},
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:      config.RegionCN,
	})
	if exit != 0 {
		t.Fatalf("update command failed: exit=%d stdout=%q", exit, stdout.String())
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("update returned invalid JSON: %v stdout=%q", err, stdout.String())
	}
	data := result["data"].(map[string]any)
	meta := result["meta"].(map[string]any)
	if data["previous_cli_version"] != "0.14.2" || data["cli_version"] != "0.15.0" {
		t.Fatalf("update result lost installed-version semantics: %#v", result)
	}
	if meta["executingCliVersion"] != buildinfo.Version {
		t.Fatalf("update result lost executing-version semantics: %#v", result)
	}
	if _, legacy := meta["cliVersion"]; legacy {
		t.Fatalf("ambiguous legacy CLI version leaked: %#v", result)
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	recoveryResult := make(chan error, 1)
	go func() {
		recoveryResult <- reconcileActivationAtStartup(ctx, configDir, &dependencies)
	}()

	activationObserver := flock.New(filepath.Join(configDir, updatepkg.ActivationLockFilename))
	observerDeadline := time.NewTimer(5 * time.Second)
	defer observerDeadline.Stop()
	for {
		select {
		case err = <-recoveryResult:
			t.Fatalf("startup recovery returned before reaching the child commit barrier: %v", err)
		case <-observerDeadline.C:
			t.Fatal("startup recovery did not acquire the outer activation lock")
		default:
		}

		available, observeErr := activationObserver.TryLock()
		if observeErr != nil {
			t.Fatalf("could not observe the outer activation lock: %v", observeErr)
		}
		if !available {
			break
		}
		if observeErr := activationObserver.Unlock(); observeErr != nil {
			t.Fatalf("could not release the outer activation lock probe: %v", observeErr)
		}
		time.Sleep(time.Millisecond)
	}

	cancel()
	select {
	case err = <-recoveryResult:
	case <-time.After(time.Second):
		t.Fatal("startup recovery did not stop after its context was cancelled")
	}
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

func TestOfficialSkillBundleIncludesAccessAndTip(t *testing.T) {
	t.Parallel()
	found := map[string]bool{"viceme-access": false, "viceme-tip": false}
	for _, name := range officialSkillNames {
		if _, tracked := found[name]; tracked {
			found[name] = true
		}
	}
	for name, included := range found {
		if !included {
			t.Fatalf("official Skill list omitted %s: %#v", name, officialSkillNames)
		}
	}
	if len(retiredOfficialSkills) != 0 {
		t.Fatalf("active official Skills must not remain retired: %#v", retiredOfficialSkills)
	}
	for name := range found {
		if _, err := skillcontent.New(cliembed.EmbeddedSkills()).Package(name); err != nil {
			t.Fatalf("official Skill bundle omitted %s: %v", name, err)
		}
	}
	bundle := skillcontent.New(cliembed.EmbeddedSkills())
	template, _, err := bundle.Read("viceme-tip", "templates/single-html.html")
	if err != nil {
		t.Fatalf("official Skill bundle omitted the single HTML template: %v", err)
	}
	resolved := strings.ReplaceAll(string(template), "REPLACE_WITH_SDK_SCRIPT_URL", "https://viceme.example/viceme-sdk/v1/viceme.min.js")
	if !strings.Contains(resolved, `src="https://viceme.example/viceme-sdk/v1/viceme.min.js"`) || strings.Contains(resolved, "https://https://") {
		t.Fatalf("single HTML template does not accept the complete SDK URL")
	}
	if _, _, err := bundle.Read("viceme-tip", "references/integration-contract.md"); err != nil {
		t.Fatalf("official Skill bundle omitted its integration contract: %v", err)
	}
}

func TestOfficialSkillNamesMatchEmbeddedBundle(t *testing.T) {
	t.Parallel()
	bundled, err := skillcontent.New(cliembed.EmbeddedSkills()).List()
	if err != nil {
		t.Fatalf("could not list the embedded Skill bundle: %v", err)
	}
	bundledNames := make([]string, 0, len(bundled))
	for _, info := range bundled {
		bundledNames = append(bundledNames, info.Name)
	}
	sort.Strings(bundledNames)
	declared := append([]string(nil), officialSkillNames...)
	sort.Strings(declared)
	if !reflect.DeepEqual(declared, bundledNames) {
		t.Fatalf("official Skill list drifted from the embedded bundle: declared %#v, bundled %#v", declared, bundledNames)
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
		profile: config.Profile{ID: "profile-default", Name: "default", APIBaseURL: config.APIBaseURL(config.RegionCN)},
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

func TestInstallTreatsActiveProfileNetworkReadinessAsAdvisory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/health/ready" {
			http.NotFound(writer, request)
			return
		}
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	store := securestore.NewMemory()
	scope, err := credentialScopeForAPIBase(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	manager := credentialauth.Manager{Store: store, Region: "cn", ProfileID: "default", ProfileName: "default", Scope: scope}
	if err := manager.Save(credentialauth.Credential{AccessToken: "vme_cli_test", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := Execute([]string{"install", "--agent", "agents"}, Dependencies{
		Out: &stdout, ErrOut: &stderr,
		Store: store, Updater: &startupRecoveryUpdater{},
		Skills:      skillcontent.New(cliembed.EmbeddedSkills()),
		Environment: skillcontent.Environment{Home: root, ConfigDir: configDir},
		APIBaseURL:  server.URL, Region: config.RegionCN,
	})
	if exit != 0 {
		t.Fatalf("install rolled back because the old profile API was unavailable: exit=%d stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Warnings []string `json:"warnings"`
			NextStep struct {
				Command string `json:"command"`
			} `json:"nextStep"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("install returned invalid JSON: %v stdout=%q", err, stdout.String())
	}
	if !envelope.OK || len(envelope.Data.Warnings) != 1 || !strings.Contains(envelope.Data.Warnings[0], "API is unreachable") {
		t.Fatalf("install omitted the advisory API warning: %#v", envelope)
	}
	if envelope.Data.NextStep.Command != "viceme skill publish --path <dir-or-zip>" {
		t.Fatalf("install returned the obsolete pre-preview workflow: %#v", envelope.Data.NextStep)
	}
	for _, name := range officialSkillNames {
		if !skillcontent.New(cliembed.EmbeddedSkills()).Doctor(name, "agents", skillcontent.Environment{Home: root, ConfigDir: configDir}).Healthy {
			t.Fatalf("install did not commit %s while the old profile API was unavailable", name)
		}
	}
}

func TestFreshGlobalInstallPersistsCompleteGlobalAuthority(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/v1/health/ready" {
			http.NotFound(writer, request)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := Execute([]string{"install", "--agent", "agents", "--region", "global"}, Dependencies{
		Out: &stdout, ErrOut: &stderr,
		Store: securestore.NewMemory(), Updater: &startupRecoveryUpdater{},
		Skills:      skillcontent.New(cliembed.EmbeddedSkills()),
		Environment: skillcontent.Environment{Home: root, ConfigDir: configDir},
		APIBaseURL:  server.URL, Region: config.RegionCN,
	})
	if exit != 0 {
		t.Fatalf("fresh global install failed: exit=%d stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
	}
	persisted, err := config.LoadOrDefault(configDir)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := persisted.Resolve(config.DefaultProfileName)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.DistributionRegion != config.RegionGlobal ||
		profile.APIBaseURL != config.APIBaseURL(config.RegionGlobal) ||
		profile.WebBaseURL != config.WebBaseURL(config.RegionGlobal) ||
		profile.MarketRegion != config.RegionGlobal {
		t.Fatalf("fresh global install persisted a split authority: %#v", persisted)
	}
	if !strings.Contains(stdout.String(), `"region": "global"`) {
		t.Fatalf("fresh global install reported the wrong region: %s", stdout.String())
	}
}

func TestCredentialScopeUsesCanonicalAPIOrigin(t *testing.T) {
	first, err := credentialScopeForAPIBase("https://API.EXAMPLE.com:443/")
	if err != nil {
		t.Fatal(err)
	}
	same, err := credentialScopeForAPIBase("https://api.example.com")
	if err != nil {
		t.Fatal(err)
	}
	other, err := credentialScopeForAPIBase("https://other.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if first == "" || first != same || first == other {
		t.Fatalf("credential scope did not follow canonical API origin: first=%q same=%q other=%q", first, same, other)
	}
}

func TestLegacyCredentialRegionOnlyMatchesOfficialAPIOrigins(t *testing.T) {
	t.Parallel()
	if got := legacyCredentialRegionForAPIBase("HTTPS://API.VICEME.CN:443/"); got != "cn" {
		t.Fatalf("CN official origin legacy region = %q", got)
	}
	if got := legacyCredentialRegionForAPIBase("https://api.viceme.ai"); got != "global" {
		t.Fatalf("GLOBAL official origin legacy region = %q", got)
	}
	for _, endpoint := range []string{"http://127.0.0.1:3001", "https://shop-dev.example.com"} {
		if got := legacyCredentialRegionForAPIBase(endpoint); got != "" {
			t.Fatalf("custom endpoint %q inherited official credentials through %q", endpoint, got)
		}
	}
}
