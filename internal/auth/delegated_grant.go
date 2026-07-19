package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/gofrs/flock"
)

const (
	delegatedGrantKeyPrefix      = "delegated-grant:"
	delegatedRecoveryKeyPrefix   = "delegated-grant-recovery:"
	maxDelegatedGrantBytes       = 4096
	delegatedGrantVersion        = 1
	delegatedRecoveryVersion     = 1
	delegatedGrantLockPermission = 0o700
)

type DelegatedGrantStatus struct {
	CredentialRef string `json:"credential_ref"`
	Stored        bool   `json:"stored"`
}

// DelegatedGrantManager keeps one-time delegated publication credentials in
// the same OS keychain boundary as login credentials, but under a distinct
// namespace. Callers only handle a non-sensitive reference after Save.
type DelegatedGrantManager struct {
	Store securestore.Store
	// Region is the backwards-compatible namespace for the canonical cn and
	// global API origins. Scope is required for custom API origins.
	Region string
	Scope  string
	NewID  func() string
	// LockDir must resolve to the same directory in every CLI process that can
	// access Store. Per-reference OS file locks serialize keychain state changes.
	LockDir string
}

// DelegatedPublicationBinding contains only non-sensitive publication state.
// IntentFingerprint identifies the stable user invocation, while
// RequestFingerprint identifies the exact frozen request sent to the API.
type DelegatedPublicationBinding struct {
	IntentFingerprint  string `json:"intent_fingerprint"`
	RequestFingerprint string `json:"request_fingerprint"`
	ResolutionID       string `json:"resolution_id"`
	Selector           string `json:"selector,omitempty"`
}

// DelegatedPublicationResume is safe for callers to inspect before provider
// resolution. It is loaded from a separate metadata entry that never contains
// the delegated credential.
type DelegatedPublicationResume struct {
	Bound              bool
	ClientRequestID    string
	IntentFingerprint  string
	RequestFingerprint string
	ResolutionID       string
	Selector           string
}

type DelegatedPublicationLease struct {
	Credential         string
	ClientRequestID    string
	RequestFingerprint string
	ResolutionID       string
	Selector           string
}

type delegatedGrantEnvelope struct {
	Version         int                          `json:"version"`
	Credential      string                       `json:"credential"`
	ClientRequestID string                       `json:"client_request_id"`
	Publication     *DelegatedPublicationBinding `json:"publication,omitempty"`
}

type delegatedRecoveryEnvelope struct {
	Version         int                         `json:"version"`
	ClientRequestID string                      `json:"client_request_id"`
	Publication     DelegatedPublicationBinding `json:"publication"`
}

func ValidateDelegatedGrantReference(reference string) error {
	if len(reference) < 1 || len(reference) > 64 {
		return output.Validation("delegated_grant_ref_invalid", "delegated grant reference must be 1 to 64 characters")
	}
	for _, value := range reference {
		if (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z') ||
			(value >= '0' && value <= '9') || value == '.' || value == '_' || value == '-' {
			continue
		}
		return output.Validation("delegated_grant_ref_invalid", "delegated grant reference may contain only letters, numbers, dot, underscore, and hyphen")
	}
	return nil
}

func NormalizeDelegatedGrantCredential(value string) (string, error) {
	credential := strings.TrimSpace(value)
	if len(credential) < 32 || len(credential) > maxDelegatedGrantBytes {
		return "", output.Authentication("delegated_grant_invalid", "delegated grant credential is invalid")
	}
	if strings.IndexFunc(credential, unicode.IsSpace) >= 0 || strings.ContainsAny(credential, "\r\n\x00") {
		return "", output.Authentication("delegated_grant_invalid", "delegated grant credential is invalid")
	}
	return credential, nil
}

func (m *DelegatedGrantManager) Save(reference, credential string) error {
	if err := ValidateDelegatedGrantReference(reference); err != nil {
		return err
	}
	normalized, err := NormalizeDelegatedGrantCredential(credential)
	if err != nil {
		return err
	}
	return m.withReferenceLock(reference, func() error {
		if _, getErr := m.Store.Get(m.key(reference)); getErr == nil {
			return output.Validation("delegated_grant_ref_exists", "delegated grant reference already exists; delete it explicitly before saving a replacement")
		} else if !errors.Is(getErr, securestore.ErrNotFound) {
			return output.Authentication("delegated_grant_store_unavailable", "could not inspect the delegated grant keychain entry").WithCause(getErr)
		}
		// A credential-first cleanup crash can leave only non-sensitive recovery
		// metadata. Remove it before installing a replacement grant.
		if deleteErr := m.deleteRecoveryLocked(reference); deleteErr != nil {
			return deleteErr
		}
		envelope := delegatedGrantEnvelope{
			Version:         delegatedGrantVersion,
			Credential:      normalized,
			ClientRequestID: m.newID(),
		}
		encoded, encodeErr := encodeDelegatedGrantEnvelope(envelope)
		if encodeErr != nil {
			return encodeErr
		}
		if setErr := m.Store.Set(m.key(reference), encoded); setErr != nil {
			return output.Authentication("delegated_grant_store_unavailable", "could not save the delegated grant in the operating system keychain").WithCause(setErr)
		}
		return nil
	})
}

func (m *DelegatedGrantManager) Load(reference string) (string, error) {
	if err := ValidateDelegatedGrantReference(reference); err != nil {
		return "", err
	}
	value, err := m.Store.Get(m.key(reference))
	if errors.Is(err, securestore.ErrNotFound) {
		return "", output.Authentication("delegated_grant_not_found", "delegated grant credential was not found")
	}
	if err != nil {
		return "", output.Authentication("delegated_grant_store_unavailable", "could not read the delegated grant from the operating system keychain").WithCause(err)
	}
	envelope, _, err := m.decode(value)
	if err != nil {
		return "", err
	}
	return envelope.Credential, nil
}

// PeekPublication reads only the non-sensitive recovery entry. An unbound
// reference has no recovery entry, so first use still resolves and selects a
// provider candidate before the credential key is read.
func (m *DelegatedGrantManager) PeekPublication(reference, intentFingerprint string) (DelegatedPublicationResume, error) {
	if err := ValidateDelegatedGrantReference(reference); err != nil {
		return DelegatedPublicationResume{}, err
	}
	if err := validateFingerprint(intentFingerprint); err != nil {
		return DelegatedPublicationResume{}, err
	}
	var resume DelegatedPublicationResume
	err := m.withReferenceLock(reference, func() error {
		recovery, found, loadErr := m.loadRecoveryLocked(reference)
		if loadErr != nil {
			return loadErr
		}
		if !found {
			return nil
		}
		if recovery.Publication.IntentFingerprint != intentFingerprint {
			return delegatedPublicationMismatch()
		}
		resume = recovery.resume()
		return nil
	})
	return resume, err
}

// BeginPublication binds a keychain reference to exactly one immutable
// publication intent before returning its credential. Repeated invocations
// reuse the original request and resolution identities; a different intent
// fails closed instead of replaying the one-time grant.
func (m *DelegatedGrantManager) BeginPublication(reference string, binding DelegatedPublicationBinding) (DelegatedPublicationLease, error) {
	if err := ValidateDelegatedGrantReference(reference); err != nil {
		return DelegatedPublicationLease{}, err
	}
	if err := validatePublicationBinding(binding); err != nil {
		return DelegatedPublicationLease{}, err
	}
	var lease DelegatedPublicationLease
	err := m.withReferenceLock(reference, func() error {
		value, getErr := m.Store.Get(m.key(reference))
		if errors.Is(getErr, securestore.ErrNotFound) {
			return output.Authentication("delegated_grant_not_found", "delegated grant credential was not found")
		}
		if getErr != nil {
			return output.Authentication("delegated_grant_store_unavailable", "could not read the delegated grant from the operating system keychain").WithCause(getErr)
		}
		envelope, legacy, decodeErr := m.decode(value)
		if decodeErr != nil {
			return decodeErr
		}
		legacyRawCredential := legacy
		if envelope.ClientRequestID == "" {
			envelope.ClientRequestID = m.newID()
			legacy = true
		}

		recovery, recoveryFound, recoveryErr := m.loadRecoveryLocked(reference)
		if recoveryErr != nil {
			return recoveryErr
		}
		switch {
		case envelope.Publication != nil:
			if envelope.Publication.IntentFingerprint != binding.IntentFingerprint {
				return delegatedPublicationMismatch()
			}
			if recoveryFound && !recovery.matches(envelope.ClientRequestID, *envelope.Publication) {
				return output.Authentication("delegated_grant_invalid", "stored delegated grant recovery state is inconsistent")
			}
			if !recoveryFound {
				recovery = newDelegatedRecoveryEnvelope(envelope.ClientRequestID, *envelope.Publication)
				if setErr := m.storeRecoveryLocked(reference, recovery); setErr != nil {
					return setErr
				}
			}
		case recoveryFound:
			// Metadata is written before the credential envelope. If a process
			// stopped between those writes, use it to finish the binding without
			// contacting the provider or exposing the credential prematurely.
			if recovery.Publication.IntentFingerprint != binding.IntentFingerprint {
				return delegatedPublicationMismatch()
			}
			if legacyRawCredential {
				// A raw legacy credential has no durable request id. When a prior
				// metadata-first bind was interrupted, its non-sensitive id is the
				// authoritative recovery value.
				envelope.ClientRequestID = recovery.ClientRequestID
			} else if recovery.ClientRequestID != envelope.ClientRequestID {
				return output.Authentication("delegated_grant_invalid", "stored delegated grant recovery state is inconsistent")
			}
			copy := recovery.Publication
			envelope.Publication = &copy
			legacy = true
		default:
			copy := binding
			recovery = newDelegatedRecoveryEnvelope(envelope.ClientRequestID, copy)
			// Metadata-first ordering makes an interrupted bind resumable. No
			// credential is returned unless the authoritative envelope is stored.
			if setErr := m.storeRecoveryLocked(reference, recovery); setErr != nil {
				return setErr
			}
			envelope.Publication = &copy
			legacy = true
		}
		if legacy {
			encoded, encodeErr := encodeDelegatedGrantEnvelope(envelope)
			if encodeErr != nil {
				return encodeErr
			}
			if setErr := m.Store.Set(m.key(reference), encoded); setErr != nil {
				return output.Authentication("delegated_grant_store_unavailable", "could not persist delegated publication recovery state").WithCause(setErr)
			}
		}
		lease = DelegatedPublicationLease{
			Credential:         envelope.Credential,
			ClientRequestID:    envelope.ClientRequestID,
			RequestFingerprint: envelope.Publication.RequestFingerprint,
			ResolutionID:       envelope.Publication.ResolutionID,
			Selector:           envelope.Publication.Selector,
		}
		return nil
	})
	return lease, err
}

// CompletePublication deletes the credential and recovery state only when the
// receipt matches the publication lease that was actually sent.
func (m *DelegatedGrantManager) CompletePublication(reference, clientRequestID, requestFingerprint string) error {
	if err := ValidateDelegatedGrantReference(reference); err != nil {
		return err
	}
	return m.withReferenceLock(reference, func() error {
		value, getErr := m.Store.Get(m.key(reference))
		if errors.Is(getErr, securestore.ErrNotFound) {
			return m.deleteRecoveryLocked(reference)
		}
		if getErr != nil {
			return output.Authentication("delegated_grant_store_unavailable", "could not read delegated publication recovery state").WithCause(getErr)
		}
		envelope, _, decodeErr := m.decode(value)
		if decodeErr != nil {
			return decodeErr
		}
		if envelope.Publication == nil || envelope.ClientRequestID != clientRequestID || envelope.Publication.RequestFingerprint != requestFingerprint {
			return output.Authentication(
				"delegated_grant_cleanup_conflict",
				"delegated grant recovery state changed before cleanup; the keychain entry was retained",
			)
		}
		return m.deleteLocked(reference)
	})
}

func (m *DelegatedGrantManager) Delete(reference string) error {
	if err := ValidateDelegatedGrantReference(reference); err != nil {
		return err
	}
	return m.withReferenceLock(reference, func() error {
		return m.deleteLocked(reference)
	})
}

func (m *DelegatedGrantManager) Status(reference string) (DelegatedGrantStatus, error) {
	if err := ValidateDelegatedGrantReference(reference); err != nil {
		return DelegatedGrantStatus{}, err
	}
	_, err := m.Store.Get(m.key(reference))
	if errors.Is(err, securestore.ErrNotFound) {
		return DelegatedGrantStatus{CredentialRef: reference, Stored: false}, nil
	}
	if err != nil {
		return DelegatedGrantStatus{}, output.Authentication("delegated_grant_store_unavailable", "could not inspect the delegated grant keychain entry").WithCause(err)
	}
	return DelegatedGrantStatus{CredentialRef: reference, Stored: true}, nil
}

func (m *DelegatedGrantManager) key(reference string) string {
	return delegatedGrantKeyPrefix + m.namespace() + ":" + reference
}

func (m *DelegatedGrantManager) recoveryKey(reference string) string {
	return delegatedRecoveryKeyPrefix + m.namespace() + ":" + reference
}

func (m *DelegatedGrantManager) namespace() string {
	if scope := strings.TrimSpace(m.Scope); scope != "" {
		return scope
	}
	region := strings.ToLower(strings.TrimSpace(m.Region))
	if region == "" {
		region = "cn"
	}
	return region
}

func (m *DelegatedGrantManager) decode(value string) (delegatedGrantEnvelope, bool, error) {
	if !strings.HasPrefix(strings.TrimSpace(value), "{") {
		normalized, err := NormalizeDelegatedGrantCredential(value)
		if err != nil {
			return delegatedGrantEnvelope{}, false, output.Authentication("delegated_grant_invalid", "stored delegated grant state is invalid")
		}
		return delegatedGrantEnvelope{
			Version:         delegatedGrantVersion,
			Credential:      normalized,
			ClientRequestID: m.newID(),
		}, true, nil
	}
	var envelope delegatedGrantEnvelope
	if err := json.Unmarshal([]byte(value), &envelope); err != nil || envelope.Version != delegatedGrantVersion {
		return delegatedGrantEnvelope{}, false, output.Authentication("delegated_grant_invalid", "stored delegated grant state is invalid")
	}
	normalized, err := NormalizeDelegatedGrantCredential(envelope.Credential)
	if err != nil {
		return delegatedGrantEnvelope{}, false, output.Authentication("delegated_grant_invalid", "stored delegated grant state is invalid")
	}
	envelope.Credential = normalized
	if !validClientRequestID(envelope.ClientRequestID) {
		return delegatedGrantEnvelope{}, false, output.Authentication("delegated_grant_invalid", "stored delegated grant recovery state is invalid")
	}
	if envelope.Publication != nil {
		if err := validatePublicationBinding(*envelope.Publication); err != nil {
			return delegatedGrantEnvelope{}, false, output.Authentication("delegated_grant_invalid", "stored delegated grant recovery state is invalid")
		}
	}
	return envelope, false, nil
}

func validatePublicationBinding(binding DelegatedPublicationBinding) error {
	if err := validateFingerprint(binding.IntentFingerprint); err != nil {
		return err
	}
	if err := validateFingerprint(binding.RequestFingerprint); err != nil {
		return err
	}
	if strings.TrimSpace(binding.ResolutionID) == "" || strings.TrimSpace(binding.ResolutionID) != binding.ResolutionID || len(binding.ResolutionID) > 255 {
		return output.Validation("resolution_id_invalid", "delegated publication requires a valid immutable resolution id")
	}
	if strings.TrimSpace(binding.Selector) != binding.Selector || len(binding.Selector) > 512 {
		return output.Validation("selector_invalid", "Skill selector is invalid")
	}
	return nil
}

func validateFingerprint(fingerprint string) error {
	if !strings.HasPrefix(fingerprint, "sha256:") || len(fingerprint) != len("sha256:")+64 {
		return output.Internal("delegated_publication_fingerprint", "delegated publication fingerprint is invalid", nil)
	}
	for _, value := range fingerprint[len("sha256:"):] {
		if (value < '0' || value > '9') && (value < 'a' || value > 'f') {
			return output.Internal("delegated_publication_fingerprint", "delegated publication fingerprint is invalid", nil)
		}
	}
	return nil
}

func validClientRequestID(value string) bool {
	return value != "" && len(value) <= 128 && strings.TrimSpace(value) == value
}

func encodeDelegatedGrantEnvelope(envelope delegatedGrantEnvelope) (string, error) {
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return "", output.Internal("delegated_grant_encode", "failed to encode delegated grant recovery state", err)
	}
	return string(encoded), nil
}

func newDelegatedRecoveryEnvelope(clientRequestID string, binding DelegatedPublicationBinding) delegatedRecoveryEnvelope {
	return delegatedRecoveryEnvelope{
		Version:         delegatedRecoveryVersion,
		ClientRequestID: clientRequestID,
		Publication:     binding,
	}
}

func (envelope delegatedRecoveryEnvelope) resume() DelegatedPublicationResume {
	return DelegatedPublicationResume{
		Bound:              true,
		ClientRequestID:    envelope.ClientRequestID,
		IntentFingerprint:  envelope.Publication.IntentFingerprint,
		RequestFingerprint: envelope.Publication.RequestFingerprint,
		ResolutionID:       envelope.Publication.ResolutionID,
		Selector:           envelope.Publication.Selector,
	}
}

func (envelope delegatedRecoveryEnvelope) matches(clientRequestID string, binding DelegatedPublicationBinding) bool {
	return envelope.ClientRequestID == clientRequestID && envelope.Publication == binding
}

func (m *DelegatedGrantManager) loadRecoveryLocked(reference string) (delegatedRecoveryEnvelope, bool, error) {
	value, err := m.Store.Get(m.recoveryKey(reference))
	if errors.Is(err, securestore.ErrNotFound) {
		return delegatedRecoveryEnvelope{}, false, nil
	}
	if err != nil {
		return delegatedRecoveryEnvelope{}, false, output.Authentication("delegated_grant_store_unavailable", "could not read delegated publication recovery metadata").WithCause(err)
	}
	var envelope delegatedRecoveryEnvelope
	if decodeErr := json.Unmarshal([]byte(value), &envelope); decodeErr != nil || envelope.Version != delegatedRecoveryVersion || !validClientRequestID(envelope.ClientRequestID) {
		return delegatedRecoveryEnvelope{}, false, output.Authentication("delegated_grant_invalid", "stored delegated grant recovery metadata is invalid")
	}
	if validateErr := validatePublicationBinding(envelope.Publication); validateErr != nil {
		return delegatedRecoveryEnvelope{}, false, output.Authentication("delegated_grant_invalid", "stored delegated grant recovery metadata is invalid")
	}
	return envelope, true, nil
}

func (m *DelegatedGrantManager) storeRecoveryLocked(reference string, envelope delegatedRecoveryEnvelope) error {
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return output.Internal("delegated_grant_encode", "failed to encode delegated grant recovery metadata", err)
	}
	if err := m.Store.Set(m.recoveryKey(reference), string(encoded)); err != nil {
		return output.Authentication("delegated_grant_store_unavailable", "could not persist delegated publication recovery metadata").WithCause(err)
	}
	return nil
}

func (m *DelegatedGrantManager) deleteLocked(reference string) error {
	err := m.Store.Delete(m.key(reference))
	if err != nil && !errors.Is(err, securestore.ErrNotFound) {
		return output.Authentication("delegated_grant_store_unavailable", "could not delete the delegated grant from the operating system keychain").WithCause(err)
	}
	return m.deleteRecoveryLocked(reference)
}

func (m *DelegatedGrantManager) deleteRecoveryLocked(reference string) error {
	err := m.Store.Delete(m.recoveryKey(reference))
	if errors.Is(err, securestore.ErrNotFound) {
		return nil
	}
	if err != nil {
		return output.Authentication("delegated_grant_store_unavailable", "could not delete delegated publication recovery metadata").WithCause(err)
	}
	return nil
}

func (m *DelegatedGrantManager) withReferenceLock(reference string, operation func() error) (err error) {
	lockDirectory := strings.TrimSpace(m.LockDir)
	if lockDirectory == "" {
		return output.Internal("delegated_grant_lock_unavailable", "delegated grant lock directory is not configured", nil)
	}
	if mkdirErr := os.MkdirAll(lockDirectory, delegatedGrantLockPermission); mkdirErr != nil {
		return output.Authentication("delegated_grant_store_unavailable", "could not create the delegated grant lock directory").WithCause(mkdirErr)
	}
	digest := sha256.Sum256([]byte(m.key(reference)))
	referenceLock := flock.New(filepath.Join(lockDirectory, fmt.Sprintf("%x.lock", digest[:])))
	if lockErr := referenceLock.Lock(); lockErr != nil {
		return output.Authentication("delegated_grant_store_unavailable", "could not lock delegated grant recovery state").WithCause(lockErr)
	}
	defer func() {
		if unlockErr := referenceLock.Unlock(); err == nil && unlockErr != nil {
			err = output.Authentication("delegated_grant_store_unavailable", "could not unlock delegated grant recovery state").WithCause(unlockErr)
		}
	}()
	return operation()
}

func delegatedPublicationMismatch() error {
	return output.Validation(
		"delegated_grant_request_mismatch",
		"delegated grant reference is already bound to a different publication request; retry the original request or delete the reference",
	)
}

func (m *DelegatedGrantManager) newID() string {
	if m.NewID != nil {
		if value := strings.TrimSpace(m.NewID()); value != "" && len(value) <= 128 {
			return value
		}
	}
	var value [16]byte
	if _, err := rand.Read(value[:]); err == nil {
		value[6] = (value[6] & 0x0f) | 0x40
		value[8] = (value[8] & 0x3f) | 0x80
		return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
	}
	return fmt.Sprintf("request-%d", time.Now().UnixNano())
}
