import { AlertTriangle, CalendarDays, Check, FileText, GitMerge, ListChecks, RefreshCw, Trash2, X } from "lucide-react";
import { Badge, Button, IconButton, cx } from "../../components/primitives";
import type { CoreViewModelSource } from "./coreViewModelSource";
import type { CalendarEventViewModel, NoteViewModel, TaskViewModel } from "./coreViewModels";

type DuplicateKind = "task" | "event" | "note";

interface DuplicateItem {
  detail: string;
  id: string;
  kind: DuplicateKind;
  title: string;
}

export interface DuplicateGroup {
  id: string;
  items: DuplicateItem[];
  keyLabel: string;
  kind: DuplicateKind;
  reason: string;
  title: string;
}

interface DuplicateReviewPanelProps {
  onOpenTask: (taskId: string) => void;
  source: CoreViewModelSource;
}

const iconByKind = {
  task: ListChecks,
  event: CalendarDays,
  note: FileText
};

export function DuplicateReviewPanel({ onOpenTask, source }: DuplicateReviewPanelProps): JSX.Element | null {
  const groups = duplicateGroups(source);

  if (groups.length === 0) {
    return null;
  }

  async function dismissGroup(groupId: string): Promise<void> {
    await source.updateSettings({
      dismissedDuplicateGroupIds: [...new Set([...source.settings.dismissedDuplicateGroupIds, groupId])]
    });
  }

  async function deleteItem(item: DuplicateItem): Promise<void> {
    if (!window.confirm(`Delete "${item.title}"?`)) {
      return;
    }

    if (item.kind === "task") {
      await source.deleteTask(item.id);
      return;
    }

    const result = item.kind === "event"
      ? await window.hcb?.calendar.delete({ id: item.id })
      : await window.hcb?.notes.delete({ id: item.id });

    if (!result?.ok) {
      window.alert(result?.error.message ?? "Delete failed.");
      return;
    }

    source.refreshUndoStatus();
    source.refresh();
  }

  function openItem(item: DuplicateItem): void {
    if (item.kind === "task") {
      onOpenTask(item.id);
      return;
    }

    window.dispatchEvent(new CustomEvent("hcb:open-entity", {
      detail: { id: item.id, kind: item.kind }
    }));
  }

  async function mergeGroup(group: DuplicateGroup): Promise<void> {
    if (group.items.length < 2 || !window.confirm(`Merge ${group.items.length} duplicate ${group.kind}s into the first item?`)) {
      return;
    }

    const [winnerItem, ...loserItems] = group.items;

    if (!winnerItem) {
      return;
    }

    const merged = await window.hcb?.duplicates.cleanup({
      kind: group.kind,
      winnerId: winnerItem.id,
      loserIds: loserItems.map((item) => item.id)
    });

    if (!merged?.ok) {
      window.alert(merged?.error.message ?? "Merge failed.");
      return;
    }

    await dismissGroup(group.id);
    source.refreshUndoStatus();
    source.refresh();
  }

  return (
    <section className="overflow-hidden rounded-hcbLg border border-border bg-bg-secondary">
      <div className="flex flex-wrap items-center gap-2 border-b border-border px-3 py-2">
        <AlertTriangle aria-hidden="true" className="text-warning" size={16} />
        <h2 className="text-[var(--text-md)] font-semibold text-text-primary">Duplicate review</h2>
        <Badge tone="warning">{groups.length}</Badge>
        <div className="flex-1" />
        <Button onClick={() => source.refresh()} size="sm" variant="ghost">
          <RefreshCw aria-hidden="true" size={14} />
          Refresh
        </Button>
      </div>
      <div className="grid max-h-80 overflow-auto">
        {groups.map((group) => {
          const Icon = iconByKind[group.kind];

          return (
            <div className="grid gap-2 border-b border-border p-3 last:border-b-0" key={group.id}>
              <div className="flex min-w-0 items-start gap-2">
                <Icon aria-hidden="true" className="mt-0.5 shrink-0 text-text-muted" size={16} />
                <div className="min-w-0 flex-1">
                  <div className="truncate text-[var(--text-sm)] font-semibold text-text-primary">{group.title}</div>
                  <div className="truncate text-[var(--text-xs)] text-text-muted">{group.reason} · {group.keyLabel}</div>
                </div>
                <IconButton icon={GitMerge} label="Merge duplicate group" onClick={() => void mergeGroup(group)} variant="ghost" />
                <IconButton icon={X} label="Dismiss duplicate group" onClick={() => void dismissGroup(group.id)} variant="ghost" />
              </div>
              <div className="grid gap-1.5">
                {group.items.map((item) => (
                  <div
                    className={cx(
                      "grid grid-cols-[minmax(0,1fr)_auto] items-center gap-2 rounded-hcbMd border border-border bg-bg-primary px-2 py-1.5"
                    )}
                    key={item.id}
                  >
                    <button className="min-w-0 text-left" onClick={() => openItem(item)} type="button">
                      <div className="truncate text-[var(--text-sm)] font-medium text-text-primary">{item.title}</div>
                      <div className="truncate text-[var(--text-xs)] text-text-muted">{item.detail}</div>
                    </button>
                    <div className="flex items-center gap-1">
                      <IconButton icon={Check} label="Open duplicate item" onClick={() => openItem(item)} variant="ghost" />
                      <IconButton icon={Trash2} label="Delete duplicate item" onClick={() => void deleteItem(item)} variant="ghost" />
                    </div>
                  </div>
                ))}
              </div>
            </div>
          );
        })}
      </div>
    </section>
  );
}

export function duplicateGroups(source: CoreViewModelSource): DuplicateGroup[] {
  const dismissed = new Set(source.settings.dismissedDuplicateGroupIds);
  return [
    ...groupTaskDuplicates(source.largeTaskWindow),
    ...groupEventDuplicates(source.calendarAgendaEvents.filter((event) => event.sourceKind !== "task")),
    ...groupNoteDuplicates(source.initialNotes)
  ].filter((group) => !dismissed.has(group.id));
}

function groupTaskDuplicates(tasks: readonly TaskViewModel[]): DuplicateGroup[] {
  return duplicateGroupsFromItems(
    tasks.filter((task) => task.status === "open"),
    (task) => ["task", normalizeTitle(task.title), task.listId, task.dueDate ?? "no-due"].join("|"),
    (task) => ({
      detail: `${task.list} · ${task.dueLabel}${task.snoozeUntil ? " · snoozed" : ""}`,
      id: task.id,
      kind: "task",
      title: task.title
    }),
    "task",
    "same title, list, due date, and active status"
  );
}

function groupNoteDuplicates(notes: readonly NoteViewModel[]): DuplicateGroup[] {
  return duplicateGroupsFromItems(
    notes,
    (note) => ["note", normalizeTitle(note.title), note.listId].join("|"),
    (note) => ({
      detail: `${note.listTitle} · ${note.updatedLabel}`,
      id: note.id,
      kind: "note",
      title: note.title
    }),
    "note",
    "same title and note list"
  );
}

function groupEventDuplicates(events: readonly CalendarEventViewModel[]): DuplicateGroup[] {
  return duplicateGroupsFromItems(
    events.filter((event) => event.status !== "cancelled"),
    (event) => [
      "event",
      normalizeTitle(event.title),
      event.calendarId,
      event.startsAt,
      event.endsAt,
      event.allDay ? "all-day" : "timed"
    ].join("|"),
    (event) => ({
      detail: `${event.calendar} · ${event.rangeLabel}`,
      id: event.id,
      kind: "event",
      title: event.title
    }),
    "event",
    "same title, calendar, start, end, and all-day state"
  );
}

function duplicateGroupsFromItems<T>(
  items: readonly T[],
  keyFor: (item: T) => string,
  itemFor: (item: T) => DuplicateItem,
  kind: DuplicateKind,
  reason: string
): DuplicateGroup[] {
  const grouped = new Map<string, T[]>();

  for (const item of items) {
    const key = keyFor(item);
    grouped.set(key, [...(grouped.get(key) ?? []), item]);
  }

  return Array.from(grouped.entries())
    .filter(([, groupItems]) => groupItems.length > 1)
    .map(([key, groupItems]) => {
      const duplicateItems = groupItems.map(itemFor);
      const title = duplicateItems[0]?.title || "Untitled";
      return {
        id: `dup-${hashString(key)}`,
        items: duplicateItems,
        keyLabel: title,
        kind,
        reason,
        title
      };
    });
}

function normalizeTitle(value: string): string {
  return value.normalize("NFKD").trim().toLowerCase().replace(/\s+/g, " ");
}

function hashString(value: string): string {
  let hash = 2166136261;

  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 16777619);
  }

  return (hash >>> 0).toString(36);
}
