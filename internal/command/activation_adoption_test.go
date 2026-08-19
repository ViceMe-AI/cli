package command

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	updatepkg "github.com/ViceMe-AI/cli/internal/update"
)

// 外部升级（直接 npm install -g，无 activation journal）在合法递增时必须
// 自动提交新代际，而不是反复要求重启。
func TestNPMEntryAdoptsExternalUpgradeWithoutJournal(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	previous, err := updatepkg.NewNPMGeneration("0.10.0")
	if err != nil {
		t.Fatal(err)
	}
	if err := updatepkg.CommitActiveGeneration(configDir, previous); err != nil {
		t.Fatal(err)
	}

	const upgraded = "9.9.9"
	updater := updatepkg.NewNPMService(buildinfo.Version, upgraded, "npm")
	updater.ConfigDir = configDir
	dependencies := Dependencies{
		Updater:     updater,
		Environment: skillcontent.Environment{Home: root, ConfigDir: configDir},
	}
	if err := reconcileActivationAtStartup(context.Background(), configDir, &dependencies); err != nil {
		t.Fatalf("external npm upgrade was not adopted automatically: %v", err)
	}
	active, exists, err := updatepkg.ReadActiveGeneration(configDir)
	if err != nil || !exists {
		t.Fatalf("active generation missing after adoption: exists=%t err=%v", exists, err)
	}
	running, err := updatepkg.NewNPMGeneration(upgraded)
	if err != nil {
		t.Fatal(err)
	}
	if active != running {
		t.Fatalf("active generation was not advanced to the running generation: active=%#v running=%#v", active, running)
	}
}

// 外部降级仍然拒绝：代际回退可能对应被撤销的发布，不能静默接受。
func TestNPMEntryRejectsExternalDowngradeWithoutJournal(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	active, err := updatepkg.NewNPMGeneration("9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	if err := updatepkg.CommitActiveGeneration(configDir, active); err != nil {
		t.Fatal(err)
	}

	updater := updatepkg.NewNPMService(buildinfo.Version, "0.10.0", "npm")
	updater.ConfigDir = configDir
	dependencies := Dependencies{
		Updater:     updater,
		Environment: skillcontent.Environment{Home: root, ConfigDir: configDir},
	}
	if err := reconcileActivationAtStartup(context.Background(), configDir, &dependencies); !errors.Is(err, updatepkg.ErrActivationRestartNeeded) {
		t.Fatalf("external npm downgrade was not blocked: %v", err)
	}
	after, exists, err := updatepkg.ReadActiveGeneration(configDir)
	if err != nil || !exists || after != active {
		t.Fatalf("blocked downgrade mutated the active generation: active=%#v exists=%t err=%v", after, exists, err)
	}
}
