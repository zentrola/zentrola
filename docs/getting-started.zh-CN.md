# 快速开始

[English](getting-started.md) | [简体中文](getting-started.zh-CN.md)

本指南使用 Zentrola 发布包完成安装和首次管理配置，不包含从源代码构建的开发流程。

## 1. 准备依赖

Zentrola 必须使用 PostgreSQL 17。推荐使用 Redis 保存认证与路由缓存，以及服务商短期冷却状态。Redis 暂时不可用不会导致 readiness 失败，Gateway 会回退到 PostgreSQL，但生产环境仍应部署 Redis。

仓库提供了一个只启动 PostgreSQL 的 Compose 示例：

```shell
docker compose up -d postgres
```

该示例将 PostgreSQL 发布到 `127.0.0.1:15432`，数据保存在 `zentrola_postgres_data` 卷中。它不会启动 Redis、Backend 或 Admin Web。

## 2. 准备发布目录

发布包目录结构如下：

```text
zentrola              # macOS/Linux Backend
zentrola.exe          # Windows Backend
zentrola-web          # macOS/Linux Admin Web
zentrola-web.exe      # Windows Admin Web
.env.example
dist/
  index.html
  config.js
  assets/
```

实际发布包只包含目标操作系统对应的可执行文件。将同目录的 `.env.example` 重命名为 `.env`。

## 3. 配置 Backend

配置 PostgreSQL 和独立的 Admin JWT 签名密钥：

```dotenv
APP_ENV=prod
HTTP_ADDR=:9527

POSTGRES_HOST=127.0.0.1
POSTGRES_PORT=5432
POSTGRES_DB=zentrola
POSTGRES_USER=zentrola
POSTGRES_PASSWORD=请替换为高强度密码
POSTGRES_SSLMODE=disable

REDIS_HOST=127.0.0.1
REDIS_PORT=6379
REDIS_DB=2
REDIS_PASSWORD=请替换为Redis密码

ADMIN_JWT_SECRET=请替换为至少32个随机字节的Base64字符串
```

使用仓库 Compose 示例时设置 `POSTGRES_PORT=15432`。生产数据库应启用 TLS，并选择合适的 `POSTGRES_SSLMODE`。

未设置 `ACP_MASTER_KEY` 或 `ACP_MASTER_KEY_FILE` 时，Backend 会生成 `data/secrets/master.key`。该文件用于加密 Provider Credential，必须与数据库一起备份。

## 4. 配置 Admin Web

```dotenv
WEB_ADDR=:9528
WEB_API_BASE_URL=http://127.0.0.1:9527
WEB_GATEWAY_BASE_URL=http://127.0.0.1:9527

CORS_ENABLED=true
CORS_ALLOWED_ORIGINS=http://127.0.0.1:9528
```

`WEB_API_BASE_URL` 和 `WEB_GATEWAY_BASE_URL` 必须是用户浏览器和客户端能够访问的地址。远程部署时应填写公网 HTTPS 地址，不能填写仅容器内部可见的主机名。同时将 Admin Web 的实际 Origin 加入 `CORS_ALLOWED_ORIGINS`。

## 5. 启动服务

macOS/Linux：

```bash
chmod +x zentrola zentrola-web
./zentrola start
./zentrola-web
```

Windows PowerShell：

```powershell
.\zentrola.exe start
.\zentrola-web.exe
```

使用 `start` 时 Backend 在后台运行。不传命令执行 `./zentrola` 或 `.\zentrola.exe`，则在当前终端前台运行。

打开 `http://127.0.0.1:9528` 并创建首位管理员。系统没有默认账号；首位管理员创建成功后，初始化入口会自动关闭。

## 6. 配置第一条治理链路

1. 打开“服务商”，初始化内置服务商元数据或手动创建服务商。
2. 添加 API Key，或添加受支持的 ChatGPT 个人订阅凭据。
3. 测试凭据；同步受支持的模型目录，或手动添加 Provider Model 映射。
4. 启用逻辑模型及其服务商映射。
5. 创建成员和 Group。
6. 将成员加入 Group，并向 Group 授权逻辑模型。
7. 为成员签发 Virtual Key 并立即妥善保存；完整密钥不会再次展示。

随后阅读[客户端接入](client-configuration.zh-CN.md)。

## 后续步骤

- 正式部署前检查 [`.env.example`](../.env.example) 中的每一项配置。
- Admin Web 或 Gateway 暴露到可信网络之外前必须配置 HTTPS。
- 保存真实 Provider Credential 前阅读[部署与运维](operations.zh-CN.md)。
