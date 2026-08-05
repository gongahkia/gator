#!/bin/sh
set -eu

: "${NORBOT_BACKUP_PASSPHRASE:?set NORBOT_BACKUP_PASSPHRASE}"
: "${NORBOT_WAL_ARCHIVE_HOST_DIR:?set NORBOT_WAL_ARCHIVE_HOST_DIR}"
[ "${NORBOT_RESTORE_DRILL:-}" = "1" ] || { echo "set NORBOT_RESTORE_DRILL=1 to run a PITR restore drill" >&2; exit 2; }
command -v gpg >/dev/null || { echo "gpg is required for authenticated backup decryption" >&2; exit 127; }
repo="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
backup="${1:?usage: scripts/restore-drill-pitr.sh /absolute/path/to/norbot-postgres-*.base.tar.enc}"
checksum="$backup.sha256"
[ -f "$backup" ] && [ -f "$checksum" ] || { echo "backup and matching checksum are required" >&2; exit 2; }
shasum -a 256 -c "$checksum"
root="$(mktemp -d "${TMPDIR:-/tmp}/norbot-pitr.XXXXXX")"
name="norbot-pitr-drill-$(date -u +%Y%m%d%H%M%S)"
cleanup() { docker rm -f "$name" >/dev/null 2>&1 || true; rm -rf "$root"; }
trap cleanup EXIT HUP INT TERM
printf '%s' "$NORBOT_BACKUP_PASSPHRASE" | gpg --batch --yes --pinentry-mode loopback --passphrase-fd 0 --decrypt "$backup" | tar -x -C "$root"
touch "$root/recovery.signal"
printf "%s\n" "restore_command = '/usr/local/bin/norbot-restore-wal %f %p'" "recovery_target = 'immediate'" "recovery_target_action = 'promote'" >>"$root/postgresql.auto.conf"
docker run -d --name "$name" -e NORBOT_BACKUP_PASSPHRASE -e NORBOT_WAL_ARCHIVE_DIR=/var/lib/postgresql/wal-archive -v "$root:/var/lib/postgresql/data" -v "$NORBOT_WAL_ARCHIVE_HOST_DIR:/var/lib/postgresql/wal-archive:ro" -v "$repo/deploy/postgres/norbot-restore-wal:/usr/local/bin/norbot-restore-wal:ro" norbot-postgres-pitr:16 >/dev/null
for attempt in $(seq 1 90); do
  docker exec "$name" pg_isready -U "${POSTGRES_USER:-norbot}" -d "${POSTGRES_DB:-norbot}" >/dev/null 2>&1 && break
  [ "$attempt" -eq 90 ] && { docker logs "$name" >&2; echo "PITR restore drill database did not start" >&2; exit 1; }
  sleep 1
done
docker exec "$name" psql -U "${POSTGRES_USER:-norbot}" -d "${POSTGRES_DB:-norbot}" -Atqc "SELECT count(*) FROM schema_migrations" | grep -Eq '^[1-9][0-9]*$'
echo "PITR restore drill passed: $backup"
