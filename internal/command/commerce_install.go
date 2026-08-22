package command

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/commerceartifact"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/semver"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	"github.com/spf13/cobra"
)

const commerceRuntimeVersion = "1.1.0"

type commerceSkillInstallResult struct {
	StableName     string                     `json:"stableName"`
	ProductID      string                     `json:"productId"`
	SkillReleaseID string                     `json:"skillReleaseId"`
	ArtifactDigest string                     `json:"artifactDigest"`
	SigningKeyID   string                     `json:"signingKeyId"`
	Distribution   string                     `json:"distribution"`
	Install        skillcontent.InstallReport `json:"install"`
}

func newCommerceSkillInstallCommand(runtime *Runtime) *cobra.Command {
	var agent, distribution string
	command := &cobra.Command{
		Use:   "install <stable-name>",
		Short: "Verify and atomically install a signed product purchase Skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			stableName := args[0]
			if !validStableName(stableName) {
				return output.Validation("COMMERCE_SKILL_NAME_INVALID", "purchase Skill stable name is invalid")
			}
			distribution = strings.ToUpper(distribution)
			if distribution != "DIRECT" && distribution != "WORKBUDDY" {
				return output.Validation("COMMERCE_SKILL_DISTRIBUTION_INVALID", "--distribution must be DIRECT or WORKBUDDY")
			}
			descriptor, err := runtime.client().GetProductPurchaseSkill(command.Context(), stableName)
			if err != nil {
				return err
			}
			install, err := runtime.client().GetProductPurchaseSkillInstall(command.Context(), stableName, distribution)
			if err != nil {
				return err
			}
			if install.StableName != descriptor.StableName ||
				install.SkillReleaseID != descriptor.ActiveRelease.SkillReleaseID ||
				install.ArtifactDigest != descriptor.ActiveRelease.ArtifactDigest {
				return output.Policy("COMMERCE_SKILL_INSTALL_IDENTITY_MISMATCH", "install response does not match the authoritative purchase Skill descriptor")
			}
			if err := validateCommerceRuntimeBootstrap(install, descriptor, distribution); err != nil {
				return err
			}
			publicKey, err := runtime.resolveCommerceTrustKey(command.Context(), descriptor.ActiveRelease.SigningKeyID)
			if err != nil {
				return err
			}
			artifact, err := runtime.client().DownloadArtifact(command.Context(), install.DownloadURL)
			if err != nil {
				return err
			}
			verified, err := commerceartifact.Verify(artifact, publicKey, commerceartifact.Expected{
				ArtifactDigest: install.ArtifactDigest, ArtifactType: "PRODUCT_PURCHASE",
				ProductID: descriptor.ProductID, StableName: descriptor.StableName,
				SkillReleaseID: descriptor.ActiveRelease.SkillReleaseID,
				ReleaseVersion: descriptor.ActiveRelease.Version,
				SigningKeyID:   descriptor.ActiveRelease.SigningKeyID,
				EnvelopeDigest: descriptor.ActiveRelease.SignedEnvelopeDigest,
				Signature:      descriptor.ActiveRelease.Signature,
			})
			if err != nil {
				return output.Policy("COMMERCE_SKILL_VERIFICATION_FAILED", err.Error())
			}
			if verified.Signature.Envelope.RuntimeProtocolVersion != 1 {
				return output.Policy("COMMERCE_RUNTIME_PROTOCOL_UNSUPPORTED", "purchase Skill requires an unsupported Commerce Runtime protocol")
			}
			if err := validateCommerceRuntimeVersion(verified.Signature.Envelope.MinimumRuntimeVersion); err != nil {
				return err
			}
			report, err := installVerifiedCommerceSkill(stableName, agent, verified.Files, runtime.deps.Environment)
			if err != nil {
				return err
			}
			if !report.AllSucceeded {
				return output.Internal("COMMERCE_SKILL_INSTALL_FAILED", "one or more Skill targets could not be installed", nil).
					WithDetails(map[string]any{"report": report})
			}
			return runtime.business(commerceSkillInstallResult{
				StableName: stableName, ProductID: descriptor.ProductID,
				SkillReleaseID: install.SkillReleaseID, ArtifactDigest: install.ArtifactDigest,
				SigningKeyID: descriptor.ActiveRelease.SigningKeyID,
				Distribution: distribution, Install: report,
			})
		},
	}
	command.Flags().StringVar(&agent, "agent", "auto", "installation target: auto, codex, claude, workbuddy, or agents")
	command.Flags().StringVar(&distribution, "distribution", "DIRECT", "artifact distribution: DIRECT or WORKBUDDY")
	return command
}

func validateCommerceRuntimeVersion(minimumRuntimeVersion string) error {
	comparison, err := semver.Compare(commerceRuntimeVersion, minimumRuntimeVersion)
	if err != nil || comparison < 0 {
		return output.Policy("COMMERCE_RUNTIME_VERSION_UNSUPPORTED", "purchase Skill requires a newer ViceMe Commerce Runtime")
	}
	return nil
}

func validateCommerceRuntimeBootstrap(install api.ProductPurchaseSkillInstall, descriptor api.ProductPurchaseSkillDescriptor, distribution string) error {
	runtime := install.Runtime
	if runtime.Kind != "VICEME_CLI" || runtime.ProtocolVersion != 1 ||
		runtime.MinimumRuntimeVersion != descriptor.ActiveRelease.MinimumRuntimeVersion {
		return output.Policy("COMMERCE_RUNTIME_BOOTSTRAP_INVALID", "purchase Skill installation response contains an invalid Commerce Runtime bootstrap")
	}
	if runtime.InstallerContractURL != "https://s3.viceme.cn/start/agent-install.md" &&
		runtime.InstallerContractURL != "https://s3.viceme.ai/start/agent-install.md" {
		return output.Policy("COMMERCE_RUNTIME_BOOTSTRAP_INVALID", "purchase Skill installation response contains an untrusted CLI installation contract")
	}
	agent := "auto"
	if distribution == "WORKBUDDY" {
		agent = "workbuddy"
	}
	expectedCommand := fmt.Sprintf("viceme commerce skill install %s --agent %s --distribution %s", descriptor.StableName, agent, distribution)
	if runtime.InstallCommand != expectedCommand {
		return output.Policy("COMMERCE_RUNTIME_BOOTSTRAP_INVALID", "purchase Skill installation response contains a mismatched CLI installation command")
	}
	return nil
}

func (runtime *Runtime) resolveCommerceTrustKey(ctx context.Context, keyID string) (string, error) {
	if key := compiledCommerceTrustKeys()[keyID]; key != "" {
		return key, nil
	}
	origin, err := api.NormalizeAPIOrigin(runtime.apiBaseURL)
	if err != nil || !isLoopbackOrigin(origin) {
		return "", output.Policy("COMMERCE_SKILL_SIGNING_KEY_UNTRUSTED", "the official CLI trust ring does not contain this Commerce Skill signing key").
			WithHint("install a ViceMe CLI release that contains the platform's current Commerce Skill public key")
	}
	development, err := runtime.client().GetCommerceSkillTrustKey(ctx, keyID)
	if err != nil {
		return "", err
	}
	if development.KeyID != keyID || development.Algorithm != "Ed25519" || development.PublicKey == "" {
		return "", output.Policy("COMMERCE_SKILL_TRUST_KEY_INVALID", "local Commerce Skill trust-key response is invalid")
	}
	return development.PublicKey, nil
}

func compiledCommerceTrustKeys() map[string]string {
	keys, err := commerceartifact.ParseTrustRing(buildinfo.CommerceSkillTrustKeys)
	if err != nil {
		return map[string]string{}
	}
	return keys
}

func installVerifiedCommerceSkill(stableName, target string, files map[string][]byte, environment skillcontent.Environment) (skillcontent.InstallReport, error) {
	root, err := os.MkdirTemp("", "viceme-commerce-skill-")
	if err != nil {
		return skillcontent.InstallReport{}, output.Internal("COMMERCE_SKILL_STAGE_FAILED", "could not create a private Skill staging directory", err)
	}
	defer os.RemoveAll(root)
	skillRoot := filepath.Join(root, stableName)
	if err := os.MkdirAll(skillRoot, 0o700); err != nil {
		return skillcontent.InstallReport{}, output.Internal("COMMERCE_SKILL_STAGE_FAILED", "could not create the Skill staging directory", err)
	}
	paths := make([]string, 0, len(files))
	for name := range files {
		paths = append(paths, name)
	}
	sort.Strings(paths)
	for _, name := range paths {
		destination := filepath.Join(skillRoot, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return skillcontent.InstallReport{}, output.Internal("COMMERCE_SKILL_STAGE_FAILED", "could not create a Skill staging path", err)
		}
		if err := os.WriteFile(destination, files[name], 0o600); err != nil {
			return skillcontent.InstallReport{}, output.Internal("COMMERCE_SKILL_STAGE_FAILED", "could not stage a verified Skill file", err)
		}
	}
	openAI := fmt.Sprintf("interface:\n  display_name: %q\n  short_description: %q\n  default_prompt: %q\n",
		"Buy with "+stableName,
		"Purchase the server-bound ViceMe Product",
		"Use $"+stableName+" to purchase its bound Product through ViceMe.",
	)
	if err := os.MkdirAll(filepath.Join(skillRoot, "agents"), 0o700); err != nil {
		return skillcontent.InstallReport{}, output.Internal("COMMERCE_SKILL_STAGE_FAILED", "could not stage Skill metadata", err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "agents", "openai.yaml"), []byte(openAI), 0o600); err != nil {
		return skillcontent.InstallReport{}, output.Internal("COMMERCE_SKILL_STAGE_FAILED", "could not stage Skill metadata", err)
	}
	metadata, _ := json.MarshalIndent(map[string]any{
		"schema_version":      1,
		"skill_version":       buildinfo.SkillVersion,
		"minimum_cli_version": buildinfo.MinimumCLIVersion,
		"cli_compatibility":   buildinfo.CLICompatibility,
	}, "", "  ")
	metadata = append(metadata, '\n')
	if err := os.WriteFile(filepath.Join(skillRoot, "skill-package.json"), metadata, 0o600); err != nil {
		return skillcontent.InstallReport{}, output.Internal("COMMERCE_SKILL_STAGE_FAILED", "could not stage Skill package metadata", err)
	}
	bundle := skillcontent.New(os.DirFS(root))
	report := bundle.Install(stableName, target, environment)
	return report, nil
}
