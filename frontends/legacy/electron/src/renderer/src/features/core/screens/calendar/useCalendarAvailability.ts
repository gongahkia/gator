import { useEffect, useMemo, useState } from "react";
import type { Dispatch, SetStateAction } from "react";
import type { CoreViewModelSource } from "../../coreViewModelSource";
import {
  addUtcDaysIso,
  dateInputValue,
  dateRangeInputToInclusiveIsoRange,
  startOfUtcDayIso
} from "../../coreScreenShared";
import {
  calendarAvailabilitySnippet,
  sortedCalendarTimeBlocks
} from "./calendarGrid";
import { defaultCalendarId } from "./drafts";
import type { CalendarTimeBlock } from "./types";

export function useCalendarAvailability(source: CoreViewModelSource): {
  addAvailabilitySlot: (slot: CalendarTimeBlock) => void;
  availabilityBusyBlockCount: number | null;
  availabilityCalendarId: string;
  availabilityDurationMinutes: number;
  availabilityEndDate: string;
  availabilityError: string | undefined;
  availabilityHoldPending: boolean;
  availabilityPending: boolean;
  availabilitySlots: CalendarTimeBlock[];
  availabilitySnippet: string;
  availabilityStartDate: string;
  availabilityText: string;
  availabilityTitle: string;
  copyAvailabilitySnippet: () => void;
  createAvailabilityHolds: () => Promise<void>;
  exportAvailability: () => Promise<void>;
  removeAvailabilitySlot: (slotId: string) => void;
  setAvailabilityCalendarId: Dispatch<SetStateAction<string>>;
  setAvailabilityDurationMinutes: Dispatch<SetStateAction<number>>;
  setAvailabilityEndDate: Dispatch<SetStateAction<string>>;
  setAvailabilityStartDate: Dispatch<SetStateAction<string>>;
  setAvailabilityTitle: Dispatch<SetStateAction<string>>;
  setShareAvailabilityOpen: Dispatch<SetStateAction<boolean>>;
  shareAvailabilityOpen: boolean;
} {
  const [shareAvailabilityOpen, setShareAvailabilityOpen] = useState(false);
  const [availabilityTitle, setAvailabilityTitle] = useState("Meeting");
  const [availabilityDurationMinutes, setAvailabilityDurationMinutes] = useState(30);
  const [availabilityCalendarId, setAvailabilityCalendarId] = useState(() => defaultCalendarId(source));
  const [availabilitySlots, setAvailabilitySlots] = useState<CalendarTimeBlock[]>([]);
  const [availabilityHoldPending, setAvailabilityHoldPending] = useState(false);
  const [availabilityStartDate, setAvailabilityStartDate] = useState(() =>
    dateInputValue(startOfUtcDayIso(new Date()))
  );
  const [availabilityEndDate, setAvailabilityEndDate] = useState(() =>
    dateInputValue(addUtcDaysIso(startOfUtcDayIso(new Date()), 6))
  );
  const [availabilityCalendarIds, setAvailabilityCalendarIds] = useState<string[]>([]);
  const [availabilityText, setAvailabilityText] = useState("");
  const [availabilityError, setAvailabilityError] = useState<string | undefined>();
  const [availabilityBusyBlockCount, setAvailabilityBusyBlockCount] = useState<number | null>(null);
  const [availabilityPending, setAvailabilityPending] = useState(false);
  const availableCalendarIds = useMemo(
    () => new Set(source.calendarSources.map((calendar) => calendar.id)),
    [source.calendarSources]
  );
  const selectedAvailabilityCalendarIds = availabilityCalendarIds.filter((calendarId) =>
    availableCalendarIds.has(calendarId)
  );
  const availabilityRange = dateRangeInputToInclusiveIsoRange(availabilityStartDate, availabilityEndDate);
  const canExportAvailability =
    selectedAvailabilityCalendarIds.length > 0 &&
    availabilityRange !== null &&
    Date.parse(availabilityRange.end) > Date.parse(availabilityRange.start) &&
    !availabilityPending;
  const availabilitySnippet = useMemo(
    () =>
      calendarAvailabilitySnippet({
        durationMinutes: availabilityDurationMinutes,
        slots: availabilitySlots,
        timeZone: source.settings.defaultTimeZone,
        title: availabilityTitle
      }),
    [availabilityDurationMinutes, availabilitySlots, availabilityTitle, source.settings.defaultTimeZone]
  );

  useEffect(() => {
    if (availabilityCalendarIds.length > 0 || source.calendarSources.length === 0) {
      return;
    }

    const selectedCalendarIds = source.calendarSources
      .filter((calendar) => calendar.selected)
      .map((calendar) => calendar.id);

    setAvailabilityCalendarIds(
      selectedCalendarIds.length > 0
        ? selectedCalendarIds
        : source.calendarSources.map((calendar) => calendar.id)
    );
  }, [availabilityCalendarIds.length, source.calendarSources]);

  useEffect(() => {
    if (source.calendarSources.length === 0) {
      setAvailabilityCalendarId("");
      return;
    }

    if (availabilityCalendarId && source.calendarSources.some((calendar) => calendar.id === availabilityCalendarId)) {
      return;
    }

    setAvailabilityCalendarId(defaultCalendarId(source));
  }, [availabilityCalendarId, source, source.calendarSources]);

  async function exportAvailability(): Promise<void> {
    if (!canExportAvailability || availabilityRange === null) {
      setAvailabilityError("Choose at least one calendar and a valid date range.");
      return;
    }

    setAvailabilityPending(true);
    setAvailabilityError(undefined);

    const result = await window.hcb?.calendar.exportAvailability({
      calendarIds: selectedAvailabilityCalendarIds,
      start: availabilityRange.start,
      end: availabilityRange.end,
      format: "text"
    });

    setAvailabilityPending(false);

    if (!result?.ok) {
      setAvailabilityError(result?.error.message ?? "Availability export failed.");
      return;
    }

    setAvailabilityText(result.data.text);
    setAvailabilityBusyBlockCount(result.data.busyBlockCount);
  }

  function addAvailabilitySlot(slot: CalendarTimeBlock): void {
    setAvailabilityError(undefined);
    setAvailabilitySlots((current) => {
      if (current.some((candidate) => candidate.id === slot.id)) {
        return current;
      }

      return sortedCalendarTimeBlocks([...current, slot]);
    });
  }

  function removeAvailabilitySlot(slotId: string): void {
    setAvailabilitySlots((current) => current.filter((slot) => slot.id !== slotId));
  }

  function copyAvailabilitySnippet(): void {
    if (availabilitySlots.length === 0) {
      return;
    }

    void navigator.clipboard?.writeText(availabilitySnippet);
  }

  async function createAvailabilityHolds(): Promise<void> {
    if (availabilitySlots.length === 0) {
      setAvailabilityError("Select at least one time block.");
      return;
    }

    if (!availabilityCalendarId) {
      setAvailabilityError("Choose a calendar.");
      return;
    }

    setAvailabilityHoldPending(true);
    setAvailabilityError(undefined);

    for (const slot of sortedCalendarTimeBlocks(availabilitySlots)) {
      const result = await window.hcb?.calendar.create({
        allDay: false,
        calendarId: availabilityCalendarId,
        endsAt: slot.endsAt,
        guestEmails: [],
        location: "",
        notes: "Availability hold",
        recurrence: null,
        reminderMinutes: [],
        startsAt: slot.startsAt,
        title: availabilityTitle.trim() || "Hold"
      });

      if (!result?.ok) {
        setAvailabilityHoldPending(false);
        setAvailabilityError(result?.error.message ?? "Hold write failed.");
        return;
      }
    }

    setAvailabilityHoldPending(false);
    setAvailabilitySlots([]);
    source.refresh();
  }

  return {
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
  };
}
