# syntax=docker/dockerfile:1

ARG BASE_IMAGE=longjianghu/zentrola-codex-base:0.154.0

FROM ${BASE_IMAGE}

ARG TARGETARCH

LABEL org.opencontainers.image.title="Zentrola" \
      org.opencontainers.image.authors="Longjianghu <215241062@qq.com>" \
      org.opencontainers.image.vendor="Longjianghu" \
      maintainer="Longjianghu <215241062@qq.com>"

WORKDIR /app

COPY --chown=zentrola:zentrola dist/linux/${TARGETARCH}/ /app/
COPY --chmod=0755 --chown=zentrola:zentrola dist/linux/${TARGETARCH}/zentrola /app/zentrola
COPY --chmod=0755 --chown=zentrola:zentrola dist/linux/${TARGETARCH}/zentrola-web /app/zentrola-web
COPY --chmod=0755 --chown=root:root scripts/docker-entrypoint.sh /usr/local/bin/zentrola-entrypoint

USER zentrola

EXPOSE 9527 9528

ENTRYPOINT ["/usr/local/bin/zentrola-entrypoint"]
CMD ["backend"]
