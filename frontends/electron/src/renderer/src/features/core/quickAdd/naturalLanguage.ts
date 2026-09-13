import type { CalendarEventRecurrence } from "@shared/ipc/contracts";

export type QuickAddMode = "event" | "task" | "note" | "birthday";

export interface MatchedToken {
  kind: "date" | "list" | "time" | "duration" | "location" | "allDay" | "recurrence";
  display: string;
}

export interface ParsedQuickAddTask {
  title: string;
  dueDate: string | null;
  taskListHint: string | null;
  matchedTokens: MatchedToken[];
}

export interface ParsedQuickAddEvent {
  summary: string;
  startDate: Date | null;
  endDate: Date | null;
  location: string | null;
  isAllDay: boolean;
  recurrence: CalendarEventRecurrence | null;
  matchedTokens: MatchedToken[];
}

const monthNames: Record<string, number> = {
  jan: 1,
  january: 1,
  feb: 2,
  february: 2,
  mar: 3,
  march: 3,
  apr: 4,
  april: 4,
  may: 5,
  jun: 6,
  june: 6,
  jul: 7,
  july: 7,
  aug: 8,
  august: 8,
  sep: 9,
  sept: 9,
  september: 9,
  oct: 10,
  october: 10,
  nov: 11,
  november: 11,
  dec: 12,
  december: 12
};

const weekdayIndexes: Record<string, number> = {
  sun: 0,
  sunday: 0,
  su: 0,
  mon: 1,
  monday: 1,
  mo: 1,
  tue: 2,
  tues: 2,
  tuesday: 2,
  tu: 2,
  wed: 3,
  wednesday: 3,
  we: 3,
  thu: 4,
  thur: 4,
  thurs: 4,
  thursday: 4,
  th: 4,
  fri: 5,
  friday: 5,
  fr: 5,
  sat: 6,
  saturday: 6,
  sa: 6
};

const weekdayCodes = ["SU", "MO", "TU", "WE", "TH", "FR", "SA"] as const;
type WeekdayCode = (typeof weekdayCodes)[number];

function cleanSpaces(value: string): string {
  return value.replace(/\s+/g, " ").trim();
}

function startOfLocalDay(value: Date): Date {
  return new Date(value.getFullYear(), value.getMonth(), value.getDate());
}

function addDays(value: Date, days: number): Date {
  const next = new Date(value.getTime());
  next.setDate(next.getDate() + days);
  return next;
}

function addMonths(value: Date, months: number): Date {
  const next = new Date(value.getTime());
  next.setMonth(next.getMonth() + months);
  return next;
}

function addYears(value: Date, years: number): Date {
  const next = new Date(value.getTime());
  next.setFullYear(next.getFullYear() + years);
  return next;
}

function dateOnly(value: Date): string {
  const month = String(value.getMonth() + 1).padStart(2, "0");
  const day = String(value.getDate()).padStart(2, "0");
  return `${value.getFullYear()}-${month}-${day}`;
}

function displayDate(value: Date): string {
  return value.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

function displayWeekday(value: Date, forceNext: boolean): string {
  const label = value.toLocaleDateString(undefined, { weekday: "long" });
  return forceNext ? `Next ${label}` : label;
}

function endOfWeek(now: Date): Date {
  const today = startOfLocalDay(now);
  const delta = (6 - today.getDay() + 7) % 7 || 7;
  return addDays(today, delta);
}

function endOfMonth(now: Date): Date {
  return new Date(now.getFullYear(), now.getMonth() + 1, 0);
}

function endOfYear(now: Date): Date {
  return new Date(now.getFullYear(), 11, 31);
}

function resolveWeekday(text: string, now: Date, forceNext: boolean): { date: Date; display: string } | null {
  const weekday = (() => {
    switch (text) {
      case "sun":
      case "sunday":
      case "su":
        return 0;
      case "mon":
      case "monday":
      case "mo":
        return 1;
      case "tue":
      case "tues":
      case "tuesday":
      case "tu":
        return 2;
      case "wed":
      case "wednesday":
      case "we":
        return 3;
      case "thu":
      case "thur":
      case "thurs":
      case "thursday":
      case "th":
        return 4;
      case "fri":
      case "friday":
      case "fr":
        return 5;
      case "sat":
      case "saturday":
      case "sa":
        return 6;
      default:
        return -1;
    }
  })();

  if (weekday < 0) {
    return null;
  }

  const today = startOfLocalDay(now);
  let delta = weekday - today.getDay();

  if (delta <= 0) {
    delta += 7;
  }

  if (forceNext && delta < 7) {
    delta += 7;
  }

  const date = addDays(today, delta);
  return { date, display: displayWeekday(date, forceNext) };
}

function resolveMonthDay(month: number, day: number, now: Date): { date: Date; display: string } | null {
  if (month < 1 || month > 12 || day < 1 || day > 31) {
    return null;
  }

  const today = startOfLocalDay(now);
  let date = new Date(now.getFullYear(), month - 1, day);

  if (date.getMonth() !== month - 1 || date.getDate() !== day) {
    return null;
  }

  if (date < today) {
    date = new Date(now.getFullYear() + 1, month - 1, day);
  }

  return { date, display: displayDate(date) };
}

function resolveIsoDate(year: number, month: number, day: number): { date: Date; display: string } | null {
  if (year < 1970 || month < 1 || month > 12 || day < 1 || day > 31) {
    return null;
  }

  const date = new Date(year, month - 1, day);

  if (date.getFullYear() !== year || date.getMonth() !== month - 1 || date.getDate() !== day) {
    return null;
  }

  return {
    date,
    display: date.toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" })
  };
}

function removeRange(value: string, index: number, length: number): string {
  return `${value.slice(0, index)} ${value.slice(index + length)}`;
}

function extractHashHint(text: string): { hint: string; index: number; length: number } | null {
  const match = /(^|\s)#([A-Za-z0-9][A-Za-z0-9_-]{0,119})(?=\s|$)/.exec(text);

  if (!match || match.index === undefined) {
    return null;
  }

  const prefix = match[1] ?? "";
  return {
    hint: match[2] ?? "",
    index: match.index + prefix.length,
    length: match[0].length - prefix.length
  };
}

type DateHit = { date: Date; display: string; index: number; length: number };

function firstDateHit(text: string, now: Date, allowNextWeekday: boolean): DateHit | null {
  const lower = text.toLowerCase();
  const checks: Array<(value: string) => DateHit | null> = [
    (value) => matchSimple(value, /\beod\b/, startOfLocalDay(now), "End of day"),
    (value) => matchSimple(value, /\beow\b/, endOfWeek(now), "End of week"),
    (value) => matchSimple(value, /\beom\b/, endOfMonth(now), "End of month"),
    (value) => matchSimple(value, /\beoy\b/, endOfYear(now), "End of year"),
    (value) => matchSimple(value, /\b(today|tdy|tnt|tonight|td)\b/, startOfLocalDay(now), "Today"),
    (value) => matchSimple(value, /\b(tomorrow|tmrw|tmr|tmw|tomo|2mrw|2moro|2mro)\b/, addDays(startOfLocalDay(now), 1), "Tomorrow"),
    (value) => matchSimple(value, /\b(yesterday|ytd|yday)\b/, addDays(startOfLocalDay(now), -1), "Yesterday"),
    (value) => matchSimple(value, /\b(day\s+after\s+tomorrow|dat)\b/, addDays(startOfLocalDay(now), 2), "Day after tomorrow"),
    matchRelative,
    (value) => matchSimple(value, /\b(next\s+week|nw)\b/, addDays(startOfLocalDay(now), 7), "Next week"),
    (value) => matchSimple(value, /\b(next\s+month|nm)\b/, addMonths(startOfLocalDay(now), 1), "Next month"),
    (value) => matchSimple(value, /\b(next\s+year|ny)\b/, addYears(startOfLocalDay(now), 1), "Next year"),
    (value) => matchWeekend(value),
    (value) => matchWeekday(value, allowNextWeekday),
    matchMonthNameDay,
    matchDayMonthName,
    matchIso,
    matchNumericMonthDay
  ];

  for (const check of checks) {
    const hit = check(lower);

    if (hit && text[hit.index - 1] !== "#") {
      return hit;
    }
  }

  return null;

  function matchSimple(value: string, pattern: RegExp, date: Date, display: string): DateHit | null {
    const match = pattern.exec(value);
    return match ? { date, display, index: match.index, length: match[0].length } : null;
  }

  function matchRelative(value: string): DateHit | null {
    const match = /\bin\s+(\d{1,3})\s+(hour|hours|hr|hrs|day|days|d|week|weeks|wk|wks|month|months|mo)\b/.exec(value);

    if (!match) {
      return null;
    }

    const amount = Number(match[1]);
    const unit = match[2] ?? "day";
    const today = startOfLocalDay(now);

    if (["hour", "hours", "hr", "hrs"].includes(unit)) {
      return { date: addDays(today, 0), display: "Today", index: match.index, length: match[0].length };
    }

    if (["week", "weeks", "wk", "wks"].includes(unit)) {
      return {
        date: addDays(today, amount * 7),
        display: `In ${amount} week${amount === 1 ? "" : "s"}`,
        index: match.index,
        length: match[0].length
      };
    }

    if (["month", "months", "mo"].includes(unit)) {
      return {
        date: addMonths(today, amount),
        display: `In ${amount} month${amount === 1 ? "" : "s"}`,
        index: match.index,
        length: match[0].length
      };
    }

    return {
      date: addDays(today, amount),
      display: `In ${amount} day${amount === 1 ? "" : "s"}`,
      index: match.index,
      length: match[0].length
    };
  }

  function matchWeekend(value: string): DateHit | null {
    const match = /\b(this\s+)?weekend\b/.exec(value);
    const resolved = resolveWeekday("sat", now, false);
    return match && resolved
      ? { date: resolved.date, display: "Weekend", index: match.index, length: match[0].length }
      : null;
  }

  function matchWeekday(value: string, allowNext: boolean): DateHit | null {
    const pattern = allowNext
      ? /\b(next\s+)?(monday|tuesday|wednesday|thursday|friday|saturday|sunday|mon|tue|tues|wed|thu|thur|thurs|fri|sat|sun|mo|tu|we|th|fr|sa|su)\b/
      : /\b(monday|tuesday|wednesday|thursday|friday|saturday|sunday|mon|tue|tues|wed|thu|thur|thurs|fri|sat|sun|mo|tu|we|th|fr|sa|su)\b/;
    const match = pattern.exec(value);

    if (!match) {
      return null;
    }

    const weekdayText = match[allowNext ? 2 : 1] ?? "";
    const forceNext = allowNext && Boolean(match[1]);
    const resolved = resolveWeekday(weekdayText, now, forceNext);

    return resolved
      ? { date: resolved.date, display: resolved.display, index: match.index, length: match[0].length }
      : null;
  }

  function matchMonthNameDay(value: string): DateHit | null {
    const match = /\b(jan|feb|mar|apr|may|jun|jul|aug|sep|sept|oct|nov|dec|january|february|march|april|may|june|july|august|september|october|november|december)\s+(\d{1,2})(?:st|nd|rd|th)?\b/.exec(value);
    const resolved = match ? resolveMonthDay(monthNames[match[1] ?? ""] ?? 0, Number(match[2]), now) : null;
    return match && resolved
      ? { date: resolved.date, display: resolved.display, index: match.index, length: match[0].length }
      : null;
  }

  function matchDayMonthName(value: string): DateHit | null {
    const match = /\b(\d{1,2})(?:st|nd|rd|th)?\s+(jan|feb|mar|apr|may|jun|jul|aug|sep|sept|oct|nov|dec|january|february|march|april|may|june|july|august|september|october|november|december)\b/.exec(value);
    const resolved = match ? resolveMonthDay(monthNames[match[2] ?? ""] ?? 0, Number(match[1]), now) : null;
    return match && resolved
      ? { date: resolved.date, display: resolved.display, index: match.index, length: match[0].length }
      : null;
  }

  function matchIso(value: string): DateHit | null {
    const match = /\b(\d{4})-(\d{1,2})-(\d{1,2})\b/.exec(value);
    const resolved = match ? resolveIsoDate(Number(match[1]), Number(match[2]), Number(match[3])) : null;
    return match && resolved
      ? { date: resolved.date, display: resolved.display, index: match.index, length: match[0].length }
      : null;
  }

  function matchNumericMonthDay(value: string): DateHit | null {
    const match = /\b(\d{1,2})[/./-](\d{1,2})\b/.exec(value);
    const resolved = match ? resolveMonthDay(Number(match[1]), Number(match[2]), now) : null;
    return match && resolved
      ? { date: resolved.date, display: resolved.display, index: match.index, length: match[0].length }
      : null;
  }
}

type RecurrenceHit = {
  recurrence: CalendarEventRecurrence;
  display: string;
  index: number;
  length: number;
  dateHint: Date | null;
  ranges: Array<{ index: number; length: number }>;
};

function weekdayCodeForText(text: string): WeekdayCode | null {
  const index = weekdayIndexes[text.toLowerCase()];
  return index === undefined ? null : weekdayCodes[index] ?? null;
}

function nextDateForWeekdays(days: WeekdayCode[], now: Date): Date | null {
  if (days.length === 0) {
    return null;
  }

  const today = startOfLocalDay(now);
  let best: Date | null = null;

  for (const day of days) {
    const target = weekdayCodes.indexOf(day);
    let delta = target - today.getDay();

    if (delta <= 0) {
      delta += 7;
    }

    const candidate = addDays(today, delta);

    if (!best || candidate < best) {
      best = candidate;
    }
  }

  return best;
}

function extractWeekdayCodes(text: string): WeekdayCode[] {
  const days: WeekdayCode[] = [];
  const seen = new Set<WeekdayCode>();
  const pattern = /\b(sundays?|mondays?|tuesdays?|wednesdays?|thursdays?|fridays?|saturdays?|sun|mon|tue|tues|wed|thu|thur|thurs|fri|sat|su|mo|tu|we|th|fr|sa)\b/gi;
  let match: RegExpExecArray | null;

  while ((match = pattern.exec(text)) !== null) {
    const code = weekdayCodeForText((match[1] ?? "").replace(/s$/i, ""));

    if (code && !seen.has(code)) {
      seen.add(code);
      days.push(code);
    }
  }

  return days;
}

function recurrenceDisplay(recurrence: CalendarEventRecurrence): string {
  const unit = recurrence.frequency === "daily"
    ? "day"
    : recurrence.frequency === "weekly"
      ? "week"
      : recurrence.frequency === "monthly"
        ? "month"
        : "year";
  const cadence = recurrence.interval === 1 ? `Every ${unit}` : `Every ${recurrence.interval} ${unit}s`;
  const days = recurrence.byDay?.length ? ` on ${recurrence.byDay.join(",")}` : "";
  const end = recurrence.endsOn ? ` until ${recurrence.endsOn}` : recurrence.count ? ` for ${recurrence.count} times` : "";

  return `${cadence}${days}${end}`;
}

function extractRecurrence(text: string, now: Date): RecurrenceHit | null {
  const lower = text.toLowerCase();
  const checks: Array<(value: string) => Omit<RecurrenceHit, "display" | "ranges"> | null> = [
    matchEveryOtherWeekday,
    matchEveryUnit,
    matchEveryWeekday,
    matchSimpleCadence,
    matchWeekdays,
    matchPluralWeekday
  ];

  for (const check of checks) {
    const hit = check(lower);

    if (hit) {
      const end = extractRecurrenceEnd(text, now, hit.index + hit.length);
      const recurrence = {
        ...hit.recurrence,
        ...(end?.endsOn ? { endsOn: end.endsOn } : {}),
        ...(end?.count ? { count: end.count } : {})
      };

      return {
        ...hit,
        recurrence,
        display: recurrenceDisplay(recurrence),
        ranges: end?.ranges ?? []
      };
    }
  }

  return null;

  function matchEveryOtherWeekday(value: string): Omit<RecurrenceHit, "display" | "ranges"> | null {
    const match = /\b(?:every\s+other|alternate|alternating)\s+(sundays?|mondays?|tuesdays?|wednesdays?|thursdays?|fridays?|saturdays?|sun|mon|tue|tues|wed|thu|thur|thurs|fri|sat|su|mo|tu|we|th|fr|sa)\b/.exec(value);
    const byDay = match ? extractWeekdayCodes(match[1] ?? "") : [];

    return match && byDay.length
      ? {
          recurrence: {
            frequency: "weekly",
            interval: 2,
            endsOn: null,
            count: null,
            byDay
          },
          index: match.index,
          length: match[0].length,
          dateHint: nextDateForWeekdays(byDay, now)
        }
      : null;
  }

  function matchEveryUnit(value: string): Omit<RecurrenceHit, "display" | "ranges"> | null {
    const match = /\b(?:every|each)\s+(\d{1,3})?\s*(days|day|weeks|week|months|month|years|year)(?:\s+on\s+([a-z,\s/&-]{2,80}))?/.exec(value);

    if (!match) {
      return null;
    }

    const unit = match[2] ?? "week";
    const frequency = unit.startsWith("day")
      ? "daily"
      : unit.startsWith("week")
        ? "weekly"
        : unit.startsWith("month")
          ? "monthly"
          : "yearly";
    const byDay = frequency === "weekly" && match[3] ? extractWeekdayCodes(match[3]) : [];
    const recurrence: CalendarEventRecurrence = {
      frequency,
      interval: Math.max(1, Math.min(366, Number(match[1] ?? "1") || 1)),
      endsOn: null,
      count: null,
      ...(byDay.length ? { byDay } : {})
    };

    return {
      recurrence,
      index: match.index,
      length: match[0].length,
      dateHint: nextDateForWeekdays(byDay, now)
    };
  }

  function matchEveryWeekday(value: string): Omit<RecurrenceHit, "display" | "ranges"> | null {
    const match = /\b(?:every|each)\s+((?:sun(?:day)?s?|mon(?:day)?s?|tue(?:s|sday)?s?|wed(?:nesday)?s?|thu(?:r|rs|rsday|rday|day)?s?|fri(?:day)?s?|sat(?:urday)?s?)(?:\s*(?:,|\/|&|and)\s*(?:sun(?:day)?s?|mon(?:day)?s?|tue(?:s|sday)?s?|wed(?:nesday)?s?|thu(?:r|rs|rsday|rday|day)?s?|fri(?:day)?s?|sat(?:urday)?s?))*)\b/.exec(value);
    const byDay = match ? extractWeekdayCodes(match[1] ?? "") : [];

    return match && byDay.length
      ? {
          recurrence: {
            frequency: "weekly",
            interval: 1,
            endsOn: null,
            count: null,
            byDay
          },
          index: match.index,
          length: match[0].length,
          dateHint: nextDateForWeekdays(byDay, now)
        }
      : null;
  }

  function matchSimpleCadence(value: string): Omit<RecurrenceHit, "display" | "ranges"> | null {
    const match = /\b(daily|weekly|monthly|yearly|annually)\b/.exec(value);

    if (!match) {
      return null;
    }

    const word = match[1] ?? "weekly";
    const frequency = word === "daily"
      ? "daily"
      : word === "weekly"
        ? "weekly"
        : word === "monthly"
          ? "monthly"
          : "yearly";

    return {
      recurrence: {
        frequency,
        interval: 1,
        endsOn: null,
        count: null
      },
      index: match.index,
      length: match[0].length,
      dateHint: null
    };
  }

  function matchWeekdays(value: string): Omit<RecurrenceHit, "display" | "ranges"> | null {
    const match = /\b(weekdays|weekends)\b/.exec(value);

    if (!match) {
      return null;
    }

    const byDay: WeekdayCode[] = match[1] === "weekdays"
      ? ["MO", "TU", "WE", "TH", "FR"]
      : ["SA", "SU"];

    return {
      recurrence: {
        frequency: "weekly",
        interval: 1,
        endsOn: null,
        count: null,
        byDay
      },
      index: match.index,
      length: match[0].length,
      dateHint: nextDateForWeekdays(byDay, now)
    };
  }

  function matchPluralWeekday(value: string): Omit<RecurrenceHit, "display" | "ranges"> | null {
    const match = /\b(sundays|mondays|tuesdays|wednesdays|thursdays|fridays|saturdays)\b/.exec(value);
    const byDay = match ? extractWeekdayCodes(match[1] ?? "") : [];

    return match && byDay.length
      ? {
          recurrence: {
            frequency: "weekly",
            interval: 1,
            endsOn: null,
            count: null,
            byDay
          },
          index: match.index,
          length: match[0].length,
          dateHint: nextDateForWeekdays(byDay, now)
        }
      : null;
  }
}

function extractRecurrenceEnd(
  text: string,
  now: Date,
  startIndex: number
): { endsOn: string | null; count: number | null; ranges: Array<{ index: number; length: number }> } | null {
  const working = text.slice(startIndex);
  const lower = working.toLowerCase();
  const ranges: Array<{ index: number; length: number }> = [];
  let endsOn: string | null = null;
  let count: number | null = null;
  const until = /\b(?:until|through|thru|ending|ends\s+on)\b/.exec(lower);

  if (until) {
    const suffix = working.slice(until.index);
    const dateHit = firstDateHit(suffix, now, true);

    if (dateHit) {
      endsOn = dateOnly(dateHit.date);
      ranges.push({
        index: startIndex + until.index,
        length: dateHit.index + dateHit.length
      });
    }
  }

  const countMatch = /\b(?:for\s+)?(\d{1,3})\s+(?:times|occurrences|instances)\b|\b(\d{1,3})x\b/.exec(lower);

  if (countMatch) {
    count = Math.max(1, Math.min(366, Number(countMatch[1] ?? countMatch[2])));
    ranges.push({
      index: startIndex + countMatch.index,
      length: countMatch[0].length
    });
  }

  return endsOn || count ? { endsOn, count, ranges } : null;
}

export function parseQuickAddTask(input: string, now = new Date()): ParsedQuickAddTask {
  let working = ` ${input.trim()} `;
  const matchedTokens: MatchedToken[] = [];
  let taskListHint: string | null = null;
  const hash = extractHashHint(working);

  if (hash) {
    taskListHint = hash.hint;
    working = removeRange(working, hash.index, hash.length);
    matchedTokens.push({ kind: "list", display: `#${hash.hint}` });
  }

  let dueDate: string | null = null;
  const dateHit = firstDateHit(working, now, true);

  if (dateHit) {
    dueDate = dateOnly(dateHit.date);
    working = removeRange(working, dateHit.index, dateHit.length);
    matchedTokens.push({ kind: "date", display: dateHit.display });
  }

  return {
    title: cleanSpaces(working),
    dueDate,
    taskListHint,
    matchedTokens
  };
}

type RawTime = { hour: number; minute: number; meridiem: "am" | "pm" | null };
type ResolvedTime = { hour: number; minute: number };
type TimeHit = { start: ResolvedTime; end: ResolvedTime | null; index: number; length: number };

function resolveTime(raw: RawTime, inherited: "am" | "pm" | null = null): ResolvedTime | null {
  const meridiem = raw.meridiem ?? inherited;

  if (raw.minute < 0 || raw.minute > 59) {
    return null;
  }

  if (meridiem) {
    if (raw.hour < 1 || raw.hour > 12) {
      return null;
    }

    return {
      hour: raw.hour % 12 + (meridiem === "pm" ? 12 : 0),
      minute: raw.minute
    };
  }

  return raw.hour >= 0 && raw.hour <= 23 ? { hour: raw.hour, minute: raw.minute } : null;
}

function timeMinutes(value: ResolvedTime): number {
  return value.hour * 60 + value.minute;
}

function rawTime(match: RegExpExecArray, hourGroup: number, minuteGroup: number, meridiemGroup?: number): RawTime | null {
  const hour = Number(match[hourGroup]);
  const minute = match[minuteGroup] === undefined || match[minuteGroup] === "" ? 0 : Number(match[minuteGroup]);
  const marker = meridiemGroup === undefined ? "" : (match[meridiemGroup] ?? "").toLowerCase();
  const meridiem = marker.startsWith("p") ? "pm" : marker.startsWith("a") ? "am" : null;

  return Number.isInteger(hour) && Number.isInteger(minute) ? { hour, minute, meridiem } : null;
}

function matchTimeExpression(text: string): TimeHit | null {
  const lower = text.toLowerCase();
  const timeToken = String.raw`(\d{1,2})(?:[:.](\d{2}))?\s*((?:a|p)\.?m\.?)?`;
  const rangePattern = new RegExp(String.raw`\b(?:from\s+|at\s+)?${timeToken}\s*(?:-|\u2013|\u2014|to|until|til|till)\s*${timeToken}\b`);
  const range = rangePattern.exec(lower);

  if (range) {
    const startRaw = rawTime(range, 1, 2, 3);
    const endRaw = rawTime(range, 4, 5, 6);
    const end = endRaw ? resolveTime(endRaw) : null;
    let start = startRaw ? resolveTime(startRaw, startRaw.meridiem ? null : endRaw?.meridiem ?? null) : null;

    if (start && end && startRaw?.meridiem === null && endRaw?.meridiem === "pm" && timeMinutes(start) > timeMinutes(end)) {
      start = resolveTime(startRaw, "am");
    }

    if (start && endRaw?.meridiem === null && end && timeMinutes(end) <= timeMinutes(start) && end.hour + 12 <= 23) {
      return {
        start,
        end: { ...end, hour: end.hour + 12 },
        index: range.index,
        length: range[0].length
      };
    }

    return start && end ? { start, end, index: range.index, length: range[0].length } : null;
  }

  const keyword = /\b(?:at\s+)?(noon|midnight)\b/.exec(lower);

  if (keyword) {
    return {
      start: keyword[1] === "noon" ? { hour: 12, minute: 0 } : { hour: 0, minute: 0 },
      end: null,
      index: keyword.index,
      length: keyword[0].length
    };
  }

  const ampm = /\b(?:at\s+)?(\d{1,2})(?:[:.](\d{2}))?\s*(a|p)\.?m\.?\b/.exec(lower);

  if (ampm) {
    const start = resolveTime(rawTime(ampm, 1, 2, 3) ?? { hour: -1, minute: -1, meridiem: null });
    return start ? { start, end: null, index: ampm.index, length: ampm[0].length } : null;
  }

  const compact = /\b(?:at\s+)?(\d{1,2})([0-5]\d)\s*(a|p)\.?m\.?\b/.exec(lower);

  if (compact) {
    const start = resolveTime(rawTime(compact, 1, 2, 3) ?? { hour: -1, minute: -1, meridiem: null });
    return start ? { start, end: null, index: compact.index, length: compact[0].length } : null;
  }

  const twentyFour = /\b(?:at\s+)?([01]?\d|2[0-3])[:.]([0-5]\d)\b/.exec(lower);

  if (twentyFour) {
    const start = resolveTime(rawTime(twentyFour, 1, 2) ?? { hour: -1, minute: -1, meridiem: null });
    return start ? { start, end: null, index: twentyFour.index, length: twentyFour[0].length } : null;
  }

  return null;
}

function withTime(base: Date, time: ResolvedTime): Date {
  return new Date(base.getFullYear(), base.getMonth(), base.getDate(), time.hour, time.minute, 0, 0);
}

function displayTimeRange(start: Date, end: Date | null): string {
  const startText = start.toLocaleString(undefined, { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" });

  if (!end) {
    return startText;
  }

  const sameDay = dateOnly(start) === dateOnly(end);
  const endText = sameDay
    ? end.toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" })
    : end.toLocaleString(undefined, { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" });

  return `${startText}-${endText}`;
}

function extractDuration(text: string): { minutes: number; display: string; index: number; length: number } | null {
  const lower = text.toLowerCase();
  const checks: Array<(value: string) => { minutes: number; index: number; length: number } | null> = [
    (value) => {
      const match = /\b(?:for\s+)?(\d{1,3})\s*h(?:r|rs|our|ours)?\s*(\d{1,2})\s*m(?:in|ins|inute|inutes)?\b/.exec(value);
      return match ? { minutes: Number(match[1]) * 60 + Number(match[2]), index: match.index, length: match[0].length } : null;
    },
    (value) => {
      const match = /\b(?:for\s+)?(\d+(?:\.\d+)?)\s*(hours?|hrs?|h)\b/.exec(value);
      return match ? { minutes: Math.round(Number(match[1]) * 60), index: match.index, length: match[0].length } : null;
    },
    (value) => {
      const match = /\b(?:for\s+)?(\d{1,3})\s*(min|mins|minutes?|m)\b/.exec(value);
      return match ? { minutes: Number(match[1]), index: match.index, length: match[0].length } : null;
    },
    (value) => {
      const match = /\bhalf\s+an?\s*hour\b|\bhalf\s+hour\b/.exec(value);
      return match ? { minutes: 30, index: match.index, length: match[0].length } : null;
    },
    (value) => {
      const match = /\bquarter\s+(?:of\s+)?an?\s*hour\b|\bquarter\s+hour\b/.exec(value);
      return match ? { minutes: 15, index: match.index, length: match[0].length } : null;
    }
  ];

  for (const check of checks) {
    const hit = check(lower);

    if (hit && hit.minutes > 0) {
      return {
        ...hit,
        display: hit.minutes >= 60
          ? `${Math.floor(hit.minutes / 60)}h${hit.minutes % 60 === 0 ? "" : ` ${hit.minutes % 60}m`}`
          : `${hit.minutes}m`
      };
    }
  }

  return null;
}

function extractLocation(text: string): { location: string; index: number; length: number } | null {
  const quoted = /\s(?:@|at\s+)(["“])([^"”]{1,200})["”]/.exec(text);

  if (quoted?.[2]?.trim()) {
    return {
      location: quoted[2].trim(),
      index: quoted.index,
      length: quoted[0].length
    };
  }

  const match = /\s(?:@|at\s+)([A-Za-z0-9][^#\n]{1,160}?)(?=\s+(?:with|about|for|every|daily|weekly|monthly|yearly|until|through|thru)\b|\s+#|$)/i.exec(text);
  const location = match?.[1]?.trim().replace(/[,.]$/, "");

  return match && location
    ? { location, index: match.index, length: match[0].length }
    : null;
}

export function parseQuickAddEvent(input: string, now = new Date()): ParsedQuickAddEvent {
  let working = ` ${input.trim()} `;
  const matchedTokens: MatchedToken[] = [];
  const duration = extractDuration(working);
  let durationMinutes: number | null = null;

  if (duration) {
    durationMinutes = duration.minutes;
    working = removeRange(working, duration.index, duration.length);
    matchedTokens.push({ kind: "duration", display: duration.display });
  }

  const recurrenceHit = extractRecurrence(working, now);
  let recurrence: CalendarEventRecurrence | null = null;

  if (recurrenceHit) {
    recurrence = recurrenceHit.recurrence;
    const ranges = [
      { index: recurrenceHit.index, length: recurrenceHit.length },
      ...recurrenceHit.ranges
    ].sort((left, right) => right.index - left.index);

    for (const range of ranges) {
      working = removeRange(working, range.index, range.length);
    }

    matchedTokens.push({ kind: "recurrence", display: recurrenceHit.display });
  }

  const dateHit = firstDateHit(working, now, true);
  const timeHit = matchTimeExpression(working);
  let startDate: Date | null = null;
  let endDate: Date | null = null;
  let isAllDay = false;

  if (timeHit) {
    const base = dateHit?.date ?? recurrenceHit?.dateHint ?? startOfLocalDay(now);
    startDate = withTime(base, timeHit.start);
    endDate = timeHit.end ? withTime(base, timeHit.end) : null;

    if (endDate && endDate <= startDate) {
      endDate = addDays(endDate, 1);
    }

    if (!endDate) {
      endDate = new Date(startDate.getTime() + (durationMinutes ?? 60) * 60_000);
    }

    const ranges = [
      ...(dateHit ? [{ index: dateHit.index, length: dateHit.length }] : []),
      { index: timeHit.index, length: timeHit.length }
    ].sort((left, right) => right.index - left.index);

    for (const range of ranges) {
      working = removeRange(working, range.index, range.length);
    }

    matchedTokens.push({ kind: "time", display: displayTimeRange(startDate, endDate) });
  } else if (dateHit) {
    startDate = dateHit.date;
    endDate = dateHit.date;
    isAllDay = true;
    working = removeRange(working, dateHit.index, dateHit.length);
    matchedTokens.push({ kind: "date", display: dateHit.display });
    matchedTokens.push({ kind: "allDay", display: "All-day" });
  } else if (recurrenceHit?.dateHint) {
    startDate = recurrenceHit.dateHint;
    endDate = recurrenceHit.dateHint;
    isAllDay = true;
    matchedTokens.push({ kind: "date", display: recurrenceHit.dateHint.toLocaleDateString(undefined, { month: "short", day: "numeric" }) });
    matchedTokens.push({ kind: "allDay", display: "All-day" });
  }

  const location = extractLocation(working);
  let locationValue: string | null = null;

  if (location) {
    locationValue = location.location;
    working = removeRange(working, location.index, location.length);
    matchedTokens.push({ kind: "location", display: `@${location.location}` });
  }

  return {
    summary: cleanSpaces(working),
    startDate,
    endDate,
    location: locationValue,
    isAllDay,
    recurrence,
    matchedTokens
  };
}

export function normalizeDestinationToken(value: string): string {
  return value.toLowerCase().replace(/[^a-z0-9]/g, "");
}

export function stripHashToken(title: string, token: string | null): string {
  if (!token) {
    return title.trim();
  }

  const escaped = token.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const stripped = title.replace(new RegExp(`(^|\\s)#${escaped}(?=\\s|$)`, "i"), " ");
  return cleanSpaces(stripped) || title.trim();
}

export function firstHashHint(input: string): string | null {
  return extractHashHint(` ${input.trim()} `)?.hint ?? null;
}

export function toIso(value: Date): string {
  return value.toISOString();
}

export function toDateInput(value: Date): string {
  return dateOnly(value);
}
