#!/bin/sh
set -eu

: "${NORBOT_BACKUP_DIR:?set NORBOT_BACKUP_DIR to independent durable storage}"
: "${NORBOT_BACKUP_PASSPHRASE:?set NORBOT_BACKUP_PASSPHRASE}"
command -v gpg >/dev/null || { echo "gpg is required for authenticated backup encryption" >&2; exit 127; }
repo="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
compose() { docker compose --project-directory "$repo" -f "$repo/docker-compose.yml" -f "$repo/docker-compose.production.yml" "$@"; }
mkdir -p "$NORBOT_BACKUP_DIR"
chmod 700 "$NORBOT_BACKUP_DIR"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
temporary="$(mktemp "$NORBOT_BACKUP_DIR/.norbot-base-$stamp.XXXXXX")"
output="$NORBOT_BACKUP_DIR/norbot-postgres-$stamp.base.tar.enc"
trap 'rm -f "$temporary"' EXIT HUP INT TERM

compose exec -T postgres pg_basebackup -U "${POSTGRES_USER:-norbot}" --pgdata=- --format=tar --wal-method=none --checkpoint=fast >"$temporary"
printf '%s' "$NORBOT_BACKUP_PASSPHRASE" | gpg --batch --yes --pinentry-mode loopback --passphrase-fd 0 --symmetric --cipher-algo AES256 --output "$output" "$temporary"
shasum -a 256 "$output" >"$output.sha256"
lsn="$(compose exec -T postgres psql -U "${POSTGRES_USER:-norbot}" -d "${POSTGRES_DB:-norbot}" -Atqc 'SELECT pg_switch_wal(), pg_current_wal_lsn()' | tail -1)"
printf '%s\n' "created_at=$stamp" "format=pg_basebackup_tar" "wal_method=none" "restore_requires=retained_encrypted_wal" "switch_lsn=$lsn" "sha256_file=$(basename "$output").sha256" >"$output.manifest"
echo "$output"
