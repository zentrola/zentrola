# Zentrola Codex 基础镜像

该目录维护只包含 Alpine 运行库和 Codex 原生运行文件的基础镜像。构建阶段会临时使用 Node.js/npm 下载对应架构的 Codex，最终镜像不包含 Node.js/npm。

## 构建

```powershell
docker build `
  -f docker/codex-base/Dockerfile `
  -t longjianghu/zentrola-codex-base:0.154.0 .
```

应用镜像通过 `BASE_IMAGE` 参数继承该基础镜像：

```powershell
docker build `
  --build-arg BASE_IMAGE=longjianghu/zentrola-codex-base:0.154.0 `
  -t longjianghu/zentrola:1.0.0 .
```
