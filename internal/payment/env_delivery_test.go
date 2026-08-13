package payment

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/output"
)

func TestDeliverAPIKeyToEnvFilePreservesProjectEnvAndProtectsIt(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	target := filepath.Join(root, ".env.local")
	if err := os.WriteFile(target, []byte("APP_NAME=demo\nVICEME_PAYMENT_API_KEY=old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := DeliverAPIKeyToEnvFile(root, ".env.local", "VICEME_PAYMENT_API_KEY", "vcp_sandbox_secret")
	if err != nil {
		t.Fatal(err)
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if result.Created || !result.Updated || !result.GitIgnoreUpdated || result.EnvFile != resolvedTarget {
		t.Fatalf("unexpected delivery result: %#v", result)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "APP_NAME=demo\nVICEME_PAYMENT_API_KEY=vcp_sandbox_secret\n" {
		t.Fatalf("dotenv content was not safely updated: %q", data)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("dotenv permissions are not private: %o", info.Mode().Perm())
		}
	}
	ignore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if string(ignore) != "/.env.local\n" {
		t.Fatalf("unexpected .gitignore content: %q", ignore)
	}
}

func TestDeliverWebhookSigningSecretToEnvFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	secret := "whsec_sandbox_super_secret_value"
	result, err := DeliverWebhookSigningSecretToEnvFile(root, ".env.local", "VICEME_PAYMENT_WEBHOOK_SECRET", secret)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || !result.Updated {
		t.Fatalf("unexpected delivery result: %#v", result)
	}
	data, err := os.ReadFile(filepath.Join(root, ".env.local"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "VICEME_PAYMENT_WEBHOOK_SECRET="+secret+"\n" {
		t.Fatalf("Webhook signing secret was not safely delivered: %q", data)
	}
}

func TestDeliverAPIKeyToEnvFileRejectsDuplicateVariable(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	target := filepath.Join(root, ".env")
	if err := os.WriteFile(target, []byte("VICEME_PAYMENT_API_KEY=one\nexport VICEME_PAYMENT_API_KEY=two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := DeliverAPIKeyToEnvFile(root, ".env", "VICEME_PAYMENT_API_KEY", "vcp_sandbox_secret")
	assertPaymentDeliveryError(t, err, "PAYMENT_ENV_VARIABLE_DUPLICATE")
	data, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if strings.Contains(string(data), "vcp_sandbox_secret") {
		t.Fatal("duplicate-variable failure changed the dotenv file")
	}
}

func TestDeliverAPIKeyToEnvFileRejectsEscapingAndSymlinkTargets(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	_, err := DeliverAPIKeyToEnvFile(root, "../.env", "VICEME_PAYMENT_API_KEY", "vcp_sandbox_secret")
	assertPaymentDeliveryError(t, err, "PAYMENT_ENV_FILE_INVALID")
	_, err = DeliverAPIKeyToEnvFile(root, ".env.example", "VICEME_PAYMENT_API_KEY", "vcp_sandbox_secret")
	assertPaymentDeliveryError(t, err, "PAYMENT_ENV_FILE_UNSAFE")

	if runtime.GOOS == "windows" {
		return
	}
	outside := filepath.Join(t.TempDir(), ".env.local")
	if err := os.WriteFile(outside, []byte("SAFE=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".env.local")); err != nil {
		t.Fatal(err)
	}
	_, err = DeliverAPIKeyToEnvFile(root, ".env.local", "VICEME_PAYMENT_API_KEY", "vcp_sandbox_secret")
	assertPaymentDeliveryError(t, err, "PAYMENT_ENV_FILE_UNSAFE")
}

func TestDeliverAPIKeyToEnvFileRejectsBrowserExposedVariable(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	_, err := DeliverAPIKeyToEnvFile(root, ".env.local", "NEXT_PUBLIC_VICEME_PAYMENT_API_KEY", "vcp_sandbox_secret")
	assertPaymentDeliveryError(t, err, "PAYMENT_ENV_VARIABLE_PUBLIC")
	if _, err := os.Stat(filepath.Join(root, ".env.local")); !os.IsNotExist(err) {
		t.Fatalf("public-variable failure created a dotenv file: %v", err)
	}
}

func TestDeliverAPIKeyToEnvFileRejectsGitTrackedTarget(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	root := t.TempDir()
	if output, err := exec.Command("git", "-C", root, "init", "--quiet").CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v: %s", err, output)
	}
	target := filepath.Join(root, ".env.local")
	if err := os.WriteFile(target, []byte("SAFE=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "-C", root, "add", ".env.local").CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v: %s", err, output)
	}
	_, err := DeliverAPIKeyToEnvFile(root, ".env.local", "VICEME_PAYMENT_API_KEY", "vcp_sandbox_secret")
	assertPaymentDeliveryError(t, err, "PAYMENT_ENV_FILE_TRACKED")
	data, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if strings.Contains(string(data), "vcp_sandbox_secret") {
		t.Fatal("tracked-file failure changed the dotenv file")
	}
}

func assertPaymentDeliveryError(t *testing.T, err error, code string) {
	t.Helper()
	var cliError *output.Error
	if !errors.As(err, &cliError) || cliError.Subtype != code {
		t.Fatalf("expected %s, got %#v", code, err)
	}
}
