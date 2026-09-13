import type {
  CalendarEventSummary,
  NativeAction,
  NativeCapabilityReport,
  NativeCapabilitiesResponse,
  NativeFeatureState,
  NativeNotificationPermissionResponse,
  NativeRoute,
  SettingsSnapshot,
  SearchQueryResponse,
  SyncRunNowRequest,
  TaskSummary
} from "@shared/ipc/contracts";

export const HCB_DEEP_LINK_SCHEME = "hotcrossbuns";

export interface NativeOperationResult {
  ok: boolean;
  checkedAt?: string;
  downloadUrl?: string;
  latestVersion?: string;
  releaseName?: string;
  releaseUrl?: string;
  state?: NativeFeatureState;
  message?: string;
  updateAvailable?: boolean;
}

export interface NativeAppPaths {
  configDirectory: string;
  dataDirectory: string;
  cacheDirectory: string;
  logsDirectory: string;
  diagnosticsDirectory: string;
  tempDirectory: string;
}

export type NativePackageFormat = NativeCapabilityReport["packageFormat"];

export interface NativePlatformCapabilities {
  platform: NativeCapabilitiesResponse["platform"];
  adapterId: string;
  notifications: boolean;
  globalShortcuts: boolean;
  tray: boolean;
  deepLinks: boolean;
  updaterChecks: boolean;
  capabilityReport: NativeCapabilityReport;
}

export interface NativeTrayActions {
  primaryClick: () => void;
  openMainWindow: () => void;
  showOrHideMainWindow: () => void;
  openQuickAdd: () => void;
  refresh: () => void;
  openSettings: () => void;
  openRoute: (route: NativeRoute) => void;
  snapshot: () => NativeMenuBarSnapshot;
  quit: () => void;
}

export type NativeMenuBarAction =
  | "refresh"
  | "openSettings"
  | "showWindow"
  | "quit";

export interface NativeMenuBarItem {
  label: string;
  detail?: string;
  route?: NativeRoute;
  action?: NativeMenuBarAction;
}

export interface NativeMenuBarCalendarDay {
  key: string;
  label: string;
  inCurrentMonth: boolean;
  isToday: boolean;
  isSelected: boolean;
}

export interface NativeMenuBarCalendarSnapshot {
  monthLabel: string;
  weekdayLabels: string[];
  days: NativeMenuBarCalendarDay[];
  selectedLabel: string;
  selectedMeta: string;
  selectedItems: NativeMenuBarItem[];
}

export interface NativeMenuBarAccountSnapshot {
  displayName: string;
  email?: string;
  avatarUrl?: string;
  connectionState: string;
}

export interface NativeMenuBarSection {
  title?: string;
  items: NativeMenuBarItem[];
}

export interface NativeMenuBarSnapshot {
  panelStyle: SettingsSnapshot["menuBarPanelStyle"];
  iconName: SettingsSnapshot["menuBarIconName"];
  calendarIconId: SettingsSnapshot["menuBarCalendarIconId"];
  calendarDoneMode: SettingsSnapshot["menuBarCalendarDoneMode"];
  customMenuBarIcons: SettingsSnapshot["customMenuBarIcons"];
  calendarDone: boolean;
  primaryClickAction: SettingsSnapshot["trayClickAction"];
  title: string;
  subtitle?: string;
  statusLabel?: string;
  syncLabel: string;
  badgeLabel?: string;
  dockBadgeLabel?: string;
  tooltip: string;
  sections: NativeMenuBarSection[];
  calendar?: NativeMenuBarCalendarSnapshot;
  account?: NativeMenuBarAccountSnapshot;
}

export interface NativeNotificationRequest {
  id: string;
  title: string;
  body: string;
  deliveryDate: Date;
  action?: NativeAction;
}

export interface ScheduledNativeNotification {
  id: string;
  cancel: () => void;
}

export interface NativePlatformAdapter {
  appPaths: () => NativeAppPaths;
  capabilities: () => NativePlatformCapabilities;
  credentialStorageStatus: () => NativeOperationResult;
  installAppMenu: (actions: NativeTrayActions) => NativeOperationResult;
  createTray: (actions: NativeTrayActions) => NativeOperationResult;
  destroyTray: () => void;
  registerGlobalShortcut: (
    accelerator: string,
    action: () => void
  ) => NativeOperationResult;
  unregisterGlobalShortcut: (accelerator?: string) => void;
  registerProtocolClient: (scheme: typeof HCB_DEEP_LINK_SCHEME) => NativeOperationResult;
  requestNotificationPermission: () => NativeNotificationPermissionResponse;
  listFontFamilies: () => string[] | Promise<string[]>;
  scheduleNotification: (
    request: NativeNotificationRequest,
    onClick: () => void,
    onFailure?: (message: string) => void
  ) => ScheduledNativeNotification | undefined;
  clearScheduledNotifications: () => void;
  setAutostart: (enabled: boolean) => NativeOperationResult;
  autostartStatus: () => NativeOperationResult;
  checkForUpdates: () => NativeOperationResult | Promise<NativeOperationResult>;
  openExternalUrl: (url: string) => NativeOperationResult | Promise<NativeOperationResult>;
  openPath: (path: string) => NativeOperationResult | Promise<NativeOperationResult>;
  collectDiagnostics: () => NativeOperationResult;
  dispose: () => void;
}

export interface NativePlannerSnapshotSource {
  listTasks: (request: { status: "active"; limit: number }) => { items: TaskSummary[] };
  listCalendarEvents: (request: {
    start: string;
    end: string;
    limit: number;
  }) => { items: CalendarEventSummary[] };
  search?: (request: { query: string; limit: number }) => SearchQueryResponse;
}

export interface NativeSettingsSource {
  get: () => SettingsSnapshot;
}

export interface NativeShellWindowActions {
  showMainWindow: () => void;
  hideMainWindow: () => void;
  showOrHideMainWindow: () => void;
  quit: () => void;
  dispatchAction: (action: NativeAction) => void;
}

export interface NativeShellSyncActions {
  runNow: (request: SyncRunNowRequest) => void | Promise<unknown>;
}
