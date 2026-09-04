//go:build !windows

package replicapreview

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	orphanHelperRoleEnvironment   = "VICEME_PREVIEW_ORPHAN_HELPER_ROLE"
	orphanHelperMarkerEnvironment = "VICEME_PREVIEW_ORPHAN_HELPER_MARKER"
)

func TestOwnedSessionClosesDescendantsAfterTheDevScriptParentExits(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "child-ready")
	command := exec.Command(os.Args[0], "-test.run", "^TestReplicaPreviewOrphanProcessHelper$")
	command.Env = withPreviewTestEnvironment(os.Environ(), map[string]string{
		orphanHelperRoleEnvironment:   "parent",
		orphanHelperMarkerEnvironment: marker,
	})
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	configureProcess(command)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL) })
	session := newOwnedSession(command)

	waitForPreviewTest(t, 3*time.Second, func() bool {
		_, err := os.Stat(marker)
		return err == nil
	})
	select {
	case <-session.done:
	case <-time.After(3 * time.Second):
		t.Fatal("dev script parent did not exit")
	}
	if err := syscall.Kill(-command.Process.Pid, 0); err != nil {
		t.Fatalf("fixture did not leave a descendant in the owned process group: %v", err)
	}

	if err := session.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	waitForPreviewTest(t, 3*time.Second, func() bool {
		return errors.Is(syscall.Kill(-command.Process.Pid, 0), syscall.ESRCH)
	})
}

func TestReplicaPreviewOrphanProcessHelper(t *testing.T) {
	switch os.Getenv(orphanHelperRoleEnvironment) {
	case "parent":
		child := exec.Command(os.Args[0], "-test.run", "^TestReplicaPreviewOrphanProcessHelper$")
		child.Env = withPreviewTestEnvironment(os.Environ(), map[string]string{
			orphanHelperRoleEnvironment: "child",
		})
		child.Stdout = io.Discard
		child.Stderr = io.Discard
		if err := child.Start(); err != nil {
			os.Exit(91)
		}
		marker := os.Getenv(orphanHelperMarkerEnvironment)
		deadline := time.Now().Add(2 * time.Second)
		for {
			if _, err := os.Stat(marker); err == nil {
				os.Exit(0)
			}
			if time.Now().After(deadline) {
				os.Exit(92)
			}
			time.Sleep(10 * time.Millisecond)
		}
	case "child":
		signal.Ignore(syscall.SIGHUP, syscall.SIGTERM)
		if err := os.WriteFile(os.Getenv(orphanHelperMarkerEnvironment), []byte("ready"), 0o600); err != nil {
			os.Exit(93)
		}
		for {
			time.Sleep(time.Hour)
		}
	}
}

func withPreviewTestEnvironment(environment []string, values map[string]string) []string {
	result := make([]string, 0, len(environment)+len(values))
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		if _, replaced := values[name]; !replaced {
			result = append(result, entry)
		}
	}
	for name, value := range values {
		result = append(result, name+"="+value)
	}
	return result
}

func waitForPreviewTest(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition did not become true before timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
