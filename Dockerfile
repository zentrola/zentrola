# syntax=docker/dockerfile:1

ARG TARGETARCH=amd64
ARG CODEX_VERSION=0.154.0

FROM alpine:3.22 AS codex

ARG CODEX_VERSION

RUN apk add --no-cache nodejs npm \
    && npm install --global --omit=dev "@openai/codex@${CODEX_VERSION}" \
    && codex --version \
    && codex app-server --help >/dev/null

FROM alpine:3.22

ARG TARGETARCH

RUN apk add --no-cache ca-certificates nodejs tzdata \
    && addgroup -S -g 10001 zentrola \
    && adduser -S -D -H -u 10001 -G zentrola zentrola \
    && mkdir -p /app/data /app/run \
    && chown -R zentrola:zentrola /app

COPY --from=codex /usr/local/lib/node_modules/@openai/ /usr/local/lib/node_modules/@openai/

RUN ln -s ../lib/node_modules/@openai/codex/bin/codex.js /usr/local/bin/codex

WORKDIR /app

COPY --chown=zentrola:zentrola dist/linux/${TARGETARCH}/ /app/
COPY --chown=root:root scripts/docker-entrypoint.sh /usr/local/bin/zentrola-entrypoint

RUN chmod 0755 /app/zentrola /app/zentrola-web /usr/local/bin/zentrola-entrypoint

ENV CODEX_EXECUTABLE=/usr/local/bin/codex

USER zentrola

EXPOSE 9527 9528

ENTRYPOINT ["/usr/local/bin/zentrola-entrypoint"]
CMD ["backend"]
