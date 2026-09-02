package command

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestSkillPublishRequiresEditionSelectionBeforeReadingSource(t *testing.T) {
	for _, sourceFlags := range [][]string{
		{"--path", "does-not-exist"},
		{"--path", "does-not-exist", "--listing", "66666666-6666-4666-8666-666666666666"},
		{"--github", "example/skill"},
		{"--xiaohongshu-skill-id", "skill-id"},
	} {
		for _, flags := range [][]string{{}, {"--edition-key", "paid-tool"}, {"--edition-order", "1"}} {
			t.Run(strings.Join(append(append([]string{}, sourceFlags...), flags...), " "), func(t *testing.T) {
				root := t.TempDir()
				var stdout bytes.Buffer
				args := append(append([]string{"skill", "publish"}, sourceFlags...), flags...)
				exit := Execute(args, Dependencies{
					Out: &stdout, ErrOut: io.Discard,
					Environment: skillcontent.Environment{Home: root, ConfigDir: root},
				})
				if exit == 0 || !strings.Contains(stdout.String(), "SKILL_EDITION_SELECTION_REQUIRED") {
					t.Fatalf("ambiguous edition must fail before source/auth/network operations: %s", stdout.String())
				}
			})
		}
	}
}

func TestResumeCannotChangeEditionIdentity(t *testing.T) {
	for _, flags := range [][]string{
		{"--edition-key", "another"}, {"--edition-order", "1"},
		{"--edition-title", "Another"}, {"--edition-highlight", "Extra"}, {"--new-listing"},
	} {
		t.Run(strings.Join(flags, " "), func(t *testing.T) {
			root := t.TempDir()
			var stdout bytes.Buffer
			args := append([]string{"skill", "publish", "--resume", "not-a-pending-publication"}, flags...)
			exit := Execute(args, Dependencies{
				Out: &stdout, ErrOut: io.Discard,
				Environment: skillcontent.Environment{Home: root, ConfigDir: root},
			})
			if exit == 0 || !strings.Contains(stdout.String(), "PUBLICATION_FLAGS_CONFLICT") {
				t.Fatalf("resume must not silently ignore a changed edition: %s", stdout.String())
			}
		})
	}
}
