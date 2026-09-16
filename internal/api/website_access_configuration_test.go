package api

import (
	"encoding/json"
	"testing"
)

func TestWebsiteAccessPendingChannelAndOriginlessWork(t *testing.T) {
	feature := WorkAccessFeature{FeatureKey: "export", Title: "Export", PolicyType: "WORK_ENTITLEMENT", Availability: "PENDING_CHANNEL", PricingIntent: &WebsiteAccessPricingIntent{Currency: "USD", AmountMinor: 199}, Status: "ACTIVE"}
	if !validWorkAccessFeatures([]WorkAccessFeature{feature}) {
		t.Fatal("valid pending channel feature rejected")
	}
	feature.Availability = "DISABLED"
	feature.Status = "DISABLED"
	if !validWorkAccessFeatures([]WorkAccessFeature{feature}) {
		t.Fatal("disabled pending feature rejected")
	}
	feature.Availability = "ACTIVE"
	if validWorkAccessFeatures([]WorkAccessFeature{feature}) {
		t.Fatal("active paid feature without product accepted")
	}
	website := WebsiteWork{OwnershipStatus: "UNVERIFIED", VerificationVersion: 1}
	if err := website.validateAPIResponse(); err != nil {
		t.Fatal(err)
	}
	website.OwnershipStatus = "VERIFIED"
	if website.validateAPIResponse() == nil {
		t.Fatal("verified ownership without origin accepted")
	}
}

func TestWebsiteAccessContractPriceUnionAndPendingStatus(t *testing.T) {
	for _, priceJSON := range []string{`{"currency":"CNY","amountCents":199}`, `{"currency":"CNY","amountMinor":199}`, `{"currency":"USD","amountMinor":199}`} {
		var price WorkAccessPrice
		if err := json.Unmarshal([]byte(priceJSON), &price); err != nil || !validWorkAccessPrice(&price) || price.MinorUnits() != 199 {
			t.Fatalf("valid contract price rejected: %s %#v %v", priceJSON, price, err)
		}
		encoded, _ := json.Marshal(price)
		var roundtrip WorkAccessPrice
		_ = json.Unmarshal(encoded, &roundtrip)
		if !workAccessPricesEqual(&price, &roundtrip) {
			t.Fatalf("lost price: %s", encoded)
		}
	}
	for _, price := range []WorkAccessPrice{{Currency: "USD", AmountCents: 199}, {Currency: "USD", AmountMinor: -1}, {Currency: "CNY", AmountCents: 199, AmountMinor: 199}} {
		if validWorkAccessPrice(&price) {
			t.Fatalf("invalid price accepted: %#v", price)
		}
	}
	feature := WorkAccessFeature{FeatureKey: "export", Title: "Export", PolicyType: "WORK_ENTITLEMENT", Status: "PENDING_CHANNEL", PricingIntent: &WebsiteAccessPricingIntent{Currency: "USD", AmountMinor: 199}}
	if !validWorkAccessFeatures([]WorkAccessFeature{feature}) {
		t.Fatal("status-only pending response rejected")
	}
	feature.PolicyType = "FOLLOW_OWNER"
	feature.PricingIntent = nil
	if validWorkAccessFeatures([]WorkAccessFeature{feature}) {
		t.Fatal("pending follow accepted")
	}
}
