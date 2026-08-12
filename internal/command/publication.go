package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/publication"
	"github.com/spf13/cobra"
)

func newPublicationCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "publication", Short: "Review and complete an in-progress Skill publication"}
	command.AddCommand(newPublicationGetCommand(runtime))
	command.AddCommand(newPublicationWaitCommand(runtime))
	command.AddCommand(newPublicationReviewCommand(runtime))
	command.AddCommand(newPublicationAssetCommand(runtime))
	command.AddCommand(newPublicationUpdateCommand(runtime))
	command.AddCommand(newPublicationConfirmCommand(runtime))
	command.AddCommand(newPublicationPublishCommand(runtime))
	command.AddCommand(newPublicationCancelCommand(runtime))
	return command
}

func newPublicationWaitCommand(runtime *Runtime) *cobra.Command {
	var timeout time.Duration
	var interval time.Duration
	command := &cobra.Command{
		Use:   "wait <publication-id>",
		Short: "Wait for listing analysis to reach a terminal state",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if timeout <= 0 {
				return output.Validation("PUBLICATION_WAIT_TIMEOUT_INVALID", "--timeout must be greater than zero")
			}
			if interval < time.Second || interval > 30*time.Second {
				return output.Validation("PUBLICATION_WAIT_INTERVAL_INVALID", "--interval must be between 1s and 30s")
			}
			ctx, cancel := context.WithTimeout(command.Context(), timeout)
			defer cancel()
			client := runtime.client()
			progress(runtime, "Waiting for ViceMe listing analysis to finish")
			for {
				result, err := client.GetSkillPublication(ctx, args[0])
				if err != nil {
					if errors.Is(ctx.Err(), context.DeadlineExceeded) {
						return publicationWaitTimeout(args[0], timeout)
					}
					return err
				}
				if result.Analysis == nil || result.Analysis.Status != "PENDING" {
					return runtime.business(result)
				}
				if err := runtime.deps.Sleep(ctx, interval); err != nil {
					if errors.Is(ctx.Err(), context.DeadlineExceeded) {
						return publicationWaitTimeout(args[0], timeout)
					}
					return err
				}
			}
		},
	}
	command.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "maximum time to wait for listing analysis")
	command.Flags().DurationVar(&interval, "interval", 5*time.Second, "poll interval while listing analysis is pending")
	return command
}

func publicationWaitTimeout(publicationID string, timeout time.Duration) error {
	return output.Network(
		"PUBLICATION_ANALYSIS_WAIT_TIMEOUT",
		"listing analysis is still pending after the wait deadline",
		context.DeadlineExceeded,
	).WithDetails(map[string]any{
		"publicationId":  publicationID,
		"timeoutSeconds": int64(timeout / time.Second),
	}).WithHint("retry publication wait with the same publication ID; do not upload the package again")
}

func newPublicationGetCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use: "get <publication-id>", Short: "Get authoritative publication state", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			result, err := runtime.client().GetSkillPublication(command.Context(), args[0])
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
}

func newPublicationReviewCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use: "review <publication-id>", Short: "Show bilingual summaries, usage instructions, price, media, analysis suggestions, and review digest", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			result, err := runtime.client().GetSkillPublication(command.Context(), args[0])
			if err != nil {
				return err
			}
			return runtime.business(map[string]any{
				"publicationId": result.ID, "status": result.Status, "draft": result.Draft,
				"analysis": result.Analysis, "uploads": result.Uploads,
				"reviewRevision": result.ReviewRevision, "reviewDigest": result.ReviewDigest,
				"requiresExplicitConfirmation": result.Status == "REVIEW_REQUIRED",
			})
		},
	}
}

func newPublicationAssetCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "asset", Short: "Manage publication cover and gallery assets"}
	command.AddCommand(newPublicationAssetUploadCommand(runtime))
	return command
}

func newPublicationAssetUploadCommand(runtime *Runtime) *cobra.Command {
	var role string
	var filename string
	command := &cobra.Command{
		Use: "upload <publication-id>", Short: "Upload a replacement cover or gallery image", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if role != "cover" && role != "gallery" {
				return output.Validation("MEDIA_ROLE_INVALID", "--role must be cover or gallery")
			}
			candidate, err := publication.ReadCandidate(filename)
			if err != nil {
				return err
			}
			client := runtime.client()
			current, err := client.GetSkillPublication(command.Context(), args[0])
			if err != nil {
				return err
			}
			matched := matchingMediaUpload(current.Uploads, candidate)
			uploadedID := ""
			if matched != nil && matched.Status == "VERIFIED" {
				uploadedID = matched.ID
			}
			if uploadedID == "" {
				slot := 0
				if matched != nil {
					slot = matched.SortOrder
				} else {
					var slotErr error
					slot, slotErr = firstAvailableMediaSlot(current.Uploads)
					if slotErr != nil {
						return slotErr
					}
				}
				authorization, err := client.AuthorizeUpload(command.Context(), args[0], api.UploadAuthorizationRequest{
					Kind: "MEDIA", Digest: candidate.Digest, SizeBytes: candidate.SizeBytes,
					FileName: candidate.FileName, ContentType: candidate.ContentType, SortOrder: slot,
				})
				if err != nil {
					return err
				}
				progress(runtime, "Uploading replacement listing media")
				if err := client.PutUpload(command.Context(), authorization, bytes.NewReader(candidate.Bytes), candidate.SizeBytes); err != nil {
					return err
				}
				current, err = client.CompleteUpload(command.Context(), args[0], authorization.UploadID)
				if err != nil {
					return err
				}
				for _, upload := range current.Uploads {
					if upload.ID == authorization.UploadID && upload.Status == "VERIFIED" {
						uploadedID = upload.ID
						break
					}
				}
			}
			if uploadedID == "" {
				return output.Internal("MEDIA_UPLOAD_NOT_VERIFIED", "uploaded media was not returned as verified", nil)
			}
			draft := current.Draft
			if role == "cover" {
				draft.CoverUploadID = &uploadedID
			} else if !containsString(draft.GalleryUploadIDs, uploadedID) {
				draft.GalleryUploadIDs = append(draft.GalleryUploadIDs, uploadedID)
			}
			updated, err := client.UpdateListingDraft(command.Context(), args[0], draft)
			if err != nil {
				return err
			}
			return runtime.business(updated)
		},
	}
	command.Flags().StringVar(&role, "role", "", "asset role: cover or gallery")
	command.Flags().StringVar(&filename, "path", "", "image file")
	_ = command.MarkFlagRequired("role")
	_ = command.MarkFlagRequired("path")
	return command
}

func matchingMediaUpload(uploads []api.SkillPublicationUpload, candidate publication.Candidate) *api.SkillPublicationUpload {
	for _, upload := range uploads {
		if upload.Kind == "MEDIA" && upload.Digest == candidate.Digest && upload.SizeBytes == candidate.SizeBytes && upload.FileName == candidate.FileName && upload.ContentType == candidate.ContentType && upload.RelativePath == nil {
			matched := upload
			return &matched
		}
	}
	return nil
}

func newPublicationUpdateCommand(runtime *Runtime) *cobra.Command {
	var filename string
	command := &cobra.Command{
		Use: "update <publication-id>", Short: "Replace the review draft from a strict JSON file", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			draft, err := readDraftFile(filename)
			if err != nil {
				return err
			}
			result, err := runtime.client().UpdateListingDraft(command.Context(), args[0], draft)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&filename, "input", "", "strict JSON file containing the complete listing draft")
	_ = command.MarkFlagRequired("input")
	return command
}

func newPublicationConfirmCommand(runtime *Runtime) *cobra.Command {
	var digest string
	command := &cobra.Command{
		Use: "confirm <publication-id>", Short: "Explicitly confirm both summaries, price, cover, and gallery", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			result, err := runtime.client().ConfirmPublication(command.Context(), args[0], digest)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&digest, "review-digest", "", "exact digest shown by publication review")
	_ = command.MarkFlagRequired("review-digest")
	return command
}

func newPublicationPublishCommand(runtime *Runtime) *cobra.Command {
	var digest string
	command := &cobra.Command{
		Use: "publish <publication-id>", Short: "Publish a previously confirmed Skill listing", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			result, err := runtime.client().PublishSkill(command.Context(), args[0], digest)
			if err != nil {
				return err
			}
			if result.Status == "PUBLISHED" {
				store := publication.PendingStore{Directory: filepath.Join(runtime.configBase, "publications"), Now: runtime.deps.Now}
				if pending, loadErr := store.Load(args[0]); loadErr == nil {
					if err := retirePublicationRecovery(store, pending, result.Status); err != nil {
						return err
					}
				} else if output.AsError(loadErr).Subtype != "PUBLICATION_RECOVERY_NOT_FOUND" {
					return loadErr
				}
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&digest, "review-digest", "", "exact digest confirmed by the user")
	_ = command.MarkFlagRequired("review-digest")
	return command
}

func newPublicationCancelCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use: "cancel <publication-id>", Short: "Cancel an unpublished Skill publication", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			result, err := runtime.client().CancelSkillPublication(command.Context(), args[0])
			if err != nil {
				return err
			}
			store := publication.PendingStore{Directory: filepath.Join(runtime.configBase, "publications"), Now: runtime.deps.Now}
			if pending, loadErr := store.Load(args[0]); loadErr == nil {
				if err := retirePublicationRecovery(store, pending, "CANCELLED"); err != nil {
					return err
				}
			} else if output.AsError(loadErr).Subtype != "PUBLICATION_RECOVERY_NOT_FOUND" {
				return loadErr
			}
			return runtime.business(result)
		},
	}
}

func readDraftFile(filename string) (api.SkillPublicationDraft, error) {
	file, err := os.Open(filename)
	if err != nil {
		return api.SkillPublicationDraft{}, output.Validation("PUBLICATION_DRAFT_READ_FAILED", "could not open the publication draft file").WithCause(err)
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.DisallowUnknownFields()
	var draft api.SkillPublicationDraft
	if err := decoder.Decode(&draft); err != nil {
		return draft, output.Validation("PUBLICATION_DRAFT_INVALID", "publication draft must be strict JSON").WithCause(err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return draft, output.Validation("PUBLICATION_DRAFT_INVALID", "publication draft contains trailing JSON")
	}
	return draft, nil
}

func firstAvailableMediaSlot(uploads []api.SkillPublicationUpload) (int, error) {
	used := make(map[int]struct{})
	for _, upload := range uploads {
		if upload.Kind == "MEDIA" {
			used[upload.SortOrder] = struct{}{}
		}
	}
	for slot := 0; slot < 12; slot++ {
		if _, exists := used[slot]; !exists {
			return slot, nil
		}
	}
	return 0, output.Validation("MEDIA_LIMIT_REACHED", "publication already has the maximum 12 media uploads")
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
