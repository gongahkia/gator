import type { NativeRoute } from "@shared/ipc/contracts";
import type { NativeMenuBarItem } from "../types";

export type MenuBarPanelNavigation =
  | { kind: "route"; route: NativeRoute }
  | { kind: "action"; action: NonNullable<NativeMenuBarItem["action"]> };

export function parseMenuBarPanelUrl(url: string): MenuBarPanelNavigation | null {
  let parsed: URL;

  try {
    parsed = new URL(url);
  } catch {
    return null;
  }

  if (parsed.protocol !== "hcb-panel:") {
    return null;
  }

  if (parsed.hostname === "route") {
    return parseMenuBarPanelRoute(parsed.searchParams);
  }

  if (parsed.hostname === "action") {
    return parseMenuBarPanelAction(parsed.searchParams);
  }

  return null;
}

function parseMenuBarPanelRoute(params: URLSearchParams): MenuBarPanelNavigation | null {
  const kind = params.get("kind");

  if (
    kind !== "today" &&
    kind !== "tasks" &&
    kind !== "task" &&
    kind !== "calendar" &&
    kind !== "event" &&
    kind !== "notes" &&
    kind !== "note" &&
    kind !== "settings" &&
    kind !== "search"
  ) {
    return null;
  }

  const route: NativeRoute = { kind };
  const id = params.get("id")?.trim();
  const query = params.get("query")?.trim();

  if (id) {
    route.id = id;
  }

  if (query) {
    route.query = query;
  }

  return { kind: "route", route };
}

function parseMenuBarPanelAction(params: URLSearchParams): MenuBarPanelNavigation | null {
  const action = params.get("name");

  if (
    action !== "refresh" &&
    action !== "openSettings" &&
    action !== "showWindow" &&
    action !== "quit"
  ) {
    return null;
  }

  return { kind: "action", action };
}
