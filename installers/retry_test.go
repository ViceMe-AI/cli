package installers

import (
	"crypto/sha256"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type retryFixture struct {
	root, temporary, destination string
	server                       *httptest.Server
	environment                  []string
	mu                           sync.Mutex
	requests                     map[string]int
}

func newRetryFixture(t *testing.T) *retryFixture {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer; Windows has a native PowerShell process test")
	}
	f := &retryFixture{root: filepath.Join(t.TempDir(), "user space"), requests: map[string]int{}}
	f.temporary = filepath.Join(f.root, "tmp")
	f.destination = filepath.Join(f.root, ".local", "bin", "viceme")
	for _, directory := range []string{f.temporary, filepath.Dir(f.destination)} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeInstallerTestFile(t, f.destination, "previous installation", 0o755)
	f.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests[r.URL.Path]++
		f.mu.Unlock()
		asset := filepath.Base(r.URL.Path)
		if asset == "latest" {
			_, _ = fmt.Fprintln(w, "1.2.3")
			return
		}
		body := `#!/bin/sh
if [ "$VICEME_TEST_ACTIVATION_EXIT" != 0 ]; then
  printf '{"ok":false,"error":{"code":"UPDATE_PERMISSION_REQUIRED"}}\n'
  exit "$VICEME_TEST_ACTIVATION_EXIT"
fi
shift 2
while [ "$#" -gt 0 ]; do
  case "$1" in
    --destination) cp "$0" "$2"; shift 2 ;;
    *) shift ;;
  esac
done
printf '{"ok":true}\n'
`
		if strings.HasSuffix(asset, ".sha256") {
			_, _ = fmt.Fprintf(w, "%x  %s\n", sha256.Sum256([]byte(body)), strings.TrimSuffix(asset, ".sha256"))
		} else {
			_, _ = w.Write([]byte(body))
		}
	}))
	t.Cleanup(f.server.Close)
	certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: f.server.Certificate().Raw})
	certificatePath := filepath.Join(f.root, "ca.pem")
	if err := os.WriteFile(certificatePath, certificate, 0o600); err != nil {
		t.Fatal(err)
	}
	f.environment = append(os.Environ(), "HOME="+f.root, "TMPDIR="+f.temporary,
		"VICEME_INSTALL_DIR="+filepath.Dir(f.destination), "CURL_CA_BUNDLE="+certificatePath,
		"NO_PROXY=127.0.0.1", "no_proxy=127.0.0.1")
	return f
}

func (f *retryFixture) run(exit int, origin, version string) (int, string) {
	command := exec.Command("sh", "./install.sh")
	command.Env = append(append([]string(nil), f.environment...),
		"VICEME_DOWNLOAD_BASE_URL="+f.server.URL+origin, "VICEME_VERSION="+version,
		fmt.Sprintf("VICEME_TEST_ACTIVATION_EXIT=%d", exit))
	output, err := command.CombinedOutput()
	if err != nil {
		if failure, ok := err.(*exec.ExitError); ok {
			return failure.ExitCode(), string(output)
		}
		return -1, err.Error()
	}
	return 0, string(output)
}

func (f *retryFixture) counts() (binary, checksum int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for key, count := range f.requests {
		if strings.HasSuffix(key, ".sha256") {
			checksum += count
		} else if filepath.Base(key) != "latest" {
			binary += count
		}
	}
	return
}

func (f *retryFixture) cached(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(f.temporary, "viceme-bootstrap-cache-*", "*"))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestShellPermissionRetryDownloadsOnceAndPreservesExistingInstall(t *testing.T) {
	f := newRetryFixture(t)
	for i := 0; i < 2; i++ {
		if code, output := f.run(6, "/cn", "1.2.3"); code != 6 {
			t.Fatalf("permission exit lost: code=%d output=%s", code, output)
		}
		if b, err := os.ReadFile(f.destination); err != nil || string(b) != "previous installation" {
			t.Fatalf("permission denial changed the installed generation: %v", err)
		}
	}
	files := f.cached(t)
	if len(files) != 1 {
		t.Fatalf("expected one retained binary, got %v", files)
	}
	info, err := os.Stat(filepath.Dir(files[0]))
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("cache is not private: info=%v err=%v", info, err)
	}
	if code, output := f.run(0, "/cn", "1.2.3"); code != 0 {
		t.Fatalf("authorized retry failed: code=%d output=%s", code, output)
	}
	if binary, checksum := f.counts(); binary != 1 || checksum != 3 {
		t.Fatalf("retry redownloaded bytes or skipped trust verification: binary=%d checksum=%d", binary, checksum)
	}
	if files := f.cached(t); len(files) != 0 {
		t.Fatalf("successful activation left retained downloads: %v", files)
	}
}

func TestShellRetryRejectsCorruptCacheAndExpiresAbandonedDownloads(t *testing.T) {
	for _, scenario := range []string{"corrupt", "expired"} {
		t.Run(scenario, func(t *testing.T) {
			f := newRetryFixture(t)
			if code, output := f.run(6, "/cn", "1.2.3"); code != 6 {
				t.Fatalf("prepare retry: %d %s", code, output)
			}
			files := f.cached(t)
			if len(files) != 1 {
				t.Fatalf("missing retained download: %v", files)
			}
			if scenario == "corrupt" {
				writeInstallerTestFile(t, files[0], "untrusted bytes", 0o600)
			} else if err := os.Chtimes(files[0], time.Now().Add(-72*time.Hour), time.Now().Add(-72*time.Hour)); err != nil {
				t.Fatal(err)
			}
			if code, output := f.run(0, "/cn", "1.2.3"); code != 0 {
				t.Fatalf("verified replacement failed: %d %s", code, output)
			}
			if binary, _ := f.counts(); binary != 2 {
				t.Fatalf("unusable cache was reused: binary requests=%d", binary)
			}
		})
	}
}

func TestShellRetrySeparatesOriginsAndVersions(t *testing.T) {
	f := newRetryFixture(t)
	for _, selection := range [][2]string{{"/cn", "1.2.3"}, {"/global", "1.2.3"}, {"/cn", "1.2.4"}} {
		if code, output := f.run(6, selection[0], selection[1]); code != 6 {
			t.Fatalf("prepare separate release: %d %s", code, output)
		}
	}
	if binary, checksum := f.counts(); binary != 3 || checksum != 3 || len(f.cached(t)) != 3 {
		t.Fatalf("different release identities shared a cache: binary=%d checksum=%d", binary, checksum)
	}
}

func TestShellConcurrentPermissionRetriesUseIndependentVerifiedCopies(t *testing.T) {
	f := newRetryFixture(t)
	if code, output := f.run(6, "/cn", "1.2.3"); code != 6 {
		t.Fatalf("prepare retry: %d %s", code, output)
	}
	var group sync.WaitGroup
	for i := 0; i < 4; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if code, output := f.run(6, "/cn", "1.2.3"); code != 6 {
				t.Errorf("concurrent retry: %d %s", code, output)
			}
		}()
	}
	group.Wait()
	if binary, checksum := f.counts(); binary != 1 || checksum != 5 {
		t.Fatalf("concurrent retry did not reuse verified bytes: binary=%d checksum=%d", binary, checksum)
	}
	if code, output := f.run(9, "/cn", "1.2.3"); code != 9 || len(f.cached(t)) != 0 {
		t.Fatalf("other failure was hidden or retained a download: %d %s", code, output)
	}
}

func TestShellInstallerRefusesSymlinkCacheRoot(t *testing.T) {
	f := newRetryFixture(t)
	outside := filepath.Join(f.root, "unrelated")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	writeInstallerTestFile(t, filepath.Join(outside, "sentinel"), "untouched", 0o600)
	cache := filepath.Join(f.temporary, fmt.Sprintf("viceme-bootstrap-cache-%d", os.Getuid()))
	if err := os.Symlink(outside, cache); err != nil {
		t.Fatal(err)
	}
	if code, output := f.run(0, "/cn", "1.2.3"); code == 0 || !strings.Contains(output, "Unsafe ViceMe download cache") {
		t.Fatalf("unsafe cache accepted: %d %s", code, output)
	}
	if b, err := os.ReadFile(filepath.Join(outside, "sentinel")); err != nil || string(b) != "untouched" {
		t.Fatalf("followed cache symlink: %v", err)
	}
}

func TestShellRetryToleratesConcurrentCacheRemoval(t *testing.T) {
	f := newRetryFixture(t)
	if code, output := f.run(6, "/cn", "1.2.3"); code != 6 {
		t.Fatalf("prepare retry: %d %s", code, output)
	}
	// Deterministically simulate a successful peer removing the entry between
	// this installer's existence check and copy; activation must still verify.
	wrappers := filepath.Join(f.root, "wrappers")
	if err := os.Mkdir(wrappers, 0o700); err != nil {
		t.Fatal(err)
	}
	copyCommand, err := exec.LookPath("cp")
	if err != nil {
		t.Fatal(err)
	}
	writeInstallerTestFile(t, filepath.Join(wrappers, "cp"), `#!/bin/sh
case "$1" in
  */viceme-bootstrap-cache-*/*) rm -f -- "$1" ;;
esac
exec "$VICEME_TEST_REAL_CP" "$@"
`, 0o755)
	f.environment = append(f.environment, "PATH="+wrappers+string(os.PathListSeparator)+os.Getenv("PATH"), "VICEME_TEST_REAL_CP="+copyCommand)
	if code, output := f.run(0, "/cn", "1.2.3"); code != 0 {
		t.Fatalf("concurrent cache cleanup prevented installation: %d %s", code, output)
	}
	if binary, checksum := f.counts(); binary != 2 || checksum != 2 {
		t.Fatalf("missing cache did not use verified acquisition: binary=%d checksum=%d", binary, checksum)
	}
}
