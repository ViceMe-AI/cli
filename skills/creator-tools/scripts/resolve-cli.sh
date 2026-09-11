#!/bin/sh
set -eu

# Resolve the existing entrypoint, preserving PATH precedence and npm launchers.
# This helper never installs, updates, searches the disk, or changes a profile.
windows_posix=0
case "${OS:-}" in
  Windows_NT) windows_posix=1 ;;
esac
if [ "$windows_posix" -eq 0 ]; then
  kernel="$(uname -s 2>/dev/null || true)"
  case "$kernel" in
    CYGWIN*|MINGW*|MSYS*) windows_posix=1 ;;
  esac
fi

as_posix_path() {
  if [ "$windows_posix" -eq 1 ] && command -v cygpath >/dev/null 2>&1; then
    cygpath -u "$1"
  else
    printf '%s\n' "$1"
  fi
}

if candidate="$(command -v viceme)"; then
  :
elif [ -n "${VICEME_INSTALL_DIR:-}" ]; then
  install_dir="$(as_posix_path "$VICEME_INSTALL_DIR")"
  if [ "$windows_posix" -eq 1 ]; then
    candidate="$install_dir/viceme.exe"
  else
    candidate="$install_dir/viceme"
  fi
elif [ "$windows_posix" -eq 1 ] && [ -n "${LOCALAPPDATA:-}" ]; then
  local_app_data="$(as_posix_path "$LOCALAPPDATA")"
  candidate="$local_app_data/ViceMe/bin/viceme.exe"
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
