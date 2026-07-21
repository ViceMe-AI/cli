package command

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

func newJobCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "job", Short: "Read and control durable Skill Agent publications"}
	command.AddCommand(newJobGetCommand(runtime))
	command.AddCommand(newJobWaitCommand(runtime))
	command.AddCommand(newJobResumeCommand(runtime))
	command.AddCommand(newJobRetryCommand(runtime))
	command.AddCommand(newJobCancelCommand(runtime))
	return command
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

func newJobResumeCommand(runtime *Runtime) *cobra.Command {
	var actionID string
	var expectedDigest string
	var payloadStdin bool
	command := &cobra.Command{
		Use:   "resume <publication-id>",
		Short: "Resolve a typed next action on the same publication",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if actionID == "" || expectedDigest == "" || !payloadStdin {
				return output.Validation("resume_flags", "resume requires --action-id, --expected-payload-digest, and --payload-stdin")
			}
			payload, err := readLimited(runtime.deps.In, maxStdinBytes)
			if err != nil {
				return err
			}
			if !json.Valid([]byte(payload)) {
				return output.Validation("action_payload", "stdin must contain one valid JSON action payload")
			}
			publication, err := runtime.client().ResolveAction(command.Context(), args[0], actionID, api.ResolveActionRequest{
				ExpectedPayloadDigest: expectedDigest,
				Payload:               json.RawMessage(payload),
			})
			if err != nil {
				return err
			}
			return runtime.success(publication)
		},
	}
	command.Flags().StringVar(&actionID, "action-id", "", "typed action receipt ID")
	command.Flags().StringVar(&expectedDigest, "expected-payload-digest", "", "digest of the action payload being answered")
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
	case "share_published", "awaiting_action", "unsupported", "rejected", "payment_required", "target_conflict", "cancelled", "failed":
		return true
	default:
		return false
	}
}
