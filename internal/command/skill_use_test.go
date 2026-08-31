package command

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

const downloadableProductID = "11111111-1111-4111-8111-111111111111"
const downloadableReleaseID = "22222222-2222-4222-8222-222222222222"

func TestFreeSkillInstallIsAnonymousAndVerifiesTheArtifact(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	archive := downloadableSkillArchive(t)
	digest := fmt.Sprintf("%x", sha256.Sum256(archive))
	var authCalls atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/skills/" + downloadableProductID + "/access":
			writeJSONResponse(writer, skillAccessFixture(true, false, digest, server.URL+"/purchase"))
		case "/v1/downloads/free/" + downloadableProductID:
			writeJSONResponse(writer, map[string]any{
				"url": server.URL + "/artifact", "fileName": "free.zip", "releaseId": downloadableReleaseID, "artifactDigest": digest, "expiresAt": "2027-08-27T00:00:00Z",
			})
		case "/artifact":
			writer.Header().Set("Content-Type", "application/zip")
			_, _ = writer.Write(archive)
		case "/v1/cli/auth/status":
			authCalls.Add(1)
			writer.WriteHeader(http.StatusUnauthorized)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	home := t.TempDir()
	exit, envelope := executeSkillUseCommand(t, server, home,
		"skill", "install", downloadableProductID, "--agent", "codex",
	)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("anonymous free install failed: exit=%d envelope=%#v", exit, envelope)
	}
	if authCalls.Load() != 0 {
		t.Fatalf("free install unexpectedly checked login %d times", authCalls.Load())
	}
	stableName := "free-test"
	for _, filename := range []string{
		filepath.Join(home, ".codex", "skills", stableName, "SKILL.md"),
		filepath.Join(home, ".agents", "skills", stableName, "SKILL.md"),
	} {
		content, err := os.ReadFile(filename)
		if err != nil || !bytes.Contains(content, []byte("Free Test Skill")) {
			t.Fatalf("installed Skill %s is invalid: %q, %v", filename, content, err)
		}
	}
	executable := filepath.Join(home, ".codex", "skills", stableName, "scripts", "run.sh")
	info, err := os.Stat(executable)
	if err != nil {
		t.Fatalf("installed executable is missing: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o755 {
		t.Fatalf("installed executable mode was not preserved: mode=%v err=%v", info, err)
	}
}

func TestPaidSkillInstallRequiresLoginBeforeEntitlementLookup(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	var cliAccessCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/skills/" + downloadableProductID + "/access":
			writeJSONResponse(writer, skillAccessFixture(false, false, "a1", "https://shop.example.test/purchase"))
		case "/v1/cli/skills/" + downloadableProductID + "/access":
			cliAccessCalls.Add(1)
			writer.WriteHeader(http.StatusUnauthorized)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	exit, envelope := executeSkillUseCommand(t, server, t.TempDir(),
		"skill", "install", downloadableProductID, "--agent", "agents",
	)
	if exit == 0 || envelope["ok"] != false {
		t.Fatalf("paid anonymous install unexpectedly succeeded: exit=%d envelope=%#v", exit, envelope)
	}
	errorBody := envelope["error"].(map[string]any)
	if errorBody["code"] != "NOT_LOGGED_IN" || cliAccessCalls.Load() != 0 {
		t.Fatalf("paid install did not stop at login boundary: %#v calls=%d", errorBody, cliAccessCalls.Load())
	}
}

func TestCanonicalWorkURLSelectsTheFreeEditionByDefault(t *testing.T) {
	const paidProductID = "44444444-4444-4444-8444-444444444444"
	const transactionalProductID = "66666666-6666-4666-8666-666666666666"
	var requestedAccessPath string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/public/creators/creator/works/example-skill":
			writeJSONResponse(writer, map[string]any{
				"creator": map[string]any{"handle": "creator"},
				"work": map[string]any{
					"canonicalPath": "/creator/example-skill",
					"products": []any{
						map[string]any{"id": transactionalProductID, "minimumPriceCents": 0, "isFree": false, "installKind": nil, "activeRelease": nil, "edition": nil},
						map[string]any{"id": paidProductID, "currency": "CNY", "minimumPriceCents": 300, "maximumPriceCents": 300, "isFree": false, "installKind": "PURCHASE_REQUIRED", "activeRelease": map[string]any{"id": downloadableReleaseID, "artifactDigest": strings.Repeat("b", 64), "fileName": "pro.zip"}, "edition": map[string]any{"key": "pro", "title": "Pro", "sortOrder": 0, "highlights": []string{"Advanced workflow", "Priority templates"}}},
						map[string]any{"id": downloadableProductID, "currency": "CNY", "minimumPriceCents": 0, "maximumPriceCents": 0, "isFree": true, "installKind": "PUBLIC_FREE", "activeRelease": map[string]any{"id": downloadableReleaseID, "artifactDigest": strings.Repeat("a", 64), "fileName": "free.zip"}, "edition": map[string]any{"key": "free", "title": "Free", "sortOrder": 2, "highlights": []string{"Core workflow"}}},
					},
				},
			})
		case "/v1/skills/" + downloadableProductID + "/access":
			requestedAccessPath = request.URL.Path
			writeJSONResponse(writer, skillAccessFixture(true, false, "a1", ""))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	exit, envelope := executeSkillUseCommand(t, server, t.TempDir(),
		"skill", "access", server.URL+"/creator/example-skill",
	)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("canonical Work URL did not resolve: exit=%d envelope=%#v", exit, envelope)
	}
	if requestedAccessPath != "/v1/skills/"+downloadableProductID+"/access" {
		t.Fatalf("canonical Work URL selected the wrong edition: %q", requestedAccessPath)
	}
	detailExit, detail := executeSkillUseCommand(t, server, t.TempDir(),
		"skill", "detail", server.URL+"/creator/example-skill",
	)
	if detailExit != 0 || detail["ok"] != true {
		t.Fatalf("canonical Work detail failed: exit=%d envelope=%#v", detailExit, detail)
	}
	products := detail["data"].(map[string]any)["work"].(map[string]any)["products"].([]any)
	paid := products[1].(map[string]any)
	if paid["currency"] != "CNY" || paid["maximumPriceCents"] != float64(300) {
		t.Fatalf("detail lost edition price fields: %#v", paid)
	}
	highlights := paid["edition"].(map[string]any)["highlights"].([]any)
	if len(highlights) != 2 {
		t.Fatalf("detail lost edition highlights: %#v", paid)
	}
	for name, targetCode := range map[string]string{
		"malformed":       "SKILL_EDITION_SELECTOR_INVALID",
		"foreign":         "SKILL_EDITION_NOT_IN_WORK",
		"not-installable": "SKILL_EDITION_NOT_INSTALLABLE",
	} {
		t.Run(name, func(t *testing.T) {
			selector := "not-a-product"
			if name == "foreign" {
				selector = "55555555-5555-4555-8555-555555555555"
			} else if name == "not-installable" {
				selector = transactionalProductID
			}
			exit, failure := executeSkillUseCommand(t, server, t.TempDir(),
				"skill", "access", server.URL+"/creator/example-skill?product="+selector,
			)
			if exit == 0 || failure["ok"] != false {
				t.Fatalf("invalid explicit selector unexpectedly fell back: %#v", failure)
			}
			errorBody, _ := failure["error"].(map[string]any)
			if errorBody["code"] != targetCode {
				t.Fatalf("invalid explicit selector returned %#v, want %s", errorBody, targetCode)
			}
		})
	}
}

func TestOwnedPaidSkillReinstallsWithoutAnotherPurchase(t *testing.T) {
	const accessToken = "vme_cli_1234567890123456789012345678901234567890123"
	t.Setenv(processAccessTokenEnvironment, accessToken)
	archive := downloadableSkillArchive(t)
	digest := fmt.Sprintf("%x", sha256.Sum256(archive))
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/skills/"+downloadableProductID+"/access" && request.URL.Path != "/artifact" &&
			request.Header.Get("Authorization") != "Bearer "+accessToken {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/v1/skills/" + downloadableProductID + "/access":
			writeJSONResponse(writer, skillAccessFixture(false, false, digest, server.URL+"/purchase"))
		case "/v1/cli/auth/status":
			writeJSONResponse(writer, map[string]any{
				"authenticated": true,
				"user":          map[string]any{"id": "33333333-3333-4333-8333-333333333333", "displayName": "Buyer", "avatarUrl": nil},
				"scopes":        []string{"profile:read", "skill-use:read"}, "expiresAt": "2027-08-27T00:00:00Z",
			})
		case "/v1/cli/skills/" + downloadableProductID + "/access":
			writeJSONResponse(writer, skillAccessFixture(false, true, digest, server.URL+"/purchase"))
		case "/v1/cli/skills/" + downloadableProductID + "/download":
			writeJSONResponse(writer, map[string]any{
				"url": server.URL + "/artifact", "fileName": "paid.zip", "releaseId": downloadableReleaseID, "artifactDigest": digest, "expiresAt": "2027-08-27T00:00:00Z",
			})
		case "/artifact":
			writer.Header().Set("Content-Type", "application/zip")
			_, _ = writer.Write(archive)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	home := t.TempDir()
	exit, envelope := executeSkillUseCommand(t, server, home,
		"skill", "install", downloadableProductID, "--agent", "agents",
	)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("owned paid reinstall failed: exit=%d envelope=%#v", exit, envelope)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "free-test", "SKILL.md")); err != nil {
		t.Fatalf("owned paid Skill was not installed: %v", err)
	}
}

func executeSkillUseCommand(t *testing.T, server *httptest.Server, home string, arguments ...string) (int, map[string]any) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	exit := Execute(arguments, Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: securestore.NewMemory(),
		HTTPClient: server.Client(), APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{
			Home: home, CodexHome: filepath.Join(home, ".codex"), ConfigDir: filepath.Join(home, ".viceme-cli"),
		},
	})
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid envelope: exit=%d stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
	}
	return exit, envelope
}

func skillAccessFixture(free, owned bool, digest, purchaseURL string) map[string]any {
	editionKey, editionTitle := "pro", "Pro"
	installKind := "PURCHASE_REQUIRED"
	purchaseAvailable := true
	var resolvedPurchaseURL any = purchaseURL
	if free {
		editionKey, editionTitle = "free", "Free"
		installKind = "PUBLIC_FREE"
		purchaseAvailable = false
		resolvedPurchaseURL = nil
	} else if owned {
		installKind = "OWNED_PAID"
		purchaseAvailable = false
		resolvedPurchaseURL = nil
	}
	return map[string]any{
		"productId": downloadableProductID, "isFree": free, "owned": owned || free,
		"downloadAvailable": owned || free, "installKind": installKind,
		"purchaseAvailable": purchaseAvailable, "purchaseUrl": resolvedPurchaseURL,
		"unavailableReason": nil,
		"edition":           map[string]any{"key": editionKey, "title": editionTitle, "sortOrder": 0, "highlights": []string{"Try the core workflow"}},
		"release":           map[string]any{"id": downloadableReleaseID, "artifactDigest": digest, "fileName": "skill.zip"},
	}
}

func TestUnclaimedMerchantPaidSkillFailsBeforeLoginOrWait(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/skills/"+downloadableProductID+"/access" {
			http.NotFound(writer, request)
			return
		}
		response := skillAccessFixture(false, false, "a1", "")
		response["installKind"] = "PURCHASE_UNAVAILABLE"
		response["purchaseAvailable"] = false
		response["purchaseUrl"] = nil
		response["unavailableReason"] = "MERCHANT_NOT_CLAIMED"
		writeJSONResponse(writer, response)
	}))
	defer server.Close()

	exit, envelope := executeSkillUseCommand(t, server, t.TempDir(),
		"skill", "install", downloadableProductID, "--wait", "10s",
	)
	if exit == 0 || envelope["ok"] != false {
		t.Fatalf("unclaimed paid edition unexpectedly entered purchase flow: %#v", envelope)
	}
	errorBody, _ := envelope["error"].(map[string]any)
	if errorBody["code"] != "SKILL_PURCHASE_UNAVAILABLE" {
		t.Fatalf("unclaimed paid edition returned %#v", errorBody)
	}
}

func downloadableSkillArchive(t *testing.T) []byte {
	t.Helper()
	var body bytes.Buffer
	writer := zip.NewWriter(&body)
	entry, err := writer.Create("SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("---\nname: free-test\ndescription: Free Test Skill\n---\n\n# Free Test Skill\n")); err != nil {
		t.Fatal(err)
	}
	header := &zip.FileHeader{Name: "scripts/run.sh", Method: zip.Deflate}
	header.SetMode(0o755)
	script, err := writer.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := script.Write([]byte("#!/bin/sh\necho ok\n")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}
