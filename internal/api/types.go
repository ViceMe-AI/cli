package api

import "encoding/json"

const SkillPublicationContractVersion = "2026-08-24"

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

type MerchantAccount struct {
	ID               string  `json:"id"`
	CreatorAccountID *string `json:"creatorAccountId"`
	DisplayName      string  `json:"displayName"`
	Status           string  `json:"status"`
	StatusVersion    int     `json:"statusVersion"`
}

type MerchantAccountsResponse struct {
	Items []MerchantAccount `json:"items"`
}

type WorkOwner struct {
	Kind              string  `json:"kind"`
	UserID            *string `json:"userId,omitempty"`
	CreatorAccountID  *string `json:"creatorAccountId,omitempty"`
	MerchantAccountID *string `json:"merchantAccountId,omitempty"`
}

type WebsiteWork struct {
	CanonicalOrigin     string  `json:"canonicalOrigin"`
	DomainASCII         string  `json:"domainAscii"`
	OwnershipStatus     string  `json:"ownershipStatus"`
	VerificationVersion int     `json:"verificationVersion"`
	VerifiedAt          *string `json:"verifiedAt"`
}

type MerchantWork struct {
	ID             string          `json:"id"`
	Kind           string          `json:"kind"`
	Origin         string          `json:"origin"`
	Slug           string          `json:"slug"`
	Title          string          `json:"title"`
	Status         string          `json:"status"`
	Revision       int             `json:"revision"`
	Owner          WorkOwner       `json:"owner"`
	Skill          json.RawMessage `json:"skill"`
	Service        json.RawMessage `json:"service"`
	Website        *WebsiteWork    `json:"website"`
	ActiveRevision json.RawMessage `json:"activeRevision"`
	DraftRevision  json.RawMessage `json:"draftRevision"`
	CreatedAt      string          `json:"createdAt"`
	UpdatedAt      string          `json:"updatedAt"`
}

type WorkPreviewGrant struct {
	ID                     string   `json:"id"`
	WorkID                 string   `json:"workId"`
	WorkRevisionID         string   `json:"workRevisionId"`
	AllowedRepresentations []string `json:"allowedRepresentations"`
	HTMLURL                *string  `json:"htmlUrl"`
	MarkdownURL            *string  `json:"markdownUrl"`
	ExpiresAt              string   `json:"expiresAt"`
	RevokedAt              *string  `json:"revokedAt"`
}

type MerchantWorksResponse struct {
	Items []MerchantWork `json:"items"`
}

type CreateWebsiteVerificationRequest struct {
	MerchantAccountID string `json:"merchantAccountId"`
	ExpectedRevision  int    `json:"expectedRevision"`
}

type VerifyWebsiteRequest struct {
	MerchantAccountID           string `json:"merchantAccountId"`
	ExpectedVerificationVersion int    `json:"expectedVerificationVersion"`
}

type RevokeWebsiteOwnershipRequest struct {
	MerchantAccountID string `json:"merchantAccountId"`
	ExpectedRevision  int    `json:"expectedRevision"`
}

type WebsiteVerification struct {
	ID            string  `json:"id"`
	WebsiteWorkID string  `json:"websiteWorkId"`
	Version       int     `json:"version"`
	DNSRecordName string  `json:"dnsRecordName"`
	Challenge     *string `json:"challenge,omitempty"`
	Status        string  `json:"status"`
	ExpiresAt     string  `json:"expiresAt"`
	VerifiedAt    *string `json:"verifiedAt"`
	FailureCode   *string `json:"failureCode"`
}

type CreateWorkSdkAccessRequest struct {
	MerchantAccountID string              `json:"merchantAccountId"`
	Features          []string            `json:"features"`
	AccessFeatures    []WorkAccessFeature `json:"accessFeatures,omitempty"`
}

type UpdateWorkSdkAccessRequest struct {
	MerchantAccountID     string              `json:"merchantAccountId"`
	ExpectedConfigVersion int                 `json:"expectedConfigVersion"`
	Features              []string            `json:"features"`
	AccessFeatures        []WorkAccessFeature `json:"accessFeatures,omitempty"`
}

type WorkAccessPrice struct {
	Currency    string `json:"currency"`
	AmountCents int    `json:"amountCents"`
}

type WorkAccessFeature struct {
	FeatureKey string           `json:"featureKey"`
	Title      string           `json:"title"`
	PolicyType string           `json:"policyType"`
	ProductID  *string          `json:"productId,omitempty"`
	Price      *WorkAccessPrice `json:"price"`
	Status     string           `json:"status"`
}

type WorkSdkAccess struct {
	WorkID         string              `json:"workId"`
	WorkKey        string              `json:"workKey"`
	Status         string              `json:"status"`
	ConfigVersion  int                 `json:"configVersion"`
	Features       []string            `json:"features"`
	AccessFeatures []WorkAccessFeature `json:"accessFeatures"`
	CreatedAt      string              `json:"createdAt"`
	UpdatedAt      string              `json:"updatedAt"`
}

type WorkSdkAccessesResponse struct {
	Items []WorkSdkAccess `json:"items"`
}

type CreateCommerceApplicationRequest struct {
	MerchantAccountID string   `json:"merchantAccountId"`
	WorkID            string   `json:"workId"`
	Kind              string   `json:"kind"`
	Environment       string   `json:"environment"`
	DisplayName       string   `json:"displayName"`
	Origins           []string `json:"origins,omitempty"`
	ReturnURLs        []string `json:"returnUrls,omitempty"`
}

type UpdateCommerceApplicationRequest struct {
	MerchantAccountID string    `json:"merchantAccountId"`
	ExpectedRevision  int       `json:"expectedRevision"`
	DisplayName       *string   `json:"displayName,omitempty"`
	Origins           *[]string `json:"origins,omitempty"`
	ReturnURLs        *[]string `json:"returnUrls,omitempty"`
}

type CommerceApplicationCommand struct {
	MerchantAccountID string `json:"merchantAccountId"`
	ExpectedRevision  int    `json:"expectedRevision"`
}

type CommerceApplicationOrigin struct {
	ID     string `json:"id"`
	Origin string `json:"origin"`
}

type CommerceApplicationReturnURL struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

type CommerceApplicationProduct struct {
	ProductID string  `json:"productId"`
	Alias     *string `json:"alias"`
	Title     string  `json:"title"`
	Status    string  `json:"status"`
}

type CommerceApplication struct {
	ID                string                         `json:"id"`
	WorkID            string                         `json:"workId"`
	MerchantAccountID string                         `json:"merchantAccountId"`
	PublicClientID    string                         `json:"publicClientId"`
	Kind              string                         `json:"kind"`
	Environment       string                         `json:"environment"`
	Status            string                         `json:"status"`
	DisplayName       string                         `json:"displayName"`
	Revision          int                            `json:"revision"`
	Origins           []CommerceApplicationOrigin    `json:"origins"`
	ReturnURLs        []CommerceApplicationReturnURL `json:"returnUrls"`
	Products          []CommerceApplicationProduct   `json:"products"`
	ActivatedAt       *string                        `json:"activatedAt"`
	SuspendedAt       *string                        `json:"suspendedAt"`
	CreatedAt         string                         `json:"createdAt"`
	UpdatedAt         string                         `json:"updatedAt"`
}

type CommerceApplicationsResponse struct {
	Items []CommerceApplication `json:"items"`
}

type CandidatePurchaseSkill struct {
	WorkID         string `json:"workId"`
	StableName     string `json:"stableName"`
	SkillReleaseID string `json:"skillReleaseId"`
	Version        int    `json:"version"`
	ManifestDigest string `json:"manifestDigest"`
	ArtifactDigest string `json:"artifactDigest"`
}

type ProductValidationIssue struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

type ProductValidation struct {
	MissingFields []string                 `json:"missingFields"`
	Errors        []ProductValidationIssue `json:"errors"`
	Warnings      []ProductValidationIssue `json:"warnings"`
}

type MerchantProductDraftResponse struct {
	ProductID                   string                  `json:"productId"`
	Revision                    int                     `json:"revision"`
	CandidateSalesSpecVersionID *string                 `json:"candidateSalesSpecVersionId"`
	CandidateDigest             *string                 `json:"candidateDigest"`
	CandidatePurchaseSkill      *CandidatePurchaseSkill `json:"candidatePurchaseSkill"`
	Validation                  ProductValidation       `json:"validation"`
}

type MerchantProductSummary struct {
	ProductID                   string             `json:"productId"`
	SubjectWorkID               *string            `json:"subjectWorkId"`
	Slug                        string             `json:"slug"`
	Title                       string             `json:"title"`
	Status                      string             `json:"status"`
	Visibility                  string             `json:"visibility"`
	Revision                    int                `json:"revision"`
	ActiveProduct               json.RawMessage    `json:"activeProduct"`
	CandidateSalesSpecVersionID *string            `json:"candidateSalesSpecVersionId"`
	CandidateDigest             *string            `json:"candidateDigest"`
	Validation                  *ProductValidation `json:"validation"`
	UpdatedAt                   string             `json:"updatedAt"`
}

type MerchantProductsResponse struct {
	Items      []MerchantProductSummary `json:"items"`
	NextCursor *string                  `json:"nextCursor"`
}

type MerchantProductLifecycleResponse struct {
	ProductID string `json:"productId"`
	Status    string `json:"status"`
	Revision  int    `json:"revision"`
}

type ProductSKU struct {
	ID              string            `json:"id"`
	Code            string            `json:"code"`
	Title           string            `json:"title"`
	Currency        string            `json:"currency"`
	PriceCents      int               `json:"priceCents"`
	Status          string            `json:"status"`
	InventoryPolicy string            `json:"inventoryPolicy"`
	Attributes      map[string]any    `json:"attributes"`
	SelectedOptions map[string]string `json:"selectedOptions"`
	AvailableAt     *string           `json:"availableAt"`
}

type CommerceProductSubjectWork struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Slug  string `json:"slug"`
	Title string `json:"title"`
}

type CommerceProduct struct {
	ID                string                      `json:"id"`
	Slug              string                      `json:"slug"`
	Title             string                      `json:"title"`
	Summary           string                      `json:"summary"`
	Description       string                      `json:"description"`
	UsageInstructions string                      `json:"usageInstructions"`
	Status            string                      `json:"status"`
	Visibility        string                      `json:"visibility"`
	Revision          int                         `json:"revision"`
	SubjectWork       *CommerceProductSubjectWork `json:"subjectWork"`
	Merchant          json.RawMessage             `json:"merchant"`
	SalesSpec         struct {
		ID                string          `json:"id"`
		Version           int             `json:"version"`
		Digest            string          `json:"digest"`
		PricingPolicyCode string          `json:"pricingPolicyCode"`
		PaymentPolicyCode string          `json:"paymentPolicyCode"`
		EarningPolicyCode string          `json:"earningPolicyCode"`
		Quantity          json.RawMessage `json:"quantity"`
		SKUs              []ProductSKU    `json:"skus"`
		Options           json.RawMessage `json:"options"`
		BuyerContract     json.RawMessage `json:"buyerContract"`
		FulfillmentSteps  json.RawMessage `json:"fulfillmentSteps"`
	} `json:"salesSpec"`
}

type MerchantProductActivationResponse struct {
	Product                 CommerceProduct `json:"product"`
	PurchaseSkillStableName string          `json:"purchaseSkillStableName"`
	ProductDetailURL        string          `json:"productDetailUrl"`
}

type PurchaseSkillRelease struct {
	SkillReleaseID        string          `json:"skillReleaseId"`
	Version               int             `json:"version"`
	Manifest              json.RawMessage `json:"manifest"`
	ManifestDigest        string          `json:"manifestDigest"`
	ArtifactDigest        string          `json:"artifactDigest"`
	SignedEnvelope        json.RawMessage `json:"signedEnvelope"`
	SignedEnvelopeDigest  string          `json:"signedEnvelopeDigest"`
	SigningKeyID          string          `json:"signingKeyId"`
	Signature             string          `json:"signature"`
	MinimumRuntimeVersion string          `json:"minimumRuntimeVersion"`
}

type PurchaseSkillBinding struct {
	BindingType string `json:"bindingType"`
	ProductID   string `json:"productId"`
}

type PurchaseSkillProduct struct {
	ID                  string `json:"id"`
	Slug                string `json:"slug"`
	Title               string `json:"title"`
	Summary             string `json:"summary"`
	Status              string `json:"status"`
	Visibility          string `json:"visibility"`
	MerchantDisplayName string `json:"merchantDisplayName"`
}

type ProductPurchaseSkillDescriptor struct {
	WorkID        string                 `json:"workId"`
	Binding       PurchaseSkillBinding   `json:"binding"`
	StableName    string                 `json:"stableName"`
	Status        string                 `json:"status"`
	Revision      int                    `json:"revision"`
	Products      []PurchaseSkillProduct `json:"products"`
	ActiveRelease PurchaseSkillRelease   `json:"activeRelease"`
	Distributions json.RawMessage        `json:"distributions"`
}

type ProductPurchaseSkillInstall struct {
	StableName     string                               `json:"stableName"`
	SkillReleaseID string                               `json:"skillReleaseId"`
	ArtifactDigest string                               `json:"artifactDigest"`
	DownloadURL    string                               `json:"downloadUrl"`
	ExpiresAt      string                               `json:"expiresAt"`
	Runtime        ProductPurchaseSkillRuntimeBootstrap `json:"runtime"`
}

type ProductPurchaseSkillRuntimeBootstrap struct {
	Kind                         string `json:"kind"`
	ProtocolVersion              int    `json:"protocolVersion"`
	MinimumRuntimeVersion        string `json:"minimumRuntimeVersion"`
	InstallerContractURL         string `json:"installerContractUrl"`
	CommerceInstallerContractURL string `json:"commerceInstallerContractUrl"`
	InstallCommand               string `json:"installCommand"`
}

type CommerceSkillTrustKey struct {
	KeyID     string `json:"keyId"`
	Algorithm string `json:"algorithm"`
	PublicKey string `json:"publicKey"`
}

type CommerceSession struct {
	SessionID     string  `json:"sessionId"`
	PrincipalID   string  `json:"principalId"`
	PrincipalKind string  `json:"principalKind"`
	Token         string  `json:"token"`
	ExpiresAt     string  `json:"expiresAt"`
	Recovered     bool    `json:"recovered"`
	ProductID     *string `json:"productId"`
}

type ContractAssetUpload struct {
	AssetID string `json:"assetId"`
	Status  string `json:"status"`
	Upload  struct {
		URL       string            `json:"url"`
		Headers   map[string]string `json:"headers"`
		ExpiresAt string            `json:"expiresAt"`
	} `json:"upload"`
	ReservationExpiresAt string `json:"reservationExpiresAt"`
}

type ContractAsset struct {
	AssetID     string `json:"assetId"`
	Status      string `json:"status"`
	FileName    string `json:"fileName"`
	ContentType string `json:"contentType"`
	SizeBytes   int64  `json:"sizeBytes"`
	Digest      string `json:"digest"`
	ExpiresAt   string `json:"expiresAt"`
}

type ProductQuote struct {
	ID                  string                  `json:"id"`
	Product             json.RawMessage         `json:"product"`
	Attribution         json.RawMessage         `json:"attribution"`
	SKU                 json.RawMessage         `json:"sku"`
	Currency            string                  `json:"currency"`
	UnitAmountCents     int                     `json:"unitAmountCents"`
	Quantity            int                     `json:"quantity"`
	SubtotalAmountCents int                     `json:"subtotalAmountCents"`
	ShippingAmountCents int                     `json:"shippingAmountCents"`
	TotalAmountCents    int                     `json:"totalAmountCents"`
	ContractSummary     json.RawMessage         `json:"contractSummary"`
	Fulfillment         json.RawMessage         `json:"fulfillment"`
	PaymentOptions      []CommercePaymentOption `json:"paymentOptions"`
	ExpiresAt           string                  `json:"expiresAt"`
}

type CommercePaymentOption struct {
	Provider string   `json:"provider"`
	Scenes   []string `json:"scenes"`
}

type CreateOrderResponse struct {
	Order CommerceOrder `json:"order"`
}

type CommerceOrder struct {
	OrderNo             string                       `json:"orderNo"`
	Kind                string                       `json:"kind"`
	Status              string                       `json:"status"`
	Region              string                       `json:"region"`
	Currency            string                       `json:"currency"`
	AmountCents         int                          `json:"amountCents"`
	PaymentProvider     string                       `json:"paymentProvider"`
	PrincipalKind       string                       `json:"principalKind"`
	Item                json.RawMessage              `json:"item"`
	PaymentAction       json.RawMessage              `json:"paymentAction"`
	PaymentPresentation *CommercePaymentPresentation `json:"paymentPresentation,omitempty"`
	ExpiresAt           string                       `json:"expiresAt"`
	PaidAt              *string                      `json:"paidAt"`
	ClosedAt            *string                      `json:"closedAt"`
	CreatedAt           string                       `json:"createdAt"`
}

type CommercePaymentPresentation struct {
	Type      string `json:"type"`
	Purpose   string `json:"purpose"`
	MIMEType  string `json:"mimeType"`
	ImagePath string `json:"imagePath"`
	AltText   string `json:"altText"`
	ExpiresAt string `json:"expiresAt"`
}

type OrderStatusResponse struct {
	OrderNo     string          `json:"orderNo"`
	Payment     json.RawMessage `json:"payment"`
	Fulfillment json.RawMessage `json:"fulfillment"`
	ServiceCase *ServiceCase    `json:"serviceCase"`
}

type ServiceCase struct {
	ID               string          `json:"id"`
	CaseNo           string          `json:"caseNo"`
	OrderNo          string          `json:"orderNo"`
	FulfillmentID    string          `json:"fulfillmentId"`
	Work             json.RawMessage `json:"work"`
	Merchant         json.RawMessage `json:"merchant"`
	Status           string          `json:"status"`
	CurrentStageCode string          `json:"currentStageCode"`
	Stages           json.RawMessage `json:"stages"`
	Intake           json.RawMessage `json:"intake"`
	PublicProgress   json.RawMessage `json:"publicProgress"`
	LockVersion      int             `json:"lockVersion"`
	Events           json.RawMessage `json:"events"`
	SubmittedAt      string          `json:"submittedAt"`
	CompletedAt      *string         `json:"completedAt"`
	UpdatedAt        string          `json:"updatedAt"`
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
	ClientRequestID   string                   `json:"clientRequestId"`
	ContractVersion   string                   `json:"contractVersion"`
	CLIVersion        string                   `json:"cliVersion"`
	Manifest          SkillPublicationManifest `json:"manifest"`
	ManifestDigest    string                   `json:"manifestDigest"`
	Artifact          SkillPublicationFile     `json:"artifact"`
	ListingID         string                   `json:"listingId"`
	MerchantAccountID string                   `json:"merchantAccountId"`
}

type CreateSkillPublicationResponse struct {
	PublicationID     string               `json:"publicationId"`
	ListingID         string               `json:"listingId"`
	MerchantAccountID string               `json:"merchantAccountId"`
	DraftRevision     int                  `json:"draftRevision"`
	Status            string               `json:"status"`
	PackageUpload     *UploadAuthorization `json:"packageUpload"`
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
	ID                string                   `json:"id"`
	ListingID         string                   `json:"listingId"`
	MerchantAccountID string                   `json:"merchantAccountId"`
	DraftRevision     int                      `json:"draftRevision"`
	Status            string                   `json:"status"`
	Manifest          SkillPublicationManifest `json:"manifest"`
	Draft             SkillPublicationDraft    `json:"draft"`
	ReviewRevision    int                      `json:"reviewRevision"`
	ReviewDigest      *string                  `json:"reviewDigest"`
	Uploads           []SkillPublicationUpload `json:"uploads"`
	Analysis          *PublicationAnalysis     `json:"analysis"`
	Product           *PublishedProduct        `json:"product"`
	FailureCode       *string                  `json:"failureCode"`
	CreatedAt         string                   `json:"createdAt"`
	UpdatedAt         string                   `json:"updatedAt"`
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

type APIError struct {
	StatusCode int                   `json:"statusCode"`
	Code       string                `json:"code"`
	Message    any                   `json:"message"`
	RequestID  string                `json:"requestId"`
	Recovery   *APIRecoveryReference `json:"recovery,omitempty"`
}

type APIRecoveryReference struct {
	ResourceType string `json:"resourceType"`
	ResourceID   string `json:"resourceId"`
}
