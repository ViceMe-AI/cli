package command

import (
	"bytes"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestShowcaseSubmissionRequiresConsentBeforeAnyAuthenticationOrWrite(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("unconsented showcase reached API") }))
	defer server.Close()
	home := t.TempDir()
	var stdout bytes.Buffer
	exit := Execute([]string{"replica", "showcase", "submit", "--replica", "11111111-1111-4111-8111-111111111111", "--entitlement", "22222222-2222-4222-8222-222222222222", "--version", "33333333-3333-4333-8333-333333333333", "--title", "Reading site", "--preview-url", "https://example.com/site", "--screenshot-url", "https://example.com/screenshot.png", "--author-name", "Alice", "--changes", "Added reading lists"}, Dependencies{Out: &stdout, ErrOut: &bytes.Buffer{}, HTTPClient: server.Client(), Store: securestore.NewMemory(), Environment: skillcontent.Environment{Home: home, ConfigDir: filepath.Join(home, "config")}, Region: config.RegionCN, APIBaseURL: server.URL})
	if exit != output.ExitConfirmation || !bytes.Contains(stdout.Bytes(), []byte("REPLICA_SHOWCASE_CONSENT_REQUIRED")) {
		t.Fatalf("consent gate missing: %d %s", exit, stdout.String())
	}
}
func TestShowcasePublicURLsRejectCredentialsAndLocalResources(t *testing.T) {
	for _, value := range []string{"javascript:alert(1)", "http://example.com", "https://localhost", "https://127.0.0.1/a", "https://[::1]", "https://user:password@example.com", "https://example.com/#fragment", "https://host.local/a"} {
		if validPublicShowcaseURL(value) {
			t.Errorf("accepted unsafe URL %q", value)
		}
	}
	if !validPublicShowcaseURL("https://example.com/website?theme=blue") {
		t.Fatal("valid public website rejected")
	}
}

func TestShowcaseTextMatchesTrimmedContractBoundaries(t *testing.T) {
	for _, tc := range []struct {
		value string
		valid bool
	}{{"", false}, {"  ", false}, {strings.Repeat("a", 120), true}, {strings.Repeat("a", 121), false}, {"  " + strings.Repeat("a", 120) + "  ", true}, {strings.Repeat("🌱", 60), true}, {strings.Repeat("🌱", 61), false}} {
		if validShowcaseText(tc.value, 120) != tc.valid {
			t.Errorf("unexpected validity for UTF-16 contract length")
		}
	}
}
