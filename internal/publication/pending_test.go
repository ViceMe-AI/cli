package publication

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		SourcePath: "/tmp/skill", PriceMinor: 1, ArtifactDigest: "b" + fingerprint[1:],
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
