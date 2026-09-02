package command

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	"github.com/spf13/cobra"
)

// Skill 试用付费:匿名试用授权、按次计数闸口与本地门禁段。
// 计数权威在服务端;本机只保存 (installId, secret)。installId 属于本机 CLI,
// 不随 Skill 卸载重置,重装继续计数;清空 CLI 凭证等价于换设备。

const skillTrialGateMarker = "<!-- viceme-trial:v1"

type skillTrialCredential struct {
	InstallID string `json:"installId"`
	Secret    string `json:"secret"`
}

func skillTrialStoreKey(productID string) string {
	return "skill-trial-grant:" + productID
}

func loadSkillTrialCredential(runtime *Runtime, productID string) (skillTrialCredential, bool, error) {
	raw, err := runtime.deps.Store.Get(skillTrialStoreKey(productID))
	if err != nil || raw == "" {
		return skillTrialCredential{}, false, nil
	}
	var credential skillTrialCredential
	if err := json.Unmarshal([]byte(raw), &credential); err != nil || credential.InstallID == "" || credential.Secret == "" {
		return skillTrialCredential{}, false, nil
	}
	return credential, true, nil
}

func saveSkillTrialCredential(runtime *Runtime, productID string, credential skillTrialCredential) error {
	encoded, err := json.Marshal(credential)
	if err != nil {
		return output.Internal("SKILL_TRIAL_CREDENTIAL_ENCODE_FAILED", "could not encode the local Skill trial credential", err)
	}
	return runtime.deps.Store.Set(skillTrialStoreKey(productID), string(encoded))
}

// ensureSkillTrialGrant returns a usable (grant, credential) pair for this
// machine. Reuse the stored installId so reinstalling never resets the count;
// when the local secret was lost but the server still knows the installId,
// fall back to a fresh installId (a fresh grant with a fresh secret).
func ensureSkillTrialGrant(ctx context.Context, runtime *Runtime, productID string) (api.SkillTrialGrant, skillTrialCredential, error) {
	stored, hasStored, err := loadSkillTrialCredential(runtime, productID)
	if err != nil {
		return api.SkillTrialGrant{}, skillTrialCredential{}, err
	}
	installID := runtime.deps.NewID()
	if hasStored {
		installID = stored.InstallID
	}
	grant, err := runtime.client().CreateSkillTrialGrant(ctx, productID, installID)
	if err != nil {
		return api.SkillTrialGrant{}, skillTrialCredential{}, err
	}
	if grant.Secret != nil && *grant.Secret != "" {
		credential := skillTrialCredential{InstallID: grant.InstallID, Secret: *grant.Secret}
		if err := saveSkillTrialCredential(runtime, productID, credential); err != nil {
			return api.SkillTrialGrant{}, skillTrialCredential{}, err
		}
		return grant, credential, nil
	}
	if hasStored {
		return grant, stored, nil
	}
	// 本地凭证丢失且服务端按旧 installId 幂等返回:换新 installId 重发。
	freshID := runtime.deps.NewID()
	grant, err = runtime.client().CreateSkillTrialGrant(ctx, productID, freshID)
	if err != nil {
		return api.SkillTrialGrant{}, skillTrialCredential{}, err
	}
	if grant.Secret == nil || *grant.Secret == "" {
		return api.SkillTrialGrant{}, skillTrialCredential{}, output.Internal("SKILL_TRIAL_GRANT_INVALID", "the trial grant response did not carry a secret", nil)
	}
	credential := skillTrialCredential{InstallID: grant.InstallID, Secret: *grant.Secret}
	if err := saveSkillTrialCredential(runtime, productID, credential); err != nil {
		return api.SkillTrialGrant{}, skillTrialCredential{}, err
	}
	return grant, credential, nil
}

// injectSkillTrialGate appends the pre-use gate section to the package's
// SKILL.md before installation. Agents must read SKILL.md before using a
// skill, so every use funnels through `viceme skill use`. The marker keeps the
// append idempotent.
func injectSkillTrialGate(files map[string]downloadableSkillFile, productID string) {
	manifest, exists := files["SKILL.md"]
	if !exists || strings.Contains(string(manifest.Data), skillTrialGateMarker) {
		return
	}
	section := fmt.Sprintf("\n\n%s product=%s -->\n\n## 试用版使用规则（viceme-trial）\n\n本技能当前是 ViceMe 试用版，必须遵守：\n\n1. 每次开始使用本技能前，必须先运行 `viceme skill use %s`，并严格按其输出继续；未取得放行不得开始使用。\n2. 输出显示剩余次数时，先用一句话告知用户剩余试用次数，然后继续当前任务。\n3. 输出显示试用已用完时，停止使用本技能，引导用户扫码付费；支付完成后运行 `viceme skill install %s` 转正，再继续任务。\n",
		skillTrialGateMarker, productID, productID, productID)
	data := make([]byte, 0, len(manifest.Data)+len(section))
	data = append(data, manifest.Data...)
	data = append(data, []byte(section)...)
	files["SKILL.md"] = downloadableSkillFile{Data: data, Mode: manifest.Mode}
}

// removeSkillTrialGates strips the gate section from every local installation
// of the given Product (matched through the provenance install manifest). The
// section was appended at the end of SKILL.md, so truncating at the marker is
// exact. Returns the cleaned skill directories.
func removeSkillTrialGates(runtime *Runtime, productID string) []string {
	environment := runtime.deps.Environment
	roots := []string{
		filepath.Join(environment.Home, ".codex", "skills"),
		filepath.Join(environment.Home, ".claude", "skills"),
		filepath.Join(environment.Home, ".workbuddy", "skills"),
		environment.AgentsSkillsDir,
	}
	if roots[3] == "" {
		roots[3] = filepath.Join(environment.Home, ".agents", "skills")
	}
	cleaned := []string{}
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			// 同名 Skill 会安装到多个根目录,每个根目录都要独立清理。
			skillDir := filepath.Join(root, entry.Name())
			if !trialGateBelongsToProduct(skillDir, productID) {
				continue
			}
			if stripTrialGateSection(filepath.Join(skillDir, "SKILL.md")) {
				cleaned = append(cleaned, skillDir)
			}
		}
	}
	return cleaned
}

func trialGateBelongsToProduct(skillDir, productID string) bool {
	manifestPath := filepath.Join(skillDir, ".viceme", "install-manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return false
	}
	manifest := struct {
		ProductID string `json:"product_id"`
	}{}
	if err := json.Unmarshal(raw, &manifest); err != nil || manifest.ProductID != productID {
		return false
	}
	skillMarkdown, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		return false
	}
	return strings.Contains(string(skillMarkdown), skillTrialGateMarker)
}

func stripTrialGateSection(skillMarkdownPath string) bool {
	raw, err := os.ReadFile(skillMarkdownPath)
	if err != nil {
		return false
	}
	content := string(raw)
	index := strings.Index(content, skillTrialGateMarker)
	if index < 0 {
		return false
	}
	trimmed := strings.TrimRight(content[:index], "\n \t") + "\n"
	info, err := os.Stat(skillMarkdownPath)
	if err != nil {
		return false
	}
	return os.WriteFile(skillMarkdownPath, []byte(trimmed), info.Mode().Perm()) == nil
}

type trialInstallSummary struct {
	InstallID     string `json:"installId"`
	LimitUses     int    `json:"limitUses"`
	RemainingUses int    `json:"remainingUses"`
}

// installTrialSkill is the anonymous trial path of `viceme skill install`:
// issue (or reuse) the machine grant, download the same release artifact,
// inject the pre-use gate, and install without login or payment.
func installTrialSkill(ctx context.Context, runtime *Runtime, productID string, workSlug, agentTarget string, access api.SkillAccess) error {
	grant, credential, err := ensureSkillTrialGrant(ctx, runtime, productID)
	if err != nil {
		return err
	}
	download, err := runtime.client().GetTrialSkillDownload(ctx, productID, credential.InstallID)
	if err != nil {
		return err
	}
	if download.ReleaseID != access.Release.ID || download.ArtifactDigest != access.Release.ArtifactDigest {
		return output.Policy("SKILL_DOWNLOAD_RECEIPT_MISMATCH", "download authorization does not match the authorized Skill release")
	}
	artifact, err := runtime.client().DownloadArtifact(ctx, download.URL)
	if err != nil {
		return err
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(artifact))
	if digest != download.ArtifactDigest {
		return output.Policy("SKILL_ARTIFACT_DIGEST_MISMATCH", "downloaded Skill package does not match the active release")
	}
	files, err := extractDownloadableSkill(artifact)
	if err != nil {
		return err
	}
	injectSkillTrialGate(files, productID)
	manifestName, err := downloadableSkillManifestName(files)
	if err != nil {
		return err
	}
	installedName := downloadableSkillName(productID, manifestName, access.Edition.Title, workSlug)
	report, err := installDownloadableSkill(installedName, agentTarget, files, runtime.deps.Environment, skillcontent.SkillProvenance{
		ProductID: productID,
		ReleaseID: access.Release.ID,
	})
	if err != nil {
		return err
	}
	if !report.AllSucceeded {
		return output.Internal("SKILL_INSTALL_FAILED", "one or more Skill targets could not be installed", nil).WithDetails(map[string]any{"report": report})
	}
	return runtime.business(downloadableSkillInstallResult{
		ProductID: productID, Edition: access.Edition, ReleaseID: access.Release.ID, ArtifactDigest: digest,
		InstalledName: installedName, Install: report,
		NextAction: "CONTINUE_ORIGINAL_TASK_WITH_INSTALLED_SKILL", Invocation: "$" + installedName,
		Trial: &trialInstallSummary{InstallID: grant.InstallID, LimitUses: grant.LimitUses, RemainingUses: grant.RemainingUses},
	})
}

type skillTrialUseResult struct {
	ProductID     string   `json:"productId"`
	Allowed       bool     `json:"allowed"`
	Owned         bool     `json:"owned"`
	RemainingUses *int     `json:"remainingUses,omitempty"`
	LimitUses     *int     `json:"limitUses,omitempty"`
	LastUse       bool     `json:"lastUse"`
	RemovedGates  []string `json:"removedGates,omitempty"`
	OrderNo       string   `json:"orderNo,omitempty"`
	NextAction    string   `json:"nextAction"`
	Invocation    string   `json:"invocation,omitempty"`
}

// newSkillUsePrecheckCommand is the per-use gate: it consumes one trial use on
// the server and tells the agent whether to continue, and it closes the
// purchase loop (WeChat QR + wait) once the trial is exhausted. An already
// owned Skill short-circuits to gate removal.
func newSkillUsePrecheckCommand(runtime *Runtime) *cobra.Command {
	var wait time.Duration
	command := &cobra.Command{
		Use: "use <product-id-or-work-url>", Short: "Consume one trial use of a Skill edition and gate further use", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			productID, _, err := resolveSkillUseTarget(command.Context(), runtime, args[0])
			if err != nil {
				return err
			}
			// 已购短路:entitlement 生效后移除本地门禁,不再消耗试用次数。
			if runtimeHasAuthentication(runtime) {
				access, accessErr := runtime.client().GetSkillAccess(command.Context(), productID)
				if accessErr == nil && access.Owned {
					removed := removeSkillTrialGates(runtime, productID)
					return runtime.business(skillTrialUseResult{
						ProductID: productID, Allowed: true, Owned: true, RemovedGates: removed,
						NextAction: "CONTINUE_TASK",
					})
				}
			}
			credential, hasCredential, err := loadSkillTrialCredential(runtime, productID)
			if err != nil {
				return err
			}
			if !hasCredential {
				return output.Policy("SKILL_TRIAL_GRANT_MISSING", "this machine has no active trial grant for the Skill edition").WithDetails(map[string]any{"productId": productID}).WithHint("run 'viceme skill install <product-id-or-work-url>' first; a paid edition with a trial offer installs the trial without login")
			}
			use, err := runtime.client().ConsumeSkillTrialUse(command.Context(), productID, credential.InstallID, credential.Secret)
			if err != nil {
				return err
			}
			if use.Allowed {
				lastUse := use.RemainingUses != nil && *use.RemainingUses == 0
				return runtime.business(skillTrialUseResult{
					ProductID: productID, Allowed: true, RemainingUses: use.RemainingUses, LimitUses: use.LimitUses, LastUse: lastUse,
					NextAction: "CONTINUE_TASK",
				})
			}
			// 试用耗尽:购买需要登录,然后走既有扫码购买闭环。
			if err := runtime.requireSkillUseAuthentication(command.Context()); err != nil {
				return err
			}
			order, err := recoverPendingSkillPurchaseOrder(command.Context(), runtime, productID)
			if err != nil {
				return err
			}
			if order == nil {
				orderValue, err := createSkillPurchaseOrder(command.Context(), runtime, productID)
				if err != nil {
					return err
				}
				order = &orderValue
			}
			presentation, err := presentSkillPaymentQR(runtime, order)
			if err != nil {
				return err
			}
			if wait <= 0 {
				return output.Confirmation("SKILL_PURCHASE_REQUIRED", "the trial is exhausted; purchase this edition to keep using it").WithDetails(map[string]any{
					"productId": productID, "orderNo": order.OrderNo, "amountCents": order.AmountCents, "expiresAt": order.ExpiresAt,
					"paymentPresentation": presentation,
				}).WithHint("present the payment QR to the user, then rerun the same use command with --wait while the payment is in progress")
			}
			if err := waitForSkillOrderPayment(command.Context(), runtime, productID, order.OrderNo, wait); err != nil {
				return err
			}
			removed := removeSkillTrialGates(runtime, productID)
			return runtime.business(skillTrialUseResult{
				ProductID: productID, Allowed: true, Owned: true, RemovedGates: removed, OrderNo: order.OrderNo,
				NextAction: "CONTINUE_TASK",
			})
		},
	}
	command.Flags().DurationVar(&wait, "wait", 5*time.Minute, "wait up to this duration for the WeChat QR payment after the trial is exhausted; 0 presents the QR without waiting")
	return command
}
