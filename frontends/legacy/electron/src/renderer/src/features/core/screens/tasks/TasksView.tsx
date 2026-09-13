import { useEffect, useMemo, useRef, useState } from "react";
import { useCoreViewModelSource } from "../../coreViewModelSource";
import { DuplicateReviewPanel } from "../../DuplicateReviewPanel";
import type { ConvertSourceCleanup } from "../../conversionEvents";
import {
  readLocalStorageNumberRecord,
  readLocalStorageStringArray,
  writeLocalStorageJSON
} from "../../localStorageHelpers";
import {
  CacheStatePanel,
  scheduledBlockByTaskId
} from "../../coreScreenShared";
import type { TaskDraft } from "../../inspectors/TaskInspectorBody";
import {
  TaskMutationErrorBanner,
  TaskRefreshPanel
} from "./TaskPanels";
import {
  GoogleTasksBoard,
  type TaskBoardSelection,
  type TaskListSort
} from "./GoogleTasksBoard";
import { useTaskInspector } from "./useTaskInspector";

export interface TaskSurfaceCommand {
  id: "task.create";
  nonce: number;
  paneId?: string;
}

const starredTasksStorageKey = "hcb.starredTaskIds";
const starredTasksAtStorageKey = "hcb.starredTaskAt";

export function TasksView({ command }: { command?: TaskSurfaceCommand | null }): JSX.Element {
  const source = useCoreViewModelSource();
  const {
    addSubtaskForTask,
    deleteTask,
    duplicateTask,
    openNewTask,
    selectedTaskId,
    selectTask,
    toggleTask
  } = useTaskInspector(source);
  const [selectedBoardView, setSelectedBoardView] = useState<TaskBoardSelection>({
    mode: "lists",
    listIds: null
  });
  const [starredTaskIds, setStarredTaskIds] = useState<Set<string>>(
    () => new Set(readLocalStorageStringArray(starredTasksStorageKey))
  );
  const [starredTaskAt, setStarredTaskAt] = useState<Record<string, number>>(
    () => readLocalStorageNumberRecord(starredTasksAtStorageKey)
  );
  const [listSorts, setListSorts] = useState<Record<string, TaskListSort>>({});
  const handledCommandNonce = useRef<number | null>(null);
  const scheduledBlocksByTask = useMemo(
    () => scheduledBlockByTaskId(source.scheduledTaskBlocks),
    [source.scheduledTaskBlocks]
  );

  useEffect(() => {
    if (!command || handledCommandNonce.current === command.nonce) {
      return;
    }

    handledCommandNonce.current = command.nonce;
    setSelectedBoardView({ mode: "lists", listIds: null });

    openNewTask();
  }, [command, openNewTask]);

  useEffect(() => {
    function handleTaskCommand(event: Event): void {
      const detail = (event as CustomEvent<{
        action: string;
        cleanup?: ConvertSourceCleanup;
        taskId?: string;
        draft?: Partial<Omit<TaskDraft, "mode">> | TaskDraft;
      }>).detail;

      if (detail?.action === "open-task" && detail.taskId) {
        setSelectedBoardView({ mode: "lists", listIds: null });
        selectTask(detail.taskId);
      }

      if (detail?.action === "new-task") {
        setSelectedBoardView({ mode: "lists", listIds: null });
        openNewTask(detail.draft ?? {});
      }

      if (detail?.action === "convert-to-task") {
        setSelectedBoardView({ mode: "lists", listIds: null });
        openNewTask(detail.draft ?? {}, detail.cleanup);
      }
    }

    window.addEventListener("hcb:task-command", handleTaskCommand);
    return () => window.removeEventListener("hcb:task-command", handleTaskCommand);
  }, [openNewTask, selectTask]);

  useEffect(() => {
    writeLocalStorageJSON(starredTasksStorageKey, [...starredTaskIds]);
  }, [starredTaskIds]);

  useEffect(() => {
    writeLocalStorageJSON(starredTasksAtStorageKey, starredTaskAt);
  }, [starredTaskAt]);

  if (
    (source.dataState === "loading" ||
      source.dataState === "offline" ||
      source.dataState === "error") &&
    !source.hasCachedData
  ) {
    return <CacheStatePanel title="Tasks" />;
  }

  function deleteTaskList(taskListId: string): void {
    void source.deleteTaskList(taskListId);
  }

  function promptCreateTaskList(): void {
    const title = window.prompt("Create new list")?.trim();

    if (!title || source.taskMutationPending) {
      return;
    }

    void source.createTaskList({ title });
  }

  function promptRenameTaskList(taskList: { id: string; title: string }): void {
    const title = window.prompt("Rename list", taskList.title)?.trim();

    if (!title || title === taskList.title || source.taskMutationPending) {
      return;
    }

    void source.renameTaskList({ id: taskList.id, title });
  }

  function confirmDeleteTaskList(taskListId: string): void {
    const taskList = source.taskLists.find((list) => list.id === taskListId);
    const title = taskList?.title ?? "this list";

    if (!window.confirm(`Delete ${title}? This also deletes tasks in the list.`)) {
      return;
    }

    deleteTaskList(taskListId);
  }

  function toggleTaskStar(taskId: string): void {
    setStarredTaskIds((current) => {
      const next = new Set(current);

      if (next.has(taskId)) {
        next.delete(taskId);
        setStarredTaskAt((timestamps) => {
          const result = { ...timestamps };
          delete result[taskId];
          return result;
        });
      } else {
        next.add(taskId);
        setStarredTaskAt((timestamps) => ({
          ...timestamps,
          [taskId]: Date.now()
        }));
      }

      return next;
    });
  }

  function setListSort(listId: string, sort: TaskListSort): void {
    setListSorts((current) => ({
      ...current,
      [listId]: sort
    }));
  }

  function moveTaskToList(taskId: string, listId: string): void {
    void source.moveTask({ id: taskId, listId, parentId: null });
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <TaskMutationErrorBanner source={source} />
      <DuplicateReviewPanel onOpenTask={selectTask} source={source} />

      <div className="grid min-h-0 flex-1 gap-3">
        {source.dataState === "stale" ? <TaskRefreshPanel /> : null}
        <GoogleTasksBoard
          listSorts={listSorts}
          onAddSubtask={addSubtaskForTask}
          onCreateList={promptCreateTaskList}
          onCreateTask={openNewTask}
          onDeleteList={confirmDeleteTaskList}
          onDeleteTask={(taskId) => void deleteTask(taskId)}
          onDuplicateTask={duplicateTask}
          onMoveTask={moveTaskToList}
          onMoveTaskRequest={(request) => { void source.moveTask(request); }}
          onOpenTask={selectTask}
          onRenameList={promptRenameTaskList}
          onSetListSort={setListSort}
          onToggleStar={toggleTaskStar}
          onToggleTask={(taskId) => void toggleTask(taskId)}
          scheduledBlocksByTask={scheduledBlocksByTask}
          selectedTaskId={selectedTaskId}
          selectedView={selectedBoardView}
          setSelectedView={setSelectedBoardView}
          source={source}
          starred={{ ids: starredTaskIds, starredAt: starredTaskAt }}
        />
      </div>
    </div>
  );
}
