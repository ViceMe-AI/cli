package command

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestReplicaRepairConfirmsOnlyPageAndResumesLostResponse(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(project, "dist"), 0700); err != nil {
		t.Fatal(err)
	}
	html := filepath.Join(project, "dist", "index.html")
	if err := os.WriteFile(html, []byte("<h1>Repaired</h1>"), 0600); err != nil {
		t.Fatal(err)
	}
	publication := replicaPublicationAPIResponse(now, "PUBLISHED_DEGRADED", "ACTIVATED")
	publication["rollback"] = map[string]any{"activePair": replicaVersionPair(replicaPublicationTestRequestID, replicaPublicationTestVersionID, 1, replicaPublicationTestWorkID, nil), "availablePairs": []any{}}
	var original map[string]any
	var repair map[string]any
	creates, uploads, completes := 0, 0, 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/cli/auth/status":
			writeJSONResponse(w, map[string]any{"authenticated": true, "user": map[string]any{"id": replicaPublicationTestCreatorID, "displayName": "Creator", "avatarUrl": nil}, "scopes": []string{"website-replica:read", "website-replica:write"}, "expiresAt": now.Add(time.Hour).Format(time.RFC3339)})
		case r.Method == http.MethodGet:
			writeJSONResponse(w, publication)
		case strings.HasSuffix(r.URL.Path, "/page-repairs"):
			creates++
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if original != nil {
				if !reflect.DeepEqual(original, body) {
					t.Error("retry changed original repair request")
				}
			} else {
				original = body
				page := body["page"].(map[string]any)
				clone := map[string]any{}
				for k, v := range page {
					clone[k] = v
				}
				clone["status"] = "WAITING_UPLOAD"
				clone["verifiedAt"] = nil
				repair = map[string]any{"id": replicaPublicationTestRequestID, "publicationId": replicaPublicationTestID, "clientRequestId": body["clientRequestId"], "status": "WAITING_UPLOAD", "page": clone, "failure": nil, "result": nil, "createdAt": now.Format(time.RFC3339), "updatedAt": now.Format(time.RFC3339)}
			}
			writeJSONResponse(w, repair)
		case strings.HasSuffix(r.URL.Path, "/upload-authorizations"):
			writeJSONResponse(w, map[string]any{"publicationId": replicaPublicationTestID, "repairId": replicaPublicationTestRequestID, "upload": map[string]any{"method": "PUT", "url": server.URL + "/upload", "headers": map[string]string{"Content-Type": "application/zip"}, "expiresAt": now.Add(time.Hour).Format(time.RFC3339)}})
		case r.Method == http.MethodPut && r.URL.Path == "/upload":
			uploads++
			data, _ := io.ReadAll(r.Body)
			contents := readReplicaZIP(t, data)
			if len(contents["viceme-page.json"]) == 0 || len(contents["VICEME-REPLICA.md"]) != 0 {
				t.Error("repair must upload only page artifact")
			}
			if r.Header.Get("Authorization") != "" {
				t.Error("upload leaked bearer token")
			}
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/complete-upload"):
			completes++
			repair["status"] = "PUBLISHED"
			repair["page"].(map[string]any)["status"] = "ACTIVATED"
			repair["page"].(map[string]any)["verifiedAt"] = now.Format(time.RFC3339)
			repair["result"] = map[string]any{"replicaVersionId": replicaPublicationTestVersionID, "pageRelease": map[string]any{"id": replicaPublicationTestWorkID, "version": 2}, "publishedAt": now.Format(time.RFC3339)}
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			conn.Close()
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	t.Setenv(processAccessTokenEnvironment, "vme_cli_1234567890123456789012345678901234567890123")
	run := func(args []string) (int, []byte) {
		var out bytes.Buffer
		code := Execute(args, Dependencies{Out: &out, ErrOut: &bytes.Buffer{}, HTTPClient: server.Client(), Store: securestore.NewMemory(), Environment: skillcontent.Environment{Home: root, ConfigDir: root + "/config"}, Region: config.RegionCN, APIBaseURL: server.URL, Now: func() time.Time { return now }, NewID: func() string { return replicaPublicationTestRequestID }})
		return code, out.Bytes()
	}
	args := []string{"replica", "repair-hosting", "--publication", replicaPublicationTestID, "--path", project}
	code, out := run(args)
	if code != 10 || creates != 0 || uploads != 0 {
		t.Fatalf("preview wrote: %d %s", code, out)
	}
	var response struct {
		Error struct {
			Details struct{ ConfirmArgs []string }
		}
	}
	if err := json.Unmarshal(out, &response); err != nil {
		t.Fatal(err)
	}
	confirmed := response.Error.Details.ConfirmArgs
	code, out = run(confirmed)
	if code == 0 || uploads != 1 || completes != 1 {
		t.Fatalf("lost response reported success: %d %s", code, out)
	}
	code, out = run(confirmed)
	if code != 0 || creates != 2 || uploads != 1 || completes != 1 || !bytes.Contains(out, []byte("HOSTING_REPAIRED")) {
		t.Fatalf("resume failed: %d %s", code, out)
	}
	if err := os.WriteFile(html, []byte("<h1>Changed after review</h1>"), 0600); err != nil {
		t.Fatal(err)
	}
	code, out = run(confirmed)
	if code != 2 || creates != 2 {
		t.Fatalf("changed page was uploaded: %d %s", code, out)
	}
}
