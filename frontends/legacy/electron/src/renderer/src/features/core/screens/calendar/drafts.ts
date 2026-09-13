import type {
  CalendarEventCreateRequest,
  CalendarEventRecurrence,
  CalendarEventReminder
} from "@shared/ipc/contracts";
import type { useCoreViewModelSource } from "../../coreViewModelSource";
import type { CalendarEventViewModel } from "../../coreViewModels";
import {
  addUtcDaysIso,
  dateInputValue,
  normalizeGuestEmails,
  normalizeReminderMinutes,
  startOfUtcDayIso
} from "../../coreScreenShared";
import { calendarDateTimeLocalInputValue } from "./calendarDateUtils";
import type { CalendarCreateSeed, CalendarEventDraft, CalendarRepeatWeekday } from "./types";

const recurrenceWeekdays: CalendarRepeatWeekday[] = ["SU", "MO", "TU", "WE", "TH", "FR", "SA"];

function recurrenceWeekdayForIso(value: string): CalendarRepeatWeekday {
  const date = new Date(value);

  return recurrenceWeekdays[Number.isFinite(date.getTime()) ? date.getUTCDay() : 0] ?? "SU";
}

export function defaultCalendarId(source: ReturnType<typeof useCoreViewModelSource>): string {
  return (
    source.calendarSources.find((calendar) => calendar.selected)?.id ??
    source.calendarSources[0]?.id ??
    ""
  );
}

function defaultTimedStart(seed?: string): string {
  const base = seed ? new Date(seed) : new Date();

  if (!Number.isFinite(base.getTime())) {
    return new Date().toISOString();
  }

  if (seed) {
    base.setUTCSeconds(0, 0);
  } else {
    base.setUTCMinutes(0, 0, 0);
  }

  return base.toISOString();
}

export function newCalendarDraft(
  source: ReturnType<typeof useCoreViewModelSource>,
  seed?: CalendarCreateSeed
): CalendarEventDraft {
  const allDay = seed?.allDay ?? false;
  const startsAt = allDay ? startOfUtcDayIso(seed?.startsAt ?? new Date().toISOString()) : defaultTimedStart(seed?.startsAt);
  const endsAt = allDay ? addUtcDaysIso(startsAt, 1) : addUtcDaysIso(startsAt, 0);
  const seedEnd = seed?.endsAt && Date.parse(seed.endsAt) > Date.parse(startsAt) ? seed.endsAt : null;
  const allDayEnd = allDay && seedEnd ? startOfUtcDayIso(seedEnd) : null;
  const timedEnd = allDay ? endsAt : seedEnd ?? new Date(Date.parse(startsAt) + 60 * 60 * 1000).toISOString();

  return {
    mode: "create",
    eventId: undefined,
    hcbKind: undefined,
    mutationState: undefined,
    completedAt: null,
    title: "",
    calendarId: defaultCalendarId(source),
    colorId: "",
    startsAt,
    endsAt: allDay ? allDayEnd ?? endsAt : timedEnd,
    timeZone: undefined,
    allDay,
    location: "",
    notes: "",
    tags: [],
    guests: "",
    reminderMinutes: "",
    reminders: [],
    remindersUseDefault: false,
    attendees: [],
    addMeet: false,
    transparency: "opaque",
    visibility: "default",
    conference: null,
    recurringEventId: null,
    originalStartAt: null,
    repeatFrequency: "none",
    repeatCustomFrequency: "weekly",
    repeatEndMode: "never",
    repeatInterval: "1",
    repeatEndsOn: "",
    repeatCount: "",
    repeatWeekdays: [recurrenceWeekdayForIso(startsAt)],
    repeatMonthlyMode: "dayOfMonth",
    repeatMonthDay: String(new Date(startsAt).getUTCDate()),
    repeatSetPos: "1"
  };
}

export function editCalendarDraft(event: CalendarEventViewModel): CalendarEventDraft {
  const recurrence = calendarDraftRecurrenceFromRule(event.recurrenceRule);

  return {
    mode: "edit",
    id: event.id,
    eventId: event.eventId,
    hcbKind: event.hcbKind,
    mutationState: event.mutationState,
    completedAt: event.completedAt ?? null,
    title: event.title,
    calendarId: event.calendarId,
    colorId: event.colorId ?? "",
    startsAt: event.startsAt,
    endsAt: event.endsAt,
    timeZone: event.timeZone || undefined,
    allDay: event.allDay,
    location: event.location === "Scheduled" || event.location === "All day" ? "" : event.location,
    notes: event.notes === "No notes" ? "" : event.notes,
    tags: event.tags ?? [],
    guests: event.guestEmails.join(", "),
    reminderMinutes: event.reminderMinutes[0] === undefined ? "" : String(event.reminderMinutes[0]),
    reminders: event.reminders.length > 0
      ? event.reminders
      : event.reminderMinutes.map((minutes) => ({ method: "popup", minutes })),
    remindersUseDefault: event.remindersUseDefault,
    attendees: event.attendees,
    addMeet: false,
    transparency: event.transparency ?? "opaque",
    visibility: event.visibility ?? "default",
    conference: event.conference,
    recurringEventId: event.recurringEventId ?? null,
    originalStartAt: event.originalStartAt ?? null,
    repeatFrequency: recurrence?.repeatFrequency ?? "none",
    repeatCustomFrequency: recurrence?.repeatCustomFrequency ?? "weekly",
    repeatEndMode: recurrence?.repeatEndMode ?? "never",
    repeatInterval: recurrence ? String(recurrence.interval) : "1",
    repeatEndsOn: recurrence?.endsOn ?? "",
    repeatCount: recurrence?.count === null || recurrence?.count === undefined ? "" : String(recurrence.count),
    repeatWeekdays: recurrence?.byDay?.length ? recurrence.byDay : [recurrenceWeekdayForIso(event.startsAt)],
    repeatMonthlyMode: recurrence?.bySetPos ? "weekday" : "dayOfMonth",
    repeatMonthDay: recurrence?.byMonthDay ? String(recurrence.byMonthDay) : String(new Date(event.startsAt).getUTCDate()),
    repeatSetPos: recurrence?.bySetPos ? String(recurrence.bySetPos) : "1"
  };
}

export function calendarEventPayload(draft: CalendarEventDraft): CalendarEventCreateRequest {
  const reminders = normalizeDraftReminders(draft);
  const reminderMinutes = normalizeReminderMinutes(reminders.map((reminder) => reminder.minutes));

  return {
    title: draft.title.trim(),
    calendarId: draft.calendarId,
    colorId: draft.colorId.trim() || null,
    startsAt: draft.startsAt,
    endsAt: draft.endsAt,
    timeZone: draft.timeZone,
    allDay: draft.allDay,
    location: draft.location,
    notes: draft.notes,
    tags: draft.tags,
    guestEmails: normalizeGuestEmails(draft.guests.split(",")),
    reminderMinutes,
    reminders,
    remindersUseDefault: draft.remindersUseDefault,
    conferenceCreateRequest: draft.addMeet ? { type: "hangoutsMeet" } : null,
    transparency: draft.transparency,
    visibility: draft.visibility,
    recurrence: calendarDraftRecurrence(draft),
    hcbKind: draft.hcbKind
  };
}

export function calendarEventDraftsEqual(
  left: CalendarEventDraft | null,
  right: CalendarEventDraft | null
): boolean {
  if (left === right) {
    return true;
  }

  if (!left || !right) {
    return false;
  }

  return (
    left.mode === right.mode &&
    left.id === right.id &&
    left.eventId === right.eventId &&
    left.hcbKind === right.hcbKind &&
    left.mutationState === right.mutationState &&
    left.title === right.title &&
    left.calendarId === right.calendarId &&
    left.colorId === right.colorId &&
    left.startsAt === right.startsAt &&
    left.endsAt === right.endsAt &&
    left.timeZone === right.timeZone &&
    left.allDay === right.allDay &&
    left.location === right.location &&
    left.notes === right.notes &&
    left.tags.join("\u001f") === right.tags.join("\u001f") &&
    left.guests === right.guests &&
    left.reminderMinutes === right.reminderMinutes &&
    JSON.stringify(left.reminders) === JSON.stringify(right.reminders) &&
    left.remindersUseDefault === right.remindersUseDefault &&
    JSON.stringify(left.attendees) === JSON.stringify(right.attendees) &&
    left.addMeet === right.addMeet &&
    left.transparency === right.transparency &&
    left.visibility === right.visibility &&
    JSON.stringify(left.conference ?? null) === JSON.stringify(right.conference ?? null) &&
    left.recurringEventId === right.recurringEventId &&
    left.originalStartAt === right.originalStartAt &&
    left.repeatFrequency === right.repeatFrequency &&
    left.repeatCustomFrequency === right.repeatCustomFrequency &&
    left.repeatEndMode === right.repeatEndMode &&
    left.repeatInterval === right.repeatInterval &&
    left.repeatEndsOn === right.repeatEndsOn &&
    left.repeatCount === right.repeatCount &&
    left.repeatWeekdays.join(",") === right.repeatWeekdays.join(",") &&
    left.repeatMonthlyMode === right.repeatMonthlyMode &&
    left.repeatMonthDay === right.repeatMonthDay &&
    left.repeatSetPos === right.repeatSetPos
  );
}

function normalizeDraftReminders(draft: CalendarEventDraft): CalendarEventReminder[] {
  if (draft.reminders.length > 0) {
    return draft.reminders
      .filter((reminder) => reminder.method === "popup" || reminder.method === "email")
      .filter((reminder) => Number.isInteger(reminder.minutes) && reminder.minutes >= 0 && reminder.minutes <= 28 * 24 * 60)
      .slice(0, 10);
  }

  return draft.reminderMinutes === ""
    ? []
    : normalizeReminderMinutes([Number(draft.reminderMinutes)]).map((minutes) => ({ method: "popup", minutes }));
}

function calendarDraftRecurrence(draft: CalendarEventDraft): CalendarEventRecurrence | null {
  if (draft.repeatFrequency === "none") {
    return null;
  }

  const frequency = draft.repeatFrequency === "custom" ? draft.repeatCustomFrequency : draft.repeatFrequency;
  const interval = Math.min(366, Math.max(1, Number.parseInt(draft.repeatInterval, 10) || 1));
  const count = draft.repeatEndMode !== "after" || draft.repeatCount.trim() === ""
    ? null
    : Math.min(366, Math.max(1, Number.parseInt(draft.repeatCount, 10) || 1));

  return {
    frequency,
    interval: draft.repeatFrequency === "custom" ? interval : 1,
    endsOn: draft.repeatEndMode === "on" ? draft.repeatEndsOn.trim() || null : null,
    count,
    ...(draft.repeatFrequency === "custom" && frequency === "monthly" && draft.repeatMonthlyMode === "dayOfMonth"
      ? { byMonthDay: Math.min(31, Math.max(1, Number.parseInt(draft.repeatMonthDay, 10) || 1)) }
      : {}),
    ...(draft.repeatFrequency === "custom" && frequency === "monthly" && draft.repeatMonthlyMode === "weekday"
      ? { byDay: draft.repeatWeekdays.slice(0, 1), bySetPos: Math.min(5, Math.max(-5, Number.parseInt(draft.repeatSetPos, 10) || 1)) }
      : {}),
    ...(draft.repeatFrequency === "custom" && frequency === "weekly" && draft.repeatWeekdays.length > 0
      ? { byDay: draft.repeatWeekdays }
      : {})
  };
}

function calendarDraftRecurrenceFromRule(rule: string | null | undefined): (CalendarEventRecurrence & {
  repeatCustomFrequency: CalendarEventRecurrence["frequency"];
  repeatEndMode: CalendarEventDraft["repeatEndMode"];
  repeatFrequency: CalendarEventDraft["repeatFrequency"];
}) | null {
  const line = rule
    ?.split("\n")
    .map((candidate) => candidate.trim())
    .find((candidate) => candidate.startsWith("RRULE:"));

  if (!line) {
    return null;
  }

  const parts = Object.fromEntries(
    line
      .slice("RRULE:".length)
      .split(";")
      .map((part) => part.split("=", 2))
      .filter((part): part is [string, string] => part.length === 2)
  );
  const frequency = parts.FREQ?.toLowerCase();
  const byDay = parts.BYDAY
    ?.split(",")
    .map((day) => day.replace(/^[+-]?\d+/, ""))
    .filter((day): day is CalendarRepeatWeekday => recurrenceWeekdays.includes(day as CalendarRepeatWeekday));

  if (
    frequency !== "daily" &&
    frequency !== "weekly" &&
    frequency !== "monthly" &&
    frequency !== "yearly"
  ) {
    return null;
  }

  const interval = Math.min(366, Math.max(1, Number.parseInt(parts.INTERVAL ?? "1", 10) || 1));
  const count = parts.COUNT ? Math.min(366, Math.max(1, Number.parseInt(parts.COUNT, 10) || 1)) : null;
  const endsOn = parts.UNTIL ? recurrenceDateInputValue(parts.UNTIL) : null;
  const byMonthDay = parts.BYMONTHDAY ? Math.min(31, Math.max(1, Number.parseInt(parts.BYMONTHDAY, 10) || 1)) : null;
  const bySetPos = parts.BYSETPOS ? Math.min(5, Math.max(-5, Number.parseInt(parts.BYSETPOS, 10) || 1)) : null;
  const custom = interval !== 1 || count !== null || endsOn !== null || byMonthDay !== null || bySetPos !== null || (byDay?.length ?? 0) > 0;

  return {
    frequency,
    interval,
    endsOn,
    count,
    ...(byDay?.length ? { byDay } : {}),
    ...(byMonthDay !== null ? { byMonthDay } : {}),
    ...(bySetPos !== null ? { bySetPos } : {}),
    repeatCustomFrequency: frequency,
    repeatEndMode: count !== null ? "after" : endsOn !== null ? "on" : "never",
    repeatFrequency: custom ? "custom" : frequency
  };
}

function recurrenceDateInputValue(value: string): string | null {
  const dateOnly = /^(\d{4})(\d{2})(\d{2})/.exec(value);

  return dateOnly ? `${dateOnly[1]}-${dateOnly[2]}-${dateOnly[3]}` : null;
}

export function calendarRecurrenceSummary(draft: CalendarEventDraft): string {
  const recurrence = calendarDraftRecurrence(draft);

  if (!recurrence) {
    return "Does not repeat";
  }

  const unit =
    recurrence.frequency === "daily"
      ? "day"
      : recurrence.frequency === "weekly"
        ? "week"
        : recurrence.frequency === "monthly"
          ? "month"
          : "year";
  const cadence = recurrence.interval === 1
    ? `Every ${unit}`
    : `Every ${recurrence.interval} ${unit}s`;
  const weekdayLabels = recurrence.frequency === "weekly" && recurrence.byDay?.length
    ? ` on ${recurrence.byDay.map(recurrenceWeekdayLabel).join(", ")}`
    : "";
  const monthlyLabel = recurrence.frequency === "monthly" && recurrence.byMonthDay
    ? ` on day ${recurrence.byMonthDay}`
    : recurrence.frequency === "monthly" && recurrence.bySetPos && recurrence.byDay?.[0]
      ? ` on the ${recurrenceSetPosLabel(recurrence.bySetPos)} ${recurrenceWeekdayLabel(recurrence.byDay[0])}`
      : "";
  const qualifiers = [
    recurrence.endsOn ? `until ${recurrence.endsOn}` : null,
    recurrence.count ? `${recurrence.count} times` : null
  ].filter((part): part is string => part !== null);

  return qualifiers.length > 0
    ? `${cadence}${weekdayLabels}${monthlyLabel}, ${qualifiers.join(", ")}`
    : `${cadence}${weekdayLabels}${monthlyLabel}`;
}

export function calendarRecurrenceRulePreview(draft: CalendarEventDraft): string {
  const recurrence = calendarDraftRecurrence(draft);

  if (!recurrence) {
    return "";
  }

  const parts = [
    `FREQ=${recurrence.frequency.toUpperCase()}`,
    `INTERVAL=${recurrence.interval}`
  ];

  if (recurrence.byDay?.length) {
    parts.push(`BYDAY=${recurrence.byDay.join(",")}`);
  }

  if (recurrence.byMonthDay) {
    parts.push(`BYMONTHDAY=${recurrence.byMonthDay}`);
  }

  if (recurrence.bySetPos) {
    parts.push(`BYSETPOS=${recurrence.bySetPos}`);
  }

  if (recurrence.endsOn) {
    parts.push(`UNTIL=${recurrence.endsOn.replace(/-/g, "")}`);
  }

  if (recurrence.count) {
    parts.push(`COUNT=${recurrence.count}`);
  }

  return `RRULE:${parts.join(";")}`;
}

function recurrenceSetPosLabel(value: number): string {
  return value === -1
    ? "last"
    : value === 1
      ? "first"
      : value === 2
        ? "second"
        : value === 3
          ? "third"
          : `${value}th`;
}

function recurrenceWeekdayLabel(day: CalendarRepeatWeekday): string {
  const labels: Record<CalendarRepeatWeekday, string> = {
    SU: "Sun",
    MO: "Mon",
    TU: "Tue",
    WE: "Wed",
    TH: "Thu",
    FR: "Fri",
    SA: "Sat"
  };

  return labels[day];
}

export function allDayEndInputValue(endsAt: string): string {
  const end = new Date(endsAt);
  end.setUTCDate(end.getUTCDate() - 1);
  return dateInputValue(end.toISOString());
}

function calendarDraftLocalDateTimeParts(value: string, timeZone: string): { date: string; time: string } {
  const localValue = calendarDateTimeLocalInputValue(value, timeZone);
  return {
    date: localValue.slice(0, 10),
    time: localValue.slice(11, 16)
  };
}

export function calendarDraftRangeLabel(draft: CalendarEventDraft, timeZone = "UTC"): string {
  if (draft.allDay) {
    return `${dateInputValue(draft.startsAt)} · All day`;
  }

  const start = calendarDraftLocalDateTimeParts(draft.startsAt, timeZone);
  const end = calendarDraftLocalDateTimeParts(draft.endsAt, timeZone);

  return start.date === end.date
    ? `${start.date} · ${start.time}-${end.time}`
    : `${start.date} · ${start.time}-${end.date} · ${end.time}`;
}

export function calendarDraftDurationLabel(draft: CalendarEventDraft): string {
  if (draft.allDay) {
    const days = Math.max(
      1,
      Math.round((Date.parse(draft.endsAt) - Date.parse(draft.startsAt)) / (24 * 60 * 60 * 1000))
    );

    return `${days} day${days === 1 ? "" : "s"}`;
  }

  const minutes = Math.max(0, Math.round((Date.parse(draft.endsAt) - Date.parse(draft.startsAt)) / 60_000));
  if (minutes < 60) {
    return `${minutes} min`;
  }

  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;
  return remainingMinutes === 0 ? `${hours} hr` : `${hours} hr ${remainingMinutes} min`;
}
