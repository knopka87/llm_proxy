#!/usr/bin/env bash
set -euo pipefail

echo "[entrypoint] starting…"

# -------- ENV & defaults ----------
: "${PGDATA:=/var/lib/postgresql/data}"
: "${PGPORT:=5432}"
: "${POSTGRES_USER:=childbot}"
: "${POSTGRES_PASSWORD:=childbot}"
: "${POSTGRES_DB:=childbot}"
: "${MIGRATIONS_DIR:=/app/migrations}"
# Если DATABASE_URL не задан — используем локальный Postgres в этом же контейнере
: "${DATABASE_URL:=postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@127.0.0.1:${PGPORT}/${POSTGRES_DB}?sslmode=disable}"

export PGDATA PGPORT

# -------- initdb (первый старт) ----------
if [[ ! -s "${PGDATA}/PG_VERSION" ]]; then
  echo "[postgres] initdb at ${PGDATA}"
  /usr/libexec/postgresql16/initdb -D "${PGDATA}" >/dev/null
  # Безопасные дефолты: слушаем только localhost
  echo "listen_addresses = '127.0.0.1'" >> "${PGDATA}/postgresql.conf"
  echo "port = ${PGPORT}" >> "${PGDATA}/postgresql.conf"

  # Настроим md5-авторизацию
  echo "host all all 127.0.0.1/32 md5" >> "${PGDATA}/pg_hba.conf"
fi

# -------- start postgres (foreground child) ----------
echo "[postgres] starting…"
# Запустим сервер (как postgres-пользователь, мы и так под ним)
pg_ctl -D "${PGDATA}" -o "-c listen_addresses=127.0.0.1 -p ${PGPORT}" -w start

# Остановим Postgres по завершению контейнера
trap 'echo "[postgres] stopping"; pg_ctl -D "${PGDATA}" -m fast -w stop' TERM INT EXIT

# -------- ensure user/db ----------
# superuser по умолчанию — 'postgres'
if ! psql -U postgres -h 127.0.0.1 -p "${PGPORT}" -tAc "SELECT 1 FROM pg_roles WHERE rolname='${POSTGRES_USER}'" | grep -q 1; then
  echo "[postgres] creating role ${POSTGRES_USER}"
  psql -U postgres -h 127.0.0.1 -p "${PGPORT}" -v ON_ERROR_STOP=1 <<SQL
CREATE ROLE ${POSTGRES_USER} LOGIN PASSWORD '${POSTGRES_PASSWORD}';
ALTER ROLE ${POSTGRES_USER} CREATEDB;
SQL
fi

if ! psql -U postgres -h 127.0.0.1 -p "${PGPORT}" -tAc "SELECT 1 FROM pg_database WHERE datname='${POSTGRES_DB}'" | grep -q 1; then
  echo "[postgres] creating database ${POSTGRES_DB}"
  createdb -U postgres -h 127.0.0.1 -p "${PGPORT}" -O "${POSTGRES_USER}" "${POSTGRES_DB}"
fi

# -------- migrations ----------
if [[ -d "${MIGRATIONS_DIR}" ]]; then
  echo "[migrate] applying migrations from ${MIGRATIONS_DIR}"
  set +e
  migrate -path "${MIGRATIONS_DIR}" -database "${DATABASE_URL}" up
  code=$?
  set -e
  # 0 = успех, 1 = no change
  if [[ $code -ne 0 && $code -ne 1 ]]; then
    echo "[migrate] failed with code ${code}"
    exit $code
  fi
else
  echo "[migrate] migrations dir not found: ${MIGRATIONS_DIR} (skip)"
fi

# -------- run app ----------
echo "[server] starting /app/server (PORT=${PORT:-8080})"
exec /app/server