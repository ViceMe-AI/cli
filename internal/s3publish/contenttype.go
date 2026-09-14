// Package s3publish publishes an assembled CLI release contract to the CN and
// Global S3-compatible origins used by official installers and hosted Skills.
package s3publish

import "strings"

const (
	cacheImmutable = "public,max-age=31536000,immutable"
	cacheStable    = "public,max-age=300"
	cacheLatest    = "public,max-age=60"
	cacheProbe     = "private,no-store"
	markdownType   = "text/markdown; charset=utf-8"
	plainType      = "text/plain"
	skillsBucket   = "skills"
	releasePrefix  = "cli/releases"
)

// HostedContentType maps a hosted object's path to the Content-Type used by
// the current release contract. Unknown extensions stay empty so the origin
// stores them as application/octet-stream rather than mislabeling binaries.
func HostedContentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".zip"):
		return "application/zip"
	case strings.HasSuffix(name, ".json"):
		return "application/json; charset=utf-8"
	case strings.HasSuffix(name, ".md"), strings.HasSuffix(name, ".markdown"):
		return markdownType
	case strings.HasSuffix(name, ".txt"):
		return "text/plain; charset=utf-8"
	case strings.HasSuffix(name, ".py"):
		return "text/x-python; charset=utf-8"
	case strings.HasSuffix(name, ".sh"):
		return "text/x-shellscript; charset=utf-8"
	case strings.HasSuffix(name, ".yaml"), strings.HasSuffix(name, ".yml"):
		return "text/yaml; charset=utf-8"
	case strings.HasSuffix(name, ".toml"):
		return "application/toml; charset=utf-8"
	case strings.HasSuffix(name, ".html"), strings.HasSuffix(name, ".htm"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(name, ".js"), strings.HasSuffix(name, ".mjs"):
		return "text/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".svg"):
		return "image/svg+xml"
	default:
		return ""
	}
}

func flatDistContentType(name string) string {
	if strings.HasSuffix(name, ".md") {
		return markdownType
	}
	return ""
}
