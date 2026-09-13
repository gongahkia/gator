import type { SavedTaskView } from "@shared/ipc/contracts";
import type {
  CorePriority,
  TaskFilterId,
  TaskGroupViewModel,
  TaskViewModel
} from "../../coreViewModels";
import { addLocalDays, dateOnlyFromLocalDate } from "./taskDateUtils";

export type TaskPerspectiveId = "inbox" | "forecast" | "review" | "tags" | "projects" | "saved";

export interface TaskPerspectiveTab {
  id: TaskPerspectiveId;
  label: string;
}

export interface TaskPerspectiveViewModel {
  description: string;
  groups: TaskGroupViewModel[];
  state: "ready" | "empty" | "error";
}

export const taskPerspectiveTabs: TaskPerspectiveTab[] = [
  { id: "inbox", label: "Inbox" },
  { id: "forecast", label: "Forecast" },
  { id: "review", label: "Review" },
  { id: "tags", label: "Tags" },
  { id: "projects", label: "Projects" },
  { id: "saved", label: "Saved" }
];

function taskCountLabel(count: number): string {
  return `${count} ${count === 1 ? "task" : "tasks"}`;
}

function taskMatchesFilter(task: TaskViewModel, filterId: TaskFilterId): boolean {
  if (filterId === "open") {
    return task.status === "open";
  }

  if (filterId === "completed" || filterId === "hidden" || filterId === "deleted") {
    return task.status === filterId;
  }

  return false;
}

function taskListTitle(taskLists: readonly { id: string; title: string }[], listId: string): string {
  return taskLists.find((list) => list.id === listId)?.title ?? listId;
}

function taskPriorityRank(priority: CorePriority): number {
  if (priority === "high") {
    return 0;
  }

  if (priority === "medium") {
    return 1;
  }

  if (priority === "low") {
    return 2;
  }

  return 3;
}

function sortPerspectiveTasks(tasks: TaskViewModel[], sortBy: SavedTaskView["sortBy"] = "dueDate"): TaskViewModel[] {
  return [...tasks].sort((left, right) => {
    if (sortBy === "title") {
      return left.title.localeCompare(right.title);
    }

    if (sortBy === "updatedAt") {
      return Date.parse(right.updatedAt ?? "") - Date.parse(left.updatedAt ?? "");
    }

    if (sortBy === "priority") {
      return taskPriorityRank(left.priority) - taskPriorityRank(right.priority);
    }

    return (left.dueDate ?? "9999-12-31").localeCompare(right.dueDate ?? "9999-12-31");
  });
}

function createTaskGroup(id: string, title: string, description: string, tasks: TaskViewModel[]): TaskGroupViewModel {
  return {
    id,
    title,
    description,
    countLabel: taskCountLabel(tasks.length),
    tasks
  };
}

function dateRangeLabel(date: string): string {
  if (!date) {
    return "No due date";
  }

  return date;
}

function buildGroupedTaskPerspective(
  groupBy: SavedTaskView["groupBy"],
  tasks: TaskViewModel[],
  taskLists: readonly { id: string; title: string }[],
  sortBy: SavedTaskView["sortBy"] = "dueDate"
): TaskGroupViewModel[] {
  if (groupBy === "none") {
    return [createTaskGroup("all", "All matching tasks", "Saved perspective matches", sortPerspectiveTasks(tasks, sortBy))];
  }

  const groups = new Map<string, { title: string; tasks: TaskViewModel[] }>();

  for (const task of tasks) {
    if (groupBy === "tag") {
      const tags = task.tags?.length ? task.tags : ["Untagged"];

      for (const tag of tags) {
        const key = tag.toLowerCase();
        const group = groups.get(key) ?? { title: tag, tasks: [] };
        group.tasks.push(task);
        groups.set(key, group);
      }

      continue;
    }

    const key =
      groupBy === "dueDate"
        ? task.dueDate ?? "none"
        : groupBy === "list"
          ? task.listId
          : task.status;
    const title =
      groupBy === "dueDate"
        ? dateRangeLabel(task.dueDate ?? "")
        : groupBy === "list"
          ? taskListTitle(taskLists, task.listId)
          : task.status === "open"
            ? "Active"
            : `${task.status[0]?.toUpperCase() ?? ""}${task.status.slice(1)}`;
    const group = groups.get(key) ?? { title, tasks: [] };
    group.tasks.push(task);
    groups.set(key, group);
  }

  return Array.from(groups.entries())
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([key, group]) =>
      createTaskGroup(
        `saved-${groupBy}-${key}`,
        group.title,
        groupBy === "list" ? "Project list" : `Grouped by ${groupBy}`,
        sortPerspectiveTasks(group.tasks, sortBy)
      )
    );
}

function taskDueBucket(task: TaskViewModel, today: string, inFourteenDays: string): SavedTaskView["filters"]["due"] | null {
  if (!task.dueDate) {
    return "none";
  }

  if (task.dueDate < today) {
    return "overdue";
  }

  if (task.dueDate === today) {
    return "today";
  }

  if (task.dueDate <= inFourteenDays) {
    return "next14";
  }

  return null;
}

function taskStatusForSavedView(task: TaskViewModel): "active" | "completed" | "hidden" | "deleted" {
  return task.status === "open" ? "active" : task.status;
}

function taskMatchesSavedView(
  task: TaskViewModel,
  view: SavedTaskView,
  today: string,
  inFourteenDays: string
): boolean {
  const filters = view.filters;

  if (filters.statuses?.length && !filters.statuses.includes(taskStatusForSavedView(task))) {
    return false;
  }

  if (filters.listIds?.length && !filters.listIds.includes(task.listId)) {
    return false;
  }

  if (filters.tags?.length) {
    const taskTags = new Set((task.tags ?? []).map((tag) => tag.toLowerCase()));

    if (!filters.tags.every((tag) => taskTags.has(tag.toLowerCase()))) {
      return false;
    }
  }

  if (filters.due && taskDueBucket(task, today, inFourteenDays) !== filters.due) {
    return false;
  }

  if (filters.planned === "planned" && !task.plannedStart) {
    return false;
  }

  if (filters.planned === "unplanned" && task.plannedStart) {
    return false;
  }

  return true;
}

export function savedTaskViewFilterChips(
  view: SavedTaskView,
  taskLists: readonly { id: string; title: string }[]
): string[] {
  const chips: string[] = [];
  const filters = view.filters;

  if (filters.statuses?.length) {
    chips.push(`Status: ${filters.statuses.join(", ")}`);
  }

  if (filters.listIds?.length) {
    chips.push(`Lists: ${filters.listIds.map((id) => taskListTitle(taskLists, id)).join(", ")}`);
  }

  if (filters.tags?.length) {
    chips.push(`Tags: ${filters.tags.join(", ")}`);
  }

  if (filters.due) {
    chips.push(`Due: ${filters.due}`);
  }

  if (filters.planned) {
    chips.push(`Plan: ${filters.planned}`);
  }

  chips.push(`Group: ${view.groupBy}`);
  chips.push(`Sort: ${view.sortBy}`);
  return chips;
}

function buildSavedTaskPerspective(
  view: SavedTaskView,
  tasks: TaskViewModel[],
  taskLists: readonly { id: string; title: string }[],
  now: Date
): TaskPerspectiveViewModel {
  const today = dateOnlyFromLocalDate(now);
  const inFourteenDays = dateOnlyFromLocalDate(addLocalDays(now, 14));
  const matchingTasks = tasks.filter((task) => taskMatchesSavedView(task, view, today, inFourteenDays));
  const groups = buildGroupedTaskPerspective(view.groupBy, matchingTasks, taskLists, view.sortBy);

  return {
    description: `${taskCountLabel(matchingTasks.length)} in ${view.name}`,
    groups,
    state: matchingTasks.length > 0 ? "ready" : "empty"
  };
}

export function buildTaskPerspective(
  perspectiveId: TaskPerspectiveId,
  tasks: TaskViewModel[],
  taskLists: readonly { id: string; title: string }[],
  filterId: TaskFilterId,
  savedView: SavedTaskView | null,
  now: Date
): TaskPerspectiveViewModel {
  if (filterId === "error") {
    return { description: "Recoverable renderer error state", groups: [], state: "error" };
  }

  if (filterId === "empty") {
    return { description: "Empty filtered state", groups: [], state: "empty" };
  }

  if (perspectiveId === "saved") {
    return savedView
      ? buildSavedTaskPerspective(savedView, tasks, taskLists, now)
      : { description: "Select a saved perspective", groups: [], state: "empty" };
  }

  const statusFilteredTasks = tasks.filter((task) => taskMatchesFilter(task, filterId));
  const today = dateOnlyFromLocalDate(now);
  const inFourteenDays = dateOnlyFromLocalDate(addLocalDays(now, 14));
  const inboxListId =
    taskLists.find((list) => list.title.trim().toLowerCase() === "inbox")?.id ?? taskLists[0]?.id ?? "";
  let groups: TaskGroupViewModel[] = [];

  if (perspectiveId === "inbox") {
    const inboxTasks = statusFilteredTasks.filter(
      (task) =>
        task.status === "open" &&
        (task.listId === inboxListId || (task.parentId === null && !task.plannedStart))
    );
    groups = [createTaskGroup("perspective-inbox", "Inbox", "Active root tasks without a planned slot", sortPerspectiveTasks(inboxTasks))];
  } else if (perspectiveId === "forecast") {
    const byDate = statusFilteredTasks.filter(
      (task) => task.dueDate !== null && task.dueDate >= today && task.dueDate <= inFourteenDays
    );
    groups = buildGroupedTaskPerspective("dueDate", byDate, taskLists);
  } else if (perspectiveId === "review") {
    const reviewBefore = now.getTime() - 14 * 24 * 60 * 60 * 1000;
    const reviewTasks = statusFilteredTasks.filter(
      (task) => task.status === "open" && Date.parse(task.updatedAt ?? "") < reviewBefore
    );
    groups = [createTaskGroup("perspective-review", "Needs review", "Active tasks untouched for 14 days", sortPerspectiveTasks(reviewTasks, "updatedAt"))];
  } else if (perspectiveId === "tags") {
    const taggedTasks = statusFilteredTasks.filter((task) => (task.tags ?? []).length > 0);
    groups = buildGroupedTaskPerspective("tag", taggedTasks, taskLists, "priority");
  } else {
    groups = buildGroupedTaskPerspective("list", statusFilteredTasks, taskLists, "priority");
  }

  const count = groups.reduce((total, group) => total + group.tasks.length, 0);

  return {
    description: `${taskCountLabel(count)} in ${taskPerspectiveTabs.find((tab) => tab.id === perspectiveId)?.label ?? "Perspective"}`,
    groups: groups.filter((group) => group.tasks.length > 0),
    state: count > 0 ? "ready" : "empty"
  };
}
