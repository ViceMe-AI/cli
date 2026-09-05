package command

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const setupTestApplication = "10000000-0000-4000-8000-000000000001"

func TestCreatorPageSetupResolvesApprovedApplicationForExactMerchant(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "vme_cli_"+strings.Repeat("a", 43))
	const merchantID = "20000000-0000-4000-8000-000000000001"
	var resolved atomic.Int32
	var read atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "unexpected mutation", 500)
			return
		}
		switch r.URL.Path {
		case "/v1/cli/auth/status":
			writeMerchantOnboardingAuth(w)
			return
		case "/v1/cli/merchant/onboarding/targets/" + merchantID + "/page-setup":
			resolved.Add(1)
		case "/v1/cli/merchant/onboarding/" + setupTestApplication + "/page-setup":
			read.Add(1)
		default:
			http.NotFound(w, r)
			return
		}
		writeJSONResponse(w, map[string]any{"applicationId": setupTestApplication, "applicationStatus": "APPROVED", "displayName": "Creator", "selection": map[string]any{"mode": "IMPORT_EXISTING"}, "selectedAt": "2026-09-05T00:00:00Z"})
	}))
	defer server.Close()
	exit, envelope := executeMerchantOnboardingCommand(t, server, "merchant", "onboarding", "page-setup", "--merchant", merchantID, "--wait")
	if exit != 0 {
		t.Fatalf("failed to resume approved application: %#v", envelope)
	}
	data := envelope["data"].(map[string]any)
	if resolved.Load() != 1 || read.Load() != 1 || data["status"] != "selected" || data["applicationId"] != setupTestApplication {
		t.Fatalf("wrong resumed application: %#v", data)
	}
}

func TestCreatorPageSetupResumesEachChoiceWithoutLoginOrApplying(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "vme_cli_"+strings.Repeat("a", 43))
	for _, mode := range []string{"IMPORT_EXISTING", "BONJOUR", "SKIP"} {
		t.Run(mode, func(t *testing.T) {
			var writes atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					writes.Add(1)
					http.Error(w, "unexpected write", 500)
					return
				}
				if r.URL.Path == "/v1/cli/auth/status" {
					writeMerchantOnboardingAuth(w)
					return
				}
				if r.URL.Path != "/v1/cli/merchant/onboarding/"+setupTestApplication+"/page-setup" {
					http.NotFound(w, r)
					return
				}
				writeJSONResponse(w, map[string]any{"applicationId": setupTestApplication, "applicationStatus": "APPROVED", "displayName": "Creator", "selection": map[string]any{"mode": mode}, "selectedAt": "2026-09-05T00:00:00Z"})
			}))
			defer server.Close()
			exit, envelope := executeMerchantOnboardingCommand(t, server, "merchant", "onboarding", "page-setup", setupTestApplication, "--wait")
			if exit != 0 {
				t.Fatalf("failed: %#v", envelope)
			}
			data := envelope["data"].(map[string]any)
			if data["status"] != "selected" || data["selection"].(map[string]any)["mode"] != mode || writes.Load() != 0 {
				t.Fatalf("bad result: %#v", data)
			}
			if !strings.Contains(data["setupUrl"].(string), "/creator-page-setup?application="+setupTestApplication) {
				t.Fatalf("wrong setup link: %#v", data)
			}
		})
	}
}

func TestCreatorPageSetupTimeoutOnlyPausesPageSetup(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "vme_cli_"+strings.Repeat("a", 43))
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/cli/auth/status" {
			writeMerchantOnboardingAuth(w)
			return
		}
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/page-setup") {
			http.Error(w, "unexpected mutation", 500)
			return
		}
		calls.Add(1)
		writeJSONResponse(w, map[string]any{"applicationId": setupTestApplication, "applicationStatus": "SUBMITTED", "displayName": "Creator", "selection": nil, "selectedAt": nil})
	}))
	defer server.Close()
	exit, envelope := executeMerchantOnboardingCommand(t, server, "merchant", "onboarding", "page-setup", setupTestApplication, "--wait", "--timeout", "50ms")
	if exit != 0 {
		t.Fatalf("timeout should pause: %#v", envelope)
	}
	data := envelope["data"].(map[string]any)
	if data["status"] != "pending" || data["timedOut"] != true || data["selection"] != nil || calls.Load() != 1 {
		t.Fatalf("bad pending state: %#v", data)
	}
}

func TestCreatorPageSetupWaitReceivesWebChoiceAndOptionalDetails(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "vme_cli_"+strings.Repeat("a", 43))
	var reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "unexpected mutation", 500)
			return
		}
		if r.URL.Path == "/v1/cli/auth/status" {
			writeMerchantOnboardingAuth(w)
			return
		}
		if r.URL.Path != "/v1/cli/merchant/onboarding/"+setupTestApplication+"/page-setup" {
			http.NotFound(w, r)
			return
		}
		result := map[string]any{"applicationId": setupTestApplication, "applicationStatus": "SUBMITTED", "displayName": "Creator", "selection": nil, "selectedAt": nil}
		if reads.Add(1) > 1 {
			result["selection"] = map[string]any{
				"mode":     "BONJOUR",
				"contacts": []map[string]string{{"platform": "GitHub", "value": "https://github.com/example"}},
				"works":    []map[string]string{{"title": "My project"}},
			}
			result["selectedAt"] = "2026-09-05T00:00:00Z"
		}
		writeJSONResponse(w, result)
	}))
	defer server.Close()
	exit, envelope := executeMerchantOnboardingCommand(t, server, "merchant", "onboarding", "page-setup", setupTestApplication, "--wait", "--timeout", "10s")
	if exit != 0 {
		t.Fatalf("failed waiting for web choice: %#v", envelope)
	}
	data := envelope["data"].(map[string]any)
	selection := data["selection"].(map[string]any)
	if data["status"] != "selected" || data["timedOut"] != false || reads.Load() != 2 {
		t.Fatalf("unexpected waiting result: %#v", data)
	}
	contacts := selection["contacts"].([]any)
	works := selection["works"].([]any)
	if contacts[0].(map[string]any)["value"] != "https://github.com/example" || works[0].(map[string]any)["title"] != "My project" {
		t.Fatalf("lost optional details: %#v", selection)
	}
}
