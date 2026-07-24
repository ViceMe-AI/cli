package command

import (
	"sort"
	"strings"

	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type CommandManifest struct {
	SchemaVersion     int               `json:"schema_version"`
	CLIVersion        string            `json:"cli_version"`
	MinimumCLIVersion string            `json:"minimum_cli_version"`
	GlobalFlags       []ManifestFlag    `json:"global_flags"`
	Commands          []ManifestCommand `json:"commands"`
}

type ManifestCommand struct {
	Path                 string         `json:"path"`
	Use                  string         `json:"use"`
	Short                string         `json:"short"`
	Runnable             bool           `json:"runnable"`
	SideEffect           string         `json:"side_effect"`
	RequiresConfirmation bool           `json:"requires_confirmation"`
	Flags                []ManifestFlag `json:"flags,omitempty"`
}

type ManifestFlag struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand,omitempty"`
	Type      string `json:"type"`
	Default   string `json:"default,omitempty"`
	Usage     string `json:"usage"`
}

func BuildCommandManifest(root *cobra.Command) CommandManifest {
	manifest := CommandManifest{
		SchemaVersion:     1,
		CLIVersion:        buildinfo.ReleaseVersion,
		MinimumCLIVersion: buildinfo.MinimumCLIVersion,
		GlobalFlags:       manifestFlags(root.PersistentFlags()),
	}
	var visit func(*cobra.Command)
	visit = func(parent *cobra.Command) {
		for _, command := range parent.Commands() {
			if command.Hidden || command.Name() == "help" || command.Name() == "completion" {
				continue
			}
			path := strings.TrimPrefix(command.CommandPath(), root.Name()+" ")
			runnable := command.Run != nil || command.RunE != nil
			sideEffect, confirmation := commandPolicy(path, runnable)
			manifest.Commands = append(manifest.Commands, ManifestCommand{
				Path:                 path,
				Use:                  command.Use,
				Short:                command.Short,
				Runnable:             runnable,
				SideEffect:           sideEffect,
				RequiresConfirmation: confirmation,
				Flags:                manifestFlags(command.LocalNonPersistentFlags()),
			})
			visit(command)
		}
	}
	visit(root)
	sort.Slice(manifest.Commands, func(left, right int) bool {
		return manifest.Commands[left].Path < manifest.Commands[right].Path
	})
	return manifest
}

func manifestFlags(flags *pflag.FlagSet) []ManifestFlag {
	var result []ManifestFlag
	flags.VisitAll(func(flag *pflag.Flag) {
		result = append(result, ManifestFlag{
			Name:      flag.Name,
			Shorthand: flag.Shorthand,
			Type:      flag.Value.Type(),
			Default:   flag.DefValue,
			Usage:     flag.Usage,
		})
	})
	sort.Slice(result, func(left, right int) bool { return result[left].Name < result[right].Name })
	return result
}

func commandPolicy(path string, runnable bool) (string, bool) {
	switch path {
	case "skill publish", "job cancel", "job retry":
		return "public", true
	case "job resume", "job metadata", "job edit", "job run", "job accept":
		return "public", false
	case "auth logout":
		return "remote_and_local", false
	case "auth login", "config keychain-downgrade", "install", "profile add", "profile configure", "profile use", "profile remove", "profile rename", "skills install", "update":
		return "local", false
	case "version", "auth status", "profile list", "skill inspect", "skill target get", "skill target list", "job bind", "job get", "job wait", "job preview", "job edit-get", "job run-get", "skills list", "skills read", "skills doctor":
		return "none", false
	default:
		if runnable {
			// An empty classification is deliberate: the quality gate must fail
			// whenever a new executable command has not been reviewed here.
			return "", false
		}
		return "none", false
	}
}
