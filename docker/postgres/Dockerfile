# syntax=docker/dockerfile:1

ARG POSTGRES_VERSION=17.11
ARG PGVECTOR_VERSION=0.8.7
ARG PGVECTOR_SHA256=cac0b10c360f05b2d521200105ba3697e773d4cd3731f5a915a7e37ebe0bea85
ARG PG_BIGM_VERSION=1.2-20250903
ARG PG_BIGM_SHA256=4d4fb48161e3f9e99207851299379c83b3e5d1d3c1a07c35f6a3153d577b3142

# 与运行镜像使用同一官方镜像编译，保证 PostgreSQL 主版本、头文件与 musl 一致。
FROM postgres:${POSTGRES_VERSION}-alpine AS extension-builder

ARG PGVECTOR_VERSION
ARG PGVECTOR_SHA256
ARG PG_BIGM_VERSION
ARG PG_BIGM_SHA256

# icu-dev：PostgreSQL 服务端头文件 pg_locale.h 依赖 ICU 头文件。
RUN apk add --no-cache build-base icu-dev

WORKDIR /tmp/build

# with_llvm=no：不生成 JIT bitcode，避免依赖与官方镜像编译时完全一致的 clang/llvm 版本；
# OPTFLAGS=""：pgvector 默认使用 -march=native，关闭后镜像可在任意同架构 CPU 上运行。
RUN set -eux; \
    wget -qO pgvector.tar.gz "https://github.com/pgvector/pgvector/archive/refs/tags/v${PGVECTOR_VERSION}.tar.gz"; \
    echo "${PGVECTOR_SHA256}  pgvector.tar.gz" | sha256sum -c -; \
    wget -qO pg_bigm.tar.gz "https://github.com/pgbigm/pg_bigm/archive/refs/tags/v${PG_BIGM_VERSION}.tar.gz"; \
    echo "${PG_BIGM_SHA256}  pg_bigm.tar.gz" | sha256sum -c -; \
    mkdir pgvector pg_bigm; \
    tar -xzf pgvector.tar.gz -C pgvector --strip-components=1; \
    tar -xzf pg_bigm.tar.gz -C pg_bigm --strip-components=1; \
    make -C pgvector with_llvm=no OPTFLAGS=""; \
    make -C pgvector with_llvm=no install DESTDIR=/out; \
    make -C pg_bigm USE_PGXS=1 with_llvm=no; \
    make -C pg_bigm USE_PGXS=1 with_llvm=no install DESTDIR=/out; \
    mkdir -p /out/usr/share/licenses/pgvector /out/usr/share/licenses/pg_bigm; \
    cp pgvector/LICENSE /out/usr/share/licenses/pgvector/LICENSE; \
    cp pg_bigm/LICENSE /out/usr/share/licenses/pg_bigm/LICENSE

FROM postgres:${POSTGRES_VERSION}-alpine

ARG POSTGRES_VERSION
ARG PGVECTOR_VERSION
ARG PG_BIGM_VERSION
ARG VCS_REF=unknown
ARG BUILD_DATE

LABEL org.opencontainers.image.title="zentrola-postgres" \
      org.opencontainers.image.description="PostgreSQL ${POSTGRES_VERSION} (Alpine) with pgvector and pg_bigm for Zentrola" \
      org.opencontainers.image.version="${POSTGRES_VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.source="https://github.com/zentrola/zentrola" \
      org.opencontainers.image.licenses="PostgreSQL" \
      org.opencontainers.image.authors="longjianghu <longjianghu1982@gmail.com>" \
      org.opencontainers.image.vendor="Longjianghu" \
      maintainer="longjianghu <longjianghu1982@gmail.com>" \
      pgvector.version="${PGVECTOR_VERSION}" \
      pg_bigm.version="${PG_BIGM_VERSION}"

COPY --from=extension-builder /out/ /
COPY docker/postgres/NOTICE /usr/share/licenses/zentrola-postgres/NOTICE
