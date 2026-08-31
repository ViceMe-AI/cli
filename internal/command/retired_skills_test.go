package command

import "testing"

func TestRetiredEngagementSkillCatalogCoversCommittedManifestIdentities(t *testing.T) {
	t.Parallel()

	requiredVersions := map[string]map[string]bool{
		"viceme-danmaku": {"0.12.4": false, "0.14.3": false, "0.19.0-beta.0": false},
		"viceme-tip":     {"0.15.1": false, "0.16.1": false, "0.19.0-beta.0": false, "0.19.0": false},
	}
	requiredPOCDigests := map[string]bool{
		"sha256:4535c3bca44af35e0790fc0540679474c62b187aa2f7588db9bd0f4b9956a79f": false,
		"sha256:0f7d703da8df031f734739012c7776df0081c00ba002483b87749dd3510ae70c": false,
		"sha256:22ed6dc74c886ca35968df635aa8044311d2f6f2bf448a20ad21851bcf3ee154": false,
		"sha256:46b31aa02779533fa1cc92f7b6c8875421d222ed887d331fc0b58fde0374a237": false,
		"sha256:4e2ddccf72062191eb3e9d24133cb4eacc68414a41f1591bd0749adc6c3e02de": false,
		"sha256:b34806134a95b895038dc50fcf8d39239ef0386d5cc936bbc290afe1d9474085": false,
	}
	identities := make(map[string]bool, len(retiredOfficialSkills))
	for _, identity := range retiredOfficialSkills {
		versions, knownName := requiredVersions[identity.Name]
		if !knownName {
			t.Fatalf("unexpected retired official Skill: %s", identity.Name)
		}
		if _, required := versions[identity.SkillVersion]; required {
			versions[identity.SkillVersion] = true
		}
		if _, required := requiredPOCDigests[identity.FullBundleDigest]; required {
			requiredPOCDigests[identity.FullBundleDigest] = true
		}
		key := identity.Name + "\x00" + identity.SkillVersion + "\x00" + identity.FullBundleDigest
		if identities[key] {
			t.Fatalf("duplicate retired Skill identity: %#v", identity)
		}
		identities[key] = true
	}
	if len(retiredOfficialSkills) != 74 {
		t.Fatalf("retired Skill catalog does not cover every committed manifest identity: %d", len(retiredOfficialSkills))
	}
	for name, versions := range requiredVersions {
		for version, included := range versions {
			if !included {
				t.Fatalf("retired Skill catalog omitted %s %s", name, version)
			}
		}
	}
	for digest, included := range requiredPOCDigests {
		if !included {
			t.Fatalf("retired Skill catalog omitted public POC identity %s", digest)
		}
	}
}
