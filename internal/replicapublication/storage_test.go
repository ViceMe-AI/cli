package replicapublication

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/privatefile"
	"github.com/ViceMe-AI/cli/internal/replicacontent"
)

func TestStorageProbeReportsDeniedOperationWithoutPrivatePaths(t *testing.T) {
	for _, operation := range []string{"rename", "replace"} {
		t.Run(operation, func(t *testing.T) {
			dir := t.TempDir()
			original := privatefile.ReplaceFile
			calls := 0
			privatefile.ReplaceFile = func(from, to string) error {
				calls++
				if operation == "rename" || calls == 2 {
					return &os.LinkError{Op: "rename", Old: from, New: to, Err: syscall.EPERM}
				}
				return original(from, to)
			}
			t.Cleanup(func() { privatefile.ReplaceFile = original })
			err := ProbeDirectory(dir)
			failure := output.AsError(err)
			if failure == nil || failure.Subtype != "REPLICA_PUBLICATION_STORAGE_PERMISSION_REQUIRED" {
				t.Fatalf("unexpected failure: %v", err)
			}
			details := failure.Details.(map[string]any)
			if details["operation"] != operation || details["reason"] != "PERMISSION_DENIED" {
				t.Fatalf("lost failure stage: %#v", details)
			}
			encoded, _ := json.Marshal(failure)
			if bytes.Contains(encoded, []byte(dir)) {
				t.Fatal("diagnostics leaked private path")
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 0 {
				t.Fatalf("probe leaked files when cleanup allowed: %v", err)
			}
		})
	}
}

func TestProjectStorageProtectsGitAndProbePreservesExistingState(t *testing.T) {
	dir := t.TempDir()
	store := Store{Directory: filepath.Join(dir, "scope"), EndpointOrigin: "https://api.viceme.cn", Market: "CN", ProjectScoped: true}
	if err := store.Preflight(); err != nil {
		t.Fatal(err)
	}
	ignore, err := os.ReadFile(filepath.Join(store.Directory, ".gitignore"))
	if err != nil || string(ignore) != "*\n" {
		t.Fatal("project recovery state is not ignored by Git")
	}
	original := []byte("existing state must survive preflight")
	target := filepath.Join(store.Directory, "pending-existing.json")
	if err := os.WriteFile(target, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.Preflight(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("preflight mutated existing state")
	}
	entries, _ := os.ReadDir(store.Directory)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".publication-probe-") {
			t.Fatal("preflight retained probe")
		}
	}
}

func TestExpiredCleanupSkipsActivePublicationLock(t *testing.T) {
	now := time.Now()
	project := newPublicationProject(t)
	frozen, err := replicacontent.FreezeSourceArchive(project, replicacontent.FreezeSourceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer frozen.Cleanup()
	store, pending := publicationStoreFixture(t, project, frozen.Summary, now)
	pending.ArtifactExpiresAt = now.Add(-time.Minute)
	if err := store.SaveArtifact(pending.ClientRequestID, frozen); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(&pending); err != nil {
		t.Fatal(err)
	}
	lock, err := store.Lock(pending.ProjectFingerprint)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- store.CleanupExpiredArtifacts() }()
	select {
	case err := <-done:
		if err != nil {
			_ = lock.Unlock()
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		_ = lock.Unlock()
		<-done
		t.Fatal("cleanup waited on active publication")
	}
	_, statErr := os.Stat(store.artifactFilename(pending.ClientRequestID))
	if err := lock.Unlock(); err != nil {
		t.Fatal(err)
	}
	if statErr != nil {
		t.Fatal("cleanup deleted an active artifact")
	}
	if err := store.CleanupExpiredArtifacts(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.artifactFilename(pending.ClientRequestID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("idle expired artifact was not cleaned")
	}
}
