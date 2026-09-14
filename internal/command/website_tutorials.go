package command

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

func newMerchantWorkTutorialCommand(runtime *Runtime) *cobra.Command {
	group := &cobra.Command{Use: "tutorial", Short: "Publish versioned website creation tutorials"}
	var filename, merchant, requestID string
	var version, preview, price int
	command := &cobra.Command{Use: "publish <work-id>", Short: "Publish a tutorial JSON file as an immutable version", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		content, err := readStrictJSONObject[api.WebsiteTutorialContent](filename, "WEBSITE_TUTORIAL_INPUT_INVALID")
		if err != nil {
			return err
		}
		if err = content.Validate(); err != nil {
			return err
		}
		if !replicaUUIDPattern.MatchString(args[0]) || !replicaUUIDPattern.MatchString(merchant) || !replicaUUIDPattern.MatchString(requestID) || version < 0 || preview < 0 || preview > len(content.Steps) || price < 0 || price > 10000000 {
			return output.Validation("WEBSITE_TUTORIAL_INPUT_INVALID", "invalid Work, merchant, request ID, expected version, preview count or price")
		}
		if err = runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
			return err
		}
		result, err := runtime.client().PublishWebsiteTutorial(command.Context(), args[0], api.PublishWebsiteTutorialRequest{MerchantAccountID: merchant, ClientRequestID: requestID, ExpectedVersion: version, Content: content, PreviewStepCount: preview, PriceCents: price})
		if err != nil {
			return err
		}
		return runtime.business(result)
	}}
	command.Flags().StringVar(&filename, "input", "", "tutorial content JSON file")
	command.Flags().StringVar(&merchant, "merchant", "", "merchant account UUID")
	command.Flags().StringVar(&requestID, "request-id", "", "stable UUID; reuse with the identical input when retrying")
	command.Flags().IntVar(&version, "expected-version", 0, "current tutorial version; 0 for the first publication")
	command.Flags().IntVar(&preview, "preview-steps", 0, "number of publicly readable steps")
	command.Flags().IntVar(&price, "price-cents", 0, "price in minor currency units; 0 makes the entire tutorial public")
	for _, flag := range []string{"input", "merchant", "request-id", "expected-version", "preview-steps", "price-cents"} {
		_ = command.MarkFlagRequired(flag)
	}
	group.AddCommand(command)
	return group
}
func newTutorialCommand(runtime *Runtime) *cobra.Command {
	group := &cobra.Command{Use: "tutorial", Short: "Preview, read, buy and download website creation tutorials"}
	for _, action := range []string{"preview", "read", "download", "buy"} {
		group.AddCommand(newTutorialReadCommand(runtime, action))
	}
	return group
}
func newTutorialReadCommand(runtime *Runtime, action string) *cobra.Command {
	var version int
	var destination string
	command := &cobra.Command{Use: action + " <work-id>", Short: action + " a website tutorial (latest version by default)", Args: cobra.ExactArgs(1), RunE: func(command *cobra.Command, args []string) error {
		if command.Flags().Changed("version") && version < 1 {
			return output.Validation("WEBSITE_TUTORIAL_VERSION_INVALID", "--version must be positive")
		}
		view, err := runtime.client().GetWebsiteTutorial(command.Context(), args[0], version, action == "preview")
		if err != nil {
			return err
		}
		switch action {
		case "download":
			if !view.Unlocked {
				return output.Policy("WEBSITE_TUTORIAL_LOCKED", "purchase this tutorial version before downloading").WithDetails(map[string]any{"workId": view.WorkID, "version": view.Version, "productId": view.ProductID})
			}
			content := api.WebsiteTutorialContent{SchemaVersion: 1, Title: view.Title, Summary: view.Summary, Prerequisites: view.Prerequisites, Steps: make([]api.WebsiteTutorialStep, 0, len(view.Steps))}
			for _, step := range view.Steps {
				if step.Content == nil {
					return output.Validation("WEBSITE_TUTORIAL_RESPONSE_INVALID", "unlocked tutorial contains missing content")
				}
				content.Steps = append(content.Steps, *step.Content)
			}
			if err = content.Validate(); err != nil {
				return output.Validation("WEBSITE_TUTORIAL_RESPONSE_INVALID", "invalid downloadable tutorial")
			}
			target, err := writeTutorialFile(destination, content)
			if err != nil {
				return err
			}
			return runtime.business(map[string]any{"workId": view.WorkID, "version": view.Version, "path": target})
		case "buy":
			if view.Unlocked {
				return runtime.business(map[string]any{"workId": view.WorkID, "version": view.Version, "unlocked": true, "next": "DOWNLOAD"})
			}
			if runtime.region != config.RegionCN {
				return output.Policy("PRODUCT_PAYMENT_CAPABILITY_UNAVAILABLE", "tutorial purchases are not available in this market")
			}
			checkout := "/checkout?" + url.Values{"productId": {*view.ProductID}}.Encode()
			login := strings.TrimRight(runtime.profile.ResolvedWebBaseURL(), "/") + "/login?" + url.Values{"returnTo": {checkout}}.Encode()
			return runtime.business(map[string]any{"workId": view.WorkID, "version": view.Version, "productId": view.ProductID, "priceCents": view.PriceCents, "unlocked": false, "next": "OPEN_CHECKOUT", "checkoutUrl": login, "instruction": "Sign in to the website with the same account as this CLI; complete checkout, then download this exact version. No payment has been made by this command."})
		default:
			return runtime.business(view)
		}
	}}
	command.Flags().IntVar(&version, "version", 0, "exact tutorial version (omit for latest)")
	if action == "download" {
		command.Flags().StringVar(&destination, "output", "", "new JSON output file; existing files are never overwritten")
		_ = command.MarkFlagRequired("output")
	}
	return command
}

// Stage the complete private file, then publish it without replacing an existing path.
func writeTutorialFile(destination string, content api.WebsiteTutorialContent) (string, error) {
	target, err := filepath.Abs(destination)
	if err != nil {
		return "", output.Validation("WEBSITE_TUTORIAL_OUTPUT_INVALID", "invalid output path")
	}
	data, err := json.MarshalIndent(content, "", "  ")
	if err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), ".tutorial-*.tmp")
	if err != nil {
		return "", output.Validation("WEBSITE_TUTORIAL_OUTPUT_INVALID", "cannot create output file")
	}
	defer os.Remove(temporary.Name())
	if _, err = temporary.Write(append(data, '\n')); err != nil {
		_ = temporary.Close()
		return "", output.Validation("WEBSITE_TUTORIAL_OUTPUT_FAILED", "cannot write output file")
	}
	if err = temporary.Close(); err != nil {
		return "", output.Validation("WEBSITE_TUTORIAL_OUTPUT_FAILED", "cannot close output file")
	}
	if err = os.Link(temporary.Name(), target); err != nil {
		return "", output.Validation("WEBSITE_TUTORIAL_OUTPUT_FAILED", "cannot publish output file; choose a new path on a filesystem supporting hard links")
	}
	return target, nil
}
