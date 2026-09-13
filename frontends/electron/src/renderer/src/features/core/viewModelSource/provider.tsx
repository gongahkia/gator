import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import type {
  CalendarEventSummary,
  CalendarRangeRequest,
  CalendarRangeResponse,
  GoogleStatusResponse,
  NoteListSummary,
  NoteSummary,
  ScheduledTaskBlockListRequest,
  ScheduledTaskBlockListResponse,
  ScheduledTaskBlockSummary,
  TaskSummary
} from "@shared/ipc/contracts";
import type { CalendarEventViewModel, TaskViewModel } from "../coreViewModels";
import { emptySnapshot } from "./defaults";
import { dateOnlyFromLocalDate, visibleCalendarRange } from "./dateFormat";
import { safeHydrationErrorMessage } from "./hydrationError";
import { hydrateCoreData, loadAllPages, loadCoreData, type CoreDataHydrationSnapshot } from "./loader";
import { unwrap } from "./result";
import { useSettingsMutations } from "./settingsMutations";
import { hasSnapshotData } from "./snapshot";
import { buildCoreViewModelSource } from "./sourceBuilder";
import { useTaskMutations } from "./taskMutations";
import type {
  CalendarRangeLoadRequest,
  CoreDataLoadState,
  CoreDataSnapshot,
  CoreViewModelSource
} from "./types";

const CoreDataContext = createContext<CoreViewModelSource | null>(null);

function systemPrefersDark(): boolean {
  return typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-color-scheme: dark)").matches;
}

function normalizedCalendarRange(range: CalendarRangeLoadRequest): CalendarRangeLoadRequest | null {
  const startMs = Date.parse(range.start);
  const endMs = Date.parse(range.end);

  if (!Number.isFinite(startMs) || !Number.isFinite(endMs) || endMs <= startMs) {
    return null;
  }

  return {
    start: new Date(startMs).toISOString(),
    end: new Date(endMs).toISOString()
  };
}

function calendarRangeLoaded(
  loadedRanges: readonly CalendarRangeLoadRequest[],
  range: CalendarRangeLoadRequest
): boolean {
  return loadedRanges.some((loadedRange) =>
    loadedRange.start <= range.start && loadedRange.end >= range.end
  );
}

function mergedCalendarRanges(
  ranges: readonly CalendarRangeLoadRequest[]
): CalendarRangeLoadRequest[] {
  const sorted = [...ranges].sort((left, right) => left.start.localeCompare(right.start));
  const merged: CalendarRangeLoadRequest[] = [];

  for (const range of sorted) {
    const previous = merged.at(-1);

    if (!previous || range.start > previous.end) {
      merged.push({ ...range });
      continue;
    }

    if (range.end > previous.end) {
      previous.end = range.end;
    }
  }

  return merged;
}

function mergeById<T extends { id: string }>(current: readonly T[], next: readonly T[]): T[] {
  const merged = new Map(current.map((item) => [item.id, item]));

  for (const item of next) {
    merged.set(item.id, item);
  }

  return [...merged.values()];
}

function mergeHydratedSnapshot(
  current: CoreDataSnapshot,
  hydration: CoreDataHydrationSnapshot
): CoreDataSnapshot {
  return {
    ...current,
    tasks: hydration.tasks ? mergeHydratedTasks(current.tasks, hydration.tasks) : current.tasks,
    notes: hydration.notes ? mergeById<NoteSummary>(current.notes, hydration.notes) : current.notes,
    noteLists: hydration.noteLists
      ? mergeById<NoteListSummary>(current.noteLists, hydration.noteLists)
      : current.noteLists,
    resourceCounts: {
      ...current.resourceCounts,
      ...hydration.resourceCounts
    }
  };
}

function mergeHydratedTasks(
  current: readonly TaskSummary[],
  hydration: readonly TaskSummary[]
): TaskSummary[] {
  const merged = new Map(hydration.map((task) => [task.id, task]));

  for (const task of current) {
    const hydrated = merged.get(task.id);

    if (!hydrated || shouldKeepCurrentTask(task, hydrated)) {
      merged.set(task.id, task);
    }
  }

  return [...merged.values()];
}

function shouldKeepCurrentTask(current: TaskSummary, hydrated: TaskSummary): boolean {
  if (current.mutationState === "queued" || current.mutationState === "failed") {
    return true;
  }

  const currentMs = Date.parse(current.updatedAt);
  const hydratedMs = Date.parse(hydrated.updatedAt);

  return Number.isFinite(currentMs) && Number.isFinite(hydratedMs) && currentMs > hydratedMs;
}

export function CoreDataProvider({ children }: { children: ReactNode }): JSX.Element {
  const source = usePreloadCoreSource();

  return <CoreDataContext.Provider value={source}>{children}</CoreDataContext.Provider>;
}

export function useCoreViewModelSource(): CoreViewModelSource {
  const source = useContext(CoreDataContext);

  if (!source) {
    throw new Error("CoreDataProvider is missing.");
  }

  return source;
}

function usePreloadCoreSource(): CoreViewModelSource {
  const [loadState, setLoadState] = useState<CoreDataLoadState>({
    snapshot: emptySnapshot,
    appearanceReady: false,
    state: "loading",
    hydrationState: "idle"
  });
  const [prefersDark, setPrefersDark] = useState(systemPrefersDark);
  const cachedDataReported = useRef(false);
  const googleStatusRequested = useRef(false);
  const runtimeStatusRequested = useRef(false);
  const scheduleSuggestionRequested = useRef(false);
  const backgroundHydrationRequested = useRef(false);
  const loadedCalendarRanges = useRef<CalendarRangeLoadRequest[]>([]);
  const pendingCalendarRangeLoads = useRef(new Map<string, Promise<boolean>>());
  const taskViewModelCache = useRef(new Map<string, { signature: string; viewModel: TaskViewModel }>());
  const calendarEventViewModelCache = useRef(
    new Map<string, { signature: string; viewModel: CalendarEventViewModel }>()
  );

  useEffect(() => {
    if (typeof window.matchMedia !== "function") {
      return;
    }

    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const listener = (event: MediaQueryListEvent): void => setPrefersDark(event.matches);
    setPrefersDark(media.matches);

    if (typeof media.addEventListener === "function") {
      media.addEventListener("change", listener);
      return () => media.removeEventListener("change", listener);
    }

    media.addListener(listener);
    return () => media.removeListener(listener);
  }, []);

  const refreshSyncStatus = useCallback(() => {
    void window.hcb?.sync.status().then((result) => {
      if (!result?.ok) {
        return;
      }

      setLoadState((current) => ({
        ...current,
        snapshot: {
          ...current.snapshot,
          syncStatus: result.data
        }
      }));
    });
  }, []);

  const refreshUndoStatus = useCallback(() => {
    void window.hcb?.undo.status().then((result) => {
      if (!result?.ok) {
        return;
      }

      setLoadState((current) => ({
        ...current,
        snapshot: {
          ...current.snapshot,
          undoStatus: result.data
        }
      }));
    });
  }, []);

  const refreshNativeStatus = useCallback(() => {
    void window.hcb?.native.capabilities().then((result) => {
      if (!result?.ok) {
        return;
      }

      setLoadState((current) => ({
        ...current,
        snapshot: {
          ...current.snapshot,
          native: result.data
        }
      }));
    });
  }, []);

  const refreshDiagnosticsSummary = useCallback(() => {
    void window.hcb?.diagnostics.summary().then((result) => {
      if (!result?.ok) {
        return;
      }

      setLoadState((current) => ({
        ...current,
        snapshot: {
          ...current.snapshot,
          diagnosticsSummary: result.data
        }
      }));
    });
  }, []);

  const setGoogleStatus = useCallback((googleStatus: GoogleStatusResponse) => {
    setLoadState((current) => ({
      ...current,
      snapshot: {
        ...current.snapshot,
        googleStatus
      }
    }));
  }, []);

  const refreshGoogleStatus = useCallback(() => {
    void window.hcb?.google.status().then((result) => {
      if (!result?.ok) {
        return;
      }

      setGoogleStatus(result.data);
    });
  }, [setGoogleStatus]);

  const load = useCallback(() => {
    if (!window.hcb) {
      setLoadState({
        snapshot: emptySnapshot,
        appearanceReady: true,
        state: "offline",
        errorMessage: "Preload bridge is unavailable.",
        hydrationState: "idle"
      });
      return;
    }

    setLoadState((current) => ({
      ...current,
      state: hasSnapshotData(current.snapshot) ? "stale" : "loading",
      errorMessage: undefined,
      hydrationErrorMessage: undefined,
      hydrationState: "idle"
    }));
    scheduleSuggestionRequested.current = false;
    backgroundHydrationRequested.current = false;
    runtimeStatusRequested.current = false;
    googleStatusRequested.current = false;

    const bootstrapGet = (window.hcb as { bootstrap?: { get?: typeof window.hcb.bootstrap.get } })
      .bootstrap?.get;
    const settingsPromise = bootstrapGet
      ? undefined
      : window.hcb.settings.get().then((result) => unwrap(result, "Settings failed"));

    void settingsPromise?.then(
      (settings) => {
        setLoadState((current) => ({
          ...current,
          appearanceReady: true,
          snapshot: {
            ...current.snapshot,
            settings
          }
        }));
      },
      () => undefined
    );

    const initialCalendarRange = visibleCalendarRange();

    void loadCoreData(settingsPromise, initialCalendarRange).then(
      (snapshot) => {
        loadedCalendarRanges.current = [initialCalendarRange];
        pendingCalendarRangeLoads.current.clear();
        setLoadState(() => ({
          appearanceReady: true,
          snapshot,
          state: hasSnapshotData(snapshot) ? "ready" : "empty",
          hydrationState: "idle"
        }));
      },
      (error: unknown) => {
        setLoadState((current) => ({
          ...current,
          state: hasSnapshotData(current.snapshot) ? "stale" : "error",
          errorMessage: error instanceof Error ? error.message : "Planner data read failed.",
          hydrationState: "idle"
        }));
      }
    );
  }, []);

  const ensureCalendarRange = useCallback((range: CalendarRangeLoadRequest): Promise<boolean> => {
    if (!window.hcb) {
      return Promise.resolve(false);
    }

    const normalized = normalizedCalendarRange(range);

    if (!normalized) {
      return Promise.resolve(false);
    }

    if (calendarRangeLoaded(loadedCalendarRanges.current, normalized)) {
      return Promise.resolve(true);
    }

    const key = `${normalized.start}|${normalized.end}`;
    const pending = pendingCalendarRangeLoads.current.get(key);

    if (pending) {
      return pending;
    }

    const loadPromise = Promise.all([
      loadAllPages<CalendarRangeRequest, CalendarRangeResponse>(
        { start: normalized.start, end: normalized.end, limit: 500 },
        (request) => window.hcb!.calendar
          .listEvents(request)
          .then((result) => unwrap(result, "Calendar events failed"))
      ),
      loadAllPages<ScheduledTaskBlockListRequest, ScheduledTaskBlockListResponse>(
        { start: normalized.start, end: normalized.end, limit: 500 },
        (request) => window.hcb!.calendar
          .listScheduledTaskBlocks(request)
          .then((result) => unwrap(result, "Scheduled task blocks failed"))
      )
    ])
      .then(([events, scheduledTaskBlocks]) => {
        loadedCalendarRanges.current = mergedCalendarRanges([
          ...loadedCalendarRanges.current,
          normalized
        ]);
        setLoadState((current) => ({
          ...current,
          snapshot: {
            ...current.snapshot,
            events: mergeById<CalendarEventSummary>(current.snapshot.events, events.items),
            scheduledTaskBlocks: mergeById<ScheduledTaskBlockSummary>(
              current.snapshot.scheduledTaskBlocks,
              scheduledTaskBlocks.items
            )
          }
        }));
        return true;
      })
      .catch(() => false)
      .finally(() => {
        pendingCalendarRangeLoads.current.delete(key);
      });

    pendingCalendarRangeLoads.current.set(key, loadPromise);
    return loadPromise;
  }, []);

  const {
    taskMutation,
    clearTaskMutationError,
    retryLastTaskMutation,
    createTask,
    updateTask,
    completeTask,
    reopenTask,
    completeEvent,
    reopenEvent,
    moveTask,
    bulkRescheduleTasks,
    deleteTask,
    createTaskList,
    renameTaskList,
    deleteTaskList,
    scheduleTaskBlock,
    moveScheduledTaskBlock,
    unscheduleTaskBlock
  } = useTaskMutations({
    setLoadState,
    refreshSyncStatus,
    refreshUndoStatus
  });

  const {
    settingsMutation,
    updateSettings,
    runRecoveryAction
  } = useSettingsMutations({
    load,
    refreshDiagnosticsSummary,
    refreshSyncStatus,
    setLoadState
  });

  const createTag = useCallback(async (request: Parameters<NonNullable<typeof window.hcb>["tags"]["create"]>[0]) => {
    const result = await window.hcb?.tags.create(request);

    if (!result?.ok) {
      return null;
    }

    load();
    return result.data;
  }, [load]);

  const updateTag = useCallback(async (request: Parameters<NonNullable<typeof window.hcb>["tags"]["update"]>[0]) => {
    const result = await window.hcb?.tags.update(request);

    if (!result?.ok) {
      return null;
    }

    load();
    return result.data;
  }, [load]);

  const deleteTag = useCallback(async (request: Parameters<NonNullable<typeof window.hcb>["tags"]["delete"]>[0]) => {
    const result = await window.hcb?.tags.delete(request);

    if (!result?.ok) {
      return null;
    }

    load();
    return result.data;
  }, [load]);

  const mergeTags = useCallback(async (request: Parameters<NonNullable<typeof window.hcb>["tags"]["merge"]>[0]) => {
    const result = await window.hcb?.tags.merge(request);

    if (!result?.ok) {
      return null;
    }

    load();
    return result.data;
  }, [load]);

  const bulkApplyTags = useCallback(async (request: Parameters<NonNullable<typeof window.hcb>["tags"]["bulkApply"]>[0]) => {
    const result = await window.hcb?.tags.bulkApply(request);

    if (!result?.ok) {
      return null;
    }

    load();
    return result.data;
  }, [load]);

  const previewAutoTagReapply = useCallback(async (request: Parameters<NonNullable<typeof window.hcb>["tags"]["previewAutoReapply"]>[0]) => {
    const result = await window.hcb?.tags.previewAutoReapply(request);
    return result?.ok ? result.data : null;
  }, []);

  const applyAutoTagReapply = useCallback(async (request: Parameters<NonNullable<typeof window.hcb>["tags"]["applyAutoReapply"]>[0]) => {
    const result = await window.hcb?.tags.applyAutoReapply(request);

    if (!result?.ok) {
      return null;
    }

    load();
    void refreshUndoStatus();
    return result.data;
  }, [load, refreshUndoStatus]);

  const tagAnalytics = useCallback(async () => {
    const result = await window.hcb?.tags.analytics();
    return result?.ok ? result.data : null;
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    if (
      backgroundHydrationRequested.current ||
      !window.hcb ||
      (loadState.state !== "ready" && loadState.state !== "empty")
    ) {
      return;
    }

    backgroundHydrationRequested.current = true;
    const timeout = window.setTimeout(() => {
      const startedAt = performance.now();

      setLoadState((current) => ({
        ...current,
        hydrationErrorMessage: undefined,
        hydrationState: "loading"
      }));
      recordRendererTiming({
        kind: "startup",
        name: "startup.hydration.deferred-start",
        durationMs: 0
      });

      void hydrateCoreData().then(
        (hydration) => {
          const failed = hydration.failedResources.length > 0;
          setLoadState((current) => ({
            ...current,
            hydrationErrorMessage: failed ? hydration.errorMessage : undefined,
            hydrationState: failed ? "failed" : "success",
            snapshot: mergeHydratedSnapshot(current.snapshot, hydration)
          }));
          recordRendererTiming({
            kind: "startup",
            name: "startup.hydration.merge",
            durationMs: performance.now() - startedAt,
            metadata: {
              outcome: failed ? "failed" : "success",
              errorMessage: failed ? hydration.errorMessage ?? "Background hydration failed." : null,
              failedResources: failed ? hydration.failedResources.join(",") : null,
              tasks: hydration.tasks?.length ?? null,
              notes: hydration.notes?.length ?? null
            }
          });
          if (failed) {
            refreshDiagnosticsSummary();
          }
        },
        (error) => {
          const errorMessage = safeHydrationErrorMessage(error);
          setLoadState((current) => ({
            ...current,
            hydrationErrorMessage: errorMessage,
            hydrationState: "failed"
          }));
          recordRendererTiming({
            kind: "startup",
            name: "startup.hydration.merge",
            durationMs: performance.now() - startedAt,
            metadata: {
              errorMessage,
              outcome: "failed"
            }
          });
          refreshDiagnosticsSummary();
        }
      );
    }, 1_000);

    return () => window.clearTimeout(timeout);
  }, [loadState.state, refreshDiagnosticsSummary]);

  useEffect(() => {
    if (
      loadState.snapshot.diagnosticsSummary ||
      (loadState.state !== "ready" && loadState.state !== "empty")
    ) {
      return;
    }

    const timeout = window.setTimeout(() => {
      refreshDiagnosticsSummary();
    }, 5_000);

    return () => window.clearTimeout(timeout);
  }, [loadState.snapshot.diagnosticsSummary, loadState.state, refreshDiagnosticsSummary]);

  useEffect(() => {
    if (
      googleStatusRequested.current ||
      (loadState.state !== "ready" && loadState.state !== "empty")
    ) {
      return;
    }

    googleStatusRequested.current = true;
    const timeout = window.setTimeout(() => {
      refreshGoogleStatus();
    }, 250);

    return () => window.clearTimeout(timeout);
  }, [loadState.state, refreshGoogleStatus]);

  useEffect(() => {
    if (
      runtimeStatusRequested.current ||
      (loadState.state !== "ready" && loadState.state !== "empty")
    ) {
      return;
    }

    runtimeStatusRequested.current = true;
    const timeout = window.setTimeout(() => {
      refreshSyncStatus();
      refreshUndoStatus();
      refreshNativeStatus();
    }, 250);

    return () => window.clearTimeout(timeout);
  }, [loadState.state, refreshNativeStatus, refreshSyncStatus, refreshUndoStatus]);

  useEffect(() => {
    if (
      scheduleSuggestionRequested.current ||
      !window.hcb ||
      (loadState.state !== "ready" && loadState.state !== "empty")
    ) {
      return;
    }

    scheduleSuggestionRequested.current = true;
    const settings = loadState.snapshot.settings;
    const scheduleDate = dateOnlyFromLocalDate(new Date());
    const startedAt = performance.now();

    void window.hcb.calendar
      .scheduleSuggest({
        date: scheduleDate,
        capacityMinutes: settings.todayCapacityMinutes,
        workingHours: {
          start: settings.todayWorkingHoursStart,
          end: settings.todayWorkingHoursEnd
        }
      })
      .then((result) => {
        if (!result?.ok) {
          recordRendererTiming({
            kind: "startup",
            name: "startup.schedule-suggest.deferred",
            durationMs: performance.now() - startedAt,
            metadata: {
              outcome: "failed",
              errorCode: result && !result.ok ? result.error.code : null
            }
          });
          return;
        }

        recordRendererTiming({
          kind: "startup",
          name: "startup.schedule-suggest.deferred",
          durationMs: performance.now() - startedAt,
          metadata: {
            outcome: "success",
            slots: result.data.slots.length,
            unscheduled: result.data.unscheduled.length,
            overloadMinutes: result.data.overloadMinutes
          }
        });

        setLoadState((current) => ({
          ...current,
          snapshot: {
            ...current.snapshot,
            scheduleSuggestion: result.data
          }
        }));
      })
      .catch(() => {
        recordRendererTiming({
          kind: "startup",
          name: "startup.schedule-suggest.deferred",
          durationMs: performance.now() - startedAt,
          metadata: {
            outcome: "threw"
          }
        });
      });
  }, [
    loadState.snapshot.settings,
    loadState.state
  ]);

  useEffect(() => {
    return window.hcb?.sync.subscribeStatus((syncStatus) => {
      setLoadState((current) => ({
        ...current,
        snapshot: {
          ...current.snapshot,
          syncStatus
        }
      }));
    });
  }, []);

  useEffect(() => {
    if (
      cachedDataReported.current ||
      (loadState.state !== "ready" && loadState.state !== "empty")
    ) {
      return;
    }

    cachedDataReported.current = true;
    void window.hcb?.diagnostics.markCachedDataRendered();
  }, [loadState.state]);

  const runUndo = useCallback(async () => {
    const result = await window.hcb?.undo.undo();

    if (!result?.ok) {
      return null;
    }

    load();
    return result.data;
  }, [load]);

  const runRedo = useCallback(async () => {
    const result = await window.hcb?.undo.redo();

    if (!result?.ok) {
      return null;
    }

    load();
    return result.data;
  }, [load]);

  return useMemo(
    () => buildCoreViewModelSource(loadState.snapshot, {
      appearanceReady: loadState.appearanceReady,
      state: loadState.state,
      systemPrefersDark: prefersDark,
      errorMessage: loadState.errorMessage,
      hydrationErrorMessage: loadState.hydrationErrorMessage,
      hydrationState: loadState.hydrationState,
      refresh: load,
      refreshGoogleStatus,
      setGoogleStatus,
      ensureCalendarRange,
      taskViewModelCache: taskViewModelCache.current,
      calendarEventViewModelCache: calendarEventViewModelCache.current,
      taskMutation,
      settingsMutation,
      updateSettings,
      createTag,
      updateTag,
      deleteTag,
      mergeTags,
      bulkApplyTags,
      previewAutoTagReapply,
      applyAutoTagReapply,
      tagAnalytics,
      runRecoveryAction,
      undo: runUndo,
      redo: runRedo,
      refreshUndoStatus,
      clearTaskMutationError,
      retryLastTaskMutation,
      createTask,
      updateTask,
      completeTask,
      reopenTask,
      completeEvent,
      reopenEvent,
      moveTask,
      bulkRescheduleTasks,
      deleteTask,
      createTaskList,
      renameTaskList,
      deleteTaskList,
      scheduleTaskBlock,
      moveScheduledTaskBlock,
      unscheduleTaskBlock
    }),
    [
      clearTaskMutationError,
      completeEvent,
      completeTask,
      bulkApplyTags,
      applyAutoTagReapply,
      createTag,
      createTask,
      createTaskList,
      deleteTag,
      deleteTask,
      deleteTaskList,
      ensureCalendarRange,
      load,
      loadState,
      bulkRescheduleTasks,
      moveTask,
      moveScheduledTaskBlock,
      mergeTags,
      previewAutoTagReapply,
      prefersDark,
      refreshGoogleStatus,
      refreshUndoStatus,
      renameTaskList,
      retryLastTaskMutation,
      runRedo,
      runRecoveryAction,
      runUndo,
      reopenEvent,
      reopenTask,
      setGoogleStatus,
      settingsMutation,
      tagAnalytics,
      scheduleTaskBlock,
      taskMutation,
      unscheduleTaskBlock,
      updateSettings,
      updateTag,
      updateTask
    ]
  );
}

function recordRendererTiming(request: {
  kind: "startup" | "cached_render" | "ipc" | "sqlite_query" | "search";
  name: string;
  durationMs: number;
  metadata?: Record<string, string | number | boolean | null>;
}): void {
  void window.hcb?.diagnostics.recordTiming(request);
}
