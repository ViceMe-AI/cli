package command

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
)

func TestOwnedSkillInstallPrecedesTrialAndStorefrontAvailability(t *testing.T) {
	for _, publicStatus := range []int{http.StatusOK, http.StatusNotFound} {
		t.Run(strconv.Itoa(publicStatus), func(t *testing.T) {
			t.Setenv(processAccessTokenEnvironment, skillPurchaseAccessToken)
			state := newSkillTrialTestServer(t)
			defer state.server.Close()
			state.paymentStatus = "PAID"
			var trialCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/skills/" + downloadableProductID + "/access":
					if publicStatus != http.StatusOK {
						w.WriteHeader(publicStatus)
						return
					}
				case "/v1/skills/" + downloadableProductID + "/trial-grants":
					trialCalls.Add(1)
				case "/v1/cli/skills/" + downloadableProductID + "/download":
					writeJSONResponse(w, map[string]any{
						"url": state.server.URL + "/artifact", "fileName": "paid.zip",
						"releaseId": downloadableReleaseID, "artifactDigest": state.archiveDigest,
					})
					return
				}
				state.serveHTTP(w, r)
			}))
			defer server.Close()
			home := t.TempDir()
			exit, envelope, _ := executeSkillTrialCommand(t, server, home, securestore.NewMemory(),
				"skill", "install", downloadableProductID, "--agent", "agents")
			if exit != 0 || envelope["ok"] != true {
				t.Fatalf("owned install failed: exit=%d envelope=%#v", exit, envelope)
			}
			data := envelope["data"].(map[string]any)
			if data["trial"] != nil || trialCalls.Load() != 0 {
				t.Fatalf("owned edition became a trial: data=%#v trial calls=%d", data, trialCalls.Load())
			}
			content, err := os.ReadFile(filepath.Join(home, ".agents", "skills", "free-test", "SKILL.md"))
			if err != nil || bytes.Contains(content, []byte(skillTrialGateMarker)) {
				t.Fatalf("owned install contains a trial gate: %q, %v", content, err)
			}
		})
	}
}

func TestSkillPurchasePinsTheVerifiedCredential(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillPurchaseTestServer(t)
	defer state.server.Close()
	store := securestore.NewMemory()
	var manager credentialauth.Manager
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/cli/auth/status" {
			// Simulate another process switching this Profile while the command is
			// running. Every request after verification must retain the first token.
			if err := manager.Save(credentialauth.Credential{AccessToken: "vme_cli_replaced_123456789012345678901234567890123456", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
				t.Error(err)
			}
		}
		state.serveHTTP(w, r)
	}))
	defer server.Close()
	scope, err := credentialScopeForAPIBase(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	manager = credentialauth.Manager{
		Store: store, Region: string(config.RegionCN), ProfileID: config.DefaultProfileName,
		ProfileName: config.DefaultProfileName, Scope: scope,
	}
	if err := manager.Save(credentialauth.Credential{AccessToken: skillPurchaseAccessToken, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	exit, envelope, _ := executeSkillTrialCommand(t, server, t.TempDir(), store,
		"skill", "install", downloadableProductID, "--purchase", "--wait", "0")
	errorBody, _ := envelope["error"].(map[string]any)
	if exit == 0 || errorBody["code"] != "SKILL_PURCHASE_REQUIRED" {
		t.Fatalf("credential switch changed the active purchase: exit=%d envelope=%#v", exit, envelope)
	}
}

func TestSkillAccessFailureDoesNotFallBackToTrial(t *testing.T) {
	for _, command := range []string{"access", "install", "use"} {
		for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusServiceUnavailable} {
			t.Run(command+"/"+strconv.Itoa(status), func(t *testing.T) {
				t.Setenv(processAccessTokenEnvironment, skillPurchaseAccessToken)
				state := newSkillTrialTestServer(t)
				defer state.server.Close()
				var writes atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/v1/cli/skills/"+downloadableProductID+"/access" {
						w.WriteHeader(status)
						return
					}
					if r.Method == http.MethodPost {
						writes.Add(1)
					}
					state.serveHTTP(w, r)
				}))
				defer server.Close()
				store := securestore.NewMemory()
				if err := store.Set(skillTrialStoreKey(downloadableProductID), `{"installId":"11111111-1111-4111-8111-111111111111","secret":"existing-secret"}`); err != nil {
					t.Fatal(err)
				}
				exit, envelope, _ := executeSkillTrialCommand(t, server, t.TempDir(), store,
					"skill", command, downloadableProductID)
				if exit == 0 || envelope["ok"] != false || writes.Load() != 0 {
					t.Fatalf("failed ownership lookup entered trial/purchase: exit=%d writes=%d envelope=%#v", exit, writes.Load(), envelope)
				}
			})
		}
	}
}

func TestExplicitSkillPurchaseSkipsAvailableTrial(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, skillPurchaseAccessToken)
	state := newSkillPurchaseTestServer(t)
	defer state.server.Close()
	var trialCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/skills/" + downloadableProductID + "/access", "/v1/cli/skills/" + downloadableProductID + "/access":
			state.mu.Lock()
			owned := r.URL.Path == "/v1/cli/skills/"+downloadableProductID+"/access" && state.paymentStatus == "PAID"
			state.mu.Unlock()
			access := skillAccessFixture(false, owned, state.archiveDigest, state.server.URL+"/purchase")
			access["trial"] = map[string]any{"available": true, "limitUses": 2}
			writeJSONResponse(w, access)
			return
		case "/v1/skills/" + downloadableProductID + "/trial-grants":
			trialCalls.Add(1)
		}
		state.serveHTTP(w, r)
	}))
	defer server.Close()
	home := t.TempDir()
	exit, envelope, _ := executeSkillPurchaseCommand(t, server, home,
		"skill", "install", downloadableProductID, "--purchase", "--wait", "0")
	errorBody, _ := envelope["error"].(map[string]any)
	if exit == 0 || errorBody["code"] != "SKILL_PURCHASE_REQUIRED" || trialCalls.Load() != 0 {
		t.Fatalf("direct purchase did not present an order: exit=%d trials=%d envelope=%#v", exit, trialCalls.Load(), envelope)
	}
	details, _ := errorBody["details"].(map[string]any)
	if details["orderNo"] != skillPurchaseOrderNo || details["paymentUrl"] == "" || details["paymentPresentation"] == nil {
		t.Fatalf("direct purchase lost payment recovery details: %#v", details)
	}
	// A fresh invocation must resume this order through payment and installation.
	exit, envelope, _ = executeSkillPurchaseCommand(t, server, home,
		"skill", "install", downloadableProductID, "--agent", "agents", "--purchase", "--wait", "10m")
	if exit != 0 || envelope["ok"] != true || state.orderCreates != 1 || state.orderAttempts != 1 || trialCalls.Load() != 0 {
		t.Fatalf("direct purchase did not resume into one paid install: exit=%d envelope=%#v creates=%d attempts=%d trials=%d", exit, envelope, state.orderCreates, state.orderAttempts, trialCalls.Load())
	}
	// Keeping the flag after settlement must not require new purchase scopes.
	state.scopes = []string{"skill-use:read"}
	exit, envelope, _ = executeSkillPurchaseCommand(t, server, home,
		"skill", "install", downloadableProductID, "--agent", "agents", "--purchase", "--wait", "0")
	if exit != 0 || envelope["ok"] != true || state.orderAttempts != 1 {
		t.Fatalf("owned --purchase retried buying: exit=%d envelope=%#v attempts=%d", exit, envelope, state.orderAttempts)
	}
}

func TestUnreadableSkillLoginDoesNotBecomeAnonymousTrial(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	for _, command := range []string{"access", "install", "use"} {
		t.Run(command, func(t *testing.T) {
			state := newSkillTrialTestServer(t)
			defer state.server.Close()
			exit, envelope, _ := executeSkillTrialCommand(t, state.server, t.TempDir(), unavailableCredentialStore{},
				"skill", command, downloadableProductID)
			errorBody, _ := envelope["error"].(map[string]any)
			if exit == 0 || errorBody["code"] != "credential_store_unavailable" {
				t.Fatalf("unreadable login was treated as anonymous: exit=%d envelope=%#v", exit, envelope)
			}
		})
	}
}
