# Docker Compose

This guide covers the Docker Compose installation created by `scripts/install.sh`.

## Configuration

The installer writes a local `.env` file on first run. Keep this file private because it contains authentication and agent secrets.

Common settings:

- `APP_NAME`: display name shown in the console.
- `WEB_PORT`: host port for the web console. Default: `3000`.
- `CONTROL_PLANE_PORT`: host port for the control-plane API. Default: `8080`.
- `CONTROL_PLANE_BIND_HOST`: host interface for the control-plane API. Default: `127.0.0.1`; use `0.0.0.0` when agents on other hosts must connect.
- `PUBLIC_WEB_URL`: browser URL for the web console.
- `CONTROL_PLANE_URL`: URL that agents use to reach the control plane. When `CONTROL_PLANE_BIND_HOST=0.0.0.0`, set this to a reachable host or DNS name, not localhost.
- `BETTER_AUTH_URL`: optional auth base URL override. Defaults to `PUBLIC_WEB_URL`.
- `PRISM_OSS_DATABASE_URL`: SQLite database path inside the containers. Default: `/data/oss.db`.

Do not edit generated secrets after the first start unless you are intentionally rotating credentials.

## Start And Stop

```sh
docker compose up -d
docker compose ps
docker compose logs -f web control-plane
docker compose down
```

Open the web console at the `PUBLIC_WEB_URL` value and create the first owner account. After that account exists, this single-user edition disables further sign-ups.

The installer builds `./node-agent` and `./monitor-agent` in the repository root before starting the stack. Those binaries are used by the agent install commands shown in the console.

## Upgrade

```sh
./scripts/install.sh --version latest
```

The installer downloads the selected release archive, replaces source-managed files, rebuilds agent binaries, and runs `docker compose up -d --force-recreate --remove-orphans`. The `migrate` service runs core migrations before the control plane starts. Existing `.env` files and Docker volumes are preserved.

For a pinned upgrade, pass an explicit tag:

```sh
./scripts/install.sh --version v0.1.0
```

## Backup

The SQLite database lives in the `sqlite-data` Docker volume. To copy it out:

```sh
docker compose stop web control-plane
docker compose run --rm -v "$PWD:/backup" migrate sh -lc 'cp /data/oss.db /backup/oss.db.backup && for suffix in -wal -shm; do if [ -f "/data/oss.db${suffix}" ]; then cp "/data/oss.db${suffix}" "/backup/oss.db.backup${suffix}"; fi; done'
docker compose up -d
```

Keep backups private. They contain user, rule, target, node, audit, and token metadata. The command copies `/data/oss.db`, `/data/oss.db-wal`, and `/data/oss.db-shm` when the WAL files are present; keep `oss.db.backup`, `oss.db.backup-wal`, and `oss.db.backup-shm` together because SQLite WAL mode can store committed data in the WAL file.

## Reset

Reset removes containers and local data volumes:

```sh
docker compose down -v --remove-orphans
```

Run `./scripts/install.sh` again to recreate `.env` only if you removed it yourself. Existing `.env` files are never overwritten by the installer.
