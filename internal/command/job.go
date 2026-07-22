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
	command.AddCommand(newJobRunCommand(runtime))
	command.AddCommand(newJobAcceptCommand(runtime))
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
			nextAction, ok := publication["next_action"].(map[string]any)
			if !ok || nextAction["type"] != "bind_channel_account" {
				return output.Validation("channel_binding_not_required", "this publication does not require channel account binding")
			}
			bindingURL, _ := nextAction["binding_url"].(string)
			if strings.TrimSpace(bindingURL) == "" {
				return output.Internal("channel_binding_url_missing", "the ViceMe API did not return a channel binding URL", nil)
			}
			return runtime.success(map[string]any{
				"publication_id": publication.ID(),
				"status":         publication.Status(),
				"binding_url":    bindingURL,
				"binding_status": nextAction["binding_status"],
				"provider":       nextAction["provider"],
				"expires_at":     nextAction["expires_at"],
				"retry_mode":     "new_publication",
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
			meta := runtime.meta
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
	var candidateDigest, editRequest string
	var timeout time.Duration
	command := &cobra.Command{
		Use:   "edit <publication-id>",
		Short: "Submit a natural-language candidate edit and wait for the new candidate",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if strings.TrimSpace(editRequest) == "" {
				return output.Validation("edit_request", "edit requires --request with a natural-language edit")
			}
			if candidateDigest == "" {
				return output.Validation("edit_flags", "edit requires --candidate-digest binding the current candidate")
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
			final, err := waitPublicationEdit(command.Context(), runtime, args[0], receipt.EditID, timeout)
			if err != nil {
				return err
			}
			return runtime.success(final)
		},
	}
	command.Flags().StringVar(&candidateDigest, "candidate-digest", "", "current exact release candidate digest")
	command.Flags().StringVar(&editRequest, "request", "", "natural-language edit request")
	command.Flags().DurationVar(&timeout, "timeout", 2*time.Minute, "maximum time to wait for the edit")
	return command
}

func waitPublicationEdit(ctx context.Context, runtime *Runtime, publicationID, editID string, timeout time.Duration) (api.PublicationEditReceipt, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		receipt, err := runtime.client().GetPublicationEdit(ctx, publicationID, editID)
		if err != nil {
			return receipt, err
		}
		switch receipt.Status {
		case "applied", "failed":
			return receipt, nil
		}
		if err := runtime.deps.Sleep(ctx, 3*time.Second); err != nil {
			return receipt, err
		}
	}
}

func newJobRunCommand(runtime *Runtime) *cobra.Command {
	var candidateDigest string
	var inputFlags []string
	var timeout time.Duration
	command := &cobra.Command{
		Use:   "run <publication-id>",
		Short: "Run one real preview test of the exact candidate and show the result",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if candidateDigest == "" {
				return output.Validation("run_flags", "run requires --candidate-digest binding the exact candidate")
			}
			inputs, err := parseKeyValueInputs(inputFlags)
			if err != nil {
				return err
			}
			if timeout <= 0 {
				timeout = 3 * time.Minute
			}
			started, err := runtime.client().StartSkillPreviewRun(command.Context(), args[0], api.PreviewRunStartRequest{
				Inputs: inputs, ExpectedCandidateDigest: candidateDigest,
			})
			if err != nil {
				return err
			}
			final, err := waitSkillPreviewRun(command.Context(), runtime, args[0], started.PreviewRunID, timeout)
			if err != nil {
				return err
			}
			return runtime.success(final)
		},
	}
	command.Flags().StringVar(&candidateDigest, "candidate-digest", "", "exact release candidate digest to test")
	command.Flags().StringArrayVar(&inputFlags, "input", nil, "preview input as name=value (repeatable)")
	command.Flags().DurationVar(&timeout, "timeout", 3*time.Minute, "maximum time to wait for the run")
	return command
}

func parseKeyValueInputs(flags []string) (map[string]string, error) {
	inputs := make(map[string]string, len(flags))
	for _, flag := range flags {
		name, value, found := strings.Cut(flag, "=")
		if !found || strings.TrimSpace(name) == "" {
			return nil, output.Validation("run_inputs", "--input must be name=value")
		}
		inputs[name] = value
	}
	return inputs, nil
}

func waitSkillPreviewRun(ctx context.Context, runtime *Runtime, publicationID, previewRunID string, timeout time.Duration) (api.SkillPreviewRun, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		run, err := runtime.client().GetSkillPreviewRun(ctx, publicationID, previewRunID)
		if err != nil {
			return run, err
		}
		if run.Status == "succeeded" || run.Status == "failed" {
			return run, nil
		}
		if err := runtime.deps.Sleep(ctx, 3*time.Second); err != nil {
			return run, err
		}
	}
}

func newJobAcceptCommand(runtime *Runtime) *cobra.Command {
	var previewRunID, candidateDigest, inputsDigest string
	command := &cobra.Command{
		Use:   "accept <publication-id>",
		Short: "Accept the actual result of a preview test run",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if previewRunID == "" || candidateDigest == "" {
				return output.Validation("accept_flags", "accept requires --run-id and --candidate-digest")
			}
			if inputsDigest == "" {
				return output.Validation("accept_flags", "accept requires --inputs-digest binding the accepted input set (PRE-04); take inputs_digest from the job run receipt")
			}
			receipt, err := runtime.client().AcceptSkillPreviewRun(command.Context(), args[0], previewRunID, api.PreviewRunAcceptRequest{
				ExpectedCandidateDigest: candidateDigest, ExpectedInputsDigest: inputsDigest,
			})
			if err != nil {
				return err
			}
			return runtime.success(receipt)
		},
	}
	command.Flags().StringVar(&previewRunID, "run-id", "", "preview run receipt ID")
	command.Flags().StringVar(&candidateDigest, "candidate-digest", "", "exact release candidate digest of the accepted result")
	command.Flags().StringVar(&inputsDigest, "inputs-digest", "", "digest of the exact input set being accepted (PRE-04, from the job run receipt)")
	return command
}

func newJobMetadataCommand(runtime *Runtime) *cobra.Command {
	var actionID string
	var expectedDigest string
	var decision string
	var title string
	var description string
	var author string
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
	command.Flags().StringVar(&title, "title", "", "optional title edit (1-20 visible characters)")
	command.Flags().StringVar(&description, "description", "", "optional description edit (1-100 visible characters)")
	command.Flags().StringVar(&author, "author", "", "optional source-author edit / missing-author fill (1-100 visible characters)")
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
				// previewed release candidate; the CLI never infers it.
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
				request.Decision = decision
				request.ExpectedReleaseCandidateDigest = expectedCandidateDigest
				request.ExpectedPublicSummaryDigest = expectedSummaryDigest
			} else {
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
			}
			publication, err := runtime.client().ResolveAction(command.Context(), args[0], actionID, request)
			if err != nil {
				return err
			}
			return runtime.success(publication)
		},
	}
	command.Flags().StringVar(&actionID, "action-id", "", "typed action receipt ID")
	command.Flags().StringVar(&expectedDigest, "expected-payload-digest", "", "digest of the action payload being answered")
	command.Flags().StringVar(&expectedCandidateDigest, "expected-release-candidate-digest", "", "exact release candidate digest shown in the preview")
	command.Flags().StringVar(&expectedSummaryDigest, "expected-public-summary-digest", "", "public_summary_digest from the job preview output (binds the confirmation to the summary receipt)")
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
	case "share_published", "awaiting_action", "binding_required", "unsupported", "rejected", "payment_required", "target_conflict", "cancelled", "failed":
		return true
	default:
		return false
	}
}
