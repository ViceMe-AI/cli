package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	cliembed "github.com/ViceMe-AI/cli"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	updatepkg "github.com/ViceMe-AI/cli/internal/update"
)

type failingEntropyReader struct{}

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

func (updater *startupRecoveryUpdater) RecoverAtStartup(context.Context) error {
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
	if exit == 0 || !updater.called.Load() || !strings.Contains(stdout.String(), "NPM_ACTIVATION_RECOVERY_FAILED") {
		t.Fatalf("business command was not blocked by recovery failure: exit=%d called=%t stdout=%q", exit, updater.called.Load(), stdout.String())
	}
}

func TestCoordinatedNPMChildSkipsOnlyItsOuterRecovery(t *testing.T) {
	t.Parallel()
	if !coordinatedNPMChild([]string{"install", "--agent", "codex", "--internal-skip-launcher-ensure", "--internal-activation-child"}) {
		t.Fatal("coordinated npm child was not recognized")
	}
	if coordinatedNPMChild([]string{"version", "--internal-activation-child"}) || coordinatedNPMChild([]string{"install", "--internal-skip-launcher-ensure"}) {
		t.Fatal("ordinary command could bypass npm startup recovery")
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
