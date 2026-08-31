package publication

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/output"
)

func TestPendingAndIntentRoundTripUsesPrivateFiles(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	store := PendingStore{Directory: directory, Now: func() time.Time { return now }}
	fingerprint := strings.Repeat("a", 64)
	id := "11111111-1111-4111-8111-111111111111"
	intent, err := store.LoadOrCreateIntent(fingerprint, func() string { return id })
	if err != nil {
		t.Fatal(err)
	}
	intent.PublicationID = "22222222-2222-4222-8222-222222222222"
	if err := store.SaveIntent(intent); err != nil {
		t.Fatal(err)
	}
	pending := Pending{
		PublicationID: intent.PublicationID, ClientRequestID: intent.ClientRequestID,
		MerchantAccountID: "99999999-9999-4999-8999-999999999999",
		Fingerprint:       fingerprint,
		SourcePath:        "/tmp/skill", PriceMinor: intPointer(1), ArtifactDigest: "b" + fingerprint[1:],
	}
	if err := store.Save(pending); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(pending.PublicationID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.PublicationID != pending.PublicationID || !loaded.CreatedAt.Equal(now) || !loaded.UpdatedAt.Equal(now) {
		t.Fatalf("unexpected pending record: %#v", loaded)
	}
	for _, filename := range []string{store.filename(pending.PublicationID), store.intentFilename(fingerprint)} {
		info, err := os.Stat(filename)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("recovery file is not private: %s mode=%o", filepath.Base(filename), info.Mode().Perm())
		}
	}
	if err := store.Delete(pending.PublicationID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.filename(pending.PublicationID)); !os.IsNotExist(err) {
		t.Fatalf("pending record was not deleted: %v", err)
	}
}

func intPointer(value int) *int { return &value }

func TestIntentConcurrentCreationAndTerminalRetirement(t *testing.T) {
	directory := t.TempDir()
	store := PendingStore{Directory: directory}
	fingerprint := strings.Repeat("c", 64)
	var allocated atomic.Int32
	newID := func() string {
		allocated.Add(1)
		return "33333333-3333-4333-8333-333333333333"
	}
	const workers = 20
	intents := make(chan Intent, workers)
	errors := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			intent, err := store.LoadOrCreateIntent(fingerprint, newID)
			intents <- intent
			errors <- err
		}()
	}
	group.Wait()
	close(intents)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	for intent := range intents {
		if intent.ClientRequestID != "33333333-3333-4333-8333-333333333333" {
			t.Fatalf("concurrent caller observed another intent: %#v", intent)
		}
	}
	if allocated.Load() != 1 {
		t.Fatalf("allocated %d client request IDs, want one", allocated.Load())
	}

	intent, err := store.LoadOrCreateIntent(fingerprint, newID)
	if err != nil {
		t.Fatal(err)
	}
	intent.PublicationID = "44444444-4444-4444-8444-444444444444"
	if err := store.SaveIntent(intent); err != nil {
		t.Fatal(err)
	}
	if err := store.RetireIntent(fingerprint, intent.PublicationID, intent.ClientRequestID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.intentFilename(fingerprint)); !os.IsNotExist(err) {
		t.Fatalf("terminal intent was not retired: %v", err)
	}
}

func TestPendingRejectsCorruptState(t *testing.T) {
	t.Parallel()
	store := PendingStore{Directory: t.TempDir()}
	id := "22222222-2222-4222-8222-222222222222"
	if err := os.WriteFile(store.filename(id), []byte(`{"schemaVersion":1,"publicationId":"../escape"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(id); err == nil {
		t.Fatal("corrupt recovery state was accepted")
	}
}

func TestIntentReportsRecoveryDirectoryPermissionInsteadOfLockFailure(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "publications")
	err := recoveryOperationError(directory, "PUBLICATION_INTENT_LOCK_FAILED", "could not lock publication intent", fs.ErrPermission)
	var cliError *output.Error
	if !errors.As(err, &cliError) {
		t.Fatalf("expected CLI error, got %v", err)
	}
	if cliError.Subtype != "PUBLICATION_RECOVERY_PERMISSION_REQUIRED" || cliError.Type != "policy" {
		t.Fatalf("unexpected error: %#v", cliError)
	}
	if cliError.Subtype == "PUBLICATION_INTENT_LOCK_FAILED" {
		t.Fatal("permission denial was reported as an intent lock failure")
	}
	details, ok := cliError.Details.(map[string]any)
	if !ok || details["directory"] != directory {
		t.Fatalf("missing recovery directory details: %#v", cliError.Details)
	}
	if !strings.Contains(cliError.Hint, "exact same command") || !strings.Contains(cliError.Hint, "do not delete lock files") {
		t.Fatalf("permission recovery hint is not actionable: %q", cliError.Hint)
	}
}
