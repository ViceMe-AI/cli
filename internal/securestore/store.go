package securestore

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	keyring "github.com/zalando/go-keyring"
)

var ErrNotFound = errors.New("secure value not found")

type Store interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
}

// PersistenceProbe is implemented by stores that can verify the complete
// persistence path without receiving a real credential. Device login uses it
// before exchanging a one-time authorization code.
type PersistenceProbe interface {
	Preflight(key string) error
}

// KeychainDowngradeResult describes a controlled macOS downgrade from an OS
// Keychain protected master key to a private local master-key file.
type KeychainDowngradeResult struct {
	Status              string `json:"status"`
	MasterKeyPath       string `json:"master_key_path"`
	MigratedCredentials int    `json:"migrated_credentials"`
}

// KeychainDowngrader is intentionally separate from Store so the public
// command can fail closed on platforms/backends where it is not supported.
type KeychainDowngrader interface {
	DowngradeKeychain(keys []string) (KeychainDowngradeResult, error)
}

type KeyringStore struct {
	Service string
}

func NewKeyring(service string) *KeyringStore {
	return &KeyringStore{Service: service}
}

func (s *KeyringStore) Get(key string) (string, error) {
	value, err := keyring.Get(s.Service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return value, err
}

func (s *KeyringStore) Set(key, value string) error {
	return keyring.Set(s.Service, key, value)
}

func (s *KeyringStore) Delete(key string) error {
	err := keyring.Delete(s.Service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

// Preflight verifies that this process can write, read, and delete a harmless
// probe entry. The random account prevents concurrent CLI processes from
// interfering with each other.
func (s *KeyringStore) Preflight(key string) error {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return err
	}
	probeKey := fmt.Sprintf("%s:preflight:%s", key, hex.EncodeToString(random[:]))
	const probeValue = "viceme-credential-store-preflight"
	if err := s.Set(probeKey, probeValue); err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = s.Delete(probeKey)
		}
	}()
	value, err := s.Get(probeKey)
	if err != nil {
		return err
	}
	if value != probeValue {
		return errors.New("secure store preflight read-back mismatch")
	}
	err = s.Delete(probeKey)
	cleanup = false
	return err
}

// MemoryStore is intentionally small and is useful for embedding the command
// tree in tests or other trusted callers without touching the host keychain.
type MemoryStore struct {
	mu     sync.RWMutex
	values map[string]string
}

func NewMemory() *MemoryStore {
	return &MemoryStore{values: make(map[string]string)}
}

func (s *MemoryStore) Get(key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.values[key]
	if !ok {
		return "", ErrNotFound
	}
	return value, nil
}

func (s *MemoryStore) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
	return nil
}

func (s *MemoryStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.values[key]; !ok {
		return ErrNotFound
	}
	delete(s.values, key)
	return nil
}

func (s *MemoryStore) Preflight(string) error { return nil }
