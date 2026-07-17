// Package update implements the npm-backed Viceme CLI update path. The npm
// launcher owns binary acquisition and checksum verification; this package
// updates that launcher at an exact version and then refreshes the bundled
// Agent Skill using the same exact package version.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/ViceMe-AI/cli/internal/semver"
)

const (
	PackageName      = "@viceme-ai/cli"
	RegistryURL      = "https://registry.npmjs.org"
	ScopeRegistryArg = "--@viceme-ai:registry=" + RegistryURL
)

var ErrNPMInstallRequired = errors.New("viceme update requires the npm-installed launcher")

type CheckResult struct {
	CurrentVersion   string `json:"current_version"`
	AvailableVersion string `json:"available_version,omitempty"`
	UpdateAvailable  bool   `json:"update_available"`
	Method           string `json:"method"`
	Package          string `json:"package"`
}

type TargetResult struct {
	Target string `json:"target"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type ApplyOptions struct {
	RefreshSkills bool
	SkillTarget   string
}

type ApplyResult struct {
	PreviousCLIVersion string         `json:"previous_cli_version"`
	CLIVersion         string         `json:"cli_version"`
	Targets            []TargetResult `json:"targets"`
}

type Service interface {
	EnsureLauncher(context.Context) (TargetResult, error)
	Check(context.Context) (CheckResult, error)
	Apply(context.Context, CheckResult, ApplyOptions) (ApplyResult, error)
}

type Runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

type NPMService struct {
	CurrentVersion    string
	ComparableVersion string
	InstallMethod     string
	Runner            Runner
}

func NewNPMService(currentVersion, comparableVersion, installMethod string) *NPMService {
	return &NPMService{
		CurrentVersion:    currentVersion,
		ComparableVersion: comparableVersion,
		InstallMethod:     installMethod,
		Runner:            ExecRunner{},
	}
}

func (service *NPMService) EnsureLauncher(ctx context.Context) (TargetResult, error) {
	result := TargetResult{Target: "npm_global", Status: "skipped"}
	if service.InstallMethod != "npm" {
		return result, nil
	}
	if _, err := semver.Parse(service.ComparableVersion); err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		return result, fmt.Errorf("refuse launcher install without an exact semantic version: %w", err)
	}
	result.Status = "updated"
	output, err := service.installExactPackage(ctx, service.ComparableVersion)
	if err != nil {
		result.Status = "failed"
		result.Error = commandError(err, output)
		return result, fmt.Errorf("install persistent npm launcher: %w", err)
	}
	return result, nil
}

func (service *NPMService) Check(ctx context.Context) (CheckResult, error) {
	result := CheckResult{
		CurrentVersion: service.CurrentVersion,
		Method:         "npm",
		Package:        PackageName,
	}
	output, err := service.runner().Run(ctx, "npm", "view", PackageName, "version", "--json", "--registry="+RegistryURL, ScopeRegistryArg)
	if err != nil {
		return result, fmt.Errorf("query npm release: %w", err)
	}
	available, err := parseNPMVersion(output)
	if err != nil {
		return result, err
	}
	comparison, err := semver.Compare(available, service.ComparableVersion)
	if err != nil {
		return result, fmt.Errorf("compare npm release with current CLI: %w", err)
	}
	result.AvailableVersion = available
	result.UpdateAvailable = comparison > 0
	return result, nil
}

func (service *NPMService) Apply(ctx context.Context, check CheckResult, options ApplyOptions) (ApplyResult, error) {
	result := ApplyResult{PreviousCLIVersion: service.CurrentVersion, CLIVersion: service.CurrentVersion}
	if service.InstallMethod != "npm" {
		return result, ErrNPMInstallRequired
	}
	if _, err := semver.Parse(check.AvailableVersion); err != nil {
		return result, fmt.Errorf("refuse update without an exact semantic version: %w", err)
	}
	targetVersion := service.ComparableVersion
	if check.UpdateAvailable {
		targetVersion = check.AvailableVersion
	}
	if _, err := semver.Parse(targetVersion); err != nil {
		return result, fmt.Errorf("refuse update without an exact semantic version: %w", err)
	}
	exactPackage := PackageName + "@" + targetVersion
	cliTarget := TargetResult{Target: "npm_global", Status: "unchanged"}
	if check.UpdateAvailable {
		output, err := service.installExactPackage(ctx, targetVersion)
		if err != nil {
			cliTarget.Status = "failed"
			cliTarget.Error = commandError(err, output)
			result.Targets = append(result.Targets, cliTarget)
			return result, fmt.Errorf("update npm launcher: %w", err)
		}
		cliTarget.Status = "updated"
		result.CLIVersion = check.AvailableVersion
	}
	result.Targets = append(result.Targets, cliTarget)
	if !options.RefreshSkills {
		return result, nil
	}
	target := options.SkillTarget
	if target == "" {
		target = "auto"
	}
	skillTarget := TargetResult{Target: "agent_skill:" + target, Status: "updated"}
	output, err := service.runner().Run(
		ctx,
		"npm",
		"exec",
		"--registry="+RegistryURL,
		ScopeRegistryArg,
		"--yes",
		"--package="+exactPackage,
		"--",
		"viceme",
		"skills",
		"install",
		"--target",
		target,
		"--json",
	)
	if err != nil {
		skillTarget.Status = "failed"
		skillTarget.Error = commandError(err, output)
		result.Targets = append(result.Targets, skillTarget)
		return result, fmt.Errorf("refresh Agent Skill with updated CLI: %w", err)
	}
	result.Targets = append(result.Targets, skillTarget)
	return result, nil
}

func (service *NPMService) installExactPackage(ctx context.Context, version string) ([]byte, error) {
	return service.runner().Run(ctx, "npm", "install", "--registry="+RegistryURL, ScopeRegistryArg, "--global", "--ignore-scripts", "--no-audit", "--no-fund", PackageName+"@"+version)
}

func (service *NPMService) runner() Runner {
	if service.Runner == nil {
		return ExecRunner{}
	}
	return service.Runner
}

func parseNPMVersion(output []byte) (string, error) {
	var version string
	if err := json.Unmarshal(output, &version); err != nil {
		version = strings.TrimSpace(string(output))
	}
	version = strings.Trim(version, `"`)
	if _, err := semver.Parse(version); err != nil {
		return "", fmt.Errorf("npm returned an invalid package version: %w", err)
	}
	return version, nil
}

func commandError(err error, output []byte) string {
	// npm output can reflect registry configuration. Keep it out of stable JSON
	// rather than risking credential-bearing diagnostic text.
	_ = output
	return err.Error()
}
