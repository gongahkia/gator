import type {
  NativeCapabilitiesResponse,
  NativeFontFamiliesResponse,
  NativeNotificationPermissionResponse,
  NativeRoute,
  SettingsSnapshot
} from "@shared/ipc/contracts";
import type { GoogleAccountConnectionStatusDto } from "../google";
import { appLogger } from "../diagnostics/appLogger";
import type { DomainJsonObject, NativeDomainService, WebhookDomainService } from "../services/domainInterfaces";
import { parseHotCrossBunsDeepLink } from "./deepLinks";
import { normalizeFontFamilies } from "./fontFamilies";
import { buildNativeMenuBarSnapshot } from "./menuBarSnapshot";
import { buildNativeNotificationRequests } from "./notificationScheduling";
import {
  featureStatus,
  initialStatus,
  isPromise,
  messageFromError,
  sanitizedNativeMessage,
  statusFromResult,
  updateCapabilityReportStatus
} from "./status";
import {
  HCB_DEEP_LINK_SCHEME,
  type NativeMenuBarSnapshot,
  type NativeOperationResult,
  type NativePlannerSnapshotSource,
  type NativePlatformAdapter,
  type NativeSettingsSource,
  type NativeShellSyncActions,
  type NativeShellWindowActions,
  type NativeTrayActions
} from "./types";

export { parseHotCrossBunsDeepLink } from "./deepLinks";

export interface NativeShellServiceOptions {
  adapter: NativePlatformAdapter;
  planner: NativePlannerSnapshotSource;
  account?: {
    latest: () => GoogleAccountConnectionStatusDto | null;
  };
  settings: NativeSettingsSource;
  recordUpdateCheck?: (checkedAt: string) => void;
  windows: NativeShellWindowActions;
  sync: NativeShellSyncActions;
  webhooks?: Pick<WebhookDomainService, "emit">;
  now?: () => Date;
}

export class NativeShellService implements NativeDomainService {
  private readonly now: () => Date;
  private status: NativeCapabilitiesResponse;
  private deferredStarted = false;
  private readonly eventStartingWebhookKeys = new Set<string>();
  private registeredGlobalShortcut: string | null = null;

  constructor(private readonly options: NativeShellServiceOptions) {
    this.now = options.now ?? (() => new Date());
    this.status = initialStatus(options.adapter.capabilities(), options.settings.get());
  }

  installAppMenu(): void {
    this.options.adapter.installAppMenu(this.actions());
  }

  startDeferredStartup(): void {
    if (this.deferredStarted) {
      return;
    }

    this.deferredStarted = true;
    this.status = {
      ...this.status,
      deferredStartup: {
        state: "running",
        startedAt: this.now().toISOString()
      }
    };

    setTimeout(() => {
      void this.runDeferredStartup();
    }, 0);
  }

  applySettings(snapshot: SettingsSnapshot): void {
    this.status = {
      ...this.status,
      mcpStatus: snapshot.mcpEnabled
        ? featureStatus("pending", "MCP listener startup is managed by the main runtime service.")
        : featureStatus("disabled", "MCP local agent access is disabled.")
    };

    if (!snapshot.showTrayIcon) {
      this.options.adapter.destroyTray();
      this.status = {
        ...this.status,
        trayStatus: featureStatus("disabled", "Menu bar icon is disabled in Settings.")
      };
    } else if (this.deferredStarted && this.status.trayStatus.state === "disabled") {
      this.setupTray();
    } else if (this.deferredStarted && this.status.trayStatus.state === "ready") {
      this.setupTray();
    }

    if (!snapshot.notificationsEnabled) {
      this.options.adapter.clearScheduledNotifications();
      this.status = {
        ...this.status,
        notificationsStatus: {
          ...this.status.notificationsStatus,
          scheduledCount: 0,
          state: "disabled",
          message: "Local notifications are disabled in Settings."
        }
      };
    } else if (this.deferredStarted) {
      this.scheduleNotificationsFromCache();
    }

    if (this.deferredStarted) {
      this.syncGlobalShortcut(snapshot);
      this.applyAutostartSetting(snapshot.startOnLogin);
    }

    if (!this.deferredStarted) {
      return;
    }
  }

  capabilities(): NativeCapabilitiesResponse {
    return structuredClone(this.status);
  }

  async listFontFamilies(): Promise<NativeFontFamiliesResponse> {
    const families = normalizeFontFamilies(await this.options.adapter.listFontFamilies());

    return {
      platform: this.status.platform,
      families
    };
  }

  requestNotificationPermission(): NativeNotificationPermissionResponse {
    const permission = this.options.adapter.requestNotificationPermission();

    this.status = {
      ...this.status,
      notificationsStatus: {
        ...this.status.notificationsStatus,
        permission: permission.state,
        state:
          permission.state === "unsupported"
            ? this.status.notifications
              ? "ready"
              : "unsupported"
            : permission.state === "denied"
              ? "error"
              : "ready",
        message:
          permission.state === "unsupported"
            ? this.status.notifications
              ? "Notification permission state cannot be queried on this platform; delivery is validated when reminders are shown."
              : "Local notifications are not supported by this platform adapter."
            : permission.state === "denied"
              ? "Notification permission was denied by the operating system."
              : "Notification permission request was handed to the operating system."
      }
    };

    return permission;
  }

  async checkForUpdates(): Promise<NativeCapabilitiesResponse["updaterStatus"]> {
    const result = this.options.adapter.checkForUpdates();
    const resolved = isPromise(result) ? await result : result;
    this.applyUpdateStatus(resolved);
    if (resolved.checkedAt) {
      this.options.recordUpdateCheck?.(resolved.checkedAt);
    }
    return structuredClone(this.status.updaterStatus);
  }

  async openExternalUrl(request: { url: string }): Promise<NativeCapabilitiesResponse["updaterStatus"]> {
    const result = this.options.adapter.openExternalUrl(request.url);
    const resolved = isPromise(result) ? await result : result;

    return statusFromResult(
      resolved,
      "ready",
      "External URL was opened by the operating system."
    );
  }

  rescheduleNotificationsForDiagnostics(): NativeCapabilitiesResponse["notificationsStatus"] {
    this.scheduleNotificationsFromCache();

    return structuredClone(this.status.notificationsStatus);
  }

  handleDeepLink(rawUrl: string): boolean {
    const action = parseHotCrossBunsDeepLink(rawUrl);

    if (!action) {
      this.status = {
        ...this.status,
        deepLinkStatus: {
          ...this.status.deepLinkStatus,
          state: "error",
          message: "Ignored malformed hotcrossbuns deep link."
        }
      };
      return false;
    }

    this.options.windows.showMainWindow();
    this.options.windows.dispatchAction(action);
    this.status = {
      ...this.status,
      deepLinkStatus: {
        ...this.status.deepLinkStatus,
        state: this.status.deepLinkStatus.registered ? "ready" : this.status.deepLinkStatus.state,
        message: "Last deep link was routed to the renderer through preload."
      }
    };

    return true;
  }

  dispose(): void {
    if (this.registeredGlobalShortcut) {
      this.options.adapter.unregisterGlobalShortcut(this.registeredGlobalShortcut);
      this.registeredGlobalShortcut = null;
    }
    this.options.adapter.dispose();
  }

  private async runDeferredStartup(): Promise<void> {
    try {
      this.setupTray();
      const settings = this.options.settings.get();
      this.registerProtocolClient();
      this.syncGlobalShortcut(settings);
      this.scheduleNotificationsFromCache();
      if (shouldRunAutomaticUpdateCheck(settings, this.now())) {
        await this.checkForUpdates();
      } else {
        this.status = {
          ...this.status,
          updaterStatus: {
            state: settings.lastUpdateCheckAt === null ? "disabled" : "ready",
            ...(settings.lastUpdateCheckAt ? { checkedAt: settings.lastUpdateCheckAt } : {}),
            message: settings.lastUpdateCheckAt === null
              ? "Automatic GitHub release checks are disabled."
              : "Automatic GitHub release check skipped; last check was within 24 hours."
          }
        };
      }
      this.applyAutostartSetting(settings.startOnLogin);
      this.updateMcpDeferredStatus(settings);
      this.status = {
        ...this.status,
        deferredStartup: {
          ...this.status.deferredStartup,
          state: "complete",
          completedAt: this.now().toISOString()
        }
      };
    } catch (error) {
      this.status = {
        ...this.status,
        deferredStartup: {
          ...this.status.deferredStartup,
          state: "error",
          message: messageFromError(error, "Deferred native startup failed.")
        }
      };
    }
  }

  private actions(): NativeTrayActions {
    return {
      primaryClick: () => this.handleTrayPrimaryAction(),
      openMainWindow: this.options.windows.showMainWindow,
      showOrHideMainWindow: this.options.windows.showOrHideMainWindow,
      openQuickAdd: () => {
        this.options.windows.showMainWindow();
        this.options.windows.dispatchAction({ type: "openQuickAdd" });
      },
      refresh: () => {
        this.options.windows.showMainWindow();
        this.options.windows.dispatchAction({ type: "refresh" });
        void this.options.sync.runNow({
          resources: ["tasks", "calendar"],
          full: false,
          dryRun: false
        });
      },
      openSettings: () => {
        this.options.windows.showMainWindow();
        this.options.windows.dispatchAction({ type: "openSettings" });
      },
      openRoute: (route: NativeRoute) => {
        this.options.windows.showMainWindow();
        this.options.windows.dispatchAction({ type: "openRoute", route });
      },
      snapshot: () => this.menuBarSnapshot(),
      quit: this.options.windows.quit
    };
  }

  private setupTray(): void {
    if (!this.status.tray) {
      this.status = {
        ...this.status,
        trayStatus: featureStatus("unsupported", "Tray/menu bar adapter is unavailable.")
      };
      return;
    }

    if (!this.options.settings.get().showTrayIcon) {
      this.status = {
        ...this.status,
        trayStatus: featureStatus("disabled", "Menu bar icon is disabled in Settings.")
      };
      return;
    }

    const result = this.options.adapter.createTray(this.actions());
    this.status = {
      ...this.status,
      trayStatus: statusFromResult(result, "ready", "Menu bar item is ready.")
    };
  }

  private registerProtocolClient(): void {
    if (!this.status.deepLinks) {
      this.status = {
        ...this.status,
        deepLinkStatus: {
          scheme: HCB_DEEP_LINK_SCHEME,
          registered: false,
          state: "unsupported",
          message: "Deep links are not supported by this platform adapter."
        }
      };
      return;
    }

    const result = this.options.adapter.registerProtocolClient(HCB_DEEP_LINK_SCHEME);
    this.status = {
      ...this.status,
      deepLinkStatus: {
        scheme: HCB_DEEP_LINK_SCHEME,
        registered: result.ok,
        state: result.ok ? "ready" : result.state ?? "error",
        message: sanitizedNativeMessage(
          result.message ??
            (result.ok ? "Protocol handler is registered." : "Protocol handler registration failed.")
        )
      }
    };
  }

  private syncGlobalShortcut(settings: SettingsSnapshot): void {
    const accelerator = settings.keybindings["quickAdd.open"]?.trim() || null;

    if (this.registeredGlobalShortcut && this.registeredGlobalShortcut !== accelerator) {
      this.options.adapter.unregisterGlobalShortcut(this.registeredGlobalShortcut);
      this.registeredGlobalShortcut = null;
    }

    if (!this.status.globalShortcuts) {
      this.status = {
        ...this.status,
        capabilityReport: updateCapabilityReportStatus(
          this.status.capabilityReport,
          "globalShortcuts",
          {
            ok: false,
            state: "unsupported",
            message: "Global shortcuts are not supported by this platform adapter."
          },
          "Global shortcuts are ready."
        )
      };
      return;
    }

    if (!accelerator) {
      if (this.registeredGlobalShortcut) {
        this.options.adapter.unregisterGlobalShortcut(this.registeredGlobalShortcut);
        this.registeredGlobalShortcut = null;
      }

      this.status = {
        ...this.status,
        capabilityReport: updateCapabilityReportStatus(
          this.status.capabilityReport,
          "globalShortcuts",
          {
            ok: false,
            state: "disabled",
            message: "Global quick add shortcut is not configured."
          },
          "Global quick add shortcut is registered."
        )
      };
      return;
    }

    if (this.registeredGlobalShortcut === accelerator) {
      return;
    }

    const result = this.options.adapter.registerGlobalShortcut(accelerator, () => {
      this.options.windows.showMainWindow();
      this.options.windows.dispatchAction({ type: "openQuickAdd" });
    });

    this.registeredGlobalShortcut = result.ok ? accelerator : null;
    this.status = {
      ...this.status,
      capabilityReport: updateCapabilityReportStatus(
        this.status.capabilityReport,
        "globalShortcuts",
        result,
        `${accelerator} is registered for global quick add.`
      )
    };
  }

  private scheduleNotificationsFromCache(): void {
    if (!this.status.notifications) {
      this.status = {
        ...this.status,
        notificationsStatus: {
          permission: "unsupported",
          scheduledCount: 0,
          state: "unsupported",
          message: "Local notifications are not supported by this platform adapter."
        }
      };
      return;
    }

    const settings = this.options.settings.get();

    if (!settings.notificationsEnabled) {
      this.options.adapter.clearScheduledNotifications();
      this.status = {
        ...this.status,
        notificationsStatus: {
          ...this.status.notificationsStatus,
          scheduledCount: 0,
          state: "disabled",
          message: "Local notifications are disabled in Settings."
        }
      };
      return;
    }

    this.options.adapter.clearScheduledNotifications();

    const requests = buildNativeNotificationRequests({
      planner: this.options.planner,
      now: this.now(),
      leadMinutes: settings.notificationLeadMinutes
    });
    let scheduledCount = 0;
    let failedCount = 0;

    for (const request of requests) {
      const scheduled = this.options.adapter.scheduleNotification(request, () => {
        if (request.action) {
          this.options.windows.showMainWindow();
          this.options.windows.dispatchAction(request.action);
        }
      }, (message) => {
        this.markNotificationFailure(message);
      });

      if (scheduled) {
        scheduledCount += 1;
        this.emitEventStartingWebhook(request);
      } else {
        failedCount += 1;
      }
    }

    this.status = {
      ...this.status,
      notificationsStatus: {
        ...this.status.notificationsStatus,
        scheduledCount,
        state: failedCount > 0 ? "error" : "ready",
        message:
          failedCount > 0
            ? `${failedCount} local notification${failedCount === 1 ? "" : "s"} could not be scheduled.`
            : scheduledCount === 0
            ? "No due tasks or upcoming events are in the next 24 hours."
            : `${scheduledCount} local notification${scheduledCount === 1 ? "" : "s"} scheduled.`
      }
    };
  }

  private markNotificationFailure(message: string): void {
    this.status = {
      ...this.status,
      notificationsStatus: {
        ...this.status.notificationsStatus,
        state: "error",
        message: sanitizedNativeMessage(message)
      }
    };
  }

  private menuBarSnapshot(): NativeMenuBarSnapshot {
    return buildNativeMenuBarSnapshot({
      planner: this.options.planner,
      settings: this.options.settings.get(),
      account: this.options.account?.latest() ?? null,
      now: this.now()
    });
  }

  private applyUpdateStatus(result: NativeOperationResult): void {
    this.status = {
      ...this.status,
      updaterStatus: statusFromResult(
        result,
        "ready",
        "Preview update check completed without blocking startup."
      )
    };
  }

  private handleTrayPrimaryAction(): void {
    const action = this.options.settings.get().trayClickAction;

    if (action === "open-today") {
      this.options.windows.showMainWindow();
      this.options.windows.dispatchAction({ type: "openRoute", route: { kind: "today" } });
      return;
    }

    if (action === "open-menu") {
      return;
    }

    this.options.windows.showOrHideMainWindow();
  }

  private updateMcpDeferredStatus(settings: SettingsSnapshot): void {
    this.status = {
      ...this.status,
      mcpStatus: settings.mcpEnabled
        ? featureStatus("pending", "MCP listener startup is managed by the main runtime service.")
        : featureStatus("disabled", "MCP local agent access is disabled.")
    };
  }

  private applyAutostartSetting(enabled: boolean): void {
    const result = this.options.adapter.setAutostart(enabled);

    this.status = {
      ...this.status,
      capabilityReport: updateCapabilityReportStatus(
        this.status.capabilityReport,
        "autostart",
        result,
        enabled ? "Open-at-login is enabled." : "Open-at-login is disabled."
      )
    };
  }

  private emitEventStartingWebhook(request: { id: string; body: string; deliveryDate: Date }): void {
    if (!request.id.startsWith("event:")) {
      return;
    }

    const eventId = request.id.slice("event:".length);
    const deliveryAt = request.deliveryDate.toISOString();
    const key = `${eventId}:${deliveryAt}`;

    if (this.eventStartingWebhookKeys.has(key)) {
      return;
    }

    this.eventStartingWebhookKeys.add(key);
    const payload: DomainJsonObject = {
      id: eventId,
      title: request.body,
      notificationDeliveryAt: deliveryAt
    };

    void Promise.resolve(this.options.webhooks?.emit("event.starting", payload)).catch((error) => {
      appLogger.warn("event starting webhook emit failed", "webhook", {
        message: error instanceof Error ? error.message : String(error)
      });
    });
  }
}

function shouldRunAutomaticUpdateCheck(settings: SettingsSnapshot, now: Date): boolean {
  if (settings.lastUpdateCheckAt === null) {
    return false;
  }

  const last = new Date(settings.lastUpdateCheckAt).getTime();

  return !Number.isFinite(last) || now.getTime() - last >= 24 * 60 * 60 * 1000;
}
