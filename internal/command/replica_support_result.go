package command

import (
	"os"
	"path/filepath"
	"strings"

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
	WidgetPath         string `json:"widgetPath"`
	WidgetMIMEType     string `json:"widgetMimeType"`
	ReplacesWidgetPath string `json:"replacesWidgetPath"`
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
	data := replicaPaymentWidgetData(state)
	data.Status = "PAID"
	data.ResultTitle = "创作者已收到你的支持"
	data.ResultDescription = "感谢你支持这个创意，正在为你准备作品。"
	if state.Locale == "en-US" {
		data.ResultTitle = "The creator has received your support"
		data.ResultDescription = "Thank you for supporting this idea. Your work is being prepared."
	}
	widget, err := renderPaymentWidget(data, "")
	if err != nil {
		return replicaInstallResult{}, supportPresentationFailure(err)
	}
	directory, err := filepath.Abs(filepath.Join(runtime.configBase, commercePaymentPresentationDirectory))
	if err != nil {
		return replicaInstallResult{}, supportPresentationFailure(err)
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return replicaInstallResult{}, supportPresentationFailure(err)
	}
	if err := secureCommercePaymentDirectory(directory); err != nil {
		return replicaInstallResult{}, supportPresentationFailure(err)
	}
	stem := strings.TrimSuffix(commercePaymentPresentationFilename(state.OrderNo), ".png")
	// A distinct path avoids hosts reusing a cached copy of the pending HTML.
	path := filepath.Join(directory, stem+".support.html")
	if err := writeCommercePaymentPresentation(path, widget); err != nil {
		return replicaInstallResult{}, supportPresentationFailure(err)
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
		Presentation: replicaSupportPresentation{WidgetPath: path, WidgetMIMEType: "text/html", ReplacesWidgetPath: filepath.Join(directory, stem+".html")},
		Continuation: replicaSupportContinuation{Mode: "RECOVERY_ONLY", Args: args},
	}}, nil
}

func supportPresentationFailure(err error) error {
	return output.Internal("REPLICA_SUPPORT_PRESENTATION_FAILED", "confirmed support result could not be prepared", err).WithDetails(map[string]any{"stage": "PRESENT_SUPPORT_RESULT"})
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
