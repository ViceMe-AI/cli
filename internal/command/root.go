package command

import (
	"context"
	"crypto/rand"
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
	APIBaseURL  string
	Region      config.Region
}

type options struct {
	version bool
	profile string
}

type Runtime struct {
	deps                 Dependencies
	opts                 options
	printer              *output.Printer
	meta                 output.Meta
	region               config.Region
	apiBaseURL           string
	apiBaseURLOverridden bool
	config               config.Config
	profile              config.Profile
	configBase           string
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
			return nil, nil, output.Internal("config_load", "could not load Viceme CLI configuration", err)
		}
	} else {
		resolvedRegion, err := config.ParseRegion(string(dependencies.Region))
		if err != nil {
			return nil, nil, output.Internal("config_region", "invalid injected Viceme region", err)
		}
		resolvedConfig = config.Default(resolvedRegion)
	}
	resolvedProfile, err := resolvedConfig.Resolve("")
	if err != nil {
		return nil, nil, output.Internal("config_profile", "could not resolve the active Viceme CLI profile", err)
	}
	region := resolvedProfile.Region
	apiBaseURL := dependencies.APIBaseURL
	apiBaseURLOverridden := apiBaseURL != ""
	if apiBaseURL == "" {
		apiBaseURL = os.Getenv("VICEME_API_BASE_URL")
		apiBaseURLOverridden = apiBaseURL != ""
	}
	if apiBaseURL == "" {
		apiBaseURL = config.APIBaseURL(region)
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
		deps:                 dependencies,
		meta:                 meta,
		region:               region,
		apiBaseURL:           apiBaseURL,
		apiBaseURLOverridden: apiBaseURLOverridden,
		config:               resolvedConfig,
		profile:              *resolvedProfile,
		configBase:           configBase,
		printer: &output.Printer{
			Out:    dependencies.Out,
			ErrOut: dependencies.ErrOut,
			Meta:   meta,
		},
	}
	root := &cobra.Command{
		Use:           "viceme",
		Short:         "Publish external Skills as stable Viceme Agents",
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
	if dependencies.Store == nil {
		dependencies.Store = securestore.NewKeyring("viceme-cli")
	}
	if dependencies.Skills == nil {
		dependencies.Skills = skillcontent.New(cliembed.EmbeddedSkills())
	}
	if dependencies.Environment.Home == "" {
		dependencies.Environment = skillcontent.DefaultEnvironment()
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
	}
}

func (r *Runtime) client() *api.Client {
	return api.NewClient(r.apiBaseURL, r.deps.HTTPClient, r.manager(), "viceme/"+buildinfo.Version)
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

func (r *Runtime) setRegion(region config.Region) {
	r.region = region
	r.profile.Region = region
	if !r.apiBaseURLOverridden {
		r.apiBaseURL = config.APIBaseURL(region)
	}
}

func (r *Runtime) selectProfile(name string) error {
	profile, err := r.config.Resolve(name)
	if err != nil {
		return output.Validation("profile_not_found", err.Error())
	}
	r.profile = *profile
	r.setRegion(profile.Region)
	return nil
}

func (r *Runtime) reloadConfig(profileName string) error {
	resolved, err := config.LoadOrDefault(r.configBase)
	if err != nil {
		return output.Internal("config_load", "could not reload Viceme CLI configuration", err)
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

// errorsAs is a small indirection so the rest of the command tree does not
// accidentally special-case Cobra errors differently from typed CLI errors.
func errorsAs(err error, target any) bool {
	return errors.As(err, target)
}
