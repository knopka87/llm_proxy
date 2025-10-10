# syntax=docker/dockerfile:1.6

# ---------- build ----------
FROM golang:1.24-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates tzdata

# go.mod / go.sum из корня
COPY go.mod go.sum ./
RUN go mod download

# исходники
COPY api ./api

# билд бота (main в api/cmd/bot)
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/server ./api/cmd/bot

# ставим CLI для миграций (postgres)
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    GOBIN=/out go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.17.0

# миграции и entrypoint
COPY api/migrations /out/migrations
COPY api/docker/entrypoint.sh /out/entrypoint.sh
RUN chmod +x /out/entrypoint.sh

# ---------- runtime ----------
FROM alpine:3.20
WORKDIR /app

# Постгресс + утилиты
RUN apk add --no-cache ca-certificates tzdata bash \
    postgresql16 postgresql16-client su-exec

# Бинарники и миграции
COPY --from=build /out/server /app/server
COPY --from=build /out/migrate /usr/local/bin/migrate
COPY --from=build /out/migrations /app/migrations
COPY --from=build /out/entrypoint.sh /app/entrypoint.sh

# Конфиг и дефолты
ENV PORT=8000 \
    MIGRATIONS_DIR=/app/migrations \
    PGDATA=/var/lib/postgresql/data \
    PGPORT=5432 \
    POSTGRES_USER=childbot \
    POSTGRES_PASSWORD=childbot \
    POSTGRES_DB=childbot
# Если DATABASE_URL не задан извне — соберём его в entrypoint по localhost

# Порты
EXPOSE 8080

# Подготовим каталог данных Postgres
RUN mkdir -p /var/lib/postgresql /var/lib/postgresql/data && \
    chown -R postgres:postgres /var/lib/postgresql && \
    chmod 700 /var/lib/postgresql/data

# Запуск под пользователем postgres (и Postgres, и ваше приложение)
USER postgres:postgres

ENTRYPOINT ["/app/entrypoint.sh"]