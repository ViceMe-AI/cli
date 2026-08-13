package payment

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
)

type APIKeyCredential struct {
	APIKey        string `json:"api_key"`
	CredentialID  string `json:"credential_id"`
	EnvironmentID string `json:"environment_id"`
}

type WebhookCredential struct {
	SigningSecret string `json:"signing_secret"`
	SigningKeyID  string `json:"signing_key_id"`
	EndpointID    string `json:"endpoint_id"`
	EnvironmentID string `json:"environment_id"`
}

type Manager struct {
	Store     securestore.Store
	ProfileID string
	Scope     string
}

func (manager Manager) key(kind, ownerID string) string {
	profileID := strings.TrimSpace(manager.ProfileID)
	if profileID == "" {
		profileID = "default"
	}
	scope := strings.TrimSpace(manager.Scope)
	if scope == "" {
		scope = "cn"
	}
	return "payment:" + profileID + ":" + scope + ":" + kind + ":" + ownerID
}

func (manager Manager) PreflightAPIKey(environmentID string) error {
	return manager.preflight(manager.key("api-key", environmentID))
}

func (manager Manager) EnsureAPIKeyAbsent(environmentID string) error {
	_, err := manager.Store.Get(manager.key("api-key", environmentID))
	if err == nil {
		return output.Policy("PAYMENT_API_KEY_ALREADY_STORED", "a Payment API Key is already stored for this project environment").WithHint("use 'viceme payment api-key rotate' or revoke the existing key before creating another")
	}
	if errors.Is(err, securestore.ErrNotFound) {
		return nil
	}
	return output.Authentication("PAYMENT_SECRET_STORE_UNAVAILABLE", "could not inspect the Payment secret store").WithCause(err)
}

func (manager Manager) PreflightWebhook(environmentID string) error {
	return manager.preflight(manager.key("webhook-preflight", environmentID))
}

func (manager Manager) preflight(key string) error {
	probe, ok := manager.Store.(securestore.PersistenceProbe)
	if !ok {
		return output.Authentication("PAYMENT_SECRET_STORE_UNAVAILABLE", "the Payment secret store cannot be verified")
	}
	if err := probe.Preflight(key); err != nil {
		return output.Authentication("PAYMENT_SECRET_STORE_UNAVAILABLE", "the Payment secret store is not writable").WithCause(err)
	}
	return nil
}

func (manager Manager) SaveAPIKey(credential APIKeyCredential) error {
	if credential.APIKey == "" || credential.CredentialID == "" || credential.EnvironmentID == "" {
		return output.Internal("PAYMENT_API_KEY_INVALID", "ViceMe returned an incomplete Payment API Key", nil)
	}
	return manager.save(manager.key("api-key", credential.EnvironmentID), credential)
}

func (manager Manager) LoadAPIKey(environmentID string) (APIKeyCredential, error) {
	var credential APIKeyCredential
	if err := manager.load(manager.key("api-key", environmentID), &credential, output.Authentication("PAYMENT_API_KEY_NOT_STORED", "no Payment API Key is stored for this project environment").WithHint("run 'viceme payment api-key create' first")); err != nil {
		return APIKeyCredential{}, err
	}
	if credential.APIKey == "" || credential.CredentialID == "" || credential.EnvironmentID != environmentID {
		return APIKeyCredential{}, output.Authentication("PAYMENT_API_KEY_INVALID", "the stored Payment API Key is invalid")
	}
	return credential, nil
}

func (manager Manager) DeleteAPIKey(environmentID string) error {
	return manager.delete(manager.key("api-key", environmentID))
}

func (manager Manager) SaveWebhook(credential WebhookCredential) error {
	if credential.SigningSecret == "" || credential.SigningKeyID == "" || credential.EndpointID == "" || credential.EnvironmentID == "" {
		return output.Internal("WEBHOOK_SECRET_INVALID", "ViceMe returned an incomplete Webhook signing secret", nil)
	}
	return manager.save(manager.key("webhook", credential.EndpointID), credential)
}

func (manager Manager) LoadWebhook(endpointID, environmentID string) (WebhookCredential, error) {
	var credential WebhookCredential
	if err := manager.load(manager.key("webhook", endpointID), &credential, output.Authentication("WEBHOOK_SECRET_NOT_STORED", "no Webhook signing secret is stored for this endpoint").WithHint("run 'viceme payment webhook create' first")); err != nil {
		return WebhookCredential{}, err
	}
	if credential.SigningSecret == "" || credential.SigningKeyID == "" || credential.EndpointID != endpointID || credential.EnvironmentID != environmentID {
		return WebhookCredential{}, output.Authentication("WEBHOOK_SECRET_INVALID", "the stored Webhook signing secret is invalid")
	}
	return credential, nil
}

func (manager Manager) DeleteWebhook(endpointID string) error {
	return manager.delete(manager.key("webhook", endpointID))
}

func (manager Manager) save(key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return output.Internal("PAYMENT_SECRET_ENCODE_FAILED", "could not encode Payment secret metadata", err)
	}
	if err := manager.Store.Set(key, string(data)); err != nil {
		return output.Authentication("PAYMENT_SECRET_STORE_UNAVAILABLE", "could not save the Payment secret in the secure local store").WithCause(err)
	}
	return nil
}

func (manager Manager) load(key string, destination any, notFound error) error {
	value, err := manager.Store.Get(key)
	if errors.Is(err, securestore.ErrNotFound) {
		return notFound
	}
	if err != nil {
		return output.Authentication("PAYMENT_SECRET_STORE_UNAVAILABLE", "could not read the Payment secret from the secure local store").WithCause(err)
	}
	if err := json.Unmarshal([]byte(value), destination); err != nil {
		return output.Authentication("PAYMENT_SECRET_INVALID", "the stored Payment secret is invalid")
	}
	return nil
}

func (manager Manager) delete(key string) error {
	err := manager.Store.Delete(key)
	if errors.Is(err, securestore.ErrNotFound) {
		return nil
	}
	if err != nil {
		return output.Authentication("PAYMENT_SECRET_STORE_UNAVAILABLE", "could not remove the Payment secret from the secure local store").WithCause(err)
	}
	return nil
}
