package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/securestore"
)

const pendingDeviceLoginSchemaVersion = 1

type PendingDeviceLogin struct {
	SchemaVersion   int       `json:"schema_version"`
	ProfileID       string    `json:"profile_id"`
	ProfileName     string    `json:"profile_name"`
	Region          string    `json:"region"`
	APIBaseURL      string    `json:"api_base_url"`
	APIOrigin       string    `json:"api_origin"`
	CredentialScope string    `json:"credential_scope"`
	IntervalSeconds int       `json:"interval_seconds"`
	ExpiresAt       time.Time `json:"expires_at"`
}

func PendingDeviceLoginKey(deviceCode string) string {
	digest := sha256.Sum256([]byte(deviceCode))
	return "device-login:" + hex.EncodeToString(digest[:])
}

func SavePendingDeviceLogin(store securestore.Store, deviceCode string, pending PendingDeviceLogin) error {
	if err := validatePendingDeviceLogin(pending); err != nil {
		return err
	}
	data, err := json.Marshal(pending)
	if err != nil {
		return fmt.Errorf("encode pending device login: %w", err)
	}
	if err := store.Set(PendingDeviceLoginKey(deviceCode), string(data)); err != nil {
		return fmt.Errorf("save pending device login: %w", err)
	}
	return nil
}

func LoadPendingDeviceLogin(store securestore.Store, deviceCode string) (PendingDeviceLogin, error) {
	value, err := store.Get(PendingDeviceLoginKey(deviceCode))
	if err != nil {
		return PendingDeviceLogin{}, err
	}
	var pending PendingDeviceLogin
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&pending); err != nil {
		return PendingDeviceLogin{}, fmt.Errorf("decode pending device login: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return PendingDeviceLogin{}, errors.New("decode pending device login: trailing JSON data")
	}
	if err := validatePendingDeviceLogin(pending); err != nil {
		return PendingDeviceLogin{}, err
	}
	return pending, nil
}

func DeletePendingDeviceLogin(store securestore.Store, deviceCode string) error {
	err := store.Delete(PendingDeviceLoginKey(deviceCode))
	if errors.Is(err, securestore.ErrNotFound) {
		return nil
	}
	return err
}

func validatePendingDeviceLogin(pending PendingDeviceLogin) error {
	if pending.SchemaVersion != pendingDeviceLoginSchemaVersion ||
		strings.TrimSpace(pending.ProfileID) == "" ||
		strings.TrimSpace(pending.ProfileName) == "" ||
		strings.TrimSpace(pending.Region) == "" ||
		strings.TrimSpace(pending.APIBaseURL) == "" ||
		strings.TrimSpace(pending.APIOrigin) == "" ||
		pending.IntervalSeconds < 1 ||
		pending.ExpiresAt.IsZero() {
		return errors.New("pending device login metadata is invalid")
	}
	return nil
}
