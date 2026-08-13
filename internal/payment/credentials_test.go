package payment

import (
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/securestore"
)

func TestAPIKeyIsNamespacedAndNeverReturnedAsMetadata(t *testing.T) {
	store := securestore.NewMemory()
	manager := Manager{Store: store, ProfileID: "profile-a", Scope: "https://api.example.test"}
	want := APIKeyCredential{APIKey: "vcp_sandbox_secret", CredentialID: "credential-id", EnvironmentID: "environment-id"}
	if err := manager.SaveAPIKey(want); err != nil {
		t.Fatal(err)
	}
	got, err := manager.LoadAPIKey(want.EnvironmentID)
	if err != nil || got != want {
		t.Fatalf("unexpected credential: got=%#v err=%v", got, err)
	}
	if strings.Contains(manager.key("api-key", want.EnvironmentID), want.APIKey) {
		t.Fatal("secure-store namespace leaked the Payment API Key")
	}
}

func TestAPIKeyCreationPreflightRejectsAnExistingCredential(t *testing.T) {
	store := securestore.NewMemory()
	manager := Manager{Store: store, ProfileID: "profile-a", Scope: "https://api.example.test"}
	if err := manager.EnsureAPIKeyAbsent("environment-id"); err != nil {
		t.Fatalf("empty secure store was rejected: %v", err)
	}
	if err := manager.SaveAPIKey(APIKeyCredential{
		APIKey:        "vcp_sandbox_secret",
		CredentialID:  "credential-id",
		EnvironmentID: "environment-id",
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.EnsureAPIKeyAbsent("environment-id"); err == nil {
		t.Fatal("existing Payment API Key was not rejected")
	}
}

func TestWebhookCredentialCanBeLoadedWithoutEnteringMetadata(t *testing.T) {
	store := securestore.NewMemory()
	manager := Manager{Store: store, ProfileID: "profile-a", Scope: "https://api.example.test"}
	want := WebhookCredential{SigningSecret: "whsec_sandbox_secret", SigningKeyID: "key-id", EndpointID: "endpoint-id", EnvironmentID: "environment-id"}
	if err := manager.SaveWebhook(want); err != nil {
		t.Fatal(err)
	}
	got, err := manager.LoadWebhook(want.EndpointID, want.EnvironmentID)
	if err != nil || got != want {
		t.Fatalf("unexpected Webhook credential: got=%#v err=%v", got, err)
	}
	if strings.Contains(manager.key("webhook", want.EndpointID), want.SigningSecret) {
		t.Fatal("secure-store namespace leaked the Webhook signing secret")
	}
	if _, err := manager.LoadWebhook(want.EndpointID, "other-environment"); err == nil {
		t.Fatal("Webhook signing secret was loaded for a different environment")
	}
}
