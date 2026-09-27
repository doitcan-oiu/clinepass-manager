# syntax=docker/dockerfile:1

FROM node:24-bookworm-slim AS web-build
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.26-bookworm AS server-build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd/server/ ./cmd/server/
COPY internal/ ./internal/
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.23 AS runtime
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
ENV ADDR=:9999 \
    DATA_DIR=/app/data \
    TZ=Asia/Shanghai
COPY --from=server-build /out/server /app/server
COPY --from=web-build /src/web/dist /app/web/dist
RUN mkdir -p /app/data
EXPOSE 9999
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -q -T 3 -O /dev/null "http://127.0.0.1:${ADDR##*:}/api/health" || exit 1
ENTRYPOINT ["/app/server"]
