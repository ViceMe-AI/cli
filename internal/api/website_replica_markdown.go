package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"time"
	"unicode/utf8"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/workurl"
)

// ReadWebsiteReplicaOwnerMarkdown uses only the selected profile's Web origin.
// Markdown intentionally falls back to a public view for non-owners, so verify
// ownership at the private API before offering it as an analytics document.
func (c *Client) ReadWebsiteReplicaOwnerMarkdown(ctx context.Context, webBaseURL, workURL string) (string, string, error) {
	base, err := config.NormalizeWebBaseURL(webBaseURL)
	if err != nil {
		return "", "", output.Validation("REPLICA_WEB_AUTHORITY_REQUIRED", "select a profile with a matching Web base URL")
	}
	target, err := url.Parse(workURL)
	if err != nil || target.Scheme+"://"+target.Host != base || target.Fragment != "" {
		return "", "", output.Validation("REPLICA_WORK_URL_INVALID", "Work URL must belong to the selected profile's Web authority")
	}
	handle, slug, ok := workurl.PublicParts(target)
	if !ok {
		return "", "", output.Validation("REPLICA_WORK_URL_INVALID", "use /<handle>?workSlug=<slug> with public Work parameters")
	}
	work, err := c.GetPublicWork(ctx, handle, slug)
	if err != nil {
		return "", "", err
	}
	if !uuidPattern.MatchString(work.Work.ID) || work.Work.Kind != "WEBSITE" || work.Creator.Handle != handle || work.Work.Slug != slug {
		return "", "", invalidAPIResponse(errors.New("analytics Work target mismatch"))
	}
	var access struct {
		WorkID string `json:"workId"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/v1/website-replicas/owner-analytics/works/"+url.PathEscape(work.Work.ID), nil, &access, "@stored"); err != nil {
		return "", "", err
	}
	if access.WorkID != work.Work.ID {
		return "", "", invalidAPIResponse(errors.New("analytics ownership target mismatch"))
	}
	credential, err := c.Tokens.Token(ctx)
	if err != nil {
		return "", "", err
	}
	// Ownership was checked above. Explicit creator mode selects the private
	// Markdown document; public defaults must never stand in for owner data.
	markdownTarget, _ := url.Parse(base)
	markdownTarget.Path = "/" + handle + ".md"
	markdownTarget.RawQuery = url.Values{"mode": {"creator"}, "view": {"work"}, "workSlug": {slug}}.Encode()
	markdownURL := markdownTarget.String()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, markdownURL, nil)
	if err != nil {
		return "", "", output.Validation("REPLICA_WORK_URL_INVALID", "could not construct the Work Markdown URL")
	}
	request.Header.Set("Authorization", "Bearer "+credential)
	request.Header.Set("Accept", "text/markdown")
	request.Header.Set("Cache-Control", "no-cache, no-store")
	request.Header.Set("Pragma", "no-cache")
	request.Header.Set("User-Agent", c.UserAgent)
	client := withoutRedirects(c.HTTPClient)
	if client.Timeout <= 0 || client.Timeout > 30*time.Second {
		client.Timeout = 30 * time.Second
	}
	response, err := client.Do(request)
	if err != nil {
		return "", "", output.Network("REPLICA_MARKDOWN_UNREACHABLE", "could not read the current Work Markdown", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return "", "", output.Network("REPLICA_MARKDOWN_READ_FAILED", "could not read the Work Markdown response", err)
	}
	if response.StatusCode != http.StatusOK {
		return "", "", decodeServerError(response.StatusCode, data, response.Header.Get("X-Request-Id"))
	}
	contentType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if contentType != "text/markdown" || len(data) > maxResponseBytes || len(bytes.TrimSpace(data)) == 0 || !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return "", "", invalidAPIResponse(errors.New("Work Markdown response is invalid"))
	}
	return markdownURL, string(data), nil
}
