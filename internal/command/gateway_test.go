package command

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

type gatewayTestTransport func(*http.Request) (*http.Response, error)

func (f gatewayTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGatewayUpgradeKeepsProfileCredentialAndLogout(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	for _, region := range []config.Region{config.RegionCN, config.RegionGlobal} {
		t.Run(string(region), func(t *testing.T) {
			home := t.TempDir()
			configured := config.Default(region)
			legacy := "https://api.viceme.cn"
			if region == config.RegionGlobal {
				legacy = "https://api.viceme.ai"
			}
			configured.Profiles[0].APIBaseURL = legacy
			data, _ := json.Marshal(configured)
			if err := os.WriteFile(config.ConfigPath(home), data, 0o600); err != nil {
				t.Fatal(err)
			}
			store := securestore.NewMemory()
			// Compute the historical key directly, without the new normalizer.
			digest := sha256.Sum256([]byte(legacy))
			old := auth.Manager{Store: store, ProfileID: configured.Profiles[0].ID, Scope: fmt.Sprintf("custom:%x", digest), LegacyRegion: string(region)}
			if err := old.Save(auth.Credential{AccessToken: "existing-test-token", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
				t.Fatal(err)
			}
			calls := 0
			client := &http.Client{Transport: gatewayTestTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != config.APIBaseURL(region)+"/v1/cli/auth/status" || r.Header.Get("Authorization") != "Bearer existing-test-token" {
					t.Fatalf("upgrade used the wrong endpoint or credential: %s", r.URL)
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"authenticated":true,"user":{"id":"55555555-5555-4555-8555-555555555555","displayName":"Creator","avatarUrl":null},"scopes":["profile:read"],"expiresAt":"2027-08-12T08:00:00Z"}`))}, nil
			})}
			var stdout, stderr bytes.Buffer
			exit := Execute([]string{"auth", "status"}, Dependencies{Out: &stdout, ErrOut: &stderr, Store: store, HTTPClient: client, Environment: skillcontent.Environment{Home: home, ConfigDir: home}})
			if exit != 0 || calls != 1 {
				t.Fatalf("existing login not reused: exit=%d calls=%d response=%s", exit, calls, stdout.String())
			}
			current, _ := os.ReadFile(config.ConfigPath(home))
			if !bytes.Equal(data, current) {
				t.Fatal("ordinary command rewrote profile")
			}
			scope, err := credentialScopeForAPIBase(config.APIBaseURL(region))
			if err != nil || scope != old.Scope {
				t.Fatalf("credential key changed: %v", err)
			}
			manager := auth.Manager{Store: store, ProfileID: old.ProfileID, Scope: scope, LegacyRegion: string(region)}
			if err := manager.Delete(); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{old.StorageKey(), "credential:" + old.ProfileID + ":" + string(region)} {
				if _, err := store.Get(key); !errors.Is(err, securestore.ErrNotFound) {
					t.Fatalf("logout left a reusable old credential: %v", err)
				}
			}
		})
	}
}

func TestGatewayProcessCredentialIsRestrictedToExactAPIBase(t *testing.T) {
	for _, endpoint := range []string{"https://viceme.cn/api", "https://viceme.ai/api", "https://api.viceme.cn", "http://localhost:3001"} {
		if err := validatePublicationCredentialTarget(endpoint); err != nil {
			t.Fatalf("valid endpoint %s: %v", endpoint, err)
		}
	}
	for _, endpoint := range []string{"https://viceme.cn", "https://viceme.ai/profile", "https://viceme.cn/api/other", "https://viceme.cn:8443/api", "https://api.viceme.cn/other", "https://api.viceme.cn.example.com"} {
		if validatePublicationCredentialTarget(endpoint) == nil || legacyCredentialRegionForAPIBase(endpoint) != "" {
			t.Fatalf("credential target too broad: %s", endpoint)
		}
	}
}

func TestGatewayUpgradeFindsOriginalPendingTrialRequest(t *testing.T) {
	home := t.TempDir()
	runtime := &Runtime{configBase: t.TempDir(), apiBaseURL: config.APIBaseURL(config.RegionCN), region: config.RegionCN, deps: Dependencies{Environment: skillcontent.Environment{Home: home}}}
	digest := sha256.Sum256([]byte("https://api.viceme.cn\x00" + downloadableProductID))
	filename := filepath.Join(runtime.configBase, "trial-use-pending", fmt.Sprintf("%x.json", digest[:16]))
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(trialUsePending{ProductID: downloadableProductID, RequestID: "unconfirmed-before-cutover", CreatedAt: 1})
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatal(err)
	}
	state := scriptTrialState{InstallID: "existing-install", Secret: "test-secret", ProductID: downloadableProductID, Market: "cn"}
	if err := withScriptTrialLock(runtime, downloadableProductID, func() error {
		return migrateLegacyTrialUsePending(runtime, downloadableProductID, &state)
	}); err != nil {
		t.Fatal(err)
	}
	if state.PendingRequestID != "unconfirmed-before-cutover" || state.InstallID != "existing-install" {
		t.Fatalf("pending identity lost: %#v", state)
	}
}

func TestGatewayUpgradeRepairsOldSkillInPlace(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()
	home, store := t.TempDir(), securestore.NewMemory()
	serverURL, _ := url.Parse(state.server.URL)
	client := &http.Client{Transport: gatewayTestTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "viceme.cn" {
			if !strings.HasPrefix(r.URL.Path, "/api/v1/") {
				t.Fatalf("missing Gateway prefix: %s", r.URL)
			}
			cloned := r.Clone(context.Background())
			cloned.URL.Scheme, cloned.URL.Host = serverURL.Scheme, serverURL.Host
			cloned.URL.Path = strings.TrimPrefix(cloned.URL.Path, "/api")
			return state.server.Client().Transport.RoundTrip(cloned)
		}
		if r.URL.Host != serverURL.Host {
			t.Fatalf("unexpected network target: %s", r.URL)
		}
		return state.server.Client().Transport.RoundTrip(r)
	})}
	run := func(args ...string) map[string]any {
		t.Helper()
		var out, errOut bytes.Buffer
		code := Execute(args, Dependencies{Out: &out, ErrOut: &errOut, Store: store, HTTPClient: client, Environment: skillcontent.Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli")}})
		var result map[string]any
		if err := json.Unmarshal(out.Bytes(), &result); err != nil || code != 0 {
			t.Fatalf("%v: exit=%d out=%s err=%s", args, code, out.String(), errOut.String())
		}
		return result
	}
	run("skill", "install", downloadableProductID, "--agent", "agents")
	directory := filepath.Join(home, ".agents", "skills", "free-test")
	filename := filepath.Join(directory, ".viceme/runtime.json")
	raw, _ := os.ReadFile(filename)
	var manifest skillcontent.RuntimeManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.APIBaseURL = "https://api.viceme.cn"
	raw, _ = json.Marshal(manifest)
	if err := os.WriteFile(filename, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "user-output.txt"), []byte("preserve"), 0o644); err != nil {
		t.Fatal(err)
	}
	trialPath := filepath.Join(home, ".viceme/trial", downloadableProductID+".json")
	before, err := os.ReadFile(trialPath)
	if err != nil {
		t.Fatal(err)
	}
	ready := run("skill", "ready", downloadableProductID, "--skill-dir", directory)
	if ready["data"].(map[string]any)["ready"] != false {
		t.Fatal("old embedded script incorrectly reported ready")
	}
	run("skill", "install", downloadableProductID, "--skill-dir", directory)
	ready = run("skill", "ready", downloadableProductID, "--skill-dir", directory)
	if ready["data"].(map[string]any)["ready"] != true {
		t.Fatalf("repair not ready: %#v", ready)
	}
	after, _ := os.ReadFile(trialPath)
	if !bytes.Equal(before, after) || len(state.useRequests) != 0 {
		t.Fatal("repair changed trial state or consumed a use")
	}
	if raw, err := os.ReadFile(filepath.Join(directory, "user-output.txt")); err != nil || string(raw) != "preserve" {
		t.Fatal("repair lost user output")
	}
}
