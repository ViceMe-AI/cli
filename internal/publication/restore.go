package publication

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

// 作者侧渠道还原：固定分支上的 Skill 目录已经被试用门禁包装（SKILL.md 是
// 入口、原文在 .viceme/trial-body.md、生成文件记录在 .viceme/runtime.json）。
// 再次发布必须在临时构建区还原规范原始包，不能把门禁入口当业务正文上传，
// 也不能把生成文件混进原始包。还原只发生在内存中的 entries，作者工作区
// 保持不动。

type channelRuntimeManifest struct {
	SchemaVersion int               `json:"schemaVersion"`
	ProductID     string            `json:"productId"`
	ReleaseID     string            `json:"releaseId"`
	Market        string            `json:"market"`
	Kind          string            `json:"kind"`
	Files         map[string]string `json:"files"`
}

func gatedByMarker(skill []byte) bool {
	if len(skill) == 0 {
		return false
	}
	content := strings.ReplaceAll(string(skill), "\r\n", "\n")
	if !strings.HasPrefix(content, "---\n") {
		return false
	}
	end := strings.Index(content[4:]+"\n", "\n---\n")
	if end < 0 {
		return false
	}
	body := content[4+end+len("\n---\n"):]
	return strings.HasPrefix(body, skillcontent.TrialGateMarker+" product=")
}

// RestoreGatedDirectory detects a channel-delivered Skill directory and
// restores the canonical author package in memory. When SKILL.md carries the
// trial gate, the authored document comes back from .viceme/trial-body.md;
// when SKILL.md is still the authored entry (e.g. delivery stopped on a local
// conflict), it is packaged as-is. In both cases every runtime-generated file
// recorded in .viceme/runtime.json drops out of the package and unknown author
// files stay. The working directory itself is never modified.
func RestoreGatedDirectory(root string, entries []sourceEntry) ([]sourceEntry, error) {
	var gated bool
	var mode fs.FileMode
	for _, entry := range entries {
		if entry.name == "SKILL.md" {
			gated = gatedByMarker(entry.data)
			mode = entry.mode
			break
		}
	}
	bodyPath := filepath.Join(root, filepath.FromSlash(skillcontent.TrialBodyPath))
	bodyRaw, bodyErr := os.ReadFile(bodyPath)
	runtimeRaw, runtimeErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(skillcontent.RuntimeManifestPath)))
	if gated && (bodyErr != nil || runtimeErr != nil) {
		return nil, output.Validation("SKILL_CHANNEL_RESTORE_INCOMPLETE",
			"this Skill directory carries a trial channel gate, but .viceme/trial-body.md or .viceme/runtime.json is missing").
			WithHint("re-run 'viceme publication deliver <publication-id> --skill-dir <dir>' to regenerate the channel files, or remove the leftover gate files; do not publish the gated entry as authored content")
	}
	var runtime channelRuntimeManifest
	if runtimeErr == nil {
		if err := json.Unmarshal(runtimeRaw, &runtime); err != nil || len(runtime.Files) == 0 {
			return nil, output.Validation("SKILL_CHANNEL_RESTORE_INCOMPLETE",
				".viceme/runtime.json does not carry a usable generated-file inventory").
				WithCause(err)
		}
	}
	if gated {
		if !mode.IsRegular() {
			mode = 0o644
		}
		if len(bodyRaw) == 0 {
			return nil, output.Validation("SKILL_CHANNEL_RESTORE_INCOMPLETE",
				".viceme/trial-body.md is empty, so the authored SKILL.md cannot be restored")
		}
	} else {
		for _, entry := range entries {
			if entry.name == "SKILL.md" {
				bodyRaw = entry.data
				break
			}
		}
	}
	restored := make([]sourceEntry, 0, len(entries))
	for _, entry := range entries {
		if _, generated := runtime.Files[entry.name]; generated {
			continue
		}
		if entry.name == "SKILL.md" {
			entry = sourceEntry{name: "SKILL.md", mode: mode.Perm(), data: bodyRaw}
		}
		restored = append(restored, entry)
	}
	if len(restored) == 0 {
		return nil, output.Validation("SKILL_PACKAGE_EMPTY", "restored Skill package contains no files")
	}
	return restored, nil
}

// RejectGatedArchive refuses a channel ZIP as publication source. The ZIP
// reader strips .viceme/, so publishing it would upload the gate entry while
// losing the authored body entirely; the unpacked directory is the supported
// update path.
func RejectGatedArchive(entries []sourceEntry) error {
	for _, entry := range entries {
		if entry.name == "SKILL.md" && gatedByMarker(entry.data) {
			return output.Validation("SKILL_CHANNEL_ARCHIVE_NOT_AUTHORABLE",
				"this ZIP is a trial channel package; its SKILL.md is a generated gate entry").
				WithHint("publish from the unpacked channel directory instead: 'viceme skill publish --path <skill-dir>' restores the authored body automatically")
		}
	}
	return nil
}
