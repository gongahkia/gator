import { describe, expect, it } from "vitest";
import { duplicateGroups } from "./DuplicateReviewPanel";
import type { CalendarEventViewModel, NoteViewModel, TaskViewModel } from "./coreViewModels";
import type { CoreViewModelSource } from "./coreViewModelSource";

function task(overrides: Partial<TaskViewModel>): TaskViewModel {
  return {
    id: "task-1",
    listId: "list-1",
    parentId: null,
    title: "Plan",
    detail: "",
    list: "Inbox",
    dueDate: null,
    dueLabel: "No due date",
    priority: "none",
    status: "open",
    subtasks: [],
    ...overrides
  };
}

function event(overrides: Partial<CalendarEventViewModel>): CalendarEventViewModel {
  return {
    id: "event-1",
    eventId: "event-1",
    sourceKind: "event",
    status: "confirmed",
    calendarId: "calendar-1",
    title: "Plan",
    calendar: "Work",
    completedAt: null,
    timeLabel: "09:00",
    rangeLabel: "09:00-10:00",
    startsAt: "2026-06-08T09:00:00.000Z",
    endsAt: "2026-06-08T10:00:00.000Z",
    timeZone: "UTC",
    allDay: false,
    location: "",
    notes: "",
    guestEmails: [],
    reminderMinutes: [],
    attendees: [],
    reminders: [],
    remindersUseDefault: false,
    transparency: null,
    visibility: null,
    conference: null,
    recurrenceRule: null,
    ...overrides
  };
}

function note(overrides: Partial<NoteViewModel>): NoteViewModel {
  return {
    id: "note-1",
    listId: "list-1",
    listTitle: "Inbox",
    title: "Plan",
    body: "",
    preview: "",
    updatedLabel: "now",
    ...overrides
  };
}

function source(overrides: {
  calendarAgendaEvents?: CalendarEventViewModel[];
  dismissedDuplicateGroupIds?: string[];
  initialNotes?: NoteViewModel[];
  largeTaskWindow?: TaskViewModel[];
} = {}): CoreViewModelSource {
  return {
    calendarAgendaEvents: overrides.calendarAgendaEvents ?? [],
    initialNotes: overrides.initialNotes ?? [],
    largeTaskWindow: overrides.largeTaskWindow ?? [],
    settings: {
      dismissedDuplicateGroupIds: overrides.dismissedDuplicateGroupIds ?? []
    } as CoreViewModelSource["settings"]
  } as CoreViewModelSource;
}

describe("duplicateGroups", () => {
  it("uses the same title normalization for task, event, and note groups", () => {
    const groups = duplicateGroups(source({
      largeTaskWindow: [
        task({ id: "task-1", title: "  Launch   Plan ", dueDate: "2026-06-08" }),
        task({ id: "task-2", title: "launch plan", dueDate: "2026-06-08" })
      ],
      calendarAgendaEvents: [
        event({ id: "event-1", eventId: "event-1", title: "  Launch   Plan " }),
        event({ id: "event-2", eventId: "event-2", title: "launch plan" })
      ],
      initialNotes: [
        note({ id: "note-1", title: "  Launch   Plan " }),
        note({ id: "note-2", title: "launch plan" })
      ]
    }));

    expect(groups.map((group) => group.kind).sort()).toEqual(["event", "note", "task"]);
  });

  it("keeps group ids stable and filters dismissed groups", () => {
    const input = source({
      largeTaskWindow: [
        task({ id: "task-1", title: "Plan", dueDate: null }),
        task({ id: "task-2", title: "plan", dueDate: null })
      ]
    });
    const [group] = duplicateGroups(input);

    expect(group?.id).toBe(duplicateGroups(input)[0]?.id);
    expect(duplicateGroups(source({
      largeTaskWindow: input.largeTaskWindow,
      dismissedDuplicateGroupIds: group ? [group.id] : []
    }))).toEqual([]);
  });

  it("includes active child tasks in duplicate task groups", () => {
    const groups = duplicateGroups(source({
      largeTaskWindow: [
        task({ id: "task-parent", title: "Plan", parentId: null }),
        task({ id: "task-child", title: "plan", parentId: "task-parent" })
      ]
    }));

    expect(groups).toHaveLength(1);
    expect(groups[0]?.items.map((item) => item.id).sort()).toEqual(["task-child", "task-parent"]);
  });

  it("excludes cancelled event candidates", () => {
    const groups = duplicateGroups(source({
      calendarAgendaEvents: [
        event({ id: "event-1", eventId: "event-1", title: "Plan", status: "cancelled" }),
        event({ id: "event-2", eventId: "event-2", title: "plan", status: "cancelled" })
      ]
    }));

    expect(groups).toEqual([]);
  });

  it("groups notes by normalized title and note list only", () => {
    const groups = duplicateGroups(source({
      initialNotes: [
        note({ id: "note-1", title: "Plan", listId: "list-1" }),
        note({ id: "note-2", title: "plan", listId: "list-1" }),
        note({ id: "note-3", title: "plan", listId: "list-2" })
      ]
    }));

    expect(groups).toHaveLength(1);
    expect(groups[0]?.items.map((item) => item.id).sort()).toEqual(["note-1", "note-2"]);
  });
});
