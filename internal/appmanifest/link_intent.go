package appmanifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const linkIntentSchemaVersion = 1

type LinkIntent struct {
	SchemaVersion   int    `json:"schemaVersion"`
	ClientRequestID string `json:"clientRequestId"`
	Name            string `json:"name"`
	HostingMode     string `json:"hostingMode"`
}

func LinkIntentPath(directory string) string {
	return filepath.Join(directory, ".viceme", "app-link-pending.json")
}

func LoadOrCreateLinkIntent(directory, name, hostingMode string, newID func() string) (LinkIntent, error) {
	filename := LinkIntentPath(directory)
	data, err := os.ReadFile(filename)
	if err == nil {
		intent, decodeErr := decodeLinkIntent(data)
		if decodeErr != nil {
			return LinkIntent{}, decodeErr
		}
		if intent.Name != name || intent.HostingMode != hostingMode {
			return LinkIntent{}, errors.New("a pending App creation uses different input; retry the original command or remove .viceme/app-link-pending.json after verifying the remote result")
		}
		return intent, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return LinkIntent{}, fmt.Errorf("read pending App link intent: %w", err)
	}
	if newID == nil {
		return LinkIntent{}, errors.New("create pending App link intent: request ID generator is unavailable")
	}
	intent := LinkIntent{
		SchemaVersion:   linkIntentSchemaVersion,
		ClientRequestID: newID(),
		Name:            name,
		HostingMode:     hostingMode,
	}
	if err := validateLinkIntent(intent); err != nil {
		return LinkIntent{}, err
	}
	if err := writeLinkIntent(filename, intent); err != nil {
		return LinkIntent{}, err
	}
	return intent, nil
}

func RemoveLinkIntent(directory string) error {
	err := os.Remove(LinkIntentPath(directory))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func decodeLinkIntent(data []byte) (LinkIntent, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var intent LinkIntent
	if err := decoder.Decode(&intent); err != nil {
		return LinkIntent{}, fmt.Errorf("decode pending App link intent: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return LinkIntent{}, errors.New("decode pending App link intent: trailing JSON data")
	}
	if err := validateLinkIntent(intent); err != nil {
		return LinkIntent{}, err
	}
	return intent, nil
}

func validateLinkIntent(intent LinkIntent) error {
	if intent.SchemaVersion != linkIntentSchemaVersion ||
		!uuidPattern.MatchString(strings.ToLower(intent.ClientRequestID)) ||
		strings.TrimSpace(intent.Name) == "" || utf8.RuneCountInString(intent.Name) > 100 ||
		intent.HostingMode != "EXTERNAL" {
		return errors.New("pending App link intent is invalid")
	}
	return nil
}

func writeLinkIntent(filename string, intent LinkIntent) error {
	directory := filepath.Dir(filename)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create App metadata directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".app-link-pending-*.json")
	if err != nil {
		return fmt.Errorf("create pending App link intent: %w", err)
	}
	temporaryName := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("protect pending App link intent: %w", err)
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(intent); err != nil {
		return fmt.Errorf("encode pending App link intent: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync pending App link intent: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close pending App link intent: %w", err)
	}
	if err := os.Rename(temporaryName, filename); err != nil {
		return fmt.Errorf("replace pending App link intent: %w", err)
	}
	committed = true
	return nil
}
