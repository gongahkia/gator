import type {
  TaskListSummary,
  TaskSummary,
  TaskUpdateRequest
} from "@shared/ipc/contracts";
import type {
  TaskFilterViewModel,
  TaskGroupViewModel,
  TaskViewModel
} from "../coreViewModels";
import { getTaskById as getMockTaskById } from "../mockCoreViewModels";
import { dueLabel, slugId } from "./dateFormat";

export function taskFilters(
  openTasks: TaskViewModel[],
  completedTasks: TaskViewModel[],
  hiddenTasks: TaskViewModel[],
  deletedTasks: TaskViewModel[],
  taskLists: TaskListSummary[]
): TaskFilterViewModel[] {
  return [
    {
      id: "open",
      label: "Open",
      countLabel: String(openTasks.length),
      groups: taskGroups(openTasks, "open", taskLists),
      state: openTasks.length === 0 ? "empty" : "ready"
    },
    {
      id: "completed",
      label: "Completed",
      countLabel: String(completedTasks.length),
      groups: taskGroups(completedTasks, "completed", taskLists),
      state: completedTasks.length === 0 ? "empty" : "ready"
    },
    {
      id: "hidden",
      label: "Hidden",
      countLabel: String(hiddenTasks.length),
      groups: taskGroups(hiddenTasks, "hidden", taskLists),
      state: hiddenTasks.length === 0 ? "empty" : "ready"
    },
    {
      id: "deleted",
      label: "Deleted",
      countLabel: String(deletedTasks.length),
      groups: taskGroups(deletedTasks, "deleted", taskLists),
      state: deletedTasks.length === 0 ? "empty" : "ready"
    },
    {
      id: "empty",
      label: "Empty",
      countLabel: "0",
      groups: [],
      state: "empty"
    },
    {
      id: "error",
      label: "Error",
      countLabel: "!",
      groups: [],
      state: "error"
    }
  ];
}

function taskGroups(
  tasks: TaskViewModel[],
  state: "open" | "completed" | "hidden" | "deleted",
  taskLists: TaskListSummary[]
): TaskGroupViewModel[] {
  const byList = new Map<string, TaskViewModel[]>();
  const listOrder = new Map(taskLists.map((list, index) => [list.title, index]));

  for (const task of tasks) {
    const listTasks = byList.get(task.list) ?? [];
    listTasks.push(task);
    byList.set(task.list, listTasks);
  }

  return Array.from(byList, ([list, listTasks]) => ({
    id: `${state}-${slugId(list)}`,
    title: list,
    description: `${taskStateLabel(state)} tasks`,
    countLabel: `${listTasks.length} ${listTasks.length === 1 ? "task" : "tasks"}`,
    tasks: listTasks
  })).sort(
    (left, right) =>
      (listOrder.get(left.title) ?? Number.MAX_SAFE_INTEGER) -
        (listOrder.get(right.title) ?? Number.MAX_SAFE_INTEGER) ||
      left.title.localeCompare(right.title)
  );
}

function taskStateLabel(state: "open" | "completed" | "hidden" | "deleted"): string {
  if (state === "completed") {
    return "Completed";
  }

  if (state === "hidden") {
    return "Hidden";
  }

  if (state === "deleted") {
    return "Deleted";
  }

  return "Open";
}

export function groupChildTasks(tasks: TaskSummary[]): Map<string, TaskSummary[]> {
  const byParent = new Map<string, TaskSummary[]>();

  for (const task of tasks) {
    if (!task.parentId) {
      continue;
    }

    const children = byParent.get(task.parentId) ?? [];
    children.push(task);
    byParent.set(task.parentId, children);
  }

  for (const children of byParent.values()) {
    children.sort(
      (left, right) =>
        (left.sortOrder ?? 0) - (right.sortOrder ?? 0) ||
        left.updatedAt.localeCompare(right.updatedAt) ||
        left.id.localeCompare(right.id)
    );
  }

  return byParent;
}

export function stableTaskViewModel(
  task: TaskSummary,
  listTitle: string | undefined,
  children: TaskSummary[],
  cache: Map<string, { signature: string; viewModel: TaskViewModel }>
): TaskViewModel {
  const fixtureTask = getMockTaskById(task.id);
  const hasFixtureTask = fixtureTask.id === task.id;
  const signature = taskViewModelSignature(task, listTitle, children, hasFixtureTask ? fixtureTask : null);
  const cached = cache.get(task.id);

  if (cached?.signature === signature) {
    return cached.viewModel;
  }

  const subtasks = children.length > 0
    ? children.map((child) => ({
        id: child.id,
        title: child.title,
        completed: child.status === "completed"
      }))
    : hasFixtureTask
      ? fixtureTask.subtasks
      : [];
  const viewModel: TaskViewModel = {
    id: task.id,
    ...(task.accountId === undefined ? {} : { accountId: task.accountId }),
    listId: task.listId,
    parentId: task.parentId ?? null,
    title: task.title,
    detail: task.notes || (hasFixtureTask ? fixtureTask.detail : ""),
    list: listTitle ?? task.listId,
    dueDate: task.dueAt ? task.dueAt.slice(0, 10) : null,
    dueLabel: task.status === "completed" ? "Done" : dueLabel(task.dueAt),
    sortOrder: task.sortOrder,
    updatedAt: task.updatedAt,
    plannedStart: task.plannedStart ?? null,
    plannedEnd: task.plannedEnd ?? null,
    durationMinutes: task.durationMinutes ?? null,
    lockedSchedule: task.lockedSchedule ?? false,
    snoozeUntil: task.snoozeUntil ?? null,
    tags: task.tags ?? [],
    priority: task.priority ?? (hasFixtureTask ? fixtureTask.priority : "none"),
    status: taskStatusViewModel(task.status),
    mutationState: task.mutationState,
    subtasks
  };

  cache.set(task.id, { signature, viewModel });
  return viewModel;
}

function taskViewModelSignature(
  task: TaskSummary,
  listTitle: string | undefined,
  children: TaskSummary[],
  fixtureTask: TaskViewModel | null
): string {
  return [
    task.id,
    task.accountId ?? "",
    task.listId,
    task.parentId ?? "",
    task.title,
    task.notes ?? "",
    task.status,
    task.dueAt ?? "",
    task.sortOrder ?? "",
    task.updatedAt ?? "",
    task.priority ?? "",
    task.plannedStart ?? "",
    task.plannedEnd ?? "",
    task.durationMinutes ?? "",
    task.lockedSchedule ? "1" : "0",
    task.snoozeUntil ?? "",
    task.mutationState ?? "",
    listTitle ?? "",
    (task.tags ?? []).join("\u001f"),
    children.map((child) => `${child.id}\u001f${child.title}\u001f${child.status}`).join("\u001e"),
    fixtureTask
      ? [
          fixtureTask.detail,
          fixtureTask.priority,
          fixtureTask.subtasks.map((child) => `${child.id}\u001f${child.title}\u001f${child.completed}`).join("\u001e")
        ].join("\u001d")
      : ""
  ].join("\u001c");
}

function taskStatusViewModel(status: TaskSummary["status"]): TaskViewModel["status"] {
  if (status === "completed" || status === "hidden" || status === "deleted") {
    return status;
  }

  return "open";
}

export function optimisticTaskPatch(task: TaskSummary, request: TaskUpdateRequest): TaskSummary {
  return {
    ...task,
    ...(request.title === undefined ? {} : { title: request.title }),
    ...(request.notes === undefined ? {} : { notes: request.notes }),
    ...(request.dueDate === undefined
      ? {}
      : { dueAt: request.dueDate ? `${request.dueDate}T00:00:00.000Z` : null }),
    ...(request.listId === undefined ? {} : { listId: request.listId }),
    ...(request.parentId === undefined ? {} : { parentId: request.parentId }),
    ...(request.priority === undefined ? {} : { priority: request.priority }),
    ...(request.plannedStart === undefined ? {} : { plannedStart: request.plannedStart }),
    ...(request.plannedEnd === undefined ? {} : { plannedEnd: request.plannedEnd }),
    ...(request.durationMinutes === undefined ? {} : { durationMinutes: request.durationMinutes }),
    ...(request.lockedSchedule === undefined ? {} : { lockedSchedule: request.lockedSchedule }),
    ...(request.snoozeUntil === undefined ? {} : { snoozeUntil: request.snoozeUntil }),
    ...(request.tags === undefined ? {} : { tags: request.tags }),
    updatedAt: new Date().toISOString(),
    mutationState: "queued"
  };
}

export function uniqueTasks(tasks: TaskSummary[]): TaskSummary[] {
  return Array.from(new Map(tasks.map((task) => [task.id, task])).values());
}

export function missingTask(taskId: string): TaskViewModel {
  return {
    id: taskId,
    listId: "unknown",
    parentId: null,
    title: "Missing task",
    detail: "The task is no longer available.",
    list: "Unknown",
    dueDate: null,
    dueLabel: "Unknown",
    priority: "none",
    status: "open",
    subtasks: []
  };
}
