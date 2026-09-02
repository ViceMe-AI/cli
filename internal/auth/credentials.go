package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
)

type Credential struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	UserID       string    `json:"user_id,omitempty"`
}

type Status struct {
	Authenticated      bool       `json:"authenticated"`
	Profile            string     `json:"profile"`
	DistributionRegion string     `json:"distributionRegion"`
	UserID             string     `json:"userId,omitempty"`
	ExpiresAt          *time.Time `json:"expiresAt,omitempty"`
}

type Manager struct {
	Store       securestore.Store
	Region      string
	ProfileID   string
	ProfileName string
	// Scope is derived from the canonical API origin and nested under ProfileID,
	// so credentials never depend on the independently selected update region.
	Scope string
	// LegacyRegion is set only for an official API endpoint. New releases keep
	// that former region key in sync so an interrupted activation can roll back
	// to the previous binary without losing the authenticated session.
	LegacyRegion string
}

func (m *Manager) key() string {
	profileID := m.ProfileID
	if profileID == "" {
		profileID = "default"
	}
	endpointScope := strings.TrimSpace(m.Scope)
	if endpointScope == "" {
		endpointScope = strings.ToLower(strings.TrimSpace(m.Region))
		if endpointScope == "" {
			endpointScope = "cn"
		}
	}
	return "credential:" + profileID + ":" + endpointScope
}

// StorageKey returns the opaque profile + endpoint namespace used by the
// secure store. It contains no credential material and is exposed so the
// controlled macOS downgrade can migrate every configured profile.
func (m *Manager) StorageKey() string { return m.key() }

func (m *Manager) legacyKey() string {
	legacyRegion := strings.ToLower(strings.TrimSpace(m.LegacyRegion))
	if legacyRegion == "" {
		return ""
	}
	legacy := (&Manager{ProfileID: m.ProfileID, Region: legacyRegion}).key()
	if legacy == m.key() {
		return ""
	}
	return legacy
}

func (m *Manager) storageKeys() []string {
	keys := []string{m.key()}
	if legacy := m.legacyKey(); legacy != "" {
		keys = append(keys, legacy)
	}
	return keys
}

// PreflightSave verifies the complete local persistence path before a device
// authorization is created or its one-time code is exchanged.
func (m *Manager) PreflightSave() error {
	probe, ok := m.Store.(securestore.PersistenceProbe)
	if !ok {
		return output.Authentication("credential_store_unavailable", "the local credential store cannot be verified before device authorization").
			WithHint("use a supported ViceMe credential store and retry; no device authorization was consumed")
	}
	for _, key := range m.storageKeys() {
		if err := probe.Preflight(key); err != nil {
			return output.Authentication("credential_store_unavailable", "the local credential store is not writable from this process").
				WithHint("verify that the ViceMe configuration directory is writable and private, then retry; if this command runs inside an agent sandbox, allow writes to the ViceMe configuration directory or run the login from an unsandboxed terminal; no device authorization was consumed").
				WithCause(err)
		}
	}
	return nil
}

func (m *Manager) Save(credential Credential) error {
	if credential.AccessToken == "" {
		return output.Authentication("invalid_token", "the server returned an empty access token")
	}
	data, err := json.Marshal(credential)
	if err != nil {
		return output.Internal("credential_encode", "failed to encode credentials", err)
	}
	primaryKey := m.key()
	previous, previousErr := m.Store.Get(primaryKey)
	if previousErr != nil && !errors.Is(previousErr, securestore.ErrNotFound) {
		return output.Authentication("credential_store_unavailable", "could not read credentials from the secure local credential store before saving").
			WithHint("unlock the operating-system credential manager and retry from an interactive Terminal").
			WithCause(previousErr)
	}
	if err := m.Store.Set(primaryKey, string(data)); err != nil {
		return output.Authentication("credential_store_unavailable", "could not save credentials in the secure local credential store").
			WithHint("verify that the ViceMe configuration directory is private and writable, then start a new login flow").
			WithCause(err)
	}
	if legacyKey := m.legacyKey(); legacyKey != "" {
		if err := m.Store.Set(legacyKey, string(data)); err != nil {
			if errors.Is(previousErr, securestore.ErrNotFound) {
				_ = m.Store.Delete(primaryKey)
			} else {
				_ = m.Store.Set(primaryKey, previous)
			}
			return output.Authentication("credential_store_unavailable", "could not save credentials in the secure local credential store").
				WithHint("verify that the ViceMe configuration directory is private and writable, then start a new login flow").
				WithCause(err)
		}
	}
	return nil
}

func (m *Manager) Load() (Credential, error) {
	value, err := m.Store.Get(m.key())
	legacyFallback := false
	if errors.Is(err, securestore.ErrNotFound) {
		legacyKey := m.legacyKey()
		if legacyKey == "" {
			return Credential{}, output.Authentication("not_logged_in", "not logged in to ViceMe")
		}
		value, err = m.Store.Get(legacyKey)
		if errors.Is(err, securestore.ErrNotFound) {
			return Credential{}, output.Authentication("not_logged_in", "not logged in to ViceMe")
		}
		legacyFallback = err == nil
	}
	if err != nil {
		return Credential{}, output.Authentication("credential_store_unavailable", "could not read credentials from the secure local credential store").
			WithHint("unlock the operating-system credential manager and retry from an interactive Terminal; do not copy or export the access token into the agent sandbox").
			WithCause(err)
	}
	var credential Credential
	if err := json.Unmarshal([]byte(value), &credential); err != nil || credential.AccessToken == "" {
		return Credential{}, output.Authentication("credential_invalid", "stored ViceMe credentials are invalid")
	}
	if legacyFallback {
		if setErr := m.Store.Set(m.key(), value); setErr != nil {
			return Credential{}, output.Authentication("credential_store_unavailable", "could not migrate credentials to the API endpoint namespace").
				WithHint("unlock the operating-system credential manager and retry from an interactive Terminal").
				WithCause(setErr)
		}
	}
	return credential, nil
}

func (m *Manager) Delete() error {
	_, err := DeleteStorageKeys(m.Store, m.storageKeys())
	return err
}

// DeleteStorageKeys removes a set of credential entries as one recoverable
// local operation. Every existing value is read before the first mutation, and
// a partial delete restores the complete snapshot so callers can safely retry.
func DeleteStorageKeys(store securestore.Store, keys []string) (int, error) {
	unique := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		value, err := store.Get(key)
		if errors.Is(err, securestore.ErrNotFound) {
			continue
		}
		if err != nil {
			return 0, credentialDeleteError("could not inspect credentials in the secure local credential store", err)
		}
		unique = append(unique, key)
		values[key] = value
	}

	removed := 0
	for index, key := range unique {
		err := store.Delete(key)
		if err == nil {
			removed++
			continue
		}
		if errors.Is(err, securestore.ErrNotFound) {
			continue
		}
		rollbackErrors := []error{err}
		// Keys after the failing delete were never touched. Restore only the
		// attempted prefix so a concurrent writer to a later key is not replaced
		// by this operation's older snapshot.
		for _, restoreKey := range unique[:index+1] {
			value := values[restoreKey]
			if restoreErr := store.Set(restoreKey, value); restoreErr != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("restore credential %q: %w", restoreKey, restoreErr))
			}
		}
		message := "could not remove credentials from the secure local credential store; the previous credential set was restored"
		if len(rollbackErrors) > 1 {
			message = "could not remove credentials from the secure local credential store; the previous credential set could not be fully restored"
		}
		return 0, credentialDeleteError(
			message,
			errors.Join(rollbackErrors...),
		)
	}
	return removed, nil
}

func credentialDeleteError(message string, cause error) *output.Error {
	return output.Authentication("credential_store_unavailable", message).
		WithHint("unlock the operating-system credential manager and retry from an interactive Terminal").
		WithCause(cause)
}

func (m *Manager) Token(_ context.Context) (string, error) {
	credential, err := m.Load()
	if err != nil {
		return "", err
	}
	if !credential.ExpiresAt.IsZero() && time.Now().After(credential.ExpiresAt) {
		return "", output.Authentication("token_expired", "ViceMe login has expired; run 'viceme auth login'")
	}
	return credential.AccessToken, nil
}

func (m *Manager) CurrentStatus() (Status, error) {
	credential, err := m.Load()
	if err != nil {
		var cliErr *output.Error
		if errors.As(err, &cliErr) && cliErr.Subtype == "not_logged_in" {
			return Status{Authenticated: false, Profile: m.profile(), DistributionRegion: m.region()}, nil
		}
		return Status{}, err
	}
	status := Status{Authenticated: true, Profile: m.profile(), DistributionRegion: m.region(), UserID: credential.UserID}
	if !credential.ExpiresAt.IsZero() {
		expires := credential.ExpiresAt
		status.ExpiresAt = &expires
		if time.Now().After(expires) {
			status.Authenticated = false
		}
	}
	return status, nil
}

func (m *Manager) profile() string {
	if m.ProfileName == "" {
		return "default"
	}
	return m.ProfileName
}

func (m *Manager) region() string {
	if m.Region == "" {
		return "cn"
	}
	return m.Region
}
