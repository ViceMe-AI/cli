// dev-setup is shipped only in manual dev test packages, never in production installers.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/devsetup"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	"github.com/ViceMe-AI/cli/internal/update"
)

type build struct {
	SchemaVersion int               `json:"schemaVersion"`
	BuildID       string            `json:"buildId"`
	Commit        string            `json:"commit"`
	OS            string            `json:"os"`
	Arch          string            `json:"arch"`
	Files         map[string]string `json:"files"`
}

func main() {
	result, err := run(os.Args[1:])
	if err != nil {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"ok": false, "error": err.Error()})
		os.Exit(1)
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"ok": true, "data": result})
}
func run(args []string) (any, error) {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		return map[string]any{"commands": []string{"install --help", "uninstall --help"}}, nil
	}
	if len(args) == 0 {
		return nil, errors.New("use install or uninstall; --help lists options")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	destination := filepath.Join(home, ".local", "bin", "viceme")
	if runtime.GOOS == "windows" {
		destination = filepath.Join(os.Getenv("LOCALAPPDATA"), "ViceMe", "bin", "viceme.exe")
	}
	configDir := os.Getenv("VICEME_CLI_CONFIG_DIR")
	if configDir == "" {
		configDir = filepath.Join(home, ".viceme-cli")
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	target := flags.String("destination", destination, "explicit standalone CLI path")
	base := flags.String("config-dir", configDir, "existing CLI config directory")
	pkg := flags.String("package-dir", filepath.Dir(exe), "verified extracted platform package")
	region := flags.String("region", "cn", "cn or global")
	agent := flags.String("agent", "auto", "official Skill target")
	replace := flags.Bool("replace-standalone", false, "authorize retirement of an existing standalone CLI")
	expected := flags.String("sha256", "", "required installed dev binary digest for uninstall/resume")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return map[string]any{"help": true}, nil
		}
		return nil, err
	}
	if flags.NArg() != 0 {
		return nil, errors.New("unexpected positional arguments")
	}
	if os.Getenv("VICEME_INSTALL_METHOD") == "npm" {
		return nil, errors.New("npm launcher detected; remove with npm before installing the standalone dev package")
	}
	*target, err = filepath.Abs(*target)
	if err != nil {
		return nil, err
	}
	*base, err = filepath.Abs(*base)
	if err != nil {
		return nil, err
	}
	env := append(os.Environ(), "VICEME_CLI_CONFIG_DIR="+*base, "CI=1")
	if args[0] == "uninstall" {
		if *expected == "" {
			return nil, errors.New("--sha256 from the installed test package BUILD.json is required")
		}
		if _, err := os.Stat(*target); err == nil {
			version, err := inspectVersion(*target, env)
			if err != nil {
				return nil, err
			}
			if version["version"] != "dev" {
				return nil, errors.New("uninstall accepts a dev CLI only; production installation is preserved")
			}
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		backup, err := devsetup.Retire(*base, *target, *expected)
		return map[string]any{"retiredBinary": backup, "profilesAndSkillsPreserved": true}, err
	}
	if args[0] != "install" {
		return nil, errors.New("unknown operation")
	}
	if *region != "cn" && *region != "global" {
		return nil, errors.New("invalid region")
	}
	data, err := os.ReadFile(filepath.Join(*pkg, "BUILD.json"))
	if err != nil {
		return nil, err
	}
	var metadata build
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}
	if metadata.SchemaVersion != 1 || metadata.OS != runtime.GOOS || metadata.Arch != runtime.GOARCH || len(metadata.Commit) != 40 || !strings.HasPrefix(metadata.BuildID, "dev-") {
		return nil, errors.New("test package metadata does not match this platform")
	}
	binaryName := "viceme"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	source, err := filepath.Abs(filepath.Join(*pkg, binaryName))
	if err != nil {
		return nil, err
	}
	digest, err := devsetup.Digest(source)
	if err != nil {
		return nil, err
	}
	if metadata.Files[binaryName] != digest {
		return nil, errors.New("test binary SHA-256 mismatch")
	}
	if source == *target {
		return nil, errors.New("extract the test package outside the installation directory")
	}
	version, err := inspectVersion(source, env)
	if err != nil {
		return nil, err
	}
	if version["version"] != "dev" || version["commit"] != metadata.Commit {
		return nil, errors.New("test executable version/commit mismatch")
	}
	cfg, err := config.LoadOrDefault(*base)
	if err != nil {
		return nil, err
	}
	web := "https://dev.viceme.cn"
	if *region == "global" {
		web = "https://dev.viceme.ai"
	}
	profile, err := cfg.Resolve("dev")
	if err == nil {
		if profile.APIBaseURL != web+"/api" || profile.WebBaseURL != web || string(profile.MarketRegion) != *region {
			return nil, errors.New("existing dev profile points elsewhere; resolve it explicitly before installation")
		}
	} else {
		if _, err := cfg.AddProfile("dev", web+"/api", web, config.Region(*region)); err != nil {
			return nil, err
		}
	}
	environment := skillcontent.DefaultEnvironment()
	environment.ConfigDir = *base
	if err := os.MkdirAll(filepath.Dir(*target), 0700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(*base, 0700); err != nil {
		return nil, err
	}
	if err := update.ProbeRenameCapability(filepath.Dir(*target)); err != nil {
		return nil, err
	}
	if err := update.ProbeRenameCapability(*base); err != nil {
		return nil, err
	}
	if err := skillcontent.ProbeInstallPermissions(*agent, environment); err != nil {
		return nil, err
	}
	if err := devsetup.Resume(*base, *target); err != nil {
		return nil, err
	}
	if _, err := os.Lstat(*target); err == nil {
		if !*replace {
			return nil, errors.New("an existing CLI will be retired; pass --replace-standalone after verifying its path")
		}
		oldDigest, err := devsetup.Digest(*target)
		if err != nil {
			return nil, err
		}
		if _, err := inspectVersion(*target, env); err != nil {
			return nil, err
		}
		if _, err := devsetup.Retire(*base, *target, oldDigest); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	// Bootstrap is still the single owner of executable + official Skill activation.
	if _, err := invoke(source, env, "bootstrap", "activate", "--destination", *target, "--agent", *agent, "--region", *region); err != nil {
		return nil, err
	}
	// Use normal profile commands after activation; do not rewrite credentials/config.
	installed, err := config.LoadOrDefault(*base)
	if err != nil {
		return nil, err
	}
	if _, err := installed.Resolve("dev"); err != nil {
		if _, err := invoke(*target, env, "profile", "add", "--name", "dev", "--api-base-url", web+"/api", "--web-base-url", web, "--market-region", *region); err != nil {
			return nil, err
		}
	}
	if _, err := invoke(*target, env, "profile", "use", "dev"); err != nil {
		return nil, err
	}
	return map[string]any{"buildId": metadata.BuildID, "commit": metadata.Commit, "destination": *target, "profile": "dev", "webBaseUrl": web, "sha256": digest}, nil
}

// Inspect an incoming package without letting its startup recovery touch the
// installed generation. Installation itself still uses the real config.
func inspectVersion(path string, env []string) (map[string]any, error) {
	directory, err := os.MkdirTemp("", "viceme-dev-version-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(directory)
	return versionOf(path, append(env, "VICEME_CLI_CONFIG_DIR="+directory))
}

func versionOf(path string, env []string) (map[string]any, error) {
	return invoke(path, env, "--version")
}
func invoke(path string, env []string, args ...string) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = env
	out, err := cmd.Output()
	var response struct {
		OK    bool           `json:"ok"`
		Data  map[string]any `json:"data"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if decode := json.Unmarshal(out, &response); decode != nil {
		return nil, errors.New("CLI returned an invalid result; preserve installation and inspect locally")
	}
	if err != nil || !response.OK {
		return nil, fmt.Errorf("CLI operation failed (%s); preserve recovery state", response.Error.Code)
	}
	return response.Data, nil
}
