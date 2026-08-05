package skillcontent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
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
	if runtime.GOOS == "windows" {
		if directory := os.Getenv("LOCALAPPDATA"); directory != "" {
			preferred := filepath.Join(directory, "ViceMe", "Config")
			legacy := filepath.Join(home, ".viceme-cli")
			if _, err := os.Stat(filepath.Join(preferred, "config.json")); err == nil || !errors.Is(err, fs.ErrNotExist) {
				return preferred
			}
			if _, err := os.Stat(filepath.Join(legacy, "config.json")); err == nil || !errors.Is(err, fs.ErrNotExist) {
				return legacy
			}
			return preferred
		}
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
	statuses, installErr := b.installAtomically(name, paths)
	report := InstallReport{AllSucceeded: installErr == nil}
	for _, resolved := range paths {
		status := statuses[resolved.path]
		if installErr != nil && status != "unchanged" {
			status = "failed"
		} else if status == "" {
			status = "failed"
		}
		result := InstallResult{Target: resolved.name, Path: resolved.path, Status: status}
		if status == "failed" && installErr != nil {
			result.Error = installErr.Error()
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

type stagedInstall struct {
	target      targetPath
	stageRoot   string
	stagedSkill string
	backup      string
	hadExisting bool
	backedUp    bool
	activated   bool
}

func (b *Bundle) installAtomically(name string, targets []targetPath) (map[string]string, error) {
	statuses := make(map[string]string, len(targets))
	ordered := append([]targetPath(nil), targets...)
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].path < ordered[right].path })
	locks := make([]*flock.Flock, 0, len(ordered))
	defer func() {
		for index := len(locks) - 1; index >= 0; index-- {
			_ = locks[index].Unlock()
		}
	}()
	for _, target := range ordered {
		parent := filepath.Dir(target.path)
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return statuses, fmt.Errorf("create %s Skill parent: %w", target.name, err)
		}
		installLock := flock.New(target.path + ".viceme-install-lock")
		locked, err := installLock.TryLock()
		if err != nil {
			return statuses, fmt.Errorf("acquire %s Skill install lock: %w", target.name, err)
		}
		if !locked {
			return statuses, fmt.Errorf("another Skill install is already updating %s", target.path)
		}
		locks = append(locks, installLock)
	}

	expected, err := b.Digests(name)
	if err != nil {
		return statuses, err
	}
	manifest, err := b.installManifest(name)
	if err != nil {
		return statuses, err
	}
	var staged []*stagedInstall
	committed := false
	defer func() {
		cleanupSkillInstallStaging(staged, committed)
	}()
	for _, target := range ordered {
		if b.installationCurrent(name, target.path) {
			statuses[target.path] = "unchanged"
			continue
		}
		item := &stagedInstall{target: target}
		item.stageRoot, err = os.MkdirTemp(filepath.Dir(target.path), ".viceme-stage-")
		if err != nil {
			return statuses, fmt.Errorf("create %s Skill staging directory: %w", target.name, err)
		}
		staged = append(staged, item)
		item.stagedSkill = filepath.Join(item.stageRoot, name)
		item.backup = filepath.Join(item.stageRoot, "previous")
		if err := os.MkdirAll(item.stagedSkill, 0o755); err != nil {
			return statuses, fmt.Errorf("create staged %s Skill directory: %w", target.name, err)
		}
		if err := copyTree(b.FS, name, item.stagedSkill); err != nil {
			return statuses, fmt.Errorf("stage %s Skill: %w", target.name, err)
		}
		if err := writeInstallManifest(item.stagedSkill, manifest); err != nil {
			return statuses, fmt.Errorf("stage %s Skill manifest: %w", target.name, err)
		}
		stagedBundle := New(os.DirFS(item.stageRoot))
		if err := stagedBundle.Validate(name); err != nil {
			return statuses, fmt.Errorf("validate staged %s Skill: %w", target.name, err)
		}
		actual, err := digestsInstalled(item.stagedSkill)
		if err != nil || actual != expected {
			if err != nil {
				return statuses, fmt.Errorf("verify staged %s Skill: %w", target.name, err)
			}
			return statuses, fmt.Errorf("verify staged %s Skill: digest mismatch", target.name)
		}
		statuses[target.path] = "pending"
	}

	for _, item := range staged {
		if _, err := os.Lstat(item.target.path); err == nil {
			item.hadExisting = true
			if err := os.Rename(item.target.path, item.backup); err != nil {
				rollbackErr := rollbackSkillInstalls(staged)
				return failedInstallStatuses(statuses), errors.Join(fmt.Errorf("back up %s Skill: %w", item.target.name, err), rollbackErr)
			}
			item.backedUp = true
		} else if !errors.Is(err, fs.ErrNotExist) {
			rollbackErr := rollbackSkillInstalls(staged)
			return failedInstallStatuses(statuses), errors.Join(fmt.Errorf("inspect existing %s Skill: %w", item.target.name, err), rollbackErr)
		}
		if err := os.Rename(item.stagedSkill, item.target.path); err != nil {
			rollbackErr := rollbackSkillInstalls(staged)
			return failedInstallStatuses(statuses), errors.Join(fmt.Errorf("activate staged %s Skill: %w", item.target.name, err), rollbackErr)
		}
		item.activated = true
		actual, err := digestsInstalled(item.target.path)
		if err != nil || actual != expected {
			rollbackErr := rollbackSkillInstalls(staged)
			if err != nil {
				return failedInstallStatuses(statuses), errors.Join(fmt.Errorf("verify installed %s Skill: %w", item.target.name, err), rollbackErr)
			}
			return failedInstallStatuses(statuses), errors.Join(fmt.Errorf("verify installed %s Skill: digest mismatch", item.target.name), rollbackErr)
		}
		statuses[item.target.path] = "updated"
	}
	committed = true
	return statuses, nil
}

func cleanupSkillInstallStaging(staged []*stagedInstall, committed bool) {
	for _, item := range staged {
		// A failed rollback leaves the only copy of the user's previous Skill in
		// this staging root. Preserve it for manual recovery and include its path
		// in the rollback error instead of deleting it during deferred cleanup.
		if !committed && item.backedUp {
			continue
		}
		_ = os.RemoveAll(item.stageRoot)
	}
}

func failedInstallStatuses(statuses map[string]string) map[string]string {
	for target, status := range statuses {
		if status != "unchanged" {
			statuses[target] = "failed"
		}
	}
	return statuses
}

func rollbackSkillInstalls(staged []*stagedInstall) error {
	var rollbackErrors []error
	for index := len(staged) - 1; index >= 0; index-- {
		item := staged[index]
		if item.activated {
			if err := os.RemoveAll(item.target.path); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("remove new %s Skill: %w", item.target.name, err))
				continue
			}
			item.activated = false
		}
		if item.backedUp {
			if err := os.Rename(item.backup, item.target.path); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("restore previous %s Skill from %s: %w", item.target.name, item.backup, err))
				continue
			}
			item.backedUp = false
		}
	}
	for _, item := range staged {
		if item.backedUp {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("previous %s Skill is preserved at %s for manual recovery", item.target.name, item.backup))
		}
	}
	return errors.Join(rollbackErrors...)
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
