package command

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestGuidanceRegisteredPurchaseRetriesSameUserAndNeverDownloads(t *testing.T) {
	largePrompt := strings.Repeat("完整需求", 100_000)
	largeFacts := map[string]string{}
	for i := 0; i < 31; i++ {
		largeFacts[fmt.Sprintf(" %d%s ", i, strings.Repeat("名", 81))] = strings.Repeat("值", 4001)
	}
	t.Setenv(processAccessTokenEnvironment, skillPurchaseAccessToken)
	state := newSkillPurchaseTestServer(t)
	defer state.server.Close()
	original := state.server.Config.Handler
	var user atomic.Value
	user.Store(state.userID)
	var guidanceCalls atomic.Int32
	state.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/cli/auth/status" {
			writeJSONResponse(w, map[string]any{"authenticated": true, "user": map[string]any{"id": user.Load()}, "scopes": state.scopes})
			return
		}
		if r.URL.Path == "/v1/cli/skill-guidance/requests" {
			guidanceCalls.Add(1)
			var input map[string]any
			_ = json.NewDecoder(r.Body).Decode(&input)
			if input["prompt"] != largePrompt || len(input["facts"].(map[string]any)) != 31 {
				t.Error("complete large task was not preserved")
			}
			if _, hasSecret := input["secret"]; hasSecret || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer vme_cli_") {
				t.Error("registered guidance credential boundary violated")
			}
			state.mu.Lock()
			paid := state.paymentStatus == "PAID"
			state.mu.Unlock()
			if !paid {
				w.WriteHeader(403)
				writeJSONResponse(w, map[string]any{"statusCode": 403, "code": "SKILL_GUIDANCE_ENTITLEMENT_REQUIRED", "message": "purchase required", "requestId": "trace"})
				return
			}
			writeJSONResponse(w, map[string]any{"requestId": input["requestKey"], "releaseId": input["releaseId"], "version": 1, "status": "SUCCEEDED", "outcome": "ready", "instructions": "Execute this task.", "message": nil, "retryable": false, "errorCode": nil, "expiresAt": "2099-01-01T00:00:00Z"})
			return
		}
		if strings.Contains(r.URL.Path, "download") || r.URL.Path == "/artifact" {
			t.Error("registered payment downloaded a package")
		}
		original.ServeHTTP(w, r)
	})
	home := t.TempDir()
	inputPath := filepath.Join(home, "task.json")
	raw, _ := json.Marshal(map[string]any{"productId": downloadableProductID, "releaseId": downloadableReleaseID, "requestKey": "12121212-1212-4212-8212-121212121212", "prompt": largePrompt, "facts": largeFacts})
	if err := os.WriteFile(inputPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	invoke := func() (int, map[string]any) {
		code, result, _ := executeSkillPurchaseCommand(t, state.server, home, "skill", "guidance", "--input", inputPath, "--wait", "0")
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
	before := guidanceCalls.Load()
	user.Store("34343434-3434-4434-8434-343434343434")
	code, result = invoke()
	if code == 0 || result["error"].(map[string]any)["code"] != "SKILL_GUIDANCE_REQUEST_CONFLICT" || guidanceCalls.Load() != before {
		t.Fatalf("task changed user: %#v", result)
	}
}

func TestGuidanceFileTransportLimitBeforeIdentityOrNetwork(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized.json")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(guidanceMaxInputBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	command := newSkillGuidanceCommand(&Runtime{})
	command.SetArgs([]string{"--input", path})
	err = command.Execute()
	if err == nil || !strings.Contains(err.Error(), "16 MiB") {
		t.Fatalf("expected bounded file read failure: %v", err)
	}
}

func TestGuidanceInstallLostResponseCrossRunnerAndPaidRecovery(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()
	// Authored metadata is an ordinary attachment; platform metadata must not overwrite it.
	archive, err := zip.NewReader(bytes.NewReader(state.archive), int64(len(state.archive)))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for _, file := range archive.File {
		if err := writer.Copy(file); err != nil {
			t.Fatal(err)
		}
	}
	metadata, err := writer.Create("skill-package.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = metadata.Write([]byte("author custom metadata"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	state.archive = buf.Bytes()
	state.archiveDigest = fmt.Sprintf("%x", sha256Sum256ForTest(state.archive))

	home, store := t.TempDir(), securestore.NewMemory()
	var mu sync.Mutex
	tasks := map[string]map[string]any{}
	var submitted []string
	loseResponse := true
	paid := false
	deliveries, downloads, oldUses := 0, 0, 0
	original := state.server.Config.Handler
	state.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/skill-guidance/trial/requests/") && strings.HasSuffix(r.URL.Path, "/read") {
			writeJSONResponse(w, map[string]any{"requestId": "89898989-8989-4989-8989-898989898989", "releaseId": downloadableReleaseID, "version": 1, "status": "SUCCEEDED", "instructions": "Wrong task must never execute.", "message": nil, "outcome": "ready", "errorCode": nil, "retryable": false, "expiresAt": "2099-01-01T00:00:00Z"})
			return
		}
		if r.URL.Path == "/v1/skill-guidance/trial/requests" {
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
					writeJSONResponse(w, map[string]any{"statusCode": 403, "code": "SKILL_GUIDANCE_TRIAL_EXHAUSTED", "message": "quota exhausted", "requestId": "trace-guidance"})
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
				result = map[string]any{"requestId": key, "releaseId": downloadableReleaseID, "version": 1, "status": "SUCCEEDED", "instructions": instructions, "message": message, "outcome": outcome, "errorCode": nil, "retryable": false, "expiresAt": "2099-01-01T00:00:00Z", "trial": nil}
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
				body["deliveryMode"] = "PROTECTED"
			}
			if strings.HasSuffix(r.URL.Path, "/trial-purchase/download") {
				body["access"].(map[string]any)["deliveryMode"] = "PROTECTED"
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
	state.grantUses = state.trialLimit // An exhausted guidance trial still installs the short entrypoint.
	code, installed := invoke("skill", "install", downloadableProductID, "--agent", "codex")
	if code != 0 {
		t.Fatalf("guidance install: %#v", installed)
	}
	data := installed["data"].(map[string]any)
	skillPath := data["skillPath"].(string)
	authorMetadata, _ := os.ReadFile(filepath.Join(filepath.Dir(skillPath), "skill-package.json"))
	if string(authorMetadata) != "author custom metadata" {
		t.Fatal("author metadata was overwritten")
	}
	originalSkill, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(originalSkill, []byte(skillTrialGateMarker)) || bytes.Contains(originalSkill, []byte("viceme-trial-disabled")) {
		t.Fatal("guidance stub was gated or suspended")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(skillPath), skillTrialRuntimePath)); !os.IsNotExist(err) {
		t.Fatal("guidance installed legacy trial rules")
	}
	code, ready := invoke("skill", "ready", downloadableProductID, "--agent", "codex")
	if code != 0 || ready["data"].(map[string]any)["nextAction"] != "CHECK_PURCHASE_ACCESS" {
		t.Fatalf("guidance ready: %#v", ready)
	}
	if code, _ = invoke("skill", "use", downloadableProductID); code == 0 {
		t.Fatal("guidance use granted offline execution")
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
	if code, _ = invoke("skill", "guidance", "--input", inputPath, "--wait", "0"); code == 0 {
		t.Fatal("lost response falsely succeeded")
	}
	code, result := invoke("skill", "guidance", "--input", inputPath, "--wait", "0")
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
	child := exec.Command(python, data["runtimePath"].(string), "guidance", "--input", inputPath, "--wait", "0")
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
	if code, result = invoke("skill", "guidance", "--input", inputPath, "--wait", "0"); code == 0 || result["error"].(map[string]any)["code"] != "SKILL_GUIDANCE_REQUEST_CONFLICT" {
		t.Fatalf("mutated input: %#v", result)
	}
	for index, prompt := range []string{"question", "refuse"} {
		newKey := []string{"33333333-3333-4333-8333-333333333333", "44444444-4444-4444-8444-444444444444"}[index]
		writeInput(newKey, prompt)
		code, result = invoke("skill", "guidance", "--input", inputPath, "--wait", "0")
		if code != 0 || result["data"].(map[string]any)["allowed"] != false {
			t.Fatalf("nonready: %#v", result)
		}
		if _, exists := result["data"].(map[string]any)["executionPath"]; exists {
			t.Fatal("non-ready response delivered instructions")
		}
	}
	writeInput("78787878-7878-4787-8787-787878787878", "queued")
	code, result = invoke("skill", "guidance", "--input", inputPath, "--wait", "1s")
	if code == 0 || result["error"].(map[string]any)["code"] != "SKILL_GUIDANCE_RESPONSE_INVALID" {
		t.Fatalf("poll returned other task: %#v", result)
	}
	child = exec.Command(python, data["runtimePath"].(string), "guidance", "--input", inputPath, "--wait", "1")
	child.Env = append(os.Environ(), "HOME="+home, "PYTHONDONTWRITEBYTECODE=1")
	raw, err = child.CombinedOutput()
	if err == nil || !bytes.Contains(raw, []byte("SKILL_GUIDANCE_RESPONSE_INVALID")) {
		t.Fatalf("Python accepted wrong polled task: %v %s", err, raw)
	}
	paidKey := "55555555-5555-4555-8555-555555555555"
	writeInput(paidKey, "paid task")
	code, result = invoke("skill", "guidance", "--input", inputPath, "--wait", "0")
	if code == 0 || result["error"].(map[string]any)["code"] != "SKILL_PURCHASE_REQUIRED" {
		t.Fatalf("purchase: %#v", result)
	}
	// An earlier-version pending task must not block recovery of this task.
	paths, err := filepath.Glob(filepath.Join(home, ".viceme", "guidance", "*", paidKey+".json"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("pending record: %v %v", paths, err)
	}
	pending, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	var older guidanceTaskRecord
	if err := json.Unmarshal(pending, &older); err != nil {
		t.Fatal(err)
	}
	older.Input.RequestKey = "11111111-1111-4111-8111-111111111111"
	older.Input.ReleaseID = "99999999-9999-4999-8999-999999999999"
	olderPath := filepath.Join(filepath.Dir(paths[0]), older.Input.RequestKey+".json")
	if err := writeGuidanceRecord(olderPath, older); err != nil {
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
	if len(resumed) != 2 || resumed[0].(map[string]any)["error"].(map[string]any)["code"] != "SKILL_GUIDANCE_RELEASE_MISMATCH" || resumed[1].(map[string]any)["requestKey"] != paidKey || resumed[1].(map[string]any)["allowed"] != true {
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
	code, result = invoke("skill", "guidance", "--input", inputPath, "--wait", "0")
	if code == 0 || result["error"].(map[string]any)["code"] != "SKILL_GUIDANCE_RELEASE_MISMATCH" {
		t.Fatalf("old task accepted new public assets: %#v", result)
	}
}

func TestGuidanceDirectPurchaseInstallsPublicPackageWithoutTrialAndKeepsIdentity(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()
	home, store := t.TempDir(), securestore.NewMemory()
	original := state.server.Config.Handler
	var firstID, firstSecret string
	state.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/trial-grants") || strings.Contains(r.URL.Path, "/skill-guidance/trial/") {
			t.Error("direct purchase used a trial")
			w.WriteHeader(400)
			return
		}
		if r.URL.Path == "/v1/downloads/guidance/"+downloadableProductID {
			writeJSONResponse(w, map[string]any{"url": state.server.URL + "/artifact", "deliveryMode": "PROTECTED", "releaseId": downloadableReleaseID, "artifactDigest": state.archiveDigest, "expiresAt": "2099-01-01T00:00:00Z"})
			return
		}
		if r.URL.Path == "/v1/skill-guidance/purchase/requests" {
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
			writeJSONResponse(w, map[string]any{"requestId": body["requestKey"], "releaseId": downloadableReleaseID, "version": 1, "status": "SUCCEEDED", "outcome": "ready", "instructions": "Follow the local workflow.", "message": nil, "errorCode": nil, "retryable": false, "expiresAt": "2099-01-01T00:00:00Z"})
			return
		}
		recorder := httptest.NewRecorder()
		original.ServeHTTP(recorder, r)
		var body map[string]any
		if strings.HasSuffix(r.URL.Path, "/access") && json.Unmarshal(recorder.Body.Bytes(), &body) == nil {
			body["deliveryMode"], body["trial"], body["purchaseAvailable"] = "PROTECTED", nil, true
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
	if manifest["kind"] != "purchase" || manifest["deliveryMode"] != "PROTECTED" {
		t.Fatalf("wrong runtime: %#v", manifest)
	}
	task := filepath.Join(home, "task.json")
	input, _ := json.Marshal(map[string]any{"productId": downloadableProductID, "requestKey": "12121212-1212-4212-8212-121212121212", "prompt": "review local inputs", "facts": map[string]string{}})
	if err := os.WriteFile(task, input, 0o600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		code, result = invoke("skill", "guidance", "--input", task, "--agent", "codex", "--wait", "0")
		if code != 0 || result["data"].(map[string]any)["allowed"] != true {
			t.Fatalf("guidance: %#v", result)
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
func TestGuidanceTaskUsesExactSkillDirectory(t *testing.T) {
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
				if r.URL.Path != "/v1/skill-guidance/purchase/requests" {
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
				writeJSONResponse(w, map[string]any{"requestId": input["requestKey"], "releaseId": input["releaseId"], "version": 1, "status": "SUCCEEDED", "outcome": "ready", "instructions": "Follow the selected local workflow.", "message": nil, "retryable": false, "errorCode": nil, "expiresAt": "2099-01-01T00:00:00Z"})
			}))
			defer server.Close()
			workspace := t.TempDir()

			// Discovery would pick this alphabetically first installation. Its different
			// kind also detects an exact task lookup followed by a broad buyer lookup.
			writeGuidanceRuntimeFixture(t, filepath.Join(workspace, "a-old-owned"), server.URL, downloadableReleaseID, "owned")
			selected := filepath.Join(workspace, test.directory)
			if !test.missing {
				writeGuidanceRuntimeFixture(t, selected, server.URL, selectedRelease, "purchase")
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
			code, result, _ := executeSkillTrialCommand(t, server, home, store, "skill", "guidance", "--input", inputPath, "--skill-dir", selected, "--wait", "0")
			if test.allowed {
				if code != 0 || result["data"].(map[string]any)["allowed"] != true || calls.Load() != 1 {
					t.Fatalf("selected installation could not run: calls=%d result=%#v", calls.Load(), result)
				}
				if _, err := os.Stat(filepath.Join(home, ".viceme", "purchases", downloadableProductID+".json")); err != nil {
					t.Fatal(err)
				}
			} else {
				if code == 0 || result["error"].(map[string]any)["code"] != "SKILL_GUIDANCE_RELEASE_MISMATCH" || calls.Load() != 0 {
					t.Fatalf("invalid selected installation reached task or wrong error: calls=%d result=%#v", calls.Load(), result)
				}
				if _, err := os.Stat(filepath.Join(home, ".viceme", "purchases", downloadableProductID+".json")); !os.IsNotExist(err) {
					t.Fatalf("invalid installation created a purchase identity: %v", err)
				}
			}
		})
	}
}

func writeGuidanceRuntimeFixture(t *testing.T, directory, apiBaseURL, release, kind string) {
	t.Helper()
	files := map[string]downloadableSkillFile{
		"SKILL.md":    {Data: []byte("---\nname: guidance-fixture\n---\nUse the public workflow.\n"), Mode: 0o644},
		"WORKFLOW.md": {Data: []byte("Read local input and apply this task's guidance.\n"), Mode: 0o644},
	}
	runtime := &Runtime{apiBaseURL: apiBaseURL, region: config.RegionCN}
	if err := addSkillRuntime(runtime, files, downloadableProductID, release, kind, "PROTECTED"); err != nil {
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

func TestGuidancePurchaseRecoveryUsesExactSkillDirectory(t *testing.T) {
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
				var guidanceCalls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/v1/skills/" + downloadableProductID + "/" + endpoint + "/status":
						writeJSONResponse(w, map[string]any{"productId": downloadableProductID, "orderNo": skillPurchaseOrderNo, "title": "Guidance fixture", "status": "PAID", "amountCents": 100, "currency": "CNY", "expiresAt": "2099-01-01T00:00:00Z", "paymentAction": nil})
					case "/v1/skills/" + downloadableProductID + "/" + endpoint + "/download":
						writeJSONResponse(w, map[string]any{"access": map[string]any{"productId": downloadableProductID, "owned": true, "installKind": "OWNED_PAID", "deliveryMode": "PROTECTED", "release": map[string]any{"id": selectedRelease}}, "download": nil})
					case "/v1/skill-guidance/" + kind + "/requests":
						guidanceCalls.Add(1)
						var input map[string]any
						if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
							t.Error(err)
							return
						}
						if input["releaseId"] != selectedRelease || input["installId"] != installID {
							t.Error("purchase recovery changed the installation or buyer identity")
						}
						writeJSONResponse(w, map[string]any{"requestId": input["requestKey"], "releaseId": input["releaseId"], "version": 1, "status": "SUCCEEDED", "outcome": "ready", "instructions": "Follow the selected local workflow.", "message": nil, "retryable": false, "errorCode": nil, "expiresAt": "2099-01-01T00:00:00Z"})
					default:
						t.Errorf("unexpected purchase recovery request: %s", r.URL.Path)
						http.Error(w, "unexpected request", http.StatusBadRequest)
					}
				}))
				defer server.Close()
				selected := filepath.Join(t.TempDir(), "selected.v2")
				writeGuidanceRuntimeFixture(t, selected, server.URL, selectedRelease, kind)
				writeGuidanceRuntimeFixture(t, filepath.Join(home, ".agents", "skills", "old-version"), server.URL, downloadableReleaseID, "owned")
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
				record := guidanceTaskRecord{SchemaVersion: 1, APIBaseURL: server.URL, Market: "cn", Principal: kind + ":" + installID, PaymentRequired: true,
					Input: api.SkillGuidanceSubmit{ProductID: downloadableProductID, ReleaseID: selectedRelease, RequestKey: "12121212-1212-4212-8212-121212121212", Prompt: "review local inputs", Facts: map[string]string{}}}
				filename := filepath.Join(guidanceTaskDirectory(runtime, downloadableProductID), record.Input.RequestKey+".json")
				if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := writeGuidanceRecord(filename, record); err != nil {
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
					if task["allowed"] != false || task["error"].(map[string]any)["code"] != "SKILL_GUIDANCE_RELEASE_MISMATCH" || guidanceCalls.Load() != 0 {
						t.Fatalf("corrupt selected installation ran after payment: %#v", result)
					}
				} else if task["allowed"] != true || guidanceCalls.Load() != 1 {
					t.Fatalf("paid task did not resume in selected installation: %#v", result)
				}
			})
		}
	}
}

func TestGuidanceExistingAnonymousIdentitySurvivesRevokedStoredLogin(t *testing.T) {
	for _, kind := range []string{"trial", "purchase"} {
		for _, scenario := range []struct {
			name      string
			status    int
			override  bool
			boundUser bool
			allowed   bool
		}{
			{name: "revoked", status: 401, allowed: true},
			{name: "not-authenticated", status: 200, allowed: true},
			{name: "outage", status: 503},
			{name: "explicit-credential", status: 401, override: true},
			{name: "bound-user", status: 401, boundUser: true},
		} {
			t.Run(kind+"/"+scenario.name, func(t *testing.T) {
				runtime, input, calls, cleanup := guidanceIdentityFixture(t, kind, scenario.status)
				defer cleanup()
				if scenario.override {
					runtime.processCredential = &publicationCredential{raw: skillPurchaseAccessToken}
				}
				if scenario.boundUser {
					if _, _, err := prepareGuidanceRecord(runtime, input, "user:22222222-2222-4222-8222-222222222222"); err != nil {
						t.Fatal(err)
					}
				}
				result, err := runGuidanceTask(context.Background(), runtime, input, 0)
				if scenario.allowed {
					if err != nil || result["allowed"] != true || calls.Load() != 1 {
						t.Fatalf("anonymous access blocked: %v %#v calls=%d", err, result, calls.Load())
					}
				} else if err == nil || calls.Load() != 0 {
					t.Fatalf("identity changed on an uncertain/explicit account failure: %v %#v calls=%d", err, result, calls.Load())
				}
			})
		}
	}
}

// Replaying the same immutable request across runners preserves its principal.
func TestGuidanceRequestReplayRetainsCrossRunnerPrincipal(t *testing.T) {
	for _, kind := range []string{"trial", "purchase"} {
		t.Run(kind, func(t *testing.T) {
			runtime, input, calls, cleanup := guidanceIdentityFixture(t, kind, 401)
			defer cleanup()
			result, err := runGuidanceTask(context.Background(), runtime, input, 0)
			if err != nil {
				t.Fatal(err)
			}
			// A separate Python process replays the original Go request.
			python, err := exec.LookPath("python3")
			if err != nil {
				t.Fatal(err)
			}
			module, err := filepath.Abs("../../skills/use-a-skill/scripts/trial_runtime.py")
			if err != nil {
				t.Fatal(err)
			}
			inputPath := filepath.Join(runtime.deps.Environment.Home, "followup.json")
			raw, _ := json.Marshal(input)
			if err := os.WriteFile(inputPath, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			child := exec.Command(python, "-c", "import importlib.util,sys; spec=importlib.util.spec_from_file_location('runtime',sys.argv[1]); m=importlib.util.module_from_spec(spec); spec.loader.exec_module(m); m.API_ORIGIN['cn']=sys.argv[2]; m.guidance_cli_fallback=lambda *a,**kw: None; sys.exit(m.run(['guidance','--product',sys.argv[3],'--input',sys.argv[4]]))", module, runtime.apiBaseURL, input.ProductID, inputPath)
			child.Env = append(os.Environ(), "HOME="+runtime.deps.Environment.Home, "PATH=/usr/bin:/bin", "VICEME_INSTALL_DIR="+filepath.Join(runtime.deps.Environment.Home, "no-cli"), "VICEME_ACCESS_TOKEN=", "PYTHONDONTWRITEBYTECODE=1")
			pythonOutput, err := child.CombinedOutput()
			var pythonResult map[string]any
			if err != nil || json.Unmarshal(pythonOutput, &pythonResult) != nil || pythonResult["allowed"] != true {
				t.Fatalf("cross-runner request replay failed: %v %s", err, pythonOutput)
			}
			// The stored login now works and owns a different purchase. Replaying
			// this request must use its existing anonymous credential without asking it.
			original := runtime.deps.HTTPClient.Transport
			runtime.deps.HTTPClient.Transport = guidanceTestTransport(func(request *http.Request) (*http.Response, error) {
				if request.URL.Path == "/v1/cli/auth/status" {
					t.Error("replayed request reselected its account")
				}
				if original != nil {
					return original.RoundTrip(request)
				}
				return http.DefaultTransport.RoundTrip(request)
			})
			result, err = runGuidanceTask(context.Background(), runtime, input, 0)
			if err != nil || result["allowed"] != true || calls.Load() != 3 {
				t.Fatalf("request identity changed: %v %#v", err, result)
			}

		})
	}
}

func TestGuidanceRequestDoesNotChangeAccount(t *testing.T) {
	runtime, input, calls, cleanup := guidanceIdentityFixture(t, "purchase", 200)
	defer cleanup()
	prior := input
	record, filename, err := prepareGuidanceRecord(runtime, prior, "user:22222222-2222-4222-8222-222222222222")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeGuidanceRecord(filename, record); err != nil {
		t.Fatal(err)
	}
	_, err = runGuidanceTask(context.Background(), runtime, input, 0)
	if err == nil || output.AsError(err).Type != "authentication" || calls.Load() != 0 {
		t.Fatalf("user request fell back to anonymous identity: %v", err)
	}
	runtime.deps.HTTPClient.Transport = guidanceTestTransport(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/v1/cli/auth/status" {
			t.Errorf("different account must not submit a saved task: %s", request.URL.Path)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"authenticated":true,"user":{"id":"77777777-7777-4777-8777-777777777777"}}`))}, nil
	})
	_, err = runGuidanceTask(context.Background(), runtime, input, 0)
	if err == nil || output.AsError(err).Subtype != "SKILL_GUIDANCE_REQUEST_CONFLICT" || calls.Load() != 0 {
		t.Fatalf("replayed request changed accounts: %v", err)
	}
}

func guidanceIdentityFixture(t *testing.T, kind string, authStatus int) (*Runtime, api.SkillGuidanceSubmit, *atomic.Int32, func()) {
	t.Helper()
	calls := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/cli/auth/status" {
			w.WriteHeader(authStatus)
			if authStatus == 200 {
				writeJSONResponse(w, map[string]any{"authenticated": false})
				return
			}
			writeJSONResponse(w, map[string]any{"statusCode": authStatus, "code": "ACCOUNT_UNAVAILABLE", "message": "test account unavailable", "requestId": "test"})
			return
		}
		if r.URL.Path != "/v1/skill-guidance/"+kind+"/requests" {
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(500)
			return
		}
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["installId"] != "44444444-4444-4444-8444-444444444444" || body["secret"] != strings.Repeat("01", 32) || r.Header.Get("Authorization") != "" {
			t.Error("anonymous credential changed")
		}
		calls.Add(1)
		writeJSONResponse(w, map[string]any{"requestId": body["requestKey"], "releaseId": downloadableReleaseID, "version": 1, "status": "SUCCEEDED", "outcome": "ready", "instructions": "Execute the selected task.", "message": nil, "errorCode": nil, "retryable": false, "expiresAt": "2099-01-01T00:00:00Z"})
	}))
	home, store := t.TempDir(), securestore.NewMemory()
	scope, err := credentialScopeForAPIBase(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &Runtime{deps: Dependencies{Store: store, HTTPClient: server.Client(), Now: time.Now, Environment: skillcontent.Environment{Home: home, CodexHome: filepath.Join(home, ".codex"), ConfigDir: filepath.Join(home, ".viceme-cli")}}, apiBaseURL: server.URL, region: config.RegionCN, credentialScope: scope}
	if err := runtime.manager().Save(auth.Credential{AccessToken: skillPurchaseAccessToken}); err != nil {
		t.Fatal(err)
	}
	state := scriptTrialState{ProductID: downloadableProductID, Market: "cn", InstallID: "44444444-4444-4444-8444-444444444444", Secret: strings.Repeat("01", 32)}
	filename := scriptTrialCredentialPath(runtime, downloadableProductID)
	if kind == "purchase" {
		state.CredentialKind = "purchase"
		filename = scriptPurchasePath(runtime, downloadableProductID)
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := saveScriptStateAt(filename, state); err != nil {
		t.Fatal(err)
	}
	input := api.SkillGuidanceSubmit{ProductID: downloadableProductID, ReleaseID: downloadableReleaseID, RequestKey: "55555555-5555-4555-8555-555555555555", Prompt: "review inputs", Facts: map[string]string{}}
	return runtime, input, calls, server.Close
}

type guidanceTestTransport func(*http.Request) (*http.Response, error)

func (f guidanceTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGuidanceRequestWithDifferentAccountPurchaseDoesNotRepurchase(t *testing.T) {
	for _, stage := range []string{"accepted-request", "rejected-request"} {
		t.Run(stage, func(t *testing.T) {
			runtime, input, calls, cleanup := guidanceIdentityFixture(t, "purchase", 200)
			defer cleanup()
			runtime.deps.NewID = func() string { return "88888888-8888-4888-8888-888888888888" }
			prior := input
			record, filename, err := prepareGuidanceRecord(runtime, prior, "purchase:44444444-4444-4444-8444-444444444444")
			if err != nil {
				t.Fatal(err)
			}
			if err := writeGuidanceRecord(filename, record); err != nil {
				t.Fatal(err)
			}
			runtime.deps.HTTPClient.Transport = guidanceTestTransport(func(request *http.Request) (*http.Response, error) {
				status, body := 200, `{"authenticated":true,"user":{"id":"22222222-2222-4222-8222-222222222222"}}`
				switch request.URL.Path {
				case "/v1/cli/auth/status":
				case "/v1/cli/skills/" + input.ProductID + "/access":
					body = `{"owned":true}`
				case "/v1/skill-guidance/purchase/requests":
					calls.Add(1)
					status, body = 403, `{"statusCode":403,"code":"SKILL_GUIDANCE_ENTITLEMENT_REQUIRED","message":"original identity has no access","requestId":"test"}`
				default:
					t.Errorf("must not create a second purchase: %s", request.URL.Path)
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			inputPath := filepath.Join(runtime.deps.Environment.Home, "task.json")
			raw, _ := json.Marshal(input)
			if err := os.WriteFile(inputPath, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			command := newSkillGuidanceCommand(runtime)
			command.SilenceErrors, command.SilenceUsage = true, true
			command.SetArgs([]string{"--input", inputPath, "--wait", "0"})
			err = command.Execute()
			if err == nil || output.AsError(err).Subtype != "SKILL_GUIDANCE_REQUEST_ENTITLEMENT_REQUIRED" || calls.Load() != 1 {
				t.Fatalf("wrong request purchase transition: %v calls=%d", err, calls.Load())
			}
		})
	}
}

func TestGuidancePurchaseBeforeTaskUsesAccountOrCreatesDirectPurchaseIdentity(t *testing.T) {
	for _, owned := range []bool{false, true} {
		t.Run(fmt.Sprint(owned), func(t *testing.T) {
			t.Setenv(processAccessTokenEnvironment, "")
			if owned {
				t.Setenv(processAccessTokenEnvironment, skillPurchaseAccessToken)
			}
			home, store := t.TempDir(), securestore.NewMemory()
			var purchases, guidance int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/cli/skills/" + downloadableProductID + "/access":
					access := skillAccessFixture(false, owned, strings.Repeat("a", 64), "")
					access["deliveryMode"] = "PROTECTED"
					writeJSONResponse(w, access)
				case "/v1/skills/" + downloadableProductID + "/purchase":
					purchases++
					var body map[string]any
					_ = json.NewDecoder(r.Body).Decode(&body)
					if body["installId"] == "" || body["secret"] == "" {
						t.Error("missing purchase identity")
					}
					writeJSONResponse(w, map[string]any{"productId": downloadableProductID, "orderNo": skillPurchaseOrderNo, "title": "Protected", "status": "PAID", "amountCents": 100, "currency": "CNY", "expiresAt": "2099-01-01T00:00:00Z", "paymentAction": nil})
				case "/v1/skills/" + downloadableProductID + "/purchase/download":
					writeJSONResponse(w, map[string]any{"access": map[string]any{"productId": downloadableProductID, "owned": true, "installKind": "OWNED_PAID", "deliveryMode": "PROTECTED", "release": map[string]any{"id": downloadableReleaseID}}, "download": nil})
				default:
					if strings.Contains(r.URL.Path, "guidance") {
						guidance++
					}
					t.Errorf("unexpected purchase request: %s", r.URL.Path)
					http.Error(w, "unexpected", 400)
				}
			}))
			defer server.Close()
			selected := filepath.Join(t.TempDir(), "purchase-entry")
			writeGuidanceRuntimeFixture(t, selected, server.URL, downloadableReleaseID, "purchase")
			code, result, _ := executeSkillTrialCommand(t, server, home, store, "skill", "trial-purchase", downloadableProductID, "--skill-dir", selected, "--market", "cn", "--wait", "0")
			if code != 0 || result["data"].(map[string]any)["owned"] != true {
				t.Fatalf("purchase before task: %#v", result)
			}
			if guidance != 0 || (owned && purchases != 0) || (!owned && purchases != 1) {
				t.Fatalf("purchase=%d guidance=%d", purchases, guidance)
			}
			_, err := os.Stat(filepath.Join(home, ".viceme", "purchases", downloadableProductID+".json"))
			if owned && !os.IsNotExist(err) {
				t.Fatal("account owner created anonymous identity")
			}
			if !owned && err != nil {
				t.Fatal("direct purchase identity not persisted")
			}
		})
	}
}

func TestGuidancePurchaseRecoveryWithRefusedStoredLogin(t *testing.T) {
	for _, kind := range []string{"purchase", "trial"} {
		for _, scenario := range []struct {
			name     string
			status   int
			override bool
			missing  bool
			allowed  bool
		}{
			{"stored-refused", 401, false, false, true},
			{"explicit-refused", 401, true, false, false},
			{"service-unavailable", 503, false, false, false},
			{"no-existing-identity", 401, false, true, false},
		} {
			t.Run(kind+"/"+scenario.name, func(t *testing.T) {
				t.Setenv(processAccessTokenEnvironment, "")
				if scenario.override {
					t.Setenv(processAccessTokenEnvironment, skillPurchaseAccessToken)
				}
				runtime, input, _, cleanup := guidanceIdentityFixture(t, kind, 200)
				defer cleanup()
				if scenario.override {
					runtime.processCredential = &publicationCredential{raw: skillPurchaseAccessToken}
				}
				var stdout bytes.Buffer
				runtime.printer = &output.Printer{Out: &stdout, ErrOut: io.Discard}
				runtime.deps.NewID = func() string { return "88888888-8888-4888-8888-888888888888" }
				identityPath := scriptPurchasePath(runtime, input.ProductID)
				if kind == "trial" {
					identityPath = scriptTrialCredentialPath(runtime, input.ProductID)
				}
				if scenario.missing {
					if err := os.Remove(identityPath); err != nil {
						t.Fatal(err)
					}
				}
				selected := filepath.Join(t.TempDir(), "installed")
				writeGuidanceRuntimeFixture(t, selected, runtime.apiBaseURL, input.ReleaseID, kind)
				purchases := 0
				runtime.deps.HTTPClient.Transport = guidanceTestTransport(func(request *http.Request) (*http.Response, error) {
					status, body := 200, ""
					switch request.URL.Path {
					case "/v1/cli/skills/" + input.ProductID + "/access":
						status = scenario.status
						body = fmt.Sprintf(`{"statusCode":%d,"code":"ACCOUNT_UNAVAILABLE","message":"account unavailable","requestId":"test"}`, status)
					case "/v1/skills/" + input.ProductID + "/trial-purchase", "/v1/skills/" + input.ProductID + "/purchase":
						purchases++
						var purchase map[string]any
						_ = json.NewDecoder(request.Body).Decode(&purchase)
						if purchase["installId"] != "44444444-4444-4444-8444-444444444444" || purchase["secret"] != strings.Repeat("01", 32) || request.Header.Get("Authorization") != "" {
							t.Error("purchase recovery did not preserve anonymous identity")
						}
						body = `{"productId":"` + input.ProductID + `","orderNo":"` + skillPurchaseOrderNo + `","title":"Protected","status":"PAID","amountCents":100,"currency":"CNY","expiresAt":"2099-01-01T00:00:00Z","paymentAction":null}`
					case "/v1/skills/" + input.ProductID + "/trial-purchase/download", "/v1/skills/" + input.ProductID + "/purchase/download":
						body = `{"access":{"productId":"` + input.ProductID + `","owned":true,"installKind":"OWNED_PAID","deliveryMode":"PROTECTED","release":{"id":"` + input.ReleaseID + `"}},"download":null}`
					default:
						t.Errorf("unexpected recovery request: %s", request.URL.Path)
						status, body = 500, `{}`
					}
					return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
				})
				err := runTrialPurchase(context.Background(), runtime, input.ProductID, 0, "auto", selected)
				if scenario.allowed {
					var envelope struct {
						Data struct {
							Owned bool `json:"owned"`
						} `json:"data"`
					}
					if err != nil || purchases != 1 || json.Unmarshal(stdout.Bytes(), &envelope) != nil || !envelope.Data.Owned {
						t.Fatalf("existing anonymous recovery failed: %v calls=%d output=%s", err, purchases, stdout.String())
					}
				} else if err == nil || purchases != 0 {
					t.Fatalf("unexpected anonymous fallback: %v calls=%d", err, purchases)
				}
				if scenario.missing {
					if _, err := os.Stat(identityPath); !os.IsNotExist(err) {
						t.Fatal("refused login created a new anonymous identity")
					}
				}
			})
		}
	}
}

func TestGuidancePurchaseResumeWithDifferentAccountRequestsNewCompleteInput(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	runtime, input, _, cleanup := guidanceIdentityFixture(t, "purchase", 200)
	defer cleanup()
	record, filename, err := prepareGuidanceRecord(runtime, input, "purchase:44444444-4444-4444-8444-444444444444")
	if err != nil {
		t.Fatal(err)
	}
	record.PaymentRequired = true
	if err := writeGuidanceRecord(filename, record); err != nil {
		t.Fatal(err)
	}
	runtime.deps.HTTPClient.Transport = guidanceTestTransport(func(request *http.Request) (*http.Response, error) {
		status, body := 200, `{"authenticated":true,"user":{"id":"22222222-2222-4222-8222-222222222222"}}`
		switch request.URL.Path {
		case "/v1/cli/auth/status":
		case "/v1/cli/skills/" + input.ProductID + "/access":
			body = `{"owned":true}`
		case "/v1/skill-guidance/purchase/requests":
			status, body = 403, `{"statusCode":403,"code":"SKILL_GUIDANCE_ENTITLEMENT_REQUIRED","message":"original identity has no access","requestId":"test"}`
		default:
			t.Errorf("must not purchase or transfer the old request: %s", request.URL.Path)
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	result, err := resumeGuidanceAfterPurchase(context.Background(), runtime, input.ProductID)
	if err != nil {
		t.Fatal(err)
	}
	resumed := result["resumedTasks"].([]map[string]any)
	if len(resumed) != 1 || resumed[0]["nextAction"] != "SUBMIT_COMPLETE_INPUT_WITH_NEW_KEY" || resumed[0]["error"].(map[string]any)["code"] != "SKILL_GUIDANCE_REQUEST_ENTITLEMENT_REQUIRED" || resumed[0]["error"].(map[string]any)["hint"] == "" {
		t.Fatalf("invalid account purchase recovery: %#v", result)
	}
	raw, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	var after guidanceTaskRecord
	if json.Unmarshal(raw, &after) != nil || after.Principal != record.Principal || after.Input.RequestKey != input.RequestKey || !after.PaymentRequired {
		t.Fatal("original anonymous request was changed or lost")
	}
}
