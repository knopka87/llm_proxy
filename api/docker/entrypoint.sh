#!/usr/bin/env bash
set -euo pipefail

echo "[entrypoint] starting app…"

: "${PORT:=8080}"
: "${MIGRATIONS_DIR:=/app/migrations}"
: "${DATABASE_URL:?DATABASE_URL is required}"

# ждём БД
# --- wait for DB socket to be reachable (TCP) ---
ATTEMPTS=60
for i in $(seq 1 ${ATTEMPTS}); do
  if bash -c '>/dev/tcp/db/5432' 2>/dev/null; then
    echo "[migrate] tcp to db:5432 is open"
    break
  fi
  echo "[migrate] db not ready yet… (${i}/${ATTEMPTS})"
  sleep 1
  if [ "$i" = "${ATTEMPTS}" ]; then
    echo "[migrate] giving up waiting for db"
    exit 1
  fi
done

# --- run migrations (idempotent) ---
MURL="${MIGRATE_DATABASE_URL:-$DATABASE_URL}"
SRC_DIR="${MIGRATIONS_DIR:-/app/migrations}"
set +e
/usr/local/bin/migrate -source "file://${SRC_DIR}" -database "${MURL}" up
code=$?
set -e
# migrate exit codes: 0=applied, 1=no change
if [ $code -ne 0 ] && [ $code -ne 1 ]; then
  echo "[migrate] failed with code ${code}"
  exit $code
fi

echo "[server] starting at :${PORT}"
trap 'echo "[server] received stop signal"; exit 0' TERM INT
exec /app/server