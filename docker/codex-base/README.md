# Zentrola Codex 基础镜像

该目录保留独立的 Codex CLI 镜像构建方案，供需要 CLI 的其他场景使用。Zentrola 主应用镜像现直接基于 Alpine 构建，订阅账号认证刷新、额度查询及重置卡兑换均通过 HTTP 完成，不再继承此镜像。

构建阶段会临时使用 Node.js/npm 下载对应架构的 Codex，在复制到最终镜像前裁剪语音资源、`codex-code-mode-host` 和文件搜索工具。最终镜像保留 Codex 主程序、必要的运行文件、证书和时区数据。交互式语音、code-mode 和文件搜索功能需要完整的 Codex 安装。

## 构建

该 Dockerfile 支持 `linux/amd64` 和 `linux/arm64`。多平台构建使用 Docker Buildx。
启用 containerd 镜像存储的 Docker 可用 `--load` 将多平台镜像加载到本地；传统镜像存储需使用 OCI 归档或推送到镜像仓库。

推送到镜像仓库（推荐）：

```powershell
docker buildx build `
  --platform linux/amd64,linux/arm64 `
  -f docker/codex-base/Dockerfile `
  --build-arg VCS_REF=(git rev-parse HEAD) `
  --build-arg BUILD_DATE=(Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ') `
  -t longjianghu/zentrola-codex-base:0.171.0 `
  --push `
  .
```

只在本地生成多平台 OCI 镜像归档（不会推送）：

```powershell
docker buildx build `
  --platform linux/amd64,linux/arm64 `
  -f docker/codex-base/Dockerfile `
  --build-arg VCS_REF=(git rev-parse HEAD) `
  --build-arg BUILD_DATE=(Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ') `
  -t longjianghu/zentrola-codex-base:0.171.0 `
  --output "type=oci,dest=dist/zentrola-codex-base-0.171.0.tar" `
  .
```

主应用镜像使用 Alpine，不依赖此镜像：

```powershell
docker build `
  --build-arg APP_VERSION=((Select-String -Path .env.example -Pattern '^APP_VERSION=(.+)$').Matches[0].Groups[1].Value.Trim()) `
  --build-arg VCS_REF=(git rev-parse HEAD) `
  --build-arg BUILD_DATE=(Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ') `
  -t longjianghu/zentrola:1.0.0 .
```
