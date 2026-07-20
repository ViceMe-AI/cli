package auth

import (
	"context"
	"encoding/json"
	"errors"
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
	Authenticated bool       `json:"authenticated"`
	Profile       string     `json:"profile"`
	Region        string     `json:"region"`
	UserID        string     `json:"user_id,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
}

type Manager struct {
	Store       securestore.Store
	Region      string
	ProfileID   string
	ProfileName string
}

func (m *Manager) key() string {
	profileID := m.ProfileID
	if profileID == "" {
		profileID = "default"
	}
	region := m.Region
	if region == "" {
		region = "cn"
	}
	return "credential:" + profileID + ":" + region
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
		return output.Authentication("keychain_unavailable", "could not save credentials in the operating system keychain").WithCause(err)
	}
	return nil
}

func (m *Manager) Load() (Credential, error) {
	value, err := m.Store.Get(m.key())
	if errors.Is(err, securestore.ErrNotFound) {
		return Credential{}, output.Authentication("not_logged_in", "not logged in to Viceme")
	}
	if err != nil {
		return Credential{}, output.Authentication("keychain_unavailable", "could not read credentials from the operating system keychain").WithCause(err)
	}
	var credential Credential
	if err := json.Unmarshal([]byte(value), &credential); err != nil || credential.AccessToken == "" {
		return Credential{}, output.Authentication("credential_invalid", "stored Viceme credentials are invalid")
	}
	return credential, nil
}

func (m *Manager) Delete() error {
	err := m.Store.Delete(m.key())
	if errors.Is(err, securestore.ErrNotFound) {
		return nil
	}
	if err != nil {
		return output.Authentication("keychain_unavailable", "could not remove credentials from the operating system keychain").WithCause(err)
	}
	return nil
}

func (m *Manager) Token(_ context.Context) (string, error) {
	credential, err := m.Load()
	if err != nil {
		return "", err
	}
	if !credential.ExpiresAt.IsZero() && time.Now().After(credential.ExpiresAt) {
		return "", output.Authentication("token_expired", "Viceme login has expired; run 'viceme auth login'")
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
	status := Status{Authenticated: true, Profile: m.profile(), Region: m.region(), UserID: credential.UserID}
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
