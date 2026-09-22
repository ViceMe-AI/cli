package s3publish

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDevPublicationCannotTargetProduction(t *testing.T) {
	cfg := Config{DistDir: t.TempDir(), Version: "dev-123-1-abcdefabcdef", Regions: []Region{{Label: "CN", Bucket: "start", PublicOrigin: "https://s3.viceme.cn/start"}}}
	if err := PublishDev(context.Background(), cfg); err == nil {
		t.Fatal("production target accepted")
	}
	cfg.Regions[0].Bucket = "dev"
	if err := PublishDev(context.Background(), cfg); err == nil {
		t.Fatal("production public origin accepted")
	}
}
func TestDevBuildOrder(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want bool
	}{
		{"dev-123-2-abcdefabcdef", "dev-123-1-abcdefabcdef", true},
		{"dev-124-1-abcdefabcdef", "dev-123-9-abcdefabcdef", true},
		{"dev-123-1-abcdefabcdef", "dev-124-1-abcdefabcdef", false},
		{"dev-123-1-000000000000", "dev-123-1-abcdefabcdef", false},
	} {
		if got := newerDevBuild(tt.a, tt.b); got != tt.want {
			t.Fatalf("%s vs %s: %v", tt.a, tt.b, got)
		}
	}
}

func TestDevPublicationWritesIsolatedImmutableBuildBeforePointer(t *testing.T) {
	fake := newFakeS3()
	defer fake.close()
	root := t.TempDir()
	build := "dev-123-1-abcdefabcdef"
	packages := []map[string]string{}
	write := func(name string, data []byte) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, osname := range []string{"darwin", "linux", "windows"} {
		for _, arch := range []string{"amd64", "arm64"} {
			file := "viceme-" + build + "-" + osname + "-" + arch + ".zip"
			data := []byte(osname + arch)
			hash := sha256.Sum256(data)
			write(file, data)
			packages = append(packages, map[string]string{"file": file, "os": osname, "arch": arch, "sha256": hex.EncodeToString(hash[:])})
		}
	}
	manifest, _ := json.Marshal(map[string]any{"schemaVersion": 1, "channel": "dev", "sourceDirty": false, "buildId": build, "commit": "abcdefabcdef0000000000000000000000000000", "packages": packages})
	write("delivery.json", manifest)
	for _, name := range []string{"SHA256SUMS", "start/agent-install.md", "skills/manifest.json", "skills/use-a-skill/scripts/trial.py"} {
		write(name, []byte(name))
	}
	cfg := Config{DistDir: root, Version: build, Regions: []Region{{Label: "CN", Bucket: "dev", PublicOrigin: "https://s3.dev.viceme.cn/dev", Endpoint: fake.endpoint(), AccessKey: "test", SecretKey: "test"}}}
	factory := func(ctx context.Context, cfg Config, region Region) (*regionRuntime, error) {
		client := fake.server.Client()
		s3, err := newS3Client(ctx, region, client)
		if err != nil {
			return nil, err
		}
		region.PublicOrigin = fake.endpoint() + "/dev"
		return &regionRuntime{cfg: cfg, region: region, store: &awsStore{client: s3, label: region.Label}, http: client}, nil
	}
	if err := publishDev(context.Background(), cfg, factory); err != nil {
		t.Fatal(err)
	}
	if _, ok := fake.get("dev", "builds/"+build+"/delivery.json"); !ok {
		t.Fatal("immutable manifest absent")
	}
	if got, ok := fake.get("dev", "delivery.json"); !ok || string(got.Body) != string(manifest) {
		t.Fatal("pointer differs")
	}
	for _, key := range fake.putKeys {
		if !strings.HasPrefix(key, "dev\x00") {
			t.Fatal("production bucket mutated", key)
		}
	}
	if fake.putKeys[len(fake.putKeys)-1] != objectID("dev", "delivery.json") {
		t.Fatal("pointer was not last")
	}
	// Same bytes are idempotent; changed immutable bytes must never advance pointers.
	if err := publishDev(context.Background(), cfg, factory); err != nil {
		t.Fatal(err)
	}
	write("start/agent-install.md", []byte("changed"))
	previous := fake.putCount("dev", "delivery.json")
	if err := publishDev(context.Background(), cfg, factory); err == nil {
		t.Fatal("immutable conflict accepted")
	}
	if fake.putCount("dev", "delivery.json") != previous {
		t.Fatal("advanced pointer after failed immutable write")
	}
}
