package installers

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// The downloaded fixture must be an actual Windows executable, not a .cmd
// renamed to .exe. Reuse this test binary in a narrowly selected child mode.
func init() {
	if os.Getenv("VICEME_TEST_WINDOWS_BOOTSTRAP_HELPER") != "1" || len(os.Args) < 2 || os.Args[1] != "bootstrap" {
		return
	}
	code, _ := strconv.Atoi(os.Getenv("VICEME_TEST_ACTIVATION_EXIT"))
	if code == 0 {
		for i := 1; i+1 < len(os.Args); i++ {
			if os.Args[i] == "--destination" {
				data, err := os.ReadFile(os.Args[0])
				if err != nil || os.WriteFile(os.Args[i+1], data, 0o755) != nil {
					os.Exit(9)
				}
			}
		}
	}
	fmt.Printf("{\"ok\":%t}\n", code == 0)
	os.Exit(code)
}

func TestPowerShellInstallerPermissionRetry(t *testing.T) {
	// Successful Windows installation persists the user's PATH. Run only on a
	// disposable CI host, and still restore that registry value in the harness.
	if os.Getenv("CI") != "true" {
		t.Skip("requires disposable Windows CI user for persistent PATH integration")
	}
	root := filepath.Join(t.TempDir(), "user space")
	fixtures := filepath.Join(root, "fixtures")
	temporary := filepath.Join(root, "tmp")
	installDir := filepath.Join(root, "bin")
	for _, directory := range []string{fixtures, temporary, installDir} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	asset := "viceme_1.2.3_windows_" + runtime.GOARCH + ".exe"
	if err := os.WriteFile(filepath.Join(fixtures, asset), binary, 0o755); err != nil {
		t.Fatal(err)
	}
	writeInstallerTestFile(t, filepath.Join(fixtures, asset+".sha256"), fmt.Sprintf("%x  %s", sha256.Sum256(binary), asset), 0o600)
	destination := filepath.Join(installDir, "viceme.exe")
	writeInstallerTestFile(t, destination, "previous installation", 0o755)
	requests := filepath.Join(root, "requests.txt")
	installer, err := filepath.Abs("install.ps1")
	if err != nil {
		t.Fatal(err)
	}
	wrapper := filepath.Join(root, "invoke.ps1")
	writeInstallerTestFile(t, wrapper, `$ErrorActionPreference = 'Stop'
# The CI driver is PowerShell 7; this child exercises Windows PowerShell 5.1.
# Its module discovery must use the child runtime rather than the parent's.
$env:PSModulePath = Join-Path $PSHOME 'Modules'
function Invoke-WebRequest {
  param([switch]$UseBasicParsing, [int]$TimeoutSec, [string]$Uri, [string]$OutFile)
  Add-Content -LiteralPath $env:VICEME_TEST_REQUESTS -Value $Uri
  Copy-Item -LiteralPath (Join-Path $env:VICEME_TEST_FIXTURES ([IO.Path]::GetFileName($Uri))) -Destination $OutFile
}
$originalPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$installerExit = 1
try { & $env:VICEME_TEST_INSTALLER; $installerExit = $LASTEXITCODE } finally { [Environment]::SetEnvironmentVariable('Path', $originalPath, 'User') }
exit $installerExit
`, 0o600)
	for _, code := range []int{6, 6, 0} {
		command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", wrapper)
		command.Env = append(os.Environ(), "TEMP="+temporary, "TMP="+temporary,
			"VICEME_INSTALL_DIR="+installDir, "VICEME_VERSION=1.2.3",
			"VICEME_TEST_INSTALLER="+installer, "VICEME_TEST_REQUESTS="+requests, "VICEME_TEST_FIXTURES="+fixtures,
			"VICEME_TEST_WINDOWS_BOOTSTRAP_HELPER=1", fmt.Sprintf("VICEME_TEST_ACTIVATION_EXIT=%d", code))
		output, err := command.CombinedOutput()
		actual := 0
		if err != nil {
			failure, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatal(err)
			}
			actual = failure.ExitCode()
		}
		if actual != code {
			t.Fatalf("PowerShell lost activation exit: want=%d got=%d output=%s", code, actual, output)
		}
		if code == 6 {
			data, err := os.ReadFile(destination)
			if err != nil || string(data) != "previous installation" {
				t.Fatalf("permission failure changed installed executable: %v", err)
			}
		}
	}
	data, err := os.ReadFile(requests)
	if err != nil {
		t.Fatal(err)
	}
	binaryRequests, checksumRequests := 0, 0
	for _, request := range strings.Fields(string(data)) {
		if strings.HasSuffix(request, asset+".sha256") {
			checksumRequests++
		} else if strings.HasSuffix(request, asset) {
			binaryRequests++
		}
	}
	if binaryRequests != 1 || checksumRequests != 3 {
		t.Fatalf("PowerShell did not resume a verified download: binary=%d checksum=%d", binaryRequests, checksumRequests)
	}
	entries, err := os.ReadDir(filepath.Join(temporary, "viceme-bootstrap-cache"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("successful activation left cached bytes: entries=%v err=%v", entries, err)
	}
}
