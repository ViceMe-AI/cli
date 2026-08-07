// Package managedrelease persists the per-project release state for a managed
// Skill App (VICEME_HOSTED). This is distinct from appmanifest.Manifest, which
// tracks the Creator App binding shared by EXTERNAL hosted apps: the managed
// release carries candidate/release IDs and content digests that the publish
// flow must echo back to the API.
package managedrelease

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

const SchemaVersion = 1

var (
	ErrNotFound = errors.New("managed release state not found; run 'viceme app init'")
	digestRegex = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// State captures the mutable release candidate state for a managed Skill App.
// The CLI writes it after `app init`, updates it after `app preview` (recording
// the source/build digests the API returned), and reads it during
// `app publish` so the confirmation can cite the exact bytes the user reviewed.
type State struct {
	SchemaVersion         int    `json:"schemaVersion"`
	AppID                 string `json:"appId"`
	ReleaseID             string `json:"releaseId"`
	CandidateID           string `json:"candidateId"`
	Environment           string `json:"environment"`
	PublishableKey        string `json:"publishableKey"`
	RuntimeReleaseID      string `json:"runtimeReleaseId"`
	RuntimeContractDigest string `json:"runtimeContractDigest"`
	TemplateName          string `json:"templateName"`
	TemplateVersion       string `json:"templateVersion"`
	TemplateDigest        string `json:"templateDigest"`
	AppSDKVersion         string `json:"appSdkVersion"`
	SourceDigest          string `json:"sourceDigest"`
	BuildDigest           string `json:"buildDigest"`
	PreviewRunID          string `json:"previewRunId,omitempty"`
	PreviewURL            string `json:"previewUrl,omitempty"`
}

// Path returns the absolute location of the managed-release state file.
func Path(directory string) string {
	return filepath.Join(directory, ".viceme", "managed-release.json")
}

func Load(directory string) (State, error) {
	file, err := os.Open(Path(directory))
	if errors.Is(err, os.ErrNotExist) {
		return State{}, ErrNotFound
	}
	if err != nil {
		return State{}, fmt.Errorf("open managed release state: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.DisallowUnknownFields()
	var state State
	if err := decoder.Decode(&state); err != nil {
		return State{}, fmt.Errorf("decode managed release state: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return State{}, errors.New("decode managed release state: trailing JSON data")
	}
	if err := validate(state); err != nil {
		return State{}, err
	}
	return state, nil
}

func Save(directory string, state State) (string, error) {
	if state.SchemaVersion == 0 {
		state.SchemaVersion = SchemaVersion
	}
	if err := validate(state); err != nil {
		return "", err
	}
	directory = filepath.Clean(directory)
	stateDirectory := filepath.Join(directory, ".viceme")
	if err := os.MkdirAll(stateDirectory, 0o755); err != nil {
		return "", fmt.Errorf("create managed release directory: %w", err)
	}
	temporary, err := os.CreateTemp(stateDirectory, ".managed-release-*.json")
	if err != nil {
		return "", fmt.Errorf("create managed release temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o644); err != nil {
		return "", fmt.Errorf("set managed release permissions: %w", err)
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(state); err != nil {
		return "", fmt.Errorf("encode managed release state: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return "", fmt.Errorf("sync managed release state: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("close managed release state: %w", err)
	}
	filename := Path(directory)
	if err := os.Rename(temporaryName, filename); err != nil {
		return "", fmt.Errorf("replace managed release state: %w", err)
	}
	committed = true
	return filename, nil
}

func validate(state State) error {
	if state.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported managed release schemaVersion %d", state.SchemaVersion)
	}
	if state.AppID == "" {
		return errors.New("managed release state is missing appId")
	}
	if state.CandidateID == "" {
		return errors.New("managed release state is missing candidateId")
	}
	if state.PublishableKey == "" {
		return errors.New("managed release state is missing publishableKey")
	}
	if state.RuntimeReleaseID == "" {
		return errors.New("managed release state is missing runtimeReleaseId")
	}
	if state.TemplateDigest != "" && !digestRegex.MatchString(state.TemplateDigest) {
		return errors.New("managed release templateDigest is malformed")
	}
	if state.RuntimeContractDigest != "" && !digestRegex.MatchString(state.RuntimeContractDigest) {
		return errors.New("managed release runtimeContractDigest is malformed")
	}
	if state.SourceDigest != "" && !digestRegex.MatchString(state.SourceDigest) {
		return errors.New("managed release sourceDigest is malformed")
	}
	if state.BuildDigest != "" && !digestRegex.MatchString(state.BuildDigest) {
		return errors.New("managed release buildDigest is malformed")
	}
	return nil
}
