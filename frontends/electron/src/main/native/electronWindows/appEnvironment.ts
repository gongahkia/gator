import { Notification, app, safeStorage, shell } from "electron";
import { join } from "node:path";
import { windowsSafeStorageStatus } from "../../credentials/secretStore";
import {
  buildNativeCapabilityReport,
  capabilityDiagnostic
} from "../capabilityReport";
import {
  checkGitHubReleaseForUpdates,
  windowsReleaseAssetPreferences
} from "../githubReleaseUpdates";
import {
  HCB_DEEP_LINK_SCHEME,
  type NativeAppPaths,
  type NativeOperationResult,
  type NativePlatformCapabilities
} from "../types";
import { sanitizedFailure, unsupported } from "./operationResults";
import { applyWindowsAppIdentity, windowsAppUserModelId } from "./identity";

export { windowsAppUserModelId };

export function ensureWindowsAppIdentity(): NativeOperationResult {
  return applyWindowsAppIdentity("win32");
}

export function appPaths(): NativeAppPaths {
  const userData = app.getPath("userData");
  const sessionData = safeAppPath("sessionData", join(userData, "session"));
  const logs = safeAppPath("logs", join(userData, "logs"));
  const temp = safeAppPath("temp", join(userData, "tmp"));

  return {
    configDirectory: join(userData, "config"),
    dataDirectory: join(userData, "data"),
    cacheDirectory: sessionData,
    logsDirectory: logs,
    diagnosticsDirectory: join(userData, "diagnostics"),
    tempDirectory: join(temp, "hot-cross-buns")
  };
}

export function capabilities(): NativePlatformCapabilities {
  ensureWindowsAppIdentity();
  const paths = appPaths();
  const credentialStatus = credentialStorageStatus();
  const notifications = Notification.isSupported();
  const packaged = app.isPackaged;
  const flags = {
    supportsAppPaths: true,
    supportsTray: true,
    supportsAppMenu: false,
    supportsGlobalShortcut: true,
    supportsNotifications: notifications,
    supportsNotificationPermissionQuery: false,
    supportsProtocolRegistration: true,
    supportsProtocolRegistrationCheck: true,
    supportsAutostart: packaged,
    supportsInPlaceAutoUpdate: false,
    supportsInstallerMetadata: packaged,
    supportsExternalUrlOpen: true,
    supportsDiagnosticsCollection: true,
    supportsCredentialStorage: credentialStatus.ok,
    supportsOAuthLoopback: true,
    supportsMcpLoopback: true,
    requiresSignedBuildForNotifications: true
  };

  return {
    platform: "win32",
    adapterId: "electron-windows-preview",
    notifications,
    globalShortcuts: true,
    tray: true,
    deepLinks: true,
    updaterChecks: true,
    capabilityReport: buildNativeCapabilityReport({
      platform: "win32",
      adapterId: "electron-windows-preview",
      appPaths: paths,
      packageFormat: packaged ? "nsis" : "development",
      flags,
      capabilityOverrides: {
        credentialStorage: {
          state: credentialStatus.state ?? "error",
          message: credentialStatus.message
        },
        tray: {
          state: "pending",
          message: "Windows notification-area tray support is wired and still requires installed-build manual QA."
        },
        appMenu: {
          state: "unsupported",
          message: "Windows application menu customization is not enabled for the first technical preview."
        },
        globalShortcuts: {
          state: "pending",
          message: "Windows global shortcut support is wired and still requires conflict/manual QA on Windows."
        },
        notifications: {
          state: notifications ? "pending" : "unsupported",
          message: notifications
            ? "Windows notifications are wired, but AppUserModelID, Start Menu identity, and unsigned preview behavior require installed-build QA."
            : "Electron reports Windows notifications are unavailable in this runtime."
        },
        customProtocol: {
          state: "pending",
          message: "hotcrossbuns:// registration is wired and requires warm/cold-start NSIS install validation."
        },
        autostart: {
          state: packaged ? "pending" : "unsupported",
          message: packaged
            ? "Windows open-at-login is available through Electron login item settings and requires installed-build QA."
            : "Windows open-at-login is only applied from a packaged installer build."
        },
        updater: {
          state: "ready",
          message: "GitHub release checks are available for Windows installer assets; in-place auto-update remains disabled."
        },
        installerMetadata: {
          state: packaged ? "ready" : "pending",
          message: packaged
            ? "Windows installer metadata is available from the packaged app."
            : "Windows NSIS installer metadata is not active in this development runtime."
        },
        externalOpen: {
          state: "ready",
          message: "External URL and path opening is delegated to Electron shell APIs."
        },
        oauthLoopback: {
          state: "pending",
          message: "OAuth loopback is shared code, but Windows browser handoff and firewall behavior require manual QA."
        },
        mcpLoopback: {
          state: "pending",
          message: "MCP loopback is shared code, but Windows Defender/firewall and credential storage require manual QA."
        },
        packaging: {
          state: packaged ? "ready" : "pending",
          message: packaged
            ? "Windows NSIS package metadata was detected."
            : "Windows NSIS packaging is not active in this development runtime."
        }
      },
      diagnostics: [
        credentialStatus.ok
          ? capabilityDiagnostic(
              "credentialStorage",
              "info",
              "Windows credential storage is available through Electron safeStorage."
            )
          : capabilityDiagnostic(
              "credentialStorage",
              "blocker",
              credentialStatus.message ??
                "Windows credential storage is unavailable; credential-dependent features must stay blocked."
            ),
        capabilityDiagnostic(
          "notifications",
          "warning",
          "Windows notification behavior depends on installed Start Menu identity and signed-public-build trust; validate before release claims."
        ),
        capabilityDiagnostic(
          "packaging",
          "warning",
          "Unsigned Windows preview installers will trigger SmartScreen and trust warnings."
        )
      ]
    })
  };
}

export function credentialStorageStatus(): NativeOperationResult {
  return windowsSafeStorageStatus(safeStorage, "win32");
}

export function installAppMenu(): NativeOperationResult {
  return unsupported("Windows application menu customization is not enabled for the first technical preview.");
}

export function registerProtocolClient(scheme: typeof HCB_DEEP_LINK_SCHEME): NativeOperationResult {
  ensureWindowsAppIdentity();
  const defaultApp = (process as NodeJS.Process & { defaultApp?: boolean }).defaultApp;
  const ok = defaultApp && process.argv.length >= 2
    ? app.setAsDefaultProtocolClient(scheme, process.execPath, [process.argv[1]])
    : app.setAsDefaultProtocolClient(scheme);

  return {
    ok,
    state: ok ? "ready" : "error",
    message: ok
      ? `${scheme}:// links are registered for this Windows app.`
      : `${scheme}:// links could not be registered for this Windows app.`
  };
}

export function requestNotificationPermission() {
  return {
    state: Notification.isSupported() ? "granted" as const : "unsupported" as const
  };
}

export function setAutostart(enabled: boolean): NativeOperationResult {
  if (!app.isPackaged) {
    return developmentAutostartResult(enabled);
  }

  try {
    app.setLoginItemSettings({
      openAtLogin: enabled
    });
    const status = app.getLoginItemSettings();

    return {
      ok: status.openAtLogin === enabled,
      state: status.openAtLogin === enabled ? "ready" : "error",
      message:
        status.openAtLogin === enabled
          ? enabled
            ? "Windows open-at-login is enabled."
            : "Windows open-at-login is disabled."
          : "Windows open-at-login did not match the requested setting."
    };
  } catch {
    return {
      ok: false,
      state: "error",
      message: "Windows open-at-login could not be updated."
    };
  }
}

export function autostartStatus(): NativeOperationResult {
  if (!app.isPackaged) {
    return developmentAutostartResult(false);
  }

  try {
    const status = app.getLoginItemSettings();

    return {
      ok: true,
      state: status.openAtLogin ? "ready" : "disabled",
      message: status.openAtLogin ? "Windows open-at-login is enabled." : "Windows open-at-login is disabled."
    };
  } catch {
    return {
      ok: false,
      state: "error",
      message: "Windows open-at-login status could not be read."
    };
  }
}

export async function checkForUpdates(): Promise<NativeOperationResult> {
  return checkGitHubReleaseForUpdates({
    appVersion: app.getVersion(),
    assetPreferences: windowsReleaseAssetPreferences,
    userAgentVersion: app.getVersion()
  });
}

export async function openExternalUrl(url: string): Promise<NativeOperationResult> {
  try {
    await shell.openExternal(url);

    return {
      ok: true,
      state: "ready",
      message: "External URL was opened by Windows."
    };
  } catch {
    return sanitizedFailure("External URL could not be opened by Windows.");
  }
}

export async function openPath(path: string): Promise<NativeOperationResult> {
  try {
    const result = await shell.openPath(path);

    return result
      ? sanitizedFailure("Path could not be opened by Windows.")
      : {
          ok: true,
          state: "ready",
          message: "Path was opened by Windows."
        };
  } catch {
    return sanitizedFailure("Path could not be opened by Windows.");
  }
}

export function collectDiagnostics(): NativeOperationResult {
  return {
    ok: true,
    state: "ready",
    message: "Windows native adapter diagnostics are available through the sanitized capability report."
  };
}

function developmentAutostartResult(enabled: boolean): NativeOperationResult {
  return {
    ok: !enabled,
    state: enabled ? "unsupported" : "disabled",
    message: enabled
      ? "Windows open-at-login is only applied from a packaged installer build."
      : "Windows open-at-login is not modified during development runs."
  };
}

function safeAppPath(name: Parameters<typeof app.getPath>[0], fallback: string): string {
  try {
    return app.getPath(name);
  } catch {
    return fallback;
  }
}
