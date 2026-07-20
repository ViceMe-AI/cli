package config

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type Region string

const (
	RegionCN           Region = "cn"
	RegionGlobal       Region = "global"
	DefaultProfileName        = "default"
)

type Profile struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Region Region `json:"region"`
	UserID string `json:"userId,omitempty"`
}

type Config struct {
	CurrentProfile  string    `json:"currentProfile"`
	PreviousProfile string    `json:"previousProfile,omitempty"`
	Profiles        []Profile `json:"profiles"`
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

func Default(region Region) Config {
	if parsed, err := ParseRegion(string(region)); err == nil {
		region = parsed
	} else {
		region = RegionCN
	}
	return Config{
		CurrentProfile: DefaultProfileName,
		Profiles: []Profile{{
			ID:     DefaultProfileName,
			Name:   DefaultProfileName,
			Region: region,
		}},
	}
}

// LoadOrDefault loads ~/.viceme-cli/config.json or returns the default profile
// when the file does not exist.
func LoadOrDefault(configBase string) (Config, error) {
	config, err := load(ConfigPath(configBase))
	if err == nil {
		return config, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return Config{}, err
	}
	return Default(RegionCN), nil
}

func Save(configBase string, config Config) (EnsureResult, error) {
	if err := validate(&config); err != nil {
		return EnsureResult{Path: ConfigPath(configBase), Status: "invalid"}, err
	}
	filename := ConfigPath(configBase)
	status := "created"
	if _, err := os.Stat(filename); err == nil {
		status = "updated"
	} else if !errors.Is(err, fs.ErrNotExist) {
		return EnsureResult{Path: filename, Status: "failed"}, err
	}
	if isCanonical(filename, config) {
		return EnsureResult{Path: filename, Status: "unchanged"}, nil
	}
	if err := write(filename, config); err != nil {
		return EnsureResult{Path: filename, Status: "failed"}, err
	}
	return EnsureResult{Path: filename, Status: status}, nil
}

func (config *Config) Resolve(profileOverride string) (*Profile, error) {
	name := profileOverride
	if name == "" {
		name = config.CurrentProfile
	}
	for index := range config.Profiles {
		if config.Profiles[index].Name == name {
			return &config.Profiles[index], nil
		}
	}
	return nil, fmt.Errorf("profile %q not found; available profiles: %s", name, strings.Join(config.ProfileNames(), ", "))
}

func (config *Config) FindProfileIndex(name string) int {
	for index := range config.Profiles {
		if config.Profiles[index].Name == name {
			return index
		}
	}
	return -1
}

func (config *Config) ProfileNames() []string {
	names := make([]string, len(config.Profiles))
	for index := range config.Profiles {
		names[index] = config.Profiles[index].Name
	}
	return names
}

func (config *Config) AddProfile(name string, region Region) (*Profile, error) {
	if err := ValidateProfileName(name); err != nil {
		return nil, err
	}
	if config.FindProfileIndex(name) >= 0 {
		return nil, fmt.Errorf("profile %q already exists", name)
	}
	parsedRegion, err := ParseRegion(string(region))
	if err != nil {
		return nil, err
	}
	id, err := newProfileID()
	if err != nil {
		return nil, fmt.Errorf("create profile id: %w", err)
	}
	config.Profiles = append(config.Profiles, Profile{ID: id, Name: name, Region: parsedRegion})
	return &config.Profiles[len(config.Profiles)-1], nil
}

func ValidateProfileName(name string) error {
	if name == "" {
		return fmt.Errorf("profile name cannot be empty")
	}
	if utf8.RuneCountInString(name) > 64 {
		return fmt.Errorf("profile name %q is too long (max 64 characters)", name)
	}
	for _, character := range name {
		if character <= 0x1f || character == 0x7f {
			return fmt.Errorf("profile name %q contains control characters", name)
		}
		switch character {
		case ' ', '\t', '/', '\\', '"', '\'', '`', '$', '#', '!', '&', '|', ';', '(', ')', '{', '}', '[', ']', '<', '>', '?', '*', '~':
			return fmt.Errorf("profile name %q contains invalid character %q", name, character)
		}
	}
	return nil
}

func ConfigPath(configBase string) string {
	return filepath.Join(configBase, "config.json")
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
	if err := validate(&config); err != nil {
		return Config{}, err
	}
	return config, nil
}

func validate(config *Config) error {
	if len(config.Profiles) == 0 {
		return fmt.Errorf("config must contain at least one profile")
	}
	ids := make(map[string]struct{}, len(config.Profiles))
	names := make(map[string]struct{}, len(config.Profiles))
	for index := range config.Profiles {
		profile := &config.Profiles[index]
		if profile.ID == "" {
			return fmt.Errorf("profile %q is missing an id", profile.Name)
		}
		if _, exists := ids[profile.ID]; exists {
			return fmt.Errorf("duplicate profile id %q", profile.ID)
		}
		ids[profile.ID] = struct{}{}
		if err := ValidateProfileName(profile.Name); err != nil {
			return err
		}
		if _, exists := names[profile.Name]; exists {
			return fmt.Errorf("duplicate profile name %q", profile.Name)
		}
		names[profile.Name] = struct{}{}
		region, err := ParseRegion(string(profile.Region))
		if err != nil {
			return fmt.Errorf("profile %q: %w", profile.Name, err)
		}
		profile.Region = region
	}
	if config.CurrentProfile == "" {
		config.CurrentProfile = config.Profiles[0].Name
	}
	if _, exists := names[config.CurrentProfile]; !exists {
		return fmt.Errorf("current profile %q does not exist", config.CurrentProfile)
	}
	if config.PreviousProfile != "" {
		if _, exists := names[config.PreviousProfile]; !exists {
			return fmt.Errorf("previous profile %q does not exist", config.PreviousProfile)
		}
	}
	return nil
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

func newProfileID() (string, error) {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return "profile_" + hex.EncodeToString(value), nil
}
