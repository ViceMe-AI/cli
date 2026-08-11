package api

type DeviceAuthorizationRequest struct {
	ClientName string   `json:"clientName"`
	CLIVersion string   `json:"cliVersion"`
	Scopes     []string `json:"scopes"`
}

type DeviceAuthorization struct {
	DeviceCode              string `json:"deviceCode"`
	UserCode                string `json:"userCode"`
	VerificationURI         string `json:"verificationUri"`
	VerificationURIComplete string `json:"verificationUriComplete"`
	ExpiresIn               int    `json:"expiresIn"`
	Interval                int    `json:"interval"`
}

type DeviceTokenRequest struct {
	DeviceCode string `json:"deviceCode"`
}

type DeviceToken struct {
	Status      string   `json:"status"`
	Interval    int      `json:"interval,omitempty"`
	AccessToken string   `json:"accessToken,omitempty"`
	TokenType   string   `json:"tokenType,omitempty"`
	ExpiresAt   string   `json:"expiresAt,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
}

type AuthStatus struct {
	Authenticated bool     `json:"authenticated"`
	User          AuthUser `json:"user"`
	Scopes        []string `json:"scopes"`
	ExpiresAt     string   `json:"expiresAt"`
}

type AuthUser struct {
	ID          string  `json:"id"`
	DisplayName *string `json:"displayName"`
	AvatarURL   *string `json:"avatarUrl"`
}

type SkillPublicationManifest struct {
	APIVersion string                   `json:"apiVersion" yaml:"apiVersion"`
	Kind       string                   `json:"kind" yaml:"kind"`
	Metadata   SkillPublicationMetadata `json:"metadata" yaml:"metadata"`
	Spec       SkillPublicationSpec     `json:"spec" yaml:"spec"`
}

type SkillPublicationMetadata struct {
	Title   string `json:"title" yaml:"title"`
	Summary string `json:"summary" yaml:"summary"`
}

type SkillPublicationSpec struct {
	Source SkillPublicationSource `json:"source" yaml:"source"`
	Sale   SkillPublicationSale   `json:"sale" yaml:"sale"`
}

type SkillPublicationSource struct {
	Entry string `json:"entry" yaml:"entry"`
}

type SkillPublicationSale struct {
	Currency    string `json:"currency" yaml:"currency"`
	PriceMinor  int    `json:"priceMinor" yaml:"priceMinor"`
	Entitlement string `json:"entitlement" yaml:"entitlement"`
}

type SkillPublicationFile struct {
	Digest      string `json:"digest"`
	SizeBytes   int64  `json:"sizeBytes"`
	FileName    string `json:"fileName"`
	ContentType string `json:"contentType"`
}

type CreateSkillPublicationRequest struct {
	ClientRequestID    string                   `json:"clientRequestId"`
	ContractVersion    string                   `json:"contractVersion"`
	CLIVersion         string                   `json:"cliVersion"`
	Manifest           SkillPublicationManifest `json:"manifest"`
	ManifestDigest     string                   `json:"manifestDigest"`
	Artifact           SkillPublicationFile     `json:"artifact"`
	ProductID          string                   `json:"productId,omitempty"`
	CreatorDisplayName string                   `json:"creatorDisplayName,omitempty"`
}

type CreateSkillPublicationResponse struct {
	PublicationID string               `json:"publicationId"`
	Status        string               `json:"status"`
	PackageUpload *UploadAuthorization `json:"packageUpload"`
}

type UploadAuthorizationRequest struct {
	Kind         string `json:"kind"`
	Digest       string `json:"digest"`
	SizeBytes    int64  `json:"sizeBytes"`
	FileName     string `json:"fileName"`
	ContentType  string `json:"contentType"`
	RelativePath string `json:"relativePath,omitempty"`
	SortOrder    int    `json:"sortOrder"`
}

type UploadAuthorization struct {
	UploadID  string            `json:"uploadId"`
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	ExpiresAt string            `json:"expiresAt"`
	Headers   map[string]string `json:"headers"`
}

type CompleteUploadRequest struct {
	UploadID string `json:"uploadId"`
}

type SkillPublicationDraft struct {
	Title            string   `json:"title"`
	Summary          string   `json:"summary"`
	Currency         string   `json:"currency"`
	PriceMinor       int      `json:"priceMinor"`
	CoverUploadID    *string  `json:"coverUploadId"`
	GalleryUploadIDs []string `json:"galleryUploadIds"`
}

type SkillPublicationUpload struct {
	ID           string  `json:"id"`
	Kind         string  `json:"kind"`
	Status       string  `json:"status"`
	RelativePath *string `json:"relativePath"`
	FileName     string  `json:"fileName"`
	ContentType  string  `json:"contentType"`
	SizeBytes    int64   `json:"sizeBytes"`
	Digest       string  `json:"digest"`
	SortOrder    int     `json:"sortOrder"`
	ViewURL      *string `json:"viewUrl"`
}

type ListingMediaSuggestion struct {
	CoverCandidateID    *string  `json:"coverCandidateId"`
	GalleryCandidateIDs []string `json:"galleryCandidateIds"`
	Reasons             []string `json:"reasons"`
	Warnings            []string `json:"warnings"`
}

type PublicationAnalysis struct {
	Status      string                  `json:"status"`
	Suggestions *ListingMediaSuggestion `json:"suggestions"`
	ErrorCode   *string                 `json:"errorCode"`
}

type PublishedProduct struct {
	ID        string `json:"id"`
	Slug      string `json:"slug"`
	DetailURL string `json:"detailUrl"`
	ReleaseID string `json:"releaseId"`
}

type SkillPublication struct {
	ID             string                   `json:"id"`
	Status         string                   `json:"status"`
	Manifest       SkillPublicationManifest `json:"manifest"`
	Draft          SkillPublicationDraft    `json:"draft"`
	ReviewRevision int                      `json:"reviewRevision"`
	ReviewDigest   *string                  `json:"reviewDigest"`
	Uploads        []SkillPublicationUpload `json:"uploads"`
	Analysis       *PublicationAnalysis     `json:"analysis"`
	Product        *PublishedProduct        `json:"product"`
	FailureCode    *string                  `json:"failureCode"`
	CreatedAt      string                   `json:"createdAt"`
	UpdatedAt      string                   `json:"updatedAt"`
}

type ReviewDigestRequest struct {
	ReviewDigest string `json:"reviewDigest"`
}

type CancelPublicationResponse struct {
	Cancelled bool `json:"cancelled"`
}

type APIError struct {
	StatusCode int    `json:"statusCode"`
	Code       string `json:"code"`
	Message    any    `json:"message"`
	RequestID  string `json:"requestId"`
}
