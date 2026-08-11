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
	APIBaseURL  string
	Region      config.Region
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
}

const (
	apiBaseURLEnvironment         = "VICEME_API_BASE_URL"
	processAccessTokenEnvironment = "VICEME_ACCESS_TOKEN"
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
	root, runtime, err := NewRoot(dependencies)
	if err != nil {
		printer := &output.Printer{Out: writerOr(dependencies.Out, os.Stdout), ErrOut: writerOr(dependencies.ErrOut, os.Stderr), CLIVersion: buildinfo.Version}
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
			Out:        dependencies.Out,
			ErrOut:     dependencies.ErrOut,
			CLIVersion: buildinfo.Version,
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
		if err := runtime.validateProfileOverrideAuthority(runtime.opts.profile); err != nil {
			return err
		}
		return runtime.selectProfile(runtime.opts.profile)
	}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return output.Validation("invalid_flag", err.Error())
	})
	root.AddCommand(newVersionCommand(runtime))
	root.AddCommand(newInstallCommand(runtime))
	root.AddCommand(newDoctorCommand(runtime))
	root.AddCommand(newUpdateCommand(runtime))
	root.AddCommand(newAuthCommand(runtime))
	root.AddCommand(newProfileCommand(runtime))
	root.AddCommand(newSkillCommand(runtime))
	root.AddCommand(newPublicationCommand(runtime))
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
	shared, err := r.deps.Skills.Digests("viceme-shared")
	if err != nil {
		return err
	}
	publish, err := r.deps.Skills.Digests("viceme-publish")
	if err != nil {
		return err
	}
	return r.business(versionResult{
		Info: buildinfo.Current(),
		Skills: map[string]skillcontent.Digests{
			"viceme-shared":  shared,
			"viceme-publish": publish,
		},
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
		apiBaseURL = config.APIBaseURL(profile.Region)
	}
	if err := validatePublicationProcessCredentialTarget(r.processCredential, apiBaseURL); err != nil {
		return err
	}
	scope, err := credentialScopeForAPIBase(apiBaseURL, profile.Region)
	if err != nil {
		return output.Validation("api_base_url", "ViceMe API base URL must use HTTPS; HTTP is allowed only for localhost or loopback development")
	}
	r.profile = profile
	r.region = profile.Region
	r.apiBaseURL = apiBaseURL
	r.credentialScope = scope
	if regionAware, ok := r.deps.Updater.(updatepkg.RegionAware); ok {
		regionAware.SetRegion(string(profile.Region))
	}
	return nil
}

func (r *Runtime) credentialScopeForProfile(profile config.Profile) (string, error) {
	apiBaseURL := config.APIBaseURL(profile.Region)
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

func credentialScopeForAPIBase(apiBaseURL string, region config.Region) (string, error) {
	origin, err := api.NormalizeAPIOrigin(apiBaseURL)
	if err != nil {
		return "", err
	}
	canonicalOrigin, err := api.NormalizeAPIOrigin(config.APIBaseURL(region))
	if err != nil {
		return "", err
	}
	if origin == canonicalOrigin {
		return "", nil
	}
	return customCredentialScope(apiBaseURL)
}

// errorsAs is a small indirection so the rest of the command tree does not
// accidentally special-case Cobra errors differently from typed CLI errors.
func errorsAs(err error, target any) bool {
	return errors.As(err, target)
}
