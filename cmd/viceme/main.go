package main

import (
	"os"

	"github.com/ViceMe-AI/cli/internal/command"
)

func main() {
	os.Exit(command.Execute(os.Args[1:], command.Dependencies{}))
}
