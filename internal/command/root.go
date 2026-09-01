package command

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	cliembed "github.com/ViceMe-AI/cli"
	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/semver"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	updatepkg "github.com/ViceMe-AI/cli/internal/update"
	"github.com/gofrs/flock"
	"github.com/spf13/cobra"
)

type Dependencies struct {
	In          io.Reader
	Out         io.Writer
	ErrOut      io.Writer
	HTTPClient  *http.Client
	Store       securestore.Store
	Skills      *skillcontent.Bundle
	Updater     updatepkg.Service
	Environment skillcontent.Environment
	Now         func() time.Time
	Sleep       func(context.Context, time.Duration) error
	NewID       func() string
	APIBaseURL  string
	Region      config.Region
	Reexecute   func(context.Context, []string, []string) (int, error)

	coordinatedActivationChild  bool
	activationChildRequest      npmActivationChildRequest
	activationChildParseError   error
	bootstrapActivationCommand  bool
	activationNPMRecoverer      *updatepkg.NPMService
	runningActivationGeneration *updatepkg.ActiveGeneration
	allowDevelopmentAutoUpdate  bool
}

type npmActivationChildRequest struct {
	Requested     bool
	SkipLauncher  bool
	Nonce         string
	TargetVersion string
	SkillTarget   string
}

type options struct {
	version bool
	profile string
}

type Runtime struct {
	deps               Dependencies
	opts               options
	printer            *output.Printer
	region             config.Region
	apiBaseURL         string
	apiBaseURLOverride string
	apiBaseURLFromEnv  bool
	credentialScope    string
	config             config.Config
	profile            config.Profile
	configBase         string
	processCredential  *publicationCredential
	commerceContextID  string
}

const (
	apiBaseURLEnvironment         = "VICEME_API_BASE_URL"
	processAccessTokenEnvironment = "VICEME_ACCESS_TOKEN"
	autoUpdateReexecEnvironment   = "VICEME_AUTO_UPDATE_REEXEC"
	autoUpdateFromEnvironment     = "VICEME_AUTO_UPDATE_FROM"
	autoUpdateToEnvironment       = "VICEME_AUTO_UPDATE_TO"
	npmLauncherPathEnvironment    = "VICEME_NPM_LAUNCHER_PATH"
	npmLauncherRuntimeEnvironment = "VICEME_NPM_LAUNCHER_RUNTIME"
	activationOperationTimeout    = 12 * time.Minute
)

var fallbackUUIDSequence atomic.Uint64

type publicationCredential struct {
	raw string
}

type processTokenSource string

func (source processTokenSource) Token(context.Context) (string, error) {
	return string(source), nil
}

func Execute(args []string, dependencies Dependencies) int {
	dependencies.activationChildRequest, dependencies.activationChildParseError = parseNPMActivationChild(args)
	dependencies.bootstrapActivationCommand = isBootstrapActivationCommand(args)
	dependencies = defaults(dependencies)
	root, runtime, err := NewRoot(dependencies)
	if err != nil {
		printer := &output.Printer{Out: writerOr(dependencies.Out, os.Stdout), ErrOut: writerOr(dependencies.ErrOut, os.Stderr), ExecutingCLIVersion: buildinfo.Version}
		if errors.Is(err, updatepkg.ErrActivationRestartNeeded) && os.Getenv(autoUpdateReexecEnvironment) != "1" {
			active, exists, readErr := updatepkg.ReadActiveGeneration(runtimeConfigBase(dependencies.Environment))
			if readErr == nil && exists {
				return reexecuteOriginalCommand(args, dependencies, printer, buildinfo.CompatibilityVersion(), active.Version)
			}
		}
		return printer.Failure(err)
	}
	root.SetArgs(args)
	if err := root.ExecuteContext(context.Background()); err != nil {
		var applied *automaticUpdateApplied
		if errors.As(err, &applied) {
			if applied.Scheduled {
				failure := output.NewError(
					output.ExitInternal,
					"internal",
					"AUTO_UPDATE_RESTART_REQUIRED",
					"ViceMe scheduled a complete CLI and Skill update; rerun the command after activation finishes",
				)
				failure.Retryable = true
				failure.Hint = "rerun the same command"
				return runtime.printer.Failure(failure)
			}
			return reexecuteOriginalCommand(args, runtime.deps, runtime.printer, applied.From, applied.To)
		}
		return runtime.failure(err)
	}
	return 0
}

func reexecuteOriginalCommand(args []string, dependencies Dependencies, printer *output.Printer, from, to string) int {
	environment := withEnvironmentValues(os.Environ(), map[string]string{
		autoUpdateReexecEnvironment: "1",
		autoUpdateFromEnvironment:   from,
		autoUpdateToEnvironment:     to,
	})
	exitCode, err := dependencies.Reexecute(context.Background(), args, environment)
	if err != nil {
		return printer.Failure(output.Internal(
			"AUTO_UPDATE_REEXEC_FAILED",
			"ViceMe updated successfully but could not continue the original command with the new version",
			err,
		))
	}
	return exitCode
}

func NewRoot(dependencies Dependencies) (*cobra.Command, *Runtime, error) {
	if err := buildinfo.ValidateNPMLaunch(
		os.Getenv("VICEME_INSTALL_METHOD"),
		os.Getenv("VICEME_NPM_PACKAGE_VERSION"),
		buildinfo.Version,
	); err != nil {
		return nil, nil, output.Internal("launcher_version_mismatch", "npm launcher and Go binary versions do not match", err)
	}
	dependencies = defaults(dependencies)
	configBase := runtimeConfigBase(dependencies.Environment)
	if dependencies.activationChildParseError != nil {
		return nil, nil, output.Validation("ACTIVATION_CHILD_INVALID", dependencies.activationChildParseError.Error())
	}
	if dependencies.activationChildRequest.Requested {
		if err := authorizeNPMActivationChild(configBase, &dependencies); err != nil {
			return nil, nil, output.Policy("ACTIVATION_CHILD_INVALID", "the internal activation child is not authorized by a committing parent journal")
		}
		dependencies.coordinatedActivationChild = true
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), activationOperationTimeout)
		err := reconcileActivationAtStartup(ctx, configBase, &dependencies)
		cancel()
		if err != nil {
			return nil, nil, output.Internal("ACTIVATION_RECOVERY_FAILED", "could not reconcile the active ViceMe CLI and Skill generation", err)
		}
	}
	if err := skillcontent.RecoverInstallTransactionAuto(dependencies.Environment); err != nil {
		return nil, nil, output.Internal("INSTALL_RECOVERY_FAILED", "could not reconcile an interrupted ViceMe installation", err)
	}
	resolvedConfig := config.Default(config.RegionCN)
	if dependencies.Region == "" {
		var err error
		resolvedConfig, err = config.LoadOrDefault(configBase)
		if err != nil {
			return nil, nil, configLoadFailure("could not load ViceMe CLI configuration", err)
		}
	} else {
		resolvedRegion, err := config.ParseRegion(string(dependencies.Region))
		if err != nil {
			return nil, nil, output.Internal("config_region", "invalid injected ViceMe region", err)
		}
		resolvedConfig = config.Default(resolvedRegion)
	}
	resolvedProfile, err := resolvedConfig.Resolve("")
	if err != nil {
		return nil, nil, output.Internal("config_profile", "could not resolve the active ViceMe CLI profile", err)
	}
	region := resolvedConfig.DistributionRegion
	apiBaseURLOverride := dependencies.APIBaseURL
	apiBaseURLFromEnv := false
	if apiBaseURLOverride == "" {
		apiBaseURLOverride = os.Getenv(apiBaseURLEnvironment)
		apiBaseURLFromEnv = apiBaseURLOverride != ""
	}
	processCredential, err := parsePublicationCredential(os.Getenv(processAccessTokenEnvironment))
	if err != nil {
		return nil, nil, output.Authentication("process_credential_invalid", err.Error())
	}
	runtime := &Runtime{
		deps:               dependencies,
		region:             region,
		apiBaseURLOverride: apiBaseURLOverride,
		apiBaseURLFromEnv:  apiBaseURLFromEnv,
		config:             resolvedConfig,
		profile:            *resolvedProfile,
		configBase:         configBase,
		processCredential:  processCredential,
		printer: &output.Printer{
			Out:                 dependencies.Out,
			ErrOut:              dependencies.ErrOut,
			ExecutingCLIVersion: buildinfo.Version,
			AutoUpdate:          automaticUpdateMetaFromEnvironment(),
		},
	}
	if err := runtime.selectProfile(resolvedProfile.Name); err != nil {
		return nil, nil, err
	}
	root := &cobra.Command{
		Use:           "viceme",
		Short:         "Publish Skills and manage ViceMe creator tooling",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if runtime.opts.version {
				return runtime.writeVersion()
			}
			cmd.SetOut(runtime.deps.ErrOut)
			if err := cmd.Help(); err != nil {
				return output.Internal("help_render_failed", "could not render ViceMe command help", err)
			}
			return runtime.business(map[string]any{
				"command": "viceme",
				"help":    "human-readable command help was written to stderr",
			})
		},
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetIn(dependencies.In)
	root.SetOut(dependencies.Out)
	root.SetErr(dependencies.ErrOut)
	root.Flags().BoolVarP(&runtime.opts.version, "version", "v", false, "print version information")
	root.PersistentFlags().StringVar(&runtime.opts.profile, "profile", "", "use a specific profile for this command")
	root.PersistentPreRunE = func(command *cobra.Command, _ []string) error {
		if err := runtime.validateProfileOverrideAuthority(runtime.opts.profile); err != nil {
			return err
		}
		if err := runtime.selectProfile(runtime.opts.profile); err != nil {
			return err
		}
		if err := runtime.ensureAutomaticUpdate(command); err != nil {
			return err
		}
		if commerceCommandRequested(command) {
			if err := pruneCommercePaymentPresentations(runtime); err != nil {
				return output.Internal(
					"COMMERCE_PAYMENT_PRESENTATION_CLEANUP_FAILED",
					"stale local payment QR files could not be removed",
					err,
				)
			}
		}
		return nil
	}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return output.Validation("invalid_flag", err.Error())
	})
	root.AddCommand(newVersionCommand(runtime))
	root.AddCommand(newBootstrapCommand(runtime))
	root.AddCommand(newInstallCommand(runtime))
	root.AddCommand(newDoctorCommand(runtime))
	root.AddCommand(newUpdateCommand(runtime))
	root.AddCommand(newAuthCommand(runtime))
	root.AddCommand(newProfileCommand(runtime))
	root.AddCommand(newSkillCommand(runtime))
	root.AddCommand(newPublicationCommand(runtime))
	root.AddCommand(newSubscriptionCommand(runtime))
	root.AddCommand(newMerchantCommand(runtime))
	root.AddCommand(newCommerceCommand(runtime))
	return root, runtime, nil
}

func parseNPMActivationChild(args []string) (npmActivationChildRequest, error) {
	request := npmActivationChildRequest{SkillTarget: "auto"}
	install := len(args) > 0 && args[0] == "install"
	readValue := func(index *int, argument, name string) (string, bool, error) {
		prefix := name + "="
		if strings.HasPrefix(argument, prefix) {
			value := strings.TrimPrefix(argument, prefix)
			if value == "" {
				return "", true, fmt.Errorf("%s requires a value", name)
			}
			return value, true, nil
		}
		if argument != name {
			return "", false, nil
		}
		if *index+1 >= len(args) || strings.HasPrefix(args[*index+1], "--") {
			return "", true, fmt.Errorf("%s requires a value", name)
		}
		*index = *index + 1
		return args[*index], true, nil
	}
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if argument == "install" {
			continue
		}
		if argument == "--internal-skip-launcher-ensure" {
			request.Requested = true
			request.SkipLauncher = true
			continue
		}
		if value, matched, err := readValue(&index, argument, "--internal-activation-child"); matched {
			request.Requested = true
			if err != nil {
				return request, err
			}
			request.Nonce = value
			continue
		}
		if value, matched, err := readValue(&index, argument, "--internal-activation-target"); matched {
			request.Requested = true
			if err != nil {
				return request, err
			}
			request.TargetVersion = value
			continue
		}
		if value, matched, err := readValue(&index, argument, "--agent"); matched {
			if err != nil {
				return request, err
			}
			request.SkillTarget = value
		}
	}
	if !request.Requested {
		return request, nil
	}
	if !install || !request.SkipLauncher || request.Nonce == "" || request.TargetVersion == "" {
		return request, errors.New("internal activation flags require a complete coordinated install child")
	}
	return request, nil
}

func isBootstrapActivationCommand(args []string) bool {
	return len(args) >= 2 && args[0] == "bootstrap" && args[1] == "activate"
}

func reconcileActivationAtStartup(ctx context.Context, configDir string, dependencies *Dependencies) error {
	running, err := runningActivationGeneration(*dependencies)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return err
	}
	activationLock := flock.New(filepath.Join(configDir, updatepkg.ActivationLockFilename))
	locked, err := activationLock.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil {
		return err
	}
	if !locked {
		return errors.New("another ViceMe bootstrap or update is active")
	}
	defer activationLock.Unlock()
	memberLock := flock.New(filepath.Join(configDir, updatepkg.ActivationMemberLockFilename))
	memberAvailable, err := memberLock.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil {
		return fmt.Errorf("an activation child is still committing Skills and config: %w", err)
	}
	if !memberAvailable {
		return errors.New("an activation child is still committing Skills and config")
	}
	_ = memberLock.Unlock()

	outer, err := updatepkg.InspectOuterActivationJournals(configDir)
	if err != nil {
		return err
	}
	if outer.Bootstrap && outer.NPM {
		return errors.New("standalone and npm activation journals cannot be recovered together")
	}
	if err := recoverBootstrapActivation(configDir, dependencies.Environment); err != nil {
		return err
	}
	recoverer, ok := dependencies.Updater.(updatepkg.LockedStartupRecoverer)
	if !ok {
		recoverer = npmRecoveryService(configDir, *dependencies)
	}
	if err := recoverer.RecoverActivationWhileLocked(ctx); err != nil {
		return err
	}
	active, exists, err := updatepkg.ReadActiveGeneration(configDir)
	if err != nil {
		return err
	}
	if exists && active != running && !dependencies.bootstrapActivationCommand {
		return updatepkg.ErrActivationRestartNeeded
	}
	dependencies.runningActivationGeneration = &running
	return nil
}

func authorizeNPMActivationChild(configDir string, dependencies *Dependencies) error {
	outer, err := updatepkg.InspectOuterActivationJournals(configDir)
	if err != nil {
		return err
	}
	if outer.Bootstrap {
		return errors.New("a standalone activation journal is pending")
	}
	request := dependencies.activationChildRequest
	service := npmRecoveryService(configDir, *dependencies)
	target, err := service.ValidateActivationChild(request.Nonce, request.TargetVersion, request.SkillTarget)
	if err != nil {
		return err
	}
	running, err := runningActivationGeneration(*dependencies)
	if err != nil {
		return err
	}
	if running != target {
		return errors.New("activation child is not running the journal target generation")
	}
	dependencies.runningActivationGeneration = &running
	return nil
}

func npmRecoveryService(configDir string, dependencies Dependencies) *updatepkg.NPMService {
	if dependencies.activationNPMRecoverer != nil {
		if dependencies.activationNPMRecoverer.ConfigDir == "" {
			dependencies.activationNPMRecoverer.ConfigDir = configDir
		}
		return dependencies.activationNPMRecoverer
	}
	if service, ok := dependencies.Updater.(*updatepkg.NPMService); ok {
		if service.ConfigDir == "" {
			service.ConfigDir = configDir
		}
		return service
	}
	service := updatepkg.NewNPMService(
		buildinfo.Version,
		buildinfo.CompatibilityVersion(),
		"npm",
	)
	service.ConfigDir = configDir
	service.HTTPClient = dependencies.HTTPClient
	return service
}

func runningActivationGeneration(dependencies Dependencies) (updatepkg.ActiveGeneration, error) {
	version := buildinfo.CompatibilityVersion()
	installMethod := os.Getenv("VICEME_INSTALL_METHOD")
	switch service := dependencies.Updater.(type) {
	case *updatepkg.NPMService:
		if service.ComparableVersion != "" {
			version = service.ComparableVersion
		}
		if service.InstallMethod == "npm" {
			installMethod = "npm"
		}
	case *updatepkg.ReleaseService:
		if service.ComparableVersion != "" {
			version = service.ComparableVersion
		}
	}
	if installMethod == "npm" {
		return updatepkg.NewNPMGeneration(version)
	}
	executable, err := os.Executable()
	if err != nil {
		return updatepkg.ActiveGeneration{}, err
	}
	digest, err := bootstrapFileHash(executable)
	if err != nil {
		return updatepkg.ActiveGeneration{}, err
	}
	return updatepkg.NewStandaloneGeneration(version, digest)
}

type automaticUpdateApplied struct {
	From      string
	To        string
	Scheduled bool
}

func (applied *automaticUpdateApplied) Error() string {
	return "automatic CLI and Skill update applied"
}

func (r *Runtime) ensureAutomaticUpdate(command *cobra.Command) error {
	if command == nil || command.Name() == "update" || r.deps.coordinatedActivationChild ||
		r.deps.bootstrapActivationCommand || os.Getenv(autoUpdateReexecEnvironment) == "1" {
		return nil
	}
	if _, err := semver.Parse(buildinfo.Version); err != nil && !r.deps.allowDevelopmentAutoUpdate {
		return nil
	}
	checker, ok := r.deps.Updater.(updatepkg.AutomaticChecker)
	if !ok {
		return nil
	}
	checkContext, cancelCheck := context.WithTimeout(command.Context(), 5*time.Second)
	check, err := checker.CheckAutomatic(checkContext)
	cancelCheck()
	if err != nil {
		// Release discovery is fail-open. The current process is already a
		// complete verified generation and remains usable while offline.
		return nil
	}
	if !check.UpdateAvailable {
		return nil
	}
	if err := updatepkg.ProbeRenameCapability(r.configBase); err != nil {
		// Activation is fail-open, like release discovery: the current process
		// is already a complete verified generation. An agent sandbox that
		// denies renames cannot replace the executable, and failing here would
		// break every business command instead of just the update.
		if errors.Is(err, updatepkg.ErrRenameDenied) {
			_, _ = fmt.Fprintln(r.deps.ErrOut, "Automatic CLI update skipped: this environment cannot activate a new ViceMe generation; run 'viceme update' from an unsandboxed terminal.")
		}
		return nil
	}
	_, _ = fmt.Fprintf(r.deps.ErrOut, "Updating ViceMe CLI and official Skills %s -> %s; the original command will continue automatically.\n", check.CurrentVersion, check.AvailableVersion)
	applyContext, cancelApply := context.WithTimeout(command.Context(), activationOperationTimeout)
	result, err := r.deps.Updater.Apply(applyContext, check, updatepkg.ApplyOptions{RefreshSkills: true, SkillTarget: "auto"})
	cancelApply()
	if err != nil {
		return updaterError(err, result)
	}
	scheduled := false
	for _, target := range result.Targets {
		if target.Status == "scheduled" {
			scheduled = true
			break
		}
	}
	return &automaticUpdateApplied{From: check.CurrentVersion, To: result.CLIVersion, Scheduled: scheduled}
}

func defaults(dependencies Dependencies) Dependencies {
	if dependencies.In == nil {
		dependencies.In = os.Stdin
	}
	dependencies.Out = writerOr(dependencies.Out, os.Stdout)
	dependencies.ErrOut = writerOr(dependencies.ErrOut, os.Stderr)
	if dependencies.HTTPClient == nil {
		dependencies.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if dependencies.Skills == nil {
		dependencies.Skills = skillcontent.New(cliembed.EmbeddedSkills())
	}
	if dependencies.Environment.Home == "" {
		dependencies.Environment = skillcontent.DefaultEnvironment()
	}
	if dependencies.Store == nil {
		dependencies.Store = securestore.NewDefault("viceme-cli", runtimeConfigBase(dependencies.Environment))
	}
	if dependencies.Updater == nil {
		if os.Getenv("VICEME_INSTALL_METHOD") == "npm" {
			updater := updatepkg.NewNPMService(
				buildinfo.Version,
				buildinfo.CompatibilityVersion(),
				"npm",
			)
			updater.ConfigDir = runtimeConfigBase(dependencies.Environment)
			updater.HTTPClient = dependencies.HTTPClient
			dependencies.Updater = updater
		} else {
			updater := updatepkg.NewReleaseService(
				buildinfo.Version,
				buildinfo.CompatibilityVersion(),
			)
			updater.ConfigDir = runtimeConfigBase(dependencies.Environment)
			updater.HTTPClient = dependencies.HTTPClient
			dependencies.Updater = updater
		}
	}
	if dependencies.Now == nil {
		dependencies.Now = time.Now
	}
	if dependencies.Sleep == nil {
		dependencies.Sleep = sleepContext
	}
	if dependencies.NewID == nil {
		dependencies.NewID = randomUUID
	}
	if dependencies.Reexecute == nil {
		dependencies.Reexecute = func(ctx context.Context, args, environment []string) (int, error) {
			name, arguments, err := reexecutionCommand(args)
			if err != nil {
				return 0, err
			}
			command := exec.CommandContext(ctx, name, arguments...)
			command.Stdin = dependencies.In
			command.Stdout = dependencies.Out
			command.Stderr = dependencies.ErrOut
			command.Env = environment
			err = command.Run()
			if err == nil {
				return 0, nil
			}
			var exitError *exec.ExitError
			if errors.As(err, &exitError) {
				return exitError.ExitCode(), nil
			}
			return 0, err
		}
	}
	return dependencies
}

func reexecutionCommand(args []string) (string, []string, error) {
	if os.Getenv("VICEME_INSTALL_METHOD") == "npm" {
		launcher := os.Getenv(npmLauncherPathEnvironment)
		runtime := os.Getenv(npmLauncherRuntimeEnvironment)
		if launcher == "" || runtime == "" {
			return "", nil, errors.New("npm launcher did not provide its re-execution authority")
		}
		return runtime, append([]string{launcher}, args...), nil
	}
	executable, err := os.Executable()
	if err != nil {
		return "", nil, err
	}
	return executable, append([]string(nil), args...), nil
}

func withEnvironmentValues(environment []string, values map[string]string) []string {
	result := append([]string(nil), environment...)
	for name, value := range values {
		prefix := name + "="
		filtered := result[:0]
		for _, entry := range result {
			if !strings.HasPrefix(entry, prefix) {
				filtered = append(filtered, entry)
			}
		}
		result = append(filtered, prefix+value)
	}
	return result
}

func automaticUpdateMetaFromEnvironment() *output.AutoUpdateMeta {
	if os.Getenv(autoUpdateReexecEnvironment) != "1" {
		return nil
	}
	from := os.Getenv(autoUpdateFromEnvironment)
	to := os.Getenv(autoUpdateToEnvironment)
	if _, err := semver.Parse(from); err != nil {
		return nil
	}
	if _, err := semver.Parse(to); err != nil {
		return nil
	}
	return &output.AutoUpdateMeta{From: from, To: to, Status: "updated"}
}

func newVersionCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print CLI and bundled Skill versions",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runtime.writeVersion()
		},
	}
}

func (r *Runtime) manager() *auth.Manager {
	return &auth.Manager{
		Store:        r.deps.Store,
		Region:       string(r.region),
		ProfileID:    r.profile.ID,
		ProfileName:  r.profile.Name,
		Scope:        r.credentialScope,
		LegacyRegion: legacyCredentialRegionForAPIBase(r.apiBaseURL),
	}
}

func (r *Runtime) client() *api.Client {
	var tokens api.TokenSource = r.manager()
	if token, _, _ := r.overrideCredential(); token != "" {
		tokens = processTokenSource(token)
	}
	return api.NewClient(r.apiBaseURL, r.deps.HTTPClient, tokens, "viceme/"+buildinfo.Version)
}

func (r *Runtime) success(data any) error {
	return r.printer.Success(data)
}

func (r *Runtime) business(data any) error {
	return r.printer.Business(data)
}

func (r *Runtime) successWithMeta(data any, meta output.Meta) error {
	return r.printer.SuccessWithMeta(data, meta)
}

type versionResult struct {
	buildinfo.Info
	Skills map[string]skillcontent.Digests `json:"skills"`
}

func (r *Runtime) writeVersion() error {
	skills := make(map[string]skillcontent.Digests, len(officialSkillNames))
	for _, name := range officialSkillNames {
		digests, err := r.deps.Skills.Digests(name)
		if err != nil {
			return err
		}
		skills[name] = digests
	}
	return r.business(versionResult{
		Info:   buildinfo.Current(),
		Skills: skills,
	})
}

func (r *Runtime) failure(err error) int {
	var cliError *output.Error
	if !errorsAs(err, &cliError) {
		err = output.Validation("invalid_command", err.Error())
	}
	return r.printer.Failure(err)
}

func writerOr(value, fallback io.Writer) io.Writer {
	if value == nil {
		return fallback
	}
	return value
}

func (r *Runtime) setRegion(region config.Region) error {
	r.config.DistributionRegion = region
	return r.applyProfile(r.profile)
}

func (r *Runtime) selectProfile(name string) error {
	profile, err := r.config.Resolve(name)
	if err != nil {
		return output.Validation("profile_not_found", err.Error())
	}
	return r.applyProfile(*profile)
}

func (r *Runtime) validateProfileOverrideAuthority(name string) error {
	if name == "" || !r.apiBaseURLFromEnv {
		return nil
	}
	return output.Validation(
		"profile_api_base_url_conflict",
		"`--profile` cannot be combined with `VICEME_API_BASE_URL`; select one profile and endpoint authority",
	).WithHint(
		"Unset VICEME_API_BASE_URL to use the selected profile's configured endpoint, or omit --profile to use the process endpoint override.",
	)
}

func (r *Runtime) applyProfile(profile config.Profile) error {
	apiBaseURL := r.apiBaseURLOverride
	if apiBaseURL == "" {
		apiBaseURL = profile.ResolvedAPIBaseURL()
	}
	normalizedAPIBaseURL, err := config.NormalizeAPIBaseURL(apiBaseURL)
	if err != nil {
		return output.Validation("api_base_url", "ViceMe API base URL must use HTTPS; HTTP is allowed only for localhost or loopback development")
	}
	apiBaseURL = normalizedAPIBaseURL
	if err := validatePublicationProcessCredentialTarget(r.processCredential, apiBaseURL); err != nil {
		return err
	}
	scope, err := credentialScopeForAPIBase(apiBaseURL)
	if err != nil {
		return output.Validation("api_base_url", err.Error())
	}
	r.profile = profile
	r.region = r.config.DistributionRegion
	r.apiBaseURL = apiBaseURL
	r.credentialScope = scope
	if regionAware, ok := r.deps.Updater.(updatepkg.RegionAware); ok {
		regionAware.SetRegion(string(r.config.DistributionRegion))
	}
	return nil
}

func (r *Runtime) credentialScopeForProfile(profile config.Profile) (string, error) {
	return credentialScopeForAPIBase(profile.ResolvedAPIBaseURL())
}

func (r *Runtime) credentialStorageKeys() ([]string, error) {
	seen := make(map[string]struct{})
	keys := make([]string, 0, len(r.config.Profiles)*3+1)
	add := func(manager *auth.Manager) {
		key := manager.StorageKey()
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	for _, profile := range r.config.Profiles {
		for _, region := range []config.Region{config.RegionCN, config.RegionGlobal} {
			add(&auth.Manager{ProfileID: profile.ID, ProfileName: profile.Name, Region: string(region)})
		}
		scope, err := r.credentialScopeForProfile(profile)
		if err != nil {
			return nil, err
		}
		add(&auth.Manager{
			ProfileID: profile.ID, ProfileName: profile.Name,
			Region: string(r.config.DistributionRegion), Scope: scope,
		})
	}
	add(r.manager())
	return keys, nil
}

func (r *Runtime) overrideCredential() (token, source string, persistent bool) {
	if r.processCredential != nil {
		return r.processCredential.raw, "process", false
	}
	return "", "", false
}

func parsePublicationCredential(raw string) (*publicationCredential, error) {
	if raw == "" {
		return nil, nil
	}
	if strings.TrimSpace(raw) != raw || strings.ContainsAny(raw, "\r\n\x00") {
		return nil, errors.New("the process publication credential is invalid")
	}
	if !strings.HasPrefix(raw, "vme_cli_") || len(raw) != len("vme_cli_")+43 {
		return nil, errors.New("the process credential is not a ViceMe CLI token")
	}
	for _, character := range strings.TrimPrefix(raw, "vme_cli_") {
		if !((character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-' || character == '_') {
			return nil, errors.New("the process publication credential is invalid")
		}
	}
	return &publicationCredential{raw: raw}, nil
}

func validatePublicationProcessCredentialTarget(credential *publicationCredential, apiBaseURL string) error {
	if credential == nil {
		return nil
	}
	if err := validatePublicationCredentialTarget(apiBaseURL); err != nil {
		return output.Authentication("process_credential_origin_mismatch", err.Error())
	}
	return nil
}

func validatePublicationCredentialTarget(apiBaseURL string) error {
	origin, err := api.NormalizeAPIOrigin(apiBaseURL)
	if err != nil {
		return errors.New("the process credential target is invalid")
	}
	cn, _ := api.NormalizeAPIOrigin(config.APIBaseURL(config.RegionCN))
	global, _ := api.NormalizeAPIOrigin(config.APIBaseURL(config.RegionGlobal))
	if origin == cn || origin == global || isLoopbackOrigin(origin) {
		return nil
	}
	return errors.New("VICEME_ACCESS_TOKEN may only target an official ViceMe API origin or loopback development")
}

func isLoopbackOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "localhost" {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func (r *Runtime) reloadConfig(profileName string) error {
	resolved, err := config.LoadOrDefault(r.configBase)
	if err != nil {
		return configLoadFailure("could not reload ViceMe CLI configuration", err)
	}
	r.config = resolved
	return r.selectProfile(profileName)
}

func (r *Runtime) recordProfileUserID(userID string) error {
	if userID == "" {
		return nil
	}
	profile, err := r.config.Resolve(r.profile.Name)
	if err != nil {
		return output.Internal("config_profile", "could not update the active profile", err)
	}
	profile.UserID = userID
	if _, err := config.Save(r.configBase, r.config); err != nil {
		return output.Internal("config_save", "could not save the authenticated profile", err)
	}
	r.profile = *profile
	return nil
}

func runtimeConfigBase(environment skillcontent.Environment) string {
	if environment.ConfigDir != "" {
		return environment.ConfigDir
	}
	return filepath.Join(environment.Home, ".viceme-cli")
}

func configLoadFailure(message string, err error) *output.Error {
	result := output.Internal("config_load", message, err)
	var loadErr *config.LoadError
	if !errors.As(err, &loadErr) {
		return result
	}
	result.WithDetails(map[string]any{
		"path":  loadErr.Path,
		"stage": loadErr.Stage,
	})
	switch loadErr.Stage {
	case "read":
		result.WithHint("verify that the reported configuration path exists and is readable by this process")
	case "decode", "validate":
		result.WithHint("repair or recreate the reported configuration file; do not share credentials from it")
	case "permissions":
		result.WithHint("restrict the reported configuration file to the current user and retry")
	}
	return result
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func randomUUID() string {
	return uuidFromEntropy(rand.Reader, time.Now(), os.Getpid(), fallbackUUIDSequence.Add(1))
}

func uuidFromEntropy(reader io.Reader, now time.Time, processID int, sequence uint64) string {
	var value [16]byte
	if _, err := io.ReadFull(reader, value[:]); err != nil {
		fallback := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%d", now.UnixNano(), processID, sequence)))
		copy(value[:], fallback[:len(value)])
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}

func customCredentialScope(apiBaseURL string) (string, error) {
	origin, err := api.NormalizeAPIOrigin(apiBaseURL)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(origin))
	return fmt.Sprintf("custom:%x", digest[:]), nil
}

func credentialScopeForAPIBase(apiBaseURL string) (string, error) {
	return customCredentialScope(apiBaseURL)
}

func legacyCredentialRegionForAPIBase(apiBaseURL string) string {
	origin, err := api.NormalizeAPIOrigin(apiBaseURL)
	if err != nil {
		return ""
	}
	for _, region := range []config.Region{config.RegionCN, config.RegionGlobal} {
		officialOrigin, normalizeErr := api.NormalizeAPIOrigin(config.APIBaseURL(region))
		if normalizeErr == nil && origin == officialOrigin {
			return string(region)
		}
	}
	return ""
}

// errorsAs is a small indirection so the rest of the command tree does not
// accidentally special-case Cobra errors differently from typed CLI errors.
func errorsAs(err error, target any) bool {
	return errors.As(err, target)
}
