# syntax=docker/dockerfile:1

ARG APP_VERSION=1.0.0
ARG BASE_IMAGE=longjianghu/zentrola-codex-base:0.154.0

FROM ${BASE_IMAGE}

ARG APP_VERSION
ARG BASE_IMAGE
ARG TARGETARCH
ARG VCS_REF=unknown
ARG BUILD_DATE

LABEL org.opencontainers.image.title="Zentrola" \
      org.opencontainers.image.description="Enterprise AI coding control plane" \
      org.opencontainers.image.version="${APP_VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.base.name="${BASE_IMAGE}" \
      org.opencontainers.image.source="https://github.com/zentrola/zentrola" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.authors="longjianghu <longjianghu1982@gmail.com>" \
      org.opencontainers.image.vendor="Longjianghu" \
      maintainer="longjianghu <longjianghu1982@gmail.com>"

WORKDIR /app

COPY --chown=zentrola:zentrola dist/linux/${TARGETARCH}/ /app/
COPY --chmod=0755 --chown=zentrola:zentrola dist/linux/${TARGETARCH}/zentrola /app/zentrola
COPY --chmod=0755 --chown=zentrola:zentrola dist/linux/${TARGETARCH}/zentrola-web /app/zentrola-web
COPY --chmod=0755 --chown=root:root scripts/docker-entrypoint.sh /usr/local/bin/zentrola-entrypoint
COPY --chown=root:root LICENSE NOTICE THIRD_PARTY_NOTICES.md /usr/share/licenses/zentrola/
COPY --chown=root:root third_party_licenses/ /usr/share/licenses/zentrola/third_party/

USER zentrola

EXPOSE 9527 9528

ENTRYPOINT ["/usr/local/bin/zentrola-entrypoint"]
CMD ["backend"]
