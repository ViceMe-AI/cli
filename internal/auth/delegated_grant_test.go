package auth

import (
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/securestore"
)

func TestDelegatedGrantManagerIsolatesRegionsAndNeverReturnsSecretFromStatus(t *testing.T) {
	t.Parallel()
	store := securestore.NewMemory()
	first := &DelegatedGrantManager{Store: store, Region: "cn"}
	second := &DelegatedGrantManager{Store: store, Region: "global"}
	secret := strings.Repeat("a", 43)
	if err := first.Save("creator-one", secret); err != nil {
		t.Fatal(err)
	}
	status, err := first.Status("creator-one")
	if err != nil || !status.Stored || status.CredentialRef != "creator-one" {
		t.Fatalf("unexpected status: %#v err=%v", status, err)
	}
	if strings.Contains(status.CredentialRef, secret) {
		t.Fatal("status leaked the credential")
	}
	if _, err := second.Load("creator-one"); err == nil {
		t.Fatal("credential crossed region namespace")
	}
	loaded, err := first.Load("creator-one")
	if err != nil || loaded != secret {
		t.Fatalf("load failed: %q err=%v", loaded, err)
	}
}

func TestDelegatedGrantCredentialRejectsWhitespaceAndHeaderInjection(t *testing.T) {
	t.Parallel()
	for _, value := range []string{
		"too-short",
		strings.Repeat("a", 32) + " value",
		strings.Repeat("a", 32) + "\r\nx-leak: yes",
	} {
		if _, err := NormalizeDelegatedGrantCredential(value); err == nil {
			t.Fatalf("accepted unsafe credential %q", value)
		}
	}
}
