package command

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/payment"
	"github.com/ViceMe-AI/cli/internal/paymentconfig"
	"github.com/spf13/cobra"
)

const maxPaymentInputBytes = 1 << 20

type paymentContextResponse struct {
	Environment struct {
		ID            string `json:"id"`
		ApplicationID string `json:"applicationId"`
		Mode          string `json:"mode"`
		MarketRegion  string `json:"marketRegion"`
		Status        string `json:"status"`
	} `json:"environment"`
	Installation struct {
		ID            string `json:"id"`
		EnvironmentID string `json:"environmentId"`
		Capability    string `json:"capability"`
		Version       string `json:"version"`
		Status        string `json:"status"`
	} `json:"installation"`
}

type paymentCredentialSummary struct {
	ID                    string   `json:"id"`
	EnvironmentID         string   `json:"environmentId"`
	Name                  string   `json:"name"`
	Prefix                string   `json:"prefix"`
	Scopes                []string `json:"scopes"`
	ExpiresAt             *string  `json:"expiresAt"`
	LastUsedAt            *string  `json:"lastUsedAt"`
	RevokedAt             *string  `json:"revokedAt"`
	RotationOverlapEndsAt *string  `json:"rotationOverlapEndsAt"`
	CreatedAt             string   `json:"createdAt"`
}

type issuePaymentAPIKeyResponse struct {
	Credential paymentCredentialSummary `json:"credential"`
	APIKey     string                   `json:"apiKey"`
}

type rotatePaymentAPIKeyResponse struct {
	Credential paymentCredentialSummary `json:"credential"`
	APIKey     string                   `json:"apiKey"`
	Rotation   struct {
		ID            string `json:"id"`
		OverlapEndsAt string `json:"overlapEndsAt"`
	} `json:"rotation"`
}

func newPaymentCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "payment", Short: "Configure and test the ViceMe Payment capability"}
	command.AddCommand(newPaymentEligibilityCommand(runtime))
	command.AddCommand(newPaymentInitCommand(runtime))
	command.AddCommand(newPaymentContextCommand(runtime))
	command.AddCommand(newPaymentEnvironmentCommand(runtime))
	command.AddCommand(newPaymentProductCommand(runtime))
	command.AddCommand(newPaymentPriceCommand(runtime))
	command.AddCommand(newPaymentPriceVersionCommand(runtime))
	command.AddCommand(newPaymentTemplateCommand(runtime))
	command.AddCommand(newPaymentOriginCommand(runtime))
	command.AddCommand(newPaymentReturnTargetCommand(runtime))
	command.AddCommand(newPaymentWebhookCommand(runtime))
	command.AddCommand(newPaymentAPIKeyCommand(runtime))
	command.AddCommand(newPaymentCheckoutCommand(runtime))
	command.AddCommand(newPaymentOrderCommand(runtime))
	return command
}

func newPaymentEnvironmentCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "environment", Short: "Select an existing Payment environment"}
	command.AddCommand(newPaymentEnvironmentUseCommand(runtime))
	return command
}

func newPaymentEnvironmentUseCommand(runtime *Runtime) *cobra.Command {
	var root string
	command := &cobra.Command{
		Use: "use <sandbox|live>", Short: "Switch this project to an existing Payment environment", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			mode := strings.ToLower(strings.TrimSpace(args[0]))
			if mode != "sandbox" && mode != "live" {
				return output.Validation("PAYMENT_ENVIRONMENT_INVALID", "Payment environment must be sandbox or live")
			}
			configured, _, err := loadPaymentConfig(root)
			if err != nil {
				return err
			}
			candidate := configured
			candidate.Environment = mode
			remote, err := fetchPaymentContext(command, runtime, candidate)
			if err != nil {
				return err
			}
			candidate.EnvironmentID = remote.Environment.ID
			candidate.InstallationID = remote.Installation.ID
			candidate.MarketRegion = remote.Environment.MarketRegion
			candidate.PaymentAPIKeyID = ""
			configPath, err := paymentconfig.Save(root, candidate)
			if err != nil {
				return output.Internal("PAYMENT_PROJECT_SAVE_FAILED", "the remote Payment environment exists but local context could not be updated", err)
			}
			return runtime.business(map[string]any{"configPath": configPath, "context": candidate, "remote": remote})
		},
	}
	command.Flags().StringVar(&root, "dir", ".", "project directory")
	return command
}

func newPaymentOriginCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "origin", Short: "Manage verified application origins"}
	command.AddCommand(newPaymentControlListCommand(runtime, "list", "List application origins", func(configured paymentconfig.Config) string {
		return "/v1/capability-applications/" + url.PathEscape(configured.ApplicationID) + "/origins"
	}))
	command.AddCommand(newPaymentJSONCreateCommand(runtime, "create", "Register an application origin", func(configured paymentconfig.Config, _ string) string {
		return "/v1/capability-applications/" + url.PathEscape(configured.ApplicationID) + "/origins"
	}))
	command.AddCommand(newPaymentControlJSONActionCommand(runtime, "verify <origin-id>", "Verify an application origin", http.MethodPost, "/v1/capability-application-origins/", "/verify"))
	return command
}

func newPaymentReturnTargetCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "return-target", Short: "Manage pre-registered hosted checkout return targets"}
	command.AddCommand(newPaymentControlListCommand(runtime, "list", "List checkout return targets", func(configured paymentconfig.Config) string {
		return "/v1/capability-environments/" + url.PathEscape(configured.EnvironmentID) + "/checkout-return-targets"
	}))
	command.AddCommand(newPaymentJSONCreateCommand(runtime, "create", "Create a checkout return target", func(configured paymentconfig.Config, _ string) string {
		return "/v1/capability-environments/" + url.PathEscape(configured.EnvironmentID) + "/checkout-return-targets"
	}))
	command.AddCommand(newPaymentControlJSONActionCommand(runtime, "update <return-target-id>", "Update a checkout return target", http.MethodPatch, "/v1/capability-checkout-return-targets/", ""))
	return command
}

type webhookSecretResponse struct {
	Endpoint struct {
		ID            string `json:"id"`
		EnvironmentID string `json:"environmentId"`
		URL           string `json:"url"`
		Status        string `json:"status"`
	} `json:"endpoint"`
	SigningKey struct {
		ID    string `json:"id"`
		KeyID string `json:"keyId"`
	} `json:"signingKey"`
	SigningSecret string `json:"signingSecret"`
}

type webhookRotationResponse struct {
	SigningKey struct {
		ID    string `json:"id"`
		KeyID string `json:"keyId"`
	} `json:"signingKey"`
	SigningSecret string `json:"signingSecret"`
	Rotation      struct {
		ID            string `json:"id"`
		OverlapEndsAt string `json:"overlapEndsAt"`
	} `json:"rotation"`
}

func newPaymentWebhookCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "webhook", Short: "Manage signed Payment webhooks"}
	command.AddCommand(newPaymentWebhookCreateCommand(runtime))
	command.AddCommand(newPaymentWebhookDeliverCommand(runtime))
	command.AddCommand(newPaymentControlListCommand(runtime, "list", "List Payment webhook endpoints", func(configured paymentconfig.Config) string {
		return "/v1/capability-environments/" + url.PathEscape(configured.EnvironmentID) + "/webhooks"
	}))
	command.AddCommand(newPaymentControlJSONActionCommand(runtime, "update <webhook-endpoint-id>", "Update a Payment webhook endpoint", http.MethodPatch, "/v1/capability-webhooks/", ""))
	command.AddCommand(newPaymentControlActionCommand(runtime, "verify <webhook-endpoint-id>", "Run the webhook endpoint verification challenge", "/v1/capability-webhooks/", "/verify", false))
	command.AddCommand(newPaymentWebhookRevokeCommand(runtime))
	command.AddCommand(newPaymentWebhookRotateCommand(runtime))
	command.AddCommand(newPaymentControlActionCommand(runtime, "abort-rotation <rotation-id>", "Abort an active webhook signing-secret rotation", "/v1/capability-webhook-signing-secret-rotations/", "/abort", true))
	return command
}

func newPaymentWebhookDeliverCommand(runtime *Runtime) *cobra.Command {
	var root, envFile, variable string
	command := &cobra.Command{
		Use: "deliver <webhook-endpoint-id>", Short: "Safely deliver a stored Webhook signing secret to a project dotenv file", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			configured, _, err := loadPaymentConfig(root)
			if err != nil {
				return err
			}
			credential, err := runtime.paymentManager().LoadWebhook(args[0], configured.EnvironmentID)
			if err != nil {
				return err
			}
			delivery, err := payment.DeliverWebhookSigningSecretToEnvFile(root, envFile, variable, credential.SigningSecret)
			if err != nil {
				return err
			}
			return runtime.business(map[string]any{
				"endpointId":    credential.EndpointID,
				"signingKeyId":  credential.SigningKeyID,
				"environmentId": configured.EnvironmentID,
				"environment":   configured.Environment,
				"delivery":      delivery,
				"delivered":     true,
			})
		},
	}
	command.Flags().StringVar(&root, "dir", ".", "project directory")
	command.Flags().StringVar(&envFile, "env-file", ".env.local", "project-relative dotenv file")
	command.Flags().StringVar(&variable, "env-var", "VICEME_PAYMENT_WEBHOOK_SECRET", "server-only environment variable name")
	return command
}

func newPaymentWebhookCreateCommand(runtime *Runtime) *cobra.Command {
	var root, input string
	command := &cobra.Command{
		Use: "create", Short: "Create a webhook endpoint and securely store its signing secret", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			configured, _, err := loadPaymentConfig(root)
			if err != nil {
				return err
			}
			body, err := readPaymentJSON(input)
			if err != nil {
				return err
			}
			secrets := runtime.paymentManager()
			if err := secrets.PreflightWebhook(configured.EnvironmentID); err != nil {
				return err
			}
			var issued webhookSecretResponse
			endpoint := "/v1/capability-environments/" + url.PathEscape(configured.EnvironmentID) + "/webhooks"
			if err := runtime.client().PaymentControl(command.Context(), http.MethodPost, endpoint, body, &issued); err != nil {
				return err
			}
			if err := secrets.SaveWebhook(payment.WebhookCredential{SigningSecret: issued.SigningSecret, SigningKeyID: issued.SigningKey.KeyID, EndpointID: issued.Endpoint.ID, EnvironmentID: configured.EnvironmentID}); err != nil {
				_ = revokeWebhook(command, runtime, issued.Endpoint.ID)
				return err
			}
			return runtime.business(map[string]any{"endpoint": issued.Endpoint, "signingKey": issued.SigningKey, "stored": true})
		},
	}
	command.Flags().StringVar(&root, "dir", ".", "project directory")
	command.Flags().StringVar(&input, "input", "", "strict JSON webhook request file")
	_ = command.MarkFlagRequired("input")
	return command
}

func newPaymentWebhookRotateCommand(runtime *Runtime) *cobra.Command {
	var root string
	command := &cobra.Command{
		Use: "rotate-secret <webhook-endpoint-id>", Short: "Rotate and securely store a webhook signing secret", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			configured, _, err := loadPaymentConfig(root)
			if err != nil {
				return err
			}
			secrets := runtime.paymentManager()
			if err := secrets.PreflightWebhook(configured.EnvironmentID); err != nil {
				return err
			}
			var rotated webhookRotationResponse
			endpoint := "/v1/capability-webhooks/" + url.PathEscape(args[0]) + "/signing-secret/rotate"
			if err := runtime.client().PaymentControl(command.Context(), http.MethodPost, endpoint, struct{}{}, &rotated); err != nil {
				return err
			}
			if err := secrets.SaveWebhook(payment.WebhookCredential{SigningSecret: rotated.SigningSecret, SigningKeyID: rotated.SigningKey.KeyID, EndpointID: args[0], EnvironmentID: configured.EnvironmentID}); err != nil {
				_ = abortWebhookRotation(command, runtime, rotated.Rotation.ID)
				return err
			}
			return runtime.business(map[string]any{"signingKey": rotated.SigningKey, "rotation": rotated.Rotation, "stored": true})
		},
	}
	command.Flags().StringVar(&root, "dir", ".", "project directory")
	return command
}

func newPaymentWebhookRevokeCommand(runtime *Runtime) *cobra.Command {
	var root string
	var yes bool
	command := &cobra.Command{
		Use: "revoke <webhook-endpoint-id>", Short: "Revoke a Payment webhook and remove its local signing secret", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if !yes {
				return output.Confirmation("CONFIRMATION_REQUIRED", "webhook revocation requires --yes")
			}
			if _, _, err := loadPaymentConfig(root); err != nil {
				return err
			}
			var result map[string]any
			endpoint := "/v1/capability-webhooks/" + url.PathEscape(args[0]) + "/revoke"
			if err := runtime.client().PaymentControl(command.Context(), http.MethodPost, endpoint, struct{}{}, &result); err != nil {
				return err
			}
			if err := runtime.paymentManager().DeleteWebhook(args[0]); err != nil {
				return err
			}
			return runtime.business(map[string]any{"endpoint": result, "stored": false})
		},
	}
	command.Flags().StringVar(&root, "dir", ".", "project directory")
	command.Flags().BoolVar(&yes, "yes", false, "confirm webhook revocation")
	return command
}

func newPaymentControlListCommand(runtime *Runtime, use, short string, endpoint func(paymentconfig.Config) string) *cobra.Command {
	var root string
	command := &cobra.Command{
		Use: use, Short: short, Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			configured, _, err := loadPaymentConfig(root)
			if err != nil {
				return err
			}
			var result []map[string]any
			if err := runtime.client().PaymentControl(command.Context(), http.MethodGet, endpoint(configured), nil, &result); err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&root, "dir", ".", "project directory")
	return command
}

func newPaymentControlJSONActionCommand(runtime *Runtime, use, short, method, prefix, suffix string) *cobra.Command {
	var root, input string
	command := &cobra.Command{
		Use: use, Short: short, Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if _, _, err := loadPaymentConfig(root); err != nil {
				return err
			}
			body, err := readPaymentJSON(input)
			if err != nil {
				return err
			}
			var result map[string]any
			if err := runtime.client().PaymentControl(command.Context(), method, prefix+url.PathEscape(args[0])+suffix, body, &result); err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&root, "dir", ".", "project directory")
	command.Flags().StringVar(&input, "input", "", "strict JSON request file")
	_ = command.MarkFlagRequired("input")
	return command
}

func revokeWebhook(command *cobra.Command, runtime *Runtime, endpointID string) error {
	var ignored map[string]any
	endpoint := "/v1/capability-webhooks/" + url.PathEscape(endpointID) + "/revoke"
	return runtime.client().PaymentControl(command.Context(), http.MethodPost, endpoint, struct{}{}, &ignored)
}

func abortWebhookRotation(command *cobra.Command, runtime *Runtime, rotationID string) error {
	var ignored map[string]any
	endpoint := "/v1/capability-webhook-signing-secret-rotations/" + url.PathEscape(rotationID) + "/abort"
	return runtime.client().PaymentControl(command.Context(), http.MethodPost, endpoint, struct{}{}, &ignored)
}

func newPaymentEligibilityCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use: "eligibility", Short: "Check whether the active user can issue Payment API Keys", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			var result map[string]any
			if err := runtime.client().PaymentControl(command.Context(), http.MethodGet, "/v1/me/payment-api-key-eligibility", nil, &result); err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
}

func newPaymentInitCommand(runtime *Runtime) *cobra.Command {
	var root, slug, name string
	command := &cobra.Command{
		Use: "init", Short: "Create a Payment application and its SANDBOX environment", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if strings.TrimSpace(slug) == "" || strings.TrimSpace(name) == "" {
				return output.Validation("PAYMENT_PROJECT_INVALID", "--slug and --name are required")
			}
			filename, err := paymentconfig.Path(root)
			if err != nil {
				return output.Validation("PAYMENT_PROJECT_INVALID", err.Error())
			}
			if _, err := os.Stat(filename); err == nil {
				return output.Validation("PAYMENT_PROJECT_EXISTS", "this directory already has .viceme/payment.yaml").WithHint("run 'viceme payment context --dir <path>' to inspect it")
			} else if !os.IsNotExist(err) {
				return output.Internal("PAYMENT_PROJECT_READ_FAILED", "could not inspect the Payment project config", err)
			}
			var space struct {
				ID string `json:"id"`
			}
			client := runtime.client()
			if err := client.PaymentControl(command.Context(), http.MethodPost, "/v1/me/capability-space", struct{}{}, &space); err != nil {
				return err
			}
			var application struct {
				ID   string `json:"id"`
				Slug string `json:"slug"`
			}
			if err := client.PaymentControl(command.Context(), http.MethodPost, "/v1/capability-spaces/"+url.PathEscape(space.ID)+"/applications", map[string]string{"slug": slug, "name": name}, &application); err != nil {
				return err
			}
			var installed paymentContextResponse
			endpoint := "/v1/capability-applications/" + url.PathEscape(application.ID) + "/environments/sandbox/installations"
			if err := client.PaymentControl(command.Context(), http.MethodPost, endpoint, map[string]string{"capability": "payment", "version": "v1"}, &installed); err != nil {
				return err
			}
			configured := paymentconfig.Config{
				SchemaVersion: 1, CapabilitySpace: space.ID, ApplicationID: application.ID,
				ApplicationSlug: application.Slug, Environment: "sandbox", MarketRegion: installed.Environment.MarketRegion,
				EnvironmentID: installed.Environment.ID, InstallationID: installed.Installation.ID,
			}
			configPath, err := paymentconfig.Save(root, configured)
			if err != nil {
				return output.Internal("PAYMENT_PROJECT_SAVE_FAILED", "the remote Payment application was created but local context could not be saved", err).
					WithDetails(map[string]any{"applicationId": application.ID, "environmentId": installed.Environment.ID, "installationId": installed.Installation.ID})
			}
			return runtime.business(map[string]any{"configPath": configPath, "context": configured})
		},
	}
	command.Flags().StringVar(&root, "dir", ".", "project directory")
	command.Flags().StringVar(&slug, "slug", "", "application slug")
	command.Flags().StringVar(&name, "name", "", "application display name")
	return command
}

func newPaymentContextCommand(runtime *Runtime) *cobra.Command {
	var root string
	command := &cobra.Command{
		Use: "context", Short: "Verify local Payment context against ViceMe", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			configured, filename, err := loadPaymentConfig(root)
			if err != nil {
				return err
			}
			remote, err := fetchPaymentContext(command, runtime, configured)
			if err != nil {
				return err
			}
			if remote.Environment.ID != configured.EnvironmentID || remote.Installation.ID != configured.InstallationID {
				return output.Policy("PAYMENT_CONTEXT_MISMATCH", "local Payment context does not match the authenticated remote application")
			}
			return runtime.business(map[string]any{"configPath": filename, "context": configured, "remote": remote})
		},
	}
	command.Flags().StringVar(&root, "dir", ".", "project directory")
	return command
}

func newPaymentProductCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "product", Short: "Manage Payment products"}
	command.AddCommand(newPaymentJSONCreateCommand(runtime, "create", "Create a Payment product", func(configured paymentconfig.Config, _ string) string {
		return "/v1/payment/installations/" + url.PathEscape(configured.InstallationID) + "/products"
	}))
	return command
}

func newPaymentPriceCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "price", Short: "Manage Payment prices"}
	command.AddCommand(newPaymentResourceJSONCommand(runtime, "create <product-id>", "Create a Payment price", "product-id", func(id string) string {
		return "/v1/payment/products/" + url.PathEscape(id) + "/prices"
	}))
	return command
}

func newPaymentPriceVersionCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "price-version", Short: "Manage immutable Payment price versions"}
	command.AddCommand(newPaymentResourceJSONCommand(runtime, "create <price-id>", "Create a Payment price version", "price-id", func(id string) string {
		return "/v1/payment/prices/" + url.PathEscape(id) + "/versions"
	}))
	command.AddCommand(newPaymentControlActionCommand(runtime, "activate <price-version-id>", "Activate a Payment price version", "/v1/payment/price-versions/", "/activate", false))
	return command
}

func newPaymentTemplateCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "template", Short: "Manage optional custom hosted checkout templates"}
	command.AddCommand(newPaymentJSONCreateCommand(runtime, "create", "Create and publish an optional custom checkout template", func(configured paymentconfig.Config, _ string) string {
		return "/v1/payment/installations/" + url.PathEscape(configured.InstallationID) + "/templates"
	}))
	return command
}

func newPaymentJSONCreateCommand(runtime *Runtime, use, short string, endpoint func(paymentconfig.Config, string) string) *cobra.Command {
	var root, input string
	command := &cobra.Command{
		Use: use, Short: short, Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			configured, _, err := loadPaymentConfig(root)
			if err != nil {
				return err
			}
			body, err := readPaymentJSON(input)
			if err != nil {
				return err
			}
			var result map[string]any
			if err := runtime.client().PaymentControl(command.Context(), http.MethodPost, endpoint(configured, ""), body, &result); err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&root, "dir", ".", "project directory")
	command.Flags().StringVar(&input, "input", "", "strict JSON request file")
	_ = command.MarkFlagRequired("input")
	return command
}

func newPaymentResourceJSONCommand(runtime *Runtime, use, short, argumentName string, endpoint func(string) string) *cobra.Command {
	var root, input string
	command := &cobra.Command{
		Use: use, Short: short, Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if _, _, err := loadPaymentConfig(root); err != nil {
				return err
			}
			body, err := readPaymentJSON(input)
			if err != nil {
				return err
			}
			var result map[string]any
			if err := runtime.client().PaymentControl(command.Context(), http.MethodPost, endpoint(args[0]), body, &result); err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&root, "dir", ".", "project directory")
	command.Flags().StringVar(&input, "input", "", "strict JSON request file")
	_ = command.MarkFlagRequired("input")
	command.ValidArgsFunction = noFileCompletionFor(argumentName)
	return command
}

func newPaymentControlActionCommand(runtime *Runtime, use, short, prefix, suffix string, destructive bool) *cobra.Command {
	var root string
	var yes bool
	command := &cobra.Command{
		Use: use, Short: short, Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if _, _, err := loadPaymentConfig(root); err != nil {
				return err
			}
			if destructive && !yes {
				return output.Confirmation("CONFIRMATION_REQUIRED", "this Payment action requires --yes")
			}
			var result map[string]any
			if err := runtime.client().PaymentControl(command.Context(), http.MethodPost, prefix+url.PathEscape(args[0])+suffix, struct{}{}, &result); err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&root, "dir", ".", "project directory")
	if destructive {
		command.Flags().BoolVar(&yes, "yes", false, "confirm the destructive action")
	}
	return command
}

func newPaymentAPIKeyCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "api-key", Short: "Manage the environment Payment API Key in secure storage"}
	command.AddCommand(newPaymentAPIKeyCreateCommand(runtime))
	command.AddCommand(newPaymentAPIKeyDeliverCommand(runtime))
	command.AddCommand(newPaymentAPIKeyRotateCommand(runtime))
	command.AddCommand(newPaymentAPIKeyRevokeCommand(runtime))
	return command
}

func newPaymentAPIKeyCreateCommand(runtime *Runtime) *cobra.Command {
	var root, name, scopesText string
	command := &cobra.Command{
		Use: "create", Short: "Issue and securely store a Payment API Key for the selected environment", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			configured, _, err := loadPaymentConfig(root)
			if err != nil {
				return err
			}
			if configured.PaymentAPIKeyID != "" {
				return output.Policy("PAYMENT_API_KEY_ALREADY_CONFIGURED", "this project environment already references a Payment API Key").WithHint("use 'viceme payment api-key rotate' or revoke the existing key before creating another")
			}
			scopes, err := paymentScopes(scopesText)
			if err != nil {
				return err
			}
			secrets := runtime.paymentManager()
			if err := secrets.EnsureAPIKeyAbsent(configured.EnvironmentID); err != nil {
				return err
			}
			if err := secrets.PreflightAPIKey(configured.EnvironmentID); err != nil {
				return err
			}
			var issued issuePaymentAPIKeyResponse
			endpoint := "/v1/capability-environments/" + url.PathEscape(configured.EnvironmentID) + "/api-keys"
			if err := runtime.client().PaymentControl(command.Context(), http.MethodPost, endpoint, map[string]any{"name": name, "scopes": scopes}, &issued); err != nil {
				return err
			}
			if err := secrets.SaveAPIKey(payment.APIKeyCredential{APIKey: issued.APIKey, CredentialID: issued.Credential.ID, EnvironmentID: configured.EnvironmentID}); err != nil {
				_ = revokeIssuedPaymentAPIKey(command, runtime, issued.Credential.ID, "LOCAL_SECRET_PERSISTENCE_FAILED")
				return err
			}
			configured.PaymentAPIKeyID = issued.Credential.ID
			configPath, err := paymentconfig.Save(root, configured)
			if err != nil {
				_ = revokeIssuedPaymentAPIKey(command, runtime, issued.Credential.ID, "LOCAL_CONTEXT_PERSISTENCE_FAILED")
				_ = secrets.DeleteAPIKey(configured.EnvironmentID)
				return output.Internal("PAYMENT_PROJECT_SAVE_FAILED", "the issued Payment API Key was revoked because local context could not be saved", err)
			}
			return runtime.business(map[string]any{"credential": issued.Credential, "stored": true, "configPath": configPath})
		},
	}
	command.Flags().StringVar(&root, "dir", ".", "project directory")
	command.Flags().StringVar(&name, "name", "payment-backend", "Payment API Key label")
	command.Flags().StringVar(&scopesText, "scopes", "payment:products:read,payment:checkouts:create,payment:orders:read,payment:orders:close", "comma-separated runtime scopes")
	return command
}

func newPaymentAPIKeyRotateCommand(runtime *Runtime) *cobra.Command {
	var root string
	command := &cobra.Command{
		Use: "rotate", Short: "Rotate the stored Payment API Key with a 24-hour overlap", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			configured, _, err := loadPaymentConfig(root)
			if err != nil {
				return err
			}
			secrets := runtime.paymentManager()
			previous, err := secrets.LoadAPIKey(configured.EnvironmentID)
			if err != nil {
				return err
			}
			var rotated rotatePaymentAPIKeyResponse
			endpoint := "/v1/capability-api-keys/" + url.PathEscape(previous.CredentialID) + "/rotate"
			if err := runtime.client().PaymentControl(command.Context(), http.MethodPost, endpoint, struct{}{}, &rotated); err != nil {
				return err
			}
			if err := secrets.SaveAPIKey(payment.APIKeyCredential{APIKey: rotated.APIKey, CredentialID: rotated.Credential.ID, EnvironmentID: configured.EnvironmentID}); err != nil {
				_ = abortPaymentAPIKeyRotation(command, runtime, rotated.Rotation.ID)
				return err
			}
			configured.PaymentAPIKeyID = rotated.Credential.ID
			configPath, err := paymentconfig.Save(root, configured)
			if err != nil {
				_ = abortPaymentAPIKeyRotation(command, runtime, rotated.Rotation.ID)
				_ = secrets.SaveAPIKey(previous)
				return output.Internal("PAYMENT_PROJECT_SAVE_FAILED", "the Payment API Key rotation was aborted because local context could not be saved", err)
			}
			return runtime.business(map[string]any{"credential": rotated.Credential, "rotation": rotated.Rotation, "stored": true, "configPath": configPath})
		},
	}
	command.Flags().StringVar(&root, "dir", ".", "project directory")
	return command
}

func newPaymentAPIKeyDeliverCommand(runtime *Runtime) *cobra.Command {
	var root, envFile, variable string
	command := &cobra.Command{
		Use: "deliver", Short: "Safely deliver the stored Payment API Key to a project dotenv file", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			configured, _, err := loadPaymentConfig(root)
			if err != nil {
				return err
			}
			credential, err := runtime.paymentManager().LoadAPIKey(configured.EnvironmentID)
			if err != nil {
				return err
			}
			delivery, err := payment.DeliverAPIKeyToEnvFile(root, envFile, variable, credential.APIKey)
			if err != nil {
				return err
			}
			return runtime.business(map[string]any{
				"credentialId":  credential.CredentialID,
				"environmentId": credential.EnvironmentID,
				"environment":   configured.Environment,
				"delivery":      delivery,
				"delivered":     true,
			})
		},
	}
	command.Flags().StringVar(&root, "dir", ".", "project directory")
	command.Flags().StringVar(&envFile, "env-file", ".env.local", "project-relative dotenv file")
	command.Flags().StringVar(&variable, "env-var", "VICEME_PAYMENT_API_KEY", "server-only environment variable name")
	return command
}

func newPaymentAPIKeyRevokeCommand(runtime *Runtime) *cobra.Command {
	var root, reason string
	var yes bool
	command := &cobra.Command{
		Use: "revoke", Short: "Immediately revoke and remove the stored Payment API Key", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if !yes {
				return output.Confirmation("CONFIRMATION_REQUIRED", "Payment API Key revocation requires --yes")
			}
			configured, _, err := loadPaymentConfig(root)
			if err != nil {
				return err
			}
			credentialID := configured.PaymentAPIKeyID
			if credentialID == "" {
				credential, err := runtime.paymentManager().LoadAPIKey(configured.EnvironmentID)
				if err != nil {
					return err
				}
				credentialID = credential.CredentialID
			}
			var result map[string]any
			endpoint := "/v1/capability-api-keys/" + url.PathEscape(credentialID) + "/revoke"
			if err := runtime.client().PaymentControl(command.Context(), http.MethodPost, endpoint, map[string]string{"reason": reason}, &result); err != nil {
				return err
			}
			if err := runtime.paymentManager().DeleteAPIKey(configured.EnvironmentID); err != nil {
				return err
			}
			configured.PaymentAPIKeyID = ""
			if _, err := paymentconfig.Save(root, configured); err != nil {
				return output.Internal("PAYMENT_PROJECT_SAVE_FAILED", "the Payment API Key was revoked but local context could not be updated", err)
			}
			return runtime.business(map[string]any{"credential": result, "stored": false})
		},
	}
	command.Flags().StringVar(&root, "dir", ".", "project directory")
	command.Flags().StringVar(&reason, "reason", "USER_REQUEST", "revocation reason")
	command.Flags().BoolVar(&yes, "yes", false, "confirm immediate revocation")
	return command
}

func newPaymentCheckoutCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "checkout", Short: "Use the stored Payment API Key for checkout"}
	command.AddCommand(newPaymentCheckoutProductsCommand(runtime))
	command.AddCommand(newPaymentCheckoutCreateCommand(runtime))
	return command
}

func newPaymentCheckoutProductsCommand(runtime *Runtime) *cobra.Command {
	var root string
	command := &cobra.Command{
		Use: "products", Short: "List active runtime products", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			configured, _, credential, err := loadPaymentRuntime(runtime, root)
			if err != nil {
				return err
			}
			_ = configured
			var result []map[string]any
			if err := runtime.client().PaymentRuntime(command.Context(), http.MethodGet, "/v1/checkout/v1/products", nil, &result, credential.APIKey, ""); err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&root, "dir", ".", "project directory")
	return command
}

func newPaymentCheckoutCreateCommand(runtime *Runtime) *cobra.Command {
	var root, input, idempotencyKey string
	command := &cobra.Command{
		Use: "create", Short: "Create an idempotent checkout session", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			_, _, credential, err := loadPaymentRuntime(runtime, root)
			if err != nil {
				return err
			}
			body, err := readPaymentJSON(input)
			if err != nil {
				return err
			}
			if strings.TrimSpace(idempotencyKey) == "" {
				return output.Validation("IDEMPOTENCY_KEY_REQUIRED", "--idempotency-key is required")
			}
			var result map[string]any
			if err := runtime.client().PaymentRuntime(command.Context(), http.MethodPost, "/v1/checkout/v1/sessions", body, &result, credential.APIKey, idempotencyKey); err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&root, "dir", ".", "project directory")
	command.Flags().StringVar(&input, "input", "", "strict JSON checkout request file")
	command.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "stable retry identity")
	_ = command.MarkFlagRequired("input")
	_ = command.MarkFlagRequired("idempotency-key")
	return command
}

func newPaymentOrderCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "order", Short: "Inspect or close Payment orders"}
	command.AddCommand(newPaymentOrderGetCommand(runtime))
	command.AddCommand(newPaymentOrderCloseCommand(runtime))
	return command
}

func newPaymentOrderGetCommand(runtime *Runtime) *cobra.Command {
	var root string
	command := &cobra.Command{
		Use: "get <payment-no>", Short: "Get a Payment order", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			_, _, credential, err := loadPaymentRuntime(runtime, root)
			if err != nil {
				return err
			}
			var result map[string]any
			endpoint := "/v1/checkout/v1/orders/" + url.PathEscape(args[0])
			if err := runtime.client().PaymentRuntime(command.Context(), http.MethodGet, endpoint, nil, &result, credential.APIKey, ""); err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&root, "dir", ".", "project directory")
	return command
}

func newPaymentOrderCloseCommand(runtime *Runtime) *cobra.Command {
	var root string
	var yes bool
	command := &cobra.Command{
		Use: "close <payment-no>", Short: "Authoritatively close an open Payment order", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if !yes {
				return output.Confirmation("CONFIRMATION_REQUIRED", "Payment order close requires --yes")
			}
			_, _, credential, err := loadPaymentRuntime(runtime, root)
			if err != nil {
				return err
			}
			var result map[string]any
			endpoint := "/v1/checkout/v1/orders/" + url.PathEscape(args[0]) + "/close"
			if err := runtime.client().PaymentRuntime(command.Context(), http.MethodPost, endpoint, struct{}{}, &result, credential.APIKey, ""); err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&root, "dir", ".", "project directory")
	command.Flags().BoolVar(&yes, "yes", false, "confirm order close")
	return command
}

func loadPaymentConfig(root string) (paymentconfig.Config, string, error) {
	configured, filename, err := paymentconfig.Load(root)
	if err == nil {
		return configured, filename, nil
	}
	if os.IsNotExist(err) {
		return paymentconfig.Config{}, filename, output.Validation("PAYMENT_PROJECT_NOT_INITIALIZED", "no .viceme/payment.yaml exists in this directory").WithHint("run 'viceme payment init --dir <path> --slug <slug> --name <name>'")
	}
	return paymentconfig.Config{}, filename, output.Validation("PAYMENT_PROJECT_INVALID", err.Error())
}

func fetchPaymentContext(command *cobra.Command, runtime *Runtime, configured paymentconfig.Config) (paymentContextResponse, error) {
	var remote paymentContextResponse
	endpoint := "/v1/capability-applications/" + url.PathEscape(configured.ApplicationID) + "/environments/" + url.PathEscape(configured.Environment) + "/installations/payment"
	err := runtime.client().PaymentControl(command.Context(), http.MethodGet, endpoint, nil, &remote)
	return remote, err
}

func loadPaymentRuntime(runtime *Runtime, root string) (paymentconfig.Config, string, payment.APIKeyCredential, error) {
	configured, filename, err := loadPaymentConfig(root)
	if err != nil {
		return paymentconfig.Config{}, filename, payment.APIKeyCredential{}, err
	}
	credential, err := runtime.paymentManager().LoadAPIKey(configured.EnvironmentID)
	return configured, filename, credential, err
}

func (runtime *Runtime) paymentManager() payment.Manager {
	return payment.Manager{Store: runtime.deps.Store, ProfileID: runtime.profile.ID, Scope: runtime.credentialScope}
}

func readPaymentJSON(filename string) (any, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return nil, output.Validation("PAYMENT_INPUT_REQUIRED", "--input is required")
	}
	absolute, err := filepath.Abs(filename)
	if err != nil {
		return nil, output.Validation("PAYMENT_INPUT_INVALID", err.Error())
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, output.Validation("PAYMENT_INPUT_INVALID", err.Error())
	}
	if !info.Mode().IsRegular() || info.Size() > maxPaymentInputBytes {
		return nil, output.Validation("PAYMENT_INPUT_INVALID", "Payment input must be a regular JSON file no larger than 1 MiB")
	}
	data, err := os.ReadFile(absolute)
	if err != nil {
		return nil, output.Validation("PAYMENT_INPUT_INVALID", err.Error())
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, output.Validation("PAYMENT_INPUT_INVALID", "Payment input is not valid JSON")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, output.Validation("PAYMENT_INPUT_INVALID", "Payment input must contain exactly one JSON value")
	}
	if _, ok := value.(map[string]any); !ok {
		return nil, output.Validation("PAYMENT_INPUT_INVALID", "Payment input root must be a JSON object")
	}
	return value, nil
}

func paymentScopes(raw string) ([]string, error) {
	allowed := map[string]bool{
		"payment:products:read": true, "payment:checkouts:create": true,
		"payment:orders:read": true, "payment:orders:close": true,
		"payment:subscriptions:read": true, "payment:subscriptions:cancel": true,
	}
	seen := make(map[string]bool)
	var scopes []string
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if !allowed[item] {
			return nil, output.Validation("PAYMENT_SCOPE_INVALID", fmt.Sprintf("unsupported Payment API Key scope %q", item))
		}
		if !seen[item] {
			scopes = append(scopes, item)
			seen[item] = true
		}
	}
	if len(scopes) == 0 {
		return nil, output.Validation("PAYMENT_SCOPE_INVALID", "at least one Payment API Key scope is required")
	}
	return scopes, nil
}

func revokeIssuedPaymentAPIKey(command *cobra.Command, runtime *Runtime, credentialID, reason string) error {
	var ignored map[string]any
	endpoint := "/v1/capability-api-keys/" + url.PathEscape(credentialID) + "/revoke"
	return runtime.client().PaymentControl(command.Context(), http.MethodPost, endpoint, map[string]string{"reason": reason}, &ignored)
}

func abortPaymentAPIKeyRotation(command *cobra.Command, runtime *Runtime, rotationID string) error {
	var ignored map[string]any
	endpoint := "/v1/capability-api-key-rotations/" + url.PathEscape(rotationID) + "/abort"
	return runtime.client().PaymentControl(command.Context(), http.MethodPost, endpoint, struct{}{}, &ignored)
}

func noFileCompletionFor(string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}
