package command

import (
	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
)

// A payment result is a host presentation boundary, not an installation receipt.
// The continuation revalidates the original entitlement without creating an order.
type replicaSupportResult struct {
	NextAction   string                         `json:"nextAction"`
	OrderNo      string                         `json:"orderNo"`
	Title        string                         `json:"title"`
	AmountCents  int                            `json:"amountCents"`
	Currency     string                         `json:"currency"`
	Target       string                         `json:"target"`
	Payment      api.WebsiteReplicaPaymentState `json:"payment"`
	Presentation replicaSupportPresentation     `json:"presentation"`
	Continuation replicaSupportContinuation     `json:"continuation"`
}

type replicaSupportPresentation struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type replicaSupportContinuation struct {
	Mode string   `json:"mode"`
	Args []string `json:"args"`
}

func prepareReplicaSupportResult(runtime *Runtime, store replicaPurchaseStore, state replicaPurchaseState, payment api.WebsiteReplicaPaymentState, anonymous bool) (replicaInstallResult, error) {
	if payment.Status != "PAID" || state.OrderNo == "" || state.PriceCents <= 0 {
		return replicaInstallResult{}, invalidReplicaResponse("support presentation requires a confirmed paid order")
	}
	if err := store.verifyReservation(state); err != nil {
		return replicaInstallResult{}, err
	}
	title, description := "已支付", "付款已确认，请勿重复支付"
	if state.Locale == "en-US" {
		title, description = "Paid", "Payment confirmed. Please do not pay again."
	}
	args := []string{"replica", "install", "VICEME-REPLICA:" + state.ShortCode, "--recovery-only", "--expected-order-no", state.OrderNo, "--target", state.Target, "--profile", runtime.profile.Name}
	if replicaUUIDPattern.MatchString(state.InvitationFlowID) {
		args = append(args, "--invitation-flow-id", state.InvitationFlowID)
	}
	if anonymous {
		args = append(args, "--anonymous")
	}
	return replicaInstallResult{SupportResult: &replicaSupportResult{
		NextAction: "PRESENT_SUPPORT_RESULT", OrderNo: state.OrderNo, Title: state.ProductTitle,
		AmountCents: state.PriceCents, Currency: state.Currency, Target: state.Target, Payment: payment,
		Presentation: replicaSupportPresentation{Title: title, Description: description},
		Continuation: replicaSupportContinuation{Mode: "RECOVERY_ONLY", Args: args},
	}}, nil
}

func verifyReplicaSupportContinuation(store replicaPurchaseStore, expectedOrderNo string) error {
	completion, completed, err := store.loadCompletion()
	if err != nil {
		return err
	}
	if completed && completion.Result.OrderNo == expectedOrderNo {
		return nil
	}
	state, exists, err := store.load()
	if err != nil {
		return err
	}
	if !completed && exists && state.OrderNo == expectedOrderNo {
		return nil
	}
	return output.Policy("REPLICA_PURCHASE_RECOVERY_CONFLICT", "support continuation does not match the original purchase").WithDetails(map[string]any{"nextAction": "STOP_AND_REPORT"})
}
