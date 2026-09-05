package command

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

var pageSetupApplicationID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type creatorPageSetupResult struct {
	api.CreatorPageSetup
	SetupURL string `json:"setupUrl"`
	Status   string `json:"status"`
	TimedOut bool   `json:"timedOut"`
}

func newCreatorPageSetupCommand(runtime *Runtime) *cobra.Command {
	var wait bool
	var timeout time.Duration
	var merchantID string
	cmd := &cobra.Command{
		Use: "page-setup [application-id]", Short: "Read or wait for the creator page choice after submitting an application", Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if (len(args) == 0) == (merchantID == "") {
				return output.Validation("PAGE_SETUP_TARGET_REQUIRED", "provide an application ID or --merchant, but not both")
			}
			applicationID := ""
			if len(args) == 1 {
				applicationID = args[0]
			}
			if applicationID != "" && !pageSetupApplicationID.MatchString(applicationID) {
				return output.Validation("APPLICATION_ID_INVALID", "application ID must be a UUID")
			}
			if merchantID != "" && !pageSetupApplicationID.MatchString(merchantID) {
				return output.Validation("MERCHANT_ID_INVALID", "merchant ID must be a UUID")
			}
			if timeout <= 0 || timeout > 15*time.Minute {
				return output.Validation("PAGE_SETUP_TIMEOUT_INVALID", "timeout must be between zero and 15 minutes")
			}
			if err := runtime.requireSkillPublicationAuthentication(cmd.Context()); err != nil {
				return err
			}
			if merchantID != "" {
				setup, err := runtime.client().GetCreatorPageSetupForMerchant(cmd.Context(), merchantID)
				if err != nil {
					return err
				}
				if !pageSetupApplicationID.MatchString(setup.ApplicationID) {
					return output.Internal("PAGE_SETUP_RESPONSE_INVALID", "ViceMe returned an invalid application", nil)
				}
				applicationID = setup.ApplicationID
			}
			setupURL, err := url.JoinPath(runtime.profile.ResolvedWebBaseURL(), "creator-page-setup")
			if err != nil {
				return err
			}
			setupURL += "?application=" + url.QueryEscape(applicationID)
			return waitForCreatorPageSetup(cmd.Context(), runtime, applicationID, setupURL, wait, timeout)
		},
	}
	cmd.Flags().BoolVar(&wait, "wait", false, "wait for a page choice; never wait for application review")
	cmd.Flags().StringVar(&merchantID, "merchant", "", "resume the signed-in applicant's page setup for this exact merchant")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "bounded wait for a page choice (at most 15m)")
	return cmd
}

func waitForCreatorPageSetup(ctx context.Context, runtime *Runtime, applicationID, setupURL string, wait bool, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var latest api.CreatorPageSetup
	announced := false
	finishPending := func() error {
		return runtime.business(creatorPageSetupResult{CreatorPageSetup: latest, SetupURL: setupURL, Status: "pending", TimedOut: true})
	}
	for {
		result, err := runtime.client().GetCreatorPageSetup(ctx, applicationID)
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) && latest.ApplicationID != "" {
				return finishPending()
			}
			return err
		}
		if result.ApplicationID != applicationID {
			return output.Internal("PAGE_SETUP_RESPONSE_INVALID", "ViceMe returned a different application", nil)
		}
		latest = result
		if result.Selection != nil {
			switch result.Selection.Mode {
			case "BONJOUR", "IMPORT_EXISTING", "SKIP":
				return runtime.business(creatorPageSetupResult{CreatorPageSetup: result, SetupURL: setupURL, Status: "selected"})
			default:
				return output.Internal("PAGE_SETUP_RESPONSE_INVALID", "ViceMe returned an invalid page choice", nil)
			}
		}
		if !wait {
			return runtime.business(creatorPageSetupResult{CreatorPageSetup: result, SetupURL: setupURL, Status: "pending"})
		}
		if !announced {
			_, _ = fmt.Fprintf(runtime.deps.ErrOut, "Your application is submitted; review continues independently.\nOpen this page to choose how to prepare your creator page:\n\n%s\n\nWaiting for your choice (including set up later)…\n", setupURL)
			announced = true
		}
		if err := runtime.deps.Sleep(ctx, 2*time.Second); err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return finishPending()
			}
			return err
		}
	}
}
