package command

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/atomicfile"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/gofrs/flock"
)

type replicaPurchaseState struct {
	SchemaVersion  int       `json:"schemaVersion"`
	APIOrigin      string    `json:"apiOrigin"`
	ShortCode      string    `json:"shortCode"`
	Target         string    `json:"target"`
	ReplicaID      string    `json:"replicaId,omitempty"`
	QuoteRequestID string    `json:"quoteRequestId"`
	QuoteID        string    `json:"quoteId,omitempty"`
	OrderRequestID string    `json:"orderRequestId,omitempty"`
	OrderNo        string    `json:"orderNo,omitempty"`
	OrderExpiresAt string    `json:"orderExpiresAt,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type replicaPurchaseStore struct {
	directory string
	filename  string
	lock      *flock.Flock
	origin    string
	shortCode string
	target    string
	now       func() time.Time
}

func newReplicaPurchaseStore(runtime *Runtime, shortCode, target string) (replicaPurchaseStore, error) {
	origin, err := api.NormalizeAPIOrigin(runtime.apiBaseURL)
	if err != nil {
		return replicaPurchaseStore{}, output.Validation("REPLICA_API_ORIGIN_INVALID", "Website Replica API origin is invalid").WithCause(err)
	}
	directory := filepath.Join(runtime.configBase, "replica-purchases")
	fingerprint := sha256.Sum256([]byte(origin + "\n" + shortCode))
	name := hex.EncodeToString(fingerprint[:])
	return replicaPurchaseStore{
		directory: directory,
		filename:  filepath.Join(directory, name+".json"),
		lock:      flock.New(filepath.Join(directory, name+".lock")),
		origin:    origin,
		shortCode: shortCode,
		target:    target,
		now:       runtime.deps.Now,
	}, nil
}

func (store replicaPurchaseStore) withLock(run func() error) error {
	if err := os.MkdirAll(store.directory, 0o700); err != nil {
		return output.Internal("REPLICA_PURCHASE_STATE_FAILED", "could not create Website Replica recovery directory", err)
	}
	if err := store.lock.Lock(); err != nil {
		return output.Internal("REPLICA_PURCHASE_LOCK_FAILED", "could not lock Website Replica purchase recovery", err)
	}
	defer store.lock.Unlock()
	return run()
}

func (store replicaPurchaseStore) load() (replicaPurchaseState, bool, error) {
	data, err := os.ReadFile(store.filename)
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
	return state, true, nil
}

func (store replicaPurchaseStore) create(quoteRequestID string) replicaPurchaseState {
	now := store.now().UTC()
	return replicaPurchaseState{
		SchemaVersion: 1, APIOrigin: store.origin, ShortCode: store.shortCode,
		Target: store.target, QuoteRequestID: quoteRequestID,
		CreatedAt: now, UpdatedAt: now,
	}
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
	temporary, err := os.CreateTemp(store.directory, ".replica-purchase-*.tmp")
	if err != nil {
		return output.Internal("REPLICA_PURCHASE_STATE_FAILED", "could not create Website Replica purchase recovery", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return output.Internal("REPLICA_PURCHASE_STATE_FAILED", "could not secure Website Replica purchase recovery", err)
	}
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

func (store replicaPurchaseStore) retire() error {
	if err := os.Remove(store.filename); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return output.Internal("REPLICA_PURCHASE_STATE_FAILED", "could not retire Website Replica purchase recovery", err)
	}
	if err := atomicfile.SyncDirectory(store.directory); err != nil {
		return output.Internal("REPLICA_PURCHASE_STATE_FAILED", "could not sync Website Replica purchase recovery retirement", err)
	}
	return nil
}

func (store replicaPurchaseStore) valid(state replicaPurchaseState) bool {
	if state.SchemaVersion != 1 || state.APIOrigin != store.origin || state.ShortCode != store.shortCode ||
		!filepath.IsAbs(state.Target) || filepath.Clean(state.Target) != state.Target ||
		!replicaUUIDPattern.MatchString(state.QuoteRequestID) ||
		state.CreatedAt.IsZero() || state.UpdatedAt.IsZero() {
		return false
	}
	if state.ReplicaID != "" && !replicaUUIDPattern.MatchString(state.ReplicaID) {
		return false
	}
	if state.QuoteID != "" && !replicaUUIDPattern.MatchString(state.QuoteID) {
		return false
	}
	if state.OrderRequestID != "" && (state.QuoteID == "" || !replicaUUIDPattern.MatchString(state.OrderRequestID)) {
		return false
	}
	if state.OrderNo != "" {
		if state.OrderRequestID == "" || state.OrderExpiresAt == "" || len(state.OrderNo) < 6 || len(state.OrderNo) > 40 {
			return false
		}
		if _, err := time.Parse(time.RFC3339Nano, state.OrderExpiresAt); err != nil {
			return false
		}
	} else if state.OrderExpiresAt != "" {
		return false
	}
	return true
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
