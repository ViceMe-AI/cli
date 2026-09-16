package main

import (
	"strings"
	"testing"
)

func TestConfigFromEnvRequiresSecretsWithoutPrintingThem(t *testing.T) {
	t.Setenv("VERSION", "0.42.0")
	t.Setenv("GITHUB_RUN_ID", "1")
	t.Setenv("CN_ENDPOINT", "https://example.invalid")
	t.Setenv("CN_BUCKET", "start")
	t.Setenv("CN_ACCESS_KEY_ID", "super-secret-key")
	t.Setenv("CN_SECRET_ACCESS_KEY", "super-secret-secret")
	t.Setenv("CN_S3_HTTPS_PROXY", "")
	t.Setenv("GLOBAL_ENDPOINT", "https://example.invalid")
	t.Setenv("GLOBAL_BUCKET", "start")
	t.Setenv("GLOBAL_ACCESS_KEY_ID", "global-key")
	t.Setenv("GLOBAL_SECRET_ACCESS_KEY", "global-secret")
	_, err := configFromEnv()
	if err == nil {
		t.Fatal("expected missing proxy to fail")
	}
	message := err.Error()
	if !strings.Contains(message, "CN_S3_HTTPS_PROXY") {
		t.Fatalf("error should name the missing variable: %s", message)
	}
	if strings.Contains(message, "super-secret") || strings.Contains(message, "global-secret") {
		t.Fatalf("error leaked a credential: %s", message)
	}
}

func TestConfigFromEnvRejectsInvalidConcurrency(t *testing.T) {
	t.Setenv("VERSION", "0.42.0")
	t.Setenv("GITHUB_RUN_ID", "1")
	t.Setenv("CN_ENDPOINT", "https://example.invalid")
	t.Setenv("CN_BUCKET", "start")
	t.Setenv("CN_ACCESS_KEY_ID", "k")
	t.Setenv("CN_SECRET_ACCESS_KEY", "s")
	t.Setenv("CN_S3_HTTPS_PROXY", "http://127.0.0.1:8888")
	t.Setenv("GLOBAL_ENDPOINT", "https://example.invalid")
	t.Setenv("GLOBAL_BUCKET", "start")
	t.Setenv("GLOBAL_ACCESS_KEY_ID", "k")
	t.Setenv("GLOBAL_SECRET_ACCESS_KEY", "s")
	t.Setenv("S3_PUBLISH_CONCURRENCY", "0")
	_, err := configFromEnv()
	if err == nil {
		t.Fatal("expected invalid concurrency to fail")
	}
}
