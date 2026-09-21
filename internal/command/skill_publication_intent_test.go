package command

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestResumeCannotCreateAnotherListing(t *testing.T) {
	for _, flags := range [][]string{
		{"--new-listing"},
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
