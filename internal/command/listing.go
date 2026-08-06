package command

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
	"unicode/utf16"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

var listingSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

const maxListingInputBytes = 128 << 10

func newListingCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "listing", Short: "Manage the public Listing for a Creator App"}
	command.AddCommand(newListingUpsertCommand(runtime))
	command.AddCommand(newListingGetCommand(runtime))
	return command
}

func newListingUpsertCommand(runtime *Runtime) *cobra.Command {
	var directory string
	var appID string
	var inputFile string
	command := &cobra.Command{
		Use:   "upsert",
		Short: "Create or update the linked App public Listing",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			_, manifest, err := loadAppBinding(directory, appID)
			if err != nil {
				return err
			}
			input, err := readListingInput(command.InOrStdin(), inputFile)
			if err != nil {
				return err
			}
			if err := validateListingInput(manifest.HostingMode, &input); err != nil {
				return err
			}
			listing, err := runtime.client().UpsertCreatorAppListing(command.Context(), manifest.AppID, input)
			if err != nil {
				return err
			}
			return runtime.business(map[string]any{"listing": listing})
		},
	}
	addBindingFlags(command, &directory, &appID)
	command.Flags().StringVar(&inputFile, "input-file", "", "complete Listing JSON file, or - for stdin")
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

func requiredListingURL(name, value string) (string, error) {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	if len(utf16.Encode([]rune(value))) > 2_048 || err != nil || parsed.Hostname() == "" || parsed.User != nil {
		return "", output.Validation("listing_"+strings.ReplaceAll(name, "-", "_"), name+" must be an absolute HTTP(S) URL of at most 2048 characters without credentials")
	}
	if parsed.Scheme != "https" {
		ip := net.ParseIP(parsed.Hostname())
		if parsed.Scheme != "http" || !(strings.EqualFold(parsed.Hostname(), "localhost") || ip != nil && ip.IsLoopback()) {
			return "", output.Validation("listing_"+strings.ReplaceAll(name, "-", "_"), "--"+name+" must use HTTPS; HTTP is allowed only for loopback development")
		}
	}
	return value, nil
}

func readListingInput(stdin io.Reader, inputFile string) (api.UpsertCreatorAppListingRequest, error) {
	inputFile = strings.TrimSpace(inputFile)
	if inputFile == "" {
		return api.UpsertCreatorAppListingRequest{}, output.Validation("listing_input_file", "--input-file is required")
	}
	var reader io.Reader = stdin
	var closeFile func() error
	if inputFile != "-" {
		file, err := os.Open(inputFile)
		if err != nil {
			return api.UpsertCreatorAppListingRequest{}, output.Validation("listing_input_file", "could not open --input-file")
		}
		reader = file
		closeFile = file.Close
	}
	if closeFile != nil {
		defer func() { _ = closeFile() }()
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxListingInputBytes+1))
	if err != nil {
		return api.UpsertCreatorAppListingRequest{}, output.Validation("listing_input_file", "could not read --input-file")
	}
	if len(data) > maxListingInputBytes {
		return api.UpsertCreatorAppListingRequest{}, output.Validation("listing_input_file", "--input-file exceeds 128 KiB")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return api.UpsertCreatorAppListingRequest{}, output.Validation("listing_input_json", "--input-file must contain one JSON object")
	}
	for _, field := range []string{"slug", "title", "summary", "description", "externalUrl", "coverUrl", "mediaUrls", "offerId", "status"} {
		if _, ok := fields[field]; !ok {
			return api.UpsertCreatorAppListingRequest{}, output.Validation("listing_input_json", "complete Listing JSON must include "+field)
		}
	}
	var input api.UpsertCreatorAppListingRequest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return api.UpsertCreatorAppListingRequest{}, output.Validation("listing_input_json", "--input-file does not match the Listing contract")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return api.UpsertCreatorAppListingRequest{}, output.Validation("listing_input_json", "--input-file must contain exactly one JSON object")
	}
	if input.MediaURLs == nil {
		return api.UpsertCreatorAppListingRequest{}, output.Validation("listing_media_urls", "mediaUrls must be an array")
	}
	return input, nil
}

func validateListingInput(hostingMode string, input *api.UpsertCreatorAppListingRequest) error {
	input.Slug = strings.TrimSpace(input.Slug)
	if len(input.Slug) < 2 || len(input.Slug) > 80 || !listingSlugPattern.MatchString(input.Slug) {
		return output.Validation("listing_slug", "slug must contain 2-80 lowercase letters, numbers, or single hyphens")
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Summary = strings.TrimSpace(input.Summary)
	input.Description = strings.TrimSpace(input.Description)
	if err := validateListingText("title", input.Title, 100); err != nil {
		return err
	}
	if err := validateListingText("summary", input.Summary, 280); err != nil {
		return err
	}
	if err := validateListingText("description", input.Description, 20_000); err != nil {
		return err
	}
	if err := normalizeOptionalListingURL("external-url", &input.ExternalURL); err != nil {
		return err
	}
	if err := normalizeOptionalListingURL("cover-url", &input.CoverURL); err != nil {
		return err
	}
	if len(input.MediaURLs) > 12 {
		return output.Validation("listing_media_urls", "mediaUrls can contain at most 12 items")
	}
	for index, value := range input.MediaURLs {
		parsed, err := requiredListingURL("media-url", value)
		if err != nil {
			return err
		}
		input.MediaURLs[index] = parsed
	}
	if input.OfferID != nil {
		value := strings.ToLower(strings.TrimSpace(*input.OfferID))
		if !commerceUUIDPattern.MatchString(value) {
			return output.Validation("listing_offer_id", "offerId must be a UUID or null")
		}
		input.OfferID = &value
	}
	input.Status = strings.ToUpper(strings.TrimSpace(input.Status))
	if input.Status != "DRAFT" && input.Status != "PUBLIC" && input.Status != "UNLISTED" {
		return output.Validation("listing_status", "status must be DRAFT, PUBLIC, or UNLISTED")
	}
	if input.Status == "PUBLIC" && hostingMode == "EXTERNAL" && input.ExternalURL == nil {
		return output.Validation("listing_external_url", "a PUBLIC EXTERNAL App requires externalUrl")
	}
	return nil
}

func normalizeOptionalListingURL(name string, target **string) error {
	if *target == nil {
		return nil
	}
	parsed, err := requiredListingURL(name, **target)
	if err != nil {
		return err
	}
	*target = &parsed
	return nil
}
