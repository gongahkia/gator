import { nativeActionSchema } from "@shared/ipc/contracts";
import type { NativeAction } from "@shared/ipc/contracts";
import { HCB_DEEP_LINK_SCHEME } from "./types";

export function parseHotCrossBunsDeepLink(rawUrl: string): NativeAction | null {
  let parsed: URL;

  try {
    parsed = new URL(rawUrl);
  } catch {
    return null;
  }

  if (parsed.protocol !== `${HCB_DEEP_LINK_SCHEME}:`) {
    return null;
  }

  const host = parsed.hostname.toLowerCase();
  const decodedId = safeDecodeURIComponent(parsed.pathname.replace(/^\/+/, ""));
  const id = decodedId?.trim() ?? null;
  const query = parsed.searchParams.get("q")?.trim();

  if (id === null) {
    return null;
  }

  if (host === "today" || host === "") {
    return safeNativeAction({ type: "openRoute", route: { kind: "today" } });
  }

  if (host === "settings") {
    return safeNativeAction({ type: "openSettings" });
  }

  if (host === "search") {
    return query ? safeNativeAction({ type: "openRoute", route: { kind: "search", query } }) : null;
  }

  if (host === "task" || host === "tasks") {
    return id
      ? safeNativeAction({ type: "openRoute", route: { kind: "task", id } })
      : safeNativeAction({ type: "openRoute", route: { kind: "tasks" } });
  }

  if (host === "event" || host === "calendar") {
    return id
      ? safeNativeAction({ type: "openRoute", route: { kind: "event", id } })
      : safeNativeAction({ type: "openRoute", route: { kind: "calendar" } });
  }

  if (host === "note" || host === "notes") {
    return id
      ? safeNativeAction({ type: "openRoute", route: { kind: "note", id } })
      : safeNativeAction({ type: "openRoute", route: { kind: "notes" } });
  }

  return null;
}

function safeNativeAction(action: NativeAction): NativeAction | null {
  const parsed = nativeActionSchema.safeParse(action);

  return parsed.success ? parsed.data : null;
}

function safeDecodeURIComponent(value: string): string | null {
  try {
    return decodeURIComponent(value);
  } catch {
    return null;
  }
}
