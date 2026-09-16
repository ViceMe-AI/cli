package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/output"
)

func TestMerchantWorkOriginContract(t *testing.T) {
	for _, test := range []struct {
		name           string
		origin, domain any
		status         string
		omit           string
		verifiedAt     any
		wantError      bool
	}{
		{name: "bound verified", origin: "https://example.com", domain: "example.com", status: "VERIFIED"},
		{name: "origin free unverified", status: "UNVERIFIED"},
		{name: "origin free revoked", status: "REVOKED"},
		{name: "origin free verified rejected", status: "VERIFIED", wantError: true},
		{name: "origin without domain", origin: "https://example.com", status: "UNVERIFIED", wantError: true},
		{name: "domain without origin", domain: "example.com", status: "UNVERIFIED", wantError: true},
		{name: "missing origin", omit: "canonicalOrigin", status: "UNVERIFIED", wantError: true},
		{name: "missing domain", omit: "domainAscii", status: "UNVERIFIED", wantError: true},
		{name: "empty origin", origin: "", domain: "example.com", status: "UNVERIFIED", wantError: true},
		{name: "empty domain", origin: "https://example.com", domain: "", status: "UNVERIFIED", wantError: true},
		{name: "oversized domain", origin: "https://example.com", domain: strings.Repeat("a", 254), status: "UNVERIFIED", wantError: true},
		{name: "invalid origin type", origin: 42, domain: "example.com", status: "UNVERIFIED", wantError: true},
		{name: "invalid domain type", origin: "https://example.com", domain: 42, status: "UNVERIFIED", wantError: true},
		{name: "origin free invalid timestamp", status: "UNVERIFIED", verifiedAt: "invalid", wantError: true},
		{name: "origin free invalid status", status: "UNKNOWN", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			bound := testWebsiteMerchantWork("VERIFIED", 1, 1)
			data, err := json.Marshal(bound)
			if err != nil {
				t.Fatal(err)
			}
			var payload map[string]any
			if err := json.Unmarshal(data, &payload); err != nil {
				t.Fatal(err)
			}
			website := payload["website"].(map[string]any)
			website["canonicalOrigin"], website["domainAscii"] = test.origin, test.domain
			website["ownershipStatus"], website["verifiedAt"] = test.status, test.verifiedAt
			if test.omit != "" {
				delete(website, test.omit)
			}
			payload["tutorials"] = map[string]any{"instructions": "部署说明保持不变", "videoLinks": []any{map[string]any{"type": "VIDEO", "title": "示例视频", "url": "https://example.com/video"}}}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("unexpected method: %s", r.Method)
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				if r.URL.Path == "/v1/cli/merchant/works" {
					_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{bound, payload}})
				} else {
					_ = json.NewEncoder(w).Encode(payload)
				}
			}))
			defer server.Close()
			client := NewClient(server.URL, server.Client(), staticToken("vme_cli_test"), "viceme/test")
			work, getErr := client.GetMerchantWork(context.Background(), testWebsiteWorkID, testMerchantAccountID)
			list, listErr := client.ListMerchantWorks(context.Background(), testMerchantAccountID)
			for name, err := range map[string]error{"get": getErr, "mixed list": listErr} {
				if test.wantError {
					if err == nil || output.AsError(err).Subtype != "RESPONSE_INVALID" {
						t.Fatalf("%s: expected RESPONSE_INVALID, got %v", name, err)
					}
				} else if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
			}
			if test.wantError {
				return
			}
			if len(list.Items) != 2 {
				t.Fatalf("lost mixed list items: %d", len(list.Items))
			}
			for _, got := range []MerchantWork{work, list.Items[1]} {
				if got.Tutorials == nil || got.Tutorials.Instructions != "部署说明保持不变" || len(got.Tutorials.VideoLinks) != 1 || got.Tutorials.VideoLinks[0].URL != "https://example.com/video" {
					t.Fatal("public materials changed")
				}
				if test.origin == nil {
					encoded, err := json.Marshal(got.Website)
					if err != nil {
						t.Fatal(err)
					}
					var fields map[string]any
					if err := json.Unmarshal(encoded, &fields); err != nil {
						t.Fatal(err)
					}
					for _, field := range []string{"canonicalOrigin", "domainAscii"} {
						value, exists := fields[field]
						if !exists || value != nil {
							t.Fatalf("%s must remain explicit null", field)
						}
					}
				}
			}
		})
	}
}
