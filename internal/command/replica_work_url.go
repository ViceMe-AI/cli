package command

import (
	"net/url"

	"github.com/ViceMe-AI/cli/internal/workurl"
)

// Replica inputs and confirmed publication URLs must be absolute public Work URLs.
func replicaWorkURLParts(parsed *url.URL) (string, string, bool) {
	if parsed == nil || !parsed.IsAbs() || parsed.Hostname() == "" {
		return "", "", false
	}
	return workurl.PublicParts(parsed)
}
