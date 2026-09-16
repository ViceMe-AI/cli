package command

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestProfilePageUploadAndPublishNeverRequireMerchantOrClientSelectedTarget(t *testing.T) {
	var mu sync.Mutex
	var requests []string
	var draftBody, publishBody string
	var uploadedPage, uploadedSource []byte
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		requests = append(requests, request.Method+" "+request.URL.Path)
		mu.Unlock()
		switch request.URL.Path {
		case "/v1/cli/auth/status":
			writeJSONResponse(writer, map[string]any{
				"authenticated": true,
				"user": map[string]any{
					"id": "33333333-3333-4333-8333-333333333333", "displayName": "Member", "avatarUrl": nil,
				},
				"scopes": []string{"profile:read", "profile:write"}, "expiresAt": "2027-09-12T00:00:00Z",
			})
		case "/v1/cli/profile/page-customizations/describe":
			writeJSONResponse(writer, profilePageDescription())
		case "/v1/cli/profile/page-customizations/drafts":
			body, _ := io.ReadAll(request.Body)
			draftBody = string(body)
			writeJSONResponse(writer, map[string]any{"release": pageTestRelease("UPLOADING")})
		case "/v1/cli/profile/page-customizations/releases/" + pageTestReleaseID + "/upload-authorizations":
			writeJSONResponse(writer, map[string]any{
				"uploadUrl": server.URL + "/upload/profile.zip", "expiresAt": "2027-09-12T00:15:00Z",
				"headers": map[string]string{"content-type": "application/zip", "if-none-match": "*"},
			})
		case "/upload/profile.zip":
			uploadedPage, _ = io.ReadAll(request.Body)
			writer.WriteHeader(http.StatusNoContent)
		case "/v1/cli/profile/page-customizations/releases/" + pageTestReleaseID + "/source-upload-authorizations":
			writeJSONResponse(writer, map[string]any{
				"uploadUrl": server.URL + "/upload/profile-source.zip", "expiresAt": "2027-09-12T00:15:00Z",
				"headers": map[string]string{"content-type": "application/zip", "if-none-match": "*"},
			})
		case "/upload/profile-source.zip":
			uploadedSource, _ = io.ReadAll(request.Body)
			writer.WriteHeader(http.StatusNoContent)
		case "/v1/cli/profile/page-customizations/releases/" + pageTestReleaseID + "/complete-upload":
			writeJSONResponse(writer, pageTestRelease("VALIDATED"))
		case "/v1/cli/profile/page-customizations/releases/" + pageTestReleaseID + "/publish":
			body, _ := io.ReadAll(request.Body)
			publishBody = string(body)
			writeJSONResponse(writer, pageTestRelease("PUBLISHED"))
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
		}
	}))
	defer server.Close()
	t.Setenv(processAccessTokenEnvironment, pageTestToken)

	root := t.TempDir()
	pageZIP := writeCommandPageZIP(t, root)
	pageSource := writeCommandPageSource(t, root)
	dependencies := Dependencies{
		Store: securestore.NewMemory(), HTTPClient: server.Client(), APIBaseURL: server.URL,
		Region: config.RegionCN, NewID: func() string { return "55555555-5555-4555-8555-555555555555" },
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
	}
	run := func(arguments ...string) map[string]any {
		t.Helper()
		var stdout, stderr bytes.Buffer
		dependencies.Out = &stdout
		dependencies.ErrOut = &stderr
		if exit := Execute(arguments, dependencies); exit != 0 {
			t.Fatalf("command failed: exit=%d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
		}
		var envelope map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || envelope["ok"] != true {
			t.Fatalf("invalid JSON envelope: result=%#v err=%v", envelope, err)
		}
		return envelope
	}

	upload := run("profile", "page", "upload", "--path", pageZIP, "--source", pageSource)
	if len(uploadedPage) == 0 || len(uploadedSource) == 0 || !strings.Contains(draftBody, `"contractVersion":"2026-09-12"`) || strings.Contains(draftBody, "merchantAccountId") || strings.Contains(draftBody, `"target"`) {
		t.Fatalf("personal upload crossed authority boundary: page=%d source=%d body=%s", len(uploadedPage), len(uploadedSource), draftBody)
	}
	uploadData := upload["data"].(map[string]any)
	if uploadData["profileUrl"] != "https://viceme.cn/alice-maker" {
		t.Fatalf("upload omitted the server-authoritative profile URL: %#v", uploadData)
	}
	published := run(
		"profile", "page", "publish", pageTestReleaseID,
		"--expected-active", "none", "--expected-concurrency", strings.Repeat("a", 64),
	)
	if strings.Contains(publishBody, "merchantAccountId") || !strings.Contains(publishBody, `"expectedActiveReleaseId":null`) || !strings.Contains(publishBody, `"expectedConcurrencyToken":"`+strings.Repeat("a", 64)+`"`) {
		t.Fatalf("unexpected personal publish body: %s", publishBody)
	}
	if published["data"].(map[string]any)["profileUrl"] != "https://viceme.cn/alice-maker" {
		t.Fatalf("publish omitted the exact profile URL: %#v", published)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, request := range requests {
		if strings.Contains(request, "/merchant/") {
			t.Fatalf("personal profile command called a merchant endpoint: %s", request)
		}
	}
}

func TestProfilePageWriteReportsMissingScopeWithoutCallingThePageAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/cli/auth/status" {
			t.Fatalf("missing scope still called page API: %s", request.URL.Path)
		}
		writeJSONResponse(writer, map[string]any{
			"authenticated": true,
			"user": map[string]any{
				"id": "33333333-3333-4333-8333-333333333333", "displayName": "Member", "avatarUrl": nil,
			},
			"scopes": []string{"profile:read"}, "expiresAt": "2027-09-12T00:00:00Z",
		})
	}))
	defer server.Close()
	t.Setenv(processAccessTokenEnvironment, pageTestToken)
	root := t.TempDir()
	pageZIP := writeCommandPageZIP(t, root)
	pageSource := writeCommandPageSource(t, root)
	var stdout, stderr bytes.Buffer
	exit := Execute([]string{"profile", "page", "upload", "--path", pageZIP, "--source", pageSource}, Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: securestore.NewMemory(), HTTPClient: server.Client(),
		APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
	})
	if exit == 0 || !strings.Contains(stdout.String(), `"code": "PROFILE_PAGE_SCOPE_REQUIRED"`) || !strings.Contains(stdout.String(), `"profile:write"`) {
		t.Fatalf("missing scope was not explicit: exit=%d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
}

func TestProfilePageSourceRestoreRejectsMismatchedSize(t *testing.T) {
	archive := []byte("not-a-complete-owner-source")
	digest := sha256.Sum256(archive)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/cli/auth/status":
			writeJSONResponse(writer, map[string]any{
				"authenticated": true,
				"user": map[string]any{
					"id": "33333333-3333-4333-8333-333333333333", "displayName": "Member", "avatarUrl": nil,
				},
				"scopes": []string{"profile:read"}, "expiresAt": "2027-09-12T00:00:00Z",
			})
		case "/v1/cli/profile/page-customizations/source":
			writeJSONResponse(writer, map[string]any{
				"target": map[string]any{"type": "CREATOR", "creatorHandle": "alice-maker"}, "ownerVerified": true,
				"activeRelease":    map[string]any{"id": pageTestReleaseID, "version": 1},
				"concurrencyToken": strings.Repeat("c", 64), "availability": "RESTORABLE",
				"source": map[string]any{
					"releaseId": pageTestReleaseID, "releaseVersion": 1,
					"digest": hex.EncodeToString(digest[:]), "sizeBytes": len(archive) + 1,
					"fileName": "personal-profile-source.zip", "createdAt": "2027-09-12T00:00:00Z", "template": nil,
				},
			})
		case "/v1/cli/profile/page-customizations/releases/" + pageTestReleaseID + "/source":
			writer.Header().Set("Content-Type", "application/zip")
			_, _ = writer.Write(archive)
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
		}
	}))
	defer server.Close()
	t.Setenv(processAccessTokenEnvironment, pageTestToken)
	root := t.TempDir()
	destination := filepath.Join(root, "restored-profile")
	var stdout bytes.Buffer
	exit := Execute([]string{
		"profile", "page", "source", "restore", "--destination", destination,
	}, Dependencies{
		Out: &stdout, Store: securestore.NewMemory(), HTTPClient: server.Client(), APIBaseURL: server.URL,
		Region: config.RegionCN, Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
	})
	if exit == 0 || !strings.Contains(stdout.String(), `"code": "PAGE_SOURCE_SIZE_MISMATCH"`) {
		t.Fatalf("source size mismatch was not rejected: exit=%d output=%s", exit, stdout.String())
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("size mismatch created a restore destination: %v", err)
	}
}

func TestProfilePagePublishRejectsInvalidConcurrencyTokenBeforeNetwork(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	root := t.TempDir()
	var stdout bytes.Buffer
	exit := Execute([]string{
		"profile", "page", "publish", pageTestReleaseID,
		"--expected-active", "none", "--expected-concurrency", "short",
	}, Dependencies{
		Out: &stdout, Store: securestore.NewMemory(), HTTPClient: server.Client(), APIBaseURL: server.URL,
		Region: config.RegionCN, Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
	})
	if exit == 0 || calls != 0 || !strings.Contains(stdout.String(), `"code": "PAGE_CONCURRENCY_TOKEN_INVALID"`) {
		t.Fatalf("invalid concurrency token crossed the network: exit=%d calls=%d output=%s", exit, calls, stdout.String())
	}
}

func profilePageDescription() map[string]any {
	return map[string]any{
		"target":       map[string]any{"type": "CREATOR", "creatorHandle": "alice-maker"},
		"manifestKind": "CreatorPage", "sdkVersion": "1", "contextSchema": "viceme.creator-page-context/v1",
		"capabilityGroups": []any{}, "profileUrl": "https://viceme.cn/alice-maker", "markdownUrl": "https://viceme.cn/alice-maker.md",
	}
}
