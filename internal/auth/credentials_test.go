package auth

import (
	"errors"
	"testing"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
)

type failingStore struct{}

func (failingStore) Get(string) (string, error) { return "", errors.New("no keychain") }
func (failingStore) Set(string, string) error   { return errors.New("no keychain") }
func (failingStore) Delete(string) error        { return errors.New("no keychain") }

func TestSecureStoreFailsClosed(t *testing.T) {
	t.Parallel()
	manager := &Manager{Store: failingStore{}}
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

func TestCredentialsAreIsolatedByRegion(t *testing.T) {
	t.Parallel()
	store := securestore.NewMemory()
	cn := &Manager{Store: store, Region: "cn"}
	global := &Manager{Store: store, Region: "global"}
	if err := cn.Save(Credential{AccessToken: "cn-token"}); err != nil {
		t.Fatal(err)
	}
	if err := global.Save(Credential{AccessToken: "global-token"}); err != nil {
		t.Fatal(err)
	}
	cnCredential, err := cn.Load()
	if err != nil {
		t.Fatal(err)
	}
	globalCredential, err := global.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cnCredential.AccessToken != "cn-token" || globalCredential.AccessToken != "global-token" {
		t.Fatalf("credentials crossed regions: cn=%q global=%q", cnCredential.AccessToken, globalCredential.AccessToken)
	}
}
