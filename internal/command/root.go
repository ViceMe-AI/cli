package command

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	cliembed "github.com/ViceMe-AI/cli"
	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	updatepkg "github.com/ViceMe-AI/cli/internal/update"
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
	// OpenBrowser opens an authorization URL in the user's default browser.
	// Tests override it so command execution never launches a real GUI process.
	OpenBrowser func(string) error
	APIBaseURL  string
	Region      config.Region
	// ManagedAppBuilder builds a managed Skill App project in place (install +
	// build). Defaults to `pnpm install --ignore-scripts && pnpm build`; tests
	// override it to avoid a real toolchain dependency.
	ManagedAppBuilder ManagedAppBuilder
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
	processLockRoot    string
}

const (
	apiBaseURLEnvironment = "VICEME_API_BASE_URL"
)

func Execute(args []string, dependencies Dependencies) int {
	root, runtime, err := NewRoot(dependencies)
	if err != nil {
		printer := &output.Printer{Out: writerOr(dependencies.Out, os.Stdout), ErrOut: writerOr(dependencies.ErrOut, os.Stderr)}
		return printer.Failure(err)
	}
	root.SetArgs(args)
	if err := root.ExecuteContext(context.Background()); err != nil {
		return runtime.failure(err)
	}
	return 0
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
	region := resolvedProfile.Region
	apiBaseURLOverride := dependencies.APIBaseURL
	apiBaseURLFromEnv := false
	if apiBaseURLOverride == "" {
		apiBaseURLOverride = os.Getenv(apiBaseURLEnvironment)
		apiBaseURLFromEnv = apiBaseURLOverride != ""
	}
	runtime := &Runtime{
		deps:               dependencies,
		region:             region,
		apiBaseURLOverride: apiBaseURLOverride,
		apiBaseURLFromEnv:  apiBaseURLFromEnv,
		config:             resolvedConfig,
		profile:            *resolvedProfile,
		configBase:         configBase,
		processLockRoot:    stableProcessLockRoot(dependencies.Environment),
		printer: &output.Printer{
			Out:    dependencies.Out,
			ErrOut: dependencies.ErrOut,
		},
	}
	if err := runtime.selectProfile(resolvedProfile.Name); err != nil {
		return nil, nil, err
	}
	root := &cobra.Command{
		Use:           "viceme",
		Short:         "Connect projects to ViceMe Creator capabilities",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if runtime.opts.version {
				return runtime.writeVersion()
			}
			return cmd.Help()
		},
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetIn(dependencies.In)
	root.SetOut(dependencies.Out)
	root.SetErr(dependencies.ErrOut)
	root.Flags().BoolVarP(&runtime.opts.version, "version", "v", false, "print version information")
	root.PersistentFlags().StringVar(&runtime.opts.profile, "profile", "", "use a specific profile for this command")
	root.PersistentPreRunE = func(command *cobra.Command, _ []string) error {
		runtime.prepareUpdateNotice(command)
		if err := runtime.validateProfileOverrideAuthority(command, runtime.opts.profile); err != nil {
			return err
		}
		return runtime.selectProfile(runtime.opts.profile)
	}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return output.Validation("invalid_flag", err.Error())
	})
	root.AddCommand(newVersionCommand(runtime))
	root.AddCommand(newInstallCommand(runtime))
	root.AddCommand(newUpdateCommand(runtime))
	root.AddCommand(newAuthCommand(runtime))
	root.AddCommand(newConfigCommand(runtime))
	root.AddCommand(newProfileCommand(runtime))
	root.AddCommand(newAppCommand(runtime))
	root.AddCommand(newCapabilityCommand(runtime))
	root.AddCommand(newCommerceCommand(runtime))
	root.AddCommand(newListingCommand(runtime))
	root.AddCommand(newSkillsCommand(runtime))
	root.AddCommand(newRuntimeCommand(runtime))
	root.AddCommand(newJobCommand(runtime))
	return root, runtime, nil
}

func (r *Runtime) prepareUpdateNotice(command *cobra.Command) {
	if command != nil && command.Name() == "update" {
		// The update result already reports its exact outcome. Do not attach a
		// stale pre-update reminder to that response.
		r.printer.Notice = nil
		return
	}
	notifier, ok := r.deps.Updater.(updatepkg.Notifier)
	if !ok {
		return
	}
	r.printer.Notice = func() map[string]any {
		notice := notifier.CachedNotice()
		if notice == nil {
			return nil
		}
		return map[string]any{
			"update": map[string]any{
				"current": notice.Current,
				"latest":  notice.Latest,
				"message": notice.Message(),
				"command": "viceme update",
			},
		}
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		notifier.RefreshNotice(ctx)
	}()
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
		updater := updatepkg.NewNPMService(
			buildinfo.Version,
			buildinfo.CompatibilityVersion(),
			os.Getenv("VICEME_INSTALL_METHOD"),
		)
		updater.ConfigDir = runtimeConfigBase(dependencies.Environment)
		updater.HTTPClient = dependencies.HTTPClient
		dependencies.Updater = updater
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
	if dependencies.OpenBrowser == nil {
		dependencies.OpenBrowser = openBrowser
	}
	return dependencies
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
		Store:       r.deps.Store,
		Region:      string(r.region),
		ProfileID:   r.profile.ID,
		ProfileName: r.profile.Name,
		Scope:       r.credentialScope,
		LockRoot:    r.processLockRoot,
		Now:         r.deps.Now,
	}
}

func (r *Runtime) client() *api.Client {
	client := api.NewClient(r.apiBaseURL, r.deps.HTTPClient, nil, "viceme/"+buildinfo.Version)
	client.Tokens = &refreshingTokenSource{
		manager: r.manager(),
		client:  client,
		now:     r.deps.Now,
		newID:   r.deps.NewID,
	}
	return client
}

type refreshingTokenSource struct {
	manager *auth.Manager
	client  *api.Client
	now     func() time.Time
	newID   func() string
}

func (source *refreshingTokenSource) Token(ctx context.Context) (string, error) {
	credential, err := source.manager.Load()
	if err != nil {
		return "", err
	}
	now := source.now()
	if credential.RefreshRequestID == "" && (credential.ExpiresAt.IsZero() || now.Before(credential.ExpiresAt.Add(-30*time.Second))) {
		return credential.AccessToken, nil
	}
	var token string
	err = source.manager.WithCredentialLock(ctx, func() error {
		var refreshErr error
		token, refreshErr = source.tokenWhileLocked(ctx)
		return refreshErr
	})
	return token, err
}

func (source *refreshingTokenSource) tokenWhileLocked(ctx context.Context) (string, error) {
	credential, err := source.manager.Load()
	if err != nil {
		return "", err
	}
	now := source.now()
	if credential.RefreshRequestID == "" && (credential.ExpiresAt.IsZero() || now.Before(credential.ExpiresAt.Add(-30*time.Second))) {
		return credential.AccessToken, nil
	}
	if credential.RefreshToken == "" || (!credential.RefreshExpiresAt.IsZero() && !now.Before(credential.RefreshExpiresAt)) {
		return "", output.Authentication("token_expired", "ViceMe login has expired; run 'viceme auth login'")
	}
	requestID := credential.RefreshRequestID
	if requestID == "" {
		newID := source.newID
		if newID == nil {
			newID = randomUUID
		}
		requestID = newID()
		credential.RefreshRequestID = requestID
		if err := source.manager.Save(credential); err != nil {
			return "", output.Authentication("credential_persistence_failed", "the refresh recovery state could not be saved; no token request was sent").
				WithHint("fix the local credential store and retry the command").
				WithCause(err)
		}
	}
	refreshed, err := source.client.RefreshDeviceToken(ctx, credential.RefreshToken, requestID)
	if err != nil {
		return "", err
	}
	next := auth.Credential{
		AccessToken:      refreshed.AccessToken,
		RefreshToken:     refreshed.RefreshToken,
		TokenType:        refreshed.TokenType,
		ExpiresAt:        refreshed.ExpiresAt,
		RefreshExpiresAt: refreshed.RefreshExpiresAt,
		RefreshRequestID: "",
		UserID:           refreshed.UserID,
		Scope:            refreshed.Scope,
	}
	if err := source.manager.Save(next); err != nil {
		return "", output.Authentication("credential_persistence_failed", "refreshed credentials could not be saved").
			WithHint("fix the local credential store and retry the command; the persisted refresh request can recover the same server result").
			WithCause(err)
	}
	return refreshed.AccessToken, nil
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
	skillcontent.Digests
}

func (r *Runtime) writeVersion() error {
	digests, err := r.deps.Skills.Digests("viceme")
	if err != nil {
		return err
	}
	return r.business(versionResult{
		Info:    buildinfo.Current(),
		Digests: digests,
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
	r.profile.Region = region
	return r.applyProfile(r.profile)
}

func (r *Runtime) selectProfile(name string) error {
	profile, err := r.config.Resolve(name)
	if err != nil {
		return output.Validation("profile_not_found", err.Error())
	}
	return r.applyProfile(*profile)
}

func (r *Runtime) validateProfileOverrideAuthority(command *cobra.Command, name string) error {
	if name == "" || !r.apiBaseURLFromEnv {
		return nil
	}
	if command != nil && command.Name() == "login" && command.Parent() != nil && command.Parent().Name() == "auth" {
		deviceCode, err := command.Flags().GetString("device-code")
		if err == nil && deviceCode != "" {
			return nil
		}
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
		apiBaseURL = profile.APIBaseURL
	}
	if apiBaseURL == "" {
		apiBaseURL = config.APIBaseURL(profile.Region)
	}
	normalizedAPIBaseURL, err := api.NormalizeAPIBaseURL(apiBaseURL)
	if err != nil {
		return output.Validation("api_base_url", "ViceMe API base URL must use HTTPS; HTTP is allowed only for localhost or loopback development")
	}
	scope, err := credentialScopeForAPIBase(normalizedAPIBaseURL, profile.Region)
	if err != nil {
		return output.Validation("api_base_url", "ViceMe API base URL must use HTTPS; HTTP is allowed only for localhost or loopback development")
	}
	r.profile = profile
	r.region = profile.Region
	r.apiBaseURL = normalizedAPIBaseURL
	r.credentialScope = scope
	return nil
}

func (r *Runtime) credentialScopeForProfile(profile config.Profile) (string, error) {
	apiBaseURL := r.apiBaseURLOverride
	if apiBaseURL == "" {
		apiBaseURL = profile.APIBaseURL
	}
	if apiBaseURL == "" {
		apiBaseURL = config.APIBaseURL(profile.Region)
	}
	return credentialScopeForAPIBase(apiBaseURL, profile.Region)
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
		if profile.APIBaseURL != "" {
			scope, err := credentialScopeForAPIBase(profile.APIBaseURL, profile.Region)
			if err != nil {
				return nil, err
			}
			add(&auth.Manager{ProfileID: profile.ID, ProfileName: profile.Name, Region: string(profile.Region), Scope: scope})
		}
	}
	add(r.manager())
	return keys, nil
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

func stableProcessLockRoot(environment skillcontent.Environment) string {
	return filepath.Join(environment.Home, ".viceme-cli-locks")
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
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		digest := sha256.Sum256([]byte(fmt.Sprintf("viceme-request-%d", time.Now().UnixNano())))
		copy(value[:], digest[:16])
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}

func customCredentialScope(apiBaseURL string) (string, error) {
	normalized, err := api.NormalizeAPIBaseURL(apiBaseURL)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("custom:%x", digest[:]), nil
}

func credentialScopeForAPIBase(apiBaseURL string, region config.Region) (string, error) {
	normalized, err := api.NormalizeAPIBaseURL(apiBaseURL)
	if err != nil {
		return "", err
	}
	canonical, err := api.NormalizeAPIBaseURL(config.APIBaseURL(region))
	if err != nil {
		return "", err
	}
	if normalized == canonical {
		return "", nil
	}
	return customCredentialScope(apiBaseURL)
}

// errorsAs is a small indirection so the rest of the command tree does not
// accidentally special-case Cobra errors differently from typed CLI errors.
func errorsAs(err error, target any) bool {
	return errors.As(err, target)
}
