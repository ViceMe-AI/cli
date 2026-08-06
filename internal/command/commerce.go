package command

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf16"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/appmanifest"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

var commerceUUIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func newCommerceCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "commerce", Short: "Configure ViceMe Commerce for a Creator App"}
	offer := &cobra.Command{Use: "offer", Short: "Manage fixed-price Commerce Offers"}
	offer.AddCommand(newCommerceOfferCreateCommand(runtime))
	offer.AddCommand(newCommerceOfferListCommand(runtime))
	command.AddCommand(offer)
	ledger := &cobra.Command{Use: "ledger", Short: "Inspect the Creator App Commerce ledger"}
	ledger.AddCommand(newCommerceLedgerListCommand(runtime))
	command.AddCommand(ledger)
	return command
}

func newCommerceOfferCreateCommand(runtime *Runtime) *cobra.Command {
	var directory string
	var appID string
	var clientRequestID string
	var name string
	var amountMinor int
	var currency string
	var purpose string
	command := &cobra.Command{
		Use:   "create",
		Short: "Create one fixed-price Offer in the linked App environment",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			_, manifest, err := loadCommerceBinding(directory, appID)
			if err != nil {
				return err
			}
			clientRequestID = strings.ToLower(strings.TrimSpace(clientRequestID))
			if !commerceUUIDPattern.MatchString(clientRequestID) {
				return output.Validation("commerce_request_id", "--client-request-id must be a UUID and must be reused when retrying the same Offer creation")
			}
			name = strings.TrimSpace(name)
			if name == "" {
				return output.Validation("commerce_offer_name", "--name is required")
			}
			if len(utf16.Encode([]rune(name))) > 100 {
				return output.Validation("commerce_offer_name", "--name must contain at most 100 UTF-16 code units")
			}
			if amountMinor < 1 || amountMinor > 100_000_000 {
				return output.Validation("commerce_offer_amount", "--amount-minor must be between 1 and 100000000")
			}
			currency = strings.ToUpper(strings.TrimSpace(currency))
			if currency != "CNY" && currency != "USD" {
				return output.Validation("commerce_offer_currency", "--currency must be CNY or USD")
			}
			if manifest.Environment == "LIVE" && currency != "CNY" {
				return output.Policy("commerce_live_currency", "Commerce LIVE supports CNY only")
			}
			purpose = strings.ToUpper(strings.TrimSpace(purpose))
			if purpose != "TIP" && purpose != "UNLOCK" {
				return output.Validation("commerce_offer_purpose", "--purpose must be TIP or UNLOCK")
			}
			offer, err := runtime.client().CreateCommerceOffer(command.Context(), manifest.AppID, manifest.Environment, api.CreateCommerceOfferRequest{
				ClientRequestID: clientRequestID,
				Name:            name,
				AmountMinor:     amountMinor,
				Currency:        currency,
				Purpose:         purpose,
			})
			if err != nil {
				return err
			}
			binding := manifest.Capabilities["commerce"]
			packageSpec := binding.SDKPackage + "@" + binding.SDKVersion
			return runtime.business(map[string]any{
				"offer": offer,
				"widget": map[string]string{
					"attribute":       "data-viceme-checkout",
					"offer_id":        offer.ID,
					"publishable_key": manifest.PublishableKey,
					"api_base_url":    strings.TrimRight(runtime.apiBaseURL, "/") + "/v1",
					"sdk_package":     binding.SDKPackage,
					"sdk_version":     binding.SDKVersion,
					"package_spec":    packageSpec,
					"cdn_url":         fmt.Sprintf("https://cdn.jsdelivr.net/npm/%s", packageSpec),
				},
			})
		},
	}
	addBindingFlags(command, &directory, &appID)
	command.Flags().StringVar(&clientRequestID, "client-request-id", "", "UUID idempotency key; reuse it for every retry of this exact create operation")
	command.Flags().StringVar(&name, "name", "", "Offer name")
	command.Flags().IntVar(&amountMinor, "amount-minor", 0, "fixed amount in the currency's minor unit")
	command.Flags().StringVar(&currency, "currency", "CNY", "currency: CNY or USD")
	command.Flags().StringVar(&purpose, "purpose", "TIP", "Offer purpose: TIP or UNLOCK")
	return command
}

func newCommerceOfferListCommand(runtime *Runtime) *cobra.Command {
	var directory string
	var appID string
	command := &cobra.Command{
		Use:   "list",
		Short: "List Offers in the linked App environment",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			_, manifest, err := loadCommerceBinding(directory, appID)
			if err != nil {
				return err
			}
			offers, err := runtime.client().ListCommerceOffers(command.Context(), manifest.AppID, manifest.Environment)
			if err != nil {
				return err
			}
			return runtime.business(offers)
		},
	}
	addBindingFlags(command, &directory, &appID)
	return command
}

func newCommerceLedgerListCommand(runtime *Runtime) *cobra.Command {
	var directory string
	var appID string
	var cursor string
	var limit int
	command := &cobra.Command{
		Use:   "list",
		Short: "List immutable payable, refund, payout, and adjustment entries",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			_, manifest, err := loadCommerceBinding(directory, appID)
			if err != nil {
				return err
			}
			if limit < 1 || limit > 100 {
				return output.Validation("commerce_ledger_limit", "--limit must be between 1 and 100")
			}
			ledger, err := runtime.client().ListCreatorLedger(command.Context(), manifest.AppID, strings.TrimSpace(cursor), limit)
			if err != nil {
				return err
			}
			return runtime.business(ledger)
		},
	}
	addBindingFlags(command, &directory, &appID)
	command.Flags().StringVar(&cursor, "cursor", "", "opaque cursor from the previous response")
	command.Flags().IntVar(&limit, "limit", 50, "maximum entries to return (1-100)")
	return command
}

func loadCommerceBinding(directory, expectedAppID string) (string, appmanifest.Manifest, error) {
	projectDirectory, manifest, err := loadAppBinding(directory, expectedAppID)
	if err != nil {
		return "", appmanifest.Manifest{}, err
	}
	if _, ok := manifest.Capabilities["commerce"]; !ok {
		return "", appmanifest.Manifest{}, output.Validation("commerce_capability_missing", "Commerce is not enabled; run 'viceme capability add commerce' first")
	}
	return projectDirectory, manifest, nil
}
