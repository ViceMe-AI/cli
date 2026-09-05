package replicapreview

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestStartReusesAnExplicitRunningLoopbackService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "ready")
	}))
	defer server.Close()

	session, err := Start(context.Background(), Options{ExistingURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result := session.Result()
	if !result.Reused || result.StartedByCLI || result.TargetURL != server.URL+"/" {
		t.Fatalf("unexpected reuse result: %#v", result)
	}
	if err := session.Wait(context.Background()); err != nil {
		t.Fatalf("reused service should not keep the CLI alive: %v", err)
	}
}

func TestStartRejectsNonLoopbackReuseTargets(t *testing.T) {
	_, err := Start(context.Background(), Options{ExistingURL: "https://example.com"})
	assertStartError(t, err, "REPLICA_PREVIEW_URL_INVALID", StageInspect)
}

func assertStartError(t *testing.T, err error, code string, stage Stage) {
	t.Helper()
	var actual *StartError
	if !errors.As(err, &actual) || actual.Code != code || actual.Stage != stage {
		t.Fatalf("unexpected error: %v", err)
	}
}
func TestMissingURLDoesNotInspectProjectFiles(t *testing.T) {
	for _, name := range []string{"my-reading-corner.html", "app.py", "package.json"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			os.WriteFile(filepath.Join(root, name), []byte("not a node manifest"), 0600)
			_, err := Start(context.Background(), Options{ProjectPath: root})
			assertStartError(t, err, "REPLICA_PREVIEW_URL_REQUIRED", StageInspect)
			files, _ := os.ReadDir(root)
			if len(files) != 1 {
				t.Fatal("source modified")
			}
		})
	}
}
func TestActualPathAndQueryArePreservedAndServiceRemainsRunning(t *testing.T) {
	for _, path := range []string{"/my-reading-corner.html?year=2026", "/python/dashboard", "/custom/start/page"} {
		t.Run(path, func(t *testing.T) {
			var received string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received = r.URL.RequestURI(); w.WriteHeader(200) }))
			defer server.Close()
			session, err := Start(context.Background(), Options{ExistingURL: server.URL + path})
			if err != nil {
				t.Fatal(err)
			}
			if received != path || session.Result().TargetURL != server.URL+path {
				t.Fatal("actual entry lost")
			}
			session.Close()
			if !probeService(context.Background(), server.URL+path) {
				t.Fatal("agent service stopped")
			}
		})
	}
}
func TestUnsafeURLsAndUnavailableService(t *testing.T) {
	for _, target := range []string{"file:///tmp/site.html", "http://user:password@localhost/", "http://127.0.0.1/#fragment", "http://example.com/"} {
		_, err := Start(context.Background(), Options{ExistingURL: target})
		assertStartError(t, err, "REPLICA_PREVIEW_URL_INVALID", StageInspect)
	}
	server := httptest.NewServer(http.NotFoundHandler())
	target := server.URL
	server.Close()
	_, err := Start(context.Background(), Options{ExistingURL: target})
	assertStartError(t, err, "REPLICA_PREVIEW_URL_UNAVAILABLE", StageInspect)
}
