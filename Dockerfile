########################
# Build stage
########################
FROM golang:1.24-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates tzdata

# Сначала зависимости — кэшируется отдельно
COPY go.mod go.sum ./
RUN go mod download

# Код
COPY api ./api

# Сборка бинаря
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/server ./api/cmd/bot

# golang-migrate (postgres)
RUN GOBIN=/out go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.17.0

# Миграции и entrypoint
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