package command

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

const (
	merchantEngagementToken         = "vme_cli_1234567890123456789012345678901234567890123"
	merchantEngagementMerchantID    = "11111111-1111-4111-8111-111111111111"
	merchantEngagementWorkID        = "22222222-2222-4222-8222-222222222222"
	merchantEngagementApplicationID = "33333333-3333-4333-8333-333333333333"
)

type merchantEngagementCommandCase struct {
	name     string
	args     []string
	write    bool
	method   string
	path     string
	query    string
	bodyJSON string
}

type capturedMerchantEngagementRequest struct {
	method string
	path   string
	query  string
	body   []byte
}

type merchantEngagementServerState struct {
	mu       sync.Mutex
	requests []capturedMerchantEngagementRequest
}

func (state *merchantEngagementServerState) append(request capturedMerchantEngagementRequest) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.requests = append(state.requests, request)
}

func (state *merchantEngagementServerState) snapshot() []capturedMerchantEngagementRequest {
	state.mu.Lock()
	defer state.mu.Unlock()
	return append([]capturedMerchantEngagementRequest(nil), state.requests...)
}

func TestMerchantEngagementCommandTreeReplacesLegacyRoots(t *testing.T) {
	rootDirectory := t.TempDir()
	root, _, err := NewRoot(Dependencies{
		Store: securestore.NewMemory(),
		Environment: skillcontent.Environment{
			Home: rootDirectory, ConfigDir: filepath.Join(rootDirectory, "config"),
		},
		Region: config.RegionCN,
	})
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{
		"merchant work website-verification create",
		"merchant work website-verification get",
		"merchant work website-verification verify",
		"merchant work website-verification revoke",
		"merchant work sdk-access create",
		"merchant work sdk-access get",
		"merchant work sdk-access list",
		"merchant work sdk-access update",
		"merchant work sdk-access disable",
		"merchant commerce-application create",
		"merchant commerce-application list",
		"merchant commerce-application get",
		"merchant commerce-application update",
		"merchant commerce-application activate",
		"merchant commerce-application suspend",
	}
	for _, path := range paths {
		command, remaining, findErr := root.Find(strings.Fields(path))
		if findErr != nil || command.CommandPath() != "viceme "+path || len(remaining) != 0 {
			t.Fatalf("command path %q was not registered: command=%q remaining=%v err=%v", path, command.CommandPath(), remaining, findErr)
		}
	}
	for _, retired := range []string{"website", "access", "creator-app"} {
		for _, command := range root.Commands() {
			if command.Name() == retired {
				t.Fatalf("retired root command %q remains registered", retired)
			}
		}
	}
}

func TestMerchantEngagementCommandsCheckScopesBeforeBusinessRequests(t *testing.T) {
	for _, testCase := range merchantEngagementCommandCases(t) {
		t.Run(testCase.name, func(t *testing.T) {
			server, state := newMerchantEngagementServer(t, []string{"merchant-commerce:read"})
			defer server.Close()
			exit, _ := executeMerchantEngagementCommand(t, server, testCase.args)
			requests := state.snapshot()
			if len(requests) == 0 || requests[0].path != "/v1/cli/auth/status" {
				t.Fatalf("scope preflight did not run first: requests=%#v", requests)
			}
			if testCase.write {
				if exit == 0 || len(requests) != 1 {
					t.Fatalf("write command ran without write scope: exit=%d requests=%#v", exit, requests)
				}
				return
			}
			if len(requests) != 2 || requests[1].path != testCase.path {
				t.Fatalf("read-scoped command required write access or missed its request: requests=%#v", requests)
			}
		})
	}
}

func TestMerchantEngagementFlagsMapToCanonicalAPIRequests(t *testing.T) {
	for _, testCase := range merchantEngagementCommandCases(t) {
		t.Run(testCase.name, func(t *testing.T) {
			server, state := newMerchantEngagementServer(t, []string{"merchant-commerce:read", "merchant-commerce:write"})
			defer server.Close()
			_, _ = executeMerchantEngagementCommand(t, server, testCase.args)
			requests := state.snapshot()
			if len(requests) != 2 || requests[0].path != "/v1/cli/auth/status" {
				t.Fatalf("unexpected request sequence: %#v", requests)
			}
			request := requests[1]
			if request.method != testCase.method || request.path != testCase.path || request.query != testCase.query {
				t.Fatalf("request target mismatch: got=%s %s?%s want=%s %s?%s", request.method, request.path, request.query, testCase.method, testCase.path, testCase.query)
			}
			assertMerchantEngagementJSONEqual(t, testCase.bodyJSON, request.body)
		})
	}
}

func TestMerchantEngagementInvalidVersionsAndFeaturesMakeNoNetworkRequests(t *testing.T) {
	directory := t.TempDir()
	invalidUpdate := filepath.Join(directory, "invalid-revision.json")
	if err := os.WriteFile(invalidUpdate, []byte(`{"merchantAccountId":"`+merchantEngagementMerchantID+`","expectedRevision":0}`), 0o600); err != nil {
		t.Fatal(err)
	}
	requests := [][]string{
		{"merchant", "work", "website-verification", "create", merchantEngagementWorkID, "--merchant", merchantEngagementMerchantID, "--expected-revision", "0"},
		{"merchant", "work", "website-verification", "verify", merchantEngagementWorkID, "--merchant", merchantEngagementMerchantID, "--expected-verification-version", "0"},
		{"merchant", "work", "website-verification", "revoke", merchantEngagementWorkID, "--merchant", merchantEngagementMerchantID, "--expected-revision", "-1"},
		{"merchant", "work", "sdk-access", "create", merchantEngagementWorkID, "--merchant", merchantEngagementMerchantID},
		{"merchant", "work", "sdk-access", "create", merchantEngagementWorkID, "--merchant", merchantEngagementMerchantID, "--feature", "unknown"},
		{"merchant", "work", "sdk-access", "update", merchantEngagementWorkID, "--merchant", merchantEngagementMerchantID, "--expected-config-version", "0", "--feature", "tip"},
		{"merchant", "work", "sdk-access", "update", merchantEngagementWorkID, "--merchant", merchantEngagementMerchantID, "--expected-config-version", "1", "--feature", "TIP"},
		{"merchant", "commerce-application", "update", merchantEngagementApplicationID, "--input", invalidUpdate},
		{"merchant", "commerce-application", "activate", merchantEngagementApplicationID, "--merchant", merchantEngagementMerchantID, "--expected-revision", "0"},
		{"merchant", "commerce-application", "suspend", merchantEngagementApplicationID, "--merchant", merchantEngagementMerchantID, "--expected-revision", "-1"},
	}
	var networkRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		networkRequests.Add(1)
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	for _, args := range requests {
		before := networkRequests.Load()
		exit, output := executeMerchantEngagementCommand(t, server, args)
		after := networkRequests.Load()
		if exit == 0 || after != before {
			t.Fatalf("invalid local input reached the network: args=%v exit=%d before=%d after=%d output=%s", args, exit, before, after, output)
		}
	}
}

func TestMerchantCommerceApplicationInputRejectsUnknownFields(t *testing.T) {
	directory := t.TempDir()
	createInput := filepath.Join(directory, "create.json")
	updateInput := filepath.Join(directory, "update.json")
	if err := os.WriteFile(createInput, []byte(`{"merchantAccountId":"`+merchantEngagementMerchantID+`","workId":"`+merchantEngagementWorkID+`","kind":"WEBSITE_WIDGET","environment":"SANDBOX","displayName":"Widget","unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(updateInput, []byte(`{"merchantAccountId":"`+merchantEngagementMerchantID+`","expectedRevision":1,"unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		name string
		args []string
	}{
		{name: "create", args: []string{"merchant", "commerce-application", "create", "--input", createInput}},
		{name: "update", args: []string{"merchant", "commerce-application", "update", merchantEngagementApplicationID, "--input", updateInput}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server, state := newMerchantEngagementServer(t, []string{"merchant-commerce:read", "merchant-commerce:write"})
			defer server.Close()
			exit, output := executeMerchantEngagementCommand(t, server, testCase.args)
			if exit == 0 || !strings.Contains(output, "COMMERCE_APPLICATION_INPUT_INVALID") {
				t.Fatalf("unknown JSON field was accepted: exit=%d output=%s", exit, output)
			}
			for _, request := range state.snapshot() {
				if request.path != "/v1/cli/auth/status" {
					t.Fatalf("unknown JSON field reached the business API: requests=%#v", state.snapshot())
				}
			}
		})
	}
}

func merchantEngagementCommandCases(t *testing.T) []merchantEngagementCommandCase {
	t.Helper()
	directory := t.TempDir()
	createInput := filepath.Join(directory, "create.json")
	updateInput := filepath.Join(directory, "update.json")
	createJSON := `{"merchantAccountId":"` + merchantEngagementMerchantID + `","workId":"` + merchantEngagementWorkID + `","kind":"WEBSITE_WIDGET","environment":"SANDBOX","displayName":"Widget","origins":["https://example.com"],"returnUrls":["https://example.com/return"]}`
	updateJSON := `{"merchantAccountId":"` + merchantEngagementMerchantID + `","expectedRevision":4,"displayName":"Updated Widget","origins":[],"returnUrls":["https://example.com/updated"]}`
	if err := os.WriteFile(createInput, []byte(createJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(updateInput, []byte(updateJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	workPath := "/v1/cli/merchant/works/" + merchantEngagementWorkID
	applicationPath := "/v1/cli/merchant/commerce-applications/" + merchantEngagementApplicationID
	merchantQuery := "merchantAccountId=" + merchantEngagementMerchantID
	return []merchantEngagementCommandCase{
		{
			name: "website-verification-create", write: true, method: http.MethodPost,
			args: []string{"merchant", "work", "website-verification", "create", merchantEngagementWorkID, "--merchant", merchantEngagementMerchantID, "--expected-revision", "3"},
			path: workPath + "/website-verifications", bodyJSON: `{"merchantAccountId":"` + merchantEngagementMerchantID + `","expectedRevision":3}`,
		},
		{
			name: "website-verification-get", method: http.MethodGet,
			args: []string{"merchant", "work", "website-verification", "get", merchantEngagementWorkID, "--merchant", merchantEngagementMerchantID},
			path: workPath + "/website-verifications/latest", query: merchantQuery,
		},
		{
			name: "website-verification-verify", write: true, method: http.MethodPost,
			args: []string{"merchant", "work", "website-verification", "verify", merchantEngagementWorkID, "--merchant", merchantEngagementMerchantID, "--expected-verification-version", "5"},
			path: workPath + "/website-verifications/verify", bodyJSON: `{"merchantAccountId":"` + merchantEngagementMerchantID + `","expectedVerificationVersion":5}`,
		},
		{
			name: "website-verification-revoke", write: true, method: http.MethodPost,
			args: []string{"merchant", "work", "website-verification", "revoke", merchantEngagementWorkID, "--merchant", merchantEngagementMerchantID, "--expected-revision", "6"},
			path: workPath + "/website-verifications/revoke", bodyJSON: `{"merchantAccountId":"` + merchantEngagementMerchantID + `","expectedRevision":6}`,
		},
		{
			name: "sdk-access-create", write: true, method: http.MethodPost,
			args: []string{"merchant", "work", "sdk-access", "create", merchantEngagementWorkID, "--merchant", merchantEngagementMerchantID, "--feature", "tip", "--feature", "danmaku", "--feature", "tip"},
			path: workPath + "/sdk-access", bodyJSON: `{"merchantAccountId":"` + merchantEngagementMerchantID + `","features":["danmaku","tip"]}`,
		},
		{
			name: "sdk-access-get", method: http.MethodGet,
			args: []string{"merchant", "work", "sdk-access", "get", merchantEngagementWorkID, "--merchant", merchantEngagementMerchantID},
			path: workPath + "/sdk-access", query: merchantQuery,
		},
		{
			name: "sdk-access-list", method: http.MethodGet,
			args: []string{"merchant", "work", "sdk-access", "list", "--merchant", merchantEngagementMerchantID},
			path: "/v1/cli/merchant/work-sdk-accesses", query: merchantQuery,
		},
		{
			name: "sdk-access-update", write: true, method: http.MethodPut,
			args: []string{"merchant", "work", "sdk-access", "update", merchantEngagementWorkID, "--merchant", merchantEngagementMerchantID, "--expected-config-version", "7", "--feature", "tip", "--feature", "danmaku"},
			path: workPath + "/sdk-access", bodyJSON: `{"merchantAccountId":"` + merchantEngagementMerchantID + `","expectedConfigVersion":7,"features":["danmaku","tip"]}`,
		},
		{
			name: "sdk-access-disable", write: true, method: http.MethodDelete,
			args: []string{"merchant", "work", "sdk-access", "disable", merchantEngagementWorkID, "--merchant", merchantEngagementMerchantID},
			path: workPath + "/sdk-access", query: merchantQuery,
		},
		{
			name: "commerce-application-create", write: true, method: http.MethodPost,
			args: []string{"merchant", "commerce-application", "create", "--input", createInput},
			path: "/v1/cli/merchant/commerce-applications", bodyJSON: createJSON,
		},
		{
			name: "commerce-application-list", method: http.MethodGet,
			args: []string{"merchant", "commerce-application", "list", "--merchant", merchantEngagementMerchantID},
			path: "/v1/cli/merchant/commerce-applications", query: merchantQuery,
		},
		{
			name: "commerce-application-get", method: http.MethodGet,
			args: []string{"merchant", "commerce-application", "get", merchantEngagementApplicationID, "--merchant", merchantEngagementMerchantID},
			path: applicationPath, query: merchantQuery,
		},
		{
			name: "commerce-application-update", write: true, method: http.MethodPatch,
			args: []string{"merchant", "commerce-application", "update", merchantEngagementApplicationID, "--input", updateInput},
			path: applicationPath, bodyJSON: updateJSON,
		},
		{
			name: "commerce-application-activate", write: true, method: http.MethodPost,
			args: []string{"merchant", "commerce-application", "activate", merchantEngagementApplicationID, "--merchant", merchantEngagementMerchantID, "--expected-revision", "8"},
			path: applicationPath + "/activate", bodyJSON: `{"merchantAccountId":"` + merchantEngagementMerchantID + `","expectedRevision":8}`,
		},
		{
			name: "commerce-application-suspend", write: true, method: http.MethodPost,
			args: []string{"merchant", "commerce-application", "suspend", merchantEngagementApplicationID, "--merchant", merchantEngagementMerchantID, "--expected-revision", "9"},
			path: applicationPath + "/suspend", bodyJSON: `{"merchantAccountId":"` + merchantEngagementMerchantID + `","expectedRevision":9}`,
		},
	}
}

func newMerchantEngagementServer(t *testing.T, scopes []string) (*httptest.Server, *merchantEngagementServerState) {
	t.Helper()
	state := &merchantEngagementServerState{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		state.append(capturedMerchantEngagementRequest{
			method: request.Method,
			path:   request.URL.Path,
			query:  request.URL.RawQuery,
			body:   body,
		})
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/v1/cli/auth/status" {
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"authenticated": true,
				"user": map[string]any{
					"id": "44444444-4444-4444-8444-444444444444", "displayName": "Merchant", "avatarUrl": nil,
				},
				"scopes": scopes, "expiresAt": "2027-08-21T00:00:00Z",
			})
			return
		}
		writer.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"statusCode": http.StatusBadRequest,
			"code":       "REQUEST_CAPTURED",
			"message":    "request captured by command test",
			"requestId":  "req_merchant_engagement",
		})
	}))
	return server, state
}

func executeMerchantEngagementCommand(t *testing.T, server *httptest.Server, args []string) (int, string) {
	t.Helper()
	t.Setenv(processAccessTokenEnvironment, merchantEngagementToken)
	directory := t.TempDir()
	var stdout, stderr bytes.Buffer
	exit := Execute(args, Dependencies{
		Out: &stdout, ErrOut: &stderr,
		HTTPClient: server.Client(), APIBaseURL: server.URL,
		Store:  securestore.NewMemory(),
		Region: config.RegionCN,
		Environment: skillcontent.Environment{
			Home: directory, ConfigDir: filepath.Join(directory, "config"),
		},
	})
	return exit, stdout.String() + stderr.String()
}

func assertMerchantEngagementJSONEqual(t *testing.T, expected string, actual []byte) {
	t.Helper()
	if expected == "" {
		if len(bytes.TrimSpace(actual)) != 0 {
			t.Fatalf("unexpected request body: %s", actual)
		}
		return
	}
	var expectedValue, actualValue any
	if err := json.Unmarshal([]byte(expected), &expectedValue); err != nil {
		t.Fatalf("invalid expected JSON: %v", err)
	}
	if err := json.Unmarshal(actual, &actualValue); err != nil {
		t.Fatalf("invalid request JSON: %v body=%s", err, actual)
	}
	if !reflect.DeepEqual(actualValue, expectedValue) {
		t.Fatalf("request JSON mismatch: got=%s want=%s", actual, expected)
	}
}
