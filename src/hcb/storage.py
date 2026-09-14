"""Local SQLite persistence and sync bookkeeping."""

from __future__ import annotations

import sqlite3
import time
from collections.abc import Iterator
from contextlib import contextmanager
from dataclasses import dataclass
from datetime import date, datetime
from pathlib import Path
from typing import Any, cast

from .models import (
    Metadata,
    NotesProjection,
)
from .paths import AppPaths

SCHEMA_VERSION = 14


@dataclass(frozen=True, slots=True)
class WorkspaceSearchDocument:
    """A ranked, indexed local workspace record before model hydration."""

    kind: str
    entity_id: str
    title: str
    body: str
    rank: float


_SCHEMA = """
CREATE TABLE accounts (
    id TEXT PRIMARY KEY, email TEXT NOT NULL, display_name TEXT, provider TEXT NOT NULL,
    enabled INTEGER NOT NULL, created_at TEXT NOT NULL
);
CREATE TABLE task_lists (
    id TEXT NOT NULL, account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    title TEXT NOT NULL, remote_id TEXT, position INTEGER NOT NULL DEFAULT 0,
    etag TEXT, remote_updated_at TEXT, local_updated_at TEXT NOT NULL,
    deleted INTEGER NOT NULL DEFAULT 0, dirty INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (account_id, id), UNIQUE (account_id, remote_id)
);
CREATE TABLE tasks (
    id TEXT NOT NULL, account_id TEXT NOT NULL, list_id TEXT NOT NULL, title TEXT NOT NULL,
    notes TEXT, status TEXT NOT NULL, due TEXT, completed_at TEXT, parent_id TEXT,
    position TEXT, remote_id TEXT, etag TEXT, remote_updated_at TEXT,
    local_updated_at TEXT NOT NULL, deleted INTEGER NOT NULL DEFAULT 0,
    dirty INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (account_id, id),
    FOREIGN KEY (account_id, list_id) REFERENCES task_lists(account_id, id) ON DELETE CASCADE,
    UNIQUE (account_id, remote_id)
);
CREATE TABLE calendars (
    id TEXT NOT NULL, account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    summary TEXT NOT NULL, remote_id TEXT, description TEXT, time_zone TEXT, color TEXT,
    selected INTEGER NOT NULL DEFAULT 1, etag TEXT, remote_updated_at TEXT,
    local_updated_at TEXT NOT NULL, deleted INTEGER NOT NULL DEFAULT 0,
    dirty INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (account_id, id),
    UNIQUE (account_id, remote_id)
);
CREATE TABLE events (
    id TEXT NOT NULL, account_id TEXT NOT NULL, calendar_id TEXT NOT NULL,
    summary TEXT NOT NULL, start_kind TEXT NOT NULL, start_value TEXT NOT NULL,
    start_time_zone TEXT, end_kind TEXT NOT NULL, end_value TEXT NOT NULL,
    end_time_zone TEXT, remote_id TEXT, canonical_id TEXT, occurrence_id TEXT,
    description TEXT, location TEXT, status TEXT NOT NULL, recurrence TEXT NOT NULL DEFAULT '[]',
    etag TEXT, remote_updated_at TEXT, local_updated_at TEXT NOT NULL,
    deleted INTEGER NOT NULL DEFAULT 0, dirty INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (account_id, id),
    FOREIGN KEY (account_id, calendar_id) REFERENCES calendars(account_id, id) ON DELETE CASCADE,
    UNIQUE (account_id, remote_id, occurrence_id)
);
CREATE INDEX events_range ON events(account_id, start_value, end_value);
CREATE INDEX events_canonical ON events(account_id, canonical_id, occurrence_id);
CREATE TABLE outbox (
    id INTEGER PRIMARY KEY AUTOINCREMENT, account_id TEXT NOT NULL REFERENCES accounts(id)
      ON DELETE CASCADE, entity_type TEXT NOT NULL, entity_id TEXT NOT NULL,
    operation TEXT NOT NULL, payload TEXT NOT NULL, created_at TEXT NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0, last_error TEXT
);
CREATE INDEX outbox_account ON outbox(account_id, id);
CREATE TABLE sync_cursors (
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE, scope TEXT NOT NULL,
    cursor TEXT, updated_at TEXT NOT NULL, PRIMARY KEY(account_id, scope)
);
CREATE TABLE sync_checkpoints (
    id INTEGER PRIMARY KEY AUTOINCREMENT, account_id TEXT NOT NULL, scope TEXT NOT NULL,
    started_at TEXT NOT NULL, completed_at TEXT, cursor TEXT, error TEXT
);
CREATE TABLE conflicts (
    id INTEGER PRIMARY KEY AUTOINCREMENT, account_id TEXT NOT NULL REFERENCES accounts(id)
      ON DELETE CASCADE, entity_type TEXT NOT NULL, entity_id TEXT NOT NULL,
    local_payload TEXT NOT NULL, remote_payload TEXT NOT NULL, status TEXT NOT NULL,
    created_at TEXT NOT NULL, resolved_at TEXT
);
CREATE TABLE reminder_state (
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    event_id TEXT NOT NULL, occurrence_id TEXT NOT NULL DEFAULT '',
    remind_at TEXT NOT NULL, notified_at TEXT, dismissed_at TEXT,
    PRIMARY KEY(account_id, event_id, occurrence_id, remind_at)
);
"""

_MIGRATION_2 = """
ALTER TABLE tasks ADD COLUMN priority TEXT NOT NULL DEFAULT 'none';
ALTER TABLE tasks ADD COLUMN due_time_zone TEXT;
CREATE TABLE saved_searches (
    id TEXT PRIMARY KEY, account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name TEXT NOT NULL, query TEXT NOT NULL, created_at TEXT NOT NULL,
    UNIQUE(account_id, name)
);
CREATE TABLE task_event_links (
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    task_id TEXT NOT NULL, event_id TEXT NOT NULL, created_at TEXT NOT NULL,
    PRIMARY KEY(account_id, task_id, event_id)
);
CREATE TABLE intents (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    action TEXT NOT NULL, entity_type TEXT NOT NULL, entity_id TEXT NOT NULL,
    before_payload TEXT, after_payload TEXT, state TEXT NOT NULL DEFAULT 'applied',
    created_at TEXT NOT NULL
);
CREATE INDEX intents_account_state ON intents(account_id, state, id);
CREATE TABLE app_settings (
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    key TEXT NOT NULL, value TEXT NOT NULL, PRIMARY KEY(account_id, key)
);
"""

_MIGRATION_3 = """
ALTER TABLE calendars ADD COLUMN default_reminders TEXT NOT NULL DEFAULT '[]';
ALTER TABLE events ADD COLUMN reminder_use_default INTEGER NOT NULL DEFAULT 1;
ALTER TABLE events ADD COLUMN reminder_overrides TEXT NOT NULL DEFAULT '[]';
ALTER TABLE events ADD COLUMN attendees TEXT NOT NULL DEFAULT '[]';
ALTER TABLE events ADD COLUMN attendee_response TEXT;
ALTER TABLE events ADD COLUMN event_type TEXT;
ALTER TABLE events ADD COLUMN transparency TEXT;
ALTER TABLE events ADD COLUMN visibility TEXT;
ALTER TABLE events ADD COLUMN color_id TEXT;
ALTER TABLE events ADD COLUMN attachments TEXT NOT NULL DEFAULT '[]';
ALTER TABLE events ADD COLUMN conference TEXT;
CREATE TABLE reminder_deliveries (
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    source_type TEXT NOT NULL, source_id TEXT NOT NULL, occurrence_id TEXT NOT NULL DEFAULT '',
    scheduled_at TEXT NOT NULL, delivered_at TEXT, dismissed_at TEXT, snoozed_until TEXT,
    last_error TEXT, attempts INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY(account_id, source_type, source_id, occurrence_id, scheduled_at)
);
CREATE INDEX reminder_deliveries_due
ON reminder_deliveries(account_id, scheduled_at, delivered_at, dismissed_at);
"""

_MIGRATION_4 = """
ALTER TABLE events ADD COLUMN guests_can_invite_others INTEGER;
ALTER TABLE events ADD COLUMN guests_can_modify INTEGER;
ALTER TABLE events ADD COLUMN guests_can_see_other_guests INTEGER;
ALTER TABLE events ADD COLUMN anyone_can_add_self INTEGER;
ALTER TABLE events ADD COLUMN focus_time_properties TEXT;
ALTER TABLE events ADD COLUMN out_of_office_properties TEXT;
ALTER TABLE events ADD COLUMN working_location_properties TEXT;
DELETE FROM task_event_links
WHERE rowid NOT IN (
    SELECT MAX(rowid) FROM task_event_links GROUP BY account_id,task_id
);
CREATE UNIQUE INDEX one_active_task_block
ON task_event_links(account_id, task_id);
CREATE TABLE drive_files (
    id TEXT NOT NULL, account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name TEXT NOT NULL, mime_type TEXT, web_view_link TEXT, icon_link TEXT,
    modified_time TEXT, PRIMARY KEY(account_id,id)
);
"""

_MIGRATION_5 = """
ALTER TABLE outbox ADD COLUMN delivery_state TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE outbox ADD COLUMN request_id TEXT;
ALTER TABLE outbox ADD COLUMN sending_started_at TEXT;
CREATE INDEX outbox_delivery_state ON outbox(account_id,delivery_state,id);
"""

_MIGRATION_6 = """
ALTER TABLE calendars ADD COLUMN foreground_color TEXT;
ALTER TABLE calendars ADD COLUMN location TEXT;
ALTER TABLE calendars ADD COLUMN summary_override TEXT;
ALTER TABLE calendars ADD COLUMN hidden INTEGER NOT NULL DEFAULT 0;
ALTER TABLE calendars ADD COLUMN notification_settings TEXT NOT NULL DEFAULT '[]';
ALTER TABLE events ADD COLUMN derived INTEGER NOT NULL DEFAULT 0;
CREATE INDEX events_derived_range ON events(account_id,calendar_id,derived,start_value,end_value);
CREATE TABLE event_instance_ranges (
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    calendar_id TEXT NOT NULL,
    start_value TEXT NOT NULL,
    end_value TEXT NOT NULL,
    refreshed_at TEXT NOT NULL,
    PRIMARY KEY(account_id,calendar_id,start_value,end_value)
);
"""

_MIGRATION_7 = """
ALTER TABLE event_instance_ranges ADD COLUMN state TEXT NOT NULL DEFAULT 'fresh';
ALTER TABLE event_instance_ranges ADD COLUMN stale_at TEXT;
ALTER TABLE event_instance_ranges ADD COLUMN stale_reason TEXT;
CREATE INDEX event_instance_ranges_coverage
ON event_instance_ranges(account_id,calendar_id,state,start_value,end_value);
"""

_MIGRATION_8 = """
CREATE TABLE workspace_search_documents (
    id INTEGER PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    title TEXT NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL,
    status TEXT,
    priority TEXT,
    due TEXT,
    event_date TEXT,
    list_id TEXT,
    list_name TEXT,
    calendar_id TEXT,
    calendar_name TEXT,
    UNIQUE(account_id, kind, entity_id)
);
CREATE INDEX workspace_search_documents_filters ON workspace_search_documents(
    account_id, kind, source, status, priority, due, event_date
);
CREATE VIRTUAL TABLE workspace_search USING fts5(
    title, body,
    content='workspace_search_documents',
    content_rowid='id',
    tokenize='trigram'
);
CREATE TRIGGER workspace_search_documents_ai AFTER INSERT ON workspace_search_documents BEGIN
    INSERT INTO workspace_search(rowid,title,body) VALUES (new.id,new.title,new.body);
END;
CREATE TRIGGER workspace_search_documents_ad BEFORE DELETE ON workspace_search_documents BEGIN
    INSERT INTO workspace_search(workspace_search,rowid,title,body)
    VALUES ('delete',old.id,old.title,old.body);
END;
CREATE TRIGGER workspace_search_documents_au BEFORE UPDATE ON workspace_search_documents BEGIN
    INSERT INTO workspace_search(workspace_search,rowid,title,body)
    VALUES ('delete',old.id,old.title,old.body);
END;
CREATE TRIGGER workspace_search_documents_au_after AFTER UPDATE ON workspace_search_documents BEGIN
    INSERT INTO workspace_search(rowid,title,body) VALUES (new.id,new.title,new.body);
END;

CREATE TRIGGER workspace_search_task_lists_ai AFTER INSERT ON task_lists BEGIN
    DELETE FROM workspace_search_documents
    WHERE account_id=new.account_id AND kind='task-list' AND entity_id=new.id;
    INSERT INTO workspace_search_documents(account_id,kind,entity_id,title,body,source)
    SELECT new.account_id,'task-list',new.id,new.title,'','google' WHERE new.deleted=0;
END;
CREATE TRIGGER workspace_search_task_lists_au AFTER UPDATE ON task_lists BEGIN
    DELETE FROM workspace_search_documents
    WHERE account_id=new.account_id AND kind IN ('task-list','task')
      AND (entity_id=new.id OR list_id=new.id);
    INSERT INTO workspace_search_documents(account_id,kind,entity_id,title,body,source)
    SELECT new.account_id,'task-list',new.id,new.title,'','google' WHERE new.deleted=0;
    INSERT INTO workspace_search_documents(
        account_id,kind,entity_id,title,body,source,status,priority,due,list_id,list_name
    )
    SELECT t.account_id,'task',t.id,t.title,COALESCE(t.notes,''),'google',t.status,t.priority,
           t.due,t.list_id,new.title
    FROM tasks t WHERE t.account_id=new.account_id AND t.list_id=new.id AND t.deleted=0;
END;
CREATE TRIGGER workspace_search_task_lists_ad AFTER DELETE ON task_lists BEGIN
    DELETE FROM workspace_search_documents
    WHERE account_id=old.account_id AND kind IN ('task-list','task')
      AND (entity_id=old.id OR list_id=old.id);
END;

CREATE TRIGGER workspace_search_tasks_ai AFTER INSERT ON tasks BEGIN
    DELETE FROM workspace_search_documents
    WHERE account_id=new.account_id AND kind='task' AND entity_id=new.id;
    INSERT INTO workspace_search_documents(
        account_id,kind,entity_id,title,body,source,status,priority,due,list_id,list_name
    )
    SELECT new.account_id,'task',new.id,new.title,COALESCE(new.notes,''),'google',new.status,
           new.priority,new.due,new.list_id,
           COALESCE((
               SELECT title FROM task_lists WHERE account_id=new.account_id AND id=new.list_id
           ),'')
    WHERE new.deleted=0;
END;
CREATE TRIGGER workspace_search_tasks_au AFTER UPDATE ON tasks BEGIN
    DELETE FROM workspace_search_documents
    WHERE account_id=new.account_id AND kind='task' AND entity_id=new.id;
    INSERT INTO workspace_search_documents(
        account_id,kind,entity_id,title,body,source,status,priority,due,list_id,list_name
    )
    SELECT new.account_id,'task',new.id,new.title,COALESCE(new.notes,''),'google',new.status,
           new.priority,new.due,new.list_id,
           COALESCE((
               SELECT title FROM task_lists WHERE account_id=new.account_id AND id=new.list_id
           ),'')
    WHERE new.deleted=0;
END;
CREATE TRIGGER workspace_search_tasks_ad AFTER DELETE ON tasks BEGIN
    DELETE FROM workspace_search_documents
    WHERE account_id=old.account_id AND kind='task' AND entity_id=old.id;
END;

CREATE TRIGGER workspace_search_calendars_ai AFTER INSERT ON calendars BEGIN
    DELETE FROM workspace_search_documents
    WHERE account_id=new.account_id AND kind='calendar' AND entity_id=new.id;
    INSERT INTO workspace_search_documents(account_id,kind,entity_id,title,body,source)
    SELECT new.account_id,'calendar',new.id,new.summary,
           COALESCE(new.description,'') || ' ' || COALESCE(new.location,'') || ' ' ||
           COALESCE(new.summary_override,''),'google'
    WHERE new.deleted=0;
END;
CREATE TRIGGER workspace_search_calendars_au AFTER UPDATE ON calendars BEGIN
    DELETE FROM workspace_search_documents
    WHERE account_id=new.account_id AND kind IN ('calendar','event')
      AND (entity_id=new.id OR calendar_id=new.id);
    INSERT INTO workspace_search_documents(account_id,kind,entity_id,title,body,source)
    SELECT new.account_id,'calendar',new.id,new.summary,
           COALESCE(new.description,'') || ' ' || COALESCE(new.location,'') || ' ' ||
           COALESCE(new.summary_override,''),'google'
    WHERE new.deleted=0;
    INSERT INTO workspace_search_documents(
        account_id,kind,entity_id,title,body,source,status,event_date,calendar_id,calendar_name
    )
    SELECT e.account_id,'event',e.id,e.summary,
           COALESCE(e.description,'') || ' ' || COALESCE(e.location,'') || ' ' ||
           COALESCE(e.attendees,'') || ' ' || COALESCE(e.attachments,'') || ' ' ||
           COALESCE(e.conference,'') || ' ' || COALESCE(e.recurrence,''),'google',e.status,
           substr(e.start_value,1,10),e.calendar_id,new.summary
    FROM events e WHERE e.account_id=new.account_id AND e.calendar_id=new.id AND e.deleted=0;
END;
CREATE TRIGGER workspace_search_calendars_ad AFTER DELETE ON calendars BEGIN
    DELETE FROM workspace_search_documents
    WHERE account_id=old.account_id AND kind IN ('calendar','event')
      AND (entity_id=old.id OR calendar_id=old.id);
END;

CREATE TRIGGER workspace_search_events_ai AFTER INSERT ON events BEGIN
    DELETE FROM workspace_search_documents
    WHERE account_id=new.account_id AND kind='event' AND entity_id=new.id;
    INSERT INTO workspace_search_documents(
        account_id,kind,entity_id,title,body,source,status,event_date,calendar_id,calendar_name
    )
    SELECT new.account_id,'event',new.id,new.summary,
           COALESCE(new.description,'') || ' ' || COALESCE(new.location,'') || ' ' ||
           COALESCE(new.attendees,'') || ' ' || COALESCE(new.attachments,'') || ' ' ||
           COALESCE(new.conference,'') || ' ' || COALESCE(new.recurrence,''),'google',new.status,
           substr(new.start_value,1,10),new.calendar_id,
           COALESCE((
               SELECT summary FROM calendars WHERE account_id=new.account_id AND id=new.calendar_id
           ),'')
    WHERE new.deleted=0;
END;
CREATE TRIGGER workspace_search_events_au AFTER UPDATE ON events BEGIN
    DELETE FROM workspace_search_documents
    WHERE account_id=new.account_id AND kind='event' AND entity_id=new.id;
    INSERT INTO workspace_search_documents(
        account_id,kind,entity_id,title,body,source,status,event_date,calendar_id,calendar_name
    )
    SELECT new.account_id,'event',new.id,new.summary,
           COALESCE(new.description,'') || ' ' || COALESCE(new.location,'') || ' ' ||
           COALESCE(new.attendees,'') || ' ' || COALESCE(new.attachments,'') || ' ' ||
           COALESCE(new.conference,'') || ' ' || COALESCE(new.recurrence,''),'google',new.status,
           substr(new.start_value,1,10),new.calendar_id,
           COALESCE((
               SELECT summary FROM calendars WHERE account_id=new.account_id AND id=new.calendar_id
           ),'')
    WHERE new.deleted=0;
END;
CREATE TRIGGER workspace_search_events_ad AFTER DELETE ON events BEGIN
    DELETE FROM workspace_search_documents
    WHERE account_id=old.account_id AND kind='event' AND entity_id=old.id;
END;

CREATE TRIGGER workspace_search_drive_files_ai AFTER INSERT ON drive_files BEGIN
    DELETE FROM workspace_search_documents
    WHERE account_id=new.account_id AND kind='drive' AND entity_id=new.id;
    INSERT INTO workspace_search_documents(account_id,kind,entity_id,title,body,source)
    VALUES (new.account_id,'drive',new.id,new.name,
            COALESCE(new.mime_type,'') || ' ' || COALESCE(new.web_view_link,''),'google');
END;
CREATE TRIGGER workspace_search_drive_files_au AFTER UPDATE ON drive_files BEGIN
    DELETE FROM workspace_search_documents
    WHERE account_id=new.account_id AND kind='drive' AND entity_id=new.id;
    INSERT INTO workspace_search_documents(account_id,kind,entity_id,title,body,source)
    VALUES (new.account_id,'drive',new.id,new.name,
            COALESCE(new.mime_type,'') || ' ' || COALESCE(new.web_view_link,''),'google');
END;
CREATE TRIGGER workspace_search_drive_files_ad AFTER DELETE ON drive_files BEGIN
    DELETE FROM workspace_search_documents
    WHERE account_id=old.account_id AND kind='drive' AND entity_id=old.id;
END;

CREATE TRIGGER workspace_search_saved_searches_ai AFTER INSERT ON saved_searches BEGIN
    DELETE FROM workspace_search_documents
    WHERE account_id=new.account_id AND kind='saved-search' AND entity_id=new.id;
    INSERT INTO workspace_search_documents(account_id,kind,entity_id,title,body,source)
    VALUES (new.account_id,'saved-search',new.id,new.name,new.query,'local');
END;
CREATE TRIGGER workspace_search_saved_searches_au AFTER UPDATE ON saved_searches BEGIN
    DELETE FROM workspace_search_documents
    WHERE account_id=new.account_id AND kind='saved-search' AND entity_id=new.id;
    INSERT INTO workspace_search_documents(account_id,kind,entity_id,title,body,source)
    VALUES (new.account_id,'saved-search',new.id,new.name,new.query,'local');
END;
CREATE TRIGGER workspace_search_saved_searches_ad AFTER DELETE ON saved_searches BEGIN
    DELETE FROM workspace_search_documents
    WHERE account_id=old.account_id AND kind='saved-search' AND entity_id=old.id;
END;

CREATE TRIGGER workspace_search_conflicts_ai AFTER INSERT ON conflicts BEGIN
    DELETE FROM workspace_search_documents
    WHERE account_id=new.account_id AND kind='conflict' AND entity_id=CAST(new.id AS TEXT);
    INSERT INTO workspace_search_documents(account_id,kind,entity_id,title,body,source,status)
    VALUES (new.account_id,'conflict',CAST(new.id AS TEXT),new.entity_type || ' conflict',
            new.local_payload || ' ' || new.remote_payload,'local',new.status);
END;
CREATE TRIGGER workspace_search_conflicts_au AFTER UPDATE ON conflicts BEGIN
    DELETE FROM workspace_search_documents
    WHERE account_id=new.account_id AND kind='conflict' AND entity_id=CAST(new.id AS TEXT);
    INSERT INTO workspace_search_documents(account_id,kind,entity_id,title,body,source,status)
    VALUES (new.account_id,'conflict',CAST(new.id AS TEXT),new.entity_type || ' conflict',
            new.local_payload || ' ' || new.remote_payload,'local',new.status);
END;
CREATE TRIGGER workspace_search_conflicts_ad AFTER DELETE ON conflicts BEGIN
    DELETE FROM workspace_search_documents
    WHERE account_id=old.account_id AND kind='conflict' AND entity_id=CAST(old.id AS TEXT);
END;

INSERT INTO workspace_search_documents(account_id,kind,entity_id,title,body,source)
SELECT account_id,'task-list',id,title,'','google' FROM task_lists WHERE deleted=0;
INSERT INTO workspace_search_documents(
    account_id,kind,entity_id,title,body,source,status,priority,due,list_id,list_name
)
SELECT t.account_id,'task',t.id,t.title,COALESCE(t.notes,''),'google',t.status,t.priority,t.due,
       t.list_id,COALESCE(l.title,'')
FROM tasks t LEFT JOIN task_lists l ON l.account_id=t.account_id AND l.id=t.list_id
WHERE t.deleted=0;
INSERT INTO workspace_search_documents(account_id,kind,entity_id,title,body,source)
SELECT account_id,'calendar',id,summary,
       COALESCE(description,'') || ' ' || COALESCE(location,'') || ' ' ||
       COALESCE(summary_override,''),'google'
FROM calendars WHERE deleted=0;
INSERT INTO workspace_search_documents(
    account_id,kind,entity_id,title,body,source,status,event_date,calendar_id,calendar_name
)
SELECT e.account_id,'event',e.id,e.summary,
       COALESCE(e.description,'') || ' ' || COALESCE(e.location,'') || ' ' ||
       COALESCE(e.attendees,'') || ' ' || COALESCE(e.attachments,'') || ' ' ||
       COALESCE(e.conference,'') || ' ' || COALESCE(e.recurrence,''),'google',e.status,
       substr(e.start_value,1,10),e.calendar_id,COALESCE(c.summary,'')
FROM events e LEFT JOIN calendars c ON c.account_id=e.account_id AND c.id=e.calendar_id
WHERE e.deleted=0;
INSERT INTO workspace_search_documents(account_id,kind,entity_id,title,body,source)
SELECT account_id,'drive',id,name,
       COALESCE(mime_type,'') || ' ' || COALESCE(web_view_link,''),'google'
FROM drive_files;
INSERT INTO workspace_search_documents(account_id,kind,entity_id,title,body,source)
SELECT account_id,'saved-search',id,name,query,'local' FROM saved_searches;
INSERT INTO workspace_search_documents(account_id,kind,entity_id,title,body,source,status)
SELECT account_id,'conflict',CAST(id AS TEXT),entity_type || ' conflict',
       local_payload || ' ' || remote_payload,'local',status
FROM conflicts;
INSERT INTO workspace_search(workspace_search) VALUES ('rebuild');
"""

_MIGRATION_9 = """
CREATE TABLE calendar_refresh_candidates (
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    calendar_id TEXT NOT NULL,
    event_id TEXT NOT NULL,
    PRIMARY KEY(account_id,calendar_id,event_id)
);
"""

_MIGRATION_10 = """
CREATE TABLE task_private_notes (
    account_id TEXT NOT NULL,
    task_id TEXT NOT NULL,
    notes TEXT,
    PRIMARY KEY(account_id,task_id),
    FOREIGN KEY(account_id,task_id) REFERENCES tasks(account_id,id) ON DELETE CASCADE
);
INSERT INTO task_private_notes(account_id,task_id,notes)
SELECT t.account_id,t.id,t.notes FROM tasks t
JOIN app_settings s ON s.account_id=t.account_id AND s.key='notes_projection'
WHERE s.value='disabled' AND t.notes IS NOT NULL;
"""

_MIGRATION_11 = """
CREATE INDEX tasks_parent_resolution ON tasks(account_id,list_id,parent_id)
WHERE parent_id IS NOT NULL;
"""

_MIGRATION_12 = """
CREATE INDEX tasks_page ON tasks(account_id,deleted,status,due,title,id);
"""

_MIGRATION_13 = """
CREATE TABLE bridge_mutation_receipts (
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    idempotency_key TEXT NOT NULL,
    method TEXT NOT NULL,
    path TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    response_status INTEGER NOT NULL,
    response_data TEXT NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY(account_id,idempotency_key)
);
CREATE INDEX bridge_mutation_receipts_expiry ON bridge_mutation_receipts(created_at);
"""

_MIGRATION_14 = """
ALTER TABLE task_lists ADD COLUMN selected INTEGER NOT NULL DEFAULT 1;
"""


def _iso(value: date | datetime | None) -> str | None:
    return value.isoformat() if value is not None else None


def _datetime(value: str | None) -> datetime | None:
    return datetime.fromisoformat(value) if value else None


def _metadata(row: sqlite3.Row) -> Metadata:
    return Metadata(
        etag=row["etag"],
        remote_updated_at=_datetime(row["remote_updated_at"]),
        local_updated_at=datetime.fromisoformat(row["local_updated_at"]),
        deleted=bool(row["deleted"]),
        dirty=bool(row["dirty"]),
    )


class _StorageCore:
    """A single local database. Public entity methods always require an account id."""

    def __init__(self, path: Path | str | None = None) -> None:
        target = Path(path) if path is not None else AppPaths.discover().database_file
        target.parent.mkdir(parents=True, exist_ok=True)
        self.path = target
        self.connection = sqlite3.connect(target, isolation_level=None)
        self.connection.row_factory = sqlite3.Row
        try:
            self.connection.execute("PRAGMA foreign_keys = ON")
            self.connection.execute("PRAGMA busy_timeout = 5000")
            self._enable_wal()
            self._migrate()
        except BaseException:
            self.connection.close()
            raise

    def get_notes_projection(self, account_id: str) -> NotesProjection:
        row = self.connection.execute(
            "SELECT value FROM app_settings WHERE account_id=? AND key='notes_projection'",
            (account_id,),
        ).fetchone()
        return NotesProjection(row["value"]) if row else NotesProjection.MIRRORED

    def set_private_task_note(self, account_id: str, task_id: str, notes: str | None) -> None:
        self.connection.execute(
            """INSERT INTO task_private_notes(account_id,task_id,notes) VALUES (?,?,?)
            ON CONFLICT(account_id,task_id) DO UPDATE SET notes=excluded.notes""",
            (account_id, task_id, notes),
        )

    def private_task_notes(self, account_id: str, list_id: str) -> dict[str, str | None]:
        rows = self.connection.execute(
            """SELECT n.task_id,n.notes FROM task_private_notes n
            JOIN tasks t ON t.account_id=n.account_id AND t.id=n.task_id
            WHERE n.account_id=? AND t.list_id=?""",
            (account_id, list_id),
        )
        return {row["task_id"]: row["notes"] for row in rows}

    def clear_private_task_note(self, account_id: str, task_id: str) -> None:
        self.connection.execute(
            "DELETE FROM task_private_notes WHERE account_id=? AND task_id=?",
            (account_id, task_id),
        )

    def _enable_wal(self) -> None:
        deadline = time.monotonic() + 5
        while True:
            try:
                self.connection.execute("PRAGMA journal_mode = WAL")
                return
            except sqlite3.OperationalError as error:
                if (
                    error.sqlite_errorcode not in {sqlite3.SQLITE_BUSY, sqlite3.SQLITE_LOCKED}
                    or time.monotonic() >= deadline
                ):
                    raise
                time.sleep(0.05)

    def _execute_schema_script(self, script: str) -> None:
        # executescript() commits an open transaction before running the script.
        statement = ""
        for line in script.splitlines(keepends=True):
            statement += line
            if sqlite3.complete_statement(statement):
                self.connection.execute(statement)
                statement = ""
        if statement.strip():
            raise ValueError("incomplete migration SQL")

    def _migrate(self) -> None:
        version = int(self.connection.execute("PRAGMA user_version").fetchone()[0])
        if version > SCHEMA_VERSION:
            raise RuntimeError(
                f"database schema {version} is newer than supported {SCHEMA_VERSION}"
            )
        if version == SCHEMA_VERSION:
            return
        scripts = (
            _SCHEMA,
            _MIGRATION_2,
            _MIGRATION_3,
            _MIGRATION_4,
            _MIGRATION_5,
            _MIGRATION_6,
            _MIGRATION_7,
            _MIGRATION_8,
            _MIGRATION_9,
            _MIGRATION_10,
            _MIGRATION_11,
            _MIGRATION_12,
            _MIGRATION_13,
            _MIGRATION_14,
        )
        with self.transaction():
            # Another process may have completed the migration while we waited
            # for SQLite's writer lock.
            version = int(self.connection.execute("PRAGMA user_version").fetchone()[0])
            if version > SCHEMA_VERSION:
                raise RuntimeError(
                    f"database schema {version} is newer than supported {SCHEMA_VERSION}"
                )
            for next_version in range(version + 1, SCHEMA_VERSION + 1):
                self._execute_schema_script(scripts[next_version - 1])
                self.connection.execute(f"PRAGMA user_version = {next_version}")

    def close(self) -> None:
        self.connection.close()

    def __enter__(self) -> Storage:
        return cast(Storage, self)

    def __exit__(self, *_: object) -> None:
        self.close()

    @contextmanager
    def transaction(self) -> Iterator[sqlite3.Connection]:
        if self.connection.in_transaction:
            name = f"hcb_{id(object())}"
            self.connection.execute(f"SAVEPOINT {name}")
            try:
                yield self.connection
            except BaseException:
                self.connection.execute(f"ROLLBACK TO {name}")
                self.connection.execute(f"RELEASE {name}")
                raise
            else:
                self.connection.execute(f"RELEASE {name}")
            return
        self.connection.execute("BEGIN IMMEDIATE")
        try:
            yield self.connection
        except BaseException:
            self.connection.rollback()
            raise
        else:
            self.connection.commit()

    @staticmethod
    def _meta_values(metadata: Metadata) -> tuple[Any, ...]:
        return (
            metadata.etag,
            _iso(metadata.remote_updated_at),
            _iso(metadata.local_updated_at),
            metadata.deleted,
            metadata.dirty,
        )


# Storage owns one SQLite connection and transaction manager; repositories below
# expose cohesive persistence APIs over that shared owner.
from .storage_accounts import AccountTaskRepository  # noqa: E402
from .storage_bridge import BridgeRepository  # noqa: E402
from .storage_calendars import CalendarEventRepository  # noqa: E402
from .storage_reminders import ReminderRepository  # noqa: E402
from .storage_sync import SyncStateRepository  # noqa: E402


class Storage(
    AccountTaskRepository,
    BridgeRepository,
    CalendarEventRepository,
    SyncStateRepository,
    ReminderRepository,
):
    """A single local database with cohesive repository APIs."""
