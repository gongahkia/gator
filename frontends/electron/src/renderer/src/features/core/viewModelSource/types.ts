import type {
  CalendarEventSummary,
  CalendarEventCompletionScope,
  CalendarListSummary,
  CalendarScheduleSuggestResponse,
  DiagnosticsHealthResponse,
  DiagnosticsSummaryResponse,
  GoogleStatusResponse,
  NativeCapabilitiesResponse,
  NoteListSummary,
  NoteSummary,
  ScheduledTaskBlockCreateRequest,
  ScheduledTaskBlockMoveRequest,
  ScheduledTaskBlockSummary,
  SettingsRecoveryActionRequest,
  SettingsRecoveryActionResponse,
  SettingsSnapshot,
  SettingsUpdateRequest,
  SyncStatusResponse,
  AutoTagReapplyApplyRequest,
  AutoTagReapplyApplyResponse,
  AutoTagReapplyPreviewRequest,
  AutoTagReapplyPreviewResponse,
  TagAnalyticsResponse,
  TagBulkApplyRequest,
  TagCreateRequest,
  TagDeleteRequest,
  TagListResponse,
  TagMergeRequest,
  TagMutationResponse,
  TagSummary,
  TagUpdateRequest,
  UndoApplyResponse,
  UndoStackStatusResponse,
  TaskCreateRequest,
  TaskBulkRescheduleRequest,
  TaskListCreateRequest,
  TaskListRenameRequest,
  TaskListSummary,
  TaskMoveRequest,
  TaskSummary,
  TaskUpdateRequest
} from "@shared/ipc/contracts";
import type { ColorThemeDefinition } from "@shared/ipc/themeCatalog";
import type { ParsedLocalSearchQuery } from "@shared/search/localSearch";
import type {
  CalendarDayViewModel,
  CalendarEventViewModel,
  CalendarMonthWeekViewModel,
  NoteViewModel,
  SearchViewModel,
  ScheduledTaskBlockViewModel,
  SettingsSectionViewModel,
  TaskFilterId,
  TaskFilterViewModel,
  TaskViewModel
} from "../coreViewModels";

export interface CoreViewModelSource {
  activeColorTheme: ColorThemeDefinition;
  activeColorThemeId: ColorThemeDefinition["id"];
  appearanceReady: boolean;
  calendarAgendaEvents: CalendarEventViewModel[];
  calendarDayView: CalendarDayViewModel;
  calendarEventsById: Record<string, CalendarEventViewModel>;
  calendarMonthWeeks: CalendarMonthWeekViewModel[];
  calendarSources: CalendarListSummary[];
  calendarWeekDays: CalendarDayViewModel[];
  dataState: CoreDataState;
  errorMessage?: string;
  getSearchViewModel: (query: string) => SearchViewModel;
  getTaskById: (taskId: string) => TaskViewModel;
  getScheduledTaskBlockById: (blockId: string) => ScheduledTaskBlockViewModel | null;
  getTaskFilterViewModel: (filterId: TaskFilterId) => TaskFilterViewModel;
  hasCachedData: boolean;
  hydrationErrorMessage?: string;
  hydrationState: CoreHydrationState;
  initialNotes: NoteViewModel[];
  noteLists: NoteListSummary[];
  tags: TagSummary[];
  ensureCalendarRange: (range: CalendarRangeLoadRequest) => Promise<boolean>;
  isOffline: boolean;
  isStale: boolean;
  largeTaskWindow: TaskViewModel[];
  refresh: () => void;
  refreshGoogleStatus: () => void;
  resourceCounts: CoreResourceCounts;
  setGoogleStatus: (status: GoogleStatusResponse) => void;
  settings: SettingsSnapshot;
  diagnosticsSummary?: DiagnosticsSummaryResponse;
  googleStatus: GoogleStatusResponse;
  undoStatus: UndoStackStatusResponse;
  native: NativeCapabilitiesResponse;
  settingsMutationError?: string;
  settingsMutationPending: boolean;
  updateSettings: (request: SettingsUpdateRequest) => Promise<boolean>;
  createTag: (request: TagCreateRequest) => Promise<TagMutationResponse | null>;
  updateTag: (request: TagUpdateRequest) => Promise<TagMutationResponse | null>;
  deleteTag: (request: TagDeleteRequest) => Promise<TagMutationResponse | null>;
  mergeTags: (request: TagMergeRequest) => Promise<TagMutationResponse | null>;
  bulkApplyTags: (request: TagBulkApplyRequest) => Promise<TagMutationResponse | null>;
  previewAutoTagReapply: (
    request: AutoTagReapplyPreviewRequest
  ) => Promise<AutoTagReapplyPreviewResponse | null>;
  applyAutoTagReapply: (
    request: AutoTagReapplyApplyRequest
  ) => Promise<AutoTagReapplyApplyResponse | null>;
  tagAnalytics: () => Promise<TagAnalyticsResponse | null>;
  runRecoveryAction: (
    request: SettingsRecoveryActionRequest
  ) => Promise<SettingsRecoveryActionResponse | null>;
  undo: () => Promise<UndoApplyResponse | null>;
  redo: () => Promise<UndoApplyResponse | null>;
  refreshUndoStatus: () => void;
  taskMutationError?: string;
  taskMutationPending: boolean;
  clearTaskMutationError: () => void;
  retryLastTaskMutation: () => void;
  createTask: (request: TaskCreateRequest) => Promise<boolean>;
  updateTask: (request: TaskUpdateRequest) => Promise<boolean>;
  completeTask: (taskId: string) => Promise<boolean>;
  reopenTask: (taskId: string) => Promise<boolean>;
  completeEvent: (eventId: string, scope?: CalendarEventCompletionScope) => Promise<boolean>;
  reopenEvent: (eventId: string, scope?: CalendarEventCompletionScope) => Promise<boolean>;
  moveTask: (request: TaskMoveRequest) => Promise<boolean>;
  bulkRescheduleTasks: (request: TaskBulkRescheduleRequest) => Promise<boolean>;
  deleteTask: (taskId: string) => Promise<boolean>;
  createTaskList: (request: TaskListCreateRequest) => Promise<boolean>;
  renameTaskList: (request: TaskListRenameRequest) => Promise<boolean>;
  deleteTaskList: (taskListId: string) => Promise<boolean>;
  scheduleTaskBlock: (request: ScheduledTaskBlockCreateRequest) => Promise<boolean>;
  moveScheduledTaskBlock: (request: ScheduledTaskBlockMoveRequest) => Promise<boolean>;
  unscheduleTaskBlock: (blockId: string) => Promise<boolean>;
  scheduledTaskBlocks: ScheduledTaskBlockViewModel[];
  settingsSections: SettingsSectionViewModel[];
  syncStatus: SyncStatusResponse;
  taskFilterViewModels: TaskFilterViewModel[];
  taskLists: TaskListSummary[];
}

export interface CalendarRangeLoadRequest {
  start: string;
  end: string;
}

export type CoreDataState = "loading" | "ready" | "empty" | "error" | "offline" | "stale";

export interface CoreDataSnapshot {
  taskLists: TaskListSummary[];
  tasks: TaskSummary[];
  calendars: CalendarListSummary[];
  events: CalendarEventSummary[];
  scheduledTaskBlocks: ScheduledTaskBlockSummary[];
  scheduleSuggestion: CalendarScheduleSuggestResponse;
  notes: NoteSummary[];
  noteLists: NoteListSummary[];
  tags: TagSummary[];
  settings: SettingsSnapshot;
  syncStatus: SyncStatusResponse;
  googleStatus: GoogleStatusResponse;
  undoStatus: UndoStackStatusResponse;
  health?: DiagnosticsHealthResponse;
  native: NativeCapabilitiesResponse;
  diagnosticsSummary?: DiagnosticsSummaryResponse;
  resourceCounts: CoreResourceCounts;
}

export interface CoreResourceCounts {
  calendarEvents: number | null;
  notes: number | null;
  tasks: number | null;
}

export type CoreHydrationState = "idle" | "loading" | "success" | "failed";

export interface CoreDataLoadState {
  snapshot: CoreDataSnapshot;
  state: CoreDataState;
  appearanceReady: boolean;
  errorMessage?: string;
  hydrationErrorMessage?: string;
  hydrationState: CoreHydrationState;
}

export interface SearchHookState {
  viewModel: SearchViewModel;
  state: "idle" | "loading" | "results" | "empty" | "error" | "offline" | "stale" | "invalid";
  parsed: ParsedLocalSearchQuery;
  errorMessage?: string;
  latencyMs?: number;
}

export interface TaskMutationUiState {
  pending: boolean;
  error?: string;
}

export type SettingsMutationUiState = TaskMutationUiState;

export interface CalendarEventDayIndex {
  eventsByDay: Map<string, CalendarEventViewModel[]>;
}

export interface CoreViewModelSourceOptions {
  appearanceReady: boolean;
  state: CoreDataState;
  systemPrefersDark: boolean;
  errorMessage?: string;
  hydrationErrorMessage?: string;
  hydrationState: CoreHydrationState;
  refresh: () => void;
  refreshGoogleStatus: () => void;
  setGoogleStatus: (status: GoogleStatusResponse) => void;
  ensureCalendarRange: (range: CalendarRangeLoadRequest) => Promise<boolean>;
  taskViewModelCache: Map<string, { signature: string; viewModel: TaskViewModel }>;
  calendarEventViewModelCache: Map<string, { signature: string; viewModel: CalendarEventViewModel }>;
  taskMutation: TaskMutationUiState;
  settingsMutation: SettingsMutationUiState;
  updateSettings: (request: SettingsUpdateRequest) => Promise<boolean>;
  createTag: (request: TagCreateRequest) => Promise<TagMutationResponse | null>;
  updateTag: (request: TagUpdateRequest) => Promise<TagMutationResponse | null>;
  deleteTag: (request: TagDeleteRequest) => Promise<TagMutationResponse | null>;
  mergeTags: (request: TagMergeRequest) => Promise<TagMutationResponse | null>;
  bulkApplyTags: (request: TagBulkApplyRequest) => Promise<TagMutationResponse | null>;
  previewAutoTagReapply: (
    request: AutoTagReapplyPreviewRequest
  ) => Promise<AutoTagReapplyPreviewResponse | null>;
  applyAutoTagReapply: (
    request: AutoTagReapplyApplyRequest
  ) => Promise<AutoTagReapplyApplyResponse | null>;
  tagAnalytics: () => Promise<TagAnalyticsResponse | null>;
  runRecoveryAction: (
    request: SettingsRecoveryActionRequest
  ) => Promise<SettingsRecoveryActionResponse | null>;
  undo: () => Promise<UndoApplyResponse | null>;
  redo: () => Promise<UndoApplyResponse | null>;
  refreshUndoStatus: () => void;
  clearTaskMutationError: () => void;
  retryLastTaskMutation: () => void;
  createTask: (request: TaskCreateRequest) => Promise<boolean>;
  updateTask: (request: TaskUpdateRequest) => Promise<boolean>;
  completeTask: (taskId: string) => Promise<boolean>;
  reopenTask: (taskId: string) => Promise<boolean>;
  completeEvent: (eventId: string, scope?: CalendarEventCompletionScope) => Promise<boolean>;
  reopenEvent: (eventId: string, scope?: CalendarEventCompletionScope) => Promise<boolean>;
  moveTask: (request: TaskMoveRequest) => Promise<boolean>;
  bulkRescheduleTasks: (request: TaskBulkRescheduleRequest) => Promise<boolean>;
  deleteTask: (taskId: string) => Promise<boolean>;
  createTaskList: (request: TaskListCreateRequest) => Promise<boolean>;
  renameTaskList: (request: TaskListRenameRequest) => Promise<boolean>;
  deleteTaskList: (taskListId: string) => Promise<boolean>;
  scheduleTaskBlock: (request: ScheduledTaskBlockCreateRequest) => Promise<boolean>;
  moveScheduledTaskBlock: (request: ScheduledTaskBlockMoveRequest) => Promise<boolean>;
  unscheduleTaskBlock: (blockId: string) => Promise<boolean>;
}
