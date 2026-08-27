package command

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

func newMerchantCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{
		Use:   "merchant",
		Short: "Author and operate products for an approved ViceMe merchant",
	}
	command.AddCommand(newMerchantAccountsCommand(runtime))
	command.AddCommand(newMerchantContractCommand(runtime))
	command.AddCommand(newMerchantWorkCommand(runtime))
	command.AddCommand(newMerchantProductCommand(runtime))
	return command
}

var merchantInputContractCodes = map[string]struct{}{
	"work-create":      {},
	"analysis-create":  {},
	"analysis-confirm": {},
	"product-create":   {},
}

func newMerchantContractCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "contract", Short: "Inspect and validate authoritative merchant input contracts"}
	command.AddCommand(newMerchantContractShowCommand(runtime))
	command.AddCommand(newMerchantContractValidateCommand(runtime))
	return command
}

func newMerchantContractShowCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use: "show <contract-code>", Short: "Show one authoritative JSON Schema and complete example", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := validateMerchantInputContractCode(args[0]); err != nil {
				return err
			}
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), false); err != nil {
				return err
			}
			result, err := runtime.client().DescribeMerchantInputContract(command.Context(), args[0])
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
}

func newMerchantContractValidateCommand(runtime *Runtime) *cobra.Command {
	var inputFile string
	command := &cobra.Command{
		Use: "validate <contract-code>", Short: "Validate strict JSON without creating or changing anything", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := validateMerchantInputContractCode(args[0]); err != nil {
				return err
			}
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), false); err != nil {
				return err
			}
			input, err := readJSONObject(inputFile, "MERCHANT_INPUT_CONTRACT_INVALID")
			if err != nil {
				return err
			}
			return runtime.validateMerchantInput(command.Context(), args[0], input)
		},
	}
	command.Flags().StringVar(&inputFile, "input", "", "strict JSON file to validate without side effects")
	_ = command.MarkFlagRequired("input")
	return command
}

func validateMerchantInputContractCode(code string) error {
	if _, ok := merchantInputContractCodes[code]; ok {
		return nil
	}
	return output.Validation("MERCHANT_INPUT_CONTRACT_UNKNOWN", fmt.Sprintf("unknown merchant input contract %q", code)).
		WithHint("use work-create, analysis-create, analysis-confirm, or product-create")
}

func (runtime *Runtime) validateMerchantInput(ctx context.Context, code string, input json.RawMessage) error {
	result, err := runtime.client().ValidateMerchantInputContract(ctx, code, input)
	if err != nil {
		return err
	}
	if result.Valid {
		return runtime.business(result)
	}
	return output.Validation("MERCHANT_INPUT_CONTRACT_INVALID", "input does not match the authoritative merchant contract").
		WithHint("correct every reported issue before retrying; never retry unchanged input").
		WithDetails(result)
}

func (runtime *Runtime) requireValidMerchantInput(ctx context.Context, code string, input json.RawMessage) error {
	result, err := runtime.client().ValidateMerchantInputContract(ctx, code, input)
	if err != nil {
		return err
	}
	if result.Valid {
		return nil
	}
	return output.Validation("MERCHANT_INPUT_CONTRACT_INVALID", "input does not match the authoritative merchant contract").
		WithHint("correct every reported issue before retrying; never retry unchanged input").
		WithDetails(result)
}

func newMerchantAccountsCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "accounts",
		Short: "List merchant accounts owned by the current login",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), false); err != nil {
				return err
			}
			result, err := runtime.client().ListMerchantAccounts(command.Context())
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
}

func newMerchantWorkCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "work", Short: "Manage merchant Works"}
	command.AddCommand(newMerchantWorkCreateCommand(runtime))
	command.AddCommand(newMerchantWorkListCommand(runtime))
	command.AddCommand(newMerchantWorkGetCommand(runtime))
	command.AddCommand(newMerchantWorkUpdateCommand(runtime))
	command.AddCommand(newMerchantWorkAnalysisCommand(runtime))
	command.AddCommand(newMerchantWorkDraftCommand(runtime))
	command.AddCommand(newMerchantWorkPreviewCommand(runtime))
	command.AddCommand(newMerchantWorkActivateCommand(runtime))
	return command
}

func newMerchantWorkAnalysisCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "analysis", Short: "Create and persist user-reviewed scenario analysis before Interaction compilation"}
	command.AddCommand(newMerchantWorkAnalysisCreateCommand(runtime))
	command.AddCommand(newMerchantWorkAnalysisShowCommand(runtime))
	command.AddCommand(newMerchantWorkAnalysisConfirmCommand(runtime))
	return command
}

func newMerchantWorkAnalysisCreateCommand(runtime *Runtime) *cobra.Command {
	var inputFile string
	command := &cobra.Command{
		Use: "create", Short: "Create an exact scenario analysis requiring creator confirmation", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			input, err := readJSONObject(inputFile, "MERCHANT_INTERACTION_ANALYSIS_INPUT_INVALID")
			if err != nil {
				return err
			}
			if err := runtime.requireValidMerchantInput(command.Context(), "analysis-create", input); err != nil {
				return err
			}
			workID, request, err := splitInteractionEnvelope(input, false, "MERCHANT_INTERACTION_ANALYSIS_INPUT_INVALID")
			if err != nil {
				return err
			}
			result, err := runtime.client().CreateInteractionAnalysis(command.Context(), workID, request)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&inputFile, "input", "", "private strict JSON containing workId, merchantAccountId, sourceType, and analysis")
	_ = command.MarkFlagRequired("input")
	return command
}

func newMerchantWorkAnalysisShowCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	command := &cobra.Command{
		Use: "show <work-id>", Short: "Show the current scenario analysis and confirmation requirements", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), false); err != nil {
				return err
			}
			result, err := runtime.client().ShowInteractionAnalysis(command.Context(), args[0], merchantAccountID)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID")
	_ = command.MarkFlagRequired("merchant")
	return command
}

func newMerchantWorkAnalysisConfirmCommand(runtime *Runtime) *cobra.Command {
	var inputFile, merchantAccountID, analysisID, analysisDigest string
	var rawResolutions []string
	command := &cobra.Command{
		Use: "confirm <work-id>", Short: "Confirm the current reviewed analysis without hand-writing internal acknowledgments",
		Args: func(command *cobra.Command, args []string) error {
			if inputFile != "" {
				return cobra.NoArgs(command, args)
			}
			return cobra.ExactArgs(1)(command, args)
		},
		RunE: func(command *cobra.Command, args []string) error {
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			var input json.RawMessage
			if inputFile != "" {
				legacyInput, err := readJSONObject(inputFile, "MERCHANT_INTERACTION_ANALYSIS_CONFIRMATION_INVALID")
				if err != nil {
					return err
				}
				input = legacyInput
			} else {
				if strings.TrimSpace(merchantAccountID) == "" || strings.TrimSpace(analysisID) == "" || strings.TrimSpace(analysisDigest) == "" {
					return output.Validation("MERCHANT_INTERACTION_ANALYSIS_CONFIRMATION_INVALID", "--merchant, --analysis, and --digest are required").
						WithHint("reuse the exact analysisId and digest returned by analysis create or show")
				}
				generated, current, err := runtime.buildInteractionAnalysisConfirmation(command.Context(), args[0], merchantAccountID, analysisID, analysisDigest, rawResolutions)
				if err != nil {
					return err
				}
				if current != nil {
					return runtime.business(current)
				}
				input = generated
			}
			if err := runtime.requireValidMerchantInput(command.Context(), "analysis-confirm", input); err != nil {
				return err
			}
			workID, analysisID, request, err := splitInteractionAnalysisConfirmation(input)
			if err != nil {
				return err
			}
			result, err := runtime.client().ConfirmInteractionAnalysis(command.Context(), workID, analysisID, request)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&inputFile, "input", "", "legacy strict confirmation JSON file")
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID")
	command.Flags().StringVar(&analysisID, "analysis", "", "exact analysis ID returned by Shop")
	command.Flags().StringVar(&analysisDigest, "digest", "", "exact analysis digest returned by Shop")
	command.Flags().StringArrayVar(&rawResolutions, "resolution", nil, "open decision as CODE=VALUE; repeat for multiple decisions")
	return command
}

type currentInteractionAnalysis struct {
	AnalysisID string `json:"analysisId"`
	Digest     string `json:"digest"`
	Status     string `json:"status"`
	Analysis   struct {
		ConfirmationItems []struct {
			Code string `json:"code"`
		} `json:"confirmationItems"`
		OpenDecisions []struct {
			Code    string   `json:"code"`
			Options []string `json:"options"`
		} `json:"openDecisions"`
	} `json:"analysis"`
}

func (runtime *Runtime) buildInteractionAnalysisConfirmation(ctx context.Context, workID, merchantAccountID, analysisID, analysisDigest string, rawResolutions []string) (json.RawMessage, any, error) {
	rawCurrent, err := runtime.client().ShowInteractionAnalysis(ctx, workID, merchantAccountID)
	if err != nil {
		return nil, nil, err
	}
	var current currentInteractionAnalysis
	if err := json.Unmarshal(rawCurrent, &current); err != nil {
		return nil, nil, output.Internal("MERCHANT_INTERACTION_ANALYSIS_RESPONSE_INVALID", "Shop returned an invalid scenario analysis", err)
	}
	if current.AnalysisID != analysisID || current.Digest != analysisDigest {
		return nil, nil, output.Validation("MERCHANT_INTERACTION_ANALYSIS_CHANGED", "the current scenario analysis no longer matches the reviewed version").
			WithHint("show the current analysis and obtain confirmation for that exact version")
	}
	if current.Status == "CONFIRMED" || current.Status == "CONSUMED" {
		return nil, json.RawMessage(rawCurrent), nil
	}
	if current.Status != "REVIEW_REQUIRED" {
		return nil, nil, output.Validation("MERCHANT_INTERACTION_ANALYSIS_NOT_CONFIRMABLE", "the current scenario analysis cannot be confirmed").
			WithDetails(map[string]any{"status": current.Status})
	}
	resolutions, err := parseInteractionAnalysisResolutions(rawResolutions)
	if err != nil {
		return nil, nil, err
	}
	for _, decision := range current.Analysis.OpenDecisions {
		value, ok := resolutions[decision.Code]
		if !ok {
			return nil, nil, output.Validation("MERCHANT_INTERACTION_ANALYSIS_RESOLUTION_REQUIRED", fmt.Sprintf("open decision %s requires one resolution", decision.Code)).
				WithDetails(map[string]any{"code": decision.Code, "allowedValues": decision.Options})
		}
		allowed := false
		for _, option := range decision.Options {
			if value == option {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, nil, output.Validation("MERCHANT_INTERACTION_ANALYSIS_RESOLUTION_INVALID", fmt.Sprintf("resolution for %s is not an allowed value", decision.Code)).
				WithDetails(map[string]any{"code": decision.Code, "allowedValues": decision.Options})
		}
	}
	if len(resolutions) != len(current.Analysis.OpenDecisions) {
		return nil, nil, output.Validation("MERCHANT_INTERACTION_ANALYSIS_RESOLUTION_UNKNOWN", "a resolution was supplied for an unknown open decision")
	}
	acknowledgedCodes := make([]string, 0, len(current.Analysis.ConfirmationItems))
	for _, item := range current.Analysis.ConfirmationItems {
		acknowledgedCodes = append(acknowledgedCodes, item.Code)
	}
	resolutionItems := make([]map[string]string, 0, len(resolutions))
	for _, decision := range current.Analysis.OpenDecisions {
		resolutionItems = append(resolutionItems, map[string]string{"code": decision.Code, "value": resolutions[decision.Code]})
	}
	input, err := json.Marshal(map[string]any{
		"workId": workID, "analysisId": analysisID, "merchantAccountId": merchantAccountID,
		"analysisDigest": analysisDigest, "acknowledgedCodes": acknowledgedCodes, "resolutions": resolutionItems,
	})
	if err != nil {
		return nil, nil, output.Internal("MERCHANT_INTERACTION_ANALYSIS_CONFIRMATION_INVALID", "failed to encode scenario analysis confirmation", err)
	}
	return input, nil, nil
}

func parseInteractionAnalysisResolutions(raw []string) (map[string]string, error) {
	resolutions := make(map[string]string, len(raw))
	for _, item := range raw {
		code, value, ok := strings.Cut(item, "=")
		code, value = strings.TrimSpace(code), strings.TrimSpace(value)
		if !ok || code == "" || value == "" {
			return nil, output.Validation("MERCHANT_INTERACTION_ANALYSIS_RESOLUTION_INVALID", "--resolution must use CODE=VALUE")
		}
		if _, exists := resolutions[code]; exists {
			return nil, output.Validation("MERCHANT_INTERACTION_ANALYSIS_RESOLUTION_DUPLICATE", fmt.Sprintf("resolution %s was provided more than once", code))
		}
		resolutions[code] = value
	}
	return resolutions, nil
}

func newMerchantWorkPreviewCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "preview", Short: "Create and revoke private HTML/Markdown Work previews"}
	command.AddCommand(newMerchantWorkPreviewCreateCommand(runtime))
	command.AddCommand(newMerchantWorkPreviewRevokeCommand(runtime))
	return command
}

func newMerchantWorkPreviewCreateCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	var expectedRevision, expiresInSeconds int
	command := &cobra.Command{
		Use:   "create <work-id>",
		Short: "Create an Interaction preview or a revocable Work revision preview",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if expectedRevision < 0 {
				return output.Validation("MERCHANT_WORK_REVISION_INVALID", "--expected-revision cannot be negative")
			}
			if expectedRevision > 0 && (expiresInSeconds < 60 || expiresInSeconds > 3600) {
				return output.Validation("MERCHANT_WORK_PREVIEW_TTL_INVALID", "--expires-in must be between 60 and 3600 seconds")
			}
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			var result any
			var err error
			if expectedRevision == 0 {
				result, err = runtime.client().CreateInteractionPreview(command.Context(), args[0], merchantAccountID)
			} else {
				result, err = runtime.client().CreateMerchantWorkPreview(command.Context(), args[0], merchantAccountID, expectedRevision, expiresInSeconds)
			}
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID")
	command.Flags().IntVar(&expectedRevision, "expected-revision", 0, "exact Work revision")
	command.Flags().IntVar(&expiresInSeconds, "expires-in", 900, "preview lifetime in seconds")
	_ = command.MarkFlagRequired("merchant")
	return command
}

func newMerchantWorkDraftCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "draft", Short: "Manage deterministic Interaction definition drafts"}
	command.AddCommand(newMerchantWorkDraftCreateCommand(runtime))
	command.AddCommand(newMerchantWorkDraftShowCommand(runtime))
	return command
}

func newMerchantWorkDraftCreateCommand(runtime *Runtime) *cobra.Command {
	var inputFile string
	command := &cobra.Command{
		Use: "create", Short: "Create a validated Interaction draft from compiled JSON", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			input, err := readJSONObject(inputFile, "MERCHANT_INTERACTION_DRAFT_INPUT_INVALID")
			if err != nil {
				return err
			}
			workID, request, err := splitInteractionDraftInput(input)
			if err != nil {
				return err
			}
			result, err := runtime.client().CreateInteractionDraft(command.Context(), workID, request)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&inputFile, "input", "", "private strict JSON containing workId, merchantAccountId, sourceType, and definition")
	_ = command.MarkFlagRequired("input")
	return command
}

func newMerchantWorkDraftShowCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	command := &cobra.Command{
		Use: "show <work-id>", Short: "Show the latest Interaction draft and exact preview digest", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), false); err != nil {
				return err
			}
			result, err := runtime.client().ShowInteractionDraft(command.Context(), args[0], merchantAccountID)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID")
	_ = command.MarkFlagRequired("merchant")
	return command
}

func newMerchantWorkActivateCommand(runtime *Runtime) *cobra.Command {
	var inputFile string
	command := &cobra.Command{
		Use: "activate <work-id>", Short: "Atomically activate one confirmed Interaction candidate", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			input, err := readJSONObject(inputFile, "MERCHANT_INTERACTION_ACTIVATION_INPUT_INVALID")
			if err != nil {
				return err
			}
			result, err := runtime.client().ActivateInteractionDefinition(command.Context(), args[0], input)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&inputFile, "input", "", "strict JSON containing merchantAccountId, expectedDraftRevision, and candidateDigest")
	_ = command.MarkFlagRequired("input")
	return command
}

func splitInteractionDraftInput(input json.RawMessage) (string, json.RawMessage, error) {
	return splitInteractionEnvelope(input, true, "MERCHANT_INTERACTION_DRAFT_INPUT_INVALID")
}

func splitInteractionAnalysisConfirmation(input json.RawMessage) (string, string, json.RawMessage, error) {
	workID, request, err := splitInteractionEnvelope(input, true, "MERCHANT_INTERACTION_ANALYSIS_CONFIRMATION_INVALID")
	if err != nil {
		return "", "", nil, err
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(request, &envelope); err != nil {
		return "", "", nil, output.Validation("MERCHANT_INTERACTION_ANALYSIS_CONFIRMATION_INVALID", "interaction analysis confirmation must be a JSON object")
	}
	var analysisID string
	if raw, ok := envelope["analysisId"]; ok {
		_ = json.Unmarshal(raw, &analysisID)
	}
	if strings.TrimSpace(analysisID) == "" {
		return "", "", nil, output.Validation("MERCHANT_INTERACTION_ANALYSIS_REQUIRED", "interaction analysis confirmation must contain analysisId")
	}
	delete(envelope, "analysisId")
	request, err = json.Marshal(envelope)
	if err != nil {
		return "", "", nil, output.Internal("MERCHANT_INTERACTION_ANALYSIS_CONFIRMATION_INVALID", "failed to encode interaction analysis confirmation", err)
	}
	return workID, analysisID, request, nil
}

func splitInteractionEnvelope(input json.RawMessage, keepAnalysisID bool, code string) (string, json.RawMessage, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(input, &envelope); err != nil {
		return "", nil, output.Validation(code, "interaction input must be a JSON object")
	}
	var workID string
	if raw, ok := envelope["workId"]; ok {
		_ = json.Unmarshal(raw, &workID)
	}
	if strings.TrimSpace(workID) == "" {
		return "", nil, output.Validation("MERCHANT_INTERACTION_WORK_REQUIRED", "interaction input must contain workId")
	}
	delete(envelope, "workId")
	if !keepAnalysisID {
		delete(envelope, "analysisId")
	}
	request, err := json.Marshal(envelope)
	if err != nil {
		return "", nil, output.Internal(code, "failed to encode interaction input", err)
	}
	return workID, request, nil
}

func newMerchantWorkPreviewRevokeCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	command := &cobra.Command{
		Use:   "revoke <work-id> <preview-id>",
		Short: "Revoke a private Work preview",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			result, err := runtime.client().RevokeMerchantWorkPreview(command.Context(), args[0], args[1], merchantAccountID)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID")
	_ = command.MarkFlagRequired("merchant")
	return command
}

func newMerchantWorkCreateCommand(runtime *Runtime) *cobra.Command {
	var inputFile string
	command := &cobra.Command{
		Use:   "create",
		Short: "Create a merchant Work from a strict JSON request",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			input, err := readJSONObject(inputFile, "MERCHANT_WORK_INPUT_INVALID")
			if err != nil {
				return err
			}
			if err := runtime.requireValidMerchantInput(command.Context(), "work-create", input); err != nil {
				return err
			}
			result, err := runtime.client().CreateMerchantWork(command.Context(), input)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&inputFile, "input", "", "strict create-Work JSON file")
	_ = command.MarkFlagRequired("input")
	return command
}

func newMerchantWorkListCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	command := &cobra.Command{
		Use:   "list",
		Short: "List Works owned by one merchant",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), false); err != nil {
				return err
			}
			result, err := runtime.client().ListMerchantWorks(command.Context(), merchantAccountID)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID")
	_ = command.MarkFlagRequired("merchant")
	return command
}

func newMerchantWorkGetCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	command := &cobra.Command{
		Use:   "get <work-id>",
		Short: "Get one merchant Work",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), false); err != nil {
				return err
			}
			result, err := runtime.client().GetMerchantWork(command.Context(), args[0], merchantAccountID)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID")
	_ = command.MarkFlagRequired("merchant")
	return command
}

func newMerchantWorkUpdateCommand(runtime *Runtime) *cobra.Command {
	var inputFile string
	command := &cobra.Command{
		Use:   "update <work-id>",
		Short: "Update a merchant Work from a strict JSON request",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			input, err := readJSONObject(inputFile, "MERCHANT_WORK_INPUT_INVALID")
			if err != nil {
				return err
			}
			result, err := runtime.client().UpdateMerchantWork(command.Context(), args[0], input)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&inputFile, "input", "", "strict update-Work JSON file")
	_ = command.MarkFlagRequired("input")
	return command
}

func newMerchantProductCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "product", Short: "Manage merchant Products and generated purchase Skills"}
	command.AddCommand(newMerchantProductAuthoringTemplatesCommand(runtime))
	command.AddCommand(newMerchantProductCreateCommand(runtime))
	command.AddCommand(newMerchantProductUpdateCommand(runtime))
	command.AddCommand(newMerchantProductCompileCommand(runtime))
	command.AddCommand(newMerchantProductActivateCommand(runtime))
	command.AddCommand(newMerchantProductLifecycleCommand(runtime, "suspend"))
	command.AddCommand(newMerchantProductLifecycleCommand(runtime, "archive"))
	command.AddCommand(newMerchantProductListCommand(runtime))
	return command
}

func newMerchantProductAuthoringTemplatesCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	command := &cobra.Command{
		Use:   "templates",
		Short: "List Product authoring templates available to one merchant",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), false); err != nil {
				return err
			}
			result, err := runtime.client().ListMerchantProductAuthoringTemplates(command.Context(), merchantAccountID)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID")
	_ = command.MarkFlagRequired("merchant")
	return command
}

func newMerchantProductCreateCommand(runtime *Runtime) *cobra.Command {
	var inputFile string
	command := &cobra.Command{
		Use:   "create",
		Short: "Create a generic merchant Product draft from strict JSON",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			input, err := readJSONObject(inputFile, "MERCHANT_PRODUCT_INPUT_INVALID")
			if err != nil {
				return err
			}
			if err := runtime.requireValidMerchantInput(command.Context(), "product-create", input); err != nil {
				return err
			}
			result, err := runtime.client().CreateMerchantProduct(command.Context(), input)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&inputFile, "input", "", "strict create-Product JSON file")
	_ = command.MarkFlagRequired("input")
	return command
}

func newMerchantProductUpdateCommand(runtime *Runtime) *cobra.Command {
	var inputFile string
	command := &cobra.Command{
		Use:   "update <product-id>",
		Short: "Replace a generic Product draft from strict JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			input, err := readJSONObject(inputFile, "MERCHANT_PRODUCT_INPUT_INVALID")
			if err != nil {
				return err
			}
			result, err := runtime.client().UpdateMerchantProduct(command.Context(), args[0], input)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&inputFile, "input", "", "strict update-Product JSON file")
	_ = command.MarkFlagRequired("input")
	return command
}

func newMerchantProductCompileCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	var expectedRevision int
	command := &cobra.Command{
		Use:   "compile <product-id>",
		Short: "Compile a Product draft and its signed purchase Skill candidate",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if expectedRevision < 1 {
				return output.Validation("MERCHANT_PRODUCT_REVISION_INVALID", "--expected-revision must be positive")
			}
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			result, err := runtime.client().CompileMerchantProduct(command.Context(), args[0], merchantAccountID, expectedRevision)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID")
	command.Flags().IntVar(&expectedRevision, "expected-revision", 0, "exact Product revision")
	_ = command.MarkFlagRequired("merchant")
	_ = command.MarkFlagRequired("expected-revision")
	return command
}

func newMerchantProductActivateCommand(runtime *Runtime) *cobra.Command {
	var inputFile string
	command := &cobra.Command{
		Use:   "activate <product-id>",
		Short: "Activate the exact reviewed Product and purchase Skill candidate",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			input, err := readJSONObject(inputFile, "MERCHANT_PRODUCT_ACTIVATION_INVALID")
			if err != nil {
				return err
			}
			result, err := runtime.client().ActivateMerchantProduct(command.Context(), args[0], input)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&inputFile, "input", "", "strict activation JSON containing the reviewed candidate digests")
	_ = command.MarkFlagRequired("input")
	return command
}

func newMerchantProductLifecycleCommand(runtime *Runtime, action string) *cobra.Command {
	var merchantAccountID string
	var expectedRevision int
	command := &cobra.Command{
		Use:   action + " <product-id>",
		Short: strings.ToUpper(action[:1]) + action[1:] + " a merchant Product",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if expectedRevision < 1 {
				return output.Validation("MERCHANT_PRODUCT_REVISION_INVALID", "--expected-revision must be positive")
			}
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), true); err != nil {
				return err
			}
			result, err := runtime.client().CommandMerchantProduct(command.Context(), args[0], action, merchantAccountID, expectedRevision)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID")
	command.Flags().IntVar(&expectedRevision, "expected-revision", 0, "exact Product revision")
	_ = command.MarkFlagRequired("merchant")
	_ = command.MarkFlagRequired("expected-revision")
	return command
}

func newMerchantProductListCommand(runtime *Runtime) *cobra.Command {
	var merchantAccountID string
	var status string
	var cursor string
	var limit int
	command := &cobra.Command{
		Use:   "list",
		Short: "List Products owned by one merchant",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if limit < 1 || limit > 100 {
				return output.Validation("MERCHANT_PRODUCT_LIMIT_INVALID", "--limit must be between 1 and 100")
			}
			if err := runtime.requireMerchantCommerceAuthentication(command.Context(), false); err != nil {
				return err
			}
			result, err := runtime.client().ListMerchantProducts(command.Context(), merchantAccountID, strings.ToUpper(status), cursor, limit)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&merchantAccountID, "merchant", "", "merchant account ID")
	command.Flags().StringVar(&status, "status", "", "optional DRAFT, ACTIVE, SUSPENDED, or ARCHIVED filter")
	command.Flags().StringVar(&cursor, "cursor", "", "pagination cursor")
	command.Flags().IntVar(&limit, "limit", 20, "page size")
	_ = command.MarkFlagRequired("merchant")
	return command
}

func (runtime *Runtime) requireMerchantCommerceAuthentication(ctx context.Context, write bool) error {
	status, err := runtime.client().AuthStatus(ctx)
	if err != nil {
		return err
	}
	required := []string{"merchant-commerce:read"}
	if write {
		required = append(required, "merchant-commerce:write")
	}
	available := make(map[string]struct{}, len(status.Scopes))
	for _, scope := range status.Scopes {
		available[scope] = struct{}{}
	}
	var missing []string
	for _, scope := range required {
		if _, ok := available[scope]; !ok {
			missing = append(missing, scope)
		}
	}
	if len(missing) > 0 {
		return output.Authorization("MERCHANT_COMMERCE_SCOPE_REQUIRED", "the current login is not authorized to manage merchant commerce").
			WithHint("run 'viceme auth login' again for the current profile to grant merchant commerce access").
			WithDetails(map[string]any{"profile": runtime.profile.Name, "missingScopes": missing})
	}
	return nil
}
