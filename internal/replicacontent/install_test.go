package replicacontent

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type archiveEntry struct {
	name    string
	content []byte
	mode    os.FileMode
	method  uint16
}

func TestInstallArchiveRejectsUnsafeEntries(t *testing.T) {
	t.Parallel()
	largeCompressible := bytes.Repeat([]byte("x"), 20_000)
	tests := []struct {
		name    string
		entries []archiveEntry
	}{
		{name: "parent traversal", entries: []archiveEntry{{name: "../outside.html", content: []byte("x")}}},
		{name: "absolute", entries: []archiveEntry{{name: "/absolute.html", content: []byte("x")}}},
		{name: "UNC", entries: []archiveEntry{{name: "//server/share.html", content: []byte("x")}}},
		{name: "Windows drive", entries: []archiveEntry{{name: "C:/windows.html", content: []byte("x")}}},
		{name: "dot segment", entries: []archiveEntry{{name: "site/./index.html", content: []byte("x")}}},
		{name: "dot dot segment", entries: []archiveEntry{{name: "site/../index.html", content: []byte("x")}}},
		{name: "empty segment", entries: []archiveEntry{{name: "site//index.html", content: []byte("x")}}},
		{name: "backslash", entries: []archiveEntry{{name: `site\index.html`, content: []byte("x")}}},
		{name: "alternate data stream", entries: []archiveEntry{{name: "index.html:stream", content: []byte("x")}}},
		{name: "duplicate", entries: []archiveEntry{{name: "index.html", content: []byte("one")}, {name: "index.html", content: []byte("two")}}},
		{name: "case conflict", entries: []archiveEntry{{name: "Index.html", content: []byte("one")}, {name: "index.html", content: []byte("two")}}},
		{name: "file parent conflict", entries: []archiveEntry{{name: "site", content: []byte("file")}, {name: "site/index.html", content: []byte("child")}}},
		{name: "symlink", entries: []archiveEntry{{name: "link", content: []byte("target"), mode: os.ModeSymlink | 0o777}}},
		{name: "special file", entries: []archiveEntry{{name: "pipe", mode: os.ModeNamedPipe | 0o600}}},
		{name: "reserved license", entries: []archiveEntry{{name: LicenseFilePath, content: []byte("forged")}}},
		{name: "compression ratio", entries: []archiveEntry{{name: "bomb.txt", content: largeCompressible, method: zip.Deflate}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			archive := filepath.Join(root, "source.zip")
			writeArchive(t, archive, test.entries)
			target := filepath.Join(root, "installed")
			if _, err := InstallArchive(archive, target, testLicenseRecord()); err == nil {
				t.Fatal("unsafe archive was installed")
			}
			if _, err := os.Lstat(target); !os.IsNotExist(err) {
				t.Fatalf("unsafe install left a target: %v", err)
			}
			assertNoInstallStaging(t, target)
		})
	}
}

func TestInstallArchiveRefusesExistingTargetWithoutChangingIt(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	archive := filepath.Join(root, "source.zip")
	writeArchive(t, archive, []archiveEntry{{name: "index.html", content: []byte("new")}})
	target := filepath.Join(root, "installed")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(target, "keep.txt")
	if err := os.WriteFile(marker, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallArchive(archive, target, testLicenseRecord()); err == nil {
		t.Fatal("existing target was overwritten")
	}
	if content, err := os.ReadFile(marker); err != nil || string(content) != "original" {
		t.Fatalf("existing target changed: content=%q err=%v", content, err)
	}
	assertNoInstallStaging(t, target)
}

func TestInstallArchiveCleansStagingAfterExtractionFailure(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	archive := filepath.Join(root, "source.zip")
	writeArchive(t, archive, []archiveEntry{{name: "index.html", content: []byte("UNIQUE-CONTENT"), method: zip.Store}})
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	index := bytes.Index(data, []byte("UNIQUE-CONTENT"))
	if index < 0 {
		t.Fatal("could not locate stored ZIP payload")
	}
	data[index] ^= 0xff
	if err := os.WriteFile(archive, data, 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "installed")
	if _, err := InstallArchive(archive, target, testLicenseRecord()); err == nil {
		t.Fatal("corrupt archive was installed")
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("failed extraction left a target: %v", err)
	}
	assertNoInstallStaging(t, target)
}

func TestInstallArchiveAtomicallyWritesSourceAndSignedLicense(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	archive := filepath.Join(root, "source.zip")
	writeArchive(t, archive, []archiveEntry{
		{name: "assets/", mode: os.ModeDir | 0o755},
		{name: "index.html", content: []byte("<h1>Replica</h1>")},
		{name: "assets/app.js", content: []byte("console.log('replica')"), mode: 0o755},
	})
	target := filepath.Join(root, "installed")
	result, err := InstallArchive(archive, target, testLicenseRecord())
	if err != nil {
		t.Fatal(err)
	}
	if result.Target != target || result.FileCount != 2 || result.ExpandedBytes == 0 || result.LicensePath != filepath.Join(target, filepath.FromSlash(LicenseFilePath)) {
		t.Fatalf("unexpected install result: %#v", result)
	}
	if content, err := os.ReadFile(filepath.Join(target, "index.html")); err != nil || string(content) != "<h1>Replica</h1>" {
		t.Fatalf("source was not installed: content=%q err=%v", content, err)
	}
	licenseData, err := os.ReadFile(result.LicensePath)
	if err != nil {
		t.Fatal(err)
	}
	var license struct {
		SchemaVersion  int             `json:"schemaVersion"`
		ReplicaID      string          `json:"replicaId"`
		VersionID      string          `json:"versionId"`
		Version        int             `json:"version"`
		ArtifactDigest string          `json:"artifactDigest"`
		License        json.RawMessage `json:"license"`
	}
	if err := json.Unmarshal(licenseData, &license); err != nil {
		t.Fatal(err)
	}
	var gotLicense, wantLicense any
	gotLicenseErr := json.Unmarshal(license.License, &gotLicense)
	wantLicenseErr := json.Unmarshal(testLicenseRecord().License, &wantLicense)
	if license.SchemaVersion != 1 || license.ReplicaID != testLicenseRecord().ReplicaID || license.VersionID != testLicenseRecord().VersionID || license.Version != 7 || license.ArtifactDigest != strings.Repeat("a", 64) || gotLicenseErr != nil || wantLicenseErr != nil || !reflect.DeepEqual(gotLicense, wantLicense) {
		t.Fatalf("license metadata changed: %s", licenseData)
	}
	assertNoInstallStaging(t, target)
}

func testLicenseRecord() LicenseRecord {
	return LicenseRecord{
		ReplicaID: "11111111-1111-4111-8111-111111111111", VersionID: "22222222-2222-4222-8222-222222222222",
		Version: 7, ArtifactDigest: strings.Repeat("a", 64),
		License: json.RawMessage(`{"payload":{"buyer":"licensed"},"signature":"signed-license-value"}`),
	}
}

func writeArchive(t *testing.T, filename string, entries []archiveEntry) {
	t.Helper()
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: entry.method}
		if header.Method == 0 {
			header.Method = zip.Store
		}
		mode := entry.mode
		if mode == 0 {
			mode = 0o644
		}
		header.SetMode(mode)
		output, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := output.Write(entry.content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertNoInstallStaging(t *testing.T, target string) {
	t.Helper()
	for _, pattern := range []string{target + ".viceme-replica-stage*", target + ".viceme-replica-install.json"} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 0 {
			t.Fatalf("install staging was not cleaned: %v", matches)
		}
	}
}
