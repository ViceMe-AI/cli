package workurl

import (
	"net/url"
	"regexp"
	"strings"
)

var workURLHandlePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
var workURLSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// PublicParts owns the CLI public Work route contract. Missing mode/view
// mean consumer/work only when workSlug exists. Explicit values never get ignored.
// Product and install selectors remain on the original URL for their owning flows.
func PublicParts(parsed *url.URL) (string, string, bool) {
	if parsed == nil || parsed.User != nil || parsed.Opaque != "" ||
		(parsed.IsAbs() && ((parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "")) ||
		(!parsed.IsAbs() && parsed.Host != "") {
		return "", "", false
	}
	path := strings.TrimSuffix(parsed.Path, ".md")
	if !strings.HasPrefix(path, "/") || strings.Contains(path[1:], "/") || parsed.EscapedPath() != parsed.Path {
		return "", "", false
	}
	handle := strings.TrimPrefix(path, "/")
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return "", "", false
	}
	for _, values := range query {
		if len(values) != 1 {
			return "", "", false
		}
	}
	if (query.Has("mode") && query.Get("mode") != "consumer") ||
		(query.Has("view") && query.Get("view") != "work") {
		return "", "", false
	}
	for _, key := range []string{"action", "type", "panel", "analytics", "orderNo", "payoutId", "listingId", "editionKey", "workId", "productId", "entryId", "threadId", "inquiryId", "publicationId", "applicationId", "kind"} {
		if query.Has(key) {
			return "", "", false
		}
	}
	slug := query.Get("workSlug")
	if len(handle) < 2 || len(handle) > 32 || !workURLHandlePattern.MatchString(handle) ||
		len(slug) < 2 || len(slug) > 64 || !workURLSlugPattern.MatchString(slug) {
		return "", "", false
	}
	switch slug {
	case "works", "skills", "manage", "posts", "about":
		return "", "", false
	}
	return handle, slug, true
}

// Display returns a short public Work link for presentation only. It must not
// rewrite canonical, confirmation, recovery, or signed values in stored data.
func Display(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if _, _, ok := PublicParts(parsed); !ok {
		return raw
	}
	query := parsed.Query()
	// Unknown query fields may carry signatures or protocol state. Only shorten
	// the public-page contract; never rewrite arbitrary URLs or opaque payloads.
	for key := range query {
		switch key {
		case "mode", "view", "workSlug", "product", "install":
		default:
			return raw
		}
	}
	if !query.Has("mode") && !query.Has("view") {
		return raw
	}
	// Retain all other query bytes (including encoded product values and order).
	parts := strings.Split(parsed.RawQuery, "&")
	kept := parts[:0]
	for _, part := range parts {
		key, _, _ := strings.Cut(part, "=")
		key, _ = url.QueryUnescape(key)
		if key != "mode" && key != "view" {
			kept = append(kept, part)
		}
	}
	parsed.RawQuery = strings.Join(kept, "&")
	return parsed.String()
}
