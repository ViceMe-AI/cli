package command

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ViceMe-AI/cli/internal/api"
	archivepkg "github.com/ViceMe-AI/cli/internal/archive"
	"github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

const maxStdinBytes = 1 << 20

func newSkillCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "skill", Short: "Inspect and publish external Skills"}
	command.AddCommand(newSkillInspectCommand(runtime))
	command.AddCommand(newSkillPublishCommand(runtime))
	command.AddCommand(newTargetCommand(runtime))
	command.AddCommand(newDelegatedGrantCommand(runtime))
	return command
}

func newSkillInspectCommand(runtime *Runtime) *cobra.Command {
	var expressionStdin bool
	var dryRun bool
	command := &cobra.Command{
		Use:   "inspect [source]",
		Short: "Resolve an immutable external Skill snapshot without publishing it",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 1 {
				return output.Validation("source_count", "inspect accepts at most one source argument")
			}
			return nil
		},
		RunE: func(command *cobra.Command, args []string) error {
			if expressionStdin == (len(args) == 1) {
				return output.Validation("source_required", "provide exactly one source argument or --expression-stdin")
			}
			if dryRun {
				mode := "argument"
				if expressionStdin {
					mode = "stdin"
				}
				return runtime.success(map[string]any{"dry_run": true, "operation": "skill.inspect", "source_mode": mode})
			}
			expression := ""
			if expressionStdin {
				value, err := readLimited(runtime.deps.In, maxStdinBytes)
				if err != nil {
					return err
				}
				expression = value
			} else {
				expression = args[0]
			}
			if strings.TrimSpace(expression) == "" {
				return output.Validation("source_empty", "source expression cannot be empty")
			}
			response, err := runtime.client().Inspect(command.Context(), api.InspectRequest{Source: api.Source{Kind: "expression", Value: expression}})
			if err != nil {
				return err
			}
			return runtime.success(response)
		},
	}
	command.Flags().BoolVar(&expressionStdin, "expression-stdin", false, "read a copied provider expression from stdin")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "validate and print the operation without calling Viceme")
	return command
}

type publishOptions struct {
	resolutionID          string
	expressionStdin       bool
	file                  string
	directory             string
	skillRoot             string
	newTarget             bool
	targetAlias           string
	targetAliasSet        bool
	targetID              string
	expectedTargetVersion int64
	expectedVersionSet    bool
	targetLocale          string
	yes                   bool
	wait                  bool
	timeout               time.Duration
	dryRun                bool
	clientRequestID       string
	delegatedGrantStdin   bool
	delegatedGrantRef     string
}

func newSkillPublishCommand(runtime *Runtime) *cobra.Command {
	var opts publishOptions
	command := &cobra.Command{
		Use:   "publish [source]",
		Short: "Create a durable Skill Agent publication",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 1 {
				return output.Validation("source_count", "publish accepts at most one source argument")
			}
			return nil
		},
		RunE: func(command *cobra.Command, args []string) error {
			opts.expectedVersionSet = command.Flags().Changed("expected-target-version")
			opts.targetAliasSet = command.Flags().Changed("target-alias")
			if err := validatePublishOptions(args, opts); err != nil {
				return err
			}
			destination := publishDestination(opts)
			if opts.dryRun {
				result := map[string]any{
					"dry_run":            true,
					"operation":          "skill.publish",
					"source_mode":        publishSourceMode(args, opts),
					"destination":        destination,
					"wait":               opts.wait,
					"confirmation_ok":    opts.yes,
					"publish_mode":       "confirm",
					"confirmation_scope": "publication_admission/v1",
					"ownership_mode":     publishOwnershipMode(opts),
				}
				if source := delegatedCredentialSource(opts); source != "" {
					result["delegated_credential_source"] = source
				}
				return runtime.success(result)
			}
			delegated := opts.delegatedGrantStdin || opts.delegatedGrantRef != ""
			request := api.CreatePublicationRequest{
				ClientRequestID: opts.clientRequestID,
				ResolutionID:    opts.resolutionID,
				Selector:        opts.skillRoot,
				Destination:     destination,
				Options: api.PublicationOptions{
					TargetLocale:          opts.targetLocale,
					PublishMode:           "confirm",
					AdmissionConfirmation: true,
				},
			}
			var delegatedIntentFingerprint string
			var grantManager *auth.DelegatedGrantManager
			var err error
			if delegated && opts.resolutionID == "" {
				source, err := expressionPublicationSource(runtime, args, opts)
				if err != nil {
					return err
				}
				delegatedIntentFingerprint, err = delegatedPublicationIntentFingerprint(request, &source)
				if err != nil {
					return err
				}
				grantManager = delegatedGrantManager(runtime)
				resume, err := grantManager.PeekPublication(opts.delegatedGrantRef, delegatedIntentFingerprint)
				if err != nil {
					return err
				}
				if resume.Bound {
					request.ClientRequestID = resume.ClientRequestID
					request.ResolutionID = resume.ResolutionID
					request.Selector = resume.Selector
				} else {
					inspection, inspectErr := runtime.client().Inspect(command.Context(), api.InspectRequest{Source: source})
					if inspectErr != nil {
						return inspectErr
					}
					resolutionID, selector, selectionErr := selectInspectedCandidate(inspection, opts.skillRoot)
					if selectionErr != nil {
						return selectionErr
					}
					request.ResolutionID = resolutionID
					request.Selector = selector
				}
			} else if opts.resolutionID == "" {
				source, err := publicationSource(command, runtime, args, opts)
				if err != nil {
					return err
				}
				request.Source = &source
			}
			if request.ClientRequestID == "" && opts.delegatedGrantRef == "" {
				request.ClientRequestID = runtime.deps.NewID()
			}
			delegatedGrantCredential := ""
			var delegatedLease auth.DelegatedPublicationLease
			if delegated {
				if opts.delegatedGrantRef != "" {
					if grantManager == nil {
						grantManager = delegatedGrantManager(runtime)
					}
					if delegatedIntentFingerprint == "" {
						delegatedIntentFingerprint, err = delegatedPublicationIntentFingerprint(request, nil)
						if err != nil {
							return err
						}
					}
					requestFingerprint, fingerprintErr := delegatedPublicationRequestFingerprint(request)
					if fingerprintErr != nil {
						return fingerprintErr
					}
					delegatedLease, err = grantManager.BeginPublication(opts.delegatedGrantRef, auth.DelegatedPublicationBinding{
						IntentFingerprint:  delegatedIntentFingerprint,
						RequestFingerprint: requestFingerprint,
						ResolutionID:       request.ResolutionID,
						Selector:           request.Selector,
					})
					if err != nil {
						return err
					}
					delegatedGrantCredential = delegatedLease.Credential
					request.ClientRequestID = delegatedLease.ClientRequestID
					request.ResolutionID = delegatedLease.ResolutionID
					request.Selector = delegatedLease.Selector
				} else {
					delegatedGrantCredential, err = readDelegatedGrantStdin(runtime)
					if err != nil {
						return err
					}
					delegatedGrantCredential, err = auth.NormalizeDelegatedGrantCredential(delegatedGrantCredential)
					if err != nil {
						return err
					}
				}
			}
			publication, err := createPublication(command.Context(), runtime, request, delegatedGrantCredential)
			if err != nil {
				return err
			}
			if delegatedGrantCredential != "" {
				if err := validateDelegatedPublicationReceipt(publication); err != nil {
					return err
				}
			}
			cleanupDelegatedGrantReference(runtime, opts, publication, delegatedLease)
			if !opts.wait {
				return runtime.success(publication)
			}
			publicationID := publication.ID()
			if publicationID == "" {
				return output.Internal("publication_response", "Viceme API did not return a publication_id", nil)
			}
			final, timedOut, err := waitPublication(command.Context(), runtime, publicationID, opts.timeout)
			if err != nil {
				return err
			}
			carryDelegatedPublicationMetadata(final, publication)
			meta := runtime.meta
			if timedOut {
				value := true
				meta.WaitTimedOut = &value
			}
			return runtime.successWithMeta(final, meta)
		},
	}
	flags := command.Flags()
	flags.StringVar(&opts.resolutionID, "resolution-id", "", "publish an immutable snapshot returned by inspect")
	flags.BoolVar(&opts.expressionStdin, "expression-stdin", false, "read a copied provider expression from stdin")
	flags.StringVar(&opts.file, "file", "", "upload a Skill archive")
	flags.StringVar(&opts.directory, "dir", "", "deterministically archive and upload a Skill directory")
	flags.StringVar(&opts.skillRoot, "skill-root", "", "select a Skill root within the immutable source")
	flags.BoolVar(&opts.newTarget, "new-target", false, "create a new logical Agent Target")
	flags.StringVar(&opts.targetAlias, "target-alias", "", "owner-scoped alias for a new Target")
	flags.StringVar(&opts.targetID, "target-id", "", "publish to an existing Target")
	flags.Int64Var(&opts.expectedTargetVersion, "expected-target-version", 0, "expected CAS version of an existing Target")
	flags.StringVar(&opts.targetLocale, "target-locale", "", "target locale for compiled instructions")
	flags.BoolVar(&opts.yes, "yes", false, "confirm the public publication side effect")
	flags.BoolVar(&opts.wait, "wait", false, "wait for a bounded publication result")
	flags.DurationVar(&opts.timeout, "timeout", 60*time.Second, "maximum wait duration")
	flags.BoolVar(&opts.dryRun, "dry-run", false, "validate and print the operation without reading input or calling Viceme")
	flags.StringVar(&opts.clientRequestID, "client-request-id", "", "idempotency key; required with --delegated-grant-stdin")
	flags.BoolVar(&opts.delegatedGrantStdin, "delegated-grant-stdin", false, "read a grant from protected non-TTY stdin; requires --resolution-id and --client-request-id")
	flags.StringVar(&opts.delegatedGrantRef, "delegated-grant-ref", "", "publish with a keychain grant and persisted exact-request recovery state")
	return command
}

func validateDelegatedPublicationReceipt(publication api.Publication) error {
	receiptID, ok := publication["delegated_grant_receipt_id"].(string)
	if !ok || strings.TrimSpace(receiptID) == "" || strings.TrimSpace(receiptID) != receiptID {
		return output.Internal(
			"delegated_grant_receipt_missing",
			"Viceme accepted the request without returning a delegated grant receipt; the local credential was retained",
			nil,
		)
	}
	return nil
}

func cleanupDelegatedGrantReference(runtime *Runtime, opts publishOptions, publication api.Publication, lease auth.DelegatedPublicationLease) {
	if opts.delegatedGrantRef == "" {
		return
	}
	if err := delegatedGrantManager(runtime).CompletePublication(
		opts.delegatedGrantRef,
		lease.ClientRequestID,
		lease.RequestFingerprint,
	); err != nil {
		publication["delegated_credential_cleanup"] = "required"
		publication["delegated_credential_ref"] = opts.delegatedGrantRef
		return
	}
	publication["delegated_credential_cleanup"] = "deleted"
}

func carryDelegatedPublicationMetadata(destination, source api.Publication) {
	for _, key := range []string{
		"delegated_grant_receipt_id",
		"delegated_credential_cleanup",
		"delegated_credential_ref",
	} {
		if value, ok := source[key]; ok {
			destination[key] = value
		}
	}
}

func createPublication(ctx context.Context, runtime *Runtime, request api.CreatePublicationRequest, delegatedGrantCredential string) (api.Publication, error) {
	client := runtime.client()
	publication, err := client.CreatePublicationWithDelegatedGrant(ctx, request, delegatedGrantCredential)
	if err == nil {
		return publication, nil
	}
	var cliError *output.Error
	if !errors.As(err, &cliError) || cliError.Type != "network" || !cliError.Retryable {
		return nil, err
	}
	if sleepErr := runtime.deps.Sleep(ctx, 250*time.Millisecond); sleepErr != nil {
		return nil, err
	}
	// Reuse the exact request and client_request_id. The server owns the
	// idempotency receipt, so an ambiguous transport failure cannot create a
	// second publication.
	return client.CreatePublicationWithDelegatedGrant(ctx, request, delegatedGrantCredential)
}

func validatePublishOptions(args []string, opts publishOptions) error {
	sources := 0
	if len(args) == 1 {
		sources++
	}
	for _, present := range []bool{opts.resolutionID != "", opts.expressionStdin, opts.file != "", opts.directory != ""} {
		if present {
			sources++
		}
	}
	if sources != 1 {
		return output.Validation("source_required", "provide exactly one source, --resolution-id, --expression-stdin, --file, or --dir")
	}
	if opts.delegatedGrantStdin && opts.delegatedGrantRef != "" {
		return output.Validation("delegated_grant_source", "--delegated-grant-stdin and --delegated-grant-ref are mutually exclusive")
	}
	if opts.delegatedGrantStdin && opts.expressionStdin {
		return output.Validation("stdin_conflict", "source expression and delegated grant cannot both read from stdin; store the grant and use --delegated-grant-ref")
	}
	if opts.delegatedGrantRef != "" {
		if err := auth.ValidateDelegatedGrantReference(opts.delegatedGrantRef); err != nil {
			return err
		}
		if opts.clientRequestID != "" {
			return output.Validation("client_request_id_managed", "--delegated-grant-ref manages its stable client request id; do not also pass --client-request-id")
		}
	}
	if opts.delegatedGrantStdin && opts.clientRequestID == "" && !opts.dryRun {
		return output.Validation("client_request_id_required", "--delegated-grant-stdin requires --client-request-id so an ambiguous create can be retried safely")
	}
	if opts.delegatedGrantStdin && opts.resolutionID == "" && !opts.dryRun {
		return output.Validation("resolution_id_required", "--delegated-grant-stdin requires --resolution-id so retries reuse the exact immutable request")
	}
	if (opts.delegatedGrantStdin || opts.delegatedGrantRef != "") && (opts.file != "" || opts.directory != "") {
		return output.Validation("delegated_upload_unsupported", "delegated publication requires an inspected immutable provider resolution; file and directory upload are not supported")
	}
	if opts.clientRequestID != "" && (strings.TrimSpace(opts.clientRequestID) != opts.clientRequestID || len(opts.clientRequestID) > 128) {
		return output.Validation("client_request_id_invalid", "--client-request-id must be 1 to 128 non-whitespace characters")
	}
	if !opts.yes && !opts.dryRun {
		return output.Confirmation("confirmation_required", "publishing creates or updates a public share link; rerun with --yes after user confirmation")
	}
	if opts.timeout <= 0 {
		return output.Validation("timeout", "--timeout must be greater than zero")
	}
	if opts.newTarget && opts.targetID != "" {
		return output.Validation("target_flags", "--new-target and --target-id are mutually exclusive")
	}
	if opts.targetAliasSet && !opts.newTarget {
		return output.Validation("target_alias", "--target-alias requires --new-target")
	}
	if opts.targetAliasSet && (opts.targetAlias == "" || !utf8.ValidString(opts.targetAlias) || strings.TrimSpace(opts.targetAlias) != opts.targetAlias || utf8.RuneCountInString(opts.targetAlias) > 191) {
		return output.Validation("target_alias", "--target-alias must be 1 to 191 Unicode characters without leading or trailing whitespace")
	}
	if opts.targetID == "" && opts.expectedVersionSet {
		return output.Validation("target_version", "--expected-target-version requires --target-id")
	}
	if opts.targetID != "" && !opts.expectedVersionSet {
		return output.Validation("target_version", "--target-id requires --expected-target-version")
	}
	if opts.expectedVersionSet && opts.expectedTargetVersion < 0 {
		return output.Validation("target_version", "--expected-target-version must be zero or greater")
	}
	if (opts.file != "" || opts.directory != "") && !opts.newTarget && opts.targetID == "" {
		return output.Validation("upload_target", "uploaded input requires --new-target or --target-id with --expected-target-version")
	}
	return nil
}

func publishOwnershipMode(opts publishOptions) string {
	if opts.delegatedGrantStdin || opts.delegatedGrantRef != "" {
		return "delegated"
	}
	return "direct"
}

func expressionPublicationSource(runtime *Runtime, args []string, opts publishOptions) (api.Source, error) {
	if len(args) == 1 {
		if strings.TrimSpace(args[0]) == "" {
			return api.Source{}, output.Validation("source_empty", "source expression cannot be empty")
		}
		return api.Source{Kind: "expression", Value: args[0]}, nil
	}
	if opts.expressionStdin {
		value, err := readLimited(runtime.deps.In, maxStdinBytes)
		if err != nil {
			return api.Source{}, err
		}
		if strings.TrimSpace(value) == "" {
			return api.Source{}, output.Validation("source_empty", "source expression cannot be empty")
		}
		return api.Source{Kind: "expression", Value: value}, nil
	}
	return api.Source{}, output.Validation("delegated_source_invalid", "delegated publication requires an expression source or --resolution-id")
}

func selectInspectedCandidate(inspection api.InspectResponse, requestedSelector string) (string, string, error) {
	resolutionID := strings.TrimSpace(inspection.ResolutionID)
	if resolutionID == "" || resolutionID != inspection.ResolutionID || len(resolutionID) > 255 {
		return "", "", output.Internal("inspect_response_invalid", "Viceme API did not return a valid immutable resolution id", nil)
	}
	if len(inspection.Candidates) == 0 {
		return "", "", output.Internal("inspect_response_invalid", "Viceme API did not return any immutable Skill candidates", nil)
	}
	if len(inspection.Candidates) > 1 && requestedSelector == "" {
		choices := make([]map[string]string, 0, len(inspection.Candidates))
		for _, candidate := range inspection.Candidates {
			choices = append(choices, map[string]string{"selector": candidate.Selector, "title": candidate.Title})
		}
		return "", "", output.Validation(
			"selection_required",
			"multiple Skill roots were found; rerun with --skill-root using one returned selector",
		).WithDetails(map[string]any{"candidates": choices})
	}
	selected := inspection.Candidates[0]
	if requestedSelector != "" {
		matched := false
		for _, candidate := range inspection.Candidates {
			if candidate.Selector == requestedSelector {
				selected = candidate
				matched = true
				break
			}
		}
		if !matched {
			return "", "", output.Validation("selector_invalid", "--skill-root does not match any immutable candidate returned by inspect")
		}
	}
	if strings.TrimSpace(selected.Selector) == "" || strings.TrimSpace(selected.Selector) != selected.Selector || len(selected.Selector) > 512 {
		return "", "", output.Internal("inspect_response_invalid", "Viceme API returned an invalid candidate selector", nil)
	}
	return resolutionID, selected.Selector, nil
}

func delegatedPublicationIntentFingerprint(request api.CreatePublicationRequest, inspectedSource *api.Source) (string, error) {
	intent := struct {
		Format       string                 `json:"format"`
		Source       *api.Source            `json:"source,omitempty"`
		ResolutionID string                 `json:"resolution_id,omitempty"`
		Selector     string                 `json:"selector,omitempty"`
		Destination  api.Destination        `json:"destination"`
		Options      api.PublicationOptions `json:"options"`
	}{
		Format:       "viceme-delegated-publication-intent/v2",
		Source:       inspectedSource,
		ResolutionID: request.ResolutionID,
		Selector:     request.Selector,
		Destination:  request.Destination,
		Options:      request.Options,
	}
	return delegatedPublicationFingerprint(intent)
}

func delegatedPublicationRequestFingerprint(request api.CreatePublicationRequest) (string, error) {
	frozen := struct {
		Format       string                 `json:"format"`
		ResolutionID string                 `json:"resolution_id"`
		Selector     string                 `json:"selector,omitempty"`
		Destination  api.Destination        `json:"destination"`
		Options      api.PublicationOptions `json:"options"`
	}{
		Format:       "viceme-delegated-publication-request/v1",
		ResolutionID: request.ResolutionID,
		Selector:     request.Selector,
		Destination:  request.Destination,
		Options:      request.Options,
	}
	return delegatedPublicationFingerprint(frozen)
}

func delegatedPublicationFingerprint(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", output.Internal("delegated_publication_fingerprint", "failed to encode delegated publication state", err)
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("sha256:%x", digest[:]), nil
}

func delegatedCredentialSource(opts publishOptions) string {
	switch {
	case opts.delegatedGrantStdin:
		return "stdin"
	case opts.delegatedGrantRef != "":
		return "keychain"
	default:
		return ""
	}
}

func publishDestination(opts publishOptions) api.Destination {
	switch {
	case opts.newTarget:
		return api.Destination{Mode: "new", Alias: opts.targetAlias}
	case opts.targetID != "":
		version := opts.expectedTargetVersion
		return api.Destination{Mode: "existing", TargetID: opts.targetID, ExpectedTargetVersion: &version}
	default:
		return api.Destination{Mode: "auto"}
	}
}

func publishSourceMode(args []string, opts publishOptions) string {
	switch {
	case len(args) == 1:
		return "argument"
	case opts.resolutionID != "":
		return "resolution"
	case opts.expressionStdin:
		return "stdin"
	case opts.file != "":
		return "file"
	case opts.directory != "":
		return "directory"
	default:
		return "unknown"
	}
}

func publicationSource(command *cobra.Command, runtime *Runtime, args []string, opts publishOptions) (api.Source, error) {
	if len(args) == 1 {
		if strings.TrimSpace(args[0]) == "" {
			return api.Source{}, output.Validation("source_empty", "source expression cannot be empty")
		}
		return api.Source{Kind: "expression", Value: args[0]}, nil
	}
	if opts.expressionStdin {
		value, err := readLimited(runtime.deps.In, maxStdinBytes)
		if err != nil {
			return api.Source{}, err
		}
		if strings.TrimSpace(value) == "" {
			return api.Source{}, output.Validation("source_empty", "source expression cannot be empty")
		}
		return api.Source{Kind: "expression", Value: value}, nil
	}
	artifact, err := publicationArtifact(command, opts)
	if err != nil {
		return api.Source{}, err
	}
	defer artifact.Cleanup()
	uploadID, err := uploadArtifact(command, runtime, artifact)
	if err != nil {
		return api.Source{}, err
	}
	return api.Source{Kind: "upload", Value: uploadID}, nil
}

func publicationArtifact(command *cobra.Command, opts publishOptions) (archivepkg.Artifact, error) {
	if opts.file != "" {
		artifact, err := archivepkg.FromFile(opts.file, archivepkg.DefaultMaxBytes)
		if err != nil {
			return archivepkg.Artifact{}, err
		}
		ext := strings.ToLower(filepath.Ext(opts.file))
		if ext == ".zip" {
			artifact.ContentType = "application/zip"
		}
		return artifact, nil
	}
	return archivepkg.BuildDirectory(command.Context(), opts.directory, archivepkg.DefaultMaxBytes)
}

func uploadArtifact(command *cobra.Command, runtime *Runtime, artifact archivepkg.Artifact) (string, error) {
	client := runtime.client()
	prepared, err := client.PrepareUpload(command.Context(), api.UploadPrepareRequest{
		Filename:     artifact.Filename,
		ContentType:  artifact.ContentType,
		Size:         artifact.Size,
		SHA256Digest: artifact.SHA256Digest,
	})
	if err != nil {
		return "", err
	}
	if prepared.UploadID == "" || prepared.UploadURL == "" {
		return "", output.Internal("upload_prepare_response", "Viceme API returned an incomplete upload slot", nil)
	}
	file, err := artifact.Open()
	if err != nil {
		return "", output.Internal("upload_open", "failed to reopen the prepared Skill archive", err)
	}
	defer file.Close()
	if err := client.PutUpload(command.Context(), prepared, file, artifact.Size); err != nil {
		return "", err
	}
	completed, err := client.CompleteUpload(command.Context(), prepared.UploadID, api.UploadCompleteRequest{Size: artifact.Size, SHA256Digest: artifact.SHA256Digest})
	if err != nil {
		return "", err
	}
	if completed.UploadID != "" && completed.UploadID != prepared.UploadID {
		return "", output.Internal("upload_complete_response", "Viceme API changed upload identity during completion", nil)
	}
	return prepared.UploadID, nil
}

func newTargetCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "target", Short: "Read stable Skill Agent Targets"}
	command.AddCommand(&cobra.Command{
		Use:   "get <target-id|share-url|alias>",
		Short: "Get a Target owned by the authenticated user",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			response, err := runtime.client().GetTarget(command.Context(), args[0])
			if err != nil {
				return err
			}
			return runtime.success(response)
		},
	})
	command.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List Targets owned by the authenticated user",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			response, err := runtime.client().ListTargets(command.Context())
			if err != nil {
				return err
			}
			return runtime.success(response)
		},
	})
	return command
}

func readLimited(reader io.Reader, limit int64) (string, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return "", output.Validation("stdin_read", "failed to read stdin")
	}
	if int64(len(data)) > limit {
		return "", output.Policy("stdin_too_large", fmt.Sprintf("stdin exceeds the %d byte limit", limit))
	}
	return string(data), nil
}
