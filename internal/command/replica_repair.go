package command

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/pagepackage"
	"github.com/spf13/cobra"
)

type replicaRepairReview struct {
	Authority       string                              `json:"authority"`
	PublicationID   string                              `json:"publicationId"`
	SourceVersionID string                              `json:"sourceVersionId"`
	WorkURL         string                              `json:"workUrl"`
	Request         api.WebsiteReplicaPageRepairRequest `json:"request"`
	ExpiresAt       time.Time                           `json:"expiresAt"`
}

func newReplicaRepairHostingCommand(runtime *Runtime) *cobra.Command {
	var publicationID, path, confirm, snapshot string
	command := &cobra.Command{Use: "repair-hosting", Short: "Review and repair page hosting without publishing new source", Args: cobra.NoArgs}
	command.Flags().StringVar(&publicationID, "publication", "", "degraded Publication UUID")
	command.Flags().StringVar(&path, "path", "", "repaired project with static output, or validated WorkPage ZIP")
	command.Flags().StringVar(&confirm, "confirm", "", "exact review digest")
	command.Flags().StringVar(&snapshot, "request", "", "exact request snapshot from the review")
	_ = command.MarkFlagRequired("publication")
	command.RunE = func(cmd *cobra.Command, _ []string) error {
		if err := requireReplicaPublicationCN(runtime); err != nil {
			return err
		}
		if !replicaUUIDPattern.MatchString(publicationID) {
			return output.Validation("REPLICA_PUBLICATION_ID_INVALID", "--publication must be a UUID")
		}
		if path == "" {
			return output.Validation("REPLICA_REPAIR_PATH_REQUIRED", "repair and preview the local page before selecting --path").WithDetails(map[string]any{"nextAction": "PREPARE_HOSTING_REPAIR", "publicationId": publicationID})
		}
		var pkg pagepackage.Package
		var err error
		if strings.EqualFold(filepath.Ext(path), ".zip") {
			pkg, err = pagepackage.Inspect(path)
		} else {
			pkg, _, err = pagepackage.BuildWebsiteWorkPage(path, "Repaired website")
		}
		if err != nil {
			return err
		}
		if len(pkg.Bytes) == 0 || pkg.Manifest.Kind != "WorkPage" {
			return output.Validation("REPLICA_REPAIR_PAGE_REQUIRED", "repair requires a valid static WorkPage artifact")
		}
		artifact := api.WebsiteReplicaPublicationSourceArtifact{FileName: pkg.Artifact.FileName, ContentType: pkg.Artifact.ContentType, SizeBytes: pkg.Artifact.SizeBytes, Digest: pkg.Artifact.Digest}
		var review replicaRepairReview
		if confirm != "" {
			raw, err := base64.RawURLEncoding.DecodeString(snapshot)
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.DisallowUnknownFields()
			if err != nil || decoder.Decode(&review) != nil || decoder.Decode(new(any)) != io.EOF {
				return output.Validation("REPLICA_REPAIR_CONFIRMATION_INVALID", "use the exact returned confirmation command")
			}
			canonical, _ := json.Marshal(review)
			sum := sha256.Sum256(canonical)
			if hex.EncodeToString(sum[:]) != confirm || review.Authority != runtime.client().BaseURL || review.PublicationID != publicationID || !replicaUUIDPattern.MatchString(review.SourceVersionID) || !replicaUUIDPattern.MatchString(review.Request.ClientRequestID) || !reflect.DeepEqual(review.Request.Page, artifact) {
				return output.Validation("REPLICA_REPAIR_CONFIRMATION_INVALID", "page bytes or reviewed target changed; request a new review")
			}
			if !runtime.deps.Now().Before(review.ExpiresAt) || review.ExpiresAt.After(runtime.deps.Now().Add(30*time.Minute)) {
				return output.Validation("REPLICA_REPAIR_CONFIRMATION_EXPIRED", "repair confirmation expired; inspect publication status and request a new review")
			}
		} else if snapshot != "" {
			return output.Validation("REPLICA_REPAIR_CONFIRMATION_INVALID", "--request requires --confirm")
		}
		if err := runtime.requireWebsiteReplicaAuthentication(cmd.Context(), "website-replica:read", "website-replica:write"); err != nil {
			return err
		}
		if confirm == "" {
			publication, err := runtime.client().GetWebsiteReplicaPublication(cmd.Context(), publicationID)
			if err != nil {
				return err
			}
			if publication.Status != "PUBLISHED_DEGRADED" || publication.Result == nil || publication.Rollback.ActivePair == nil || publication.Rollback.ActivePair.ReplicaVersion.ID != publication.Result.VersionID {
				return output.Policy("REPLICA_REPAIR_NOT_ALLOWED", "only the current degraded source version can repair hosting")
			}
			requestID := runtime.deps.NewID()
			if !replicaUUIDPattern.MatchString(requestID) {
				return output.Internal("REPLICA_CLIENT_REQUEST_ID_INVALID", "could not create request identity", nil)
			}
			review = replicaRepairReview{Authority: runtime.client().BaseURL, PublicationID: publicationID, SourceVersionID: publication.Result.VersionID, WorkURL: publication.Result.WorkURL, Request: api.WebsiteReplicaPageRepairRequest{ClientRequestID: requestID, Page: artifact}, ExpiresAt: runtime.deps.Now().UTC().Add(30 * time.Minute)}
			encoded, _ := json.Marshal(review)
			sum := sha256.Sum256(encoded)
			args := []string{"replica", "repair-hosting", "--publication", publicationID, "--path", path, "--confirm", hex.EncodeToString(sum[:]), "--request", base64.RawURLEncoding.EncodeToString(encoded)}
			quoted := []string{"viceme"}
			for _, arg := range args {
				quoted = append(quoted, shellQuote(arg))
			}
			return output.Confirmation("REPLICA_REPAIR_CONFIRMATION_REQUIRED", "review the repaired page and existing source version before uploading").WithDetails(map[string]any{"nextAction": "CONFIRM_HOSTING_REPAIR", "review": review, "impact": "Only page hosting changes. Source version, price and buyer rights remain unchanged; the original degraded audit is retained.", "confirmCommand": strings.Join(quoted, " "), "confirmArgs": args})
		}
		// Replaying create returns the authoritative repair for the same immutable
		// request. No new source, Publication or sales mutation is involved.
		repair, err := runtime.client().CreateWebsiteReplicaPageRepair(cmd.Context(), publicationID, review.Request)
		if err != nil {
			return err
		}
		if repair.Status == "WAITING_UPLOAD" {
			authorization, err := runtime.client().AuthorizeWebsiteReplicaPageRepairUpload(cmd.Context(), publicationID, repair.ID)
			if err != nil {
				return err
			}
			upload := authorization.Upload
			if err := runtime.client().PutUpload(cmd.Context(), api.UploadAuthorization{Method: upload.Method, URL: upload.URL, Headers: upload.Headers, ExpiresAt: upload.ExpiresAt}, bytes.NewReader(pkg.Bytes), int64(len(pkg.Bytes))); err != nil {
				return err
			}
		}
		if repair.Status != "PUBLISHED" && repair.Status != "FAILED" {
			repair, err = runtime.client().CompleteWebsiteReplicaPageRepairUpload(cmd.Context(), publicationID, repair.ID)
			if err != nil {
				return err
			}
		}
		if repair.Result != nil && repair.Result.ReplicaVersionID != review.SourceVersionID {
			return output.Policy("RESPONSE_INVALID", "repair returned a different source version")
		}
		next := "RESUME_HOSTING_REPAIR"
		if repair.Status == "PUBLISHED" {
			next = "HOSTING_REPAIRED"
		} else if repair.Status == "FAILED" {
			next = "PREPARE_HOSTING_REPAIR"
		}
		return runtime.business(struct {
			NextAction string                       `json:"nextAction"`
			Repair     api.WebsiteReplicaPageRepair `json:"repair"`
		}{next, repair})
	}
	return command
}
