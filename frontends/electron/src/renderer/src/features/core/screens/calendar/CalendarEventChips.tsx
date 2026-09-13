import { useRef, useState } from "react";
import type { CSSProperties, DragEvent, KeyboardEvent } from "react";
import type { CalendarEventCompletionScope, SettingsSnapshot } from "@shared/ipc/contracts";
import { CheckCircle2, Circle } from "lucide-react";
import { FloatingMenu } from "../../../../components/FloatingMenu";
import { cx } from "../../../../components/primitives";
import type { CalendarEventViewModel } from "../../coreViewModels";

const calendarSourceTones = [
  {
    border: "border-l-accent",
    swatch: "bg-accent"
  },
  {
    border: "border-l-success",
    swatch: "bg-success"
  },
  {
    border: "border-l-warning",
    swatch: "bg-warning"
  },
  {
    border: "border-l-info",
    swatch: "bg-info"
  },
  {
    border: "border-l-danger",
    swatch: "bg-danger"
  }
] as const;

const calendarSourceToneCache = new Map<string, (typeof calendarSourceTones)[number]>();

export function calendarSourceTone(calendarId: string): (typeof calendarSourceTones)[number] {
  const cached = calendarSourceToneCache.get(calendarId);

  if (cached) {
    return cached;
  }

  let hash = 0;

  for (let index = 0; index < calendarId.length; index += 1) {
    hash = (hash * 31 + calendarId.charCodeAt(index)) >>> 0;
  }

  const tone = calendarSourceTones[hash % calendarSourceTones.length];
  calendarSourceToneCache.set(calendarId, tone);
  return tone;
}

export function normalizeCalendarColor(color: string | null | undefined): string | null {
  const normalized = color?.trim() ?? "";

  if (!/^#[0-9A-Fa-f]{6}$/.test(normalized)) {
    return null;
  }

  return normalized;
}

export function calendarSourceColorStyle(color: string | null | undefined): CSSProperties | undefined {
  const normalized = normalizeCalendarColor(color);

  if (!normalized) {
    return undefined;
  }

  return { backgroundColor: normalized };
}

function calendarEventAccentStyle(color: string | null | undefined): CSSProperties | undefined {
  const normalized = normalizeCalendarColor(color);

  if (!normalized) {
    return undefined;
  }

  return {
    backgroundImage: `linear-gradient(90deg, ${hexToRgba(normalized, 0.1)} 0%, transparent 56%)`
  };
}

export function calendarEventFillStyle(event: CalendarEventViewModel): CSSProperties | undefined {
  const background = normalizeCalendarColor(event.displayBackgroundColor ?? event.calendarBackgroundColor);

  if (!background) {
    return undefined;
  }

  return {
    backgroundColor: background,
    borderColor: background,
    color: normalizeCalendarColor(event.displayForegroundColor ?? event.calendarForegroundColor) ?? readableTextColor(background)
  };
}

function hexToRgba(color: string, alpha: number): string {
  const red = Number.parseInt(color.slice(1, 3), 16);
  const green = Number.parseInt(color.slice(3, 5), 16);
  const blue = Number.parseInt(color.slice(5, 7), 16);

  return `rgba(${red}, ${green}, ${blue}, ${alpha})`;
}

function readableTextColor(color: string): string {
  const red = Number.parseInt(color.slice(1, 3), 16);
  const green = Number.parseInt(color.slice(3, 5), 16);
  const blue = Number.parseInt(color.slice(5, 7), 16);
  const luminance = (0.299 * red + 0.587 * green + 0.114 * blue) / 255;

  return luminance > 0.55 ? "#1d1d1d" : "#ffffff";
}

function calendarEventLabel(
  event: CalendarEventViewModel,
  variant: "range" | "time" | "title"
): string {
  if (variant === "range") {
    return `${event.rangeLabel} ${event.title}`;
  }

  if (variant === "time") {
    return event.allDay ? event.title : `${event.timeLabel} ${event.title}`;
  }

  return event.title;
}

type CalendarEventChipSize = "default" | "compact";
export type EventCompletionDefaultScope = SettingsSnapshot["eventCompletionDefaultScope"];

const eventCompletionScopeOptions: Array<{ id: CalendarEventCompletionScope; label: string }> = [
  { id: "occurrence", label: "This occurrence" },
  { id: "seriesFuture", label: "Future series" },
  { id: "seriesAll", label: "Whole series" }
];

function eventCompleted(event: CalendarEventViewModel): boolean {
  return event.completedAt !== null && event.completedAt !== undefined || event.taskStatus === "completed";
}

function compareOverflowEvents(left: CalendarEventViewModel, right: CalendarEventViewModel): number {
  const leftCompleted = eventCompleted(left) ? 0 : 1;
  const rightCompleted = eventCompleted(right) ? 0 : 1;

  return (
    leftCompleted - rightCompleted ||
    left.startsAt.localeCompare(right.startsAt) ||
    left.endsAt.localeCompare(right.endsAt) ||
    left.id.localeCompare(right.id)
  );
}

export function CalendarItemCompletionButton({
  event,
  eventCompletionDefaultScope,
  onToggleEvent,
  onToggleTask,
  size = "default"
}: {
  event: CalendarEventViewModel;
  eventCompletionDefaultScope?: EventCompletionDefaultScope;
  onToggleEvent?: (eventId: string, scope?: CalendarEventCompletionScope) => void;
  onToggleTask?: (taskId: string) => void;
  size?: CalendarEventChipSize;
}): JSX.Element | null {
  const [scopeMenuOpen, setScopeMenuOpen] = useState(false);
  const buttonRef = useRef<HTMLButtonElement | null>(null);
  const isTask = event.sourceKind === "task";
  const isCompletedTask = event.taskStatus === "completed";
  const isCompletedEvent = event.sourceKind === "event" && eventCompleted(event);
  const completed = isCompletedTask || isCompletedEvent;
  const Icon = completed ? CheckCircle2 : Circle;
  const iconSize = size === "compact" ? 13 : 14;

  if (isTask && event.taskId && onToggleTask) {
    const label = isCompletedTask ? `Reopen ${event.title}` : `Mark ${event.title} complete`;

    return (
      <button
        aria-label={label}
        className="shrink-0 rounded-full text-current transition-colors duration-fast ease-hcb hover:text-accent focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent"
        onClick={(clickEvent) => {
          clickEvent.stopPropagation();
          onToggleTask(event.taskId ?? "");
        }}
        onKeyDown={(keyEvent) => keyEvent.stopPropagation()}
        onPointerDown={(pointerEvent) => pointerEvent.stopPropagation()}
        title={label}
        type="button"
      >
        <Icon aria-hidden="true" size={iconSize} />
      </button>
    );
  }

  if (event.sourceKind !== "event" || !onToggleEvent) {
    return isTask ? <Icon aria-hidden="true" className="shrink-0" size={iconSize} /> : null;
  }

  const label = isCompletedEvent ? `Reopen ${event.title}` : `Mark ${event.title} complete`;

  function chooseScope(scope: CalendarEventCompletionScope): void {
    setScopeMenuOpen(false);
    onToggleEvent?.(event.id, scope);
  }

  return (
    <>
      <button
        aria-label={label}
        className="shrink-0 rounded-full text-current transition-colors duration-fast ease-hcb hover:text-accent focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent"
        onClick={(clickEvent) => {
          clickEvent.stopPropagation();
          if (eventCompletionDefaultScope === "ask") {
            setScopeMenuOpen(true);
            return;
          }
          onToggleEvent(event.id, eventCompletionDefaultScope ?? "occurrence");
        }}
        onKeyDown={(keyEvent) => keyEvent.stopPropagation()}
        onPointerDown={(pointerEvent) => pointerEvent.stopPropagation()}
        ref={buttonRef}
        title={label}
        type="button"
      >
        <Icon aria-hidden="true" size={iconSize} />
      </button>
      {scopeMenuOpen ? (
        <FloatingMenu anchorRef={buttonRef} onClose={() => setScopeMenuOpen(false)} width={220}>
          <div className="grid p-1">
            {eventCompletionScopeOptions.map((scope) => (
              <button
                className="rounded-hcbSm px-2 py-1.5 text-left text-[var(--text-sm)] text-text-secondary hover:bg-surface-0 hover:text-text-primary focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent"
                key={scope.id}
                onClick={() => chooseScope(scope.id)}
                type="button"
              >
                {scope.label}
              </button>
            ))}
          </div>
        </FloatingMenu>
      ) : null}
    </>
  );
}

export function CalendarSourceSwatch({
  calendarId,
  color,
  className
}: {
  calendarId: string;
  color?: string | null;
  className?: string;
}): JSX.Element {
  const tone = calendarSourceTone(calendarId);
  const colorStyle = calendarSourceColorStyle(color);

  return (
    <span
      aria-hidden="true"
      className={cx("size-2.5 shrink-0 rounded-full", colorStyle ? undefined : tone.swatch, className)}
      style={colorStyle}
    />
  );
}

export function CalendarEventChip({
  className,
  draggable = false,
  event,
  labelVariant,
  onDragStart,
  eventCompletionDefaultScope,
  onKeyDown,
  onOpen,
  onToggleEvent,
  onToggleTask,
  size = "default"
}: {
  className?: string;
  draggable?: boolean;
  event: CalendarEventViewModel;
  eventCompletionDefaultScope?: EventCompletionDefaultScope;
  labelVariant: "range" | "time" | "title";
  onDragStart?: (dragEvent: DragEvent<HTMLElement>) => void;
  onKeyDown?: (keyEvent: KeyboardEvent<HTMLElement>) => void;
  onOpen?: (event: CalendarEventViewModel) => void;
  onToggleEvent?: (eventId: string, scope?: CalendarEventCompletionScope) => void;
  onToggleTask?: (taskId: string) => void;
  size?: CalendarEventChipSize;
}): JSX.Element {
  const tone = calendarSourceTone(event.calendarId);
  const fillStyle = calendarEventFillStyle(event);
  const accentStyle = fillStyle ?? calendarEventAccentStyle(event.calendarBackgroundColor);
  const label = calendarEventLabel(event, labelVariant);
  const isCompletedTask = event.taskStatus === "completed";
  const isCompletedEvent = eventCompleted(event);
  const completed = isCompletedTask || isCompletedEvent;

  return (
    <div
      aria-label={label}
      className={cx(
        "group flex w-full min-w-0 cursor-default items-center gap-1.5 overflow-hidden rounded-hcbSm border border-border bg-surface-0 text-left text-text-secondary shadow-sm transition-colors duration-fast ease-hcb hover:bg-surface-1 hover:text-text-primary focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent",
        size === "compact" ? "min-h-0 px-1.5 py-0.5 text-[11px]" : "min-h-6 px-2 py-1 text-[var(--text-xs)]",
        draggable && "cursor-grab active:cursor-grabbing",
        event.allDay && "font-medium",
        completed && "text-text-muted opacity-70",
        fillStyle && "hover:brightness-95",
        accentStyle ? undefined : tone.border,
        className
      )}
      draggable={draggable}
      onClick={(clickEvent) => {
        clickEvent.stopPropagation();
        onOpen?.(event);
      }}
      onDragStart={onDragStart}
      onKeyDown={(keyEvent) => {
        onKeyDown?.(keyEvent);
        if (keyEvent.defaultPrevented) {
          return;
        }

        if (keyEvent.key === "Enter" || keyEvent.key === " ") {
          keyEvent.preventDefault();
          keyEvent.stopPropagation();
          onOpen?.(event);
        }
      }}
      onPointerDown={(pointerEvent) => pointerEvent.stopPropagation()}
      role="button"
      style={accentStyle}
      tabIndex={0}
      title={`${label} - ${event.calendar}`}
    >
      <CalendarItemCompletionButton
        event={event}
        eventCompletionDefaultScope={eventCompletionDefaultScope}
        onToggleEvent={onToggleEvent}
        onToggleTask={onToggleTask}
        size={size}
      />
      <span className={cx("min-w-0 flex-1 truncate leading-tight", completed && "line-through")}>
        {label}
      </span>
    </div>
  );
}

export function CalendarOverflowChip({
  count,
  onOpen
}: {
  count: number;
  onOpen?: () => void;
}): JSX.Element {
  const className =
    "inline-flex min-h-5 max-w-full items-center truncate rounded-hcbSm border border-dashed border-border px-2 text-[var(--text-xs)] text-text-muted";

  if (onOpen) {
    return (
      <button
        aria-label={`Show ${count} more calendar items`}
        className={cx(className, "hover:bg-surface-0 hover:text-accent focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent")}
        onClick={(event) => {
          event.stopPropagation();
          onOpen();
        }}
        onPointerDown={(event) => event.stopPropagation()}
        type="button"
      >
        {count} more
      </button>
    );
  }

  return (
    <span className={className}>
      {count} more
    </span>
  );
}

export function CalendarOverflowPopover({
  eventCompletionDefaultScope,
  events,
  onClose,
  onOpen,
  onToggleEvent,
  onToggleTask,
  title
}: {
  eventCompletionDefaultScope?: EventCompletionDefaultScope;
  events: CalendarEventViewModel[];
  onClose: () => void;
  onOpen: (event: CalendarEventViewModel) => void;
  onToggleEvent?: (eventId: string, scope?: CalendarEventCompletionScope) => void;
  onToggleTask?: (taskId: string) => void;
  title: string;
}): JSX.Element {
  const orderedEvents = [...events].sort(compareOverflowEvents);

  return (
    <div
      className="fixed inset-0 z-50 grid place-items-center bg-bg-tertiary/45 p-3 backdrop-blur-sm"
      onClick={(event) => {
        event.stopPropagation();
        onClose();
      }}
      onKeyDown={(event) => {
        if (event.key === "Escape") {
          event.stopPropagation();
          onClose();
        }
      }}
      role="presentation"
    >
      <section
        aria-label={title}
        aria-modal="true"
        className="w-full max-w-md overflow-hidden rounded-hcbLg border border-border bg-bg-primary shadow-xl"
        onClick={(event) => event.stopPropagation()}
        role="dialog"
      >
        <div className="flex items-center justify-between gap-3 border-b border-border px-3 py-2">
          <h2 className="min-w-0 truncate text-[var(--text-sm)] font-semibold text-text-primary">{title}</h2>
          <button
            aria-label="Close overflow events"
            className="rounded-hcbSm px-2 py-1 text-[var(--text-xs)] font-semibold text-text-muted hover:bg-surface-0 hover:text-text-primary focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
            onClick={onClose}
            type="button"
          >
            X
          </button>
        </div>
        <div className="grid max-h-[60vh] gap-1 overflow-auto p-2">
          {orderedEvents.map((event) => (
            <CalendarEventChip
              event={event}
              eventCompletionDefaultScope={eventCompletionDefaultScope}
              key={event.id}
              labelVariant="range"
              onOpen={(selectedEvent) => {
                onClose();
                onOpen(selectedEvent);
              }}
              onToggleEvent={onToggleEvent}
              onToggleTask={onToggleTask}
            />
          ))}
        </div>
      </section>
    </div>
  );
}

export function CalendarAllDayLane({
  dayLabel,
  eventCompletionDefaultScope,
  events,
  onCreate,
  onOpen,
  onToggleEvent,
  onToggleTask,
  visibleCount = 4
}: {
  dayLabel: string;
  eventCompletionDefaultScope?: EventCompletionDefaultScope;
  events: CalendarEventViewModel[];
  onCreate?: () => void;
  onOpen: (event: CalendarEventViewModel) => void;
  onToggleEvent?: (eventId: string, scope?: CalendarEventCompletionScope) => void;
  onToggleTask?: (taskId: string) => void;
  visibleCount?: number;
}): JSX.Element {
  const visibleEvents = events.slice(0, visibleCount);
  const overflowCount = Math.max(0, events.length - visibleEvents.length);

  return (
    <div
      aria-label={`All-day events for ${dayLabel}`}
      className="grid min-h-10 grid-cols-[72px_minmax(0,1fr)] border-b border-border bg-bg-secondary"
      role="group"
    >
      <div className="border-r border-border px-2 py-2 text-[var(--text-xs)] font-medium text-text-muted">
        All day
      </div>
      <div className="flex min-w-0 flex-wrap items-center gap-1 px-2 py-1.5">
        {visibleEvents.map((event) => (
          <div className="min-w-0 basis-[180px] grow" key={event.id}>
            <CalendarEventChip
              event={event}
              eventCompletionDefaultScope={eventCompletionDefaultScope}
              labelVariant="title"
              onOpen={onOpen}
              onToggleEvent={onToggleEvent}
              onToggleTask={onToggleTask}
            />
          </div>
        ))}
        {overflowCount > 0 ? <CalendarOverflowChip count={overflowCount} /> : null}
        {events.length === 0 && onCreate ? (
          <button
            className="min-h-7 rounded-hcbSm border border-dashed border-border px-2 text-left text-[var(--text-xs)] text-text-muted transition-colors duration-fast ease-hcb hover:bg-surface-0 hover:text-accent focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
            data-action-id="calendar.create"
            onClick={onCreate}
            type="button"
          >
            Add all-day event
          </button>
        ) : null}
      </div>
    </div>
  );
}
