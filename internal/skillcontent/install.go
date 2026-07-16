package skillcontent

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gofrs/flock"
)

type Environment struct {
	Home            string
	CodexHome       string
	ClaudeConfigDir string
}

func DefaultEnvironment() Environment {
	home, _ := os.UserHomeDir()
	return Environment{
		Home:            home,
		CodexHome:       os.Getenv("CODEX_HOME"),
		ClaudeConfigDir: os.Getenv("CLAUDE_CONFIG_DIR"),
	}
}

type InstallResult struct {
	Target string `json:"target"`
	Path   string `json:"path"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type InstallReport struct {
	AllSucceeded bool            `json:"all_succeeded"`
	Results      []InstallResult `json:"results"`
}

type DoctorResult struct {
	Target         string `json:"target"`
	Path           string `json:"path"`
	Installed      bool   `json:"installed"`
	Healthy        bool   `json:"healthy"`
	ExpectedDigest string `json:"expected_digest"`
	ActualDigest   string `json:"actual_digest,omitempty"`
	Problem        string `json:"problem,omitempty"`
}

type DoctorReport struct {
	Healthy bool           `json:"healthy"`
	Results []DoctorResult `json:"results"`
}

func (b *Bundle) Install(name, target string, environment Environment) InstallReport {
	paths, err := resolveTargets(target, environment, true)
	if err != nil {
		return InstallReport{AllSucceeded: false, Results: []InstallResult{{Target: target, Status: "failed", Error: err.Error()}}}
	}
	report := InstallReport{AllSucceeded: true}
	for _, resolved := range paths {
		result := InstallResult{Target: resolved.name, Path: resolved.path, Status: "updated"}
		beforeDigest, beforeErr := digestInstalled(resolved.path)
		expected, _ := b.Digests(name)
		if beforeErr == nil && beforeDigest == expected.Full {
			result.Status = "unchanged"
			report.Results = append(report.Results, result)
			continue
		}
		if err := b.installOne(name, resolved.path); err != nil {
			result.Status = "failed"
			result.Error = err.Error()
			report.AllSucceeded = false
		}
		report.Results = append(report.Results, result)
	}
	return report
}

func (b *Bundle) Doctor(name, target string, environment Environment) DoctorReport {
	paths, err := resolveTargets(target, environment, false)
	if err != nil {
		return DoctorReport{Healthy: false, Results: []DoctorResult{{Target: target, Problem: err.Error()}}}
	}
	expected, err := b.Digests(name)
	if err != nil {
		return DoctorReport{Healthy: false, Results: []DoctorResult{{Target: target, Problem: err.Error()}}}
	}
	report := DoctorReport{Healthy: true}
	for _, resolved := range paths {
		result := DoctorResult{Target: resolved.name, Path: resolved.path, ExpectedDigest: expected.Full}
		actual, err := digestInstalled(resolved.path)
		if errors.Is(err, fs.ErrNotExist) {
			result.Problem = "not installed"
			report.Healthy = false
		} else if err != nil {
			result.Problem = err.Error()
			report.Healthy = false
		} else {
			result.Installed = true
			result.ActualDigest = actual
			result.Healthy = actual == expected.Full
			if !result.Healthy {
				result.Problem = "installed files differ from this CLI release"
				report.Healthy = false
			}
		}
		report.Results = append(report.Results, result)
	}
	return report
}

type targetPath struct {
	name string
	path string
}

func resolveTargets(target string, environment Environment, forInstall bool) ([]targetPath, error) {
	if target == "" {
		target = "auto"
	}
	codexHome := environment.CodexHome
	if codexHome == "" {
		codexHome = filepath.Join(environment.Home, ".codex")
	}
	claudeHome := environment.ClaudeConfigDir
	if claudeHome == "" {
		claudeHome = filepath.Join(environment.Home, ".claude")
	}
	known := map[string]targetPath{
		"codex":  {name: "codex", path: filepath.Join(codexHome, "skills", "viceme")},
		"claude": {name: "claude", path: filepath.Join(claudeHome, "skills", "viceme")},
	}
	if target != "auto" {
		resolved, ok := known[target]
		if !ok {
			return nil, fmt.Errorf("unsupported Skill target %q; use auto, codex, or claude", target)
		}
		return []targetPath{resolved}, nil
	}
	var result []targetPath
	for _, name := range []string{"codex", "claude"} {
		resolved := known[name]
		base := filepath.Dir(filepath.Dir(resolved.path))
		if _, err := os.Stat(base); err == nil {
			result = append(result, resolved)
		}
	}
	if len(result) == 0 {
		if forInstall {
			result = append(result, known["codex"])
		} else {
			result = append(result, known["codex"], known["claude"])
		}
	}
	return result, nil
}

func (b *Bundle) installOne(name, destination string) error {
	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create Skill parent: %w", err)
	}
	lockPath := destination + ".viceme-install-lock"
	installLock := flock.New(lockPath)
	locked, err := installLock.TryLock()
	if err != nil {
		return fmt.Errorf("acquire Skill install lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("another Skill install is already updating %s", destination)
	}
	defer installLock.Unlock()
	expected, err := b.Digests(name)
	if err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".viceme-stage-")
	if err != nil {
		return fmt.Errorf("create Skill staging directory: %w", err)
	}
	defer os.RemoveAll(stage)
	stagedSkill := filepath.Join(stage, name)
	if err := os.MkdirAll(stagedSkill, 0o755); err != nil {
		return fmt.Errorf("create staged Skill directory: %w", err)
	}
	if err := copyTree(b.FS, name, stagedSkill); err != nil {
		return err
	}
	stagedBundle := New(os.DirFS(stage))
	if err := stagedBundle.Validate(name); err != nil {
		return fmt.Errorf("validate staged Skill: %w", err)
	}
	backup := destination + ".viceme-backup"
	_ = os.RemoveAll(backup)
	hadExisting := false
	if _, err := os.Lstat(destination); err == nil {
		hadExisting = true
		if err := os.Rename(destination, backup); err != nil {
			return fmt.Errorf("stage existing Skill: %w", err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("inspect existing Skill: %w", err)
	}
	if err := os.Rename(stagedSkill, destination); err != nil {
		if hadExisting {
			_ = os.Rename(backup, destination)
		}
		return fmt.Errorf("activate staged Skill: %w", err)
	}
	actualDigest, err := digestInstalled(destination)
	if err != nil || actualDigest != expected.Full {
		_ = os.RemoveAll(destination)
		if hadExisting {
			_ = os.Rename(backup, destination)
		}
		if err != nil {
			return fmt.Errorf("verify installed Skill: %w", err)
		}
		return fmt.Errorf("verify installed Skill: digest mismatch")
	}
	if hadExisting {
		_ = os.RemoveAll(backup)
	}
	return nil
}

func copyTree(source fs.FS, root, destination string) error {
	return fs.WalkDir(source, root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(name, root), "/")
		if rel == "" {
			return nil
		}
		outPath := filepath.Join(destination, filepath.FromSlash(rel))
		if entry.IsDir() {
			return os.MkdirAll(outPath, 0o755)
		}
		data, err := fs.ReadFile(source, name)
		if err != nil {
			return err
		}
		return os.WriteFile(outPath, data, 0o644)
	})
}

func digestInstalled(directory string) (string, error) {
	if _, err := os.Stat(directory); err != nil {
		return "", err
	}
	return digestFS(os.DirFS(directory), ".", func(string) bool { return true })
}

func InstalledFiles(directory string) ([]string, error) {
	var result []string
	err := filepath.WalkDir(directory, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(directory, name)
		if err != nil {
			return err
		}
		result = append(result, path.Clean(filepath.ToSlash(rel)))
		return nil
	})
	sort.Strings(result)
	return result, err
}
