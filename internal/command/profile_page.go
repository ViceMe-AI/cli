package command

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/pagepackage"
	"github.com/ViceMe-AI/cli/internal/privatefile"
	"github.com/ViceMe-AI/cli/internal/privatepath"
	"github.com/ViceMe-AI/cli/internal/replicacontent"
	"github.com/spf13/cobra"
)

type profilePageUploadResult struct {
	Release    api.PageCustomizationRelease `json:"release"`
	ProfileURL string                       `json:"profileUrl"`
	SourcePath string                       `json:"sourcePath"`
	FileCount  int                          `json:"fileCount"`
}

type profilePageReleaseResult struct {
	Release    api.PageCustomizationRelease `json:"release"`
	ProfileURL string                       `json:"profileUrl"`
}

func newProfilePageCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "page", Short: "Publish and update the current user's personal profile page"}
	command.AddCommand(newProfilePageDescribeCommand(runtime))
	command.AddCommand(newMerchantPageInspectCommand(runtime))
	command.AddCommand(newProfilePageUploadCommand(runtime))
	command.AddCommand(newProfilePageStatusCommand(runtime))
	command.AddCommand(newProfilePageSourceCommand(runtime))
	command.AddCommand(newProfilePageReleaseCommand(runtime, "publish"))
	command.AddCommand(newProfilePageReleaseCommand(runtime, "activate"))
	return command
}

func newProfilePageDescribeCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use: "describe", Short: "Return the current user's exact profile URL and page capabilities", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := runtime.requireProfilePageAuthentication(command.Context(), false); err != nil {
				return err
			}
			description, err := runtime.client().DescribeProfilePageCustomization(command.Context())
			if err != nil {
				return err
			}
			if err := validateProfilePageDescription(description); err != nil {
				return err
			}
			return runtime.business(description)
		},
	}
}

func newProfilePageUploadCommand(runtime *Runtime) *cobra.Command {
	var source, editableSource, templateID, templateVersion string
	command := &cobra.Command{
		Use: "upload", Short: "Upload and validate the current user's personal profile without creating an online preview", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := runtime.requireProfilePageAuthentication(command.Context(), true); err != nil {
				return err
			}
			description, err := runtime.client().DescribeProfilePageCustomization(command.Context())
			if err != nil {
				return err
			}
			if err := validateProfilePageDescription(description); err != nil {
				return err
			}
			pkg, err := inspectPagePackageForTarget(source, description.Target)
			if err != nil {
				return err
			}
			snapshot, template, err := freezePageSource(description.Target, editableSource, templateID, templateVersion)
			if err != nil {
				return err
			}
			defer snapshot.Cleanup()
			release, err := uploadProfilePageCustomization(command.Context(), runtime, pkg, snapshot, template)
			if err != nil {
				return err
			}
			return runtime.business(profilePageUploadResult{
				Release: release, ProfileURL: description.ProfileURL,
				SourcePath: pkg.SourcePath, FileCount: pkg.FileCount,
			})
		},
	}
	command.Flags().StringVar(&source, "path", "", "personal profile page ZIP")
	command.Flags().StringVar(&editableSource, "source", "", "editable personal-profile project directory")
	command.Flags().StringVar(&templateID, "template-id", "", "source template ID")
	command.Flags().StringVar(&templateVersion, "template-version", "", "source template version")
	_ = command.MarkFlagRequired("path")
	_ = command.MarkFlagRequired("source")
	return command
}

func newProfilePageStatusCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use: "status", Short: "List the active and recent personal profile releases", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := runtime.requireProfilePageAuthentication(command.Context(), false); err != nil {
				return err
			}
			state, err := runtime.client().GetProfilePageCustomizationState(command.Context())
			if err != nil {
				return err
			}
			return runtime.business(state)
		},
	}
}

func newProfilePageSourceCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "source", Short: "Check or restore the editable personal profile source"}
	command.AddCommand(&cobra.Command{
		Use: "status", Short: "Check whether the active personal profile source can be restored", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := runtime.requireProfilePageAuthentication(command.Context(), false); err != nil {
				return err
			}
			status, err := runtime.client().GetProfilePageCustomizationSourceStatus(command.Context())
			if err != nil {
				return err
			}
			return runtime.business(status)
		},
	})
	command.AddCommand(newProfilePageSourceRestoreCommand(runtime))
	return command
}

func newProfilePageSourceRestoreCommand(runtime *Runtime) *cobra.Command {
	var destination string
	command := &cobra.Command{
		Use: "restore", Short: "Restore the active personal profile source into a new local directory", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := runtime.requireProfilePageAuthentication(command.Context(), false); err != nil {
				return err
			}
			status, err := runtime.client().GetProfilePageCustomizationSourceStatus(command.Context())
			if err != nil {
				return err
			}
			if status.Availability != "RESTORABLE" || status.Source == nil {
				return output.Validation("PAGE_SOURCE_NOT_RESTORABLE", "the active personal profile has no verified source snapshot")
			}
			archive, err := runtime.client().DownloadProfilePageCustomizationSource(command.Context(), status.Source.ReleaseID)
			if err != nil {
				return err
			}
			if int64(len(archive)) != status.Source.SizeBytes {
				return output.Internal("PAGE_SOURCE_SIZE_MISMATCH", "the downloaded personal profile source size does not match the verified snapshot", nil)
			}
			digest := sha256.Sum256(archive)
			if !strings.EqualFold(hex.EncodeToString(digest[:]), status.Source.Digest) {
				return output.Internal("PAGE_SOURCE_DIGEST_MISMATCH", "the downloaded personal profile source does not match the verified snapshot", nil)
			}
			temporaryDirectory, err := privatepath.CreateTempDirectory(os.TempDir(), ".viceme-profile-source-restore-*")
			if err != nil {
				return output.Internal("PAGE_SOURCE_TEMP_FAILED", "could not create a private temporary directory for personal profile source", err)
			}
			defer os.RemoveAll(temporaryDirectory)
			archivePath := filepath.Join(temporaryDirectory, "source.zip")
			if err := privatefile.Write(archivePath, archive, ".profile-source-*.tmp"); err != nil {
				return output.Internal("PAGE_SOURCE_WRITE_FAILED", "could not stage the personal profile source", err)
			}
			installed, err := replicacontent.RestoreOwnerSourceArchive(archivePath, destination)
			if err != nil {
				if errors.Is(err, os.ErrExist) {
					return output.Validation("PAGE_SOURCE_DESTINATION_EXISTS", "--destination must not already exist").WithCause(err)
				}
				return output.Validation("PAGE_SOURCE_RESTORE_FAILED", "personal profile source could not be safely restored").WithCause(err)
			}
			return runtime.business(pageSourceRestoreResult{
				Target: installed.Target, ReleaseID: status.Source.ReleaseID, ReleaseVersion: status.Source.ReleaseVersion,
				Digest: status.Source.Digest, ConcurrencyToken: status.ConcurrencyToken, Template: status.Source.Template,
				FileCount: installed.FileCount, ExpandedBytes: installed.ExpandedBytes,
			})
		},
	}
	command.Flags().StringVar(&destination, "destination", "", "new local directory for the restored editable project")
	_ = command.MarkFlagRequired("destination")
	return command
}

func newProfilePageReleaseCommand(runtime *Runtime, action string) *cobra.Command {
	var expectedActive, expectedConcurrency string
	short := "Publish an exact validated personal profile release"
	if action == "activate" {
		short = "Roll back by activating an exact historical personal profile release"
	}
	command := &cobra.Command{
		Use: action + " <release-id>", Short: short, Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if !pageUUIDPattern.MatchString(strings.ToLower(args[0])) {
				return output.Validation("PAGE_RELEASE_ID_INVALID", "release ID must be a UUID")
			}
			expected, err := parseExpectedActiveRelease(expectedActive)
			if err != nil {
				return err
			}
			expectedConcurrency = strings.ToLower(strings.TrimSpace(expectedConcurrency))
			if len(expectedConcurrency) != sha256.Size*2 {
				return output.Validation("PAGE_CONCURRENCY_TOKEN_INVALID", "--expected-concurrency must be a SHA-256 concurrency token")
			}
			if _, err := hex.DecodeString(expectedConcurrency); err != nil {
				return output.Validation("PAGE_CONCURRENCY_TOKEN_INVALID", "--expected-concurrency must be a SHA-256 concurrency token").WithCause(err)
			}
			if err := runtime.requireProfilePageAuthentication(command.Context(), true); err != nil {
				return err
			}
			description, err := runtime.client().DescribeProfilePageCustomization(command.Context())
			if err != nil {
				return err
			}
			if err := validateProfilePageDescription(description); err != nil {
				return err
			}
			release, err := runtime.client().PublishProfilePageCustomization(
				command.Context(), args[0], expected, expectedConcurrency, action,
			)
			if err != nil {
				return err
			}
			return runtime.business(profilePageReleaseResult{Release: release, ProfileURL: description.ProfileURL})
		},
	}
	command.Flags().StringVar(&expectedActive, "expected-active", "", "currently active release UUID, or 'none'")
	command.Flags().StringVar(&expectedConcurrency, "expected-concurrency", "", "concurrency token returned by profile page status")
	_ = command.MarkFlagRequired("expected-active")
	_ = command.MarkFlagRequired("expected-concurrency")
	return command
}

func uploadProfilePageCustomization(ctx context.Context, runtime *Runtime, pkg pagepackage.Package, source *replicacontent.FrozenSourceArchive, template *api.PageCustomizationTemplateReference) (api.PageCustomizationRelease, error) {
	client := runtime.client()
	created, err := client.CreateProfilePageCustomizationDraft(ctx, api.CreateProfilePageCustomizationDraftRequest{
		ClientRequestID: runtime.deps.NewID(), ContractVersion: api.ProfilePageCustomizationContractVersion,
		CLIVersion: buildinfo.Version, Artifact: pkg.Artifact,
		SourceSnapshot: &api.PageCustomizationSourceSnapshotInput{
			Artifact: api.PageCustomizationArtifact{
				Digest: source.Summary.Digest, SizeBytes: source.Summary.SizeBytes,
				FileName: "personal-profile-source.zip", ContentType: "application/zip",
			},
			Template: template,
		},
	})
	if err != nil {
		return api.PageCustomizationRelease{}, err
	}
	authorization, err := client.AuthorizeProfilePageCustomizationUpload(ctx, created.Release.ID)
	if err != nil {
		return api.PageCustomizationRelease{}, err
	}
	if err := client.PutPresigned(ctx, authorization.UploadURL, authorization.Headers, bytes.NewReader(pkg.Bytes), int64(len(pkg.Bytes))); err != nil {
		return api.PageCustomizationRelease{}, err
	}
	sourceAuthorization, err := client.AuthorizeProfilePageCustomizationSourceUpload(ctx, created.Release.ID)
	if err != nil {
		return api.PageCustomizationRelease{}, err
	}
	file, info, err := source.Open()
	if err != nil {
		return api.PageCustomizationRelease{}, output.Internal("PAGE_SOURCE_OPEN_FAILED", "could not open the frozen personal profile source", err)
	}
	defer func() { _ = file.Close() }()
	if err := client.PutPresigned(ctx, sourceAuthorization.UploadURL, sourceAuthorization.Headers, file, info.Size()); err != nil {
		return api.PageCustomizationRelease{}, err
	}
	return client.CompleteProfilePageCustomizationUpload(ctx, created.Release.ID)
}

func validateProfilePageDescription(description api.ProfilePageCustomizationTargetDescription) error {
	if description.Target.Type != "CREATOR" || description.Target.CreatorHandle == "" || description.Target.WorkSlug != "" || description.ProfileURL == "" || description.MarkdownURL == "" {
		return output.Internal("PROFILE_PAGE_TARGET_INVALID", "ViceMe API returned an invalid personal profile target", nil)
	}
	return nil
}

func (runtime *Runtime) requireProfilePageAuthentication(ctx context.Context, write bool) error {
	status, err := runtime.client().AuthStatus(ctx)
	if err != nil {
		return err
	}
	if !status.Authenticated {
		return output.Authentication("NOT_LOGGED_IN", "sign in before managing the personal profile page").
			WithHint("run 'viceme auth login' for the current profile")
	}
	required := []string{"profile:read"}
	if write {
		required = append(required, "profile:write")
	}
	available := make(map[string]struct{}, len(status.Scopes))
	for _, scope := range status.Scopes {
		available[scope] = struct{}{}
	}
	missing := make([]string, 0, len(required))
	for _, scope := range required {
		if _, ok := available[scope]; !ok {
			missing = append(missing, scope)
		}
	}
	if len(missing) != 0 {
		return output.Authorization("PROFILE_PAGE_SCOPE_REQUIRED", "the current login is not authorized to manage the personal profile page").
			WithHint("run 'viceme auth login' again for the current profile to grant profile access").
			WithDetails(map[string]any{"profile": runtime.profile.Name, "missingScopes": missing})
	}
	return nil
}
