package update

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type permissionRunner struct {
	prefix       string
	probeError   string
	installError string
	installs     int
	execs        int
	realProbe    bool
	shim         string
}

func (r *permissionRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if name == "node" {
		if r.probeError != "" {
			return []byte(r.probeError), errors.New("probe denied")
		}
		if r.realProbe {
			if r.shim != "" {
				args = append([]string{"--require", r.shim}, args...)
			}
			return ExecRunner{}.Run(ctx, name, args...)
		}
		return nil, nil
	}
	if slices.Contains(args, "root") {
		return []byte(filepath.Join(r.prefix, "lib", "node_modules")), nil
	}
	if slices.Contains(args, "prefix") {
		return []byte(r.prefix), nil
	}
	if slices.Contains(args, "install") && slices.Contains(args, "--global") {
		r.installs++
		if r.installError != "" {
			return []byte(r.installError), errors.New("install denied")
		}
		return nil, nil
	}
	if slices.Contains(args, "exec") {
		r.execs++
		return nil, nil
	}
	return nil, errors.New("unexpected runner call")
}

func permissionService(t *testing.T, runner *permissionRunner) *NPMService {
	t.Helper()
	service := NewNPMService("1.2.3", "1.2.3", "npm")
	service.ConfigDir = filepath.Join(t.TempDir(), "config")
	service.Runner = runner
	previous, err := NewNPMGeneration("1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if err := CommitActiveGeneration(service.ConfigDir, previous); err != nil {
		t.Fatal(err)
	}
	return service
}

func TestNPMPermissionPreflightDoesNotCreateRecoveryLoop(t *testing.T) {
	for _, failure := range []string{"CODEBUDDY_BROKER_DENY ENOTEMPTY", "EPERM", "EACCES", "EROFS"} {
		t.Run(failure, func(t *testing.T) {
			runner := &permissionRunner{prefix: t.TempDir(), probeError: failure}
			service := permissionService(t, runner)
			for range 2 {
				result, err := service.Apply(context.Background(), CheckResult{AvailableVersion: "1.2.4", UpdateAvailable: true}, ApplyOptions{RefreshSkills: true})
				if !IsPermissionDenied(err) || len(result.Targets) != 2 || result.Targets[0].Status != "blocked" || result.Targets[1].Status != "blocked" {
					t.Fatalf("result=%#v err=%v", result, err)
				}
				if _, exists, err := service.readNPMActivation(); err != nil || exists || runner.installs != 0 || runner.execs != 0 {
					t.Fatalf("denied preflight mutated installation: journal=%t err=%v runner=%#v", exists, err, runner)
				}
				if err := service.RecoverAtStartup(context.Background()); err != nil {
					t.Fatalf("unchanged version could not start: %v", err)
				}
			}
			// Host grants access; the same public update path must succeed.
			runner.probeError = ""
			if _, err := service.Apply(context.Background(), CheckResult{AvailableVersion: "1.2.4", UpdateAvailable: true}, ApplyOptions{RefreshSkills: true}); err != nil {
				t.Fatal(err)
			}
			active, exists, err := ReadActiveGeneration(service.ConfigDir)
			if err != nil || !exists || active.Version != "1.2.4" || runner.installs != 1 || runner.execs != 1 {
				t.Fatalf("authorized update did not finish: active=%#v err=%v runner=%#v", active, err, runner)
			}
		})
	}
}

func TestNPMRecoveryPermissionDenialPreservesJournal(t *testing.T) {
	for _, status := range []string{"PREPARING", "COMMITTING", "ROLLING_BACK"} {
		t.Run(status, func(t *testing.T) {
			runner := &permissionRunner{prefix: t.TempDir(), probeError: "CODEBUDDY_BROKER_DENY"}
			service := permissionService(t, runner)
			target, _ := NewNPMGeneration("1.2.4")
			journal, err := service.newNPMActivationJournal(target, "auto", true)
			if err != nil {
				t.Fatal(err)
			}
			journal.Status = status
			if err := service.writeNPMActivation(journal); err != nil {
				t.Fatal(err)
			}
			filename := filepath.Join(service.ConfigDir, npmActivationFilename)
			before, _ := os.ReadFile(filename)
			for range 2 {
				if err := service.RecoverAtStartup(context.Background()); !IsPermissionDenied(err) {
					t.Fatalf("want permission refusal, got %v", err)
				}
				after, _ := os.ReadFile(filename)
				if !bytes.Equal(before, after) || runner.installs != 0 || runner.execs != 0 {
					t.Fatal("recovery changed journal or attempted npm mutation without permission")
				}
			}
			runner.probeError = ""
			if err := service.RecoverActivationWhileLocked(context.Background()); err != nil {
				t.Fatalf("authorized recovery failed: %v", err)
			}
			if _, exists, err := service.readNPMActivation(); err != nil || exists {
				t.Fatalf("recovery journal remained: exists=%t err=%v", exists, err)
			}
		})
	}
}

func TestNPMPermissionChangedAfterPreflightRequiresRecovery(t *testing.T) {
	runner := &permissionRunner{prefix: t.TempDir(), installError: "CODEBUDDY_BROKER_DENY"}
	service := permissionService(t, runner)
	result, err := service.Apply(context.Background(), CheckResult{AvailableVersion: "1.2.4", UpdateAvailable: true}, ApplyOptions{RefreshSkills: true})
	if !IsPermissionDenied(err) || result.Targets[0].Status != "recovery_pending" || runner.installs != 1 {
		t.Fatalf("partial attempt incorrectly declared safe: result=%#v err=%v runner=%#v", result, err, runner)
	}
	before, _ := os.ReadFile(filepath.Join(service.ConfigDir, npmActivationFilename))
	if err := service.RecoverAtStartup(context.Background()); !IsPermissionDenied(err) {
		t.Fatalf("want permission error, got %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(service.ConfigDir, npmActivationFilename))
	if !bytes.Equal(before, after) || runner.installs != 2 || runner.execs != 0 {
		t.Fatal("permission denial must not trigger another install to roll back")
	}
}

func TestNPMProbeUsesRealNodeBrokerAndLeavesPackageUntouched(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("Node is required to test the npm filesystem boundary")
	}
	for _, deny := range []bool{false, true} {
		t.Run(map[bool]string{false: "allowed", true: "broker_denied"}[deny], func(t *testing.T) {
			prefix := filepath.Join(t.TempDir(), "prefix with spaces")
			scope := filepath.Join(prefix, "lib", "node_modules", "@viceme-ai")
			packageDir := filepath.Join(scope, "cli")
			for _, dir := range []string{packageDir, filepath.Join(prefix, "bin")} {
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			entry := filepath.Join(packageDir, "package.json")
			if err := os.WriteFile(entry, []byte("original package"), 0o600); err != nil {
				t.Fatal(err)
			}
			runner := &permissionRunner{prefix: prefix, realProbe: true}
			if deny {
				runner.shim = filepath.Join(t.TempDir(), "broker.cjs")
				// Model the recorded host's Node broker, while Go configuration
				// renames remain allowed. No host permission controls are changed.
				shim := `const fs = require('node:fs/promises'); const rename = fs.rename; fs.rename = async (from, to) => { if (from.includes('@viceme-ai')) throw Object.assign(new Error('private diagnostic'), {code:'CODEBUDDY_BROKER_DENY'}); return rename(from, to); };`
				if err := os.WriteFile(runner.shim, []byte(shim), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			service := permissionService(t, runner)
			err := service.probeNPMActivation(context.Background())
			if (err == nil) == deny || (deny && !IsPermissionDenied(err)) {
				t.Fatalf("deny=%t err=%v", deny, err)
			}
			data, _ := os.ReadFile(entry)
			if string(data) != "original package" {
				t.Fatal("preflight touched the installed package")
			}
			if err := filepath.WalkDir(prefix, func(path string, _ os.DirEntry, err error) error {
				if strings.Contains(filepath.Base(path), ".viceme-permission-probe-") {
					t.Errorf("probe left debris: %s", path)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
