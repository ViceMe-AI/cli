// Package update implements the npm-backed ViceMe CLI update path. The npm
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
	NoUpdateNotifierEnv     = "VICEME_NO_UPDATE_NOTIFIER"
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

// Notice is the stable machine-readable update hint injected into CLI output.
// It intentionally contains no registry response or local filesystem details.
type Notice struct {
	Current string `json:"current"`
	Latest  string `json:"latest"`
}

func (notice Notice) Message() string {
	return fmt.Sprintf("ViceMe CLI %s is available; current %s; run: viceme update", notice.Latest, notice.Current)
}

type Service interface {
	EnsureLauncher(context.Context) (TargetResult, error)
	RollbackLauncher(context.Context) (TargetResult, error)
	Check(context.Context) (CheckResult, error)
	Apply(context.Context, CheckResult, ApplyOptions) (ApplyResult, error)
}

// Notifier is an optional read-only extension implemented by npm-backed
// services. Commands never fail when the registry or notification cache is
// unavailable.
type Notifier interface {
	CachedNotice() *Notice
	RefreshNotice(context.Context)
}

type Runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Env = withoutEnvironmentVariable(os.Environ(), "VICEME_ACCESS_TOKEN")
	return command.CombinedOutput()
}

func withoutEnvironmentVariable(environment []string, name string) []string {
	prefix := name + "="
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		if !strings.HasPrefix(entry, prefix) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

type NPMService struct {
	CurrentVersion             string
	ComparableVersion          string
	InstallMethod              string
	ConfigDir                  string
	RegistryEndpoint           string
	HTTPClient                 *http.Client
	Now                        func() time.Time
	Runner                     Runner
	bootstrapPreviousVersion   string
	bootstrapPreviousInstalled bool
	bootstrapChanged           bool
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
	previousVersion, previousInstalled, err := service.installedGlobalVersion(ctx)
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		return result, err
	}
	service.bootstrapPreviousVersion = previousVersion
	service.bootstrapPreviousInstalled = previousInstalled
	service.bootstrapChanged = !previousInstalled || previousVersion != service.ComparableVersion
	if !service.bootstrapChanged {
		result.Status = "unchanged"
		return result, nil
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

func (service *NPMService) RollbackLauncher(ctx context.Context) (TargetResult, error) {
	result := TargetResult{Target: "npm_global", Status: "unchanged"}
	if !service.bootstrapChanged {
		return result, nil
	}
	var output []byte
	var err error
	if service.bootstrapPreviousInstalled {
		output, err = service.installExactPackage(ctx, service.bootstrapPreviousVersion)
	} else {
		output, err = service.runNPM(
			ctx,
			"uninstall",
			"--registry="+RegistryURL,
			ScopeRegistryArg,
			"--global",
			"--ignore-scripts",
			"--no-audit",
			"--no-fund",
			PackageName,
		)
	}
	if err != nil {
		result.Status = "rollback_failed"
		result.Error = commandError(err, output)
		return result, fmt.Errorf("roll back persistent npm launcher: %w", err)
	}
	result.Status = "rolled_back"
	service.bootstrapChanged = false
	return result, nil
}

func (service *NPMService) installedGlobalVersion(ctx context.Context) (string, bool, error) {
	output, commandErr := service.runNPM(
		ctx,
		"list",
		"--loglevel=silent",
		"--global",
		"--depth=0",
		"--json",
		PackageName,
	)
	if len(output) > maximumRegistryResponse {
		return "", false, &OperationError{Kind: ErrorNPMCommand, Cause: errors.New("npm global package query exceeded the output limit")}
	}
	var document struct {
		Dependencies map[string]struct {
			Version string `json:"version"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal(output, &document); err != nil {
		if commandErr != nil {
			return "", false, commandErr
		}
		return "", false, &OperationError{Kind: ErrorNPMCommand, Cause: errors.New("npm returned invalid global package metadata")}
	}
	installed, exists := document.Dependencies[PackageName]
	if !exists || installed.Version == "" {
		return "", false, nil
	}
	if _, err := semver.Parse(installed.Version); err != nil {
		return "", false, &OperationError{Kind: ErrorNPMCommand, Cause: errors.New("installed global ViceMe launcher has an invalid version")}
	}
	return installed.Version, true, nil
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
		refreshErr := fmt.Errorf("refresh Agent Skill with updated CLI: %w", err)
		if !check.UpdateAvailable {
			return result, refreshErr
		}
		rollbackOutput, rollbackErr := service.installExactPackage(ctx, service.ComparableVersion)
		if rollbackErr != nil {
			result.Targets[0].Status = "rollback_failed"
			result.Targets[0].Error = commandError(rollbackErr, rollbackOutput)
			return result, errors.Join(refreshErr, fmt.Errorf("roll back npm launcher: %w", rollbackErr))
		}
		result.Targets[0].Status = "rolled_back"
		result.CLIVersion = service.CurrentVersion
		return result, refreshErr
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

// CachedNotice performs local I/O only. Like lark-cli, it may use the last
// validated cached version while a stale cache is refreshed in the background.
func (service *NPMService) CachedNotice() *Notice {
	if service.shouldSkipNotifier() {
		return nil
	}
	state, ok := service.loadUpdateState()
	if !ok {
		return nil
	}
	comparison, err := semver.Compare(state.LatestVersion, service.ComparableVersion)
	if err != nil || comparison <= 0 {
		return nil
	}
	return &Notice{Current: service.CurrentVersion, Latest: state.LatestVersion}
}

// RefreshNotice refreshes the version cache at most once per 24 hours.
// All failures are intentionally ignored by callers: update discovery must
// never change command output, latency, or exit status.
func (service *NPMService) RefreshNotice(ctx context.Context) {
	if service.shouldSkipNotifier() {
		return
	}
	if state, ok := service.loadUpdateState(); ok && service.updateStateIsFresh(state) {
		return
	}
	version, err := service.fetchLatestVersion(ctx)
	if err == nil {
		service.saveUpdateState(version)
	}
}

func (service *NPMService) shouldSkipNotifier() bool {
	if service.InstallMethod != "npm" || os.Getenv(NoUpdateNotifierEnv) != "" {
		return true
	}
	for _, key := range []string{"CI", "BUILD_NUMBER", "RUN_ID"} {
		if os.Getenv(key) != "" {
			return true
		}
	}
	_, err := semver.Parse(service.ComparableVersion)
	return err != nil
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

func (service *NPMService) loadUpdateState() (updateState, bool) {
	filename := service.updateStatePath()
	if filename == "" {
		return updateState{}, false
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return updateState{}, false
	}
	var state updateState
	if json.Unmarshal(data, &state) != nil || state.CheckedAt <= 0 {
		return updateState{}, false
	}
	if _, err := semver.Parse(state.LatestVersion); err != nil {
		return updateState{}, false
	}
	return state, true
}

func (service *NPMService) updateStateIsFresh(state updateState) bool {
	age := service.now().Sub(time.Unix(state.CheckedAt, 0))
	return age >= 0 && age <= updateCacheTTL
}

func (service *NPMService) loadFreshUpdateState() (string, bool) {
	state, ok := service.loadUpdateState()
	if !ok || !service.updateStateIsFresh(state) {
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
