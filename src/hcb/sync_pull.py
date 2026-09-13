"""Remote pagination and mirror-refresh workflows for synchronization."""

from __future__ import annotations

from collections.abc import Callable
from concurrent.futures import Future, ThreadPoolExecutor
from dataclasses import replace
from datetime import datetime, timedelta
from functools import partial
from types import TracebackType

from .errors import GoogleApiError
from .google_client import Json, Page, rfc3339
from .models import Calendar, Event, NotesProjection, SyncCursor, TaskList
from .sync import (
    SyncResult,
    _parse_datetime,
    _RetryContext,
    _SyncEngineBase,
    calendar_from_google,
    event_from_google,
    task_from_google,
    task_list_from_google,
)


class _FirstPagePrefetch:
    def __init__(self, requests: dict[str, Callable[[], Page]], workers: int) -> None:
        self._pending = iter(requests.items())
        self._workers = min(workers, len(requests))
        self._executor: ThreadPoolExecutor | None = None
        self._futures: dict[str, Future[Page]] = {}
        self._active: Future[Page] | None = None

    def __enter__(self) -> _FirstPagePrefetch:
        if self._workers > 1:
            self._executor = ThreadPoolExecutor(
                max_workers=self._workers, thread_name_prefix="hcb-pull"
            )
            self.replenish()
        return self

    def take(self, scope: str) -> Future[Page] | None:
        self._active = self._futures.pop(scope, None)
        return self._active

    def replenish(self) -> None:
        self._active = None
        if self._executor is None:
            return
        while len(self._futures) < self._workers:
            try:
                scope, fetch = next(self._pending)
            except StopIteration:
                return
            self._futures[scope] = self._executor.submit(fetch)

    def __exit__(
        self,
        exc_type: type[BaseException] | None,
        exc: BaseException | None,
        traceback: TracebackType | None,
    ) -> None:
        if self._executor is not None:
            if self._active is not None:
                self._active.cancel()
            for future in self._futures.values():
                future.cancel()
            self._executor.shutdown(wait=True, cancel_futures=True)


class PullSyncMixin(_SyncEngineBase):
    def _prefetch_first_pages(self, requests: dict[str, Callable[[], Page]]) -> _FirstPagePrefetch:
        workers = self.pull_workers if getattr(self.gateway, "parallel_reads_safe", False) else 0
        return _FirstPagePrefetch(requests, workers)

    def _task_updated_min(self, account_id: str, remote_list_id: str) -> str | None:
        cursor = self.storage.get_cursor(account_id, f"tasks:{remote_list_id}")
        if cursor and cursor.cursor:
            last = _parse_datetime(cursor.cursor)
            if last:
                return rfc3339(last - timedelta(minutes=5))
        return None

    def _paged(
        self,
        account_id: str,
        scope: str,
        fetch: Callable[[str | None], Page],
        apply: Callable[[Json], None],
        *,
        final_cursor_scope: str | None = None,
        context: _RetryContext | None = None,
        on_complete: Callable[[], None] | None = None,
    ) -> SyncResult:
        context = context or self._retry_context()
        resumed = self.storage.resumable_checkpoint(account_id, scope)
        checkpoint, token = resumed or (self.storage.start_checkpoint(account_id, scope), None)
        pages = pulled = 0
        try:
            while True:

                def fetch_current_page(token: str | None = token) -> Page:
                    return fetch(token)

                page = self._retry_call(f"Fetching {scope}", fetch_current_page, context)
                with self.storage.transaction():
                    for item in page.items:
                        apply(item)
                    pages += 1
                    pulled += len(page.items)
                    token = page.next_page_token
                    if token is not None:
                        self.storage.save_checkpoint_page(checkpoint, token)
                    else:
                        if on_complete is not None:
                            on_complete()
                        if final_cursor_scope is not None:
                            self.storage.set_cursor(
                                SyncCursor(account_id, final_cursor_scope, page.next_sync_token)
                            )
                        self.storage.finish_checkpoint(checkpoint, cursor=page.next_sync_token)
                if token is None:
                    break
        except Exception as exc:
            # The incomplete checkpoint deliberately retains its next page token.
            self.storage.connection.execute(
                "UPDATE sync_checkpoints SET error=? WHERE id=?", (str(exc), checkpoint)
            )
            raise
        return SyncResult(pages=pages, pulled=pulled)

    def sync_task_lists(
        self,
        account_id: str,
        *,
        progress: Callable[[str], None] | None = None,
        context: _RetryContext | None = None,
    ) -> SyncResult:
        context = context or self._retry_context(progress)

        def apply(item: Json) -> None:
            existing = self.storage.get_task_list_by_remote(account_id, str(item["id"]))
            if existing is not None and existing.metadata.dirty:
                return
            incoming = task_list_from_google(
                account_id, item, local_id=existing.id if existing else None
            )
            if existing is not None and (
                replace(
                    incoming,
                    metadata=replace(
                        incoming.metadata,
                        local_updated_at=existing.metadata.local_updated_at,
                    ),
                )
                == existing
            ):
                return
            self.storage.upsert_task_list(incoming)

        result = self._paged(
            account_id,
            "task-lists",
            lambda token: self.gateway.list_task_lists(page_token=token),
            apply,
            context=context,
        )
        report = context.progress
        task_lists = [item for item in self.storage.list_task_lists(account_id) if item.remote_id]
        requests: dict[str, Callable[[], Page]] = {}
        if self.pull_workers > 1 and getattr(self.gateway, "parallel_reads_safe", False):
            for task_list in task_lists:
                assert task_list.remote_id is not None
                remote_id = task_list.remote_id
                if self.storage.resumable_checkpoint(account_id, f"tasks:{remote_id}"):
                    continue
                updated_min = self._task_updated_min(account_id, remote_id)
                requests[task_list.id] = partial(
                    self.gateway.list_tasks, remote_id, updated_min=updated_min
                )
        with self._prefetch_first_pages(requests) as prefetch:
            for index, task_list in enumerate(task_lists, start=1):
                report(f"Fetching tasks {index}/{len(task_lists)}")
                result = result.plus(
                    self.sync_tasks(
                        account_id,
                        task_list,
                        context=context,
                        first_page=prefetch.take(task_list.id),
                    )
                )
                prefetch.replenish()
        return result

    def sync_tasks(
        self,
        account_id: str,
        task_list: TaskList,
        *,
        context: _RetryContext | None = None,
        first_page: Future[Page] | None = None,
    ) -> SyncResult:
        context = context or self._retry_context()
        assert task_list.remote_id is not None
        remote_list_id = task_list.remote_id
        scope = f"tasks:{remote_list_id}"
        updated_min = self._task_updated_min(account_id, remote_list_id)
        completed_at = rfc3339(self.now())
        preserve_local_notes = (
            self.storage.get_notes_projection(account_id) is NotesProjection.DISABLED
        )

        def fetch(token: str | None) -> Page:
            nonlocal first_page
            if token is None and first_page is not None:
                pending, first_page = first_page, None
                return pending.result()
            return self.gateway.list_tasks(
                remote_list_id, page_token=token, updated_min=updated_min
            )

        def apply(item: Json) -> None:
            existing = self.storage.get_task_by_remote(account_id, str(item["id"]))
            if existing is None or not existing.metadata.dirty:
                incoming = task_from_google(
                    account_id,
                    task_list.id,
                    item,
                    local_id=existing.id if existing else None,
                )
                if preserve_local_notes and existing is not None:
                    incoming = replace(incoming, notes=existing.notes)
                self.storage.upsert_task(incoming)

        result = self._paged(
            account_id,
            scope,
            fetch,
            apply,
            context=context,
        )
        self.storage.set_cursor(SyncCursor(account_id, scope, completed_at))
        return result

    def sync_calendars(
        self,
        account_id: str,
        *,
        progress: Callable[[str], None] | None = None,
        context: _RetryContext | None = None,
    ) -> SyncResult:
        context = context or self._retry_context(progress)

        def apply(item: Json) -> None:
            existing = self.storage.get_calendar_by_remote(account_id, str(item["id"]))
            if existing is None or not existing.metadata.dirty:
                self.storage.upsert_calendar(
                    calendar_from_google(
                        account_id, item, local_id=existing.id if existing else None
                    )
                )

        scope = "calendar-list"
        cursor = self.storage.get_cursor(account_id, scope)
        reset = False
        while True:
            initial_token = cursor.cursor if cursor else None

            def fetch(token: str | None, *, initial: str | None = initial_token) -> Page:
                return self.gateway.list_calendars(page_token=token, sync_token=initial)

            try:
                result = self._paged(
                    account_id,
                    scope,
                    fetch,
                    apply,
                    final_cursor_scope=scope,
                    context=context,
                )
                break
            except GoogleApiError as exc:
                if exc.status != 410 or reset:
                    raise
                with self.storage.transaction():
                    self.storage.delete_cursor(account_id, scope)
                    stale = self.storage.resumable_checkpoint(account_id, scope)
                    if stale:
                        self.storage.finish_checkpoint(stale[0], error="sync token expired")
                cursor = None
                reset = True
        report = context.progress
        calendars = [
            item
            for item in self.storage.list_calendars(account_id)
            if item.remote_id and item.selected
        ]
        requests: dict[str, Callable[[], Page]] = {}
        if self.pull_workers > 1 and getattr(self.gateway, "parallel_reads_safe", False):
            for calendar in calendars:
                assert calendar.remote_id is not None
                remote_id = calendar.remote_id
                if self.storage.resumable_checkpoint(account_id, f"events:{remote_id}"):
                    continue
                cursor = self.storage.get_cursor(account_id, f"events:{remote_id}")
                sync_token = cursor.cursor if cursor else None
                requests[calendar.id] = partial(
                    self.gateway.list_events, remote_id, sync_token=sync_token, single_events=False
                )
        with self._prefetch_first_pages(requests) as prefetch:
            for index, calendar in enumerate(calendars, start=1):
                report(f"Fetching calendar {index}/{len(calendars)}")
                result = result.plus(
                    self.sync_events(
                        account_id,
                        calendar,
                        context=context,
                        first_page=prefetch.take(calendar.id),
                    )
                )
                prefetch.replenish()
        return result

    def sync_events(
        self,
        account_id: str,
        calendar: Calendar,
        *,
        context: _RetryContext | None = None,
        first_page: Future[Page] | None = None,
    ) -> SyncResult:
        context = context or self._retry_context()
        assert calendar.remote_id is not None
        remote_calendar_id = calendar.remote_id
        scope = f"events:{remote_calendar_id}"
        cursor = self.storage.get_cursor(account_id, scope)

        def apply(item: Json) -> None:
            existing = self.storage.get_event_by_remote(account_id, str(item["id"]))
            if existing is not None:
                self.storage.mark_calendar_event_seen(account_id, calendar.id, existing.id)
            affects_instance_cache = bool(
                item.get("recurrence")
                or item.get("recurringEventId")
                or item.get("originalStartTime")
                or (existing and (existing.recurrence or existing.is_occurrence))
            )
            if item.get("status") == "cancelled" and ("start" not in item or "end" not in item):
                if existing and not existing.metadata.dirty:
                    self.storage.upsert_event(
                        replace(existing, metadata=replace(existing.metadata, deleted=True))
                    )
                if affects_instance_cache:
                    self.storage.mark_instance_ranges_stale(
                        account_id, calendar.id, reason="remote-recurring-event-changed"
                    )
                return
            if existing is None or not existing.metadata.dirty:
                self.storage.upsert_event(
                    event_from_google(
                        account_id,
                        calendar.id,
                        item,
                        local_id=existing.id if existing else None,
                    )
                )
            if affects_instance_cache:
                self.storage.mark_instance_ranges_stale(
                    account_id, calendar.id, reason="remote-recurring-event-changed"
                )

        reset = False
        while True:
            sync_token = cursor.cursor if cursor else None

            def fetch(token: str | None, *, initial: str | None = sync_token) -> Page:
                nonlocal first_page
                if token is None and first_page is not None:
                    pending, first_page = first_page, None
                    return pending.result()
                return self.gateway.list_events(
                    remote_calendar_id,
                    page_token=token,
                    sync_token=initial,
                    single_events=False,
                )

            try:
                return self._paged(
                    account_id,
                    scope,
                    fetch,
                    apply,
                    final_cursor_scope=scope,
                    context=context,
                    on_complete=lambda: self.storage.finish_calendar_refresh(
                        account_id, calendar.id
                    ),
                )
            except GoogleApiError as exc:
                if exc.status != 410 or reset:
                    raise
                # A Calendar sync token expires independently of every other calendar.
                with self.storage.transaction():
                    self.storage.delete_cursor(account_id, scope)
                    self.storage.clear_calendar_mirror(account_id, calendar.id)
                    stale = self.storage.resumable_checkpoint(account_id, scope)
                    if stale:
                        self.storage.finish_checkpoint(stale[0], error="sync token expired")
                cursor = None
                reset = True

    def refresh_occurrences(
        self, account_id: str, calendar_id: str, start: datetime, end: datetime
    ) -> list[Event]:
        """Fetch an explicit remote range and persist recurring instances locally."""
        with self.sync_ownership():
            return self._refresh_occurrences_owned(account_id, calendar_id, start, end)

    def _refresh_occurrences_owned(
        self, account_id: str, calendar_id: str, start: datetime, end: datetime
    ) -> list[Event]:
        calendar = self.storage.get_calendar(account_id, calendar_id)
        if calendar is None or calendar.remote_id is None:
            raise ValueError("calendar is not synchronized")
        token: str | None = None
        result: list[Event] = []
        while True:
            page = self.gateway.list_events(
                calendar.remote_id,
                page_token=token,
                time_min=rfc3339(start),
                time_max=rfc3339(end),
                single_events=True,
            )
            for item in page.items:
                if "start" in item and "end" in item and item.get("recurringEventId"):
                    result.append(event_from_google(account_id, calendar.id, item, derived=True))
            token = page.next_page_token
            if token is None:
                with self.storage.transaction():
                    self.storage.replace_cached_instances(
                        account_id, calendar.id, start, end, result
                    )
                return result

    def load_occurrences(
        self, account_id: str, calendar_id: str, start: datetime, end: datetime
    ) -> list[Event]:
        """Backward-compatible name for the explicit, caching occurrence refresh."""
        return self.refresh_occurrences(account_id, calendar_id, start, end)
