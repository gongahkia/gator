import type { SectionId } from "../../data/mockPlanner";
import { CalendarView } from "./screens/CalendarView";
import { NotesView } from "./screens/NotesView";
import { SettingsView } from "./screens/SettingsView";
import { TasksView, type TaskSurfaceCommand } from "./screens/TasksView";

export { SettingsView };
export type { TaskSurfaceCommand };

export function SectionContent({
  activeSectionId,
  taskCommand,
  visibleCalendarIds
}: {
  activeSectionId: SectionId;
  taskCommand?: TaskSurfaceCommand | null;
  visibleCalendarIds: ReadonlySet<string>;
}): JSX.Element {
  if (activeSectionId === "tasks") {
    return <TasksView command={taskCommand} />;
  }

  if (activeSectionId === "calendar") {
    return <CalendarView visibleCalendarIds={visibleCalendarIds} />;
  }

  if (activeSectionId === "notes") {
    return <NotesView />;
  }

  if (activeSectionId === "settings") {
    return <SettingsView />;
  }

  return <CalendarView visibleCalendarIds={visibleCalendarIds} />;
}
