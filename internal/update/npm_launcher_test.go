package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNPMReexecutionLauncherValidatesInstalledPackage(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		version    string
		rootOutput string
		manifest   string
		missing    bool
		directory  bool
		npmError   error
		wantOK     bool
	}{
		{name: "matching package", wantOK: true},
		{name: "invalid target", version: "latest"},
		{name: "relative npm root", rootOutput: "node_modules"},
		{name: "multiple npm roots", rootOutput: "/first\n/second\n"},
		{name: "different package", manifest: `{"name":"other","version":"1.2.3"}`},
		{name: "stale version", manifest: `{"name":"@viceme-ai/cli","version":"1.2.2"}`},
		{name: "newer concurrent version", manifest: `{"name":"@viceme-ai/cli","version":"1.2.4"}`},
		{name: "invalid manifest", manifest: `{`},
		{name: "missing launcher", missing: true},
		{name: "directory launcher", directory: true},
		{name: "npm failure", npmError: errors.New("npm failed")},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := filepath.Join(t.TempDir(), "npm prefix with spaces", "node_modules")
			packageDir := filepath.Join(root, "@viceme-ai", "cli")
			launcher := filepath.Join(packageDir, "npm", "bin", "viceme.mjs")
			if err := os.MkdirAll(filepath.Dir(launcher), 0o700); err != nil {
				t.Fatal(err)
			}
			manifest := test.manifest
			if manifest == "" {
				manifest = `{"name":"@viceme-ai/cli","version":"1.2.3"}`
			}
			if err := os.WriteFile(filepath.Join(packageDir, "package.json"), []byte(manifest), 0o600); err != nil {
				t.Fatal(err)
			}
			if test.directory {
				if err := os.Mkdir(launcher, 0o700); err != nil {
					t.Fatal(err)
				}
			} else if !test.missing {
				if err := os.WriteFile(launcher, []byte("// launcher"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			rootOutput := test.rootOutput
			if rootOutput == "" {
				rootOutput = root + "\r\n"
			}
			runner := &fakeRunner{outputs: [][]byte{[]byte(rootOutput)}, errors: []error{test.npmError}}
			service := NewNPMService("1.2.2", "1.2.2", "npm")
			service.Runner = runner
			version := test.version
			if version == "" {
				version = "1.2.3"
			}
			got, err := service.ReexecutionLauncher(context.Background(), version)
			if (err == nil) != test.wantOK || (test.wantOK && got != launcher) {
				t.Fatalf("launcher=%q err=%v", got, err)
			}
			if test.version != "" {
				if len(runner.calls) != 0 {
					t.Fatal("invalid target invoked npm")
				}
			} else if len(runner.calls) != 1 || runner.calls[0].name != "npm" || !reflect.DeepEqual(runner.calls[0].args, []string{"root", "--global", "--loglevel=silent", "--no-update-notifier"}) {
				t.Fatalf("resolution must only query the installed npm root: %#v", runner.calls)
			}
		})
	}
}
