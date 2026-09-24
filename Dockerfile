# syntax=docker/dockerfile:1

ARG TARGETARCH=amd64
ARG BASE_IMAGE=longjianghu/zentrola-codex-base:0.154.0

FROM ${BASE_IMAGE}

ARG TARGETARCH

WORKDIR /app

COPY --chown=zentrola:zentrola dist/linux/${TARGETARCH}/ /app/
COPY --chown=root:root scripts/docker-entrypoint.sh /usr/local/bin/zentrola-entrypoint

USER root

RUN chmod 0755 /app/zentrola /app/zentrola-web /usr/local/bin/zentrola-entrypoint

USER zentrola

EXPOSE 9527 9528

ENTRYPOINT ["/usr/local/bin/zentrola-entrypoint"]
CMD ["backend"]
