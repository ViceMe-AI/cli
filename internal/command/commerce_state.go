package command

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/gofrs/flock"
)

var errCommerceSessionMissing = errors.New("commerce session missing")

type commerceSessionState struct {
	LocalContextID string    `json:"localContextId"`
	StableName     string    `json:"stableName"`
	ProductID      string    `json:"productId"`
	SessionID      string    `json:"sessionId"`
	PrincipalID    string    `json:"principalId"`
	PrincipalKind  string    `json:"principalKind"`
	Token          string    `json:"token"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

type commerceResourceBinding struct {
	LocalContextID string                      `json:"localContextId"`
	StableName     string                      `json:"stableName"`
	SessionID      string                      `json:"sessionId"`
	ExpiresAt      time.Time                   `json:"expiresAt"`
	PaymentOptions []api.CommercePaymentOption `json:"paymentOptions,omitempty"`
}

type commerceIntentState struct {
	ClientRequestID string     `json:"clientRequestId"`
	ResourceID      string     `json:"resourceId,omitempty"`
	ExpiresAt       *time.Time `json:"expiresAt,omitempty"`
}

type commerceSessionStartIntent struct {
	ClientRequestID string `json:"clientRequestId"`
	ReplaySecret    string `json:"replaySecret"`
}

func (runtime *Runtime) commerceStateKey(kind string, values ...string) string {
	parts := []string{runtime.profile.ID, runtime.credentialScope, kind}
	parts = append(parts, values...)
	digest := sha256.Sum256([]byte(joinStateParts(parts)))
	return "commerce:" + kind + ":" + hex.EncodeToString(digest[:])
}

func joinStateParts(parts []string) string {
	encoded, _ := json.Marshal(parts)
	return string(encoded)
}

func (runtime *Runtime) loadCommerceSession(localContextID, stableName string) (commerceSessionState, error) {
	var state commerceSessionState
	raw, err := runtime.deps.Store.Get(runtime.commerceStateKey("session", localContextID, stableName))
	if errors.Is(err, securestore.ErrNotFound) {
		return state, output.Validation("COMMERCE_SESSION_NOT_FOUND", "no local Commerce Session exists for this purchase Skill").
			WithHint("run 'viceme commerce session start --skill <stable-name>' and keep its localContextId in this Agent task").
			WithCause(errCommerceSessionMissing)
	}
	if err != nil {
		return state, output.Internal("COMMERCE_SESSION_READ_FAILED", "could not read the Commerce Session from the secure store", err)
	}
	if err := json.Unmarshal([]byte(raw), &state); err != nil || state.LocalContextID != localContextID || state.StableName != stableName || state.SessionID == "" || state.Token == "" {
		return state, output.Internal("COMMERCE_SESSION_INVALID", "the locally stored Commerce Session is invalid", err)
	}
	return state, nil
}

func (runtime *Runtime) saveCommerceSession(state commerceSessionState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return output.Internal("COMMERCE_SESSION_ENCODE_FAILED", "could not encode the Commerce Session", err)
	}
	if err := runtime.deps.Store.Set(runtime.commerceStateKey("session", state.LocalContextID, state.StableName), string(data)); err != nil {
		return output.Internal("COMMERCE_SESSION_SAVE_FAILED", "could not save the Commerce Session in the secure store", err).
			WithHint("repair the local secure credential store before starting a purchase")
	}
	return nil
}

func (runtime *Runtime) deleteCommerceSessionCredentials(localContextID, stableName string) error {
	for _, key := range []string{
		runtime.commerceStateKey("session", localContextID, stableName),
		runtime.commerceStateKey("session-start", localContextID, stableName),
	} {
		if err := runtime.deps.Store.Delete(key); err != nil && !errors.Is(err, securestore.ErrNotFound) {
			return output.Internal("COMMERCE_SESSION_DELETE_FAILED", "could not delete expired Commerce Session credentials", err)
		}
	}
	return nil
}

func (runtime *Runtime) saveCommerceBinding(kind, id string, binding commerceResourceBinding) error {
	data, err := json.Marshal(binding)
	if err != nil {
		return output.Internal("COMMERCE_BINDING_ENCODE_FAILED", "could not encode the Commerce resource binding", err)
	}
	if err := runtime.deps.Store.Set(runtime.commerceStateKey(kind, binding.LocalContextID, id), string(data)); err != nil {
		return output.Internal("COMMERCE_BINDING_SAVE_FAILED", "could not save the Commerce resource binding", err)
	}
	return nil
}

func (runtime *Runtime) loadCommerceBinding(kind, localContextID, id string) (commerceResourceBinding, error) {
	var binding commerceResourceBinding
	raw, err := runtime.deps.Store.Get(runtime.commerceStateKey(kind, localContextID, id))
	if errors.Is(err, securestore.ErrNotFound) {
		return binding, output.Validation("COMMERCE_RESOURCE_NOT_IN_SESSION", fmt.Sprintf("%s is not bound to a locally recoverable Commerce Session", kind)).
			WithHint("use the same purchase Skill and local profile that created this resource; cross-session order queries are not supported")
	}
	if err != nil {
		return binding, output.Internal("COMMERCE_BINDING_READ_FAILED", "could not read the Commerce resource binding", err)
	}
	if err := json.Unmarshal([]byte(raw), &binding); err != nil || binding.LocalContextID != localContextID || binding.StableName == "" || binding.SessionID == "" {
		return binding, output.Internal("COMMERCE_BINDING_INVALID", "the locally stored Commerce resource binding is invalid", err)
	}
	return binding, nil
}

func (runtime *Runtime) intentFor(operation, localContextID, stableName string, input map[string]json.RawMessage) (string, string, error) {
	if raw, ok := input["clientRequestId"]; ok {
		var provided string
		if err := json.Unmarshal(raw, &provided); err != nil || provided == "" {
			return "", "", output.Validation("COMMERCE_CLIENT_REQUEST_ID_INVALID", "clientRequestId must be a non-empty string")
		}
		return provided, "", nil
	}
	canonical, err := json.Marshal(input)
	if err != nil {
		return "", "", output.Internal("COMMERCE_INTENT_ENCODE_FAILED", "could not encode the Commerce intent", err)
	}
	digest := sha256.Sum256(canonical)
	key := runtime.commerceStateKey("intent", localContextID, operation, stableName, hex.EncodeToString(digest[:]))
	if raw, readErr := runtime.deps.Store.Get(key); readErr == nil {
		var existing commerceIntentState
		if json.Unmarshal([]byte(raw), &existing) == nil && existing.ClientRequestID != "" {
			if existing.ExpiresAt == nil || existing.ExpiresAt.After(runtime.deps.Now()) {
				return existing.ClientRequestID, key, nil
			}
		}
	} else if !errors.Is(readErr, securestore.ErrNotFound) {
		return "", "", output.Internal("COMMERCE_INTENT_READ_FAILED", "could not read the Commerce idempotency intent", readErr)
	}
	intent := commerceIntentState{ClientRequestID: runtime.deps.NewID()}
	data, err := json.Marshal(intent)
	if err != nil {
		return "", "", output.Internal("COMMERCE_INTENT_ENCODE_FAILED", "could not encode the Commerce idempotency intent", err)
	}
	if err := runtime.deps.Store.Set(key, string(data)); err != nil {
		return "", "", output.Internal("COMMERCE_INTENT_SAVE_FAILED", "could not persist the Commerce idempotency intent before the request", err)
	}
	return intent.ClientRequestID, key, nil
}

func (runtime *Runtime) lockCommerceSession(ctx context.Context, localContextID, stableName string) (func(), error) {
	if err := os.MkdirAll(runtime.configBase, 0o700); err != nil {
		return nil, output.Internal("COMMERCE_SESSION_LOCK_FAILED", "could not create the Commerce Session lock directory", err)
	}
	digest := sha256.Sum256([]byte(joinStateParts([]string{
		runtime.profile.ID,
		runtime.credentialScope,
		localContextID,
		stableName,
	})))
	lock := flock.New(filepath.Join(runtime.configBase, "commerce-session-"+hex.EncodeToString(digest[:])+".lock"))
	locked, err := lock.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil {
		return nil, output.Internal("COMMERCE_SESSION_LOCK_FAILED", "could not acquire the Commerce Session lock", err)
	}
	if !locked {
		return nil, output.Network("COMMERCE_SESSION_LOCK_INTERRUPTED", "Commerce Session creation was interrupted", ctx.Err())
	}
	return func() { _ = lock.Unlock() }, nil
}

func (runtime *Runtime) lockCommerceIntent(
	ctx context.Context,
	operation string,
	localContextID string,
	stableName string,
	input map[string]json.RawMessage,
) (func(), error) {
	canonical, err := json.Marshal(input)
	if err != nil {
		return nil, output.Internal("COMMERCE_INTENT_LOCK_FAILED", "could not encode the Commerce intent lock identity", err)
	}
	if err := os.MkdirAll(runtime.configBase, 0o700); err != nil {
		return nil, output.Internal("COMMERCE_INTENT_LOCK_FAILED", "could not create the Commerce intent lock directory", err)
	}
	inputDigest := sha256.Sum256(canonical)
	lockDigest := sha256.Sum256([]byte(joinStateParts([]string{
		runtime.profile.ID,
		runtime.credentialScope,
		operation,
		localContextID,
		stableName,
		hex.EncodeToString(inputDigest[:]),
	})))
	lock := flock.New(filepath.Join(runtime.configBase, "commerce-intent-"+hex.EncodeToString(lockDigest[:])+".lock"))
	locked, err := lock.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil {
		return nil, output.Internal("COMMERCE_INTENT_LOCK_FAILED", "could not acquire the Commerce intent lock", err)
	}
	if !locked {
		return nil, output.Network("COMMERCE_INTENT_LOCK_INTERRUPTED", "Commerce operation was interrupted while waiting for its recovery lock", ctx.Err())
	}
	return func() { _ = lock.Unlock() }, nil
}

func (runtime *Runtime) completeIntent(key, requestID, resourceID string, expiresAt *time.Time) error {
	if key == "" {
		return nil
	}
	data, err := json.Marshal(commerceIntentState{
		ClientRequestID: requestID,
		ResourceID:      resourceID,
		ExpiresAt:       expiresAt,
	})
	if err != nil {
		return output.Internal("COMMERCE_INTENT_ENCODE_FAILED", "could not encode the completed Commerce intent", err)
	}
	if err := runtime.deps.Store.Set(key, string(data)); err != nil {
		return output.Internal("COMMERCE_INTENT_SAVE_FAILED", "could not persist the completed Commerce intent", err)
	}
	return nil
}

func (runtime *Runtime) loadOrCreateCommerceSessionStartIntent(localContextID, stableName string) (commerceSessionStartIntent, error) {
	key := runtime.commerceStateKey("session-start", localContextID, stableName)
	var intent commerceSessionStartIntent
	raw, err := runtime.deps.Store.Get(key)
	if err == nil {
		if json.Unmarshal([]byte(raw), &intent) == nil &&
			intent.ClientRequestID == localContextID &&
			validCommerceReplaySecret(intent.ReplaySecret) {
			return intent, nil
		}
		return commerceSessionStartIntent{}, output.Internal("COMMERCE_SESSION_INTENT_INVALID", "the locally stored Commerce Session start intent is invalid", nil)
	}
	if !errors.Is(err, securestore.ErrNotFound) {
		return commerceSessionStartIntent{}, output.Internal("COMMERCE_SESSION_INTENT_READ_FAILED", "could not read the Commerce Session start intent", err)
	}
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return commerceSessionStartIntent{}, output.Internal("COMMERCE_SESSION_INTENT_RANDOM_FAILED", "could not generate the Commerce Session replay credential", err)
	}
	intent = commerceSessionStartIntent{
		ClientRequestID: localContextID,
		ReplaySecret:    base64.RawURLEncoding.EncodeToString(secretBytes),
	}
	encoded, err := json.Marshal(intent)
	if err != nil {
		return commerceSessionStartIntent{}, output.Internal("COMMERCE_SESSION_INTENT_ENCODE_FAILED", "could not encode the Commerce Session start intent", err)
	}
	if err := runtime.deps.Store.Set(key, string(encoded)); err != nil {
		return commerceSessionStartIntent{}, output.Internal("COMMERCE_SESSION_INTENT_SAVE_FAILED", "could not persist the Commerce Session start intent before the request", err)
	}
	return intent, nil
}

func validCommerceReplaySecret(value string) bool {
	if len(value) != 43 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
}
