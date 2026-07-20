package command

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestDefaultRegionUsesChinaEndpoint(t *testing.T) {
	t.Setenv("VICEME_API_BASE_URL", "")
	_, runtime, err := NewRoot(Dependencies{
		Store:       securestore.NewMemory(),
		Environment: skillcontent.Environment{Home: t.TempDir(), ConfigDir: t.TempDir()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.region != config.RegionCN || runtime.apiBaseURL != "https://api.viceme.cn" {
		t.Fatalf("region=%q API base URL=%q", runtime.region, runtime.apiBaseURL)
	}
}

func TestAPIBaseURLEnvironmentOverride(t *testing.T) {
	t.Setenv("VICEME_API_BASE_URL", "http://localhost:3000")
	_, runtime, err := NewRoot(Dependencies{
		Store:       securestore.NewMemory(),
		Environment: skillcontent.Environment{Home: t.TempDir(), ConfigDir: t.TempDir()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.apiBaseURL != "http://localhost:3000" {
		t.Fatalf("environment API base URL = %q", runtime.apiBaseURL)
	}
}

func TestStoredGlobalRegionUsesGlobalEndpoint(t *testing.T) {
	t.Setenv("VICEME_API_BASE_URL", "")
	configBase := t.TempDir()
	if _, err := config.Save(configBase, config.Default(config.RegionGlobal)); err != nil {
		t.Fatal(err)
	}
	_, runtime, err := NewRoot(Dependencies{
		Store:       securestore.NewMemory(),
		Environment: skillcontent.Environment{Home: t.TempDir(), ConfigDir: configBase},
	})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.region != config.RegionGlobal || runtime.apiBaseURL != "https://api.viceme.ai" {
		t.Fatalf("region=%q API base URL=%q", runtime.region, runtime.apiBaseURL)
	}
}

func TestVersionJSONEnvelope(t *testing.T) {
	t.Parallel()
	code, stdout, stderr, _ := runCLI(t, nil, nil, "--version")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope["ok"] != true {
		t.Fatalf("unexpected envelope: %#v", envelope)
	}
	meta := envelope["meta"].(map[string]any)
	if meta["full_skill_bundle_digest"] == "" || meta["embedded_content_digest"] == "" {
		t.Fatalf("missing digest metadata: %#v", meta)
	}
}

func TestPublishRequiresConfirmationAndExplicitUploadTarget(t *testing.T) {
	t.Parallel()
	code, _, stderr, _ := runCLI(t, nil, nil, "skill", "publish", "https://github.com/acme/skill")
	if code != 10 || !strings.Contains(stderr, "confirmation_required") {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	code, _, stderr, _ = runCLI(t, nil, nil, "skill", "publish", "--file", "missing.zip", "--yes")
	if code != 2 || !strings.Contains(stderr, "upload_target") {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	code, stdout, stderr, _ := runCLI(t, nil, nil, "skill", "publish", "--file", "missing.zip", "--new-target", "--yes", "--dry-run")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"source_mode":"file"`) ||
		!strings.Contains(stdout, `"publish_mode":"confirm"`) ||
		!strings.Contains(stdout, `"confirmation_scope":"publication_admission/v1"`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	code, _, stderr, _ = runCLI(t, nil, nil, "skill", "publish", "https://github.com/acme/skill", "--target-id", "target_1", "--yes")
	if code != 2 || !strings.Contains(stderr, "target_version") {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	code, stdout, stderr, _ = runCLI(t, nil, nil, "skill", "publish", "https://github.com/acme/skill", "--target-id", "target_1", "--expected-target-version", "4", "--yes", "--dry-run")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"expected_target_version":4`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestAuthNoWaitJSONNeverReturnsToken(t *testing.T) {
	t.Parallel()
	completeURL := "https://viceme.test/cli/auth?user_code=ABCD-EFGH"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/cli/auth/device" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		_, _ = io.WriteString(writer, `{"verification_url":"https://viceme.test/cli/auth","verification_url_complete":"`+completeURL+`","device_code":"device-public","user_code":"ABCD-EFGH","expires_at":"2030-01-01T00:00:00Z","interval_seconds":2}`)
	}))
	defer server.Close()
	code, stdout, stderr, _ := runCLI(t, server, nil, "auth", "login", "--no-wait", "--json")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "device-public") || !strings.Contains(stdout, `"profile":"default"`) || !strings.Contains(stdout, `"region":"cn"`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if strings.Contains(stdout, "access_token") || strings.Contains(stderr, "access_token") {
		t.Fatal("login start leaked an access token field")
	}
	var envelope struct {
		Data struct {
			VerificationURL         string `json:"verification_url"`
			VerificationURLComplete string `json:"verification_url_complete"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.VerificationURL != completeURL || envelope.Data.VerificationURLComplete != completeURL {
		t.Fatalf("CLI did not return the direct browser URL: %#v", envelope.Data)
	}
}

func TestAuthLoginGuidesHumanAndWaitsForAuthorization(t *testing.T) {
	t.Parallel()
	completeURL := "https://viceme.test/cli/auth?user_code=ABCD-EFGH"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/cli/auth/device":
			_, _ = io.WriteString(writer, `{"verification_url":"https://viceme.test/cli/auth","verification_url_complete":"`+completeURL+`","device_code":"device-public","user_code":"ABCD-EFGH","expires_at":"2030-01-01T00:00:00Z","interval_seconds":0}`)
		case "/v1/cli/auth/token":
			_, _ = io.WriteString(writer, `{"access_token":"top-secret","refresh_token":"refresh-secret","user_id":"user_1","expires_at":"2030-01-01T00:00:00Z"}`)
		default:
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
	}))
	defer server.Close()
	store := securestore.NewMemory()
	code, stdout, stderr, _ := runCLI(t, server, store, "auth", "login")
	if code != 0 || stdout != "" {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	for _, expected := range []string{completeURL, "ABCD-EFGH", "Waiting for authorization...", "Authorization successful.", "Profile: default", "Region: cn"} {
		if !strings.Contains(stderr, expected) {
			t.Fatalf("human login output lacks %q: %s", expected, stderr)
		}
	}
	for _, secret := range []string{"device-public", "top-secret", "refresh-secret", `{"ok":`} {
		if strings.Contains(stderr, secret) {
			t.Fatalf("human login leaked %q: %s", secret, stderr)
		}
	}
	manager := &credentialauth.Manager{Store: store, Region: "cn"}
	credential, err := manager.Load()
	if err != nil || credential.AccessToken != "top-secret" {
		t.Fatalf("credential was not stored: credential=%#v err=%v", credential, err)
	}
}

func TestAgentLoginFlowRequiresJSON(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args    []string
		subtype string
	}{
		{args: []string{"auth", "login", "--no-wait"}, subtype: "auth_json_required"},
		{args: []string{"auth", "login", "--device-code", "device-public"}, subtype: "auth_json_required"},
		{args: []string{"auth", "login", "--json"}, subtype: "auth_json_flow"},
	} {
		code, stdout, stderr, _ := runCLI(t, nil, nil, test.args...)
		if code != 2 || stdout != "" || !strings.Contains(stderr, `"subtype":"`+test.subtype+`"`) {
			t.Fatalf("args=%v code=%d stdout=%s stderr=%s", test.args, code, stdout, stderr)
		}
	}
}

func TestDeviceLoginStoresTokenButDoesNotPrintIt(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/cli/auth/token" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		_, _ = io.WriteString(writer, `{"access_token":"top-secret","refresh_token":"refresh-secret","user_id":"user_1","expires_at":"2030-01-01T00:00:00Z"}`)
	}))
	defer server.Close()
	store := securestore.NewMemory()
	code, stdout, stderr, _ := runCLI(t, server, store, "auth", "login", "--device-code", "device-public", "--json")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if strings.Contains(stdout, "top-secret") || strings.Contains(stdout, "refresh-secret") {
		t.Fatal("completed login leaked credentials")
	}
	manager := &credentialauth.Manager{Store: store, Region: "cn"}
	credential, err := manager.Load()
	if err != nil {
		t.Fatal(err)
	}
	if credential.AccessToken != "top-secret" {
		t.Fatal("credential was not stored")
	}
}

func TestInspectAndPublishRequestContracts(t *testing.T) {
	t.Parallel()
	var inspected atomic.Bool
	var published atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("x-api-key") != "test-token" {
			t.Fatalf("missing API key: %q", request.Header.Get("x-api-key"))
		}
		switch request.URL.Path {
		case "/v1/skill-agent-publications/inspect":
			var body map[string]any
			_ = json.NewDecoder(request.Body).Decode(&body)
			source := body["source"].(map[string]any)
			if source["kind"] != "expression" || source["value"] != "copied expression" {
				t.Fatalf("unexpected inspect body: %#v", body)
			}
			inspected.Store(true)
			_, _ = io.WriteString(writer, `{"resolution_id":"res_1","destination":{"mode":"new_auto"}}`)
		case "/v1/skill-agent-publications":
			var body map[string]any
			_ = json.NewDecoder(request.Body).Decode(&body)
			if body["client_request_id"] != "request-fixed" {
				t.Fatalf("unexpected request id: %#v", body)
			}
			destination := body["destination"].(map[string]any)
			if destination["mode"] != "auto" {
				t.Fatalf("unexpected destination: %#v", destination)
			}
			options := body["options"].(map[string]any)
			if options["publish_mode"] != "confirm" || options["admission_confirmation"] != true {
				t.Fatalf("unexpected publication admission: %#v", options)
			}
			published.Store(true)
			writer.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(writer, `{"publication_id":"pub_1","status":"received","status_url":"/v1/skill-agent-publications/pub_1"}`)
		default:
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
	}))
	defer server.Close()
	store := authenticatedStore(t)
	code, stdout, stderr, _ := runCLIWithInput(t, server, store, "copied expression", "skill", "inspect", "--expression-stdin")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "res_1") || !inspected.Load() {
		t.Fatalf("inspect code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runCLI(t, server, store, "skill", "publish", "https://github.com/acme/skill", "--yes")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "pub_1") || !published.Load() {
		t.Fatalf("publish code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestPublishRetriesAmbiguousTransportWithSameRequestID(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return nil, fmt.Errorf("connection closed after write")
		}
		var body map[string]any
		_ = json.NewDecoder(request.Body).Decode(&body)
		if body["client_request_id"] != "request-fixed" {
			t.Fatalf("retry changed request id: %#v", body)
		}
		options := body["options"].(map[string]any)
		if options["publish_mode"] != "confirm" || options["admission_confirmation"] != true {
			t.Fatalf("retry changed publication admission: %#v", options)
		}
		return &http.Response{
			StatusCode: http.StatusAccepted,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"publication_id":"pub_retry","status":"received"}`)),
			Request:    request,
		}, nil
	})
	code, stdout, stderr, _ := runCLIWithDependencies(t, nil, authenticatedStore(t), "", Dependencies{
		HTTPClient: &http.Client{Transport: transport},
		APIBaseURL: "https://api.viceme.test",
	}, "skill", "publish", "https://github.com/acme/skill", "--yes")
	if code != 0 || stderr != "" || calls.Load() != 2 || !strings.Contains(stdout, "pub_retry") {
		t.Fatalf("code=%d calls=%d stdout=%s stderr=%s", code, calls.Load(), stdout, stderr)
	}
}

func TestJobWaitReturnsBusinessFailureWithExitZero(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/skill-agent-publications/pub_1" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		if calls.Add(1) == 1 {
			_, _ = io.WriteString(writer, `{"publication_id":"pub_1","status":"compiling"}`)
			return
		}
		_, _ = io.WriteString(writer, `{"publication_id":"pub_1","status":"unsupported","failure":{"code":"UNSUPPORTED"}}`)
	}))
	defer server.Close()
	code, stdout, stderr, _ := runCLI(t, server, authenticatedStore(t), "job", "wait", "pub_1")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"status":"unsupported"`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestJobMetadataReadAndResolveContract(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/v1/skill-agent-publications/pub_1/metadata" && request.Method == http.MethodGet:
			_, _ = io.WriteString(writer, `{"publication_id":"pub_1","status":"meta_review","title":"海报文案","description":"为产品海报写文案","author":"acme/poster","missing":[],"action_id":"meta_1","expires_at":"2030-01-01T00:00:00Z"}`)
		case request.URL.Path == "/v1/skill-agent-publications/pub_1/metadata/resolve" && request.Method == http.MethodPost:
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatalf("decode resolve body: %v", err)
			}
			if body["action_id"] != "meta_1" || body["decision"] != "confirm" || body["title"] != "探针海报" ||
				body["expected_payload_digest"] != "sha256:payload" {
				t.Fatalf("metadata resolve body = %#v", body)
			}
			_, _ = io.WriteString(writer, `{"action_id":"meta_1","status":"resolved","publication_status":"meta_confirmed","resolution_digest":"sha256:resolution","resolved_at":"2026-07-20T00:00:00Z"}`)
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()
	code, stdout, stderr, _ := runCLI(t, server, authenticatedStore(t), "job", "metadata", "pub_1")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"author":"acme/poster"`) || !strings.Contains(stdout, `"status":"meta_review"`) {
		t.Fatalf("metadata read: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runCLI(t, server, authenticatedStore(t),
		"job", "metadata", "pub_1",
		"--action-id", "meta_1",
		"--expected-payload-digest", "sha256:payload",
		"--decision", "confirm",
		"--title", "探针海报",
	)
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"publication_status":"meta_confirmed"`) {
		t.Fatalf("metadata resolve: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestJobMetadataDecisionValidation(t *testing.T) {
	t.Parallel()
	code, _, stderr, _ := runCLI(t, nil, authenticatedStore(t),
		"job", "metadata", "pub_1", "--decision", "maybe")
	if code != 2 || !strings.Contains(stderr, "metadata_decision") {
		t.Fatalf("invalid decision: code=%d stderr=%s", code, stderr)
	}
	code, _, stderr, _ = runCLI(t, nil, authenticatedStore(t),
		"job", "metadata", "pub_1", "--decision", "confirm")
	if code != 2 || !strings.Contains(stderr, "metadata_flags") {
		t.Fatalf("missing action flags: code=%d stderr=%s", code, stderr)
	}
}

func TestSkillsInstallAndDoctorCommands(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	code, stdout, stderr, _ := runCLIWithDependencies(t, nil, nil, "", Dependencies{
		Environment: skillcontent.Environment{Home: home},
	}, "skills", "install", "--target", "codex")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"all_succeeded":true`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runCLIWithDependencies(t, nil, nil, "", Dependencies{
		Environment: skillcontent.Environment{Home: home},
	}, "skills", "doctor", "--target", "codex")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"healthy":true`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestSkillInstallPartialFailureIsTyped(t *testing.T) {
	t.Parallel()
	code, stdout, stderr, _ := runCLI(t, nil, nil, "skills", "install", "--target", "unknown")
	if code != 5 || stdout != "" || !strings.Contains(stderr, "skill_install_partial") || !strings.Contains(stderr, "all_succeeded") {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestJobWaitTimeoutReturnsLastStatus(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, `{"publication_id":"pub_1","status":"compiling"}`)
	}))
	defer server.Close()
	base := time.Now()
	var nowCalls atomic.Int32
	code, stdout, stderr, _ := runCLIWithDependencies(t, server, authenticatedStore(t), "", Dependencies{
		Now: func() time.Time {
			if nowCalls.Add(1) == 1 {
				return base
			}
			return base.Add(2 * time.Second)
		},
	}, "job", "wait", "pub_1", "--timeout", "1s")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"wait_timed_out":true`) || !strings.Contains(stdout, `"status":"compiling"`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestJobResumeConfirmPublishSendsDecisionContract(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/skill-agent-publications/pub_1/actions/act_1/resolve" || request.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode resolve body: %v", err)
		}
		if body["decision"] != "confirm" ||
			body["expected_release_candidate_digest"] != "sha256:candidate" ||
			body["expected_payload_digest"] != "sha256:payload" {
			t.Fatalf("confirm_publish resolve body = %#v", body)
		}
		if _, ok := body["payload"]; ok {
			t.Fatalf("decision resolution must not carry a typed payload: %#v", body)
		}
		_, _ = io.WriteString(writer, `{"action_id":"act_1","status":"resolved","publication_status":"release_authorized","resolution_digest":"sha256:resolution","resolved_at":"2026-07-18T00:00:00Z"}`)
	}))
	defer server.Close()
	code, stdout, stderr, _ := runCLI(t, server, authenticatedStore(t),
		"job", "resume", "pub_1",
		"--action-id", "act_1",
		"--expected-payload-digest", "sha256:payload",
		"--expected-release-candidate-digest", "sha256:candidate",
		"--decision", "confirm",
	)
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"publication_status":"release_authorized"`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestJobResumeCancelPublishSendsDecisionContract(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/skill-agent-publications/pub_1/actions/act_1/resolve" || request.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode resolve body: %v", err)
		}
		if body["decision"] != "cancel" ||
			body["expected_release_candidate_digest"] != "sha256:candidate" ||
			body["expected_payload_digest"] != "sha256:payload" {
			t.Fatalf("cancel resolve body = %#v", body)
		}
		if _, ok := body["payload"]; ok {
			t.Fatalf("cancel resolution must not carry a typed payload: %#v", body)
		}
		_, _ = io.WriteString(writer, `{"action_id":"act_1","status":"resolved","publication_status":"cancelled","resolution_digest":"sha256:resolution","resolved_at":"2026-07-18T00:00:00Z"}`)
	}))
	defer server.Close()
	code, stdout, stderr, _ := runCLI(t, server, authenticatedStore(t),
		"job", "resume", "pub_1",
		"--action-id", "act_1",
		"--expected-payload-digest", "sha256:payload",
		"--expected-release-candidate-digest", "sha256:candidate",
		"--decision", "cancel",
	)
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"publication_status":"cancelled"`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestJobResumeDecisionValidation(t *testing.T) {
	t.Parallel()
	code, _, stderr, _ := runCLI(t, nil, authenticatedStore(t),
		"job", "resume", "pub_1",
		"--action-id", "act_1",
		"--expected-payload-digest", "sha256:payload",
		"--decision", "confirm",
	)
	if code != 2 || !strings.Contains(stderr, "resume_flags") {
		t.Fatalf("missing candidate digest: code=%d stderr=%s", code, stderr)
	}
	code, _, stderr, _ = runCLI(t, nil, authenticatedStore(t),
		"job", "resume", "pub_1",
		"--action-id", "act_1",
		"--expected-payload-digest", "sha256:payload",
		"--expected-release-candidate-digest", "sha256:candidate",
		"--decision", "maybe",
	)
	if code != 2 || !strings.Contains(stderr, "resume_decision") {
		t.Fatalf("invalid decision: code=%d stderr=%s", code, stderr)
	}
	code, _, stderr, _ = runCLIWithInput(t, nil, authenticatedStore(t), `{"selector":"skills/poster"}`,
		"job", "resume", "pub_1",
		"--action-id", "act_1",
		"--expected-payload-digest", "sha256:payload",
		"--expected-release-candidate-digest", "sha256:candidate",
		"--decision", "confirm",
		"--payload-stdin",
	)
	if code != 2 || !strings.Contains(stderr, "resume_flags") {
		t.Fatalf("decision with payload stdin: code=%d stderr=%s", code, stderr)
	}
}

func authenticatedStore(t *testing.T) *securestore.MemoryStore {
	t.Helper()
	store := securestore.NewMemory()
	manager := &credentialauth.Manager{Store: store, Region: "cn"}
	if err := manager.Save(credentialauth.Credential{AccessToken: "test-token", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	return store
}

func runCLI(t *testing.T, server *httptest.Server, store securestore.Store, args ...string) (int, string, string, securestore.Store) {
	t.Helper()
	return runCLIWithInput(t, server, store, "", args...)
}

func runCLIWithInput(t *testing.T, server *httptest.Server, store securestore.Store, input string, args ...string) (int, string, string, securestore.Store) {
	t.Helper()
	return runCLIWithDependencies(t, server, store, input, Dependencies{}, args...)
}

func runCLIWithDependencies(t *testing.T, server *httptest.Server, store securestore.Store, input string, extra Dependencies, args ...string) (int, string, string, securestore.Store) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if store == nil {
		store = securestore.NewMemory()
	}
	extra.In = strings.NewReader(input)
	extra.Out = &stdout
	extra.ErrOut = &stderr
	extra.Store = store
	extra.NewID = func() string { return "request-fixed" }
	extra.Sleep = func(context.Context, time.Duration) error { return nil }
	if extra.Environment.Home == "" {
		extra.Environment = skillcontent.Environment{Home: t.TempDir(), ConfigDir: t.TempDir()}
	}
	if extra.Now == nil {
		extra.Now = time.Now
	}
	if server != nil {
		extra.HTTPClient = server.Client()
		extra.APIBaseURL = server.URL
	}
	code := Execute(args, extra)
	return code, stdout.String(), stderr.String(), store
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
