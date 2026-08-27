package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func validAccessConfig() accessConfig {
	return accessConfig{
		SchemaVersion: 2,
		WorkKey:       "wrk_dagou_tap",
		Region:        "cn",
		APIBaseURL:    "https://api.viceme.cn",
		WebBaseURL:    "https://viceme.cn",
		DisplayName:   "Dagou Tap",
		Features: map[string]accessFeatureConfig{
			"danmaku": {Title: "弹幕", Policy: accessFeaturePolicy{Type: "PUBLIC"}},
		},
		Status:        "ACTIVE",
		ConfigVersion: 1,
	}
}

func TestAccessConfigSupportsOnlyPublicFeatures(t *testing.T) {
	config := validAccessConfig()
	if err := validateAccessConfig(config); err != nil {
		t.Fatalf("validateAccessConfig() error = %v", err)
	}
	request := config.applyRequest()
	if len(request.Features) != 1 || request.Features[0].FeatureKey != "danmaku" || request.Features[0].Policy.Type != "PUBLIC" {
		t.Fatalf("features are not stable and sorted: %#v", request.Features)
	}
}

func TestAccessConfigRejectsNonPublicPolicy(t *testing.T) {
	config := validAccessConfig()
	config.Features["danmaku"] = accessFeatureConfig{
		Title:  "弹幕",
		Policy: accessFeaturePolicy{Type: "FOLLOW_OWNER"},
	}
	err := validateAccessConfig(config)
	if err == nil {
		t.Fatal("validateAccessConfig() error = nil, want POLICY_TYPE_UNSUPPORTED")
	}
	cliError, ok := err.(*output.Error)
	if !ok || cliError.Subtype != "POLICY_TYPE_UNSUPPORTED" {
		t.Fatalf("validateAccessConfig() error = %#v", err)
	}
}

func TestAccessInitCreatesAndActivatesPublicDanmaku(t *testing.T) {
	t.Parallel()
	requests := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests <- request.Method + " " + request.URL.Path
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/v1/cli/sdk-works":
			_, _ = io.WriteString(writer, `{"workKey":"wrk_public_danmaku","displayName":"Demo","status":"DRAFT","configVersion":1,"features":[],"capabilities":[],"createdAt":"2026-08-25T00:00:00Z","updatedAt":"2026-08-25T00:00:00Z"}`)
		case request.Method == http.MethodPut && request.URL.Path == "/v1/cli/sdk-works/wrk_public_danmaku":
			var body api.ApplySdkWorkRequest
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.ExpectedConfigVersion != 1 || body.Status != "ACTIVE" || len(body.Features) != 1 || body.Features[0].FeatureKey != "danmaku" || body.Features[0].Policy.Type != "PUBLIC" {
				t.Fatalf("unexpected apply request: %#v", body)
			}
			_, _ = io.WriteString(writer, `{"workKey":"wrk_public_danmaku","displayName":"Demo","status":"ACTIVE","configVersion":2,"features":[{"featureKey":"danmaku","title":"弹幕","policy":{"type":"PUBLIC"},"status":"ACTIVE"}],"capabilities":["danmaku"],"createdAt":"2026-08-25T00:00:00Z","updatedAt":"2026-08-25T00:00:01Z"}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "access.yaml")
	result, err := executeCreatorAppCommand(t, server.URL, "access", "init", "--config", configPath, "--name", "Demo", "--danmaku")
	if err != nil {
		t.Fatalf("access init failed: %v", err)
	}
	if first, second := <-requests, <-requests; first != "POST /v1/cli/sdk-works" || second != "PUT /v1/cli/sdk-works/wrk_public_danmaku" {
		t.Fatalf("unexpected requests: %q, %q", first, second)
	}
	if !strings.Contains(result, `"workKey": "wrk_public_danmaku"`) && !strings.Contains(result, `"workKey":"wrk_public_danmaku"`) {
		t.Fatalf("result omitted work key: %s", result)
	}
	if !strings.Contains(result, "/viceme-sdk/v1/viceme.min.js") || !strings.Contains(result, `data-viceme-work=\"wrk_public_danmaku\"`) {
		t.Fatalf("result omitted hosted embed snippet: %s", result)
	}
	config, err := readAccessConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if config.ConfigVersion != 2 || config.APIBaseURL != server.URL || config.WebBaseURL == "" {
		t.Fatalf("unexpected persisted authority: %#v", config)
	}
}

func TestAccessInitKeepsRecoverableConfigWhenFirstApplyFails(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPost && request.URL.Path == "/v1/cli/sdk-works" {
			_, _ = io.WriteString(writer, `{"workKey":"wrk_recoverable","displayName":"Demo","status":"DRAFT","configVersion":7,"features":[],"capabilities":[],"createdAt":"2026-08-25T00:00:00Z","updatedAt":"2026-08-25T00:00:00Z"}`)
			return
		}
		writer.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(writer, `{"statusCode":503,"code":"UPSTREAM_UNAVAILABLE","message":"retry later"}`)
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "access.yaml")
	if _, err := executeCreatorAppCommand(t, server.URL, "access", "init", "--config", configPath, "--name", "Demo", "--danmaku"); err == nil {
		t.Fatal("access init succeeded despite failed first apply")
	}
	local, err := readAccessConfig(configPath)
	if err != nil {
		t.Fatalf("recoverable config was not retained: %v", err)
	}
	if local.WorkKey != "wrk_recoverable" || local.ConfigVersion != 7 || local.Features[danmakuFeatureKey].Policy.Type != "PUBLIC" {
		t.Fatalf("recoverable config lost created Work state: %#v", local)
	}
}

func TestAccessApplyConflictLeavesLocalConfigByteIdentical(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(writer, `{"statusCode":409,"code":"CONFIG_VERSION_CONFLICT","message":"inspect and retry"}`)
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "access.yaml")
	local := validAccessConfig()
	local.APIBaseURL = server.URL
	local.WebBaseURL = server.URL
	if err := writeAccessConfig(configPath, local); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executeCreatorAppCommand(t, server.URL, "access", "apply", "--config", configPath); err == nil {
		t.Fatal("access apply succeeded despite config version conflict")
	}
	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("conflict mutated local config:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestAccessListAndDeleteExposeOrphanRecovery(t *testing.T) {
	t.Parallel()
	const workKey = "wrk_orphan_recovery"
	requests := make(chan string, 2)
	var requestCount atomic.Int32
	work := `{"workKey":"` + workKey + `","displayName":"Orphan","status":"DRAFT","configVersion":1,"features":[],"capabilities":[],"createdAt":"2026-08-25T00:00:00Z","updatedAt":"2026-08-25T00:00:00Z"}`
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestCount.Add(1)
		requests <- request.Method + " " + request.URL.Path
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/cli/sdk-works":
			_, _ = io.WriteString(writer, `{"works":[`+work+`]}`)
		case request.Method == http.MethodDelete && request.URL.Path == "/v1/cli/sdk-works/"+workKey:
			_, _ = io.WriteString(writer, work)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	listOutput, err := executeCreatorAppCommand(t, server.URL, "access", "list")
	if err != nil {
		t.Fatalf("access list failed: %v", err)
	}
	if !strings.Contains(listOutput, workKey) {
		t.Fatalf("access list omitted orphan Work: %s", listOutput)
	}

	confirmationOutput, err := executeCreatorAppCommand(t, server.URL, "access", "delete", workKey)
	if err == nil || !strings.Contains(confirmationOutput, "SDK_WORK_DELETE_CONFIRMATION_REQUIRED") {
		t.Fatalf("access delete did not require confirmation: output=%s err=%v", confirmationOutput, err)
	}
	if requestCount.Load() != 1 {
		t.Fatalf("unconfirmed delete reached the API: requests=%d", requestCount.Load())
	}

	deleteOutput, err := executeCreatorAppCommand(t, server.URL, "access", "delete", workKey, "--yes")
	if err != nil {
		t.Fatalf("confirmed access delete failed: %v", err)
	}
	if !strings.Contains(deleteOutput, `"deleted": true`) || !strings.Contains(deleteOutput, workKey) {
		t.Fatalf("delete output omitted cleanup result: %s", deleteOutput)
	}
	if first, second := <-requests, <-requests; first != "GET /v1/cli/sdk-works" || second != "DELETE /v1/cli/sdk-works/"+workKey {
		t.Fatalf("unexpected recovery requests: %q, %q", first, second)
	}
}

func TestAccessInitUnknownCreateOutcomeIsNonRetryableAndActionable(t *testing.T) {
	t.Parallel()
	var createRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/cli/sdk-works" {
			t.Errorf("unexpected request: %s %s", request.Method, request.URL.Path)
			return
		}
		createRequests.Add(1)
		panic(http.ErrAbortHandler)
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "access.yaml")
	commandOutput, err := executeCreatorAppCommand(
		t,
		server.URL,
		"access", "init", "--config", configPath, "--name", "Demo", "--danmaku",
	)
	if err == nil {
		t.Fatal("access init succeeded after its POST response was lost")
	}
	exitError, ok := err.(*commandExitError)
	if !ok || exitError.exit != output.ExitNetwork {
		t.Fatalf("unknown create outcome used the wrong exit: %#v", err)
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
	if envelope.OK || envelope.Error.Code != "SDK_WORK_CREATE_OUTCOME_UNKNOWN" || envelope.Error.Retryable {
		t.Fatalf("unknown outcome was not fail-closed and non-retryable: %#v", envelope)
	}
	for _, command := range []string{
		"viceme access list",
		"viceme access delete <work-key> --yes",
		"viceme access init --work-key <work-key> --name <name> --danmaku",
	} {
		if !strings.Contains(envelope.Error.Hint, command) {
			t.Fatalf("unknown outcome hint omitted %q: %q", command, envelope.Error.Hint)
		}
	}
	if createRequests.Load() != 1 {
		t.Fatalf("access init retried an unknown POST: requests=%d", createRequests.Load())
	}
	if _, statErr := os.Stat(configPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("unknown POST outcome wrote a guessed local binding: %v", statErr)
	}
}

func TestAccessInitInvalidCreateResponsesAreOutcomeUnknown(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"empty":            "\n",
		"empty_object":     `{}`,
		"invalid_work_key": `{"workKey":"invalid","displayName":"Demo","status":"DRAFT","configVersion":1,"features":[],"capabilities":[],"createdAt":"2026-08-25T00:00:00Z","updatedAt":"2026-08-25T00:00:00Z"}`,
		"missing_fields":   `{"workKey":"wrk_incomplete","displayName":"Demo","status":"DRAFT","configVersion":1}`,
	} {
		name, body := name, body
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				requests.Add(1)
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(http.StatusCreated)
				_, _ = io.WriteString(writer, body)
			}))
			defer server.Close()
			configPath := filepath.Join(t.TempDir(), "access.yaml")
			commandOutput, err := executeCreatorAppCommand(
				t, server.URL, "access", "init", "--config", configPath, "--name", "Demo", "--danmaku",
			)
			if err == nil {
				t.Fatalf("access init accepted %s create response", name)
			}
			var envelope struct {
				Error struct {
					Code      string `json:"code"`
					Retryable bool   `json:"retryable"`
					Hint      string `json:"hint"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(commandOutput), &envelope); err != nil {
				t.Fatalf("invalid JSON error envelope: %v output=%q", err, commandOutput)
			}
			if envelope.Error.Code != "SDK_WORK_CREATE_OUTCOME_UNKNOWN" || envelope.Error.Retryable ||
				!strings.Contains(envelope.Error.Hint, "viceme access list") {
				t.Fatalf("invalid create response did not produce actionable unknown outcome: %#v", envelope)
			}
			if requests.Load() != 1 {
				t.Fatalf("invalid create response was retried: requests=%d", requests.Load())
			}
			if _, statErr := os.Stat(configPath); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("invalid create response wrote local state: %v", statErr)
			}
		})
	}
}

func TestAccessInitCanExplicitlyRecoverSelectedDraftWork(t *testing.T) {
	t.Parallel()
	const workKey = "wrk_selected_orphan"
	requests := make(chan string, 2)
	var createRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPost {
			createRequests.Add(1)
		}
		requests <- request.Method + " " + request.URL.Path
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/cli/sdk-works/"+workKey:
			_, _ = io.WriteString(writer, `{"workKey":"`+workKey+`","displayName":"Recovered","status":"DRAFT","configVersion":7,"features":[],"capabilities":[],"createdAt":"2026-08-25T00:00:00Z","updatedAt":"2026-08-25T00:00:00Z"}`)
		case request.Method == http.MethodPut && request.URL.Path == "/v1/cli/sdk-works/"+workKey:
			var body api.ApplySdkWorkRequest
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Error(err)
				return
			}
			if body.ExpectedConfigVersion != 7 || body.DisplayName != "Demo" || len(body.Features) != 1 || body.Features[0].Policy.Type != "PUBLIC" {
				t.Errorf("unexpected recovery apply request: %#v", body)
			}
			_, _ = io.WriteString(writer, `{"workKey":"`+workKey+`","displayName":"Demo","status":"ACTIVE","configVersion":8,"features":[{"featureKey":"danmaku","title":"弹幕","policy":{"type":"PUBLIC"},"status":"ACTIVE"}],"capabilities":["danmaku"],"createdAt":"2026-08-25T00:00:00Z","updatedAt":"2026-08-25T00:00:01Z"}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "access.yaml")
	result, err := executeCreatorAppCommand(
		t,
		server.URL,
		"access", "init", "--config", configPath, "--name", "Demo", "--danmaku", "--work-key", workKey,
	)
	if err != nil {
		t.Fatalf("explicit Work recovery failed: %v", err)
	}
	if createRequests.Load() != 0 {
		t.Fatalf("explicit Work recovery created another Work: requests=%d", createRequests.Load())
	}
	if first, second := <-requests, <-requests; first != "GET /v1/cli/sdk-works/"+workKey || second != "PUT /v1/cli/sdk-works/"+workKey {
		t.Fatalf("unexpected explicit recovery requests: %q, %q", first, second)
	}
	local, err := readAccessConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if local.WorkKey != workKey || local.ConfigVersion != 8 || local.APIBaseURL != server.URL || local.WebBaseURL != server.URL {
		t.Fatalf("explicit recovery persisted the wrong binding: %#v", local)
	}
	if !strings.Contains(result, `data-viceme-work=\"`+workKey+`\"`) {
		t.Fatalf("explicit recovery omitted embed snippet: %s", result)
	}
}

func TestAccessInspectRecoversConfigVersionAfterLostApplyResponse(t *testing.T) {
	t.Parallel()
	const workKey = "wrk_lost_put_response"
	var applied atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPut && request.URL.Path == "/v1/cli/sdk-works/"+workKey:
			var body api.ApplySdkWorkRequest
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Error(err)
				return
			}
			if body.ExpectedConfigVersion != 1 || body.DisplayName != "Demo" || body.Status != "ACTIVE" {
				t.Errorf("unexpected lost-response apply request: %#v", body)
			}
			applied.Store(true)
			panic(http.ErrAbortHandler)
		case request.Method == http.MethodGet && request.URL.Path == "/v1/cli/sdk-works/"+workKey:
			if !applied.Load() {
				t.Error("inspect ran before the simulated remote apply")
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"workKey":"`+workKey+`","displayName":"Demo","status":"ACTIVE","configVersion":2,"features":[{"featureKey":"danmaku","title":"弹幕","policy":{"type":"PUBLIC"},"status":"ACTIVE"}],"capabilities":["danmaku"],"createdAt":"2026-08-25T00:00:00Z","updatedAt":"2026-08-25T00:00:01Z"}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "access.yaml")
	local := validAccessConfig()
	local.WorkKey = workKey
	local.DisplayName = "Demo"
	local.APIBaseURL = server.URL
	local.WebBaseURL = server.URL
	if err := writeAccessConfig(configPath, local); err != nil {
		t.Fatal(err)
	}
	if _, err := executeCreatorAppCommand(t, server.URL, "access", "apply", "--config", configPath); err == nil {
		t.Fatal("apply succeeded despite its lost response")
	}
	stale, err := readAccessConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if stale.ConfigVersion != 1 {
		t.Fatalf("failed apply guessed a new configVersion: %#v", stale)
	}

	inspectOutput, err := executeCreatorAppCommand(t, server.URL, "access", "inspect", "--config", configPath)
	if err != nil {
		t.Fatalf("inspect did not recover the lost PUT response: %v", err)
	}
	recovered, err := readAccessConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.ConfigVersion != 2 {
		t.Fatalf("inspect did not persist the authoritative remote version: %#v", recovered)
	}
	if !strings.Contains(inspectOutput, `"localConfigVersion": 2`) || !strings.Contains(inspectOutput, `"remoteConfigVersion": 2`) {
		t.Fatalf("inspect did not report reconciled versions: %s", inspectOutput)
	}
}

func TestAccessApplyInvalidSuccessResponsePreservesExpectedConfig(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"empty":          "\n",
		"empty_object":   `{}`,
		"missing_fields": `{"workKey":"wrk_bad_put","displayName":"Demo","status":"ACTIVE","configVersion":2}`,
	} {
		name, body := name, body
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodPut || request.URL.Path != "/v1/cli/sdk-works/wrk_bad_put" {
					http.NotFound(writer, request)
					return
				}
				writer.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(writer, body)
			}))
			defer server.Close()
			configPath := filepath.Join(t.TempDir(), "access.yaml")
			local := validAccessConfig()
			local.WorkKey = "wrk_bad_put"
			local.DisplayName = "Demo"
			local.APIBaseURL = server.URL
			local.WebBaseURL = server.URL
			if err := writeAccessConfig(configPath, local); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := executeCreatorAppCommand(t, server.URL, "access", "apply", "--config", configPath); err == nil {
				t.Fatalf("access apply accepted %s response", name)
			}
			after, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatalf("invalid PUT response changed expected local config:\nbefore=%s\nafter=%s", before, after)
			}
			preserved, err := readAccessConfig(configPath)
			if err != nil || preserved.ConfigVersion != 1 {
				t.Fatalf("invalid PUT response persisted a guessed version: config=%#v err=%v", preserved, err)
			}
		})
	}
}

func TestConcurrentAccessInitProcessesCreateOneWork(t *testing.T) {
	if os.Getenv("VICEME_TEST_ACCESS_INIT_HELPER") == "1" {
		configDir := os.Getenv("VICEME_TEST_CONFIG_DIR")
		configPath := os.Getenv("VICEME_TEST_ACCESS_CONFIG")
		exit := Execute(
			[]string{"access", "init", "--config", configPath, "--name", "Demo", "--danmaku"},
			Dependencies{
				Out: os.Stdout, ErrOut: os.Stderr, Store: securestore.NewMemory(),
				Environment: skillcontent.Environment{Home: os.Getenv("VICEME_TEST_HOME"), ConfigDir: configDir},
			},
		)
		if exit != 0 {
			t.Fatalf("access init helper exited %d", exit)
		}
		return
	}

	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	configPath := filepath.Join(root, "site", ".viceme", "access.yaml")
	var posts atomic.Int32
	var puts atomic.Int32
	var gets atomic.Int32
	postStarted := make(chan struct{}, 1)
	releasePost := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/v1/cli/sdk-works":
			posts.Add(1)
			postStarted <- struct{}{}
			<-releasePost
			_, _ = io.WriteString(writer, `{"workKey":"wrk_concurrent","displayName":"Demo","status":"DRAFT","configVersion":1,"features":[],"capabilities":[],"createdAt":"2026-08-25T00:00:00Z","updatedAt":"2026-08-25T00:00:00Z"}`)
		case request.Method == http.MethodPut && request.URL.Path == "/v1/cli/sdk-works/wrk_concurrent":
			puts.Add(1)
			_, _ = io.WriteString(writer, `{"workKey":"wrk_concurrent","displayName":"Demo","status":"ACTIVE","configVersion":2,"features":[{"featureKey":"danmaku","title":"弹幕","policy":{"type":"PUBLIC"},"status":"ACTIVE"}],"capabilities":["danmaku"],"createdAt":"2026-08-25T00:00:00Z","updatedAt":"2026-08-25T00:00:01Z"}`)
		case request.Method == http.MethodGet && request.URL.Path == "/v1/cli/sdk-works/wrk_concurrent":
			gets.Add(1)
			_, _ = io.WriteString(writer, `{"workKey":"wrk_concurrent","displayName":"Demo","status":"ACTIVE","configVersion":2,"features":[{"featureKey":"danmaku","title":"弹幕","policy":{"type":"PUBLIC"},"status":"ACTIVE"}],"capabilities":["danmaku"],"createdAt":"2026-08-25T00:00:00Z","updatedAt":"2026-08-25T00:00:01Z"}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	configured := config.Default(config.RegionCN)
	if err := configured.SetProfileAuthority(config.DefaultProfileName, server.URL, server.URL, config.RegionCN); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Save(configDir, configured); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	newProcess := func() (*exec.Cmd, *bytes.Buffer, *bytes.Buffer) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestConcurrentAccessInitProcessesCreateOneWork$")
		environment := make([]string, 0, len(os.Environ())+5)
		for _, entry := range os.Environ() {
			if strings.HasPrefix(entry, apiBaseURLEnvironment+"=") || strings.HasPrefix(entry, processAccessTokenEnvironment+"=") ||
				strings.HasPrefix(entry, "VICEME_TEST_ACCESS_INIT_HELPER=") {
				continue
			}
			environment = append(environment, entry)
		}
		command.Env = append(environment,
			"VICEME_TEST_ACCESS_INIT_HELPER=1",
			"VICEME_TEST_CONFIG_DIR="+configDir,
			"VICEME_TEST_ACCESS_CONFIG="+configPath,
			"VICEME_TEST_HOME="+root,
			processAccessTokenEnvironment+"=vme_cli_1234567890123456789012345678901234567890123",
		)
		command.Stdout = stdout
		command.Stderr = stderr
		return command, stdout, stderr
	}
	first, firstOut, firstErr := newProcess()
	if err := first.Start(); err != nil {
		t.Fatal(err)
	}
	firstDone := make(chan error, 1)
	go func() { firstDone <- first.Wait() }()
	select {
	case <-postStarted:
	case err := <-firstDone:
		t.Fatalf("first process exited before POST: %v stdout=%q stderr=%q", err, firstOut.String(), firstErr.String())
	case <-ctx.Done():
		t.Fatalf("first process did not reach POST: %v", ctx.Err())
	}
	second, secondOut, secondErr := newProcess()
	if err := second.Start(); err != nil {
		t.Fatal(err)
	}
	secondDone := make(chan error, 1)
	go func() { secondDone <- second.Wait() }()
	time.Sleep(250 * time.Millisecond)
	if posts.Load() != 1 {
		t.Fatalf("second process bypassed the access init lock: posts=%d", posts.Load())
	}
	close(releasePost)
	if err := <-firstDone; err != nil {
		t.Fatalf("first process failed: %v stdout=%q stderr=%q", err, firstOut.String(), firstErr.String())
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second process failed: %v stdout=%q stderr=%q", err, secondOut.String(), secondErr.String())
	}
	if posts.Load() != 1 || puts.Load() != 1 || gets.Load() != 1 {
		t.Fatalf("concurrent init did not coalesce: posts=%d puts=%d gets=%d", posts.Load(), puts.Load(), gets.Load())
	}
	local, err := readAccessConfig(configPath)
	if err != nil || local.WorkKey != "wrk_concurrent" || local.ConfigVersion != 2 {
		t.Fatalf("concurrent init did not commit one binding: config=%#v err=%v", local, err)
	}
}

func TestSdkWorkConfigVersionRecoveryRequiresExactRemoteContent(t *testing.T) {
	t.Parallel()
	local := validAccessConfig()
	request := local.applyRequest()
	work := api.SdkWork{
		WorkKey: local.WorkKey, DisplayName: request.DisplayName, Status: request.Status,
		ConfigVersion: 2, Features: request.Features,
	}
	if !sdkWorkMatchesAccessConfig(local, work) {
		t.Fatal("identical remote config was not recognized")
	}
	work.Features[0].Title = "Different"
	if sdkWorkMatchesAccessConfig(local, work) {
		t.Fatal("different remote config was treated as a safe version recovery")
	}
}

func TestAccessCommandsRejectAPIBaseEnvironmentOverride(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		http.NotFound(writer, request)
	}))
	defer server.Close()
	t.Setenv(apiBaseURLEnvironment, server.URL)

	commandOutput, err := executeCreatorAppCommand(t, server.URL, "access", "list")
	if err == nil {
		t.Fatal("access list accepted VICEME_API_BASE_URL")
	}
	if !strings.Contains(commandOutput, "PROFILE_AUTHORITY_OVERRIDE_ACTIVE") {
		t.Fatalf("access list returned the wrong override error: %s", commandOutput)
	}
	if requests.Load() != 0 {
		t.Fatalf("blocked authority override reached the API: requests=%d", requests.Load())
	}
}
