package skillcontent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const provenanceTestSkill = "marketplace-skill"

func provenanceTestEnvironment(t *testing.T) Environment {
	t.Helper()
	home := t.TempDir()
	return Environment{
		Home:      home,
		CodexHome: filepath.Join(home, ".codex"),
		ConfigDir: filepath.Join(home, ".viceme-cli"),
	}
}

func provenanceTestBundle(t *testing.T, body string) *Bundle {
	t.Helper()
	root := t.TempDir()
	writeTestSkill(t, root, provenanceTestSkill)
	if body != "" {
		skillFile := filepath.Join(root, provenanceTestSkill, "SKILL.md")
		content, err := os.ReadFile(skillFile)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(skillFile, []byte(string(content)+body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return New(os.DirFS(root))
}

type testManifestProvenance struct {
	ProductID string `json:"product_id"`
	ReleaseID string `json:"release_id"`
}

func readTestManifest(t *testing.T, directory string) testManifestProvenance {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(directory, ".viceme", "install-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest testManifestProvenance
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func TestMarketplaceInstallPersistsProductProvenance(t *testing.T) {
	t.Parallel()
	bundle := provenanceTestBundle(t, "")
	environment := provenanceTestEnvironment(t)
	provenance := SkillProvenance{ProductID: "product-1", ReleaseID: "release-1"}

	report := bundle.InstallWithProvenance(provenanceTestSkill, "agents", environment, provenance)
	if !report.AllSucceeded {
		t.Fatalf("marketplace Skill did not install: %#v", report)
	}
	manifest := readTestManifest(t, filepath.Join(environment.Home, ".agents", "skills", provenanceTestSkill))
	if manifest.ProductID != "product-1" || manifest.ReleaseID != "release-1" {
		t.Fatalf("install manifest lost provenance: %#v", manifest)
	}

	unchanged := bundle.InstallWithProvenance(provenanceTestSkill, "agents", environment, provenance)
	if !unchanged.AllSucceeded || unchanged.Results[0].Status != "unchanged" {
		t.Fatalf("same Product reinstall was not idempotent: %#v", unchanged)
	}
}

func TestMarketplaceInstallAllowsSameProductUpgrade(t *testing.T) {
	t.Parallel()
	environment := provenanceTestEnvironment(t)
	first := provenanceTestBundle(t, "")
	provenance := SkillProvenance{ProductID: "product-1", ReleaseID: "release-1"}
	if report := first.InstallWithProvenance(provenanceTestSkill, "agents", environment, provenance); !report.AllSucceeded {
		t.Fatalf("initial install failed: %#v", report)
	}

	second := provenanceTestBundle(t, "\n## More\n")
	upgrade := SkillProvenance{ProductID: "product-1", ReleaseID: "release-2"}
	report := second.InstallWithProvenance(provenanceTestSkill, "agents", environment, upgrade)
	if !report.AllSucceeded || report.Results[0].Status != "updated" {
		t.Fatalf("same Product upgrade was refused: %#v", report)
	}
	manifest := readTestManifest(t, filepath.Join(environment.Home, ".agents", "skills", provenanceTestSkill))
	if manifest.ProductID != "product-1" || manifest.ReleaseID != "release-2" {
		t.Fatalf("upgrade did not refresh provenance: %#v", manifest)
	}
	content, err := os.ReadFile(filepath.Join(environment.Home, ".agents", "skills", provenanceTestSkill, "SKILL.md"))
	if err != nil || !strings.Contains(string(content), "## More") {
		t.Fatalf("upgrade did not replace content: %v", err)
	}
}

func TestMarketplaceInstallRefusesForeignOfficialAndUserDirectories(t *testing.T) {
	t.Parallel()

	t.Run("different Product", func(t *testing.T) {
		t.Parallel()
		environment := provenanceTestEnvironment(t)
		bundle := provenanceTestBundle(t, "")
		first := bundle.InstallWithProvenance(provenanceTestSkill, "agents", environment, SkillProvenance{ProductID: "product-1", ReleaseID: "release-1"})
		if !first.AllSucceeded {
			t.Fatalf("initial install failed: %#v", first)
		}
		blocked := bundle.InstallWithProvenance(provenanceTestSkill, "agents", environment, SkillProvenance{ProductID: "product-2", ReleaseID: "release-9"})
		if blocked.AllSucceeded || !strings.Contains(blocked.Results[0].Error, "different Product") {
			t.Fatalf("foreign Product overwrite was not refused: %#v", blocked)
		}
	})

	t.Run("official managed directory", func(t *testing.T) {
		t.Parallel()
		environment := provenanceTestEnvironment(t)
		bundle := provenanceTestBundle(t, "")
		// The official bundle path installs without Product provenance.
		if report := bundle.Install(provenanceTestSkill, "agents", environment); !report.AllSucceeded {
			t.Fatalf("official install fixture failed: %#v", report)
		}
		blocked := bundle.InstallWithProvenance(provenanceTestSkill, "agents", environment, SkillProvenance{ProductID: "product-1", ReleaseID: "release-1"})
		if blocked.AllSucceeded || !strings.Contains(blocked.Results[0].Error, "official or legacy") {
			t.Fatalf("official Skill overwrite was not refused: %#v", blocked)
		}
	})

	t.Run("user-owned directory", func(t *testing.T) {
		t.Parallel()
		environment := provenanceTestEnvironment(t)
		bundle := provenanceTestBundle(t, "")
		userDir := filepath.Join(environment.Home, ".agents", "skills", provenanceTestSkill)
		if err := os.MkdirAll(userDir, 0o755); err != nil {
			t.Fatal(err)
		}
		userContent := []byte("user-owned Skill, no ViceMe manifest\n")
		if err := os.WriteFile(filepath.Join(userDir, "SKILL.md"), userContent, 0o644); err != nil {
			t.Fatal(err)
		}
		blocked := bundle.InstallWithProvenance(provenanceTestSkill, "agents", environment, SkillProvenance{ProductID: "product-1", ReleaseID: "release-1"})
		if blocked.AllSucceeded || !strings.Contains(blocked.Results[0].Error, "not managed by a ViceMe installation") {
			t.Fatalf("user-owned overwrite was not refused: %#v", blocked)
		}
		actual, err := os.ReadFile(filepath.Join(userDir, "SKILL.md"))
		if err != nil || string(actual) != string(userContent) {
			t.Fatalf("user-owned directory was changed: %v", err)
		}
	})

	t.Run("symlink destination", func(t *testing.T) {
		t.Parallel()
		environment := provenanceTestEnvironment(t)
		bundle := provenanceTestBundle(t, "")
		skillsDir := filepath.Join(environment.Home, ".agents", "skills")
		if err := os.MkdirAll(skillsDir, 0o755); err != nil {
			t.Fatal(err)
		}
		linkTarget := t.TempDir()
		if err := os.Symlink(linkTarget, filepath.Join(skillsDir, provenanceTestSkill)); err != nil {
			t.Fatal(err)
		}
		blocked := bundle.InstallWithProvenance(provenanceTestSkill, "agents", environment, SkillProvenance{ProductID: "product-1", ReleaseID: "release-1"})
		if blocked.AllSucceeded || !strings.Contains(blocked.Results[0].Error, "not a ViceMe-managed Skill directory") {
			t.Fatalf("symlink overwrite was not refused: %#v", blocked)
		}
	})
}

func TestMarketplaceInstallRequiresProvenance(t *testing.T) {
	t.Parallel()
	bundle := provenanceTestBundle(t, "")
	environment := provenanceTestEnvironment(t)
	report := bundle.InstallWithProvenance(provenanceTestSkill, "agents", environment, SkillProvenance{ProductID: "", ReleaseID: "release-1"})
	if report.AllSucceeded || !strings.Contains(report.Results[0].Error, "Product ID") {
		t.Fatalf("empty Product provenance was not refused: %#v", report)
	}
	if _, _, err := bundle.PrepareInstallSetWithProvenance([]string{provenanceTestSkill}, "agents", environment, SkillProvenance{ProductID: "product-1"}); err == nil {
		t.Fatal("empty Release provenance was not refused")
	}
}

func TestMarketplaceReinstallAfterRetirementCleanupIsFresh(t *testing.T) {
	t.Parallel()
	environment := provenanceTestEnvironment(t)
	bundle := provenanceTestBundle(t, "")
	provenance := SkillProvenance{ProductID: "product-1", ReleaseID: "release-1"}
	if report := bundle.InstallWithProvenance(provenanceTestSkill, "agents", environment, provenance); !report.AllSucceeded {
		t.Fatalf("initial install failed: %#v", report)
	}
	// A directory deleted outside the CLI (for example by retirement cleanup)
	// must be reinstallable: no stale provenance may linger.
	if err := os.RemoveAll(filepath.Join(environment.Home, ".agents", "skills", provenanceTestSkill)); err != nil {
		t.Fatal(err)
	}
	report := bundle.InstallWithProvenance(provenanceTestSkill, "agents", environment, provenance)
	if !report.AllSucceeded {
		t.Fatalf("fresh reinstall after removal failed: %#v", report)
	}
}

var _ = errors.Is // keep errors import if unused after edits
