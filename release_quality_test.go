package cli_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	cliembed "github.com/ViceMe-AI/cli"
	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/semver"
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

type npmPackage struct {
	Name    string            `json:"name"`
	Version string            `json:"version"`
	Bin     map[string]string `json:"bin"`
	Scripts map[string]string `json:"scripts"`
}

func TestReleaseManifestPinsVersionsCompatibilityAndDigests(t *testing.T) {
	t.Parallel()
	bundle := skillcontent.New(cliembed.EmbeddedSkills())
	digests, err := bundle.Digests("viceme")
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := bundle.Package("viceme")
	if err != nil {
		t.Fatal(err)
	}
	commandManifest, err := os.ReadFile("skills/viceme/references/command-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	commandDigest := sha256.Sum256(commandManifest)
	actual := releaseManifest{
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
	data, err := os.ReadFile("quality/release-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var expected releaseManifest
	if err := json.Unmarshal(data, &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) {
		encoded, _ := json.MarshalIndent(actual, "", "  ")
		t.Fatalf("release manifest drifted:\n%s\n", encoded)
	}
	if buildinfo.SkillVersion != metadata.SkillVersion || buildinfo.MinimumCLIVersion != metadata.MinimumCLIVersion || buildinfo.CLICompatibility != metadata.CLICompatibility {
		t.Fatal("Go build metadata and skill-package.json are not aligned")
	}
	compatible, err := semver.Satisfies(buildinfo.ReleaseVersion, metadata.CLICompatibility)
	if err != nil || !compatible {
		t.Fatalf("release CLI does not satisfy bundled Skill compatibility: compatible=%t err=%v", compatible, err)
	}
}

func TestNPMPackageMatchesReleaseAndHasNoInstallLifecycleScript(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("package.json")
	if err != nil {
		t.Fatal(err)
	}
	var document npmPackage
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if document.Name != "@viceme-ai/cli" || document.Version != buildinfo.ReleaseVersion {
		t.Fatalf("npm identity/version drift: %#v", document)
	}
	if document.Bin["viceme"] != "npm/bin/viceme.mjs" {
		t.Fatalf("npm package does not expose the launcher: %#v", document.Bin)
	}
	for _, lifecycle := range []string{"preinstall", "install", "postinstall"} {
		if document.Scripts[lifecycle] != "" {
			t.Fatalf("npm package must not execute lifecycle script %q", lifecycle)
		}
	}
}
