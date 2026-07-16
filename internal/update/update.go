// Package update defines the release/update seam without choosing a package
// manager or unsigned download mechanism in the MVP.
package update

import "context"

type CheckResult struct {
	CurrentVersion   string `json:"current_version"`
	AvailableVersion string `json:"available_version,omitempty"`
	UpdateAvailable  bool   `json:"update_available"`
	Method           string `json:"method,omitempty"`
}

type TargetResult struct {
	Target string `json:"target"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type ApplyResult struct {
	CLIVersion string         `json:"cli_version"`
	Targets    []TargetResult `json:"targets"`
}

// Service will be implemented by the signed release-manifest/npm/Homebrew
// distribution layer. Keeping it outside command prevents the MVP from
// silently introducing an unverified self-update path.
type Service interface {
	Check(context.Context) (CheckResult, error)
	Apply(context.Context, CheckResult) (ApplyResult, error)
}
