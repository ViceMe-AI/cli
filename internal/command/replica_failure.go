package command

import (
	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
)

// Preserve payment facts from this order's validated API response when later
// delivery fails. An order number or an opened payment page alone is not proof.
func replicaPaidFailure(err error, runtime *Runtime, state replicaPurchaseState, payment api.WebsiteReplicaPaymentState, anonymous bool) error {
	if err == nil || payment.Status != "PAID" || state.OrderNo == "" {
		return err
	}
	failure := *output.AsError(err)
	details := map[string]any{}
	if previous, ok := failure.Details.(map[string]any); ok {
		for key, value := range previous {
			details[key] = value
		}
	} else if failure.Details != nil {
		details["causeDetails"] = failure.Details
	}
	details["orderNo"] = state.OrderNo
	details["payment"] = payment
	if _, exists := details["stage"]; !exists {
		details["stage"] = "INSTALL_REPLICA"
	}
	details["nextAction"] = "STOP_AND_REPORT"
	args := []string{"replica", "install", "VICEME-REPLICA:" + state.ShortCode, "--recovery-only", "--target", state.Target, "--profile", runtime.profile.Name}
	if anonymous {
		args = append(args, "--anonymous")
	}
	details["recovery"] = map[string]any{"mode": "RECOVERY_ONLY", "requiresUserRequest": true, "args": args}
	failure.Details = details
	if failure.Hint != "" {
		failure.Hint += "; "
	}
	failure.Hint += "payment is confirmed; do not purchase again; resolve the reported delivery failure, then use recovery-only for the same order when the user requests recovery"
	return &failure
}
