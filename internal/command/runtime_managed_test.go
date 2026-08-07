package command

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/managedrelease"
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

// --- Managed App tests (Slice E) ---

func templateArchiveFixture(t *testing.T) ([]byte, string) {
	t.Helper()
	var buffer bytes.Buffer
	zipWriter := zip.NewWriter(&buffer)
	files := map[string]string{
		"package.json": `{"name":"image-tool-starter","version":"1.0.0"}`,
		"src/app.js":   "// entry",
		"build.mjs":    "// build script",
	}
	for name, content := range files {
		writer, err := zipWriter.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	data := buffer.Bytes()
	sum := sha256.Sum256(data)
	return data, "sha256:" + hex.EncodeToString(sum[:])
}

// TestManagedAppInitDownloadsTemplateAndWritesManifest verifies the P1 fix for
// #67: app init must download the template, verify its digest, extract it to
// --output, and persist both the app.json manifest and managed-release.json.
// It also verifies the security fix: the download/digest/extract happens BEFORE
// the remote App is created, so a bad template never leaves an orphan App.
func TestManagedAppInitDownloadsTemplateAndWritesManifest(t *testing.T) {
	archiveBytes, expectedDigest := templateArchiveFixture(t)
	archiveServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(archiveBytes)
	}))
	defer archiveServer.Close()

	var initCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/managed-apps/templates/image-tool-starter":
			writeCommandJSON(t, writer, map[string]any{
				"name": "image-tool-starter", "version": "1.0.0", "digest": expectedDigest,
				"sdkPackage": "@viceme-ai/app-sdk", "sdkVersion": "0.1.0",
				"downloadUrl": archiveServer.URL,
			})
		case "/v1/managed-apps/runtime-contract/" + testRuntimeReleaseID:
			writeCommandJSON(t, writer, map[string]any{
				"runtimeReleaseId": testRuntimeReleaseID, "contractVersion": "1.0.0",
				"contractDigest": "sha256:" + strings.Repeat("a", 64),
				"inputSchema":    map[string]any{}, "outputSchema": map[string]any{},
				"toolAllowlist": []any{map[string]any{"name": "image-gen", "version": "1.0.0"}},
			})
		case "/v1/managed-apps/apps/init":
			initCalled = true
			writeCommandJSON(t, writer, map[string]any{
				"appId": testAppID, "releaseId": testReleaseID, "candidateId": testCandidateID,
				"status": "DRAFT", "sourceDigest": "sha256:" + strings.Repeat("0", 64),
				"templateName": "image-tool-starter", "templateVersion": "1.0.0",
				"environment": "TEST", "publishableKey": testPublishableKey,
			})
		default:
			http.Error(writer, "not found: "+request.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()

	outDir := filepath.Join(t.TempDir(), "my-app")
	store := securestore.NewMemory()
	dependencies := authenticatedDependencies(t, server, store)
	dependencies.NewID = func() string { return testRuntimeReleaseID }
	code, _, stderr, _ := runCLIWithDependencies(t, server, store, "", dependencies,
		"app", "init",
		"--template", "image-tool-starter",
		"--runtime-release", testRuntimeReleaseID,
		"--name", "My App",
		"--output", outDir)
	if code != 0 || stderr != "" {
		t.Fatalf("app init code=%d stderr=%s", code, stderr)
	}
	for _, expected := range []string{"package.json", "src/app.js", "build.mjs"} {
		if _, err := os.Stat(filepath.Join(outDir, expected)); err != nil {
			t.Fatalf("template file %s was not extracted: %v", expected, err)
		}
	}
	state, err := managedrelease.Load(outDir)
	if err != nil {
		t.Fatalf("managed-release.json not written or invalid: %v", err)
	}
	if state.AppID != testAppID || state.CandidateID != testCandidateID {
		t.Fatalf("managed-release state mismatch: %+v", state)
	}
	if state.RuntimeContractDigest != "sha256:"+strings.Repeat("a", 64) {
		t.Fatalf("contract digest not persisted: %s", state.RuntimeContractDigest)
	}
	if !initCalled {
		t.Fatalf("remote App was never created")
	}
}

// TestManagedAppInitRejectsTamperedTemplate verifies the digest is enforced:
// a mismatched download must abort before extraction AND before the remote App
// is created (no orphan App on the server).
func TestManagedAppInitRejectsTamperedTemplate(t *testing.T) {
	archiveBytes, _ := templateArchiveFixture(t)
	archiveServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		// Append a byte so the downloaded content differs from the expected digest.
		_, _ = writer.Write(append(archiveBytes, 0))
	}))
	defer archiveServer.Close()

	var initCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/managed-apps/templates/image-tool-starter":
			writeCommandJSON(t, writer, map[string]any{
				"name": "image-tool-starter", "version": "1.0.0",
				"digest":     "sha256:" + strings.Repeat("9", 64),
				"sdkPackage": "@viceme-ai/app-sdk", "sdkVersion": "0.1.0",
				"downloadUrl": archiveServer.URL,
			})
		case "/v1/managed-apps/runtime-contract/" + testRuntimeReleaseID:
			writeCommandJSON(t, writer, map[string]any{
				"runtimeReleaseId": testRuntimeReleaseID, "contractVersion": "1.0.0",
				"contractDigest": "sha256:" + strings.Repeat("a", 64),
				"inputSchema":    map[string]any{}, "outputSchema": map[string]any{},
				"toolAllowlist": []any{map[string]any{"name": "image-gen", "version": "1.0.0"}},
			})
		case "/v1/managed-apps/apps/init":
			initCalled = true
			writeCommandJSON(t, writer, map[string]any{
				"appId": testAppID, "releaseId": testReleaseID, "candidateId": testCandidateID,
				"status": "DRAFT", "sourceDigest": "sha256:" + strings.Repeat("0", 64),
				"templateName": "image-tool-starter", "templateVersion": "1.0.0",
				"environment": "TEST", "publishableKey": testPublishableKey,
			})
		default:
			http.Error(writer, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	outDir := t.TempDir()
	store := securestore.NewMemory()
	dependencies := authenticatedDependencies(t, server, store)
	dependencies.NewID = func() string { return testRuntimeReleaseID }
	code, _, stderr, _ := runCLIWithDependencies(t, server, store, "", dependencies,
		"app", "init",
		"--template", "image-tool-starter",
		"--runtime-release", testRuntimeReleaseID,
		"--output", outDir)
	if code == 0 {
		t.Fatalf("app init with a tampered template should fail")
	}
	if !strings.Contains(stderr, "template_digest_mismatch") {
		t.Fatalf("expected digest mismatch error, got: %s", stderr)
	}
	if initCalled {
		t.Fatalf("remote App must NOT be created when the template digest is invalid (orphan App)")
	}
}

// TestManagedAppPreviewUploadsSourceAndArtifactViaMultipart verifies the P1 fix
// for #67: preview must build locally and upload BOTH source and built artifact
// via multipart/form-data (not JSON), and record the returned digests.
func TestManagedAppPreviewUploadsSourceAndArtifactViaMultipart(t *testing.T) {
	var sourceUpload, artifactUpload multipartParts
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/managed-apps/releases/source":
			parts := parseMultipart(t, request)
			sourceUpload = parts
			writeCommandJSON(t, writer, map[string]any{
				"releaseId": testReleaseID, "candidateId": testCandidateID,
				"sourceDigest": "sha256:" + strings.Repeat("5", 64),
			})
		case "/v1/managed-apps/releases/artifact":
			parts := parseMultipart(t, request)
			artifactUpload = parts
			writeCommandJSON(t, writer, map[string]any{
				"releaseId": testReleaseID, "candidateId": testCandidateID,
				"status": "PREVIEW", "buildDigest": "sha256:" + strings.Repeat("6", 64),
			})
		case "/v1/managed-apps/releases/preview":
			writeCommandJSON(t, writer, map[string]any{
				"releaseId": testReleaseID, "candidateId": testCandidateID,
				"status": "PREVIEW", "previewRunId": testRunID,
				"previewUrl": "https://preview.example.com/run",
			})
		default:
			http.Error(writer, "not found: "+request.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()

	project := t.TempDir()
	if _, err := managedrelease.Save(project, managedrelease.State{
		AppID: testAppID, ReleaseID: testReleaseID, CandidateID: testCandidateID,
		Environment: "TEST", PublishableKey: testPublishableKey,
		RuntimeReleaseID:      testRuntimeReleaseID,
		RuntimeContractDigest: "sha256:" + strings.Repeat("a", 64),
		TemplateName:          "image-tool-starter", TemplateVersion: "1.0.0",
		TemplateDigest: "sha256:" + strings.Repeat("1", 64),
		AppSDKVersion:  "0.1.0",
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "dist", "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := securestore.NewMemory()
	dependencies := authenticatedDependencies(t, server, store)
	dependencies.ManagedAppBuilder = func(ctx context.Context, dir string) error { return nil }
	code, _, stderr, _ := runCLIWithDependencies(t, server, store, "", dependencies,
		"app", "preview", "--dir", project, "--yes")
	if code != 0 || stderr != "" {
		t.Fatalf("app preview code=%d stderr=%s", code, stderr)
	}
	if len(sourceUpload.fields) == 0 || sourceUpload.fileField == "" {
		t.Fatalf("source upload was not multipart with a file: %+v", sourceUpload)
	}
	if sourceUpload.fields["appId"] != testAppID || sourceUpload.fields["candidateId"] != testCandidateID {
		t.Fatalf("source upload missing required fields: %+v", sourceUpload.fields)
	}
	if sourceUpload.fields["runtimeContractDigest"] != "sha256:"+strings.Repeat("a", 64) {
		t.Fatalf("source upload did not echo contract digest: %+v", sourceUpload.fields)
	}
	if len(artifactUpload.fields) == 0 || artifactUpload.fileField == "" {
		t.Fatalf("artifact upload was not multipart with a file: %+v", artifactUpload)
	}
	state, err := managedrelease.Load(project)
	if err != nil {
		t.Fatal(err)
	}
	if state.SourceDigest != "sha256:"+strings.Repeat("5", 64) || state.BuildDigest != "sha256:"+strings.Repeat("6", 64) {
		t.Fatalf("digests not persisted after preview: source=%s build=%s", state.SourceDigest, state.BuildDigest)
	}
}

// TestManagedAppPublishRequiresPreview verifies the P1 fix for #67: publish must
// be built on the preview artifacts (it reads digests from managed-release.json
// produced by preview, not from flags).
func TestManagedAppPublishRequiresPreview(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "publish should not be reached", http.StatusBadRequest)
	}))
	defer server.Close()

	project := t.TempDir()
	// State without source/build digests (as if preview was never run).
	if _, err := managedrelease.Save(project, managedrelease.State{
		AppID: testAppID, ReleaseID: testReleaseID, CandidateID: testCandidateID,
		Environment: "TEST", PublishableKey: testPublishableKey,
		RuntimeReleaseID:      testRuntimeReleaseID,
		RuntimeContractDigest: "sha256:" + strings.Repeat("a", 64),
		TemplateName:          "image-tool-starter", TemplateVersion: "1.0.0",
		TemplateDigest: "sha256:" + strings.Repeat("1", 64),
		AppSDKVersion:  "0.1.0",
	}); err != nil {
		t.Fatal(err)
	}

	store := securestore.NewMemory()
	dependencies := authenticatedDependencies(t, server, store)
	code, _, stderr, _ := runCLIWithDependencies(t, server, store, "", dependencies,
		"app", "publish", "--dir", project, "--yes")
	if code == 0 {
		t.Fatalf("publish without preview should fail")
	}
	if !strings.Contains(stderr, "preview_required") {
		t.Fatalf("expected preview_required error, got: %s", stderr)
	}
}

// TestManagedAppPublishEchoesPreviewDigests verifies publish reads the digests
// from managed-release.json (not flags) and confirms before publishing.
func TestManagedAppPublishEchoesPreviewDigests(t *testing.T) {
	var publishBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/managed-apps/releases/publish" {
			http.Error(writer, "not found", http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(request.Body)
		_ = json.Unmarshal(body, &publishBody)
		writeCommandJSON(t, writer, map[string]any{
			"releaseId": testReleaseID, "candidateId": testCandidateID,
			"status": "PUBLISHED", "publishedAt": "2026-08-06T00:00:00Z",
		})
	}))
	defer server.Close()

	project := t.TempDir()
	sourceDigest := "sha256:" + strings.Repeat("5", 64)
	buildDigest := "sha256:" + strings.Repeat("6", 64)
	contractDigest := "sha256:" + strings.Repeat("a", 64)
	if _, err := managedrelease.Save(project, managedrelease.State{
		AppID: testAppID, ReleaseID: testReleaseID, CandidateID: testCandidateID,
		Environment: "TEST", PublishableKey: testPublishableKey,
		RuntimeReleaseID:      testRuntimeReleaseID,
		RuntimeContractDigest: contractDigest,
		TemplateName:          "image-tool-starter", TemplateVersion: "1.0.0",
		TemplateDigest: "sha256:" + strings.Repeat("1", 64),
		AppSDKVersion:  "0.1.0",
		SourceDigest:   sourceDigest, BuildDigest: buildDigest,
	}); err != nil {
		t.Fatal(err)
	}

	store := securestore.NewMemory()
	dependencies := authenticatedDependencies(t, server, store)
	code, _, stderr, _ := runCLIWithDependencies(t, server, store, "", dependencies,
		"app", "publish", "--dir", project, "--yes")
	if code != 0 || stderr != "" {
		t.Fatalf("app publish code=%d stderr=%s", code, stderr)
	}
	if publishBody["expectedSourceDigest"] != sourceDigest {
		t.Fatalf("publish did not echo source digest: %+v", publishBody)
	}
	if publishBody["expectedBuildDigest"] != buildDigest {
		t.Fatalf("publish did not echo build digest: %+v", publishBody)
	}
	if publishBody["expectedRuntimeContractDigest"] != contractDigest {
		t.Fatalf("publish did not echo contract digest: %+v", publishBody)
	}
}

// --- multipart test helpers ---

type multipartParts struct {
	fields    map[string]string
	fileField string
	fileName  string
}

func parseMultipart(t *testing.T, request *http.Request) multipartParts {
	t.Helper()
	if err := request.ParseMultipartForm(50 << 20); err != nil {
		t.Fatalf("failed to parse multipart form: %v", err)
	}
	parts := multipartParts{fields: map[string]string{}}
	for key, values := range request.MultipartForm.Value {
		if len(values) > 0 {
			parts.fields[key] = values[0]
		}
	}
	if request.MultipartForm.File != nil {
		for field, headers := range request.MultipartForm.File {
			if len(headers) > 0 {
				parts.fileField = field
				parts.fileName = headers[0].Filename
			}
		}
	}
	return parts
}

// TestSourceUploadDenylistRejectsSecrets verifies the #67 security fix: the
// managed-app source upload must refuse to package likely-secret files (.env,
// *.pem, id_rsa, credentials) before they can leave the machine.
func TestSourceUploadDenylistRejectsSecrets(t *testing.T) {
	for _, name := range []string{".env", "service.pem", "id_rsa", "credentials.json", ".env.local", "deploy.key"} {
		if !isDeniedSecretFile(name) {
			t.Fatalf("expected %q to be denied", name)
		}
	}
	for _, name := range []string{"package.json", "README.md", "src/app.js", "icon.png"} {
		if isDeniedSecretFile(name) {
			t.Fatalf("expected %q to be allowed", name)
		}
	}

	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "src", "app.js"), []byte("// ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".env"), []byte("SECRET=1"), 0o600); err != nil {
		t.Fatal(err)
	}
	denied, err := findDeniedSecretFiles(project)
	if err != nil {
		t.Fatal(err)
	}
	if len(denied) != 1 || denied[0] != ".env" {
		t.Fatalf("expected only .env to be denied, got %+v", denied)
	}

	// BuildDirectory would normally be reached; assert the guard short-circuits
	// before packaging by checking the upload path returns an error.
	_, err = buildProjectArtifact(context.Background(), project, false)
	if err == nil {
		t.Fatalf("source packaging must fail when a secret file is present")
	}
	if !strings.Contains(err.Error(), "refusing to upload source") {
		t.Fatalf("expected source-secret rejection, got: %v", err)
	}
}

// TestExtractionRejectsZipBomb verifies the #67 extraction-limit fix: a zip with
// too many entries or an oversized entry is rejected before writing to disk.
func TestExtractionRejectsZipBomb(t *testing.T) {
	dir := t.TempDir()

	// Too many entries.
	var buffer bytes.Buffer
	tooMany := zip.NewWriter(&buffer)
	for i := 0; i < 11_000; i++ {
		w, err := tooMany.Create(fmt.Sprintf("f%d", i))
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte("x"))
	}
	if err := tooMany.Close(); err != nil {
		t.Fatal(err)
	}
	if err := extractZipArchive(buffer.Bytes(), dir); err == nil {
		t.Fatalf("expected extraction of too many entries to fail")
	}
}

// Compile-time checks that retain imports for fixtures even when the compiler
// trims unused references in future edits.
var (
	_ = io.ReadAll
	_ = multipart.NewWriter
	_ = api.RuntimeArtifact{}
)
