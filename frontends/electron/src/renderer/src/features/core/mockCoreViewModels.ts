import type {
  CalendarDayViewModel,
  CalendarEventViewModel,
  CalendarMonthWeekViewModel,
  NoteViewModel,
  SearchBucketViewModel,
  SearchViewModel,
  SettingsSectionViewModel,
  TaskFilterId,
  TaskFilterViewModel,
  TaskViewModel
} from "./coreViewModels";

export type {
  CalendarDayViewModel,
  CalendarEventViewModel,
  CalendarMonthWeekViewModel,
  CorePriority,
  NoteViewModel,
  SearchBucketViewModel,
  SearchSource,
  SearchViewModel,
  SettingsSectionId,
  SettingsSectionViewModel,
  TaskFilterId,
  TaskFilterViewModel,
  TaskGroupViewModel,
  TaskStatus,
  TaskSubtaskViewModel,
  TaskViewModel
} from "./coreViewModels";

const inboxTasks: TaskViewModel[] = [
  {
    id: "task-inbox-rules",
    listId: "list-inbox",
    parentId: null,
    title: "Draft inbox triage rules",
    detail: "Define keyboard-first review states before sync writes exist.",
    list: "Inbox",
    dueDate: "2026-05-22",
    dueLabel: "Today",
    priority: "high",
    status: "open",
    subtasks: [
      { id: "subtask-inbox-shortcuts", title: "Map shortcut states", completed: true },
      { id: "subtask-inbox-empty", title: "Name empty views", completed: false }
    ]
  },
  {
    id: "task-calendar-fixtures",
    listId: "list-inbox",
    parentId: null,
    title: "Review calendar fixture shape",
    detail: "Keep visible rows stable for future agenda virtualization.",
    list: "Inbox",
    dueDate: "2026-05-22",
    dueLabel: "Today",
    priority: "medium",
    status: "open",
    subtasks: [
      { id: "subtask-calendar-agenda", title: "Agenda rows", completed: true },
      { id: "subtask-calendar-month", title: "Month grid shell", completed: false }
    ]
  }
];

const planningTasks: TaskViewModel[] = [
  {
    id: "task-offline-copy",
    listId: "list-planning",
    parentId: null,
    title: "Tighten offline banner copy",
    detail: "Make retry and planner state explicit without exposing service details.",
    list: "Planning",
    dueDate: "2026-05-23",
    dueLabel: "Tomorrow",
    priority: "low",
    status: "open",
    subtasks: [
      { id: "subtask-offline-retry", title: "Retry copy", completed: false },
      { id: "subtask-offline-redaction", title: "Redaction note", completed: true }
    ]
  },
  {
    id: "task-settings-states",
    listId: "list-planning",
    parentId: null,
    title: "Map settings empty and error states",
    detail: "Prepare rows for OAuth, hotkeys, diagnostics, MCP, and planner data.",
    list: "Planning",
    dueDate: "2026-05-29",
    dueLabel: "Friday",
    priority: "none",
    status: "open",
    subtasks: [
      { id: "subtask-settings-sections", title: "Required sections", completed: true },
      { id: "subtask-settings-conflict", title: "Shortcut conflict", completed: false }
    ]
  }
];

const completedTasks: TaskViewModel[] = [
  {
    id: "task-shell-visible",
    listId: "list-performance",
    parentId: null,
    title: "Report shell-visible timing",
    detail: "Mock-only diagnostics call is already available through preload.",
    list: "Performance",
    dueDate: null,
    dueLabel: "Done",
    priority: "low",
    status: "completed",
    subtasks: [
      { id: "subtask-shell-mark", title: "Mark first paint", completed: true },
      { id: "subtask-shell-health", title: "Health status", completed: true }
    ]
  },
  {
    id: "task-command-palette",
    listId: "list-performance",
    parentId: null,
    title: "Keep command palette cheap to mount",
    detail: "Command registry stays in memory and does not wait for data hydration.",
    list: "Performance",
    dueDate: null,
    dueLabel: "Done",
    priority: "medium",
    status: "completed",
    subtasks: [
      { id: "subtask-command-filter", title: "Filter commands", completed: true },
      { id: "subtask-command-navigate", title: "Navigate sections", completed: true }
    ]
  }
];

const hiddenTasks: TaskViewModel[] = [
  {
    id: "task-hidden-legacy-import",
    listId: "list-backlog",
    parentId: null,
    title: "Legacy import comparison",
    detail: "Hidden until planner migrations exist.",
    list: "Backlog",
    dueDate: null,
    dueLabel: "Hidden",
    priority: "none",
    status: "hidden",
    subtasks: [{ id: "subtask-hidden-scope", title: "Keep reference-only", completed: false }]
  }
];

const deletedTasks: TaskViewModel[] = [
  {
    id: "task-deleted-stale-demo",
    listId: "list-backlog",
    parentId: null,
    title: "Remove stale demo row",
    detail: "Deleted task shell for trash filters and recovery copy.",
    list: "Backlog",
    dueDate: null,
    dueLabel: "Deleted",
    priority: "low",
    status: "deleted",
    subtasks: [{ id: "subtask-deleted-audit", title: "Audit mutation", completed: true }]
  }
];

export const largeTaskWindow: TaskViewModel[] = Array.from({ length: 96 }, (_, index) => ({
  id: `task-generated-${index + 1}`,
  listId: index % 2 === 0 ? "list-inbox" : "list-planning",
  parentId: null,
  title: `Generated planning task ${String(index + 1).padStart(2, "0")}`,
  detail: "Virtualized placeholder row for large planner data testing.",
  list: index % 2 === 0 ? "Inbox" : "Planning",
  dueDate: index % 3 === 0 ? "2026-05-22" : null,
  dueLabel: index % 3 === 0 ? "This week" : "Later",
  priority: index % 5 === 0 ? "high" : index % 3 === 0 ? "medium" : "none",
  status: "open",
  subtasks: []
}));

export const taskFilterViewModels: TaskFilterViewModel[] = [
  {
    id: "open",
    label: "Open",
    countLabel: "4",
    groups: [
      {
        id: "inbox-open",
        title: "Inbox",
        description: "Precomputed open tasks due today",
        countLabel: "2 tasks",
        tasks: inboxTasks
      },
      {
        id: "planning-open",
        title: "Planning",
        description: "Precomputed open planning tasks",
        countLabel: "2 tasks",
        tasks: planningTasks
      }
    ]
  },
  {
    id: "completed",
    label: "Completed",
    countLabel: "2",
    groups: [
      {
        id: "completed-history",
        title: "Completed history",
        description: "Finished rows from the mock data",
        countLabel: "2 tasks",
        tasks: completedTasks
      }
    ]
  },
  {
    id: "hidden",
    label: "Hidden",
    countLabel: "1",
    groups: [
      {
        id: "hidden",
        title: "Hidden",
        description: "Rows excluded from the default planner",
        countLabel: "1 task",
        tasks: hiddenTasks
      }
    ]
  },
  {
    id: "deleted",
    label: "Deleted",
    countLabel: "1",
    groups: [
      {
        id: "deleted",
        title: "Deleted",
        description: "Trash shell for recover or purge flows",
        countLabel: "1 task",
        tasks: deletedTasks
      }
    ]
  },
  {
    id: "empty",
    label: "Empty",
    countLabel: "0",
    groups: [],
    state: "empty"
  },
  {
    id: "error",
    label: "Error",
    countLabel: "!",
    groups: [],
    state: "error"
  }
];

function mockCalendarEvent({
  allDay = false,
  calendar,
  calendarId,
  endsAt,
  id,
  location,
  notes,
    rangeLabel,
    startsAt,
    timeZone = "UTC",
    timeLabel,
    title
}: {
  allDay?: boolean;
  calendar: string;
  calendarId: string;
  endsAt: string;
  id: string;
  location: string;
  notes: string;
  rangeLabel: string;
  startsAt: string;
  timeZone?: string;
  timeLabel: string;
  title: string;
}): CalendarEventViewModel {
  return {
    id,
    eventId: id,
    calendarId,
    title,
    calendar,
    timeLabel,
    rangeLabel,
    startsAt,
    endsAt,
    timeZone,
    allDay,
    location,
    notes,
    guestEmails: [],
    reminderMinutes: [],
    attendees: [],
    reminders: [],
    remindersUseDefault: false,
    transparency: null,
    visibility: null,
    conference: null,
    recurrenceRule: null
  };
}

export const calendarEventsById: Record<string, CalendarEventViewModel> = {
  "event-standup": mockCalendarEvent({
    id: "event-standup",
    title: "Planner shell standup",
    calendarId: "cal-product",
    calendar: "Product",
    timeLabel: "09:30",
    rangeLabel: "09:30-09:50",
    startsAt: "2026-05-22T09:30:00.000Z",
    endsAt: "2026-05-22T09:50:00.000Z",
    location: "Local mock room",
    notes: "Review Today and Tasks shape."
  }),
  "event-focus": mockCalendarEvent({
    id: "event-focus",
    title: "Focused implementation block",
    calendarId: "cal-engineering",
    calendar: "Engineering",
    timeLabel: "11:00",
    rangeLabel: "11:00-13:00",
    startsAt: "2026-05-22T11:00:00.000Z",
    endsAt: "2026-05-22T13:00:00.000Z",
    location: "Desk",
    notes: "Renderer-only feature work."
  }),
  "event-sync-review": mockCalendarEvent({
    id: "event-sync-review",
    title: "Sync contract review",
    calendarId: "cal-engineering",
    calendar: "Engineering",
    timeLabel: "14:00",
    rangeLabel: "14:00-14:45",
    startsAt: "2026-05-22T14:00:00.000Z",
    endsAt: "2026-05-22T14:45:00.000Z",
    location: "Video",
    notes: "Confirm missing preload contracts."
  }),
  "event-review": mockCalendarEvent({
    id: "event-review",
    title: "Renderer acceptance review",
    calendarId: "cal-qa",
    calendar: "QA",
    timeLabel: "15:30",
    rangeLabel: "15:30-16:15",
    startsAt: "2026-05-22T15:30:00.000Z",
    endsAt: "2026-05-22T16:15:00.000Z",
    location: "Codex",
    notes: "Check screen coverage and tests."
  }),
  "event-notes": mockCalendarEvent({
    id: "event-notes",
    title: "Notes local state pass",
    calendarId: "cal-product",
    calendar: "Product",
    timeLabel: "17:00",
    rangeLabel: "17:00-17:25",
    startsAt: "2026-05-22T17:00:00.000Z",
    endsAt: "2026-05-22T17:25:00.000Z",
    location: "Local mock room",
    notes: "Exercise create, edit, delete state."
  })
};

export const calendarAgendaEvents: CalendarEventViewModel[] = [
  calendarEventsById["event-standup"],
  calendarEventsById["event-focus"],
  calendarEventsById["event-sync-review"],
  calendarEventsById["event-review"],
  calendarEventsById["event-notes"],
  ...Array.from({ length: 42 }, (_, index) => {
    const hour = String(8 + (index % 10)).padStart(2, "0");
    const calendar = index % 2 === 0 ? "Product" : "Engineering";

    return mockCalendarEvent({
      id: `event-generated-${index + 1}`,
      title: `Generated agenda event ${String(index + 1).padStart(2, "0")}`,
      calendarId: index % 2 === 0 ? "cal-product" : "cal-engineering",
      calendar,
      timeLabel: `${hour}:15`,
      rangeLabel: `${hour}:15-${hour}:45`,
      startsAt: `2026-05-22T${hour}:15:00.000Z`,
      endsAt: `2026-05-22T${hour}:45:00.000Z`,
      location: "Mock calendar",
      notes: "Virtualized agenda placeholder."
    });
  })
];

export const calendarDayView: CalendarDayViewModel = {
  id: "day-2026-05-22",
  weekday: "Friday",
  dateLabel: "May 22",
  isToday: true,
  events: [
    calendarEventsById["event-standup"],
    calendarEventsById["event-focus"],
    calendarEventsById["event-sync-review"],
    calendarEventsById["event-review"],
    calendarEventsById["event-notes"]
  ]
};

export const calendarWeekDays: CalendarDayViewModel[] = [
  {
    id: "week-mon",
    weekday: "Mon",
    dateLabel: "18",
    events: [calendarEventsById["event-standup"]]
  },
  {
    id: "week-tue",
    weekday: "Tue",
    dateLabel: "19",
    events: []
  },
  {
    id: "week-wed",
    weekday: "Wed",
    dateLabel: "20",
    events: [calendarEventsById["event-focus"]]
  },
  {
    id: "week-thu",
    weekday: "Thu",
    dateLabel: "21",
    events: [calendarEventsById["event-sync-review"]]
  },
  {
    id: "week-fri",
    weekday: "Fri",
    dateLabel: "22",
    isToday: true,
    events: [calendarEventsById["event-review"], calendarEventsById["event-notes"]]
  },
  {
    id: "week-sat",
    weekday: "Sat",
    dateLabel: "23",
    events: []
  },
  {
    id: "week-sun",
    weekday: "Sun",
    dateLabel: "24",
    events: []
  }
];

export const calendarMonthWeeks: CalendarMonthWeekViewModel[] = [
  {
    id: "month-week-1",
    days: [
      { id: "month-apr-27", weekday: "Mon", dateLabel: "27", isOutsideMonth: true, events: [] },
      { id: "month-apr-28", weekday: "Tue", dateLabel: "28", isOutsideMonth: true, events: [] },
      { id: "month-apr-29", weekday: "Wed", dateLabel: "29", isOutsideMonth: true, events: [] },
      { id: "month-apr-30", weekday: "Thu", dateLabel: "30", isOutsideMonth: true, events: [] },
      { id: "month-may-1", weekday: "Fri", dateLabel: "1", events: [] },
      { id: "month-may-2", weekday: "Sat", dateLabel: "2", events: [] },
      { id: "month-may-3", weekday: "Sun", dateLabel: "3", events: [] }
    ]
  },
  {
    id: "month-week-2",
    days: [
      { id: "month-may-4", weekday: "Mon", dateLabel: "4", events: [] },
      { id: "month-may-5", weekday: "Tue", dateLabel: "5", events: [] },
      { id: "month-may-6", weekday: "Wed", dateLabel: "6", events: [] },
      { id: "month-may-7", weekday: "Thu", dateLabel: "7", events: [] },
      { id: "month-may-8", weekday: "Fri", dateLabel: "8", events: [] },
      { id: "month-may-9", weekday: "Sat", dateLabel: "9", events: [] },
      { id: "month-may-10", weekday: "Sun", dateLabel: "10", events: [] }
    ]
  },
  {
    id: "month-week-3",
    days: [
      { id: "month-may-11", weekday: "Mon", dateLabel: "11", events: [] },
      { id: "month-may-12", weekday: "Tue", dateLabel: "12", events: [] },
      { id: "month-may-13", weekday: "Wed", dateLabel: "13", events: [] },
      { id: "month-may-14", weekday: "Thu", dateLabel: "14", events: [] },
      { id: "month-may-15", weekday: "Fri", dateLabel: "15", events: [] },
      { id: "month-may-16", weekday: "Sat", dateLabel: "16", events: [] },
      { id: "month-may-17", weekday: "Sun", dateLabel: "17", events: [] }
    ]
  },
  {
    id: "month-week-4",
    days: [
      { id: "month-may-18", weekday: "Mon", dateLabel: "18", events: [calendarEventsById["event-standup"]] },
      { id: "month-may-19", weekday: "Tue", dateLabel: "19", events: [] },
      { id: "month-may-20", weekday: "Wed", dateLabel: "20", events: [calendarEventsById["event-focus"]] },
      { id: "month-may-21", weekday: "Thu", dateLabel: "21", events: [calendarEventsById["event-sync-review"]] },
      {
        id: "month-may-22",
        weekday: "Fri",
        dateLabel: "22",
        isToday: true,
        events: [calendarEventsById["event-review"], calendarEventsById["event-notes"]]
      },
      { id: "month-may-23", weekday: "Sat", dateLabel: "23", events: [] },
      { id: "month-may-24", weekday: "Sun", dateLabel: "24", events: [] }
    ]
  },
  {
    id: "month-week-5",
    days: [
      { id: "month-may-25", weekday: "Mon", dateLabel: "25", events: [] },
      { id: "month-may-26", weekday: "Tue", dateLabel: "26", events: [] },
      { id: "month-may-27", weekday: "Wed", dateLabel: "27", events: [] },
      { id: "month-may-28", weekday: "Thu", dateLabel: "28", events: [] },
      { id: "month-may-29", weekday: "Fri", dateLabel: "29", events: [] },
      { id: "month-may-30", weekday: "Sat", dateLabel: "30", events: [] },
      { id: "month-may-31", weekday: "Sun", dateLabel: "31", events: [] }
    ]
  }
];

export const initialNotes: NoteViewModel[] = [
  {
    id: "note-cache-first",
    listId: "list-inbox",
    listTitle: "Notes",
    title: "Startup data flow",
    body: "Renderer should paint a useful shell before Google, SQLite, or MCP work is wired.",
    preview: "Renderer should paint a useful shell before Google, SQLite, or MCP work is wired.",
    updatedLabel: "Updated 8m ago"
  },
  {
    id: "note-command-surface",
    listId: "list-inbox",
    listTitle: "Notes",
    title: "Command palette surface",
    body: "Commands stay in memory and execute the same future services as visible controls.",
    preview: "Commands stay in memory and execute the same future services as visible controls.",
    updatedLabel: "Updated 21m ago"
  },
  {
    id: "note-density",
    listId: "list-inbox",
    listTitle: "Notes",
    title: "Compact density checks",
    body: "Use 13px body text, stable toolbar controls, and visible focus rings.",
    preview: "Use 13px body text, stable toolbar controls, and visible focus rings.",
    updatedLabel: "Updated yesterday"
  }
];

export const searchBuckets: SearchBucketViewModel[] = [
  {
    id: "tasks",
    label: "Task matches",
    matchTerms: ["task", "tasks", "inbox", "triage", "calendar fixture", "offline", "settings"],
    results: [
      {
        id: "search-task-inbox-rules",
        targetId: "task-inbox-rules",
        source: "task",
        title: "Draft inbox triage rules",
        detail: "Inbox task due today with two subtasks.",
        deepLinkLabel: "Tasks / Inbox"
      },
      {
        id: "search-task-calendar-fixtures",
        targetId: "task-calendar-fixtures",
        source: "task",
        title: "Review calendar fixture shape",
        detail: "Planning task for stable agenda virtualization rows.",
        deepLinkLabel: "Tasks / Planning"
      }
    ]
  },
  {
    id: "events",
    label: "Event matches",
    matchTerms: ["event", "events", "agenda", "calendar", "review", "standup", "sync"],
    results: [
      {
        id: "search-event-review",
        targetId: "event-review",
        source: "event",
        title: "Renderer acceptance review",
        detail: "QA calendar event at 15:30.",
        deepLinkLabel: "Calendar / Agenda"
      },
      {
        id: "search-event-sync",
        targetId: "event-sync-review",
        source: "event",
        title: "Sync contract review",
        detail: "Engineering calendar event at 14:00.",
        deepLinkLabel: "Calendar / Day"
      }
    ]
  },
  {
    id: "notes",
    label: "Note matches",
    matchTerms: ["note", "notes", "local", "cache", "command", "density"],
    results: [
      {
        id: "search-note-command",
        targetId: "note-command-surface",
        source: "note",
        title: "Command palette surface",
        detail: "Note updated 21m ago.",
        deepLinkLabel: "Notes"
      },
      {
        id: "search-note-cache",
        targetId: "note-cache-first",
        source: "note",
        title: "Startup data flow",
        detail: "Note about shell visibility.",
        deepLinkLabel: "Notes"
      }
    ]
  }
];

export const settingsSections: SettingsSectionViewModel[] = [
  {
    id: "google",
    title: "Google",
    status: "Disconnected",
    detail: "OAuth setup shell only. The renderer never receives tokens or client secrets.",
    rows: [
      { id: "account", label: "Account", value: "Not connected" },
      { id: "scopes", label: "Scopes", value: "Tasks and Calendar pending" }
    ]
  },
  {
    id: "sync",
    title: "Sync",
    status: "Mock only",
    detail: "Background refresh and full-resync controls.",
    rows: [
      { id: "mode", label: "Mode", value: "Background sync" },
      { id: "queue", label: "Mutation queue", value: "Idle" }
    ]
  },
  {
    id: "appearance",
    title: "Appearance",
    status: "System",
    detail: "Theme and density controls backed by semantic renderer tokens.",
    rows: [
      { id: "theme", label: "Theme", value: "System" },
      { id: "density", label: "Density", value: "Compact" }
    ]
  },
  {
    id: "tray",
    title: "Tray",
    status: "Enabled shell",
    detail: "Menu bar visibility and quick actions will be handled by native adapters.",
    rows: [
      { id: "show", label: "Show menu bar icon", value: "On" },
      { id: "click", label: "Primary click", value: "Show or hide window" }
    ]
  },
  {
    id: "notifications",
    title: "Notifications",
    status: "Not requested",
    detail: "Local notification scheduling and permission status shell.",
    rows: [
      { id: "permission", label: "Permission", value: "Not requested" },
      { id: "schedule", label: "Task reminders", value: "Local only" }
    ]
  },
  {
    id: "mcp",
    title: "MCP",
    status: "Off",
    detail: "Local agent access is opt-in and must stay on 127.0.0.1.",
    rows: [
      { id: "server", label: "Server", value: "Off" },
      { id: "writes", label: "Writes", value: "Confirm before apply" }
    ]
  },
  {
    id: "diagnostics",
    title: "Diagnostics",
    status: "Ready",
    detail: "Copyable summaries are sanitized before leaving app services.",
    rows: [
      { id: "redaction", label: "Redaction", value: "Enabled" },
      { id: "profiling", label: "Renderer profiling", value: "Off by default" }
    ]
  }
];

export function getTaskFilterViewModel(filterId: TaskFilterId): TaskFilterViewModel {
  return taskFilterViewModels.find((filter) => filter.id === filterId) ?? taskFilterViewModels[0];
}

export function getPrecomputedSearchViewModel(query: string): SearchViewModel {
  const normalizedQuery = query.trim().toLowerCase();

  if (!normalizedQuery) {
    return {
      state: "idle",
      summary: "Search waits for a local query",
      results: []
    };
  }

  const bucket = searchBuckets.find((candidate) =>
    candidate.matchTerms.some((term) => term.includes(normalizedQuery) || normalizedQuery.includes(term))
  );

  if (!bucket) {
    return {
      state: "empty",
      summary: "No capped mock results",
      results: []
    };
  }

  return {
    state: "results",
    summary: `${bucket.results.length} ${bucket.label.toLowerCase()}`,
    results: bucket.results
  };
}

export function getTaskById(taskId: string): TaskViewModel {
  const allTasks = [
    ...inboxTasks,
    ...planningTasks,
    ...completedTasks,
    ...hiddenTasks,
    ...deletedTasks,
    ...largeTaskWindow
  ];

  return allTasks.find((task) => task.id === taskId) ?? inboxTasks[0];
}
