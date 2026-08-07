package api

// Managed Skill App API types for Slice E (skill-app-platform.md §10/§11).
//
// The Shop managed-app.ts contracts are the source of truth for these types.

type ManagedAppTemplate struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Digest      string `json:"digest"`
	SDKPackage  string `json:"sdkPackage"`
	SDKVersion  string `json:"sdkVersion"`
	DownloadURL string `json:"downloadUrl"`
}

type ManagedAppRuntimeContract struct {
	RuntimeReleaseID string `json:"runtimeReleaseId"`
	ContractVersion  string `json:"contractVersion"`
	ContractDigest   string `json:"contractDigest"`
	InputSchema      any    `json:"inputSchema"`
	OutputSchema     any    `json:"outputSchema"`
	ToolAllowlist    []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"toolAllowlist"`
}

type InitManagedAppRequest struct {
	ClientRequestID  string `json:"clientRequestId"`
	Name             string `json:"name"`
	RuntimeReleaseID string `json:"runtimeReleaseId"`
	TemplateName     string `json:"templateName"`
	TemplateVersion  string `json:"templateVersion"`
	// appSdkVersion is intentionally absent: the Shop derives it from its
	// server-side template record (authoritative), not from the client.
}

// InitManagedAppResponse mirrors the Shop managedAppReleaseSchema. Note that
// the Shop returns `releaseId` (not `candidateId`-only); the CLI persists both.
type InitManagedAppResponse struct {
	AppID           string `json:"appId"`
	ReleaseID       string `json:"releaseId"`
	CandidateID     string `json:"candidateId"`
	Status          string `json:"status"`
	SourceDigest    string `json:"sourceDigest"`
	TemplateName    string `json:"templateName"`
	TemplateVersion string `json:"templateVersion"`
	Environment     string `json:"environment"`
	PublishableKey  string `json:"publishableKey"`
}

// UploadSourceRequest mirrors uploadManagedAppSourceRequestSchema. These are the
// multipart text fields sent alongside the source archive file.
type UploadSourceRequest struct {
	AppID                 string `json:"appId"`
	CandidateID           string `json:"candidateId"`
	RuntimeReleaseID      string `json:"runtimeReleaseId"`
	RuntimeContractDigest string `json:"runtimeContractDigest"`
	TemplateName          string `json:"templateName"`
	TemplateVersion       string `json:"templateVersion"`
	TemplateDigest        string `json:"templateDigest"`
	// appSdkVersion is server-authoritative (template record), not client-supplied.
}

type UploadSourceResponse struct {
	ReleaseID    string `json:"releaseId"`
	CandidateID  string `json:"candidateId"`
	SourceDigest string `json:"sourceDigest"`
}

// UploadBuildArtifactRequest mirrors uploadManagedAppBuildArtifactRequestSchema.
type UploadBuildArtifactRequest struct {
	AppID       string `json:"appId"`
	CandidateID string `json:"candidateId"`
}

type UploadBuildArtifactResponse struct {
	ReleaseID   string `json:"releaseId"`
	CandidateID string `json:"candidateId"`
	Status      string `json:"status"`
	BuildDigest string `json:"buildDigest"`
}

type CreatePreviewResponse struct {
	ReleaseID    string `json:"releaseId"`
	CandidateID  string `json:"candidateId"`
	Status       string `json:"status"`
	PreviewRunID string `json:"previewRunId"`
	PreviewURL   string `json:"previewUrl"`
}

type PublishReleaseRequest struct {
	AppID                         string `json:"appId"`
	CandidateID                   string `json:"candidateId"`
	ExpectedSourceDigest          string `json:"expectedSourceDigest"`
	ExpectedBuildDigest           string `json:"expectedBuildDigest"`
	ExpectedRuntimeContractDigest string `json:"expectedRuntimeContractDigest"`
}

type PublishReleaseResponse struct {
	ReleaseID   string `json:"releaseId"`
	CandidateID string `json:"candidateId"`
	Status      string `json:"status"`
	PublishedAt string `json:"publishedAt"`
}
