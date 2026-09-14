package api

import (
	"context"
	"testing"
)

func TestReplicaRedistributionGrantSchema(t *testing.T) {
	data := map[string]any{"items": []any{map[string]any{"entitlementId": testEntitlementID, "replicaId": testReplicaID, "versionId": testVersionID, "title": "Source", "canRedistribute": true}}, "nextCursor": nil}
	call := func(client *Client) error {
		_, err := client.GetWebsiteReplicaRedistributionGrants(context.Background(), "")
		return err
	}
	if err := callWebsiteReplicaResponse(t, data, call); err != nil {
		t.Fatal(err)
	}
	data["items"].([]any)[0].(map[string]any)["entitlementId"] = "invalid"
	if err := callWebsiteReplicaResponse(t, data, call); err == nil {
		t.Fatal("invalid entitlement accepted")
	}
}
