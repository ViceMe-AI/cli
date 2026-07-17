package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ViceMe-AI/cli/internal/command"
	"github.com/ViceMe-AI/cli/internal/securestore"
)

func main() {
	root, _, err := command.NewRoot(command.Dependencies{Store: securestore.NewMemory()})
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(command.BuildCommandManifest(root)); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
