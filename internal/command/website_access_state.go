package command

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/privatefile"
	"github.com/ViceMe-AI/cli/internal/privatepath"
	"github.com/gofrs/flock"
)

const websiteAccessStateName = "website-access.v1.json"

type websiteAccessState struct {
	SchemaVersion    int                                     `json:"schemaVersion"`
	EndpointOrigin   string                                  `json:"endpointOrigin"`
	Market           string                                  `json:"market"`
	ProjectPath      string                                  `json:"projectPath"`
	ProjectBindingID string                                  `json:"projectBindingId"`
	Attempted        bool                                    `json:"attempted"`
	Phase            string                                  `json:"phase"`
	Request          api.WebsiteAccessConfigurationRequest   `json:"request"`
	PreviousAccess   *api.WorkSdkAccess                      `json:"previousAccess,omitempty"`
	Result           *api.WebsiteAccessConfigurationResponse `json:"result,omitempty"`
	HostReceipt      *websiteAccessHostReceipt               `json:"hostReceipt,omitempty"`
	Enrichment       *websiteEnrichmentState                 `json:"enrichment,omitempty"`
}

type websiteAccessHostReceipt struct {
	RequestID     string   `json:"requestId"`
	WorkID        string   `json:"workId"`
	ConfigVersion int      `json:"configVersion"`
	ModifiedFiles []string `json:"modifiedFiles"`
	Checks        []string `json:"checks"`
	Verified      bool     `json:"verified"`
}

type websiteEnrichmentState struct {
	ExpectedRevision int             `json:"expectedRevision"`
	Content          json.RawMessage `json:"content"`
}

func openWebsiteAccessState(runtime *Runtime, project string) (string, *flock.Flock, error) {
	absolute, err := filepath.Abs(project)
	if err != nil {
		return "", nil, websiteAccessStateError(err)
	}
	info, err := os.Lstat(absolute)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", nil, output.Validation("WEBSITE_ACCESS_PROJECT_INVALID", "website access requires a real project directory")
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", nil, websiteAccessStateError(err)
	}
	directory := filepath.Join(canonical, ".viceme")
	if _, err = privatepath.EnsureDirectory(directory); err != nil {
		return "", nil, websiteAccessStateError(err)
	}
	lockPath := filepath.Join(directory, "website-access.lock")
	if _, err = privatepath.EnsureFile(lockPath); err != nil {
		return "", nil, websiteAccessStateError(err)
	}
	lock := flock.New(lockPath)
	locked, err := lock.TryLock()
	if err != nil {
		return "", nil, websiteAccessStateError(err)
	}
	if !locked {
		return "", nil, output.Policy("WEBSITE_ACCESS_BUSY", "another website access operation is using this project")
	}
	return canonical, lock, nil
}

func loadWebsiteAccessState(runtime *Runtime, project string) (websiteAccessState, bool, error) {
	var state websiteAccessState
	filename := filepath.Join(project, ".viceme", websiteAccessStateName)
	if _, err := os.Lstat(filename); errors.Is(err, os.ErrNotExist) {
		return state, false, nil
	} else if err != nil {
		return state, false, websiteAccessStateError(err)
	}
	if err := privatepath.RequirePrivateFile(filename); err != nil {
		return state, false, websiteAccessStateError(err)
	}
	data, err := readReplicaBoundedFile(filename, maxCommandJSONBytes)
	if err != nil {
		return state, false, websiteAccessStateError(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&state); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return state, false, output.Validation("WEBSITE_ACCESS_STATE_INVALID", "website access recovery state is invalid")
	}
	if state.SchemaVersion != 1 || state.EndpointOrigin != runtime.apiBaseURL || state.Market != replicaPublicationMarket(runtime) || state.ProjectPath != project || !replicaUUIDPattern.MatchString(state.ProjectBindingID) || state.Request.ProjectBindingID != state.ProjectBindingID || !replicaUUIDPattern.MatchString(state.Request.RequestID) || state.Request.Market != state.Market {
		return state, false, output.Validation("WEBSITE_ACCESS_BINDING_CONFLICT", "website access binding does not match this project, market or API authority")
	}
	switch state.Phase {
	case "PREPARED", "PLATFORM_CONFIGURED", "HOST_INTEGRATED", "VERIFIED":
	default:
		return state, false, output.Validation("WEBSITE_ACCESS_STATE_INVALID", "website access recovery phase is invalid")
	}
	return state, true, nil
}

func saveWebsiteAccessState(project string, state websiteAccessState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return websiteAccessStateError(err)
	}
	if err = privatefile.WriteAtomic(filepath.Join(project, ".viceme", websiteAccessStateName), data, ".website-access-*.tmp"); err != nil {
		return websiteAccessStateError(err)
	}
	return nil
}

func websiteAccessStateError(err error) error {
	return output.Internal("WEBSITE_ACCESS_STORAGE_FAILED", "could not safely preserve website access recovery state", err)
}
