# Open-source forwarding platform

This repository contains a single-user forwarding control plane and agent stack for managing network forwarding rules.

## Features

- TCP and UDP forwarding with Proxy Protocol support.
- Node and monitor agents with outbound control-plane connections.
- Single-user account setup and local authorization.
- Targets, target groups, rules, import/export, basic metrics, and audit records.
- Core goose migrations only.
- A Next.js control console that uses `APP_NAME` for display text.

## Quick Start

The easiest way to run the stack is Docker Compose:

```sh
curl -fsSL https://github.com/noxaaa/prism-oss/releases/latest/download/install.sh | bash
```

The installer downloads the latest release archive, writes a local `.env` file when one does not exist, and starts the web console, control plane, migration job, and Redis with the included `docker-compose.yml`.

Open the console at `http://127.0.0.1:3000` and create the first owner account. This single-user edition disables further sign-ups after the first owner setup.

Pinned release flow:

```sh
curl -fsSL https://github.com/noxaaa/prism-oss/releases/download/v0.1.0/install.sh -o install.sh
sh ./install.sh --version v0.1.0
```

Useful options:

```sh
./scripts/install.sh --version v0.1.0 --dir "$HOME/prism-oss" --app-name "OSS Control Console" --web-port 3000 --control-port 8080 --control-bind-host 127.0.0.1 --control-url http://127.0.0.1:8080
```

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
