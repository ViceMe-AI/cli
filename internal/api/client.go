package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	whatwgurl "github.com/nlnwa/whatwg-url/url"
)

const maxResponseBytes = 8 << 20

var (
	liveWorkSdkKeyPattern       = regexp.MustCompile(`^wrk_live_[A-Za-z0-9_-]{4,119}$`)
	testWorkSdkKeyPattern       = regexp.MustCompile(`^wrk_test_[A-Za-z0-9_-]{4,119}$`)
	accessWorkFeatureKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,63}$`)
	uuidPattern                 = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	websiteReplicaCodePattern   = regexp.MustCompile(`^VMR-[A-Z0-9]{20}$`)
	sha256HexPattern            = regexp.MustCompile(`^[a-f0-9]{64}$`)
	shopURLParser               = whatwgurl.NewParser(whatwgurl.WithPathPercentEncodeSet(whatwgurl.PathPercentEncodeSet.Set('^')))
)

type TokenSource interface {
	Token(context.Context) (string, error)
}

type Client struct {
	BaseURL          string
	HTTPClient       *http.Client
	UploadHTTPClient *http.Client
	Tokens           TokenSource
	UserAgent        string
}

func NewClient(baseURL string, httpClient *http.Client, tokens TokenSource, userAgent string) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	uploadHTTPClient := *httpClient
	uploadHTTPClient.Timeout = 10 * time.Minute
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"), HTTPClient: httpClient,
		UploadHTTPClient: &uploadHTTPClient, Tokens: tokens, UserAgent: userAgent,
	}
}

func (c *Client) StartDeviceAuthorization(ctx context.Context, request DeviceAuthorizationRequest) (DeviceAuthorization, error) {
	var response DeviceAuthorization
	err := c.doJSON(ctx, http.MethodPost, "/v1/cli/device-authorizations", request, &response, "")
	return response, err
}

func (c *Client) ExchangeDeviceToken(ctx context.Context, deviceCode string) (DeviceToken, error) {
	var response DeviceToken
	err := c.doJSON(ctx, http.MethodPost, "/v1/cli/device-authorizations/token", DeviceTokenRequest{DeviceCode: deviceCode}, &response, "")
	return response, err
}

func (c *Client) AuthStatus(ctx context.Context) (AuthStatus, error) {
	var response AuthStatus
	err := c.doJSON(ctx, http.MethodGet, "/v1/cli/auth/status", nil, &response, "@stored")
	return response, err
}

func (c *Client) GetGithubChannelVerified(ctx context.Context, merchantAccountID string) (GithubChannelVerified, error) {
	var response GithubChannelVerified
	endpoint := "/v1/cli/merchant/channels/github/verified?merchantAccountId=" + url.QueryEscape(merchantAccountID)
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, "@stored")
	return response, err
}

func (c *Client) GetCreatorSubscriptionPlan(ctx context.Context, merchantAccountID string) (CreatorSubscriptionPlan, error) {
	var response CreatorSubscriptionPlan
	endpoint := "/v1/cli/merchant/creator-subscription-plan?merchantAccountId=" + url.QueryEscape(merchantAccountID)
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, "@stored")
	return response, err
}

func (c *Client) SetCreatorSubscriptionPlan(ctx context.Context, merchantAccountID string, priceMinor int) (CreatorSubscriptionPlan, error) {
	var response CreatorSubscriptionPlan
	payload := map[string]any{"merchantAccountId": merchantAccountID, "priceMinor": priceMinor}
	err := c.doJSON(ctx, http.MethodPut, "/v1/cli/merchant/creator-subscription-plan", payload, &response, "@stored")
	return response, err
}

func (c *Client) DisableCreatorSubscriptionPlan(ctx context.Context, merchantAccountID string) (CreatorSubscriptionPlan, error) {
	var response CreatorSubscriptionPlan
	endpoint := "/v1/cli/merchant/creator-subscription-plan?merchantAccountId=" + url.QueryEscape(merchantAccountID)
	err := c.doJSON(ctx, http.MethodDelete, endpoint, nil, &response, "@stored")
	return response, err
}

func (c *Client) Revoke(ctx context.Context, accessToken string) error {
	return c.doJSON(ctx, http.MethodPost, "/v1/cli/auth/logout", struct{}{}, nil, accessToken)
}

func (c *Client) ListMerchantAccounts(ctx context.Context) (MerchantAccountsResponse, error) {
	var response MerchantAccountsResponse
	err := c.doJSON(ctx, http.MethodGet, "/v1/cli/merchant/accounts", nil, &response, "@stored")
	return response, err
}

func (c *Client) GetMerchantOnboarding(ctx context.Context) (CurrentMerchantOnboarding, error) {
	var response CurrentMerchantOnboarding
	err := c.doJSON(ctx, http.MethodGet, "/v1/cli/merchant/onboarding/current", nil, &response, "@stored")
	return response, err
}

func (c *Client) CreateMerchantApplication(ctx context.Context, clientRequestID string, displayName, handle *string) (MerchantOnboarding, error) {
	var response MerchantOnboarding
	payload := map[string]any{"clientRequestId": clientRequestID}
	// displayName/handle 均可由服务端派生；空值不发送（最少提问申请链路）。
	if displayName != nil {
		payload["displayName"] = *displayName
	}
	if handle != nil {
		payload["handle"] = *handle
	}
	err := c.doJSON(ctx, http.MethodPost, "/v1/cli/merchant/onboarding/applications", payload, &response, "@stored")
	return response, err
}

func (c *Client) StartGithubChannel(ctx context.Context, merchantAccountID string) (GithubAuthorizationStart, error) {
	var response GithubAuthorizationStart
	payload := map[string]any{"merchantAccountId": merchantAccountID, "returnTo": "/cli/github-result"}
	err := c.doJSON(ctx, http.MethodPost, "/v1/cli/merchant/channels/github/start", payload, &response, "@stored")
	return response, err
}

func (c *Client) GetGithubChannelStatus(ctx context.Context, merchantAccountID, attemptID string) (GithubAuthorizationStatus, error) {
	var response GithubAuthorizationStatus
	endpoint := "/v1/cli/merchant/channels/github/status?merchantAccountId=" + url.QueryEscape(merchantAccountID) + "&attemptId=" + url.QueryEscape(attemptID)
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, "@stored")
	return response, err
}

func (c *Client) UploadMerchantOnboardingEvidence(ctx context.Context, onboardingID string, lockVersion int, filename string, image []byte) (MerchantOnboarding, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("lockVersion", fmt.Sprintf("%d", lockVersion))
	part, err := writer.CreateFormFile("image", filepath.Base(filename))
	if err != nil {
		return MerchantOnboarding{}, output.Internal("ONBOARDING_EVIDENCE_ENCODE_FAILED", "could not encode evidence upload", err)
	}
	if _, err := part.Write(image); err != nil {
		return MerchantOnboarding{}, output.Internal("ONBOARDING_EVIDENCE_ENCODE_FAILED", "could not encode evidence upload", err)
	}
	if err := writer.Close(); err != nil {
		return MerchantOnboarding{}, output.Internal("ONBOARDING_EVIDENCE_ENCODE_FAILED", "could not finish evidence upload", err)
	}
	var response MerchantOnboarding
	endpoint := "/v1/cli/merchant/onboarding/" + url.PathEscape(onboardingID) + "/evidence"
	err = c.doBody(ctx, http.MethodPost, endpoint, &body, writer.FormDataContentType(), &response, "@stored", maxResponseBytes)
	return response, err
}

// UploadMerchantOnboardingEvidenceText 提交一轮材料中的文字说明（与截图互斥，
// 服务端限制每轮一条）。走同一 evidence 端点的 multipart text 字段。
func (c *Client) UploadMerchantOnboardingEvidenceText(ctx context.Context, onboardingID string, lockVersion int, text string) (MerchantOnboarding, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("lockVersion", fmt.Sprintf("%d", lockVersion))
	_ = writer.WriteField("text", text)
	if err := writer.Close(); err != nil {
		return MerchantOnboarding{}, output.Internal("ONBOARDING_EVIDENCE_ENCODE_FAILED", "could not encode evidence upload", err)
	}
	var response MerchantOnboarding
	endpoint := "/v1/cli/merchant/onboarding/" + url.PathEscape(onboardingID) + "/evidence"
	err := c.doBody(ctx, http.MethodPost, endpoint, &body, writer.FormDataContentType(), &response, "@stored", maxResponseBytes)
	return response, err
}

func (c *Client) SubmitMerchantOnboarding(ctx context.Context, onboardingID string, lockVersion int) (MerchantOnboarding, error) {
	var response MerchantOnboarding
	endpoint := "/v1/cli/merchant/onboarding/" + url.PathEscape(onboardingID) + "/submit"
	err := c.doJSON(ctx, http.MethodPost, endpoint, map[string]any{"lockVersion": lockVersion}, &response, "@stored")
	return response, err
}

type SkillSourceArchive struct {
	Bytes           []byte
	Private         bool
	ResolvedCommit  string
	OwnerSubjectID  string
	Repository      string
	Path            string
	SkillID         string
	ArtifactVersion string
	ArtifactDigest  string
	SourceReceiptID string
	PackageDigest   string
}

func (c *Client) DownloadGithubSkillSource(ctx context.Context, merchantAccountID, repository, ref, repositoryPath string) (SkillSourceArchive, error) {
	var selectedPath any
	if strings.TrimSpace(repositoryPath) != "" {
		selectedPath = repositoryPath
	}
	payload := map[string]any{"merchantAccountId": merchantAccountID, "repository": repository, "ref": ref, "path": selectedPath}
	data, headers, err := c.postJSONBytes(ctx, "/v1/cli/merchant/channels/github/archive", payload, "@stored", 20<<20)
	decodedRepository, _ := url.QueryUnescape(headers.Get("X-ViceMe-Github-Repository"))
	decodedPath, _ := url.QueryUnescape(headers.Get("X-ViceMe-Github-Path"))
	return SkillSourceArchive{
		Bytes: data, Private: strings.EqualFold(headers.Get("X-ViceMe-Github-Private"), "true"),
		ResolvedCommit: headers.Get("X-ViceMe-Github-Commit"), OwnerSubjectID: headers.Get("X-ViceMe-Github-Owner-Subject"),
		Repository: decodedRepository, Path: decodedPath, SourceReceiptID: headers.Get("X-ViceMe-Source-Receipt"), PackageDigest: headers.Get("X-ViceMe-Package-Digest"),
	}, err
}

func (c *Client) DownloadXiaohongshuSkillSource(ctx context.Context, merchantAccountID, skillID string) (SkillSourceArchive, error) {
	payload := map[string]any{"merchantAccountId": merchantAccountID, "skillId": skillID}
	data, headers, err := c.postJSONBytes(ctx, "/v1/cli/merchant/channels/xiaohongshu/archive", payload, "@stored", 20<<20)
	version, _ := url.QueryUnescape(headers.Get("X-ViceMe-Xiaohongshu-Artifact-Version"))
	return SkillSourceArchive{Bytes: data, SkillID: headers.Get("X-ViceMe-Xiaohongshu-Skill-Id"), ArtifactVersion: version, ArtifactDigest: headers.Get("X-ViceMe-Xiaohongshu-Artifact-Digest"), SourceReceiptID: headers.Get("X-ViceMe-Source-Receipt"), PackageDigest: headers.Get("X-ViceMe-Package-Digest")}, err
}

func (c *Client) SearchXiaohongshuSkills(ctx context.Context, merchantAccountID, query string) (XiaohongshuSkillSearch, error) {
	var response XiaohongshuSkillSearch
	payload := map[string]any{"merchantAccountId": merchantAccountID, "query": query}
	err := c.doJSON(ctx, http.MethodPost, "/v1/cli/merchant/channels/xiaohongshu/search", payload, &response, "@stored")
	return response, err
}

func (c *Client) StartXiaohongshuChannelVerification(ctx context.Context, merchantAccountID, subjectID, accountName string, externalHandle, profileURL *string) (MerchantOnboarding, error) {
	var response MerchantOnboarding
	payload := map[string]any{"merchantAccountId": merchantAccountID, "externalSubjectId": subjectID, "externalHandle": externalHandle, "publicAccountName": accountName, "profileUrl": profileURL}
	err := c.doJSON(ctx, http.MethodPost, "/v1/cli/merchant/channels/xiaohongshu/verification", payload, &response, "@stored")
	return response, err
}

func (c *Client) GetPublicWork(ctx context.Context, creatorHandle, workSlug string) (PublicWorkProjection, error) {
	var response PublicWorkProjection
	endpoint := "/v1/public/creators/" + url.PathEscape(creatorHandle) + "/works/" + url.PathEscape(workSlug)
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, "")
	return response, err
}

func (c *Client) CreateMerchantWork(ctx context.Context, input json.RawMessage) (MerchantWork, error) {
	var response MerchantWork
	err := c.doJSON(ctx, http.MethodPost, "/v1/cli/merchant/works", input, &response, "@stored")
	return response, err
}

func (c *Client) ListMerchantWorks(ctx context.Context, merchantAccountID string) (MerchantWorksResponse, error) {
	var response MerchantWorksResponse
	query := url.Values{"merchantAccountId": {merchantAccountID}}
	err := c.doJSON(ctx, http.MethodGet, "/v1/cli/merchant/works?"+query.Encode(), nil, &response, "@stored")
	return response, err
}

func (c *Client) GetMerchantWork(ctx context.Context, workID, merchantAccountID string) (MerchantWork, error) {
	var response MerchantWork
	query := url.Values{"merchantAccountId": {merchantAccountID}}
	endpoint := "/v1/cli/merchant/works/" + url.PathEscape(workID) + "?" + query.Encode()
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, "@stored")
	return response, err
}

func (c *Client) GetPageCustomizationState(ctx context.Context, merchantAccountID string, target PageCustomizationTarget) (PageCustomizationState, error) {
	var response PageCustomizationState
	query := pageCustomizationTargetQuery(merchantAccountID, target)
	err := c.doJSON(ctx, http.MethodGet, "/v1/cli/merchant/page-customizations?"+query.Encode(), nil, &response, "@stored")
	return response, err
}

func (c *Client) DescribePageCustomizationTarget(ctx context.Context, merchantAccountID string, target PageCustomizationTarget) (PageCustomizationTargetDescription, error) {
	var response PageCustomizationTargetDescription
	query := pageCustomizationTargetQuery(merchantAccountID, target)
	err := c.doJSON(ctx, http.MethodGet, "/v1/cli/merchant/page-customizations/describe?"+query.Encode(), nil, &response, "@stored")
	return response, err
}

func pageCustomizationTargetQuery(merchantAccountID string, target PageCustomizationTarget) url.Values {
	query := url.Values{
		"merchantAccountId": {merchantAccountID},
		"targetType":        {target.Type},
		"creatorHandle":     {target.CreatorHandle},
	}
	if target.WorkSlug != "" {
		query.Set("workSlug", target.WorkSlug)
	}
	return query
}

func (c *Client) CreatePageCustomizationDraft(ctx context.Context, request CreatePageCustomizationDraftRequest) (CreatePageCustomizationDraftResponse, error) {
	var response CreatePageCustomizationDraftResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/cli/merchant/page-customizations/drafts", request, &response, "@stored")
	return response, err
}

func (c *Client) AuthorizePageCustomizationUpload(ctx context.Context, releaseID, merchantAccountID string) (PageCustomizationUploadAuthorization, error) {
	var response PageCustomizationUploadAuthorization
	endpoint := "/v1/cli/merchant/page-customizations/releases/" + url.PathEscape(releaseID) + "/upload-authorizations"
	err := c.doJSON(ctx, http.MethodPost, endpoint, map[string]any{"merchantAccountId": merchantAccountID}, &response, "@stored")
	return response, err
}

func (c *Client) CompletePageCustomizationUpload(ctx context.Context, releaseID, merchantAccountID string) (PageCustomizationRelease, error) {
	var response PageCustomizationRelease
	endpoint := "/v1/cli/merchant/page-customizations/releases/" + url.PathEscape(releaseID) + "/complete-upload"
	err := c.doJSON(ctx, http.MethodPost, endpoint, map[string]any{"merchantAccountId": merchantAccountID}, &response, "@stored")
	return response, err
}

func (c *Client) CreatePageCustomizationPreview(ctx context.Context, releaseID, merchantAccountID string, expiresInSeconds int) (PageCustomizationPreview, error) {
	var response PageCustomizationPreview
	endpoint := "/v1/cli/merchant/page-customizations/releases/" + url.PathEscape(releaseID) + "/previews"
	payload := map[string]any{"merchantAccountId": merchantAccountID, "expiresInSeconds": expiresInSeconds}
	err := c.doJSON(ctx, http.MethodPost, endpoint, payload, &response, "@stored")
	return response, err
}

func (c *Client) PublishPageCustomization(ctx context.Context, releaseID, merchantAccountID string, expectedActiveReleaseID *string, action string) (PageCustomizationRelease, error) {
	var response PageCustomizationRelease
	endpoint := "/v1/cli/merchant/page-customizations/releases/" + url.PathEscape(releaseID) + "/" + action
	payload := map[string]any{"merchantAccountId": merchantAccountID, "expectedActiveReleaseId": expectedActiveReleaseID}
	err := c.doJSON(ctx, http.MethodPost, endpoint, payload, &response, "@stored")
	return response, err
}

func (c *Client) UpdateMerchantWork(ctx context.Context, workID string, input json.RawMessage) (MerchantWork, error) {
	var response MerchantWork
	endpoint := "/v1/cli/merchant/works/" + url.PathEscape(workID)
	err := c.doJSON(ctx, http.MethodPatch, endpoint, input, &response, "@stored")
	return response, err
}

func (c *Client) CreateMerchantWorkPreview(ctx context.Context, workID, merchantAccountID string, expectedRevision, expiresInSeconds int) (WorkPreviewGrant, error) {
	var response WorkPreviewGrant
	endpoint := "/v1/cli/merchant/works/" + url.PathEscape(workID) + "/previews"
	request := map[string]any{
		"merchantAccountId":      merchantAccountID,
		"expectedRevision":       expectedRevision,
		"expiresInSeconds":       expiresInSeconds,
		"allowedRepresentations": []string{"HTML", "MARKDOWN"},
	}
	err := c.doJSON(ctx, http.MethodPost, endpoint, request, &response, "@stored")
	return response, err
}

func (c *Client) RevokeMerchantWorkPreview(ctx context.Context, workID, grantID, merchantAccountID string) (WorkPreviewGrant, error) {
	var response WorkPreviewGrant
	endpoint := "/v1/cli/merchant/works/" + url.PathEscape(workID) + "/previews/" + url.PathEscape(grantID)
	request := map[string]any{"merchantAccountId": merchantAccountID}
	err := c.doJSON(ctx, http.MethodDelete, endpoint, request, &response, "@stored")
	return response, err
}

func (c *Client) CreateWebsiteVerification(ctx context.Context, workID string, request CreateWebsiteVerificationRequest) (WebsiteVerification, error) {
	var response WebsiteVerification
	endpoint := "/v1/cli/merchant/works/" + url.PathEscape(workID) + "/website-verifications"
	err := c.doJSON(ctx, http.MethodPost, endpoint, request, &response, "@stored")
	if err == nil && (response.WebsiteWorkID != workID || response.Status != "PENDING" || response.Challenge == nil) {
		err = invalidAPIResponse(errors.New("created Website verification does not match the requested Work"))
	}
	return response, err
}

func (c *Client) GetLatestWebsiteVerification(ctx context.Context, workID, merchantAccountID string) (WebsiteVerification, error) {
	var response WebsiteVerification
	query := url.Values{"merchantAccountId": {merchantAccountID}}
	endpoint := "/v1/cli/merchant/works/" + url.PathEscape(workID) + "/website-verifications/latest?" + query.Encode()
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, "@stored")
	if err == nil && response.WebsiteWorkID != workID {
		err = invalidAPIResponse(errors.New("Website verification response does not match the requested Work"))
	}
	return response, err
}

func (c *Client) VerifyWebsite(ctx context.Context, workID string, request VerifyWebsiteRequest) (MerchantWork, error) {
	var response MerchantWork
	endpoint := "/v1/cli/merchant/works/" + url.PathEscape(workID) + "/website-verifications/verify"
	err := c.doJSON(ctx, http.MethodPost, endpoint, request, &response, "@stored")
	if err == nil {
		err = validateWebsiteWorkMutation(response, workID, request.MerchantAccountID, "VERIFIED")
		if err == nil && (response.Website.VerificationVersion != request.ExpectedVerificationVersion || response.Website.VerifiedAt == nil) {
			err = errors.New("verified Website response does not match the requested verification version")
		}
		if err != nil {
			err = invalidAPIResponse(err)
		}
	}
	return response, err
}

func (c *Client) RevokeWebsiteOwnership(ctx context.Context, workID string, request RevokeWebsiteOwnershipRequest) (MerchantWork, error) {
	var response MerchantWork
	endpoint := "/v1/cli/merchant/works/" + url.PathEscape(workID) + "/website-verifications/revoke"
	err := c.doJSON(ctx, http.MethodPost, endpoint, request, &response, "@stored")
	if err == nil {
		err = validateWebsiteWorkMutation(response, workID, request.MerchantAccountID, "REVOKED")
		if err == nil && (response.Revision <= request.ExpectedRevision || response.Website.VerifiedAt != nil) {
			err = errors.New("revoked Website response did not advance the Work revision")
		}
		if err != nil {
			err = invalidAPIResponse(err)
		}
	}
	return response, err
}

func (c *Client) CreateWorkSdkAccess(ctx context.Context, workID string, request CreateWorkSdkAccessRequest) (WorkSdkAccess, error) {
	var response WorkSdkAccess
	endpoint := "/v1/cli/merchant/works/" + url.PathEscape(workID) + "/sdk-access"
	err := c.doJSON(ctx, http.MethodPost, endpoint, request, &response, "@stored")
	if err == nil && (response.WorkID != workID || response.Status != "ACTIVE" ||
		!workSdkFeaturesEqual(response.Features, request.Features) ||
		!workAccessFeaturesMatchRequest(response.AccessFeatures, request.AccessFeatures)) {
		err = invalidAPIResponse(errors.New("created Work SDK access does not match the request"))
	}
	return response, err
}

func (c *Client) GetWorkSdkAccess(ctx context.Context, workID, merchantAccountID string) (WorkSdkAccess, error) {
	var response WorkSdkAccess
	query := url.Values{"merchantAccountId": {merchantAccountID}}
	endpoint := "/v1/cli/merchant/works/" + url.PathEscape(workID) + "/sdk-access?" + query.Encode()
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, "@stored")
	if err == nil && response.WorkID != workID {
		err = invalidAPIResponse(errors.New("Work SDK access response does not match the requested Work"))
	}
	return response, err
}

func (c *Client) ListWorkSdkAccesses(ctx context.Context, merchantAccountID string) (WorkSdkAccessesResponse, error) {
	var response WorkSdkAccessesResponse
	query := url.Values{"merchantAccountId": {merchantAccountID}}
	err := c.doJSON(ctx, http.MethodGet, "/v1/cli/merchant/work-sdk-accesses?"+query.Encode(), nil, &response, "@stored")
	return response, err
}

func (c *Client) UpdateWorkSdkAccess(ctx context.Context, workID string, request UpdateWorkSdkAccessRequest) (WorkSdkAccess, error) {
	var response WorkSdkAccess
	endpoint := "/v1/cli/merchant/works/" + url.PathEscape(workID) + "/sdk-access"
	err := c.doJSON(ctx, http.MethodPut, endpoint, request, &response, "@stored")
	if err == nil && (response.WorkID != workID || response.Status != "ACTIVE" ||
		response.ConfigVersion <= request.ExpectedConfigVersion || !workSdkFeaturesEqual(response.Features, request.Features) ||
		!workAccessFeaturesMatchRequest(response.AccessFeatures, request.AccessFeatures)) {
		err = invalidAPIResponse(errors.New("updated Work SDK access does not match the request"))
	}
	return response, err
}

func (c *Client) DisableWorkSdkAccess(ctx context.Context, workID, merchantAccountID string) (WorkSdkAccess, error) {
	var response WorkSdkAccess
	query := url.Values{"merchantAccountId": {merchantAccountID}}
	endpoint := "/v1/cli/merchant/works/" + url.PathEscape(workID) + "/sdk-access?" + query.Encode()
	err := c.doJSON(ctx, http.MethodDelete, endpoint, nil, &response, "@stored")
	if err == nil && (response.WorkID != workID || response.Status != "DISABLED") {
		err = invalidAPIResponse(errors.New("disabled Work SDK access does not match the requested Work"))
	}
	return response, err
}

func (c *Client) CreateCommerceApplication(ctx context.Context, request CreateCommerceApplicationRequest) (CommerceApplication, error) {
	var response CommerceApplication
	err := c.doJSON(ctx, http.MethodPost, "/v1/cli/merchant/commerce-applications", request, &response, "@stored")
	if err == nil && !commerceApplicationMatchesCreateRequest(response, request) {
		err = invalidAPIResponse(errors.New("created Commerce Application does not match the request"))
	}
	return response, err
}

func (c *Client) ListCommerceApplications(ctx context.Context, merchantAccountID string) (CommerceApplicationsResponse, error) {
	var response CommerceApplicationsResponse
	query := url.Values{"merchantAccountId": {merchantAccountID}}
	err := c.doJSON(ctx, http.MethodGet, "/v1/cli/merchant/commerce-applications?"+query.Encode(), nil, &response, "@stored")
	if err == nil {
		for _, application := range response.Items {
			if application.MerchantAccountID != merchantAccountID {
				err = invalidAPIResponse(errors.New("Commerce Application list contains another Merchant account"))
				break
			}
		}
	}
	return response, err
}

func (c *Client) GetCommerceApplication(ctx context.Context, applicationID, merchantAccountID string) (CommerceApplication, error) {
	var response CommerceApplication
	query := url.Values{"merchantAccountId": {merchantAccountID}}
	endpoint := "/v1/cli/merchant/commerce-applications/" + url.PathEscape(applicationID) + "?" + query.Encode()
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, "@stored")
	if err == nil && (response.ID != applicationID || response.MerchantAccountID != merchantAccountID) {
		err = invalidAPIResponse(errors.New("Commerce Application response does not match the requested resource"))
	}
	return response, err
}

func (c *Client) UpdateCommerceApplication(ctx context.Context, applicationID string, request UpdateCommerceApplicationRequest) (CommerceApplication, error) {
	var response CommerceApplication
	endpoint := "/v1/cli/merchant/commerce-applications/" + url.PathEscape(applicationID)
	err := c.doJSON(ctx, http.MethodPatch, endpoint, request, &response, "@stored")
	if err == nil && !commerceApplicationMatchesUpdateRequest(response, applicationID, request) {
		err = invalidAPIResponse(errors.New("updated Commerce Application does not match the request"))
	}
	return response, err
}

func (c *Client) CommandCommerceApplication(ctx context.Context, applicationID, command string, request CommerceApplicationCommand) (CommerceApplication, error) {
	expectedStatus := ""
	switch command {
	case "activate":
		expectedStatus = "ACTIVE"
	case "suspend":
		expectedStatus = "SUSPENDED"
	default:
		return CommerceApplication{}, output.Validation("COMMERCE_APPLICATION_COMMAND_INVALID", "Commerce Application command must be activate or suspend")
	}

	var response CommerceApplication
	endpoint := "/v1/cli/merchant/commerce-applications/" + url.PathEscape(applicationID) + "/" + command
	err := c.doJSON(ctx, http.MethodPost, endpoint, request, &response, "@stored")
	if err == nil && (response.ID != applicationID || response.MerchantAccountID != request.MerchantAccountID ||
		response.Status != expectedStatus || response.Revision <= request.ExpectedRevision) {
		err = invalidAPIResponse(errors.New("Commerce Application command response does not match the request"))
	}
	return response, err
}

func (c *Client) ListMerchantProductAuthoringTemplates(ctx context.Context, merchantAccountID string) (json.RawMessage, error) {
	var response json.RawMessage
	query := url.Values{"merchantAccountId": {merchantAccountID}}
	err := c.doJSON(ctx, http.MethodGet, "/v1/cli/merchant/product-authoring-templates?"+query.Encode(), nil, &response, "@stored")
	return response, err
}

func (c *Client) CreateMerchantProduct(ctx context.Context, input json.RawMessage) (MerchantProductDraftResponse, error) {
	var response MerchantProductDraftResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/cli/merchant/products", input, &response, "@stored")
	return response, err
}

func (c *Client) UpdateMerchantProduct(ctx context.Context, productID string, input json.RawMessage) (MerchantProductDraftResponse, error) {
	var response MerchantProductDraftResponse
	endpoint := "/v1/cli/merchant/products/" + url.PathEscape(productID) + "/draft"
	err := c.doJSON(ctx, http.MethodPatch, endpoint, input, &response, "@stored")
	return response, err
}

func (c *Client) CompileMerchantProduct(ctx context.Context, productID, merchantAccountID string, expectedRevision int) (MerchantProductDraftResponse, error) {
	var response MerchantProductDraftResponse
	endpoint := "/v1/cli/merchant/products/" + url.PathEscape(productID) + "/compile"
	request := map[string]any{"merchantAccountId": merchantAccountID, "expectedRevision": expectedRevision}
	err := c.doJSON(ctx, http.MethodPost, endpoint, request, &response, "@stored")
	return response, err
}

func (c *Client) ActivateMerchantProduct(ctx context.Context, productID string, input json.RawMessage) (MerchantProductActivationResponse, error) {
	var response MerchantProductActivationResponse
	endpoint := "/v1/cli/merchant/products/" + url.PathEscape(productID) + "/activate"
	err := c.doJSON(ctx, http.MethodPost, endpoint, input, &response, "@stored")
	return response, err
}

func (c *Client) CommandMerchantProduct(ctx context.Context, productID, command, merchantAccountID string, expectedRevision int) (MerchantProductLifecycleResponse, error) {
	var response MerchantProductLifecycleResponse
	endpoint := "/v1/cli/merchant/products/" + url.PathEscape(productID) + "/" + url.PathEscape(command)
	request := map[string]any{"merchantAccountId": merchantAccountID, "expectedRevision": expectedRevision}
	err := c.doJSON(ctx, http.MethodPost, endpoint, request, &response, "@stored")
	return response, err
}

func (c *Client) ListMerchantProducts(ctx context.Context, merchantAccountID, status, cursor string, limit int) (MerchantProductsResponse, error) {
	var response MerchantProductsResponse
	query := url.Values{"merchantAccountId": {merchantAccountID}, "limit": {fmt.Sprintf("%d", limit)}}
	if status != "" {
		query.Set("status", status)
	}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	err := c.doJSON(ctx, http.MethodGet, "/v1/cli/merchant/products?"+query.Encode(), nil, &response, "@stored")
	return response, err
}

func (c *Client) GetProductPurchaseSkill(ctx context.Context, stableName string) (ProductPurchaseSkillDescriptor, error) {
	var response ProductPurchaseSkillDescriptor
	err := c.doJSON(ctx, http.MethodGet, "/v1/product-purchase-skills/"+url.PathEscape(stableName), nil, &response, "")
	return response, err
}

func (c *Client) GetProductPurchaseSkillInstall(ctx context.Context, stableName, target string) (ProductPurchaseSkillInstall, error) {
	var response ProductPurchaseSkillInstall
	query := url.Values{"target": {target}}
	endpoint := "/v1/product-purchase-skills/" + url.PathEscape(stableName) + "/install?" + query.Encode()
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, "")
	return response, err
}

func (c *Client) GetCommerceSkillTrustKey(ctx context.Context, keyID string) (CommerceSkillTrustKey, error) {
	var response CommerceSkillTrustKey
	endpoint := "/v1/commerce-skill-trust-keys/" + url.PathEscape(keyID)
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, "")
	return response, err
}

func (c *Client) DownloadArtifact(ctx context.Context, rawURL string) ([]byte, error) {
	if err := validateUploadURL(rawURL); err != nil {
		return nil, output.Validation("ARTIFACT_URL_INVALID", "artifact URL must use HTTPS; loopback HTTP is allowed only for development")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, output.Internal("ARTIFACT_REQUEST_INVALID", "failed to create artifact request", err)
	}
	response, err := withoutRedirects(c.uploadClient()).Do(request)
	if err != nil {
		return nil, output.Network("ARTIFACT_DOWNLOAD_FAILED", "failed to download the Commerce Skill artifact", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes))
		return nil, output.Network("ARTIFACT_DOWNLOAD_REJECTED", fmt.Sprintf("artifact endpoint returned HTTP %d", response.StatusCode), nil)
	}
	const maxArtifactBytes = 32 << 20
	data, err := io.ReadAll(io.LimitReader(response.Body, maxArtifactBytes+1))
	if err != nil {
		return nil, output.Network("ARTIFACT_DOWNLOAD_FAILED", "failed to read the Commerce Skill artifact", err)
	}
	if len(data) > maxArtifactBytes {
		return nil, output.Validation("ARTIFACT_TOO_LARGE", "Commerce Skill artifact exceeds the 32 MiB limit")
	}
	return data, nil
}

func (c *Client) DownloadPresigned(ctx context.Context, rawURL string, destination io.Writer, maxBytes int64) (int64, error) {
	if maxBytes < 1 {
		return 0, output.Validation("REPLICA_DOWNLOAD_LIMIT_INVALID", "Website Replica download limit is invalid")
	}
	if err := validateUploadURL(rawURL); err != nil {
		return 0, output.Validation("REPLICA_DOWNLOAD_URL_INVALID", "Website Replica download URL must use HTTPS; loopback HTTP is allowed only for development")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, output.Internal("REPLICA_DOWNLOAD_REQUEST_INVALID", "failed to create Website Replica download request", err)
	}
	response, err := withoutRedirects(c.uploadClient()).Do(request)
	if err != nil {
		return 0, output.Network("REPLICA_DOWNLOAD_FAILED", "failed to download the Website Replica source package", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes))
		return 0, output.Network("REPLICA_DOWNLOAD_REJECTED", fmt.Sprintf("Website Replica download endpoint returned HTTP %d", response.StatusCode), nil)
	}
	if response.ContentLength > maxBytes {
		return 0, output.Validation("REPLICA_DOWNLOAD_TOO_LARGE", "Website Replica download exceeds the authorized size")
	}
	written, err := io.Copy(destination, io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return written, output.Network("REPLICA_DOWNLOAD_FAILED", "failed while streaming the Website Replica source package", err)
	}
	if written > maxBytes {
		return written, output.Validation("REPLICA_DOWNLOAD_TOO_LARGE", "Website Replica download exceeds the authorized size")
	}
	return written, nil
}

func (c *Client) GetPublicSkillAccess(ctx context.Context, productID string) (SkillAccess, error) {
	var response SkillAccess
	endpoint := "/v1/skills/" + url.PathEscape(productID) + "/access"
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, "")
	return response, err
}

func (c *Client) GetSkillAccess(ctx context.Context, productID string) (SkillAccess, error) {
	var response SkillAccess
	endpoint := "/v1/cli/skills/" + url.PathEscape(productID) + "/access"
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, "@stored")
	return response, err
}

func (c *Client) GetFreeSkillDownload(ctx context.Context, productID string) (DownloadURL, error) {
	var response DownloadURL
	endpoint := "/v1/downloads/free/" + url.PathEscape(productID)
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, "")
	return response, err
}

func (c *Client) GetOwnedSkillDownload(ctx context.Context, productID string) (DownloadURL, error) {
	var response DownloadURL
	endpoint := "/v1/cli/skills/" + url.PathEscape(productID) + "/download"
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, "@stored")
	return response, err
}

// Skill trial endpoints stay anonymous: the install device holds its own
// grant (installId + one-time secret); no login is required to trial.
func (c *Client) CreateSkillTrialGrant(ctx context.Context, productID, installID string) (SkillTrialGrant, error) {
	var response SkillTrialGrant
	endpoint := "/v1/skills/" + url.PathEscape(productID) + "/trial-grants"
	err := c.doJSON(ctx, http.MethodPost, endpoint, skillTrialGrantRequest{InstallID: installID}, &response, "")
	return response, err
}

func (c *Client) ConsumeSkillTrialUse(ctx context.Context, productID, installID, secret string) (SkillTrialUse, error) {
	var response SkillTrialUse
	endpoint := "/v1/skills/" + url.PathEscape(productID) + "/trial-use"
	err := c.doJSON(ctx, http.MethodPost, endpoint, skillTrialUseRequest{InstallID: installID, Secret: secret}, &response, "")
	return response, err
}

func (c *Client) GetTrialSkillDownload(ctx context.Context, productID, installID string) (DownloadURL, error) {
	var response DownloadURL
	endpoint := "/v1/downloads/trial/" + url.PathEscape(productID) + "?installId=" + url.QueryEscape(installID)
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, "")
	return response, err
}

func (c *Client) UpdateListingTrialUseLimit(ctx context.Context, publicationID string, trialUseLimit *int) (SkillPublication, error) {
	return c.UpdateListingDraftPatch(ctx, publicationID, UpdateSkillPublicationDraftRequest{TrialUseLimit: trialUseLimit})
}

func (c *Client) GetSkillDetail(ctx context.Context, productID string) (json.RawMessage, error) {
	var response json.RawMessage
	err := c.doJSON(ctx, http.MethodGet, "/v1/skills/"+url.PathEscape(productID), nil, &response, "")
	return response, err
}

func (c *Client) CreateCommerceSession(ctx context.Context, stableName, clientRequestID, replaySecret string) (CommerceSession, error) {
	var response CommerceSession
	payload := map[string]any{
		"purchaseSkillStableName": stableName,
		"clientRequestId":         clientRequestID,
		"replaySecret":            replaySecret,
	}
	err := c.doJSON(ctx, http.MethodPost, "/v1/commerce-sessions", payload, &response, "")
	return response, err
}

func (c *Client) GetCommerceProduct(ctx context.Context, identifier, locale, sessionToken string) (CommerceProduct, error) {
	var response CommerceProduct
	query := url.Values{"locale": {locale}}
	endpoint := "/v1/products/" + url.PathEscape(identifier) + "?" + query.Encode()
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, sessionToken)
	return response, err
}

func (c *Client) CreateContractAsset(ctx context.Context, input json.RawMessage, sessionToken string) (ContractAssetUpload, error) {
	var response ContractAssetUpload
	err := c.doJSON(ctx, http.MethodPost, "/v1/contract-assets", input, &response, sessionToken)
	return response, err
}

func (c *Client) CompleteContractAsset(ctx context.Context, assetID, sessionToken string) (ContractAsset, error) {
	var response ContractAsset
	endpoint := "/v1/contract-assets/" + url.PathEscape(assetID) + "/complete"
	err := c.doJSON(ctx, http.MethodPost, endpoint, nil, &response, sessionToken)
	return response, err
}

func (c *Client) CreateProductQuote(ctx context.Context, input json.RawMessage, sessionToken string) (ProductQuote, error) {
	var response ProductQuote
	err := c.doJSON(ctx, http.MethodPost, "/v1/product-quotes", input, &response, sessionToken)
	return response, err
}

func (c *Client) CreateCommerceOrder(ctx context.Context, input json.RawMessage, sessionToken string) (CreateOrderResponse, error) {
	var response CreateOrderResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/orders", input, &response, sessionToken)
	return response, err
}

// Buyer endpoints authenticate the CLI credential and reuse the Shop commerce domains.
func (c *Client) CreateBuyerQuote(ctx context.Context, input json.RawMessage) (ProductQuote, error) {
	var response ProductQuote
	err := c.doJSON(ctx, http.MethodPost, "/v1/cli/product-quotes", input, &response, "@stored")
	return response, err
}

func (c *Client) CreateBuyerOrder(ctx context.Context, input json.RawMessage) (CreateOrderResponse, error) {
	var response CreateOrderResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/cli/orders", input, &response, "@stored")
	return response, err
}

func (c *Client) GetBuyerOrderStatus(ctx context.Context, orderNo string) (OrderStatusResponse, error) {
	var response OrderStatusResponse
	err := c.doJSON(ctx, http.MethodGet, "/v1/cli/orders/"+url.PathEscape(orderNo)+"/status", nil, &response, "@stored")
	return response, err
}

// GetProduct loads the buyer-facing product projection (including the active
// sales-spec SKUs) used to build a purchase quote. The endpoint accepts
// anonymous reads, but the purchase flow always runs authenticated.
func (c *Client) GetProduct(ctx context.Context, productID string) (CommerceProduct, error) {
	var response CommerceProduct
	err := c.doJSON(ctx, http.MethodGet, "/v1/products/"+url.PathEscape(productID), nil, &response, "@stored")
	return response, err
}

// GetOrder reads one of the current user's orders by order number, including
// its payment action, so a pending purchase can be recovered and re-presented.
func (c *Client) GetOrder(ctx context.Context, orderNo string) (CommerceOrder, error) {
	var response CommerceOrder
	err := c.doJSON(ctx, http.MethodGet, "/v1/cli/orders/"+url.PathEscape(orderNo), nil, &response, "@stored")
	return response, err
}

// ResumeSkillOrderPayment reuses the existing order and Shop's fenced payment
// creation. It never creates a new purchase or changes the order amount.
func (c *Client) ResumeSkillOrderPayment(ctx context.Context, orderNo, locale string) (CommerceOrder, error) {
	var response CommerceOrder
	input, _ := json.Marshal(map[string]string{"locale": locale})
	err := c.doJSON(ctx, http.MethodPost, "/v1/cli/orders/"+url.PathEscape(orderNo)+"/skill-payment", json.RawMessage(input), &response, "@stored")
	return response, err
}

// ListOrdersByStatus lists the current user's orders filtered by status.
func (c *Client) ListOrdersByStatus(ctx context.Context, status string) (MyOrdersResponse, error) {
	var response MyOrdersResponse
	endpoint := "/v1/cli/orders?status=" + url.QueryEscape(status)
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, "@stored")
	return response, err
}

// CreateCreatorSubscriptionOrder opens a fan-subscription order for the
// current user with an explicit payment channel action.
func (c *Client) CreateCreatorSubscriptionOrder(ctx context.Context, input json.RawMessage) (CreatePaymentResponse, error) {
	var response CreatePaymentResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/cli/creator-subscription-orders", input, &response, "@stored")
	return response, err
}

// GetCreatorSubscriptionOrderStatus reads one fan-subscription order's
// payment status for the current user.
func (c *Client) GetCreatorSubscriptionOrderStatus(ctx context.Context, orderNo string) (PaymentStatusResponse, error) {
	var response PaymentStatusResponse
	endpoint := "/v1/cli/creator-subscription-orders/" + url.PathEscape(orderNo)
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, "@stored")
	return response, err
}

func (c *Client) GetCommerceOrderStatus(ctx context.Context, orderNo, sessionToken string) (OrderStatusResponse, error) {
	var response OrderStatusResponse
	endpoint := "/v1/orders/" + url.PathEscape(orderNo) + "/status"
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, sessionToken)
	return response, err
}

func (c *Client) GetCommerceServiceCaseByOrder(ctx context.Context, orderNo, sessionToken string) (ServiceCase, error) {
	var response ServiceCase
	endpoint := "/v1/commerce/service-cases/orders/" + url.PathEscape(orderNo)
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, sessionToken)
	return response, err
}

func (c *Client) PutPresigned(ctx context.Context, rawURL string, headers map[string]string, body io.Reader, size int64) error {
	if err := validateUploadURL(rawURL); err != nil {
		return output.Validation("UPLOAD_URL_INVALID", "upload URL must use HTTPS; loopback HTTP is allowed only for development")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, rawURL, body)
	if err != nil {
		return output.Internal("UPLOAD_REQUEST_INVALID", "failed to create upload request", err)
	}
	request.ContentLength = size
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := withoutRedirects(c.uploadClient()).Do(request)
	if err != nil {
		return output.Network("UPLOAD_TRANSPORT_FAILED", "failed to upload contract asset", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes))
	if response.StatusCode == http.StatusPreconditionFailed {
		return nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return output.Network("UPLOAD_REJECTED", fmt.Sprintf("upload endpoint returned HTTP %d", response.StatusCode), nil)
	}
	return nil
}

// HealthReady performs an unauthenticated, redirect-free connectivity check.
// Doctor owns the short deadline so this probe can never inherit the normal
// command timeout or disclose the stored publication credential.
func (c *Client) HealthReady(ctx context.Context) error {
	return c.doJSON(ctx, http.MethodGet, "/v1/health/ready", nil, nil, "")
}

func (c *Client) CreateSkillPublication(ctx context.Context, request CreateSkillPublicationRequest) (CreateSkillPublicationResponse, error) {
	var response CreateSkillPublicationResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/creator/skill-publications", request, &response, "@stored")
	return response, err
}

func (c *Client) PrepareSkillListing(ctx context.Context, request PrepareSkillListingRequest) (PrepareSkillListingResponse, error) {
	var response PrepareSkillListingResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/creator/skill-listings/prepare", request, &response, "@stored")
	return response, err
}

func (c *Client) ListSkillListingCandidates(ctx context.Context, request SkillListingCandidatesRequest) (SkillListingCandidatesResponse, error) {
	var response SkillListingCandidatesResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/creator/skill-listings/candidates", request, &response, "@stored")
	return response, err
}

func (c *Client) GetSkillListingPreview(ctx context.Context, listingID string) (SkillListingPreview, error) {
	var response SkillListingPreview
	err := c.doJSON(ctx, http.MethodGet, "/v1/creator/skill-listings/"+url.PathEscape(listingID)+"/preview", nil, &response, "@stored")
	return response, err
}

func (c *Client) CreateSkillPreviewLaunch(ctx context.Context, listingID string) (CreateSkillPreviewLaunchResponse, error) {
	var response CreateSkillPreviewLaunchResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/creator/skill-listings/"+url.PathEscape(listingID)+"/preview-launch", nil, &response, "@stored")
	return response, err
}

func (c *Client) GetSkillPublication(ctx context.Context, publicationID string) (SkillPublication, error) {
	var response SkillPublication
	err := c.doJSON(ctx, http.MethodGet, publicationPath(publicationID), nil, &response, "@stored")
	return response, err
}

func (c *Client) AuthorizeUpload(ctx context.Context, publicationID string, request UploadAuthorizationRequest) (UploadAuthorization, error) {
	var response UploadAuthorization
	err := c.doJSON(ctx, http.MethodPost, publicationPath(publicationID)+"/upload-authorizations", request, &response, "@stored")
	return response, err
}

func (c *Client) CompleteUpload(ctx context.Context, publicationID, uploadID string) (SkillPublication, error) {
	var response SkillPublication
	err := c.doJSON(ctx, http.MethodPost, publicationPath(publicationID)+"/complete-upload", CompleteUploadRequest{UploadID: uploadID}, &response, "@stored")
	return response, err
}

func (c *Client) CreateWebsiteReplicaUpload(ctx context.Context, request CreateWebsiteReplicaUploadRequest) (CreateWebsiteReplicaUploadResponse, error) {
	var response CreateWebsiteReplicaUploadResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/website-replicas/uploads", request, &response, "@stored")
	return response, err
}

func (c *Client) CompleteWebsiteReplicaUpload(ctx context.Context, replicaID, uploadID string) (CompleteWebsiteReplicaUploadResponse, error) {
	var response CompleteWebsiteReplicaUploadResponse
	endpoint := "/v1/website-replicas/" + url.PathEscape(replicaID) + "/uploads/" + url.PathEscape(uploadID) + "/complete"
	err := c.doJSON(ctx, http.MethodPost, endpoint, nil, &response, "@stored")
	if err == nil && response.ReplicaID != replicaID {
		err = invalidAPIResponse(errors.New("Website Replica publication response does not match the requested Replica"))
	}
	return response, err
}

func (c *Client) ResolveWebsiteReplica(ctx context.Context, code string) (WebsiteReplicaResolution, error) {
	return c.resolveWebsiteReplica(ctx, code, "@stored")
}

func (c *Client) ResolveWebsiteReplicaPublic(ctx context.Context, code string) (WebsiteReplicaResolution, error) {
	return c.resolveWebsiteReplica(ctx, code, "")
}

func (c *Client) resolveWebsiteReplica(ctx context.Context, code, credential string) (WebsiteReplicaResolution, error) {
	var response WebsiteReplicaResolution
	err := c.doJSON(ctx, http.MethodPost, "/v1/website-replicas/resolve", ResolveWebsiteReplicaRequest{Instruction: code}, &response, credential)
	if err == nil && code != "VICEME-REPLICA:"+response.ShortCode {
		err = invalidAPIResponse(errors.New("Website Replica resolution does not match the requested code"))
	}
	return response, err
}

func (c *Client) CreateWebsiteReplicaSession(ctx context.Context, request CreateWebsiteReplicaSessionRequest) (WebsiteReplicaSession, error) {
	var response WebsiteReplicaSession
	err := c.doJSON(ctx, http.MethodPost, "/v1/website-replica-sessions", request, &response, "")
	return response, err
}

func (c *Client) CheckoutWebsiteReplica(ctx context.Context, sessionID, token string, request CheckoutWebsiteReplicaRequest) (CheckoutWebsiteReplicaResponse, error) {
	var response CheckoutWebsiteReplicaResponse
	endpoint := "/v1/website-replica-sessions/" + url.PathEscape(sessionID) + "/checkout"
	err := c.doJSON(ctx, http.MethodPost, endpoint, request, &response, token)
	return response, err
}

func (c *Client) GetWebsiteReplicaSessionOrderStatus(ctx context.Context, sessionID, token, orderNo string) (WebsiteReplicaOrderStatus, error) {
	var response WebsiteReplicaOrderStatus
	endpoint := "/v1/website-replica-sessions/" + url.PathEscape(sessionID) + "/orders/" + url.PathEscape(orderNo) + "/status"
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, token)
	if err == nil && response.OrderNo != orderNo {
		err = invalidAPIResponse(errors.New("Website Replica order status is invalid"))
	}
	return response, err
}

func (c *Client) GetWebsiteReplicaSessionDownload(ctx context.Context, sessionID, token string) (WebsiteReplicaDownload, error) {
	var response WebsiteReplicaDownload
	endpoint := "/v1/website-replica-sessions/" + url.PathEscape(sessionID) + "/download"
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, token)
	return response, err
}

func (c *Client) RecoverWebsiteReplicaDownload(ctx context.Context, request RecoverWebsiteReplicaDownloadRequest) (WebsiteReplicaDownload, error) {
	var response WebsiteReplicaDownload
	err := c.doJSON(ctx, http.MethodPost, "/v1/website-replica-sessions/recover-download", request, &response, "")
	return response, err
}

func (c *Client) CreateWebsiteReplicaQuote(ctx context.Context, request CreateWebsiteReplicaQuoteRequest) (WebsiteReplicaQuote, error) {
	var response WebsiteReplicaQuote
	err := c.doJSON(ctx, http.MethodPost, "/v1/website-replicas/quotes", request, &response, "@stored")
	return response, err
}

func (c *Client) CreateWebsiteReplicaOrder(ctx context.Context, request CreateWebsiteReplicaOrderRequest) (WebsiteReplicaOrder, error) {
	var response WebsiteReplicaOrder
	err := c.doJSON(ctx, http.MethodPost, "/v1/website-replicas/orders", request, &response, "@stored")
	return response, err
}

func (c *Client) GetWebsiteReplicaOrderStatus(ctx context.Context, orderNo string) (WebsiteReplicaOrderStatus, error) {
	var response WebsiteReplicaOrderStatus
	endpoint := "/v1/website-replicas/orders/" + url.PathEscape(orderNo) + "/status"
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, "@stored")
	if err == nil && response.OrderNo != orderNo {
		err = invalidAPIResponse(errors.New("Website Replica order status is invalid"))
	}
	return response, err
}

func (c *Client) GetWebsiteReplicaDownload(ctx context.Context, shortCode string) (WebsiteReplicaDownload, error) {
	var response WebsiteReplicaDownload
	endpoint := "/v1/website-replicas/" + url.PathEscape(shortCode) + "/download"
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, "@stored")
	return response, err
}

func validWebsiteReplicaOrderStatus(status string) bool {
	switch status {
	case "PENDING", "PAID", "CLOSED":
		return true
	default:
		return false
	}
}

func (c *Client) AnalyzeListing(ctx context.Context, publicationID string) (SkillPublication, error) {
	var response SkillPublication
	err := c.doJSON(ctx, http.MethodPost, publicationPath(publicationID)+"/analyze-listing", struct{}{}, &response, "@stored")
	return response, err
}

func (c *Client) UpdateListingDraft(ctx context.Context, publicationID string, request SkillPublicationDraft) (SkillPublication, error) {
	var response SkillPublication
	err := c.doJSON(ctx, http.MethodPatch, publicationPath(publicationID)+"/listing-draft", request, &response, "@stored")
	return response, err
}

func (c *Client) UpdateListingDraftPatch(ctx context.Context, publicationID string, request UpdateSkillPublicationDraftRequest) (SkillPublication, error) {
	var response SkillPublication
	err := c.doJSON(ctx, http.MethodPatch, publicationPath(publicationID)+"/listing-draft", request, &response, "@stored")
	return response, err
}

func (c *Client) SuggestListingDraft(ctx context.Context, publicationID string, request SuggestSkillPublicationDraftRequest) (SkillPublication, error) {
	var response SkillPublication
	err := c.doJSON(ctx, http.MethodPatch, publicationPath(publicationID)+"/listing-suggestion", request, &response, "@stored")
	return response, err
}

func (c *Client) UpdateListingPrice(ctx context.Context, publicationID string, priceMinor int) (SkillPublication, error) {
	return c.UpdateListingDraftPatch(ctx, publicationID, UpdateSkillPublicationDraftRequest{PriceMinor: &priceMinor})
}

func (c *Client) ConfirmPublication(ctx context.Context, publicationID, reviewDigest string) (SkillPublication, error) {
	var response SkillPublication
	err := c.doJSON(ctx, http.MethodPost, publicationPath(publicationID)+"/confirm-review", ReviewDigestRequest{ReviewDigest: reviewDigest}, &response, "@stored")
	return response, err
}

func (c *Client) PublishSkill(ctx context.Context, publicationID, reviewDigest string) (SkillPublication, error) {
	var response SkillPublication
	err := c.doJSON(ctx, http.MethodPost, publicationPath(publicationID)+"/publish", ReviewDigestRequest{ReviewDigest: reviewDigest}, &response, "@stored")
	return response, err
}

func (c *Client) CancelSkillPublication(ctx context.Context, publicationID string) (CancelPublicationResponse, error) {
	var response CancelPublicationResponse
	err := c.doJSON(ctx, http.MethodPost, publicationPath(publicationID)+"/cancel", struct{}{}, &response, "@stored")
	return response, err
}

func (c *Client) PutUpload(ctx context.Context, authorization UploadAuthorization, body io.Reader, size int64) error {
	if err := validateUploadURL(authorization.URL); err != nil {
		return output.Validation("UPLOAD_URL_INVALID", "upload URL must use HTTPS; loopback HTTP is allowed only for development")
	}
	request, err := http.NewRequestWithContext(ctx, authorization.Method, authorization.URL, body)
	if err != nil {
		return output.Internal("UPLOAD_REQUEST_INVALID", "failed to create upload request", err)
	}
	request.ContentLength = size
	for key, value := range authorization.Headers {
		request.Header.Set(key, value)
	}
	response, err := withoutRedirects(c.uploadClient()).Do(request)
	if err != nil {
		return output.Network("UPLOAD_TRANSPORT_FAILED", "failed to upload publication asset", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes))
	// Uploads are content-addressed and authorized with If-None-Match. A 412
	// means the immutable object is already present, commonly after a lost
	// response. The authenticated completion endpoint still verifies the
	// stored size, digest, and metadata before accepting it.
	if response.StatusCode == http.StatusPreconditionFailed {
		return nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return output.Network("UPLOAD_REJECTED", fmt.Sprintf("upload endpoint returned HTTP %d", response.StatusCode), nil)
	}
	return nil
}

func (c *Client) uploadClient() *http.Client {
	if c.UploadHTTPClient != nil {
		return c.UploadHTTPClient
	}
	if c.HTTPClient != nil {
		clone := *c.HTTPClient
		clone.Timeout = 10 * time.Minute
		return &clone
	}
	return &http.Client{Timeout: 10 * time.Minute}
}

func validateUploadURL(raw string) error {
	target, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || target.Hostname() == "" || target.User != nil || target.Fragment != "" || target.Opaque != "" {
		return errors.New("invalid upload URL")
	}
	if strings.EqualFold(target.Scheme, "https") {
		return nil
	}
	if strings.EqualFold(target.Scheme, "http") {
		host := target.Hostname()
		if strings.EqualFold(host, "localhost") || (net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()) {
			return nil
		}
	}
	return errors.New("upload URL must use HTTPS or loopback HTTP")
}

func (c *Client) postJSONBytes(ctx context.Context, endpoint string, requestBody any, credential string, limit int64) ([]byte, http.Header, error) {
	encoded, err := json.Marshal(requestBody)
	if err != nil {
		return nil, nil, output.Internal("REQUEST_ENCODE_FAILED", "failed to encode the API request", err)
	}
	response, err := c.sendBody(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded), "application/json", credential)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if readErr != nil {
		return nil, response.Header, output.Network("RESPONSE_READ_FAILED", "failed to read the ViceMe API response", readErr)
	}
	if int64(len(data)) > limit {
		return nil, response.Header, output.Validation("SKILL_PACKAGE_TOO_LARGE", "Skill package exceeds the download limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, response.Header, decodeServerError(response.StatusCode, data, response.Header.Get("X-Request-Id"))
	}
	return data, response.Header, nil
}

func (c *Client) doBody(ctx context.Context, method, endpoint string, body io.Reader, contentType string, responseBody any, credential string, limit int64) error {
	response, err := c.sendBody(ctx, method, endpoint, body, contentType, credential)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return output.Network("RESPONSE_READ_FAILED", "failed to read the ViceMe API response", err)
	}
	if int64(len(data)) > limit {
		return output.Internal("RESPONSE_TOO_LARGE", "ViceMe API response exceeded the client limit", nil)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return decodeServerError(response.StatusCode, data, response.Header.Get("X-Request-Id"))
	}
	if responseBody == nil {
		return nil
	}
	if err := json.Unmarshal(data, responseBody); err != nil {
		return invalidAPIResponse(err)
	}
	return nil
}

func (c *Client) sendBody(ctx context.Context, method, endpoint string, body io.Reader, contentType, credential string) (*http.Response, error) {
	base, err := validateAPIBaseURL(c.BaseURL)
	if err != nil {
		return nil, output.Validation("API_BASE_URL_INVALID", "ViceMe API base URL must use HTTPS; loopback HTTP is allowed only for development")
	}
	relative, err := url.Parse(endpoint)
	if err != nil || relative.IsAbs() || relative.Host != "" {
		return nil, output.Internal("REQUEST_ENDPOINT_INVALID", "failed to construct the ViceMe API endpoint", err)
	}
	base.Path = path.Join(base.Path, relative.Path)
	base.RawQuery = relative.RawQuery
	request, err := http.NewRequestWithContext(ctx, method, base.String(), body)
	if err != nil {
		return nil, output.Internal("REQUEST_CREATE_FAILED", "failed to create the ViceMe API request", err)
	}
	request.Header.Set("Accept", "application/json, application/zip")
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if c.UserAgent != "" {
		request.Header.Set("User-Agent", c.UserAgent)
	}
	if credential == "@stored" {
		if c.Tokens == nil {
			return nil, output.Authentication("NOT_LOGGED_IN", "not logged in to ViceMe")
		}
		credential, err = c.Tokens.Token(ctx)
		if err != nil {
			return nil, err
		}
	}
	if credential != "" {
		request.Header.Set("Authorization", "Bearer "+credential)
	}
	response, err := withoutRedirects(c.HTTPClient).Do(request)
	if err != nil {
		return nil, output.Network("API_UNREACHABLE", "failed to reach the ViceMe API", err)
	}
	return response, nil
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, requestBody, responseBody any, credential string) error {
	base, err := validateAPIBaseURL(c.BaseURL)
	if err != nil {
		return output.Validation("API_BASE_URL_INVALID", "ViceMe API base URL must use HTTPS; loopback HTTP is allowed only for development")
	}
	relative, err := url.Parse(endpoint)
	if err != nil || relative.IsAbs() || relative.Host != "" {
		return output.Internal("REQUEST_ENDPOINT_INVALID", "failed to construct the ViceMe API endpoint", err)
	}
	base.Path = path.Join(base.Path, relative.Path)
	base.RawQuery = relative.RawQuery
	var body io.Reader
	if requestBody != nil {
		encoded, encodeErr := json.Marshal(requestBody)
		if encodeErr != nil {
			return output.Internal("REQUEST_ENCODE_FAILED", "failed to encode the API request", encodeErr)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, base.String(), body)
	if err != nil {
		return output.Internal("REQUEST_CREATE_FAILED", "failed to create the API request", err)
	}
	request.Header.Set("Accept", "application/json")
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.UserAgent != "" {
		request.Header.Set("User-Agent", c.UserAgent)
	}
	if credential == "@stored" {
		if c.Tokens == nil {
			return output.Authentication("NOT_LOGGED_IN", "not logged in to ViceMe")
		}
		credential, err = c.Tokens.Token(ctx)
		if err != nil {
			return err
		}
	}
	if credential != "" {
		request.Header.Set("Authorization", "Bearer "+credential)
	}
	response, err := withoutRedirects(c.HTTPClient).Do(request)
	if err != nil {
		return output.Network("API_UNREACHABLE", "failed to reach the ViceMe API", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return output.Network("RESPONSE_READ_FAILED", "failed to read the ViceMe API response", err)
	}
	if len(data) > maxResponseBytes {
		return output.Internal("RESPONSE_TOO_LARGE", "ViceMe API response exceeded the client limit", nil)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return decodeServerError(response.StatusCode, data, response.Header.Get("X-Request-Id"))
	}
	if responseBody == nil {
		return nil
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return invalidAPIResponse(errors.New("response body is empty"))
	}
	var decodeErr error
	if _, strict := responseBody.(strictAPIResponse); strict {
		decodeErr = decodeStrictAPIResponse(data, responseBody)
	} else {
		decodeErr = json.Unmarshal(data, responseBody)
	}
	if decodeErr != nil {
		return invalidAPIResponse(decodeErr)
	}
	if validator, ok := responseBody.(interface{ validateAPIResponse() error }); ok {
		if err := validator.validateAPIResponse(); err != nil {
			return invalidAPIResponse(err)
		}
	}
	return nil
}

func invalidAPIResponse(cause error) error {
	return output.Internal("RESPONSE_INVALID", "ViceMe API returned an incomplete or invalid response", cause)
}

func (work *MerchantWork) validateAPIResponse() error {
	if work == nil || !uuidPattern.MatchString(work.ID) || strings.TrimSpace(work.Slug) == "" ||
		strings.TrimSpace(work.Title) == "" || work.Revision < 1 || work.ActiveRevision == nil || work.DraftRevision == nil ||
		!validWorkKind(work.Kind) || !validWorkOrigin(work.Origin) || !validWorkStatus(work.Status) ||
		validateWorkOwner(work.Owner) != nil || !validTimestamp(work.CreatedAt) || !validTimestamp(work.UpdatedAt) {
		return errors.New("Work response is missing required fields")
	}

	switch work.Kind {
	case "SKILL":
		if rawJSONIsNull(work.Skill) || !rawJSONIsNull(work.Service) || work.Website != nil {
			return errors.New("Work typed child does not match SKILL kind")
		}
	case "SERVICE":
		if rawJSONIsNull(work.Service) || !rawJSONIsNull(work.Skill) || work.Website != nil {
			return errors.New("Work typed child does not match SERVICE kind")
		}
	case "WEBSITE":
		if !rawJSONIsNull(work.Skill) || !rawJSONIsNull(work.Service) || work.Website == nil || work.Website.validateAPIResponse() != nil {
			return errors.New("Work typed child does not match WEBSITE kind")
		}
	}
	return nil
}

func (website *WebsiteWork) validateAPIResponse() error {
	if website == nil || len(website.DomainASCII) < 1 || len(website.DomainASCII) > 253 ||
		website.VerificationVersion < 1 || !validWebsiteOwnershipStatus(website.OwnershipStatus) {
		return errors.New("Website Work response is missing required fields")
	}
	normalizedOrigin, ok := normalizeCommerceApplicationOrigin(website.CanonicalOrigin)
	if !ok || website.CanonicalOrigin != normalizedOrigin {
		return errors.New("Website Work response contains an invalid canonical origin")
	}
	if website.VerifiedAt != nil && !validTimestamp(*website.VerifiedAt) {
		return errors.New("Website Work response contains an invalid verifiedAt")
	}
	return nil
}

func (works *MerchantWorksResponse) validateAPIResponse() error {
	if works == nil || works.Items == nil {
		return errors.New("Work list response is missing items")
	}
	for index := range works.Items {
		if err := works.Items[index].validateAPIResponse(); err != nil {
			return err
		}
	}
	return nil
}

func (verification *WebsiteVerification) validateAPIResponse() error {
	if verification == nil || !uuidPattern.MatchString(verification.ID) || !uuidPattern.MatchString(verification.WebsiteWorkID) ||
		verification.Version < 1 || len(verification.DNSRecordName) < 1 || len(verification.DNSRecordName) > 253 ||
		!validWebsiteVerificationStatus(verification.Status) || !validTimestamp(verification.ExpiresAt) {
		return errors.New("Website verification response is missing required fields")
	}
	if verification.Challenge != nil && (len(*verification.Challenge) < 32 || len(*verification.Challenge) > 255) {
		return errors.New("Website verification response contains an invalid challenge")
	}
	if verification.VerifiedAt != nil && !validTimestamp(*verification.VerifiedAt) {
		return errors.New("Website verification response contains an invalid verifiedAt")
	}
	return nil
}

func (access *WorkSdkAccess) validateAPIResponse() error {
	if access == nil || !uuidPattern.MatchString(access.WorkID) ||
		!liveWorkSdkKeyPattern.MatchString(access.Keys.Live) ||
		!testWorkSdkKeyPattern.MatchString(access.Keys.Test) ||
		(access.Status != "ACTIVE" && access.Status != "DISABLED") || access.ConfigVersion < 1 ||
		!validWorkSdkFeatures(access.Features) || !validWorkAccessFeatures(access.AccessFeatures) ||
		!validTimestamp(access.CreatedAt) || !validTimestamp(access.UpdatedAt) {
		return errors.New("Work SDK access response is missing required fields")
	}
	return nil
}

func (accesses *WorkSdkAccessesResponse) validateAPIResponse() error {
	if accesses == nil || accesses.Items == nil {
		return errors.New("Work SDK access list response is missing items")
	}
	for index := range accesses.Items {
		if err := accesses.Items[index].validateAPIResponse(); err != nil {
			return err
		}
	}
	return nil
}

func (application *CommerceApplication) validateAPIResponse() error {
	if application == nil || !uuidPattern.MatchString(application.ID) || !uuidPattern.MatchString(application.WorkID) ||
		!uuidPattern.MatchString(application.MerchantAccountID) || len(application.PublicClientID) < 16 || len(application.PublicClientID) > 96 ||
		!validCommerceApplicationKind(application.Kind) || !validCommerceApplicationEnvironment(application.Environment) ||
		!validCommerceApplicationStatus(application.Status) || utf16CodeUnits(application.DisplayName) < 1 || utf16CodeUnits(application.DisplayName) > 120 ||
		application.Revision < 1 || application.Origins == nil || application.ReturnURLs == nil || application.Products == nil ||
		!validTimestamp(application.CreatedAt) || !validTimestamp(application.UpdatedAt) {
		return errors.New("Commerce Application response is missing required fields")
	}
	for _, origin := range application.Origins {
		if !uuidPattern.MatchString(origin.ID) || !validCommerceApplicationOrigin(origin.Origin) {
			return errors.New("Commerce Application response contains an invalid Origin")
		}
	}
	for _, returnURL := range application.ReturnURLs {
		if !uuidPattern.MatchString(returnURL.ID) || !validCommerceApplicationReturnURL(returnURL.URL) {
			return errors.New("Commerce Application response contains an invalid Return URL")
		}
	}
	for _, product := range application.Products {
		if !uuidPattern.MatchString(product.ProductID) || strings.TrimSpace(product.Title) == "" || !validProductStatus(product.Status) {
			return errors.New("Commerce Application response contains an invalid Product")
		}
	}
	if application.ActivatedAt != nil && !validTimestamp(*application.ActivatedAt) {
		return errors.New("Commerce Application response contains an invalid activatedAt")
	}
	if application.SuspendedAt != nil && !validTimestamp(*application.SuspendedAt) {
		return errors.New("Commerce Application response contains an invalid suspendedAt")
	}
	return nil
}

func (applications *CommerceApplicationsResponse) validateAPIResponse() error {
	if applications == nil || applications.Items == nil {
		return errors.New("Commerce Application list response is missing items")
	}
	for index := range applications.Items {
		if err := applications.Items[index].validateAPIResponse(); err != nil {
			return err
		}
	}
	return nil
}

func validateWebsiteWorkMutation(work MerchantWork, workID, merchantAccountID, ownershipStatus string) error {
	if work.ID != workID || work.Kind != "WEBSITE" || work.Origin != "USER_AUTHORED" || work.Website == nil ||
		work.Website.OwnershipStatus != ownershipStatus || work.Owner.Kind != "MERCHANT" || work.Owner.MerchantAccountID == nil ||
		*work.Owner.MerchantAccountID != merchantAccountID {
		return errors.New("Website Work response does not match the requested mutation")
	}
	return nil
}

func validateWorkOwner(owner WorkOwner) error {
	switch owner.Kind {
	case "USER":
		if owner.UserID == nil || !uuidPattern.MatchString(*owner.UserID) || owner.CreatorAccountID != nil || owner.MerchantAccountID != nil {
			return errors.New("invalid User Work owner")
		}
	case "CREATOR":
		if owner.CreatorAccountID == nil || !uuidPattern.MatchString(*owner.CreatorAccountID) || owner.UserID != nil || owner.MerchantAccountID != nil {
			return errors.New("invalid Creator Work owner")
		}
	case "MERCHANT":
		if owner.MerchantAccountID == nil || !uuidPattern.MatchString(*owner.MerchantAccountID) || owner.UserID != nil || owner.CreatorAccountID != nil {
			return errors.New("invalid Merchant Work owner")
		}
	default:
		return errors.New("invalid Work owner kind")
	}
	return nil
}

func validWorkKind(kind string) bool {
	return kind == "SKILL" || kind == "SERVICE" || kind == "WEBSITE"
}

func validWorkOrigin(origin string) bool {
	return origin == "USER_AUTHORED" || origin == "PLATFORM_AUTHORED" || origin == "PLATFORM_GENERATED"
}

func validWorkStatus(status string) bool {
	return status == "DRAFT" || status == "PUBLISHED" || status == "SUSPENDED" || status == "ARCHIVED"
}

func validWebsiteOwnershipStatus(status string) bool {
	return status == "UNVERIFIED" || status == "VERIFIED" || status == "REVOKED"
}

func validWebsiteVerificationStatus(status string) bool {
	return status == "PENDING" || status == "VERIFIED" || status == "FAILED" || status == "EXPIRED"
}

func validCommerceApplicationKind(kind string) bool {
	return kind == "HOSTED_CHECKOUT" || kind == "SKILL_RUNTIME" || kind == "WEBSITE_WIDGET"
}

func validCommerceApplicationEnvironment(environment string) bool {
	return environment == "SANDBOX" || environment == "PRODUCTION"
}

func validCommerceApplicationStatus(status string) bool {
	return status == "DRAFT" || status == "ACTIVE" || status == "SUSPENDED" || status == "REVOKED"
}

func validProductStatus(status string) bool {
	return status == "DRAFT" || status == "ACTIVE" || status == "SUSPENDED" || status == "ARCHIVED"
}

func validWorkSdkFeatures(features []string) bool {
	if features == nil || len(features) > 2 {
		return false
	}
	seen := make(map[string]struct{}, len(features))
	for _, feature := range features {
		if feature != "danmaku" && feature != "tip" {
			return false
		}
		if _, exists := seen[feature]; exists {
			return false
		}
		seen[feature] = struct{}{}
	}
	return true
}

func validWorkAccessFeatures(features []WorkAccessFeature) bool {
	if features == nil || len(features) > 100 {
		return false
	}
	seen := make(map[string]struct{}, len(features))
	for _, feature := range features {
		if !accessWorkFeatureKeyPattern.MatchString(feature.FeatureKey) ||
			utf16CodeUnits(strings.TrimSpace(feature.Title)) < 1 || utf16CodeUnits(strings.TrimSpace(feature.Title)) > 120 ||
			(feature.Status != "ACTIVE" && feature.Status != "DISABLED") {
			return false
		}
		if _, exists := seen[feature.FeatureKey]; exists {
			return false
		}
		seen[feature.FeatureKey] = struct{}{}
		switch feature.PolicyType {
		case "PUBLIC", "FOLLOW_OWNER":
			if feature.Price != nil || feature.ProductID != nil {
				return false
			}
		case "WORK_ENTITLEMENT":
			if feature.Price == nil || feature.Price.Currency != "CNY" || feature.Price.AmountCents < 1 ||
				feature.ProductID == nil || !uuidPattern.MatchString(*feature.ProductID) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func validWorkAccessFeatureInputs(features []WorkAccessFeatureInput) bool {
	if len(features) > 100 {
		return false
	}
	seen := make(map[string]struct{}, len(features))
	for _, feature := range features {
		if !accessWorkFeatureKeyPattern.MatchString(feature.FeatureKey) ||
			utf16CodeUnits(strings.TrimSpace(feature.Title)) < 1 || utf16CodeUnits(strings.TrimSpace(feature.Title)) > 120 ||
			(feature.Status != "ACTIVE" && feature.Status != "DISABLED") {
			return false
		}
		if _, exists := seen[feature.FeatureKey]; exists {
			return false
		}
		seen[feature.FeatureKey] = struct{}{}
		switch feature.PolicyType {
		case "PUBLIC", "FOLLOW_OWNER":
			if feature.Price != nil {
				return false
			}
		case "WORK_ENTITLEMENT":
			if feature.Price == nil || feature.Price.Currency != "CNY" || feature.Price.AmountCents < 1 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func workAccessFeaturesMatchRequest(actual []WorkAccessFeature, expected []WorkAccessFeatureInput) bool {
	if len(actual) != len(expected) || !validWorkAccessFeatures(actual) || !validWorkAccessFeatureInputs(expected) {
		return false
	}
	expectedByKey := make(map[string]WorkAccessFeatureInput, len(expected))
	for _, feature := range expected {
		expectedByKey[feature.FeatureKey] = feature
	}
	for _, feature := range actual {
		expectedFeature, exists := expectedByKey[feature.FeatureKey]
		if !exists || feature.Title != strings.TrimSpace(expectedFeature.Title) || feature.PolicyType != expectedFeature.PolicyType ||
			feature.Status != expectedFeature.Status || !workAccessPricesEqual(feature.Price, expectedFeature.Price) {
			return false
		}
	}
	return true
}

func workAccessPricesEqual(actual, expected *WorkAccessPrice) bool {
	if actual == nil || expected == nil {
		return actual == nil && expected == nil
	}
	return *actual == *expected
}

func workSdkFeaturesEqual(actual, expected []string) bool {
	if len(actual) != len(expected) || !validWorkSdkFeatures(actual) {
		return false
	}
	expectedSet := make(map[string]struct{}, len(expected))
	for _, feature := range expected {
		if feature != "danmaku" && feature != "tip" {
			return false
		}
		if _, exists := expectedSet[feature]; exists {
			return false
		}
		expectedSet[feature] = struct{}{}
	}
	for _, feature := range actual {
		if _, exists := expectedSet[feature]; !exists {
			return false
		}
	}
	return true
}

func commerceApplicationMatchesCreateRequest(application CommerceApplication, request CreateCommerceApplicationRequest) bool {
	return application.MerchantAccountID == request.MerchantAccountID && application.WorkID == request.WorkID &&
		application.Kind == request.Kind && application.Environment == request.Environment && application.DisplayName == strings.TrimSpace(request.DisplayName) &&
		application.Status == "DRAFT" && commerceOriginsEqual(application.Origins, request.Origins) &&
		commerceReturnURLsEqual(application.ReturnURLs, request.ReturnURLs)
}

func commerceApplicationMatchesUpdateRequest(application CommerceApplication, applicationID string, request UpdateCommerceApplicationRequest) bool {
	if application.ID != applicationID || application.MerchantAccountID != request.MerchantAccountID || application.Revision <= request.ExpectedRevision {
		return false
	}
	if request.DisplayName != nil && application.DisplayName != strings.TrimSpace(*request.DisplayName) {
		return false
	}
	if request.Origins != nil && !commerceOriginsEqual(application.Origins, *request.Origins) {
		return false
	}
	return request.ReturnURLs == nil || commerceReturnURLsEqual(application.ReturnURLs, *request.ReturnURLs)
}

func commerceOriginsEqual(actual []CommerceApplicationOrigin, expected []string) bool {
	values := make([]string, len(actual))
	for index, origin := range actual {
		values[index] = origin.Origin
	}
	return normalizedStringSetEqual(values, expected, normalizeCommerceApplicationOrigin)
}

func commerceReturnURLsEqual(actual []CommerceApplicationReturnURL, expected []string) bool {
	values := make([]string, len(actual))
	for index, returnURL := range actual {
		values[index] = returnURL.URL
	}
	return normalizedStringSetEqual(values, expected, normalizeCommerceApplicationReturnURL)
}

func normalizedStringSetEqual(actual, expected []string, normalize func(string) (string, bool)) bool {
	expectedSet := make(map[string]struct{}, len(expected))
	for _, value := range expected {
		normalized, ok := normalize(value)
		if !ok {
			return false
		}
		expectedSet[normalized] = struct{}{}
	}
	if len(actual) != len(expectedSet) {
		return false
	}
	seen := make(map[string]struct{}, len(actual))
	for _, value := range actual {
		normalized, ok := normalize(value)
		if !ok {
			return false
		}
		if _, exists := seen[normalized]; exists {
			return false
		}
		if _, exists := expectedSet[normalized]; !exists {
			return false
		}
		seen[normalized] = struct{}{}
	}
	return true
}

func validCommerceApplicationOrigin(value string) bool {
	normalized, ok := normalizeCommerceApplicationOrigin(value)
	return ok && value == normalized
}

func normalizeCommerceApplicationOrigin(value string) (string, bool) {
	target, err := shopURLParser.Parse(value)
	if err != nil || target.Scheme() != "https" || target.Hostname() == "" || target.Username() != "" ||
		target.Password() != "" || target.Pathname() != "/" || target.Search() != "" || target.Hash() != "" {
		return "", false
	}
	return "https://" + target.Host(), true
}

func validCommerceApplicationReturnURL(value string) bool {
	normalized, ok := normalizeCommerceApplicationReturnURL(value)
	return ok && value == normalized
}

func normalizeCommerceApplicationReturnURL(value string) (string, bool) {
	target, err := shopURLParser.Parse(value)
	if err != nil || target.Scheme() != "https" || target.Hostname() == "" || target.Username() != "" || target.Password() != "" || target.Hash() != "" {
		return "", false
	}
	return target.Href(false), true
}

func utf16CodeUnits(value string) int {
	return len(utf16.Encode([]rune(value)))
}

func validTimestamp(value string) bool {
	_, err := time.Parse(time.RFC3339, value)
	return err == nil
}

func rawJSONIsNull(value json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}

func publicationPath(publicationID string) string {
	return "/v1/creator/skill-publications/" + url.PathEscape(publicationID)
}

func decodeServerError(status int, data []byte, headerRequestID string) error {
	var serverError APIError
	_ = json.Unmarshal(data, &serverError)
	message := "ViceMe API request failed"
	switch value := serverError.Message.(type) {
	case string:
		if value != "" {
			message = value
		}
	case []any:
		var parts []string
		for _, item := range value {
			if text, ok := item.(string); ok {
				parts = append(parts, text)
			}
		}
		if len(parts) > 0 {
			message = strings.Join(parts, "; ")
		}
	}
	code := serverError.Code
	if code == "" {
		code = fmt.Sprintf("HTTP_%d", status)
	}
	exitCode, errorType := exitForStatus(status)
	cliError := output.NewError(exitCode, errorType, code, message)
	cliError.RequestID = serverError.RequestID
	if cliError.RequestID == "" {
		cliError.RequestID = headerRequestID
	}
	cliError.Retryable = (status == http.StatusTooManyRequests || status >= 500) && code != "OAUTH_PROVIDER_NOT_CONFIGURED"
	if validAPIRecoveryReference(serverError.Recovery) {
		cliError.Details = map[string]any{"recovery": *serverError.Recovery}
	}
	return cliError
}

func validAPIRecoveryReference(reference *APIRecoveryReference) bool {
	return reference != nil && reference.ResourceType == "ORDER" && len(reference.ResourceID) >= 6 && len(reference.ResourceID) <= 40
}

func RecoveryReferenceFromError(err error) (APIRecoveryReference, bool) {
	var cliError *output.Error
	if !errors.As(err, &cliError) {
		return APIRecoveryReference{}, false
	}
	details, ok := cliError.Details.(map[string]any)
	if !ok {
		return APIRecoveryReference{}, false
	}
	reference, ok := details["recovery"].(APIRecoveryReference)
	return reference, ok && validAPIRecoveryReference(&reference)
}

func exitForStatus(status int) (int, string) {
	switch status {
	case http.StatusUnauthorized:
		return output.ExitAuthentication, "authentication"
	case http.StatusForbidden:
		return output.ExitAuthentication, "authorization"
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return output.ExitNetwork, "network"
	case http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusGone, http.StatusUnprocessableEntity:
		return output.ExitValidation, "validation"
	default:
		return output.ExitInternal, "internal"
	}
}

func NormalizeAPIOrigin(raw string) (string, error) {
	base, err := validateAPIBaseURL(raw)
	if err != nil {
		return "", err
	}
	scheme := strings.ToLower(base.Scheme)
	host := strings.ToLower(base.Hostname())
	port := base.Port()
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port != "" {
		host += ":" + port
	}
	return scheme + "://" + host, nil
}

func validateAPIBaseURL(raw string) (*url.URL, error) {
	normalized, err := config.NormalizeAPIBaseURL(raw)
	if err != nil {
		return nil, errors.New("API URL must use HTTPS or loopback HTTP")
	}
	return url.Parse(normalized)
}

func withoutRedirects(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	copy := *client
	copy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &copy
}
