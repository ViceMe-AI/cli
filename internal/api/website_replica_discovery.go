package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
)

type WebsiteReplicaDiscovery struct {
	ReplicaID     string                `json:"replicaId"`
	ShortCode     string                `json:"shortCode"`
	Title         string                `json:"title"`
	Summary       string                `json:"summary"`
	Tags          []string              `json:"tags"`
	BodyMarkdown  string                `json:"bodyMarkdown"`
	PreviewURL    string                `json:"previewUrl"`
	DiscoveryURL  string                `json:"discoveryUrl"`
	ViceMeWorkURL string                `json:"viceMeWorkUrl"`
	Creator       WebsiteReplicaCreator `json:"creator"`
	Statistics    struct {
		AcquisitionCount int `json:"acquisitionCount"`
		CommentCount     int `json:"commentCount"`
	} `json:"statistics"`
}

func (*WebsiteReplicaDiscovery) strictAPIResponse() {}

func (c *Client) GetWebsiteReplicaDiscovery(ctx context.Context, shortCode string) (WebsiteReplicaDiscovery, error) {
	var response WebsiteReplicaDiscovery
	err := c.doJSON(ctx, http.MethodGet, "/v1/website-replicas/"+url.PathEscape(shortCode)+"/discovery", nil, &response, "")
	if err == nil && !validWebsiteReplicaDiscovery(response, shortCode) {
		err = invalidAPIResponse(errors.New("invalid Website Replica discovery"))
	}
	return response, err
}

func validWebsiteReplicaDiscovery(value WebsiteReplicaDiscovery, code string) bool {
	return value.ShortCode == code && zodUUIDPattern.MatchString(value.ReplicaID) && value.Title != "" &&
		validHTTPSURL(value.PreviewURL) && validHTTPSURL(value.DiscoveryURL) &&
		validHTTPSURL(value.ViceMeWorkURL) && value.Creator.Handle != "" &&
		value.Statistics.AcquisitionCount >= 0 && value.Statistics.CommentCount >= 0
}
