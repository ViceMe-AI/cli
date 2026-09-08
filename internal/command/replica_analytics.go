package command

import (
	"time"

	"github.com/spf13/cobra"
)

func newReplicaAnalyticsCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "analytics <work-url>",
		Short: "Read current owner analytics from the authenticated Work Markdown",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			url, markdown, err := runtime.client().ReadWebsiteReplicaOwnerMarkdown(command.Context(), runtime.profile.ResolvedWebBaseURL(), args[0])
			if err != nil {
				return err
			}
			return runtime.business(struct {
				MarkdownURL string `json:"markdownUrl"`
				Markdown    string `json:"markdown"`
				FetchedAt   string `json:"fetchedAt"`
			}{url, markdown, runtime.deps.Now().UTC().Format(time.RFC3339)})
		},
	}
}
