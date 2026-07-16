package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	archivepkg "github.com/ViceMe-AI/cli/internal/archive"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

const maxStdinBytes = 1 << 20

func newSkillCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "skill", Short: "Inspect and publish external Skills"}
	command.AddCommand(newSkillInspectCommand(runtime))
	command.AddCommand(newSkillPublishCommand(runtime))
	command.AddCommand(newTargetCommand(runtime))
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
	targetID              string
	expectedTargetVersion int64
	targetLocale          string
	yes                   bool
	wait                  bool
	timeout               time.Duration
	dryRun                bool
	clientRequestID       string
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
			if err := validatePublishOptions(args, opts); err != nil {
				return err
			}
			destination := publishDestination(opts)
			if opts.dryRun {
				return runtime.success(map[string]any{
					"dry_run":         true,
					"operation":       "skill.publish",
					"source_mode":     publishSourceMode(args, opts),
					"destination":     destination,
					"wait":            opts.wait,
					"confirmation_ok": opts.yes,
				})
			}
			requestID := opts.clientRequestID
			if requestID == "" {
				requestID = runtime.deps.NewID()
			}
			request := api.CreatePublicationRequest{
				ClientRequestID: requestID,
				ResolutionID:    opts.resolutionID,
				Selector:        opts.skillRoot,
				Destination:     destination,
				Options: api.PublicationOptions{
					TargetLocale: opts.targetLocale,
					PublishMode:  "auto",
				},
			}
			if opts.resolutionID == "" {
				source, err := publicationSource(command, runtime, args, opts)
				if err != nil {
					return err
				}
				request.Source = &source
			}
			publication, err := createPublication(command.Context(), runtime, request)
			if err != nil {
				return err
			}
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
	flags.StringVar(&opts.clientRequestID, "client-request-id", "", "idempotency key for this publication action")
	return command
}

func createPublication(ctx context.Context, runtime *Runtime, request api.CreatePublicationRequest) (api.Publication, error) {
	publication, err := runtime.client().CreatePublication(ctx, request)
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
	return runtime.client().CreatePublication(ctx, request)
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
	if !opts.yes && !opts.dryRun {
		return output.Confirmation("confirmation_required", "publishing creates or updates a public share link; rerun with --yes after user confirmation")
	}
	if opts.timeout <= 0 {
		return output.Validation("timeout", "--timeout must be greater than zero")
	}
	if opts.newTarget && opts.targetID != "" {
		return output.Validation("target_flags", "--new-target and --target-id are mutually exclusive")
	}
	if opts.targetAlias != "" && !opts.newTarget {
		return output.Validation("target_alias", "--target-alias requires --new-target")
	}
	if opts.targetID == "" && opts.expectedTargetVersion != 0 {
		return output.Validation("target_version", "--expected-target-version requires --target-id")
	}
	if opts.targetID != "" && opts.expectedTargetVersion <= 0 {
		return output.Validation("target_version", "--target-id requires a positive --expected-target-version")
	}
	if (opts.file != "" || opts.directory != "") && !opts.newTarget && opts.targetID == "" {
		return output.Validation("upload_target", "uploaded input requires --new-target or --target-id with --expected-target-version")
	}
	return nil
}

func publishDestination(opts publishOptions) api.Destination {
	switch {
	case opts.newTarget:
		return api.Destination{Mode: "new", Alias: opts.targetAlias}
	case opts.targetID != "":
		return api.Destination{Mode: "existing", TargetID: opts.targetID, ExpectedTargetVersion: opts.expectedTargetVersion}
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
