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

type skillRelease struct {
	SkillVersion          string `json:"skill_version"`
	MinimumCLIVersion     string `json:"minimum_cli_version"`
	CLICompatibility      string `json:"cli_compatibility"`
	FullBundleDigest      string `json:"full_skill_bundle_digest"`
	EmbeddedContentDigest string `json:"embedded_content_digest"`
}

type releaseManifest struct {
	SchemaVersion           int                     `json:"schema_version"`
	NPMPackage              string                  `json:"npm_package"`
	CLIVersion              string                  `json:"cli_version"`
	Skills                  map[string]skillRelease `json:"skills"`
	BootstrapContractDigest string                  `json:"bootstrap_contract_digest"`
	InstallerDigests        map[string]string       `json:"installer_digests"`
}

func main() {
	output := flag.String("output", "quality/release-manifest.json", "release manifest output path")
	flag.Parse()

	bundle := skillcontent.New(cliembed.EmbeddedSkills())
	skillNames := []string{"creator-tools", "become-a-creator", "sell-a-skill", "use-a-skill", "charge-for-your-work", "let-people-interact", "let-others-make-a-copy"}
	skills := make(map[string]skillRelease, len(skillNames))
	for _, name := range skillNames {
		digests, err := bundle.Digests(name)
		if err != nil {
			fatal(err)
		}
		metadata, err := bundle.Package(name)
		if err != nil {
			fatal(err)
		}
		skills[name] = skillRelease{
			SkillVersion: metadata.SkillVersion, MinimumCLIVersion: metadata.MinimumCLIVersion,
			CLICompatibility: metadata.CLICompatibility, FullBundleDigest: digests.Full,
			EmbeddedContentDigest: digests.Embedded,
		}
	}
	manifest := releaseManifest{
		SchemaVersion:           2,
		NPMPackage:              "@viceme-ai/cli",
		CLIVersion:              buildinfo.ReleaseVersion,
		Skills:                  skills,
		BootstrapContractDigest: digestFile("release/bootstrap-contract.json"),
		InstallerDigests: map[string]string{
			"install.sh":  digestFile("installers/install.sh"),
			"install.ps1": digestFile("installers/install.ps1"),
		},
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

func digestFile(filename string) string {
	data, err := os.ReadFile(filename)
	if err != nil {
		fatal(err)
	}
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
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
