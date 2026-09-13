import type {
  CalendarDayViewModel,
  CalendarEventViewModel,
  CalendarMonthWeekViewModel
} from "../../coreViewModels";
import type { CalendarEventDayIndex } from "./types";
import {
  calendarAddUtcDays,
  calendarDateFromIsoDate,
  calendarIsoDate,
  calendarLocalPoint,
  calendarStartOfUtcDate,
  calendarTodayKey,
  calendarWeekdayLabel
} from "./calendarDateUtils";

export function calendarEventRangeDayKeys(event: CalendarEventViewModel): string[] {
  if (event.allDay) {
    return calendarUtcRangeDayKeys(event.startsAt, event.endsAt);
  }

  const startMs = Date.parse(event.startsAt);
  const endMs = Date.parse(event.endsAt);

  if (!Number.isFinite(startMs) || !Number.isFinite(endMs) || endMs <= startMs) {
    return [];
  }

  const timeZone = event.timeZone || "UTC";
  const start = calendarLocalPoint(event.startsAt, timeZone);
  const end = calendarLocalPoint(new Date(endMs - 1).toISOString(), timeZone);
  const keys: string[] = [];
  const cursor = calendarDateFromIsoDate(start.dayKey);
  const lastDay = calendarDateFromIsoDate(end.dayKey);

  while (cursor.getTime() <= lastDay.getTime()) {
    keys.push(calendarIsoDate(cursor));
    cursor.setUTCDate(cursor.getUTCDate() + 1);
  }

  return keys;
}

function calendarUtcRangeDayKeys(startsAt: string, endsAt: string): string[] {
  const startMs = Date.parse(startsAt);
  const endMs = Date.parse(endsAt);

  if (!Number.isFinite(startMs) || !Number.isFinite(endMs) || endMs <= startMs) {
    return [];
  }

  const keys: string[] = [];
  const cursor = calendarStartOfUtcDate(new Date(startMs));
  const lastDay = calendarStartOfUtcDate(new Date(endMs - 1));

  while (cursor.getTime() <= lastDay.getTime()) {
    keys.push(calendarIsoDate(cursor));
    cursor.setUTCDate(cursor.getUTCDate() + 1);
  }

  return keys;
}

export function buildCalendarEventDayIndex(events: CalendarEventViewModel[]): CalendarEventDayIndex {
  const eventsByDay = new Map<string, CalendarEventViewModel[]>();

  for (const event of events) {
    for (const day of calendarEventRangeDayKeys(event)) {
      const dayEvents = eventsByDay.get(day) ?? [];
      dayEvents.push(event);
      eventsByDay.set(day, dayEvents);
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

export function calendarEventsForDay(index: CalendarEventDayIndex, day: string): CalendarEventViewModel[] {
  return index.eventsByDay.get(day) ?? [];
}

export function calendarDayViewForDate(
  index: CalendarEventDayIndex,
  day: string,
  variant: "day" | "range" | "month" = "range",
  currentMonth?: number
): CalendarDayViewModel {
  const date = calendarDateFromIsoDate(day);

  return {
    id: `${variant}-${day}`,
    weekday: calendarWeekdayLabel(date, variant === "day" ? "long" : "short"),
    dateLabel: String(date.getUTCDate()),
    isToday: day === calendarTodayKey(),
    isOutsideMonth: currentMonth === undefined ? false : date.getUTCMonth() !== currentMonth,
    events: calendarEventsForDay(index, day)
  };
}

export function calendarWeekDaysForDate(index: CalendarEventDayIndex, day: string): CalendarDayViewModel[] {
  const anchor = calendarDateFromIsoDate(day);
  const sunday = new Date(anchor);
  sunday.setUTCDate(anchor.getUTCDate() - anchor.getUTCDay());

  return Array.from({ length: 7 }, (_, dayOffset) => {
    const date = new Date(sunday);
    date.setUTCDate(sunday.getUTCDate() + dayOffset);
    return calendarDayViewForDate(index, calendarIsoDate(date), "range");
  });
}

export function calendarRangeDaysForDate(
  index: CalendarEventDayIndex,
  day: string,
  dayCount: number
): CalendarDayViewModel[] {
  const count = Math.max(1, dayCount);

  return Array.from({ length: count }, (_, dayOffset) =>
    calendarDayViewForDate(index, calendarAddUtcDays(day, dayOffset), count === 1 ? "day" : "range")
  );
}

export function calendarMonthWeeksForDate(
  index: CalendarEventDayIndex,
  day: string
): CalendarMonthWeekViewModel[] {
  const anchor = calendarDateFromIsoDate(day);
  const first = new Date(Date.UTC(anchor.getUTCFullYear(), anchor.getUTCMonth(), 1));
  const gridStart = new Date(first);
  gridStart.setUTCDate(first.getUTCDate() - first.getUTCDay());

  return Array.from({ length: 6 }, (_, weekIndex) => ({
    id: `month-${day}-week-${weekIndex}`,
    days: Array.from({ length: 7 }, (_, dayIndex) => {
      const date = new Date(gridStart);
      date.setUTCDate(gridStart.getUTCDate() + weekIndex * 7 + dayIndex);
      return calendarDayViewForDate(index, calendarIsoDate(date), "month", first.getUTCMonth());
    })
  }));
}
