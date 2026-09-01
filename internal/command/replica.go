package command

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/replicacontent"
	"github.com/spf13/cobra"
)

var (
	replicaUUIDPattern      = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	replicaShortCodePattern = regexp.MustCompile(`^VMR-[A-Z0-9]{20}$`)
)

type replicaPublishResult struct {
	ReplicaID   string `json:"replicaId"`
	ReplicaCode string `json:"replicaCode"`
}

func newReplicaCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "replica", Short: "Publish and install Website Replica source packages"}
	command.AddCommand(newReplicaPublishCommand(runtime))
	command.AddCommand(newReplicaInstallCommand(runtime))
	return command
}

func newReplicaPublishCommand(runtime *Runtime) *cobra.Command {
	var source, workID, title, summary string
	var priceCents int
	command := &cobra.Command{
		Use:   "publish",
		Short: "Upload a Website Replica ZIP and return its stable sharing code",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := publishReplica(command.Context(), runtime, source, workID, title, summary, priceCents)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&source, "path", "", "Website Replica ZIP path")
	command.Flags().StringVar(&workID, "work-id", "", "Website Work UUID")
	command.Flags().StringVar(&title, "title", "", "buyer-visible Replica title")
	command.Flags().StringVar(&summary, "summary", "", "buyer-visible Replica summary")
	command.Flags().IntVar(&priceCents, "price-cents", 0, "price in the market currency's minor unit")
	_ = command.MarkFlagRequired("path")
	_ = command.MarkFlagRequired("work-id")
	_ = command.MarkFlagRequired("title")
	_ = command.MarkFlagRequired("price-cents")
	return command
}

func publishReplica(ctx context.Context, runtime *Runtime, source, workID, title, summary string, priceCents int) (replicaPublishResult, error) {
	workID = strings.TrimSpace(workID)
	title = strings.TrimSpace(title)
	summary = strings.TrimSpace(summary)
	if !replicaUUIDPattern.MatchString(workID) {
		return replicaPublishResult{}, output.Validation("REPLICA_WORK_ID_INVALID", "--work-id must be a UUID")
	}
	if title == "" {
		return replicaPublishResult{}, output.Validation("REPLICA_METADATA_INVALID", "--title cannot be empty")
	}
	if priceCents < 1 || priceCents > 10_000_000 {
		return replicaPublishResult{}, output.Validation("REPLICA_PRICE_INVALID", "--price-cents must be between 1 and 10000000")
	}
	file, info, err := openReplicaArchive(source)
	if err != nil {
		return replicaPublishResult{}, err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return replicaPublishResult{}, output.Validation("REPLICA_ARCHIVE_READ_FAILED", "could not read the Website Replica ZIP").WithCause(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return replicaPublishResult{}, output.Internal("REPLICA_ARCHIVE_REWIND_FAILED", "could not rewind the Website Replica ZIP", err)
	}
	if err := runtime.requireWebsiteReplicaAuthentication(ctx, "website-replica:read", "website-replica:write"); err != nil {
		return replicaPublishResult{}, err
	}
	clientRequestID := runtime.deps.NewID()
	if !replicaUUIDPattern.MatchString(clientRequestID) {
		return replicaPublishResult{}, output.Internal("REPLICA_CLIENT_REQUEST_ID_INVALID", "could not create a valid Replica request identity", nil)
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	created, err := runtime.client().CreateWebsiteReplicaUpload(ctx, api.CreateWebsiteReplicaUploadRequest{
		ClientRequestID: clientRequestID,
		WorkID:          workID,
		Title:           title,
		Summary:         summary,
		FileName:        filepath.Base(source),
		SizeBytes:       info.Size(),
		Digest:          digest,
		PriceCents:      priceCents,
	})
	if err != nil {
		return replicaPublishResult{}, err
	}
	progress(runtime, "Uploading Website Replica source package")
	if err := runtime.client().PutUpload(ctx, api.UploadAuthorization{
		Method: created.Upload.Method, URL: created.Upload.URL,
		ExpiresAt: created.Upload.ExpiresAt, Headers: created.Upload.Headers,
	}, file, info.Size()); err != nil {
		return replicaPublishResult{}, err
	}
	completed, err := runtime.client().CompleteWebsiteReplicaUpload(ctx, created.ReplicaID, created.UploadID)
	if err != nil {
		return replicaPublishResult{}, err
	}
	if completed.ReplicaID != created.ReplicaID {
		return replicaPublishResult{}, output.Internal("REPLICA_COMPLETE_RESPONSE_INVALID", "Website Replica completion returned a different Replica", nil)
	}
	if !replicaShortCodePattern.MatchString(completed.ShortCode) {
		return replicaPublishResult{}, output.Internal("REPLICA_COMPLETE_RESPONSE_INVALID", "Website Replica completion returned an invalid sharing code", nil)
	}
	return replicaPublishResult{ReplicaID: created.ReplicaID, ReplicaCode: "VICEME-REPLICA:" + completed.ShortCode}, nil
}

func openReplicaArchive(filename string) (*os.File, fs.FileInfo, error) {
	if !strings.EqualFold(filepath.Ext(filename), ".zip") {
		return nil, nil, output.Validation("REPLICA_ARCHIVE_INVALID", "--path must identify a ZIP file")
	}
	pathInfo, err := os.Lstat(filename)
	if err != nil {
		return nil, nil, output.Validation("REPLICA_ARCHIVE_READ_FAILED", "could not inspect the Website Replica ZIP").WithCause(err)
	}
	if !pathInfo.Mode().IsRegular() || pathInfo.Mode()&os.ModeSymlink != 0 {
		return nil, nil, output.Validation("REPLICA_ARCHIVE_INVALID", "Website Replica source must be a regular ZIP file")
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, nil, output.Validation("REPLICA_ARCHIVE_READ_FAILED", "could not open the Website Replica ZIP").WithCause(err)
	}
	fail := func(err error) (*os.File, fs.FileInfo, error) {
		_ = file.Close()
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		return fail(output.Validation("REPLICA_ARCHIVE_READ_FAILED", "could not inspect the opened Website Replica ZIP").WithCause(err))
	}
	if !info.Mode().IsRegular() || !os.SameFile(pathInfo, info) {
		return fail(output.Validation("REPLICA_ARCHIVE_INVALID", "Website Replica source changed while it was opened"))
	}
	if info.Size() <= 0 || info.Size() > replicacontent.MaxArchiveBytes {
		return fail(output.Validation("REPLICA_ARCHIVE_SIZE_INVALID", fmt.Sprintf("Website Replica ZIP must be between 1 byte and %d bytes", replicacontent.MaxArchiveBytes)))
	}
	if _, err := zip.NewReader(file, info.Size()); err != nil {
		return fail(output.Validation("REPLICA_ARCHIVE_INVALID", "Website Replica source is not a valid ZIP archive").WithCause(err))
	}
	return file, info, nil
}

func (runtime *Runtime) requireWebsiteReplicaAuthentication(ctx context.Context, required ...string) error {
	status, err := runtime.client().AuthStatus(ctx)
	if err != nil {
		return err
	}
	if !status.Authenticated {
		return output.Authentication("NOT_LOGGED_IN", "sign in before using Website Replicas").
			WithHint("run 'viceme auth login' for the current profile")
	}
	available := make(map[string]struct{}, len(status.Scopes))
	for _, scope := range status.Scopes {
		available[scope] = struct{}{}
	}
	missing := make([]string, 0, len(required))
	for _, scope := range required {
		if _, ok := available[scope]; !ok {
			missing = append(missing, scope)
		}
	}
	if len(missing) != 0 {
		return output.Authorization("REPLICA_SCOPE_REQUIRED", "the current login is not authorized for this Website Replica operation").
			WithHint("run 'viceme auth login' again for the current profile to grant Website Replica access").
			WithDetails(map[string]any{"profile": runtime.profile.Name, "missingScopes": missing})
	}
	return nil
}
