import { describe, expect, it, vi } from "vitest";
import type {
  CalendarEventSummary,
  NativeAction,
  SettingsSnapshot,
  TaskSummary
} from "@shared/ipc/contracts";
import { NativeShellService, parseHotCrossBunsDeepLink } from "./service";
import {
  buildNativeCapabilityReport,
  defaultNativeAppPaths
} from "./capabilityReport";
import type {
  NativeAppPaths,
  NativeNotificationRequest,
  NativeOperationResult,
  NativePlatformAdapter,
  NativePlatformCapabilities,
  NativeTrayActions,
  ScheduledNativeNotification
} from "./types";
import {
  defaultHistoryCategoryVisibility,
  defaultKeybindings,
  defaultLeaderKey,
  defaultLeaderKeybindings
} from "@shared/settingsCatalog";
import { defaultSemanticSearchModels } from "@shared/ipc/contracts";

const now = new Date("2026-05-22T12:00:00.000Z");

async function flushNativeStartup(): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 0));
  await Promise.resolve();
  await Promise.resolve();
}

function defaultSettings(overrides: Partial<SettingsSnapshot> = {}): SettingsSnapshot {
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
    setupCompletedAt: "2026-05-22T00:00:00.000Z",
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
    notificationsEnabled: true,
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

class FakeNativeAdapter implements NativePlatformAdapter {
  appPathsValue: NativeAppPaths = defaultNativeAppPaths();
  capabilitiesValue: NativePlatformCapabilities = {
    platform: "darwin",
    adapterId: "fake",
    notifications: true,
    globalShortcuts: true,
    tray: true,
    deepLinks: true,
    updaterChecks: false,
    capabilityReport: buildNativeCapabilityReport({
      platform: "darwin",
      adapterId: "fake",
      appPaths: defaultNativeAppPaths(),
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
      }
    })
  };
  shortcutResult: NativeOperationResult = {
    ok: true,
    state: "ready",
    message: "registered"
  };
  updateResult: NativeOperationResult = {
    ok: false,
    state: "unsupported",
    message: "not configured"
  };
  trayResult: NativeOperationResult = {
    ok: true,
    state: "ready",
    message: "tray"
  };
  trayActions: NativeTrayActions | undefined;
  appMenuActions: NativeTrayActions | undefined;
  registeredShortcuts: string[] = [];
  unregisteredShortcuts: string[] = [];
  shortcutActions = new Map<string, () => void>();
  protocolSchemes: string[] = [];
  scheduledNotifications: NativeNotificationRequest[] = [];
  notificationFailureCallbacks: Array<(message: string) => void> = [];
  autostartRequests: boolean[] = [];
  fontFamilies: string[] = ["Avenir", "SF Pro Text"];
  trayCreateCount = 0;
  trayDestroyCount = 0;

  appPaths(): NativeAppPaths {
    return this.appPathsValue;
  }

  capabilities(): NativePlatformCapabilities {
    return this.capabilitiesValue;
  }

  credentialStorageStatus(): NativeOperationResult {
    return {
      ok: false,
      state: "unsupported",
      message: "credentials"
    };
  }

  installAppMenu(actions: NativeTrayActions): NativeOperationResult {
    this.appMenuActions = actions;
    return { ok: true, state: "ready", message: "menu" };
  }

  createTray(actions: NativeTrayActions): NativeOperationResult {
    this.trayActions = actions;
    this.trayCreateCount += 1;
    return this.trayResult;
  }

  destroyTray(): void {
    this.trayDestroyCount += 1;
  }

  registerGlobalShortcut(accelerator: string, action: () => void): NativeOperationResult {
    this.registeredShortcuts.push(accelerator);
    this.shortcutActions.set(accelerator, action);
    return this.shortcutResult;
  }

  unregisterGlobalShortcut(accelerator?: string): void {
    if (accelerator) {
      this.unregisteredShortcuts.push(accelerator);
      this.shortcutActions.delete(accelerator);
    }
  }

  registerProtocolClient(scheme: "hotcrossbuns"): NativeOperationResult {
    this.protocolSchemes.push(scheme);
    return { ok: true, state: "ready", message: "protocol" };
  }

  requestNotificationPermission() {
    return { state: "prompt" as const };
  }

  listFontFamilies(): string[] {
    return this.fontFamilies;
  }

  scheduleNotification(
    request: NativeNotificationRequest,
    _onClick: () => void,
    onFailure?: (message: string) => void
  ): ScheduledNativeNotification {
    this.scheduledNotifications.push(request);
    if (onFailure) {
      this.notificationFailureCallbacks.push(onFailure);
    }

    return {
      id: request.id,
      cancel: () => undefined
    };
  }

  clearScheduledNotifications(): void {
    this.scheduledNotifications = [];
    this.notificationFailureCallbacks = [];
  }

  setAutostart(enabled: boolean): NativeOperationResult {
    this.autostartRequests.push(enabled);
    return {
      ok: true,
      state: "ready",
      message: enabled ? "Open-at-login is enabled." : "Open-at-login is disabled."
    };
  }

  autostartStatus(): NativeOperationResult {
    return {
      ok: true,
      state: "disabled",
      message: "Open-at-login is disabled."
    };
  }

  checkForUpdates(): NativeOperationResult {
    return this.updateResult;
  }

  openExternalUrl(): NativeOperationResult {
    return {
      ok: true,
      state: "ready",
      message: "opened"
    };
  }

  openPath(): NativeOperationResult {
    return {
      ok: true,
      state: "ready",
      message: "opened"
    };
  }

  collectDiagnostics(): NativeOperationResult {
    return {
      ok: true,
      state: "ready",
      message: "diagnostics"
    };
  }

  dispose(): void {
    this.registeredShortcuts = [];
    this.shortcutActions.clear();
    this.scheduledNotifications = [];
    this.notificationFailureCallbacks = [];
  }
}

function createService(input: {
  adapter?: FakeNativeAdapter;
  settings?: SettingsSnapshot;
  tasks?: TaskSummary[];
  events?: CalendarEventSummary[];
  dispatch?: (action: NativeAction) => void;
  webhookEmit?: (...args: unknown[]) => void;
  recordUpdateCheck?: (checkedAt: string) => void;
} = {}) {
  const adapter = input.adapter ?? new FakeNativeAdapter();
  const settings = { current: input.settings ?? defaultSettings() };
  const dispatch = input.dispatch ?? vi.fn();
  const showMainWindow = vi.fn();
  const sync = vi.fn();
  const service = new NativeShellService({
    adapter,
    planner: {
      listTasks: () => ({ items: input.tasks ?? [] }),
      listCalendarEvents: () => ({ items: input.events ?? [] })
    },
    settings: {
      get: () => settings.current
    },
    recordUpdateCheck: input.recordUpdateCheck,
    windows: {
      showMainWindow,
      hideMainWindow: vi.fn(),
      showOrHideMainWindow: vi.fn(),
      quit: vi.fn(),
      dispatchAction: dispatch
    },
    sync: {
      runNow: sync
    },
    webhooks: input.webhookEmit
      ? {
          emit: (event, payload) => {
            input.webhookEmit?.(event, payload);
          }
        }
      : undefined,
    now: () => now
  });

  return { adapter, service, settings, dispatch, showMainWindow, sync };
}

describe("native shell service", () => {
  it("records and exposes manual GitHub release check status", async () => {
    const checkedAt = "2026-05-22T12:00:00.000Z";
    const recordUpdateCheck = vi.fn();
    const adapter = new FakeNativeAdapter();
    adapter.updateResult = {
      checkedAt,
      downloadUrl: "https://github.com/gongahkia/hot-cross-buns/releases/download/v1.2.0/HotCrossBuns.dmg",
      latestVersion: "1.2.0",
      ok: true,
      releaseName: "v1.2.0",
      releaseUrl: "https://github.com/gongahkia/hot-cross-buns/releases/tag/v1.2.0",
      state: "ready",
      updateAvailable: true,
      message: "Hot Cross Buns 1.2.0 is available from GitHub Releases."
    };
    const { service } = createService({ adapter, recordUpdateCheck });

    await expect(service.checkForUpdates()).resolves.toMatchObject({
      checkedAt,
      latestVersion: "1.2.0",
      updateAvailable: true
    });
    expect(recordUpdateCheck).toHaveBeenCalledWith(checkedAt);
    expect(service.capabilities().updaterStatus).toMatchObject({
      releaseUrl: "https://github.com/gongahkia/hot-cross-buns/releases/tag/v1.2.0",
      updateAvailable: true
    });
  });

  it("defers tray, hotkey, notifications, protocol, updater, and MCP startup", async () => {
    const { adapter, service } = createService({
      tasks: [
        {
          id: "task-1",
          listId: "inbox",
          title: "Pay invoice",
          status: "active",
          dueAt: "2026-05-22T00:00:00.000Z",
          updatedAt: now.toISOString(),
          priority: "none"
        }
      ],
      events: [
        {
          id: "event-1",
          calendarId: "cal-1",
          title: "Standup",
          startsAt: "2026-05-22T12:20:00.000Z",
          endsAt: "2026-05-22T12:40:00.000Z",
          allDay: false,
          updatedAt: now.toISOString(),
          reminderMinutes: []
        }
      ]
    });

    expect(service.capabilities().deferredStartup.state).toBe("pending");
    expect(adapter.trayCreateCount).toBe(0);
    expect(adapter.registeredShortcuts).toEqual([]);

    service.startDeferredStartup();
    await flushNativeStartup();

    expect(adapter.trayCreateCount).toBe(1);
    expect(adapter.registeredShortcuts).toEqual([]);
    expect(adapter.protocolSchemes).toEqual(["hotcrossbuns"]);
    expect(adapter.scheduledNotifications.map((request) => request.id)).toEqual([
      "task:task-1",
      "event:event-1"
    ]);
    expect(service.capabilities()).toMatchObject({
      deferredStartup: {
        state: "complete"
      },
      notificationsStatus: {
        scheduledCount: 2,
        state: "ready"
      }
    });
  });

  it("registers configured global quick add shortcut and dispatches native action", async () => {
    const quickAddShortcut = "CommandOrControl+Shift+Space";
    const replacementShortcut = "CommandOrControl+Alt+Space";
    const { adapter, dispatch, service, settings, showMainWindow } = createService({
      settings: defaultSettings({
        keybindings: {
          ...defaultKeybindings,
          "quickAdd.open": quickAddShortcut
        }
      })
    });
    adapter.shortcutResult = {
      ok: true,
      state: "ready"
    };

    service.startDeferredStartup();
    await flushNativeStartup();

    expect(adapter.registeredShortcuts).toEqual([quickAddShortcut]);
    expect(
      service.capabilities().capabilityReport.capabilities.find(
        (capability) => capability.key === "globalShortcuts"
      )
    ).toMatchObject({
      state: "ready",
      message: `${quickAddShortcut} is registered for global quick add.`
    });

    adapter.shortcutActions.get(quickAddShortcut)?.();
    expect(showMainWindow).toHaveBeenCalledTimes(1);
    expect(dispatch).toHaveBeenCalledWith({ type: "openQuickAdd" });

    settings.current = defaultSettings({
      keybindings: {
        ...defaultKeybindings,
        "quickAdd.open": replacementShortcut
      }
    });
    service.applySettings(settings.current);

    expect(adapter.unregisteredShortcuts).toContain(quickAddShortcut);
    expect(adapter.registeredShortcuts).toEqual([quickAddShortcut, replacementShortcut]);

    settings.current = defaultSettings({
      keybindings: {
        ...defaultKeybindings,
        "quickAdd.open": null
      }
    });
    service.applySettings(settings.current);

    expect(adapter.unregisteredShortcuts).toContain(replacementShortcut);
    expect(
      service.capabilities().capabilityReport.capabilities.find(
        (capability) => capability.key === "globalShortcuts"
      )
    ).toMatchObject({
      state: "disabled",
      message: "Global quick add shortcut is not configured."
    });

    service.dispose();
    expect(adapter.shortcutActions.size).toBe(0);
  });

  it("emits event-starting webhooks when event notifications are scheduled", async () => {
    const webhookEmit = vi.fn();
    const { service } = createService({
      webhookEmit,
      events: [
        {
          id: "event-1",
          calendarId: "cal-1",
          title: "Standup",
          startsAt: "2026-05-22T12:20:00.000Z",
          endsAt: "2026-05-22T12:40:00.000Z",
          allDay: false,
          updatedAt: now.toISOString(),
          reminderMinutes: []
        }
      ]
    });

    service.startDeferredStartup();
    await flushNativeStartup();

    expect(webhookEmit).toHaveBeenCalledWith("event.starting", {
      id: "event-1",
      title: "Standup",
      notificationDeliveryAt: "2026-05-22T12:10:00.000Z"
    });
  });

  it("routes tray menu actions without renderer-native calls", async () => {
    const { adapter, dispatch, service, sync } = createService();

    service.startDeferredStartup();
    await flushNativeStartup();
    adapter.trayActions?.openSettings();
    adapter.trayActions?.refresh();

    expect(dispatch).toHaveBeenCalledWith({ type: "openSettings" });
    expect(dispatch).toHaveBeenCalledWith({ type: "refresh" });
    expect(sync).toHaveBeenCalledWith({
      resources: ["tasks", "calendar"],
      full: false,
      dryRun: false
    });
  });

  it("lists sanitized native font families without exposing renderer privileges", async () => {
    const adapter = new FakeNativeAdapter();
    adapter.fontFamilies = ["SF Pro Text", " Avenir ", "", "Avenir"];
    const { service } = createService({ adapter });

    await expect(service.listFontFamilies()).resolves.toEqual({
      platform: "darwin",
      families: ["Avenir", "SF Pro Text"]
    });
  });

  it("builds an adaptive menu bar snapshot from cached tasks and events", async () => {
    const { adapter, service } = createService({
      tasks: [
        {
          id: "task-overdue",
          listId: "inbox",
          title: "File overdue report",
          status: "active",
          dueAt: "2026-05-21T01:00:00.000Z",
          updatedAt: now.toISOString(),
          priority: "high"
        },
        {
          id: "task-today",
          listId: "inbox",
          title: "Send launch notes",
          status: "active",
          dueAt: "2026-05-22T02:00:00.000Z",
          updatedAt: now.toISOString(),
          priority: "none"
        }
      ],
      events: [
        {
          id: "event-today",
          calendarId: "cal-1",
          title: "Release sync",
          startsAt: "2026-05-22T13:00:00.000Z",
          endsAt: "2026-05-22T13:30:00.000Z",
          allDay: false,
          updatedAt: now.toISOString(),
          reminderMinutes: []
        }
      ]
    });

    service.startDeferredStartup();
    await flushNativeStartup();

    const snapshot = adapter.trayActions?.snapshot();

    expect(snapshot).toMatchObject({
      panelStyle: "adaptive",
      primaryClickAction: "open-menu",
      title: "Agenda",
      statusLabel: "Release sync - in 1h",
      syncLabel: "Local",
      badgeLabel: "1"
    });
    expect(snapshot?.sections.map((section) => section.title)).toEqual(["Today", "Tomorrow"]);
    expect(snapshot?.sections.flatMap((section) => section.items.map((item) => item.label))).toEqual(
      expect.arrayContaining(["Release sync"])
    );
  });

  it("builds a calendar-style menu bar snapshot with month and selected day data", async () => {
    const { adapter, service } = createService({
      settings: defaultSettings({ menuBarPanelStyle: "calendar", menuBarIconName: "calendar" }),
      tasks: [
        {
          id: "task-today",
          listId: "inbox",
          title: "Send launch notes",
          status: "active",
          dueAt: "2026-05-22T02:00:00.000Z",
          updatedAt: now.toISOString(),
          priority: "none"
        }
      ],
      events: [
        {
          id: "event-today",
          calendarId: "cal-1",
          title: "Release sync",
          startsAt: "2026-05-22T13:00:00.000Z",
          endsAt: "2026-05-22T13:30:00.000Z",
          allDay: false,
          updatedAt: now.toISOString(),
          reminderMinutes: []
        }
      ]
    });

    service.startDeferredStartup();
    await flushNativeStartup();

    const snapshot = adapter.trayActions?.snapshot();

    expect(snapshot).toMatchObject({
      panelStyle: "calendar",
      iconName: "calendar",
      title: "Calendar",
      syncLabel: "Local",
      calendar: {
        monthLabel: "May 2026",
        selectedMeta: "1 event - 1 task"
      }
    });
    expect(snapshot?.calendar?.days).toHaveLength(42);
    expect(snapshot?.calendar?.selectedItems.map((item) => item.label)).toEqual([
      "Release sync",
      "Send launch notes"
    ]);
  });

  it("parses and dispatches custom protocol deep links safely", () => {
    expect(parseHotCrossBunsDeepLink("hotcrossbuns://task/task-1")).toEqual({
      type: "openRoute",
      route: {
        kind: "task",
        id: "task-1"
      }
    });
    expect(parseHotCrossBunsDeepLink("hotcrossbuns://search?q=invoice")).toEqual({
      type: "openRoute",
      route: {
        kind: "search",
        query: "invoice"
      }
    });
    expect(parseHotCrossBunsDeepLink("https://example.com/task/task-1")).toBeNull();
    expect(parseHotCrossBunsDeepLink("hotcrossbuns://task/%E0%A4%A")).toBeNull();
    expect(parseHotCrossBunsDeepLink(`hotcrossbuns://task/${"a".repeat(257)}`)).toBeNull();
    expect(parseHotCrossBunsDeepLink(`hotcrossbuns://search?q=${"a".repeat(201)}`)).toBeNull();

    const dispatch = vi.fn();
    const { service } = createService({ dispatch });

    expect(service.handleDeepLink("hotcrossbuns://event/event-1")).toBe(true);
    expect(dispatch).toHaveBeenCalledWith({
      type: "openRoute",
      route: {
        kind: "event",
        id: "event-1"
      }
    });
  });

  it("keeps disabled tray and notification settings reflected in native status", async () => {
    const { adapter, service } = createService({
      settings: defaultSettings({
        showTrayIcon: false,
        notificationsEnabled: false
      })
    });

    service.startDeferredStartup();
    await flushNativeStartup();

    expect(adapter.trayCreateCount).toBe(0);
    expect(adapter.scheduledNotifications).toEqual([]);
    expect(service.capabilities()).toMatchObject({
      trayStatus: {
        state: "disabled"
      },
      notificationsStatus: {
        state: "disabled",
        scheduledCount: 0
      }
    });
  });

  it("marks native notification delivery failures as sanitized recoverable errors", async () => {
    const { adapter, service } = createService({
      tasks: [
        {
          id: "task-1",
          listId: "inbox",
          title: "Pay invoice",
          status: "active",
          dueAt: "2026-05-22T00:00:00.000Z",
          updatedAt: now.toISOString(),
          priority: "none"
        }
      ]
    });

    service.startDeferredStartup();
    await flushNativeStartup();

    expect(service.capabilities().notificationsStatus).toMatchObject({
      scheduledCount: 1,
      state: "ready"
    });

    adapter.notificationFailureCallbacks[0](
      "access_token=fake-token failed under /Users/example/Library"
    );

    const message = service.capabilities().notificationsStatus.message ?? "";

    expect(service.capabilities().notificationsStatus).toMatchObject({
      scheduledCount: 1,
      state: "error"
    });
    expect(message).not.toContain("fake-token");
    expect(message).not.toContain("/Users/example");
    expect(message).toContain("[REDACTED]");
  });

  it("applies settings changes to tray, notifications, and MCP status", async () => {
    const { adapter, service, settings } = createService();

    service.startDeferredStartup();
    await flushNativeStartup();

    settings.current = defaultSettings({
      showTrayIcon: false,
      notificationsEnabled: false,
      mcpEnabled: true,
      mcpPort: 7331
    });
    service.applySettings(settings.current);

    expect(adapter.unregisteredShortcuts).toEqual([]);
    expect(adapter.registeredShortcuts).toEqual([]);
    expect(adapter.trayDestroyCount).toBe(1);
    expect(service.capabilities()).toMatchObject({
      trayStatus: {
        state: "disabled"
      },
      notificationsStatus: {
        state: "disabled",
        scheduledCount: 0
      },
      mcpStatus: {
        state: "pending"
      }
    });

    settings.current = defaultSettings({
      showTrayIcon: true,
      notificationsEnabled: false
    });
    service.applySettings(settings.current);

    expect(adapter.unregisteredShortcuts).toEqual([]);
    expect(adapter.trayCreateCount).toBe(2);
  });

  it("redacts native adapter status messages before exposing them", async () => {
    const adapter = new FakeNativeAdapter();
    adapter.trayResult = {
      ok: false,
      state: "error",
      message: "access_token=fake-token failed under /Users/example/Library"
    };
    const { service } = createService({ adapter });

    service.startDeferredStartup();
    await flushNativeStartup();

    const message = service.capabilities().trayStatus.message ?? "";

    expect(message).not.toContain("fake-token");
    expect(message).not.toContain("/Users/example");
    expect(message).toContain("[REDACTED]");
  });

  it("exposes a detailed adapter capability report without platform-specific renderer logic", async () => {
    const { service } = createService();

    service.startDeferredStartup();
    await flushNativeStartup();

    const report = service.capabilities().capabilityReport;

    expect(report).toMatchObject({
      adapterId: "fake",
      platform: "darwin",
      flags: {
        supportsTray: true,
        supportsGlobalShortcut: true,
        supportsAutostart: true,
        supportsCredentialStorage: false
      }
    });
    expect(report.capabilities.map((capability) => capability.key)).toEqual(
      expect.arrayContaining([
        "appPaths",
        "credentialStorage",
        "tray",
        "appMenu",
        "globalShortcuts",
        "notifications",
        "customProtocol",
        "autostart",
        "updater",
        "installerMetadata",
        "externalOpen",
        "diagnostics",
        "oauthLoopback",
        "mcpLoopback",
        "packaging"
      ])
    );
    expect(report.capabilities).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ key: "tray", label: "Tray icon" }),
        expect.objectContaining({ key: "globalShortcuts", label: "Global shortcuts" }),
        expect.objectContaining({ key: "oauthLoopback", label: "OAuth loopback" }),
        expect.objectContaining({ key: "credentialStorage", label: "Credential storage" }),
        expect.objectContaining({ key: "notifications", label: "Notifications" }),
        expect.objectContaining({ key: "customProtocol", label: "Protocol registration" }),
        expect.objectContaining({ key: "mcpLoopback", label: "MCP loopback" })
      ])
    );
    expect(report.paths.map((path) => path.role)).toEqual(
      expect.arrayContaining(["config", "data", "cache", "logs", "diagnostics", "temp"])
    );
  });

  it("routes open-at-login settings through the native adapter contract", async () => {
    const { adapter, service, settings } = createService();

    service.startDeferredStartup();
    await flushNativeStartup();

    settings.current = defaultSettings({ startOnLogin: true });
    service.applySettings(settings.current);

    expect(adapter.autostartRequests).toContain(true);
    expect(
      service
        .capabilities()
        .capabilityReport.capabilities.find((capability) => capability.key === "autostart")
    ).toMatchObject({
      state: "ready",
      message: "Open-at-login is enabled."
    });
  });
});
