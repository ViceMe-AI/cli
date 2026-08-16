package cli

import (
	"embed"
	"io/fs"
)

// embeddedSkills contains the agent-readable documents, code blueprints, and
// install metadata. Keeping them in the binary lets doctor compare an installed
// Skill with the exact CLI release that is invoking it.
//
//go:embed skills/*/SKILL.md skills/*/skill-package.json skills/*/agents/*.yaml skills/*/references/*.md skills/*/assets/react-tailwind/* skills/*/assets/cdn/*
var embeddedSkills embed.FS

// EmbeddedSkills returns an FS rooted at skills/.
func EmbeddedSkills() fs.FS {
	sub, err := fs.Sub(embeddedSkills, "skills")
	if err != nil {
		panic(err)
	}
	return sub
}
