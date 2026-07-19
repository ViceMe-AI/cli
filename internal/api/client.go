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
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/output"
)

const maxResponseBytes = 8 << 20

type TokenSource interface {
	Token(context.Context) (string, error)
}

// CredentialHeaderFunc applies a stored CLI credential to an API request.
// Device login currently issues scoped API keys, so the default transport is
// x-api-key. Keeping this injectable prevents a future credential type from
// requiring route-by-route client changes.
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
	err := c.doJSON(ctx, http.MethodPost, "/v1/cli/auth/device", struct{}{}, &response, false, "")
	if err == nil && response.VerificationURLComplete != "" {
		// The complete URL carries the one-time user code and opens the exact
		// authorization request in the browser. Keep verification_url as the
		// canonical agent-facing field while retaining the explicit complete
		// field for callers that understand the full server contract.
		response.VerificationURL = response.VerificationURLComplete
	}
	return response, err
}

func (c *Client) ExchangeDeviceToken(ctx context.Context, deviceCode string) (DeviceToken, error) {
	var response DeviceToken
	err := c.doJSON(ctx, http.MethodPost, "/v1/cli/auth/token", DeviceTokenRequest{DeviceCode: deviceCode}, &response, false, "")
	return response, err
}

func (c *Client) Revoke(ctx context.Context, accessToken string) error {
	return c.doJSON(ctx, http.MethodPost, "/v1/cli/auth/revoke", struct{}{}, nil, false, accessToken)
}

func (c *Client) Inspect(ctx context.Context, request InspectRequest) (InspectResponse, error) {
	var response InspectResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/skill-agent-publications/inspect", request, &response, true, "")
	return response, err
}

func (c *Client) CreatePublication(ctx context.Context, request CreatePublicationRequest) (Publication, error) {
	var response Publication
	err := c.doJSON(ctx, http.MethodPost, "/v1/skill-agent-publications", request, &response, true, "")
	return response, err
}

func (c *Client) GetPublication(ctx context.Context, id string) (Publication, error) {
	var response Publication
	err := c.doJSON(ctx, http.MethodGet, "/v1/skill-agent-publications/"+url.PathEscape(id), nil, &response, true, "")
	return response, err
}

func (c *Client) ResolveAction(ctx context.Context, publicationID, actionID string, request ResolveActionRequest) (Publication, error) {
	var response Publication
	endpoint := "/v1/skill-agent-publications/" + url.PathEscape(publicationID) + "/actions/" + url.PathEscape(actionID) + "/resolve"
	err := c.doJSON(ctx, http.MethodPost, endpoint, request, &response, true, "")
	return response, err
}

func (c *Client) CancelPublication(ctx context.Context, id string) (Publication, error) {
	var response Publication
	err := c.doJSON(ctx, http.MethodPost, "/v1/skill-agent-publications/"+url.PathEscape(id)+"/cancel", struct{}{}, &response, true, "")
	return response, err
}

func (c *Client) ListTargets(ctx context.Context) (TargetList, error) {
	var response TargetList
	err := c.doJSON(ctx, http.MethodGet, "/v1/skill-agent-publish-targets", nil, &response, true, "")
	return response, err
}

func (c *Client) GetTarget(ctx context.Context, identifier string) (Target, error) {
	var response Target
	endpoint := "/v1/skill-agent-publish-targets/" + url.PathEscape(identifier)
	if code, ok := shareCode(identifier); ok {
		endpoint = "/v1/skill-agent-publish-targets/by-share-code/" + url.PathEscape(code)
	}
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, true, "")
	return response, err
}

func (c *Client) PrepareUpload(ctx context.Context, request UploadPrepareRequest) (UploadPrepareResponse, error) {
	var response UploadPrepareResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/skill-agent-publication-uploads", request, &response, true, "")
	return response, err
}

func (c *Client) CompleteUpload(ctx context.Context, id string, request UploadCompleteRequest) (UploadCompleteResponse, error) {
	var response UploadCompleteResponse
	endpoint := "/v1/skill-agent-publication-uploads/" + url.PathEscape(id) + "/complete"
	err := c.doJSON(ctx, http.MethodPost, endpoint, request, &response, true, "")
	return response, err
}

func (c *Client) PutUpload(ctx context.Context, prepared UploadPrepareResponse, body io.Reader, size int64) error {
	method := prepared.Method
	if method == "" {
		method = http.MethodPut
	}
	request, err := http.NewRequestWithContext(ctx, method, prepared.UploadURL, body)
	if err != nil {
		return output.Internal("upload_request", "failed to create upload request", err)
	}
	request.ContentLength = size
	for key, value := range prepared.Headers {
		request.Header.Set(key, value)
	}
	response, err := c.HTTPClient.Do(request)
	if err != nil {
		return output.Network("upload_transport", "failed to upload the Skill bundle", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return output.Network("upload_rejected", fmt.Sprintf("upload endpoint returned HTTP %d", response.StatusCode), nil)
	}
	return nil
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, requestBody, responseBody any, authenticated bool, explicitToken string) error {
	base, err := validateAPIBaseURL(c.BaseURL)
	if err != nil {
		return output.Validation("api_base_url", "Viceme API base URL must use HTTPS; HTTP is allowed only for localhost or loopback development")
	}
	base.Path = path.Join(base.Path, endpoint)
	var body io.Reader
	if requestBody != nil {
		data, err := json.Marshal(requestBody)
		if err != nil {
			return output.Internal("request_encode", "failed to encode the API request", err)
		}
		body = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, base.String(), body)
	if err != nil {
		return output.Internal("request_create", "failed to create the API request", err)
	}
	request.Header.Set("Accept", "application/json")
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.UserAgent != "" {
		request.Header.Set("User-Agent", c.UserAgent)
	}
	token := explicitToken
	if authenticated && token == "" {
		if c.Tokens == nil {
			return output.Authentication("not_logged_in", "not logged in to Viceme")
		}
		token, err = c.Tokens.Token(ctx)
		if err != nil {
			return err
		}
	}
	if token != "" {
		applyCredential := c.CredentialHeader
		if applyCredential == nil {
			applyCredential = ApplyAPIKeyCredential
		}
		applyCredential(request, token)
	}
	response, err := c.HTTPClient.Do(request)
	if err != nil {
		return output.Network("transport", "failed to reach the Viceme API", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return output.Network("response_read", "failed to read the Viceme API response", err)
	}
	if len(data) > maxResponseBytes {
		return output.Internal("response_too_large", "Viceme API response exceeded the client limit", nil)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return decodeServerError(response.StatusCode, data)
	}
	if responseBody == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	return decodeSuccess(data, responseBody)
}

func validateAPIBaseURL(raw string) (*url.URL, error) {
	base, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || base.Hostname() == "" || base.User != nil {
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

func decodeSuccess(data []byte, out any) error {
	var possibleEnvelope struct {
		OK   *bool           `json:"ok"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &possibleEnvelope); err == nil && possibleEnvelope.OK != nil && len(possibleEnvelope.Data) > 0 {
		data = possibleEnvelope.Data
	}
	if err := json.Unmarshal(data, out); err != nil {
		return output.Internal("response_decode", "Viceme API returned an invalid JSON response", err)
	}
	return nil
}

func decodeServerError(status int, data []byte) error {
	var envelope struct {
		Error ServerError `json:"error"`
	}
	serverError := ServerError{}
	if err := json.Unmarshal(data, &envelope); err == nil && envelope.Error.Message != "" {
		serverError = envelope.Error
	} else {
		_ = json.Unmarshal(data, &serverError)
	}
	if serverError.Message == "" {
		serverError.Message = fmt.Sprintf("Viceme API returned HTTP %d", status)
	}
	if serverError.Subtype == "" {
		serverError.Subtype = http.StatusText(status)
	}
	code, typ := exitForStatus(status, serverError.Type)
	cliError := output.NewError(code, typ, serverError.Subtype, serverError.Message)
	cliError.Retryable = serverError.Retryable
	cliError.Hint = serverError.Hint
	cliError.PublicationID = serverError.PublicationID
	cliError.ConsoleURL = serverError.ConsoleURL
	cliError.Details = serverError.Details
	return cliError
}

func exitForStatus(status int, serverType string) (int, string) {
	if serverType != "" {
		switch serverType {
		case "authentication", "authorization":
			return output.ExitAuthentication, serverType
		case "validation":
			return output.ExitValidation, serverType
		case "policy":
			return output.ExitPolicy, serverType
		}
	}
	switch status {
	case http.StatusBadRequest,
		http.StatusNotFound,
		http.StatusGone,
		http.StatusConflict,
		http.StatusUnsupportedMediaType,
		http.StatusUnprocessableEntity:
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

func shareCode(identifier string) (string, bool) {
	parsed, err := url.Parse(identifier)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) >= 2 && (parts[len(parts)-2] == "v" || parts[len(parts)-2] == "share") && parts[len(parts)-1] != "" {
		return parts[len(parts)-1], true
	}
	return "", false
}

func IsSubtype(err error, subtype string) bool {
	var cliErr *output.Error
	return errors.As(err, &cliErr) && cliErr.Subtype == subtype
}
