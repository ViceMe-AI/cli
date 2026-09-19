package publication

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestCloudPublicationRequiresExplicitDisclosureAndBindsManifest(t *testing.T) {
	directory := t.TempDir()
	writeTestFile(t, filepath.Join(directory, "WORKFLOW.md"), []byte("Read inputs locally, apply core rules, build the poster."), 0o644)
	writeTestFile(t, filepath.Join(directory, "SKILL.md"), []byte(testSkillMarkdown), 0o644)
	writeTestFile(t, filepath.Join(directory, "private.md"), []byte("private recipe"), 0o644)
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	writeTestFile(t, filepath.Join(directory, "cover.png"), png, 0o644)
	declaration := []byte(`{"version":1,"purpose":"draft a poster","localWorkflow":"WORKFLOW.md","privateFiles":["SKILL.md","private.md"],"publicFiles":["WORKFLOW.md","cover.png"]}`)
	writeTestFile(t, filepath.Join(directory, "viceme-cloud.json"), declaration, 0o644)
	pkg, err := Build(directory)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Manifest.Spec.DeliveryMode != "CLOUD" || pkg.Manifest.Spec.Cloud == nil || len(pkg.Candidates) != 1 || pkg.Candidates[0].RelativePath != "cover.png" {
		t.Fatalf("cloud declaration lost: %#v", pkg.Manifest)
	}
	writeTestFile(t, filepath.Join(directory, "SKILL.md"), []byte("---\nname: poster-skill\n---\nPRIVATE-BODY-MUST-NOT-BECOME-LISTING"), 0o644)
	missingDescription, err := Build(directory)
	if err != nil || missingDescription.Manifest.Metadata.Summary != "draft a poster" {
		t.Fatal("private Markdown body became listing metadata")
	}
	writeTestFile(t, filepath.Join(directory, "SKILL.md"), []byte(testSkillMarkdown), 0o644)
	digest := pkg.Artifact.Digest
	for _, invalid := range []string{
		`{"version":1,"purpose":"draft","privateFiles":["SKILL.md","private.md"],"publicFiles":[]}`,
		`{"version":1,"purpose":"draft","privateFiles":["SKILL.md","private.md","cover.png"],"publicFiles":[]}`,
		`{"version":1,"purpose":"draft","privateFiles":["SKILL.md","private.md"],"publicFiles":["cover.png","COVER.png"]}`,
		`{"version":1,"purpose":"draft","privateFiles":["SKILL.md","private.md"],"publicFiles":["cover.png"],"unknown":true}`,
	} {
		writeTestFile(t, filepath.Join(directory, "viceme-cloud.json"), []byte(invalid), 0o644)
		if _, err := Build(directory); err == nil {
			t.Fatalf("accepted unclassified/private image: %s", invalid)
		}
	}
	writeTestFile(t, filepath.Join(directory, "viceme-cloud.json"), declaration, 0o644)
	writeTestFile(t, filepath.Join(directory, "private.md"), []byte("changed private recipe"), 0o644)
	changed, err := Build(directory)
	if err != nil || changed.Artifact.Digest == digest {
		t.Fatal("private source change did not change publication identity")
	}
	if err := os.Remove(filepath.Join(directory, "viceme-cloud.json")); err != nil {
		t.Fatal(err)
	}
	source, err := Build(directory)
	if err != nil || source.Manifest.Spec.DeliveryMode != "" || source.Manifest.Spec.Cloud != nil {
		t.Fatal("SOURCE compatibility failed")
	}
}
