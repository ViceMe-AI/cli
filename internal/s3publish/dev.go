package s3publish

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Dev publication uses a separate storage endpoint. It cannot invoke stable
// release publication, touch production pointers or publish npm/tags.
var devBuildPattern = regexp.MustCompile("^dev-([0-9]+)-([0-9]+)-[a-f0-9]{12}$")

func PublishDev(ctx context.Context, cfg Config) error {
	return publishDev(ctx, cfg, func(ctx context.Context, cfg Config, region Region) (*regionRuntime, error) {
		client, err := httpClientFor(region.ProxyURL, defaultTimeout)
		if err != nil {
			return nil, err
		}
		s3, err := newS3Client(ctx, region, client)
		if err != nil {
			return nil, err
		}
		return &regionRuntime{cfg: cfg, region: region, store: &awsStore{client: s3, label: region.Label}, http: client}, nil
	})
}

func publishDev(ctx context.Context, cfg Config, factory func(context.Context, Config, Region) (*regionRuntime, error)) error {
	if !devBuildPattern.MatchString(cfg.Version) || cfg.DistDir == "" || len(cfg.Regions) != 1 {
		return errors.New("invalid dev publication")
	}
	for _, region := range cfg.Regions {
		if region.Label != "CN" || region.Bucket != "start" || region.PublicOrigin != "https://s3.dev.viceme.cn/start" || region.Endpoint != "https://s3.dev.viceme.cn" || region.AccessKey == "" || region.SecretKey == "" {
			return errors.New("dev credentials must target the isolated dev endpoint and official start origin")
		}
	}
	var manifest struct {
		SchemaVersion int    `json:"schemaVersion"`
		Channel       string `json:"channel"`
		SourceDirty   bool   `json:"sourceDirty"`
		BuildID       string `json:"buildId"`
		Commit        string `json:"commit"`
		Packages      []struct {
			File   string `json:"file"`
			OS     string `json:"os"`
			Arch   string `json:"arch"`
			SHA256 string `json:"sha256"`
		} `json:"packages"`
	}
	data, err := os.ReadFile(filepath.Join(cfg.DistDir, "delivery.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	if manifest.SchemaVersion != 1 || manifest.Channel != "dev" || manifest.SourceDirty || manifest.BuildID != cfg.Version || len(manifest.Commit) != 40 || !strings.HasSuffix(cfg.Version, manifest.Commit[:12]) || len(manifest.Packages) != 6 {
		return errors.New("dev delivery manifest is incomplete")
	}
	seen := map[string]bool{}
	for _, pkg := range manifest.Packages {
		platform := pkg.OS + "-" + pkg.Arch
		if (pkg.OS != "darwin" && pkg.OS != "linux" && pkg.OS != "windows") || (pkg.Arch != "amd64" && pkg.Arch != "arm64") || seen[platform] || pkg.File != "viceme-"+cfg.Version+"-"+platform+".zip" {
			return errors.New("invalid dev package identity")
		}
		seen[platform] = true
		content, err := os.ReadFile(filepath.Join(cfg.DistDir, pkg.File))
		if err != nil {
			return err
		}
		digest := sha256.Sum256(content)
		if hex.EncodeToString(digest[:]) != pkg.SHA256 {
			return errors.New("dev package checksum mismatch")
		}
	}
	var immutable, startAliases, skillAliases []upload
	err = filepath.WalkDir(cfg.DistDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("dev artifacts cannot contain symlinks")
		}
		relative, err := filepath.Rel(cfg.DistDir, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		item := upload{Path: path, Key: "builds/" + cfg.Version + "/" + relative, ContentType: HostedContentType(relative), Cache: cacheImmutable, Immutable: true}
		immutable = append(immutable, item)
		if strings.HasPrefix(relative, "skills/") || strings.HasPrefix(relative, "start/") {
			_, item.Key, _ = strings.Cut(relative, "/")
			item.Cache = cacheStable
			item.Immutable = false
			if strings.HasPrefix(relative, "start/") {
				startAliases = append(startAliases, item)
			} else {
				skillAliases = append(skillAliases, item)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, region := range cfg.Regions {
		r, err := factory(ctx, cfg, region)
		if err != nil {
			return err
		}
		region = r.region

		if err := r.runUploads(ctx, "dev-immutable", region.Bucket, immutable, listing{available: false}); err != nil {
			return err
		}
		// Verify the actual public installer bytes before publishing mutable entrypoints.
		checks := []string{"delivery.json", "SHA256SUMS", "start/agent-install.md", "skills/manifest.json", "skills/use-a-skill/scripts/trial.py"}
		for _, p := range manifest.Packages {
			checks = append(checks, p.File)
		}
		for _, file := range checks {
			if err := r.verifyPublic(ctx, publicCheck{URL: region.PublicOrigin + "/builds/" + cfg.Version + "/" + file, File: file, Immutable: true}); err != nil {
				return err
			}
		}
		prior, err := r.store.Get(ctx, region.Bucket, "delivery.json")
		if err != nil && !isNotFound(err) {
			return err
		}
		if err == nil {
			var old struct {
				BuildID string `json:"buildId"`
			}
			if json.Unmarshal(prior, &old) != nil || !devBuildPattern.MatchString(old.BuildID) {
				return errors.New("invalid existing dev pointer")
			}
			if !newerDevBuild(cfg.Version, old.BuildID) {
				return fmt.Errorf("dev pointer already targets newer build %s; immutable artifacts retained", old.BuildID)
			}
		}
		if err := r.runUploads(ctx, "dev-start", "start", startAliases, listing{available: false}); err != nil {
			return err
		}
		if err := r.runUploads(ctx, "dev-skills", "skills", skillAliases, listing{available: false}); err != nil {
			return err
		}
		if err := r.store.Put(ctx, region.Bucket, "delivery.json", data, "no-cache", "application/json"); err != nil {
			return err
		}
		for _, file := range []string{"delivery.json", "start/agent-install.md", "skills/use-a-skill/scripts/trial.py"} {
			url := strings.TrimSuffix(region.PublicOrigin, "/start") + "/" + file
			if file == "delivery.json" {
				url = region.PublicOrigin + "/" + file
			}
			if err := r.verifyPublic(ctx, publicCheck{URL: url, File: file}); err != nil {
				return err
			}
		}
	}
	return nil
}
func newerDevBuild(candidate, previous string) bool {
	a, b := devBuildPattern.FindStringSubmatch(candidate), devBuildPattern.FindStringSubmatch(previous)
	if a == nil || b == nil {
		return false
	}
	for i := 1; i <= 2; i++ {
		x, e1 := strconv.ParseUint(a[i], 10, 64)
		y, e2 := strconv.ParseUint(b[i], 10, 64)
		if e1 != nil || e2 != nil {
			return false
		}
		if x != y {
			return x > y
		}
	}
	return candidate == previous
}
