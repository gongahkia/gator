"""Account, task, task-list, and local search persistence."""

from __future__ import annotations

import sqlite3
from datetime import date, datetime, timedelta
from typing import Any

from .models import Account, Provider, Task, TaskList, TaskPriority, TaskStatus
from .search import DateWindow
from .storage import WorkspaceSearchDocument, _datetime, _iso, _metadata, _StorageCore


class AccountTaskRepository(_StorageCore):
    def upsert_account(self, account: Account) -> None:
        self.connection.execute(
            """INSERT INTO accounts VALUES (?, ?, ?, ?, ?, ?)
            ON CONFLICT(id) DO UPDATE SET email=excluded.email,
            display_name=excluded.display_name, provider=excluded.provider,
            enabled=excluded.enabled""",
            (
                account.id,
                account.email,
                account.display_name,
                account.provider.value,
                account.enabled,
                _iso(account.created_at),
            ),
        )

    def get_account(self, account_id: str) -> Account | None:
        row = self.connection.execute("SELECT * FROM accounts WHERE id=?", (account_id,)).fetchone()
        return (
            Account(
                id=row["id"],
                email=row["email"],
                display_name=row["display_name"],
                provider=Provider(row["provider"]),
                enabled=bool(row["enabled"]),
                created_at=datetime.fromisoformat(row["created_at"]),
            )
            if row
            else None
        )

    def list_accounts(self) -> list[Account]:
        return [
            account
            for row in self.connection.execute("SELECT id FROM accounts ORDER BY email")
            if (account := self.get_account(row["id"])) is not None
        ]

    def delete_account(self, account_id: str) -> None:
        self.connection.execute("DELETE FROM accounts WHERE id=?", (account_id,))

    def upsert_task_list(self, item: TaskList) -> None:
        self.connection.execute(
            """INSERT INTO task_lists(
                id,account_id,title,remote_id,position,etag,remote_updated_at,local_updated_at,
                deleted,dirty,selected
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT(account_id,id) DO UPDATE SET title=excluded.title,
            remote_id=excluded.remote_id, position=excluded.position, etag=excluded.etag,
            remote_updated_at=excluded.remote_updated_at,
            local_updated_at=excluded.local_updated_at, deleted=excluded.deleted,
            dirty=excluded.dirty, selected=excluded.selected""",
            (
                item.id,
                item.account_id,
                item.title,
                item.remote_id,
                item.position,
                *self._meta_values(item.metadata),
                item.selected,
            ),
        )

    def _task_list(self, row: sqlite3.Row) -> TaskList:
        return TaskList(
            row["id"],
            row["account_id"],
            row["title"],
            row["remote_id"],
            row["position"],
            _metadata(row),
            bool(row["selected"]),
        )

    def get_task_list(self, account_id: str, list_id: str) -> TaskList | None:
        row = self.connection.execute(
            "SELECT * FROM task_lists WHERE account_id=? AND id=?", (account_id, list_id)
        ).fetchone()
        return self._task_list(row) if row else None

    def get_task_list_by_remote(self, account_id: str, remote_id: str) -> TaskList | None:
        row = self.connection.execute(
            "SELECT * FROM task_lists WHERE account_id=? AND remote_id=?",
            (account_id, remote_id),
        ).fetchone()
        return self._task_list(row) if row else None

    def list_task_lists(self, account_id: str, *, include_deleted: bool = False) -> list[TaskList]:
        sql = "SELECT * FROM task_lists WHERE account_id=?"
        if not include_deleted:
            sql += " AND deleted=0"
        sql += " ORDER BY position,title"
        return [self._task_list(row) for row in self.connection.execute(sql, (account_id,))]

    def delete_task_list(self, account_id: str, list_id: str) -> None:
        self.connection.execute(
            "DELETE FROM task_lists WHERE account_id=? AND id=?", (account_id, list_id)
        )

    def upsert_task(self, task: Task) -> None:
        self.connection.execute(
            """INSERT INTO tasks(
                id,account_id,list_id,title,notes,status,due,completed_at,parent_id,
                position,remote_id,etag,remote_updated_at,local_updated_at,deleted,dirty,
                priority,due_time_zone
            ) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
            ON CONFLICT(account_id,id) DO UPDATE SET list_id=excluded.list_id,
            title=excluded.title, notes=excluded.notes, status=excluded.status,
            due=excluded.due, completed_at=excluded.completed_at,
            parent_id=excluded.parent_id, position=excluded.position,
            local_order=CASE WHEN excluded.dirty=0 THEN 0 ELSE tasks.local_order END,
            remote_id=excluded.remote_id, etag=excluded.etag,
            remote_updated_at=excluded.remote_updated_at,
            local_updated_at=excluded.local_updated_at, deleted=excluded.deleted,
            dirty=excluded.dirty, priority=excluded.priority,
            due_time_zone=excluded.due_time_zone""",
            (
                task.id,
                task.account_id,
                task.list_id,
                task.title,
                task.notes,
                task.status.value,
                _iso(task.due),
                _iso(task.completed_at),
                task.parent_id,
                task.position,
                task.remote_id,
                *self._meta_values(task.metadata),
                task.priority.value,
                task.due_time_zone,
            ),
        )

    def _task(self, row: sqlite3.Row) -> Task:
        return Task(
            id=row["id"],
            account_id=row["account_id"],
            list_id=row["list_id"],
            title=row["title"],
            notes=row["notes"],
            status=TaskStatus(row["status"]),
            due=date.fromisoformat(row["due"]) if row["due"] else None,
            completed_at=_datetime(row["completed_at"]),
            parent_id=row["parent_id"],
            position=row["position"],
            remote_id=row["remote_id"],
            metadata=_metadata(row),
            priority=TaskPriority(row["priority"]),
            due_time_zone=row["due_time_zone"],
        )

    def get_task(self, account_id: str, task_id: str) -> Task | None:
        row = self.connection.execute(
            "SELECT * FROM tasks WHERE account_id=? AND id=?", (account_id, task_id)
        ).fetchone()
        return self._task(row) if row else None

    def get_task_by_remote(self, account_id: str, remote_id: str) -> Task | None:
        row = self.connection.execute(
            "SELECT * FROM tasks WHERE account_id=? AND remote_id=?", (account_id, remote_id)
        ).fetchone()
        return self._task(row) if row else None

    def resolve_task_parent_ids(self, account_id: str, list_id: str) -> None:
        """Convert Google parent IDs to stable local task IDs after all pages arrive."""
        self.connection.execute(
            """UPDATE tasks SET parent_id=(
                SELECT parent.id FROM tasks parent
                WHERE parent.account_id=tasks.account_id AND parent.list_id=tasks.list_id
                  AND parent.remote_id=tasks.parent_id
            )
            WHERE account_id=? AND list_id=? AND dirty=0 AND parent_id IS NOT NULL
              AND EXISTS (
                SELECT 1 FROM tasks parent
                WHERE parent.account_id=tasks.account_id AND parent.list_id=tasks.list_id
                  AND parent.remote_id=tasks.parent_id AND parent.id!=tasks.parent_id
              )""",
            (account_id, list_id),
        )

    def list_tasks(
        self, account_id: str, list_id: str | None = None, *, include_deleted: bool = False
    ) -> list[Task]:
        sql, args = "SELECT * FROM tasks WHERE account_id=?", [account_id]
        if list_id is not None:
            sql, args = sql + " AND list_id=?", [*args, list_id]
        if not include_deleted:
            sql += " AND deleted=0"
        sql += " ORDER BY status,due,title"
        return [self._task(row) for row in self.connection.execute(sql, args)]

    def list_tasks_page(
        self,
        account_id: str,
        *,
        limit: int,
        offset: int,
        list_id: str | None = None,
    ) -> list[Task]:
        """Load a bounded active-task page in the stable frontend list order."""
        sql, args = "SELECT * FROM tasks WHERE account_id=? AND deleted=0", [account_id]
        if list_id is not None:
            sql, args = sql + " AND list_id=?", [*args, list_id]
        sql += " ORDER BY status,local_order,due,title,id LIMIT ? OFFSET ?"
        return [self._task(row) for row in self.connection.execute(sql, [*args, limit, offset])]

    def list_task_sibling_ids(
        self, account_id: str, list_id: str, parent_id: str | None
    ) -> list[str]:
        """Return active siblings in the durable local presentation order."""
        rows = self.connection.execute(
            """SELECT id FROM tasks
            WHERE account_id=? AND list_id=? AND parent_id IS ? AND deleted=0
            ORDER BY local_order,due,title,id""",
            (account_id, list_id, parent_id),
        )
        return [str(row["id"]) for row in rows]

    def set_task_sibling_order(
        self,
        account_id: str,
        list_id: str,
        parent_id: str | None,
        task_ids: list[str],
        moved_task_id: str,
    ) -> None:
        """Persist a sibling sequence with sparse ranks where possible.

        Normal moves only need one rank between adjacent siblings. Rebuilding all
        ranks is reserved for the rare case where repeated insertions exhaust an
        integer gap.
        """
        rows = list(
            self.connection.execute(
                """SELECT id,local_order FROM tasks
                WHERE account_id=? AND list_id=? AND parent_id IS ? AND deleted=0
                ORDER BY local_order,due,title,id""",
                (account_id, list_id, parent_id),
            )
        )
        current_ids = [str(row["id"]) for row in rows]
        if len(task_ids) != len(set(task_ids)) or set(current_ids) != set(task_ids):
            raise ValueError("task sibling order references an unavailable task")
        if current_ids == task_ids:
            return

        current_ranks = {str(row["id"]): int(row["local_order"]) for row in rows}

        def sparse_rank(target: str) -> int | None:
            index = task_ids.index(target)
            other_ranks = [rank for item, rank in current_ranks.items() if item != target]
            before = task_ids[index - 1] if index > 0 else None
            after = task_ids[index + 1] if index + 1 < len(task_ids) else None
            if before is None:
                return min(other_ranks, default=0) - 1024
            if after is None:
                return max(other_ranks, default=0) + 1024
            lower = current_ranks[before]
            upper = current_ranks[after]
            return lower + (upper - lower) // 2 if upper - lower > 1 else None

        if moved_task_id not in current_ranks:
            raise ValueError("task sibling order references an unavailable task")
        rank = sparse_rank(moved_task_id)
        if rank is not None:
            result = self.connection.execute(
                """UPDATE tasks SET local_order=?
                WHERE account_id=? AND id=? AND list_id=? AND parent_id IS ? AND deleted=0""",
                (rank, account_id, moved_task_id, list_id, parent_id),
            )
            if result.rowcount == 1:
                return
            raise ValueError("task sibling order references an unavailable task")

        for local_order, task_id in enumerate(task_ids):
            result = self.connection.execute(
                """UPDATE tasks SET local_order=?
                WHERE account_id=? AND id=? AND list_id=? AND parent_id IS ? AND deleted=0""",
                (local_order * 1024, account_id, task_id, list_id, parent_id),
            )
            if result.rowcount != 1:
                raise ValueError("task sibling order references an unavailable task")

    def search_tasks(self, account_id: str, query: str, *, limit: int = 50) -> list[Task]:
        escaped = query.replace("\\", "\\\\").replace("%", "\\%").replace("_", "\\_")
        rows = self.connection.execute(
            """SELECT * FROM tasks WHERE account_id=? AND deleted=0
            AND (title LIKE ? ESCAPE '\\' OR notes LIKE ? ESCAPE '\\')
            ORDER BY status,due,title LIMIT ?""",
            (account_id, f"%{escaped}%", f"%{escaped}%", limit),
        )
        return [self._task(row) for row in rows]

    @staticmethod
    def _workspace_like(value: str) -> str:
        escaped = value.replace("\\", "\\\\").replace("%", "\\%").replace("_", "\\_")
        return f"%{escaped}%"

    @staticmethod
    def _workspace_date_clause(
        column: str, window: DateWindow, today: date
    ) -> tuple[str, list[str]]:
        today_key = today.isoformat()
        if window.kind == "today":
            return f"{column}=?", [today_key]
        if window.kind == "past":
            return f"{column}<?", [today_key]
        if window.kind == "upcoming":
            return f"{column}>=?", [today_key]
        monday = today - timedelta(days=today.weekday())
        if window.kind == "this-week":
            return f"{column} BETWEEN ? AND ?", [
                monday.isoformat(),
                (monday + timedelta(days=6)).isoformat(),
            ]
        if window.kind == "next-week":
            start = monday + timedelta(days=7)
            return f"{column} BETWEEN ? AND ?", [
                start.isoformat(),
                (start + timedelta(days=6)).isoformat(),
            ]
        if window.kind == "day" and window.day is not None:
            return f"{column}=?", [window.day]
        if window.start is not None and window.end is not None:
            return f"{column} BETWEEN ? AND ?", [window.start, window.end]
        raise ValueError("date window is incomplete")

    def search_workspace(
        self,
        account_id: str,
        *,
        text: str,
        search_body: bool,
        kinds: tuple[str, ...] = (),
        source: str | None = None,
        status: str | None = None,
        priority: str | None = None,
        due: DateWindow | str | None = None,
        completed: bool | None = None,
        event_date: DateWindow | None = None,
        list_query: str | None = None,
        calendar_query: str | None = None,
        today: date | None = None,
        limit: int = 50,
    ) -> list[WorkspaceSearchDocument]:
        """Search indexed local records with SQL-side workspace filters.

        FTS5 handles normal three-character-or-longer text queries. One- and two-character
        queries retain substring behavior through a bounded SQLite fallback, where a trigram
        index cannot represent the query.
        """
        if limit <= 0:
            return []
        clauses = ["d.account_id=?"]
        arguments: list[Any] = [account_id]
        if kinds:
            clauses.append("d.kind IN (" + ",".join("?" for _ in kinds) + ")")
            arguments.extend(kinds)
        if source is not None:
            clauses.append("d.source=?")
            arguments.append(source)
        if status is not None:
            if status.casefold() == "open":
                clauses.append("d.status='needsAction'")
            else:
                clauses.append("LOWER(COALESCE(d.status,''))=?")
                arguments.append(status.casefold())
        if priority is not None:
            clauses.append("d.priority=?")
            arguments.append(priority)
        if completed is not None:
            clauses.append("d.status=?" if completed else "d.status<>?")
            arguments.append("completed")
        current_day = today or date.today()
        if due == "none":
            clauses.append("d.due IS NULL")
        elif isinstance(due, DateWindow):
            due_clause, due_arguments = self._workspace_date_clause("d.due", due, current_day)
            clauses.append(due_clause)
            arguments.extend(due_arguments)
            if due.kind == "past":
                clauses.append("d.status<>'completed'")
        if event_date is not None:
            date_clause, date_arguments = self._workspace_date_clause(
                "d.event_date", event_date, current_day
            )
            clauses.append(date_clause)
            arguments.extend(date_arguments)
        if list_query:
            pattern = self._workspace_like(list_query)
            clauses.append("(d.list_id LIKE ? ESCAPE '\\' OR d.list_name LIKE ? ESCAPE '\\')")
            arguments.extend((pattern, pattern))
        if calendar_query:
            pattern = self._workspace_like(calendar_query)
            clauses.append(
                "(d.calendar_id LIKE ? ESCAPE '\\' OR d.calendar_name LIKE ? ESCAPE '\\')"
            )
            arguments.extend((pattern, pattern))

        query_text = text.strip()
        if len(query_text) >= 3:
            phrase = f'"{query_text.replace(chr(34), chr(34) * 2)}"'
            match = f"title : {phrase} OR body : {phrase}" if search_body else f"title : {phrase}"
            sql = """SELECT d.kind,d.entity_id,d.title,d.body,
            bm25(workspace_search, 10.0, 1.0) AS rank
            FROM workspace_search JOIN workspace_search_documents d ON d.id=workspace_search.rowid
            WHERE workspace_search MATCH ? AND """
            arguments = [match, *arguments]
            order = "rank,d.kind,d.title COLLATE NOCASE,d.entity_id"
        else:
            sql = (
                "SELECT d.kind,d.entity_id,d.title,d.body,0.0 AS rank "
                "FROM workspace_search_documents d WHERE "
            )
            if query_text:
                pattern = self._workspace_like(query_text)
                title_or_body = "d.title LIKE ? ESCAPE '\\'"
                clauses.append(
                    f"({title_or_body} OR d.body LIKE ? ESCAPE '\\')"
                    if search_body
                    else title_or_body
                )
                arguments.extend((pattern, pattern) if search_body else (pattern,))
            order = "d.kind,d.title COLLATE NOCASE,d.entity_id"
        rows = self.connection.execute(
            sql + " AND ".join(clauses) + f" ORDER BY {order} LIMIT ?",  # noqa: S608
            (*arguments, limit),
        )
        return [
            WorkspaceSearchDocument(
                row["kind"], row["entity_id"], row["title"], row["body"], float(row["rank"])
            )
            for row in rows
        ]

    def delete_task(self, account_id: str, task_id: str) -> None:
        self.connection.execute(
            "DELETE FROM tasks WHERE account_id=? AND id=?", (account_id, task_id)
        )
