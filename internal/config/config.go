package config

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/ViceMe-AI/cli/internal/privatefile"
)

type Region string

const (
	RegionCN           Region = "cn"
	RegionGlobal       Region = "global"
	DefaultProfileName        = "default"
)

type Profile struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	APIBaseURL   string `json:"apiBaseUrl"`
	WebBaseURL   string `json:"webBaseUrl,omitempty"`
	MarketRegion Region `json:"marketRegion,omitempty"`
	UserID       string `json:"userId,omitempty"`
}

type Config struct {
	DistributionRegion Region    `json:"distributionRegion"`
	CurrentProfile     string    `json:"currentProfile"`
	PreviousProfile    string    `json:"previousProfile,omitempty"`
	Profiles           []Profile `json:"profiles"`
}

type EnsureResult struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

type LoadError struct {
	Path  string
	Stage string
	Err   error
}

func (err *LoadError) Error() string {
	return fmt.Sprintf("%s config: %v", err.Stage, err.Err)
}

func (err *LoadError) Unwrap() error {
	return err.Err
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

func WebBaseURL(region Region) string {
	if region == RegionGlobal {
		return "https://viceme.ai"
	}
	return "https://viceme.cn"
}

func StableReleaseBaseURL(region Region) string {
	if region == RegionGlobal {
		return "https://s3.viceme.ai/start/cli/releases"
	}
	return "https://s3.viceme.cn/start/cli/releases"
}

// AgentInstallDocURL is the agent-facing CLI installation contract injected
// into trial gates. It mirrors the INSTALL_DOC_URL the Shop markdown
// passphrase template already hands to agents.
func AgentInstallDocURL(region Region) string {
	if region == RegionGlobal {
		return "https://s3.viceme.ai/start/agent-install.md"
	}
	return "https://s3.viceme.cn/start/agent-install.md"
}

// NormalizeAPIBaseURL returns the canonical persisted endpoint for a profile.
// Remote endpoints must use HTTPS; loopback HTTP is intentionally supported
// for local Shop development only.
func NormalizeAPIBaseURL(raw string) (string, error) {
	return normalizeBaseURL(raw, "API base URL")
}

func NormalizeWebBaseURL(raw string) (string, error) {
	return normalizeBaseURL(raw, "Web base URL")
}

func NormalizeReleaseBaseURL(raw string) (string, error) {
	return normalizeBaseURL(raw, "release base URL")
}

func normalizeBaseURL(raw, label string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("%s cannot be empty", label)
	}
	if len(value) > 2048 {
		return "", fmt.Errorf("%s is too long", label)
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return "", fmt.Errorf("invalid %s", label)
	}
	scheme := strings.ToLower(parsed.Scheme)
	hostname := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if hostname == "" {
		return "", fmt.Errorf("invalid %s", label)
	}
	switch scheme {
	case "https":
	case "http":
		address := net.ParseIP(hostname)
		if hostname != "localhost" && (address == nil || !address.IsLoopback()) {
			return "", fmt.Errorf("%s must use HTTPS; HTTP is allowed only for localhost or loopback development", label)
		}
	default:
		return "", fmt.Errorf("%s must use HTTPS; HTTP is allowed only for localhost or loopback development", label)
	}
	for _, segment := range strings.Split(parsed.Path, "/") {
		if segment == "." || segment == ".." {
			return "", fmt.Errorf("%s path cannot contain dot segments", label)
		}
	}
	if strings.Contains(parsed.Path, `\`) {
		return "", fmt.Errorf("%s path cannot contain backslashes", label)
	}
	port := parsed.Port()
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	host := hostname
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port != "" {
		host += ":" + port
	}
	basePath := path.Clean(parsed.Path)
	if basePath == "." || basePath == "/" {
		basePath = ""
	}
	return (&url.URL{Scheme: scheme, Host: host, Path: basePath}).String(), nil
}

func (profile Profile) ResolvedAPIBaseURL() string {
	return profile.APIBaseURL
}

func (profile Profile) ResolvedWebBaseURL() string {
	if profile.WebBaseURL != "" {
		return profile.WebBaseURL
	}
	if profile.APIBaseURL == APIBaseURL(RegionGlobal) {
		return WebBaseURL(RegionGlobal)
	}
	if profile.APIBaseURL == APIBaseURL(RegionCN) {
		return WebBaseURL(RegionCN)
	}
	return ""
}

func (profile Profile) ResolvedMarketRegion() (Region, error) {
	return ParseRegion(string(profile.MarketRegion))
}

func Default(region Region) Config {
	if parsed, err := ParseRegion(string(region)); err == nil {
		region = parsed
	} else {
		region = RegionCN
	}
	return Config{
		DistributionRegion: region,
		CurrentProfile:     DefaultProfileName,
		Profiles: []Profile{{
			ID:           DefaultProfileName,
			Name:         DefaultProfileName,
			APIBaseURL:   APIBaseURL(region),
			WebBaseURL:   WebBaseURL(region),
			MarketRegion: region,
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

func (config *Config) AddProfile(name, apiBaseURL, webBaseURL string, marketRegion Region) (*Profile, error) {
	if err := ValidateProfileName(name); err != nil {
		return nil, err
	}
	if config.FindProfileIndex(name) >= 0 {
		return nil, fmt.Errorf("profile %q already exists", name)
	}
	profile := Profile{
		Name:         name,
		APIBaseURL:   apiBaseURL,
		WebBaseURL:   webBaseURL,
		MarketRegion: marketRegion,
	}
	if err := normalizeProfileAuthority(&profile, true); err != nil {
		return nil, err
	}
	id, err := newProfileID()
	if err != nil {
		return nil, fmt.Errorf("create profile id: %w", err)
	}
	profile.ID = id
	config.Profiles = append(config.Profiles, profile)
	return &config.Profiles[len(config.Profiles)-1], nil
}

func (config *Config) SetProfileAuthority(name, apiBaseURL, webBaseURL string, marketRegion Region) error {
	index := config.FindProfileIndex(name)
	if index < 0 {
		return fmt.Errorf("profile %q not found", name)
	}
	candidate := config.Profiles[index]
	candidate.APIBaseURL = apiBaseURL
	candidate.WebBaseURL = webBaseURL
	candidate.MarketRegion = marketRegion
	if err := normalizeProfileAuthority(&candidate, true); err != nil {
		return err
	}
	config.Profiles[index] = candidate
	return nil
}

func ValidateProfileAuthority(profile Profile) error {
	return normalizeProfileAuthority(&profile, true)
}

func normalizeProfileAuthority(profile *Profile, requireComplete bool) error {
	normalizedAPIBaseURL, err := NormalizeAPIBaseURL(profile.APIBaseURL)
	if err != nil {
		return err
	}
	profile.APIBaseURL = normalizedAPIBaseURL

	apiRegion := officialAPIRegion(profile.APIBaseURL)
	if apiRegion != "" {
		if profile.APIBaseURL != APIBaseURL(apiRegion) {
			return fmt.Errorf(
				"official %s API profiles must use API base URL %s",
				strings.ToUpper(string(apiRegion)), APIBaseURL(apiRegion),
			)
		}
		if strings.TrimSpace(profile.WebBaseURL) == "" {
			profile.WebBaseURL = WebBaseURL(apiRegion)
		}
		if profile.MarketRegion == "" {
			profile.MarketRegion = apiRegion
		}
	}

	if strings.TrimSpace(profile.WebBaseURL) != "" {
		normalizedWebBaseURL, err := NormalizeWebBaseURL(profile.WebBaseURL)
		if err != nil {
			return err
		}
		profile.WebBaseURL = normalizedWebBaseURL
	} else {
		profile.WebBaseURL = ""
	}
	if profile.MarketRegion != "" {
		marketRegion, err := ParseRegion(string(profile.MarketRegion))
		if err != nil {
			return fmt.Errorf("market: %w", err)
		}
		profile.MarketRegion = marketRegion
	}

	if apiRegion != "" {
		if profile.WebBaseURL != WebBaseURL(apiRegion) || profile.MarketRegion != apiRegion {
			return fmt.Errorf(
				"official %s API profiles must use Web base URL %s and market region %s",
				strings.ToUpper(string(apiRegion)), WebBaseURL(apiRegion), strings.ToUpper(string(apiRegion)),
			)
		}
		return nil
	}
	if webRegion := officialWebRegion(profile.WebBaseURL); webRegion != "" {
		return fmt.Errorf(
			"custom API profiles cannot use official %s Web base URL %s",
			strings.ToUpper(string(webRegion)), WebBaseURL(webRegion),
		)
	}
	if requireComplete && profile.WebBaseURL == "" {
		return fmt.Errorf("custom API profiles require an explicit Web base URL")
	}
	if requireComplete && profile.MarketRegion == "" {
		return fmt.Errorf("custom API profiles require an explicit market region")
	}
	if profile.WebBaseURL != "" && profile.MarketRegion == "" {
		return fmt.Errorf("custom API profiles with a Web base URL require an explicit market region")
	}
	return nil
}

func officialAPIRegion(apiBaseURL string) Region {
	return officialURLRegion(apiBaseURL, APIBaseURL)
}

func officialWebRegion(webBaseURL string) Region {
	return officialURLRegion(webBaseURL, WebBaseURL)
}

func officialURLRegion(value string, baseURL func(Region) string) Region {
	candidate, err := url.Parse(value)
	if err != nil {
		return ""
	}
	for _, region := range []Region{RegionCN, RegionGlobal} {
		official, err := url.Parse(baseURL(region))
		if err == nil && candidate.Scheme == official.Scheme && candidate.Hostname() == official.Hostname() {
			return region
		}
	}
	return ""
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
		return Config{}, configLoadError(filename, "read", err)
	}
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return Config{}, configLoadError(filename, "decode", err)
	}
	// Migrate the pre-endpoint-authority shape in memory. Region used to be
	// duplicated on every Profile; it now selects only the distribution channel.
	if config.DistributionRegion == "" {
		var legacy struct {
			Profiles []struct {
				Name       string `json:"name"`
				Region     Region `json:"region"`
				APIBaseURL string `json:"apiBaseUrl"`
			} `json:"profiles"`
		}
		if err := json.Unmarshal(data, &legacy); err != nil {
			return Config{}, configLoadError(filename, "decode", err)
		}
		config.DistributionRegion = RegionCN
		for _, candidate := range legacy.Profiles {
			if candidate.Name == config.CurrentProfile && candidate.Region != "" {
				config.DistributionRegion = candidate.Region
			}
			for index := range config.Profiles {
				if config.Profiles[index].Name == candidate.Name && config.Profiles[index].APIBaseURL == "" {
					config.Profiles[index].APIBaseURL = APIBaseURL(candidate.Region)
				}
				if config.Profiles[index].Name == candidate.Name && config.Profiles[index].MarketRegion == "" {
					config.Profiles[index].MarketRegion = candidate.Region
				}
			}
		}
	}
	for index := range config.Profiles {
		profile := &config.Profiles[index]
		for _, region := range []Region{RegionCN, RegionGlobal} {
			if profile.APIBaseURL != APIBaseURL(region) {
				continue
			}
			if profile.WebBaseURL == "" {
				profile.WebBaseURL = WebBaseURL(region)
			}
			if profile.MarketRegion == "" {
				profile.MarketRegion = region
			}
			break
		}
	}
	if err := validate(&config); err != nil {
		return Config{}, configLoadError(filename, "validate", err)
	}
	if err := requirePrivateFile(filename); err != nil {
		return Config{}, configLoadError(filename, "permissions", err)
	}
	return config, nil
}

func configLoadError(path, stage string, err error) error {
	return &LoadError{Path: path, Stage: stage, Err: err}
}

func validate(config *Config) error {
	region, err := ParseRegion(string(config.DistributionRegion))
	if err != nil {
		return fmt.Errorf("distribution: %w", err)
	}
	config.DistributionRegion = region
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
		if err := normalizeProfileAuthority(profile, false); err != nil {
			return fmt.Errorf("profile %q: %w", profile.Name, err)
		}
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
	if err := requirePrivateFile(filename); err != nil {
		return false
	}
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
	if err := privatefile.Write(filename, data, ".config-*.tmp"); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	// securePrivateFile applies the Windows ACL hardening to the final file;
	// the staged rename path already carries Unix 0600 permissions.
	if err := securePrivateFile(filename); err != nil {
		return fmt.Errorf("secure config: %w", err)
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
