package replicapublication

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/privatefile"
	"github.com/ViceMe-AI/cli/internal/replicacontent"
)

const (
	testPublicationID = "11111111-1111-4111-8111-111111111111"
	testRequestID     = "22222222-2222-4222-8222-222222222222"
	testMerchantID    = "33333333-3333-4333-8333-333333333333"
	testWorkID        = "44444444-4444-4444-8444-444444444444"
	testReplicaID     = "55555555-5555-4555-8555-555555555555"
	testVersionID     = "66666666-6666-4666-8666-666666666666"
	testProductID     = "77777777-7777-4777-8777-777777777777"
	testSKUID         = "88888888-8888-4888-8888-888888888888"
)

func TestStorePersistsPrivateRecoverableStateAndDetectsArtifactChanges(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	project := newPublicationProject(t)
	frozen, err := replicacontent.FreezeSourceArchive(project, replicacontent.FreezeSourceOptions{ExpiresAt: now.Add(30 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	defer frozen.Cleanup()

	store, pending := publicationStoreFixture(t, project, frozen.Summary, now)
	if err := store.SaveArtifact(pending.ClientRequestID, frozen); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(&pending); err != nil {
		t.Fatal(err)
	}

	stateFilename := store.stateFilename(pending.ProjectFingerprint)
	artifactFilename := store.artifactFilename(pending.ClientRequestID)
	for _, filename := range []string{stateFilename, artifactFilename} {
		assertPrivateFile(t, filename)
	}
	for _, directory := range []string{store.Directory, filepath.Dir(filepath.Dir(artifactFilename)), filepath.Dir(artifactFilename)} {
		assertPrivateDirectory(t, directory)
	}
	stateData, err := os.ReadFile(stateFilename)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"vme_cli_secret", "https://storage.example/source.zip", "X-Upload-Signature"} {
		if bytes.Contains(stateData, []byte(forbidden)) {
			t.Fatalf("recoverable state persisted capability %q: %s", forbidden, stateData)
		}
	}
	loaded, found, err := store.LoadProject(pending.ProjectFingerprint)
	if err != nil || !found || loaded.ClientRequestID != pending.ClientRequestID || loaded.UpdatedAt != now {
		t.Fatalf("LoadProject() = %#v, %v, %v", loaded, found, err)
	}
	artifact, err := store.OpenArtifact(loaded)
	if err != nil {
		t.Fatal(err)
	}
	contents, readErr := io.ReadAll(artifact)
	closeErr := artifact.Close()
	if readErr != nil || closeErr != nil || int64(len(contents)) != frozen.Summary.SizeBytes {
		t.Fatalf("read frozen artifact: bytes=%d readErr=%v closeErr=%v", len(contents), readErr, closeErr)
	}
	if stages, err := filepath.Glob(filepath.Join(filepath.Dir(artifactFilename), ".source-*.tmp")); err != nil || len(stages) != 0 {
		t.Fatalf("artifact save left staging files: files=%v err=%v", stages, err)
	}

	file, err := os.OpenFile(artifactFilename, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteAt([]byte{contents[0] ^ 0xff}, 0); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = store.OpenArtifact(loaded)
	if cliErr := output.AsError(err); err == nil || cliErr.Subtype != "REPLICA_PUBLICATION_ARTIFACT_CHANGED" {
		t.Fatalf("tampered artifact was accepted: %#v", cliErr)
	}
}

func TestStoreRejectsUnknownStateFieldsAndScopesEndpointAndMarket(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	project := newPublicationProject(t)
	summary := replicacontent.SourceArchiveSummary{Digest: strings.Repeat("a", 64), SizeBytes: 1024}
	store, pending := publicationStoreFixture(t, project, summary, now)
	if err := store.Save(&pending); err != nil {
		t.Fatal(err)
	}
	filename := store.stateFilename(pending.ProjectFingerprint)
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	raw["uploadUrl"] = "https://storage.example/source.zip"
	data, err = json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.LoadProject(pending.ProjectFingerprint); err == nil || output.AsError(err).Subtype != "REPLICA_PUBLICATION_STATE_INVALID" {
		t.Fatalf("state with an unknown capability field was accepted: %v", err)
	}
	if err := os.WriteFile(filename, []byte(`{"schemaVersion":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.LoadProject(pending.ProjectFingerprint); err == nil || output.AsError(err).Subtype != "REPLICA_PUBLICATION_STATE_INVALID" {
		t.Fatalf("truncated recovery state was accepted: %v", err)
	}

	base := filepath.Dir(store.Directory)
	cn := ScopedDirectory(base, "https://api.viceme.cn", "CN")
	global := ScopedDirectory(base, "https://api.viceme.ai", "GLOBAL")
	otherEndpoint := ScopedDirectory(base, "https://api.dev.example", "CN")
	if cn == global || cn == otherEndpoint || global == otherEndpoint {
		t.Fatalf("publication stores were not scoped independently: cn=%q global=%q other=%q", cn, global, otherEndpoint)
	}
}

func TestStoreCleansExpiredArtifactsAcrossProjectsWithoutDroppingRequestIdentity(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	expiredProject := newPublicationProject(t)
	expiredFrozen, err := replicacontent.FreezeSourceArchive(expiredProject, replicacontent.FreezeSourceOptions{ExpiresAt: now.Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	defer expiredFrozen.Cleanup()
	store, expired := publicationStoreFixture(t, expiredProject, expiredFrozen.Summary, now)
	expired.ArtifactExpiresAt = now.Add(-time.Minute)
	expired.Confirmation = &api.WebsiteReplicaPublicationConfirmationChallenge{
		Version: "wrv1-" + strings.Repeat("a", 64),
		Review: api.WebsiteReplicaPublicationReview{
			ProjectFingerprint: expired.ProjectFingerprint,
			Source:             expired.Request.Source,
		},
		IssuedAt:  now.Add(-31 * time.Minute).Format(time.RFC3339),
		ExpiresAt: now.Add(-time.Minute).Format(time.RFC3339),
	}
	confirmedAt := now.Add(-30 * time.Minute)
	expired.ConfirmedAt = &confirmedAt
	if err := store.SaveArtifact(expired.ClientRequestID, expiredFrozen); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(&expired); err != nil {
		t.Fatal(err)
	}

	freshProject := newPublicationProject(t)
	freshFrozen, err := replicacontent.FreezeSourceArchive(freshProject, replicacontent.FreezeSourceOptions{ExpiresAt: now.Add(30 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	defer freshFrozen.Cleanup()
	freshFingerprint, freshPath, err := ProjectFingerprint(store.EndpointOrigin, store.Market, freshProject)
	if err != nil {
		t.Fatal(err)
	}
	fresh := expired
	fresh.ProjectPath = freshPath
	fresh.ProjectFingerprint = freshFingerprint
	fresh.ClientRequestID = "99999999-9999-4999-8999-999999999999"
	fresh.Request.ClientRequestID = fresh.ClientRequestID
	fresh.Request.ProjectFingerprint = freshFingerprint
	fresh.Request.Source = api.WebsiteReplicaPublicationSourceArtifact{
		FileName: "source.zip", ContentType: "application/zip",
		SizeBytes: freshFrozen.Summary.SizeBytes, Digest: freshFrozen.Summary.Digest,
	}
	fresh.SourceArchive = freshFrozen.Summary
	fresh.ArtifactExpiresAt = now.Add(30 * time.Minute)
	fresh.Confirmation = nil
	fresh.ConfirmedAt = nil
	if err := store.SaveArtifact(fresh.ClientRequestID, freshFrozen); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(&fresh); err != nil {
		t.Fatal(err)
	}

	if err := store.CleanupExpiredArtifacts(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.artifactFilename(expired.ClientRequestID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired artifact was retained: %v", err)
	}
	loadedExpired, found, err := store.LoadProject(expired.ProjectFingerprint)
	if err != nil || !found {
		t.Fatalf("expired request identity was dropped: found=%v err=%v", found, err)
	}
	if loadedExpired.ClientRequestID != expired.ClientRequestID || loadedExpired.Confirmation != nil || loadedExpired.ConfirmedAt != nil {
		t.Fatalf("expired request was not reset safely: %#v", loadedExpired)
	}
	if _, err := os.Stat(store.artifactFilename(fresh.ClientRequestID)); err != nil {
		t.Fatalf("fresh artifact was removed: %v", err)
	}
}

func TestStoreKeepsConfirmedDraftIdentityAfterArtifactExpires(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	project := newPublicationProject(t)
	frozen, err := replicacontent.FreezeSourceArchive(project, replicacontent.FreezeSourceOptions{ExpiresAt: now.Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	defer frozen.Cleanup()
	store, pending := publicationStoreFixture(t, project, frozen.Summary, now)
	pending.ArtifactExpiresAt = now.Add(-time.Minute)
	pending.Confirmation = &api.WebsiteReplicaPublicationConfirmationChallenge{
		Version: "wrv1-" + strings.Repeat("b", 64),
		Review: api.WebsiteReplicaPublicationReview{
			ProjectFingerprint: pending.ProjectFingerprint, Title: pending.Request.Title,
			Summary: pending.Request.Summary, PriceCents: pending.Request.PriceCents, Source: pending.Request.Source,
		},
		IssuedAt:  now.Add(-31 * time.Minute).Format(time.RFC3339),
		ExpiresAt: now.Add(-time.Minute).Format(time.RFC3339),
	}
	confirmedAt := now.Add(-30 * time.Minute)
	pending.ConfirmedAt = &confirmedAt
	pending.Publication = &PublicationReference{
		ID: testPublicationID, Status: "DRAFT", StatusURL: "https://viceme.cn/me/website-replica-publications/" + testPublicationID,
	}
	if err := store.SaveArtifact(pending.ClientRequestID, frozen); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(&pending); err != nil {
		t.Fatal(err)
	}

	if err := store.CleanupExpiredArtifacts(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.artifactFilename(pending.ClientRequestID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired draft artifact was retained: %v", err)
	}
	loaded, found, err := store.LoadPublication(testPublicationID)
	if err != nil || !found {
		t.Fatalf("confirmed draft identity was dropped: found=%v err=%v", found, err)
	}
	if loaded.Confirmation == nil || loaded.ConfirmedAt == nil || loaded.Publication == nil || loaded.Publication.ID != testPublicationID {
		t.Fatalf("confirmed draft recovery metadata was cleared: %#v", loaded)
	}
}

func TestStoreStateWriteFailsClosedWhenAtomicActivationIsDenied(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	project := newPublicationProject(t)
	summary := replicacontent.SourceArchiveSummary{Digest: strings.Repeat("a", 64), SizeBytes: 1024}
	store, pending := publicationStoreFixture(t, project, summary, now)
	if err := store.Save(&pending); err != nil {
		t.Fatal(err)
	}
	filename := store.stateFilename(pending.ProjectFingerprint)
	before, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	originalReplace := privatefile.ReplaceFile
	privatefile.ReplaceFile = func(string, string) error { return syscall.EPERM }
	t.Cleanup(func() { privatefile.ReplaceFile = originalReplace })
	pending.AutoApplyCreator = true
	err = store.Save(&pending)
	if cliErr := output.AsError(err); err == nil || cliErr.Subtype != "REPLICA_PUBLICATION_STORAGE_PERMISSION_REQUIRED" {
		t.Fatalf("state write did not fail closed: %#v", cliErr)
	}
	after, readErr := os.ReadFile(filename)
	if readErr != nil || !bytes.Equal(after, before) {
		t.Fatalf("failed atomic activation changed state: equal=%v err=%v", bytes.Equal(after, before), readErr)
	}
}

func TestStoreRejectsDraftPublicationWithoutLocalFinalConfirmation(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	project := newPublicationProject(t)
	summary := replicacontent.SourceArchiveSummary{Digest: strings.Repeat("a", 64), SizeBytes: 1024}
	store, pending := publicationStoreFixture(t, project, summary, now)
	pending.Publication = &PublicationReference{
		ID: testPublicationID, Status: "DRAFT", StatusURL: "https://viceme.cn/me/website-replica-publications/" + testPublicationID,
	}
	err := store.Save(&pending)
	if cliErr := output.AsError(err); err == nil || cliErr.Subtype != "REPLICA_PUBLICATION_STATE_INVALID" {
		t.Fatalf("unconfirmed draft publication was persisted: %#v", cliErr)
	}
}

func TestBindingIsAtomicPrivateAndCompletedOnlyAtPublishedState(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	project := newPublicationProject(t)
	summary := replicacontent.SourceArchiveSummary{Digest: strings.Repeat("a", 64), SizeBytes: 1024}
	_, pending := publicationStoreFixture(t, project, summary, now)
	processing := publicationFixture(now, "PROCESSING", "VERIFIED")
	pending.Publication = &PublicationReference{ID: processing.ID, Status: processing.Status, StatusURL: processing.StatusURL}
	pending.TakenOver = true
	bindingStore := BindingStore{EndpointOrigin: pending.EndpointOrigin, Market: pending.Market, Now: func() time.Time { return now }}

	if err := bindingStore.Save(pending.ProjectPath, pending, processing); err != nil {
		t.Fatal(err)
	}
	filename := bindingPath(pending.ProjectPath)
	assertPrivateDirectory(t, filepath.Dir(filename))
	assertPrivateFile(t, filename)
	binding, found, err := bindingStore.Load(pending.ProjectPath)
	if err != nil || !found {
		t.Fatalf("Load() = %#v, %v, %v", binding, found, err)
	}
	if binding.Publication.Status != "PROCESSING" || binding.Merchant.ID != testMerchantID ||
		binding.Work != nil || binding.Replica != nil || binding.Product != nil || binding.Version != nil {
		t.Fatalf("processing binding exposed terminal associations: %#v", binding)
	}

	published := publicationFixture(now, "PUBLISHED", "ACTIVATED")
	pending.Publication = &PublicationReference{ID: published.ID, Status: published.Status, StatusURL: published.StatusURL}
	if err := bindingStore.Save(pending.ProjectPath, pending, published); err != nil {
		t.Fatal(err)
	}
	binding, found, err = bindingStore.Load(pending.ProjectPath)
	if err != nil || !found || binding.Work == nil || binding.Replica == nil || binding.Product == nil || binding.Version == nil {
		t.Fatalf("published binding is incomplete: binding=%#v found=%v err=%v", binding, found, err)
	}
	if binding.Work.ID != testWorkID || binding.Replica.ID != testReplicaID || binding.Product.ID != testProductID ||
		binding.Product.PriceCents != 990 || binding.Version.ID != testVersionID {
		t.Fatalf("published binding has incorrect stable associations: %#v", binding)
	}
	if stages, err := filepath.Glob(filepath.Join(pending.ProjectPath, ".viceme", ".website-replica-*.tmp")); err != nil || len(stages) != 0 {
		t.Fatalf("binding save left staging files: files=%v err=%v", stages, err)
	}
}

func TestBindingForZIPSourceUsesAdjacentProjectStateDirectory(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "source.zip")
	if err := os.WriteFile(sourcePath, []byte("zip fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	summary := replicacontent.SourceArchiveSummary{Digest: strings.Repeat("a", 64), SizeBytes: 1024}
	_, pending := publicationStoreFixture(t, sourcePath, summary, now)
	publication := publicationFixture(now, "PROCESSING", "VERIFIED")
	pending.Publication = &PublicationReference{ID: publication.ID, Status: publication.Status, StatusURL: publication.StatusURL}
	pending.TakenOver = true
	store := BindingStore{EndpointOrigin: pending.EndpointOrigin, Market: pending.Market, Now: func() time.Time { return now }}
	if err := store.Save(pending.ProjectPath, pending, publication); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(directory, ".viceme", bindingFilename)
	assertPrivateDirectory(t, filepath.Dir(filename))
	assertPrivateFile(t, filename)
	binding, found, err := store.Load(pending.ProjectPath)
	if err != nil || !found || binding.ProjectFingerprint != pending.ProjectFingerprint {
		t.Fatalf("ZIP binding load = %#v, %v, %v", binding, found, err)
	}
	if _, err := os.Stat(filepath.Join(sourcePath, ".viceme", bindingFilename)); !errors.Is(err, os.ErrNotExist) && !errors.Is(err, syscall.ENOTDIR) {
		t.Fatalf("binding was written below the ZIP file: %v", err)
	}
}

func TestBindingAcceptsPublishedDatetimeWithoutSeconds(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	project := newPublicationProject(t)
	summary := replicacontent.SourceArchiveSummary{Digest: strings.Repeat("a", 64), SizeBytes: 1024}
	_, pending := publicationStoreFixture(t, project, summary, now)
	publication := publicationFixture(now, "PUBLISHED", "ACTIVATED")
	publication.Result.PublishedAt = "2026-09-04T12:00Z"
	pending.Publication = &PublicationReference{ID: publication.ID, Status: publication.Status, StatusURL: publication.StatusURL}
	pending.TakenOver = true
	store := BindingStore{EndpointOrigin: pending.EndpointOrigin, Market: pending.Market, Now: func() time.Time { return now }}
	if err := store.Save(pending.ProjectPath, pending, publication); err != nil {
		t.Fatalf("valid z.iso.datetime value without seconds was rejected: %v", err)
	}
	binding, found, err := store.Load(pending.ProjectPath)
	if err != nil || !found || binding.Version == nil || !binding.Version.PublishedAt.Equal(now) {
		t.Fatalf("published time was not persisted: binding=%#v found=%v err=%v", binding, found, err)
	}
}

func TestBindingCompletesStableAssociationsForDegradedPublishedState(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	project := newPublicationProject(t)
	summary := replicacontent.SourceArchiveSummary{Digest: strings.Repeat("a", 64), SizeBytes: 1024}
	_, pending := publicationStoreFixture(t, project, summary, now)
	publication := publicationFixture(now, "PUBLISHED_DEGRADED", "ACTIVATED")
	pending.Publication = &PublicationReference{ID: publication.ID, Status: publication.Status, StatusURL: publication.StatusURL}
	pending.TakenOver = true
	store := BindingStore{EndpointOrigin: pending.EndpointOrigin, Market: pending.Market, Now: func() time.Time { return now }}
	if err := store.Save(pending.ProjectPath, pending, publication); err != nil {
		t.Fatal(err)
	}
	binding, found, err := store.Load(pending.ProjectPath)
	if err != nil || !found || binding.Publication.Status != "PUBLISHED_DEGRADED" ||
		binding.Work == nil || binding.Replica == nil || binding.Product == nil || binding.Version == nil {
		t.Fatalf("degraded terminal binding is incomplete: binding=%#v found=%v err=%v", binding, found, err)
	}
}

func TestBindingFailsClosedWhenAtomicRenameIsDenied(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	project := newPublicationProject(t)
	summary := replicacontent.SourceArchiveSummary{Digest: strings.Repeat("a", 64), SizeBytes: 1024}
	_, pending := publicationStoreFixture(t, project, summary, now)
	publication := publicationFixture(now, "PROCESSING", "VERIFIED")
	pending.Publication = &PublicationReference{ID: publication.ID, Status: publication.Status, StatusURL: publication.StatusURL}
	pending.TakenOver = true
	store := BindingStore{EndpointOrigin: pending.EndpointOrigin, Market: pending.Market, Now: func() time.Time { return now }}
	if err := store.Save(pending.ProjectPath, pending, publication); err != nil {
		t.Fatal(err)
	}
	filename := bindingPath(pending.ProjectPath)
	before, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	originalReplace := privatefile.ReplaceFile
	privatefile.ReplaceFile = func(string, string) error { return syscall.EPERM }
	t.Cleanup(func() { privatefile.ReplaceFile = originalReplace })
	publication.Status = "CANCELLED"
	publication.CancelledAt = pointer(now.Format(time.RFC3339))
	err = store.Save(pending.ProjectPath, pending, publication)
	if cliErr := output.AsError(err); err == nil || cliErr.Subtype != "REPLICA_BINDING_PERMISSION_REQUIRED" {
		t.Fatalf("binding used a non-atomic fallback: %#v", cliErr)
	}
	after, readErr := os.ReadFile(filename)
	if readErr != nil || !bytes.Equal(before, after) {
		t.Fatalf("failed atomic binding write changed target: equal=%v err=%v", bytes.Equal(before, after), readErr)
	}
}

func TestBindingRejectsSymlinkedStateDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not generally available to unprivileged Windows tests")
	}
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	project := newPublicationProject(t)
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(project, ".viceme")); err != nil {
		t.Fatal(err)
	}
	summary := replicacontent.SourceArchiveSummary{Digest: strings.Repeat("a", 64), SizeBytes: 1024}
	_, pending := publicationStoreFixture(t, project, summary, now)
	publication := publicationFixture(now, "PROCESSING", "VERIFIED")
	pending.Publication = &PublicationReference{ID: publication.ID, Status: publication.Status, StatusURL: publication.StatusURL}
	pending.TakenOver = true
	err := (BindingStore{EndpointOrigin: pending.EndpointOrigin, Market: pending.Market, Now: func() time.Time { return now }}).
		Save(pending.ProjectPath, pending, publication)
	if cliErr := output.AsError(err); err == nil || cliErr.Subtype != "REPLICA_BINDING_PERMISSION_REQUIRED" {
		t.Fatalf("symlinked binding directory was accepted: %#v", cliErr)
	}
	if _, err := os.Stat(filepath.Join(target, bindingFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("binding escaped through the symlink: %v", err)
	}
}

func publicationStoreFixture(t *testing.T, project string, summary replicacontent.SourceArchiveSummary, now time.Time) (Store, Pending) {
	t.Helper()
	const endpoint = "https://api.viceme.cn"
	fingerprint, canonical, err := ProjectFingerprint(endpoint, "CN", project)
	if err != nil {
		t.Fatal(err)
	}
	store := Store{
		Directory:      ScopedDirectory(filepath.Join(t.TempDir(), "replica-publications"), endpoint, "CN"),
		EndpointOrigin: endpoint, Market: "CN", Now: func() time.Time { return now },
	}
	return store, Pending{
		EndpointOrigin: endpoint, Market: "CN", ProjectPath: canonical,
		ProjectFingerprint: fingerprint, ClientRequestID: testRequestID,
		Request: api.CreateWebsiteReplicaPublicationRequest{
			ProtocolVersion: api.WebsiteReplicaPublicationProtocolVersion, ClientRequestID: testRequestID,
			Market: "CN", ProjectFingerprint: fingerprint,
			Target: api.WebsiteReplicaPublicationTarget{Kind: "NEW_WORK", Slug: "replica-site"},
			Title:  "Replica", Summary: "Replica summary", PriceCents: 990,
			Source: api.WebsiteReplicaPublicationSourceArtifact{
				FileName: "source.zip", ContentType: "application/zip", SizeBytes: summary.SizeBytes, Digest: summary.Digest,
			},
		},
		SourceArchive: summary, ArtifactExpiresAt: now.Add(30 * time.Minute),
	}
}

func publicationFixture(now time.Time, status, sourceStatus string) api.WebsiteReplicaPublication {
	verifiedAt := pointer(now.Add(-time.Minute).Format(time.RFC3339))
	submittedAt := pointer(now.Add(-2 * time.Minute).Format(time.RFC3339))
	publication := api.WebsiteReplicaPublication{
		ID: testPublicationID, ClientRequestID: testRequestID, Market: "CN", MerchantAccountID: testMerchantID,
		WorkID: testWorkID, ReplicaID: testReplicaID, Status: status,
		StatusURL: "https://viceme.cn/me/website-replica-publications/" + testPublicationID,
		Source: api.WebsiteReplicaPublicationSource{
			FileName: "source.zip", ContentType: "application/zip", SizeBytes: 1024,
			Digest: strings.Repeat("a", 64), Status: sourceStatus, VerifiedAt: verifiedAt,
		},
		SubmittedAt: submittedAt, CreatedAt: now.Add(-3 * time.Minute).Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339),
	}
	if status == "PUBLISHED" || status == "PUBLISHED_DEGRADED" {
		publication.Result = &api.WebsiteReplicaPublicationResult{
			WorkURL: "https://viceme.cn/replica-maker/replica-site", VersionID: testVersionID, Version: 1,
			ShortCode: "VMR-ABCDEFGHIJKLMNOPQRST", Instruction: "VICEME-REPLICA:VMR-ABCDEFGHIJKLMNOPQRST",
			Product: api.WebsiteReplicaProduct{
				ID: testProductID, SKUID: testSKUID, Title: "Replica", Currency: "CNY", PriceCents: 990,
			},
			PublishedAt: now.Format(time.RFC3339),
		}
		if status == "PUBLISHED_DEGRADED" {
			publication.Failure = &api.WebsiteReplicaPublicationFailure{Code: "HOSTING_FAILED", Message: "Hosting failed", Retryable: false}
		}
	}
	return publication
}

func newPublicationProject(t *testing.T) string {
	t.Helper()
	project := filepath.Join(t.TempDir(), "site")
	if err := os.Mkdir(project, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "index.html"), []byte("<h1>Replica</h1>"), 0o600); err != nil {
		t.Fatal(err)
	}
	return project
}

func assertPrivateFile(t *testing.T, filename string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Lstat(filename)
	if err != nil {
		t.Fatalf("stat %s: %v", filename, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("%s is not a private regular file: mode=%v", filename, info.Mode())
	}
}

func assertPrivateDirectory(t *testing.T, directory string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Lstat(directory)
	if err != nil {
		t.Fatalf("stat %s: %v", directory, err)
	}
	if !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("%s is not a private directory: mode=%v", directory, info.Mode())
	}
}

func pointer[T any](value T) *T { return &value }
