import type { NativeCapabilitiesResponse } from "@shared/ipc/contracts";
import type { SettingsSectionViewModel } from "../coreViewModels";
import { syncLabel } from "./dateFormat";
import type { CoreDataSnapshot } from "./types";

export function settingsSections(snapshot: CoreDataSnapshot): SettingsSectionViewModel[] {
  const sync = snapshot.syncStatus;
  const summary = snapshot.diagnosticsSummary;
  const build = summary?.build ?? snapshot.health?.build;
  const account = snapshot.googleStatus.account;
  const selectedTaskListCount =
    summary?.selectedResources.taskLists.filter((resource) => resource.selected).length ??
    snapshot.settings.selectedTaskListIds.length;
  const selectedCalendarCount =
    summary?.selectedResources.calendars.filter((resource) => resource.selected).length ??
    snapshot.settings.selectedCalendarIds.length;
  const capabilityReport = snapshot.native.capabilityReport;
  const platformBlockerCount = capabilityReport.diagnostics.filter(
    (diagnostic) => diagnostic.severity === "blocker"
  ).length;

  return [
    {
      id: "google",
      title: "Google",
      status: account?.connectionState === "connected" ? "Connected" : "Disconnected",
      detail: "Google account connection state",
      rows: [
        { id: "client", label: "OAuth client", value: snapshot.googleStatus.oauthClientConfigured ? "Configured" : "Missing" },
        { id: "state", label: "State", value: account?.connectionState ?? "signed_out" },
        {
          id: "account",
          label: "Account",
          value: account?.connectionState === "connected" ? account.email ?? "Connected account" : "Not connected"
        },
        {
          id: "scopes",
          label: "Missing scopes",
          value: String(account?.missingScopes.length ?? 2)
        }
      ]
    },
    {
      id: "resources",
      title: "Resources",
      status: `${selectedTaskListCount}/${selectedCalendarCount}`,
      detail: "Selected task lists and calendars",
      rows: [
        { id: "task-lists", label: "Selected task lists", value: String(selectedTaskListCount) },
        { id: "calendars", label: "Selected calendars", value: String(selectedCalendarCount) },
        { id: "default-time-zone", label: "Default timezone", value: snapshot.settings.defaultTimeZone },
        { id: "today-capacity", label: "Today capacity", value: `${snapshot.settings.todayCapacityMinutes} min` },
        {
          id: "today-hours",
          label: "Today hours",
          value: `${snapshot.settings.todayWorkingHoursStart}:00-${snapshot.settings.todayWorkingHoursEnd}:00`
        }
      ]
    },
    {
      id: "sync",
      title: "Sync",
      status: syncLabel(sync),
      detail: "Read sync state",
      rows: [
        { id: "mode", label: "Mode", value: snapshot.settings.syncMode },
        { id: "pending", label: "Pending mutations", value: String(sync.pendingMutationCount) },
        { id: "completed", label: "Last completed", value: sync.lastCompletedAt ?? "Never" },
        { id: "error", label: "Last error", value: sync.lastErrorCode ?? "None" }
      ]
    },
    {
      id: "appearance",
      title: "Appearance",
      status: snapshot.settings.theme,
      detail: "Theme and color palette preference",
      rows: [
        { id: "theme", label: "Theme", value: snapshot.settings.theme },
        { id: "color-theme", label: "Color theme", value: snapshot.settings.colorTheme },
        {
          id: "font",
          label: "Font",
          value: snapshot.settings.uiFontName ?? "System"
        },
        {
          id: "text-size",
          label: "Text size",
          value: `${snapshot.settings.uiTextSizePoints} pt`
        }
      ]
    },
    {
      id: "tray",
      title: "Tray",
      status: nativeStateLabel(snapshot.native.trayStatus.state),
      detail: "Menu bar state",
      rows: [
        { id: "icon", label: "Show icon", value: snapshot.settings.showTrayIcon ? "Yes" : "No" },
        { id: "click", label: "Click action", value: snapshot.settings.trayClickAction },
        { id: "panel", label: "Panel style", value: snapshot.settings.menuBarPanelStyle },
        { id: "badge", label: "Overdue badge", value: snapshot.settings.showMenuBarBadge ? "On" : "Off" },
        { id: "native", label: "Native state", value: snapshot.native.trayStatus.message ?? "No native status reported" }
      ]
    },
    {
      id: "notifications",
      title: "Notifications",
      status: nativeStateLabel(snapshot.native.notificationsStatus.state),
      detail: "Notification permission",
      rows: [
        { id: "enabled", label: "Local notifications", value: snapshot.settings.notificationsEnabled ? "On" : "Off" },
        { id: "lead", label: "Lead time", value: `${snapshot.settings.notificationLeadMinutes} min` },
        { id: "permission", label: "Permission", value: snapshot.native.notificationsStatus.permission },
        { id: "scheduled", label: "Scheduled", value: String(snapshot.native.notificationsStatus.scheduledCount) }
      ]
    },
    {
      id: "localData",
      title: "Local data",
      status: summary ? "Ready" : "Pending",
      detail: "Cache, checkpoint, and pending mutation state",
      rows: [
        { id: "cache", label: "Cached items", value: String((summary?.cache.taskCount ?? 0) + (summary?.cache.eventCount ?? 0)) },
        { id: "checkpoints", label: "Checkpoints", value: String(summary?.checkpoints.totalCount ?? 0) },
        { id: "pending", label: "Pending mutations", value: String(summary?.pendingMutations.totalCount ?? sync.pendingMutationCount) }
      ]
    },
    {
      id: "mcp",
      title: "MCP",
      status: snapshot.settings.mcpEnabled
        ? nativeStateLabel(snapshot.native.mcpStatus.state)
        : "Disabled",
      detail: "Local agent access",
      rows: [
        { id: "enabled", label: "Enabled", value: snapshot.settings.mcpEnabled ? "Yes" : "No" },
        { id: "mode", label: "Permission mode", value: snapshot.settings.mcpPermissionMode },
        { id: "token", label: "Token state", value: summary?.mcp.tokenState ?? "not_configured" },
        { id: "startup", label: "Startup", value: snapshot.native.mcpStatus.message ?? "No native status reported" }
      ]
    },
    {
      id: "platform",
      title: "Platform",
      status: platformBlockerCount > 0 ? `${platformBlockerCount} blocker${platformBlockerCount === 1 ? "" : "s"}` : "Ready",
      detail: "Adapter capability report",
      rows: [
        { id: "platform", label: "Runtime", value: capabilityReport.platform },
        { id: "adapter", label: "Adapter", value: capabilityReport.adapterId },
        { id: "package", label: "Package", value: capabilityReport.packageFormat },
        { id: "tray", label: "Tray", value: capabilityReport.flags.supportsTray ? "Supported" : "Unsupported" },
        {
          id: "hotkeys",
          label: "Global shortcuts",
          value: capabilityReport.flags.supportsGlobalShortcut ? "Supported" : "Unsupported"
        },
        {
          id: "credentials",
          label: "Credential storage",
          value: capabilityReport.flags.supportsCredentialStorage ? "Supported" : "Blocked"
        },
        {
          id: "updater",
          label: "In-place updater",
          value: capabilityReport.flags.supportsInPlaceAutoUpdate ? "Supported" : "Unsupported"
        }
      ]
    },
    {
      id: "diagnostics",
      title: "Diagnostics",
      status: "Ready",
      detail: "Sanitized local diagnostics",
      rows: [
        { id: "version", label: "Version", value: build?.version ?? snapshot.health?.version ?? "Unknown" },
        { id: "environment", label: "Environment", value: build?.environment ?? snapshot.health?.environment ?? "Unknown" },
        { id: "commit", label: "Build commit", value: build?.commit ?? "Not recorded" },
        { id: "build-date", label: "Build date", value: build?.buildDate ?? "Not recorded" },
        { id: "package-tool", label: "Package tool", value: build?.packageTool ?? "Not recorded" },
        {
          id: "database",
          label: "Database ready",
          value: snapshot.health?.startup.databaseReadyMs === undefined
            ? "Not marked"
            : `${snapshot.health.startup.databaseReadyMs}ms`
        }
      ]
    }
  ];
}

function nativeStateLabel(state: NativeCapabilitiesResponse["trayStatus"]["state"]): string {
  if (state === "ready") {
    return "Ready";
  }

  if (state === "conflict") {
    return "Conflict";
  }

  if (state === "error") {
    return "Error";
  }

  if (state === "unsupported") {
    return "Unsupported";
  }

  if (state === "disabled") {
    return "Disabled";
  }

  return "Pending";
}
