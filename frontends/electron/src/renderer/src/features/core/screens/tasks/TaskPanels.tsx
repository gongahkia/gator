import type { SavedTaskView } from "@shared/ipc/contracts";
import type { Dispatch, SetStateAction } from "react";
import {
  CheckCircle2,
  Filter,
  ListPlus,
  Plus,
  RotateCcw,
  Save,
  Trash2,
  X
} from "lucide-react";
import { Badge, Button, IconButton, Input, Panel, StatusBanner, cx } from "../../../../components/primitives";
import { EmptyState, ErrorState, LoadingState } from "../../../../components/states";
import type { useCoreViewModelSource } from "../../coreViewModelSource";
import type {
  ScheduledTaskBlockViewModel,
  TaskFilterId,
  TaskViewModel
} from "../../coreViewModels";
import {
  SectionChrome,
  TaskGroupPanel,
  actionDescription,
  actionLabel
} from "../../coreScreenShared";
import {
  savedTaskViewFilterChips,
  taskPerspectiveTabs,
  type TaskPerspectiveId,
  type TaskPerspectiveViewModel
} from "./taskPerspectives";

type CoreViewModelSource = ReturnType<typeof useCoreViewModelSource>;

export function TaskHeader({
  onCreateTask,
  onDeleteSelectedTask,
  onToggleSelectedTask,
  selectedTask
}: {
  onCreateTask: () => void;
  onDeleteSelectedTask: () => void;
  onToggleSelectedTask: () => void;
  selectedTask: TaskViewModel | null;
}): JSX.Element {
  return (
    <div className="flex flex-wrap items-center justify-between gap-3">
      <div className="flex min-w-0 items-center gap-2 overflow-x-auto pb-1" role="toolbar" aria-label="Task actions">
        <Button
          data-action-id="task.create"
          onClick={onCreateTask}
          title={actionDescription("task.create")}
          variant="primary"
        >
          <Plus aria-hidden="true" size={15} />
          {actionLabel("task.create")}
        </Button>
        <Button
          data-action-id="task.completeSelected"
          disabled={!selectedTask}
          onClick={selectedTask ? onToggleSelectedTask : undefined}
          title={selectedTask ? actionDescription("task.completeSelected") : "No selected task"}
          variant="ghost"
        >
          {selectedTask?.status === "completed" ? (
            <RotateCcw aria-hidden="true" size={15} />
          ) : (
            <CheckCircle2 aria-hidden="true" size={15} />
          )}
          {selectedTask?.status === "completed" ? "Reopen" : "Complete"}
        </Button>
        <Button
          data-action-id="task.deleteSelected"
          disabled={!selectedTask}
          onClick={selectedTask ? onDeleteSelectedTask : undefined}
          title={selectedTask ? actionDescription("task.deleteSelected") : "No selected task"}
          variant="danger"
        >
          <Trash2 aria-hidden="true" size={15} />
          Delete
        </Button>
      </div>
    </div>
  );
}

export function TaskPerspectiveTabs({
  activePerspective,
  activePerspectiveId,
  activeSavedTaskViewId,
  onSelectPerspective,
  onSelectSavedTaskView,
  savedTaskViews
}: {
  activePerspective: TaskPerspectiveViewModel;
  activePerspectiveId: TaskPerspectiveId;
  activeSavedTaskViewId: string | null;
  onSelectPerspective: (perspectiveId: TaskPerspectiveId) => void;
  onSelectSavedTaskView: (viewId: string | null) => void;
  savedTaskViews: SavedTaskView[];
}): JSX.Element {
  return (
    <div className="flex items-center gap-2 overflow-x-auto" role="tablist" aria-label="Task perspectives">
      {taskPerspectiveTabs.map((perspective) => {
        const selected = perspective.id === activePerspectiveId;

        return (
          <Button
            aria-selected={selected}
            key={perspective.id}
            onClick={() => {
              onSelectPerspective(perspective.id);

              if (perspective.id === "saved" && !activeSavedTaskViewId) {
                onSelectSavedTaskView(savedTaskViews[0]?.id ?? null);
              }
            }}
            role="tab"
            size="sm"
            variant={selected ? "secondary" : "ghost"}
          >
            {perspective.label}
            {perspective.id === "saved" ? (
              <Badge tone="neutral">{savedTaskViews.length}</Badge>
            ) : null}
          </Button>
        );
      })}
      <Badge tone={activePerspective.state === "error" ? "warning" : "neutral"}>
        {activePerspective.description}
      </Badge>
    </div>
  );
}

export function TaskFilterToolbar({
  activeFilterId,
  allVisibleTasksSelected,
  onSelectFilter,
  onToggleVisibleTaskSelection,
  source,
  visibleTaskCount
}: {
  activeFilterId: TaskFilterId;
  allVisibleTasksSelected: boolean;
  onSelectFilter: (filterId: TaskFilterId) => void;
  onToggleVisibleTaskSelection: () => void;
  source: CoreViewModelSource;
  visibleTaskCount: number;
}): JSX.Element {
  return (
    <div className="flex items-center gap-2 overflow-x-auto" role="toolbar" aria-label="Task filters">
      <Filter aria-hidden="true" className="shrink-0 text-text-muted" size={15} />
      {source.taskFilterViewModels.map((filter) => (
        <Button
          aria-pressed={filter.id === activeFilterId}
          key={filter.id}
          onClick={() => onSelectFilter(filter.id)}
          size="sm"
          variant={filter.id === activeFilterId ? "secondary" : "ghost"}
        >
          {filter.label}
          <Badge tone={filter.state === "error" ? "warning" : "neutral"}>{filter.countLabel}</Badge>
        </Button>
      ))}
      <Button
        disabled={visibleTaskCount === 0}
        onClick={onToggleVisibleTaskSelection}
        size="sm"
        variant={allVisibleTasksSelected ? "secondary" : "ghost"}
      >
        {allVisibleTasksSelected ? (
          <X aria-hidden="true" size={14} />
        ) : (
          <CheckCircle2 aria-hidden="true" size={14} />
        )}
        {allVisibleTasksSelected ? "Clear visible" : "Select visible"}
      </Button>
    </div>
  );
}

export function TaskMutationErrorBanner({ source }: { source: CoreViewModelSource }): JSX.Element | null {
  if (!source.taskMutationError) {
    return null;
  }

  return (
    <StatusBanner
      action={
        <div className="flex items-center gap-2">
          <Button onClick={source.retryLastTaskMutation} size="sm" variant="secondary">
            <RotateCcw aria-hidden="true" size={14} />
            Retry
          </Button>
          <IconButton
            icon={X}
            label="Dismiss task write error"
            onClick={source.clearTaskMutationError}
            variant="ghost"
          />
        </div>
      }
      description={source.taskMutationError}
      icon={RotateCcw}
      title="Task write not saved"
      tone="warning"
    />
  );
}

export function BulkTaskSelectionBanner({
  bulkCompletionLabel,
  bulkMoveTargetListId,
  bulkSelectedTaskIdsInWindow,
  bulkSelectedTasks,
  onClearSelection,
  onCompleteSelectedTasks,
  onDeleteSelectedTasks,
  onMoveSelectedTasks,
  onSelectMoveList,
  source
}: {
  bulkCompletionLabel: string;
  bulkMoveTargetListId: string;
  bulkSelectedTaskIdsInWindow: string[];
  bulkSelectedTasks: TaskViewModel[];
  onClearSelection: () => void;
  onCompleteSelectedTasks: () => void;
  onDeleteSelectedTasks: () => void;
  onMoveSelectedTasks: () => void;
  onSelectMoveList: (listId: string) => void;
  source: CoreViewModelSource;
}): JSX.Element | null {
  if (bulkSelectedTaskIdsInWindow.length === 0) {
    return null;
  }

  return (
    <StatusBanner
      action={
        <div className="flex items-center gap-2">
          <select
            aria-label="Bulk move list"
            className="h-7 rounded-hcbMd border border-border bg-surface-0 px-2 text-[var(--text-sm)] text-text-primary focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
            onChange={(event) => onSelectMoveList(event.target.value)}
            value={bulkMoveTargetListId}
          >
            {source.taskLists.map((taskList) => (
              <option key={taskList.id} value={taskList.id}>
                {taskList.title}
              </option>
            ))}
          </select>
          <Button
            disabled={!bulkMoveTargetListId || source.taskMutationPending}
            onClick={onMoveSelectedTasks}
            size="sm"
            variant="secondary"
          >
            <ListPlus aria-hidden="true" size={14} />
            Move selected
          </Button>
          <Button
            disabled={source.taskMutationPending}
            onClick={onCompleteSelectedTasks}
            size="sm"
            variant="secondary"
          >
            <CheckCircle2 aria-hidden="true" size={14} />
            {bulkCompletionLabel}
          </Button>
          <Button
            disabled={source.taskMutationPending}
            onClick={onDeleteSelectedTasks}
            size="sm"
            variant="danger"
          >
            <Trash2 aria-hidden="true" size={14} />
            Delete selected
          </Button>
          <IconButton
            icon={X}
            label="Clear task selection"
            onClick={onClearSelection}
            variant="ghost"
          />
        </div>
      }
      description={`${bulkSelectedTasks.map((task) => task.title).slice(0, 3).join(", ")}`}
      icon={CheckCircle2}
      title={`${bulkSelectedTaskIdsInWindow.length} ${
        bulkSelectedTaskIdsInWindow.length === 1 ? "task" : "tasks"
      } selected`}
      tone="info"
    />
  );
}

export function TaskListsSidebarPanel({
  listTitleDrafts,
  newListTitle,
  onCreateTaskList,
  onDeleteTaskList,
  onRenameTaskList,
  setListTitleDrafts,
  setNewListTitle,
  source
}: {
  listTitleDrafts: Record<string, string>;
  newListTitle: string;
  onCreateTaskList: () => void;
  onDeleteTaskList: (taskListId: string) => void;
  onRenameTaskList: (taskListId: string, currentTitle: string) => void;
  setListTitleDrafts: Dispatch<SetStateAction<Record<string, string>>>;
  setNewListTitle: Dispatch<SetStateAction<string>>;
  source: CoreViewModelSource;
}): JSX.Element {
  return (
    <Panel
      title="Task lists"
      description={source.taskLists.length === 0 ? "Task lists unavailable" : "Lists"}
    >
      <div className="grid gap-2 p-3">
        <div className="flex items-center gap-2">
          <Input
            aria-label="New task list title"
            onChange={(event) => setNewListTitle(event.target.value)}
            placeholder="New list"
            value={newListTitle}
          />
          <IconButton
            disabled={!newListTitle.trim() || source.taskMutationPending}
            icon={Plus}
            label="Create task list"
            onClick={onCreateTaskList}
            variant="primary"
          />
        </div>
        {source.taskLists.map((taskList) => {
          const draftTitle = listTitleDrafts[taskList.id] ?? taskList.title;

          return (
            <div className="grid grid-cols-[minmax(0,1fr)_32px_32px] gap-2" key={taskList.id}>
              <Input
                aria-label={`Rename ${taskList.title}`}
                onChange={(event) =>
                  setListTitleDrafts((current) => ({
                    ...current,
                    [taskList.id]: event.target.value
                  }))
                }
                value={draftTitle}
              />
              <IconButton
                disabled={
                  !draftTitle.trim() ||
                  draftTitle.trim() === taskList.title ||
                  source.taskMutationPending
                }
                icon={Save}
                label={`Save ${taskList.title}`}
                onClick={() => onRenameTaskList(taskList.id, taskList.title)}
                variant="ghost"
              />
              <IconButton
                disabled={source.taskMutationPending}
                icon={Trash2}
                label={`Delete ${taskList.title}`}
                onClick={() => onDeleteTaskList(taskList.id)}
                variant="danger"
              />
            </div>
          );
        })}
      </div>
    </Panel>
  );
}

export function SavedTaskPerspectivesPanel({
  activeSavedTaskView,
  onDeleteSavedTaskView,
  onSelectSavedTaskView,
  source
}: {
  activeSavedTaskView: SavedTaskView | null;
  onDeleteSavedTaskView: (viewId: string) => void;
  onSelectSavedTaskView: (viewId: string) => void;
  source: CoreViewModelSource;
}): JSX.Element {
  return (
    <Panel
      title="Saved perspectives"
      description={`${source.settings.savedTaskViews.length} local views`}
    >
      <div className="grid gap-2 p-3" role="list" aria-label="Saved task perspectives">
        {source.settings.savedTaskViews.length > 0 ? (
          source.settings.savedTaskViews.map((view) => {
            const selected = view.id === activeSavedTaskView?.id;

            return (
              <div
                className="grid grid-cols-[minmax(0,1fr)_32px] gap-2"
                key={view.id}
                role="listitem"
              >
                <button
                  aria-current={selected ? "true" : undefined}
                  aria-pressed={selected}
                  className={cx(
                    "min-w-0 rounded-hcbMd border px-3 py-2 text-left transition-colors duration-fast ease-hcb focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent",
                    selected
                      ? "border-accent bg-surface-0"
                      : "border-border bg-bg-tertiary hover:bg-surface-0"
                  )}
                  onClick={() => onSelectSavedTaskView(view.id)}
                  type="button"
                >
                  <span className="block truncate text-[var(--text-sm)] font-medium text-text-primary">
                    {view.name}
                  </span>
                  <span className="mt-1 flex flex-wrap gap-1">
                    {savedTaskViewFilterChips(view, source.taskLists).map((chip) => (
                      <Badge key={chip} tone="accent">
                        {chip}
                      </Badge>
                    ))}
                  </span>
                </button>
                <IconButton
                  disabled={source.settingsMutationPending}
                  icon={Trash2}
                  label={`Delete saved task perspective ${view.name}`}
                  onClick={() => onDeleteSavedTaskView(view.id)}
                  variant="danger"
                />
              </div>
            );
          })
        ) : (
          <EmptyState
            description="Saved task perspectives will appear here once settings contain task views."
            title="No saved perspectives"
          />
        )}
      </div>
    </Panel>
  );
}

export function TaskPerspectiveContent({
  activeFilterId,
  activePerspective,
  bulkSelectedTaskIdsInWindow,
  onBulkSelectTask,
  onDeleteTask,
  onSelectTask,
  onToggleTask,
  scheduledBlocksByTask,
  selectedTaskId,
  shouldRenderPerspectiveGroups
}: {
  activeFilterId: TaskFilterId;
  activePerspective: TaskPerspectiveViewModel;
  bulkSelectedTaskIdsInWindow: string[];
  onBulkSelectTask: (taskId: string, selected: boolean) => void;
  onDeleteTask: (taskId: string) => void;
  onSelectTask: (taskId: string) => void;
  onToggleTask: (taskId: string) => void;
  scheduledBlocksByTask: Map<string, ScheduledTaskBlockViewModel>;
  selectedTaskId: string | null;
  shouldRenderPerspectiveGroups: boolean;
}): JSX.Element | null {
  if (!shouldRenderPerspectiveGroups) {
    return null;
  }

  if (activePerspective.state === "empty") {
    return (
      <Panel title="Task list" description="Empty filtered state">
        <EmptyState
          description={
            activeFilterId === "empty"
              ? "No tasks match this filter."
              : "No tasks match this perspective."
          }
          title={activeFilterId === "empty" ? "No tasks in this filter" : "No tasks in this perspective"}
        />
      </Panel>
    );
  }

  if (activePerspective.state === "error") {
    return (
      <Panel title="Task list" description="Recoverable renderer error state">
        <ErrorState />
      </Panel>
    );
  }

  return (
    <>
      {activePerspective.groups.map((group) => (
        <TaskGroupPanel
          bulkSelectedTaskIds={bulkSelectedTaskIdsInWindow}
          group={group}
          onBulkSelectTask={onBulkSelectTask}
          key={group.id}
          onDeleteTask={onDeleteTask}
          onSelectTask={onSelectTask}
          onToggleTask={onToggleTask}
          scheduledBlocksByTaskId={scheduledBlocksByTask}
          selectedTaskId={selectedTaskId}
        />
      ))}
    </>
  );
}

export function TasksSectionChrome({
  children,
  sidebar
}: {
  children: React.ReactNode;
  sidebar: React.ReactNode;
}): JSX.Element {
  return (
    <SectionChrome title="Tasks" sidebar={sidebar}>
      <div className="grid gap-3">{children}</div>
    </SectionChrome>
  );
}

export function TaskRefreshPanel(): JSX.Element {
  return (
    <Panel title="Refresh state" description="Current rows remain visible">
      <LoadingState description="Refreshing planner data." title="Refreshing" />
    </Panel>
  );
}
