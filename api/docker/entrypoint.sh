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

# -------- ensure runtime socket dir ----------
# На Alpine Postgres по умолчанию использует /run/postgresql для Unix-сокета
if [[ ! -d /run/postgresql ]]; then
  echo "[postgres] creating /run/postgresql"
  mkdir -p /run/postgresql
fi
chmod 775 /run/postgresql
chown -R postgres:postgres /run/postgresql

# -------- ensure PGDATA dir ----------
if [[ ! -d "${PGDATA}" ]]; then
  echo "[postgres] creating PGDATA at ${PGDATA}"
  mkdir -p "${PGDATA}"
fi
chown -R postgres:postgres "${PGDATA}"
chmod 700 "${PGDATA}"

# helper to run commands as postgres user
as_postgres() { su-exec postgres:postgres "$@"; }

# -------- initdb (первый старт) ----------
if [[ ! -s "${PGDATA}/PG_VERSION" ]]; then
  echo "[postgres] initdb at ${PGDATA}"
  as_postgres /usr/libexec/postgresql16/initdb -D "${PGDATA}" --auth-local=trust --auth-host=md5 >/dev/null

  # Слушаем только localhost
  as_postgres sh -c "printf '%s\n' \
    "listen_addresses = '127.0.0.1'" \
    "port = ${PGPORT}" \
    "unix_socket_directories = '/run/postgresql'" \
    >> '${PGDATA}/postgresql.conf'"

  # Разрешим md5 для TCP с localhost (локальное FORCE уже trust)
  as_postgres sh -c "echo \"host all all 127.0.0.1/32 md5\" >> '${PGDATA}/pg_hba.conf'"
fi

# -------- start postgres (foreground child) ----------
# Явно укажем директорию сокета, чтобы не зависеть от конфигов
echo "[postgres] starting…"
as_postgres pg_ctl -D "${PGDATA}" \
  -o "-c listen_addresses=127.0.0.1 -p ${PGPORT} -c unix_socket_directories=/run/postgresql" \
  -w start

# Остановим Postgres по завершению контейнера
trap 'echo "[postgres] stopping"; su-exec postgres:postgres pg_ctl -D "${PGDATA}" -m fast -w stop' TERM INT EXIT

# -------- ensure user/db ----------
# superuser по умолчанию — 'postgres'
if ! as_postgres psql -U postgres -h 127.0.0.1 -p "${PGPORT}" -tAc "SELECT 1 FROM pg_roles WHERE rolname='${POSTGRES_USER}'" | grep -q 1; then
  echo "[postgres] creating role ${POSTGRES_USER}"
  as_postgres psql -U postgres -h 127.0.0.1 -p "${PGPORT}" -v ON_ERROR_STOP=1 <<SQL
CREATE ROLE ${POSTGRES_USER} LOGIN PASSWORD '${POSTGRES_PASSWORD}';
ALTER ROLE ${POSTGRES_USER} CREATEDB;
SQL
fi

if ! as_postgres psql -U postgres -h 127.0.0.1 -p "${PGPORT}" -tAc "SELECT 1 FROM pg_database WHERE datname='${POSTGRES_DB}'" | grep -q 1; then
  echo "[postgres] creating database ${POSTGRES_DB}"
  as_postgres createdb -U postgres -h 127.0.0.1 -p "${PGPORT}" -O "${POSTGRES_USER}" "${POSTGRES_DB}"
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