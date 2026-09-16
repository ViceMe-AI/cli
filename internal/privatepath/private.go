package privatepath

import "errors"

// ACLMismatchError reports that a path exists and is readable by this process,
// but its permission profile does not match the expected owner-only shape.
// The most common cause on end-user machines is security software that
// injects its own access entry into newly created files; the path is then
// still writable, only the hardened profile check refuses it. Callers writing
// non-credential state may degrade to a plain write; credential stores must
// keep failing closed on this error.
type ACLMismatchError struct {
	Path   string
	Reason string
}

func (e *ACLMismatchError) Error() string {
	return e.Path + ": permission profile mismatch: " + e.Reason
}

// IsACLMismatch reports whether err describes an unexpected permission
// profile rather than an I/O failure.
func IsACLMismatch(err error) bool {
	var mismatch *ACLMismatchError
	return errors.As(err, &mismatch)
}
