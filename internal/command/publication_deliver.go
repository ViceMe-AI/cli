package command

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/channeldelivery"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/publication"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	"github.com/spf13/cobra"
)

// 作者渠道交付：发布成功后，以服务端保存的准确 Release 原始字节为源，用官方
// 门禁生成器在本地重建试用渠道内容，原位应用到原 Skill 路径，并产出同一份
// 构建结果的渠道 ZIP。固定分支与 PR 的 Git 编排由上层（sell-a-skill 与本地
// Git/GitHub 工具）完成；本命令只负责确定性的本地文件工作与交付恢复记录。
func newPublicationDeliverCommand(runtime *Runtime) *cobra.Command {
	var skillDir string
	var zipPath string
	command := &cobra.Command{
		Use:   "deliver <publication-id>",
		Short: "Build and apply the trial channel package of one published Skill version onto its original directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return deliverSkillChannel(command, runtime, args[0], skillDir, zipPath)
		},
	}
	command.Flags().StringVar(&skillDir, "skill-dir", "", "author Skill directory that received the original publication; channel files are applied in place")
	_ = command.MarkFlagRequired("skill-dir")
	command.Flags().StringVar(&zipPath, "zip", "", "channel ZIP output path; defaults to <skill-dir>-viceme-trial-channel.zip next to the directory")
	return command
}

func deliverSkillChannel(command *cobra.Command, runtime *Runtime, publicationID, skillDir, zipPath string) error {
	ctx := command.Context()
	if err := runtime.requireSkillPublicationAuthentication(ctx); err != nil {
		return err
	}
	published, err := runtime.client().GetSkillPublication(ctx, publicationID)
	if err != nil {
		return err
	}
	if published.Status != "PUBLISHED" || published.Product == nil || published.Product.ReleaseID == "" {
		return output.Validation("SKILL_CHANNEL_PUBLICATION_NOT_PUBLISHED",
			"channel delivery needs a published Skill version").
			WithDetails(map[string]any{"publicationId": publicationID, "status": published.Status}).
			WithHint("finish 'viceme publication confirm' and 'publication publish' first; delivery never republishes or rolls a publication back to draft")
	}
	product := *published.Product

	absSkillDir, err := filepath.Abs(skillDir)
	if err != nil {
		return output.Validation("SKILL_CHANNEL_DIR_INVALID", "could not resolve the Skill directory").WithCause(err)
	}
	info, err := os.Lstat(absSkillDir)
	if err != nil {
		return output.Validation("SKILL_CHANNEL_DIR_NOT_FOUND", "the Skill directory does not exist").WithCause(err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return output.Validation("SKILL_CHANNEL_DIR_INVALID", "--skill-dir must be a real directory, not a symlink")
	}

	var warnings []string
	binding, hasBinding := readChannelBinding(absSkillDir)
	if hasBinding {
		if binding.ListingID != published.ListingID {
			return output.Validation("SKILL_CHANNEL_BINDING_MISMATCH",
				"the local .viceme/skill.json binding points at a different Skill listing").
				WithDetails(map[string]any{"boundListingId": binding.ListingID, "publicationListingId": published.ListingID}).
				WithHint("deliver into the directory that produced this publication; a name or digest match does not prove listing identity")
		}
		if binding.EndpointOrigin != runtime.apiBaseURL {
			return output.Validation("SKILL_CHANNEL_BINDING_MISMATCH",
				"the local .viceme/skill.json binding belongs to a different API endpoint").
				WithDetails(map[string]any{"boundEndpoint": binding.EndpointOrigin, "endpoint": runtime.apiBaseURL})
		}
		if binding.Market != string(runtime.region) {
			return output.Validation("SKILL_CHANNEL_BINDING_MISMATCH",
				"the local .viceme/skill.json binding belongs to a different market").
				WithDetails(map[string]any{"boundMarket": binding.Market, "market": string(runtime.region)})
		}
	} else {
		warnings = append(warnings, "the Skill directory has no .viceme/skill.json binding; listing identity was verified only through the publication")
	}

	if existing, ok, err := readChannelRuntimeIdentity(absSkillDir); err != nil {
		return err
	} else if ok {
		if existing.ProductID != product.ID || existing.Market != string(runtime.region) || (existing.Kind != "" && existing.Kind != "trial") {
			return output.Validation("SKILL_CHANNEL_DIR_OWNED_BY_OTHER_PRODUCT",
				"the directory already carries channel runtime files for a different product, market, or kind").
				WithDetails(map[string]any{"directoryProductId": existing.ProductID, "productId": product.ID,
					"directoryMarket": existing.Market, "market": string(runtime.region), "directoryKind": existing.Kind}).
				WithHint("pick the directory this publication was delivered to; never reset an unknown channel directory in place")
		}
	}

	export, err := runtime.client().GetSkillPublicationPackage(ctx, publicationID)
	if err != nil {
		return err
	}
	if export.ReleaseID != product.ReleaseID {
		return output.Validation("SKILL_CHANNEL_RELEASE_MISMATCH",
			"the exported package release differs from the publication release").
			WithDetails(map[string]any{"exportedReleaseId": export.ReleaseID, "releaseId": product.ReleaseID})
	}
	raw, err := runtime.client().DownloadArtifact(ctx, export.URL)
	if err != nil {
		return err
	}
	if digest := fmt.Sprintf("%x", sha256.Sum256(raw)); digest != strings.TrimPrefix(export.ArtifactDigest, "sha256:") {
		return output.Validation("SKILL_CHANNEL_ARTIFACT_MISMATCH",
			"downloaded original package bytes do not match the published artifact digest").
			WithDetails(map[string]any{"expected": export.ArtifactDigest, "actual": digest})
	}

	files, err := extractDownloadableSkill(raw)
	if err != nil {
		return err
	}
	if err := injectSkillTrialGate(files, product.ID, string(runtime.region)); err != nil {
		return err
	}
	if err := addSkillRuntime(runtime, files, product.ID, product.ReleaseID, "trial"); err != nil {
		return err
	}

	channelFiles := make(map[string]channeldelivery.File, len(files))
	for name, file := range files {
		if isChannelGeneratedPath(name) {
			channelFiles[name] = channeldelivery.File{Data: file.Data, Mode: file.Mode}
		}
	}

	if zipPath == "" {
		zipPath = filepath.Join(filepath.Dir(absSkillDir), filepath.Base(absSkillDir)+"-viceme-trial-channel.zip")
	}
	absZip, err := filepath.Abs(zipPath)
	if err != nil {
		return output.Validation("SKILL_CHANNEL_ZIP_PATH_INVALID", "could not resolve the channel ZIP path").WithCause(err)
	}
	if absZip == absSkillDir || strings.HasPrefix(absZip, absSkillDir+string(os.PathSeparator)) {
		return output.Validation("SKILL_CHANNEL_ZIP_PATH_INVALID",
			"the channel ZIP must live outside the Skill directory; a ZIP inside it would be packaged on the next publish")
	}
	zipBytes, err := channeldelivery.WriteZip(mapStringFiles(files))
	if err != nil {
		return err
	}
	zipDigest := fmt.Sprintf("%x", sha256.Sum256(zipBytes))

	store := channeldelivery.Store{Directory: filepath.Join(runtime.configBase, "channel-deliveries"),
		EndpointOrigin: runtime.apiBaseURL, Now: runtime.deps.Now, ReportDegraded: runtime.deps.ReportDegradedWrite}
	record, hasRecord, err := store.Load(publicationID, absSkillDir)
	if err != nil {
		return err
	}
	if hasRecord && record.ReleaseID != "" && record.ReleaseID != product.ReleaseID {
		warnings = append(warnings, fmt.Sprintf("this directory previously delivered release %s; the channel files now move to release %s", record.ReleaseID, product.ReleaseID))
	}

	owned := map[string][]string{}
	for name, digest := range record.AppliedFiles {
		owned[name] = append(owned[name], digest)
	}
	// The ungated SKILL.md identical to the published trial body may be safely
	// replaced by the gate entry; a gated SKILL.md whose on-disk trial body
	// already equals the published one is only a gate refresh.
	if trialBody, ok := files[skillcontent.TrialBodyPath]; ok {
		bodyDigest := fmt.Sprintf("%x", sha256.Sum256(trialBody.Data))
		owned["SKILL.md"] = append(owned["SKILL.md"], bodyDigest)
		if diskBody, err := os.ReadFile(filepath.Join(absSkillDir, filepath.FromSlash(skillcontent.TrialBodyPath))); err == nil &&
			fmt.Sprintf("%x", sha256.Sum256(diskBody)) == bodyDigest {
			if diskSkill, err := os.ReadFile(filepath.Join(absSkillDir, "SKILL.md")); err == nil &&
				strings.Contains(string(diskSkill), skillcontent.TrialGateMarker+" product="+product.ID) {
				owned["SKILL.md"] = append(owned["SKILL.md"], fmt.Sprintf("%x", sha256.Sum256(diskSkill)))
			}
		}
	}

	apply, err := channeldelivery.Apply(absSkillDir, channelFiles, owned)
	if err != nil {
		return err
	}
	if err := channeldelivery.SaveZip(absZip, zipBytes); err != nil {
		return err
	}

	applied := make(map[string]string, len(apply.Written)+len(apply.Unchanged)+len(apply.Conflicts))
	newDigests := mapStringDigests(channelFiles)
	for _, name := range append(append([]string{}, apply.Written...), apply.Unchanged...) {
		applied[name] = newDigests[name]
	}
	// Conflicted paths keep the last digest this process generated, so a
	// later run can still tell "pristine generated content" (safe to
	// overwrite) from "content we never wrote" (always a conflict).
	for _, conflict := range apply.Conflicts {
		if digest, ok := record.AppliedFiles[conflict.Path]; ok {
			applied[conflict.Path] = digest
		}
	}
	branch := channeldelivery.BranchName(product.Slug)
	if branch == "" {
		branch = "viceme-skill-" + strings.ReplaceAll(product.ID, "-", "")[:12]
	}
	next := channeldelivery.Record{
		APIVersion: channeldelivery.RecordAPIVersion, EndpointOrigin: runtime.apiBaseURL,
		Market: string(runtime.region), PublicationID: publicationID, ListingID: published.ListingID,
		ProductID: product.ID, ReleaseID: product.ReleaseID, ProductSlug: product.Slug, Branch: branch,
		SkillDir: absSkillDir, AppliedFiles: applied, ZipPath: absZip, ZipDigest: zipDigest,
		Generator: buildinfo.Version, CreatedAt: record.CreatedAt,
	}
	if err := store.Save(next); err != nil {
		return err
	}

	details := map[string]any{
		"publicationId": publicationID, "listingId": published.ListingID,
		"productId": product.ID, "releaseId": product.ReleaseID, "productSlug": product.Slug,
		"market": string(runtime.region), "status": "DELIVERED",
		"skillDir": absSkillDir, "branch": branch,
		"trialBodyEditPosition": skillcontent.TrialBodyPath,
		"zip": map[string]any{"path": absZip, "digest": zipDigest, "sizeBytes": len(zipBytes)},
		"written": apply.Written, "unchanged": apply.Unchanged, "removed": apply.Removed,
		"commitScope": append(append([]string{}, apply.Written...), apply.Removed...),
	}
	if len(apply.Conflicts) > 0 {
		details["conflicts"] = apply.Conflicts
		details["warnings"] = warnings
		return output.Validation("SKILL_CHANNEL_LOCAL_CONFLICT",
			"local edits to generated channel files were preserved; nothing of the author's work was overwritten").
			WithDetails(details).
			WithHint("publish the author's edits first ('viceme skill publish --path <skill-dir>' restores them), then re-run this command; or discard the local edits and re-run")
	}
	details["warnings"] = warnings
	return runtime.business(details)
}

func isChannelGeneratedPath(name string) bool {
	return strings.HasPrefix(name, ".viceme/") || name == "SKILL.md" || name == skillcontent.TrialRuntimePath
}

func mapStringFiles(files map[string]downloadableSkillFile) map[string]channeldelivery.File {
	converted := make(map[string]channeldelivery.File, len(files))
	for name, file := range files {
		converted[name] = channeldelivery.File{Data: file.Data, Mode: file.Mode}
	}
	return converted
}

func mapStringDigests(files map[string]channeldelivery.File) map[string]string {
	digests := make(map[string]string, len(files))
	for name, file := range files {
		digests[name] = fmt.Sprintf("%x", sha256.Sum256(file.Data))
	}
	return digests
}

func readChannelBinding(directory string) (publication.SkillBinding, bool) {
	raw, err := os.ReadFile(filepath.Join(directory, ".viceme", "skill.json"))
	if err != nil {
		return publication.SkillBinding{}, false
	}
	binding := publication.SkillBinding{}
	if err := json.Unmarshal(raw, &binding); err != nil || binding.APIVersion != publication.BindingAPIVersion {
		return publication.SkillBinding{}, false
	}
	return binding, true
}

type channelDirectoryIdentity struct {
	ProductID string
	Market    string
	Kind      string
}

func readChannelRuntimeIdentity(directory string) (channelDirectoryIdentity, bool, error) {
	raw, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(skillcontent.RuntimeManifestPath)))
	if os.IsNotExist(err) {
		return channelDirectoryIdentity{}, false, nil
	}
	if err != nil {
		return channelDirectoryIdentity{}, false, output.Internal("SKILL_CHANNEL_RUNTIME_READ_FAILED", "could not read the directory runtime manifest", err)
	}
	identity := channelDirectoryIdentity{}
	if err := json.Unmarshal(raw, &identity); err != nil || identity.ProductID == "" {
		return channelDirectoryIdentity{}, true, output.Validation("SKILL_CHANNEL_RESTORE_INCOMPLETE",
			"the directory has a .viceme/runtime.json without a usable product identity").
			WithHint("re-run the delivery for the product this directory belongs to, or remove the leftover channel files")
	}
	return identity, true, nil
}
