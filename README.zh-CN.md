# 开源转发平台

[English](./README.md)

本仓库提供一个单用户的网络转发控制面和节点 Agent，用于集中管理 TCP/UDP 转发规则、节点、目标和基础指标。

## 功能

- 支持 TCP 和 UDP 转发，支持 Proxy Protocol。
- 节点 Agent 主动连接控制面，适合节点在 NAT 或防火墙后运行。
- 单用户初始化和本地授权。
- 支持目标、目标组、规则、导入导出、基础指标和审计记录。
- 仅支持 PostgreSQL，使用 goose migration，并拆分 `auth` 和 `app` schema。
- Next.js 控制台通过 `APP_NAME` 显示产品名称。

## 快速开始

Release 安装脚本只使用预构建的 GHCR 镜像和 Release 二进制文件，不会在目标机器上执行 Go 或 npm 编译。

使用 Docker Compose 安装 OSS 控制面：

```sh
curl -fsSL https://github.com/noxaaa/prism-oss/releases/latest/download/install.sh | bash
```

安装脚本会写入本地 `.env`，生成基于镜像的 `docker-compose.yml`，拉取指定 Release 镜像，运行 `migrate` 镜像，并启动 PostgreSQL 16、Redis、控制面和 Web 控制台。安装完成后打开脚本输出的 setup URL 创建第一个 owner 账号。

如果安装在远程主机上，自动识别的地址不是浏览器可访问地址时，显式传入外部访问地址：

```sh
curl -fsSL https://github.com/noxaaa/prism-oss/releases/latest/download/install.sh -o install.sh
sh ./install.sh --public-web-url http://YOUR_SERVER_IP:3000 --control-url http://YOUR_SERVER_IP:8080
```

安装指定版本：

```sh
curl -fsSL https://github.com/noxaaa/prism-oss/releases/download/vX.Y.Z/install.sh -o install.sh
sh ./install.sh --version vX.Y.Z
```

常用选项：

```sh
sh ./install.sh \
  --version vX.Y.Z \
  --dir "$HOME/prism-oss" \
  --app-name "OSS Control Console" \
  --web-port 3000 \
  --public-web-url http://YOUR_SERVER_IP:3000 \
  --control-port 8080 \
  --control-bind-host 0.0.0.0 \
  --control-url http://YOUR_SERVER_IP:8080
```

使用外部 PostgreSQL 16，而不是内置 PostgreSQL 容器：

```sh
sh ./install.sh --database-url "postgres://USER:PASSWORD@HOST:5432/DB?sslmode=require"
```

当前 indev 阶段已经移除 SQLite。已有测试环境需要执行 `./uninstall.sh --purge` 后重新安装，不提供 SQLite 数据升级路径。

## 升级控制面

进入安装目录后执行：

```sh
cd "$HOME/prism-oss"
./upgrade.sh --version latest
./upgrade.sh --version vX.Y.Z
```

升级脚本会更新 `.env` 中的镜像 tag 和 Agent Release 版本，拉取新镜像，运行迁移，并重启 Compose 服务。已有密钥、自定义环境变量和 Docker volumes 会保留。

## 卸载控制面

进入安装目录后执行：

```sh
cd "$HOME/prism-oss"
./uninstall.sh
```

默认卸载会停止并移除 Compose containers，同时保留 `.env`、`docker-compose.yml` 和 Docker volumes。

如果要删除本地生成文件和数据 volumes：

```sh
./uninstall.sh --purge
```

`--purge` 会删除数据，执行前请确认已经备份。

## 安装节点 Agent

节点 Agent 目前面向 Linux systemd 环境。推荐直接复制控制台生成的节点注册命令，也可以在节点机器上以 root 运行 Release helper：

```sh
(tmp=$(mktemp) && curl -fsSL https://github.com/noxaaa/prism-oss/releases/latest/download/install-node-agent.sh -o "$tmp" && sudo env APP_NAME='OSS Control Console' sh "$tmp" --version latest --control-url http://YOUR_CONTROL_PLANE:8080 --registration-token YOUR_NODE_REGISTRATION_TOKEN; status=$?; rm -f "${tmp:-}"; exit "$status")
```

helper 会下载 `node-agent-linux-<arch>.tar.gz`，校验 `SHA256SUMS`，调用 `node-agent install`，注册并启动 `prism-node-agent.service`。命令退出后，Agent 会由 systemd 在后台运行。

手动升级节点 Agent：

```sh
sudo /opt/prism-node-agent/current/node-agent upgrade --version vX.Y.Z
```

也可以重新运行安装 helper 并指定版本和当前节点注册 token：

```sh
(tmp=$(mktemp) && curl -fsSL https://github.com/noxaaa/prism-oss/releases/download/vX.Y.Z/install-node-agent.sh -o "$tmp" && sudo env APP_NAME='OSS Control Console' sh "$tmp" --version vX.Y.Z --control-url http://YOUR_CONTROL_PLANE:8080 --registration-token YOUR_NODE_REGISTRATION_TOKEN; status=$?; rm -f "${tmp:-}"; exit "$status")
```

卸载节点 Agent service：

```sh
(tmp=$(mktemp) && curl -fsSL https://github.com/noxaaa/prism-oss/releases/latest/download/uninstall-node-agent.sh -o "$tmp" && sudo sh "$tmp"; status=$?; rm -f "${tmp:-}"; exit "$status")
```

默认会保留 Agent 配置和 credential。需要一并删除时加 `--purge`：

```sh
(tmp=$(mktemp) && curl -fsSL https://github.com/noxaaa/prism-oss/releases/latest/download/uninstall-node-agent.sh -o "$tmp" && sudo sh "$tmp" --purge; status=$?; rm -f "${tmp:-}"; exit "$status")
```

## 安装 Monitor Agent

创建 Monitor 后，可以复制控制台生成的注册命令，也可以在监控节点上以 root 运行 Release helper：

```sh
(tmp=$(mktemp) && curl -fsSL https://github.com/noxaaa/prism-oss/releases/latest/download/install-monitor-agent.sh -o "$tmp" && sudo env APP_NAME='OSS Control Console' sh "$tmp" --version latest --control-url http://YOUR_CONTROL_PLANE:8080 --registration-token YOUR_MONITOR_REGISTRATION_TOKEN; status=$?; rm -f "${tmp:-}"; exit "$status")
```

helper 会下载 `monitor-agent-linux-<arch>.tar.gz`，校验 `SHA256SUMS`，调用 `monitor-agent install`，注册并启动 `prism-monitor-agent.service`。卸载 Monitor Agent service：

```sh
(tmp=$(mktemp) && curl -fsSL https://github.com/noxaaa/prism-oss/releases/latest/download/uninstall-monitor-agent.sh -o "$tmp" && sudo sh "$tmp"; status=$?; rm -f "${tmp:-}"; exit "$status")
```

升级 Monitor Agent 时重新运行 install helper 并指定目标 `--version` 即可。如果同一台主机要对接多个控制面，可以用 `--service-name`、`--install-dir`、`--config-file` 和 `--credential-file` 隔离服务与状态。

主动健康检查和 DNS failover 依赖 Monitor Agent。Compose 安装器会自动生成 `DNS_SECRET_ENCRYPTION_KEY`；自定义部署需要在控制面配置稳定的 32 字节随机密钥，重启后必须保持不变，否则已加密的 DNS provider token 无法解密。

## Docker Compose 运维

配置、升级、备份、日志和重置步骤见 [Docker Compose 运维](./docs/docker-compose.zh-CN.md)。

## 本地开发

依赖：

- Go 1.24
- Node.js 22
- npm

安装依赖并运行默认检查：

```sh
npm ci
go test ./...
npm --workspace apps/web test
NEXT_PUBLIC_PRISM_EDITION=oss npm --workspace apps/web run build
```

Go module path 是 `github.com/noxaaa/prism-oss`。

## 许可证

本项目使用 AGPL-3.0。完整许可证见 [LICENSE](./LICENSE)。
