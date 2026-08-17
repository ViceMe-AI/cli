package command

import (
	"testing"

	"github.com/ViceMe-AI/cli/internal/output"
)

func validAccessConfig() accessConfig {
	product := "dagou-tap"
	return accessConfig{
		SchemaVersion: 1,
		WorkKey:       "wrk_dagou_tap",
		Region:        "cn",
		DisplayName:   "Dagou Tap",
		ProductSlug:   &product,
		Features: map[string]accessFeatureConfig{
			"dingdong": {Title: "叮咚鸡", Policy: accessFeaturePolicy{Type: "FOLLOW_OWNER"}},
			"emperor":  {Title: "帝皇", Policy: accessFeaturePolicy{Type: "PURCHASE_BOUND_PRODUCT"}},
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
}

func TestAccessConfigSupportsPublicDanmaku(t *testing.T) {
	config := validAccessConfig()
	config.ProductSlug = nil
	delete(config.Features, "emperor")
	config.Features["danmaku"] = accessFeatureConfig{
		Title:  "弹幕",
		Policy: accessFeaturePolicy{Type: "PUBLIC"},
	}
	if err := validateAccessConfig(config); err != nil {
		t.Fatalf("validateAccessConfig() error = %v", err)
	}
	request := config.applyRequest()
	if len(request.Features) != 2 || request.Features[0].FeatureKey != "danmaku" || request.Features[0].Policy.Type != "PUBLIC" {
		t.Fatalf("danmaku feature is not stable: %#v", request.Features)
	}
}

func TestAccessInitExposesDanmakuFlag(t *testing.T) {
	flag := newAccessInitCommand(&Runtime{}).Flags().Lookup("danmaku")
	if flag == nil || flag.DefValue != "false" {
		t.Fatalf("danmaku flag = %#v", flag)
	}
}

func TestAccessConfigRejectsPurchaseWithoutProduct(t *testing.T) {
	config := validAccessConfig()
	config.ProductSlug = nil
	err := validateAccessConfig(config)
	if err == nil {
		t.Fatal("validateAccessConfig() error = nil, want WORK_PRODUCT_NOT_BOUND")
	}
	cliError, ok := err.(*output.Error)
	if !ok || cliError.Subtype != "WORK_PRODUCT_NOT_BOUND" {
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
		nil,
	)
	if err != nil {
		t.Fatalf("buildQuickAccessFeatures() error = %v", err)
	}
	if features["dingdong"].Policy.Type != "FOLLOW_OWNER" {
		t.Fatalf("dingdong policy = %q", features["dingdong"].Policy.Type)
	}
	if features["emperor"].Policy.Type != "PURCHASE_BOUND_PRODUCT" {
		t.Fatalf("emperor policy = %q", features["emperor"].Policy.Type)
	}
}

func TestBuildQuickAccessFeaturesRejectsDuplicateKeys(t *testing.T) {
	_, err := buildQuickAccessFeatures(
		[]string{"premium"},
		[]string{"premium"},
		nil,
	)
	if err == nil {
		t.Fatal("buildQuickAccessFeatures() error = nil, want duplicate")
	}
	cliError, ok := err.(*output.Error)
	if !ok || cliError.Subtype != "ACCESS_FEATURE_DUPLICATE" {
		t.Fatalf("buildQuickAccessFeatures() error = %#v", err)
	}
}
