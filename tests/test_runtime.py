import sqlite3
from datetime import UTC, datetime
from pathlib import Path

import pytest

from hcb.models import Account, Event
from hcb.paths import AppPaths
from hcb.runtime import Runtime
from hcb.storage import Storage


def test_instance_refresh_uses_and_closes_an_independent_database(tmp_path: Path) -> None:
    paths = AppPaths(tmp_path / "config", tmp_path / "data", tmp_path / "cache")
    runtime = Runtime(paths, environ={})
    runtime.storage.upsert_account(Account("a", "private@example.test"))

    class Engine:
        def __init__(self) -> None:
            self.storage: Storage = runtime.storage

        def refresh_occurrences(
            self, account_id: str, calendar_id: str, start: datetime, end: datetime
        ) -> list[Event]:
            assert self.storage is not runtime.storage
            assert self.storage.get_account(account_id) is not None
            assert calendar_id == "cal"
            assert end > start
            return []

    engine = Engine()
    runtime.sync_engine = lambda _account: engine  # type: ignore[method-assign,assignment]
    start = datetime(2026, 8, 21, tzinfo=UTC)

    assert runtime.refresh_occurrences("a", "cal", start, start.replace(day=28)) == []
    with pytest.raises(sqlite3.ProgrammingError):
        engine.storage.connection.execute("SELECT 1")
    runtime.close()
