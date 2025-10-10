#!/usr/bin/env bash
set -euo pipefail

echo "[entrypoint] starting…"

# -------- ENV & defaults ----------
: "${PGDATA:=/app/pgdata}"
: "${PGPORT:=5432}"
: "${POSTGRES_USER:=childbot}"
: "${POSTGRES_PASSWORD:=childbot}"
: "${POSTGRES_DB:=childbot}"
: "${MIGRATIONS_DIR:=/app/migrations}"
# Если DATABASE_URL не задан — используем локальный Postgres по TCP (127.0.0.1)
: "${DATABASE_URL:=postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@127.0.0.1:${PGPORT}/${POSTGRES_DB}?sslmode=disable}"

export PGDATA PGPORT

echo "[debug] PGDATA=${PGDATA}"
echo "[debug] DATABASE_URL=${DATABASE_URL}"

# -------- ensure PGDATA dir ----------
if [[ ! -d "${PGDATA}" ]]; then
  echo "[postgres] creating PGDATA at ${PGDATA}"
  mkdir -p "${PGDATA}"
fi
chmod 700 "${PGDATA}"

# -------- initdb (первый старт) ----------
if [[ ! -s "${PGDATA}/PG_VERSION" ]]; then
  echo "[postgres] initdb at ${PGDATA}"
  /usr/libexec/postgresql16/initdb -D "${PGDATA}" --auth-local=trust --auth-host=md5 >/dev/null

  # Слушаем только localhost; сокеты оставим в /tmp на всякий случай
  printf "%s\n" \
    "listen_addresses = '127.0.0.1'" \
    "port = ${PGPORT}" \
    "unix_socket_directories = '/tmp'" \
    >> "${PGDATA}/postgresql.conf"

  # Разрешим md5 для TCP с localhost
  echo "host all all 127.0.0.1/32 md5" >> "${PGDATA}/pg_hba.conf"
fi

# -------- start postgres (foreground child) ----------
echo "[postgres] starting…"
pg_ctl -D "${PGDATA}" \
  -o "-c listen_addresses=127.0.0.1 -p ${PGPORT} -c unix_socket_directories=/tmp" \
  -w start

# Остановим Postgres по завершению контейнера
trap 'echo "[postgres] stopping"; pg_ctl -D "${PGDATA}" -m fast -w stop' TERM INT EXIT

# -------- ensure user/db ----------
until psql -h /tmp -p "${PGPORT}" -U postgres -c '\q'; do
  >&2 echo "Postgres is unavailable - sleeping"
  sleep 1
done

if ! psql -h /tmp -p "${PGPORT}" -U postgres -tAc "SELECT 1 FROM pg_roles WHERE rolname='${POSTGRES_USER}'" | grep -q 1; then
  psql -h /tmp -p "${PGPORT}" -U postgres -c "CREATE USER ${POSTGRES_USER} WITH PASSWORD '${POSTGRES_PASSWORD}';"
fi

if ! psql -h /tmp -p "${PGPORT}" -U postgres -lqt | cut -d \| -f 1 | grep -qw "${POSTGRES_DB}"; then
  createdb -h /tmp -p "${PGPORT}" -U postgres -O "${POSTGRES_USER}" "${POSTGRES_DB}"
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