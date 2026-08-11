package command

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/publication"
	"github.com/spf13/cobra"
)

type inspectResult struct {
	publication.Package
	PriceConfirmed bool `json:"priceConfirmed"`
}

func newSkillCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "skill", Short: "Inspect and publish a local Skill"}
	command.AddCommand(newSkillInspectCommand(runtime))
	command.AddCommand(newSkillPublishCommand(runtime))
	return command
}

func newSkillInspectCommand(runtime *Runtime) *cobra.Command {
	var source string
	command := &cobra.Command{
		Use: "inspect", Short: "Validate a local Skill directory or ZIP without side effects", Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			result, err := publication.Build(source, 0)
			if err != nil {
				return err
			}
			return runtime.business(inspectResult{Package: result, PriceConfirmed: false})
		},
	}
	command.Flags().StringVar(&source, "path", "", "Skill directory or ZIP")
	_ = command.MarkFlagRequired("path")
	return command
}

func newSkillPublishCommand(runtime *Runtime) *cobra.Command {
	var source string
	var resume string
	var priceMinor int
	var productID string
	var creatorDisplayName string
	var dryRun bool
	command := &cobra.Command{
		Use: "publish", Short: "Upload a Skill and prepare its listing for explicit review", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if source != "" && resume != "" {
				return output.Validation("PUBLICATION_FLAGS_CONFLICT", "--path and --resume cannot be used together")
			}
			if source == "" && resume == "" {
				return output.Validation("SKILL_PATH_REQUIRED", "provide --path or --resume")
			}
			if dryRun && resume != "" {
				return output.Validation("PUBLICATION_FLAGS_CONFLICT", "--dry-run cannot be combined with --resume")
			}
			store := publication.PendingStore{Directory: filepath.Join(runtime.configBase, "publications"), Now: runtime.deps.Now}
			if resume != "" {
				pending, err := store.Load(resume)
				if err != nil {
					return err
				}
				pkg, err := publication.Build(pending.SourcePath, pending.PriceMinor)
				if err != nil {
					return err
				}
				if pkg.Artifact.Digest != pending.ArtifactDigest {
					return output.Validation("PUBLICATION_SOURCE_CHANGED", "local Skill source changed after the publication started").WithHint("restore the original source or start a new publication")
				}
				return continueSkillPublication(command.Context(), runtime, store, pending, pkg, nil)
			}
			if !command.Flags().Changed("price-minor") {
				return output.Confirmation("SKILL_PRICE_CONFIRMATION_REQUIRED", "provide the explicitly confirmed CNY price using --price-minor")
			}
			pkg, err := publication.Build(source, priceMinor)
			if err != nil {
				return err
			}
			if dryRun {
				return runtime.business(inspectResult{Package: pkg, PriceConfirmed: true})
			}
			fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(
				runtime.profile.ID+"\x00"+pkg.SourcePath+"\x00"+pkg.Artifact.Digest+"\x00"+pkg.Digest+"\x00"+
					fmt.Sprintf("%d", priceMinor)+"\x00"+productID+"\x00"+creatorDisplayName+"\x00"+buildinfo.Version,
			)))
			intent, err := store.LoadOrCreateIntent(fingerprint, runtime.deps.NewID)
			if err != nil {
				return err
			}
			if intent.PublicationID != "" {
				pending := publication.Pending{
					PublicationID: intent.PublicationID, ClientRequestID: intent.ClientRequestID,
					Fingerprint: fingerprint,
					SourcePath:  pkg.SourcePath, PriceMinor: priceMinor, ArtifactDigest: pkg.Artifact.Digest,
				}
				if err := store.Save(pending); err != nil {
					return err
				}
				return continueSkillPublication(command.Context(), runtime, store, pending, pkg, nil)
			}
			created, err := runtime.client().CreateSkillPublication(command.Context(), api.CreateSkillPublicationRequest{
				ClientRequestID: intent.ClientRequestID, ContractVersion: "2026-08-10", CLIVersion: buildinfo.Version,
				Manifest: pkg.Manifest, ManifestDigest: pkg.Digest, Artifact: pkg.Artifact,
				ProductID: productID, CreatorDisplayName: creatorDisplayName,
			})
			if err != nil {
				return err
			}
			intent.PublicationID = created.PublicationID
			if err := store.SaveIntent(intent); err != nil {
				return err
			}
			pending := publication.Pending{
				PublicationID: created.PublicationID, ClientRequestID: intent.ClientRequestID,
				Fingerprint: fingerprint,
				SourcePath:  pkg.SourcePath, PriceMinor: priceMinor, ArtifactDigest: pkg.Artifact.Digest,
			}
			if err := store.Save(pending); err != nil {
				return err
			}
			return continueSkillPublication(command.Context(), runtime, store, pending, pkg, created.PackageUpload)
		},
	}
	command.Flags().StringVar(&source, "path", "", "Skill directory or ZIP")
	command.Flags().StringVar(&resume, "resume", "", "resume an interrupted publication by ID")
	command.Flags().IntVar(&priceMinor, "price-minor", 0, "confirmed CNY price in fen")
	command.Flags().StringVar(&productID, "product-id", "", "update an owned product instead of creating one")
	command.Flags().StringVar(&creatorDisplayName, "creator-display-name", "", "creator display name used when the account has none")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "validate and show the deterministic plan without network writes")
	return command
}

func continueSkillPublication(ctx context.Context, runtime *Runtime, store publication.PendingStore, pending publication.Pending, pkg publication.Package, initialUpload *api.UploadAuthorization) error {
	client := runtime.client()
	current, err := client.GetSkillPublication(ctx, pending.PublicationID)
	if err != nil {
		return err
	}
	if current.Status == "PUBLISHED" || current.Status == "CANCELLED" {
		retirePublicationRecovery(store, pending)
		return runtime.business(current)
	}
	if !verifiedUpload(current.Uploads, "PACKAGE", pkg.Artifact.Digest, "") {
		authorization := initialUpload
		if authorization == nil {
			value, err := client.AuthorizeUpload(ctx, pending.PublicationID, api.UploadAuthorizationRequest{
				Kind: "PACKAGE", Digest: pkg.Artifact.Digest, SizeBytes: pkg.Artifact.SizeBytes,
				FileName: pkg.Artifact.FileName, ContentType: pkg.Artifact.ContentType, SortOrder: 0,
			})
			if err != nil {
				return err
			}
			authorization = &value
		}
		progress(runtime, "Uploading deterministic Skill package")
		if err := client.PutUpload(ctx, *authorization, bytes.NewReader(pkg.Bytes), int64(len(pkg.Bytes))); err != nil {
			return err
		}
		current, err = client.CompleteUpload(ctx, pending.PublicationID, authorization.UploadID)
		if err != nil {
			return err
		}
	}
	for index, candidate := range pkg.Candidates {
		if verifiedUpload(current.Uploads, "MEDIA", candidate.Digest, candidate.RelativePath) {
			continue
		}
		authorization, err := client.AuthorizeUpload(ctx, pending.PublicationID, api.UploadAuthorizationRequest{
			Kind: "MEDIA", Digest: candidate.Digest, SizeBytes: candidate.SizeBytes,
			FileName: candidate.FileName, ContentType: candidate.ContentType,
			RelativePath: candidate.RelativePath, SortOrder: index,
		})
		if err != nil {
			return err
		}
		progress(runtime, fmt.Sprintf("Uploading listing candidate %d/%d", index+1, len(pkg.Candidates)))
		if err := client.PutUpload(ctx, authorization, bytes.NewReader(candidate.Bytes), candidate.SizeBytes); err != nil {
			return err
		}
		current, err = client.CompleteUpload(ctx, pending.PublicationID, authorization.UploadID)
		if err != nil {
			return err
		}
	}
	if len(pkg.Candidates) > 0 && current.Analysis == nil && (current.Status == "DRAFT" || current.Status == "FAILED") {
		progress(runtime, "Requesting non-authoritative cover and gallery suggestions")
		current, err = client.AnalyzeListing(ctx, pending.PublicationID)
		if err != nil {
			return err
		}
	}
	if current.Status == "PUBLISHED" {
		retirePublicationRecovery(store, pending)
	}
	return runtime.business(current)
}

func retirePublicationRecovery(store publication.PendingStore, pending publication.Pending) {
	_ = store.RetireIntent(pending.Fingerprint, pending.PublicationID, pending.ClientRequestID)
	_ = store.Delete(pending.PublicationID)
}

func verifiedUpload(uploads []api.SkillPublicationUpload, kind, digest, relativePath string) bool {
	for _, upload := range uploads {
		if upload.Kind == kind && upload.Status == "VERIFIED" && upload.Digest == digest {
			if relativePath == "" || (upload.RelativePath != nil && *upload.RelativePath == relativePath) {
				return true
			}
		}
	}
	return false
}

func progress(runtime *Runtime, message string) {
	_, _ = fmt.Fprintln(runtime.deps.ErrOut, message+"...")
}
