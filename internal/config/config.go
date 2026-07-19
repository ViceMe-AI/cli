package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type Region string

const (
	RegionCN     Region = "cn"
	RegionGlobal Region = "global"
)

type Config struct {
	Region Region `json:"region"`
}

type EnsureResult struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

func ParseRegion(raw string) (Region, error) {
	region := Region(strings.ToLower(strings.TrimSpace(raw)))
	switch region {
	case RegionCN, RegionGlobal:
		return region, nil
	default:
		return "", fmt.Errorf("region must be cn or global")
	}
}

func APIBaseURL(region Region) string {
	if region == RegionGlobal {
		return "https://api.viceme.ai"
	}
	return "https://api.viceme.cn"
}

func LoadOrDefault(configBase string) (Config, error) {
	config, err := load(configPath(configBase))
	if errors.Is(err, fs.ErrNotExist) {
		return Config{Region: RegionCN}, nil
	}
	return config, err
}

func Ensure(configBase string, desired Config) (EnsureResult, error) {
	region, err := ParseRegion(string(desired.Region))
	if err != nil {
		return EnsureResult{}, err
	}
	desired.Region = region
	filename := configPath(configBase)
	current, err := load(filename)
	if err == nil && current.Region == desired.Region && isCanonical(filename, desired) {
		return EnsureResult{Path: filename, Status: "unchanged"}, nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return EnsureResult{Path: filename, Status: "invalid"}, fmt.Errorf("existing config is invalid; refusing to overwrite %s: %w", filename, err)
	}
	status := "created"
	if err == nil {
		status = "updated"
	}
	if err := write(filename, desired); err != nil {
		return EnsureResult{Path: filename, Status: "failed"}, err
	}
	return EnsureResult{Path: filename, Status: status}, nil
}

func load(filename string) (Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return Config{}, err
	}
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if config.Region == "" {
		config.Region = RegionCN
	} else {
		region, err := ParseRegion(string(config.Region))
		if err != nil {
			return Config{}, err
		}
		config.Region = region
	}
	return config, nil
}

func isCanonical(filename string, config Config) bool {
	actual, err := os.ReadFile(filename)
	if err != nil {
		return false
	}
	expected, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return false
	}
	expected = append(expected, '\n')
	return bytes.Equal(actual, expected)
}

func write(filename string, config Config) error {
	directory := filepath.Dir(filename)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(directory, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create config staging file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure config staging file: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write config staging file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close config staging file: %w", err)
	}
	if err := os.Rename(temporaryName, filename); err != nil {
		return fmt.Errorf("activate config: %w", err)
	}
	return nil
}

func configPath(configBase string) string {
	return filepath.Join(configBase, "viceme", "config.json")
}
