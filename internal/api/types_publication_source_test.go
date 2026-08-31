package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSkillPublicationSourceMarshalsGitHubRootWithPathNull(t *testing.T) {
	encoded, err := json.Marshal(SkillPublicationSource{Type: "GITHUB", Entry: "SKILL.md"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"path":null`) {
		t.Fatalf("GitHub root source must serialize an explicit path null, got %s", encoded)
	}
}

func TestSkillPublicationSourceMarshalsGitHubSubdirectoryPath(t *testing.T) {
	path := "skills/cover"
	encoded, err := json.Marshal(SkillPublicationSource{Type: "GITHUB", Entry: "SKILL.md", Path: &path})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"path":"skills/cover"`) {
		t.Fatalf("GitHub subdirectory source must serialize its path, got %s", encoded)
	}
}

func TestSkillPublicationSourceOmitsPathForOtherKinds(t *testing.T) {
	encoded, err := json.Marshal(SkillPublicationSource{Type: "WORKSPACE", Entry: "SKILL.md"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "path") {
		t.Fatalf("non-GitHub sources must not carry a path key, got %s", encoded)
	}
}
