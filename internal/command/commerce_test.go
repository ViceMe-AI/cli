package command

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/appmanifest"
	"github.com/ViceMe-AI/cli/internal/securestore"
)

func TestCommerceOfferCreateUsesLinkedEnvironmentAndExplicitIdempotencyKey(t *testing.T) {
	var received api.CreateCommerceOfferRequest
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/creator-apps/"+testAppID+"/environments/TEST/commerce/offers" {
			http.Error(writer, "not found", http.StatusNotFound)
			return
		}
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Fatal(err)
		}
		writeCommandJSON(t, writer, api.CommerceOffer{
			ID:          "550e8400-e29b-41d4-a716-446655440020",
			AppID:       testAppID,
			Environment: "TEST",
			Name:        received.Name,
			AmountMinor: received.AmountMinor,
			Currency:    received.Currency,
			Purpose:     received.Purpose,
			Status:      "ACTIVE",
			CreatedAt:   time.Date(2026, time.August, 6, 1, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2026, time.August, 6, 1, 0, 0, 0, time.UTC),
		})
	}))
	defer server.Close()

	project := createCommerceProject(t, "TEST", true)
	store := securestore.NewMemory()
	dependencies := authenticatedDependencies(t, server, store)
	code, stdout, stderr, _ := runCLIWithDependencies(
		t,
		server,
		store,
		"",
		dependencies,
		"commerce", "offer", "create",
		"--dir", project,
		"--client-request-id", testClientRequestID,
		"--name", "Support this work",
		"--amount-minor", "1",
		"--currency", "cny",
		"--purpose", "tip",
	)
	if code != 0 || stderr != "" {
		t.Fatalf("commerce create code=%d stderr=%s", code, stderr)
	}
	if received.ClientRequestID != testClientRequestID || received.AmountMinor != 1 || received.Currency != "CNY" || received.Purpose != "TIP" {
		t.Fatalf("unexpected request: %#v", received)
	}
	if !strings.Contains(stdout, `"data-viceme-checkout"`) || !strings.Contains(stdout, testPublishableKey) {
		t.Fatalf("output omitted Widget integration data: %s", stdout)
	}
}

func TestCommerceOfferCreateRejectsMissingIdempotencyBeforeNetwork(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(writer, "unexpected", http.StatusInternalServerError)
	}))
	defer server.Close()

	project := createCommerceProject(t, "TEST", true)
	store := securestore.NewMemory()
	dependencies := authenticatedDependencies(t, server, store)
	code, _, stderr, _ := runCLIWithDependencies(
		t,
		server,
		store,
		"",
		dependencies,
		"commerce", "offer", "create",
		"--dir", project,
		"--name", "No key",
		"--amount-minor", "1",
	)
	if code == 0 || !strings.Contains(stderr, "commerce_request_id") {
		t.Fatalf("missing idempotency key code=%d stderr=%s", code, stderr)
	}
	if requests.Load() != 0 {
		t.Fatalf("invalid command reached the API: requests=%d", requests.Load())
	}
}

func TestCommerceOfferCommandsRequireTESTCommerceBinding(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	store := securestore.NewMemory()
	dependencies := authenticatedDependencies(t, server, store)

	liveProject := createCommerceProject(t, "LIVE", true)
	code, _, stderr, _ := runCLIWithDependencies(t, server, store, "", dependencies, "commerce", "offer", "list", "--dir", liveProject)
	if code == 0 || !strings.Contains(stderr, "commerce_test_only") {
		t.Fatalf("LIVE binding code=%d stderr=%s", code, stderr)
	}

	missingProject := createCommerceProject(t, "TEST", false)
	code, _, stderr, _ = runCLIWithDependencies(t, server, store, "", dependencies, "commerce", "offer", "list", "--dir", missingProject)
	if code == 0 || !strings.Contains(stderr, "commerce_capability_missing") {
		t.Fatalf("missing capability code=%d stderr=%s", code, stderr)
	}
}

func createCommerceProject(t *testing.T, environment string, commerce bool) string {
	t.Helper()
	project := filepath.Join(t.TempDir(), "commerce-project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	capabilities := map[string]appmanifest.Capability{}
	if commerce {
		capabilities["commerce"] = appmanifest.Capability{
			ContractVersion: "1.0.0",
			SDKPackage:      "@viceme/web-sdk",
			SDKVersion:      "0.1.0",
		}
	}
	publishableKey := testPublishableKey
	if environment == "LIVE" {
		publishableKey = livePublishableKey
	}
	if _, err := appmanifest.Save(project, appmanifest.Manifest{
		SchemaVersion:  appmanifest.SchemaVersion,
		AppID:          testAppID,
		HostingMode:    "EXTERNAL",
		Environment:    environment,
		PublishableKey: publishableKey,
		Origin:         "https://creator.example",
		Capabilities:   capabilities,
	}); err != nil {
		t.Fatal(err)
	}
	return project
}
