package command

import (
	"context"
	"errors"
	"time"

	"github.com/ViceMe-AI/cli/internal/output"
	updatepkg "github.com/ViceMe-AI/cli/internal/update"
	"github.com/spf13/cobra"
)

func newUpdateCommand(runtime *Runtime) *cobra.Command {
	var checkOnly bool
	var skipSkillInstall bool
	var target string
	command := &cobra.Command{
		Use:   "update",
		Short: "Update the npm launcher, verified Go binary, and bundled Agent Skill",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(command.Context(), 3*time.Minute)
			defer cancel()
			check, err := runtime.deps.Updater.Check(ctx)
			if err != nil {
				return output.Network("update_check_failed", "could not query the npm release", err)
			}
			if checkOnly {
				return runtime.success(check)
			}
			result, err := runtime.deps.Updater.Apply(ctx, check, updatepkg.ApplyOptions{
				RefreshSkills: !skipSkillInstall,
				SkillTarget:   target,
			})
			if errors.Is(err, updatepkg.ErrNPMInstallRequired) {
				return output.Policy("update_install_method", "this CLI was not started through the npm launcher").WithHint("run 'npx --yes --registry=https://registry.npmjs.org --@viceme-ai:registry=https://registry.npmjs.org --package=@viceme-ai/cli@latest -- viceme install', then use the installed 'viceme' launcher")
			}
			if err != nil {
				return output.Internal("update_partial", "CLI update did not complete for every target", err).WithDetails(result)
			}
			return runtime.success(result)
		},
	}
	command.Flags().BoolVar(&checkOnly, "check", false, "check the latest npm release without changing local state")
	command.Flags().BoolVar(&skipSkillInstall, "skip-skill-install", false, "update only the npm launcher and binary")
	command.Flags().StringVar(&target, "target", "auto", "Agent Skill target refreshed after update: auto, codex, or claude")
	return command
}
