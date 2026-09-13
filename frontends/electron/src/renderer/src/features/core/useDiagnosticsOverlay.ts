import { useEffect, useMemo, useRef, useState } from "react";
import type { MutableRefObject } from "react";
import type {
  DiagnosticsHistoryEntry,
  DiagnosticsLogEntry,
  DiagnosticsLogLevel,
  DiagnosticsLogsResponse,
  DiagnosticsPendingMutation,
  DiagnosticsSummaryResponse
} from "@shared/ipc/contracts";
import { useCoreViewModelSource } from "./coreViewModelSource";
import type { DiagnosticsTab } from "./DiagnosticsTabs";
import { formatDateTime } from "./diagnosticsFormatting";

type DiagnosticsMessage = { tone: "success" | "warning" | "danger"; text: string };
const unavailableMessage = "Diagnostics preload bridge is unavailable.";

export function useDiagnosticsOverlay(onClose: () => void, initialTab: DiagnosticsTab = "overview"): {
  cancelMutation: (id: string) => Promise<void>;
  clearLogs: () => Promise<void>;
  copyDiagnosticSummary: () => Promise<void>;
  copyHistory: () => Promise<void>;
  copyLogs: () => Promise<void>;
  copyVisibleSyncIssues: (text: string) => Promise<void>;
  copyVisibleLogs: () => Promise<void>;
  dialogRef: MutableRefObject<HTMLElement | null>;
  exportBundle: () => Promise<void>;
  filteredHistory: DiagnosticsHistoryEntry[];
  filteredLogs: DiagnosticsLogEntry[];
  historyEntries: DiagnosticsHistoryEntry[];
  historyError: string | null;
  historyQuery: string;
  logLevel: DiagnosticsLogLevel;
  logQuery: string;
  logs: DiagnosticsLogsResponse | null;
  message: DiagnosticsMessage | null;
  pendingMutationsError: string | null;
  pendingMutations: DiagnosticsPendingMutation[];
  rebuildNotifications: () => Promise<void>;
  refreshLogs: () => Promise<void>;
  refreshSyncDiagnostics: () => Promise<void>;
  revealLogsFolder: () => Promise<void>;
  retryMutation: (id: string) => Promise<void>;
  runRecovery: (action: "refresh" | "forceFullResync" | "clearGoogleCache") => Promise<void>;
  setHistoryQuery: (query: string) => void;
  setLogLevel: (level: DiagnosticsLogLevel) => void;
  setLogQuery: (query: string) => void;
  setTab: (tab: DiagnosticsTab) => void;
  source: ReturnType<typeof useCoreViewModelSource>;
  summary: DiagnosticsSummaryResponse | null;
  summaryError: string | null;
  tab: DiagnosticsTab;
  working: boolean;
} {
  const source = useCoreViewModelSource();
  const dialogRef = useRef<HTMLElement | null>(null);
  const [tab, setTab] = useState<DiagnosticsTab>(initialTab);
  const [summary, setSummary] = useState<DiagnosticsSummaryResponse | null>(
    source.diagnosticsSummary ?? null
  );
  const [logs, setLogs] = useState<DiagnosticsLogsResponse | null>(null);
  const [historyEntries, setHistoryEntries] = useState<DiagnosticsHistoryEntry[]>([]);
  const [pendingMutations, setPendingMutations] = useState<DiagnosticsPendingMutation[]>([]);
  const [summaryError, setSummaryError] = useState<string | null>(null);
  const [historyError, setHistoryError] = useState<string | null>(null);
  const [pendingMutationsError, setPendingMutationsError] = useState<string | null>(null);
  const [logLevel, setLogLevel] = useState<DiagnosticsLogLevel>("info");
  const [logQuery, setLogQuery] = useState("");
  const [historyQuery, setHistoryQuery] = useState("");
  const [message, setMessage] = useState<DiagnosticsMessage | null>(null);
  const [working, setWorking] = useState(false);

  useEffect(() => {
    dialogRef.current?.focus();
    void refreshAll();

    function handleKeyDown(event: KeyboardEvent): void {
      if (event.key === "Escape") {
        event.preventDefault();
        onClose();
      }
    }

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  useEffect(() => {
    setTab(initialTab);
  }, [initialTab]);

  useEffect(() => {
    void refreshLogs();
  }, [logLevel]);

  const filteredLogs = useMemo(() => {
    const query = logQuery.trim().toLowerCase();
    const entries = logs?.entries ?? [];

    if (!query) {
      return entries;
    }

    return entries.filter((entry) =>
      [entry.timestamp, entry.level, entry.category, entry.message, entry.metadataLine ?? ""]
        .join("\n")
        .toLowerCase()
        .includes(query)
    );
  }, [logQuery, logs?.entries]);

  const filteredHistory = useMemo(() => {
    const query = historyQuery.trim().toLowerCase();

    if (!query) {
      return historyEntries;
    }

    return historyEntries.filter((entry) =>
      [entry.timestamp, entry.kind, entry.summary, entry.metadataLine ?? ""]
        .join("\n")
        .toLowerCase()
        .includes(query)
    );
  }, [historyEntries, historyQuery]);

  async function refreshAll(): Promise<void> {
    await Promise.all([refreshSummary(), refreshLogs(), refreshHistory(), refreshPendingMutations()]);
  }

  async function refreshSummary(): Promise<void> {
    try {
      const result = await window.hcb?.diagnostics.summary();

      if (result?.ok) {
        setSummary(result.data);
        setSummaryError(null);
        return;
      }

      setSummaryError(result?.error.message ?? unavailableMessage);
    } catch (error) {
      setSummaryError(error instanceof Error ? error.message : "Diagnostics summary failed.");
    }
  }

  async function refreshLogs(): Promise<void> {
    const result = await window.hcb?.diagnostics.logs({ minimumLevel: logLevel, limit: 200 });

    if (result?.ok) {
      setLogs(result.data);
    }
  }

  async function refreshHistory(): Promise<void> {
    try {
      const result = await window.hcb?.diagnostics.history({ limit: source.settings.visibleHistoryEntryCount });

      if (result?.ok) {
        setHistoryEntries(result.data.entries);
        setHistoryError(null);
        return;
      }

      setHistoryError(result?.error.message ?? unavailableMessage);
    } catch (error) {
      setHistoryError(error instanceof Error ? error.message : "Diagnostics history failed.");
    }
  }

  async function refreshPendingMutations(): Promise<void> {
    try {
      const result = await window.hcb?.diagnostics.pendingMutations({ limit: 100 });

      if (result?.ok) {
        setPendingMutations(result.data.mutations);
        setPendingMutationsError(null);
        return;
      }

      setPendingMutationsError(result?.error.message ?? unavailableMessage);
    } catch (error) {
      setPendingMutationsError(error instanceof Error ? error.message : "Pending mutations failed.");
    }
  }

  async function refreshSyncDiagnostics(): Promise<void> {
    await Promise.all([refreshPendingMutations(), refreshSummary()]);
  }

  async function runRecovery(
    action: "refresh" | "forceFullResync" | "clearGoogleCache"
  ): Promise<void> {
    if (
      action === "forceFullResync" &&
      !window.confirm("Force a full Google resync by clearing sync checkpoints?")
    ) {
      return;
    }

    if (
      action === "clearGoogleCache" &&
      !window.confirm("Clear cached Google task and calendar data from this Mac?")
    ) {
      return;
    }

    setWorking(true);
    setMessage(null);

    const result = await source.runRecoveryAction({
      action,
      ...(action === "forceFullResync"
        ? { confirmation: { accepted: true, phrase: "FULL RESYNC" } }
        : {}),
      ...(action === "clearGoogleCache"
        ? { confirmation: { accepted: true, phrase: "CLEAR CACHE" } }
        : {})
    });

    setWorking(false);

    if (result) {
      setMessage({ tone: "success", text: result.message });
      await refreshAll();
    }
  }

  async function rebuildNotifications(): Promise<void> {
    setWorking(true);
    setMessage(null);
    const result = await window.hcb?.diagnostics.rescheduleNotifications();
    setWorking(false);

    if (result?.ok) {
      setMessage({ tone: "success", text: result.data.message });
      await refreshSummary();
      return;
    }

    if (result && !result.ok) {
      setMessage({ tone: "warning", text: result.error.message });
    }
  }

  async function retryMutation(id: string): Promise<void> {
    const result = await window.hcb?.diagnostics.retryPendingMutation({ id });

    if (result?.ok) {
      setMessage({ tone: "success", text: "Pending mutation was queued for retry." });
      await Promise.all([refreshPendingMutations(), refreshHistory(), refreshSummary()]);
      return;
    }

    setMessage({ tone: "warning", text: result?.error.message ?? unavailableMessage });
  }

  async function copyVisibleSyncIssues(text: string): Promise<void> {
    await navigator.clipboard?.writeText(text);
    setMessage({ tone: "success", text: "Visible sync issues copied." });
  }

  async function cancelMutation(id: string): Promise<void> {
    if (!window.confirm("Cancel this pending Google write?")) {
      return;
    }

    const result = await window.hcb?.diagnostics.cancelPendingMutation({ id });

    if (result?.ok) {
      setMessage({ tone: "success", text: "Pending mutation was cancelled." });
      await Promise.all([refreshPendingMutations(), refreshHistory(), refreshSummary()]);
      return;
    }

    setMessage({ tone: "warning", text: result?.error.message ?? unavailableMessage });
  }

  async function clearLogs(): Promise<void> {
    if (!window.confirm("Clear all local diagnostics logs?")) {
      return;
    }

    const result = await window.hcb?.diagnostics.clearLogs();

    if (result?.ok) {
      setMessage({ tone: "success", text: "Logs cleared." });
      await refreshLogs();
    }
  }

  async function revealLogsFolder(): Promise<void> {
    const result = await window.hcb?.diagnostics.revealLogsFolder();

    if (result?.ok) {
      setMessage({
        tone: result.data.opened ? "success" : "warning",
        text: result.data.message
      });
    }
  }

  async function copyDiagnosticSummary(): Promise<void> {
    const result = await window.hcb?.diagnostics.copyableSummary();

    if (result?.ok) {
      await navigator.clipboard?.writeText(result.data.text);
      setMessage({ tone: "success", text: "Diagnostic summary copied." });
    }
  }

  async function exportBundle(): Promise<void> {
    const result = await window.hcb?.diagnostics.exportBundle();

    if (result?.ok) {
      setMessage({
        tone: result.data.exported ? "success" : "warning",
        text: result.data.message
      });
    }
  }

  async function copyLogs(): Promise<void> {
    const text =
      logs?.persistedText ||
      (logs?.entries ?? []).map((entry) => entry.formattedLine).join("\n");

    await navigator.clipboard?.writeText(text);
    setMessage({ tone: "success", text: "All logs copied." });
  }

  async function copyVisibleLogs(): Promise<void> {
    const text = filteredLogs.map((entry) => entry.formattedLine).join("\n");

    await navigator.clipboard?.writeText(text);
    setMessage({ tone: "success", text: "Visible logs copied." });
  }

  async function copyHistory(): Promise<void> {
    const text = filteredHistory
      .map((entry) => `${formatDateTime(entry.timestamp)} [${entry.kind}] ${entry.summary}`)
      .join("\n");

    await navigator.clipboard?.writeText(text);
    setMessage({ tone: "success", text: "Visible history copied." });
  }

  return {
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
  };
}
