package command

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/privatefile"
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
	webEndpoint := server.URL + "/web/"
	if exit, envelope := run("profile", "add", "--name", "shop-dev", "--api-base-url", endpoint, "--web-base-url", webEndpoint, "--market-region", "cn", "--use"); exit != 0 || envelope["ok"] != true {
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
	if profile.APIBaseURL != server.URL+"/api" || profile.WebBaseURL != server.URL+"/web" || profile.MarketRegion != config.RegionCN || configured.CurrentProfile != profile.Name {
		t.Fatalf("custom profile was not persisted canonically: %#v", configured)
	}
	scope, err := credentialScopeForAPIBase(profile.ResolvedAPIBaseURL())
	if err != nil {
		t.Fatal(err)
	}
	manager := credentialauth.Manager{
		Store: store, Region: string(configured.DistributionRegion), ProfileID: profile.ID, ProfileName: profile.Name, Scope: scope,
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
		if !ok || shopDev["apiBaseUrl"] != server.URL+"/api" || shopDev["webBaseUrl"] != server.URL+"/web" || shopDev["marketRegion"] != "cn" || shopDev["authenticated"] != true {
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

func TestProfileListMigratesOfficialLegacyCredential(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	store := securestore.NewMemory()
	legacy := credentialauth.Manager{Store: store, Region: "cn", ProfileID: "default", ProfileName: "default"}
	if err := legacy.Save(credentialauth.Credential{AccessToken: "legacy-official-token"}); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := Execute([]string{"profile", "list"}, Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: store,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
	})
	if exit != 0 {
		t.Fatalf("profile list failed: exit=%d stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	items, ok := envelope["data"].([]any)
	if !ok || len(items) != 1 || items[0].(map[string]any)["authenticated"] != true {
		t.Fatalf("profile list lost the official legacy credential: %#v", envelope)
	}
	scope, err := credentialScopeForAPIBase(config.APIBaseURL(config.RegionCN))
	if err != nil {
		t.Fatal(err)
	}
	endpoint := credentialauth.Manager{Store: store, Region: "cn", ProfileID: "default", ProfileName: "default", Scope: scope}
	if _, err := store.Get(endpoint.StorageKey()); err != nil {
		t.Fatalf("profile list did not migrate the endpoint credential: %v", err)
	}
	if _, err := store.Get(legacy.StorageKey()); err != nil {
		t.Fatalf("profile list removed the rollback credential: %v", err)
	}
}

func TestProfileRemoveAllRequiresConfirmationAndClearsEveryCredential(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	configured := config.Default(config.RegionCN)
	shopTest, err := configured.AddProfile(
		"shop-test",
		"https://shop-test.example.com/api",
		"https://shop-test.example.com",
		config.RegionCN,
	)
	if err != nil {
		t.Fatal(err)
	}
	configured.CurrentProfile = shopTest.Name
	if _, err := config.Save(configDir, configured); err != nil {
		t.Fatal(err)
	}

	store := securestore.NewMemory()
	credentialKeys := make([]string, 0, len(configured.Profiles))
	for _, profile := range configured.Profiles {
		scope, err := credentialScopeForAPIBase(profile.ResolvedAPIBaseURL())
		if err != nil {
			t.Fatal(err)
		}
		key := (&credentialauth.Manager{
			Store: store, Region: string(configured.DistributionRegion), ProfileID: profile.ID, ProfileName: profile.Name, Scope: scope,
		}).StorageKey()
		credentialKeys = append(credentialKeys, key)
		if err := store.Set(key, `{"access_token":"vme_cli_test"}`); err != nil {
			t.Fatal(err)
		}
	}

	dependencies := Dependencies{
		Store: store, Environment: skillcontent.Environment{Home: root, ConfigDir: configDir},
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

	if exit, envelope := run("profile", "remove", "--all"); exit != output.ExitConfirmation || envelope["ok"] != false {
		t.Fatalf("bulk removal did not require explicit confirmation: exit=%d result=%#v", exit, envelope)
	}
	unchanged, err := config.LoadOrDefault(configDir)
	if err != nil || len(unchanged.Profiles) != 2 || unchanged.CurrentProfile != "shop-test" {
		t.Fatalf("unconfirmed bulk removal changed profiles: config=%#v err=%v", unchanged, err)
	}
	for _, key := range credentialKeys {
		if _, err := store.Get(key); err != nil {
			t.Fatalf("unconfirmed bulk removal changed credential %q: %v", key, err)
		}
	}

	if exit, envelope := run("profile", "remove", "--all", "--yes"); exit != 0 || envelope["ok"] != true {
		t.Fatalf("confirmed bulk removal failed: exit=%d result=%#v", exit, envelope)
	} else if data, ok := envelope["data"].(map[string]any); !ok || data["removedCredentials"] != float64(len(credentialKeys)) {
		t.Fatalf("bulk removal reported the wrong credential count: %#v", envelope)
	}
	reset, err := config.LoadOrDefault(configDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(reset.Profiles) != 1 || reset.CurrentProfile != config.DefaultProfileName || reset.Profiles[0].Name != config.DefaultProfileName || reset.Profiles[0].UserID != "" {
		t.Fatalf("bulk removal did not create one clean default profile: %#v", reset)
	}
	for _, key := range credentialKeys {
		if _, err := store.Get(key); !errors.Is(err, securestore.ErrNotFound) {
			t.Fatalf("bulk removal retained credential %q: %v", key, err)
		}
	}
	if exit, envelope := run("auth", "status"); exit != 0 || envelope["ok"] != true {
		t.Fatalf("clean default auth status failed: exit=%d result=%#v", exit, envelope)
	} else if data, ok := envelope["data"].(map[string]any); !ok || data["authenticated"] != false || data["profile"] != "default" {
		t.Fatalf("clean default unexpectedly remained authenticated: %#v", envelope)
	}
}

type deleteFailureStore struct {
	store   *securestore.MemoryStore
	failKey string
	failed  bool
}

func (store *deleteFailureStore) Get(key string) (string, error) {
	return store.store.Get(key)
}

func (store *deleteFailureStore) Set(key, value string) error {
	return store.store.Set(key, value)
}

func (store *deleteFailureStore) Delete(key string) error {
	if key == store.failKey && !store.failed {
		store.failed = true
		return errors.New("injected credential delete failure")
	}
	return store.store.Delete(key)
}

func TestProfileRemoveKeepsCredentialWhenConfigCommitFails(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	configured := config.Default(config.RegionCN)
	profile, err := configured.AddProfile(
		"shop-test", "https://shop-test.example.com/api", "https://shop-test.example.com", config.RegionCN,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.Save(configDir, configured); err != nil {
		t.Fatal(err)
	}
	store := securestore.NewMemory()
	scope, err := credentialScopeForAPIBase(profile.ResolvedAPIBaseURL())
	if err != nil {
		t.Fatal(err)
	}
	manager := credentialauth.Manager{
		Store: store, Region: "cn", ProfileID: profile.ID, ProfileName: profile.Name, Scope: scope,
	}
	if err := manager.Save(credentialauth.Credential{AccessToken: "credential-to-preserve"}); err != nil {
		t.Fatal(err)
	}

	originalRename := privatefile.RenameFile
	privatefile.RenameFile = func(string, string) error { return syscall.EIO }
	t.Cleanup(func() { privatefile.RenameFile = originalRename })
	var stdout bytes.Buffer
	exit := Execute([]string{"profile", "remove", profile.Name}, Dependencies{
		Out: &stdout, ErrOut: &bytes.Buffer{}, Store: store,
		Environment: skillcontent.Environment{Home: root, ConfigDir: configDir},
	})
	if exit != output.ExitInternal {
		t.Fatalf("config commit failure returned exit %d: %s", exit, stdout.String())
	}
	persisted, err := config.LoadOrDefault(configDir)
	if err != nil || persisted.FindProfileIndex(profile.Name) < 0 {
		t.Fatalf("failed removal changed the persisted Profile: config=%#v err=%v", persisted, err)
	}
	if _, err := manager.Load(); err != nil {
		t.Fatalf("failed Profile commit discarded its credential: %v", err)
	}
}

func TestProfileRemoveAllRollsBackConfigAndCredentialsOnDeleteFailure(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	configured := config.Default(config.RegionCN)
	profile, err := configured.AddProfile(
		"shop-test", "https://shop-test.example.com/api", "https://shop-test.example.com", config.RegionCN,
	)
	if err != nil {
		t.Fatal(err)
	}
	configured.CurrentProfile = profile.Name
	if _, err := config.Save(configDir, configured); err != nil {
		t.Fatal(err)
	}

	memory := securestore.NewMemory()
	keys := make([]string, 0, len(configured.Profiles))
	for _, candidate := range configured.Profiles {
		scope, err := credentialScopeForAPIBase(candidate.ResolvedAPIBaseURL())
		if err != nil {
			t.Fatal(err)
		}
		key := (&credentialauth.Manager{
			Store: memory, Region: "cn", ProfileID: candidate.ID, ProfileName: candidate.Name, Scope: scope,
		}).StorageKey()
		keys = append(keys, key)
		if err := memory.Set(key, "credential-for-"+candidate.Name); err != nil {
			t.Fatal(err)
		}
	}
	store := &deleteFailureStore{store: memory, failKey: keys[len(keys)-1]}
	var stdout bytes.Buffer
	exit := Execute([]string{"profile", "remove", "--all", "--yes"}, Dependencies{
		Out: &stdout, ErrOut: &bytes.Buffer{}, Store: store,
		Environment: skillcontent.Environment{Home: root, ConfigDir: configDir},
	})
	if exit != output.ExitAuthentication {
		t.Fatalf("credential delete failure returned exit %d: %s", exit, stdout.String())
	}
	persisted, err := config.LoadOrDefault(configDir)
	if err != nil || len(persisted.Profiles) != len(configured.Profiles) || persisted.CurrentProfile != configured.CurrentProfile {
		t.Fatalf("failed bulk cleanup did not restore Profiles: config=%#v err=%v", persisted, err)
	}
	for index, key := range keys {
		value, err := memory.Get(key)
		if err != nil || value != "credential-for-"+configured.Profiles[index].Name {
			t.Fatalf("failed bulk cleanup did not restore credential %q: value=%q err=%v", key, value, err)
		}
	}
}
