package devsetup

import (
	"encoding/json"
	"github.com/ViceMe-AI/cli/internal/update"
	"github.com/gofrs/flock"
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	cfg := filepath.Join(root, "config")
	_ = os.Mkdir(cfg, 0700)
	binary := filepath.Join(root, "viceme")
	_ = os.WriteFile(binary, []byte("verified-dev-binary"), 0700)
	digest, _ := Digest(binary)
	gen, _ := update.NewStandaloneGeneration("0.45.5", digest)
	if err := update.CommitActiveGeneration(cfg, gen, nil); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(cfg, "config.json"), []byte("profile-preserved"), 0600)
	_ = os.WriteFile(filepath.Join(cfg, "managed-skills.json"), []byte("registry-preserved"), 0600)
	return cfg, binary, digest
}
func TestRetirementPreservesConfigurationAndAllowsSameVersionProduction(t *testing.T) {
	cfg, bin, digest := fixture(t)
	backup, err := Retire(cfg, bin, digest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bin); !os.IsNotExist(err) {
		t.Fatal("dev executable remains")
	}
	if got, _ := Digest(backup); got != digest {
		t.Fatal("backup differs")
	}
	for _, name := range []string{"config.json", "managed-skills.json"} {
		if _, err := os.Stat(filepath.Join(cfg, name)); err != nil {
			t.Fatal(err)
		}
	}
	target, _ := update.NewStandaloneGeneration("0.45.5", "abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd")
	if err := update.ValidateActivationTarget(cfg, target); err != nil {
		t.Fatal("production reinstall still blocked", err)
	}
}
func TestRetirementFailsClosed(t *testing.T) {
	for _, mode := range []string{"hash", "npm", "bootstrap-activation.json", "npm-activation.json", "install-transaction.json", "busy"} {
		t.Run(mode, func(t *testing.T) {
			cfg, bin, digest := fixture(t)
			switch mode {
			case "hash":
				digest = "abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"
			case "npm":
				g, _ := update.NewNPMGeneration("0.45.5")
				data, _ := json.Marshal(g)
				_ = os.WriteFile(filepath.Join(cfg, "active-generation.json"), data, 0600)
			case "busy":
				lock := flock.New(filepath.Join(cfg, update.ActivationLockFilename))
				_ = lock.Lock()
				defer lock.Unlock()
			default:
				_ = os.WriteFile(filepath.Join(cfg, mode), []byte("pending"), 0600)
			}
			if _, err := Retire(cfg, bin, digest); err == nil {
				t.Fatal("unsafe retirement accepted")
			}
			if _, err := os.Stat(bin); err != nil {
				t.Fatal("executable changed")
			}
			if _, err := os.Stat(filepath.Join(cfg, "active-generation.json")); err != nil {
				t.Fatal("generation removed")
			}
		})
	}
}
func TestResumeAfterBinaryRetired(t *testing.T) {
	cfg, bin, digest := fixture(t)
	generation, _, _ := update.ReadActiveGeneration(cfg)
	backup := filepath.Join(filepath.Dir(bin), ".viceme-retired-"+digest)
	state := Retirement{SchemaVersion: 1, Destination: bin, Digest: digest, Backup: backup, Generation: &generation}
	data, _ := json.Marshal(state)
	_ = os.WriteFile(filepath.Join(cfg, "dev-retirement.json"), data, 0600)
	_ = os.Rename(bin, backup)
	if err := Resume(cfg, bin); err != nil {
		t.Fatal(err)
	}
	if _, present, _ := update.ReadActiveGeneration(cfg); present {
		t.Fatal("retired generation remains")
	}
}
func TestResumeNeverRemovesNewGeneration(t *testing.T) {
	cfg, bin, digest := fixture(t)
	generation, _, _ := update.ReadActiveGeneration(cfg)
	state := Retirement{SchemaVersion: 1, Destination: bin, Digest: digest, Backup: filepath.Join(filepath.Dir(bin), ".viceme-retired-"+digest), Generation: &generation}
	data, _ := json.Marshal(state)
	_ = os.WriteFile(filepath.Join(cfg, "dev-retirement.json"), data, 0600)
	newer, _ := update.NewStandaloneGeneration("0.45.6", "abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd")
	_ = update.CommitActiveGeneration(cfg, newer, nil)
	if _, err := Retire(cfg, bin, digest); err == nil {
		t.Fatal("removed newer installation")
	}
	actual, _, _ := update.ReadActiveGeneration(cfg)
	if actual != newer {
		t.Fatal("new generation mutated")
	}
}
