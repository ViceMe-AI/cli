package command

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// Runtime command surface for Slice D (skill-app-platform.md §8/§11).
//
//   viceme runtime inspect <runtimeReleaseId>
//   viceme job get <runId> --publishable-key <key> --origin <url>
//   viceme job wait <runId> --publishable-key <key> --origin <url>
//   viceme job cancel <runId> --publishable-key <key> --origin <url>

func newRuntimeCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "runtime", Short: "Read Runtime Contracts and manage Skill Runs"}
	command.AddCommand(newRuntimeInspectCommand(runtime))
	return command
}

func newJobCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "job", Short: "Manage Runtime Runs (get, wait, cancel)"}
	command.AddCommand(newJobGetCommand(runtime))
	command.AddCommand(newJobWaitCommand(runtime))
	command.AddCommand(newJobCancelCommand(runtime))
	return command
}

func newRuntimeInspectCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect <runtimeReleaseId>",
		Short: "Read the Runtime Contract for a Runtime Release",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			client := runtime.client()
			contract, err := client.GetRuntimeContract(ctx, args[0])
			if err != nil {
				return err
			}
			return runtime.business(map[string]any{
				"runtimeContract": contract,
			})
		},
	}
}

func newJobGetCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{
		Use:   "get <runId>",
		Short: "Get the status and result of a Runtime Run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flags, err := parseRuntimeRunFlags(cmd)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			run, err := runtime.client().GetRuntimeRun(ctx, args[0], flags.publishableKey, flags.origin)
			if err != nil {
				return err
			}
			return runtime.business(map[string]any{"run": run})
		},
	}
	registerRuntimeRunFlags(command)
	return command
}

func newJobWaitCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{
		Use:   "wait <runId>",
		Short: "Wait for a Runtime Run to reach a terminal state",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flags, err := parseRuntimeRunFlags(cmd)
			if err != nil {
				return err
			}
			timeout, _ := cmd.Flags().GetDuration("timeout")
			if timeout == 0 {
				timeout = 5 * time.Minute
			}
			ctx := cmd.Context()
			deadline := time.Now().Add(timeout)
			client := runtime.client()
			for time.Now().Before(deadline) {
				run, err := client.GetRuntimeRun(ctx, args[0], flags.publishableKey, flags.origin)
				if err != nil {
					return err
				}
				if isTerminalRunStatus(run.Status) {
					return runtime.business(map[string]any{
						"run":            run,
						"wait_timed_out": false,
					})
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(2 * time.Second):
				}
			}
			return runtime.business(map[string]any{
				"ok":    false,
				"meta":  map[string]any{"wait_timed_out": true},
				"runId": args[0],
			})
		},
	}
	command.Flags().Duration("timeout", 5*time.Minute, "Maximum time to wait for the Run to complete")
	registerRuntimeRunFlags(command)
	return command
}

func newJobCancelCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{
		Use:   "cancel <runId>",
		Short: "Request cancellation of a Runtime Run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flags, err := parseRuntimeRunFlags(cmd)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			result, err := runtime.client().CancelRuntimeRun(ctx, args[0], flags.publishableKey, flags.origin)
			if err != nil {
				return err
			}
			return runtime.business(map[string]any{"cancel": result})
		},
	}
	registerRuntimeRunFlags(command)
	return command
}

type runtimeRunFlags struct {
	publishableKey string
	origin         string
}

func registerRuntimeRunFlags(command *cobra.Command) {
	command.Flags().String("publishable-key", "", "Publishable Key for the App environment (required)")
	command.Flags().String("origin", "", "Origin of the App (required for CORS)")
	_ = command.MarkFlagRequired("publishable-key")
	_ = command.MarkFlagRequired("origin")
}

func parseRuntimeRunFlags(cmd *cobra.Command) (runtimeRunFlags, error) {
	pk, _ := cmd.Flags().GetString("publishable-key")
	origin, _ := cmd.Flags().GetString("origin")
	if pk == "" {
		return runtimeRunFlags{}, fmt.Errorf("--publishable-key is required")
	}
	if origin == "" {
		return runtimeRunFlags{}, fmt.Errorf("--origin is required")
	}
	return runtimeRunFlags{publishableKey: pk, origin: origin}, nil
}

func isTerminalRunStatus(status string) bool {
	switch status {
	case "SUCCEEDED", "FAILED", "CANCELLED", "TIMED_OUT":
		return true
	default:
		return false
	}
}

// generateClientRequestID creates a random UUID v4 for idempotency keys.
func generateClientRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	)
}
