package skillcontent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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
	"widgets/payment.html", "guides/widgets.md", "guides/trial-usage.md",
}

// FindRuntimeInstall checks only the selected host and its normal shared root.
// No CLI credentials, server calls, writes, or install/activation decisions occur.
func FindRuntimeInstall(environment Environment, target, productID, apiBaseURL string) (string, RuntimeManifest, bool, error) {
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
			raw, err = os.ReadFile(filepath.Join(directory, ".viceme", "runtime.json"))
			var manifest RuntimeManifest
			if err != nil || json.Unmarshal(raw, &manifest) != nil || manifest.SchemaVersion != 1 || manifest.ProductID != productID || manifest.ReleaseID != owner.ReleaseID || manifest.APIBaseURL != apiBaseURL || (manifest.Runner != "cli" && manifest.Runner != "python") {
				return "", RuntimeManifest{}, true, nil
			}
			// Readiness verifies only platform resources, not every authored asset.
			// The trial suspension intentionally replaces SKILL.md after exhaustion.
			skill, skillErr := os.Stat(filepath.Join(directory, "SKILL.md"))
			if skillErr != nil || !skill.Mode().IsRegular() {
				return "", RuntimeManifest{}, true, nil
			}
			required := append([]string{".viceme/environment.json"}, runtimeFiles()...)
			if manifest.Kind == "trial" {
				required = append(required, "references/viceme-runtime.md")
			}
			valid := manifest.Kind == "trial" || manifest.Kind == "free" || manifest.Kind == "owned"
			for _, relative := range required {
				valid = valid && manifest.Files[relative] != ""
			}
			for relative, digest := range manifest.Files {
				if !fs.ValidPath(relative) || strings.Contains(relative, "\\") {
					valid = false
					break
				}
				filename := filepath.Join(directory, filepath.FromSlash(relative))
				resolved, err := filepath.EvalSymlinks(filename)
				base, baseErr := filepath.EvalSymlinks(directory)
				if err != nil || baseErr != nil || !strings.HasPrefix(resolved, base+string(filepath.Separator)) {
					valid = false
					break
				}
				data, err := os.ReadFile(filename)
				if err != nil || fmt.Sprintf("%x", sha256.Sum256(data)) != digest {
					valid = false
					break
				}
			}
			if valid {
				return directory, manifest, true, nil
			}
			return "", RuntimeManifest{}, true, nil
		}
	}
	return "", RuntimeManifest{}, found, nil
}

func runtimeFiles() []string {
	paths := make([]string, 0, len(RuntimeResourcePaths))
	for _, relative := range RuntimeResourcePaths {
		paths = append(paths, ".viceme/"+relative)
	}
	return paths
}
