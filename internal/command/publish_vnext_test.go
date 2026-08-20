package command

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestTopLevelSkillPublishUsesThePrivateDraftFlowAndPreservesSkillPublish(t *testing.T) {
	state := &publicationAPITestState{
		publicationID: "22222222-2222-4222-8222-222222222222",
		reviewDigest:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	server := httptest.NewServer(http.HandlerFunc(state.serveHTTP))
	defer server.Close()
	state.baseURL = server.URL

	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.MkdirTemp(workingDirectory, "canghe-comic-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(source)
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte(`---
name: Canghe Comic
description: Generate deterministic educational comics.
---

# Canghe Comic
`), 0o644); err != nil {
		t.Fatal(err)
	}
	relativeSource, err := filepath.Rel(workingDirectory, source)
	if err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
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
	dependencies := Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: store, APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		NewID:       func() string { return "11111111-1111-4111-8111-111111111111" },
	}
	execute := func(arguments ...string) (int, map[string]any) {
		t.Helper()
		stdout.Reset()
		stderr.Reset()
		exit := Execute(arguments, dependencies)
		var envelope map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("command did not emit one JSON envelope: exit=%d stdout=%q stderr=%q err=%v", exit, stdout.String(), stderr.String(), err)
		}
		return exit, envelope
	}

	for _, testCase := range []struct {
		name string
		args []string
		code string
	}{
		{name: "missing path", args: []string{"publish"}, code: "SKILL_PATH_REQUIRED"},
		{name: "multiple paths", args: []string{"publish", "one", "two"}, code: "SKILL_PATH_ARGUMENTS_INVALID"},
		{name: "invalid source", args: []string{"publish", "missing-canghe-comic"}, code: "SKILL_PATH_NOT_FOUND"},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			exit, envelope := execute(testCase.args...)
			if exit != output.ExitValidation {
				t.Fatalf("validation exit=%d envelope=%#v", exit, envelope)
			}
			errorData, _ := envelope["error"].(map[string]any)
			if errorData["code"] != testCase.code {
				t.Fatalf("validation code=%#v envelope=%#v", errorData["code"], envelope)
			}
		})
	}

	if exit, envelope := execute("publish", relativeSource); exit == 0 || envelope["ok"] != false {
		t.Fatalf("lost create response did not preserve recoverable failure: exit=%d envelope=%#v", exit, envelope)
	}
	if exit, envelope := execute("publish", relativeSource); exit != 0 || envelope["ok"] != true {
		t.Fatalf("top-level publish did not recover the private Draft: exit=%d envelope=%#v", exit, envelope)
	} else {
		data, _ := envelope["data"].(map[string]any)
		presentation, _ := data["presentation"].(map[string]any)
		if data["publicationId"] != state.publicationID || data["requiresPrice"] != true || presentation["intent"] != "OPEN_OWNER_PREVIEW" || presentation["fallbackUrl"] == "" {
			t.Fatalf("top-level publish did not return the private Draft owner preview: %#v", envelope)
		}
	}
	if _, err := os.Stat(filepath.Join(source, ".viceme", "skill.json")); err != nil {
		t.Fatalf("relative source did not retain the workspace binding: %v", err)
	}

	state.mu.Lock()
	if state.createCalls != 2 || !state.packageVerified || state.mediaVerified || state.status != "DRAFT" || state.confirmCalls != 0 || state.publishCalls != 0 {
		state.mu.Unlock()
		t.Fatalf("top-level command left the private Draft flow: %#v", state)
	}
	state.mu.Unlock()

	if exit, envelope := execute("skill", "publish", "--path", relativeSource); exit != 0 || envelope["ok"] != true {
		t.Fatalf("existing skill publish command regressed: exit=%d envelope=%#v", exit, envelope)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.createCalls != 2 || state.confirmCalls != 0 || state.publishCalls != 0 || state.status != "DRAFT" {
		t.Fatalf("existing skill publish no longer preserved the same private Draft: %#v", state)
	}
}
