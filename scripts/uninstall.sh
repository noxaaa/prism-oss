#!/usr/bin/env sh
set -eu

install_dir="$(pwd -P)"
purge=0

usage() {
  cat <<'USAGE'
Usage: uninstall.sh [options]

Options:
  --dir DIR   Installed prism-oss directory. Defaults to the current directory.
  --purge     Remove Docker volumes and generated local install files.
  -h, --help  Show this help.
USAGE
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --dir) install_dir="${2:?missing value for --dir}"; shift 2 ;;
    --purge) purge=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

if [ ! -f "$install_dir/.env" ] || [ ! -f "$install_dir/docker-compose.yml" ]; then
  echo "$install_dir is not an installed prism-oss directory" >&2
  exit 1
fi
install_dir="$(cd "$install_dir" && pwd -P)"

if ! docker compose version >/dev/null 2>&1; then
  echo "docker compose v2 is required" >&2
  exit 1
fi

cd "$install_dir"

if [ "$purge" -eq 1 ]; then
  docker compose down -v --remove-orphans
  rm -rf geoip
  rm -f .env docker-compose.yml upgrade.sh uninstall.sh
  echo "Uninstalled prism-oss and removed generated config plus Docker volumes."
else
  docker compose down --remove-orphans
  echo "Uninstalled prism-oss containers. Local config and Docker volumes were preserved."
fi
