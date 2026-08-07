package command

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/appmanifest"
	"github.com/ViceMe-AI/cli/internal/archive"
	"github.com/ViceMe-AI/cli/internal/managedrelease"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

// Managed App command surface for Slice E (skill-app-platform.md §10/§11).
//
//   viceme app init --template <name> --runtime-release <id> --output <dir>
//   viceme app preview [--dir <path>] --yes
//   viceme app publish [--dir <path>] --yes
//
// The API never executes untrusted author code, so the CLI builds the static
// dist locally, then uploads the source archive and the built artifact archive
// via multipart/form-data.

// ManagedAppBuilder builds a managed Skill App project in place: it installs
// dependencies and produces a static dist. The default implementation runs
// `pnpm install --ignore-scripts && pnpm build`; tests override it to avoid a
// real toolchain dependency.
type ManagedAppBuilder func(ctx context.Context, dir string) error

func defaultManagedAppBuilder(ctx context.Context, dir string) error {
	for _, command := range [][]string{
		{"pnpm", "install", "--ignore-scripts"},
		{"pnpm", "build"},
	} {
		if err := runBuildStep(ctx, dir, command[0], command[1:]...); err != nil {
			return err
		}
	}
	return nil
}

func runBuildStep(ctx context.Context, dir, name string, args ...string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return output.Internal("app_build_failed", fmt.Sprintf("%s %s failed in %s", name, strings.Join(args, " "), dir), err).
			WithHint("install the required toolchain (pnpm) and rerun 'viceme app preview'")
	}
	return nil
}

func managedAppBuilder(deps Dependencies) ManagedAppBuilder {
	if deps.ManagedAppBuilder != nil {
		return deps.ManagedAppBuilder
	}
	return defaultManagedAppBuilder
}

func newManagedAppCommands(runtime *Runtime) []*cobra.Command {
	var commands []*cobra.Command

	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a managed Skill App from a template",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runManagedAppInit(cmd, runtime)
		},
	}
	initCmd.Flags().String("template", "image-tool-starter", "Template name")
	initCmd.Flags().String("runtime-release", "", "Runtime Release ID (required)")
	initCmd.Flags().String("name", "", "App name")
	initCmd.Flags().String("output", ".", "Output directory for generated site")
	_ = initCmd.MarkFlagRequired("runtime-release")
	commands = append(commands, initCmd)

	previewCmd := &cobra.Command{
		Use:   "preview",
		Short: "Build, upload and create a preview of the managed Skill App",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runManagedAppPreview(cmd, runtime)
		},
	}
	previewCmd.Flags().String("dir", ".", "Project directory containing .viceme/managed-release.json")
	previewCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")
	commands = append(commands, previewCmd)

	publishCmd := &cobra.Command{
		Use:   "publish",
		Short: "Publish a managed Skill App release",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runManagedAppPublish(cmd, runtime)
		},
	}
	publishCmd.Flags().String("dir", ".", "Project directory containing .viceme/managed-release.json")
	publishCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")
	commands = append(commands, publishCmd)

	return commands
}

func runManagedAppInit(cmd *cobra.Command, runtime *Runtime) error {
	ctx := cmd.Context()
	client := runtime.client()

	template, _ := cmd.Flags().GetString("template")
	runtimeReleaseID, _ := cmd.Flags().GetString("runtime-release")
	outputDir, _ := cmd.Flags().GetString("output")
	name, _ := cmd.Flags().GetString("name")

	// Fetch template metadata (downloadUrl + digest) and the runtime contract
	// (contractDigest echoed back on source upload).
	tpl, err := client.GetManagedAppTemplate(ctx, template)
	if err != nil {
		return err
	}
	contract, err := client.GetManagedAppRuntimeContract(ctx, runtimeReleaseID)
	if err != nil {
		return err
	}
	if contract.RuntimeReleaseID != runtimeReleaseID {
		return output.Validation("runtime_release_mismatch", "the runtime contract does not match the requested Runtime Release")
	}
	// The Shop rejects an empty name; default to the template name so a bare
	// `viceme app init` works out of the box (P1 fix).
	if name == "" {
		name = tpl.Name
	}

	// Resolve and create the output directory before downloading the template.
	projectDir, err := resolveProjectDirectoryOrCreate(outputDir)
	if err != nil {
		return err
	}

	// Idempotency and resumability: an interrupted init must be retryable with
	// the SAME clientRequestId so it converges on the same Shop App instead of
	// creating an orphan. The pending marker (.viceme/managed-release.json) is
	// the on-disk source of truth that an init is in flight; it is written
	// BEFORE the template is downloaded/extracted and before InitManagedApp, so
	// every later failure point (download, extract, inject, InitManagedApp) can
	// resume with the same key (P1 fix).
	//
	// The marker may hold two shapes: the partial pending state (only
	// schemaVersion + clientRequestId + init parameters, written before
	// InitManagedApp) or the full state (written after a successful init).
	// Load() only accepts the full state; a partial state fails validate() —
	// that failure is the "an init is in flight" signal, so it must not be
	// treated as fatal here.
	existing, loadErr := managedrelease.Load(projectDir)
	var clientRequestID string
	retrying := false
	switch {
	case loadErr == nil:
		// Completed init: the directory is owned by a finished project; the
		// guard below refuses to re-extract over it.
		clientRequestID = existing.ClientRequestID
	case errors.Is(loadErr, managedrelease.ErrNotFound):
		// First run: no marker yet, nothing to resume.
	default:
		// Interrupted init (partial marker) or an unreadable marker. Resume
		// only when the marker still parses and carries the idempotency key —
		// and only when the CURRENT command resumes the SAME init: a changed
		// template/runtime-release would converge on the old App while the
		// local state records the new parameters, corrupting preview/publish
		// (P2 fix).
		pending, pendingErr := managedrelease.LoadPending(projectDir)
		if pendingErr != nil {
			return output.Internal("managed_release_pending", "the init marker cannot be read; remove the .viceme directory and retry", pendingErr).
				WithDetails(map[string]any{"directory": projectDir})
		}
		clientRequestID = pending.ClientRequestID
		retrying = clientRequestID != ""
		if retrying &&
			(pending.TemplateName != tpl.Name ||
				pending.TemplateVersion != tpl.Version ||
				pending.RuntimeReleaseID != runtimeReleaseID) {
			return output.Validation("init_retry_mismatch",
				"the interrupted init used different template/runtime-release parameters; remove the .viceme directory and retry").
				WithDetails(map[string]any{
					"directory":        projectDir,
					"pendingTemplate":  pending.TemplateName,
					"pendingVersion":   pending.TemplateVersion,
					"pendingReleaseID": pending.RuntimeReleaseID,
				})
		}
	}
	if !directoryEmptyOrRetryMarker(projectDir, retrying) {
		return output.Validation("init_output_not_empty",
			"the output directory is not empty; choose an empty directory or rerun the same command on the interrupted project").
			WithDetails(map[string]any{"directory": projectDir})
	}
	if clientRequestID == "" {
		clientRequestID = generateClientRequestID()
		if err := managedrelease.SavePending(projectDir, managedrelease.PendingInit{
			ClientRequestID:  clientRequestID,
			TemplateName:     tpl.Name,
			TemplateVersion:  tpl.Version,
			RuntimeReleaseID: runtimeReleaseID,
		}); err != nil {
			return output.Internal("managed_release_pending", "failed to persist the init idempotency key", err)
		}
	}

	// Security: download, verify the digest and extract the template BEFORE
	// creating the remote App. If the digest mismatches (tampered archive or a
	// stale catalog) we abort here without ever calling InitManagedApp, so a bad
	// template can never leave an orphan App/candidate on the server.
	archiveBytes, err := downloadAndVerifyDigest(ctx, runtime, tpl.DownloadURL, tpl.Digest)
	if err != nil {
		return err
	}
	if err := extractZipArchive(archiveBytes, projectDir); err != nil {
		return output.Internal("template_extract", "failed to extract the template archive", err)
	}
	// The template's runtime entry needs the actual Release id; swap the
	// placeholder the template ships with. Done before any remote side effect
	// so a failed inject is a local-only failure.
	if err := injectTemplateRuntimeReleaseID(projectDir, runtimeReleaseID); err != nil {
		return output.Internal("template_release_inject", "failed to inject the Runtime Release id into the template", err)
	}

	// Initialize the App/release candidate. The Shop returns the appId,
	// candidateId, publishableKey and environment the project will be bound to.
	initResp, err := client.InitManagedApp(ctx, api.InitManagedAppRequest{
		ClientRequestID:  clientRequestID,
		Name:             name,
		RuntimeReleaseID: runtimeReleaseID,
		TemplateName:     tpl.Name,
		TemplateVersion:  tpl.Version,
	})
	if err != nil {
		return err
	}

	// Persist the Creator App binding so `app doctor` and capability commands
	// continue to work. Managed apps are VICEME_HOSTED with an empty capability
	// set until capabilities are added through the control plane.
	manifestPath, err := appmanifest.Save(projectDir, appmanifest.Manifest{
		SchemaVersion:  appmanifest.SchemaVersion,
		AppID:          initResp.AppID,
		HostingMode:    "VICEME_HOSTED",
		Environment:    initResp.Environment,
		PublishableKey: initResp.PublishableKey,
		Capabilities:   map[string]appmanifest.Capability{},
	})
	if err != nil {
		return output.Internal("app_manifest_save", "the managed App was initialized but the local manifest could not be saved", err).
			WithHint("fix local project permissions and rerun the same 'viceme app init' command; creation is idempotent")
	}

	// Persist the release-candidate state for preview/publish.
	releasePath, err := managedrelease.Save(projectDir, managedrelease.State{
		AppID:                 initResp.AppID,
		ReleaseID:             initResp.ReleaseID,
		CandidateID:           initResp.CandidateID,
		Environment:           initResp.Environment,
		PublishableKey:        initResp.PublishableKey,
		RuntimeReleaseID:      runtimeReleaseID,
		RuntimeContractDigest: contract.ContractDigest,
		TemplateName:          tpl.Name,
		TemplateVersion:       tpl.Version,
		TemplateDigest:        tpl.Digest,
	})
	if err != nil {
		return output.Internal("managed_release_save", "the managed App was initialized but the release state could not be saved", err)
	}

	return runtime.business(map[string]any{
		"app":             initResp,
		"runtimeContract": contract,
		"template":        tpl,
		"manifest":        manifestPath,
		"release":         releasePath,
	})
}

func runManagedAppPreview(cmd *cobra.Command, runtime *Runtime) error {
	ctx := cmd.Context()
	dir, _ := cmd.Flags().GetString("dir")
	skipConfirm, _ := cmd.Flags().GetBool("yes")

	projectDir, err := resolveProjectDirectory(dir)
	if err != nil {
		return err
	}
	state, err := managedrelease.Load(projectDir)
	if err != nil {
		return output.Validation("managed_release_missing", err.Error())
	}
	if state.RuntimeContractDigest == "" || state.TemplateDigest == "" {
		return output.Validation("managed_release_incomplete", "managed release state is missing digest information; rerun 'viceme app init'").WithDetails(map[string]any{"app_id": state.AppID})
	}

	// Confirm before building/uploading unless --yes was supplied. The build
	// runs author-controlled install scripts only if --yes is set; by default we
	// use --ignore-scripts to avoid executing untrusted code without consent.
	if !skipConfirm {
		confirmed, confirmErr := confirmManagedPreview(cmd, state)
		if confirmErr != nil {
			return confirmErr
		}
		if !confirmed {
			return output.Validation("preview_cancelled", "preview cancelled by the user")
		}
	}

	// Build the static dist locally. The API never runs author code, so the CLI
	// owns this step and uploads the resulting artifact.
	builder := managedAppBuilder(runtime.deps)
	if err := builder(ctx, projectDir); err != nil {
		return err
	}

	client := runtime.client()

	// Package and upload the source snapshot.
	sourceArtifact, err := buildProjectArtifact(ctx, projectDir, false)
	if err != nil {
		return err
	}
	defer sourceArtifact.Cleanup()
	sourceResp, err := client.UploadManagedAppSource(ctx, api.UploadSourceRequest{
		AppID:                 state.AppID,
		CandidateID:           state.CandidateID,
		RuntimeReleaseID:      state.RuntimeReleaseID,
		RuntimeContractDigest: state.RuntimeContractDigest,
		TemplateName:          state.TemplateName,
		TemplateVersion:       state.TemplateVersion,
		TemplateDigest:        state.TemplateDigest,
	}, sourceArtifact.Path)
	if err != nil {
		return err
	}

	// Package and upload the built dist artifact.
	buildArtifact, err := buildProjectArtifact(ctx, projectDir, true)
	if err != nil {
		return err
	}
	defer buildArtifact.Cleanup()
	buildResp, err := client.UploadManagedAppBuildArtifact(ctx, api.UploadBuildArtifactRequest{
		AppID:       state.AppID,
		CandidateID: state.CandidateID,
	}, buildArtifact.Path)
	if err != nil {
		return err
	}

	// Create the preview run.
	previewResp, err := client.CreateManagedAppPreview(ctx, state.AppID, state.CandidateID)
	if err != nil {
		return err
	}

	// Record the digests the publish flow must echo back.
	state.SourceDigest = sourceResp.SourceDigest
	state.BuildDigest = buildResp.BuildDigest
	state.PreviewRunID = previewResp.PreviewRunID
	state.PreviewURL = previewResp.PreviewURL
	if _, err := managedrelease.Save(projectDir, state); err != nil {
		return output.Internal("managed_release_update", "the preview was created but the local release state could not be updated", err)
	}

	return runtime.business(map[string]any{
		"upload":  sourceResp,
		"build":   buildResp,
		"preview": previewResp,
	})
}

func runManagedAppPublish(cmd *cobra.Command, runtime *Runtime) error {
	ctx := cmd.Context()
	dir, _ := cmd.Flags().GetString("dir")
	skipConfirm, _ := cmd.Flags().GetBool("yes")

	projectDir, err := resolveProjectDirectory(dir)
	if err != nil {
		return err
	}
	state, err := managedrelease.Load(projectDir)
	if err != nil {
		return output.Validation("managed_release_missing", err.Error())
	}
	if state.SourceDigest == "" || state.BuildDigest == "" || state.RuntimeContractDigest == "" {
		return output.Validation("preview_required", "run 'viceme app preview' first; publish confirms the digests produced during preview").
			WithDetails(map[string]any{"app_id": state.AppID})
	}

	// Confirmation is built on the actual build artifacts, not on flags. We cite
	// the exact content digests the user reviewed during preview so the user can
	// detect any drift before the release becomes immutable.
	if !skipConfirm {
		confirmed, confirmErr := confirmManagedPublish(cmd, state)
		if confirmErr != nil {
			return confirmErr
		}
		if !confirmed {
			return output.Validation("publish_cancelled", "publish cancelled by the user")
		}
	}

	resp, err := runtime.client().PublishManagedApp(ctx, api.PublishReleaseRequest{
		AppID:                         state.AppID,
		CandidateID:                   state.CandidateID,
		ExpectedSourceDigest:          state.SourceDigest,
		ExpectedBuildDigest:           state.BuildDigest,
		ExpectedRuntimeContractDigest: state.RuntimeContractDigest,
	})
	if err != nil {
		return err
	}
	return runtime.business(map[string]any{"publish": resp})
}

func confirmManagedPreview(cmd *cobra.Command, state managedrelease.State) (bool, error) {
	return confirmf(cmd, "Build and upload the managed Skill App for App %s (candidate %s)?", state.AppID, state.CandidateID)
}

func confirmManagedPublish(cmd *cobra.Command, state managedrelease.State) (bool, error) {
	return confirmf(cmd, "Publish App %s with source %s and build %s?", state.AppID, state.SourceDigest, state.BuildDigest)
}

// confirmf prints a y/N prompt to stderr and reads a line from stdin. An empty
// or non-TTY stdin is treated as a decline so that piped/non-interactive runs
// require an explicit --yes rather than silently proceeding.
func confirmf(cmd *cobra.Command, format string, args ...any) (bool, error) {
	reader := cmd.InOrStdin()
	writer := cmd.ErrOrStderr()
	prompt := fmt.Sprintf(format, args...) + " [y/N]: "
	if _, err := fmt.Fprint(writer, prompt); err != nil {
		return false, err
	}
	buffer := make([]byte, 1024)
	n, err := reader.Read(buffer)
	if err != nil && err != io.EOF {
		return false, err
	}
	if n == 0 {
		return false, nil
	}
	answer := strings.TrimSpace(strings.ToLower(string(buffer[:n])))
	return answer == "y" || answer == "yes", nil
}

// buildProjectArtifact packages a project directory into a zip archive. When
// artifact is true, only the built dist directory is included; otherwise the
// full source tree (excluding dist and build caches) is included. The returned
// archive is temporary and must be cleaned up via Cleanup.
func buildProjectArtifact(ctx context.Context, projectDir string, artifact bool) (archive.Artifact, error) {
	maxBytes := int64(50) << 20
	if artifact {
		distDir := filepath.Join(projectDir, "dist")
		if _, err := os.Stat(distDir); err != nil {
			return archive.Artifact{}, output.Validation("dist_missing", "the build did not produce a dist/ directory; run the build before preview").
				WithDetails(map[string]any{"project_dir": projectDir})
		}
		return archive.BuildDirectory(ctx, distDir, maxBytes)
	}
	// Security: scan the source tree for secret/credential files BEFORE
	// packaging, so a leaked private key is never uploaded to the API.
	if denied, err := findDeniedSecretFiles(projectDir); err != nil {
		return archive.Artifact{}, err
	} else if len(denied) > 0 {
		return archive.Artifact{}, output.Validation("source_secret_file",
			fmt.Sprintf("refusing to upload source containing likely-secret files: %s", strings.Join(denied, ", "))).
			WithHint("remove the listed files or add them to a local ignore rule; never commit secrets into a managed App source upload")
	}
	return archive.BuildDirectory(ctx, projectDir, maxBytes)
}

// secretDenylist matches filenames that should never be uploaded as managed App
// source. The check is conservative: it flags exact names and extensions
// associated with private keys, environment secrets and cloud credentials so a
// leaked secret is rejected before it leaves the machine.
func secretDenylist() map[string]bool {
	return map[string]bool{
		".env":             true,
		".env.local":       true,
		".env.production":  true,
		".env.development": true,
		"id_rsa":           true,
		"id_dsa":           true,
		"id_ecdsa":         true,
		"id_ed25519":       true,
		"credentials":      true, // gcloud/AWS shared filename
		"credentials.json": true, // gcloud service-account key
		".npmrc":           true, // may carry _authToken
		".pypirc":          true, // may carry pypi token
		".netrc":           true, // may carry HTTP credentials
	}
}

func isDeniedSecretFile(name string) bool {
	base := strings.ToLower(filepath.Base(name))
	if secretDenylist()[base] {
		return true
	}
	switch filepath.Ext(base) {
	case ".key", ".pem", ".p12", ".keystore":
		return true
	}
	// SSH private-key companion files like id_rsa, id_ed25519 (covered above) and
	// EC private keys (covered by .pem); also catch ".env.*" variants.
	if strings.HasPrefix(base, ".env.") {
		return true
	}
	return false
}

func findDeniedSecretFiles(root string) ([]string, error) {
	var denied []string
	err := filepath.WalkDir(root, func(path string, dirEntry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return nil
			}
			return walkErr
		}
		if path == root || dirEntry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		// Skip the same directories BuildDirectory ignores (node_modules, dist,
		// .git, ...) so a stray key in a dependency does not block the upload.
		if ignoredArchivePath(rel) {
			if dirEntry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if dirEntry.IsDir() {
			return nil
		}
		if isDeniedSecretFile(rel) {
			denied = append(denied, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, output.Validation("source_scan", fmt.Sprintf("failed to scan the source tree for secret files: %v", err))
	}
	return denied, nil
}

// ignoredArchivePath mirrors the directories the archive package skips so the
// denylist scan does not flag files inside vendored/build directories.
func ignoredArchivePath(rel string) bool {
	base := filepath.Base(rel)
	switch base {
	case ".git", "node_modules", ".cache", ".next", "dist", "build", "__pycache__", ".DS_Store":
		return true
	}
	return false
}

// downloadAndVerifyDigest fetches a URL and verifies its sha256 digest matches
// the expected `sha256:<hex>` value before returning the bytes. This is the
// integrity boundary for template downloads.
func downloadAndVerifyDigest(ctx context.Context, runtime *Runtime, downloadURL, expectedDigest string) ([]byte, error) {
	if expectedDigest == "" {
		return nil, output.Validation("template_digest_missing", "the template catalog did not return a digest to verify against")
	}
	// The catalog URL is server-controlled, but the transport policy still
	// applies: HTTPS (or loopback HTTP) only, and no redirects — consistent
	// with the artifact download client (#67 review P2). The digest check is
	// the integrity boundary; this prevents the probe surface.
	parsed, err := api.ValidateDownloadURL(downloadURL)
	if err != nil {
		return nil, output.Validation("template_download_url", "the template catalog returned an invalid download URL")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, output.Internal("template_download_request", "failed to create the template download request", err)
	}
	if ua := runtime.client().UserAgent; ua != "" {
		request.Header.Set("User-Agent", ua)
	}
	response, err := api.WithoutRedirects(runtime.deps.HTTPClient).Do(request)
	if err != nil {
		return nil, output.Network("template_download", "failed to download the template archive", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, output.Network("template_download", fmt.Sprintf("template download failed with HTTP %d: %s", response.StatusCode, bytes.TrimSpace(data)), nil)
	}
	const maxTemplateBytes = 100 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, maxTemplateBytes+1))
	if err != nil {
		return nil, output.Network("template_download", "failed to read the template archive", err)
	}
	if int64(len(body)) > maxTemplateBytes {
		return nil, output.Policy("template_too_large", "template archive exceeds the 100 MiB limit")
	}
	sum := sha256.Sum256(body)
	actual := "sha256:" + hex.EncodeToString(sum[:])
	if actual != expectedDigest {
		return nil, output.Validation("template_digest_mismatch", fmt.Sprintf("template digest mismatch: expected %s, got %s", expectedDigest, actual)).
			WithHint("the template catalog may be stale or the download was tampered with; rerun 'viceme app init'")
	}
	return body, nil
}

// extractZipArchive extracts a zip archive into dir. Path traversal entries
// (absolute paths or ../) are rejected to avoid writing outside the target, and
// bounded extraction limits (file count, per-file size, total size) defend
// against zip bombs.
func extractZipArchive(archiveBytes []byte, dir string) error {
	reader, err := zip.NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)))
	if err != nil {
		return fmt.Errorf("open zip archive: %w", err)
	}
	const (
		maxFileCount = 10_000
		maxFileSize  = int64(50) << 20  // 50 MiB per file
		maxTotalSize = int64(250) << 20 // 250 MiB across all files
	)
	if len(reader.File) > maxFileCount {
		return fmt.Errorf("template archive contains too many entries: %d (limit %d)", len(reader.File), maxFileCount)
	}
	var totalWritten int64
	for _, file := range reader.File {
		written, err := extractZipEntry(file, dir, maxFileSize, maxTotalSize-totalWritten)
		if err != nil {
			return err
		}
		totalWritten += written
		if totalWritten > maxTotalSize {
			return fmt.Errorf("template archive exceeds the %d MiB total extraction limit", maxTotalSize>>20)
		}
	}
	return nil
}

func extractZipEntry(file *zip.File, dir string, maxFileBytes, maxRemaining int64) (int64, error) {
	// Validate BEFORE any filesystem side effect — directory entries included.
	// A hostile archive can name entries "..", use an absolute path, or smuggle
	// a Windows drive root; and a pre-existing symlink inside the output
	// directory can redirect writes outside it (P0/P1 fixes).
	destination, err := safeExtractPath(dir, file.Name)
	if err != nil {
		return 0, err
	}
	if file.FileInfo().IsDir() {
		return 0, os.MkdirAll(destination, file.Mode())
	}
	if file.UncompressedSize64 > uint64(maxFileBytes) {
		return 0, fmt.Errorf("archive entry %s exceeds the %d MiB per-file limit", file.Name, maxFileBytes>>20)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return 0, err
	}
	source, err := file.Open()
	if err != nil {
		return 0, fmt.Errorf("open archive entry %s: %w", file.Name, err)
	}
	defer source.Close()
	target, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
	if err != nil {
		return 0, fmt.Errorf("create %s: %w", file.Name, err)
	}
	defer target.Close()
	// Bound the copy at the smaller of the per-file or remaining-total budget so
	// a misreported UncompressedSize64 cannot exhaust the disk.
	limit := maxFileBytes
	if maxRemaining > 0 && maxRemaining < limit {
		limit = maxRemaining
	}
	written, err := io.Copy(target, io.LimitReader(source, limit+1))
	if err != nil {
		return 0, fmt.Errorf("write %s: %w", file.Name, err)
	}
	if written > limit {
		return 0, fmt.Errorf("archive entry %s exceeds the extraction limit", file.Name)
	}
	return written, nil
}

// safeExtractPath resolves an archive entry name inside dir and rejects any
// entry that would escape it: absolute paths, ".." segments, Windows drive
// roots, or traversal through a pre-existing symbolic link. The check runs
// before any directory or file is created (P0 fix: directory entries were
// previously extracted unchecked).
func safeExtractPath(dir, name string) (string, error) {
	cleaned := filepath.Clean(name)
	if cleaned == "." || cleaned == "" {
		return "", fmt.Errorf("archive entry has an empty path")
	}
	if filepath.IsAbs(cleaned) ||
		cleaned == ".." ||
		strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) ||
		filepath.VolumeName(cleaned) != "" ||
		strings.Contains(name, "\\") {
		return "", fmt.Errorf("archive entry escapes the output directory: %s", name)
	}
	baseAbs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	destination := filepath.Join(baseAbs, cleaned)
	cleanedAbs, err := filepath.Abs(destination)
	if err != nil {
		return "", err
	}
	sep := string(filepath.Separator)
	if cleanedAbs != baseAbs &&
		!strings.HasPrefix(cleanedAbs, baseAbs+sep) {
		return "", fmt.Errorf("archive entry escapes the output directory: %s", name)
	}
	// Symlink guard: no existing component of the target path may be a
	// symbolic link, or a write would follow it outside dir (P1 fix).
	current := baseAbs
	for _, part := range strings.Split(cleaned, "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			break // remaining components do not exist yet
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("archive entry traverses a symbolic link: %s", name)
		}
	}
	return cleanedAbs, nil
}

// injectTemplateRuntimeReleaseID replaces the template placeholder
// REPLACE_WITH_RUNTIME_RELEASE_ID with the actual Release id in the project's
// JS files (the starter's app.js reads it to create runs). Conservative:
// only regular .js files under the project directory are touched, and only
// the exact placeholder token is replaced.
func injectTemplateRuntimeReleaseID(projectDir, runtimeReleaseID string) error {
	placeholder := []byte("REPLACE_WITH_RUNTIME_RELEASE_ID")
	replacement := []byte(runtimeReleaseID)
	err := filepath.WalkDir(projectDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".js") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Contains(data, placeholder) {
			return nil
		}
		return os.WriteFile(path, bytes.ReplaceAll(data, placeholder, replacement), 0o644)
	})
	if err != nil {
		return err
	}
	return nil
}

// directoryEmptyOrRetryMarker reports whether init may extract into the
// directory: it must be empty, or it must hold the interrupted-init marker
// (.viceme/managed-release.json with a persisted clientRequestId). During a
// retry the directory may additionally contain template files left by the
// interrupted extraction — re-extracting the digest-verified archive over them
// is safe (extraction overwrites, never deletes).
func directoryEmptyOrRetryMarker(directory string, retrying bool) bool {
	if !retrying {
		entries, err := os.ReadDir(directory)
		if err != nil {
			return false
		}
		return len(entries) == 0
	}
	_, err := os.Stat(managedrelease.Path(directory))
	return err == nil
}

func resolveProjectDirectoryOrCreate(directory string) (string, error) {
	abs, err := filepath.Abs(directory)
	if err != nil {
		return "", output.Validation("project_directory", "could not resolve the output directory")
	}
	info, err := os.Stat(abs)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", output.Validation("project_directory", fmt.Sprintf("could not stat the output directory: %v", err))
		}
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return "", output.Validation("project_directory", fmt.Sprintf("could not create the output directory: %v", err))
		}
	} else if !info.IsDir() {
		return "", output.Validation("project_directory", "output path exists and is not a directory")
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return abs, nil
	}
	return resolved, nil
}
