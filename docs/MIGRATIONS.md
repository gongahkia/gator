# Database migrations

Norbot applies ordered migrations under a PostgreSQL advisory lock. `schema_migrations` stores a SHA-256 checksum for each applied version; startup stops if a historical migration changes.

Rules:

1. Never edit an applied migration string.
2. Add the next numbered migration only; use `IF EXISTS`/`IF NOT EXISTS` where a safe upgrade needs it.
3. Use expand/contract changes: first add compatible fields/indexes, deploy readers/writers, backfill, then remove obsolete fields in a later release.
4. Run `go test -race ./...` against an upgraded PostgreSQL instance before release. CI supplies Postgres and exercises `Store.Migrate`.

The migration ledger is not a backup. Perform a tested restore before any destructive production migration.
