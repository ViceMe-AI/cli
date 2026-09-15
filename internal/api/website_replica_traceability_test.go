package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
)

func TestWebsiteReplicaTraceabilityResponses(t *testing.T) {
	// Synthetic identities serialized by Shop's actual response DTO schemas:
	// https://github.com/Leizhenpeng/ViceMe-Shop/blob/79820ea8910b6965a5dc9d8627598a82b4c4f28e/packages/contracts/src/website-replicas.ts
	// Keep this fixture independent of CLI structs: CLI-only fixtures missed the
	// field added by Shop and allowed an incompatible release to pass tests.
	data, err := os.ReadFile("testdata/shop-replica-traceability.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		value   any
		omitted bool
		invalid bool
	}{
		{name: "legacy-omitted", omitted: true},
		{name: "disabled", value: false},
		{name: "enabled", value: true},
		{name: "null", invalid: true},
		{name: "string", value: "true", invalid: true},
		{name: "number", value: 1, invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var fixtures map[string]map[string]any
			if err := json.Unmarshal(data, &fixtures); err != nil {
				t.Fatal(err)
			}
			review := fixtures["confirmation"]["nextAction"].(map[string]any)["confirmation"].(map[string]any)["review"].(map[string]any)
			for _, object := range []map[string]any{review, fixtures["publication"]} {
				if test.omitted {
					delete(object, "traceabilityEnabled")
				} else {
					object["traceabilityEnabled"] = test.value
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/v1/website-replica-publications":
					var submitted map[string]any
					if err := json.NewDecoder(r.Body).Decode(&submitted); err != nil {
						t.Error(err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					if confirmation, ok := submitted["confirmation"].(map[string]any); ok {
						if !reflect.DeepEqual(confirmation["review"], review) {
							t.Error("confirmation resubmission changed the reviewed fields")
						}
					}
					writeReplicaPublicationJSON(w, fixtures["confirmation"])
				case r.Method == http.MethodGet && r.URL.Path == "/v1/website-replica-publications/"+testReplicaPublicationID:
					writeReplicaPublicationJSON(w, fixtures["publication"])
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			client := NewClient(server.URL, server.Client(), staticToken("vme_cli_test"), "viceme/test")
			request := canonicalReplicaPublicationRequest()
			created, createErr := client.CreateWebsiteReplicaPublication(context.Background(), request)
			publication, getErr := client.GetWebsiteReplicaPublication(context.Background(), testReplicaPublicationID)
			if test.invalid {
				assertInvalidWebsiteReplicaResponse(t, createErr)
				assertInvalidWebsiteReplicaResponse(t, getErr)
				return
			}
			if createErr != nil || getErr != nil {
				t.Fatalf("Shop response rejected: create=%v get=%v", createErr, getErr)
			}
			challenge := created.NextAction.Confirmation
			if challenge == nil {
				t.Fatal("missing publication confirmation")
			}
			for _, flag := range []*bool{challenge.Review.TraceabilityEnabled, publication.TraceabilityEnabled} {
				if test.omitted {
					if flag != nil {
						t.Fatal("legacy omission must remain absent")
					}
				} else if flag == nil || *flag != test.value.(bool) {
					t.Fatal("traceability value was lost")
				}
			}
			request.Confirmation = &WebsiteReplicaPublicationConfirmation{
				Version: challenge.Version, Review: challenge.Review,
				IssuedAt: challenge.IssuedAt, ExpiresAt: challenge.ExpiresAt, ConfirmedAt: challenge.IssuedAt,
			}
			if _, err := client.CreateWebsiteReplicaPublication(context.Background(), request); err != nil {
				t.Fatalf("confirmation resubmission failed: %v", err)
			}
		})
	}
}
