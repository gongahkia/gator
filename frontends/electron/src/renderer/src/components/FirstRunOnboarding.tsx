import { useEffect, useMemo, useState, type ReactNode } from "react";
import type { SettingsSnapshot } from "@shared/ipc/contracts";
import {
  Bell,
  CalendarDays,
  CheckCircle2,
  Cloud,
  ExternalLink,
  HardDrive,
  ListChecks,
  RefreshCw,
  Server
} from "lucide-react";
import type { CoreViewModelSource } from "../features/core/coreViewModelSource";
import { Badge, Button, Input, StatusBanner } from "./primitives";

export function FirstRunOnboarding({ source }: { source: CoreViewModelSource }): JSX.Element {
  const initialTaskListIds =
    source.settings.selectedTaskListIds.length > 0
      ? source.settings.selectedTaskListIds
      : source.taskLists.map((taskList) => taskList.id);
  const initialCalendarIds =
    source.settings.selectedCalendarIds.length > 0
      ? source.settings.selectedCalendarIds
      : source.calendarSources.filter((calendar) => calendar.selected).map((calendar) => calendar.id);
  const [selectedTaskListIds, setSelectedTaskListIds] = useState<string[]>(initialTaskListIds);
  const [selectedCalendarIds, setSelectedCalendarIds] = useState<string[]>(initialCalendarIds);
  const [storageBackend, setStorageBackend] =
    useState<SettingsSnapshot["storageBackend"]>(source.settings.storageBackend);
  const [hcbHosterEndpoint, setHcbHosterEndpoint] = useState(source.settings.hcbHosterEndpoint ?? "");
  const [syncMode, setSyncMode] = useState<SettingsSnapshot["syncMode"]>(source.settings.syncMode);
  const [notificationsEnabled, setNotificationsEnabled] = useState(source.settings.notificationsEnabled);
  const [mcpEnabled, setMcpEnabled] = useState(source.settings.mcpEnabled);
  const [mcpPermissionMode, setMcpPermissionMode] =
    useState<SettingsSnapshot["mcpPermissionMode"]>(source.settings.mcpPermissionMode);
  const [localError, setLocalError] = useState<string | null>(null);
  const [googleMessage, setGoogleMessage] = useState<string | null>(null);
  const [googleClientId, setGoogleClientId] = useState(source.googleStatus.clientId ?? "");
  const [googleClientSecret, setGoogleClientSecret] = useState("");
  const [googleConnecting, setGoogleConnecting] = useState(false);
  const [googleClientSaving, setGoogleClientSaving] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const selectedTaskLists = useMemo(() => new Set(selectedTaskListIds), [selectedTaskListIds]);
  const selectedCalendars = useMemo(() => new Set(selectedCalendarIds), [selectedCalendarIds]);
  const accountState = source.diagnosticsSummary?.account.state ?? "signed_out";
  const googleConnected =
    source.googleStatus.account?.connectionState === "connected" || accountState === "connected";
  const localBackendSelected = storageBackend !== "google";
  const setupCanFinish = localBackendSelected || googleConnected;
  const googleAccountLabel =
    source.googleStatus.account?.displayName ?? source.googleStatus.account?.email ?? "Google account";
  const nativeFlags = source.diagnosticsSummary?.native.flags ?? source.native.capabilityReport.flags;
  const oauthRuntimeReady =
    nativeFlags.supportsOAuthLoopback ??
    nativeFlags.supportsCredentialStorage ??
    false;
  const googleClientConfigured = source.googleStatus.oauthClientConfigured;

  useEffect(() => {
    setGoogleClientId(source.googleStatus.clientId ?? "");
  }, [source.googleStatus.clientId]);

  function toggleTaskList(taskListId: string, selected: boolean): void {
    setSelectedTaskListIds((current) => {
      const next = new Set(current);

      if (selected) {
        next.add(taskListId);
      } else {
        next.delete(taskListId);
      }

      return [...next];
    });
  }

  function toggleCalendar(calendarId: string, selected: boolean): void {
    setSelectedCalendarIds((current) => {
      const next = new Set(current);

      if (selected) {
        next.add(calendarId);
      } else {
        next.delete(calendarId);
      }

      return [...next];
    });
  }

  async function completeSetup(
    overrides: Partial<Pick<
      SettingsSnapshot,
      | "selectedTaskListIds"
      | "selectedCalendarIds"
      | "storageBackend"
      | "hcbHosterEndpoint"
      | "syncMode"
      | "notificationsEnabled"
      | "mcpEnabled"
      | "mcpPermissionMode"
    >> = {}
  ): Promise<void> {
    setSubmitting(true);
    setLocalError(null);

    const saved = await source.updateSettings({
      selectedTaskListIds: overrides.selectedTaskListIds ?? selectedTaskListIds,
      selectedCalendarIds: overrides.selectedCalendarIds ?? selectedCalendarIds,
      storageBackend: overrides.storageBackend ?? storageBackend,
      hcbHosterEndpoint:
        overrides.hcbHosterEndpoint ??
        (hcbHosterEndpoint.trim().length > 0 ? hcbHosterEndpoint.trim() : null),
      syncMode: overrides.syncMode ?? syncMode,
      notificationsEnabled: overrides.notificationsEnabled ?? notificationsEnabled,
      mcpEnabled: overrides.mcpEnabled ?? mcpEnabled,
      mcpPermissionMode: overrides.mcpPermissionMode ?? mcpPermissionMode,
      setupCompletedAt: new Date().toISOString()
    });

    if (!saved) {
      setSubmitting(false);
      setLocalError("Setup preferences were not saved.");
    }
  }

  async function connectGoogle(): Promise<void> {
    if (!googleClientConfigured) {
      setLocalError("Save a Google Desktop OAuth client ID before connecting.");
      return;
    }

    setGoogleConnecting(true);
    setGoogleMessage(null);
    setLocalError(null);

    const result = await window.hcb?.google.beginOAuth();

    if (result?.ok) {
      setGoogleMessage(result.data.message);
      source.refreshGoogleStatus();
      for (const delayMs of [2_000, 5_000, 10_000]) {
        window.setTimeout(() => source.refreshGoogleStatus(), delayMs);
      }
    } else {
      setLocalError(result?.error.message ?? "Google authorization could not start.");
    }

    setGoogleConnecting(false);
  }

  async function saveGoogleClient(): Promise<void> {
    setGoogleClientSaving(true);
    setGoogleMessage(null);
    setLocalError(null);

    const request =
      googleClientSecret.trim().length > 0
        ? { clientId: googleClientId, clientSecret: googleClientSecret.trim() }
        : { clientId: googleClientId };
    const result = await window.hcb?.google.saveOAuthClient(request);

    if (result?.ok) {
      source.setGoogleStatus(result.data);
      setGoogleClientSecret("");
      setGoogleMessage("Google OAuth client saved.");
    } else {
      setLocalError(result?.error.message ?? "Google OAuth client could not be saved.");
    }

    setGoogleClientSaving(false);
  }

  return (
    <div
      aria-labelledby="first-run-title"
      aria-modal="true"
      className="fixed inset-0 z-50 flex items-center justify-center bg-bg-primary/80 p-3 sm:p-6"
      role="dialog"
    >
      <div className="flex max-h-[calc(100vh-48px)] w-full max-w-5xl flex-col overflow-hidden rounded-hcbMd border border-border bg-bg-primary shadow-hcbLg">
        <header className="flex min-h-14 flex-wrap items-center justify-between gap-3 border-b border-border px-3 py-2 sm:px-5">
          <div className="min-w-0">
            <h2 className="truncate text-[var(--text-xl)] font-bold text-text-primary" id="first-run-title">
              First-run setup
            </h2>
            <p className="truncate text-[var(--text-sm)] text-text-muted">
              Choose planner storage, then finish Google-backed or local-first setup.
            </p>
          </div>
          <Badge tone={setupCanFinish ? "success" : "warning"}>
            {localBackendSelected ? "Local backend" : `Google ${googleConnected ? "connected" : "not connected"}`}
          </Badge>
        </header>

        <div className="grid min-h-0 gap-3 overflow-y-auto p-4">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <SetupCard
              description={
                storageBackend === "google"
                  ? "Use Google Tasks and Calendar as source of truth."
                  : storageBackend === "hcb-hoster"
                    ? "Use a local hoster endpoint and stop Google mutation queueing."
                    : "Use the encrypted HCB local vault and stop Google mutation queueing."
              }
              icon={HardDrive}
              status={storageBackend === "google" ? "Google" : "Local"}
              title="1. Backend"
            >
              <select
                aria-label="Onboarding storage backend"
                className={onboardingSelectClass}
                onChange={(event) => setStorageBackend(event.target.value as SettingsSnapshot["storageBackend"])}
                value={storageBackend}
              >
                <option value="google">Google Tasks/Calendar</option>
                <option value="hcb-local">HCB local vault</option>
                <option value="hcb-hoster">HCB local hoster</option>
              </select>
              {storageBackend === "hcb-hoster" ? (
                <Input
                  aria-label="HCB local hoster endpoint"
                  onChange={(event) => setHcbHosterEndpoint(event.currentTarget.value)}
                  placeholder="http://127.0.0.1:7419"
                  value={hcbHosterEndpoint}
                />
              ) : null}
            </SetupCard>
            <SetupCard
              description={
                localBackendSelected
                  ? "Optional. Connect later if you want Google sync."
                  : googleConnected
                    ? `Connected as ${googleAccountLabel}.`
                    : !oauthRuntimeReady
                      ? "Google OAuth browser handoff is unavailable in this runtime."
                      : googleClientConfigured
                        ? "Open the browser to authorize Google Tasks and Calendar sync."
                        : "Save a Desktop OAuth client ID, then connect your Google account."
              }
              icon={Cloud}
              status={
                localBackendSelected
                  ? "Optional"
                  : googleConnected
                    ? "Connected"
                    : !oauthRuntimeReady
                      ? "Unavailable"
                      : googleClientConfigured
                        ? "Ready"
                        : "Needs client"
              }
              title="2. Google account"
            >
              {!googleConnected ? (
                <div className="grid w-full gap-2">
                  <Input
                    aria-label="Google OAuth client ID"
                    onChange={(event) => setGoogleClientId(event.currentTarget.value)}
                    placeholder="Desktop OAuth client ID"
                    value={googleClientId}
                  />
                  <Input
                    aria-label="Google OAuth client secret"
                    onChange={(event) => setGoogleClientSecret(event.currentTarget.value)}
                    placeholder={source.googleStatus.hasClientSecret ? "Stored client secret" : "Client secret (optional)"}
                    type="password"
                    value={googleClientSecret}
                  />
                </div>
              ) : null}
              {!googleConnected ? (
                <Button
                  disabled={googleClientId.trim().length < 10 || googleClientSaving}
                  onClick={() => void saveGoogleClient()}
                  variant="secondary"
                >
                  Save OAuth Client
                </Button>
              ) : null}
              <Button
                disabled={googleConnecting || !oauthRuntimeReady || !googleClientConfigured || googleConnected}
                onClick={() => void connectGoogle()}
                variant="primary"
              >
                <ExternalLink aria-hidden="true" size={14} />
                {googleConnected ? "Google connected" : googleConnecting ? "Opening Google" : "Connect Google"}
              </Button>
              {googleMessage ? (
                <p className="text-[var(--text-xs)] text-text-muted">{googleMessage}</p>
              ) : null}
            </SetupCard>
            <SetupCard
              description={`${selectedTaskListIds.length} task list${selectedTaskListIds.length === 1 ? "" : "s"} selected`}
              icon={ListChecks}
              status={source.taskLists.length === 0 && !localBackendSelected ? "None" : "Selected"}
              title="3. Task lists"
            />
            <SetupCard
              description={`${selectedCalendarIds.length} calendar${selectedCalendarIds.length === 1 ? "" : "s"} selected`}
              icon={CalendarDays}
              status={source.calendarSources.length === 0 && !localBackendSelected ? "None" : "Selected"}
              title="4. Calendars"
            />
          </div>

          <div className="grid grid-cols-1 gap-3 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
            <section className="min-w-0 rounded-hcbMd border border-border bg-bg-secondary">
              <div className="border-b border-border px-3 py-2">
                <h3 className="text-[var(--text-md)] font-semibold text-text-primary">Task lists</h3>
                <p className="text-[var(--text-xs)] text-text-muted">
                  {localBackendSelected ? "Local inbox is created if none exist." : "Google Tasks lists"}
                </p>
              </div>
              <div className="grid max-h-44 gap-2 overflow-y-auto p-3">
                {source.taskLists.length === 0 ? (
                  <p className="text-[var(--text-sm)] text-text-muted">
                    {localBackendSelected ? "Local inbox will be created." : "No task lists."}
                  </p>
                ) : source.taskLists.map((taskList) => (
                  <label
                    className="flex min-h-8 items-center gap-2 rounded-hcbMd border border-border bg-bg-tertiary px-3 text-[var(--text-sm)] text-text-secondary"
                    key={taskList.id}
                  >
                    <input
                      aria-label={`Select task list ${taskList.title}`}
                      checked={selectedTaskLists.has(taskList.id)}
                      className="accent-[var(--color-accent)]"
                      onChange={(event) => toggleTaskList(taskList.id, event.target.checked)}
                      type="checkbox"
                    />
                    <span className="min-w-0 flex-1 truncate">{taskList.title}</span>
                    <Badge>{taskList.activeTaskCount ?? taskList.taskCount ?? 0}</Badge>
                  </label>
                ))}
              </div>
            </section>

            <section className="min-w-0 rounded-hcbMd border border-border bg-bg-secondary">
              <div className="border-b border-border px-3 py-2">
                <h3 className="text-[var(--text-md)] font-semibold text-text-primary">Calendars</h3>
                <p className="text-[var(--text-xs)] text-text-muted">
                  {localBackendSelected ? "Local calendar is created if none exist." : "Google Calendar lists"}
                </p>
              </div>
              <div className="grid max-h-44 gap-2 overflow-y-auto p-3">
                {source.calendarSources.length === 0 ? (
                  <p className="text-[var(--text-sm)] text-text-muted">
                    {localBackendSelected ? "Local calendar will be created." : "No calendars."}
                  </p>
                ) : source.calendarSources.map((calendar) => (
                  <label
                    className="flex min-h-8 items-center gap-2 rounded-hcbMd border border-border bg-bg-tertiary px-3 text-[var(--text-sm)] text-text-secondary"
                    key={calendar.id}
                  >
                    <input
                      aria-label={`Select calendar ${calendar.title}`}
                      checked={selectedCalendars.has(calendar.id)}
                      className="accent-[var(--color-accent)]"
                      onChange={(event) => toggleCalendar(calendar.id, event.target.checked)}
                      type="checkbox"
                    />
                    <span className="min-w-0 flex-1 truncate">{calendar.title}</span>
                    <Badge>{calendar.eventCount ?? 0}</Badge>
                  </label>
                ))}
              </div>
            </section>
          </div>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <SetupOption title="5. Sync mode" icon={RefreshCw}>
              <select
                aria-label="Onboarding sync mode"
                className={onboardingSelectClass}
                onChange={(event) => setSyncMode(event.target.value as SettingsSnapshot["syncMode"])}
                value={syncMode}
              >
                <option value="manual">Manual</option>
                <option value="balanced">Balanced</option>
                <option value="near-real-time">Near real time</option>
              </select>
            </SetupOption>
            <SetupOption title="6. Notifications" icon={Bell}>
              <label className="flex min-h-8 items-center gap-2 text-[var(--text-sm)] text-text-secondary">
                <input
                  checked={notificationsEnabled}
                  className="accent-[var(--color-accent)]"
                  onChange={(event) => setNotificationsEnabled(event.target.checked)}
                  type="checkbox"
                />
                Local notifications
              </label>
            </SetupOption>
            <SetupOption title="7. MCP access" icon={Server}>
              <div className="grid gap-2">
                <label className="flex min-h-8 items-center gap-2 text-[var(--text-sm)] text-text-secondary">
                  <input
                    checked={mcpEnabled}
                    className="accent-[var(--color-accent)]"
                    onChange={(event) => setMcpEnabled(event.target.checked)}
                    type="checkbox"
                  />
                  Enable MCP
                </label>
                <select
                  aria-label="Onboarding MCP permission mode"
                  className={onboardingSelectClass}
                  disabled={!mcpEnabled}
                  onChange={(event) =>
                    setMcpPermissionMode(event.target.value as SettingsSnapshot["mcpPermissionMode"])
                  }
                  value={mcpPermissionMode}
                >
                  <option value="read-only">Read-only</option>
                  <option value="confirm-writes">Confirm writes</option>
                  <option value="allow-writes">Allow writes</option>
                </select>
              </div>
            </SetupOption>
          </div>

          {source.settingsMutationError || localError ? (
            <StatusBanner
              description={source.settingsMutationError ?? localError ?? "Setup settings failed."}
              title={source.settingsMutationError ? "Setup not saved" : "Setup needs attention"}
              tone="warning"
            />
          ) : null}
        </div>

        <footer className="flex min-h-14 flex-wrap items-center justify-between gap-3 border-t border-border px-3 py-2 sm:px-5">
          <p className="text-[var(--text-sm)] text-text-muted">
            {localBackendSelected ? "Local setup can finish without Google." : "Google connection is required before setup can finish."}
          </p>
          <Button
            disabled={submitting || source.settingsMutationPending || !setupCanFinish}
            onClick={() => void completeSetup()}
            variant="primary"
          >
            <CheckCircle2 aria-hidden="true" size={15} />
            Finish setup
          </Button>
        </footer>
      </div>
    </div>
  );
}

function SetupCard({
  children,
  description,
  icon: Icon,
  status,
  title
}: {
  children?: ReactNode;
  description: string;
  icon: typeof Cloud;
  status: string;
  title: string;
}): JSX.Element {
  return (
    <section className="min-w-0 rounded-hcbMd border border-border bg-bg-secondary p-3">
      <div className="flex items-center gap-2">
        <div className="flex size-7 shrink-0 items-center justify-center rounded-hcbSm bg-surface-0 text-accent">
          <Icon aria-hidden="true" size={15} />
        </div>
        <div className="min-w-0 flex-1">
          <h3 className="truncate text-[var(--text-md)] font-semibold text-text-primary">{title}</h3>
        </div>
        <Badge tone={status === "Ready" || status === "Selected" || status === "Connected" ? "success" : "warning"}>{status}</Badge>
      </div>
      <p className="mt-2 line-clamp-2 text-[var(--text-sm)] text-text-muted">{description}</p>
      {children ? <div className="mt-3 grid gap-2">{children}</div> : null}
    </section>
  );
}

function SetupOption({
  children,
  icon: Icon,
  title
}: {
  children: JSX.Element;
  icon: typeof Cloud;
  title: string;
}): JSX.Element {
  return (
    <section className="min-w-0 rounded-hcbMd border border-border bg-bg-secondary p-3">
      <div className="mb-3 flex items-center gap-2">
        <Icon aria-hidden="true" className="text-accent" size={15} />
        <h3 className="truncate text-[var(--text-md)] font-semibold text-text-primary">{title}</h3>
      </div>
      {children}
    </section>
  );
}

const onboardingSelectClass =
  "h-8 w-full rounded-hcbMd border border-border bg-surface-0 px-2 text-[var(--text-base)] text-text-primary focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent";
