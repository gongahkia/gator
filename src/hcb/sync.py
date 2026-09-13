"""Resumable, account-partitioned synchronization with Google."""

from __future__ import annotations

import errno
import os
import random
import time
from collections.abc import Callable, Iterator
from contextlib import contextmanager
from dataclasses import dataclass
from datetime import date, datetime
from threading import RLock
from typing import TypeVar

from .errors import (
    ConfigurationError,
    GoogleApiError,
    RequestNotSentError,
    StorageError,
    SyncBusyError,
    TransientTransportError,
)
from .google_client import GoogleGateway, Json
from .models import (
    Calendar,
    DateTimeKind,
    Event,
    EventDateTime,
    EventStatus,
    Metadata,
    PendingMutation,
    ReminderOverride,
    Task,
    TaskList,
    TaskStatus,
    utc_now,
)
from .storage import Storage

RATE_LIMIT_REASONS = {"rateLimitExceeded", "userRateLimitExceeded", "quotaExceeded"}
T = TypeVar("T")


def _ignore_progress(_: str) -> None:
    """Default progress callback for non-interactive callers."""


def _not_cancelled() -> bool:
    return False


@dataclass(frozen=True, slots=True)
class _RetryContext:
    progress: Callable[[str], None]
    cancelled: Callable[[], bool]
    cancel_hint: str


class _RetryCancelled(Exception):
    """Internal signal used to leave a retry wait without another request."""


class _RetryExhausted(Exception):
    """Internal signal carrying the final retryable failure."""

    def __init__(self, operation: str, error: Exception, retry_after: float) -> None:
        super().__init__(f"{operation} could not complete after bounded retries: {error}")
        self.operation = operation
        self.error = error
        self.retry_after = retry_after


@dataclass(frozen=True, slots=True)
class SyncResult:
    pages: int = 0
    pulled: int = 0
    pushed: int = 0
    conflicts: int = 0
    retry_pending: bool = False
    retry_after: float | None = None
    retry_exhausted: bool = False
    cancelled: bool = False
    retry_message: str | None = None

    def plus(self, other: SyncResult) -> SyncResult:
        delays = [value for value in (self.retry_after, other.retry_after) if value is not None]
        return SyncResult(
            pages=self.pages + other.pages,
            pulled=self.pulled + other.pulled,
            pushed=self.pushed + other.pushed,
            conflicts=self.conflicts + other.conflicts,
            retry_pending=self.retry_pending or other.retry_pending,
            retry_after=max(delays) if delays else None,
            retry_exhausted=self.retry_exhausted or other.retry_exhausted,
            cancelled=self.cancelled or other.cancelled,
            retry_message=other.retry_message or self.retry_message,
        )


def _parse_datetime(value: str | None) -> datetime | None:
    if not value:
        return None
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def _metadata(item: Json, *, deleted: bool = False) -> Metadata:
    return Metadata(
        etag=item.get("etag"),
        remote_updated_at=_parse_datetime(item.get("updated")),
        deleted=deleted,
    )


def _event_time(value: Json) -> EventDateTime:
    if "date" in value:
        return EventDateTime(DateTimeKind.DATE, date.fromisoformat(value["date"]))
    parsed = _parse_datetime(value.get("dateTime"))
    if parsed is None:
        raise ValueError("Google event has no date or dateTime")
    return EventDateTime(DateTimeKind.DATETIME, parsed, value.get("timeZone"))


def task_list_from_google(account_id: str, item: Json, *, local_id: str | None = None) -> TaskList:
    remote_id = str(item["id"])
    return TaskList(
        local_id or remote_id,
        account_id,
        item.get("title", ""),
        remote_id,
        metadata=_metadata(item, deleted=bool(item.get("deleted"))),
    )


def task_from_google(
    account_id: str, list_id: str, item: Json, *, local_id: str | None = None
) -> Task:
    remote_id = str(item["id"])
    due = _parse_datetime(item.get("due"))
    return Task(
        id=local_id or remote_id,
        account_id=account_id,
        list_id=list_id,
        title=item.get("title", ""),
        notes=item.get("notes"),
        status=TaskStatus(item.get("status", TaskStatus.NEEDS_ACTION.value)),
        due=due.date() if due else None,
        completed_at=_parse_datetime(item.get("completed")),
        parent_id=item.get("parent"),
        position=item.get("position"),
        remote_id=remote_id,
        metadata=_metadata(item, deleted=bool(item.get("deleted"))),
    )


def calendar_from_google(account_id: str, item: Json, *, local_id: str | None = None) -> Calendar:
    remote_id = str(item["id"])
    return Calendar(
        id=local_id or remote_id,
        account_id=account_id,
        summary=item.get("summary", ""),
        remote_id=remote_id,
        description=item.get("description"),
        time_zone=item.get("timeZone"),
        color=item.get("backgroundColor") or item.get("colorId"),
        foreground_color=item.get("foregroundColor"),
        location=item.get("location"),
        summary_override=item.get("summaryOverride"),
        hidden=bool(item.get("hidden", False)),
        selected=bool(item.get("selected", True)),
        metadata=_metadata(item, deleted=bool(item.get("deleted"))),
        default_reminders=tuple(
            ReminderOverride(str(value.get("method", "")), int(value.get("minutes", 0)))
            for value in item.get("defaultReminders") or ()
            if isinstance(value, dict)
        ),
        notification_settings=tuple(
            value
            for value in ((item.get("notificationSettings") or {}).get("notifications") or ())
            if isinstance(value, dict)
        ),
    )


def event_from_google(
    account_id: str,
    calendar_id: str,
    item: Json,
    *,
    local_id: str | None = None,
    derived: bool = False,
) -> Event:
    remote_id = str(item["id"])
    original = item.get("originalStartTime") or {}
    occurrence_id = original.get("dateTime") or original.get("date")
    reminders = item.get("reminders") or {}
    attendees = tuple(value for value in item.get("attendees") or () if isinstance(value, dict))
    own_attendee = next((value for value in attendees if value.get("self")), None)
    return Event(
        id=local_id or (f"{remote_id}@{occurrence_id}" if occurrence_id else remote_id),
        account_id=account_id,
        calendar_id=calendar_id,
        summary=item.get("summary", ""),
        start=_event_time(item["start"]),
        end=_event_time(item["end"]),
        remote_id=remote_id,
        canonical_id=item.get("recurringEventId"),
        occurrence_id=occurrence_id,
        description=item.get("description"),
        location=item.get("location"),
        status=EventStatus(item.get("status", EventStatus.CONFIRMED.value)),
        recurrence=tuple(item.get("recurrence") or ()),
        derived=derived,
        metadata=_metadata(item, deleted=item.get("status") == "cancelled"),
        reminder_use_default=bool(reminders.get("useDefault", True)),
        reminder_overrides=tuple(
            ReminderOverride(str(value.get("method", "")), int(value.get("minutes", 0)))
            for value in reminders.get("overrides") or ()
            if isinstance(value, dict)
        ),
        attendees=attendees,
        attendee_response=own_attendee.get("responseStatus") if own_attendee else None,
        event_type=item.get("eventType"),
        transparency=item.get("transparency"),
        visibility=item.get("visibility"),
        color_id=item.get("colorId"),
        attachments=tuple(
            value for value in item.get("attachments") or () if isinstance(value, dict)
        ),
        conference=item.get("conferenceData")
        if isinstance(item.get("conferenceData"), dict)
        else None,
        guests_can_invite_others=item.get("guestsCanInviteOthers"),
        guests_can_modify=item.get("guestsCanModify"),
        guests_can_see_other_guests=item.get("guestsCanSeeOtherGuests"),
        anyone_can_add_self=item.get("anyoneCanAddSelf"),
        focus_time_properties=(
            item.get("focusTimeProperties")
            if isinstance(item.get("focusTimeProperties"), dict)
            else None
        ),
        out_of_office_properties=(
            item.get("outOfOfficeProperties")
            if isinstance(item.get("outOfOfficeProperties"), dict)
            else None
        ),
        working_location_properties=(
            item.get("workingLocationProperties")
            if isinstance(item.get("workingLocationProperties"), dict)
            else None
        ),
    )


def _clean_body(payload: Json) -> Json:
    return dict(payload.get("body", payload))


def _required_remote(value: object, entity: str) -> str:
    if not isinstance(value, str) or not value:
        raise ValueError(f"{entity} has no remote id")
    return value


class _SyncEngineBase:
    def __init__(
        self,
        storage: Storage,
        gateway: GoogleGateway,
        *,
        now: Callable[[], datetime] = utc_now,
        crash_hook: Callable[[str, PendingMutation], None] | None = None,
        max_retries: int = 4,
        base_retry_delay: float = 1.0,
        max_retry_delay: float = 32.0,
        random_source: Callable[[], float] = random.random,
        wait_for_retry: Callable[[float, Callable[[], bool]], bool] | None = None,
    ) -> None:
        if max_retries < 1:
            raise ValueError("max_retries must be at least one")
        if base_retry_delay <= 0 or max_retry_delay < base_retry_delay:
            raise ValueError("retry delays must be positive and ordered")
        self.storage = storage
        self.gateway = gateway
        self.now = now
        self.crash_hook = crash_hook or (lambda _phase, _mutation: None)
        self.max_retries = max_retries
        self.base_retry_delay = base_retry_delay
        self.max_retry_delay = max_retry_delay
        self.random_source = random_source
        self.wait_for_retry = wait_for_retry or self._wait_for_retry
        self._sync_guard = RLock()
        self._sync_depth = 0

    @contextmanager
    def sync_ownership(self) -> Iterator[None]:
        """Own this database's sync work across threads and processes."""
        if not self._sync_guard.acquire(blocking=False):
            raise SyncBusyError("Another HCB sync is active; try again when it finishes")
        try:
            if self._sync_depth:
                self._sync_depth += 1
                try:
                    yield
                finally:
                    self._sync_depth -= 1
                return
            try:
                import fcntl
            except ImportError as error:
                raise ConfigurationError("Process-level sync locking requires POSIX") from error
            database_path = self.storage.path.resolve()
            lock_path = database_path.with_name(database_path.name + ".sync.lock")
            flags = os.O_CREAT | os.O_RDWR | getattr(os, "O_CLOEXEC", 0)
            flags |= getattr(os, "O_NOFOLLOW", 0)
            try:
                descriptor = os.open(lock_path, flags, 0o600)
            except OSError as error:
                raise StorageError(f"Cannot open sync lock: {error}") from error
            try:
                try:
                    fcntl.flock(descriptor, fcntl.LOCK_EX | fcntl.LOCK_NB)
                except OSError as error:
                    if error.errno in {errno.EACCES, errno.EAGAIN}:
                        raise SyncBusyError(
                            "Another HCB sync is active; try again when it finishes"
                        ) from error
                    raise StorageError(f"Cannot acquire sync lock: {error}") from error
                self._sync_depth = 1
                try:
                    yield
                finally:
                    self._sync_depth = 0
                    fcntl.flock(descriptor, fcntl.LOCK_UN)
            finally:
                os.close(descriptor)
        finally:
            self._sync_guard.release()

    @staticmethod
    def _wait_for_retry(delay: float, cancelled: Callable[[], bool]) -> bool:
        """Wait in short intervals so a TUI cancellation is observed promptly."""
        deadline = time.monotonic() + delay
        while True:
            if cancelled():
                return False
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                return True
            time.sleep(min(0.1, remaining))

    @staticmethod
    def _transient(error: Exception) -> bool:
        if isinstance(error, (RequestNotSentError, TransientTransportError)):
            return True
        return isinstance(error, GoogleApiError) and (
            error.retryable or (error.status == 403 and error.reason in RATE_LIMIT_REASONS)
        )

    def _retry_delay(self, error: Exception, retry: int) -> float:
        retry_after = error.retry_after if isinstance(error, GoogleApiError) else None
        backoff = min(self.max_retry_delay, self.base_retry_delay * (2 ** (retry - 1)))
        jitter = min(1.0, max(0.0, float(self.random_source())))
        return float(max(retry_after or 0.0, backoff + jitter))

    @staticmethod
    def _check_cancel(context: _RetryContext) -> None:
        if context.cancelled():
            raise _RetryCancelled

    def _retry_call(
        self,
        operation: str,
        call: Callable[[], T],
        context: _RetryContext,
        *,
        safe_to_retry: Callable[[Exception], bool] | None = None,
    ) -> T:
        """Retry a transient request serially, without replaying unsafe creates."""
        retry = 0
        while True:
            self._check_cancel(context)
            try:
                return call()
            except (RequestNotSentError, TransientTransportError, GoogleApiError) as error:
                if not self._transient(error) or (
                    safe_to_retry is not None and not safe_to_retry(error)
                ):
                    raise
                if retry >= self.max_retries:
                    raise _RetryExhausted(
                        operation, error, self._retry_delay(error, retry + 1)
                    ) from error
                retry += 1
                delay = self._retry_delay(error, retry)
                context.progress(
                    f"{operation} temporarily failed; retrying {retry}/{self.max_retries} "
                    f"in {delay:.1f}s. {context.cancel_hint}"
                )
                if not self.wait_for_retry(delay, context.cancelled):
                    raise _RetryCancelled from None

    def _retry_context(
        self,
        progress: Callable[[str], None] | None = None,
        cancelled: Callable[[], bool] | None = None,
        cancel_hint: str = "Press Ctrl+C to cancel.",
    ) -> _RetryContext:
        return _RetryContext(progress or _ignore_progress, cancelled or _not_cancelled, cancel_hint)

    def sync(
        self,
        account_id: str,
        *,
        full_tasks: bool = False,
        progress: Callable[[str], None] | None = None,
        cancelled: Callable[[], bool] | None = None,
        cancel_hint: str = "Press Ctrl+C to cancel.",
    ) -> SyncResult:
        """Synchronize an account and report completed stages when requested."""
        with self.sync_ownership():
            return self._sync_owned(
                account_id,
                full_tasks=full_tasks,
                progress=progress,
                cancelled=cancelled,
                cancel_hint=cancel_hint,
            )

    def _sync_owned(
        self,
        account_id: str,
        *,
        full_tasks: bool = False,
        progress: Callable[[str], None] | None = None,
        cancelled: Callable[[], bool] | None = None,
        cancel_hint: str = "Press Ctrl+C to cancel.",
    ) -> SyncResult:
        if self.storage.get_account(account_id) is None:
            raise ValueError(f"unknown account {account_id!r}")
        if full_tasks:
            for item in self.storage.list_task_lists(account_id):
                if item.remote_id:
                    self.storage.delete_cursor(account_id, f"tasks:{item.remote_id}")
        context = self._retry_context(progress, cancelled, cancel_hint)
        result = SyncResult()
        try:
            self._check_cancel(context)
            context.progress("Sending local changes")
            result = self.flush_outbox(account_id, context=context)
            if result.retry_pending or result.cancelled:
                return result
            context.progress("Fetching task lists")
            result = result.plus(
                self.sync_task_lists(account_id, progress=context.progress, context=context)
            )
            context.progress("Fetching calendars")
            result = result.plus(
                self.sync_calendars(account_id, progress=context.progress, context=context)
            )
            context.progress("Finishing sync")
            return result
        except _RetryCancelled:
            return result.plus(
                SyncResult(
                    cancelled=True,
                    retry_message=(
                        "Sync cancelled. Local changes remain queued; run sync to resume."
                    ),
                )
            )
        except _RetryExhausted as error:
            return result.plus(
                SyncResult(
                    retry_pending=True,
                    retry_after=error.retry_after,
                    retry_exhausted=True,
                    retry_message=(
                        f"{error.operation} paused after bounded retries. "
                        "Local changes remain queued; run sync to resume."
                    ),
                )
            )

    # These operations are supplied by the pull and delivery mixins below.
    # Keeping their contracts here makes the coordinator's dependencies explicit.
    def flush_outbox(self, account_id: str, *, context: _RetryContext | None = None) -> SyncResult:
        raise NotImplementedError

    def sync_task_lists(
        self,
        account_id: str,
        *,
        progress: Callable[[str], None] | None = None,
        context: _RetryContext | None = None,
    ) -> SyncResult:
        raise NotImplementedError

    def sync_calendars(
        self,
        account_id: str,
        *,
        progress: Callable[[str], None] | None = None,
        context: _RetryContext | None = None,
    ) -> SyncResult:
        raise NotImplementedError


# These imports are intentionally late: pull and delivery implementations share
# the retry boundary above, while this module remains the stable public API.
from .sync_delivery import DeliverySyncMixin  # noqa: E402
from .sync_pull import PullSyncMixin  # noqa: E402


class SyncEngine(PullSyncMixin, DeliverySyncMixin):
    """Resumable, account-partitioned synchronization with Google."""

    def pull_only(self, account_id: str) -> tuple[SyncResult, SyncResult]:
        """Run the reminder process's pull mode under one sync owner."""
        with self.sync_ownership():
            return self.sync_task_lists(account_id), self.sync_calendars(account_id)
