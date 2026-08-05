package auth

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/gofrs/flock"
)

type Credential struct {
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token,omitempty"`
	TokenType        string    `json:"token_type,omitempty"`
	ExpiresAt        time.Time `json:"expires_at,omitempty"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at,omitempty"`
	RefreshRequestID string    `json:"refresh_request_id,omitempty"`
	UserID           string    `json:"user_id,omitempty"`
	Scope            []string  `json:"scope,omitempty"`
}

type Status struct {
	Authenticated    bool       `json:"authenticated"`
	Profile          string     `json:"profile"`
	Region           string     `json:"region"`
	UserID           string     `json:"user_id,omitempty"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	RefreshExpiresAt *time.Time `json:"refresh_expires_at,omitempty"`
}

type Manager struct {
	Store       securestore.Store
	Region      string
	ProfileID   string
	ProfileName string
	// Scope overrides the region namespace for custom API origins. It is still
	// nested under ProfileID so credentials never cross profiles.
	Scope string
	// LockRoot is the CLI-owned private configuration directory used for an
	// OS-level lock around refresh credential read/modify/write operations.
	LockRoot string
	Now      func() time.Time
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

// WithCredentialLock serializes a complete credential transaction across CLI
// processes. Atomic Store.Set calls alone cannot protect Load -> refresh -> Set
// from two processes generating different recovery request IDs.
func (m *Manager) WithCredentialLock(ctx context.Context, action func() error) error {
	if strings.TrimSpace(m.LockRoot) == "" {
		return output.Authentication("credential_lock_unavailable", "the credential transaction lock directory is not configured")
	}
	lockDirectory := filepath.Join(m.LockRoot, "locks")
	if err := os.MkdirAll(lockDirectory, 0o700); err != nil {
		return output.Authentication("credential_lock_unavailable", "could not create the credential transaction lock directory").WithCause(err)
	}
	digest := sha256.Sum256([]byte(m.key()))
	fileLock := flock.New(filepath.Join(lockDirectory, fmt.Sprintf("credential-%x.lock", digest[:])))
	locked, err := fileLock.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil || !locked {
		return output.Authentication("credential_lock_unavailable", "could not acquire the credential transaction lock").WithCause(err)
	}
	var actionErr error
	var unlockErr error
	func() {
		defer func() { unlockErr = fileLock.Unlock() }()
		actionErr = action()
	}()
	if actionErr != nil {
		return actionErr
	}
	if unlockErr != nil {
		return output.Internal("credential_lock_release", "could not release the credential transaction lock", unlockErr)
	}
	return nil
}

// PreflightSave verifies the complete local persistence path before a device
// authorization is created or its one-time code is exchanged.
func (m *Manager) PreflightSave() error {
	probe, ok := m.Store.(securestore.PersistenceProbe)
	if !ok {
		return output.Authentication("credential_store_unavailable", "the local credential store cannot be verified before device authorization").
			WithHint("use a supported ViceMe credential store and retry; no device authorization was consumed")
	}
	if err := probe.Preflight(m.key()); err != nil {
		return output.Authentication("credential_store_unavailable", "the local credential store is not writable from this process").
			WithHint("verify that the ViceMe configuration directory is writable, then retry; no device authorization was consumed").
			WithCause(err)
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
	if err := m.Store.Set(m.key(), string(data)); err != nil {
		return output.Authentication("credential_store_unavailable", "could not save credentials in the secure local credential store").
			WithHint("verify that the ViceMe configuration directory is private and writable, then start a new login flow").
			WithCause(err)
	}
	return nil
}

func (m *Manager) Load() (Credential, error) {
	value, err := m.Store.Get(m.key())
	if errors.Is(err, securestore.ErrNotFound) {
		return Credential{}, output.Authentication("not_logged_in", "not logged in to ViceMe")
	}
	if err != nil {
		return Credential{}, output.Authentication("credential_store_unavailable", "could not read credentials from the secure local credential store").
			WithHint("on macOS, run 'viceme config keychain-downgrade' once from an interactive Terminal when this process is a Codex or Claude Code sandbox").
			WithCause(err)
	}
	var credential Credential
	if err := json.Unmarshal([]byte(value), &credential); err != nil || credential.AccessToken == "" {
		return Credential{}, output.Authentication("credential_invalid", "stored ViceMe credentials are invalid")
	}
	return credential, nil
}

func (m *Manager) Delete() error {
	err := m.Store.Delete(m.key())
	if errors.Is(err, securestore.ErrNotFound) {
		return nil
	}
	if err != nil {
		return output.Authentication("credential_store_unavailable", "could not remove credentials from the secure local credential store").
			WithHint("unlock the operating-system credential manager; on macOS sandboxes, run 'viceme config keychain-downgrade' once from an interactive Terminal").
			WithCause(err)
	}
	return nil
}

func (m *Manager) Token(_ context.Context) (string, error) {
	credential, err := m.Load()
	if err != nil {
		return "", err
	}
	if !credential.ExpiresAt.IsZero() && !m.now().Before(credential.ExpiresAt) {
		return "", output.Authentication("token_expired", "ViceMe login has expired; run 'viceme auth login'")
	}
	return credential.AccessToken, nil
}

func (m *Manager) CurrentStatus() (Status, error) {
	credential, err := m.Load()
	if err != nil {
		var cliErr *output.Error
		if errors.As(err, &cliErr) && cliErr.Subtype == "not_logged_in" {
			return Status{Authenticated: false, Profile: m.profile(), Region: m.region()}, nil
		}
		return Status{}, err
	}
	now := m.now()
	status := Status{Authenticated: true, Profile: m.profile(), Region: m.region(), UserID: credential.UserID}
	if !credential.ExpiresAt.IsZero() {
		expires := credential.ExpiresAt
		status.ExpiresAt = &expires
	}
	if !credential.RefreshExpiresAt.IsZero() {
		refreshExpires := credential.RefreshExpiresAt
		status.RefreshExpiresAt = &refreshExpires
	}
	accessValid := credential.ExpiresAt.IsZero() || now.Before(credential.ExpiresAt)
	refreshValid := credential.RefreshToken != "" &&
		(credential.RefreshExpiresAt.IsZero() || now.Before(credential.RefreshExpiresAt))
	status.Authenticated = credential.AccessToken != "" && (accessValid || refreshValid)
	return status, nil
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
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
