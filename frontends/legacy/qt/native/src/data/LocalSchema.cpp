#include "data/LocalSchema.h"

#include "sqlite3.h"

#include <QCryptographicHash>

#include <array>
#include <optional>

namespace hcb {
namespace {

constexpr char settingsSchemaSql[] = R"(
CREATE TABLE local_settings (
  scope TEXT NOT NULL CHECK(length(trim(scope)) > 0),
  key TEXT NOT NULL CHECK(length(trim(key)) > 0),
  value_json TEXT NOT NULL CHECK(json_valid(value_json)),
  updated_at TEXT NOT NULL CHECK(length(trim(updated_at)) > 0),
  PRIMARY KEY(scope, key)
) STRICT, WITHOUT ROWID
)";

constexpr char accountSchemaSql[] = R"(
CREATE TABLE local_accounts (
  id TEXT PRIMARY KEY CHECK(length(trim(id)) BETWEEN 1 AND 256),
  provider TEXT NOT NULL CHECK(provider = 'google'),
  provider_account_id TEXT CHECK(provider_account_id IS NULL OR length(trim(provider_account_id)) BETWEEN 1 AND 256),
  email TEXT CHECK(email IS NULL OR length(trim(email)) BETWEEN 1 AND 254),
  display_name TEXT CHECK(display_name IS NULL OR length(trim(display_name)) BETWEEN 1 AND 256),
  avatar_url TEXT CHECK(avatar_url IS NULL OR length(trim(avatar_url)) BETWEEN 1 AND 2048),
  locale TEXT CHECK(locale IS NULL OR length(trim(locale)) BETWEEN 1 AND 64),
  time_zone TEXT CHECK(time_zone IS NULL OR length(trim(time_zone)) BETWEEN 1 AND 128),
  connection_state TEXT NOT NULL CHECK(connection_state IN ('signed_out', 'connected', 'reauth_required', 'sync_paused')),
  granted_scopes_json TEXT NOT NULL CHECK(length(granted_scopes_json) <= 8192 AND json_valid(granted_scopes_json) AND json_type(granted_scopes_json) = 'array' AND json_array_length(granted_scopes_json) <= 20),
  missing_scopes_json TEXT NOT NULL CHECK(length(missing_scopes_json) <= 8192 AND json_valid(missing_scopes_json) AND json_type(missing_scopes_json) = 'array' AND json_array_length(missing_scopes_json) <= 20),
  last_authenticated_at TEXT CHECK(last_authenticated_at IS NULL OR length(trim(last_authenticated_at)) BETWEEN 1 AND 64),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')) CHECK(length(trim(created_at)) BETWEEN 1 AND 64),
  updated_at TEXT NOT NULL CHECK(length(trim(updated_at)) BETWEEN 1 AND 64),
  deleted_at TEXT CHECK(deleted_at IS NULL OR length(trim(deleted_at)) BETWEEN 1 AND 64),
  UNIQUE(provider, provider_account_id)
) STRICT, WITHOUT ROWID;

CREATE INDEX local_accounts_active_recency
ON local_accounts(connection_state, updated_at DESC, id)
WHERE deleted_at IS NULL
)";

constexpr char taskListSchemaSql[] = R"(
CREATE TABLE local_task_lists (
  id TEXT PRIMARY KEY CHECK(length(trim(id)) BETWEEN 1 AND 256),
  account_id TEXT NOT NULL REFERENCES local_accounts(id) ON UPDATE CASCADE ON DELETE RESTRICT,
  remote_id TEXT NOT NULL CHECK(length(trim(remote_id)) BETWEEN 1 AND 256),
  title TEXT NOT NULL CHECK(length(trim(title)) BETWEEN 1 AND 1024),
  etag TEXT CHECK(etag IS NULL OR length(etag) <= 4096),
  sort_order INTEGER NOT NULL DEFAULT 0,
  is_selected INTEGER NOT NULL DEFAULT 1 CHECK(is_selected IN (0, 1)),
  remote_updated_at TEXT CHECK(remote_updated_at IS NULL OR length(trim(remote_updated_at)) BETWEEN 1 AND 64),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')) CHECK(length(trim(created_at)) BETWEEN 1 AND 64),
  updated_at TEXT NOT NULL CHECK(length(trim(updated_at)) BETWEEN 1 AND 64),
  deleted_at TEXT CHECK(deleted_at IS NULL OR length(trim(deleted_at)) BETWEEN 1 AND 64),
  UNIQUE(account_id, remote_id)
) STRICT, WITHOUT ROWID;

CREATE INDEX local_task_lists_active_navigation
ON local_task_lists(account_id, is_selected DESC, sort_order, title COLLATE NOCASE, id)
WHERE deleted_at IS NULL
)";

constexpr char taskSchemaSql[] = R"(
CREATE TABLE local_tasks (
  id TEXT PRIMARY KEY CHECK(length(trim(id)) BETWEEN 1 AND 256),
  task_list_id TEXT NOT NULL REFERENCES local_task_lists(id) ON UPDATE CASCADE ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
  remote_id TEXT NOT NULL CHECK(length(trim(remote_id)) BETWEEN 1 AND 256),
  parent_task_id TEXT REFERENCES local_tasks(id) ON UPDATE CASCADE ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED CHECK(parent_task_id IS NULL OR parent_task_id != id),
  title TEXT NOT NULL CHECK(length(trim(title)) BETWEEN 1 AND 500),
  notes TEXT CHECK(notes IS NULL OR length(notes) <= 10000),
  state TEXT NOT NULL DEFAULT 'active' CHECK(state IN ('active', 'completed')),
  due_at TEXT CHECK(due_at IS NULL OR length(trim(due_at)) BETWEEN 1 AND 64),
  due_time_zone TEXT CHECK(due_time_zone IS NULL OR length(trim(due_time_zone)) BETWEEN 1 AND 128),
  completed_at TEXT CHECK(completed_at IS NULL OR length(trim(completed_at)) BETWEEN 1 AND 64),
  remote_position TEXT CHECK(remote_position IS NULL OR length(remote_position) <= 256),
  sort_order INTEGER NOT NULL DEFAULT 0,
  is_hidden INTEGER NOT NULL DEFAULT 0 CHECK(is_hidden IN (0, 1)),
  priority TEXT NOT NULL DEFAULT 'none' CHECK(priority IN ('none', 'low', 'medium', 'high')),
  planned_start_at TEXT CHECK(planned_start_at IS NULL OR length(trim(planned_start_at)) BETWEEN 1 AND 64),
  planned_end_at TEXT CHECK(planned_end_at IS NULL OR length(trim(planned_end_at)) BETWEEN 1 AND 64),
  duration_minutes INTEGER CHECK(duration_minutes IS NULL OR duration_minutes >= 0),
  is_schedule_locked INTEGER NOT NULL DEFAULT 0 CHECK(is_schedule_locked IN (0, 1)),
  snoozed_until TEXT CHECK(snoozed_until IS NULL OR length(trim(snoozed_until)) BETWEEN 1 AND 64),
  tags_json TEXT NOT NULL DEFAULT '[]' CHECK(length(tags_json) <= 8192 AND json_valid(tags_json) AND json_type(tags_json) = 'array' AND json_array_length(tags_json) <= 64),
  etag TEXT CHECK(etag IS NULL OR length(etag) <= 4096),
  remote_updated_at TEXT CHECK(remote_updated_at IS NULL OR length(trim(remote_updated_at)) BETWEEN 1 AND 64),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')) CHECK(length(trim(created_at)) BETWEEN 1 AND 64),
  updated_at TEXT NOT NULL CHECK(length(trim(updated_at)) BETWEEN 1 AND 64),
  deleted_at TEXT CHECK(deleted_at IS NULL OR length(trim(deleted_at)) BETWEEN 1 AND 64),
  UNIQUE(task_list_id, remote_id)
) STRICT, WITHOUT ROWID;

CREATE INDEX local_tasks_active_navigation
ON local_tasks(task_list_id, is_hidden, state, due_at, sort_order, updated_at DESC, id)
WHERE deleted_at IS NULL AND parent_task_id IS NULL;

CREATE INDEX local_tasks_active_subtasks
ON local_tasks(parent_task_id, is_hidden, sort_order, id)
WHERE deleted_at IS NULL;

CREATE TRIGGER local_tasks_validate_parent_insert
BEFORE INSERT ON local_tasks
WHEN NEW.parent_task_id IS NOT NULL
BEGIN
  SELECT CASE WHEN EXISTS (
    SELECT 1
    FROM local_tasks AS parent
    WHERE parent.id = NEW.parent_task_id AND parent.task_list_id != NEW.task_list_id
  ) THEN RAISE(ABORT, 'Task parent must belong to the same list') END;
  SELECT CASE WHEN EXISTS (
    WITH RECURSIVE ancestors(id, parent_task_id) AS (
      SELECT id, parent_task_id FROM local_tasks WHERE id = NEW.parent_task_id
      UNION ALL
      SELECT task.id, task.parent_task_id
      FROM local_tasks AS task
      JOIN ancestors ON task.id = ancestors.parent_task_id
    )
    SELECT 1 FROM ancestors WHERE id = NEW.id
  ) THEN RAISE(ABORT, 'Task parent cycle') END;
END;

CREATE TRIGGER local_tasks_validate_parent_update
BEFORE UPDATE OF parent_task_id, task_list_id ON local_tasks
BEGIN
  SELECT CASE WHEN NEW.parent_task_id IS NOT NULL AND EXISTS (
    SELECT 1
    FROM local_tasks AS parent
    WHERE parent.id = NEW.parent_task_id AND parent.task_list_id != NEW.task_list_id
  ) THEN RAISE(ABORT, 'Task parent must belong to the same list') END;
  SELECT CASE WHEN EXISTS (
    SELECT 1
    FROM local_tasks AS child
    WHERE child.parent_task_id = NEW.id AND child.task_list_id != NEW.task_list_id
  ) THEN RAISE(ABORT, 'Task children must belong to the same list') END;
  SELECT CASE WHEN NEW.parent_task_id IS NOT NULL AND EXISTS (
    WITH RECURSIVE ancestors(id, parent_task_id) AS (
      SELECT id, parent_task_id FROM local_tasks WHERE id = NEW.parent_task_id
      UNION ALL
      SELECT task.id, task.parent_task_id
      FROM local_tasks AS task
      JOIN ancestors ON task.id = ancestors.parent_task_id
    )
    SELECT 1 FROM ancestors WHERE id = NEW.id
  ) THEN RAISE(ABORT, 'Task parent cycle') END;
END
)";

constexpr char calendarSchemaSql[] = R"(
CREATE TABLE local_calendars (
  id TEXT PRIMARY KEY CHECK(length(trim(id)) BETWEEN 1 AND 256),
  account_id TEXT NOT NULL REFERENCES local_accounts(id) ON UPDATE CASCADE ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
  remote_id TEXT NOT NULL CHECK(length(trim(remote_id)) BETWEEN 1 AND 256),
  title TEXT NOT NULL CHECK(length(trim(title)) BETWEEN 1 AND 500),
  description TEXT CHECK(description IS NULL OR length(description) <= 20000),
  time_zone TEXT CHECK(time_zone IS NULL OR length(trim(time_zone)) BETWEEN 1 AND 120),
  background_color TEXT CHECK(background_color IS NULL OR (length(background_color) = 7 AND background_color GLOB '#[0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f]')),
  foreground_color TEXT CHECK(foreground_color IS NULL OR (length(foreground_color) = 7 AND foreground_color GLOB '#[0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f]')),
  access_role TEXT CHECK(access_role IS NULL OR access_role IN ('freeBusyReader', 'reader', 'writer', 'owner')),
  is_selected INTEGER NOT NULL DEFAULT 1 CHECK(is_selected IN (0, 1)),
  is_hidden INTEGER NOT NULL DEFAULT 0 CHECK(is_hidden IN (0, 1)),
  is_primary INTEGER NOT NULL DEFAULT 0 CHECK(is_primary IN (0, 1)),
  etag TEXT CHECK(etag IS NULL OR length(etag) <= 4096),
  remote_updated_at TEXT CHECK(remote_updated_at IS NULL OR length(trim(remote_updated_at)) BETWEEN 1 AND 64),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')) CHECK(length(trim(created_at)) BETWEEN 1 AND 64),
  updated_at TEXT NOT NULL CHECK(length(trim(updated_at)) BETWEEN 1 AND 64),
  deleted_at TEXT CHECK(deleted_at IS NULL OR length(trim(deleted_at)) BETWEEN 1 AND 64),
  UNIQUE(account_id, remote_id)
) STRICT, WITHOUT ROWID;

CREATE INDEX local_calendars_active_navigation
ON local_calendars(account_id, is_hidden, is_primary DESC, title COLLATE NOCASE, id)
WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX local_calendars_one_primary_per_account
ON local_calendars(account_id)
WHERE is_primary = 1 AND deleted_at IS NULL
)";

constexpr char calendarEventSchemaSql[] = R"(
CREATE TABLE local_calendar_events (
  id TEXT PRIMARY KEY CHECK(length(trim(id)) BETWEEN 1 AND 256),
  calendar_id TEXT NOT NULL REFERENCES local_calendars(id) ON UPDATE CASCADE ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
  remote_id TEXT CHECK(remote_id IS NULL OR length(trim(remote_id)) BETWEEN 1 AND 256),
  recurring_remote_id TEXT CHECK(recurring_remote_id IS NULL OR length(trim(recurring_remote_id)) BETWEEN 1 AND 256),
  original_start_at TEXT CHECK(original_start_at IS NULL OR (length(trim(original_start_at)) BETWEEN 1 AND 64 AND julianday(original_start_at) IS NOT NULL)),
  status TEXT NOT NULL DEFAULT 'confirmed' CHECK(status IN ('confirmed', 'tentative', 'cancelled')),
  title TEXT NOT NULL CHECK(length(trim(title)) BETWEEN 1 AND 500),
  description TEXT CHECK(description IS NULL OR length(description) <= 20000),
  location TEXT CHECK(location IS NULL OR length(location) <= 1000),
  start_at TEXT NOT NULL CHECK(length(trim(start_at)) BETWEEN 1 AND 64),
  start_time_zone TEXT CHECK(start_time_zone IS NULL OR length(trim(start_time_zone)) BETWEEN 1 AND 120),
  end_at TEXT NOT NULL CHECK(length(trim(end_at)) BETWEEN 1 AND 64),
  end_time_zone TEXT CHECK(end_time_zone IS NULL OR length(trim(end_time_zone)) BETWEEN 1 AND 120),
  is_all_day INTEGER NOT NULL DEFAULT 0 CHECK(is_all_day IN (0, 1)),
  recurrence_rule TEXT CHECK(recurrence_rule IS NULL OR length(recurrence_rule) BETWEEN 1 AND 4096),
  color_id TEXT CHECK(color_id IS NULL OR length(trim(color_id)) BETWEEN 1 AND 32),
  transparency TEXT CHECK(transparency IS NULL OR transparency IN ('opaque', 'transparent')),
  visibility TEXT CHECK(visibility IS NULL OR visibility IN ('default', 'public', 'private')),
  time_zone TEXT CHECK(time_zone IS NULL OR length(trim(time_zone)) BETWEEN 1 AND 120),
  hcb_kind TEXT CHECK(hcb_kind IS NULL OR hcb_kind = 'birthday'),
  tags_json TEXT NOT NULL DEFAULT '[]' CHECK(length(tags_json) <= 8192 AND json_valid(tags_json) AND json_type(tags_json) = 'array' AND json_array_length(tags_json) <= 64),
  attendee_emails_json TEXT NOT NULL DEFAULT '[]' CHECK(length(attendee_emails_json) <= 32768 AND json_valid(attendee_emails_json) AND json_type(attendee_emails_json) = 'array' AND json_array_length(attendee_emails_json) <= 50),
  attendee_details_json TEXT NOT NULL DEFAULT '[]' CHECK(length(attendee_details_json) <= 65536 AND json_valid(attendee_details_json) AND json_type(attendee_details_json) = 'array' AND json_array_length(attendee_details_json) <= 50),
  reminder_minutes_json TEXT NOT NULL DEFAULT '[]' CHECK(length(reminder_minutes_json) <= 8192 AND json_valid(reminder_minutes_json) AND json_type(reminder_minutes_json) = 'array' AND json_array_length(reminder_minutes_json) <= 10),
  reminders_json TEXT NOT NULL DEFAULT '[]' CHECK(length(reminders_json) <= 8192 AND json_valid(reminders_json) AND json_type(reminders_json) = 'array' AND json_array_length(reminders_json) <= 10),
  reminders_use_default INTEGER NOT NULL DEFAULT 0 CHECK(reminders_use_default IN (0, 1)),
  conference_json TEXT CHECK(conference_json IS NULL OR (length(conference_json) <= 32768 AND json_valid(conference_json) AND json_type(conference_json) = 'object')),
  etag TEXT CHECK(etag IS NULL OR length(etag) <= 4096),
  sequence INTEGER CHECK(sequence IS NULL OR sequence >= 0),
  remote_updated_at TEXT CHECK(remote_updated_at IS NULL OR length(trim(remote_updated_at)) BETWEEN 1 AND 64),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')) CHECK(length(trim(created_at)) BETWEEN 1 AND 64),
  updated_at TEXT NOT NULL CHECK(length(trim(updated_at)) BETWEEN 1 AND 64),
  deleted_at TEXT CHECK(deleted_at IS NULL OR length(trim(deleted_at)) BETWEEN 1 AND 64),
  CHECK(status = 'cancelled' OR (julianday(start_at) IS NOT NULL AND julianday(end_at) IS NOT NULL AND julianday(end_at) > julianday(start_at)))
) STRICT, WITHOUT ROWID;

CREATE UNIQUE INDEX local_calendar_events_remote_identity
ON local_calendar_events(calendar_id, remote_id)
WHERE remote_id IS NOT NULL;

CREATE INDEX local_calendar_events_active_range
ON local_calendar_events(calendar_id, start_at, end_at, id)
WHERE deleted_at IS NULL AND status != 'cancelled';

CREATE INDEX local_calendar_events_recurrence_instances
ON local_calendar_events(calendar_id, recurring_remote_id, original_start_at, id)
WHERE deleted_at IS NULL AND recurring_remote_id IS NOT NULL
)";

constexpr char calendarEventTypeSchemaSql[] = R"(
ALTER TABLE local_calendar_events
ADD COLUMN event_type TEXT CHECK(event_type IS NULL OR event_type IN (
  'default', 'birthday', 'focusTime', 'fromGmail', 'outOfOffice', 'workingLocation'
))
)";

constexpr char mutationTelemetrySchemaSql[] = R"(
CREATE TABLE local_mutation_telemetry (
  id TEXT PRIMARY KEY CHECK(length(trim(id)) BETWEEN 1 AND 128),
  mutation_id TEXT NOT NULL DEFAULT '' CHECK(length(mutation_id) <= 128),
  resource TEXT NOT NULL CHECK(resource IN ('task', 'task_list', 'event')),
  operation TEXT NOT NULL CHECK(length(trim(operation)) BETWEEN 1 AND 128),
  scope TEXT NOT NULL CHECK(scope IN ('none', 'this_instance', 'this_and_following', 'full_series')),
  all_day INTEGER NOT NULL CHECK(all_day IN (0, 1)),
  target_start_at TEXT CHECK(target_start_at IS NULL OR length(trim(target_start_at)) BETWEEN 1 AND 64),
  target_end_at TEXT CHECK(target_end_at IS NULL OR length(trim(target_end_at)) BETWEEN 1 AND 64),
  phase TEXT NOT NULL CHECK(phase IN ('intent', 'remote_applied', 'remote_failed', 'rollback')),
  remote_outcome TEXT CHECK(remote_outcome IS NULL OR length(trim(remote_outcome)) BETWEEN 1 AND 128),
  error_code TEXT CHECK(error_code IS NULL OR length(trim(error_code)) BETWEEN 1 AND 128),
  rollback_reason TEXT CHECK(rollback_reason IS NULL OR length(trim(rollback_reason)) BETWEEN 1 AND 128),
  created_at TEXT NOT NULL CHECK(length(trim(created_at)) BETWEEN 1 AND 64)
) STRICT, WITHOUT ROWID;

CREATE INDEX local_mutation_telemetry_recency
ON local_mutation_telemetry(created_at DESC, id DESC)
)";

constexpr char calendarEventMetadataSchemaSql[] = R"(
DROP TRIGGER local_calendar_events_fts_insert;
DROP TRIGGER local_calendar_events_fts_delete;
DROP TRIGGER local_calendar_events_fts_update;
DROP INDEX local_calendar_events_remote_identity;
DROP INDEX local_calendar_events_active_range;
DROP INDEX local_calendar_events_recurrence_instances;
ALTER TABLE local_calendar_events RENAME TO local_calendar_events_legacy;

CREATE TABLE local_calendar_events (
  id TEXT PRIMARY KEY CHECK(length(trim(id)) BETWEEN 1 AND 256),
  calendar_id TEXT NOT NULL REFERENCES local_calendars(id) ON UPDATE CASCADE ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
  remote_id TEXT CHECK(remote_id IS NULL OR length(trim(remote_id)) BETWEEN 1 AND 256),
  recurring_remote_id TEXT CHECK(recurring_remote_id IS NULL OR length(trim(recurring_remote_id)) BETWEEN 1 AND 256),
  original_start_at TEXT CHECK(original_start_at IS NULL OR (length(trim(original_start_at)) BETWEEN 1 AND 64 AND julianday(original_start_at) IS NOT NULL)),
  status TEXT NOT NULL DEFAULT 'confirmed' CHECK(status IN ('confirmed', 'tentative', 'cancelled')),
  title TEXT NOT NULL CHECK(length(trim(title)) BETWEEN 1 AND 500),
  description TEXT CHECK(description IS NULL OR length(description) <= 20000),
  location TEXT CHECK(location IS NULL OR length(location) <= 1000),
  start_at TEXT NOT NULL CHECK(length(trim(start_at)) BETWEEN 1 AND 64),
  start_time_zone TEXT CHECK(start_time_zone IS NULL OR length(trim(start_time_zone)) BETWEEN 1 AND 120),
  end_at TEXT NOT NULL CHECK(length(trim(end_at)) BETWEEN 1 AND 64),
  end_time_zone TEXT CHECK(end_time_zone IS NULL OR length(trim(end_time_zone)) BETWEEN 1 AND 120),
  is_all_day INTEGER NOT NULL DEFAULT 0 CHECK(is_all_day IN (0, 1)),
  recurrence_rule TEXT CHECK(recurrence_rule IS NULL OR length(recurrence_rule) BETWEEN 1 AND 4096),
  color_id TEXT CHECK(color_id IS NULL OR length(trim(color_id)) BETWEEN 1 AND 32),
  transparency TEXT CHECK(transparency IS NULL OR transparency IN ('opaque', 'transparent')),
  visibility TEXT CHECK(visibility IS NULL OR visibility IN ('default', 'public', 'private', 'confidential')),
  time_zone TEXT CHECK(time_zone IS NULL OR length(trim(time_zone)) BETWEEN 1 AND 120),
  hcb_kind TEXT CHECK(hcb_kind IS NULL OR hcb_kind = 'birthday'),
  tags_json TEXT NOT NULL DEFAULT '[]' CHECK(length(tags_json) <= 8192 AND json_valid(tags_json) AND json_type(tags_json) = 'array' AND json_array_length(tags_json) <= 64),
  attendee_emails_json TEXT NOT NULL DEFAULT '[]' CHECK(length(attendee_emails_json) <= 65536 AND json_valid(attendee_emails_json) AND json_type(attendee_emails_json) = 'array' AND json_array_length(attendee_emails_json) <= 200),
  attendee_details_json TEXT NOT NULL DEFAULT '[]' CHECK(length(attendee_details_json) <= 262144 AND json_valid(attendee_details_json) AND json_type(attendee_details_json) = 'array' AND json_array_length(attendee_details_json) <= 200),
  reminder_minutes_json TEXT NOT NULL DEFAULT '[]' CHECK(length(reminder_minutes_json) <= 8192 AND json_valid(reminder_minutes_json) AND json_type(reminder_minutes_json) = 'array' AND json_array_length(reminder_minutes_json) <= 5),
  reminders_json TEXT NOT NULL DEFAULT '[]' CHECK(length(reminders_json) <= 8192 AND json_valid(reminders_json) AND json_type(reminders_json) = 'array' AND json_array_length(reminders_json) <= 5),
  reminders_use_default INTEGER NOT NULL DEFAULT 0 CHECK(reminders_use_default IN (0, 1)),
  conference_json TEXT CHECK(conference_json IS NULL OR (length(conference_json) <= 32768 AND json_valid(conference_json) AND json_type(conference_json) = 'object')),
  etag TEXT CHECK(etag IS NULL OR length(etag) <= 4096),
  sequence INTEGER CHECK(sequence IS NULL OR sequence >= 0),
  remote_updated_at TEXT CHECK(remote_updated_at IS NULL OR length(trim(remote_updated_at)) BETWEEN 1 AND 64),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')) CHECK(length(trim(created_at)) BETWEEN 1 AND 64),
  updated_at TEXT NOT NULL CHECK(length(trim(updated_at)) BETWEEN 1 AND 64),
  deleted_at TEXT CHECK(deleted_at IS NULL OR length(trim(deleted_at)) BETWEEN 1 AND 64),
  event_type TEXT CHECK(event_type IS NULL OR event_type IN ('default', 'birthday', 'focusTime', 'fromGmail', 'outOfOffice', 'workingLocation')),
  CHECK(status = 'cancelled' OR (julianday(start_at) IS NOT NULL AND julianday(end_at) IS NOT NULL AND julianday(end_at) > julianday(start_at)))
) STRICT, WITHOUT ROWID;

INSERT INTO local_calendar_events (
  id, calendar_id, remote_id, recurring_remote_id, original_start_at, status, title, description,
  location, start_at, start_time_zone, end_at, end_time_zone, is_all_day, recurrence_rule,
  color_id, transparency, visibility, time_zone, hcb_kind, tags_json, attendee_emails_json,
  attendee_details_json, reminder_minutes_json, reminders_json, reminders_use_default,
  conference_json, etag, sequence, remote_updated_at, created_at, updated_at, deleted_at, event_type
)
SELECT id, calendar_id, remote_id, recurring_remote_id, original_start_at, status, title, description,
       location, start_at, start_time_zone, end_at, end_time_zone, is_all_day, recurrence_rule,
       color_id, transparency, visibility, time_zone, hcb_kind, tags_json, attendee_emails_json,
       attendee_details_json,
       COALESCE((SELECT json_group_array(json(value))
                 FROM (SELECT value FROM json_each(local_calendar_events_legacy.reminder_minutes_json)
                       ORDER BY CAST(key AS INTEGER) LIMIT 5)), '[]'),
       COALESCE((SELECT json_group_array(json(value))
                 FROM (SELECT value FROM json_each(local_calendar_events_legacy.reminders_json)
                       ORDER BY CAST(key AS INTEGER) LIMIT 5)), '[]'),
       reminders_use_default,
       conference_json, etag, sequence, remote_updated_at, created_at, updated_at, deleted_at, event_type
FROM local_calendar_events_legacy;
DROP TABLE local_calendar_events_legacy;

CREATE UNIQUE INDEX local_calendar_events_remote_identity
ON local_calendar_events(calendar_id, remote_id)
WHERE remote_id IS NOT NULL;
CREATE INDEX local_calendar_events_active_range
ON local_calendar_events(calendar_id, start_at, end_at, id)
WHERE deleted_at IS NULL AND status != 'cancelled';
CREATE INDEX local_calendar_events_recurrence_instances
ON local_calendar_events(calendar_id, recurring_remote_id, original_start_at, id)
WHERE deleted_at IS NULL AND recurring_remote_id IS NOT NULL;

CREATE TRIGGER local_calendar_events_fts_insert
AFTER INSERT ON local_calendar_events
BEGIN
  INSERT INTO local_calendar_events_fts(calendar_event_id, title, description, location)
  VALUES (NEW.id, NEW.title, COALESCE(NEW.description, ''), COALESCE(NEW.location, ''));
END;
CREATE TRIGGER local_calendar_events_fts_delete
AFTER DELETE ON local_calendar_events
BEGIN
  DELETE FROM local_calendar_events_fts WHERE calendar_event_id = OLD.id;
END;
CREATE TRIGGER local_calendar_events_fts_update
AFTER UPDATE OF id, title, description, location ON local_calendar_events
BEGIN
  DELETE FROM local_calendar_events_fts WHERE calendar_event_id = OLD.id;
  INSERT INTO local_calendar_events_fts(calendar_event_id, title, description, location)
  VALUES (NEW.id, NEW.title, COALESCE(NEW.description, ''), COALESCE(NEW.location, ''));
END;
DELETE FROM local_calendar_events_fts;
INSERT INTO local_calendar_events_fts(calendar_event_id, title, description, location)
SELECT id, title, COALESCE(description, ''), COALESCE(location, '') FROM local_calendar_events
)";

constexpr char noteProjectionSchemaSql[] = R"(
CREATE VIEW local_note_projections AS
SELECT tasks.id,
       tasks.task_list_id AS list_id,
       lists.title AS list_title,
       tasks.title,
       COALESCE(tasks.notes, '') AS body,
       tasks.tags_json,
       tasks.updated_at
FROM local_tasks AS tasks
INNER JOIN local_task_lists AS lists ON lists.id = tasks.task_list_id
WHERE tasks.deleted_at IS NULL
  AND tasks.is_hidden = 0
  AND tasks.state = 'active'
  AND tasks.parent_task_id IS NULL
  AND tasks.due_at IS NULL
  AND lists.deleted_at IS NULL;

CREATE INDEX local_tasks_active_note_recency
ON local_tasks(updated_at DESC, id)
WHERE deleted_at IS NULL
  AND is_hidden = 0
  AND state = 'active'
  AND parent_task_id IS NULL
  AND due_at IS NULL
)";

constexpr char pendingMutationSchemaSql[] = R"(
CREATE TABLE local_pending_mutations (
  id TEXT PRIMARY KEY CHECK(length(trim(id)) BETWEEN 1 AND 256),
  account_id TEXT REFERENCES local_accounts(id) ON UPDATE CASCADE ON DELETE SET NULL DEFERRABLE INITIALLY DEFERRED,
  resource_type TEXT NOT NULL CHECK(resource_type IN ('task', 'task_list', 'event')),
  resource_id TEXT NOT NULL CHECK(length(trim(resource_id)) BETWEEN 1 AND 256),
  operation TEXT NOT NULL CHECK(length(trim(operation)) BETWEEN 1 AND 128),
  payload_json TEXT NOT NULL CHECK(length(payload_json) <= 262144 AND json_valid(payload_json) AND json_type(payload_json) = 'object'),
  status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'applying', 'failed', 'applied', 'cancelled')),
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK(attempt_count >= 0),
  next_retry_at TEXT CHECK(next_retry_at IS NULL OR length(trim(next_retry_at)) BETWEEN 1 AND 64),
  lease_id TEXT CHECK(lease_id IS NULL OR length(trim(lease_id)) BETWEEN 1 AND 256),
  lease_expires_at TEXT CHECK(lease_expires_at IS NULL OR length(trim(lease_expires_at)) BETWEEN 1 AND 64),
  last_error_code TEXT CHECK(last_error_code IS NULL OR length(trim(last_error_code)) BETWEEN 1 AND 64),
  last_error_message TEXT CHECK(last_error_message IS NULL OR length(last_error_message) <= 4096),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')) CHECK(length(trim(created_at)) BETWEEN 1 AND 64),
  updated_at TEXT NOT NULL CHECK(length(trim(updated_at)) BETWEEN 1 AND 64),
  applied_at TEXT CHECK(applied_at IS NULL OR length(trim(applied_at)) BETWEEN 1 AND 64),
  CHECK((status = 'applying' AND lease_id IS NOT NULL AND lease_expires_at IS NOT NULL) OR (status != 'applying' AND lease_id IS NULL AND lease_expires_at IS NULL)),
  CHECK((status = 'applied' AND applied_at IS NOT NULL) OR (status != 'applied' AND applied_at IS NULL))
) STRICT, WITHOUT ROWID;

CREATE INDEX local_pending_mutations_due
ON local_pending_mutations(status, next_retry_at, created_at, id)
WHERE status IN ('pending', 'failed');

CREATE INDEX local_pending_mutations_active_resource
ON local_pending_mutations(resource_type, resource_id, status, created_at, id)
WHERE status IN ('pending', 'applying', 'failed');

CREATE INDEX local_pending_mutations_expired_lease
ON local_pending_mutations(lease_expires_at, created_at, id)
WHERE status = 'applying'
)";

constexpr char undoSchemaSql[] = R"(
CREATE TABLE local_undo_entries (
  id TEXT PRIMARY KEY CHECK(length(trim(id)) BETWEEN 1 AND 256),
  session_id TEXT NOT NULL CHECK(length(trim(session_id)) BETWEEN 1 AND 256),
  stack TEXT NOT NULL CHECK(stack IN ('undo', 'redo')),
  ordinal INTEGER NOT NULL CHECK(ordinal >= 0),
  action_kind TEXT NOT NULL CHECK(length(trim(action_kind)) BETWEEN 1 AND 128),
  label TEXT NOT NULL CHECK(length(trim(label)) BETWEEN 1 AND 512),
  resource_type TEXT NOT NULL CHECK(resource_type IN ('task', 'task_list', 'event')),
  resource_id TEXT NOT NULL CHECK(length(trim(resource_id)) BETWEEN 1 AND 256),
  before_json TEXT NOT NULL CHECK(length(before_json) <= 262144 AND json_valid(before_json) AND json_type(before_json) = 'object'),
  after_json TEXT NOT NULL CHECK(length(after_json) <= 262144 AND json_valid(after_json) AND json_type(after_json) = 'object'),
  created_at TEXT NOT NULL CHECK(length(trim(created_at)) BETWEEN 1 AND 64),
  updated_at TEXT NOT NULL CHECK(length(trim(updated_at)) BETWEEN 1 AND 64),
  applied_at TEXT CHECK(applied_at IS NULL OR length(trim(applied_at)) BETWEEN 1 AND 64),
  UNIQUE(session_id, stack, ordinal)
) STRICT, WITHOUT ROWID;

CREATE INDEX local_undo_entries_session_stack
ON local_undo_entries(session_id, stack, ordinal DESC, id DESC);

CREATE INDEX local_undo_entries_recovery
ON local_undo_entries(created_at, id)
)";

constexpr char syncCheckpointSchemaSql[] = R"(
CREATE TABLE local_sync_checkpoints (
  id TEXT PRIMARY KEY CHECK(length(trim(id)) BETWEEN 1 AND 256),
  account_id TEXT NOT NULL REFERENCES local_accounts(id) ON UPDATE CASCADE ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
  resource_type TEXT NOT NULL CHECK(resource_type IN ('tasks', 'task_list', 'calendar', 'calendar_list', 'calendar_event')),
  resource_id TEXT NOT NULL CHECK(length(trim(resource_id)) BETWEEN 1 AND 256),
  checkpoint_type TEXT NOT NULL CHECK(length(trim(checkpoint_type)) BETWEEN 1 AND 128),
  checkpoint_value TEXT NOT NULL CHECK(length(checkpoint_value) BETWEEN 1 AND 16384),
  metadata_json TEXT NOT NULL DEFAULT '{}' CHECK(length(metadata_json) <= 16384 AND json_valid(metadata_json) AND json_type(metadata_json) = 'object'),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')) CHECK(length(trim(created_at)) BETWEEN 1 AND 64),
  last_successful_sync_at TEXT NOT NULL CHECK(length(trim(last_successful_sync_at)) BETWEEN 1 AND 64),
  updated_at TEXT NOT NULL CHECK(length(trim(updated_at)) BETWEEN 1 AND 64),
  UNIQUE(account_id, resource_type, resource_id, checkpoint_type)
) STRICT, WITHOUT ROWID;

CREATE INDEX local_sync_checkpoints_account_recency
ON local_sync_checkpoints(account_id, updated_at DESC, id)
)";

constexpr char syncConflictSchemaSql[] = R"(
CREATE TABLE local_sync_conflicts (
  id TEXT PRIMARY KEY CHECK(length(trim(id)) BETWEEN 1 AND 256),
  account_id TEXT REFERENCES local_accounts(id) ON UPDATE CASCADE ON DELETE SET NULL DEFERRABLE INITIALLY DEFERRED,
  resource_type TEXT NOT NULL CHECK(resource_type IN ('task', 'task_list', 'event')),
  resource_id TEXT NOT NULL CHECK(length(trim(resource_id)) BETWEEN 1 AND 256),
  mutation_id TEXT NOT NULL UNIQUE CHECK(length(trim(mutation_id)) BETWEEN 1 AND 256),
  error_code TEXT NOT NULL CHECK(length(trim(error_code)) BETWEEN 1 AND 64),
  error_message TEXT NOT NULL CHECK(length(error_message) BETWEEN 1 AND 4096),
  payload_json TEXT NOT NULL CHECK(length(payload_json) <= 262144 AND json_valid(payload_json) AND json_type(payload_json) = 'object'),
  status TEXT NOT NULL DEFAULT 'unresolved' CHECK(status IN ('unresolved', 'resolved')),
  resolution TEXT CHECK(resolution IS NULL OR resolution IN ('keep_local', 'keep_remote')),
  created_at TEXT NOT NULL CHECK(length(trim(created_at)) BETWEEN 1 AND 64),
  updated_at TEXT NOT NULL CHECK(length(trim(updated_at)) BETWEEN 1 AND 64),
  resolved_at TEXT CHECK(resolved_at IS NULL OR length(trim(resolved_at)) BETWEEN 1 AND 64),
  CHECK((status = 'unresolved' AND resolution IS NULL AND resolved_at IS NULL) OR
        (status = 'resolved' AND resolution IS NOT NULL AND resolved_at IS NOT NULL))
) STRICT, WITHOUT ROWID;

CREATE INDEX local_sync_conflicts_unresolved
ON local_sync_conflicts(status, updated_at, id)
WHERE status = 'unresolved'
)";

constexpr char ftsSchemaSql[] = R"(
CREATE VIRTUAL TABLE local_task_lists_fts
USING fts5(
  task_list_id UNINDEXED,
  title,
  tokenize = 'unicode61 remove_diacritics 2',
  prefix = '2 3'
);

CREATE VIRTUAL TABLE local_tasks_fts
USING fts5(
  task_id UNINDEXED,
  title,
  notes,
  tokenize = 'unicode61 remove_diacritics 2',
  prefix = '2 3'
);

CREATE VIRTUAL TABLE local_calendars_fts
USING fts5(
  calendar_id UNINDEXED,
  title,
  tokenize = 'unicode61 remove_diacritics 2',
  prefix = '2 3'
);

CREATE VIRTUAL TABLE local_calendar_events_fts
USING fts5(
  calendar_event_id UNINDEXED,
  title,
  description,
  location,
  tokenize = 'unicode61 remove_diacritics 2',
  prefix = '2 3'
);

CREATE TRIGGER local_task_lists_fts_insert
AFTER INSERT ON local_task_lists
BEGIN
  INSERT INTO local_task_lists_fts(task_list_id, title)
  VALUES (NEW.id, NEW.title);
END;

CREATE TRIGGER local_task_lists_fts_delete
AFTER DELETE ON local_task_lists
BEGIN
  DELETE FROM local_task_lists_fts WHERE task_list_id = OLD.id;
END;

CREATE TRIGGER local_task_lists_fts_update
AFTER UPDATE OF id, title ON local_task_lists
BEGIN
  DELETE FROM local_task_lists_fts WHERE task_list_id = OLD.id;
  INSERT INTO local_task_lists_fts(task_list_id, title)
  VALUES (NEW.id, NEW.title);
END;

CREATE TRIGGER local_tasks_fts_insert
AFTER INSERT ON local_tasks
BEGIN
  INSERT INTO local_tasks_fts(task_id, title, notes)
  VALUES (NEW.id, NEW.title, COALESCE(NEW.notes, ''));
END;

CREATE TRIGGER local_tasks_fts_delete
AFTER DELETE ON local_tasks
BEGIN
  DELETE FROM local_tasks_fts WHERE task_id = OLD.id;
END;

CREATE TRIGGER local_tasks_fts_update
AFTER UPDATE OF id, title, notes ON local_tasks
BEGIN
  DELETE FROM local_tasks_fts WHERE task_id = OLD.id;
  INSERT INTO local_tasks_fts(task_id, title, notes)
  VALUES (NEW.id, NEW.title, COALESCE(NEW.notes, ''));
END;

CREATE TRIGGER local_calendars_fts_insert
AFTER INSERT ON local_calendars
BEGIN
  INSERT INTO local_calendars_fts(calendar_id, title)
  VALUES (NEW.id, NEW.title);
END;

CREATE TRIGGER local_calendars_fts_delete
AFTER DELETE ON local_calendars
BEGIN
  DELETE FROM local_calendars_fts WHERE calendar_id = OLD.id;
END;

CREATE TRIGGER local_calendars_fts_update
AFTER UPDATE OF id, title ON local_calendars
BEGIN
  DELETE FROM local_calendars_fts WHERE calendar_id = OLD.id;
  INSERT INTO local_calendars_fts(calendar_id, title)
  VALUES (NEW.id, NEW.title);
END;

CREATE TRIGGER local_calendar_events_fts_insert
AFTER INSERT ON local_calendar_events
BEGIN
  INSERT INTO local_calendar_events_fts(calendar_event_id, title, description, location)
  VALUES (NEW.id, NEW.title, COALESCE(NEW.description, ''), COALESCE(NEW.location, ''));
END;

CREATE TRIGGER local_calendar_events_fts_delete
AFTER DELETE ON local_calendar_events
BEGIN
  DELETE FROM local_calendar_events_fts WHERE calendar_event_id = OLD.id;
END;

CREATE TRIGGER local_calendar_events_fts_update
AFTER UPDATE OF id, title, description, location ON local_calendar_events
BEGIN
  DELETE FROM local_calendar_events_fts WHERE calendar_event_id = OLD.id;
  INSERT INTO local_calendar_events_fts(calendar_event_id, title, description, location)
  VALUES (NEW.id, NEW.title, COALESCE(NEW.description, ''), COALESCE(NEW.location, ''));
END;

INSERT INTO local_task_lists_fts(task_list_id, title)
SELECT id, title FROM local_task_lists;

INSERT INTO local_tasks_fts(task_id, title, notes)
SELECT id, title, COALESCE(notes, '') FROM local_tasks;

INSERT INTO local_calendars_fts(calendar_id, title)
SELECT id, title FROM local_calendars;

INSERT INTO local_calendar_events_fts(calendar_event_id, title, description, location)
SELECT id, title, COALESCE(description, ''), COALESCE(location, '') FROM local_calendar_events
)";

constexpr char noteFtsSchemaSql[] = R"(
CREATE VIRTUAL TABLE local_notes_fts
USING fts5(
  note_id UNINDEXED,
  list_id UNINDEXED,
  title,
  body,
  tags,
  tokenize = 'unicode61 remove_diacritics 2',
  prefix = '2 3'
);

CREATE TRIGGER local_notes_fts_task_insert
AFTER INSERT ON local_tasks
WHEN NEW.deleted_at IS NULL
  AND NEW.is_hidden = 0
  AND NEW.state = 'active'
  AND NEW.parent_task_id IS NULL
  AND NEW.due_at IS NULL
  AND EXISTS (SELECT 1 FROM local_task_lists WHERE id = NEW.task_list_id AND deleted_at IS NULL)
BEGIN
  INSERT INTO local_notes_fts(note_id, list_id, title, body, tags)
  VALUES (NEW.id, NEW.task_list_id, NEW.title, COALESCE(NEW.notes, ''), NEW.tags_json);
END;

CREATE TRIGGER local_notes_fts_task_delete
AFTER DELETE ON local_tasks
BEGIN
  DELETE FROM local_notes_fts WHERE note_id = OLD.id;
END;

CREATE TRIGGER local_notes_fts_task_update
AFTER UPDATE OF id, task_list_id, title, notes, tags_json, deleted_at, is_hidden, state, parent_task_id, due_at ON local_tasks
BEGIN
  DELETE FROM local_notes_fts WHERE note_id = OLD.id;
  INSERT INTO local_notes_fts(note_id, list_id, title, body, tags)
  SELECT NEW.id, NEW.task_list_id, NEW.title, COALESCE(NEW.notes, ''), NEW.tags_json
  WHERE NEW.deleted_at IS NULL
    AND NEW.is_hidden = 0
    AND NEW.state = 'active'
    AND NEW.parent_task_id IS NULL
    AND NEW.due_at IS NULL
    AND EXISTS (SELECT 1 FROM local_task_lists WHERE id = NEW.task_list_id AND deleted_at IS NULL);
END;

CREATE TRIGGER local_notes_fts_task_list_update
AFTER UPDATE OF id, deleted_at ON local_task_lists
BEGIN
  DELETE FROM local_notes_fts WHERE list_id = OLD.id OR list_id = NEW.id;
  INSERT INTO local_notes_fts(note_id, list_id, title, body, tags)
  SELECT tasks.id, tasks.task_list_id, tasks.title, COALESCE(tasks.notes, ''), tasks.tags_json
  FROM local_tasks AS tasks
  WHERE tasks.task_list_id = NEW.id
    AND NEW.deleted_at IS NULL
    AND tasks.deleted_at IS NULL
    AND tasks.is_hidden = 0
    AND tasks.state = 'active'
    AND tasks.parent_task_id IS NULL
    AND tasks.due_at IS NULL;
END;

INSERT INTO local_notes_fts(note_id, list_id, title, body, tags)
SELECT tasks.id, tasks.task_list_id, tasks.title, COALESCE(tasks.notes, ''), tasks.tags_json
FROM local_tasks AS tasks
INNER JOIN local_task_lists AS lists ON lists.id = tasks.task_list_id
WHERE tasks.deleted_at IS NULL
  AND tasks.is_hidden = 0
  AND tasks.state = 'active'
  AND tasks.parent_task_id IS NULL
  AND tasks.due_at IS NULL
  AND lists.deleted_at IS NULL
)";

constexpr char allUndatedNotesSchemaSql[] = R"(
DROP VIEW local_note_projections;
DROP INDEX local_tasks_active_note_recency;
CREATE VIEW local_note_projections AS
SELECT tasks.id,
       tasks.task_list_id AS list_id,
       lists.title AS list_title,
       tasks.title,
       COALESCE(tasks.notes, '') AS body,
       tasks.tags_json,
       tasks.updated_at
FROM local_tasks AS tasks
INNER JOIN local_task_lists AS lists ON lists.id = tasks.task_list_id
WHERE tasks.deleted_at IS NULL
  AND tasks.is_hidden = 0
  AND tasks.due_at IS NULL
  AND lists.deleted_at IS NULL;

CREATE INDEX local_tasks_active_note_recency
ON local_tasks(updated_at DESC, id)
WHERE deleted_at IS NULL
  AND is_hidden = 0
  AND due_at IS NULL;

DROP TRIGGER local_notes_fts_task_insert;
DROP TRIGGER local_notes_fts_task_delete;
DROP TRIGGER local_notes_fts_task_update;
DROP TRIGGER local_notes_fts_task_list_update;
DROP TABLE local_notes_fts;

CREATE VIRTUAL TABLE local_notes_fts
USING fts5(
  note_id UNINDEXED,
  list_id UNINDEXED,
  title,
  body,
  tags,
  tokenize = 'unicode61 remove_diacritics 2',
  prefix = '2 3'
);

CREATE TRIGGER local_notes_fts_task_insert
AFTER INSERT ON local_tasks
WHEN NEW.deleted_at IS NULL
  AND NEW.is_hidden = 0
  AND NEW.due_at IS NULL
  AND EXISTS (SELECT 1 FROM local_task_lists WHERE id = NEW.task_list_id AND deleted_at IS NULL)
BEGIN
  INSERT INTO local_notes_fts(note_id, list_id, title, body, tags)
  VALUES (NEW.id, NEW.task_list_id, NEW.title, COALESCE(NEW.notes, ''), NEW.tags_json);
END;

CREATE TRIGGER local_notes_fts_task_delete
AFTER DELETE ON local_tasks
BEGIN
  DELETE FROM local_notes_fts WHERE note_id = OLD.id;
END;

CREATE TRIGGER local_notes_fts_task_update
AFTER UPDATE OF id, task_list_id, title, notes, tags_json, deleted_at, is_hidden, state, parent_task_id, due_at ON local_tasks
BEGIN
  DELETE FROM local_notes_fts WHERE note_id = OLD.id;
  INSERT INTO local_notes_fts(note_id, list_id, title, body, tags)
  SELECT NEW.id, NEW.task_list_id, NEW.title, COALESCE(NEW.notes, ''), NEW.tags_json
  WHERE NEW.deleted_at IS NULL
    AND NEW.is_hidden = 0
    AND NEW.due_at IS NULL
    AND EXISTS (SELECT 1 FROM local_task_lists WHERE id = NEW.task_list_id AND deleted_at IS NULL);
END;

CREATE TRIGGER local_notes_fts_task_list_update
AFTER UPDATE OF id, deleted_at ON local_task_lists
BEGIN
  DELETE FROM local_notes_fts WHERE list_id = OLD.id OR list_id = NEW.id;
  INSERT INTO local_notes_fts(note_id, list_id, title, body, tags)
  SELECT tasks.id, tasks.task_list_id, tasks.title, COALESCE(tasks.notes, ''), tasks.tags_json
  FROM local_tasks AS tasks
  WHERE tasks.task_list_id = NEW.id
    AND NEW.deleted_at IS NULL
    AND tasks.deleted_at IS NULL
    AND tasks.is_hidden = 0
    AND tasks.due_at IS NULL;
END;

INSERT INTO local_notes_fts(note_id, list_id, title, body, tags)
SELECT tasks.id, tasks.task_list_id, tasks.title, COALESCE(tasks.notes, ''), tasks.tags_json
FROM local_tasks AS tasks
INNER JOIN local_task_lists AS lists ON lists.id = tasks.task_list_id
WHERE tasks.deleted_at IS NULL
  AND tasks.is_hidden = 0
  AND tasks.due_at IS NULL
  AND lists.deleted_at IS NULL
)";

constexpr char taskRecurrenceClaimsSchemaSql[] = R"(
CREATE TABLE local_task_recurrence_claims (
  account_id TEXT NOT NULL REFERENCES local_accounts(id) ON UPDATE CASCADE ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
  series_id TEXT NOT NULL CHECK(length(trim(series_id)) BETWEEN 1 AND 64),
  occurrence_id TEXT NOT NULL CHECK(length(trim(occurrence_id)) BETWEEN 3 AND 80),
  source_task_id TEXT NOT NULL UNIQUE REFERENCES local_tasks(id) ON UPDATE CASCADE ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
  successor_task_id TEXT UNIQUE REFERENCES local_tasks(id) ON UPDATE CASCADE ON DELETE SET NULL DEFERRABLE INITIALLY DEFERRED,
  created_at TEXT NOT NULL CHECK(length(trim(created_at)) BETWEEN 1 AND 64),
  updated_at TEXT NOT NULL CHECK(length(trim(updated_at)) BETWEEN 1 AND 64),
  PRIMARY KEY(account_id, series_id, occurrence_id)
) STRICT, WITHOUT ROWID;

CREATE INDEX local_task_recurrence_claims_successor
ON local_task_recurrence_claims(successor_task_id)
WHERE successor_task_id IS NOT NULL
)";

constexpr char taskRecurrenceSafetySchemaSql[] = R"(
ALTER TABLE local_tasks
ADD COLUMN is_assigned INTEGER NOT NULL DEFAULT 0 CHECK(is_assigned IN (0, 1));

ALTER TABLE local_tasks
ADD COLUMN recurrence_diagnostic TEXT CHECK(recurrence_diagnostic IS NULL OR length(recurrence_diagnostic) BETWEEN 1 AND 1024)
)";

constexpr char calendarInstanceCacheSchemaSql[] = R"(
ALTER TABLE local_calendar_events
ADD COLUMN is_instance_cache INTEGER NOT NULL DEFAULT 0 CHECK(is_instance_cache IN (0, 1));

CREATE INDEX local_calendar_events_instance_cache
ON local_calendar_events(calendar_id, recurring_remote_id, original_start_at, is_instance_cache)
WHERE deleted_at IS NULL AND is_instance_cache = 1;

CREATE TABLE local_calendar_instance_coverage (
  calendar_id TEXT NOT NULL REFERENCES local_calendars(id) ON UPDATE CASCADE ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
  recurring_remote_id TEXT NOT NULL CHECK(length(trim(recurring_remote_id)) BETWEEN 1 AND 256),
  range_start_at TEXT NOT NULL CHECK(length(trim(range_start_at)) BETWEEN 1 AND 64 AND julianday(range_start_at) IS NOT NULL),
  range_end_at TEXT NOT NULL CHECK(length(trim(range_end_at)) BETWEEN 1 AND 64 AND julianday(range_end_at) IS NOT NULL AND julianday(range_end_at) > julianday(range_start_at)),
  fetched_at TEXT NOT NULL CHECK(length(trim(fetched_at)) BETWEEN 1 AND 64 AND julianday(fetched_at) IS NOT NULL),
  expires_at TEXT NOT NULL CHECK(length(trim(expires_at)) BETWEEN 1 AND 64 AND julianday(expires_at) IS NOT NULL AND julianday(expires_at) > julianday(fetched_at)),
  PRIMARY KEY(calendar_id, recurring_remote_id, range_start_at, range_end_at)
) STRICT, WITHOUT ROWID;

CREATE INDEX local_calendar_instance_coverage_lookup
ON local_calendar_instance_coverage(calendar_id, recurring_remote_id, range_start_at, range_end_at, expires_at);
)";

constexpr char calendarRecurrenceStorageSchemaSql[] = R"(
CREATE TABLE local_calendar_event_recurrences (
  event_id TEXT PRIMARY KEY REFERENCES local_calendar_events(id) ON UPDATE CASCADE ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
  recurrence_rule TEXT NOT NULL CHECK(length(recurrence_rule) BETWEEN 1 AND 524416)
) STRICT, WITHOUT ROWID;

INSERT INTO local_calendar_event_recurrences(event_id, recurrence_rule)
SELECT id, recurrence_rule
FROM local_calendar_events
WHERE recurrence_rule IS NOT NULL;
)";

constexpr char calendarRichEventMetadataSchemaSql[] = R"(
ALTER TABLE local_calendar_events
ADD COLUMN attachments_json TEXT NOT NULL DEFAULT '[]' CHECK(length(attachments_json) <= 65536 AND json_valid(attachments_json) AND json_type(attachments_json) = 'array' AND json_array_length(attachments_json) <= 25);
ALTER TABLE local_calendar_events
ADD COLUMN guest_permissions_json TEXT NOT NULL DEFAULT '{}' CHECK(length(guest_permissions_json) <= 1024 AND json_valid(guest_permissions_json) AND json_type(guest_permissions_json) = 'object');
ALTER TABLE local_calendar_events
ADD COLUMN status_properties_json TEXT NOT NULL DEFAULT '{}' CHECK(length(status_properties_json) <= 16384 AND json_valid(status_properties_json) AND json_type(status_properties_json) = 'object');

DELETE FROM local_sync_checkpoints
WHERE resource_type = 'calendar_event';
)";

constexpr char calendarReminderDefaultsSchemaSql[] = R"(
ALTER TABLE local_calendars
ADD COLUMN default_reminders_json TEXT NOT NULL DEFAULT '[]' CHECK(length(default_reminders_json) <= 8192 AND json_valid(default_reminders_json) AND json_type(default_reminders_json) = 'array' AND json_array_length(default_reminders_json) <= 5);
)";

constexpr char reminderStateSchemaSql[] = R"(
CREATE TABLE local_reminder_state (
  identifier TEXT PRIMARY KEY CHECK(length(trim(identifier)) BETWEEN 1 AND 256),
  event_id TEXT NOT NULL REFERENCES local_calendar_events(id) ON UPDATE CASCADE ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
  trigger_at TEXT NOT NULL CHECK(length(trim(trigger_at)) BETWEEN 1 AND 64),
  snoozed_until TEXT CHECK(snoozed_until IS NULL OR length(trim(snoozed_until)) BETWEEN 1 AND 64),
  dismissed_at TEXT CHECK(dismissed_at IS NULL OR length(trim(dismissed_at)) BETWEEN 1 AND 64),
  delivered_at TEXT CHECK(delivered_at IS NULL OR length(trim(delivered_at)) BETWEEN 1 AND 64),
  updated_at TEXT NOT NULL CHECK(length(trim(updated_at)) BETWEEN 1 AND 64)
) STRICT, WITHOUT ROWID;

CREATE INDEX local_reminder_state_active
ON local_reminder_state(event_id, trigger_at, snoozed_until)
WHERE dismissed_at IS NULL;
)";

constexpr char calendarListColorSchemaSql[] = R"(
ALTER TABLE local_calendars
ADD COLUMN color_id TEXT CHECK(color_id IS NULL OR length(trim(color_id)) BETWEEN 1 AND 32);
)";

[[nodiscard]] QString checksum(const char* sql) {
  return QString::fromLatin1(
      QCryptographicHash::hash(QByteArray(sql), QCryptographicHash::Algorithm::Sha256).toHex());
}

[[nodiscard]] std::optional<AppError>
applySchema(SqliteConnection& connection, const char* sql, const QString& description) {
  sqlite3* const handle = connection.nativeHandle();
  if (handle == nullptr) {
    return AppError(AppErrorCode::Database,
                    description + QStringLiteral(" connection is unavailable"));
  }
  char* errorMessage = nullptr;
  const int result = sqlite3_exec(handle, sql, nullptr, nullptr, &errorMessage);
  sqlite3_free(errorMessage);
  if (result != SQLITE_OK) {
    return AppError(AppErrorCode::Database,
                    description + QStringLiteral(" setup failed (%1)").arg(result));
  }
  return std::nullopt;
}

[[nodiscard]] std::optional<AppError> applySettingsSchema(SqliteConnection& connection) {
  return applySchema(connection, settingsSchemaSql, QStringLiteral("SQLite settings schema"));
}

[[nodiscard]] std::optional<AppError> applyAccountSchema(SqliteConnection& connection) {
  return applySchema(connection, accountSchemaSql, QStringLiteral("SQLite account schema"));
}

[[nodiscard]] std::optional<AppError> applyTaskListSchema(SqliteConnection& connection) {
  return applySchema(connection, taskListSchemaSql, QStringLiteral("SQLite task-list schema"));
}

[[nodiscard]] std::optional<AppError> applyTaskSchema(SqliteConnection& connection) {
  return applySchema(connection, taskSchemaSql, QStringLiteral("SQLite task schema"));
}

[[nodiscard]] std::optional<AppError> applyCalendarSchema(SqliteConnection& connection) {
  return applySchema(connection, calendarSchemaSql, QStringLiteral("SQLite calendar schema"));
}

[[nodiscard]] std::optional<AppError> applyCalendarEventSchema(SqliteConnection& connection) {
  return applySchema(
      connection, calendarEventSchemaSql, QStringLiteral("SQLite calendar-event schema"));
}

[[nodiscard]] std::optional<AppError> applyCalendarEventTypeSchema(SqliteConnection& connection) {
  return applySchema(
      connection, calendarEventTypeSchemaSql, QStringLiteral("SQLite calendar-event type schema"));
}

[[nodiscard]] std::optional<AppError>
applyCalendarEventMetadataSchema(SqliteConnection& connection) {
  return applySchema(connection,
                     calendarEventMetadataSchemaSql,
                     QStringLiteral("SQLite calendar-event metadata schema"));
}

[[nodiscard]] std::optional<AppError> applyNoteProjectionSchema(SqliteConnection& connection) {
  return applySchema(
      connection, noteProjectionSchemaSql, QStringLiteral("SQLite note projection schema"));
}

[[nodiscard]] std::optional<AppError> applyPendingMutationSchema(SqliteConnection& connection) {
  return applySchema(
      connection, pendingMutationSchemaSql, QStringLiteral("SQLite pending-mutation schema"));
}

[[nodiscard]] std::optional<AppError> applyUndoSchema(SqliteConnection& connection) {
  return applySchema(connection, undoSchemaSql, QStringLiteral("SQLite undo schema"));
}

[[nodiscard]] std::optional<AppError> applySyncCheckpointSchema(SqliteConnection& connection) {
  return applySchema(
      connection, syncCheckpointSchemaSql, QStringLiteral("SQLite sync-checkpoint schema"));
}

[[nodiscard]] std::optional<AppError> applySyncConflictSchema(SqliteConnection& connection) {
  return applySchema(
      connection, syncConflictSchemaSql, QStringLiteral("SQLite sync-conflict schema"));
}

[[nodiscard]] std::optional<AppError> applyFtsSchema(SqliteConnection& connection) {
  return applySchema(connection, ftsSchemaSql, QStringLiteral("SQLite FTS schema"));
}

[[nodiscard]] std::optional<AppError> applyNoteFtsSchema(SqliteConnection& connection) {
  return applySchema(connection, noteFtsSchemaSql, QStringLiteral("SQLite note FTS schema"));
}

[[nodiscard]] std::optional<AppError> applyAllUndatedNotesSchema(SqliteConnection& connection) {
  return applySchema(
      connection, allUndatedNotesSchemaSql, QStringLiteral("SQLite all-undated-notes schema"));
}

[[nodiscard]] std::optional<AppError> applyTaskRecurrenceClaimsSchema(SqliteConnection& connection) {
  return applySchema(connection,
                     taskRecurrenceClaimsSchemaSql,
                     QStringLiteral("SQLite task-recurrence claims schema"));
}

[[nodiscard]] std::optional<AppError> applyTaskRecurrenceSafetySchema(SqliteConnection& connection) {
  return applySchema(connection,
                     taskRecurrenceSafetySchemaSql,
                     QStringLiteral("SQLite task-recurrence safety schema"));
}

[[nodiscard]] std::optional<AppError> applyCalendarInstanceCacheSchema(SqliteConnection& connection) {
  return applySchema(connection,
                     calendarInstanceCacheSchemaSql,
                     QStringLiteral("SQLite calendar-instance cache schema"));
}

[[nodiscard]] std::optional<AppError> applyCalendarRecurrenceStorageSchema(SqliteConnection& connection) {
  return applySchema(connection,
                     calendarRecurrenceStorageSchemaSql,
                     QStringLiteral("SQLite calendar-recurrence storage schema"));
}

[[nodiscard]] std::optional<AppError> applyCalendarRichEventMetadataSchema(SqliteConnection& connection) {
  return applySchema(connection,
                     calendarRichEventMetadataSchemaSql,
                     QStringLiteral("SQLite rich calendar-event metadata schema"));
}

[[nodiscard]] std::optional<AppError> applyCalendarReminderDefaultsSchema(SqliteConnection& connection) {
  return applySchema(connection,
                     calendarReminderDefaultsSchemaSql,
                     QStringLiteral("SQLite calendar reminder-default schema"));
}

[[nodiscard]] std::optional<AppError> applyReminderStateSchema(SqliteConnection& connection) {
  return applySchema(connection, reminderStateSchemaSql, QStringLiteral("SQLite reminder-state schema"));
}

[[nodiscard]] std::optional<AppError> applyCalendarListColorSchema(SqliteConnection& connection) {
  return applySchema(connection,
                     calendarListColorSchemaSql,
                     QStringLiteral("SQLite calendar-list color schema"));
}

[[nodiscard]] std::optional<AppError> applyMutationTelemetrySchema(SqliteConnection& connection) {
  return applySchema(connection,
                     mutationTelemetrySchemaSql,
                     QStringLiteral("SQLite mutation telemetry schema"));
}

[[nodiscard]] const std::array<SqliteMigration, 25>& migrations() {
  static const std::array<SqliteMigration, 25> catalogue = {{
      {1,
       QStringLiteral("create local settings"),
       checksum(settingsSchemaSql),
       applySettingsSchema},
      {2, QStringLiteral("create local accounts"), checksum(accountSchemaSql), applyAccountSchema},
      {3,
       QStringLiteral("create local task lists"),
       checksum(taskListSchemaSql),
       applyTaskListSchema},
      {4, QStringLiteral("create local tasks"), checksum(taskSchemaSql), applyTaskSchema},
      {5,
       QStringLiteral("create local calendars"),
       checksum(calendarSchemaSql),
       applyCalendarSchema},
      {6,
       QStringLiteral("create local calendar events"),
       checksum(calendarEventSchemaSql),
       applyCalendarEventSchema},
      {7,
       QStringLiteral("create local note projection"),
       checksum(noteProjectionSchemaSql),
       applyNoteProjectionSchema},
      {8,
       QStringLiteral("create local pending mutations"),
       checksum(pendingMutationSchemaSql),
       applyPendingMutationSchema},
      {9,
       QStringLiteral("create local sync checkpoints"),
       checksum(syncCheckpointSchemaSql),
       applySyncCheckpointSchema},
      {10, QStringLiteral("create local FTS indexes"), checksum(ftsSchemaSql), applyFtsSchema},
      {11, QStringLiteral("create local undo entries"), checksum(undoSchemaSql), applyUndoSchema},
      {12,
       QStringLiteral("create local sync conflicts"),
       checksum(syncConflictSchemaSql),
       applySyncConflictSchema},
      {13,
       QStringLiteral("create local note FTS index"),
       checksum(noteFtsSchemaSql),
       applyNoteFtsSchema},
      {14,
       QStringLiteral("add calendar event type"),
       checksum(calendarEventTypeSchemaSql),
       applyCalendarEventTypeSchema},
      {15,
       QStringLiteral("expand calendar event metadata"),
       checksum(calendarEventMetadataSchemaSql),
       applyCalendarEventMetadataSchema,
       QStringLiteral("d09e8f72bd4585c49c509e0e13bdb130848fdd5dafd6c0b17282851627f92f2a")},
      {16,
       QStringLiteral("project all undated tasks as notes"),
       checksum(allUndatedNotesSchemaSql),
       applyAllUndatedNotesSchema},
      {17,
       QStringLiteral("add task recurrence claims"),
       checksum(taskRecurrenceClaimsSchemaSql),
       applyTaskRecurrenceClaimsSchema},
      {18,
       QStringLiteral("add task recurrence safety metadata"),
       checksum(taskRecurrenceSafetySchemaSql),
       applyTaskRecurrenceSafetySchema},
      {19,
       QStringLiteral("add calendar instance cache"),
       checksum(calendarInstanceCacheSchemaSql),
       applyCalendarInstanceCacheSchema},
      {20,
       QStringLiteral("store full calendar recurrence lines"),
       checksum(calendarRecurrenceStorageSchemaSql),
       applyCalendarRecurrenceStorageSchema},
      {21,
       QStringLiteral("store rich calendar event metadata"),
       checksum(calendarRichEventMetadataSchemaSql),
       applyCalendarRichEventMetadataSchema},
      {22,
       QStringLiteral("store calendar default reminders"),
       checksum(calendarReminderDefaultsSchemaSql),
       applyCalendarReminderDefaultsSchema},
      {23,
       QStringLiteral("store local reminder state"),
       checksum(reminderStateSchemaSql),
       applyReminderStateSchema},
      {24,
       QStringLiteral("store calendar-list color IDs"),
       checksum(calendarListColorSchemaSql),
       applyCalendarListColorSchema},
      {25,
       QStringLiteral("store redacted mutation telemetry"),
       checksum(mutationTelemetrySchemaSql),
       applyMutationTelemetrySchema},
  }};
  return catalogue;
}

} // namespace

SqliteMigrationRunResultOrError LocalSchema::initialize(SqliteConnection& connection) {
  return SqliteMigrationRunner::run(connection, migrations());
}

} // namespace hcb
