package command

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestGithubPublicationAuthorizesOnlyWhenRequiredAndBoundsRetry(t *testing.T) {
	for _, testCase := range []struct {
		name, firstError, expectedError  string
		persistent                       bool
		archiveCalls, authorizationCalls int32
	}{
		{name: "public", archiveCalls: 1},
		{name: "private", firstError: "GITHUB_SOURCE_AUTHORIZATION_REQUIRED", archiveCalls: 2, authorizationCalls: 1},
		{name: "reauthorize", firstError: "GITHUB_SOURCE_REAUTHORIZATION_REQUIRED", archiveCalls: 2, authorizationCalls: 1},
		{name: "still-denied", firstError: "GITHUB_SOURCE_REAUTHORIZATION_REQUIRED", expectedError: "GITHUB_SOURCE_REAUTHORIZATION_REQUIRED", persistent: true, archiveCalls: 2, authorizationCalls: 1},
		{name: "awaiting-handover", firstError: "ACCOUNT_HANDOVER_REQUIRED", expectedError: "ACCOUNT_HANDOVER_REQUIRED", archiveCalls: 1},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			archive := downloadableSkillArchive(t)
			digest := fmt.Sprintf("%x", sha256.Sum256(archive))
			var archives, authorizations, statuses, unexpected atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/v1/cli/skill-sources/github/archive":
					attempt := archives.Add(1)
					if testCase.firstError != "" && (attempt == 1 || testCase.persistent) {
						writer.Header().Set("Content-Type", "application/json")
						writer.WriteHeader(http.StatusConflict)
						writeJSONResponse(writer, map[string]any{"statusCode": 409, "code": testCase.firstError, "message": "Private source unavailable", "requestId": "test-source"})
						return
					}
					writer.Header().Set("Content-Type", "application/zip")
					writer.Header().Set("X-ViceMe-Github-Private", fmt.Sprint(testCase.firstError != ""))
					writer.Header().Set("X-ViceMe-Github-Commit", strings.Repeat("a", 40))
					writer.Header().Set("X-ViceMe-Github-Owner-Subject", "42")
					writer.Header().Set("X-ViceMe-Github-Repository", "someone/example")
					writer.Header().Set("X-ViceMe-Source-Receipt", "77777777-7777-4777-8777-777777777777")
					writer.Header().Set("X-ViceMe-Package-Digest", digest)
					_, _ = writer.Write(archive)
				case "/v1/cli/skill-sources/github/start":
					authorizations.Add(1)
					writeJSONResponse(writer, map[string]any{"kind": "authorization", "authorizationUrl": "https://github.example/authorize", "attemptId": "88888888-8888-4888-8888-888888888888"})
				case "/v1/cli/skill-sources/github/status":
					statuses.Add(1)
					writeJSONResponse(writer, map[string]any{"kind": "authorized"})
				default:
					unexpected.Add(1)
					http.NotFound(writer, request)
				}
			}))
			defer server.Close()
			_, runtime, err := NewRoot(Dependencies{
				Out: io.Discard, ErrOut: io.Discard, Store: securestore.NewMemory(),
				HTTPClient: server.Client(), APIBaseURL: server.URL, Region: config.RegionCN,
				Environment: skillcontent.Environment{Home: t.TempDir(), ConfigDir: t.TempDir()},
				Sleep:       func(context.Context, time.Duration) error { return nil },
			})
			if err != nil {
				t.Fatal(err)
			}
			runtime.processCredential = &publicationCredential{raw: "vme_cli_" + strings.Repeat("t", 43)}
			pkg, source, _, err := resolveSkillPublicationPackage(context.Background(), runtime,
				"44444444-4444-4444-8444-444444444444", "", "someone/example", "HEAD", "", "", "", "basic", "Basic", 0, nil)
			if testCase.expectedError != "" {
				if err == nil || output.AsError(err).Subtype != testCase.expectedError {
					t.Fatalf("expected %s, got %v", testCase.expectedError, err)
				}
			} else if err != nil {
				t.Fatal(err)
			} else if pkg.Artifact.Digest != digest || source.Ref != strings.Repeat("a", 40) || source.SourceReceiptID == "" || source.Private == nil || *source.Private != (testCase.firstError != "") {
				t.Fatalf("publication lost immutable source provenance: %#v", source)
			}
			if archives.Load() != testCase.archiveCalls || authorizations.Load() != testCase.authorizationCalls || statuses.Load() != testCase.authorizationCalls || unexpected.Load() != 0 {
				t.Fatalf("unexpected publication requests: archive=%d authorize=%d status=%d other=%d", archives.Load(), authorizations.Load(), statuses.Load(), unexpected.Load())
			}
		})
	}
}
