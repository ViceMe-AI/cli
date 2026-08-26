package command

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ViceMe-AI/cli/internal/api"
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
	ExperiencePlan       json.RawMessage         `json:"experiencePlan"`
	InitialInputSchema   json.RawMessage         `json:"initialInputSchema"`
	InitialInputGuide    []interactionInputField `json:"initialInputGuide"`
}

type interactionFlowCreateResult struct {
	NextAction  string          `json:"nextAction"`
	InstanceURL string          `json:"instanceUrl,omitempty"`
	Interaction json.RawMessage `json:"interaction"`
}

type interactionFlowProgressResult struct {
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
			if install.StableName != stableName || install.SkillReleaseID != descriptor.ActiveRelease.SkillReleaseID || install.ArtifactDigest != descriptor.ActiveRelease.ArtifactDigest || install.Runtime.Kind != "VICEME_CLI" || install.Runtime.ProtocolVersion != 2 || install.Runtime.MinimumRuntimeVersion != descriptor.ActiveRelease.MinimumRuntimeVersion {
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
	comparison, err := semver.Compare("1.6.0", minimumRuntimeVersion)
	if err != nil || comparison < 0 {
		return output.Policy("INTERACTION_RUNTIME_VERSION_UNSUPPORTED", "Interaction Skill requires a newer ViceMe Interaction Runtime")
	}
	return nil
}

func newInteractionFlowCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "flow", Short: "Run the deterministic direct-service entry flow"}
	command.AddCommand(newInteractionFlowStartCommand(runtime))
	command.AddCommand(newInteractionFlowCreateCommand(runtime))
	command.AddCommand(newInteractionFlowShowCommand(runtime))
	command.AddCommand(newInteractionFlowActCommand(runtime))
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
		nextAction := "CREATE_INSTANCE"
		for _, field := range guide {
			if field.Required {
				nextAction = "COLLECT_INPUT"
				break
			}
		}
		return runtime.business(interactionFlowStartResult{NextAction: nextAction, StableName: descriptor.StableName, WorkID: descriptor.WorkID, DefinitionVersionID: descriptor.DefinitionVersionID, CreateIdempotencyKey: runtime.deps.NewID(), Title: descriptor.Title, ExperiencePlan: descriptor.Experience, InitialInputSchema: descriptor.InitialInput, InitialInputGuide: guide})
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
		return runtime.business(interactionFlowCreateResult{NextAction: interactionProjectionNextAction(projection), InstanceURL: instanceURL, Interaction: projection})
	}}
	command.Flags().StringVar(&stableName, "skill", "", "Interaction Skill stable name")
	command.Flags().StringVar(&inputJSON, "input-json", "", "scenario input as one JSON object")
	command.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "createIdempotencyKey returned by interaction flow start")
	_ = command.MarkFlagRequired("skill")
	_ = command.MarkFlagRequired("input-json")
	_ = command.MarkFlagRequired("idempotency-key")
	return command
}

func newInteractionFlowShowCommand(runtime *Runtime) *cobra.Command {
	var instanceNo string
	command := &cobra.Command{Use: "show", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		projection, err := runtime.client().GetInteraction(command.Context(), instanceNo)
		if err != nil {
			return err
		}
		return runtime.business(interactionFlowProgressResult{NextAction: interactionProjectionNextAction(projection), InstanceURL: interactionInstanceURL(projection), Interaction: projection})
	}}
	command.Flags().StringVar(&instanceNo, "instance", "", "Interaction instance number")
	_ = command.MarkFlagRequired("instance")
	return command
}

func newInteractionFlowActCommand(runtime *Runtime) *cobra.Command {
	var instanceNo, actionCode, inputJSON, idempotencyKey, taskID string
	var expectedVersion int
	var assets, audiences []string
	command := &cobra.Command{Use: "act", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		if expectedVersion < 1 {
			return output.Validation("INTERACTION_ACTION_INPUT_INVALID", "--expected-version must be positive")
		}
		if strings.TrimSpace(inputJSON) == "" {
			inputJSON = "{}"
		}
		payload, err := normalizeInlineJSONObject(inputJSON)
		if err != nil {
			return err
		}
		artifactIDs, err := uploadInteractionAssets(command, runtime, instanceNo, assets, audiences, "")
		if err != nil {
			return err
		}
		request := map[string]any{
			"expectedInstanceVersion": expectedVersion,
			"idempotencyKey":          idempotencyKey,
			"payload":                 json.RawMessage(payload),
			"artifacts":               artifactIDs,
		}
		if taskID != "" {
			request["taskId"] = taskID
		}
		encoded, err := rawJSONObject(request)
		if err != nil {
			return output.Internal("INTERACTION_ACTION_INPUT_INVALID", "could not encode Interaction action", err)
		}
		projection, err := runtime.client().ActInteraction(command.Context(), instanceNo, actionCode, encoded)
		if err != nil {
			return err
		}
		return runtime.business(interactionFlowProgressResult{NextAction: interactionProjectionNextAction(projection), InstanceURL: interactionInstanceURL(projection), Interaction: projection})
	}}
	command.Flags().StringVar(&instanceNo, "instance", "", "Interaction instance number")
	command.Flags().StringVar(&actionCode, "action", "", "allowed action code returned by the Interaction projection")
	command.Flags().IntVar(&expectedVersion, "expected-version", 0, "current instance version")
	command.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "idempotency key for this action")
	command.Flags().StringVar(&taskID, "task", "", "optional open task id")
	command.Flags().StringVar(&inputJSON, "input-json", "{}", "action input as one JSON object")
	command.Flags().StringSliceVar(&assets, "asset", nil, "local artifact path; repeat for multiple files")
	command.Flags().StringSliceVar(&audiences, "asset-audience", []string{"PARTICIPANT"}, "artifact audience: PARTICIPANT, CREATOR, or ALL_PARTICIPANTS")
	_ = command.MarkFlagRequired("instance")
	_ = command.MarkFlagRequired("action")
	_ = command.MarkFlagRequired("expected-version")
	_ = command.MarkFlagRequired("idempotency-key")
	return command
}

func uploadInteractionAssets(command *cobra.Command, runtime *Runtime, instanceNo string, paths, audiences []string, token string) ([]string, error) {
	if len(paths) == 0 {
		return []string{}, nil
	}
	allowed := map[string]bool{"PARTICIPANT": true, "CREATOR": true, "ALL_PARTICIPANTS": true}
	for _, audience := range audiences {
		if !allowed[audience] {
			return nil, output.Validation("INTERACTION_ARTIFACT_AUDIENCE_INVALID", "--asset-audience contains an unsupported audience")
		}
	}
	ids := make([]string, 0, len(paths))
	for _, localPath := range paths {
		file, err := os.Open(localPath)
		if err != nil {
			return nil, output.Validation("INTERACTION_ARTIFACT_FILE_INVALID", "could not open an Interaction artifact")
		}
		info, statErr := file.Stat()
		if statErr != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 100<<20 {
			_ = file.Close()
			return nil, output.Validation("INTERACTION_ARTIFACT_FILE_INVALID", "Interaction artifact must be a non-empty regular file up to 100 MiB")
		}
		hash := sha256.New()
		if _, err = io.Copy(hash, file); err != nil {
			_ = file.Close()
			return nil, output.Internal("INTERACTION_ARTIFACT_READ_FAILED", "could not read an Interaction artifact", err)
		}
		if _, err = file.Seek(0, io.SeekStart); err != nil {
			_ = file.Close()
			return nil, output.Internal("INTERACTION_ARTIFACT_READ_FAILED", "could not rewind an Interaction artifact", err)
		}
		contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(info.Name())))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		request, err := rawJSONObject(map[string]any{"fileName": info.Name(), "contentType": contentType, "sizeBytes": info.Size(), "digest": fmt.Sprintf("%x", hash.Sum(nil)), "audience": audiences})
		if err != nil {
			_ = file.Close()
			return nil, output.Internal("INTERACTION_ARTIFACT_INPUT_INVALID", "could not encode artifact metadata", err)
		}
		var prepared api.InteractionArtifactUpload
		if token == "" {
			prepared, err = runtime.client().PrepareInteractionArtifact(command.Context(), instanceNo, request)
		} else {
			prepared, err = runtime.client().PrepareInteractionArtifactWithToken(command.Context(), instanceNo, request, token)
		}
		if err == nil {
			err = runtime.client().PutPresigned(command.Context(), prepared.UploadURL, prepared.Headers, file, info.Size())
		}
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		var completed api.InteractionArtifactCompletion
		if token == "" {
			completed, err = runtime.client().CompleteInteractionArtifact(command.Context(), instanceNo, prepared.ArtifactID)
		} else {
			completed, err = runtime.client().CompleteInteractionArtifactWithToken(command.Context(), instanceNo, prepared.ArtifactID, token)
		}
		if err != nil || completed.Status != "COMPLETED" || completed.ArtifactID != prepared.ArtifactID {
			if err != nil {
				return nil, err
			}
			return nil, output.Internal("INTERACTION_ARTIFACT_COMPLETION_INVALID", "artifact completion response is invalid", nil)
		}
		ids = append(ids, completed.ArtifactID)
	}
	return ids, nil
}

func interactionProjectionNextAction(projection json.RawMessage) string {
	var value struct {
		Instance struct {
			LifecycleStatus string `json:"lifecycleStatus"`
		} `json:"instance"`
		Tasks []struct {
			Type string `json:"type"`
		} `json:"tasks"`
		AllowedActions []json.RawMessage `json:"allowedActions"`
	}
	if json.Unmarshal(projection, &value) != nil {
		return "INSPECT_INTERACTION"
	}
	if value.Instance.LifecycleStatus != "OPEN" {
		return "INTERACTION_CLOSED"
	}
	for _, task := range value.Tasks {
		if task.Type == "OPEN" {
			return "COMPLETE_TASK"
		}
	}
	if len(value.AllowedActions) > 0 {
		return "EXECUTE_ACTION"
	}
	return "WAIT_FOR_ROLE"
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
