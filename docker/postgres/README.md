# Zentrola PostgreSQL 镜像

该目录维护 Zentrola 使用的 PostgreSQL 镜像 `longjianghu/postgres:17.11-alpine3.24`：基于官方 `postgres:17.11-alpine3.24`，只额外编译安装知识库所需的两个扩展：

| 扩展 | 用途 |
|---|---|
| [pgvector](https://github.com/pgvector/pgvector) | 向量存储与 HNSW 相似度检索 |
| [pg_bigm](https://github.com/pgbigm/pg_bigm) | 二元组（bigram）全文索引，支持中文 2 字及以上关键词的 `LIKE '%…%'` 走索引 |

基础镜像、PostgreSQL 主版本与 musl 运行库都与官方 Alpine 镜像一致，现有 `postgres:17.11-alpine` / `postgres:17.11-alpine3.24` 的数据卷可以直接挂载，无需导出导入或重建索引。升级前仍应先用 `pg_dump` 备份。

镜像只提供扩展文件，不在初始化脚本中执行 `CREATE EXTENSION`；扩展由 Zentrola 迁移按需创建，已有数据卷升级时同样生效。时区保持官方镜像默认（UTC），由 compose 的 `TZ`/`PGTZ` 控制。

## 构建

该 Dockerfile 支持 `linux/amd64` 和 `linux/arm64`，构建上下文为仓库根目录。

推送到镜像仓库（推荐）：

```powershell
docker buildx build `
  --platform linux/amd64,linux/arm64 `
  -f docker/postgres/Dockerfile `
  --build-arg VCS_REF=(git rev-parse HEAD) `
  --build-arg BUILD_DATE=(Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ') `
  -t longjianghu/postgres:17.11-alpine3.24 `
  --push `
  .
```

本地构建当前架构：

```powershell
docker build -f docker/postgres/Dockerfile -t zentrola-postgres:local .
```

## 发布要求

`compose.yaml` 直接引用 `longjianghu/postgres:17.11-alpine3.24`，发布引用该镜像的 Zentrola 版本前，必须先推送多平台镜像，并在发布说明中记录镜像 digest。CI 集成测试不依赖镜像仓库，而是在任务中用本目录的 Dockerfile 现场构建。

## 升级扩展版本

修改 Dockerfile 中的 `PGVECTOR_VERSION`/`PG_BIGM_VERSION`，并同步更新对应的 `*_SHA256`（GitHub tag 源码包的 SHA-256）。升级 PostgreSQL 小版本或 Alpine 版本时修改 `POSTGRES_VERSION`/`ALPINE_VERSION`，并同步更新镜像标签与 `compose.yaml`；主版本升级需按 PostgreSQL 官方流程迁移数据。

## 验证

```sql
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_bigm;
SELECT extname, extversion FROM pg_extension WHERE extname IN ('vector', 'pg_bigm');
```
