package main

import (
	"context"
	"fmt"
	"github.com/ViceMe-AI/cli/internal/s3publish"
	"os"
)

func main() {
	regions := []s3publish.Region{}
	for _, name := range []string{"CN", "GLOBAL"} {
		origin := "https://s3.dev.viceme.cn/dev"
		if name == "GLOBAL" {
			origin = "https://s3.viceme.ai/dev"
		}
		regions = append(regions, s3publish.Region{Label: name, Endpoint: os.Getenv(name + "_ENDPOINT"), Bucket: "dev", AccessKey: os.Getenv(name + "_ACCESS_KEY_ID"), SecretKey: os.Getenv(name + "_SECRET_ACCESS_KEY"), ProxyURL: os.Getenv(name + "_S3_HTTPS_PROXY"), PublicOrigin: origin})
	}
	err := s3publish.PublishDev(context.Background(), s3publish.Config{DistDir: os.Getenv("DIST_DIR"), Version: os.Getenv("BUILD_ID"), RunID: os.Getenv("GITHUB_RUN_ID"), Regions: regions, Log: os.Stderr})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
