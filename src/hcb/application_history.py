"""Conflict resolution, delivery reconciliation, and undo/redo workflows."""

from __future__ import annotations

import json
from dataclasses import replace

from .application import Json, _ApplicationServiceBase
from .errors import ConflictError, NotFoundError
from .models import (
    Conflict,
    ConflictStatus,
    EntityType,
    MutationOperation,
    PendingMutation,
    utc_now,
)


class HistoryServiceMixin(_ApplicationServiceBase):
    def list_conflicts(self, account_id: str) -> tuple[Conflict, ...]:
        return tuple(self.storage.list_conflicts(account_id))

    def resolve_conflict(
        self,
        account_id: str,
        conflict_id: int,
        resolution: ConflictStatus | str,
        *,
        merged_payload: Json | None = None,
    ) -> Conflict:
        status = ConflictStatus(resolution)
        if status is ConflictStatus.OPEN:
            raise ValueError("conflict resolution cannot be open")
        conflict = next(
            (
                item
                for item in self.storage.list_conflicts(account_id, open_only=False)
                if item.id == conflict_id
            ),
            None,
        )
        if conflict is None:
            raise NotFoundError(f"Conflict {conflict_id} does not exist")
        if conflict.status is not ConflictStatus.OPEN:
            raise ConflictError(f"Conflict {conflict_id} is already resolved")
        if conflict.local_payload.get("kind") == "uncertain-delivery":
            raise ValueError(
                "uncertain delivery requires the explicit retry or delivered reconciliation action"
            )
        payload = conflict.local_payload if status is ConflictStatus.KEEP_LOCAL else merged_payload
        if status is ConflictStatus.MERGED and payload is None:
            raise ValueError("merged conflict resolution requires merged_payload")
        with self.storage.transaction():
            if status in {ConflictStatus.KEEP_LOCAL, ConflictStatus.MERGED}:
                retry_payload = dict(payload or {})
                retry_payload.pop("etag", None)
                mutation_id = conflict.remote_payload.get("mutation_id")
                if isinstance(mutation_id, int) and mutation_id > 0:
                    self.storage.enqueue(
                        PendingMutation(
                            mutation_id,
                            account_id,
                            conflict.entity_type,
                            conflict.entity_id,
                            MutationOperation.UPDATE,
                            retry_payload,
                        )
                    )
                else:
                    later = self.storage.connection.execute(
                        """SELECT 1 FROM outbox WHERE account_id=? AND entity_type=?
                        AND entity_id=? LIMIT 1""",
                        (account_id, conflict.entity_type.value, conflict.entity_id),
                    ).fetchone()
                    if later is not None:
                        raise ConflictError(
                            "Cannot preserve later queued change order for this older conflict"
                        )
                    self._enqueue(
                        account_id,
                        conflict.entity_type,
                        conflict.entity_id,
                        MutationOperation.UPDATE,
                        retry_payload,
                    )
            else:
                self.storage.discard_conflicted_change(
                    account_id, conflict.entity_type, conflict.entity_id
                )
            self.storage.resolve_conflict(account_id, conflict_id, status)
        return replace(conflict, status=status, resolved_at=utc_now())

    def resolve_uncertain_delivery(
        self,
        account_id: str,
        conflict_id: int,
        action: str,
        *,
        remote_id: str | None = None,
    ) -> Conflict:
        conflict = next(
            (
                item
                for item in self.storage.list_conflicts(account_id, open_only=False)
                if item.id == conflict_id
            ),
            None,
        )
        if conflict is None:
            raise NotFoundError(f"Conflict {conflict_id} does not exist")
        if conflict.status is not ConflictStatus.OPEN:
            raise ConflictError(f"Conflict {conflict_id} is already resolved")
        if conflict.local_payload.get("kind") != "uncertain-delivery":
            raise ValueError("conflict is not an uncertain-delivery create")
        mutation = conflict.local_payload.get("mutation")
        if not isinstance(mutation, dict):
            raise ValueError("uncertain-delivery conflict has invalid mutation data")
        operation = MutationOperation(str(mutation.get("operation")))
        entity_type = EntityType(str(mutation.get("entity_type")))
        entity_id = str(mutation.get("entity_id") or "")
        payload = mutation.get("payload")
        original_id = mutation.get("id")
        if (
            operation not in {MutationOperation.CREATE, MutationOperation.MOVE}
            or not entity_id
            or not isinstance(payload, dict)
            or (original_id is not None and (type(original_id) is not int or original_id <= 0))
        ):
            raise ValueError("uncertain-delivery conflict has invalid local intent")
        normalized = action.casefold()
        if normalized not in {"retry", "delivered"}:
            raise ValueError("action must be retry or delivered")
        if normalized == "delivered" and not remote_id:
            raise ValueError("delivered reconciliation requires --remote-id")

        status = ConflictStatus.KEEP_LOCAL if normalized == "retry" else ConflictStatus.KEEP_REMOTE
        with self.storage.transaction():
            if normalized == "retry":
                if original_id is None:
                    # Conflicts written before the original ID was recorded still
                    # need the create ahead of edits that were already queued.
                    first_id = self.storage.connection.execute(
                        "SELECT MIN(id) FROM outbox"
                    ).fetchone()[0]
                    original_id = first_id - 1 if first_id is not None else None
                self.storage.enqueue(
                    PendingMutation(
                        original_id,
                        account_id,
                        entity_type,
                        entity_id,
                        operation,
                        payload,
                    )
                )
            else:
                table = {
                    EntityType.TASK_LIST: "task_lists",
                    EntityType.TASK: "tasks",
                    EntityType.CALENDAR: "calendars",
                }.get(entity_type)
                if table is None:
                    raise ValueError("delivered reconciliation is unsupported for this entity")
                later_edit = self.storage.connection.execute(
                    """SELECT 1 FROM outbox WHERE account_id=? AND entity_type=? AND entity_id=?
                    LIMIT 1""",
                    (account_id, entity_type.value, entity_id),
                ).fetchone()
                cursor = self.storage.connection.execute(
                    f"""UPDATE {table} SET remote_id=?,dirty=?
                    WHERE account_id=? AND id=?""",  # noqa: S608
                    (remote_id, int(later_edit is not None), account_id, entity_id),
                )
                if cursor.rowcount != 1:
                    raise NotFoundError(f"{entity_type.value} {entity_id!r} does not exist")
            self.storage.resolve_conflict(account_id, conflict_id, status)
        return replace(conflict, status=status, resolved_at=utc_now())

    def _restore_snapshot(
        self, table: str, account_id: str, entity_id: str, snapshot: Json | None
    ) -> None:
        if snapshot is None:
            self.storage.connection.execute(
                f"DELETE FROM {table} WHERE account_id=? AND id=?",  # noqa: S608
                (account_id, entity_id),
            )
            return
        columns = tuple(snapshot)
        placeholders = ",".join("?" for _ in columns)
        self.storage.connection.execute(
            f"INSERT OR REPLACE INTO {table} ({','.join(columns)}) "  # noqa: S608
            f"VALUES ({placeholders})",
            tuple(snapshot.values()),
        )

    def _intent_change(self, account_id: str, *, redo: bool) -> int | None:
        state, next_state = ("undone", "applied") if redo else ("applied", "undone")
        order = "ASC" if redo else "DESC"
        row = self.storage.connection.execute(
            f"""SELECT * FROM intents WHERE account_id=? AND state=?
            ORDER BY id {order} LIMIT 1""",  # noqa: S608
            (account_id, state),
        ).fetchone()
        if row is None:
            return None
        entity_type = EntityType(row["entity_type"])
        table = {
            EntityType.TASK_LIST: "task_lists",
            EntityType.TASK: "tasks",
            EntityType.CALENDAR: "calendars",
            EntityType.EVENT: "events",
        }[entity_type]
        target_raw = row["after_payload"] if redo else row["before_payload"]
        target = json.loads(target_raw) if target_raw else None
        with self.storage.transaction():
            self._restore_snapshot(table, account_id, row["entity_id"], target)
            self.storage.connection.execute(
                "DELETE FROM outbox WHERE account_id=? AND entity_type=? AND entity_id=?",
                (account_id, entity_type.value, row["entity_id"]),
            )
            operation = (
                MutationOperation.DELETE
                if target is None
                else MutationOperation.CREATE
                if (row["before_payload"] is None and redo)
                else MutationOperation.UPDATE
            )
            # Undoing an unpushed create only cancels that create; there is
            # nothing at Google to delete.
            if not (target is None and row["before_payload"] is None and not redo):
                self._enqueue(
                    account_id,
                    entity_type,
                    row["entity_id"],
                    operation,
                    self._payload_for_snapshot(entity_type, target),
                )
            self.storage.connection.execute(
                "UPDATE intents SET state=? WHERE id=?", (next_state, row["id"])
            )
        return int(row["id"])

    @staticmethod
    def _payload_for_snapshot(entity_type: EntityType, snapshot: Json | None) -> Json:
        if snapshot is None:
            return {}
        remote_id = snapshot.get("remote_id")
        if entity_type is EntityType.TASK_LIST:
            return {"remote_id": remote_id, "body": {"title": snapshot["title"]}}
        if entity_type is EntityType.TASK:
            body: Json = {
                "title": snapshot["title"],
                "status": snapshot["status"],
            }
            if snapshot.get("notes") is not None:
                body["notes"] = snapshot["notes"]
            if snapshot.get("due") is not None:
                body["due"] = snapshot["due"]
            return {
                "remote_id": remote_id,
                "list_id": snapshot["list_id"],
                "body": body,
            }
        if entity_type is EntityType.CALENDAR:
            return {
                "remote_id": remote_id,
                "body": {
                    "summary": snapshot["summary"],
                    "description": snapshot.get("description"),
                    "timeZone": snapshot.get("time_zone"),
                },
            }
        return {
            "remote_id": remote_id,
            "calendar_id": snapshot["calendar_id"],
            "body": {
                "summary": snapshot["summary"],
                "start": {
                    snapshot["start_kind"]: snapshot["start_value"],
                    **(
                        {"timeZone": snapshot["start_time_zone"]}
                        if snapshot.get("start_time_zone")
                        else {}
                    ),
                },
                "end": {
                    snapshot["end_kind"]: snapshot["end_value"],
                    **(
                        {"timeZone": snapshot["end_time_zone"]}
                        if snapshot.get("end_time_zone")
                        else {}
                    ),
                },
            },
        }

    def undo(self, account_id: str) -> int | None:
        return self._intent_change(account_id, redo=False)

    def redo(self, account_id: str) -> int | None:
        return self._intent_change(account_id, redo=True)

    def diagnostics(self) -> Json:
        result = dict(self.storage.diagnostics())
        result["path"] = "<redacted>"
        result["accounts"] = len(self.storage.list_accounts())
        result["saved_searches"] = self.storage.connection.execute(
            "SELECT count(*) FROM saved_searches"
        ).fetchone()[0]
        result["orphaned_links"] = self.storage.connection.execute(
            """SELECT count(*) FROM task_event_links l
            LEFT JOIN tasks t ON t.account_id=l.account_id AND t.id=l.task_id
            LEFT JOIN events e ON e.account_id=l.account_id AND e.id=l.event_id
            WHERE t.id IS NULL OR t.deleted=1 OR e.id IS NULL OR e.deleted=1"""
        ).fetchone()[0]
        return result
