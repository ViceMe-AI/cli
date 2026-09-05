package api

import (
	"context"
	"strings"
	"testing"
)

func TestWebsiteReplicaSalesStrictResponse(t *testing.T) {
	for name, mutate := range map[string]func(map[string]any){
		"missing permission":  func(s map[string]any) { delete(s, "operationsEnabled") },
		"unknown field":       func(s map[string]any) { s["secret"] = "unexpected" },
		"wrong target":        func(s map[string]any) { s["replicaId"] = s["workId"] },
		"invalid digest":      func(s map[string]any) { s["replicaVersion"].(map[string]any)["digest"] = "bad" },
		"long title":          func(s map[string]any) { s["replicaVersion"].(map[string]any)["title"] = strings.Repeat("a", 201) },
		"inconsistent status": func(s map[string]any) { s["saleStatus"] = "DELISTED" },
		"missing price":       func(s map[string]any) { delete(s["product"].(map[string]any), "priceCents") },
		"negative price":      func(s map[string]any) { s["product"].(map[string]any)["priceCents"] = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			state := replicaSalesFixture()
			mutate(state)
			err := callWebsiteReplicaResponse(t, state, func(c *Client) error {
				_, err := c.GetWebsiteReplicaSales(context.Background(), "11111111-1111-4111-8111-111111111111")
				return err
			})
			assertInvalidWebsiteReplicaResponse(t, err)
		})
	}
}
func replicaSalesFixture() map[string]any {
	return map[string]any{
		"replicaId": "11111111-1111-4111-8111-111111111111", "workId": "33333333-3333-4333-8333-333333333333", "saleStatus": "ACTIVE", "operationsEnabled": true,
		"replicaVersion": map[string]any{"id": "44444444-4444-4444-8444-444444444444", "version": 2, "title": "Website source", "digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		"product":        map[string]any{"id": "55555555-5555-4555-8555-555555555555", "revision": 3, "status": "ACTIVE", "salesSpecVersionId": "66666666-6666-4666-8666-666666666666", "salesSpecVersion": 2, "skuId": "77777777-7777-4777-8777-777777777777", "currency": "CNY", "priceCents": 800},
	}
}
