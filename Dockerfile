FROM golang:1.26-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /app/vilicus ./cmd/bot

FROM alpine:3.20

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S vilicus && adduser -S -G vilicus vilicus \
    && mkdir -p /app/data /app/backups \
    && chown -R vilicus:vilicus /app/data /app/backups

COPY --from=builder /app/vilicus /app/vilicus
COPY --from=builder /app/web/templates /app/web/templates
# Static assets (vendored htmx + fonts) are served at runtime; without this
# the dashboard boots but every /static request 404s.
COPY --from=builder /app/web/static /app/web/static

USER vilicus

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s \
    CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/app/vilicus"]
