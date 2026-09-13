import type { KeyboardEvent, ReactNode } from "react";
import type { SettingsSnapshot } from "@shared/ipc/contracts";
import {
  AlertTriangle,
  Bell,
  Brush,
  CalendarClock,
  CalendarPlus,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  Circle,
  Clock3,
  Copy,
  Eye,
  EyeOff,
  FileText,
  Flag,
  Gift,
  Info,
  Keyboard,
  Languages,
  ListPlus,
  MapPin,
  Pencil,
  Filter,
  Minus,
  PanelLeft,
  PanelRight,
  Plus,
  Power,
  RotateCcw,
  Save,
  Server,
  ShieldCheck,
  SlidersHorizontal,
  Sparkles,
  StepBack,
  StepForward,
  Search,
  Settings2,
  Trash2,
  Users,
  X
} from "lucide-react";
import { getPlannerAction, type PlannerActionId } from "../../actions/plannerActions";
import { Badge, IconButton, ListRow, Panel, cx } from "../../components/primitives";
import { EmptyState, ErrorState, LoadingState, OfflineState } from "../../components/states";
import { VirtualizedList } from "../../components/VirtualizedList";
import { useCoreViewModelSource } from "./coreViewModelSource";
import type {
  CalendarEventViewModel,
  CorePriority,
  SearchSource,
  ScheduledTaskBlockViewModel,
  TaskGroupViewModel,
  TaskViewModel
} from "./coreViewModels";

export type CalendarSourceViewModel = ReturnType<typeof useCoreViewModelSource>["calendarSources"][number];
export type CompactTone = "neutral" | "accent" | "success" | "warning" | "danger" | "info";

export function priorityTone(priority: CorePriority): "neutral" | "accent" | "warning" | "danger" {
  if (priority === "high") {
    return "danger";
  }

  if (priority === "medium") {
    return "warning";
  }

  if (priority === "low") {
    return "accent";
  }

  return "neutral";
}

export function currentSystemPrefersDark(): boolean {
  return typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-color-scheme: dark)").matches;
}

export function fontFamilyOptions(fontFamilies: readonly string[], currentFontName: string | null): string[] {
  const unique = new Set<string>();

  for (const fontName of [...fontFamilies, currentFontName ?? ""]) {
    const trimmed = fontName.trim();

    if (trimmed) {
      unique.add(trimmed);
    }
  }

  return [...unique].sort((left, right) => left.localeCompare(right));
}

export function priorityLabel(priority: CorePriority): string {
  if (priority === "none") {
    return "No priority";
  }

  return `${priority[0].toUpperCase()}${priority.slice(1)} priority`;
}

export function taskDueCue(task: TaskViewModel): { label: string; tone: CompactTone } | null {
  if (task.status === "completed") {
    return { label: "Done", tone: "success" };
  }

  if (!task.dueDate) {
    return null;
  }

  const today = dateOnlyFromLocalDate(new Date());

  if (task.dueDate < today) {
    return { label: "Overdue", tone: "danger" };
  }

  if (task.dueDate === today) {
    return { label: "Due today", tone: "warning" };
  }

  return { label: task.dueLabel, tone: "neutral" };
}

export function taskDurationLabel(minutes: number | null | undefined): string {
  if (!minutes || minutes <= 0) {
    return "30 min";
  }

  if (minutes < 60) {
    return `${minutes} min`;
  }

  const hours = Math.floor(minutes / 60);
  const remaining = minutes % 60;

  return remaining === 0 ? `${hours} hr` : `${hours} hr ${remaining} min`;
}

export function taskScheduleLabel(
  task: TaskViewModel,
  scheduledBlock?: ScheduledTaskBlockViewModel
): string | null {
  if (scheduledBlock) {
    return scheduledBlock.status === "orphaned"
      ? `Orphaned ${scheduledBlock.rangeLabel}`
      : `Scheduled ${scheduledBlock.rangeLabel}`;
  }

  if (task.plannedStart && task.plannedEnd) {
    return `Planned ${timeLabel(task.plannedStart)}-${timeLabel(task.plannedEnd)}`;
  }

  if (task.durationMinutes) {
    return taskDurationLabel(task.durationMinutes);
  }

  return null;
}

export function taskScheduleTone(scheduledBlock?: ScheduledTaskBlockViewModel): CompactTone {
  if (scheduledBlock?.status === "orphaned") {
    return "warning";
  }

  if (scheduledBlock) {
    return "info";
  }

  return "neutral";
}

export function taskBridgeDescription(
  task: TaskViewModel,
  scheduledBlock?: ScheduledTaskBlockViewModel
): string {
  const parts = [task.detail];
  const scheduleLabel = taskScheduleLabel(task, scheduledBlock);

  if (scheduleLabel) {
    parts.push(scheduleLabel);
  }

  if (task.lockedSchedule) {
    parts.push("Locked");
  }

  return parts.filter(Boolean).join(" - ");
}

export function scheduledBlockByTaskId(
  blocks: ScheduledTaskBlockViewModel[]
): Map<string, ScheduledTaskBlockViewModel> {
  const byTaskId = new Map<string, ScheduledTaskBlockViewModel>();

  for (const block of blocks) {
    const existing = byTaskId.get(block.taskId);

    if (!existing || existing.status === "orphaned" || block.status === "scheduled") {
      byTaskId.set(block.taskId, block);
    }
  }

  return byTaskId;
}

export function sourceTone(source: SearchSource): "accent" | "success" | "info" {
  if (source === "task") {
    return "success";
  }

  if (source === "event") {
    return "accent";
  }

  return "info";
}

export function settingTone(status: string): "neutral" | "success" | "warning" | "info" {
  if (status === "Ready") {
    return "success";
  }

  if (status === "Conflict" || status === "Not requested" || status.includes("blocker")) {
    return "warning";
  }

  if (status === "Mock only" || status === "Enabled shell") {
    return "info";
  }

  return "neutral";
}

export const guestEmailPattern = /^[^@\s]+@[^@\s]+\.[^@\s]+$/;

export function scheduleRendererFrame(callback: () => void): void {
  if (typeof window.requestAnimationFrame === "function") {
    window.requestAnimationFrame(callback);
    return;
  }

  window.setTimeout(callback, 0);
}

export function actionLabel(actionId: PlannerActionId): string {
  return getPlannerAction(actionId).label;
}

export function actionDescription(actionId: PlannerActionId): string {
  return getPlannerAction(actionId).description;
}

export function handleActivationKeyDown(event: KeyboardEvent<HTMLElement>, callback: () => void): void {
  if (event.key !== "Enter" && event.key !== " ") {
    return;
  }

  event.preventDefault();
  callback();
}

export function normalizeGuestEmails(values: readonly string[] | undefined): string[] {
  const seen = new Set<string>();
  const normalized: string[] = [];

  for (const value of values ?? []) {
    const email = value.trim().toLowerCase();

    if (!guestEmailPattern.test(email) || seen.has(email)) {
      continue;
    }

    seen.add(email);
    normalized.push(email);
  }

  return normalized;
}

export function normalizeReminderMinutes(values: readonly number[] | undefined): number[] {
  const seen = new Set<number>();
  const normalized: number[] = [];

  for (const value of values ?? []) {
    if (!Number.isInteger(value) || value < 0 || value > 28 * 24 * 60 || seen.has(value)) {
      continue;
    }

    seen.add(value);
    normalized.push(value);
  }

  return normalized.sort((left, right) => left - right);
}

export function startOfUtcDayIso(value: string | Date): string {
  const date = typeof value === "string" ? new Date(value) : value;

  return new Date(Date.UTC(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate())).toISOString();
}

export function addUtcDaysIso(value: string | Date, days: number): string {
  const date = typeof value === "string" ? new Date(value) : new Date(value.getTime());

  date.setUTCDate(date.getUTCDate() + days);
  return date.toISOString();
}

export function dateInputValue(value: string): string {
  const parsed = new Date(value);

  if (!Number.isFinite(parsed.getTime())) {
    return "";
  }

  return parsed.toISOString().slice(0, 10);
}

export function dateInputToIso(value: string): string {
  return `${value}T00:00:00.000Z`;
}

export function dateRangeInputToInclusiveIsoRange(
  startDate: string,
  endDate: string
): { start: string; end: string } | null {
  if (!startDate || !endDate) {
    return null;
  }

  const start = dateInputToIso(startDate);
  const endDay = dateInputToIso(endDate);

  if (!Number.isFinite(Date.parse(start)) || !Number.isFinite(Date.parse(endDay))) {
    return null;
  }

  return {
    start,
    end: addUtcDaysIso(endDay, 1)
  };
}

export function dateTimeLocalInputValue(value: string): string {
  const parsed = new Date(value);

  if (!Number.isFinite(parsed.getTime())) {
    return "";
  }

  return parsed.toISOString().slice(0, 16);
}

export function dateTimeLocalInputToIso(value: string): string {
  const parsed = new Date(`${value}:00.000Z`);

  return Number.isFinite(parsed.getTime()) ? parsed.toISOString() : new Date().toISOString();
}

export function MetricTile({ label, value }: { label: string; value: string }): JSX.Element {
  return (
    <div className="min-w-0 rounded-hcbMd border border-border bg-bg-secondary px-3 py-2">
      <div className="truncate text-[var(--text-xs)] text-text-muted">{label}</div>
      <div className="mt-1 truncate text-[var(--text-lg)] font-semibold text-text-primary">{value}</div>
    </div>
  );
}

export function CacheStatePanel({ title }: { title: string }): JSX.Element | null {
  const source = useCoreViewModelSource();

  if (source.dataState === "loading") {
    return (
      <Panel title={title} description="Planner data">
        <LoadingState description="Reading planner data." />
      </Panel>
    );
  }

  if (source.dataState === "error" && !source.hasCachedData) {
    return (
      <Panel title={title} description="Planner data">
        <ErrorState description={source.errorMessage ?? "The planner data request failed."} />
      </Panel>
    );
  }

  if (source.dataState === "offline" && !source.hasCachedData) {
    return (
      <Panel title={title} description="Planner data">
        <OfflineState description="The preload bridge is unavailable in this renderer context." />
      </Panel>
    );
  }

  if (source.dataState === "empty" && !source.hasCachedData) {
    return (
      <Panel title={title} description="Planner data">
        <EmptyState
          description="No tasks, events, or notes are available yet."
          title="Nothing here yet"
        />
      </Panel>
    );
  }

  return null;
}

export function SectionChrome({
  children,
  sidebar,
  title
}: {
  children: ReactNode;
  sidebar?: ReactNode;
  title: string;
}): JSX.Element {
  if (!sidebar) {
    return <div className="min-h-0 min-w-0 flex-1">{children}</div>;
  }

  return (
    <div className="grid min-h-0 flex-1 grid-cols-1 gap-3 xl:grid-cols-[minmax(0,1fr)_360px]">
      <div className="min-w-0">{children}</div>
      <aside aria-label={`${title} support`} className="min-w-0">
        {sidebar}
      </aside>
    </div>
  );
}

export function TaskCompletionButton({
  completed,
  onToggle,
  task
}: {
  completed: boolean;
  onToggle: (taskId: string) => void;
  task: TaskViewModel;
}): JSX.Element {
  return (
    <button
      aria-label={completed ? `Reopen ${task.title}` : `Complete ${task.title}`}
      aria-pressed={completed}
      className={cx(
        "flex size-7 shrink-0 items-center justify-center rounded-hcbMd border transition-colors duration-fast ease-hcb focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent",
        completed
          ? "border-success bg-bg-secondary text-success"
          : "border-border bg-surface-0 text-text-muted hover:border-accent hover:text-accent"
      )}
      onClick={() => onToggle(task.id)}
      type="button"
    >
      {completed ? <CheckCircle2 aria-hidden="true" size={17} /> : <Circle aria-hidden="true" size={17} />}
    </button>
  );
}

export function TaskRow({
  bulkSelected,
  completed,
  onDelete,
  onBulkSelect,
  onSelect,
  onToggle,
  selected,
  scheduledBlock,
  showBulkSelection = false,
  task
}: {
  bulkSelected: boolean;
  completed: boolean;
  onDelete: (taskId: string) => void;
  onBulkSelect: (taskId: string, selected: boolean) => void;
  onSelect: (taskId: string) => void;
  onToggle: (taskId: string) => void;
  selected: boolean;
  scheduledBlock?: ScheduledTaskBlockViewModel;
  showBulkSelection?: boolean;
  task: TaskViewModel;
}): JSX.Element {
  const dueCue = taskDueCue(task);
  const scheduleLabel = taskScheduleLabel(task, scheduledBlock);

  return (
    <div
      className={cx(
        "min-h-[76px] border-b border-border px-3 py-2 last:border-b-0",
        selected || bulkSelected ? "bg-surface-0" : "bg-transparent"
      )}
      role="listitem"
    >
      <div className="flex min-w-0 flex-wrap items-start gap-3 sm:flex-nowrap">
        <TaskCompletionButton completed={completed} onToggle={onToggle} task={task} />
        {showBulkSelection ? (
          <input
            aria-label={`Select ${task.title}`}
            checked={bulkSelected}
            className="mt-1 size-4 shrink-0 accent-[var(--color-accent)]"
            onChange={(event) => onBulkSelect(task.id, event.target.checked)}
            type="checkbox"
          />
        ) : null}
        <button
          className="min-w-0 flex-1 text-left focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
          onClick={() => onSelect(task.id)}
          type="button"
        >
          <div className="flex min-w-0 items-center gap-2">
            <span
              className={cx(
                "truncate text-[var(--text-md)] font-medium text-text-primary",
                completed && "text-text-muted line-through"
              )}
            >
              {task.title}
            </span>
            {dueCue ? (
              <Badge aria-label={`Task due state ${task.title}`} tone={dueCue.tone}>
                {dueCue.label}
              </Badge>
            ) : null}
          </div>
          <p className="truncate text-[var(--text-sm)] text-text-muted">{task.detail}</p>
          {scheduleLabel || task.tags?.length ? (
            <div className="mt-2 flex flex-wrap gap-1">
              {scheduleLabel ? (
                <Badge
                  aria-label={`Task schedule ${task.title}`}
                  className="gap-1"
                  tone={taskScheduleTone(scheduledBlock)}
                >
                  <CalendarClock aria-hidden="true" size={11} />
                  {scheduleLabel}
                </Badge>
              ) : null}
              {task.tags?.slice(0, 3).map((tag) => (
                <Badge key={tag} tone="neutral">
                  +{tag}
                </Badge>
              ))}
            </div>
          ) : null}
          {task.subtasks.length > 0 ? (
            <div
              aria-label={`Subtasks for ${task.title}`}
              className="mt-2 flex flex-wrap gap-1"
            >
              {task.subtasks.map((subtask) => (
                <span
                  className="inline-flex max-w-full items-center gap-1 rounded-hcbSm border border-border bg-bg-tertiary px-2 py-0.5 text-[var(--text-xs)] text-text-secondary"
                  key={subtask.id}
                >
                  {subtask.completed ? (
                    <CheckCircle2 aria-hidden="true" className="text-success" size={12} />
                  ) : (
                    <Circle aria-hidden="true" className="text-text-muted" size={12} />
                  )}
                  <span className="truncate">{subtask.title}</span>
                </span>
              ))}
            </div>
          ) : null}
        </button>
        <div className="flex shrink-0 items-center gap-2">
          <Badge aria-label={`Task priority ${task.title}`} className="gap-1" tone={priorityTone(task.priority)}>
            <Flag aria-hidden="true" size={11} />
            {priorityLabel(task.priority)}
          </Badge>
          <Badge>{task.list}</Badge>
          <IconButton
            icon={Trash2}
            label={`Delete ${task.title}`}
            onClick={() => onDelete(task.id)}
            variant="ghost"
          />
        </div>
      </div>
    </div>
  );
}

export function TaskGroupPanel({
  group,
  bulkSelectedTaskIds,
  onBulkSelectTask,
  onDeleteTask,
  onSelectTask,
  onToggleTask,
  scheduledBlocksByTaskId,
  selectedTaskId
}: {
  group: TaskGroupViewModel;
  bulkSelectedTaskIds: string[];
  onBulkSelectTask: (taskId: string, selected: boolean) => void;
  onDeleteTask: (taskId: string) => void;
  onSelectTask: (taskId: string) => void;
  onToggleTask: (taskId: string) => void;
  scheduledBlocksByTaskId?: Map<string, ScheduledTaskBlockViewModel>;
  selectedTaskId: string | null;
}): JSX.Element {
  return (
    <Panel
      description={group.description}
      title={group.title}
      action={<Badge tone="neutral">{group.countLabel}</Badge>}
    >
      <VirtualizedList
        ariaLabel={`${group.title} tasks`}
        estimateRowHeight={88}
        getKey={(task) => task.id}
        items={group.tasks}
        performanceLabel={`tasks.${group.id}`}
        renderRow={(task) => (
          <TaskRow
            bulkSelected={bulkSelectedTaskIds.includes(task.id)}
            completed={task.status === "completed"}
            onBulkSelect={onBulkSelectTask}
            onDelete={onDeleteTask}
            onSelect={onSelectTask}
            onToggle={onToggleTask}
            scheduledBlock={scheduledBlocksByTaskId?.get(task.id)}
            selected={task.id === selectedTaskId}
            showBulkSelection={bulkSelectedTaskIds.length > 0}
            task={task}
          />
        )}
        viewportHeight={Math.min(250, Math.max(106, group.tasks.length * 88))}
      />
    </Panel>
  );
}

export function EventRow({
  event,
  onOpen
}: {
  event: CalendarEventViewModel;
  onOpen?: (event: CalendarEventViewModel) => void;
}): JSX.Element {
  const content = (
    <>
      <span className="flex h-7 w-16 shrink-0 items-center justify-center rounded-hcbSm border border-border bg-surface-0 font-mono text-[var(--text-xs)] text-text-secondary">
        {event.timeLabel}
      </span>
      <span className="min-w-0 flex-1">
        <span className="flex min-w-0 items-center gap-2">
          <span className="truncate text-[var(--text-md)] font-medium text-text-primary">{event.title}</span>
          <span className="shrink-0 text-[var(--text-xs)] text-text-muted">{event.rangeLabel}</span>
        </span>
        <span className="block truncate text-[var(--text-sm)] text-text-muted">
          {event.calendar} - {event.location} - {event.notes}
        </span>
      </span>
      <Badge tone="accent">Event</Badge>
    </>
  );

  if (onOpen) {
    return (
      <button
        className="flex min-h-11 w-full items-center gap-3 border-b border-border bg-transparent px-3 py-2 text-left last:border-b-0 transition-colors duration-fast ease-hcb hover:bg-surface-0 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
        onClick={() => onOpen(event)}
        role="listitem"
        type="button"
      >
        {content}
      </button>
    );
  }

  return (
    <div
      className="flex min-h-11 w-full items-center gap-3 border-b border-border bg-transparent px-3 py-2 last:border-b-0"
      role="listitem"
    >
      {content}
    </div>
  );
}

export function timeLabel(value: string): string {
  return new Intl.DateTimeFormat("en-US", {
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
    timeZone: "UTC"
  }).format(new Date(value));
}

export function addMinutesIso(startsAt: string, minutes: number): string {
  return new Date(Date.parse(startsAt) + minutes * 60 * 1000).toISOString();
}

export function dateOnlyFromLocalDate(date: Date): string {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");

  return `${year}-${month}-${day}`;
}

export function defaultTaskListId(source: ReturnType<typeof useCoreViewModelSource>): string {
  return source.taskLists[0]?.id ?? "";
}

export function defaultCalendarId(source: ReturnType<typeof useCoreViewModelSource>): string {
  return (
    source.calendarSources.find((calendar) => calendar.selected)?.id ??
    source.calendarSources[0]?.id ??
    ""
  );
}

function sanitizeInspectorDetails(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value.map(sanitizeInspectorDetails);
  }

  if (!value || typeof value !== "object") {
    return value;
  }

  const result: Record<string, unknown> = {};

  for (const [key, entry] of Object.entries(value)) {
    const normalized = key.toLowerCase();

    if (
      normalized.includes("token") ||
      normalized.includes("secret") ||
      normalized.includes("credential") ||
      normalized.includes("password") ||
      normalized.includes("path") ||
      normalized.includes("payload") ||
      normalized.includes("body")
    ) {
      result[key] = "[redacted]";
      continue;
    }

    result[key] = sanitizeInspectorDetails(entry);
  }

  return result;
}

export function sanitizedJson(value: unknown): string {
  return JSON.stringify(sanitizeInspectorDetails(value), null, 2);
}
