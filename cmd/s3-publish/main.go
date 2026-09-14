package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ViceMe-AI/cli/internal/s3publish"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := configFromEnv()
	if err != nil {
		return err
	}
	return s3publish.Publish(context.Background(), cfg)
}

func configFromEnv() (s3publish.Config, error) {
	required := []string{
		"VERSION",
		"GITHUB_RUN_ID",
		"CN_ENDPOINT",
		"CN_BUCKET",
		"CN_ACCESS_KEY_ID",
		"CN_SECRET_ACCESS_KEY",
		"CN_S3_HTTPS_PROXY",
		"GLOBAL_ENDPOINT",
		"GLOBAL_BUCKET",
		"GLOBAL_ACCESS_KEY_ID",
		"GLOBAL_SECRET_ACCESS_KEY",
	}
	for _, name := range required {
		if strings.TrimSpace(os.Getenv(name)) == "" {
			return s3publish.Config{}, fmt.Errorf("%s is required", name)
		}
	}
	concurrency := 0
	if raw := strings.TrimSpace(os.Getenv("S3_PUBLISH_CONCURRENCY")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			return s3publish.Config{}, fmt.Errorf("S3_PUBLISH_CONCURRENCY must be a positive integer")
		}
		concurrency = value
	}
	distDir := strings.TrimSpace(os.Getenv("DIST_DIR"))
	if distDir == "" {
		distDir = "dist"
	}
	return s3publish.Config{
		DistDir:     distDir,
		Version:     os.Getenv("VERSION"),
		RunID:       os.Getenv("GITHUB_RUN_ID"),
		Concurrency: concurrency,
		Log:         os.Stderr,
		Regions: []s3publish.Region{
			{
				Label:        "CN",
				Endpoint:     os.Getenv("CN_ENDPOINT"),
				Bucket:       os.Getenv("CN_BUCKET"),
				AccessKey:    os.Getenv("CN_ACCESS_KEY_ID"),
				SecretKey:    os.Getenv("CN_SECRET_ACCESS_KEY"),
				ProxyURL:     os.Getenv("CN_S3_HTTPS_PROXY"),
				PublicOrigin: "https://s3.viceme.cn/start",
			},
			{
				Label:        "GLOBAL",
				Endpoint:     os.Getenv("GLOBAL_ENDPOINT"),
				Bucket:       os.Getenv("GLOBAL_BUCKET"),
				AccessKey:    os.Getenv("GLOBAL_ACCESS_KEY_ID"),
				SecretKey:    os.Getenv("GLOBAL_SECRET_ACCESS_KEY"),
				PublicOrigin: "https://s3.viceme.ai/start",
			},
		},
	}, nil
}
