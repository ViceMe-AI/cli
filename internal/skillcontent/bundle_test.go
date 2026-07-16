package skillcontent_test

import (
	"os"
	"path/filepath"
	"testing"

	cliembed "github.com/ViceMe-AI/cli"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
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
	for _, directory := range []string{filepath.Join(home, ".codex"), filepath.Join(home, ".claude")} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	environment := skillcontent.Environment{Home: home}
	report := bundle.Install("viceme", "auto", environment)
	if !report.AllSucceeded || len(report.Results) != 2 {
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
	if !doctor.Healthy || len(doctor.Results) != 2 {
		t.Fatalf("unexpected doctor report: %#v", doctor)
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
