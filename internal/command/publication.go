package command

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/publication"
	"github.com/spf13/cobra"
)

func newPublicationCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "publication", Short: "Review and complete an in-progress Skill publication"}
	command.AddCommand(newPublicationGetCommand(runtime))
	command.AddCommand(newPublicationReviewCommand(runtime))
	command.AddCommand(newPublicationAssetCommand(runtime))
	command.AddCommand(newPublicationUpdateCommand(runtime))
	command.AddCommand(newPublicationConfirmCommand(runtime))
	command.AddCommand(newPublicationPublishCommand(runtime))
	command.AddCommand(newPublicationCancelCommand(runtime))
	return command
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
		Use: "review <publication-id>", Short: "Show price, cover, gallery, analysis suggestions, and review digest", Args: cobra.ExactArgs(1),
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
			uploadedID := verifiedMediaUploadID(current.Uploads, candidate.Digest)
			if uploadedID == "" {
				slot, err := firstAvailableMediaSlot(current.Uploads)
				if err != nil {
					return err
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

func verifiedMediaUploadID(uploads []api.SkillPublicationUpload, digest string) string {
	for _, upload := range uploads {
		if upload.Kind == "MEDIA" && upload.Status == "VERIFIED" && upload.Digest == digest {
			return upload.ID
		}
	}
	return ""
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
		Use: "confirm <publication-id>", Short: "Explicitly confirm the current price, cover, and gallery", Args: cobra.ExactArgs(1),
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
				_ = store.Delete(args[0])
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
			_ = store.Delete(args[0])
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
