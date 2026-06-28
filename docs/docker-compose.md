# Docker Compose

This guide covers the Docker Compose installation created by `scripts/install.sh`. Release installs use prebuilt GHCR images and release binaries; they do not compile Go or npm assets on the target host.

## Configuration

The installer writes a local `.env` file on first run. Keep this file private because it contains authentication, database, and agent secrets. On later runs, the upgrade helper preserves secrets and custom values while updating the image tag used by Compose.

On first install, when a terminal is available and no configuration options are provided, the installer prompts for `WEB_PORT`, `PUBLIC_WEB_URL`, and `CONTROL_PLANE_PORT`. Pass explicit options such as `--web-port`, `--public-web-url`, and `--control-port` for unattended installs.

Common settings:

- `APP_NAME`: display name shown in the console.
- `WEB_PORT`: host port for the web console. Default: `3000`; the port is published on `0.0.0.0`.
- `CONTROL_PLANE_PORT`: host port for the control-plane API. Default: `8080`.
- `CONTROL_PLANE_BIND_HOST`: host interface for the control-plane API. Default: `0.0.0.0`.
- `PUBLIC_WEB_URL`: browser URL for the web console. Set this explicitly when automatic address detection does not match the URL you use in the browser.
- `CONTROL_PLANE_URL`: URL that agents use to reach the control plane. The installer derives this as `http://<PUBLIC_WEB_URL host>:<CONTROL_PLANE_PORT>` unless `--control-url` is provided.
- `DATABASE_URL`: PostgreSQL connection URL used by migrate, control-plane, and web.
- `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`: bundled PostgreSQL container settings. These are omitted when installing with `--database-url`.
- `AGENT_RELEASE_VERSION`: GitHub Release tag used by copied node install commands.
- `PRISM_IMAGE_REGISTRY`: image registry namespace. Default: `ghcr.io/noxaaa`.
- `PRISM_IMAGE_TAG`: image tag used for `prism-oss-web`, `prism-oss-control-plane`, and `prism-oss-migrate`.
- `BETTER_AUTH_URL`: optional auth base URL override. Defaults to `PUBLIC_WEB_URL`.
- `BETTER_AUTH_TRUSTED_ORIGINS`: comma-separated browser origins accepted by the auth service.
- `BETTER_AUTH_TRUST_PROXY_HEADERS`: set to `true` only when Prism is behind a trusted reverse proxy that strips incoming client-supplied `X-Forwarded-*` headers and rewrites them itself.
- `OSS_SETUP_TOKEN`: one-time first-owner setup token.

## Reverse Proxy

When the web console is placed behind an HTTPS reverse proxy, the auth service must see or trust the browser-facing origin. Set `PUBLIC_WEB_URL` and `BETTER_AUTH_URL` to the public console URL, and include that same origin in `BETTER_AUTH_TRUSTED_ORIGINS` when you keep additional localhost or IP origins.

Set `BETTER_AUTH_TRUST_PROXY_HEADERS=true` only for a trusted proxy boundary. Do not enable it when browsers can reach the web container directly or when the proxy passes through client-supplied `X-Forwarded-*` headers.

The proxy should forward the original host and scheme:

```nginx
proxy_set_header Host $http_host;
proxy_set_header X-Forwarded-Host $http_host;
proxy_set_header X-Forwarded-Proto $scheme;
proxy_set_header X-Forwarded-Port $server_port;
```

Use a host value that preserves a non-default public port, or forward the port separately with `X-Forwarded-Port`.

If these values point at the internal container URL instead of the public URL, BetterAuth can reject `/api/auth/sign-in/email` or `/api/auth/sign-out` with `INVALID_ORIGIN` or `MISSING_OR_NULL_ORIGIN`.

The default install starts a bundled `postgres:16` container and stores data in the `postgres-data` Docker volume. To use external PostgreSQL 16 instead, pass:

```sh
./scripts/install.sh --database-url "postgres://USER:PASSWORD@HOST:5432/DB?sslmode=require"
```

External PostgreSQL installs do not render a `postgres` service in `docker-compose.yml`.

SQLite has been removed. During indev, old local test instances must be purged and rebuilt; no SQLite-to-PostgreSQL data migration is provided.

## Start And Stop

```sh
docker compose up -d
docker compose ps
docker compose logs -f web control-plane postgres
docker compose down
```

Open the setup URL printed by the installer and create the first owner account. The setup URL includes `OSS_SETUP_TOKEN`; sign-up is rejected without that token until the first owner completes setup. After that account exists, this single-user edition disables further sign-ups.

## Upgrade

```sh
./upgrade.sh --version latest
```

The upgrade helper updates `PRISM_IMAGE_TAG` and `AGENT_RELEASE_VERSION`, pulls the selected release images, runs the `migrate` image, and restarts the Compose services. Existing secrets, custom trusted origins, custom `.env` values, and Docker volumes are preserved.

For a pinned upgrade, pass an explicit tag:

```sh
./upgrade.sh --version v0.1.3
```

## Backup

For the bundled PostgreSQL container:

```sh
docker compose exec -T postgres pg_dump -U "${POSTGRES_USER:-prism}" -d "${POSTGRES_DB:-prism}" --format=custom > prism.dump
```

For external PostgreSQL, use your database provider's managed backup workflow or run `pg_dump` against the external `DATABASE_URL`.

## Reset

Reset removes containers and local data volumes:

```sh
docker compose down -v --remove-orphans
```

The release uninstall helper exposes the same destructive reset as:

```sh
./uninstall.sh --purge
```

Run `./scripts/install.sh` again to recreate `.env` only if you removed it yourself. Existing secret values are not regenerated by the installer.
