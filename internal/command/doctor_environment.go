package command

import (
	"runtime"

	"github.com/ViceMe-AI/cli/internal/agentenv"
)

// doctorEnvironmentSection reports the detected agent host together with the
// verified login orchestration facts that official Skills dispatch on. getenv
// is os.Getenv in production and injected in tests.
func doctorEnvironmentSection(getenv func(string) string, configBase string) map[string]any {
	capabilities := agentenv.CapabilityProfile(agentenv.Detect(getenv))
	return map[string]any{
		"platform":              string(capabilities.Platform),
		"os":                    runtime.GOOS,
		"loginLaunchStrategy":   string(capabilities.LoginLaunch),
		"taskOutputStderr":      capabilities.TaskOutputStderr,
		"loginPresentationPath": deviceLoginPresentationFilePath(configBase),
	}
}
