import { startOfUtcDayIso } from "../../coreScreenShared";
import type { CalendarDayViewModel, CalendarViewId } from "../../coreViewModels";

export interface CalendarLocalPoint {
  dayKey: string;
  minutes: number;
}

export function hourSlotIso(day: string, hour: number, timeZone = "UTC"): string {
  return zonedDateTimeIso(day, hour, 0, timeZone);
}

export function addUtcMinutesIso(value: string, minutes: number): string {
  return new Date(Date.parse(value) + minutes * 60 * 1000).toISOString();
}

export function hourSlotLabel(hour: number): string {
  return `${String(hour).padStart(2, "0")}:00`;
}

export function calendarDisplayHourLabel(hour: number): string {
  if (hour === 0) {
    return "12 AM";
  }

  if (hour === 12) {
    return "12 PM";
  }

  return hour < 12 ? `${hour} AM` : `${hour - 12} PM`;
}

export function calendarDayKey(day: CalendarDayViewModel): string {
  const separatorIndex = day.id.indexOf("-");
  return separatorIndex >= 0 ? day.id.slice(separatorIndex + 1) : day.id;
}

export function calendarViewLabel(viewId: CalendarViewId): string {
  if (viewId === "multiDay") {
    return "Multi-Day";
  }

  return `${viewId[0].toUpperCase()}${viewId.slice(1)}`;
}

export function isCalendarTimelineView(viewId: CalendarViewId): boolean {
  return viewId === "day" || viewId === "multiDay" || viewId === "week";
}

function calendarDateFromDay(day: CalendarDayViewModel): Date {
  return new Date(`${calendarDayKey(day)}T00:00:00.000Z`);
}

export function calendarDateTitle(day: CalendarDayViewModel, includeYear = true): string {
  return calendarDateTitleFromIso(calendarDayKey(day), includeYear);
}

export function calendarDateTitleFromIso(day: string, includeYear = true): string {
  return new Intl.DateTimeFormat(undefined, {
    day: "numeric",
    month: "long",
    year: includeYear ? "numeric" : undefined,
    timeZone: "UTC"
  }).format(new Date(`${day}T00:00:00.000Z`));
}

export function calendarRangeTitle(days: CalendarDayViewModel[]): string {
  const firstDay = days[0];
  const lastDay = days.at(-1);

  if (!firstDay || !lastDay) {
    return "Calendar";
  }

  const start = calendarDateFromDay(firstDay);
  const end = calendarDateFromDay(lastDay);
  const sameMonth =
    start.getUTCFullYear() === end.getUTCFullYear() &&
    start.getUTCMonth() === end.getUTCMonth();

  if (sameMonth) {
    const month = new Intl.DateTimeFormat(undefined, {
      month: "long",
      timeZone: "UTC"
    }).format(start);
    return `${month} ${start.getUTCDate()}-${end.getUTCDate()}, ${start.getUTCFullYear()}`;
  }

  const formatter = new Intl.DateTimeFormat(undefined, {
    day: "numeric",
    month: "short",
    timeZone: "UTC"
  });
  return `${formatter.format(start)} - ${formatter.format(end)}, ${end.getUTCFullYear()}`;
}

export function calendarIsoDate(date: Date): string {
  return date.toISOString().slice(0, 10);
}

export function calendarDateFromIsoDate(day: string): Date {
  return new Date(`${day}T00:00:00.000Z`);
}

export function calendarUtcDayOffset(fromDay: string, toDay: string): number {
  return Math.round(
    (calendarDateFromIsoDate(toDay).getTime() - calendarDateFromIsoDate(fromDay).getTime()) / 86_400_000
  );
}

export function calendarTodayKey(): string {
  return calendarIsoDate(new Date(startOfUtcDayIso(new Date())));
}

export function calendarCurrentLocalPoint(timeZone = "UTC"): CalendarLocalPoint {
  return calendarLocalPoint(new Date().toISOString(), timeZone);
}

export function calendarCurrentDayKey(timeZone = "UTC"): string {
  return calendarCurrentLocalPoint(timeZone).dayKey;
}

export function calendarDateTimeLocalInputValue(value: string, timeZone = "UTC"): string {
  const parsed = new Date(value);

  if (!Number.isFinite(parsed.getTime())) {
    return "";
  }

  const point = calendarLocalPoint(value, timeZone);
  const hour = Math.floor(point.minutes / 60);
  const minute = point.minutes % 60;

  return `${point.dayKey}T${String(hour).padStart(2, "0")}:${String(minute).padStart(2, "0")}`;
}

export function calendarDateTimeLocalInputToIso(value: string, timeZone = "UTC"): string {
  const match = /^(\d{4}-\d{2}-\d{2})T(\d{2}):(\d{2})$/.exec(value);

  if (!match) {
    return new Date().toISOString();
  }

  return zonedDateTimeIso(match[1], Number(match[2]), Number(match[3]), timeZone);
}

export function calendarStartOfUtcDate(date: Date): Date {
  return new Date(Date.UTC(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate()));
}

export function calendarAddUtcDays(day: string, days: number): string {
  const date = calendarDateFromIsoDate(day);
  date.setUTCDate(date.getUTCDate() + days);
  return calendarIsoDate(date);
}

export function calendarAddUtcMonths(day: string, months: number): string {
  const source = calendarDateFromIsoDate(day);
  const target = new Date(Date.UTC(source.getUTCFullYear(), source.getUTCMonth() + months, 1));
  const lastDay = new Date(Date.UTC(target.getUTCFullYear(), target.getUTCMonth() + 1, 0)).getUTCDate();

  target.setUTCDate(Math.min(source.getUTCDate(), lastDay));
  return calendarIsoDate(target);
}

export function calendarMonthOffset(fromDay: string, toDay: string): number {
  const from = calendarDateFromIsoDate(fromDay);
  const to = calendarDateFromIsoDate(toDay);

  return (to.getUTCFullYear() - from.getUTCFullYear()) * 12 + to.getUTCMonth() - from.getUTCMonth();
}

export function calendarMonthTitle(day: string): string {
  return new Intl.DateTimeFormat(undefined, {
    month: "long",
    year: "numeric",
    timeZone: "UTC"
  }).format(calendarDateFromIsoDate(day));
}

export function calendarWeekdayLabel(date: Date, style: "long" | "short" = "short"): string {
  return new Intl.DateTimeFormat(undefined, {
    weekday: style,
    timeZone: "UTC"
  }).format(date);
}

export function sameTimeOnDate(value: string, day: string): string {
  return `${day}T${value.slice(11)}`;
}

export function calendarEventTimeOfDayIso(sourceIso: string, dayKey: string, timeZone = "UTC"): string {
  const parsed = new Date(sourceIso);

  if (!Number.isFinite(parsed.getTime())) {
    return zonedDateTimeIso(dayKey, 0, 0, timeZone);
  }

  const point = calendarLocalPoint(sourceIso, timeZone);

  return zonedDateTimeIso(dayKey, Math.floor(point.minutes / 60), point.minutes % 60, timeZone);
}

export function calendarLocalPoint(value: string, timeZone: string): CalendarLocalPoint {
  const date = new Date(value);

  if (!Number.isFinite(date.getTime())) {
    return { dayKey: value.slice(0, 10), minutes: 0 };
  }

  const parts = calendarLocalDateTimeParts(date, timeZone);

  return {
    dayKey: `${parts.year}-${parts.month}-${parts.day}`,
    minutes: Number(parts.hour) * 60 + Number(parts.minute)
  };
}

function calendarLocalDateTimeParts(
  date: Date,
  timeZone: string
): { year: string; month: string; day: string; hour: string; minute: string } {
  try {
    const formatter = new Intl.DateTimeFormat("en-CA", {
      day: "2-digit",
      hour: "2-digit",
      hourCycle: "h23",
      minute: "2-digit",
      month: "2-digit",
      timeZone,
      year: "numeric"
    });
    const parts = Object.fromEntries(
      formatter.formatToParts(date).map((part) => [part.type, part.value])
    );

    return {
      year: parts.year ?? "1970",
      month: parts.month ?? "01",
      day: parts.day ?? "01",
      hour: parts.hour ?? "00",
      minute: parts.minute ?? "00"
    };
  } catch {
    return calendarLocalDateTimeParts(date, "UTC");
  }
}

export function zonedDateTimeIso(dayKey: string, hour: number, minute: number, timeZone: string): string {
  const [year = 1970, month = 1, day = 1] = dayKey.split("-").map(Number);
  const baseDate = new Date(Date.UTC(year, month - 1, day));
  const normalizedMinute = Math.max(0, Math.min(59, minute));
  const dayOffset = Math.floor(hour / 24);
  const normalizedHour = ((hour % 24) + 24) % 24;

  baseDate.setUTCDate(baseDate.getUTCDate() + dayOffset);

  const target: CalendarLocalPoint = {
    dayKey: calendarIsoDate(baseDate),
    minutes: normalizedHour * 60 + normalizedMinute
  };
  let utcMs = Date.UTC(
    baseDate.getUTCFullYear(),
    baseDate.getUTCMonth(),
    baseDate.getUTCDate(),
    Math.floor(target.minutes / 60),
    target.minutes % 60
  );

  for (let index = 0; index < 4; index += 1) {
    const actual = calendarLocalPoint(new Date(utcMs).toISOString(), timeZone);
    const deltaMinutes = calendarLocalPointSerial(target) - calendarLocalPointSerial(actual);

    if (deltaMinutes === 0) {
      break;
    }

    utcMs += deltaMinutes * 60_000;
  }

  return new Date(utcMs).toISOString();
}

export function compareCalendarLocalPoints(left: CalendarLocalPoint, right: CalendarLocalPoint): number {
  return left.dayKey.localeCompare(right.dayKey) || left.minutes - right.minutes;
}

function calendarLocalPointSerial(point: CalendarLocalPoint): number {
  return Date.parse(`${point.dayKey}T00:00:00.000Z`) / 60_000 + point.minutes;
}
