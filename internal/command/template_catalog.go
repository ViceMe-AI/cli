package command

import (
	"context"
	"errors"
	"os"

	"github.com/ViceMe-AI/cli/internal/buildinfo"
	"github.com/ViceMe-AI/cli/internal/commerceartifact"
	"github.com/ViceMe-AI/cli/internal/config"
	"github.com/ViceMe-AI/cli/internal/output"
	"github.com/ViceMe-AI/cli/internal/templatecatalog"
	"github.com/spf13/cobra"
)

const templateCatalogOriginEnvironment = "VICEME_TEMPLATE_CATALOG_ORIGIN"

func newTemplateCatalogCommand(runtime *Runtime) *cobra.Command {
	command := &cobra.Command{Use: "template", Short: "Browse and fetch verified creator page templates", Args: cobra.NoArgs}
	command.AddCommand(newTemplateListCommand(runtime))
	command.AddCommand(newTemplateFetchCommand(runtime))
	return command
}

func newTemplateListCommand(runtime *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List verified creator page templates",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			client, err := runtime.templateCatalogClient()
			if err != nil {
				return err
			}
			manifest, err := client.Load(context.Background())
			if err != nil {
				return templateCatalogCommandError(err)
			}
			catalogURL, err := client.CatalogURL()
			if err != nil {
				return templateCatalogCommandError(err)
			}
			return runtime.success(map[string]any{"catalog_url": catalogURL, "templates": manifest.Templates})
		},
	}
}

func newTemplateFetchCommand(runtime *Runtime) *cobra.Command {
	var version, destination string
	command := &cobra.Command{
		Use:   "fetch <template-id>",
		Short: "Download one verified creator page template",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if version == "" || destination == "" {
				return output.Validation("TEMPLATE_CATALOG_FETCH_ARGS_INVALID", "--version and --destination are required")
			}
			client, err := runtime.templateCatalogClient()
			if err != nil {
				return err
			}
			if err := client.Fetch(context.Background(), args[0], version, destination); err != nil {
				return templateCatalogCommandError(err)
			}
			return runtime.success(map[string]any{
				"template_id": args[0], "version": version, "source_path": destination + string(os.PathSeparator) + args[0],
			})
		},
	}
	command.Flags().StringVar(&version, "version", "", "verified template version")
	command.Flags().StringVar(&destination, "destination", "", "directory for the extracted template")
	return command
}

func templateCatalogCommandError(err error) error {
	switch {
	case errors.Is(err, templatecatalog.ErrCatalogUnavailable):
		return output.Network("TEMPLATE_CATALOG_UNAVAILABLE", "template catalog is temporarily unavailable", err)
	case errors.Is(err, templatecatalog.ErrUntrustedManifest):
		return output.Policy("TEMPLATE_CATALOG_SIGNATURE_INVALID", "template catalog signature verification failed")
	case errors.Is(err, templatecatalog.ErrTemplateNotFound):
		return output.Validation("TEMPLATE_CATALOG_TEMPLATE_NOT_FOUND", "the requested template version is not available")
	case errors.Is(err, templatecatalog.ErrSourceDigestMismatch):
		return output.Policy("TEMPLATE_CATALOG_SOURCE_DIGEST_MISMATCH", "template source checksum verification failed")
	case errors.Is(err, templatecatalog.ErrArchiveInvalid):
		return output.Policy("TEMPLATE_CATALOG_ARCHIVE_INVALID", "template source archive failed safety validation")
	default:
		return output.Internal("TEMPLATE_CATALOG_FAILED", "template catalog operation failed", err)
	}
}

func (runtime *Runtime) templateCatalogClient() (templatecatalog.Client, error) {
	trustedKeys, err := commerceartifact.ParseTrustRing(buildinfo.TemplateCatalogTrustKeys)
	if err != nil || len(trustedKeys) == 0 {
		return templatecatalog.Client{}, output.Policy("TEMPLATE_CATALOG_TRUST_UNAVAILABLE", "this CLI build cannot verify the template catalog")
	}
	origin := catalogOrigin(runtime.region)
	allowInsecure := false
	if buildinfo.Version == "dev" {
		if override := os.Getenv(templateCatalogOriginEnvironment); override != "" {
			origin = override
			allowInsecure = true
		}
	}
	return templatecatalog.Client{Origin: origin, HTTPClient: runtime.deps.HTTPClient, TrustedKeys: trustedKeys, AllowInsecure: allowInsecure}, nil
}

func catalogOrigin(region config.Region) string {
	if region == config.RegionGlobal {
		return "https://s3.viceme.ai/templates"
	}
	return "https://s3.viceme.cn/templates"
}
