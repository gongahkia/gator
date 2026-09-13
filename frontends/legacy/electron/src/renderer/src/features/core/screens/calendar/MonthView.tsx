import { useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties, PointerEvent as ReactPointerEvent } from "react";
import type { CalendarEventCompletionScope } from "@shared/ipc/contracts";
import { cx } from "../../../../components/primitives";
import { handleActivationKeyDown } from "../../coreScreenShared";
import type { CalendarEventViewModel, CalendarMonthWeekViewModel } from "../../coreViewModels";
import {
  CalendarEventChip,
  CalendarOverflowChip,
  CalendarOverflowPopover,
  type EventCompletionDefaultScope
} from "./CalendarEventChips";
import { calendarAddUtcDays, calendarDateTitle, calendarMonthVisibleChipCount, visibleCalendarMonthWeeks } from "./calendarGrid";
import type { CalendarCreateSeed, CalendarTimelineAllDaySegment } from "./types";

const monthEventLaneHeight = 24;

type MonthPointerEvent = Pick<globalThis.PointerEvent, "buttons" | "clientX" | "clientY" | "pointerId" | "type" | "preventDefault">;

function monthAllDaySegmentStyle(segment: CalendarTimelineAllDaySegment): CSSProperties {
  return {
    gridColumn: `${segment.startDayIndex + 1} / span ${segment.daySpan}`,
    gridRow: `${segment.laneIndex + 2}`
  };
}

function monthEventChipStyle(dayIndex: number, laneIndex: number): CSSProperties {
  return {
    gridColumn: `${dayIndex + 1}`,
    gridRow: `${laneIndex + 2}`
  };
}

function monthOverflowStyle(dayIndex: number): CSSProperties {
  return {
    gridColumn: `${dayIndex + 1}`,
    gridRow: `${calendarMonthVisibleChipCount + 2}`
  };
}

export function MonthView({
  eventCompletionDefaultScope,
  weeks,
  onCreate,
  onOpen,
  onToggleEvent,
  onToggleTask,
  todayKey,
  visibleCalendarIds
}: {
  eventCompletionDefaultScope?: EventCompletionDefaultScope;
  weeks: CalendarMonthWeekViewModel[];
  onCreate: (seed?: CalendarCreateSeed) => void;
  onOpen: (event: CalendarEventViewModel) => void;
  onToggleEvent?: (eventId: string, scope?: CalendarEventCompletionScope) => void;
  onToggleTask?: (taskId: string) => void;
  todayKey: string;
  visibleCalendarIds: ReadonlySet<string>;
}): JSX.Element {
  const [activeOverflow, setActiveOverflow] = useState<{
    events: CalendarEventViewModel[];
    title: string;
  } | null>(null);
  const [rangeSelection, setRangeSelection] = useState<{ end: string; start: string } | null>(null);
  const [suppressClick, setSuppressClick] = useState(false);
  const visibleWeeks = useMemo(
    () => visibleCalendarMonthWeeks(weeks, visibleCalendarIds),
    [weeks, visibleCalendarIds]
  );
  const visibleWeekIds = visibleWeeks.map((week) => week.id).join("|");
  const dragRangeRef = useRef<{ last: string; moved: boolean; start: string } | null>(null);
  const monthGridRef = useRef<HTMLDivElement | null>(null);
  const removeMonthPointerListenersRef = useRef<(() => void) | null>(null);
  const todayCellRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    const frame = window.requestAnimationFrame(() => {
      todayCellRef.current?.scrollIntoView?.({ block: "center", inline: "nearest" });
    });

    return () => window.cancelAnimationFrame(frame);
  }, [todayKey, visibleWeekIds]);

  useEffect(() => () => removeMonthPointerListenersRef.current?.(), []);

  function orderedRange(start: string, end: string): { end: string; start: string } {
    return start <= end ? { start, end } : { start: end, end: start };
  }

  function rangeContains(dayKey: string): boolean {
    if (!rangeSelection) {
      return false;
    }

    const range = orderedRange(rangeSelection.start, rangeSelection.end);
    return dayKey >= range.start && dayKey <= range.end;
  }

  function dayKeyFromPointer(pointerEvent: Pick<MonthPointerEvent, "clientX" | "clientY">): string | null {
    const rows = monthGridRef.current?.querySelectorAll<HTMLElement>("[data-calendar-month-week-row]");

    if (!rows) {
      return null;
    }

    for (let weekIndex = 0; weekIndex < rows.length; weekIndex += 1) {
      const rect = rows[weekIndex].getBoundingClientRect();

      if (
        pointerEvent.clientY < rect.top ||
        pointerEvent.clientY > rect.bottom ||
        pointerEvent.clientX < rect.left ||
        pointerEvent.clientX > rect.right
      ) {
        continue;
      }

      const dayIndex = Math.max(
        0,
        Math.min(6, Math.floor(((pointerEvent.clientX - rect.left) / Math.max(1, rect.width)) * 7))
      );
      const day = visibleWeeks[weekIndex]?.days[dayIndex]?.day;

      return day ? day.id.slice("month-".length) : null;
    }

    return null;
  }

  function updateDayPointerDrag(pointerEvent: MonthPointerEvent): string | null {
    const drag = dragRangeRef.current;

    if (!drag) {
      return null;
    }

    pointerEvent.preventDefault();
    const dayKey = dayKeyFromPointer(pointerEvent) ?? drag.last;
    drag.moved = drag.moved || drag.start !== dayKey;
    drag.last = dayKey;
    setRangeSelection({ start: drag.start, end: dayKey });
    return dayKey;
  }

  function clearDayPointerDrag(): void {
    dragRangeRef.current = null;
    setRangeSelection(null);
    removeMonthPointerListenersRef.current?.();
    removeMonthPointerListenersRef.current = null;
  }

  function installMonthPointerListeners(pointerId: number): void {
    removeMonthPointerListenersRef.current?.();

    const handlePointerMove = (pointerEvent: globalThis.PointerEvent): void => {
      if (pointerEvent.pointerId === pointerId) {
        updateDayPointerDrag(pointerEvent);
      }
    };
    const handlePointerUp = (pointerEvent: globalThis.PointerEvent): void => {
      if (pointerEvent.pointerId === pointerId) {
        handleDayPointerUp(pointerEvent);
      }
    };
    const handlePointerCancel = (pointerEvent: globalThis.PointerEvent): void => {
      if (pointerEvent.pointerId === pointerId) {
        clearDayPointerDrag();
      }
    };

    window.addEventListener("pointermove", handlePointerMove, { passive: false });
    window.addEventListener("pointerup", handlePointerUp, { passive: false });
    window.addEventListener("pointercancel", handlePointerCancel, { passive: false });
    removeMonthPointerListenersRef.current = () => {
      window.removeEventListener("pointermove", handlePointerMove);
      window.removeEventListener("pointerup", handlePointerUp);
      window.removeEventListener("pointercancel", handlePointerCancel);
    };
  }

  function handleDayPointerDown(pointerEvent: ReactPointerEvent<HTMLDivElement>, dayKey: string): void {
    if (pointerEvent.button > 0) {
      return;
    }

    pointerEvent.preventDefault();
    pointerEvent.currentTarget.setPointerCapture?.(pointerEvent.pointerId);
    dragRangeRef.current = { last: dayKey, moved: false, start: dayKey };
    installMonthPointerListeners(pointerEvent.pointerId);
    setRangeSelection({ start: dayKey, end: dayKey });
  }

  function handleDayPointerUp(pointerEvent: MonthPointerEvent): void {
    if (!dragRangeRef.current) {
      return;
    }

    const start = dragRangeRef.current.start;
    const end = updateDayPointerDrag(pointerEvent) ?? dragRangeRef.current.last;
    const moved = dragRangeRef.current.moved || start !== end;
    const range = orderedRange(start, end);
    clearDayPointerDrag();

    if (!moved) {
      return;
    }

    setSuppressClick(true);
    onCreate({
      allDay: true,
      startsAt: `${range.start}T00:00:00.000Z`,
      endsAt: `${calendarAddUtcDays(range.end, 1)}T00:00:00.000Z`
    });
  }

  return (
    <div className="flex min-h-[680px] select-none flex-col overflow-hidden rounded-hcbMd border border-border bg-bg-secondary" role="grid" aria-label="Calendar month view">
      <div className="grid grid-cols-7 border-b border-border bg-bg-primary/40 text-center text-[var(--text-xs)] font-semibold text-text-muted" role="row">
        {["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"].map((weekday) => (
          <div className="border-r border-border px-2 py-2 last:border-r-0" key={weekday} role="columnheader">
            {weekday}
          </div>
        ))}
      </div>
      <div
        className="grid flex-1 grid-rows-6"
        onPointerCancel={() => {
          dragRangeRef.current = null;
          setRangeSelection(null);
        }}
        onPointerMove={updateDayPointerDrag}
        onPointerUp={handleDayPointerUp}
        ref={monthGridRef}
        role="rowgroup"
      >
        {visibleWeeks.map((week, weekIndex) => (
          <div
            className="grid gap-y-1 border-b border-border last:border-b-0"
            data-calendar-month-week-row
            key={week.id}
            role="row"
            style={{
              gridTemplateColumns: "repeat(7, minmax(0, 1fr))",
              gridTemplateRows: `28px repeat(${calendarMonthVisibleChipCount}, ${monthEventLaneHeight}px) ${monthEventLaneHeight}px minmax(0, 1fr)`
            }}
          >
            {week.days.map(({ day, overflowCount, visibleEventChips }, dayIndex) => {
              const dayKey = day.id.slice("month-".length);
              const isCurrentDay = dayKey === todayKey;

              return (
                <div
                  className={cx(
                    "relative z-0 min-h-[126px] border-r border-border bg-bg-tertiary px-2 py-1.5 text-left transition-colors duration-fast ease-hcb last:border-r-0 hover:bg-surface-0 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent",
                    isCurrentDay && "bg-surface-0 ring-1 ring-inset ring-accent",
                    day.isOutsideMonth && "opacity-50",
                    rangeContains(dayKey) && "bg-info/15 ring-1 ring-inset ring-info"
                  )}
                  data-calendar-month-today={isCurrentDay || undefined}
                  key={day.id}
                  onClick={() => {
                    if (suppressClick) {
                      setSuppressClick(false);
                      return;
                    }

                    onCreate({ startsAt: `${dayKey}T00:00:00.000Z`, allDay: true });
                  }}
                  onKeyDown={(event) =>
                    handleActivationKeyDown(event, () =>
                      onCreate({ startsAt: `${dayKey}T00:00:00.000Z`, allDay: true })
                    )
                  }
                  onPointerEnter={updateDayPointerDrag}
                  onPointerDown={(event) => handleDayPointerDown(event, dayKey)}
                  onPointerMove={updateDayPointerDrag}
                  onPointerUp={handleDayPointerUp}
                  ref={(node) => {
                    if (isCurrentDay) {
                      todayCellRef.current = node;
                    }
                  }}
                  role="gridcell"
                  style={{ gridColumn: `${dayIndex + 1}`, gridRow: "1 / -1" }}
                  tabIndex={0}
                >
                  <span className="sr-only">{day.weekday}</span>
                  <span className="text-[var(--text-sm)] font-semibold text-text-primary">{day.dateLabel}</span>
                </div>
              );
            })}
            {week.allDaySegments.map((segment) => (
              <div
                className="z-10 min-w-0 px-1.5"
                data-calendar-month-all-day-segment={segment.event.id}
                data-day-span={segment.daySpan}
                data-ends-after-range={segment.endsAfterRange}
                data-lane-index={segment.laneIndex}
                data-start-day-index={segment.startDayIndex}
                data-starts-before-range={segment.startsBeforeRange}
                key={segment.event.id}
                role="presentation"
                style={monthAllDaySegmentStyle(segment)}
              >
                <CalendarEventChip
                  className="h-5"
                  event={segment.event}
                  eventCompletionDefaultScope={eventCompletionDefaultScope}
                  labelVariant="title"
                  onKeyDown={(keyEvent) => {
                    keyEvent.stopPropagation();
                    handleActivationKeyDown(keyEvent, () => onOpen(segment.event));
                  }}
                  onOpen={onOpen}
                  onToggleEvent={onToggleEvent}
                  onToggleTask={onToggleTask}
                  size="compact"
                />
              </div>
            ))}
            {week.days.flatMap(({ day, visibleEventChips }, dayIndex) =>
              visibleEventChips.map(({ event, laneIndex }) => (
                <div
                  className="z-10 min-w-0 px-1.5"
                  key={`${day.id}-${event.id}`}
                  role="presentation"
                  style={monthEventChipStyle(dayIndex, laneIndex)}
                >
                  <CalendarEventChip
                    className="h-5"
                    event={event}
                    eventCompletionDefaultScope={eventCompletionDefaultScope}
                    labelVariant="title"
                    onKeyDown={(keyEvent) => {
                      keyEvent.stopPropagation();
                      handleActivationKeyDown(keyEvent, () => onOpen(event));
                    }}
                    onOpen={onOpen}
                    onToggleEvent={onToggleEvent}
                    onToggleTask={onToggleTask}
                    size="compact"
                  />
                </div>
              ))
            )}
            {week.days.map(({ day, overflowCount, popupEvents }, dayIndex) =>
              overflowCount > 0 ? (
                <div
                  className="z-10 min-w-0 px-1.5"
                  key={`${day.id}-overflow`}
                  role="presentation"
                  style={monthOverflowStyle(dayIndex)}
                >
                  <CalendarOverflowChip
                    count={overflowCount}
                    onOpen={() =>
                      setActiveOverflow({
                        events: popupEvents,
                        title: `Items for ${calendarDateTitle(day)}`
                      })
                    }
                  />
                </div>
              ) : null
            )}
          </div>
        ))}
      </div>
      {activeOverflow ? (
        <CalendarOverflowPopover
          eventCompletionDefaultScope={eventCompletionDefaultScope}
          events={activeOverflow.events}
          onClose={() => setActiveOverflow(null)}
          onOpen={onOpen}
          onToggleEvent={onToggleEvent}
          onToggleTask={onToggleTask}
          title={activeOverflow.title}
        />
      ) : null}
    </div>
  );
}
