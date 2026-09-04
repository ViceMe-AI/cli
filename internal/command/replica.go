package command

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/replicacontent"
	"github.com/spf13/cobra"
)

var (
	replicaUUIDPattern      = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	replicaShortCodePattern = regexp.MustCompile(`^VMR-[A-Z0-9]{20}$`)
)

type replicaPublishResult struct {
	ReplicaID     string                              `json:"replicaId"`
	ReplicaCode   string                              `json:"replicaCode"`
	BuyerEntry    api.WebsiteReplicaBuyerEntry        `json:"buyerEntry"`
	SourceArchive replicacontent.SourceArchiveSummary `json:"sourceArchive"`
}

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
				NextAction: "OPEN_WORK_PREVIEW", WorkURL: resolved.ViceMeWorkURL,
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

func newReplicaPublishCommand(runtime *Runtime) *cobra.Command {
	var source, workID, title, summary string
	var priceCents int
	command := &cobra.Command{
		Use:   "publish",
		Short: "Freeze and upload Website Replica source, then return its stable sharing code",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := publishReplica(command.Context(), runtime, source, workID, title, summary, priceCents)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&source, "path", "", "Website Replica project directory or existing ZIP path")
	command.Flags().StringVar(&workID, "work-id", "", "Website Work UUID")
	command.Flags().StringVar(&title, "title", "", "buyer-visible Replica title")
	command.Flags().StringVar(&summary, "summary", "", "buyer-visible Replica summary")
	command.Flags().IntVar(&priceCents, "price-cents", 0, "price in the market currency's minor unit")
	_ = command.MarkFlagRequired("path")
	_ = command.MarkFlagRequired("work-id")
	_ = command.MarkFlagRequired("title")
	_ = command.MarkFlagRequired("price-cents")
	return command
}

func publishReplica(ctx context.Context, runtime *Runtime, source, workID, title, summary string, priceCents int) (replicaPublishResult, error) {
	workID = strings.TrimSpace(workID)
	title = strings.TrimSpace(title)
	summary = strings.TrimSpace(summary)
	if !replicaUUIDPattern.MatchString(workID) {
		return replicaPublishResult{}, output.Validation("REPLICA_WORK_ID_INVALID", "--work-id must be a UUID")
	}
	if title == "" {
		return replicaPublishResult{}, output.Validation("REPLICA_METADATA_INVALID", "--title cannot be empty")
	}
	if priceCents < 0 || priceCents > 10_000_000 {
		return replicaPublishResult{}, output.Validation("REPLICA_PRICE_INVALID", "--price-cents must be between 0 and 10000000")
	}
	frozen, err := replicacontent.FreezeSourceArchive(source, replicacontent.FreezeSourceOptions{Purpose: summary})
	if err != nil {
		return replicaPublishResult{}, replicaSourceArchiveError(err)
	}
	defer frozen.Cleanup()
	if err := runtime.requireWebsiteReplicaAuthentication(ctx, "website-replica:read", "website-replica:write"); err != nil {
		return replicaPublishResult{}, err
	}
	clientRequestID := runtime.deps.NewID()
	if !replicaUUIDPattern.MatchString(clientRequestID) {
		return replicaPublishResult{}, output.Internal("REPLICA_CLIENT_REQUEST_ID_INVALID", "could not create a valid Replica request identity", nil)
	}
	created, err := runtime.client().CreateWebsiteReplicaUpload(ctx, api.CreateWebsiteReplicaUploadRequest{
		ClientRequestID: clientRequestID,
		WorkID:          workID,
		Title:           title,
		Summary:         summary,
		FileName:        filepath.Base(frozen.Path()),
		SizeBytes:       frozen.Summary.SizeBytes,
		Digest:          frozen.Summary.Digest,
		PriceCents:      priceCents,
	})
	if err != nil {
		return replicaPublishResult{}, err
	}
	file, info, err := frozen.Open()
	if err != nil {
		return replicaPublishResult{}, output.Internal("REPLICA_ARCHIVE_SNAPSHOT_INVALID", "the frozen Website Replica ZIP is no longer available", err)
	}
	defer file.Close()
	progress(runtime, "Uploading Website Replica source package")
	if err := runtime.client().PutUpload(ctx, api.UploadAuthorization{
		Method: created.Upload.Method, URL: created.Upload.URL,
		ExpiresAt: created.Upload.ExpiresAt, Headers: created.Upload.Headers,
	}, file, info.Size()); err != nil {
		return replicaPublishResult{}, err
	}
	completed, err := runtime.client().CompleteWebsiteReplicaUpload(ctx, created.ReplicaID, created.UploadID)
	if err != nil {
		return replicaPublishResult{}, err
	}
	if !replicaPublicationMatchesRequest(completed, created.ReplicaID, title, priceCents) {
		return replicaPublishResult{}, invalidReplicaResponse("Website Replica completion does not match the publication request")
	}
	return replicaPublishResult{
		ReplicaID: created.ReplicaID, ReplicaCode: "VICEME-REPLICA:" + completed.ShortCode,
		BuyerEntry: completed.BuyerEntry, SourceArchive: frozen.Summary,
	}, nil
}

func replicaPublicationMatchesRequest(completed api.CompleteWebsiteReplicaUploadResponse, replicaID, title string, priceCents int) bool {
	return completed.ReplicaID == replicaID && replicaShortCodePattern.MatchString(completed.ShortCode) &&
		completed.Product.Title == title && completed.Product.Currency == "CNY" && completed.Product.PriceCents == priceCents
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
