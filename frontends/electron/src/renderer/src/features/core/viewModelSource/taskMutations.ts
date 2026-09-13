import { useCallback, useRef, useState } from "react";
import type { Dispatch, SetStateAction } from "react";
import type {
  CalendarEventCompletionScope,
  CalendarEventSummary,
  ScheduledTaskBlockCreateRequest,
  ScheduledTaskBlockMoveRequest,
  ScheduledTaskBlockSummary,
  TaskBulkRescheduleRequest,
  TaskCreateRequest,
  TaskListCreateRequest,
  TaskListRenameRequest,
  TaskListSummary,
  TaskMoveRequest,
  TaskSummary,
  TaskUpdateRequest
} from "@shared/ipc/contracts";
import { optimisticScheduledBlockPatch } from "./calendarViewModels";
import { optimisticTaskPatch } from "./taskViewModels";
import type { CoreDataLoadState, TaskMutationUiState } from "./types";

interface UseTaskMutationsOptions {
  setLoadState: Dispatch<SetStateAction<CoreDataLoadState>>;
  refreshSyncStatus: () => void;
  refreshUndoStatus: () => void;
}

export function useTaskMutations({
  setLoadState,
  refreshSyncStatus,
  refreshUndoStatus
}: UseTaskMutationsOptions): {
  taskMutation: TaskMutationUiState;
  clearTaskMutationError: () => void;
  retryLastTaskMutation: () => void;
  createTask: (request: TaskCreateRequest) => Promise<boolean>;
  updateTask: (request: TaskUpdateRequest) => Promise<boolean>;
  completeTask: (taskId: string) => Promise<boolean>;
  reopenTask: (taskId: string) => Promise<boolean>;
  completeEvent: (eventId: string, scope?: CalendarEventCompletionScope) => Promise<boolean>;
  reopenEvent: (eventId: string, scope?: CalendarEventCompletionScope) => Promise<boolean>;
  moveTask: (request: TaskMoveRequest) => Promise<boolean>;
  bulkRescheduleTasks: (request: TaskBulkRescheduleRequest) => Promise<boolean>;
  deleteTask: (taskId: string) => Promise<boolean>;
  createTaskList: (request: TaskListCreateRequest) => Promise<boolean>;
  renameTaskList: (request: TaskListRenameRequest) => Promise<boolean>;
  deleteTaskList: (taskListId: string) => Promise<boolean>;
  scheduleTaskBlock: (request: ScheduledTaskBlockCreateRequest) => Promise<boolean>;
  moveScheduledTaskBlock: (request: ScheduledTaskBlockMoveRequest) => Promise<boolean>;
  unscheduleTaskBlock: (blockId: string) => Promise<boolean>;
} {
  const [taskMutation, setTaskMutation] = useState<TaskMutationUiState>({ pending: false });
  const retryTaskMutation = useRef<() => void>(() => undefined);

  const setTasksSnapshot = useCallback((updater: (tasks: TaskSummary[]) => TaskSummary[]) => {
    setLoadState((current) => ({
      ...current,
      snapshot: {
        ...current.snapshot,
        tasks: updater(current.snapshot.tasks)
      }
    }));
  }, [setLoadState]);

  const setScheduledBlocksSnapshot = useCallback(
    (updater: (blocks: ScheduledTaskBlockSummary[]) => ScheduledTaskBlockSummary[]) => {
      setLoadState((current) => ({
        ...current,
        snapshot: {
          ...current.snapshot,
          scheduledTaskBlocks: updater(current.snapshot.scheduledTaskBlocks)
        }
      }));
    },
    [setLoadState]
  );

  const setEventsSnapshot = useCallback(
    (updater: (events: CalendarEventSummary[]) => CalendarEventSummary[]) => {
      setLoadState((current) => ({
        ...current,
        snapshot: {
          ...current.snapshot,
          events: updater(current.snapshot.events)
        }
      }));
    },
    [setLoadState]
  );

  const setTaskListsSnapshot = useCallback(
    (updater: (taskLists: TaskListSummary[]) => TaskListSummary[]) => {
      setLoadState((current) => ({
        ...current,
        snapshot: {
          ...current.snapshot,
          taskLists: updater(current.snapshot.taskLists)
        }
      }));
    },
    [setLoadState]
  );

  const beginTaskMutation = useCallback((incrementPendingMutationCount = true) => {
    setTaskMutation({ pending: true });
    if (!incrementPendingMutationCount) {
      return;
    }

    setLoadState((current) => ({
      ...current,
      snapshot: {
        ...current.snapshot,
        syncStatus: {
          ...current.snapshot.syncStatus,
          pendingMutationCount: current.snapshot.syncStatus.pendingMutationCount + 1
        }
      }
    }));
  }, [setLoadState]);

  const failTaskMutation = useCallback((message: string, retry: () => void) => {
    retryTaskMutation.current = retry;
    setTaskMutation({ pending: false, error: message });
    refreshSyncStatus();
  }, [refreshSyncStatus]);

  const finishTaskMutation = useCallback(() => {
    setTaskMutation({ pending: false });
    refreshSyncStatus();
    refreshUndoStatus();
  }, [refreshSyncStatus, refreshUndoStatus]);

  const createTask = useCallback(
    async (request: TaskCreateRequest): Promise<boolean> => {
      if (!window.hcb) {
        failTaskMutation("Task writes require the preload bridge.", () => void createTask(request));
        return false;
      }

      const optimisticId = `optimistic:task:${Date.now()}:${Math.random().toString(36).slice(2)}`;
      const now = new Date().toISOString();
      const optimisticTask: TaskSummary = {
        id: optimisticId,
        listId: request.listId,
        title: request.title,
        status: "active",
        dueAt: request.dueDate ? `${request.dueDate}T00:00:00.000Z` : null,
        updatedAt: now,
        notes: request.notes ?? "",
        parentId: request.parentId ?? null,
        priority: request.priority ?? "none",
        plannedStart: request.plannedStart ?? null,
        plannedEnd: request.plannedEnd ?? null,
        durationMinutes: request.durationMinutes ?? null,
        lockedSchedule: request.lockedSchedule ?? false,
        snoozeUntil: request.snoozeUntil ?? null,
        tags: request.tags ?? [],
        mutationState: "queued"
      };

      beginTaskMutation();
      setTasksSnapshot((tasks) => [optimisticTask, ...tasks]);

      const result = await window.hcb.tasks.create(request);

      if (result.ok) {
        setTasksSnapshot((tasks) =>
          tasks.map((task) => (task.id === optimisticId ? result.data : task))
        );
        finishTaskMutation();
        return true;
      }

      setTasksSnapshot((tasks) => tasks.filter((task) => task.id !== optimisticId));
      failTaskMutation(result.error.message, () => void createTask(request));
      return false;
    },
    [beginTaskMutation, failTaskMutation, finishTaskMutation, setTasksSnapshot]
  );

  const updateTask = useCallback(
    async (request: TaskUpdateRequest): Promise<boolean> => {
      if (!window.hcb) {
        failTaskMutation("Task writes require the preload bridge.", () => void updateTask(request));
        return false;
      }

      let previousTasks: TaskSummary[] = [];
      beginTaskMutation();
      setTasksSnapshot((tasks) => {
        previousTasks = tasks;
        return tasks.map((task) => (task.id === request.id ? optimisticTaskPatch(task, request) : task));
      });

      const result = await window.hcb.tasks.update(request);

      if (result.ok) {
        setTasksSnapshot((tasks) => tasks.map((task) => (task.id === request.id ? result.data : task)));
        finishTaskMutation();
        return true;
      }

      setTasksSnapshot(() => previousTasks);
      failTaskMutation(result.error.message, () => void updateTask(request));
      return false;
    },
    [beginTaskMutation, failTaskMutation, finishTaskMutation, setTasksSnapshot]
  );

  const completeTask = useCallback(
    async (taskId: string): Promise<boolean> => {
      if (!window.hcb) {
        failTaskMutation("Task writes require the preload bridge.", () => void completeTask(taskId));
        return false;
      }

      let previousTasks: TaskSummary[] = [];
      beginTaskMutation();
      setTasksSnapshot((tasks) => {
        previousTasks = tasks;
        return tasks.map((task) =>
          task.id === taskId
            ? { ...task, status: "completed", mutationState: "queued", updatedAt: new Date().toISOString() }
            : task
        );
      });

      const result = await window.hcb.tasks.complete({ id: taskId });

      if (result.ok) {
        setTasksSnapshot((tasks) => tasks.map((task) => (task.id === taskId ? result.data : task)));
        finishTaskMutation();
        return true;
      }

      setTasksSnapshot(() => previousTasks);
      failTaskMutation(result.error.message, () => void completeTask(taskId));
      return false;
    },
    [beginTaskMutation, failTaskMutation, finishTaskMutation, setTasksSnapshot]
  );

  const reopenTask = useCallback(
    async (taskId: string): Promise<boolean> => {
      if (!window.hcb) {
        failTaskMutation("Task writes require the preload bridge.", () => void reopenTask(taskId));
        return false;
      }

      let previousTasks: TaskSummary[] = [];
      beginTaskMutation();
      setTasksSnapshot((tasks) => {
        previousTasks = tasks;
        return tasks.map((task) =>
          task.id === taskId
            ? { ...task, status: "active", mutationState: "queued", updatedAt: new Date().toISOString() }
            : task
        );
      });

      const result = await window.hcb.tasks.reopen({ id: taskId });

      if (result.ok) {
        setTasksSnapshot((tasks) => tasks.map((task) => (task.id === taskId ? result.data : task)));
        finishTaskMutation();
        return true;
      }

      setTasksSnapshot(() => previousTasks);
      failTaskMutation(result.error.message, () => void reopenTask(taskId));
      return false;
    },
    [beginTaskMutation, failTaskMutation, finishTaskMutation, setTasksSnapshot]
  );

  const setEventCompletion = useCallback(
    async (
      eventId: string,
      completed: boolean,
      scope: CalendarEventCompletionScope = "occurrence"
    ): Promise<boolean> => {
      if (!window.hcb) {
        const retry = () => void setEventCompletion(eventId, completed, scope);
        failTaskMutation("Calendar event writes require the preload bridge.", retry);
        return false;
      }

      let previousEvents: CalendarEventSummary[] = [];
      const now = new Date().toISOString();
      beginTaskMutation(false);
      setEventsSnapshot((events) => {
        previousEvents = events;
        return optimisticEventCompletionPatch(events, eventId, completed ? now : null, scope);
      });

      const result = completed
        ? await window.hcb.calendar.complete({ id: eventId, scope })
        : await window.hcb.calendar.reopen({ id: eventId, scope });

      if (result.ok) {
        setEventsSnapshot((events) =>
          events.map((event) => (event.id === result.data.id ? result.data : event))
        );
        finishTaskMutation();
        return true;
      }

      setEventsSnapshot(() => previousEvents);
      failTaskMutation(result.error.message, () => void setEventCompletion(eventId, completed, scope));
      return false;
    },
    [beginTaskMutation, failTaskMutation, finishTaskMutation, setEventsSnapshot]
  );

  const completeEvent = useCallback(
    (eventId: string, scope?: CalendarEventCompletionScope): Promise<boolean> =>
      setEventCompletion(eventId, true, scope),
    [setEventCompletion]
  );

  const reopenEvent = useCallback(
    (eventId: string, scope?: CalendarEventCompletionScope): Promise<boolean> =>
      setEventCompletion(eventId, false, scope),
    [setEventCompletion]
  );

  const moveTask = useCallback(
    async (request: TaskMoveRequest): Promise<boolean> => {
      if (!window.hcb) {
        failTaskMutation("Task writes require the preload bridge.", () => void moveTask(request));
        return false;
      }

      let previousTasks: TaskSummary[] = [];
      beginTaskMutation();
      setTasksSnapshot((tasks) => {
        previousTasks = tasks;
        return tasks.map((task) =>
          task.id === request.id
            ? optimisticTaskPatch(task, {
                id: request.id,
                ...(request.listId === undefined ? {} : { listId: request.listId }),
                ...(request.parentId === undefined ? {} : { parentId: request.parentId })
              })
            : task
        );
      });

      const result = await window.hcb.tasks.move(request);

      if (result.ok) {
        setTasksSnapshot((tasks) => tasks.map((task) => (task.id === request.id ? result.data : task)));
        finishTaskMutation();
        return true;
      }

      setTasksSnapshot(() => previousTasks);
      failTaskMutation(result.error.message, () => void moveTask(request));
      return false;
    },
    [beginTaskMutation, failTaskMutation, finishTaskMutation, setTasksSnapshot]
  );

  const bulkRescheduleTasks = useCallback(
    async (request: TaskBulkRescheduleRequest): Promise<boolean> => {
      if (!window.hcb) {
        failTaskMutation("Bulk task reschedule requires the preload bridge.", () => void bulkRescheduleTasks(request));
        return false;
      }

      const ids = new Set(request.taskIds);
      let previousTasks: TaskSummary[] = [];
      beginTaskMutation();
      setTasksSnapshot((tasks) => {
        previousTasks = tasks;
        return tasks.map((task) =>
          ids.has(task.id)
            ? optimisticTaskPatch(task, {
                id: task.id,
                dueDate: request.dueDate
              })
            : task
        );
      });

      const result = await window.hcb.tasks.bulkReschedule(request);

      if (result.ok) {
        finishTaskMutation();
        return true;
      }

      setTasksSnapshot(() => previousTasks);
      failTaskMutation(result.error.message, () => void bulkRescheduleTasks(request));
      return false;
    },
    [beginTaskMutation, failTaskMutation, finishTaskMutation, setTasksSnapshot]
  );

  const deleteTask = useCallback(
    async (taskId: string): Promise<boolean> => {
      if (!window.hcb) {
        failTaskMutation("Task writes require the preload bridge.", () => void deleteTask(taskId));
        return false;
      }

      let previousTasks: TaskSummary[] = [];
      let previousEvents: CalendarEventSummary[] = [];
      beginTaskMutation();
      setLoadState((current) => {
        previousTasks = current.snapshot.tasks;
        previousEvents = current.snapshot.events;
        return {
          ...current,
          snapshot: {
            ...current.snapshot,
            events: current.snapshot.events.filter((event) => event.linkedTaskId !== taskId),
            tasks: current.snapshot.tasks.filter((task) => task.id !== taskId)
          }
        };
      });

      const result = await window.hcb.tasks.delete({ id: taskId });

      if (result.ok) {
        finishTaskMutation();
        return true;
      }

      setLoadState((current) => ({
        ...current,
        snapshot: {
          ...current.snapshot,
          events: previousEvents,
          tasks: previousTasks
        }
      }));
      failTaskMutation(result.error.message, () => void deleteTask(taskId));
      return false;
    },
    [beginTaskMutation, failTaskMutation, finishTaskMutation, setLoadState]
  );

  const createTaskList = useCallback(
    async (request: TaskListCreateRequest): Promise<boolean> => {
      if (!window.hcb) {
        failTaskMutation("Task list writes require the preload bridge.", () => void createTaskList(request));
        return false;
      }

      const optimisticId = `optimistic:list:${Date.now()}`;
      const now = new Date().toISOString();

      beginTaskMutation();
      setTaskListsSnapshot((taskLists) => [
        ...taskLists,
        { id: optimisticId, title: request.title, updatedAt: now, taskCount: 0, activeTaskCount: 0 }
      ]);

      const result = await window.hcb.tasks.createTaskList(request);

      if (result.ok) {
        setTaskListsSnapshot((taskLists) =>
          taskLists.map((taskList) => (taskList.id === optimisticId ? result.data : taskList))
        );
        finishTaskMutation();
        return true;
      }

      setTaskListsSnapshot((taskLists) => taskLists.filter((taskList) => taskList.id !== optimisticId));
      failTaskMutation(result.error.message, () => void createTaskList(request));
      return false;
    },
    [beginTaskMutation, failTaskMutation, finishTaskMutation, setTaskListsSnapshot]
  );

  const renameTaskList = useCallback(
    async (request: TaskListRenameRequest): Promise<boolean> => {
      if (!window.hcb) {
        failTaskMutation("Task list writes require the preload bridge.", () => void renameTaskList(request));
        return false;
      }

      let previousTaskLists: TaskListSummary[] = [];
      beginTaskMutation();
      setTaskListsSnapshot((taskLists) => {
        previousTaskLists = taskLists;
        return taskLists.map((taskList) =>
          taskList.id === request.id ? { ...taskList, title: request.title } : taskList
        );
      });

      const result = await window.hcb.tasks.renameTaskList(request);

      if (result.ok) {
        setTaskListsSnapshot((taskLists) =>
          taskLists.map((taskList) => (taskList.id === request.id ? result.data : taskList))
        );
        finishTaskMutation();
        return true;
      }

      setTaskListsSnapshot(() => previousTaskLists);
      failTaskMutation(result.error.message, () => void renameTaskList(request));
      return false;
    },
    [beginTaskMutation, failTaskMutation, finishTaskMutation, setTaskListsSnapshot]
  );

  const deleteTaskList = useCallback(
    async (taskListId: string): Promise<boolean> => {
      if (!window.hcb) {
        failTaskMutation("Task list writes require the preload bridge.", () => void deleteTaskList(taskListId));
        return false;
      }

      let previousTaskLists: TaskListSummary[] = [];
      let previousTasks: TaskSummary[] = [];
      beginTaskMutation();
      setTaskListsSnapshot((taskLists) => {
        previousTaskLists = taskLists;
        return taskLists.filter((taskList) => taskList.id !== taskListId);
      });
      setTasksSnapshot((tasks) => {
        previousTasks = tasks;
        return tasks.filter((task) => task.listId !== taskListId);
      });

      const result = await window.hcb.tasks.deleteTaskList({ id: taskListId });

      if (result.ok) {
        finishTaskMutation();
        return true;
      }

      setTaskListsSnapshot(() => previousTaskLists);
      setTasksSnapshot(() => previousTasks);
      failTaskMutation(result.error.message, () => void deleteTaskList(taskListId));
      return false;
    },
    [beginTaskMutation, failTaskMutation, finishTaskMutation, setTaskListsSnapshot, setTasksSnapshot]
  );

  const scheduleTaskBlock = useCallback(
    async (request: ScheduledTaskBlockCreateRequest): Promise<boolean> => {
      if (!window.hcb) {
        failTaskMutation("Task scheduling requires the preload bridge.", () => void scheduleTaskBlock(request));
        return false;
      }

      beginTaskMutation();
      const result = await window.hcb.calendar.scheduleTaskBlock(request);

      if (result.ok) {
        setScheduledBlocksSnapshot((blocks) => [result.data, ...blocks]);
        finishTaskMutation();
        return true;
      }

      failTaskMutation(result.error.message, () => void scheduleTaskBlock(request));
      return false;
    },
    [beginTaskMutation, failTaskMutation, finishTaskMutation, setScheduledBlocksSnapshot]
  );

  const moveScheduledTaskBlock = useCallback(
    async (request: ScheduledTaskBlockMoveRequest): Promise<boolean> => {
      if (!window.hcb) {
        failTaskMutation("Scheduled task moves require the preload bridge.", () => void moveScheduledTaskBlock(request));
        return false;
      }

      let previousBlocks: ScheduledTaskBlockSummary[] = [];
      beginTaskMutation();
      setScheduledBlocksSnapshot((blocks) => {
        previousBlocks = blocks;
        return blocks.map((block) =>
          block.id === request.id ? optimisticScheduledBlockPatch(block, request) : block
        );
      });

      const result = await window.hcb.calendar.moveScheduledTaskBlock(request);

      if (result.ok) {
        setScheduledBlocksSnapshot((blocks) =>
          blocks.map((block) => (block.id === request.id ? result.data : block))
        );
        finishTaskMutation();
        return true;
      }

      setScheduledBlocksSnapshot(() => previousBlocks);
      failTaskMutation(result.error.message, () => void moveScheduledTaskBlock(request));
      return false;
    },
    [beginTaskMutation, failTaskMutation, finishTaskMutation, setScheduledBlocksSnapshot]
  );

  const unscheduleTaskBlock = useCallback(
    async (blockId: string): Promise<boolean> => {
      if (!window.hcb) {
        failTaskMutation("Unscheduling requires the preload bridge.", () => void unscheduleTaskBlock(blockId));
        return false;
      }

      let previousBlocks: ScheduledTaskBlockSummary[] = [];
      let previousEvents: CalendarEventSummary[] = [];
      let removedEventId: string | null = null;
      beginTaskMutation();
      setScheduledBlocksSnapshot((blocks) => {
        previousBlocks = blocks;
        removedEventId = blocks.find((block) => block.id === blockId)?.calendarEventId ?? null;
        return blocks.filter((block) => block.id !== blockId);
      });
      if (removedEventId) {
        setLoadState((current) => ({
          ...current,
          snapshot: {
            ...current.snapshot,
            events: (() => {
              previousEvents = current.snapshot.events;
              return current.snapshot.events.filter(
                (event) => (event.eventId ?? event.id) !== removedEventId
              );
            })()
          }
        }));
      }

      const result = await window.hcb.calendar.unscheduleTaskBlock({
        id: blockId,
        deleteCalendarEvent: true
      });

      if (result.ok) {
        finishTaskMutation();
        return true;
      }

      setScheduledBlocksSnapshot(() => previousBlocks);
      if (previousEvents.length > 0) {
        setLoadState((current) => ({
          ...current,
          snapshot: {
            ...current.snapshot,
            events: previousEvents
          }
        }));
      }
      failTaskMutation(result.error.message, () => void unscheduleTaskBlock(blockId));
      return false;
    },
    [beginTaskMutation, failTaskMutation, finishTaskMutation, setLoadState, setScheduledBlocksSnapshot]
  );

  return {
    taskMutation,
    clearTaskMutationError: () => setTaskMutation((current) => ({ ...current, error: undefined })),
    retryLastTaskMutation: () => retryTaskMutation.current(),
    createTask,
    updateTask,
    completeTask,
    reopenTask,
    completeEvent,
    reopenEvent,
    moveTask,
    bulkRescheduleTasks,
    deleteTask,
    createTaskList,
    renameTaskList,
    deleteTaskList,
    scheduleTaskBlock,
    moveScheduledTaskBlock,
    unscheduleTaskBlock
  };
}

function optimisticEventCompletionPatch(
  events: CalendarEventSummary[],
  eventId: string,
  completedAt: string | null,
  scope: CalendarEventCompletionScope
): CalendarEventSummary[] {
  const target = events.find((event) => event.id === eventId || (event.eventId ?? event.id) === eventId);

  if (!target) {
    return events;
  }

  return events.map((event) => {
    if (!eventCompletionScopeMatches(event, target, scope)) {
      return event;
    }

    return {
      ...event,
      completedAt,
      updatedAt: completedAt ?? new Date().toISOString()
    };
  });
}

function eventCompletionScopeMatches(
  event: CalendarEventSummary,
  target: CalendarEventSummary,
  scope: CalendarEventCompletionScope
): boolean {
  if (scope === "seriesAll") {
    return (event.eventId ?? event.id) === (target.eventId ?? target.id);
  }

  if (scope === "seriesFuture") {
    return (
      (event.eventId ?? event.id) === (target.eventId ?? target.id) &&
      event.startsAt >= target.startsAt
    );
  }

  return event.id === target.id;
}
