import type { HotkeyActionId } from "@shared/ipc/contracts";

export interface HotkeyDefinition {
  id: HotkeyActionId;
  label: string;
  group: "App" | "Navigation" | "Calendar";
}

export const hotkeyDefinitions: HotkeyDefinition[] = [
  { id: "task.create", label: "New Task", group: "App" },
  { id: "note.create", label: "New Note", group: "App" },
  { id: "calendar.create", label: "New Event", group: "App" },
  { id: "quickAdd.open", label: "Quick Add", group: "App" },
  { id: "commandPalette.open", label: "Quick Switcher", group: "App" },
  { id: "actionPalette.open", label: "Action Palette", group: "App" },
  { id: "sync.refresh", label: "Refresh Sync", group: "App" },
  { id: "sync.forceFullResync", label: "Force Full Resync", group: "App" },
  { id: "undo.perform", label: "Undo", group: "App" },
  { id: "redo.perform", label: "Redo", group: "App" },
  { id: "navigation.tasks", label: "Go to Tasks", group: "Navigation" },
  { id: "navigation.calendar", label: "Go to Calendar", group: "Navigation" },
  { id: "navigation.notes", label: "Go to Notes", group: "Navigation" },
  { id: "navigation.search", label: "Search Command Palette", group: "Navigation" },
  { id: "navigation.settings", label: "Open Settings", group: "Navigation" },
  { id: "navigation.diagnostics.toggle", label: "Diagnostics", group: "Navigation" },
  { id: "navigation.sidebar.toggle", label: "Toggle Sidebar", group: "Navigation" },
  { id: "navigation.notifications.toggle", label: "Notifications", group: "Navigation" },
  { id: "pane.create", label: "New Pane", group: "Navigation" },
  { id: "pane.close", label: "Close Pane", group: "Navigation" },
  { id: "pane.split.horizontal", label: "Split Pane Right", group: "Navigation" },
  { id: "pane.split.vertical", label: "Split Pane Bottom", group: "Navigation" },
  { id: "pane.focus.left", label: "Focus Pane Left", group: "Navigation" },
  { id: "pane.focus.right", label: "Focus Pane Right", group: "Navigation" },
  { id: "pane.focus.up", label: "Focus Pane Up", group: "Navigation" },
  { id: "pane.focus.down", label: "Focus Pane Down", group: "Navigation" },
  { id: "web.tab.create", label: "New Web Tab", group: "Navigation" },
  { id: "calendar.view.agenda", label: "Agenda View", group: "Calendar" },
  { id: "calendar.view.day", label: "Day View", group: "Calendar" },
  { id: "calendar.view.multiDay", label: "Multi-day View", group: "Calendar" },
  { id: "calendar.view.week", label: "Week View", group: "Calendar" },
  { id: "calendar.view.month", label: "Month View", group: "Calendar" }
];

export function eventMatchesAccelerator(
  event: Pick<KeyboardEvent, "altKey" | "ctrlKey" | "key" | "metaKey" | "shiftKey">,
  accelerator: string | null | undefined
): boolean {
  if (!accelerator) {
    return false;
  }

  const parsed = parseAccelerator(accelerator);

  if (!parsed) {
    return false;
  }

  const wantsPrimary = parsed.modifiers.has("cmdorctrl");
  const primaryPressed = event.metaKey || event.ctrlKey;

  if (wantsPrimary ? !primaryPressed : event.metaKey || event.ctrlKey) {
    return false;
  }

  if (parsed.modifiers.has("cmd") !== event.metaKey && !wantsPrimary) {
    return false;
  }

  if (parsed.modifiers.has("ctrl") !== event.ctrlKey && !wantsPrimary) {
    return false;
  }

  return parsed.modifiers.has("shift") === event.shiftKey &&
    parsed.modifiers.has("alt") === event.altKey &&
    normalizeKey(event.key) === parsed.key;
}

export function acceleratorFromKeyboardEvent(
  event: Pick<KeyboardEvent, "altKey" | "ctrlKey" | "key" | "metaKey" | "shiftKey">
): string | null {
  const key = normalizeKey(event.key);

  if (!key || ["shift", "control", "meta", "alt"].includes(key)) {
    return null;
  }

  const parts: string[] = [];

  if (event.metaKey || event.ctrlKey) {
    parts.push("CmdOrCtrl");
  }

  if (event.altKey) {
    parts.push("Alt");
  }

  if (event.shiftKey) {
    parts.push("Shift");
  }

  parts.push(displayKey(key));

  return parts.join("+");
}

export function displayAccelerator(accelerator: string | null | undefined): string {
  if (!accelerator) {
    return "Unassigned";
  }

  const parsed = parseAccelerator(accelerator);

  if (!parsed) {
    return accelerator;
  }

  const parts: string[] = [];

  if (parsed.modifiers.has("cmdorctrl")) {
    parts.push("Cmd");
  }

  if (parsed.modifiers.has("cmd")) {
    parts.push("Cmd");
  }

  if (parsed.modifiers.has("ctrl")) {
    parts.push("Ctrl");
  }

  if (parsed.modifiers.has("alt")) {
    parts.push("Opt");
  }

  if (parsed.modifiers.has("shift")) {
    parts.push("Shift");
  }

  parts.push(displayKey(parsed.key));

  return parts.join(" ");
}

export function ariaKeyShortcuts(accelerator: string | null | undefined): string | undefined {
  const parsed = accelerator ? parseAccelerator(accelerator) : null;

  if (!parsed) {
    return undefined;
  }

  const key = ariaKey(parsed.key);
  const modifiers = [
    parsed.modifiers.has("alt") ? "Alt" : "",
    parsed.modifiers.has("shift") ? "Shift" : ""
  ].filter(Boolean);

  if (parsed.modifiers.has("cmdorctrl")) {
    return [
      ["Meta", ...modifiers, key].join("+"),
      ["Control", ...modifiers, key].join("+")
    ].join(" ");
  }

  if (parsed.modifiers.has("cmd")) {
    modifiers.unshift("Meta");
  }

  if (parsed.modifiers.has("ctrl")) {
    modifiers.unshift("Control");
  }

  return [...modifiers, key].join("+");
}

export function duplicateAccelerators(
  keybindings: Partial<Record<HotkeyActionId, string | null>>
): Map<string, HotkeyActionId[]> {
  const byAccelerator = new Map<string, HotkeyActionId[]>();

  for (const [actionId, accelerator] of Object.entries(keybindings) as Array<[HotkeyActionId, string | null]>) {
    if (!accelerator) {
      continue;
    }

    const normalized = normalizeAccelerator(accelerator);
    byAccelerator.set(normalized, [...(byAccelerator.get(normalized) ?? []), actionId]);
  }

  return new Map([...byAccelerator].filter(([, actionIds]) => actionIds.length > 1));
}

function normalizeAccelerator(accelerator: string): string {
  const parsed = parseAccelerator(accelerator);

  if (!parsed) {
    return accelerator.trim().toLowerCase();
  }

  return [
    parsed.modifiers.has("cmdorctrl") ? "cmdorctrl" : "",
    parsed.modifiers.has("cmd") ? "cmd" : "",
    parsed.modifiers.has("ctrl") ? "ctrl" : "",
    parsed.modifiers.has("alt") ? "alt" : "",
    parsed.modifiers.has("shift") ? "shift" : "",
    parsed.key
  ].filter(Boolean).join("+");
}

function parseAccelerator(accelerator: string): { modifiers: Set<string>; key: string } | null {
  const parts = accelerator
    .split("+")
    .map((part) => part.trim())
    .filter(Boolean);

  if (parts.length === 0) {
    return null;
  }

  const modifiers = new Set<string>();
  let key = "";

  for (const part of parts) {
    const normalized = part.toLowerCase();

    if (["cmdorctrl", "commandorcontrol"].includes(normalized)) {
      modifiers.add("cmdorctrl");
    } else if (["cmd", "command", "meta"].includes(normalized)) {
      modifiers.add("cmd");
    } else if (["ctrl", "control"].includes(normalized)) {
      modifiers.add("ctrl");
    } else if (["alt", "option", "opt"].includes(normalized)) {
      modifiers.add("alt");
    } else if (normalized === "shift") {
      modifiers.add("shift");
    } else {
      key = normalizeKey(part);
    }
  }

  return key ? { modifiers, key } : null;
}

function normalizeKey(key: string): string {
  if (key === " ") {
    return "space";
  }

  if (key === ",") {
    return ",";
  }

  return key.toLowerCase().replace(/^arrow/, "");
}

function displayKey(key: string): string {
  if (key === "space") {
    return "Space";
  }

  if (key.length === 1) {
    return key.toUpperCase();
  }

  return key.charAt(0).toUpperCase() + key.slice(1);
}

function ariaKey(key: string): string {
  if (key === "left" || key === "right" || key === "up" || key === "down") {
    return `Arrow${displayKey(key)}`;
  }

  return displayKey(key);
}
