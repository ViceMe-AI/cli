package cli

import (
	"embed"
	"io/fs"
)

// embeddedSkills contains the agent-readable documents, code blueprints, and
// install metadata. Keeping them in the binary lets doctor compare an installed
// Skill with the exact CLI release that is invoking it.
//
//go:embed skills/*/SKILL.md skills/*/skill-package.json skills/*/agents/*.yaml skills/*/references/*.md skills/*/templates/*
var embeddedSkills embed.FS

//go:embed quality/release-manifest.json
var embeddedReleaseManifest []byte

// EmbeddedSkills returns an FS rooted at skills/.
func EmbeddedSkills() fs.FS {
	sub, err := fs.Sub(embeddedSkills, "skills")
	if err != nil {
		panic(err)
	}
	return sub
}

// EmbeddedReleaseManifest returns the generated manifest compiled into this
// exact CLI binary. External package-manager upgrades must never infer Skill
// contents from a mutable network channel.
func EmbeddedReleaseManifest() []byte {
	return append([]byte(nil), embeddedReleaseManifest...)
}
