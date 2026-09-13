import { useMemo } from "react";
import { Badge, ListRow, Panel } from "../../../components/primitives";
import { EmptyState } from "../../../components/states";
import { VirtualizedList } from "../../../components/VirtualizedList";
import { useCoreViewModelSource } from "../coreViewModelSource";
import {
  CacheStatePanel,
  TaskCompletionButton,
  priorityLabel,
  priorityTone
} from "../coreScreenShared";

export function TaskNotesView(): JSX.Element {
  const source = useCoreViewModelSource();
  const undatedTasks = useMemo(
    () =>
      source.largeTaskWindow.filter(
        (task) =>
          task.status === "open" &&
          task.parentId === null &&
          !task.dueDate &&
          !task.plannedStart
      ),
    [source.largeTaskWindow]
  );

  if (
    (source.dataState === "loading" ||
      source.dataState === "offline" ||
      source.dataState === "error") &&
    !source.hasCachedData
  ) {
    return <CacheStatePanel title="Notes" />;
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <Panel
        title="Notes"
        description="Undated Google Tasks"
        action={<Badge tone="neutral">{undatedTasks.length}</Badge>}
      >
        {undatedTasks.length > 0 ? (
          <VirtualizedList
            ariaLabel="Undated task notes"
            estimateRowHeight={68}
            getKey={(task) => task.id}
            items={undatedTasks}
            performanceLabel="notes.undated-tasks"
            renderRow={(task) => (
              <ListRow
                description={task.detail}
                leading={
                  <TaskCompletionButton
                    completed={false}
                    onToggle={(taskId) => void source.completeTask(taskId)}
                    task={task}
                  />
                }
                meta={task.list}
                title={task.title}
                trailing={<Badge tone={priorityTone(task.priority)}>{priorityLabel(task.priority)}</Badge>}
              />
            )}
            viewportHeight={Math.min(560, Math.max(180, undatedTasks.length * 68))}
          />
        ) : (
          <EmptyState
            description="Google Tasks with no due date or planned block will appear here."
            title="No undated tasks"
          />
        )}
      </Panel>
    </div>
  );
}
