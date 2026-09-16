"""Saved searches and local workspace search."""

from __future__ import annotations

from datetime import date, datetime

from .application import SavedSearch, SearchResult, _ApplicationServiceBase, _id
from .errors import NotFoundError
from .models import Calendar, Conflict, DriveFile, Event, Task, TaskList, utc_now
from .search import parse_palette_query


class SearchServiceMixin(_ApplicationServiceBase):
    def save_search(
        self,
        account_id: str,
        name: str,
        query: str,
        *,
        id: str | None = None,
        max_count: int | None = None,
    ) -> SavedSearch:
        self._account(account_id)
        if not name.strip() or not query.strip():
            raise ValueError("saved search name and query are required")
        if max_count is not None and max_count < 1:
            raise ValueError("saved search max_count must be positive")
        item = SavedSearch(id or _id(), account_id, name.strip(), query.strip(), utc_now())
        with self.storage.transaction():
            if max_count is not None:
                existing = self.storage.connection.execute(
                    "SELECT 1 FROM saved_searches WHERE account_id=? AND name=?",
                    (account_id, item.name),
                ).fetchone()
                if existing is None:
                    count = self.storage.connection.execute(
                        "SELECT COUNT(*) FROM saved_searches WHERE account_id=?", (account_id,)
                    ).fetchone()[0]
                    if count >= max_count:
                        raise ValueError(f"saved search limit of {max_count} reached")
            self.storage.connection.execute(
                """INSERT INTO saved_searches VALUES (?,?,?,?,?)
                ON CONFLICT(account_id,name) DO UPDATE SET query=excluded.query""",
                (item.id, item.account_id, item.name, item.query, item.created_at.isoformat()),
            )
        row = self.storage.connection.execute(
            "SELECT * FROM saved_searches WHERE account_id=? AND name=?",
            (account_id, item.name),
        ).fetchone()
        assert row is not None
        return SavedSearch(
            row["id"],
            row["account_id"],
            row["name"],
            row["query"],
            datetime.fromisoformat(row["created_at"]),
        )

    def list_saved_searches(
        self, account_id: str, *, limit: int | None = None
    ) -> tuple[SavedSearch, ...]:
        if limit is not None and not 1 <= limit <= 1_000:
            raise ValueError("saved search limit must be between 1 and 1000")
        sql = "SELECT * FROM saved_searches WHERE account_id=? ORDER BY name"
        args: tuple[str, ...] | tuple[str, int] = (account_id,)
        if limit is not None:
            sql += " LIMIT ?"
            args = (account_id, limit)
        rows = self.storage.connection.execute(sql, args)
        return tuple(
            SavedSearch(
                row["id"],
                row["account_id"],
                row["name"],
                row["query"],
                datetime.fromisoformat(row["created_at"]),
            )
            for row in rows
        )

    def update_saved_search(
        self,
        account_id: str,
        search_id: str,
        *,
        name: str | None = None,
        query: str | None = None,
    ) -> SavedSearch:
        if name is None and query is None:
            raise ValueError("saved search update requires a name or query")
        self._account(account_id)
        row = self.storage.connection.execute(
            "SELECT * FROM saved_searches WHERE account_id=? AND id=?", (account_id, search_id)
        ).fetchone()
        if row is None:
            raise NotFoundError(f"Saved search {search_id!r} does not exist")
        next_name = row["name"] if name is None else name.strip()
        next_query = row["query"] if query is None else query.strip()
        if not next_name or not next_query:
            raise ValueError("saved search name and query are required")
        with self.storage.transaction():
            existing = self.storage.connection.execute(
                "SELECT 1 FROM saved_searches WHERE account_id=? AND name=? AND id<>?",
                (account_id, next_name, search_id),
            ).fetchone()
            if existing is not None:
                raise ValueError("saved search name already exists")
            self.storage.connection.execute(
                "UPDATE saved_searches SET name=?,query=? WHERE account_id=? AND id=?",
                (next_name, next_query, account_id, search_id),
            )
        return SavedSearch(
            search_id,
            account_id,
            next_name,
            next_query,
            datetime.fromisoformat(row["created_at"]),
        )

    def delete_saved_search(self, account_id: str, search_id: str) -> None:
        with self.storage.transaction():
            cursor = self.storage.connection.execute(
                "DELETE FROM saved_searches WHERE account_id=? AND id=?",
                (account_id, search_id),
            )
            if cursor.rowcount == 0:
                raise NotFoundError(f"Saved search {search_id!r} does not exist")

    @staticmethod
    def _score(title: str, text: str, body: str | None = None) -> int:
        if not text:
            return 1
        needle, candidate = text.casefold(), title.casefold()
        if candidate == needle:
            return 100
        if candidate.startswith(needle):
            return 80
        if needle in candidate:
            return 60
        return 20 if body and needle in body.casefold() else 0

    def search(
        self, account_id: str, query: str, *, today: date | None = None, limit: int = 50
    ) -> tuple[SearchResult, ...]:
        parsed = parse_palette_query(query)
        documents = self.storage.search_workspace(
            account_id,
            text=parsed.text,
            search_body=parsed.search_body,
            kinds=parsed.filters.types,
            source=parsed.filters.source,
            status=parsed.filters.status,
            priority=parsed.filters.priority,
            due=parsed.filters.due,
            completed=parsed.filters.completed,
            event_date=parsed.filters.date,
            list_query=parsed.filters.list_query,
            calendar_query=parsed.filters.calendar_query,
            today=today,
            limit=limit,
        )
        results: list[tuple[SearchResult, float, str]] = []
        for document in documents:
            item = self._workspace_search_item(account_id, document.kind, document.entity_id)
            if item is None:
                continue
            results.append(
                (
                    SearchResult(
                        document.kind,  # type: ignore[arg-type]
                        item,
                        self._score(
                            document.title,
                            parsed.text,
                            document.body if parsed.search_body else None,
                        ),
                    ),
                    document.rank,
                    document.entity_id,
                )
            )
        results.sort(key=lambda result: (-result[0].score, result[1], result[0].kind, result[2]))
        return tuple(result[0] for result in results)

    def _workspace_search_item(
        self, account_id: str, kind: str, entity_id: str
    ) -> Task | Event | Calendar | TaskList | DriveFile | SavedSearch | Conflict | None:
        if kind == "task":
            return self.storage.get_task(account_id, entity_id)
        if kind == "event":
            return self.storage.get_event(account_id, entity_id)
        if kind == "calendar":
            return self.storage.get_calendar(account_id, entity_id)
        if kind == "task-list":
            return self.storage.get_task_list(account_id, entity_id)
        if kind == "drive":
            return self.storage.get_drive_file(account_id, entity_id)
        if kind == "saved-search":
            row = self.storage.connection.execute(
                "SELECT * FROM saved_searches WHERE account_id=? AND id=?", (account_id, entity_id)
            ).fetchone()
            return (
                SavedSearch(
                    row["id"],
                    row["account_id"],
                    row["name"],
                    row["query"],
                    datetime.fromisoformat(row["created_at"]),
                )
                if row
                else None
            )
        if kind == "conflict" and entity_id.isdecimal():
            return self.storage.get_conflict(account_id, int(entity_id))
        return None

    def run_saved_search(
        self, account_id: str, search_id: str, *, today: date | None = None
    ) -> tuple[SearchResult, ...]:
        item = next(
            (saved for saved in self.list_saved_searches(account_id) if saved.id == search_id),
            None,
        )
        if item is None:
            raise NotFoundError(f"Saved search {search_id!r} does not exist")
        return self.search(account_id, item.query, today=today)
