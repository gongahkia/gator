import {
  CalendarDays,
  ListTodo,
  Settings,
  StickyNote
} from "lucide-react";
import type { LucideIcon } from "lucide-react";

export type SectionId = "tasks" | "calendar" | "notes" | "settings";
export type Priority = "none" | "low" | "medium" | "high";

export interface PlannerSection {
  id: SectionId;
  label: string;
  title: string;
  subtitle: string;
  metric: string;
  icon: LucideIcon;
}

export interface MockTask {
  id: string;
  title: string;
  detail: string;
  list: string;
  dueLabel: string;
  priority: Priority;
  completed: boolean;
}

export interface MockNote {
  id: string;
  title: string;
  preview: string;
  updatedLabel: string;
}

export interface MockSearchResult {
  id: string;
  type: "task" | "event" | "note" | "setting";
  title: string;
  detail: string;
}

export interface MockSetting {
  id: string;
  title: string;
  detail: string;
  status: string;
}

export const plannerSections: PlannerSection[] = [
  {
    id: "tasks",
    label: "Tasks",
    title: "Tasks",
    subtitle: "Task lists, priorities, and queued mutations",
    metric: "4 open",
    icon: ListTodo
  },
  {
    id: "calendar",
    label: "Calendar",
    title: "Calendar",
    subtitle: "Agenda and calendar previews",
    metric: "5 events",
    icon: CalendarDays
  },
  {
    id: "notes",
    label: "Notes",
    title: "Notes",
    subtitle: "Notes and planning scratchpads",
    metric: "3 notes",
    icon: StickyNote
  },
  {
    id: "settings",
    label: "Settings",
    title: "Settings",
    subtitle: "App preferences and diagnostics",
    metric: "8 areas",
    icon: Settings
  }
];

export const primaryPlannerSections = plannerSections.filter((section) =>
  section.id === "calendar" || section.id === "tasks" || section.id === "notes"
);

export const mockTasks: MockTask[] = [
  {
    id: "task-inbox-rules",
    title: "Draft inbox triage rules",
    detail: "Define keyboard-first review states before sync writes exist.",
    list: "Inbox",
    dueLabel: "Today",
    priority: "high",
    completed: false
  },
  {
    id: "task-calendar-fixtures",
    title: "Review calendar fixture shape",
    detail: "Keep visible rows stable for future agenda virtualization.",
    list: "Planning",
    dueLabel: "Today",
    priority: "medium",
    completed: false
  },
  {
    id: "task-offline-copy",
    title: "Tighten offline banner copy",
    detail: "Make retry and planner state explicit without exposing service details.",
    list: "Polish",
    dueLabel: "Tomorrow",
    priority: "low",
    completed: false
  },
  {
    id: "task-settings-states",
    title: "Map settings empty and error states",
    detail: "Prepare rows for OAuth, hotkeys, diagnostics, MCP, and planner data.",
    list: "Settings",
    dueLabel: "Friday",
    priority: "none",
    completed: false
  },
  {
    id: "task-shell-visible",
    title: "Report shell-visible timing",
    detail: "Mock-only diagnostics call is already available through preload.",
    list: "Performance",
    dueLabel: "Done",
    priority: "low",
    completed: true
  }
];

export const mockNotes: MockNote[] = [
  {
    id: "note-cache-first",
    title: "Startup data flow",
    preview: "Renderer should paint a useful shell before Google, SQLite, or MCP work is wired.",
    updatedLabel: "Updated 8m ago"
  },
  {
    id: "note-command-surface",
    title: "Command palette surface",
    preview: "Commands stay in memory and execute the same future services as visible controls.",
    updatedLabel: "Updated 21m ago"
  },
  {
    id: "note-density",
    title: "Compact density checks",
    preview: "Use 13px body text, stable toolbar controls, and visible focus rings.",
    updatedLabel: "Updated yesterday"
  }
];

export const mockSettings: MockSetting[] = [
  {
    id: "google",
    title: "Google account",
    detail: "OAuth setup placeholder; no tokens are present in the renderer.",
    status: "Disconnected"
  },
  {
    id: "lists",
    title: "Lists and calendars",
    detail: "Selection UI shell for future task list and calendar sources.",
    status: "Not selected"
  },
  {
    id: "sync",
    title: "Sync mode",
    detail: "Synced data first, background refresh later.",
    status: "Mock only"
  },
  {
    id: "appearance",
    title: "Appearance",
    detail: "Semantic CSS variables support dark and light themes.",
    status: "System"
  },
  {
    id: "diagnostics",
    title: "Diagnostics",
    detail: "Copyable summaries will be sanitized before leaving app services.",
    status: "Ready"
  }
];

export const mockSearchResults: MockSearchResult[] = [
  {
    id: "search-task-inbox-rules",
    type: "task",
    title: "Draft inbox triage rules",
    detail: "Task in Inbox due today"
  },
  {
    id: "search-event-review",
    type: "event",
    title: "Renderer acceptance review",
    detail: "Calendar event at 15:30"
  },
  {
    id: "search-note-command",
    type: "note",
    title: "Command palette surface",
    detail: "Note updated 21m ago"
  },
  {
    id: "search-setting-hotkeys",
    type: "setting",
    title: "Hotkeys",
    detail: "Settings area with recoverable conflict"
  }
];

export function getPlannerSection(sectionId: SectionId): PlannerSection {
  return plannerSections.find((section) => section.id === sectionId) ??
    plannerSections.find((section) => section.id === "calendar") ??
    plannerSections[0];
}

export function priorityLabel(priority: Priority): string {
  if (priority === "none") {
    return "No priority";
  }

  return `${priority[0].toUpperCase()}${priority.slice(1)} priority`;
}
