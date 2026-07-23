package buildinfo

import (
	"fmt"

	"github.com/ViceMe-AI/cli/internal/semver"
)

var (
	// Version is replaced with -ldflags for release builds.
	Version = "dev"
	Commit  = "unknown"
)

const (
	// ReleaseVersion is the source-tree and npm package version. Development
	// builds still report Version=dev, but use ReleaseVersion for compatibility
	// evaluation.
	ReleaseVersion = "0.8.0"
	// SkillVersion is versioned independently so compatibility drift can be
	// diagnosed even when the binary and Skill happen to ship together.
	SkillVersion      = "0.8.0"
	MinimumCLIVersion = "0.8.0"
	CLICompatibility  = ">=0.8.0 <0.9.0"
)

type Info struct {
	Version       string `json:"version"`
	Commit        string `json:"commit"`
	SkillVersion  string `json:"skill_version"`
	Compatibility string `json:"cli_compatibility"`
}

func Current() Info {
	return Info{
		Version:       Version,
		Commit:        Commit,
		SkillVersion:  SkillVersion,
		Compatibility: CLICompatibility,
	}
}

func CompatibilityVersion() string {
	if Version == "dev" {
		return ReleaseVersion
	}
	return Version
}

func ValidateNPMLaunch(installMethod, packageVersion, binaryVersion string) error {
	if installMethod != "npm" {
		return nil
	}
	if _, err := semver.Parse(packageVersion); err != nil {
		return fmt.Errorf("npm launcher package version is missing or invalid: %w", err)
	}
	if _, err := semver.Parse(binaryVersion); err != nil {
		return fmt.Errorf("Go binary version is invalid for an npm launch: %w", err)
	}
	if packageVersion != binaryVersion {
		return fmt.Errorf("npm package version %s does not match Go binary version %s", packageVersion, binaryVersion)
	}
	return nil
}
