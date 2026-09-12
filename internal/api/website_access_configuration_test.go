package api

import "testing"

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
