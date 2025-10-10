#!/usr/bin/env bash
set -euo pipefail

echo "[entrypoint] starting app…"

: "${PORT:=8080}"
: "${MIGRATIONS_DIR:=/app/migrations}"
: "${DATABASE_URL:?DATABASE_URL is required}"

# ждём БД
until /usr/local/bin/migrate -path "${MIGRATIONS_DIR}" -database "${DATABASE_URL}" version >/dev/null 2>&1; do
  echo "[migrate] db not ready yet…"
  sleep 1
done

# миграции (0|1 = успех)
set +e
/usr/local/bin/migrate -path "${MIGRATIONS_DIR}" -database "${DATABASE_URL}" up
code=$?
set -e
if [ $code -ne 0 ] && [ $code -ne 1 ]; then
  echo "[migrate] failed with code ${code}"
  exit $code
fi

echo "[server] starting at :${PORT}"
trap 'echo "[server] received stop signal"; exit 0' TERM INT
exec /app/server