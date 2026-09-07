package command

import (
	"net"
	"net/url"
	"strings"
	"unicode/utf16"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/replicacontent"
	"github.com/spf13/cobra"
)

func newReplicaShowcaseCommand(runtime *Runtime) *cobra.Command {
	root := &cobra.Command{Use: "showcase", Short: "Submit, withdraw and review consented Website Replica examples"}
	for _, operation := range []string{"submit", "withdraw", "list", "review"} {
		var replicaID, id, status, workURL, target string
		var anonymous bool
		var revision int
		var input api.WebsiteReplicaShowcaseSubmission
		cmd := &cobra.Command{Use: operation, Args: cobra.NoArgs}
		if operation != "withdraw" {
			cmd.Flags().StringVar(&replicaID, "replica", "", "source Website Replica UUID")
			if operation != "submit" {
				_ = cmd.MarkFlagRequired("replica")
			}
		}
		if operation == "withdraw" || operation == "review" {
			cmd.Flags().StringVar(&id, "showcase", "", "showcase UUID")
			_ = cmd.MarkFlagRequired("showcase")
		}
		if operation == "submit" || operation == "withdraw" {
			cmd.Flags().BoolVar(&anonymous, "anonymous", false, "use the original anonymous installation proof")
			cmd.Flags().StringVar(&workURL, "work-url", "", "original Work URL or Replica instruction")
			cmd.Flags().StringVar(&target, "target", "", "original installed project directory")
		}
		if operation == "submit" {
			for _, field := range []struct {
				value      *string
				name, help string
			}{
				{&input.EntitlementID, "entitlement", "verified download entitlement UUID"}, {&input.VersionID, "version", "source version UUID"},
				{&input.Title, "title", "public case title"}, {&input.PreviewURL, "preview-url", "public HTTPS website URL"},
				{&input.ScreenshotURL, "screenshot-url", "public HTTPS screenshot URL"}, {&input.AuthorName, "author-name", "public author name"},
				{&input.ChangeDescription, "changes", "what changed from the original website"},
			} {
				cmd.Flags().StringVar(field.value, field.name, "", field.help)
				if field.name != "entitlement" && field.name != "version" {
					_ = cmd.MarkFlagRequired(field.name)
				}
			}
			cmd.Flags().BoolVar(&input.Consent, "consent", false, "author explicitly agreed to public showcase display")
		}
		if operation == "review" {
			cmd.Flags().StringVar(&status, "status", "", "APPROVED or REJECTED")
			cmd.Flags().IntVar(&revision, "expected-revision", 0, "revision from the reviewed showcase snapshot")
			_ = cmd.MarkFlagRequired("status")
			_ = cmd.MarkFlagRequired("expected-revision")
		}
		cmd.RunE = func(cmd *cobra.Command, _ []string) error {
			if !anonymous && operation != "withdraw" && !replicaUUIDPattern.MatchString(replicaID) || (operation == "withdraw" || operation == "review") && !replicaUUIDPattern.MatchString(id) {
				return output.Validation("REPLICA_SHOWCASE_INPUT_INVALID", "replica and showcase identities must be UUIDs")
			}
			if operation == "submit" {
				if !input.Consent {
					return output.Confirmation("REPLICA_SHOWCASE_CONSENT_REQUIRED", "review the public case fields and obtain explicit consent before submission")
				}
				if !anonymous && (!replicaUUIDPattern.MatchString(input.EntitlementID) || !replicaUUIDPattern.MatchString(input.VersionID)) || !validShowcaseText(input.Title, 120) || !validShowcaseText(input.AuthorName, 80) || !validShowcaseText(input.ChangeDescription, 1000) || !validPublicShowcaseURL(input.PreviewURL) || !validPublicShowcaseURL(input.ScreenshotURL) {
					return output.Validation("REPLICA_SHOWCASE_INPUT_INVALID", "case fields require valid identities, public HTTPS URLs and nonempty bounded text")
				}
			}
			if operation == "review" && (revision < 1 || status != "APPROVED" && status != "REJECTED") {
				return output.Validation("REPLICA_SHOWCASE_INPUT_INVALID", "review requires APPROVED or REJECTED and the reviewed revision")
			}
			if anonymous {
				if workURL == "" || target == "" {
					return output.Validation("REPLICA_SHOWCASE_INPUT_INVALID", "anonymous cases require original --work-url and --target")
				}
				instruction, err := resolveReplicaTarget(cmd.Context(), runtime, workURL)
				if err != nil {
					return err
				}
				shortCode, err := parseReplicaCode(instruction)
				if err != nil {
					return err
				}
				absolute, err := validateReplicaTarget(target)
				if err != nil {
					return err
				}
				store, err := newReplicaPurchaseStore(runtime, shortCode, absolute)
				if err != nil {
					return err
				}
				completion, exists, err := store.loadCompletion()
				if err != nil {
					return err
				}
				if !exists || !validReplicaSessionSecret(completion.RecoverySecret) {
					return output.Policy("REPLICA_SHOWCASE_PROOF_UNAVAILABLE", "original anonymous installation proof is unavailable; do not repurchase or switch identity")
				}
				proof := api.RecoverWebsiteReplicaDownloadRequest{OrderNo: completion.Result.OrderNo, RecoverySecret: completion.RecoverySecret}
				if operation == "withdraw" {
					result, err := runtime.client().WithdrawAnonymousWebsiteReplicaShowcase(cmd.Context(), id, proof)
					if err != nil {
						return err
					}
					return runtime.business(result)
				}
				record, err := replicacontent.ReadInstalledLicenseRecord(absolute)
				if err != nil {
					return output.Policy("REPLICA_SHOWCASE_PROOF_UNAVAILABLE", "original installed license is unavailable")
				}
				claims, err := verifiedReplicaLicenseClaims(cmd.Context(), runtime, api.WebsiteReplicaDownload{ReplicaID: record.ReplicaID, VersionID: record.VersionID, Version: record.Version, ArtifactDigest: record.ArtifactDigest, License: record.License}, completion.Result.OrderNo)
				if err != nil {
					return err
				}
				if claims.ReplicaID != completion.Result.ReplicaID || claims.VersionID != completion.Result.VersionID {
					return output.Policy("REPLICA_SHOWCASE_PROOF_UNAVAILABLE", "installed license does not match original completion")
				}
				input.EntitlementID = claims.EntitlementID
				input.VersionID = claims.VersionID
				result, err := runtime.client().SubmitAnonymousWebsiteReplicaShowcase(cmd.Context(), input, proof)
				if err != nil {
					return err
				}
				return runtime.business(result)
			}
			scope := "website-replica:read"
			if operation == "review" || operation == "list" {
				scope = "website-replica:write"
			}
			if err := runtime.requireWebsiteReplicaAuthentication(cmd.Context(), scope); err != nil {
				return err
			}
			switch operation {
			case "submit":
				result, err := runtime.client().SubmitWebsiteReplicaShowcase(cmd.Context(), replicaID, input)
				if err != nil {
					return err
				}
				return runtime.business(result)
			case "withdraw":
				result, err := runtime.client().WithdrawWebsiteReplicaShowcase(cmd.Context(), id)
				if err != nil {
					return err
				}
				return runtime.business(result)
			case "list":
				result, err := runtime.client().ListWebsiteReplicaShowcases(cmd.Context(), replicaID)
				if err != nil {
					return err
				}
				return runtime.business(result)
			default:
				result, err := runtime.client().ReviewWebsiteReplicaShowcase(cmd.Context(), replicaID, id, status, revision)
				if err != nil {
					return err
				}
				return runtime.business(result)
			}
		}
		root.AddCommand(cmd)
	}
	return root
}
func validShowcaseText(value string, maximum int) bool {
	return strings.TrimSpace(value) != "" && len(utf16.Encode([]rune(strings.TrimSpace(value)))) <= maximum
}
func validPublicShowcaseURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return len(value) <= 2000 && parsed.Scheme == "https" && strings.Contains(host, ".") && !strings.HasSuffix(host, ".local") && !strings.HasSuffix(host, ".localhost") && net.ParseIP(host) == nil && parsed.User == nil && parsed.Fragment == ""
}
