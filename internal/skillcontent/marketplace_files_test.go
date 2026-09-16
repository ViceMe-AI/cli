package skillcontent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func marketplaceUpgradeFixture(t *testing.T) (Environment, SkillProvenance, string) {
	t.Helper()
	environment := provenanceTestEnvironment(t)
	provenance := SkillProvenance{ProductID: "product-1", ReleaseID: "release-1"}
	if report := provenanceTestBundle(t, "old generation").InstallWithProvenance(provenanceTestSkill, "agents", environment, provenance); !report.AllSucceeded {
		t.Fatalf("fixture: %+v", report)
	}
	return environment, provenance, filepath.Join(environment.Home, ".agents", "skills", provenanceTestSkill)
}

func writeLocalFixture(t *testing.T, directory, relative, content string) {
	t.Helper()
	filename := filepath.Join(directory, relative)
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMarketplaceInventoryRetainsOnlyUserFilesAndLinkEntries(t *testing.T) {
	environment, provenance, directory := marketplaceUpgradeFixture(t)
	writeLocalFixture(t, directory, "references/user-notes.md", "<<<<<<< This is user output, not authored package content\n")
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(directory, "external-link.md")); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	if err := os.Symlink("missing-target", filepath.Join(directory, "dangling-link")); err != nil {
		t.Fatal(err)
	}
	provenance.ReleaseID = "release-2"
	report := provenanceTestBundle(t, "new generation").InstallWithProvenance(provenanceTestSkill, "agents", environment, provenance)
	if !report.AllSucceeded || len(report.LocalRecoveries) != 0 {
		t.Fatalf("upgrade: %+v", report)
	}
	if data, err := os.ReadFile(filepath.Join(directory, "references/user-notes.md")); err != nil || !strings.HasPrefix(string(data), "<<<<<<<") {
		t.Fatalf("user output lost: %q %v", data, err)
	}
	if link, err := os.Readlink(filepath.Join(directory, "external-link.md")); err != nil || link != outside {
		t.Fatalf("link changed: %q %v", link, err)
	}
	if link, err := os.Readlink(filepath.Join(directory, "dangling-link")); err != nil || link != "missing-target" {
		t.Fatalf("dangling link changed: %q %v", link, err)
	}
	if data, err := os.ReadFile(outside); err != nil || string(data) != "outside original" {
		t.Fatalf("external target changed: %q %v", data, err)
	}
	inventory, valid := readPackageFiles(directory, provenance)
	if !valid || inventory.Files["references/user-notes.md"] != "" || inventory.Files["external-link.md"] != "" {
		t.Fatalf("user files became publisher-owned: %+v", inventory)
	}
	if !PackageInstallationComplete(directory, provenance.ProductID) {
		t.Fatal("user links or output blocked valid package readiness")
	}
	if err := os.Remove(outside); err != nil {
		t.Fatal(err)
	}
	if !PackageInstallationComplete(directory, provenance.ProductID) {
		t.Fatal("readiness followed an unowned external link")
	}
}

func TestLegacyMarketplaceMigrationPreservesCompleteVerifiedRecovery(t *testing.T) {
	for _, variant := range []string{"missing", "invalid", "foreign"} {
		t.Run(variant, func(t *testing.T) {
			environment, provenance, directory := marketplaceUpgradeFixture(t)
			filename := filepath.Join(directory, PackageFilesPath)
			switch variant {
			case "missing":
				if err := os.Remove(filename); err != nil {
					t.Fatal(err)
				}
			case "invalid":
				writeLocalFixture(t, directory, PackageFilesPath, `{"schemaVersion":1,"files":{"../outside":"invalid"}}`)
			case "foreign":
				inventory, _ := readPackageFiles(directory, provenance)
				inventory.ProductID = "another-product"
				data, _ := json.Marshal(inventory)
				writeLocalFixture(t, directory, PackageFilesPath, string(data))
			}
			writeLocalFixture(t, directory, "rules/unknown-authored.txt", "retired rule")
			writeLocalFixture(t, directory, "reports/user-output.txt", "important user output")
			before, err := recoveryTree(directory)
			if err != nil {
				t.Fatal(err)
			}
			provenance.ReleaseID = "release-2"
			report := provenanceTestBundle(t, "new generation").InstallWithProvenance(provenanceTestSkill, "agents", environment, provenance)
			if !report.AllSucceeded || len(report.LocalRecoveries) != 1 {
				t.Fatalf("migration: %+v", report)
			}
			recovery := report.LocalRecoveries[0]
			if !strings.HasPrefix(recovery.Directory, filepath.Join(environment.Home, ".viceme", "recovery")+string(filepath.Separator)) {
				t.Fatalf("recovery remains in host Skill tree: %+v", recovery)
			}
			if info, err := os.Stat(recovery.Directory); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o700) {
				t.Fatalf("recovery is not private: %v", err)
			}
			after, err := recoveryTree(filepath.Join(recovery.Directory, "files"))
			if err != nil || !reflect.DeepEqual(before, after) || len(recovery.Files) != len(before) {
				t.Fatalf("recovery differs from source: %v", err)
			}
			if _, err := os.Stat(filepath.Join(directory, "rules/unknown-authored.txt")); !os.IsNotExist(err) {
				t.Fatalf("unknown old resource still active: %v", err)
			}
			if _, err := os.Stat(filepath.Join(recovery.Directory, "recovery.json")); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(recovery.Directory, "recovery.json"))
			if err != nil {
				t.Fatal(err)
			}
			var saved struct {
				SkillDirectory string `json:"skillDirectory"`
			}
			if err := json.Unmarshal(data, &saved); err != nil {
				t.Fatal(err)
			}
			recordedDirectory, recordedErr := os.Stat(saved.SkillDirectory)
			originalDirectory, originalErr := os.Stat(directory)
			if recordedErr != nil || originalErr != nil || !os.SameFile(recordedDirectory, originalDirectory) {
				t.Fatalf("durable recovery lost its original directory: %+v recorded=%v original=%v", saved, recordedErr, originalErr)
			}
			if _, valid := readPackageFiles(directory, provenance); !valid {
				t.Fatal("new generation missing ownership inventory")
			}
		})
	}
}

func TestLegacyMarketplaceRecoveryRollsBackAndRecoversAfterCrash(t *testing.T) {
	for _, crash := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "startup-recovery"}[crash], func(t *testing.T) {
			environment, provenance, directory := marketplaceUpgradeFixture(t)
			if err := os.Remove(filepath.Join(directory, PackageFilesPath)); err != nil {
				t.Fatal(err)
			}
			writeLocalFixture(t, directory, "user-output.txt", "keep through rollback")
			before, err := recoveryTree(directory)
			if err != nil {
				t.Fatal(err)
			}
			provenance.ReleaseID = "release-2"
			transaction, reports, err := provenanceTestBundle(t, "new generation").PrepareInstallSetWithProvenance([]string{provenanceTestSkill}, "agents", environment, provenance)
			if err != nil || len(reports[0].LocalRecoveries) != 1 {
				t.Fatalf("prepare: %+v %v", reports, err)
			}
			recovery := reports[0].LocalRecoveries[0].Directory
			if crash {
				transaction.close()
				err = RecoverInstallTransaction(environment, false)
			} else {
				err = transaction.Rollback()
			}
			if err != nil {
				t.Fatal(err)
			}
			after, err := recoveryTree(directory)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("old generation not restored: %v", err)
			}
			if _, err := os.Lstat(recovery); !os.IsNotExist(err) {
				t.Fatalf("rolled-back recovery copy remains: %v", err)
			}
		})
	}
}

func TestPackageInventoryCompletionAllowsAuthoredEdits(t *testing.T) {
	_, provenance, directory := marketplaceUpgradeFixture(t)
	if !PackageInstallationComplete(directory, provenance.ProductID) {
		t.Fatal("complete package not ready")
	}
	writeLocalFixture(t, directory, "SKILL.md", "unfinished formal entry")
	if !PackageInstallationComplete(directory, provenance.ProductID) {
		t.Fatal("editing authored files changed the committed installation state")
	}
	if err := os.Remove(filepath.Join(directory, PackageFilesPath)); err != nil {
		t.Fatal(err)
	}
	if !PackageInstallationComplete(directory, provenance.ProductID) {
		t.Fatal("legacy readiness contract unexpectedly changed")
	}
}

func TestAuthoredPackageCannotClaimManagedInventory(t *testing.T) {
	root := t.TempDir()
	writeTestSkill(t, root, provenanceTestSkill)
	writeLocalFixture(t, filepath.Join(root, provenanceTestSkill), PackageFilesPath, `{"schemaVersion":1}`)
	report := New(os.DirFS(root)).InstallWithProvenance(provenanceTestSkill, "agents", provenanceTestEnvironment(t), SkillProvenance{ProductID: "product-1", ReleaseID: "release-1"})
	if report.AllSucceeded || !strings.Contains(report.Results[0].Error, "managed package file inventory") {
		t.Fatalf("authored inventory accepted: %+v", report)
	}
}

func TestMarketplaceUpgradeChecksUnownedFileCollisionsBeforeActivation(t *testing.T) {
	for _, variant := range []string{"different", "identical", "symlink"} {
		t.Run(variant, func(t *testing.T) {
			environment, provenance, directory := marketplaceUpgradeFixture(t)
			local := "user notes"
			if variant == "identical" {
				local = "publisher notes"
			}
			writeLocalFixture(t, directory, "references/new.txt", local)
			if variant == "symlink" {
				outside := filepath.Join(t.TempDir(), "same-content")
				if err := os.WriteFile(outside, []byte("publisher notes"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(filepath.Join(directory, "references/new.txt")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, filepath.Join(directory, "references/new.txt")); err != nil {
					t.Skipf("symbolic links unavailable: %v", err)
				}
			}
			before, err := recoveryTree(directory)
			if err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			writeTestSkill(t, root, provenanceTestSkill)
			writeLocalFixture(t, filepath.Join(root, provenanceTestSkill), "references/new.txt", "publisher notes")
			provenance.ReleaseID = "release-2"
			report := New(os.DirFS(root)).InstallWithProvenance(provenanceTestSkill, "agents", environment, provenance)
			if variant == "identical" {
				if !report.AllSucceeded {
					t.Fatalf("identical file not adopted: %+v", report)
				}
				inventory, valid := readPackageFiles(directory, provenance)
				if !valid || inventory.Files["references/new.txt"] == "" {
					t.Fatal("new resource was not recorded")
				}
			} else {
				if report.AllSucceeded {
					t.Fatal("unowned file overwritten")
				}
				after, err := recoveryTree(directory)
				if err != nil || !reflect.DeepEqual(before, after) {
					t.Fatalf("conflict changed original tree: %v", err)
				}
			}
		})
	}
}

func TestMarketplaceInventoryTransitionIsRepairableButNotReady(t *testing.T) {
	_, provenance, directory := marketplaceUpgradeFixture(t)
	inventory, valid := readPackageFiles(directory, provenance)
	if !valid {
		t.Fatal("missing initial inventory")
	}
	inventory.PreviousReleaseID, inventory.ReleaseID = provenance.ReleaseID, "release-2"
	data, _ := json.Marshal(inventory)
	writeLocalFixture(t, directory, PackageFilesPath, string(data))
	if _, valid := readPackageFiles(directory, provenance); !valid {
		t.Fatal("transition lost recoverable ownership")
	}
	if PackageInstallationComplete(directory, provenance.ProductID) {
		t.Fatal("uncommitted generation accepted by readiness")
	}
	if !PackageIdentityTransitionMatches(directory, provenance.ProductID, "release-1", "release-2") {
		t.Fatal("recorded metadata transition cannot be repaired")
	}
	if PackageIdentityTransitionMatches(directory, provenance.ProductID, "release-1", "unrelated-release") ||
		PackageIdentityTransitionMatches(directory, "other-product", "release-1", "release-2") {
		t.Fatal("unrelated identity was accepted as an interrupted transition")
	}
}

func TestMarketplaceSameReleaseInstallingMarkerIsNotReady(t *testing.T) {
	_, provenance, directory := marketplaceUpgradeFixture(t)
	inventory, valid := readPackageFiles(directory, provenance)
	if !valid {
		t.Fatal("missing inventory")
	}
	inventory.Installing = true
	data, _ := json.Marshal(inventory)
	writeLocalFixture(t, directory, PackageFilesPath, string(data))
	if _, valid := readPackageFiles(directory, provenance); !valid {
		t.Fatal("same-release transition cannot be repaired")
	}
	if PackageInstallationComplete(directory, provenance.ProductID) {
		t.Fatal("same-release uncommitted installation is ready")
	}
}

func TestMarketplaceResumesPythonLegacyInventoryAndReportsExistingRecovery(t *testing.T) {
	environment, provenance, directory := marketplaceUpgradeFixture(t)
	if err := os.Remove(filepath.Join(directory, PackageFilesPath)); err != nil {
		t.Fatal(err)
	}
	writeLocalFixture(t, directory, "user-output.txt", "saved before the interrupted migration")
	recovery := filepath.Join(environment.Home, ".viceme", "recovery", provenance.ProductID, "python-interrupted")
	if err := os.MkdirAll(recovery, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := copyTreeOnDisk(directory, filepath.Join(recovery, "files")); err != nil {
		t.Fatal(err)
	}
	snapshot, err := recoveryTree(directory)
	if err != nil {
		t.Fatal(err)
	}
	metadata, _ := json.Marshal(localRecoveryRecord{1, directory, provenance.ProductID, provenance.ReleaseID, snapshot})
	if err := os.WriteFile(filepath.Join(recovery, "recovery.json"), metadata, 0o600); err != nil {
		t.Fatal(err)
	}
	// Python can classify all old paths only after this full recovery copy has
	// been verified; its pending union keeps the original entry executable.
	if err := writePackageFiles(directory, provenance); err != nil {
		t.Fatal(err)
	}
	inventory, valid := readPackageFiles(directory, provenance)
	if !valid {
		t.Fatal("missing inventory")
	}
	inventory.RecoveryDirectory = recovery
	inventory.PreviousReleaseID, inventory.ReleaseID = provenance.ReleaseID, "release-2"
	data, _ := json.Marshal(inventory)
	writeLocalFixture(t, directory, PackageFilesPath, string(data))
	if PackageInstallationComplete(directory, provenance.ProductID) {
		t.Fatal("pending legacy migration is ready")
	}
	provenance.ReleaseID = "release-2"
	report := provenanceTestBundle(t, "new generation").InstallWithProvenance(provenanceTestSkill, "agents", environment, provenance)
	if !report.AllSucceeded || len(report.LocalRecoveries) != 1 || report.LocalRecoveries[0].Directory != recovery {
		t.Fatalf("original recovery was lost or duplicated: %+v", report)
	}
	if _, err := os.Stat(filepath.Join(directory, "user-output.txt")); !os.IsNotExist(err) {
		t.Fatalf("unclassified old paths remained live: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(recovery, "files", "user-output.txt")); err != nil || string(data) != "saved before the interrupted migration" {
		t.Fatalf("preserved output lost: %q %v", data, err)
	}
	if !PackageInstallationComplete(directory, provenance.ProductID) {
		t.Fatal("committed package remains unready")
	}
}

func TestMarketplaceInvalidPendingRecoveryPreservesInterruptedState(t *testing.T) {
	for _, failure := range []string{"snapshot", "installation-identity"} {
		t.Run(failure, func(t *testing.T) {
			environment, provenance, directory := marketplaceUpgradeFixture(t)
			if err := os.Remove(filepath.Join(directory, PackageFilesPath)); err != nil {
				t.Fatal(err)
			}
			writeLocalFixture(t, directory, "user-notes.txt", "original user notes")
			provenance.ReleaseID = "release-2"
			report := provenanceTestBundle(t, "new generation").InstallWithProvenance(provenanceTestSkill, "agents", environment, provenance)
			if !report.AllSucceeded || len(report.LocalRecoveries) != 1 {
				t.Fatalf("legacy fixture: %+v", report)
			}
			recovery := report.LocalRecoveries[0].Directory
			inventory, valid := readPackageFiles(directory, provenance)
			if !valid {
				t.Fatal("missing installed inventory")
			}
			inventory.RecoveryDirectory, inventory.Installing = recovery, true
			data, _ := json.Marshal(inventory)
			writeLocalFixture(t, directory, PackageFilesPath, string(data))
			if failure == "snapshot" {
				writeLocalFixture(t, recovery, "files/user-notes.txt", "incomplete recovery copy")
			} else {
				data, err := os.ReadFile(filepath.Join(recovery, "recovery.json"))
				if err != nil {
					t.Fatal(err)
				}
				var record localRecoveryRecord
				if err := json.Unmarshal(data, &record); err != nil {
					t.Fatal(err)
				}
				record.SkillDirectory = filepath.Join(environment.Home, "other-skill")
				data, _ = json.Marshal(record)
				writeLocalFixture(t, recovery, "recovery.json", string(data))
			}
			beforeSkill, err := recoveryTree(directory)
			if err != nil {
				t.Fatal(err)
			}
			beforeRecoveries, err := recoveryTree(filepath.Dir(recovery))
			if err != nil {
				t.Fatal(err)
			}
			provenance.ReleaseID = "release-3"
			report = provenanceTestBundle(t, "next generation").InstallWithProvenance(provenanceTestSkill, "agents", environment, provenance)
			if report.AllSucceeded || len(report.LocalRecoveries) != 0 {
				t.Fatalf("invalid recovery was replaced by a successful partial backup: %+v", report)
			}
			afterSkill, err := recoveryTree(directory)
			if err != nil || !reflect.DeepEqual(beforeSkill, afterSkill) {
				t.Fatalf("failed recovery changed the interrupted installation: %v", err)
			}
			afterRecoveries, err := recoveryTree(filepath.Dir(recovery))
			if err != nil || !reflect.DeepEqual(beforeRecoveries, afterRecoveries) {
				t.Fatalf("failed recovery replaced or duplicated the original backup: %v", err)
			}
		})
	}
}

func TestMarketplaceRecoveryAcceptsSameDirectoryCaseAliases(t *testing.T) {
	for _, aliasKind := range []string{"skill-directory", "home"} {
		t.Run(aliasKind, func(t *testing.T) {
			environment, provenance, directory := marketplaceUpgradeFixture(t)
			if err := os.Remove(filepath.Join(directory, PackageFilesPath)); err != nil {
				t.Fatal(err)
			}
			writeLocalFixture(t, directory, "user-notes.txt", "do not discard")
			provenance.ReleaseID = "release-2"
			report := provenanceTestBundle(t, "new generation").InstallWithProvenance(provenanceTestSkill, "agents", environment, provenance)
			if !report.AllSucceeded || len(report.LocalRecoveries) != 1 {
				t.Fatalf("legacy fixture: %+v", report)
			}
			recovery := report.LocalRecoveries[0].Directory
			inventory, valid := readPackageFiles(directory, provenance)
			if !valid {
				t.Fatal("missing inventory")
			}
			inventory.Installing, inventory.RecoveryDirectory = true, recovery
			data, _ := json.Marshal(inventory)
			writeLocalFixture(t, directory, PackageFilesPath, string(data))
			environment.InstallDirectory = directory
			original := directory
			if aliasKind == "home" {
				original = environment.Home
			}
			alias := strings.ToUpper(original)
			originalInfo, originalErr := os.Stat(original)
			aliasInfo, aliasErr := os.Stat(alias)
			if originalErr != nil || aliasErr != nil || !os.SameFile(originalInfo, aliasInfo) {
				t.Skip("filesystem does not support this case alias")
			}
			if aliasKind == "home" {
				environment.Home = alias
			} else {
				environment.InstallDirectory = alias
			}
			provenance.ReleaseID = "release-3"
			report = provenanceTestBundle(t, "next generation").InstallWithProvenance(provenanceTestSkill, "agents", environment, provenance)
			if !report.AllSucceeded || len(report.LocalRecoveries) != 1 || report.LocalRecoveries[0].Directory != recovery {
				t.Fatalf("same filesystem installation was rejected or lost recovery through its case alias: %+v", report)
			}
			if data, err := os.ReadFile(filepath.Join(recovery, "files", "user-notes.txt")); err != nil || string(data) != "do not discard" {
				t.Fatalf("case alias recovery lost original output: %q %v", data, err)
			}
		})
	}
}
