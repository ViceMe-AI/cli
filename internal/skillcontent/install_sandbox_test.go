package skillcontent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ViceMe-AI/cli/internal/privatefile"
)

// denyDirectoryMutations simulates an agent sandbox that allows plain file
// writes but denies every rename and remove, matching the observed WorkBuddy
// Seatbelt behavior.
func denyDirectoryMutations(t *testing.T) {
	t.Helper()
	originalRename, originalRemoveAll := renamePath, removeAllPath
	renamePath = func(oldName, _ string) error {
		return fmt.Errorf("rename %s: %w", oldName, syscall.EPERM)
	}
	removeAllPath = func(target string) error {
		return fmt.Errorf("remove %s: %w", target, syscall.EPERM)
	}
	t.Cleanup(func() {
		renamePath, removeAllPath = originalRename, originalRemoveAll
	})
}

func rewriteTestSkill(t *testing.T, root, name, extraSection string, files map[string]string) {
	t.Helper()
	skill := filepath.Join(root, name)
	writeTestSkill(t, root, name)
	skillDoc := filepath.Join(skill, "SKILL.md")
	content, err := os.ReadFile(skillDoc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillDoc, append(content, []byte(extraSection)...), 0o644); err != nil {
		t.Fatal(err)
	}
	for relative, body := range files {
		path := filepath.Join(skill, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSandboxDeniedInstallDegradesToPlainWrites(t *testing.T) {
	root := t.TempDir()
	rewriteTestSkill(t, root, "viceme-test", "\n## First\n", nil)
	home := t.TempDir()
	environment := Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli")}
	destination := filepath.Join(home, ".agents", "skills", "viceme-test")

	first := New(os.DirFS(root))
	if report := first.Install("viceme-test", "agents", environment); !report.AllSucceeded {
		t.Fatalf("initial install failed: %#v", report)
	}

	rewriteTestSkill(t, root, "viceme-test", "\n## Second\n", nil)
	denyDirectoryMutations(t)
	degraded := New(os.DirFS(root))
	report := degraded.Install("viceme-test", "agents", environment)
	if !report.AllSucceeded {
		t.Fatalf("sandboxed install did not degrade to plain writes: %#v", report)
	}
	installed, err := os.ReadFile(filepath.Join(destination, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(installed), "## Second") {
		t.Fatalf("destination was not updated through the degraded write: %q", installed)
	}
	backup := destination + ".viceme-transaction-backup"
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("degraded backup missing: %v", err)
	}
	if !backupIsDegradedDebris(backup) {
		t.Fatal("degraded backup lacks the marker that later installs recognize")
	}
	backupContent, err := os.ReadFile(filepath.Join(backup, "SKILL.md"))
	if err != nil || !strings.Contains(string(backupContent), "## First") {
		t.Fatalf("degraded backup does not hold the previous Skill: %q, %v", backupContent, err)
	}

	// A retry must not be blocked by the degraded backup debris it left behind.
	rewriteTestSkill(t, root, "viceme-test", "\n## Third\n", nil)
	retry := New(os.DirFS(root)).Install("viceme-test", "agents", environment)
	if !retry.AllSucceeded {
		t.Fatalf("retry was blocked by degraded backup debris: %#v", retry)
	}
	installed, err = os.ReadFile(filepath.Join(destination, "SKILL.md"))
	if err != nil || !strings.Contains(string(installed), "## Third") {
		t.Fatalf("retry did not update the destination: %q, %v", installed, err)
	}
}

func TestSandboxDeniedInstallReportsStaleFilesAndRollsBack(t *testing.T) {
	root := t.TempDir()
	rewriteTestSkill(t, root, "viceme-test", "\n## First\n", map[string]string{"references/extra.md": "first-only\n"})
	home := t.TempDir()
	environment := Environment{Home: home, ConfigDir: filepath.Join(home, ".viceme-cli")}
	destination := filepath.Join(home, ".agents", "skills", "viceme-test")

	if report := New(os.DirFS(root)).Install("viceme-test", "agents", environment); !report.AllSucceeded {
		t.Fatalf("initial install failed: %#v", report)
	}
	previous, err := os.ReadFile(filepath.Join(destination, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}

	// The new version drops references/extra.md, which the sandbox cannot
	// remove from the destination, so the tree digest cannot match.
	rewriteTestSkill(t, root, "viceme-test", "\n## Second\n", nil)
	if err := os.Remove(filepath.Join(root, "viceme-test", "references", "extra.md")); err != nil {
		t.Fatal(err)
	}
	denyDirectoryMutations(t)
	report := New(os.DirFS(root)).Install("viceme-test", "agents", environment)
	if report.AllSucceeded {
		t.Fatal("degraded install unexpectedly verified a tree with stale files")
	}
	if len(report.Results) == 0 || !strings.Contains(report.Results[0].Error, "degraded sandbox write") {
		t.Fatalf("install did not explain the sandbox stale-file mismatch: %#v", report)
	}
	// Rollback restored the previous Skill content through plain writes.
	restored, err := os.ReadFile(filepath.Join(destination, "SKILL.md"))
	if err != nil || string(restored) != string(previous) {
		t.Fatalf("rollback did not restore the previous Skill: %q, %v", restored, err)
	}
}

func TestSweepStaleInstallDebrisKeepsFreshArtifacts(t *testing.T) {
	parent := t.TempDir()
	staleStage := filepath.Join(parent, ".viceme-stage-1000")
	freshStage := filepath.Join(parent, ".viceme-stage-2000")
	staleBackup := filepath.Join(parent, "skill.viceme-transaction-backup")
	staleMarker := filepath.Join(parent, "skill.viceme-transaction-backup.viceme-degraded")
	for _, directory := range []string{staleStage, freshStage, staleBackup} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{staleMarker} {
		if err := os.WriteFile(name, []byte("debris\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	staleTime := time.Now().Add(-2 * staleInstallDebrisAge)
	for _, name := range []string{staleStage, staleBackup, staleMarker} {
		if err := os.Chtimes(name, staleTime, staleTime); err != nil {
			t.Fatal(err)
		}
	}

	sweepStaleInstallDebris(parent)
	for _, removed := range []string{staleStage, staleBackup, staleMarker} {
		if _, err := os.Stat(removed); !os.IsNotExist(err) {
			t.Fatalf("stale debris survived the sweep: %s (%v)", removed, err)
		}
	}
	if _, err := os.Stat(freshStage); err != nil {
		t.Fatalf("fresh staging directory removed by the sweep: %v", err)
	}
}

func TestRestoreBackupSkillDegradesWhenRemovalIsDenied(t *testing.T) {
	parent := t.TempDir()
	destination := filepath.Join(parent, "skill")
	backup := filepath.Join(parent, "skill.viceme-transaction-backup")
	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(backup, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "SKILL.md"), []byte("partial new content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backup, "SKILL.md"), []byte("previous content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	denyDirectoryMutations(t)
	if err := restoreBackupSkill(backup, destination); err != nil {
		t.Fatalf("restoreBackupSkill() under denied renames error = %v", err)
	}
	restored, err := os.ReadFile(filepath.Join(destination, "SKILL.md"))
	if err != nil || string(restored) != "previous content\n" {
		t.Fatalf("degraded restore did not recover the backup: %q, %v", restored, err)
	}
	if !privatefile.IsPermissionDenial(fmt.Errorf("probe: %w", syscall.EPERM)) {
		t.Fatal("IsPermissionDenial stopped classifying EPERM")
	}
}
