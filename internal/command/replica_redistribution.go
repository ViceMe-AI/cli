package command

import (
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

func newReplicaRedistributionGrantsCommand(runtime *Runtime) *cobra.Command {
	var cursor string
	cmd := &cobra.Command{Use: "redistribution-grants", Short: "List your source entitlements and their ViceMe republication permission", Args: cobra.NoArgs}
	cmd.Flags().StringVar(&cursor, "cursor", "", "nextCursor from the previous page")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if cursor != "" && !replicaUUIDPattern.MatchString(cursor) {
			return output.Validation("REPLICA_CURSOR_INVALID", "--cursor must be a UUID")
		}
		if err := runtime.requireWebsiteReplicaAuthentication(cmd.Context(), "website-replica:read"); err != nil {
			return err
		}
		result, err := runtime.client().GetWebsiteReplicaRedistributionGrants(cmd.Context(), cursor)
		if err != nil {
			return err
		}
		return runtime.business(result)
	}
	return cmd
}
