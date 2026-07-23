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
	meta               output.Meta
	region             config.Region
	apiBaseURL         string
	apiBaseURLOverride string
	credentialScope    string
	config             config.Config
	profile            config.Profile
	configBase         string
	processCredential  *publicationCredential
}

const (
	processAccessTokenEnvironment     = "VICEME_ACCESS_TOKEN"
	localProcessCredentialEnvironment = "VICEME_CLI_ALLOW_LOCAL_PROCESS_CREDENTIAL"
)

type publicationCredentialAudience string

const (
	publicationCredentialAudienceCNProd     publicationCredentialAudience = "cn-prod"
	publicationCredentialAudienceGlobalProd publicationCredentialAudience = "global-prod"
	publicationCredentialAudienceLocalDev   publicationCredentialAudience = "local-dev"
)

type publicationCredential struct {
	raw      string
	audience publicationCredentialAudience
}

type processTokenSource string

func (source processTokenSource) Token(context.Context) (string, error) {
	return string(source), nil
}

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
			return nil, nil, output.Internal("config_load", "could not load ViceMe CLI configuration", err)
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
	if apiBaseURLOverride == "" {
		apiBaseURLOverride = os.Getenv("VICEME_API_BASE_URL")
	}
	processCredential, err := parsePublicationCredential(os.Getenv(processAccessTokenEnvironment))
	if err != nil {
		return nil, nil, output.Authentication("process_credential_invalid", err.Error())
	}
	digests, err := dependencies.Skills.Digests("viceme")
	if err != nil {
		return nil, nil, err
	}
	meta := output.Meta{
		CLIVersion:            buildinfo.Version,
		SkillVersion:          buildinfo.SkillVersion,
		FullSkillBundleDigest: digests.Full,
		EmbeddedContentDigest: digests.Embedded,
	}
	runtime := &Runtime{
		deps:               dependencies,
		meta:               meta,
		region:             region,
		apiBaseURLOverride: apiBaseURLOverride,
		config:             resolvedConfig,
		profile:            *resolvedProfile,
		configBase:         configBase,
		processCredential:  processCredential,
		printer: &output.Printer{
			Out:    dependencies.Out,
			ErrOut: dependencies.ErrOut,
			Meta:   meta,
		},
	}
	if err := runtime.selectProfile(resolvedProfile.Name); err != nil {
		return nil, nil, err
	}
	root := &cobra.Command{
		Use:           "viceme",
		Short:         "Publish external Skills as stable ViceMe Agents",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if runtime.opts.version {
				return runtime.success(buildinfo.Current())
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
	root.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
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
	root.AddCommand(newSkillCommand(runtime))
	root.AddCommand(newJobCommand(runtime))
	root.AddCommand(newSkillsCommand(runtime))
	return root, runtime, nil
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
	return dependencies
}

func newVersionCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print CLI and bundled Skill versions",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runtime.success(buildinfo.Current())
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

func (r *Runtime) successWithMeta(data any, meta output.Meta) error {
	return r.printer.SuccessWithMeta(data, meta)
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

func (r *Runtime) applyProfile(profile config.Profile) error {
	apiBaseURL := r.apiBaseURLOverride
	if apiBaseURL == "" {
		apiBaseURL = profile.APIBaseURL
	}
	if apiBaseURL == "" {
		apiBaseURL = config.APIBaseURL(profile.Region)
	}
	if profile.AccessToken != "" {
		credential, err := parsePublicationCredential(profile.AccessToken)
		if err != nil {
			return output.Authentication("profile_credential_invalid", "the selected profile access token is not a supported audience-bound publication credential")
		}
		if err := validatePublicationCredentialTarget(credential, profile.APIBaseURL, true); err != nil {
			return output.Authentication("profile_credential_origin_mismatch", err.Error())
		}
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

func (r *Runtime) overrideCredential() (token, source string, persistent bool) {
	if r.processCredential != nil {
		return r.processCredential.raw, "process", false
	}
	if r.profile.AccessToken != "" && sameAPIOrigin(r.profile.APIBaseURL, r.apiBaseURL) {
		return r.profile.AccessToken, "local_profile", true
	}
	return "", "", false
}

func sameAPIOrigin(left, right string) bool {
	leftOrigin, leftErr := api.NormalizeAPIOrigin(left)
	rightOrigin, rightErr := api.NormalizeAPIOrigin(right)
	return leftErr == nil && rightErr == nil && leftOrigin == rightOrigin
}

func parsePublicationCredential(raw string) (*publicationCredential, error) {
	if raw == "" {
		return nil, nil
	}
	if strings.TrimSpace(raw) != raw || strings.ContainsAny(raw, "\r\n\x00") {
		return nil, errors.New("the process publication credential is invalid")
	}
	parts := strings.SplitN(raw, ".", 3)
	if len(parts) != 3 || parts[0] != "vpa1" || len(parts[2]) != 43 {
		return nil, errors.New("the process publication credential is not a supported audience-bound credential")
	}
	audience := publicationCredentialAudience(parts[1])
	switch audience {
	case publicationCredentialAudienceCNProd,
		publicationCredentialAudienceGlobalProd,
		publicationCredentialAudienceLocalDev:
	default:
		return nil, errors.New("the process publication credential audience is unsupported")
	}
	for _, character := range parts[2] {
		if !((character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-' || character == '_') {
			return nil, errors.New("the process publication credential is invalid")
		}
	}
	return &publicationCredential{raw: raw, audience: audience}, nil
}

func validatePublicationProcessCredentialTarget(credential *publicationCredential, apiBaseURL string) error {
	if credential == nil {
		return nil
	}
	if err := validatePublicationCredentialTarget(credential, apiBaseURL, false); err != nil {
		message := strings.Replace(err.Error(), "the publication credential", "the process publication credential", 1)
		return output.Authentication("process_credential_origin_mismatch", message)
	}
	return nil
}

func validatePublicationCredentialTarget(credential *publicationCredential, apiBaseURL string, allowLocalProfile bool) error {
	origin, err := api.NormalizeAPIOrigin(apiBaseURL)
	if err != nil {
		return errors.New("the publication credential target is invalid")
	}
	var expected string
	switch credential.audience {
	case publicationCredentialAudienceCNProd:
		expected, err = api.NormalizeAPIOrigin(config.APIBaseURL(config.RegionCN))
	case publicationCredentialAudienceGlobalProd:
		expected, err = api.NormalizeAPIOrigin(config.APIBaseURL(config.RegionGlobal))
	case publicationCredentialAudienceLocalDev:
		if !isLoopbackOrigin(origin) {
			return errors.New("local-dev publication credentials require a loopback target")
		}
		if !allowLocalProfile && os.Getenv(localProcessCredentialEnvironment) != "1" {
			return errors.New("local process credentials require an explicit loopback debug target")
		}
		return nil
	default:
		return errors.New("the publication credential audience is unsupported")
	}
	if err != nil || origin != expected {
		return errors.New("the publication credential audience does not match the selected ViceMe API origin")
	}
	return nil
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
		return output.Internal("config_load", "could not reload ViceMe CLI configuration", err)
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
		return fmt.Sprintf("request-%d", time.Now().UnixNano())
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
