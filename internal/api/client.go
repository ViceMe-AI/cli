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

func (c *Client) RefreshDeviceToken(ctx context.Context, refreshToken string) (DeviceToken, error) {
	var response DeviceToken
	err := c.doJSON(ctx, http.MethodPost, "/v1/cli/auth/refresh", RefreshTokenRequest{RefreshToken: refreshToken}, &response, false, "", nil)
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

func (c *Client) GetPublicAppContext(ctx context.Context, publishableKey, origin string) (PublicAppContext, error) {
	var response PublicAppContext
	headers := http.Header{}
	if origin != "" {
		headers.Set("Origin", origin)
	}
	err := c.doJSON(ctx, http.MethodGet, "/v1/creator-app-context/"+url.PathEscape(publishableKey), nil, &response, false, "", headers)
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

	response, err := withoutRedirects(c.HTTPClient).Do(request)
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

func withoutRedirects(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	copy := *client
	copy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &copy
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
