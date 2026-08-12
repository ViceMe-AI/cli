package command

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestCustomEndpointProfilePersistsRoutesAndRemovesScopedCredential(t *testing.T) {
	const accessToken = "vme_cli_1234567890123456789012345678901234567890123"
	var statusRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/cli/auth/status" {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		statusRequests.Add(1)
		if request.Header.Get("Authorization") != "Bearer "+accessToken {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		writeJSONResponse(writer, map[string]any{
			"authenticated": true,
			"user": map[string]any{
				"id": "55555555-5555-4555-8555-555555555555", "displayName": "Creator", "avatarUrl": nil,
			},
			"scopes":    []string{"profile:read", "skill-publication:read", "skill-publication:write"},
			"expiresAt": "2027-08-12T08:00:00Z",
		})
	}))
	defer server.Close()

	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	store := securestore.NewMemory()
	dependencies := Dependencies{
		Store: store, HTTPClient: server.Client(),
		Environment: skillcontent.Environment{Home: root, ConfigDir: configDir},
	}
	run := func(arguments ...string) (int, map[string]any) {
		t.Helper()
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		dependencies.Out = &stdout
		dependencies.ErrOut = &stderr
		exit := Execute(arguments, dependencies)
		var envelope map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("invalid command response: exit=%d stdout=%q stderr=%q err=%v", exit, stdout.String(), stderr.String(), err)
		}
		return exit, envelope
	}

	endpoint := server.URL + "/api/"
	if exit, envelope := run("profile", "add", "--name", "shop-dev", "--region", "cn", "--api-base-url", endpoint, "--use"); exit != 0 || envelope["ok"] != true {
		t.Fatalf("could not add custom endpoint profile: exit=%d result=%#v", exit, envelope)
	}
	configured, err := config.LoadOrDefault(configDir)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := configured.Resolve("shop-dev")
	if err != nil {
		t.Fatal(err)
	}
	if profile.APIBaseURL != server.URL+"/api" || configured.CurrentProfile != profile.Name {
		t.Fatalf("custom profile was not persisted canonically: %#v", configured)
	}
	scope, err := credentialScopeForAPIBase(profile.ResolvedAPIBaseURL(), profile.Region)
	if err != nil {
		t.Fatal(err)
	}
	manager := credentialauth.Manager{
		Store: store, Region: string(profile.Region), ProfileID: profile.ID, ProfileName: profile.Name, Scope: scope,
	}
	if err := manager.Save(credentialauth.Credential{
		AccessToken: accessToken, ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	if exit, envelope := run("auth", "status"); exit != 0 || envelope["ok"] != true || statusRequests.Load() != 1 {
		t.Fatalf("active profile did not route to its persisted endpoint: exit=%d requests=%d result=%#v", exit, statusRequests.Load(), envelope)
	}
	if exit, envelope := run("profile", "list"); exit != 0 || envelope["ok"] != true {
		t.Fatalf("profile list failed: exit=%d result=%#v", exit, envelope)
	} else {
		items, ok := envelope["data"].([]any)
		if !ok || len(items) != 2 {
			t.Fatalf("profile list returned unexpected data: %#v", envelope)
		}
		shopDev, ok := items[1].(map[string]any)
		if !ok || shopDev["apiBaseUrl"] != server.URL+"/api" || shopDev["authenticated"] != true {
			t.Fatalf("profile list hid endpoint or used the wrong credential scope: %#v", items)
		}
	}
	if exit, envelope := run("profile", "remove", "shop-dev"); exit != 0 || envelope["ok"] != true {
		t.Fatalf("profile removal failed: exit=%d result=%#v", exit, envelope)
	}
	if _, err := store.Get(manager.StorageKey()); !errors.Is(err, securestore.ErrNotFound) {
		t.Fatalf("custom endpoint credential survived profile removal: %v", err)
	}
}
