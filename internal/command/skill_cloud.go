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
type cloudTaskRecord struct {
	SchemaVersion   int                  `json:"schemaVersion"`
	APIBaseURL      string               `json:"apiBaseUrl"`
	Market          string               `json:"market"`
	Principal       string               `json:"principal"`
	Input           api.SkillCloudSubmit `json:"input"`
	PaymentRequired bool                 `json:"paymentRequired"`
}

func cloudResources(resources localSkillResources, mode string) localSkillResources {
	resources.DeliveryMode = api.DeliveryMode(mode)
	return resources
}

func newSkillCloudCommand(runtime *Runtime) *cobra.Command {
	var inputPath, productID, agent, market string
	var wait time.Duration
	command := &cobra.Command{Use: "cloud --input <task.json>", Short: "Run a cloud Skill task using an immutable UUID request key; retry the same file after an interrupted response", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			if market != "" {
				region, err := config.ParseRegion(market)
				if err != nil {
					return output.Validation("SKILL_CLOUD_INPUT_INVALID", "--market must be cn or global")
				}
				runtime.region = region
			}
			raw, err := os.ReadFile(inputPath)
			if err != nil {
				return output.Validation("SKILL_CLOUD_INPUT_INVALID", "could not read the task JSON file")
			}
			var input api.SkillCloudSubmit
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.DisallowUnknownFields()
			if len(raw) > 1024*1024 || decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
				return output.Validation("SKILL_CLOUD_INPUT_INVALID", "task input must be one JSON object with productId, requestKey, optional sessionId, prompt and facts")
			}
			if input.ProductID == "" {
				input.ProductID = productID
			}
			if productID != "" && input.ProductID != productID {
				return output.Validation("SKILL_CLOUD_INPUT_INVALID", "--product does not match the task input")
			}
			if input.Facts == nil {
				input.Facts = map[string]string{}
			}
			if err := completeCloudInput(runtime, &input, agent); err != nil {
				return err
			}
			if err := validateCloudInput(input); err != nil {
				return err
			}
			if wait < 0 || wait > 10*time.Minute {
				return output.Validation("SKILL_CLOUD_WAIT_INVALID", "--wait must be between 0 and 10m")
			}
			result, err := runCloudTask(command.Context(), runtime, input, wait, agent)
			if err != nil && isCloudPaymentRequired(err) {
				raw, readErr := os.ReadFile(filepath.Join(cloudTaskDirectory(runtime, input.ProductID), input.RequestKey+".json"))
				var record cloudTaskRecord
				if readErr != nil || json.Unmarshal(raw, &record) != nil {
					return err
				}
				if strings.HasPrefix(record.Principal, "trial:") {
					return runTrialPurchase(command.Context(), runtime, input.ProductID, 0, agent)
				}
				return presentRegisteredCloudPurchase(command.Context(), runtime, input, agent)
			}
			if err != nil {
				return err
			}
			return runtime.business(result)
		}}
	command.Flags().StringVar(&inputPath, "input", "", "task JSON file; keep the same UUID requestKey and input on retry")
	command.Flags().StringVar(&productID, "product", "", "product UUID when omitted from the task file")
	command.Flags().StringVar(&market, "market", "", "market from the installed runtime (cn or global)")
	command.Flags().StringVar(&agent, "agent", "auto", "host used by payment presentation")
	command.Flags().DurationVar(&wait, "wait", 60*time.Second, "bounded task wait (0 to 10m); timeout preserves the original request")
	_ = command.MarkFlagRequired("input")
	return command
}

func validateCloudInput(input api.SkillCloudSubmit) error {
	valid := skillUseProductIDPattern.MatchString(input.ReleaseID) && skillUseProductIDPattern.MatchString(input.ProductID) && skillUseProductIDPattern.MatchString(input.RequestKey) && (input.SessionID == "" || skillUseProductIDPattern.MatchString(input.SessionID))
	valid = valid && strings.TrimSpace(input.Prompt) != "" && utf8.RuneCountInString(input.Prompt) <= 8000 && len(input.Facts) <= 30
	for key, value := range input.Facts {
		valid = valid && strings.TrimSpace(key) != "" && utf8.RuneCountInString(key) <= 80 && utf8.RuneCountInString(value) <= 4000
	}
	if !valid {
		return output.Validation("SKILL_CLOUD_INPUT_INVALID", "provide UUID productId/requestKey/sessionId, a prompt of 1–8000 characters, and at most 30 facts (key 1–80, value at most 4000 characters)")
	}
	return nil
}

func cloudTaskDirectory(runtime *Runtime, productID string) string {
	digest := sha256.Sum256([]byte(strings.TrimRight(runtime.apiBaseURL, "/") + "\x00" + productID))
	return filepath.Join(runtime.deps.Environment.Home, ".viceme", "cloud", fmt.Sprintf("%x", digest[:]))
}

func cloudPrincipal(ctx context.Context, runtime *Runtime, input api.SkillCloudSubmit) (string, skillTrialCredential, error) {
	credential, exists, err := trialPurchaseCredential(runtime, input.ProductID)
	if err != nil {
		return "", credential, err
	}
	savedPrincipal := ""
	raw, readErr := os.ReadFile(filepath.Join(cloudTaskDirectory(runtime, input.ProductID), input.RequestKey+".json"))
	if readErr == nil {
		var record cloudTaskRecord
		if json.Unmarshal(raw, &record) != nil {
			return "", credential, output.Policy("SKILL_CLOUD_STATE_INVALID", "cloud recovery record is invalid; preserve it")
		}
		savedPrincipal = record.Principal
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return "", credential, readErr
	}
	if strings.HasPrefix(savedPrincipal, "trial:") {
		if !exists || savedPrincipal != "trial:"+credential.InstallID {
			return "", credential, output.Policy("SKILL_CLOUD_REQUEST_CONFLICT", "restore the original task credentials before retrying")
		}
		return savedPrincipal, credential, nil
	}
	if runtimeHasAuthentication(runtime) {
		status, err := runtime.client().AuthStatus(ctx)
		if err != nil {
			return "", credential, err
		}
		if !status.Authenticated || status.User.ID == "" {
			return "", credential, output.Authentication("NOT_LOGGED_IN", "sign in to execute this cloud Skill")
		}
		if savedPrincipal == "" {
			access, err := runtime.client().GetSkillAccess(ctx, input.ProductID)
			if err != nil {
				return "", credential, err
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
		return "", credential, output.Authentication("NOT_LOGGED_IN", "sign in as the original cloud task user to retry")
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

func writeCloudRecord(filename string, record cloudTaskRecord) error {
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return privatefile.WriteAtomic(filename, raw, ".cloud-*")
}

func prepareCloudRecord(runtime *Runtime, input api.SkillCloudSubmit, principal string) (cloudTaskRecord, string, error) {
	record := cloudTaskRecord{SchemaVersion: 1, APIBaseURL: strings.TrimRight(runtime.apiBaseURL, "/"), Market: string(runtime.region), Principal: principal, Input: input}
	filename := filepath.Join(cloudTaskDirectory(runtime, input.ProductID), input.RequestKey+".json")
	err := withScriptTrialLock(runtime, input.ProductID, func() error {
		raw, err := os.ReadFile(filename)
		if err == nil {
			var saved cloudTaskRecord
			if json.Unmarshal(raw, &saved) != nil || saved.SchemaVersion != 1 || saved.APIBaseURL != record.APIBaseURL || saved.Market != record.Market || saved.Principal != record.Principal || !reflect.DeepEqual(saved.Input, record.Input) {
				return output.Policy("SKILL_CLOUD_REQUEST_CONFLICT", "this request key already belongs to different input or credentials; preserve it and use the original task")
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
		return writeCloudRecord(filename, record)
	})
	return record, filename, err
}

func isCloudPaymentRequired(err error) bool {
	code := output.AsError(err).Subtype
	return code == "SKILL_CLOUD_TRIAL_EXHAUSTED" || code == "SKILL_CLOUD_ENTITLEMENT_REQUIRED"
}

func runCloudTask(ctx context.Context, runtime *Runtime, input api.SkillCloudSubmit, wait time.Duration, agents ...string) (map[string]any, error) {
	agent := "auto"
	if len(agents) > 0 {
		agent = agents[0]
	}
	if err := completeCloudInput(runtime, &input, agent); err != nil {
		return nil, err
	}
	principal, credential, err := cloudPrincipal(ctx, runtime, input)
	if err != nil {
		return nil, err
	}
	record, filename, err := prepareCloudRecord(runtime, input, principal)
	if err != nil {
		return nil, err
	}
	inputPath := strings.TrimSuffix(filename, ".json") + ".input.json"
	rawInput, _ := json.Marshal(input)
	if err := privatefile.WriteAtomic(inputPath, rawInput, ".cloud-input-*"); err != nil {
		return nil, err
	}
	result, err := runtime.client().SubmitSkillCloud(ctx, input, credential.InstallID, credential.Secret)
	if err != nil {
		if isCloudPaymentRequired(err) {
			record.PaymentRequired = true
			if saveErr := withScriptTrialLock(runtime, input.ProductID, func() error { return writeCloudRecord(filename, record) }); saveErr != nil {
				return nil, saveErr
			}
		}
		return nil, err
	}
	if result.ReleaseID != input.ReleaseID || input.SessionID != "" && result.SessionID != input.SessionID {
		return nil, output.Policy("SKILL_CLOUD_RESPONSE_INVALID", "cloud response changed the task session")
	}
	deadline := runtime.deps.Now().Add(wait)
	for (result.Status == "QUEUED" || result.Status == "RUNNING") && runtime.deps.Now().Before(deadline) {
		if err := runtime.deps.Sleep(ctx, min(time.Second, deadline.Sub(runtime.deps.Now()))); err != nil {
			return nil, err
		}
		next, err := runtime.client().ReadSkillCloud(ctx, input.ProductID, result.RequestID, credential.InstallID, credential.Secret, false)
		if err != nil {
			return nil, err
		}
		if next.RequestID != result.RequestID || next.SessionID != result.SessionID || next.ReleaseID != result.ReleaseID || next.Version != result.Version {
			return nil, output.Policy("SKILL_CLOUD_RESPONSE_INVALID", "cloud response changed the task identity")
		}
		result = next
	}
	record.PaymentRequired = false
	if err := withScriptTrialLock(runtime, input.ProductID, func() error { return writeCloudRecord(filename, record) }); err != nil {
		return nil, err
	}
	data := map[string]any{"productId": input.ProductID, "requestKey": input.RequestKey, "deliveryMode": "CLOUD", "task": result, "inputPath": inputPath, "allowed": false, "nextAction": "RETRY_SAME_TASK"}
	if result.Status == "SUCCEEDED" && result.Outcome != nil {
		switch *result.Outcome {
		case "ready":
			if err := completeCloudInput(runtime, &input, agent); err != nil {
				return nil, err
			}
			execution := strings.TrimSuffix(filename, ".json") + ".execution.md"
			if err := privatefile.WriteAtomic(execution, []byte(*result.Instructions), ".execution-*"); err != nil {
				return nil, err
			}
			data["allowed"], data["nextAction"], data["executionPath"] = true, "EXECUTE_CLOUD_INSTRUCTIONS", execution
		case "needs_input":
			data["nextAction"] = "SUPPLY_INPUT_WITH_NEW_KEY_AND_SAME_SESSION"
		case "refused":
			data["nextAction"] = "TASK_REFUSED"
		}
	}
	if result.Status == "FAILED" && !result.Retryable {
		data["nextAction"] = "TASK_FAILED"
	}
	return data, nil
}

func resumeCloudAfterPurchase(ctx context.Context, runtime *Runtime, productID string, agents ...string) (map[string]any, error) {
	entries, err := os.ReadDir(cloudTaskDirectory(runtime, productID))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	results := []map[string]any{}
	appendFailure := func(name string, err error) {
		failure := output.AsError(err)
		results = append(results, map[string]any{"requestKey": strings.TrimSuffix(name, ".json"), "allowed": false, "nextAction": "RETRY_SAME_TASK", "error": map[string]any{"code": failure.Subtype, "message": failure.Message, "requestId": failure.RequestID}})
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || strings.HasSuffix(entry.Name(), ".input.json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(cloudTaskDirectory(runtime, productID), entry.Name()))
		if err != nil {
			appendFailure(entry.Name(), output.Policy("SKILL_CLOUD_STATE_INVALID", "cloud recovery record cannot be read; preserve it"))
			continue
		}
		var record cloudTaskRecord
		if json.Unmarshal(raw, &record) != nil || record.SchemaVersion != 1 || record.Input.ProductID != productID || validateCloudInput(record.Input) != nil {
			appendFailure(entry.Name(), output.Policy("SKILL_CLOUD_STATE_INVALID", "cloud recovery record is invalid; preserve it"))
			continue
		}
		if !record.PaymentRequired {
			continue
		}
		result, err := runCloudTask(ctx, runtime, record.Input, 60*time.Second, agents...)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			appendFailure(entry.Name(), err)
			continue
		}
		results = append(results, result)
	}
	return map[string]any{"productId": productID, "deliveryMode": "CLOUD", "owned": true, "allowed": false, "nextAction": "SUBMIT_CLOUD_TASK", "resumedTasks": results}, nil
}

func presentRegisteredCloudPurchase(ctx context.Context, runtime *Runtime, input api.SkillCloudSubmit, agent string) error {
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
			return output.Policy("SKILL_PURCHASE_PENDING", "payment entitlement is not yet available; retry the same cloud task")
		}
		result, err := runCloudTask(ctx, runtime, input, 60*time.Second, agent)
		if err != nil {
			return err
		}
		return runtime.business(result)
	}
	presentation, err := presentSkillPaymentQR(runtime, &order)
	if err != nil {
		return err
	}
	return output.Confirmation("SKILL_PURCHASE_REQUIRED", "complete this purchase, then retry the same cloud task").WithDetails(map[string]any{"productId": input.ProductID, "requestKey": input.RequestKey, "orderNo": order.OrderNo, "paymentUrl": skillOrderPaymentURL(runtime, order.OrderNo), "paymentPresentation": presentation}).WithHint("present the payment URL and QR; after payment rerun cloud with the original input file and UUID requestKey")
}

func completeCloudInput(runtime *Runtime, input *api.SkillCloudSubmit, agent string) error {
	if input.ProductID == "" {
		raw, err := os.ReadFile(filepath.Join(cloudWorkingDirectory(), ".viceme", "runtime.json"))
		var manifest skillcontent.RuntimeManifest
		if err == nil && json.Unmarshal(raw, &manifest) == nil && manifest.DeliveryMode == "CLOUD" && manifest.APIBaseURL == runtime.apiBaseURL {
			input.ProductID = manifest.ProductID
		}
	}
	if !skillUseProductIDPattern.MatchString(input.ProductID) || !skillUseProductIDPattern.MatchString(input.RequestKey) {
		return output.Validation("SKILL_CLOUD_INPUT_INVALID", "provide a valid productId and requestKey")
	}
	if input.ReleaseID == "" {
		raw, err := os.ReadFile(filepath.Join(cloudTaskDirectory(runtime, input.ProductID), input.RequestKey+".json"))
		if err == nil {
			var record cloudTaskRecord
			if json.Unmarshal(raw, &record) == nil {
				input.ReleaseID = record.Input.ReleaseID
			}
		}
	}
	directory, manifest, found, err := skillcontent.FindRuntimeInstall(runtime.deps.Environment, agent, input.ProductID, runtime.apiBaseURL)
	if err != nil {
		return err
	}
	if found && directory == "" {
		return output.Policy("SKILL_CLOUD_RELEASE_MISMATCH", "the installed runtime cannot verify this task version; repair the matching installation before retrying")
	}
	if directory != "" {
		if manifest.DeliveryMode != "CLOUD" {
			return output.Policy("SKILL_CLOUD_INSTALL_REQUIRED", "install this cloud Skill before executing a task")
		}
		if input.ReleaseID == "" {
			input.ReleaseID = manifest.ReleaseID
		}
		if input.ReleaseID != manifest.ReleaseID {
			return output.Policy("SKILL_CLOUD_RELEASE_MISMATCH", "task release differs from the installed public files; restore the matching installation before retrying")
		}
	}
	if input.ReleaseID == "" {
		return output.Validation("SKILL_CLOUD_INPUT_INVALID", "install the cloud Skill or provide its exact releaseId; the active release is never inferred")
	}
	return nil
}

func cloudWorkingDirectory() string { directory, _ := os.Getwd(); return directory }
