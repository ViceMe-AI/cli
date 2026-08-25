package command

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"

	cliembed "github.com/ViceMe-AI/cli"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	updatepkg "github.com/ViceMe-AI/cli/internal/update"
)

type embeddedSkillRelease struct {
	SkillVersion          string `json:"skill_version"`
	MinimumCLIVersion     string `json:"minimum_cli_version"`
	CLICompatibility      string `json:"cli_compatibility"`
	FullBundleDigest      string `json:"full_skill_bundle_digest"`
	EmbeddedContentDigest string `json:"embedded_content_digest"`
}

type embeddedReleaseManifest struct {
	SchemaVersion           int                             `json:"schema_version"`
	NPMPackage              string                          `json:"npm_package"`
	CLIVersion              string                          `json:"cli_version"`
	Skills                  map[string]embeddedSkillRelease `json:"skills"`
	BootstrapContractDigest string                          `json:"bootstrap_contract_digest"`
	InstallerDigests        map[string]string               `json:"installer_digests"`
}

func officialSkillsForRelease(bundle *skillcontent.Bundle, target updatepkg.ActiveGeneration) ([]string, error) {
	if bundle == nil {
		return nil, errors.New("the running release has no embedded Skill bundle")
	}
	data := cliembed.EmbeddedReleaseManifest()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest embeddedReleaseManifest
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decode embedded release manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("embedded release manifest contains trailing JSON")
	}
	if manifest.SchemaVersion != 2 || manifest.NPMPackage != updatepkg.PackageName || manifest.CLIVersion != target.Version || len(manifest.Skills) == 0 {
		return nil, errors.New("embedded release manifest does not identify the running CLI generation")
	}
	bundled, err := bundle.List()
	if err != nil {
		return nil, err
	}
	if len(bundled) != len(manifest.Skills) {
		return nil, errors.New("embedded release manifest and Skill bundle contain different official Skill sets")
	}
	names := make([]string, 0, len(manifest.Skills))
	for _, info := range bundled {
		release, exists := manifest.Skills[info.Name]
		if !exists {
			return nil, fmt.Errorf("embedded release manifest omits official Skill %s", info.Name)
		}
		metadata, err := bundle.Package(info.Name)
		if err != nil {
			return nil, err
		}
		digests, err := bundle.Digests(info.Name)
		if err != nil {
			return nil, err
		}
		if release.SkillVersion != metadata.SkillVersion ||
			release.MinimumCLIVersion != metadata.MinimumCLIVersion ||
			release.CLICompatibility != metadata.CLICompatibility ||
			release.FullBundleDigest != digests.Full ||
			release.EmbeddedContentDigest != digests.Embedded {
			return nil, fmt.Errorf("embedded release manifest does not match official Skill %s", info.Name)
		}
		names = append(names, info.Name)
	}
	sort.Strings(names)
	return names, nil
}
