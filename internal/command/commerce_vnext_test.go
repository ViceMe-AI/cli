package command

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestCommerceRuntimeUsesCompiledTrustRingForOfficialOrigin(t *testing.T) {
	const publicKey = "MCowBQYDK2VwAyEA11qYAYKxCrfVS_7TyWQHOg7hcvPapiMlrwIaaPcHURo"
	original := buildinfo.CommerceSkillTrustKeys
	buildinfo.CommerceSkillTrustKeys = "release-v1:" + publicKey
	t.Cleanup(func() { buildinfo.CommerceSkillTrustKeys = original })

	runtime := &Runtime{apiBaseURL: "https://api.viceme.com"}
	resolved, err := runtime.resolveCommerceTrustKey(context.Background(), "release-v1")
	if err != nil {
		t.Fatalf("compiled Commerce trust key was rejected: %v", err)
	}
	if resolved != publicKey {
		t.Fatalf("resolved unexpected Commerce trust key: %q", resolved)
	}
}

func TestCommerceSessionStartSerializesOneLocalContext(t *testing.T) {
	const (
		stableName     = "buy-photo-printing"
		localContextID = "10000000-0000-4000-8000-000000000001"
	)
	var sessionCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/product-purchase-skills/" + stableName:
			writeJSONResponse(writer, map[string]any{
				"workId": "20000000-0000-4000-8000-000000000001", "productId": "30000000-0000-4000-8000-000000000001",
				"stableName": stableName, "status": "ACTIVE", "revision": 1,
				"product": map[string]any{}, "activeRelease": map[string]any{}, "distributions": []any{},
			})
		case "/v1/commerce-sessions":
			sessionCalls.Add(1)
			time.Sleep(25 * time.Millisecond)
			writeJSONResponse(writer, map[string]any{
				"sessionId": "40000000-0000-4000-8000-000000000001", "principalId": "50000000-0000-4000-8000-000000000001",
				"principalKind": "GENERATED", "token": "vcs_serialized-token", "recovered": false,
				"expiresAt": time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
			})
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	runtime := &Runtime{
		deps: Dependencies{
			Store: securestore.NewMemory(), HTTPClient: server.Client(),
			Now: time.Now, NewID: func() string { return localContextID },
			Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		},
		profile:         config.Profile{ID: "profile", Name: "default", APIBaseURL: server.URL},
		credentialScope: "test", configBase: filepath.Join(root, "config"), apiBaseURL: server.URL,
	}
	type result struct {
		state     commerceSessionState
		recovered bool
		err       error
	}
	results := make(chan result, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			state, recovered, err := runtime.startCommerceSession(context.Background(), localContextID, stableName)
			results <- result{state: state, recovered: recovered, err: err}
		}()
	}
	group.Wait()
	close(results)
	recoveredCount := 0
	for item := range results {
		if item.err != nil {
			t.Fatalf("session start failed: %v", item.err)
		}
		if item.state.LocalContextID != localContextID || item.state.SessionID != "40000000-0000-4000-8000-000000000001" {
			t.Fatalf("unexpected scoped session: %#v", item.state)
		}
		if item.recovered {
			recoveredCount++
		}
	}
	if sessionCalls.Load() != 1 || recoveredCount != 1 {
		t.Fatalf("same context was not serialized: calls=%d recovered=%d", sessionCalls.Load(), recoveredCount)
	}
}

func TestCommerceSessionStartRecoversAfterCommittedResponseIsLost(t *testing.T) {
	const (
		stableName     = "buy-photo-printing"
		localContextID = "10000000-0000-4000-8000-000000000003"
	)
	var sessionCalls atomic.Int32
	var firstClientRequestID, firstReplaySecret string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/product-purchase-skills/" + stableName:
			writeJSONResponse(writer, map[string]any{
				"workId": "20000000-0000-4000-8000-000000000003", "productId": "30000000-0000-4000-8000-000000000003",
				"stableName": stableName, "status": "ACTIVE", "revision": 1,
				"product": map[string]any{}, "activeRelease": map[string]any{}, "distributions": []any{},
			})
		case "/v1/commerce-sessions":
			var body map[string]any
			_ = json.NewDecoder(request.Body).Decode(&body)
			clientRequestID, _ := body["clientRequestId"].(string)
			replaySecret, _ := body["replaySecret"].(string)
			if sessionCalls.Add(1) == 1 {
				firstClientRequestID = clientRequestID
				firstReplaySecret = replaySecret
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(http.StatusCreated)
				_, _ = writer.Write([]byte(`{"sessionId":`))
				return
			}
			if clientRequestID != firstClientRequestID || replaySecret != firstReplaySecret || !validCommerceReplaySecret(replaySecret) {
				writer.WriteHeader(http.StatusConflict)
				return
			}
			writeJSONResponse(writer, map[string]any{
				"sessionId": "40000000-0000-4000-8000-000000000003", "principalId": "50000000-0000-4000-8000-000000000003",
				"principalKind": "GENERATED", "token": "vcs_recovered-token", "recovered": true,
				"expiresAt": time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
			})
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	runtime := &Runtime{
		deps: Dependencies{
			Store: securestore.NewMemory(), HTTPClient: server.Client(), Now: time.Now,
			Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		},
		profile:         config.Profile{ID: "profile", Name: "default", APIBaseURL: server.URL},
		credentialScope: "test", configBase: filepath.Join(root, "config"), apiBaseURL: server.URL,
	}
	if _, _, err := runtime.startCommerceSession(context.Background(), localContextID, stableName); err == nil {
		t.Fatal("truncated committed response unexpectedly succeeded")
	}
	state, recovered, err := runtime.startCommerceSession(context.Background(), localContextID, stableName)
	if err != nil {
		t.Fatalf("response-loss retry failed: %v", err)
	}
	if !recovered || state.LocalContextID != localContextID || sessionCalls.Load() != 2 {
		t.Fatalf("response-loss retry did not replay the original request: state=%#v recovered=%v calls=%d", state, recovered, sessionCalls.Load())
	}
}

func TestCommerceOrderIntentSerializesResponseLossRecovery(t *testing.T) {
	const (
		stableName     = "buy-photo-printing"
		localContextID = "10000000-0000-4000-8000-000000000004"
		sessionID      = "40000000-0000-4000-8000-000000000004"
		quoteID        = "60000000-0000-4000-8000-000000000004"
		orderNo        = "VME202608210004"
		token          = "vcs_order-recovery-token"
	)
	var calls atomic.Int32
	var idsMu sync.Mutex
	var clientRequestIDs []string
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/orders" || request.Header.Get("Authorization") != "Bearer "+token {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(request.Body).Decode(&body)
		clientRequestID, _ := body["clientRequestId"].(string)
		idsMu.Lock()
		clientRequestIDs = append(clientRequestIDs, clientRequestID)
		idsMu.Unlock()
		if calls.Add(1) == 1 {
			close(firstStarted)
			<-releaseFirst
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusCreated)
			_, _ = writer.Write([]byte(`{"order":`))
			return
		}
		secondStarted <- struct{}{}
		writeJSONResponse(writer, map[string]any{"order": map[string]any{
			"orderNo": orderNo, "kind": "PRODUCT_PURCHASE", "status": "PENDING", "region": "CN",
			"currency": "CNY", "amountCents": 1500, "paymentProvider": "WECHAT_PAY",
			"principalKind": "GENERATED", "item": map[string]any{},
			"paymentAction": map[string]any{"type": "WECHAT_NATIVE", "codeUrl": "weixin://pay/recovered"},
			"expiresAt":     time.Now().UTC().Add(20 * time.Minute).Format(time.RFC3339),
			"paidAt":        nil, "closedAt": nil, "createdAt": time.Now().UTC().Format(time.RFC3339),
		}})
	}))
	defer server.Close()

	root := t.TempDir()
	store := securestore.NewMemory()
	var idSequence atomic.Int32
	newRuntime := func() *Runtime {
		return &Runtime{
			deps: Dependencies{
				Store: store, HTTPClient: server.Client(), Now: time.Now,
				NewID: func() string {
					value := idSequence.Add(1)
					return fmt.Sprintf("70000000-0000-4000-8000-%012d", value)
				},
				Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
			},
			profile:         config.Profile{ID: "profile", Name: "default", APIBaseURL: server.URL},
			credentialScope: "test", configBase: filepath.Join(root, "config"), apiBaseURL: server.URL,
			commerceContextID: localContextID,
		}
	}
	seed := newRuntime()
	expiresAt := time.Now().UTC().Add(time.Hour)
	if err := seed.saveCommerceSession(commerceSessionState{
		LocalContextID: localContextID, StableName: stableName,
		ProductID: "30000000-0000-4000-8000-000000000004", SessionID: sessionID,
		PrincipalID: "50000000-0000-4000-8000-000000000004", PrincipalKind: "GENERATED",
		Token: token, ExpiresAt: expiresAt,
	}); err != nil {
		t.Fatal(err)
	}
	if err := seed.saveCommerceBinding("quote", quoteID, commerceResourceBinding{
		LocalContextID: localContextID, StableName: stableName, SessionID: sessionID, ExpiresAt: expiresAt,
	}); err != nil {
		t.Fatal(err)
	}
	inputFile := filepath.Join(root, "order-recovery.json")
	if err := os.WriteFile(inputFile, []byte(`{"quoteId":"`+quoteID+`","paymentProvider":"WECHAT_PAY","paymentScene":"NATIVE","locale":"zh-CN"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	type result struct {
		orderNo string
		err     error
	}
	results := make(chan result, 2)
	go func() {
		created, err := createCommerceOrder(context.Background(), newRuntime(), stableName, inputFile)
		results <- result{orderNo: created.Order.OrderNo, err: err}
	}()
	select {
	case <-firstStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first order request did not start")
	}
	go func() {
		created, err := createCommerceOrder(context.Background(), newRuntime(), stableName, inputFile)
		results <- result{orderNo: created.Order.OrderNo, err: err}
	}()
	select {
	case <-secondStarted:
		t.Fatal("same Commerce intent reached the provider while the first request still held its lock")
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseFirst)
	select {
	case <-secondStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("response-loss retry did not reach the provider after the first lock was released")
	}

	var succeeded, failed int
	for range 2 {
		select {
		case item := <-results:
			if item.err != nil {
				failed++
			} else if item.orderNo == orderNo {
				succeeded++
			}
		case <-time.After(2 * time.Second):
			t.Fatal("order recovery command did not finish")
		}
	}
	idsMu.Lock()
	ids := append([]string(nil), clientRequestIDs...)
	idsMu.Unlock()
	if succeeded != 1 || failed != 1 || calls.Load() != 2 || len(ids) != 2 || ids[0] == "" || ids[0] != ids[1] {
		t.Fatalf("order response-loss recovery was not idempotent: success=%d failed=%d calls=%d ids=%v", succeeded, failed, calls.Load(), ids)
	}
	binding, err := seed.loadCommerceBinding("order", localContextID, orderNo)
	if err != nil || binding.SessionID != sessionID {
		t.Fatalf("recovered order binding was not persisted: binding=%#v err=%v", binding, err)
	}
}

func TestCommerceStateSeparatesIdenticalResourcesAcrossLocalContexts(t *testing.T) {
	runtime := &Runtime{
		deps:    Dependencies{Store: securestore.NewMemory()},
		profile: config.Profile{ID: "profile"}, credentialScope: "scope",
	}
	contextA := "10000000-0000-4000-8000-000000000001"
	contextB := "10000000-0000-4000-8000-000000000002"
	for localContextID, sessionID := range map[string]string{
		contextA: "20000000-0000-4000-8000-000000000001",
		contextB: "20000000-0000-4000-8000-000000000002",
	} {
		if err := runtime.saveCommerceSession(commerceSessionState{
			LocalContextID: localContextID, StableName: "buy-photo-printing", SessionID: sessionID,
			ProductID: "30000000-0000-4000-8000-000000000001", Token: "token", ExpiresAt: time.Now().Add(time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
		if err := runtime.saveCommerceBinding("order", "VME-SAME", commerceResourceBinding{
			LocalContextID: localContextID, StableName: "buy-photo-printing", SessionID: sessionID, ExpiresAt: time.Now().Add(time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
	}
	loadedA, err := runtime.loadCommerceBinding("order", contextA, "VME-SAME")
	if err != nil {
		t.Fatal(err)
	}
	loadedB, err := runtime.loadCommerceBinding("order", contextB, "VME-SAME")
	if err != nil {
		t.Fatal(err)
	}
	if loadedA.SessionID == loadedB.SessionID {
		t.Fatalf("contexts shared one resource binding: A=%#v B=%#v", loadedA, loadedB)
	}
}

func TestCommerceRuntimeKeepsOneSessionFromQuoteThroughTerminalFulfillment(t *testing.T) {
	const (
		stableName = "buy-photo-printing"
		productID  = "22222222-2222-4222-8222-222222222222"
		quoteID    = "33333333-3333-4333-8333-333333333333"
		orderNo    = "VME202608210001"
		token      = "commerce-secret-token-that-must-never-be-printed"
	)
	var sessionCalls, statusCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, "/v1/products/") ||
			request.URL.Path == "/v1/product-quotes" ||
			request.URL.Path == "/v1/orders" ||
			strings.HasSuffix(request.URL.Path, "/status") {
			if request.Header.Get("Authorization") != "Bearer "+token {
				writer.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		switch request.URL.Path {
		case "/v1/product-purchase-skills/" + stableName:
			writeJSONResponse(writer, map[string]any{
				"workId": "11111111-1111-4111-8111-111111111111", "productId": productID,
				"stableName": stableName, "status": "ACTIVE", "revision": 2,
				"product": map[string]any{}, "distributions": []any{},
				"activeRelease": map[string]any{
					"skillReleaseId": "44444444-4444-4444-8444-444444444444", "version": 1,
					"manifest": map[string]any{}, "manifestDigest": strings.Repeat("a", 64),
					"artifactDigest": strings.Repeat("b", 64), "signedEnvelope": map[string]any{},
					"signedEnvelopeDigest": strings.Repeat("c", 64), "signingKeyId": "local-v1",
					"signature": "signature", "minimumRuntimeVersion": "1.0.0",
				},
			})
		case "/v1/commerce-sessions":
			sessionCalls.Add(1)
			var body map[string]any
			_ = json.NewDecoder(request.Body).Decode(&body)
			if body["purchaseSkillStableName"] != stableName || body["clientRequestId"] == "" || body["replaySecret"] == "" {
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			writeJSONResponse(writer, map[string]any{
				"sessionId":     "55555555-5555-4555-8555-555555555555",
				"principalId":   "66666666-6666-4666-8666-666666666666",
				"principalKind": "GENERATED", "token": token,
				"expiresAt": time.Now().UTC().Add(time.Hour).Format(time.RFC3339), "recovered": false,
			})
		case "/v1/products/" + productID:
			if request.URL.Query().Get("locale") != "zh-CN" {
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			writeJSONResponse(writer, map[string]any{
				"id": productID, "slug": "photo-printing", "title": "照片打印",
				"summary": "打印并发货", "description": "", "usageInstructions": "",
				"status": "ACTIVE", "visibility": "PUBLIC", "revision": 3,
				"subjectWork": map[string]any{"id": "11111111-1111-4111-8111-111111111111", "kind": "SKILL", "slug": "photo", "title": "照片打印"},
				"merchant":    map[string]any{},
				"salesSpec": map[string]any{
					"id": "77777777-7777-4777-8777-777777777777", "version": 1,
					"digest": strings.Repeat("d", 64), "pricingPolicyCode": "UNIT_PLUS_SHIPPING",
					"paymentPolicyCode": "STANDARD", "earningPolicyCode": "MERCHANT_RECEIVABLE",
					"quantity": map[string]any{"min": 1, "max": 20}, "options": []any{},
					"buyerContract": map[string]any{"fields": []any{}}, "fulfillmentSteps": []any{},
					"skus": []any{map[string]any{
						"id": "88888888-8888-4888-8888-888888888888", "code": "six-inch", "title": "6 英寸",
						"currency": "CNY", "priceCents": 1000, "status": "ACTIVE", "inventoryPolicy": "MADE_TO_ORDER",
						"attributes": map[string]any{}, "selectedOptions": map[string]any{}, "availableAt": nil,
					}},
				},
			})
		case "/v1/product-quotes":
			var body map[string]any
			_ = json.NewDecoder(request.Body).Decode(&body)
			if body["clientRequestId"] == "" || body["skuId"] == "" {
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			writeJSONResponse(writer, map[string]any{
				"id": quoteID, "product": map[string]any{}, "attribution": map[string]any{}, "sku": map[string]any{},
				"currency": "CNY", "unitAmountCents": 1000, "quantity": 1,
				"subtotalAmountCents": 1000, "shippingAmountCents": 500, "totalAmountCents": 1500,
				"contractSummary": map[string]any{}, "fulfillment": map[string]any{},
				"expiresAt": time.Now().UTC().Add(15 * time.Minute).Format(time.RFC3339),
			})
		case "/v1/orders":
			var body map[string]any
			_ = json.NewDecoder(request.Body).Decode(&body)
			if body["quoteId"] != quoteID || body["clientRequestId"] == "" {
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			writeJSONResponse(writer, map[string]any{"order": map[string]any{
				"orderNo": orderNo, "kind": "PRODUCT_PURCHASE", "status": "PENDING", "region": "CN",
				"currency": "CNY", "amountCents": 1500, "paymentProvider": "WECHAT_PAY",
				"principalKind": "GENERATED", "item": map[string]any{},
				"paymentAction": map[string]any{"type": "WECHAT_NATIVE", "codeUrl": "weixin://pay/test"},
				"expiresAt":     time.Now().UTC().Add(20 * time.Minute).Format(time.RFC3339),
				"paidAt":        nil, "closedAt": nil, "createdAt": time.Now().UTC().Format(time.RFC3339),
			}})
		case "/v1/orders/" + orderNo + "/status":
			if statusCalls.Add(1) == 1 {
				writeJSONResponse(writer, map[string]any{
					"orderNo": orderNo, "payment": map[string]any{"status": "PENDING", "paidAt": nil, "closedAt": nil},
					"fulfillment": map[string]any{"status": "AWAITING_PAYMENT"},
				})
				return
			}
			writeJSONResponse(writer, map[string]any{
				"orderNo": orderNo, "payment": map[string]any{"status": "PAID", "paidAt": time.Now().UTC().Format(time.RFC3339), "closedAt": nil},
				"fulfillment": map[string]any{"status": "SUCCEEDED"},
			})
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	store := securestore.NewMemory()
	var stdout, stderr bytes.Buffer
	var idSequence atomic.Int32
	dependencies := Dependencies{
		Out: &stdout, ErrOut: &stderr, Store: store, HTTPClient: server.Client(),
		APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		NewID: func() string {
			value := idSequence.Add(1)
			return fmt.Sprintf("00000000-0000-4000-8000-%012d", value)
		},
		Sleep: func(_ context.Context, _ time.Duration) error { return nil },
	}
	run := func(arguments ...string) map[string]any {
		t.Helper()
		stdout.Reset()
		stderr.Reset()
		if exit := Execute(arguments, dependencies); exit != 0 {
			t.Fatalf("command failed: args=%v exit=%d stdout=%s stderr=%s", arguments, exit, stdout.String(), stderr.String())
		}
		if strings.Contains(stdout.String(), token) || strings.Contains(stderr.String(), token) {
			t.Fatalf("Commerce Session token leaked: stdout=%q stderr=%q", stdout.String(), stderr.String())
		}
		var envelope map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || envelope["ok"] != true {
			t.Fatalf("invalid command envelope: %s err=%v", stdout.String(), err)
		}
		return envelope
	}

	session := run("commerce", "session", "start", "--skill", stableName)
	localContextID := session["data"].(map[string]any)["localContextId"].(string)
	run("commerce", "product", "describe", "--skill", stableName, "--session-context", localContextID)
	quoteInput := filepath.Join(root, "quote.json")
	if err := os.WriteFile(quoteInput, []byte(`{"skuId":"88888888-8888-4888-8888-888888888888","quantity":1,"contractInput":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	run("commerce", "quote", "create", "--skill", stableName, "--input", quoteInput, "--session-context", localContextID)
	orderInput := filepath.Join(root, "order.json")
	if err := os.WriteFile(orderInput, []byte(`{"quoteId":"`+quoteID+`","paymentProvider":"WECHAT_PAY","paymentScene":"NATIVE","locale":"zh-CN"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	run("commerce", "order", "create", "--skill", stableName, "--input", orderInput, "--session-context", localContextID)
	result := run("commerce", "order", "wait", "--skill", stableName, "--order", orderNo, "--timeout", "2s", "--interval", "250ms", "--session-context", localContextID)
	data := result["data"].(map[string]any)
	fulfillment := data["fulfillment"].(map[string]any)
	if fulfillment["status"] != "SUCCEEDED" || sessionCalls.Load() != 1 || statusCalls.Load() != 2 {
		t.Fatalf("runtime did not retain one session through terminal fulfillment: result=%#v sessions=%d statuses=%d", data, sessionCalls.Load(), statusCalls.Load())
	}
}
