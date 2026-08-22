package command

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestMerchantCommandsRequireScopedLoginBeforeAuthoring(t *testing.T) {
	const accessToken = "vme_cli_1234567890123456789012345678901234567890123"
	const merchantID = "11111111-1111-4111-8111-111111111111"
	var writeEnabled atomic.Bool
	var createCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/v1/cli/auth/status" {
			scopes := []string{"profile:read", "merchant-commerce:read"}
			if writeEnabled.Load() {
				scopes = append(scopes, "merchant-commerce:write")
			}
			writeJSONResponse(writer, map[string]any{
				"authenticated": true,
				"user":          map[string]any{"id": "22222222-2222-4222-8222-222222222222", "displayName": "Creator", "avatarUrl": nil},
				"scopes":        scopes, "expiresAt": "2027-08-21T00:00:00Z",
			})
			return
		}
		if request.Header.Get("Authorization") != "Bearer "+accessToken {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/v1/cli/merchant/accounts":
			writeJSONResponse(writer, map[string]any{"items": []any{map[string]any{
				"id": merchantID, "creatorChannelAccountId": "33333333-3333-4333-8333-333333333333",
				"displayName": "Creator", "status": "ACTIVE", "statusVersion": 1,
			}}})
		case "/v1/cli/merchant/works":
			createCalls.Add(1)
			writeJSONResponse(writer, map[string]any{
				"id": "44444444-4444-4444-8444-444444444444", "kind": "SKILL", "origin": "USER_AUTHORED",
				"slug": "photo-printing", "title": "照片打印", "status": "DRAFT", "revision": 1,
				"owner": map[string]any{}, "skill": map[string]any{}, "website": nil,
				"createdAt": "2026-08-21T00:00:00Z", "updatedAt": "2026-08-21T00:00:00Z",
			})
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	t.Setenv(processAccessTokenEnvironment, accessToken)

	root := t.TempDir()
	input := filepath.Join(root, "work.json")
	if err := os.WriteFile(input, []byte(`{"kind":"SKILL","merchantAccountId":"`+merchantID+`","clientRequestId":"55555555-5555-4555-8555-555555555555","market":"CN","slug":"photo-printing","title":"照片打印"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	dependencies := Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: securestore.NewMemory(),
		HTTPClient: server.Client(), APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
	}
	run := func(arguments ...string) (int, map[string]any) {
		t.Helper()
		stdout.Reset()
		stderr.Reset()
		exit := Execute(arguments, dependencies)
		if strings.Contains(stdout.String(), accessToken) || strings.Contains(stderr.String(), accessToken) {
			t.Fatalf("CLI access token leaked: stdout=%q stderr=%q", stdout.String(), stderr.String())
		}
		var envelope map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("invalid envelope: exit=%d stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
		}
		return exit, envelope
	}
	if exit, envelope := run("merchant", "accounts"); exit != 0 || envelope["ok"] != true {
		t.Fatalf("read-scoped account lookup failed: exit=%d result=%#v", exit, envelope)
	}
	if exit, envelope := run("merchant", "work", "create", "--input", input); exit == 0 || envelope["ok"] != false || createCalls.Load() != 0 {
		t.Fatalf("write ran without write scope: exit=%d result=%#v calls=%d", exit, envelope, createCalls.Load())
	}
	writeEnabled.Store(true)
	if exit, envelope := run("merchant", "work", "create", "--input", input); exit != 0 || envelope["ok"] != true || createCalls.Load() != 1 {
		t.Fatalf("authorized Work creation failed: exit=%d result=%#v calls=%d", exit, envelope, createCalls.Load())
	}
}
