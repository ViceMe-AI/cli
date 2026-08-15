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
		Origins:       []string{"https://creator.example.com"},
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
