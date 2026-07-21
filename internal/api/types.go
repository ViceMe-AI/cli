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

type InspectCandidate struct {
	Selector    string   `json:"selector"`
	Title       string   `json:"title,omitempty"`
	Source      Document `json:"source,omitempty"`
	Destination Document `json:"destination,omitempty"`
}

type InspectResponse struct {
	ResolutionID  string             `json:"resolution_id"`
	ExpiresAt     time.Time          `json:"expires_at,omitempty"`
	Source        Document           `json:"source,omitempty"`
	SourceVersion Document           `json:"source_version,omitempty"`
	Destination   Document           `json:"destination,omitempty"`
	Candidates    []InspectCandidate `json:"candidates"`
}

type Destination struct {
	Mode                  string `json:"mode"`
	Alias                 string `json:"alias,omitempty"`
	TargetID              string `json:"target_id,omitempty"`
	ExpectedTargetVersion *int64 `json:"expected_target_version,omitempty"`
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
	ExpectedPayloadDigest string `json:"expected_payload_digest"`
	// Payload answers typed payload actions (select_root). confirm_publish
	// decisions instead carry Decision plus the candidate/summary digests.
	Payload                        json.RawMessage `json:"payload,omitempty"`
	ExpectedReleaseCandidateDigest string          `json:"expected_release_candidate_digest,omitempty"`
	ExpectedPublicSummaryDigest    string          `json:"expected_public_summary_digest,omitempty"`
	Decision                       string          `json:"decision,omitempty"`
}

// ResolveMetadataRequest resolves the confirm_metadata checkpoint: confirm
// (optionally editing title/description/author within the visible-char
// limits) or cancel with zero assets.
type ResolveMetadataRequest struct {
	ActionID              string `json:"action_id"`
	ExpectedPayloadDigest string `json:"expected_payload_digest"`
	Decision              string `json:"decision"`
	Title                 string `json:"title,omitempty"`
	Description           string `json:"description,omitempty"`
	Author                string `json:"author,omitempty"`
}

// PublicationEditRequest submits a natural-language candidate edit (Host typed action).
type PublicationEditRequest struct {
	EditRequest            string `json:"edit_request"`
	CurrentCandidateDigest string `json:"current_candidate_digest"`
}

// PublicationEditReceipt is the durable edit receipt (pending/applied/failed).
type PublicationEditReceipt struct {
	EditID                string  `json:"edit_id"`
	Status                string  `json:"status"`
	Class                 *string `json:"class"`
	BaseCandidateDigest   string  `json:"base_candidate_digest"`
	ResultCandidateDigest *string `json:"result_candidate_digest"`
	Error                 any     `json:"error"`
	CreatedAt             string  `json:"created_at"`
	CompletedAt           *string `json:"completed_at"`
}

// PreviewRunStartRequest starts one real test run of the exact candidate.
type PreviewRunStartRequest struct {
	Inputs                   map[string]string `json:"inputs"`
	ExpectedCandidateDigest  string            `json:"expected_candidate_digest"`
}

// PreviewRunStartResponse is the accepted start receipt.
type PreviewRunStartResponse struct {
	PreviewRunID string `json:"preview_run_id"`
	Status       string `json:"status"`
}

// SkillPreviewRun is the durable preview-run receipt with the bounded result.
type SkillPreviewRun struct {
	PublicationID   string         `json:"publication_id"`
	PreviewRunID    string         `json:"preview_run_id"`
	RunnerRunID     string         `json:"runner_run_id"`
	CandidateDigest string         `json:"candidate_digest"`
	InputsDigest    *string        `json:"inputs_digest"`
	Status          string         `json:"status"`
	ResultDigest    *string        `json:"result_digest"`
	Result          map[string]any `json:"result"`
	Accepted        bool           `json:"accepted"`
	AcceptedAt      *string        `json:"accepted_at"`
}

// PreviewRunAcceptRequest accepts the actual result of a preview run.
// PRE-04: the acceptance must bind the exact input set that produced the
// result, so ExpectedInputsDigest is required.
type PreviewRunAcceptRequest struct {
	ExpectedCandidateDigest string `json:"expected_candidate_digest"`
	ExpectedInputsDigest    string `json:"expected_inputs_digest"`
}

// PreviewRunAcceptResponse is the acceptance receipt.
type PreviewRunAcceptResponse struct {
	PublicationID string `json:"publication_id"`
	PreviewRunID  string `json:"preview_run_id"`
	Status        string `json:"status"`
	AcceptedAt    string `json:"accepted_at"`
}

// PublicationMetadata is the metadata checkpoint read model.
type PublicationMetadata struct {
	PublicationID string   `json:"publication_id"`
	Status        string   `json:"status"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Author        string   `json:"author"`
	Missing       []string `json:"missing"`
	ActionID      string   `json:"action_id"`
	ExpiresAt     string   `json:"expires_at"`
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
