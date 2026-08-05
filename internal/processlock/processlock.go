package processlock

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

var (
	ErrUnavailable  = errors.New("process lock unavailable")
	lockNamePattern = regexp.MustCompile(`^[a-z0-9-]+$`)
)

// With serializes one complete operation across processes for the same
// namespace and identity. Lock files contain only a hash of the identity.
func With(ctx context.Context, root, namespace, identity string, action func() error) error {
	if strings.TrimSpace(root) == "" ||
		!lockNamePattern.MatchString(namespace) ||
		strings.TrimSpace(identity) == "" ||
		action == nil {
		return fmt.Errorf("%w: invalid lock configuration", ErrUnavailable)
	}
	lockDirectory := filepath.Join(root, "locks")
	if err := os.MkdirAll(lockDirectory, 0o700); err != nil {
		return fmt.Errorf("%w: create %s lock directory: %v", ErrUnavailable, namespace, err)
	}
	digest := sha256.Sum256([]byte(namespace + "\x00" + identity))
	fileLock := flock.New(filepath.Join(lockDirectory, fmt.Sprintf("%s-%x.lock", namespace, digest[:])))
	locked, err := fileLock.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil || !locked {
		return fmt.Errorf("%w: acquire %s lock: %v", ErrUnavailable, namespace, err)
	}
	var actionErr error
	var unlockErr error
	func() {
		defer func() { unlockErr = fileLock.Unlock() }()
		actionErr = action()
	}()
	if actionErr != nil {
		return actionErr
	}
	if unlockErr != nil {
		return fmt.Errorf("%w: release %s lock: %v", ErrUnavailable, namespace, unlockErr)
	}
	return nil
}
