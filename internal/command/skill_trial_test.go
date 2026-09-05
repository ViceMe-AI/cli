package command

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

var skillTrialSecret = strings.Repeat("a", 64)

type skillTrialTestServer struct {
	mu sync.Mutex
	// grantUses counts accepted trial-use calls; the trial limit is 2 in tests.
	grantUses          int
	trialLimit         int
	paymentStatus      string
	server             *httptest.Server
	archiveDigest      string
	archive            []byte
	ownedArchiveDigest string
	ownedArchive       []byte
	ownedDownloadCalls int
	// grantRequests records the installId of every trial-grants request in
	// order, and grantedInstallIDs remembers which ones already received a
	// secret so replays stay idempotent (mirrors the real API contract).
	// useRequests records every trial-use body so tests can assert the
	// idempotency key; trialUseFailures forces that many 500 responses first.
	grantRequests     []string
	grantedInstallIDs map[string]bool
	useRequests       []map[string]any
	trialUseFailures  int
}

func newSkillTrialTestServer(t *testing.T) *skillTrialTestServer {
	t.Helper()
	state := &skillTrialTestServer{trialLimit: 2, paymentStatus: "PENDING", grantedInstallIDs: map[string]bool{}}
	state.archive = downloadableSkillArchive(t)
	state.archiveDigest = fmt.Sprintf("%x", sha256Sum256ForTest(state.archive))
	state.ownedArchive = downloadableSkillArchiveNamed(t, "free-test", "Owned Current Skill")
	state.ownedArchiveDigest = fmt.Sprintf("%x", sha256Sum256ForTest(state.ownedArchive))
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
		var body struct {
			InstallID string `json:"installId"`
		}
		_ = json.NewDecoder(request.Body).Decode(&body)
		s.mu.Lock()
		s.grantRequests = append(s.grantRequests, body.InstallID)
		issued := s.grantedInstallIDs[body.InstallID]
		s.grantedInstallIDs[body.InstallID] = true
		limit := s.trialLimit
		s.mu.Unlock()
		response := map[string]any{
			"installId": body.InstallID, "limitUses": limit, "remainingUses": limit,
		}
		if !issued {
			response["secret"] = skillTrialSecret
		}
		writeJSONResponse(writer, response)
	case request.URL.Path == "/v1/skills/"+downloadableProductID+"/trial-use" && request.Method == http.MethodPost:
		if request.Header.Get("Authorization") != "" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		var useBody map[string]any
		_ = json.NewDecoder(request.Body).Decode(&useBody)
		s.mu.Lock()
		s.useRequests = append(s.useRequests, useBody)
		forced := s.trialUseFailures > 0
		if forced {
			s.trialUseFailures--
		}
		s.grantUses++
		used := s.grantUses
		limit := s.trialLimit
		s.mu.Unlock()
		if forced {
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
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
		digest := s.archiveDigest
		if paid {
			digest = s.ownedArchiveDigest
		}
		s.mu.Unlock()
		access := skillAccessFixture(false, paid, digest, s.server.URL+"/purchase")
		if !paid {
			access["trial"] = map[string]any{"available": true, "limitUses": s.trialLimit}
		}
		writeJSONResponse(writer, access)
	case request.URL.Path == "/v1/cli/skills/"+downloadableProductID+"/download":
		s.mu.Lock()
		s.ownedDownloadCalls++
		digest := s.ownedArchiveDigest
		s.mu.Unlock()
		writeJSONResponse(writer, map[string]any{
			"url": s.server.URL + "/owned-artifact", "fileName": "owned.zip",
			"releaseId": downloadableReleaseID, "artifactDigest": digest, "expiresAt": "2027-08-27T00:00:00Z",
		})
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
	case request.URL.Path == "/owned-artifact":
		writer.Header().Set("Content-Type", "application/zip")
		_, _ = writer.Write(s.ownedArchive)
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

// 免 CLI 安装脚本留下的明文凭证必须被 CLI 收编:同机两条安装路共用同一个
// grant,不得再发第二份试用。
func TestSkillInstallAdoptsScriptTrialCredential(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()

	home := t.TempDir()
	scriptInstallID := "22222222-2222-4222-8222-222222222222"
	trialDir := filepath.Join(home, ".viceme", "trial")
	if err := os.MkdirAll(trialDir, 0o700); err != nil {
		t.Fatal(err)
	}
	scriptCredential := fmt.Sprintf(`{"installId":%q,"secret":"script-secret","productId":%q,"market":"cn"}`, scriptInstallID, downloadableProductID)
	if err := os.WriteFile(filepath.Join(trialDir, downloadableProductID+".json"), []byte(scriptCredential), 0o600); err != nil {
		t.Fatal(err)
	}

	store := securestore.NewMemory()
	exit, envelope, _ := executeSkillTrialCommand(t, state.server, home, store,
		"skill", "install", downloadableProductID, "--agent", "codex",
	)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("install over a script credential failed: exit=%d envelope=%#v", exit, envelope)
	}
	state.mu.Lock()
	requests := append([]string(nil), state.grantRequests...)
	state.mu.Unlock()
	if len(requests) != 1 || requests[0] != scriptInstallID {
		t.Fatalf("CLI must adopt the script credential's installId, got grant requests %v", requests)
	}
	data, _ := envelope["data"].(map[string]any)
	trial, _ := data["trial"].(map[string]any)
	if trial == nil || trial["installId"] != scriptInstallID {
		t.Fatalf("trial summary did not carry the adopted installId: %#v", trial)
	}

	// 收编后 use 也走同一凭证:预检正常扣次。
	exit, envelope, _ = executeSkillTrialCommand(t, state.server, home, store,
		"skill", "use", downloadableProductID,
	)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("use after adoption failed: exit=%d envelope=%#v", exit, envelope)
	}

	// 明文文件保留在原地:脚本路继续可用同一份计数。
	if _, err := os.Stat(filepath.Join(trialDir, downloadableProductID+".json")); err != nil {
		t.Fatalf("the script credential file must stay in place: %v", err)
	}
}

// 脚本路装过的试用,`viceme skill use` 必须能直接收编明文凭证扣次,
// 而不是报 SKILL_TRIAL_GRANT_MISSING。
func TestSkillUseAdoptsScriptTrialCredentialWithoutInstall(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()

	home := t.TempDir()
	scriptInstallID := "33333333-3333-4333-8333-333333333333"
	trialDir := filepath.Join(home, ".viceme", "trial")
	if err := os.MkdirAll(trialDir, 0o700); err != nil {
		t.Fatal(err)
	}
	scriptCredential := fmt.Sprintf(`{"installId":%q,"secret":"script-secret","productId":%q,"market":"cn"}`, scriptInstallID, downloadableProductID)
	if err := os.WriteFile(filepath.Join(trialDir, downloadableProductID+".json"), []byte(scriptCredential), 0o600); err != nil {
		t.Fatal(err)
	}

	exit, envelope, _ := executeSkillTrialCommand(t, state.server, home, securestore.NewMemory(),
		"skill", "use", downloadableProductID,
	)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("use over a script-only credential failed: exit=%d envelope=%#v", exit, envelope)
	}
	data, _ := envelope["data"].(map[string]any)
	if data["allowed"] != true {
		t.Fatalf("adopted use was not allowed: %#v", data)
	}
}

func writeScriptTrialFile(t *testing.T, home, installID, pendingRequestID string) {
	t.Helper()
	trialDir := filepath.Join(home, ".viceme", "trial")
	if err := os.MkdirAll(trialDir, 0o700); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"installId": installID, "secret": "script-secret",
		"productId": downloadableProductID, "market": "cn",
	}
	if pendingRequestID != "" {
		payload["pendingRequestId"] = pendingRequestID
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(trialDir, downloadableProductID+".json"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readScriptPendingRequestID(t *testing.T, home string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(home, ".viceme", "trial", downloadableProductID+".json"))
	if err != nil {
		t.Fatalf("script trial file missing: %v", err)
	}
	var state struct {
		PendingRequestID string `json:"pendingRequestId"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	return state.PendingRequestID
}

func recordedUseRequestIDs(state *skillTrialTestServer) []string {
	state.mu.Lock()
	defer state.mu.Unlock()
	ids := make([]string, 0, len(state.useRequests))
	for _, body := range state.useRequests {
		ids = append(ids, body["requestId"].(string))
	}
	return ids
}

// 脚本结果未知后切 CLI:未确认幂等键必须被接管并复用,服务端按同一键
// 回放旧结果;权威结果送达后才清掉脚本文件里的副本,下一次使用换新键。
func TestSkillUseReplaysScriptPendingRequestId(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()

	home := t.TempDir()
	writeScriptTrialFile(t, home, "44444444-4444-4444-8444-444444444444", "script-req-1")

	store := securestore.NewMemory()
	exit, envelope, _ := executeSkillTrialCommand(t, state.server, home, store,
		"skill", "use", downloadableProductID,
	)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("use over a script pending failed: exit=%d envelope=%#v", exit, envelope)
	}
	if ids := recordedUseRequestIDs(state); len(ids) != 1 || ids[0] != "script-req-1" {
		t.Fatalf("CLI must replay the script's pending request id, got %v", ids)
	}
	if pending := readScriptPendingRequestID(t, home); pending != "" {
		t.Fatalf("script pending copy must be cleared after the authoritative result, got %q", pending)
	}

	exit, _, _ = executeSkillTrialCommand(t, state.server, home, store,
		"skill", "use", downloadableProductID,
	)
	if exit != 0 {
		t.Fatalf("second use failed")
	}
	if ids := recordedUseRequestIDs(state); len(ids) != 2 || ids[1] == "script-req-1" {
		t.Fatalf("a fresh use must use a fresh request id, got %v", ids)
	}
}

// 响应未知(服务端 500)时未确认键保留:CLI 重试仍复用同一键,
// 不会对同一使用发第二个键。
func TestSkillUseKeepsScriptPendingWhenResponseLost(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()
	state.mu.Lock()
	state.trialUseFailures = 1
	state.mu.Unlock()

	home := t.TempDir()
	writeScriptTrialFile(t, home, "44444444-4444-4444-8444-444444444444", "script-req-1")

	store := securestore.NewMemory()
	exit, _, _ := executeSkillTrialCommand(t, state.server, home, store,
		"skill", "use", downloadableProductID,
	)
	if exit == 0 {
		t.Fatal("use must fail while the response is unknown")
	}
	if pending := readScriptPendingRequestID(t, home); pending != "script-req-1" {
		t.Fatalf("unknown outcome must keep the script pending key, got %q", pending)
	}

	exit, envelope, _ := executeSkillTrialCommand(t, state.server, home, store,
		"skill", "use", downloadableProductID,
	)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("retry after the lost response failed: exit=%d envelope=%#v", exit, envelope)
	}
	if ids := recordedUseRequestIDs(state); len(ids) != 2 || ids[1] != "script-req-1" {
		t.Fatalf("the retry must replay the same pending key, got %v", ids)
	}
	if pending := readScriptPendingRequestID(t, home); pending != "" {
		t.Fatalf("script pending copy must be cleared after the retry succeeds, got %q", pending)
	}
}

// Go 的接管/清理必须与脚本共用同一把 O_EXCL 状态锁:脚本进程持锁期间
// 有界等待并失败,而不是读到撕裂 JSON 生成新键;陈旧锁会被抢占。
func TestAdoptScriptTrialPendingRespectsScriptLock(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()

	home := t.TempDir()
	writeScriptTrialFile(t, home, "44444444-4444-4444-8444-444444444444", "script-req-1")
	lockPath := filepath.Join(home, ".viceme", "trial", downloadableProductID+".json.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, []byte("script-pid"), 0o600); err != nil {
		t.Fatal(err)
	}

	originalWait := scriptTrialLockWait
	scriptTrialLockWait = 300 * time.Millisecond
	defer func() { scriptTrialLockWait = originalWait }()

	started := time.Now()
	exit, envelope, _ := executeSkillTrialCommand(t, state.server, home, securestore.NewMemory(),
		"skill", "use", downloadableProductID,
	)
	if exit == 0 {
		t.Fatal("use must fail while the script route holds the state lock")
	}
	failure, _ := envelope["error"].(map[string]any)
	if time.Since(started) > 5*time.Second || failure["code"] != "SKILL_TRIAL_PENDING_ADOPT_FAILED" {
		t.Fatalf("use must give up on the contended script lock promptly: %v %#v", time.Since(started), envelope)
	}
	if ids := recordedUseRequestIDs(state); len(ids) != 0 {
		t.Fatalf("no trial use may be sent while the state lock is held, got %v", ids)
	}

	// 释放锁后接管恢复正常:同一键回放。
	if err := os.Remove(lockPath); err != nil {
		t.Fatal(err)
	}
	exit, envelope, _ = executeSkillTrialCommand(t, state.server, home, securestore.NewMemory(),
		"skill", "use", downloadableProductID,
	)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("use must succeed after the script lock is released: exit=%d envelope=%#v", exit, envelope)
	}
	if ids := recordedUseRequestIDs(state); len(ids) != 1 || ids[0] != "script-req-1" {
		t.Fatalf("use must replay the script's pending key after lock release, got %v", ids)
	}
}

func TestAdoptScriptTrialPendingStealsStaleScriptLock(t *testing.T) {
	t.Setenv(processAccessTokenEnvironment, "")
	state := newSkillTrialTestServer(t)
	defer state.server.Close()

	home := t.TempDir()
	writeScriptTrialFile(t, home, "44444444-4444-4444-8444-444444444444", "script-req-1")
	lockPath := filepath.Join(home, ".viceme", "trial", downloadableProductID+".json.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, []byte("dead-pid"), 0o600); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().Add(-scriptTrialLockStale - time.Minute)
	if err := os.Chtimes(lockPath, stale, stale); err != nil {
		t.Fatal(err)
	}

	exit, envelope, _ := executeSkillTrialCommand(t, state.server, home, securestore.NewMemory(),
		"skill", "use", downloadableProductID,
	)
	if exit != 0 || envelope["ok"] != true {
		t.Fatalf("use must steal the stale script lock and proceed: exit=%d envelope=%#v", exit, envelope)
	}
	if ids := recordedUseRequestIDs(state); len(ids) != 1 || ids[0] != "script-req-1" {
		t.Fatalf("use must replay the pending key after stealing the stale lock, got %v", ids)
	}
	if _, err := os.Stat(lockPath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the stale lock must be cleaned up, stat err=%v", err)
	}
}

func TestSkillUseConsumesTrialThenClosesPurchaseAndReinstallsCanonicalPackage(t *testing.T) {
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
	if data["install"] == nil {
		t.Fatalf("paid trial use did not report the canonical reinstall: %#v", data)
	}
	state.mu.Lock()
	ownedDownloadCalls := state.ownedDownloadCalls
	state.mu.Unlock()
	if ownedDownloadCalls != 1 {
		t.Fatalf("trial conversion must download the canonical owned package once, got %d", ownedDownloadCalls)
	}
	for _, path := range []string{
		filepath.Join(home, ".agents", "skills", "free-test", "SKILL.md"),
	} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("installed Skill disappeared after purchase: %v", err)
		}
		if bytes.Contains(content, []byte(skillTrialGateMarker)) {
			t.Fatalf("trial gate survived the purchase in %s", path)
		}
		if !bytes.Contains(content, []byte("Owned Current Skill")) || bytes.Contains(content, []byte("Free Test Skill")) {
			t.Fatalf("the canonical owned package did not replace the trial package in %s: %q", path, content)
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
	const installDoc = "https://s3.viceme.cn/start/agent-install.md"
	const frontmatter = "---\nname: demo\ndescription: Demo.\n---\n"
	const body = "\n# Demo Skill\n\n作者正文第一段。\n"

	t.Run("inserts after frontmatter and keeps the author body", func(t *testing.T) {
		files := gateFiles(frontmatter + body)
		injectSkillTrialGate(files, productID, installDoc)
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

	t.Run("funnels machines without the CLI into the install contract", func(t *testing.T) {
		files := gateFiles(frontmatter + body)
		injectSkillTrialGate(files, productID, installDoc)
		content := string(files["SKILL.md"].Data)
		marker := strings.Index(content, skillTrialGateMarker)
		for _, needle := range []string{
			installDoc,
			"不得跳过检查直接使用本技能",
			"`viceme doctor`",
			"停止使用本技能",
			"viceme skill use " + productID,
		} {
			if !strings.Contains(content, needle) {
				t.Fatalf("gate is missing %q:\n%s", needle, content)
			}
		}
		tail := strings.Index(content, skillTrialGateTail)
		bodyStart := strings.Index(content, "# Demo Skill")
		if tail < 0 || tail < marker || tail > bodyStart {
			t.Fatalf("tail anchor must stay inside the gate, before the author body:\n%s", content)
		}
	})

	t.Run("prepends when the file has no frontmatter", func(t *testing.T) {
		files := gateFiles("# Demo Skill\n")
		injectSkillTrialGate(files, productID, installDoc)
		content := string(files["SKILL.md"].Data)
		if !strings.HasPrefix(content, skillTrialGateMarker) {
			t.Fatalf("gate must be prepended without frontmatter:\n%s", content)
		}
	})

	t.Run("is idempotent on reinstall", func(t *testing.T) {
		files := gateFiles(frontmatter + body)
		injectSkillTrialGate(files, productID, installDoc)
		once := files["SKILL.md"].Data
		injectSkillTrialGate(files, productID, installDoc)
		if !bytes.Equal(once, files["SKILL.md"].Data) {
			t.Fatalf("second injection changed the file")
		}
		if strings.Count(string(once), skillTrialGateMarker) != 1 {
			t.Fatalf("marker injected more than once")
		}
	})

	t.Run("normalizes CRLF before injecting", func(t *testing.T) {
		files := gateFiles(strings.ReplaceAll(frontmatter+body, "\n", "\r\n"))
		injectSkillTrialGate(files, productID, installDoc)
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
		injectSkillTrialGate(files, productID, installDoc)
		if files["SKILL.md"].Mode != 0o755 {
			t.Fatalf("file mode was not preserved")
		}
	})
}

func TestStripTrialGateSectionEdgeCases(t *testing.T) {
	const productID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	const installDoc = "https://s3.viceme.cn/start/agent-install.md"

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
		injectSkillTrialGate(files, productID, installDoc)
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
