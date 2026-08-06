package skillcontent_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cliembed "github.com/ViceMe-AI/cli"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	"github.com/gofrs/flock"
)

func TestEmbeddedVicemeSkillIsValid(t *testing.T) {
	t.Parallel()
	bundle := skillcontent.New(cliembed.EmbeddedSkills())
	if err := bundle.Validate("viceme"); err != nil {
		t.Fatal(err)
	}
	list, err := bundle.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "viceme" || list[0].Description == "" {
		t.Fatalf("unexpected list: %#v", list)
	}
	digests, err := bundle.Digests("viceme")
	if err != nil {
		t.Fatal(err)
	}
	if digests.Full == "" || digests.Embedded == "" || digests.Full == digests.Embedded {
		t.Fatalf("expected distinct non-empty digests: %#v", digests)
	}
	data, resolved, err := bundle.Read("viceme", "references/commands.md")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != "references/commands.md" || len(data) == 0 {
		t.Fatalf("unexpected read result: %q %d", resolved, len(data))
	}
	commands := string(data)
	for _, required := range []string{
		"pnpm add @viceme/web-sdk@0.1.0",
		"https://cdn.jsdelivr.net/npm/@viceme/web-sdk@0.1.0",
		"mountCommerceCheckoutWidget",
		"Next.js Client Component",
	} {
		if !strings.Contains(commands, required) {
			t.Fatalf("embedded Commerce integration omitted %q", required)
		}
	}
	if strings.Contains(commands, "<ViceMe CDN>") {
		t.Fatal("embedded Commerce integration contains an unresolved CDN placeholder")
	}
}

func TestReadRejectsPathTraversal(t *testing.T) {
	t.Parallel()
	bundle := skillcontent.New(cliembed.EmbeddedSkills())
	if _, _, err := bundle.Read("viceme", "../secrets"); err == nil {
		t.Fatal("expected path traversal to fail")
	}
}

func TestInstallAndDoctorTargetsIndependently(t *testing.T) {
	t.Parallel()
	bundle := skillcontent.New(cliembed.EmbeddedSkills())
	home := t.TempDir()
	for _, directory := range []string{filepath.Join(home, ".codex"), filepath.Join(home, ".claude"), filepath.Join(home, ".agents")} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	environment := skillcontent.Environment{Home: home}
	report := bundle.Install("viceme", "auto", environment)
	if !report.AllSucceeded || len(report.Results) != 3 {
		t.Fatalf("unexpected install report: %#v", report)
	}
	for _, result := range report.Results {
		if result.Status != "updated" {
			t.Fatalf("unexpected target result: %#v", result)
		}
		if _, err := os.Stat(filepath.Join(result.Path, "SKILL.md")); err != nil {
			t.Fatal(err)
		}
	}
	doctor := bundle.Doctor("viceme", "auto", environment)
	if !doctor.Healthy || len(doctor.Results) != 3 {
		t.Fatalf("unexpected doctor report: %#v", doctor)
	}
	manifestPath := filepath.Join(home, ".codex", "skills", "viceme", ".viceme", "install-manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["cli_compatibility"] = ">=9.0.0 <10.0.0"
	manifestData, _ = json.Marshal(manifest)
	if err := os.WriteFile(manifestPath, manifestData, 0o600); err != nil {
		t.Fatal(err)
	}
	doctor = bundle.Doctor("viceme", "codex", environment)
	if doctor.Healthy || doctor.Results[0].Checks.Compatibility.Healthy {
		t.Fatalf("doctor missed compatibility manifest drift: %#v", doctor)
	}
	compatibilityRepair := bundle.Install("viceme", "codex", environment)
	if !compatibilityRepair.AllSucceeded || compatibilityRepair.Results[0].Status != "updated" {
		t.Fatalf("unexpected compatibility repair: %#v", compatibilityRepair)
	}

	codexSkill := filepath.Join(home, ".codex", "skills", "viceme", "SKILL.md")
	if err := os.WriteFile(codexSkill, []byte("locally modified\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	doctor = bundle.Doctor("viceme", "codex", environment)
	if doctor.Healthy || len(doctor.Results) != 1 || doctor.Results[0].Healthy {
		t.Fatalf("doctor missed local modification: %#v", doctor)
	}

	repair := bundle.Install("viceme", "codex", environment)
	if !repair.AllSucceeded || repair.Results[0].Status != "updated" {
		t.Fatalf("unexpected repair report: %#v", repair)
	}
	doctor = bundle.Doctor("viceme", "codex", environment)
	if !doctor.Healthy {
		t.Fatalf("repair did not restore healthy Skill: %#v", doctor)
	}

	unchanged := bundle.Install("viceme", "codex", environment)
	if !unchanged.AllSucceeded || unchanged.Results[0].Status != "unchanged" {
		t.Fatalf("expected unchanged install: %#v", unchanged)
	}
}

func TestMultiTargetInstallDoesNotMutateAnyTargetWhenOneLockIsUnavailable(t *testing.T) {
	bundle := skillcontent.New(cliembed.EmbeddedSkills())
	home := t.TempDir()
	for _, directory := range []string{filepath.Join(home, ".codex"), filepath.Join(home, ".claude")} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	claudeSkill := filepath.Join(home, ".claude", "skills", "viceme")
	if err := os.MkdirAll(claudeSkill, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(claudeSkill, "SKILL.md")
	if err := os.WriteFile(marker, []byte("previous installation\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	codexSkill := filepath.Join(home, ".codex", "skills", "viceme")
	if err := os.MkdirAll(filepath.Dir(codexSkill), 0o755); err != nil {
		t.Fatal(err)
	}
	blocked := flock.New(codexSkill + ".viceme-install-lock")
	if err := blocked.Lock(); err != nil {
		t.Fatal(err)
	}
	defer blocked.Unlock()

	report := bundle.Install("viceme", "auto", skillcontent.Environment{Home: home})
	if report.AllSucceeded {
		t.Fatalf("blocked multi-target install succeeded: %#v", report)
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != "previous installation\n" {
		t.Fatalf("another target changed before all locks were acquired: data=%q err=%v report=%#v", data, err, report)
	}
	if _, err := os.Stat(filepath.Join(codexSkill, "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("blocked target was partially installed: %v", err)
	}
}

func TestValidateRejectsMergeConflictMarkers(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	source := filepath.Join(root, "src")
	if err := os.MkdirAll(filepath.Join(source, "viceme", "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, "viceme", "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"viceme/SKILL.md":               "---\nname: viceme\ndescription: test\n---\n# ok\n",
		"viceme/agents/openai.yaml":     "interface: {}\n",
		"viceme/skill-package.json":     `{"schema_version":1,"skill_version":"0.1.0","minimum_cli_version":"0.1.0","cli_compatibility":">=0.1.0 <0.2.0"}`,
		"viceme/references/commands.md": "safe\n||||||| 2895654\n## duplicated\n",
	}
	for relative, content := range files {
		if err := os.WriteFile(filepath.Join(source, relative), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bundle := skillcontent.New(os.DirFS(source))
	err := bundle.Validate("viceme")
	if err == nil || !strings.Contains(err.Error(), "merge conflict marker") {
		t.Fatalf("merge markers must fail validation: %v", err)
	}
}
