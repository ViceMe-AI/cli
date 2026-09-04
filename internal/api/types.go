package api

import "encoding/json"

const SkillPublicationContractVersion = "2026-08-27"
const PageCustomizationContractVersion = "2026-09-06"

type DeviceAuthorizationRequest struct {
	ClientName string   `json:"clientName"`
	CLIVersion string   `json:"cliVersion"`
	Scopes     []string `json:"scopes"`
	Purpose    string   `json:"purpose,omitempty"`
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
	Status                     string                      `json:"status"`
	Interval                   int                         `json:"interval,omitempty"`
	AccessToken                string                      `json:"accessToken,omitempty"`
	TokenType                  string                      `json:"tokenType,omitempty"`
	ExpiresAt                  string                      `json:"expiresAt,omitempty"`
	Scopes                     []string                    `json:"scopes,omitempty"`
	CreatorOnboardingSelection *CreatorOnboardingSelection `json:"creatorOnboardingSelection,omitempty"`
}

type CreatorOnboardingSelection struct {
	Mode     string                     `json:"mode"`
	Works    []CreatorOnboardingWork    `json:"works,omitempty"`
	Contacts []CreatorOnboardingContact `json:"contacts,omitempty"`
}

type CreatorOnboardingWork struct {
	Title         string `json:"title,omitempty"`
	Summary       string `json:"summary,omitempty"`
	URL           string `json:"url,omitempty"`
	CoverImageURL string `json:"coverImageUrl,omitempty"`
}

type CreatorOnboardingContact struct {
	Platform string `json:"platform"`
	Label    string `json:"label,omitempty"`
	Value    string `json:"value,omitempty"`
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
	StatusVersion    int     `json:"statusVersion"`
}

type MerchantOnboardingEvidence struct {
	ID          string  `json:"id"`
	Kind        string  `json:"kind"`
	FileName    string  `json:"fileName"`
	ContentType string  `json:"contentType"`
	SizeBytes   int64   `json:"sizeBytes"`
	Content     *string `json:"content"`
	CreatedAt   string  `json:"createdAt"`
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
	Onboarding      *MerchantOnboarding `json:"onboarding"`
	Merchant        *MerchantAccount    `json:"merchant"`
	CreatorIdentity *CreatorIdentity    `json:"creatorIdentity"`
	NextAction      string              `json:"nextAction"`
}

// CreatorIdentity 是登录即预生成的创作者身份路由；SuggestedHandle 是申请前的
// handle 派生预览（机器占位且有合规派生时给出，已确认或派生不出为 null）。
type CreatorIdentity struct {
	Handle          string  `json:"handle"`
	DisplayName     string  `json:"displayName"`
	Status          string  `json:"status"`
	ProfilePath     string  `json:"profilePath"`
	MarkdownPath    string  `json:"markdownPath"`
	SuggestedHandle *string `json:"suggestedHandle"`
	ProfileURL      string  `json:"profileUrl"`
	MarkdownURL     string  `json:"markdownUrl"`
}

type GithubAuthorizationStart struct {
	Kind             string  `json:"kind"`
	AuthorizationURL *string `json:"authorizationUrl"`
	AttemptID        *string `json:"attemptId"`
}

type GithubAuthorizationStatus struct {
	Kind string `json:"kind"`
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

type MerchantWorksResponse struct {
	Items []MerchantWork `json:"items"`
}

type PageCustomizationTarget struct {
	Type          string `json:"type"`
	CreatorHandle string `json:"creatorHandle"`
	WorkSlug      string `json:"workSlug,omitempty"`
}

type PageCustomizationMethodDescriptor struct {
	Method string `json:"method"`
	Call   string `json:"call"`
	Access string `json:"access"`
	Effect string `json:"effect"`
}

type PageCustomizationCapabilityDescriptor struct {
	Capability string                              `json:"capability"`
	Targets    []string                            `json:"targets"`
	Methods    []PageCustomizationMethodDescriptor `json:"methods"`
}

type PageCustomizationCapabilityGroup struct {
	Category     string                                  `json:"category"`
	Capabilities []PageCustomizationCapabilityDescriptor `json:"capabilities"`
}

type PageCustomizationTargetDescription struct {
	Target           PageCustomizationTarget            `json:"target"`
	ManifestKind     string                             `json:"manifestKind"`
	SDKVersion       string                             `json:"sdkVersion"`
	ContextSchema    string                             `json:"contextSchema"`
	CapabilityGroups []PageCustomizationCapabilityGroup `json:"capabilityGroups"`
}

type PageCustomizationManifestMetadata struct {
	Name string `json:"name"`
}

type PageCustomizationManifestSpec struct {
	Entry        string   `json:"entry"`
	SDKVersion   string   `json:"sdkVersion"`
	Capabilities []string `json:"capabilities"`
}

type PageCustomizationManifest struct {
	APIVersion string                            `json:"apiVersion"`
	Kind       string                            `json:"kind"`
	Metadata   PageCustomizationManifestMetadata `json:"metadata"`
	Spec       PageCustomizationManifestSpec     `json:"spec"`
}

type PageCustomizationArtifact struct {
	Digest      string `json:"digest"`
	SizeBytes   int64  `json:"sizeBytes"`
	FileName    string `json:"fileName"`
	ContentType string `json:"contentType"`
}

type CreatePageCustomizationDraftRequest struct {
	ClientRequestID   string                    `json:"clientRequestId"`
	ContractVersion   string                    `json:"contractVersion"`
	CLIVersion        string                    `json:"cliVersion"`
	MerchantAccountID string                    `json:"merchantAccountId"`
	Target            PageCustomizationTarget   `json:"target"`
	Artifact          PageCustomizationArtifact `json:"artifact"`
}

type PageCustomizationRelease struct {
	ID               string                     `json:"id"`
	CustomizationID  string                     `json:"customizationId"`
	Version          int                        `json:"version"`
	Status           string                     `json:"status"`
	Target           PageCustomizationTarget    `json:"target"`
	Artifact         PageCustomizationArtifact  `json:"artifact"`
	Manifest         *PageCustomizationManifest `json:"manifest"`
	ValidationIssues []string                   `json:"validationIssues"`
	CreatedAt        string                     `json:"createdAt"`
	UploadedAt       *string                    `json:"uploadedAt"`
	ValidatedAt      *string                    `json:"validatedAt"`
	PublishedAt      *string                    `json:"publishedAt"`
}

type CreatePageCustomizationDraftResponse struct {
	Release PageCustomizationRelease `json:"release"`
}

type PageCustomizationUploadAuthorization struct {
	UploadURL string            `json:"uploadUrl"`
	ExpiresAt string            `json:"expiresAt"`
	Headers   map[string]string `json:"headers"`
}

type PageCustomizationPreview struct {
	ReleaseID string `json:"releaseId"`
	URL       string `json:"url"`
	ExpiresAt string `json:"expiresAt"`
}

type PageCustomizationState struct {
	Target          PageCustomizationTarget    `json:"target"`
	ActiveReleaseID *string                    `json:"activeReleaseId"`
	Revision        int                        `json:"revision"`
	Releases        []PageCustomizationRelease `json:"releases"`
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
	MerchantAccountID string                   `json:"merchantAccountId"`
	Features          []string                 `json:"features"`
	AccessFeatures    []WorkAccessFeatureInput `json:"accessFeatures,omitempty"`
}

type UpdateWorkSdkAccessRequest struct {
	MerchantAccountID     string                   `json:"merchantAccountId"`
	ExpectedConfigVersion int                      `json:"expectedConfigVersion"`
	Features              []string                 `json:"features"`
	AccessFeatures        []WorkAccessFeatureInput `json:"accessFeatures,omitempty"`
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

type WorkAccessFeatureInput struct {
	FeatureKey string           `json:"featureKey"`
	Title      string           `json:"title"`
	PolicyType string           `json:"policyType"`
	Price      *WorkAccessPrice `json:"price"`
	Status     string           `json:"status"`
}

type WorkSdkAccessKeys struct {
	Test string `json:"test"`
	Live string `json:"live"`
}

type WorkSdkAccess struct {
	WorkID         string              `json:"workId"`
	Keys           WorkSdkAccessKeys   `json:"keys"`
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

type MyOrdersResponse struct {
	Items      []CommerceOrder `json:"items"`
	NextCursor *string         `json:"nextCursor"`
}

// PaymentOrder is the order projection shared by the channel payment
// endpoints (creator subscription orders and payment status).
type PaymentOrder struct {
	OrderNo     string  `json:"orderNo"`
	Status      string  `json:"status"`
	ProductID   *string `json:"productId"`
	Provider    string  `json:"provider"`
	Currency    string  `json:"currency"`
	AmountCents int     `json:"amountCents"`
	ExpiresAt   string  `json:"expiresAt"`
}

type CreatePaymentResponse struct {
	Order  PaymentOrder    `json:"order"`
	Action json.RawMessage `json:"action"`
}

type PaymentStatusResponse struct {
	Order  PaymentOrder    `json:"order"`
	PaidAt *string         `json:"paidAt"`
	Action json.RawMessage `json:"action"`
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

type CreateWebsiteReplicaUploadRequest struct {
	ClientRequestID string `json:"clientRequestId"`
	WorkID          string `json:"workId"`
	Title           string `json:"title"`
	Summary         string `json:"summary"`
	FileName        string `json:"fileName"`
	SizeBytes       int64  `json:"sizeBytes"`
	Digest          string `json:"digest"`
	PriceCents      int    `json:"priceCents"`
}

type CreateWebsiteReplicaUploadResponse struct {
	ReplicaID string                            `json:"replicaId"`
	UploadID  string                            `json:"uploadId"`
	Upload    WebsiteReplicaUploadAuthorization `json:"upload"`
}

type WebsiteReplicaUploadAuthorization struct {
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt string            `json:"expiresAt"`
}

type WebsiteReplicaPublication struct {
	ID                string                            `json:"id"`
	ClientRequestID   string                            `json:"clientRequestId"`
	Market            string                            `json:"market"`
	MerchantAccountID string                            `json:"merchantAccountId"`
	WorkID            string                            `json:"workId"`
	ReplicaID         string                            `json:"replicaId"`
	Status            string                            `json:"status"`
	StatusURL         string                            `json:"statusUrl"`
	AllowedActions    []string                          `json:"allowedActions"`
	Retry             WebsiteReplicaPublicationRetry    `json:"retry"`
	Source            WebsiteReplicaPublicationSource   `json:"source"`
	Page              *WebsiteReplicaPublicationSource  `json:"page"`
	Failure           *WebsiteReplicaPublicationFailure `json:"failure"`
	Result            *WebsiteReplicaPublicationResult  `json:"result"`
	SubmittedAt       *string                           `json:"submittedAt"`
	FailedAt          *string                           `json:"failedAt"`
	CancelledAt       *string                           `json:"cancelledAt"`
	CreatedAt         string                            `json:"createdAt"`
	UpdatedAt         string                            `json:"updatedAt"`
}

type WebsiteReplicaPublicationRetry struct {
	AutomaticRetries    int     `json:"automaticRetries"`
	MaxAutomaticRetries int     `json:"maxAutomaticRetries"`
	NextAttemptAt       *string `json:"nextAttemptAt"`
}

type WebsiteReplicaPublicationSource struct {
	FileName    string  `json:"fileName"`
	ContentType string  `json:"contentType"`
	SizeBytes   int64   `json:"sizeBytes"`
	Digest      string  `json:"digest"`
	Status      string  `json:"status"`
	VerifiedAt  *string `json:"verifiedAt"`
}

type WebsiteReplicaPublicationFailure struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type WebsiteReplicaPublicationResult struct {
	WorkURL     string                                `json:"workUrl"`
	VersionID   string                                `json:"versionId"`
	Version     int                                   `json:"version"`
	ShortCode   string                                `json:"shortCode"`
	Instruction string                                `json:"instruction"`
	Product     WebsiteReplicaProduct                 `json:"product"`
	PageRelease *WebsiteReplicaPublicationPageRelease `json:"pageRelease"`
	PublishedAt string                                `json:"publishedAt"`
}

type WebsiteReplicaPublicationPageRelease struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

type WebsiteReplicaProduct struct {
	ID         string `json:"id"`
	SKUID      string `json:"skuId"`
	Title      string `json:"title"`
	Currency   string `json:"currency"`
	PriceCents int    `json:"priceCents"`
}

type CompleteWebsiteReplicaUploadResponse struct {
	ReplicaID   string                   `json:"replicaId"`
	VersionID   string                   `json:"versionId"`
	Version     int                      `json:"version"`
	ShortCode   string                   `json:"shortCode"`
	Instruction string                   `json:"instruction"`
	Product     WebsiteReplicaProduct    `json:"product"`
	BuyerEntry  WebsiteReplicaBuyerEntry `json:"buyerEntry"`
	PublishedAt string                   `json:"publishedAt"`
}

type WebsiteReplicaBuyerEntry struct {
	Instruction   string                     `json:"instruction"`
	Prompts       WebsiteReplicaBuyerPrompts `json:"prompts"`
	ViceMeWorkURL string                     `json:"viceMeWorkUrl"`
}

type WebsiteReplicaBuyerPrompts struct {
	ZH string `json:"zh-CN"`
	EN string `json:"en-US"`
}

type ResolveWebsiteReplicaRequest struct {
	Instruction string `json:"instruction"`
}

type WebsiteReplicaResolution struct {
	ReplicaID     string                `json:"replicaId"`
	ShortCode     string                `json:"shortCode"`
	Title         string                `json:"title"`
	Summary       string                `json:"summary"`
	Creator       WebsiteReplicaCreator `json:"creator"`
	ViceMeWorkURL string                `json:"viceMeWorkUrl"`
	Product       WebsiteReplicaProduct `json:"product"`
}

type CreateWebsiteReplicaSessionRequest struct {
	Instruction     string `json:"instruction"`
	ClientRequestID string `json:"clientRequestId"`
	ReplaySecret    string `json:"replaySecret"`
}

type WebsiteReplicaSession struct {
	SessionID string                   `json:"sessionId"`
	Token     string                   `json:"token"`
	ExpiresAt string                   `json:"expiresAt"`
	Recovered bool                     `json:"recovered"`
	Replica   WebsiteReplicaResolution `json:"replica"`
}

type CheckoutWebsiteReplicaRequest struct {
	AcceptedPriceCents     int    `json:"acceptedPriceCents"`
	QuoteClientRequestID   string `json:"quoteClientRequestId"`
	OrderClientRequestID   string `json:"orderClientRequestId"`
	DownloadRecoverySecret string `json:"downloadRecoverySecret"`
	Locale                 string `json:"locale"`
}

type RecoverWebsiteReplicaDownloadRequest struct {
	OrderNo        string `json:"orderNo"`
	RecoverySecret string `json:"recoverySecret"`
}

type CheckoutWebsiteReplicaResponse struct {
	OrderNo       string                       `json:"orderNo"`
	Status        string                       `json:"status"`
	PaymentAction *WebsiteReplicaPaymentAction `json:"paymentAction"`
	ExpiresAt     string                       `json:"expiresAt"`
	CheckoutURL   string                       `json:"checkoutUrl"`
}

type WebsiteReplicaCreator struct {
	Handle      string `json:"handle"`
	DisplayName string `json:"displayName"`
}

type CreateWebsiteReplicaQuoteRequest struct {
	Instruction     string `json:"instruction"`
	ClientRequestID string `json:"clientRequestId"`
}

type WebsiteReplicaQuote struct {
	ID                  string                         `json:"id"`
	Product             WebsiteReplicaQuoteProduct     `json:"product"`
	Attribution         WebsiteReplicaAttribution      `json:"attribution"`
	SKU                 WebsiteReplicaQuoteSKU         `json:"sku"`
	Currency            string                         `json:"currency"`
	UnitAmountCents     int                            `json:"unitAmountCents"`
	Quantity            int                            `json:"quantity"`
	SubtotalAmountCents int                            `json:"subtotalAmountCents"`
	ShippingAmountCents int                            `json:"shippingAmountCents"`
	TotalAmountCents    int                            `json:"totalAmountCents"`
	ContractSummary     WebsiteReplicaContractSummary  `json:"contractSummary"`
	Fulfillment         WebsiteReplicaQuoteFulfillment `json:"fulfillment"`
	PaymentOptions      []WebsiteReplicaPaymentOption  `json:"paymentOptions"`
	ExpiresAt           string                         `json:"expiresAt"`
}

type WebsiteReplicaQuoteProduct struct {
	ID    string `json:"id"`
	Slug  string `json:"slug"`
	Title string `json:"title"`
}

type WebsiteReplicaAttribution struct {
	SubjectWorkID         string  `json:"subjectWorkId"`
	EntryWorkID           *string `json:"entryWorkId"`
	CommerceApplicationID *string `json:"commerceApplicationId"`
}

type WebsiteReplicaQuoteSKU struct {
	ID              string            `json:"id"`
	Code            string            `json:"code"`
	Title           string            `json:"title"`
	SelectedOptions map[string]string `json:"selectedOptions"`
}

type WebsiteReplicaContractSummary struct {
	PublicFields       map[string]any `json:"publicFields"`
	SensitiveFieldKeys []string       `json:"sensitiveFieldKeys"`
	AssetCount         int            `json:"assetCount"`
}

type WebsiteReplicaQuoteFulfillment struct {
	Capabilities   []string `json:"capabilities"`
	EstimatedState string   `json:"estimatedState"`
}

type WebsiteReplicaPaymentOption struct {
	Provider string   `json:"provider"`
	Scenes   []string `json:"scenes"`
}

type CreateWebsiteReplicaOrderRequest struct {
	QuoteID         string `json:"quoteId"`
	ClientRequestID string `json:"clientRequestId"`
	Locale          string `json:"locale"`
}

type WebsiteReplicaPaymentAction struct {
	Type    string `json:"-"`
	Content string `json:"-"`
}

type WebsiteReplicaOrder struct {
	OrderNo       string                       `json:"orderNo"`
	Status        string                       `json:"status"`
	PaymentAction *WebsiteReplicaPaymentAction `json:"paymentAction"`
	ExpiresAt     string                       `json:"expiresAt"`
}

type WebsiteReplicaOrderStatus struct {
	OrderNo     string                     `json:"orderNo"`
	Payment     WebsiteReplicaPaymentState `json:"payment"`
	Fulfillment *WebsiteReplicaFulfillment `json:"fulfillment"`
	ServiceCase *WebsiteReplicaServiceCase `json:"serviceCase"`
}

type WebsiteReplicaPaymentState struct {
	Status   string  `json:"status"`
	PaidAt   *string `json:"paidAt"`
	ClosedAt *string `json:"closedAt"`
}

type WebsiteReplicaFulfillment struct {
	ID            string                          `json:"id"`
	Status        string                          `json:"status"`
	Version       int                             `json:"version"`
	CurrentTask   *WebsiteReplicaFulfillmentTask  `json:"currentTask"`
	Tasks         []WebsiteReplicaFulfillmentTask `json:"tasks"`
	FailureCode   *string                         `json:"failureCode"`
	ResultSummary *string                         `json:"resultSummary"`
}

type WebsiteReplicaFulfillmentTask struct {
	ID             string  `json:"id"`
	Sequence       int     `json:"sequence"`
	CapabilityCode string  `json:"capabilityCode"`
	Status         string  `json:"status"`
	Version        int     `json:"version"`
	FailureCode    *string `json:"failureCode"`
	ResultSummary  *string `json:"resultSummary"`
	StartedAt      *string `json:"startedAt"`
	CompletedAt    *string `json:"completedAt"`
}

type WebsiteReplicaServiceCase struct {
	ID               string                     `json:"id"`
	CaseNo           string                     `json:"caseNo"`
	OrderNo          string                     `json:"orderNo"`
	FulfillmentID    string                     `json:"fulfillmentId"`
	Work             WebsiteReplicaCaseWork     `json:"work"`
	Merchant         WebsiteReplicaCaseMerchant `json:"merchant"`
	Status           string                     `json:"status"`
	CurrentStageCode string                     `json:"currentStageCode"`
	Stages           []WebsiteReplicaCaseStage  `json:"stages"`
	Intake           map[string]any             `json:"intake"`
	PublicProgress   map[string]any             `json:"publicProgress"`
	LockVersion      int                        `json:"lockVersion"`
	Events           []WebsiteReplicaCaseEvent  `json:"events"`
	SubmittedAt      string                     `json:"submittedAt"`
	CompletedAt      *string                    `json:"completedAt"`
	UpdatedAt        string                     `json:"updatedAt"`
}

type WebsiteReplicaCaseWork struct {
	CreatorHandle string `json:"creatorHandle"`
	Slug          string `json:"slug"`
	Title         string `json:"title"`
}

type WebsiteReplicaCaseMerchant struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

type WebsiteReplicaCaseStage struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	Terminal bool   `json:"terminal"`
}

type WebsiteReplicaCaseEvent struct {
	Sequence      int     `json:"sequence"`
	FromStatus    *string `json:"fromStatus"`
	ToStatus      string  `json:"toStatus"`
	StageCode     string  `json:"stageCode"`
	ActorType     string  `json:"actorType"`
	Note          *string `json:"note"`
	PublicMessage *string `json:"publicMessage"`
	CreatedAt     string  `json:"createdAt"`
}

type WebsiteReplicaLicenseClaims struct {
	SchemaVersion       string `json:"schemaVersion"`
	EntitlementID       string `json:"entitlementId"`
	ReplicaID           string `json:"replicaId"`
	VersionID           string `json:"versionId"`
	Version             int    `json:"version"`
	OrderNo             string `json:"orderNo"`
	ArtifactDigest      string `json:"artifactDigest"`
	LicenseTermsVersion string `json:"licenseTermsVersion"`
	IssuedAt            string `json:"issuedAt"`
}

type WebsiteReplicaLicense struct {
	Claims           WebsiteReplicaLicenseClaims `json:"claims"`
	Algorithm        string                      `json:"algorithm"`
	SigningKeyID     string                      `json:"signingKeyId"`
	SigningPublicKey string                      `json:"signingPublicKey"`
	Signature        string                      `json:"signature"`
}

type WebsiteReplicaDownload struct {
	ReplicaID      string          `json:"replicaId"`
	VersionID      string          `json:"versionId"`
	Version        int             `json:"version"`
	FileName       string          `json:"fileName"`
	SizeBytes      int64           `json:"sizeBytes"`
	ArtifactDigest string          `json:"artifactDigest"`
	DownloadURL    string          `json:"downloadUrl"`
	ExpiresAt      string          `json:"expiresAt"`
	License        json.RawMessage `json:"license"`
}

type CompleteWebsiteReplicaInstallationRequest struct {
	EntitlementID string `json:"entitlementId"`
	VersionID     string `json:"versionId"`
}

type WebsiteReplicaInstallationReceipt struct {
	ReplicaID   string `json:"replicaId"`
	VersionID   string `json:"versionId"`
	Version     int    `json:"version"`
	InstalledAt string `json:"installedAt"`
}

type CompleteUploadRequest struct {
	UploadID string `json:"uploadId"`
}

type SkillPublicationDraft struct {
	Title                 string   `json:"title"`
	SummaryZhCN           *string  `json:"summaryZhCn"`
	UsageInstructionsZhCN *string  `json:"usageInstructionsZhCn"`
	Currency              string   `json:"currency"`
	PriceMinor            *int     `json:"priceMinor"`
	TrialUseLimit         *int     `json:"trialUseLimit"`
	CoverUploadID         *string  `json:"coverUploadId"`
	GalleryUploadIDs      []string `json:"galleryUploadIds"`
}

type UpdateSkillPublicationDraftRequest struct {
	PriceMinor       *int     `json:"priceMinor,omitempty"`
	TrialUseLimit    *int     `json:"trialUseLimit,omitempty"`
	CoverUploadID    *string  `json:"coverUploadId,omitempty"`
	GalleryUploadIDs []string `json:"galleryUploadIds,omitempty"`
}

type SkillPublicationAgentSuggestionPatch struct {
	SummaryZhCN           string   `json:"summaryZhCn"`
	UsageInstructionsZhCN string   `json:"usageInstructionsZhCn"`
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
	UsageInstructionsZhCN string   `json:"usageInstructionsZhCn"`
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
	FailureMessage    *string                     `json:"failureMessage"`
	CreatedAt         string                      `json:"createdAt"`
	UpdatedAt         string                      `json:"updatedAt"`
}

type PublishedSkillEdition struct {
	ProductID     string   `json:"productId"`
	ReleaseID     string   `json:"releaseId"`
	Key           string   `json:"key"`
	Title         string   `json:"title"`
	SortOrder     int      `json:"sortOrder"`
	Highlights    []string `json:"highlights"`
	Currency      string   `json:"currency"`
	PriceMinor    int      `json:"priceMinor"`
	TrialUseLimit *int     `json:"trialUseLimit"`
}

type SkillPublicationNextAction struct {
	Kind      string `json:"kind"`
	ListingID string `json:"listingId"`
}

type SkillAccessSubscription struct {
	Available       bool    `json:"available"`
	PriceMinor      int     `json:"priceMinor"`
	PeriodDays      int     `json:"periodDays"`
	SubscribedUntil *string `json:"subscribedUntil"`
}

type CreatorSubscriptionPlan struct {
	CreatorAccountID      string `json:"creatorAccountId"`
	CreatorHandle         string `json:"creatorHandle"`
	DisplayName           string `json:"displayName"`
	PriceMinor            int    `json:"priceMinor"`
	PeriodDays            int    `json:"periodDays"`
	Status                string `json:"status"`
	ActiveSubscriberCount int    `json:"activeSubscriberCount"`
	UpdatedAt             string `json:"updatedAt"`
}

type SkillAccess struct {
	ProductID         string                  `json:"productId"`
	IsFree            bool                    `json:"isFree"`
	Owned             bool                    `json:"owned"`
	DownloadAvailable bool                    `json:"downloadAvailable"`
	InstallKind       string                  `json:"installKind"`
	PurchaseAvailable bool                    `json:"purchaseAvailable"`
	PurchaseURL       *string                 `json:"purchaseUrl"`
	Subscription      SkillAccessSubscription `json:"subscription"`
	Trial             *SkillAccessTrial       `json:"trial"`
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

/** 付费 Skill 的试用块:available=该款配置了试用次数。 */
type SkillAccessTrial struct {
	Available bool `json:"available"`
	LimitUses int  `json:"limitUses"`
}

type skillTrialGrantRequest struct {
	InstallID string `json:"installId"`
}

type SkillTrialGrant struct {
	InstallID     string `json:"installId"`
	LimitUses     int    `json:"limitUses"`
	RemainingUses int    `json:"remainingUses"`
	// Secret 仅在首次发放时非空;服务端只存哈希,丢失后只能换新的 installId。
	Secret *string `json:"secret"`
}

type skillTrialUseRequest struct {
	InstallID string `json:"installId"`
	Secret    string `json:"secret"`
	// RequestID 是本次使用的幂等键:响应未送达的重试复用同一键由服务端
	// 回放,新使用必须换新键。没有时间窗口兜底。
	RequestID string `json:"requestId"`
}

type SkillTrialUse struct {
	Allowed       bool    `json:"allowed"`
	RemainingUses *int    `json:"remainingUses"`
	LimitUses     *int    `json:"limitUses"`
	Reason        *string `json:"reason"`
	PurchaseURL   *string `json:"purchaseUrl"`
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
	ID                string                   `json:"id"`
	Slug              string                   `json:"slug"`
	Title             string                   `json:"title"`
	Summary           string                   `json:"summary"`
	Status            string                   `json:"status"`
	Visibility        string                   `json:"visibility"`
	Currency          string                   `json:"currency"`
	MinimumPriceCents int                      `json:"minimumPriceCents"`
	MaximumPriceCents int                      `json:"maximumPriceCents"`
	IsFree            bool                     `json:"isFree"`
	InstallKind       *string                  `json:"installKind"`
	PurchaseAvailable bool                     `json:"purchaseAvailable"`
	ActiveRelease     *PublicWorkActiveRelease `json:"activeRelease"`
	Edition           *PublicWorkEdition       `json:"edition"`
	PriceAsOf         string                   `json:"priceAsOf"`
	BuyerFields       []struct {
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
		Products             []PublicWorkProduct `json:"products"`
		Service              any                 `json:"service"`
		WebsiteAction        any                 `json:"websiteAction"`
		WebsiteReplicaAction *struct {
			Instruction string `json:"instruction"`
		} `json:"websiteReplicaAction"`
		Metrics struct {
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
