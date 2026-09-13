export const dayPlanningHours = Array.from({ length: 12 }, (_, index) => index + 7);
export const calendarTimelineHours = Array.from({ length: 24 }, (_, index) => index);
export const calendarDaySlotRowHeight = 64;
export const calendarDayViewportHeight = 520;
export const calendarWeekColumnWidth = 160;
export const calendarMonthVisibleChipCount = 3;
export const calendarWeekVisibleTimedCount = 4;
export const calendarWeekVisibleAllDayCount = 2;
export const calendarTimelineVisibleAllDayCount = 4;
export const calendarTimelineHourRowHeight = 96;
export const calendarTimelineHourRowHeights = {
  compact: 64,
  comfortable: 80,
  spacious: 112
} as const;

export type CalendarTimelineDensity = keyof typeof calendarTimelineHourRowHeights;

export function calendarTimelineHourHeight(density: string): number {
  return calendarTimelineHourRowHeights[density as CalendarTimelineDensity] ?? calendarTimelineHourRowHeights.compact;
}
