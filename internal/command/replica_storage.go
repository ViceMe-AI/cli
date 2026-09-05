package command

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/replicapublication"
	"github.com/spf13/cobra"
)

func addReplicaStorageFlag(command *cobra.Command, runtime *Runtime) {
	command.Flags().StringVar(&runtime.replicaProject, "state-project", "", "source project directory or ZIP whose .viceme directory stores publication recovery state")
}

func projectReplicaPublicationStore(runtime *Runtime, project string) (replicapublication.Store, error) {
	directory, err := replicapublication.ProjectStoreDirectory(project, runtime.apiBaseURL, replicaPublicationMarket(runtime))
	if err != nil {
		return replicapublication.Store{}, err
	}
	return replicapublication.Store{ProjectScoped: true, Directory: directory, EndpointOrigin: runtime.apiBaseURL, Market: replicaPublicationMarket(runtime), Now: runtime.deps.Now}, nil
}

func selectedReplicaPublicationStore(runtime *Runtime) (replicapublication.Store, error) {
	if runtime.replicaProject == "" {
		return replicaPublicationStore(runtime), nil
	}
	_, project, err := replicapublication.ProjectFingerprint(runtime.apiBaseURL, replicaPublicationMarket(runtime), runtime.replicaProject)
	if err != nil {
		return replicapublication.Store{}, err
	}
	runtime.replicaProject = project
	return projectReplicaPublicationStore(runtime, project)
}

// Selection is serialized in the project for both storage modes. Never switch
// an existing request to another store or treat unreadable state as absent.
func prepareReplicaPublishStore(runtime *Runtime, project, fingerprint string) (_ replicapublication.Store, unlock func() error, returnErr error) {
	projectStore, err := projectReplicaPublicationStore(runtime, project)
	if err != nil {
		return replicapublication.Store{}, nil, err
	}
	if runtime.replicaProject != "" {
		_, selected, err := replicapublication.ProjectFingerprint(runtime.apiBaseURL, replicaPublicationMarket(runtime), runtime.replicaProject)
		if err != nil {
			return replicapublication.Store{}, nil, err
		}
		if selected != project {
			return replicapublication.Store{}, nil, output.Validation("REPLICA_STORAGE_PROJECT_MISMATCH", "--state-project must identify the same source as --path")
		}
		runtime.replicaProject = selected
	}
	if err := replicaBindingStore(runtime).Preflight(project); err != nil {
		return replicapublication.Store{}, nil, err
	}
	projectLock, err := projectStore.Lock(fingerprint)
	if err != nil {
		return replicapublication.Store{}, nil, err
	}
	unlock = projectLock.Unlock
	defer func() {
		if returnErr != nil {
			returnErr = errors.Join(returnErr, unlock())
		}
	}()
	globalStore := replicaPublicationStore(runtime)
	// Keep the legacy lock too: older clients use it without the project lock.
	// Locking is supported by the reported sandbox; no global rename is needed.
	globalLock, err := globalStore.Lock(fingerprint)
	if err != nil {
		return replicapublication.Store{}, unlock, err
	}
	unlock = func() error { return errors.Join(globalLock.Unlock(), projectLock.Unlock()) }
	_, globalFound, err := globalStore.LoadProject(fingerprint)
	if err != nil {
		return replicapublication.Store{}, unlock, err
	}
	_, projectFound, err := projectStore.LoadProject(fingerprint)
	if err != nil {
		return replicapublication.Store{}, unlock, err
	}
	if globalFound && projectFound {
		return replicapublication.Store{}, unlock, replicaStorageConflict()
	}
	if globalFound && runtime.replicaProject != "" {
		return replicapublication.Store{}, unlock, replicaStorageConflict()
	}
	store := globalStore
	if projectFound || runtime.replicaProject != "" {
		store = projectStore
		runtime.replicaProject = project
	}
	if err := store.Preflight(); err != nil {
		return replicapublication.Store{}, unlock, err
	}
	return store, unlock, nil
}

func replicaStorageConflict() error {
	return output.Policy("REPLICA_PUBLICATION_STORAGE_CONFLICT", "an existing publication request must remain in its original recovery store").
		WithHint("resume or cancel the original request using its original storage selection; do not migrate files or create another publication")
}

func replicaStorageCommand(runtime *Runtime, command string) string {
	if runtime.replicaProject != "" {
		command += " --state-project " + shellQuote(runtime.replicaProject)
	}
	profile := runtime.opts.profile
	if profile == "" && runtime.replicaProject != "" {
		profile = runtime.profile.Name
	}
	if profile != "" {
		command += " --profile " + shellQuote(profile)
	}
	return command
}

func presentStoredReplicaPublication(runtime *Runtime, publication api.WebsiteReplicaPublication) replicaPublicationPresentation {
	result := presentReplicaPublication(publication)
	result.Resume.Command = replicaStorageCommand(runtime, result.Resume.Command)
	return result
}

func preflightReplicaRecovery(runtime *Runtime, store replicapublication.Store, pending replicapublication.Pending) error {
	if runtime.replicaProject != "" && filepath.Clean(runtime.replicaProject) != pending.ProjectPath {
		return replicaStorageConflict()
	}
	if err := store.Preflight(); err != nil {
		return err
	}
	return replicaBindingStore(runtime).Preflight(pending.ProjectPath)
}

// Merge recovery details instead of discarding the storage failure stage.
func mergeReplicaRecoveryDetails(err *output.Error, details map[string]any) {
	merged := map[string]any{}
	if existing, ok := err.Details.(map[string]any); ok {
		for key, value := range existing {
			merged[key] = value
		}
	}
	for key, value := range details {
		merged[key] = value
	}
	err.Details = merged
	if strings.HasPrefix(err.Subtype, "REPLICA_PUBLICATION_STORAGE_") {
		err.Retryable = false
	}
}

// Control commands acquire the same project -> legacy lock order as publish.
// Re-read after locking so a concurrent command cannot drive stale state.
func loadReplicaRecovery(runtime *Runtime, store replicapublication.Store, publicationID string) (_ replicapublication.Pending, found bool, unlock func() error, returnErr error) {
	pending, found, err := store.LoadPublication(publicationID)
	if err != nil {
		return pending, false, func() error { return nil }, err
	}
	if runtime.replicaProject != "" {
		_, globalFound, globalErr := replicaPublicationStore(runtime).LoadPublication(publicationID)
		if globalErr != nil {
			return pending, false, func() error { return nil }, globalErr
		}
		if globalFound {
			return pending, false, func() error { return nil }, replicaStorageConflict()
		}
	}
	if !found {
		return pending, false, func() error { return nil }, nil
	}
	projectStore, err := projectReplicaPublicationStore(runtime, pending.ProjectPath)
	if err != nil {
		return pending, false, func() error { return nil }, err
	}
	localLock, err := projectStore.Lock(pending.ProjectFingerprint)
	if err != nil {
		return pending, false, func() error { return nil }, err
	}
	unlock = localLock.Unlock
	defer func() {
		if returnErr != nil {
			returnErr = errors.Join(returnErr, unlock())
		}
	}()
	globalLock, err := replicaPublicationStore(runtime).Lock(pending.ProjectFingerprint)
	if err != nil {
		return pending, false, unlock, err
	}
	unlock = func() error { return errors.Join(globalLock.Unlock(), localLock.Unlock()) }
	pending, found, err = store.LoadPublication(publicationID)
	if err != nil || !found {
		return pending, found, unlock, err
	}
	if err := preflightReplicaRecovery(runtime, store, pending); err != nil {
		return pending, false, unlock, err
	}
	return pending, true, unlock, nil
}
