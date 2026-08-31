package command

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

func TestLegacyRetiredSkillMigrationsMatchAuditedHistory(t *testing.T) {
	t.Parallel()

	expectedCounts := map[string]int{
		"viceme-danmaku": 51,
		"viceme-tip":     24,
	}
	counts := make(map[string]int)
	catalog := make(map[string]bool, len(legacyRetiredOfficialSkillMigrations))
	canonical := make([]string, 0, len(legacyRetiredOfficialSkillMigrations))
	tagCount := 0
	commitCount := 0
	for _, identity := range legacyRetiredOfficialSkillMigrations {
		if err := skillcontent.ValidateLegacyRetiredSkillIdentity(identity); err != nil {
			t.Fatalf("invalid legacy migration identity: %v", err)
		}
		if _, known := expectedCounts[identity.Name]; !known {
			t.Fatalf("unexpected retired official Skill: %s", identity.Name)
		}
		counts[identity.Name]++
		key := legacyMigrationKey(identity)
		if catalog[key] {
			t.Fatalf("duplicate legacy migration identity: %#v", identity)
		}
		catalog[key] = true
		canonical = append(canonical, key)
		switch {
		case strings.HasPrefix(identity.Provenance, "tag:"):
			tagCount++
			if strings.HasPrefix(identity.Provenance, "tag:v") && strings.TrimPrefix(identity.Provenance, "tag:v") != identity.SkillVersion {
				t.Fatalf("release tag provenance does not match Skill version: %#v", identity)
			}
		case strings.HasPrefix(identity.Provenance, "commit:"):
			commitCount++
		default:
			t.Fatalf("unexpected legacy migration provenance: %q", identity.Provenance)
		}
	}
	if len(legacyRetiredOfficialSkillMigrations) != 75 {
		t.Fatalf("legacy migration catalog does not cover all 75 audited identities: %d", len(legacyRetiredOfficialSkillMigrations))
	}
	if !reflect.DeepEqual(counts, expectedCounts) {
		t.Fatalf("legacy migration catalog has unexpected per-Skill counts: %#v", counts)
	}
	if tagCount != 34 || commitCount != 41 {
		t.Fatalf("legacy migration provenance must remain the audited 34 tags and 41 commits: tags=%d commits=%d", tagCount, commitCount)
	}

	aggregated := make(map[string]bool, len(catalog))
	retiredNames := make(map[string]bool, len(retiredOfficialSkills))
	for _, retired := range retiredOfficialSkills {
		if retiredNames[retired.Name] {
			t.Fatalf("retired official Skill is duplicated: %s", retired.Name)
		}
		retiredNames[retired.Name] = true
		for _, active := range officialSkillNames {
			if retired.Name == active {
				t.Fatalf("active official Skill %s must not be retired", active)
			}
		}
		for _, identity := range retired.LegacyMigrations {
			key := legacyMigrationKey(identity)
			if !catalog[key] {
				t.Fatalf("retirement wiring contains an unaudited legacy identity: %#v", identity)
			}
			if aggregated[key] {
				t.Fatalf("retirement wiring repeats a legacy identity: %#v", identity)
			}
			aggregated[key] = true
		}
	}
	if !reflect.DeepEqual(aggregated, catalog) {
		t.Fatal("retirement wiring does not include every audited legacy identity exactly once")
	}
	if len(retiredNames) != len(expectedCounts) {
		t.Fatalf("retirement wiring contains unexpected Skills: %#v", retiredNames)
	}

	sort.Strings(canonical)
	digest := sha256.Sum256([]byte(strings.Join(canonical, "\n")))
	const auditedCatalogDigest = "ef43eea6bb6a8994b2d57b5ebdf041e835191658f263e03328c719c3836c66be"
	if actual := fmt.Sprintf("%x", digest); actual != auditedCatalogDigest {
		t.Fatalf("legacy migration catalog differs from the audited manifest history: %s", actual)
	}
}

func legacyMigrationKey(identity skillcontent.LegacyRetiredSkillIdentity) string {
	return strings.Join([]string{
		identity.Name,
		identity.SkillVersion,
		identity.MinimumCLIVersion,
		identity.CLICompatibility,
		identity.FullBundleDigest,
		identity.EmbeddedContentDigest,
		identity.Provenance,
	}, "\x00")
}
