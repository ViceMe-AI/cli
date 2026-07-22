package skillcontent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/semver"
	"github.com/gofrs/flock"
)

const installManifestPath = ".viceme/install-manifest.json"

type Environment struct {
	Home            string
	CodexHome       string
	ClaudeConfigDir string
	ConfigDir       string
}

func DefaultEnvironment() Environment {
	home, _ := os.UserHomeDir()
	return Environment{
		Home:            home,
		CodexHome:       os.Getenv("CODEX_HOME"),
		ClaudeConfigDir: os.Getenv("CLAUDE_CONFIG_DIR"),
		ConfigDir:       defaultConfigDir(home),
	}
}

func defaultConfigDir(home string) string {
	if directory := os.Getenv("VICEME_CLI_CONFIG_DIR"); directory != "" {
		return directory
	}
	return filepath.Join(home, ".viceme-cli")
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
	Target                 string       `json:"target"`
	Path                   string       `json:"path"`
	Installed              bool         `json:"installed"`
	Healthy                bool         `json:"healthy"`
	ExpectedDigest         string       `json:"expected_digest"`
	ActualDigest           string       `json:"actual_digest,omitempty"`
	ExpectedEmbeddedDigest string       `json:"expected_embedded_digest"`
	ActualEmbeddedDigest   string       `json:"actual_embedded_digest,omitempty"`
	ManifestPath           string       `json:"manifest_path"`
	Checks                 DoctorChecks `json:"checks"`
	Problem                string       `json:"problem,omitempty"`
}

type DoctorReport struct {
	Healthy bool           `json:"healthy"`
	Results []DoctorResult `json:"results"`
}

type DoctorCheck struct {
	Healthy  bool   `json:"healthy"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
	Recorded string `json:"recorded,omitempty"`
	Problem  string `json:"problem,omitempty"`
}

type DoctorChecks struct {
	CLIVersion            DoctorCheck `json:"cli_version"`
	SkillVersion          DoctorCheck `json:"skill_version"`
	FullBundleDigest      DoctorCheck `json:"full_bundle_digest"`
	EmbeddedContentDigest DoctorCheck `json:"embedded_content_digest"`
	Compatibility         DoctorCheck `json:"compatibility"`
}

type installManifest struct {
	SchemaVersion         int    `json:"schema_version"`
	CLIVersion            string `json:"cli_version"`
	SkillVersion          string `json:"skill_version"`
	MinimumCLIVersion     string `json:"minimum_cli_version"`
	CLICompatibility      string `json:"cli_compatibility"`
	FullBundleDigest      string `json:"full_skill_bundle_digest"`
	EmbeddedContentDigest string `json:"embedded_content_digest"`
}

func (b *Bundle) Install(name, target string, environment Environment) InstallReport {
	paths, err := resolveTargets(target, environment, true)
	if err != nil {
		return InstallReport{AllSucceeded: false, Results: []InstallResult{{Target: target, Status: "failed", Error: err.Error()}}}
	}
	report := InstallReport{AllSucceeded: true}
	for _, resolved := range paths {
		result := InstallResult{Target: resolved.name, Path: resolved.path, Status: "updated"}
		if b.installationCurrent(name, resolved.path) {
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
	packageMetadata, err := b.Package(name)
	if err != nil {
		return DoctorReport{Healthy: false, Results: []DoctorResult{{Target: target, Problem: err.Error()}}}
	}
	report := DoctorReport{Healthy: true}
	for _, resolved := range paths {
		result := DoctorResult{
			Target:                 resolved.name,
			Path:                   resolved.path,
			ExpectedDigest:         expected.Full,
			ExpectedEmbeddedDigest: expected.Embedded,
			ManifestPath:           filepath.Join(resolved.path, filepath.FromSlash(installManifestPath)),
		}
		actual, err := digestsInstalled(resolved.path)
		if errors.Is(err, fs.ErrNotExist) {
			result.Problem = "not installed"
			report.Healthy = false
		} else if err != nil {
			result.Problem = err.Error()
			report.Healthy = false
		} else {
			result.Installed = true
			result.ActualDigest = actual.Full
			result.ActualEmbeddedDigest = actual.Embedded
			manifest, manifestErr := readInstallManifest(resolved.path)
			result.Checks = doctorChecks(packageMetadata, expected, actual, manifest, manifestErr)
			result.Healthy, result.Problem = summarizeChecks(result.Checks)
			if !result.Healthy {
				report.Healthy = false
			}
		}
		report.Results = append(report.Results, result)
	}
	return report
}

func doctorChecks(packageMetadata PackageMetadata, expected, actual Digests, manifest installManifest, manifestErr error) DoctorChecks {
	checks := DoctorChecks{
		CLIVersion:            DoctorCheck{Expected: buildinfo.Version},
		SkillVersion:          DoctorCheck{Expected: packageMetadata.SkillVersion},
		FullBundleDigest:      DoctorCheck{Expected: expected.Full, Actual: actual.Full},
		EmbeddedContentDigest: DoctorCheck{Expected: expected.Embedded, Actual: actual.Embedded},
		Compatibility: DoctorCheck{
			Expected: fmt.Sprintf("minimum %s; %s", packageMetadata.MinimumCLIVersion, packageMetadata.CLICompatibility),
			Actual:   buildinfo.CompatibilityVersion(),
		},
	}
	if manifestErr != nil {
		problem := "install manifest is missing or invalid: " + manifestErr.Error()
		checks.CLIVersion.Problem = problem
		checks.SkillVersion.Problem = problem
		checks.FullBundleDigest.Problem = problem
		checks.EmbeddedContentDigest.Problem = problem
		checks.Compatibility.Problem = problem
		return checks
	}
	checks.CLIVersion.Actual = manifest.CLIVersion
	checks.CLIVersion.Healthy = manifest.CLIVersion == buildinfo.Version
	if !checks.CLIVersion.Healthy {
		checks.CLIVersion.Problem = "installed Skill was written by a different CLI version"
	}
	checks.SkillVersion.Actual = manifest.SkillVersion
	checks.SkillVersion.Healthy = manifest.SkillVersion == packageMetadata.SkillVersion
	if !checks.SkillVersion.Healthy {
		checks.SkillVersion.Problem = "installed Skill version differs from the bundled Skill"
	}
	checks.FullBundleDigest.Recorded = manifest.FullBundleDigest
	checks.FullBundleDigest.Healthy = actual.Full == expected.Full && manifest.FullBundleDigest == expected.Full
	if !checks.FullBundleDigest.Healthy {
		checks.FullBundleDigest.Problem = "installed files or recorded full bundle digest differ from this CLI release"
	}
	checks.EmbeddedContentDigest.Recorded = manifest.EmbeddedContentDigest
	checks.EmbeddedContentDigest.Healthy = actual.Embedded == expected.Embedded && manifest.EmbeddedContentDigest == expected.Embedded
	if !checks.EmbeddedContentDigest.Healthy {
		checks.EmbeddedContentDigest.Problem = "agent-readable files or recorded embedded digest differ from this CLI release"
	}
	checks.Compatibility.Recorded = fmt.Sprintf("minimum %s; %s", manifest.MinimumCLIVersion, manifest.CLICompatibility)
	compatible, compatibilityErr := semver.Satisfies(buildinfo.CompatibilityVersion(), manifest.CLICompatibility)
	minimumComparison, minimumErr := semver.Compare(buildinfo.CompatibilityVersion(), manifest.MinimumCLIVersion)
	checks.Compatibility.Healthy = manifest.SchemaVersion == 1 &&
		manifest.MinimumCLIVersion == packageMetadata.MinimumCLIVersion &&
		manifest.CLICompatibility == packageMetadata.CLICompatibility &&
		compatibilityErr == nil && minimumErr == nil && compatible && minimumComparison >= 0
	if !checks.Compatibility.Healthy {
		checks.Compatibility.Problem = "current CLI does not satisfy the installed Skill compatibility contract"
	}
	return checks
}

func summarizeChecks(checks DoctorChecks) (bool, string) {
	var problems []string
	for name, check := range map[string]DoctorCheck{
		"cli_version":             checks.CLIVersion,
		"skill_version":           checks.SkillVersion,
		"full_bundle_digest":      checks.FullBundleDigest,
		"embedded_content_digest": checks.EmbeddedContentDigest,
		"compatibility":           checks.Compatibility,
	} {
		if !check.Healthy {
			problems = append(problems, name+": "+check.Problem)
		}
	}
	sort.Strings(problems)
	return len(problems) == 0, strings.Join(problems, "; ")
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
		"agents": {name: "agents", path: filepath.Join(environment.Home, ".agents", "skills", "viceme")},
	}
	if target != "auto" {
		resolved, ok := known[target]
		if !ok {
			return nil, fmt.Errorf("unsupported Skill target %q; use auto, codex, claude, or agents", target)
		}
		return []targetPath{resolved}, nil
	}
	var result []targetPath
	for _, name := range []string{"codex", "claude", "agents"} {
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
			result = append(result, known["codex"], known["claude"], known["agents"])
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
	manifest, err := b.installManifest(name)
	if err != nil {
		return err
	}
	if err := writeInstallManifest(stagedSkill, manifest); err != nil {
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
	actualDigests, err := digestsInstalled(destination)
	if err != nil || actualDigests != expected {
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

func (b *Bundle) installManifest(name string) (installManifest, error) {
	metadata, err := b.Package(name)
	if err != nil {
		return installManifest{}, err
	}
	digests, err := b.Digests(name)
	if err != nil {
		return installManifest{}, err
	}
	return installManifest{
		SchemaVersion:         1,
		CLIVersion:            buildinfo.Version,
		SkillVersion:          metadata.SkillVersion,
		MinimumCLIVersion:     metadata.MinimumCLIVersion,
		CLICompatibility:      metadata.CLICompatibility,
		FullBundleDigest:      digests.Full,
		EmbeddedContentDigest: digests.Embedded,
	}, nil
}

func writeInstallManifest(directory string, manifest installManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Skill install manifest: %w", err)
	}
	manifestFile := filepath.Join(directory, filepath.FromSlash(installManifestPath))
	if err := os.MkdirAll(filepath.Dir(manifestFile), 0o755); err != nil {
		return fmt.Errorf("create Skill manifest directory: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(manifestFile, data, 0o644); err != nil {
		return fmt.Errorf("write Skill install manifest: %w", err)
	}
	return nil
}

func readInstallManifest(directory string) (installManifest, error) {
	data, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(installManifestPath)))
	if err != nil {
		return installManifest{}, err
	}
	var manifest installManifest
	if err := decodeStrictJSON(data, &manifest); err != nil {
		return installManifest{}, err
	}
	return manifest, nil
}

func (b *Bundle) installationCurrent(name, directory string) bool {
	expected, err := b.Digests(name)
	if err != nil {
		return false
	}
	actual, err := digestsInstalled(directory)
	if err != nil || actual != expected {
		return false
	}
	want, err := b.installManifest(name)
	if err != nil {
		return false
	}
	installed, err := readInstallManifest(directory)
	return err == nil && installed == want
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

func digestsInstalled(directory string) (Digests, error) {
	if _, err := os.Stat(directory); err != nil {
		return Digests{}, err
	}
	fsys := os.DirFS(directory)
	full, err := digestFS(fsys, ".", func(relative string) bool { return relative != installManifestPath })
	if err != nil {
		return Digests{}, err
	}
	embedded, err := digestFS(fsys, ".", func(relative string) bool {
		return relative == "SKILL.md" || strings.HasPrefix(relative, "references/")
	})
	if err != nil {
		return Digests{}, err
	}
	return Digests{Full: full, Embedded: embedded}, nil
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
