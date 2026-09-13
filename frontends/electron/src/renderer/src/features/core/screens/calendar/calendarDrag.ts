import type { DragEvent } from "react";

const calendarEventDragType = "application/x-hcb-calendar-event";
const calendarEventResizeDragType = "application/x-hcb-calendar-event-resize";

export function startCalendarEventDrag(dragEvent: DragEvent<HTMLElement>, eventId: string): void {
  dragEvent.dataTransfer.effectAllowed = "move";
  dragEvent.dataTransfer.setData(calendarEventDragType, eventId);
  dragEvent.dataTransfer.setData("text/plain", eventId);
}

export function startCalendarEventResizeDrag(dragEvent: DragEvent<HTMLElement>, eventId: string): void {
  dragEvent.stopPropagation();
  dragEvent.dataTransfer.effectAllowed = "move";
  dragEvent.dataTransfer.setData(calendarEventResizeDragType, eventId);
  dragEvent.dataTransfer.setData("text/plain", eventId);
}

export function allowCalendarDrop(dragEvent: DragEvent<HTMLElement>): void {
  dragEvent.preventDefault();
  dragEvent.dataTransfer.dropEffect = "move";
}

export function calendarEventDragId(dragEvent: DragEvent<HTMLElement>): string {
  return dragEvent.dataTransfer.getData(calendarEventDragType);
}

export function calendarEventResizeDragId(dragEvent: DragEvent<HTMLElement>): string {
  return dragEvent.dataTransfer.getData(calendarEventResizeDragType);
}
