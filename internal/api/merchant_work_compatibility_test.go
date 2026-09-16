package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMerchantWorkAcceptsLegacyOmittedService(t *testing.T) {
	for _, omit := range []bool{false, true} {
		work := testWebsiteMerchantWork("UNVERIFIED", 1, 1)
		data, err := json.Marshal(work)
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatal(err)
		}
		if omit {
			delete(payload, "service")
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v1/cli/merchant/works" {
				json.NewEncoder(w).Encode(map[string]any{"items": []any{payload}})
			} else {
				json.NewEncoder(w).Encode(payload)
			}
		}))
		client := NewClient(server.URL, server.Client(), staticToken("vme_cli_test"), "viceme/test")
		if _, err := client.ListMerchantWorks(context.Background(), testMerchantAccountID); err != nil {
			t.Errorf("list omit=%t: %v", omit, err)
		}
		if _, err := client.GetMerchantWork(context.Background(), testWebsiteWorkID, testMerchantAccountID); err != nil {
			t.Errorf("get omit=%t: %v", omit, err)
		}
		server.Close()
	}
}

// Legacy compatibility applies only to an inapplicable service child.
func TestMerchantWorkRejectsMissingRequiredAndConflictingChildren(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*MerchantWork)
	}{
		{"website with service", func(w *MerchantWork) { w.Service = json.RawMessage(`{"offeringId":"unexpected"}`) }},
		{"website without website", func(w *MerchantWork) { w.Website = nil }},
		{"website without skill null", func(w *MerchantWork) { w.Skill = nil }},
		{"missing revision", func(w *MerchantWork) { w.ActiveRevision = nil }},
		{"service without service", func(w *MerchantWork) { w.Kind = "SERVICE"; w.Website = nil; w.Service = nil }},
		{"service with null service", func(w *MerchantWork) { w.Kind = "SERVICE"; w.Website = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			work := testWebsiteMerchantWork("UNVERIFIED", 1, 1)
			test.mutate(&work)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				data, err := json.Marshal(work)
				if err != nil {
					t.Error(err)
					return
				}
				var payload map[string]any
				if err := json.Unmarshal(data, &payload); err != nil {
					t.Error(err)
					return
				}
				if work.Skill == nil {
					delete(payload, "skill")
				}
				if work.ActiveRevision == nil {
					delete(payload, "activeRevision")
				}
				if work.Service == nil {
					delete(payload, "service")
				}
				json.NewEncoder(w).Encode(map[string]any{"items": []any{payload}})
			}))
			defer server.Close()
			client := NewClient(server.URL, server.Client(), staticToken("vme_cli_test"), "viceme/test")
			if _, err := client.ListMerchantWorks(context.Background(), testMerchantAccountID); err == nil {
				t.Fatal("invalid Work was accepted")
			}
		})
	}
}

func TestMerchantWorkTutorialsUpdateAndReadback(t *testing.T) {
	work := testWebsiteMerchantWork("UNVERIFIED", 1, 1)
	tutorials := &WorkTutorials{Instructions: "部署说明", VideoLinks: []WorkVideoLink{{Type: "VIDEO_LINK", Title: "教程", URL: "https://www.bilibili.com/video/BV1example"}}}
	request, err := json.Marshal(map[string]any{"merchantAccountId": testMerchantAccountID, "expectedRevision": 1, "tutorials": tutorials})
	if err != nil {
		t.Fatal(err)
	}
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			var body map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
				return
			}
			if len(body) != 3 || body["content"] != nil || body["status"] != nil {
				t.Error("tutorial update must not publish a work revision")
			}
			if err := json.Unmarshal(body["tutorials"], &work.Tutorials); err != nil {
				t.Error(err)
				return
			}
			writes++
		}
		json.NewEncoder(w).Encode(work)
	}))
	defer server.Close()
	client := NewClient(server.URL, server.Client(), staticToken("vme_cli_test"), "viceme/test")
	updated, err := client.UpdateMerchantWork(context.Background(), testWebsiteWorkID, request)
	if err != nil {
		t.Fatal(err)
	}
	readback, err := client.GetMerchantWork(context.Background(), testWebsiteWorkID, testMerchantAccountID)
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range []MerchantWork{updated, readback} {
		if result.Tutorials == nil || result.Tutorials.Instructions != tutorials.Instructions || len(result.Tutorials.VideoLinks) != 1 || result.Tutorials.VideoLinks[0] != tutorials.VideoLinks[0] {
			t.Fatalf("tutorials lost in API response: %#v", result.Tutorials)
		}
	}
	if writes != 1 {
		t.Fatal("unexpected repeated update")
	}
}
