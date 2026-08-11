package command

import (
	"errors"
	"regexp"
	"testing"
	"time"
)

type failingEntropyReader struct{}

func (failingEntropyReader) Read([]byte) (int, error) {
	return 0, errors.New("entropy unavailable")
}

func TestUUIDFallbackStillProducesAValidVersionFourUUID(t *testing.T) {
	t.Parallel()
	value := uuidFromEntropy(failingEntropyReader{}, time.Unix(1_700_000_000, 42), 123, 7)
	pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !pattern.MatchString(value) {
		t.Fatalf("fallback returned an invalid UUID: %q", value)
	}
}
