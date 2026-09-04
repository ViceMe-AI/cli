package command

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	cliembed "github.com/ViceMe-AI/cli"
	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	updatepkg "github.com/ViceMe-AI/cli/internal/update"
	"github.com/spf13/cobra"
)

func TestBootstrapCoalescesAnAlreadyCompleteStandaloneGeneration(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	environment := skillcontent.Environment{Home: root, ConfigDir: configDir}
	skills := skillcontent.New(cliembed.EmbeddedSkills())
	for _, report := range skills.InstallSet(officialSkillNames, "agents", environment) {
		if !report.AllSucceeded {
			t.Fatalf("prepare complete official Skill generation: %#v", report)
		}
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "bin", "viceme")
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyBootstrapExecutable(executable, destination); err != nil {
		t.Fatal(err)
	}
	targetHash, err := bootstrapFileHash(destination)
	if err != nil {
		t.Fatal(err)
	}
	target, err := updatepkg.NewStandaloneGeneration(buildinfo.CompatibilityVersion(), targetHash)
	if err != nil {
		t.Fatal(err)
	}
	if err := updatepkg.CommitActiveGeneration(configDir, target); err != nil {
		t.Fatal(err)
	}
	configured := config.Default(config.RegionCN)
	if _, err := config.Save(configDir, configured); err != nil {
		t.Fatal(err)
	}
	runtime := &Runtime{
		configBase: configDir,
		region:     config.RegionCN,
		config:     configured,
		deps: Dependencies{
			Skills:      skills,
			Store:       securestore.NewMemory(),
			Environment: environment,
		},
	}
	command := &cobra.Command{}
	command.SetContext(context.Background())
	if bootstrapGenerationIsComplete(runtime, destination, targetHash, "agents", "global", true) {
		t.Fatal("same-version bootstrap to another region was incorrectly coalesced")
	}
	result, err := activateBootstrap(command, runtime, destination, "agents", "cn", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Destination != destination || len(result.Install.Skills) != 0 {
		t.Fatalf("already active generation was not coalesced: %#v", result)
	}
	persisted, err := config.LoadOrDefault(configDir)
	if err != nil || persisted.DistributionRegion != config.RegionCN {
		t.Fatalf("coalesced activation changed config: config=%#v err=%v", persisted, err)
	}
	if _, err := os.Stat(filepath.Join(configDir, bootstrapActivationJournalFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("coalesced activation created a recovery journal: %v", err)
	}
}

func TestBootstrapCLIOnlyActivationDoesNotWriteSkillsOrConfig(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	environment := skillcontent.Environment{Home: root, ConfigDir: configDir}
	destination := filepath.Join(root, "bin", "viceme")
	writeBootstrapTestFile(t, destination, "previous-binary")
	runtime := &Runtime{
		configBase: configDir,
		region:     config.RegionCN,
		config:     config.Default(config.RegionCN),
		deps: Dependencies{
			Skills:      skillcontent.New(cliembed.EmbeddedSkills()),
			Store:       securestore.NewMemory(),
			Environment: environment,
		},
	}
	command := &cobra.Command{}
	command.SetContext(context.Background())
	result, err := activateBootstrap(command, runtime, destination, "agents", "cn", true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Destination != destination || len(result.Install.Skills) != 0 {
		t.Fatalf("CLI-only bootstrap result=%#v", result)
	}
	if _, err := os.Stat(config.ConfigPath(configDir)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("CLI-only bootstrap wrote profile config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", "skills")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("CLI-only bootstrap wrote official Skills: %v", err)
	}
	active, exists, err := updatepkg.ReadActiveGeneration(configDir)
	if err != nil || !exists || active.InstallMethod != "standalone" {
		t.Fatalf("CLI-only bootstrap did not commit the executable generation: active=%#v exists=%t err=%v", active, exists, err)
	}
}

func TestConcurrentBootstrapActivationsCommitOneStandaloneGeneration(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	environment := skillcontent.Environment{Home: root, ConfigDir: configDir}
	destination := filepath.Join(root, "bin", "viceme")
	writeBootstrapTestFile(t, destination, "previous-binary")
	health := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":"ok"}`))
	}))
	defer health.Close()

	configured := config.Default(config.RegionCN)
	if err := configured.SetProfileAuthority(config.DefaultProfileName, health.URL, health.URL, config.RegionCN); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Save(configDir, configured); err != nil {
		t.Fatal(err)
	}
	newRuntime := func() *Runtime {
		copyOfConfig := configured
		resolved, err := copyOfConfig.Resolve("")
		if err != nil {
			t.Fatal(err)
		}
		return &Runtime{
			configBase: configDir,
			region:     config.RegionCN,
			apiBaseURL: health.URL,
			config:     copyOfConfig,
			profile:    *resolved,
			deps: Dependencies{
				HTTPClient:  health.Client(),
				Skills:      skillcontent.New(cliembed.EmbeddedSkills()),
				Store:       securestore.NewMemory(),
				Updater:     updatepkg.NewReleaseService(buildinfo.Version, buildinfo.CompatibilityVersion()),
				Environment: environment,
			},
		}
	}
	type outcome struct {
		result bootstrapActivationResult
		err    error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for _, runtime := range []*Runtime{newRuntime(), newRuntime()} {
		runtime := runtime
		go func() {
			command := &cobra.Command{}
			command.SetContext(context.Background())
			ready.Done()
			<-start
			result, err := activateBootstrap(command, runtime, destination, "agents", "cn", false)
			outcomes <- outcome{result: result, err: err}
		}()
	}
	ready.Wait()
	close(start)
	first := <-outcomes
	second := <-outcomes
	if first.err != nil || second.err != nil {
		t.Fatalf("concurrent bootstrap outcomes: first=%v second=%v", first.err, second.err)
	}
	fullActivations := 0
	for _, result := range []bootstrapActivationResult{first.result, second.result} {
		if len(result.Install.Skills) == len(officialSkillNames) {
			fullActivations++
		} else if len(result.Install.Skills) != 0 {
			t.Fatalf("partial bootstrap activation result: %#v", result)
		}
	}
	if fullActivations != 1 {
		t.Fatalf("concurrent bootstrap performed %d real activations: first=%#v second=%#v", fullActivations, first.result, second.result)
	}
	active, exists, err := updatepkg.ReadActiveGeneration(configDir)
	if err != nil || !exists || active.Version != buildinfo.CompatibilityVersion() || active.InstallMethod != "standalone" {
		t.Fatalf("concurrent bootstrap did not commit the target: active=%#v exists=%t err=%v", active, exists, err)
	}
}

func TestBootstrapRecoveryKeepsBinarySkillsAndConfigOnOneGeneration(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name       string
		status     string
		wantBinary string
		wantSkill  string
		wantConfig string
	}{
		{name: "preparing rolls back everything", status: "PREPARING", wantBinary: "old-binary", wantSkill: "old-skill", wantConfig: "old-config"},
		{name: "committing rolls forward everything", status: "COMMITTING", wantBinary: "new-binary", wantSkill: "new-skill", wantConfig: "new-config"},
	} {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			configDir := filepath.Join(root, "config")
			if err := os.MkdirAll(configDir, 0o700); err != nil {
				t.Fatal(err)
			}
			destination := filepath.Join(root, "bin", "viceme")
			staged := filepath.Join(configDir, bootstrapActivationStagedFilename)
			backup := filepath.Join(configDir, bootstrapActivationBackupFilename)
			writeBootstrapTestFile(t, destination, "old-binary")
			writeBootstrapTestFile(t, staged, "new-binary")
			writeBootstrapTestFile(t, backup, "old-binary")
			targetHash, err := bootstrapFileHash(staged)
			if err != nil {
				t.Fatal(err)
			}
			previousHash, err := bootstrapFileHash(backup)
			if err != nil {
				t.Fatal(err)
			}
			journal := bootstrapActivationJournal{
				SchemaVersion: 1,
				Status:        scenario.status,
				Destination:   destination,
				Staged:        staged,
				Backup:        backup,
				HadExisting:   true,
				PreviousHash:  previousHash,
				TargetHash:    targetHash,
				TargetVersion: buildinfo.CompatibilityVersion(),
			}
			if err := writeBootstrapJournal(filepath.Join(configDir, bootstrapActivationJournalFilename), journal); err != nil {
				t.Fatal(err)
			}

			skillDestination := filepath.Join(root, ".agents", "skills", "sell-a-skill")
			skillBackup := skillDestination + ".viceme-transaction-backup"
			configPath := filepath.Join(configDir, "config.json")
			configBackup := configPath + ".viceme-transaction-backup"
			writeBootstrapTestFile(t, filepath.Join(skillDestination, "state.txt"), "new-skill")
			writeBootstrapTestFile(t, filepath.Join(skillBackup, "state.txt"), "old-skill")
			writeBootstrapTestFile(t, configPath, "new-config")
			writeBootstrapTestFile(t, configBackup, "old-config")
			writeInstallRecoveryJournalForBootstrapTest(t, configDir, skillDestination, skillBackup, configPath, configBackup)

			environment := skillcontent.Environment{Home: root, ConfigDir: configDir}
			if err := recoverBootstrapActivation(configDir, environment); err != nil {
				t.Fatal(err)
			}
			assertBootstrapTestContent(t, destination, scenario.wantBinary)
			assertBootstrapTestContent(t, filepath.Join(skillDestination, "state.txt"), scenario.wantSkill)
			assertBootstrapTestContent(t, configPath, scenario.wantConfig)
			active, exists, err := updatepkg.ReadActiveGeneration(configDir)
			if scenario.status == "COMMITTING" {
				if err != nil || !exists || active.Version != buildinfo.CompatibilityVersion() || active.InstallMethod != "standalone" || active.Identity != targetHash {
					t.Fatalf("committed bootstrap did not publish its generation: active=%#v exists=%t err=%v", active, exists, err)
				}
			} else if err != nil || exists {
				t.Fatalf("rolled-back bootstrap published a generation: active=%#v exists=%t err=%v", active, exists, err)
			}
			for _, filename := range []string{
				filepath.Join(configDir, bootstrapActivationJournalFilename),
				filepath.Join(configDir, "install-transaction.json"),
				staged,
				backup,
			} {
				if _, err := os.Stat(filename); !os.IsNotExist(err) {
					t.Fatalf("recovery artifact was not retired: %s: %v", filename, err)
				}
			}
		})
	}
}

func TestBootstrapRejectsLateOlderGenerationInsideActivationLock(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	active, err := updatepkg.NewStandaloneGeneration(versionAfterCurrentRelease(t), strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	if err := updatepkg.CommitActiveGeneration(configDir, active); err != nil {
		t.Fatal(err)
	}
	runtime := &Runtime{
		configBase: configDir,
		region:     config.RegionCN,
		deps: Dependencies{
			Store:       securestore.NewMemory(),
			Environment: skillcontent.Environment{Home: root, ConfigDir: configDir},
		},
	}
	destination := filepath.Join(root, "bin", "viceme")
	command := &cobra.Command{}
	command.SetContext(context.Background())
	_, err = activateBootstrap(command, runtime, destination, "agents", "cn", false)
	cliError := output.AsError(err)
	if cliError.Subtype != "BOOTSTRAP_DOWNGRADE_REFUSED" {
		t.Fatalf("late older standalone activation was not fenced: %#v", cliError)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("fenced standalone activation changed destination: %v", err)
	}
	current, exists, err := updatepkg.ReadActiveGeneration(configDir)
	if err != nil || !exists || current != active {
		t.Fatalf("fenced activation changed active generation: current=%#v exists=%t err=%v", current, exists, err)
	}
}

func versionAfterCurrentRelease(t *testing.T) string {
	t.Helper()
	majorText, _, found := strings.Cut(strings.TrimPrefix(buildinfo.CompatibilityVersion(), "v"), ".")
	if !found {
		t.Fatalf("current compatibility version is not semantic: %q", buildinfo.CompatibilityVersion())
	}
	major, err := strconv.ParseUint(majorText, 10, 64)
	if err != nil || major == ^uint64(0) {
		t.Fatalf("current compatibility version has an invalid major component: %q", buildinfo.CompatibilityVersion())
	}
	return strconv.FormatUint(major+1, 10) + ".0.0"
}

func TestBootstrapRejectsNPMToStandaloneMigrationBeforeMutation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	active, err := updatepkg.NewNPMGeneration(buildinfo.CompatibilityVersion())
	if err != nil {
		t.Fatal(err)
	}
	if err := updatepkg.CommitActiveGeneration(configDir, active); err != nil {
		t.Fatal(err)
	}
	runtime := &Runtime{
		configBase: configDir,
		region:     config.RegionCN,
		deps: Dependencies{
			Store:       securestore.NewMemory(),
			Environment: skillcontent.Environment{Home: root, ConfigDir: configDir},
		},
	}
	destination := filepath.Join(root, "bin", "viceme")
	_, err = activateBootstrap(&cobra.Command{}, runtime, destination, "agents", "cn", false)
	cliError := output.AsError(err)
	if cliError.Subtype != "BOOTSTRAP_INSTALL_METHOD_CHANGE_REFUSED" {
		t.Fatalf("cross-method bootstrap was not rejected: %#v", cliError)
	}
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected cross-method bootstrap mutated destination: %v", err)
	}
	current, exists, err := updatepkg.ReadActiveGeneration(configDir)
	if err != nil || !exists || current != active {
		t.Fatalf("rejected cross-method bootstrap changed active generation: active=%#v exists=%t err=%v", current, exists, err)
	}
}

func TestBootstrapRechecksCrashedNPMJournalInsideActivationLock(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	target, err := updatepkg.NewNPMGeneration(buildinfo.CompatibilityVersion())
	if err != nil {
		t.Fatal(err)
	}
	journal, err := json.Marshal(map[string]any{
		"schemaVersion": 3,
		"status":        "COMMITTING",
		"nonce":         strings.Repeat("d", 64),
		"targetVersion": target.Version,
		"target":        target,
		"skillTarget":   "agents",
		"refreshSkills": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, updatepkg.NPMActivationJournalFilename), journal, 0o600); err != nil {
		t.Fatal(err)
	}
	runtime := &Runtime{
		configBase: configDir,
		region:     config.RegionCN,
		deps: Dependencies{
			Store:       securestore.NewMemory(),
			Environment: skillcontent.Environment{Home: root, ConfigDir: configDir},
		},
	}
	destination := filepath.Join(root, "bin", "viceme")

	_, err = activateBootstrap(&cobra.Command{}, runtime, destination, "agents", "cn", false)
	if cliError := output.AsError(err); cliError.Subtype != "BOOTSTRAP_NPM_RECOVERY_REQUIRED" {
		t.Fatalf("bootstrap did not reject the late npm journal: %#v", cliError)
	}
	if _, err := os.Stat(filepath.Join(configDir, bootstrapActivationJournalFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("bootstrap created a second outer journal: %v", err)
	}
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("bootstrap mutated its destination after arbitration failed: %v", err)
	}

	runner := &commandNPMRunner{}
	npm := updatepkg.NewNPMService(buildinfo.Version, buildinfo.CompatibilityVersion(), "npm")
	npm.ConfigDir = configDir
	npm.Runner = runner
	dependencies := Dependencies{
		Updater: npm,
		Environment: skillcontent.Environment{
			Home: root, ConfigDir: configDir,
		},
	}
	if err := reconcileActivationAtStartup(context.Background(), configDir, &dependencies); err != nil {
		t.Fatalf("ordinary startup could not recover the retained npm journal: %v", err)
	}
	if runner.calls != 2 {
		t.Fatalf("npm recovery did not finish the exact launcher and Skill generation: calls=%d", runner.calls)
	}
	if _, err := os.Stat(filepath.Join(configDir, updatepkg.NPMActivationJournalFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("npm recovery did not retire its journal: %v", err)
	}
	active, exists, err := updatepkg.ReadActiveGeneration(configDir)
	if err != nil || !exists || active != target {
		t.Fatalf("npm recovery did not publish the retained target: active=%#v exists=%t err=%v", active, exists, err)
	}
}

func TestNPMApplyRechecksCrashedBootstrapJournalInsideActivationLock(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	destination := filepath.Join(root, "bin", "viceme")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	journal := bootstrapActivationJournal{
		SchemaVersion: 1,
		Status:        "PREPARING",
		Destination:   destination,
		Staged:        filepath.Join(configDir, bootstrapActivationStagedFilename),
		Backup:        filepath.Join(configDir, bootstrapActivationBackupFilename),
		TargetHash:    strings.Repeat("a", 64),
	}
	if err := writeBootstrapJournal(filepath.Join(configDir, bootstrapActivationJournalFilename), journal); err != nil {
		t.Fatal(err)
	}
	runner := &commandNPMRunner{}
	npm := updatepkg.NewNPMService(buildinfo.Version, buildinfo.CompatibilityVersion(), "npm")
	npm.ConfigDir = configDir
	npm.Runner = runner

	_, err := npm.Apply(
		context.Background(),
		updatepkg.CheckResult{AvailableVersion: buildinfo.CompatibilityVersion(), UpdateAvailable: false},
		updatepkg.ApplyOptions{RefreshSkills: true, SkillTarget: "agents"},
	)
	if err == nil || !strings.Contains(err.Error(), "standalone bootstrap") {
		t.Fatalf("npm mutation did not reject the late bootstrap journal: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("npm performed a network mutation before outer journal arbitration: calls=%d", runner.calls)
	}
	if _, err := os.Stat(filepath.Join(configDir, updatepkg.NPMActivationJournalFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("npm created a second outer journal: %v", err)
	}
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected npm activation changed the bootstrap destination: %v", err)
	}

	dependencies := Dependencies{
		Updater: npm,
		Environment: skillcontent.Environment{
			Home: root, ConfigDir: configDir,
		},
	}
	if err := reconcileActivationAtStartup(context.Background(), configDir, &dependencies); err != nil {
		t.Fatalf("ordinary startup could not recover the retained bootstrap journal: %v", err)
	}
	if _, err := os.Stat(filepath.Join(configDir, bootstrapActivationJournalFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("bootstrap recovery did not retire its journal: %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("bootstrap recovery unexpectedly invoked npm: calls=%d", runner.calls)
	}
}

func TestStandaloneRecoveryRequiresOldProcessToRestartAfterRollForward(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	destination := filepath.Join(root, "bin", "viceme")
	staged := filepath.Join(configDir, bootstrapActivationStagedFilename)
	backup := filepath.Join(configDir, bootstrapActivationBackupFilename)
	writeBootstrapTestFile(t, destination, "old-binary")
	writeBootstrapTestFile(t, staged, "new-binary")
	writeBootstrapTestFile(t, backup, "old-binary")
	targetHash, err := bootstrapFileHash(staged)
	if err != nil {
		t.Fatal(err)
	}
	previousHash, err := bootstrapFileHash(backup)
	if err != nil {
		t.Fatal(err)
	}
	journal := bootstrapActivationJournal{
		SchemaVersion: 1,
		Status:        "COMMITTING",
		Destination:   destination,
		Staged:        staged,
		Backup:        backup,
		HadExisting:   true,
		PreviousHash:  previousHash,
		TargetHash:    targetHash,
		TargetVersion: buildinfo.CompatibilityVersion(),
	}
	if err := writeBootstrapJournal(filepath.Join(configDir, bootstrapActivationJournalFilename), journal); err != nil {
		t.Fatal(err)
	}
	dependencies := Dependencies{
		Updater: updatepkg.NewReleaseService(buildinfo.Version, buildinfo.CompatibilityVersion()),
		Environment: skillcontent.Environment{
			Home: root, ConfigDir: configDir,
		},
	}
	if err := reconcileActivationAtStartup(context.Background(), configDir, &dependencies); !errors.Is(err, updatepkg.ErrActivationRestartNeeded) {
		t.Fatalf("old standalone process continued after recovery: %v", err)
	}
	assertBootstrapTestContent(t, destination, "new-binary")
	active, exists, err := updatepkg.ReadActiveGeneration(configDir)
	if err != nil || !exists || active.InstallMethod != "standalone" || active.Identity != targetHash {
		t.Fatalf("standalone target did not commit before restart fence: active=%#v exists=%t err=%v", active, exists, err)
	}
}

func writeInstallRecoveryJournalForBootstrapTest(t *testing.T, configDir, skillDestination, skillBackup, configPath, configBackup string) {
	t.Helper()
	contents := `{
  "schema_version": 1,
  "status": "COMMITTING",
  "target_cli_version": "dev",
  "entries": [
    {"destination": ` + quotedJSON(skillDestination) + `, "stage": "", "backup": ` + quotedJSON(skillBackup) + `, "had_existing": true, "activating": true},
    {"destination": ` + quotedJSON(configPath) + `, "stage": "", "backup": ` + quotedJSON(configBackup) + `, "had_existing": true, "activating": true}
  ]
}
`
	if err := os.WriteFile(filepath.Join(configDir, "install-transaction.json"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func quotedJSON(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func writeBootstrapTestFile(t *testing.T, filename, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func assertBootstrapTestContent(t *testing.T, filename, expected string) {
	t.Helper()
	content, err := os.ReadFile(filename)
	if err != nil || string(content) != expected {
		t.Fatalf("unexpected content at %s: content=%q err=%v", filename, content, err)
	}
}
