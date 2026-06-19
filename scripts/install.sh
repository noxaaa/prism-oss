#!/usr/bin/env sh
set -eu

release_repo="https://github.com/noxaaa/prism-oss/releases"
install_dir="${HOME}/prism-oss"
app_name="OSS Control Console"
app_name_provided=0
web_port="3000"
web_port_provided=0
public_web_url=""
public_web_url_provided=0
control_port="8080"
control_port_provided=0
control_bind_host="0.0.0.0"
control_bind_host_provided=0
control_url=""
control_url_provided=0
image_registry="ghcr.io/noxaaa"
image_registry_provided=0
version="latest"

usage() {
  cat <<'USAGE'
Usage: install.sh [options]

Options:
  --version VERSION       Release tag to install. Defaults to latest.
  --dir DIR               Installation directory. Defaults to $HOME/prism-oss.
  --app-name NAME         Console display name. Defaults to "OSS Control Console".
  --web-port PORT         Host port for the web console. Defaults to 3000.
  --public-web-url URL    Browser URL for the web console.
  --control-port PORT     Host port for the control-plane API. Defaults to 8080.
  --control-bind-host HOST
                           Host interface for the control-plane API. Defaults to 0.0.0.0.
  --control-url URL        URL that node agents use to reach the control plane.
  --image-registry HOST    Image registry namespace. Defaults to ghcr.io/noxaaa.
  -h, --help              Show this help.
USAGE
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version) version="${2:?missing value for --version}"; shift 2 ;;
    --dir) install_dir="${2:?missing value for --dir}"; shift 2 ;;
    --app-name) app_name="${2:?missing value for --app-name}"; app_name_provided=1; shift 2 ;;
    --web-port) web_port="${2:?missing value for --web-port}"; web_port_provided=1; shift 2 ;;
    --public-web-url) public_web_url="${2:?missing value for --public-web-url}"; public_web_url_provided=1; shift 2 ;;
    --control-port) control_port="${2:?missing value for --control-port}"; control_port_provided=1; shift 2 ;;
    --control-bind-host) control_bind_host="${2:?missing value for --control-bind-host}"; control_bind_host_provided=1; shift 2 ;;
    --control-url) control_url="${2:?missing value for --control-url}"; control_url_provided=1; shift 2 ;;
    --image-registry) image_registry="${2:?missing value for --image-registry}"; image_registry_provided=1; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

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

generate_url_safe_secret() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 48 | tr -d '\n'
    return
  fi
  if command -v python3 >/dev/null 2>&1; then
    python3 -c 'import secrets; print(secrets.token_urlsafe(48))'
    return
  fi
  echo "openssl or python3 is required to generate secrets" >&2
  exit 1
}

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
  printf 'latest'
}

first_non_loopback_host() {
  if command -v ip >/dev/null 2>&1; then
    address="$(ip route get 1.1.1.1 2>/dev/null | sed -n 's/.* src \([0-9.]*\).*/\1/p' | head -n 1)"
    if [ -n "$address" ]; then
      printf '%s' "$address"
      return
    fi
  fi
  if command -v hostname >/dev/null 2>&1; then
    for address in $(hostname -I 2>/dev/null || true); do
      case "$address" in
        127.*|0.0.0.0|::1|fe80:*|"") ;;
        *:*) ;;
        *) printf '%s' "$address"; return ;;
      esac
    done
  fi
  printf '127.0.0.1'
}

public_url() {
  if [ -n "$public_web_url" ]; then
    printf '%s' "$public_web_url"
  else
    printf 'http://%s:%s' "$(first_non_loopback_host)" "$web_port"
  fi
}

agent_control_url() {
  if [ -n "$control_url" ]; then
    printf '%s' "$control_url"
    return
  fi
  case "$control_bind_host" in
    127.*|localhost|::1|\[::1\])
      printf 'http://127.0.0.1:%s' "$control_port"
      return
      ;;
  esac
  web_url="$1"
  rest="${web_url#*://}"
  host_port="${rest%%/*}"
  host_port="${host_port##*@}"
  case "$host_port" in
    \[*\]*)
      host="${host_port%%]*}"
      host="${host}]"
      ;;
    *)
      host="${host_port%%:*}"
      ;;
  esac
  [ -n "$host" ] || host="$(first_non_loopback_host)"
  printf 'http://%s:%s' "$host" "$control_port"
}

query_encode() {
  if command -v python3 >/dev/null 2>&1; then
    VALUE="$1" python3 -c 'import os, urllib.parse; print(urllib.parse.quote(os.environ["VALUE"], safe=""))'
    return
  fi
  printf '%s' "$1" | sed -e 's/%/%25/g' -e 's/+/%2B/g' -e 's/\//%2F/g' -e 's/=/%3D/g' -e 's/ /%20/g' -e 's/?/%3F/g' -e 's/&/%26/g' -e 's/#/%23/g'
}

env_value() {
  key="$1"
  if [ -f ".env" ]; then
    sed -n "s/^${key}=//p" ".env" | tail -n 1
  fi
}

set_env_value() {
  key="$1"
  value="$2"
  tmp_env=".env.tmp.$$"
  if [ -f ".env" ] && grep -q "^${key}=" ".env"; then
    awk -v key="$key" -v value="$value" 'BEGIN { prefix = key "=" } index($0, prefix) == 1 { print key "=" value; next } { print }' ".env" > "$tmp_env"
  else
    [ -f ".env" ] && cat ".env" > "$tmp_env" || : > "$tmp_env"
    printf '%s=%s\n' "$key" "$value" >> "$tmp_env"
  fi
  mv "$tmp_env" ".env"
  chmod 600 ".env"
}

env_key_exists() {
  [ -f ".env" ] && grep -q "^${1}=" ".env"
}

set_env_value_if_requested_or_missing() {
  key="$1"
  value="$2"
  requested="$3"
  if [ "$requested" = "1" ] || ! env_key_exists "$key"; then
    set_env_value "$key" "$value"
  fi
}

write_compose() {
  cat > docker-compose.yml <<'YAML'
services:
  redis:
    image: redis:7-alpine
    command: ["redis-server", "--appendonly", "yes"]
    volumes:
      - redis-data:/data

  sqlite-permissions:
    image: busybox:1.36
    command: ["sh", "-c", "chown -R 65532:65532 /data"]
    volumes:
      - sqlite-data:/data

  migrate:
    image: ${PRISM_IMAGE_REGISTRY:-ghcr.io/noxaaa}/prism-oss-migrate:${PRISM_IMAGE_TAG:-latest}
    depends_on:
      sqlite-permissions:
        condition: service_completed_successfully
    command: ["up"]
    environment:
      DATABASE_URL: ${PRISM_OSS_DATABASE_URL:-/data/oss.db}
      PRISM_EDITION: oss
    volumes:
      - sqlite-data:/data

  control-plane:
    image: ${PRISM_IMAGE_REGISTRY:-ghcr.io/noxaaa}/prism-oss-control-plane:${PRISM_IMAGE_TAG:-latest}
    depends_on:
      migrate:
        condition: service_completed_successfully
      redis:
        condition: service_started
    environment:
      APP_NAME: ${APP_NAME:-OSS Control Console}
      APP_ENV: ${APP_ENV:-production}
      PRISM_EDITION: oss
      PUBLIC_WEB_URL: ${PUBLIC_WEB_URL:-http://127.0.0.1:3000}
      CONTROL_PLANE_URL: ${CONTROL_PLANE_URL:-http://127.0.0.1:8080}
      CONTROL_PLANE_INTERNAL_URL: ${CONTROL_PLANE_INTERNAL_URL:-http://control-plane:8080}
      AGENT_RELEASE_VERSION: ${AGENT_RELEASE_VERSION:-latest}
      CONTROL_PLANE_INTERNAL_JWT_SECRET: ${CONTROL_PLANE_INTERNAL_JWT_SECRET:?set CONTROL_PLANE_INTERNAL_JWT_SECRET in .env}
      AGENT_TOKEN_SIGNING_SECRET: ${AGENT_TOKEN_SIGNING_SECRET:?set AGENT_TOKEN_SIGNING_SECRET in .env}
      DATABASE_URL: ${PRISM_OSS_DATABASE_URL:-/data/oss.db}
      QUEUE_REDIS_URL: ${QUEUE_REDIS_URL:-redis://redis:6379/0}
      CACHE_REDIS_URL: ${CACHE_REDIS_URL:-redis://redis:6379/0}
      CONTROL_PLANE_HTTP_ADDR: 0.0.0.0:8080
    ports:
      - "${CONTROL_PLANE_BIND_HOST:-0.0.0.0}:${CONTROL_PLANE_PORT:-8080}:8080"
    volumes:
      - sqlite-data:/data

  web:
    image: ${PRISM_IMAGE_REGISTRY:-ghcr.io/noxaaa}/prism-oss-web:${PRISM_IMAGE_TAG:-latest}
    depends_on:
      control-plane:
        condition: service_started
    environment:
      APP_NAME: ${APP_NAME:-OSS Control Console}
      BETTER_AUTH_SECRET: ${BETTER_AUTH_SECRET:?set BETTER_AUTH_SECRET in .env}
      BETTER_AUTH_URL: ${BETTER_AUTH_URL:-${PUBLIC_WEB_URL:-http://127.0.0.1:3000}}
      BETTER_AUTH_TRUSTED_ORIGINS: ${BETTER_AUTH_TRUSTED_ORIGINS:-${PUBLIC_WEB_URL:-http://127.0.0.1:3000},http://127.0.0.1:${WEB_PORT:-3000},http://localhost:${WEB_PORT:-3000}}
      OSS_SETUP_TOKEN: ${OSS_SETUP_TOKEN:?set OSS_SETUP_TOKEN in .env}
      DATABASE_URL: ${PRISM_OSS_DATABASE_URL:-/data/oss.db}
      CONTROL_PLANE_INTERNAL_URL: ${CONTROL_PLANE_INTERNAL_URL:-http://control-plane:8080}
      CONTROL_PLANE_INTERNAL_JWT_SECRET: ${CONTROL_PLANE_INTERNAL_JWT_SECRET:?set CONTROL_PLANE_INTERNAL_JWT_SECRET in .env}
      NEXT_PUBLIC_PRISM_EDITION: oss
    ports:
      - "0.0.0.0:${WEB_PORT:-3000}:3000"
    volumes:
      - sqlite-data:/data

volumes:
  redis-data:
  sqlite-data:
YAML
}

write_upgrade_script() {
  cat > upgrade.sh <<'SH'
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
SH
  chmod +x upgrade.sh
}

is_prism_install_dir() {
  env_file="$1/.env"
  [ -f "$env_file" ] || return 1
  grep -Eq '^(PRISM_EDITION=oss|PRISM_IMAGE_TAG=|AGENT_RELEASE_VERSION=|OSS_SETUP_TOKEN=|AGENT_TOKEN_SIGNING_SECRET=|CONTROL_PLANE_INTERNAL_JWT_SECRET=)' "$env_file"
}

validate_install_dir() {
  if [ ! -e "$install_dir" ]; then
    return
  fi
  if [ ! -d "$install_dir" ]; then
    echo "$install_dir exists but is not a directory" >&2
    exit 1
  fi
  if [ -z "$(ls -A "$install_dir" 2>/dev/null)" ]; then
    return
  fi
  if is_prism_install_dir "$install_dir"; then
    return
  fi
  echo "$install_dir is not empty and does not look like a prism-oss install directory" >&2
  exit 1
}

validate_install_dir

require_command curl
require_command docker
if ! docker compose version >/dev/null 2>&1; then
  echo "docker compose v2 is required" >&2
  exit 1
fi

resolved_version="$(resolve_release_version)"
mkdir -p "$install_dir"
cd "$install_dir"
write_compose
write_upgrade_script

if [ -f ".env" ]; then
  if [ "$web_port_provided" = "0" ]; then
    existing_web_port="$(env_value WEB_PORT)"
    [ -n "$existing_web_port" ] && web_port="$existing_web_port"
  fi
  if [ "$control_port_provided" = "0" ]; then
    existing_control_port="$(env_value CONTROL_PLANE_PORT)"
    [ -n "$existing_control_port" ] && control_port="$existing_control_port"
  fi
  if [ "$control_bind_host_provided" = "0" ]; then
    existing_control_bind_host="$(env_value CONTROL_PLANE_BIND_HOST)"
    [ -n "$existing_control_bind_host" ] && control_bind_host="$existing_control_bind_host"
  fi
fi

resolved_public_url="$(public_url)"
if [ -f ".env" ] && [ "$public_web_url_provided" = "0" ]; then
  existing_public_url="$(env_value PUBLIC_WEB_URL)"
  [ -n "$existing_public_url" ] && resolved_public_url="$existing_public_url"
fi

resolved_control_url="$(agent_control_url "$resolved_public_url")"
control_url_requested=0
if [ "$control_url_provided" = "1" ] || [ "$control_port_provided" = "1" ] || [ "$control_bind_host_provided" = "1" ] || [ "$public_web_url_provided" = "1" ]; then
  control_url_requested=1
fi
if [ -f ".env" ] && [ "$control_url_requested" = "0" ]; then
  existing_control_url="$(env_value CONTROL_PLANE_URL)"
  [ -n "$existing_control_url" ] && resolved_control_url="$existing_control_url"
fi

trusted_origins="${resolved_public_url},http://127.0.0.1:${web_port},http://localhost:${web_port}"
trusted_origins_requested=0
if [ "$public_web_url_provided" = "1" ] || [ "$web_port_provided" = "1" ]; then
  trusted_origins_requested=1
fi

if [ ! -f ".env" ]; then
  umask 077
  {
    printf 'APP_NAME=%s\n' "$app_name"
    printf 'APP_ENV=production\n'
    printf 'PRISM_EDITION=oss\n'
    printf 'PRISM_IMAGE_REGISTRY=%s\n' "$image_registry"
    printf 'PRISM_IMAGE_TAG=%s\n' "$resolved_version"
    printf 'AGENT_RELEASE_VERSION=%s\n' "$resolved_version"
    printf 'WEB_PORT=%s\n' "$web_port"
    printf 'CONTROL_PLANE_PORT=%s\n' "$control_port"
    printf 'CONTROL_PLANE_BIND_HOST=%s\n' "$control_bind_host"
    printf 'PUBLIC_WEB_URL=%s\n' "$resolved_public_url"
    printf 'CONTROL_PLANE_URL=%s\n' "$resolved_control_url"
    printf 'CONTROL_PLANE_INTERNAL_URL=http://control-plane:8080\n'
    printf 'PRISM_OSS_DATABASE_URL=/data/oss.db\n'
    printf 'QUEUE_REDIS_URL=redis://redis:6379/0\n'
    printf 'CACHE_REDIS_URL=redis://redis:6379/0\n'
    printf 'BETTER_AUTH_SECRET=%s\n' "$(generate_secret)"
    printf 'BETTER_AUTH_URL=%s\n' "$resolved_public_url"
    printf 'BETTER_AUTH_TRUSTED_ORIGINS=%s\n' "$trusted_origins"
    printf 'OSS_SETUP_TOKEN=%s\n' "$(generate_url_safe_secret)"
    printf 'CONTROL_PLANE_INTERNAL_JWT_SECRET=%s\n' "$(generate_secret)"
    printf 'AGENT_TOKEN_SIGNING_SECRET=%s\n' "$(generate_secret)"
  } > .env
  chmod 600 .env
  echo "Created .env"
else
  echo "Using existing .env"
  set_env_value_if_requested_or_missing APP_NAME "$app_name" "$app_name_provided"
  set_env_value_if_requested_or_missing PRISM_IMAGE_REGISTRY "$image_registry" "$image_registry_provided"
  set_env_value PRISM_IMAGE_TAG "$resolved_version"
  set_env_value AGENT_RELEASE_VERSION "$resolved_version"
  set_env_value_if_requested_or_missing WEB_PORT "$web_port" "$web_port_provided"
  set_env_value_if_requested_or_missing CONTROL_PLANE_PORT "$control_port" "$control_port_provided"
  set_env_value_if_requested_or_missing CONTROL_PLANE_BIND_HOST "$control_bind_host" "$control_bind_host_provided"
  set_env_value_if_requested_or_missing PUBLIC_WEB_URL "$resolved_public_url" "$public_web_url_provided"
  set_env_value_if_requested_or_missing CONTROL_PLANE_URL "$resolved_control_url" "$control_url_requested"
  set_env_value_if_requested_or_missing BETTER_AUTH_URL "$resolved_public_url" "$public_web_url_provided"
  set_env_value_if_requested_or_missing BETTER_AUTH_TRUSTED_ORIGINS "$trusted_origins" "$trusted_origins_requested"
fi

docker compose pull
docker compose run --rm sqlite-permissions
docker compose run --rm migrate up
docker compose up -d --remove-orphans

console_url="$(env_value PUBLIC_WEB_URL)"
[ -n "$console_url" ] || console_url="$resolved_public_url"
setup_token="$(env_value OSS_SETUP_TOKEN)"
setup_url="$console_url"
if [ -n "$setup_token" ]; then
  case "$setup_url" in
    *\?*) setup_url="${setup_url}&setup_token=$(query_encode "$setup_token")" ;;
    *) setup_url="${setup_url}?setup_token=$(query_encode "$setup_token")" ;;
  esac
fi

cat <<EOF

Started the stack from prebuilt release assets.

Console: ${console_url}
Setup URL: ${setup_url}

Useful commands:
  cd "$install_dir"
  ./upgrade.sh --version latest
  docker compose ps
  docker compose logs -f web control-plane
  docker compose down
EOF
