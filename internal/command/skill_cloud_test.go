package command

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestCloudRegisteredPurchaseRetriesSameUserAndNeverDownloads(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, skillPurchaseAccessToken)
	state := newSkillPurchaseTestServer(t)
	defer state.server.Close()
	original := state.server.Config.Handler
	var user atomic.Value
	user.Store(state.userID)
	var cloudCalls atomic.Int32
	state.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/cli/auth/status" {
			writeJSONResponse(w, map[string]any{"authenticated": true, "user": map[string]any{"id": user.Load()}, "scopes": state.scopes})
			return
		}
		if r.URL.Path == "/v1/cli/skill-cloud/requests" {
			cloudCalls.Add(1)
			var input map[string]any
			_ = json.NewDecoder(r.Body).Decode(&input)
			if _, hasSecret := input["secret"]; hasSecret || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer vme_cli_") {
				t.Error("registered cloud credential boundary violated")
			}
			state.mu.Lock()
			paid := state.paymentStatus == "PAID"
			state.mu.Unlock()
			if !paid {
				w.WriteHeader(403)
				writeJSONResponse(w, map[string]any{"statusCode": 403, "code": "SKILL_CLOUD_ENTITLEMENT_REQUIRED", "message": "purchase required", "requestId": "trace"})
				return
			}
			writeJSONResponse(w, map[string]any{"requestId": input["requestKey"], "sessionId": "11111111-1111-4111-8111-111111111111", "releaseId": input["releaseId"], "version": 1, "status": "SUCCEEDED", "outcome": "ready", "instructions": "Execute this task.", "message": nil, "retryable": false, "errorCode": nil, "expiresAt": "2099-01-01T00:00:00Z"})
			return
		}
		if strings.Contains(r.URL.Path, "download") || r.URL.Path == "/artifact" {
			t.Error("registered payment downloaded a package")
		}
		original.ServeHTTP(w, r)
	})
	home := t.TempDir()
	inputPath := filepath.Join(home, "task.json")
	raw, _ := json.Marshal(map[string]any{"productId": downloadableProductID, "releaseId": downloadableReleaseID, "requestKey": "12121212-1212-4212-8212-121212121212", "prompt": "draft", "facts": map[string]string{}})
	if err := os.WriteFile(inputPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	invoke := func() (int, map[string]any) {
		code, result, _ := executeSkillPurchaseCommand(t, state.server, home, "skill", "cloud", "--input", inputPath, "--wait", "0")
		return code, result
	}
	code, result := invoke()
	if code == 0 || result["error"].(map[string]any)["code"] != "SKILL_PURCHASE_REQUIRED" {
		t.Fatalf("registered purchase: %#v", result)
	}
	state.mu.Lock()
	state.paymentStatus = "PAID"
	state.mu.Unlock()
	// A refreshed token for the same user must reuse the persisted identity.
	t.Setenv(processAccessTokenEnvironment, "vme_cli_9876543210987654321098765432109876543210987")
	code, result = invoke()
	if code != 0 || result["data"].(map[string]any)["allowed"] != true {
		t.Fatalf("paid task recovery: %#v", result)
	}
	before := cloudCalls.Load()
	user.Store("34343434-3434-4434-8434-343434343434")
	code, result = invoke()
	if code == 0 || result["error"].(map[string]any)["code"] != "SKILL_CLOUD_REQUEST_CONFLICT" || cloudCalls.Load() != before {
		t.Fatalf("task changed user: %#v", result)
	}
}

func TestCloudInstallLostResponseCrossRunnerAndPaidRecovery(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()
	home, store := t.TempDir(), securestore.NewMemory()
	var mu sync.Mutex
	tasks := map[string]map[string]any{}
	var submitted []string
	loseResponse := true
	paid := false
	deliveries, downloads, oldUses := 0, 0, 0
	original := state.server.Config.Handler
	state.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/skill-cloud/trial/requests/") && strings.HasSuffix(r.URL.Path, "/read") {
			writeJSONResponse(w, map[string]any{"requestId": "89898989-8989-4989-8989-898989898989", "sessionId": "11111111-1111-4111-8111-111111111111", "releaseId": downloadableReleaseID, "version": 1, "status": "SUCCEEDED", "instructions": "Wrong task must never execute.", "message": nil, "outcome": "ready", "errorCode": nil, "retryable": false, "expiresAt": "2099-01-01T00:00:00Z"})
			return
		}
		if r.URL.Path == "/v1/skill-cloud/trial/requests" {
			var input map[string]any
			_ = json.NewDecoder(r.Body).Decode(&input)
			if input["releaseId"] != downloadableReleaseID || input["secret"] != skillTrialSecret || input["installId"] == "" || r.Header.Get("Authorization") != "" {
				t.Error("invalid anonymous request credential boundary")
				w.WriteHeader(401)
				return
			}
			key := input["requestKey"].(string)
			mu.Lock()
			defer mu.Unlock()
			submitted = append(submitted, key)
			result := tasks[key]
			if result == nil {
				if input["prompt"] == "paid task" && !paid {
					w.WriteHeader(403)
					writeJSONResponse(w, map[string]any{"statusCode": 403, "code": "SKILL_CLOUD_TRIAL_EXHAUSTED", "message": "quota exhausted", "requestId": "trace-cloud"})
					return
				}
				outcome := "ready"
				var instructions any = "Use the supplied facts for this task."
				var message any
				if input["prompt"] == "question" {
					outcome = "needs_input"
					instructions = nil
					message = "Please supply the audience."
				}
				if input["prompt"] == "refuse" {
					outcome = "refused"
					instructions = nil
					message = "This task is outside the Skill purpose."
				}
				if outcome == "ready" && input["prompt"] != "queued" {
					deliveries++
				}
				result = map[string]any{"requestId": key, "sessionId": "11111111-1111-4111-8111-111111111111", "releaseId": downloadableReleaseID, "version": 1, "status": "SUCCEEDED", "instructions": instructions, "message": message, "outcome": outcome, "errorCode": nil, "retryable": false, "expiresAt": "2099-01-01T00:00:00Z", "trial": nil}
				if input["prompt"] == "queued" {
					result["status"] = "RUNNING"
					result["outcome"] = nil
					result["instructions"] = nil
				}
				tasks[key] = result
			}
			if loseResponse {
				loseResponse = false
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
				} else {
					_ = conn.Close()
				}
				return
			}
			writeJSONResponse(w, result)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/trial-use") {
			mu.Lock()
			oldUses++
			mu.Unlock()
		}
		if r.URL.Path == "/artifact" || r.URL.Path == "/owned-artifact" {
			mu.Lock()
			downloads++
			mu.Unlock()
		}
		recorder := httptest.NewRecorder()
		original.ServeHTTP(recorder, r)
		var body map[string]any
		if json.Unmarshal(recorder.Body.Bytes(), &body) == nil {
			if strings.HasSuffix(r.URL.Path, "/access") || strings.Contains(r.URL.Path, "/downloads/") {
				body["deliveryMode"] = "CLOUD"
			}
			if strings.HasSuffix(r.URL.Path, "/trial-purchase/download") {
				body["access"].(map[string]any)["deliveryMode"] = "CLOUD"
				body["download"] = nil
			}
			w.WriteHeader(recorder.Code)
			writeJSONResponse(w, body)
			return
		}
		w.WriteHeader(recorder.Code)
		_, _ = w.Write(recorder.Body.Bytes())
	})
	invoke := func(args ...string) (int, map[string]any) {
		code, result, _ := executeSkillTrialCommand(t, state.server, home, store, args...)
		return code, result
	}
	state.grantUses = state.trialLimit // An exhausted cloud trial still installs the short entrypoint.
	code, installed := invoke("skill", "install", downloadableProductID, "--agent", "codex")
	if code != 0 {
		t.Fatalf("cloud install: %#v", installed)
	}
	data := installed["data"].(map[string]any)
	skillPath := data["skillPath"].(string)
	originalSkill, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(originalSkill, []byte(skillTrialGateMarker)) || bytes.Contains(originalSkill, []byte("viceme-trial-disabled")) {
		t.Fatal("cloud stub was gated or suspended")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(skillPath), skillTrialRuntimePath)); !os.IsNotExist(err) {
		t.Fatal("cloud installed legacy trial rules")
	}
	code, ready := invoke("skill", "ready", downloadableProductID, "--agent", "codex")
	if code != 0 || ready["data"].(map[string]any)["nextAction"] != "SUBMIT_CLOUD_TASK" {
		t.Fatalf("cloud ready: %#v", ready)
	}
	if code, _ = invoke("skill", "use", downloadableProductID); code == 0 {
		t.Fatal("cloud use granted offline execution")
	}
	inputPath := filepath.Join(home, "task.json")
	key := "22222222-2222-4222-8222-222222222222"
	writeInput := func(key, prompt string) {
		raw, _ := json.Marshal(map[string]any{"productId": downloadableProductID, "requestKey": key, "prompt": prompt, "facts": map[string]string{}})
		if err := os.WriteFile(inputPath, raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeInput(key, "draft")
	if code, _ = invoke("skill", "cloud", "--input", inputPath, "--wait", "0"); code == 0 {
		t.Fatal("lost response falsely succeeded")
	}
	code, result := invoke("skill", "cloud", "--input", inputPath, "--wait", "0")
	if code != 0 || result["data"].(map[string]any)["allowed"] != true {
		t.Fatalf("retry: %#v", result)
	}
	mu.Lock()
	if deliveries != 1 || len(submitted) != 2 || submitted[0] != submitted[1] {
		t.Fatal("retry did not retain immutable task identity")
	}
	mu.Unlock()
	// Run the actual installed Python archive without import/mocking. It reads
	// product/market/API from its installed environment and shares the Go task.
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(python, data["runtimePath"].(string), "cloud", "--input", inputPath, "--wait", "0")
	child.Env = append(os.Environ(), "HOME="+home, "PYTHONDONTWRITEBYTECODE=1")
	raw, err := child.CombinedOutput()
	if err != nil {
		t.Fatalf("shipped Python: %v %s", err, raw)
	}
	var pyResult map[string]any
	if json.Unmarshal(raw, &pyResult) != nil || pyResult["ok"] != true {
		t.Fatalf("Python envelope: %s", raw)
	}
	mu.Lock()
	if deliveries != 1 {
		t.Fatal("cross-runner retry created another delivery")
	}
	mu.Unlock()
	writeInput(key, "changed")
	if code, result = invoke("skill", "cloud", "--input", inputPath, "--wait", "0"); code == 0 || result["error"].(map[string]any)["code"] != "SKILL_CLOUD_REQUEST_CONFLICT" {
		t.Fatalf("mutated input: %#v", result)
	}
	for index, prompt := range []string{"question", "refuse"} {
		newKey := []string{"33333333-3333-4333-8333-333333333333", "44444444-4444-4444-8444-444444444444"}[index]
		writeInput(newKey, prompt)
		code, result = invoke("skill", "cloud", "--input", inputPath, "--wait", "0")
		if code != 0 || result["data"].(map[string]any)["allowed"] != false {
			t.Fatalf("nonready: %#v", result)
		}
		if _, exists := result["data"].(map[string]any)["executionPath"]; exists {
			t.Fatal("non-ready response delivered instructions")
		}
	}
	writeInput("78787878-7878-4787-8787-787878787878", "queued")
	code, result = invoke("skill", "cloud", "--input", inputPath, "--wait", "1s")
	if code == 0 || result["error"].(map[string]any)["code"] != "SKILL_CLOUD_RESPONSE_INVALID" {
		t.Fatalf("poll returned other task: %#v", result)
	}
	child = exec.Command(python, data["runtimePath"].(string), "cloud", "--input", inputPath, "--wait", "1")
	child.Env = append(os.Environ(), "HOME="+home, "PYTHONDONTWRITEBYTECODE=1")
	raw, err = child.CombinedOutput()
	if err == nil || !bytes.Contains(raw, []byte("SKILL_CLOUD_RESPONSE_INVALID")) {
		t.Fatalf("Python accepted wrong polled task: %v %s", err, raw)
	}
	paidKey := "55555555-5555-4555-8555-555555555555"
	writeInput(paidKey, "paid task")
	code, result = invoke("skill", "cloud", "--input", inputPath, "--wait", "0")
	if code == 0 || result["error"].(map[string]any)["code"] != "SKILL_PURCHASE_REQUIRED" {
		t.Fatalf("purchase: %#v", result)
	}
	// An earlier-version pending task must not block recovery of this task.
	paths, err := filepath.Glob(filepath.Join(home, ".viceme", "cloud", "*", paidKey+".json"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("pending record: %v %v", paths, err)
	}
	pending, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	var older cloudTaskRecord
	if err := json.Unmarshal(pending, &older); err != nil {
		t.Fatal(err)
	}
	older.Input.RequestKey = "11111111-1111-4111-8111-111111111111"
	older.Input.ReleaseID = "99999999-9999-4999-8999-999999999999"
	olderPath := filepath.Join(filepath.Dir(paths[0]), older.Input.RequestKey+".json")
	if err := writeCloudRecord(olderPath, older); err != nil {
		t.Fatal(err)
	}
	olderBefore, _ := os.ReadFile(olderPath)
	state.mu.Lock()
	state.paymentStatus = "PAID"
	state.mu.Unlock()
	mu.Lock()
	paid = true
	beforeDownloads := downloads
	mu.Unlock()
	code, result = invoke("skill", "trial-purchase", downloadableProductID, "--wait", "0")
	if code != 0 {
		t.Fatalf("paid recovery: %#v", result)
	}
	resumed := result["data"].(map[string]any)["resumedTasks"].([]any)
	if len(resumed) != 2 || resumed[0].(map[string]any)["error"].(map[string]any)["code"] != "SKILL_CLOUD_RELEASE_MISMATCH" || resumed[1].(map[string]any)["requestKey"] != paidKey || resumed[1].(map[string]any)["allowed"] != true {
		t.Fatalf("wrong recovered task: %#v", result)
	}
	olderAfter, _ := os.ReadFile(olderPath)
	if !bytes.Equal(olderBefore, olderAfter) {
		t.Fatal("recovery changed the previous release's pending task")
	}
	afterSkill, _ := os.ReadFile(skillPath)
	mu.Lock()
	if downloads != beforeDownloads || oldUses != 0 || !bytes.Equal(afterSkill, originalSkill) {
		t.Fatal("payment downloaded/replaced source or invoked legacy trial quota")
	}
	mu.Unlock()
	// A saved v1 task cannot execute after public assets were installed as v2.
	replacement := "99999999-9999-4999-8999-999999999999"
	for _, name := range []string{"runtime.json", "install-manifest.json"} {
		path := filepath.Join(filepath.Dir(skillPath), ".viceme", name)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		raw = bytes.ReplaceAll(raw, []byte(downloadableReleaseID), []byte(replacement))
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	code, result = invoke("skill", "cloud", "--input", inputPath, "--wait", "0")
	if code == 0 || result["error"].(map[string]any)["code"] != "SKILL_CLOUD_RELEASE_MISMATCH" {
		t.Fatalf("old task accepted new public assets: %#v", result)
	}
}

func TestCloudDirectPurchaseInstallsPublicPackageWithoutTrialAndKeepsIdentity(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()
	home, store := t.TempDir(), securestore.NewMemory()
	original := state.server.Config.Handler
	var firstID, firstSecret string
	state.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/trial-grants") || strings.Contains(r.URL.Path, "/skill-cloud/trial/") {
			t.Error("direct purchase used a trial")
			w.WriteHeader(400)
			return
		}
		if r.URL.Path == "/v1/downloads/cloud/"+downloadableProductID {
			writeJSONResponse(w, map[string]any{"url": state.server.URL + "/artifact", "deliveryMode": "CLOUD", "releaseId": downloadableReleaseID, "artifactDigest": state.archiveDigest, "expiresAt": "2099-01-01T00:00:00Z"})
			return
		}
		if r.URL.Path == "/v1/skill-cloud/purchase/requests" {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			id, secret := body["installId"].(string), body["secret"].(string)
			if len(secret) != 64 || !skillUseProductIDPattern.MatchString(id) {
				t.Error("invalid identity")
			}
			if firstID == "" {
				firstID, firstSecret = id, secret
			} else if firstID != id || firstSecret != secret {
				t.Error("purchase identity changed")
			}
			writeJSONResponse(w, map[string]any{"requestId": body["requestKey"], "sessionId": "11111111-1111-4111-8111-111111111111", "releaseId": downloadableReleaseID, "version": 1, "status": "SUCCEEDED", "outcome": "ready", "instructions": "Follow the local workflow.", "message": nil, "errorCode": nil, "retryable": false, "expiresAt": "2099-01-01T00:00:00Z"})
			return
		}
		recorder := httptest.NewRecorder()
		original.ServeHTTP(recorder, r)
		var body map[string]any
		if strings.HasSuffix(r.URL.Path, "/access") && json.Unmarshal(recorder.Body.Bytes(), &body) == nil {
			body["deliveryMode"], body["trial"], body["purchaseAvailable"] = "CLOUD", nil, true
			writeJSONResponse(w, body)
			return
		}
		w.WriteHeader(recorder.Code)
		_, _ = w.Write(recorder.Body.Bytes())
	})
	invoke := func(args ...string) (int, map[string]any) {
		code, result, _ := executeSkillTrialCommand(t, state.server, home, store, args...)
		return code, result
	}
	code, result := invoke("skill", "install", downloadableProductID, "--agent", "codex")
	if code != 0 {
		t.Fatalf("install: %#v", result)
	}
	skillPath := result["data"].(map[string]any)["skillPath"].(string)
	manifestBytes, err := os.ReadFile(filepath.Join(filepath.Dir(skillPath), ".viceme", "runtime.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	_ = json.Unmarshal(manifestBytes, &manifest)
	if manifest["kind"] != "purchase" || manifest["deliveryMode"] != "CLOUD" {
		t.Fatalf("wrong runtime: %#v", manifest)
	}
	task := filepath.Join(home, "task.json")
	input, _ := json.Marshal(map[string]any{"productId": downloadableProductID, "requestKey": "12121212-1212-4212-8212-121212121212", "prompt": "review local inputs", "facts": map[string]string{}})
	if err := os.WriteFile(task, input, 0o600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		code, result = invoke("skill", "cloud", "--input", task, "--agent", "codex", "--wait", "0")
		if code != 0 || result["data"].(map[string]any)["allowed"] != true {
			t.Fatalf("cloud: %#v", result)
		}
	}
	purchasePath := filepath.Join(home, ".viceme", "purchases", downloadableProductID+".json")
	persisted, err := os.ReadFile(purchasePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(persisted, []byte(firstID)) || !bytes.Contains(persisted, []byte(firstSecret)) {
		t.Fatal("identity not durable")
	}
	if _, err := os.Stat(filepath.Join(home, ".viceme", "trials", downloadableProductID+".json")); !os.IsNotExist(err) {
		t.Fatal("created trial state")
	}
}

// Task execution and direct-purchase identity selection must both honor the
// invoking directory, including workspace names excluded from host discovery.
func TestCloudTaskUsesExactSkillDirectory(t *testing.T) {
	const selectedRelease = "99999999-9999-4999-8999-999999999999"
	for _, test := range []struct {
		name      string
		directory string
		release   string
		missing   bool
		corrupt   bool
		allowed   bool
	}{
		{name: "coexisting versions", directory: "selected-v2", allowed: true},
		{name: "dotted workspace directory", directory: "selected.v2", allowed: true},
		{name: "explicit old version cannot select sibling", directory: "selected-v2", release: downloadableReleaseID},
		{name: "missing directory with explicit release", directory: "missing", release: downloadableReleaseID, missing: true},
		{name: "missing directory without explicit release", directory: "missing", missing: true},
		{name: "corrupt directory cannot use valid sibling", directory: "selected-v2", release: downloadableReleaseID, corrupt: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(processAccessTokenEnvironment, "")
			home, store := t.TempDir(), securestore.NewMemory()
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/v1/skill-cloud/purchase/requests" {
					t.Errorf("exact purchase installation used another flow: %s", r.URL.Path)
					http.Error(w, "unexpected request", http.StatusBadRequest)
					return
				}
				var input map[string]any
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					t.Error(err)
					return
				}
				if input["releaseId"] != selectedRelease || input["installId"] == "" || input["secret"] == "" {
					t.Errorf("task did not bind selected installation: release=%v", input["releaseId"])
				}
				writeJSONResponse(w, map[string]any{"requestId": input["requestKey"], "sessionId": "11111111-1111-4111-8111-111111111111", "releaseId": input["releaseId"], "version": 1, "status": "SUCCEEDED", "outcome": "ready", "instructions": "Follow the selected local workflow.", "message": nil, "retryable": false, "errorCode": nil, "expiresAt": "2099-01-01T00:00:00Z"})
			}))
			defer server.Close()
			workspace := t.TempDir()

			// Discovery would pick this alphabetically first installation. Its different
			// kind also detects an exact task lookup followed by a broad buyer lookup.
			writeCloudRuntimeFixture(t, filepath.Join(workspace, "a-old-owned"), server.URL, downloadableReleaseID, "owned")
			selected := filepath.Join(workspace, test.directory)
			if !test.missing {
				writeCloudRuntimeFixture(t, selected, server.URL, selectedRelease, "purchase")
			}
			if test.corrupt {
				if err := os.WriteFile(filepath.Join(selected, "WORKFLOW.md"), []byte("changed"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			input := map[string]any{"productId": downloadableProductID, "requestKey": "12121212-1212-4212-8212-121212121212", "prompt": "review local inputs", "facts": map[string]string{}}
			if test.release != "" {
				input["releaseId"] = test.release
			}
			raw, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			inputPath := filepath.Join(home, "task.json")
			if err := os.WriteFile(inputPath, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			code, result, _ := executeSkillTrialCommand(t, server, home, store, "skill", "cloud", "--input", inputPath, "--skill-dir", selected, "--wait", "0")
			if test.allowed {
				if code != 0 || result["data"].(map[string]any)["allowed"] != true || calls.Load() != 1 {
					t.Fatalf("selected installation could not run: calls=%d result=%#v", calls.Load(), result)
				}
				if _, err := os.Stat(filepath.Join(home, ".viceme", "purchases", downloadableProductID+".json")); err != nil {
					t.Fatal(err)
				}
			} else {
				if code == 0 || result["error"].(map[string]any)["code"] != "SKILL_CLOUD_RELEASE_MISMATCH" || calls.Load() != 0 {
					t.Fatalf("invalid selected installation reached task or wrong error: calls=%d result=%#v", calls.Load(), result)
				}
				if _, err := os.Stat(filepath.Join(home, ".viceme", "purchases", downloadableProductID+".json")); !os.IsNotExist(err) {
					t.Fatalf("invalid installation created a purchase identity: %v", err)
				}
			}
		})
	}
}

func writeCloudRuntimeFixture(t *testing.T, directory, apiBaseURL, release, kind string) {
	t.Helper()
	files := map[string]downloadableSkillFile{
		"SKILL.md":    {Data: []byte("---\nname: cloud-fixture\n---\nUse the public workflow.\n"), Mode: 0o644},
		"WORKFLOW.md": {Data: []byte("Read local input and apply this task's guidance.\n"), Mode: 0o644},
	}
	runtime := &Runtime{apiBaseURL: apiBaseURL, region: config.RegionCN}
	if err := addSkillRuntime(runtime, files, downloadableProductID, release, kind, "CLOUD"); err != nil {
		t.Fatal(err)
	}
	owner, err := json.Marshal(map[string]string{"product_id": downloadableProductID, "release_id": release})
	if err != nil {
		t.Fatal(err)
	}
	files[".viceme/install-manifest.json"] = downloadableSkillFile{Data: owner, Mode: 0o600}
	for name, file := range files {
		filename := filepath.Join(directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, file.Data, file.Mode); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCloudPurchaseRecoveryUsesExactSkillDirectory(t *testing.T) {
	const selectedRelease = "99999999-9999-4999-8999-999999999999"
	const installID = "33333333-3333-4333-8333-333333333333"
	for _, kind := range []string{"trial", "purchase"} {
		for _, corrupt := range []bool{false, true} {
			name := kind + "/ready"
			if corrupt {
				name = kind + "/corrupt"
			}
			t.Run(name, func(t *testing.T) {
				t.Setenv(processAccessTokenEnvironment, "")
				home, store := t.TempDir(), securestore.NewMemory()
				endpoint := "trial-purchase"
				if kind == "purchase" {
					endpoint = "purchase"
				}
				var cloudCalls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/v1/skills/" + downloadableProductID + "/" + endpoint + "/status":
						writeJSONResponse(w, map[string]any{"productId": downloadableProductID, "orderNo": skillPurchaseOrderNo, "title": "Cloud fixture", "status": "PAID", "amountCents": 100, "currency": "CNY", "expiresAt": "2099-01-01T00:00:00Z", "paymentAction": nil})
					case "/v1/skills/" + downloadableProductID + "/" + endpoint + "/download":
						writeJSONResponse(w, map[string]any{"access": map[string]any{"productId": downloadableProductID, "owned": true, "installKind": "OWNED_PAID", "deliveryMode": "CLOUD", "release": map[string]any{"id": selectedRelease}}, "download": nil})
					case "/v1/skill-cloud/" + kind + "/requests":
						cloudCalls.Add(1)
						var input map[string]any
						if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
							t.Error(err)
							return
						}
						if input["releaseId"] != selectedRelease || input["installId"] != installID {
							t.Error("purchase recovery changed the installation or buyer identity")
						}
						writeJSONResponse(w, map[string]any{"requestId": input["requestKey"], "sessionId": "11111111-1111-4111-8111-111111111111", "releaseId": input["releaseId"], "version": 1, "status": "SUCCEEDED", "outcome": "ready", "instructions": "Follow the selected local workflow.", "message": nil, "retryable": false, "errorCode": nil, "expiresAt": "2099-01-01T00:00:00Z"})
					default:
						t.Errorf("unexpected purchase recovery request: %s", r.URL.Path)
						http.Error(w, "unexpected request", http.StatusBadRequest)
					}
				}))
				defer server.Close()
				selected := filepath.Join(t.TempDir(), "selected.v2")
				writeCloudRuntimeFixture(t, selected, server.URL, selectedRelease, kind)
				writeCloudRuntimeFixture(t, filepath.Join(home, ".agents", "skills", "old-version"), server.URL, downloadableReleaseID, "owned")
				if corrupt {
					if err := os.WriteFile(filepath.Join(selected, "WORKFLOW.md"), []byte("changed"), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				runtime := &Runtime{apiBaseURL: server.URL, region: config.RegionCN, deps: Dependencies{Environment: skillcontent.Environment{Home: home}}}
				identity := scriptTrialState{ProductID: downloadableProductID, Market: "cn", InstallID: installID, Secret: skillTrialSecret, Purchase: &trialPurchaseState{ClientRequestID: "44444444-4444-4444-8444-444444444444", OrderNo: skillPurchaseOrderNo, Presented: true}}
				identityPath := scriptTrialCredentialPath(runtime, downloadableProductID)
				if kind == "purchase" {
					identity.CredentialKind = "purchase"
					identityPath = scriptPurchasePath(runtime, downloadableProductID)
				}
				if err := os.MkdirAll(filepath.Dir(identityPath), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := saveSkillPurchaseState(runtime, downloadableProductID, kind, identity); err != nil {
					t.Fatal(err)
				}
				record := cloudTaskRecord{SchemaVersion: 1, APIBaseURL: server.URL, Market: "cn", Principal: kind + ":" + installID, PaymentRequired: true,
					Input: api.SkillCloudSubmit{ProductID: downloadableProductID, ReleaseID: selectedRelease, RequestKey: "12121212-1212-4212-8212-121212121212", Prompt: "review local inputs", Facts: map[string]string{}}}
				filename := filepath.Join(cloudTaskDirectory(runtime, downloadableProductID), record.Input.RequestKey+".json")
				if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := writeCloudRecord(filename, record); err != nil {
					t.Fatal(err)
				}
				code, result, _ := executeSkillTrialCommand(t, server, home, store, "skill", "trial-purchase", downloadableProductID, "--skill-dir", selected, "--wait", "0")
				if code != 0 {
					t.Fatalf("purchase recovery: %#v", result)
				}
				tasks := result["data"].(map[string]any)["resumedTasks"].([]any)
				if len(tasks) != 1 {
					t.Fatalf("pending task was lost: %#v", result)
				}
				task := tasks[0].(map[string]any)
				if corrupt {
					if task["allowed"] != false || task["error"].(map[string]any)["code"] != "SKILL_CLOUD_RELEASE_MISMATCH" || cloudCalls.Load() != 0 {
						t.Fatalf("corrupt selected installation ran after payment: %#v", result)
					}
				} else if task["allowed"] != true || cloudCalls.Load() != 1 {
					t.Fatalf("paid task did not resume in selected installation: %#v", result)
				}
			})
		}
	}
}
