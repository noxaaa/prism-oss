#!/usr/bin/env sh
set -eu

release_repo="https://github.com/noxaaa/prism-oss/releases"
install_dir="${HOME}/prism-oss"
app_name="OSS Control Console"
web_port="3000"
control_port="8080"
control_bind_host="0.0.0.0"
control_url=""
dir_was_set="0"
version="latest"

usage() {
  cat <<'USAGE'
Usage: scripts/install.sh [options]

Options:
  --version VERSION     Release tag to install. Defaults to latest.
  --dir DIR             Installation directory. Defaults to the current repo when run from an OSS tree, otherwise $HOME/prism-oss.
  --app-name NAME       Console display name. Defaults to "OSS Control Console".
  --web-port PORT       Host port for the web console. Defaults to 3000.
  --control-port PORT   Host port for the control-plane API. Defaults to 8080.
  --control-bind-host HOST
                         Host interface for the control-plane API. Defaults to 0.0.0.0.
  --control-url URL      URL that node and monitor agents use to reach the control plane.
  -h, --help            Show this help.
USAGE
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      [ "$#" -ge 2 ] || { echo "missing value for --version" >&2; exit 2; }
      version="$2"
      shift 2
      ;;
    --dir)
      [ "$#" -ge 2 ] || { echo "missing value for --dir" >&2; exit 2; }
      install_dir="$2"
      dir_was_set="1"
      shift 2
      ;;
    --app-name)
      [ "$#" -ge 2 ] || { echo "missing value for --app-name" >&2; exit 2; }
      app_name="$2"
      shift 2
      ;;
    --web-port)
      [ "$#" -ge 2 ] || { echo "missing value for --web-port" >&2; exit 2; }
      web_port="$2"
      shift 2
      ;;
    --control-port)
      [ "$#" -ge 2 ] || { echo "missing value for --control-port" >&2; exit 2; }
      control_port="$2"
      shift 2
      ;;
    --control-bind-host)
      [ "$#" -ge 2 ] || { echo "missing value for --control-bind-host" >&2; exit 2; }
      control_bind_host="$2"
      shift 2
      ;;
    --control-url)
      [ "$#" -ge 2 ] || { echo "missing value for --control-url" >&2; exit 2; }
      control_url="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [ "$dir_was_set" = "0" ] && [ -f "go.mod" ] && grep -q '^module github.com/noxaaa/prism-oss$' "go.mod"; then
  install_dir="$(pwd)"
fi

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "$1 is required" >&2
    exit 1
  fi
}

generate_secret() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 48 | tr -d '\n'
    return
  fi
  if command -v python3 >/dev/null 2>&1; then
    python3 -c 'import secrets; print(secrets.token_urlsafe(48))'
    return
  fi
  echo "openssl or python3 is required to generate secrets" >&2
  exit 1
}

secret_or_exit() {
  name="$1"
  secret="$(generate_secret)" || {
    echo "failed to generate ${name}" >&2
    exit 1
  }
  if [ -z "$secret" ]; then
    echo "generated empty ${name}" >&2
    exit 1
  fi
  printf '%s' "$secret"
}

validate_module_path() {
  checkout_dir="$1"
  if [ ! -f "$checkout_dir/go.mod" ] || ! grep -q '^module github.com/noxaaa/prism-oss$' "$checkout_dir/go.mod"; then
    echo "$checkout_dir is not a github.com/noxaaa/prism-oss install tree" >&2
    exit 1
  fi
}

release_download_base() {
  if [ "$version" = "latest" ]; then
    printf '%s/latest/download' "$release_repo"
  else
    printf '%s/download/%s' "$release_repo" "$version"
  fi
}

release_source_url() {
  printf '%s/prism-oss-source.tar.gz' "$(release_download_base)"
}

download_public_url() {
  download_url="$1"
  output_path="$2"
  curl -fsSL "$download_url" -o "$output_path"
}

download_release_source() {
  output_path="$1"
  download_public_url "$(release_source_url)" "$output_path"
}

first_path_in_dir() {
  find "$1" -mindepth 1 -maxdepth 1 -print -quit 2>/dev/null || true
}

prepare_install_dir() {
  destination="$1"
  if [ -e "$destination" ] && [ ! -d "$destination" ]; then
    echo "$destination exists but is not a directory" >&2
    exit 1
  fi
  if [ -d "$destination" ]; then
    if [ -f "$destination/go.mod" ]; then
      validate_module_path "$destination"
    elif [ -n "$(first_path_in_dir "$destination")" ]; then
      echo "$destination exists but is not an OSS install tree" >&2
      exit 1
    fi
  else
    mkdir -p "$destination"
  fi
}

download_release() {
  destination="$1"
  prepare_install_dir "$destination"
  tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/prism-oss-install.XXXXXX")"
  archive_path="$tmp_dir/prism-oss-source.tar.gz"
  extract_dir="$tmp_dir/extract"
  trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM
  mkdir -p "$extract_dir"

  echo "Downloading OSS release ${version}"
  download_release_source "$archive_path"
  tar -xzf "$archive_path" -C "$extract_dir"

  source_root=""
  if [ -f "$extract_dir/go.mod" ]; then
    source_root="$extract_dir"
  else
    for candidate in "$extract_dir"/*; do
      if [ -d "$candidate" ]; then
        source_root="$candidate"
        break
      fi
    done
  fi
  if [ -z "$source_root" ]; then
    echo "release archive does not contain an install tree" >&2
    exit 1
  fi
  validate_module_path "$source_root"

  rm -rf \
    "$destination/.github" \
    "$destination/apps" \
    "$destination/cmd" \
    "$destination/docs" \
    "$destination/internal" \
    "$destination/migrations" \
    "$destination/pkg" \
    "$destination/scripts" \
    "$destination/.golangci.yml" \
    "$destination/.gitignore" \
    "$destination/README.md" \
    "$destination/LICENSE" \
    "$destination/Makefile" \
    "$destination/docker-compose.yml" \
    "$destination/go.mod" \
    "$destination/go.sum" \
    "$destination/package.json" \
    "$destination/package-lock.json" \
    "$destination/node-agent" \
    "$destination/monitor-agent"

  (cd "$source_root" && tar -cf - .) | (cd "$destination" && tar -xf -)
  printf '%s\n' "$version" > "$destination/.prism-oss-version"
  rm -rf "$tmp_dir"
  trap - EXIT HUP INT TERM
}

require_command curl
require_command tar
require_command docker
if ! docker compose version >/dev/null 2>&1; then
  echo "docker compose v2 is required" >&2
  exit 1
fi

download_release "$install_dir"

cd "$install_dir"

env_value() {
  key="$1"
  if [ -f ".env" ]; then
    sed -n "s/^${key}=//p" ".env" | tail -n 1
  fi
}

if [ ! -f ".env" ]; then
  if [ -z "$control_url" ]; then
    control_url="http://127.0.0.1:${control_port}"
  fi
  umask 077
  better_auth_secret="$(secret_or_exit BETTER_AUTH_SECRET)"
  internal_jwt_secret="$(secret_or_exit CONTROL_PLANE_INTERNAL_JWT_SECRET)"
  agent_token_secret="$(secret_or_exit AGENT_TOKEN_SIGNING_SECRET)"
  tmp_env=".env.tmp.$$"
  trap 'rm -f "$tmp_env"' EXIT HUP INT TERM
  {
    printf 'APP_NAME=%s\n' "$app_name"
    printf 'APP_ENV=production\n'
    printf 'PRISM_EDITION=oss\n'
    printf 'WEB_PORT=%s\n' "$web_port"
    printf 'CONTROL_PLANE_PORT=%s\n' "$control_port"
    printf 'CONTROL_PLANE_BIND_HOST=%s\n' "$control_bind_host"
    printf 'PUBLIC_WEB_URL=http://127.0.0.1:%s\n' "$web_port"
    printf 'CONTROL_PLANE_URL=%s\n' "$control_url"
    printf 'CONTROL_PLANE_INTERNAL_URL=http://control-plane:8080\n'
    printf 'PRISM_OSS_DATABASE_URL=/data/oss.db\n'
    printf 'QUEUE_REDIS_URL=redis://redis:6379/0\n'
    printf 'CACHE_REDIS_URL=redis://redis:6379/0\n'
    printf 'BETTER_AUTH_SECRET=%s\n' "$better_auth_secret"
    printf 'CONTROL_PLANE_INTERNAL_JWT_SECRET=%s\n' "$internal_jwt_secret"
    printf 'AGENT_TOKEN_SIGNING_SECRET=%s\n' "$agent_token_secret"
  } > "$tmp_env"
  mv "$tmp_env" ".env"
  trap - EXIT HUP INT TERM
  echo "Created .env"
else
  echo "Using existing .env"
fi

docker compose run -T --rm --no-deps agent-build
docker compose up -d --force-recreate --remove-orphans

console_url="$(env_value PUBLIC_WEB_URL)"
if [ -z "$console_url" ]; then
  saved_web_port="$(env_value WEB_PORT)"
  if [ -z "$saved_web_port" ]; then
    saved_web_port="$web_port"
  fi
  console_url="http://127.0.0.1:${saved_web_port}"
fi

cat <<EOF

Started the stack.

Console: ${console_url}

Create the first owner account in the browser. After owner setup, sign-up is disabled for this single-user edition.

Useful commands:
  cd "$install_dir"
  docker compose ps
  docker compose logs -f web control-plane
  ./node-agent --help
  ./monitor-agent --help
  docker compose down
  docker compose down -v --remove-orphans
EOF
