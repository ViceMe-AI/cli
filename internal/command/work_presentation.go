package command

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/workurl"
)

type publicWorkPresentation struct {
	api.PublicWorkProjection
	WorkURL     string `json:"workUrl"`
	MarkdownURL string `json:"markdownUrl"`
}

func presentPublicWork(work api.PublicWorkProjection, target, webBaseURL string) publicWorkPresentation {
	path := work.Work.CanonicalPath
	// Record selectors belong to the caller's requested Work, not a default edition.
	if requested, err := url.Parse(target); err == nil {
		canonical, _ := url.Parse(path)
		handle, slug, ok := workurl.PublicParts(requested)
		expectedHandle, expectedSlug, expectedOK := workurl.PublicParts(canonical)
		if ok && expectedOK && handle == expectedHandle && slug == expectedSlug {
			path = target
		}
	}
	html, err := url.Parse(path)
	if err != nil {
		return publicWorkPresentation{PublicWorkProjection: work}
	}
	if !html.IsAbs() {
		base, err := url.Parse(webBaseURL)
		if err == nil {
			html = base.ResolveReference(html)
		}
	}
	html.Path = strings.TrimSuffix(html.Path, ".md")
	html.RawPath = ""
	markdown := *html
	markdown.Path += ".md"
	return publicWorkPresentation{PublicWorkProjection: work, WorkURL: workurl.Display(html.String()), MarkdownURL: workurl.Display(markdown.String())}
}

// Product detail is an extensible API document. Preserve each raw field and add
// presentation URLs only from its existing canonical identity and selected ID.
func presentSkillDetail(raw json.RawMessage, productID, webBaseURL string) json.RawMessage {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return raw
	}
	var path string
	if json.Unmarshal(fields["canonicalPath"], &path) != nil {
		return raw
	}
	parsed, err := url.Parse(path)
	if err != nil {
		return raw
	}
	if _, _, ok := workurl.PublicParts(parsed); !ok {
		return raw
	}
	if !parsed.IsAbs() {
		base, err := url.Parse(webBaseURL)
		if err == nil {
			parsed = base.ResolveReference(parsed)
		}
	}
	query := parsed.Query()
	var packageType string
	if json.Unmarshal(fields["assetPackageType"], &packageType) == nil && packageType != "" {
		query.Del("product")
	} else {
		query.Set("product", productID)
	}
	parsed.RawQuery = query.Encode()
	parsed.Path = strings.TrimSuffix(parsed.Path, ".md")
	fields["workUrl"], _ = json.Marshal(workurl.Display(parsed.String()))
	parsed.Path += ".md"
	fields["markdownUrl"], _ = json.Marshal(workurl.Display(parsed.String()))
	result, err := json.Marshal(fields)
	if err != nil {
		return raw
	}
	return result
}
