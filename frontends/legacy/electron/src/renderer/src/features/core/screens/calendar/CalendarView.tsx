import { useEffect, useMemo, useRef, useState } from "react";
import type { CalendarEventCompletionScope, CalendarEventDetail } from "@shared/ipc/contracts";
import { AlertTriangle, X } from "lucide-react";
import { IconButton, StatusBanner } from "../../../../components/primitives";
import { rendererNow, reportRendererTimingSince } from "../../../../hooks/useRenderTiming";
import { stableCalendarEventViewModel } from "../../viewModelSource/calendarViewModels";
import { useCoreViewModelSource } from "../../coreViewModelSource";
import type { ConvertSourceCleanup } from "../../conversionEvents";
import type { CalendarDayViewModel, CalendarEventViewModel, CalendarViewId } from "../../coreViewModels";
import {
  CacheStatePanel,
  SectionChrome,
  scheduleRendererFrame
} from "../../coreScreenShared";
import { CalendarAgendaView } from "./CalendarAgendaView";
import { CalendarHeader } from "./CalendarHeader";
import { ShareAvailabilityPanel, SmartReschedulePanel } from "./CalendarSidebar";
import { DayView, MultiDayView, WeekView } from "./CalendarTimelineView";
import { MonthView } from "./MonthView";
import {
  buildCalendarEventDayIndex,
  calendarAddUtcDays,
  calendarAddUtcMonths,
  calendarCurrentDayKey,
  calendarDateFromIsoDate,
  calendarDateTitleFromIso,
  calendarDayViewForDate,
  calendarDayKey,
  calendarEventsForDay,
  calendarMonthOffset,
  calendarMonthTitle,
  calendarMonthWeeksForDate,
  calendarRangeDaysForDate,
  calendarRangeTitle,
  calendarWeekDaysForDate,
  visibleCalendarEvent
} from "./calendarGrid";
import { useCalendarAvailability } from "./useCalendarAvailability";
import { useCalendarEventInspector } from "./useCalendarEventInspector";
import { useTaskInspector } from "../tasks/useTaskInspector";
import type { CalendarCreateMode, CalendarCreateSeed, CalendarEventDraft } from "./types";

function calendarDayRange(day: string): { start: string; end: string } {
  return {
    start: `${day}T00:00:00.000Z`,
    end: `${calendarAddUtcDays(day, 1)}T00:00:00.000Z`
  };
}

function calendarRangeForDays(days: readonly CalendarDayViewModel[]): { start: string; end: string } {
  const dayKeys = days.map(calendarDayKey).sort();
  const firstDay = dayKeys[0];
  const lastDay = dayKeys.at(-1);

  if (!firstDay || !lastDay) {
    return calendarDayRange(calendarCurrentDayKey());
  }

  return {
    start: `${firstDay}T00:00:00.000Z`,
    end: `${calendarAddUtcDays(lastDay, 1)}T00:00:00.000Z`
  };
}

function calendarMonthRange(anchorDay: string): { start: string; end: string } {
  const anchor = calendarDateFromIsoDate(anchorDay);
  const first = new Date(Date.UTC(anchor.getUTCFullYear(), anchor.getUTCMonth(), 1));
  const gridStart = new Date(first);
  gridStart.setUTCDate(first.getUTCDate() - first.getUTCDay());
  const gridEnd = new Date(gridStart);
  gridEnd.setUTCDate(gridStart.getUTCDate() + 42);

  return {
    start: gridStart.toISOString(),
    end: gridEnd.toISOString()
  };
}

function calendarRangeAllowedByRetention(
  range: { start: string; end: string },
  daysBack: number,
  timeZone: string
): boolean {
  if (daysBack <= 0) {
    return true;
  }

  const oldestDay = calendarAddUtcDays(calendarCurrentDayKey(timeZone), -daysBack);
  return range.end > `${oldestDay}T00:00:00.000Z`;
}

function eventViewModelFromDetail(
  detail: CalendarEventDetail,
  source: ReturnType<typeof useCoreViewModelSource>
): CalendarEventViewModel {
  const calendar = source.calendarSources.find((candidate) => candidate.id === detail.calendarId);

  return stableCalendarEventViewModel(
    detail,
    calendar?.title ?? detail.calendarTitle,
    calendar?.timeZone ?? null,
    calendar?.backgroundColor ?? null,
    calendar?.foregroundColor ?? null,
    source.settings.calendarEventColorOverrides,
    source.activeColorTheme,
    source.settings.defaultTimeZone,
    new Map()
  );
}

function findCalendarEventBySearchId(
  eventsById: Record<string, CalendarEventViewModel>,
  eventId: string
): CalendarEventViewModel | undefined {
  return eventsById[eventId] ?? Object.values(eventsById).find((event) => event.eventId === eventId);
}

export function CalendarView({
  visibleCalendarIds
}: {
  visibleCalendarIds: ReadonlySet<string>;
}): JSX.Element {
  const source = useCoreViewModelSource();
  const [activeViewId, setActiveViewId] = useState<CalendarViewId>("agenda");
  const [calendarAnchorDate, setCalendarAnchorDate] = useState(() =>
    calendarCurrentDayKey(source.settings.defaultTimeZone)
  );
  const [multiDayCount, setMultiDayCount] = useState(3);
  const [smartRescheduleOpen, setSmartRescheduleOpen] = useState(false);
  const calendarNavigationStartedAt = useRef<number | null>(null);
  const {
    addAvailabilitySlot,
    availabilityBusyBlockCount,
    availabilityCalendarId,
    availabilityDurationMinutes,
    availabilityEndDate,
    availabilityError,
    availabilityHoldPending,
    availabilityPending,
    availabilitySlots,
    availabilitySnippet,
    availabilityStartDate,
    availabilityText,
    availabilityTitle,
    copyAvailabilitySnippet,
    createAvailabilityHolds,
    exportAvailability,
    removeAvailabilitySlot,
    setAvailabilityCalendarId,
    setAvailabilityDurationMinutes,
    setAvailabilityEndDate,
    setAvailabilityStartDate,
    setAvailabilityTitle,
    setShareAvailabilityOpen,
    shareAvailabilityOpen
  } = useCalendarAvailability(source);
  const {
    calendarActionError,
    moveCalendarEvent,
    openCreate,
    openEdit,
    resizeCalendarEvent,
    setCalendarActionError
  } = useCalendarEventInspector(source);
  const taskInspector = useTaskInspector(source);
  const visibleCalendarEvents = useMemo(
    () =>
      source.calendarAgendaEvents.filter((event) =>
        visibleCalendarEvent(event, visibleCalendarIds)
      ),
    [source.calendarAgendaEvents, visibleCalendarIds]
  );
  const visibleCalendarViewIds = useMemo(() => {
    const hidden = new Set(source.settings.hiddenCalendarViewModes);
    const visible = (["agenda", "day", "multiDay", "week", "month"] as CalendarViewId[]).filter(
      (viewId) => !hidden.has(viewId)
    );

    return visible.length > 0 ? visible : (["month"] as CalendarViewId[]);
  }, [source.settings.hiddenCalendarViewModes]);
  const visibleEventDayIndex = useMemo(
    () => buildCalendarEventDayIndex(visibleCalendarEvents),
    [visibleCalendarEvents]
  );
  const calendarDay = useMemo(
    () => calendarDayViewForDate(visibleEventDayIndex, calendarAnchorDate, "day"),
    [calendarAnchorDate, visibleEventDayIndex]
  );
  const calendarWeekDays = useMemo(
    () => calendarWeekDaysForDate(visibleEventDayIndex, calendarAnchorDate),
    [calendarAnchorDate, visibleEventDayIndex]
  );
  const calendarMultiDayDays = useMemo(
    () => calendarRangeDaysForDate(visibleEventDayIndex, calendarAnchorDate, multiDayCount),
    [calendarAnchorDate, multiDayCount, visibleEventDayIndex]
  );
  const calendarMonthWeeks = useMemo(
    () => calendarMonthWeeksForDate(visibleEventDayIndex, calendarAnchorDate),
    [calendarAnchorDate, visibleEventDayIndex]
  );
  const calendarAgendaEvents = useMemo(
    () => calendarEventsForDay(visibleEventDayIndex, calendarAnchorDate),
    [calendarAnchorDate, visibleEventDayIndex]
  );
  const calendarVisibleRange = useMemo(() => {
    if (activeViewId === "month") {
      return calendarRangeForDays(calendarMonthWeeks.flatMap((week) => week.days));
    }

    if (activeViewId === "week") {
      return calendarRangeForDays(calendarWeekDays);
    }

    if (activeViewId === "multiDay") {
      return calendarRangeForDays(calendarMultiDayDays);
    }

    return calendarDayRange(calendarAnchorDate);
  }, [activeViewId, calendarAnchorDate, calendarMonthWeeks, calendarMultiDayDays, calendarWeekDays]);
  const shareAvailabilityVisible = activeViewId === "day" || activeViewId === "multiDay" || activeViewId === "week";
  const calendarRangeLabel =
    activeViewId === "month"
      ? calendarMonthTitle(calendarAnchorDate)
      : activeViewId === "week"
        ? calendarRangeTitle(calendarWeekDays)
        : activeViewId === "multiDay"
          ? calendarRangeTitle(calendarMultiDayDays)
          : calendarDateTitleFromIso(calendarAnchorDate);
  const previousRangeLabel =
    activeViewId === "month"
      ? "Previous month"
      : activeViewId === "week"
        ? "Previous week"
        : activeViewId === "multiDay"
          ? `Previous ${multiDayCount} days`
          : "Previous day";
  const nextRangeLabel =
    activeViewId === "month"
      ? "Next month"
      : activeViewId === "week"
        ? "Next week"
        : activeViewId === "multiDay"
          ? `Next ${multiDayCount} days`
          : "Next day";

  function setCalendarView(viewId: CalendarViewId): void {
    if (!visibleCalendarViewIds.includes(viewId)) {
      return;
    }

    calendarNavigationStartedAt.current = rendererNow();
    setActiveViewId(viewId);
    if (viewId === "month") {
      setCalendarAnchorDate(calendarCurrentDayKey(source.settings.defaultTimeZone));
    }
  }

  function shiftCalendarAnchor(direction: -1 | 1): void {
    calendarNavigationStartedAt.current = rendererNow();
    setCalendarAnchorDate((current) => {
      if (activeViewId === "month") {
        const next = calendarAddUtcMonths(current, direction);
        const nextRange = calendarMonthRange(next);
        const offset = calendarMonthOffset(calendarCurrentDayKey(source.settings.defaultTimeZone), next);

        if (
          !calendarRangeAllowedByRetention(
            nextRange,
            source.settings.eventRetentionDaysBack,
            source.settings.defaultTimeZone
          ) ||
          offset > source.settings.monthScrollFutureMonths
        ) {
          return current;
        }

        return next;
      }

      if (activeViewId === "week") {
        return calendarAddUtcDays(current, direction * 7);
      }

      if (activeViewId === "multiDay") {
        return calendarAddUtcDays(current, direction * multiDayCount);
      }

      return calendarAddUtcDays(current, direction);
    });
  }

  function resetCalendarAnchor(): void {
    calendarNavigationStartedAt.current = rendererNow();
    setCalendarAnchorDate(calendarCurrentDayKey(source.settings.defaultTimeZone));
  }

  function openCalendarItem(calendarItem: typeof visibleCalendarEvents[number]): void {
    if (calendarItem.sourceKind === "task" && calendarItem.taskId) {
      taskInspector.selectTask(calendarItem.taskId);
      return;
    }

    openEdit(calendarItem);
  }

  async function openCalendarSearchResult(eventId: string): Promise<void> {
    const loadedEvent = findCalendarEventBySearchId(source.calendarEventsById, eventId);

    if (loadedEvent) {
      setCalendarAnchorDate(loadedEvent.startsAt.slice(0, 10));
      openEdit(loadedEvent);
      return;
    }

    const result = await window.hcb?.calendar.get({ id: eventId });

    if (!result?.ok) {
      setCalendarActionError("Calendar event could not be opened from search.");
      return;
    }

    const eventDate = result.data.startsAt.slice(0, 10);
    const loaded = await source.ensureCalendarRange(calendarDayRange(eventDate));

    if (!loaded) {
      setCalendarActionError("Calendar event context could not be loaded.");
      return;
    }

    setCalendarAnchorDate(eventDate);
    openEdit(eventViewModelFromDetail(result.data, source));
  }

  function toggleCalendarTask(taskId: string): void {
    void taskInspector.toggleTask(taskId);
  }

  function toggleCalendarEvent(eventId: string, scope?: CalendarEventCompletionScope): void {
    const event = source.calendarEventsById[eventId];

    if (event?.completedAt) {
      void source.reopenEvent(eventId, scope);
      return;
    }

    void source.completeEvent(eventId, scope);
  }

  useEffect(() => {
    if (!visibleCalendarViewIds.includes(activeViewId)) {
      setCalendarView(visibleCalendarViewIds[0]);
    }
  }, [activeViewId, visibleCalendarViewIds]);

  useEffect(() => {
    function handleCalendarCommand(event: Event): void {
      const detail = (event as CustomEvent<{
        action: string;
        createMode?: CalendarCreateMode;
        cleanup?: ConvertSourceCleanup;
        draft?: Partial<CalendarEventDraft>;
        eventId?: string;
        seed?: CalendarCreateSeed;
        viewId?: CalendarViewId;
      }>).detail;

      if (detail?.action === "new-event") {
        openCreate(detail.seed);
      }

      if (detail?.action === "quick-add") {
        openCreate(undefined, {
          createMode: detail.createMode,
          draft: detail.draft
        });
      }

      if (detail?.action === "convert-to-event") {
        openCreate(undefined, {
          cleanup: detail.cleanup,
          createMode: "event",
          draft: detail.draft
        });
      }

      if (detail?.action === "open-event" && detail.eventId) {
        void openCalendarSearchResult(detail.eventId);
      }

      if (detail?.action === "set-view" && detail.viewId) {
        setCalendarView(detail.viewId);
      }
    }

    window.addEventListener("hcb:calendar-command", handleCalendarCommand);
    return () => window.removeEventListener("hcb:calendar-command", handleCalendarCommand);
  }, [source]);

  useEffect(() => {
    let cancelled = false;

    if (
      ((source.dataState === "loading" ||
        source.dataState === "offline" ||
        source.dataState === "error") &&
        !source.hasCachedData) ||
      !calendarRangeAllowedByRetention(
        calendarVisibleRange,
        source.settings.eventRetentionDaysBack,
        source.settings.defaultTimeZone
      )
    ) {
      return;
    }

    void source.ensureCalendarRange(calendarVisibleRange).then((loaded) => {
      if (!loaded && !cancelled) {
        setCalendarActionError("Calendar range could not be loaded.");
      }
    });

    return () => {
      cancelled = true;
    };
  }, [
    calendarVisibleRange.end,
    calendarVisibleRange.start,
    source.ensureCalendarRange,
    source.settings.defaultTimeZone,
    source.settings.eventRetentionDaysBack
  ]);

  useEffect(() => {
    scheduleRendererFrame(() => {
      reportRendererTimingSince("calendar.navigate", calendarNavigationStartedAt.current, {
        anchor: calendarAnchorDate,
        view: activeViewId,
        eventCount: visibleCalendarEvents.length
      });
      calendarNavigationStartedAt.current = null;
    });
  }, [activeViewId, calendarAnchorDate, visibleCalendarEvents.length]);

  if (
    (source.dataState === "loading" ||
      source.dataState === "offline" ||
      source.dataState === "error") &&
    !source.hasCachedData
  ) {
    return <CacheStatePanel title="Calendar" />;
  }

  const availabilityMode = shareAvailabilityVisible && shareAvailabilityOpen;

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <CalendarHeader
        activeViewId={activeViewId}
        calendarRangeLabel={calendarRangeLabel}
        nextRangeLabel={nextRangeLabel}
        onCreate={() => openCreate()}
        onResetRange={resetCalendarAnchor}
        onSetView={setCalendarView}
        onShiftRange={shiftCalendarAnchor}
        onToggleShareAvailability={() => {
          setSmartRescheduleOpen(false);
          setShareAvailabilityOpen((open) => !open);
        }}
        onToggleSmartReschedule={() => {
          setShareAvailabilityOpen(false);
          setSmartRescheduleOpen((open) => !open);
        }}
        previousRangeLabel={previousRangeLabel}
        shareAvailabilityOpen={shareAvailabilityOpen}
        shareAvailabilityVisible={shareAvailabilityVisible}
        smartRescheduleOpen={smartRescheduleOpen}
        visibleCalendarViewIds={visibleCalendarViewIds}
      />

      {calendarActionError ? (
        <StatusBanner
          action={
            <IconButton
              icon={X}
              label="Dismiss calendar interaction error"
              onClick={() => setCalendarActionError(undefined)}
              variant="ghost"
            />
          }
          description={calendarActionError}
          icon={AlertTriangle}
          title="Calendar interaction not saved"
          tone="warning"
        />
      ) : null}

      <SectionChrome
        title="Calendar"
        sidebar={
          smartRescheduleOpen ? (
            <SmartReschedulePanel
              calendars={source.calendarSources}
              defaultTimeZone={source.settings.defaultTimeZone}
              initialDate={calendarAnchorDate}
              onApplied={() => source.refresh()}
              onClose={() => setSmartRescheduleOpen(false)}
            />
          ) : shareAvailabilityVisible && shareAvailabilityOpen ? (
            <ShareAvailabilityPanel
              calendarId={availabilityCalendarId}
              calendars={source.calendarSources}
              durationMinutes={availabilityDurationMinutes}
              error={availabilityError}
              onCalendarChange={setAvailabilityCalendarId}
              onClose={() => setShareAvailabilityOpen(false)}
              onCopySnippet={copyAvailabilitySnippet}
              onCreateHolds={() => void createAvailabilityHolds()}
              onDurationChange={setAvailabilityDurationMinutes}
              onEndDateChange={setAvailabilityEndDate}
              onExportAvailability={() => void exportAvailability()}
              onRemoveSlot={removeAvailabilitySlot}
              onStartDateChange={setAvailabilityStartDate}
              onTitleChange={setAvailabilityTitle}
              pending={availabilityHoldPending}
              exportBusyBlockCount={availabilityBusyBlockCount}
              exportPending={availabilityPending}
              exportText={availabilityText}
              slots={availabilitySlots}
              snippet={availabilitySnippet}
              startDate={availabilityStartDate}
              timeZone={source.settings.defaultTimeZone}
              title={availabilityTitle}
              endDate={availabilityEndDate}
            />
          ) : undefined
        }
      >
        <div className="h-full min-h-0 overflow-auto pr-1">
          {activeViewId === "agenda" ? (
            <CalendarAgendaView
              eventCompletionDefaultScope={source.settings.eventCompletionDefaultScope}
              events={calendarAgendaEvents}
              label={calendarDateTitleFromIso(calendarAnchorDate)}
              onOpen={openCalendarItem}
              onToggleEvent={toggleCalendarEvent}
              onToggleTask={toggleCalendarTask}
            />
          ) : null}
          {activeViewId === "day" ? (
            <DayView
              availabilityMode={availabilityMode}
              availabilitySlots={availabilitySlots}
              day={calendarDay}
              eventCompletionDefaultScope={source.settings.eventCompletionDefaultScope}
              onAddAvailabilitySlot={addAvailabilitySlot}
              onCreate={openCreate}
              onMoveEvent={moveCalendarEvent}
              onOpen={openCalendarItem}
              onResizeEvent={resizeCalendarEvent}
              onToggleEvent={toggleCalendarEvent}
              onToggleTask={toggleCalendarTask}
              visibleCalendarIds={visibleCalendarIds}
            />
          ) : null}
          {activeViewId === "multiDay" ? (
            <MultiDayView
              availabilityMode={availabilityMode}
              availabilitySlots={availabilitySlots}
              dayCount={multiDayCount}
              days={calendarMultiDayDays}
              eventCompletionDefaultScope={source.settings.eventCompletionDefaultScope}
              onAddAvailabilitySlot={addAvailabilitySlot}
              onCreate={openCreate}
              onDayCountChange={setMultiDayCount}
              onMoveEvent={moveCalendarEvent}
              onOpen={openCalendarItem}
              onResizeEvent={resizeCalendarEvent}
              onToggleEvent={toggleCalendarEvent}
              onToggleTask={toggleCalendarTask}
              visibleCalendarIds={visibleCalendarIds}
            />
          ) : null}
          {activeViewId === "week" ? (
            <WeekView
              availabilityMode={availabilityMode}
              availabilitySlots={availabilitySlots}
              days={calendarWeekDays}
              eventCompletionDefaultScope={source.settings.eventCompletionDefaultScope}
              onAddAvailabilitySlot={addAvailabilitySlot}
              onCreate={openCreate}
              onMoveEvent={moveCalendarEvent}
              onOpen={openCalendarItem}
              onResizeEvent={resizeCalendarEvent}
              onToggleEvent={toggleCalendarEvent}
              onToggleTask={toggleCalendarTask}
              visibleCalendarIds={visibleCalendarIds}
            />
          ) : null}
          {activeViewId === "month" ? (
            <MonthView
              eventCompletionDefaultScope={source.settings.eventCompletionDefaultScope}
              weeks={calendarMonthWeeks}
              onCreate={openCreate}
              onOpen={openCalendarItem}
              onToggleEvent={toggleCalendarEvent}
              onToggleTask={toggleCalendarTask}
              todayKey={calendarCurrentDayKey(source.settings.defaultTimeZone)}
              visibleCalendarIds={visibleCalendarIds}
            />
          ) : null}
        </div>
      </SectionChrome>
    </div>
  );
}
