//go:build windows

package command

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/ViceMe-AI/cli/internal/privatepath"
	"github.com/ViceMe-AI/cli/internal/replicapublication"
)

func TestWindowsBindingPreflightRemainsReusable(t *testing.T) {
	for _, bindingFirst := range []bool{false, true} {
		name := "store-first"
		if bindingFirst {
			name = "binding-first"
		}
		t.Run(name, func(t *testing.T) {
			project := t.TempDir()
			binding := replicapublication.BindingStore{}
			if bindingFirst {
				if err := binding.Preflight(project); err != nil {
					t.Fatalf("initial binding preflight: %v; cause: %v", err, errors.Unwrap(err))
				}
			}
			directory, err := replicapublication.ProjectStoreDirectory(project, "https://api.viceme.cn", "CN")
			if err != nil {
				t.Fatal(err)
			}
			store := replicapublication.Store{Directory: directory, EndpointOrigin: "https://api.viceme.cn", Market: "CN", ProjectScoped: true}
			if err := store.Preflight(); err != nil {
				t.Fatalf("project store preflight: %v; cause: %v", err, errors.Unwrap(err))
			}
			if err := binding.Preflight(project); err != nil {
				t.Fatalf("repeated binding preflight: %v; cause: %v", err, errors.Unwrap(err))
			}
			for _, path := range []string{filepath.Join(project, ".viceme"), filepath.Join(project, ".viceme", "publications"), directory} {
				if err := privatepath.RequirePrivateDirectory(path); err != nil {
					t.Fatalf("managed directory %s privacy: %v", filepath.Base(path), err)
				}
			}
			if err := binding.Preflight(project); err != nil {
				t.Fatalf("repeated binding preflight: %v; cause: %v", err, errors.Unwrap(err))
			}

		})
	}
}
