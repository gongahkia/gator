import type { NativeNotificationPermissionResponse } from "@shared/ipc/contracts";
import {
  HCB_DEEP_LINK_SCHEME,
  type NativeAppPaths,
  type NativeOperationResult,
  type NativePlatformAdapter,
  type NativePlatformCapabilities,
  type NativeNotificationRequest,
  type NativeTrayActions,
  type ScheduledNativeNotification
} from "./types";
import {
  buildNativeCapabilityReport,
  capabilityDiagnostic,
  defaultNativeAppPaths,
  nativePlatform
} from "./capabilityReport";

export function createNoopNativeAdapter(
  platform: NativePlatformCapabilities["platform"] = nativePlatform()
): NativePlatformAdapter {
  const appPaths = defaultNativeAppPaths();
  const flags = {
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
    requiresSignedBuildForNotifications: platform === "win32",
    ...(platform === "linux"
      ? {
          hasWaylandSession: process.env.XDG_SESSION_TYPE === "wayland",
          hasPortalShortcutSupport: false
        }
      : {})
  };
  const capabilities: NativePlatformCapabilities = {
    platform,
    adapterId: "noop",
    notifications: false,
    globalShortcuts: false,
    tray: false,
    deepLinks: false,
    updaterChecks: false,
    capabilityReport: buildNativeCapabilityReport({
      platform,
      adapterId: "noop",
      appPaths,
      flags,
      capabilityOverrides: {
        oauthLoopback: {
          state: "pending",
          message: "OAuth loopback is shared code, but browser handoff has not been verified for this adapter."
        },
        mcpLoopback: {
          state: "pending",
          message: "MCP loopback is shared code, but native lifecycle and credential storage are not wired."
        },
        packaging: {
          state: "unsupported",
          message: "No package metadata is installed by the noop adapter."
        }
      },
      diagnostics: [
        capabilityDiagnostic(
          "packaging",
          "blocker",
          "No platform adapter is installed for this runtime; non-Mac support is not claimed."
        ),
        capabilityDiagnostic(
          "credentialStorage",
          "blocker",
          "OS credential storage is not wired for this adapter."
        )
      ]
    })
  };

  return {
    appPaths: (): NativeAppPaths => appPaths,
    capabilities: () => capabilities,
    credentialStorageStatus: () => unsupported("OS credential storage is unavailable."),
    installAppMenu: (_actions: NativeTrayActions) => unsupported("App menu is unavailable."),
    createTray: (_actions: NativeTrayActions) => unsupported("Tray/menu bar is unavailable."),
    destroyTray: () => undefined,
    registerGlobalShortcut: () => unsupported("Global shortcuts are unavailable."),
    unregisterGlobalShortcut: () => undefined,
    registerProtocolClient: (_scheme: typeof HCB_DEEP_LINK_SCHEME) =>
      unsupported("Protocol registration is unavailable."),
    requestNotificationPermission: (): NativeNotificationPermissionResponse => ({
      state: "unsupported"
    }),
    listFontFamilies: () => [],
    scheduleNotification: (
      _request: NativeNotificationRequest,
      _onClick: () => void
    ): ScheduledNativeNotification | undefined => undefined,
    clearScheduledNotifications: () => undefined,
    setAutostart: () => unsupported("Open-at-login is unavailable."),
    autostartStatus: () => unsupported("Open-at-login is unavailable."),
    checkForUpdates: () => unsupported("Preview update checks are not configured."),
    openExternalUrl: () => unsupported("External URL opening is unavailable."),
    openPath: () => unsupported("External file opening is unavailable."),
    collectDiagnostics: () => ({
      ok: true,
      state: "ready",
      message: "Noop adapter diagnostic metadata is available."
    }),
    dispose: () => undefined
  };
}

function unsupported(message: string): NativeOperationResult {
  return {
    ok: false,
    state: "unsupported",
    message
  };
}
