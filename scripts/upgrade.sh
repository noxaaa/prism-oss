#!/usr/bin/env sh
set -eu

install_dir="$(pwd -P)"
version="latest"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --dir)
      install_dir="${2:-}"
      shift 2
      ;;
    --version)
      version="${2:-}"
      shift 2
      ;;
    *)
      echo "unknown option: $1" >&2
      exit 2
      ;;
  esac
done

if [ "$version" = "latest" ]; then
  install_url="https://github.com/noxaaa/prism-oss/releases/latest/download/install.sh"
else
  escaped_version="$(printf '%s' "$version" | sed 's/%/%25/g; s/+/%2B/g; s/&/%26/g; s#/#%2F#g; s/ /%20/g; s/#/%23/g; s/?/%3F/g')"
  install_url="https://github.com/noxaaa/prism-oss/releases/download/${escaped_version}/install.sh"
fi

tmp_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT HUP INT TERM

curl -fsSL "$install_url" -o "$tmp_dir/install.sh"
sh "$tmp_dir/install.sh" --dir "$install_dir" --version "$version"
