# syntax=docker/dockerfile:1

ARG CODEX_VERSION=0.154.0
ARG ALPINE_VERSION=3.22

# ---------- 构建阶段 ----------

# 固定在构建机原生架构上运行，跨架构构建时不需要 QEMU 模拟。
FROM --platform=$BUILDPLATFORM alpine:${ALPINE_VERSION} AS codex-builder

# TARGETARCH 由 BuildKit 自动注入，这里只声明，不设默认值。
ARG TARGETARCH
ARG CODEX_VERSION

RUN apk add --no-cache nodejs npm

WORKDIR /tmp/codex

# 直接按目标架构拉取平台包并解压，不依赖 npm 按当前 CPU 筛选 optional dependency 的行为。
RUN set -eux; \
    case "${TARGETARCH}" in \
        amd64) pkg_suffix='linux-x64'; triple='x86_64-unknown-linux-musl';; \
        arm64) pkg_suffix='linux-arm64'; triple='aarch64-unknown-linux-musl';; \
        *) echo "unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1;; \
    esac; \
    tarball="$(npm pack "@openai/codex@${CODEX_VERSION}-${pkg_suffix}" --silent)"; \
    mkdir -p /out/codex; \
    tar -xzf "${tarball}" -C /out/codex --strip-components=1; \
    # 构建期校验：目录结构变了就直接失败，而不是等到运行时。 \
    test -x "/out/codex/vendor/${triple}/bin/codex"; \
    # 把 triple 记下来，最终阶段直接读取，避免两处重复维护映射。 \
    echo "${triple}" > /out/codex/TRIPLE

# ---------- 运行阶段 ----------

FROM alpine:${ALPINE_VERSION}

ARG CODEX_VERSION

LABEL org.opencontainers.image.title="codex-runtime" \
      org.opencontainers.image.description="Codex native runtime on Alpine (no Node.js)" \
      codex.version="${CODEX_VERSION}"

# 家目录直接设为 /app/data，这样 ~/.codex 有可写位置。
# 如果 Codex 需要操作 Git 仓库或执行 shell 脚本，再加上 git bash。
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 zentrola \
    && adduser -S -D -h /app/data -u 10001 -G zentrola zentrola \
    && mkdir -p /app/data/.codex /app/run \
    && chown -R zentrola:zentrola /app

COPY --from=codex-builder /out/codex /opt/codex

# 符号链接在这里创建并立刻校验，悬空链接会让构建失败。
RUN triple="$(cat /opt/codex/TRIPLE)" \
    && ln -s "/opt/codex/vendor/${triple}/bin/codex" /usr/local/bin/codex \
    && test -x /usr/local/bin/codex

ENV HOME=/app/data \
    CODEX_HOME=/app/data/.codex \
    CODEX_EXECUTABLE=/usr/local/bin/codex \
    TZ=Asia/Shanghai

WORKDIR /app

# 登录状态和会话数据持久化。
VOLUME ["/app/data"]

USER zentrola
