package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestActiveGenerationRejectsLateOlderTarget(t *testing.T) {
	t.Parallel()
	configDir := t.TempDir()
	newer, err := NewNPMGeneration("1.2.4")
	if err != nil {
		t.Fatal(err)
	}
	if err := CommitActiveGeneration(configDir, newer); err != nil {
		t.Fatal(err)
	}
	older, err := NewNPMGeneration("1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateActivationTarget(configDir, older); !errors.Is(err, ErrActivationDowngrade) {
		t.Fatalf("late older target was not fenced: %v", err)
	}
	active, exists, err := ReadActiveGeneration(configDir)
	if err != nil || !exists || active != newer {
		t.Fatalf("newer active generation changed: active=%#v exists=%t err=%v", active, exists, err)
	}
}

func TestNPMApplyDoesNotRunOlderTargetAfterNewerGenerationCommitted(t *testing.T) {
	t.Parallel()
	configDir := t.TempDir()
	newer, err := NewNPMGeneration("1.2.4")
	if err != nil {
		t.Fatal(err)
	}
	if err := CommitActiveGeneration(configDir, newer); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	service := NewNPMService("1.2.3", "1.2.3", "npm")
	service.ConfigDir = configDir
	service.Runner = runner
	_, err = service.Apply(
		context.Background(),
		CheckResult{AvailableVersion: "1.2.3", UpdateAvailable: false},
		ApplyOptions{RefreshSkills: true},
	)
	if !errors.Is(err, ErrActivationDowngrade) {
		t.Fatalf("stale updater was not fenced: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("stale updater invoked npm: %#v", runner.calls)
	}
}

func TestNPMStartupRecoveryCompletesGenerationBeforeOrdinaryCommand(t *testing.T) {
	t.Parallel()
	configDir := t.TempDir()
	previous, err := NewNPMGeneration("1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if err := CommitActiveGeneration(configDir, previous); err != nil {
		t.Fatal(err)
	}
	target, err := NewNPMGeneration("1.2.4")
	if err != nil {
		t.Fatal(err)
	}
	service := NewNPMService("1.2.4", "1.2.4", "npm")
	service.ConfigDir = configDir
	service.Runner = &fakeRunner{}
	journal, err := service.newNPMActivationJournal(target, "agents", true)
	if err != nil {
		t.Fatal(err)
	}
	journal.Status = "COMMITTING"
	if err := service.writeNPMActivation(journal); err != nil {
		t.Fatal(err)
	}
	if err := service.RecoverAtStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(configDir, npmActivationFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("startup recovery did not retire journal: %v", err)
	}
	active, exists, err := ReadActiveGeneration(configDir)
	if err != nil || !exists || active != target {
		t.Fatalf("startup recovery did not commit target: active=%#v exists=%t err=%v", active, exists, err)
	}
	runner := service.Runner.(*fakeRunner)
	if len(runner.calls) != 2 {
		t.Fatalf("startup recovery did not restore launcher and Skills: %#v", runner.calls)
	}
}

func TestNPMStartupRecoveryRequiresRestartWhenCurrentProcessIsPreviousGeneration(t *testing.T) {
	t.Parallel()
	configDir := t.TempDir()
	previous, err := NewNPMGeneration("1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if err := CommitActiveGeneration(configDir, previous); err != nil {
		t.Fatal(err)
	}
	target, err := NewNPMGeneration("1.2.4")
	if err != nil {
		t.Fatal(err)
	}
	service := NewNPMService("1.2.3", "1.2.3", "npm")
	service.ConfigDir = configDir
	service.Runner = &fakeRunner{}
	journal, err := service.newNPMActivationJournal(target, "agents", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.writeNPMActivation(journal); err != nil {
		t.Fatal(err)
	}
	if err := service.RecoverAtStartup(context.Background()); !errors.Is(err, ErrActivationRestartNeeded) {
		t.Fatalf("old process continued after rolling forward a newer generation: %v", err)
	}
	active, exists, err := ReadActiveGeneration(configDir)
	if err != nil || !exists || active != target {
		t.Fatalf("target generation was not durably recovered: active=%#v exists=%t err=%v", active, exists, err)
	}
}

func TestNPMStartupRecoveryRestoresPreviousGenerationWhenForwardFails(t *testing.T) {
	t.Parallel()
	configDir := t.TempDir()
	previous, err := NewNPMGeneration("1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if err := CommitActiveGeneration(configDir, previous); err != nil {
		t.Fatal(err)
	}
	target, err := NewNPMGeneration("1.2.4")
	if err != nil {
		t.Fatal(err)
	}
	service := NewNPMService("1.2.4", "1.2.4", "npm")
	service.ConfigDir = configDir
	service.Runner = &fakeRunner{errors: []error{errors.New("target unavailable"), nil, nil}}
	journal, err := service.newNPMActivationJournal(target, "codex", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.writeNPMActivation(journal); err != nil {
		t.Fatal(err)
	}
	if err := service.RecoverAtStartup(context.Background()); !errors.Is(err, ErrActivationRestartNeeded) {
		t.Fatalf("restoring the previous launcher did not fence the current process: %v", err)
	}
	active, exists, err := ReadActiveGeneration(configDir)
	if err != nil || !exists || active != previous {
		t.Fatalf("rollback did not restore previous generation: active=%#v exists=%t err=%v", active, exists, err)
	}
	runner := service.Runner.(*fakeRunner)
	if len(runner.calls) != 3 {
		t.Fatalf("rollback did not attempt target then previous launcher and Skills: %#v", runner.calls)
	}
}
