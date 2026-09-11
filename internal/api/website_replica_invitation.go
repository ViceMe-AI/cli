package api

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// Invitation analytics use separate endpoints so old servers and failed reports
// cannot change the purchase request or its response contract.
func (c *Client) RecordReplicaInvitation(ctx context.Context, flowID, shortCode, version string) bool {
	return c.replicaInvitationEvent(ctx, "/v1/website-replica-invitation-flows", map[string]string{"flowId": flowID, "shortCode": shortCode, "engine": "CLI", "clientVersion": version}, "")
}

func (c *Client) ReportReplicaInvitation(ctx context.Context, flowID, event string) bool {
	return c.replicaInvitationEvent(ctx, "/v1/website-replica-invitation-flows/events", map[string]string{"flowId": flowID, "event": event}, "")
}

func (c *Client) AssociateReplicaInvitation(ctx context.Context, flowID, orderNo, sessionID, token string) bool {
	endpoint := "/v1/website-replicas/orders/" + url.PathEscape(orderNo) + "/invitation-flow"
	credential := "@stored"
	if sessionID != "" {
		endpoint = "/v1/website-replica-sessions/" + url.PathEscape(sessionID) + "/orders/" + url.PathEscape(orderNo) + "/invitation-flow"
		credential = token
	}
	return c.replicaInvitationEvent(ctx, endpoint, map[string]string{"flowId": flowID}, credential)
}

func (c *Client) replicaInvitationEvent(ctx context.Context, endpoint string, body any, credential string) bool {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	var response struct {
		Recorded bool `json:"recorded"`
	}
	return c.doJSON(ctx, http.MethodPost, endpoint, body, &response, credential) == nil && response.Recorded
}
