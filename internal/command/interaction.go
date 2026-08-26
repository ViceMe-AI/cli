package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ViceMe-AI/cli/internal/interactionartifact"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/semver"
	"github.com/spf13/cobra"
)

type interactionInputField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Type        string `json:"type,omitempty"`
	Format      string `json:"format,omitempty"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
	Options     []any  `json:"options,omitempty"`
}

type interactionFlowStartResult struct {
	NextAction           string                  `json:"nextAction"`
	StableName           string                  `json:"stableName"`
	WorkID               string                  `json:"workId"`
	DefinitionVersionID  string                  `json:"definitionVersionId"`
	CreateIdempotencyKey string                  `json:"createIdempotencyKey"`
	Title                string                  `json:"title"`
	InitialInputSchema   json.RawMessage         `json:"initialInputSchema"`
	InitialInputGuide    []interactionInputField `json:"initialInputGuide"`
}

type interactionFlowCreateResult struct {
	NextAction  string          `json:"nextAction"`
	InstanceURL string          `json:"instanceUrl,omitempty"`
	Interaction json.RawMessage `json:"interaction"`
}

func newInteractionCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "interaction", Short: "Run signed ViceMe service interaction Skills"}
	command.AddCommand(newInteractionSkillCommand(runtime))
	command.AddCommand(newInteractionFlowCommand(runtime))
	return command
}

func newInteractionSkillCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "skill", Short: "Inspect or install a generated Interaction Skill"}
	command.AddCommand(&cobra.Command{
		Use: "get <stable-name>", Aliases: []string{"show"}, Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			descriptor, err := runtime.client().GetInteractionSkill(command.Context(), args[0])
			if err != nil {
				return err
			}
			return runtime.business(descriptor)
		},
	})
	command.AddCommand(newInteractionSkillInstallCommand(runtime))
	return command
}

func newInteractionSkillInstallCommand(runtime *Runtime) *cobra.Command {
	var agent string
	command := &cobra.Command{
		Use: "install <stable-name>", Short: "Verify and install a signed service Interaction Skill", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			stableName := args[0]
			if !validStableName(stableName) {
				return output.Validation("INTERACTION_SKILL_NAME_INVALID", "Interaction Skill stable name is invalid")
			}
			agent = strings.ToLower(agent)
			if agent != "auto" && agent != "codex" && agent != "claude" && agent != "workbuddy" && agent != "agents" {
				return output.Validation("INTERACTION_SKILL_AGENT_INVALID", "--agent must be auto, codex, claude, workbuddy, or agents")
			}
			apiAgent := "auto"
			if agent == "workbuddy" {
				apiAgent = "workbuddy"
			}
			descriptor, err := runtime.client().GetInteractionSkill(command.Context(), stableName)
			if err != nil {
				return err
			}
			manifest, err := interactionartifact.ParseManifest(descriptor.ActiveRelease.Manifest)
			if err != nil {
				return output.Policy("INTERACTION_SKILL_VERIFICATION_FAILED", err.Error())
			}
			envelope, err := interactionartifact.ParseEnvelope(descriptor.ActiveRelease.SignedEnvelope)
			if err != nil {
				return output.Policy("INTERACTION_SKILL_VERIFICATION_FAILED", err.Error())
			}
			if descriptor.WorkID != manifest.WorkID || descriptor.WorkID != envelope.WorkID ||
				descriptor.DefinitionVersionID != manifest.DefinitionVersionID || descriptor.DefinitionVersionID != envelope.DefinitionVersionID ||
				descriptor.StableName != manifest.StableName || descriptor.StableName != envelope.StableName ||
				descriptor.ActiveRelease.SkillReleaseID != manifest.SkillReleaseID || descriptor.ActiveRelease.SkillReleaseID != envelope.SkillReleaseID ||
				descriptor.ActiveRelease.Version != manifest.ReleaseVersion || descriptor.ActiveRelease.Version != envelope.ReleaseVersion {
				return output.Policy("INTERACTION_SKILL_VERIFICATION_FAILED", "Interaction Skill descriptor identities do not match")
			}
			install, err := runtime.client().GetInteractionSkillInstall(command.Context(), stableName, apiAgent)
			if err != nil {
				return err
			}
			if install.StableName != stableName || install.SkillReleaseID != descriptor.ActiveRelease.SkillReleaseID || install.ArtifactDigest != descriptor.ActiveRelease.ArtifactDigest || install.Runtime.Kind != "VICEME_CLI" || install.Runtime.ProtocolVersion != 1 || install.Runtime.MinimumRuntimeVersion != descriptor.ActiveRelease.MinimumRuntimeVersion {
				return output.Policy("INTERACTION_SKILL_INSTALL_IDENTITY_MISMATCH", "install response does not match the requested Interaction Skill")
			}
			if install.Runtime.InstallCommand != fmt.Sprintf("viceme interaction skill install %s --agent %s", stableName, apiAgent) {
				return output.Policy("INTERACTION_RUNTIME_BOOTSTRAP_INVALID", "Interaction Skill installation command is invalid")
			}
			if err := validateInteractionRuntimeVersion(descriptor.ActiveRelease.MinimumRuntimeVersion); err != nil {
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
			verified, err := interactionartifact.Verify(artifact, publicKey, interactionartifact.Expected{
				ArtifactDigest: install.ArtifactDigest,
				ManifestDigest: descriptor.ActiveRelease.ManifestDigest,
				EnvelopeDigest: descriptor.ActiveRelease.SignedEnvelopeDigest,
				SigningKeyID:   descriptor.ActiveRelease.SigningKeyID,
				Signature:      descriptor.ActiveRelease.Signature,
				Manifest:       manifest,
				Envelope:       envelope,
			})
			if err != nil {
				return output.Policy("INTERACTION_SKILL_VERIFICATION_FAILED", err.Error())
			}
			report, err := installVerifiedSkill(stableName, agent, verified.Files, runtime.deps.Environment, descriptor.Title, "Start this ViceMe service interaction", "Use $"+stableName+" to start this service and provide the required information.")
			if err != nil {
				return err
			}
			if !report.AllSucceeded {
				return output.Internal("INTERACTION_SKILL_INSTALL_FAILED", "one or more Skill targets could not be installed", nil).WithDetails(map[string]any{"report": report})
			}
			return runtime.business(map[string]any{"stableName": stableName, "workId": descriptor.WorkID, "definitionVersionId": descriptor.DefinitionVersionID, "skillReleaseId": install.SkillReleaseID, "artifactDigest": install.ArtifactDigest, "install": report})
		},
	}
	command.Flags().StringVar(&agent, "agent", "auto", "installation target: auto, codex, claude, workbuddy, or agents")
	return command
}

func validateInteractionRuntimeVersion(minimumRuntimeVersion string) error {
	comparison, err := semver.Compare("1.5.0", minimumRuntimeVersion)
	if err != nil || comparison < 0 {
		return output.Policy("INTERACTION_RUNTIME_VERSION_UNSUPPORTED", "Interaction Skill requires a newer ViceMe Interaction Runtime")
	}
	return nil
}

func newInteractionFlowCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "flow", Short: "Run the deterministic direct-service entry flow"}
	command.AddCommand(newInteractionFlowStartCommand(runtime))
	command.AddCommand(newInteractionFlowCreateCommand(runtime))
	return command
}

func newInteractionFlowStartCommand(runtime *Runtime) *cobra.Command {
	var stableName string
	command := &cobra.Command{Use: "start", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		descriptor, err := runtime.client().GetInteractionSkill(command.Context(), stableName)
		if err != nil {
			return err
		}
		guide, err := buildInteractionInputGuide(descriptor.InitialInput)
		if err != nil {
			return err
		}
		return runtime.business(interactionFlowStartResult{NextAction: "COLLECT_INPUT", StableName: descriptor.StableName, WorkID: descriptor.WorkID, DefinitionVersionID: descriptor.DefinitionVersionID, CreateIdempotencyKey: runtime.deps.NewID(), Title: descriptor.Title, InitialInputSchema: descriptor.InitialInput, InitialInputGuide: guide})
	}}
	command.Flags().StringVar(&stableName, "skill", "", "Interaction Skill stable name")
	_ = command.MarkFlagRequired("skill")
	return command
}

func newInteractionFlowCreateCommand(runtime *Runtime) *cobra.Command {
	var stableName, inputJSON, idempotencyKey string
	command := &cobra.Command{Use: "create", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		payload, err := normalizeInlineJSONObject(inputJSON)
		if err != nil {
			return err
		}
		descriptor, err := runtime.client().GetInteractionSkill(command.Context(), stableName)
		if err != nil {
			return err
		}
		request, err := rawJSONObject(map[string]any{"workId": descriptor.WorkID, "definitionVersionId": descriptor.DefinitionVersionID, "entryMode": "DIRECT", "idempotencyKey": idempotencyKey, "payload": json.RawMessage(payload)})
		if err != nil {
			return output.Internal("INTERACTION_CREATE_INPUT_INVALID", "could not encode Interaction input", err)
		}
		projection, err := runtime.client().CreateInteraction(command.Context(), request)
		if err != nil {
			return err
		}
		instanceURL := interactionInstanceURL(projection)
		return runtime.business(interactionFlowCreateResult{NextAction: "INSTANCE_CREATED", InstanceURL: instanceURL, Interaction: projection})
	}}
	command.Flags().StringVar(&stableName, "skill", "", "Interaction Skill stable name")
	command.Flags().StringVar(&inputJSON, "input-json", "", "scenario input as one JSON object")
	command.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "createIdempotencyKey returned by interaction flow start")
	_ = command.MarkFlagRequired("skill")
	_ = command.MarkFlagRequired("input-json")
	_ = command.MarkFlagRequired("idempotency-key")
	return command
}

func normalizeInlineJSONObject(raw string) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace([]byte(raw))
	var object map[string]json.RawMessage
	if len(trimmed) == 0 || trimmed[0] != '{' || json.Unmarshal(trimmed, &object) != nil || object == nil {
		return nil, output.Validation("INTERACTION_INPUT_INVALID", "--input-json must be one valid JSON object")
	}
	return json.Marshal(object)
}

func buildInteractionInputGuide(schema json.RawMessage) ([]interactionInputField, error) {
	var value struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(schema, &value); err != nil {
		return nil, output.Policy("INTERACTION_INPUT_SCHEMA_INVALID", "Interaction initial input schema is invalid")
	}
	required := map[string]bool{}
	for _, key := range value.Required {
		required[key] = true
	}
	guide := make([]interactionInputField, 0, len(value.Properties))
	keys := make([]string, 0, len(value.Properties))
	for key := range value.Properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		raw := value.Properties[key]
		var property struct {
			Title       string `json:"title"`
			Type        string `json:"type"`
			Format      string `json:"format"`
			Description string `json:"description"`
			Enum        []any  `json:"enum"`
		}
		if err := json.Unmarshal(raw, &property); err != nil {
			continue
		}
		label := property.Title
		if label == "" {
			label = key
		}
		guide = append(guide, interactionInputField{Key: key, Label: label, Type: property.Type, Format: property.Format, Description: property.Description, Required: required[key], Options: property.Enum})
	}
	return guide, nil
}

func interactionInstanceURL(projection json.RawMessage) string {
	var value struct {
		Instance struct {
			InstanceNo string `json:"instanceNo"`
			Work       struct {
				CreatorHandle string `json:"creatorHandle"`
				Slug          string `json:"slug"`
			} `json:"work"`
		} `json:"instance"`
	}
	if json.Unmarshal(projection, &value) != nil || value.Instance.InstanceNo == "" {
		return ""
	}
	return fmt.Sprintf("/interaction/%s/%s/%s", value.Instance.Work.CreatorHandle, value.Instance.Work.Slug, value.Instance.InstanceNo)
}
