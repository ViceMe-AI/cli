package command

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func executeCreatorAppCommand(t *testing.T, baseURL string, args ...string) (string, error) {
	return executeCreatorAppCommandForRegion(t, baseURL, config.RegionCN, args...)
}

func executeCreatorAppCommandForRegion(t *testing.T, baseURL string, region config.Region, args ...string) (string, error) {
	t.Helper()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	configured := config.Default(region)
	if err := configured.SetProfileAuthority(config.DefaultProfileName, baseURL, baseURL, region); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Save(configDir, configured); err != nil {
		t.Fatal(err)
	}
	store := securestore.NewMemory()
	scope, err := credentialScopeForAPIBase(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	manager := credentialauth.Manager{Store: store, Region: string(region), ProfileID: "default", ProfileName: "default", Scope: scope}
	if err := manager.Save(credentialauth.Credential{AccessToken: "vme_cli_test", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	dependencies := Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: store,
		Environment: skillcontent.Environment{Home: root, ConfigDir: configDir},
	}
	if exit := Execute(args, dependencies); exit != 0 {
		return stdout.String(), &commandExitError{exit: exit, stdout: stdout.String(), stderr: stderr.String()}
	}
	return stdout.String(), nil
}

type commandExitError struct {
	exit   int
	stdout string
	stderr string
}

func (e *commandExitError) Error() string {
	if e.stderr != "" {
		return e.stderr
	}
	return e.stdout
}

func TestCreatorAppCreatePostsToCliEndpoint(t *testing.T) {
	t.Parallel()
	var gotPath, gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"11111111-1111-4111-8111-111111111111","kind":"EXTERNAL","name":"Demo","domains":[],"createdAt":"2026-08-18T00:00:00Z"}`))
	}))
	defer server.Close()

	output, err := executeCreatorAppCommand(t, server.URL, "creator-app", "create", "--name", "Demo")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/v1/cli/creator-apps" {
		t.Fatalf("unexpected request %s %s", gotMethod, gotPath)
	}
	var decoded struct {
		Data struct {
			App struct {
				ID string `json:"id"`
			} `json:"app"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if decoded.Data.App.ID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("unexpected app id: %q", decoded.Data.App.ID)
	}
}

func TestCreatorAppCreateUnknownOutcomeIsNonRetryableAndActionable(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		body  string
		abort bool
	}{
		"lost_response":  {abort: true},
		"empty_response": {body: "\n"},
		"empty_object":   {body: `{}`},
		"missing_fields": {body: `{"id":"app-1","kind":"EXTERNAL","name":"Demo"}`},
	}
	for name, test := range tests {
		name, test := name, test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				requests.Add(1)
				if request.Method != http.MethodPost || request.URL.Path != "/v1/cli/creator-apps" {
					t.Errorf("unexpected request: %s %s", request.Method, request.URL.Path)
					return
				}
				if test.abort {
					panic(http.ErrAbortHandler)
				}
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(http.StatusCreated)
				_, _ = writer.Write([]byte(test.body))
			}))
			defer server.Close()

			commandOutput, err := executeCreatorAppCommand(t, server.URL, "creator-app", "create", "--name", "Demo")
			exitError, ok := err.(*commandExitError)
			if !ok || exitError.exit != output.ExitNetwork {
				t.Fatalf("unknown create outcome used the wrong exit: output=%s err=%#v", commandOutput, err)
			}
			var envelope struct {
				OK    bool `json:"ok"`
				Error struct {
					Code      string `json:"code"`
					Retryable bool   `json:"retryable"`
					Hint      string `json:"hint"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(commandOutput), &envelope); err != nil {
				t.Fatalf("unknown outcome returned invalid JSON: %v output=%q", err, commandOutput)
			}
			if envelope.OK || envelope.Error.Code != "CREATOR_APP_CREATE_OUTCOME_UNKNOWN" || envelope.Error.Retryable {
				t.Fatalf("Creator App outcome was not fail-closed: %#v", envelope)
			}
			if !strings.Contains(envelope.Error.Hint, "viceme creator-app list") || !strings.Contains(envelope.Error.Hint, "do not retry") {
				t.Fatalf("Creator App recovery hint is not actionable: %q", envelope.Error.Hint)
			}
			if requests.Load() != 1 {
				t.Fatalf("Creator App POST was retried: requests=%d", requests.Load())
			}
		})
	}
}

func TestCreatorAppDomainAddSurfacesVerificationToken(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/v1/cli/creator-apps/11111111-1111-4111-8111-111111111111/domains" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"11111111-1111-4111-8111-111111111111","kind":"EXTERNAL","name":"Demo","domains":[{"domain":"shop.viceme.dev","verified":false,"verificationToken":"0123456789abcdef0123456789abcdef"}],"createdAt":"2026-08-18T00:00:00Z"}`))
	}))
	defer server.Close()

	output, err := executeCreatorAppCommand(t, server.URL, "creator-app", "domain", "add", "11111111-1111-4111-8111-111111111111", "shop.viceme.dev")
	if err != nil {
		t.Fatalf("domain add failed: %v", err)
	}
	if !strings.Contains(output, "0123456789abcdef0123456789abcdef") {
		t.Fatalf("verification token missing from output: %s", output)
	}
}

func TestCreatorAppDomainVerify(t *testing.T) {
	t.Parallel()
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"11111111-1111-4111-8111-111111111111","kind":"EXTERNAL","name":"Demo","domains":[{"domain":"shop.viceme.dev","verified":true,"verificationToken":null}],"createdAt":"2026-08-18T00:00:00Z"}`))
	}))
	defer server.Close()

	output, err := executeCreatorAppCommand(t, server.URL, "creator-app", "domain", "verify", "11111111-1111-4111-8111-111111111111", "shop.viceme.dev")
	if err != nil {
		t.Fatalf("domain verify failed: %v", err)
	}
	if gotPath != "/v1/cli/creator-apps/11111111-1111-4111-8111-111111111111/domains/shop.viceme.dev/verify" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if !strings.Contains(output, `"verified": true`) && !strings.Contains(output, `"verified":true`) {
		t.Fatalf("verified state missing: %s", output)
	}
}

func TestCreatorAppShowRendersEmbedSnippet(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"id":"11111111-1111-4111-8111-111111111111","kind":"EXTERNAL","name":"Demo","domains":[{"domain":"shop.viceme.dev","verified":true,"verificationToken":null}],"createdAt":"2026-08-18T00:00:00Z"}]}`))
	}))
	defer server.Close()

	output, err := executeCreatorAppCommand(t, server.URL, "creator-app", "show", "11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatalf("show failed: %v", err)
	}
	var decoded struct {
		Data struct {
			EmbedSnippet string `json:"embedSnippet"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if !strings.Contains(decoded.Data.EmbedSnippet, `data-creator-app-id="11111111-1111-4111-8111-111111111111"`) {
		t.Fatalf("embed snippet missing app id: %s", decoded.Data.EmbedSnippet)
	}
	if !strings.Contains(decoded.Data.EmbedSnippet, "/widget/tip-embed.js") {
		t.Fatalf("embed snippet missing script url: %s", decoded.Data.EmbedSnippet)
	}
}

func TestCreatorAppShowRendersAuthoritativeCombinedSnippet(t *testing.T) {
	t.Parallel()
	const appID = "11111111-1111-4111-8111-111111111111"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/cli/creator-apps":
			_, _ = w.Write([]byte(`{"items":[{"id":"` + appID + `","kind":"EXTERNAL","name":"Demo","domains":[{"domain":"shop.viceme.dev","verified":true,"verificationToken":null}],"createdAt":"2026-08-18T00:00:00Z"}]}`))
		case "/v1/cli/sdk-works/wrk_public_danmaku":
			_, _ = w.Write([]byte(`{"workKey":"wrk_public_danmaku","displayName":"Demo","status":"ACTIVE","configVersion":2,"features":[{"featureKey":"danmaku","title":"弹幕","policy":{"type":"PUBLIC"},"status":"ACTIVE"}],"capabilities":["danmaku"],"createdAt":"2026-08-25T00:00:00Z","updatedAt":"2026-08-25T00:00:01Z"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	output, err := executeCreatorAppCommand(t, server.URL, "creator-app", "show", appID, "--work-key", "wrk_public_danmaku")
	if err != nil {
		t.Fatalf("show failed: %v", err)
	}
	var decoded struct {
		Data struct {
			EngagementEmbedSnippet string `json:"engagementEmbedSnippet"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	expected := `<script defer src="` + server.URL + `/widget/engagement-embed.js" data-viceme-work="wrk_public_danmaku" data-creator-app-id="` + appID + `" data-viceme-region="cn" data-viceme-target="body" data-viceme-theme="auto" data-locale="zh-CN"></script>`
	if decoded.Data.EngagementEmbedSnippet != expected {
		t.Fatalf("combined wrapper mismatch:\nactual:   %s\nexpected: %s", decoded.Data.EngagementEmbedSnippet, expected)
	}
}

func TestCreatorAppShowRendersGlobalEnglishCombinedSnippet(t *testing.T) {
	t.Parallel()
	const appID = "11111111-1111-4111-8111-111111111111"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/cli/creator-apps":
			_, _ = w.Write([]byte(`{"items":[{"id":"` + appID + `","kind":"EXTERNAL","name":"Demo","domains":[],"createdAt":"2026-08-18T00:00:00Z"}]}`))
		case "/v1/cli/sdk-works/wrk_public_danmaku":
			_, _ = w.Write([]byte(`{"workKey":"wrk_public_danmaku","displayName":"Demo","status":"ACTIVE","configVersion":2,"features":[{"featureKey":"danmaku","title":"Danmaku","policy":{"type":"PUBLIC"},"status":"ACTIVE"}],"capabilities":["danmaku"],"createdAt":"2026-08-25T00:00:00Z","updatedAt":"2026-08-25T00:00:01Z"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	commandOutput, err := executeCreatorAppCommandForRegion(
		t,
		server.URL,
		config.RegionGlobal,
		"creator-app", "show", appID,
		"--work-key", "wrk_public_danmaku",
		"--locale", "en-US",
	)
	if err != nil {
		t.Fatalf("show failed: %v", err)
	}
	var decoded struct {
		Data struct {
			EmbedSnippet           string `json:"embedSnippet"`
			EngagementEmbedSnippet string `json:"engagementEmbedSnippet"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(commandOutput), &decoded); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if !strings.Contains(decoded.Data.EmbedSnippet, `data-locale="en-US"`) {
		t.Fatalf("tip snippet did not use the selected locale: %s", decoded.Data.EmbedSnippet)
	}
	expected := `<script defer src="` + server.URL + `/widget/engagement-embed.js" data-viceme-work="wrk_public_danmaku" data-creator-app-id="` + appID + `" data-viceme-region="global" data-viceme-target="body" data-viceme-theme="auto" data-locale="en-US"></script>`
	if decoded.Data.EngagementEmbedSnippet != expected {
		t.Fatalf("combined wrapper mismatch:\nactual:   %s\nexpected: %s", decoded.Data.EngagementEmbedSnippet, expected)
	}
}

func TestCreatorAppShowRejectsUnsupportedLocaleBeforeRequest(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.NotFound(w, r)
	}))
	defer server.Close()

	commandOutput, err := executeCreatorAppCommand(
		t, server.URL, "creator-app", "show", "app-1", "--locale", "fr-FR",
	)
	if err == nil || !strings.Contains(commandOutput, "LOCALE_INVALID") {
		t.Fatalf("unsupported locale was not rejected: output=%s err=%v", commandOutput, err)
	}
	if requests.Load() != 0 {
		t.Fatalf("unsupported locale reached the API: requests=%d", requests.Load())
	}
}

func TestCreatorAppList(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/cli/creator-apps" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"id":"11111111-1111-4111-8111-111111111111","kind":"EXTERNAL","name":"Demo","domains":[],"createdAt":"2026-08-18T00:00:00Z"}]}`))
	}))
	defer server.Close()

	output, err := executeCreatorAppCommand(t, server.URL, "creator-app", "list")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(output, "11111111-1111-4111-8111-111111111111") {
		t.Fatalf("list output missing app: %s", output)
	}
}

func TestCreatorAppCommandsRejectAPIBaseEnvironmentOverrideBeforeRequest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		http.NotFound(writer, request)
	}))
	defer server.Close()
	t.Setenv(apiBaseURLEnvironment, server.URL)

	commands := [][]string{
		{"creator-app", "create", "--name", "Demo"},
		{"creator-app", "list"},
		{"creator-app", "show", "app-1"},
		{"creator-app", "domain", "add", "app-1", "shop.example.com"},
		{"creator-app", "domain", "verify", "app-1", "shop.example.com"},
	}
	for _, args := range commands {
		commandOutput, err := executeCreatorAppCommand(t, server.URL, args...)
		if err == nil || !strings.Contains(commandOutput, "PROFILE_AUTHORITY_OVERRIDE_ACTIVE") {
			t.Fatalf("%q did not reject the debug authority override: output=%s err=%v", strings.Join(args, " "), commandOutput, err)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("blocked Creator App commands reached the API: requests=%d", requests.Load())
	}
}
