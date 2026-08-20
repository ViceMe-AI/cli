package command

import (
	"fmt"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

func newCreatorAppCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "creator-app", Short: "Manage Creator Apps for widget embedding"}
	command.AddCommand(newCreatorAppCreateCommand(runtime))
	command.AddCommand(newCreatorAppListCommand(runtime))
	command.AddCommand(newCreatorAppShowCommand(runtime))
	command.AddCommand(newCreatorAppDomainCommand(runtime))
	return command
}

func newCreatorAppCreateCommand(runtime *Runtime) *cobra.Command {
	var name string
	command := &cobra.Command{
		Use:   "create",
		Short: "Create an external Creator App",
		RunE: func(command *cobra.Command, args []string) error {
			if strings.TrimSpace(name) == "" {
				return output.Validation("CREATOR_APP_NAME_REQUIRED", "--name is required")
			}
			app, err := runtime.client().CreateCreatorApp(command.Context(), api.CreateCreatorAppRequest{Name: strings.TrimSpace(name)})
			if err != nil {
				return err
			}
			return runtime.success(map[string]any{"app": creatorAppJSON(app)})
		},
	}
	command.Flags().StringVar(&name, "name", "", "Creator App display name")
	return command
}

func newCreatorAppListCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List Creator Apps owned by the authenticated account",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			response, err := runtime.client().ListCreatorApps(command.Context())
			if err != nil {
				return err
			}
			apps := make([]any, 0, len(response.Items))
			for _, app := range response.Items {
				apps = append(apps, creatorAppJSON(app))
			}
			return runtime.success(map[string]any{"apps": apps})
		},
	}
}

func newCreatorAppShowCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "show <app-id>",
		Short: "Show a Creator App with its embed snippet",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			response, err := runtime.client().ListCreatorApps(command.Context())
			if err != nil {
				return err
			}
			for _, app := range response.Items {
				if app.ID == args[0] {
					webBase := strings.TrimRight(runtime.profile.ResolvedWebBaseURL(), "/")
					if webBase == "" {
						return output.Validation("PROFILE_WEB_BASE_URL_REQUIRED", "the selected Profile has no Web address; configure it with `viceme profile add --web-base-url`")
					}
					return runtime.success(map[string]any{
						"app":          creatorAppJSON(app),
						"embedSnippet": buildTipEmbedSnippet(webBase, app.ID, "zh-CN"),
					})
				}
			}
			return output.Validation("CREATOR_APP_NOT_FOUND", fmt.Sprintf("Creator App %q was not found in the authenticated account", args[0]))
		},
	}
}

func newCreatorAppDomainCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "domain", Short: "Manage Creator App domains"}
	command.AddCommand(newCreatorAppDomainAddCommand(runtime))
	command.AddCommand(newCreatorAppDomainVerifyCommand(runtime))
	return command
}

func newCreatorAppDomainAddCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "add <app-id> <domain>",
		Short: "Register a domain and return its verification token",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			app, err := runtime.client().AddCreatorAppDomain(command.Context(), args[0], args[1])
			if err != nil {
				return err
			}
			return runtime.success(map[string]any{"app": creatorAppJSON(app)})
		},
	}
}

func newCreatorAppDomainVerifyCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "verify <app-id> <domain>",
		Short: "Trigger domain ownership verification",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			app, err := runtime.client().VerifyCreatorAppDomain(command.Context(), args[0], args[1])
			if err != nil {
				return err
			}
			return runtime.success(map[string]any{"app": creatorAppJSON(app)})
		},
	}
}

func creatorAppJSON(app api.CreatorApp) map[string]any {
	domains := make([]any, 0, len(app.Domains))
	for _, domain := range app.Domains {
		entry := map[string]any{
			"domain":   domain.Domain,
			"verified": domain.Verified,
		}
		if domain.VerificationToken != nil {
			entry["verificationToken"] = *domain.VerificationToken
			entry["verificationPath"] = "/.well-known/viceme-app-verification.txt"
		}
		domains = append(domains, entry)
	}
	return map[string]any{
		"id":        app.ID,
		"kind":      app.Kind,
		"name":      app.Name,
		"domains":   domains,
		"createdAt": app.CreatedAt,
	}
}

func buildTipEmbedSnippet(webBaseURL, appID, locale string) string {
	return fmt.Sprintf(
		`<script async src="%s/widget/tip-embed.js" data-creator-app-id="%s" data-locale="%s"></script>`,
		webBaseURL, appID, locale,
	)
}
