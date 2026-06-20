# Open-source forwarding platform

This repository contains a single-user forwarding control plane and agent stack for managing network forwarding rules.

## Features

- TCP and UDP forwarding with Proxy Protocol support.
- Node agents with outbound control-plane connections.
- Single-user account setup and local authorization.
- Targets, target groups, rules, import/export, basic metrics, and audit records.
- PostgreSQL-only goose migrations with separate `auth` and `app` schemas.
- A Next.js control console that uses `APP_NAME` for display text.

## Quick Start

The release installer uses prebuilt GHCR images and release binaries. It does not run Go or npm builds on the target host.

Install the OSS control plane with Docker Compose:

```sh
curl -fsSL https://github.com/noxaaa/prism-oss/releases/latest/download/install.sh | bash
```

The installer writes a local `.env`, writes an image-based `docker-compose.yml`, pulls the selected release images, runs the `migrate` image, and starts PostgreSQL 16, Redis, the control plane, and the web console. Open the setup URL printed by the installer. On a remote host, pass `--public-web-url http://YOUR_SERVER_IP:3000` and `--control-url http://YOUR_SERVER_IP:8080` when automatic address detection cannot infer reachable URLs.

Pinned release flow:

```sh
curl -fsSL https://github.com/noxaaa/prism-oss/releases/download/v0.1.3/install.sh -o install.sh
sh ./install.sh --version v0.1.3
```

Useful options:

```sh
./scripts/install.sh --version v0.1.3 --dir "$HOME/prism-oss" --app-name "OSS Control Console" --web-port 3000 --public-web-url http://YOUR_SERVER_IP:3000 --control-port 8080 --control-bind-host 0.0.0.0 --control-url http://YOUR_SERVER_IP:8080
```

Use an external PostgreSQL 16 database instead of the bundled container:

```sh
./scripts/install.sh --database-url "postgres://USER:PASSWORD@HOST:5432/DB?sslmode=require"
```

SQLite has been removed during indev. Existing test installs must be rebuilt with `./uninstall.sh --purge` and reinstalled; no SQLite data upgrade path is provided.

Upgrade an installed control plane from the install directory:

```sh
cd "$HOME/prism-oss"
./upgrade.sh --version latest
./upgrade.sh --version v0.1.9
```

Uninstall the control plane from the install directory:

```sh
cd "$HOME/prism-oss"
./uninstall.sh
./uninstall.sh --purge
```

The default control-plane uninstall stops and removes Compose containers while preserving `.env`, `docker-compose.yml`, and Docker volumes. Add `--purge` only when you also want to remove generated local install files and data volumes.

Install a node Agent as a Linux systemd service. Use the copied registration token command from the console, or run the release helper directly as root:

```sh
(tmp=$(mktemp) && curl -fsSL https://github.com/noxaaa/prism-oss/releases/latest/download/install-node-agent.sh -o "$tmp" && sudo env APP_NAME='OSS Control Console' sh "$tmp" --version latest --control-url http://YOUR_CONTROL_PLANE:8080 --registration-token YOUR_NODE_REGISTRATION_TOKEN; status=$?; rm -f "${tmp:-}"; exit "$status")
```

The helper downloads `node-agent-linux-<arch>.tar.gz`, verifies `SHA256SUMS`, calls `node-agent install`, registers `prism-node-agent.service`, and exits. The Agent then runs in the background under systemd.

Manually upgrade a node Agent over SSH:

```sh
sudo /opt/prism-node-agent/current/node-agent upgrade --version v0.1.9
```

You can also rerun the install helper with a target release tag and the current node registration token:

```sh
(tmp=$(mktemp) && curl -fsSL https://github.com/noxaaa/prism-oss/releases/download/v0.1.9/install-node-agent.sh -o "$tmp" && sudo env APP_NAME='OSS Control Console' sh "$tmp" --version v0.1.9 --control-url http://YOUR_CONTROL_PLANE:8080 --registration-token YOUR_NODE_REGISTRATION_TOKEN; status=$?; rm -f "${tmp:-}"; exit "$status")
```

Uninstall the node Agent service:

```sh
(tmp=$(mktemp) && curl -fsSL https://github.com/noxaaa/prism-oss/releases/latest/download/uninstall-node-agent.sh -o "$tmp" && sudo sh "$tmp"; status=$?; rm -f "${tmp:-}"; exit "$status")
```

Add `--purge` to remove the Agent config and credential state as well.

See [Docker Compose operations](./docs/docker-compose.md) for configuration, upgrades, backups, logs, and reset steps.

## Local Development

Prerequisites:

- Go 1.24
- Node.js 22
- npm

Install dependencies and run the default checks:

```sh
npm ci
go test ./...
npm --workspace apps/web test
NEXT_PUBLIC_PRISM_EDITION=oss npm --workspace apps/web run build
```

The Go module path is `github.com/noxaaa/prism-oss`.

## License

This project is licensed under AGPL-3.0. See `LICENSE` for the full license text.
