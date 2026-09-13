import {
  HCB_DEEP_LINK_SCHEME,
  type NativeAppPaths,
  type NativeNotificationRequest,
  type NativeOperationResult,
  type NativePlatformAdapter,
  type NativePlatformCapabilities,
  type NativeTrayActions,
  type ScheduledNativeNotification
} from "./types";
import {
  appPaths,
  autostartStatus,
  capabilities,
  checkForUpdates,
  collectDiagnostics,
  credentialStorageStatus,
  ensureWindowsAppIdentity,
  installAppMenu,
  openExternalUrl,
  openPath,
  registerProtocolClient,
  requestNotificationPermission,
  setAutostart
} from "./electronWindows/appEnvironment";
import { WindowsGlobalShortcutRegistry } from "./electronWindows/globalShortcuts";
import { WindowsNotificationScheduler } from "./electronWindows/notifications";
import { WindowsTrayController } from "./electronWindows/tray";

export function createElectronWindowsNativeAdapter(): NativePlatformAdapter {
  return new ElectronWindowsNativeAdapter();
}

class ElectronWindowsNativeAdapter implements NativePlatformAdapter {
  private readonly shortcuts = new WindowsGlobalShortcutRegistry();
  private readonly notifications = new WindowsNotificationScheduler();
  private readonly tray = new WindowsTrayController();

  constructor() {
    ensureWindowsAppIdentity();
  }

  appPaths(): NativeAppPaths {
    return appPaths();
  }

  capabilities(): NativePlatformCapabilities {
    return capabilities();
  }

  credentialStorageStatus(): NativeOperationResult {
    return credentialStorageStatus();
  }

  installAppMenu(_actions: NativeTrayActions): NativeOperationResult {
    return installAppMenu();
  }

  createTray(actions: NativeTrayActions): NativeOperationResult {
    return this.tray.create(actions);
  }

  destroyTray(): void {
    this.tray.destroy();
  }

  registerGlobalShortcut(accelerator: string, action: () => void): NativeOperationResult {
    return this.shortcuts.register(accelerator, action);
  }

  unregisterGlobalShortcut(accelerator?: string): void {
    this.shortcuts.unregister(accelerator);
  }

  registerProtocolClient(scheme: typeof HCB_DEEP_LINK_SCHEME): NativeOperationResult {
    return registerProtocolClient(scheme);
  }

  requestNotificationPermission() {
    return requestNotificationPermission();
  }

  listFontFamilies(): string[] {
    return [];
  }

  scheduleNotification(
    request: NativeNotificationRequest,
    onClick: () => void,
    onFailure?: (message: string) => void
  ): ScheduledNativeNotification | undefined {
    return this.notifications.schedule(request, onClick, onFailure);
  }

  clearScheduledNotifications(): void {
    this.notifications.clear();
  }

  setAutostart(enabled: boolean): NativeOperationResult {
    return setAutostart(enabled);
  }

  autostartStatus(): NativeOperationResult {
    return autostartStatus();
  }

  checkForUpdates(): Promise<NativeOperationResult> {
    return checkForUpdates();
  }

  async openExternalUrl(url: string): Promise<NativeOperationResult> {
    return openExternalUrl(url);
  }

  async openPath(path: string): Promise<NativeOperationResult> {
    return openPath(path);
  }

  collectDiagnostics(): NativeOperationResult {
    return collectDiagnostics();
  }

  dispose(): void {
    this.unregisterGlobalShortcut();
    this.clearScheduledNotifications();
    this.tray.destroy();
  }
}
