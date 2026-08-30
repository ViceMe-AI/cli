package api

import "encoding/json"

const SkillPublicationContractVersion = "2026-08-27"

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

type GithubChannelVerified struct {
	Verified bool `json:"verified"`
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
	OwnershipStatus  string  `json:"ownershipStatus"`
	ClaimProvider    *string `json:"claimProvider"`
	StatusVersion    int     `json:"statusVersion"`
}

type MerchantOnboardingEvidence struct {
	ID          string `json:"id"`
	FileName    string `json:"fileName"`
	ContentType string `json:"contentType"`
	SizeBytes   int64  `json:"sizeBytes"`
	CreatedAt   string `json:"createdAt"`
}

type MerchantOnboarding struct {
	ID                   string                       `json:"id"`
	Kind                 string                       `json:"kind"`
	MerchantAccountID    *string                      `json:"merchantAccountId"`
	Provider             *string                      `json:"provider"`
	RequestedHandle      *string                      `json:"requestedHandle"`
	DisplayName          string                       `json:"displayName"`
	Status               string                       `json:"status"`
	LockVersion          int                          `json:"lockVersion"`
	PublicAccountName    *string                      `json:"publicAccountName"`
	ProfileURL           *string                      `json:"profileUrl"`
	ReservationExpiresAt *string                      `json:"reservationExpiresAt"`
	ReasonCode           *string                      `json:"reasonCode"`
	ReviewNote           *string                      `json:"reviewNote"`
	SubmittedAt          *string                      `json:"submittedAt"`
	ReviewedAt           *string                      `json:"reviewedAt"`
	Evidence             []MerchantOnboardingEvidence `json:"evidence"`
}

type CurrentMerchantOnboarding struct {
	Onboarding *MerchantOnboarding `json:"onboarding"`
	Merchant   *MerchantAccount    `json:"merchant"`
	NextAction string              `json:"nextAction"`
}

type GithubAuthorizationStart struct {
	Kind             string              `json:"kind"`
	AuthorizationURL *string             `json:"authorizationUrl"`
	AttemptID        *string             `json:"attemptId"`
	Onboarding       *MerchantOnboarding `json:"onboarding,omitempty"`
}

type GithubAuthorizationStatus struct {
	Kind string `json:"kind"`
}

type MerchantAccountsResponse struct {
	Items []MerchantAccount `json:"items"`
}

type MerchantWork struct {
	ID             string          `json:"id"`
	Kind           string          `json:"kind"`
	Origin         string          `json:"origin"`
	Slug           string          `json:"slug"`
	Title          string          `json:"title"`
	Status         string          `json:"status"`
	Revision       int             `json:"revision"`
	Owner          json.RawMessage `json:"owner"`
	Skill          json.RawMessage `json:"skill"`
	Service        json.RawMessage `json:"service"`
	Website        json.RawMessage `json:"website"`
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

type SdkWorkFeatureConfig struct {
	FeatureKey string               `json:"featureKey"`
	Title      string               `json:"title"`
	Policy     SdkWorkFeaturePolicy `json:"policy"`
	PriceCents *int                 `json:"priceCents"`
	Status     string               `json:"status"`
}

type SdkWorkFeaturePolicy struct {
	Type string `json:"type"`
}

type CreateSdkWorkRequest struct {
	DisplayName string `json:"displayName"`
}

type PublishCreatorWebsiteRequest struct {
	ClientRequestID    string        `json:"clientRequestId"`
	ClientWorkID       string        `json:"clientWorkId"`
	SourceDigest       string        `json:"sourceDigest"`
	DisplayName        string        `json:"displayName"`
	CreatorDisplayName string        `json:"creatorDisplayName,omitempty"`
	SourceURL          string        `json:"sourceUrl,omitempty"`
	DescriptionZhCN    string        `json:"descriptionZhCn,omitempty"`
	DescriptionEnUS    string        `json:"descriptionEnUs,omitempty"`
	Cover              *WebsiteCover `json:"cover,omitempty"`
}

type WebsiteCover struct {
	Digest      string `json:"digest"`
	SizeBytes   int64  `json:"sizeBytes"`
	FileName    string `json:"fileName"`
	ContentType string `json:"contentType"`
}

type AuthorizeWebsiteCoverUploadRequest struct {
	ClientWorkID string `json:"clientWorkId"`
	WebsiteCover
}

type ApplySdkWorkRequest struct {
	ExpectedConfigVersion int                    `json:"expectedConfigVersion"`
	DisplayName           string                 `json:"displayName"`
	Features              []SdkWorkFeatureConfig `json:"features"`
	Status                string                 `json:"status"`
}

type SdkWork struct {
	CreatorWorkID *string                `json:"creatorWorkId"`
	WorkKey       string                 `json:"workKey"`
	Publication   *SdkWorkPublication    `json:"publication"`
	DisplayName   string                 `json:"displayName"`
	Status        string                 `json:"status"`
	ConfigVersion int                    `json:"configVersion"`
	Offers        []SdkWorkOffer         `json:"offers"`
	Features      []SdkWorkFeatureConfig `json:"features"`
	Capabilities  []string               `json:"capabilities"`
	CreatedAt     string                 `json:"createdAt"`
	UpdatedAt     string                 `json:"updatedAt"`
}

type SdkWorkPublication struct {
	ClientWorkID    string  `json:"clientWorkId"`
	SourceDigest    string  `json:"sourceDigest"`
	SourceURL       *string `json:"sourceUrl"`
	DescriptionZhCN *string `json:"descriptionZhCn"`
	DescriptionEnUS *string `json:"descriptionEnUs"`
	CoverURL        *string `json:"coverUrl"`
	ReleaseID       string  `json:"releaseId"`
	Version         int     `json:"version"`
	PublishedAt     string  `json:"publishedAt"`
	Unchanged       bool    `json:"unchanged"`
}

type SdkWorkOffer struct {
	ID          string `json:"id"`
	FeatureKey  string `json:"featureKey"`
	Type        string `json:"type"`
	AmountCents int    `json:"amountCents"`
	Currency    string `json:"currency"`
	Status      string `json:"status"`
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
	PublishMode string                  `json:"publishMode" yaml:"publishMode"`
	Source      SkillPublicationSource  `json:"source" yaml:"source"`
	Edition     SkillPublicationEdition `json:"edition" yaml:"edition"`
	Sale        SkillPublicationSale    `json:"sale" yaml:"sale"`
}

type SkillPublicationSource struct {
	Type            string  `json:"type" yaml:"type"`
	Entry           string  `json:"entry" yaml:"entry"`
	Repository      string  `json:"repository,omitempty" yaml:"repository,omitempty"`
	Ref             string  `json:"ref,omitempty" yaml:"ref,omitempty"`
	Private         *bool   `json:"private,omitempty" yaml:"private,omitempty"`
	OwnerSubjectID  string  `json:"ownerSubjectId,omitempty" yaml:"ownerSubjectId,omitempty"`
	Path            *string `json:"path,omitempty" yaml:"path,omitempty"`
	SkillID         string  `json:"skillId,omitempty" yaml:"skillId,omitempty"`
	ArtifactVersion string  `json:"artifactVersion,omitempty" yaml:"artifactVersion,omitempty"`
	ArtifactDigest  string  `json:"artifactDigest,omitempty" yaml:"artifactDigest,omitempty"`
	SourceReceiptID string  `json:"sourceReceiptId,omitempty" yaml:"sourceReceiptId,omitempty"`
}

// MarshalJSON keeps the repository root explicit for GitHub sources: the
// server canonicalizes the manifest after parsing, so the digest only matches
// when a root-level GitHub source serializes "path": null instead of omitting
// the key. Other source kinds keep omitting the union-inapplicable fields.
func (s SkillPublicationSource) MarshalJSON() ([]byte, error) {
	type skillPublicationSourceAlias SkillPublicationSource
	encoded, err := json.Marshal(skillPublicationSourceAlias(s))
	if err != nil {
		return nil, err
	}
	if s.Type != "GITHUB" || s.Path != nil {
		return encoded, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return nil, err
	}
	fields["path"] = []byte("null")
	return json.Marshal(fields)
}

type SkillPublicationEdition struct {
	Key        string   `json:"key" yaml:"key"`
	Title      string   `json:"title" yaml:"title"`
	SortOrder  int      `json:"sortOrder" yaml:"sortOrder"`
	Highlights []string `json:"highlights" yaml:"highlights"`
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
	Resolution        string               `json:"resolution"`
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
	ID                string                      `json:"id"`
	ListingID         string                      `json:"listingId"`
	MerchantAccountID string                      `json:"merchantAccountId"`
	DraftRevision     int                         `json:"draftRevision"`
	Status            string                      `json:"status"`
	Manifest          SkillPublicationManifest    `json:"manifest"`
	Draft             SkillPublicationDraft       `json:"draft"`
	ReviewRevision    int                         `json:"reviewRevision"`
	ReviewDigest      *string                     `json:"reviewDigest"`
	Uploads           []SkillPublicationUpload    `json:"uploads"`
	Analysis          *PublicationAnalysis        `json:"analysis"`
	Product           *PublishedProduct           `json:"product"`
	Editions          []PublishedSkillEdition     `json:"editions"`
	NextAction        *SkillPublicationNextAction `json:"nextAction"`
	FailureCode       *string                     `json:"failureCode"`
	CreatedAt         string                      `json:"createdAt"`
	UpdatedAt         string                      `json:"updatedAt"`
}

type PublishedSkillEdition struct {
	ProductID  string   `json:"productId"`
	ReleaseID  string   `json:"releaseId"`
	Key        string   `json:"key"`
	Title      string   `json:"title"`
	SortOrder  int      `json:"sortOrder"`
	Highlights []string `json:"highlights"`
	Currency   string   `json:"currency"`
	PriceMinor int      `json:"priceMinor"`
}

type SkillPublicationNextAction struct {
	Kind      string `json:"kind"`
	ListingID string `json:"listingId"`
}

type SkillAccess struct {
	ProductID         string  `json:"productId"`
	IsFree            bool    `json:"isFree"`
	Owned             bool    `json:"owned"`
	DownloadAvailable bool    `json:"downloadAvailable"`
	InstallKind       string  `json:"installKind"`
	PurchaseAvailable bool    `json:"purchaseAvailable"`
	PurchaseURL       *string `json:"purchaseUrl"`
	UnavailableReason *string `json:"unavailableReason"`
	Edition           struct {
		Key        string   `json:"key"`
		Title      string   `json:"title"`
		SortOrder  int      `json:"sortOrder"`
		Highlights []string `json:"highlights"`
	} `json:"edition"`
	Release struct {
		ID             string `json:"id"`
		ArtifactDigest string `json:"artifactDigest"`
		FileName       string `json:"fileName"`
	} `json:"release"`
}

type DownloadURL struct {
	URL            string `json:"url"`
	FileName       string `json:"fileName"`
	ReleaseID      string `json:"releaseId"`
	ArtifactDigest string `json:"artifactDigest"`
	ExpiresAt      string `json:"expiresAt"`
}

type XiaohongshuSkillCandidate struct {
	SkillID           string  `json:"skillId"`
	SkillName         *string `json:"skillName"`
	AuthorDisplayName *string `json:"authorDisplayName"`
	ArtifactVersion   string  `json:"artifactVersion"`
	ArtifactDigest    string  `json:"artifactDigest"`
	ArtifactSizeBytes int64   `json:"artifactSizeBytes"`
	ObservedAt        string  `json:"observedAt"`
}

type XiaohongshuSkillSearch struct {
	Items []XiaohongshuSkillCandidate `json:"items"`
}

type PublicWorkEdition struct {
	Key        string   `json:"key"`
	Title      string   `json:"title"`
	SortOrder  int      `json:"sortOrder"`
	Highlights []string `json:"highlights"`
}

type PublicWorkActiveRelease struct {
	ID             string `json:"id"`
	ArtifactDigest string `json:"artifactDigest"`
	FileName       string `json:"fileName"`
}

type PublicWorkProduct struct {
	ID                        string                   `json:"id"`
	Slug                      string                   `json:"slug"`
	Title                     string                   `json:"title"`
	Summary                   string                   `json:"summary"`
	Status                    string                   `json:"status"`
	Visibility                string                   `json:"visibility"`
	Currency                  string                   `json:"currency"`
	MinimumPriceCents         int                      `json:"minimumPriceCents"`
	MaximumPriceCents         int                      `json:"maximumPriceCents"`
	IsFree                    bool                     `json:"isFree"`
	InstallKind               *string                  `json:"installKind"`
	PurchaseAvailable         bool                     `json:"purchaseAvailable"`
	PurchaseUnavailableReason *string                  `json:"purchaseUnavailableReason"`
	ActiveRelease             *PublicWorkActiveRelease `json:"activeRelease"`
	Edition                   *PublicWorkEdition       `json:"edition"`
	PriceAsOf                 string                   `json:"priceAsOf"`
	BuyerFields               []struct {
		Key         string `json:"key"`
		Label       string `json:"label"`
		Kind        string `json:"kind"`
		Required    bool   `json:"required"`
		Sensitivity string `json:"sensitivity"`
	} `json:"buyerFields"`
	Fulfillment []struct {
		Sequence       int    `json:"sequence"`
		CapabilityCode string `json:"capabilityCode"`
	} `json:"fulfillment"`
	PurchaseSkill *struct {
		StableName    string `json:"stableName"`
		InstallPrompt string `json:"installPrompt"`
	} `json:"purchaseSkill"`
}

type PublicWorkProjection struct {
	Creator struct {
		ID                 string  `json:"id"`
		Handle             string  `json:"handle"`
		DisplayName        string  `json:"displayName"`
		AvatarURL          *string `json:"avatarUrl"`
		Occupation         *string `json:"occupation"`
		Bio                *string `json:"bio"`
		ExternalIdentities []struct {
			Provider       string  `json:"provider"`
			ExternalHandle *string `json:"externalHandle"`
		} `json:"externalIdentities"`
		IsOfficial bool `json:"isOfficial"`
	} `json:"creator"`
	Work struct {
		ID            string `json:"id"`
		Kind          string `json:"kind"`
		Slug          string `json:"slug"`
		Status        string `json:"status"`
		CanonicalPath string `json:"canonicalPath"`
		MarkdownPath  string `json:"markdownPath"`
		Revision      struct {
			Version             int              `json:"version"`
			Digest              string           `json:"digest"`
			Title               string           `json:"title"`
			Summary             string           `json:"summary"`
			BodyMarkdown        string           `json:"bodyMarkdown"`
			TemplateType        string           `json:"templateType"`
			Tags                []string         `json:"tags"`
			UsageInstructions   *string          `json:"usageInstructions"`
			ServiceInstructions *string          `json:"serviceInstructions"`
			Media               []map[string]any `json:"media"`
			ActivatedAt         *string          `json:"activatedAt"`
		} `json:"revision"`
		Products      []PublicWorkProduct `json:"products"`
		Service       any                 `json:"service"`
		WebsiteAction any                 `json:"websiteAction"`
		Metrics       struct {
			LikeCount    int `json:"likeCount"`
			CommentCount int `json:"commentCount"`
		} `json:"metrics"`
		CreatedAt string `json:"createdAt"`
		UpdatedAt string `json:"updatedAt"`
	} `json:"work"`
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
