package command

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestWebsiteBindingKeepsStableSourceIdentity(t *testing.T) {
	filename := filepath.Join(t.TempDir(), ".viceme", "website.json")
	want := websiteBinding{
		SchemaVersion:   1,
		ClientWorkID:    "11111111-1111-4111-8111-111111111111",
		WorkID:          "22222222-2222-4222-8222-222222222222",
		WorkKey:         "wrk_stable_work_01",
		Region:          "cn",
		DisplayName:     "Dagou Tap",
		SourceURL:       "https://creator.example.com/dagou-tap",
		DescriptionZhCN: "创作者网站。",
		DescriptionEnUS: "A creator website.",
		Cover: &api.WebsiteCover{
			Digest: strings.Repeat("a", 64), SizeBytes: 128,
			FileName: "cover.png", ContentType: "image/png",
		},
	}
	if err := writeWebsiteBinding(filename, want); err != nil {
		t.Fatalf("writeWebsiteBinding() error = %v", err)
	}
	got, found, err := loadWebsiteBinding(filename)
	if err != nil {
		t.Fatalf("loadWebsiteBinding() error = %v", err)
	}
	if !found || got.ClientWorkID != want.ClientWorkID || got.WorkID != want.WorkID || got.WorkKey != want.WorkKey {
		t.Fatalf("binding = %#v, want stable identity %#v", got, want)
	}
}

func TestRequirePublishedWebsiteBindingRejectsMissingWorkKey(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, ".viceme", "website.json")
	if err := writeWebsiteBinding(filename, websiteBinding{
		SchemaVersion: 1,
		ClientWorkID:  "11111111-1111-4111-8111-111111111111",
		Region:        "cn",
		DisplayName:   "Pending",
		SourceURL:     "https://creator.example.com/pending",
	}); err != nil {
		t.Fatalf("writeWebsiteBinding() error = %v", err)
	}
	if _, _, err := requirePublishedWebsiteBinding(root); err == nil {
		t.Fatal("requirePublishedWebsiteBinding() error = nil")
	}
}

func TestWebsiteURLIsOptionalButValidatedWhenProvided(t *testing.T) {
	if err := validateWebsiteURL(""); err != nil {
		t.Fatalf("validateWebsiteURL(\"\") error = %v", err)
	}
	if err := validateWebsiteURL("https://creator.example.com/tool"); err != nil {
		t.Fatalf("validateWebsiteURL(valid) error = %v", err)
	}
	if err := validateWebsiteURL("creator.example.com/tool"); err == nil {
		t.Fatal("validateWebsiteURL(relative) error = nil")
	}
}

func TestWebsitePublishAcceptsFirstCreatorDisplayName(t *testing.T) {
	command := newWebsitePublishCommand(&Runtime{})
	flag := command.Flags().Lookup("creator-display-name")
	if flag == nil || flag.DefValue != "" {
		t.Fatalf("creator-display-name flag = %#v", flag)
	}
	for _, name := range []string{"description-zh-cn", "description-en-us", "cover"} {
		if metadataFlag := command.Flags().Lookup(name); metadataFlag == nil || metadataFlag.DefValue != "" {
			t.Fatalf("%s flag = %#v", name, metadataFlag)
		}
	}
}

func TestWebsitePublishRequiresSamePublicationScopesAsSkill(t *testing.T) {
	var publishCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/v1/cli/auth/status" {
			writeJSONResponse(writer, map[string]any{
				"authenticated": true,
				"user":          map[string]any{"id": "55555555-5555-4555-8555-555555555555", "displayName": "Creator", "avatarUrl": nil},
				"scopes":        []string{"profile:read", "skill-publication:read"},
				"expiresAt":     time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
			})
			return
		}
		publishCalled = true
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	root := t.TempDir()
	source := filepath.Join(root, "website")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "index.html"), []byte("website"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := securestore.NewMemory()
	scope, err := credentialScopeForAPIBase(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	manager := credentialauth.Manager{Store: store, Region: "cn", ProfileID: "default", ProfileName: "default", Scope: scope}
	if err := manager.Save(credentialauth.Credential{AccessToken: "vme_cli_test", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := Execute([]string{"website", "publish", "--path", source, "--name", "Test Website"}, Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: store, APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
	})
	if exit != output.ExitAuthentication {
		t.Fatalf("website publish scope failure exit = %d, stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	errorData, _ := envelope["error"].(map[string]any)
	details, _ := errorData["details"].(map[string]any)
	missingScopes, _ := details["missingScopes"].([]any)
	if errorData["code"] != "PUBLICATION_SCOPE_REQUIRED" || len(missingScopes) != 1 || missingScopes[0] != "skill-publication:write" {
		t.Fatalf("website publish did not enforce the Skill publication scopes: %#v", envelope)
	}
	if publishCalled {
		t.Fatal("website publish called a mutation endpoint without the required publication scopes")
	}
	if _, err := os.Stat(filepath.Join(source, ".viceme", "website.json")); !os.IsNotExist(err) {
		t.Fatalf("website binding was written before scope validation: %v", err)
	}
}

func TestWebsiteDigestChangesWithContentButIgnoresLocalBinding(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := digestWebsiteDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".viceme"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".viceme", "website.json"), []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignored, err := digestWebsiteDirectory(root)
	if err != nil || ignored != first {
		t.Fatalf("binding changed digest: %q, %v", ignored, err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := digestWebsiteDirectory(root)
	if err != nil || second == first {
		t.Fatalf("content did not change digest: %q, %v", second, err)
	}
}
