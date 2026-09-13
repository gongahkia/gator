import { useCallback, useEffect, useRef, useState } from "react";
import type { Dispatch, ReactNode, SetStateAction } from "react";
import type { CalendarEventCompletionScope, CalendarEventUpdateRequest } from "@shared/ipc/contracts";
import { ArrowRightLeft, Copy, Pencil, Save, Trash2, X } from "lucide-react";
import { useInspector } from "../../../../components/Inspector";
import { Button } from "../../../../components/primitives";
import {
  conversionCleanup,
  dispatchConvertCommand,
  type ConvertSourceCleanup
} from "../../conversionEvents";
import { copiedTitle } from "../../copyLabels";
import type { CoreViewModelSource } from "../../coreViewModelSource";
import type { CalendarEventViewModel } from "../../coreViewModels";
import {
  addUtcDaysIso,
  dateInputValue,
  defaultTaskListId,
  startOfUtcDayIso
} from "../../coreScreenShared";
import { CalendarEventDetails, CalendarEventForm } from "./CalendarEventForm";
import {
  calendarDraftRangeLabel,
  calendarEventDraftsEqual,
  calendarEventPayload,
  editCalendarDraft,
  newCalendarDraft
} from "./drafts";
import type { CalendarCreateMode, CalendarCreateSeed, CalendarEventDraft } from "./types";

interface CalendarCreateOptions {
  cleanup?: ConvertSourceCleanup;
  createMode?: CalendarCreateMode;
  draft?: Partial<CalendarEventDraft>;
  taskListId?: string;
}

export function useCalendarEventInspector(source: CoreViewModelSource): {
  calendarActionError: string | undefined;
  moveCalendarEvent: (eventId: string, startsAt: string, allDay: boolean) => void;
  openCreate: (seed?: CalendarCreateSeed, options?: CalendarCreateOptions) => void;
  openEdit: (event: CalendarEventViewModel) => void;
  resizeCalendarEvent: (eventId: string, endsAt: string) => void;
  setCalendarActionError: Dispatch<SetStateAction<string | undefined>>;
} {
  const {
    close: closeInspector,
    current: currentInspector,
    open: openInspector,
    update: updateInspector
  } = useInspector();
  const [draft, setDraftState] = useState<CalendarEventDraft | null>(null);
  const [createMode, setCreateModeState] = useState<CalendarCreateMode>("event");
  const [createTaskListId, setCreateTaskListIdState] = useState(() => defaultTaskListId(source));
  const [formError, setFormError] = useState<string | undefined>();
  const [calendarInspectorMode, setCalendarInspectorModeState] = useState<"view" | "edit">("edit");
  const [calendarActionError, setCalendarActionError] = useState<string | undefined>();
  const [eventWriteScope, setEventWriteScopeState] = useState<CalendarEventCompletionScope>("seriesAll");
  const calendarDraftRef = useRef<CalendarEventDraft | null>(draft);
  const calendarDraftBaselineRef = useRef<CalendarEventDraft | null>(draft);
  const calendarInspectorDirtyRef = useRef(false);
  const calendarInspectorInstanceRef = useRef(0);
  const calendarInspectorModeRef = useRef<"view" | "edit">("edit");
  const createModeRef = useRef<CalendarCreateMode>("event");
  const createTaskListIdRef = useRef(createTaskListId);
  const eventWriteScopeRef = useRef<CalendarEventCompletionScope>("seriesAll");
  const conversionCleanupRef = useRef<ConvertSourceCleanup | null>(null);
  const setDraft = useCallback<Dispatch<SetStateAction<CalendarEventDraft | null>>>((next) => {
    setDraftState((current) => {
      const resolved =
        typeof next === "function"
          ? (next as (value: CalendarEventDraft | null) => CalendarEventDraft | null)(current)
          : next;

      calendarDraftRef.current = resolved;
      calendarInspectorDirtyRef.current = !calendarEventDraftsEqual(
        resolved,
        calendarDraftBaselineRef.current
      );

      return resolved;
    });
  }, []);

  function setCalendarInspectorMode(mode: "view" | "edit"): void {
    calendarInspectorModeRef.current = mode;
    setCalendarInspectorModeState(mode);
  }

  function applyCreateModeToDraft(nextDraft: CalendarEventDraft, mode: CalendarCreateMode): CalendarEventDraft {
    if (mode !== "birthday") {
      return { ...nextDraft, hcbKind: undefined };
    }

    const startsAt = startOfUtcDayIso(nextDraft.startsAt);
    return {
      ...nextDraft,
      hcbKind: "birthday",
      allDay: true,
      startsAt,
      endsAt: addUtcDaysIso(startsAt, 1),
      tags: [],
      repeatFrequency: "yearly",
      repeatCustomFrequency: "yearly",
      repeatEndMode: "never",
      repeatCount: "",
      repeatEndsOn: "",
      repeatInterval: "1"
    };
  }

  function setCreateModeValue(mode: CalendarCreateMode): void {
    createModeRef.current = mode;
    setCreateModeState(mode);
  }

  function setEventWriteScope(scope: CalendarEventCompletionScope): void {
    eventWriteScopeRef.current = scope;
    setEventWriteScopeState(scope);
  }

  function setCreateMode(mode: CalendarCreateMode): void {
    setCreateModeValue(mode);

    setDraft((current) => current ? applyCreateModeToDraft(current, mode) : current);
  }

  function setCreateTaskListId(listId: string): void {
    createTaskListIdRef.current = listId;
    setCreateTaskListIdState(listId);
  }

  useEffect(() => {
    if (currentInspector?.kind !== "event" || !draft) {
      return;
    }

    const dirty =
      calendarInspectorMode === "edit" &&
      !calendarEventDraftsEqual(draft, calendarDraftBaselineRef.current);
    calendarInspectorDirtyRef.current = dirty;
    updateInspector({
      actions: eventInspectorActions(draft, calendarInspectorMode),
      body: eventInspectorBody(draft, calendarInspectorMode),
      dirty,
      hideHeader: eventInspectorHidesHeader(draft, calendarInspectorMode),
      subtitle: eventInspectorSubtitle(draft),
      title: eventInspectorTitle(draft)
    });
  }, [
    currentInspector?.kind,
    draft,
    formError,
    calendarInspectorMode,
    source.calendarSources,
    source.taskLists,
    createMode,
    createTaskListId,
    source.settings.defaultTimeZone,
    source.settings.calendarEventColorOverrides,
    eventWriteScope,
    updateInspector
  ]);

  useEffect(() => {
    if (createTaskListIdRef.current || source.taskLists.length === 0) {
      return;
    }

    setCreateTaskListId(defaultTaskListId(source));
  }, [source, source.taskLists]);

  function canReplaceEventInspector(): boolean {
    return (
      currentInspector?.kind !== "event" ||
      calendarInspectorModeRef.current !== "edit" ||
      !calendarInspectorDirtyRef.current
    );
  }

  function eventInspectorTitle(nextDraft: CalendarEventDraft): string {
    if (nextDraft.mode === "create") {
      if (createModeRef.current === "task") {
        return "New task";
      }

      if (createModeRef.current === "birthday") {
        return "New birthday";
      }
    }

    return nextDraft.mode === "edit" ? nextDraft.title || "Event" : "New event";
  }

  function eventInspectorSubtitle(nextDraft: CalendarEventDraft): string {
    const calendar = source.calendarSources.find((calendarSource) => calendarSource.id === nextDraft.calendarId);
    return `${calendar?.title ?? "Calendar"} · ${calendarDraftRangeLabel(nextDraft, calendar?.timeZone ?? source.settings.defaultTimeZone)}`;
  }

  function eventInspectorBody(
    nextDraft: CalendarEventDraft,
    mode = calendarInspectorModeRef.current
  ): ReactNode {
    if (nextDraft.mode === "edit" && mode === "view") {
      return (
        <CalendarEventDetails
          calendars={source.calendarSources}
          defaultTimeZone={source.settings.defaultTimeZone}
          draft={nextDraft}
          eventColorOverrides={source.settings.calendarEventColorOverrides}
          key={`view-${calendarInspectorInstanceRef.current}`}
          rules={source.settings.autoTagRules}
          source={source}
        />
      );
    }

    return (
      <CalendarEventForm
        calendars={source.calendarSources}
        createMode={createModeRef.current}
        defaultTimeZone={source.settings.defaultTimeZone}
        draft={nextDraft}
        error={formError}
        eventColorOverrides={source.settings.calendarEventColorOverrides}
        key={calendarInspectorInstanceRef.current}
        onCreateModeChange={setCreateMode}
        rules={source.settings.autoTagRules}
        setDraft={(next) => setDraft(next)}
        setTaskListId={setCreateTaskListId}
        taskListId={createTaskListId}
        taskLists={source.taskLists}
      />
    );
  }

  function recurrenceScopeSelect(): ReactNode {
    return (
      <select
        aria-label="Recurring event edit scope"
        className="h-7 rounded-hcbMd border border-border bg-surface-0 px-2 text-[var(--text-sm)] text-text-primary"
        onChange={(event) => setEventWriteScope(event.currentTarget.value as CalendarEventCompletionScope)}
        value={eventWriteScope}
      >
        <option value="occurrence">This occurrence</option>
        <option value="seriesFuture">This and future</option>
        <option value="seriesAll">Whole series</option>
      </select>
    );
  }

  function eventInspectorActions(
    nextDraft: CalendarEventDraft,
    mode = calendarInspectorModeRef.current
  ): ReactNode {
    if (nextDraft.mode === "edit" && mode === "view") {
      return (
        <div className="flex w-full items-center justify-between gap-2">
          <div className="flex flex-wrap items-center gap-2">
            {recurringDraft(nextDraft) ? recurrenceScopeSelect() : null}
            <Button onClick={() => void deleteDraft()} size="sm" variant="danger">
              <Trash2 aria-hidden="true" size={14} />
              Delete event
            </Button>
            <Button onClick={() => setCalendarInspectorMode("edit")} size="sm" variant="secondary">
              <Pencil aria-hidden="true" size={14} />
              Edit
            </Button>
            <Button onClick={() => duplicateEventDraftValue(nextDraft)} size="sm" variant="secondary">
              <Copy aria-hidden="true" size={14} />
              Duplicate
            </Button>
            {nextDraft.hcbKind !== "birthday" ? (
              <>
                <Button onClick={() => convertEventDraft(nextDraft, "task")} size="sm" variant="secondary">
                  <ArrowRightLeft aria-hidden="true" size={14} />
                  Convert to task
                </Button>
                <Button onClick={() => convertEventDraft(nextDraft, "note")} size="sm" variant="secondary">
                  <ArrowRightLeft aria-hidden="true" size={14} />
                  Convert to note
                </Button>
              </>
            ) : null}
          </div>
          <Button onClick={() => void cancelEventInspector()} size="sm" variant="ghost">
            <X aria-hidden="true" size={14} />
            Close
          </Button>
        </div>
      );
    }

    return (
      <>
        {nextDraft.mode === "edit" && recurringDraft(nextDraft) ? recurrenceScopeSelect() : null}
        {nextDraft.mode === "edit" ? (
          <Button onClick={() => void deleteDraft()} size="sm" variant="danger">
            <Trash2 aria-hidden="true" size={14} />
            Delete event
          </Button>
        ) : null}
        {nextDraft.mode === "edit" ? (
          <Button onClick={() => duplicateEventDraftValue(nextDraft)} size="sm" variant="secondary">
            <Copy aria-hidden="true" size={14} />
            Duplicate
          </Button>
        ) : null}
        {nextDraft.mode === "edit" && nextDraft.hcbKind !== "birthday" ? (
          <>
            <Button onClick={() => convertEventDraft(nextDraft, "task")} size="sm" variant="secondary">
              <ArrowRightLeft aria-hidden="true" size={14} />
              Convert to task
            </Button>
            <Button onClick={() => convertEventDraft(nextDraft, "note")} size="sm" variant="secondary">
              <ArrowRightLeft aria-hidden="true" size={14} />
              Convert to note
            </Button>
          </>
        ) : null}
        <Button onClick={() => void cancelEventInspector()} size="sm" variant="ghost">
          <X aria-hidden="true" size={14} />
          Cancel
        </Button>
        <Button onClick={() => void saveDraft()} size="sm" variant="primary">
          <Save aria-hidden="true" size={14} />
          Save
        </Button>
      </>
    );
  }

  function eventInspectorHidesHeader(
    nextDraft: CalendarEventDraft,
    mode = calendarInspectorModeRef.current
  ): boolean {
    return nextDraft.mode === "edit" && mode === "view";
  }

  function openEventInspector(
    nextDraft: CalendarEventDraft,
    mode: "view" | "edit" = nextDraft.mode === "edit" ? "view" : "edit"
  ): void {
    calendarInspectorInstanceRef.current += 1;
    calendarDraftBaselineRef.current = nextDraft;
    calendarDraftRef.current = nextDraft;
    calendarInspectorDirtyRef.current = false;
    setCalendarInspectorMode(mode);
    setEventWriteScope(defaultEventWriteScope(nextDraft, source.settings.eventCompletionDefaultScope));
    setFormError(undefined);
    setDraft(nextDraft);
    openInspector({
      actions: eventInspectorActions(nextDraft, mode),
      body: eventInspectorBody(nextDraft, mode),
      dirty: false,
      hideHeader: eventInspectorHidesHeader(nextDraft, mode),
      id: nextDraft.id ?? "new",
      kind: "event",
      onConfirmClose: () => {
        if (calendarDraftRef.current?.mode === "create") {
          discardEventInspectorState();
          return true;
        }

        return calendarInspectorModeRef.current !== "edit" || !calendarInspectorDirtyRef.current;
      },
      subtitle: eventInspectorSubtitle(nextDraft),
      title: eventInspectorTitle(nextDraft)
    });
  }

  function openCreate(seed?: CalendarCreateSeed, options: CalendarCreateOptions = {}): void {
    if (!canReplaceEventInspector()) {
      return;
    }

    const nextMode = options.createMode ?? "event";
    const nextDraft = applyCreateModeToDraft({
      ...newCalendarDraft(source, seed),
      ...options.draft,
      mode: "create"
    }, nextMode);

    setCreateModeValue(nextMode);
    setCreateTaskListId(options.taskListId ?? defaultTaskListId(source));
    conversionCleanupRef.current = options.cleanup ?? null;
    openEventInspector(nextDraft, "edit");
  }

  function openEdit(event: CalendarEventViewModel): void {
    if (!canReplaceEventInspector()) {
      return;
    }

    setCreateMode("event");
    conversionCleanupRef.current = null;
    openEventInspector(editCalendarDraft(event), "view");
  }

  function duplicateEventDraftValue(sourceDraft: CalendarEventDraft): void {
    const nextMode: CalendarCreateMode = isBirthdayLikeDraft(sourceDraft) ? "birthday" : "event";
    const nextDraft = applyCreateModeToDraft({
      ...sourceDraft,
      completedAt: null,
      conference: null,
      guests: "",
      id: undefined,
      mode: "create",
      mutationState: undefined,
      title: copiedTitle(sourceDraft.title, "Untitled event")
    }, nextMode);

    setCreateModeValue(nextMode);
    setCreateTaskListId(defaultTaskListId(source));
    conversionCleanupRef.current = null;
    openEventInspector(nextDraft, "edit");
  }

  function convertEventDraft(sourceDraft: CalendarEventDraft, target: "task" | "note"): void {
    if (!sourceDraft.id || sourceDraft.hcbKind === "birthday") {
      return;
    }

    const cleanup = conversionCleanup("event", sourceDraft.id, target);

    if (target === "task") {
      dispatchConvertCommand({
        cleanup,
        target,
        taskDraft: {
          title: sourceDraft.title,
          notes: sourceDraft.notes,
          dueDate: dateInputValue(sourceDraft.startsAt),
          listId: defaultTaskListId(source),
          priority: "none",
          plannedStart: sourceDraft.allDay ? null : sourceDraft.startsAt,
          plannedEnd: sourceDraft.allDay ? null : sourceDraft.endsAt,
          durationMinutes: null,
          lockedSchedule: false,
          tags: sourceDraft.tags
        }
      });
      return;
    }

    dispatchConvertCommand({
      cleanup,
      target,
      noteDraft: {
        title: sourceDraft.title,
        body: eventNoteBody(
          sourceDraft,
          source.calendarSources.find((calendarSource) => calendarSource.id === sourceDraft.calendarId)?.timeZone ??
            source.settings.defaultTimeZone
        ),
        tags: sourceDraft.tags,
        listId: defaultTaskListId(source)
      }
    });
  }

  function isBirthdayLikeDraft(candidate: CalendarEventDraft): boolean {
    if (candidate.hcbKind === "birthday") {
      return true;
    }

    const frequency =
      candidate.repeatFrequency === "custom" ? candidate.repeatCustomFrequency : candidate.repeatFrequency;

    return (
      candidate.allDay &&
      frequency === "yearly" &&
      candidate.location.trim() === "" &&
      candidate.notes.trim() === "" &&
      candidate.guests.trim() === "" &&
      candidate.conference === null
    );
  }

  async function closeEventInspectorAfterMutation(): Promise<void> {
    discardEventInspectorState();
    await closeInspector();
    source.refresh();
  }

  async function cleanupConvertedSource(): Promise<string | null> {
    const cleanup = conversionCleanupRef.current;

    if (!cleanup) {
      return null;
    }

    conversionCleanupRef.current = null;

    if (cleanup.kind === "event") {
      const result = await window.hcb?.calendar.delete({ id: cleanup.id });
      return result?.ok ? null : result?.error.message ?? "Original event was not removed.";
    }

    const result = await window.hcb?.tasks.delete({ id: cleanup.id });
    return result?.ok ? null : result?.error.message ?? "Original task was not removed.";
  }

  function discardEventInspectorState(): void {
    calendarDraftBaselineRef.current = null;
    calendarDraftRef.current = null;
    calendarInspectorDirtyRef.current = false;
    conversionCleanupRef.current = null;
    setCalendarInspectorMode("edit");
    setDraft(null);
    setCreateMode("event");
    setFormError(undefined);
  }

  async function saveDraft(): Promise<void> {
    const currentDraft = calendarDraftRef.current;

    if (!currentDraft) {
      return;
    }

    const payload = calendarEventPayload(currentDraft);

    if (!payload.title) {
      setFormError("Title is required.");
      return;
    }

    if (currentDraft.mode === "create" && createModeRef.current === "task") {
      const listId = createTaskListIdRef.current || defaultTaskListId(source);

      if (!listId) {
        setFormError("Choose a task list.");
        return;
      }

      const saved = await source.createTask({
        title: payload.title,
        notes: currentDraft.notes,
        dueDate: dateInputValue(currentDraft.startsAt),
        listId,
        parentId: null,
        priority: "none",
        plannedStart: null,
        plannedEnd: null,
        durationMinutes: null,
        lockedSchedule: false,
        tags: currentDraft.tags
      });

      if (!saved) {
        setFormError(source.taskMutationError ?? "Task write failed.");
        return;
      }

      await closeEventInspectorAfterMutation();
      return;
    }

    if (!payload.calendarId) {
      setFormError("Choose a calendar.");
      return;
    }

    const isBirthdayDraft =
      currentDraft.hcbKind === "birthday" ||
      (currentDraft.mode === "create" && createModeRef.current === "birthday");
    const writePayload =
      isBirthdayDraft
        ? {
            ...payload,
            allDay: true,
            startsAt: startOfUtcDayIso(currentDraft.startsAt),
            endsAt: addUtcDaysIso(startOfUtcDayIso(currentDraft.startsAt), 1),
            hcbKind: "birthday" as const,
            location: "",
            notes: "",
            tags: [],
            guestEmails: [],
            recurrence: {
              frequency: "yearly" as const,
              interval: 1,
              endsOn: null,
              count: null
            }
          }
        : payload;

    const result =
      currentDraft.mode === "create"
        ? await window.hcb?.calendar.create(writePayload)
        : await window.hcb?.calendar.update({
            id: currentDraft.id ?? "",
            ...(recurringDraft(currentDraft) ? { scope: eventWriteScopeRef.current } : {}),
            ...writePayload
          } satisfies CalendarEventUpdateRequest);

    if (!result?.ok) {
      setFormError(result?.error.message ?? "Calendar event write failed.");
      return;
    }

    const cleanupError = await cleanupConvertedSource();
    await closeEventInspectorAfterMutation();

    if (cleanupError) {
      setCalendarActionError(`Converted item was saved, but ${cleanupError}`);
    }
  }

  async function deleteDraft(): Promise<void> {
    const currentDraft = calendarDraftRef.current;

    if (!currentDraft?.id) {
      return;
    }

    const result = await window.hcb?.calendar.delete({
      id: currentDraft.id,
      ...(recurringDraft(currentDraft) ? { scope: eventWriteScopeRef.current } : {})
    });

    if (!result?.ok) {
      setFormError(result?.error.message ?? "Calendar event delete failed.");
      return;
    }

    await closeEventInspectorAfterMutation();
  }

  async function cancelEventInspector(): Promise<void> {
    discardEventInspectorState();
    await closeInspector();
  }

  async function updateCalendarEventTime(
    request: Pick<CalendarEventUpdateRequest, "id" | "startsAt" | "endsAt" | "allDay" | "scope">
  ): Promise<void> {
    const result = await window.hcb?.calendar.update(request);

    if (!result?.ok) {
      setCalendarActionError(result?.error.message ?? "Calendar event update failed.");
      return;
    }

    setCalendarActionError(undefined);
    source.refresh();
  }

  function moveCalendarEvent(eventId: string, startsAt: string, allDay: boolean): void {
    const event = source.calendarEventsById[eventId];

    if (!event) {
      return;
    }

    const durationMs = Math.max(5 * 60 * 1000, Date.parse(event.endsAt) - Date.parse(event.startsAt));
    const endsAt = allDay
      ? addUtcDaysIso(startsAt, Math.max(1, Math.round(durationMs / (24 * 60 * 60 * 1000))))
      : new Date(Date.parse(startsAt) + durationMs).toISOString();

    void updateCalendarEventTime({
      id: event.id,
      startsAt,
      endsAt,
      allDay
    });
  }

  function resizeCalendarEvent(eventId: string, endsAt: string): void {
    const event = source.calendarEventsById[eventId];

    if (!event || Date.parse(endsAt) <= Date.parse(event.startsAt)) {
      return;
    }

    void updateCalendarEventTime({
      id: event.id,
      endsAt
    });
  }

  return {
    calendarActionError,
    moveCalendarEvent,
    openCreate,
    openEdit,
    resizeCalendarEvent,
    setCalendarActionError
  };
}

function eventNoteBody(sourceDraft: CalendarEventDraft, timeZone: string): string {
  return [
    sourceDraft.notes.trim(),
    `Event: ${calendarDraftRangeLabel(sourceDraft, timeZone)}`,
    sourceDraft.location.trim() ? `Location: ${sourceDraft.location.trim()}` : ""
  ].filter(Boolean).join("\n\n");
}

function recurringDraft(draft: CalendarEventDraft): boolean {
  return draft.repeatFrequency !== "none" ||
    !!draft.recurringEventId ||
    !!draft.originalStartAt ||
    (!!draft.eventId && draft.eventId !== draft.id);
}

function defaultEventWriteScope(
  draft: CalendarEventDraft,
  setting: CoreViewModelSource["settings"]["eventCompletionDefaultScope"]
): CalendarEventCompletionScope {
  if (!recurringDraft(draft)) {
    return "seriesAll";
  }

  return setting === "ask" ? "occurrence" : setting;
}
