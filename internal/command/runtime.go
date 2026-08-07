package command

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/spf13/cobra"
)

// Runtime command surface for Slice D (skill-app-platform.md §8/§11).
//
//   viceme runtime inspect <runtimeReleaseId>
//   viceme job get <runId> --publishable-key <key> --origin <url> --runtime-ticket <ticket>
//   viceme job wait <runId> --publishable-key <key> --origin <url> --runtime-ticket <ticket>
//   viceme job cancel <runId> --publishable-key <key> --origin <url> --runtime-ticket <ticket>
//   viceme job artifacts <runId> --publishable-key <key> --origin <url> --runtime-ticket <ticket> [--artifact-id <id> --out <dir>]

func newRuntimeCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "runtime", Short: "Read Runtime Contracts and manage Skill Runs"}
	command.AddCommand(newRuntimeInspectCommand(runtime))
	return command
}

func newJobCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "job", Short: "Manage Runtime Runs (get, wait, cancel, artifacts)"}
	command.AddCommand(newJobGetCommand(runtime))
	command.AddCommand(newJobWaitCommand(runtime))
	command.AddCommand(newJobCancelCommand(runtime))
	command.AddCommand(newJobArtifactsCommand(runtime))
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
			run, err := runtime.client().GetRuntimeRun(ctx, args[0], flags.publishableKey, flags.origin, flags.runtimeTicket)
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
				run, err := client.GetRuntimeRun(ctx, args[0], flags.publishableKey, flags.origin, flags.runtimeTicket)
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
			result, err := runtime.client().CancelRuntimeRun(ctx, args[0], flags.publishableKey, flags.origin, flags.runtimeTicket)
			if err != nil {
				return err
			}
			return runtime.business(map[string]any{"cancel": result})
		},
	}
	registerRuntimeRunFlags(command)
	return command
}

func newJobArtifactsCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{
		Use:   "artifacts <runId>",
		Short: "List Runtime Run artifacts, or download one with --artifact-id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flags, err := parseRuntimeRunFlags(cmd)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			run, err := runtime.client().GetRuntimeRun(ctx, args[0], flags.publishableKey, flags.origin, flags.runtimeTicket)
			if err != nil {
				return err
			}
			artifactID, _ := cmd.Flags().GetString("artifact-id")
			outDir, _ := cmd.Flags().GetString("out")
			if artifactID == "" {
				return runtime.business(map[string]any{"artifacts": run.Artifacts})
			}
			artifact, err := selectArtifact(run.Artifacts, artifactID)
			if err != nil {
				return err
			}
			downloaded, err := downloadArtifact(ctx, runtime, artifact, outDir, args[0])
			if err != nil {
				return err
			}
			return runtime.business(map[string]any{"artifact": artifact, "download": downloaded})
		},
	}
	command.Flags().String("artifact-id", "", "Download a single artifact by ID (omit to list)")
	command.Flags().String("out", ".", "Directory to write the downloaded artifact")
	registerRuntimeRunFlags(command)
	return command
}

type runtimeRunFlags struct {
	publishableKey string
	origin         string
	runtimeTicket  string
}

func registerRuntimeRunFlags(command *cobra.Command) {
	command.Flags().String("publishable-key", "", "Publishable Key for the App environment (required)")
	command.Flags().String("origin", "", "Origin of the App (required for CORS)")
	command.Flags().String("runtime-ticket", "", "Runtime Ticket (Bearer) obtained from the ViceMe web UI; required to authorize Run access")
	_ = command.MarkFlagRequired("publishable-key")
	_ = command.MarkFlagRequired("origin")
}

func parseRuntimeRunFlags(cmd *cobra.Command) (runtimeRunFlags, error) {
	pk, _ := cmd.Flags().GetString("publishable-key")
	origin, _ := cmd.Flags().GetString("origin")
	ticket, _ := cmd.Flags().GetString("runtime-ticket")
	if pk == "" {
		return runtimeRunFlags{}, fmt.Errorf("--publishable-key is required")
	}
	if origin == "" {
		return runtimeRunFlags{}, fmt.Errorf("--origin is required")
	}
	return runtimeRunFlags{publishableKey: pk, origin: origin, runtimeTicket: ticket}, nil
}

func isTerminalRunStatus(status string) bool {
	switch status {
	case "SUCCEEDED", "FAILED", "CANCELLED", "TIMED_OUT":
		return true
	default:
		return false
	}
}

func selectArtifact(artifacts []api.RuntimeArtifact, artifactID string) (api.RuntimeArtifact, error) {
	for _, artifact := range artifacts {
		if artifact.ArtifactID == artifactID {
			return artifact, nil
		}
	}
	return api.RuntimeArtifact{}, fmt.Errorf("artifact %q not found on this run", artifactID)
}

func downloadArtifact(ctx context.Context, runtime *Runtime, artifact api.RuntimeArtifact, outDir, runID string) (map[string]any, error) {
	if artifact.DownloadURL == "" {
		return nil, fmt.Errorf("artifact %q has no download URL (it may be expired or the caller lacks permission)", artifact.ArtifactID)
	}
	body, err := runtime.client().DownloadArtifact(ctx, artifact.DownloadURL)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}
	filename := artifactFilename(artifact, runID)
	destination := filepath.Join(outDir, filename)
	temporary, err := os.CreateTemp(outDir, ".artifact-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryName)
		}
	}()
	written, err := io.Copy(temporary, body)
	if err != nil {
		return nil, fmt.Errorf("write artifact: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return nil, fmt.Errorf("close artifact: %w", err)
	}
	if err := os.Rename(temporaryName, destination); err != nil {
		return nil, fmt.Errorf("save artifact: %w", err)
	}
	committed = true
	return map[string]any{
		"path":      destination,
		"sizeBytes": written,
		"filename":  filename,
	}, nil
}

func artifactFilename(artifact api.RuntimeArtifact, runID string) string {
	extension := extensionForKind(artifact.Kind, artifact.ContentType)
	if extension == "" {
		extension = ".bin"
	}
	return fmt.Sprintf("%s-%s%s", runID, artifact.ArtifactID, extension)
}

func extensionForKind(kind, contentType string) string {
	switch strings.ToUpper(kind) {
	case "IMAGE":
		switch strings.ToLower(contentType) {
		case "image/png":
			return ".png"
		case "image/jpeg", "image/jpg":
			return ".jpg"
		case "image/webp":
			return ".webp"
		case "image/gif":
			return ".gif"
		}
		return ".img"
	case "TEXT":
		return ".txt"
	case "DATA":
		return ".json"
	default:
		switch strings.ToLower(contentType) {
		case "application/json":
			return ".json"
		case "text/plain":
			return ".txt"
		case "application/pdf":
			return ".pdf"
		}
		return ""
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
