import type { SettingsRecoveryActionRequest, SettingsSnapshot } from "@shared/ipc/contracts";

export type NavigationTabId = SettingsSnapshot["hiddenNavigationTabs"][number];
export type ToolbarActionId = SettingsSnapshot["toolbarActionOrder"][number];
export type CalendarViewModeId = SettingsSnapshot["hiddenCalendarViewModes"][number];
export type FontSurfaceId = keyof SettingsSnapshot["perSurfaceFontOverrides"];

export const navigationTabs: Array<{ id: NavigationTabId; label: string }> = [
  { id: "calendar", label: "Calendar" },
  { id: "tasks", label: "Tasks" },
  { id: "notes", label: "Notes" }
];

export const toolbarActions: Array<{ id: ToolbarActionId; label: string }> = [
  { id: "commandPalette", label: "Command palette" },
  { id: "notifications", label: "Notifications" },
  { id: "diagnostics", label: "Diagnostics" },
  { id: "splitPane", label: "Split view" },
  { id: "refresh", label: "Reload" },
  { id: "settings", label: "Settings" }
];

export const calendarViewModes: Array<{ id: CalendarViewModeId; label: string }> = [
  { id: "agenda", label: "Agenda" },
  { id: "day", label: "Day" },
  { id: "multiDay", label: "Multi-day" },
  { id: "week", label: "Week" },
  { id: "month", label: "Month" }
];

export const retentionOptions: Array<{ label: string; value: number }> = [
  { label: "Forever", value: 0 },
  { label: "30 days", value: 30 },
  { label: "90 days", value: 90 },
  { label: "180 days", value: 180 },
  { label: "1 year", value: 365 },
  { label: "2 years", value: 730 }
];

export const fontSurfaceOptions: Array<{ id: FontSurfaceId; label: string }> = [
  { id: "markdownEditor", label: "Markdown editor" },
  { id: "sidebar", label: "Sidebar" },
  { id: "calendarGrid", label: "Calendar grid" },
  { id: "taskList", label: "Task list" },
  { id: "inspector", label: "Inspector" },
  { id: "menuBar", label: "Menu bar" }
];

export function recoveryPhrase(action: SettingsRecoveryActionRequest["action"]): string {
  if (action === "forceFullResync") {
    return "FULL RESYNC";
  }

  if (action === "clearGoogleCache") {
    return "CLEAR CACHE";
  }

  if (action === "resetMcpToken") {
    return "RESET MCP TOKEN";
  }

  return "";
}
