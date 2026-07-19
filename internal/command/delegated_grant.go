package command

import (
	"path/filepath"

	"github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

const maxDelegatedGrantStdinBytes = 4096

func newDelegatedGrantCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{
		Use:   "delegated-grant",
		Short: "Manage delegated publication credentials in the OS keychain",
	}
	command.AddCommand(newDelegatedGrantSaveCommand(runtime))
	command.AddCommand(newDelegatedGrantStatusCommand(runtime))
	command.AddCommand(newDelegatedGrantDeleteCommand(runtime))
	return command
}

func newDelegatedGrantSaveCommand(runtime *Runtime) *cobra.Command {
	var stdin bool
	command := &cobra.Command{
		Use:   "save <credential-ref>",
		Short: "Read a delegated grant from stdin and save it without printing it",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if !stdin {
				return output.Validation("delegated_grant_stdin_required", "use --stdin so the delegated grant is never passed as a command argument")
			}
			value, err := readDelegatedGrantStdin(runtime)
			if err != nil {
				return err
			}
			manager := delegatedGrantManager(runtime)
			if err := manager.Save(args[0], value); err != nil {
				return err
			}
			return runtime.success(auth.DelegatedGrantStatus{CredentialRef: args[0], Stored: true})
		},
	}
	command.Flags().BoolVar(&stdin, "stdin", false, "read from protected non-TTY stdin; raw credentials are never accepted as flags")
	return command
}

func newDelegatedGrantStatusCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "status <credential-ref>",
		Short: "Check whether a delegated grant reference exists without reading its value",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			status, err := delegatedGrantManager(runtime).Status(args[0])
			if err != nil {
				return err
			}
			return runtime.success(status)
		},
	}
}

func newDelegatedGrantDeleteCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <credential-ref>",
		Short: "Delete a delegated grant reference from the OS keychain",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := delegatedGrantManager(runtime).Delete(args[0]); err != nil {
				return err
			}
			return runtime.success(auth.DelegatedGrantStatus{CredentialRef: args[0], Stored: false})
		},
	}
}

func delegatedGrantManager(runtime *Runtime) *auth.DelegatedGrantManager {
	return &auth.DelegatedGrantManager{
		Store:  runtime.deps.Store,
		Region: string(runtime.region),
		Scope:  runtime.credentialScope,
		NewID:  runtime.deps.NewID,
		LockDir: filepath.Join(
			runtimeConfigBase(runtime.deps.Environment),
			"viceme",
			"locks",
		),
	}
}

func readDelegatedGrantStdin(runtime *Runtime) (string, error) {
	if runtime.deps.InputIsTerminal != nil && runtime.deps.InputIsTerminal() {
		return "", output.Validation(
			"delegated_grant_tty_unsupported",
			"delegated grant stdin is an interactive terminal and may echo secrets; pipe it from a protected channel instead",
		)
	}
	return readLimited(runtime.deps.In, maxDelegatedGrantStdinBytes)
}
