//go:build darwin

package replicapublication

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/replicacontent"
)

// Reproduces the reported Seatbelt operation boundary in a real child process.
// All paths belong to the test. No developer CLI, credentials or HOME is used.
func TestReplicaStorageSeatbelt(t *testing.T) {
	if os.Getenv("VICEME_TEST_SEATBELT_ROOT") != "" {
		testReplicaStorageSeatbeltChild(t)
		return
	}
	sandbox, err := exec.LookPath("sandbox-exec")
	if err != nil {
		t.Skip("sandbox-exec unavailable")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	profile := "(version 1)(allow default)(deny file-write-unlink (subpath " + strconv.Quote(filepath.Join(root, "restricted")) + "))"
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, sandbox, "-p", profile, exe, "-test.run=^TestReplicaStorageSeatbelt$", "-test.v")
	command.Env = append(os.Environ(), "VICEME_TEST_SEATBELT_ROOT="+root)
	result, err := command.CombinedOutput()
	if err != nil && bytes.Contains(result, []byte("sandbox_apply: Operation not permitted")) {
		t.Skip("host cannot create a nested Seatbelt sandbox")
	}
	if err != nil {
		t.Fatalf("Seatbelt regression: %v\n%s", err, result)
	}
	t.Log(strings.TrimSpace(string(result)))
}

func testReplicaStorageSeatbeltChild(t *testing.T) {
	root := os.Getenv("VICEME_TEST_SEATBELT_ROOT")
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "index.html"), []byte("<h1>Seatbelt fixture</h1>"), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	frozen, err := replicacontent.FreezeSourceArchive(project, replicacontent.FreezeSourceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer frozen.Cleanup()
	global, pending := publicationStoreFixture(t, project, frozen.Summary, now)
	global.Directory = filepath.Join(root, "restricted", "state")
	// Original failure: creation and copying succeed; rename is actually denied.
	err = global.SaveArtifact(pending.ClientRequestID, frozen)
	if err == nil || output.AsError(err).Subtype != "REPLICA_PUBLICATION_STORAGE_PERMISSION_REQUIRED" || !errors.Is(err, syscall.EPERM) {
		t.Fatalf("did not reproduce original OS denial: %v", err)
	}
	stages, _ := filepath.Glob(filepath.Join(global.artifactDirectory(pending.ClientRequestID), ".source-*.tmp"))
	if len(stages) != 1 {
		t.Fatal("expected the undeletable copied source reported in the incident")
	}
	data, err := os.ReadFile(stages[0])
	if err != nil || int64(len(data)) != frozen.Summary.SizeBytes {
		t.Fatal("source was not fully copied before rename denial")
	}
	if err := global.Preflight(); err == nil {
		t.Fatal("restricted store passed preflight")
	}
	if err := os.Mkdir(filepath.Join(project, ".viceme"), 0700); err != nil {
		t.Fatal(err)
	}
	directory, err := ProjectStoreDirectory(project, global.EndpointOrigin, global.Market)
	if err != nil {
		t.Fatal(err)
	}
	local := global
	local.Directory = directory
	local.ProjectScoped = true
	if err := local.Preflight(); err != nil {
		t.Fatal(err)
	}
	if err := local.SaveArtifact(pending.ClientRequestID, frozen); err != nil {
		t.Fatal(err)
	}
	if err := local.Save(&pending); err != nil {
		t.Fatal(err)
	}
	loaded, found, err := local.LoadProject(pending.ProjectFingerprint)
	if err != nil || !found || loaded.ClientRequestID != pending.ClientRequestID {
		t.Fatal("project storage lost request")
	}
	artifact, err := local.OpenArtifact(loaded)
	if err != nil {
		t.Fatal(err)
	}
	_ = artifact.Close()
	if err := local.DeleteArtifact(loaded); err != nil {
		t.Fatal(err)
	}
	if err := local.Delete(loaded); err != nil {
		t.Fatal(err)
	}
	t.Log("reproduced restricted rename/unlink; project storage persisted, recovered and removed the same artifact")
}
