package command

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ViceMe-AI/cli/internal/api"
)

func TestReplicaPublicationDisplayPreservesAuthoritativeURL(t *testing.T) {
	const canonical = "https://viceme.cn/alice?mode=consumer&view=work&workSlug=site"
	const private = "https://viceme.cn/alice?mode=creator&view=publication&publicationId=one"
	source := api.WebsiteReplicaPublication{Status: "PUBLISHED", StatusURL: private, Result: &api.WebsiteReplicaPublicationResult{WorkURL: canonical}}
	for range 2 {
		result := presentReplicaPublication(source)
		if result.Result.WorkURL != "https://viceme.cn/alice/site" || result.StatusURL != private {
			t.Fatalf("wrong display: %#v", result)
		}
		if source.Result.WorkURL != canonical || source.Result == result.Result {
			t.Fatal("presentation mutated canonical source")
		}
	}
	view := newReplicaWorkPresentation(true, canonical)
	if view.URL != "https://viceme.cn/alice/site" {
		t.Fatalf("wrong preview: %#v", view)
	}
}

func TestPublicWorkDisplayPreservesEditionAndCanonical(t *testing.T) {
	source := api.PublicWorkProjection{}
	source.Work.CanonicalPath = "/alice?mode=consumer&view=work&workSlug=site"
	source.Work.MarkdownPath = "/alice.md?mode=consumer&view=work&workSlug=site"
	target := "https://dev.viceme.cn/alice.md?mode=consumer&view=work&workSlug=site&product=one&install=owned"
	result := presentPublicWork(source, target, "https://viceme.ai")
	if result.WorkURL != "https://dev.viceme.cn/alice/site?product=one&install=owned" || result.MarkdownURL != "https://dev.viceme.cn/alice/site.md?product=one&install=owned" {
		t.Fatalf("lost display selectors: %#v", result)
	}
	if source.Work.CanonicalPath != result.Work.CanonicalPath || source.Work.MarkdownPath != result.Work.MarkdownPath {
		t.Fatal("canonical output changed")
	}
	if got := presentPublicWork(source, "/other/other", "https://viceme.ai"); got.WorkURL != "https://viceme.ai/alice/site" {
		t.Fatal("different Work replaced source identity")
	}
}

func TestProductDetailDisplaysSelectedEditionAndRetainsAPIFields(t *testing.T) {
	const canonical = "/alice?mode=consumer&view=work&workSlug=site"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/skills/"+downloadableProductID {
			t.Errorf("unexpected API: %s", r.URL.Path)
		}
		writeJSONResponse(w, map[string]any{"id": downloadableProductID, "canonicalPath": canonical, "unknown": map[string]any{"keep": "exact"}})
	}))
	defer server.Close()
	code, result := executeSkillUseCommand(t, server, t.TempDir(), "skill", "detail", downloadableProductID)
	if code != 0 {
		t.Fatalf("detail failed: %#v", result)
	}
	data := result["data"].(map[string]any)
	if data["canonicalPath"] != canonical || data["unknown"].(map[string]any)["keep"] != "exact" {
		t.Fatal("raw API identity/fields changed")
	}
	if data["workUrl"] != "https://viceme.cn/alice/site?product="+downloadableProductID+"" || data["markdownUrl"] != "https://viceme.cn/alice/site.md?product="+downloadableProductID+"" {
		t.Fatalf("wrong selected edition URLs: %#v", data)
	}
}
