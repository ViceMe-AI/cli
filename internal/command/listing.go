package command

import (
	"net"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf16"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

var listingSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func newListingCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "listing", Short: "Manage the public Listing for a Creator App"}
	command.AddCommand(newListingUpsertCommand(runtime))
	command.AddCommand(newListingGetCommand(runtime))
	return command
}

func newListingUpsertCommand(runtime *Runtime) *cobra.Command {
	var directory string
	var appID string
	var slug string
	var title string
	var summary string
	var description string
	var externalURL string
	var coverURL string
	var mediaURLs []string
	var offerID string
	var status string
	command := &cobra.Command{
		Use:   "upsert",
		Short: "Create or update the linked App public Listing",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			_, manifest, err := loadAppBinding(directory, appID)
			if err != nil {
				return err
			}
			slug = strings.ToLower(strings.TrimSpace(slug))
			if len(slug) < 2 || len(slug) > 80 || !listingSlugPattern.MatchString(slug) {
				return output.Validation("listing_slug", "--slug must contain 2-80 lowercase letters, numbers, or single hyphens")
			}
			title = strings.TrimSpace(title)
			summary = strings.TrimSpace(summary)
			description = strings.TrimSpace(description)
			if err := validateListingText("title", title, 100); err != nil {
				return err
			}
			if err := validateListingText("summary", summary, 280); err != nil {
				return err
			}
			if err := validateListingText("description", description, 20_000); err != nil {
				return err
			}
			external, err := optionalListingURL("external-url", externalURL)
			if err != nil {
				return err
			}
			cover, err := optionalListingURL("cover-url", coverURL)
			if err != nil {
				return err
			}
			if len(mediaURLs) > 12 {
				return output.Validation("listing_media_urls", "--media-url can be provided at most 12 times")
			}
			media := make([]string, 0, len(mediaURLs))
			for _, value := range mediaURLs {
				parsed, parseErr := requiredListingURL("media-url", value)
				if parseErr != nil {
					return parseErr
				}
				media = append(media, parsed)
			}
			var offer *string
			offerID = strings.ToLower(strings.TrimSpace(offerID))
			if offerID != "" {
				if !commerceUUIDPattern.MatchString(offerID) {
					return output.Validation("listing_offer_id", "--offer must be a UUID")
				}
				offer = &offerID
			}
			status = strings.ToUpper(strings.TrimSpace(status))
			if status != "DRAFT" && status != "PUBLIC" && status != "UNLISTED" {
				return output.Validation("listing_status", "--status must be DRAFT, PUBLIC, or UNLISTED")
			}
			if status == "PUBLIC" && manifest.HostingMode == "EXTERNAL" && external == nil {
				return output.Validation("listing_external_url", "a PUBLIC EXTERNAL App requires --external-url")
			}
			listing, err := runtime.client().UpsertCreatorAppListing(command.Context(), manifest.AppID, api.UpsertCreatorAppListingRequest{
				Slug:        slug,
				Title:       title,
				Summary:     summary,
				Description: description,
				ExternalURL: external,
				CoverURL:    cover,
				MediaURLs:   media,
				OfferID:     offer,
				Status:      status,
			})
			if err != nil {
				return err
			}
			return runtime.business(map[string]any{"listing": listing})
		},
	}
	addBindingFlags(command, &directory, &appID)
	command.Flags().StringVar(&slug, "slug", "", "public Listing slug")
	command.Flags().StringVar(&title, "title", "", "Listing title")
	command.Flags().StringVar(&summary, "summary", "", "Listing summary")
	command.Flags().StringVar(&description, "description", "", "Listing description")
	command.Flags().StringVar(&externalURL, "external-url", "", "deployed external App URL")
	command.Flags().StringVar(&coverURL, "cover-url", "", "public HTTPS cover image URL")
	command.Flags().StringSliceVar(&mediaURLs, "media-url", nil, "public HTTPS media URL; repeat for multiple items")
	command.Flags().StringVar(&offerID, "offer", "", "optional LIVE Commerce Offer UUID")
	command.Flags().StringVar(&status, "status", "DRAFT", "Listing status: DRAFT, PUBLIC, or UNLISTED")
	return command
}

func newListingGetCommand(runtime *Runtime) *cobra.Command {
	var directory string
	var appID string
	command := &cobra.Command{
		Use:   "get",
		Short: "Get the linked App Listing",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			_, manifest, err := loadAppBinding(directory, appID)
			if err != nil {
				return err
			}
			listing, err := runtime.client().GetCreatorAppListing(command.Context(), manifest.AppID)
			if err != nil {
				return err
			}
			return runtime.business(listing)
		},
	}
	addBindingFlags(command, &directory, &appID)
	return command
}

func validateListingText(name, value string, max int) error {
	if value == "" {
		return output.Validation("listing_"+strings.ReplaceAll(name, "-", "_"), "--"+name+" is required")
	}
	if len(utf16.Encode([]rune(value))) > max {
		return output.Validation("listing_"+strings.ReplaceAll(name, "-", "_"), "--"+name+" exceeds the platform limit")
	}
	return nil
}

func optionalListingURL(name, value string) (*string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := requiredListingURL(name, value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func requiredListingURL(name, value string) (string, error) {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", output.Validation("listing_"+strings.ReplaceAll(name, "-", "_"), "--"+name+" must be an absolute HTTP(S) URL without credentials or fragments")
	}
	if parsed.Scheme != "https" {
		ip := net.ParseIP(parsed.Hostname())
		if parsed.Scheme != "http" || !(strings.EqualFold(parsed.Hostname(), "localhost") || ip != nil && ip.IsLoopback()) {
			return "", output.Validation("listing_"+strings.ReplaceAll(name, "-", "_"), "--"+name+" must use HTTPS; HTTP is allowed only for loopback development")
		}
	}
	return parsed.String(), nil
}
