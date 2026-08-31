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

	publishedExpectedCounts := map[string]int{
		"viceme-access":             4,
		"viceme-creator-onboarding": 2,
		"viceme-danmaku":            19,
		"viceme-engagement":         5,
		"viceme-publish":            28,
		"viceme-shared":             28,
		"viceme-tip":                10,
	}
	publishedCounts := make(map[string]int)
	for _, identity := range publishedTagRetiredSkillMigrations {
		publishedCounts[identity.Name]++
		if identity.Provenance != "" {
			t.Fatalf("generated published-tag identity unexpectedly embeds provenance: %#v", identity)
		}
		identity.Provenance = "tag:v" + identity.SkillVersion
		if err := skillcontent.ValidateLegacyRetiredSkillIdentity(identity); err != nil {
			t.Fatalf("invalid published-tag migration identity: %v", err)
		}
	}
	if len(publishedTagRetiredSkillMigrations) != 96 || !reflect.DeepEqual(publishedCounts, publishedExpectedCounts) {
		t.Fatalf("published-tag migration catalog drifted: total=%d counts=%#v", len(publishedTagRetiredSkillMigrations), publishedCounts)
	}

	expectedRetiredNames := map[string]bool{
		"viceme-shared":             true,
		"viceme-creator-onboarding": true,
		"viceme-publish":            true,
		"viceme-paid-skill":         true,
		"viceme-access":             true,
		"viceme-engagement":         true,
		"viceme-danmaku":            true,
		"viceme-tip":                true,
	}
	retiredNames := make(map[string]bool, len(retiredOfficialSkills))
	aggregated := make(map[string]bool)
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
			if err := skillcontent.ValidateLegacyRetiredSkillIdentity(identity); err != nil {
				t.Fatalf("invalid wired retirement identity: %v", err)
			}
			key := legacyMigrationKey(identity)
			if aggregated[key] {
				t.Fatalf("retirement wiring repeats a legacy identity: %#v", identity)
			}
			aggregated[key] = true
		}
	}
	for key := range catalog {
		if !aggregated[key] {
			t.Fatalf("retirement wiring omitted audited origin/dev identity: %q", key)
		}
	}
	if !reflect.DeepEqual(retiredNames, expectedRetiredNames) {
		t.Fatalf("retirement wiring contains unexpected Skill names: %#v", retiredNames)
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
