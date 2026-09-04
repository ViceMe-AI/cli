package command

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"regexp"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/replicacontent"
	"github.com/spf13/cobra"
)

var (
	replicaUUIDPattern      = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	replicaShortCodePattern = regexp.MustCompile(`^VMR-[A-Z0-9]{20}$`)
)

type replicaInspectResult struct {
	NextAction                  string                       `json:"nextAction"`
	WorkURL                     string                       `json:"workUrl"`
	StandaloneRecoveryAvailable bool                         `json:"standaloneRecoveryAvailable"`
	Replica                     api.WebsiteReplicaResolution `json:"replica"`
}

func newReplicaCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "replica", Short: "Publish and install Website Replica source packages"}
	command.AddCommand(newReplicaPreviewCommand(runtime))
	command.AddCommand(newReplicaPublishCommand(runtime))
	command.AddCommand(newReplicaInspectCommand(runtime))
	command.AddCommand(newReplicaStatusCommand(runtime))
	command.AddCommand(newReplicaResumeCommand(runtime))
	command.AddCommand(newReplicaCancelCommand(runtime))
	command.AddCommand(newReplicaInstallCommand(runtime))
	return command
}

func newReplicaInspectCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect <replica-code>",
		Short: "Inspect a Website Replica and return its public Work preview",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if _, err := parseReplicaCode(args[0]); err != nil {
				return err
			}
			resolved, err := runtime.client().ResolveWebsiteReplicaPublic(command.Context(), args[0])
			if err != nil {
				return replicaInspectFailure(err)
			}
			recoveryAvailable, err := standaloneReplicaRecoveryAvailable(command.Context(), runtime, resolved)
			if err != nil {
				return replicaInspectFailure(err)
			}
			return runtime.business(replicaInspectResult{
				NextAction: "CONFIRM_INLINE_PREVIEW", WorkURL: resolved.ViceMeWorkURL,
				StandaloneRecoveryAvailable: recoveryAvailable, Replica: resolved,
			})
		},
	}
}

func replicaInspectFailure(err error) error {
	failure := *output.AsError(err)
	failure.Retryable = false
	failure.Details = map[string]any{
		"nextAction": "STOP_AND_REPORT",
		"stage":      "INSPECT_REPLICA",
	}
	failure.Hint = "report that the selected ViceMe service could not inspect the Work; do not retry or diagnose local services"
	return &failure
}

func replicaSourceArchiveError(err error) error {
	switch {
	case errors.Is(err, replicacontent.ErrSensitiveContent):
		return output.Validation("REPLICA_SENSITIVE_CONTENT", "Website Replica source contains suspected credentials or user data").WithCause(err)
	case errors.Is(err, replicacontent.ErrForbiddenReplicaContent):
		return output.Validation("REPLICA_FORBIDDEN_CONTENT", "Website Replica source contains platform-controlled Replica content").WithCause(err)
	case errors.Is(err, replicacontent.ErrProjectHandoff):
		return output.Validation(
			"REPLICA_DEPLOYMENT_GUIDE_INVALID",
			fmt.Sprintf("Website Replica ZIP must contain a valid UTF-8 root %s no larger than %d bytes", replicacontent.ProjectHandoffFile, replicacontent.MaxProjectHandoffBytes),
		).WithCause(err)
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, fs.ErrPermission):
		return output.Validation("REPLICA_ARCHIVE_READ_FAILED", "could not read the Website Replica source").WithCause(err)
	default:
		return output.Validation("REPLICA_ARCHIVE_INVALID", "Website Replica source is not a safe readable project directory or ZIP archive").WithCause(err)
	}
}

func (runtime *Runtime) requireWebsiteReplicaAuthentication(ctx context.Context, required ...string) error {
	status, err := runtime.client().AuthStatus(ctx)
	if err != nil {
		return err
	}
	if !status.Authenticated {
		return output.Authentication("NOT_LOGGED_IN", "sign in before using Website Replicas").
			WithHint("run 'viceme auth login' for the current profile")
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
		return output.Authorization("REPLICA_SCOPE_REQUIRED", "the current login is not authorized for this Website Replica operation").
			WithHint("run 'viceme auth login' again for the current profile to grant Website Replica access").
			WithDetails(map[string]any{"profile": runtime.profile.Name, "missingScopes": missing})
	}
	return nil
}
