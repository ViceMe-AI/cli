package api

const SkillPublicationContractVersion = "2026-08-17"

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

type SdkWorkFeatureConfig struct {
	FeatureKey string               `json:"featureKey"`
	Title      string               `json:"title"`
	Policy     SdkWorkFeaturePolicy `json:"policy"`
	Status     string               `json:"status"`
}

type SdkWorkFeaturePolicy struct {
	Type string `json:"type"`
}

type CreateSdkWorkRequest struct {
	DisplayName string `json:"displayName"`
}

type ApplySdkWorkRequest struct {
	ExpectedConfigVersion int                    `json:"expectedConfigVersion"`
	DisplayName           string                 `json:"displayName"`
	Features              []SdkWorkFeatureConfig `json:"features"`
	Status                string                 `json:"status"`
}

type SdkWork struct {
	WorkKey       string                 `json:"workKey"`
	DisplayName   string                 `json:"displayName"`
	Status        string                 `json:"status"`
	ConfigVersion int                    `json:"configVersion"`
	Features      []SdkWorkFeatureConfig `json:"features"`
	Capabilities  []string               `json:"capabilities"`
	CreatedAt     string                 `json:"createdAt"`
	UpdatedAt     string                 `json:"updatedAt"`
}

type SdkWorks struct {
	Works []SdkWork `json:"works"`
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
	PriceMinor  *int   `json:"priceMinor" yaml:"priceMinor"`
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
	ListingID          string                   `json:"listingId"`
	CreatorDisplayName string                   `json:"creatorDisplayName,omitempty"`
}

type CreateSkillPublicationResponse struct {
	PublicationID string               `json:"publicationId"`
	ListingID     string               `json:"listingId"`
	DraftRevision int                  `json:"draftRevision"`
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
	Title                 string   `json:"title"`
	SummaryZhCN           *string  `json:"summaryZhCn"`
	SummaryEnUS           *string  `json:"summaryEnUs"`
	UsageInstructionsZhCN *string  `json:"usageInstructionsZhCn"`
	UsageInstructionsEnUS *string  `json:"usageInstructionsEnUs"`
	Currency              string   `json:"currency"`
	PriceMinor            *int     `json:"priceMinor"`
	CoverUploadID         *string  `json:"coverUploadId"`
	GalleryUploadIDs      []string `json:"galleryUploadIds"`
}

type UpdateSkillPublicationDraftRequest struct {
	PriceMinor       *int     `json:"priceMinor,omitempty"`
	CoverUploadID    *string  `json:"coverUploadId,omitempty"`
	GalleryUploadIDs []string `json:"galleryUploadIds,omitempty"`
}

type SkillPublicationAgentSuggestionPatch struct {
	SummaryZhCN           string   `json:"summaryZhCn"`
	SummaryEnUS           string   `json:"summaryEnUs"`
	UsageInstructionsZhCN string   `json:"usageInstructionsZhCn"`
	UsageInstructionsEnUS string   `json:"usageInstructionsEnUs"`
	CoverUploadID         *string  `json:"coverUploadId"`
	GalleryUploadIDs      []string `json:"galleryUploadIds"`
}

type SuggestSkillPublicationDraftRequest struct {
	BaseDraftRevision int                                  `json:"baseDraftRevision"`
	Patch             SkillPublicationAgentSuggestionPatch `json:"patch"`
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

type ListingSuggestion struct {
	SummaryZhCN           string   `json:"summaryZhCn"`
	SummaryEnUS           string   `json:"summaryEnUs"`
	UsageInstructionsZhCN string   `json:"usageInstructionsZhCn"`
	UsageInstructionsEnUS string   `json:"usageInstructionsEnUs"`
	CoverCandidateID      *string  `json:"coverCandidateId"`
	GalleryCandidateIDs   []string `json:"galleryCandidateIds"`
	Reasons               []string `json:"reasons"`
	Warnings              []string `json:"warnings"`
}

type PublicationAnalysis struct {
	Status      string             `json:"status"`
	Suggestions *ListingSuggestion `json:"suggestions"`
	ErrorCode   *string            `json:"errorCode"`
}

type PublishedProduct struct {
	ID        string `json:"id"`
	Slug      string `json:"slug"`
	DetailURL string `json:"detailUrl"`
	ReleaseID string `json:"releaseId"`
}

type SkillPublication struct {
	ID             string                   `json:"id"`
	ListingID      string                   `json:"listingId"`
	DraftRevision  int                      `json:"draftRevision"`
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

type PrepareSkillListingRequest struct {
	ClientRequestID string                    `json:"clientRequestId"`
	Source          PrepareSkillListingSource `json:"source"`
	Resolution      *SkillListingResolution   `json:"resolution,omitempty"`
}

type PrepareSkillListingSource struct {
	Type           string  `json:"type"`
	ClientWorkID   string  `json:"clientWorkId"`
	BindingReceipt *string `json:"bindingReceipt"`
	PackageDigest  string  `json:"packageDigest"`
	DisplayName    string  `json:"displayName"`
}

type SkillListingResolution struct {
	Mode      string `json:"mode"`
	ListingID string `json:"listingId,omitempty"`
}

type SkillListingPreviewViewModel struct {
	SchemaVersion string  `json:"schemaVersion"`
	ListingID     string  `json:"listingId"`
	DraftRevision int     `json:"draftRevision"`
	State         string  `json:"state"`
	Title         *string `json:"title"`
	ThumbnailURL  *string `json:"thumbnailUrl"`
	FallbackURL   string  `json:"fallbackUrl"`
}

type PrepareSkillListingResponse struct {
	ListingID       string                       `json:"listingId"`
	Market          string                       `json:"market"`
	Status          string                       `json:"status"`
	DraftRevision   int                          `json:"draftRevision"`
	OwnerPreviewURL string                       `json:"ownerPreviewUrl"`
	BindingReceipt  string                       `json:"bindingReceipt"`
	Resolution      string                       `json:"resolution"`
	Preview         SkillListingPreviewViewModel `json:"preview"`
	NextActions     []string                     `json:"nextActions"`
}

type CreateSkillPreviewLaunchResponse struct {
	LaunchURL string `json:"launchUrl"`
	ExpiresAt string `json:"expiresAt"`
}

type SkillListingCandidatesRequest struct {
	PackageDigest string `json:"packageDigest"`
}

type SkillListingCandidate struct {
	ListingID       string  `json:"listingId"`
	Title           *string `json:"title"`
	UpdatedAt       string  `json:"updatedAt"`
	OwnerPreviewURL string  `json:"ownerPreviewUrl"`
}

type SkillListingCandidatesResponse struct {
	Candidates []SkillListingCandidate `json:"candidates"`
}

type SkillListingPublicationPreview struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type SkillListingPreview struct {
	ListingID     string                          `json:"listingId"`
	Status        string                          `json:"status"`
	DraftRevision int                             `json:"draftRevision"`
	Publication   *SkillListingPublicationPreview `json:"publication"`
	PublicURL     *string                         `json:"publicUrl"`
	Preview       SkillListingPreviewViewModel    `json:"preview"`
}

type ReviewDigestRequest struct {
	ReviewDigest string `json:"reviewDigest"`
}

type CancelPublicationResponse struct {
	Cancelled bool `json:"cancelled"`
}

type CreateCreatorAppRequest struct {
	Name string `json:"name"`
}

type CreatorAppDomainInfo struct {
	Domain            string  `json:"domain"`
	Verified          bool    `json:"verified"`
	VerificationToken *string `json:"verificationToken"`
}

type CreatorApp struct {
	ID        string                 `json:"id"`
	Kind      string                 `json:"kind"`
	Name      string                 `json:"name"`
	Domains   []CreatorAppDomainInfo `json:"domains"`
	CreatedAt string                 `json:"createdAt"`
}

type CreatorAppsResponse struct {
	Items []CreatorApp `json:"items"`
}

type AddCreatorAppDomainRequest struct {
	Domain string `json:"domain"`
}

type APIError struct {
	StatusCode int    `json:"statusCode"`
	Code       string `json:"code"`
	Message    any    `json:"message"`
	RequestID  string `json:"requestId"`
}
