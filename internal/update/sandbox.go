package update

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/ViceMe-AI/cli/internal/privatefile"
)

// ErrRenameDenied reports that the current environment denies rename
// operations on the ViceMe configuration directory. Agent sandboxes with this
// behavior (observed: WorkBuddy's Seatbelt profile allows plain file creation
// and writes outside the session workspace but denies rename and unlink)
// cannot atomically replace the CLI executable or activate transactional
// Skill updates in place.
var ErrRenameDenied = errors.New("this environment denies the file renames required to activate a new ViceMe CLI generation")

const (
	activationProbeStaged    = ".viceme-activation-probe"
	activationProbeActivated = ".viceme-activation-probe-activated"
)

// ProbeRenameCapability verifies that this process can stage and rename a
// harmless file inside directory. The automatic update path uses it to skip
// update attempts that cannot complete, and the explicit update command uses
// it to fail fast with actionable guidance.
//
// The probe keeps at most one staged file behind in sandboxes that also deny
// its removal: the names are fixed, later probes truncate the same file, and
// an environment that later allows renames cleans it up on the next probe.
func ProbeRenameCapability(directory string) error {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	staged := filepath.Join(directory, activationProbeStaged)
	file, err := os.OpenFile(staged, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write([]byte("viceme activation rename probe")); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := privatefile.RenameFile(staged, filepath.Join(directory, activationProbeActivated)); err != nil {
		if privatefile.IsPermissionDenial(err) {
			return ErrRenameDenied
		}
		return err
	}
	return os.Remove(filepath.Join(directory, activationProbeActivated))
}
