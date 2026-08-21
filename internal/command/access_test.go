package command

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
)

func validAccessConfig() accessConfig {
	price := 1_000
	return accessConfig{
		SchemaVersion: 1,
		WorkKey:       "wrk_dagou_tap",
		Region:        "cn",
		DisplayName:   "Dagou Tap",
		Features: map[string]accessFeatureConfig{
			"dingdong": {Title: "叮咚鸡", Policy: accessFeaturePolicy{Type: "FOLLOW_OWNER"}},
			"emperor":  {Title: "帝皇", Policy: accessFeaturePolicy{Type: "WORK_ENTITLEMENT"}, PriceCents: &price},
		},
		Status:        "ACTIVE",
		ConfigVersion: 1,
	}
}

func TestAccessConfigSupportsFollowAndPurchase(t *testing.T) {
	config := validAccessConfig()
	if err := validateAccessConfig(config); err != nil {
		t.Fatalf("validateAccessConfig() error = %v", err)
	}
	request := config.applyRequest()
	if len(request.Features) != 2 || request.Features[0].FeatureKey != "dingdong" || request.Features[1].FeatureKey != "emperor" {
		t.Fatalf("features are not stable and sorted: %#v", request.Features)
	}
	if request.Features[0].PriceCents != nil || request.Features[1].PriceCents == nil || *request.Features[1].PriceCents != 1_000 {
		t.Fatalf("feature prices are not scoped to purchase features: %#v", request.Features)
	}
}

func TestAccessConfigRejectsPurchaseWithoutPrice(t *testing.T) {
	config := validAccessConfig()
	feature := config.Features["emperor"]
	feature.PriceCents = nil
	config.Features["emperor"] = feature
	err := validateAccessConfig(config)
	if err == nil {
		t.Fatal("validateAccessConfig() error = nil, want WORK_PRICE_REQUIRED")
	}
	cliError, ok := err.(*output.Error)
	if !ok || cliError.Subtype != "WORK_PRICE_REQUIRED" {
		t.Fatalf("validateAccessConfig() error = %#v", err)
	}
}

func TestAccessConfigRejectsReservedSubscriptionPolicy(t *testing.T) {
	config := validAccessConfig()
	config.Features["emperor"] = accessFeatureConfig{
		Title:  "帝皇",
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

func TestReadAccessConfigMigratesLegacyWorkPrice(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "access.yaml")
	if err := os.WriteFile(filename, []byte(`schemaVersion: 1
workKey: wrk_dagou_tap
region: cn
displayName: Dagou Tap
priceCents: 1000
features:
  premium:
    title: Premium
    policy:
      type: WORK_ENTITLEMENT
status: ACTIVE
configVersion: 1
`), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := readAccessConfig(filename)
	if err != nil {
		t.Fatalf("readAccessConfig() error = %v", err)
	}
	if config.LegacyPrice != nil || config.Features["premium"].PriceCents == nil || *config.Features["premium"].PriceCents != 1_000 {
		t.Fatalf("legacy price was not migrated: %#v", config)
	}
}

func TestParseAccessFeatureSpecUsesKeyAsDefaultTitle(t *testing.T) {
	key, title, err := parseAccessFeatureSpec("premium")
	if err != nil {
		t.Fatalf("parseAccessFeatureSpec() error = %v", err)
	}
	if key != "premium" || title != "premium" {
		t.Fatalf("parseAccessFeatureSpec() = %q, %q", key, title)
	}
}

func TestParseAccessFeatureSpecAcceptsLocalizedTitle(t *testing.T) {
	key, title, err := parseAccessFeatureSpec("premium=付费内容")
	if err != nil {
		t.Fatalf("parseAccessFeatureSpec() error = %v", err)
	}
	if key != "premium" || title != "付费内容" {
		t.Fatalf("parseAccessFeatureSpec() = %q, %q", key, title)
	}
}

func TestBuildQuickAccessFeaturesAssignsPolicies(t *testing.T) {
	features, err := buildQuickAccessFeatures(
		[]string{"dingdong=叮咚鸡"},
		[]string{"emperor=帝皇"},
		[]string{"1000"},
	)
	if err != nil {
		t.Fatalf("buildQuickAccessFeatures() error = %v", err)
	}
	if features["dingdong"].Policy.Type != "FOLLOW_OWNER" {
		t.Fatalf("dingdong policy = %q", features["dingdong"].Policy.Type)
	}
	if features["emperor"].Policy.Type != "WORK_ENTITLEMENT" {
		t.Fatalf("emperor policy = %q", features["emperor"].Policy.Type)
	}
}

func TestBuildQuickAccessFeaturesRejectsDuplicateKeys(t *testing.T) {
	_, err := buildQuickAccessFeatures(
		[]string{"premium"},
		[]string{"premium"},
		[]string{"1000"},
	)
	if err == nil {
		t.Fatal("buildQuickAccessFeatures() error = nil, want duplicate")
	}
	cliError, ok := err.(*output.Error)
	if !ok || cliError.Subtype != "ACCESS_FEATURE_DUPLICATE" {
		t.Fatalf("buildQuickAccessFeatures() error = %#v", err)
	}
}

func TestBuildQuickAccessFeaturesSupportsMultiplePrices(t *testing.T) {
	features, err := buildQuickAccessFeatures(
		nil,
		[]string{"basic=基础版", "pro=专业版"},
		[]string{"500", "1500"},
	)
	if err != nil {
		t.Fatalf("buildQuickAccessFeatures() error = %v", err)
	}
	if *features["basic"].PriceCents != 500 || *features["pro"].PriceCents != 1500 {
		t.Fatalf("feature prices = %#v", features)
	}
}

func TestEnsurePublishedWebsiteBindingPublishesBeforeAccessSetup(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("website"), 0o644); err != nil {
		t.Fatal(err)
	}
	published := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/cli/sdk-works/publish" || request.Method != http.MethodPost {
			http.NotFound(writer, request)
			return
		}
		if request.Header.Get("Authorization") == "" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		published = true
		writeJSONResponse(writer, map[string]any{
			"creatorWorkId": "22222222-2222-4222-8222-222222222222",
			"workKey":       "wrk_published_from_access", "displayName": "Dagou Tap",
			"status": "DRAFT", "configVersion": 1, "offers": []any{}, "features": []any{},
			"capabilities": []any{"auth", "follow", "access"},
			"publication": map[string]any{
				"clientWorkId": body["clientWorkId"], "sourceDigest": body["sourceDigest"],
				"sourceUrl": nil, "descriptionZhCn": nil, "descriptionEnUs": nil, "coverUrl": nil,
				"releaseId": "33333333-3333-4333-8333-333333333333", "version": 1,
				"publishedAt": "2026-08-21T00:00:00.000Z", "unchanged": false,
			},
		})
	}))
	defer server.Close()
	runtime := &Runtime{
		region: config.RegionCN, apiBaseURL: server.URL,
		deps:              Dependencies{HTTPClient: server.Client(), ErrOut: io.Discard},
		processCredential: &publicationCredential{raw: "vme_cli_" + strings.Repeat("a", 43)},
	}

	binding, err := ensurePublishedWebsiteBinding(context.Background(), runtime, root, "Dagou Tap")
	if err != nil {
		t.Fatalf("ensurePublishedWebsiteBinding() error = %v", err)
	}
	if !published || binding.WorkKey != "wrk_published_from_access" {
		t.Fatalf("website was not published before access setup: %#v", binding)
	}
}
