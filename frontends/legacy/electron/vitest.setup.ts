import "@testing-library/jest-dom/vitest";
import { beforeEach, vi } from "vitest";
import type { CalendarEventDetail, SettingsSnapshot, TaskDetail } from "./src/shared/ipc/contracts";
import { defaultSemanticSearchModels } from "./src/shared/ipc/contracts";
import type { HcbApi } from "./src/shared/ipc/preloadApi";
import { ok } from "./src/shared/ipc/result";
import {
  defaultHistoryCategoryVisibility,
  defaultKeybindings,
  defaultLeaderKey,
  defaultLeaderKeybindings
} from "./src/shared/settingsCatalog";

const now = new Date("2026-05-22T00:00:00.000Z").toISOString();
const later = new Date("2026-05-22T01:00:00.000Z").toISOString();

function installTestLocalStorage(): Storage | undefined {
  if (typeof window === "undefined") {
    return undefined;
  }

  const storage = window.localStorage;

  if (
    typeof storage?.getItem === "function" &&
    typeof storage.setItem === "function" &&
    typeof storage.removeItem === "function"
  ) {
    return storage;
  }

  const values = new Map<string, string>();
  const memoryStorage: Storage = {
    get length() {
      return values.size;
    },
    clear() {
      values.clear();
    },
    getItem(key: string) {
      return values.get(key) ?? null;
    },
    key(index: number) {
      return Array.from(values.keys())[index] ?? null;
    },
    removeItem(key: string) {
      values.delete(key);
    },
    setItem(key: string, value: string) {
      values.set(key, value);
    }
  };

  Object.defineProperty(window, "localStorage", {
    configurable: true,
    value: memoryStorage
  });

  return memoryStorage;
}

beforeEach(() => {
  installTestLocalStorage()?.removeItem("hcb.paneWorkspace.v1");
});

function testTask(overrides: Partial<TaskDetail> = {}): TaskDetail {
  return {
    id: "test-task",
    listId: "test-list",
    title: "Test task",
    status: "active" as const,
    updatedAt: now,
    priority: "none" as const,
    dueAt: null,
    notes: "",
    parentId: null,
    ...overrides
  };
}

function testCalendarEvent(overrides: Partial<CalendarEventDetail> = {}): CalendarEventDetail {
  return {
    id: "test-event",
    calendarId: "test-calendar",
    title: "Test event",
    startsAt: now,
    endsAt: later,
    allDay: false,
    updatedAt: now,
    calendarTitle: "Test calendar",
    deepLink: "hotcrossbuns://calendar/test-event",
    location: "",
    notes: "",
    guestEmails: [],
    reminderMinutes: [],
    ...overrides
  };
}

const hcbApi: HcbApi = {
  bootstrap: {
    get: vi.fn(async (request) =>
      ok({
        taskLists: await hcbApi.tasks.listTaskLists({ limit: 100 }).then((result) => result.ok ? result.data : { items: [], page: { limit: 100, totalKnown: 0 } }),
        tasks: await hcbApi.tasks.list({ status: "all", limit: 100 }).then((result) => result.ok ? result.data : { items: [], page: { limit: 100, totalKnown: 0 } }),
        hiddenTasks: await hcbApi.tasks.list({ status: "hidden", limit: 100 }).then((result) => result.ok ? result.data : { items: [], page: { limit: 100, totalKnown: 0 } }),
        deletedTasks: await hcbApi.tasks.list({ status: "deleted", limit: 100 }).then((result) => result.ok ? result.data : { items: [], page: { limit: 100, totalKnown: 0 } }),
        calendars: await hcbApi.calendar.listCalendars({ limit: 100 }).then((result) => result.ok ? result.data : { items: [], page: { limit: 100, totalKnown: 0 } }),
        events: await hcbApi.calendar.listEvents(request.calendarRange).then((result) => result.ok ? result.data : { items: [], page: { limit: request.calendarRange.limit ?? 100, totalKnown: 0 } }),
        scheduledTaskBlocks: await hcbApi.calendar.listScheduledTaskBlocks(request.calendarRange).then((result) => result.ok ? result.data : { items: [], page: { limit: request.calendarRange.limit ?? 100, totalKnown: 0 } }),
        notes: await hcbApi.notes.list({ limit: 50 }).then((result) => result.ok ? result.data : { items: [], lists: [], page: { limit: 50, totalKnown: 0 } }),
        tags: await hcbApi.tags.list({ limit: 100 }).then((result) => result.ok ? result.data : { items: [], page: { limit: 100, totalKnown: 0 } }),
        settings: await hcbApi.settings.get().then((result) => {
          if (!result.ok) {
            throw new Error("settings fixture failed");
          }
          return result.data;
        }),
        syncStatus: await hcbApi.sync.status().then((result) => {
          if (!result.ok) {
            throw new Error("sync fixture failed");
          }
          return result.data;
        }),
        googleStatus: await hcbApi.google.status().then((result) => {
          if (!result.ok) {
            throw new Error("google fixture failed");
          }
          return result.data;
        }),
        undoStatus: await hcbApi.undo.status().then((result) => {
          if (!result.ok) {
            throw new Error("undo fixture failed");
          }
          return result.data;
        }),
        native: await hcbApi.native.capabilities().then((result) => {
          if (!result.ok) {
            throw new Error("native fixture failed");
          }
          return result.data;
        }),
        resourceCounts: {
          calendarEvents: 0,
          notes: 0,
          tasks: 0
        }
      })
    )
  },
  tasks: {
    listTaskLists: vi.fn(async (request = {}) =>
      ok({
        items: [],
        page: {
          limit: request.limit ?? 50,
          totalKnown: 0
        }
      })
    ),
    list: vi.fn(async (request = {}) =>
      ok({
        items: [],
        page: {
          limit: request.limit ?? 50,
          totalKnown: 0
        }
      })
    ),
    get: vi.fn(async (request) =>
      ok(testTask({ id: request.id }))
    ),
    create: vi.fn(async (request) =>
      ok(testTask({
        id: "task-created",
        listId: request.listId,
        title: request.title,
        notes: request.notes,
        parentId: request.parentId ?? null,
        priority: request.priority ?? "none"
      }))
    ),
    update: vi.fn(async (request) =>
      ok(testTask({
        id: request.id,
        title: request.title ?? "Updated task",
        notes: request.notes ?? "",
        listId: request.listId ?? "test-list",
        parentId: request.parentId ?? null,
        priority: request.priority ?? "none"
      }))
    ),
    complete: vi.fn(async (request) =>
      ok(testTask({ id: request.id, status: "completed" }))
    ),
    reopen: vi.fn(async (request) =>
      ok(testTask({ id: request.id, status: "active" }))
    ),
    move: vi.fn(async (request) =>
      ok(testTask({
        id: request.id,
        listId: request.listId ?? "test-list",
        parentId: request.parentId ?? null
      }))
    ),
    bulkReschedule: vi.fn(async (request) =>
      ok({
        ids: request.taskIds,
        updatedCount: request.taskIds.length,
        queued: true,
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
    createTaskList: vi.fn(async (request) =>
      ok({
        id: "task-list-created",
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
        taskCount: 0,
        activeTaskCount: 0
      })
    ),
    deleteTaskList: vi.fn(async (request) =>
      ok({
        id: request.id,
        queued: false,
        revision: now
      })
    )
  },
  calendar: {
    listCalendars: vi.fn(async (request = {}) =>
      ok({
        items: [],
        page: {
          limit: request.limit ?? 50,
          totalKnown: 0
        }
      })
    ),
    listEvents: vi.fn(async (request) =>
      ok({
        items: [],
        page: {
          limit: request.limit ?? 100,
          totalKnown: 0
        }
      })
    ),
    get: vi.fn(async (request) =>
      ok(testCalendarEvent({ id: request.id }))
    ),
    create: vi.fn(async (request) =>
      ok(testCalendarEvent({
        id: "event-created",
        calendarId: request.calendarId,
        title: request.title,
        startsAt: request.startsAt,
        endsAt: request.endsAt,
        allDay: request.allDay ?? false,
        location: request.location ?? "",
        notes: request.notes ?? "",
        guestEmails: request.guestEmails ?? [],
        reminderMinutes: request.reminderMinutes ?? []
      }))
    ),
    update: vi.fn(async (request) =>
      ok(testCalendarEvent({
        id: request.id,
        calendarId: request.calendarId ?? "test-calendar",
        title: request.title ?? "Updated event",
        startsAt: request.startsAt ?? now,
        endsAt: request.endsAt ?? later,
        allDay: request.allDay ?? false,
        location: request.location ?? "",
        notes: request.notes ?? "",
        guestEmails: request.guestEmails ?? [],
        reminderMinutes: request.reminderMinutes ?? []
      }))
    ),
    delete: vi.fn(async (request) =>
      ok({
        id: request.id,
        queued: false,
        revision: now
      })
    ),
    complete: vi.fn(async (request) =>
      ok(testCalendarEvent({
        id: request.id,
        completedAt: now,
        completionScopeApplied: request.scope ?? "occurrence"
      }))
    ),
    reopen: vi.fn(async (request) =>
      ok(testCalendarEvent({
        id: request.id,
        completedAt: null,
        completionScopeApplied: request.scope ?? "occurrence"
      }))
    ),
    listScheduledTaskBlocks: vi.fn(async (request) =>
      ok({
        items: [],
        page: {
          limit: request.limit ?? 100,
          totalKnown: 0
        }
      })
    ),
    scheduleTaskBlock: vi.fn(async (request) =>
      ok({
        id: "block-created",
        taskId: request.taskId,
        calendarEventId: "event-created",
        calendarId: request.calendarId,
        title: "Test task",
        startsAt: request.startsAt,
        endsAt: new Date(Date.parse(request.startsAt) + (request.durationMinutes ?? 30) * 60 * 1000).toISOString(),
        durationMinutes: request.durationMinutes ?? 30,
        status: "scheduled" as const,
        mutationState: "queued" as const,
        updatedAt: now
      })
    ),
    moveScheduledTaskBlock: vi.fn(async (request) =>
      ok({
        id: request.id,
        taskId: "test-task",
        calendarEventId: "event-created",
        calendarId: request.calendarId ?? "test-calendar",
        title: "Test task",
        startsAt: request.startsAt ?? now,
        endsAt: new Date(Date.parse(request.startsAt ?? now) + (request.durationMinutes ?? 30) * 60 * 1000).toISOString(),
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
    scheduleSuggest: vi.fn(async () =>
      ok({
        slots: [],
        unscheduled: [],
        overloadMinutes: 0
      })
    ),
    smartReschedule: vi.fn(async (request) =>
      ok({
        suggestions: [],
        skipped: [],
        applied: request.apply ?? false,
        appliedBlocks: []
      })
    ),
    exportAvailability: vi.fn(async (request) =>
      ok({
        format: "text" as const,
        text: `Availability from ${request.start} to ${request.end}`,
        generatedAt: now,
        busyBlockCount: 0
      })
    )
  },
  notes: {
    list: vi.fn(async (request = {}) =>
      ok({
        items: [],
        lists: [{ id: "note-list:default", title: "Local notes", noteCount: 0, updatedAt: now }],
        page: {
          limit: request.limit ?? 50,
          totalKnown: 0
        }
      })
    ),
    createList: vi.fn(async (request) =>
      ok({
        id: "note-list-created",
        title: request.title,
        noteCount: 0,
        updatedAt: now
      })
    ),
    renameList: vi.fn(async (request) =>
      ok({
        id: request.id,
        title: request.title,
        noteCount: 0,
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
    get: vi.fn(async (request) =>
      ok({
        id: request.id,
        listId: "note-list:default",
        listTitle: "Local notes",
        title: "Test note",
        preview: "",
        body: "",
        updatedAt: now
      })
    ),
    create: vi.fn(async (request) =>
      ok({
        id: "note-created",
        listId: request.listId ?? "note-list:default",
        listTitle: "Local notes",
        title: request.title,
        preview: request.body ?? "",
        body: request.body ?? "",
        updatedAt: now
      })
    ),
    update: vi.fn(async (request) =>
      ok({
        id: request.id,
        listId: request.listId ?? "note-list:default",
        listTitle: "Local notes",
        title: request.title ?? "Updated note",
        preview: request.body ?? "",
        body: request.body ?? "",
        updatedAt: now
      })
    ),
    delete: vi.fn(async (request) =>
      ok({
        id: request.id,
        queued: false,
        revision: now
      })
    ),
    linkSuggest: vi.fn(async () => ok({ items: [] })),
    listBrokenLinks: vi.fn(async () => ok({ items: [] })),
    entityLinks: vi.fn(async () => ok({ backlinks: [], broken: [], outgoing: [] }))
  },
  tags: {
    list: vi.fn(async (request = {}) =>
      ok({
        items: [],
        page: {
          limit: request.limit ?? 100,
          totalKnown: 0
        }
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
          name: request.name ?? request.id,
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
    delete: vi.fn(async (request) => ok({ id: request.id, queued: false, revision: now })),
    merge: vi.fn(async (request) => ok({ id: request.targetId, queued: false, revision: now })),
    bulkApply: vi.fn(async () => ok({ id: "bulk-tags", queued: false, revision: now })),
    previewAutoReapply: vi.fn(async (request) =>
      ok({
        kind: request.kind,
        scope: "all" as const,
        scanned: 0,
        changed: 0,
        skipped: 0,
        failed: 0,
        blocked: false,
        message: "No changes.",
        sample: []
      })
    ),
    applyAutoReapply: vi.fn(async (request) =>
      ok({
        kind: request.kind,
        scope: "all" as const,
        scanned: 0,
        changed: 0,
        skipped: 0,
        failed: 0,
        blocked: false,
        message: "No changes.",
        sample: [],
        queued: false,
        revision: now
      })
    ),
    analytics: vi.fn(async () =>
      ok({
        totalTags: 0,
        unusedTags: 0,
        linkedEntities: 0,
        topTags: [],
        staleTags: []
      })
    )
  },
  search: {
    query: vi.fn(async (request) =>
      ok({
        items: [],
        page: {
          limit: request.limit ?? 20,
          totalKnown: 0
        },
        diagnostics: {
          mode: request.mode ?? "lexical",
          semanticEnabled: false,
          indexedCount: 0,
          staleCount: 0,
          modelId: "hcb-local-hash-384"
        }
      })
    ),
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
  duplicates: {
    cleanup: vi.fn(async (request) =>
      ok({
        id: request.winnerId,
        kind: request.kind,
        loserIds: request.loserIds,
        queued: false,
        revision: now
      })
    )
  },
  sync: {
    status: vi.fn(async () =>
      ok({
        state: "idle" as const,
        pendingMutationCount: 0
      })
    ),
    runNow: vi.fn(async (request = {}) =>
      ok({
        accepted: true,
        dryRun: request.dryRun ?? false,
        drainOnly: request.drainOnly ?? false,
        resources: request.drainOnly ? [] : request.resources ?? ["tasks", "calendar"]
      })
    ),
    subscribeStatus: vi.fn(() => () => undefined)
  },
  google: {
    status: vi.fn(async () =>
      ok({
        oauthClientConfigured: true,
        clientId: "desktop-client-id.apps.googleusercontent.com",
        hasClientSecret: false,
        account: {
          accountId: "google:test-account",
          googleAccountId: "test-account",
          email: "planner@example.com",
          displayName: "Planner Test",
          connectionState: "connected" as const,
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
            connectionState: "connected" as const,
            grantedScopes: [
              "https://www.googleapis.com/auth/tasks",
              "https://www.googleapis.com/auth/calendar"
            ],
            missingScopes: [],
            lastAuthenticatedAt: now,
            updatedAt: now
          }
        ]
      })
    ),
    saveOAuthClient: vi.fn(async (request) =>
      ok({
        oauthClientConfigured: true,
        clientId: request.clientId,
        hasClientSecret: Boolean(request.clientSecret),
        accounts: []
      })
    ),
    beginOAuth: vi.fn(async () =>
      ok({
        accepted: true,
        openedExternalBrowser: true,
        expiresAt: later,
        scopes: [
          "https://www.googleapis.com/auth/tasks",
          "https://www.googleapis.com/auth/calendar"
        ],
        redirectUri: "http://127.0.0.1:42813/oauth/google/callback",
        message: "Google authorization opened in the browser."
      })
    ),
    disconnect: vi.fn(async () =>
      ok({
        oauthClientConfigured: false,
        clientId: null,
        hasClientSecret: false,
        accounts: []
      })
    )
  },
  settings: {
    get: vi.fn(async () =>
      ok({
        theme: "system" as const,
        colorTheme: "notion" as const,
        customBackground: null,
        useInferredBackgroundTheme: true,
        appLanguage: "system" as const,
        uiFontName: null,
        uiTextSizePoints: 13,
        perSurfaceFontOverrides: {},
        calendarEventColorOverrides: {},
        autoTagRules: [],
        autoTagBackgroundReapplyMode: "preview" as const,
        performanceMode: "snappy" as const,
        appBackgroundTranslucencyEnabled: false,
        appBackgroundOpacity: 1,
        disableAnimations: false,
        uiLayoutScale: 1,
        navigationPlacement: "left" as const,
        hiddenNavigationTabs: [],
        navigationTabOrder: ["calendar", "tasks", "notes"] as SettingsSnapshot["navigationTabOrder"],
        toolbarActionOrder: ["commandPalette", "notifications", "diagnostics", "splitPane", "refresh", "settings"] as SettingsSnapshot["toolbarActionOrder"],
        hiddenCalendarViewModes: [],
        showCompletedInCalendarViews: true,
        eventCompletionDefaultScope: "occurrence" as const,
        calendarTimelineDensity: "compact" as const,
        monthScrollPastMonths: 0,
        monthScrollFutureMonths: 1,
        quickCreateExpandedByDefault: false,
        restoreWindowStateEnabled: true,
        startOnLogin: false,
        selectedTaskListIds: [],
        selectedCalendarIds: [],
        storageBackend: "google" as const,
        hcbHosterEndpoint: null,
        hcbVaultPath: null,
        hcbHosterLastPackageSha256: null,
        setupCompletedAt: now,
        syncMode: "balanced" as const,
        syncTasksEnabled: true,
        syncCalendarEventsEnabled: true,
        eventRetentionDaysBack: 0,
        completedTaskRetentionDaysBack: 365,
        keybindings: defaultKeybindings,
        leaderKey: defaultLeaderKey,
        leaderKeybindings: defaultLeaderKeybindings,
        showTrayIcon: true,
        trayClickAction: "open-menu" as const,
        menuBarPanelStyle: "adaptive" as const,
        menuBarIconName: "calendar" as const,
        menuBarCalendarIconId: "calendar",
        menuBarCalendarDoneMode: "visibleTodayDone" as const,
        customMenuBarIcons: [],
        showMenuBarBadge: true,
        showDockBadge: true,
        notificationsEnabled: false,
        notificationLeadMinutes: 10,
        taskCompletionSoundEnabled: true,
        taskCompletionSoundId: "glass" as const,
        eventCompletionSoundEnabled: true,
        eventCompletionSoundId: "pop" as const,
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
        mcpEnabled: false,
        mcpPermissionMode: "confirm-writes" as const,
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
        semanticSearchMode: "lexical" as const,
        embeddingModelId: "hcb-local-hash-384",
        semanticSearchModels: defaultSemanticSearchModels,
        agentActionTrayEnabled: true,
        webhooksEnabled: false
      })
    ),
    update: vi.fn(async (request) =>
      ok({
        theme: request.theme ?? "system",
        colorTheme: request.colorTheme ?? "notion",
        customBackground: request.customBackground ?? null,
        useInferredBackgroundTheme: request.useInferredBackgroundTheme ?? true,
        appLanguage: request.appLanguage ?? "system",
        uiFontName: request.uiFontName === undefined ? null : request.uiFontName,
        uiTextSizePoints: request.uiTextSizePoints ?? 13,
        perSurfaceFontOverrides: request.perSurfaceFontOverrides ?? {},
        calendarEventColorOverrides: request.calendarEventColorOverrides ?? {},
        autoTagRules: request.autoTagRules ?? [],
        autoTagBackgroundReapplyMode: request.autoTagBackgroundReapplyMode ?? "preview",
        performanceMode: request.performanceMode ?? "snappy",
        appBackgroundTranslucencyEnabled: request.appBackgroundTranslucencyEnabled ?? false,
        appBackgroundOpacity: request.appBackgroundOpacity ?? 1,
        disableAnimations: request.disableAnimations ?? false,
        uiLayoutScale: request.uiLayoutScale ?? 1,
        navigationPlacement: request.navigationPlacement ?? "left",
        hiddenNavigationTabs: request.hiddenNavigationTabs ?? [],
        navigationTabOrder: request.navigationTabOrder ?? ["calendar", "tasks", "notes"],
        toolbarActionOrder: request.toolbarActionOrder ?? ["commandPalette", "notifications", "diagnostics", "splitPane", "refresh", "settings"],
        hiddenCalendarViewModes: request.hiddenCalendarViewModes ?? [],
        showCompletedInCalendarViews: request.showCompletedInCalendarViews ?? true,
        eventCompletionDefaultScope: request.eventCompletionDefaultScope ?? "occurrence" as const,
        calendarTimelineDensity: request.calendarTimelineDensity ?? "compact",
        monthScrollPastMonths: request.monthScrollPastMonths ?? 0,
        monthScrollFutureMonths: request.monthScrollFutureMonths ?? 1,
        quickCreateExpandedByDefault: request.quickCreateExpandedByDefault ?? false,
        restoreWindowStateEnabled: request.restoreWindowStateEnabled ?? true,
        startOnLogin: request.startOnLogin ?? false,
        selectedTaskListIds: request.selectedTaskListIds ?? [],
        selectedCalendarIds: request.selectedCalendarIds ?? [],
        storageBackend: request.storageBackend ?? "google",
        hcbHosterEndpoint: request.hcbHosterEndpoint ?? null,
        hcbVaultPath: request.hcbVaultPath ?? null,
        hcbHosterLastPackageSha256: request.hcbHosterLastPackageSha256 ?? null,
        setupCompletedAt: request.setupCompletedAt === undefined ? now : request.setupCompletedAt,
        syncMode: request.syncMode ?? "balanced",
        syncTasksEnabled: request.syncTasksEnabled ?? true,
        syncCalendarEventsEnabled: request.syncCalendarEventsEnabled ?? true,
        eventRetentionDaysBack: request.eventRetentionDaysBack ?? 0,
        completedTaskRetentionDaysBack: request.completedTaskRetentionDaysBack ?? 365,
        keybindings: request.keybindings ?? defaultKeybindings,
        leaderKey: request.leaderKey === undefined ? defaultLeaderKey : request.leaderKey,
        leaderKeybindings: request.leaderKeybindings ?? defaultLeaderKeybindings,
        showTrayIcon: request.showTrayIcon ?? true,
        trayClickAction: request.trayClickAction ?? "open-menu",
        menuBarPanelStyle: request.menuBarPanelStyle ?? "adaptive",
        menuBarIconName: request.menuBarIconName ?? "calendar",
        menuBarCalendarIconId: request.menuBarCalendarIconId ?? "calendar",
        menuBarCalendarDoneMode: request.menuBarCalendarDoneMode ?? "visibleTodayDone",
        customMenuBarIcons: request.customMenuBarIcons ?? [],
        showMenuBarBadge: request.showMenuBarBadge ?? true,
        showDockBadge: request.showDockBadge ?? true,
        notificationsEnabled: request.notificationsEnabled ?? false,
        notificationLeadMinutes: request.notificationLeadMinutes ?? 10,
        taskCompletionSoundEnabled: request.taskCompletionSoundEnabled ?? true,
        taskCompletionSoundId: request.taskCompletionSoundId ?? "glass",
        eventCompletionSoundEnabled: request.eventCompletionSoundEnabled ?? true,
        eventCompletionSoundId: request.eventCompletionSoundId ?? "pop",
        importedSoundCount: request.importedSoundCount ?? 0,
        perTabListFilters: request.perTabListFilters ?? {
          tasks: { useCustomFilter: false, selectedTaskListIds: [] },
          notes: { useCustomFilter: false, selectedTaskListIds: [] }
        },
        portableExportOnlySelectedTaskLists: request.portableExportOnlySelectedTaskLists ?? false,
        portableExportOnlySelectedCalendars: request.portableExportOnlySelectedCalendars ?? false,
        portableExportOnlyFutureCurrentEvents: request.portableExportOnlyFutureCurrentEvents ?? false,
        dailyLocalBackupEnabled: request.dailyLocalBackupEnabled ?? false,
        localBackupRetentionCount: request.localBackupRetentionCount ?? 14,
        lastLocalBackupAt: request.lastLocalBackupAt ?? null,
        visibleHistoryEntryCount: request.visibleHistoryEntryCount ?? 50,
        historyStorageCap: request.historyStorageCap ?? 5_000,
        historyCategoryVisibility: request.historyCategoryVisibility ?? defaultHistoryCategoryVisibility,
        dismissedDuplicateGroupIds: request.dismissedDuplicateGroupIds ?? [],
        taskTemplates: request.taskTemplates ?? [],
        eventTemplates: request.eventTemplates ?? [],
        noteTemplates: request.noteTemplates ?? [],
        lastUpdateCheckAt: request.lastUpdateCheckAt ?? null,
        mcpEnabled: request.mcpEnabled ?? false,
        mcpPermissionMode: request.mcpPermissionMode ?? "confirm-writes",
        mcpPort: request.mcpPort ?? 0,
        localHostersEnabled: request.localHostersEnabled ?? false,
        localHosterPort: request.localHosterPort ?? 0,
        defaultTimeZone: request.defaultTimeZone ?? "UTC",
        todayCapacityMinutes: request.todayCapacityMinutes ?? 480,
        todayWorkingHoursStart: request.todayWorkingHoursStart ?? 6,
        todayWorkingHoursEnd: request.todayWorkingHoursEnd ?? 22,
        diagnosticsIncludePerformance: request.diagnosticsIncludePerformance ?? true,
        rawGoogleDiagnosticsEnabled: request.rawGoogleDiagnosticsEnabled ?? false,
        savedSearchViews: request.savedSearchViews ?? [],
        pinnedSavedSearchViewIds: request.pinnedSavedSearchViewIds ?? [],
        savedTaskViews: request.savedTaskViews ?? [],
        semanticSearchEnabled: request.semanticSearchEnabled ?? false,
        semanticSearchMode: request.semanticSearchMode ?? "lexical",
        embeddingModelId: request.embeddingModelId ?? "Xenova/all-MiniLM-L6-v2",
        semanticSearchModels: request.semanticSearchModels ?? defaultSemanticSearchModels,
        agentActionTrayEnabled: request.agentActionTrayEnabled ?? true,
        webhooksEnabled: request.webhooksEnabled ?? false
      })
    ),
    recoveryAction: vi.fn(async (request) =>
      ok({
        action: request.action,
        accepted: true,
        destructive: request.action !== "refresh" && request.action !== "resetOnboarding",
        requiresReload: request.action === "clearGoogleCache",
        message: "Recovery action accepted."
      })
    ),
    exportPortableArchive: vi.fn(async () => {
      const exportedAt = new Date().toISOString();
      return ok({
        path: "/tmp/hcb-test.hcbexport",
        exportedAt,
        manifest: {
          formatVersion: 1 as const,
          exportedAt,
          appVersion: "0.0.0",
          stateFile: "hot-cross-buns-state.json" as const,
          stateSha256: "0".repeat(64),
          attachmentDirectory: "Attachments" as const,
          attachments: [],
          skippedPointers: [],
          notes: ["Test archive."]
        }
      });
    }),
    previewPortableImport: vi.fn(async (request) =>
      ok({
        path: request.path,
        exportedAt: new Date().toISOString(),
        formatVersion: 1 as const,
        destructive: true as const,
        tasks: { added: 0, removed: 0, changed: 0 },
        events: { added: 0, removed: 0, changed: 0 },
        calendars: { added: 0, removed: 0, changed: 0 },
        taskLists: { added: 0, removed: 0, changed: 0 },
        settingsWillChange: false,
        queuedMutationCount: 0,
        attachments: { bundled: 0, missing: 0, corrupt: 0, skipped: 0 },
        items: { tasks: [], events: [], calendars: [], taskLists: [] }
      })
    ),
    importPortableArchive: vi.fn(async (request) =>
      ok({
        importedAt: new Date().toISOString(),
        backupPath: "/tmp/hcb-test-backup.sqlite3",
        preview: {
          path: request.path,
          exportedAt: new Date().toISOString(),
          formatVersion: 1 as const,
          destructive: true as const,
          tasks: { added: 0, removed: 0, changed: 0 },
          events: { added: 0, removed: 0, changed: 0 },
          calendars: { added: 0, removed: 0, changed: 0 },
          taskLists: { added: 0, removed: 0, changed: 0 },
          settingsWillChange: false,
          queuedMutationCount: 0,
          attachments: { bundled: 0, missing: 0, corrupt: 0, skipped: 0 },
          items: { tasks: [], events: [], calendars: [], taskLists: [] }
        }
      })
    ),
    exportHcbVault: vi.fn(async (request) =>
      ok({
        path: request.out ?? "/tmp/hcb-test.hcbvault",
        exportedAt: now,
        manifest: {
          formatVersion: 1 as const,
          kind: "hot-cross-buns-vault" as const,
          exportedAt: now,
          appVersion: "0.0.0",
          stateEncoding: "hcb-portable-state-json" as const,
          stateSha256: "0".repeat(64),
          payloadFile: "payload.hcbenc" as const,
          payloadSha256: "1".repeat(64),
          encryption: {
            algorithm: "scrypt-AES-256-GCM" as const,
            kdf: "scrypt" as const,
            saltBase64: "salt",
            ivBase64: "iv",
            tagBase64: "tag",
            keyLength: 32 as const,
            cost: 32768,
            blockSize: 8,
            parallelization: 1
          },
          notes: ["Test vault."]
        }
      })
    ),
    importHcbVault: vi.fn(async () =>
      ok({
        importedAt: now,
        backupPath: "/tmp/hcb-test-backup.sqlite3",
        manifest: {
          formatVersion: 1 as const,
          kind: "hot-cross-buns-vault" as const,
          exportedAt: now,
          appVersion: "0.0.0",
          stateEncoding: "hcb-portable-state-json" as const,
          stateSha256: "0".repeat(64),
          payloadFile: "payload.hcbenc" as const,
          payloadSha256: "1".repeat(64),
          encryption: {
            algorithm: "scrypt-AES-256-GCM" as const,
            kdf: "scrypt" as const,
            saltBase64: "salt",
            ivBase64: "iv",
            tagBase64: "tag",
            keyLength: 32 as const,
            cost: 32768,
            blockSize: 8,
            parallelization: 1
          },
          notes: ["Test vault."]
        }
      })
    ),
    hcbVaultRemoteStatus: vi.fn(async (request) =>
      ok({
        endpoint: request.endpoint ?? "https://hcb.test/hcb/v1/vault",
        remote: {
          kind: "hcbVaultHostInfo" as const,
          protocolVersion: 1 as const,
          hcbVaultFormatVersions: [1 as const],
          routes: ["/hcb/v1/vault/info", "/hcb/v1/vault"],
          hasVault: true,
          vaultName: "test.hcbvault",
          maxPackageBytes: 134217728
        }
      })
    ),
    pushHcbVaultRemote: vi.fn(async (request) =>
      ok({
        endpoint: request.endpoint ?? "https://hcb.test/hcb/v1/vault",
        exportedAt: now,
        path: request.out ?? "/tmp/hcb-test.hcbvault",
        manifest: {
          formatVersion: 1 as const,
          kind: "hot-cross-buns-vault" as const,
          exportedAt: now,
          appVersion: "0.0.0",
          stateEncoding: "hcb-portable-state-json" as const,
          stateSha256: "0".repeat(64),
          payloadFile: "payload.hcbenc" as const,
          payloadSha256: "1".repeat(64),
          encryption: {
            algorithm: "scrypt-AES-256-GCM" as const,
            kdf: "scrypt" as const,
            saltBase64: "salt",
            ivBase64: "iv",
            tagBase64: "tag",
            keyLength: 32 as const,
            cost: 32768,
            blockSize: 8,
            parallelization: 1
          },
          notes: ["Test vault."]
        },
        remote: {
          kind: "hcbVaultHostInfo" as const,
          protocolVersion: 1 as const,
          hcbVaultFormatVersions: [1 as const],
          routes: ["/hcb/v1/vault/info", "/hcb/v1/vault"],
          hasVault: true,
          vaultName: "test.hcbvault",
          maxPackageBytes: 134217728
        }
      })
    ),
    pullHcbVaultRemote: vi.fn(async (request) =>
      ok({
        endpoint: request.endpoint ?? "https://hcb.test/hcb/v1/vault",
        importedAt: now,
        backupPath: "/tmp/hcb-test-backup.sqlite3",
        manifest: {
          formatVersion: 1 as const,
          kind: "hot-cross-buns-vault" as const,
          exportedAt: now,
          appVersion: "0.0.0",
          stateEncoding: "hcb-portable-state-json" as const,
          stateSha256: "0".repeat(64),
          payloadFile: "payload.hcbenc" as const,
          payloadSha256: "1".repeat(64),
          encryption: {
            algorithm: "scrypt-AES-256-GCM" as const,
            kdf: "scrypt" as const,
            saltBase64: "salt",
            ivBase64: "iv",
            tagBase64: "tag",
            keyLength: 32 as const,
            cost: 32768,
            blockSize: 8,
            parallelization: 1
          },
          notes: ["Test vault."]
        },
        remote: {
          kind: "hcbVaultHostInfo" as const,
          protocolVersion: 1 as const,
          hcbVaultFormatVersions: [1 as const],
          routes: ["/hcb/v1/vault/info", "/hcb/v1/vault"],
          hasVault: true,
          vaultName: "test.hcbvault",
          maxPackageBytes: 134217728
        }
      })
    ),
    hcbVaultRemoteCredentialStatus: vi.fn(async (request = {}) =>
      ok({
        endpoint: request.endpoint ?? "https://hcb.test/hcb/v1/vault",
        configured: false,
        secretStore: {
          ok: true,
          state: "ready",
          message: "Test credential storage is available."
        }
      })
    ),
    saveHcbVaultRemoteCredentials: vi.fn(async (request) =>
      ok({
        endpoint: request.endpoint ?? "https://hcb.test/hcb/v1/vault",
        configured: true,
        secretStore: {
          ok: true,
          state: "ready",
          message: "Test credential storage is available."
        }
      })
    ),
    deleteHcbVaultRemoteCredentials: vi.fn(async (request = {}) =>
      ok({
        endpoint: request.endpoint ?? "https://hcb.test/hcb/v1/vault",
        configured: false,
        secretStore: {
          ok: true,
          state: "ready",
          message: "Test credential storage is available."
        }
      })
    ),
    listLocalPointers: vi.fn(async () => ok({ items: [], totalKnown: 0 })),
    repairLocalPointer: vi.fn(async (request) =>
      ok({
        pointer: request.pointer,
        replacementPointer: request.replacementPath,
        updated: 0,
        queued: false,
        revision: now
      })
    ),
    customizationStatus: vi.fn(async () => ok(mockCustomizationStatus())),
    reloadCustomization: vi.fn(async () => ok(mockCustomizationStatus())),
    setSnippetEnabled: vi.fn(async () => ok(mockCustomizationStatus())),
    setExtensionEnabled: vi.fn(async () => ok(mockCustomizationStatus())),
    logExtensionMessage: vi.fn(async () => ok(mockCustomizationStatus())),
    listAttachments: vi.fn(async () => ok({ items: [] })),
    addAttachment: vi.fn(async () => ok({ items: [], queued: false, revision: now })),
    removeAttachment: vi.fn(async () => ok({ items: [], queued: false, revision: now })),
    openAttachment: vi.fn(async (request) => ok({ path: request.pointer, message: "Opened." })),
    downloadAttachment: vi.fn(async (request) => ok({ path: request.pointer, message: "Downloaded." })),
    importIcs: vi.fn(async (request) =>
      ok({
        calendarId: "ics:test",
        calendarTitle: request.calendarTitle ?? request.fileName,
        importedEventCount: 0,
        skippedEventCount: 0,
        revision: now
      })
    ),
    listIcsSubscriptions: vi.fn(async () => ok({ items: [] })),
    subscribeIcs: vi.fn(async () => ok({ items: [] })),
    refreshIcsSubscription: vi.fn(async () => ok({ items: [] })),
    deleteIcsSubscription: vi.fn(async () => ok({ items: [] })),
    exportLocalReport: vi.fn(async (request) =>
      ok({
        path: "/tmp/hcb-test-report.md",
        format: request.format ?? "markdown",
        generatedAt: now,
        itemCount: 0
      })
    )
  },
  undo: {
    status: vi.fn(async () => ok({ canUndo: false, canRedo: false })),
    undo: vi.fn(async () => ok({ action: "undo" as const, applied: false })),
    redo: vi.fn(async () => ok({ action: "redo" as const, applied: false }))
  },
  mcp: {
    status: vi.fn(async () =>
      ok({
        enabled: false,
        running: false,
        readOnly: true,
        confirmationRequired: true,
        permissionMode: "read-only" as const,
        port: 0,
        tokenState: "not_configured" as const
      })
    ),
    setEnabled: vi.fn(async (request) =>
      ok({
        enabled: request.enabled,
        running: false,
        readOnly: true,
        confirmationRequired: request.confirmationRequired ?? true,
        permissionMode: request.permissionMode ?? "read-only",
        port: request.port ?? 0,
        tokenState: "not_configured" as const
      })
    )
  },
  agent: {
    listActions: vi.fn(async (request = {}) =>
      ok({
        items: [],
        page: {
          limit: request.limit ?? 50,
          totalKnown: 0
        }
      })
    ),
    applyAction: vi.fn(async (request) =>
      ok({
        action: {
          id: request.id,
          status: "applied" as const,
          toolName: "planner.create_task",
          summary: "planner.create_task: test",
          createdAt: now,
          expiresAt: later,
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
          toolName: "planner.create_task",
          summary: "planner.create_task: test",
          createdAt: now,
          expiresAt: later,
          updatedAt: now,
          appliedAt: null,
          errorMessage: null
        }
      })
    ),
    clearExpired: vi.fn(async () => ok({ cleared: 0 }))
  },
  webhooks: {
    list: vi.fn(async (request = {}) =>
      ok({
        items: [],
        page: {
          limit: request.limit ?? 50,
          totalKnown: 0
        }
      })
    ),
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
  },
  native: {
    capabilities: vi.fn(async () =>
      ok({
        platform: "darwin" as const,
        notifications: false,
        globalShortcuts: false,
        tray: false,
        deepLinks: false,
        trayStatus: {
          state: "unsupported" as const,
          message: "Tray/menu bar is unavailable."
        },
        notificationsStatus: {
          permission: "unsupported" as const,
          scheduledCount: 0,
          state: "unsupported" as const,
          message: "Notifications are unavailable."
        },
        deepLinkStatus: {
          scheme: "hotcrossbuns" as const,
          registered: false,
          state: "unsupported" as const,
          message: "Deep links are unavailable."
        },
        updaterStatus: {
          state: "unsupported" as const,
          message: "Preview update checks are not configured."
        },
        mcpStatus: {
          state: "disabled" as const,
          message: "MCP local agent access is disabled."
        },
        capabilityReport: {
          platform: "darwin" as const,
          adapterId: "test",
          packageFormat: "development" as const,
          flags: {
            supportsAppPaths: true,
            supportsTray: false,
            supportsAppMenu: false,
            supportsGlobalShortcut: false,
            supportsNotifications: false,
            supportsNotificationPermissionQuery: false,
            supportsProtocolRegistration: false,
            supportsProtocolRegistrationCheck: false,
            supportsAutostart: false,
            supportsInPlaceAutoUpdate: false,
            supportsInstallerMetadata: false,
            supportsExternalUrlOpen: false,
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
          state: "pending" as const
        }
      })
    ),
    requestNotificationPermission: vi.fn(async () =>
      ok({
        state: "unsupported" as const
      })
    ),
    listFontFamilies: vi.fn(async () =>
      ok({
        platform: "darwin" as const,
        families: ["Avenir", "SF Pro Text"]
      })
    ),
    importMenuBarIcon: vi.fn(async (request) =>
      ok({
        id: "custom:test",
        name: request.name,
        fileName: "custom-test.png",
        createdAt: "2026-01-01T00:00:00.000Z",
        updatedAt: "2026-01-01T00:00:00.000Z"
      })
    ),
    openExternalUrl: vi.fn(async () =>
      ok({
        state: "ready" as const,
        message: "External URL opened."
      })
    ),
    subscribeAction: vi.fn(() => () => undefined)
  },
  diagnostics: {
    health: vi.fn(async () =>
      ok({
        status: "ok" as const,
        version: "0.0.0-test",
        environment: "test" as const,
        timestamp: now,
        uptimeMs: 1,
        startup: {
          processStartedMs: 0
        },
        build: {
          appName: "Hot Cross Buns",
          version: "0.0.0-test",
          environment: "test" as const,
          nodeVersion: process.versions.node,
          packaged: false
        }
      })
    ),
    markShellVisible: vi.fn(async () =>
      ok({
        processStartedMs: 0,
        shellVisibleMs: 1
      })
    ),
    markCachedDataRendered: vi.fn(async () =>
      ok({
        processStartedMs: 0,
        cachedDataRenderedMs: 2
      })
    ),
    ipcMetrics: vi.fn(async () =>
      ok({
        totalCalls: 0,
        validationFailures: 0,
        serviceFailures: 0,
        responseFailures: 0,
        routes: []
      })
    ),
    performance: vi.fn(async () =>
      ok({
        timings: []
      })
    ),
    recordTiming: vi.fn(async () =>
      ok({
        recorded: true
      })
    ),
    summary: vi.fn(async () =>
      ok({
        status: "ok" as const,
        generatedAt: now,
        account: {
          state: "signed_out" as const,
          grantedScopeCount: 0,
          missingScopeCount: 2
        },
        sync: {
          state: "idle" as const,
          pendingMutationCount: 0,
          offline: true,
          stale: true,
          mode: "balanced" as const
        },
        cache: {
          taskListCount: 0,
          taskCount: 0,
          calendarCount: 0,
          eventCount: 0,
          noteCount: 0,
          performanceSampleCount: 0,
          migrationVersion: 2,
          migrationDurationMs: 0
        },
        selectedResources: {
          taskLists: [],
          calendars: []
        },
        checkpoints: {
          totalCount: 0,
          tasksCount: 0,
          calendarCount: 0
        },
        pendingMutations: {
          totalCount: 0,
          pendingCount: 0,
          applyingCount: 0,
          failedCount: 0,
          retryableCount: 0,
          authPausedCount: 0,
          byResourceType: []
        },
        mcp: {
          enabled: false,
          running: false,
          permissionMode: "read-only" as const,
          confirmationRequired: true,
          port: 0,
          tokenState: "not_configured" as const,
          requestCounts: {
            totalRequests: 0,
            successCount: 0,
            rejectedCount: 0,
            errorCount: 0,
            rateLimitedCount: 0
          }
        },
        native: {
          platform: "darwin" as const,
          adapterId: "test",
          packageFormat: "development" as const,
          flags: {
            supportsAppPaths: true,
            supportsTray: false,
            supportsAppMenu: false,
            supportsGlobalShortcut: false,
            supportsNotifications: false,
            supportsNotificationPermissionQuery: false,
            supportsProtocolRegistration: false,
            supportsProtocolRegistrationCheck: false,
            supportsAutostart: false,
            supportsInPlaceAutoUpdate: false,
            supportsInstallerMetadata: false,
            supportsExternalUrlOpen: false,
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
        build: {
          appName: "Hot Cross Buns",
          version: "0.0.0-test",
          environment: "test" as const,
          nodeVersion: process.versions.node,
          packaged: false
        },
        performance: {
          startup: {
            processStartedMs: 0
          },
          migrationDurationMs: 0,
          slowQuerySamples: [],
          pendingMutationCounts: {
            totalCount: 0,
            failedCount: 0
          },
          mcpRequestCounts: {
            totalRequests: 0,
            successCount: 0,
            rejectedCount: 0,
            errorCount: 0,
            rateLimitedCount: 0
          }
        },
        redaction: {
          credentials: "redacted" as const,
          googlePayloads: "omitted" as const,
          mcpBearerTokens: "redacted" as const,
          sensitiveBodies: "omitted" as const
        }
      })
    ),
    logs: vi.fn(async () =>
      ok({
        entries: [],
        retainedEntryCount: 0,
        persistedText: ""
      })
    ),
    clearLogs: vi.fn(async () =>
      ok({
        clearedAt: now
      })
    ),
    revealLogsFolder: vi.fn(async () =>
      ok({
        opened: true,
        path: "/tmp/hot-cross-buns-test/logs",
        message: "Logs folder opened."
      })
    ),
    history: vi.fn(async () =>
      ok({
        entries: [],
        retainedEntryCount: 0
      })
    ),
    pendingMutations: vi.fn(async () =>
      ok({
        mutations: []
      })
    ),
    retryPendingMutation: vi.fn(async (request) =>
      ok({
        id: request.id,
        status: "pending" as const,
        updatedAt: now
      })
    ),
    cancelPendingMutation: vi.fn(async (request) =>
      ok({
        id: request.id,
        status: "cancelled" as const,
        updatedAt: now
      })
    ),
    copyableSummary: vi.fn(async () =>
      ok({
        text: "Hot Cross Buns diagnostics\nStatus: ok",
        generatedAt: now
      })
    ),
    exportBundle: vi.fn(async () =>
      ok({
        exported: true,
        path: "/tmp/hot-cross-buns-test/diagnostics.txt",
        message: "Diagnostic bundle exported."
      })
    ),
    rescheduleNotifications: vi.fn(async () =>
      ok({
        status: {
          state: "unsupported" as const,
          permission: "unsupported" as const,
          scheduledCount: 0
        },
        message: "Notification scheduling is unavailable in tests."
      })
    )
  }
};

function mockCustomizationStatus() {
  return {
    configDirectory: "/tmp/hcb-test",
    snippetsDirectory: "/tmp/hcb-test/snippets",
    extensionsDirectory: "/tmp/hcb-test/extensions",
    settingsJsonPath: "/tmp/hcb-test/settings.json",
    keymapJsonPath: "/tmp/hcb-test/keymap.json",
    snippets: [],
    externalSettings: {
      path: "/tmp/hcb-test/settings.json",
      exists: false,
      valid: true,
      appliedKeys: [],
      conflicts: [],
      error: null
    },
    externalKeymap: {
      path: "/tmp/hcb-test/keymap.json",
      exists: false,
      valid: true,
      appliedKeys: [],
      conflicts: [],
      error: null
    },
    extensions: [],
    safeMode: false
  };
}

if (typeof window !== "undefined") {
  Object.defineProperty(window, "hcb", {
    configurable: true,
    value: hcbApi
  });
}
