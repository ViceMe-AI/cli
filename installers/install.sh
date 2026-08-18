#!/bin/sh
set -eu

region="${VICEME_REGION:-cn}"
case "$region" in
  cn) base_url="${VICEME_DOWNLOAD_BASE_URL:-https://s3.viceme.cn/start/cli/releases}"; api_base_url="https://api.viceme.cn" ;;
  global) base_url="${VICEME_DOWNLOAD_BASE_URL:-https://s3.viceme.ai/start/cli/releases}"; api_base_url="https://api.viceme.ai" ;;
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
cleanup() {
  status=$?
  trap - EXIT HUP INT TERM
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
destination="$install_dir/viceme"
chmod 755 "$temporary/viceme"
"$temporary/viceme" bootstrap activate \
  --destination "$destination" --agent auto --region "$region" \
  --api-base-url "$api_base_url" --release-channel stable \
  --release-base-url "$base_url" --allow-channel-switch

resolved="$(command -v viceme 2>/dev/null || true)"
if [ "$resolved" != "$destination" ]; then
  echo "ViceMe was installed to $destination, but another command may take precedence: ${resolved:-not found}" >&2
  echo "Put this directory first in PATH: export PATH=\"$install_dir:\$PATH\"" >&2
fi
