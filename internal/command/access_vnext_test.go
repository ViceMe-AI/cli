package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ViceMe-AI/cli/internal/output"
)

func TestAccessVNextFeaturesUsesServerContractShape(t *testing.T) {
	price := 990
	features := accessVNextFeatures(map[string]accessFeatureConfig{
		"members": {
			Title: "会员内容", Policy: accessFeaturePolicy{Type: "WORK_ENTITLEMENT"},
			PriceCents: &price, Status: "ACTIVE",
		},
		"followers": {
			Title: "关注可见", Policy: accessFeaturePolicy{Type: "FOLLOW_OWNER"}, Status: "ACTIVE",
		},
	})
	if len(features) != 2 || features[0].FeatureKey != "followers" || features[1].FeatureKey != "members" {
		t.Fatalf("features are not stable and sorted: %#v", features)
	}
	if features[0].Price != nil || features[1].Price == nil || features[1].Price.AmountCents != price {
		t.Fatalf("policy prices do not match the new API contract: %#v", features)
	}
}

func TestReadAccessVNextConfigRejectsLegacySdkWorkConfig(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "access.yaml")
	if err := os.WriteFile(filename, []byte("schemaVersion: 2\nworkKey: wrk_legacy\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := readAccessVNextConfig(filename)
	if err == nil || output.AsError(err).Subtype != "LEGACY_ACCESS_CONFIG_UNSUPPORTED" {
		t.Fatalf("legacy config error = %v", err)
	}
}
