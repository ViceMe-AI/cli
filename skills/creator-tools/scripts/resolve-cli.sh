#!/bin/sh
set -eu

# Resolve the existing entrypoint, preserving PATH precedence and npm launchers.
# This helper never installs, updates, searches the disk, or changes a profile.
if candidate="$(command -v viceme)"; then
  :
elif [ -n "${VICEME_INSTALL_DIR:-}" ]; then
  candidate="$VICEME_INSTALL_DIR/viceme"
elif [ -n "${HOME:-}" ]; then
  candidate="$HOME/.local/bin/viceme"
else
  echo "CLI_NOT_FOUND" >&2
  exit 127
fi

if [ ! -f "$candidate" ] && [ ! -L "$candidate" ]; then
  echo "CLI_NOT_FOUND" >&2
  exit 127
fi
if [ ! -f "$candidate" ] || [ ! -x "$candidate" ]; then
  echo "CLI_EXECUTION_PERMISSION_REQUIRED: request host permission for the existing CLI" >&2
  exit 6
fi

directory="$(CDPATH= cd -P -- "$(dirname -- "$candidate")" && pwd)"
printf '%s/%s\n' "$directory" "$(basename -- "$candidate")"
