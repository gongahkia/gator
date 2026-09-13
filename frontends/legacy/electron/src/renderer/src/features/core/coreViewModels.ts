export type CorePriority = "none" | "low" | "medium" | "high";
export type TaskStatus = "open" | "completed" | "hidden" | "deleted";
export type TaskFilterId = "open" | "completed" | "hidden" | "deleted" | "empty" | "error";
export type CalendarViewId = "agenda" | "day" | "multiDay" | "week" | "month";
export type SearchSource = "task" | "event" | "note";
export type SettingsSectionId =
  | "google"
  | "resources"
  | "sync"
  | "appearance"
  | "hotkeys"
  | "tray"
  | "notifications"
  | "localData"
  | "mcp"
  | "platform"
  | "diagnostics";

export interface TaskSubtaskViewModel {
  id: string;
  title: string;
  completed: boolean;
}

export interface TaskViewModel {
  id: string;
  accountId?: string;
  listId: string;
  parentId: string | null;
  title: string;
  detail: string;
  list: string;
  dueDate: string | null;
  dueLabel: string;
  sortOrder?: number;
  updatedAt?: string;
  plannedStart?: string | null;
  plannedEnd?: string | null;
  durationMinutes?: number | null;
  lockedSchedule?: boolean;
  snoozeUntil?: string | null;
  tags?: string[];
  priority: CorePriority;
  status: TaskStatus;
  mutationState?: "synced" | "queued" | "failed";
  subtasks: TaskSubtaskViewModel[];
}

export interface TaskGroupViewModel {
  id: string;
  title: string;
  description: string;
  countLabel: string;
  tasks: TaskViewModel[];
}

export interface TaskFilterViewModel {
  id: TaskFilterId;
  label: string;
  countLabel: string;
  groups: TaskGroupViewModel[];
  state?: "ready" | "empty" | "error";
}

export interface CalendarEventViewModel {
  id: string;
  accountId?: string;
  eventId: string;
  hcbKind?: "birthday";
  status?: "confirmed" | "tentative" | "cancelled";
  sourceKind?: "event" | "task";
  taskId?: string;
  taskListId?: string;
  taskStatus?: TaskStatus;
  calendarId: string;
  colorId?: string | null;
  title: string;
  calendar: string;
  calendarBackgroundColor?: string | null;
  calendarForegroundColor?: string | null;
  displayBackgroundColor?: string | null;
  displayForegroundColor?: string | null;
  completedAt?: string | null;
  timeLabel: string;
  rangeLabel: string;
  startsAt: string;
  endsAt: string;
  timeZone: string;
  allDay: boolean;
  location: string;
  notes: string;
  guestEmails: string[];
  reminderMinutes: number[];
  attendees: Array<{
    email: string;
    displayName?: string;
    responseStatus?: "needsAction" | "declined" | "tentative" | "accepted";
    self?: boolean;
    resource?: boolean;
  }>;
  reminders: Array<{
    method: "popup" | "email";
    minutes: number;
  }>;
  remindersUseDefault: boolean;
  transparency: "opaque" | "transparent" | null;
  visibility: "default" | "public" | "private" | null;
  tags?: string[];
  conference: {
    solutionName?: string;
    videoUri?: string;
    videoLabel?: string;
    phoneUri?: string;
    phoneLabel?: string;
    phonePin?: string;
    moreUri?: string;
    moreLabel?: string;
  } | null;
  mutationState?: "synced" | "queued" | "failed";
  recurrenceRule: string | null;
  recurringEventId?: string | null;
  originalStartAt?: string | null;
}

export interface ScheduledTaskBlockViewModel {
  id: string;
  taskId: string;
  calendarEventId: string;
  calendarId: string;
  title: string;
  calendar: string;
  timeLabel: string;
  rangeLabel: string;
  startsAt: string;
  endsAt: string;
  durationMinutes: number;
  status: "scheduled" | "orphaned";
  mutationState?: "synced" | "queued" | "failed";
  conflictCount: number;
  conflictTitles: string[];
  isNextUp?: boolean;
}

export interface CalendarDayViewModel {
  id: string;
  weekday: string;
  dateLabel: string;
  isToday?: boolean;
  isOutsideMonth?: boolean;
  events: CalendarEventViewModel[];
}

export interface CalendarMonthWeekViewModel {
  id: string;
  days: CalendarDayViewModel[];
}

export interface NoteViewModel {
  id: string;
  listId: string;
  listTitle: string;
  title: string;
  body: string;
  preview: string;
  tags?: string[];
  updatedLabel: string;
}

export interface SearchResultViewModel {
  id: string;
  targetId: string;
  source: SearchSource;
  title: string;
  detail: string;
  snoozeUntil?: string | null;
  deepLinkLabel: string;
}

export interface SearchBucketViewModel {
  id: string;
  label: string;
  matchTerms: string[];
  results: SearchResultViewModel[];
}

export interface SearchViewModel {
  state: "idle" | "results" | "empty";
  summary: string;
  results: SearchResultViewModel[];
}

export interface SettingsSectionViewModel {
  id: SettingsSectionId;
  title: string;
  status: string;
  detail: string;
  rows: Array<{
    id: string;
    label: string;
    value: string;
  }>;
}
