// Package agentenv identifies the agent host that invoked the CLI.
//
// The fingerprint table is the single authority for "which agent platform is
// this process running inside". The no-CLI trial runtime
// (skills/use-a-skill/scripts/trial_runtime.py) mirrors it in
// AGENT_ENV_MARKERS because the shipped script must stay standalone;
// the anchor test in this package keeps both tables identical.
package agentenv

import "strings"

// Platform names the invoking agent host. The empty value means the process
// is not running inside a known agent host (for example a plain terminal).
type Platform string

const (
	WorkBuddy Platform = "workbuddy"
	Codex     Platform = "codex"
	Claude    Platform = "claude"
	Unknown   Platform = ""
)

// AgentEnvMarkers is the invoking-agent fingerprint table in detection
// precedence order. Marker tuples must stay identical, and in the same
// platform order, as AGENT_ENV_MARKERS in the trial runtime script.
var AgentEnvMarkers = []struct {
	Platform Platform
	Markers  []string
}{
	{WorkBuddy, []string{"CODEBUDDY_SESSION_ID", "CODEBUDDY_SANDBOX_BROKER_SESSION_ID", "WORKBUDDY_SESSION_ID"}},
	{Codex, []string{"CODEX_SESSION_ID", "CODEX_THREAD_ID", "CODEX_SANDBOX"}},
	{Claude, []string{"CLAUDECODE", "CLAUDE_AGENT_SDK_VERSION"}},
}

// Detect returns the invoking agent platform, or Unknown when no known agent
// host marker is present. AI_AGENT=claude-code* (the Claude Desktop agent
// runtime) also maps to Claude, mirroring the trial runtime's fallback.
func Detect(getenv func(string) string) Platform {
	if getenv == nil {
		return Unknown
	}
	for _, entry := range AgentEnvMarkers {
		for _, marker := range entry.Markers {
			if getenv(marker) != "" {
				return entry.Platform
			}
		}
	}
	if strings.HasPrefix(getenv("AI_AGENT"), "claude-code") {
		return Claude
	}
	return Unknown
}
