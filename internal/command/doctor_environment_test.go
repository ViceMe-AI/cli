package command

import (
	"path/filepath"
	"testing"
)

func TestDoctorEnvironmentSectionReportsVerifiedCapabilities(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		env        map[string]string
		platform   string
		launch     string
		stderrSeen bool
	}{
		{"doubao", map[string]string{"DOUBAO_OFFICE_EDITION": "public"}, "doubao", "foreground-auto-background", false},
		{"workbuddy", map[string]string{"CODEBUDDY_SESSION_ID": "1"}, "workbuddy", "background-param", true},
		{"codex", map[string]string{"CODEX_SESSION_ID": "1"}, "codex", "unverified", false},
		{"claude", map[string]string{"CLAUDECODE": "1"}, "claude", "unverified", false},
		{"unknown host", map[string]string{"SHELL": "/bin/zsh"}, "", "unverified", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			getenv := func(key string) string { return tc.env[key] }
			section := doctorEnvironmentSection(getenv, "/config-base")
			if section["platform"] != tc.platform {
				t.Fatalf("platform = %v, want %q", section["platform"], tc.platform)
			}
			if section["loginLaunchStrategy"] != tc.launch {
				t.Fatalf("loginLaunchStrategy = %v, want %q", section["loginLaunchStrategy"], tc.launch)
			}
			if section["taskOutputStderr"] != tc.stderrSeen {
				t.Fatalf("taskOutputStderr = %v, want %v", section["taskOutputStderr"], tc.stderrSeen)
			}
			if osName, ok := section["os"].(string); !ok || osName == "" {
				t.Fatalf("os must be a non-empty string, got %v", section["os"])
			}
		})
	}
}

func TestDoctorEnvironmentSectionExposesDeterministicPresentationPath(t *testing.T) {
	t.Parallel()
	section := doctorEnvironmentSection(func(string) string { return "" }, "/config-base")
	path, ok := section["loginPresentationPath"].(string)
	if !ok || path == "" {
		t.Fatalf("loginPresentationPath must be a non-empty string, got %v", section["loginPresentationPath"])
	}
	if want := filepath.Join("/config-base", "auth-presentations", "login-presentation.json"); path != want {
		t.Fatalf("loginPresentationPath = %q, want %q", path, want)
	}
}
