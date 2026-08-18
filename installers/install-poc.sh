#!/bin/sh
set -eu

version="0.16.1-poc.1"
release_tag="danmaku-poc-20260818-v4"
release_url="https://github.com/ViceMe-AI/cli/releases/download/${release_tag}"
profile_name="danmaku-poc-20260818"
api_base_url="https://api-poc.preview.tencent-zeabur.cn"

command -v curl >/dev/null 2>&1 || {
  echo "curl is required to install the ViceMe danmaku POC" >&2
  exit 1
}

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

asset="viceme_${version}_${goos}_${goarch}"
curl -fsSL --connect-timeout 15 --max-time 300 --proto '=https' --tlsv1.2 \
  "$release_url/$asset" -o "$temporary/viceme"
curl -fsSL --connect-timeout 15 --max-time 120 --proto '=https' --tlsv1.2 \
  "$release_url/$asset.sha256" -o "$temporary/viceme.sha256"

expected="$(awk 'NR == 1 { print $1 }' "$temporary/viceme.sha256")"
[ "${#expected}" -eq 64 ] || {
  echo "ViceMe danmaku POC checksum is invalid" >&2
  exit 1
}
case "$expected" in
  *[!0-9a-f]*) echo "ViceMe danmaku POC checksum is invalid" >&2; exit 1 ;;
esac
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$temporary/viceme" | awk '{print $1}')"
else
  actual="$(shasum -a 256 "$temporary/viceme" | awk '{print $1}')"
fi
[ "$actual" = "$expected" ] || {
  echo "ViceMe danmaku POC binary verification failed" >&2
  exit 1
}

install_dir="${VICEME_INSTALL_DIR:-$HOME/.local/bin}"
destination="$install_dir/viceme"
mkdir -p "$install_dir"
chmod 755 "$temporary/viceme"
"$temporary/viceme" bootstrap activate \
  --destination "$destination" \
  --agent "${VICEME_AGENT_TARGET:-auto}" \
  --region cn

if "$destination" profile use "$profile_name" >/dev/null 2>&1; then
  :
else
  "$destination" profile add \
    --name "$profile_name" \
    --region cn \
    --api-base-url "$api_base_url" \
    --use
fi

echo "ViceMe danmaku POC is ready."
echo "Next: run '$destination auth login', then start a new Agent conversation."
case ":$PATH:" in
  *":$install_dir:"*) ;;
  *) echo "Add this directory to PATH: export PATH=\"$install_dir:\$PATH\"" >&2 ;;
esac
