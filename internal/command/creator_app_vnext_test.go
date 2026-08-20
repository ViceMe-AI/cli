package command

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func executeCreatorAppCommand(t *testing.T, baseURL string, args ...string) (string, error) {
	t.Helper()
	root := t.TempDir()
	store := securestore.NewMemory()
	scope, err := credentialScopeForAPIBase(baseURL, config.RegionCN)
	if err != nil {
		t.Fatal(err)
	}
	manager := credentialauth.Manager{Store: store, Region: "cn", ProfileID: "default", ProfileName: "default", Scope: scope}
	if err := manager.Save(credentialauth.Credential{AccessToken: "vme_cli_test", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	dependencies := Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: store, APIBaseURL: baseURL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
	}
	if exit := Execute(args, dependencies); exit != 0 {
		return "", &commandExitError{exit: exit, stderr: stderr.String()}
	}
	return stdout.String(), nil
}

type commandExitError struct {
	exit   int
	stderr string
}

func (e *commandExitError) Error() string { return e.stderr }

func TestCreatorAppCreatePostsToCliEndpoint(t *testing.T) {
	t.Parallel()
	var gotPath, gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"app-1","kind":"EXTERNAL","name":"Demo","domains":[],"createdAt":"2026-08-18T00:00:00Z"}`))
	}))
	defer server.Close()

	output, err := executeCreatorAppCommand(t, server.URL, "creator-app", "create", "--name", "Demo")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/v1/cli/creator-apps" {
		t.Fatalf("unexpected request %s %s", gotMethod, gotPath)
	}
	var decoded struct {
		Data struct {
			App struct {
				ID string `json:"id"`
			} `json:"app"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if decoded.Data.App.ID != "app-1" {
		t.Fatalf("unexpected app id: %q", decoded.Data.App.ID)
	}
}

func TestCreatorAppDomainAddSurfacesVerificationToken(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/v1/cli/creator-apps/app-1/domains" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"app-1","kind":"EXTERNAL","name":"Demo","domains":[{"domain":"shop.example.com","verified":false,"verificationToken":"tok-123"}],"createdAt":"2026-08-18T00:00:00Z"}`))
	}))
	defer server.Close()

	output, err := executeCreatorAppCommand(t, server.URL, "creator-app", "domain", "add", "app-1", "shop.example.com")
	if err != nil {
		t.Fatalf("domain add failed: %v", err)
	}
	if !strings.Contains(output, "tok-123") {
		t.Fatalf("verification token missing from output: %s", output)
	}
}

func TestCreatorAppDomainVerify(t *testing.T) {
	t.Parallel()
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"app-1","kind":"EXTERNAL","name":"Demo","domains":[{"domain":"shop.example.com","verified":true,"verificationToken":null}],"createdAt":"2026-08-18T00:00:00Z"}`))
	}))
	defer server.Close()

	output, err := executeCreatorAppCommand(t, server.URL, "creator-app", "domain", "verify", "app-1", "shop.example.com")
	if err != nil {
		t.Fatalf("domain verify failed: %v", err)
	}
	if gotPath != "/v1/cli/creator-apps/app-1/domains/shop.example.com/verify" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if !strings.Contains(output, `"verified": true`) && !strings.Contains(output, `"verified":true`) {
		t.Fatalf("verified state missing: %s", output)
	}
}

func TestCreatorAppShowRendersEmbedSnippet(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"id":"app-1","kind":"EXTERNAL","name":"Demo","domains":[{"domain":"shop.example.com","verified":true,"verificationToken":null}],"createdAt":"2026-08-18T00:00:00Z"}]}`))
	}))
	defer server.Close()

	output, err := executeCreatorAppCommand(t, server.URL, "creator-app", "show", "app-1")
	if err != nil {
		t.Fatalf("show failed: %v", err)
	}
	var decoded struct {
		Data struct {
			EmbedSnippet string `json:"embedSnippet"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if !strings.Contains(decoded.Data.EmbedSnippet, `data-creator-app-id="app-1"`) {
		t.Fatalf("embed snippet missing app id: %s", decoded.Data.EmbedSnippet)
	}
	if !strings.Contains(decoded.Data.EmbedSnippet, "/widget/tip-embed.js") {
		t.Fatalf("embed snippet missing script url: %s", decoded.Data.EmbedSnippet)
	}
}

func TestCreatorAppList(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/cli/creator-apps" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"id":"app-1","kind":"EXTERNAL","name":"Demo","domains":[],"createdAt":"2026-08-18T00:00:00Z"}]}`))
	}))
	defer server.Close()

	output, err := executeCreatorAppCommand(t, server.URL, "creator-app", "list")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(output, "app-1") {
		t.Fatalf("list output missing app: %s", output)
	}
}
