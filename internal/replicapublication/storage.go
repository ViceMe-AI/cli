package replicapublication

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/privatefile"
	"github.com/ViceMe-AI/cli/internal/privatepath"
	"github.com/gofrs/flock"
)

// ProjectStoreDirectory resolves only the managed publication directory. It
// never changes credential storage or follows a project-controlled symlink.
func ProjectStoreDirectory(projectPath, endpoint, market string) (string, error) {
	root := filepath.Dir(bindingPath(projectPath))
	for _, directory := range []string{root, filepath.Join(root, "publications")} {
		if err := privatepath.RequirePrivateDirectory(directory); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", StorageError("STORAGE_SELECTION", "inspect", err)
		}
	}
	return ScopedDirectory(filepath.Join(root, "publications"), endpoint, market), nil
}

// Preflight verifies the operations required by the entire recovery lifecycle,
// using only random private probe files. Actual writes must still fail closed.
func (store Store) Preflight() error {
	if err := store.ensureDirectory(); err != nil {
		return err
	}
	if err := ProbeDirectory(store.Directory); err != nil {
		return err
	}
	if store.ProjectScoped {
		if err := privatefile.WriteAtomic(filepath.Join(store.Directory, ".gitignore"), []byte("*\n"), ".ignore-*.tmp"); err != nil {
			return StorageError("STORAGE_PREFLIGHT", "protect_git", err)
		}
	}
	return nil
}

func (store BindingStore) Preflight(projectPath string) error {
	directory := filepath.Dir(bindingPath(projectPath))
	if err := ensureBindingDirectory(directory); err != nil {
		return StorageError("BINDING_PREFLIGHT", "create", err)
	}
	return ProbeDirectory(directory)
}

func ProbeDirectory(directory string) (returnErr error) {
	probe, err := privatepath.CreateTempDirectory(directory, ".publication-probe-*")
	if err != nil {
		return StorageError("STORAGE_PREFLIGHT", "create", err)
	}
	// Never sweep arbitrary user files or recursively remove a substituted path.
	defer func() {
		for _, name := range []string{"stage", "target", "lock"} {
			if err := os.Remove(filepath.Join(probe, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
				returnErr = errors.Join(returnErr, StorageError("STORAGE_PREFLIGHT", "unlink", err))
			}
		}
		if err := os.Remove(probe); err != nil {
			returnErr = errors.Join(returnErr, StorageError("STORAGE_PREFLIGHT", "rmdir", err))
		}
	}()
	write := func(name string) error {
		file, err := privatepath.CreateExclusiveFile(filepath.Join(probe, name))
		if err != nil {
			return StorageError("STORAGE_PREFLIGHT", "create", err)
		}
		_, writeErr := file.Write([]byte("publication storage probe\n"))
		syncErr := file.Sync()
		closeErr := file.Close()
		if writeErr != nil {
			return StorageError("STORAGE_PREFLIGHT", "write", writeErr)
		}
		if syncErr != nil {
			return StorageError("STORAGE_PREFLIGHT", "sync", syncErr)
		}
		if closeErr != nil {
			return StorageError("STORAGE_PREFLIGHT", "close", closeErr)
		}
		return nil
	}
	if err := write("stage"); err != nil {
		return err
	}
	if err := privatefile.ReplaceFile(filepath.Join(probe, "stage"), filepath.Join(probe, "target")); err != nil {
		return StorageError("STORAGE_PREFLIGHT", "rename", err)
	}
	if err := write("stage"); err != nil {
		return err
	}
	if err := privatefile.ReplaceFile(filepath.Join(probe, "stage"), filepath.Join(probe, "target")); err != nil {
		return StorageError("STORAGE_PREFLIGHT", "replace", err)
	}
	first := flock.New(filepath.Join(probe, "lock"))
	locked, err := first.TryLock()
	if err != nil {
		return StorageError("STORAGE_PREFLIGHT", "lock", err)
	}
	if !locked {
		return StorageError("STORAGE_PREFLIGHT", "lock", errors.New("probe lock unavailable"))
	}
	defer func() { returnErr = errors.Join(returnErr, first.Unlock()) }()
	second := flock.New(filepath.Join(probe, "lock"))
	other, err := second.TryLock()
	if other {
		_ = second.Unlock()
		return StorageError("STORAGE_PREFLIGHT", "lock", errors.New("probe lock does not exclude another handle"))
	}
	if err != nil {
		return StorageError("STORAGE_PREFLIGHT", "lock", err)
	}
	return nil
}

// StorageError exposes categories, never paths or raw OS error strings.
func StorageError(stage, operation string, err error) *output.Error {
	failure := output.Internal("REPLICA_PUBLICATION_STORAGE_FAILED", "Website Replica recovery storage is unavailable", err)
	reason := "IO_ERROR"
	if errors.Is(err, fs.ErrPermission) || privatefile.IsPermissionDenial(err) {
		failure = output.Policy("REPLICA_PUBLICATION_STORAGE_PERMISSION_REQUIRED", "ViceMe cannot safely read or persist private Website Replica publication state").WithCause(err)
		reason = "PERMISSION_DENIED"
	}
	if errors.Is(err, syscall.EROFS) {
		reason = "READ_ONLY_FILESYSTEM"
	}
	return failure.WithDetails(map[string]any{"stage": stage, "operation": operation, "reason": reason, "nextAction": "RESTORE_STORAGE_ACCESS", "automaticRetry": false}).
		WithHint("use the host's official approval mechanism or continue the same request in an authorized terminal; for a new request only, use --state-project with the source project to select workspace storage; do not retry unchanged, change permissions, bypass the host broker, or create a parallel publication")
}
