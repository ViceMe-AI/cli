package skillcontent

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const suspensionProduct = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
const suspensionPurchase = "https://shop.example.test/purchase"
const suspensionInstallDoc = "https://s3.viceme.cn/start/agent-install.md"

func suspensionFixture(t *testing.T, root, name, product string, trial bool) (string, []byte) {
	t.Helper()
	directory := filepath.Join(root, name)
	for _, relative := range []string{".viceme", "scripts", "references", "outputs"} {
		if err := os.MkdirAll(filepath.Join(directory, relative), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	content := "---\r\nname: " + name + "\r\ntitle: Demo\r\nmetadata:\r\n  author: test # preserve\r\n---\r\n"
	if trial {
		content += "<!-- viceme-trial:v1 product=" + product + " -->\r\n\r\n## 使用前必读\r\n\r\nCheck first.\r\n<!-- /viceme-trial:v1 -->\r\n"
	}
	content += "\r\n# Original author instructions\r\n"
	manifest, _ := json.Marshal(map[string]any{"product_id": product, "release_id": "release-one"})
	for relative, data := range map[string][]byte{
		"SKILL.md": []byte(content), installManifestPath: manifest,
		"scripts/run.sh": []byte("original script"), "references/viceme-runtime.md": []byte("original rules"),
		"outputs/user.md": []byte("user output must stay"),
	} {
		if err := os.WriteFile(filepath.Join(directory, relative), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return directory, []byte(content)
}

func TestTrialSuspensionPreservesFrontmatterAndEveryOtherFile(t *testing.T) {
	home := t.TempDir()
	environment := Environment{Home: home, CodexHome: filepath.Join(home, "custom-codex")}
	known, err := resolveKnownTargets("demo", environment)
	if err != nil {
		t.Fatal(err)
	}
	var directories []string
	for _, target := range known {
		directory, original := suspensionFixture(t, filepath.Dir(target.path), "demo", suspensionProduct, true)
		directories = append(directories, directory)
		before, _ := os.ReadFile(filepath.Join(directory, installManifestPath))
		count, err := SuspendTrialSkills(environment, suspensionProduct, suspensionPurchase, suspensionInstallDoc)
		if err != nil || count != len(directories) {
			t.Fatalf("suspension failed: %d, %v", count, err)
		}
		after, _ := os.ReadFile(filepath.Join(directory, "SKILL.md"))
		frontmatterEnd := bytes.Index(original[5:], []byte("\r\n---\r\n")) + 5 + len("\r\n---\r\n")
		if !bytes.Equal(after[:frontmatterEnd], original[:frontmatterEnd]) || bytes.Contains(after, []byte("Original author instructions")) || !bytes.Contains(after, []byte(TrialDisabledMarker)) || !bytes.Contains(after, []byte("--owned")) {
			t.Fatalf("suspension did not preserve metadata and replace the body: %q", after)
		}
		manifest, _ := os.ReadFile(filepath.Join(directory, installManifestPath))
		if !bytes.Equal(before, manifest) {
			t.Fatal("suspension changed provenance")
		}
		for relative, want := range map[string]string{"scripts/run.sh": "original script", "references/viceme-runtime.md": "original rules", "outputs/user.md": "user output must stay"} {
			data, err := os.ReadFile(filepath.Join(directory, relative))
			if err != nil || string(data) != want {
				t.Fatalf("non-entry file changed: %s, %v", relative, err)
			}
		}
		_, err = SuspendTrialSkills(environment, suspensionProduct, suspensionPurchase, suspensionInstallDoc)
		current, _ := os.ReadFile(filepath.Join(directory, "SKILL.md"))
		if err != nil || !bytes.Equal(current, after) {
			t.Fatalf("repeated suspension was not idempotent: %v", err)
		}
	}
}

func TestTrialSuspensionSkipsOwnedForeignUnmanagedAndSymlinkEntries(t *testing.T) {
	for _, kind := range []string{"owned", "foreign", "unmanaged", "symlink", "author-marker"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			product := suspensionProduct
			if kind == "foreign" {
				product = "other-product"
			}
			directory, original := suspensionFixture(t, filepath.Join(home, ".agents", "skills"), "demo", product, kind != "owned")
			filename := filepath.Join(directory, "SKILL.md")
			if kind == "unmanaged" {
				if err := os.Remove(filepath.Join(directory, installManifestPath)); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "author-marker" {
				original = bytes.Replace(original, []byte("## 使用前必读"), []byte("# Author example of a marker"), 1)
				if err := os.WriteFile(filename, original, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "symlink" {
				external := filepath.Join(t.TempDir(), "external.md")
				if err := os.Rename(filename, external); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(external, filename); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			}
			count, err := SuspendTrialSkills(Environment{Home: home}, suspensionProduct, suspensionPurchase, suspensionInstallDoc)
			after, _ := os.ReadFile(filename)
			if err != nil || count != 0 || !bytes.Equal(after, original) {
				t.Fatalf("unrelated/unsafe entry was changed: count=%d err=%v", count, err)
			}
		})
	}
}

func TestTrialSuspensionPreservesEntryOnPermissionOrRecoveryConflict(t *testing.T) {
	for _, kind := range []string{"permission", "active-install", "pending-recovery"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			directory, original := suspensionFixture(t, filepath.Join(home, ".agents", "skills"), "demo", suspensionProduct, true)
			if kind == "permission" {
				previous := replaceTrialEntry
				replaceTrialEntry = func(string, string) error { return fs.ErrPermission }
				t.Cleanup(func() { replaceTrialEntry = previous })
			} else if kind == "active-install" {
				locks, err := tryAcquireInstallPathLocks([]string{directory})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { releaseInstallPathLocks(locks) })
			} else {
				journal := filepath.Join(home, "pending-install.json")
				if err := os.WriteFile(journal, []byte("pending"), 0o600); err != nil {
					t.Fatal(err)
				}
				normalizedDirectory, err := normalizeInstallPathLockDestination(directory)
				if err != nil {
					t.Fatal(err)
				}
				journal, err = normalizeTransactionPath(journal)
				if err != nil {
					t.Fatal(err)
				}
				if err := writeInstallPathOwner(installPathOwnerFilename(normalizedDirectory), journal); err != nil {
					t.Fatal(err)
				}
			}
			count, err := SuspendTrialSkills(Environment{Home: home}, suspensionProduct, suspensionPurchase, suspensionInstallDoc)
			after, _ := os.ReadFile(filepath.Join(directory, "SKILL.md"))
			if err == nil || count != 0 || !bytes.Equal(after, original) {
				t.Fatalf("unsafe replacement was not blocked: count=%d err=%v", count, err)
			}
		})
	}
}

func TestPythonSuspensionSharesNativeDestinationLockAndNotice(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	home := t.TempDir()
	directory, original := suspensionFixture(t, filepath.Join(home, ".agents", "skills"), "demo", suspensionProduct, true)
	script, err := filepath.Abs("../../skills/use-a-skill/scripts/trial.py")
	if err != nil {
		t.Fatal(err)
	}
	invoke := func(want string) {
		t.Helper()
		program := "import importlib.util,sys\ns=importlib.util.spec_from_file_location('trial',sys.argv[1]); m=importlib.util.module_from_spec(s); s.loader.exec_module(m)\ntry:\n with m.ProductLock(sys.argv[2]):\n  print(m.suspend_trial_skills('cn',sys.argv[2],sys.argv[3]))\nexcept m.Failure as e:\n print(e.code)\n"
		cmd := exec.Command(python, "-c", program, script, suspensionProduct, suspensionPurchase)
		for _, value := range os.Environ() {
			key := strings.SplitN(value, "=", 2)[0]
			if key != "HOME" && key != "CODEX_HOME" && key != "CLAUDE_CONFIG_DIR" && key != "WORKBUDDY_CONFIG_DIR" && key != "VICEME_AGENTS_SKILLS_DIR" && key != "PYTHONDONTWRITEBYTECODE" {
				cmd.Env = append(cmd.Env, value)
			}
		}
		cmd.Env = append(cmd.Env, "HOME="+home, "PYTHONDONTWRITEBYTECODE=1")
		out, err := cmd.CombinedOutput()
		if err != nil || strings.TrimSpace(string(out)) != want {
			t.Fatalf("Python suspension returned %q, %v; want %s", out, err, want)
		}
	}
	locks, err := tryAcquireInstallPathLocks([]string{directory})
	if err != nil {
		t.Fatal(err)
	}
	invoke("TRIAL_SUSPEND_FAILED")
	releaseInstallPathLocks(locks)
	after, _ := os.ReadFile(filepath.Join(directory, "SKILL.md"))
	if !bytes.Equal(after, original) {
		t.Fatal("Python changed the entry while a native install held the destination")
	}
	invoke("1")
	after, _ = os.ReadFile(filepath.Join(directory, "SKILL.md"))
	want, ok := suspendedTrialMarkdown(original, "demo", suspensionProduct, suspensionPurchase, suspensionInstallDoc)
	if !ok || !bytes.Equal(after, want) {
		t.Fatal("Go and Python suspension notices diverged")
	}
	if _, err := os.Stat(filepath.Join(directory, "scripts/run.sh")); errors.Is(err, fs.ErrNotExist) {
		t.Fatal("Python deleted the scripts")
	}
}
