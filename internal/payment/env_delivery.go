package payment

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
)

const maxPaymentEnvFileBytes = 1 << 20

var paymentEnvVariablePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var paymentSecretPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
var webhookSigningSecretPattern = regexp.MustCompile(`^whsec_(sandbox|live)_[A-Za-z0-9_-]+$`)

type EnvDeliveryResult struct {
	EnvFile          string `json:"envFile"`
	Variable         string `json:"variable"`
	Created          bool   `json:"created"`
	Updated          bool   `json:"updated"`
	GitIgnorePath    string `json:"gitIgnorePath"`
	GitIgnoreUpdated bool   `json:"gitIgnoreUpdated"`
}

// DeliverAPIKeyToEnvFile copies a stored Payment API Key into a project-local
// dotenv file without returning the secret to the caller. The target is made
// private and receives an exact root-relative .gitignore rule before delivery.
func DeliverAPIKeyToEnvFile(projectRoot, envFile, variable, apiKey string) (EnvDeliveryResult, error) {
	if !paymentSecretPattern.MatchString(apiKey) {
		return EnvDeliveryResult{}, output.Internal("PAYMENT_API_KEY_INVALID", "the stored Payment API Key cannot be delivered", nil)
	}
	return deliverPaymentSecretToEnvFile(projectRoot, envFile, variable, apiKey)
}

// DeliverWebhookSigningSecretToEnvFile copies a stored Webhook signing secret
// into a project-local dotenv file without returning it to the caller.
func DeliverWebhookSigningSecretToEnvFile(projectRoot, envFile, variable, signingSecret string) (EnvDeliveryResult, error) {
	if !webhookSigningSecretPattern.MatchString(signingSecret) {
		return EnvDeliveryResult{}, output.Internal("WEBHOOK_SECRET_INVALID", "the stored Webhook signing secret cannot be delivered", nil)
	}
	return deliverPaymentSecretToEnvFile(projectRoot, envFile, variable, signingSecret)
}

func deliverPaymentSecretToEnvFile(projectRoot, envFile, variable, secret string) (EnvDeliveryResult, error) {
	root, target, relative, err := resolvePaymentEnvTarget(projectRoot, envFile)
	if err != nil {
		return EnvDeliveryResult{}, err
	}
	variable = strings.TrimSpace(variable)
	if !paymentEnvVariablePattern.MatchString(variable) {
		return EnvDeliveryResult{}, output.Validation("PAYMENT_ENV_VARIABLE_INVALID", "Payment environment variable must be a valid dotenv identifier")
	}
	for _, prefix := range []string{"NEXT_PUBLIC_", "VITE_", "REACT_APP_", "NUXT_PUBLIC_", "EXPO_PUBLIC_", "PUBLIC_"} {
		if strings.HasPrefix(variable, prefix) {
			return EnvDeliveryResult{}, output.Policy("PAYMENT_ENV_VARIABLE_PUBLIC", "Payment secrets cannot use a browser-exposed environment variable name")
		}
	}
	if tracked, err := gitTracksPath(root, relative); err != nil {
		return EnvDeliveryResult{}, output.Internal("PAYMENT_ENV_GIT_CHECK_FAILED", "could not verify whether the Payment environment file is tracked", err)
	} else if tracked {
		return EnvDeliveryResult{}, output.Policy("PAYMENT_ENV_FILE_TRACKED", "the Payment environment file is already tracked by Git").WithHint("remove the file from Git tracking before delivering a Payment secret")
	}

	existing, created, err := readPaymentEnvFile(target)
	if err != nil {
		return EnvDeliveryResult{}, err
	}
	updated, changed, err := renderPaymentEnv(existing, variable, secret)
	if err != nil {
		return EnvDeliveryResult{}, err
	}
	ignorePath, ignoreUpdated, err := ensurePaymentEnvIgnored(root, relative)
	if err != nil {
		return EnvDeliveryResult{}, err
	}
	if changed || created {
		if err := writePaymentFileAtomic(target, updated, 0o600, true); err != nil {
			return EnvDeliveryResult{}, output.Internal("PAYMENT_ENV_DELIVERY_FAILED", "could not write the Payment secret to the project environment file", err)
		}
	} else if err := config.ProtectPrivateFile(target); err != nil {
		return EnvDeliveryResult{}, output.Internal("PAYMENT_ENV_DELIVERY_FAILED", "could not protect the Payment environment file", err)
	}
	return EnvDeliveryResult{
		EnvFile:          target,
		Variable:         variable,
		Created:          created,
		Updated:          changed,
		GitIgnorePath:    ignorePath,
		GitIgnoreUpdated: ignoreUpdated,
	}, nil
}

func resolvePaymentEnvTarget(projectRoot, envFile string) (string, string, string, error) {
	root, err := filepath.Abs(strings.TrimSpace(projectRoot))
	if err != nil {
		return "", "", "", output.Validation("PAYMENT_ENV_FILE_INVALID", "the Payment project directory is invalid")
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", "", output.Validation("PAYMENT_ENV_FILE_INVALID", "the Payment project directory cannot be resolved")
	}
	raw := strings.TrimSpace(envFile)
	if raw == "" || filepath.IsAbs(raw) {
		return "", "", "", output.Validation("PAYMENT_ENV_FILE_INVALID", "--env-file must be a project-relative .env file")
	}
	relative := filepath.Clean(raw)
	if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", "", output.Validation("PAYMENT_ENV_FILE_INVALID", "--env-file must stay inside the Payment project")
	}
	base := filepath.Base(relative)
	if base != ".env" && !strings.HasPrefix(base, ".env.") {
		return "", "", "", output.Validation("PAYMENT_ENV_FILE_INVALID", "--env-file basename must be .env or start with .env.")
	}
	lowerBase := strings.ToLower(base)
	if strings.Contains(lowerBase, "example") || strings.Contains(lowerBase, "sample") || strings.Contains(lowerBase, "template") {
		return "", "", "", output.Policy("PAYMENT_ENV_FILE_UNSAFE", "Payment secrets cannot be delivered to example or template environment files")
	}
	parent, err := filepath.EvalSymlinks(filepath.Join(root, filepath.Dir(relative)))
	if err != nil {
		return "", "", "", output.Validation("PAYMENT_ENV_FILE_INVALID", "the Payment environment file parent directory must already exist")
	}
	within, err := filepath.Rel(root, parent)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return "", "", "", output.Validation("PAYMENT_ENV_FILE_INVALID", "--env-file must stay inside the Payment project")
	}
	target := filepath.Join(parent, base)
	if info, err := os.Lstat(target); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return "", "", "", output.Policy("PAYMENT_ENV_FILE_UNSAFE", "the Payment environment target must be a regular file, not a symlink")
		}
	} else if !os.IsNotExist(err) {
		return "", "", "", output.Validation("PAYMENT_ENV_FILE_INVALID", "the Payment environment target cannot be inspected")
	}
	resolvedRelative, err := filepath.Rel(root, target)
	if err != nil {
		return "", "", "", output.Validation("PAYMENT_ENV_FILE_INVALID", "the Payment environment target cannot be resolved")
	}
	return root, target, resolvedRelative, nil
}

func gitTracksPath(root, relative string) (bool, error) {
	if !hasGitMetadata(root) {
		return false, nil
	}
	_, err := exec.LookPath("git")
	if err != nil {
		return false, err
	}
	command := exec.Command("git", "-C", root, "ls-files", "--error-unmatch", "--", filepath.ToSlash(relative))
	command.Stdout = nil
	command.Stderr = nil
	err = command.Run()
	if err == nil {
		return true, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		if exitError.ExitCode() == 1 {
			return false, nil
		}
	}
	return false, err
}

func hasGitMetadata(root string) bool {
	current := root
	for {
		if _, err := os.Lstat(filepath.Join(current, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false
		}
		current = parent
	}
}

func ensurePaymentEnvIgnored(root, relative string) (string, bool, error) {
	filename := filepath.Join(root, ".gitignore")
	mode := os.FileMode(0o644)
	if info, err := os.Lstat(filename); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return "", false, output.Policy("PAYMENT_GITIGNORE_UNSAFE", "the project .gitignore must be a regular file, not a symlink")
		}
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return "", false, output.Internal("PAYMENT_GITIGNORE_UPDATE_FAILED", "could not inspect the project .gitignore", err)
	}
	data, err := os.ReadFile(filename)
	if err != nil && !os.IsNotExist(err) {
		return "", false, output.Internal("PAYMENT_GITIGNORE_UPDATE_FAILED", "could not read the project .gitignore", err)
	}
	if len(data) > maxPaymentEnvFileBytes {
		return "", false, output.Policy("PAYMENT_GITIGNORE_UNSAFE", "the project .gitignore is too large to update safely")
	}
	rule := "/" + filepath.ToSlash(relative)
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == rule {
			return filename, false, nil
		}
	}
	updated := append([]byte(nil), data...)
	if len(updated) > 0 && updated[len(updated)-1] != '\n' {
		updated = append(updated, '\n')
	}
	updated = append(updated, []byte(rule+"\n")...)
	if err := writePaymentFileAtomic(filename, updated, mode, false); err != nil {
		return "", false, output.Internal("PAYMENT_GITIGNORE_UPDATE_FAILED", "could not add the Payment environment file to .gitignore", err)
	}
	return filename, true, nil
}

func readPaymentEnvFile(filename string) ([]byte, bool, error) {
	data, err := os.ReadFile(filename)
	if os.IsNotExist(err) {
		return nil, true, nil
	}
	if err != nil {
		return nil, false, output.Internal("PAYMENT_ENV_DELIVERY_FAILED", "could not read the Payment environment file", err)
	}
	if len(data) > maxPaymentEnvFileBytes {
		return nil, false, output.Policy("PAYMENT_ENV_FILE_UNSAFE", "the Payment environment file is too large to update safely")
	}
	return data, false, nil
}

func renderPaymentEnv(existing []byte, variable, secret string) ([]byte, bool, error) {
	if bytes.IndexByte(existing, 0) >= 0 {
		return nil, false, output.Policy("PAYMENT_ENV_FILE_UNSAFE", "the Payment environment file contains binary data")
	}
	newline := "\n"
	if bytes.Contains(existing, []byte("\r\n")) {
		newline = "\r\n"
	}
	normalized := strings.ReplaceAll(string(existing), "\r\n", "\n")
	hadFinalNewline := strings.HasSuffix(normalized, "\n")
	body := strings.TrimSuffix(normalized, "\n")
	lines := []string{}
	if body != "" {
		lines = strings.Split(body, "\n")
	}
	matches := 0
	for index, line := range lines {
		if dotenvAssignmentName(line) != variable {
			continue
		}
		matches++
		lines[index] = variable + "=" + secret
	}
	if matches > 1 {
		return nil, false, output.Policy("PAYMENT_ENV_VARIABLE_DUPLICATE", fmt.Sprintf("the Payment environment file defines %s more than once", variable))
	}
	if matches == 0 {
		lines = append(lines, variable+"="+secret)
		hadFinalNewline = true
	}
	rendered := strings.Join(lines, newline)
	if hadFinalNewline || rendered != "" {
		rendered += newline
	}
	return []byte(rendered), rendered != string(existing), nil
}

func dotenvAssignmentName(line string) string {
	value := strings.TrimSpace(line)
	if strings.HasPrefix(value, "export") && len(value) > len("export") && (value[len("export")] == ' ' || value[len("export")] == '\t') {
		value = strings.TrimSpace(value[len("export"):])
	}
	separator := strings.IndexByte(value, '=')
	if separator < 0 {
		return ""
	}
	name := strings.TrimSpace(value[:separator])
	if !paymentEnvVariablePattern.MatchString(name) {
		return ""
	}
	return name
}

func writePaymentFileAtomic(filename string, data []byte, mode os.FileMode, private bool) error {
	temporary, err := os.CreateTemp(filepath.Dir(filename), ".viceme-payment-env-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if private {
		if err := config.ProtectPrivateFile(temporaryName); err != nil {
			return err
		}
	}
	if err := os.Rename(temporaryName, filename); err != nil {
		return err
	}
	if private {
		return config.ProtectPrivateFile(filename)
	}
	return nil
}
