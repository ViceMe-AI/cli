package securestore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	masterKeyBytes       = 32
	masterKeyAccount     = "credential-master-key:v1"
	fileMasterKeyName    = "master.key.file"
	encryptedFileVersion = byte(1)
	keychainTimeout      = 5 * time.Second
)

var (
	errMasterKeyCorrupt = errors.New("credential master key is invalid")
	errKeychainBlocked  = errors.New("operating-system keychain access is blocked")
)

// EncryptedFileStore keeps credential payloads in private AES-256-GCM files.
// The master key normally lives in the operating-system keychain. A private
// file master key is used only after an explicit downgrade or when a fresh
// sandboxed installation cannot access the keychain.
type EncryptedFileStore struct {
	Service string
	Root    string
	Keyring Store
}

func NewEncryptedFile(root, service string, keyring Store) *EncryptedFileStore {
	return &EncryptedFileStore{Root: root, Service: service, Keyring: keyring}
}

func (s *EncryptedFileStore) Get(key string) (string, error) {
	data, err := os.ReadFile(s.credentialPath(key))
	if errors.Is(err, fs.ErrNotExist) {
		// Once a file master key exists, encrypted files are the authoritative
		// store. Do not fall back to a legacy raw Keychain entry from a sandbox;
		// the explicit downgrade command imports all configured legacy entries.
		if _, masterErr := s.readFileMasterKey(); masterErr == nil {
			return "", ErrNotFound
		} else if !errors.Is(masterErr, fs.ErrNotExist) {
			return "", masterErr
		}
		value, legacyErr := s.legacyGet(key)
		if legacyErr == nil || errors.Is(legacyErr, ErrNotFound) {
			return value, legacyErr
		}
		// A fresh sandbox has no encrypted assets yet and cannot distinguish
		// an empty Keychain from a blocked one. Report logged-out so auth login
		// can run its write preflight and establish the encrypted file fallback.
		// Existing encrypted assets fail closed instead of risking a second key.
		if !s.hasEncryptedCredentials() {
			return "", ErrNotFound
		}
		return "", legacyErr
	}
	if err != nil {
		return "", err
	}
	if err := requirePrivateFile(s.credentialPath(key)); err != nil {
		return "", err
	}

	var keyErrors []error
	if fileKey, fileErr := s.readFileMasterKey(); fileErr == nil {
		plaintext, decryptErr := decryptCredential(data, fileKey, s.associatedData(key))
		if decryptErr == nil {
			return plaintext, nil
		}
		keyErrors = append(keyErrors, decryptErr)
	} else if !errors.Is(fileErr, fs.ErrNotExist) {
		keyErrors = append(keyErrors, fileErr)
	}

	systemKey, systemErr := s.systemMasterKey(false)
	if systemErr == nil {
		plaintext, decryptErr := decryptCredential(data, systemKey, s.associatedData(key))
		if decryptErr == nil {
			return plaintext, nil
		}
		keyErrors = append(keyErrors, decryptErr)
	} else {
		keyErrors = append(keyErrors, systemErr)
	}
	return "", fmt.Errorf("decrypt stored credential: %w", errors.Join(keyErrors...))
}

func (s *EncryptedFileStore) Set(key, value string) error {
	masterKey, err := s.masterKeyForWrite()
	if err != nil {
		return err
	}
	data, err := encryptCredential(value, masterKey, s.associatedData(key))
	if err != nil {
		return err
	}
	return s.writeCredential(key, data)
}

func (s *EncryptedFileStore) Delete(key string) error {
	path := s.credentialPath(key)
	fileErr := os.Remove(path)
	if errors.Is(fileErr, fs.ErrNotExist) {
		fileErr = nil
	}
	legacyErr := s.callKeyring(func() error { return s.Keyring.Delete(key) })
	if errors.Is(legacyErr, ErrNotFound) {
		legacyErr = nil
	}
	if legacyErr != nil && fileErr == nil {
		if _, masterErr := s.readFileMasterKey(); masterErr == nil {
			// The encrypted file namespace is pinned once a file master key
			// exists, so an unreachable cold-backup Keychain entry cannot become
			// active again in this context.
			legacyErr = nil
		}
	}
	return errors.Join(fileErr, legacyErr)
}

func (s *EncryptedFileStore) Preflight(key string) error {
	masterKey, err := s.masterKeyForWrite()
	if err != nil {
		return err
	}
	probe, err := encryptCredential("viceme-credential-store-preflight", masterKey, s.associatedData(key+":preflight"))
	if err != nil {
		return err
	}
	if _, err := decryptCredential(probe, masterKey, s.associatedData(key+":preflight")); err != nil {
		return err
	}
	if err := s.ensureDirectory(); err != nil {
		return err
	}
	file, err := os.CreateTemp(s.directory(), ".preflight-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(probe); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	readBack, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	_, err = decryptCredential(readBack, masterKey, s.associatedData(key+":preflight"))
	return err
}

func (s *EncryptedFileStore) DowngradeKeychain(keys []string) (KeychainDowngradeResult, error) {
	result := KeychainDowngradeResult{MasterKeyPath: s.masterKeyPath()}
	masterKey, err := s.readFileMasterKey()
	if err == nil {
		result.Status = "already_downgraded"
	} else if !errors.Is(err, fs.ErrNotExist) {
		return result, err
	} else {
		masterKey, err = s.systemMasterKey(false)
		switch {
		case err == nil:
			result.Status = "copied_keychain_master_key"
		case errors.Is(err, ErrNotFound):
			masterKey = make([]byte, masterKeyBytes)
			if _, err = rand.Read(masterKey); err != nil {
				return result, err
			}
			result.Status = "created_file_master_key"
		default:
			return result, err
		}
		if _, err = s.createFileMasterKey(masterKey); err != nil {
			return result, err
		}
	}

	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if _, statErr := os.Stat(s.credentialPath(key)); statErr == nil {
			continue
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			return result, statErr
		}
		value, getErr := s.legacyGet(key)
		if errors.Is(getErr, ErrNotFound) {
			continue
		}
		if getErr != nil {
			return result, getErr
		}
		data, encryptErr := encryptCredential(value, masterKey, s.associatedData(key))
		if encryptErr != nil {
			return result, encryptErr
		}
		if writeErr := s.writeCredential(key, data); writeErr != nil {
			return result, writeErr
		}
		result.MigratedCredentials++
	}
	return result, nil
}

func (s *EncryptedFileStore) masterKeyForWrite() ([]byte, error) {
	if key, err := s.readFileMasterKey(); err == nil {
		return key, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if key, err := s.systemMasterKey(true); err == nil {
		return key, nil
	} else if s.hasEncryptedCredentials() {
		return nil, fmt.Errorf("%w; run 'viceme config keychain-downgrade' from an interactive macOS terminal before using this sandbox", err)
	}
	key := make([]byte, masterKeyBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return s.createFileMasterKey(key)
}

func (s *EncryptedFileStore) systemMasterKey(allowCreate bool) ([]byte, error) {
	encoded, err := callWithTimeout(keychainTimeout, func() (string, error) {
		return s.Keyring.Get(masterKeyAccount)
	})
	if err == nil {
		key, decodeErr := base64.StdEncoding.DecodeString(encoded)
		if decodeErr != nil || len(key) != masterKeyBytes {
			return nil, errMasterKeyCorrupt
		}
		return key, nil
	}
	if !errors.Is(err, ErrNotFound) || !allowCreate {
		return nil, err
	}
	key := make([]byte, masterKeyBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	encoded = base64.StdEncoding.EncodeToString(key)
	if err := s.callKeyring(func() error { return s.Keyring.Set(masterKeyAccount, encoded) }); err != nil {
		return nil, err
	}
	canonical, err := callWithTimeout(keychainTimeout, func() (string, error) {
		return s.Keyring.Get(masterKeyAccount)
	})
	if err != nil {
		return nil, err
	}
	key, err = base64.StdEncoding.DecodeString(canonical)
	if err != nil || len(key) != masterKeyBytes {
		return nil, errMasterKeyCorrupt
	}
	return key, nil
}

func (s *EncryptedFileStore) readFileMasterKey() ([]byte, error) {
	path := s.masterKeyPath()
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := requirePrivateFile(path); err != nil {
		return nil, err
	}
	if len(key) != masterKeyBytes {
		return nil, errMasterKeyCorrupt
	}
	return key, nil
}

func (s *EncryptedFileStore) createFileMasterKey(key []byte) ([]byte, error) {
	if len(key) != masterKeyBytes {
		return nil, errMasterKeyCorrupt
	}
	if err := s.ensureDirectory(); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(s.masterKeyPath(), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return s.readFileMasterKey()
	}
	if err != nil {
		return nil, err
	}
	failed := true
	defer func() {
		if failed {
			_ = os.Remove(s.masterKeyPath())
		}
	}()
	if _, err := file.Write(key); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	failed = false
	return s.readFileMasterKey()
}

func (s *EncryptedFileStore) legacyGet(key string) (string, error) {
	return callWithTimeout(keychainTimeout, func() (string, error) { return s.Keyring.Get(key) })
}

func (s *EncryptedFileStore) callKeyring(call func() error) error {
	_, err := callWithTimeout(keychainTimeout, func() (struct{}, error) {
		return struct{}{}, call()
	})
	return err
}

func (s *EncryptedFileStore) writeCredential(key string, data []byte) error {
	if err := s.ensureDirectory(); err != nil {
		return err
	}
	file, err := os.CreateTemp(s.directory(), ".credential-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, s.credentialPath(key))
}

func (s *EncryptedFileStore) ensureDirectory() error {
	if err := os.MkdirAll(s.directory(), 0o700); err != nil {
		return err
	}
	info, err := os.Stat(s.directory())
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("credential directory %s must not be accessible by group or other users", s.directory())
	}
	return nil
}

func (s *EncryptedFileStore) hasEncryptedCredentials() bool {
	entries, err := os.ReadDir(s.directory())
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".enc") {
			return true
		}
	}
	return false
}

func (s *EncryptedFileStore) directory() string { return filepath.Join(s.Root, "credentials") }
func (s *EncryptedFileStore) masterKeyPath() string {
	return filepath.Join(s.directory(), fileMasterKeyName)
}
func (s *EncryptedFileStore) credentialPath(key string) string {
	digest := sha256.Sum256([]byte(key))
	return filepath.Join(s.directory(), fmt.Sprintf("%x.enc", digest[:]))
}
func (s *EncryptedFileStore) associatedData(key string) []byte {
	return []byte(s.Service + "\x00" + key)
}

func encryptCredential(plaintext string, key, associatedData []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	sealed := gcm.Seal(nil, nonce, []byte(plaintext), associatedData)
	result := make([]byte, 1, 1+len(nonce)+len(sealed))
	result[0] = encryptedFileVersion
	result = append(result, nonce...)
	result = append(result, sealed...)
	return result, nil
}

func decryptCredential(data, key, associatedData []byte) (string, error) {
	if len(data) < 2 || data[0] != encryptedFileVersion {
		return "", errors.New("unsupported encrypted credential format")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < 1+gcm.NonceSize()+gcm.Overhead() {
		return "", errors.New("encrypted credential is truncated")
	}
	nonce := data[1 : 1+gcm.NonceSize()]
	plaintext, err := gcm.Open(nil, nonce, data[1+gcm.NonceSize():], associatedData)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func requirePrivateFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("credential file %s must have permissions 0600", path)
	}
	return nil
}

func callWithTimeout[T any](timeout time.Duration, call func() (T, error)) (T, error) {
	type result struct {
		value T
		err   error
	}
	channel := make(chan result, 1)
	go func() {
		value, err := call()
		channel <- result{value: value, err: err}
	}()
	select {
	case result := <-channel:
		return result.value, result.err
	case <-time.After(timeout):
		var zero T
		return zero, errKeychainBlocked
	}
}
