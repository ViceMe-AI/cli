package command

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type dryRunExample struct {
	Name                string   `json:"name"`
	DocumentationAnchor string   `json:"documentation_anchor"`
	Input               string   `json:"input"`
	Args                []string `json:"args"`
	Operation           string   `json:"operation"`
	SourceMode          string   `json:"source_mode"`
}

func TestDocumentedPublishExamplesExecuteAsNetworkFreeDryRuns(t *testing.T) {
	repository := repositoryRoot(t)
	data, err := os.ReadFile(filepath.Join(repository, "quality", "example-dry-runs.json"))
	if err != nil {
		t.Fatal(err)
	}
	var examples []dryRunExample
	if err := json.Unmarshal(data, &examples); err != nil {
		t.Fatal(err)
	}
	if len(examples) < 8 {
		t.Fatalf("expected source and destination coverage, got %d examples", len(examples))
	}
	commandsDocument, err := os.ReadFile(filepath.Join(repository, "skills", "viceme", "references", "commands.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, example := range examples {
		example := example
		t.Run(example.Name, func(t *testing.T) {
			if !strings.Contains(string(commandsDocument), example.DocumentationAnchor) {
				t.Fatalf("quality case is not anchored to the Agent Skill documentation: %s", example.DocumentationAnchor)
			}
			if !containsArgument(example.Args, "--dry-run") || !containsArgument(example.Args, "--json") {
				t.Fatal("quality case must execute the JSON dry-run path")
			}
			code, stdout, stderr, _ := runCLIWithDependencies(t, nil, nil, example.Input, Dependencies{
				HTTPClient: &http.Client{Transport: noNetworkTransport{t: t}},
				APIBaseURL: "https://network-must-not-run.invalid",
			}, example.Args...)
			if code != 0 || stderr != "" {
				t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
			}
			var envelope struct {
				Data map[string]any `json:"data"`
			}
			if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Data["dry_run"] != true || envelope.Data["operation"] != example.Operation || envelope.Data["source_mode"] != example.SourceMode {
				t.Fatalf("unexpected dry-run result: %#v", envelope.Data)
			}
		})
	}
}

func containsArgument(args []string, wanted string) bool {
	for _, argument := range args {
		if argument == wanted {
			return true
		}
	}
	return false
}

type noNetworkTransport struct{ t *testing.T }

func (transport noNetworkTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.t.Helper()
	transport.t.Fatalf("dry-run attempted network access: %s", request.URL)
	return nil, fmt.Errorf("network forbidden")
}

var _ http.RoundTripper = noNetworkTransport{}
