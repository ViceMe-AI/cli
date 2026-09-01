package command

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/config"
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
	expectedDigest := "sha256:" + hex.EncodeToString(digest[:])

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
			if input["clientRequestId"] != requestID || input["workId"] != workID || input["title"] != "Replica title" || input["summary"] != "Replica summary" || input["fileName"] != "source.zip" || input["digest"] != expectedDigest || input["sizeBytes"] != float64(len(archive)) || input["priceCents"] != float64(990) {
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
			writeJSONResponse(writer, map[string]any{"replicaId": replicaID, "shortCode": shortCode})
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
		"--title", "Replica title", "--summary", "Replica summary", "--price-cents", "990",
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
			ReplicaID   string `json:"replicaId"`
			ReplicaCode string `json:"replicaCode"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid command output: %v: %s", err, stdout.String())
	}
	if !envelope.OK || envelope.Data.ReplicaID != replicaID || envelope.Data.ReplicaCode != "VICEME-REPLICA:"+shortCode {
		t.Fatalf("unexpected publication output: %#v", envelope)
	}
	for _, output := range []string{stdout.String(), stderr.String()} {
		if strings.Contains(output, accessToken) || strings.Contains(output, objectServer.URL) || strings.Contains(output, "upload-capability") {
			t.Fatalf("publication output leaked a capability: %q", output)
		}
	}
	assertReplicaSecretsAbsentFromFiles(t, root, accessToken, objectServer.URL, "upload-capability")
}

func replicaTestZIP(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range files {
		entry, err := writer.Create(name)
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
