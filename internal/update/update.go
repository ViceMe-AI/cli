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
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/semver"
)

const (
	PackageName             = "@viceme-ai/cli"
	RegistryURL             = "https://registry.npmjs.org"
	RegistryPackageURL      = RegistryURL + "/@viceme-ai%2fcli/latest"
	ScopeRegistryArg        = "--@viceme-ai:registry=" + RegistryURL
	updateStateFilename     = "update-state.json"
	npmCacheDirectory       = "npm-cache"
	updateCacheTTL          = 24 * time.Hour
	maximumRegistryResponse = 256 << 10
)

var ErrNPMInstallRequired = errors.New("viceme update requires the npm-installed launcher")

type ErrorKind string

const (
	ErrorRegistryNetwork  ErrorKind = "registry_network"
	ErrorRegistryResponse ErrorKind = "registry_response"
	ErrorNPMMissing       ErrorKind = "npm_missing"
	ErrorNPMPermission    ErrorKind = "npm_permission"
	ErrorNPMCommand       ErrorKind = "npm_command"
)

type OperationError struct {
	Kind  ErrorKind
	Cause error
}

func (err *OperationError) Error() string {
	if err.Cause == nil {
		return string(err.Kind)
	}
	return err.Cause.Error()
}

func (err *OperationError) Unwrap() error { return err.Cause }

func ErrorKindOf(err error) ErrorKind {
	var operationError *OperationError
	if errors.As(err, &operationError) {
		return operationError.Kind
	}
	return ""
}

type CheckResult struct {
	CurrentVersion   string `json:"current_version"`
	AvailableVersion string `json:"available_version,omitempty"`
	UpdateAvailable  bool   `json:"update_available"`
	Method           string `json:"method"`
	Package          string `json:"package"`
	Source           string `json:"source"`
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
	ConfigDir         string
	RegistryEndpoint  string
	HTTPClient        *http.Client
	Now               func() time.Time
	Runner            Runner
}

func NewNPMService(currentVersion, comparableVersion, installMethod string) *NPMService {
	return &NPMService{
		CurrentVersion:    currentVersion,
		ComparableVersion: comparableVersion,
		InstallMethod:     installMethod,
		RegistryEndpoint:  RegistryPackageURL,
		HTTPClient:        &http.Client{Timeout: 15 * time.Second},
		Now:               time.Now,
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
	available, source, err := service.latestVersion(ctx)
	if err != nil {
		return result, err
	}
	comparison, err := semver.Compare(available, service.ComparableVersion)
	if err != nil {
		return result, fmt.Errorf("compare npm release with current CLI: %w", err)
	}
	result.AvailableVersion = available
	result.UpdateAvailable = comparison > 0
	result.Source = source
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
	output, err := service.runNPM(
		ctx,
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
	return service.runNPM(ctx, "install", "--registry="+RegistryURL, ScopeRegistryArg, "--global", "--ignore-scripts", "--no-audit", "--no-fund", PackageName+"@"+version)
}

func (service *NPMService) runner() Runner {
	if service.Runner == nil {
		return ExecRunner{}
	}
	return service.Runner
}

func (service *NPMService) latestVersion(ctx context.Context) (string, string, error) {
	version, err := service.fetchLatestVersion(ctx)
	if err == nil {
		service.saveUpdateState(version)
		return version, "registry", nil
	}
	if ErrorKindOf(err) == ErrorRegistryNetwork {
		if cached, ok := service.loadFreshUpdateState(); ok {
			return cached, "cache", nil
		}
	}
	return "", "", err
}

func (service *NPMService) fetchLatestVersion(ctx context.Context) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, service.registryEndpoint(), nil)
	if err != nil {
		return "", &OperationError{Kind: ErrorRegistryResponse, Cause: fmt.Errorf("build npm registry request: %w", err)}
	}
	request.Header.Set("Accept", "application/json")
	response, err := service.httpClient().Do(request)
	if err != nil {
		return "", &OperationError{Kind: ErrorRegistryNetwork, Cause: fmt.Errorf("query npm registry: %w", err)}
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		kind := ErrorRegistryResponse
		if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= http.StatusInternalServerError {
			kind = ErrorRegistryNetwork
		}
		return "", &OperationError{Kind: kind, Cause: fmt.Errorf("npm registry returned HTTP %d", response.StatusCode)}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumRegistryResponse+1))
	if err != nil {
		return "", &OperationError{Kind: ErrorRegistryNetwork, Cause: fmt.Errorf("read npm registry response: %w", err)}
	}
	if len(body) > maximumRegistryResponse {
		return "", &OperationError{Kind: ErrorRegistryResponse, Cause: errors.New("npm registry response exceeded the size limit")}
	}
	var document struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &document); err != nil {
		return "", &OperationError{Kind: ErrorRegistryResponse, Cause: errors.New("npm registry returned invalid JSON")}
	}
	version := strings.TrimSpace(document.Version)
	if _, err := semver.Parse(version); err != nil {
		return "", &OperationError{Kind: ErrorRegistryResponse, Cause: fmt.Errorf("npm registry returned an invalid package version: %w", err)}
	}
	return version, nil
}

type updateState struct {
	LatestVersion string `json:"latest_version"`
	CheckedAt     int64  `json:"checked_at"`
}

func (service *NPMService) loadFreshUpdateState() (string, bool) {
	filename := service.updateStatePath()
	if filename == "" {
		return "", false
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", false
	}
	var state updateState
	if json.Unmarshal(data, &state) != nil || state.CheckedAt <= 0 {
		return "", false
	}
	age := service.now().Sub(time.Unix(state.CheckedAt, 0))
	if age < 0 || age > updateCacheTTL {
		return "", false
	}
	if _, err := semver.Parse(state.LatestVersion); err != nil {
		return "", false
	}
	return state.LatestVersion, true
}

func (service *NPMService) saveUpdateState(version string) {
	filename := service.updateStatePath()
	if filename == "" {
		return
	}
	if err := os.MkdirAll(service.ConfigDir, 0o700); err != nil {
		return
	}
	data, err := json.Marshal(updateState{LatestVersion: version, CheckedAt: service.now().Unix()})
	if err != nil {
		return
	}
	temporary, err := os.CreateTemp(service.ConfigDir, ".update-state-*")
	if err != nil {
		return
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if temporary.Chmod(0o600) != nil {
		temporary.Close()
		return
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return
	}
	if temporary.Close() != nil {
		return
	}
	_ = os.Rename(temporaryName, filename)
}

func (service *NPMService) runNPM(ctx context.Context, args ...string) ([]byte, error) {
	if cacheArg := service.npmCacheArg(); cacheArg != "" {
		if err := os.MkdirAll(filepath.Join(service.ConfigDir, npmCacheDirectory), 0o700); err != nil {
			return nil, &OperationError{Kind: ErrorNPMPermission, Cause: errors.New("could not create the ViceMe npm cache directory")}
		}
		args = append([]string{cacheArg}, args...)
	}
	output, err := service.runner().Run(ctx, "npm", args...)
	if err == nil {
		return output, nil
	}
	return output, classifyNPMError(err, output)
}

func classifyNPMError(err error, output []byte) error {
	var executableError *exec.Error
	if errors.As(err, &executableError) || errors.Is(err, exec.ErrNotFound) {
		return &OperationError{Kind: ErrorNPMMissing, Cause: errors.New("npm is not available in PATH")}
	}
	normalized := strings.ToUpper(string(output))
	if errors.Is(err, os.ErrPermission) || strings.Contains(normalized, "EPERM") || strings.Contains(normalized, "EACCES") {
		return &OperationError{Kind: ErrorNPMPermission, Cause: errors.New("npm could not write its cache or global installation directory")}
	}
	return &OperationError{Kind: ErrorNPMCommand, Cause: errors.New("npm command failed")}
}

func (service *NPMService) npmCacheArg() string {
	if service.ConfigDir == "" {
		return ""
	}
	return "--cache=" + filepath.Join(service.ConfigDir, npmCacheDirectory)
}

func (service *NPMService) updateStatePath() string {
	if service.ConfigDir == "" {
		return ""
	}
	return filepath.Join(service.ConfigDir, updateStateFilename)
}

func (service *NPMService) registryEndpoint() string {
	if service.RegistryEndpoint == "" {
		return RegistryPackageURL
	}
	return service.RegistryEndpoint
}

func (service *NPMService) httpClient() *http.Client {
	if service.HTTPClient == nil {
		return &http.Client{Timeout: 15 * time.Second}
	}
	return service.HTTPClient
}

func (service *NPMService) now() time.Time {
	if service.Now == nil {
		return time.Now()
	}
	return service.Now()
}

func commandError(err error, output []byte) string {
	// npm output can reflect registry configuration. Keep it out of stable JSON
	// rather than risking credential-bearing diagnostic text.
	_ = output
	return err.Error()
}
