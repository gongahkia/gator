#!/bin/sh
set -eu

: "${NORBOT_BACKUP_DIR:?set NORBOT_BACKUP_DIR to a durable backup directory}"
: "${NORBOT_BACKUP_PASSPHRASE:?set NORBOT_BACKUP_PASSPHRASE to an encryption secret held outside this host}"

retention_days="${NORBOT_BACKUP_RETENTION_DAYS:-30}"
case "$retention_days" in *[!0-9]*|'') echo "NORBOT_BACKUP_RETENTION_DAYS must be a positive integer" >&2; exit 2;; esac

mkdir -p "$NORBOT_BACKUP_DIR"
chmod 700 "$NORBOT_BACKUP_DIR"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
temporary="$(mktemp "$NORBOT_BACKUP_DIR/.norbot-$stamp.XXXXXX")"
output="$NORBOT_BACKUP_DIR/norbot-postgres-$stamp.dump.enc"
trap 'rm -f "$temporary"' EXIT HUP INT TERM

docker compose exec -T postgres pg_dump -U "${POSTGRES_USER:-norbot}" -d "${POSTGRES_DB:-norbot}" --format=custom >"$temporary"
openssl enc -aes-256-cbc -pbkdf2 -salt -pass env:NORBOT_BACKUP_PASSPHRASE -in "$temporary" -out "$output"
shasum -a 256 "$output" >"$output.sha256"
printf '%s\n' "created_at=$stamp" "format=pg_dump_custom" "encrypted=aes-256-cbc-pbkdf2" "sha256_file=$(basename "$output").sha256" >"$output.manifest"
find "$NORBOT_BACKUP_DIR" -type f -name 'norbot-postgres-*' -mtime "+$retention_days" -delete
echo "$output"
