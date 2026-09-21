package skillcontent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ViceMe-AI/cli/internal/config"
)

// RuntimeManifest describes installed files, not an entitlement or trial grant.
// It shares its on-disk shape with the standalone Python installer.
type RuntimeManifest struct {
	SchemaVersion int               `json:"schemaVersion"`
	ProductID     string            `json:"productId"`
	ReleaseID     string            `json:"releaseId"`
	APIBaseURL    string            `json:"apiBaseUrl"`
	Market        string            `json:"market"`
	Runner        string            `json:"runner"`
	Kind          string            `json:"kind"`
	Files         map[string]string `json:"files"`
}

var RuntimeResourcePaths = []string{
	"scripts/trial.py", "scripts/qrcodegen.py", "widgets/onboarding.html",
	"guides/widgets.md", "guides/trial-usage.md",
	"guides/purchase.md", "guides/host-presentation.md",
}

// FindRuntimeInstall checks only the selected host and its normal shared root.
// No CLI credentials, server calls, writes, or install/activation decisions occur.
func FindRuntimeInstall(environment Environment, target, productID, apiBaseURL string, directories ...string) (string, RuntimeManifest, bool, error) {
	if len(directories) > 0 && directories[0] != "" {
		directory, err := filepath.Abs(directories[0])
		if err != nil {
			return "", RuntimeManifest{}, true, err
		}
		manifest, valid := readRuntimeInstall(directory, productID, apiBaseURL)
		if valid {
			return directory, manifest, true, nil
		}
		return "", RuntimeManifest{}, true, nil
	}
	targets, err := resolveTargets("runtime-lookup", target, environment)
	if err != nil {
		return "", RuntimeManifest{}, false, err
	}
	found := false
	for _, target := range targets {
		root := filepath.Dir(target.path)
		entries, err := os.ReadDir(root)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", RuntimeManifest{}, found, err
		}
		for _, entry := range entries {
			if !entry.IsDir() || strings.Contains(entry.Name(), ".") {
				continue
			}
			directory := filepath.Join(root, entry.Name())
			// Both installers write these provenance fields. Decoding only the
			// shared fields also accepts Python's smaller install manifest.
			var owner struct {
				ProductID string `json:"product_id"`
				ReleaseID string `json:"release_id"`
			}
			raw, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(installManifestPath)))
			if err != nil || json.Unmarshal(raw, &owner) != nil || owner.ProductID != productID {
				continue
			}
			found = true
			manifest, valid := readRuntimeInstall(directory, productID, apiBaseURL)
			if valid {
				return directory, manifest, true, nil
			}
			return "", RuntimeManifest{}, true, nil
		}
	}
	return "", RuntimeManifest{}, found, nil
}

// ReadRuntimeIdentity verifies destination ownership without requiring usable runtime files.
// Receipt restoration may repair incomplete or older installations.
func ReadRuntimeIdentity(directory, productID, apiBaseURL string) (RuntimeManifest, bool) {
	var manifest RuntimeManifest
	for _, relative := range []string{"", ".viceme", installManifestPath, ".viceme/runtime.json", "SKILL.md"} {
		info, err := os.Lstat(filepath.Join(directory, relative))
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return manifest, false
		}
	}
	raw, err := os.ReadFile(filepath.Join(directory, installManifestPath))
	var owner struct {
		ProductID string `json:"product_id"`
		ReleaseID string `json:"release_id"`
	}
	if err != nil || json.Unmarshal(raw, &owner) != nil || owner.ProductID != productID || owner.ReleaseID == "" {
		return manifest, false
	}
	raw, err = os.ReadFile(filepath.Join(directory, ".viceme/runtime.json"))
	if err != nil || json.Unmarshal(raw, &manifest) != nil || manifest.SchemaVersion != 1 || manifest.ProductID != productID || !config.EquivalentAPIBaseURLs(manifest.APIBaseURL, apiBaseURL) || (manifest.Runner != "cli" && manifest.Runner != "python") {
		return manifest, false
	}
	// A cross-client restore can stop between writing its two identity files.
	// Only the recorded old/new release pair permits another repair to resume;
	// runtime readiness still rejects an unfinished package transition below.
	if manifest.ReleaseID != owner.ReleaseID && !PackageIdentityTransitionMatches(directory, productID, owner.ReleaseID, manifest.ReleaseID) {
		return manifest, false
	}
	skill, err := os.Stat(filepath.Join(directory, "SKILL.md"))
	if err != nil || !skill.Mode().IsRegular() {
		return manifest, false
	}
	return manifest, manifest.Kind == "trial" || manifest.Kind == "owned" || manifest.Kind == "free" || manifest.Kind == "purchase"
}

func readRuntimeInstall(directory, productID, apiBaseURL string) (RuntimeManifest, bool) {
	manifest, valid := ReadRuntimeIdentity(directory, productID, apiBaseURL)
	if !valid {
		return manifest, false
	}
	// An older embedded Python script still calls the removed public API host.
	// Ownership remains valid for explicit repair, but do not report it ready.
	normalized, err := config.NormalizeAPIBaseURL(manifest.APIBaseURL)
	if err != nil || normalized != manifest.APIBaseURL {
		return manifest, false
	}
	required := append([]string{".viceme/environment.json"}, runtimeFiles()...)
	if manifest.Kind == "trial" {
		required = append(required, "references/viceme-runtime.md", TrialBodyPath)
	}
	if manifest.Kind == "purchase" {
		required = append(required, "SKILL.md", "references/purchase.md")
	}
	if manifest.Kind != "trial" && manifest.Kind != "free" && manifest.Kind != "owned" && manifest.Kind != "purchase" {
		return manifest, false
	}
	for _, relative := range required {
		if manifest.Files[relative] == "" {
			return manifest, false
		}
	}
	for relative, digest := range manifest.Files {
		if !fs.ValidPath(relative) || strings.Contains(relative, "\\") {
			return manifest, false
		}
		filename := filepath.Join(directory, filepath.FromSlash(relative))
		resolved, err := filepath.EvalSymlinks(filename)
		base, baseErr := filepath.EvalSymlinks(directory)
		if err != nil || baseErr != nil || !strings.HasPrefix(resolved, base+string(filepath.Separator)) {
			return manifest, false
		}
		data, err := os.ReadFile(filename)
		if err != nil || fmt.Sprintf("%x", sha256.Sum256(data)) != digest {
			return manifest, false
		}
	}
	if !PackageInstallationComplete(directory, productID) {
		return manifest, false
	}
	return manifest, true
}

func runtimeFiles() []string {
	paths := make([]string, 0, len(RuntimeResourcePaths))
	for _, relative := range RuntimeResourcePaths {
		paths = append(paths, ".viceme/"+relative)
	}
	return paths
}
