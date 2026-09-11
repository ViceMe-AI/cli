package api

import (
	"context"
	"net/http"
	"net/url"
)

type WithdrawalCapabilities struct {
	Provider              string `json:"provider"`
	IdentityReady         bool   `json:"identityReady"`
	RequiresPayoutAccount bool   `json:"requiresPayoutAccount"`
	MinAmount             int64  `json:"minAmount"`
	MaxAmount             int64  `json:"maxAmount"`
	RemainingDailyAmount  *int64 `json:"remainingDailyAmount"`
	AgreementVersion      string `json:"agreementVersion"`
}

type WithdrawalMethod struct {
	RecipientRevision string  `json:"recipientRevision"`
	ID                string  `json:"id"`
	Channel           string  `json:"channel"`
	Account           string  `json:"account"`
	BankName          *string `json:"bankName"`
	MinAmount         int64   `json:"minAmount"`
	MaxAmount         int64   `json:"maxAmount"`
}

type WithdrawalContext struct {
	RecipientRevision         *string                `json:"recipientRevision"`
	Capabilities              WithdrawalCapabilities `json:"capabilities"`
	WithdrawableBalanceCents  int64                  `json:"withdrawableBalanceCents"`
	PayoutMethods             []WithdrawalMethod     `json:"payoutMethods"`
	RecommendedPayoutMethodID *string                `json:"recommendedPayoutMethodId"`
	SetupURL                  string                 `json:"setupUrl"`
}

func (c *Client) GetWithdrawalContext(ctx context.Context) (WithdrawalContext, error) {
	var result WithdrawalContext
	err := c.doJSON(ctx, http.MethodGet, "/v1/cli/withdrawals/context", nil, &result, "@stored")
	return result, err
}

type WithdrawalRequest struct {
	RecipientRevision string `json:"recipientRevision"`
	Provider          string `json:"provider"`
	PayoutMethodID    string `json:"payoutMethodId,omitempty"`
	Amount            int64  `json:"amount"`
	SourceReference   string `json:"sourceReference"`
	AgreementVersion  string `json:"agreementVersion"`
}

type WithdrawalPayout struct {
	ID               string `json:"id"`
	OrderID          string `json:"orderId"`
	SourceReference  string `json:"sourceReference"`
	Provider         string `json:"provider"`
	Amount           int64  `json:"amount"`
	Status           string `json:"status"`
	RecipientAccount string `json:"recipientAccount"`
	SucceededAmount  int64  `json:"succeededAmount"`
	ReleasedAmount   int64  `json:"releasedAmount"`
	PendingAmount    int64  `json:"pendingAmount"`
	ActionRequired   *struct {
		Kind   string `json:"kind"`
		ItemID string `json:"itemId"`
	} `json:"actionRequired"`
}

func (c *Client) CreateWithdrawal(ctx context.Context, input WithdrawalRequest) (WithdrawalPayout, error) {
	var result WithdrawalPayout
	err := c.doJSON(ctx, http.MethodPost, "/v1/cli/withdrawals", input, &result, "@stored")
	return result, err
}

func (c *Client) FindWithdrawal(ctx context.Context, sourceReference string, refresh bool) (*WithdrawalPayout, error) {
	var result struct {
		Payout *WithdrawalPayout `json:"payout"`
	}
	endpoint := "/v1/cli/withdrawals/by-source-reference?sourceReference=" + url.QueryEscape(sourceReference)
	method := http.MethodGet
	if refresh {
		method = http.MethodPost
		endpoint = "/v1/cli/withdrawals/by-source-reference/refresh?sourceReference=" + url.QueryEscape(sourceReference)
	}
	err := c.doJSON(ctx, method, endpoint, nil, &result, "@stored")
	return result.Payout, err
}
