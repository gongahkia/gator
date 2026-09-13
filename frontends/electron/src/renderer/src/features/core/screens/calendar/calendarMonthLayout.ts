import type { CalendarMonthWeekViewModel } from "../../coreViewModels";
import type { CalendarTimelineAllDaySegment, VisibleCalendarMonthWeek } from "./types";
import { calendarMonthVisibleChipCount } from "./calendarConstants";
import { calendarAllDayLayout } from "./calendarTimelineLayout";
import { visibleCalendarEvent } from "./calendarVisibility";

export function visibleCalendarMonthWeeks(
  weeks: CalendarMonthWeekViewModel[],
  visibleCalendarIds: ReadonlySet<string>
): VisibleCalendarMonthWeek[] {
  return weeks.map((week) => {
    const allDayLayout = calendarAllDayLayout(
      week.days,
      visibleCalendarIds,
      calendarMonthVisibleChipCount
    );

    return {
      allDaySegments: allDayLayout.segments,
      id: week.id,
      days: week.days.map((day, dayIndex) => {
        const visibleEvents = day.events.filter((event) => visibleCalendarEvent(event, visibleCalendarIds));
        const timedEvents = visibleEvents.filter((event) => !event.allDay);
        const occupiedAllDayLanes = monthAllDayLaneIndexesForDay(allDayLayout.segments, dayIndex);
        const availableLaneIndexes = Array.from(
          { length: calendarMonthVisibleChipCount },
          (_, laneIndex) => laneIndex
        ).filter((laneIndex) => !occupiedAllDayLanes.has(laneIndex));
        const visibleTimedEvents = timedEvents.slice(0, availableLaneIndexes.length);
        const overflowEvents = [
          ...(allDayLayout.overflowEvents[dayIndex] ?? []),
          ...timedEvents.slice(visibleTimedEvents.length)
        ];
        const visibleEventChips = visibleTimedEvents.map((event, index) => ({
          event,
          laneIndex: availableLaneIndexes[index] ?? 0
        }));

        return {
          day,
          overflowCount: overflowEvents.length,
          overflowEvents,
          popupEvents: visibleEvents,
          visibleEventChips
        };
      })
    };
  });
}

function monthAllDayLaneIndexesForDay(
  segments: CalendarTimelineAllDaySegment[],
  dayIndex: number
): Set<number> {
  const laneIndexes = new Set<number>();

  for (const segment of segments) {
    const segmentEndDayIndex = segment.startDayIndex + segment.daySpan - 1;

    if (dayIndex >= segment.startDayIndex && dayIndex <= segmentEndDayIndex) {
      laneIndexes.add(segment.laneIndex);
    }
  }

  return laneIndexes;
}
