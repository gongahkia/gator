import type {
  CalendarEventSummary,
  SettingsSnapshot,
  TaskSummary
} from "@shared/ipc/contracts";
import type { GoogleAccountConnectionStatusDto } from "../google";
import type { NativeMenuBarSnapshot, NativePlannerSnapshotSource } from "./types";

interface BuildNativeMenuBarSnapshotOptions {
  planner: NativePlannerSnapshotSource;
  settings: SettingsSnapshot;
  account: GoogleAccountConnectionStatusDto | null;
  now: Date;
}

interface MenuBarSnapshotData {
  overdueTasks: TaskSummary[];
  todayTasks: TaskSummary[];
  tomorrowTasks: TaskSummary[];
  todayEvents: CalendarEventSummary[];
  tomorrowEvents: CalendarEventSummary[];
}

export function buildNativeMenuBarSnapshot(
  options: BuildNativeMenuBarSnapshotOptions
): NativeMenuBarSnapshot {
  const { account, now, planner, settings } = options;
  const todayStart = startOfLocalDay(now);
  const tomorrowStart = addDays(todayStart, 1);
  const dayAfterTomorrowStart = addDays(todayStart, 2);
  const tasks = activeTasks(planner);
  const events = calendarEvents(planner, todayStart, dayAfterTomorrowStart);
  const overdueTasks = tasks
    .filter((task) => taskDueBefore(task, todayStart))
    .sort(compareTasksByDueDate);
  const todayTasks = tasks
    .filter((task) => taskDueBetween(task, todayStart, tomorrowStart))
    .sort(compareTasksByDueDate);
  const tomorrowTasks = tasks
    .filter((task) => taskDueBetween(task, tomorrowStart, dayAfterTomorrowStart))
    .sort(compareTasksByDueDate);
  const todayEvents = events
    .filter((event) => eventStartsBetween(event, todayStart, tomorrowStart))
    .sort(compareEventsByStart);
  const tomorrowEvents = events
    .filter((event) => eventStartsBetween(event, tomorrowStart, dayAfterTomorrowStart))
    .sort(compareEventsByStart);
  const currentEvent = todayEvents.find((event) => {
    const startsAt = dateFromIso(event.startsAt);
    const endsAt = dateFromIso(event.endsAt);

    return Boolean(startsAt && endsAt && startsAt <= now && endsAt > now);
  });
  const nextEvent = todayEvents.find((event) => {
    const startsAt = dateFromIso(event.startsAt);

    return Boolean(startsAt && startsAt > now);
  });
  const sections = menuBarSections(settings.menuBarPanelStyle, {
    overdueTasks,
    todayTasks,
    tomorrowTasks,
    todayEvents,
    tomorrowEvents
  });
  const pinnedFilters = menuBarPinnedFilterItems(planner, settings);
  const todayCount = todayTasks.length + todayEvents.length;
  const calendar =
    settings.menuBarPanelStyle === "calendar"
      ? menuBarCalendarSnapshot(todayStart, todayEvents, todayTasks, now)
      : undefined;
  const accountSnapshot = menuBarAccountSnapshot(account);
  const syncLabel = accountSnapshot?.connectionState === "connected" ? "Synced" : "Local";
  const statusLabel =
    settings.menuBarPanelStyle === "adaptive" ? adaptiveTrayLabel(now, tasks, events) : undefined;
  const title =
    settings.menuBarPanelStyle === "adaptive"
      ? "Agenda"
      : settings.menuBarPanelStyle === "calendar"
        ? "Calendar"
        : menuBarTitle(overdueTasks.length, todayCount, currentEvent, nextEvent);
  const subtitle =
    settings.menuBarPanelStyle === "adaptive" || settings.menuBarPanelStyle === "calendar"
      ? undefined
      : menuBarSubtitle(
          overdueTasks.length,
          todayCount,
          tomorrowTasks.length + tomorrowEvents.length
        );
  const badgeLabel =
    settings.showMenuBarBadge && overdueTasks.length > 0
      ? cappedBadgeLabel(overdueTasks.length)
      : undefined;
  const dockBadgeLabel =
    settings.showDockBadge && overdueTasks.length > 0
      ? cappedBadgeLabel(overdueTasks.length)
      : undefined;
  const tooltip = statusLabel
    ? `Hot Cross Buns - ${statusLabel}`
    : subtitle
      ? `${title} - ${subtitle}`
      : title;

  return {
    panelStyle: settings.menuBarPanelStyle,
    iconName: settings.menuBarIconName,
    calendarIconId: settings.menuBarCalendarIconId,
    calendarDoneMode: settings.menuBarCalendarDoneMode,
    customMenuBarIcons: settings.customMenuBarIcons,
    calendarDone: isCalendarDone(settings, todayTasks, todayEvents),
    primaryClickAction: settings.trayClickAction,
    title,
    subtitle,
    statusLabel,
    syncLabel,
    badgeLabel,
    dockBadgeLabel,
    tooltip,
    sections: pinnedFilters.length > 0
      ? [{ title: "Pinned filters", items: pinnedFilters }, ...sections]
      : sections,
    calendar,
    account: accountSnapshot
  };
}

function menuBarPinnedFilterItems(
  planner: NativePlannerSnapshotSource,
  settings: SettingsSnapshot
): NativeMenuBarSnapshot["sections"][number]["items"] {
  if (!planner.search || settings.pinnedSavedSearchViewIds.length === 0) {
    return [];
  }

  const pinnedIds = new Set(settings.pinnedSavedSearchViewIds);
  return settings.savedSearchViews
    .filter((filter) => pinnedIds.has(filter.id))
    .slice(0, 6)
    .map((filter) => {
      let count = 0;

      try {
        count = planner.search?.({ query: filter.query, limit: 1 }).page.totalKnown ?? 0;
      } catch {
        count = 0;
      }

      return {
        label: truncateMenuLabel(filter.name),
        detail: `${count} match${count === 1 ? "" : "es"}`,
        route: { kind: "search", query: filter.query.slice(0, 200) } as const
      };
    });
}

function activeTasks(planner: NativePlannerSnapshotSource): TaskSummary[] {
  try {
    return planner.listTasks({ status: "active", limit: 100 }).items;
  } catch {
    return [];
  }
}

function calendarEvents(
  planner: NativePlannerSnapshotSource,
  start: Date,
  end: Date
): CalendarEventSummary[] {
  try {
    return planner.listCalendarEvents({
      start: start.toISOString(),
      end: end.toISOString(),
      limit: 100
    }).items;
  } catch {
    return [];
  }
}

function isCalendarDone(
  settings: SettingsSnapshot,
  todayTasks: TaskSummary[],
  todayEvents: CalendarEventSummary[]
): boolean {
  if (settings.menuBarCalendarDoneMode === "neverAutoSwitch") {
    return false;
  }

  if (settings.menuBarCalendarDoneMode === "tasksOnly") {
    return todayTasks.length === 0;
  }

  return todayTasks.length === 0 && todayEvents.every((event) => Boolean(event.completedAt));
}

function menuBarSections(
  style: SettingsSnapshot["menuBarPanelStyle"],
  data: MenuBarSnapshotData
): NativeMenuBarSnapshot["sections"] {
  if (style === "adaptive") {
    return [
      {
        title: "Today",
        items: menuBarEventItems(data.todayEvents, 40)
      },
      {
        title: "Tomorrow",
        items: menuBarEventItems(data.tomorrowEvents, 40)
      }
    ].map((section) =>
      section.items.length > 0
        ? section
        : {
            ...section,
            items: [{ label: "Nothing scheduled", detail: section.title?.toLowerCase() }]
          }
    );
  }

  const sections: NativeMenuBarSnapshot["sections"] = [
    {
      title: "Today",
      items: [
        ...menuBarEventItems(data.todayEvents, 12),
        ...menuBarTaskItems(data.todayTasks, "Due today", 8)
      ].slice(0, 20)
    },
    {
      title: "Tomorrow",
      items: [
        ...menuBarEventItems(data.tomorrowEvents, 12),
        ...menuBarTaskItems(data.tomorrowTasks, "Due tomorrow", 8)
      ].slice(0, 20)
    }
  ];

  return sections.map((section) =>
    section.items.length > 0
      ? section
      : {
          ...section,
          items: [{ label: "Nothing scheduled", detail: section.title?.toLowerCase() }]
        }
  );
}

function menuBarTaskItems(tasks: TaskSummary[], fallbackDetail: string, limit = 5) {
  return tasks.slice(0, limit).map((task) => ({
    label: truncateMenuLabel(task.title),
    detail: task.dueAt ? dueDetail(task.dueAt, fallbackDetail) : fallbackDetail,
    route: { kind: "task", id: task.id } as const
  }));
}

function menuBarEventItems(events: CalendarEventSummary[], limit = 5) {
  return events.slice(0, limit).map((event) => ({
    label: truncateMenuLabel(event.title),
    detail: eventDetail(event),
    route: { kind: "event", id: event.id } as const
  }));
}

function menuBarTitle(
  overdueCount: number,
  todayCount: number,
  currentEvent: CalendarEventSummary | undefined,
  nextEvent: CalendarEventSummary | undefined
): string {
  if (overdueCount > 0) {
    return `${overdueCount} overdue`;
  }

  if (currentEvent) {
    return `Now: ${truncateMenuLabel(currentEvent.title, 36)}`;
  }

  if (nextEvent) {
    return `Next: ${truncateMenuLabel(nextEvent.title, 36)}`;
  }

  if (todayCount > 0) {
    return `${todayCount} today`;
  }

  return "Hot Cross Buns";
}

function menuBarSubtitle(
  overdueCount: number,
  todayCount: number,
  tomorrowCount: number
): string {
  const parts = [
    overdueCount > 0 ? `${overdueCount} overdue` : "",
    todayCount > 0 ? `${todayCount} today` : "Nothing today",
    tomorrowCount > 0 ? `${tomorrowCount} tomorrow` : ""
  ].filter(Boolean);

  return parts.join(", ");
}

function adaptiveTrayLabel(
  now: Date,
  tasks: TaskSummary[],
  events: CalendarEventSummary[]
): string | undefined {
  const currentEvent = events
    .filter((event) => !event.allDay)
    .find((event) => {
      const startsAt = dateFromIso(event.startsAt);
      const endsAt = dateFromIso(event.endsAt);

      return Boolean(startsAt && endsAt && startsAt <= now && endsAt > now);
    });

  if (currentEvent) {
    const endsAt = dateFromIso(currentEvent.endsAt);
    return statusLabel(currentEvent.title, `${endsAt ? durationText(now, endsAt) : "now"} left`);
  }

  const nextEvent = events
    .filter((event) => !event.allDay)
    .find((event) => {
      const startsAt = dateFromIso(event.startsAt);

      return Boolean(startsAt && startsAt > now);
    });

  if (nextEvent) {
    const startsAt = dateFromIso(nextEvent.startsAt);
    return statusLabel(nextEvent.title, startsAt ? `in ${durationText(now, startsAt)}` : "next");
  }

  const nextTask = tasks.filter((task) => task.dueAt).sort(compareTasksByDueDate)[0];

  if (!nextTask || !nextTask.dueAt) {
    return undefined;
  }

  return statusLabel(nextTask.title, taskDueStatus(new Date(now), nextTask.dueAt));
}

function durationText(from: Date, to: Date): string {
  const minutes = Math.max(1, Math.ceil(Math.max(0, to.getTime() - from.getTime()) / 60_000));

  if (minutes < 60) {
    return `${minutes}m`;
  }

  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;

  if (hours < 24) {
    return remainingMinutes === 0 ? `${hours}h` : `${hours}h ${remainingMinutes}m`;
  }

  const days = Math.floor(hours / 24);
  const remainingHours = hours % 24;
  return remainingHours === 0 ? `${days}d` : `${days}d ${remainingHours}h`;
}

function taskDueStatus(now: Date, dueAtIso: string): string {
  const dueAt = dateFromIso(dueAtIso);

  if (!dueAt) {
    return "due";
  }

  const today = startOfLocalDay(now);
  const dueDay = startOfLocalDay(dueAt);
  const dayDelta = Math.round((dueDay.getTime() - today.getTime()) / (24 * 60 * 60 * 1000));

  if (dayDelta < 0) {
    return "overdue";
  }

  if (dayDelta === 0) {
    return "due today";
  }

  if (dayDelta === 1) {
    return "due tomorrow";
  }

  return `due in ${dayDelta}d`;
}

function statusLabel(title: string, detail: string): string {
  const shortened = shortStatusTitle(title);

  return shortened.isTruncated
    ? `${shortened.text} (${detail})`
    : `${shortened.text} - ${detail}`;
}

function shortStatusTitle(value: string): { text: string; isTruncated: boolean } {
  const trimmed = value.trim() || "Untitled";
  const maxLength = 28;

  if (trimmed.length <= maxLength) {
    return { text: trimmed, isTruncated: false };
  }

  return {
    text: `${trimmed.slice(0, maxLength - 3)}...`,
    isTruncated: true
  };
}

function cappedBadgeLabel(count: number): string {
  return count > 99 ? "99+" : String(count);
}

function menuBarCalendarSnapshot(
  selectedDay: Date,
  events: CalendarEventSummary[],
  tasks: TaskSummary[],
  now: Date
): NativeMenuBarSnapshot["calendar"] {
  const sortedEvents = [...events].sort((left, right) => {
    if (left.allDay !== right.allDay) {
      return left.allDay ? -1 : 1;
    }

    return compareEventsByStart(left, right);
  });
  const sortedTasks = [...tasks].sort(compareTasksByDueDate);
  const selectedItems = [
    ...menuBarEventItems(sortedEvents, 8),
    ...menuBarTaskItems(sortedTasks, "Due today", 6)
  ];

  return {
    monthLabel: new Intl.DateTimeFormat(undefined, {
      month: "long",
      year: "numeric"
    }).format(selectedDay),
    weekdayLabels: ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"],
    days: calendarGridDays(selectedDay).map((day) => ({
      key: localDateKey(day),
      label: String(day.getDate()),
      inCurrentMonth: day.getMonth() === selectedDay.getMonth(),
      isToday: isSameLocalDay(day, now),
      isSelected: isSameLocalDay(day, selectedDay)
    })),
    selectedLabel: new Intl.DateTimeFormat(undefined, {
      weekday: "long",
      day: "numeric",
      month: "short"
    }).format(selectedDay),
    selectedMeta: `${events.length} event${events.length === 1 ? "" : "s"} - ${tasks.length} task${
      tasks.length === 1 ? "" : "s"
    }`,
    selectedItems:
      selectedItems.length > 0 ? selectedItems : [{ label: "Nothing scheduled", detail: "today" }]
  };
}

function menuBarAccountSnapshot(
  account: GoogleAccountConnectionStatusDto | null
): NativeMenuBarSnapshot["account"] {
  if (!account || account.connectionState === "signed_out") {
    return undefined;
  }

  const email = account.email?.trim();
  const avatarUrl = account.avatarUrl?.trim();
  const displayName = account.displayName?.trim() || email || "Google account";

  return {
    displayName,
    ...(email ? { email } : {}),
    ...(avatarUrl ? { avatarUrl } : {}),
    connectionState: account.connectionState
  };
}

function calendarGridDays(date: Date): Date[] {
  const monthStart = new Date(date);
  monthStart.setDate(1);
  monthStart.setHours(0, 0, 0, 0);

  const gridStart = addDays(monthStart, -monthStart.getDay());
  return Array.from({ length: 42 }, (_, index) => addDays(gridStart, index));
}

function localDateKey(date: Date): string {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function isSameLocalDay(left: Date, right: Date): boolean {
  return (
    left.getFullYear() === right.getFullYear() &&
    left.getMonth() === right.getMonth() &&
    left.getDate() === right.getDate()
  );
}

function startOfLocalDay(date: Date): Date {
  const start = new Date(date);
  start.setHours(0, 0, 0, 0);
  return start;
}

function addDays(date: Date, days: number): Date {
  const copy = new Date(date);
  copy.setDate(copy.getDate() + days);
  return copy;
}

function taskDueBefore(task: TaskSummary, date: Date): boolean {
  const dueAt = task.dueAt ? dateFromIso(task.dueAt) : null;

  return Boolean(dueAt && dueAt < date);
}

function taskDueBetween(task: TaskSummary, start: Date, end: Date): boolean {
  const dueAt = task.dueAt ? dateFromIso(task.dueAt) : null;

  return Boolean(dueAt && dueAt >= start && dueAt < end);
}

function eventStartsBetween(event: CalendarEventSummary, start: Date, end: Date): boolean {
  const startsAt = dateFromIso(event.startsAt);

  return Boolean(startsAt && startsAt >= start && startsAt < end);
}

function compareTasksByDueDate(left: TaskSummary, right: TaskSummary): number {
  return isoTime(left.dueAt) - isoTime(right.dueAt) || left.title.localeCompare(right.title);
}

function compareEventsByStart(left: CalendarEventSummary, right: CalendarEventSummary): number {
  return isoTime(left.startsAt) - isoTime(right.startsAt) || left.title.localeCompare(right.title);
}

function isoTime(value: string | null | undefined): number {
  return value ? dateFromIso(value)?.getTime() ?? Number.MAX_SAFE_INTEGER : Number.MAX_SAFE_INTEGER;
}

function dueDetail(value: string, fallback: string): string {
  const dueAt = dateFromIso(value);

  if (!dueAt) {
    return fallback;
  }

  return isLocalMidnight(dueAt) ? fallback : `${fallback} ${formatShortTime(dueAt)}`;
}

function eventDetail(event: CalendarEventSummary): string {
  if (event.allDay) {
    return "All day";
  }

  const startsAt = dateFromIso(event.startsAt);
  const endsAt = dateFromIso(event.endsAt);

  if (!startsAt || !endsAt) {
    return "Scheduled";
  }

  return `${formatShortTime(startsAt)}-${formatShortTime(endsAt)}`;
}

function formatShortTime(date: Date): string {
  return new Intl.DateTimeFormat(undefined, {
    hour: "numeric",
    minute: "2-digit"
  }).format(date);
}

function isLocalMidnight(date: Date): boolean {
  return date.getHours() === 0 && date.getMinutes() === 0 && date.getSeconds() === 0;
}

function truncateMenuLabel(value: string, maxLength = 54): string {
  const trimmed = value.trim();

  if (trimmed.length <= maxLength) {
    return trimmed || "Untitled";
  }

  return `${trimmed.slice(0, maxLength - 1)}...`;
}

function dateFromIso(value: string): Date | null {
  const date = new Date(value);

  return Number.isFinite(date.getTime()) ? date : null;
}
