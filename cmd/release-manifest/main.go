package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	cliembed "github.com/ViceMe-AI/cli"
	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

type releaseManifest struct {
	SchemaVersion         int    `json:"schema_version"`
	NPMPackage            string `json:"npm_package"`
	CLIVersion            string `json:"cli_version"`
	SkillVersion          string `json:"skill_version"`
	MinimumCLIVersion     string `json:"minimum_cli_version"`
	CLICompatibility      string `json:"cli_compatibility"`
	FullBundleDigest      string `json:"full_skill_bundle_digest"`
	EmbeddedContentDigest string `json:"embedded_content_digest"`
	CommandManifestDigest string `json:"command_manifest_digest"`
}

func main() {
	output := flag.String("output", "quality/release-manifest.json", "release manifest output path")
	flag.Parse()

	bundle := skillcontent.New(cliembed.EmbeddedSkills())
	digests, err := bundle.Digests("viceme")
	if err != nil {
		fatal(err)
	}
	metadata, err := bundle.Package("viceme")
	if err != nil {
		fatal(err)
	}
	commandManifest, err := os.ReadFile("skills/viceme/references/command-manifest.json")
	if err != nil {
		fatal(err)
	}
	commandDigest := sha256.Sum256(commandManifest)
	manifest := releaseManifest{
		SchemaVersion:         1,
		NPMPackage:            "@viceme-ai/cli",
		CLIVersion:            buildinfo.ReleaseVersion,
		SkillVersion:          metadata.SkillVersion,
		MinimumCLIVersion:     metadata.MinimumCLIVersion,
		CLICompatibility:      metadata.CLICompatibility,
		FullBundleDigest:      digests.Full,
		EmbeddedContentDigest: digests.Embedded,
		CommandManifestDigest: "sha256:" + hex.EncodeToString(commandDigest[:]),
	}
	var data bytes.Buffer
	encoder := json.NewEncoder(&data)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(manifest); err != nil {
		fatal(err)
	}
	if err := writeAtomic(*output, data.Bytes()); err != nil {
		fatal(err)
	}
}

func writeAtomic(filename string, data []byte) error {
	directory := filepath.Dir(filename)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".release-manifest-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, filename)
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
