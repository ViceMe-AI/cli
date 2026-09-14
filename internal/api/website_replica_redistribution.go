package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
)

type WebsiteReplicaRedistributionGrant struct {
	EntitlementID   string `json:"entitlementId"`
	ReplicaID       string `json:"replicaId"`
	VersionID       string `json:"versionId"`
	Title           string `json:"title"`
	CanRedistribute bool   `json:"canRedistribute"`
}
type WebsiteReplicaRedistributionGrants struct {
	Items      []WebsiteReplicaRedistributionGrant `json:"items"`
	NextCursor *string                             `json:"nextCursor"`
}

func (*WebsiteReplicaRedistributionGrants) strictAPIResponse() {}
func (g *WebsiteReplicaRedistributionGrants) validateAPIResponse() error {
	if len(g.Items) > 50 || (g.NextCursor != nil && !zodUUIDPattern.MatchString(*g.NextCursor)) {
		return errors.New("invalid redistribution grant page")
	}
	for _, item := range g.Items {
		if !zodUUIDPattern.MatchString(item.EntitlementID) || !zodUUIDPattern.MatchString(item.ReplicaID) || !zodUUIDPattern.MatchString(item.VersionID) {
			return errors.New("invalid redistribution grant")
		}
	}
	return nil
}
func (c *Client) GetWebsiteReplicaRedistributionGrants(ctx context.Context, cursor string) (WebsiteReplicaRedistributionGrants, error) {
	path := "/v1/website-replicas/redistribution/grants"
	if cursor != "" {
		path += "?cursor=" + url.QueryEscape(cursor)
	}
	var response WebsiteReplicaRedistributionGrants
	err := c.doJSON(ctx, http.MethodGet, path, nil, &response, "@stored")
	return response, err
}
