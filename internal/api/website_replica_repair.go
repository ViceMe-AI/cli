package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

type WebsiteReplicaPageRepairRequest struct {
	ClientRequestID string                                  `json:"clientRequestId"`
	Page            WebsiteReplicaPublicationSourceArtifact `json:"page"`
}
type WebsiteReplicaPageRepair struct {
	ID              string                            `json:"id"`
	PublicationID   string                            `json:"publicationId"`
	ClientRequestID string                            `json:"clientRequestId"`
	Status          string                            `json:"status"`
	Page            WebsiteReplicaPublicationSource   `json:"page"`
	Failure         *WebsiteReplicaPublicationFailure `json:"failure"`
	Result          *WebsiteReplicaPageRepairResult   `json:"result"`
	CreatedAt       string                            `json:"createdAt"`
	UpdatedAt       string                            `json:"updatedAt"`
}
type WebsiteReplicaPageRepairResult struct {
	ReplicaVersionID string                             `json:"replicaVersionId"`
	PageRelease      WebsiteReplicaPageReleaseReference `json:"pageRelease"`
	PublishedAt      string                             `json:"publishedAt"`
}
type WebsiteReplicaPageRepairUpload struct {
	PublicationID string                            `json:"publicationId"`
	RepairID      string                            `json:"repairId"`
	Upload        WebsiteReplicaUploadAuthorization `json:"upload"`
}

func (*WebsiteReplicaPageRepair) strictAPIResponse()       {}
func (*WebsiteReplicaPageRepairUpload) strictAPIResponse() {}
func (r *WebsiteReplicaPageRepair) validateAPIResponse() error {
	invalid := errors.New("invalid Website Replica page repair")
	if !zodUUIDPattern.MatchString(r.ID) || !zodUUIDPattern.MatchString(r.PublicationID) || !zodUUIDPattern.MatchString(r.ClientRequestID) || !validStringEnum(r.Status, "WAITING_UPLOAD", "UPLOADED", "VALIDATING", "PUBLISHED", "FAILED") || validateWebsiteReplicaPublicationSource(r.Page) != nil || !validZodDatetime(r.CreatedAt) || !validZodDatetime(r.UpdatedAt) {
		return invalid
	}
	if (r.Status == "PUBLISHED") != (r.Result != nil) || (r.Status == "FAILED") != (r.Failure != nil) || validStringEnum(r.Page.Status, "VERIFIED", "ACTIVATED") != (r.Page.VerifiedAt != nil) {
		return invalid
	}
	if r.Result != nil && (r.Page.Status != "ACTIVATED" || !zodUUIDPattern.MatchString(r.Result.ReplicaVersionID) || !zodUUIDPattern.MatchString(r.Result.PageRelease.ID) || !validPositiveSafeInteger(r.Result.PageRelease.Version) || !validZodDatetime(r.Result.PublishedAt)) {
		return invalid
	}
	if f := r.Failure; f != nil && (strings.TrimSpace(f.Code) == "" || utf16CodeUnits(f.Code) > 64 || strings.TrimSpace(f.Message) == "" || utf16CodeUnits(f.Message) > 500) {
		return invalid
	}
	return nil
}
func (r *WebsiteReplicaPageRepairUpload) validateAPIResponse() error {
	if !zodUUIDPattern.MatchString(r.PublicationID) || !zodUUIDPattern.MatchString(r.RepairID) || r.Upload.Method != "PUT" || !validAbsoluteURL(r.Upload.URL) || r.Upload.Headers == nil || !validZodDatetime(r.Upload.ExpiresAt) {
		return errors.New("invalid page repair upload authorization")
	}
	return nil
}
func (c *Client) CreateWebsiteReplicaPageRepair(ctx context.Context, publicationID string, input WebsiteReplicaPageRepairRequest) (WebsiteReplicaPageRepair, error) {
	var result WebsiteReplicaPageRepair
	err := c.doJSON(ctx, http.MethodPost, websiteReplicaPublicationPath(publicationID)+"/page-repairs", input, &result, "@stored")
	if err == nil && (result.PublicationID != publicationID || result.ClientRequestID != input.ClientRequestID || result.Page.FileName != input.Page.FileName || result.Page.SizeBytes != input.Page.SizeBytes || result.Page.Digest != input.Page.Digest || result.Page.ContentType != input.Page.ContentType) {
		err = invalidAPIResponse(errors.New("repair request target mismatch"))
	}
	return result, err
}
func (c *Client) AuthorizeWebsiteReplicaPageRepairUpload(ctx context.Context, publicationID, repairID string) (WebsiteReplicaPageRepairUpload, error) {
	var result WebsiteReplicaPageRepairUpload
	err := c.doJSON(ctx, http.MethodPost, websiteReplicaPublicationPath(publicationID)+"/page-repairs/"+url.PathEscape(repairID)+"/upload-authorizations", struct{}{}, &result, "@stored")
	if err == nil && (result.PublicationID != publicationID || result.RepairID != repairID) {
		err = invalidAPIResponse(errors.New("repair upload target mismatch"))
	}
	return result, err
}
func (c *Client) CompleteWebsiteReplicaPageRepairUpload(ctx context.Context, publicationID, repairID string) (WebsiteReplicaPageRepair, error) {
	var result WebsiteReplicaPageRepair
	err := c.doJSON(ctx, http.MethodPost, websiteReplicaPublicationPath(publicationID)+"/page-repairs/"+url.PathEscape(repairID)+"/complete-upload", struct{}{}, &result, "@stored")
	if err == nil && (result.PublicationID != publicationID || result.ID != repairID) {
		err = invalidAPIResponse(errors.New("repair completion target mismatch"))
	}
	return result, err
}
