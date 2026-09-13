import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, vi } from "vitest";
import type {
  GoogleStatusResponse,
  NativeCapabilitiesResponse,
  SearchResultItem,
  SettingsSnapshot,
  TaskDetail
} from "@shared/ipc/contracts";
import {
  defaultHistoryCategoryVisibility,
  defaultKeybindings,
  defaultLeaderKey,
  defaultLeaderKeybindings,
  defaultSemanticSearchModels
} from "@shared/ipc/contracts";
import type { HcbApi } from "@shared/ipc/preloadApi";
import { ok } from "@shared/ipc/result";

export const originalHcb = window.hcb;
export const todayDate = new Date().toISOString().slice(0, 10);
export const now = `${todayDate}T00:00:00.000Z`;
const tomorrow = new Date(now);
tomorrow.setUTCDate(tomorrow.getUTCDate() + 1);
export const tomorrowIso = tomorrow.toISOString();

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  document.documentElement.removeAttribute("data-theme");
  document.documentElement.removeAttribute("data-color-theme");
  document.documentElement.removeAttribute("style");
  Object.defineProperty(window, "hcb", {
    configurable: true,
    value: originalHcb
  });
});

export function primaryNavigation(): HTMLElement {
  return screen.getByRole("navigation", { name: "Primary" });
}

export async function runPaletteCommand(user: ReturnType<typeof userEvent.setup>, query: string, label: RegExp): Promise<void> {
  await user.keyboard("{Meta>}p{/Meta}");
  const dialog = await screen.findByRole("dialog", { name: "Command palette" });
  const input = within(dialog).getByRole("searchbox", { name: "Filter commands" });

  await user.type(input, query);
  await user.click(await within(dialog).findByRole("option", { name: label }));
}

export async function goToSection(label: string): Promise<void> {
  const user = userEvent.setup();

  if (label === "Settings") {
    await user.click(screen.getByRole("button", { name: "Settings" }));
    await screen.findByRole("dialog", { name: "Settings" });
    return;
  }

  if (label === "Today") {
    await runPaletteCommand(user, "calendar agenda", /Calendar agenda view/);
    return;
  }

  await user.click(within(primaryNavigation()).getByRole("button", { name: label }));
}

export function testDataTransfer(): DataTransfer {
  const data = new Map<string, string>();
  const transfer = {
    dropEffect: "none",
    effectAllowed: "all",
    files: [],
    items: [],
    types: [] as string[],
    clearData: vi.fn((format?: string) => {
      if (format) {
        data.delete(format);
        transfer.types = transfer.types.filter((type) => type !== format);
        return;
      }

      data.clear();
      transfer.types = [];
    }),
    getData: vi.fn((format: string) => data.get(format) ?? ""),
    setData: vi.fn((format: string, value: string) => {
      data.set(format, value);

      if (!transfer.types.includes(format)) {
        transfer.types.push(format);
      }
    }),
    setDragImage: vi.fn()
  };

  return transfer as unknown as DataTransfer;
}

export function utcWeekStartDate(value: string): string {
  const date = new Date(value);
  const start = new Date(Date.UTC(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate()));

  start.setUTCDate(start.getUTCDate() - start.getUTCDay());
  return start.toISOString().slice(0, 10);
}

export function installHcb(api: HcbApi | undefined): void {
  Object.defineProperty(window, "hcb", {
    configurable: true,
    value: api
  });
}

export function testSettings(overrides: Partial<SettingsSnapshot> = {}): SettingsSnapshot {
  return {
    theme: "system",
    colorTheme: "notion",
    customBackground: null,
    useInferredBackgroundTheme: true,
    appLanguage: "system",
    uiFontName: null,
    uiTextSizePoints: 13,
    perSurfaceFontOverrides: {},
    calendarEventColorOverrides: {},
    autoTagRules: [],
    autoTagBackgroundReapplyMode: "preview",
    disableAnimations: false,
    uiLayoutScale: 1,
    navigationPlacement: "left",
    hiddenNavigationTabs: [],
    navigationTabOrder: ["calendar", "tasks", "notes"],
    toolbarActionOrder: ["commandPalette", "notifications", "diagnostics", "splitPane", "refresh", "settings"],
    hiddenCalendarViewModes: [],
    showCompletedInCalendarViews: true,
    eventCompletionDefaultScope: "occurrence",
    calendarTimelineDensity: "compact",
    monthScrollPastMonths: 0,
    monthScrollFutureMonths: 1,
    quickCreateExpandedByDefault: false,
    restoreWindowStateEnabled: true,
    startOnLogin: false,
    selectedTaskListIds: [],
    selectedCalendarIds: [],
    setupCompletedAt: now,
    syncMode: "balanced",
    syncTasksEnabled: true,
    syncCalendarEventsEnabled: true,
    eventRetentionDaysBack: 0,
    completedTaskRetentionDaysBack: 365,
    keybindings: defaultKeybindings,
    leaderKey: defaultLeaderKey,
    leaderKeybindings: defaultLeaderKeybindings,
    showTrayIcon: true,
    trayClickAction: "open-menu",
    menuBarPanelStyle: "adaptive",
    menuBarIconName: "calendar",
    menuBarCalendarIconId: "calendar",
    menuBarCalendarDoneMode: "visibleTodayDone",
    customMenuBarIcons: [],
    showMenuBarBadge: true,
    showDockBadge: true,
    notificationsEnabled: false,
    notificationLeadMinutes: 10,
    taskCompletionSoundEnabled: true,
    taskCompletionSoundId: "glass",
    eventCompletionSoundEnabled: true,
    eventCompletionSoundId: "pop",
    importedSoundCount: 0,
    perTabListFilters: {
      tasks: { useCustomFilter: false, selectedTaskListIds: [] },
      notes: { useCustomFilter: false, selectedTaskListIds: [] }
    },
    portableExportOnlySelectedTaskLists: false,
    portableExportOnlySelectedCalendars: false,
    portableExportOnlyFutureCurrentEvents: false,
    dailyLocalBackupEnabled: false,
    localBackupRetentionCount: 14,
    lastLocalBackupAt: null,
    visibleHistoryEntryCount: 50,
    historyStorageCap: 5_000,
    historyCategoryVisibility: defaultHistoryCategoryVisibility,
    dismissedDuplicateGroupIds: [],
    taskTemplates: [],
    eventTemplates: [],
    noteTemplates: [],
    lastUpdateCheckAt: null,
    storageBackend: "google",
    hcbHosterEndpoint: null,
    hcbVaultPath: null,
    hcbHosterLastPackageSha256: null,
    mcpEnabled: false,
    mcpPermissionMode: "confirm-writes",
    mcpPort: 0,
    localHostersEnabled: false,
    localHosterPort: 0,
    defaultTimeZone: "UTC",
    todayCapacityMinutes: 480,
    todayWorkingHoursStart: 6,
    todayWorkingHoursEnd: 22,
    diagnosticsIncludePerformance: true,
    rawGoogleDiagnosticsEnabled: false,
    savedSearchViews: [],
    pinnedSavedSearchViewIds: [],
    savedTaskViews: [],
    semanticSearchEnabled: false,
    semanticSearchMode: "lexical",
    embeddingModelId: "hcb-local-hash-384",
    semanticSearchModels: defaultSemanticSearchModels,
    agentActionTrayEnabled: true,
    webhooksEnabled: false,
    ...overrides
  };
}

export function testNativeCapabilities(
  overrides: Partial<NativeCapabilitiesResponse> = {}
): NativeCapabilitiesResponse {
  return {
    platform: "darwin",
    notifications: true,
    globalShortcuts: true,
    tray: true,
    deepLinks: true,
    trayStatus: {
      state: "ready",
      message: "Menu bar item is ready."
    },
    notificationsStatus: {
      permission: "prompt",
      scheduledCount: 0,
      state: "disabled",
      message: "Local notifications are disabled in Settings."
    },
    deepLinkStatus: {
      scheme: "hotcrossbuns",
      registered: true,
      state: "ready",
      message: "Protocol handler is registered."
    },
    updaterStatus: {
      state: "unsupported",
      message: "Preview update checks are not configured."
    },
    mcpStatus: {
      state: "disabled",
      message: "MCP local agent access is disabled."
    },
    capabilityReport: {
      platform: "darwin",
      adapterId: "test",
      packageFormat: "development",
      flags: {
        supportsAppPaths: true,
        supportsTray: true,
        supportsAppMenu: true,
        supportsGlobalShortcut: true,
        supportsNotifications: true,
        supportsNotificationPermissionQuery: false,
        supportsProtocolRegistration: true,
        supportsProtocolRegistrationCheck: true,
        supportsAutostart: true,
        supportsInPlaceAutoUpdate: false,
        supportsInstallerMetadata: true,
        supportsExternalUrlOpen: true,
        supportsDiagnosticsCollection: true,
        supportsCredentialStorage: false,
        supportsOAuthLoopback: true,
        supportsMcpLoopback: true,
        requiresSignedBuildForNotifications: false
      },
      paths: [],
      capabilities: [],
      diagnostics: []
    },
    deferredStartup: {
      state: "complete"
    },
    ...overrides
  };
}

export function connectedGoogleStatus(overrides: Partial<GoogleStatusResponse> = {}): GoogleStatusResponse {
  return {
    oauthClientConfigured: true,
    clientId: "desktop-client-id.apps.googleusercontent.com",
    hasClientSecret: false,
    account: {
      accountId: "google:test-account",
      googleAccountId: "test-account",
      email: "planner@example.com",
      displayName: "Planner Test",
      connectionState: "connected",
      grantedScopes: [
        "https://www.googleapis.com/auth/tasks",
        "https://www.googleapis.com/auth/calendar"
      ],
      missingScopes: [],
      lastAuthenticatedAt: now,
      updatedAt: now
    },
    accounts: [
      {
        accountId: "google:test-account",
        googleAccountId: "test-account",
        email: "planner@example.com",
        displayName: "Planner Test",
        connectionState: "connected",
        grantedScopes: [
          "https://www.googleapis.com/auth/tasks",
          "https://www.googleapis.com/auth/calendar"
        ],
        missingScopes: [],
        lastAuthenticatedAt: now,
        updatedAt: now
      }
    ],
    ...overrides
  };
}

function unwrapFixture<T>(result: { ok: true; data: T } | { ok: false }): T {
  if (!result.ok) {
    throw new Error("Fixture preload call failed.");
  }

  return result.data;
}

type FixturePageRequest = Record<string, unknown> & { cursor?: string };
type FixturePagedResponse<Item> = {
  items: Item[];
  page: {
    limit: number;
    nextCursor?: string;
    totalKnown?: number;
  };
};

async function loadAllFixturePages<
  Item,
  Response extends FixturePagedResponse<Item>
>(
  request: FixturePageRequest,
  loadPage: (request: any) => Promise<Response>
): Promise<Response> {
  const items: Item[] = [];
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

  if (!firstPage || !lastPage) {
    throw new Error("Paged fixture request returned no data.");
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

function bootstrapFixture(api: HcbApi): HcbApi["bootstrap"] {
  return {
    get: vi.fn(async (request) => {
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
        loadAllFixturePages({ limit: 100 }, (pageRequest) =>
          api.tasks.listTaskLists(pageRequest).then(unwrapFixture)
        ),
        loadAllFixturePages({ status: "all", limit: 100 }, (pageRequest) =>
          api.tasks.list(pageRequest).then(unwrapFixture)
        ),
        loadAllFixturePages({ status: "hidden", limit: 100 }, (pageRequest) =>
          api.tasks.list(pageRequest).then(unwrapFixture)
        ),
        loadAllFixturePages({ status: "deleted", limit: 100 }, (pageRequest) =>
          api.tasks.list(pageRequest).then(unwrapFixture)
        ),
        loadAllFixturePages({ limit: 100 }, (pageRequest) =>
          api.calendar.listCalendars(pageRequest).then(unwrapFixture)
        ),
        loadAllFixturePages(request.calendarRange, (pageRequest) =>
          api.calendar.listEvents(pageRequest).then(unwrapFixture)
        ),
        loadAllFixturePages({
          start: request.calendarRange.start,
          end: request.calendarRange.end,
          limit: 500
        }, (pageRequest) =>
          api.calendar.listScheduledTaskBlocks(pageRequest).then(unwrapFixture)
        ),
        loadAllFixturePages({ limit: 50 }, (pageRequest) =>
          api.notes.list(pageRequest).then(unwrapFixture)
        ),
        loadAllFixturePages({ limit: 100 }, (pageRequest) =>
          api.tags.list(pageRequest).then(unwrapFixture)
        ),
        api.settings.get().then(unwrapFixture),
        api.sync.status().then(unwrapFixture),
        api.google.status().then(unwrapFixture),
        api.undo.status().then(unwrapFixture),
        api.native.capabilities().then(unwrapFixture)
      ]);

      return ok({
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
        native,
        resourceCounts: {
          calendarEvents: calendars.items.every((calendar) => calendar.eventCount !== undefined)
            ? calendars.items.reduce((count, calendar) => count + (calendar.eventCount ?? 0), 0)
            : events.page.totalKnown ?? events.items.length,
          notes: notes.page.totalKnown ?? notes.items.length,
          tasks: taskLists.items.every((taskList) => taskList.taskCount !== undefined)
            ? taskLists.items.reduce((count, taskList) => count + (taskList.taskCount ?? 0), 0)
            : (tasks.page.totalKnown ?? tasks.items.length) +
              (hiddenTasks.page.totalKnown ?? hiddenTasks.items.length) +
              (deletedTasks.page.totalKnown ?? deletedTasks.items.length)
        }
      });
    })
  };
}

export function signedOutGoogleStatus(overrides: Partial<GoogleStatusResponse> = {}): GoogleStatusResponse {
  return {
    oauthClientConfigured: false,
    clientId: null,
    hasClientSecret: false,
    accounts: [],
    ...overrides
  };
}

export function seededTaskDetail(id: string, overrides: Partial<TaskDetail> = {}): TaskDetail {
  return {
    id,
    listId: id === "task-inbox-rules" || id === "task-note-startup" ? "list-inbox" : "list-planning",
    title:
      id === "task-note-startup"
        ? "Startup data flow"
        : id === "task-done"
        ? "Report shell-visible timing"
        : id === "task-calendar-fixtures"
          ? "Review calendar fixture shape"
          : "Draft inbox triage rules",
    status: id === "task-done" ? "completed" as const : "active" as const,
    priority:
      id === "task-inbox-rules"
        ? "high" as const
        : id === "task-calendar-fixtures"
          ? "medium" as const
          : "low" as const,
    dueAt: id === "task-done" || id === "task-note-startup" ? null : now,
    updatedAt: now,
    tags: id === "task-inbox-rules" ? ["ops"] : id === "task-calendar-fixtures" ? ["calendar"] : [],
    notes:
      id === "task-note-startup"
        ? "Renderer paints from SQLite before fresh sync completes."
        : id === "task-done"
        ? "Already complete."
        : id === "task-calendar-fixtures"
          ? "Keep visible rows stable for future agenda virtualization."
          : "Define keyboard-first review states.",
    parentId: null,
    ...overrides
  };
}

export function seededHcb(): HcbApi {
  const api = originalHcb!;
  let createdTaskCount = 0;

  const seededApi: HcbApi = {
    ...api,
    tasks: {
      ...api.tasks,
      listTaskLists: vi.fn(async () =>
        ok({
          items: [
            {
              id: "list-inbox",
              title: "Inbox",
              updatedAt: now,
              taskCount: 3,
              activeTaskCount: 2
            },
            {
              id: "list-planning",
              title: "Planning",
              updatedAt: now,
              taskCount: 1,
              activeTaskCount: 1
            }
          ],
          page: { limit: 100, totalKnown: 2 }
        })
      ),
      list: vi.fn(async () =>
        ok({
          items: [
            {
              id: "task-inbox-rules",
              listId: "list-inbox",
              title: "Draft inbox triage rules",
              status: "active" as const,
              priority: "high" as const,
              dueAt: now,
              updatedAt: now,
              tags: ["ops"]
            },
            {
              id: "task-note-startup",
              listId: "list-inbox",
              title: "Startup data flow",
              status: "active" as const,
              priority: "none" as const,
              dueAt: null,
              updatedAt: now,
              notes: "Renderer paints from SQLite before fresh sync completes."
            },
            {
              id: "task-calendar-fixtures",
              listId: "list-planning",
              title: "Review calendar fixture shape",
              status: "active" as const,
              priority: "medium" as const,
              dueAt: now,
              updatedAt: now,
              tags: ["calendar"]
            },
            {
              id: "task-done",
              listId: "list-planning",
              title: "Report shell-visible timing",
              status: "completed" as const,
              priority: "low" as const,
              dueAt: null,
              updatedAt: now
            }
          ],
          page: { limit: 100, totalKnown: 3 }
        })
      ),
      get: vi.fn(async ({ id }) => ok(seededTaskDetail(id))),
      create: vi.fn(async (request) =>
        ok({
          id: `task-created-${++createdTaskCount}`,
          listId: request.listId,
          title: request.title,
          status: "active" as const,
          priority: request.priority ?? "none",
          dueAt: request.dueDate ? `${request.dueDate}T00:00:00.000Z` : null,
          updatedAt: now,
          notes: request.notes ?? "",
          parentId: request.parentId ?? null,
          plannedStart: request.plannedStart ?? null,
          plannedEnd: request.plannedEnd ?? null,
          durationMinutes: request.durationMinutes ?? null,
          lockedSchedule: request.lockedSchedule ?? false,
          snoozeUntil: request.snoozeUntil ?? null,
          tags: request.tags ?? []
        })
      ),
      update: vi.fn(async (request) =>
        ok(
          seededTaskDetail(request.id, {
            ...(request.title === undefined ? {} : { title: request.title }),
            ...(request.notes === undefined ? {} : { notes: request.notes }),
            ...(request.dueDate === undefined
              ? {}
              : { dueAt: request.dueDate ? `${request.dueDate}T00:00:00.000Z` : null }),
            ...(request.listId === undefined ? {} : { listId: request.listId }),
            ...(request.parentId === undefined ? {} : { parentId: request.parentId }),
            ...(request.priority === undefined ? {} : { priority: request.priority }),
            ...(request.plannedStart === undefined ? {} : { plannedStart: request.plannedStart }),
            ...(request.plannedEnd === undefined ? {} : { plannedEnd: request.plannedEnd }),
            ...(request.durationMinutes === undefined ? {} : { durationMinutes: request.durationMinutes }),
            ...(request.lockedSchedule === undefined ? {} : { lockedSchedule: request.lockedSchedule }),
            ...(request.snoozeUntil === undefined ? {} : { snoozeUntil: request.snoozeUntil }),
            ...(request.tags === undefined ? {} : { tags: request.tags })
          })
        )
      ),
      complete: vi.fn(async ({ id }) => ok(seededTaskDetail(id, { status: "completed" }))),
      reopen: vi.fn(async ({ id }) => ok(seededTaskDetail(id, { status: "active" }))),
      move: vi.fn(async (request) =>
        ok(
          seededTaskDetail(request.id, {
            ...(request.listId === undefined ? {} : { listId: request.listId }),
            ...(request.parentId === undefined ? {} : { parentId: request.parentId })
          })
        )
      ),
      delete: vi.fn(async ({ id }) => ok({ id, queued: true, revision: now })),
      createTaskList: vi.fn(async (request) =>
        ok({
          id: "list-created",
          title: request.title,
          updatedAt: now,
          taskCount: 0,
          activeTaskCount: 0
        })
      ),
      renameTaskList: vi.fn(async (request) =>
        ok({
          id: request.id,
          title: request.title,
          updatedAt: now,
          taskCount: request.id === "list-inbox" ? 2 : 1,
          activeTaskCount: request.id === "list-inbox" ? 1 : 1
        })
      ),
      deleteTaskList: vi.fn(async ({ id }) => ok({ id, queued: true, revision: now }))
    },
    calendar: {
      ...api.calendar,
      listCalendars: vi.fn(async () =>
        ok({
          items: [
            {
              id: "cal-product",
              title: "Product",
              selected: true,
              timeZone: "UTC",
              backgroundColor: "#34a853",
              foregroundColor: "#ffffff",
              updatedAt: now,
              eventCount: 1
            }
          ],
          page: { limit: 100, totalKnown: 1 }
        })
      ),
      listEvents: vi.fn(async () =>
        ok({
          items: [
            {
              id: "event-standup",
              calendarId: "cal-product",
              title: "Planner shell standup",
              startsAt: `${todayDate}T09:30:00.000Z`,
              endsAt: `${todayDate}T09:50:00.000Z`,
              allDay: false,
              updatedAt: now
            },
            {
              id: "event-review",
              calendarId: "cal-product",
              title: "Renderer acceptance review",
              startsAt: `${todayDate}T15:30:00.000Z`,
              endsAt: `${todayDate}T16:15:00.000Z`,
              allDay: false,
              updatedAt: now
            }
          ],
          page: { limit: 250, totalKnown: 2 }
        })
      ),
      listScheduledTaskBlocks: vi.fn(async () =>
        ok({
          items: [],
          page: { limit: 250, totalKnown: 0 }
        })
      ),
      scheduleTaskBlock: vi.fn(async (request) =>
        ok({
          id: "block-created",
          taskId: request.taskId,
          calendarEventId: "event-task-block",
          calendarId: request.calendarId,
          title: request.taskId === "task-calendar-fixtures"
            ? "Review calendar fixture shape"
            : "Draft inbox triage rules",
          startsAt: request.startsAt,
          endsAt: new Date(
            Date.parse(request.startsAt) + (request.durationMinutes ?? 30) * 60 * 1000
          ).toISOString(),
          durationMinutes: request.durationMinutes ?? 30,
          status: "scheduled" as const,
          mutationState: "queued" as const,
          updatedAt: now
        })
      ),
      moveScheduledTaskBlock: vi.fn(async (request) =>
        ok({
          id: request.id,
          taskId: "task-inbox-rules",
          calendarEventId: "event-task-block",
          calendarId: request.calendarId ?? "cal-product",
          title: "Draft inbox triage rules",
          startsAt: request.startsAt ?? `${todayDate}T10:00:00.000Z`,
          endsAt: new Date(
            Date.parse(request.startsAt ?? `${todayDate}T10:00:00.000Z`) +
              (request.durationMinutes ?? 30) * 60 * 1000
          ).toISOString(),
          durationMinutes: request.durationMinutes ?? 30,
          status: "scheduled" as const,
          mutationState: "queued" as const,
          updatedAt: now
        })
      ),
      unscheduleTaskBlock: vi.fn(async (request) =>
        ok({
          id: request.id,
          queued: request.deleteCalendarEvent ?? true,
          revision: now
        })
      ),
      scheduleSuggest: vi.fn(async (request) =>
        ok({
          slots: [
            {
              startsAt: `${request.date}T09:30:00.000Z`,
              endsAt: `${request.date}T09:50:00.000Z`,
              eventId: "event-standup",
              locked: true,
              conflict: false
            },
            {
              startsAt: `${request.date}T10:00:00.000Z`,
              endsAt: `${request.date}T10:45:00.000Z`,
              taskId: "task-calendar-fixtures",
              locked: false,
              conflict: false
            }
          ],
          unscheduled: [
            seededTaskDetail("task-inbox-rules", {
              durationMinutes: 30
            })
          ],
          overloadMinutes: 0
        })
      ),
      exportAvailability: vi.fn(async (request) =>
        ok({
          format: "text" as const,
          text: `Availability from ${request.start} to ${request.end}`,
          generatedAt: now,
          busyBlockCount: 2
        })
      )
    },
    notes: {
      ...api.notes,
      list: vi.fn(async () =>
        ok({
          items: [
            {
              id: "note-cache-first",
              listId: "list-inbox",
              listTitle: "Notes",
              title: "Startup data flow",
              preview: "Renderer paints from SQLite.",
              updatedAt: now
            }
          ],
          lists: [{ id: "list-inbox", title: "Notes", noteCount: 1, updatedAt: now }],
          page: { limit: 50, totalKnown: 1 }
        })
      ),
      get: vi.fn(async ({ id }) =>
        ok({
          id,
          listId: "list-inbox",
          listTitle: "Notes",
          title: "Startup data flow",
          preview: "Renderer paints from SQLite.",
          body: "Renderer paints from SQLite before fresh sync completes.",
          updatedAt: now
        })
      ),
      create: vi.fn(async (request) =>
        ok({
          id: "note-created",
          listId: request.listId ?? "list-inbox",
          listTitle: "Notes",
          title: request.title,
          preview: request.body ?? "Empty note",
          body: request.body ?? "",
          updatedAt: now
        })
      ),
      update: vi.fn(async (request) =>
        ok({
          id: request.id,
          listId: request.listId ?? "list-inbox",
          listTitle: "Notes",
          title: request.title ?? "Untitled note",
          preview: request.body ?? "",
          body: request.body ?? "",
          updatedAt: now
        })
      ),
      createList: vi.fn(async (request) =>
        ok({
          id: "note-list:new",
          title: request.title,
          noteCount: 0,
          updatedAt: now
        })
      ),
      renameList: vi.fn(async (request) =>
        ok({
          id: request.id,
          title: request.title,
          noteCount: 1,
          updatedAt: now
        })
      ),
      deleteList: vi.fn(async (request) =>
        ok({
          id: request.id,
          queued: false,
          revision: now
        })
      ),
      delete: vi.fn(async (request) =>
        ok({
          id: request.id,
          queued: false,
          revision: now
        })
      ),
      linkSuggest: vi.fn(async (request) => {
        const query = request.query.toLowerCase();
        const items = [
          { kind: "note" as const, id: "note-cache-first", label: "Startup data flow" },
          { kind: "task" as const, id: "task-inbox-rules", label: "Draft inbox triage rules" },
          { kind: "event" as const, id: "event-standup", label: "Planner shell standup" }
        ].filter((item) => item.label.toLowerCase().includes(query));

        return ok({ items: items.slice(0, request.limit ?? 8) });
      }),
      listBrokenLinks: vi.fn(async () => ok({ items: [] })),
      entityLinks: vi.fn(async () => ok({ outgoing: [], backlinks: [], broken: [] }))
    },
    tags: {
      ...api.tags,
      list: vi.fn(async () =>
        ok({
          items: [
            {
              id: "tag-ops",
              name: "ops",
              color: null,
              taskCount: 1,
              eventCount: 0,
              noteCount: 0,
              totalCount: 1,
              createdAt: now,
              updatedAt: now
            }
          ],
          page: { limit: 100, totalKnown: 1 }
        })
      ),
      create: vi.fn(async (request) =>
        ok({
          id: `tag-${request.name}`,
          queued: false,
          revision: now,
          tag: {
            id: `tag-${request.name}`,
            name: request.name,
            color: request.color ?? null,
            taskCount: 0,
            eventCount: 0,
            noteCount: 0,
            totalCount: 0,
            createdAt: now,
            updatedAt: now
          }
        })
      ),
      update: vi.fn(async (request) =>
        ok({
          id: request.id,
          queued: false,
          revision: now,
          tag: {
            id: request.id,
            name: request.name ?? "ops",
            color: request.color ?? null,
            taskCount: 1,
            eventCount: 0,
            noteCount: 0,
            totalCount: 1,
            createdAt: now,
            updatedAt: now
          }
        })
      ),
      delete: vi.fn(async (request) => ok({ id: request.id, queued: false, revision: now })),
      merge: vi.fn(async (request) => ok({ id: request.targetId, queued: false, revision: now })),
      bulkApply: vi.fn(async () => ok({ id: "bulk-tags", queued: false, revision: now })),
      previewAutoReapply: vi.fn(async (request) =>
        ok({
          kind: request.kind,
          scope: request.scope ?? "all",
          scanned: 1,
          changed: request.kind === "task" ? 1 : 0,
          skipped: request.kind === "task" ? 0 : 1,
          failed: 0,
          blocked: false,
          message: request.kind === "task" ? "1 item needs reapply." : "No items need reapply.",
          sample: []
        })
      ),
      applyAutoReapply: vi.fn(async (request) =>
        ok({
          kind: request.kind,
          scope: request.scope ?? "all",
          scanned: 1,
          changed: request.kind === "task" ? 1 : 0,
          skipped: request.kind === "task" ? 0 : 1,
          failed: 0,
          blocked: false,
          message: request.kind === "task" ? "1 item updated." : "No items need reapply.",
          sample: [],
          queued: request.kind === "task",
          revision: now,
          undoLabel: request.kind === "task" ? "Auto-tag reapply" : undefined
        })
      )
    },
    duplicates: {
      cleanup: vi.fn(async (request) =>
        ok({
          id: request.winnerId,
          kind: request.kind,
          loserIds: request.loserIds,
          queued: true,
          revision: now
        })
      )
    },
    search: {
      query: vi.fn(async (request) => {
        const query = request.query.toLowerCase();
        const items: SearchResultItem[] = [];

        if (query.includes("triage") || query.includes("task")) {
          items.push({
            id: "task-inbox-rules",
            domain: "tasks" as const,
            title: "Draft inbox triage rules",
            snippet: "Task in Inbox",
            updatedAt: now
          });
        }

        if (query.includes("review") || query.includes("event")) {
          items.push({
            id: "event-review",
            domain: "calendar" as const,
            title: "Renderer acceptance review",
            snippet: "Calendar event at 15:30",
            updatedAt: now
          });
        }

        if (query.includes("cache") || query.includes("note")) {
          items.push({
            id: "note-cache-first",
            domain: "notes" as const,
            title: "Startup data flow",
            snippet: "Note updated from sync",
            updatedAt: now
          });
        }

        return ok({
          items,
          page: { limit: 30, totalKnown: items.length }
        });
      }),
      listModels: vi.fn(async () =>
        ok({
          models: defaultSemanticSearchModels,
          selectedModelId: "hcb-local-hash-384",
          enabled: false
        })
      ),
      installModel: vi.fn(async (request) =>
        ok({
          model: { ...defaultSemanticSearchModels[0], id: request.modelId, installed: true, installState: "installed" as const, updatedAt: now },
          selectedModelId: request.modelId,
          enabled: true
        })
      ),
      uninstallModel: vi.fn(async (request) =>
        ok({
          model: { ...defaultSemanticSearchModels[0], id: request.modelId, installed: false, installState: "not-installed" as const, updatedAt: now },
          selectedModelId: "hcb-local-hash-384",
          enabled: false
        })
      ),
      rebuildIndex: vi.fn(async (request = {}) =>
        ok({
          modelId: request.modelId ?? "hcb-local-hash-384",
          indexedCount: 0,
          staleCount: 0
        })
      )
    },
    sync: {
      ...api.sync,
      status: vi.fn(async () =>
        ok({
          state: "idle" as const,
          pendingMutationCount: 0,
          lastCompletedAt: now,
          offline: false,
          stale: false
        })
      )
    },
    google: {
      ...api.google,
      status: vi.fn(async () => ok(connectedGoogleStatus()))
    },
    settings: {
      ...api.settings,
      listAttachments: vi.fn(async () => ok({ items: [] })),
      addAttachment: vi.fn(async () => ok({ items: [], queued: false, revision: now })),
      removeAttachment: vi.fn(async () => ok({ items: [], queued: false, revision: now })),
      openAttachment: vi.fn(async () => ok({ path: "/tmp/hcb-attachment", message: "Attachment opened." })),
      downloadAttachment: vi.fn(async () => ok({ path: "/tmp/hcb-attachment", message: "Attachment downloaded." }))
    },
    undo: {
      ...api.undo,
      status: vi.fn(async () => ok({ canUndo: false, canRedo: false })),
      undo: vi.fn(async () => ok({ action: "undo" as const, applied: false })),
      redo: vi.fn(async () => ok({ action: "redo" as const, applied: false }))
    },
    agent: {
      listActions: vi.fn(async () => ok({ items: [], page: { limit: 50, totalKnown: 0 } })),
      applyAction: vi.fn(async (request) =>
        ok({
          action: {
            id: request.id,
            status: "applied" as const,
            toolName: "hcb_create_task",
            summary: "Applied",
            createdAt: now,
            expiresAt: now,
            updatedAt: now,
            appliedAt: now,
            errorMessage: null
          }
        })
      ),
      rejectAction: vi.fn(async (request) =>
        ok({
          action: {
            id: request.id,
            status: "rejected" as const,
            toolName: "hcb_create_task",
            summary: "Rejected",
            createdAt: now,
            expiresAt: now,
            updatedAt: now,
            appliedAt: null,
            errorMessage: null
          }
        })
      ),
      clearExpired: vi.fn(async () => ok({ cleared: 0 }))
    },
    webhooks: {
      list: vi.fn(async () => ok({ items: [], page: { limit: 50, totalKnown: 0 } })),
      upsert: vi.fn(async (request) =>
        ok({
          id: request.id ?? "webhook:test",
          queued: false,
          revision: now,
          subscription: {
            id: request.id ?? "webhook:test",
            url: request.url,
            events: request.events,
            enabled: request.enabled,
            includePrivateBodies: request.includePrivateBodies ?? false,
            createdAt: now,
            updatedAt: now,
            lastDeliveryAt: null,
            lastError: null
          }
        })
      ),
      delete: vi.fn(async (request) => ok({ id: request.id, queued: false, revision: now })),
      test: vi.fn(async (request) => ok({ id: request.id, queued: false, revision: now }))
    }
  };

  seededApi.bootstrap = bootstrapFixture(seededApi);
  return seededApi;
}

export function onboardingHcb(
  overrides: Partial<SettingsSnapshot> = {}
): { api: HcbApi; getSettings: () => SettingsSnapshot } {
  const api = seededHcb();
  let settings = testSettings({
    setupCompletedAt: null,
    selectedTaskListIds: [],
    selectedCalendarIds: [],
    ...overrides
  });

  api.settings.get = vi.fn(async () => ok(settings));
  api.google.status = vi.fn(async () => ok(signedOutGoogleStatus()));
  api.settings.update = vi.fn(async (request) => {
    settings = testSettings({
      ...settings,
      ...request,
      setupCompletedAt:
        request.setupCompletedAt === undefined ? settings.setupCompletedAt : request.setupCompletedAt
    });

    return ok(settings);
  });
  api.settings.recoveryAction = vi.fn(async (request) => {
    if (request.action === "resetOnboarding") {
      settings = testSettings({
        ...settings,
        setupCompletedAt: null
      });
    }

    return ok({
      action: request.action,
      accepted: true,
      destructive: request.action !== "refresh" && request.action !== "resetOnboarding",
      requiresReload: request.action === "clearGoogleCache",
      message: "Recovery action accepted."
    });
  });

  return { api, getSettings: () => settings };
}

export function loadingHcb(): HcbApi {
  const api = originalHcb!;
  const pendingRead = new Promise<never>(() => undefined);

  return {
    ...api,
    bootstrap: {
      get: vi.fn(() => pendingRead)
    },
    tasks: {
      ...api.tasks,
      listTaskLists: vi.fn(() => pendingRead)
    }
  };
}

export function settingsLoadingHcb(): HcbApi {
  const api = seededHcb();
  const pendingRead = new Promise<never>(() => undefined);

  const loadingApi: HcbApi = {
    ...api,
    settings: {
      ...api.settings,
      get: vi.fn(() => pendingRead)
    }
  };

  loadingApi.bootstrap = bootstrapFixture(loadingApi);
  return loadingApi;
}
