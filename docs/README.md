# Documentation

[简体中文](./README.zh-CN.md)

Start with the Docker Compose guide for local installation and operations:

- [Docker Compose](./docker-compose.md)

The root README contains the quick start, development checks, and license summary.

## Frontend open-core boundary

The OSS console core lives in `packages/web-core` and is published as `@noxaaa/prism-oss-web-core` with each release tag. Core console UI, API client types, shared components, and OSS pages must be changed in that package first. Downstream editions should consume the package and register extensions instead of copying OSS pages.
