package api

// Runtime API types and client methods for the Headless Runtime surface
// (skill-app-platform.md §8 / runtime-headless-api.md).
//
// The CLI uses these to:
//   - Read Runtime Contract for a Release (skill inspect)
//   - Create, query and cancel Runtime Runs (job get/wait/cancel)

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
	PublishableKey    string `json:"publishableKey"`
	RuntimeReleaseID  string `json:"runtimeReleaseId"`
	ClientRequestID   string `json:"clientRequestId"`
	Input             any    `json:"input"`
}

type CreateRuntimeRunResponse struct {
	RunID  string `json:"runId"`
	Status string `json:"status"`
}

type RuntimeRunDetail struct {
	RunID       string         `json:"runId"`
	Status      string         `json:"status"`
	Input       any            `json:"input"`
	Result      any            `json:"result"`
	Error       *RuntimeError  `json:"error"`
	Artifacts   []RuntimeArtifact `json:"artifacts"`
	StartedAt   *string        `json:"startedAt"`
	FinishedAt  *string        `json:"finishedAt"`
	RuntimeRelease struct {
		ID              string `json:"id"`
		ContractVersion string `json:"contractVersion"`
	} `json:"runtimeRelease"`
}

type RuntimeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type RuntimeArtifact struct {
	ObjectKey      string `json:"objectKey"`
	Digest         string `json:"digest"`
	SizeBytes      int64  `json:"sizeBytes"`
	ContentType    string `json:"contentType"`
	Kind           string `json:"kind"`
	TurnNumber     int    `json:"turnNumber"`
	ProducedByTool string `json:"producedByTool"`
}

type CancelRuntimeRunResponse struct {
	Cancelled bool   `json:"cancelled"`
	Status    string `json:"status"`
}

type ListRuntimeRunsResponse struct {
	Items      []struct {
		RunID      string  `json:"runId"`
		Status     string  `json:"status"`
		StartedAt  *string `json:"startedAt"`
		FinishedAt *string `json:"finishedAt"`
	} `json:"items"`
	NextCursor *string `json:"nextCursor"`
}
