package command

import (
	"fmt"
	"strings"

	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/spf13/cobra"
)

func newSkillsCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "skills", Short: "Read, install, and diagnose bundled Agent Skill content"}
	command.AddCommand(newSkillsListCommand(runtime))
	command.AddCommand(newSkillsReadCommand(runtime))
	command.AddCommand(newSkillsInstallCommand(runtime))
	command.AddCommand(newSkillsDoctorCommand(runtime))
	return command
}

func newSkillsListCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List Skills embedded in this CLI build",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			skills, err := runtime.deps.Skills.List()
			if err != nil {
				return err
			}
			return runtime.success(map[string]any{"skills": skills, "count": len(skills)})
		},
	}
}

func newSkillsReadCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{
		Use:   "read <name>[/<path>] [path]",
		Short: "Read embedded Skill content",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) < 1 || len(args) > 2 {
				return output.Validation("skill_read_args", "read requires <name>[/<path>] [path]")
			}
			return nil
		},
		RunE: func(_ *cobra.Command, args []string) error {
			name, relative := splitSkillReadArgs(args)
			data, resolved, err := runtime.deps.Skills.Read(name, relative)
			if err != nil {
				return err
			}
			if runtime.opts.JSON {
				return runtime.success(map[string]any{"skill": name, "path": resolved, "content": string(data)})
			}
			if _, err := runtime.deps.Out.Write(data); err != nil {
				return output.Internal("stdout_write", "failed to write Skill content", err)
			}
			if len(data) == 0 || data[len(data)-1] != '\n' {
				_, _ = fmt.Fprintln(runtime.deps.Out)
			}
			return nil
		},
	}
	return command
}

func splitSkillReadArgs(args []string) (string, string) {
	if len(args) == 2 {
		return args[0], args[1]
	}
	parts := strings.SplitN(args[0], "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return args[0], ""
}

func newSkillsInstallCommand(runtime *Runtime) *cobra.Command {
	var target string
	command := &cobra.Command{
		Use:   "install",
		Short: "Atomically install the bundled Viceme Skill",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := runtime.deps.Skills.Validate("viceme"); err != nil {
				return err
			}
			report := runtime.deps.Skills.Install("viceme", target, runtime.deps.Environment)
			if !report.AllSucceeded {
				return output.Internal("skill_install_partial", "one or more Skill targets could not be installed", nil).WithDetails(report)
			}
			return runtime.success(report)
		},
	}
	command.Flags().StringVar(&target, "target", "auto", "Skill target: auto, codex, or claude")
	return command
}

func newSkillsDoctorCommand(runtime *Runtime) *cobra.Command {
	var target string
	command := &cobra.Command{
		Use:   "doctor",
		Short: "Check installed Skill content against this CLI release",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			report := runtime.deps.Skills.Doctor("viceme", target, runtime.deps.Environment)
			return runtime.success(report)
		},
	}
	command.Flags().StringVar(&target, "target", "auto", "Skill target: auto, codex, or claude")
	return command
}
