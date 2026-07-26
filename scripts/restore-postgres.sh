#!/bin/sh
set -eu

: "${NORBOT_BACKUP_PASSPHRASE:?set NORBOT_BACKUP_PASSPHRASE}" 
backup="${1:?usage: scripts/restore-postgres.sh /absolute/path/to/norbot-postgres-*.dump.enc}"
checksum="$backup.sha256"

if [ ! -f "$backup" ] || [ ! -f "$checksum" ]; then
  echo "backup and matching .sha256 file are required" >&2
  exit 2
fi
shasum -a 256 -c "$checksum"
printf 'Restoring %s destroys and recreates objects in %s. Type RESTORE: ' "$backup" "${POSTGRES_DB:-norbot}" >&2
read -r confirmation
[ "$confirmation" = RESTORE ] || { echo "aborted" >&2; exit 1; }
openssl enc -d -aes-256-cbc -pbkdf2 -pass env:NORBOT_BACKUP_PASSPHRASE -in "$backup" | docker compose exec -T postgres pg_restore -U "${POSTGRES_USER:-norbot}" -d "${POSTGRES_DB:-norbot}" --clean --if-exists --exit-on-error
