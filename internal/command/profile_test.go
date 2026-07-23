package command

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestProfileAddAndConfigureExplicitLocalOverrides(t *testing.T) {
	t.Parallel()
	localToken := testProcessCredential("local-dev")
	home := t.TempDir()
	environment := skillcontent.Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli")}
	store := securestore.NewMemory()
	dependencies := Dependencies{Environment: environment}

	code, stdout, stderr, _ := runCLIWithDependencies(t, nil, store, "", dependencies,
		"profile", "add", "--name", "local", "--region", "cn", "--api-base-url", "http://localhost:8090/", "--access-token", localToken, "--use")
	if code != 0 || stderr != "" || strings.Contains(stdout, localToken) || !strings.Contains(stdout, `"access_token_configured":true`) {
		t.Fatalf("add code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	loaded, err := config.LoadOrDefault(environment.ConfigDir)
	if err != nil {
		t.Fatal(err)
	}
	local, err := loaded.Resolve("local")
	if err != nil || local.APIBaseURL != "http://localhost:8090" || local.AccessToken != localToken {
		t.Fatalf("profile=%#v err=%v", local, err)
	}

	var requestSeen bool
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requestSeen = true
		if request.URL.Scheme+"://"+request.URL.Host != "http://localhost:8090" {
			t.Fatalf("request origin=%s", request.URL.Scheme+"://"+request.URL.Host)
		}
		if request.Header.Get("x-api-key") != localToken {
			t.Fatal("local profile token was not applied")
		}
		return jsonHTTPResponse(request, http.StatusOK, `{"targets":[]}`), nil
	})
	code, stdout, stderr, _ = runCLIWithDependencies(t, nil, store, "", Dependencies{
		Environment: environment,
		HTTPClient:  &http.Client{Transport: transport},
	}, "skill", "target", "list")
	if code != 0 || stderr != "" || !requestSeen || strings.Contains(stdout, localToken) {
		t.Fatalf("request code=%d seen=%v stdout=%s stderr=%s", code, requestSeen, stdout, stderr)
	}

	code, stdout, stderr, _ = runCLIWithDependencies(t, nil, store, "", dependencies, "auth", "status")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"source":"local_profile"`) || strings.Contains(stdout, localToken) {
		t.Fatalf("status code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	code, _, stderr, _ = runCLIWithDependencies(t, nil, store, "", dependencies, "auth", "login")
	if code == 0 || !strings.Contains(stderr, "local_profile_credential_active") || strings.Contains(stderr, localToken) {
		t.Fatalf("login code=%d stderr=%s", code, stderr)
	}
	code, stdout, stderr, _ = runCLIWithDependencies(t, nil, store, "", dependencies, "profile", "list")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"credential_source":"local_profile"`) || strings.Contains(stdout, localToken) {
		t.Fatalf("list code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	code, stdout, stderr, _ = runCLIWithDependencies(t, nil, store, "", dependencies,
		"profile", "configure", "local", "--api-base-url", "http://localhost:9090")
	if code == 0 || !strings.Contains(stderr, "profile_access_token_scope") || strings.Contains(stdout+stderr, localToken) {
		t.Fatalf("cross-origin configure code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	loaded, err = config.LoadOrDefault(environment.ConfigDir)
	if err != nil {
		t.Fatal(err)
	}
	local, _ = loaded.Resolve("local")
	if local.APIBaseURL != "http://localhost:8090" || local.AccessToken != localToken {
		t.Fatalf("rejected endpoint change was persisted: %#v", local)
	}

	code, stdout, stderr, _ = runCLIWithDependencies(t, nil, store, "", dependencies,
		"profile", "configure", "local", "--clear-access-token", "--clear-api-base-url")
	if code != 0 || stderr != "" || strings.Contains(stdout, localToken) || !strings.Contains(stdout, `"access_token_configured":false`) {
		t.Fatalf("clear code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	loaded, err = config.LoadOrDefault(environment.ConfigDir)
	if err != nil {
		t.Fatal(err)
	}
	local, _ = loaded.Resolve("local")
	if local.AccessToken != "" || local.APIBaseURL != "" {
		t.Fatalf("local overrides were not cleared: %#v", local)
	}
}

func TestProfileConfigureValidatesExplicitAccessToken(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	environment := skillcontent.Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli")}
	store := securestore.NewMemory()
	dependencies := Dependencies{Environment: environment}
	code, _, stderr, _ := runCLIWithDependencies(t, nil, store, "", dependencies, "profile", "add", "--name", "local")
	if code != 0 || stderr != "" {
		t.Fatalf("add code=%d stderr=%s", code, stderr)
	}
	code, stdout, stderr, _ := runCLIWithDependencies(t, nil, store, "", dependencies,
		"profile", "configure", "local", "--access-token", " replacement-secret ")
	if code == 0 || !strings.Contains(stderr, "profile_access_token") || strings.Contains(stdout+stderr, "replacement-secret") {
		t.Fatalf("invalid token code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	code, _, stderr, _ = runCLIWithDependencies(t, nil, store, "", dependencies,
		"profile", "configure", "local", "--api-base-url", "http://localhost:8090", "--access-token", strings.Repeat("x", (64<<10)+1))
	if code == 0 || !strings.Contains(stderr, "64 KiB") {
		t.Fatalf("oversized token code=%d stderr=%s", code, stderr)
	}
	code, _, stderr, _ = runCLIWithDependencies(t, nil, store, "", dependencies,
		"profile", "configure", "local", "--api-base-url", "http://localhost:8090", "--access-token", testProcessCredential("cn-prod"))
	if code == 0 || !strings.Contains(stderr, "profile_access_token_scope") {
		t.Fatalf("audience mismatch code=%d stderr=%s", code, stderr)
	}
}

func TestProcessCredentialTakesPriorityOverLocalProfileCredential(t *testing.T) {
	processToken := "vpa1.local-dev." + strings.Repeat("p", 43)
	profileToken := "vpa1.local-dev." + strings.Repeat("l", 43)
	t.Setenv(processAccessTokenEnvironment, processToken)
	t.Setenv(localProcessCredentialEnvironment, "1")
	home := t.TempDir()
	environment := skillcontent.Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli")}
	configured := config.Default(config.RegionCN)
	configured.Profiles[0].APIBaseURL = "http://localhost:8090"
	configured.Profiles[0].AccessToken = profileToken
	if _, err := config.Save(environment.ConfigDir, configured); err != nil {
		t.Fatal(err)
	}
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("x-api-key") != processToken {
			t.Fatalf("credential priority header=%q", request.Header.Get("x-api-key"))
		}
		return jsonHTTPResponse(request, http.StatusOK, `{"targets":[]}`), nil
	})
	code, stdout, stderr, _ := runCLIWithDependencies(t, nil, securestore.NewMemory(), "", Dependencies{
		Environment: environment,
		HTTPClient:  &http.Client{Transport: transport},
	}, "skill", "target", "list")
	if code != 0 || stderr != "" || strings.Contains(stdout, processToken) || strings.Contains(stdout, profileToken) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestProfileLifecycleAndGlobalOverride(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	environment := skillcontent.Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli")}
	store := securestore.NewMemory()
	dependencies := Dependencies{Environment: environment}

	code, stdout, stderr, _ := runCLIWithDependencies(t, nil, store, "", dependencies,
		"profile", "add", "--name", "work", "--region", "global", "--use")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"name":"work"`) {
		t.Fatalf("add code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}

	loaded, err := config.LoadOrDefault(environment.ConfigDir)
	if err != nil || loaded.CurrentProfile != "work" || loaded.PreviousProfile != "default" {
		t.Fatalf("config=%#v err=%v", loaded, err)
	}
	work, err := loaded.Resolve("work")
	if err != nil {
		t.Fatal(err)
	}
	manager := credentialauth.Manager{
		Store:       store,
		Region:      "global",
		ProfileID:   work.ID,
		ProfileName: work.Name,
	}
	if err := manager.Save(credentialauth.Credential{AccessToken: "work-token", UserID: "user-work", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr, _ = runCLIWithDependencies(t, nil, store, "", dependencies,
		"--profile", "work", "auth", "status")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"profile":"work"`) || !strings.Contains(stdout, `"authenticated":true`) {
		t.Fatalf("status code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}

	code, stdout, stderr, _ = runCLIWithDependencies(t, nil, store, "", dependencies,
		"profile", "list")
	if code != 0 || stderr != "" {
		t.Fatalf("list code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	var envelope struct {
		Data []profileListItem `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Data) != 2 || envelope.Data[1].Name != "work" || !envelope.Data[1].Active || !envelope.Data[1].Authenticated {
		t.Fatalf("unexpected profiles: %#v", envelope.Data)
	}

	code, _, stderr, _ = runCLIWithDependencies(t, nil, store, "", dependencies,
		"profile", "rename", "work", "company")
	if code != 0 || stderr != "" {
		t.Fatalf("rename code=%d stderr=%s", code, stderr)
	}
	loaded, _ = config.LoadOrDefault(environment.ConfigDir)
	company, err := loaded.Resolve("company")
	if err != nil || company.ID != work.ID {
		t.Fatalf("renamed profile lost identity: %#v err=%v", company, err)
	}

	code, _, stderr, _ = runCLIWithDependencies(t, nil, store, "", dependencies,
		"profile", "use", "default")
	if code != 0 || stderr != "" {
		t.Fatalf("use code=%d stderr=%s", code, stderr)
	}
	code, _, stderr, _ = runCLIWithDependencies(t, nil, store, "", dependencies,
		"profile", "remove", "company")
	if code != 0 || stderr != "" {
		t.Fatalf("remove code=%d stderr=%s", code, stderr)
	}
	if _, err := manager.Load(); err == nil {
		t.Fatal("removed profile credentials still exist")
	}
}

func TestProfileUseToggleAndValidation(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	environment := skillcontent.Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli")}
	dependencies := Dependencies{Environment: environment}
	store := securestore.NewMemory()
	code, _, stderr, _ := runCLIWithDependencies(t, nil, store, "", dependencies,
		"profile", "add", "--name", "global", "--region", "global")
	if code != 0 || stderr != "" {
		t.Fatalf("add code=%d stderr=%s", code, stderr)
	}
	for _, name := range []string{"global", "-"} {
		code, _, stderr, _ = runCLIWithDependencies(t, nil, store, "", dependencies, "profile", "use", name)
		if code != 0 || stderr != "" {
			t.Fatalf("use %s code=%d stderr=%s", name, code, stderr)
		}
	}
	loaded, _ := config.LoadOrDefault(environment.ConfigDir)
	if loaded.CurrentProfile != "default" || loaded.PreviousProfile != "global" {
		t.Fatalf("toggle did not restore default: %#v", loaded)
	}
	code, _, stderr, _ = runCLIWithDependencies(t, nil, store, "", dependencies,
		"--profile", "missing", "auth", "status")
	if code != 2 || !strings.Contains(stderr, "profile_not_found") {
		t.Fatalf("missing profile code=%d stderr=%s", code, stderr)
	}
}

func TestInstallUsesSelectedProfileRegionWhenRegionFlagIsOmitted(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	environment := skillcontent.Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli")}
	store := securestore.NewMemory()
	dependencies := Dependencies{
		Environment: environment,
		Updater:     &fakeUpdateService{},
	}
	code, _, stderr, _ := runCLIWithDependencies(t, nil, store, "", dependencies,
		"profile", "add", "--name", "work", "--region", "global")
	if code != 0 || stderr != "" {
		t.Fatalf("add code=%d stderr=%s", code, stderr)
	}
	code, stdout, stderr, _ := runCLIWithDependencies(t, nil, store, "", dependencies,
		"--profile", "work", "install", "--target", "codex")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"profile":"work"`) || !strings.Contains(stdout, `"region":"global"`) {
		t.Fatalf("install code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	loaded, err := config.LoadOrDefault(environment.ConfigDir)
	if err != nil {
		t.Fatal(err)
	}
	defaultProfile, _ := loaded.Resolve("default")
	workProfile, _ := loaded.Resolve("work")
	if defaultProfile.Region != config.RegionCN || workProfile.Region != config.RegionGlobal {
		t.Fatalf("install changed the wrong profile: %#v", loaded)
	}
}

func TestInstallRegionChangeRemovesPreviousProfileCredential(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	environment := skillcontent.Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli")}
	store := securestore.NewMemory()
	manager := credentialauth.Manager{
		Store:       store,
		Region:      "cn",
		ProfileID:   "default",
		ProfileName: "default",
	}
	if err := manager.Save(credentialauth.Credential{AccessToken: "cn-token", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr, _ := runCLIWithDependencies(t, nil, store, "", Dependencies{
		Environment: environment,
		Updater:     &fakeUpdateService{},
	}, "install", "--target", "codex", "--region", "global")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"region":"global"`) || !strings.Contains(stdout, `"authenticated":false`) {
		t.Fatalf("install code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if _, err := manager.Load(); err == nil {
		t.Fatal("previous region credential still exists")
	}
}
