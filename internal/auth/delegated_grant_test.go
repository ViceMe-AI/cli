package auth

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
)

func TestDelegatedGrantManagerIsolatesRegionsAndNeverReturnsSecretFromStatus(t *testing.T) {
	t.Parallel()
	store := securestore.NewMemory()
	lockDir := t.TempDir()
	first := &DelegatedGrantManager{Store: store, Region: "cn", LockDir: lockDir}
	second := &DelegatedGrantManager{Store: store, Region: "global", LockDir: lockDir}
	secret := strings.Repeat("a", 43)
	if err := first.Save("creator-one", secret); err != nil {
		t.Fatal(err)
	}
	status, err := first.Status("creator-one")
	if err != nil || !status.Stored || status.CredentialRef != "creator-one" {
		t.Fatalf("unexpected status: %#v err=%v", status, err)
	}
	if strings.Contains(status.CredentialRef, secret) {
		t.Fatal("status leaked the credential")
	}
	if _, err := second.Load("creator-one"); err == nil {
		t.Fatal("credential crossed region namespace")
	}
	loaded, err := first.Load("creator-one")
	if err != nil || loaded != secret {
		t.Fatalf("load failed: %q err=%v", loaded, err)
	}
}

func TestDelegatedGrantCredentialRejectsWhitespaceAndHeaderInjection(t *testing.T) {
	t.Parallel()
	for _, value := range []string{
		"too-short",
		strings.Repeat("a", 32) + " value",
		strings.Repeat("a", 32) + "\r\nx-leak: yes",
	} {
		if _, err := NormalizeDelegatedGrantCredential(value); err == nil {
			t.Fatalf("accepted unsafe credential %q", value)
		}
	}
}

func TestDelegatedGrantEnvelopeReusesStablePublicationLease(t *testing.T) {
	t.Parallel()
	store := securestore.NewMemory()
	manager := &DelegatedGrantManager{
		Store:   store,
		Region:  "cn",
		NewID:   func() string { return "stable-request-id" },
		LockDir: t.TempDir(),
	}
	secret := strings.Repeat("b", 43)
	if err := manager.Save("creator", secret); err != nil {
		t.Fatal(err)
	}
	loaded, err := manager.Load("creator")
	if err != nil || loaded != secret {
		t.Fatalf("versioned envelope did not round-trip the credential: %q err=%v", loaded, err)
	}
	binding := DelegatedPublicationBinding{
		IntentFingerprint:  "sha256:" + strings.Repeat("0", 64),
		RequestFingerprint: "sha256:" + strings.Repeat("a", 64),
		ResolutionID:       "res_frozen",
		Selector:           "skills/poster",
	}
	first, err := manager.BeginPublication("creator", binding)
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.BeginPublication("creator", DelegatedPublicationBinding{
		IntentFingerprint:  binding.IntentFingerprint,
		RequestFingerprint: binding.RequestFingerprint,
		ResolutionID:       "res_new_inspection_is_ignored",
		Selector:           binding.Selector,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first.Credential != secret || first.ClientRequestID != "stable-request-id" || first.ResolutionID != "res_frozen" {
		t.Fatalf("publication lease was not stable: first=%#v second=%#v", first, second)
	}
	if err := manager.CompletePublication("creator", first.ClientRequestID, first.RequestFingerprint); err != nil {
		t.Fatal(err)
	}
	status, err := manager.Status("creator")
	if err != nil || status.Stored {
		t.Fatalf("successful receipt did not atomically remove the envelope: %#v err=%v", status, err)
	}
}

func TestDelegatedGrantEnvelopeRejectsDifferentIntentAndRetainsCredential(t *testing.T) {
	t.Parallel()
	store := securestore.NewMemory()
	manager := &DelegatedGrantManager{Store: store, Region: "cn", NewID: func() string { return "request-1" }, LockDir: t.TempDir()}
	if err := manager.Save("creator", strings.Repeat("c", 43)); err != nil {
		t.Fatal(err)
	}
	first := DelegatedPublicationBinding{IntentFingerprint: "sha256:" + strings.Repeat("0", 64), RequestFingerprint: "sha256:" + strings.Repeat("1", 64), ResolutionID: "res_1"}
	if _, err := manager.BeginPublication("creator", first); err != nil {
		t.Fatal(err)
	}
	_, err := manager.BeginPublication("creator", DelegatedPublicationBinding{IntentFingerprint: "sha256:" + strings.Repeat("9", 64), RequestFingerprint: "sha256:" + strings.Repeat("2", 64), ResolutionID: "res_2"})
	var cliError *output.Error
	if !errors.As(err, &cliError) || cliError.Subtype != "delegated_grant_request_mismatch" {
		t.Fatalf("expected request mismatch, got %T: %v", err, err)
	}
	status, statusErr := manager.Status("creator")
	if statusErr != nil || !status.Stored {
		t.Fatalf("mismatch removed the credential: %#v err=%v", status, statusErr)
	}
}

func TestDelegatedGrantLegacyRawEntryMigratesOnFirstPublication(t *testing.T) {
	t.Parallel()
	store := securestore.NewMemory()
	secret := strings.Repeat("d", 43)
	if err := store.Set("delegated-grant:cn:legacy", secret); err != nil {
		t.Fatal(err)
	}
	manager := &DelegatedGrantManager{Store: store, Region: "cn", NewID: func() string { return "migrated-request" }, LockDir: t.TempDir()}
	lease, err := manager.BeginPublication("legacy", DelegatedPublicationBinding{
		IntentFingerprint:  "sha256:" + strings.Repeat("a", 64),
		RequestFingerprint: "sha256:" + strings.Repeat("e", 64),
		ResolutionID:       "res_legacy",
	})
	if err != nil || lease.Credential != secret || lease.ClientRequestID != "migrated-request" {
		t.Fatalf("legacy migration failed: %#v err=%v", lease, err)
	}
	stored, err := store.Get("delegated-grant:cn:legacy")
	if err != nil || !strings.HasPrefix(stored, "{") || strings.Contains(stored, `"publication":null`) {
		t.Fatalf("legacy entry was not upgraded to a bound envelope: %q err=%v", stored, err)
	}
}

func TestDelegatedGrantScopesAndReplacementAreFailClosed(t *testing.T) {
	t.Parallel()
	store := securestore.NewMemory()
	lockDir := t.TempDir()
	production := &DelegatedGrantManager{Store: store, Region: "cn", LockDir: lockDir}
	custom := &DelegatedGrantManager{Store: store, Region: "cn", Scope: "custom:origin", LockDir: lockDir}
	secret := strings.Repeat("f", 43)
	if err := production.Save("creator", secret); err != nil {
		t.Fatal(err)
	}
	if _, err := custom.Load("creator"); err == nil {
		t.Fatal("custom API origin read a production delegated grant")
	}
	if err := production.Save("creator", strings.Repeat("g", 43)); err == nil {
		t.Fatal("save silently replaced an existing one-time grant")
	}
}

func TestDelegatedGrantsAreIsolatedByProfile(t *testing.T) {
	t.Parallel()
	store := securestore.NewMemory()
	lockDir := t.TempDir()
	personal := &DelegatedGrantManager{Store: store, Region: "cn", ProfileID: "personal", LockDir: lockDir}
	work := &DelegatedGrantManager{Store: store, Region: "cn", ProfileID: "work", LockDir: lockDir}
	if err := personal.Save("creator", strings.Repeat("p", 43)); err != nil {
		t.Fatal(err)
	}
	if _, err := work.Load("creator"); err == nil {
		t.Fatal("delegated grant crossed profiles")
	}
}

func TestDelegatedRecoveryPeekNeverReadsCredentialEntry(t *testing.T) {
	t.Parallel()
	base := securestore.NewMemory()
	store := &credentialReadTrackingStore{Store: base}
	manager := &DelegatedGrantManager{
		Store:   store,
		Region:  "cn",
		NewID:   func() string { return "request-peek" },
		LockDir: t.TempDir(),
	}
	if err := manager.Save("creator", strings.Repeat("p", 43)); err != nil {
		t.Fatal(err)
	}
	store.credentialGets.Store(0)
	intent := "sha256:" + strings.Repeat("1", 64)
	resume, err := manager.PeekPublication("creator", intent)
	if err != nil || resume.Bound {
		t.Fatalf("unbound peek = %#v err=%v", resume, err)
	}
	if store.credentialGets.Load() != 0 {
		t.Fatal("unbound recovery peek read the credential entry")
	}
	binding := DelegatedPublicationBinding{
		IntentFingerprint:  intent,
		RequestFingerprint: "sha256:" + strings.Repeat("2", 64),
		ResolutionID:       "res_peek",
		Selector:           "skills/poster",
	}
	if _, err := manager.BeginPublication("creator", binding); err != nil {
		t.Fatal(err)
	}
	store.credentialGets.Store(0)
	resume, err = manager.PeekPublication("creator", intent)
	if err != nil || !resume.Bound || resume.ClientRequestID != "request-peek" || resume.ResolutionID != "res_peek" {
		t.Fatalf("bound peek = %#v err=%v", resume, err)
	}
	if store.credentialGets.Load() != 0 {
		t.Fatal("bound recovery peek read the credential entry")
	}
}

func TestDelegatedConcurrentDifferentIntentsOnlyOneReceivesCredential(t *testing.T) {
	t.Parallel()
	base := securestore.NewMemory()
	lockDir := t.TempDir()
	seed := &DelegatedGrantManager{Store: base, Region: "cn", NewID: func() string { return "request-concurrent" }, LockDir: lockDir}
	secret := strings.Repeat("q", 43)
	if err := seed.Save("creator", secret); err != nil {
		t.Fatal(err)
	}
	store := newBindingBarrierStore(base)
	first := &DelegatedGrantManager{Store: store, Region: "cn", LockDir: lockDir}
	second := &DelegatedGrantManager{Store: store, Region: "cn", LockDir: lockDir}
	bindings := []DelegatedPublicationBinding{
		{IntentFingerprint: "sha256:" + strings.Repeat("3", 64), RequestFingerprint: "sha256:" + strings.Repeat("4", 64), ResolutionID: "res_one"},
		{IntentFingerprint: "sha256:" + strings.Repeat("5", 64), RequestFingerprint: "sha256:" + strings.Repeat("6", 64), ResolutionID: "res_two"},
	}
	type result struct {
		lease DelegatedPublicationLease
		err   error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	for index, manager := range []*DelegatedGrantManager{first, second} {
		index, manager := index, manager
		go func() {
			<-start
			lease, err := manager.BeginPublication("creator", bindings[index])
			results <- result{lease: lease, err: err}
		}()
	}
	close(start)
	successes := 0
	mismatches := 0
	for range 2 {
		result := <-results
		if result.err == nil {
			successes++
			if result.lease.Credential != secret {
				t.Fatalf("successful lease omitted credential: %#v", result.lease)
			}
			continue
		}
		var cliError *output.Error
		if errors.As(result.err, &cliError) && cliError.Subtype == "delegated_grant_request_mismatch" {
			mismatches++
			continue
		}
		t.Fatalf("unexpected concurrent bind error: %v", result.err)
	}
	if successes != 1 || mismatches != 1 {
		t.Fatalf("concurrent binding was not exclusive: successes=%d mismatches=%d", successes, mismatches)
	}
}

func TestDelegatedCleanupCannotDeleteConcurrentReplacement(t *testing.T) {
	t.Parallel()
	base := securestore.NewMemory()
	lockDir := t.TempDir()
	manager := &DelegatedGrantManager{Store: base, Region: "cn", NewID: func() string { return "request-old" }, LockDir: lockDir}
	oldSecret := strings.Repeat("r", 43)
	if err := manager.Save("creator", oldSecret); err != nil {
		t.Fatal(err)
	}
	binding := DelegatedPublicationBinding{
		IntentFingerprint:  "sha256:" + strings.Repeat("7", 64),
		RequestFingerprint: "sha256:" + strings.Repeat("8", 64),
		ResolutionID:       "res_old",
	}
	lease, err := manager.BeginPublication("creator", binding)
	if err != nil {
		t.Fatal(err)
	}
	store := newBlockingCredentialGetStore(base)
	completeManager := &DelegatedGrantManager{Store: store, Region: "cn", LockDir: lockDir}
	replacementManager := &DelegatedGrantManager{Store: store, Region: "cn", NewID: func() string { return "request-new" }, LockDir: lockDir}
	completeDone := make(chan error, 1)
	go func() {
		completeDone <- completeManager.CompletePublication("creator", lease.ClientRequestID, lease.RequestFingerprint)
	}()
	<-store.captured
	newSecret := strings.Repeat("s", 43)
	replacementDone := make(chan error, 1)
	go func() {
		if deleteErr := replacementManager.Delete("creator"); deleteErr != nil {
			replacementDone <- deleteErr
			return
		}
		replacementDone <- replacementManager.Save("creator", newSecret)
	}()
	replacedBeforeRelease := false
	select {
	case err := <-replacementDone:
		replacedBeforeRelease = true
		if err != nil {
			t.Fatalf("replacement failed before cleanup release: %v", err)
		}
	case <-time.After(100 * time.Millisecond):
	}
	close(store.release)
	if err := <-completeDone; err != nil {
		t.Fatal(err)
	}
	if !replacedBeforeRelease {
		if err := <-replacementDone; err != nil {
			t.Fatal(err)
		}
	}
	if replacedBeforeRelease {
		t.Fatal("replacement entered the keychain transaction while cleanup held the reference lock")
	}
	loaded, err := replacementManager.Load("creator")
	if err != nil || loaded != newSecret {
		t.Fatalf("cleanup deleted the replacement grant: credential=%q err=%v", loaded, err)
	}
}

func TestDelegatedLegacyBindRepairsMetadataFirstInterruption(t *testing.T) {
	t.Parallel()
	base := securestore.NewMemory()
	secret := strings.Repeat("t", 43)
	if err := base.Set("delegated-grant:cn:legacy-crash", secret); err != nil {
		t.Fatal(err)
	}
	store := &failFirstBoundCredentialSetStore{Store: base}
	var ids atomic.Int32
	manager := &DelegatedGrantManager{
		Store:   store,
		Region:  "cn",
		NewID:   func() string { return fmt.Sprintf("request-%d", ids.Add(1)) },
		LockDir: t.TempDir(),
	}
	binding := DelegatedPublicationBinding{
		IntentFingerprint:  "sha256:" + strings.Repeat("a", 64),
		RequestFingerprint: "sha256:" + strings.Repeat("b", 64),
		ResolutionID:       "res_crash",
		Selector:           "skills/poster",
	}
	if _, err := manager.BeginPublication("legacy-crash", binding); err == nil {
		t.Fatal("injected credential envelope write failure was ignored")
	}
	resume, err := manager.PeekPublication("legacy-crash", binding.IntentFingerprint)
	if err != nil || !resume.Bound || resume.ClientRequestID != "request-1" {
		t.Fatalf("metadata-first recovery was not persisted: %#v err=%v", resume, err)
	}
	lease, err := manager.BeginPublication("legacy-crash", binding)
	if err != nil {
		t.Fatal(err)
	}
	if lease.ClientRequestID != "request-1" || lease.Credential != secret || ids.Load() < 2 {
		t.Fatalf("legacy recovery did not reuse metadata identity: lease=%#v generated_ids=%d", lease, ids.Load())
	}
}

type credentialReadTrackingStore struct {
	securestore.Store
	credentialGets atomic.Int32
}

func (store *credentialReadTrackingStore) Get(key string) (string, error) {
	if strings.HasPrefix(key, delegatedGrantKeyPrefix) {
		store.credentialGets.Add(1)
	}
	return store.Store.Get(key)
}

type bindingBarrierStore struct {
	securestore.Store
	credentialGets atomic.Int32
	release        chan struct{}
	releaseOnce    sync.Once
}

func newBindingBarrierStore(store securestore.Store) *bindingBarrierStore {
	return &bindingBarrierStore{Store: store, release: make(chan struct{})}
}

func (store *bindingBarrierStore) Get(key string) (string, error) {
	value, err := store.Store.Get(key)
	if !strings.HasPrefix(key, delegatedGrantKeyPrefix) {
		return value, err
	}
	if store.credentialGets.Add(1) == 2 {
		store.releaseOnce.Do(func() { close(store.release) })
	}
	select {
	case <-store.release:
	case <-time.After(150 * time.Millisecond):
	}
	return value, err
}

type blockingCredentialGetStore struct {
	securestore.Store
	captured chan struct{}
	release  chan struct{}
	blocked  atomic.Bool
}

func newBlockingCredentialGetStore(store securestore.Store) *blockingCredentialGetStore {
	return &blockingCredentialGetStore{Store: store, captured: make(chan struct{}), release: make(chan struct{})}
}

func (store *blockingCredentialGetStore) Get(key string) (string, error) {
	value, err := store.Store.Get(key)
	if strings.HasPrefix(key, delegatedGrantKeyPrefix) && store.blocked.CompareAndSwap(false, true) {
		close(store.captured)
		<-store.release
	}
	return value, err
}

type failFirstBoundCredentialSetStore struct {
	securestore.Store
	failed atomic.Bool
}

func (store *failFirstBoundCredentialSetStore) Set(key, value string) error {
	if strings.HasPrefix(key, delegatedGrantKeyPrefix) && strings.Contains(value, `"publication"`) && store.failed.CompareAndSwap(false, true) {
		return errors.New("injected bound credential envelope write failure")
	}
	return store.Store.Set(key, value)
}
