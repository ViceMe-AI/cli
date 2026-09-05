#!/bin/sh
set -eu
umask 077

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    echo "sha256sum or shasum is required to verify the ViceMe binary" >&2
    return 1
  fi
}

region="${VICEME_REGION:-cn}"
case "$region" in
  cn) base_url="${VICEME_DOWNLOAD_BASE_URL:-https://s3.viceme.cn/start/cli/releases}" ;;
  global) base_url="${VICEME_DOWNLOAD_BASE_URL:-https://s3.viceme.ai/start/cli/releases}" ;;
  *) echo "VICEME_REGION must be cn or global" >&2; exit 2 ;;
esac

command -v curl >/dev/null 2>&1 || { echo "curl is required to install ViceMe" >&2; exit 1; }

os_name="$(uname -s)"
case "$os_name" in
  Darwin) goos="darwin" ;;
  Linux) goos="linux" ;;
  *) echo "Unsupported operating system: $os_name" >&2; exit 2 ;;
esac

machine="$(uname -m)"
case "$machine" in
  x86_64|amd64) goarch="amd64" ;;
  arm64|aarch64) goarch="arm64" ;;
  *) echo "Unsupported CPU architecture: $machine" >&2; exit 2 ;;
esac

temporary="$(mktemp -d "${TMPDIR:-/tmp}/viceme-install.XXXXXX")"
cache_file=""
verified=false
cleanup() {
  status=$?
  trap - EXIT HUP INT TERM
  set +e
  # Reusable bytes are not activation state. Never alter the CLI's journals.
  if [ "$verified" = true ] && [ "$status" -eq 6 ]; then
    cache_stage="$(mktemp "$cache_root/.retain.XXXXXX")"
    if [ -n "$cache_stage" ] && cp "$temporary/viceme" "$cache_stage"; then
      mv -f "$cache_stage" "$cache_file"
    fi
    [ -z "$cache_stage" ] || rm -f "$cache_stage"
  elif [ -n "$cache_file" ]; then
    rm -f "$cache_file"
  fi
  rm -rf "$temporary"
  exit "$status"
}
trap cleanup EXIT HUP INT TERM

version="${VICEME_VERSION:-}"
if [ -z "$version" ]; then
  curl -fsSL --connect-timeout 15 --max-time 120 --retry 3 --retry-delay 2 --proto '=https' --tlsv1.2 "$base_url/latest" -o "$temporary/latest"
  version="$(tr -d '\r\n' <"$temporary/latest")"
fi
printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || { echo "Release index returned an invalid version" >&2; exit 1; }

asset="viceme_${version}_${goos}_${goarch}"
release_url="$base_url/v${version}"
# Revalidate against the selected official origin on every permission retry.
curl -fsSL --connect-timeout 15 --max-time 120 --retry 3 --retry-delay 2 --proto '=https' --tlsv1.2 "$release_url/$asset.sha256" -o "$temporary/viceme.sha256"

expected="$(awk 'NR == 1 { print $1 }' "$temporary/viceme.sha256")"
[ "${#expected}" -eq 64 ] || { echo "Release checksum is invalid" >&2; exit 1; }
case "$expected" in *[!0-9a-f]*) echo "Release checksum is invalid" >&2; exit 1 ;; esac
printf '%s\n%s\n' "$release_url/$asset" "$expected" >"$temporary/cache-key"
cache_key="$(sha256_file "$temporary/cache-key")"
cache_root="${TMPDIR:-/tmp}/viceme-bootstrap-cache-$(id -u)"
# TMPDIR may be shared. Reject foreign directories and symlinks before using
# an owner-only cache; atomic publication keeps concurrent retries independent.
[ ! -L "$cache_root" ] || { echo "Unsafe ViceMe download cache" >&2; exit 1; }
if ! mkdir -m 700 "$cache_root" 2>/dev/null; then
  [ -d "$cache_root" ] && [ "$(ls -nd "$cache_root" | awk '{print $3}')" = "$(id -u)" ] || { echo "Unsafe ViceMe download cache" >&2; exit 1; }
  chmod 700 "$cache_root"
fi
# Expiration is best effort while other installers publish or remove entries.
find "$cache_root" -type f -mtime +1 -exec rm -f {} + 2>/dev/null || :
cache_file="$cache_root/$cache_key"
if [ ! -L "$cache_file" ] && [ -f "$cache_file" ]; then
  if cp "$cache_file" "$temporary/viceme"; then
    if [ "$(sha256_file "$temporary/viceme")" != "$expected" ]; then
      rm -f "$temporary/viceme" "$cache_file"
    fi
  else
    # A concurrent successful activation may have removed this cache entry.
    rm -f "$temporary/viceme"
  fi
fi
if [ ! -f "$temporary/viceme" ]; then
  curl -fsSL --connect-timeout 15 --max-time 300 --retry 3 --retry-delay 2 --proto '=https' --tlsv1.2 "$release_url/$asset" -o "$temporary/viceme"
fi
actual="$(sha256_file "$temporary/viceme")"
[ "$actual" = "$expected" ] || { echo "ViceMe binary checksum verification failed" >&2; exit 1; }
verified=true

install_dir="${VICEME_INSTALL_DIR:-}"
if [ -z "$install_dir" ]; then
  [ -n "${HOME:-}" ] || { echo "HOME is not set; set VICEME_INSTALL_DIR to choose an install directory" >&2; exit 1; }
  install_dir="$HOME/.local/bin"
fi
mkdir -p "$install_dir"
destination="$install_dir/viceme"
chmod 755 "$temporary/viceme"
"$temporary/viceme" bootstrap activate --destination "$destination" --agent auto --region "$region"

case ":$PATH:" in
  *":$install_dir:"*) ;;
  *)
    echo "ViceMe was installed to $install_dir/viceme" >&2
    echo "Add this directory to PATH: export PATH=\"$install_dir:\$PATH\"" >&2
    ;;
esac
