package command

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/securestore"
)

func TestListingUpsertUsesLinkedAppAndReturnsAuthoritativePublicURL(t *testing.T) {
	const offerID = "550e8400-e29b-41d4-a716-446655440020"
	publicURL := "https://shop.example/apps/example-work"
	var received api.UpsertCreatorAppListingRequest
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPut || request.URL.Path != "/v1/creator-apps/"+testAppID+"/listing" {
			http.Error(writer, "not found", http.StatusNotFound)
			return
		}
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Fatal(err)
		}
		writeCommandJSON(t, writer, api.CreatorAppListing{
			ID: "550e8400-e29b-41d4-a716-446655440030", AppID: testAppID,
			Slug: received.Slug, Title: received.Title, Summary: received.Summary,
			Description: received.Description, ExternalURL: received.ExternalURL,
			CoverURL: received.CoverURL, MediaURLs: received.MediaURLs,
			OfferID: received.OfferID, Status: received.Status,
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), PublicURL: &publicURL,
		})
	}))
	defer server.Close()

	project := createCommerceProject(t, "LIVE", true)
	store := securestore.NewMemory()
	dependencies := authenticatedDependencies(t, server, store)
	code, stdout, stderr, _ := runCLIWithDependencies(
		t, server, store, "", dependencies,
		"listing", "upsert",
		"--dir", project,
		"--slug", "example-work",
		"--title", "Example Work",
		"--summary", "A complete public work",
		"--description", "Longer description",
		"--external-url", "https://creator.example/work",
		"--cover-url", "https://creator.example/cover.png",
		"--media-url", "https://creator.example/one.png",
		"--offer", offerID,
		"--status", "public",
	)
	if code != 0 || stderr != "" {
		t.Fatalf("listing upsert code=%d stderr=%s", code, stderr)
	}
	if received.Status != "PUBLIC" || received.OfferID == nil || *received.OfferID != offerID {
		t.Fatalf("unexpected Listing request: %#v", received)
	}
	if !strings.Contains(stdout, publicURL) {
		t.Fatalf("output omitted authoritative public URL: %s", stdout)
	}
}

func TestListingUpsertRejectsUnsafeURLBeforeNetwork(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	project := createCommerceProject(t, "LIVE", true)
	store := securestore.NewMemory()
	dependencies := authenticatedDependencies(t, server, store)
	code, _, stderr, _ := runCLIWithDependencies(
		t, server, store, "", dependencies,
		"listing", "upsert",
		"--dir", project,
		"--slug", "unsafe-work",
		"--title", "Unsafe",
		"--summary", "Unsafe URL",
		"--description", "Description",
		"--external-url", "http://public.example/work",
		"--status", "PUBLIC",
	)
	if code == 0 || !strings.Contains(stderr, "listing_external_url") {
		t.Fatalf("unsafe URL code=%d stderr=%s", code, stderr)
	}
}

func TestCommerceLedgerListForwardsOpaqueCursor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/v1/creator-apps/"+testAppID+"/commerce/ledger" {
			http.Error(writer, "not found", http.StatusNotFound)
			return
		}
		if request.URL.Query().Get("cursor") != "opaque-cursor" || request.URL.Query().Get("limit") != "20" {
			http.Error(writer, "wrong query", http.StatusBadRequest)
			return
		}
		writeCommandJSON(t, writer, api.CreatorLedgerResponse{
			Items:    []api.CreatorLedgerEntry{},
			Balances: []api.CreatorLedgerBalance{{Currency: "CNY", AmountMinor: 1}},
		})
	}))
	defer server.Close()
	project := createCommerceProject(t, "LIVE", true)
	store := securestore.NewMemory()
	dependencies := authenticatedDependencies(t, server, store)
	code, stdout, stderr, _ := runCLIWithDependencies(
		t, server, store, "", dependencies,
		"commerce", "ledger", "list",
		"--dir", project,
		"--cursor", "opaque-cursor",
		"--limit", "20",
	)
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"amountMinor": 1`) {
		t.Fatalf("ledger list code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}
