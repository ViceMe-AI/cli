package main

// 官方 Skills 云端托管物生成器:把内嵌的官方技能打成确定性 zip 并输出
// 机器可读清单。Release Workflow 把产物发布到 start 桶:
//
//	skills/<name>.zip、skills/manifest.json                  稳定(随最高稳定版更新)
//	cli/releases/v<version>/skills/<name>.zip、.../manifest.json   不可变版本化副本
//
// 非 CLI 环境的路由脚本与口令从这些 URL 直接安装官方技能;digest 与
// release-manifest.json 同源(同一内嵌 FS 的 Bundle.Digests),CLI 内嵌
// 安装与云端下载安装的校验口径完全一致。
//
// zip 必须字节确定:固定时间戳、固定权限、排序遍历、不写目录条目与
// extra 字段——发布侧对已存在对象做字节一致校验,任何非确定性都会把
// 正常重发布误判为篡改。

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	cliembed "github.com/ViceMe-AI/cli"
	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

// zipEpoch 固定的条目修改时间,保证同一内容重复打包字节一致。
var zipEpoch = time.Unix(0, 0).UTC()

type hostedSkill struct {
	SkillVersion          string   `json:"skill_version"`
	MinimumCLIVersion     string   `json:"minimum_cli_version"`
	CLICompatibility      string   `json:"cli_compatibility"`
	Zip                   string   `json:"zip"`
	ZipSHA256             string   `json:"zip_sha256"`
	FullBundleDigest      string   `json:"full_skill_bundle_digest"`
	EmbeddedContentDigest string   `json:"embedded_content_digest"`
	Files                 []string `json:"files"`
}

type hostingManifest struct {
	SchemaVersion int                    `json:"schema_version"`
	CLIVersion    string                 `json:"cli_version"`
	Skills        map[string]hostedSkill `json:"skills"`
}

func main() {
	version := flag.String("version", buildinfo.ReleaseVersion, "release version recorded in the hosting manifest")
	outputDir := flag.String("output", "dist/skills", "output directory for zips and the manifest")
	flag.Parse()

	files, manifest, err := buildHostingArchive(cliembed.EmbeddedSkills())
	if err != nil {
		fatal(err)
	}
	manifest.CLIVersion = *version

	// Shared host presentation assets are not Skill directories or packages.
	if err := addWidgetAssets(files, cliembed.EmbeddedWidgets()); err != nil {
		fatal(err)
	}

	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		fatal(err)
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		target := filepath.Join(*outputDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			fatal(err)
		}
		if err := os.WriteFile(target, files[name], 0o644); err != nil {
			fatal(err)
		}
	}
	fmt.Printf("skills-archive: wrote %d objects (%d skills, zips + flat files + manifest) for CLI %s\n",
		len(files), len(manifest.Skills), *version)
}

func addWidgetAssets(files map[string][]byte, widgets fs.FS) error {
	entries, err := fs.ReadDir(widgets, ".")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		data, err := fs.ReadFile(widgets, entry.Name())
		if err != nil {
			return err
		}
		files["_widgets/"+entry.Name()] = data
		digest := sha256.Sum256(data)
		files["_widgets/sha256-"+hex.EncodeToString(digest[:])+"/"+entry.Name()] = data
	}
	return nil
}

// buildHostingArchive 从与 CLI 内嵌一致的 FS 产出托管物:每个官方技能一个
// 确定性 zip(zip 根目录为技能目录名),外加 manifest.json。manifest 的
// digest 字段直接取 Bundle.Digests,与 release-manifest.json 同源。
func buildHostingArchive(skillsFS fs.FS) (map[string][]byte, hostingManifest, error) {
	bundle := skillcontent.New(skillsFS)
	names, err := officialSkillDirs(skillsFS)
	if err != nil {
		return nil, hostingManifest{}, err
	}
	skills := make(map[string]hostedSkill, len(names))
	files := make(map[string][]byte, len(names)+1)
	for _, name := range names {
		archive, listing, err := archiveSkill(skillsFS, name)
		if err != nil {
			return nil, hostingManifest{}, err
		}
		digests, err := bundle.Digests(name)
		if err != nil {
			return nil, hostingManifest{}, err
		}
		metadata, err := bundle.Package(name)
		if err != nil {
			return nil, hostingManifest{}, err
		}
		zipName := name + ".zip"
		files[zipName] = archive
		zipDigest := sha256.Sum256(archive)
		// 平铺副本:每个文件按原路径输出(<skill>/<relative>),供
		// 「云端说明书」用法直接按 URL 读单个文件(SKILL.md、脚本等),
		// 字节与 zip 内同名条目同源,不需要下载整包。
		for _, relative := range listing {
			data, err := fs.ReadFile(skillsFS, name+"/"+relative)
			if err != nil {
				return nil, hostingManifest{}, fmt.Errorf("read %s/%s: %w", name, relative, err)
			}
			files[name+"/"+relative] = data
		}
		skills[name] = hostedSkill{
			SkillVersion:          metadata.SkillVersion,
			MinimumCLIVersion:     metadata.MinimumCLIVersion,
			CLICompatibility:      metadata.CLICompatibility,
			Zip:                   zipName,
			ZipSHA256:             "sha256:" + hex.EncodeToString(zipDigest[:]),
			FullBundleDigest:      digests.Full,
			EmbeddedContentDigest: digests.Embedded,
			Files:                 listing,
		}
	}
	manifest := hostingManifest{
		SchemaVersion: 1,
		Skills:        skills,
	}
	encoded, err := encodeManifest(manifest)
	if err != nil {
		return nil, hostingManifest{}, err
	}
	files["manifest.json"] = encoded
	return files, manifest, nil
}

// officialSkillDirs 枚举官方技能目录名(排序)。digest、安装目录与 manifest
// key 都以目录名为权威,不读 frontmatter。
func officialSkillDirs(skillsFS fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(skillsFS, ".")
	if err != nil {
		return nil, fmt.Errorf("read skills root: %w", err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("no official skills found")
	}
	sort.Strings(names)
	return names, nil
}

// archiveSkill 打包单个技能:zip 条目名为 <skill>/<relative path>,排序
// 遍历,固定时间戳与权限,不写目录条目;解压到 ~/.agents/skills/ 即完成
// 安装。返回 zip 字节与按序文件清单。
func archiveSkill(skillsFS fs.FS, skill string) ([]byte, []string, error) {
	listing, err := skillFiles(skillsFS, skill)
	if err != nil {
		return nil, nil, err
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, relative := range listing {
		data, err := fs.ReadFile(skillsFS, skill+"/"+relative)
		if err != nil {
			return nil, nil, fmt.Errorf("read %s/%s: %w", skill, relative, err)
		}
		header := &zip.FileHeader{
			Name:     skill + "/" + relative,
			Method:   zip.Deflate,
			Modified: zipEpoch,
		}
		header.SetMode(0o644)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			return nil, nil, fmt.Errorf("zip entry %s: %w", relative, err)
		}
		if _, err := entry.Write(data); err != nil {
			return nil, nil, fmt.Errorf("zip write %s: %w", relative, err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, nil, fmt.Errorf("close zip: %w", err)
	}
	return buffer.Bytes(), listing, nil
}

func skillFiles(skillsFS fs.FS, skill string) ([]string, error) {
	var listing []string
	err := fs.WalkDir(skillsFS, skill, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		listing = append(listing, strings.TrimPrefix(name, skill+"/"))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", skill, err)
	}
	if len(listing) == 0 {
		return nil, fmt.Errorf("skill %s has no files", skill)
	}
	sort.Strings(listing)
	return listing, nil
}

func encodeManifest(manifest hostingManifest) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(manifest); err != nil {
		return nil, fmt.Errorf("encode hosting manifest: %w", err)
	}
	return buffer.Bytes(), nil
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
