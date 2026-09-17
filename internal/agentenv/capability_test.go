package agentenv

import "testing"

func TestCapabilityProfileVerifiedPlatforms(t *testing.T) {
	cases := []struct {
		platform Platform
		want     Capabilities
	}{
		{WorkBuddy, Capabilities{Platform: WorkBuddy, LoginLaunch: LoginLaunchBackgroundParam, TaskOutputStderr: true}},
		{Doubao, Capabilities{Platform: Doubao, LoginLaunch: LoginLaunchForegroundAuto, TaskOutputStderr: false}},
		{Codex, Capabilities{Platform: Codex, LoginLaunch: LoginLaunchUnverified, TaskOutputStderr: false}},
		{Claude, Capabilities{Platform: Claude, LoginLaunch: LoginLaunchUnverified, TaskOutputStderr: false}},
	}
	for _, tc := range cases {
		t.Run(string(tc.platform), func(t *testing.T) {
			got := CapabilityProfile(tc.platform)
			if got != tc.want {
				t.Fatalf("CapabilityProfile(%q) = %+v, want %+v", tc.platform, got, tc.want)
			}
		})
	}
}

func TestCapabilityProfileUnknownIsFailSafe(t *testing.T) {
	got := CapabilityProfile(Unknown)
	if got.Platform != Unknown {
		t.Fatalf("platform = %q, want unknown", got.Platform)
	}
	if got.LoginLaunch != LoginLaunchUnverified {
		t.Fatalf("login launch = %q, want unverified", got.LoginLaunch)
	}
	if got.TaskOutputStderr {
		t.Fatal("unknown host must not claim stderr through task reads")
	}
}

func TestCapabilityProfileEveryDetectedPlatformHasEntry(t *testing.T) {
	for _, entry := range AgentEnvMarkers {
		caps := CapabilityProfile(entry.Platform)
		if caps.Platform != entry.Platform {
			t.Fatalf("platform %q lost its identity in the capability profile", entry.Platform)
		}
		switch caps.LoginLaunch {
		case LoginLaunchBackgroundParam, LoginLaunchForegroundAuto, LoginLaunchUnverified:
		default:
			t.Fatalf("platform %q has unknown launch strategy %q", entry.Platform, caps.LoginLaunch)
		}
	}
}
