FROM node:24-bookworm-slim AS web-build
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go tool swag init -d ./cmd/server,./internal/transport/http -g main.go --parseInternal --parseDependencyLevel 1 --parseFuncBody --outputTypes json -o ./internal/transport/http/apidocs
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/zentrola ./cmd/server
RUN mkdir -p /out/secrets && chmod 0700 /out/secrets

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/zentrola /zentrola
COPY --from=web-build /src/web/dist /web/dist
COPY --from=build --chown=65532:65532 /out/secrets /data/secrets
USER 65532:65532
ENV LOG_FORMAT=json
EXPOSE 8080
ENTRYPOINT ["/zentrola"]
