package api

import (
	"encoding/json"
	"fmt"
	"time"
)

type Document map[string]any

func (d Document) StringValue(key string) string {
	value, _ := d[key].(string)
	return value
}

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

type DeviceToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	UserID       string    `json:"user_id,omitempty"`
}

type Source struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type InspectRequest struct {
	Source    Source `json:"source"`
	SkillRoot string `json:"skill_root,omitempty"`
}

type InspectResponse Document

type Destination struct {
	Mode                  string `json:"mode"`
	Alias                 string `json:"alias,omitempty"`
	TargetID              string `json:"target_id,omitempty"`
	ExpectedTargetVersion int64  `json:"expected_target_version,omitempty"`
}

type PublicationOptions struct {
	TargetLocale          string `json:"target_locale,omitempty"`
	PublishMode           string `json:"publish_mode"`
	AdmissionConfirmation bool   `json:"admission_confirmation"`
}

type CreatePublicationRequest struct {
	ClientRequestID string             `json:"client_request_id"`
	Source          *Source            `json:"source,omitempty"`
	ResolutionID    string             `json:"resolution_id,omitempty"`
	Selector        string             `json:"selector,omitempty"`
	Destination     Destination        `json:"destination"`
	Options         PublicationOptions `json:"options"`
}

type Publication Document

func (p Publication) Status() string {
	return Document(p).StringValue("status")
}

func (p Publication) ID() string {
	return Document(p).StringValue("publication_id")
}

func (p Publication) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any(p))
}

type ResolveActionRequest struct {
	ExpectedPayloadDigest string          `json:"expected_payload_digest"`
	Payload               json.RawMessage `json:"payload"`
}

type UploadPrepareRequest struct {
	Filename     string `json:"filename"`
	ContentType  string `json:"content_type"`
	Size         int64  `json:"size"`
	SHA256Digest string `json:"sha256_digest"`
}

type UploadPrepareResponse struct {
	UploadID  string            `json:"upload_id"`
	UploadURL string            `json:"upload_url"`
	Method    string            `json:"method,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	ExpiresAt time.Time         `json:"expires_at,omitempty"`
}

type UploadCompleteRequest struct {
	Size         int64  `json:"size"`
	SHA256Digest string `json:"sha256_digest"`
}

type UploadCompleteResponse struct {
	UploadID string `json:"upload_id"`
	Status   string `json:"status"`
}

type Target Document

type TargetList Document

type ServerError struct {
	Type          string `json:"type"`
	Subtype       string `json:"subtype"`
	Message       string `json:"message"`
	Retryable     bool   `json:"retryable"`
	Hint          string `json:"hint,omitempty"`
	PublicationID string `json:"publication_id,omitempty"`
	ConsoleURL    string `json:"console_url,omitempty"`
	Details       any    `json:"details,omitempty"`
}

func (e ServerError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("Viceme API error (%s)", e.Subtype)
}
