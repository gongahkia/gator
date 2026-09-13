from __future__ import annotations

import sqlite3
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
from threading import Barrier

import pytest

from hcb import storage as storage_module
from hcb.models import Account, TaskList
from hcb.storage import SCHEMA_VERSION, Storage


def _legacy_database(path: Path, version: int) -> None:
    connection = sqlite3.connect(path)
    connection.executescript(storage_module._SCHEMA)
    connection.execute("PRAGMA user_version = 1")
    if version >= 2:
        connection.executescript(storage_module._MIGRATION_2)
        connection.execute("PRAGMA user_version = 2")
    if version >= 3:
        connection.executescript(storage_module._MIGRATION_3)
        connection.execute("PRAGMA user_version = 3")
    if version >= 4:
        connection.executescript(storage_module._MIGRATION_4)
        connection.execute("PRAGMA user_version = 4")
    if version >= 5:
        connection.executescript(storage_module._MIGRATION_5)
        connection.execute("PRAGMA user_version = 5")
    if version >= 6:
        connection.executescript(storage_module._MIGRATION_6)
        connection.execute("PRAGMA user_version = 6")
    if version >= 7:
        connection.executescript(storage_module._MIGRATION_7)
        connection.execute("PRAGMA user_version = 7")
    connection.execute(
        """INSERT INTO accounts(id,email,display_name,provider,enabled,created_at)
        VALUES ('legacy','redacted@example.test',NULL,'google',1,'2026-08-21T00:00:00+00:00')"""
    )
    connection.execute(
        """INSERT INTO task_lists(
            id,account_id,title,remote_id,position,etag,remote_updated_at,
            local_updated_at,deleted,dirty
        ) VALUES ('inbox','legacy','Legacy inbox',NULL,0,NULL,NULL,
                  '2026-08-21T00:00:00+00:00',0,0)"""
    )
    connection.execute(
        """INSERT INTO outbox(
            account_id,entity_type,entity_id,operation,payload,created_at,attempts,last_error
        ) VALUES (
            'legacy','task','local-task','create','{"body":{"title":"Legacy intent"}}',
            '2026-08-21T00:00:00+00:00',0,NULL
        )"""
    )
    connection.commit()
    connection.close()


def test_fresh_database_and_repeated_open_are_idempotent(tmp_path: Path) -> None:
    path = tmp_path / "fresh.db"
    with Storage(path) as storage:
        storage.upsert_account(Account("fresh", "redacted@example.test"))
        storage.upsert_task_list(TaskList("inbox", "fresh", "Inbox"))
        assert storage.connection.execute("PRAGMA user_version").fetchone()[0] == SCHEMA_VERSION

    for _ in range(3):
        with Storage(path) as reopened:
            assert reopened.get_task_list("fresh", "inbox") is not None
            version = reopened.connection.execute("PRAGMA user_version").fetchone()[0]
            assert version == SCHEMA_VERSION


@pytest.mark.parametrize("version", [1, 2, 3, 4, 5, 6, 7])
def test_historical_databases_migrate_to_current_without_data_loss(
    tmp_path: Path, version: int
) -> None:
    path = tmp_path / f"v{version}.db"
    _legacy_database(path, version)

    with Storage(path) as migrated:
        assert migrated.connection.execute("PRAGMA user_version").fetchone()[0] == SCHEMA_VERSION
        assert migrated.get_account("legacy") is not None
        assert migrated.get_task_list("legacy", "inbox").title == "Legacy inbox"
        columns = {row["name"] for row in migrated.connection.execute("PRAGMA table_info(events)")}
        assert "working_location_properties" in columns
        outbox_columns = {
            row["name"] for row in migrated.connection.execute("PRAGMA table_info(outbox)")
        }
        assert {"delivery_state", "request_id", "sending_started_at"} <= outbox_columns
        range_columns = {
            row["name"]
            for row in migrated.connection.execute("PRAGMA table_info(event_instance_ranges)")
        }
        assert {"state", "stale_at", "stale_reason"} <= range_columns
        mutation = migrated.pending_mutations("legacy")[0]
        assert mutation.delivery_state.value == "pending"
        assert mutation.payload["body"]["title"] == "Legacy intent"
        assert [
            document.kind
            for document in migrated.search_workspace("legacy", text="Legacy", search_body=False)
        ] == ["task-list"]
        assert migrated.connection.execute("PRAGMA quick_check").fetchone()[0] == "ok"

    with Storage(path) as repeated:
        assert repeated.connection.execute("PRAGMA user_version").fetchone()[0] == SCHEMA_VERSION
        assert repeated.get_task_list("legacy", "inbox") is not None


def test_failed_migration_leaves_original_schema_retryable(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    path = tmp_path / "failed-migration.db"
    _legacy_database(path, 7)
    monkeypatch.setattr(
        storage_module,
        "_MIGRATION_8",
        storage_module._MIGRATION_8 + "\nSELECT * FROM missing_migration_table;\n",
    )

    with pytest.raises(sqlite3.OperationalError, match="missing_migration_table"):
        Storage(path)

    with sqlite3.connect(path) as connection:
        assert connection.execute("PRAGMA user_version").fetchone()[0] == 7
        assert (
            connection.execute(
                "SELECT name FROM sqlite_master WHERE name='workspace_search_documents'"
            ).fetchone()
            is None
        )


def test_two_concurrent_openers_can_migrate_one_legacy_database(tmp_path: Path) -> None:
    path = tmp_path / "concurrent-migration.db"
    _legacy_database(path, 7)
    with sqlite3.connect(path) as connection:
        connection.execute("PRAGMA journal_mode = WAL")
    ready = Barrier(2)

    class ContendedStorage(Storage):
        def transaction(self):
            if not getattr(self, "_migration_started", False):
                self._migration_started = True
                ready.wait(timeout=5)
            return super().transaction()

    def open_database() -> int:
        with ContendedStorage(path) as storage:
            return int(storage.connection.execute("PRAGMA user_version").fetchone()[0])

    with ThreadPoolExecutor(max_workers=2) as executor:
        futures = [executor.submit(open_database) for _ in range(2)]
        assert [future.result(timeout=10) for future in futures] == [SCHEMA_VERSION] * 2


def test_two_concurrent_openers_can_initialize_fresh_databases(tmp_path: Path) -> None:
    def open_database(path: Path, ready: Barrier) -> int:
        ready.wait(timeout=5)
        with Storage(path) as storage:
            return int(storage.connection.execute("PRAGMA user_version").fetchone()[0])

    for index in range(16):
        path = tmp_path / f"fresh-concurrent-{index}.db"
        ready = Barrier(2)
        with ThreadPoolExecutor(max_workers=2) as executor:
            futures = [executor.submit(open_database, path, ready) for _ in range(2)]
            assert [future.result(timeout=10) for future in futures] == [SCHEMA_VERSION] * 2
