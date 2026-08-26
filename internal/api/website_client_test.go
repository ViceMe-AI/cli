package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebsiteClientUsesPublishAndCoverAuthorizationEndpoints(t *testing.T) {
	t.Parallel()
	requests := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests <- request.Method + " " + request.URL.Path
		if request.Header.Get("Authorization") != "Bearer vme_cli_test" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/cli/sdk-works/cover-upload-authorizations":
			body, _ := io.ReadAll(request.Body)
			if !strings.Contains(string(body), `"clientWorkId":"11111111-1111-4111-8111-111111111111"`) {
				t.Fatalf("cover request = %s", body)
			}
			_, _ = io.WriteString(writer, `{"method":"PUT","url":"https://storage.example.com/cover","expiresAt":"2026-08-26T01:00:00Z","headers":{}}`)
		case "/v1/cli/sdk-works/publish":
			var body PublishCreatorWebsiteRequest
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			_, _ = io.WriteString(writer, `{"creatorWorkId":"22222222-2222-4222-8222-222222222222","workKey":"wrk_published_site","publication":{"clientWorkId":"11111111-1111-4111-8111-111111111111","sourceDigest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","sourceUrl":null,"descriptionZhCn":null,"descriptionEnUs":null,"coverUrl":null,"releaseId":"33333333-3333-4333-8333-333333333333","version":1,"publishedAt":"2026-08-26T00:00:00Z","unchanged":false},"displayName":"Dagou Tap","status":"DRAFT","configVersion":1,"offers":[],"features":[],"capabilities":[],"createdAt":"2026-08-26T00:00:00Z","updatedAt":"2026-08-26T00:00:00Z"}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client(), staticToken("vme_cli_test"), "viceme/test")
	cover := WebsiteCover{Digest: strings.Repeat("a", 64), SizeBytes: 100, FileName: "cover.png", ContentType: "image/png"}
	if _, err := client.AuthorizeWebsiteCoverUpload(context.Background(), AuthorizeWebsiteCoverUploadRequest{
		ClientWorkID: "11111111-1111-4111-8111-111111111111", WebsiteCover: cover,
	}); err != nil {
		t.Fatal(err)
	}
	work, err := client.PublishCreatorWebsite(context.Background(), PublishCreatorWebsiteRequest{
		ClientRequestID: "44444444-4444-4444-8444-444444444444",
		ClientWorkID:    "11111111-1111-4111-8111-111111111111",
		SourceDigest:    strings.Repeat("a", 64),
		DisplayName:     "Dagou Tap",
		Cover:           &cover,
	})
	if err != nil {
		t.Fatal(err)
	}
	if work.CreatorWorkID == nil || work.Publication == nil || work.Publication.Version != 1 {
		t.Fatalf("work = %#v", work)
	}
	if first, second := <-requests, <-requests; first != "POST /v1/cli/sdk-works/cover-upload-authorizations" || second != "POST /v1/cli/sdk-works/publish" {
		t.Fatalf("requests = %q, %q", first, second)
	}
}
