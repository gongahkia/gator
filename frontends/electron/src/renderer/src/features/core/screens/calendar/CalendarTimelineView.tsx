import { useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties, DragEvent, MouseEvent, PointerEvent, ReactNode } from "react";
import type { CalendarEventCompletionScope } from "@shared/ipc/contracts";
import { Minus, Plus } from "lucide-react";
import { IconButton, cx } from "../../../../components/primitives";
import { useCoreViewModelSource } from "../../coreViewModelSource";
import { handleActivationKeyDown } from "../../coreScreenShared";
import type { CalendarDayViewModel, CalendarEventViewModel } from "../../coreViewModels";
import {
  CalendarEventChip,
  CalendarOverflowChip,
  CalendarOverflowPopover,
  type EventCompletionDefaultScope
} from "./CalendarEventChips";
import {
  addUtcMinutesIso,
  calendarDateTitle,
  calendarDayKey,
  calendarDisplayHourLabel,
  calendarEventTimeOfDayIso,
  calendarLocalPoint,
  calendarPointerTimeIso,
  calendarRangeTitle,
  calendarTimeBlock,
  calendarTimelineHourHeight,
  calendarTimelineHours,
  calendarTimelineVisibleAllDayCount,
  calendarTodayKey,
  hourSlotIso,
  hourSlotLabel,
  visibleCalendarTimeline,
  zonedDateTimeIso
} from "./calendarGrid";
import {
  allowCalendarDrop,
  calendarEventDragId,
  calendarEventResizeDragId,
  startCalendarEventDrag,
  startCalendarEventResizeDrag
} from "./calendarDrag";
import type { CalendarCreateSeed, CalendarTimeBlock, CalendarTimelineAllDaySegment } from "./types";

const allDayLaneHeight = 28;

function CalendarTimelineEventChip({
  className,
  event,
  eventCompletionDefaultScope,
  labelVariant,
  onMoveEvent,
  onOpen,
  onToggleEvent,
  onToggleTask
}: {
  className?: string;
  event: CalendarEventViewModel;
  eventCompletionDefaultScope?: EventCompletionDefaultScope;
  labelVariant: "range" | "time" | "title";
  onMoveEvent: (eventId: string, startsAt: string, allDay: boolean) => void;
  onOpen: (event: CalendarEventViewModel) => void;
  onToggleEvent?: (eventId: string, scope?: CalendarEventCompletionScope) => void;
  onToggleTask?: (taskId: string) => void;
}): JSX.Element {
  const draggable = event.sourceKind !== "task";

  return (
    <>
      <CalendarEventChip
        className={cx("h-full min-h-0", className)}
        draggable={draggable}
        event={event}
        eventCompletionDefaultScope={eventCompletionDefaultScope}
        labelVariant={labelVariant}
        onDragStart={draggable ? (dragEvent) => startCalendarEventDrag(dragEvent, event.id) : undefined}
        onKeyDown={(keyEvent) => {
          if (!draggable) {
            return;
          }

          if (keyEvent.key !== "ArrowDown" && keyEvent.key !== "ArrowUp") {
            return;
          }

          keyEvent.preventDefault();
          keyEvent.stopPropagation();
          if (event.allDay) {
            return;
          }

          const direction = keyEvent.key === "ArrowDown" ? 1 : -1;
          onMoveEvent(
            event.id,
            new Date(Date.parse(event.startsAt) + direction * 15 * 60 * 1000).toISOString(),
            event.allDay
          );
        }}
        onOpen={onOpen}
        onToggleEvent={onToggleEvent}
        onToggleTask={onToggleTask}
        size="compact"
      />
      {draggable && !event.allDay ? (
        <button
          aria-label={`Resize ${event.title} end`}
          className="sr-only"
          draggable
          onDragStart={(dragEvent) => startCalendarEventResizeDrag(dragEvent, event.id)}
          title={`Resize ${event.title} end`}
          type="button"
        />
      ) : null}
    </>
  );
}

function timelineEventStyle({
  height,
  laneCount,
  laneIndex,
  top
}: {
  height: number;
  laneCount: number;
  laneIndex: number;
  top: number;
}): CSSProperties {
  const laneWidth = 100 / Math.max(1, laneCount);

  return {
    height: `${Math.max(22, height)}px`,
    left: `calc(${laneIndex * laneWidth}% + 4px)`,
    top: `${top}px`,
    width: `calc(${laneWidth}% - 8px)`
  };
}

function allDaySegmentStyle(segment: CalendarTimelineAllDaySegment): CSSProperties {
  return {
    gridColumn: `${segment.startDayIndex + 1} / span ${segment.daySpan}`,
    gridRow: `${segment.laneIndex + 1}`
  };
}

function timelinePreviewSegments(
  blocks: CalendarTimeBlock[],
  days: Array<{ day: CalendarDayViewModel }>,
  hourRowHeight: number,
  timeZone: string
): Array<{ dayId: string; height: number; id: string; top: number }> {
  return blocks.flatMap((block) => {
    const start = calendarLocalPoint(block.startsAt, timeZone);
    const end = calendarLocalPoint(block.endsAt, timeZone);

    return days.flatMap(({ day }) => {
      const dayKey = calendarDayKey(day);
      const startMinute = start.dayKey < dayKey ? 0 : start.dayKey > dayKey ? 1_440 : start.minutes;
      const endMinute = end.dayKey > dayKey ? 1_440 : end.dayKey < dayKey ? 0 : end.minutes;
      const clampedStart = Math.max(0, Math.min(1_440, startMinute));
      const clampedEnd = Math.max(0, Math.min(1_440, endMinute));

      if (clampedEnd <= clampedStart) {
        return [];
      }

      return [{
        dayId: day.id,
        height: Math.max(6, ((clampedEnd - clampedStart) / 60) * hourRowHeight),
        id: `${block.id}-${day.id}`,
        top: (clampedStart / 60) * hourRowHeight
      }];
    });
  });
}

function CalendarTimelineView({
  availabilityMode = false,
  availabilitySlots = [],
  days,
  dayCountControl,
  eventCompletionDefaultScope,
  gridLabel,
  label,
  onAddAvailabilitySlot,
  onCreate,
  onMoveEvent,
  onOpen,
  onResizeEvent,
  onToggleEvent,
  onToggleTask,
  timedLabelVariant = "time",
  title,
  visibleCalendarIds
}: {
  availabilityMode?: boolean;
  availabilitySlots?: CalendarTimeBlock[];
  days: CalendarDayViewModel[];
  dayCountControl?: ReactNode;
  eventCompletionDefaultScope?: EventCompletionDefaultScope;
  gridLabel: string;
  label: string;
  onAddAvailabilitySlot?: (slot: CalendarTimeBlock) => void;
  onCreate: (seed?: CalendarCreateSeed) => void;
  onMoveEvent: (eventId: string, startsAt: string, allDay: boolean) => void;
  onOpen: (event: CalendarEventViewModel) => void;
  onResizeEvent: (eventId: string, endsAt: string) => void;
  onToggleEvent?: (eventId: string, scope?: CalendarEventCompletionScope) => void;
  onToggleTask?: (taskId: string) => void;
  timedLabelVariant?: "range" | "time";
  title: string;
  visibleCalendarIds: ReadonlySet<string>;
}): JSX.Element {
  const source = useCoreViewModelSource();
  const [activeOverflow, setActiveOverflow] = useState<{
    events: CalendarEventViewModel[];
    title: string;
  } | null>(null);
  const [dragSelection, setDragSelection] = useState<CalendarTimeBlock | null>(null);
  const [dropPreview, setDropPreview] = useState<CalendarTimeBlock | null>(null);
  const [nowIso, setNowIso] = useState(() => new Date().toISOString());
  const scrollContainerRef = useRef<HTMLDivElement | null>(null);
  const timeGridRef = useRef<HTMLDivElement | null>(null);
  const timelineDragRef = useRef<{
    dayKey: string;
    mode: "availability" | "create";
    moved: boolean;
    startClientY: number;
    startsAt: string;
  } | null>(null);
  const suppressNextClickRef = useRef(false);
  const hourRowHeight = calendarTimelineHourHeight(source.settings.calendarTimelineDensity);
  const timeline = useMemo(
    () => visibleCalendarTimeline(days, visibleCalendarIds, hourRowHeight),
    [days, hourRowHeight, visibleCalendarIds]
  );
  const visibleDays = timeline.days;
  const visibleDayKeys = visibleDays.map(({ day }) => calendarDayKey(day)).join("|");
  const currentPoint = useMemo(
    () => calendarLocalPoint(nowIso, source.settings.defaultTimeZone),
    [nowIso, source.settings.defaultTimeZone]
  );
  const currentDayIndex = visibleDays.findIndex(({ day }) => calendarDayKey(day) === currentPoint.dayKey);
  const currentTimeTop = (currentPoint.minutes / 60) * hourRowHeight;
  const hasAllDayOverflow = timeline.allDayOverflowCounts.some((count) => count > 0);
  const allDayRowHeight = Math.max(
    112,
    calendarTimelineVisibleAllDayCount * allDayLaneHeight + (hasAllDayOverflow ? allDayLaneHeight : 0) + 16
  );
  const firstVisibleDayKey = visibleDays[0]
    ? calendarDayKey(visibleDays[0].day)
    : days[0]
      ? calendarDayKey(days[0])
      : calendarTodayKey();
  const dayColumnMinWidth = days.length <= 1 ? 520 : days.length <= 3 ? 220 : 160;
  const gridTemplateColumns = `repeat(${Math.max(1, days.length)}, minmax(${dayColumnMinWidth}px, 1fr))`;
  const previewSegments = timelinePreviewSegments(
    [
      ...availabilitySlots,
      ...(dragSelection ? [dragSelection] : []),
      ...(dropPreview ? [dropPreview] : [])
    ],
    visibleDays,
    hourRowHeight,
    source.settings.defaultTimeZone
  );

  useEffect(() => {
    const timer = window.setInterval(() => setNowIso(new Date().toISOString()), 60_000);
    return () => window.clearInterval(timer);
  }, []);

  useEffect(() => {
    if (currentDayIndex < 0) {
      return;
    }

    const scrollContainer = scrollContainerRef.current;
    const timeGrid = timeGridRef.current;

    if (!scrollContainer || !timeGrid) {
      return;
    }

    const frame = window.requestAnimationFrame(() => {
      const targetTop = timeGrid.offsetTop + currentTimeTop - scrollContainer.clientHeight * 0.35;
      scrollContainer.scrollTop = Math.max(0, targetTop);
    });

    return () => window.cancelAnimationFrame(frame);
  }, [currentDayIndex, currentPoint.dayKey, gridLabel, hourRowHeight, visibleDayKeys]);

  function handleDrop(
    dragEvent: DragEvent<HTMLElement>,
    dayKey: string,
    startsAt: string,
    allDay: boolean
  ): void {
    dragEvent.preventDefault();
    dragEvent.stopPropagation();
    setDropPreview(null);
    const resizeEventId = calendarEventResizeDragId(dragEvent);

    if (resizeEventId) {
      onResizeEvent(resizeEventId, startsAt);
      return;
    }

    const eventId = calendarEventDragId(dragEvent);
    const draggedEvent = eventId ? source.calendarEventsById[eventId] : undefined;

    if (!draggedEvent) {
      return;
    }

    if (allDay && !draggedEvent.allDay) {
      const nextStartsAt = calendarEventTimeOfDayIso(
        draggedEvent.startsAt,
        dayKey,
        draggedEvent.timeZone || source.settings.defaultTimeZone
      );
      onMoveEvent(draggedEvent.id, nextStartsAt, false);
      return;
    }

    onMoveEvent(
      draggedEvent.id,
      allDay ? `${dayKey}T00:00:00.000Z` : startsAt,
      allDay
    );
  }

  function previewDrop(
    dragEvent: DragEvent<HTMLElement>,
    dayKey: string,
    startsAt: string,
    allDay: boolean
  ): void {
    allowCalendarDrop(dragEvent);
    if (dragEvent.defaultPrevented === false) {
      return;
    }

    const resizeEventId = calendarEventResizeDragId(dragEvent);

    if (resizeEventId) {
      const event = source.calendarEventsById[resizeEventId];

      if (event) {
        setDropPreview(calendarTimeBlock(event.startsAt, startsAt, event.timeZone || source.settings.defaultTimeZone));
      }
      return;
    }

    const eventId = calendarEventDragId(dragEvent);
    const event = eventId ? source.calendarEventsById[eventId] : undefined;

    if (!event) {
      return;
    }

    const durationMs = Math.max(15 * 60 * 1000, Date.parse(event.endsAt) - Date.parse(event.startsAt));
    const nextStartsAt = allDay
      ? `${dayKey}T00:00:00.000Z`
      : startsAt;
    const nextEndsAt = allDay
      ? addUtcMinutesIso(nextStartsAt, 24 * 60)
      : new Date(Date.parse(nextStartsAt) + durationMs).toISOString();

    setDropPreview(calendarTimeBlock(nextStartsAt, nextEndsAt, event.timeZone || source.settings.defaultTimeZone));
  }

  function dayKeyForAllDayDrag(dragEvent: DragEvent<HTMLElement>): string | null {
    const container = dragEvent.currentTarget.closest<HTMLElement>("[data-calendar-all-day-events]");
    const rect = container?.getBoundingClientRect();

    if (!rect || visibleDays.length === 0) {
      return null;
    }

    const clientX = Number.isFinite(dragEvent.clientX) ? dragEvent.clientX : rect.left;
    const offset = Math.min(Math.max(clientX - rect.left, 0), Math.max(1, rect.width) - 1);
    const index = Math.min(visibleDays.length - 1, Math.floor((offset / Math.max(1, rect.width)) * visibleDays.length));
    const day = visibleDays[index]?.day;
    return day ? calendarDayKey(day) : null;
  }

  function startIsoForTimedDrag(dragEvent: DragEvent<HTMLElement>, dayKey: string): string | null {
    const container = dragEvent.currentTarget.closest<HTMLElement>("[data-calendar-day-events]");
    const rect = container?.getBoundingClientRect();

    if (!rect) {
      return null;
    }

    const clientY = Number.isFinite(dragEvent.clientY) ? dragEvent.clientY : rect.top;
    const offset = Math.min(Math.max(clientY - rect.top, 0), Math.max(1, rect.height) - 1);
    const totalMinutes = Math.min(
      23 * 60 + 45,
      Math.max(0, Math.floor(((offset / Math.max(1, rect.height)) * 24 * 60) / 15) * 15)
    );

    return zonedDateTimeIso(
      dayKey,
      Math.floor(totalMinutes / 60),
      totalMinutes % 60,
      source.settings.defaultTimeZone
    );
  }

  function previewAllDayEventDrop(dragEvent: DragEvent<HTMLElement>): void {
    const dayKey = dayKeyForAllDayDrag(dragEvent);

    if (dayKey) {
      previewDrop(dragEvent, dayKey, `${dayKey}T00:00:00.000Z`, true);
    }
  }

  function handleAllDayEventDrop(dragEvent: DragEvent<HTMLElement>): void {
    const dayKey = dayKeyForAllDayDrag(dragEvent);

    if (dayKey) {
      handleDrop(dragEvent, dayKey, `${dayKey}T00:00:00.000Z`, true);
    }
  }

  function previewTimedEventDrop(dragEvent: DragEvent<HTMLElement>, dayKey: string): void {
    const startsAt = startIsoForTimedDrag(dragEvent, dayKey);

    if (startsAt) {
      previewDrop(dragEvent, dayKey, startsAt, false);
    }
  }

  function handleTimedEventDrop(dragEvent: DragEvent<HTMLElement>, dayKey: string): void {
    const startsAt = startIsoForTimedDrag(dragEvent, dayKey);

    if (startsAt) {
      handleDrop(dragEvent, dayKey, startsAt, false);
    }
  }

  type TimeDragEvent = PointerEvent<HTMLElement> | MouseEvent<HTMLElement>;

  function updateTimeDrag(
    pointerEvent: TimeDragEvent,
    dayKey: string,
    hour: number
  ): CalendarTimeBlock | null {
    const drag = timelineDragRef.current;

    if (!drag || (pointerEvent.buttons !== 1 && pointerEvent.type !== "pointerup")) {
      return null;
    }

    pointerEvent.preventDefault();
    const pointerAt = calendarPointerTimeIso(dayKey, hour, pointerEvent, source.settings.defaultTimeZone);
    const nextSelection = calendarTimeBlock(drag.startsAt, pointerAt, source.settings.defaultTimeZone);

    if (
      Math.abs(pointerEvent.clientY - drag.startClientY) > 4 ||
      nextSelection.startsAt !== drag.startsAt ||
      Date.parse(nextSelection.endsAt) - Date.parse(nextSelection.startsAt) > 15 * 60 * 1000
    ) {
      drag.moved = true;
    }

    setDragSelection(nextSelection);
    return nextSelection;
  }

  function handleTimePointerDown(
    pointerEvent: TimeDragEvent,
    dayKey: string,
    hour: number
  ): void {
    if (pointerEvent.button !== 0) {
      return;
    }

    pointerEvent.preventDefault();
    const startsAt = calendarPointerTimeIso(dayKey, hour, pointerEvent, source.settings.defaultTimeZone);
    const initialSelection = calendarTimeBlock(startsAt, startsAt, source.settings.defaultTimeZone);

    timelineDragRef.current = {
      dayKey,
      mode: availabilityMode ? "availability" : "create",
      moved: false,
      startClientY: pointerEvent.clientY,
      startsAt
    };
    setDragSelection(initialSelection);
  }

  function handleTimePointerUp(
    pointerEvent: TimeDragEvent,
    dayKey: string,
    hour: number
  ): void {
    const drag = timelineDragRef.current;

    if (!drag) {
      return;
    }

    const finalSelection = updateTimeDrag(pointerEvent, dayKey, hour) ?? dragSelection;
    timelineDragRef.current = null;
    setDragSelection(null);

    if (!drag.moved || !finalSelection) {
      return;
    }

    suppressNextClickRef.current = true;

    if (drag.mode === "availability") {
      onAddAvailabilitySlot?.(finalSelection);
      return;
    }

    onCreate({
      allDay: false,
      startsAt: finalSelection.startsAt,
      endsAt: finalSelection.endsAt
    });
  }

  return (
    <div className="flex h-full min-h-[680px] flex-col overflow-hidden rounded-hcbMd border border-border bg-bg-secondary">
      <div className="flex min-h-12 items-center justify-between gap-3 border-b border-border bg-bg-primary/40 px-3 py-2">
        <div className="min-w-0">
          <div className="truncate text-[var(--text-md)] font-semibold text-text-primary">{title}</div>
          <div className="truncate text-[var(--text-xs)] text-text-muted">{label}</div>
        </div>
        {dayCountControl}
      </div>
      <div
        className="min-h-0 flex-1 overflow-auto"
        data-calendar-timeline-scroll
        ref={scrollContainerRef}
        role="grid"
        aria-label={gridLabel}
      >
        <div className="min-w-[720px]">
          <div className="grid grid-cols-[64px_minmax(0,1fr)] border-b border-border bg-bg-secondary/80">
            <div className="border-r border-border" aria-hidden="true" />
            <div className="grid" style={{ gridTemplateColumns }}>
              {visibleDays.map(({ day }) => (
                <div
                  className="min-h-16 border-r border-border px-2 py-2 text-center last:border-r-0"
                  key={day.id}
                  role="columnheader"
                >
                  <div className="text-[var(--text-xs)] font-semibold text-text-muted">{day.weekday}</div>
                  <div
                    className={cx(
                      "mx-auto mt-1 flex size-8 items-center justify-center rounded-full text-[var(--text-md)] font-semibold text-text-primary",
                      day.isToday && "bg-accent text-bg-tertiary"
                    )}
                  >
                    {day.dateLabel}
                  </div>
                </div>
              ))}
            </div>
          </div>
          <div
            className="grid grid-cols-[64px_minmax(0,1fr)] border-b border-border"
            style={{ minHeight: allDayRowHeight }}
          >
            <div className="border-r border-border px-2 py-3 text-[var(--text-xs)] font-semibold text-text-muted">
              All-day
            </div>
            <div
              aria-label={`All-day events ${label}`}
              className="relative"
              data-calendar-all-day-events
              role="group"
              style={{ minHeight: allDayRowHeight }}
            >
              <div className="absolute inset-0 grid" style={{ gridTemplateColumns }}>
                {visibleDays.map(({ day }) => {
                  const dayKey = calendarDayKey(day);

                  return (
                    <div
                      className="border-r border-border bg-bg-tertiary last:border-r-0 hover:bg-surface-0 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
                      key={day.id}
                      onClick={() => {
                        if (!availabilityMode) {
                          onCreate({ startsAt: `${dayKey}T00:00:00.000Z`, allDay: true });
                        }
                      }}
                      onDragOver={(dragEvent) =>
                        previewDrop(dragEvent, dayKey, `${dayKey}T00:00:00.000Z`, true)
                      }
                      onDrop={(dragEvent) =>
                        handleDrop(dragEvent, dayKey, `${dayKey}T00:00:00.000Z`, true)
                      }
                      onKeyDown={(event) =>
                        !availabilityMode
                          ? handleActivationKeyDown(event, () =>
                              onCreate({ startsAt: `${dayKey}T00:00:00.000Z`, allDay: true })
                            )
                          : undefined
                      }
                      role="gridcell"
                      tabIndex={0}
                    />
                  );
                })}
              </div>
              <div
                className="pointer-events-none relative grid gap-y-1 py-2"
                style={{
                  gridTemplateColumns,
                  gridTemplateRows: `repeat(${
                    calendarTimelineVisibleAllDayCount + (hasAllDayOverflow ? 1 : 0)
                  }, ${allDayLaneHeight}px)`,
                  minHeight: allDayRowHeight
                }}
              >
                {timeline.allDaySegments.map((segment) => (
                  <div
                    className="pointer-events-auto min-w-0 px-2"
                    data-calendar-all-day-segment={segment.event.id}
                    data-day-span={segment.daySpan}
                    data-ends-after-range={segment.endsAfterRange}
                    data-lane-index={segment.laneIndex}
                    data-start-day-index={segment.startDayIndex}
                    data-starts-before-range={segment.startsBeforeRange}
                    key={segment.event.id}
                    onDragOver={previewAllDayEventDrop}
                    onDrop={handleAllDayEventDrop}
                    style={allDaySegmentStyle(segment)}
                  >
                    <CalendarTimelineEventChip
                      event={segment.event}
                      eventCompletionDefaultScope={eventCompletionDefaultScope}
                      labelVariant="title"
                      onMoveEvent={onMoveEvent}
                      onOpen={onOpen}
                      onToggleEvent={onToggleEvent}
                      onToggleTask={onToggleTask}
                    />
                  </div>
                ))}
                {timeline.allDayOverflowCounts.map((count, dayIndex) => {
                  const visibleDay = visibleDays[dayIndex];
                  const day = visibleDay?.day;
                  const popupEvents = visibleDay?.allDayEvents ?? [];

                  return count > 0 && day ? (
                    <div
                      className="pointer-events-auto min-w-0 px-2"
                      key={`${visibleDays[dayIndex]?.day.id ?? dayIndex}-overflow`}
                      style={{
                        gridColumn: `${dayIndex + 1}`,
                        gridRow: `${calendarTimelineVisibleAllDayCount + 1}`
                      }}
                    >
                      <CalendarOverflowChip
                        count={count}
                        onOpen={() =>
                          setActiveOverflow({
                            events: popupEvents,
                            title: `Items for ${calendarDateTitle(day)}`
                          })
                        }
                      />
                    </div>
                  ) : null;
                })}
              </div>
            </div>
          </div>
          <div className="relative" ref={timeGridRef}>
            {calendarTimelineHours.map((hour) => (
              <div
                aria-label={`${hourSlotLabel(hour)} Open slot`}
                className="grid grid-cols-[64px_minmax(0,1fr)] border-b border-border last:border-b-0"
                key={hour}
                onDragLeave={() => setDropPreview(null)}
                onDragOver={(dragEvent) =>
                  previewDrop(
                    dragEvent,
                    firstVisibleDayKey,
                    hourSlotIso(firstVisibleDayKey, hour, source.settings.defaultTimeZone),
                    false
                  )
                }
                onDrop={(dragEvent) =>
                  handleDrop(
                    dragEvent,
                    firstVisibleDayKey,
                    hourSlotIso(firstVisibleDayKey, hour, source.settings.defaultTimeZone),
                    false
                  )
                }
                role="row"
                style={{ height: hourRowHeight }}
              >
                <div className="border-r border-border px-2 py-2 text-right text-[var(--text-xs)] font-semibold text-text-muted">
                  {calendarDisplayHourLabel(hour)}
                </div>
                <div className="grid" style={{ gridTemplateColumns }}>
                  {visibleDays.map(({ day }) => {
                    const dayKey = calendarDayKey(day);
                    const startsAt = hourSlotIso(dayKey, hour, source.settings.defaultTimeZone);

                    return (
                      <div
                        aria-label={`${calendarDateTitle(day)} ${calendarDisplayHourLabel(hour)}`}
                        className="border-r border-border bg-bg-tertiary px-2 py-1.5 last:border-r-0 hover:bg-surface-0 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
                        key={`${day.id}-${hour}`}
                        onClick={() => {
                          if (suppressNextClickRef.current) {
                            suppressNextClickRef.current = false;
                            return;
                          }

                          if (!availabilityMode) {
                            onCreate({
                              allDay: false,
                              startsAt,
                              endsAt: addUtcMinutesIso(startsAt, 60)
                            });
                          }
                        }}
                        onDragOver={(dragEvent) => previewDrop(dragEvent, dayKey, startsAt, false)}
                        onDrop={(dragEvent) => handleDrop(dragEvent, dayKey, startsAt, false)}
                        onMouseDown={(mouseEvent) => handleTimePointerDown(mouseEvent, dayKey, hour)}
                        onMouseMove={(mouseEvent) => updateTimeDrag(mouseEvent, dayKey, hour)}
                        onMouseUp={(mouseEvent) => handleTimePointerUp(mouseEvent, dayKey, hour)}
                        onPointerDown={(pointerEvent) => handleTimePointerDown(pointerEvent, dayKey, hour)}
                        onPointerEnter={(pointerEvent) => {
                          if (timelineDragRef.current) {
                            updateTimeDrag(pointerEvent, dayKey, hour);
                          }
                        }}
                        onPointerMove={(pointerEvent) => updateTimeDrag(pointerEvent, dayKey, hour)}
                        onPointerUp={(pointerEvent) => handleTimePointerUp(pointerEvent, dayKey, hour)}
                        onKeyDown={(event) =>
                          !availabilityMode
                            ? handleActivationKeyDown(event, () =>
                                onCreate({
                                  allDay: false,
                                  startsAt,
                                  endsAt: addUtcMinutesIso(startsAt, 60)
                                })
                              )
                            : undefined
                        }
                        role="gridcell"
                        tabIndex={0}
                      >
                        {!availabilityMode ? (
                          <button
                            aria-label={`Create event at ${hourSlotLabel(hour)}`}
                            className="sr-only"
                            onClick={(buttonEvent) => {
                              buttonEvent.stopPropagation();
                              onCreate({
                                allDay: false,
                                startsAt,
                                endsAt: addUtcMinutesIso(startsAt, 60)
                              });
                            }}
                            type="button"
                          />
                        ) : null}
                      </div>
                    );
                  })}
                </div>
              </div>
            ))}
            <div
              className="pointer-events-none absolute bottom-0 left-16 right-0 top-0 grid"
              style={{ gridTemplateColumns }}
            >
              {visibleDays.map(({ day }) => (
                <div className="relative min-w-0 border-r border-transparent last:border-r-0" key={`${day.id}-previews`}>
                  {previewSegments
                    .filter((segment) => segment.dayId === day.id)
                    .map((segment) => (
                      <div
                        className="absolute left-2 right-2 rounded-hcbSm border border-info bg-info/15"
                        data-calendar-drop-preview
                        key={segment.id}
                        style={{ height: segment.height, top: segment.top }}
                      />
                    ))}
                </div>
              ))}
            </div>
            <div
              className="pointer-events-none absolute bottom-0 left-16 right-0 top-0 grid"
              style={{ gridTemplateColumns }}
            >
              {visibleDays.map(({ day, timedEventLayouts }) => (
                <div
                  className="relative min-w-0 border-r border-transparent last:border-r-0"
                  data-calendar-day-events
                  key={`${day.id}-events`}
                >
                  {timedEventLayouts.map((layout) => (
                    <div
                      className="pointer-events-auto absolute min-w-0"
                      data-calendar-event-layout={layout.event.id}
                      data-duration-minutes={layout.durationMinutes}
                      data-lane-count={layout.laneCount}
                      data-lane-index={layout.laneIndex}
                      data-start-minute={layout.startMinute}
                      key={layout.event.id}
                      onDragOver={(dragEvent) => previewTimedEventDrop(dragEvent, calendarDayKey(day))}
                      onDrop={(dragEvent) => handleTimedEventDrop(dragEvent, calendarDayKey(day))}
                      style={timelineEventStyle(layout)}
                    >
                      <CalendarTimelineEventChip
                        className="min-w-0"
                        event={layout.event}
                        eventCompletionDefaultScope={eventCompletionDefaultScope}
                        labelVariant={timedLabelVariant}
                        onMoveEvent={onMoveEvent}
                        onOpen={onOpen}
                        onToggleEvent={onToggleEvent}
                        onToggleTask={onToggleTask}
                      />
                    </div>
                  ))}
                </div>
              ))}
            </div>
            {currentDayIndex >= 0 ? (
              <div
                aria-hidden="true"
                className="pointer-events-none absolute bottom-0 left-16 right-0 top-0 z-30 grid"
                style={{ gridTemplateColumns }}
              >
                {visibleDays.map(({ day }, dayIndex) => (
                  <div className="relative min-w-0" key={`${day.id}-now-line`}>
                    {dayIndex === currentDayIndex ? (
                      <div
                        className="absolute left-0 right-0"
                        data-calendar-now-line
                        data-calendar-now-minute={currentPoint.minutes}
                        style={{ top: currentTimeTop }}
                      >
                        <span className="absolute -left-1.5 -top-1 block size-2.5 rounded-full bg-danger" />
                        <span className="block h-px bg-danger" />
                      </div>
                    ) : null}
                  </div>
                ))}
              </div>
            ) : null}
          </div>
        </div>
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

export function DayView({
  availabilityMode,
  availabilitySlots,
  day,
  eventCompletionDefaultScope,
  onAddAvailabilitySlot,
  onCreate,
  onMoveEvent,
  onOpen,
  onResizeEvent,
  onToggleEvent,
  onToggleTask,
  visibleCalendarIds
}: {
  availabilityMode: boolean;
  availabilitySlots: CalendarTimeBlock[];
  day: CalendarDayViewModel;
  eventCompletionDefaultScope?: EventCompletionDefaultScope;
  onAddAvailabilitySlot: (slot: CalendarTimeBlock) => void;
  onCreate: (seed?: CalendarCreateSeed) => void;
  onMoveEvent: (eventId: string, startsAt: string, allDay: boolean) => void;
  onOpen: (event: CalendarEventViewModel) => void;
  onResizeEvent: (eventId: string, endsAt: string) => void;
  onToggleEvent?: (eventId: string, scope?: CalendarEventCompletionScope) => void;
  onToggleTask?: (taskId: string) => void;
  visibleCalendarIds: ReadonlySet<string>;
}): JSX.Element {
  return (
    <CalendarTimelineView
      availabilityMode={availabilityMode}
      availabilitySlots={availabilitySlots}
      days={[day]}
      eventCompletionDefaultScope={eventCompletionDefaultScope}
      gridLabel="Calendar day view"
      label={calendarDateTitle(day)}
      onAddAvailabilitySlot={onAddAvailabilitySlot}
      onCreate={onCreate}
      onMoveEvent={onMoveEvent}
      onOpen={onOpen}
      onResizeEvent={onResizeEvent}
      onToggleEvent={onToggleEvent}
      onToggleTask={onToggleTask}
      timedLabelVariant="range"
      title="Day view"
      visibleCalendarIds={visibleCalendarIds}
    />
  );
}

export function MultiDayView({
  availabilityMode,
  availabilitySlots,
  dayCount,
  days,
  eventCompletionDefaultScope,
  onAddAvailabilitySlot,
  onCreate,
  onDayCountChange,
  onMoveEvent,
  onOpen,
  onResizeEvent,
  onToggleEvent,
  onToggleTask,
  visibleCalendarIds
}: {
  availabilityMode: boolean;
  availabilitySlots: CalendarTimeBlock[];
  dayCount: number;
  days: CalendarDayViewModel[];
  eventCompletionDefaultScope?: EventCompletionDefaultScope;
  onAddAvailabilitySlot: (slot: CalendarTimeBlock) => void;
  onCreate: (seed?: CalendarCreateSeed) => void;
  onDayCountChange: (dayCount: number) => void;
  onMoveEvent: (eventId: string, startsAt: string, allDay: boolean) => void;
  onOpen: (event: CalendarEventViewModel) => void;
  onResizeEvent: (eventId: string, endsAt: string) => void;
  onToggleEvent?: (eventId: string, scope?: CalendarEventCompletionScope) => void;
  onToggleTask?: (taskId: string) => void;
  visibleCalendarIds: ReadonlySet<string>;
}): JSX.Element {
  return (
    <CalendarTimelineView
      availabilityMode={availabilityMode}
      availabilitySlots={availabilitySlots}
      dayCountControl={
        <div className="flex shrink-0 items-center gap-2">
          <IconButton
            disabled={dayCount <= 2}
            icon={Minus}
            label="Show fewer days"
            onClick={() => onDayCountChange(Math.max(2, dayCount - 1))}
            size="sm"
            variant="ghost"
          />
          <span className="min-w-14 text-center text-[var(--text-sm)] font-semibold text-text-primary">
            {dayCount} days
          </span>
          <IconButton
            disabled={dayCount >= 6}
            icon={Plus}
            label="Show more days"
            onClick={() => onDayCountChange(Math.min(6, dayCount + 1))}
            size="sm"
            variant="ghost"
          />
        </div>
      }
      days={days}
      eventCompletionDefaultScope={eventCompletionDefaultScope}
      gridLabel="Calendar multi-day view"
      label={calendarRangeTitle(days)}
      onAddAvailabilitySlot={onAddAvailabilitySlot}
      onCreate={onCreate}
      onMoveEvent={onMoveEvent}
      onOpen={onOpen}
      onResizeEvent={onResizeEvent}
      onToggleEvent={onToggleEvent}
      onToggleTask={onToggleTask}
      title="Multi-Day view"
      visibleCalendarIds={visibleCalendarIds}
    />
  );
}

export function WeekView({
  availabilityMode,
  availabilitySlots,
  days,
  eventCompletionDefaultScope,
  onAddAvailabilitySlot,
  onCreate,
  onMoveEvent,
  onOpen,
  onResizeEvent,
  onToggleEvent,
  onToggleTask,
  visibleCalendarIds
}: {
  availabilityMode: boolean;
  availabilitySlots: CalendarTimeBlock[];
  days: CalendarDayViewModel[];
  eventCompletionDefaultScope?: EventCompletionDefaultScope;
  onAddAvailabilitySlot: (slot: CalendarTimeBlock) => void;
  onCreate: (seed?: CalendarCreateSeed) => void;
  onMoveEvent: (eventId: string, startsAt: string, allDay: boolean) => void;
  onOpen: (event: CalendarEventViewModel) => void;
  onResizeEvent: (eventId: string, endsAt: string) => void;
  onToggleEvent?: (eventId: string, scope?: CalendarEventCompletionScope) => void;
  onToggleTask?: (taskId: string) => void;
  visibleCalendarIds: ReadonlySet<string>;
}): JSX.Element {
  return (
    <CalendarTimelineView
      availabilityMode={availabilityMode}
      availabilitySlots={availabilitySlots}
      days={days}
      eventCompletionDefaultScope={eventCompletionDefaultScope}
      gridLabel="Calendar week view"
      label={calendarRangeTitle(days)}
      onAddAvailabilitySlot={onAddAvailabilitySlot}
      onCreate={onCreate}
      onMoveEvent={onMoveEvent}
      onOpen={onOpen}
      onResizeEvent={onResizeEvent}
      onToggleEvent={onToggleEvent}
      onToggleTask={onToggleTask}
      title="Week view"
      visibleCalendarIds={visibleCalendarIds}
    />
  );
}
