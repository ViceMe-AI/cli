package command

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

// Each test child runs the real command in its own process and shares only
// the temporary home and the public HTTP fixture with the other runner.
func TestTrialUseProcessHelper(t *testing.T) {
	if os.Getenv("VICEME_TRIAL_TEST_CHILD") != "1" {
		return
	}
	home := os.Getenv("VICEME_TRIAL_TEST_HOME")
	scriptTrialLockWait = 100 * time.Millisecond
	code := Execute([]string{"skill", "use", downloadableProductID, "--wait", "0"}, Dependencies{
		Out: os.Stdout, ErrOut: os.Stderr, Store: securestore.NewMemory(),
		APIBaseURL: os.Getenv("VICEME_TRIAL_TEST_API"), Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: home, CodexHome: filepath.Join(home, ".codex"), ConfigDir: filepath.Join(home, ".viceme-cli")},
	})
	os.Exit(code)
}

func trialUseChild(t *testing.T, home, apiURL string) *exec.Cmd {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestTrialUseProcessHelper$")
	cmd.Env = append(os.Environ(), "VICEME_TRIAL_TEST_CHILD=1", "VICEME_TRIAL_TEST_HOME="+home, "VICEME_TRIAL_TEST_API="+apiURL, "HOME="+home, "USERPROFILE="+home, "VICEME_ACCESS_TOKEN=", "VICEME_CLI_CONFIG_DIR="+filepath.Join(home, ".viceme-cli"))
	return cmd
}

type trialUseReplayFixture struct {
	state       *skillTrialTestServer
	server      *httptest.Server
	mu          sync.Mutex
	requests    []string
	snapshots   map[string][]byte
	beforeReply func(int, http.ResponseWriter) bool
}

func newTrialUseReplayFixture(t *testing.T) *trialUseReplayFixture {
	t.Helper()
	f := &trialUseReplayFixture{state: newSkillTrialTestServer(t), snapshots: map[string][]byte{}}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/trial-use") {
			f.state.serveHTTP(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		id, ok := body["requestId"].(string)
		if !ok {
			t.Error("requestId missing")
			w.WriteHeader(400)
			return
		}
		f.mu.Lock()
		f.requests = append(f.requests, id)
		call := len(f.requests)
		if _, exists := f.snapshots[id]; !exists {
			raw, _ := json.Marshal(body)
			recorded := httptest.NewRecorder()
			f.state.serveHTTP(recorded, httptest.NewRequest(r.Method, r.URL.String(), bytes.NewReader(raw)))
			f.snapshots[id] = recorded.Body.Bytes()
		}
		response := f.snapshots[id]
		f.mu.Unlock()
		if f.beforeReply != nil && f.beforeReply(call, w) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(response)
	}))
	t.Cleanup(f.state.server.Close)
	t.Cleanup(f.server.Close)
	return f
}

func TestTrialUseConcurrentProcessesChargeIndependentTasks(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	f := newTrialUseReplayFixture(t)
	firstArrived, releaseFirst := make(chan struct{}), make(chan struct{})
	var release sync.Once
	unblock := func() { release.Do(func() { close(releaseFirst) }) }
	t.Cleanup(unblock)
	f.beforeReply = func(call int, w http.ResponseWriter) bool {
		if call == 1 {
			close(firstArrived)
			<-releaseFirst
		}
		return false
	}
	home := t.TempDir()
	if code, result, _ := executeSkillTrialCommand(t, f.server, home, securestore.NewMemory(), "skill", "install", downloadableProductID, "--agent", "agents"); code != 0 {
		t.Fatalf("install: %+v", result)
	}
	first := trialUseChild(t, home, f.server.URL)
	var firstOutput bytes.Buffer
	first.Stdout, first.Stderr = &firstOutput, &firstOutput
	if err := first.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Process.Kill() })
	select {
	case <-firstArrived:
	case <-time.After(10 * time.Second):
		t.Fatal("first process did not reach trial-use")
	}
	blocked, err := trialUseChild(t, home, f.server.URL).CombinedOutput()
	if err == nil || !bytes.Contains(blocked, []byte("SKILL_TRIAL_LOCK_BUSY")) || bytes.Contains(blocked, []byte("skillMarkdown")) {
		t.Fatalf("concurrent process reused the active task: err=%v output=%s", err, blocked)
	}
	f.mu.Lock()
	callsWhileLocked := len(f.requests)
	f.mu.Unlock()
	if callsWhileLocked != 1 {
		t.Fatalf("another process consumed while the first was running: %d", callsWhileLocked)
	}
	unblock()
	if err := first.Wait(); err != nil {
		t.Fatalf("first use: %v %s", err, firstOutput.Bytes())
	}
	secondOutput, err := trialUseChild(t, home, f.server.URL).CombinedOutput()
	if err != nil {
		t.Fatalf("second independent task: %v %s", err, secondOutput)
	}
	var firstResult, secondResult struct {
		Data skillTrialUseResult `json:"data"`
	}
	if err := json.Unmarshal(firstOutput.Bytes(), &firstResult); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(secondOutput, &secondResult); err != nil {
		t.Fatal(err)
	}
	a, b := firstResult.Data, secondResult.Data
	if !a.Allowed || !b.Allowed || a.SkillMarkdown == "" || b.SkillMarkdown == "" || a.RequestID == b.RequestID || !b.LastUse || !b.EntrySuspended {
		t.Fatalf("independent task delivery incorrect: first=%+v second=%+v", a, b)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.snapshots) != 2 {
		t.Fatalf("two tasks charged %d uses", len(f.snapshots))
	}
}

func TestTrialUseFailedGoProcessReplaysInPython(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python is required for the shared runtime contract")
	}
	t.Setenv(processAccessTokenEnvironment, "")
	f := newTrialUseReplayFixture(t)
	f.beforeReply = func(call int, w http.ResponseWriter) bool {
		if call == 1 {
			w.WriteHeader(500)
			return true
		}
		return false
	}
	home := t.TempDir()
	if code, result, _ := executeSkillTrialCommand(t, f.server, home, securestore.NewMemory(), "skill", "install", downloadableProductID, "--agent", "agents"); code != 0 {
		t.Fatalf("install: %+v", result)
	}
	raw, err := trialUseChild(t, home, f.server.URL).CombinedOutput()
	if err == nil {
		t.Fatalf("lost response must fail: %s", raw)
	}
	script, err := filepath.Abs("../../skills/use-a-skill/scripts/trial_runtime.py")
	if err != nil {
		t.Fatal(err)
	}
	code := `import importlib.util,sys
spec=importlib.util.spec_from_file_location("trial",sys.argv[1]);trial=importlib.util.module_from_spec(spec);spec.loader.exec_module(trial)
trial.API_ORIGIN["cn"]=sys.argv[2];trial.SCRIPT_ORIGIN["cn"]=sys.argv[2];trial.home_directory=lambda:sys.argv[3]
sys.exit(trial.run(["use","--product",sys.argv[4],"--market","cn","--agent","agents"]))`
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, python, "-B", "-c", code, script, f.server.URL, home, downloadableProductID)
	cmd.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home, "VICEME_ACCESS_TOKEN=")
	raw, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Python recovery: %v %s", err, raw)
	}
	var recovered map[string]any
	if err := json.Unmarshal(raw, &recovered); err != nil {
		t.Fatal(err)
	}
	if recovered["allowed"] != true || recovered["remainingUses"] != float64(1) || recovered["skillMarkdown"] == nil {
		t.Fatalf("task not recovered: %+v", recovered)
	}
	f.mu.Lock()
	if len(f.requests) != 2 || f.requests[0] != f.requests[1] || len(f.snapshots) != 1 {
		t.Errorf("cross-runner replay charged again: %v", f.requests)
	}
	f.mu.Unlock()
	raw, err = trialUseChild(t, home, f.server.URL).CombinedOutput()
	if err != nil {
		t.Fatalf("Go task after Python settlement: %v %s", err, raw)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) != 3 || f.requests[2] == f.requests[0] || len(f.snapshots) != 2 {
		t.Fatalf("new task reused Python's settled key: %v", f.requests)
	}
}

func TestTrialUseLegacyMigrationDoesNotRevivePythonSettlement(t *testing.T) {
	home := t.TempDir()
	runtime := &Runtime{configBase: t.TempDir(), apiBaseURL: "https://api.viceme.cn", region: config.RegionCN, deps: Dependencies{Environment: skillcontent.Environment{Home: home}}}
	id, err := beginTrialUsePending(runtime.configBase, runtime.apiBaseURL, downloadableProductID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// Crash after persisting the migration but before removing the CLI file;
	// Python then replays the shared key and atomically clears pendingRequestId.
	state := scriptTrialState{InstallID: "test-install", Secret: "test-secret", ProductID: downloadableProductID, Market: "cn", MigratedCLIRequestID: id}
	err = withScriptTrialLock(runtime, downloadableProductID, func() error {
		if err := saveScriptTrialState(runtime, downloadableProductID, state); err != nil {
			return err
		}
		return migrateLegacyTrialUsePending(runtime, downloadableProductID, &state)
	})
	if err != nil {
		t.Fatal(err)
	}
	if state.PendingRequestID != "" {
		t.Fatal("legacy copy revived a delivered task")
	}
	if _, err := os.Stat(trialUsePendingPath(runtime.configBase, runtime.apiBaseURL, downloadableProductID)); !os.IsNotExist(err) {
		t.Fatalf("legacy file not retired: %v", err)
	}
}

func TestScriptTrialLockDoesNotExpireLiveProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "live.lock")
	if err := os.WriteFile(path, []byte(fmt.Sprint(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().Add(-2 * scriptTrialLockStale)
	if err := os.Chtimes(path, stale, stale); err != nil {
		t.Fatal(err)
	}
	steal, err := scriptTrialLockShouldSteal(path)
	if err != nil || steal {
		t.Fatalf("live process lost its lock due to age: steal=%v err=%v", steal, err)
	}
}

func TestTrialUseKilledProcessReplaysWithoutAnotherCharge(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	f := newTrialUseReplayFixture(t)
	firstArrived, releaseFirst := make(chan struct{}), make(chan struct{})
	var release sync.Once
	unblock := func() { release.Do(func() { close(releaseFirst) }) }
	t.Cleanup(unblock)
	f.beforeReply = func(call int, w http.ResponseWriter) bool {
		if call == 1 {
			close(firstArrived)
			<-releaseFirst
		}
		return false
	}
	home := t.TempDir()
	if code, result, _ := executeSkillTrialCommand(t, f.server, home, securestore.NewMemory(), "skill", "install", downloadableProductID, "--agent", "agents"); code != 0 {
		t.Fatalf("install: %+v", result)
	}
	first := trialUseChild(t, home, f.server.URL)
	var discarded bytes.Buffer
	first.Stdout, first.Stderr = &discarded, &discarded
	if err := first.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Process.Kill() })
	select {
	case <-firstArrived:
	case <-time.After(10 * time.Second):
		t.Fatal("first process did not consume")
	}
	if err := first.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = first.Wait()
	unblock()
	recovered, err := trialUseChild(t, home, f.server.URL).CombinedOutput()
	if err != nil {
		t.Fatalf("dead-holder recovery: %v %s", err, recovered)
	}
	var result struct {
		Data skillTrialUseResult `json:"data"`
	}
	if err := json.Unmarshal(recovered, &result); err != nil {
		t.Fatal(err)
	}
	if !result.Data.Allowed || result.Data.SkillMarkdown == "" || result.Data.RemainingUses == nil || *result.Data.RemainingUses != 1 {
		t.Fatalf("lost task was not recovered: %+v", result.Data)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) != 2 || f.requests[0] != f.requests[1] || len(f.snapshots) != 1 {
		t.Fatalf("crash recovery consumed twice: %v", f.requests)
	}
}

func TestTrialUseSettledLegacyMarkerDoesNotRequestAnotherUse(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python is required for the shared runtime contract")
	}
	t.Setenv(processAccessTokenEnvironment, "")
	f := newTrialUseReplayFixture(t)
	home := t.TempDir()
	if code, result, _ := executeSkillTrialCommand(t, f.server, home, securestore.NewMemory(), "skill", "install", downloadableProductID, "--agent", "agents"); code != 0 {
		t.Fatalf("install: %+v", result)
	}
	runtime := &Runtime{configBase: filepath.Join(home, ".viceme-cli"), apiBaseURL: f.server.URL, region: config.RegionCN, deps: Dependencies{Environment: skillcontent.Environment{Home: home}}}
	id, err := beginTrialUsePending(runtime.configBase, runtime.apiBaseURL, downloadableProductID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	state, exists := readScriptTrialState(runtime, downloadableProductID)
	if !exists {
		t.Fatal("missing installed identity")
	}
	// Crash after shared migration is durable, before removing the legacy copy.
	state.PendingRequestID, state.MigratedCLIRequestID = id, id
	if err := withScriptTrialLock(runtime, downloadableProductID, func() error {
		return saveScriptTrialState(runtime, downloadableProductID, state)
	}); err != nil {
		t.Fatal(err)
	}
	code, ready, _ := executeSkillTrialCommand(t, f.server, home, securestore.NewMemory(), "skill", "ready", downloadableProductID, "--agent", "agents")
	if code != 0 || ready["data"].(map[string]any)["nextAction"] != "RESUME_TRIAL_USE" {
		t.Fatalf("unconfirmed migrated task lost recovery priority: %+v", ready)
	}
	code, purchase, _ := executeSkillTrialCommand(t, f.server, home, securestore.NewMemory(), "skill", "trial-purchase", downloadableProductID, "--wait", "0")
	if code == 0 || purchase["error"].(map[string]any)["code"] != "SKILL_TRIAL_USE_PENDING" {
		t.Fatalf("purchase bypassed unconfirmed migrated task: %+v", purchase)
	}
	script, err := filepath.Abs("../../skills/use-a-skill/scripts/trial_runtime.py")
	if err != nil {
		t.Fatal(err)
	}
	pycode := `import importlib.util,sys
spec=importlib.util.spec_from_file_location("trial",sys.argv[1]);trial=importlib.util.module_from_spec(spec);spec.loader.exec_module(trial)
trial.API_ORIGIN["cn"]=sys.argv[2];trial.SCRIPT_ORIGIN["cn"]=sys.argv[2];trial.home_directory=lambda:sys.argv[3]
sys.exit(trial.run(["use","--product",sys.argv[4],"--market","cn","--agent","agents"]))`
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, python, "-B", "-c", pycode, script, f.server.URL, home, downloadableProductID)
	cmd.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home, "VICEME_ACCESS_TOKEN=", "CODEX_HOME=", "CLAUDE_CONFIG_DIR=", "WORKBUDDY_CONFIG_DIR=", "VICEME_AGENTS_SKILLS_DIR=")
	if raw, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Python resume: %v %s", err, raw)
	}
	after, _ := readScriptTrialState(runtime, downloadableProductID)
	if after.PendingRequestID != "" || after.MigratedCLIRequestID != id {
		t.Fatalf("Python did not settle migration: %+v", after)
	}
	if got := readReusableTrialUsePending(trialUsePendingPath(runtime.configBase, runtime.apiBaseURL, downloadableProductID), downloadableProductID); got != id {
		t.Fatalf("fixture lost the legacy crash residue: %q", got)
	}
	code, ready, _ = executeSkillTrialCommand(t, f.server, home, securestore.NewMemory(), "skill", "ready", downloadableProductID, "--agent", "agents")
	if code != 0 {
		t.Fatalf("ready after settlement: %+v", ready)
	}
	data := ready["data"].(map[string]any)
	if data["nextAction"] == "RESUME_TRIAL_USE" || data["pendingUse"] == true {
		t.Fatalf("settled legacy copy instructed another charged use: %+v", data)
	}
	code, purchase, _ = executeSkillTrialCommand(t, f.server, home, securestore.NewMemory(), "skill", "trial-purchase", downloadableProductID, "--wait", "0")
	if code == 0 || purchase["error"].(map[string]any)["code"] != "SKILL_PURCHASE_REQUIRED" {
		t.Fatalf("settled legacy copy blocked purchase: %+v", purchase)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.snapshots) != 1 || len(f.requests) != 1 || f.requests[0] != id {
		t.Fatalf("migration recovery charged another task: requests=%v distinct=%d", f.requests, len(f.snapshots))
	}
}

func TestTrialUseMigrationMarkerCannotHideAnotherIdentityPending(t *testing.T) {
	runtime := &Runtime{configBase: t.TempDir(), apiBaseURL: "https://api.viceme.cn", region: config.RegionCN}
	id, err := beginTrialUsePending(runtime.configBase, runtime.apiBaseURL, downloadableProductID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	credential := skillTrialCredential{InstallID: "local-install", Secret: "local-secret"}
	for _, field := range []string{"product", "market", "install", "secret"} {
		t.Run(field, func(t *testing.T) {
			state := scriptTrialState{ProductID: downloadableProductID, Market: "cn", InstallID: credential.InstallID, Secret: credential.Secret, MigratedCLIRequestID: id}
			switch field {
			case "product":
				state.ProductID = "another-product"
			case "market":
				state.Market = "global"
			case "install":
				state.InstallID = "another-install"
			case "secret":
				state.Secret = "another-secret"
			}
			if !hasPendingTrialUse(runtime, downloadableProductID, credential, state) {
				t.Fatal("foreign migration marker hid the local unconfirmed request")
			}
		})
	}
}
