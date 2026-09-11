package command

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

const accountAvatarMaxBytes = 2 * 1024 * 1024

func newAccountCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "account", Short: "Manage the current ViceMe account profile"}
	command.AddCommand(newAccountProfileCommand(runtime))
	command.AddCommand(newAccountAvatarCommand(runtime))
	return command
}

func newAccountProfileCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "profile", Short: "Manage the current account display name"}
	command.AddCommand(newAccountProfileUpdateCommand(runtime))
	return command
}

func newAccountProfileUpdateCommand(runtime *Runtime) *cobra.Command {
	var displayName string
	command := &cobra.Command{
		Use: "update", Short: "Update the current account display name", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			name := strings.TrimSpace(displayName)
			if name == "" {
				return output.Validation("ACCOUNT_DISPLAY_NAME_REQUIRED", "--display-name must not be blank")
			}
			if err := runtime.requireProfileWriteAuthentication(command.Context()); err != nil {
				return err
			}
			profile, err := runtime.client().UpdateAccountDisplayName(command.Context(), name)
			if err != nil {
				return err
			}
			return runtime.business(profile)
		},
	}
	command.Flags().StringVar(&displayName, "display-name", "", "new public display name")
	_ = command.MarkFlagRequired("display-name")
	return command
}

func newAccountAvatarCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "avatar", Short: "Manage the current account avatar"}
	command.AddCommand(newAccountAvatarUploadCommand(runtime))
	return command
}

func newAccountAvatarUploadCommand(runtime *Runtime) *cobra.Command {
	var filename string
	command := &cobra.Command{
		Use: "upload", Short: "Upload a PNG, JPEG, WebP, GIF, or AVIF account avatar", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			image, err := readAccountAvatar(filename)
			if err != nil {
				return err
			}
			if err := runtime.requireProfileWriteAuthentication(command.Context()); err != nil {
				return err
			}
			result, err := runtime.client().UploadAccountAvatar(command.Context(), filename, image)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&filename, "path", "", "path to an avatar image (maximum 2 MiB)")
	_ = command.MarkFlagRequired("path")
	return command
}

func readAccountAvatar(filename string) ([]byte, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, output.Validation("ACCOUNT_AVATAR_READ_FAILED", "could not read the avatar image").WithCause(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, output.Validation("ACCOUNT_AVATAR_READ_FAILED", "could not read the avatar image").WithCause(err)
	}
	if !info.Mode().IsRegular() {
		return nil, output.Validation("ACCOUNT_AVATAR_FILE_INVALID", "--path must identify a regular image file")
	}
	image, err := io.ReadAll(io.LimitReader(file, accountAvatarMaxBytes+1))
	if err != nil {
		return nil, output.Validation("ACCOUNT_AVATAR_READ_FAILED", "could not read the avatar image").WithCause(err)
	}
	if len(image) == 0 || len(image) > accountAvatarMaxBytes {
		return nil, output.Validation("ACCOUNT_AVATAR_SIZE_INVALID", "avatar image must be between 1 byte and 2 MiB")
	}
	return image, nil
}

func (runtime *Runtime) requireProfileWriteAuthentication(ctx context.Context) error {
	status, err := runtime.client().AuthStatus(ctx)
	if err != nil {
		return err
	}
	if !status.Authenticated {
		return output.Authentication("NOT_LOGGED_IN", "sign in before updating the account profile").
			WithHint("run 'viceme auth login' for the current profile").
			WithDetails(map[string]any{"profile": runtime.profile.Name, "apiBaseUrl": runtime.apiBaseURL})
	}
	for _, scope := range status.Scopes {
		if scope == "profile:write" {
			return nil
		}
	}
	return output.Authorization("PROFILE_WRITE_SCOPE_REQUIRED", "the current login is not authorized to update the account profile").
		WithHint("run 'viceme auth login' again for the current profile to grant profile update access").
		WithDetails(map[string]any{"profile": runtime.profile.Name, "missingScopes": []string{"profile:write"}})
}
