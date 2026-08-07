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
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/output"
)

const maxResponseBytes = 8 << 20

type TokenSource interface {
	Token(context.Context) (string, error)
}

type CredentialHeaderFunc func(*http.Request, string)

func ApplyAPIKeyCredential(request *http.Request, credential string) {
	request.Header.Set("x-api-key", credential)
}

type Client struct {
	BaseURL          string
	HTTPClient       *http.Client
	Tokens           TokenSource
	UserAgent        string
	CredentialHeader CredentialHeaderFunc
}

func NewClient(baseURL string, httpClient *http.Client, tokens TokenSource, userAgent string) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		BaseURL:          strings.TrimRight(baseURL, "/"),
		HTTPClient:       httpClient,
		Tokens:           tokens,
		UserAgent:        userAgent,
		CredentialHeader: ApplyAPIKeyCredential,
	}
}

func (c *Client) StartDeviceAuthorization(ctx context.Context) (DeviceAuthorization, error) {
	var response DeviceAuthorization
	err := c.doJSON(ctx, http.MethodPost, "/v1/cli/auth/device", map[string]string{"client_id": "viceme-cli"}, &response, false, "", nil)
	return response, err
}

func (c *Client) ExchangeDeviceToken(ctx context.Context, deviceCode string) (DeviceToken, error) {
	var response DeviceToken
	err := c.doJSON(ctx, http.MethodPost, "/v1/cli/auth/token", DeviceTokenRequest{DeviceCode: deviceCode}, &response, false, "", nil)
	return response, err
}

func (c *Client) RefreshDeviceToken(ctx context.Context, refreshToken, clientRequestID string) (DeviceToken, error) {
	var response DeviceToken
	err := c.doJSON(ctx, http.MethodPost, "/v1/cli/auth/refresh", RefreshTokenRequest{RefreshToken: refreshToken, ClientRequestID: clientRequestID}, &response, false, "", nil)
	return response, err
}

func (c *Client) Revoke(ctx context.Context) error {
	var response RevokeResponse
	return c.doJSON(ctx, http.MethodPost, "/v1/cli/auth/revoke", struct{}{}, &response, true, "", nil)
}

func (c *Client) RevokeWithToken(ctx context.Context, accessToken string) error {
	var response RevokeResponse
	return c.doJSON(ctx, http.MethodPost, "/v1/cli/auth/revoke", struct{}{}, &response, false, accessToken, nil)
}

func (c *Client) CreateCreatorApp(ctx context.Context, request CreateCreatorAppRequest) (CreatorApp, error) {
	var response CreatorApp
	err := c.doJSON(ctx, http.MethodPost, "/v1/creator-apps", request, &response, true, "", nil)
	return response, err
}

func (c *Client) ListCreatorApps(ctx context.Context) (CreatorAppsResponse, error) {
	var response CreatorAppsResponse
	err := c.doJSON(ctx, http.MethodGet, "/v1/creator-apps", nil, &response, true, "", nil)
	return response, err
}

func (c *Client) GetCreatorApp(ctx context.Context, appID string) (CreatorApp, error) {
	var response CreatorApp
	err := c.doJSON(ctx, http.MethodGet, "/v1/creator-apps/"+url.PathEscape(appID), nil, &response, true, "", nil)
	return response, err
}

func (c *Client) AddCreatorAppOrigin(ctx context.Context, appID, environment, origin string) (OriginResponse, error) {
	var response OriginResponse
	endpoint := "/v1/creator-apps/" + url.PathEscape(appID) + "/environments/" + url.PathEscape(environment) + "/origins"
	err := c.doJSON(ctx, http.MethodPost, endpoint, AddOriginRequest{Origin: origin}, &response, true, "", nil)
	return response, err
}

func (c *Client) CapabilityCatalog(ctx context.Context) (CapabilityCatalog, error) {
	var response CapabilityCatalog
	err := c.doJSON(ctx, http.MethodGet, "/v1/creator-apps/capabilities/catalog", nil, &response, true, "", nil)
	return response, err
}

func (c *Client) AddCreatorAppCapability(ctx context.Context, appID, environment string, request AddCapabilityRequest) (CreatorAppCapability, error) {
	var response CreatorAppCapability
	endpoint := "/v1/creator-apps/" + url.PathEscape(appID) + "/environments/" + url.PathEscape(environment) + "/capabilities"
	err := c.doJSON(ctx, http.MethodPost, endpoint, request, &response, true, "", nil)
	return response, err
}

func (c *Client) GetCreatorAppCapability(ctx context.Context, appID, environment, capability string) (CreatorAppCapability, error) {
	var response CreatorAppCapability
	endpoint := "/v1/creator-apps/" + url.PathEscape(appID) + "/environments/" + url.PathEscape(environment) + "/capabilities/" + url.PathEscape(capability)
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, true, "", nil)
	return response, err
}

func (c *Client) CreateCommerceOffer(ctx context.Context, appID, environment string, request CreateCommerceOfferRequest) (CommerceOffer, error) {
	var response CommerceOffer
	endpoint := "/v1/creator-apps/" + url.PathEscape(appID) + "/environments/" + url.PathEscape(environment) + "/commerce/offers"
	err := c.doJSON(ctx, http.MethodPost, endpoint, request, &response, true, "", nil)
	return response, err
}

func (c *Client) ListCommerceOffers(ctx context.Context, appID, environment string) (CommerceOffersResponse, error) {
	var response CommerceOffersResponse
	endpoint := "/v1/creator-apps/" + url.PathEscape(appID) + "/environments/" + url.PathEscape(environment) + "/commerce/offers"
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, true, "", nil)
	return response, err
}

func (c *Client) UpsertCreatorAppListing(ctx context.Context, appID string, request UpsertCreatorAppListingRequest) (CreatorAppListing, error) {
	var response CreatorAppListing
	endpoint := "/v1/creator-apps/" + url.PathEscape(appID) + "/listing"
	err := c.doJSON(ctx, http.MethodPut, endpoint, request, &response, true, "", nil)
	return response, err
}

func (c *Client) GetCreatorAppListing(ctx context.Context, appID string) (CreatorAppListing, error) {
	var response CreatorAppListing
	endpoint := "/v1/creator-apps/" + url.PathEscape(appID) + "/listing"
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, true, "", nil)
	return response, err
}

func (c *Client) ListCreatorLedger(ctx context.Context, appID, cursor string, limit int) (CreatorLedgerResponse, error) {
	var response CreatorLedgerResponse
	query := url.Values{}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	if limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", limit))
	}
	endpoint := "/v1/creator-apps/" + url.PathEscape(appID) + "/commerce/ledger"
	if encoded := query.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, true, "", nil)
	return response, err
}

func (c *Client) GetPublicAppContext(ctx context.Context, publishableKey, origin string) (PublicAppContext, error) {
	var response PublicAppContext
	headers := http.Header{}
	if origin != "" {
		headers.Set("Origin", origin)
	}
	err := c.doJSON(ctx, http.MethodGet, "/v1/creator-app-context/"+url.PathEscape(publishableKey), nil, &response, false, "", headers)
	return response, err
}

// Runtime API methods (skill-app-platform.md §8 / runtime-headless-api.md).

func (c *Client) GetRuntimeContract(ctx context.Context, runtimeReleaseID string) (RuntimeContractResponse, error) {
	var response RuntimeContractResponse
	err := c.doJSON(ctx, http.MethodGet, "/v1/managed-apps/runtime-contract/"+url.PathEscape(runtimeReleaseID), nil, &response, true, "", nil)
	return response, err
}

func (c *Client) CreateRuntimeRun(ctx context.Context, request CreateRuntimeRunRequest, origin string) (CreateRuntimeRunResponse, error) {
	var response CreateRuntimeRunResponse
	headers := http.Header{}
	if origin != "" {
		headers.Set("Origin", origin)
	}
	err := c.doJSON(ctx, http.MethodPost, "/v1/runtime/runs", request, &response, false, "", headers)
	return response, err
}

// applyRuntimeTicket attaches a Runtime Ticket as a Bearer token. The Shop runs
// surface accepts it as `Authorization: Bearer <ticket>` (or the explicit
// `x-viceme-runtime-ticket` header); a publishable key + origin alone are
// never a sufficient identity, so callers that omit the ticket are rejected
// with RUNTIME_AUTH_REQUIRED.
func applyRuntimeTicket(headers http.Header, runtimeTicket string) {
	if runtimeTicket == "" {
		return
	}
	headers.Set("Authorization", "Bearer "+runtimeTicket)
}

func (c *Client) GetRuntimeRun(ctx context.Context, runID, publishableKey, origin, runtimeTicket string) (RuntimeRunDetail, error) {
	var response RuntimeRunDetail
	if err := validateRunID(runID); err != nil {
		return response, err
	}
	endpoint := "/v1/runtime/runs/" + url.PathEscape(runID) + "?publishableKey=" + url.QueryEscape(publishableKey)
	headers := http.Header{}
	if origin != "" {
		headers.Set("Origin", origin)
	}
	applyRuntimeTicket(headers, runtimeTicket)
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, false, "", headers)
	return response, err
}

func (c *Client) CancelRuntimeRun(ctx context.Context, runID, publishableKey, origin, runtimeTicket string) (CancelRuntimeRunResponse, error) {
	var response CancelRuntimeRunResponse
	if err := validateRunID(runID); err != nil {
		return response, err
	}
	endpoint := "/v1/runtime/runs/" + url.PathEscape(runID) + "/cancel?publishableKey=" + url.QueryEscape(publishableKey)
	headers := http.Header{}
	if origin != "" {
		headers.Set("Origin", origin)
	}
	applyRuntimeTicket(headers, runtimeTicket)
	err := c.doJSON(ctx, http.MethodPost, endpoint, nil, &response, false, "", headers)
	return response, err
}

func (c *Client) ListRuntimeRuns(ctx context.Context, publishableKey, origin, runtimeTicket, cursor string, limit int) (ListRuntimeRunsResponse, error) {
	var response ListRuntimeRunsResponse
	endpoint := "/v1/runtime/runs?publishableKey=" + url.QueryEscape(publishableKey)
	if cursor != "" {
		endpoint += "&cursor=" + url.QueryEscape(cursor)
	}
	if limit > 0 {
		endpoint += "&limit=" + fmt.Sprintf("%d", limit)
	}
	headers := http.Header{}
	if origin != "" {
		headers.Set("Origin", origin)
	}
	applyRuntimeTicket(headers, runtimeTicket)
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, false, "", headers)
	return response, err
}

// DownloadArtifact fetches the bytes at a Runtime Artifact's short-lived signed
// downloadUrl. The URL is opaque to the CLI and may point at a storage host
// outside the API base, so it is used verbatim without API base resolution —
// but the scheme is still confined to HTTPS (or HTTP loopback), so a
// compromised server cannot point the CLI at internal addresses (#66 review
// P2).
func (c *Client) DownloadArtifact(ctx context.Context, downloadURL string) (io.ReadCloser, error) {
	parsed, err := ValidateDownloadURL(downloadURL)
	if err != nil {
		return nil, output.Validation("artifact_download_url", "artifact download URL is missing or invalid")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, output.Internal("artifact_download_request", "failed to create the artifact download request", err)
	}
	request.Header.Set("Accept", "*/*")
	if c.UserAgent != "" {
		request.Header.Set("User-Agent", c.UserAgent)
	}
	response, err := WithoutRedirects(c.HTTPClient).Do(request)
	if err != nil {
		return nil, output.Network("artifact_download", "failed to download the artifact", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer response.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
		return nil, decodeServerError(response.StatusCode, data)
	}
	return response.Body, nil
}

// Runtime Ticket issuance (Slice F authorization).
//
// POST /v1/runtime/tickets exchanges a CLI device token for a short-lived
// Runtime Ticket scoped to one App environment (publishable key + origin). The
// CLI already holds a valid access token from `viceme auth login`, so the
// initial implementation authenticates with it (authenticated=true). This
// requires the Shop API to accept CLI device tokens for the ticket exchange — a
// cross-repo dependency tracked alongside this change; until the Shop honours
// it, callers should obtain a Runtime Ticket from the ViceMe web UI or via the
// `--browser` device-code flow on `runtime ticket`.
func (c *Client) CreateRuntimeTicket(ctx context.Context, publishableKey, origin string) (CreateRuntimeTicketResponse, error) {
	var response CreateRuntimeTicketResponse
	headers := http.Header{}
	if origin != "" {
		headers.Set("Origin", origin)
	}
	err := c.doJSON(ctx, http.MethodPost, "/v1/runtime/tickets", map[string]string{
		"publishableKey": publishableKey,
		"origin":         origin,
	}, &response, true, "", headers)
	return response, err
}

// CreateRuntimeTicketWithDeviceCode completes the --browser device-code flow:
// the user authorizes in the web UI and receives a one-time code, which the CLI
// exchanges here for a Runtime Ticket. The code is short-lived and single-use.
func (c *Client) CreateRuntimeTicketWithDeviceCode(ctx context.Context, publishableKey, origin, code string) (CreateRuntimeTicketResponse, error) {
	var response CreateRuntimeTicketResponse
	headers := http.Header{}
	if origin != "" {
		headers.Set("Origin", origin)
	}
	err := c.doJSON(ctx, http.MethodPost, "/v1/runtime/tickets/exchange", map[string]string{
		"publishableKey": publishableKey,
		"origin":         origin,
		"code":           code,
	}, &response, true, "", headers)
	return response, err
}

// Managed App API methods (skill-app-platform.md §10/§11).

func (c *Client) GetManagedAppTemplate(ctx context.Context, name string) (ManagedAppTemplate, error) {
	var response ManagedAppTemplate
	err := c.doJSON(ctx, http.MethodGet, "/v1/managed-apps/templates/"+url.PathEscape(name), nil, &response, true, "", nil)
	return response, err
}

func (c *Client) GetManagedAppRuntimeContract(ctx context.Context, runtimeReleaseID string) (ManagedAppRuntimeContract, error) {
	var response ManagedAppRuntimeContract
	err := c.doJSON(ctx, http.MethodGet, "/v1/managed-apps/runtime-contract/"+url.PathEscape(runtimeReleaseID), nil, &response, true, "", nil)
	return response, err
}

func (c *Client) InitManagedApp(ctx context.Context, request InitManagedAppRequest) (InitManagedAppResponse, error) {
	var response InitManagedAppResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/managed-apps/apps/init", request, &response, true, "", nil)
	return response, err
}

// UploadManagedAppSource uploads a source archive via multipart/form-data to
// POST /v1/managed-apps/releases/source. The Shop route is FileInterceptor-based
// and expects text fields alongside a `file` part; a JSON body is rejected.
func (c *Client) UploadManagedAppSource(ctx context.Context, request UploadSourceRequest, archivePath string) (UploadSourceResponse, error) {
	var response UploadSourceResponse
	fields := map[string]string{
		"appId":                 request.AppID,
		"candidateId":           request.CandidateID,
		"runtimeReleaseId":      request.RuntimeReleaseID,
		"runtimeContractDigest": request.RuntimeContractDigest,
		"templateName":          request.TemplateName,
		"templateVersion":       request.TemplateVersion,
		"templateDigest":        request.TemplateDigest,
		// appSdkVersion is server-authoritative (template record), not
		// client-supplied.
	}
	err := c.doMultipart(ctx, http.MethodPost, "/v1/managed-apps/releases/source", fields, "file", archivePath, &response)
	return response, err
}

// UploadManagedAppBuildArtifact uploads the locally-built dist archive via
// multipart/form-data to POST /v1/managed-apps/releases/artifact. The API never
// executes untrusted author code: the CLI builds locally and uploads the result.
func (c *Client) UploadManagedAppBuildArtifact(ctx context.Context, request UploadBuildArtifactRequest, archivePath string) (UploadBuildArtifactResponse, error) {
	var response UploadBuildArtifactResponse
	fields := map[string]string{
		"appId":       request.AppID,
		"candidateId": request.CandidateID,
	}
	err := c.doMultipart(ctx, http.MethodPost, "/v1/managed-apps/releases/artifact", fields, "file", archivePath, &response)
	return response, err
}

func (c *Client) CreateManagedAppPreview(ctx context.Context, appID, candidateID string) (CreatePreviewResponse, error) {
	var response CreatePreviewResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/managed-apps/releases/preview", map[string]string{
		"appId":       appID,
		"candidateId": candidateID,
	}, &response, true, "", nil)
	return response, err
}

func (c *Client) PublishManagedApp(ctx context.Context, request PublishReleaseRequest) (PublishReleaseResponse, error) {
	var response PublishReleaseResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/managed-apps/releases/publish", request, &response, true, "", nil)
	return response, err
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, requestBody, responseBody any, authenticated bool, explicitToken string, headers http.Header) error {
	base, err := validateAPIBaseURL(c.BaseURL)
	if err != nil {
		return output.Validation("api_base_url", "ViceMe API base URL must use HTTPS; HTTP is allowed only for localhost or loopback development")
	}
	relative, err := url.Parse(endpoint)
	if err != nil || relative.IsAbs() || relative.Host != "" {
		return output.Internal("request_endpoint", "failed to construct the ViceMe API endpoint", err)
	}
	base.Path = path.Join(base.Path, relative.Path)
	base.RawQuery = relative.RawQuery

	var encoded []byte
	if requestBody != nil {
		encoded, err = json.Marshal(requestBody)
		if err != nil {
			return output.Internal("request_encode", "failed to encode the API request", err)
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, base.String(), bytes.NewReader(encoded))
	if err != nil {
		return output.Internal("request_create", "failed to create the ViceMe API request", err)
	}
	request.Header.Set("Accept", "application/json")
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.UserAgent != "" {
		request.Header.Set("User-Agent", c.UserAgent)
	}
	for key, values := range headers {
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}
	token := explicitToken
	if authenticated && token == "" {
		if c.Tokens == nil {
			return output.Authentication("not_logged_in", "not logged in to ViceMe")
		}
		token, err = c.Tokens.Token(ctx)
		if err != nil {
			return err
		}
	}
	if token != "" {
		apply := c.CredentialHeader
		if apply == nil {
			apply = ApplyAPIKeyCredential
		}
		apply(request, token)
	}

	response, err := WithoutRedirects(c.HTTPClient).Do(request)
	if err != nil {
		return output.Network("transport", "failed to reach the ViceMe API", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return output.Network("response_read", "failed to read the ViceMe API response", err)
	}
	if len(data) > maxResponseBytes {
		return output.Internal("response_too_large", "ViceMe API response exceeded the client limit", nil)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return decodeServerError(response.StatusCode, data)
	}
	if responseBody == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, responseBody); err != nil {
		return output.Internal("response_decode", "ViceMe API returned an invalid JSON response", err)
	}
	return nil
}

func NormalizeAPIOrigin(raw string) (string, error) {
	base, err := validateAPIBaseURL(raw)
	if err != nil {
		return "", err
	}
	return normalizedAPIOrigin(base), nil
}

// doMultipart performs an authenticated multipart/form-data upload. `fileField`
// names the part carrying the file at `filePath`; `fields` are the text fields.
// The Shop managed-app source/artifact routes are FileInterceptor-based and
// reject a JSON body, so this helper is required for those endpoints.
func (c *Client) doMultipart(ctx context.Context, method, endpoint string, fields map[string]string, fileField, filePath string, responseBody any) error {
	base, err := validateAPIBaseURL(c.BaseURL)
	if err != nil {
		return output.Validation("api_base_url", "ViceMe API base URL must use HTTPS; HTTP is allowed only for localhost or loopback development")
	}
	relative, err := url.Parse(endpoint)
	if err != nil || relative.IsAbs() || relative.Host != "" {
		return output.Internal("request_endpoint", "failed to construct the ViceMe API endpoint", err)
	}
	base.Path = path.Join(base.Path, relative.Path)
	base.RawQuery = relative.RawQuery

	file, err := os.Open(filePath)
	if err != nil {
		return output.Validation("file_open", fmt.Sprintf("cannot open upload file %q: %v", filePath, err))
	}
	defer file.Close()
	fileInfo, err := file.Stat()
	if err != nil {
		return output.Validation("file_stat", fmt.Sprintf("cannot inspect upload file %q: %v", filePath, err))
	}
	if !fileInfo.Mode().IsRegular() {
		return output.Validation("file_type", "upload file must be a regular file")
	}

	pipeReader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)
	go func() {
		var writeErr error
		defer func() {
			closeErr := writer.Close()
			_ = pipeWriter.CloseWithError(errors.Join(writeErr, closeErr))
		}()
		for name, value := range fields {
			if writeErr = writer.WriteField(name, value); writeErr != nil {
				return
			}
		}
		part, partErr := writer.CreateFormFile(fileField, filepath.Base(filePath))
		if partErr != nil {
			writeErr = partErr
			return
		}
		if _, writeErr = io.Copy(part, file); writeErr != nil {
			return
		}
	}()

	request, err := http.NewRequestWithContext(ctx, method, base.String(), pipeReader)
	if err != nil {
		_ = pipeReader.Close()
		return output.Internal("request_create", "failed to create the ViceMe API request", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if c.UserAgent != "" {
		request.Header.Set("User-Agent", c.UserAgent)
	}
	if c.Tokens == nil {
		_ = pipeReader.Close()
		return output.Authentication("not_logged_in", "not logged in to ViceMe")
	}
	token, err := c.Tokens.Token(ctx)
	if err != nil {
		_ = pipeReader.Close()
		return err
	}
	apply := c.CredentialHeader
	if apply == nil {
		apply = ApplyAPIKeyCredential
	}
	apply(request, token)

	response, err := WithoutRedirects(c.HTTPClient).Do(request)
	if err != nil {
		return output.Network("transport", "failed to reach the ViceMe API", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return output.Network("response_read", "failed to read the ViceMe API response", err)
	}
	if len(data) > maxResponseBytes {
		return output.Internal("response_too_large", "ViceMe API response exceeded the client limit", nil)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return decodeServerError(response.StatusCode, data)
	}
	if responseBody == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, responseBody); err != nil {
		return output.Internal("response_decode", "ViceMe API returned an invalid JSON response", err)
	}
	return nil
}

// NormalizeAPIBaseURL canonicalizes the credential and request namespace while
// preserving a meaningful API path. Query strings and fragments are rejected
// by validateAPIBaseURL because they are not stable endpoint authority.
func NormalizeAPIBaseURL(raw string) (string, error) {
	base, err := validateAPIBaseURL(raw)
	if err != nil {
		return "", err
	}
	origin := normalizedAPIOrigin(base)
	basePath := strings.TrimRight(base.EscapedPath(), "/")
	if basePath == "" {
		return origin, nil
	}
	return origin + basePath, nil
}

func normalizedAPIOrigin(base *url.URL) string {
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
	return scheme + "://" + host
}

// WithoutRedirects returns a copy of client that never follows HTTP
// redirects. Signed artifact/template fetches must not be redirected onto an
// internal host by a compromised server.
func WithoutRedirects(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	copy := *client
	copy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &copy
}

// ValidateDownloadURL restricts a fetch to HTTPS (HTTP allowed only for
// loopback development), matching validateAPIBaseURL's transport policy.
// Signed artifact and template URLs are opaque and may point at storage hosts
// outside the API base, but a compromised server must not be able to point the
// CLI at internal addresses such as cloud metadata endpoints (#66/#67 review
// P2).
func ValidateDownloadURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return nil, errors.New("URL is missing or invalid")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		return parsed, nil
	case "http":
		host := parsed.Hostname()
		if strings.EqualFold(host, "localhost") {
			return parsed, nil
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return parsed, nil
		}
		return nil, errors.New("HTTP URL is allowed only for loopback development")
	default:
		return nil, errors.New("URL must use HTTPS")
	}
}

func validateAPIBaseURL(raw string) (*url.URL, error) {
	base, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || base.Hostname() == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || base.Opaque != "" {
		return nil, errors.New("invalid API URL")
	}
	switch strings.ToLower(base.Scheme) {
	case "https":
		return base, nil
	case "http":
		host := base.Hostname()
		if strings.EqualFold(host, "localhost") {
			return base, nil
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return base, nil
		}
		return nil, errors.New("HTTP API URL is allowed only for loopback development")
	default:
		return nil, errors.New("API URL must use HTTPS")
	}
}

func decodeServerError(status int, data []byte) error {
	var response struct {
		Code      string          `json:"code"`
		Message   json.RawMessage `json:"message"`
		RequestID string          `json:"requestId"`
	}
	_ = json.Unmarshal(data, &response)
	message := decodeMessage(response.Message)
	if message == "" {
		message = fmt.Sprintf("ViceMe API returned HTTP %d", status)
	}
	subtype := strings.ToLower(response.Code)
	if subtype == "" {
		subtype = strings.ToLower(strings.ReplaceAll(http.StatusText(status), " ", "_"))
	}
	code, typ := exitForStatus(status)
	result := output.NewError(code, typ, subtype, message)
	result.Retryable = status == http.StatusTooManyRequests || status >= 500
	if response.RequestID != "" {
		result.Details = map[string]any{"request_id": response.RequestID}
	}
	return result
}

func decodeMessage(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		return strings.Join(list, "; ")
	}
	return ""
}

func exitForStatus(status int) (int, string) {
	switch status {
	case http.StatusBadRequest, http.StatusNotFound, http.StatusGone, http.StatusConflict, http.StatusUnsupportedMediaType, http.StatusUnprocessableEntity:
		return output.ExitValidation, "validation"
	case http.StatusUnauthorized:
		return output.ExitAuthentication, "authentication"
	case http.StatusForbidden:
		return output.ExitAuthentication, "authorization"
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return output.ExitNetwork, "network"
	default:
		return output.ExitInternal, "internal"
	}
}

func IsSubtype(err error, subtype string) bool {
	var cliErr *output.Error
	return errors.As(err, &cliErr) && cliErr.Subtype == subtype
}
