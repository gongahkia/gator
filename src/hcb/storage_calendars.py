"""Calendar, event, cached-instance, and Drive metadata persistence."""

from __future__ import annotations

import json
import sqlite3
from datetime import UTC, date, datetime
from typing import Any

from .models import (
    Calendar,
    DateTimeKind,
    DriveFile,
    Event,
    EventDateTime,
    EventStatus,
    ReminderOverride,
    utc_now,
)
from .storage import _datetime, _iso, _metadata, _StorageCore


class CalendarEventRepository(_StorageCore):
    def upsert_calendar(self, calendar: Calendar) -> None:
        self.connection.execute(
            """INSERT INTO calendars(
                id,account_id,summary,remote_id,description,time_zone,color,foreground_color,
                location,summary_override,hidden,selected,etag,remote_updated_at,local_updated_at,
                deleted,dirty,default_reminders,notification_settings
            ) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
            ON CONFLICT(account_id,id) DO UPDATE SET summary=excluded.summary,
            remote_id=excluded.remote_id, description=excluded.description,
            time_zone=excluded.time_zone, color=excluded.color,
            foreground_color=excluded.foreground_color,location=excluded.location,
            summary_override=excluded.summary_override,hidden=excluded.hidden,
            selected=excluded.selected,
            etag=excluded.etag, remote_updated_at=excluded.remote_updated_at,
            local_updated_at=excluded.local_updated_at, deleted=excluded.deleted,
            dirty=excluded.dirty,default_reminders=excluded.default_reminders,
            notification_settings=excluded.notification_settings""",
            (
                calendar.id,
                calendar.account_id,
                calendar.summary,
                calendar.remote_id,
                calendar.description,
                calendar.time_zone,
                calendar.color,
                calendar.foreground_color,
                calendar.location,
                calendar.summary_override,
                calendar.hidden,
                calendar.selected,
                *self._meta_values(calendar.metadata),
                json.dumps(
                    [
                        {"method": item.method, "minutes": item.minutes}
                        for item in calendar.default_reminders
                    ]
                ),
                json.dumps(calendar.notification_settings),
            ),
        )

    def _calendar(self, row: sqlite3.Row) -> Calendar:
        return Calendar(
            id=row["id"],
            account_id=row["account_id"],
            summary=row["summary"],
            remote_id=row["remote_id"],
            description=row["description"],
            time_zone=row["time_zone"],
            color=row["color"],
            foreground_color=row["foreground_color"],
            location=row["location"],
            summary_override=row["summary_override"],
            hidden=bool(row["hidden"]),
            selected=bool(row["selected"]),
            metadata=_metadata(row),
            default_reminders=tuple(
                ReminderOverride(str(item["method"]), int(item["minutes"]))
                for item in json.loads(row["default_reminders"])
            ),
            notification_settings=tuple(json.loads(row["notification_settings"])),
        )

    def get_calendar(self, account_id: str, calendar_id: str) -> Calendar | None:
        row = self.connection.execute(
            "SELECT * FROM calendars WHERE account_id=? AND id=?", (account_id, calendar_id)
        ).fetchone()
        return self._calendar(row) if row else None

    def get_calendar_by_remote(self, account_id: str, remote_id: str) -> Calendar | None:
        row = self.connection.execute(
            "SELECT * FROM calendars WHERE account_id=? AND remote_id=?",
            (account_id, remote_id),
        ).fetchone()
        return self._calendar(row) if row else None

    def list_calendars(self, account_id: str, *, include_deleted: bool = False) -> list[Calendar]:
        sql = "SELECT * FROM calendars WHERE account_id=?"
        if not include_deleted:
            sql += " AND deleted=0"
        return [
            self._calendar(row)
            for row in self.connection.execute(sql + " ORDER BY summary", (account_id,))
        ]

    def delete_calendar(self, account_id: str, calendar_id: str) -> None:
        self.connection.execute(
            "DELETE FROM calendars WHERE account_id=? AND id=?", (account_id, calendar_id)
        )

    def upsert_event(self, event: Event) -> None:
        self.connection.execute(
            """INSERT INTO events(
                id,account_id,calendar_id,summary,start_kind,start_value,start_time_zone,
                end_kind,end_value,end_time_zone,remote_id,canonical_id,occurrence_id,
                description,location,status,recurrence,etag,remote_updated_at,local_updated_at,
                deleted,dirty,reminder_use_default,reminder_overrides,attendees,
                attendee_response,event_type,transparency,visibility,color_id,attachments,conference
                ,guests_can_invite_others,guests_can_modify,guests_can_see_other_guests,
                anyone_can_add_self,focus_time_properties,out_of_office_properties,
                working_location_properties,derived
            ) VALUES (
                ?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,
                ?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?
            )
            ON CONFLICT(account_id,id) DO UPDATE SET calendar_id=excluded.calendar_id,
            summary=excluded.summary, start_kind=excluded.start_kind,
            start_value=excluded.start_value, start_time_zone=excluded.start_time_zone,
            end_kind=excluded.end_kind, end_value=excluded.end_value,
            end_time_zone=excluded.end_time_zone, remote_id=excluded.remote_id,
            canonical_id=excluded.canonical_id, occurrence_id=excluded.occurrence_id,
            description=excluded.description, location=excluded.location,
            status=excluded.status, recurrence=excluded.recurrence, etag=excluded.etag,
            remote_updated_at=excluded.remote_updated_at,
            local_updated_at=excluded.local_updated_at, deleted=excluded.deleted,
            dirty=excluded.dirty,reminder_use_default=excluded.reminder_use_default,
            reminder_overrides=excluded.reminder_overrides,attendees=excluded.attendees,
            attendee_response=excluded.attendee_response,event_type=excluded.event_type,
            transparency=excluded.transparency,visibility=excluded.visibility,
            color_id=excluded.color_id,attachments=excluded.attachments,
            conference=excluded.conference,
            guests_can_invite_others=excluded.guests_can_invite_others,
            guests_can_modify=excluded.guests_can_modify,
            guests_can_see_other_guests=excluded.guests_can_see_other_guests,
            anyone_can_add_self=excluded.anyone_can_add_self,
            focus_time_properties=excluded.focus_time_properties,
            out_of_office_properties=excluded.out_of_office_properties,
            working_location_properties=excluded.working_location_properties,
            derived=excluded.derived""",
            (
                event.id,
                event.account_id,
                event.calendar_id,
                event.summary,
                event.start.kind.value,
                _iso(event.start.value),
                event.start.time_zone,
                event.end.kind.value,
                _iso(event.end.value),
                event.end.time_zone,
                event.remote_id,
                event.canonical_id,
                event.occurrence_id,
                event.description,
                event.location,
                event.status.value,
                json.dumps(event.recurrence),
                *self._meta_values(event.metadata),
                event.reminder_use_default,
                json.dumps(
                    [
                        {"method": item.method, "minutes": item.minutes}
                        for item in event.reminder_overrides
                    ]
                ),
                json.dumps(event.attendees),
                event.attendee_response,
                event.event_type,
                event.transparency,
                event.visibility,
                event.color_id,
                json.dumps(event.attachments),
                json.dumps(event.conference) if event.conference is not None else None,
                event.guests_can_invite_others,
                event.guests_can_modify,
                event.guests_can_see_other_guests,
                event.anyone_can_add_self,
                json.dumps(event.focus_time_properties)
                if event.focus_time_properties is not None
                else None,
                json.dumps(event.out_of_office_properties)
                if event.out_of_office_properties is not None
                else None,
                json.dumps(event.working_location_properties)
                if event.working_location_properties is not None
                else None,
                event.derived,
            ),
        )

    @staticmethod
    def _event_time(kind: str, value: str, zone: str | None) -> EventDateTime:
        parsed: date | datetime
        parsed = (
            datetime.fromisoformat(value)
            if kind == DateTimeKind.DATETIME
            else date.fromisoformat(value)
        )
        return EventDateTime(DateTimeKind(kind), parsed, zone)

    def _event(self, row: sqlite3.Row) -> Event:
        # canonical empty arrays need no JSON decoding; nonempty values still parse normally.
        recurrence = row["recurrence"]
        reminder_overrides = row["reminder_overrides"]
        attendees = row["attendees"]
        attachments = row["attachments"]
        return Event(
            id=row["id"],
            account_id=row["account_id"],
            calendar_id=row["calendar_id"],
            summary=row["summary"],
            start=self._event_time(row["start_kind"], row["start_value"], row["start_time_zone"]),
            end=self._event_time(row["end_kind"], row["end_value"], row["end_time_zone"]),
            remote_id=row["remote_id"],
            canonical_id=row["canonical_id"],
            occurrence_id=row["occurrence_id"],
            description=row["description"],
            location=row["location"],
            status=EventStatus(row["status"]),
            recurrence=() if recurrence == "[]" else tuple(json.loads(recurrence)),
            derived=bool(row["derived"]),
            metadata=_metadata(row),
            reminder_use_default=bool(row["reminder_use_default"]),
            reminder_overrides=(
                ()
                if reminder_overrides == "[]"
                else tuple(
                    ReminderOverride(str(item["method"]), int(item["minutes"]))
                    for item in json.loads(reminder_overrides)
                )
            ),
            attendees=() if attendees == "[]" else tuple(json.loads(attendees)),
            attendee_response=row["attendee_response"],
            event_type=row["event_type"],
            transparency=row["transparency"],
            visibility=row["visibility"],
            color_id=row["color_id"],
            attachments=() if attachments == "[]" else tuple(json.loads(attachments)),
            conference=json.loads(row["conference"]) if row["conference"] else None,
            guests_can_invite_others=(
                bool(row["guests_can_invite_others"])
                if row["guests_can_invite_others"] is not None
                else None
            ),
            guests_can_modify=(
                bool(row["guests_can_modify"]) if row["guests_can_modify"] is not None else None
            ),
            guests_can_see_other_guests=(
                bool(row["guests_can_see_other_guests"])
                if row["guests_can_see_other_guests"] is not None
                else None
            ),
            anyone_can_add_self=(
                bool(row["anyone_can_add_self"]) if row["anyone_can_add_self"] is not None else None
            ),
            focus_time_properties=(
                json.loads(row["focus_time_properties"]) if row["focus_time_properties"] else None
            ),
            out_of_office_properties=(
                json.loads(row["out_of_office_properties"])
                if row["out_of_office_properties"]
                else None
            ),
            working_location_properties=(
                json.loads(row["working_location_properties"])
                if row["working_location_properties"]
                else None
            ),
        )

    def get_event(self, account_id: str, event_id: str) -> Event | None:
        row = self.connection.execute(
            "SELECT * FROM events WHERE account_id=? AND id=?", (account_id, event_id)
        ).fetchone()
        return self._event(row) if row else None

    def get_event_by_remote(
        self, account_id: str, remote_id: str, *, occurrence_id: str | None = None
    ) -> Event | None:
        sql = "SELECT * FROM events WHERE account_id=? AND remote_id=?"
        args: list[Any] = [account_id, remote_id]
        if occurrence_id is None:
            sql += " AND occurrence_id IS NULL"
        else:
            sql, args = sql + " AND occurrence_id=?", [*args, occurrence_id]
        row = self.connection.execute(sql, args).fetchone()
        return self._event(row) if row else None

    def list_events(
        self,
        account_id: str,
        calendar_id: str | None = None,
        *,
        start: date | datetime | None = None,
        end: date | datetime | None = None,
        include_deleted: bool = False,
    ) -> list[Event]:
        sql = "SELECT * FROM events WHERE account_id=?"
        args: list[Any] = [account_id]
        if calendar_id is not None:
            sql, args = sql + " AND calendar_id=?", [*args, calendar_id]
        if start is not None:
            sql, args = sql + " AND end_value>?", [*args, _iso(start)]
        if end is not None:
            sql, args = sql + " AND start_value<?", [*args, _iso(end)]
        if not include_deleted:
            sql += " AND deleted=0"
        return [
            self._event(row) for row in self.connection.execute(sql + " ORDER BY start_value", args)
        ]

    def search_events(self, account_id: str, query: str, *, limit: int = 50) -> list[Event]:
        escaped = query.replace("\\", "\\\\").replace("%", "\\%").replace("_", "\\_")
        rows = self.connection.execute(
            """SELECT * FROM events WHERE account_id=? AND deleted=0
            AND (summary LIKE ? ESCAPE '\\' OR description LIKE ? ESCAPE '\\'
                 OR location LIKE ? ESCAPE '\\') ORDER BY start_value LIMIT ?""",
            (account_id, *(f"%{escaped}%" for _ in range(3)), limit),
        )
        return [self._event(row) for row in rows]

    def delete_event(self, account_id: str, event_id: str) -> None:
        self.connection.execute(
            "DELETE FROM events WHERE account_id=? AND id=?", (account_id, event_id)
        )

    def replace_cached_instances(
        self,
        account_id: str,
        calendar_id: str,
        start: date | datetime,
        end: date | datetime,
        events: list[Event],
    ) -> None:
        """Atomically replace the derived instances intersecting an expanded range."""
        identifiers = [event.id for event in events]
        sql = """DELETE FROM events WHERE account_id=? AND calendar_id=? AND derived=1
        AND deleted=0 AND end_value>? AND start_value<?"""
        arguments: list[Any] = [account_id, calendar_id, _iso(start), _iso(end)]
        if identifiers:
            sql += " AND id NOT IN (" + ",".join("?" for _ in identifiers) + ")"
            arguments.extend(identifiers)
        self.connection.execute(sql, arguments)
        for event in events:
            if not event.derived:
                raise ValueError("cached instance events must be marked derived")
            self.upsert_event(event)
        self.connection.execute(
            """INSERT INTO event_instance_ranges(
            account_id,calendar_id,start_value,end_value,refreshed_at,state,stale_at,stale_reason
            )
            VALUES (?,?,?,?,?,'fresh',NULL,NULL)
            ON CONFLICT(account_id,calendar_id,start_value,end_value)
            DO UPDATE SET refreshed_at=excluded.refreshed_at,state='fresh',stale_at=NULL,
            stale_reason=NULL""",
            (account_id, calendar_id, _iso(start), _iso(end), _iso(utc_now())),
        )

    def list_instance_ranges(
        self, account_id: str, calendar_id: str | None = None
    ) -> list[dict[str, str]]:
        sql = "SELECT * FROM event_instance_ranges WHERE account_id=?"
        arguments: list[str] = [account_id]
        if calendar_id is not None:
            sql += " AND calendar_id=?"
            arguments.append(calendar_id)
        sql += " ORDER BY calendar_id,start_value,end_value"
        return [dict(row) for row in self.connection.execute(sql, arguments)]

    def mark_instance_ranges_stale(self, account_id: str, calendar_id: str, *, reason: str) -> int:
        """Mark every cached occurrence range stale after a series-affecting change."""
        cursor = self.connection.execute(
            """UPDATE event_instance_ranges
            SET state='stale',stale_at=?,stale_reason=?
            WHERE account_id=? AND calendar_id=?""",
            (utc_now().isoformat(), reason, account_id, calendar_id),
        )
        return cursor.rowcount

    @staticmethod
    def _range_datetime(value: date | datetime) -> datetime:
        result = (
            value if isinstance(value, datetime) else datetime.combine(value, datetime.min.time())
        )
        return result.astimezone(UTC).replace(tzinfo=None) if result.tzinfo is not None else result

    def instance_cache_status(
        self,
        account_id: str,
        calendar_id: str,
        start: date | datetime,
        end: date | datetime,
    ) -> dict[str, Any]:
        """Describe local instance-cache coverage for a half-open time range.

        A range is fresh only when fresh cached ranges cover the complete query.
        Stale ranges remain visible as diagnostic metadata; they never contribute
        to fresh coverage.
        """
        start_at = self._range_datetime(start)
        end_at = self._range_datetime(end)
        if end_at <= start_at:
            raise ValueError("instance cache range end must be after start")
        ranges = self.list_instance_ranges(account_id, calendar_id)
        overlaps: list[dict[str, Any]] = []
        fresh: list[tuple[datetime, datetime]] = []
        for item in ranges:
            range_start = self._range_datetime(datetime.fromisoformat(item["start_value"]))
            range_end = self._range_datetime(datetime.fromisoformat(item["end_value"]))
            if range_end <= start_at or range_start >= end_at:
                continue
            overlaps.append(item)
            if item["state"] == "fresh":
                fresh.append((max(range_start, start_at), min(range_end, end_at)))
        fresh.sort()
        covered_until = start_at
        for range_start, range_end in fresh:
            if range_start > covered_until:
                break
            if range_end > covered_until:
                covered_until = range_end
            if covered_until >= end_at:
                break
        fresh_coverage = covered_until >= end_at
        if fresh_coverage:
            state = "fresh"
        elif fresh:
            state = "partial"
        elif overlaps:
            state = "stale"
        else:
            state = "missing"
        return {
            "calendar_id": calendar_id,
            "start": start_at.isoformat(),
            "end": end_at.isoformat(),
            "state": state,
            "fresh_coverage": fresh_coverage,
            "ranges": overlaps,
        }

    def clear_calendar_mirror(self, account_id: str, calendar_id: str) -> None:
        """Hide clean canonical rows until the full pull determines which still exist."""
        self.connection.execute(
            """INSERT OR IGNORE INTO calendar_refresh_candidates
            SELECT account_id,calendar_id,id FROM events
            WHERE account_id=? AND calendar_id=? AND dirty=0 AND derived=0""",
            (account_id, calendar_id),
        )
        self.connection.execute(
            """UPDATE events SET deleted=1
            WHERE account_id=? AND calendar_id=? AND dirty=0 AND derived=0""",
            (account_id, calendar_id),
        )
        self.connection.execute(
            "DELETE FROM events WHERE account_id=? AND calendar_id=? AND dirty=0 AND derived=1",
            (account_id, calendar_id),
        )
        self.connection.execute(
            "DELETE FROM event_instance_ranges WHERE account_id=? AND calendar_id=?",
            (account_id, calendar_id),
        )

    def mark_calendar_event_seen(self, account_id: str, calendar_id: str, event_id: str) -> None:
        self.connection.execute(
            """DELETE FROM calendar_refresh_candidates
            WHERE account_id=? AND calendar_id=? AND event_id=?""",
            (account_id, calendar_id, event_id),
        )

    def finish_calendar_refresh(self, account_id: str, calendar_id: str) -> None:
        self.connection.execute(
            """DELETE FROM events WHERE account_id=? AND calendar_id=? AND dirty=0
            AND id IN (SELECT event_id FROM calendar_refresh_candidates
                       WHERE account_id=? AND calendar_id=?)""",
            (account_id, calendar_id, account_id, calendar_id),
        )
        self.connection.execute(
            "DELETE FROM calendar_refresh_candidates WHERE account_id=? AND calendar_id=?",
            (account_id, calendar_id),
        )

    def hide_cached_series_instances(
        self, account_id: str, calendar_id: str, canonical_id: str
    ) -> None:
        """Remove derived display rows after a canonical series deletion is queued."""
        self.connection.execute(
            """UPDATE events SET deleted=1,local_updated_at=?
            WHERE account_id=? AND calendar_id=? AND derived=1 AND canonical_id=?""",
            (utc_now().isoformat(), account_id, calendar_id, canonical_id),
        )

    def upsert_drive_file(self, item: DriveFile) -> None:
        self.connection.execute(
            """INSERT INTO drive_files VALUES (?,?,?,?,?,?,?)
            ON CONFLICT(account_id,id) DO UPDATE SET name=excluded.name,
            mime_type=excluded.mime_type,web_view_link=excluded.web_view_link,
            icon_link=excluded.icon_link,modified_time=excluded.modified_time""",
            (
                item.id,
                item.account_id,
                item.name,
                item.mime_type,
                item.web_view_link,
                item.icon_link,
                _iso(item.modified_time),
            ),
        )

    def get_drive_file(self, account_id: str, file_id: str) -> DriveFile | None:
        row = self.connection.execute(
            "SELECT * FROM drive_files WHERE account_id=? AND id=?", (account_id, file_id)
        ).fetchone()
        return (
            DriveFile(
                row["id"],
                row["account_id"],
                row["name"],
                row["mime_type"],
                row["web_view_link"],
                row["icon_link"],
                _datetime(row["modified_time"]),
            )
            if row
            else None
        )

    def list_drive_files(self, account_id: str) -> list[DriveFile]:
        return [
            DriveFile(
                row["id"],
                row["account_id"],
                row["name"],
                row["mime_type"],
                row["web_view_link"],
                row["icon_link"],
                _datetime(row["modified_time"]),
            )
            for row in self.connection.execute(
                "SELECT * FROM drive_files WHERE account_id=? ORDER BY name", (account_id,)
            )
        ]
