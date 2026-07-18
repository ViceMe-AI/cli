package securestore

import (
	"errors"
	"sync"

	keyring "github.com/zalando/go-keyring"
)

var ErrNotFound = errors.New("secure value not found")

type Store interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
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
