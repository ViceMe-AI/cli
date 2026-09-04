package update

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ViceMe-AI/cli/internal/privatefile"
)

// IsPermissionDenied deliberately examines typed errors and known provider
// codes only. Raw npm output can contain credentials and is never returned.
func IsPermissionDenied(err error) bool {
	kind := ErrorKindOf(err)
	return kind == ErrorPermission || kind == ErrorNPMPermission || errors.Is(err, ErrRenameDenied) || privatefile.IsPermissionDenial(err)
}

// probeNPMActivation runs through Node as npm does, preserving the host's
// broker and permission environment. A Go-only probe cannot detect a Node
// filesystem broker refusal. Call while holding the activation lock, before
// writing a journal or attempting any package replacement.
func (service *NPMService) probeNPMActivation(ctx context.Context) error {
	if err := ProbeRenameCapability(service.ConfigDir); err != nil {
		return err
	}
	paths := []string{service.ConfigDir, filepath.Join(service.ConfigDir, npmCacheDirectory)}
	for _, query := range []string{"root", "prefix"} {
		output, err := service.runNPM(ctx, query, "--global", "--loglevel=silent", "--no-update-notifier")
		if err != nil {
			return err
		}
		directory := strings.TrimSpace(string(output))
		if !filepath.IsAbs(directory) || strings.ContainsAny(directory, "\r\n\x00") {
			return &OperationError{Kind: ErrorNPMCommand, Cause: errors.New("npm returned an invalid installation directory during permission checking")}
		}
		if query == "root" {
			paths = append(paths, filepath.Join(directory, "@viceme-ai"))
		} else {
			// Unix launchers live in prefix/bin; Windows launchers in prefix.
			if runtime.GOOS != "windows" {
				directory = filepath.Join(directory, "bin")
			}
			paths = append(paths, directory)
		}
	}
	args := append([]string{"-e", npmPermissionProbe, "--"}, paths...)
	output, err := service.runner().Run(ctx, "node", args...)
	if err != nil {
		return classifyNPMError(err, output)
	}
	return nil
}

// Only disposable probe objects are renamed or removed. Existing packages,
// launchers, journals and credentials are never changed by this check. Probe
// both a populated directory move and replacement of an existing file, then
// verify cleanup. Missing target directories use their nearest existing parent.
const npmPermissionProbe = `
const fs = require('node:fs/promises');
const path = require('node:path');
(async () => {
  const checked = new Set();
  for (let directory of process.argv.slice(1)) {
    for (;;) {
      try {
        if (!(await fs.stat(directory)).isDirectory()) throw {code: 'ENOTDIR'};
        break;
      } catch (error) {
        if (error.code !== 'ENOENT' || path.dirname(directory) === directory) throw error;
        directory = path.dirname(directory);
      }
    }
    directory = await fs.realpath(directory);
    if (checked.has(directory)) continue;
    checked.add(directory);
    const probe = await fs.mkdtemp(path.join(directory, '.viceme-permission-probe-'));
    try {
      await fs.mkdir(path.join(probe, 'package'));
      await fs.writeFile(path.join(probe, 'package', 'entry'), 'probe', {mode: 0o600});
      await fs.rename(path.join(probe, 'package'), path.join(probe, 'retired'));
      await fs.writeFile(path.join(probe, 'staged'), 'probe', {mode: 0o600});
      await fs.writeFile(path.join(probe, 'active'), 'probe', {mode: 0o600});
      await fs.rename(path.join(probe, 'staged'), path.join(probe, 'active'));
    } finally {
      await fs.rm(probe, {recursive: true, force: true});
    }
  }
})().catch(error => {
  const codes = ['CODEBUDDY_BROKER_DENY', 'EPERM', 'EACCES', 'EROFS'];
  process.stderr.write(codes.includes(error.code) ? error.code : 'NPM_PERMISSION_PROBE_FAILED');
  process.exitCode = 1;
});
`
