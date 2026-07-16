package command

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	cliembed "github.com/ViceMe-AI/cli"
	"github.com/ViceMe-AI/cli/internal/api"
	"github.com/ViceMe-AI/cli/internal/auth"
	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/securestore"
	"github.com/ViceMe-AI/cli/internal/skillcontent"
	"github.com/spf13/cobra"
)

const defaultAPIBaseURL = "https://api.viceme.ai"

type Dependencies struct {
	In          io.Reader
	Out         io.Writer
	ErrOut      io.Writer
	HTTPClient  *http.Client
	Store       securestore.Store
	Skills      *skillcontent.Bundle
	Environment skillcontent.Environment
	Now         func() time.Time
	Sleep       func(context.Context, time.Duration) error
	NewID       func() string
	APIBaseURL  string
}

type options struct {
	JSON       bool
	Profile    string
	APIBaseURL string
	version    bool
}

type Runtime struct {
	deps    Dependencies
	opts    options
	printer *output.Printer
	meta    output.Meta
}

func Execute(args []string, dependencies Dependencies) int {
	root, runtime, err := NewRoot(dependencies)
	if err != nil {
		printer := &output.Printer{Out: writerOr(dependencies.Out, os.Stdout), ErrOut: writerOr(dependencies.ErrOut, os.Stderr), JSON: hasJSONFlag(args)}
		return printer.Failure(err)
	}
	root.SetArgs(args)
	if err := root.ExecuteContext(context.Background()); err != nil {
		return runtime.failure(err)
	}
	return 0
}

func NewRoot(dependencies Dependencies) (*cobra.Command, *Runtime, error) {
	dependencies = defaults(dependencies)
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
		deps: dependencies,
		meta: meta,
		printer: &output.Printer{
			Out:    dependencies.Out,
			ErrOut: dependencies.ErrOut,
			Meta:   meta,
		},
	}
	runtime.opts.APIBaseURL = dependencies.APIBaseURL
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
	root.PersistentFlags().BoolVar(&runtime.opts.JSON, "json", false, "emit stable JSON output")
	root.PersistentFlags().StringVar(&runtime.opts.Profile, "profile", "default", "credential profile")
	root.PersistentFlags().StringVar(&runtime.opts.APIBaseURL, "api-base-url", dependencies.APIBaseURL, "Viceme API base URL")
	root.Flags().BoolVarP(&runtime.opts.version, "version", "v", false, "print version information")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return output.Validation("invalid_flag", err.Error())
	})
	root.AddCommand(newVersionCommand(runtime))
	root.AddCommand(newAuthCommand(runtime))
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
	if dependencies.Now == nil {
		dependencies.Now = time.Now
	}
	if dependencies.Sleep == nil {
		dependencies.Sleep = sleepContext
	}
	if dependencies.NewID == nil {
		dependencies.NewID = randomUUID
	}
	if dependencies.APIBaseURL == "" {
		dependencies.APIBaseURL = os.Getenv("VICEME_API_BASE_URL")
		if dependencies.APIBaseURL == "" {
			dependencies.APIBaseURL = defaultAPIBaseURL
		}
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
	return &auth.Manager{Store: r.deps.Store, Profile: r.opts.Profile}
}

func (r *Runtime) client() *api.Client {
	return api.NewClient(r.opts.APIBaseURL, r.deps.HTTPClient, r.manager(), "viceme/"+buildinfo.Version)
}

func (r *Runtime) success(data any) error {
	r.printer.JSON = r.opts.JSON
	return r.printer.Success(data)
}

func (r *Runtime) successWithMeta(data any, meta output.Meta) error {
	r.printer.JSON = r.opts.JSON
	return r.printer.SuccessWithMeta(data, meta)
}

func (r *Runtime) failure(err error) int {
	r.printer.JSON = r.opts.JSON
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

func hasJSONFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--json" || arg == "--json=true" {
			return true
		}
	}
	return false
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
