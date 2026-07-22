package securestore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type fakeKeyring struct {
	mu      sync.Mutex
	blocked bool
	values  map[string]string
}

func newFakeKeyring() *fakeKeyring { return &fakeKeyring{values: make(map[string]string)} }

func (s *fakeKeyring) Get(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked {
		return "", errors.New("sandbox blocked keychain")
	}
	value, exists := s.values[key]
	if !exists {
		return "", ErrNotFound
	}
	return value, nil
}

func (s *fakeKeyring) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked {
		return errors.New("sandbox blocked keychain")
	}
	s.values[key] = value
	return nil
}

func (s *fakeKeyring) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked {
		return errors.New("sandbox blocked keychain")
	}
	if _, exists := s.values[key]; !exists {
		return ErrNotFound
	}
	delete(s.values, key)
	return nil
}

func (s *fakeKeyring) setBlocked(blocked bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blocked = blocked
}

func TestEncryptedFileStoreFallsBackWhenKeychainIsBlocked(t *testing.T) {
	keyring := newFakeKeyring()
	keyring.setBlocked(true)
	root := t.TempDir()
	store := NewEncryptedFile(root, "viceme-cli-test", keyring)
	const storageKey = "credential:default:custom:origin-a"
	const secret = "sandbox-secret-that-must-not-be-plaintext"
	if _, err := store.Get(storageKey); !errors.Is(err, ErrNotFound) {
		t.Fatalf("fresh blocked sandbox Get() error = %v, want ErrNotFound", err)
	}

	if err := store.Preflight(storageKey); err != nil {
		t.Fatalf("Preflight() error = %v", err)
	}
	if err := store.Set(storageKey, secret); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	got, err := store.Get(storageKey)
	if err != nil || got != secret {
		t.Fatalf("Get() = %q, %v", got, err)
	}

	masterInfo, err := os.Stat(store.masterKeyPath())
	if err != nil {
		t.Fatalf("file master key missing: %v", err)
	}
	if masterInfo.Mode().Perm() != 0o600 {
		t.Fatalf("master key mode = %o, want 600", masterInfo.Mode().Perm())
	}
	directoryInfo, err := os.Stat(store.directory())
	if err != nil {
		t.Fatal(err)
	}
	if directoryInfo.Mode().Perm() != 0o700 {
		t.Fatalf("credential directory mode = %o, want 700", directoryInfo.Mode().Perm())
	}
	credentialInfo, err := os.Stat(store.credentialPath(storageKey))
	if err != nil {
		t.Fatal(err)
	}
	if credentialInfo.Mode().Perm() != 0o600 {
		t.Fatalf("credential file mode = %o, want 600", credentialInfo.Mode().Perm())
	}
	ciphertext, err := os.ReadFile(store.credentialPath(storageKey))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(ciphertext), secret) {
		t.Fatal("encrypted credential file contains plaintext token")
	}
}

func TestDowngradeMigratesExistingLegacyKeychainCredential(t *testing.T) {
	keyring := newFakeKeyring()
	const storageKey = "credential:profile-work:custom:origin-work"
	const credential = `{"access_token":"legacy-external-token"}`
	keyring.values[storageKey] = credential
	store := NewEncryptedFile(t.TempDir(), "viceme-cli-test", keyring)

	result, err := store.DowngradeKeychain([]string{storageKey})
	if err != nil {
		t.Fatalf("DowngradeKeychain() error = %v", err)
	}
	if result.Status != "created_file_master_key" || result.MigratedCredentials != 1 {
		t.Fatalf("unexpected downgrade result: %#v", result)
	}
	keyring.setBlocked(true)
	got, err := store.Get(storageKey)
	if err != nil || got != credential {
		t.Fatalf("sandbox Get() after downgrade = %q, %v", got, err)
	}
}

func TestDowngradeCopiesSystemMasterKeyForExistingEncryptedCredential(t *testing.T) {
	keyring := newFakeKeyring()
	store := NewEncryptedFile(t.TempDir(), "viceme-cli-test", keyring)
	const storageKey = "credential:default:cn"
	const credential = `{"access_token":"external-token"}`
	if err := store.Set(storageKey, credential); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.masterKeyPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("normal external storage unexpectedly created file master key: %v", err)
	}

	keyring.setBlocked(true)
	if _, err := store.Get(storageKey); err == nil {
		t.Fatal("sandbox unexpectedly read Keychain-protected credential before downgrade")
	}
	keyring.setBlocked(false)
	result, err := store.DowngradeKeychain([]string{storageKey})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "copied_keychain_master_key" || result.MigratedCredentials != 0 {
		t.Fatalf("unexpected downgrade result: %#v", result)
	}
	keyring.setBlocked(true)
	got, err := store.Get(storageKey)
	if err != nil || got != credential {
		t.Fatalf("sandbox Get() after copied master key = %q, %v", got, err)
	}
}

func TestEncryptedFileStoreKeepsProfileAndOriginKeysIsolated(t *testing.T) {
	keyring := newFakeKeyring()
	keyring.setBlocked(true)
	store := NewEncryptedFile(t.TempDir(), "viceme-cli-test", keyring)
	values := map[string]string{
		"credential:personal:custom:origin-a": `{"access_token":"personal-a"}`,
		"credential:personal:custom:origin-b": `{"access_token":"personal-b"}`,
		"credential:work:custom:origin-a":     `{"access_token":"work-a"}`,
	}
	for key, value := range values {
		if err := store.Set(key, value); err != nil {
			t.Fatalf("Set(%q) error = %v", key, err)
		}
	}
	for key, want := range values {
		got, err := store.Get(key)
		if err != nil || got != want {
			t.Fatalf("Get(%q) = %q, %v; want %q", key, got, err, want)
		}
	}
	if _, err := store.Get("credential:work:custom:origin-b"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing isolated scope error = %v, want ErrNotFound", err)
	}

	entries, err := os.ReadDir(filepath.Join(store.Root, "credentials"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "personal") || strings.Contains(entry.Name(), "origin") {
			t.Fatalf("credential filename leaked scope metadata: %s", entry.Name())
		}
	}
}
