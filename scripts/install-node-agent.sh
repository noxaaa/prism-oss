#!/usr/bin/env sh
set -eu

version="latest"
control_url=""
registration_token=""
credential_file="agent-credential.json"
repo="https://github.com/noxaaa/prism-oss"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      version="${2:-}"
      shift 2
      ;;
    --control-url)
      control_url="${2:-}"
      shift 2
      ;;
    --registration-token)
      registration_token="${2:-}"
      shift 2
      ;;
    --credential-file)
      credential_file="${2:-}"
      shift 2
      ;;
    *)
      echo "unknown option: $1" >&2
      exit 2
      ;;
  esac
done

if [ -z "$control_url" ] || [ -z "$registration_token" ] || [ -z "$credential_file" ]; then
  echo "--control-url, --registration-token, and --credential-file are required" >&2
  exit 2
fi

url_path_escape() {
  printf '%s' "$1" | sed 's/%/%25/g; s/+/%2B/g; s/&/%26/g; s#/#%2F#g; s/ /%20/g; s/#/%23/g; s/?/%3F/g'
}

case "$(uname -m)" in
  x86_64|amd64) asset="node-agent-linux-amd64.tar.gz" ;;
  aarch64|arm64) asset="node-agent-linux-arm64.tar.gz" ;;
  *)
    echo "unsupported architecture: $(uname -m)" >&2
    exit 1
    ;;
esac

if [ "$version" = "latest" ]; then
  base="${repo}/releases/latest/download"
else
  escaped_version="$(url_path_escape "$version")"
  base="${repo}/releases/download/${escaped_version}"
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
cp "$tmp_dir/node-agent" ./node-agent
chmod +x ./node-agent

APP_NAME="${APP_NAME:-OSS Control Console}" ./node-agent install \
  --control-url "$control_url" \
  --registration-token "$registration_token" \
  --credential-file "$credential_file"
