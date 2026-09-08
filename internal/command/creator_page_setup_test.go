package command

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

const setupTestApplication = "10000000-0000-4000-8000-000000000001"

func TestCreatorPageSetupIsRetiredWithoutOpeningLegacySetup(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "legacy setup must not call the API", http.StatusInternalServerError)
	}))
	defer server.Close()
	exit, envelope := executeMerchantOnboardingCommand(t, server, "merchant", "onboarding", "page-setup", setupTestApplication)
	if exit == 0 || envelope["error"].(map[string]any)["code"] != "PAGE_SETUP_RETIRED" {
		t.Fatalf("legacy setup should be rejected: %#v", envelope)
	}
	if requests.Load() != 0 {
		t.Fatalf("retired setup must not call the API: %d requests", requests.Load())
	}
}
