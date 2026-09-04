package publication

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/privatefile"
	"github.com/gofrs/flock"
)

type Pending struct {
	SchemaVersion     int    `json:"schemaVersion"`
	PublicationID     string `json:"publicationId"`
	ClientRequestID   string `json:"clientRequestId"`
	MerchantAccountID string `json:"merchantAccountId"`
	Fingerprint       string `json:"fingerprint"`
	SourcePath        string `json:"sourcePath"`
	PriceMinor        *int   `json:"priceMinor"`
	TrialUseLimit     *int   `json:"trialUseLimit"`
	// TrialDisabled records an explicit --trial-use-limit 0: it must reach the
	// server as an explicit null ("disable"), never as an omitted field
	// ("keep"), so an inherited live trial can actually be turned off.
	TrialDisabled  bool                        `json:"trialDisabled,omitempty"`
	ArtifactDigest string                      `json:"artifactDigest"`
	Source         api.SkillPublicationSource  `json:"source"`
	Edition        api.SkillPublicationEdition `json:"edition"`
	CreatedAt      time.Time                   `json:"createdAt"`
	UpdatedAt      time.Time                   `json:"updatedAt"`
}

type PendingStore struct {
	Directory string
	Now       func() time.Time
}

type Intent struct {
	SchemaVersion   int    `json:"schemaVersion"`
	Fingerprint     string `json:"fingerprint"`
	ClientRequestID string `json:"clientRequestId"`
	PublicationID   string `json:"publicationId,omitempty"`
}

// sweepStaleIntents removes INTENT files (never the publicationId-keyed
// recovery snapshots that share this directory) that are older than maxAge
// AND never progressed to a publication — those are pure orphans from
// abandoned creates. An intent that carries a publicationId may still back a
// legitimate --resume, so it is left to the use-time terminal-state check.
func (s PendingStore) sweepStaleIntents(maxAge time.Duration) {
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	entries, err := os.ReadDir(s.Directory)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "intent-") || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		info, err := entry.Info()
		if err != nil || now.Sub(info.ModTime()) < maxAge {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.Directory, entry.Name()))
		if err != nil {
			continue
		}
		var intent Intent
		if json.Unmarshal(data, &intent) != nil || intent.PublicationID != "" {
			continue
		}
		_ = os.Remove(filepath.Join(s.Directory, entry.Name()))
	}
}

func (s PendingStore) LoadOrCreateIntent(fingerprint string, newID func() string) (Intent, error) {
	if !isHexDigest(fingerprint) {
		return Intent{}, output.Validation("PUBLICATION_INTENT_INVALID", "publication intent fingerprint is invalid")
	}
	if err := os.MkdirAll(s.Directory, 0o700); err != nil {
		return Intent{}, recoveryOperationError(s.Directory, "PUBLICATION_INTENT_SAVE_FAILED", "could not create publication recovery directory", err)
	}
	// Opportunistic sweep: intents whose retire failed (agent sandboxes deny
	// the write) would otherwise accumulate forever. Best effort only —
	// failures never block the publish flow.
	s.sweepStaleIntents(30 * 24 * time.Hour)
	intentLock := flock.New(s.intentLockFilename(fingerprint))
	if err := intentLock.Lock(); err != nil {
		return Intent{}, recoveryOperationError(s.Directory, "PUBLICATION_INTENT_LOCK_FAILED", "could not lock publication intent", err)
	}
	defer intentLock.Unlock()
	filename := s.intentFilename(fingerprint)
	data, err := os.ReadFile(filename)
	if err == nil {
		var existing Intent
		decoder := json.NewDecoder(strings.NewReader(string(data)))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&existing) == nil && existing.SchemaVersion == 1 && existing.Fingerprint == fingerprint && safeID(existing.ClientRequestID) && (existing.PublicationID == "" || safeID(existing.PublicationID)) {
			return existing, nil
		}
		return Intent{}, output.Validation("PUBLICATION_RECOVERY_INVALID", "local publication intent is invalid")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return Intent{}, recoveryOperationError(s.Directory, "PUBLICATION_INTENT_READ_FAILED", "could not read publication intent", err)
	}
	intent := Intent{SchemaVersion: 1, Fingerprint: fingerprint, ClientRequestID: newID()}
	if !safeID(intent.ClientRequestID) {
		return Intent{}, output.Internal("PUBLICATION_INTENT_ID_FAILED", "could not allocate a publication request ID", nil)
	}
	if err := writePrivateJSON(filename, intent); err != nil {
		return Intent{}, err
	}
	return intent, nil
}

func (s PendingStore) SaveIntent(intent Intent) error {
	if !isHexDigest(intent.Fingerprint) || !safeID(intent.ClientRequestID) || !safeID(intent.PublicationID) {
		return output.Validation("PUBLICATION_INTENT_INVALID", "publication intent is invalid")
	}
	if err := os.MkdirAll(s.Directory, 0o700); err != nil {
		return recoveryOperationError(s.Directory, "PUBLICATION_INTENT_SAVE_FAILED", "could not create publication recovery directory", err)
	}
	intentLock := flock.New(s.intentLockFilename(intent.Fingerprint))
	if err := intentLock.Lock(); err != nil {
		return recoveryOperationError(s.Directory, "PUBLICATION_INTENT_LOCK_FAILED", "could not lock publication intent", err)
	}
	defer intentLock.Unlock()
	data, err := os.ReadFile(s.intentFilename(intent.Fingerprint))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return output.Validation("PUBLICATION_INTENT_RETIRED", "publication intent was already retired")
		}
		return recoveryOperationError(s.Directory, "PUBLICATION_INTENT_READ_FAILED", "could not read publication intent", err)
	}
	var current Intent
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&current); err != nil || current.SchemaVersion != 1 || current.Fingerprint != intent.Fingerprint || current.ClientRequestID != intent.ClientRequestID || (current.PublicationID != "" && current.PublicationID != intent.PublicationID) {
		return output.Validation("PUBLICATION_INTENT_CONFLICT", "publication intent changed while the request was in flight")
	}
	return writePrivateJSON(s.intentFilename(intent.Fingerprint), intent)
}

// RetireIntent removes only the exact fingerprint mapping that produced this
// terminal publication. The compare-and-retire guard prevents an older process
// from deleting a newer publication intent after a delayed response.
func (s PendingStore) RetireIntent(fingerprint, publicationID, clientRequestID string) error {
	if !isHexDigest(fingerprint) || !safeID(publicationID) || !safeID(clientRequestID) {
		return output.Validation("PUBLICATION_INTENT_INVALID", "publication intent is invalid")
	}
	intentLock := flock.New(s.intentLockFilename(fingerprint))
	if err := intentLock.Lock(); err != nil {
		return recoveryOperationError(s.Directory, "PUBLICATION_INTENT_LOCK_FAILED", "could not lock publication intent", err)
	}
	defer intentLock.Unlock()
	filename := s.intentFilename(fingerprint)
	data, err := os.ReadFile(filename)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return recoveryOperationError(s.Directory, "PUBLICATION_INTENT_READ_FAILED", "could not read publication intent", err)
	}
	var current Intent
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&current); err != nil || current.SchemaVersion != 1 {
		return output.Validation("PUBLICATION_RECOVERY_INVALID", "local publication intent is invalid")
	}
	if current.Fingerprint != fingerprint || current.PublicationID != publicationID || current.ClientRequestID != clientRequestID {
		return nil
	}
	if err := os.Remove(filename); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return recoveryOperationError(s.Directory, "PUBLICATION_INTENT_RETIRE_FAILED", "could not retire publication intent", err)
	}
	return nil
}

func (s PendingStore) Save(value Pending) error {
	if !safeID(value.PublicationID) {
		return output.Validation("PUBLICATION_ID_INVALID", "publication ID is invalid")
	}
	if !isHexDigest(value.Fingerprint) || !safeID(value.ClientRequestID) || !safeID(value.MerchantAccountID) || !isHexDigest(value.ArtifactDigest) {
		return output.Validation("PUBLICATION_RECOVERY_INVALID", "publication recovery state is invalid")
	}
	if s.Now == nil {
		s.Now = time.Now
	}
	now := s.Now().UTC()
	value.SchemaVersion = 2
	if value.CreatedAt.IsZero() {
		value.CreatedAt = now
	}
	value.UpdatedAt = now
	if err := os.MkdirAll(s.Directory, 0o700); err != nil {
		return recoveryOperationError(s.Directory, "PUBLICATION_PENDING_SAVE_FAILED", "could not create the local publication recovery directory", err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return output.Internal("PUBLICATION_PENDING_SAVE_FAILED", "could not encode publication recovery state", err)
	}
	data = append(data, '\n')
	if err := privatefile.Write(s.filename(value.PublicationID), data, ".pending-*.tmp"); err != nil {
		return recoveryOperationError(s.Directory, "PUBLICATION_PENDING_SAVE_FAILED", "could not write publication recovery state", err)
	}
	return nil
}

func (s PendingStore) Load(publicationID string) (Pending, error) {
	if !safeID(publicationID) {
		return Pending{}, output.Validation("PUBLICATION_ID_INVALID", "publication ID is invalid")
	}
	data, err := os.ReadFile(s.filename(publicationID))
	if errors.Is(err, fs.ErrNotExist) {
		return Pending{}, output.Validation("PUBLICATION_RECOVERY_NOT_FOUND", "no local recovery state exists for this publication").WithHint("run skill publish again with --path, or use publication get to inspect server state")
	}
	if err != nil {
		return Pending{}, recoveryOperationError(s.Directory, "PUBLICATION_PENDING_READ_FAILED", "could not read publication recovery state", err)
	}
	var value Pending
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil || value.SchemaVersion != 2 || value.PublicationID != publicationID || !isHexDigest(value.Fingerprint) || !safeID(value.ClientRequestID) || !safeID(value.MerchantAccountID) || !isHexDigest(value.ArtifactDigest) {
		return Pending{}, output.Validation("PUBLICATION_RECOVERY_INVALID", "local publication recovery state is invalid")
	}
	return value, nil
}

func (s PendingStore) Delete(publicationID string) error {
	if !safeID(publicationID) {
		return output.Validation("PUBLICATION_ID_INVALID", "publication ID is invalid")
	}
	err := os.Remove(s.filename(publicationID))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return recoveryOperationError(s.Directory, "PUBLICATION_PENDING_DELETE_FAILED", "could not remove publication recovery state", err)
	}
	return nil
}

func (s PendingStore) filename(publicationID string) string {
	return filepath.Join(s.Directory, fmt.Sprintf("%s.json", publicationID))
}

func (s PendingStore) intentFilename(fingerprint string) string {
	return filepath.Join(s.Directory, fmt.Sprintf("intent-%s.json", fingerprint))
}

func (s PendingStore) intentLockFilename(fingerprint string) string {
	return filepath.Join(s.Directory, fmt.Sprintf("intent-%s.lock", fingerprint))
}

func writePrivateJSON(filename string, value any) error {
	directory := filepath.Dir(filename)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return recoveryOperationError(directory, "PUBLICATION_RECOVERY_SAVE_FAILED", "could not create publication recovery directory", err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return output.Internal("PUBLICATION_RECOVERY_SAVE_FAILED", "could not encode publication recovery state", err)
	}
	data = append(data, '\n')
	if err := privatefile.Write(filename, data, ".recovery-*.tmp"); err != nil {
		return recoveryOperationError(directory, "PUBLICATION_RECOVERY_SAVE_FAILED", "could not write publication recovery state", err)
	}
	return nil
}

func recoveryOperationError(directory, fallbackCode, fallbackMessage string, err error) error {
	if errors.Is(err, fs.ErrPermission) {
		return output.Policy(
			"PUBLICATION_RECOVERY_PERMISSION_REQUIRED",
			"ViceMe cannot write the local publication recovery directory from this process",
		).
			WithCause(err).
			WithDetails(map[string]any{"directory": directory}).
			WithHint("allow this process to write the reported directory, then retry the exact same command; do not delete lock files or start a second publication")
	}
	return output.Internal(fallbackCode, fallbackMessage, err)
}

func isHexDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}

func safeID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, character := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if character != '-' {
				return false
			}
			continue
		}
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f') || (character >= 'A' && character <= 'F')) {
			return false
		}
	}
	return true
}
