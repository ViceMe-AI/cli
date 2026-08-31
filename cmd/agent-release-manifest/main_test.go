package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildManifestIncludesEveryPlatformDigestAndSignatureContract(t *testing.T) {
	directory := t.TempDir()
	source := sourceManifest{
		NPMPackage: "@viceme-ai/cli", CLIVersion: "1.2.3",
		Skills:                  map[string]json.RawMessage{"viceme-paid-skill": json.RawMessage(`{"skill_version":"1.2.3"}`)},
		BootstrapContractDigest: "sha256:" + strings.Repeat("b", 64),
		InstallerDigests:        map[string]string{"install.sh": "sha256:" + strings.Repeat("c", 64)},
	}
	data, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(directory, "source.json")
	if err := os.WriteFile(sourcePath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, item := range targets {
		name := "viceme_1.2.3_" + item.os + "_" + item.arch + item.extension
		if err := os.WriteFile(filepath.Join(directory, name+".sha256"), []byte(strings.Repeat("a", 64)+"  "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	manifest, err := buildManifest("1.2.3", sourcePath, directory)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != 3 || len(manifest.Artifacts) != 6 || manifest.Signature.Bundle != "agent-release-manifest.sigstore.json" {
		t.Fatalf("unexpected signed manifest: %#v", manifest)
	}
}

func TestReadChecksumRequiresExactAssetName(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "asset.sha256")
	digest := strings.Repeat("a", 64)
	if err := os.WriteFile(filename, []byte(digest+"  asset\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if actual, err := readChecksum(filename, "asset"); err != nil || actual != digest {
		t.Fatalf("valid checksum was rejected: digest=%q err=%v", actual, err)
	}
	if _, err := readChecksum(filename, "other"); err == nil {
		t.Fatal("checksum for another asset was accepted")
	}
}
