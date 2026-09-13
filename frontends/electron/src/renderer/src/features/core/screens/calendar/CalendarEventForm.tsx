import { useEffect, useState } from "react";
import type { ReactNode } from "react";
import {
  type AutoTagRule,
  googleCalendarEventColor,
  googleCalendarEventColors,
  type SettingsSnapshot
} from "@shared/ipc/contracts";
import { Bell, CalendarPlus, Clock3, ExternalLink, FileText, Gift, ListPlus, MapPin, Phone, Plus, RotateCcw, Tag, Trash2, Users, Video, type LucideIcon } from "lucide-react";
import { EmojiInput, EmojiTextarea } from "../../../../components/EmojiTextField";
import { Badge, Input, cx } from "../../../../components/primitives";
import { ErrorState } from "../../../../components/states";
import type { useCoreViewModelSource } from "../../coreViewModelSource";
import { MarkdownPreview } from "../../MarkdownPreview";
import { TagBadges, TagInput } from "../../TagInput";
import { AutoTagAudit } from "../../AutoTagAudit";
import { AttachmentPanel } from "../../AttachmentPanel";
import { EntityLinksPanel } from "../../EntityLinksPanel";
import { plannerLinkTargets } from "../../plannerLinkTargets";
import {
  addUtcDaysIso,
  dateInputToIso,
  dateInputValue,
  startOfUtcDayIso
} from "../../coreScreenShared";
import { CalendarSourceSwatch } from "./CalendarEventChips";
import {
  calendarDateTimeLocalInputToIso,
  calendarDateTimeLocalInputValue
} from "./calendarDateUtils";
import {
  allDayEndInputValue,
  calendarDraftDurationLabel,
  calendarDraftRangeLabel,
  calendarRecurrenceRulePreview,
  calendarRecurrenceSummary
} from "./drafts";
import type { CalendarCreateMode, CalendarEventDraft, CalendarRepeatFrequency, CalendarRepeatWeekday } from "./types";

const repeatWeekdays: Array<{ id: CalendarRepeatWeekday; label: string }> = [
  { id: "SU", label: "S" },
  { id: "MO", label: "M" },
  { id: "TU", label: "T" },
  { id: "WE", label: "W" },
  { id: "TH", label: "T" },
  { id: "FR", label: "F" },
  { id: "SA", label: "S" }
];

type CalendarSource = ReturnType<typeof useCoreViewModelSource>["calendarSources"][number];
type CalendarEventColorOverrides = SettingsSnapshot["calendarEventColorOverrides"];

function repeatWeekdayForIso(value: string): CalendarRepeatWeekday {
  const date = new Date(value);
  return repeatWeekdays[Number.isFinite(date.getTime()) ? date.getUTCDay() : 0]?.id ?? "SU";
}

function DetailLine({
  children,
  icon: Icon,
  label
}: {
  children: ReactNode;
  icon?: LucideIcon;
  label?: string;
}): JSX.Element {
  return (
    <div className="grid grid-cols-[18px_minmax(0,1fr)] gap-3">
      <div className="pt-0.5 text-text-muted">
        {Icon ? <Icon aria-hidden="true" size={16} /> : null}
      </div>
      <div className="min-w-0">
        {label ? (
          <div className="text-[var(--text-xs)] font-semibold uppercase text-text-muted">{label}</div>
        ) : null}
        <div className="min-w-0 text-[var(--text-base)] leading-relaxed text-text-primary">{children}</div>
      </div>
    </div>
  );
}

function formatReminderMinutes(minutes: number): string {
  const safeMinutes = Math.max(0, Math.round(minutes));
  const days = Math.floor(safeMinutes / 1440);
  const hours = Math.floor((safeMinutes % 1440) / 60);
  const mins = safeMinutes % 60;
  const parts: string[] = [];

  if (days > 0) {
    parts.push(`${days} day${days === 1 ? "" : "s"}`);
  }

  if (hours > 0) {
    parts.push(`${hours} hr${hours === 1 ? "" : "s"}`);
  }

  if (mins > 0 || parts.length === 0) {
    parts.push(`${mins} min${mins === 1 ? "" : "s"}`);
  }

  return parts.join(" ");
}

function parseReminderMinutes(value: string): number | null {
  const trimmed = value.trim().toLowerCase();

  if (!trimmed) {
    return null;
  }

  if (/^\d+$/.test(trimmed)) {
    return Number.parseInt(trimmed, 10);
  }

  let total = 0;
  let matched = false;
  const pattern = /(\d+)\s*(d|day|days|h|hr|hrs|hour|hours|m|min|mins|minute|minutes)\b/g;
  let match: RegExpExecArray | null;

  while ((match = pattern.exec(trimmed)) !== null) {
    const amount = Number.parseInt(match[1] ?? "0", 10);
    const unit = match[2] ?? "min";
    matched = true;

    if (unit.startsWith("d")) {
      total += amount * 1440;
    } else if (unit.startsWith("h")) {
      total += amount * 60;
    } else {
      total += amount;
    }
  }

  return matched ? total : null;
}

function ReminderOffsetInput({
  label,
  minutes,
  onChange
}: {
  label: string;
  minutes: number;
  onChange: (minutes: number) => void;
}): JSX.Element {
  const [text, setText] = useState(() => formatReminderMinutes(minutes));

  useEffect(() => {
    setText(formatReminderMinutes(minutes));
  }, [minutes]);

  function commit(value: string): void {
    const parsed = parseReminderMinutes(value);

    if (parsed === null) {
      setText(formatReminderMinutes(minutes));
      return;
    }

    const nextMinutes = Math.min(40320, Math.max(0, parsed));
    onChange(nextMinutes);
    setText(formatReminderMinutes(nextMinutes));
  }

  return (
    <Input
      aria-label={label}
      onBlur={(event) => commit(event.currentTarget.value)}
      onChange={(event) => {
        const nextText = event.currentTarget.value;
        setText(nextText);
        const parsed = parseReminderMinutes(nextText);

        if (parsed !== null) {
          onChange(Math.min(40320, Math.max(0, parsed)));
        }
      }}
      onFocus={(event) => event.currentTarget.select()}
      value={text}
    />
  );
}

function calendarReminderSummary(value: string): string {
  const minutes = Number.parseInt(value.trim(), 10);

  if (!Number.isInteger(minutes) || minutes < 0) {
    return "None";
  }

  if (minutes === 0) {
    return "At start";
  }

  if (minutes < 60) {
    return `${minutes} minute${minutes === 1 ? "" : "s"} before`;
  }

  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;

  return `${hours} hr ${remainingMinutes} min before`;
}

function reminderSummary(method: "popup" | "email", minutes: number): string {
  const prefix = method === "email" ? "Email" : "Popup";
  return `${prefix} ${calendarReminderSummary(String(minutes)).toLocaleLowerCase()}`;
}

function calendarRemindersSummary(draft: CalendarEventDraft): string | null {
  if (draft.remindersUseDefault) {
    return "Calendar default reminders";
  }

  if (draft.reminders.length === 0) {
    return null;
  }

  return draft.reminders.map((reminder) => reminderSummary(reminder.method, reminder.minutes)).join(", ");
}

function attendeeStatusLabel(value: string | undefined): string {
  return value === "accepted"
    ? "accepted"
    : value === "declined"
      ? "declined"
      : value === "tentative"
        ? "tentative"
        : "needs action";
}

function eventDurationVisible(draft: CalendarEventDraft): boolean {
  if (draft.allDay) {
    return Date.parse(draft.endsAt) - Date.parse(draft.startsAt) > 24 * 60 * 60 * 1000;
  }

  return true;
}

function eventCrossesDate(draft: CalendarEventDraft, timeZone: string): boolean {
  return draft.allDay
    ? Date.parse(draft.endsAt) - Date.parse(draft.startsAt) > 24 * 60 * 60 * 1000
    : calendarDateTimeLocalInputValue(draft.startsAt, timeZone).slice(0, 10) !==
      calendarDateTimeLocalInputValue(draft.endsAt, timeZone).slice(0, 10);
}

function calendarDetailRangeLabel(draft: CalendarEventDraft, timeZone: string): string {
  if (!eventCrossesDate(draft, timeZone)) {
    return calendarDraftRangeLabel(draft, timeZone);
  }

  if (draft.allDay) {
    return `${dateInputValue(draft.startsAt)}-${allDayEndInputValue(draft.endsAt)} · All day`;
  }

  return calendarDraftRangeLabel(draft, timeZone);
}

function visibleConferenceLabel(value: string | undefined): string | undefined {
  const label = value?.trim();
  return label ? label.replace(/^https?:\/\//, "") : undefined;
}

function CalendarConferenceDetails({ conference }: { conference: CalendarEventDraft["conference"] }): JSX.Element | null {
  if (!conference) {
    return null;
  }

  const joinLabel = conference.solutionName ? `Join with ${conference.solutionName}` : "Join with Google Meet";
  const videoLabel = visibleConferenceLabel(conference.videoLabel) ?? visibleConferenceLabel(conference.videoUri);
  const phoneLabel = visibleConferenceLabel(conference.phoneLabel) ?? visibleConferenceLabel(conference.phoneUri);
  const moreLabel = visibleConferenceLabel(conference.moreLabel) ?? visibleConferenceLabel(conference.moreUri) ?? "More phone numbers";

  if (!conference.videoUri && !conference.phoneUri && !conference.moreUri) {
    return null;
  }

  return (
    <div className="grid gap-4">
      {conference.videoUri ? (
        <DetailLine icon={Video}>
          <a
            className="inline-flex items-center gap-1 text-accent hover:underline"
            href={conference.videoUri}
            rel="noreferrer"
            target="_blank"
          >
            {joinLabel}
            <ExternalLink aria-hidden="true" size={14} />
          </a>
          {videoLabel ? <div className="text-[var(--text-sm)] text-text-muted">{videoLabel}</div> : null}
        </DetailLine>
      ) : null}
      {conference.phoneUri || phoneLabel ? (
        <DetailLine icon={Phone}>
          {conference.phoneUri ? (
            <a className="text-accent hover:underline" href={conference.phoneUri}>
              Join by phone
            </a>
          ) : (
            <span>Join by phone</span>
          )}
          <div className="text-[var(--text-sm)] text-text-muted">
            {[phoneLabel, conference.phonePin ? `PIN: ${conference.phonePin}` : null]
              .filter(Boolean)
              .join(" ")}
          </div>
        </DetailLine>
      ) : null}
      {conference.moreUri ? (
        <DetailLine icon={ExternalLink}>
          <a className="text-accent hover:underline" href={conference.moreUri} rel="noreferrer" target="_blank">
            {moreLabel}
          </a>
        </DetailLine>
      ) : null}
    </div>
  );
}

function draftDisplayColor(
  draft: CalendarEventDraft,
  selectedCalendar: CalendarSource | undefined,
  eventColorOverrides: CalendarEventColorOverrides
): { background: string | null; foreground: string | null } {
  const googleColor = googleCalendarEventColor(draft.colorId || null);
  const override = googleColor ? eventColorOverrides[googleColor.id] : undefined;

  if (override) {
    return override;
  }

  if (googleColor) {
    return { background: googleColor.background, foreground: googleColor.foreground };
  }

  return {
    background: selectedCalendar?.backgroundColor ?? null,
    foreground: selectedCalendar?.foregroundColor ?? null
  };
}

function EventColorSelect({
  draft,
  eventColorOverrides,
  selectedCalendar,
  setDraft
}: {
  draft: CalendarEventDraft;
  eventColorOverrides: CalendarEventColorOverrides;
  selectedCalendar: CalendarSource | undefined;
  setDraft: (draft: CalendarEventDraft) => void;
}): JSX.Element {
  const displayColor = draftDisplayColor(draft, selectedCalendar, eventColorOverrides);

  return (
    <label className="grid gap-1 text-[var(--text-sm)] text-text-secondary">
      <span>Color</span>
      <div className="flex min-w-0 items-center gap-2">
        <span
          aria-hidden="true"
          className="h-5 w-5 shrink-0 rounded-hcbSm border border-border"
          style={{
            backgroundColor: displayColor.background ?? undefined,
            borderColor: displayColor.background ?? undefined
          }}
        />
        <select
          aria-label="Event color"
          className="h-8 min-w-0 flex-1 rounded-hcbMd border border-border bg-surface-0 px-2 text-[var(--text-base)] text-text-primary focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
          onChange={(event) => setDraft({ ...draft, colorId: event.target.value })}
          value={draft.colorId}
        >
          <option value="">Calendar default</option>
          {googleCalendarEventColors.map((color) => (
            <option key={color.id} value={color.id}>
              {eventColorOverrides[color.id] ? `${color.label} (custom)` : color.label}
            </option>
          ))}
        </select>
      </div>
    </label>
  );
}

function ReminderControls({
  draft,
  setDraft
}: {
  draft: CalendarEventDraft;
  setDraft: (draft: CalendarEventDraft) => void;
}): JSX.Element {
  const mode = draft.remindersUseDefault ? "default" : draft.reminders.length > 0 ? "custom" : "none";

  function setMode(value: string): void {
    if (value === "default") {
      setDraft({ ...draft, remindersUseDefault: true, reminders: [], reminderMinutes: "" });
      return;
    }

    if (value === "custom") {
      setDraft({
        ...draft,
        remindersUseDefault: false,
        reminders: draft.reminders.length > 0 ? draft.reminders : [{ method: "popup", minutes: 10 }],
        reminderMinutes: ""
      });
      return;
    }

    setDraft({ ...draft, remindersUseDefault: false, reminders: [], reminderMinutes: "" });
  }

  function setReminder(index: number, patch: Partial<CalendarEventDraft["reminders"][number]>): void {
    setDraft({
      ...draft,
      reminders: draft.reminders.map((reminder, reminderIndex) =>
        reminderIndex === index ? { ...reminder, ...patch } : reminder
      )
    });
  }

  return (
    <div className="grid gap-2">
      <label className="grid gap-1 text-[var(--text-sm)] text-text-secondary">
        <span className="inline-flex items-center gap-1">
          <Bell aria-hidden="true" size={13} />
          Reminders
        </span>
        <select
          aria-label="Event reminder mode"
          className="h-8 rounded-hcbMd border border-border bg-surface-0 px-2 text-[var(--text-base)] text-text-primary focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
          onChange={(event) => setMode(event.target.value)}
          value={mode}
        >
          <option value="default">Calendar default</option>
          <option value="none">None</option>
          <option value="custom">Custom</option>
        </select>
      </label>
      {mode === "custom" ? (
        <div className="grid gap-2">
          {draft.reminders.map((reminder, index) => (
            <div className="grid grid-cols-[minmax(0,1fr)_minmax(9rem,0.55fr)_32px] gap-2" key={`${reminder.method}-${index}`}>
              <select
                aria-label={`Reminder ${index + 1} method`}
                className="h-8 rounded-hcbMd border border-border bg-surface-0 px-2 text-[var(--text-base)] text-text-primary"
                onChange={(event) => setReminder(index, { method: event.target.value as "popup" | "email" })}
                value={reminder.method}
              >
                <option value="popup">Popup</option>
                <option value="email">Email</option>
              </select>
              <ReminderOffsetInput
                label={`Reminder ${index + 1} offset`}
                minutes={reminder.minutes}
                onChange={(minutes) => setReminder(index, { minutes })}
              />
              <button
                aria-label={`Remove reminder ${index + 1}`}
                className="grid size-8 place-items-center rounded-hcbMd border border-border bg-surface-0 text-text-muted hover:bg-danger/10 hover:text-danger"
                onClick={() => setDraft({ ...draft, reminders: draft.reminders.filter((_, reminderIndex) => reminderIndex !== index) })}
                type="button"
              >
                <Trash2 aria-hidden="true" size={14} />
              </button>
            </div>
          ))}
          <button
            className="inline-flex h-8 items-center justify-center gap-2 rounded-hcbMd border border-border bg-surface-0 px-2 text-[var(--text-sm)] font-medium text-text-secondary hover:bg-surface-1"
            onClick={() => setDraft({ ...draft, reminders: [...draft.reminders, { method: "popup" as const, minutes: 10 }].slice(0, 10) })}
            type="button"
          >
            <Plus aria-hidden="true" size={14} />
            Add reminder
          </button>
        </div>
      ) : null}
    </div>
  );
}

function PrivacyControls({
  draft,
  setDraft
}: {
  draft: CalendarEventDraft;
  setDraft: (draft: CalendarEventDraft) => void;
}): JSX.Element {
  return (
    <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
      <label className="grid gap-1 text-[var(--text-sm)] text-text-secondary">
        <span>Show as</span>
        <select
          aria-label="Event transparency"
          className="h-8 rounded-hcbMd border border-border bg-surface-0 px-2 text-[var(--text-base)] text-text-primary"
          onChange={(event) => setDraft({ ...draft, transparency: event.target.value as CalendarEventDraft["transparency"] })}
          value={draft.transparency ?? "opaque"}
        >
          <option value="opaque">Busy</option>
          <option value="transparent">Free</option>
        </select>
      </label>
      <label className="grid gap-1 text-[var(--text-sm)] text-text-secondary">
        <span>Visibility</span>
        <select
          aria-label="Event visibility"
          className="h-8 rounded-hcbMd border border-border bg-surface-0 px-2 text-[var(--text-base)] text-text-primary"
          onChange={(event) => setDraft({ ...draft, visibility: event.target.value as CalendarEventDraft["visibility"] })}
          value={draft.visibility ?? "default"}
        >
          <option value="default">Default</option>
          <option value="public">Public</option>
          <option value="private">Private</option>
        </select>
      </label>
    </div>
  );
}

function MeetControl({
  draft,
  setDraft
}: {
  draft: CalendarEventDraft;
  setDraft: (draft: CalendarEventDraft) => void;
}): JSX.Element {
  if (draft.conference?.videoUri) {
    return <CalendarConferenceDetails conference={draft.conference} />;
  }

  return (
    <label className="flex min-h-8 items-center gap-2 text-[var(--text-sm)] text-text-secondary">
      <input
        checked={draft.addMeet}
        className="accent-[var(--color-accent)]"
        onChange={(event) => setDraft({ ...draft, addMeet: event.target.checked })}
        type="checkbox"
      />
      <span className="inline-flex items-center gap-1">
        <Video aria-hidden="true" size={13} />
        Add Google Meet
      </span>
    </label>
  );
}

function AttendeeStatusPreview({ draft }: { draft: CalendarEventDraft }): JSX.Element | null {
  if (draft.attendees.length === 0) {
    return null;
  }

  return (
    <div className="flex flex-wrap gap-2">
      {draft.attendees.map((attendee) => (
        <Badge key={attendee.email} tone="neutral">
          {attendee.email} · {attendeeStatusLabel(attendee.responseStatus)}
        </Badge>
      ))}
    </div>
  );
}

export function CalendarEventDetails({
  calendars,
  defaultTimeZone,
  draft,
  eventColorOverrides,
  rules,
  source
}: {
  calendars: ReturnType<typeof useCoreViewModelSource>["calendarSources"];
  defaultTimeZone: string;
  draft: CalendarEventDraft;
  eventColorOverrides: CalendarEventColorOverrides;
  rules: readonly AutoTagRule[];
  source: ReturnType<typeof useCoreViewModelSource>;
}): JSX.Element {
  const selectedCalendar = calendars.find((calendar) => calendar.id === draft.calendarId);
  const displayColor = draftDisplayColor(draft, selectedCalendar, eventColorOverrides);
  const sourceTimeZone = selectedCalendar?.timeZone ?? defaultTimeZone;
  const guests = draft.guests
    .split(",")
    .map((guest) => guest.trim())
    .filter(Boolean);
  const reminderLabel = calendarRemindersSummary(draft) ??
    (draft.reminderMinutes.trim() ? calendarReminderSummary(draft.reminderMinutes) : null);
  const repeats = draft.repeatFrequency !== "none";
  const showSourceTimeZone = sourceTimeZone !== defaultTimeZone;
  const location = draft.location.trim();
  const notes = draft.notes.trim();
  const showReminder = reminderLabel !== null && reminderLabel !== "None";
  const completed = draft.completedAt !== null && draft.completedAt !== undefined;

  return (
    <div className={cx("grid gap-5 py-1", completed && "text-text-muted")}>
      <div className="grid grid-cols-[24px_minmax(0,1fr)] gap-4">
        <CalendarSourceSwatch
          calendarId={draft.calendarId}
          className="mt-2 size-3.5 rounded-hcbSm"
          color={displayColor.background}
        />
        <div className="min-w-0">
          <div className="flex min-w-0 items-start justify-between gap-3">
            <h3 className={cx(
              "min-w-0 break-words text-[var(--text-2xl)] font-semibold leading-tight text-text-primary",
              completed && "text-text-muted line-through"
            )}>
              {draft.title || "Untitled event"}
            </h3>
          </div>
          <div className={cx(
            "mt-2 flex min-w-0 flex-wrap items-center gap-2 text-[var(--text-base)] text-text-secondary",
            completed && "line-through"
          )}>
            <span>{calendarDetailRangeLabel(draft, sourceTimeZone)}</span>
            {eventDurationVisible(draft) ? <Badge tone="neutral">{calendarDraftDurationLabel(draft)}</Badge> : null}
            {selectedCalendar?.title ? <Badge tone="neutral">{selectedCalendar.title}</Badge> : null}
            {showSourceTimeZone ? <Badge tone="neutral">{sourceTimeZone}</Badge> : null}
            <Badge tone="neutral">{draft.transparency === "transparent" ? "Free" : "Busy"}</Badge>
            <Badge tone="neutral">{draft.visibility === "private" ? "Private" : draft.visibility === "public" ? "Public" : "Default visibility"}</Badge>
          </div>
        </div>
      </div>

      {notes ? (
        <DetailLine icon={FileText}>
          <MarkdownPreview
            ariaLabel="Event notes preview"
            body={notes}
            emptyDescription="No notes"
            emptyTitle="No notes"
            plannerLinkTargets={plannerLinkTargets(source)}
            variant="plain"
          />
        </DetailLine>
      ) : null}

      {draft.id ? <AttachmentPanel entityId={draft.id} entityKind="event" /> : null}

      {showReminder ? (
        <DetailLine icon={Bell}>
          {reminderLabel}
        </DetailLine>
      ) : null}

      {draft.tags.length > 0 ? (
        <DetailLine icon={Tag}>
          <TagBadges tags={draft.tags} />
        </DetailLine>
      ) : null}

      <CalendarConferenceDetails conference={draft.conference} />

      {location ? (
        <DetailLine icon={MapPin}>
          {location}
        </DetailLine>
      ) : null}

      {guests.length > 0 ? (
        <DetailLine icon={Users}>
          <div className="flex flex-wrap gap-2">
            {(draft.attendees.length > 0 ? draft.attendees : guests.map((email) => ({ email, responseStatus: undefined }))).map((guest) => (
              <Badge key={guest.email} tone="neutral">
                {guest.email}{guest.responseStatus ? ` · ${attendeeStatusLabel(guest.responseStatus)}` : ""}
              </Badge>
            ))}
          </div>
        </DetailLine>
      ) : null}

      {repeats ? (
        <DetailLine icon={RotateCcw}>
          {calendarRecurrenceSummary(draft)}
        </DetailLine>
      ) : null}

      <AutoTagAudit
        input={{
          kind: "event",
          title: draft.title,
          body: draft.notes,
          existingTags: draft.tags,
          existingEventColorId: draft.colorId || undefined,
          hcbKind: draft.hcbKind
        }}
        rules={rules}
      />

      {draft.id ? <EntityLinksPanel entityId={draft.id} entityKind="event" /> : null}
    </div>
  );
}

function CalendarCreateModeTabs({
  mode,
  onChange
}: {
  mode: CalendarCreateMode;
  onChange: (mode: CalendarCreateMode) => void;
}): JSX.Element {
  return (
    <div className="grid grid-cols-3 gap-1 rounded-hcbMd bg-surface-0 p-1" role="tablist" aria-label="Create item type">
      {([
        { id: "event" as const, label: "Event", icon: CalendarPlus },
        { id: "task" as const, label: "Task", icon: ListPlus },
        { id: "birthday" as const, label: "Birthday", icon: Gift }
      ]).map((item) => {
        const Icon = item.icon;
        const active = item.id === mode;

        return (
          <button
            aria-selected={active}
            className={cx(
              "inline-flex min-h-8 items-center justify-center gap-2 rounded-hcbSm px-2 text-[var(--text-sm)] font-medium transition-colors duration-fast ease-hcb focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent",
              active ? "bg-accent text-bg-tertiary" : "text-text-secondary hover:bg-bg-tertiary hover:text-text-primary"
            )}
            key={item.id}
            onClick={() => onChange(item.id)}
            role="tab"
            type="button"
          >
            <Icon aria-hidden="true" size={14} />
            {item.label}
          </button>
        );
      })}
    </div>
  );
}

export function CalendarEventForm({
  calendars,
  createMode,
  defaultTimeZone,
  draft,
  error,
  eventColorOverrides,
  onCreateModeChange,
  rules,
  setDraft,
  setTaskListId,
  taskListId,
  taskLists
}: {
  calendars: ReturnType<typeof useCoreViewModelSource>["calendarSources"];
  createMode: CalendarCreateMode;
  defaultTimeZone: string;
  draft: CalendarEventDraft;
  error?: string;
  eventColorOverrides: CalendarEventColorOverrides;
  onCreateModeChange: (mode: CalendarCreateMode) => void;
  rules: readonly AutoTagRule[];
  setDraft: (draft: CalendarEventDraft) => void;
  setTaskListId: (listId: string) => void;
  taskListId: string;
  taskLists: ReturnType<typeof useCoreViewModelSource>["taskLists"];
}): JSX.Element {
  const selectedCalendar = calendars.find((calendar) => calendar.id === draft.calendarId);
  const displayColor = draftDisplayColor(draft, selectedCalendar, eventColorOverrides);
  const sourceTimeZone = selectedCalendar?.timeZone ?? defaultTimeZone;
  const showSourceTimeZone = sourceTimeZone !== defaultTimeZone;
  const isBirthdayDraft = draft.hcbKind === "birthday" || (draft.mode === "create" && createMode === "birthday");

  function setAllDay(allDay: boolean): void {
    if (allDay) {
      const startsAt = startOfUtcDayIso(draft.startsAt);
      setDraft({
        ...draft,
        allDay,
        startsAt,
        endsAt: addUtcDaysIso(startsAt, 1)
      });
      return;
    }

    const startsAt = `${dateInputValue(draft.startsAt)}T09:00:00.000Z`;
    setDraft({
      ...draft,
      allDay,
      startsAt,
      endsAt: new Date(Date.parse(startsAt) + 60 * 60 * 1000).toISOString()
    });
  }

  function setAllDayStart(value: string): void {
    const startsAt = dateInputToIso(value);
    const currentEnd = Date.parse(draft.endsAt);
    const minimumEnd = Date.parse(addUtcDaysIso(startsAt, 1));
    setDraft({
      ...draft,
      startsAt,
      endsAt: currentEnd <= Date.parse(startsAt) ? new Date(minimumEnd).toISOString() : draft.endsAt,
      repeatWeekdays: [repeatWeekdayForIso(startsAt)]
    });
  }

  function setAllDayEnd(value: string): void {
    setDraft({
      ...draft,
      endsAt: addUtcDaysIso(dateInputToIso(value), 1)
    });
  }

  function setCreateDate(value: string): void {
    const startsAt = dateInputToIso(value);
    setDraft({
      ...draft,
      allDay: true,
      startsAt,
      endsAt: addUtcDaysIso(startsAt, 1),
      repeatFrequency: isBirthdayDraft ? "yearly" : draft.repeatFrequency,
      repeatWeekdays: [repeatWeekdayForIso(startsAt)]
    });
  }

  function setRepeatFrequency(value: CalendarRepeatFrequency): void {
    if (value === "none") {
      setDraft({
        ...draft,
        repeatFrequency: "none",
        repeatEndMode: "never",
        repeatEndsOn: "",
        repeatCount: ""
      });
      return;
    }

    if (value === "custom") {
      setDraft({
        ...draft,
        repeatFrequency: "custom",
        repeatCustomFrequency: draft.repeatFrequency !== "none" && draft.repeatFrequency !== "custom"
          ? draft.repeatFrequency
          : draft.repeatCustomFrequency,
        repeatWeekdays: draft.repeatWeekdays.length > 0 ? draft.repeatWeekdays : [repeatWeekdayForIso(draft.startsAt)]
      });
      return;
    }

    setDraft({
      ...draft,
      repeatFrequency: value,
      repeatCustomFrequency: value,
      repeatEndMode: "never",
      repeatInterval: "1",
      repeatEndsOn: "",
      repeatCount: "",
      repeatWeekdays: value === "weekly" ? [repeatWeekdayForIso(draft.startsAt)] : draft.repeatWeekdays
    });
  }

  function toggleRepeatWeekday(day: CalendarRepeatWeekday): void {
    const current = new Set(draft.repeatWeekdays);

    if (current.has(day)) {
      if (current.size === 1) {
        return;
      }

      current.delete(day);
    } else {
      current.add(day);
    }

    setDraft({
      ...draft,
      repeatWeekdays: repeatWeekdays.map((weekday) => weekday.id).filter((weekday) => current.has(weekday))
    });
  }

  if (draft.mode === "create" && createMode === "task") {
    return (
      <div className="grid gap-3">
        <CalendarCreateModeTabs mode={createMode} onChange={onCreateModeChange} />
        {error ? <ErrorState description={error} title="Task not saved" /> : null}
        <EmojiInput
          aria-label="Task title"
          autoFocus
          onValueChange={(title) => setDraft({ ...draft, title })}
          placeholder="New task"
          value={draft.title}
        />
        <EmojiTextarea
          aria-label="Task notes"
          className="min-h-32 w-full resize-none rounded-hcbMd border border-border bg-surface-0 px-3 py-2 text-[var(--text-base)] text-text-primary placeholder:text-text-muted transition-colors duration-fast ease-hcb focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
          onValueChange={(notes) => setDraft({ ...draft, notes })}
          placeholder="Notes"
          value={draft.notes}
        />
        <TagInput onChange={(tags) => setDraft({ ...draft, tags })} value={draft.tags} />
        <fieldset className="grid gap-2 rounded-hcbMd border border-border bg-bg-tertiary p-3">
          <legend className="px-1 text-[var(--text-sm)] font-medium text-text-secondary">Date & list</legend>
          <label className="grid gap-1 text-[var(--text-sm)] text-text-secondary">
            <span>Date</span>
            <Input
              aria-label="Task date"
              onChange={(event) => setCreateDate(event.target.value)}
              type="date"
              value={dateInputValue(draft.startsAt)}
            />
          </label>
          <label className="grid gap-1 text-[var(--text-sm)] text-text-secondary">
            <span>List</span>
            <select
              aria-label="Task list"
              className="h-8 rounded-hcbMd border border-border bg-surface-0 px-2 text-[var(--text-base)] text-text-primary focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
              onChange={(event) => setTaskListId(event.target.value)}
              value={taskListId}
            >
              {taskLists.map((taskList) => (
                <option key={taskList.id} value={taskList.id}>
                  {taskList.title}
                </option>
              ))}
            </select>
          </label>
        </fieldset>
      </div>
    );
  }

  if (isBirthdayDraft) {
    return (
      <div className="grid gap-3">
        {draft.mode === "create" ? (
          <CalendarCreateModeTabs mode={createMode} onChange={onCreateModeChange} />
        ) : null}
        {error ? <ErrorState description={error} title="Birthday not saved" /> : null}
        <EmojiInput
          aria-label="Birthday title"
          autoFocus
          onValueChange={(title) => setDraft({ ...draft, title })}
          placeholder="Whose birthday?"
          value={draft.title}
        />
        <fieldset className="grid gap-2 rounded-hcbMd border border-border bg-bg-tertiary p-3">
          <legend className="px-1 text-[var(--text-sm)] font-medium text-text-secondary">Birthday</legend>
          <label className="grid gap-1 text-[var(--text-sm)] text-text-secondary">
            <span>Date</span>
            <Input
              aria-label="Birthday date"
              onChange={(event) => setCreateDate(event.target.value)}
              type="date"
              value={dateInputValue(draft.startsAt)}
            />
          </label>
          <label className="grid gap-1 text-[var(--text-sm)] text-text-secondary">
            <span>Calendar</span>
            <select
              aria-label="Birthday calendar"
              className="h-8 rounded-hcbMd border border-border bg-surface-0 px-2 text-[var(--text-base)] text-text-primary focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
              onChange={(event) => setDraft({ ...draft, calendarId: event.target.value })}
              value={draft.calendarId}
            >
              {calendars.map((calendar) => (
                <option key={calendar.id} value={calendar.id}>
                  {calendar.title}
                </option>
              ))}
            </select>
          </label>
          <EventColorSelect
            draft={draft}
            eventColorOverrides={eventColorOverrides}
            selectedCalendar={selectedCalendar}
            setDraft={setDraft}
          />
          <label className="grid gap-1 text-[var(--text-sm)] text-text-secondary">
            <span className="inline-flex items-center gap-1">
              <Bell aria-hidden="true" size={13} />
              Reminder
            </span>
            <select
              aria-label="Birthday reminder"
              className="h-8 rounded-hcbMd border border-border bg-surface-0 px-2 text-[var(--text-base)] text-text-primary focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
              onChange={(event) => setDraft({ ...draft, reminderMinutes: event.target.value })}
              value={draft.reminderMinutes}
            >
              <option value="">None</option>
              <option value="0">At start</option>
              <option value="5">5 minutes before</option>
              <option value="10">10 minutes before</option>
              <option value="15">15 minutes before</option>
              <option value="30">30 minutes before</option>
              <option value="60">1 hour before</option>
              <option value="1440">1 day before</option>
            </select>
          </label>
          <p className="text-[var(--text-xs)] text-text-muted">
            Repeats yearly as an all-day calendar event.
          </p>
        </fieldset>
      </div>
    );
  }

  return (
    <div className="grid gap-3">
      {draft.mode === "create" ? (
        <CalendarCreateModeTabs mode={createMode} onChange={onCreateModeChange} />
      ) : null}
      {error ? <ErrorState description={error} title="Event not saved" /> : null}
      <div
        aria-label="Event context"
        className="grid gap-2 rounded-hcbMd border border-border bg-bg-tertiary p-3"
        role="group"
      >
        <div className="flex min-w-0 items-center gap-2">
          <CalendarSourceSwatch calendarId={draft.calendarId} color={displayColor.background} />
          <span className="min-w-0 flex-1 truncate text-[var(--text-sm)] font-semibold text-text-primary">
            {selectedCalendar?.title ?? "Calendar"}
          </span>
        </div>
        <div className="flex min-w-0 flex-wrap items-center gap-2 text-[var(--text-xs)] text-text-muted">
          <span className="inline-flex min-w-0 items-center gap-1">
            <Clock3 aria-hidden="true" size={13} />
            <span className="truncate">{calendarDraftRangeLabel(draft, sourceTimeZone)}</span>
          </span>
          <Badge tone="neutral">{calendarDraftDurationLabel(draft)}</Badge>
          {showSourceTimeZone ? <Badge tone="neutral">{sourceTimeZone}</Badge> : null}
        </div>
      </div>
      <EmojiInput
        aria-label="Event title"
        onValueChange={(title) => setDraft({ ...draft, title })}
        placeholder="Title"
        value={draft.title}
      />
      <fieldset className="grid gap-2 rounded-hcbMd border border-border bg-bg-tertiary p-3">
        <legend className="px-1 text-[var(--text-sm)] font-medium text-text-secondary">Calendar</legend>
        <label className="grid gap-1 text-[var(--text-sm)] text-text-secondary">
          <span>Source</span>
          <select
            aria-label="Event calendar"
            className="h-8 rounded-hcbMd border border-border bg-surface-0 px-2 text-[var(--text-base)] text-text-primary focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
            onChange={(event) => setDraft({ ...draft, calendarId: event.target.value })}
            value={draft.calendarId}
          >
            {calendars.map((calendar) => (
              <option key={calendar.id} value={calendar.id}>
                {calendar.title}
              </option>
            ))}
          </select>
        </label>
        <EventColorSelect
          draft={draft}
          eventColorOverrides={eventColorOverrides}
          selectedCalendar={selectedCalendar}
          setDraft={setDraft}
        />
        <PrivacyControls draft={draft} setDraft={setDraft} />
      </fieldset>
      <fieldset className="grid gap-2 rounded-hcbMd border border-border bg-bg-tertiary p-3">
        <legend className="px-1 text-[var(--text-sm)] font-medium text-text-secondary">Time</legend>
        <label className="flex min-h-8 items-center gap-2 text-[var(--text-sm)] text-text-secondary">
          <input
            checked={draft.allDay}
            className="accent-[var(--color-accent)]"
            onChange={(event) => setAllDay(event.target.checked)}
            type="checkbox"
          />
          All day
        </label>
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
          <Input
            aria-label="Event starts"
            onChange={(event) =>
              draft.allDay
                ? setAllDayStart(event.target.value)
                : setDraft({ ...draft, startsAt: calendarDateTimeLocalInputToIso(event.target.value, sourceTimeZone) })
            }
            type={draft.allDay ? "date" : "datetime-local"}
            value={draft.allDay ? dateInputValue(draft.startsAt) : calendarDateTimeLocalInputValue(draft.startsAt, sourceTimeZone)}
          />
          <Input
            aria-label="Event ends"
            min={draft.allDay ? dateInputValue(draft.startsAt) : undefined}
            onChange={(event) =>
              draft.allDay
                ? setAllDayEnd(event.target.value)
                : setDraft({ ...draft, endsAt: calendarDateTimeLocalInputToIso(event.target.value, sourceTimeZone) })
            }
            type={draft.allDay ? "date" : "datetime-local"}
            value={draft.allDay ? allDayEndInputValue(draft.endsAt) : calendarDateTimeLocalInputValue(draft.endsAt, sourceTimeZone)}
          />
        </div>
      </fieldset>
      <fieldset className="grid gap-2 rounded-hcbMd border border-border bg-bg-tertiary p-3">
        <legend className="px-1 text-[var(--text-sm)] font-medium text-text-secondary">Details</legend>
        <label className="grid gap-1 text-[var(--text-sm)] text-text-secondary">
          <span className="inline-flex items-center gap-1">
            <MapPin aria-hidden="true" size={13} />
            Location
          </span>
          <Input
            aria-label="Event location"
            onChange={(event) => setDraft({ ...draft, location: event.target.value })}
            placeholder="Location"
            value={draft.location}
          />
        </label>
        <label className="grid gap-1 text-[var(--text-sm)] text-text-secondary">
          <span className="inline-flex items-center gap-1">
            <Users aria-hidden="true" size={13} />
            Guests
          </span>
          <Input
            aria-label="Event guests"
            onChange={(event) => setDraft({ ...draft, guests: event.target.value })}
            placeholder="guest@example.com, team@example.com"
            value={draft.guests}
          />
        </label>
        <AttendeeStatusPreview draft={draft} />
        <ReminderControls draft={draft} setDraft={setDraft} />
        <MeetControl draft={draft} setDraft={setDraft} />
        <TagInput onChange={(tags) => setDraft({ ...draft, tags })} value={draft.tags} />
        <AutoTagAudit
          input={{
            kind: "event",
            title: draft.title,
            body: draft.notes,
            existingTags: draft.tags,
            existingEventColorId: draft.colorId || undefined,
            requestedEventColorId: draft.colorId || undefined,
            hcbKind: draft.hcbKind
          }}
          rules={rules}
        />
      </fieldset>
      {draft.id ? <AttachmentPanel editable entityId={draft.id} entityKind="event" /> : null}
      <fieldset className="grid gap-2 rounded-hcbMd border border-border bg-bg-tertiary p-3">
        <legend className="px-1 text-[var(--text-sm)] font-medium text-text-secondary">
          <span className="inline-flex items-center gap-1">
            <RotateCcw aria-hidden="true" size={13} />
            Repeat
          </span>
        </legend>
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
          <label className="grid gap-1 text-[var(--text-sm)] text-text-secondary">
            <span>Frequency</span>
            <select
              aria-label="Event repeat frequency"
              className="h-8 rounded-hcbMd border border-border bg-surface-0 px-2 text-[var(--text-base)] text-text-primary focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
              onChange={(event) =>
                setRepeatFrequency(event.target.value as CalendarRepeatFrequency)
              }
              value={draft.repeatFrequency}
            >
              <option value="none">Does not repeat</option>
              <option value="daily">Daily</option>
              <option value="weekly">Weekly</option>
              <option value="monthly">Monthly</option>
              <option value="yearly">Yearly</option>
              <option value="custom">Custom</option>
            </select>
          </label>
        </div>
        {draft.repeatFrequency === "custom" ? (
          <div className="grid gap-3 rounded-hcbMd border border-border bg-surface-0 p-3">
            <div className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)] gap-2 sm:grid-cols-[120px_minmax(0,1fr)_minmax(0,1fr)] sm:items-end">
              <span className="hidden pb-2 text-[var(--text-sm)] text-text-secondary sm:block">Repeat every</span>
              <Input
                aria-label="Repeat interval"
                min={1}
                max={366}
                onChange={(event) => setDraft({ ...draft, repeatInterval: event.target.value })}
                type="number"
                value={draft.repeatInterval}
              />
              <select
                aria-label="Repeat unit"
                className="h-8 rounded-hcbMd border border-border bg-bg-tertiary px-2 text-[var(--text-base)] text-text-primary focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
                onChange={(event) =>
                  setDraft({
                    ...draft,
                    repeatCustomFrequency: event.target.value as CalendarEventDraft["repeatCustomFrequency"]
                  })
                }
                value={draft.repeatCustomFrequency}
              >
                <option value="daily">day</option>
                <option value="weekly">week</option>
                <option value="monthly">month</option>
                <option value="yearly">year</option>
              </select>
            </div>
            {draft.repeatCustomFrequency === "weekly" ? (
              <div className="grid gap-2">
                <span className="text-[var(--text-sm)] text-text-secondary">Repeat on</span>
                <div className="flex flex-wrap gap-2" role="group" aria-label="Repeat weekdays">
                  {repeatWeekdays.map((weekday) => {
                    const selected = draft.repeatWeekdays.includes(weekday.id);

                    return (
                      <button
                        aria-pressed={selected}
                        className={cx(
                          "flex size-8 items-center justify-center rounded-full text-[var(--text-sm)] font-semibold transition-colors duration-fast ease-hcb focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent",
                          selected ? "bg-accent text-bg-tertiary" : "bg-bg-tertiary text-text-secondary hover:bg-surface-1"
                        )}
                        key={weekday.id}
                        onClick={() => toggleRepeatWeekday(weekday.id)}
                        type="button"
                      >
                        {weekday.label}
                      </button>
                    );
                  })}
                </div>
              </div>
            ) : null}
            {draft.repeatCustomFrequency === "monthly" ? (
              <div className="grid gap-2">
                <label className="grid gap-1 text-[var(--text-sm)] text-text-secondary">
                  <span>Monthly rule</span>
                  <select
                    aria-label="Monthly repeat rule"
                    className="h-8 rounded-hcbMd border border-border bg-bg-tertiary px-2 text-[var(--text-base)] text-text-primary"
                    onChange={(event) => setDraft({ ...draft, repeatMonthlyMode: event.target.value as CalendarEventDraft["repeatMonthlyMode"] })}
                    value={draft.repeatMonthlyMode}
                  >
                    <option value="dayOfMonth">Day of month</option>
                    <option value="weekday">Weekday position</option>
                  </select>
                </label>
                {draft.repeatMonthlyMode === "dayOfMonth" ? (
                  <Input
                    aria-label="Repeat month day"
                    min={1}
                    max={31}
                    onChange={(event) => setDraft({ ...draft, repeatMonthDay: event.target.value })}
                    type="number"
                    value={draft.repeatMonthDay}
                  />
                ) : (
                  <div className="grid grid-cols-2 gap-2">
                    <select
                      aria-label="Repeat weekday position"
                      className="h-8 rounded-hcbMd border border-border bg-bg-tertiary px-2 text-[var(--text-base)] text-text-primary"
                      onChange={(event) => setDraft({ ...draft, repeatSetPos: event.target.value })}
                      value={draft.repeatSetPos}
                    >
                      <option value="1">First</option>
                      <option value="2">Second</option>
                      <option value="3">Third</option>
                      <option value="4">Fourth</option>
                      <option value="-1">Last</option>
                    </select>
                    <select
                      aria-label="Repeat weekday"
                      className="h-8 rounded-hcbMd border border-border bg-bg-tertiary px-2 text-[var(--text-base)] text-text-primary"
                      onChange={(event) => setDraft({ ...draft, repeatWeekdays: [event.target.value as CalendarRepeatWeekday] })}
                      value={draft.repeatWeekdays[0] ?? repeatWeekdayForIso(draft.startsAt)}
                    >
                      {repeatWeekdays.map((weekday) => (
                        <option key={weekday.id} value={weekday.id}>
                          {weekday.id}
                        </option>
                      ))}
                    </select>
                  </div>
                )}
              </div>
            ) : null}
            <fieldset className="grid gap-2">
              <legend className="text-[var(--text-sm)] text-text-secondary">Ends</legend>
              <label className="grid min-h-8 grid-cols-[24px_72px_minmax(0,1fr)] items-center gap-2 text-[var(--text-sm)] text-text-secondary">
                <input
                  aria-label="Repeat never"
                  checked={draft.repeatEndMode === "never"}
                  className="accent-[var(--color-accent)]"
                  onChange={() => setDraft({ ...draft, repeatEndMode: "never", repeatEndsOn: "", repeatCount: "" })}
                  type="radio"
                />
                <span>Never</span>
              </label>
              <label className="grid min-h-8 grid-cols-[24px_72px_minmax(0,1fr)] items-center gap-2 text-[var(--text-sm)] text-text-secondary">
                <input
                  aria-label="Repeat on date"
                  checked={draft.repeatEndMode === "on"}
                  className="accent-[var(--color-accent)]"
                  onChange={() => setDraft({ ...draft, repeatEndMode: "on", repeatCount: "" })}
                  type="radio"
                />
                <span>On</span>
                <Input
                  aria-label="Repeat end date"
                  disabled={draft.repeatEndMode !== "on"}
                  onChange={(event) => setDraft({ ...draft, repeatEndsOn: event.target.value })}
                  type="date"
                  value={draft.repeatEndsOn}
                />
              </label>
              <label className="grid min-h-8 grid-cols-[24px_72px_minmax(0,1fr)] items-center gap-2 text-[var(--text-sm)] text-text-secondary">
                <input
                  aria-label="Repeat after count"
                  checked={draft.repeatEndMode === "after"}
                  className="accent-[var(--color-accent)]"
                  onChange={() => setDraft({ ...draft, repeatEndMode: "after", repeatEndsOn: "" })}
                  type="radio"
                />
                <span>After</span>
                <Input
                  aria-label="Repeat count"
                  disabled={draft.repeatEndMode !== "after"}
                  min={1}
                  max={366}
                  onChange={(event) => setDraft({ ...draft, repeatCount: event.target.value })}
                  placeholder="Occurrences"
                  type="number"
                  value={draft.repeatCount}
                />
              </label>
            </fieldset>
            {calendarRecurrenceRulePreview(draft) ? (
              <code className="overflow-auto rounded-hcbMd border border-border bg-bg-tertiary px-2 py-1 text-[var(--text-xs)] text-text-muted">
                {calendarRecurrenceRulePreview(draft)}
              </code>
            ) : null}
          </div>
        ) : null}
        <div className="text-[var(--text-xs)] text-text-muted">{calendarRecurrenceSummary(draft)}</div>
      </fieldset>
      <label className="grid gap-1 text-[var(--text-sm)] text-text-secondary">
        <span className="inline-flex items-center gap-1">
          <FileText aria-hidden="true" size={13} />
          Notes
        </span>
        <EmojiTextarea
          aria-label="Event notes"
          className="min-h-24 w-full resize-none rounded-hcbMd border border-border bg-surface-0 px-3 py-2 text-[var(--text-base)] text-text-primary placeholder:text-text-muted transition-colors duration-fast ease-hcb focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
          onValueChange={(notes) => setDraft({ ...draft, notes })}
          placeholder="Notes"
          value={draft.notes}
        />
      </label>
    </div>
  );
}
