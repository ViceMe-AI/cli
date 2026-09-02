package command

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/pathidentity"
	"github.com/ViceMe-AI/cli/internal/privatepath"
	"github.com/ViceMe-AI/cli/internal/replicacontent"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestReplicaInstallPurchasesDownloadsAndAtomicallyInstallsWithoutPersistingCapabilities(t *testing.T) {
	const (
		accessToken     = "vme_cli_1234567890123456789012345678901234567890123"
		fullCode        = "VICEME-REPLICA:VMR-ABCDEFGHIJKLMNOPQRST"
		shortCode       = "VMR-ABCDEFGHIJKLMNOPQRST"
		replicaID       = "11111111-1111-4111-8111-111111111111"
		quoteID         = "22222222-2222-4222-8222-222222222222"
		versionID       = "33333333-3333-4333-8333-333333333333"
		quoteReqID      = "44444444-4444-4444-8444-444444444444"
		orderReqID      = "55555555-5555-4555-8555-555555555555"
		ownedQuoteReqID = "88888888-8888-4888-8888-888888888881"
		ownedOrderReqID = "88888888-8888-4888-8888-888888888882"
		orderNo         = "VMO-20260901-000001"
	)
	archive := replicaTestZIP(t, map[string]string{
		"index.html":    "<h1>Purchased replica</h1>",
		"assets/app.js": "console.log('installed')",
	})
	digestBytes := sha256.Sum256(archive)
	digest := hex.EncodeToString(digestBytes[:])
	signer := newReplicaTestSigner(t, "replica-test-v1")
	trustReplicaTestSigner(t, signer)
	license := signedReplicaTestLicense(t, signer, replicaID, versionID, 7, orderNo, digest)
	paymentServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("CLI must open, not fetch, the payment page")
	}))
	defer paymentServer.Close()
	paymentURL := paymentServer.URL + "/demo-checkout#payment-capability"

	var objectDownloads atomic.Int32
	objectServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		objectDownloads.Add(1)
		if request.Method != http.MethodGet || request.URL.Path != "/private/source.zip" || request.URL.Query().Get("signature") != "download-capability" {
			t.Fatalf("unexpected object request: %s %s", request.Method, request.URL.String())
		}
		if authorization := request.Header.Get("Authorization"); authorization != "" {
			t.Fatalf("presigned download received API authorization: %q", authorization)
		}
		writer.Header().Set("Content-Length", strconv.Itoa(len(archive)))
		_, _ = writer.Write(archive)
	}))
	defer objectServer.Close()
	downloadURL := objectServer.URL + "/private/source.zip?signature=download-capability"

	var statusCalls atomic.Int32
	var quoteCalls atomic.Int32
	var orderCalls atomic.Int32
	controlServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+accessToken {
			t.Fatalf("control API did not receive authorization: %q", request.Header.Get("Authorization"))
		}
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/cli/auth/status":
			writeReplicaAuthStatus(writer)
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replicas/resolve":
			assertJSONFields(t, request, map[string]any{"instruction": fullCode})
			writeJSONResponse(writer, replicaResolutionResponse(replicaID, shortCode))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replicas/quotes":
			requestID := quoteReqID
			if quoteCalls.Add(1) > 1 {
				requestID = ownedQuoteReqID
			}
			assertJSONFields(t, request, map[string]any{"instruction": fullCode, "clientRequestId": requestID})
			writeJSONResponse(writer, replicaQuoteResponse(quoteID))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replicas/orders":
			requestID := orderReqID
			if orderCalls.Add(1) > 1 {
				requestID = ownedOrderReqID
			}
			assertJSONFields(t, request, map[string]any{"quoteId": quoteID, "clientRequestId": requestID, "locale": "zh-CN"})
			if requestID == ownedOrderReqID {
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(writer).Encode(map[string]any{
					"statusCode": http.StatusConflict, "code": "PRODUCT_ALREADY_OWNED", "message": "already owned", "requestId": "owned-request",
				})
				return
			}
			writeJSONResponse(writer, map[string]any{
				"orderNo": orderNo, "status": "PENDING",
				"paymentAction": map[string]any{"type": "REDIRECT", "url": paymentURL},
				"expiresAt":     time.Now().UTC().Add(time.Minute).Format(time.RFC3339),
			})
		case request.Method == http.MethodGet && request.URL.Path == "/v1/website-replicas/orders/"+orderNo+"/status":
			status := "PENDING"
			if statusCalls.Add(1) > 1 {
				status = "PAID"
			}
			writeJSONResponse(writer, map[string]any{"orderNo": orderNo, "payment": map[string]any{"status": status, "paidAt": nil, "closedAt": nil}, "fulfillment": nil, "serviceCase": nil})
		case request.Method == http.MethodGet && request.URL.Path == "/v1/website-replicas/"+shortCode+"/download":
			writeJSONResponse(writer, map[string]any{
				"replicaId": replicaID, "versionId": versionID, "version": 7, "fileName": "source.zip",
				"sizeBytes": len(archive), "artifactDigest": digest, "downloadUrl": downloadURL,
				"expiresAt": time.Now().UTC().Add(time.Minute).Format(time.RFC3339),
				"license":   license,
			})
		default:
			t.Fatalf("unexpected control request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer controlServer.Close()

	t.Setenv(processAccessTokenEnvironment, accessToken)
	root := t.TempDir()
	target := filepath.Join(root, "installed-site")
	expectedTarget, err := validateReplicaTarget(target)
	if err != nil {
		t.Fatal(err)
	}
	requestIDs := []string{quoteReqID, orderReqID, ownedQuoteReqID, ownedOrderReqID}
	nextRequestID := func() string {
		result := requestIDs[0]
		requestIDs = requestIDs[1:]
		return result
	}
	var openedURL string
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	arguments := []string{"replica", "install", fullCode, "--target", target, "--locale", "zh-CN"}
	dependencies := Dependencies{
		Out: &stdout, ErrOut: &stderr, HTTPClient: controlServer.Client(), Store: securestore.NewMemory(),
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:      config.RegionCN, APIBaseURL: controlServer.URL,
		NewID: nextRequestID,
		Sleep: func(context.Context, time.Duration) error { return nil },
		OpenURL: func(_ context.Context, value string) error {
			openedURL = value
			return nil
		},
	}
	exit := Execute(arguments, dependencies)
	if exit != 0 {
		t.Fatalf("replica install failed: exit=%d stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
	}
	if openedURL != paymentURL || strings.Contains(stderr.String(), paymentURL) || strings.Contains(stderr.String(), "payment-capability") || statusCalls.Load() < 2 || objectDownloads.Load() != 1 {
		t.Fatalf("purchase interaction was incomplete: opened=%q statusCalls=%d downloads=%d stderr=%q", openedURL, statusCalls.Load(), objectDownloads.Load(), stderr.String())
	}
	if content, err := os.ReadFile(filepath.Join(target, "index.html")); err != nil || string(content) != "<h1>Purchased replica</h1>" {
		t.Fatalf("installed source mismatch: content=%q err=%v", content, err)
	}
	licensePath := filepath.Join(expectedTarget, filepath.FromSlash(replicacontent.LicenseFilePath))
	licenseData, err := os.ReadFile(licensePath)
	if err != nil || !bytes.Contains(licenseData, []byte(license.Signature)) {
		t.Fatalf("signed license was not persisted: data=%s err=%v", licenseData, err)
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			ReplicaID      string `json:"replicaId"`
			VersionID      string `json:"versionId"`
			Version        int    `json:"version"`
			OrderNo        string `json:"orderNo"`
			Target         string `json:"target"`
			ArtifactDigest string `json:"artifactDigest"`
			LicensePath    string `json:"licensePath"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || !envelope.OK || envelope.Data.ReplicaID != replicaID || envelope.Data.VersionID != versionID || envelope.Data.Version != 7 || envelope.Data.OrderNo != orderNo || envelope.Data.Target != expectedTarget || envelope.Data.ArtifactDigest != digest || envelope.Data.LicensePath != licensePath {
		t.Fatalf("unexpected install output: envelope=%#v err=%v stdout=%q", envelope, err, stdout.String())
	}
	if strings.Contains(stdout.String(), paymentURL) || strings.Contains(stdout.String(), "payment-capability") || strings.Contains(stdout.String(), downloadURL) || strings.Contains(stdout.String(), accessToken) {
		t.Fatalf("final protocol output leaked a transient capability: %q", stdout.String())
	}
	var replayOutput bytes.Buffer
	dependencies.Out = &replayOutput
	dependencies.ErrOut = io.Discard
	dependencies.NewID = func() string {
		t.Fatal("completed installation unexpectedly created another purchase identity")
		return ""
	}
	dependencies.OpenURL = func(context.Context, string) error {
		t.Fatal("completed installation unexpectedly reopened payment")
		return nil
	}
	if replayExit := Execute(arguments, dependencies); replayExit != 0 {
		t.Fatalf("completed installation was not replayable: exit=%d output=%q", replayExit, replayOutput.String())
	}
	if objectDownloads.Load() != 1 {
		t.Fatalf("completion replay unexpectedly downloaded another archive: downloads=%d", objectDownloads.Load())
	}
	secondTarget := filepath.Join(root, "second-site")
	var ownedOutput bytes.Buffer
	dependencies.Out = &ownedOutput
	dependencies.NewID = nextRequestID
	ownedArguments := []string{"replica", "install", fullCode, "--target", secondTarget, "--locale", "zh-CN"}
	if ownedExit := Execute(ownedArguments, dependencies); ownedExit != 0 {
		t.Fatalf("owned Website Replica was not installed to a new target: exit=%d output=%q", ownedExit, ownedOutput.String())
	}
	if content, err := os.ReadFile(filepath.Join(secondTarget, "index.html")); err != nil || string(content) != "<h1>Purchased replica</h1>" {
		t.Fatalf("owned source was not reinstalled: content=%q err=%v", content, err)
	}
	if objectDownloads.Load() != 2 || orderCalls.Load() != 2 || openedURL != paymentURL {
		t.Fatalf("owned reinstall did not reuse its entitlement: downloads=%d orders=%d opened=%q", objectDownloads.Load(), orderCalls.Load(), openedURL)
	}
	dependencies.NewID = func() string {
		t.Fatal("completion replay unexpectedly created another purchase identity")
		return ""
	}
	if err := os.WriteFile(filepath.Join(expectedTarget, "index.html"), []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	var tamperedOutput bytes.Buffer
	dependencies.Out = &tamperedOutput
	if tamperedExit := Execute(arguments, dependencies); tamperedExit == 0 {
		t.Fatalf("tampered completion target was replayed: output=%q", tamperedOutput.String())
	}
	assertReplicaSecretsAbsentFromFiles(t, root, accessToken, paymentURL, "payment-capability", downloadURL, "download-capability")
}

func TestReplicaInstallRejectsQuoteForAnotherResolvedProductOrSKU(t *testing.T) {
	const (
		accessToken = "vme_cli_1234567890123456789012345678901234567890123"
		fullCode    = "VICEME-REPLICA:VMR-ABCDEFGHIJKLMNOPQRST"
		shortCode   = "VMR-ABCDEFGHIJKLMNOPQRST"
		replicaID   = "11111111-1111-4111-8111-111111111111"
		quoteID     = "22222222-2222-4222-8222-222222222222"
		quoteReqID  = "33333333-3333-4333-8333-333333333333"
	)
	t.Setenv(processAccessTokenEnvironment, accessToken)
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{
			name: "product",
			mutate: func(response map[string]any) {
				response["product"].(map[string]any)["id"] = "44444444-4444-4444-8444-444444444444"
			},
		},
		{
			name: "SKU",
			mutate: func(response map[string]any) {
				response["sku"].(map[string]any)["id"] = "55555555-5555-4555-8555-555555555555"
			},
		},
		{
			name: "product title",
			mutate: func(response map[string]any) {
				response["product"].(map[string]any)["title"] = "Other Replica"
			},
		},
		{
			name: "currency",
			mutate: func(response map[string]any) {
				response["currency"] = "USD"
			},
		},
		{
			name: "price",
			mutate: func(response map[string]any) {
				response["unitAmountCents"] = 1200
				response["subtotalAmountCents"] = 1200
				response["totalAmountCents"] = 1200
			},
		},
		{
			name: "shipping",
			mutate: func(response map[string]any) {
				response["shippingAmountCents"] = 100
				response["totalAmountCents"] = 1090
			},
		},
		{
			name: "physical fulfillment",
			mutate: func(response map[string]any) {
				response["fulfillment"].(map[string]any)["capabilities"] = []string{"SHIPMENT"}
			},
		},
		{
			name: "non-WeChat payment",
			mutate: func(response map[string]any) {
				response["paymentOptions"] = []map[string]any{{"provider": "ALIPAY", "scenes": []string{"PAGE"}}}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var orderCalls atomic.Int32
			controlServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/v1/cli/auth/status":
					writeReplicaAuthStatus(writer)
				case "/v1/website-replicas/resolve":
					writeJSONResponse(writer, replicaResolutionResponse(replicaID, shortCode))
				case "/v1/website-replicas/quotes":
					response := replicaQuoteResponse(quoteID)
					test.mutate(response)
					writeJSONResponse(writer, response)
				case "/v1/website-replicas/orders":
					orderCalls.Add(1)
					writer.WriteHeader(http.StatusInternalServerError)
				default:
					t.Errorf("unexpected control request: %s %s", request.Method, request.URL.Path)
					writer.WriteHeader(http.StatusNotFound)
				}
			}))
			defer controlServer.Close()

			root := t.TempDir()
			target := filepath.Join(root, "site")
			var stdout bytes.Buffer
			exit := Execute([]string{"replica", "install", fullCode, "--target", target}, Dependencies{
				Out: &stdout, ErrOut: io.Discard, HTTPClient: controlServer.Client(), Store: securestore.NewMemory(),
				APIBaseURL: controlServer.URL, Region: config.RegionCN,
				Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
				NewID:       func() string { return quoteReqID },
			})
			if exit == 0 || !strings.Contains(stdout.String(), "RESPONSE_INVALID") {
				t.Fatalf("mismatched quote was not rejected: exit=%d stdout=%s", exit, stdout.String())
			}
			if orderCalls.Load() != 0 {
				t.Fatalf("mismatched quote created %d orders", orderCalls.Load())
			}
			if _, err := os.Lstat(target); !os.IsNotExist(err) {
				t.Fatalf("mismatched quote created the install target: %v", err)
			}
		})
	}
}

func TestReplicaInstallResumesTheSamePaidOrderAfterInterruption(t *testing.T) {
	const (
		accessToken = "vme_cli_1234567890123456789012345678901234567890123"
		fullCode    = "VICEME-REPLICA:VMR-ABCDEFGHIJKLMNOPQRST"
		shortCode   = "VMR-ABCDEFGHIJKLMNOPQRST"
		replicaID   = "11111111-1111-4111-8111-111111111111"
		quoteID     = "22222222-2222-4222-8222-222222222222"
		versionID   = "33333333-3333-4333-8333-333333333333"
		quoteReqID  = "44444444-4444-4444-8444-444444444444"
		orderReqID  = "55555555-5555-4555-8555-555555555555"
		orderNo     = "VMO-RESUME-1"
	)
	archive := replicaTestZIP(t, map[string]string{"index.html": "resumed"})
	digestBytes := sha256.Sum256(archive)
	digest := hex.EncodeToString(digestBytes[:])
	signer := newReplicaTestSigner(t, "replica-test-v1")
	trustReplicaTestSigner(t, signer)
	license := signedReplicaTestLicense(t, signer, replicaID, versionID, 1, orderNo, digest)
	paymentURL := "http://127.0.0.1/payment#temporary-payment-capability"

	objectServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "" {
			t.Fatal("presigned download leaked API authorization")
		}
		_, _ = writer.Write(archive)
	}))
	defer objectServer.Close()
	var quoteCalls atomic.Int32
	var orderCalls atomic.Int32
	var resolveCalls atomic.Int32
	var paid atomic.Bool
	controlServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+accessToken {
			t.Fatalf("control API authorization = %q", request.Header.Get("Authorization"))
		}
		switch request.URL.Path {
		case "/v1/cli/auth/status":
			writeReplicaAuthStatus(writer)
		case "/v1/website-replicas/resolve":
			if resolveCalls.Add(1) != 1 {
				t.Error("resumed purchase unexpectedly depended on mutable resolution")
				writer.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			writeJSONResponse(writer, replicaResolutionResponse(replicaID, shortCode))
		case "/v1/website-replicas/quotes":
			quoteCalls.Add(1)
			assertJSONFields(t, request, map[string]any{"instruction": fullCode, "clientRequestId": quoteReqID})
			writeJSONResponse(writer, replicaQuoteResponse(quoteID))
		case "/v1/website-replicas/orders":
			orderCalls.Add(1)
			assertJSONFields(t, request, map[string]any{"quoteId": quoteID, "clientRequestId": orderReqID, "locale": "zh-CN"})
			writeJSONResponse(writer, map[string]any{
				"orderNo": orderNo, "status": "PENDING",
				"paymentAction": map[string]any{"type": "REDIRECT", "url": paymentURL},
				"expiresAt":     time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
			})
		case "/v1/website-replicas/orders/" + orderNo + "/status":
			status := "PENDING"
			if paid.Load() {
				status = "PAID"
			}
			writeJSONResponse(writer, map[string]any{"orderNo": orderNo, "payment": map[string]any{"status": status, "paidAt": nil, "closedAt": nil}, "fulfillment": nil, "serviceCase": nil})
		case "/v1/website-replicas/" + shortCode + "/download":
			writeJSONResponse(writer, map[string]any{
				"replicaId": replicaID, "versionId": versionID, "version": 1, "fileName": "source.zip",
				"sizeBytes": len(archive), "artifactDigest": digest, "downloadUrl": objectServer.URL,
				"expiresAt": time.Now().UTC().Add(time.Minute).Format(time.RFC3339), "license": license,
			})
		default:
			t.Fatalf("unexpected route: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer controlServer.Close()

	t.Setenv(processAccessTokenEnvironment, accessToken)
	root := t.TempDir()
	target := filepath.Join(root, "site")
	ids := []string{quoteReqID, orderReqID}
	base := Dependencies{
		Out: io.Discard, ErrOut: io.Discard, HTTPClient: controlServer.Client(), Store: securestore.NewMemory(),
		APIBaseURL: controlServer.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		NewID: func() string {
			value := ids[0]
			ids = ids[1:]
			return value
		},
		OpenURL: func(context.Context, string) error { return nil },
		Sleep:   func(context.Context, time.Duration) error { return context.Canceled },
	}
	if exit := Execute([]string{"replica", "install", fullCode, "--target", target}, base); exit == 0 {
		t.Fatal("interrupted purchase unexpectedly succeeded")
	}
	recoveryFiles, err := filepath.Glob(filepath.Join(root, "config", "replica-purchases", "*.json"))
	if err != nil || len(recoveryFiles) != 1 {
		t.Fatalf("purchase recovery was not persisted: files=%v err=%v", recoveryFiles, err)
	}
	recovery, err := os.ReadFile(recoveryFiles[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{accessToken, paymentURL, "temporary-payment-capability", objectServer.URL} {
		if bytes.Contains(recovery, []byte(secret)) {
			t.Fatalf("purchase recovery persisted transient secret %q: %s", secret, recovery)
		}
	}
	if exit := Execute(
		[]string{"replica", "install", fullCode, "--target", filepath.Join(root, "other-site")},
		base,
	); exit == 0 {
		t.Fatal("unfinished purchase was bypassed by changing the target")
	}
	if quoteCalls.Load() != 1 || orderCalls.Load() != 1 {
		t.Fatalf("target change created a duplicate quote or order: quotes=%d orders=%d", quoteCalls.Load(), orderCalls.Load())
	}

	paid.Store(true)
	base.NewID = func() string {
		t.Fatal("resumed purchase allocated a new idempotency key")
		return ""
	}
	base.Sleep = func(context.Context, time.Duration) error { return nil }
	if exit := Execute([]string{"replica", "install", fullCode, "--target", target}, base); exit != 0 {
		t.Fatalf("resumed purchase failed with exit %d", exit)
	}
	if resolveCalls.Load() != 1 || quoteCalls.Load() != 1 || orderCalls.Load() != 1 {
		t.Fatalf("resume changed the original purchase: resolves=%d quotes=%d orders=%d", resolveCalls.Load(), quoteCalls.Load(), orderCalls.Load())
	}
	if content, err := os.ReadFile(filepath.Join(target, "index.html")); err != nil || string(content) != "resumed" {
		t.Fatalf("resumed source was not installed: content=%q err=%v", content, err)
	}
	recoveryFiles, err = filepath.Glob(filepath.Join(root, "config", "replica-purchases", "*.json"))
	if err != nil || len(recoveryFiles) != 1 {
		t.Fatalf("completed purchase receipt was not retained for output recovery: files=%v err=%v", recoveryFiles, err)
	}
	if !strings.HasPrefix(filepath.Base(recoveryFiles[0]), "completed-") {
		t.Fatalf("active purchase state was not retired after completion: %v", recoveryFiles)
	}
	completedTarget, err := validateReplicaTarget(target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(replicaTargetReservationPath(completedTarget)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("completed target reservation was not released: %v", err)
	}
}

func TestReplicaPurchaseStateAcceptsEveryAPIValidatedDatetimePrecision(t *testing.T) {
	for _, value := range []string{
		"2026-09-01T12:34Z",
		"2026-09-01T12:34:56Z",
		"2026-09-01T12:34:56.123456789Z",
		"2026-09-01T12:34:56.1234567890Z",
	} {
		if !validReplicaStateDatetime(value) {
			t.Errorf("valid API datetime was rejected from recovery state: %s", value)
		}
	}
}

func TestReplicaDownloadFilenameIsPortableAcrossClientPlatforms(t *testing.T) {
	for _, value := range []string{"source.zip", "SOURCE.ZIP"} {
		if !validReplicaDownloadFileName(value) {
			t.Errorf("valid download filename was rejected: %q", value)
		}
	}
	for _, value := range []string{"", ".zip", "dir/source.zip", `dir\source.zip`, "source.zip\x00hidden"} {
		if validReplicaDownloadFileName(value) {
			t.Errorf("unsafe download filename was accepted: %q", value)
		}
	}
}

func TestReplicaTargetUsesAnExistingCanonicalParent(t *testing.T) {
	root := t.TempDir()
	realParent := filepath.Join(root, "real-parent")
	if err := os.Mkdir(realParent, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "parent-alias")
	if err := os.Symlink(realParent, alias); err != nil {
		t.Skipf("symlink test is unavailable: %v", err)
	}
	resolved, err := validateReplicaTarget(filepath.Join(alias, "site"))
	if err != nil {
		t.Fatal(err)
	}
	canonicalParent, err := filepath.EvalSymlinks(realParent)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != filepath.Join(canonicalParent, "site") {
		t.Fatalf("target parent was not canonicalized: got=%q", resolved)
	}
	missingParent := filepath.Join(root, "missing", "site")
	if _, err := validateReplicaTarget(missingParent); err == nil {
		t.Fatal("missing target parent was accepted")
	}
	if _, err := os.Lstat(filepath.Dir(missingParent)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target validation created its missing parent: %v", err)
	}
}

func TestReplicaPurchaseReservationIsPrivateAndBoundToTarget(t *testing.T) {
	root := t.TempDir()
	stateDirectory := filepath.Join(root, "state")
	if err := os.Mkdir(stateDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	parentID, err := pathidentity.Directory(root)
	if err != nil {
		t.Fatal(err)
	}
	store := replicaPurchaseStore{
		directory:      stateDirectory,
		filename:       filepath.Join(stateDirectory, "purchase.json"),
		origin:         "https://api.viceme.cn",
		shortCode:      "VMR-ABCDEFGHIJKLMNOPQRST",
		target:         filepath.Join(root, "site"),
		targetParentID: parentID,
		now:            time.Now,
	}
	state := store.create("11111111-1111-4111-8111-111111111111")
	if err := store.reserve(&state); err != nil {
		t.Fatal(err)
	}
	if err := store.save(&state); err != nil {
		t.Fatal(err)
	}
	reservation := replicaTargetReservationPath(state.Target)
	info, err := os.Lstat(reservation)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || privatepath.RequirePrivateFile(reservation) != nil {
		t.Fatalf("target reservation is not private: mode=%v", info.Mode())
	}
	if _, exists, err := store.load(); err != nil || !exists {
		t.Fatalf("owned target reservation did not load: exists=%t err=%v", exists, err)
	}
	if err := os.WriteFile(reservation, []byte(strings.Repeat("0", 64)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.load(); err == nil {
		t.Fatal("changed target reservation was accepted")
	}
}

func TestReplicaTargetReservationCollidesAcrossPortablePathAliases(t *testing.T) {
	root := t.TempDir()
	for _, aliases := range [][2]string{
		{"Site", "site"},
		{"Ĥ̱", "ẖ̂"},
	} {
		left := replicaTargetReservationPath(filepath.Join(root, aliases[0]))
		right := replicaTargetReservationPath(filepath.Join(root, aliases[1]))
		if left != right {
			t.Errorf("portable aliases did not share one target reservation: %q != %q", left, right)
		}
	}
}

func TestReplicaPurchaseReservationRejectsAReplacedTargetParent(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(parent, "site")
	parentID, err := pathidentity.Directory(parent)
	if err != nil {
		t.Fatal(err)
	}
	store := replicaPurchaseStore{
		directory:      filepath.Join(root, "state"),
		filename:       filepath.Join(root, "state", "purchase.json"),
		origin:         "https://api.viceme.cn",
		shortCode:      "VMR-ABCDEFGHIJKLMNOPQRST",
		target:         target,
		targetParentID: parentID,
		now:            time.Now,
	}
	if err := os.Mkdir(store.directory, 0o700); err != nil {
		t.Fatal(err)
	}
	state := store.create("11111111-1111-4111-8111-111111111111")
	if err := store.reserve(&state); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(replicaTargetReservationPath(target))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(parent, filepath.Join(root, "original-parent")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	forged, err := privatepath.CreateExclusiveFile(replicaTargetReservationPath(target))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := forged.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := forged.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.verifyReservation(state); err == nil || output.AsError(err).Subtype != "REPLICA_TARGET_PARENT_CHANGED" {
		t.Fatalf("replaced target parent was accepted: %v", err)
	}
}

func TestReplicaPurchaseStoreReclaimsOnlyItsOwnInterruptedReservation(t *testing.T) {
	root := t.TempDir()
	stateDirectory := filepath.Join(root, "state")
	if err := os.Mkdir(stateDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	parentID, err := pathidentity.Directory(root)
	if err != nil {
		t.Fatal(err)
	}
	store := replicaPurchaseStore{
		directory:      stateDirectory,
		filename:       filepath.Join(stateDirectory, "purchase.json"),
		origin:         "https://api.viceme.cn",
		shortCode:      "VMR-ABCDEFGHIJKLMNOPQRST",
		target:         filepath.Join(root, "site"),
		targetParentID: parentID,
		now:            time.Now,
	}
	state := store.create("11111111-1111-4111-8111-111111111111")
	if err := store.reserve(&state); err != nil {
		t.Fatal(err)
	}
	if err := store.save(&state); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(store.filename); err != nil {
		t.Fatal(err)
	}
	replacement := store.create("22222222-2222-4222-8222-222222222222")
	if err := store.reserve(&replacement); err != nil {
		t.Fatalf("owned interrupted reservation was not reclaimed: %v", err)
	}
	otherStore := store
	otherStore.filename = filepath.Join(stateDirectory, "other.json")
	other := otherStore.create("33333333-3333-4333-8333-333333333333")
	if err := otherStore.reserve(&other); err == nil {
		t.Fatal("another purchase reclaimed a reservation it did not own")
	}
}

func TestReplicaInstallRejectsMalformedCodeBeforeAnyNetworkRequest(t *testing.T) {
	invalidCodes := []string{
		"VMR-ABCDEFGHIJKLMNOPQRST",
		"VICEME-REPLICA:vmr-ABCDEFGHIJKLMNOPQRST",
		" VICEME-REPLICA:VMR-ABCDEFGHIJKLMNOPQRST",
		"VICEME-REPLICA:VMR-ABCDEFGHIJKLMNOPQRST@https://evil.example",
	}
	for _, code := range invalidCodes {
		t.Run(code, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
			defer server.Close()
			root := t.TempDir()
			var stdout bytes.Buffer
			exit := Execute([]string{"replica", "install", code, "--target", filepath.Join(root, "target")}, Dependencies{
				Out: &stdout, HTTPClient: server.Client(), Store: securestore.NewMemory(), APIBaseURL: server.URL, Region: config.RegionCN,
				Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
			})
			if exit == 0 || requests.Load() != 0 || !strings.Contains(stdout.String(), "REPLICA_CODE_INVALID") {
				t.Fatalf("malformed code reached the API: exit=%d requests=%d stdout=%q", exit, requests.Load(), stdout.String())
			}
		})
	}
}

func TestPresentReplicaPaymentRendersQRCodeWithoutExposingProviderContent(t *testing.T) {
	root := t.TempDir()
	var stderr bytes.Buffer
	var opened string
	runtime := &Runtime{
		configBase: root,
		deps: Dependencies{
			ErrOut: &stderr,
			Now:    func() time.Time { return time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC) },
			OpenURL: func(_ context.Context, target string) error {
				opened = target
				return nil
			},
		},
	}
	const providerContent = "weixin://pay/temporary-native-capability"
	err := presentReplicaPayment(context.Background(), runtime, api.WebsiteReplicaOrder{
		OrderNo: "VMO-QR-TEST",
		Status:  "PENDING",
		PaymentAction: &api.WebsiteReplicaPaymentAction{
			Type: "QR_CODE", Content: providerContent,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if opened == "" || !filepath.IsAbs(opened) {
		t.Fatalf("QR image was not opened from a local absolute path: %q", opened)
	}
	if private, err := commercePaymentPresentationIsPrivate(opened); err != nil || !private {
		t.Fatalf("QR image is not private: private=%t err=%v", private, err)
	}
	if strings.Contains(stderr.String(), providerContent) || strings.Contains(opened, providerContent) {
		t.Fatalf("provider QR content leaked: stderr=%q opened=%q", stderr.String(), opened)
	}
}

func TestReplicaInstallRejectsDownloadLengthAndDigestMismatchWithoutTarget(t *testing.T) {
	archive := replicaTestZIP(t, map[string]string{"index.html": "mismatch"})
	digestBytes := sha256.Sum256(archive)
	validDigest := hex.EncodeToString(digestBytes[:])
	tests := []struct {
		name   string
		size   int
		digest string
	}{
		{name: "length", size: len(archive) + 1, digest: validDigest},
		{name: "digest", size: len(archive), digest: strings.Repeat("0", 64)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const accessToken = "vme_cli_1234567890123456789012345678901234567890123"
			objectServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Header.Get("Authorization") != "" {
					t.Fatal("download leaked API authorization")
				}
				_, _ = writer.Write(archive)
			}))
			defer objectServer.Close()
			ids := []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"}
			controlServer := replicaPaidControlServer(t, objectServer.URL, test.size, test.digest)
			defer controlServer.Close()
			t.Setenv(processAccessTokenEnvironment, accessToken)
			root := t.TempDir()
			target := filepath.Join(root, "target")
			var stdout bytes.Buffer
			exit := Execute([]string{
				"replica", "install", "VICEME-REPLICA:VMR-ABCDEFGHIJKLMNOPQRST", "--target", target,
			}, Dependencies{
				Out: &stdout, ErrOut: io.Discard, HTTPClient: controlServer.Client(), Store: securestore.NewMemory(), APIBaseURL: controlServer.URL, Region: config.RegionCN,
				Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
				NewID: func() string {
					value := ids[0]
					ids = ids[1:]
					return value
				},
				Sleep:   func(context.Context, time.Duration) error { return nil },
				OpenURL: func(context.Context, string) error { return nil },
			})
			if exit == 0 {
				t.Fatalf("mismatched download installed: %s", stdout.String())
			}
			if _, err := os.Lstat(target); !os.IsNotExist(err) {
				t.Fatalf("mismatched download left target: %v", err)
			}
			matches, err := filepath.Glob(target + ".viceme-replica-stage*")
			if err != nil || len(matches) != 0 {
				t.Fatalf("mismatched download left staging: %v err=%v", matches, err)
			}
		})
	}
}

func TestVerifyReplicaLicenseRejectsEmbeddedAttackerKeyAndIdentityMismatch(t *testing.T) {
	const (
		replicaID = "11111111-1111-4111-8111-111111111111"
		versionID = "22222222-2222-4222-8222-222222222222"
		orderNo   = "VMO-TEST"
		digest    = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)
	trusted := newReplicaTestSigner(t, "replica-test-v1")
	attacker := newReplicaTestSigner(t, "replica-test-v1")
	trustReplicaTestSigner(t, trusted)
	runtime := &Runtime{apiBaseURL: "https://api.viceme.ai"}
	download := api.WebsiteReplicaDownload{
		ReplicaID: replicaID, VersionID: versionID, Version: 1,
		FileName: "source.zip", SizeBytes: 1, ArtifactDigest: digest,
		DownloadURL: "https://s3.viceme.ai/source.zip",
	}

	attackerLicense := signedReplicaTestLicense(t, attacker, replicaID, versionID, 1, orderNo, digest)
	download.License = mustJSON(t, attackerLicense)
	if err := verifyReplicaLicense(context.Background(), runtime, download, orderNo); err == nil || !strings.Contains(err.Error(), "untrusted signing key") {
		t.Fatalf("embedded attacker key was not rejected: %v", err)
	}

	mismatched := signedReplicaTestLicense(t, trusted, replicaID, versionID, 1, "VMO-OTHER", digest)
	download.License = mustJSON(t, mismatched)
	if err := verifyReplicaLicense(context.Background(), runtime, download, orderNo); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatched signed claims were not rejected: %v", err)
	}

	mismatchedVersion := signedReplicaTestLicense(t, trusted, replicaID, versionID, 2, orderNo, digest)
	download.License = mustJSON(t, mismatchedVersion)
	if err := verifyReplicaLicense(context.Background(), runtime, download, orderNo); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatched signed version was not rejected: %v", err)
	}

	tampered := signedReplicaTestLicense(t, trusted, replicaID, versionID, 1, orderNo, digest)
	tampered.Signature = strings.Repeat("A", len(tampered.Signature))
	download.License = mustJSON(t, tampered)
	if err := verifyReplicaLicense(context.Background(), runtime, download, orderNo); err == nil || !strings.Contains(err.Error(), "signature is invalid") {
		t.Fatalf("tampered signature was not rejected: %v", err)
	}
}

func replicaPaidControlServer(t *testing.T, downloadURL string, size int, digest string) *httptest.Server {
	t.Helper()
	const (
		accessToken = "vme_cli_1234567890123456789012345678901234567890123"
		shortCode   = "VMR-ABCDEFGHIJKLMNOPQRST"
		replicaID   = "33333333-3333-4333-8333-333333333333"
		quoteID     = "44444444-4444-4444-8444-444444444444"
		orderNo     = "VMO-TEST"
	)
	signer := newReplicaTestSigner(t, "replica-test-v1")
	trustReplicaTestSigner(t, signer)
	license := signedReplicaTestLicense(
		t,
		signer,
		replicaID,
		"55555555-5555-4555-8555-555555555555",
		1,
		orderNo,
		digest,
	)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+accessToken {
			t.Fatalf("control API authorization = %q", request.Header.Get("Authorization"))
		}
		switch request.URL.Path {
		case "/v1/cli/auth/status":
			writeReplicaAuthStatus(writer)
		case "/v1/website-replicas/resolve":
			writeJSONResponse(writer, replicaResolutionResponse(replicaID, shortCode))
		case "/v1/website-replicas/quotes":
			writeJSONResponse(writer, replicaQuoteResponse(quoteID))
		case "/v1/website-replicas/orders":
			writeJSONResponse(writer, map[string]any{"orderNo": orderNo, "status": "PENDING", "paymentAction": map[string]any{"type": "REDIRECT", "url": server.URL + "/payment#temporary-capability"}, "expiresAt": time.Now().UTC().Add(time.Minute).Format(time.RFC3339)})
		case "/v1/website-replicas/orders/" + orderNo + "/status":
			writeJSONResponse(writer, map[string]any{"orderNo": orderNo, "payment": map[string]any{"status": "PAID", "paidAt": nil, "closedAt": nil}, "fulfillment": nil, "serviceCase": nil})
		case "/v1/website-replicas/" + shortCode + "/download":
			writeJSONResponse(writer, map[string]any{
				"replicaId": replicaID, "versionId": "55555555-5555-4555-8555-555555555555", "version": 1,
				"fileName": "source.zip", "sizeBytes": size, "artifactDigest": digest, "downloadUrl": downloadURL,
				"expiresAt": time.Now().UTC().Add(time.Minute).Format(time.RFC3339), "license": license,
			})
		default:
			t.Fatalf("unexpected route: %s %s", request.Method, request.URL.Path)
		}
	}))
	return server
}

type replicaTestSigner struct {
	keyID      string
	publicKey  string
	privateKey ed25519.PrivateKey
}

func newReplicaTestSigner(t *testing.T, keyID string) replicaTestSigner {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	return replicaTestSigner{
		keyID:      keyID,
		publicKey:  base64.RawURLEncoding.EncodeToString(publicDER),
		privateKey: privateKey,
	}
}

func trustReplicaTestSigner(t *testing.T, signer replicaTestSigner) {
	t.Helper()
	original := buildinfo.CommerceSkillTrustKeys
	buildinfo.CommerceSkillTrustKeys = signer.keyID + ":" + signer.publicKey
	t.Cleanup(func() { buildinfo.CommerceSkillTrustKeys = original })
}

func signedReplicaTestLicense(t *testing.T, signer replicaTestSigner, replicaID, versionID string, version int, orderNo, digest string) api.WebsiteReplicaLicense {
	t.Helper()
	claims := api.WebsiteReplicaLicenseClaims{
		SchemaVersion: replicaLicenseTermsVersion, EntitlementID: "77777777-7777-4777-8777-777777777777",
		ReplicaID: replicaID, VersionID: versionID, Version: version, OrderNo: orderNo, ArtifactDigest: digest,
		LicenseTermsVersion: replicaLicenseTermsVersion, IssuedAt: "2026-09-01T00:00:00.000Z",
	}
	var generic map[string]any
	encodedClaims, err := json.Marshal(claims)
	if err != nil || json.Unmarshal(encodedClaims, &generic) != nil {
		t.Fatalf("encode test license claims: %v", err)
	}
	canonical, err := json.Marshal(generic)
	if err != nil {
		t.Fatal(err)
	}
	return api.WebsiteReplicaLicense{
		Claims: claims, Algorithm: "Ed25519", SigningKeyID: signer.keyID,
		SigningPublicKey: signer.publicKey,
		Signature:        base64.RawURLEncoding.EncodeToString(ed25519.Sign(signer.privateKey, canonical)),
	}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func writeReplicaAuthStatus(writer http.ResponseWriter) {
	writeJSONResponse(writer, map[string]any{
		"authenticated": true,
		"user":          map[string]any{"id": "66666666-6666-4666-8666-666666666666", "displayName": "Buyer", "avatarUrl": nil},
		"scopes":        []string{"website-replica:read", "website-replica:write", "website-replica:purchase"},
		"expiresAt":     time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
	})
}

func assertJSONFields(t *testing.T, request *http.Request, expected map[string]any) {
	t.Helper()
	var actual map[string]any
	if err := json.NewDecoder(request.Body).Decode(&actual); err != nil {
		t.Fatal(err)
	}
	if len(actual) != len(expected) {
		t.Fatalf("unexpected JSON fields: got=%#v want=%#v", actual, expected)
	}
	for key, expectedValue := range expected {
		if actual[key] != expectedValue {
			t.Fatalf("JSON field %s = %#v, want %#v", key, actual[key], expectedValue)
		}
	}
}
