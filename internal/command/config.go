package command

import (
	"runtime"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/spf13/cobra"
)

func newConfigCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "config", Short: "Manage ViceMe CLI runtime configuration"}
	command.AddCommand(newConfigKeychainDowngradeCommand(runtime))
	return command
}

func newConfigKeychainDowngradeCommand(runtimeState *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "keychain-downgrade",
		Short: "Enable encrypted credential access from macOS sandboxes",
		Long: `Copy the ViceMe credential master key from the macOS Keychain into a
private local file and migrate configured legacy Keychain credentials into
encrypted files. Run this once from an interactive Terminal session where the
macOS Keychain is reachable. Existing Keychain entries are preserved as a cold
backup; no token is printed or written in plaintext.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if runtime.GOOS != "darwin" {
				return output.Validation("keychain_downgrade_unsupported", "keychain downgrade is only supported on macOS")
			}
			downgrader, ok := runtimeState.deps.Store.(securestore.KeychainDowngrader)
			if !ok {
				return output.Policy("keychain_downgrade_unavailable", "the active credential store does not support keychain downgrade")
			}
			keys, err := runtimeState.credentialStorageKeys()
			if err != nil {
				return output.Internal("credential_scope", "could not enumerate configured credential scopes", err)
			}
			result, err := downgrader.DowngradeKeychain(keys)
			if err != nil {
				return output.Authentication("keychain_downgrade_failed", "could not materialize the ViceMe credential master key for sandbox access").
					WithHint("run this command from Terminal.app or iTerm where the macOS Keychain is reachable; then retry the original command inside Codex or Claude Code").
					WithCause(err)
			}
			return runtimeState.success(result)
		},
	}
}
