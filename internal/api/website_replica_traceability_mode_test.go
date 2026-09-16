package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestPublishedWebsiteReplicaTraceabilityModes(t *testing.T) {
	// Sanitized live PUBLISHED response, independent of CLI structs. Contract:
	// https://github.com/Leizhenpeng/ViceMe-Shop/blob/e2dbafdc6cceab3ff950c6ea402df3413ca720b2/packages/contracts/src/website-replicas.ts
	data, err := os.ReadFile("testdata/shop-replica-published-traceability-mode.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name                    string
		mode                    any
		omitted, invalid, extra bool
	}{
		{name: "legacy-omitted", omitted: true},
		{name: "off", mode: "OFF"}, {name: "basic", mode: "BASIC"}, {name: "strong", mode: "STRONG"},
		{name: "null", invalid: true}, {name: "empty", mode: "", invalid: true},
		{name: "unknown", mode: "FUTURE", invalid: true}, {name: "lowercase", mode: "basic", invalid: true},
		{name: "number", mode: 1, invalid: true}, {name: "boolean", mode: true, invalid: true},
		{name: "object", mode: map[string]any{}, invalid: true},
		{name: "unknown-sibling", mode: "BASIC", extra: true, invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var response map[string]any
			if err := json.Unmarshal(data, &response); err != nil {
				t.Fatal(err)
			}
			result := response["result"].(map[string]any)
			if test.omitted {
				delete(result, "traceabilityMode")
			} else {
				result["traceabilityMode"] = test.mode
			}
			if test.extra {
				result["unexpectedField"] = "must still be rejected"
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/website-replica-publications/"+testReplicaPublicationID {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				writeReplicaPublicationJSON(w, response)
			}))
			defer server.Close()
			client := NewClient(server.URL, server.Client(), staticToken("vme_cli_test"), "viceme/test")
			publication, err := client.GetWebsiteReplicaPublication(context.Background(), testReplicaPublicationID)
			if test.invalid {
				assertInvalidWebsiteReplicaResponse(t, err)
				return
			}
			if err != nil {
				t.Fatalf("valid published response rejected: %v", err)
			}
			if publication.Status != "PUBLISHED" || publication.Hosting.Status != "ACTIVE" || publication.Result == nil {
				t.Fatal("lost published hosting state")
			}
			mode := publication.Result.TraceabilityMode
			if test.omitted {
				if mode != nil {
					t.Fatal("legacy omission must remain absent")
				}
			} else if mode == nil || *mode != test.mode.(string) {
				t.Fatal("traceability mode was lost")
			}
		})
	}
}
