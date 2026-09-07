package api

import (
	"context"
	"testing"
)

func TestWebsiteReplicaOrderStatusAcceptsCurrentShopContract(t *testing.T) {
	calls := map[string]func(*Client) error{
		"account": func(client *Client) error {
			_, err := client.GetWebsiteReplicaOrderStatus(context.Background(), testOrderNo)
			return err
		},
		"anonymous": func(client *Client) error {
			_, err := client.GetWebsiteReplicaSessionOrderStatus(context.Background(), testReplicaID, "test-token", testOrderNo)
			return err
		},
		"paid recovery": func(client *Client) error {
			_, err := client.RecoverWebsiteReplicaOrderStatus(context.Background(), RecoverWebsiteReplicaDownloadRequest{OrderNo: testOrderNo, RecoverySecret: "test-recovery-secret"})
			return err
		},
	}
	for name, call := range calls {
		for _, status := range []string{"PENDING", "PAID"} {
			t.Run(name+"/"+status, func(t *testing.T) {
				// Shop removed serviceCase from the canonical order-status contract.
				response := replicaOrderStatusResponse()
				delete(response, "serviceCase")
				if status == "PENDING" {
					response["payment"] = map[string]any{"status": status, "paidAt": nil, "closedAt": nil}
					response["fulfillment"] = nil
				}
				if err := callWebsiteReplicaResponse(t, response, call); err != nil {
					t.Fatalf("current Shop order status must be accepted: %v", err)
				}
			})
		}
	}
}
