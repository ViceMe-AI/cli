package command

import (
	"context"
	"encoding/json"
	"net/url"
	"reflect"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/replicapublication"
	"github.com/spf13/cobra"
)

type websiteAccessInput struct {
	MerchantAccountID string                          `json:"merchantAccountId,omitempty"`
	WorkID            string                          `json:"workId,omitempty"`
	Work              api.WebsiteAccessWorkInput      `json:"work"`
	AccessFeatures    []api.WebsiteAccessFeatureInput `json:"accessFeatures"`
}

type websiteAccessResult struct {
	Phase            string `json:"phase"`
	RequestID        string `json:"requestId"`
	ProjectBindingID string `json:"projectBindingId"`
	api.WebsiteAccessConfigurationResponse
	ResumeArgs          []string                  `json:"resumeArgs"`
	HostReceipt         *websiteAccessHostReceipt `json:"hostReceipt,omitempty"`
	EnrichmentSuggested bool                      `json:"enrichmentSuggested"`
}

func newWebsiteCommand(runtime *Runtime) *cobra.Command {
	root := &cobra.Command{Use: "website", Short: "Integrate website access and enrich the same Work"}
	access := &cobra.Command{Use: "access", Short: "Configure and resume website access without creator preflight"}
	for _, mode := range []string{"configure", "resume", "status"} {
		access.AddCommand(newWebsiteAccessCommand(runtime, mode))
	}
	work := &cobra.Command{Use: "work", Short: "Enrich a bound website Work"}
	work.AddCommand(newWebsiteEnrichCommand(runtime))
	root.AddCommand(access, work)
	return root
}

func newWebsiteAccessCommand(runtime *Runtime, mode string) *cobra.Command {
	var project, inputFile, receiptFile, merchant string
	command := &cobra.Command{Use: mode, Args: cobra.NoArgs, Short: mode + " a project's website access", RunE: func(command *cobra.Command, _ []string) error {
		canonical, lock, err := openWebsiteAccessState(runtime, project)
		if err != nil {
			return err
		}
		defer lock.Unlock()
		state, exists, err := loadWebsiteAccessState(runtime, canonical)
		if err != nil {
			return err
		}
		if mode == "configure" {
			input, err := readStrictJSONObject[websiteAccessInput](inputFile, "WEBSITE_ACCESS_INPUT_INVALID")
			if err != nil {
				return err
			}
			if err = normalizeWebsiteAccessInput(&input, replicaPublicationMarket(runtime)); err != nil {
				return err
			}
			if !exists {
				id, err := runtime.newReplicaRequestID()
				if err != nil {
					return err
				}
				requestID, err := runtime.newReplicaRequestID()
				if err != nil {
					return err
				}
				state = websiteAccessState{SchemaVersion: 1, EndpointOrigin: runtime.apiBaseURL, Market: replicaPublicationMarket(runtime), ProjectPath: canonical, ProjectBindingID: id, Phase: "PREPARED"}
				state.Request = api.WebsiteAccessConfigurationRequest{RequestID: requestID, ProjectBindingID: id, Market: state.Market}
				binding, found, err := replicaBindingStore(runtime).Load(canonical)
				if err != nil {
					return err
				}
				if found && binding.Work != nil {
					fingerprint, _, err := replicapublication.ProjectFingerprint(runtime.apiBaseURL, state.Market, canonical)
					if err != nil {
						return err
					}
					if fingerprint != binding.ProjectFingerprint {
						return output.Validation("WEBSITE_ACCESS_REPLICA_BINDING_CONFLICT", "Replica binding belongs to a different project path")
					}
					state.Request.WorkID = binding.Work.ID
					state.Request.MerchantAccountID = binding.Merchant.ID
				}
			}
			if err = prepareWebsiteAccessRequest(runtime, &state, input); err != nil {
				return err
			}
			if err = saveWebsiteAccessState(canonical, state); err != nil {
				return err
			}
		} else if !exists {
			return output.Validation("WEBSITE_ACCESS_NOT_CONFIGURED", "run website access configure with the project input first")
		}
		if merchant != "" {
			if state.Request.MerchantAccountID == "" && state.Attempted && (state.Result == nil || state.Result.Access != nil) {
				return output.Policy("WEBSITE_ACCESS_REQUEST_PENDING", "resume the uncertain request before selecting a different merchant")
			}
			if !replicaUUIDPattern.MatchString(merchant) {
				return output.Validation("WEBSITE_ACCESS_INPUT_INVALID", "merchant must be a UUID")
			}
			if state.Result != nil && state.Result.Access != nil && merchant != state.Result.MerchantAccountID {
				return output.Validation("WEBSITE_ACCESS_TARGET_CONFLICT", "cannot replace the bound merchant")
			}
			if state.Request.MerchantAccountID != "" && state.Request.MerchantAccountID != merchant {
				return output.Validation("WEBSITE_ACCESS_TARGET_CONFLICT", "cannot replace the selected merchant")
			}
			state.Request.MerchantAccountID = merchant
			if err = saveWebsiteAccessState(canonical, state); err != nil {
				return err
			}
		}
		if mode == "status" {
			return refreshWebsiteAccess(command.Context(), runtime, canonical, &state, false, receiptFile)
		}
		if state.Phase != "PREPARED" {
			return refreshWebsiteAccess(command.Context(), runtime, canonical, &state, true, receiptFile)
		}
		state.Attempted = true
		if err = saveWebsiteAccessState(canonical, state); err != nil {
			return err
		}
		previous := state.Result
		if state.PreviousAccess != nil {
			previous = &api.WebsiteAccessConfigurationResponse{Access: state.PreviousAccess}
		}
		result, err := runtime.client().ConfigureWebsiteAccess(command.Context(), state.Request)
		if err != nil {
			code := output.AsError(err).Subtype
			if code == "CONFIG_VERSION_CONFLICT" || code == "WORK_SDK_CONFIG_VERSION_CONFLICT" {
				result, err = runtime.client().ConfigureWebsiteAccess(command.Context(), state.Request)
			}
		}
		if err != nil {
			return websiteAccessActionError(runtime, canonical, state, err)
		}
		if result.Access != nil {
			if (state.Request.WorkID != "" && result.WorkID != state.Request.WorkID) || (state.Request.MerchantAccountID != "" && result.MerchantAccountID != state.Request.MerchantAccountID) {
				return output.Validation("WEBSITE_ACCESS_TARGET_MISMATCH", "configuration returned a different bound target")
			}
			if err = verifyWebsiteAccessDelta(result.Access, state.Request.AccessFeatures, previous); err != nil {
				return err
			}
			state.Phase = "PLATFORM_CONFIGURED"
		}
		state.Result = &result
		if err = saveWebsiteAccessState(canonical, state); err != nil {
			return err
		}
		if result.Access != nil {
			return refreshWebsiteAccess(command.Context(), runtime, canonical, &state, true, receiptFile)
		}
		return runtime.business(websiteAccessPresentation(runtime, canonical, state))
	}}
	command.Flags().StringVar(&project, "project", ".", "website project directory")
	if mode == "configure" {
		command.Flags().StringVar(&inputFile, "input", "", "strict Work and access feature delta JSON")
		_ = command.MarkFlagRequired("input")
	}
	if mode == "resume" {
		command.Flags().StringVar(&merchant, "merchant", "", "merchant selected from the returned creator candidates")
	}
	if mode != "configure" {
		command.Flags().StringVar(&receiptFile, "receipt", "", "host integration verification receipt JSON")
	}
	return command
}

func prepareWebsiteAccessRequest(runtime *Runtime, state *websiteAccessState, input websiteAccessInput) error {
	workID, merchant := state.Request.WorkID, state.Request.MerchantAccountID
	if state.Result != nil && state.Result.Access != nil {
		workID = state.Result.WorkID
		merchant = state.Result.MerchantAccountID
	}
	if input.WorkID != "" {
		if workID != "" && input.WorkID != workID {
			return output.Validation("WEBSITE_ACCESS_TARGET_CONFLICT", "input cannot replace a bound Work")
		}
		workID = input.WorkID
	}
	if input.MerchantAccountID != "" {
		if merchant != "" && input.MerchantAccountID != merchant {
			return output.Validation("WEBSITE_ACCESS_TARGET_CONFLICT", "input cannot replace the bound merchant")
		}
		merchant = input.MerchantAccountID
	}
	candidate := state.Request
	candidate.WorkID = workID
	candidate.MerchantAccountID = merchant
	candidate.Work = input.Work
	candidate.AccessFeatures = input.AccessFeatures
	if state.Result != nil && state.Result.Access != nil {
		prior := map[string]string{}
		for _, f := range state.Result.Access.AccessFeatures {
			prior[f.FeatureKey] = f.PolicyType
		}
		for _, f := range candidate.AccessFeatures {
			if policy, exists := prior[f.FeatureKey]; exists && policy != f.PolicyType {
				return output.Validation("WEBSITE_ACCESS_POLICY_CHANGED", "use a new feature key to change access policy")
			}
		}
	}
	if !reflect.DeepEqual(candidate, state.Request) {
		if state.Phase == "PREPARED" && state.Attempted && (state.Result == nil || state.Result.Access != nil) {
			return output.Policy("WEBSITE_ACCESS_REQUEST_PENDING", "resume the original request before changing an uncertain configuration")
		}
		if state.Phase != "PREPARED" {
			id, err := runtime.newReplicaRequestID()
			if err != nil {
				return err
			}
			candidate.RequestID = id
		}
		if state.Result != nil && state.Result.Access != nil {
			state.PreviousAccess = state.Result.Access
		}
		state.Result = nil
		state.Request = candidate
		state.Phase = "PREPARED"
		state.HostReceipt = nil
		state.Attempted = false
	}
	return nil
}

func normalizeWebsiteAccessInput(input *websiteAccessInput, market string) error {
	for _, id := range []string{input.WorkID, input.MerchantAccountID} {
		if id != "" && !replicaUUIDPattern.MatchString(id) {
			return output.Validation("WEBSITE_ACCESS_INPUT_INVALID", "Work and merchant IDs must be UUIDs")
		}
	}
	if value := input.Work.CanonicalOrigin; value != "" {
		parsed, err := url.Parse(value)
		if err != nil || parsed.User != nil || parsed.Host == "" || parsed.Scheme != "https" || strings.ContainsAny(value, "\r\n\t") {
			return output.Validation("WEBSITE_ACCESS_INPUT_INVALID", "optional website address must be a valid HTTPS URL")
		}
	}
	if len(input.AccessFeatures) > 100 {
		return output.Validation("WEBSITE_ACCESS_INPUT_INVALID", "at most 100 feature changes are supported")
	}
	seen := map[string]bool{}
	for i := range input.AccessFeatures {
		f := &input.AccessFeatures[i]
		if f.Status == "" {
			f.Status = "ACTIVE"
		}
		if f.Availability == "" {
			f.Availability = f.Status
			if f.Status != "DISABLED" && f.PolicyType == "WORK_ENTITLEMENT" && market == "GLOBAL" {
				f.Availability = "PENDING_CHANNEL"
			}
		}
		if !api.ValidWebsiteAccessFeatureInput(*f) || seen[f.FeatureKey] {
			return output.Validation("WEBSITE_ACCESS_INPUT_INVALID", "feature keys, policies, status and positive CNY/USD prices must be valid and unique")
		}
		// Shop stores lifecycle separately from channel availability.
		if f.Status == "PENDING_CHANNEL" {
			f.Status = "ACTIVE"
		}
		seen[f.FeatureKey] = true
	}
	if input.AccessFeatures == nil {
		input.AccessFeatures = []api.WebsiteAccessFeatureInput{}
	}
	return nil
}

func verifyWebsiteAccessDelta(access *api.WorkSdkAccess, delta []api.WebsiteAccessFeatureInput, previous *api.WebsiteAccessConfigurationResponse) error {
	byKey := map[string]api.WorkAccessFeature{}
	for _, f := range access.AccessFeatures {
		byKey[f.FeatureKey] = f
	}
	changed := map[string]bool{}
	for _, f := range delta {
		actual, found := byKey[f.FeatureKey]
		lifecycleStatus := actual.Status
		if lifecycleStatus == "PENDING_CHANNEL" && (actual.Availability == "" || actual.Availability == "PENDING_CHANNEL") {
			lifecycleStatus = "ACTIVE"
		}
		if !found || actual.PolicyType != f.PolicyType || actual.Title != f.Title || lifecycleStatus != f.Status {
			return output.Validation("WEBSITE_ACCESS_READBACK_MISMATCH", "configured feature does not match the requested delta")
		}
		changed[f.FeatureKey] = true
		if f.PricingIntent != nil {
			price := actual.PricingIntent
			if price == nil && actual.Price != nil {
				price = &api.WebsiteAccessPricingIntent{Currency: actual.Price.Currency, AmountMinor: int64(actual.Price.MinorUnits())}
			}
			if !reflect.DeepEqual(price, f.PricingIntent) {
				return output.Validation("WEBSITE_ACCESS_READBACK_MISMATCH", "configured price does not match the requested delta")
			}
		}
	}
	if previous != nil && previous.Access != nil {
		if access.Keys != previous.Access.Keys {
			return output.Validation("WEBSITE_ACCESS_KEYS_CHANGED", "configuration unexpectedly rotated the public SDK keys")
		}
		hosted := map[string]bool{}
		for _, f := range access.Features {
			hosted[f] = true
		}
		for _, f := range previous.Access.Features {
			if !hosted[f] {
				return output.Validation("WEBSITE_ACCESS_EXISTING_FEATURE_LOST", "configuration removed an unrelated hosted feature")
			}
		}
		for _, f := range previous.Access.AccessFeatures {
			if changed[f.FeatureKey] {
				if byKey[f.FeatureKey].PolicyType != f.PolicyType {
					return output.Validation("WEBSITE_ACCESS_POLICY_CHANGED", "use a new feature key to change access policy")
				}
				continue
			}
			if !reflect.DeepEqual(byKey[f.FeatureKey], f) {
				return output.Validation("WEBSITE_ACCESS_EXISTING_FEATURE_LOST", "configuration changed an unrelated access feature")
			}
		}
	}
	return nil
}

func refreshWebsiteAccess(ctx context.Context, runtime *Runtime, project string, state *websiteAccessState, persist bool, receiptFile string) error {
	if state.Result != nil && state.Result.Access != nil {
		access, err := runtime.client().GetWorkSdkAccess(ctx, state.Result.WorkID, state.Result.MerchantAccountID)
		if err != nil {
			return websiteAccessActionError(runtime, project, *state, err)
		}
		if access.Keys != state.Result.Access.Keys {
			return output.Validation("WEBSITE_ACCESS_KEYS_CHANGED", "live configuration has different SDK keys")
		}
		if access.ConfigVersion != state.Result.Access.ConfigVersion {
			state.Phase = "PLATFORM_CONFIGURED"
			state.HostReceipt = nil
		}
		if err = verifyWebsiteAccessDelta(&access, state.Request.AccessFeatures, nil); err != nil {
			return err
		}
		state.Result.Access = &access
	}
	if receiptFile != "" {
		receipt, err := readStrictJSONObject[websiteAccessHostReceipt](receiptFile, "WEBSITE_ACCESS_RECEIPT_INVALID")
		if err != nil {
			return err
		}
		if state.Result == nil || state.Result.Access == nil || receipt.RequestID != state.Request.RequestID || receipt.WorkID != state.Result.WorkID || receipt.ConfigVersion != state.Result.Access.ConfigVersion || len(receipt.ModifiedFiles) == 0 || len(receipt.Checks) == 0 {
			return output.Validation("WEBSITE_ACCESS_RECEIPT_INVALID", "host receipt must identify this request, current Work/version, files and checks")
		}
		state.HostReceipt = &receipt
		state.Phase = "HOST_INTEGRATED"
		if receipt.Verified {
			state.Phase = "VERIFIED"
		}
		persist = true
	}
	if persist {
		if err := saveWebsiteAccessState(project, *state); err != nil {
			return err
		}
	}
	return runtime.business(websiteAccessPresentation(runtime, project, *state))
}

func websiteAccessPresentation(runtime *Runtime, project string, state websiteAccessState) websiteAccessResult {
	result := websiteAccessResult{Phase: state.Phase, RequestID: state.Request.RequestID, ProjectBindingID: state.ProjectBindingID, ResumeArgs: []string{"website", "access", "resume", "--project", project, "--profile", runtime.profile.Name}, HostReceipt: state.HostReceipt, EnrichmentSuggested: state.Phase == "VERIFIED"}
	result.NextAction = "RESUME_CONFIGURATION"
	result.CompletedSteps = []string{}
	if state.Result != nil {
		result.WebsiteAccessConfigurationResponse = *state.Result
	}
	if state.Result != nil && state.Result.Access != nil {
		result.NextAction = "INTEGRATE_HOST"
		if state.Phase == "HOST_INTEGRATED" {
			result.NextAction = "VERIFY_HOST"
		}
	}
	if state.Phase == "VERIFIED" {
		result.NextAction = "ENRICH_WORK"
	}
	return result
}

func websiteAccessActionError(runtime *Runtime, project string, state websiteAccessState, err error) error {
	failure := output.AsError(err)
	action := "RESUME_CONFIGURATION"
	if failure.Code == output.ExitAuthentication {
		action = "AUTHENTICATE_CREATOR"
	}
	switch failure.Subtype {
	case "MERCHANT_NOT_FOUND", "MERCHANT_REQUIRED":
		action = "APPLY_CREATOR"
	case "ROUTE_NOT_FOUND", "NOT_FOUND":
		action = "WAIT_PLATFORM_UPGRADE"
	}
	// Preserve authoritative structured action when the API already supplies it.
	if details, ok := failure.Details.(map[string]any); ok {
		if next, ok := details["nextAction"].(string); ok {
			action = next
		}
	}
	encoded, _ := json.Marshal(failure.Details)
	var detail map[string]any
	_ = json.Unmarshal(encoded, &detail)
	if detail == nil {
		detail = map[string]any{}
	}
	detail["nextAction"] = action
	detail["requestId"] = state.Request.RequestID
	detail["phase"] = state.Phase
	detail["resumeArgs"] = websiteAccessPresentation(runtime, project, state).ResumeArgs
	return failure.WithDetails(detail)
}
