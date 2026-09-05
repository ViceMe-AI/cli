package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestReplicaSalesRequiresConfirmationAndBindsCurrentState(t *testing.T) {
	const replicaID = "11111111-1111-4111-8111-111111111111"
	const requestID = "22222222-2222-4222-8222-222222222222"
	state := replicaSalesFixture()
	posts := 0
	var firstRequest map[string]any
	// A deliberately lost HTTP response does not join the handler goroutine.
	var stateMu sync.Mutex
	postCount := func() int {
		stateMu.Lock()
		defer stateMu.Unlock()
		return posts
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stateMu.Lock()
		defer stateMu.Unlock()
		switch r.URL.Path {
		case "/v1/cli/auth/status":
			writeJSONResponse(w, map[string]any{"authenticated": true, "user": map[string]any{"id": requestID, "displayName": "Creator", "avatarUrl": nil}, "scopes": []string{"website-replica:write"}, "expiresAt": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)})
		case "/v1/website-replicas/" + replicaID + "/sales":
			writeJSONResponse(w, state)
		case "/v1/website-replicas/" + replicaID + "/sales/price":
			posts++
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["expectedRevision"] != float64(3) || body["priceCents"] != float64(1200) || body["clientRequestId"] != requestID {
				t.Fatalf("unexpected CAS: %#v", body)
			}
			if firstRequest == nil {
				firstRequest = body
			} else if !reflect.DeepEqual(firstRequest, body) {
				t.Errorf("retry changed request: %#v", body)
			}
			if posts == 1 {
				hijacker := w.(http.Hijacker)
				conn, _, err := hijacker.Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				conn.Close()
				return
			}
			product := state["product"].(map[string]any)
			product["priceCents"] = 1200
			product["revision"] = 4
			writeJSONResponse(w, map[string]any{"mutationId": requestID, "kind": "PRICE_CHANGED", "changed": true, "state": state, "createdAt": "2026-09-05T00:00:00Z"})
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	t.Setenv(processAccessTokenEnvironment, "vme_cli_1234567890123456789012345678901234567890123")
	root := t.TempDir()
	run := func(args ...string) (int, []byte) {
		var out bytes.Buffer
		code := Execute(args, Dependencies{Out: &out, ErrOut: &bytes.Buffer{}, HTTPClient: server.Client(), Store: securestore.NewMemory(), Environment: skillcontent.Environment{Home: root, ConfigDir: root + "/config"}, Region: config.RegionCN, APIBaseURL: server.URL, NewID: func() string { return requestID }})
		return code, out.Bytes()
	}
	args := []string{"replica", "price", "--replica", replicaID, "--price-cents", "1200"}
	code, out := run(args...)
	if code != 10 || postCount() != 0 {
		t.Fatalf("preview must require confirmation without writing: %d %s", code, out)
	}
	var result struct {
		Error struct {
			Details struct {
				ConfirmDigest  string `json:"confirmDigest"`
				ConfirmCommand string `json:"confirmCommand"`
			}
		}
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatal(err)
	}
	digest := result.Error.Details.ConfirmDigest
	if len(digest) != 64 {
		t.Fatalf("missing review digest: %s", out)
	}
	confirmed := strings.Fields(result.Error.Details.ConfirmCommand)[1:]
	code, out = run(confirmed...)
	if code == 0 || postCount() != 1 {
		t.Fatalf("lost response should not report success: %d %s", code, out)
	}
	// The server committed before the response was lost. Retry must preserve the
	// old revision and request identity, allowing server-side replay.
	stateMu.Lock()
	state["product"].(map[string]any)["revision"] = 4
	stateMu.Unlock()
	code, out = run(confirmed...)
	if code != 0 || postCount() != 2 {
		t.Fatalf("idempotent retry failed: %d %s", code, out)
	}
	tampered := append([]string{}, confirmed...)
	for i, arg := range tampered {
		if arg == "--price-cents" {
			tampered[i+1] = "1300"
		}
	}
	code, out = run(tampered...)
	if code != 2 || postCount() != 2 {
		t.Fatalf("altered confirmation wrote: %d %s", code, out)
	}

}

func replicaSalesFixture() map[string]any {
	return map[string]any{
		"replicaId": "11111111-1111-4111-8111-111111111111", "workId": "33333333-3333-4333-8333-333333333333", "saleStatus": "ACTIVE", "operationsEnabled": true,
		"replicaVersion": map[string]any{"id": "44444444-4444-4444-8444-444444444444", "version": 2, "title": "Website source", "digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		"product":        map[string]any{"id": "55555555-5555-4555-8555-555555555555", "revision": 3, "status": "ACTIVE", "salesSpecVersionId": "66666666-6666-4666-8666-666666666666", "salesSpecVersion": 2, "skuId": "77777777-7777-4777-8777-777777777777", "currency": "CNY", "priceCents": 800},
	}
}

func TestReplicaSalesLifecycleAndReadOnly(t *testing.T) {
	for _, operation := range []string{"sales", "price", "delist", "relist"} {
		for _, enabled := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/enabled=%t", operation, enabled), func(t *testing.T) {
				state := replicaSalesFixture()
				state["operationsEnabled"] = enabled
				product := state["product"].(map[string]any)
				if operation == "relist" {
					state["saleStatus"] = "DELISTED"
					product["status"] = "SUSPENDED"
				}
				posts := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/v1/cli/auth/status" {
						writeJSONResponse(w, map[string]any{"authenticated": true, "user": map[string]any{"id": state["workId"], "displayName": "Creator", "avatarUrl": nil}, "scopes": []string{"website-replica:write"}, "expiresAt": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)})
						return
					}
					if r.Method == http.MethodGet {
						writeJSONResponse(w, state)
						return
					}
					posts++
					if !enabled || !strings.HasSuffix(r.URL.Path, "/sales/"+operation) {
						t.Error("unexpected mutation")
					}
					var input map[string]any
					if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
						t.Error(err)
					}
					if input["expectedProductId"] != product["id"] || input["expectedSalesSpecVersionId"] != product["salesSpecVersionId"] || input["expectedReplicaVersionId"] != state["replicaVersion"].(map[string]any)["id"] {
						t.Errorf("missing CAS targets: %#v", input)
					}
					kind := "PRICE_CHANGED"
					if operation == "price" {
						if input["priceCents"] != float64(0) {
							t.Error("free price was omitted")
						}
						product["priceCents"] = 0
					} else {
						if _, ok := input["priceCents"]; ok {
							t.Error("lifecycle mutation includes price")
						}
						if operation == "delist" {
							kind = "DELISTED"
							state["saleStatus"] = "DELISTED"
							product["status"] = "SUSPENDED"
						} else {
							kind = "RELISTED"
							state["saleStatus"] = "ACTIVE"
							product["status"] = "ACTIVE"
						}
					}
					product["revision"] = 4
					writeJSONResponse(w, map[string]any{"mutationId": state["workId"], "kind": kind, "changed": true, "state": state, "createdAt": "2026-09-05T00:00:00Z"})
				}))
				defer server.Close()
				t.Setenv(processAccessTokenEnvironment, "vme_cli_1234567890123456789012345678901234567890123")
				root := t.TempDir()
				run := func(args []string) (int, []byte) {
					var out bytes.Buffer
					code := Execute(args, Dependencies{Out: &out, ErrOut: &bytes.Buffer{}, HTTPClient: server.Client(), Store: securestore.NewMemory(), Environment: skillcontent.Environment{Home: root, ConfigDir: root + "/config"}, Region: config.RegionCN, APIBaseURL: server.URL, NewID: func() string { return state["workId"].(string) }})
					return code, out.Bytes()
				}
				args := []string{"replica", operation, "--replica", state["replicaId"].(string)}
				if operation == "price" {
					args = append(args, "--price-cents", "0")
				}
				code, out := run(args)
				if operation == "sales" {
					if code != 0 || posts != 0 {
						t.Fatalf("history unavailable: %d %s", code, out)
					}
					return
				}
				if !enabled {
					if code != 6 || posts != 0 || !bytes.Contains(out, []byte("REPLICA_SALES_READ_ONLY")) {
						t.Fatalf("read-only failed: %d %s", code, out)
					}
					return
				}
				var envelope struct {
					Error struct {
						Details struct{ ConfirmCommand string }
					}
				}
				if err := json.Unmarshal(out, &envelope); err != nil {
					t.Fatal(err)
				}
				if code != 10 || posts != 0 {
					t.Fatalf("preview failed: %d %s", code, out)
				}
				command := strings.Fields(envelope.Error.Details.ConfirmCommand)
				if len(command) < 2 {
					t.Fatalf("missing confirm command: %s", out)
				}
				code, out = run(command[1:])
				if code != 0 || posts != 1 {
					t.Fatalf("mutation failed: %d %s", code, out)
				}
			})
		}
	}
}
