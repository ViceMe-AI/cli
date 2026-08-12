package command

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestDeviceLoginSplitFlowPersistsScopedCredentialWithoutPrintingToken(t *testing.T) {
	t.Parallel()
	const accessToken = "vme_cli_1234567890123456789012345678901234567890123"
	expiresAt := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	grantedScopes := []string{
		"profile:read", "skill-publication:read", "skill-publication:write",
		"payment-capability:read", "payment-capability:write",
	}
	var tokenPolls atomic.Int32
	var revoked atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/cli/device-authorizations":
			var authorizationRequest struct {
				Scopes []string `json:"scopes"`
			}
			if err := json.NewDecoder(request.Body).Decode(&authorizationRequest); err != nil || len(authorizationRequest.Scopes) != len(grantedScopes) {
				t.Fatalf("device authorization omitted required scopes: request=%#v err=%v", authorizationRequest, err)
			}
			for index, scope := range grantedScopes {
				if authorizationRequest.Scopes[index] != scope {
					t.Fatalf("unexpected device authorization scopes: %#v", authorizationRequest.Scopes)
				}
			}
			writeJSONResponse(writer, map[string]any{
				"deviceCode": "device-code", "userCode": "ABCD-EFGH",
				"verificationUri":         "https://viceme.cn/zh-CN/cli/authorize",
				"verificationUriComplete": "https://viceme.cn/zh-CN/cli/authorize?user_code=ABCD-EFGH",
				"expiresIn":               600, "interval": 1,
			})
		case "/v1/cli/device-authorizations/token":
			if tokenPolls.Add(1) == 1 {
				writeJSONResponse(writer, map[string]any{"status": "authorization_pending", "interval": 1})
				return
			}
			writeJSONResponse(writer, map[string]any{
				"status": "authorized", "accessToken": accessToken, "tokenType": "Bearer",
				"expiresAt": expiresAt,
				"scopes":    grantedScopes,
			})
		case "/v1/cli/auth/status":
			if request.Header.Get("Authorization") != "Bearer "+accessToken {
				writer.WriteHeader(http.StatusUnauthorized)
				return
			}
			writeJSONResponse(writer, map[string]any{
				"authenticated": true,
				"user":          map[string]any{"id": "55555555-5555-4555-8555-555555555555", "displayName": "Creator", "avatarUrl": nil},
				"scopes":        grantedScopes,
				"expiresAt":     expiresAt,
			})
		case "/v1/cli/auth/logout":
			revoked.Store(true)
			writer.WriteHeader(http.StatusNoContent)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	dependencies := Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: securestore.NewMemory(), APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Sleep:       func(context.Context, time.Duration) error { return nil },
	}
	run := func(arguments ...string) (int, map[string]any) {
		t.Helper()
		stdout.Reset()
		stderr.Reset()
		exit := Execute(arguments, dependencies)
		var envelope map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("invalid CLI envelope: exit=%d stdout=%q stderr=%q err=%v", exit, stdout.String(), stderr.String(), err)
		}
		if strings.Contains(stdout.String(), accessToken) || strings.Contains(stderr.String(), accessToken) {
			t.Fatalf("access token leaked in CLI output: stdout=%q stderr=%q", stdout.String(), stderr.String())
		}
		return exit, envelope
	}
	if exit, result := run("auth", "login", "--no-wait"); exit != 0 || result["ok"] != true {
		t.Fatalf("device start failed: exit=%d result=%#v", exit, result)
	}
	if exit, result := run("auth", "login", "--device-code", "device-code"); exit != 0 || result["ok"] != true {
		t.Fatalf("device exchange failed: exit=%d result=%#v", exit, result)
	}
	if exit, result := run("auth", "status"); exit != 0 || result["ok"] != true {
		t.Fatalf("remote status failed: exit=%d result=%#v", exit, result)
	}
	if exit, result := run("auth", "logout"); exit != 0 || result["ok"] != true || !revoked.Load() {
		t.Fatalf("logout failed: exit=%d result=%#v revoked=%v", exit, result, revoked.Load())
	}
	configData := []byte(readFileString(t, filepath.Join(root, "config", "config.json")))
	if !bytes.Contains(configData, []byte("55555555-5555-4555-8555-555555555555")) {
		t.Fatalf("profile user was not persisted: config=%s", configData)
	}
}

func readFileString(t *testing.T, filename string) string {
	t.Helper()
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
