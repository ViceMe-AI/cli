package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestNPMRecoveryAfterSemanticCommitOnlyRetiresJournal(t *testing.T) {
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
	runner := &fakeRunner{errors: []error{errors.New("network must not be used")}}
	service.Runner = runner
	journal, err := service.newNPMActivationJournal(target, "agents", true)
	if err != nil {
		t.Fatal(err)
	}
	journal.Status = "COMMITTING"
	if err := service.writeNPMActivation(journal); err != nil {
		t.Fatal(err)
	}
	if err := CommitActiveGeneration(configDir, target); err != nil {
		t.Fatal(err)
	}

	if err := service.RecoverAtStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("committed target was reapplied over the network: %#v", runner.calls)
	}
	if _, err := os.Stat(filepath.Join(configDir, npmActivationFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("committed journal was not retired: %v", err)
	}
}

func TestNPMCommittedJournalFinalizesLocallyWithoutNetwork(t *testing.T) {
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
	runner := &fakeRunner{errors: []error{errors.New("network must not be used")}}
	service.Runner = runner
	journal, err := service.newNPMActivationJournal(target, "agents", true)
	if err != nil {
		t.Fatal(err)
	}
	journal.Status = "COMMITTED"
	if err := service.writeNPMActivation(journal); err != nil {
		t.Fatal(err)
	}

	if err := service.RecoverAtStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("committed journal performed a network mutation: %#v", runner.calls)
	}
	active, exists, err := ReadActiveGeneration(configDir)
	if err != nil || !exists || active != target {
		t.Fatalf("committed journal did not finalize target: active=%#v exists=%t err=%v", active, exists, err)
	}
}

func TestNPMSameGenerationRepairIsNotMistakenForAnEarlierCommit(t *testing.T) {
	t.Parallel()
	configDir := t.TempDir()
	target, err := NewNPMGeneration("1.2.4")
	if err != nil {
		t.Fatal(err)
	}
	if err := CommitActiveGeneration(configDir, target); err != nil {
		t.Fatal(err)
	}
	service := NewNPMService("1.2.4", "1.2.4", "npm")
	service.ConfigDir = configDir
	runner := &fakeRunner{}
	service.Runner = runner
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
	if len(runner.calls) != 2 {
		t.Fatalf("same-generation Skill repair was silently skipped: %#v", runner.calls)
	}
}

func TestNPMRolledBackJournalFinalizesLocallyWithoutNetwork(t *testing.T) {
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
	runner := &fakeRunner{errors: []error{errors.New("network must not be used")}}
	service.Runner = runner
	journal, err := service.newNPMActivationJournal(target, "agents", true)
	if err != nil {
		t.Fatal(err)
	}
	journal.Status = "ROLLED_BACK"
	if err := service.writeNPMActivation(journal); err != nil {
		t.Fatal(err)
	}

	if err := service.RecoverAtStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("rolled-back journal performed a network mutation: %#v", runner.calls)
	}
	if _, err := os.Stat(filepath.Join(configDir, npmActivationFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rolled-back journal was not retired: %v", err)
	}
}

func TestNPMActivationRejectsCrossMethodMigrationBeforeMutation(t *testing.T) {
	t.Parallel()
	configDir := t.TempDir()
	standalone, err := NewStandaloneGeneration("1.2.3", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	if err := CommitActiveGeneration(configDir, standalone); err != nil {
		t.Fatal(err)
	}
	service := NewNPMService("1.2.4", "1.2.4", "npm")
	service.ConfigDir = configDir
	runner := &fakeRunner{}
	service.Runner = runner
	_, err = service.Apply(
		context.Background(),
		CheckResult{AvailableVersion: "1.2.4", UpdateAvailable: false},
		ApplyOptions{RefreshSkills: true},
	)
	if !errors.Is(err, ErrActivationMethodChange) {
		t.Fatalf("cross-method activation was not rejected: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("cross-method activation mutated npm state: %#v", runner.calls)
	}
}

func TestNPMActivationChildRequiresExactJournalNonceAndTarget(t *testing.T) {
	t.Parallel()
	configDir := t.TempDir()
	target, err := NewNPMGeneration("1.2.4")
	if err != nil {
		t.Fatal(err)
	}
	service := NewNPMService("1.2.4", "1.2.4", "npm")
	service.ConfigDir = configDir
	journal, err := service.newNPMActivationJournal(target, "codex", true)
	if err != nil {
		t.Fatal(err)
	}
	journal.Status = "COMMITTING"
	if err := service.writeNPMActivation(journal); err != nil {
		t.Fatal(err)
	}
	validated, err := service.ValidateActivationChild(journal.Nonce, "1.2.4", "codex")
	if err != nil || validated != target {
		t.Fatalf("exact activation child was rejected: target=%#v err=%v", validated, err)
	}
	for _, attempt := range []struct {
		nonce   string
		version string
		target  string
	}{
		{nonce: strings.Repeat("0", len(journal.Nonce)), version: "1.2.4", target: "codex"},
		{nonce: journal.Nonce, version: "1.2.3", target: "codex"},
		{nonce: journal.Nonce, version: "1.2.4", target: "agents"},
	} {
		if _, err := service.ValidateActivationChild(attempt.nonce, attempt.version, attempt.target); err == nil {
			t.Fatalf("mismatched activation child was authorized: %#v", attempt)
		}
	}
	if err := service.ConfirmActivationChildCommitted(journal.Nonce, "1.2.4", "codex"); err != nil {
		t.Fatalf("exact child commit was not recorded: %v", err)
	}
	if _, err := service.ValidateActivationChild(journal.Nonce, "1.2.4", "codex"); err == nil {
		t.Fatal("a consumed activation child nonce authorized a second Skill commit")
	}
}
