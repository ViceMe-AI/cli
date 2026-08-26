package command

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
)

func creatorAccessConfig() accessConfig {
	price := 1_000
	return accessConfig{
		SchemaVersion: 2,
		APIBaseURL:    "https://api.viceme.cn",
		WebBaseURL:    "https://viceme.cn",
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

func TestAccessInitConfiguresPublishedWebsiteWithoutRepublishing(t *testing.T) {
	t.Parallel()
	requests := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests <- request.Method + " " + request.URL.Path
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/cli/sdk-works/wrk_published_site":
			_, _ = io.WriteString(writer, `{"creatorWorkId":"22222222-2222-4222-8222-222222222222","workKey":"wrk_published_site","publication":null,"displayName":"Dagou Tap","status":"DRAFT","configVersion":1,"offers":[],"features":[],"capabilities":[],"createdAt":"2026-08-26T00:00:00Z","updatedAt":"2026-08-26T00:00:00Z"}`)
		case request.Method == http.MethodPut && request.URL.Path == "/v1/cli/sdk-works/wrk_published_site":
			var body api.ApplySdkWorkRequest
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if len(body.Features) != 2 || body.Features[0].Policy.Type != "FOLLOW_OWNER" ||
				body.Features[1].PriceCents == nil || *body.Features[1].PriceCents != 1_000 {
				t.Fatalf("apply request = %#v", body)
			}
			_, _ = io.WriteString(writer, `{"creatorWorkId":"22222222-2222-4222-8222-222222222222","workKey":"wrk_published_site","publication":null,"displayName":"Dagou Tap","status":"ACTIVE","configVersion":2,"offers":[],"features":[{"featureKey":"dingdong","title":"叮咚鸡","policy":{"type":"FOLLOW_OWNER"},"priceCents":null,"status":"ACTIVE"},{"featureKey":"emperor","title":"帝皇","policy":{"type":"WORK_ENTITLEMENT"},"priceCents":1000,"status":"ACTIVE"}],"capabilities":["auth","follow","access","checkout"],"createdAt":"2026-08-26T00:00:00Z","updatedAt":"2026-08-26T00:00:01Z"}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	source := t.TempDir()
	if err := writeWebsiteBinding(filepath.Join(source, websiteBindingName), websiteBinding{
		SchemaVersion: 2,
		APIBaseURL:    server.URL,
		WebBaseURL:    server.URL,
		ClientWorkID:  "11111111-1111-4111-8111-111111111111",
		WorkID:        "22222222-2222-4222-8222-222222222222",
		WorkKey:       "wrk_published_site",
		Region:        "cn",
		DisplayName:   "Dagou Tap",
	}); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "access.yaml")
	if _, err := executeCreatorAppCommand(t, server.URL,
		"access", "init", "--website", source, "--config", configPath,
		"--follow", "dingdong=叮咚鸡", "--purchase", "emperor=帝皇", "--price-minor", "1000",
	); err != nil {
		t.Fatalf("access init failed: %v", err)
	}
	if first, second := <-requests, <-requests; first != "GET /v1/cli/sdk-works/wrk_published_site" ||
		second != "PUT /v1/cli/sdk-works/wrk_published_site" {
		t.Fatalf("unexpected requests: %q, %q", first, second)
	}
}

func TestAccessConfigSupportsFollowAndPurchase(t *testing.T) {
	local := creatorAccessConfig()
	if err := validateAccessConfig(local); err != nil {
		t.Fatalf("validateAccessConfig() error = %v", err)
	}
	request := local.applyRequest()
	if len(request.Features) != 2 || request.Features[0].FeatureKey != "dingdong" || request.Features[1].FeatureKey != "emperor" {
		t.Fatalf("features are not stable and sorted: %#v", request.Features)
	}
	if request.Features[0].PriceCents != nil || request.Features[1].PriceCents == nil || *request.Features[1].PriceCents != 1_000 {
		t.Fatalf("feature prices are not scoped to purchase features: %#v", request.Features)
	}
}

func TestAccessConfigRejectsPurchaseWithoutPrice(t *testing.T) {
	local := creatorAccessConfig()
	feature := local.Features["emperor"]
	feature.PriceCents = nil
	local.Features["emperor"] = feature
	err := validateAccessConfig(local)
	cliError, ok := err.(*output.Error)
	if !ok || cliError.Subtype != "WORK_PRICE_REQUIRED" {
		t.Fatalf("validateAccessConfig() error = %#v", err)
	}
}

func TestBuildQuickAccessFeaturesAssignsPoliciesAndPrices(t *testing.T) {
	features, err := buildQuickAccessFeatures(
		[]string{"dingdong=叮咚鸡"},
		[]string{"basic=基础版", "pro=专业版"},
		[]string{"500", "1500"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if features["dingdong"].Policy.Type != "FOLLOW_OWNER" ||
		features["basic"].Policy.Type != "WORK_ENTITLEMENT" ||
		*features["basic"].PriceCents != 500 || *features["pro"].PriceCents != 1500 {
		t.Fatalf("features = %#v", features)
	}
}

func TestBuildQuickAccessFeaturesRejectsDuplicateKeys(t *testing.T) {
	_, err := buildQuickAccessFeatures([]string{"premium"}, []string{"premium"}, []string{"1000"})
	cliError, ok := err.(*output.Error)
	if !ok || cliError.Subtype != "ACCESS_FEATURE_DUPLICATE" {
		t.Fatalf("buildQuickAccessFeatures() error = %#v", err)
	}
}

func TestAccessInitRequiresExplicitWebsitePublication(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("website"), 0o644); err != nil {
		t.Fatal(err)
	}
	runtime := &Runtime{
		region:     config.RegionCN,
		apiBaseURL: config.APIBaseURL(config.RegionCN),
		profile: config.Profile{
			APIBaseURL:   config.APIBaseURL(config.RegionCN),
			WebBaseURL:   config.WebBaseURL(config.RegionCN),
			MarketRegion: config.RegionCN,
		},
		deps: Dependencies{ErrOut: io.Discard},
	}
	command := newAccessInitCommand(runtime)
	command.SetArgs([]string{
		"--website", root, "--name", "Dagou Tap",
		"--purchase", "premium", "--price-minor", "100",
	})
	err := command.Execute()
	cliError, ok := err.(*output.Error)
	if !ok || cliError.Subtype != "WEBSITE_PUBLICATION_REQUIRED" {
		t.Fatalf("access init error = %#v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, defaultAccessConfigPath)); !os.IsNotExist(statErr) {
		t.Fatalf("access init wrote config before explicit publication: %v", statErr)
	}
}
