package command

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	updatepkg "github.com/ViceMe-AI/cli/internal/update"
)

func TestNPMEntryAdoptsExternalUpgradeWithoutJournal(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	previous, err := updatepkg.NewNPMGeneration("0.16.0-beta.5")
	if err != nil {
		t.Fatal(err)
	}
	if err := updatepkg.CommitActiveGeneration(configDir, previous); err != nil {
		t.Fatal(err)
	}

	updater := updatepkg.NewNPMService(buildinfo.Version, "0.16.0-beta.6", "npm")
	updater.ConfigDir = configDir
	dependencies := Dependencies{
		Updater: updater, Environment: skillcontent.Environment{Home: root, ConfigDir: configDir},
	}
	if err := reconcileActivationAtStartup(context.Background(), configDir, &dependencies); err != nil {
		t.Fatalf("external npm upgrade was not adopted: %v", err)
	}
	active, exists, err := updatepkg.ReadActiveGeneration(configDir)
	if err != nil || !exists || active.Version != "0.16.0-beta.6" || active.InstallMethod != "npm" {
		t.Fatalf("active generation was not advanced: active=%#v exists=%t err=%v", active, exists, err)
	}
}

func TestStandaloneToNPMMigrationIsAdoptedWhenVersionIncreases(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	legacy, err := updatepkg.NewStandaloneGeneration("0.13.2", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	if err := updatepkg.CommitActiveGeneration(configDir, legacy); err != nil {
		t.Fatal(err)
	}

	updater := updatepkg.NewNPMService(buildinfo.Version, "0.16.0-beta.6", "npm")
	updater.ConfigDir = configDir
	dependencies := Dependencies{
		Updater: updater, Environment: skillcontent.Environment{Home: root, ConfigDir: configDir},
	}
	if err := reconcileActivationAtStartup(context.Background(), configDir, &dependencies); err != nil {
		t.Fatalf("standalone to npm migration was not adopted: %v", err)
	}
	active, exists, err := updatepkg.ReadActiveGeneration(configDir)
	if err != nil || !exists || active.InstallMethod != "npm" || active.Version != "0.16.0-beta.6" {
		t.Fatalf("active generation did not migrate: active=%#v exists=%t err=%v", active, exists, err)
	}
}

func TestSameVersionAcrossInstallMethodsIsRejected(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	legacy, err := updatepkg.NewStandaloneGeneration("0.16.0-beta.6", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	if err := updatepkg.CommitActiveGeneration(configDir, legacy); err != nil {
		t.Fatal(err)
	}

	updater := updatepkg.NewNPMService(buildinfo.Version, "0.16.0-beta.6", "npm")
	updater.ConfigDir = configDir
	dependencies := Dependencies{
		Updater: updater, Environment: skillcontent.Environment{Home: root, ConfigDir: configDir},
	}
	if err := reconcileActivationAtStartup(context.Background(), configDir, &dependencies); !errors.Is(err, updatepkg.ErrActivationRestartNeeded) {
		t.Fatalf("same-version method switch was not blocked: %v", err)
	}
}

func TestNPMEntryRejectsExternalDowngradeWithoutJournal(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	current, err := updatepkg.NewNPMGeneration("0.16.0-beta.6")
	if err != nil {
		t.Fatal(err)
	}
	if err := updatepkg.CommitActiveGeneration(configDir, current); err != nil {
		t.Fatal(err)
	}

	updater := updatepkg.NewNPMService(buildinfo.Version, "0.16.0-beta.5", "npm")
	updater.ConfigDir = configDir
	dependencies := Dependencies{
		Updater: updater, Environment: skillcontent.Environment{Home: root, ConfigDir: configDir},
	}
	if err := reconcileActivationAtStartup(context.Background(), configDir, &dependencies); !errors.Is(err, updatepkg.ErrActivationRestartNeeded) {
		t.Fatalf("external downgrade was not blocked: %v", err)
	}
	after, exists, err := updatepkg.ReadActiveGeneration(configDir)
	if err != nil || !exists || after != current {
		t.Fatalf("blocked downgrade mutated active generation: active=%#v exists=%t err=%v", after, exists, err)
	}
}
