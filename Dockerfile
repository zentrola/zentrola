# syntax=docker/dockerfile:1

ARG TARGETARCH=amd64
FROM alpine:3.22

ARG TARGETARCH

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 zentrola \
    && adduser -S -D -H -u 10001 -G zentrola zentrola \
    && mkdir -p /app/data /app/run \
    && chown -R zentrola:zentrola /app

WORKDIR /app

COPY --chown=zentrola:zentrola dist/linux/${TARGETARCH}/ /app/
COPY --chown=root:root scripts/docker-entrypoint.sh /usr/local/bin/zentrola-entrypoint

RUN chmod 0755 /app/zentrola /app/zentrola-web /usr/local/bin/zentrola-entrypoint

USER zentrola

EXPOSE 9527 9528

ENTRYPOINT ["/usr/local/bin/zentrola-entrypoint"]
CMD ["backend"]
