package command

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func validAccessConfig() accessConfig {
	return accessConfig{
		SchemaVersion: 1,
		WorkKey:       "wrk_dagou_tap",
		ProfileID:     "default",
		Region:        "cn",
		DisplayName:   "Dagou Tap",
		Features: map[string]accessFeatureConfig{
			"danmaku": {Title: "弹幕", Policy: accessFeaturePolicy{Type: "PUBLIC"}},
		},
		Status:        "ACTIVE",
		ConfigVersion: 1,
	}
}

func TestAccessConfigSupportsOnlyPublicDanmaku(t *testing.T) {
	config := validAccessConfig()
	if err := validateAccessConfig(config); err != nil {
		t.Fatalf("validateAccessConfig() error = %v", err)
	}
	request := config.applyRequest()
	if len(request.Features) != 1 || request.Features[0].FeatureKey != "danmaku" || request.Features[0].Policy.Type != "PUBLIC" || request.PriceCents != nil {
		t.Fatalf("danmaku feature is not stable: %#v", request)
	}
}

func TestAccessInitExposesOnlyDanmakuCapabilityFlags(t *testing.T) {
	command := newAccessInitCommand(&Runtime{})
	flag := command.Flags().Lookup("danmaku")
	if flag == nil || flag.DefValue != "false" {
		t.Fatalf("danmaku flag = %#v", flag)
	}
	for _, unsupported := range []string{"follow", "product", "price-minor", "purchase", "purchase-any"} {
		if command.Flags().Lookup(unsupported) != nil {
			t.Fatalf("access init unexpectedly exposes --%s", unsupported)
		}
	}
}

func TestAccessResultUsesProfileWebBaseURLForDanmakuSnippet(t *testing.T) {
	runtime := &Runtime{
		profile: config.Profile{WebBaseURL: "https://poc.viceme.cn"},
		region:  config.RegionCN,
	}
	work := api.SdkWork{
		WorkKey:      "wrk_dagou_tap",
		Status:       "ACTIVE",
		Capabilities: []string{"danmaku"},
	}

	result, err := buildAccessResult(runtime, work, defaultAccessConfigPath, work.WorkKey)
	if err != nil {
		t.Fatalf("buildAccessResult() error = %v", err)
	}
	if result["scriptUrl"] != "https://poc.viceme.cn/viceme-sdk/v1/viceme.min.js" {
		t.Fatalf("scriptUrl = %#v", result["scriptUrl"])
	}
	snippet, ok := result["embedSnippet"].(string)
	wantSnippet := `<script
  defer src="https://poc.viceme.cn/viceme-sdk/v1/viceme.min.js" data-viceme-work="wrk_dagou_tap" data-viceme-region="cn"
  data-viceme-features="danmaku" data-viceme-target="body"
  data-viceme-theme="auto"></script>`
	if !ok || snippet != wantSnippet {
		t.Fatalf("embedSnippet = %#v", result["embedSnippet"])
	}
}

func TestAccessResultEscapesSnippetAttributes(t *testing.T) {
	runtime := &Runtime{
		profile: config.Profile{WebBaseURL: `https://poc.example/"quoted`},
		region:  config.Region(`cn" onload="bad`),
	}
	work := api.SdkWork{WorkKey: "wrk_dagou_tap", Status: "ACTIVE", Capabilities: []string{"danmaku"}}

	result, err := buildAccessResult(runtime, work, defaultAccessConfigPath, work.WorkKey)
	if err != nil {
		t.Fatal(err)
	}
	snippet := result["embedSnippet"].(string)
	if strings.Contains(snippet, `onload="bad`) || !strings.Contains(snippet, "&#34;") {
		t.Fatalf("embedSnippet did not escape attributes: %s", snippet)
	}
}

func TestAccessResultOmitsSnippetWithoutDanmakuCapability(t *testing.T) {
	result, err := buildAccessResult(
		&Runtime{region: config.RegionCN},
		api.SdkWork{WorkKey: "wrk_dagou_tap", Status: "ACTIVE"},
		defaultAccessConfigPath,
		"wrk_dagou_tap",
	)
	if err != nil {
		t.Fatalf("buildAccessResult() error = %v", err)
	}
	if _, exists := result["embedSnippet"]; exists {
		t.Fatalf("embedSnippet unexpectedly present: %#v", result)
	}
}

func TestAccessResultRejectsMismatchedWorkBinding(t *testing.T) {
	_, err := buildAccessResult(
		&Runtime{region: config.RegionCN},
		api.SdkWork{WorkKey: "wrk_other_work", Status: "ACTIVE"},
		defaultAccessConfigPath,
		"wrk_dagou_tap",
	)
	cliError, ok := err.(*output.Error)
	if !ok || cliError.Subtype != "SDK_WORK_RESPONSE_INVALID" {
		t.Fatalf("buildAccessResult() error = %#v", err)
	}
}

func TestDanmakuScriptURLRequiresProfileWebBaseURL(t *testing.T) {
	_, err := danmakuScriptURL(&Runtime{
		profile: config.Profile{APIBaseURL: "https://api.poc.example"},
		region:  config.RegionCN,
	})
	cliError, ok := err.(*output.Error)
	if !ok || cliError.Subtype != "PROFILE_WEB_BASE_URL_REQUIRED" {
		t.Fatalf("danmakuScriptURL() error = %#v", err)
	}
}

func TestDanmakuScriptURLRejectsProcessAPIOverride(t *testing.T) {
	_, err := danmakuScriptURL(&Runtime{
		profile:           config.Profile{WebBaseURL: "https://poc.example"},
		region:            config.RegionCN,
		apiBaseURLFromEnv: true,
	})
	cliError, ok := err.(*output.Error)
	if !ok || cliError.Subtype != "profile_api_base_url_conflict" {
		t.Fatalf("danmakuScriptURL() error = %#v", err)
	}
}

func TestAccessConfigRejectsAnotherProfileInSameRegion(t *testing.T) {
	err := validateAccessProfile(
		&Runtime{profile: config.Profile{ID: "profile-b"}, region: config.RegionCN},
		validAccessConfig(),
		false,
	)
	cliError, ok := err.(*output.Error)
	if !ok || cliError.Subtype != "ACCESS_PROFILE_MISMATCH" {
		t.Fatalf("validateAccessProfile() error = %#v", err)
	}
}

func TestAccessInitChecksWebBaseURLBeforePublishingWebsite(t *testing.T) {
	command := newAccessInitCommand(&Runtime{
		profile: config.Profile{APIBaseURL: "https://api.poc.example"},
		region:  config.RegionCN,
	})
	command.SetArgs([]string{"--name", "POC", "--danmaku", "--website", t.TempDir(), "--config", filepath.Join(t.TempDir(), "access.yaml")})
	err := command.Execute()
	cliError, ok := err.(*output.Error)
	if !ok || cliError.Subtype != "PROFILE_WEB_BASE_URL_REQUIRED" {
		t.Fatalf("access init error = %#v", err)
	}
}

func TestAccessInitRequiresDanmakuFlag(t *testing.T) {
	command := newAccessInitCommand(&Runtime{})
	command.SetArgs([]string{"--name", "POC", "--config", filepath.Join(t.TempDir(), "access.yaml")})
	err := command.Execute()
	cliError, ok := err.(*output.Error)
	if !ok || cliError.Subtype != "DANMAKU_FLAG_REQUIRED" {
		t.Fatalf("access init error = %#v", err)
	}
}

func TestAccessInitPublishesWebsiteAndReturnsProfileDerivedSnippet(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests = append(requests, request.Method+" "+request.URL.Path)
		if request.Header.Get("Authorization") != "Bearer vme_cli_test" {
			t.Fatalf("missing bearer credential: %q", request.Header.Get("Authorization"))
		}
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/v1/cli/sdk-works/publish":
			var input api.PublishCreatorWebsiteRequest
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			if input.DisplayName != "POC" || input.CreatorDisplayName != "Tester" || len(input.SourceDigest) != 64 {
				t.Fatalf("unexpected website publication: %#v", input)
			}
			writeJSONResponse(writer, map[string]any{
				"creatorWorkId": "22222222-2222-4222-8222-222222222222",
				"workKey":       "wrk_test_danmaku", "displayName": "POC", "status": "DRAFT", "configVersion": 1,
				"publication": map[string]any{
					"clientWorkId": input.ClientWorkID, "sourceDigest": input.SourceDigest, "sourceUrl": nil,
					"releaseId": "33333333-3333-4333-8333-333333333333", "version": 1,
					"publishedAt": "2026-08-21T00:00:00Z", "unchanged": false,
				},
				"offer": nil, "features": []any{}, "capabilities": []any{},
				"createdAt": "2026-08-21T00:00:00Z", "updatedAt": "2026-08-21T00:00:00Z",
			})
		case request.Method == http.MethodPut && request.URL.Path == "/v1/cli/sdk-works/wrk_test_danmaku":
			body, _ := io.ReadAll(request.Body)
			if !strings.Contains(string(body), `"featureKey":"danmaku"`) || !strings.Contains(string(body), `"type":"PUBLIC"`) || !strings.Contains(string(body), `"priceCents":null`) {
				t.Fatalf("danmaku config missing from apply request: %s", body)
			}
			writeJSONResponse(writer, map[string]any{
				"creatorWorkId": "22222222-2222-4222-8222-222222222222",
				"workKey":       "wrk_test_danmaku", "displayName": "POC", "status": "ACTIVE", "configVersion": 2,
				"publication": nil, "offer": nil,
				"features":     []any{map[string]any{"featureKey": "danmaku", "title": "弹幕", "policy": map[string]any{"type": "PUBLIC"}, "status": "ACTIVE"}},
				"capabilities": []string{"danmaku"},
				"createdAt":    "2026-08-21T00:00:00Z", "updatedAt": "2026-08-21T00:00:00Z",
			})
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<!doctype html><body>POC</body>"), 0o644); err != nil {
		t.Fatal(err)
	}
	configDir := filepath.Join(root, "config")
	configured := config.Default(config.RegionCN)
	profile, err := configured.AddProfile("poc", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := configured.SetProfileWebBaseURL(profile.Name, "https://poc.viceme.cn"); err != nil {
		t.Fatal(err)
	}
	configured.CurrentProfile = profile.Name
	if _, err := config.Save(configDir, configured); err != nil {
		t.Fatal(err)
	}
	profile, err = configured.Resolve(profile.Name)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := credentialScopeForAPIBase(profile.ResolvedAPIBaseURL())
	if err != nil {
		t.Fatal(err)
	}
	store := securestore.NewMemory()
	manager := credentialauth.Manager{
		Store: store, Region: string(configured.DistributionRegion), ProfileID: profile.ID, ProfileName: profile.Name, Scope: scope,
	}
	if err := manager.Save(credentialauth.Credential{AccessToken: "vme_cli_test", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	accessConfigPath := filepath.Join(root, ".viceme", "access.yaml")
	exit := Execute([]string{
		"--profile", "poc", "access", "init", "--name", "POC", "--creator-display-name", "Tester", "--danmaku",
		"--website", root, "--config", accessConfigPath,
	}, Dependencies{
		Out: &stdout, ErrOut: &stderr, HTTPClient: server.Client(), Store: store,
		Environment: skillcontent.Environment{Home: root, ConfigDir: configDir},
	})
	if exit != 0 {
		t.Fatalf("access init failed: exit=%d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	var response struct {
		Data struct {
			ScriptURL    string `json:"scriptUrl"`
			EmbedSnippet string `json:"embedSnippet"`
			WorkKey      string `json:"workKey"`
			Work         struct {
				WorkKey string `json:"workKey"`
			} `json:"work"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if response.Data.ScriptURL != "https://poc.viceme.cn/viceme-sdk/v1/viceme.min.js" ||
		response.Data.WorkKey != "wrk_test_danmaku" ||
		response.Data.Work.WorkKey != "wrk_test_danmaku" ||
		!strings.Contains(response.Data.EmbedSnippet, `src="https://poc.viceme.cn/viceme-sdk/v1/viceme.min.js"`) {
		t.Fatalf("unexpected access result: %#v", response.Data)
	}
	wantRequests := []string{
		"POST /v1/cli/sdk-works/publish",
		"PUT /v1/cli/sdk-works/wrk_test_danmaku",
	}
	if strings.Join(requests, "\n") != strings.Join(wantRequests, "\n") {
		t.Fatalf("requests = %#v, want %#v", requests, wantRequests)
	}
	storedConfig, legacy, err := readAccessConfig(accessConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if legacy || storedConfig.ProfileID != profile.ID || storedConfig.WorkKey != "wrk_test_danmaku" {
		t.Fatalf("access config lost its Profile binding: %#v", storedConfig)
	}
	binding, found, err := loadWebsiteBinding(filepath.Join(root, ".viceme", "website.json"))
	if err != nil || !found || binding.ProfileID != profile.ID || binding.WorkKey != "wrk_test_danmaku" {
		t.Fatalf("website binding was not persisted: found=%t binding=%#v err=%v", found, binding, err)
	}
}

func TestReadAccessConfigAcceptsBeta6PublicDanmaku(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "access.yaml")
	legacyDocument := `schemaVersion: 1
workKey: wrk_dagou_tap
region: cn
displayName: Dagou Tap
priceCents: null
features:
  danmaku:
    title: 弹幕
    policy:
      type: PUBLIC
status: ACTIVE
configVersion: 3
`
	if err := os.WriteFile(filename, []byte(legacyDocument), 0o644); err != nil {
		t.Fatal(err)
	}

	got, legacy, err := readAccessConfig(filename)
	if err != nil {
		t.Fatalf("readAccessConfig() error = %v", err)
	}
	if !legacy || got.ProfileID != "" || got.WorkKey != "wrk_dagou_tap" || got.ConfigVersion != 3 {
		t.Fatalf("readAccessConfig() = %#v, legacy=%t", got, legacy)
	}
}

func TestReadAccessConfigRejectsLossyBeta6Migration(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "access.yaml")
	legacyDocument := `schemaVersion: 1
workKey: wrk_dagou_tap
region: cn
displayName: Dagou Tap
priceCents: 100
features:
  premium:
    title: Premium
    policy:
      type: WORK_ENTITLEMENT
status: ACTIVE
configVersion: 3
`
	if err := os.WriteFile(filename, []byte(legacyDocument), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := readAccessConfig(filename)
	cliError, ok := err.(*output.Error)
	if !ok || cliError.Subtype != "ACCESS_CONFIG_MIGRATION_UNSUPPORTED" {
		t.Fatalf("readAccessConfig() error = %#v", err)
	}
}

func TestAccessApplyMigratesBeta6PublicDanmakuAfterRemoteSuccess(t *testing.T) {
	var request api.ApplySdkWorkRequest
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, incoming *http.Request) {
		if incoming.Method != http.MethodPut || incoming.URL.Path != "/v1/cli/sdk-works/wrk_dagou_tap" {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewDecoder(incoming.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		writeJSONResponse(writer, map[string]any{
			"creatorWorkId": "22222222-2222-4222-8222-222222222222",
			"workKey":       "wrk_dagou_tap", "displayName": "Dagou Tap", "status": "ACTIVE", "configVersion": 4,
			"publication": nil, "offer": nil,
			"features":     []any{map[string]any{"featureKey": "danmaku", "title": "弹幕", "policy": map[string]any{"type": "PUBLIC"}, "status": "ACTIVE"}},
			"capabilities": []string{"danmaku"},
			"createdAt":    "2026-08-21T00:00:00Z", "updatedAt": "2026-08-21T00:00:00Z",
		})
	}))
	defer server.Close()

	filename := filepath.Join(t.TempDir(), "access.yaml")
	legacyDocument := `schemaVersion: 1
workKey: wrk_dagou_tap
region: cn
displayName: Dagou Tap
priceCents: null
features:
  danmaku:
    title: 弹幕
    policy:
      type: PUBLIC
status: ACTIVE
configVersion: 3
`
	if err := os.WriteFile(filename, []byte(legacyDocument), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	runtime := &Runtime{
		deps:              Dependencies{HTTPClient: server.Client(), Store: securestore.NewMemory()},
		printer:           &output.Printer{Out: &stdout, ErrOut: io.Discard},
		profile:           config.Profile{ID: "profile-poc", Name: "poc", WebBaseURL: "https://poc.viceme.cn"},
		region:            config.RegionCN,
		apiBaseURL:        server.URL,
		processCredential: &publicationCredential{raw: "vme_cli_test"},
	}
	command := newAccessApplyCommand(runtime)
	command.SetArgs([]string{"--config", filename})
	if err := command.Execute(); err != nil {
		t.Fatalf("access apply error = %v", err)
	}
	if request.ExpectedConfigVersion != 3 || len(request.Features) != 1 || request.Features[0].FeatureKey != "danmaku" || request.PriceCents != nil {
		t.Fatalf("unexpected migration request: %#v", request)
	}
	stored, legacy, err := readAccessConfig(filename)
	if err != nil || legacy || stored.ProfileID != "profile-poc" || stored.ConfigVersion != 4 {
		t.Fatalf("migrated config = %#v, legacy=%t, err=%v", stored, legacy, err)
	}
	data, err := os.ReadFile(filename)
	if err != nil || strings.Contains(string(data), "priceCents") {
		t.Fatalf("legacy field remained after migration: err=%v data=%s", err, data)
	}
}

func TestAccessConfigRejectsReservedSubscriptionPolicy(t *testing.T) {
	config := validAccessConfig()
	config.Features["danmaku"] = accessFeatureConfig{
		Title:  "弹幕",
		Policy: accessFeaturePolicy{Type: "ACTIVE_CREATOR_SUBSCRIPTION"},
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
