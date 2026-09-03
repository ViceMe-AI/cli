package command

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/atomicfile"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/pathidentity"
	"github.com/ViceMe-AI/cli/internal/privatepath"
	"github.com/ViceMe-AI/cli/internal/replicacontent"
	"github.com/gofrs/flock"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"
)

type replicaPurchaseState struct {
	SchemaVersion      int       `json:"schemaVersion"`
	APIOrigin          string    `json:"apiOrigin"`
	ShortCode          string    `json:"shortCode"`
	Target             string    `json:"target"`
	TargetParentID     string    `json:"targetParentId"`
	ReplicaID          string    `json:"replicaId,omitempty"`
	ProductID          string    `json:"productId,omitempty"`
	SKUID              string    `json:"skuId,omitempty"`
	ProductTitle       string    `json:"productTitle,omitempty"`
	Currency           string    `json:"currency,omitempty"`
	PriceCents         int       `json:"priceCents,omitempty"`
	Reservation        string    `json:"reservation"`
	QuoteRequestID     string    `json:"quoteRequestId"`
	QuoteID            string    `json:"quoteId,omitempty"`
	QuoteExpiresAt     string    `json:"quoteExpiresAt,omitempty"`
	OrderRequestID     string    `json:"orderRequestId,omitempty"`
	Locale             string    `json:"locale,omitempty"`
	OrderNo            string    `json:"orderNo,omitempty"`
	OrderExpiresAt     string    `json:"orderExpiresAt,omitempty"`
	PaymentPresentedAt string    `json:"paymentPresentedAt,omitempty"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

type replicaPurchaseStore struct {
	directory          string
	filename           string
	completionFilename string
	lockFilename       string
	lock               *flock.Flock
	origin             string
	shortCode          string
	target             string
	targetParentID     string
	now                func() time.Time
}

type replicaCompletionState struct {
	SchemaVersion  int                  `json:"schemaVersion"`
	APIOrigin      string               `json:"apiOrigin"`
	ShortCode      string               `json:"shortCode"`
	Result         replicaInstallResult `json:"result"`
	TreeDigest     string               `json:"treeDigest"`
	TargetParentID string               `json:"targetParentId"`
	CompletedAt    time.Time            `json:"completedAt"`
}

func newReplicaPurchaseStore(runtime *Runtime, shortCode, target string) (replicaPurchaseStore, error) {
	origin, err := api.NormalizeAPIOrigin(runtime.apiBaseURL)
	if err != nil {
		return replicaPurchaseStore{}, output.Validation("REPLICA_API_ORIGIN_INVALID", "Website Replica API origin is invalid").WithCause(err)
	}
	targetParentID, err := pathidentity.Directory(filepath.Dir(target))
	if err != nil {
		return replicaPurchaseStore{}, output.Validation("REPLICA_TARGET_PARENT_INVALID", "could not bind Website Replica target parent identity").WithCause(err)
	}
	directory := filepath.Join(runtime.configBase, "replica-purchases")
	fingerprint := sha256.Sum256([]byte(origin + "\n" + shortCode))
	completionFingerprint := sha256.Sum256([]byte(origin + "\n" + shortCode + "\n" + target))
	name := hex.EncodeToString(fingerprint[:])
	return replicaPurchaseStore{
		directory:          directory,
		filename:           filepath.Join(directory, name+".json"),
		completionFilename: filepath.Join(directory, "completed-"+hex.EncodeToString(completionFingerprint[:])+".json"),
		lockFilename:       filepath.Join(directory, name+".lock"),
		lock:               flock.New(filepath.Join(directory, name+".lock")),
		origin:             origin,
		shortCode:          shortCode,
		target:             target,
		targetParentID:     targetParentID,
		now:                runtime.deps.Now,
	}, nil
}

func (store replicaPurchaseStore) withLock(run func() error) error {
	created, err := privatepath.EnsureDirectory(store.directory)
	if err != nil {
		return output.Internal("REPLICA_PURCHASE_STATE_FAILED", "could not create Website Replica recovery directory", err)
	}
	if created {
		if err := atomicfile.SyncDirectory(filepath.Dir(store.directory)); err != nil {
			return output.Internal("REPLICA_PURCHASE_STATE_FAILED", "could not sync Website Replica recovery directory creation", err)
		}
	}
	lockCreated, err := privatepath.EnsureFile(store.lockFilename)
	if err != nil {
		return output.Internal("REPLICA_PURCHASE_LOCK_FAILED", "could not create private Website Replica purchase lock", err)
	}
	if lockCreated {
		if err := atomicfile.SyncDirectory(store.directory); err != nil {
			return output.Internal("REPLICA_PURCHASE_LOCK_FAILED", "could not sync Website Replica purchase lock creation", err)
		}
	}
	if err := store.lock.Lock(); err != nil {
		return output.Internal("REPLICA_PURCHASE_LOCK_FAILED", "could not lock Website Replica purchase recovery", err)
	}
	defer store.lock.Unlock()
	return run()
}

func (store replicaPurchaseStore) load() (replicaPurchaseState, bool, error) {
	data, err := readReplicaPurchaseState(store.filename)
	if errors.Is(err, fs.ErrNotExist) {
		return replicaPurchaseState{}, false, nil
	}
	if err != nil {
		return replicaPurchaseState{}, false, output.Internal("REPLICA_PURCHASE_STATE_FAILED", "could not read Website Replica purchase recovery", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var state replicaPurchaseState
	if err := decoder.Decode(&state); err != nil || decoder.Decode(&struct{}{}) != io.EOF || !store.valid(state) {
		return replicaPurchaseState{}, false, output.Policy("REPLICA_PURCHASE_STATE_INVALID", "Website Replica purchase recovery state is invalid")
	}
	if err := store.verifyReservation(state); err != nil {
		return replicaPurchaseState{}, false, err
	}
	return state, true, nil
}

func (store replicaPurchaseStore) loadCompletion() (replicaCompletionState, bool, error) {
	data, err := readReplicaPurchaseState(store.completionFilename)
	if errors.Is(err, fs.ErrNotExist) {
		return replicaCompletionState{}, false, nil
	}
	if err != nil {
		return replicaCompletionState{}, false, output.Internal("REPLICA_COMPLETION_STATE_FAILED", "could not read Website Replica completion receipt", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var completion replicaCompletionState
	if err := decoder.Decode(&completion); err != nil || decoder.Decode(&struct{}{}) != io.EOF || !store.validCompletion(completion) {
		return replicaCompletionState{}, false, output.Policy("REPLICA_COMPLETION_STATE_INVALID", "Website Replica completion receipt is invalid")
	}
	return completion, true, nil
}

func readReplicaPurchaseState(filename string) ([]byte, error) {
	return readReplicaBoundedFile(filename, 16<<10)
}

func readReplicaBoundedFile(filename string, maxBytes int64) ([]byte, error) {
	if err := privatepath.RequirePrivateFile(filename); err != nil {
		return nil, err
	}
	before, err := os.Lstat(filename)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 || before.Size() > maxBytes {
		return nil, errors.New("Website Replica purchase recovery is not a bounded regular file")
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	after, statErr := file.Stat()
	data, readErr := io.ReadAll(io.LimitReader(file, maxBytes+1))
	closeErr := file.Close()
	if statErr != nil || readErr != nil || closeErr != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) || int64(len(data)) > maxBytes {
		return nil, errors.New("Website Replica purchase recovery changed while reading")
	}
	return data, nil
}

func (store replicaPurchaseStore) create(quoteRequestID string) replicaPurchaseState {
	now := store.now().UTC()
	return replicaPurchaseState{
		SchemaVersion: 4, APIOrigin: store.origin, ShortCode: store.shortCode,
		Target: store.target, TargetParentID: store.targetParentID, QuoteRequestID: quoteRequestID,
		CreatedAt: now, UpdatedAt: now,
	}
}

func (store replicaPurchaseStore) reserve(state *replicaPurchaseState) error {
	if err := store.verifyTargetParent(); err != nil {
		return err
	}
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return output.Internal("REPLICA_TARGET_RESERVATION_FAILED", "could not create the Website Replica target reservation", err)
	}
	state.Reservation = hex.EncodeToString(nonce)
	filename := replicaTargetReservationPath(state.Target)
	if err := store.removeOwnedOrphanReservation(filename); err != nil {
		return err
	}
	file, err := privatepath.CreateExclusiveFile(filename)
	if err != nil {
		return output.Policy("REPLICA_TARGET_RESERVED", "the Website Replica target is already reserved by another installation").WithCause(err)
	}
	writeErr := func() error {
		if _, err := file.Write(store.reservationPayload(*state)); err != nil {
			return err
		}
		if err := file.Sync(); err != nil {
			return err
		}
		return file.Close()
	}()
	if writeErr != nil {
		_ = file.Close()
		_ = os.Remove(filename)
		return output.Internal("REPLICA_TARGET_RESERVATION_FAILED", "could not persist the Website Replica target reservation", writeErr)
	}
	if err := atomicfile.SyncDirectory(filepath.Dir(filename)); err != nil {
		_ = os.Remove(filename)
		return output.Internal("REPLICA_TARGET_RESERVATION_FAILED", "could not sync the Website Replica target reservation", err)
	}
	return nil
}

func (store replicaPurchaseStore) removeOwnedOrphanReservation(filename string) error {
	data, err := readReplicaBoundedFile(filename, 256)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil || !bytes.HasPrefix(data, []byte(filepath.Base(store.filename)+"\n")) {
		return output.Policy("REPLICA_TARGET_RESERVED", "the Website Replica target is already reserved by another installation")
	}
	if err := os.Remove(filename); err != nil {
		return output.Internal("REPLICA_TARGET_RESERVATION_FAILED", "could not retire an interrupted target reservation", err)
	}
	if err := atomicfile.SyncDirectory(filepath.Dir(filename)); err != nil {
		return output.Internal("REPLICA_TARGET_RESERVATION_FAILED", "could not sync interrupted target reservation retirement", err)
	}
	return nil
}

func (store replicaPurchaseStore) verifyReservation(state replicaPurchaseState) error {
	if err := store.verifyTargetParent(); err != nil {
		return err
	}
	filename := replicaTargetReservationPath(state.Target)
	data, err := readReplicaBoundedFile(filename, 256)
	if err != nil || !bytes.Equal(data, store.reservationPayload(state)) {
		return output.Policy("REPLICA_TARGET_RESERVATION_INVALID", "the Website Replica target reservation changed unexpectedly")
	}
	return nil
}

func (store replicaPurchaseStore) reservationPayload(state replicaPurchaseState) []byte {
	return []byte(filepath.Base(store.filename) + "\n" + state.Reservation + "\n")
}

func replicaTargetReservationPath(target string) string {
	key := norm.NFKC.String(filepath.Base(target))
	key = cases.Lower(language.Und).String(key)
	key = cases.Upper(language.Und).String(key)
	key = norm.NFKC.String(cases.Lower(language.Und).String(key))
	fingerprint := sha256.Sum256([]byte(key))
	return filepath.Join(filepath.Dir(target), ".viceme-replica-reservation-"+hex.EncodeToString(fingerprint[:]))
}

func (store replicaPurchaseStore) save(state *replicaPurchaseState) error {
	state.UpdatedAt = store.now().UTC()
	if !store.valid(*state) {
		return output.Internal("REPLICA_PURCHASE_STATE_INVALID", "refusing to save invalid Website Replica purchase recovery", nil)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return output.Internal("REPLICA_PURCHASE_STATE_FAILED", "could not encode Website Replica purchase recovery", err)
	}
	data = append(data, '\n')
	temporary, err := privatepath.CreateTempFile(store.directory, ".replica-purchase-*.tmp")
	if err != nil {
		return output.Internal("REPLICA_PURCHASE_STATE_FAILED", "could not create Website Replica purchase recovery", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return output.Internal("REPLICA_PURCHASE_STATE_FAILED", "could not write Website Replica purchase recovery", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return output.Internal("REPLICA_PURCHASE_STATE_FAILED", "could not sync Website Replica purchase recovery", err)
	}
	if err := temporary.Close(); err != nil {
		return output.Internal("REPLICA_PURCHASE_STATE_FAILED", "could not close Website Replica purchase recovery", err)
	}
	if err := atomicfile.Replace(temporaryName, store.filename); err != nil {
		return output.Internal("REPLICA_PURCHASE_STATE_FAILED", "could not activate Website Replica purchase recovery", err)
	}
	if err := atomicfile.SyncDirectory(store.directory); err != nil {
		return output.Internal("REPLICA_PURCHASE_STATE_FAILED", "could not sync Website Replica purchase recovery", err)
	}
	return nil
}

func (store replicaPurchaseStore) saveCompletion(result replicaInstallResult) error {
	if err := store.verifyTargetParent(); err != nil {
		return err
	}
	tree, err := replicacontent.InspectInstalledTree(result.Target)
	if err != nil {
		return output.Internal("REPLICA_COMPLETION_STATE_FAILED", "could not inspect Website Replica completion target", err)
	}
	if tree.FileCount != result.FileCount || tree.ExpandedBytes != result.ExpandedBytes {
		return output.Internal("REPLICA_COMPLETION_STATE_INVALID", "Website Replica completion result does not match its installed tree", nil)
	}
	completion := replicaCompletionState{
		SchemaVersion:  1,
		APIOrigin:      store.origin,
		ShortCode:      store.shortCode,
		Result:         result,
		TreeDigest:     tree.Digest,
		TargetParentID: store.targetParentID,
		CompletedAt:    store.now().UTC(),
	}
	if !store.validCompletion(completion) {
		return output.Internal("REPLICA_COMPLETION_STATE_INVALID", "refusing to save invalid Website Replica completion receipt", nil)
	}
	data, err := json.MarshalIndent(completion, "", "  ")
	if err != nil {
		return output.Internal("REPLICA_COMPLETION_STATE_FAILED", "could not encode Website Replica completion receipt", err)
	}
	data = append(data, '\n')
	temporary, err := privatepath.CreateTempFile(store.directory, ".replica-completion-*.tmp")
	if err != nil {
		return output.Internal("REPLICA_COMPLETION_STATE_FAILED", "could not create Website Replica completion receipt", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return output.Internal("REPLICA_COMPLETION_STATE_FAILED", "could not write Website Replica completion receipt", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return output.Internal("REPLICA_COMPLETION_STATE_FAILED", "could not sync Website Replica completion receipt", err)
	}
	if err := temporary.Close(); err != nil {
		return output.Internal("REPLICA_COMPLETION_STATE_FAILED", "could not close Website Replica completion receipt", err)
	}
	if err := atomicfile.Replace(temporaryName, store.completionFilename); err != nil {
		return output.Internal("REPLICA_COMPLETION_STATE_FAILED", "could not activate Website Replica completion receipt", err)
	}
	if err := atomicfile.SyncDirectory(store.directory); err != nil {
		return output.Internal("REPLICA_COMPLETION_STATE_FAILED", "could not sync Website Replica completion receipt", err)
	}
	return nil
}

func (store replicaPurchaseStore) retire(state replicaPurchaseState) error {
	if err := store.verifyReservation(state); err != nil {
		return err
	}
	if err := os.Remove(store.filename); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return output.Internal("REPLICA_PURCHASE_STATE_FAILED", "could not retire Website Replica purchase recovery", err)
	}
	if err := atomicfile.SyncDirectory(store.directory); err != nil {
		return output.Internal("REPLICA_PURCHASE_STATE_FAILED", "could not sync Website Replica purchase recovery retirement", err)
	}
	reservation := replicaTargetReservationPath(state.Target)
	if err := os.Remove(reservation); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return output.Internal("REPLICA_TARGET_RESERVATION_FAILED", "could not retire the Website Replica target reservation", err)
	}
	if err := atomicfile.SyncDirectory(filepath.Dir(reservation)); err != nil {
		return output.Internal("REPLICA_TARGET_RESERVATION_FAILED", "could not sync target reservation retirement", err)
	}
	return nil
}

func (store replicaPurchaseStore) valid(state replicaPurchaseState) bool {
	if state.SchemaVersion != 4 || state.APIOrigin != store.origin || state.ShortCode != store.shortCode ||
		!filepath.IsAbs(state.Target) || filepath.Clean(state.Target) != state.Target ||
		state.TargetParentID != store.targetParentID ||
		!validReplicaReservation(state.Reservation) ||
		!replicaUUIDPattern.MatchString(state.QuoteRequestID) ||
		state.CreatedAt.IsZero() || state.UpdatedAt.IsZero() {
		return false
	}
	hasBinding := state.ReplicaID != "" || state.ProductID != "" || state.SKUID != "" || state.ProductTitle != "" || state.Currency != "" || state.PriceCents != 0
	if hasBinding && (!replicaUUIDPattern.MatchString(state.ReplicaID) || !replicaUUIDPattern.MatchString(state.ProductID) ||
		!replicaUUIDPattern.MatchString(state.SKUID) || state.ProductTitle == "" ||
		(state.Currency != "CNY" && state.Currency != "USD") || state.PriceCents < 0 || state.PriceCents > 10_000_000) {
		return false
	}
	if state.QuoteID != "" {
		if !hasBinding || !replicaUUIDPattern.MatchString(state.QuoteID) || !validReplicaStateDatetime(state.QuoteExpiresAt) {
			return false
		}
	} else if state.QuoteExpiresAt != "" {
		return false
	}
	if state.OrderRequestID != "" && (state.QuoteID == "" || !replicaUUIDPattern.MatchString(state.OrderRequestID) ||
		(state.Locale != "zh-CN" && state.Locale != "en-US")) {
		return false
	} else if state.OrderRequestID == "" && state.Locale != "" {
		return false
	}
	if state.OrderNo != "" {
		if state.OrderRequestID == "" || state.OrderExpiresAt == "" || len(state.OrderNo) < 6 || len(state.OrderNo) > 40 {
			return false
		}
		if !validReplicaStateDatetime(state.OrderExpiresAt) {
			return false
		}
	} else if state.OrderExpiresAt != "" {
		return false
	}
	if state.PaymentPresentedAt != "" && (state.OrderNo == "" || !validReplicaStateDatetime(state.PaymentPresentedAt)) {
		return false
	}
	return true
}

func (store replicaPurchaseStore) validCompletion(completion replicaCompletionState) bool {
	result := completion.Result
	return completion.SchemaVersion == 1 && completion.APIOrigin == store.origin && completion.ShortCode == store.shortCode &&
		completion.TargetParentID == store.targetParentID &&
		!completion.CompletedAt.IsZero() && result.Target == store.target && filepath.IsAbs(result.Target) && filepath.Clean(result.Target) == result.Target &&
		replicaUUIDPattern.MatchString(result.ReplicaID) && replicaUUIDPattern.MatchString(result.VersionID) && result.Version > 0 &&
		len(result.OrderNo) >= 6 && len(result.OrderNo) <= 40 && validReplicaDigest(result.ArtifactDigest) &&
		validReplicaDigest(completion.TreeDigest) &&
		result.LicensePath == filepath.Join(result.Target, filepath.FromSlash(replicacontent.LicenseFilePath)) &&
		result.FileCount > 0 && result.FileCount <= replicacontent.MaxFileCount && result.ExpandedBytes <= replicacontent.MaxExpandedBytes
}

func (store replicaPurchaseStore) verifyTargetParent() error {
	current, err := pathidentity.Directory(filepath.Dir(store.target))
	if err != nil || current != store.targetParentID {
		return output.Policy("REPLICA_TARGET_PARENT_CHANGED", "the Website Replica target parent changed after it was reserved").WithCause(err)
	}
	return nil
}

func validReplicaDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

func validReplicaReservation(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && value == strings.ToLower(value)
}

func validReplicaStateDatetime(value string) bool {
	_, valid := parseReplicaStateDatetime(value)
	return valid
}

func parseReplicaStateDatetime(value string) (time.Time, bool) {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed, true
	}
	if parsed, err := time.Parse("2006-01-02T15:04Z", value); err == nil {
		return parsed, true
	}
	dot := strings.LastIndexByte(value, '.')
	if dot < 0 || !strings.HasSuffix(value, "Z") {
		return time.Time{}, false
	}
	fraction := value[dot+1 : len(value)-1]
	if len(fraction) <= 9 {
		return time.Time{}, false
	}
	for _, digit := range fraction {
		if digit < '0' || digit > '9' {
			return time.Time{}, false
		}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value[:dot+1]+fraction[:9]+"Z")
	return parsed, err == nil
}

func (state replicaPurchaseState) describe() map[string]any {
	return map[string]any{
		"shortCode": state.ShortCode,
		"target":    state.Target,
		"orderNo":   state.OrderNo,
	}
}

func replicaPurchaseConflict(state replicaPurchaseState, message string) *output.Error {
	return output.Policy("REPLICA_PURCHASE_RECOVERY_CONFLICT", message).
		WithDetails(state.describe()).
		WithHint(fmt.Sprintf("preserve %s recovery state; remove or move the conflicting target, then retry the same command", state.Target))
}
