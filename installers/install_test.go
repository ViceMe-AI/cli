package installers

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstallersUsePublicStartBucketURLs(t *testing.T) {
	t.Parallel()
	expected := []string{
		"https://s3.viceme.cn/start/cli/releases",
		"https://s3.viceme.ai/start/cli/releases",
	}
	for _, filename := range []string{"install.sh", "install.ps1"} {
		contents, err := os.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		for _, releaseURL := range expected {
			if !strings.Contains(string(contents), releaseURL) {
				t.Errorf("%s does not use public start bucket URL %q", filename, releaseURL)
			}
		}
	}
}

func TestShellInstallerVerifiesChecksumAndPreservesWorkingVersionOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX bootstrap test")
	}
	root := t.TempDir()
	fixtures := filepath.Join(root, "fixtures")
	fakeBin := filepath.Join(root, "fake-bin")
	installDir := filepath.Join(root, "install")
	for _, directory := range []string{fixtures, fakeBin, installDir} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeInstallerTestFile(t, filepath.Join(fixtures, "latest"), "1.2.3\n", 0o644)
	binary := `#!/bin/sh
printf '%s\n' "$*" >>"$VICEME_TEST_INSTALL_LOG"
if [ "${1:-}" = "bootstrap" ] && [ "${2:-}" = "activate" ]; then
  if [ "${VICEME_TEST_INSTALL_FAIL:-}" = "1" ]; then exit 9; fi
  shift 2
  destination=""
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --destination) destination="$2"; shift 2 ;;
      *) shift ;;
    esac
  done
  cp "$0" "$destination"
  chmod 755 "$destination"
fi
`
	writeInstallerTestFile(t, filepath.Join(fixtures, "viceme_1.2.3_linux_amd64"), binary, 0o755)
	digest := sha256.Sum256([]byte(binary))
	writeInstallerTestFile(t, filepath.Join(fixtures, "viceme_1.2.3_linux_amd64.sha256"), fmt.Sprintf("%x  viceme_1.2.3_linux_amd64\n", digest), 0o644)
	writeInstallerTestFile(t, filepath.Join(fakeBin, "uname"), `#!/bin/sh
case "$1" in
  -s) echo Linux ;;
  -m) echo x86_64 ;;
  *) exit 2 ;;
esac
`, 0o755)
	writeInstallerTestFile(t, filepath.Join(fakeBin, "curl"), `#!/bin/sh
out=""
url=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) out="$2"; shift 2 ;;
    http*) url="$1"; shift ;;
    *) shift ;;
  esac
done
cp "$VICEME_TEST_FIXTURES/${url##*/}" "$out"
`, 0o755)

	logFile := filepath.Join(root, "install.log")
	environment := append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HOME="+root,
		"VICEME_INSTALL_DIR="+installDir,
		"VICEME_DOWNLOAD_BASE_URL=https://downloads.example.test/cli/releases",
		"VICEME_TEST_FIXTURES="+fixtures,
		"VICEME_TEST_INSTALL_LOG="+logFile,
	)
	command := exec.Command("sh", "./install.sh")
	command.Env = environment
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("installer failed: %v\n%s", err, output)
	}
	installed, err := os.ReadFile(filepath.Join(installDir, "viceme"))
	if err != nil || string(installed) != binary {
		t.Fatalf("unexpected installed binary: err=%v content=%q", err, installed)
	}
	logData, err := os.ReadFile(logFile)
	if err != nil || !strings.Contains(string(logData), "bootstrap activate --destination ") || !strings.Contains(string(logData), "--agent auto --region cn") {
		t.Fatalf("official Skills were not installed: err=%v log=%q", err, logData)
	}

	writeInstallerTestFile(t, filepath.Join(fixtures, "viceme_1.2.3_linux_amd64"), "corrupt", 0o755)
	command = exec.Command("sh", "./install.sh")
	command.Env = environment
	if output, err := command.CombinedOutput(); err == nil || !strings.Contains(string(output), "checksum verification failed") {
		t.Fatalf("corrupt release was not rejected: err=%v output=%s", err, output)
	}
	installed, err = os.ReadFile(filepath.Join(installDir, "viceme"))
	if err != nil || string(installed) != binary {
		t.Fatalf("failed reinstall damaged prior version: err=%v content=%q", err, installed)
	}

	writeInstallerTestFile(t, filepath.Join(fixtures, "latest"), "1.2.4\n", 0o644)
	brokenUpgrade := strings.Replace(binary, "#!/bin/sh", "#!/bin/sh\n# release 1.2.4", 1)
	writeInstallerTestFile(t, filepath.Join(fixtures, "viceme_1.2.4_linux_amd64"), brokenUpgrade, 0o755)
	upgradeDigest := sha256.Sum256([]byte(brokenUpgrade))
	writeInstallerTestFile(t, filepath.Join(fixtures, "viceme_1.2.4_linux_amd64.sha256"), fmt.Sprintf("%x  viceme_1.2.4_linux_amd64\n", upgradeDigest), 0o644)
	failingEnvironment := append(append([]string(nil), environment...), "VICEME_TEST_INSTALL_FAIL=1")
	command = exec.Command("sh", "./install.sh")
	command.Env = failingEnvironment
	if output, err := command.CombinedOutput(); err == nil {
		t.Fatalf("failed Skill transaction unexpectedly committed the new binary: output=%s", output)
	}
	installed, err = os.ReadFile(filepath.Join(installDir, "viceme"))
	if err != nil || string(installed) != binary {
		t.Fatalf("Skill failure did not restore the prior binary: err=%v content=%q", err, installed)
	}
}

func TestPOCShellInstallerActivatesSkillsAndPreviewProfile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX bootstrap test")
	}
	root := t.TempDir()
	fixtures := filepath.Join(root, "fixtures")
	fakeBin := filepath.Join(root, "fake-bin")
	installDir := filepath.Join(root, "install")
	for _, directory := range []string{fixtures, fakeBin, installDir} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	binary := `#!/bin/sh
printf '%s\n' "$*" >>"$VICEME_TEST_INSTALL_LOG"
if [ "${1:-}" = "bootstrap" ] && [ "${2:-}" = "activate" ]; then
  shift 2
  destination=""
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --destination) destination="$2"; shift 2 ;;
      *) shift ;;
    esac
  done
  cp "$0" "$destination"
  chmod 755 "$destination"
fi
if [ "${1:-}" = "profile" ] && [ "${2:-}" = "use" ]; then exit 1; fi
`
	asset := "viceme_0.15.3-poc.1_linux_amd64"
	writeInstallerTestFile(t, filepath.Join(fixtures, asset), binary, 0o755)
	digest := sha256.Sum256([]byte(binary))
	writeInstallerTestFile(t, filepath.Join(fixtures, asset+".sha256"), fmt.Sprintf("%x  %s\n", digest, asset), 0o644)
	writeInstallerTestFile(t, filepath.Join(fakeBin, "uname"), `#!/bin/sh
case "$1" in
  -s) echo Linux ;;
  -m) echo x86_64 ;;
  *) exit 2 ;;
esac
`, 0o755)
	writeInstallerTestFile(t, filepath.Join(fakeBin, "curl"), `#!/bin/sh
out=""
url=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) out="$2"; shift 2 ;;
    http*) url="$1"; shift ;;
    *) shift ;;
  esac
done
cp "$VICEME_TEST_FIXTURES/${url##*/}" "$out"
`, 0o755)

	logFile := filepath.Join(root, "install.log")
	command := exec.Command("sh", "./install-poc.sh")
	command.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HOME="+root,
		"VICEME_INSTALL_DIR="+installDir,
		"VICEME_TEST_FIXTURES="+fixtures,
		"VICEME_TEST_INSTALL_LOG="+logFile,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("POC installer failed: %v\n%s", err, output)
	}
	installed, err := os.ReadFile(filepath.Join(installDir, "viceme"))
	if err != nil || string(installed) != binary {
		t.Fatalf("unexpected installed POC binary: err=%v content=%q", err, installed)
	}
	logData, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	logText := string(logData)
	for _, expected := range []string{
		"bootstrap activate --destination ",
		"--agent auto --region cn",
		"profile add --name danmaku-poc-20260818 --region cn --api-base-url https://api-poc.preview.tencent-zeabur.cn --use",
	} {
		if !strings.Contains(logText, expected) {
			t.Fatalf("POC installer did not execute %q: log=%q", expected, logText)
		}
	}
}

func writeInstallerTestFile(t *testing.T, filename, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(filename, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}
