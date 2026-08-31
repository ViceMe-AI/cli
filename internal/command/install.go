package command

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	updatepkg "github.com/ViceMe-AI/cli/internal/update"
	"github.com/gofrs/flock"
	"github.com/spf13/cobra"
)

var officialSkillNames = []string{
	"viceme-shared",
	"viceme-creator-onboarding",
	"viceme-paid-skill",
	"viceme-skill-use",
	"viceme-access",
	"viceme-tip",
}

// Retired official Skills were published in releases but are no longer
// carried. Each identity pins the exact published bundle (per the release
// manifest) so an upgrade removes only CLI-managed installs; user-modified or
// user-owned same-name directories never match and are preserved.
var retiredOfficialSkills = []skillcontent.RetiredSkillIdentity{
	{
		Name:                  "viceme-publish",
		SkillVersion:          "0.20.0",
		MinimumCLIVersion:     "0.20.0",
		CLICompatibility:      ">=0.20.0 <0.21.0",
		FullBundleDigest:      "sha256:b9fb4e235ccad7c574191973e61ca05f8706a6a9d0edc6db6ba7330ea71ca74f",
		EmbeddedContentDigest: "sha256:eb1c418e14ecadf6922cbb27e703cda4e2e72c3dbb445c287e916d8a44c775c9",
	},
	{
		Name:                  "viceme-publish",
		SkillVersion:          "0.19.0",
		MinimumCLIVersion:     "0.19.0",
		CLICompatibility:      ">=0.19.0 <0.20.0",
		FullBundleDigest:      "sha256:94519e4cd619a466ce6c86ee1b8d3cc8dc7ec794184888ab2415ec0b9c2eea8f",
		EmbeddedContentDigest: "sha256:b45f0c622ba2ba4beb2f8c262ddad0cd77e6e5ea3c09daf150ca111917ab1406",
	},
	{
		Name:                  "viceme-publish",
		SkillVersion:          "0.18.0",
		MinimumCLIVersion:     "0.18.0",
		CLICompatibility:      ">=0.18.0 <0.19.0",
		FullBundleDigest:      "sha256:7ef34c3a5a8655c9228911a6a75758a53fa5ca2e1e206f382b5df7f7d38ca3b9",
		EmbeddedContentDigest: "sha256:70652050e12f0376b2174597bc57cf0c3a3fbabdb508d27dac945feb95a5fc77",
	},
	{
		Name:                  "viceme-publish",
		SkillVersion:          "0.17.0",
		MinimumCLIVersion:     "0.17.0",
		CLICompatibility:      ">=0.17.0 <0.18.0",
		FullBundleDigest:      "sha256:27ea21b4b881e54e2406ea0ce09332e095bce2238f137de2af7716926e4df33a",
		EmbeddedContentDigest: "sha256:8a4e2a8564db94a84446ed5afabaa20303b6907491549b1e3d898c5f1e6eb41f",
	},
	{
		Name:                  "viceme-publish",
		SkillVersion:          "0.16.1",
		MinimumCLIVersion:     "0.16.1",
		CLICompatibility:      ">=0.16.1 <0.17.0",
		FullBundleDigest:      "sha256:e4a36843f9fd4427f717f0f82fd4e490f195ad4ebd4c5cf1bbe88a3435ac0673",
		EmbeddedContentDigest: "sha256:8a4e2a8564db94a84446ed5afabaa20303b6907491549b1e3d898c5f1e6eb41f",
	},
	{
		Name:                  "viceme-publish",
		SkillVersion:          "0.16.0",
		MinimumCLIVersion:     "0.16.0",
		CLICompatibility:      ">=0.16.0 <0.17.0",
		FullBundleDigest:      "sha256:958adebdc89dd88abcb9de5821835424c7c0728e31ef73e181b7d98b765e62d1",
		EmbeddedContentDigest: "sha256:a00f289c9b1a8ccb043b71f4df7ba05af3a5f0bbb0ee954ccec9e9795944cfaf",
	},
	{
		Name:                  "viceme-publish",
		SkillVersion:          "0.15.2",
		MinimumCLIVersion:     "0.15.2",
		CLICompatibility:      ">=0.15.2 <0.16.0",
		FullBundleDigest:      "sha256:3f3f63cd4ef4b9392438b312520ee40a4294f1d58b1355d8e6595593f3460032",
		EmbeddedContentDigest: "sha256:a00f289c9b1a8ccb043b71f4df7ba05af3a5f0bbb0ee954ccec9e9795944cfaf",
	},
	{
		Name:                  "viceme-publish",
		SkillVersion:          "0.15.1",
		MinimumCLIVersion:     "0.15.1",
		CLICompatibility:      ">=0.15.1 <0.16.0",
		FullBundleDigest:      "sha256:cbc71a70d1c922782f5a8ed8cb308393ac62b278f9ec1f07361ebd226806ca6c",
		EmbeddedContentDigest: "sha256:7ca9b2ab9f12d62978e641eaac94a2b2857023aa1f6d2e584b6993fcd9b00da5",
	},
	{
		Name:                  "viceme-publish",
		SkillVersion:          "0.15.0",
		MinimumCLIVersion:     "0.15.0",
		CLICompatibility:      ">=0.15.0 <0.16.0",
		FullBundleDigest:      "sha256:17b17933142c73bf199b27d3a3e27af251e2ed2b855dfc5a81e03314e1d40195",
		EmbeddedContentDigest: "sha256:803d6cb6fbbe19774283d1cdfb4c0db424f226da62cc70bf2f79e2804271a098",
	},
	{
		Name:                  "viceme-publish",
		SkillVersion:          "0.14.3",
		MinimumCLIVersion:     "0.14.3",
		CLICompatibility:      ">=0.14.3 <0.15.0",
		FullBundleDigest:      "sha256:02545d681de3c4ee7afd07b2a81d9efa5c045b8f93e5cc17025a812c03a7727d",
		EmbeddedContentDigest: "sha256:6f60fb53d5842493b81e9293af8300f6ff52b8f84824ae67947e57e3de60bdd8",
	},
	{
		Name:                  "viceme-publish",
		SkillVersion:          "0.14.2",
		MinimumCLIVersion:     "0.14.2",
		CLICompatibility:      ">=0.14.2 <0.15.0",
		FullBundleDigest:      "sha256:b16b749f029a3a9d73cbf6031bac89b1d5e9888f35b93b1743974d232ea5c024",
		EmbeddedContentDigest: "sha256:6a576bbd41cf0d8a95bad109ee6af114315100dce283e11b5f5ff36c102084e7",
	},
	{
		Name:                  "viceme-publish",
		SkillVersion:          "0.14.1",
		MinimumCLIVersion:     "0.14.1",
		CLICompatibility:      ">=0.14.1 <0.15.0",
		FullBundleDigest:      "sha256:42ab34ccdc066a43d5b5ee14d498a4d69a46bd32f8e91db5196ea7cf05494a55",
		EmbeddedContentDigest: "sha256:6a576bbd41cf0d8a95bad109ee6af114315100dce283e11b5f5ff36c102084e7",
	},
	{
		Name:                  "viceme-publish",
		SkillVersion:          "0.14.0",
		MinimumCLIVersion:     "0.14.0",
		CLICompatibility:      ">=0.14.0 <0.15.0",
		FullBundleDigest:      "sha256:fb4b11424dfc719526c2b3548d6e160ffb744c74f47a46c56d94427861ba50bc",
		EmbeddedContentDigest: "sha256:6a576bbd41cf0d8a95bad109ee6af114315100dce283e11b5f5ff36c102084e7",
	},
	{
		Name:                  "viceme-publish",
		SkillVersion:          "0.13.3",
		MinimumCLIVersion:     "0.13.3",
		CLICompatibility:      ">=0.13.3 <0.14.0",
		FullBundleDigest:      "sha256:12c6ff32d0383f2e600fa1cb7f965446e062fa51df5883aee081756c6ca8a313",
		EmbeddedContentDigest: "sha256:5a63be6f8ddfd6a16cc8eb5efb6fc47fa2e22ecf7095f1b9d5f3d98d15b23771",
	},
	{
		Name:                  "viceme-publish",
		SkillVersion:          "0.13.2",
		MinimumCLIVersion:     "0.13.2",
		CLICompatibility:      ">=0.13.2 <0.14.0",
		FullBundleDigest:      "sha256:fe454ccb749e18ff89d6fd09bd34551460f9116cbad80bcc454b9b51260ad902",
		EmbeddedContentDigest: "sha256:5a63be6f8ddfd6a16cc8eb5efb6fc47fa2e22ecf7095f1b9d5f3d98d15b23771",
	},
	{
		Name:                  "viceme-publish",
		SkillVersion:          "0.13.1",
		MinimumCLIVersion:     "0.13.1",
		CLICompatibility:      ">=0.13.1 <0.14.0",
		FullBundleDigest:      "sha256:88c26645eb91a53181d8977e82ca7f22ea59c5e2fffd516bf248647e0fd852bd",
		EmbeddedContentDigest: "sha256:89b5b467370e1df963ba0c8ff1f1b07a74fe69331cdbf892fbf16b889586a7b0",
	},
	{
		Name:                  "viceme-publish",
		SkillVersion:          "0.13.0",
		MinimumCLIVersion:     "0.13.0",
		CLICompatibility:      ">=0.13.0 <0.14.0",
		FullBundleDigest:      "sha256:d6f2d32b7d75ee0d7140bc3edbf5b53e5a2f34f4afe45d7cc1b40f45f42beb54",
		EmbeddedContentDigest: "sha256:1a91c0e0101f26a2a79742c639916376a81ba535d75530a13b6eeedc6fc94c79",
	},
	{
		Name:                  "viceme-danmaku",
		SkillVersion:          "0.16.0-beta.0",
		MinimumCLIVersion:     "0.16.0-beta.0",
		CLICompatibility:      ">=0.16.0-beta.0 <0.17.0",
		FullBundleDigest:      "sha256:49eb9b01f0df6b9820b8431001625820667eaaaa38002df69e731f344145a044",
		EmbeddedContentDigest: "sha256:92a223018b69c0de4ab348106d065bf8771bac07bc909ea7281e3890dd0f678b",
	},
	// v0.16.0-beta.1 through v0.16.0-beta.5 published this same
	// package identity.
	{
		Name:                  "viceme-danmaku",
		SkillVersion:          "0.16.0-beta.1",
		MinimumCLIVersion:     "0.16.0-beta.1",
		CLICompatibility:      ">=0.16.0-beta.1 <0.17.0",
		FullBundleDigest:      "sha256:40d65aaff6c9f604c46ba29c9bed685692782ed384b58f948f7b8d8219aa9bcf",
		EmbeddedContentDigest: "sha256:92a223018b69c0de4ab348106d065bf8771bac07bc909ea7281e3890dd0f678b",
	},
	{
		Name:                  "viceme-danmaku",
		SkillVersion:          "0.16.0-beta.6",
		MinimumCLIVersion:     "0.16.0-beta.6",
		CLICompatibility:      ">=0.16.0-beta.6 <0.17.0",
		FullBundleDigest:      "sha256:5957907cd8312ee9345aa4fcd84c048f2ead85b706fbb19d4d4d5169c5c05e3c",
		EmbeddedContentDigest: "sha256:8d7a4976537455886dd41d0b6e911c25a2b09a699c0e41f975ad5d26760e7db4",
	},
	{
		Name:                  "viceme-danmaku",
		SkillVersion:          "0.16.0-beta.7",
		MinimumCLIVersion:     "0.16.0-beta.7",
		CLICompatibility:      ">=0.16.0-beta.7 <0.17.0",
		FullBundleDigest:      "sha256:e183a7f2cfa1d686d60479d9399107966fbd007124a137292aa8d8e4881bea44",
		EmbeddedContentDigest: "sha256:95c9c8313529d1ff8e822979ea182ca26f2b3b111872d7f2d9afeabc9e58a79d",
	},
	{
		Name:                  "viceme-danmaku",
		SkillVersion:          "0.16.0",
		MinimumCLIVersion:     "0.16.0",
		CLICompatibility:      ">=0.16.0 <0.17.0",
		FullBundleDigest:      "sha256:3595bbb8704c9dec0f760ce695e40b5bc94b8276debd8f1caa892a7513eccb65",
		EmbeddedContentDigest: "sha256:92a223018b69c0de4ab348106d065bf8771bac07bc909ea7281e3890dd0f678b",
	},
	{
		Name:                  "viceme-danmaku",
		SkillVersion:          "0.16.1",
		MinimumCLIVersion:     "0.16.1",
		CLICompatibility:      ">=0.16.1 <0.17.0",
		FullBundleDigest:      "sha256:01f497c77e595d5366adbc526029aecf585ebc4f10848ef9c4a8f92c7873b38a",
		EmbeddedContentDigest: "sha256:92a223018b69c0de4ab348106d065bf8771bac07bc909ea7281e3890dd0f678b",
	},
	{
		Name:                  "viceme-danmaku",
		SkillVersion:          "0.17.0",
		MinimumCLIVersion:     "0.17.0",
		CLICompatibility:      ">=0.17.0 <0.18.0",
		FullBundleDigest:      "sha256:a01902bb558d2bc452d48261589ec6f73df1d6d4e95f60153bcb7decc479fc2d",
		EmbeddedContentDigest: "sha256:57fd7d60e6664a4fb55c9e1e4aeb76b8b3bd8e48cfd642f96e4ba9dbf638defa",
	},
	{
		Name:                  "viceme-danmaku",
		SkillVersion:          "0.18.0",
		MinimumCLIVersion:     "0.18.0",
		CLICompatibility:      ">=0.18.0 <0.19.0",
		FullBundleDigest:      "sha256:33a41f123cf0c20920e8b343887bd65fb535c9882b4244133e0243e2ce59e91a",
		EmbeddedContentDigest: "sha256:57fd7d60e6664a4fb55c9e1e4aeb76b8b3bd8e48cfd642f96e4ba9dbf638defa",
	},
	{
		Name:                  "viceme-danmaku",
		SkillVersion:          "0.19.0-beta.0",
		MinimumCLIVersion:     "0.19.0-beta.0",
		CLICompatibility:      ">=0.19.0-beta.0 <0.20.0",
		FullBundleDigest:      "sha256:1f77f655f71b23ead44e287694aebdd090d27cac4dc4b71802c72df79e3c05c9",
		EmbeddedContentDigest: "sha256:646882bc6c1bef7d6d8a863a1aa753c1797ffb5f2074c50ca5f1638bcc6402ca",
	},
	{
		Name:                  "viceme-engagement",
		SkillVersion:          "0.16.0-beta.6",
		MinimumCLIVersion:     "0.16.0-beta.6",
		CLICompatibility:      ">=0.16.0-beta.6 <0.17.0",
		FullBundleDigest:      "sha256:c7c992d83f95c697016c4ff2de1b8e7143a35e959dd98c1ab0a01819f514871b",
		EmbeddedContentDigest: "sha256:dd741e8c3d644a022bd705aadf66fb258d86a8d173dd5001e6b7abb1ac092cc8",
	},
	{
		Name:                  "viceme-engagement",
		SkillVersion:          "0.16.0-beta.7",
		MinimumCLIVersion:     "0.16.0-beta.7",
		CLICompatibility:      ">=0.16.0-beta.7 <0.17.0",
		FullBundleDigest:      "sha256:29e7a2f29270eff034bebf67781a891e9b62ff8eb7168b1069624acb54821855",
		EmbeddedContentDigest: "sha256:183d845ce62386f2427732695a7f93577b10258f6fe6e880b56940c6f16eea9b",
	},
	{
		Name:                  "viceme-engagement",
		SkillVersion:          "0.17.0",
		MinimumCLIVersion:     "0.17.0",
		CLICompatibility:      ">=0.17.0 <0.18.0",
		FullBundleDigest:      "sha256:18d6d59417876f884c7d5de0c44807941d37cc25c71bece69c5e3f9ea1514112",
		EmbeddedContentDigest: "sha256:baf7ae9b09b43baa21e00eefe6f136aa484efd307fd30294c73aa4921807650e",
	},
	{
		Name:                  "viceme-engagement",
		SkillVersion:          "0.18.0",
		MinimumCLIVersion:     "0.18.0",
		CLICompatibility:      ">=0.18.0 <0.19.0",
		FullBundleDigest:      "sha256:dc0ce94cd154d1af285bc5ab3c97c23fb80ed70d7b2d58dbe8ce5c2ecd4d142b",
		EmbeddedContentDigest: "sha256:baf7ae9b09b43baa21e00eefe6f136aa484efd307fd30294c73aa4921807650e",
	},
	{
		Name:                  "viceme-engagement",
		SkillVersion:          "0.19.0-beta.0",
		MinimumCLIVersion:     "0.19.0-beta.0",
		CLICompatibility:      ">=0.19.0-beta.0 <0.20.0",
		FullBundleDigest:      "sha256:5abe12818c001577f5e13d3cbda9350bc617c96094168fe99ff74eabce6b7cce",
		EmbeddedContentDigest: "sha256:25c3cd955939c410093e7ab05882ce776df1f7e08f2e2cb1e8599eda4f976b9f",
	},
}

type installNextStep struct {
	Required bool   `json:"required"`
	Command  string `json:"command"`
	Reason   string `json:"reason"`
}

type bootstrapInstallResult struct {
	Launcher        updatepkg.TargetResult       `json:"launcher"`
	Skills          []skillcontent.InstallReport `json:"skills"`
	Config          config.EnsureResult          `json:"config"`
	Profile         string                       `json:"profile"`
	Region          config.Region                `json:"region"`
	Authenticated   bool                         `json:"authenticated"`
	AuthStatusKnown bool                         `json:"authStatusKnown"`
	Warnings        []string                     `json:"warnings,omitempty"`
	NextStep        installNextStep              `json:"nextStep"`
}

type installCommitAuthority struct {
	PrepareLauncher         func(context.Context) (updatepkg.TargetResult, error)
	BeforeCommit            func() error
	AfterCommit             func() error
	OuterJournalOwnsFailure bool
}

func newInstallCommand(runtime *Runtime) *cobra.Command {
	var agent string
	var region string
	var skipLauncherEnsure bool
	var activationChildNonce string
	var activationTarget string
	command := &cobra.Command{
		Use: "install", Short: "Install official ViceMe Skills for supported AI coding agents", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			internalFlagsPresent := skipLauncherEnsure || activationChildNonce != "" || activationTarget != ""
			if internalFlagsPresent {
				request := runtime.deps.activationChildRequest
				if !runtime.deps.coordinatedActivationChild || !skipLauncherEnsure ||
					activationChildNonce != request.Nonce || activationTarget != request.TargetVersion || agent != request.SkillTarget {
					return output.Policy("ACTIVATION_CHILD_INVALID", "internal activation flags require the exact committing parent journal")
				}
			}
			result, err := performAuthorizedInstall(command.Context(), runtime, agent, region)
			if err != nil {
				return err
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&agent, "agent", "auto", "agent target: auto, codex, claude, workbuddy, or agents")
	command.Flags().StringVar(&region, "region", "", "ViceMe region: cn or global")
	command.Flags().BoolVar(&skipLauncherEnsure, "internal-skip-launcher-ensure", false, "skip launcher persistence inside a coordinated activation")
	command.Flags().StringVar(&activationChildNonce, "internal-activation-child", "", "run inside the outer activation coordinator")
	command.Flags().StringVar(&activationTarget, "internal-activation-target", "", "bind the activation child to an exact target")
	_ = command.Flags().MarkHidden("internal-skip-launcher-ensure")
	_ = command.Flags().MarkHidden("internal-activation-child")
	_ = command.Flags().MarkHidden("internal-activation-target")
	return command
}

func performAuthorizedInstall(ctx context.Context, runtime *Runtime, agent, region string) (bootstrapInstallResult, error) {
	if runtime.deps.coordinatedActivationChild {
		return performNPMChildInstall(ctx, runtime, agent, region)
	}
	return performOrdinaryInstall(ctx, runtime, agent, region)
}

func performOrdinaryInstall(ctx context.Context, runtime *Runtime, agent, region string) (bootstrapInstallResult, error) {
	if err := os.MkdirAll(runtime.configBase, 0o700); err != nil {
		return bootstrapInstallResult{}, output.Internal("INSTALL_AUTHORITY_FAILED", "could not create the activation directory", err)
	}
	activationLock := flock.New(filepath.Join(runtime.configBase, updatepkg.ActivationLockFilename))
	locked, err := activationLock.TryLock()
	if err != nil {
		return bootstrapInstallResult{}, output.Internal("INSTALL_AUTHORITY_FAILED", "could not acquire the activation lock", err)
	}
	if !locked {
		return bootstrapInstallResult{}, output.Validation("INSTALL_ACTIVE", "another ViceMe bootstrap or update is active")
	}
	defer activationLock.Unlock()
	memberLock := flock.New(filepath.Join(runtime.configBase, updatepkg.ActivationMemberLockFilename))
	memberLocked, err := memberLock.TryLock()
	if err != nil {
		return bootstrapInstallResult{}, output.Internal("INSTALL_AUTHORITY_FAILED", "could not inspect the activation member lock", err)
	}
	if !memberLocked {
		return bootstrapInstallResult{}, output.Validation("INSTALL_ACTIVE", "an activation child is still committing Skills and config")
	}
	defer memberLock.Unlock()

	expected, err := expectedRunningGeneration(runtime)
	if err != nil {
		return bootstrapInstallResult{}, output.Internal("INSTALL_AUTHORITY_FAILED", "could not identify the running CLI generation", err)
	}
	activeInitially, activeExists, err := validateInstallAuthority(runtime.configBase, expected, nil)
	if err != nil {
		return bootstrapInstallResult{}, output.Policy("INSTALL_GENERATION_CHANGED", "the active CLI generation changed; restart the command")
	}
	authority := &installCommitAuthority{BeforeCommit: func() error {
		active, exists, err := validateInstallAuthority(runtime.configBase, expected, &activeInitially)
		if err != nil || exists != activeExists || (exists && active != activeInitially) {
			return updatepkg.ErrActivationRestartNeeded
		}
		if !exists {
			return updatepkg.CommitActiveGeneration(runtime.configBase, expected)
		}
		return nil
	}}
	if service, ok := runtime.deps.Updater.(*updatepkg.NPMService); ok && service.InstallMethod == "npm" {
		var childNonce string
		authority.OuterJournalOwnsFailure = true
		authority.PrepareLauncher = func(ctx context.Context) (updatepkg.TargetResult, error) {
			launcher, nonce, err := service.PrepareCoordinatedInstallWhileLocked(ctx, agent)
			if err == nil {
				childNonce = nonce
			}
			return launcher, err
		}
		authority.BeforeCommit = func() error {
			outer, err := updatepkg.InspectOuterActivationJournals(runtime.configBase)
			if err != nil || outer.Bootstrap || !outer.NPM {
				return updatepkg.ErrActivationRestartNeeded
			}
			active, exists, err := updatepkg.ReadActiveGeneration(runtime.configBase)
			if err != nil || exists != activeExists || (exists && active != activeInitially) {
				return updatepkg.ErrActivationRestartNeeded
			}
			target, err := service.ValidateActivationChild(childNonce, expected.Version, agent)
			if err != nil || target != expected {
				return updatepkg.ErrActivationRestartNeeded
			}
			return nil
		}
		authority.AfterCommit = func() error {
			if err := service.ConfirmActivationChildCommitted(childNonce, expected.Version, agent); err != nil {
				return err
			}
			return service.RecoverActivationWhileLocked(ctx)
		}
	}
	return performInstall(ctx, runtime, agent, region, true, authority)
}

func performNPMChildInstall(ctx context.Context, runtime *Runtime, agent, region string) (bootstrapInstallResult, error) {
	memberLock := flock.New(filepath.Join(runtime.configBase, updatepkg.ActivationMemberLockFilename))
	memberLocked, err := memberLock.TryLock()
	if err != nil {
		return bootstrapInstallResult{}, output.Internal("ACTIVATION_CHILD_LOCK_FAILED", "could not acquire the activation child lock", err)
	}
	if !memberLocked {
		return bootstrapInstallResult{}, output.Validation("ACTIVATION_CHILD_ACTIVE", "another activation child is committing Skills and config")
	}
	defer memberLock.Unlock()
	request := runtime.deps.activationChildRequest
	service := npmRecoveryService(runtime.configBase, runtime.deps)
	validate := func() error {
		target, err := service.ValidateActivationChild(request.Nonce, request.TargetVersion, request.SkillTarget)
		if err != nil {
			return err
		}
		expected, err := expectedRunningGeneration(runtime)
		if err != nil {
			return err
		}
		if target != expected {
			return updatepkg.ErrActivationRestartNeeded
		}
		return nil
	}
	if err := validate(); err != nil {
		return bootstrapInstallResult{}, output.Policy("ACTIVATION_CHILD_INVALID", "the activation child no longer owns its parent journal")
	}
	authority := &installCommitAuthority{
		BeforeCommit: validate,
		AfterCommit: func() error {
			return service.ConfirmActivationChildCommitted(request.Nonce, request.TargetVersion, request.SkillTarget)
		},
	}
	return performInstall(ctx, runtime, agent, region, false, authority)
}

func expectedRunningGeneration(runtime *Runtime) (updatepkg.ActiveGeneration, error) {
	if runtime.deps.runningActivationGeneration != nil {
		return *runtime.deps.runningActivationGeneration, nil
	}
	return runningActivationGeneration(runtime.deps)
}

func validateInstallAuthority(configDir string, expected updatepkg.ActiveGeneration, initial *updatepkg.ActiveGeneration) (updatepkg.ActiveGeneration, bool, error) {
	outer, err := updatepkg.InspectOuterActivationJournals(configDir)
	if err != nil {
		return updatepkg.ActiveGeneration{}, false, err
	}
	if outer.Bootstrap || outer.NPM {
		return updatepkg.ActiveGeneration{}, false, errors.New("an outer activation journal is pending")
	}
	active, exists, err := updatepkg.ReadActiveGeneration(configDir)
	if err != nil {
		return updatepkg.ActiveGeneration{}, false, err
	}
	if exists && active != expected {
		return active, true, updatepkg.ErrActivationRestartNeeded
	}
	if initial != nil && exists && active != *initial {
		return active, true, updatepkg.ErrActivationRestartNeeded
	}
	return active, exists, nil
}

func performInstall(ctx context.Context, runtime *Runtime, agent, region string, ensureLauncher bool, authority *installCommitAuthority) (bootstrapInstallResult, error) {
	if region == "" {
		region = string(runtime.region)
	}
	resolvedRegion, err := config.ParseRegion(region)
	if err != nil {
		return bootstrapInstallResult{}, output.Validation("REGION_INVALID", err.Error())
	}
	installContext, cancel := context.WithTimeout(ctx, activationOperationTimeout)
	defer cancel()
	expectedGeneration, err := expectedRunningGeneration(runtime)
	if err != nil {
		return bootstrapInstallResult{}, output.Internal("SKILL_RELEASE_MANIFEST_INVALID", "could not identify the running release for official Skills", err)
	}
	skillNames, err := officialSkillsForRelease(runtime.deps.Skills, expectedGeneration)
	if err != nil {
		return bootstrapInstallResult{}, output.Internal("SKILL_RELEASE_MANIFEST_INVALID", "embedded official Skills do not match the running release manifest", err)
	}
	for _, name := range skillNames {
		if err := runtime.deps.Skills.Validate(name); err != nil {
			return bootstrapInstallResult{}, err
		}
	}
	transaction, reports, err := runtime.deps.Skills.PrepareInstallSetWithRetirements(
		skillNames, retiredOfficialSkills, agent, runtime.deps.Environment,
	)
	if err != nil {
		return bootstrapInstallResult{}, output.Internal("SKILL_INSTALL_PREPARE_FAILED", "official Skills could not be prepared as one transaction", err)
	}
	rollback := func(cause error) (bootstrapInstallResult, error) {
		if rollbackErr := transaction.Rollback(); rollbackErr != nil {
			return bootstrapInstallResult{}, output.Internal("INSTALL_ROLLBACK_FAILED", "ViceMe installation failed and the previous generation could not be fully restored", errors.Join(cause, rollbackErr))
		}
		return bootstrapInstallResult{}, cause
	}
	if len(reports) != len(skillNames) {
		return rollback(output.Internal("SKILL_INSTALL_TRANSACTION_INVALID", "official Skill installation returned an incomplete report", nil))
	}
	for _, report := range reports {
		if !report.AllSucceeded {
			return rollback(output.Internal("SKILL_INSTALL_PARTIAL", "official Skills were not activated", nil).WithDetails(reports))
		}
	}
	doctorResults := make([]doctorSkillResult, 0, len(skillNames))
	for _, name := range skillNames {
		report := runtime.deps.Skills.Doctor(name, agent, runtime.deps.Environment)
		doctorResults = append(doctorResults, doctorSkillResult{Name: name, Report: report})
		if !report.Healthy {
			return rollback(output.Validation("SKILL_INSTALL_VERIFICATION_FAILED", "installed official Skills did not pass Doctor").WithDetails(doctorResults))
		}
	}
	profile, err := runtime.config.Resolve(runtime.profile.Name)
	if err != nil {
		return rollback(output.Internal("PROFILE_INVALID", "could not resolve the active profile", err))
	}
	if _, statErr := os.Stat(config.ConfigPath(runtime.configBase)); errors.Is(statErr, fs.ErrNotExist) {
		if err := runtime.config.SetProfileAuthority(
			profile.Name,
			config.APIBaseURL(resolvedRegion),
			config.WebBaseURL(resolvedRegion),
			resolvedRegion,
		); err != nil {
			return rollback(output.Internal("PROFILE_INVALID", "could not initialize the selected Profile authority", err))
		}
	} else if statErr != nil {
		return rollback(output.Internal("PROFILE_BACKUP_FAILED", "could not inspect the CLI configuration", statErr))
	}
	runtime.config.DistributionRegion = resolvedRegion
	if err := transaction.TrackPath(config.ConfigPath(runtime.configBase)); err != nil {
		return rollback(output.Internal("PROFILE_BACKUP_FAILED", "could not preserve the previous CLI configuration", err))
	}
	configResult, err := config.Save(runtime.configBase, runtime.config)
	if err != nil {
		return rollback(output.Internal("PROFILE_SAVE_FAILED", "could not initialize CLI configuration", err))
	}
	if err := runtime.reloadConfig(profile.Name); err != nil {
		return rollback(err)
	}
	network := checkDoctorNetwork(ctx, runtime)
	authenticated, authStatusKnown, warnings := installAuthenticationStatus(runtime)
	if !network.Healthy {
		warnings = append(warnings, "the active profile API is unreachable; installation completed, configure the intended profile and run viceme doctor")
	}
	if err := transaction.MarkCommitting(); err != nil {
		return rollback(output.Internal("INSTALL_COMMIT_PREPARE_FAILED", "could not persist the verified installation commit point", err))
	}
	launcher := updatepkg.TargetResult{Target: "launcher", Status: "coordinated"}
	if ensureLauncher {
		if authority != nil && authority.PrepareLauncher != nil {
			launcher, err = authority.PrepareLauncher(installContext)
		} else {
			launcher, err = runtime.deps.Updater.EnsureLauncher(installContext)
		}
		if err != nil {
			return rollback(updaterError(err, launcher))
		}
	}
	if authority != nil && authority.BeforeCommit != nil {
		if err := authority.BeforeCommit(); err != nil {
			if authority.OuterJournalOwnsFailure {
				transaction.Abandon()
				return bootstrapInstallResult{}, err
			}
			return rollback(err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return bootstrapInstallResult{}, output.Internal("INSTALL_COMMIT_FAILED", "ViceMe installation was verified but could not commit its recovery journal", err)
	}
	if authority != nil && authority.AfterCommit != nil {
		if err := authority.AfterCommit(); err != nil {
			return bootstrapInstallResult{}, output.Internal("INSTALL_AUTHORITY_COMMIT_FAILED", "ViceMe installation committed but its activation authority could not be finalized", err)
		}
	}
	result := bootstrapInstallResult{
		Launcher:        launcher,
		Skills:          reports,
		Config:          configResult,
		Profile:         profile.Name,
		Region:          resolvedRegion,
		Authenticated:   authenticated,
		AuthStatusKnown: authStatusKnown,
		Warnings:        warnings,
	}
	if authenticated {
		result.NextStep = installNextStep{Command: "viceme skill publish --path <dir-or-zip>", Reason: "upload a private Draft and open its Owner Preview"}
	} else {
		result.NextStep = installNextStep{Required: true, Command: "viceme auth login", Reason: "sign in before publishing a Skill"}
	}
	return result, nil
}

func installAuthenticationStatus(runtime *Runtime) (authenticated, known bool, warnings []string) {
	if token, _, _ := runtime.overrideCredential(); token != "" {
		return true, true, nil
	}
	status, err := runtime.manager().CurrentStatus()
	if err != nil {
		return false, false, []string{"authentication status could not be read from the secure credential store"}
	}
	return status.Authenticated, true, nil
}

type doctorSkillResult struct {
	Name   string                    `json:"name"`
	Report skillcontent.DoctorReport `json:"report"`
}

type doctorNetworkResult struct {
	Healthy bool   `json:"healthy"`
	Code    string `json:"code,omitempty"`
	Problem string `json:"problem,omitempty"`
}

func checkDoctorNetwork(ctx context.Context, runtime *Runtime) doctorNetworkResult {
	probeContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := runtime.client().HealthReady(probeContext); err != nil {
		cliError := output.AsError(err)
		return doctorNetworkResult{Healthy: false, Code: cliError.Subtype, Problem: cliError.Message}
	}
	return doctorNetworkResult{Healthy: true}
}

func newDoctorCommand(runtime *Runtime) *cobra.Command {
	var agent string
	command := &cobra.Command{
		Use: "doctor", Short: "Check the CLI, profile, credentials, and official Skills", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			results := make([]doctorSkillResult, 0, len(officialSkillNames))
			healthy := true
			for _, name := range officialSkillNames {
				report := runtime.deps.Skills.Doctor(name, agent, runtime.deps.Environment)
				results = append(results, doctorSkillResult{Name: name, Report: report})
				healthy = healthy && report.Healthy
			}
			status, err := runtime.manager().CurrentStatus()
			if err != nil {
				return err
			}
			network := checkDoctorNetwork(command.Context(), runtime)
			healthy = healthy && network.Healthy
			result := map[string]any{
				"healthy": healthy, "profile": runtime.profile.Name, "distributionRegion": runtime.region,
				"authenticated": status.Authenticated, "network": network, "skills": results,
			}
			if !healthy {
				return output.Validation("DOCTOR_UNHEALTHY", "ViceMe CLI or official Skill installation is unhealthy").WithDetails(result)
			}
			return runtime.business(result)
		},
	}
	command.Flags().StringVar(&agent, "agent", "auto", "agent target: auto, codex, claude, workbuddy, or agents")
	return command
}
