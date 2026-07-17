package command

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/spf13/cobra"
)

func TestCheckedInCommandManifestMatchesCobraSurface(t *testing.T) {
	t.Parallel()
	root, _, err := NewRoot(Dependencies{Store: securestore.NewMemory()})
	if err != nil {
		t.Fatal(err)
	}
	actual := BuildCommandManifest(root)
	filename := filepath.Join(repositoryRoot(t), "skills", "viceme", "references", "command-manifest.json")
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	var expected CommandManifest
	if err := json.Unmarshal(data, &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) {
		encoded, _ := json.MarshalIndent(actual, "", "  ")
		t.Fatalf("command manifest drifted; regenerate it from the Cobra tree:\n%s\n", encoded)
	}
}

func TestMutatingCommandsHaveExplicitManifestPolicy(t *testing.T) {
	t.Parallel()
	root, _, err := NewRoot(Dependencies{Store: securestore.NewMemory()})
	if err != nil {
		t.Fatal(err)
	}
	manifest := BuildCommandManifest(root)
	for _, command := range manifest.Commands {
		if command.Runnable && command.SideEffect == "" {
			t.Errorf("runnable command %q has no side-effect classification", command.Path)
		}
		if command.RequiresConfirmation && command.SideEffect != "public" {
			t.Errorf("command %q requires confirmation but is not public", command.Path)
		}
	}
}

func TestNewRunnableCommandMustDeclareManifestPolicy(t *testing.T) {
	t.Parallel()
	root, _, err := NewRoot(Dependencies{Store: securestore.NewMemory()})
	if err != nil {
		t.Fatal(err)
	}
	root.AddCommand(&cobra.Command{Use: "unclassified", Run: func(*cobra.Command, []string) {}})
	manifest := BuildCommandManifest(root)
	for _, command := range manifest.Commands {
		if command.Path == "unclassified" {
			if command.SideEffect != "" {
				t.Fatalf("unreviewed command silently received policy %q", command.SideEffect)
			}
			return
		}
	}
	t.Fatal("unclassified test command is missing from manifest")
}
