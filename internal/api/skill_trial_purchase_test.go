package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTrialPurchaseValidatesOrderAndNeverReadsAccountCredential(t *testing.T) {
	const product = "11111111-1111-4111-8111-111111111111"
	valid := map[string]any{"productId": product, "orderNo": "TEST_ORDER_01", "status": "PENDING", "amountCents": 100, "currency": "CNY", "expiresAt": "2026-09-06T00:00:00Z"}
	for _, field := range []string{"", "productId", "orderNo", "amountCents", "currency", "expiresAt", "status"} {
		t.Run(field, func(t *testing.T) {
			response := make(map[string]any)
			for key, value := range valid {
				response[key] = value
			}
			if field != "" {
				delete(response, field)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/v1/skills/"+product+"/trial-purchase/status" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "" {
					t.Error("unexpected authenticated or unscoped request")
				}
				var body map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["secret"] != strings.Repeat("a", 64) || body["orderNo"] != "TEST_ORDER_01" || body["clientRequestId"] != "" {
					t.Error("status must only carry its credential and original order")
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()
			client := NewClient(server.URL, server.Client(), staticToken("must-not-be-read"), "test")
			result, err := client.TrialPurchase(context.Background(), product, "install", strings.Repeat("a", 64), "request", "zh-CN", "TEST_ORDER_01")
			if (err == nil) != (field == "") {
				t.Fatalf("field %s: result=%#v err=%v", field, result, err)
			}
		})
	}
}

func TestTrialFormalReceiptRejectsPublicOrMismatchedRelease(t *testing.T) {
	const product = "11111111-1111-4111-8111-111111111111"
	for _, mismatch := range []string{"", "owned", "kind", "product", "release", "digest"} {
		t.Run(mismatch, func(t *testing.T) {
			access := map[string]any{"owned": true, "installKind": "OWNED_PAID", "productId": product, "release": map[string]any{"id": "release-1", "artifactDigest": strings.Repeat("a", 64)}}
			download := map[string]any{"releaseId": "release-1", "artifactDigest": strings.Repeat("a", 64)}
			switch mismatch {
			case "owned":
				access["owned"] = false
			case "kind":
				access["installKind"] = "FREE"
			case "product":
				access["productId"] = "other"
			case "release":
				download["releaseId"] = "other"
			case "digest":
				download["artifactDigest"] = strings.Repeat("b", 64)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "" || !strings.HasSuffix(r.URL.Path, "/trial-purchase/download") {
					t.Error("unexpected download authorization route")
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"access": access, "download": download})
			}))
			defer server.Close()
			client := NewClient(server.URL, server.Client(), staticToken("must-not-be-read"), "test")
			_, err := client.TrialOwnedSkillDownload(context.Background(), product, "install", "secret")
			if (err == nil) != (mismatch == "") {
				t.Fatalf("mismatch %s: %v", mismatch, err)
			}
		})
	}
}
