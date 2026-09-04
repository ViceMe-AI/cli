package replicapublication

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/atomicfile"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/privatefile"
	"github.com/ViceMe-AI/cli/internal/privatepath"
	"github.com/ViceMe-AI/cli/internal/replicacontent"
	"github.com/gofrs/flock"
)

const (
	stateSchemaVersion   = 1
	bindingSchemaVersion = 1
	bindingFilename      = "website-replica.json"
)

var (
	uuidPattern   = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

type Preview struct {
	Verified     bool   `json:"verified"`
	TargetURL    string `json:"targetUrl,omitempty"`
	Reused       bool   `json:"reused"`
	StartedByCLI bool   `json:"startedByCli"`
}

type Pending struct {
	SchemaVersion               int                                                 `json:"schemaVersion"`
	EndpointOrigin              string                                              `json:"endpointOrigin"`
	Market                      string                                              `json:"market"`
	ProjectPath                 string                                              `json:"projectPath"`
	ProjectFingerprint          string                                              `json:"projectFingerprint"`
	ClientRequestID             string                                              `json:"clientRequestId"`
	Request                     api.CreateWebsiteReplicaPublicationRequest          `json:"request"`
	SourceArchive               replicacontent.SourceArchiveSummary                 `json:"sourceArchive"`
	ArtifactExpiresAt           time.Time                                           `json:"artifactExpiresAt"`
	Preview                     Preview                                             `json:"preview"`
	AutoApplyCreator            bool                                                `json:"autoApplyCreator"`
	CreatorApplicationRequestID string                                              `json:"creatorApplicationRequestId,omitempty"`
	Confirmation                *api.WebsiteReplicaPublicationConfirmationChallenge `json:"confirmation,omitempty"`
	ConfirmedAt                 *time.Time                                          `json:"confirmedAt,omitempty"`
	Publication                 *PublicationReference                               `json:"publication,omitempty"`
	Target                      *api.WebsiteReplicaPublicationResolvedTarget        `json:"target,omitempty"`
	TakenOver                   bool                                                `json:"takenOver"`
	CreatedAt                   time.Time                                           `json:"createdAt"`
	UpdatedAt                   time.Time                                           `json:"updatedAt"`
}

type PublicationReference struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	StatusURL string `json:"statusUrl"`
}

type Store struct {
	Directory      string
	EndpointOrigin string
	Market         string
	Now            func() time.Time
}

func ProjectFingerprint(endpointOrigin, market, projectPath string) (string, string, error) {
	absolute, err := filepath.Abs(projectPath)
	if err != nil {
		return "", "", output.Validation("REPLICA_PROJECT_PATH_INVALID", "could not resolve the Website Replica project path").WithCause(err)
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return "", "", output.Validation("REPLICA_PROJECT_PATH_INVALID", "could not inspect the Website Replica project path").WithCause(err)
	}
	if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && (!info.Mode().IsRegular() || !strings.EqualFold(filepath.Ext(absolute), ".zip"))) {
		return "", "", output.Validation("REPLICA_PROJECT_PATH_INVALID", "Website Replica publication requires a real project directory or ZIP file")
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", "", output.Validation("REPLICA_PROJECT_PATH_INVALID", "could not resolve the Website Replica source path").WithCause(err)
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return "", "", output.Validation("REPLICA_PROJECT_PATH_INVALID", "could not normalize the Website Replica source path").WithCause(err)
	}
	digest := sha256.Sum256([]byte("website-replica-project/v1\x00" + endpointOrigin + "\x00" + market + "\x00" + filepath.Clean(canonical)))
	return hex.EncodeToString(digest[:]), canonical, nil
}

func ScopedDirectory(base, endpointOrigin, market string) string {
	digest := sha256.Sum256([]byte("website-replica-publication-store/v1\x00" + endpointOrigin + "\x00" + market))
	return filepath.Join(base, strings.ToLower(market)+"-"+hex.EncodeToString(digest[:]))
}

func (store Store) Lock(projectFingerprint string) (*flock.Flock, error) {
	if !digestPattern.MatchString(projectFingerprint) {
		return nil, output.Validation("REPLICA_PROJECT_FINGERPRINT_INVALID", "Website Replica project fingerprint is invalid")
	}
	if err := store.ensureDirectory(); err != nil {
		return nil, err
	}
	lock := flock.New(filepath.Join(store.Directory, "project-"+projectFingerprint+".lock"))
	if err := lock.Lock(); err != nil {
		return nil, stateError("REPLICA_PUBLICATION_LOCK_FAILED", "could not lock the local Website Replica publication", err)
	}
	return lock, nil
}

func (store Store) LoadProject(projectFingerprint string) (Pending, bool, error) {
	if !digestPattern.MatchString(projectFingerprint) {
		return Pending{}, false, output.Validation("REPLICA_PROJECT_FINGERPRINT_INVALID", "Website Replica project fingerprint is invalid")
	}
	return store.load(store.stateFilename(projectFingerprint))
}

func (store Store) LoadPublication(publicationID string) (Pending, bool, error) {
	if !uuidPattern.MatchString(publicationID) {
		return Pending{}, false, output.Validation("REPLICA_PUBLICATION_ID_INVALID", "Website Replica Publication ID is invalid")
	}
	if err := privatepath.RequirePrivateDirectory(store.Directory); errors.Is(err, fs.ErrNotExist) {
		return Pending{}, false, nil
	} else if err != nil {
		return Pending{}, false, stateSafetyError(err)
	}
	entries, err := os.ReadDir(store.Directory)
	if err != nil {
		return Pending{}, false, stateError("REPLICA_PUBLICATION_STATE_READ_FAILED", "could not read local Website Replica publication state", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "pending-") || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		pending, found, loadErr := store.load(filepath.Join(store.Directory, entry.Name()))
		if loadErr != nil {
			return Pending{}, false, loadErr
		}
		if found && pending.Publication != nil && pending.Publication.ID == publicationID {
			return pending, true, nil
		}
	}
	return Pending{}, false, nil
}

func (store Store) Save(pending *Pending) error {
	if pending == nil {
		return output.Validation("REPLICA_PUBLICATION_STATE_INVALID", "local Website Replica publication state is invalid")
	}
	if store.Now == nil {
		store.Now = time.Now
	}
	if pending.CreatedAt.IsZero() {
		pending.CreatedAt = store.Now().UTC()
	}
	pending.SchemaVersion = stateSchemaVersion
	pending.UpdatedAt = store.Now().UTC()
	if err := store.validatePending(*pending); err != nil {
		return err
	}
	if err := store.ensureDirectory(); err != nil {
		return err
	}
	return writePrivateJSON(store.stateFilename(pending.ProjectFingerprint), pending, "REPLICA_PUBLICATION_STATE_SAVE_FAILED", "could not save local Website Replica publication state")
}

func (store Store) Delete(pending Pending) error {
	if !digestPattern.MatchString(pending.ProjectFingerprint) {
		return output.Validation("REPLICA_PROJECT_FINGERPRINT_INVALID", "Website Replica project fingerprint is invalid")
	}
	err := os.Remove(store.stateFilename(pending.ProjectFingerprint))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return stateError("REPLICA_PUBLICATION_STATE_DELETE_FAILED", "could not remove local Website Replica publication state", err)
	}
	return nil
}

func (store Store) SaveArtifact(clientRequestID string, frozen *replicacontent.FrozenSourceArchive) (returnErr error) {
	if !uuidPattern.MatchString(clientRequestID) || frozen == nil {
		return output.Validation("REPLICA_PUBLICATION_ARTIFACT_INVALID", "Website Replica frozen artifact is invalid")
	}
	if err := store.ensureArtifactDirectory(clientRequestID); err != nil {
		return err
	}
	input, info, err := frozen.Open()
	if err != nil {
		return stateError("REPLICA_PUBLICATION_ARTIFACT_READ_FAILED", "could not reopen the frozen Website Replica source", err)
	}
	defer func() { returnErr = errors.Join(returnErr, input.Close()) }()
	if info.Size() != frozen.Summary.SizeBytes {
		return output.Validation("REPLICA_PUBLICATION_ARTIFACT_INVALID", "Website Replica frozen artifact size changed")
	}
	directory := store.artifactDirectory(clientRequestID)
	stage, err := privatepath.CreateTempFile(directory, ".source-*.tmp")
	if err != nil {
		return stateError("REPLICA_PUBLICATION_ARTIFACT_SAVE_FAILED", "could not create the Website Replica recovery artifact", err)
	}
	stageName := stage.Name()
	defer os.Remove(stageName)
	written, copyErr := io.Copy(stage, input)
	if copyErr == nil && written != frozen.Summary.SizeBytes {
		copyErr = errors.New("frozen source copy was incomplete")
	}
	if copyErr == nil {
		copyErr = stage.Sync()
	}
	closeErr := stage.Close()
	if copyErr != nil || closeErr != nil {
		return stateError("REPLICA_PUBLICATION_ARTIFACT_SAVE_FAILED", "could not persist the Website Replica recovery artifact", errors.Join(copyErr, closeErr))
	}
	target := store.artifactFilename(clientRequestID)
	if err := atomicfile.Replace(stageName, target); err != nil {
		return stateError("REPLICA_PUBLICATION_ARTIFACT_SAVE_FAILED", "could not activate the Website Replica recovery artifact", err)
	}
	if err := privatepath.RequirePrivateFile(target); err != nil {
		return stateError("REPLICA_PUBLICATION_ARTIFACT_SAVE_FAILED", "Website Replica recovery artifact is not private", err)
	}
	return nil
}

func (store Store) OpenArtifact(pending Pending) (*os.File, error) {
	if err := store.validatePending(pending); err != nil {
		return nil, err
	}
	filename := store.artifactFilename(pending.ClientRequestID)
	if err := privatepath.RequirePrivateFile(filename); err != nil {
		return nil, stateError("REPLICA_PUBLICATION_ARTIFACT_MISSING", "the frozen Website Replica source is unavailable", err)
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, stateError("REPLICA_PUBLICATION_ARTIFACT_READ_FAILED", "could not open the frozen Website Replica source", err)
	}
	valid := false
	defer func() {
		if !valid {
			_ = file.Close()
		}
	}()
	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(file, pending.SourceArchive.SizeBytes+1))
	if err != nil || written != pending.SourceArchive.SizeBytes || hex.EncodeToString(hash.Sum(nil)) != pending.SourceArchive.Digest {
		return nil, output.Validation("REPLICA_PUBLICATION_ARTIFACT_CHANGED", "the frozen Website Replica source no longer matches its confirmation").WithCause(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, stateError("REPLICA_PUBLICATION_ARTIFACT_READ_FAILED", "could not rewind the frozen Website Replica source", err)
	}
	valid = true
	return file, nil
}

func (store Store) DeleteArtifact(pending Pending) error {
	if !uuidPattern.MatchString(pending.ClientRequestID) {
		return output.Validation("REPLICA_PUBLICATION_ARTIFACT_INVALID", "Website Replica frozen artifact identity is invalid")
	}
	filename := store.artifactFilename(pending.ClientRequestID)
	if err := os.Remove(filename); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return stateError("REPLICA_PUBLICATION_ARTIFACT_DELETE_FAILED", "could not remove the frozen Website Replica source", err)
	}
	if err := os.Remove(store.artifactDirectory(pending.ClientRequestID)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return stateError("REPLICA_PUBLICATION_ARTIFACT_DELETE_FAILED", "could not remove the frozen Website Replica source directory", err)
	}
	return nil
}

// CleanupExpiredArtifacts opportunistically removes frozen source bytes that
// can no longer be uploaded. Pending request identity remains available so a
// creator qualification delay can resume with the same clientRequestId after
// freezing and confirming fresh bytes.
func (store Store) CleanupExpiredArtifacts() error {
	if store.Now == nil {
		store.Now = time.Now
	}
	if err := privatepath.RequirePrivateDirectory(store.Directory); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return stateSafetyError(err)
	}
	entries, err := os.ReadDir(store.Directory)
	if err != nil {
		return stateError("REPLICA_PUBLICATION_STATE_READ_FAILED", "could not read local Website Replica publication state", err)
	}
	now := store.Now().UTC()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "pending-") || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		fingerprint := strings.TrimSuffix(strings.TrimPrefix(entry.Name(), "pending-"), ".json")
		if !digestPattern.MatchString(fingerprint) {
			return output.Validation("REPLICA_PUBLICATION_STATE_INVALID", "local Website Replica publication state is invalid")
		}
		lock, err := store.Lock(fingerprint)
		if err != nil {
			return err
		}
		cleanupErr := func() error {
			pending, found, err := store.load(filepath.Join(store.Directory, entry.Name()))
			if err != nil || !found {
				return err
			}
			expired := !now.Before(pending.ArtifactExpiresAt)
			if !expired && !pending.TakenOver {
				return nil
			}
			if err := store.DeleteArtifact(pending); err != nil {
				return err
			}
			if expired && (pending.Confirmation != nil || pending.ConfirmedAt != nil) {
				if pending.Publication != nil && pending.Publication.Status == "DRAFT" {
					return nil
				}
				pending.Confirmation = nil
				pending.ConfirmedAt = nil
				return store.Save(&pending)
			}
			return nil
		}()
		if err := errors.Join(cleanupErr, lock.Unlock()); err != nil {
			return err
		}
	}
	return nil
}

func (store Store) load(filename string) (Pending, bool, error) {
	if err := privatepath.RequirePrivateFile(filename); errors.Is(err, fs.ErrNotExist) {
		return Pending{}, false, nil
	} else if err != nil {
		return Pending{}, false, stateSafetyError(err)
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return Pending{}, false, stateError("REPLICA_PUBLICATION_STATE_READ_FAILED", "could not read local Website Replica publication state", err)
	}
	var pending Pending
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&pending); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return Pending{}, false, output.Validation("REPLICA_PUBLICATION_STATE_INVALID", "local Website Replica publication state is invalid").WithCause(err)
	}
	if err := store.validatePending(pending); err != nil {
		return Pending{}, false, err
	}
	return pending, true, nil
}

func (store Store) validatePending(pending Pending) error {
	request := pending.Request
	if pending.SchemaVersion != stateSchemaVersion || pending.EndpointOrigin != store.EndpointOrigin || pending.Market != store.Market ||
		!filepath.IsAbs(pending.ProjectPath) || !digestPattern.MatchString(pending.ProjectFingerprint) ||
		!uuidPattern.MatchString(pending.ClientRequestID) || request.ProtocolVersion != api.WebsiteReplicaPublicationProtocolVersion ||
		request.ClientRequestID != pending.ClientRequestID || request.Market != pending.Market || request.ProjectFingerprint != pending.ProjectFingerprint ||
		request.Confirmation != nil || request.Source.Digest != pending.SourceArchive.Digest ||
		request.Source.SizeBytes != pending.SourceArchive.SizeBytes || !digestPattern.MatchString(pending.SourceArchive.Digest) ||
		pending.SourceArchive.SizeBytes < 1 || pending.ArtifactExpiresAt.IsZero() || pending.CreatedAt.IsZero() || pending.UpdatedAt.IsZero() {
		return output.Validation("REPLICA_PUBLICATION_STATE_INVALID", "local Website Replica publication state is invalid")
	}
	if pending.Confirmation != nil {
		if pending.Confirmation.Review.ProjectFingerprint != pending.ProjectFingerprint ||
			pending.Confirmation.Review.Source.Digest != pending.SourceArchive.Digest || pending.Confirmation.Review.Source.SizeBytes != pending.SourceArchive.SizeBytes {
			return output.Validation("REPLICA_PUBLICATION_STATE_INVALID", "local Website Replica confirmation does not match the frozen source")
		}
	}
	if pending.ConfirmedAt != nil && pending.Confirmation == nil {
		return output.Validation("REPLICA_PUBLICATION_STATE_INVALID", "local Website Replica confirmation state is invalid")
	}
	if pending.CreatorApplicationRequestID != "" && !uuidPattern.MatchString(pending.CreatorApplicationRequestID) {
		return output.Validation("REPLICA_PUBLICATION_STATE_INVALID", "local Website Replica creator application identity is invalid")
	}
	if pending.Publication != nil && (!uuidPattern.MatchString(pending.Publication.ID) || !validPublicationStatus(pending.Publication.Status) || pending.Publication.StatusURL == "") {
		return output.Validation("REPLICA_PUBLICATION_STATE_INVALID", "local Website Replica Publication reference is invalid")
	}
	if pending.Publication != nil && pending.Publication.Status == "DRAFT" && (pending.Confirmation == nil || pending.ConfirmedAt == nil) {
		return output.Validation("REPLICA_PUBLICATION_STATE_INVALID", "draft Website Replica Publication is missing its local final confirmation")
	}
	return nil
}

func validPublicationStatus(status string) bool {
	switch status {
	case "DRAFT", "PROCESSING", "PUBLISHED", "PUBLISHED_DEGRADED", "FAILED", "CANCELLED":
		return true
	default:
		return false
	}
}

func (store Store) ensureDirectory() error {
	if strings.TrimSpace(store.Directory) == "" || strings.TrimSpace(store.EndpointOrigin) == "" || (store.Market != "CN" && store.Market != "GLOBAL") {
		return output.Internal("REPLICA_PUBLICATION_STATE_SCOPE_INVALID", "Website Replica publication state scope is invalid", nil)
	}
	if err := os.MkdirAll(filepath.Dir(store.Directory), 0o700); err != nil {
		return stateError("REPLICA_PUBLICATION_STATE_SAVE_FAILED", "could not create local Website Replica publication state", err)
	}
	if _, err := privatepath.EnsureDirectory(store.Directory); err != nil {
		return stateError("REPLICA_PUBLICATION_STATE_SAVE_FAILED", "local Website Replica publication state directory is not private", err)
	}
	return nil
}

func (store Store) ensureArtifactDirectory(clientRequestID string) error {
	if err := store.ensureDirectory(); err != nil {
		return err
	}
	root := filepath.Join(store.Directory, "artifacts")
	if _, err := privatepath.EnsureDirectory(root); err != nil {
		return stateError("REPLICA_PUBLICATION_ARTIFACT_SAVE_FAILED", "could not create the Website Replica artifact directory", err)
	}
	directory := store.artifactDirectory(clientRequestID)
	if _, err := privatepath.EnsureDirectory(directory); err != nil {
		return stateError("REPLICA_PUBLICATION_ARTIFACT_SAVE_FAILED", "could not create the Website Replica recovery directory", err)
	}
	return nil
}

func (store Store) stateFilename(projectFingerprint string) string {
	return filepath.Join(store.Directory, "pending-"+projectFingerprint+".json")
}

func (store Store) artifactDirectory(clientRequestID string) string {
	return filepath.Join(store.Directory, "artifacts", clientRequestID)
}

func (store Store) artifactFilename(clientRequestID string) string {
	return filepath.Join(store.artifactDirectory(clientRequestID), "source.zip")
}

func writePrivateJSON(filename string, value any, code, message string) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return output.Internal(code, message, err)
	}
	data = append(data, '\n')
	if err := privatefile.WriteAtomic(filename, data, ".replica-publication-*.tmp"); err != nil {
		return stateError(code, message, err)
	}
	return nil
}

func stateError(code, message string, err error) *output.Error {
	if errors.Is(err, fs.ErrPermission) || privatefile.IsPermissionDenial(err) {
		return stateSafetyError(err)
	}
	return output.Internal(code, message, err)
}

func stateSafetyError(err error) *output.Error {
	return output.Policy("REPLICA_PUBLICATION_STORAGE_PERMISSION_REQUIRED", "ViceMe cannot safely read or persist private Website Replica publication state").WithCause(err)
}
