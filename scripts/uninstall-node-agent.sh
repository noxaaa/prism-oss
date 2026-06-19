#!/usr/bin/env sh
set -eu

service_name="prism-node-agent"
install_dir="/opt/prism-node-agent"
config_file="/etc/prism-node-agent/agent.env"
purge=""
local_binary=""
repo="https://github.com/noxaaa/prism-oss"
version="latest"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version) version="${2:-}"; shift 2 ;;
    --service-name) service_name="${2:-}"; shift 2 ;;
    --install-dir) install_dir="${2:-}"; shift 2 ;;
    --config-file) config_file="${2:-}"; shift 2 ;;
    --purge) purge="--purge"; shift ;;
    --node-agent) local_binary="${2:-}"; shift 2 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

if [ -n "$local_binary" ]; then
  "$local_binary" uninstall --service-name "$service_name" --install-dir "$install_dir" --config-file "$config_file" $purge
  exit 0
fi

url_path_escape() {
  printf '%s' "$1" | sed 's/%/%25/g; s/+/%2B/g; s/&/%26/g; s#/#%2F#g; s/ /%20/g; s/#/%23/g; s/?/%3F/g'
}

case "$(uname -m)" in
  x86_64|amd64) asset="node-agent-linux-amd64.tar.gz" ;;
  aarch64|arm64) asset="node-agent-linux-arm64.tar.gz" ;;
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
"$tmp_dir/node-agent" uninstall --service-name "$service_name" --install-dir "$install_dir" --config-file "$config_file" $purge
