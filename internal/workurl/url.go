package workurl

import (
	"net/url"
	"regexp"
	"strings"
)

var workURLHandlePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
var workURLSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// PublicParts owns the CLI public Work route contract. Both /handle/slug and
// /handle?workSlug=slug identify consumer/work. Explicit values never get ignored.
// Product and install selectors remain on the original URL for their owning flows.
func PublicParts(parsed *url.URL) (string, string, bool) {
	if parsed == nil || parsed.User != nil || parsed.Opaque != "" ||
		(parsed.IsAbs() && ((parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "")) ||
		(!parsed.IsAbs() && parsed.Host != "") {
		return "", "", false
	}
	path := strings.TrimSuffix(parsed.Path, ".md")
	if !strings.HasPrefix(path, "/") || parsed.EscapedPath() != parsed.Path {
		return "", "", false
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 1 || len(parts) > 2 {
		return "", "", false
	}
	handle := parts[0]
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
	if len(parts) == 2 {
		if query.Has("workSlug") && slug != parts[1] {
			return "", "", false
		}
		slug = parts[1]
	}
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
	handle, slug, ok := PublicParts(parsed)
	if !ok {
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
	markdown := strings.HasSuffix(parsed.Path, ".md")
	parsed.Path = "/" + handle + "/" + slug
	if markdown {
		parsed.Path += ".md"
	}
	parsed.RawPath = ""
	// Retain all other query bytes (including encoded product values and order).
	parts := strings.Split(parsed.RawQuery, "&")
	kept := parts[:0]
	for _, part := range parts {
		key, _, _ := strings.Cut(part, "=")
		key, _ = url.QueryUnescape(key)
		if key != "mode" && key != "view" && key != "workSlug" {
			kept = append(kept, part)
		}
	}
	parsed.RawQuery = strings.Join(kept, "&")
	return parsed.String()
}

// Equivalent compares public identity across URL representations without changing
// frozen confirmation or recovery records. Unknown protocol fields stay opaque.
func Equivalent(left, right string) bool {
	if left == right {
		return true
	}
	a, errA := url.Parse(left)
	b, errB := url.Parse(right)
	if errA != nil || errB != nil || !a.IsAbs() || !b.IsAbs() || a.Scheme != b.Scheme || a.Host != b.Host || a.Fragment != "" || b.Fragment != "" {
		return false
	}
	ah, as, aok := PublicParts(a)
	bh, bs, bok := PublicParts(b)
	if !aok || !bok || ah != bh || as != bs || strings.HasSuffix(a.Path, ".md") != strings.HasSuffix(b.Path, ".md") {
		return false
	}
	remaining := func(u *url.URL) (string, bool) {
		q := u.Query()
		for k := range q {
			switch k {
			case "mode", "view", "workSlug":
				q.Del(k)
			case "product", "install":
			default:
				return "", false
			}
		}
		return q.Encode(), true
	}
	aq, aok := remaining(a)
	bq, bok := remaining(b)
	return aok && bok && aq == bq
}
