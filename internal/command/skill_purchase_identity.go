package command

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
)

// Only a verified direct-purchase installation may create this identity.
// Sharing the Python product lock prevents two runners from replacing it.
func ensureCloudPurchaseIdentity(runtime *Runtime, productID, agent string) (scriptTrialState, bool, error) {
	directory, manifest, _, err := skillcontent.FindRuntimeInstall(runtime.deps.Environment, agent, productID, runtime.apiBaseURL, runtime.deps.Environment.InstallDirectory)
	if err != nil {
		return scriptTrialState{}, false, err
	}
	if directory == "" || manifest.DeliveryMode != "CLOUD" || manifest.Kind != "purchase" || manifest.Market != string(runtime.region) {
		return scriptTrialState{}, false, nil
	}
	var state scriptTrialState
	err = withScriptTrialLock(runtime, productID, func() error {
		var exists bool
		var err error
		state, exists, err = readScriptPurchaseState(runtime, productID)
		if err != nil {
			return err
		}
		if _, trialExists, err := trialPurchaseCredential(runtime, productID); err != nil {
			return err
		} else if trialExists {
			return output.Policy("PURCHASE_IDENTITY_CONFLICT", "preserve the existing trial identity; no purchase identity was created")
		}
		if _, err := os.Lstat(scriptTrialCredentialPath(runtime, productID)); err == nil {
			return output.Policy("PURCHASE_IDENTITY_CONFLICT", "preserve the existing trial state; no purchase identity was created")
		} else if !os.IsNotExist(err) {
			return err
		}
		if exists {
			return nil
		}
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return err
		}
		state = scriptTrialState{CredentialKind: "purchase", ProductID: productID, Market: string(runtime.region), InstallID: runtime.deps.NewID(), Secret: hex.EncodeToString(secret)}
		if err := os.MkdirAll(filepath.Dir(scriptPurchasePath(runtime, productID)), 0o700); err != nil {
			return err
		}
		return saveSkillPurchaseState(runtime, productID, "purchase", state)
	})
	return state, err == nil, err
}

func scriptPurchasePath(runtime *Runtime, productID string) string {
	return filepath.Join(runtime.deps.Environment.Home, ".viceme", "purchases", productID+".json")
}

// This identity is created by the standalone purchase flow, never a trial grant.
// A malformed or conflicting record must not silently become another identity.
func readScriptPurchaseState(runtime *Runtime, productID string) (scriptTrialState, bool, error) {
	var state scriptTrialState
	filename := scriptPurchasePath(runtime, productID)
	info, err := os.Lstat(filename)
	if os.IsNotExist(err) {
		return state, false, nil
	}
	invalid := output.Policy("PURCHASE_STATE_INVALID", "restore the original purchase credential; no new identity was created")
	if err != nil || !info.Mode().IsRegular() {
		return state, false, invalid
	}
	raw, err := os.ReadFile(filename)
	if err != nil || json.Unmarshal(raw, &state) != nil || state.CredentialKind != "purchase" || !skillUseProductIDPattern.MatchString(state.InstallID) {
		return state, false, invalid
	}
	secret, err := hex.DecodeString(state.Secret)
	if err != nil || len(secret) != 32 {
		return state, false, invalid
	}
	if state.ProductID != productID || state.Market != string(runtime.region) {
		return state, false, output.Policy("PURCHASE_IDENTITY_MISMATCH", "purchase credential belongs to another product or market; preserve it")
	}
	return state, true, nil
}

func readSkillPurchaseState(runtime *Runtime, productID, kind string) (scriptTrialState, bool, error) {
	if kind == "purchase" {
		return readScriptPurchaseState(runtime, productID)
	}
	state, ok := readScriptTrialState(runtime, productID)
	return state, ok, nil
}

func saveSkillPurchaseState(runtime *Runtime, productID, kind string, state scriptTrialState) error {
	if kind == "purchase" {
		return saveScriptStateAt(scriptPurchasePath(runtime, productID), state)
	}
	return saveScriptTrialState(runtime, productID, state)
}
