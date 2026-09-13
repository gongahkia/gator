import type { SyncStatusResponse } from "@shared/ipc/contracts";

export function visibleCalendarRange(): { start: string; end: string } {
  const start = startOfUtcDay(new Date());
  const end = addUtcDays(start, 1);

  return {
    start: start.toISOString(),
    end: end.toISOString()
  };
}

export function startOfUtcDay(date: Date): Date {
  return new Date(Date.UTC(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate()));
}

export function addUtcDays(value: Date, days: number): Date {
  const date = new Date(value.getTime());

  date.setUTCDate(date.getUTCDate() + days);
  return date;
}

export function dayKey(date: Date): string {
  return date.toISOString().slice(0, 10);
}

export function dateOnlyFromLocalDate(date: Date): string {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");

  return `${year}-${month}-${day}`;
}

export function dueLabel(value: string | null | undefined): string {
  if (!value) {
    return "No date";
  }

  const due = startOfUtcDay(new Date(value));
  const today = startOfUtcDay(new Date());

  if (due.getTime() === today.getTime()) {
    return "Today";
  }

  return due.toISOString().slice(0, 10);
}

export function timeLabel(value: string, timeZone = "UTC"): string {
  const date = new Date(value);

  if (!Number.isFinite(date.getTime())) {
    return "Unknown";
  }

  try {
    return new Intl.DateTimeFormat("en-US", {
      hour: "2-digit",
      hourCycle: "h23",
      minute: "2-digit",
      timeZone
    }).format(date);
  } catch {
    return new Intl.DateTimeFormat("en-US", {
      hour: "2-digit",
      hourCycle: "h23",
      minute: "2-digit",
      timeZone: "UTC"
    }).format(date);
  }
}

export function allDayRangeLabel(startsAt: string, endsAt: string): string {
  const start = dateInputValue(startsAt);
  const exclusiveEnd = new Date(endsAt);
  exclusiveEnd.setUTCDate(exclusiveEnd.getUTCDate() - 1);
  const end = dateInputValue(exclusiveEnd.toISOString());

  return start === end ? "All day" : `${start}-${end}`;
}

export function dateInputValue(value: string): string {
  const parsed = new Date(value);

  if (!Number.isFinite(parsed.getTime())) {
    return "";
  }

  return parsed.toISOString().slice(0, 10);
}

export function shortDateTime(value: string): string {
  const date = new Date(value);

  if (!Number.isFinite(date.getTime())) {
    return "Unknown";
  }

  return `${date.toISOString().slice(0, 10)} ${timeLabel(value)}`;
}

export function weekdayLabel(date: Date): string {
  return ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"][
    date.getUTCDay()
  ];
}

export function monthDayLabel(date: Date): string {
  return `${date.toLocaleString("en-US", { month: "short", timeZone: "UTC" })} ${date.getUTCDate()}`;
}

export function slugId(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "") || "cache";
}

export function syncLabel(status: SyncStatusResponse): string {
  if (status.state === "running") {
    return "Syncing";
  }

  if (status.state === "error") {
    return "Needs attention";
  }

  if (status.offline) {
    return "Offline";
  }

  if (status.stale) {
    return "Stale";
  }

  return "Ready";
}
