//go:build darwin

package command

import (
	"strings"
	"testing"

	credentialauth "github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

type recordingDowngradeStore struct {
	*securestore.MemoryStore
	keys []string
}

func (s *recordingDowngradeStore) DowngradeKeychain(keys []string) (securestore.KeychainDowngradeResult, error) {
	s.keys = append([]string(nil), keys...)
	return securestore.KeychainDowngradeResult{
		Status:              "copied_keychain_master_key",
		MasterKeyPath:       "/private/config/credentials/master.key.file",
		MigratedCredentials: 2,
	}, nil
}

func TestConfigKeychainDowngradeEnumeratesProfileAndOriginScopesWithoutSecrets(t *testing.T) {
	configBase := t.TempDir()
	configured := config.Default(config.RegionCN)
	work, err := configured.AddProfile("work", config.RegionGlobal)
	if err != nil {
		t.Fatal(err)
	}
	work.APIBaseURL = "https://staging.viceme.example/api"
	if _, err := config.Save(configBase, configured); err != nil {
		t.Fatal(err)
	}
	store := &recordingDowngradeStore{MemoryStore: securestore.NewMemory()}
	code, stdout, stderr, _ := runCLIWithDependencies(t, nil, store, "", Dependencies{
		Environment: skillcontent.Environment{Home: t.TempDir(), ConfigDir: configBase},
	}, "config", "keychain-downgrade")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"status":"copied_keychain_master_key"`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}

	expectedScope, err := credentialScopeForAPIBase(work.APIBaseURL, work.Region)
	if err != nil {
		t.Fatal(err)
	}
	expectedKey := (&credentialauth.Manager{ProfileID: work.ID, Region: string(work.Region), Scope: expectedScope}).StorageKey()
	if !containsString(store.keys, expectedKey) {
		t.Fatalf("custom profile/origin key missing from downgrade: keys=%v", store.keys)
	}
	for _, output := range []string{stdout, stderr} {
		if strings.Contains(output, "access_token") || strings.Contains(output, "credential:") {
			t.Fatalf("downgrade output exposed credential schema or secret material: %s", output)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
