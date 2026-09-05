package skillcontent

import (
	"encoding/json"
	cliembed "github.com/ViceMe-AI/cli"
	"io/fs"
	"os"
	"strings"
	"testing"
)

// These fixtures guard the official interpretation table, not model execution
// or server transitions. Misleading message text must not select a branch.
func TestReplicaOwnerFlowStructuredFixtures(t *testing.T) {
	guide, err := fs.ReadFile(cliembed.EmbeddedSkills(), "let-others-make-a-copy/references/flow-contract.md")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("testdata/replica-owner-flow.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name     string
		Exit     int
		Selector string
		Output   map[string]any
		Required string
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			if fixture.Output["ok"] != (fixture.Exit == 0) {
				t.Fatal("fixture exit and envelope disagree")
			}
			var value any = fixture.Output
			for _, part := range strings.Split(fixture.Selector, ".") {
				object, ok := value.(map[string]any)
				if !ok {
					t.Fatal("invalid structured selector")
				}
				value = object[part]
			}
			key, ok := value.(string)
			if !ok || key == "" || strings.Contains(fixture.Selector, "message") {
				t.Fatal("fixture must select structured status or action")
			}
			for _, line := range strings.Split(string(guide), "\n") {
				if strings.Contains(line, "`"+key+"`") && strings.Contains(line, fixture.Required) {
					return
				}
			}
			t.Fatalf("missing interpretation for %s: %s", key, fixture.Required)
		})
	}
}
