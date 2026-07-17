package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const SchemaVersion = 1

type Config struct {
	SchemaVersion  int    `json:"schema_version"`
	DefaultProfile string `json:"default_profile"`
	APIBaseURL     string `json:"api_base_url"`
	UpdateChannel  string `json:"update_channel"`
}

type EnsureResult struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

func Ensure(configBase string, desired Config) (EnsureResult, error) {
	desired.SchemaVersion = SchemaVersion
	if desired.DefaultProfile == "" {
		desired.DefaultProfile = "default"
	}
	if desired.UpdateChannel == "" {
		desired.UpdateChannel = "stable"
	}
	directory := filepath.Join(configBase, "viceme")
	filename := filepath.Join(directory, "config.json")
	existing, err := os.ReadFile(filename)
	if err == nil {
		var current Config
		if err := json.Unmarshal(existing, &current); err != nil || current.SchemaVersion != SchemaVersion {
			return EnsureResult{Path: filename, Status: "invalid"}, fmt.Errorf("existing config is invalid; refusing to overwrite %s", filename)
		}
		return EnsureResult{Path: filename, Status: "unchanged"}, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return EnsureResult{Path: filename, Status: "failed"}, fmt.Errorf("read config: %w", err)
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return EnsureResult{Path: filename, Status: "failed"}, fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(desired, "", "  ")
	if err != nil {
		return EnsureResult{Path: filename, Status: "failed"}, fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(directory, ".config-*.tmp")
	if err != nil {
		return EnsureResult{Path: filename, Status: "failed"}, fmt.Errorf("create config staging file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return EnsureResult{Path: filename, Status: "failed"}, fmt.Errorf("secure config staging file: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return EnsureResult{Path: filename, Status: "failed"}, fmt.Errorf("write config staging file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return EnsureResult{Path: filename, Status: "failed"}, fmt.Errorf("close config staging file: %w", err)
	}
	if err := os.Rename(temporaryName, filename); err != nil {
		return EnsureResult{Path: filename, Status: "failed"}, fmt.Errorf("activate config: %w", err)
	}
	return EnsureResult{Path: filename, Status: "created"}, nil
}
