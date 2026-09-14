package command

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/api"
)

const tutorialWorkID = "11111111-1111-4111-8111-111111111111"
const tutorialProductID = "22222222-2222-4222-8222-222222222222"

func tutorialContentFixture() api.WebsiteTutorialContent {
	return api.WebsiteTutorialContent{SchemaVersion: 1, Title: "创作教程", Summary: "真实创作步骤", Prerequisites: []string{}, Steps: []api.WebsiteTutorialStep{{Title: "构建页面", Explanation: "从需求开始", Prompt: "私有完整提示词"}}}
}
func tutorialViewFixture(unlocked bool) api.WebsiteTutorialView {
	content := tutorialContentFixture()
	product := tutorialProductID
	view := api.WebsiteTutorialView{WorkID: tutorialWorkID, Version: 2, Title: content.Title, Summary: content.Summary, Prerequisites: content.Prerequisites, PriceCents: 100, ProductID: &product, Unlocked: unlocked, Steps: []api.WebsiteTutorialViewStep{{Title: content.Steps[0].Title}}}
	if unlocked {
		view.Steps[0].Content = &content.Steps[0]
	}
	return view
}
func TestTutorialReadAndDownloadRespectVersionAndCredentials(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "vme_cli_1234567890123456789012345678901234567890123")
	for _, action := range []string{"preview", "read", "download", "buy"} {
		t.Run(action, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				expected := "/v1/cli/works/" + tutorialWorkID + "/tutorial"
				if action == "preview" {
					expected = "/v1/public/works/" + tutorialWorkID + "/tutorial"
					if r.Header.Get("Authorization") != "" {
						t.Error("public preview leaked credential")
					}
				} else if r.Header.Get("Authorization") != "Bearer vme_cli_1234567890123456789012345678901234567890123" {
					t.Error("private read missing credential")
				}
				if r.URL.Path != expected || r.URL.Query().Get("version") != "2" {
					t.Errorf("unexpected request %s", r.URL)
				}
				writeJSONResponse(w, tutorialViewFixture(action != "preview" && action != "buy"))
			}))
			defer server.Close()
			args := []string{"tutorial", action, tutorialWorkID, "--version", "2"}
			target := filepath.Join(t.TempDir(), "tutorial.json")
			if action == "download" {
				args = append(args, "--output", target)
			}
			exit, envelope := executeMerchantOnboardingCommand(t, server, args...)
			if exit != 0 {
				t.Fatalf("failed: %#v", envelope)
			}
			data, _ := json.Marshal(envelope)
			if action == "preview" && strings.Contains(string(data), "私有完整提示词") {
				t.Fatal("preview leaked private content")
			}
			if action == "buy" && (!strings.Contains(string(data), "OPEN_CHECKOUT") || !strings.Contains(string(data), tutorialProductID)) {
				t.Fatalf("missing checkout: %s", data)
			}
			if action == "download" {
				contents, err := os.ReadFile(target)
				if err != nil {
					t.Fatal(err)
				}
				var downloaded api.WebsiteTutorialContent
				if json.Unmarshal(contents, &downloaded) != nil || downloaded.Steps[0].Prompt != "私有完整提示词" {
					t.Fatal("incorrect download")
				}
				if strings.Contains(string(data), "私有完整提示词") {
					t.Fatal("download echoed private content")
				}
			}
		})
	}
}
func TestTutorialLockedAndMalformedResponsesNeverWriteFiles(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "vme_cli_1234567890123456789012345678901234567890123")
	for _, kind := range []string{"locked", "missing-content", "wrong-version", "wrong-work"} {
		t.Run(kind, func(t *testing.T) {
			view := tutorialViewFixture(false)
			switch kind {
			case "missing-content":
				view.Unlocked = true
			case "wrong-version":
				view.Version = 3
			case "wrong-work":
				view.WorkID = tutorialProductID
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writeJSONResponse(w, view) }))
			defer server.Close()
			target := filepath.Join(t.TempDir(), "tutorial.json")
			exit, _ := executeMerchantOnboardingCommand(t, server, "tutorial", "download", tutorialWorkID, "--version", "2", "--output", target)
			if exit == 0 {
				t.Fatal("invalid response succeeded")
			}
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				t.Fatal("wrote incomplete or locked tutorial")
			}
		})
	}
}
func TestTutorialPublishSendsStableRequestAndExplicitCommercialTerms(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "vme_cli_1234567890123456789012345678901234567890123")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/cli/auth/status" {
			writeJSONResponse(w, map[string]any{"authenticated": true, "user": map[string]any{"id": tutorialWorkID, "displayName": "Creator", "avatarUrl": nil}, "scopes": []string{"merchant-commerce:read", "merchant-commerce:write"}, "expiresAt": "2027-08-27T00:00:00Z"})
			return
		}
		if r.URL.Path != "/v1/cli/merchant/works/"+tutorialWorkID+"/tutorial" || r.Method != "POST" {
			t.Fatalf("unexpected %s", r.URL)
		}
		var input api.PublishWebsiteTutorialRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if input.ExpectedVersion != 1 || input.ClientRequestID != tutorialProductID || input.PriceCents != 100 || input.PreviewStepCount != 0 || input.Content.Steps[0].Prompt != "私有完整提示词" {
			t.Fatalf("incorrect input %#v", input)
		}
		writeJSONResponse(w, tutorialViewFixture(true))
	}))
	defer server.Close()
	file := filepath.Join(t.TempDir(), "input.json")
	data, _ := json.Marshal(tutorialContentFixture())
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"merchant", "work", "tutorial", "publish", tutorialWorkID, "--input", file, "--merchant", tutorialWorkID, "--request-id", tutorialProductID, "--expected-version", "1", "--price-cents", "100", "--preview-steps", "0"}
	for i := 0; i < 2; i++ {
		exit, e := executeMerchantOnboardingCommand(t, server, args...)
		if exit != 0 {
			t.Fatalf("publication failed %#v", e)
		}
	}
}
func TestTutorialPublicationRejectsInvalidInputBeforeNetwork(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("invalid input reached network") }))
	defer server.Close()
	for _, data := range []string{`{"schemaVersion":1,"unknown":true}`, `{"schemaVersion":1,"title":"x","prerequisites":[],"steps":[{"title":"x","prompt":""}]}`} {
		file := filepath.Join(t.TempDir(), "input.json")
		_ = os.WriteFile(file, []byte(data), 0600)
		exit, _ := executeMerchantOnboardingCommand(t, server, "merchant", "work", "tutorial", "publish", tutorialWorkID, "--input", file, "--merchant", tutorialWorkID, "--request-id", tutorialProductID, "--expected-version", "0", "--price-cents", "0", "--preview-steps", "0")
		if exit == 0 {
			t.Fatal("invalid content accepted")
		}
	}
}
func TestTutorialDownloadDoesNotOverwriteFilesOrSymlinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "keep.json")
	_ = os.WriteFile(target, []byte("keep"), 0600)
	if _, err := writeTutorialFile(target, tutorialContentFixture()); err == nil {
		t.Fatal("overwrote existing file")
	}
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skip(err)
	}
	if _, err := writeTutorialFile(link, tutorialContentFixture()); err == nil {
		t.Fatal("followed output symlink")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "keep" {
		t.Fatal("changed existing file")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Fatal("temporary output leaked")
	}
}
