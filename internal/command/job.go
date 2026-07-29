package command

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

func newJobCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "job", Short: "Read and control durable Skill Agent publications"}
	command.AddCommand(newJobGetCommand(runtime))
	command.AddCommand(newJobWaitCommand(runtime))
	command.AddCommand(newJobBindCommand(runtime))
	command.AddCommand(newJobMetadataCommand(runtime))
	command.AddCommand(newJobPreviewCommand(runtime))
	command.AddCommand(newJobEditCommand(runtime))
	command.AddCommand(newJobEditGetCommand(runtime))
	command.AddCommand(newJobResumeCommand(runtime))
	command.AddCommand(newJobRetryCommand(runtime))
	command.AddCommand(newJobCancelCommand(runtime))
	return command
}

func newJobBindCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "bind <publication-id>",
		Short: "Show the channel account binding URL for a blocked publication",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			publication, err := runtime.client().GetPublication(command.Context(), args[0])
			if err != nil {
				return err
			}
			if strings.TrimSpace(publication.Status()) != "binding_required" {
				return output.Validation("channel_binding_not_required", "this publication does not require channel account binding")
			}
			nextAction, ok := publication["next_action"].(map[string]any)
			if !ok || nextAction["type"] != "bind_channel_account" {
				return output.Validation("channel_binding_not_required", "this publication does not require channel account binding")
			}
			bindingURL, _ := nextAction["binding_url"].(string)
			if strings.TrimSpace(bindingURL) == "" {
				return output.Internal("channel_binding_url_missing", "the ViceMe API did not return a channel binding URL", nil)
			}
			retryMode, _ := nextAction["retry_mode"].(string)
			if retryMode != "new_publication" {
				return output.Internal("channel_binding_retry_mode_invalid", "the ViceMe API returned an invalid channel binding retry mode", nil)
			}
			return runtime.success(map[string]any{
				"publication_id": publication.ID(),
				"status":         publication.Status(),
				"binding_url":    bindingURL,
				"binding_status": nextAction["binding_status"],
				"provider":       nextAction["provider"],
				"expires_at":     nextAction["expires_at"],
				"retry_mode":     retryMode,
				"hints":          nextAction["hints"],
			})
		},
	}
}

func newJobRetryCommand(runtime *Runtime) *cobra.Command {
	var yes bool
	command := &cobra.Command{
		Use:   "retry <publication-id>",
		Short: "Explicitly retry a retryable compiler failure",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if !yes {
				return output.Confirmation("confirmation_required", "retrying compilation may invoke the model again; explicit confirmation with --yes is required")
			}
			publication, err := runtime.client().RetryPublication(command.Context(), args[0])
			if err != nil {
				return err
			}
			return runtime.success(publication)
		},
	}
	command.Flags().BoolVar(&yes, "yes", false, "confirm one bounded compiler retry")
	return command
}

func newJobGetCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "get <publication-id>",
		Short: "Get a publication's durable status",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			publication, err := runtime.client().GetPublication(command.Context(), args[0])
			if err != nil {
				return err
			}
			return runtime.success(publication)
		},
	}
}

func newJobWaitCommand(runtime *Runtime) *cobra.Command {
	var timeout time.Duration
	command := &cobra.Command{
		Use:   "wait <publication-id>",
		Short: "Wait for a bounded publication result",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if timeout <= 0 {
				return output.Validation("timeout", "--timeout must be greater than zero")
			}
			publication, timedOut, err := waitPublication(command.Context(), runtime, args[0], timeout)
			if err != nil {
				return err
			}
			meta := output.Meta{}
			if timedOut {
				value := true
				meta.WaitTimedOut = &value
			}
			return runtime.successWithMeta(publication, meta)
		},
	}
	command.Flags().DurationVar(&timeout, "timeout", 60*time.Second, "maximum time to wait")
	return command
}

func newJobPreviewCommand(runtime *Runtime) *cobra.Command {
	var actionID string
	command := &cobra.Command{
		Use:   "preview <publication-id>",
		Short: "Show the frozen public summary of the exact release candidate",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			preview, err := runtime.client().GetPublicationPreview(command.Context(), args[0], actionID)
			if err != nil {
				return err
			}
			return runtime.success(preview)
		},
	}
	command.Flags().StringVar(&actionID, "action-id", "", "confirm_publish action receipt ID (defaults to the latest)")
	return command
}

func newJobEditCommand(runtime *Runtime) *cobra.Command {
	var candidateDigest string
	var requestStdin bool
	var timeout time.Duration
	command := &cobra.Command{
		Use:   "edit <publication-id>",
		Short: "Submit a natural-language candidate edit and wait for the new candidate",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if candidateDigest == "" {
				return output.Validation("edit_flags", "edit requires --candidate-digest binding the current candidate")
			}
			if !requestStdin {
				return output.Validation("edit_flags", "edit requires --request-stdin; natural-language edit requests are never accepted through argv")
			}
			editRequest, err := readLimited(runtime.deps.In, maxStdinBytes)
			if err != nil {
				return err
			}
			if strings.TrimSpace(editRequest) == "" {
				return output.Validation("edit_request", "stdin must contain a natural-language edit request")
			}
			if timeout <= 0 {
				timeout = 2 * time.Minute
			}
			receipt, err := runtime.client().RequestPublicationEdit(command.Context(), args[0], api.PublicationEditRequest{
				EditRequest: editRequest, CurrentCandidateDigest: candidateDigest,
			})
			if err != nil {
				return err
			}
			final, timedOut, err := waitPublicationEdit(command.Context(), runtime, args[0], receipt.EditID, timeout)
			if err != nil {
				return err
			}
			// 轮询超时也必须保留已创建的 edit ID:调用方拿着同一 edit_id 继续
			// 轮询/恢复,而不是盲目重发产生第二个逻辑编辑。
			meta := output.Meta{}
			if timedOut {
				value := true
				meta.WaitTimedOut = &value
			}
			return runtime.successWithMeta(final, meta)
		},
	}
	command.Flags().StringVar(&candidateDigest, "candidate-digest", "", "current exact release candidate digest")
	command.Flags().BoolVar(&requestStdin, "request-stdin", false, "read the complete natural-language edit request from stdin")
	command.Flags().DurationVar(&timeout, "timeout", 2*time.Minute, "maximum time to wait for the edit")
	return command
}

func waitPublicationEdit(ctx context.Context, runtime *Runtime, publicationID, editID string, timeout time.Duration) (api.PublicationEditReceipt, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	last := api.PublicationEditReceipt{}
	for {
		receipt, err := runtime.client().GetPublicationEdit(ctx, publicationID, editID)
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				if last.EditID == "" {
					last = api.PublicationEditReceipt{EditID: editID, Status: "pending"}
				}
				return last, true, nil
			}
			return receipt, false, err
		}
		last = receipt
		switch receipt.Status {
		case "applied", "failed":
			return receipt, false, nil
		}
		if err := runtime.deps.Sleep(ctx, 3*time.Second); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return last, true, nil
			}
			return last, false, err
		}
	}
}

func newJobEditGetCommand(runtime *Runtime) *cobra.Command {
	var timeout time.Duration
	command := &cobra.Command{
		Use:   "edit-get <publication-id> <edit-id>",
		Short: "Read a candidate edit receipt by ID, optionally resuming the wait after a timeout",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			if timeout <= 0 {
				receipt, err := runtime.client().GetPublicationEdit(command.Context(), args[0], args[1])
				if err != nil {
					return err
				}
				return runtime.success(receipt)
			}
			final, timedOut, err := waitPublicationEdit(command.Context(), runtime, args[0], args[1], timeout)
			if err != nil {
				return err
			}
			meta := output.Meta{}
			if timedOut {
				value := true
				meta.WaitTimedOut = &value
			}
			return runtime.successWithMeta(final, meta)
		},
	}
	command.Flags().DurationVar(&timeout, "timeout", 0, "resume waiting for the edit (0 = single read)")
	return command
}

func newJobMetadataCommand(runtime *Runtime) *cobra.Command {
	var actionID string
	var expectedDigest string
	var decision string
	var title string
	var description string
	var author string
	var editsStdin bool
	command := &cobra.Command{
		Use:   "metadata <publication-id>",
		Short: "Review or resolve the metadata checkpoint of a publication",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if decision == "" {
				metadata, err := runtime.client().GetPublicationMetadata(command.Context(), args[0])
				if err != nil {
					return err
				}
				return runtime.success(metadata)
			}
			if decision != "confirm" && decision != "cancel" {
				return output.Validation("metadata_decision", "--decision must be confirm or cancel")
			}
			if actionID == "" || expectedDigest == "" {
				return output.Validation("metadata_flags", "resolving metadata requires --action-id and --expected-payload-digest")
			}
			// 用户提供的自然语言字段(标题/描述/作者)走结构化 stdin JSON 传输,
			// 不插值进带引号的 shell 命令行。
			if editsStdin {
				if command.Flags().Changed("title") || command.Flags().Changed("description") || command.Flags().Changed("author") {
					return output.Validation("metadata_flags", "--edits-stdin does not combine with --title/--description/--author")
				}
				raw, err := readLimited(runtime.deps.In, maxStdinBytes)
				if err != nil {
					return err
				}
				var edits struct {
					Title       *string `json:"title"`
					Description *string `json:"description"`
					Author      *string `json:"author"`
				}
				if !json.Valid([]byte(raw)) || json.Unmarshal([]byte(raw), &edits) != nil {
					return output.Validation("metadata_edits", "stdin must contain one JSON object with optional title/description/author")
				}
				if edits.Title != nil {
					title = *edits.Title
				}
				if edits.Description != nil {
					description = *edits.Description
				}
				if edits.Author != nil {
					author = *edits.Author
				}
			}
			publication, err := runtime.client().ResolvePublicationMetadata(command.Context(), args[0], api.ResolveMetadataRequest{
				ActionID: actionID, ExpectedPayloadDigest: expectedDigest,
				Decision: decision, Title: title, Description: description, Author: author,
			})
			if err != nil {
				return err
			}
			return runtime.success(publication)
		},
	}
	command.Flags().StringVar(&actionID, "action-id", "", "confirm_metadata action receipt ID")
	command.Flags().StringVar(&expectedDigest, "expected-payload-digest", "", "digest of the metadata action payload")
	command.Flags().StringVar(&decision, "decision", "", "metadata decision: confirm or cancel")
	command.Flags().StringVar(&title, "title", "", "optional title edit (1-20 visible characters; machine callers: prefer --edits-stdin)")
	command.Flags().StringVar(&description, "description", "", "optional description edit (1-100 visible characters; machine callers: prefer --edits-stdin)")
	command.Flags().StringVar(&author, "author", "", "optional source-author edit / missing-author fill (1-100 visible characters; machine callers: prefer --edits-stdin)")
	command.Flags().BoolVar(&editsStdin, "edits-stdin", false, "read optional {\"title\",\"description\",\"author\"} edits as one JSON object from stdin (no shell interpolation)")
	return command
}

func newJobResumeCommand(runtime *Runtime) *cobra.Command {
	var actionID string
	var expectedDigest string
	var expectedCandidateDigest string
	var expectedSummaryDigest string
	var decision string
	var payloadStdin bool
	command := &cobra.Command{
		Use:   "resume <publication-id>",
		Short: "Resolve a typed next action on the same publication",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if actionID == "" || expectedDigest == "" {
				return output.Validation("resume_flags", "resume requires --action-id and --expected-payload-digest")
			}
			request := api.ResolveActionRequest{ExpectedPayloadDigest: expectedDigest}
			if decision != "" {
				// confirm_publish binds the user's explicit decision to the exact
				// release candidate shown through the stable share preview; the CLI
				// never infers it. The decision
				// goes to the dedicated resolve-confirmation endpoint whose digest
				// contract is identical across OpenAPI/SDK/runtime.
				if decision != "confirm" && decision != "cancel" {
					return output.Validation("resume_decision", "--decision must be confirm or cancel")
				}
				if expectedCandidateDigest == "" {
					return output.Validation("resume_flags", "confirm_publish resolution requires --expected-release-candidate-digest")
				}
				if expectedSummaryDigest == "" {
					return output.Validation("resume_flags", "confirm_publish resolution requires --expected-public-summary-digest; take public_summary_digest from the job preview output")
				}
				if payloadStdin {
					return output.Validation("resume_flags", "--decision does not accept --payload-stdin")
				}
				publication, err := runtime.client().ResolveConfirmation(command.Context(), args[0], actionID, api.ResolveConfirmationRequest{
					ExpectedPayloadDigest:          expectedDigest,
					ExpectedReleaseCandidateDigest: expectedCandidateDigest,
					ExpectedPublicSummaryDigest:    expectedSummaryDigest,
					Decision:                       decision,
				})
				if err != nil {
					return err
				}
				return runtime.success(publication)
			}
			if !payloadStdin {
				return output.Validation("resume_flags", "resume requires --payload-stdin for typed payload actions")
			}
			payload, err := readLimited(runtime.deps.In, maxStdinBytes)
			if err != nil {
				return err
			}
			if !json.Valid([]byte(payload)) {
				return output.Validation("action_payload", "stdin must contain one valid JSON action payload")
			}
			request.Payload = json.RawMessage(payload)
			publication, err := runtime.client().ResolveAction(command.Context(), args[0], actionID, request)
			if err != nil {
				return err
			}
			return runtime.success(publication)
		},
	}
	command.Flags().StringVar(&actionID, "action-id", "", "typed action receipt ID")
	command.Flags().StringVar(&expectedDigest, "expected-payload-digest", "", "digest of the action payload being answered")
	command.Flags().StringVar(&expectedCandidateDigest, "expected-release-candidate-digest", "", "exact release candidate digest shown before opening the stable share preview")
	command.Flags().StringVar(&expectedSummaryDigest, "expected-public-summary-digest", "", "public_summary_digest from the job preview output (binds the confirmation to the frozen summary)")
	command.Flags().StringVar(&decision, "decision", "", "confirm_publish decision: confirm or cancel")
	command.Flags().BoolVar(&payloadStdin, "payload-stdin", false, "read the structured action answer from stdin")
	return command
}

func newJobCancelCommand(runtime *Runtime) *cobra.Command {
	var yes bool
	command := &cobra.Command{
		Use:   "cancel <publication-id>",
		Short: "Cancel a publication",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if !yes {
				return output.Confirmation("confirmation_required", "cancelling a publication requires explicit confirmation with --yes")
			}
			publication, err := runtime.client().CancelPublication(command.Context(), args[0])
			if err != nil {
				return err
			}
			return runtime.success(publication)
		},
	}
	command.Flags().BoolVar(&yes, "yes", false, "confirm cancellation")
	return command
}

func waitPublication(ctx context.Context, runtime *Runtime, id string, timeout time.Duration) (api.Publication, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	deadline := runtime.deps.Now().Add(timeout)
	var last api.Publication
	for {
		publication, err := runtime.client().GetPublication(ctx, id)
		if err != nil {
			if len(last) > 0 && errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return last, true, nil
			}
			return nil, false, err
		}
		last = publication
		if publicationWaitComplete(publication.Status()) {
			return publication, false, nil
		}
		remaining := deadline.Sub(runtime.deps.Now())
		if remaining <= 0 {
			return last, true, nil
		}
		delay := 2 * time.Second
		if remaining < delay {
			delay = remaining
		}
		if err := runtime.deps.Sleep(ctx, delay); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return last, true, nil
			}
			return nil, false, err
		}
	}
}

func publicationWaitComplete(status string) bool {
	switch status {
	case "share_published", "meta_review", "awaiting_action", "binding_required", "unsupported", "rejected", "payment_required", "target_conflict", "cancelled", "failed":
		return true
	default:
		return false
	}
}
