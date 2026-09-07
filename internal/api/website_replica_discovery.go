package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
)

type WebsiteReplicaShowcase struct {
	ID                string `json:"id"`
	Title             string `json:"title"`
	PreviewURL        string `json:"previewUrl"`
	ScreenshotURL     string `json:"screenshotUrl"`
	AuthorName        string `json:"authorName"`
	ChangeDescription string `json:"changeDescription"`
	CreatedAt         string `json:"createdAt"`
}

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
	Showcases []WebsiteReplicaShowcase `json:"showcases"`
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
	if value.ShortCode != code || !zodUUIDPattern.MatchString(value.ReplicaID) || value.Title == "" ||
		!validHTTPSURL(value.PreviewURL) || !validHTTPSURL(value.DiscoveryURL) ||
		!validHTTPSURL(value.ViceMeWorkURL) || value.Creator.Handle == "" ||
		value.Statistics.AcquisitionCount < 0 || value.Statistics.CommentCount < 0 || value.Showcases == nil {
		return false
	}
	for _, item := range value.Showcases {
		if !zodUUIDPattern.MatchString(item.ID) || item.Title == "" || item.AuthorName == "" || item.ChangeDescription == "" ||
			!validHTTPSURL(item.PreviewURL) || !validHTTPSURL(item.ScreenshotURL) || !validZodDatetime(item.CreatedAt) {
			return false
		}
	}
	return true
}

type WebsiteReplicaShowcaseSubmission struct {
	EntitlementID     string `json:"entitlementId"`
	VersionID         string `json:"versionId"`
	Title             string `json:"title"`
	PreviewURL        string `json:"previewUrl"`
	ScreenshotURL     string `json:"screenshotUrl"`
	AuthorName        string `json:"authorName"`
	ChangeDescription string `json:"changeDescription"`
	Consent           bool   `json:"consent"`
}

type WebsiteReplicaManagedShowcase struct {
	WebsiteReplicaShowcase
	Status        string `json:"status"`
	SourceVersion int    `json:"sourceVersion"`
	Revision      int    `json:"revision"`
}

type WebsiteReplicaShowcaseList struct {
	Items []WebsiteReplicaManagedShowcase `json:"items"`
}

func (r *WebsiteReplicaManagedShowcase) validateAPIResponse() error {
	if !zodUUIDPattern.MatchString(r.ID) || !validStringEnum(r.Status, "PENDING", "APPROVED", "REJECTED", "WITHDRAWN") || r.Revision < 1 || r.SourceVersion < 1 {
		return errors.New("invalid showcase response")
	}
	return nil
}
func (r *WebsiteReplicaShowcaseList) validateAPIResponse() error {
	if r.Items == nil {
		return errors.New("invalid showcase list")
	}
	for _, item := range r.Items {
		if err := item.validateAPIResponse(); err != nil {
			return err
		}
	}
	return nil
}
func (c *Client) SubmitWebsiteReplicaShowcase(ctx context.Context, replicaID string, request WebsiteReplicaShowcaseSubmission) (WebsiteReplicaManagedShowcase, error) {
	var response WebsiteReplicaManagedShowcase
	err := c.doJSON(ctx, http.MethodPost, "/v1/website-replicas/"+url.PathEscape(replicaID)+"/showcases", request, &response, "@stored")
	return response, err
}
func (c *Client) WithdrawWebsiteReplicaShowcase(ctx context.Context, id string) (WebsiteReplicaManagedShowcase, error) {
	var response WebsiteReplicaManagedShowcase
	err := c.doJSON(ctx, http.MethodPost, "/v1/website-replicas/showcases/"+url.PathEscape(id)+"/withdraw", nil, &response, "@stored")
	if err == nil && (response.ID != id || response.Status != "WITHDRAWN") {
		err = invalidAPIResponse(errors.New("showcase withdrawal does not match request"))
	}
	return response, err
}
func (c *Client) ListWebsiteReplicaShowcases(ctx context.Context, replicaID string) (WebsiteReplicaShowcaseList, error) {
	var response WebsiteReplicaShowcaseList
	err := c.doJSON(ctx, http.MethodGet, "/v1/website-replicas/"+url.PathEscape(replicaID)+"/showcases", nil, &response, "@stored")
	return response, err
}
func (c *Client) ReviewWebsiteReplicaShowcase(ctx context.Context, replicaID, id, status string, revision int) (WebsiteReplicaManagedShowcase, error) {
	var response WebsiteReplicaManagedShowcase
	request := struct {
		Status           string `json:"status"`
		ExpectedRevision int    `json:"expectedRevision"`
	}{status, revision}
	err := c.doJSON(ctx, http.MethodPost, "/v1/website-replicas/"+url.PathEscape(replicaID)+"/showcases/"+url.PathEscape(id)+"/review", request, &response, "@stored")
	if err == nil && (response.ID != id || response.Status != status) {
		err = invalidAPIResponse(errors.New("showcase review does not match request"))
	}
	return response, err
}

func (c *Client) SubmitAnonymousWebsiteReplicaShowcase(ctx context.Context, request WebsiteReplicaShowcaseSubmission, proof RecoverWebsiteReplicaDownloadRequest) (WebsiteReplicaManagedShowcase, error) {
	var response WebsiteReplicaManagedShowcase
	body := struct {
		WebsiteReplicaShowcaseSubmission
		RecoverWebsiteReplicaDownloadRequest
	}{request, proof}
	err := c.doJSON(ctx, http.MethodPost, "/v1/website-replica-sessions/showcases", body, &response, "")
	return response, err
}
func (c *Client) WithdrawAnonymousWebsiteReplicaShowcase(ctx context.Context, id string, proof RecoverWebsiteReplicaDownloadRequest) (WebsiteReplicaManagedShowcase, error) {
	var response WebsiteReplicaManagedShowcase
	err := c.doJSON(ctx, http.MethodPost, "/v1/website-replica-sessions/showcases/"+url.PathEscape(id)+"/withdraw", proof, &response, "")
	if err == nil && (response.ID != id || response.Status != "WITHDRAWN") {
		err = invalidAPIResponse(errors.New("showcase withdrawal does not match request"))
	}
	return response, err
}
