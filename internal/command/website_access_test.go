package command

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/privatepath"
	"github.com/ViceMe-AI/cli/internal/replicapublication"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func accessV3Fixture() api.WorkSdkAccess {
	return api.WorkSdkAccess{WorkID: merchantEngagementWorkID, Keys: api.WorkSdkAccessKeys{Test: "wrk_test_permanent_access", Live: "wrk_live_permanent_access"}, Status: "ACTIVE", ConfigVersion: 3, Features: []string{"danmaku"}, AccessFeatures: []api.WorkAccessFeature{{FeatureKey: "old", Title: "Existing", PolicyType: "FOLLOW_OWNER", Status: "ACTIVE"}, {FeatureKey: "export", Title: "Export", PolicyType: "FOLLOW_OWNER", Status: "ACTIVE"}}, CreatedAt: "2026-09-12T00:00:00Z", UpdatedAt: "2026-09-12T00:00:00Z"}
}
func accessV3Response(access api.WorkSdkAccess) api.WebsiteAccessConfigurationResponse {
	return api.WebsiteAccessConfigurationResponse{NextAction: "INTEGRATE_HOST", CompletedSteps: []string{"WORK_BOUND", "ACCESS_CONFIGURED"}, WorkID: merchantEngagementWorkID, MerchantAccountID: merchantEngagementMerchantID, Access: &access, Availability: "ACTIVE"}
}
func accessV3InputFile(t *testing.T, project string) string {
	t.Helper()
	p := filepath.Join(project, "input.json")
	if err := os.WriteFile(p, []byte(`{"work":{"title":"Exporter","summary":"Export images"},"accessFeatures":[{"featureKey":"export","title":"Export","policyType":"FOLLOW_OWNER"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestWebsiteAccessAttemptsBusinessFirstAndResumesAuthentication(t *testing.T) {
	project := t.TempDir()
	input := accessV3InputFile(t, project)
	var requests []api.WebsiteAccessConfigurationRequest
	access := accessV3Fixture()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/cli/website-access/configurations":
			var body api.WebsiteAccessConfigurationRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			requests = append(requests, body)
			if len(requests) == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": "CLI_TOKEN_EXPIRED", "message": "login required", "statusCode": 401})
				return
			}
			_ = json.NewEncoder(w).Encode(accessV3Response(access))
		case "/v1/cli/merchant/works/" + merchantEngagementWorkID + "/sdk-access":
			_ = json.NewEncoder(w).Encode(access)
		default:
			t.Errorf("unexpected preflight: %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	exit, out := executeMerchantEngagementCommand(t, server, []string{"website", "access", "configure", "--project", project, "--input", input})
	if exit == 0 || !strings.Contains(out, "AUTHENTICATE_CREATOR") {
		t.Fatalf("missing login recovery: %d %s", exit, out)
	}
	saved, err := os.ReadFile(filepath.Join(project, ".viceme", websiteAccessStateName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(saved), merchantEngagementToken) {
		t.Fatal("persisted login credential")
	}
	exit, out = executeMerchantEngagementCommand(t, server, []string{"website", "access", "resume", "--project", project})
	if exit != 0 || !strings.Contains(out, "PLATFORM_CONFIGURED") {
		t.Fatalf("resume failed: %d %s", exit, out)
	}
	if len(requests) != 2 || requests[0].RequestID != requests[1].RequestID || requests[0].ProjectBindingID != requests[1].ProjectBindingID {
		t.Fatalf("request changed across login: %#v", requests)
	}
	if requests[0].Work.CanonicalOrigin != "" {
		t.Fatal("origin was invented")
	}
	exit, out = executeMerchantEngagementCommand(t, server, []string{"website", "access", "status", "--project", project})
	if exit != 0 || len(requests) != 2 || strings.Contains(out, `"phase": "VERIFIED"`) {
		t.Fatalf("status mutated or claimed verification: %d %s", exit, out)
	}
}

func TestWebsiteAccessHostReceiptMatchesCurrentConfiguration(t *testing.T) {
	project := t.TempDir()
	input := accessV3InputFile(t, project)
	access := accessV3Fixture()
	var requestID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var body api.WebsiteAccessConfigurationRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			requestID = body.RequestID
			_ = json.NewEncoder(w).Encode(accessV3Response(access))
		} else {
			_ = json.NewEncoder(w).Encode(access)
		}
	}))
	defer server.Close()
	exit, out := executeMerchantEngagementCommand(t, server, []string{"website", "access", "configure", "--project", project, "--input", input})
	if exit != 0 {
		t.Fatal(out)
	}
	receipt := websiteAccessHostReceipt{RequestID: requestID, WorkID: merchantEngagementWorkID, ConfigVersion: 3, ModifiedFiles: []string{"src/export.ts"}, Checks: []string{"cancel prevents export; successful entitlement executes once"}, Verified: true}
	data, _ := json.Marshal(receipt)
	file := filepath.Join(project, "receipt.json")
	_ = os.WriteFile(file, data, 0600)
	exit, out = executeMerchantEngagementCommand(t, server, []string{"website", "access", "resume", "--project", project, "--receipt", file})
	if exit != 0 || !strings.Contains(out, "ENRICH_WORK") {
		t.Fatalf("receipt failed: %d %s", exit, out)
	}
	access.ConfigVersion = 4
	exit, out = executeMerchantEngagementCommand(t, server, []string{"website", "access", "resume", "--project", project, "--receipt", file})
	if exit == 0 || !strings.Contains(out, "WEBSITE_ACCESS_RECEIPT_INVALID") {
		t.Fatalf("stale receipt accepted: %d %s", exit, out)
	}
}

func TestWebsiteAccessInputAndPreservationBoundaries(t *testing.T) {
	global := websiteAccessInput{AccessFeatures: []api.WebsiteAccessFeatureInput{{FeatureKey: "paid", Title: "Paid", PolicyType: "WORK_ENTITLEMENT", PricingIntent: &api.WebsiteAccessPricingIntent{Currency: "USD", AmountMinor: 100}}}}
	if err := normalizeWebsiteAccessInput(&global, "GLOBAL"); err != nil || global.AccessFeatures[0].Availability != "PENDING_CHANNEL" {
		t.Fatalf("global input: %#v %v", global, err)
	}
	for _, currency := range []string{"BTC", "", "usd"} {
		invalid := global
		invalid.AccessFeatures = append([]api.WebsiteAccessFeatureInput(nil), global.AccessFeatures...)
		invalid.AccessFeatures[0].PricingIntent = &api.WebsiteAccessPricingIntent{Currency: currency, AmountMinor: 100}
		if normalizeWebsiteAccessInput(&invalid, "GLOBAL") == nil {
			t.Fatalf("accepted currency %s", currency)
		}
	}
	old := accessV3Fixture()
	response := accessV3Response(old)
	current := old
	current.AccessFeatures = append([]api.WorkAccessFeature(nil), old.AccessFeatures...)
	current.AccessFeatures = current.AccessFeatures[1:]
	if verifyWebsiteAccessDelta(&current, nil, &response) == nil {
		t.Fatal("unrelated feature removal accepted")
	}
	current = old
	current.Keys.Live = "wrk_live_rotated"
	if verifyWebsiteAccessDelta(&current, nil, &response) == nil {
		t.Fatal("rotated key accepted")
	}
	input := websiteAccessInput{Work: api.WebsiteAccessWorkInput{CanonicalOrigin: "javascript:alert(1)"}}
	if normalizeWebsiteAccessInput(&input, "CN") == nil {
		t.Fatal("unsafe website URL accepted")
	}
	for name, work := range map[string]api.WebsiteAccessWorkInput{
		"slug":     {Slug: "Bad_Slug"},
		"reserved": {Slug: "works"},
		"title":    {Title: "   "},
		"summary":  {Summary: "   "},
		"body":     {BodyMarkdown: "   "},
		"origin":   {CanonicalOrigin: "https://example.test/path"},
	} {
		t.Run(name, func(t *testing.T) {
			input := websiteAccessInput{Work: work}
			if normalizeWebsiteAccessInput(&input, "CN") == nil {
				t.Fatalf("invalid Work input accepted: %#v", work)
			}
		})
	}
	normalized := websiteAccessInput{Work: api.WebsiteAccessWorkInput{Title: " Title ", Summary: " Summary ", BodyMarkdown: " Body ", CanonicalOrigin: "https://EXAMPLE.test:443/"}, AccessFeatures: []api.WebsiteAccessFeatureInput{{FeatureKey: "public", Title: " Public ", PolicyType: "PUBLIC"}}}
	if err := normalizeWebsiteAccessInput(&normalized, "CN"); err != nil {
		t.Fatal(err)
	}
	if normalized.Work.Title != "Title" || normalized.Work.Summary != "Summary" || normalized.Work.BodyMarkdown != "Body" || normalized.Work.CanonicalOrigin != "https://example.test" || normalized.AccessFeatures[0].Title != "Public" {
		t.Fatalf("input was not normalized like Shop: %#v", normalized)
	}
}

func TestWebsiteAccessRejectsStateSymlinkBeforeNetwork(t *testing.T) {
	project := t.TempDir()
	outside := filepath.Join(t.TempDir(), "private.json")
	_ = os.WriteFile(outside, []byte("sentinel"), 0600)
	if _, err := privatepath.EnsureDirectory(filepath.Join(project, ".viceme")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(project, ".viceme", websiteAccessStateName)); err != nil {
		t.Skip(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("network called for invalid state") }))
	defer server.Close()
	exit, _ := executeMerchantEngagementCommand(t, server, []string{"website", "access", "resume", "--project", project})
	if exit == 0 {
		t.Fatal("symlink state accepted")
	}
	data, _ := os.ReadFile(outside)
	if string(data) != "sentinel" {
		t.Fatal("outside file was changed")
	}
}

func TestWebsiteEnrichmentPreservesFullSnapshot(t *testing.T) {
	active := json.RawMessage(`{"id":"read-only-id","version":99,"summary":"Old","bodyMarkdown":"Keep body","templateType":"website-replica","tags":["tag"],"media":[{"id":"keep"}],"actionConfig":{"keep":true},"usageInstructions":"Keep instructions"}`)
	summary := "New summary"
	content, err := websiteEnrichedContent(active, websiteEnrichInput{Summary: &summary})
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	_ = json.Unmarshal(content, &value)
	if value["summary"] != summary || value["bodyMarkdown"] != "Keep body" || value["usageInstructions"] != "Keep instructions" || value["id"] != nil || value["version"] != nil {
		t.Fatalf("incorrect snapshot: %s", content)
	}
	if _, err = websiteEnrichedContent(json.RawMessage(`{"summary":"missing fields"}`), websiteEnrichInput{Summary: &summary}); err == nil {
		t.Fatal("incomplete snapshot accepted")
	}
	oversized := strings.Repeat("字", 501)
	if _, err = websiteEnrichedContent(active, websiteEnrichInput{Summary: &oversized}); err == nil {
		t.Fatal("oversized summary accepted")
	}
}

func TestWebsiteAccessLostResponsePreservesIntent(t *testing.T) {
	project := t.TempDir()
	input := accessV3InputFile(t, project)
	access := accessV3Fixture()
	var attempts []api.WebsiteAccessConfigurationRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var body api.WebsiteAccessConfigurationRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			attempts = append(attempts, body)
			if len(attempts) == 1 {
				w.WriteHeader(503)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": "RESPONSE_LOST", "message": "temporary failure"})
				return
			}
			_ = json.NewEncoder(w).Encode(accessV3Response(access))
		} else {
			_ = json.NewEncoder(w).Encode(access)
		}
	}))
	defer server.Close()
	exit, _ := executeMerchantEngagementCommand(t, server, []string{"website", "access", "configure", "--project", project, "--input", input})
	if exit == 0 {
		t.Fatal("lost response marked complete")
	}
	_ = os.WriteFile(input, []byte(`{"work":{"title":"Changed"},"accessFeatures":[]}`), 0600)
	exit, out := executeMerchantEngagementCommand(t, server, []string{"website", "access", "configure", "--project", project, "--input", input})
	if exit == 0 || !strings.Contains(out, "WEBSITE_ACCESS_REQUEST_PENDING") || len(attempts) != 1 {
		t.Fatalf("uncertain request replaced: %d %s", exit, out)
	}
	exit, out = executeMerchantEngagementCommand(t, server, []string{"website", "access", "resume", "--project", project})
	if exit != 0 || len(attempts) != 2 || !websiteJSONEqual(websiteTestJSON(t, attempts[0]), websiteTestJSON(t, attempts[1])) {
		t.Fatalf("resume changed intent: %d %s", exit, out)
	}
}

func TestWebsiteAccessRejectedRequestCanBeCorrected(t *testing.T) {
	project := t.TempDir()
	input := filepath.Join(project, "input.json")
	if err := os.WriteFile(input, []byte(`{"work":{"title":"First","slug":"first-slug"},"accessFeatures":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	access := accessV3Fixture()
	var attempts []api.WebsiteAccessConfigurationRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			var body api.WebsiteAccessConfigurationRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			attempts = append(attempts, body)
			if len(attempts) == 1 {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": "REQUEST_VALIDATION_FAILED", "message": "work.slug: invalid"})
				return
			}
			_ = json.NewEncoder(w).Encode(accessV3Response(access))
		default:
			_ = json.NewEncoder(w).Encode(access)
		}
	}))
	defer server.Close()
	exit, out := executeMerchantEngagementCommand(t, server, []string{"website", "access", "configure", "--project", project, "--input", input})
	if exit == 0 || !strings.Contains(out, "CORRECT_INPUT") {
		t.Fatalf("request validation was not actionable: %d %s", exit, out)
	}
	if err := os.WriteFile(input, []byte(`{"work":{"title":"Corrected","slug":"corrected-slug"},"accessFeatures":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	exit, out = executeMerchantEngagementCommand(t, server, []string{"website", "access", "configure", "--project", project, "--input", input})
	if exit != 0 || len(attempts) != 2 {
		t.Fatalf("corrected request did not run: %d %s attempts=%d", exit, out, len(attempts))
	}
	if attempts[0].RequestID == attempts[1].RequestID || attempts[0].ProjectBindingID != attempts[1].ProjectBindingID {
		t.Fatalf("correction did not rotate only the request identity: %#v", attempts)
	}
}

func TestWebsiteAccessBusinessConflictPreservesIntent(t *testing.T) {
	project := t.TempDir()
	input := accessV3InputFile(t, project)
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "WORK_REVISION_CONFLICT", "message": "Work changed"})
	}))
	defer server.Close()
	exit, _ := executeMerchantEngagementCommand(t, server, []string{"website", "access", "configure", "--project", project, "--input", input})
	if exit == 0 || attempts != 1 {
		t.Fatalf("business conflict was not returned: exit=%d attempts=%d", exit, attempts)
	}
	if err := os.WriteFile(input, []byte(`{"work":{"title":"Changed"},"accessFeatures":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	exit, out := executeMerchantEngagementCommand(t, server, []string{"website", "access", "configure", "--project", project, "--input", input})
	if exit == 0 || !strings.Contains(out, "WEBSITE_ACCESS_REQUEST_PENDING") || attempts != 1 {
		t.Fatalf("business conflict allowed request replacement: %d %s attempts=%d", exit, out, attempts)
	}
}

func websiteTestJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestWebsiteEnrichmentRecoversCommittedWriteWithoutRepeating(t *testing.T) {
	project := t.TempDir()
	input := accessV3InputFile(t, project)
	access := accessV3Fixture()
	merchant := merchantEngagementMerchantID
	active := json.RawMessage(`{"summary":"Old","bodyMarkdown":"Keep body","templateType":"website-replica","tags":[],"media":[],"actionConfig":{"keep":true}}`)
	work := api.MerchantWork{ID: merchantEngagementWorkID, Kind: "WEBSITE", Origin: "USER_AUTHORED", Slug: "exporter", Title: "Exporter", Status: "PUBLISHED", Revision: 2, Owner: api.WorkOwner{Kind: "MERCHANT", MerchantAccountID: &merchant}, Skill: json.RawMessage("null"), Service: json.RawMessage("null"), Website: &api.WebsiteWork{OwnershipStatus: "UNVERIFIED", VerificationVersion: 1}, ActiveRevision: active, DraftRevision: json.RawMessage("null"), CreatedAt: "2026-09-12T00:00:00Z", UpdatedAt: "2026-09-12T00:00:00Z"}
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/configurations"):
			_ = json.NewEncoder(w).Encode(accessV3Response(access))
		case strings.HasSuffix(r.URL.Path, "/sdk-access"):
			_ = json.NewEncoder(w).Encode(access)
		case r.Method == "PATCH" || r.Method == "PUT":
			writes++
			var body struct {
				ExpectedRevision int             `json:"expectedRevision"`
				Content          json.RawMessage `json:"content"`
				Status           string          `json:"status"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.ExpectedRevision != 2 || body.Status != "PUBLISHED" {
				t.Errorf("bad update: %#v", body)
			}
			var normalized map[string]any
			_ = json.Unmarshal(body.Content, &normalized)
			if normalized["summary"] != "New summary" || normalized["bodyMarkdown"] != "Keep body" || normalized["usageInstructions"] != "Use it" {
				t.Errorf("content was not normalized before write: %#v", normalized)
			}
			tags, _ := normalized["tags"].([]any)
			if len(tags) != 1 || tags[0] != "tag" {
				t.Errorf("tags were not normalized before write: %#v", tags)
			}
			work.ActiveRevision, _ = json.Marshal(normalized)
			work.Revision++
			w.WriteHeader(503)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "RESPONSE_LOST", "message": "temporary failure"})
		default:
			_ = json.NewEncoder(w).Encode(work)
		}
	}))
	defer server.Close()
	exit, out := executeMerchantEngagementCommand(t, server, []string{"website", "access", "configure", "--project", project, "--input", input})
	if exit != 0 {
		t.Fatal(out)
	}
	enrichment := filepath.Join(project, "enrich.json")
	_ = os.WriteFile(enrichment, []byte(`{"summary":" New summary ","bodyMarkdown":" Keep body ","tags":[" tag "],"usageInstructions":" Use it "}`), 0600)
	exit, out = executeMerchantEngagementCommand(t, server, []string{"website", "work", "enrich", "--project", project, "--input", enrichment})
	if exit != 0 || !strings.Contains(out, `"saved": true`) || writes != 1 {
		t.Fatalf("committed recovery failed: %d %s", exit, out)
	}
	exit, out = executeMerchantEngagementCommand(t, server, []string{"website", "work", "enrich", "--project", project, "--input", enrichment})
	if exit != 0 || writes != 1 {
		t.Fatalf("identical enrichment wrote again: %d %s", exit, out)
	}
	work.DraftRevision = json.RawMessage(`{"summary":"Someone else's draft"}`)
	exit, out = executeMerchantEngagementCommand(t, server, []string{"website", "work", "enrich", "--project", project, "--input", enrichment})
	if exit == 0 || writes != 1 || !strings.Contains(out, "WEBSITE_ENRICH_DRAFT_EXISTS") {
		t.Fatalf("draft overwritten: %d %s", exit, out)
	}
}

func TestWebsiteEnrichmentPreservesTrueRevisionConflict(t *testing.T) {
	project := t.TempDir()
	input := accessV3InputFile(t, project)
	access := accessV3Fixture()
	merchant := merchantEngagementMerchantID
	work := api.MerchantWork{ID: merchantEngagementWorkID, Kind: "WEBSITE", Origin: "USER_AUTHORED", Slug: "exporter", Title: "Exporter", Status: "PUBLISHED", Revision: 2, Owner: api.WorkOwner{Kind: "MERCHANT", MerchantAccountID: &merchant}, Skill: json.RawMessage("null"), Service: json.RawMessage("null"), Website: &api.WebsiteWork{OwnershipStatus: "UNVERIFIED", VerificationVersion: 1}, ActiveRevision: json.RawMessage(`{"summary":"Old","bodyMarkdown":"Keep body","templateType":"website-replica","tags":[],"media":[],"actionConfig":{}}`), DraftRevision: json.RawMessage("null"), CreatedAt: "2026-09-12T00:00:00Z", UpdatedAt: "2026-09-12T00:00:00Z"}
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/configurations"):
			_ = json.NewEncoder(w).Encode(accessV3Response(access))
		case strings.HasSuffix(r.URL.Path, "/sdk-access"):
			_ = json.NewEncoder(w).Encode(access)
		case r.Method == http.MethodPatch || r.Method == http.MethodPut:
			writes++
			work.Revision = 3
			work.ActiveRevision = json.RawMessage(`{"summary":"Concurrent","bodyMarkdown":"Keep body","templateType":"website-replica","tags":[],"media":[],"actionConfig":{}}`)
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "WORK_REVISION_CONFLICT", "message": "Work changed"})
		default:
			_ = json.NewEncoder(w).Encode(work)
		}
	}))
	defer server.Close()
	if exit, out := executeMerchantEngagementCommand(t, server, []string{"website", "access", "configure", "--project", project, "--input", input}); exit != 0 {
		t.Fatal(out)
	}
	enrichment := filepath.Join(project, "enrich.json")
	_ = os.WriteFile(enrichment, []byte(`{"summary":"Mine"}`), 0600)
	exit, out := executeMerchantEngagementCommand(t, server, []string{"website", "work", "enrich", "--project", project, "--input", enrichment})
	if exit == 0 || !strings.Contains(out, "WORK_REVISION_CONFLICT") || writes != 1 {
		t.Fatalf("concurrent update was not preserved: %d %s writes=%d", exit, out, writes)
	}
	exit, out = executeMerchantEngagementCommand(t, server, []string{"website", "work", "enrich", "--project", project})
	if exit == 0 || !strings.Contains(out, "WORK_REVISION_CONFLICT") || writes != 1 {
		t.Fatalf("conflicting recovery wrote again: %d %s writes=%d", exit, out, writes)
	}
}

func TestWebsiteAccessImportsReplicaReadOnly(t *testing.T) {
	project := t.TempDir()
	input := accessV3InputFile(t, project)
	access := accessV3Fixture()
	var request api.WebsiteAccessConfigurationRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			_ = json.NewDecoder(r.Body).Decode(&request)
			_ = json.NewEncoder(w).Encode(accessV3Response(access))
		} else {
			_ = json.NewEncoder(w).Encode(access)
		}
	}))
	defer server.Close()
	fingerprint, _, err := replicapublication.ProjectFingerprint(server.URL, "CN", project)
	if err != nil {
		t.Fatal(err)
	}
	binding := map[string]any{"schemaVersion": 1, "kind": "WebsiteReplica", "endpointOrigin": server.URL, "market": "CN", "projectFingerprint": fingerprint, "publication": map[string]any{"id": merchantEngagementApplicationID, "clientRequestId": merchantEngagementProductID, "status": "PUBLISHED", "statusUrl": "https://example.test/status"}, "merchant": map[string]any{"id": merchantEngagementMerchantID}, "frozenSource": map[string]any{"digest": strings.Repeat("a", 64), "sizeBytes": 1}, "work": map[string]any{"id": merchantEngagementWorkID, "url": "https://example.test/author/work"}, "replica": map[string]any{"id": merchantEngagementApplicationID, "shortCode": "VMR-ABCDEFGHIJKLMNOPQRST", "instruction": "VICEME-REPLICA:VMR-ABCDEFGHIJKLMNOPQRST"}, "product": map[string]any{"id": merchantEngagementProductID, "skuId": merchantEngagementApplicationID, "currency": "CNY", "priceCents": 0}, "version": map[string]any{"id": merchantEngagementApplicationID, "number": 1, "publishedAt": "2026-09-12T00:00:00Z"}, "updatedAt": "2026-09-12T00:00:00Z"}
	data := websiteTestJSON(t, binding)
	if _, err = privatepath.EnsureDirectory(filepath.Join(project, ".viceme")); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(project, ".viceme", "website-replica.json")
	_ = os.WriteFile(file, data, 0600)
	exit, out := executeMerchantEngagementCommand(t, server, []string{"website", "access", "configure", "--project", project, "--input", input})
	if exit != 0 {
		t.Fatalf("import failed: %d %s", exit, out)
	}
	if request.WorkID != merchantEngagementWorkID || request.MerchantAccountID != merchantEngagementMerchantID {
		t.Fatalf("binding not imported: %#v", request)
	}
	after, _ := os.ReadFile(file)
	if string(after) != string(data) {
		t.Fatal("Replica binding was modified")
	}
}

func TestWebsiteAccessLegacyCopyRetainsPendingAvailability(t *testing.T) {
	original := []api.WorkAccessFeature{{FeatureKey: "export", Title: "Export", PolicyType: "WORK_ENTITLEMENT", Status: "ACTIVE", Availability: "PENDING_CHANNEL", PricingIntent: &api.WebsiteAccessPricingIntent{Currency: "USD", AmountMinor: 199}}}
	converted := workAccessFeatureInputs(original)
	if len(converted) != 1 || converted[0].Availability != "PENDING_CHANNEL" || converted[0].PricingIntent == nil || converted[0].PricingIntent.Currency != "USD" {
		t.Fatalf("legacy command discarded v3 fields: %#v", converted)
	}
}

func TestWebsiteAccessGlobalConfiguresWithoutOpeningPayment(t *testing.T) {
	project := t.TempDir()
	input := filepath.Join(project, "global.json")
	_ = os.WriteFile(input, []byte(`{"work":{"title":"Exporter"},"accessFeatures":[{"featureKey":"export","title":"Export","policyType":"WORK_ENTITLEMENT","pricingIntent":{"currency":"USD","amountMinor":299}}]}`), 0600)
	access := accessV3Fixture()
	access.AccessFeatures = []api.WorkAccessFeature{{FeatureKey: "export", Title: "Export", PolicyType: "WORK_ENTITLEMENT", Status: "ACTIVE", Availability: "PENDING_CHANNEL", PricingIntent: &api.WebsiteAccessPricingIntent{Currency: "USD", AmountMinor: 299}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/cli/website-access/configurations":
			var request api.WebsiteAccessConfigurationRequest
			_ = json.NewDecoder(r.Body).Decode(&request)
			if request.Market != "GLOBAL" || request.AccessFeatures[0].Availability != "PENDING_CHANNEL" {
				t.Errorf("wrong global request: %#v", request)
			}
			response := accessV3Response(access)
			response.NextAction = "PENDING_CHANNEL"
			response.Availability = "PENDING_CHANNEL"
			_ = json.NewEncoder(w).Encode(response)
		case "/v1/cli/merchant/works/" + merchantEngagementWorkID + "/sdk-access":
			_ = json.NewEncoder(w).Encode(access)
		default:
			t.Errorf("unexpected payment or auth call: %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	t.Setenv(processAccessTokenEnvironment, merchantEngagementToken)
	home := t.TempDir()
	var stdout, stderr bytes.Buffer
	exit := Execute([]string{"website", "access", "configure", "--project", project, "--input", input}, Dependencies{Out: &stdout, ErrOut: &stderr, HTTPClient: server.Client(), APIBaseURL: server.URL, Region: config.RegionGlobal, Store: securestore.NewMemory(), Environment: skillcontent.Environment{Home: home, ConfigDir: filepath.Join(home, "config")}})
	if exit != 0 || !strings.Contains(stdout.String(), "PENDING_CHANNEL") || !strings.Contains(stdout.String(), "PLATFORM_CONFIGURED") {
		t.Fatalf("global failed: %d %s %s", exit, stdout.String(), stderr.String())
	}
}

func TestWebsiteAccessConcurrentOwnerStopsBeforeNetwork(t *testing.T) {
	project := t.TempDir()
	_, lock, err := openWebsiteAccessState(nil, project)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Unlock()
	input := accessV3InputFile(t, project)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("network called while another command owns project")
	}))
	defer server.Close()
	exit, out := executeMerchantEngagementCommand(t, server, []string{"website", "access", "configure", "--project", project, "--input", input})
	if exit == 0 || !strings.Contains(out, "WEBSITE_ACCESS_BUSY") {
		t.Fatalf("concurrent write accepted: %d %s", exit, out)
	}
	if _, err = os.Stat(filepath.Join(project, ".viceme", websiteAccessStateName)); !os.IsNotExist(err) {
		t.Fatal("concurrent request created another identity")
	}
}

func TestWebsiteAccessCanonicalOriginAndLifecycleContract(t *testing.T) {
	for _, value := range []string{"", "https://example.test"} {
		input := websiteAccessInput{Work: api.WebsiteAccessWorkInput{CanonicalOrigin: value}}
		if err := normalizeWebsiteAccessInput(&input, "CN"); err != nil {
			t.Fatalf("valid optional origin %q: %v", value, err)
		}
	}
	input := websiteAccessInput{Work: api.WebsiteAccessWorkInput{CanonicalOrigin: "http://example.test"}}
	if normalizeWebsiteAccessInput(&input, "CN") == nil {
		t.Fatal("HTTP origin incompatible with Shop accepted")
	}
	for _, status := range []string{"PENDING_CHANNEL", "DISABLED"} {
		input := websiteAccessInput{AccessFeatures: []api.WebsiteAccessFeatureInput{{FeatureKey: "export", Title: "Export", PolicyType: "WORK_ENTITLEMENT", Status: status, PricingIntent: &api.WebsiteAccessPricingIntent{Currency: "USD", AmountMinor: 199}}}}
		if err := normalizeWebsiteAccessInput(&input, "GLOBAL"); err != nil {
			t.Fatal(err)
		}
		feature := input.AccessFeatures[0]
		if feature.Availability != status {
			t.Fatalf("lost availability: %#v", feature)
		}
		if status == "PENDING_CHANNEL" && feature.Status != "ACTIVE" {
			t.Fatalf("pending lifecycle not normalized: %#v", feature)
		}
	}
}
