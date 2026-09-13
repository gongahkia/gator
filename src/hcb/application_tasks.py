"""Optimistic task-list and task workflows."""

from __future__ import annotations

from dataclasses import replace
from datetime import date, datetime

from .application import (
    BatchActionPreview,
    BatchMovePreview,
    TaskCompletionResult,
    _ApplicationServiceBase,
    _dirty,
    _id,
    _Unset,
)
from .errors import NotFoundError
from .models import (
    EntityType,
    Metadata,
    MutationOperation,
    NotesProjection,
    Task,
    TaskList,
    TaskPriority,
    TaskStatus,
    utc_now,
)
from .task_recurrence import (
    parse_task_recurrence_notes,
    serialize_task_notes,
    task_recurrence_successor,
)

_UNSET = _Unset()


class TaskServiceMixin(_ApplicationServiceBase):
    def create_task_list(
        self, account_id: str, title: str, *, position: int = 0, id: str | None = None
    ) -> TaskList:
        self._account(account_id)
        if not title.strip():
            raise ValueError("task list title is required")
        item = TaskList(
            id or _id(), account_id, title.strip(), position=position, metadata=_dirty(Metadata())
        )
        with self.storage.transaction():
            self.storage.upsert_task_list(item)
            self._enqueue(
                account_id,
                EntityType.TASK_LIST,
                item.id,
                MutationOperation.CREATE,
                {"body": {"title": item.title}},
            )
            self._intent(
                account_id,
                "create",
                EntityType.TASK_LIST,
                item.id,
                None,
                self._snapshot("task_lists", account_id, item.id),
            )
        return item

    def update_task_list(
        self,
        account_id: str,
        list_id: str,
        *,
        title: str | None = None,
        position: int | None = None,
    ) -> TaskList:
        current = self._require_task_list(account_id, list_id)
        if title is not None and not title.strip():
            raise ValueError("task list title is required")
        updated = replace(
            current,
            title=title.strip() if title is not None else current.title,
            position=position if position is not None else current.position,
            metadata=_dirty(current.metadata),
        )
        with self.storage.transaction():
            before = self._snapshot("task_lists", account_id, list_id)
            self.storage.upsert_task_list(updated)
            self._enqueue(
                account_id,
                EntityType.TASK_LIST,
                list_id,
                MutationOperation.UPDATE,
                {
                    "body": {"title": updated.title},
                    "etag": current.metadata.etag,
                    "remote_id": current.remote_id,
                },
            )
            self._intent(
                account_id,
                "update",
                EntityType.TASK_LIST,
                list_id,
                before,
                self._snapshot("task_lists", account_id, list_id),
            )
        return updated

    def delete_task_list(self, account_id: str, list_id: str) -> TaskList:
        current = self._require_task_list(account_id, list_id)
        deleted = replace(current, metadata=_dirty(current.metadata, deleted=True))
        with self.storage.transaction():
            before = self._snapshot("task_lists", account_id, list_id)
            self.storage.upsert_task_list(deleted)
            self.storage.connection.execute(
                """UPDATE tasks SET deleted=1,dirty=1,local_updated_at=?
                WHERE account_id=? AND list_id=?""",
                (utc_now().isoformat(), account_id, list_id),
            )
            self._enqueue(
                account_id,
                EntityType.TASK_LIST,
                list_id,
                MutationOperation.DELETE,
                {"remote_id": current.remote_id, "etag": current.metadata.etag},
            )
            self._intent(
                account_id,
                "delete",
                EntityType.TASK_LIST,
                list_id,
                before,
                self._snapshot("task_lists", account_id, list_id),
            )
        return deleted

    def _require_task_list(self, account_id: str, list_id: str) -> TaskList:
        item = self.storage.get_task_list(account_id, list_id)
        if item is None or item.metadata.deleted:
            raise NotFoundError(f"Task list {list_id!r} does not exist")
        return item

    def _require_task(self, account_id: str, task_id: str) -> Task:
        item = self.storage.get_task(account_id, task_id)
        if item is None or item.metadata.deleted:
            raise NotFoundError(f"Task {task_id!r} does not exist")
        return item

    def create_task(
        self,
        account_id: str,
        list_id: str,
        title: str,
        *,
        notes: str | None = None,
        due: date | None = None,
        due_time_zone: str | None = None,
        priority: TaskPriority | str = TaskPriority.NONE,
        parent_id: str | None = None,
        position: str | None = None,
        id: str | None = None,
    ) -> Task:
        self._require_task_list(account_id, list_id)
        if not title.strip():
            raise ValueError("task title is required")
        if isinstance(due, datetime):
            raise ValueError("task due values are date-only")
        self._validate_zone(due_time_zone)
        if due is None and due_time_zone is not None:
            raise ValueError("a due time zone requires a due date")
        if parent_id is not None:
            parent = self._require_task(account_id, parent_id)
            if parent.list_id != list_id:
                raise ValueError("a task parent must be in the same list")
        task = Task(
            id=id or _id(),
            account_id=account_id,
            list_id=list_id,
            title=title.strip(),
            notes=notes,
            due=due,
            parent_id=parent_id,
            position=position,
            metadata=_dirty(Metadata()),
            priority=TaskPriority(priority),
            due_time_zone=due_time_zone,
        )
        with self.storage.transaction():
            self.storage.upsert_task(task)
            if self.notes_projection(account_id) is NotesProjection.DISABLED and notes is not None:
                self.storage.set_private_task_note(account_id, task.id, notes)
            self._enqueue(
                account_id,
                EntityType.TASK,
                task.id,
                MutationOperation.CREATE,
                {
                    "list_id": list_id,
                    "body": self._task_body(task, self.notes_projection(account_id)),
                    "parent": parent_id,
                    "previous": position,
                },
            )
            self._intent(
                account_id,
                "create",
                EntityType.TASK,
                task.id,
                None,
                self._snapshot("tasks", account_id, task.id),
            )
        return task

    def update_task(
        self,
        account_id: str,
        task_id: str,
        *,
        title: str | None = None,
        notes: str | None | _Unset = _UNSET,
        due: date | None = None,
        clear_due: bool = False,
        due_time_zone: str | None = None,
        priority: TaskPriority | str | None = None,
    ) -> Task:
        current = self._require_task(account_id, task_id)
        if title is not None and not title.strip():
            raise ValueError("task title is required")
        if isinstance(due, datetime):
            raise ValueError("task due values are date-only")
        next_due = None if clear_due else (due if due is not None else current.due)
        next_zone = due_time_zone if due_time_zone is not None else current.due_time_zone
        if clear_due:
            next_zone = None
        self._validate_zone(next_zone)
        if next_due is None and next_zone is not None:
            raise ValueError("a due time zone requires a due date")
        updated = replace(
            current,
            title=title.strip() if title is not None else current.title,
            notes=current.notes if isinstance(notes, _Unset) else notes,
            due=next_due,
            due_time_zone=next_zone,
            priority=TaskPriority(priority) if priority is not None else current.priority,
            metadata=_dirty(current.metadata),
        )
        with self.storage.transaction():
            before = self._snapshot("tasks", account_id, task_id)
            self.storage.upsert_task(updated)
            if self.notes_projection(account_id) is NotesProjection.DISABLED and not isinstance(
                notes, _Unset
            ):
                self.storage.set_private_task_note(account_id, task_id, updated.notes)
            self._enqueue(
                account_id,
                EntityType.TASK,
                task_id,
                MutationOperation.UPDATE,
                {
                    "list_id": current.list_id,
                    "body": self._task_body(updated, self.notes_projection(account_id)),
                    "etag": current.metadata.etag,
                    "remote_id": current.remote_id,
                },
            )
            self._intent(
                account_id,
                "update",
                EntityType.TASK,
                task_id,
                before,
                self._snapshot("tasks", account_id, task_id),
            )
        return updated

    def complete_task(self, account_id: str, task_id: str, *, completed: bool = True) -> Task:
        current = self._require_task(account_id, task_id)
        now = utc_now() if completed else None
        updated = replace(
            current,
            status=TaskStatus.COMPLETED if completed else TaskStatus.NEEDS_ACTION,
            completed_at=now,
            metadata=_dirty(current.metadata),
        )
        with self.storage.transaction():
            before = self._snapshot("tasks", account_id, task_id)
            self.storage.upsert_task(updated)
            self._enqueue(
                account_id,
                EntityType.TASK,
                task_id,
                MutationOperation.UPDATE,
                {
                    "list_id": current.list_id,
                    "body": self._task_body(updated, self.notes_projection(account_id)),
                    "etag": current.metadata.etag,
                    "remote_id": current.remote_id,
                },
            )
            self._intent(
                account_id,
                "complete",
                EntityType.TASK,
                task_id,
                before,
                self._snapshot("tasks", account_id, task_id),
            )
            if completed:
                self._ensure_recurrence_successor(updated)
        return updated

    def _ensure_recurrence_successor(self, task: Task) -> Task | None:
        parsed = parse_task_recurrence_notes(task.notes or "")
        if parsed.state != "managed" or parsed.marker is None:
            return None
        successor = task_recurrence_successor(parsed.marker)
        if successor is None:
            return None
        for candidate in self.storage.list_tasks(task.account_id, include_deleted=True):
            marker = parse_task_recurrence_notes(candidate.notes or "").marker
            if marker and marker.occurrence_id == successor.occurrence_id:
                return candidate
        serialized = serialize_task_notes(parsed.user_notes, successor, parsed.reminder)
        if serialized.error:
            raise ValueError(serialized.error)
        return self.create_task(
            task.account_id,
            task.list_id,
            successor.template_title,
            notes=serialized.notes,
            due=date.fromisoformat(successor.template_due_date),
            due_time_zone=successor.time_zone,
            priority=successor.template_priority,
        )

    def reconcile_task_recurrence(self, account_id: str) -> tuple[Task, ...]:
        created: list[Task] = []
        with self.storage.transaction():
            for task in self.storage.list_tasks(account_id):
                if task.status is TaskStatus.COMPLETED:
                    successor = self._ensure_recurrence_successor(task)
                    if successor and successor.id != task.id:
                        created.append(successor)
        return tuple(created)

    def complete_tasks(
        self, account_id: str, task_ids: list[str], *, completed: bool = True
    ) -> tuple[Task, ...]:
        return self.complete_tasks_detailed(account_id, task_ids, completed=completed).tasks

    def complete_tasks_detailed(
        self, account_id: str, task_ids: list[str], *, completed: bool = True
    ) -> TaskCompletionResult:
        """Complete tasks and expose any managed recurrence successors to a local UI."""
        preview = self.preview_task_completion(account_id, task_ids, completed=completed)
        completed_tasks: list[Task] = []
        successors: list[Task] = []
        with self.storage.transaction():
            for task in preview.items:
                if not isinstance(task, Task):
                    continue
                completed_task = self.complete_task(account_id, task.id, completed=completed)
                completed_tasks.append(completed_task)
                if completed:
                    successor = self._ensure_recurrence_successor(completed_task)
                    if successor is not None and successor.id != completed_task.id:
                        successors.append(successor)
        return TaskCompletionResult(tuple(completed_tasks), tuple(successors))

    def delete_tasks(self, account_id: str, task_ids: list[str]) -> tuple[Task, ...]:
        preview = self.preview_task_deletion(account_id, task_ids)
        with self.storage.transaction():
            return tuple(
                self.delete_task(account_id, task.id)
                for task in preview.items
                if isinstance(task, Task)
            )

    def _batch_tasks(self, account_id: str, task_ids: list[str]) -> tuple[Task, ...]:
        ids = tuple(dict.fromkeys(task_ids))
        if not ids:
            raise ValueError("select at least one task")
        return tuple(self._require_task(account_id, task_id) for task_id in ids)

    def preview_task_completion(
        self, account_id: str, task_ids: list[str], *, completed: bool
    ) -> BatchActionPreview:
        return BatchActionPreview(
            "task",
            "complete" if completed else "reopen",
            self._batch_tasks(account_id, task_ids),
        )

    def preview_task_deletion(self, account_id: str, task_ids: list[str]) -> BatchActionPreview:
        return BatchActionPreview("task", "delete", self._batch_tasks(account_id, task_ids))

    def preview_task_move(
        self, account_id: str, task_ids: list[str], list_id: str
    ) -> BatchMovePreview:
        """Validate a task batch before moving every target to a list's top level."""
        destination = self._require_task_list(account_id, list_id)
        targets = self._batch_tasks(account_id, task_ids)
        selected = {task.id for task in targets}
        all_tasks = {task.id: task for task in self.storage.list_tasks(account_id)}

        for task in targets:
            parent_id = task.parent_id
            seen = {task.id}
            while parent_id:
                if parent_id in seen:
                    raise ValueError("task hierarchy contains a parent cycle")
                if parent_id in selected:
                    raise ValueError(
                        "a task batch move cannot include both a parent and its subtask"
                    )
                seen.add(parent_id)
                parent = all_tasks.get(parent_id)
                if parent is None:
                    break
                parent_id = parent.parent_id

        if any(task.list_id != destination.id for task in targets):
            children = {task.parent_id for task in all_tasks.values() if task.parent_id}
            parent_ids = selected.intersection(children)
            if parent_ids:
                raise ValueError(
                    "a cross-list batch move cannot include tasks with subtasks; "
                    "move the hierarchy one task at a time"
                )
        return BatchMovePreview("task", destination.id, targets)

    def move_tasks(self, account_id: str, task_ids: list[str], list_id: str) -> tuple[Task, ...]:
        """Move validated task leaves serially to the destination list's top level."""
        preview = self.preview_task_move(account_id, task_ids, list_id)
        with self.storage.transaction():
            return tuple(
                self.move_task(account_id, task.id, list_id=preview.destination_id)
                for task in preview.items
                if isinstance(task, Task)
            )

    def move_task(
        self,
        account_id: str,
        task_id: str,
        *,
        list_id: str | None = None,
        parent_id: str | None = None,
        previous_id: str | None = None,
    ) -> Task:
        current = self._require_task(account_id, task_id)
        destination = list_id or current.list_id
        self._require_task_list(account_id, destination)
        if parent_id == task_id:
            raise ValueError("a task cannot parent itself")
        if parent_id:
            parent = self._require_task(account_id, parent_id)
            if parent.list_id != destination:
                raise ValueError("a task parent must be in the destination list")
            cursor = parent
            seen = {task_id}
            while cursor.parent_id:
                if cursor.parent_id in seen:
                    raise ValueError("task move would create a parent cycle")
                seen.add(cursor.parent_id)
                cursor = self._require_task(account_id, cursor.parent_id)
        if previous_id:
            previous = self._require_task(account_id, previous_id)
            if previous.list_id != destination or previous.parent_id != parent_id:
                raise ValueError("previous task must be a sibling in the destination list")
        updated = replace(
            current,
            list_id=destination,
            parent_id=parent_id,
            position=previous_id,
            metadata=_dirty(current.metadata),
        )
        with self.storage.transaction():
            before = self._snapshot("tasks", account_id, task_id)
            self.storage.upsert_task(updated)
            self._enqueue(
                account_id,
                EntityType.TASK,
                task_id,
                MutationOperation.MOVE,
                {
                    "source_list_id": current.list_id,
                    "list_id": destination,
                    "parent": parent_id,
                    "previous": previous_id,
                    "remote_id": current.remote_id,
                    "body": self._task_body(updated, self.notes_projection(account_id)),
                },
            )
            self._intent(
                account_id,
                "move",
                EntityType.TASK,
                task_id,
                before,
                self._snapshot("tasks", account_id, task_id),
            )
        return updated

    reparent_task = move_task
    reorder_task = move_task

    def delete_task(self, account_id: str, task_id: str) -> Task:
        current = self._require_task(account_id, task_id)
        deleted = replace(current, metadata=_dirty(current.metadata, deleted=True))
        with self.storage.transaction():
            before = self._snapshot("tasks", account_id, task_id)
            self.storage.upsert_task(deleted)
            self._enqueue(
                account_id,
                EntityType.TASK,
                task_id,
                MutationOperation.DELETE,
                {
                    "list_id": current.list_id,
                    "remote_id": current.remote_id,
                    "etag": current.metadata.etag,
                },
            )
            self._intent(
                account_id,
                "delete",
                EntityType.TASK,
                task_id,
                before,
                self._snapshot("tasks", account_id, task_id),
            )
        return deleted
