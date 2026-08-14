package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var stableVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

func main() {
	version := flag.String("version", "", "stable CLI version without v prefix")
	templatePath := flag.String("template", "release/agent-install.md.tmpl", "template path")
	outputPath := flag.String("output", "dist/agent-install.md", "output path")
	flag.Parse()
	if !stableVersion.MatchString(*version) {
		fatal("version must be a stable semantic version")
	}
	template, err := os.ReadFile(*templatePath)
	if err != nil {
		fatal(err.Error())
	}
	placeholder := []byte("{{VERSION}}")
	if !bytes.Contains(template, placeholder) {
		fatal("agent install template is missing the version placeholder")
	}
	data := bytes.ReplaceAll(template, placeholder, []byte(*version))
	if bytes.Contains(data, []byte("{{")) {
		fatal("agent install template contains an unresolved placeholder")
	}
	if err := os.MkdirAll(filepath.Dir(*outputPath), 0o755); err != nil {
		fatal(err.Error())
	}
	if err := os.WriteFile(*outputPath, data, 0o644); err != nil {
		fatal(err.Error())
	}
}

func fatal(message string) {
	_, _ = fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
