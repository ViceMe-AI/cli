package command

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestReplicaRollbackReadsAuthoritativePairsAndSubmitsCAS(t *testing.T) {
	const (
		accessToken   = "vme_cli_1234567890123456789012345678901234567890123"
		publicationID = "11111111-1111-4111-8111-111111111111"
		requestID     = "22222222-2222-4222-8222-222222222222"
		activePairID  = "33333333-3333-4333-8333-333333333333"
		targetPairID  = "44444444-4444-4444-8444-444444444444"
	)
	activePair := replicaVersionPair(activePairID, "55555555-5555-4555-8555-555555555555", 2, "66666666-6666-4666-8666-666666666666", map[string]any{
		"id": "77777777-7777-4777-8777-777777777777", "version": 2,
	})
	targetPair := replicaVersionPair(targetPairID, "88888888-8888-4888-8888-888888888888", 1, "99999999-9999-4999-8999-999999999999", nil)
	postCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+accessToken {
			t.Fatalf("control API did not receive authorization: %q", request.Header.Get("Authorization"))
		}
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/cli/auth/status":
			writeJSONResponse(writer, map[string]any{
				"authenticated": true,
				"user":          map[string]any{"id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "displayName": "Creator", "avatarUrl": nil},
				"scopes":        []string{"website-replica:read", "website-replica:write"},
				"expiresAt":     time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
			})
		case request.Method == http.MethodGet && request.URL.Path == "/v1/website-replica-publications/"+publicationID:
			writeJSONResponse(writer, map[string]any{
				"id":       publicationID,
				"status":   "PUBLISHED",
				"rollback": map[string]any{"activePair": activePair, "availablePairs": []any{targetPair}},
			})
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replica-publications/"+publicationID+"/rollbacks":
			postCount++
			var input map[string]any
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			if input["clientRequestId"] != requestID || input["targetPairId"] != targetPairID || input["expectedActivePairId"] != activePairID {
				t.Fatalf("unexpected rollback request: %#v", input)
			}
			writeJSONResponse(writer, map[string]any{
				"id":              "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
				"publicationId":   publicationID,
				"clientRequestId": requestID,
				"previousPair":    activePair,
				"activePair":      targetPair,
				"product": map[string]any{
					"id":    "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
					"skuId": "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
					"title": "Replica source v1", "currency": "CNY", "priceCents": 680,
				},
				"priceUnchanged": true,
				"rolledBackAt":   "2026-09-06T12:00:00Z",
			})
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	t.Setenv(processAccessTokenEnvironment, accessToken)
	root := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := Execute([]string{
		"replica", "rollback", "--publication", publicationID, "--pair", targetPairID,
	}, Dependencies{
		Out: &stdout, ErrOut: &stderr, HTTPClient: server.Client(), Store: securestore.NewMemory(),
		Environment: skillcontent.Environment{Home: root, ConfigDir: root + "/config"},
		Region:      config.RegionCN, APIBaseURL: server.URL, NewID: func() string { return requestID },
	})
	if exit != 0 {
		t.Fatalf("replica rollback failed: exit=%d stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
	}
	if postCount != 1 {
		t.Fatalf("rollback POST count = %d, want 1", postCount)
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			SourceVersion   int    `json:"sourceVersion"`
			SourceVersionID string `json:"sourceVersionId"`
			PageMode        string `json:"pageMode"`
			PageRelease     any    `json:"pageRelease"`
			PriceCents      int    `json:"priceCents"`
			PriceUnchanged  bool   `json:"priceUnchanged"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid command output: %v: %s", err, stdout.String())
	}
	if !envelope.OK || envelope.Data.SourceVersion != 1 || envelope.Data.SourceVersionID != "88888888-8888-4888-8888-888888888888" || envelope.Data.PageMode != "NATIVE_WORK" || envelope.Data.PageRelease != nil || envelope.Data.PriceCents != 680 || !envelope.Data.PriceUnchanged {
		t.Fatalf("unexpected rollback output: %#v", envelope)
	}
}

func TestReplicaRollbackRejectsPairOutsideAuthoritativeStatus(t *testing.T) {
	const (
		accessToken   = "vme_cli_1234567890123456789012345678901234567890123"
		publicationID = "11111111-1111-4111-8111-111111111111"
		activePairID  = "33333333-3333-4333-8333-333333333333"
		unknownPairID = "44444444-4444-4444-8444-444444444444"
	)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/cli/auth/status":
			writeJSONResponse(writer, map[string]any{
				"authenticated": true,
				"user":          map[string]any{"id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "displayName": "Creator", "avatarUrl": nil},
				"scopes":        []string{"website-replica:read", "website-replica:write"},
				"expiresAt":     time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
			})
		case request.Method == http.MethodGet && request.URL.Path == "/v1/website-replica-publications/"+publicationID:
			writeJSONResponse(writer, map[string]any{
				"id": publicationID, "status": "PUBLISHED_DEGRADED",
				"rollback": map[string]any{
					"activePair":     replicaVersionPair(activePairID, "55555555-5555-4555-8555-555555555555", 2, "66666666-6666-4666-8666-666666666666", nil),
					"availablePairs": []any{},
				},
			})
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	t.Setenv(processAccessTokenEnvironment, accessToken)
	root := t.TempDir()
	var stdout bytes.Buffer
	exit := Execute([]string{
		"replica", "rollback", "--publication", publicationID, "--pair", unknownPairID,
	}, Dependencies{
		Out: &stdout, ErrOut: &bytes.Buffer{}, HTTPClient: server.Client(), Store: securestore.NewMemory(),
		Environment: skillcontent.Environment{Home: root, ConfigDir: root + "/config"},
		Region:      config.RegionCN, APIBaseURL: server.URL, NewID: func() string { return "22222222-2222-4222-8222-222222222222" },
	})
	if exit == 0 || !bytes.Contains(stdout.Bytes(), []byte(`"code": "REPLICA_ROLLBACK_TARGET_UNAVAILABLE"`)) {
		t.Fatalf("unexpected unavailable-pair result: exit=%d stdout=%q", exit, stdout.String())
	}
}

func replicaVersionPair(pairID, versionID string, version int, workRevisionID string, pageRelease any) map[string]any {
	return map[string]any{
		"id":             pairID,
		"replicaVersion": map[string]any{"id": versionID, "version": version},
		"workRevisionId": workRevisionID,
		"pageRelease":    pageRelease,
		"createdAt":      "2026-09-05T12:00:00Z",
	}
}
