package command

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestCommandSurface(t *testing.T) {
	t.Parallel()
	root, _, err := NewRoot(Dependencies{Store: securestore.NewMemory(), Region: config.RegionCN})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"version", "install", "update", "auth login", "auth status", "auth logout",
		"skill inspect", "skill publish", "skill target get", "skill target list",
		"job get", "job wait", "job resume", "job cancel",
		"skills list", "skills read", "skills install", "skills doctor",
	} {
		if findCommand(root, strings.Fields(path)) == nil {
			t.Errorf("missing command %q", path)
		}
	}
}

func TestPublicConfigurationSurfaceStaysMinimal(t *testing.T) {
	t.Parallel()
	root, _, err := NewRoot(Dependencies{Store: securestore.NewMemory(), Region: config.RegionCN})
	if err != nil {
		t.Fatal(err)
	}
	for _, removed := range []string{"json", "api-base-url", "profile"} {
		if root.PersistentFlags().Lookup(removed) != nil {
			t.Errorf("unexpected public global flag --%s", removed)
		}
	}
	install := findCommand(root, []string{"install"})
	if install == nil || install.Flags().Lookup("region") == nil {
		t.Fatal("install command must expose the single region selector")
	}
}

func TestCommandExamplesReferenceRealFlagsAndConfirmRisk(t *testing.T) {
	t.Parallel()
	root, _, err := NewRoot(Dependencies{Store: securestore.NewMemory(), Region: config.RegionCN})
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := repositoryRoot(t)
	var examples []string
	err = filepath.WalkDir(filepath.Join(repoRoot, "skills"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		inFence := false
		pending := ""
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "```") {
				inFence = !inFence
				continue
			}
			if !inFence {
				continue
			}
			if pending != "" {
				pending += " " + strings.TrimSuffix(line, "\\")
				if !strings.HasSuffix(line, "\\") {
					examples = append(examples, strings.Join(strings.Fields(pending), " "))
					pending = ""
				}
				continue
			}
			if !strings.HasPrefix(line, "viceme ") {
				continue
			}
			if strings.HasSuffix(line, "\\") {
				pending = strings.TrimSuffix(line, "\\")
			} else {
				examples = append(examples, line)
			}
		}
		return scanner.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(examples) < 15 {
		t.Fatalf("expected substantial command coverage, got %d examples", len(examples))
	}
	for _, example := range examples {
		validateExample(t, root, example)
	}
}

func validateExample(t *testing.T, root *cobra.Command, example string) {
	t.Helper()
	tokens := strings.Fields(example)
	if len(tokens) < 2 || tokens[0] != "viceme" {
		t.Fatalf("invalid example: %s", example)
	}
	command, _, err := root.Find(tokens[1:])
	if err != nil || command == nil {
		t.Errorf("example references unknown command: %s (%v)", example, err)
		return
	}
	flags := allFlags(command)
	for _, token := range tokens[1:] {
		if !strings.HasPrefix(token, "--") {
			continue
		}
		name := strings.TrimPrefix(strings.SplitN(token, "=", 2)[0], "--")
		if !flags[name] {
			t.Errorf("example uses unknown flag --%s for %s: %s", name, command.CommandPath(), example)
		}
	}
	if (strings.HasPrefix(example, "viceme skill publish ") || strings.HasPrefix(example, "viceme job cancel ")) && !strings.Contains(example, "--yes") {
		t.Errorf("high-risk example lacks --yes: %s", example)
	}
}

func allFlags(command *cobra.Command) map[string]bool {
	result := make(map[string]bool)
	for current := command; current != nil; current = current.Parent() {
		current.LocalNonPersistentFlags().VisitAll(func(flag *pflag.Flag) { result[flag.Name] = true })
		current.PersistentFlags().VisitAll(func(flag *pflag.Flag) { result[flag.Name] = true })
	}
	return result
}

func findCommand(root *cobra.Command, parts []string) *cobra.Command {
	current := root
	for _, part := range parts {
		found := false
		for _, child := range current.Commands() {
			if child.Name() == part {
				current = child
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return current
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "../.."))
}
