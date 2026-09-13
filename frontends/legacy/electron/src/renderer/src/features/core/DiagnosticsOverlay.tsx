import { Gauge, X } from "lucide-react";
import { IconButton, StatusBanner, cx } from "../../components/primitives";
import {
  type DiagnosticsTab,
  diagnosticsTabs,
  HistoryTab,
  LogsTab,
  OverviewTab,
  SupportTab,
  SyncTab
} from "./DiagnosticsTabs";
import { useDiagnosticsOverlay } from "./useDiagnosticsOverlay";

interface DiagnosticsOverlayProps {
  initialTab?: DiagnosticsTab;
  onClose: () => void;
}

export function DiagnosticsOverlay({ initialTab = "overview", onClose }: DiagnosticsOverlayProps): JSX.Element {
  const {
    cancelMutation,
    clearLogs,
    copyDiagnosticSummary,
    copyHistory,
    copyLogs,
    copyVisibleSyncIssues,
    copyVisibleLogs,
    dialogRef,
    exportBundle,
    filteredHistory,
    filteredLogs,
    historyEntries,
    historyError,
    historyQuery,
    logLevel,
    logQuery,
    logs,
    message,
    pendingMutationsError,
    pendingMutations,
    rebuildNotifications,
    refreshLogs,
    refreshSyncDiagnostics,
    revealLogsFolder,
    retryMutation,
    runRecovery,
    setHistoryQuery,
    setLogLevel,
    setLogQuery,
    setTab,
    source,
    summary,
    summaryError,
    tab,
    working
  } = useDiagnosticsOverlay(onClose, initialTab);

  return (
    <div
      className="fixed inset-0 z-50 grid place-items-center overflow-auto bg-bg-tertiary/45 p-3 backdrop-blur-sm sm:p-5"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) {
          onClose();
        }
      }}
    >
      <section
        aria-labelledby="diagnostics-overlay-title"
        aria-modal="true"
        className="flex max-h-[calc(100dvh-24px)] w-full max-w-[1180px] flex-col overflow-hidden rounded-hcbLg border border-border bg-bg-primary shadow-2xl sm:max-h-[calc(100dvh-72px)]"
        ref={dialogRef}
        role="dialog"
        tabIndex={-1}
      >
        <header className="flex min-h-12 items-center justify-between gap-3 border-b border-border bg-bg-primary px-3 py-2">
          <div className="flex min-w-0 items-center gap-2.5">
            <div className="flex size-7 shrink-0 items-center justify-center rounded-hcbSm bg-surface-0 text-accent">
              <Gauge aria-hidden="true" size={14} strokeWidth={2} />
            </div>
            <div className="min-w-0">
              <h2 className="truncate text-[var(--text-md)] font-semibold text-text-primary" id="diagnostics-overlay-title">
                Diagnostics
              </h2>
              <p className="truncate text-[var(--text-xs)] text-text-muted">
                Runtime state, logs, sync queue, and support bundle
              </p>
            </div>
          </div>
          <IconButton icon={X} label="Close diagnostics" onClick={onClose} variant="ghost" />
        </header>

        <div className="flex min-w-0 items-center gap-1 overflow-x-auto border-b border-border bg-bg-primary px-3 py-2">
          {diagnosticsTabs.map((entry) => {
            const Icon = entry.icon;
            const selected = tab === entry.id;

            return (
              <button
                aria-pressed={selected}
                className={cx(
                  "inline-flex h-8 shrink-0 items-center justify-center gap-2 rounded-hcbMd border px-2.5 text-[var(--text-base)] font-medium transition-colors duration-fast ease-hcb focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent",
                  selected
                    ? "border-border bg-surface-0 text-text-primary"
                    : "border-transparent text-text-muted hover:bg-surface-0 hover:text-text-primary"
                )}
                key={entry.id}
                onClick={() => setTab(entry.id)}
                type="button"
              >
                <Icon aria-hidden="true" className="shrink-0" size={14} strokeWidth={2} />
                <span className="truncate">{entry.label}</span>
              </button>
            );
          })}
        </div>

        {message ? (
          <StatusBanner
            className="m-3 mb-0"
            description={message.text}
            title={message.tone === "success" ? "Diagnostics action applied" : "Diagnostics notice"}
            tone={message.tone}
          />
        ) : null}

        <div className="min-h-0 flex-1 overflow-auto p-3">
          {tab === "overview" ? (
            <OverviewTab source={source} summary={summary} />
          ) : null}
          {tab === "sync" ? (
            <SyncTab
              copyVisibleSyncIssues={copyVisibleSyncIssues}
              pendingMutationsError={pendingMutationsError}
              pendingMutations={pendingMutations}
              refresh={() => void refreshSyncDiagnostics()}
              source={source}
              summary={summary}
              summaryError={summaryError}
              retryMutation={retryMutation}
              cancelMutation={cancelMutation}
              rebuildNotifications={rebuildNotifications}
              runRecovery={runRecovery}
              working={working}
            />
          ) : null}
          {tab === "logs" ? (
            <LogsTab
              clearLogs={clearLogs}
              copyLogs={copyLogs}
              copyVisibleLogs={copyVisibleLogs}
              filteredLogs={filteredLogs}
              logLevel={logLevel}
              logQuery={logQuery}
              logs={logs}
              refreshLogs={() => void refreshLogs()}
              revealLogsFolder={revealLogsFolder}
              setLogLevel={setLogLevel}
              setLogQuery={setLogQuery}
            />
          ) : null}
          {tab === "history" ? (
            <HistoryTab
              copyHistory={copyHistory}
              filteredHistory={filteredHistory}
              historyEntries={historyEntries}
              historyError={historyError}
              historyQuery={historyQuery}
              setHistoryQuery={setHistoryQuery}
            />
          ) : null}
          {tab === "support" ? (
            <SupportTab
              copyDiagnosticSummary={copyDiagnosticSummary}
              exportBundle={exportBundle}
              summary={summary}
            />
          ) : null}
        </div>
      </section>
    </div>
  );
}
