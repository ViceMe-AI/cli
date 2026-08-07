package command

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/securestore"
)

const (
	testRuntimeReleaseID = "660e8400-e29b-41d4-a716-446655440000"
	testRunID            = "770e8400-e29b-41d4-a716-446655440000"
	testArtifactID       = "880e8400-e29b-41d4-a716-446655440000"
	testCandidateID      = "cand_test_abcdef"
	testReleaseID        = "990e8400-e29b-41d4-a716-446655440000"
)

// TestJobGetSendsRuntimeTicketAsBearer verifies the P1 fix: job get must send
// the Runtime Ticket as `Authorization: Bearer <ticket>`, not just a
// publishable key + origin (which the API rejects with RUNTIME_AUTH_REQUIRED).
func TestJobGetSendsRuntimeTicketAsBearer(t *testing.T) {
	var seenAuth string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/runtime/runs/"+testRunID {
			http.Error(writer, "not found", http.StatusNotFound)
			return
		}
		seenAuth = request.Header.Get("Authorization")
		writeCommandJSON(t, writer, runDetailFixture("SUCCEEDED"))
	}))
	defer server.Close()

	store := securestore.NewMemory()
	dependencies := authenticatedDependencies(t, server, store)
	code, _, stderr, _ := runCLIWithDependencies(t, server, store, "", dependencies,
		"job", "get", testRunID,
		"--publishable-key", testPublishableKey,
		"--origin", "http://localhost:3000",
		"--runtime-ticket", "rtk_test_ticket_value")
	if code != 0 || stderr != "" {
		t.Fatalf("job get code=%d stderr=%s", code, stderr)
	}
	if seenAuth != "Bearer rtk_test_ticket_value" {
		t.Fatalf("job get did not send runtime ticket as Bearer; got %q", seenAuth)
	}
}

// TestJobCancelSendsRuntimeTicketAsBearer verifies the same fix for cancel.
func TestJobCancelSendsRuntimeTicketAsBearer(t *testing.T) {
	var seenAuth string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/runtime/runs/"+testRunID+"/cancel" {
			http.Error(writer, "not found", http.StatusNotFound)
			return
		}
		seenAuth = request.Header.Get("Authorization")
		writeCommandJSON(t, writer, map[string]any{
			"runId": testRunID, "status": "CANCELLED", "cancelled": true,
		})
	}))
	defer server.Close()

	store := securestore.NewMemory()
	dependencies := authenticatedDependencies(t, server, store)
	code, _, stderr, _ := runCLIWithDependencies(t, server, store, "", dependencies,
		"job", "cancel", testRunID,
		"--publishable-key", testPublishableKey,
		"--origin", "http://localhost:3000",
		"--runtime-ticket", "rtk_test_ticket_value")
	if code != 0 || stderr != "" {
		t.Fatalf("job cancel code=%d stderr=%s", code, stderr)
	}
	if seenAuth != "Bearer rtk_test_ticket_value" {
		t.Fatalf("job cancel did not send runtime ticket as Bearer; got %q", seenAuth)
	}
}

// TestJobArtifactsDownloadsViaDownloadUrl verifies the P1 artifact-contract
// fix: artifacts expose downloadUrl (not objectKey/digest) and `job artifacts
// --artifact-id` fetches the bytes.
func TestJobArtifactsDownloadsViaDownloadUrl(t *testing.T) {
	artifactBytes := []byte("fake-image-bytes")
	downloadServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "image/png")
		_, _ = writer.Write(artifactBytes)
	}))
	defer downloadServer.Close()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/runtime/runs/"+testRunID {
			http.Error(writer, "not found", http.StatusNotFound)
			return
		}
		writeCommandJSON(t, writer, runDetailFixtureWithArtifact("SUCCEEDED", downloadServer.URL))
	}))
	defer server.Close()

	outDir := t.TempDir()
	store := securestore.NewMemory()
	dependencies := authenticatedDependencies(t, server, store)
	code, _, stderr, _ := runCLIWithDependencies(t, server, store, "", dependencies,
		"job", "artifacts", testRunID,
		"--artifact-id", testArtifactID,
		"--out", outDir,
		"--publishable-key", testPublishableKey,
		"--origin", "http://localhost:3000",
		"--runtime-ticket", "rtk_test_ticket_value")
	if code != 0 || stderr != "" {
		t.Fatalf("job artifacts code=%d stderr=%s", code, stderr)
	}
	files, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one downloaded artifact, got %d", len(files))
	}
	if !strings.HasSuffix(files[0].Name(), ".png") {
		t.Fatalf("expected .png extension, got %s", files[0].Name())
	}
	got, err := os.ReadFile(filepath.Join(outDir, files[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, artifactBytes) {
		t.Fatalf("downloaded bytes mismatch: got %q want %q", got, artifactBytes)
	}
}

func runDetailFixture(status string) map[string]any {
	return map[string]any{
		"runId": testRunID, "status": status, "environment": "TEST",
		"input": map[string]any{"prompt": "hello"}, "result": nil, "error": nil,
		"artifacts": []any{}, "createdAt": "2026-08-06T00:00:00Z", "updatedAt": "2026-08-06T00:00:00Z",
		"runtimeRelease": map[string]any{"id": testRuntimeReleaseID, "contractVersion": "1.0.0"},
	}
}

func runDetailFixtureWithArtifact(status, downloadURL string) map[string]any {
	detail := runDetailFixture(status)
	detail["artifacts"] = []any{
		map[string]any{
			"artifactId": testArtifactID, "contentType": "image/png", "sizeBytes": 16,
			"kind": "IMAGE", "turnNumber": 1, "createdAt": "2026-08-06T00:00:00Z",
			"downloadUrl": downloadURL,
		},
	}
	return detail
}

// Ensure the io and api imports are retained for fixtures and the download
// ReadCloser even when the compiler trims unused references in future edits.
var (
	_ = io.ReadAll
	_ = api.RuntimeArtifact{}
)
