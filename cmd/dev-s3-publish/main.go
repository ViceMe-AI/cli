package main

import (
	"context"
	"fmt"
	"os"

	"github.com/ViceMe-AI/cli/internal/s3publish"
)

func main() {
	// The deployed dev environment exists only in CN. Do not require or publish
	// overseas credentials merely because production has two regions.
	region := s3publish.Region{
		Label: "CN", Endpoint: os.Getenv("CN_ENDPOINT"), Bucket: "dev",
		AccessKey: os.Getenv("CN_ACCESS_KEY_ID"), SecretKey: os.Getenv("CN_SECRET_ACCESS_KEY"),
		ProxyURL: os.Getenv("CN_S3_HTTPS_PROXY"), PublicOrigin: "https://s3.dev.viceme.cn/dev",
	}
	err := s3publish.PublishDev(context.Background(), s3publish.Config{DistDir: os.Getenv("DIST_DIR"), Version: os.Getenv("BUILD_ID"), RunID: os.Getenv("GITHUB_RUN_ID"), Regions: []s3publish.Region{region}, Log: os.Stderr})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
