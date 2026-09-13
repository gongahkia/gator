import type { TaskCreateRequest, TaskUpdateRequest } from "@shared/ipc/contracts";
import type { useCoreViewModelSource } from "../../coreViewModelSource";
import type { TaskViewModel } from "../../coreViewModels";
import type { TaskDraft } from "../../inspectors/TaskInspectorBody";
import { copiedTitle } from "../../copyLabels";

type CoreViewModelSource = ReturnType<typeof useCoreViewModelSource>;

export function defaultTaskListId(source: CoreViewModelSource): string {
  return source.taskLists[0]?.id ?? "";
}

export function newTaskDraft(
  source: CoreViewModelSource,
  seed: Partial<Omit<TaskDraft, "mode">> = {}
): TaskDraft {
  return {
    mode: "create",
    title: seed.title ?? "",
    notes: seed.notes ?? "",
    dueDate: seed.dueDate ?? "",
    listId: seed.listId ?? defaultTaskListId(source),
    parentId: seed.parentId ?? "",
    priority: seed.priority ?? "none",
    plannedStart: seed.plannedStart ?? null,
    plannedEnd: seed.plannedEnd ?? null,
    durationMinutes: seed.durationMinutes ?? null,
    lockedSchedule: seed.lockedSchedule ?? false,
    snoozeUntil: seed.snoozeUntil ?? null,
    tags: seed.tags ?? []
  };
}

export function editTaskDraft(task: TaskViewModel): TaskDraft {
  return {
    mode: "edit",
    id: task.id,
    title: task.title,
    notes: task.detail,
    dueDate: task.dueDate ?? "",
    listId: task.listId,
    parentId: task.parentId ?? "",
    priority: task.priority,
    plannedStart: task.plannedStart ?? null,
    plannedEnd: task.plannedEnd ?? null,
    durationMinutes: task.durationMinutes ?? null,
    lockedSchedule: task.lockedSchedule ?? false,
    snoozeUntil: task.snoozeUntil ?? null,
    tags: task.tags ?? []
  };
}

export function duplicateTaskDraft(task: TaskViewModel): TaskDraft {
  return {
    mode: "create",
    title: copiedTitle(task.title, "Untitled task"),
    notes: task.detail,
    dueDate: task.dueDate ?? "",
    listId: task.listId,
    parentId: task.parentId ?? "",
    priority: task.priority,
    plannedStart: task.plannedStart ?? null,
    plannedEnd: task.plannedEnd ?? null,
    durationMinutes: task.durationMinutes ?? null,
    lockedSchedule: task.lockedSchedule ?? false,
    snoozeUntil: task.snoozeUntil ?? null,
    tags: task.tags ?? []
  };
}

export function taskCreatePayload(draft: TaskDraft): TaskCreateRequest {
  return {
    title: draft.title.trim(),
    notes: draft.notes.trim(),
    dueDate: draft.dueDate || null,
    listId: draft.listId,
    parentId: draft.parentId || null,
    priority: draft.priority,
    plannedStart: draft.plannedStart ?? null,
    plannedEnd: draft.plannedEnd ?? null,
    durationMinutes: draft.durationMinutes ?? null,
    lockedSchedule: draft.lockedSchedule ?? false,
    snoozeUntil: draft.snoozeUntil ?? null,
    tags: draft.tags ?? []
  };
}

export function taskUpdatePayload(draft: TaskDraft): TaskUpdateRequest {
  return {
    id: draft.id ?? "",
    title: draft.title.trim(),
    notes: draft.notes.trim(),
    dueDate: draft.dueDate || null,
    listId: draft.listId,
    parentId: draft.parentId || null,
    priority: draft.priority,
    snoozeUntil: draft.snoozeUntil ?? null
  };
}

export function taskParentOptions(tasks: TaskViewModel[], draft: TaskDraft): TaskViewModel[] {
  return tasks.filter(
    (task) => task.id !== draft.id && task.parentId === null && task.status !== "deleted"
  );
}

export function canSaveTaskDraft(draft: TaskDraft, mutationPending: boolean): boolean {
  return draft.title.trim().length > 0 && draft.listId.length > 0 && !mutationPending;
}

export function taskInspectorTitle(draft: TaskDraft): string {
  return draft.mode === "edit" ? draft.title || "Task" : "New task";
}
