package command

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/privatefile"
	"github.com/ViceMe-AI/cli/internal/semver"
	updatepkg "github.com/ViceMe-AI/cli/internal/update"
	"github.com/gofrs/flock"
)

const (
	automaticUpdateLockFilename  = "automatic-update.lock"
	automaticUpdateStateFilename = "automatic-update.json"
	automaticUpdateCheckInterval = 24 * time.Hour
	automaticUpdateRetryInterval = time.Hour
)

type automaticUpdateState struct {
	SchemaVersion    int    `json:"schemaVersion"`
	CurrentVersion   string `json:"currentVersion"`
	AvailableVersion string `json:"availableVersion,omitempty"`
	Status           string `json:"status"`
	CheckedAt        int64  `json:"checkedAt"`
	ErrorKind        string `json:"errorKind,omitempty"`
}

// runAutomaticUpdateWorker is the detached, silent half of automatic updates.
// Every failure is deliberately contained here: the foreground command has
// already emitted its result, and a later explicit `viceme update` remains the
// repair path when the host cannot update in the background.
func runAutomaticUpdateWorker(dependencies *Dependencies) {
	if dependencies == nil || os.Getenv("CI") != "" {
		return
	}
	currentVersion := buildinfo.CompatibilityVersion()
	if _, err := semver.Parse(currentVersion); err != nil && !dependencies.allowDevelopmentAutoUpdate {
		return
	}
	configBase := runtimeConfigBase(dependencies.Environment)
	if err := os.MkdirAll(configBase, 0o700); err != nil {
		return
	}
	workerLock := flock.New(filepath.Join(configBase, automaticUpdateLockFilename))
	locked, err := workerLock.TryLock()
	if err != nil || !locked {
		return
	}
	defer workerLock.Unlock()

	now := dependencies.Now()
	if state, ok := readAutomaticUpdateState(configBase); ok && !automaticUpdateStateIsDue(state, currentVersion, now) {
		return
	}

	state := automaticUpdateState{SchemaVersion: 1, CurrentVersion: currentVersion, Status: "checking"}
	writeAutomaticUpdateState(configBase, state)
	finish := func(status, available string, workerErr error) {
		state.Status = status
		state.AvailableVersion = available
		state.CheckedAt = dependencies.Now().Unix()
		if workerErr != nil {
			state.ErrorKind = string(updatepkg.ErrorKindOf(workerErr))
		}
		writeAutomaticUpdateState(configBase, state)
	}

	workerContext, cancel := context.WithTimeout(context.Background(), activationOperationTimeout)
	defer cancel()
	dependencies.activationMutationCommand = true
	if err := reconcileActivationAtStartup(workerContext, configBase, dependencies); err != nil {
		status := "failed"
		if errors.Is(err, updatepkg.ErrActivationRestartNeeded) {
			status = "superseded"
		}
		finish(status, "", err)
		return
	}
	if resolved, err := config.LoadOrDefault(configBase); err == nil {
		if regionAware, ok := dependencies.Updater.(updatepkg.RegionAware); ok {
			regionAware.SetRegion(string(resolved.DistributionRegion))
		}
	} else {
		finish("failed", "", err)
		return
	}

	var check updatepkg.CheckResult
	if checker, ok := dependencies.Updater.(updatepkg.AutomaticChecker); ok {
		check, err = checker.CheckAutomatic(workerContext)
	} else {
		check, err = dependencies.Updater.Check(workerContext)
	}
	if err != nil {
		finish("failed", "", err)
		return
	}
	if !check.UpdateAvailable {
		finish("current", check.AvailableVersion, nil)
		return
	}

	result, err := dependencies.Updater.Apply(workerContext, check, updatepkg.ApplyOptions{RefreshSkills: false})
	if err != nil {
		finish("failed", check.AvailableVersion, err)
		return
	}
	status := "updated"
	for _, target := range result.Targets {
		if target.Status == "scheduled" {
			status = "scheduled"
			break
		}
	}
	finish(status, result.CLIVersion, nil)
}

func automaticUpdateStateIsDue(state automaticUpdateState, currentVersion string, now time.Time) bool {
	if state.SchemaVersion != 1 || state.CurrentVersion != currentVersion || state.CheckedAt <= 0 {
		return true
	}
	switch state.Status {
	case "current", "updated", "scheduled", "failed", "superseded":
	default:
		return true
	}
	if _, err := semver.Parse(state.CurrentVersion); err != nil {
		return true
	}
	if state.AvailableVersion != "" {
		if _, err := semver.Parse(state.AvailableVersion); err != nil {
			return true
		}
	}
	age := now.Sub(time.Unix(state.CheckedAt, 0))
	if age < 0 {
		return true
	}
	interval := automaticUpdateCheckInterval
	if state.Status == "failed" {
		interval = automaticUpdateRetryInterval
	}
	return age > interval
}

func readAutomaticUpdateState(configBase string) (automaticUpdateState, bool) {
	data, err := os.ReadFile(filepath.Join(configBase, automaticUpdateStateFilename))
	if err != nil {
		return automaticUpdateState{}, false
	}
	var state automaticUpdateState
	if json.Unmarshal(data, &state) != nil {
		return automaticUpdateState{}, false
	}
	return state, true
}

func writeAutomaticUpdateState(configBase string, state automaticUpdateState) {
	data, err := json.Marshal(state)
	if err != nil {
		return
	}
	_ = privatefile.Write(filepath.Join(configBase, automaticUpdateStateFilename), data, ".automatic-update-*.tmp")
}
