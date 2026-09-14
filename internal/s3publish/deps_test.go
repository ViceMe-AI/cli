package s3publish

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestVicemeDoesNotDependOnAWSSDK(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate source file")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..")
	cmd := exec.Command("go", "list", "-deps", "./cmd/viceme")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps ./cmd/viceme: %v", err)
	}
	if strings.Contains(string(out), "github.com/aws/aws-sdk-go-v2") {
		t.Fatal("cmd/viceme must not depend on aws-sdk-go-v2")
	}
}
