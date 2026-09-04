package command

import (
	"archive/zip"
	"bytes"
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

const (
	pageTestToken      = "vme_cli_1234567890123456789012345678901234567890123"
	pageTestMerchantID = "11111111-1111-4111-8111-111111111111"
	pageTestReleaseID  = "22222222-2222-4222-8222-222222222222"
)

func TestMerchantPagePreviewUsesScopedAuthAndPresignedUpload(t *testing.T) {
	var mu sync.Mutex
	var requests []string
	var uploaded []byte
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		requests = append(requests, request.Method+" "+request.URL.Path)
		mu.Unlock()
		switch request.URL.Path {
		case "/v1/cli/auth/status":
			writeJSONResponse(writer, map[string]any{
				"authenticated": true,
				"user":          map[string]any{"id": "33333333-3333-4333-8333-333333333333", "displayName": "Creator", "avatarUrl": nil},
				"scopes":        []string{"merchant-commerce:read", "merchant-commerce:write"}, "expiresAt": "2027-08-21T00:00:00Z",
			})
		case "/v1/cli/merchant/accounts":
			writeJSONResponse(writer, map[string]any{"items": []any{map[string]any{
				"id": pageTestMerchantID, "creatorAccountId": "44444444-4444-4444-8444-444444444444",
				"displayName": "Creator", "status": "ACTIVE", "ownershipStatus": "OWNED", "statusVersion": 1,
			}}})
		case "/v1/cli/merchant/page-customizations/drafts":
			body, _ := io.ReadAll(request.Body)
			if !strings.Contains(string(body), `"contractVersion":"2026-09-06"`) || !strings.Contains(string(body), `"creatorHandle":"alice-maker"`) {
				t.Fatalf("unexpected draft body: %s", body)
			}
			writeJSONResponse(writer, map[string]any{"release": pageTestRelease("UPLOADING")})
		case "/v1/cli/merchant/page-customizations/releases/" + pageTestReleaseID + "/upload-authorizations":
			writeJSONResponse(writer, map[string]any{
				"uploadUrl": server.URL + "/upload/page.zip", "expiresAt": "2027-08-21T00:15:00Z",
				"headers": map[string]string{"content-type": "application/zip", "if-none-match": "*"},
			})
		case "/upload/page.zip":
			if request.Method != http.MethodPut || request.Header.Get("Authorization") != "" || request.Header.Get("If-None-Match") != "*" {
				t.Fatalf("unsafe presigned upload request: method=%s headers=%v", request.Method, request.Header)
			}
			uploaded, _ = io.ReadAll(request.Body)
			writer.WriteHeader(http.StatusNoContent)
		case "/v1/cli/merchant/page-customizations/releases/" + pageTestReleaseID + "/complete-upload":
			writeJSONResponse(writer, pageTestRelease("VALIDATED"))
		case "/v1/cli/merchant/page-customizations/releases/" + pageTestReleaseID + "/previews":
			writeJSONResponse(writer, map[string]any{
				"releaseId": pageTestReleaseID,
				"url":       "https://viceme.cn/alice-maker?customPreview=preview-token",
				"expiresAt": "2027-08-21T00:15:00Z",
			})
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
		}
	}))
	defer server.Close()
	t.Setenv(processAccessTokenEnvironment, pageTestToken)

	root := t.TempDir()
	pageZIP := writeCommandPageZIP(t, root)
	var stdout, stderr bytes.Buffer
	exit := Execute([]string{
		"merchant", "page", "preview", "--path", pageZIP,
		"--target", "https://viceme.cn/alice-maker", "--merchant", pageTestMerchantID,
	}, Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: securestore.NewMemory(), HTTPClient: server.Client(),
		APIBaseURL: server.URL, Region: config.RegionCN, NewID: func() string { return "55555555-5555-4555-8555-555555555555" },
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
	})
	if exit != 0 {
		t.Fatalf("preview failed: exit=%d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	if len(uploaded) == 0 || strings.Contains(stdout.String(), pageTestToken) || strings.Contains(stderr.String(), pageTestToken) {
		t.Fatalf("upload or credential boundary failed: uploaded=%d stdout=%s stderr=%s", len(uploaded), stdout.String(), stderr.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || envelope["ok"] != true {
		t.Fatalf("invalid output envelope: %#v err=%v", envelope, err)
	}
	mu.Lock()
	defer mu.Unlock()
	want := []string{
		"GET /v1/cli/auth/status",
		"GET /v1/cli/merchant/accounts",
		"POST /v1/cli/merchant/page-customizations/drafts",
		"POST /v1/cli/merchant/page-customizations/releases/" + pageTestReleaseID + "/upload-authorizations",
		"PUT /upload/page.zip",
		"POST /v1/cli/merchant/page-customizations/releases/" + pageTestReleaseID + "/complete-upload",
		"POST /v1/cli/merchant/page-customizations/releases/" + pageTestReleaseID + "/previews",
	}
	if strings.Join(requests, "\n") != strings.Join(want, "\n") {
		t.Fatalf("request sequence mismatch:\n%v\nwant:\n%v", requests, want)
	}
}

func TestMerchantPageUploadUsesPendingCreatorTenantWithoutOnlinePreview(t *testing.T) {
	var mu sync.Mutex
	var requests []string
	var uploaded []byte
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		requests = append(requests, request.Method+" "+request.URL.Path)
		mu.Unlock()
		switch request.URL.Path {
		case "/v1/cli/auth/status":
			writeJSONResponse(writer, map[string]any{
				"authenticated": true,
				"user":          map[string]any{"id": "33333333-3333-4333-8333-333333333333", "displayName": "Creator", "avatarUrl": nil},
				"scopes":        []string{"merchant-commerce:read", "merchant-commerce:write"}, "expiresAt": "2027-08-21T00:00:00Z",
			})
		case "/v1/cli/merchant/accounts":
			writeJSONResponse(writer, map[string]any{"items": []any{map[string]any{
				"id": pageTestMerchantID, "creatorAccountId": "44444444-4444-4444-8444-444444444444",
				"displayName": "Creator", "status": "SUSPENDED", "ownershipStatus": "OWNED", "statusVersion": 1,
			}}})
		case "/v1/cli/merchant/onboarding/current":
			writeJSONResponse(writer, map[string]any{
				"onboarding": map[string]any{
					"id": "77777777-7777-4777-8777-777777777777", "kind": "APPLICATION",
					"merchantAccountId": pageTestMerchantID, "provider": nil, "requestedHandle": "alice-maker",
					"displayName": "Creator", "status": "SUBMITTED", "lockVersion": 0,
					"publicAccountName": nil, "profileUrl": nil, "reservationExpiresAt": nil,
					"reasonCode": nil, "reviewNote": nil, "submittedAt": "2027-08-21T00:00:00Z", "reviewedAt": nil,
					"evidence": []any{},
				},
				"merchant": map[string]any{
					"id": pageTestMerchantID, "creatorAccountId": "44444444-4444-4444-8444-444444444444",
					"displayName": "Creator", "status": "SUSPENDED", "ownershipStatus": "OWNED", "statusVersion": 1,
				},
				"creatorIdentity": map[string]any{
					"handle": "alice-maker", "displayName": "Creator", "status": "DRAFT",
					"profilePath": "/alice-maker", "markdownPath": "/alice-maker.md",
					"suggestedHandle": nil, "profileUrl": "https://viceme.cn/alice-maker", "markdownUrl": "https://viceme.cn/alice-maker.md",
				},
				"nextAction": "WAIT_FOR_REVIEW",
			})
		case "/v1/cli/merchant/page-customizations/drafts":
			writeJSONResponse(writer, map[string]any{"release": pageTestRelease("UPLOADING")})
		case "/v1/cli/merchant/page-customizations/releases/" + pageTestReleaseID + "/upload-authorizations":
			writeJSONResponse(writer, map[string]any{
				"uploadUrl": server.URL + "/upload/page.zip", "expiresAt": "2027-08-21T00:15:00Z",
				"headers": map[string]string{"content-type": "application/zip", "if-none-match": "*"},
			})
		case "/upload/page.zip":
			uploaded, _ = io.ReadAll(request.Body)
			writer.WriteHeader(http.StatusNoContent)
		case "/v1/cli/merchant/page-customizations/releases/" + pageTestReleaseID + "/complete-upload":
			writeJSONResponse(writer, pageTestRelease("VALIDATED"))
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
		}
	}))
	defer server.Close()
	t.Setenv(processAccessTokenEnvironment, pageTestToken)

	root := t.TempDir()
	pageZIP := writeCommandPageZIP(t, root)
	var stdout, stderr bytes.Buffer
	exit := Execute([]string{
		"merchant", "page", "upload", "--path", pageZIP,
		"--target", "https://viceme.cn/alice-maker", "--merchant", pageTestMerchantID,
	}, Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: securestore.NewMemory(), HTTPClient: server.Client(),
		APIBaseURL: server.URL, Region: config.RegionCN, NewID: func() string { return "55555555-5555-4555-8555-555555555555" },
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
	})
	if exit != 0 || len(uploaded) == 0 || !strings.Contains(stdout.String(), `"status": "VALIDATED"`) {
		t.Fatalf("pending upload failed: exit=%d uploaded=%d stdout=%s stderr=%s", exit, len(uploaded), stdout.String(), stderr.String())
	}
	mu.Lock()
	defer mu.Unlock()
	want := []string{
		"GET /v1/cli/auth/status",
		"GET /v1/cli/merchant/accounts",
		"GET /v1/cli/merchant/onboarding/current",
		"POST /v1/cli/merchant/page-customizations/drafts",
		"POST /v1/cli/merchant/page-customizations/releases/" + pageTestReleaseID + "/upload-authorizations",
		"PUT /upload/page.zip",
		"POST /v1/cli/merchant/page-customizations/releases/" + pageTestReleaseID + "/complete-upload",
	}
	if strings.Join(requests, "\n") != strings.Join(want, "\n") {
		t.Fatalf("request sequence mismatch:\n%v\nwant:\n%v", requests, want)
	}
}

func TestMerchantPageDescribeReturnsTargetSpecificCapabilities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/cli/auth/status":
			writeJSONResponse(writer, map[string]any{
				"authenticated": true,
				"user":          map[string]any{"id": "33333333-3333-4333-8333-333333333333", "displayName": "Creator", "avatarUrl": nil},
				"scopes":        []string{"merchant-commerce:read"}, "expiresAt": "2027-08-21T00:00:00Z",
			})
		case "/v1/cli/merchant/accounts":
			writeJSONResponse(writer, map[string]any{"items": []any{map[string]any{
				"id": pageTestMerchantID, "creatorAccountId": "44444444-4444-4444-8444-444444444444",
				"displayName": "Creator", "status": "ACTIVE", "ownershipStatus": "OWNED", "statusVersion": 1,
			}}})
		case "/v1/cli/merchant/page-customizations/describe":
			if request.URL.Query().Get("targetType") != "WORK" || request.URL.Query().Get("creatorHandle") != "alice-maker" || request.URL.Query().Get("workSlug") != "writing-skill" {
				t.Fatalf("unexpected target query: %s", request.URL.RawQuery)
			}
			writeJSONResponse(writer, map[string]any{
				"target":       map[string]any{"type": "WORK", "creatorHandle": "alice-maker", "workSlug": "writing-skill"},
				"manifestKind": "WorkPage", "sdkVersion": "1", "contextSchema": "viceme.work-page-context/v1",
				"capabilityGroups": []any{map[string]any{
					"category": "SOCIAL",
					"capabilities": []any{map[string]any{
						"capability": "work.like", "targets": []string{"WORK"},
						"methods": []any{map[string]any{"method": "work.like.set", "call": "work.setLiked", "access": "USER", "effect": "WRITE"}},
					}},
				}},
			})
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
		}
	}))
	defer server.Close()
	t.Setenv(processAccessTokenEnvironment, pageTestToken)

	root := t.TempDir()
	var stdout bytes.Buffer
	exit := Execute([]string{
		"merchant", "page", "describe", "--target", "https://viceme.cn/alice-maker/writing-skill", "--merchant", pageTestMerchantID,
	}, Dependencies{
		Out: &stdout, Store: securestore.NewMemory(), HTTPClient: server.Client(), APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
	})
	if exit != 0 || !strings.Contains(stdout.String(), `"work.like"`) {
		t.Fatalf("describe failed: exit=%d output=%s", exit, stdout.String())
	}
}

func TestMerchantPageRejectsTargetMismatchBeforeNetwork(t *testing.T) {
	root := t.TempDir()
	pageZIP := writeCommandPageZIP(t, root)
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	var stdout bytes.Buffer
	exit := Execute([]string{
		"merchant", "page", "preview", "--path", pageZIP,
		"--target", "https://viceme.cn/alice-maker/writing-skill",
	}, Dependencies{
		Out: &stdout, Store: securestore.NewMemory(), HTTPClient: server.Client(), APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
	})
	if exit == 0 || calls != 0 || !strings.Contains(stdout.String(), "PAGE_MANIFEST_TARGET_MISMATCH") {
		t.Fatalf("target mismatch crossed the network: exit=%d calls=%d output=%s", exit, calls, stdout.String())
	}
}

func TestParsePageTargetURLMatchesPublicRouteContracts(t *testing.T) {
	tests := []string{
		"https://viceme.cn/creator",
		"https://viceme.cn/alice-maker/about",
		"https://viceme.cn/alice-maker/" + strings.Repeat("a", 65),
		"https://viceme.cn/alice-maker?preview=token",
		"http://viceme.cn/alice-maker",
	}
	for _, value := range tests {
		if _, err := parsePageTargetURL(value); err == nil {
			t.Fatalf("accepted invalid public page target %q", value)
		}
	}
	creator, err := parsePageTargetURL("https://viceme.cn/alice-maker")
	if err != nil || creator.Type != "CREATOR" {
		t.Fatalf("creator target mismatch: %#v err=%v", creator, err)
	}
	work, err := parsePageTargetURL("https://viceme.cn/alice-maker/writing-skill")
	if err != nil || work.Type != "WORK" || work.WorkSlug != "writing-skill" {
		t.Fatalf("Work target mismatch: %#v err=%v", work, err)
	}
}

func writeCommandPageZIP(t *testing.T, root string) string {
	t.Helper()
	filename := filepath.Join(root, "creator-page.zip")
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	entries := map[string]string{
		"viceme-page.json": `{"apiVersion":"page.viceme.ai/v1alpha1","kind":"CreatorPage","metadata":{"name":"Creator page"},"spec":{"entry":"dist/index.html","sdkVersion":"1","capabilities":["context.read"]}}`,
		"dist/index.html":  "<!doctype html><html><head></head><body>Creator</body></html>",
	}
	for name, value := range entries {
		entry, createErr := archive.Create(name)
		if createErr != nil {
			t.Fatal(createErr)
		}
		_, _ = entry.Write([]byte(value))
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return filename
}

func pageTestRelease(status string) map[string]any {
	manifest := any(nil)
	if status != "UPLOADING" {
		manifest = map[string]any{
			"apiVersion": "page.viceme.ai/v1alpha1", "kind": "CreatorPage", "metadata": map[string]any{"name": "Creator page"},
			"spec": map[string]any{"entry": "dist/index.html", "sdkVersion": "1", "capabilities": []string{"context.read"}},
		}
	}
	return map[string]any{
		"id": pageTestReleaseID, "customizationId": "66666666-6666-4666-8666-666666666666", "version": 1, "status": status,
		"target":   map[string]any{"type": "CREATOR", "creatorHandle": "alice-maker"},
		"artifact": map[string]any{"digest": strings.Repeat("a", 64), "sizeBytes": 128, "fileName": "creator-page.zip", "contentType": "application/zip"},
		"manifest": manifest, "validationIssues": []string{}, "createdAt": "2027-08-21T00:00:00Z",
		"uploadedAt": nil, "validatedAt": nil, "publishedAt": nil,
	}
}
