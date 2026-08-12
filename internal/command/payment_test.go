package command

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/payment"
	"github.com/ViceMe-AI/cli/internal/paymentconfig"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestPaymentAPIKeyIsStoredWithoutEnteringCommandOutput(t *testing.T) {
	t.Parallel()
	const rawKey = "vcp_sandbox_super_secret_value"
	var runtimeCredential string
	var runtimeIdempotencyKey string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/capability-environments/environment-id/api-keys":
			if request.Header.Get("Authorization") != "Bearer vme_cli_test" {
				t.Fatalf("control request did not use CLI login: %q", request.Header.Get("Authorization"))
			}
			_, _ = io.WriteString(writer, `{"credential":{"id":"credential-id","environmentId":"environment-id","name":"sandbox-backend","prefix":"vcp_sandbox_test","scopes":["payment:checkouts:create"],"expiresAt":null,"lastUsedAt":null,"revokedAt":null,"rotationOverlapEndsAt":null,"createdAt":"2026-08-12T00:00:00Z"},"apiKey":"`+rawKey+`"}`)
		case "/v1/checkout/v1/sessions":
			runtimeCredential = request.Header.Get("Authorization")
			runtimeIdempotencyKey = request.Header.Get("Idempotency-Key")
			_, _ = io.WriteString(writer, `{"id":"checkout-id","checkoutUrl":"https://example.test/checkout"}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	if _, err := paymentconfig.Save(root, paymentconfig.Config{
		SchemaVersion: 1, CapabilitySpace: "space-id", ApplicationID: "application-id", ApplicationSlug: "demo-app",
		Environment: "sandbox", MarketRegion: "CN", EnvironmentID: "environment-id", InstallationID: "installation-id",
	}); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(root, "checkout.json")
	if err := os.WriteFile(input, []byte(`{"externalOrderNo":"order-1","productCode":"product","priceCode":"price"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store := securestore.NewMemory()
	scope, err := credentialScopeForAPIBase(server.URL, config.RegionCN)
	if err != nil {
		t.Fatal(err)
	}
	login := credentialauth.Manager{Store: store, Region: "cn", ProfileID: "default", ProfileName: "default", Scope: scope}
	if err := login.Save(credentialauth.Credential{AccessToken: "vme_cli_test", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	dependencies := Dependencies{
		Out: &stdout, ErrOut: io.Discard, Store: store, HTTPClient: server.Client(), APIBaseURL: server.URL, Region: config.RegionCN,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
	}
	execute := func(arguments ...string) map[string]any {
		t.Helper()
		stdout.Reset()
		if exit := Execute(arguments, dependencies); exit != 0 {
			t.Fatalf("command failed: exit=%d output=%s", exit, stdout.String())
		}
		if strings.Contains(stdout.String(), rawKey) {
			t.Fatalf("command output leaked the Payment API Key: %s", stdout.String())
		}
		var envelope map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		return envelope
	}

	execute("payment", "api-key", "create", "--dir", root, "--scopes", "payment:checkouts:create")
	stored, err := (payment.Manager{Store: store, ProfileID: "default", Scope: scope}).LoadAPIKey("environment-id")
	if err != nil || stored.APIKey != rawKey || stored.CredentialID != "credential-id" {
		t.Fatalf("Payment API Key was not securely persisted: stored=%#v err=%v", stored, err)
	}
	execute("payment", "checkout", "create", "--dir", root, "--input", input, "--idempotency-key", "checkout-1")
	if runtimeCredential != "Bearer "+rawKey || runtimeIdempotencyKey != "checkout-1" {
		t.Fatalf("runtime request did not use the stored key safely: auth=%q idempotency=%q", runtimeCredential, runtimeIdempotencyKey)
	}
}
