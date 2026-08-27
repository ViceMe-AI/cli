package command

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestMerchantCommandsRequireScopedLoginBeforeAuthoring(t *testing.T) {
	const accessToken = "vme_cli_1234567890123456789012345678901234567890123"
	const merchantID = "11111111-1111-4111-8111-111111111111"
	var writeEnabled atomic.Bool
	var createCalls atomic.Int32
	var templateCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/v1/cli/auth/status" {
			scopes := []string{"profile:read", "merchant-commerce:read"}
			if writeEnabled.Load() {
				scopes = append(scopes, "merchant-commerce:write")
			}
			writeJSONResponse(writer, map[string]any{
				"authenticated": true,
				"user":          map[string]any{"id": "22222222-2222-4222-8222-222222222222", "displayName": "Creator", "avatarUrl": nil},
				"scopes":        scopes, "expiresAt": "2027-08-21T00:00:00Z",
			})
			return
		}
		if request.Header.Get("Authorization") != "Bearer "+accessToken {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case "/v1/cli/merchant/accounts":
			writeJSONResponse(writer, map[string]any{"items": []any{map[string]any{
				"id": merchantID, "creatorAccountId": "33333333-3333-4333-8333-333333333333",
				"displayName": "Creator", "status": "ACTIVE", "statusVersion": 1,
			}}})
		case "/v1/cli/merchant/product-authoring-templates":
			if request.URL.Query().Get("merchantAccountId") != merchantID {
				t.Fatalf("unexpected template merchant: %s", request.URL.RawQuery)
			}
			templateCalls.Add(1)
			writeJSONResponse(writer, map[string]any{"items": []any{map[string]any{"code": "GENERIC_MERCHANT", "status": "ACTIVE"}}})
		case "/v1/cli/merchant/contracts/work-create/validate":
			writeJSONResponse(writer, map[string]any{"code": "work-create", "valid": true, "issues": []any{}})
		case "/v1/cli/merchant/works":
			createCalls.Add(1)
			writeJSONResponse(writer, map[string]any{
				"id": "44444444-4444-4444-8444-444444444444", "kind": "SKILL", "origin": "USER_AUTHORED",
				"slug": "photo-printing", "title": "照片打印", "status": "DRAFT", "revision": 1,
				"owner": map[string]any{}, "skill": map[string]any{}, "website": nil,
				"createdAt": "2026-08-21T00:00:00Z", "updatedAt": "2026-08-21T00:00:00Z",
			})
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	t.Setenv(processAccessTokenEnvironment, accessToken)

	root := t.TempDir()
	input := filepath.Join(root, "work.json")
	if err := os.WriteFile(input, []byte(`{"kind":"SKILL","merchantAccountId":"`+merchantID+`","clientRequestId":"55555555-5555-4555-8555-555555555555","market":"CN","slug":"photo-printing","title":"照片打印","content":{"summary":"照片打印服务","bodyMarkdown":"## 照片打印","templateType":"service","tags":[],"media":[],"actionConfig":{}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	dependencies := Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: securestore.NewMemory(),
		HTTPClient: server.Client(), APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
	}
	run := func(arguments ...string) (int, map[string]any) {
		t.Helper()
		stdout.Reset()
		stderr.Reset()
		exit := Execute(arguments, dependencies)
		if strings.Contains(stdout.String(), accessToken) || strings.Contains(stderr.String(), accessToken) {
			t.Fatalf("CLI access token leaked: stdout=%q stderr=%q", stdout.String(), stderr.String())
		}
		var envelope map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("invalid envelope: exit=%d stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
		}
		return exit, envelope
	}
	if exit, envelope := run("merchant", "accounts"); exit != 0 || envelope["ok"] != true {
		t.Fatalf("read-scoped account lookup failed: exit=%d result=%#v", exit, envelope)
	}
	if exit, envelope := run("merchant", "product", "templates", "--merchant", merchantID); exit != 0 || envelope["ok"] != true || templateCalls.Load() != 1 {
		t.Fatalf("authoring template lookup failed: exit=%d result=%#v calls=%d", exit, envelope, templateCalls.Load())
	}
	if exit, envelope := run("merchant", "work", "create", "--input", input); exit == 0 || envelope["ok"] != false || createCalls.Load() != 0 {
		t.Fatalf("write ran without write scope: exit=%d result=%#v calls=%d", exit, envelope, createCalls.Load())
	}
	writeEnabled.Store(true)
	if exit, envelope := run("merchant", "work", "create", "--input", input); exit != 0 || envelope["ok"] != true || createCalls.Load() != 1 {
		t.Fatalf("authorized Work creation failed: exit=%d result=%#v calls=%d", exit, envelope, createCalls.Load())
	}
}

func TestMerchantWorkCreateStopsBeforeMutationWhenContractIsInvalid(t *testing.T) {
	const accessToken = "vme_cli_1234567890123456789012345678901234567890123"
	var createCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/cli/auth/status":
			writeJSONResponse(writer, map[string]any{
				"authenticated": true,
				"user":          map[string]any{"id": "22222222-2222-4222-8222-222222222222", "displayName": "Creator", "avatarUrl": nil},
				"scopes":        []string{"profile:read", "merchant-commerce:read", "merchant-commerce:write"},
				"expiresAt":     "2027-08-21T00:00:00Z",
			})
		case "/v1/cli/merchant/contracts/work-create/validate":
			writeJSONResponse(writer, map[string]any{
				"code": "work-create", "valid": false,
				"issues": []any{
					map[string]any{"path": []any{"market"}, "code": "invalid_type", "message": "expected string"},
					map[string]any{"path": []any{"content"}, "code": "invalid_type", "message": "expected object"},
				},
			})
		case "/v1/cli/merchant/works":
			createCalls.Add(1)
			writer.WriteHeader(http.StatusCreated)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	t.Setenv(processAccessTokenEnvironment, accessToken)

	root := t.TempDir()
	input := filepath.Join(root, "work.json")
	if err := os.WriteFile(input, []byte(`{"kind":"SERVICE","market":{"region":"CN"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	exit := Execute([]string{"merchant", "work", "create", "--input", input}, Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: securestore.NewMemory(),
		HTTPClient: server.Client(), APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
	})
	if exit == 0 || createCalls.Load() != 0 {
		t.Fatalf("invalid input reached mutation: exit=%d calls=%d output=%s", exit, createCalls.Load(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "MERCHANT_INPUT_CONTRACT_INVALID") ||
		!strings.Contains(stdout.String(), "market") || !strings.Contains(stdout.String(), "content") {
		t.Fatalf("all validation issues were not returned: %s", stdout.String())
	}
}

func TestSplitInteractionDraftInputKeepsStrictRequestBody(t *testing.T) {
	workID, request, err := splitInteractionDraftInput(json.RawMessage(`{"workId":"11111111-1111-4111-8111-111111111111","merchantAccountId":"22222222-2222-4222-8222-222222222222","analysisId":"33333333-3333-4333-8333-333333333333","analysisDigest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","sourceType":"STRUCTURED","definition":{"schemaVersion":1}}`))
	if err != nil {
		t.Fatal(err)
	}
	if workID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("unexpected work ID: %q", workID)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(request, &body); err != nil {
		t.Fatal(err)
	}
	if _, leaked := body["workId"]; leaked {
		t.Fatal("workId leaked into the strict Shop request body")
	}
	if string(body["sourceType"]) != `"STRUCTURED"` {
		t.Fatalf("unexpected request: %s", request)
	}
	if string(body["analysisId"]) != `"33333333-3333-4333-8333-333333333333"` {
		t.Fatalf("analysis binding was removed: %s", request)
	}
}

func TestSplitInteractionAnalysisConfirmationRemovesRouteIDs(t *testing.T) {
	workID, analysisID, request, err := splitInteractionAnalysisConfirmation(json.RawMessage(`{"workId":"11111111-1111-4111-8111-111111111111","analysisId":"33333333-3333-4333-8333-333333333333","merchantAccountId":"22222222-2222-4222-8222-222222222222","analysisDigest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","acknowledgedCodes":[],"resolutions":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if workID == "" || analysisID == "" {
		t.Fatalf("route IDs were not extracted: %q %q", workID, analysisID)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(request, &body); err != nil {
		t.Fatal(err)
	}
	if _, leaked := body["workId"]; leaked {
		t.Fatal("workId leaked into confirmation body")
	}
	if _, leaked := body["analysisId"]; leaked {
		t.Fatal("analysisId leaked into confirmation body")
	}
}

func TestSplitInteractionDraftInputRequiresWorkID(t *testing.T) {
	if _, _, err := splitInteractionDraftInput(json.RawMessage(`{"sourceType":"STRUCTURED"}`)); err == nil {
		t.Fatal("missing workId was accepted")
	}
}

func TestMerchantAnalysisConfirmBuildsInternalAcknowledgmentsFromCurrentAnalysis(t *testing.T) {
	const accessToken = "vme_cli_1234567890123456789012345678901234567890123"
	const merchantID = "11111111-1111-4111-8111-111111111111"
	const workID = "22222222-2222-4222-8222-222222222222"
	const analysisID = "33333333-3333-4333-8333-333333333333"
	const digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	var confirmCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/cli/auth/status":
			writeJSONResponse(writer, map[string]any{
				"authenticated": true,
				"user":          map[string]any{"id": "44444444-4444-4444-8444-444444444444", "displayName": "Creator", "avatarUrl": nil},
				"scopes":        []string{"profile:read", "merchant-commerce:read", "merchant-commerce:write"},
				"expiresAt":     "2027-08-21T00:00:00Z",
			})
		case "/v1/cli/merchant/works/" + workID + "/interaction-analyses/current":
			writeJSONResponse(writer, map[string]any{
				"analysisId": analysisID, "digest": digest, "status": "REVIEW_REQUIRED",
				"analysis": map[string]any{
					"confirmationItems": []any{
						map[string]any{"code": "RESULT_DELIVERY_FORMAT"},
						map[string]any{"code": "CANDIDATE_DATA_RETENTION"},
					},
					"openDecisions": []any{},
				},
			})
		case "/v1/cli/merchant/contracts/analysis-confirm/validate":
			var body struct {
				Input struct {
					AcknowledgedCodes []string `json:"acknowledgedCodes"`
				} `json:"input"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(body.Input.AcknowledgedCodes, []string{"RESULT_DELIVERY_FORMAT", "CANDIDATE_DATA_RETENTION"}) {
				t.Fatalf("expected dynamic generated acknowledgments, got %#v", body.Input.AcknowledgedCodes)
			}
			writeJSONResponse(writer, map[string]any{"code": "analysis-confirm", "valid": true, "issues": []any{}})
		case "/v1/cli/merchant/works/" + workID + "/interaction-analyses/" + analysisID + "/confirm":
			confirmCalls.Add(1)
			writeJSONResponse(writer, map[string]any{"analysisId": analysisID, "digest": digest, "status": "CONFIRMED"})
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	t.Setenv(processAccessTokenEnvironment, accessToken)
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	exit := Execute([]string{
		"merchant", "work", "analysis", "confirm", workID,
		"--merchant", merchantID, "--analysis", analysisID, "--digest", digest,
	}, Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: securestore.NewMemory(),
		HTTPClient: server.Client(), APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
	})
	if exit != 0 || confirmCalls.Load() != 1 {
		t.Fatalf("confirmation failed: exit=%d calls=%d stdout=%s stderr=%s", exit, confirmCalls.Load(), stdout.String(), stderr.String())
	}
}

func TestParseInteractionAnalysisResolutionsRejectsDuplicates(t *testing.T) {
	if _, err := parseInteractionAnalysisResolutions([]string{"MODE=A", "MODE=B"}); err == nil {
		t.Fatal("duplicate resolution was accepted")
	}
}
