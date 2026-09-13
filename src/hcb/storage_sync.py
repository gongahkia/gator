"""Outbox delivery, sync-cursor, checkpoint, and conflict persistence."""

from __future__ import annotations

import json
import sqlite3
from datetime import datetime
from typing import Any

from .errors import ConflictError, NotFoundError
from .models import (
    Conflict,
    ConflictStatus,
    EntityType,
    MutationOperation,
    OutboxDeliveryState,
    PendingMutation,
    SyncCursor,
    utc_now,
)
from .storage import _datetime, _iso, _StorageCore


class SyncStateRepository(_StorageCore):
    def has_later_entity_mutations(self, mutation: PendingMutation) -> bool:
        assert mutation.id is not None
        return (
            self.connection.execute(
                """SELECT 1 FROM outbox WHERE account_id=? AND entity_type=? AND entity_id=?
            AND id>? LIMIT 1""",
                (mutation.account_id, mutation.entity_type.value, mutation.entity_id, mutation.id),
            ).fetchone()
            is not None
        )

    def retry_mutation_without_etag(self, mutation: PendingMutation) -> None:
        assert mutation.id is not None
        payload = dict(mutation.payload)
        payload.pop("etag", None)
        cursor = self.connection.execute(
            """UPDATE outbox SET payload=?,delivery_state=?,sending_started_at=NULL,
            attempts=attempts+1,last_error=NULL WHERE account_id=? AND id=? AND delivery_state=?""",
            (
                json.dumps(payload, separators=(",", ":"), sort_keys=True),
                OutboxDeliveryState.PENDING.value,
                mutation.account_id,
                mutation.id,
                OutboxDeliveryState.SENDING.value,
            ),
        )
        if cursor.rowcount != 1:
            raise RuntimeError(f"outbox mutation {mutation.id} is not sending")

    def discard_conflicted_change(
        self, account_id: str, entity_type: EntityType, entity_id: str
    ) -> None:
        """Allow the next pull to replace one rejected local change with Google's version."""
        later = self.connection.execute(
            """SELECT 1 FROM outbox WHERE account_id=? AND entity_type=? AND entity_id=?
            LIMIT 1""",
            (account_id, entity_type.value, entity_id),
        ).fetchone()
        if later is not None:
            raise ConflictError("Later queued changes must be delivered or resolved first")
        table = {
            EntityType.TASK_LIST: "task_lists",
            EntityType.TASK: "tasks",
            EntityType.CALENDAR: "calendars",
            EntityType.EVENT: "events",
        }.get(entity_type)
        if table is None:
            raise ValueError(f"cannot discard a {entity_type.value} conflict")
        row = self.connection.execute(
            f"SELECT * FROM {table} WHERE account_id=? AND id=?",  # noqa: S608
            (account_id, entity_id),
        ).fetchone()
        if row is None:
            raise NotFoundError(f"{entity_type.value} {entity_id!r} does not exist")
        self.connection.execute(
            f"UPDATE {table} SET dirty=0 WHERE account_id=? AND id=?",  # noqa: S608
            (account_id, entity_id),
        )
        parent_table, parent_id, prefix = (
            ("task_lists", row["list_id"], "tasks")
            if entity_type is EntityType.TASK
            else ("calendars", row["calendar_id"], "events")
            if entity_type is EntityType.EVENT
            else (None, None, None)
        )
        if parent_table is not None:
            parent = self.connection.execute(
                f"SELECT remote_id FROM {parent_table} WHERE account_id=? AND id=?",  # noqa: S608
                (account_id, parent_id),
            ).fetchone()
            if parent is not None and parent["remote_id"]:
                scope = f"{prefix}:{parent['remote_id']}"
                self.delete_cursor(account_id, scope)
                checkpoint = self.resumable_checkpoint(account_id, scope)
                if checkpoint is not None:
                    self.finish_checkpoint(
                        checkpoint[0], error="conflict resolution requires full pull"
                    )

    def enqueue(self, mutation: PendingMutation) -> int:
        cursor = self.connection.execute(
            """INSERT INTO outbox(id,account_id,entity_type,entity_id,operation,payload,created_at,
            attempts,last_error,delivery_state,request_id,sending_started_at)
            VALUES (?,?,?,?,?,?,?,?,?,?,?,?)""",
            (
                mutation.id,
                mutation.account_id,
                mutation.entity_type.value,
                mutation.entity_id,
                mutation.operation.value,
                json.dumps(mutation.payload, separators=(",", ":"), sort_keys=True),
                _iso(mutation.created_at),
                mutation.attempts,
                mutation.last_error,
                mutation.delivery_state.value,
                mutation.request_id,
                _iso(mutation.sending_started_at),
            ),
        )
        if cursor.lastrowid is None:
            raise RuntimeError("SQLite did not return an outbox id")
        return cursor.lastrowid

    def pending_mutations(
        self,
        account_id: str,
        *,
        limit: int = 100,
        delivery_state: OutboxDeliveryState | None = None,
    ) -> list[PendingMutation]:
        state_sql = " AND delivery_state=?" if delivery_state is not None else ""
        arguments: tuple[Any, ...] = (
            (account_id, delivery_state.value, limit)
            if delivery_state is not None
            else (account_id, limit)
        )
        rows = self.connection.execute(
            f"""SELECT * FROM outbox WHERE account_id=?{state_sql}
            ORDER BY id LIMIT ?""",  # noqa: S608
            arguments,
        )
        return [self._mutation_from_row(row) for row in rows]

    @staticmethod
    def _mutation_from_row(row: sqlite3.Row) -> PendingMutation:
        return PendingMutation(
            row["id"],
            row["account_id"],
            EntityType(row["entity_type"]),
            row["entity_id"],
            MutationOperation(row["operation"]),
            json.loads(row["payload"]),
            datetime.fromisoformat(row["created_at"]),
            row["attempts"],
            row["last_error"],
            OutboxDeliveryState(row["delivery_state"]),
            row["request_id"],
            _datetime(row["sending_started_at"]),
        )

    def pending_mutation_count(self, account_id: str) -> int:
        row = self.connection.execute(
            "SELECT COUNT(*) FROM outbox WHERE account_id=?", (account_id,)
        ).fetchone()
        return int(row[0])

    def get_mutation(self, account_id: str, mutation_id: int) -> PendingMutation | None:
        row = self.connection.execute(
            "SELECT * FROM outbox WHERE account_id=? AND id=?", (account_id, mutation_id)
        ).fetchone()
        return self._mutation_from_row(row) if row is not None else None

    def mark_mutation_sending(
        self,
        account_id: str,
        mutation_id: int,
        *,
        request_id: str | None,
        started_at: datetime,
    ) -> None:
        cursor = self.connection.execute(
            """UPDATE outbox SET delivery_state=?,request_id=?,sending_started_at=?,
            last_error=NULL WHERE account_id=? AND id=? AND delivery_state=?""",
            (
                OutboxDeliveryState.SENDING.value,
                request_id,
                _iso(started_at),
                account_id,
                mutation_id,
                OutboxDeliveryState.PENDING.value,
            ),
        )
        if cursor.rowcount != 1:
            raise RuntimeError(f"outbox mutation {mutation_id} is not pending")

    def reset_mutation_pending(self, account_id: str, mutation_id: int, error: str) -> None:
        self.connection.execute(
            """UPDATE outbox SET delivery_state=?,sending_started_at=NULL,
            attempts=attempts+1,last_error=? WHERE account_id=? AND id=?""",
            (
                OutboxDeliveryState.PENDING.value,
                error,
                account_id,
                mutation_id,
            ),
        )

    def complete_mutation(self, account_id: str, mutation_id: int) -> None:
        self.connection.execute(
            "DELETE FROM outbox WHERE account_id=? AND id=?", (account_id, mutation_id)
        )

    def fail_mutation(self, account_id: str, mutation_id: int, error: str) -> None:
        self.reset_mutation_pending(account_id, mutation_id, error)

    def set_cursor(self, cursor: SyncCursor) -> None:
        self.connection.execute(
            """INSERT INTO sync_cursors VALUES (?,?,?,?) ON CONFLICT(account_id,scope)
            DO UPDATE SET cursor=excluded.cursor,updated_at=excluded.updated_at""",
            (cursor.account_id, cursor.scope, cursor.cursor, _iso(cursor.updated_at)),
        )

    def get_cursor(self, account_id: str, scope: str) -> SyncCursor | None:
        row = self.connection.execute(
            "SELECT * FROM sync_cursors WHERE account_id=? AND scope=?", (account_id, scope)
        ).fetchone()
        return (
            SyncCursor(
                row["account_id"],
                row["scope"],
                row["cursor"],
                datetime.fromisoformat(row["updated_at"]),
            )
            if row
            else None
        )

    def delete_cursor(self, account_id: str, scope: str) -> None:
        self.connection.execute(
            "DELETE FROM sync_cursors WHERE account_id=? AND scope=?", (account_id, scope)
        )

    def start_checkpoint(self, account_id: str, scope: str) -> int:
        cursor = self.connection.execute(
            "INSERT INTO sync_checkpoints(account_id,scope,started_at) VALUES (?,?,?)",
            (account_id, scope, _iso(utc_now())),
        )
        if cursor.lastrowid is None:
            raise RuntimeError("SQLite did not return a checkpoint id")
        return cursor.lastrowid

    def finish_checkpoint(
        self, checkpoint_id: int, *, cursor: str | None = None, error: str | None = None
    ) -> None:
        self.connection.execute(
            "UPDATE sync_checkpoints SET completed_at=?,cursor=?,error=? WHERE id=?",
            (_iso(utc_now()), cursor, error, checkpoint_id),
        )

    def resumable_checkpoint(self, account_id: str, scope: str) -> tuple[int, str | None] | None:
        row = self.connection.execute(
            """SELECT id,cursor FROM sync_checkpoints
            WHERE account_id=? AND scope=? AND completed_at IS NULL
            ORDER BY id DESC LIMIT 1""",
            (account_id, scope),
        ).fetchone()
        return (row["id"], row["cursor"]) if row else None

    def save_checkpoint_page(self, checkpoint_id: int, page_token: str | None) -> None:
        self.connection.execute(
            "UPDATE sync_checkpoints SET cursor=?,error=NULL WHERE id=?",
            (page_token, checkpoint_id),
        )

    def add_conflict(self, conflict: Conflict) -> int:
        cursor = self.connection.execute(
            """INSERT INTO conflicts(account_id,entity_type,entity_id,local_payload,
            remote_payload,status,created_at,resolved_at) VALUES (?,?,?,?,?,?,?,?)""",
            (
                conflict.account_id,
                conflict.entity_type.value,
                conflict.entity_id,
                json.dumps(conflict.local_payload, sort_keys=True),
                json.dumps(conflict.remote_payload, sort_keys=True),
                conflict.status.value,
                _iso(conflict.created_at),
                _iso(conflict.resolved_at),
            ),
        )
        if cursor.lastrowid is None:
            raise RuntimeError("SQLite did not return a conflict id")
        return cursor.lastrowid

    def list_conflicts(self, account_id: str, *, open_only: bool = True) -> list[Conflict]:
        sql, args = "SELECT * FROM conflicts WHERE account_id=?", [account_id]
        if open_only:
            sql, args = sql + " AND status=?", [*args, ConflictStatus.OPEN.value]
        return [
            Conflict(
                row["id"],
                row["account_id"],
                EntityType(row["entity_type"]),
                row["entity_id"],
                json.loads(row["local_payload"]),
                json.loads(row["remote_payload"]),
                ConflictStatus(row["status"]),
                datetime.fromisoformat(row["created_at"]),
                _datetime(row["resolved_at"]),
            )
            for row in self.connection.execute(sql + " ORDER BY id", args)
        ]

    def get_conflict(self, account_id: str, conflict_id: int) -> Conflict | None:
        row = self.connection.execute(
            "SELECT * FROM conflicts WHERE account_id=? AND id=?", (account_id, conflict_id)
        ).fetchone()
        return (
            Conflict(
                row["id"],
                row["account_id"],
                EntityType(row["entity_type"]),
                row["entity_id"],
                json.loads(row["local_payload"]),
                json.loads(row["remote_payload"]),
                ConflictStatus(row["status"]),
                datetime.fromisoformat(row["created_at"]),
                _datetime(row["resolved_at"]),
            )
            if row
            else None
        )

    def resolve_conflict(self, account_id: str, conflict_id: int, status: ConflictStatus) -> None:
        if status is ConflictStatus.OPEN:
            raise ValueError("resolution status cannot be open")
        self.connection.execute(
            "UPDATE conflicts SET status=?,resolved_at=? WHERE account_id=? AND id=?",
            (status.value, _iso(utc_now()), account_id, conflict_id),
        )
