# Zentrola Codex 基础镜像

该目录维护只包含 Alpine 运行库和 Codex 原生运行文件的基础镜像。构建阶段会临时使用 Node.js/npm 下载对应架构的 Codex，最终镜像不包含 Node.js/npm。

## 构建

该 Dockerfile 支持 `linux/amd64` 和 `linux/arm64`。多平台构建必须使用
Docker Buildx；`--load` 不能将多平台 manifest 加载到本地 Docker 镜像仓库。

推送到镜像仓库（推荐）：

```powershell
docker buildx build `
  --platform linux/amd64,linux/arm64 `
  -f docker/codex-base/Dockerfile `
  -t longjianghu/zentrola-codex-base:0.154.0 `
  --push `
  .
```

只在本地生成多平台 OCI 镜像归档（不会推送）：

```powershell
docker buildx build `
  --platform linux/amd64,linux/arm64 `
  -f docker/codex-base/Dockerfile `
  -t longjianghu/zentrola-codex-base:0.154.0 `
  --output "type=oci,dest=dist/zentrola-codex-base-0.154.0.tar" `
  .
```

应用镜像通过 `BASE_IMAGE` 参数继承该基础镜像：

```powershell
docker build `
  --build-arg BASE_IMAGE=longjianghu/zentrola-codex-base:0.154.0 `
  -t longjianghu/zentrola:1.0.0 .
```
