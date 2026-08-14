package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var sha256Hex = regexp.MustCompile(`^[a-f0-9]{64}$`)
var stableReleaseVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

type sourceManifest struct {
	NPMPackage              string                     `json:"npm_package"`
	CLIVersion              string                     `json:"cli_version"`
	Skills                  map[string]json.RawMessage `json:"skills"`
	BootstrapContractDigest string                     `json:"bootstrap_contract_digest"`
	InstallerDigests        map[string]string          `json:"installer_digests"`
}

type artifact struct {
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	File      string `json:"file"`
	SHA256    string `json:"sha256"`
	Signature string `json:"signature"`
}

type signedManifest struct {
	SchemaVersion           int                        `json:"schema_version"`
	NPMPackage              string                     `json:"npm_package"`
	CLIVersion              string                     `json:"cli_version"`
	Skills                  map[string]json.RawMessage `json:"skills"`
	BootstrapContractDigest string                     `json:"bootstrap_contract_digest"`
	InstallerDigests        map[string]string          `json:"installer_digests"`
	Artifacts               []artifact                 `json:"artifacts"`
	Signature               signatureContract          `json:"signature"`
}

type signatureContract struct {
	Format              string `json:"format"`
	Bundle              string `json:"bundle"`
	CertificateIdentity string `json:"certificate_identity"`
	OIDCIssuer          string `json:"oidc_issuer"`
}

type target struct {
	os        string
	arch      string
	extension string
}

var targets = []target{
	{os: "darwin", arch: "amd64"},
	{os: "darwin", arch: "arm64"},
	{os: "linux", arch: "amd64"},
	{os: "linux", arch: "arm64"},
	{os: "windows", arch: "amd64", extension: ".exe"},
	{os: "windows", arch: "arm64", extension: ".exe"},
}

func main() {
	version := flag.String("version", "", "stable CLI version without v prefix")
	sourcePath := flag.String("source", "quality/release-manifest.json", "generated source manifest")
	distDir := flag.String("dist", "dist", "directory containing release binaries and checksums")
	outputPath := flag.String("output", "dist/agent-release-manifest.json", "signed manifest output")
	flag.Parse()
	if !stableReleaseVersion.MatchString(*version) {
		fatalManifest("version must be a stable semantic version")
	}
	manifest, err := buildManifest(*version, *sourcePath, *distDir)
	if err != nil {
		fatalManifest(err.Error())
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		fatalManifest(err.Error())
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(*outputPath, encoded, 0o644); err != nil {
		fatalManifest(err.Error())
	}
}

func buildManifest(version, sourcePath, distDir string) (signedManifest, error) {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return signedManifest{}, err
	}
	var source sourceManifest
	if err := json.Unmarshal(data, &source); err != nil {
		return signedManifest{}, err
	}
	if source.CLIVersion != version {
		return signedManifest{}, fmt.Errorf("source manifest version does not match the release")
	}
	artifacts := make([]artifact, 0, len(targets))
	for _, item := range targets {
		name := fmt.Sprintf("viceme_%s_%s_%s%s", version, item.os, item.arch, item.extension)
		digest, err := readChecksum(filepath.Join(distDir, name+".sha256"), name)
		if err != nil {
			return signedManifest{}, err
		}
		artifacts = append(artifacts, artifact{
			OS: item.os, Arch: item.arch, File: name, SHA256: "sha256:" + digest,
			Signature: "agent-release-manifest.sigstore.json",
		})
	}
	return signedManifest{
		SchemaVersion: 3, NPMPackage: source.NPMPackage, CLIVersion: source.CLIVersion,
		Skills: source.Skills, BootstrapContractDigest: source.BootstrapContractDigest,
		InstallerDigests: source.InstallerDigests, Artifacts: artifacts,
		Signature: signatureContract{
			Format: "sigstore-bundle-v0.3", Bundle: "agent-release-manifest.sigstore.json",
			CertificateIdentity: "https://github.com/ViceMe-AI/cli/.github/workflows/release.yml@refs/heads/main",
			OIDCIssuer:          "https://token.actions.githubusercontent.com",
		},
	}, nil
}

func readChecksum(filename, expectedName string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return "", fmt.Errorf("checksum file is empty: %s", filename)
	}
	parts := strings.Fields(scanner.Text())
	if len(parts) != 2 || !sha256Hex.MatchString(parts[0]) || parts[1] != expectedName || scanner.Scan() {
		return "", fmt.Errorf("checksum file is invalid: %s", filename)
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return parts[0], nil
}

func fatalManifest(message string) {
	_, _ = fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
