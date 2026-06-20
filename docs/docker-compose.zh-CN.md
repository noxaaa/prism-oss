# Docker Compose 运维

[English](./docker-compose.md)

本文档说明由 `scripts/install.sh` 创建的 Docker Compose 安装。Release 安装使用预构建 GHCR 镜像和 Release 二进制文件，不会在目标机器上编译 Go 或 npm 资源。

## 配置

安装脚本第一次运行时会写入本地 `.env` 文件。这个文件包含认证、数据库和 Agent 相关密钥，应当视为私密文件保存。后续升级会保留已有密钥和自定义配置，只更新 Compose 使用的镜像 tag。

常用配置项：

- `APP_NAME`：控制台显示名称。
- `WEB_PORT`：Web 控制台暴露到主机的端口。默认 `3000`，监听 `0.0.0.0`。
- `CONTROL_PLANE_PORT`：控制面 API 暴露到主机的端口。默认 `8080`。
- `CONTROL_PLANE_BIND_HOST`：控制面 API 绑定的主机网卡地址。默认 `0.0.0.0`。
- `PUBLIC_WEB_URL`：浏览器访问 Web 控制台的 URL。自动识别结果不符合实际访问地址时应显式设置。
- `CONTROL_PLANE_URL`：节点 Agent 访问控制面的 URL。未传 `--control-url` 时，安装脚本会根据 `PUBLIC_WEB_URL` 和 `CONTROL_PLANE_PORT` 推导。
- `DATABASE_URL`：migrate、control-plane 和 web 共用的 PostgreSQL 连接 URL。
- `POSTGRES_DB`、`POSTGRES_USER`、`POSTGRES_PASSWORD`：内置 PostgreSQL 容器配置。使用 `--database-url` 时不会生成这些内置容器配置。
- `AGENT_RELEASE_VERSION`：控制台复制的节点安装命令使用的 GitHub Release tag。
- `PRISM_IMAGE_REGISTRY`：镜像 registry namespace。默认 `ghcr.io/noxaaa`。
- `PRISM_IMAGE_TAG`：`prism-oss-web`、`prism-oss-control-plane` 和 `prism-oss-migrate` 使用的镜像 tag。
- `BETTER_AUTH_URL`：可选的 auth base URL。默认使用 `PUBLIC_WEB_URL`。
- `BETTER_AUTH_TRUSTED_ORIGINS`：auth 服务接受的浏览器来源，多个来源用逗号分隔。
- `OSS_SETUP_TOKEN`：首次创建 owner 账号的一次性 setup token。

默认安装会启动内置 `postgres:16` 容器，数据存储在 `postgres-data` Docker volume。使用外部 PostgreSQL 16 时执行：

```sh
./scripts/install.sh --database-url "postgres://USER:PASSWORD@HOST:5432/DB?sslmode=require"
```

使用外部 PostgreSQL 时，生成的 `docker-compose.yml` 不会包含 `postgres` service。

SQLite 已经移除。当前仍处于 indev 阶段，旧的本地测试实例需要 purge 后重建，不提供 SQLite 到 PostgreSQL 的数据迁移。

## 启动和停止

```sh
docker compose up -d
docker compose ps
docker compose logs -f web control-plane postgres
docker compose down
```

安装完成后打开安装脚本输出的 setup URL 创建第一个 owner 账号。setup URL 包含 `OSS_SETUP_TOKEN`；第一个 owner 创建完成前，没有 token 的注册会被拒绝。第一个账号创建完成后，单用户版本会禁用后续注册。

## 升级

```sh
./upgrade.sh --version latest
```

升级脚本会更新 `PRISM_IMAGE_TAG` 和 `AGENT_RELEASE_VERSION`，拉取指定 Release 镜像，运行 `migrate` 镜像，并重启 Compose 服务。已有密钥、自定义 trusted origins、自定义 `.env` 值和 Docker volumes 都会保留。

升级到指定版本：

```sh
./upgrade.sh --version vX.Y.Z
```

## 备份

使用内置 PostgreSQL 容器时：

```sh
docker compose exec -T postgres pg_dump -U "${POSTGRES_USER:-prism}" -d "${POSTGRES_DB:-prism}" --format=custom > prism.dump
```

使用外部 PostgreSQL 时，优先使用数据库提供商的托管备份流程，也可以针对外部 `DATABASE_URL` 执行 `pg_dump`。

## 日志和排查

查看服务状态：

```sh
docker compose ps
```

查看控制台、控制面和数据库日志：

```sh
docker compose logs -f web control-plane postgres
```

只查看控制面日志：

```sh
docker compose logs -f control-plane
```

重新运行迁移应通过 migrate image 或升级脚本完成，不要手动执行 SQL migration 文件：

```sh
docker compose run -T --rm migrate up
```

## 重置

仅停止并移除容器：

```sh
docker compose down --remove-orphans
```

删除容器和本地数据 volumes：

```sh
docker compose down -v --remove-orphans
```

Release uninstall helper 暴露了同样的破坏性重置能力：

```sh
./uninstall.sh --purge
```

只有在你明确要删除本地数据时才使用 `--purge`。如果你自己删除了 `.env`，再次运行安装脚本会重新生成；否则安装脚本不会重置已有密钥。
