package command

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

const accountTestToken = "vme_cli_1234567890123456789012345678901234567890123"

func TestAccountCommandsUpdatePlatformProfileAndAvatarThroughCLI(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, accountTestToken)
	var updateCalls, avatarCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+accountTestToken {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/v1/cli/auth/status":
			writeAccountAuth(writer, true)
		case "/v1/cli/account/profile":
			updateCalls++
			if request.Method != http.MethodPatch {
				t.Fatalf("profile update used %s", request.Method)
			}
			var input map[string]string
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			if input["displayName"] != "jingxu" {
				t.Fatalf("unexpected profile update: %#v", input)
			}
			writeJSONResponse(writer, map[string]any{"user": map[string]any{"id": "33333333-3333-4333-8333-333333333333", "displayName": "jingxu", "avatarUrl": nil}})
		case "/v1/cli/account/avatar":
			avatarCalls++
			if request.Method != http.MethodPost {
				t.Fatalf("avatar upload used %s", request.Method)
			}
			if err := request.ParseMultipartForm(2 << 20); err != nil {
				t.Fatalf("parse avatar upload: %v", err)
			}
			file, header, err := request.FormFile("file")
			if err != nil {
				t.Fatalf("avatar part missing: %v", err)
			}
			defer file.Close()
			body := make([]byte, 3)
			if _, err := file.Read(body); err != nil || string(body) != "png" || header.Filename != "avatar.png" {
				t.Fatalf("unexpected avatar multipart payload: body=%q filename=%q err=%v", body, header.Filename, err)
			}
			writeJSONResponse(writer, map[string]any{"avatarUrl": "https://api.example.test/v1/assets/user-avatars/sha256/ab/avatar.png"})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	avatarPath := filepath.Join(root, "avatar.png")
	if err := os.WriteFile(avatarPath, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	if exit, envelope := executeAccountCommand(t, server, "account", "profile", "update", "--display-name", "  jingxu  "); exit != 0 || envelope["ok"] != true {
		t.Fatalf("profile update failed: exit=%d response=%#v", exit, envelope)
	}
	if exit, envelope := executeAccountCommand(t, server, "account", "avatar", "upload", "--path", avatarPath); exit != 0 || envelope["ok"] != true {
		t.Fatalf("avatar upload failed: exit=%d response=%#v", exit, envelope)
	}
	if updateCalls != 1 || avatarCalls != 1 {
		t.Fatalf("unexpected platform writes: profile=%d avatar=%d", updateCalls, avatarCalls)
	}
}

func TestAccountCommandsRequireProfileWriteScope(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, accountTestToken)
	var writes int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/cli/auth/status":
			writeAccountAuth(writer, false)
		default:
			writes++
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	exit, envelope := executeAccountCommand(t, server, "account", "profile", "update", "--display-name", "jingxu")
	errorBody, _ := envelope["error"].(map[string]any)
	if exit != output.ExitAuthentication || errorBody["code"] != "PROFILE_WRITE_SCOPE_REQUIRED" || writes != 0 {
		t.Fatalf("missing scope did not block the write: exit=%d response=%#v writes=%d", exit, envelope, writes)
	}
}

func TestAccountCommandsRejectUnauthenticatedStatusBeforeWrite(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, accountTestToken)
	var writes int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/cli/auth/status":
			writeJSONResponse(writer, map[string]any{
				"authenticated": false,
				"scopes":        []string{"profile:write"},
			})
		default:
			writes++
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	exit, envelope := executeAccountCommand(t, server, "account", "profile", "update", "--display-name", "jingxu")
	errorBody, _ := envelope["error"].(map[string]any)
	if exit != output.ExitAuthentication || errorBody["code"] != "NOT_LOGGED_IN" || writes != 0 {
		t.Fatalf("unauthenticated status did not block the write: exit=%d response=%#v writes=%d", exit, envelope, writes)
	}
}

func TestAccountAvatarRejectsOversizedFilesBeforeNetwork(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "oversized.png")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), accountAvatarMaxBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()

	exit, envelope := executeAccountCommand(t, server, "account", "avatar", "upload", "--path", path)
	errorBody, _ := envelope["error"].(map[string]any)
	if exit != output.ExitValidation || errorBody["code"] != "ACCOUNT_AVATAR_SIZE_INVALID" || calls != 0 {
		t.Fatalf("oversized avatar reached the network: exit=%d response=%#v calls=%d", exit, envelope, calls)
	}
}

func executeAccountCommand(t *testing.T, server *httptest.Server, arguments ...string) (int, map[string]any) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	root := t.TempDir()
	exit := Execute(arguments, Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: securestore.NewMemory(), HTTPClient: server.Client(),
		APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
	})
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid CLI envelope: exit=%d stdout=%q stderr=%q err=%v", exit, stdout.String(), stderr.String(), err)
	}
	return exit, envelope
}

func writeAccountAuth(writer http.ResponseWriter, writable bool) {
	scopes := []string{"profile:read"}
	if writable {
		scopes = append(scopes, "profile:write")
	}
	writeJSONResponse(writer, map[string]any{
		"authenticated": true,
		"user":          map[string]any{"id": "33333333-3333-4333-8333-333333333333", "displayName": "Creator", "avatarUrl": nil},
		"scopes":        scopes,
		"expiresAt":     "2027-08-27T00:00:00Z",
	})
}
