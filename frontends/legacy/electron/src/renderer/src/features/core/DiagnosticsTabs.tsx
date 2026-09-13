import type {
  DiagnosticsHistoryEntry,
  DiagnosticsLogEntry,
  DiagnosticsLogLevel,
  DiagnosticsLogsResponse,
  DiagnosticsPendingMutation,
  DiagnosticsSummaryResponse,
  SyncStatusResponse
} from "@shared/ipc/contracts";
import {
  ClipboardCopy,
  DatabaseZap,
  FileText,
  Folder,
  Gauge,
  History,
  LifeBuoy,
  RefreshCw,
  RotateCcw,
  Search,
  Trash2,
  X
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import { Badge, Button, IconButton, Input, cx } from "../../components/primitives";
import type { CoreViewModelSource } from "./coreViewModelSource";
import {
  durationText,
  formatDateTime,
  formatTime,
  operationLabel,
  selectionText
} from "./diagnosticsFormatting";

export type DiagnosticsTab = "overview" | "sync" | "logs" | "history" | "support";

export const diagnosticsTabs: Array<{ id: DiagnosticsTab; label: string; icon: LucideIcon }> = [
  { id: "overview", label: "Overview", icon: Gauge },
  { id: "sync", label: "Sync", icon: RefreshCw },
  { id: "logs", label: "Logs", icon: FileText },
  { id: "history", label: "History", icon: History },
  { id: "support", label: "Support", icon: LifeBuoy }
];

const logLevels: DiagnosticsLogLevel[] = ["debug", "info", "warn", "error"];
const authErrorCodes = new Set(["UNAUTHORIZED", "FORBIDDEN"]);

type SyncStateIssue = { id: string; title: string; detail: string; tone: "warning" | "danger" };
type SyncMutationGroup = {
  id: string;
  title: string;
  mutations: DiagnosticsPendingMutation[];
};

export function OverviewTab({
  source,
  summary
}: {
  source: CoreViewModelSource;
  summary: DiagnosticsSummaryResponse | null;
}): JSX.Element {
  const googleLabel =
    source.googleStatus.account?.displayName ??
    source.googleStatus.account?.email ??
    summary?.account.state ??
    "Unknown";
  const credentialStatus = summary?.native.capabilities.find((item) => item.key === "credentialStorage");

  return (
    <div className="grid gap-3">
      <DiagnosticSection title="Status">
        <DiagnosticRow label="Google" value={googleLabel} />
        <DiagnosticRow label="Sync" value={summary?.sync.state ?? source.syncStatus.state} />
        <DiagnosticRow label="Mode" value={source.settings.syncMode} />
        <DiagnosticRow label="Last sync" value={formatDateTime(summary?.sync.lastCompletedAt)} />
        <DiagnosticRow label="Keychain" value={credentialStatus?.state ?? "unknown"} />
      </DiagnosticSection>

      {summary?.performance ? (
        <DiagnosticSection title="Performance">
          <DiagnosticRow label="Last sync" value={durationText(summary.performance.lastSyncDurationMs)} />
          <DiagnosticRow label="Migration" value={durationText(summary.performance.migrationDurationMs)} />
          <DiagnosticRow
            label="Slow queries"
            value={`${summary.performance.slowQuerySamples.length} sample${summary.performance.slowQuerySamples.length === 1 ? "" : "s"}`}
          />
          <DiagnosticRow
            label="MCP requests"
            value={String(summary.performance.mcpRequestCounts.totalRequests)}
          />
        </DiagnosticSection>
      ) : null}

      <DiagnosticSection title="Local data">
        <DiagnosticRow label="Task lists" value={String(summary?.cache.taskListCount ?? source.taskLists.length)} />
        <DiagnosticRow label="Tasks" value={String(summary?.cache.taskCount ?? source.largeTaskWindow.length)} />
        <DiagnosticRow label="Calendars" value={String(summary?.cache.calendarCount ?? source.calendarSources.length)} />
        <DiagnosticRow label="Events" value={String(summary?.cache.eventCount ?? source.calendarAgendaEvents.length)} />
        <DiagnosticRow label="Sync checkpoints" value={String(summary?.checkpoints.totalCount ?? 0)} />
        <DiagnosticRow label="Pending writes" value={String(summary?.pendingMutations.totalCount ?? source.syncStatus.pendingMutationCount)} />
      </DiagnosticSection>

      <DiagnosticSection title="Selections">
        <DiagnosticRow
          label="Selected task lists"
          value={selectionText(summary?.selectedResources.taskLists, source.taskLists.length)}
        />
        <DiagnosticRow
          label="Selected calendars"
          value={selectionText(summary?.selectedResources.calendars, source.calendarSources.length)}
        />
        <DiagnosticRow label="Local reminders" value={source.settings.notificationsEnabled ? "Enabled" : "Disabled"} />
        <DiagnosticRow label="Onboarding" value={source.settings.setupCompletedAt ? "Completed" : "Not completed"} />
      </DiagnosticSection>

      <DiagnosticSection title="Cache">
        <DiagnosticRow
          label="Database"
          value={summary?.native.paths.find((path) => path.role === "data")?.redactedPath ?? "Unavailable"}
          mono
        />
      </DiagnosticSection>
    </div>
  );
}

export function SyncTab({
  copyVisibleSyncIssues,
  pendingMutationsError,
  pendingMutations,
  refresh,
  retryMutation,
  cancelMutation,
  rebuildNotifications,
  runRecovery,
  source,
  summary,
  summaryError,
  working
}: {
  copyVisibleSyncIssues: (text: string) => Promise<void>;
  pendingMutationsError: string | null;
  pendingMutations: DiagnosticsPendingMutation[];
  refresh: () => void;
  retryMutation: (id: string) => Promise<void>;
  cancelMutation: (id: string) => Promise<void>;
  rebuildNotifications: () => Promise<void>;
  runRecovery: (action: "refresh" | "forceFullResync" | "clearGoogleCache") => Promise<void>;
  source: CoreViewModelSource;
  summary: DiagnosticsSummaryResponse | null;
  summaryError: string | null;
  working: boolean;
}): JSX.Element {
  const syncStatus = summary?.sync ?? source.syncStatus;
  const stateIssues = syncStateIssues(syncStatus);
  const retryable = pendingMutations.filter(isRetryableOrAuthPaused);
  const failed = pendingMutations.filter((mutation) => mutation.status === "failed" && !isRetryableOrAuthPaused(mutation));
  const active = pendingMutations.filter((mutation) => mutation.status !== "failed");
  const groups: SyncMutationGroup[] = [
    { id: "failed", title: "Failed mutations", mutations: failed },
    { id: "retryable", title: "Retryable/auth-paused mutations", mutations: retryable },
    { id: "active", title: "Queued/applying mutations", mutations: active }
  ].filter((group) => group.mutations.length > 0);
  const visibleIssueText = syncIssueText(stateIssues, groups);
  const hasIssues = stateIssues.length > 0 || pendingMutations.length > 0;
  const hasError = Boolean(summaryError || pendingMutationsError);

  return (
    <div className="grid gap-3">
      <DiagnosticSection title="Recovery">
        <div className="flex flex-wrap gap-2 p-3">
          <Button disabled={working} onClick={() => void runRecovery("refresh")}>
            <RefreshCw aria-hidden="true" size={15} />
            Refresh now
          </Button>
          <Button disabled={working} onClick={() => void runRecovery("forceFullResync")}>
            <RotateCcw aria-hidden="true" size={15} />
            Force full resync
          </Button>
          <Button disabled={working} onClick={() => void rebuildNotifications()}>
            <Search aria-hidden="true" size={15} />
            Rebuild local reminders
          </Button>
          <Button disabled={working} onClick={() => void runRecovery("clearGoogleCache")} variant="danger">
            <DatabaseZap aria-hidden="true" size={15} />
            Clear cached Google data
          </Button>
          <Button onClick={refresh} variant="ghost">
            <RefreshCw aria-hidden="true" size={15} />
            Refresh diagnostics
          </Button>
        </div>
      </DiagnosticSection>

      <DiagnosticSection
        title="Sync issues"
        trailing={<Badge tone={hasIssues ? "warning" : "neutral"}>{stateIssues.length + pendingMutations.length}</Badge>}
      >
        {hasError ? (
          <div className="grid gap-1 border-b border-border px-3 py-3 text-[var(--text-sm)]">
            <div className="font-semibold text-danger">Sync diagnostics could not load.</div>
            {summaryError ? <div className="text-text-muted">Summary: {summaryError}</div> : null}
            {pendingMutationsError ? <div className="text-text-muted">Pending mutations: {pendingMutationsError}</div> : null}
          </div>
        ) : null}

        {!hasError && !hasIssues ? (
          <p className="px-3 py-4 text-[var(--text-sm)] text-text-muted">No sync issues found.</p>
        ) : null}

        {stateIssues.length > 0 ? (
          <div className="grid">
            {stateIssues.map((issue) => (
              <div className="flex items-start justify-between gap-3 border-b border-border px-3 py-2" key={issue.id}>
                <div className="min-w-0">
                  <div className="text-[var(--text-sm)] font-semibold text-text-primary">{issue.title}</div>
                  <div className="text-[var(--text-xs)] text-text-muted">{issue.detail}</div>
                </div>
                <Badge tone={issue.tone}>{issue.id}</Badge>
              </div>
            ))}
          </div>
        ) : null}

        {groups.length > 0 ? (
          <div className="grid">
            {groups.map((group) => (
              <div className="grid" key={group.id}>
                <div className="border-b border-border bg-bg-secondary px-3 py-1.5 text-[var(--text-xs)] font-semibold uppercase text-text-muted">
                  {group.title}
                </div>
                {group.mutations.map((mutation) => (
                  <div className="grid gap-2 border-b border-border px-3 py-2 last:border-b-0" key={mutation.id}>
                    <div className="flex min-w-0 items-start justify-between gap-3">
                      <div className="min-w-0">
                        <div className="truncate font-mono text-[var(--text-sm)] text-text-primary">
                          {operationLabel(mutation.operation)} · {mutation.resourceType}:{mutation.resourceId}
                        </div>
                        <div className="truncate font-mono text-[var(--text-xs)] text-text-muted">
                          {formatDateTime(mutation.createdAt)} · {mutation.attemptCount} attempts
                        </div>
                        {mutation.lastErrorMessage ? (
                          <div className="text-[var(--text-xs)] text-danger">{mutation.lastErrorMessage}</div>
                        ) : null}
                      </div>
                      <Badge tone={mutation.status === "failed" ? "danger" : "warning"}>{mutation.status}</Badge>
                    </div>
                    <div className="flex flex-wrap gap-2">
                      <IconButton icon={RefreshCw} label="Retry pending mutation" onClick={() => void retryMutation(mutation.id)} variant="ghost" />
                      <IconButton icon={ClipboardCopy} label="Copy pending mutation" onClick={() => void navigator.clipboard?.writeText(JSON.stringify(mutation, null, 2))} variant="ghost" />
                      <IconButton icon={X} label="Cancel pending mutation" onClick={() => void cancelMutation(mutation.id)} variant="ghost" />
                    </div>
                  </div>
                ))}
              </div>
            ))}
          </div>
        ) : null}

        {hasIssues ? (
          <div className="flex justify-end border-t border-border px-3 py-2">
            <Button onClick={() => void copyVisibleSyncIssues(visibleIssueText)}>
              <ClipboardCopy aria-hidden="true" size={15} />
              Copy visible sync issues
            </Button>
          </div>
        ) : null}
      </DiagnosticSection>
    </div>
  );
}

function syncStateIssues(syncStatus: SyncStatusResponse): SyncStateIssue[] {
  const issues: SyncStateIssue[] = [];

  if (syncStatus.offline) {
    issues.push({
      id: "offline",
      title: "Sync offline",
      detail: "Google sync is reporting offline mode.",
      tone: "warning"
    });
  }

  if (syncStatus.stale) {
    issues.push({
      id: "stale",
      title: "Cache is stale",
      detail: "Local data may be older than the latest Google state.",
      tone: "warning"
    });
  }

  if (syncStatus.state === "error") {
    issues.push({
      id: "error",
      title: "Sync error",
      detail: syncStatus.lastErrorCode ? `Last error: ${syncStatus.lastErrorCode}` : "Sync ended in an error state.",
      tone: "danger"
    });
  }

  return issues;
}

function isRetryableOrAuthPaused(mutation: DiagnosticsPendingMutation): boolean {
  return mutation.status === "failed" && (
    mutation.nextRetryAt !== null ||
    (mutation.lastErrorCode !== null && authErrorCodes.has(mutation.lastErrorCode))
  );
}

function syncIssueText(stateIssues: SyncStateIssue[], groups: SyncMutationGroup[]): string {
  return [
    ...stateIssues.map((issue) => `[state:${issue.id}] ${issue.title} - ${issue.detail}`),
    ...groups.flatMap((group) =>
      group.mutations.map((mutation) =>
        [
          `[${group.id}]`,
          `${operationLabel(mutation.operation)} ${mutation.resourceType}:${mutation.resourceId}`,
          `status=${mutation.status}`,
          `attempts=${mutation.attemptCount}`,
          mutation.nextRetryAt === null ? null : `nextRetryAt=${mutation.nextRetryAt}`,
          mutation.lastErrorCode === null ? null : `error=${mutation.lastErrorCode}`,
          mutation.lastErrorMessage === null ? null : mutation.lastErrorMessage
        ]
          .filter((part): part is string => part !== null)
          .join(" ")
      )
    )
  ].join("\n");
}

export function LogsTab({
  clearLogs,
  copyLogs,
  copyVisibleLogs,
  filteredLogs,
  logLevel,
  logQuery,
  logs,
  refreshLogs,
  revealLogsFolder,
  setLogLevel,
  setLogQuery
}: {
  clearLogs: () => Promise<void>;
  copyLogs: () => Promise<void>;
  copyVisibleLogs: () => Promise<void>;
  filteredLogs: DiagnosticsLogEntry[];
  logLevel: DiagnosticsLogLevel;
  logQuery: string;
  logs: DiagnosticsLogsResponse | null;
  refreshLogs: () => void;
  revealLogsFolder: () => Promise<void>;
  setLogLevel: (level: DiagnosticsLogLevel) => void;
  setLogQuery: (query: string) => void;
}): JSX.Element {
  return (
    <div className="grid min-h-[55vh] grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden rounded-hcbMd border border-border bg-bg-secondary">
      <div className="grid gap-2 border-b border-border p-3">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-[var(--text-sm)] font-semibold">Level</span>
          <div className="flex overflow-hidden rounded-hcbMd border border-border bg-surface-0">
            {logLevels.map((level) => (
              <button
                className={cx(
                  "h-7 px-2.5 text-[var(--text-sm)] font-semibold transition-colors duration-fast ease-hcb focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent",
                  logLevel === level ? "bg-accent text-bg-tertiary" : "text-text-secondary hover:bg-surface-1"
                )}
                key={level}
                onClick={() => setLogLevel(level)}
                type="button"
              >
                {level[0].toUpperCase()}{level.slice(1)}
              </button>
            ))}
          </div>
          <div className="flex-1" />
          <Button disabled={filteredLogs.length === 0} onClick={() => void copyVisibleLogs()}>
            <ClipboardCopy aria-hidden="true" size={15} />
            Copy visible
          </Button>
          <Button disabled={(logs?.entries.length ?? 0) === 0} onClick={() => void copyLogs()}>
            <ClipboardCopy aria-hidden="true" size={15} />
            Copy all
          </Button>
          <Button onClick={refreshLogs}>
            <RefreshCw aria-hidden="true" size={15} />
            Refresh
          </Button>
        </div>
        <SearchInput label="Search logs" value={logQuery} onChange={setLogQuery} />
      </div>

      <div className="min-h-0 overflow-auto bg-bg-secondary">
        {filteredLogs.length === 0 ? (
          <p className="p-4 text-[var(--text-sm)] text-text-muted">No log entries match this view.</p>
        ) : (
          filteredLogs.map((entry) => <LogRow entry={entry} key={entry.id} />)
        )}
      </div>

      <div className="flex flex-wrap items-center gap-2 border-t border-border px-3 py-2">
        <span className="text-[var(--text-sm)] text-text-muted">
          {filteredLogs.length} of {logs?.entries.length ?? 0} shown
        </span>
        <div className="flex-1" />
        <Button disabled={(logs?.entries.length ?? 0) === 0} onClick={() => void copyLogs()}>
          <ClipboardCopy aria-hidden="true" size={15} />
          Copy all logs
        </Button>
        <Button onClick={() => void revealLogsFolder()}>
          <Folder aria-hidden="true" size={15} />
          Reveal logs folder
        </Button>
        <Button onClick={() => void clearLogs()} variant="danger">
          <Trash2 aria-hidden="true" size={15} />
          Clear logs
        </Button>
      </div>
    </div>
  );
}

export function HistoryTab({
  copyHistory,
  filteredHistory,
  historyEntries,
  historyError,
  historyQuery,
  setHistoryQuery
}: {
  copyHistory: () => Promise<void>;
  filteredHistory: DiagnosticsHistoryEntry[];
  historyEntries: DiagnosticsHistoryEntry[];
  historyError: string | null;
  historyQuery: string;
  setHistoryQuery: (query: string) => void;
}): JSX.Element {
  return (
    <div className="grid min-h-[55vh] grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden rounded-hcbMd border border-border bg-bg-secondary">
      <div className="grid gap-2 border-b border-border p-3">
        <div className="flex items-center justify-between gap-3">
          <h3 className="text-[var(--text-md)] font-semibold">Mutation history</h3>
          <span className="text-[var(--text-sm)] text-text-muted">{historyEntries.length} retained</span>
        </div>
        <SearchInput label="Search history" value={historyQuery} onChange={setHistoryQuery} />
      </div>
      <div className="min-h-0 overflow-auto">
        {historyError ? (
          <div className="grid gap-1 p-4 text-[var(--text-sm)]">
            <div className="font-semibold text-danger">History could not load.</div>
            <div className="text-text-muted">{historyError}</div>
          </div>
        ) : historyEntries.length === 0 ? (
          <p className="p-4 text-[var(--text-sm)] text-text-muted">No history entries recorded.</p>
        ) : filteredHistory.length === 0 ? (
          <p className="p-4 text-[var(--text-sm)] text-text-muted">No history entries match this search.</p>
        ) : (
          filteredHistory.map((entry) => <HistoryRow entry={entry} key={entry.id} />)
        )}
      </div>
      <div className="flex flex-wrap items-center gap-2 border-t border-border px-3 py-2">
        <span className="text-[var(--text-sm)] text-text-muted">
          {filteredHistory.length} of {historyEntries.length} shown
        </span>
        <div className="flex-1" />
        <Button disabled={filteredHistory.length === 0} onClick={() => void copyHistory()}>
          <ClipboardCopy aria-hidden="true" size={15} />
          Copy visible history
        </Button>
      </div>
    </div>
  );
}

export function SupportTab({
  copyDiagnosticSummary,
  exportBundle,
  summary
}: {
  copyDiagnosticSummary: () => Promise<void>;
  exportBundle: () => Promise<void>;
  summary: DiagnosticsSummaryResponse | null;
}): JSX.Element {
  return (
    <div className="grid gap-3">
      <DiagnosticSection title="Support">
        <div className="flex flex-wrap gap-2 p-3">
          <Button onClick={() => void copyDiagnosticSummary()}>
            <ClipboardCopy aria-hidden="true" size={15} />
            Copy diagnostic summary
          </Button>
          <Button onClick={() => void exportBundle()}>
            <Folder aria-hidden="true" size={15} />
            Export diagnostic bundle...
          </Button>
        </div>
      </DiagnosticSection>
      <DiagnosticSection title="Redaction">
        <DiagnosticRow label="Credentials" value={summary?.redaction.credentials ?? "redacted"} />
        <DiagnosticRow label="Google payloads" value={summary?.redaction.googlePayloads ?? "omitted"} />
        <DiagnosticRow label="MCP bearer tokens" value={summary?.redaction.mcpBearerTokens ?? "redacted"} />
        <DiagnosticRow label="Sensitive bodies" value={summary?.redaction.sensitiveBodies ?? "omitted"} />
      </DiagnosticSection>
    </div>
  );
}

function DiagnosticSection({
  children,
  title,
  trailing
}: {
  children: ReactNode;
  title: string;
  trailing?: React.ReactNode;
}): JSX.Element {
  return (
    <section className="overflow-hidden rounded-hcbMd border border-border bg-bg-secondary">
      <div className="flex min-h-9 items-center justify-between gap-3 border-b border-border px-3 py-2">
        <h3 className="text-[var(--text-md)] font-semibold text-text-primary">{title}</h3>
        {trailing}
      </div>
      <div>{children}</div>
    </section>
  );
}

function DiagnosticRow({
  label,
  value,
  mono = false
}: {
  label: string;
  value: string;
  mono?: boolean;
}): JSX.Element {
  return (
    <div className="flex min-h-8 items-center justify-between gap-4 border-b border-border px-3 py-1.5 last:border-b-0">
      <span className="min-w-0 truncate text-[var(--text-sm)] font-medium text-text-muted">{label}</span>
      <span className={cx("min-w-0 truncate text-right text-[var(--text-sm)] font-medium text-text-primary", mono && "font-mono")}>
        {value}
      </span>
    </div>
  );
}

function SearchInput({
  label,
  onChange,
  value
}: {
  label: string;
  onChange: (value: string) => void;
  value: string;
}): JSX.Element {
  return (
    <div className="relative">
      <Search
        aria-hidden="true"
        className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-text-muted"
        size={15}
      />
      <Input
        aria-label={label}
        className="pl-9"
        onChange={(event) => onChange(event.currentTarget.value)}
        placeholder={label}
        value={value}
      />
    </div>
  );
}

function LogRow({ entry }: { entry: DiagnosticsLogEntry }): JSX.Element {
  return (
    <div className="flex gap-3 border-b border-border px-3 py-2 last:border-b-0">
      <Badge tone={entry.level === "error" ? "danger" : entry.level === "warn" ? "warning" : "info"}>
        {entry.level}
      </Badge>
      <div className="min-w-0 flex-1">
        <div className="truncate font-mono text-[var(--text-sm)]">
          [{entry.category}] {entry.message}
        </div>
        <div className="truncate font-mono text-[var(--text-xs)] text-text-muted">
          {formatTime(entry.timestamp)} {entry.metadataLine ?? ""}
        </div>
      </div>
    </div>
  );
}

function HistoryRow({ entry }: { entry: DiagnosticsHistoryEntry }): JSX.Element {
  return (
    <div className="flex gap-3 border-b border-border px-3 py-2 last:border-b-0">
      <Badge tone={entry.kind.includes("delete") || entry.kind.includes("cancel") ? "danger" : "success"}>
        {entry.kind.split(".")[0]}
      </Badge>
      <div className="min-w-0 flex-1">
        <div className="truncate font-mono text-[var(--text-sm)]">{entry.summary}</div>
        <div className="truncate font-mono text-[var(--text-xs)] text-text-muted">
          {entry.kind} · {formatDateTime(entry.timestamp)} {entry.metadataLine ?? ""}
        </div>
      </div>
    </div>
  );
}
