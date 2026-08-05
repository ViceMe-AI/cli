package api

import "time"

type DeviceAuthorization struct {
	VerificationURL         string    `json:"verification_url"`
	VerificationURLComplete string    `json:"verification_url_complete,omitempty"`
	DeviceCode              string    `json:"device_code"`
	UserCode                string    `json:"user_code,omitempty"`
	ExpiresAt               time.Time `json:"expires_at"`
	IntervalSeconds         int       `json:"interval_seconds"`
}

type DeviceTokenRequest struct {
	DeviceCode string `json:"device_code"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type DeviceToken struct {
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	TokenType        string    `json:"token_type"`
	ExpiresAt        time.Time `json:"expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
	UserID           string    `json:"user_id"`
	Scope            []string  `json:"scope"`
}

type RevokeResponse struct {
	Revoked bool `json:"revoked"`
}

type CreatorAppEnvironment struct {
	ID             string                 `json:"id"`
	Type           string                 `json:"type"`
	Status         string                 `json:"status"`
	PublishableKey string                 `json:"publishableKey"`
	AllowedOrigins []string               `json:"allowedOrigins"`
	Capabilities   []CreatorAppCapability `json:"capabilities"`
}

type CreatorAppCapability struct {
	Type            string         `json:"type"`
	Status          string         `json:"status"`
	ConfigVersion   int            `json:"configVersion"`
	ContractVersion string         `json:"contractVersion"`
	SDKPackage      string         `json:"sdkPackage"`
	SDKVersion      string         `json:"sdkVersion"`
	Config          map[string]any `json:"config,omitempty"`
}

type CreatorApp struct {
	ID                      string                  `json:"id"`
	Name                    string                  `json:"name"`
	HostingMode             string                  `json:"hostingMode"`
	Status                  string                  `json:"status"`
	CreatorChannelAccountID *string                 `json:"creatorChannelAccountId"`
	SkillProductID          *string                 `json:"skillProductId"`
	CreatedAt               time.Time               `json:"createdAt"`
	UpdatedAt               time.Time               `json:"updatedAt"`
	Environments            []CreatorAppEnvironment `json:"environments"`
}

type CreateCreatorAppRequest struct {
	ClientRequestID string `json:"clientRequestId"`
	Name            string `json:"name"`
	HostingMode     string `json:"hostingMode"`
}

type CreatorAppsResponse struct {
	Items []CreatorApp `json:"items"`
}

type AddOriginRequest struct {
	Origin string `json:"origin"`
}

type OriginResponse struct {
	ID     string `json:"id"`
	Origin string `json:"origin"`
}

type CapabilityCatalogItem struct {
	Type            string `json:"type"`
	Availability    string `json:"availability"`
	Description     string `json:"description"`
	ContractVersion string `json:"contractVersion"`
	SDKPackage      string `json:"sdkPackage"`
	SDKVersion      string `json:"sdkVersion"`
}

type CapabilityCatalog struct {
	Items []CapabilityCatalogItem `json:"items"`
}

type AddCapabilityRequest struct {
	Type   string         `json:"type"`
	Config map[string]any `json:"config"`
}

type PublicAppContext struct {
	App struct {
		Name string `json:"name"`
	} `json:"app"`
	Environment  string                 `json:"environment"`
	Capabilities []CreatorAppCapability `json:"capabilities"`
}
