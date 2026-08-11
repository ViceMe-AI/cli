package update

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/semver"
)

const maximumReleaseAssetBytes = 100 << 20

// RegionAware binds release discovery to the selected profile.
type RegionAware interface {
	SetRegion(string)
}

// ReleaseService updates binaries installed by the official S3 bootstrap. npm
// launches continue to use NPMService because the npm launcher owns its binary.
type ReleaseService struct {
	CurrentVersion    string
	ComparableVersion string
	Region            string
	HTTPClient        *http.Client
	Runner            Runner
	ExecutablePath    string
	ReleaseBaseURL    string
	GOOS              string
	GOARCH            string
	ScheduleWindows   func(staged, destination, target, region string, refreshSkills bool) error
}

func NewReleaseService(currentVersion, comparableVersion string) *ReleaseService {
	return &ReleaseService{
		CurrentVersion:    currentVersion,
		ComparableVersion: comparableVersion,
		Region:            "cn",
		HTTPClient:        &http.Client{Timeout: 5 * time.Minute},
		Runner:            ExecRunner{},
		GOOS:              runtime.GOOS,
		GOARCH:            runtime.GOARCH,
	}
}

func (service *ReleaseService) SetRegion(region string) {
	if region == "cn" || region == "global" {
		service.Region = region
	}
}

func (service *ReleaseService) EnsureLauncher(context.Context) (TargetResult, error) {
	return TargetResult{Target: "standalone_binary", Status: "unchanged"}, nil
}

func (service *ReleaseService) Check(ctx context.Context) (CheckResult, error) {
	result := CheckResult{
		CurrentVersion: service.CurrentVersion,
		Method:         "release_bundle",
		Package:        "viceme-cli",
		Source:         "official_s3",
	}
	body, err := service.get(ctx, service.baseURL()+"/latest", 64)
	if err != nil {
		return result, err
	}
	latest := strings.TrimSpace(string(body))
	if _, err := semver.Parse(latest); err != nil {
		return result, &OperationError{Kind: ErrorReleaseResponse, Cause: errors.New("release index returned an invalid semantic version")}
	}
	comparison, err := semver.Compare(latest, service.ComparableVersion)
	if err != nil {
		return result, &OperationError{Kind: ErrorReleaseResponse, Cause: errors.New("current CLI version is not comparable with the release index")}
	}
	result.AvailableVersion = latest
	result.UpdateAvailable = comparison > 0
	return result, nil
}

func (service *ReleaseService) Apply(ctx context.Context, check CheckResult, options ApplyOptions) (ApplyResult, error) {
	result := ApplyResult{PreviousCLIVersion: service.CurrentVersion, CLIVersion: service.CurrentVersion}
	targetVersion := service.ComparableVersion
	if check.UpdateAvailable {
		targetVersion = check.AvailableVersion
	}
	if _, err := semver.Parse(targetVersion); err != nil {
		return result, &OperationError{Kind: ErrorReleaseResponse, Cause: errors.New("refuse standalone update without an exact semantic version")}
	}

	executable, err := service.executable()
	if err != nil {
		return result, &OperationError{Kind: ErrorReleaseReplace, Cause: errors.New("could not resolve the current ViceMe executable")}
	}
	if check.UpdateAvailable {
		asset := fmt.Sprintf("viceme_%s_%s_%s", targetVersion, service.GOOS, service.GOARCH)
		if service.GOOS == "windows" {
			asset += ".exe"
		}
		releaseURL := fmt.Sprintf("%s/v%s", service.baseURL(), targetVersion)
		binary, err := service.get(ctx, releaseURL+"/"+asset, maximumReleaseAssetBytes)
		if err != nil {
			return result, err
		}
		checksum, err := service.get(ctx, releaseURL+"/"+asset+".sha256", 1024)
		if err != nil {
			return result, err
		}
		if err := verifyReleaseChecksum(binary, checksum); err != nil {
			return result, &OperationError{Kind: ErrorReleaseIntegrity, Cause: err}
		}
		staged, err := stageExecutable(executable, binary)
		if err != nil {
			return result, &OperationError{Kind: ErrorReleaseReplace, Cause: errors.New("could not stage the ViceMe executable")}
		}
		if service.GOOS == "windows" {
			target := options.SkillTarget
			if target == "" {
				target = "auto"
			}
			if err := service.scheduleWindows()(staged, executable, target, service.Region, options.RefreshSkills); err != nil {
				os.Remove(staged)
				return result, &OperationError{Kind: ErrorReleaseReplace, Cause: errors.New("could not schedule the Windows release activation")}
			}
			result.CLIVersion = targetVersion
			result.Targets = append(result.Targets, TargetResult{Target: "standalone_binary", Status: "scheduled"})
			if options.RefreshSkills {
				result.Targets = append(result.Targets, TargetResult{Target: "agent_skill:" + target, Status: "scheduled"})
			}
			return result, nil
		}
		defer os.Remove(staged)
		backup := executable + ".viceme-update-backup"
		_ = os.Remove(backup)
		if err := copyExecutable(executable, backup); err != nil {
			return result, &OperationError{Kind: ErrorReleaseReplace, Cause: errors.New("could not preserve the previous ViceMe executable")}
		}
		restorePrevious := func() {
			_ = os.Rename(backup, executable)
		}
		if err := os.Rename(staged, executable); err != nil {
			restorePrevious()
			return result, &OperationError{Kind: ErrorReleaseReplace, Cause: errors.New("could not atomically replace the ViceMe executable")}
		}
		if options.RefreshSkills {
			target := options.SkillTarget
			if target == "" {
				target = "auto"
			}
			output, err := service.runner().Run(ctx, executable, "install", "--agent", target, "--region", service.Region)
			if err != nil {
				restorePrevious()
				result.Targets = append(result.Targets, TargetResult{Target: "agent_skill:" + target, Status: "failed", Error: commandError(err, output)})
				return result, &OperationError{Kind: ErrorReleaseSkillRefresh, Cause: errors.New("new CLI could not install and verify its matching official Skills; the previous CLI was restored")}
			}
			result.Targets = append(result.Targets, TargetResult{Target: "agent_skill:" + target, Status: "updated"})
		}
		_ = os.Remove(backup)
		result.CLIVersion = targetVersion
		result.Targets = append(result.Targets, TargetResult{Target: "standalone_binary", Status: "updated"})
		return result, nil
	} else {
		result.Targets = append(result.Targets, TargetResult{Target: "standalone_binary", Status: "unchanged"})
	}

	if !options.RefreshSkills {
		return result, nil
	}
	target := options.SkillTarget
	if target == "" {
		target = "auto"
	}
	output, err := service.runner().Run(ctx, executable, "install", "--agent", target, "--region", service.Region)
	if err != nil {
		result.Targets = append(result.Targets, TargetResult{Target: "agent_skill:" + target, Status: "failed", Error: commandError(err, output)})
		return result, &OperationError{Kind: ErrorReleaseSkillRefresh, Cause: errors.New("updated CLI could not refresh the official Skills")}
	}
	result.Targets = append(result.Targets, TargetResult{Target: "agent_skill:" + target, Status: "updated"})
	return result, nil
}

func (service *ReleaseService) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, &OperationError{Kind: ErrorReleaseResponse, Cause: errors.New("could not build the release request")}
	}
	response, err := service.client().Do(request)
	if err != nil {
		return nil, &OperationError{Kind: ErrorReleaseNetwork, Cause: errors.New("could not reach the official release store")}
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		kind := ErrorReleaseResponse
		if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= http.StatusInternalServerError {
			kind = ErrorReleaseNetwork
		}
		return nil, &OperationError{Kind: kind, Cause: fmt.Errorf("official release store returned HTTP %d", response.StatusCode)}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, &OperationError{Kind: ErrorReleaseNetwork, Cause: errors.New("could not read the official release response")}
	}
	if int64(len(body)) > limit {
		return nil, &OperationError{Kind: ErrorReleaseResponse, Cause: errors.New("official release response exceeded the size limit")}
	}
	return body, nil
}

func (service *ReleaseService) baseURL() string {
	if service.ReleaseBaseURL != "" {
		return strings.TrimRight(service.ReleaseBaseURL, "/")
	}
	if service.Region == "global" {
		return "https://s3.viceme.ai/cli/releases"
	}
	return "https://s3.viceme.cn/cli/releases"
}

func (service *ReleaseService) executable() (string, error) {
	if service.ExecutablePath != "" {
		return filepath.Abs(service.ExecutablePath)
	}
	return os.Executable()
}

func (service *ReleaseService) runner() Runner {
	if service.Runner == nil {
		return ExecRunner{}
	}
	return service.Runner
}

func (service *ReleaseService) client() *http.Client {
	if service.HTTPClient == nil {
		return &http.Client{Timeout: 5 * time.Minute}
	}
	return service.HTTPClient
}

func (service *ReleaseService) scheduleWindows() func(string, string, string, string, bool) error {
	if service.ScheduleWindows != nil {
		return service.ScheduleWindows
	}
	return scheduleWindowsReplacement
}

func verifyReleaseChecksum(binary, checksumFile []byte) error {
	fields := strings.Fields(string(checksumFile))
	if len(fields) == 0 || len(fields[0]) != sha256.Size*2 {
		return errors.New("release checksum file is invalid")
	}
	expected, err := hex.DecodeString(fields[0])
	if err != nil {
		return errors.New("release checksum file is invalid")
	}
	actual := sha256.Sum256(binary)
	if subtle.ConstantTimeCompare(actual[:], expected) != 1 {
		return errors.New("release checksum verification failed")
	}
	return nil
}

func stageExecutable(destination string, contents []byte) (string, error) {
	directory := filepath.Dir(destination)
	pattern := ".viceme-update-*"
	if runtime.GOOS == "windows" {
		pattern += ".exe"
	}
	staged, err := os.CreateTemp(directory, pattern)
	if err != nil {
		return "", err
	}
	stagedName := staged.Name()
	if err := staged.Chmod(0o755); err != nil {
		staged.Close()
		os.Remove(stagedName)
		return "", err
	}
	if _, err := staged.Write(contents); err != nil {
		staged.Close()
		os.Remove(stagedName)
		return "", err
	}
	if err := staged.Sync(); err != nil {
		staged.Close()
		os.Remove(stagedName)
		return "", err
	}
	if err := staged.Close(); err != nil {
		os.Remove(stagedName)
		return "", err
	}
	return stagedName, nil
}

func copyExecutable(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	name := output.Name()
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		_ = os.Remove(name)
		return err
	}
	if err := output.Sync(); err != nil {
		_ = output.Close()
		_ = os.Remove(name)
		return err
	}
	if err := output.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}

func scheduleWindowsReplacement(staged, destination, target, region string, refreshSkills bool) error {
	script, err := os.CreateTemp(filepath.Dir(destination), ".viceme-activate-*.ps1")
	if err != nil {
		return err
	}
	scriptName := script.Name()
	contents := `param(
  [int]$ParentPid,
  [string]$Staged,
  [string]$Destination,
  [string]$Target,
  [string]$Region,
  [string]$RefreshSkills
)
$ErrorActionPreference = "Stop"
$Result = "$Destination.viceme-update-result.json"
$Backup = "$Destination.viceme-update-backup"
try {
  Wait-Process -Id $ParentPid -ErrorAction SilentlyContinue
  Remove-Item -Force -ErrorAction SilentlyContinue $Backup
  if (Test-Path $Destination) { Copy-Item -Force -Path $Destination -Destination $Backup }
  Move-Item -Force -Path $Staged -Destination $Destination
  if ($RefreshSkills -eq "true") {
    & $Destination install --agent $Target --region $Region
    if ($LASTEXITCODE -ne 0) { throw "official Skill refresh failed" }
  }
  @{ status = "succeeded"; updatedAt = [DateTime]::UtcNow.ToString("o") } | ConvertTo-Json -Compress | Set-Content -Encoding UTF8 $Result
  Remove-Item -Force -ErrorAction SilentlyContinue $Backup
} catch {
  Remove-Item -Force -ErrorAction SilentlyContinue $Destination
  if (Test-Path $Backup) { Move-Item -Force -Path $Backup -Destination $Destination }
  @{ status = "failed"; updatedAt = [DateTime]::UtcNow.ToString("o") } | ConvertTo-Json -Compress | Set-Content -Encoding UTF8 $Result
  throw
} finally {
  Remove-Item -Force -ErrorAction SilentlyContinue $MyInvocation.MyCommand.Path
}
`
	if _, err := script.WriteString(contents); err != nil {
		script.Close()
		os.Remove(scriptName)
		return err
	}
	if err := script.Close(); err != nil {
		os.Remove(scriptName)
		return err
	}
	command := exec.Command(
		"powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy",
		"Bypass",
		"-File",
		scriptName,
		"-ParentPid",
		strconv.Itoa(os.Getpid()),
		"-Staged",
		staged,
		"-Destination",
		destination,
		"-Target",
		target,
		"-Region",
		region,
		"-RefreshSkills",
		strconv.FormatBool(refreshSkills),
	)
	command.Env = withoutEnvironmentVariable(os.Environ(), "VICEME_ACCESS_TOKEN")
	if err := command.Start(); err != nil {
		os.Remove(scriptName)
		return err
	}
	return command.Process.Release()
}
