#!/bin/sh
set -eu

region="${VICEME_REGION:-cn}"
case "$region" in
  cn) base_url="${VICEME_DOWNLOAD_BASE_URL:-https://s3.viceme.cn/cli/releases}" ;;
  global) base_url="${VICEME_DOWNLOAD_BASE_URL:-https://s3.viceme.ai/cli/releases}" ;;
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
activation_pending=0
had_existing=0
destination=""
backup="$temporary/previous-viceme"
lock_dir=""
cleanup() {
  status=$?
  trap - EXIT HUP INT TERM
  if [ "$activation_pending" -eq 1 ] && [ -n "$destination" ]; then
    rm -f "$destination"
    if [ "$had_existing" -eq 1 ] && [ -f "$backup" ]; then
      mv -f "$backup" "$destination"
    fi
  fi
  [ -z "$lock_dir" ] || rmdir "$lock_dir" 2>/dev/null || true
  rm -rf "$temporary"
  exit "$status"
}
trap cleanup EXIT HUP INT TERM

version="${VICEME_VERSION:-}"
if [ -z "$version" ]; then
  curl -fsSL --connect-timeout 15 --max-time 120 --proto '=https' --tlsv1.2 "$base_url/latest" -o "$temporary/latest"
  version="$(tr -d '\r\n' <"$temporary/latest")"
fi
printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || { echo "Release index returned an invalid version" >&2; exit 1; }

asset="viceme_${version}_${goos}_${goarch}"
release_url="$base_url/v${version}"
curl -fsSL --connect-timeout 15 --max-time 300 --proto '=https' --tlsv1.2 "$release_url/$asset" -o "$temporary/viceme"
curl -fsSL --connect-timeout 15 --max-time 120 --proto '=https' --tlsv1.2 "$release_url/$asset.sha256" -o "$temporary/viceme.sha256"

expected="$(awk 'NR == 1 { print $1 }' "$temporary/viceme.sha256")"
[ "${#expected}" -eq 64 ] || { echo "Release checksum is invalid" >&2; exit 1; }
case "$expected" in *[!0-9a-f]*) echo "Release checksum is invalid" >&2; exit 1 ;; esac
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$temporary/viceme" | awk '{print $1}')"
else
  actual="$(shasum -a 256 "$temporary/viceme" | awk '{print $1}')"
fi
[ "$actual" = "$expected" ] || { echo "ViceMe binary checksum verification failed" >&2; exit 1; }

install_dir="${VICEME_INSTALL_DIR:-$HOME/.local/bin}"
mkdir -p "$install_dir"
lock_dir="$install_dir/.viceme-install-lock"
mkdir "$lock_dir" 2>/dev/null || { echo "Another ViceMe install is active" >&2; exit 1; }
staged_binary="$(mktemp "$install_dir/.viceme.XXXXXX")"
cp "$temporary/viceme" "$staged_binary"
chmod 755 "$staged_binary"
destination="$install_dir/viceme"
if [ -f "$destination" ]; then
  cp "$destination" "$backup"
  chmod 755 "$backup"
  had_existing=1
fi
activation_pending=1
mv -f "$staged_binary" "$destination"

"$destination" install --agent auto --region "$region"
"$destination" doctor --agent auto
activation_pending=0

case ":$PATH:" in
  *":$install_dir:"*) ;;
  *)
    echo "ViceMe was installed to $install_dir/viceme" >&2
    echo "Add this directory to PATH: export PATH=\"$install_dir:\$PATH\"" >&2
    ;;
esac
