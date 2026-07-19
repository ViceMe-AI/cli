package auth

import (
	"errors"
	"strings"
	"unicode"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
)

const (
	delegatedGrantKeyPrefix = "delegated-grant:"
	maxDelegatedGrantBytes  = 4096
)

type DelegatedGrantStatus struct {
	CredentialRef string `json:"credential_ref"`
	Stored        bool   `json:"stored"`
}

// DelegatedGrantManager keeps one-time delegated publication credentials in
// the same OS keychain boundary as login credentials, but under a distinct
// namespace. Callers only handle a non-sensitive reference after Save.
type DelegatedGrantManager struct {
	Store  securestore.Store
	Region string
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
	if err := m.Store.Set(m.key(reference), normalized); err != nil {
		return output.Authentication("delegated_grant_store_unavailable", "could not save the delegated grant in the operating system keychain").WithCause(err)
	}
	return nil
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
	return NormalizeDelegatedGrantCredential(value)
}

func (m *DelegatedGrantManager) Delete(reference string) error {
	if err := ValidateDelegatedGrantReference(reference); err != nil {
		return err
	}
	err := m.Store.Delete(m.key(reference))
	if errors.Is(err, securestore.ErrNotFound) {
		return nil
	}
	if err != nil {
		return output.Authentication("delegated_grant_store_unavailable", "could not delete the delegated grant from the operating system keychain").WithCause(err)
	}
	return nil
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
	region := strings.ToLower(strings.TrimSpace(m.Region))
	if region == "" {
		region = "cn"
	}
	return delegatedGrantKeyPrefix + region + ":" + reference
}
