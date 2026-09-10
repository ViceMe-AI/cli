package command

import "github.com/ViceMe-AI/cli/internal/output"

func replicaPaymentRestartRequired(state replicaPurchaseState, status string) *output.Error {
	return output.Policy("REPLICA_PAYMENT_RESTART_REQUIRED", "the original payment attempt is retained; replacing it requires a new explicit user request").WithDetails(map[string]any{
		"nextAction": "STOP_AND_REPORT", "orderNo": state.OrderNo, "status": status,
		"expiresAt": state.OrderExpiresAt,
	}).WithHint("do not automatically retry, cancel or replace this order; use recovery-only for a payment check when requested; --replace-unpaid-order must name this order and requires a new explicit payment request")
}

func verifyReplicaPaymentReplacement(store replicaPurchaseStore, orderNo string) error {
	state, exists, err := store.load()
	if err != nil {
		return err
	}
	if !exists || state.OrderNo != orderNo {
		return output.Policy("REPLICA_PURCHASE_RECOVERY_CONFLICT", "payment replacement does not match the original attempt").WithDetails(map[string]any{"nextAction": "STOP_AND_REPORT"})
	}
	return nil
}

func replicaStoppedPayment(err error, runtime *Runtime, state replicaPurchaseState, anonymous bool) error {
	if err == nil || state.OrderNo == "" {
		return err
	}
	failure := *output.AsError(err)
	switch failure.Subtype {
	case "REPLICA_PAYMENT_TIMEOUT", "REPLICA_PAYMENT_TERMINAL", "REPLICA_PAYMENT_INTERRUPTED", "REPLICA_PAYMENT_RESTART_REQUIRED":
	default:
		return err
	}
	details := map[string]any{}
	if previous, ok := failure.Details.(map[string]any); ok {
		for key, value := range previous {
			details[key] = value
		}
	}
	details["nextAction"] = "STOP_AND_REPORT"
	details["orderNo"] = state.OrderNo
	args := []string{"replica", "install", "VICEME-REPLICA:" + state.ShortCode, "--recovery-only", "--expected-order-no", state.OrderNo, "--target", state.Target, "--profile", runtime.profile.Name}
	if anonymous {
		args = append(args, "--anonymous")
	}
	details["recovery"] = map[string]any{"mode": "RECOVERY_ONLY", "requiresUserRequest": true, "args": args}
	failure.Details = details
	failure.Retryable = false
	return &failure
}
