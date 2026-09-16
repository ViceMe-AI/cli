package command

import (
	"net/url"
	"strings"
)

// replicaWorkURLParts accepts the public query route and legacy Work paths.
// Callers still resolve identity through the API or the exact confirmed URL.
func replicaWorkURLParts(parsed *url.URL) (string, string, bool) {
	if parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || parsed.Opaque != "" {
		return "", "", false
	}
	if strings.Contains(strings.ToLower(parsed.EscapedPath()), "%2f") {
		return "", "", false
	}
	segments := strings.Split(strings.Trim(strings.TrimSuffix(parsed.Path, ".md"), "/"), "/")
	if len(segments) > 1 && (segments[0] == "zh-CN" || segments[0] == "en-US") {
		segments = segments[1:]
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return "", "", false
	}
	var handle, slug string
	switch len(segments) {
	case 1:
		if len(query["mode"]) != 1 || query.Get("mode") != "consumer" || len(query["view"]) != 1 || query.Get("view") != "work" || len(query["workSlug"]) != 1 {
			return "", "", false
		}
		handle, slug = segments[0], query.Get("workSlug")
	case 2:
		if query.Has("workSlug") {
			return "", "", false
		}
		handle, slug = segments[0], segments[1]
	default:
		return "", "", false
	}
	for _, part := range []string{handle, slug} {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, "/\\?# \t\r\n") {
			return "", "", false
		}
	}
	return handle, slug, true
}
