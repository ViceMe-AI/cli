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
		if !errors.As(err, &cliError) || cliError.Subtype != "credential_store_unavailable" {
			t.Fatalf("unexpected save error: %#v", err)
		}
	}
	if _, err := manager.Load(); err == nil {
		t.Fatal("expected load failure")
	} else {
		var cliError *output.Error
		if !errors.As(err, &cliError) || cliError.Subtype != "credential_store_unavailable" {
			t.Fatalf("unexpected load error: %#v", err)
		}
	}
}

func TestCredentialStorePreflightFailsClosedWhenUnsupported(t *testing.T) {
	t.Parallel()
	err := (&Manager{Store: failingStore{}}).PreflightSave()
	var cliError *output.Error
	if !errors.As(err, &cliError) || cliError.Subtype != "credential_store_unavailable" {
		t.Fatalf("unexpected preflight error: %#v", err)
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

func TestCustomCredentialScopesNeverReadProductionRegionCredentials(t *testing.T) {
	t.Parallel()
	store := securestore.NewMemory()
	production := &Manager{Store: store, Region: "cn"}
	customA := &Manager{Store: store, Region: "cn", Scope: "custom:origin-a"}
	customB := &Manager{Store: store, Region: "cn", Scope: "custom:origin-b"}
	if err := production.Save(Credential{AccessToken: "production-token"}); err != nil {
		t.Fatal(err)
	}
	if _, err := customA.Load(); err == nil {
		t.Fatal("custom API scope read the production credential")
	}
	if err := customA.Save(Credential{AccessToken: "custom-token"}); err != nil {
		t.Fatal(err)
	}
	if _, err := customB.Load(); err == nil {
		t.Fatal("credential crossed custom API origins")
	}
	loaded, err := production.Load()
	if err != nil || loaded.AccessToken != "production-token" {
		t.Fatalf("legacy production credential changed: %#v err=%v", loaded, err)
	}
}

func TestCredentialsAreIsolatedByProfile(t *testing.T) {
	t.Parallel()
	store := securestore.NewMemory()
	personal := &Manager{Store: store, Region: "cn", ProfileID: "personal", ProfileName: "personal"}
	work := &Manager{Store: store, Region: "cn", ProfileID: "work", ProfileName: "work"}
	if err := personal.Save(Credential{AccessToken: "personal-token"}); err != nil {
		t.Fatal(err)
	}
	if err := work.Save(Credential{AccessToken: "work-token"}); err != nil {
		t.Fatal(err)
	}
	personalCredential, err := personal.Load()
	if err != nil {
		t.Fatal(err)
	}
	workCredential, err := work.Load()
	if err != nil {
		t.Fatal(err)
	}
	if personalCredential.AccessToken != "personal-token" || workCredential.AccessToken != "work-token" {
		t.Fatalf("credentials crossed profiles: personal=%q work=%q", personalCredential.AccessToken, workCredential.AccessToken)
	}
}

func TestDefaultProfileDoesNotReadLegacyRegionCredential(t *testing.T) {
	t.Parallel()
	store := securestore.NewMemory()
	if err := store.Set("credential:cn", `{"access_token":"legacy-token"}`); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{Store: store, Region: "cn", ProfileID: "default", ProfileName: "default"}
	if _, err := manager.Load(); err == nil {
		t.Fatal("legacy region credential must not be read or migrated")
	}
	if value, err := store.Get("credential:cn"); err != nil || value == "" {
		t.Fatalf("legacy credential was unexpectedly modified: value=%q err=%v", value, err)
	}
}

func TestCustomCredentialScopesAreIsolatedByProfile(t *testing.T) {
	t.Parallel()
	store := securestore.NewMemory()
	personal := &Manager{Store: store, Region: "cn", ProfileID: "personal", Scope: "custom:origin"}
	work := &Manager{Store: store, Region: "cn", ProfileID: "work", Scope: "custom:origin"}
	if err := personal.Save(Credential{AccessToken: "personal-custom-token"}); err != nil {
		t.Fatal(err)
	}
	if _, err := work.Load(); err == nil {
		t.Fatal("custom API credential crossed profiles")
	}
}
