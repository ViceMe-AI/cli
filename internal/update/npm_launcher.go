package update

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/ViceMe-AI/cli/internal/semver"
)

// ReexecutionLauncher resolves the global package installed by activation.
// The launching package may live in an immutable npx cache, so its original
// launcher path is not authority for the newly committed generation.
func (service *NPMService) ReexecutionLauncher(ctx context.Context, version string) (string, error) {
	if _, err := semver.Parse(version); err != nil {
		return "", errors.New("npm re-execution requires an exact target version")
	}
	// Runner captures both streams; suppress npm notices so they cannot be
	// mistaken for part of the package directory. Exit failures remain errors.
	rootOutput, err := service.runNPM(ctx, "root", "--global", "--loglevel=silent", "--no-update-notifier")
	if err != nil {
		return "", err
	}
	root := strings.TrimSpace(string(rootOutput))
	if !filepath.IsAbs(root) || strings.ContainsAny(root, "\r\n\x00") {
		return "", errors.New("npm returned an invalid global package directory")
	}
	packageDir := filepath.Join(root, "@viceme-ai", "cli")
	data, err := os.ReadFile(filepath.Join(packageDir, "package.json"))
	if err != nil {
		return "", errors.New("could not read the activated npm package")
	}
	var manifest struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if json.Unmarshal(data, &manifest) != nil || manifest.Name != PackageName || manifest.Version != version {
		return "", errors.New("global npm launcher does not match the activated generation")
	}
	launcher := filepath.Join(packageDir, "npm", "bin", "viceme.mjs")
	info, err := os.Stat(launcher)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("activated npm launcher is missing or invalid")
	}
	return launcher, nil
}
