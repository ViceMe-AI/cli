package auth

import (
	"errors"
	"testing"

	"github.com/ViceMe-AI/cli/internal/output"
)

type failingStore struct{}

func (failingStore) Get(string) (string, error) { return "", errors.New("no keychain") }
func (failingStore) Set(string, string) error   { return errors.New("no keychain") }
func (failingStore) Delete(string) error        { return errors.New("no keychain") }

func TestSecureStoreFailsClosed(t *testing.T) {
	t.Parallel()
	manager := &Manager{Store: failingStore{}, Profile: "default"}
	if err := manager.Save(Credential{AccessToken: "secret"}); err == nil {
		t.Fatal("expected save failure")
	} else {
		var cliError *output.Error
		if !errors.As(err, &cliError) || cliError.Subtype != "keychain_unavailable" {
			t.Fatalf("unexpected save error: %#v", err)
		}
	}
	if _, err := manager.Load(); err == nil {
		t.Fatal("expected load failure")
	} else {
		var cliError *output.Error
		if !errors.As(err, &cliError) || cliError.Subtype != "keychain_unavailable" {
			t.Fatalf("unexpected load error: %#v", err)
		}
	}
}
