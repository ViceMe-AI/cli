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

// Record is the local delivery-recovery state of one channel delivery. It is
// isolated per endpoint, market, publication, and Skill directory, and it is
// author-side state only: it never proves an installation, an entitlement, or
// GitHub authorization. Remote branch/PR progress must be re-verified against
// GitHub itself; this record only says what this machine last applied.
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
	ZipPath        string            `json:"zipPath"`
	ZipDigest      string            `json:"zipDigest"`
	Generator      string            `json:"generatorVersion"`
	CreatedAt      string            `json:"createdAt"`
	UpdatedAt      string            `json:"updatedAt"`
}

// Store persists delivery records under the CLI config directory, sharded by
// endpoint origin exactly like the skill-binding index.
type Store struct {
	Directory      string
	EndpointOrigin string
	Now            func() time.Time
	ReportDegraded privatefile.DegradedReporter
}

func (s Store) Key(publicationID, skillDir string) string {
	sum := sha256.Sum256([]byte(publicationID + "\x00" + skillDir))
	return hex.EncodeToString(sum[:16])
}

func (s Store) filename(key string) string {
	sum := sha256.Sum256([]byte(s.EndpointOrigin))
	return filepath.Join(s.Directory, hex.EncodeToString(sum[:8]), key+".json")
}

func (s Store) lockFilename(key string) string {
	return s.filename(key) + ".lock"
}

// Load returns the previous delivery record for the same publication and Skill
// directory. A missing record is not an error; the second return is false.
func (s Store) Load(publicationID, skillDir string) (Record, bool, error) {
	raw, err := os.ReadFile(s.filename(s.Key(publicationID, skillDir)))
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

// Save persists the record with an exclusive lock so concurrent deliveries of
// the same target serialize instead of interleaving writes.
func (s Store) Save(record Record) error {
	key := s.Key(record.PublicationID, record.SkillDir)
	filename := s.filename(key)
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		return operationError("SKILL_CHANNEL_RECORD_SAVE_FAILED", "could not create the channel delivery record directory", err)
	}
	lock := flock.New(s.lockFilename(key))
	locked, err := lock.TryLock()
	if err != nil {
		return operationError("SKILL_CHANNEL_RECORD_LOCK_FAILED", "could not lock the channel delivery record", err)
	}
	if !locked {
		return output.Policy("SKILL_CHANNEL_DELIVERY_IN_PROGRESS",
			"another channel delivery for the same Skill directory is running in this process family").
			WithHint("wait for the other delivery to finish, then re-run this command; it is idempotent")
	}
	defer lock.Unlock()
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
