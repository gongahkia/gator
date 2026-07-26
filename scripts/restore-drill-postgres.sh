#!/bin/sh
set -eu

: "${NORBOT_BACKUP_PASSPHRASE:?set NORBOT_BACKUP_PASSPHRASE}"
[ "${NORBOT_RESTORE_DRILL:-}" = "1" ] || { echo "set NORBOT_RESTORE_DRILL=1 to run a restore drill" >&2; exit 2; }
backup="${1:?usage: scripts/restore-drill-postgres.sh /absolute/path/to/norbot-postgres-*.dump.enc}"
checksum="$backup.sha256"
[ -f "$backup" ] && [ -f "$checksum" ] || { echo "backup and matching checksum are required" >&2; exit 2; }
shasum -a 256 -c "$checksum"

name="norbot-restore-drill-$(date -u +%Y%m%d%H%M%S)"
cleanup() { docker rm -f "$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT HUP INT TERM
docker run -d --name "$name" -e POSTGRES_USER=norbot -e POSTGRES_PASSWORD=norbot -e POSTGRES_DB=norbot postgres:16-alpine >/dev/null
for attempt in $(seq 1 30); do
  docker exec "$name" pg_isready -U norbot -d norbot >/dev/null 2>&1 && break
  [ "$attempt" -eq 30 ] && { echo "restore drill database did not start" >&2; exit 1; }
  sleep 1
done
openssl enc -d -aes-256-cbc -pbkdf2 -pass env:NORBOT_BACKUP_PASSPHRASE -in "$backup" | docker exec -i "$name" pg_restore -U norbot -d norbot --clean --if-exists --exit-on-error
docker exec "$name" psql -U norbot -d norbot -Atqc "SELECT count(*) FROM schema_migrations" | grep -Eq '^[1-9][0-9]*$'
echo "restore drill passed: $backup"
