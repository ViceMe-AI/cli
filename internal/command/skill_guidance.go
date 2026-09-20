package command

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/privatefile"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	"github.com/spf13/cobra"
)

// This format is shared with the bundled Python runner. It contains task data,
// never a credential. The original input survives an uncertain POST response.
type guidanceTaskRecord struct {
	SchemaVersion   int                     `json:"schemaVersion"`
	APIBaseURL      string                  `json:"apiBaseUrl"`
	Market          string                  `json:"market"`
	Principal       string                  `json:"principal"`
	Input           api.SkillGuidanceSubmit `json:"input"`
	PaymentRequired bool                    `json:"paymentRequired"`
}

func guidanceResources(resources localSkillResources, mode string) localSkillResources {
	resources.DeliveryMode = api.DeliveryMode(mode)
	return resources
}

func newSkillGuidanceCommand(runtime *Runtime) *cobra.Command {
	var inputPath, productID, agent, market, skillDirectory string
	var wait time.Duration
	command := &cobra.Command{Use: "guidance --input <task.json>", Short: "Run a protected Skill task using an immutable UUID request key; retry the same file after an interrupted response", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			if skillDirectory != "" {
				copyRuntime := *runtime
				copyRuntime.deps.Environment.InstallDirectory = skillDirectory
				runtime = &copyRuntime
			}
			if market != "" {
				region, err := config.ParseRegion(market)
				if err != nil {
					return output.Validation("SKILL_GUIDANCE_INPUT_INVALID", "--market must be cn or global")
				}
				runtime.region = region
			}
			raw, err := os.ReadFile(inputPath)
			if err != nil {
				return output.Validation("SKILL_GUIDANCE_INPUT_INVALID", "could not read the task JSON file")
			}
			var input api.SkillGuidanceSubmit
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.DisallowUnknownFields()
			if len(raw) > 1024*1024 || decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
				return output.Validation("SKILL_GUIDANCE_INPUT_INVALID", "task input must be one JSON object with productId, requestKey, a complete prompt and optional facts")
			}
			if input.ProductID == "" {
				input.ProductID = productID
			}
			if productID != "" && input.ProductID != productID {
				return output.Validation("SKILL_GUIDANCE_INPUT_INVALID", "--product does not match the task input")
			}
			if input.Facts == nil {
				input.Facts = map[string]string{}
			}
			if err := completeGuidanceInput(runtime, &input, agent); err != nil {
				return err
			}
			if err := validateGuidanceInput(input); err != nil {
				return err
			}
			if wait < 0 || wait > 10*time.Minute {
				return output.Validation("SKILL_GUIDANCE_WAIT_INVALID", "--wait must be between 0 and 10m")
			}
			result, err := runGuidanceTask(command.Context(), runtime, input, wait, agent)
			if err != nil && isGuidancePaymentRequired(err) {
				raw, readErr := os.ReadFile(filepath.Join(guidanceTaskDirectory(runtime, input.ProductID), input.RequestKey+".json"))
				var record guidanceTaskRecord
				if readErr != nil || json.Unmarshal(raw, &record) != nil {
					return err
				}
				if strings.HasPrefix(record.Principal, "trial:") || strings.HasPrefix(record.Principal, "purchase:") {
					if purchaseErr := checkGuidanceAccountPurchase(command.Context(), runtime, input); purchaseErr != nil {
						return purchaseErr
					}
					return runTrialPurchase(command.Context(), runtime, input.ProductID, 0, agent, runtime.deps.Environment.InstallDirectory)
				}
				return presentRegisteredGuidancePurchase(command.Context(), runtime, input, agent)
			}
			if err != nil {
				return err
			}
			return runtime.business(result)
		}}
	command.Flags().StringVar(&skillDirectory, "skill-dir", "", "exact installed Skill directory")
	command.Flags().StringVar(&inputPath, "input", "", "task JSON file; keep the same UUID requestKey and input on retry")
	command.Flags().StringVar(&productID, "product", "", "product UUID when omitted from the task file")
	command.Flags().StringVar(&market, "market", "", "market from the installed runtime (cn or global)")
	command.Flags().StringVar(&agent, "agent", "auto", "host used by payment presentation")
	command.Flags().DurationVar(&wait, "wait", 60*time.Second, "bounded task wait (0 to 10m); timeout preserves the original request")
	_ = command.MarkFlagRequired("input")
	return command
}

func validateGuidanceInput(input api.SkillGuidanceSubmit) error {
	tooLarge := utf8.RuneCountInString(input.Prompt) > 8000 || len(input.Facts) > 30
	for key, value := range input.Facts {
		tooLarge = tooLarge || utf8.RuneCountInString(key) > 80 || utf8.RuneCountInString(value) > 4000
	}
	if tooLarge {
		return output.Validation("SKILL_GUIDANCE_INPUT_TOO_LARGE", "prompt allows 8000 characters; facts allow 30 keys of 80 characters and values of 4000 characters").WithHint("The local Agent must condense the complete task and submit a new requestKey; do not retry the unchanged oversized input")
	}
	valid := skillUseProductIDPattern.MatchString(input.ReleaseID) && skillUseProductIDPattern.MatchString(input.ProductID) && skillUseProductIDPattern.MatchString(input.RequestKey)
	valid = valid && strings.TrimSpace(input.Prompt) != "" && utf8.RuneCountInString(input.Prompt) <= 8000 && len(input.Facts) <= 30
	for key, value := range input.Facts {
		valid = valid && strings.TrimSpace(key) != "" && utf8.RuneCountInString(key) <= 80 && utf8.RuneCountInString(value) <= 4000
	}
	if !valid {
		return output.Validation("SKILL_GUIDANCE_INPUT_INVALID", "provide UUID productId/releaseId/requestKey, a prompt of 1–8000 characters, and at most 30 facts (key 1–80, value at most 4000 characters)")
	}
	return nil
}

func guidanceTaskDirectory(runtime *Runtime, productID string) string {
	digest := sha256.Sum256([]byte(strings.TrimRight(runtime.apiBaseURL, "/") + "\x00" + productID))
	return filepath.Join(runtime.deps.Environment.Home, ".viceme", "guidance", fmt.Sprintf("%x", digest[:]))
}

func guidancePrincipal(ctx context.Context, runtime *Runtime, input api.SkillGuidanceSubmit, agent string) (string, skillTrialCredential, error) {
	credential, exists, err := trialPurchaseCredential(runtime, input.ProductID)
	if err != nil {
		return "", credential, err
	}
	purchase, purchaseExists, purchaseErr := readScriptPurchaseState(runtime, input.ProductID)
	if purchaseErr != nil {
		return "", credential, purchaseErr
	}
	anonymousIdentityAvailable := purchaseExists || exists
	if purchaseExists && exists {
		return "", credential, output.Policy("PURCHASE_IDENTITY_CONFLICT", "preserve both anonymous identities; no task was submitted")
	}
	savedPrincipal := ""
	raw, readErr := os.ReadFile(filepath.Join(guidanceTaskDirectory(runtime, input.ProductID), input.RequestKey+".json"))
	if readErr == nil {
		var record guidanceTaskRecord
		if json.Unmarshal(raw, &record) != nil {
			return "", credential, output.Policy("SKILL_GUIDANCE_STATE_INVALID", "guidance recovery record is invalid; preserve it")
		}
		savedPrincipal = record.Principal
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return "", credential, readErr
	}
	if strings.HasPrefix(savedPrincipal, "purchase:") {
		if !purchaseExists || savedPrincipal != "purchase:"+purchase.InstallID {
			return "", credential, output.Policy("SKILL_GUIDANCE_REQUEST_CONFLICT", "restore the original purchase identity before retrying")
		}
		return savedPrincipal, skillTrialCredential{InstallID: purchase.InstallID, Secret: purchase.Secret}, nil
	}
	if strings.HasPrefix(savedPrincipal, "trial:") {
		if !exists || savedPrincipal != "trial:"+credential.InstallID {
			return "", credential, output.Policy("SKILL_GUIDANCE_REQUEST_CONFLICT", "restore the original task credentials before retrying")
		}
		return savedPrincipal, credential, nil
	}
	if savedPrincipal == "" && !exists && !purchaseExists {
		purchase, purchaseExists, err = ensureGuidancePurchaseIdentity(runtime, input.ProductID, agent)
		if err != nil {
			return "", credential, err
		}
	}
	if runtimeHasAuthentication(runtime) {
		_, overrideSource, _ := runtime.overrideCredential()
		canUseAnonymous := savedPrincipal == "" && anonymousIdentityAvailable && overrideSource == ""
		useAnonymous := func() (string, skillTrialCredential, error) {
			if purchaseExists {
				return "purchase:" + purchase.InstallID, skillTrialCredential{InstallID: purchase.InstallID, Secret: purchase.Secret}, nil
			}
			return "trial:" + credential.InstallID, credential, nil
		}
		status, err := runtime.client().AuthStatus(ctx)
		if err != nil {
			// Only a definite authentication refusal permits an already-existing
			// anonymous identity. Network errors and explicit credentials remain errors.
			if canUseAnonymous && output.AsError(err).Type == "authentication" {
				return useAnonymous()
			}
			return "", credential, err
		}
		if !status.Authenticated {
			if canUseAnonymous {
				return useAnonymous()
			}
			return "", credential, output.Authentication("NOT_LOGGED_IN", "sign in to execute this protected Skill")
		}
		if status.User.ID == "" {
			return "", credential, output.Policy("SKILL_GUIDANCE_RESPONSE_INVALID", "authenticated account has no identity")
		}
		if savedPrincipal != "" && savedPrincipal != "user:"+status.User.ID {
			return "", credential, output.Policy("SKILL_GUIDANCE_REQUEST_CONFLICT", "restore the original task account before retrying").WithHint("to use a different account, explicitly submit the complete task with a new requestKey")
		}
		if savedPrincipal == "" {
			access, err := runtime.client().GetSkillAccess(ctx, input.ProductID)
			if err != nil {
				return "", credential, err
			}
			if !access.Owned && purchaseExists {
				return "purchase:" + purchase.InstallID, skillTrialCredential{InstallID: purchase.InstallID, Secret: purchase.Secret}, nil
			}
			if !access.Owned && (exists || access.Trial != nil && access.Trial.Available) {
				if !exists {
					_, credential, err = ensureSkillTrialGrant(ctx, runtime, input.ProductID)
					if err != nil {
						return "", credential, err
					}
				}
				return "trial:" + credential.InstallID, credential, nil
			}
		}
		return "user:" + status.User.ID, skillTrialCredential{}, nil
	}
	if strings.HasPrefix(savedPrincipal, "user:") {
		return "", credential, output.Authentication("NOT_LOGGED_IN", "sign in as the original guidance task user to retry")
	}
	if purchaseExists {
		return "purchase:" + purchase.InstallID, skillTrialCredential{InstallID: purchase.InstallID, Secret: purchase.Secret}, nil
	}
	if exists {
		return "trial:" + credential.InstallID, credential, nil
	}
	_, credential, err = ensureSkillTrialGrant(ctx, runtime, input.ProductID)
	if err != nil {
		return "", credential, err
	}
	return "trial:" + credential.InstallID, credential, nil
}

func writeGuidanceRecord(filename string, record guidanceTaskRecord) error {
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return privatefile.WriteAtomic(filename, raw, ".guidance-*")
}

func prepareGuidanceRecord(runtime *Runtime, input api.SkillGuidanceSubmit, principal string) (guidanceTaskRecord, string, error) {
	record := guidanceTaskRecord{SchemaVersion: 1, APIBaseURL: strings.TrimRight(runtime.apiBaseURL, "/"), Market: string(runtime.region), Principal: principal, Input: input}
	filename := filepath.Join(guidanceTaskDirectory(runtime, input.ProductID), input.RequestKey+".json")
	err := withScriptTrialLock(runtime, input.ProductID, func() error {
		raw, err := os.ReadFile(filename)
		if err == nil {
			var saved guidanceTaskRecord
			if json.Unmarshal(raw, &saved) != nil || saved.SchemaVersion != 1 || saved.APIBaseURL != record.APIBaseURL || saved.Market != record.Market || saved.Principal != record.Principal || !reflect.DeepEqual(saved.Input, record.Input) {
				return output.Policy("SKILL_GUIDANCE_REQUEST_CONFLICT", "this request key already belongs to different input or credentials; preserve it and use the original task")
			}
			record = saved
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
			return err
		}
		return writeGuidanceRecord(filename, record)
	})
	return record, filename, err
}

func isGuidancePaymentRequired(err error) bool {
	code := output.AsError(err).Subtype
	return code == "SKILL_GUIDANCE_TRIAL_EXHAUSTED" || code == "SKILL_GUIDANCE_ENTITLEMENT_REQUIRED"
}

func runGuidanceTask(ctx context.Context, runtime *Runtime, input api.SkillGuidanceSubmit, wait time.Duration, agents ...string) (map[string]any, error) {
	agent := "auto"
	if len(agents) > 0 {
		agent = agents[0]
	}
	if err := completeGuidanceInput(runtime, &input, agent); err != nil {
		return nil, err
	}
	principal, credential, err := guidancePrincipal(ctx, runtime, input, agent)
	if err != nil {
		return nil, err
	}
	record, filename, err := prepareGuidanceRecord(runtime, input, principal)
	if err != nil {
		return nil, err
	}
	inputPath := strings.TrimSuffix(filename, ".json") + ".input.json"
	rawInput, _ := json.Marshal(input)
	if err := privatefile.WriteAtomic(inputPath, rawInput, ".guidance-input-*"); err != nil {
		return nil, err
	}
	result, err := runtime.client().SubmitSkillGuidance(ctx, input, credential.InstallID, credential.Secret, strings.SplitN(principal, ":", 2)[0])
	if err != nil {
		if isGuidancePaymentRequired(err) {
			record.PaymentRequired = true
			if saveErr := withScriptTrialLock(runtime, input.ProductID, func() error { return writeGuidanceRecord(filename, record) }); saveErr != nil {
				return nil, saveErr
			}
		}
		return nil, err
	}
	if result.ReleaseID != input.ReleaseID {
		return nil, output.Policy("SKILL_GUIDANCE_RESPONSE_INVALID", "guidance response changed the task version")
	}
	if err := withScriptTrialLock(runtime, input.ProductID, func() error { return writeGuidanceRecord(filename, record) }); err != nil {
		return nil, err
	}
	deadline := runtime.deps.Now().Add(wait)
	for (result.Status == "QUEUED" || result.Status == "RUNNING") && runtime.deps.Now().Before(deadline) {
		if err := runtime.deps.Sleep(ctx, min(time.Second, deadline.Sub(runtime.deps.Now()))); err != nil {
			return nil, err
		}
		next, err := runtime.client().ReadSkillGuidance(ctx, input.ProductID, result.RequestID, credential.InstallID, credential.Secret, false, strings.SplitN(principal, ":", 2)[0])
		if err != nil {
			return nil, err
		}
		if next.RequestID != result.RequestID || next.ReleaseID != result.ReleaseID || next.Version != result.Version {
			return nil, output.Policy("SKILL_GUIDANCE_RESPONSE_INVALID", "guidance response changed the task identity")
		}
		result = next
	}
	record.PaymentRequired = false
	if err := withScriptTrialLock(runtime, input.ProductID, func() error { return writeGuidanceRecord(filename, record) }); err != nil {
		return nil, err
	}
	data := map[string]any{"productId": input.ProductID, "requestKey": input.RequestKey, "deliveryMode": "PROTECTED", "task": result, "inputPath": inputPath, "allowed": false, "nextAction": "RETRY_SAME_TASK"}
	if result.Status == "SUCCEEDED" && result.Outcome != nil {
		switch *result.Outcome {
		case "ready":
			if err := completeGuidanceInput(runtime, &input, agent); err != nil {
				return nil, err
			}
			execution := strings.TrimSuffix(filename, ".json") + ".execution.md"
			if err := privatefile.WriteAtomic(execution, []byte(*result.Instructions), ".execution-*"); err != nil {
				return nil, err
			}
			data["allowed"], data["nextAction"], data["executionPath"] = true, "EXECUTE_GUIDANCE_INSTRUCTIONS", execution
		case "needs_input":
			data["nextAction"] = "SUPPLY_COMPLETE_INPUT_WITH_NEW_KEY"
		case "refused":
			data["nextAction"] = "TASK_REFUSED"
		}
	}
	if result.Status == "FAILED" && !result.Retryable {
		data["nextAction"] = "TASK_FAILED"
		if result.ErrorCode != nil && *result.ErrorCode == "MODEL_INPUT_TOO_LARGE" {
			data["nextAction"] = "REDUCE_COMPLETE_INPUT_WITH_NEW_KEY"
		}
	}
	return data, nil
}

func resumeGuidanceAfterPurchase(ctx context.Context, runtime *Runtime, productID string, agents ...string) (map[string]any, error) {
	entries, err := os.ReadDir(guidanceTaskDirectory(runtime, productID))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	results := []map[string]any{}
	appendFailure := func(name string, err error) {
		failure := output.AsError(err)
		nextAction := "RETRY_SAME_TASK"
		if failure.Subtype == "SKILL_GUIDANCE_REQUEST_ENTITLEMENT_REQUIRED" {
			nextAction = "SUBMIT_COMPLETE_INPUT_WITH_NEW_KEY"
		}
		results = append(results, map[string]any{"requestKey": strings.TrimSuffix(name, ".json"), "allowed": false, "nextAction": nextAction, "error": map[string]any{"code": failure.Subtype, "message": failure.Message, "hint": failure.Hint, "requestId": failure.RequestID}})
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || strings.HasSuffix(entry.Name(), ".input.json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(guidanceTaskDirectory(runtime, productID), entry.Name()))
		if err != nil {
			appendFailure(entry.Name(), output.Policy("SKILL_GUIDANCE_STATE_INVALID", "guidance recovery record cannot be read; preserve it"))
			continue
		}
		var record guidanceTaskRecord
		if json.Unmarshal(raw, &record) != nil || record.SchemaVersion != 1 || record.Input.ProductID != productID || validateGuidanceInput(record.Input) != nil {
			appendFailure(entry.Name(), output.Policy("SKILL_GUIDANCE_STATE_INVALID", "guidance recovery record is invalid; preserve it"))
			continue
		}
		if !record.PaymentRequired {
			continue
		}
		result, err := runGuidanceTask(ctx, runtime, record.Input, 60*time.Second, agents...)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if isGuidancePaymentRequired(err) {
				if purchaseErr := checkGuidanceAccountPurchase(ctx, runtime, record.Input); purchaseErr != nil {
					err = purchaseErr
				}
			}
			appendFailure(entry.Name(), err)
			continue
		}
		results = append(results, result)
	}
	return map[string]any{"productId": productID, "deliveryMode": "PROTECTED", "owned": true, "allowed": false, "nextAction": "SUBMIT_GUIDANCE_TASK", "resumedTasks": results}, nil
}

// An account purchase cannot silently transfer an anonymous request.
// A payment-rejected task is already bound locally even before a server request exists.
func checkGuidanceAccountPurchase(ctx context.Context, runtime *Runtime, input api.SkillGuidanceSubmit) error {
	if !runtimeHasAuthentication(runtime) {
		return nil
	}
	status, err := runtime.client().AuthStatus(ctx)
	if err != nil {
		if output.AsError(err).Type == "authentication" {
			return nil
		}
		return err
	}
	if !status.Authenticated {
		return nil
	}
	access, err := runtime.client().GetSkillAccess(ctx, input.ProductID)
	if err != nil {
		return err
	}
	if access.Owned {
		return output.Policy("SKILL_GUIDANCE_REQUEST_ENTITLEMENT_REQUIRED", "the original task identity has no remaining access; the current account purchase belongs to another identity").WithHint("explicitly start a new task using the current account, a new requestKey containing the complete task; preserve the original task record")
	}
	return nil
}

func presentRegisteredGuidancePurchase(ctx context.Context, runtime *Runtime, input api.SkillGuidanceSubmit, agent string) error {
	if err := runtime.requireBuyerAuthentication(ctx); err != nil {
		return err
	}
	order, err := openSkillPurchaseOrder(ctx, runtime, input.ProductID)
	if err != nil {
		return err
	}
	if order.Status == "PAID" {
		access, err := runtime.client().GetSkillAccess(ctx, input.ProductID)
		if err != nil {
			return err
		}
		if !access.Owned {
			return output.Policy("SKILL_PURCHASE_PENDING", "payment entitlement is not yet available; retry the same guidance task")
		}
		result, err := runGuidanceTask(ctx, runtime, input, 60*time.Second, agent)
		if err != nil {
			return err
		}
		return runtime.business(result)
	}
	presentation, err := presentSkillPaymentQR(runtime, &order)
	if err != nil {
		return err
	}
	return output.Confirmation("SKILL_PURCHASE_REQUIRED", "complete this purchase, then retry the same guidance task").WithDetails(map[string]any{"productId": input.ProductID, "requestKey": input.RequestKey, "orderNo": order.OrderNo, "paymentUrl": skillOrderPaymentURL(runtime, order.OrderNo), "paymentPresentation": presentation}).WithHint("present the payment URL and QR; after payment rerun guidance with the original input file and UUID requestKey")
}

func completeGuidanceInput(runtime *Runtime, input *api.SkillGuidanceSubmit, agent string) error {
	if input.ProductID == "" {
		raw, err := os.ReadFile(filepath.Join(guidanceWorkingDirectory(), ".viceme", "runtime.json"))
		var manifest skillcontent.RuntimeManifest
		if err == nil && json.Unmarshal(raw, &manifest) == nil && manifest.DeliveryMode == "PROTECTED" && manifest.APIBaseURL == runtime.apiBaseURL {
			input.ProductID = manifest.ProductID
		}
	}
	if !skillUseProductIDPattern.MatchString(input.ProductID) || !skillUseProductIDPattern.MatchString(input.RequestKey) {
		return output.Validation("SKILL_GUIDANCE_INPUT_INVALID", "provide a valid productId and requestKey")
	}
	if input.ReleaseID == "" {
		raw, err := os.ReadFile(filepath.Join(guidanceTaskDirectory(runtime, input.ProductID), input.RequestKey+".json"))
		if err == nil {
			var record guidanceTaskRecord
			if json.Unmarshal(raw, &record) == nil {
				input.ReleaseID = record.Input.ReleaseID
			}
		}
	}
	directory, manifest, found, err := skillcontent.FindRuntimeInstall(runtime.deps.Environment, agent, input.ProductID, runtime.apiBaseURL, runtime.deps.Environment.InstallDirectory)
	if err != nil {
		return err
	}
	if found && directory == "" {
		return output.Policy("SKILL_GUIDANCE_RELEASE_MISMATCH", "the installed runtime cannot verify this task version; repair the matching installation before retrying")
	}
	if directory != "" {
		if manifest.DeliveryMode != "PROTECTED" {
			return output.Policy("SKILL_GUIDANCE_INSTALL_REQUIRED", "install this protected Skill before executing a task")
		}
		if input.ReleaseID == "" {
			input.ReleaseID = manifest.ReleaseID
		}
		if input.ReleaseID != manifest.ReleaseID {
			return output.Policy("SKILL_GUIDANCE_RELEASE_MISMATCH", "task release differs from the installed public files; restore the matching installation before retrying")
		}
	}
	if input.ReleaseID == "" {
		return output.Validation("SKILL_GUIDANCE_INPUT_INVALID", "install the protected Skill or provide its exact releaseId; the active release is never inferred")
	}
	return nil
}

func guidanceWorkingDirectory() string { directory, _ := os.Getwd(); return directory }
