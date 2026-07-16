package cli

import (
	"embed"
	"io/fs"
)

// embeddedSkills contains both the agent-readable documents and the small
// install bundle. Keeping both in the binary lets doctor compare an installed
// Skill with the exact CLI release that is invoking it.
//
//go:embed skills/*/SKILL.md skills/*/agents/*.yaml skills/*/references/*.md
var embeddedSkills embed.FS

// EmbeddedSkills returns an FS rooted at skills/.
func EmbeddedSkills() fs.FS {
	sub, err := fs.Sub(embeddedSkills, "skills")
	if err != nil {
		panic(err)
	}
	return sub
}
