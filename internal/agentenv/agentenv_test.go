package agentenv

import (
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func env(pairs map[string]string) func(string) string {
	return func(key string) string { return pairs[key] }
}

func TestDetectByMarkersAndPrecedence(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want Platform
	}{
		{"workbuddy session", map[string]string{"CODEBUDDY_SESSION_ID": "1"}, WorkBuddy},
		{"workbuddy broker session", map[string]string{"CODEBUDDY_SANDBOX_BROKER_SESSION_ID": "1"}, WorkBuddy},
		{"workbuddy legacy name", map[string]string{"WORKBUDDY_SESSION_ID": "1"}, WorkBuddy},
		{"codex session", map[string]string{"CODEX_SESSION_ID": "1"}, Codex},
		{"codex thread", map[string]string{"CODEX_THREAD_ID": "1"}, Codex},
		{"codex sandbox", map[string]string{"CODEX_SANDBOX": "seatbelt"}, Codex},
		{"claude cli", map[string]string{"CLAUDECODE": "1"}, Claude},
		{"claude sdk", map[string]string{"CLAUDE_AGENT_SDK_VERSION": "1"}, Claude},
		{"claude desktop agent", map[string]string{"AI_AGENT": "claude-code_1.0.0"}, Claude},
		{"doubao office app", map[string]string{"DOUBAO_OFFICE_APP_ID": "1"}, Doubao},
		{"doubao office agent name", map[string]string{"DOUBAO_OFFICE_AGENT_NAME": "1"}, Doubao},
		{"doubao office edition", map[string]string{"DOUBAO_OFFICE_EDITION": "pro"}, Doubao},
		{"unknown host", map[string]string{"SHELL": "/bin/zsh"}, Unknown},
		{"empty markers ignored", map[string]string{"CODEBUDDY_SESSION_ID": "", "CODEX_SESSION_ID": ""}, Unknown},
		{"precedence workbuddy over codex", map[string]string{"CODEBUDDY_SESSION_ID": "1", "CODEX_SESSION_ID": "1"}, WorkBuddy},
		{"precedence codex over claude", map[string]string{"CODEX_SESSION_ID": "1", "CLAUDECODE": "1"}, Codex},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Detect(env(tc.env)); got != tc.want {
				t.Fatalf("Detect() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDetectNilGetenv(t *testing.T) {
	if got := Detect(nil); got != Unknown {
		t.Fatalf("Detect(nil) = %q, want %q", got, Unknown)
	}
}

// TestTrialRuntimeMarkersMatch locks the Go fingerprint table to the trial
// runtime script so the standalone Python copy cannot drift.
func TestTrialRuntimeMarkersMatch(t *testing.T) {
	source, err := os.ReadFile("../../skills/use-a-skill/scripts/trial_runtime.py")
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)AGENT_ENV_MARKERS = \{(.*?)\n\}`).FindSubmatch(source)
	if block == nil {
		t.Fatal("trial runtime no longer declares AGENT_ENV_MARKERS")
	}
	entries := regexp.MustCompile(`"([a-z]+)":\s*\(([^)]*)\)`).
		FindAllStringSubmatch(string(block[1]), -1)
	if entries == nil {
		t.Fatal("AGENT_ENV_MARKERS block has no platform entries")
	}
	var got []struct {
		Platform Platform
		Markers  []string
	}
	for _, entry := range entries {
		var markers []string
		for _, marker := range regexp.MustCompile(`"([A-Z0-9_]+)"`).FindAllStringSubmatch(entry[2], -1) {
			markers = append(markers, marker[1])
		}
		got = append(got, struct {
			Platform Platform
			Markers  []string
		}{Platform(entry[1]), markers})
	}
	if !reflect.DeepEqual(got, AgentEnvMarkers) {
		t.Fatalf("trial runtime AGENT_ENV_MARKERS drifted from the Go table:\ngot  %#v\nwant %#v", got, AgentEnvMarkers)
	}
	if !strings.Contains(string(source), `ai_agent.startswith("claude-code")`) {
		t.Fatal("trial runtime no longer maps AI_AGENT=claude-code* to the claude platform")
	}
}
