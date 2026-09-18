package channeldelivery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/privatefile"
	"github.com/gofrs/flock"
)

const RecordAPIVersion = "channel-delivery.viceme.ai/v1"

// Record is the local delivery-recovery state of one Skill directory's
// channel. It is keyed by product and directory — not by publication — so the
// second publication of the same product inherits the ownership baseline of
// the last successful delivery instead of treating its own generated files as
// unknown author work. It is author-side state only: it never proves an
// installation, an entitlement, or GitHub authorization. Remote branch/PR
// progress must be re-verified against GitHub itself.
type Record struct {
	APIVersion     string            `json:"apiVersion"`
	EndpointOrigin string            `json:"endpointOrigin"`
	Market         string            `json:"market"`
	PublicationID  string            `json:"publicationId"`
	ListingID      string            `json:"listingId"`
	ProductID      string            `json:"productId"`
	ReleaseID      string            `json:"releaseId"`
	ProductSlug    string            `json:"productSlug"`
	Branch         string            `json:"branch"`
	SkillDir       string            `json:"skillDir"`
	AppliedFiles   map[string]string `json:"appliedFiles"`
	// RemovedFiles remembers managed paths an earlier delivery deleted, so a
	// re-run after a lost response still reports them in the commit scope
	// until a later delivery applies content at that path again.
	RemovedFiles map[string]string `json:"removedFiles,omitempty"`
	ZipPath      string            `json:"zipPath"`
	ZipDigest    string            `json:"zipDigest"`
	Generator    string            `json:"generatorVersion"`
	CreatedAt    string            `json:"createdAt"`
	UpdatedAt    string            `json:"updatedAt"`
}

// Store persists delivery records under the CLI config directory, sharded by
// endpoint origin exactly like the skill-binding index.
type Store struct {
	Directory      string
	EndpointOrigin string
	Now            func() time.Time
	ReportDegraded privatefile.DegradedReporter
}

// physicalDir resolves one Skill directory to its physical identity so path
// aliases — a symbolic link in any parent component, such as /tmp versus
// /private/tmp — share one lock and one record. A path that cannot be resolved
// is used cleaned.
func physicalDir(directory string) string {
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return filepath.Clean(directory)
	}
	return resolved
}

// Key identifies one delivery target: the product and the physical Skill
// directory under the current endpoint. Publications of the same product share
// the key so updates inherit the previous baseline.
func (s Store) Key(productID, skillDir string) string {
	sum := sha256.Sum256([]byte(productID + "\x00" + physicalDir(skillDir)))
	return hex.EncodeToString(sum[:16])
}

func (s Store) filename(key string) string {
	sum := sha256.Sum256([]byte(s.EndpointOrigin))
	return filepath.Join(s.Directory, hex.EncodeToString(sum[:8]), key+".json")
}

// Lock takes the delivery mutex of one Skill directory. The caller must hold
// it from before reading the baseline record until the delivery state is
// persisted, so concurrent deliveries — of any publication — cannot interleave
// filesystem writes on the same directory.
func (s Store) Lock(skillDir string) (func() error, error) {
	sum := sha256.Sum256([]byte(s.EndpointOrigin + "\x00" + physicalDir(skillDir)))
	shard := filepath.Join(s.Directory, hex.EncodeToString(sum[:8]))
	if err := os.MkdirAll(shard, 0o700); err != nil {
		return nil, operationError("SKILL_CHANNEL_RECORD_SAVE_FAILED", "could not create the channel delivery record directory", err)
	}
	lock := flock.New(filepath.Join(shard, hex.EncodeToString(sum[:])+".lock"))
	locked, err := lock.TryLock()
	if err != nil {
		return nil, operationError("SKILL_CHANNEL_RECORD_LOCK_FAILED", "could not lock the channel delivery record", err)
	}
	if !locked {
		return nil, output.Policy("SKILL_CHANNEL_DELIVERY_IN_PROGRESS",
			"another channel delivery for the same Skill directory is running in this process family").
			WithHint("wait for the other delivery to finish, then re-run this command; it is idempotent")
	}
	return lock.Unlock, nil
}

// Load returns the previous delivery record for the same product and Skill
// directory. A missing record is not an error; the second return is false.
func (s Store) Load(productID, skillDir string) (Record, bool, error) {
	raw, err := os.ReadFile(s.filename(s.Key(productID, skillDir)))
	if errors.Is(err, fs.ErrNotExist) {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, operationError("SKILL_CHANNEL_RECORD_READ_FAILED", "could not read the channel delivery record", err)
	}
	record := Record{}
	if err := json.Unmarshal(raw, &record); err != nil || record.APIVersion != RecordAPIVersion {
		return Record{}, false, operationError("SKILL_CHANNEL_RECORD_INVALID", "the channel delivery record is unreadable", err)
	}
	return record, true, nil
}

// Save persists the record. The caller must hold the directory Lock; writes
// happen inside it so the on-disk record never interleaves with another
// delivery of the same directory.
func (s Store) Save(record Record) error {
	record.SkillDir = physicalDir(record.SkillDir)
	filename := s.filename(s.Key(record.ProductID, record.SkillDir))
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		return operationError("SKILL_CHANNEL_RECORD_SAVE_FAILED", "could not create the channel delivery record directory", err)
	}
	if s.Now != nil {
		stamp := s.Now().UTC().Format(time.RFC3339)
		if record.CreatedAt == "" {
			record.CreatedAt = stamp
		}
		record.UpdatedAt = stamp
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return output.Internal("SKILL_CHANNEL_RECORD_SAVE_FAILED", "could not encode the channel delivery record", err)
	}
	data = append(data, '\n')
	if err := privatefile.WriteTolerant(filename, data, ".delivery-*.tmp", s.ReportDegraded); err != nil {
		return operationError("SKILL_CHANNEL_RECORD_SAVE_FAILED", "could not write the channel delivery record", err)
	}
	return nil
}

func operationError(code, message string, err error) error {
	if errors.Is(err, fs.ErrPermission) {
		return output.Policy("SKILL_CHANNEL_RECORD_PERMISSION_REQUIRED",
			"ViceMe cannot write the local channel delivery records from this process").
			WithCause(err).
			WithHint("allow this process to write the CLI config directory, then retry the exact same command")
	}
	return output.Internal(code, message, err)
}

// BranchName derives the fixed delivery branch from the stable product slug.
// Display name, price, release, and generator changes never change it. An
// empty result means the slug carried nothing branch-safe; the caller falls
// back to the compact product ID.
func BranchName(productSlug string) string {
	var builder strings.Builder
	previousDash := true
	for _, character := range strings.ToLower(strings.TrimSpace(productSlug)) {
		switch {
		case character >= 'a' && character <= 'z' || character >= '0' && character <= '9':
			builder.WriteRune(character)
			previousDash = false
		default:
			if !previousDash {
				builder.WriteByte('-')
				previousDash = true
			}
		}
	}
	suffix := strings.Trim(builder.String(), "-")
	if suffix == "" {
		return ""
	}
	return "viceme-skill-" + suffix
}
