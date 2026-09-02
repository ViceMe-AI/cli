package replicacontent

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
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
		{name: "Windows parent alias", entries: []archiveEntry{{name: ".. /outside.html", content: []byte("x")}}},
		{name: "Windows trailing dot", entries: []archiveEntry{{name: "index.html.", content: []byte("x")}}},
		{name: "Windows device", entries: []archiveEntry{{name: "assets/NUL.txt", content: []byte("x")}}},
		{name: "Windows numbered device", entries: []archiveEntry{{name: "COM1.js", content: []byte("x")}}},
		{name: "Windows superscript device", entries: []archiveEntry{{name: "COM¹.js", content: []byte("x")}}},
		{name: "Windows extended device", entries: []archiveEntry{{name: "CONOUT$.log", content: []byte("x")}}},
		{name: "Windows spaced device", entries: []archiveEntry{{name: "CON .txt", content: []byte("x")}}},
		{name: "Windows NBSP device", entries: []archiveEntry{{name: "CON\u00a0.txt", content: []byte("x")}}},
		{name: "Windows BOM device", entries: []archiveEntry{{name: "CON\ufeff.txt", content: []byte("x")}}},
		{name: "Windows invalid character", entries: []archiveEntry{{name: "bad?.txt", content: []byte("x")}}},
		{name: "empty segment", entries: []archiveEntry{{name: "site//index.html", content: []byte("x")}}},
		{name: "backslash", entries: []archiveEntry{{name: `site\index.html`, content: []byte("x")}}},
		{name: "alternate data stream", entries: []archiveEntry{{name: "index.html:stream", content: []byte("x")}}},
		{name: "duplicate", entries: []archiveEntry{{name: "index.html", content: []byte("one")}, {name: "index.html", content: []byte("two")}}},
		{name: "case conflict", entries: []archiveEntry{{name: "Index.html", content: []byte("one")}, {name: "index.html", content: []byte("two")}}},
		{name: "file parent conflict", entries: []archiveEntry{{name: "site", content: []byte("file")}, {name: "site/index.html", content: []byte("child")}}},
		{name: "symlink", entries: []archiveEntry{{name: "link", content: []byte("target"), mode: os.ModeSymlink | 0o777}}},
		{name: "special file", entries: []archiveEntry{{name: "pipe", mode: os.ModeNamedPipe | 0o600}}},
		{name: "reserved license", entries: []archiveEntry{{name: LicenseFilePath, content: []byte("forged")}}},
		{name: "reserved namespace", entries: []archiveEntry{{name: ".viceme/other.json", content: []byte("forged")}}},
		{name: "reserved folded namespace", entries: []archiveEntry{{name: ".vıceme/other.json", content: []byte("forged")}}},
		{name: "non-NFC path", entries: []archiveEntry{{name: "cafe\u0301.txt", content: []byte("x")}}},
		{name: "Unicode fold conflict", entries: []archiveEntry{{name: "Ĥ̱.txt", content: []byte("one")}, {name: "ẖ̂.txt", content: []byte("two")}}},
		{name: "fold expansion", entries: []archiveEntry{{name: strings.Repeat("ﷺ", 80) + ".txt", content: []byte("x")}}},
		{name: "compression ratio", entries: []archiveEntry{{name: "bomb.txt", content: largeCompressible, method: zip.Deflate}}},
		{name: "path depth", entries: []archiveEntry{{name: strings.Repeat("directory/", MaxArchivePathDepth) + "index.html", content: []byte("x")}}},
		{name: "segment length", entries: []archiveEntry{{name: strings.Repeat("a", MaxArchiveSegmentBytes+1), content: []byte("x")}}},
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

func TestInstallArchiveRejectsEntryStormBeforeExtraction(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	archive := filepath.Join(root, "source.zip")
	entries := make([]archiveEntry, 0, MaxArchiveEntries+1)
	for index := 0; index <= MaxArchiveEntries; index++ {
		entries = append(entries, archiveEntry{
			name: fmt.Sprintf("directory-%05d/", index),
			mode: os.ModeDir | 0o755,
		})
	}
	writeArchive(t, archive, entries)
	target := filepath.Join(root, "installed")
	if _, err := InstallArchive(archive, target, testLicenseRecord()); err == nil {
		t.Fatal("entry storm was installed")
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("entry storm left a target: %v", err)
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

func TestInstallArchiveRejectsInconsistentZIPStructures(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	baseArchive := filepath.Join(root, "base.zip")
	writeArchive(t, baseArchive, []archiveEntry{{name: "source.txt", content: []byte("UNIQUE-CONTENT"), method: zip.Store}})
	base, err := os.ReadFile(baseArchive)
	if err != nil {
		t.Fatal(err)
	}
	baseLayout := inspectSingleEntryZIP(t, base)
	if baseLayout.descriptor < 0 {
		t.Fatal("test archive did not use a data descriptor")
	}

	tests := []struct {
		name   string
		mutate func([]byte, singleEntryZIPLayout)
	}{
		{
			name: "central directory size",
			mutate: func(data []byte, layout singleEntryZIPLayout) {
				size := binary.LittleEndian.Uint32(data[layout.endRecord+12:])
				binary.LittleEndian.PutUint32(data[layout.endRecord+12:], size-1)
			},
		},
		{
			name: "underreported entry count",
			mutate: func(data []byte, layout singleEntryZIPLayout) {
				binary.LittleEndian.PutUint16(data[layout.endRecord+8:], 0)
				binary.LittleEndian.PutUint16(data[layout.endRecord+10:], 0)
			},
		},
		{
			name: "local and central names",
			mutate: func(data []byte, layout singleEntryZIPLayout) {
				data[layout.localName] = 'S'
			},
		},
		{
			name: "local and central methods",
			mutate: func(data []byte, layout singleEntryZIPLayout) {
				binary.LittleEndian.PutUint16(data[layout.localHeader+8:], zip.Deflate)
			},
		},
		{
			name: "ZIP64 sentinel",
			mutate: func(data []byte, layout singleEntryZIPLayout) {
				binary.LittleEndian.PutUint32(data[layout.centralHeader+20:], 0xffffffff)
			},
		},
		{
			name: "data descriptor size",
			mutate: func(data []byte, layout singleEntryZIPLayout) {
				compressedOffset := layout.descriptor + 4
				if binary.LittleEndian.Uint32(data[layout.descriptor:]) == zipDescriptorSignature {
					compressedOffset += 4
				}
				binary.LittleEndian.PutUint32(data[compressedOffset:], layout.compressedSize+1)
			},
		},
		{
			name: "legacy encoded name",
			mutate: func(data []byte, layout singleEntryZIPLayout) {
				data[layout.localName] = 0xff
				data[layout.centralName] = 0xff
				binary.LittleEndian.PutUint16(data[layout.localHeader+6:], binary.LittleEndian.Uint16(data[layout.localHeader+6:])&^0x800)
				binary.LittleEndian.PutUint16(data[layout.centralHeader+8:], binary.LittleEndian.Uint16(data[layout.centralHeader+8:])&^0x800)
			},
		},
		{
			name: "zero checksum bypass",
			mutate: func(data []byte, layout singleEntryZIPLayout) {
				binary.LittleEndian.PutUint32(data[layout.centralHeader+16:], 0)
				if layout.descriptor >= 0 {
					checksumOffset := layout.descriptor
					if binary.LittleEndian.Uint32(data[layout.descriptor:]) == zipDescriptorSignature {
						checksumOffset += 4
					}
					binary.LittleEndian.PutUint32(data[checksumOffset:], 0)
				} else {
					binary.LittleEndian.PutUint32(data[layout.localHeader+14:], 0)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			data := append([]byte(nil), base...)
			test.mutate(data, baseLayout)
			assertArchiveBytesRejected(t, data)
		})
	}
}

func TestInstallArchiveRejectsDuplicateLocalHeaderOffsets(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	archive := filepath.Join(root, "source.zip")
	writeArchive(t, archive, []archiveEntry{
		{name: "first.txt", content: []byte("first")},
		{name: "second.txt", content: []byte("second")},
	})
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	endRecord := bytes.LastIndex(data, []byte{'P', 'K', 0x05, 0x06})
	if endRecord < 0 {
		t.Fatal("ZIP end record is missing")
	}
	firstCentral := int(binary.LittleEndian.Uint32(data[endRecord+16:]))
	secondCentral := nextCentralEntry(t, data, firstCentral)
	firstLocalOffset := binary.LittleEndian.Uint32(data[firstCentral+42:])
	binary.LittleEndian.PutUint32(data[secondCentral+42:], firstLocalOffset)
	assertArchiveBytesRejected(t, data)
}

func TestInstallArchiveRejectsMalformedDEFLATEData(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	archive := filepath.Join(root, "source.zip")
	content := make([]byte, 4096)
	for index := range content {
		content[index] = byte((index*31 + index/7) % 251)
	}
	writeArchive(t, archive, []archiveEntry{{name: "source.bin", content: content, method: zip.Deflate}})
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	layout := inspectSingleEntryZIP(t, data)
	if layout.compressedSize < 4 {
		t.Fatal("compressed test entry is unexpectedly small")
	}
	data[layout.fileData+int(layout.compressedSize)/2] ^= 0xff
	assertArchiveBytesRejected(t, data)
}

func TestZIPExtraFieldValidationRejectsZIP64AndTruncation(t *testing.T) {
	t.Parallel()
	for _, extra := range [][]byte{
		{0x01, 0x00, 0x00, 0x00},
		{0x55, 0x54, 0x02, 0x00, 0x01},
		{0x55, 0x54, 0x01},
	} {
		if err := validateZIPExtra(extra); err == nil {
			t.Fatalf("unsafe ZIP extra field was accepted: %x", extra)
		}
	}
}

func TestWindowsDeviceNameTrimmingMatchesECMAScript(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"CON .txt", "CON\u00a0.txt", "CON\ufeff.txt"} {
		if !isWindowsDeviceName(value) {
			t.Errorf("ECMAScript-trimmed Windows device name was accepted: %q", value)
		}
	}
	if isWindowsDeviceName("CON\u0085.txt") {
		t.Fatal("NEL was trimmed even though JavaScript trimEnd preserves it")
	}
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
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("installed source root is not private: mode=%v", info.Mode())
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
	recovered, err := InstallArchive(archive, target, testLicenseRecord())
	if err != nil || recovered != result {
		t.Fatalf("committed installation was not idempotently recovered: result=%#v err=%v", recovered, err)
	}
	journalPath := target + installJournalSuffix
	recoveryStage := filepath.Join(filepath.Dir(target), "."+filepath.Base(target)+installStageSuffix+"-recovery")
	recoveryNonce := strings.Repeat("1", 64)
	if err := os.WriteFile(filepath.Join(target, filepath.FromSlash(installOwnerPath)), []byte(recoveryNonce+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeJournal(journalPath, installJournal{
		SchemaVersion: 2,
		Status:        "ACTIVATING",
		Target:        target,
		Stage:         recoveryStage,
		Nonce:         recoveryNonce,
	}); err != nil {
		t.Fatal(err)
	}
	recovered, err = InstallArchive(archive, target, testLicenseRecord())
	if err != nil || recovered != result {
		t.Fatalf("post-rename crash was not recovered: result=%#v err=%v", recovered, err)
	}
	if _, err := os.Lstat(journalPath); !os.IsNotExist(err) {
		t.Fatalf("recovered install journal remains: %v", err)
	}
	assertNoInstallStaging(t, target)
}

func TestInstallArchiveDoesNotFollowARecoveryJournalSymlink(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, "source.zip")
	writeArchive(t, archive, []archiveEntry{{name: "index.html", content: []byte("source")}})
	target := filepath.Join(root, "installed")
	victim := filepath.Join(root, "victim.json")
	victimData := []byte(`{"owned":true}`)
	if err := os.WriteFile(victim, victimData, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, target+installJournalSuffix); err != nil {
		t.Skipf("symlink test is unavailable: %v", err)
	}
	if _, err := InstallArchive(archive, target, testLicenseRecord()); err == nil {
		t.Fatal("recovery journal symlink was followed")
	}
	if data, err := os.ReadFile(victim); err != nil || !bytes.Equal(data, victimData) {
		t.Fatalf("journal symlink target changed: data=%q err=%v", data, err)
	}
}

func TestInstallArchiveRefreshesAStableLicenseAfterSigningKeyRotation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	archive := filepath.Join(root, "source.zip")
	writeArchive(t, archive, []archiveEntry{{name: "index.html", content: []byte("source")}})
	target := filepath.Join(root, "installed")
	first := testLicenseRecord()
	first.License = json.RawMessage(`{"claims":{"replicaId":"replica","version":7},"signingKeyId":"k1","signature":"one"}`)
	if _, err := InstallArchive(archive, target, first); err != nil {
		t.Fatal(err)
	}
	rotated := first
	rotated.License = json.RawMessage(`{"claims":{"replicaId":"replica","version":7},"signingKeyId":"k2","signature":"two"}`)
	if _, err := InstallArchive(archive, target, rotated); err != nil {
		t.Fatalf("stable signed claims were not recovered after key rotation: %v", err)
	}
	record, err := ReadInstalledLicenseRecord(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(record.License, []byte(`"signingKeyId": "k2"`)) || bytes.Contains(record.License, []byte(`"signingKeyId": "k1"`)) {
		t.Fatalf("installed license was not refreshed: %s", record.License)
	}
}

func TestInstallArchiveRecoveryRejectsTamperedOrExtraTargetContent(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		tamper func(t *testing.T, target string)
	}{
		{
			name: "changed source",
			tamper: func(t *testing.T, target string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(target, "index.html"), []byte("tampered"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "extra source",
			tamper: func(t *testing.T, target string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(target, "extra.txt"), []byte("extra"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			archive := filepath.Join(root, "source.zip")
			writeArchive(t, archive, []archiveEntry{{name: "index.html", content: []byte("original")}})
			target := filepath.Join(root, "installed")
			if _, err := InstallArchive(archive, target, testLicenseRecord()); err != nil {
				t.Fatal(err)
			}
			test.tamper(t, target)
			if _, err := InstallArchive(archive, target, testLicenseRecord()); err == nil {
				t.Fatal("tampered committed target was accepted as an idempotent recovery")
			}
		})
	}
}

func TestInstallArchiveRecoversEveryDurabilityBoundary(t *testing.T) {
	const (
		helperEnvironment  = "VICEME_REPLICA_INSTALL_CRASH_HELPER"
		pointEnvironment   = "VICEME_REPLICA_INSTALL_CRASH_POINT"
		archiveEnvironment = "VICEME_REPLICA_INSTALL_CRASH_ARCHIVE"
		targetEnvironment  = "VICEME_REPLICA_INSTALL_CRASH_TARGET"
		crashExitCode      = 86
	)
	if os.Getenv(helperEnvironment) == "1" {
		point := os.Getenv(pointEnvironment)
		installTestCrashHook = func(current string) {
			if current == point {
				os.Exit(crashExitCode)
			}
		}
		if _, err := InstallArchive(
			os.Getenv(archiveEnvironment),
			os.Getenv(targetEnvironment),
			testLicenseRecord(),
		); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(87)
		}
		os.Exit(88)
	}

	points := []string{
		"preparing-journal-file-synced",
		"preparing-journal-renamed",
		"preparing-journal-directory-synced",
		"source-file-synced",
		"license-file-synced",
		"stage-directory-synced",
		"activating-journal-file-synced",
		"activating-journal-renamed",
		"activating-journal-directory-synced",
		"target-activated",
		"target-directory-synced",
		"stage-owner-removed",
		"journal-removed",
		"completion-directory-synced",
	}
	for _, point := range points {
		t.Run(point, func(t *testing.T) {
			root := t.TempDir()
			archive := filepath.Join(root, "source.zip")
			writeArchive(t, archive, []archiveEntry{
				{name: "index.html", content: []byte("source")},
				{name: "assets/app.js", content: []byte("script")},
			})
			target := filepath.Join(root, "installed")
			command := exec.Command(os.Args[0], "-test.run=^TestInstallArchiveRecoversEveryDurabilityBoundary$")
			command.Env = append(os.Environ(),
				helperEnvironment+"=1",
				pointEnvironment+"="+point,
				archiveEnvironment+"="+archive,
				targetEnvironment+"="+target,
			)
			output, err := command.CombinedOutput()
			var exitError *exec.ExitError
			if !errors.As(err, &exitError) || exitError.ExitCode() != crashExitCode {
				t.Fatalf("crash helper did not stop at %s: err=%v output=%s", point, err, output)
			}

			if _, err := InstallArchive(archive, target, testLicenseRecord()); err != nil {
				t.Fatalf("installation did not recover after %s: %v", point, err)
			}
			if content, err := os.ReadFile(filepath.Join(target, "index.html")); err != nil || string(content) != "source" {
				t.Fatalf("recovered source is invalid after %s: content=%q err=%v", point, content, err)
			}
			assertNoInstallStaging(t, target)
		})
	}
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

type singleEntryZIPLayout struct {
	endRecord      int
	centralHeader  int
	centralName    int
	localHeader    int
	localName      int
	fileData       int
	descriptor     int
	compressedSize uint32
}

func inspectSingleEntryZIP(t *testing.T, data []byte) singleEntryZIPLayout {
	t.Helper()
	endRecord := bytes.LastIndex(data, []byte{'P', 'K', 0x05, 0x06})
	if endRecord < 0 {
		t.Fatal("ZIP end record is missing")
	}
	centralHeader := int(binary.LittleEndian.Uint32(data[endRecord+16:]))
	if centralHeader < 0 || centralHeader+zipCentralHeaderSize > len(data) || binary.LittleEndian.Uint32(data[centralHeader:]) != zipCentralSignature {
		t.Fatal("ZIP central header is invalid")
	}
	localHeader := int(binary.LittleEndian.Uint32(data[centralHeader+42:]))
	if localHeader < 0 || localHeader+zipLocalHeaderSize > len(data) || binary.LittleEndian.Uint32(data[localHeader:]) != zipLocalSignature {
		t.Fatal("ZIP local header is invalid")
	}
	localNameLength := int(binary.LittleEndian.Uint16(data[localHeader+26:]))
	localExtraLength := int(binary.LittleEndian.Uint16(data[localHeader+28:]))
	compressedSize := binary.LittleEndian.Uint32(data[centralHeader+20:])
	fileData := localHeader + zipLocalHeaderSize + localNameLength + localExtraLength
	descriptor := -1
	if binary.LittleEndian.Uint16(data[centralHeader+8:])&0x8 != 0 {
		descriptor = fileData + int(compressedSize)
	}
	return singleEntryZIPLayout{
		endRecord:      endRecord,
		centralHeader:  centralHeader,
		centralName:    centralHeader + zipCentralHeaderSize,
		localHeader:    localHeader,
		localName:      localHeader + zipLocalHeaderSize,
		fileData:       fileData,
		descriptor:     descriptor,
		compressedSize: compressedSize,
	}
}

func nextCentralEntry(t *testing.T, data []byte, offset int) int {
	t.Helper()
	if offset < 0 || offset+zipCentralHeaderSize > len(data) || binary.LittleEndian.Uint32(data[offset:]) != zipCentralSignature {
		t.Fatal("ZIP central entry is invalid")
	}
	return offset + zipCentralHeaderSize + int(binary.LittleEndian.Uint16(data[offset+28:])) +
		int(binary.LittleEndian.Uint16(data[offset+30:])) + int(binary.LittleEndian.Uint16(data[offset+32:]))
}

func assertArchiveBytesRejected(t *testing.T, data []byte) {
	t.Helper()
	root := t.TempDir()
	archive := filepath.Join(root, "source.zip")
	if err := os.WriteFile(archive, data, 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "installed")
	if _, err := InstallArchive(archive, target, testLicenseRecord()); err == nil {
		t.Fatal("inconsistent ZIP archive was installed")
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected ZIP left an installation target: %v", err)
	}
	assertNoInstallStaging(t, target)
}

func assertNoInstallStaging(t *testing.T, target string) {
	t.Helper()
	parent := filepath.Dir(target)
	base := filepath.Base(target)
	for _, pattern := range []string{
		filepath.Join(parent, "."+base+installStageSuffix+"-*"),
		target + installJournalSuffix,
		filepath.Join(parent, "."+base+installJournalSuffix+".tmp-*"),
	} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 0 {
			t.Fatalf("install staging was not cleaned: %v", matches)
		}
	}
}
