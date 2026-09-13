import type {
  BootstrapGetResponse,
  CalendarListRequest,
  CalendarListResponse,
  CalendarRangeRequest,
  CalendarRangeResponse,
  NoteListRequest,
  NoteListResponse,
  NoteListSummary,
  NoteSummary,
  ScheduledTaskBlockListRequest,
  ScheduledTaskBlockListResponse,
  SettingsSnapshot,
  TagListRequest,
  TagListResponse,
  TagSummary,
  TaskListRequest,
  TaskListResponse,
  TaskSummary,
  TaskListsRequest,
  TaskListsResponse
} from "@shared/ipc/contracts";
import { visibleCalendarRange } from "./dateFormat";
import { sanitizeHydrationErrorMessage } from "./hydrationError";
import { unwrap } from "./result";
import { uniqueTasks } from "./taskViewModels";
import type { CoreDataSnapshot, CoreResourceCounts } from "./types";

type CursorRequest = { cursor?: string };
type PagedResponse<Item> = {
  items: Item[];
  page: {
    limit: number;
    nextCursor?: string;
    totalKnown?: number;
  };
};

function knownTotal(pageTotal: number | undefined, itemCount: number): number {
  return pageTotal ?? itemCount;
}

export async function loadAllPages<Request extends CursorRequest, Response extends PagedResponse<unknown>>(
  request: Request,
  loadPage: (request: Request) => Promise<Response>
): Promise<Response> {
  const items: unknown[] = [];
  let firstPage: Response | null = null;
  let lastPage: Response | null = null;
  let cursor = request.cursor;

  do {
    const page = await loadPage({
      ...request,
      ...(cursor === undefined ? {} : { cursor })
    });
    firstPage ??= page;
    lastPage = page;
    items.push(...page.items);
    cursor = page.page.nextCursor;
  } while (cursor !== undefined);

  if (firstPage === null || lastPage === null) {
    throw new Error("Paged request returned no data.");
  }

  const { nextCursor: _nextCursor, ...pageMetadata } = lastPage.page;

  return {
    ...lastPage,
    items,
    page: {
      ...pageMetadata,
      totalKnown: lastPage.page.totalKnown ?? firstPage.page.totalKnown ?? items.length
    }
  } as Response;
}

export async function loadCoreData(
  settingsPromise?: Promise<SettingsSnapshot>,
  calendarRange = visibleCalendarRange()
): Promise<CoreDataSnapshot> {
  if (!window.hcb) {
    throw new Error("Preload bridge is unavailable.");
  }

  const hcb = window.hcb;
  const bootstrapGet = (hcb as { bootstrap?: { get?: typeof hcb.bootstrap.get } }).bootstrap?.get;
  const bootstrapStartedAt = performance.now();
  let fallbackReason: "missing" | "failed" | "threw" | null = bootstrapGet
    ? null
    : "missing";
  const bootstrap = bootstrapGet
    ? await bootstrapGet({
        mode: "light",
        calendarRange: {
          start: calendarRange.start,
          end: calendarRange.end,
          limit: 500
        }
      }).catch(() => {
        fallbackReason = "threw";
        return null;
      })
    : null;
  const bootstrapDurationMs = performance.now() - bootstrapStartedAt;

  if (bootstrap?.ok) {
    return snapshotFromBootstrap(bootstrap.data, { tasks: true, notes: true });
  }

  fallbackReason ??= "failed";
  const fallbackStartedAt = performance.now();
  const settingsLoad =
    settingsPromise ?? hcb.settings.get().then((result) => unwrap(result, "Settings failed"));
  const [
    taskLists,
    tasks,
    hiddenTasks,
    deletedTasks,
    calendars,
    events,
    scheduledTaskBlocks,
    notes,
    tags,
    settings,
    syncStatus,
    googleStatus,
    undoStatus,
    native
  ] = await Promise.all([
    loadAllPages<TaskListsRequest, TaskListsResponse>(
      { limit: 100 },
      (request) => hcb.tasks.listTaskLists(request).then((result) => unwrap(result, "Task lists failed"))
    ),
    loadAllPages<TaskListRequest, TaskListResponse>(
      { status: "all", limit: 100 },
      (request) => hcb.tasks.list(request).then((result) => unwrap(result, "Tasks failed"))
    ),
    loadAllPages<TaskListRequest, TaskListResponse>(
      { status: "hidden", limit: 100 },
      (request) => hcb.tasks.list(request).then((result) => unwrap(result, "Hidden tasks failed"))
    ),
    loadAllPages<TaskListRequest, TaskListResponse>(
      { status: "deleted", limit: 100 },
      (request) => hcb.tasks.list(request).then((result) => unwrap(result, "Deleted tasks failed"))
    ),
    loadAllPages<CalendarListRequest, CalendarListResponse>(
      { limit: 100 },
      (request) => hcb.calendar.listCalendars(request).then((result) => unwrap(result, "Calendars failed"))
    ),
    loadAllPages<CalendarRangeRequest, CalendarRangeResponse>(
      { start: calendarRange.start, end: calendarRange.end, limit: 500 },
      (request) => hcb.calendar.listEvents(request).then((result) => unwrap(result, "Calendar events failed"))
    ),
    loadAllPages<ScheduledTaskBlockListRequest, ScheduledTaskBlockListResponse>(
      { start: calendarRange.start, end: calendarRange.end, limit: 500 },
      (request) =>
        hcb.calendar
          .listScheduledTaskBlocks(request)
          .then((result) => unwrap(result, "Scheduled task blocks failed"))
    ),
    loadAllPages<NoteListRequest, NoteListResponse>(
      { limit: 50 },
      (request) => hcb.notes.list(request).then((result) => unwrap(result, "Notes failed"))
    ),
    loadAllPages<TagListRequest, TagListResponse>(
      { limit: 100 },
      (request) => hcb.tags.list(request).then((result) => unwrap(result, "Tags failed"))
    ),
    settingsLoad,
    hcb.sync.status().then((result) => unwrap(result, "Sync status failed")),
    hcb.google.status().then((result) => unwrap(result, "Google status failed")),
    hcb.undo.status().then((result) => unwrap(result, "Undo status failed")),
    hcb.native.capabilities().then((result) => unwrap(result, "Native status failed"))
  ]);
  const snapshot = {
    taskLists: taskLists.items,
    tasks: uniqueTasks([...tasks.items, ...hiddenTasks.items, ...deletedTasks.items]),
    calendars: calendars.items,
    events: events.items,
    scheduledTaskBlocks: scheduledTaskBlocks.items,
    scheduleSuggestion: {
      slots: [],
      unscheduled: [],
      overloadMinutes: 0
    },
    notes: notes.items,
    noteLists: notes.lists,
    tags: tags.items,
    settings,
    syncStatus,
    googleStatus,
    undoStatus,
    native,
    resourceCounts: {
      calendarEvents: calendars.items.every((calendar) => calendar.eventCount !== undefined)
        ? calendars.items.reduce((count, calendar) => count + (calendar.eventCount ?? 0), 0)
        : knownTotal(events.page.totalKnown, events.items.length),
      notes: knownTotal(notes.page.totalKnown, notes.items.length),
      tasks: taskLists.items.every((taskList) => taskList.taskCount !== undefined)
        ? taskLists.items.reduce((count, taskList) => count + (taskList.taskCount ?? 0), 0)
        : knownTotal(tasks.page.totalKnown, tasks.items.length) +
          knownTotal(hiddenTasks.page.totalKnown, hiddenTasks.items.length) +
          knownTotal(deletedTasks.page.totalKnown, deletedTasks.items.length)
    }
  };

  recordRendererTiming({
    kind: "startup",
    name: "startup.bootstrap.fallback-fanout",
    durationMs: performance.now() - fallbackStartedAt,
    metadata: {
      reason: fallbackReason,
      bootstrapDurationMs: Math.max(0, Math.round(bootstrapDurationMs * 100) / 100),
      tasks: snapshot.resourceCounts.tasks,
      calendarEvents: snapshot.resourceCounts.calendarEvents,
      notes: snapshot.resourceCounts.notes
    }
  });

  return snapshot;
}

export interface CoreDataHydrationSnapshot {
  errorMessage?: string;
  failedResources: Array<"tasks" | "notes">;
  tasks?: TaskSummary[];
  notes?: NoteSummary[];
  noteLists?: NoteListSummary[];
  tags?: TagSummary[];
  resourceCounts: Partial<CoreResourceCounts>;
}

export async function hydrateCoreData(): Promise<CoreDataHydrationSnapshot> {
  if (!window.hcb) {
    throw new Error("Preload bridge is unavailable.");
  }

  const [tasks, notes] = await Promise.allSettled([
    loadTaskHydration(),
    loadNoteHydration()
  ]);

  const failedResources: Array<"tasks" | "notes"> = [];

  if (tasks.status === "rejected") {
    failedResources.push("tasks");
  }

  if (notes.status === "rejected") {
    failedResources.push("notes");
  }

  return {
    ...(failedResources.length > 0
      ? { errorMessage: hydrationErrorMessage(tasks, notes) }
      : {}),
    failedResources,
    ...(tasks.status === "fulfilled" ? { tasks: tasks.value.items } : {}),
    ...(notes.status === "fulfilled"
      ? { notes: notes.value.items, noteLists: notes.value.lists }
      : {}),
    resourceCounts: {
      ...(tasks.status === "fulfilled" ? { tasks: tasks.value.totalKnown } : {}),
      ...(notes.status === "fulfilled" ? { notes: notes.value.totalKnown } : {})
    }
  };
}

function hydrationErrorMessage(
  tasks: PromiseSettledResult<unknown>,
  notes: PromiseSettledResult<unknown>
): string {
  const reasons = [tasks, notes]
    .filter((result): result is PromiseRejectedResult => result.status === "rejected")
    .map((result) => result.reason)
    .map((reason) => reason instanceof Error ? reason.message : "Background hydration failed.");

  return sanitizeHydrationErrorMessage(reasons.join(" | ") || "Background hydration failed.");
}

async function loadTaskHydration(): Promise<{ items: TaskSummary[]; totalKnown: number }> {
  const startedAt = performance.now();

  try {
    const [tasks, hiddenTasks, deletedTasks] = await Promise.all([
      loadAllPages<TaskListRequest, TaskListResponse>(
        { status: "all", limit: 100 },
        (request) => window.hcb!.tasks.list(request).then((result) => unwrap(result, "Tasks failed"))
      ),
      loadAllPages<TaskListRequest, TaskListResponse>(
        { status: "hidden", limit: 100 },
        (request) => window.hcb!.tasks.list(request).then((result) => unwrap(result, "Hidden tasks failed"))
      ),
      loadAllPages<TaskListRequest, TaskListResponse>(
        { status: "deleted", limit: 100 },
        (request) => window.hcb!.tasks.list(request).then((result) => unwrap(result, "Deleted tasks failed"))
      )
    ]);
    const items = uniqueTasks([...tasks.items, ...hiddenTasks.items, ...deletedTasks.items]);
    const totalKnown =
      knownTotal(tasks.page.totalKnown, tasks.items.length) +
      knownTotal(hiddenTasks.page.totalKnown, hiddenTasks.items.length) +
      knownTotal(deletedTasks.page.totalKnown, deletedTasks.items.length);

    recordRendererTiming({
      kind: "startup",
      name: "startup.hydration.tasks",
      durationMs: performance.now() - startedAt,
      metadata: {
        outcome: "success",
        loadedTasks: items.length,
        tasks: totalKnown
      }
    });

    return { items, totalKnown };
  } catch (error) {
    recordRendererTiming({
      kind: "startup",
      name: "startup.hydration.tasks",
      durationMs: performance.now() - startedAt,
      metadata: {
        outcome: "failed"
      }
    });
    throw error;
  }
}

async function loadNoteHydration(): Promise<{
  items: NoteSummary[];
  lists: NoteListSummary[];
  totalKnown: number;
}> {
  const startedAt = performance.now();

  try {
    const notes = await loadAllPages<NoteListRequest, NoteListResponse>(
      { limit: 50 },
      (request) => window.hcb!.notes.list(request).then((result) => unwrap(result, "Notes failed"))
    );
    const totalKnown = knownTotal(notes.page.totalKnown, notes.items.length);

    recordRendererTiming({
      kind: "startup",
      name: "startup.hydration.notes",
      durationMs: performance.now() - startedAt,
      metadata: {
        outcome: "success",
        loadedNotes: notes.items.length,
        notes: totalKnown
      }
    });

    return { items: notes.items, lists: notes.lists, totalKnown };
  } catch (error) {
    recordRendererTiming({
      kind: "startup",
      name: "startup.hydration.notes",
      durationMs: performance.now() - startedAt,
      metadata: {
        outcome: "failed"
      }
    });
    throw error;
  }
}

function snapshotFromBootstrap(
  bootstrap: BootstrapGetResponse,
  deferredCounts: Partial<Record<keyof CoreResourceCounts, boolean>> = {}
): CoreDataSnapshot {
  return {
    taskLists: bootstrap.taskLists.items,
    tasks: uniqueTasks([
      ...bootstrap.tasks.items,
      ...bootstrap.hiddenTasks.items,
      ...bootstrap.deletedTasks.items
    ]),
    calendars: bootstrap.calendars.items,
    events: bootstrap.events.items,
    scheduledTaskBlocks: bootstrap.scheduledTaskBlocks.items,
    scheduleSuggestion: {
      slots: [],
      unscheduled: [],
      overloadMinutes: 0
    },
    notes: bootstrap.notes.items,
    noteLists: bootstrap.notes.lists,
    tags: bootstrap.tags.items,
    settings: bootstrap.settings,
    syncStatus: bootstrap.syncStatus,
    googleStatus: bootstrap.googleStatus,
    undoStatus: bootstrap.undoStatus,
    native: bootstrap.native,
    resourceCounts: {
      ...bootstrap.resourceCounts,
      ...(deferredCounts.tasks ? { tasks: null } : {}),
      ...(deferredCounts.notes ? { notes: null } : {})
    }
  };
}

function recordRendererTiming(request: {
  kind: "startup" | "cached_render" | "ipc" | "sqlite_query" | "search";
  name: string;
  durationMs: number;
  metadata?: Record<string, string | number | boolean | null>;
}): void {
  void window.hcb?.diagnostics.recordTiming(request);
}
