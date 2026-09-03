package command

import (
	"bytes"
	"encoding/json"
	"fmt"
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

var skillTrialSecret = strings.Repeat("a", 64)

type skillTrialTestServer struct {
	mu sync.Mutex
	// grantUses counts accepted trial-use calls; the trial limit is 2 in tests.
	grantUses     int
	trialLimit    int
	paymentStatus string
	server        *httptest.Server
	archiveDigest string
	archive       []byte
}

func newSkillTrialTestServer(t *testing.T) *skillTrialTestServer {
	t.Helper()
	state := &skillTrialTestServer{trialLimit: 2, paymentStatus: "PENDING"}
	state.archive = downloadableSkillArchive(t)
	state.archiveDigest = fmt.Sprintf("%x", sha256Sum256ForTest(state.archive))
	server := httptest.NewServer(http.HandlerFunc(state.serveHTTP))
	state.server = server
	return state
}

func (s *skillTrialTestServer) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	if strings.HasPrefix(request.URL.Path, "/v1/products/") ||
		strings.HasPrefix(request.URL.Path, "/v1/product-quotes") ||
		strings.HasPrefix(request.URL.Path, "/v1/orders") ||
		strings.HasPrefix(request.URL.Path, "/v1/cli/") {
		if request.Header.Get("Authorization") != "Bearer "+skillPurchaseAccessToken {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
	}
	switch {
	case request.URL.Path == "/v1/skills/"+downloadableProductID+"/access":
		access := skillAccessFixture(false, false, s.archiveDigest, s.server.URL+"/purchase")
		access["trial"] = map[string]any{"available": true, "limitUses": s.trialLimit}
		writeJSONResponse(writer, access)
	case request.URL.Path == "/v1/skills/"+downloadableProductID+"/trial-grants" && request.Method == http.MethodPost:
		if request.Header.Get("Authorization") != "" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		writeJSONResponse(writer, map[string]any{
			"installId": "11111111-1111-4111-8111-111111111111", "limitUses": s.trialLimit,
			"remainingUses": s.trialLimit, "secret": skillTrialSecret,
		})
	case request.URL.Path == "/v1/skills/"+downloadableProductID+"/trial-use" && request.Method == http.MethodPost:
		if request.Header.Get("Authorization") != "" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		s.mu.Lock()
		s.grantUses++
		used := s.grantUses
		limit := s.trialLimit
		s.mu.Unlock()
		if used <= limit {
			writeJSONResponse(writer, map[string]any{
				"allowed": true, "remainingUses": limit - used, "limitUses": limit, "reason": nil, "purchaseUrl": nil,
			})
			return
		}
		writeJSONResponse(writer, map[string]any{
			"allowed": false, "remainingUses": nil, "limitUses": nil, "reason": "EXHAUSTED",
			"purchaseUrl": s.server.URL + "/purchase",
		})
	case request.URL.Path == "/v1/downloads/trial/"+downloadableProductID:
		if request.Header.Get("Authorization") != "" || request.URL.Query().Get("installId") == "" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		writeJSONResponse(writer, map[string]any{
			"url": s.server.URL + "/artifact", "fileName": "trial.zip",
			"releaseId": downloadableReleaseID, "artifactDigest": s.archiveDigest, "expiresAt": "2027-08-27T00:00:00Z",
		})
	case request.URL.Path == "/v1/cli/auth/status":
		writeJSONResponse(writer, map[string]any{
			"authenticated": true,
			"user":          map[string]any{"id": "33333333-3333-4333-8333-333333333333", "displayName": "Buyer", "avatarUrl": nil},
			"scopes":        []string{"profile:read", "skill-use:read", "buyer-commerce:read", "buyer-commerce:write"}, "expiresAt": "2027-08-27T00:00:00Z",
		})
	case request.URL.Path == "/v1/cli/skills/"+downloadableProductID+"/access":
		s.mu.Lock()
		paid := s.paymentStatus == "PAID"
		s.mu.Unlock()
		writeJSONResponse(writer, skillAccessFixture(false, paid, s.archiveDigest, s.server.URL+"/purchase"))
	case request.URL.Path == "/v1/products/"+downloadableProductID:
		writeJSONResponse(writer, map[string]any{
			"id": downloadableProductID, "slug": "paid-skill", "title": "Paid Skill",
			"summary": "Paid summary", "description": "Paid description", "usageInstructions": "Run it",
			"status": "ACTIVE", "visibility": "PUBLIC", "revision": 1,
			"salesSpec": map[string]any{
				"id": "66666666-6666-4666-8666-666666666666", "version": 1, "digest": "d", "quantity": nil,
				"skus": []map[string]any{{
					"id": skillPurchaseSkuID, "code": "standard", "title": "Standard",
					"currency": "CNY", "priceCents": 990, "status": "ACTIVE",
					"inventoryPolicy": "UNLIMITED", "attributes": map[string]any{}, "selectedOptions": map[string]string{},
				}},
			},
		})
	case request.URL.Path == "/v1/cli/product-quotes" && request.Method == http.MethodPost:
		writeJSONResponse(writer, map[string]any{
			"id":          skillPurchaseQuoteID,
			"product":     map[string]any{"id": downloadableProductID, "slug": "paid-skill", "title": "Paid Skill"},
			"attribution": map[string]any{"subjectWorkId": "77777777-7777-4777-8777-777777777777", "entryWorkId": nil, "commerceApplicationId": nil},
			"sku":         map[string]any{"id": skillPurchaseSkuID, "code": "standard", "title": "Standard", "selectedOptions": map[string]string{}},
			"currency":    "CNY", "unitAmountCents": 990, "quantity": 1,
			"subtotalAmountCents": 990, "shippingAmountCents": 0, "totalAmountCents": 990,
			"contractSummary": map[string]any{"publicFields": map[string]any{}, "sensitiveFieldKeys": []string{}, "assetCount": 0},
			"fulfillment":     map[string]any{"capabilities": []string{}, "estimatedState": "NONE"},
			"paymentOptions":  []map[string]any{{"provider": "WECHAT_PAY", "scenes": []string{"NATIVE"}}},
			"expiresAt":       "2027-08-27T00:00:00Z",
		})
	case request.URL.Path == "/v1/cli/orders" && request.Method == http.MethodPost:
		writeJSONResponse(writer, map[string]any{"order": map[string]any{
			"orderNo": skillPurchaseOrderNo, "kind": "PRODUCT_PURCHASE", "status": "PENDING",
			"region": "CN", "currency": "CNY", "amountCents": 990, "paymentProvider": "WECHAT_PAY",
			"principalKind": "USER",
			"item": map[string]any{
				"id": "88888888-8888-4888-8888-888888888888", "productId": downloadableProductID,
				"productSlug": "paid-skill", "workSlug": "paid-skill", "canonicalPath": "/creator/paid-skill",
				"skuId": skillPurchaseSkuID, "merchantAccountId": "99999999-9999-4999-8999-999999999999",
				"originAuthoringTemplateCode": "downloadable-skill",
				"attribution":                 map[string]any{"subjectWorkId": "77777777-7777-4777-8777-777777777777", "entryWorkId": nil, "commerceApplicationId": nil},
				"edition":                     nil, "quantity": 1, "unitAmountCents": 990, "totalAmountCents": 990,
			},
			"paymentAction": map[string]any{"type": "QR_CODE", "content": "weixin://wxpay/bizpayurl?pr=skill"},
			"expiresAt":     "2027-08-27T00:00:00Z", "createdAt": "2026-09-01T08:00:00Z",
		}})
	case request.URL.Path == "/v1/cli/orders/"+skillPurchaseOrderNo && request.Method == http.MethodGet:
		s.mu.Lock()
		pending := s.paymentStatus != "PAID"
		s.mu.Unlock()
		status := "PAID"
		if pending {
			status = "PENDING"
		}
		writeJSONResponse(writer, map[string]any{"order": map[string]any{
			"orderNo": skillPurchaseOrderNo, "status": status, "amountCents": 990,
		}})
case request.URL.Path == "/v1/cli/orders/"+skillPurchaseOrderNo+"/status":
		s.mu.Lock()
		s.paymentStatus = "PAID"
		s.mu.Unlock()
		writeJSONResponse(writer, map[string]any{
			"orderNo": skillPurchaseOrderNo, "payment": map[string]any{"status": "PAID"},
			"fulfillment": nil, "serviceCase": nil,
		})
	case request.URL.Path == "/artifact":
		writer.Header().Set("Content-Type", "application/zip")
		_, _ = writer.Write(s.archive)
	default:
		http.NotFound(writer, request)
	}
}

func executeSkillTrialCommand(t *testing.T, server *httptest.Server, home string, store securestore.Store, arguments ...string) (int, map[string]any, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	exit := Execute(arguments, Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: store,
		HTTPClient: server.Client(), APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{
			Home: home, CodexHome: filepath.Join(home, ".codex"), ConfigDir: filepath.Join(home, ".viceme-cli"),
		},
	})
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid envelope: exit=%d stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
	}
	return exit, envelope, stderr.String()
}

func TestPaidTrialSkillInstallsAnonymouslyWithGate(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()

	home := t.TempDir()
	exit, envelope, _ := executeSkillTrialCommand(t, state.server, home, securestore.NewMemory(),
		"skill", "install", downloadableProductID, "--agent", "codex",
	)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("anonymous trial install failed: exit=%d envelope=%#v", exit, envelope)
	}
	data, _ := envelope["data"].(map[string]any)
	trial, _ := data["trial"].(map[string]any)
	if trial == nil || trial["limitUses"] != float64(2) {
		t.Fatalf("trial install result did not carry the trial summary: %#v", data)
	}
	gatePaths := []string{
		filepath.Join(home, ".codex", "skills", "free-test", "SKILL.md"),
		filepath.Join(home, ".agents", "skills", "free-test", "SKILL.md"),
	}
	for _, path := range gatePaths {
		content, err := os.ReadFile(path)
		if err != nil || !bytes.Contains(content, []byte(skillTrialGateMarker)) || !bytes.Contains(content, []byte("viceme skill use "+downloadableProductID)) {
			t.Fatalf("installed Skill %s is missing the trial gate: err=%v", path, err)
		}
		// 门禁段必须位于正文顶部(先于创作者标题),保证每次加载技能第一眼读到规则。
		if strings.Index(string(content), skillTrialGateMarker) > strings.Index(string(content), "# Free Test Skill") {
			t.Fatalf("trial gate is not at the top of %s", path)
		}
	}
	state.mu.Lock()
	uses := state.grantUses
	state.mu.Unlock()
	if uses != 0 {
		t.Fatalf("trial install unexpectedly consumed a trial use: %d", uses)
	}
}

func TestSkillUseConsumesTrialThenClosesPurchaseAndRemovesGate(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, skillPurchaseAccessToken)
	state := newSkillTrialTestServer(t)
	defer state.server.Close()

	home := t.TempDir()
	store := securestore.NewMemory()
	exit, envelope, _ := executeSkillTrialCommand(t, state.server, home, store,
		"skill", "install", downloadableProductID, "--agent", "codex",
	)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("trial install failed: exit=%d envelope=%#v", exit, envelope)
	}

	// 第一次预检:放行并报告剩余次数。
	exit, envelope, _ = executeSkillTrialCommand(t, state.server, home, store,
		"skill", "use", downloadableProductID,
	)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("first trial use failed: exit=%d envelope=%#v", exit, envelope)
	}
	data, _ := envelope["data"].(map[string]any)
	if data["allowed"] != true || data["remainingUses"] != float64(1) || data["lastUse"] != false {
		t.Fatalf("first trial use did not report the remaining count: %#v", data)
	}

	// 第二次预检:最后一次试用,标记 lastUse。
	exit, envelope, _ = executeSkillTrialCommand(t, state.server, home, store,
		"skill", "use", downloadableProductID,
	)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("last trial use failed: exit=%d envelope=%#v", exit, envelope)
	}
	data, _ = envelope["data"].(map[string]any)
	if data["lastUse"] != true {
		t.Fatalf("last trial use was not flagged: %#v", data)
	}

	// 第三次预检:耗尽 → 扫码支付 → 支付成功后转正并移除门禁。
	exit, envelope, stderr := executeSkillTrialCommand(t, state.server, home, store,
		"skill", "use", downloadableProductID, "--wait", "10m",
	)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("exhausted trial use did not close the purchase loop: exit=%d envelope=%#v stderr=%q", exit, envelope, stderr)
	}
	data, _ = envelope["data"].(map[string]any)
	if data["owned"] != true || data["allowed"] != true {
		t.Fatalf("paid trial use was not reported as owned: %#v", data)
	}
	for _, path := range []string{
		filepath.Join(home, ".codex", "skills", "free-test", "SKILL.md"),
		filepath.Join(home, ".agents", "skills", "free-test", "SKILL.md"),
	} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("installed Skill disappeared after purchase: %v", err)
		}
		if bytes.Contains(content, []byte(skillTrialGateMarker)) {
			t.Fatalf("trial gate survived the purchase in %s", path)
		}
		if !bytes.Contains(content, []byte("Free Test Skill")) {
			t.Fatalf("gate removal corrupted the Skill content in %s", path)
		}
	}
}

func TestSkillUseWithoutGrantPointsToInstall(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()

	exit, envelope, _ := executeSkillTrialCommand(t, state.server, t.TempDir(), securestore.NewMemory(),
		"skill", "use", downloadableProductID,
	)
	if exit == 0 || envelope["ok"] != false {
		t.Fatalf("use without a grant unexpectedly succeeded: %#v", envelope)
	}
	errorBody, _ := envelope["error"].(map[string]any)
	if errorBody["code"] != "SKILL_TRIAL_GRANT_MISSING" {
		t.Fatalf("unexpected error without a grant: %#v", errorBody)
	}
}

func gateFiles(content string) map[string]downloadableSkillFile {
	return map[string]downloadableSkillFile{
		"SKILL.md": {Data: []byte(content), Mode: 0o644},
	}
}

func TestInjectSkillTrialGateEdgeCases(t *testing.T) {
	const productID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	const frontmatter = "---\nname: demo\ndescription: Demo.\n---\n"
	const body = "\n# Demo Skill\n\n作者正文第一段。\n"

	t.Run("inserts after frontmatter and keeps the author body", func(t *testing.T) {
		files := gateFiles(frontmatter + body)
		injectSkillTrialGate(files, productID)
		content := string(files["SKILL.md"].Data)
		marker := strings.Index(content, skillTrialGateMarker)
		heading := strings.Index(content, "# Demo Skill")
		if marker < 0 || heading < 0 || marker > heading {
			t.Fatalf("gate must sit between frontmatter and author body:\n%s", content)
		}
		if !strings.HasPrefix(content, frontmatter) {
			t.Fatalf("frontmatter must stay untouched:\n%s", content)
		}
		if !strings.HasSuffix(strings.TrimSpace(content), "作者正文第一段。") {
			t.Fatalf("author body must stay at the end:\n%s", content)
		}
	})

	t.Run("prepends when the file has no frontmatter", func(t *testing.T) {
		files := gateFiles("# Demo Skill\n")
		injectSkillTrialGate(files, productID)
		content := string(files["SKILL.md"].Data)
		if !strings.HasPrefix(content, skillTrialGateMarker) {
			t.Fatalf("gate must be prepended without frontmatter:\n%s", content)
		}
	})

	t.Run("is idempotent on reinstall", func(t *testing.T) {
		files := gateFiles(frontmatter + body)
		injectSkillTrialGate(files, productID)
		once := files["SKILL.md"].Data
		injectSkillTrialGate(files, productID)
		if !bytes.Equal(once, files["SKILL.md"].Data) {
			t.Fatalf("second injection changed the file")
		}
		if strings.Count(string(once), skillTrialGateMarker) != 1 {
			t.Fatalf("marker injected more than once")
		}
	})

	t.Run("normalizes CRLF before injecting", func(t *testing.T) {
		files := gateFiles(strings.ReplaceAll(frontmatter+body, "\n", "\r\n"))
		injectSkillTrialGate(files, productID)
		content := string(files["SKILL.md"].Data)
		if strings.Contains(content, "\r") {
			t.Fatalf("CRLF must be normalized")
		}
		if marker := strings.Index(content, skillTrialGateMarker); marker < 0 || marker > strings.Index(content, "# Demo Skill") {
			t.Fatalf("gate must sit above the author body:\n%s", content)
		}
	})

	t.Run("keeps the file mode", func(t *testing.T) {
		files := map[string]downloadableSkillFile{
			"SKILL.md": {Data: []byte(frontmatter + body), Mode: 0o755},
		}
		injectSkillTrialGate(files, productID)
		if files["SKILL.md"].Mode != 0o755 {
			t.Fatalf("file mode was not preserved")
		}
	})
}

func TestStripTrialGateSectionEdgeCases(t *testing.T) {
	const productID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

	write := func(t *testing.T, content string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "SKILL.md")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		return path
	}

	t.Run("removes the injected section and restores the author body", func(t *testing.T) {
		files := gateFiles("---\nname: demo\ndescription: Demo.\n---\n\n# Demo Skill\n\n作者正文。\n")
		injectSkillTrialGate(files, productID)
		path := write(t, string(files["SKILL.md"].Data))
		if !stripTrialGateSection(path) {
			t.Fatalf("strip reported no change")
		}
		after, _ := os.ReadFile(path)
		content := string(after)
		if strings.Contains(content, skillTrialGateMarker) {
			t.Fatalf("marker survived the strip")
		}
		if !strings.Contains(content, "作者正文。") || !strings.Contains(content, "# Demo Skill") {
			t.Fatalf("author body was corrupted:\n%s", content)
		}
	})

	t.Run("truncates at the marker when the tail anchor is missing", func(t *testing.T) {
		path := write(t, "---\nname: demo\n---\n\n# Demo Skill\n\n作者正文。\n\n"+skillTrialGateMarker+" product=x -->\n残缺段落没有尾锚\n")
		if !stripTrialGateSection(path) {
			t.Fatalf("strip reported no change")
		}
		after, _ := os.ReadFile(path)
		if strings.Contains(string(after), skillTrialGateMarker) || strings.Contains(string(after), "残缺段落") {
			t.Fatalf("legacy truncation did not clean the gate:\n%s", after)
		}
		if !strings.Contains(string(after), "作者正文。") {
			t.Fatalf("legacy truncation ate the author body:\n%s", after)
		}
	})

	t.Run("leaves files without a marker untouched", func(t *testing.T) {
		path := write(t, "# Demo Skill\n\n作者正文。\n")
		if stripTrialGateSection(path) {
			t.Fatalf("strip falsely reported a change")
		}
	})
}
