package api

import (
	"regexp"

	"github.com/ViceMe-AI/cli/internal/output"
)

// Runtime API types and client methods for the Headless Runtime surface
// (skill-app-platform.md §8 / runtime-headless-api.md).
//
// The CLI uses these to:
//   - Read Runtime Contract for a Release (skill inspect)
//   - Create, query and cancel Runtime Runs (job get/wait/cancel)
//   - Download Run Artifacts (job artifacts)

// Run ids are UUIDs issued by the API; they are interpolated into the URL
// path. url.PathEscape does NOT escape "..", and doJSON joins the endpoint
// with path.Join, so a hostile id could fold the request into an unexpected
// route — reject anything outside a plain path segment (#66 review P2).
var safeRunIDSegmentRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func validateRunID(runID string) error {
	if !safeRunIDSegmentRe.MatchString(runID) {
		return output.Validation("runtime_run_id", "run id must be a plain path segment")
	}
	return nil
}

type RuntimeContractResponse struct {
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

type CreateRuntimeRunRequest struct {
	PublishableKey   string `json:"publishableKey"`
	RuntimeReleaseID string `json:"runtimeReleaseId"`
	ClientRequestID  string `json:"clientRequestId"`
	Input            any    `json:"input"`
}

type CreateRuntimeRunResponse struct {
	RunID  string `json:"runId"`
	Status string `json:"status"`
}

// RuntimeRunDetail mirrors the Shop runtimeRunDetailSchema. The Shop is the
// source of truth for this contract: artifacts expose artifactId + downloadUrl,
// never objectKey + digest (the original CLI assumption that the review found).
type RuntimeRunDetail struct {
	RunID          string            `json:"runId"`
	Status         string            `json:"status"`
	Environment    string            `json:"environment"`
	Input          any               `json:"input"`
	Result         any               `json:"result"`
	Error          *RuntimeError     `json:"error"`
	Artifacts      []RuntimeArtifact `json:"artifacts"`
	StartedAt      *string           `json:"startedAt"`
	FinishedAt     *string           `json:"finishedAt"`
	CreatedAt      string            `json:"createdAt"`
	UpdatedAt      string            `json:"updatedAt"`
	RuntimeRelease struct {
		ID              string `json:"id"`
		ContractVersion string `json:"contractVersion"`
	} `json:"runtimeRelease"`
}

type RuntimeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// RuntimeArtifact mirrors the Shop runtimeRunArtifactSchema. The bytes are
// fetched separately via DownloadURL (a short-lived signed URL); the CLI never
// receives them inline in the run detail.
type RuntimeArtifact struct {
	ArtifactID  string `json:"artifactId"`
	ContentType string `json:"contentType"`
	SizeBytes   int64  `json:"sizeBytes"`
	Kind        string `json:"kind"`
	TurnNumber  int    `json:"turnNumber"`
	CreatedAt   string `json:"createdAt"`
	DownloadURL string `json:"downloadUrl"`
}

type CancelRuntimeRunResponse struct {
	RunID     string `json:"runId"`
	Status    string `json:"status"`
	Cancelled bool   `json:"cancelled"`
}

type ListRuntimeRunsResponse struct {
	Items []struct {
		RunID      string  `json:"runId"`
		Status     string  `json:"status"`
		StartedAt  *string `json:"startedAt"`
		FinishedAt *string `json:"finishedAt"`
	} `json:"items"`
	NextCursor *string `json:"nextCursor"`
}

// CreateRuntimeTicketResponse is returned by POST /v1/runtime/tickets. A Runtime
// Ticket is a short-lived bearer token that authorizes Run access (job get/wait/
// cancel/artifacts) scoped to a single App environment.
type CreateRuntimeTicketResponse struct {
	Ticket    string `json:"ticket"`
	ExpiresAt string `json:"expiresAt"`
}
