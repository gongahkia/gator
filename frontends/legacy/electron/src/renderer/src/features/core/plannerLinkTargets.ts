import type { CalendarEventViewModel, NoteViewModel, TaskViewModel } from "./coreViewModels";
import type { useCoreViewModelSource } from "./coreViewModelSource";
import type { MarkdownPlannerLinkTarget } from "./MarkdownPreview";

type CoreSource = ReturnType<typeof useCoreViewModelSource>;

export function plannerLinkTargets(source: CoreSource): MarkdownPlannerLinkTarget[] {
  return [
    ...source.initialNotes.map(noteLinkTarget),
    ...source.largeTaskWindow.map(taskLinkTarget),
    ...source.calendarAgendaEvents.map(eventLinkTarget),
    ...source.taskLists.map((list) => ({
      body: list.title,
      id: list.id,
      kind: "list" as const,
      title: list.title
    })),
    ...source.calendarSources.map((calendar) => ({
      body: calendar.title,
      id: calendar.id,
      kind: "calendar" as const,
      title: calendar.title
    }))
  ];
}

function noteLinkTarget(note: NoteViewModel): MarkdownPlannerLinkTarget {
  return {
    body: note.body,
    id: note.id,
    kind: "note",
    title: note.title
  };
}

function taskLinkTarget(task: TaskViewModel): MarkdownPlannerLinkTarget {
  return {
    body: task.detail,
    id: task.id,
    kind: "task",
    title: task.title
  };
}

function eventLinkTarget(event: CalendarEventViewModel): MarkdownPlannerLinkTarget {
  return {
    body: event.notes,
    id: event.id,
    kind: "event",
    title: event.title
  };
}
