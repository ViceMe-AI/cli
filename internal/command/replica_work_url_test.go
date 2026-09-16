package command

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ViceMe-AI/cli/internal/api"
)

func TestReplicaWorkURLContract(t *testing.T) {
	for _, test := range []struct {
		path  string
		valid bool
	}{
		{"/alice/site", false},
		{"/en-US/alice/site.md", false},
		{"/alice?mode=consumer&view=work&workSlug=site", true},
		{"/zh-CN/alice?workSlug=site&view=work&mode=consumer", false},
		{"/alice?mode=consumer&view=work&workSlug=other", false},
		{"/alice?mode=creator&view=work&workSlug=site", false},
		{"/alice?mode=consumer&view=profile&workSlug=site", false},
		{"/alice?view=work&workSlug=site", true},
		{"/alice.md?workSlug=site", true},
		{"/alice?mode=consumer&workSlug=site", true},
		{"/alice?mode=consumer&view=work&workSlug=site&workSlug=other", false},
		{"/alice?mode=consumer&mode=creator&view=work&workSlug=site", false},
		{"/alice?mode=consumer&view=work&workSlug=", false},
		{"/alice/site?workSlug=other", false},
		{"/alice%2fextra/site", false},
		{"/alice?mode=consumer&view=work&workSlug=site%2fother", false},
		{"/alice?mode=consumer&view=work&workSlug=..", false},
		{"/alice?mode=consumer&view=work&workSlug=site%ZZ", false},
	} {
		t.Run(test.path, func(t *testing.T) {
			workURL := "https://viceme.cn" + test.path
			target := api.WebsiteReplicaPublicationResolvedTarget{Resolution: "CREATE", MerchantAccountID: replicaPublicationTestMerchantID, WorkURL: workURL}
			review := &api.WebsiteReplicaPublicationConfirmationChallenge{Review: api.WebsiteReplicaPublicationReview{Resolution: target.Resolution, MerchantAccountID: target.MerchantAccountID, WorkURL: workURL}}
			request := api.CreateWebsiteReplicaPublicationRequest{Target: api.WebsiteReplicaPublicationTarget{Kind: "NEW_WORK", Slug: "site"}}
			if got := replicaResolvedTargetMatchesRequest(target, review, request); got != test.valid {
				t.Fatalf("target match = %t, want %t", got, test.valid)
			}
			if test.valid {
				parsed, _ := url.Parse(workURL)
				handle, slug, ok := replicaWorkURLParts(parsed)
				if !ok || handle != "alice" || slug != "site" {
					t.Fatalf("wrong identity: %q %q %t", handle, slug, ok)
				}
				target.MerchantAccountID = replicaPublicationTestCreatorID
				if replicaResolvedTargetMatchesRequest(target, review, request) {
					t.Fatal("changed merchant accepted")
				}
				target.MerchantAccountID = review.Review.MerchantAccountID
				target.WorkURL = "https://viceme.cn/other/site"
				if replicaResolvedTargetMatchesRequest(target, review, request) {
					t.Fatal("changed review URL accepted")
				}
			}
		})
	}
}

func TestReplicaInspectQueryWorkURLResolvesThroughAPI(t *testing.T) {
	reads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/public/creators/alice/works/site" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
			return
		}
		reads++
		writeJSONResponse(w, map[string]any{"work": map[string]any{}})
	}))
	defer server.Close()
	run := analyticsTestRunner(t, server)
	for _, path := range []string{"/alice.md?workSlug=site", "/alice?mode=consumer&view=work&workSlug=site", "/alice?mode=consumer&workSlug=site", "/alice?view=work&workSlug=site"} {
		_, out := run("replica", "inspect", server.URL+path)
		if !strings.Contains(string(out), "REPLICA_WORK_HAS_NO_ENTRY") {
			t.Fatalf("failed to reach authoritative Work: %s", out)
		}
	}
	if reads != 4 {
		t.Fatalf("reads=%d", reads)
	}
}
