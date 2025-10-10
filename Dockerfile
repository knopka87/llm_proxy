# syntax=docker/dockerfile:1.6

########################
# Build stage
########################
FROM golang:1.24-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates tzdata

# go.mod / go.sum из корня
COPY go.mod go.sum ./
RUN go mod download

# исходники приложения
COPY api ./api

# билд бинаря
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/server ./api/cmd/bot

# cli для миграций (postgres)
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    GOBIN=/out go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.17.0

# миграции и entrypoint
COPY api/migrations /out/migrations
COPY api/docker/entrypoint.sh /out/entrypoint.sh
RUN chmod +x /out/entrypoint.sh

########################
# Runtime stage (app-only)
########################
FROM alpine:3.20
WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata bash

# бинарь и миграции
COPY --from=build /out/server /app/server
COPY --from=build /out/migrate /usr/local/bin/migrate
COPY --from=build /out/migrations /app/migrations
COPY --from=build /out/entrypoint.sh /app/entrypoint.sh

ENV PORT=8080 \
    MIGRATIONS_DIR=/app/migrations

EXPOSE 8080
ENTRYPOINT ["/app/entrypoint.sh"]