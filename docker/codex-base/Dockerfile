# syntax=docker/dockerfile:1

ARG CODEX_VERSION=0.154.0
ARG ALPINE_VERSION=3.22

# ---------- 构建阶段 ----------

# 固定在构建机原生架构上运行，跨架构构建时不需要 QEMU 模拟。
FROM --platform=$BUILDPLATFORM alpine:${ALPINE_VERSION} AS codex-builder

# TARGETARCH 由 BuildKit 自动注入，这里只声明，不设默认值。
ARG TARGETARCH
ARG CODEX_VERSION

RUN apk add --no-cache ca-certificates nodejs npm tzdata

WORKDIR /tmp/codex

# 直接按目标架构拉取平台包并解压，不依赖 npm 按当前 CPU 筛选 optional dependency 的行为。
RUN set -eux; \
    case "${TARGETARCH}" in \
        amd64) pkg_suffix='linux-x64'; triple='x86_64-unknown-linux-musl';; \
        arm64) pkg_suffix='linux-arm64'; triple='aarch64-unknown-linux-musl';; \
        *) echo "unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1;; \
    esac; \
    tarball="$(npm pack "@openai/codex@${CODEX_VERSION}-${pkg_suffix}" --silent)"; \
    mkdir -p /app/data /out/codex /out/app/data/.codex /out/app/run /out/usr/local/bin; \
    tar -xzf "${tarball}" -C /out/codex --strip-components=1; \
    # 构建期校验：目录结构变了就直接失败，而不是等到运行时。 \
    test -x "/out/codex/vendor/${triple}/bin/codex"; \
    # 把 triple 记下来，启动脚本直接读取，避免两处重复维护映射。 \
    echo "${triple}" > /out/codex/TRIPLE; \
    printf '%s\n' '#!/bin/sh' 'exec "/opt/codex/vendor/$(cat /opt/codex/TRIPLE)/bin/codex" "$@"' > /out/usr/local/bin/codex; \
    chmod 0755 /out/usr/local/bin/codex; \
    addgroup -S -g 10001 zentrola; \
    adduser -S -D -h /app/data -u 10001 -G zentrola zentrola; \
    chown -R 10001:10001 /out/app; \
    cp /etc/passwd /out/passwd; \
    cp /etc/group /out/group; \
    cp /etc/shadow /out/shadow

# ---------- 运行阶段 ----------

FROM alpine:${ALPINE_VERSION}

ARG CODEX_VERSION

LABEL org.opencontainers.image.title="codex-runtime" \
      org.opencontainers.image.description="Codex native runtime on Alpine (no Node.js)" \
      org.opencontainers.image.authors="Longjianghu <215241062@qq.com>" \
      org.opencontainers.image.vendor="Longjianghu" \
      maintainer="Longjianghu <215241062@qq.com>" \
      codex.version="${CODEX_VERSION}"

# 家目录直接设为 /app/data，这样 ~/.codex 有可写位置。
# 如果 Codex 需要操作 Git 仓库或执行 shell 脚本，再加上 git bash。
COPY --from=codex-builder /out/codex /opt/codex
COPY --from=codex-builder /etc/ssl /etc/ssl
COPY --from=codex-builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=codex-builder /out/app /app
COPY --from=codex-builder /out/usr/local/bin/codex /usr/local/bin/codex
COPY --from=codex-builder /out/passwd /etc/passwd
COPY --from=codex-builder /out/group /etc/group
COPY --from=codex-builder /out/shadow /etc/shadow

ENV HOME=/app/data \
    CODEX_HOME=/app/data/.codex \
    CODEX_EXECUTABLE=/usr/local/bin/codex \
    TZ=Asia/Shanghai

WORKDIR /app

# 登录状态和会话数据持久化。
VOLUME ["/app/data"]

USER zentrola
