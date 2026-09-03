package command

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	updatepkg "github.com/ViceMe-AI/cli/internal/update"
)

type reexecutionNPMRunner struct{ root string }

func (runner reexecutionNPMRunner) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return []byte(runner.root + "\n"), nil
}

func TestNPMReexecutionContinuesThroughActivatedGlobalLauncher(t *testing.T) {
	clearAutomaticUpdateReexecutionEnvironment(t)
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is required for the real launcher handoff")
	}
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	npmRoot := filepath.Join(root, "npm prefix with spaces", "node_modules")
	packageDir := filepath.Join(npmRoot, "@viceme-ai", "cli")
	launcher := filepath.Join(packageDir, "npm", "bin", "viceme.mjs")
	if err := os.MkdirAll(filepath.Dir(launcher), 0o700); err != nil {
		t.Fatal(err)
	}
	targetVersion := versionAfterCurrentRelease(t)
	manifest, _ := json.Marshal(map[string]string{"name": updatepkg.PackageName, "version": targetVersion})
	if err := os.WriteFile(filepath.Join(packageDir, "package.json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	// Exercise the actual default subprocess handoff, including streams and
	// exit status. The stale npx entry must never be invoked after activation.
	if err := os.WriteFile(launcher, []byte(`import fs from "node:fs";
console.log(JSON.stringify({args: process.argv.slice(2), input: fs.readFileSync(0, "utf8"), to: process.env.VICEME_AUTO_UPDATE_TO}));
console.error("activated launcher");
process.exitCode = 7;
`), 0o600); err != nil {
		t.Fatal(err)
	}
	staleLauncher := filepath.Join(root, "stale-npx.mjs")
	if err := os.WriteFile(staleLauncher, []byte(`console.log("stale launcher"); process.exitCode = 99;`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VICEME_INSTALL_METHOD", "npm")
	t.Setenv("VICEME_NPM_LAUNCHER_PATH", staleLauncher)
	t.Setenv(npmLauncherRuntimeEnvironment, node)
	active, err := updatepkg.NewNPMGeneration(targetVersion)
	if err != nil {
		t.Fatal(err)
	}
	if err := updatepkg.CommitActiveGeneration(configDir, active); err != nil {
		t.Fatal(err)
	}
	updater := updatepkg.NewNPMService(buildinfo.Version, buildinfo.CompatibilityVersion(), "npm")
	updater.ConfigDir = configDir
	updater.Runner = reexecutionNPMRunner{root: npmRoot}
	var stdout, stderr bytes.Buffer
	args := []string{"version", "--profile", "profile with spaces"}
	dependencies := defaults(Dependencies{
		In: bytes.NewBufferString("original stdin"), Out: &stdout, ErrOut: &stderr,
		Store: securestore.NewMemory(), Updater: updater,
		Environment: skillcontent.Environment{Home: root, ConfigDir: configDir}, Region: config.RegionCN,
	})
	exit := reexecuteOriginalCommand(args, dependencies, &output.Printer{Out: &stdout, ErrOut: &stderr}, buildinfo.CompatibilityVersion(), targetVersion)
	if exit != 7 {
		t.Fatalf("handoff exit=%d stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
	}
	var result struct {
		Args  []string `json:"args"`
		Input string   `json:"input"`
		To    string   `json:"to"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Args, args) || result.Input != "original stdin" || result.To != targetVersion || stderr.String() != "activated launcher\n" {
		t.Fatalf("handoff changed command streams or target: %#v stderr=%q", result, stderr.String())
	}
	// A stale global installation must fail with one JSON envelope, without
	// falling back to the cached old launcher or running business logic.
	if err := os.WriteFile(filepath.Join(packageDir, "package.json"), []byte(`{"name":"@viceme-ai/cli","version":"0.0.1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	exit = reexecuteOriginalCommand(args, dependencies, &output.Printer{Out: &stdout, ErrOut: &stderr}, buildinfo.CompatibilityVersion(), targetVersion)
	var failure struct {
		Error struct{ Code string } `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &failure); err != nil {
		t.Fatal(err)
	}
	if exit == 0 || failure.Error.Code != "AUTO_UPDATE_REEXEC_FAILED" || stderr.Len() != 0 {
		t.Fatalf("mismatched launcher ran or lost error contract: exit=%d stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
	}
}
