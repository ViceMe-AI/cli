package agentenv

// LoginLaunchStrategy names the verified way a host's shell tooling launches
// a bounded-wait interactive command such as `viceme auth login` without
// blocking the agent turn.
type LoginLaunchStrategy string

const (
	// LoginLaunchBackgroundParam: the shell tool accepts a run_in_background
	// parameter and returns a task id immediately (verified on WorkBuddy).
	LoginLaunchBackgroundParam LoginLaunchStrategy = "background-param"
	// LoginLaunchForegroundAuto: the host moves a foreground command to the
	// background after a short wait and exposes a task id (verified on
	// Doubao, where run_in_background exists in the tool schema but returns
	// exit code -1 without creating a task).
	LoginLaunchForegroundAuto LoginLaunchStrategy = "foreground-auto-background"
	// LoginLaunchUnverified: no launch strategy has been verified on this
	// platform. Callers fall back to the host's own background mechanism or
	// launch in the foreground and let the host decide.
	LoginLaunchUnverified LoginLaunchStrategy = "unverified"
)

// Capabilities is the verified host capability profile consumed by login
// orchestration. Values mirror the human-maintained host capability registry
// (Agent 宿主能力登记表): a platform only gets a capability after a dated
// on-host verification, never from tool schema claims alone.
type Capabilities struct {
	// Platform is the detected host, empty for unknown hosts.
	Platform Platform `json:"platform"`
	// LoginLaunch is the verified launch strategy for bounded-wait commands.
	LoginLaunch LoginLaunchStrategy `json:"loginLaunchStrategy"`
	// TaskOutputStderr reports whether reading a background task surfaces its
	// stderr output. When false, agents must read the login presentation from
	// the deterministic file the CLI writes instead of task reads.
	TaskOutputStderr bool `json:"taskOutputStderr"`
}

// capabilityTable maps each detected platform to its verified capabilities.
// Doubao's run_in_background is present in the tool schema but returns exit
// code -1 without creating a task (verified 2026-09-17 on darwin and windows),
// so it is deliberately not offered as a launch strategy; its task reads never
// surface stderr, which is why the CLI persists the login presentation to a
// deterministic file.
var capabilityTable = map[Platform]Capabilities{
	WorkBuddy: {Platform: WorkBuddy, LoginLaunch: LoginLaunchBackgroundParam, TaskOutputStderr: true},
	Doubao:    {Platform: Doubao, LoginLaunch: LoginLaunchForegroundAuto, TaskOutputStderr: false},
	Codex:     {Platform: Codex, LoginLaunch: LoginLaunchUnverified, TaskOutputStderr: false},
	Claude:    {Platform: Claude, LoginLaunch: LoginLaunchUnverified, TaskOutputStderr: false},
}

// CapabilityProfile returns the verified capability profile for a platform.
// Unknown platforms get the fail-safe profile: no verified launch strategy and
// no stderr through task reads.
func CapabilityProfile(platform Platform) Capabilities {
	if caps, ok := capabilityTable[platform]; ok {
		return caps
	}
	return Capabilities{Platform: platform, LoginLaunch: LoginLaunchUnverified, TaskOutputStderr: false}
}
