import {
  resolveCalendarEventDisplayColor,
  type CalendarEventSummary,
  type ScheduledTaskBlockMoveRequest,
  type SettingsSnapshot,
  type ScheduledTaskBlockSummary
} from "@shared/ipc/contracts";
import type { ColorThemeDefinition } from "@shared/ipc/themeCatalog";
import type {
  CalendarDayViewModel,
  CalendarEventViewModel,
  CalendarMonthWeekViewModel,
  ScheduledTaskBlockViewModel,
  TaskViewModel
} from "../coreViewModels";
import {
  allDayRangeLabel,
  dayKey,
  monthDayLabel,
  startOfUtcDay,
  timeLabel,
  weekdayLabel
} from "./dateFormat";
import type { CalendarEventDayIndex } from "./types";

export function stableCalendarEventViewModel(
  event: CalendarEventSummary,
  calendarTitle: string | undefined,
  calendarTimeZone: string | null | undefined,
  calendarBackgroundColor: string | null | undefined,
  calendarForegroundColor: string | null | undefined,
  calendarEventColorOverrides: SettingsSnapshot["calendarEventColorOverrides"],
  colorTheme: ColorThemeDefinition,
  defaultTimeZone: string,
  cache: Map<string, { signature: string; viewModel: CalendarEventViewModel }>
): CalendarEventViewModel {
  const timeZone = event.timeZone?.trim() || calendarTimeZone?.trim() || defaultTimeZone || "UTC";
  const displayColor = resolveCalendarEventDisplayColor({
    colorId: event.colorId,
    colorTheme,
    overrides: calendarEventColorOverrides,
    calendarBackgroundColor,
    calendarForegroundColor
  });
  const signature = [
    event.id,
    event.accountId ?? "",
    event.eventId ?? "",
    event.hcbKind ?? "",
    event.status ?? "",
    event.calendarId,
    event.colorId ?? "",
    event.title,
    event.startsAt,
    event.endsAt,
    timeZone,
    event.allDay ? "1" : "0",
    event.location ?? "",
    event.notes ?? "",
    (event.guestEmails ?? []).join("\u001f"),
    (event.reminderMinutes ?? []).join("\u001f"),
    (event.attendees ?? []).map((attendee) => `${attendee.email}:${attendee.responseStatus ?? ""}`).join("\u001f"),
    (event.reminders ?? []).map((reminder) => `${reminder.method}:${reminder.minutes}`).join("\u001f"),
    event.remindersUseDefault === true ? "1" : "0",
    event.transparency ?? "",
    event.visibility ?? "",
    (event.tags ?? []).join("\u001f"),
    event.conference ? JSON.stringify(event.conference) : "",
    event.mutationState ?? "",
    event.completedAt ?? "",
    event.recurrenceRule ?? "",
    event.recurringEventId ?? "",
    event.originalStartAt ?? "",
    calendarTitle ?? "",
    calendarBackgroundColor ?? "",
    calendarForegroundColor ?? "",
    colorTheme.id,
    displayColor.background ?? "",
    displayColor.foreground ?? ""
  ].join("\u001c");
  const cached = cache.get(event.id);

  if (cached?.signature === signature) {
    return cached.viewModel;
  }

  const viewModel: CalendarEventViewModel = {
    id: event.id,
    ...(event.accountId === undefined ? {} : { accountId: event.accountId }),
    eventId: event.eventId ?? event.id,
    hcbKind: event.hcbKind,
    status: event.status ?? "confirmed",
    sourceKind: "event",
    calendarId: event.calendarId,
    colorId: event.colorId ?? null,
    title: event.title,
    calendar: calendarTitle ?? event.calendarId,
    calendarBackgroundColor: calendarBackgroundColor ?? null,
    calendarForegroundColor: calendarForegroundColor ?? null,
    displayBackgroundColor: displayColor.background,
    displayForegroundColor: displayColor.foreground,
    completedAt: event.completedAt ?? null,
    timeLabel: event.allDay ? "All day" : timeLabel(event.startsAt, timeZone),
    rangeLabel: event.allDay
      ? allDayRangeLabel(event.startsAt, event.endsAt)
      : `${timeLabel(event.startsAt, timeZone)}-${timeLabel(event.endsAt, timeZone)}`,
    startsAt: event.startsAt,
    endsAt: event.endsAt,
    timeZone,
    allDay: event.allDay,
    location: event.location?.trim() || (event.allDay ? "All day" : "Scheduled"),
    notes: event.notes?.trim() || "No notes",
    guestEmails: event.guestEmails ?? [],
    reminderMinutes: event.reminderMinutes ?? [],
    attendees: event.attendees ?? [],
    reminders: event.reminders ?? [],
    remindersUseDefault: event.remindersUseDefault === true,
    transparency: event.transparency ?? null,
    visibility: event.visibility ?? null,
    tags: event.tags ?? [],
    conference: event.conference ?? null,
    mutationState: event.mutationState,
    recurrenceRule: event.recurrenceRule ?? null,
    recurringEventId: event.recurringEventId ?? null,
    originalStartAt: event.originalStartAt ?? null
  };

  cache.set(event.id, { signature, viewModel });
  return viewModel;
}

export function stableTaskCalendarEventViewModel(
  task: TaskViewModel & { dueDate: string },
  defaultTimeZone: string,
  cache: Map<string, { signature: string; viewModel: CalendarEventViewModel }>
): CalendarEventViewModel {
  const startsAt = `${task.dueDate}T00:00:00.000Z`;
  const endsAt = taskCalendarEndIso(task.dueDate);
  const id = `task-calendar:${task.id}`;
  const completed = task.status === "completed";
  const signature = [
    id,
    task.id,
    task.listId,
    task.title,
    task.detail,
    task.dueDate ?? "",
    task.status,
    task.mutationState ?? "",
    (task.tags ?? []).join("\u001f"),
    task.list,
    defaultTimeZone
  ].join("\u001c");
  const cached = cache.get(id);

  if (cached?.signature === signature) {
    return cached.viewModel;
  }

  const viewModel: CalendarEventViewModel = {
    id,
    eventId: id,
    sourceKind: "task",
    taskId: task.id,
    taskListId: task.listId,
    taskStatus: task.status,
    calendarId: `task-list:${task.listId}`,
    colorId: null,
    title: task.title,
    calendar: task.list,
    calendarBackgroundColor: completed ? "#e5e7eb" : "#f9a8d4",
    calendarForegroundColor: completed ? "#6b7280" : "#3f0f24",
    displayBackgroundColor: completed ? "#f3f4f6" : "#fce7f3",
    displayForegroundColor: completed ? "#6b7280" : "#3f0f24",
    timeLabel: completed ? "Done" : "Due",
    rangeLabel: completed ? "Completed task" : "Task due",
    startsAt,
    endsAt,
    timeZone: defaultTimeZone || "UTC",
    allDay: true,
    location: task.list,
    notes: task.detail || "No notes",
    guestEmails: [],
    reminderMinutes: [],
    attendees: [],
    reminders: [],
    remindersUseDefault: false,
    transparency: null,
    visibility: null,
    tags: task.tags ?? [],
    conference: null,
    mutationState: task.mutationState,
    completedAt: null,
    recurrenceRule: null
  };

  cache.set(id, { signature, viewModel });
  return viewModel;
}

export function stableProjectedTaskCalendarEventViewModel(
  event: CalendarEventSummary & { linkedTaskId: string },
  task: TaskViewModel,
  defaultTimeZone: string,
  cache: Map<string, { signature: string; viewModel: CalendarEventViewModel }>
): CalendarEventViewModel {
  const timeZone = event.timeZone?.trim() || defaultTimeZone || "UTC";
  const completed = task.status === "completed";
  const signature = [
    event.id,
    event.eventId ?? "",
    event.linkedTaskId,
    event.calendarId,
    event.startsAt,
    event.endsAt,
    event.allDay ? "1" : "0",
    event.updatedAt,
    task.id,
    task.listId,
    task.title,
    task.detail,
    task.status,
    task.mutationState ?? "",
    (task.tags ?? []).join("\u001f"),
    task.list,
    timeZone
  ].join("\u001c");
  const cached = cache.get(event.id);

  if (cached?.signature === signature) {
    return cached.viewModel;
  }

  const viewModel: CalendarEventViewModel = {
    id: event.id,
    eventId: event.eventId ?? event.id,
    sourceKind: "task",
    taskId: task.id,
    taskListId: task.listId,
    taskStatus: task.status,
    calendarId: `task-list:${task.listId}`,
    colorId: null,
    title: task.title,
    calendar: task.list,
    calendarBackgroundColor: completed ? "#e5e7eb" : "#f9a8d4",
    calendarForegroundColor: completed ? "#6b7280" : "#3f0f24",
    displayBackgroundColor: completed ? "#f3f4f6" : "#fce7f3",
    displayForegroundColor: completed ? "#6b7280" : "#3f0f24",
    timeLabel: event.allDay ? (completed ? "Done" : "Due") : timeLabel(event.startsAt, timeZone),
    rangeLabel: event.allDay
      ? completed ? "Completed task" : "Task due"
      : `${timeLabel(event.startsAt, timeZone)}-${timeLabel(event.endsAt, timeZone)}`,
    startsAt: event.startsAt,
    endsAt: event.endsAt,
    timeZone,
    allDay: event.allDay,
    location: task.list,
    notes: task.detail || "No notes",
    guestEmails: [],
    reminderMinutes: [],
    attendees: [],
    reminders: [],
    remindersUseDefault: false,
    transparency: null,
    visibility: null,
    tags: task.tags ?? [],
    conference: null,
    mutationState: task.mutationState,
    completedAt: null,
    recurrenceRule: null
  };

  cache.set(event.id, { signature, viewModel });
  return viewModel;
}

function taskCalendarEndIso(dueDate: string): string {
  const end = new Date(`${dueDate}T00:00:00.000Z`);
  end.setUTCDate(end.getUTCDate() + 1);
  return end.toISOString();
}

export function scheduledTaskBlockViewModel(
  block: ScheduledTaskBlockSummary,
  calendarTitle: string | undefined,
  conflictTitles: string[] = []
): ScheduledTaskBlockViewModel {
  return {
    id: block.id,
    taskId: block.taskId,
    calendarEventId: block.calendarEventId,
    calendarId: block.calendarId,
    title: block.title,
    calendar: calendarTitle ?? block.calendarId,
    timeLabel: timeLabel(block.startsAt),
    rangeLabel: `${timeLabel(block.startsAt)}-${timeLabel(block.endsAt)}`,
    startsAt: block.startsAt,
    endsAt: block.endsAt,
    durationMinutes: block.durationMinutes,
    status: block.status,
    mutationState: block.mutationState,
    conflictCount: conflictTitles.length,
    conflictTitles
  };
}

export function optimisticScheduledBlockPatch(
  block: ScheduledTaskBlockSummary,
  request: ScheduledTaskBlockMoveRequest
): ScheduledTaskBlockSummary {
  const startsAt = request.startsAt ?? block.startsAt;
  const durationMinutes = request.durationMinutes ?? block.durationMinutes;

  return {
    ...block,
    ...(request.calendarId === undefined ? {} : { calendarId: request.calendarId }),
    startsAt,
    endsAt: new Date(Date.parse(startsAt) + durationMinutes * 60 * 1000).toISOString(),
    durationMinutes,
    mutationState: "queued" as const,
    updatedAt: new Date().toISOString()
  };
}

export function scheduledTaskBlockConflicts(
  blocks: ScheduledTaskBlockSummary[],
  events: CalendarEventViewModel[],
  scheduledEventIds: Set<string>
): Map<string, string[]> {
  const timedEventsByDay = new Map<string, CalendarEventViewModel[]>();
  const blocksByDay = new Map<string, ScheduledTaskBlockSummary[]>();
  const conflicts = new Map<string, string[]>();

  for (const event of events) {
    if (
      event.allDay ||
      scheduledEventIds.has(event.eventId) ||
      scheduledEventIds.has(event.id)
    ) {
      continue;
    }

    addRangeToDayBuckets(timedEventsByDay, event, event.startsAt, event.endsAt);
  }

  for (const block of blocks) {
    addRangeToDayBuckets(blocksByDay, block, block.startsAt, block.endsAt);
  }

  for (const block of blocks) {
    const titles = new Set<string>();
    const timedCandidates = candidatesForRange(timedEventsByDay, block.startsAt, block.endsAt);
    const blockCandidates = candidatesForRange(blocksByDay, block.startsAt, block.endsAt);

    for (const event of timedCandidates) {
      if (dateRangesOverlap(block.startsAt, block.endsAt, event.startsAt, event.endsAt)) {
        titles.add(event.title);
      }
    }

    for (const otherBlock of blockCandidates) {
      if (
        otherBlock.id !== block.id &&
        dateRangesOverlap(block.startsAt, block.endsAt, otherBlock.startsAt, otherBlock.endsAt)
      ) {
        titles.add(otherBlock.title);
      }
    }

    conflicts.set(block.id, Array.from(titles).slice(0, 3));
  }

  return conflicts;
}

function addRangeToDayBuckets<T>(
  buckets: Map<string, T[]>,
  value: T,
  startsAt: string,
  endsAt: string
): void {
  for (const key of dayKeysForRange(startsAt, endsAt)) {
    const values = buckets.get(key) ?? [];
    values.push(value);
    buckets.set(key, values);
  }
}

function candidatesForRange<T>(
  buckets: Map<string, T[]>,
  startsAt: string,
  endsAt: string
): T[] {
  const candidates = new Set<T>();

  for (const key of dayKeysForRange(startsAt, endsAt)) {
    for (const value of buckets.get(key) ?? []) {
      candidates.add(value);
    }
  }

  return Array.from(candidates);
}

export function nextUpTimelineItem(
  events: CalendarEventViewModel[],
  blocks: ScheduledTaskBlockViewModel[],
  now: Date
): {
  kind: "event" | "scheduledTaskBlock";
  itemId: string;
  title: string;
  detail: string;
} | null {
  const nowMs = now.getTime();
  const candidates = [
    ...events
      .filter((event) => !event.allDay && Date.parse(event.endsAt) > nowMs)
      .map((event) => ({
        kind: "event" as const,
        itemId: event.id,
        title: event.title,
        detail: `${event.rangeLabel} - ${event.calendar}`,
        startsAt: event.startsAt,
        endsAt: event.endsAt
      })),
    ...blocks
      .filter((block) => Date.parse(block.endsAt) > nowMs)
      .map((block) => ({
        kind: "scheduledTaskBlock" as const,
        itemId: block.id,
        title: block.title,
        detail: `${block.rangeLabel} - ${block.calendar}`,
        startsAt: block.startsAt,
        endsAt: block.endsAt
      }))
  ].sort(compareTimelineRows);

  const [next] = candidates;

  if (!next) {
    return null;
  }

  return {
    kind: next.kind,
    itemId: next.itemId,
    title: next.title,
    detail: next.detail
  };
}

export function compareTimelineRows(
  left: { startsAt: string; endsAt: string; itemId: string },
  right: { startsAt: string; endsAt: string; itemId: string }
): number {
  return (
    left.startsAt.localeCompare(right.startsAt) ||
    left.endsAt.localeCompare(right.endsAt) ||
    left.itemId.localeCompare(right.itemId)
  );
}

function dateRangesOverlap(
  leftStart: string,
  leftEnd: string,
  rightStart: string,
  rightEnd: string
): boolean {
  return Date.parse(leftStart) < Date.parse(rightEnd) && Date.parse(leftEnd) > Date.parse(rightStart);
}

function dayKeysForRange(startsAt: string, endsAt: string): string[] {
  const startMs = Date.parse(startsAt);
  const endMs = Date.parse(endsAt);

  if (!Number.isFinite(startMs) || !Number.isFinite(endMs) || endMs <= startMs) {
    return [];
  }

  const keys: string[] = [];
  const cursor = startOfUtcDay(new Date(startMs));
  const lastDay = startOfUtcDay(new Date(endMs - 1));

  while (cursor.getTime() <= lastDay.getTime()) {
    keys.push(dayKey(cursor));
    cursor.setUTCDate(cursor.getUTCDate() + 1);
  }

  return keys;
}

export function buildCalendarEventDayIndex(events: CalendarEventViewModel[]): CalendarEventDayIndex {
  const eventsByDay = new Map<string, CalendarEventViewModel[]>();

  for (const event of events) {
    const start = new Date(event.startsAt);
    const end = new Date(event.endsAt);

    if (!Number.isFinite(start.getTime()) || !Number.isFinite(end.getTime()) || end <= start) {
      continue;
    }

    const cursor = startOfUtcDay(start);
    const lastDay = startOfUtcDay(new Date(end.getTime() - 1));

    while (cursor.getTime() <= lastDay.getTime()) {
      const key = dayKey(cursor);
      const dayEvents = eventsByDay.get(key) ?? [];
      dayEvents.push(event);
      eventsByDay.set(key, dayEvents);
      cursor.setUTCDate(cursor.getUTCDate() + 1);
    }
  }

  for (const dayEvents of eventsByDay.values()) {
    dayEvents.sort(
      (left, right) =>
        left.startsAt.localeCompare(right.startsAt) ||
        left.endsAt.localeCompare(right.endsAt) ||
        left.id.localeCompare(right.id)
    );
  }

  return { eventsByDay };
}

export function eventsForDate(index: CalendarEventDayIndex, date: Date): CalendarEventViewModel[] {
  return index.eventsByDay.get(dayKey(date)) ?? [];
}

export function dayView(index: CalendarEventDayIndex): CalendarDayViewModel {
  const today = startOfUtcDay(new Date());

  return {
    id: `day-${today.toISOString().slice(0, 10)}`,
    weekday: weekdayLabel(today),
    dateLabel: monthDayLabel(today),
    isToday: true,
    events: eventsForDate(index, today)
  };
}

export function weekDays(index: CalendarEventDayIndex): CalendarDayViewModel[] {
  const today = startOfUtcDay(new Date());
  const sunday = new Date(today);
  sunday.setUTCDate(today.getUTCDate() - today.getUTCDay());

  return Array.from({ length: 7 }, (_, dayOffset) => {
    const date = new Date(sunday);
    date.setUTCDate(sunday.getUTCDate() + dayOffset);

    return {
      id: `week-${date.toISOString().slice(0, 10)}`,
      weekday: weekdayLabel(date).slice(0, 3),
      dateLabel: String(date.getUTCDate()),
      isToday: date.getTime() === today.getTime(),
      events: eventsForDate(index, date)
    };
  });
}

export function monthWeeks(index: CalendarEventDayIndex): CalendarMonthWeekViewModel[] {
  const today = startOfUtcDay(new Date());
  const first = new Date(Date.UTC(today.getUTCFullYear(), today.getUTCMonth(), 1));
  const gridStart = new Date(first);
  gridStart.setUTCDate(first.getUTCDate() - first.getUTCDay());

  return Array.from({ length: 6 }, (_, weekIndex) => ({
    id: `month-week-${weekIndex}`,
    days: Array.from({ length: 7 }, (_, dayIndex) => {
      const date = new Date(gridStart);
      date.setUTCDate(gridStart.getUTCDate() + weekIndex * 7 + dayIndex);

      return {
        id: `month-${date.toISOString().slice(0, 10)}`,
        weekday: weekdayLabel(date).slice(0, 3),
        dateLabel: String(date.getUTCDate()),
        isToday: date.getTime() === today.getTime(),
        isOutsideMonth: date.getUTCMonth() !== today.getUTCMonth(),
        events: eventsForDate(index, date)
      };
    })
  }));
}
