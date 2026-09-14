package s3publish

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ViceMe-AI/cli/internal/semver"
	"golang.org/x/sync/errgroup"
)

// Region is one S3-compatible origin (CN or Global) plus its public URL.
type Region struct {
	Label        string
	Endpoint     string
	Bucket       string
	AccessKey    string
	SecretKey    string
	ProxyURL     string
	PublicOrigin string
}

// Config is the complete publication request. Credentials stay in Region
// fields and must never be copied into argv, logs, or error strings.
type Config struct {
	DistDir     string
	Version     string
	RunID       string
	Concurrency int
	Regions     []Region
	Log         io.Writer
}

type upload struct {
	Path        string
	Key         string
	ContentType string
	Cache       string
	Immutable   bool
	Bucket      string
}

func (c Config) logWriter() io.Writer {
	if c.Log != nil {
		return c.Log
	}
	return io.Discard
}

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(p)
}

func (c Config) concurrency() int {
	if c.Concurrency <= 0 {
		return defaultConcurrency
	}
	return c.Concurrency
}

var requiredDistFiles = []string{
	"agent-install.md",
	"commerce-skill-install.md",
	"agent-release-manifest.json",
	"agent-release-manifest.sigstore.json",
	"install.sh",
	"install.ps1",
	"bootstrap-contract.json",
	"release-manifest.json",
}

var prefixPointerFiles = []string{
	"install.sh",
	"install.ps1",
	"bootstrap-contract.json",
	"release-manifest.json",
	"agent-release-manifest.json",
	"agent-release-manifest.sigstore.json",
	"agent-install.md",
	"commerce-skill-install.md",
}

var rootPointerFiles = []string{
	"install.sh",
	"install.ps1",
	"agent-install.md",
	"commerce-skill-install.md",
}

// Publish writes the assembled dist contract to every configured region,
// then compares the public CN and Global pointers.
func Publish(ctx context.Context, cfg Config) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}
	if cfg.Log == nil {
		cfg.Log = io.Discard
	}
	cfg.Log = &lockedWriter{w: cfg.Log}
	versioned, skills, err := collectUploads(cfg.DistDir, cfg.Version)
	if err != nil {
		return err
	}
	parent := ctx
	group, ctx := errgroup.WithContext(ctx)
	for _, region := range cfg.Regions {
		region := region
		group.Go(func() error {
			return publishRegion(ctx, cfg, region, versioned, skills)
		})
	}
	if err := group.Wait(); err != nil {
		return err
	}
	return crossVerify(parent, cfg)
}

func validateConfig(cfg Config) error {
	if strings.TrimSpace(cfg.DistDir) == "" {
		return fmt.Errorf("dist directory is required")
	}
	if strings.TrimSpace(cfg.Version) == "" {
		return fmt.Errorf("release version is required")
	}
	if strings.TrimSpace(cfg.RunID) == "" {
		return fmt.Errorf("run id is required")
	}
	if len(cfg.Regions) == 0 {
		return fmt.Errorf("at least one region is required")
	}
	for _, region := range cfg.Regions {
		if region.Label == "" || region.Endpoint == "" || region.Bucket == "" || region.AccessKey == "" || region.SecretKey == "" || region.PublicOrigin == "" {
			return fmt.Errorf("%s region configuration is incomplete", orLabel(region.Label))
		}
	}
	for _, name := range requiredDistFiles {
		if err := requireFile(filepath.Join(cfg.DistDir, name)); err != nil {
			return err
		}
	}
	if err := requireFile(filepath.Join(cfg.DistDir, "skills", "manifest.json")); err != nil {
		return err
	}
	return nil
}

func orLabel(label string) string {
	if label == "" {
		return "unnamed"
	}
	return label
}

func requireFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("required dist file %s is missing", filepath.Base(path))
	}
	if info.IsDir() {
		return fmt.Errorf("required dist file %s is a directory", filepath.Base(path))
	}
	return nil
}

func collectUploads(distDir, version string) (versioned []upload, skills []upload, err error) {
	prefix := releasePrefix + "/v" + version + "/"
	entries, err := os.ReadDir(distDir)
	if err != nil {
		return nil, nil, fmt.Errorf("read dist directory failed")
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		versioned = append(versioned, upload{
			Path:        filepath.Join(distDir, name),
			Key:         prefix + name,
			ContentType: flatDistContentType(name),
			Cache:       cacheImmutable,
			Immutable:   true,
		})
	}
	skillsDir := filepath.Join(distDir, "skills")
	err = filepath.WalkDir(skillsDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(skillsDir, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		item := upload{
			Path:        path,
			Key:         prefix + "skills/" + rel,
			ContentType: HostedContentType(rel),
			Cache:       cacheImmutable,
			Immutable:   true,
		}
		versioned = append(versioned, item)
		skills = append(skills, upload{
			Path:        path,
			Key:         rel,
			ContentType: HostedContentType(rel),
			Cache:       cacheStable,
			Immutable:   false,
			Bucket:      skillsBucket,
		})
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("read dist skills tree failed")
	}
	sort.Slice(versioned, func(i, j int) bool { return versioned[i].Key < versioned[j].Key })
	sort.Slice(skills, func(i, j int) bool { return skills[i].Key < skills[j].Key })
	return versioned, skills, nil
}

type regionRuntime struct {
	cfg    Config
	region Region
	store  objectStore
	http   *http.Client
}

func publishRegion(ctx context.Context, cfg Config, region Region, versioned, skills []upload) error {
	httpClient, err := httpClientFor(region.ProxyURL, defaultTimeout)
	if err != nil {
		return fmt.Errorf("%s region proxy setup failed", region.Label)
	}
	client, err := newS3Client(ctx, region, httpClient)
	if err != nil {
		return err
	}
	runtime := &regionRuntime{
		cfg:    cfg,
		region: region,
		store:  &awsStore{client: client, label: region.Label},
		http:   httpClient,
	}
	if err := runtime.publishVersioned(ctx, versioned); err != nil {
		return err
	}
	highest, err := runtime.isHighest(ctx)
	if err != nil {
		return err
	}
	if highest {
		if err := runtime.publishLatest(ctx, skills); err != nil {
			return err
		}
	}
	if err := runtime.probeAndVerify(ctx, highest); err != nil {
		return err
	}
	runtime.log("Published %s release v%s", region.Label, cfg.Version)
	return nil
}

func (r *regionRuntime) publishVersioned(ctx context.Context, items []upload) error {
	listed, err := r.store.List(ctx, r.region.Bucket, releasePrefix+"/v"+r.cfg.Version+"/")
	if err != nil {
		return err
	}
	return r.runUploads(ctx, "versioned", r.region.Bucket, items, listed)
}

func (r *regionRuntime) publishLatest(ctx context.Context, skills []upload) error {
	latest := []byte(r.cfg.Version + "\n")
	if err := r.store.Put(ctx, r.region.Bucket, releasePrefix+"/latest", latest, cacheLatest, plainType); err != nil {
		return err
	}
	r.log("%s latest put %s", r.region.Label, releasePrefix+"/latest")

	var pointers []upload
	for _, name := range prefixPointerFiles {
		contentType := flatDistContentType(name)
		if name == "install.sh" || name == "install.ps1" {
			contentType = ""
		}
		if strings.HasSuffix(name, ".md") {
			contentType = markdownType
		}
		pointers = append(pointers, upload{
			Path:        filepath.Join(r.cfg.DistDir, name),
			Key:         releasePrefix + "/" + name,
			ContentType: contentType,
			Cache:       cacheStable,
		})
	}
	for _, name := range rootPointerFiles {
		contentType := ""
		if strings.HasSuffix(name, ".md") {
			contentType = markdownType
		}
		pointers = append(pointers, upload{
			Path:        filepath.Join(r.cfg.DistDir, name),
			Key:         name,
			ContentType: contentType,
			Cache:       cacheStable,
		})
	}
	if err := r.runUploads(ctx, "pointers", r.region.Bucket, pointers, listing{available: false}); err != nil {
		return err
	}
	skillsListing, err := r.store.List(ctx, skillsBucket, "")
	if err != nil {
		return err
	}
	return r.runUploads(ctx, "stable-skills", skillsBucket, skills, skillsListing)
}

func (r *regionRuntime) isHighest(ctx context.Context) (bool, error) {
	existing := "0.0.0"
	data, err := r.store.Get(ctx, r.region.Bucket, releasePrefix+"/latest")
	switch {
	case err == nil:
		existing = strings.TrimSpace(strings.ReplaceAll(string(data), "\r", ""))
		if existing == "" {
			existing = "0.0.0"
		}
	case isNotFound(err):
		existing = "0.0.0"
	default:
		return false, err
	}
	comparison, err := semver.Compare(r.cfg.Version, existing)
	if err != nil {
		return false, fmt.Errorf("%s latest pointer is not a semantic version", r.region.Label)
	}
	return comparison >= 0, nil
}

func (r *regionRuntime) runUploads(ctx context.Context, phase, bucket string, items []upload, listed listing) error {
	if len(items) == 0 {
		return nil
	}
	group, ctx := errgroup.WithContext(ctx)
	group.SetLimit(r.cfg.concurrency())
	var done atomic.Int64
	total := len(items)
	for _, item := range items {
		item := item
		targetBucket := bucket
		if item.Bucket != "" {
			targetBucket = item.Bucket
		}
		group.Go(func() error {
			body, err := os.ReadFile(item.Path)
			if err != nil {
				return fmt.Errorf("%s read %s failed", r.region.Label, filepath.Base(item.Path))
			}
			action, err := r.ensureObject(ctx, targetBucket, item, body, listed)
			if err != nil {
				return err
			}
			n := done.Add(1)
			r.log("%s %s %d/%d %s %s", r.region.Label, phase, n, total, action, item.Key)
			return nil
		})
	}
	return group.Wait()
}

func (r *regionRuntime) ensureObject(ctx context.Context, bucket string, item upload, body []byte, listed listing) (string, error) {
	exists, meta, err := r.objectExists(ctx, bucket, item.Key, listed)
	if err != nil {
		return "", err
	}
	if exists {
		if item.Immutable {
			remote, getErr := r.store.Get(ctx, bucket, item.Key)
			if getErr != nil {
				return "", getErr
			}
			if !bytes.Equal(remote, body) {
				return "", fmt.Errorf("%s immutable object %s already exists with different bytes", r.region.Label, item.Key)
			}
			return "skip-identical", nil
		}
		if meta.Size == int64(len(body)) && etagMatches(meta.ETag, body) {
			return "skip-identical", nil
		}
		if meta.ETag == "" || strings.Contains(meta.ETag, "-") || meta.Size != int64(len(body)) {
			remote, getErr := r.store.Get(ctx, bucket, item.Key)
			if getErr != nil && !isNotFound(getErr) {
				return "", getErr
			}
			if getErr == nil && bytes.Equal(remote, body) {
				return "skip-identical", nil
			}
		} else if etagMatches(meta.ETag, body) {
			return "skip-identical", nil
		}
	}
	if err := r.store.Put(ctx, bucket, item.Key, body, item.Cache, item.ContentType); err != nil {
		return "", err
	}
	return "put", nil
}

func (r *regionRuntime) objectExists(ctx context.Context, bucket, key string, listed listing) (bool, objectMeta, error) {
	if listed.available {
		meta, ok := listed.objects[key]
		return ok, meta, nil
	}
	meta, ok, err := r.store.Head(ctx, bucket, key)
	return ok, meta, err
}

func (r *regionRuntime) log(format string, args ...any) {
	fmt.Fprintf(r.cfg.logWriter(), format+"\n", args...)
}
