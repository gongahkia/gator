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
  listFontFamilies,
  openExternalUrl,
  openPath,
  registerProtocolClient,
  requestNotificationPermission,
  setAutostart
} from "./electronMac/appEnvironment";
import { installAppMenu } from "./electronMac/appMenu";
import { GlobalShortcutRegistry } from "./electronMac/globalShortcuts";
import { NotificationScheduler } from "./electronMac/notifications";
import { MacTrayController } from "./electronMac/tray";

export function createElectronMacNativeAdapter(): NativePlatformAdapter {
  return new ElectronMacNativeAdapter();
}

class ElectronMacNativeAdapter implements NativePlatformAdapter {
  private readonly shortcuts = new GlobalShortcutRegistry();
  private readonly notifications = new NotificationScheduler();
  private readonly tray = new MacTrayController();

  appPaths(): NativeAppPaths {
    return appPaths();
  }

  capabilities(): NativePlatformCapabilities {
    return capabilities();
  }

  credentialStorageStatus(): NativeOperationResult {
    return credentialStorageStatus();
  }

  installAppMenu(actions: NativeTrayActions): NativeOperationResult {
    return installAppMenu(actions);
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

  async listFontFamilies(): Promise<string[]> {
    return listFontFamilies();
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
    this.clearScheduledNotifications();
    this.unregisterGlobalShortcut();
    this.tray.destroy();
  }
}
