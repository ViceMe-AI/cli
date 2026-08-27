package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
)

const maxResponseBytes = 8 << 20

var (
	sdkWorkKeyPattern   = regexp.MustCompile(`^wrk_[A-Za-z0-9_-]{4,124}$`)
	uuidPattern         = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	publicDomainPattern = regexp.MustCompile(
		`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`,
	)
	ipv4DomainPattern     = regexp.MustCompile(`^(?:\d{1,3}\.){3}\d{1,3}$`)
	reservedDomainPattern = regexp.MustCompile(`(?:^|\.)(?:localhost|local|test|invalid|example)$`)
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

func (c *Client) Revoke(ctx context.Context, accessToken string) error {
	return c.doJSON(ctx, http.MethodPost, "/v1/cli/auth/logout", struct{}{}, nil, accessToken)
}

func (c *Client) CreateSdkWork(ctx context.Context, request CreateSdkWorkRequest) (SdkWork, error) {
	var response SdkWork
	err := c.doJSON(ctx, http.MethodPost, "/v1/cli/sdk-works", request, &response, "@stored")
	if err == nil && (response.DisplayName != request.DisplayName || response.Status != "DRAFT" || len(response.Features) != 0 || len(response.Capabilities) != 0) {
		err = invalidAPIResponse(errors.New("created SDK Work does not match the requested empty Draft"))
	}
	return response, err
}

func (c *Client) PublishCreatorWebsite(ctx context.Context, request PublishCreatorWebsiteRequest) (SdkWork, error) {
	var response SdkWork
	err := c.doJSON(ctx, http.MethodPost, "/v1/cli/sdk-works/publish", request, &response, "@stored")
	if err == nil && (response.CreatorWorkID == nil || response.Publication == nil ||
		response.DisplayName != request.DisplayName || response.Publication.ClientWorkID != request.ClientWorkID ||
		response.Publication.SourceDigest != request.SourceDigest) {
		err = invalidAPIResponse(errors.New("published creator website does not match the request"))
	}
	return response, err
}

func (c *Client) AuthorizeWebsiteCoverUpload(ctx context.Context, request AuthorizeWebsiteCoverUploadRequest) (UploadAuthorization, error) {
	var response UploadAuthorization
	err := c.doJSON(ctx, http.MethodPost, "/v1/cli/sdk-works/cover-upload-authorizations", request, &response, "@stored")
	return response, err
}

func (c *Client) ListSdkWorks(ctx context.Context) (SdkWorks, error) {
	var response SdkWorks
	err := c.doJSON(ctx, http.MethodGet, "/v1/cli/sdk-works", nil, &response, "@stored")
	return response, err
}

func (c *Client) GetSdkWork(ctx context.Context, workKey string) (SdkWork, error) {
	var response SdkWork
	err := c.doJSON(ctx, http.MethodGet, "/v1/cli/sdk-works/"+url.PathEscape(workKey), nil, &response, "@stored")
	if err == nil && response.WorkKey != workKey {
		err = invalidAPIResponse(errors.New("SDK Work response does not match the requested work key"))
	}
	return response, err
}

func (c *Client) ApplySdkWork(ctx context.Context, workKey string, request ApplySdkWorkRequest) (SdkWork, error) {
	var response SdkWork
	err := c.doJSON(ctx, http.MethodPut, "/v1/cli/sdk-works/"+url.PathEscape(workKey), request, &response, "@stored")
	if err == nil && (response.WorkKey != workKey || response.ConfigVersion <= request.ExpectedConfigVersion ||
		response.DisplayName != request.DisplayName || response.Status != request.Status || !sdkWorkFeaturesEqual(response.Features, request.Features)) {
		err = invalidAPIResponse(errors.New("applied SDK Work does not match the requested configuration"))
	}
	return response, err
}

func (c *Client) DeleteSdkWork(ctx context.Context, workKey string) (SdkWork, error) {
	var response SdkWork
	err := c.doJSON(ctx, http.MethodDelete, "/v1/cli/sdk-works/"+url.PathEscape(workKey), nil, &response, "@stored")
	if err == nil && response.WorkKey != workKey {
		err = invalidAPIResponse(errors.New("deleted SDK Work response does not match the requested work key"))
	}
	return response, err
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

func (c *Client) CreateCreatorApp(ctx context.Context, request CreateCreatorAppRequest) (CreatorApp, error) {
	var response CreatorApp
	err := c.doJSON(ctx, http.MethodPost, "/v1/cli/creator-apps", request, &response, "@stored")
	if err == nil && (response.Name != request.Name || response.Kind != "EXTERNAL") {
		err = invalidAPIResponse(errors.New("created Creator App does not match the request"))
	}
	return response, err
}

func (c *Client) ListCreatorApps(ctx context.Context) (CreatorAppsResponse, error) {
	var response CreatorAppsResponse
	err := c.doJSON(ctx, http.MethodGet, "/v1/cli/creator-apps", nil, &response, "@stored")
	return response, err
}

func (c *Client) AddCreatorAppDomain(ctx context.Context, appID, domain string) (CreatorApp, error) {
	var response CreatorApp
	err := c.doJSON(ctx, http.MethodPut, "/v1/cli/creator-apps/"+url.PathEscape(appID)+"/domains", AddCreatorAppDomainRequest{Domain: domain}, &response, "@stored")
	if err == nil && (response.ID != appID || !creatorAppHasDomain(response, domain, false)) {
		err = invalidAPIResponse(errors.New("Creator App domain response does not contain the requested pending domain"))
	}
	return response, err
}

func (c *Client) VerifyCreatorAppDomain(ctx context.Context, appID, domain string) (CreatorApp, error) {
	var response CreatorApp
	err := c.doJSON(ctx, http.MethodPost, "/v1/cli/creator-apps/"+url.PathEscape(appID)+"/domains/"+url.PathEscape(domain)+"/verify", struct{}{}, &response, "@stored")
	if err == nil && (response.ID != appID || !creatorAppHasDomain(response, domain, true)) {
		err = invalidAPIResponse(errors.New("Creator App verification response does not contain the verified domain"))
	}
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
	if err := json.Unmarshal(data, responseBody); err != nil {
		return invalidAPIResponse(err)
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

func (work *SdkWork) validateAPIResponse() error {
	if work == nil || !sdkWorkKeyPattern.MatchString(work.WorkKey) || strings.TrimSpace(work.DisplayName) == "" ||
		strings.TrimSpace(work.Status) == "" || work.ConfigVersion < 1 || work.Features == nil || work.Capabilities == nil ||
		strings.TrimSpace(work.CreatedAt) == "" || strings.TrimSpace(work.UpdatedAt) == "" {
		return errors.New("SDK Work response is missing required fields")
	}
	for _, feature := range work.Features {
		if strings.TrimSpace(feature.FeatureKey) == "" || strings.TrimSpace(feature.Title) == "" ||
			strings.TrimSpace(feature.Policy.Type) == "" || strings.TrimSpace(feature.Status) == "" {
			return errors.New("SDK Work feature response is missing required fields")
		}
		priced := feature.PriceCents != nil && *feature.PriceCents > 0
		if (feature.Policy.Type == "WORK_ENTITLEMENT") != priced {
			return errors.New("SDK Work feature response contains an invalid price")
		}
	}
	if work.Publication != nil && (!uuidPattern.MatchString(work.Publication.ClientWorkID) ||
		!uuidPattern.MatchString(work.Publication.ReleaseID) || work.Publication.Version < 1 ||
		strings.TrimSpace(work.Publication.SourceDigest) == "" || strings.TrimSpace(work.Publication.PublishedAt) == "") {
		return errors.New("SDK Work publication response is missing required fields")
	}
	for _, capability := range work.Capabilities {
		if strings.TrimSpace(capability) == "" {
			return errors.New("SDK Work capability response contains an empty value")
		}
	}
	return nil
}

func (works *SdkWorks) validateAPIResponse() error {
	if works == nil || works.Works == nil {
		return errors.New("SDK Work list response is missing works")
	}
	for index := range works.Works {
		if err := works.Works[index].validateAPIResponse(); err != nil {
			return err
		}
	}
	return nil
}

func (app *CreatorApp) validateAPIResponse() error {
	if app == nil || !uuidPattern.MatchString(app.ID) ||
		(app.Kind != "EXTERNAL" && app.Kind != "VICEME_HOSTED") ||
		strings.TrimSpace(app.Name) == "" || app.Domains == nil {
		return errors.New("Creator App response is missing required fields")
	}
	if _, err := time.Parse(time.RFC3339, app.CreatedAt); err != nil {
		return errors.New("Creator App response contains an invalid createdAt")
	}
	for _, domain := range app.Domains {
		name := strings.TrimSpace(domain.Domain)
		if name != domain.Domain || len(name) > 253 || !publicDomainPattern.MatchString(name) ||
			ipv4DomainPattern.MatchString(name) || reservedDomainPattern.MatchString(name) ||
			(domain.VerificationToken != nil && len(strings.TrimSpace(*domain.VerificationToken)) < 32) {
			return errors.New("Creator App domain response is missing required fields")
		}
	}
	return nil
}

func (apps *CreatorAppsResponse) validateAPIResponse() error {
	if apps == nil || apps.Items == nil {
		return errors.New("Creator App list response is missing items")
	}
	for index := range apps.Items {
		if err := apps.Items[index].validateAPIResponse(); err != nil {
			return err
		}
	}
	return nil
}

func sdkWorkFeaturesEqual(actual, expected []SdkWorkFeatureConfig) bool {
	if len(actual) != len(expected) {
		return false
	}
	byKey := make(map[string]SdkWorkFeatureConfig, len(actual))
	for _, feature := range actual {
		if _, exists := byKey[feature.FeatureKey]; exists {
			return false
		}
		byKey[feature.FeatureKey] = feature
	}
	for _, feature := range expected {
		candidate, exists := byKey[feature.FeatureKey]
		if !exists || candidate.FeatureKey != feature.FeatureKey || candidate.Title != feature.Title ||
			candidate.Policy != feature.Policy || candidate.Status != feature.Status ||
			!nullableIntsEqual(candidate.PriceCents, feature.PriceCents) {
			return false
		}
	}
	return true
}

func nullableIntsEqual(left, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func creatorAppHasDomain(app CreatorApp, domain string, verified bool) bool {
	for _, candidate := range app.Domains {
		if strings.EqualFold(candidate.Domain, domain) && candidate.Verified == verified {
			if verified || (candidate.VerificationToken != nil && strings.TrimSpace(*candidate.VerificationToken) != "") {
				return true
			}
		}
	}
	return false
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
	cliError.Retryable = status == http.StatusTooManyRequests || status >= 500
	return cliError
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
