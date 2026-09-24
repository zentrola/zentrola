# syntax=docker/dockerfile:1

ARG CODEX_VERSION=0.154.0
ARG ALPINE_VERSION=3.22

FROM --platform=$BUILDPLATFORM alpine:${ALPINE_VERSION} AS codex-builder

ARG TARGETARCH
ARG CODEX_VERSION

RUN apk add --no-cache ca-certificates nodejs npm tzdata

WORKDIR /tmp/codex

RUN set -eux; \
    case "${TARGETARCH}" in \
        amd64) pkg_suffix='linux-x64'; triple='x86_64-unknown-linux-musl';; \
        arm64) pkg_suffix='linux-arm64'; triple='aarch64-unknown-linux-musl';; \
        *) echo "unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1;; \
    esac; \
    tarball="$(npm pack "@openai/codex@${CODEX_VERSION}-${pkg_suffix}" --silent)"; \
    mkdir -p /tmp/pkg; \
    tar -xzf "${tarball}" -C /tmp/pkg --strip-components=1; \
    test -x "/tmp/pkg/vendor/${triple}/bin/codex"; \
    R=/out/rootfs; \
    mkdir -p "${R}/opt/codex/vendor" "${R}/usr/local/bin" "${R}/etc" "${R}/usr/share/zoneinfo/Asia"; \
    cp -a "/tmp/pkg/vendor/${triple}" "${R}/opt/codex/vendor/${triple}"; \
    ln -s "/opt/codex/vendor/${triple}/bin/codex" "${R}/usr/local/bin/codex"; \
    cp /usr/share/zoneinfo/Asia/Shanghai "${R}/usr/share/zoneinfo/Asia/Shanghai"; \
    printf '%s\n' \
        'root:x:0:0:root:/root:/bin/sh' \
        'nobody:x:65534:65534:nobody:/:/sbin/nologin' \
        'zentrola:x:10001:10001::/app/data:/bin/sh' > "${R}/etc/passwd"; \
    printf '%s\n' \
        'root:x:0:root' \
        'nogroup:x:65534:' \
        'zentrola:x:10001:' > "${R}/etc/group"; \
    mkdir -p /out/app/data/.codex /out/app/run

FROM alpine:${ALPINE_VERSION}

ARG CODEX_VERSION

LABEL org.opencontainers.image.title="codex-runtime" \
      org.opencontainers.image.description="Codex native runtime on Alpine (no Node.js)" \
      org.opencontainers.image.authors="Longjianghu <215241062@qq.com>" \
      org.opencontainers.image.vendor="Longjianghu" \
      maintainer="Longjianghu <215241062@qq.com>" \
      codex.version="${CODEX_VERSION}"

COPY --from=codex-builder /out/rootfs/ /
COPY --from=codex-builder /etc/ssl /etc/ssl
COPY --from=codex-builder --chown=10001:10001 /out/app /app

ENV HOME=/app/data \
    CODEX_HOME=/app/data/.codex \
    CODEX_EXECUTABLE=/usr/local/bin/codex \
    TZ=Asia/Shanghai

WORKDIR /app

VOLUME ["/app/data"]

USER zentrola
