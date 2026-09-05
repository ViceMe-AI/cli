package command

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/replicapreview"
	"github.com/spf13/cobra"
)

type replicaPreviewResult struct {
	Mode                        string                     `json:"mode"`
	PreviewURL                  string                     `json:"previewUrl,omitempty"`
	TargetURL                   string                     `json:"targetUrl,omitempty"`
	PreviewShellOpened          bool                       `json:"previewShellOpened"`
	PreviewVerified             bool                       `json:"previewVerified"`
	BrowserVerificationRequired bool                       `json:"browserVerificationRequired"`
	HostingRequested            bool                       `json:"hostingRequested"`
	RemoteUpload                bool                       `json:"remoteUpload"`
	AuthenticationChecked       bool                       `json:"authenticationChecked"`
	MerchantChecked             bool                       `json:"merchantChecked"`
	PublicationCreated          bool                       `json:"publicationCreated"`
	ReusedLocalService          bool                       `json:"reusedLocalService,omitempty"`
	StartedByCLI                bool                       `json:"startedByCli,omitempty"`
	Performance                 *replicaPreviewPerformance `json:"performance,omitempty"`
}

type replicaPreviewPerformance struct {
	Applicable             bool   `json:"applicable"`
	GoalMilliseconds       int64  `json:"goalMilliseconds"`
	ReadyAfterMilliseconds int64  `json:"readyAfterMilliseconds"`
	ExcludedReason         string `json:"excludedReason,omitempty"`
}

func newReplicaPreviewCommand(runtime *Runtime) *cobra.Command {
	var projectPath, existingURL string
	var confirmReplicaOnly bool
	command := &cobra.Command{
		Use:   "preview",
		Short: "Preview the official make-a-copy experience around a local website",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if confirmReplicaOnly {
				if projectPath != "" || existingURL != "" {
					return output.Validation("REPLICA_PREVIEW_OPTIONS_INVALID", "--confirm-unverified-replica-only cannot be combined with --path or --url")
				}
				return runtime.business(replicaPreviewResult{
					Mode: "REPLICA_ONLY", PreviewVerified: false, HostingRequested: false,
					RemoteUpload: false, AuthenticationChecked: false, MerchantChecked: false, PublicationCreated: false,
				})
			}
			if projectPath != "" && existingURL != "" {
				return output.Validation("REPLICA_PREVIEW_OPTIONS_INVALID", "--path and --url cannot be used together")
			}
			return runReplicaPreview(command.Context(), runtime, projectPath, existingURL)
		},
	}
	command.Flags().StringVar(&projectPath, "path", "", "legacy project context; provide --url to preview a service started by your agent")
	command.Flags().StringVar(&existingURL, "url", "", "running HTTP(S) loopback service to reuse")
	command.Flags().BoolVar(&confirmReplicaOnly, "confirm-unverified-replica-only", false, "explicitly continue without verified hosting preview; performs no publication or upload")
	return command
}

func runReplicaPreview(parent context.Context, runtime *Runtime, projectPath, existingURL string) error {
	ctx, stopSignals := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	session, err := runtime.deps.StartReplicaPreview(ctx, replicapreview.Options{
		ExistingURL: existingURL,
		ProjectPath: projectPath,
		ErrOut:      runtime.deps.ErrOut,
		Report: func(event replicapreview.Event) {
			_, _ = fmt.Fprintf(runtime.deps.ErrOut, "%s...\n", event.Message)
		},
	})
	if err != nil {
		return replicaPreviewBoundaryError(err)
	}

	result := session.Result()
	previewURL, err := replicaPreviewShellURL(runtime.profile.ResolvedWebBaseURL(), result.TargetURL)
	if err != nil {
		return finishReplicaPreview(session, err)
	}
	if err := runtime.deps.OpenURL(ctx, previewURL); err != nil {
		return finishReplicaPreview(session, fmt.Errorf("open the ViceMe preview shell: %w", err))
	}
	if result.StartedByCLI {
		_, _ = fmt.Fprintln(runtime.deps.ErrOut, "Local preview is open. Press Ctrl+C to stop the dev server owned by ViceMe.")
	}
	if err := session.Wait(ctx); err != nil {
		return finishReplicaPreview(session, err)
	}
	if err := finishReplicaPreview(session, nil); err != nil {
		return err
	}

	return runtime.business(replicaPreviewResult{
		Mode:                        "LOCAL_PREVIEW",
		PreviewURL:                  previewURL,
		TargetURL:                   result.TargetURL,
		PreviewShellOpened:          true,
		PreviewVerified:             false,
		BrowserVerificationRequired: true,
		HostingRequested:            false,
		RemoteUpload:                false,
		AuthenticationChecked:       false,
		MerchantChecked:             false,
		PublicationCreated:          false,
		ReusedLocalService:          result.Reused,
		StartedByCLI:                result.StartedByCLI,
		Performance: &replicaPreviewPerformance{
			Applicable:             result.Performance.Applicable,
			GoalMilliseconds:       result.Performance.Goal.Milliseconds(),
			ReadyAfterMilliseconds: result.Performance.ReadyAfter.Milliseconds(),
			ExcludedReason:         result.Performance.ExcludedReason,
		},
	})
}

func finishReplicaPreview(session replicapreview.Running, operationErr error) error {
	if closeErr := session.Close(); closeErr != nil {
		return replicaPreviewBoundaryError(&replicapreview.StartError{
			Code:    "REPLICA_PREVIEW_CLEANUP_FAILED",
			Stage:   replicapreview.StageStarting,
			Message: "the CLI-owned dev server could not be stopped",
			Cause:   errors.Join(operationErr, closeErr),
		})
	}
	if operationErr != nil {
		return replicaPreviewBoundaryError(operationErr)
	}
	return nil
}

func replicaPreviewShellURL(webBaseURL, target string) (string, error) {
	if strings.TrimSpace(webBaseURL) == "" {
		return "", errors.New("the active Profile does not define a Web base URL")
	}
	parsed, err := url.Parse(webBaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("the active Profile Web base URL is invalid")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/website-replica/preview"
	parsed.RawPath = ""
	query := parsed.Query()
	query.Set("target", target)
	parsed.RawQuery = query.Encode()
	parsed.Fragment = ""
	return parsed.String(), nil
}

func replicaPreviewBoundaryError(err error) *output.Error {
	var existing *output.Error
	if errors.As(err, &existing) {
		return existing
	}
	details := map[string]any{
		"previewVerified":       false,
		"remoteUpload":          false,
		"authenticationChecked": false,
		"merchantChecked":       false,
		"publicationCreated":    false,
		"unverifiedBoundaries":  []string{"preview interaction", "responsive layout", "local page embedding"},
	}
	var startErr *replicapreview.StartError
	if errors.As(err, &startErr) {
		details["previewErrorCode"] = startErr.Code
		details["previewStage"] = startErr.Stage
		if startErr.Code == "REPLICA_PREVIEW_URL_REQUIRED" {
			details["nextAction"] = "PROVIDE_PREVIEW_URL"
			return output.Validation(startErr.Code, startErr.Message).WithDetails(details).WithHint("have your agent select and start the actual page, then supply --url to preview or --preview-url to publish")
		}
		// Messages are fixed runtime descriptions; never expose raw causes or URLs.
		details["previewReason"] = startErr.Message
	}
	return output.Confirmation(
		"CONFIRM_UNVERIFIED_REPLICA_ONLY",
		"preview interaction, responsive layout, and local page embedding could not be verified; no source was uploaded",
	).WithDetails(details).WithHint(
		"fix the reported preview problem and retry, or explicitly continue with 'viceme replica preview --confirm-unverified-replica-only'",
	).WithCause(err)
}
