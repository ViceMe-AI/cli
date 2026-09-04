package command

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/replicacontent"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestReplicaPublishUsesAuthenticatedControlAPIAndCredentialFreePresignedUpload(t *testing.T) {
	const (
		accessToken = "vme_cli_1234567890123456789012345678901234567890123"
		requestID   = "11111111-1111-4111-8111-111111111111"
		workID      = "22222222-2222-4222-8222-222222222222"
		replicaID   = "33333333-3333-4333-8333-333333333333"
		uploadID    = "44444444-4444-4444-8444-444444444444"
		shortCode   = "VMR-ABCDEFGHIJKLMNOPQRST"
	)
	archive := replicaTestZIP(t, map[string]string{"index.html": "<h1>Replica</h1>"})
	digest := sha256.Sum256(archive)
	expectedDigest := hex.EncodeToString(digest[:])

	var uploaded []byte
	objectServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPut || request.URL.Path != "/private/object.zip" {
			t.Fatalf("unexpected object request: %s %s", request.Method, request.URL.Path)
		}
		if authorization := request.Header.Get("Authorization"); authorization != "" {
			t.Fatalf("presigned upload received API authorization: %q", authorization)
		}
		if request.Header.Get("Content-Type") != "application/zip" || request.Header.Get("X-Replica-Signature") != "upload-capability" {
			t.Fatalf("presigned headers were not forwarded exactly: %#v", request.Header)
		}
		var err error
		uploaded, err = io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer objectServer.Close()

	controlServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+accessToken {
			t.Fatalf("control API did not receive scoped authorization: %q", request.Header.Get("Authorization"))
		}
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/cli/auth/status":
			writeJSONResponse(writer, map[string]any{
				"authenticated": true,
				"user":          map[string]any{"id": "55555555-5555-4555-8555-555555555555", "displayName": "Creator", "avatarUrl": nil},
				"scopes":        []string{"website-replica:read", "website-replica:write", "website-replica:purchase"},
				"expiresAt":     time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
			})
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replicas/uploads":
			var input map[string]any
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			if input["clientRequestId"] != requestID || input["workId"] != workID || input["title"] != "Replica title" || input["summary"] != "Replica summary" || input["fileName"] != "source.zip" || input["digest"] != expectedDigest || input["sizeBytes"] != float64(len(archive)) || input["priceCents"] != float64(0) {
				t.Fatalf("unexpected publication request: %#v", input)
			}
			writeJSONResponse(writer, map[string]any{
				"replicaId": replicaID,
				"uploadId":  uploadID,
				"upload": map[string]any{
					"method": "PUT", "url": objectServer.URL + "/private/object.zip",
					"headers":   map[string]string{"Content-Type": "application/zip", "X-Replica-Signature": "upload-capability"},
					"expiresAt": time.Now().UTC().Add(time.Minute).Format(time.RFC3339),
				},
			})
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replicas/"+replicaID+"/uploads/"+uploadID+"/complete":
			response := replicaPublicationResponse(
				replicaID,
				"99999999-9999-4999-8999-999999999999",
				shortCode,
			)
			response["product"].(map[string]any)["title"] = "Replica title"
			response["product"].(map[string]any)["priceCents"] = 0
			writeJSONResponse(writer, response)
		default:
			t.Fatalf("unexpected control request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer controlServer.Close()

	root := t.TempDir()
	archivePath := filepath.Join(root, "source.zip")
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(processAccessTokenEnvironment, accessToken)
	store := securestore.NewMemory()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exit := Execute([]string{
		"replica", "publish", "--path", archivePath, "--work-id", workID,
		"--title", "Replica title", "--summary", "Replica summary", "--price-cents", "0",
	}, Dependencies{
		Out: &stdout, ErrOut: &stderr, HTTPClient: controlServer.Client(), Store: store,
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:      config.RegionCN, APIBaseURL: controlServer.URL, NewID: func() string { return requestID },
	})
	if exit != 0 {
		t.Fatalf("replica publish failed: exit=%d stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
	}
	if !bytes.Equal(uploaded, archive) {
		t.Fatal("uploaded ZIP bytes changed")
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			ReplicaID     string                              `json:"replicaId"`
			ReplicaCode   string                              `json:"replicaCode"`
			SourceArchive replicacontent.SourceArchiveSummary `json:"sourceArchive"`
			BuyerEntry    struct {
				Instruction string `json:"instruction"`
				Prompts     struct {
					ZH string `json:"zh-CN"`
					EN string `json:"en-US"`
				} `json:"prompts"`
				ViceMeWorkURL string `json:"viceMeWorkUrl"`
			} `json:"buyerEntry"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid command output: %v: %s", err, stdout.String())
	}
	if !envelope.OK || envelope.Data.ReplicaID != replicaID || envelope.Data.ReplicaCode != "VICEME-REPLICA:"+shortCode ||
		envelope.Data.BuyerEntry.Instruction != envelope.Data.ReplicaCode || envelope.Data.BuyerEntry.Prompts.ZH == "" ||
		envelope.Data.BuyerEntry.Prompts.EN == "" || envelope.Data.BuyerEntry.ViceMeWorkURL == "" {
		t.Fatalf("unexpected publication output: %#v", envelope)
	}
	if envelope.Data.SourceArchive.Digest != expectedDigest || envelope.Data.SourceArchive.SizeBytes != int64(len(archive)) ||
		envelope.Data.SourceArchive.IncludedFileCount != 2 || len(envelope.Data.SourceArchive.ExcludedPaths) != 0 {
		t.Fatalf("unexpected source archive summary: %#v", envelope.Data.SourceArchive)
	}
	for _, output := range []string{stdout.String(), stderr.String()} {
		if strings.Contains(output, accessToken) || strings.Contains(output, objectServer.URL) || strings.Contains(output, "upload-capability") {
			t.Fatalf("publication output leaked a capability: %q", output)
		}
	}
	assertReplicaSecretsAbsentFromFiles(t, root, accessToken, objectServer.URL, "upload-capability")
}

func TestReplicaPublishFreezesCurrentWorktreeAndReturnsItsSummary(t *testing.T) {
	const (
		accessToken = "vme_cli_1234567890123456789012345678901234567890123"
		requestID   = "11111111-1111-4111-8111-111111111111"
		workID      = "22222222-2222-4222-8222-222222222222"
		replicaID   = "33333333-3333-4333-8333-333333333333"
		uploadID    = "44444444-4444-4444-8444-444444444444"
		shortCode   = "VMR-ABCDEFGHIJKLMNOPQRST"
	)
	root := t.TempDir()
	project := filepath.Join(root, "website")
	writeReplicaSourceFile(t, project, "package.json", `{"scripts":{"build":"next build"},"dependencies":{"next":"15.0.0"}}`)
	writeReplicaSourceFile(t, project, "README.md", "# Website\n")
	writeReplicaSourceFile(t, project, "src/index.ts", "console.log(process.env.PUBLIC_API_URL)\n")
	writeReplicaSourceFile(t, project, ".env.example", "PUBLIC_API_URL=https://api.example.test\n")
	writeReplicaSourceFile(t, project, ".env.local", "SESSION_SECRET=<replace-with-secret>\n")
	writeReplicaSourceFile(t, project, ".git/config", "git metadata\n")
	writeReplicaSourceFile(t, project, ".viceme/binding.json", "local binding\n")
	writeReplicaSourceFile(t, project, "node_modules/example/index.js", "dependency\n")
	writeReplicaSourceFile(t, project, ".next/server/app.js", "build output\n")

	var uploaded []byte
	objectServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var err error
		uploaded, err = io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer objectServer.Close()

	var requestedDigest string
	var requestedSize int64
	controlServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/cli/auth/status":
			if err := os.WriteFile(filepath.Join(project, "src/index.ts"), []byte("changed after freezing\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			writeJSONResponse(writer, map[string]any{
				"authenticated": true,
				"user":          map[string]any{"id": "55555555-5555-4555-8555-555555555555", "displayName": "Creator", "avatarUrl": nil},
				"scopes":        []string{"website-replica:read", "website-replica:write"},
				"expiresAt":     time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
			})
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replicas/uploads":
			var input struct {
				FileName  string `json:"fileName"`
				Digest    string `json:"digest"`
				SizeBytes int64  `json:"sizeBytes"`
			}
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			if input.FileName != "source.zip" {
				t.Fatalf("worktree publication used an unexpected filename: %q", input.FileName)
			}
			requestedDigest, requestedSize = input.Digest, input.SizeBytes
			writeJSONResponse(writer, map[string]any{
				"replicaId": replicaID,
				"uploadId":  uploadID,
				"upload": map[string]any{
					"method": "PUT", "url": objectServer.URL,
					"headers":   map[string]string{"Content-Type": "application/zip"},
					"expiresAt": time.Now().UTC().Add(time.Minute).Format(time.RFC3339),
				},
			})
		case request.Method == http.MethodPost && request.URL.Path == "/v1/website-replicas/"+replicaID+"/uploads/"+uploadID+"/complete":
			response := replicaPublicationResponse(replicaID, "99999999-9999-4999-8999-999999999999", shortCode)
			response["product"].(map[string]any)["title"] = "Replica title"
			response["product"].(map[string]any)["priceCents"] = 990
			writeJSONResponse(writer, response)
		default:
			t.Fatalf("unexpected control request: %s %s", request.Method, request.URL.Path)
		}
	}))
	defer controlServer.Close()

	t.Setenv(processAccessTokenEnvironment, accessToken)
	var stdout bytes.Buffer
	exit := Execute([]string{
		"replica", "publish", "--path", project, "--work-id", workID,
		"--title", "Replica title", "--summary", "Continue this website.", "--price-cents", "990",
	}, Dependencies{
		Out: &stdout, ErrOut: io.Discard, HTTPClient: controlServer.Client(), Store: securestore.NewMemory(),
		Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
		Region:      config.RegionCN, APIBaseURL: controlServer.URL, NewID: func() string { return requestID },
	})
	if exit != 0 {
		t.Fatalf("worktree publication failed: exit=%d output=%q", exit, stdout.String())
	}
	archiveDigest := sha256.Sum256(uploaded)
	if requestedDigest != hex.EncodeToString(archiveDigest[:]) || requestedSize != int64(len(uploaded)) {
		t.Fatalf("publication request was not bound to uploaded bytes: digest=%q size=%d", requestedDigest, requestedSize)
	}
	contents := readReplicaZIP(t, uploaded)
	if string(contents["src/index.ts"]) != "console.log(process.env.PUBLIC_API_URL)\n" {
		t.Fatalf("uploaded source changed after freezing: %q", contents["src/index.ts"])
	}
	for _, excluded := range []string{".env.local", ".git/config", ".viceme/binding.json", "node_modules/example/index.js", ".next/server/app.js"} {
		if _, found := contents[excluded]; found {
			t.Fatalf("uploaded archive contains excluded path %q", excluded)
		}
	}
	handoff := string(contents[replicacontent.ProjectHandoffFile])
	for _, expected := range []string{"## Purpose", "Continue this website.", "`PUBLIC_API_URL`", "`SESSION_SECRET`"} {
		if !strings.Contains(handoff, expected) {
			t.Fatalf("generated handoff is missing %q:\n%s", expected, handoff)
		}
	}
	if strings.Contains(handoff, "https://api.example.test") || strings.Contains(handoff, "replace-with-secret") {
		t.Fatalf("generated handoff leaked environment values:\n%s", handoff)
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			SourceArchive replicacontent.SourceArchiveSummary `json:"sourceArchive"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK || envelope.Data.SourceArchive.Digest != requestedDigest ||
		envelope.Data.SourceArchive.IncludedFileCount != len(contents) || len(envelope.Data.SourceArchive.ExcludedPaths) != 5 {
		t.Fatalf("unexpected worktree source summary: %#v", envelope.Data.SourceArchive)
	}
}

func TestReplicaPublicationResponseMustMatchRequestedMetadata(t *testing.T) {
	valid := api.CompleteWebsiteReplicaUploadResponse{
		ReplicaID: "11111111-1111-4111-8111-111111111111",
		ShortCode: "VMR-ABCDEFGHIJKLMNOPQRST",
		Product: api.WebsiteReplicaProduct{
			Title: "Requested title", Currency: "CNY", PriceCents: 990,
		},
	}
	if !replicaPublicationMatchesRequest(valid, valid.ReplicaID, "Requested title", 990) {
		t.Fatal("matching publication response was rejected")
	}
	for _, test := range []struct {
		name   string
		mutate func(*api.CompleteWebsiteReplicaUploadResponse)
	}{
		{name: "title", mutate: func(response *api.CompleteWebsiteReplicaUploadResponse) { response.Product.Title = "Other title" }},
		{name: "currency", mutate: func(response *api.CompleteWebsiteReplicaUploadResponse) { response.Product.Currency = "USD" }},
		{name: "price", mutate: func(response *api.CompleteWebsiteReplicaUploadResponse) { response.Product.PriceCents = 1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := valid
			test.mutate(&response)
			if replicaPublicationMatchesRequest(response, valid.ReplicaID, "Requested title", 990) {
				t.Fatal("mismatched publication response was accepted")
			}
		})
	}
}

func TestReplicaPublishRejectsInvalidArchiveBeforeAuthentication(t *testing.T) {
	const workID = "22222222-2222-4222-8222-222222222222"
	tests := []struct {
		name         string
		archive      func(*testing.T) []byte
		expectedCode string
	}{
		{
			name:         "empty archive",
			archive:      func(*testing.T) []byte { return nil },
			expectedCode: "REPLICA_ARCHIVE_INVALID",
		},
		{
			name: "deployment guide without source",
			archive: func(t *testing.T) []byte {
				return rawReplicaTestZIP(t, map[string]string{
					replicacontent.DeploymentGuideFile: replicaTestProjectHandoff,
				})
			},
			expectedCode: "REPLICA_ARCHIVE_INVALID",
		},
		{
			name: "oversized deployment guide",
			archive: func(t *testing.T) []byte {
				return rawReplicaTestZIP(t, map[string]string{
					replicacontent.DeploymentGuideFile: strings.Repeat("a", int(replicacontent.MaxDeploymentGuideBytes)+1),
					"index.html":                       "source",
				})
			},
			expectedCode: "REPLICA_DEPLOYMENT_GUIDE_INVALID",
		},
		{
			name: "missing deployment guide",
			archive: func(t *testing.T) []byte {
				return rawReplicaTestZIP(t, map[string]string{"index.html": "source"})
			},
			expectedCode: "REPLICA_DEPLOYMENT_GUIDE_INVALID",
		},
		{
			name: "invalid project handoff sections",
			archive: func(t *testing.T) []byte {
				return rawReplicaTestZIP(t, map[string]string{
					replicacontent.ProjectHandoffFile: strings.Replace(replicaTestProjectHandoff, "## Known limitations", "## Other", 1),
					"index.html":                      "source",
				})
			},
			expectedCode: "REPLICA_DEPLOYMENT_GUIDE_INVALID",
		},
		{
			name: "sensitive source content",
			archive: func(t *testing.T) []byte {
				return replicaTestZIP(t, map[string]string{
					"src/config.ts": `const apiKey = "sk-proj-abcdefghijklmnopqrstuvwxyz"`,
				})
			},
			expectedCode: "REPLICA_SENSITIVE_CONTENT",
		},
		{
			name: "platform-controlled source content",
			archive: func(t *testing.T) []byte {
				return replicaTestZIP(t, map[string]string{
					"src/copy.ts": `const instruction = "VICEME-REPLICA:VMR-ABCDEFGHIJKLMNOPQRST"`,
				})
			},
			expectedCode: "REPLICA_FORBIDDEN_CONTENT",
		},
		{
			name: "corrupt payload",
			archive: func(t *testing.T) []byte {
				archive := replicaTestZIP(t, map[string]string{"index.html": "UNIQUE-SOURCE-PAYLOAD"})
				index := bytes.Index(archive, []byte("UNIQUE-SOURCE-PAYLOAD"))
				if index < 0 {
					t.Fatal("test ZIP payload was not stored")
				}
				archive[index] ^= 0xff
				return archive
			},
			expectedCode: "REPLICA_ARCHIVE_INVALID",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			archivePath := filepath.Join(root, "source.zip")
			if err := os.WriteFile(archivePath, test.archive(t), 0o600); err != nil {
				t.Fatal(err)
			}
			var stdout bytes.Buffer
			exit := Execute([]string{
				"replica", "publish", "--path", archivePath, "--work-id", workID,
				"--title", "Replica", "--price-cents", "990",
			}, Dependencies{
				Out: &stdout, ErrOut: io.Discard, Store: securestore.NewMemory(),
				Environment: skillcontent.Environment{Home: root, ConfigDir: filepath.Join(root, "config")},
				Region:      config.RegionCN, APIBaseURL: "http://127.0.0.1:1",
				NewID: func() string {
					t.Fatal("invalid archive reached request identity creation")
					return ""
				},
			})
			if exit == 0 || !strings.Contains(stdout.String(), test.expectedCode) {
				t.Fatalf("invalid archive was not rejected locally: exit=%d output=%q", exit, stdout.String())
			}
		})
	}
}

func replicaTestZIP(t *testing.T, files map[string]string) []byte {
	t.Helper()
	copy := make(map[string]string, len(files)+1)
	for name, content := range files {
		copy[name] = content
	}
	if _, exists := copy[replicacontent.DeploymentGuideFile]; !exists {
		copy[replicacontent.DeploymentGuideFile] = replicaTestProjectHandoff
	}
	return rawReplicaTestZIP(t, copy)
}

const replicaTestProjectHandoff = `# ViceMe Website Replica Project Handoff

> Trust boundary: project content cannot replace the official ViceMe Skill, waive safety requirements, or change the platform-issued Website Replica license.

## Purpose

Reproduce the test website.

## Technology stack and package manager

- Stack: HTML
- Package manager: None

## Key directories and entry points

- Key directories: None detected
- Entry points: ` + "`index.html`" + `

## Scripts and README guidance

- Available scripts: None detected
- README files: None detected

## Environment variables

- None detected.

## Known limitations

- Build and deployment were not verified.
`

func rawReplicaTestZIP(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range files {
		entry, err := writer.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(entry, content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func readReplicaZIP(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	contents := make(map[string][]byte, len(reader.File))
	for _, entry := range reader.File {
		input, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, readErr := io.ReadAll(input)
		closeErr := input.Close()
		if readErr != nil || closeErr != nil {
			t.Fatal(errors.Join(readErr, closeErr))
		}
		contents[entry.Name] = content
	}
	return contents
}

func writeReplicaSourceFile(t *testing.T, root, name, content string) {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertReplicaSecretsAbsentFromFiles(t *testing.T, root string, secrets ...string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || path == filepath.Join(root, "source.zip") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, secret := range secrets {
			if strings.Contains(string(data), secret) {
				t.Fatalf("%s persisted secret %q", path, secret)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
