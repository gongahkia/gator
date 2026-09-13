import type { TaskDraft } from "./inspectors/TaskInspectorBody";
import type { CalendarEventDraft } from "./screens/calendar/types";

export type ConvertItemKind = "event" | "task" | "note";
export type ConvertSourceAction = "keep" | "replace";

export interface ConvertSourceCleanup {
  id: string;
  kind: ConvertItemKind;
}

export interface ConvertCommandDetail {
  cleanup?: ConvertSourceCleanup;
  target: ConvertItemKind;
  taskDraft?: Partial<Omit<TaskDraft, "mode">> | TaskDraft;
  noteDraft?: {
    body: string;
    id?: string;
    listId?: string;
    listTitle?: string;
    replaceSource?: boolean;
    tags?: string[];
    title: string;
  };
  eventDraft?: Partial<CalendarEventDraft>;
}

export function conversionCleanup(
  kind: ConvertItemKind,
  id: string,
  target: ConvertItemKind
): ConvertSourceCleanup | undefined {
  const replace = window.confirm(
    `Remove the original ${kind} after saving the converted ${target}? Cancel keeps the original.`
  );

  return replace ? { id, kind } : undefined;
}

export function dispatchConvertCommand(detail: ConvertCommandDetail): void {
  window.dispatchEvent(new CustomEvent("hcb:convert-command", { detail }));
}
