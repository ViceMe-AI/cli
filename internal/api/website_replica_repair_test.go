package api

import (
	"context"
	"strings"
	"testing"
)

func TestReplicaPageRepairRejectsMalformedOrMismatchedResponses(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	input := WebsiteReplicaPageRepairRequest{ClientRequestID: id, Page: WebsiteReplicaPublicationSourceArtifact{FileName: "page.zip", ContentType: "application/zip", SizeBytes: 123, Digest: strings.Repeat("a", 64)}}
	for name, mutate := range map[string]func(map[string]any){
		"missing page":             func(r map[string]any) { delete(r, "page") },
		"wrong target":             func(r map[string]any) { r["publicationId"] = "22222222-2222-4222-8222-222222222222" },
		"unknown credential":       func(r map[string]any) { r["token"] = "unexpected" },
		"published without result": func(r map[string]any) { r["status"] = "PUBLISHED" },
		"wrong artifact":           func(r map[string]any) { r["page"].(map[string]any)["digest"] = strings.Repeat("b", 64) },
		"missing failure":          func(r map[string]any) { r["status"] = "FAILED" },
	} {
		t.Run(name, func(t *testing.T) {
			r := map[string]any{"id": id, "publicationId": id, "clientRequestId": id, "status": "WAITING_UPLOAD", "page": map[string]any{"fileName": "page.zip", "contentType": "application/zip", "sizeBytes": 123, "digest": strings.Repeat("a", 64), "status": "WAITING_UPLOAD", "verifiedAt": nil}, "failure": nil, "result": nil, "createdAt": "2026-09-05T00:00:00Z", "updatedAt": "2026-09-05T00:00:00Z"}
			mutate(r)
			err := callWebsiteReplicaResponse(t, r, func(c *Client) error {
				_, err := c.CreateWebsiteReplicaPageRepair(context.Background(), id, input)
				return err
			})
			assertInvalidWebsiteReplicaResponse(t, err)
		})
	}
}
