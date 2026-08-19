#!/bin/sh
set -eu

region="cn"
base_url="${VICEME_POC_DOWNLOAD_BASE_URL:-https://viceme-shop-storage-poc.preview.tencent-zeabur.cn/start/poc/cli/releases}"
api_base_url="${VICEME_POC_API_BASE_URL:-https://viceme-shop-web-poc.preview.tencent-zeabur.cn/api}"

command -v curl >/dev/null 2>&1 || { echo "curl is required to install ViceMe POC" >&2; exit 1; }

case "$(uname -s)" in
  Darwin) goos="darwin" ;;
  Linux) goos="linux" ;;
  *) echo "Unsupported operating system: $(uname -s)" >&2; exit 2 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) goarch="amd64" ;;
  arm64|aarch64) goarch="arm64" ;;
  *) echo "Unsupported CPU architecture: $(uname -m)" >&2; exit 2 ;;
esac

temporary="$(mktemp -d "${TMPDIR:-/tmp}/viceme-poc-install.XXXXXX")"
cleanup() {
  status=$?
  trap - EXIT HUP INT TERM
  rm -rf "$temporary"
  exit "$status"
}
trap cleanup EXIT HUP INT TERM

version="${VICEME_POC_VERSION:-}"
if [ -z "$version" ]; then
  curl -fsSL --connect-timeout 15 --max-time 120 --proto '=https' --tlsv1.2 "$base_url/latest" -o "$temporary/latest"
  version="$(tr -d '\r\n' <"$temporary/latest")"
fi
printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+-poc\.[0-9]+$' || {
  echo "POC release index returned an invalid version" >&2
  exit 1
}

asset="viceme_${version}_${goos}_${goarch}"
release_url="$base_url/v${version}"
curl -fsSL --connect-timeout 15 --max-time 300 --proto '=https' --tlsv1.2 "$release_url/$asset" -o "$temporary/viceme"
curl -fsSL --connect-timeout 15 --max-time 120 --proto '=https' --tlsv1.2 "$release_url/$asset.sha256" -o "$temporary/viceme.sha256"

expected="$(awk 'NR == 1 { print $1 }' "$temporary/viceme.sha256")"
[ "${#expected}" -eq 64 ] || { echo "POC release checksum is invalid" >&2; exit 1; }
case "$expected" in *[!0-9a-f]*) echo "POC release checksum is invalid" >&2; exit 1 ;; esac
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$temporary/viceme" | awk '{print $1}')"
else
  actual="$(shasum -a 256 "$temporary/viceme" | awk '{print $1}')"
fi
[ "$actual" = "$expected" ] || { echo "ViceMe POC binary checksum verification failed" >&2; exit 1; }

install_dir="${VICEME_INSTALL_DIR:-$HOME/.local/bin}"
destination="$install_dir/viceme"
mkdir -p "$install_dir"
chmod 755 "$temporary/viceme"
"$temporary/viceme" bootstrap activate \
  --destination "$destination" \
  --agent "${VICEME_AGENT_TARGET:-auto}" \
  --region "$region" \
  --api-base-url "$api_base_url" \
  --release-channel poc \
  --release-base-url "$base_url" \
  --allow-channel-switch

echo "ViceMe POC is ready. The 'viceme' command now uses the POC API and POC update channel."
echo "Next: run '$destination auth login', then start a new Agent conversation."
case ":$PATH:" in
  *":$install_dir:"*) ;;
  *) echo "Add this directory to PATH: export PATH=\"$install_dir:\$PATH\"" >&2 ;;
esac
