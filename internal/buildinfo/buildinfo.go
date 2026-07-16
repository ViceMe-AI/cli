package buildinfo

var (
	// Version is replaced with -ldflags for release builds.
	Version = "dev"
	// SkillVersion is versioned independently so compatibility drift can be
	// diagnosed even when the binary and Skill happen to ship together.
	SkillVersion = "0.1.0"
	Commit       = "unknown"
)

const CLICompatibility = ">=0.1.0 <0.2.0"

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
