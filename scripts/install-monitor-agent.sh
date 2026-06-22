#!/usr/bin/env sh
set -eu

version="latest"
control_url=""
registration_token=""
credential_file="/var/lib/prism-monitor-agent/agent-credential.json"
service_name="prism-monitor-agent"
install_dir="/opt/prism-monitor-agent"
config_file="/etc/prism-monitor-agent/agent.env"
app_name="${APP_NAME:-OSS Control Console}"
repo="https://github.com/noxaaa/prism-oss"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version) version="${2:-}"; shift 2 ;;
    --control-url) control_url="${2:-}"; shift 2 ;;
    --registration-token) registration_token="${2:-}"; shift 2 ;;
    --credential-file) credential_file="${2:-}"; shift 2 ;;
    --service-name) service_name="${2:-}"; shift 2 ;;
    --install-dir) install_dir="${2:-}"; shift 2 ;;
    --config-file) config_file="${2:-}"; shift 2 ;;
    --app-name) app_name="${2:-}"; shift 2 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

if [ -z "$control_url" ] || [ -z "$registration_token" ]; then
  echo "--control-url and --registration-token are required" >&2
  exit 2
fi

url_path_escape() {
  printf '%s' "$1" | sed 's/%/%25/g; s/+/%2B/g; s/&/%26/g; s#/#%2F#g; s/ /%20/g; s/#/%23/g; s/?/%3F/g'
}

case "$(uname -m)" in
  x86_64|amd64) asset="monitor-agent-linux-amd64.tar.gz" ;;
  aarch64|arm64) asset="monitor-agent-linux-arm64.tar.gz" ;;
  *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

if [ "$version" = "latest" ]; then
  base="${repo}/releases/latest/download"
else
  base="${repo}/releases/download/$(url_path_escape "$version")"
fi

tmp_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT HUP INT TERM

curl -fsSL "${base}/SHA256SUMS" -o "$tmp_dir/SHA256SUMS"
curl -fsSL "${base}/${asset}" -o "$tmp_dir/${asset}"

(
  cd "$tmp_dir"
  if command -v sha256sum >/dev/null 2>&1; then
    grep "  ${asset}\$" SHA256SUMS | sha256sum -c -
  elif command -v shasum >/dev/null 2>&1; then
    grep "  ${asset}\$" SHA256SUMS | shasum -a 256 -c -
  else
    echo "sha256sum or shasum is required" >&2
    exit 1
  fi
)

tar -xzf "$tmp_dir/${asset}" -C "$tmp_dir"

"$tmp_dir/monitor-agent" install \
  --app-name "$app_name" \
  --control-url "$control_url" \
  --registration-token "$registration_token" \
  --credential-file "$credential_file" \
  --service-name "$service_name" \
  --install-dir "$install_dir" \
  --config-file "$config_file"
