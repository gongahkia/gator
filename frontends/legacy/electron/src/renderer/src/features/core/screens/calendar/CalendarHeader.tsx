import type { ReactNode } from "react";
import { CalendarPlus, ChevronLeft, ChevronRight, Sparkles } from "lucide-react";
import type { PlannerActionId } from "../../../../actions/plannerActions";
import { Button, IconButton } from "../../../../components/primitives";
import type { CalendarViewId } from "../../coreViewModels";
import { calendarViewLabel } from "./calendarGrid";

function CalendarTabButton({
  actionId,
  active,
  children,
  onClick
}: {
  actionId: PlannerActionId;
  active: boolean;
  children: ReactNode;
  onClick: () => void;
}): JSX.Element {
  return (
    <Button
      aria-selected={active}
      data-action-id={actionId}
      onClick={onClick}
      role="tab"
      size="sm"
      variant={active ? "secondary" : "ghost"}
    >
      {children}
    </Button>
  );
}

function calendarViewActionId(viewId: CalendarViewId): PlannerActionId {
  return `calendar.view.${viewId}` as PlannerActionId;
}

export function CalendarHeader({
  activeViewId,
  calendarRangeLabel,
  nextRangeLabel,
  onCreate,
  onResetRange,
  onSetView,
  onShiftRange,
  onToggleShareAvailability,
  onToggleSmartReschedule,
  previousRangeLabel,
  shareAvailabilityOpen,
  shareAvailabilityVisible,
  smartRescheduleOpen,
  visibleCalendarViewIds
}: {
  activeViewId: CalendarViewId;
  calendarRangeLabel: string;
  nextRangeLabel: string;
  onCreate: () => void;
  onResetRange: () => void;
  onSetView: (viewId: CalendarViewId) => void;
  onShiftRange: (direction: -1 | 1) => void;
  onToggleShareAvailability: () => void;
  onToggleSmartReschedule: () => void;
  previousRangeLabel: string;
  shareAvailabilityOpen: boolean;
  shareAvailabilityVisible: boolean;
  smartRescheduleOpen: boolean;
  visibleCalendarViewIds: CalendarViewId[];
}): JSX.Element {
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-2">
      <div className="flex min-w-0 items-center gap-1 rounded-hcbMd border border-border bg-bg-secondary p-1" role="tablist" aria-label="Calendar views">
        {visibleCalendarViewIds.map((viewId) => (
          <CalendarTabButton
            actionId={calendarViewActionId(viewId)}
            active={viewId === activeViewId}
            key={viewId}
            onClick={() => onSetView(viewId)}
          >
            {calendarViewLabel(viewId)}
          </CalendarTabButton>
        ))}
      </div>
      <div
        aria-label="Calendar range navigation"
        className="flex shrink-0 items-center gap-1 rounded-hcbMd border border-border bg-bg-secondary p-1"
        role="group"
      >
        <IconButton
          icon={ChevronLeft}
          label={previousRangeLabel}
          onClick={() => onShiftRange(-1)}
          size="sm"
          variant="ghost"
        />
        <Button
          aria-label="Return calendar to today"
          className="min-w-32 max-w-48 truncate px-2"
          onClick={onResetRange}
          size="sm"
          title="Return to today"
          variant="ghost"
        >
          <span className="truncate">{calendarRangeLabel}</span>
        </Button>
        <IconButton
          icon={ChevronRight}
          label={nextRangeLabel}
          onClick={() => onShiftRange(1)}
          size="sm"
          variant="ghost"
        />
      </div>
      <Button data-action-id="calendar.create" onClick={onCreate} size="sm" variant="primary">
        <CalendarPlus aria-hidden="true" size={14} />
        New event
      </Button>
      {shareAvailabilityVisible ? (
        <Button
          aria-expanded={shareAvailabilityOpen}
          onClick={onToggleShareAvailability}
          size="sm"
          variant="primary"
        >
          <CalendarPlus aria-hidden="true" size={14} />
          Share availability
        </Button>
      ) : null}
      <Button
        aria-expanded={smartRescheduleOpen}
        onClick={onToggleSmartReschedule}
        size="sm"
        variant="primary"
      >
        <Sparkles aria-hidden="true" size={14} />
        Reschedule
      </Button>
      <div aria-label="Calendar status" className="sr-only" role="status" />
    </div>
  );
}
