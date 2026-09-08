package command

import (
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

func newCreatorPageSetupCommand(_ *Runtime) *cobra.Command {
	command := &cobra.Command{
		Use:    "page-setup [application-id]",
		Short:  "Retired creator page setup command",
		Hidden: true,
		Args:   cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error {
			return output.Validation("PAGE_SETUP_RETIRED", "creator page setup is retired; continue with the installed creator Skills")
		},
	}
	command.Flags().Bool("wait", false, "retired")
	command.Flags().String("merchant", "", "retired")
	command.Flags().Duration("timeout", 0, "retired")
	return command
}
