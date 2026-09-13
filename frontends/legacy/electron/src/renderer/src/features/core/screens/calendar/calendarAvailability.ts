import type { MouseEvent, PointerEvent } from "react";
import type { CalendarTimeBlock } from "./types";
import {
  addUtcMinutesIso,
  calendarLocalPoint,
  hourSlotIso,
  zonedDateTimeIso
} from "./calendarDateUtils";

export function calendarPointerTimeIso(
  dayKey: string,
  hour: number,
  event: PointerEvent<HTMLElement> | MouseEvent<HTMLElement>,
  timeZone = "UTC"
): string {
  const rect = event.currentTarget.getBoundingClientRect();
  const offset = Math.min(Math.max(event.clientY - rect.top, 0), Math.max(1, rect.height) - 1);
  const quarter = Math.min(3, Math.max(0, Math.floor((offset / Math.max(1, rect.height)) * 4)));
  const minutes = quarter * 15;

  return zonedDateTimeIso(dayKey, hour, minutes, timeZone);
}

export function calendarTimeBlock(startsAt: string, pointerAt: string, timeZone = "UTC"): CalendarTimeBlock {
  const startMs = Date.parse(startsAt);
  const pointerMs = Date.parse(pointerAt);
  const pointerEnd = addUtcMinutesIso(pointerAt, 15);
  const starts = pointerMs < startMs ? pointerAt : startsAt;
  const ends = pointerMs < startMs ? addUtcMinutesIso(startsAt, 15) : pointerEnd;

  return {
    id: `${starts}-${ends}`,
    dayKey: calendarLocalPoint(starts, timeZone).dayKey,
    startsAt: starts,
    endsAt: ends
  };
}

export function calendarBlocksOverlapHour(
  blocks: CalendarTimeBlock[],
  dayKey: string,
  hour: number,
  timeZone = "UTC"
): boolean {
  const startsAt = Date.parse(hourSlotIso(dayKey, hour, timeZone));
  const endsAt = Date.parse(hourSlotIso(dayKey, hour + 1, timeZone));

  return blocks.some(
    (block) =>
      Date.parse(block.startsAt) < endsAt &&
      Date.parse(block.endsAt) > startsAt
  );
}

export function sortedCalendarTimeBlocks(blocks: CalendarTimeBlock[]): CalendarTimeBlock[] {
  return [...blocks].sort(
    (left, right) =>
      left.startsAt.localeCompare(right.startsAt) ||
      left.endsAt.localeCompare(right.endsAt) ||
      left.id.localeCompare(right.id)
  );
}

export function calendarTimeBlockLabel(block: CalendarTimeBlock, timeZone = "UTC"): string {
  const day = new Intl.DateTimeFormat(undefined, {
    day: "numeric",
    month: "short",
    timeZone,
    weekday: "short"
  }).format(new Date(block.startsAt));
  const timeFormatter = new Intl.DateTimeFormat(undefined, {
    hour: "numeric",
    minute: "2-digit",
    timeZone
  });

  return `${day} ${timeFormatter.format(new Date(block.startsAt))}-${timeFormatter.format(new Date(block.endsAt))}`;
}

export function calendarAvailabilitySnippet({
  durationMinutes,
  slots,
  timeZone,
  title
}: {
  durationMinutes: number;
  slots: CalendarTimeBlock[];
  timeZone: string;
  title: string;
}): string {
  const lines = sortedCalendarTimeBlocks(slots).map((slot) => `- ${calendarTimeBlockLabel(slot, timeZone)}`);

  return [
    title.trim() || "Meeting",
    `${durationMinutes} minutes - ${timeZone}`,
    ...lines
  ].join("\n");
}
