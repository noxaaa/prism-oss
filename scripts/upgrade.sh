#!/usr/bin/env sh
set -eu

release_repo="https://github.com/noxaaa/prism-oss/releases"
install_dir="$(pwd -P)"
version="latest"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --dir) install_dir="${2:?missing value for --dir}"; shift 2 ;;
    --version) version="${2:?missing value for --version}"; shift 2 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

if [ ! -f "$install_dir/.env" ] || [ ! -f "$install_dir/docker-compose.yml" ]; then
  echo "$install_dir is not an installed prism-oss directory" >&2
  exit 1
fi
install_dir="$(cd "$install_dir" && pwd -P)"

url_path_unescape() {
  value="$1"
  decoded=""
  while [ -n "$value" ]; do
    case "$value" in
      *%[0123456789ABCDEFabcdef][0123456789ABCDEFabcdef]*)
        prefix="${value%%\%[0123456789ABCDEFabcdef][0123456789ABCDEFabcdef]*}"
        rest="${value#"$prefix"%}"
        hex="${rest%"${rest#??}"}"
        char="$(printf "\\$(printf '%03o' "0x$hex")")"
        decoded="${decoded}${prefix}${char}"
        value="${rest#??}"
        ;;
      *)
        decoded="${decoded}${value}"
        value=""
        ;;
    esac
  done
  printf '%s' "$decoded"
}

resolve_release_version() {
  if [ "$version" != "latest" ]; then
    printf '%s' "$version"
    return
  fi
  if ! command -v curl >/dev/null 2>&1; then
    echo "curl is required to resolve latest release version" >&2
    exit 1
  fi
  effective_url="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$release_repo/latest" 2>/dev/null || true)"
  case "$effective_url" in
    */tag/*)
      tag="${effective_url##*/tag/}"
      tag="${tag%%[?#]*}"
      if [ -n "$tag" ]; then
        url_path_unescape "$tag"
        return
      fi
      ;;
  esac
  echo "failed to resolve latest release version" >&2
  exit 1
}

resolved_version="$(resolve_release_version)"

release_asset_url() {
  printf '%s/download/%s/%s' "$release_repo" "$1" "$2"
}

target_install="$(mktemp)"
cleanup_target_install() {
  rm -f "$target_install"
}
trap cleanup_target_install EXIT INT TERM

target_install_url="$(release_asset_url "$resolved_version" "install.sh")"
if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required to download the target release installer" >&2
  exit 1
fi
curl -fsSL "$target_install_url" -o "$target_install"
sh "$target_install" --dir "$install_dir" --version "$resolved_version"

echo "Upgraded prism-oss to ${resolved_version}"
