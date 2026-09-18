package command

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
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
	// Lock, record, and file operations share one physical directory identity:
	// parent-path aliases (a symbolic link above the directory, e.g. /tmp vs
	// /private/tmp) must not fork the delivery lock or the baseline record.
	physicalSkillDir, err := filepath.EvalSymlinks(absSkillDir)
	if err != nil {
		return output.Validation("SKILL_CHANNEL_DIR_INVALID", "could not resolve the Skill directory").WithCause(err)
	}
	absSkillDir = physicalSkillDir

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
		if !strings.EqualFold(binding.Market, string(runtime.region)) {
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
		if existing.ProductID != product.ID || !strings.EqualFold(existing.Market, string(runtime.region)) || (existing.Kind != "" && existing.Kind != "trial") {
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
	// The channel package carries the credential-free install identity from the
	// moment it is unpacked, exactly like the purchase entry export: consumers
	// can run the gate's use/ready commands in the unpacked directory without
	// a prior official install.
	files[skillcontent.InstallManifestPath] = downloadableSkillFile{
		Data: channelInstallManifest(product.ID, product.ReleaseID), Mode: 0o644,
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

	// One directory, one delivery at a time: the lock covers reading the
	// baseline record, every file modification, the ZIP, and the final record
	// persistence, for deliveries of any publication into this directory.
	store := channeldelivery.Store{Directory: filepath.Join(runtime.configBase, "channel-deliveries"),
		EndpointOrigin: runtime.apiBaseURL, Now: runtime.deps.Now, ReportDegraded: runtime.deps.ReportDegradedWrite}
	unlock, err := store.Lock(absSkillDir)
	if err != nil {
		return err
	}
	defer func() { _ = unlock() }()

	record, hasRecord, err := store.Load(product.ID, absSkillDir)
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
	// The channel directory, the channel ZIP, and a ZIP the author packs from
	// the directory must be the same delivery: unpublished local changes to
	// authored files stop the delivery instead of silently forking it.
	apply.Conflicts = append(apply.Conflicts, authorFileConflicts(absSkillDir, files, channelFiles)...)
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
	// Removed managed paths stay recorded until a later delivery re-applies
	// content at the same path, so a retry after a lost response still tells
	// the agent which deletions are waiting for their Git commit.
	removed := make(map[string]string, len(record.RemovedFiles))
	for name, release := range record.RemovedFiles {
		removed[name] = release
	}
	for _, name := range apply.Written {
		delete(removed, name)
	}
	for _, name := range apply.Unchanged {
		delete(removed, name)
	}
	for _, name := range apply.Removed {
		removed[name] = product.ReleaseID
	}
	next := channeldelivery.Record{
		APIVersion: channeldelivery.RecordAPIVersion, EndpointOrigin: runtime.apiBaseURL,
		Market: string(runtime.region), PublicationID: publicationID, ListingID: published.ListingID,
		ProductID: product.ID, ReleaseID: product.ReleaseID, ProductSlug: product.Slug, Branch: branch,
		SkillDir: absSkillDir, AppliedFiles: applied, RemovedFiles: removed,
		ZipPath: absZip, ZipDigest: zipDigest,
		Generator: buildinfo.Version, CreatedAt: record.CreatedAt,
	}
	if err := store.Save(next); err != nil {
		return err
	}

	// The commit scope is the managed path set of the directory — not only
	// this run's delta — so a re-run after a lost response still reports every
	// channel path, including deletions, that may be waiting for its Git commit.
	scope := make([]string, 0, len(applied)+len(removed))
	for name := range applied {
		scope = append(scope, name)
	}
	for name := range removed {
		scope = append(scope, name)
	}
	sort.Strings(scope)

	details := map[string]any{
		"publicationId": publicationID, "listingId": published.ListingID,
		"productId": product.ID, "releaseId": product.ReleaseID, "productSlug": product.Slug,
		"market": string(runtime.region), "status": "DELIVERED",
		"skillDir": absSkillDir, "branch": branch,
		"trialBodyEditPosition": skillcontent.TrialBodyPath,
		"zip":                   map[string]any{"path": absZip, "digest": zipDigest, "sizeBytes": len(zipBytes)},
		"written":               apply.Written, "unchanged": apply.Unchanged, "removed": apply.Removed,
		"commitScope": scope, "commitScopeNote": "managed channel paths for the fixed branch; commit the ones your Git status reports as changed",
	}
	if len(apply.Conflicts) > 0 {
		details["conflicts"] = apply.Conflicts
		details["warnings"] = warnings
		return output.Validation("SKILL_CHANNEL_LOCAL_CONFLICT",
			"local edits to generated channel files or unpublished author files were preserved; nothing of the author's work was overwritten").
			WithDetails(details).
			WithHint("publish the author's edits first ('viceme skill publish --path <skill-dir>' restores them), then re-run this command; or discard the local edits and re-run")
	}
	details["warnings"] = warnings
	return runtime.business(details)
}

// channelInstallManifest mirrors trial_install_manifest in trial_runtime.py
// byte for byte; the interop test pins the parity. It carries no credentials:
// consumer install identity (installId/secret) is created per machine on the
// first gate use.
func channelInstallManifest(productID, releaseID string) []byte {
	payload := struct {
		SchemaVersion         int    `json:"schema_version"`
		CLIVersion            string `json:"cli_version"`
		SkillVersion          string `json:"skill_version"`
		MinimumCLIVersion     string `json:"minimum_cli_version"`
		CLICompatibility      string `json:"cli_compatibility"`
		FullSkillBundleDigest string `json:"full_skill_bundle_digest"`
		EmbeddedContentDigest string `json:"embedded_content_digest"`
		ProductID             string `json:"product_id"`
		ReleaseID             string `json:"release_id"`
	}{
		SchemaVersion: 1, CLIVersion: "viceme-trial-script/1", SkillVersion: "1",
		CLICompatibility: "script", ProductID: productID, ReleaseID: releaseID,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return []byte("{}\n")
	}
	return append(data, '\n')
}

// authorFileConflicts compares the directory's authored surface against the
// exported release: every authored file of the package must exist locally with
// the published bytes, and every non-ignored local file must belong to the
// package. Anything else is unpublished local work that would fork the
// channel directory from the delivered ZIP.
func authorFileConflicts(root string, packageFiles map[string]downloadableSkillFile, channelFiles map[string]channeldelivery.File) []channeldelivery.Conflict {
	type expectedFile struct {
		digest string
		mode   os.FileMode
	}
	expectedFiles := make(map[string]expectedFile, len(packageFiles))
	for name, file := range packageFiles {
		if _, generated := channelFiles[name]; generated {
			continue
		}
		expectedFiles[name] = expectedFile{digest: fmt.Sprintf("%x", sha256.Sum256(file.Data)), mode: file.Mode}
	}
	seen := make(map[string]bool, len(expectedFiles))
	var conflicts []channeldelivery.Conflict
	_ = filepath.WalkDir(root, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		relative, relErr := filepath.Rel(root, current)
		if relErr != nil {
			return nil
		}
		relative = filepath.ToSlash(relative)
		if relative == ".viceme" {
			return filepath.SkipDir
		}
		if relative != "." && entry.IsDir() && publication.WorkspaceEntryIgnored(root, relative, true) {
			// The packager prunes ignored directories wholesale; the surface
			// comparison must not report their children as foreign files.
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		if strings.HasPrefix(relative, ".viceme/") || publication.WorkspaceEntryIgnored(root, relative, false) {
			return nil
		}
		if _, managed := channelFiles[relative]; managed {
			// Generated channel files were just applied from the release; the
			// authored surface check only governs author content.
			return nil
		}
		if file, ok := expectedFiles[relative]; ok {
			seen[relative] = true
			data, readErr := os.ReadFile(current)
			if readErr != nil || fmt.Sprintf("%x", sha256.Sum256(data)) != file.digest {
				conflicts = append(conflicts, channeldelivery.Conflict{Path: relative,
					Reason: "local authored file differs from the published release; publish it first"})
				return nil
			}
			// The packaging rules only assign meaning to the executable bit;
			// a same-bytes file without it changes how the channel directory
			// behaves compared to the delivered ZIP.
			if info, statErr := entry.Info(); statErr == nil &&
				(info.Mode().Perm()&0o111 != 0) != (file.mode.Perm()&0o111 != 0) {
				conflicts = append(conflicts, channeldelivery.Conflict{Path: relative,
					Reason: "local authored file carries a different executable bit than the published release; publish it first"})
			}
			return nil
		}
		conflicts = append(conflicts, channeldelivery.Conflict{Path: relative,
			Reason: "local file is not part of the published release; publish it or remove it"})
		return nil
	})
	names := make([]string, 0, len(expectedFiles))
	for name := range expectedFiles {
		if !seen[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		conflicts = append(conflicts, channeldelivery.Conflict{Path: name,
			Reason: "published authored file is missing from the local directory"})
	}
	return conflicts
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
