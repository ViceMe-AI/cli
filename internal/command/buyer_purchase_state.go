package command

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/privatefile"
	"github.com/ViceMe-AI/cli/internal/privatepath"
)

// Buyer intent identity includes the verified account, profile, and API scope.
// Requests are saved before sending so a lost response replays the same IDs.
type buyerPurchaseIntent struct {
	QuoteRequest json.RawMessage `json:"quoteRequest,omitempty"`
	OrderRequest json.RawMessage `json:"orderRequest,omitempty"`
	OrderNo      string          `json:"orderNo,omitempty"`
}

func (runtime *Runtime) requireBuyerAuthentication(ctx context.Context) error {
	client := runtime.client()
	token, err := client.Tokens.Token(ctx)
	if err != nil {
		return err
	}
	// Pin this verified credential throughout payment polling and installation.
	client.Tokens = processTokenSource(token)
	status, err := client.AuthStatus(ctx)
	if err != nil {
		return err
	}
	granted := map[string]bool{}
	for _, scope := range status.Scopes {
		granted[scope] = true
	}
	if !granted["buyer-commerce:read"] || !granted["buyer-commerce:write"] {
		return output.Authorization("BUYER_PURCHASE_SCOPE_REQUIRED", "the current login has not authorized purchases").WithHint("run 'viceme auth login' once to authorize purchases, then retry the same command")
	}
	if status.User.ID == "" {
		return output.Authentication("BUYER_IDENTITY_REQUIRED", "the authenticated buyer identity is missing")
	}
	runtime.buyerUserID = status.User.ID
	runtime.buyerClient = client
	return nil
}

func buyerPurchaseIntentPath(runtime *Runtime, kind, target string) string {
	digest := sha256.Sum256([]byte(joinStateParts([]string{runtime.profile.ID, runtime.credentialScope, runtime.buyerUserID, kind, target})))
	return filepath.Join(runtime.configBase, "buyer-purchases", hex.EncodeToString(digest[:])+".json")
}

func lockBuyerPurchase(ctx context.Context, runtime *Runtime, kind, target string) (func(), error) {
	if runtime.buyerUserID == "" {
		return nil, output.Authentication("BUYER_IDENTITY_REQUIRED", "verify the buyer login before purchasing")
	}
	return runtime.lockCommerceIntent(ctx, "buyer-"+kind, runtime.buyerUserID, target, nil)
}

func loadBuyerPurchaseIntent(runtime *Runtime, kind, target string) (*buyerPurchaseIntent, error) {
	raw, err := os.ReadFile(buyerPurchaseIntentPath(runtime, kind, target))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, output.Internal("BUYER_PURCHASE_INTENT_READ_FAILED", "could not read the purchase intent", err)
	}
	var intent buyerPurchaseIntent
	if json.Unmarshal(raw, &intent) != nil || (len(intent.QuoteRequest) == 0 && len(intent.OrderRequest) == 0) {
		return nil, output.Internal("BUYER_PURCHASE_INTENT_INVALID", "the purchase intent is invalid", nil)
	}
	return &intent, nil
}

func saveBuyerPurchaseIntent(runtime *Runtime, kind, target string, intent buyerPurchaseIntent) error {
	filename := buyerPurchaseIntentPath(runtime, kind, target)
	if _, err := privatepath.EnsureDirectory(filepath.Dir(filename)); err != nil {
		return output.Internal("BUYER_PURCHASE_INTENT_WRITE_FAILED", "could not create the purchase intent directory", err)
	}
	encoded, err := json.Marshal(intent)
	if err != nil {
		return output.Internal("BUYER_PURCHASE_INTENT_WRITE_FAILED", "could not encode the purchase intent", err)
	}
	if err := privatefile.Write(filename, encoded, ".buyer-intent-*"); err != nil {
		return output.Internal("BUYER_PURCHASE_INTENT_WRITE_FAILED", "could not persist the purchase intent before sending the request", err)
	}
	return nil
}

func completeBuyerPurchase(ctx context.Context, runtime *Runtime, kind, target, orderNo string) error {
	unlock, err := lockBuyerPurchase(ctx, runtime, kind, target)
	if err != nil {
		return err
	}
	defer unlock()
	intent, err := loadBuyerPurchaseIntent(runtime, kind, target)
	if err != nil {
		return err
	}
	// A late waiter cannot erase a newer purchase or subscription renewal.
	if intent != nil && intent.OrderNo == orderNo {
		return os.Remove(buyerPurchaseIntentPath(runtime, kind, target))
	}
	return nil
}
