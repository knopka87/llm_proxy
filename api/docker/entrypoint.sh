#!/usr/bin/env bash
set -euo pipefail

echo "[entrypoint] starting…"
MIGRATIONS_DIR="${MIGRATIONS_DIR:-/app/migrations}"
DATABASE_URL="${DATABASE_URL:-}"
MIGRATE_BIN="${MIGRATE_BIN:-/usr/local/bin/migrate}"
MIGRATE_CMD="${MIGRATE_CMD:-up}"   # можно переопределить: up|down|force <ver>
RETRIES="${MIGRATE_RETRIES:-20}"
SLEEP_SEC="${MIGRATE_RETRY_SLEEP:-3}"
SKIP_MIGRATIONS="${SKIP_MIGRATIONS:-false}"

if [[ "${SKIP_MIGRATIONS}" == "true" ]]; then
  echo "[migrate] SKIP_MIGRATIONS=true — пропускаю миграции"
else
  if [[ -z "${DATABASE_URL}" ]]; then
    echo "[migrate] DATABASE_URL пуст — пропускаю миграции"
  elif [[ ! -x "${MIGRATE_BIN}" ]]; then
    echo "[migrate] бинарник migrate не найден по пути ${MIGRATE_BIN} — пропускаю миграции"
  elif [[ ! -d "${MIGRATIONS_DIR}" ]]; then
    echo "[migrate] каталог миграций отсутствует: ${MIGRATIONS_DIR} — пропускаю миграции"
  else
    echo "[migrate] applying migrations from ${MIGRATIONS_DIR}"
    attempt=1
    while true; do
      set +e
      "${MIGRATE_BIN}" -path "${MIGRATIONS_DIR}" -database "${DATABASE_URL}" ${MIGRATE_CMD}
      code=$?
      set -e
      # migrate: 0 = success, 1 = no change
      if [[ $code -eq 0 || $code -eq 1 ]]; then
        echo "[migrate] done (code=${code})"
        break
      fi
      if (( attempt >= RETRIES )); then
        echo "[migrate] failed after ${RETRIES} attempts (last code=${code})"
        exit $code
      fi
      echo "[migrate] attempt ${attempt}/${RETRIES} failed (code=${code}), retrying in ${SLEEP_SEC}s…"
      attempt=$(( attempt + 1 ))
      sleep "${SLEEP_SEC}"
    done
  fi
fi

echo "[server] running /app/server (PORT=${PORT:-8080})"
exec /app/server