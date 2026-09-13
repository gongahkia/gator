import type {
  NativeCapabilitiesResponse,
  NativeCapabilityKey,
  NativeCapabilityReport,
  NativeFeatureState,
  SettingsSnapshot
} from "@shared/ipc/contracts";
import { redactDiagnosticText } from "@shared/redaction";
import {
  HCB_DEEP_LINK_SCHEME,
  type NativeOperationResult,
  type NativePlatformCapabilities
} from "./types";

export function initialStatus(
  capabilities: NativePlatformCapabilities,
  settings: SettingsSnapshot
): NativeCapabilitiesResponse {
  return {
    platform: capabilities.platform,
    notifications: capabilities.notifications,
    globalShortcuts: capabilities.globalShortcuts,
    tray: capabilities.tray,
    deepLinks: capabilities.deepLinks,
    trayStatus: capabilities.tray
      ? settings.showTrayIcon
        ? featureStatus("pending", "Tray startup is deferred until the shell is visible.")
        : featureStatus("disabled", "Menu bar icon is disabled in Settings.")
      : featureStatus("unsupported", "Tray/menu bar adapter is unavailable."),
    notificationsStatus: {
      permission: capabilities.notifications ? "prompt" : "unsupported",
      scheduledCount: 0,
      state: capabilities.notifications
        ? settings.notificationsEnabled
          ? "pending"
          : "disabled"
        : "unsupported",
      message: capabilities.notifications
        ? settings.notificationsEnabled
          ? "Notification scheduling is deferred until the shell is visible."
          : "Local notifications are disabled in Settings."
        : "Local notifications are not supported by this platform adapter."
    },
    deepLinkStatus: {
      scheme: HCB_DEEP_LINK_SCHEME,
      registered: false,
      state: capabilities.deepLinks ? "pending" : "unsupported",
      message: capabilities.deepLinks
        ? "Protocol registration is deferred until the shell is visible."
        : "Deep links are not supported by this platform adapter."
    },
    updaterStatus: capabilities.updaterChecks
      ? featureStatus("pending", "Update checks are deferred until the shell is visible.")
      : featureStatus("unsupported", "Preview update checks are not configured for this build."),
    mcpStatus: settings.mcpEnabled
      ? featureStatus("pending", "MCP listener startup is deferred until the shell is visible.")
      : featureStatus("disabled", "MCP local agent access is disabled."),
    capabilityReport: capabilities.capabilityReport,
    deferredStartup: {
      state: "pending"
    }
  };
}

export function featureStatus(state: NativeFeatureState, message: string) {
  return {
    state,
    message
  };
}

export function statusFromResult(
  result: NativeOperationResult,
  successState: NativeFeatureState,
  successMessage: string
) {
  return {
    state: result.ok ? successState : result.state ?? "error",
    message: sanitizedNativeMessage(
      result.message ?? (result.ok ? successMessage : "Native adapter operation failed.")
    ),
    ...(result.checkedAt ? { checkedAt: result.checkedAt } : {}),
    ...(result.downloadUrl ? { downloadUrl: result.downloadUrl } : {}),
    ...(result.latestVersion ? { latestVersion: result.latestVersion } : {}),
    ...(result.releaseName ? { releaseName: result.releaseName } : {}),
    ...(result.releaseUrl ? { releaseUrl: result.releaseUrl } : {}),
    ...(result.updateAvailable !== undefined ? { updateAvailable: result.updateAvailable } : {})
  };
}

export function updateCapabilityReportStatus(
  report: NativeCapabilityReport,
  key: NativeCapabilityKey,
  result: NativeOperationResult,
  successMessage: string
): NativeCapabilityReport {
  return {
    ...report,
    capabilities: report.capabilities.map((capability) =>
      capability.key === key
        ? {
            ...capability,
            supported: capability.supported || result.ok,
            state: result.ok ? "ready" : result.state ?? "error",
            message: sanitizedNativeMessage(
              result.message ?? (result.ok ? successMessage : "Native adapter operation failed.")
            )
          }
        : capability
    )
  };
}

export function messageFromError(error: unknown, fallback: string): string {
  return sanitizedNativeMessage(
    error instanceof Error && error.message.trim() ? error.message : fallback
  );
}

export function sanitizedNativeMessage(message: string): string {
  return redactDiagnosticText(message).slice(0, 500);
}

export function isPromise<T>(value: T | Promise<T>): value is Promise<T> {
  return typeof (value as Promise<T>).then === "function";
}
