import type {
  CalendarListSummary,
  GoogleStatusResponse,
  SettingsSnapshot,
  TaskListSummary
} from "@shared/ipc/contracts";
import { useEffect, useState } from "react";
import { Eye, EyeOff, Save, ShieldCheck, Trash2, Users } from "lucide-react";
import { Badge, Button, IconButton, Input, cx } from "../../../../components/primitives";
import { EmptyState } from "../../../../components/states";
import {
  SettingsControlRow,
  SettingsGroup,
  SettingsSwitch
} from "./SettingsPrimitives";

interface ProfileSettingsTabProps {
  beginGoogleOAuth: () => Promise<void>;
  calendarSources: CalendarListSummary[];
  disconnectGoogle: (accountId?: string) => Promise<void>;
  googleClientId: string;
  googleClientSecret: string;
  googleStatus: GoogleStatusResponse;
  saveGoogleOAuthClient: () => Promise<void>;
  setGoogleClientId: (value: string) => void;
  setGoogleClientSecret: (value: string) => void;
  settings: SettingsSnapshot;
  settingsMutationPending: boolean;
  taskLists: TaskListSummary[];
  updateSelectedCalendar: (calendarId: string, selected: boolean) => void;
  updateSelectedTaskList: (taskListId: string, selected: boolean) => void;
}

export function ProfileSettingsTab({
  beginGoogleOAuth,
  calendarSources,
  disconnectGoogle,
  googleClientId,
  googleClientSecret,
  googleStatus,
  saveGoogleOAuthClient,
  setGoogleClientId,
  setGoogleClientSecret,
  settings,
  settingsMutationPending,
  taskLists,
  updateSelectedCalendar,
  updateSelectedTaskList
}: ProfileSettingsTabProps): JSX.Element {
  const [showClientSecret, setShowClientSecret] = useState(false);
  const selectedTaskLists = new Set(settings.selectedTaskListIds);
  const selectedCalendars = new Set(settings.selectedCalendarIds);
  const account = googleStatus.account;
  const accounts = googleStatus.accounts.length > 0 ? googleStatus.accounts : account ? [account] : [];
  const visibleAccounts = accounts.filter(isVisibleGoogleAccount);
  const primaryAccount = visibleAccounts[0] ?? (account && isVisibleGoogleAccount(account) ? account : undefined);
  const [resourceAccountFilter, setResourceAccountFilter] = useState("all");
  const visibleTaskLists = resourceAccountFilter === "all"
    ? taskLists
    : taskLists.filter((taskList) => taskList.accountId === resourceAccountFilter);
  const visibleCalendarSources = resourceAccountFilter === "all"
    ? calendarSources
    : calendarSources.filter((calendar) => calendar.accountId === resourceAccountFilter);
  const connected = primaryAccount?.connectionState === "connected";
  const accountLabel = primaryAccount?.displayName || primaryAccount?.email || (connected ? "Connected Google account" : "Not connected");
  const accountDetail = primaryAccount?.email ?? primaryAccount?.googleAccountId ?? primaryAccount?.connectionState ?? "Google account is not connected";

  useEffect(() => {
    if (resourceAccountFilter === "all" || visibleAccounts.some((candidate) => candidate.accountId === resourceAccountFilter)) {
      return;
    }

    setResourceAccountFilter("all");
  }, [resourceAccountFilter, visibleAccounts]);

  return (
    <div className="grid gap-5">
      <SettingsGroup title="Google OAuth client">
        <SettingsControlRow
          description={googleStatus.oauthClientConfigured ? "Google Cloud OAuth client saved." : "Missing"}
          icon={ShieldCheck}
          label="Google Cloud OAuth client"
        >
          <Badge tone={googleStatus.oauthClientConfigured ? "success" : "warning"}>
            {googleStatus.oauthClientConfigured ? "Configured" : "Missing"}
          </Badge>
        </SettingsControlRow>
        <SettingsControlRow label="Desktop OAuth client ID">
          <div className="w-full max-w-full sm:w-[42rem]">
            <Input
              aria-label="Google OAuth client ID"
              onChange={(event) => setGoogleClientId(event.currentTarget.value)}
              placeholder="Client ID from Google Cloud Console"
              value={googleClientId}
            />
          </div>
        </SettingsControlRow>
        <SettingsControlRow label="Client secret (optional)">
          <div className="flex w-full max-w-full items-center gap-2 sm:w-[42rem]">
            <Input
              aria-label="Google OAuth client secret"
              onChange={(event) => setGoogleClientSecret(event.currentTarget.value)}
              placeholder={googleStatus.hasClientSecret ? "Stored in Keychain" : "Optional for Desktop clients"}
              type={showClientSecret ? "text" : "password"}
              value={googleClientSecret}
            />
            <IconButton
              icon={showClientSecret ? EyeOff : Eye}
              label={showClientSecret ? "Hide client secret" : "Show client secret"}
              onClick={() => setShowClientSecret((current) => !current)}
              variant="secondary"
            />
          </div>
        </SettingsControlRow>
        <div className="flex flex-wrap items-center gap-2 px-3 pb-3">
          <Button
            disabled={googleClientId.trim().length < 10 || settingsMutationPending}
            onClick={() => void saveGoogleOAuthClient()}
            variant="primary"
          >
            <Save aria-hidden="true" size={14} />
            Save OAuth Client
          </Button>
          <Button onClick={() => setGoogleClientSecret("")} variant="secondary">
            <Trash2 aria-hidden="true" size={14} />
            Clear
          </Button>
        </div>
      </SettingsGroup>

      <SettingsGroup title="Google accounts">
        {visibleAccounts.length > 0 ? (
          visibleAccounts.map((candidate) => {
            const candidateConnected = candidate.connectionState === "connected";
            const candidateLabel = candidate.displayName || candidate.email || "Google account";
            const candidateDetail = candidate.email ?? candidate.googleAccountId ?? candidate.connectionState;

            return (
              <div className="grid min-h-11 gap-2 border-b border-border px-3 py-2 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center" key={candidate.accountId}>
                <div className="flex min-w-0 items-center gap-2.5">
                  {candidate.avatarUrl ? (
                    <img
                      alt=""
                      className="size-8 shrink-0 rounded-full border border-border bg-surface-0 object-cover"
                      referrerPolicy="no-referrer"
                      src={candidate.avatarUrl}
                    />
                  ) : (
                    <div className="flex size-8 shrink-0 items-center justify-center rounded-full border border-border bg-surface-0 text-text-muted">
                      <Users aria-hidden="true" size={16} />
                    </div>
                  )}
                  <div className="min-w-0">
                    <div className="truncate text-[var(--text-base)] font-medium text-text-primary">{candidateLabel}</div>
                    <p className={cx(
                      "mt-0.5 truncate text-[var(--text-sm)]",
                      candidateConnected ? "text-text-muted" : "text-warning"
                    )}>
                      {candidateDetail}
                    </p>
                  </div>
                </div>
                <div className="flex items-center justify-end gap-2">
                  <Badge tone={candidateConnected ? "success" : "warning"}>
                    {candidateConnected ? "Active" : candidate.connectionState}
                  </Badge>
                  <Button onClick={() => void disconnectGoogle(candidate.accountId)} size="sm" variant="secondary">
                    Disconnect
                  </Button>
                </div>
              </div>
            );
          })
        ) : (
          <div className="grid min-h-11 gap-2 border-b border-border px-3 py-2 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
            <div className="flex min-w-0 items-center gap-2.5">
              <div className="flex size-8 shrink-0 items-center justify-center rounded-full border border-border bg-surface-0 text-text-muted">
                <Users aria-hidden="true" size={16} />
              </div>
              <div className="min-w-0">
                <div className="truncate text-[var(--text-base)] font-medium text-text-primary">{accountLabel}</div>
                <p className={cx(
                  "mt-0.5 truncate text-[var(--text-sm)]",
                  connected ? "text-text-muted" : "text-warning"
                )}>
                  {accountDetail}
                </p>
              </div>
            </div>
            <Badge tone={connected ? "success" : "warning"}>
              {connected ? "Active" : "Disconnected"}
            </Badge>
          </div>
        )}
        <div className="flex flex-wrap items-center gap-2 px-3 pb-3">
          <Button
            disabled={!googleStatus.oauthClientConfigured}
            onClick={() => void beginGoogleOAuth()}
            variant="primary"
          >
            <Users aria-hidden="true" size={14} />
            Add Google Account
          </Button>
          <Button
            disabled={visibleAccounts.length === 0}
            onClick={() => void Promise.all(visibleAccounts.map((candidate) => disconnectGoogle(candidate.accountId)))}
            variant="secondary"
          >
            Disconnect all
          </Button>
        </div>
      </SettingsGroup>

      <SettingsGroup title="Task lists">
        {visibleAccounts.length > 1 ? (
          <ResourceAccountFilter
            accounts={visibleAccounts}
            value={resourceAccountFilter}
            onChange={setResourceAccountFilter}
          />
        ) : null}
        {visibleTaskLists.length === 0 ? (
          <EmptyState description="No task lists are available yet." title="No task lists" />
        ) : visibleTaskLists.map((taskList) => (
          <SettingsSwitch
            checked={selectedTaskLists.size === 0 || selectedTaskLists.has(taskList.id)}
            key={taskList.id}
            label={taskList.title}
            onChange={(checked) => updateSelectedTaskList(taskList.id, checked)}
            trailing={(
              <span className="flex items-center gap-2">
                {visibleAccounts.length > 1 && taskList.accountId ? <Badge tone="neutral">{accountName(visibleAccounts, taskList.accountId)}</Badge> : null}
                <Badge>{taskList.activeTaskCount ?? taskList.taskCount ?? 0}</Badge>
              </span>
            )}
          />
        ))}
      </SettingsGroup>

      <SettingsGroup title="Calendars">
        {visibleAccounts.length > 1 ? (
          <ResourceAccountFilter
            accounts={visibleAccounts}
            value={resourceAccountFilter}
            onChange={setResourceAccountFilter}
          />
        ) : null}
        {visibleCalendarSources.length === 0 ? (
          <EmptyState description="No calendars are available yet." title="No calendars" />
        ) : visibleCalendarSources.map((calendar) => (
          <SettingsSwitch
            checked={selectedCalendars.size === 0 ? calendar.selected : selectedCalendars.has(calendar.id)}
            description={calendar.timeZone ?? undefined}
            key={calendar.id}
            label={calendar.title}
            onChange={(checked) => updateSelectedCalendar(calendar.id, checked)}
            trailing={(
              <span className="flex items-center gap-2">
                {visibleAccounts.length > 1 && calendar.accountId ? <Badge tone="neutral">{accountName(visibleAccounts, calendar.accountId)}</Badge> : null}
                <Badge>{calendar.eventCount ?? 0}</Badge>
              </span>
            )}
          />
        ))}
      </SettingsGroup>
    </div>
  );
}

function isVisibleGoogleAccount(account: GoogleStatusResponse["accounts"][number]): boolean {
  if (account.connectionState === "signed_out") {
    return false;
  }

  if (account.accountId === "local-google-account" || account.accountId === "local:ics") {
    return false;
  }

  return Boolean(account.email || account.googleAccountId || account.connectionState === "connected");
}

function accountName(
  accounts: GoogleStatusResponse["accounts"],
  accountId: string
): string {
  const account = accounts.find((candidate) => candidate.accountId === accountId);
  return account?.displayName || account?.email || "Account";
}

function ResourceAccountFilter({
  accounts,
  onChange,
  value
}: {
  accounts: GoogleStatusResponse["accounts"];
  onChange: (value: string) => void;
  value: string;
}): JSX.Element {
  return (
    <div className="flex flex-wrap items-center gap-2 border-b border-border px-3 py-2">
      <Button
        onClick={() => onChange("all")}
        size="sm"
        variant={value === "all" ? "primary" : "secondary"}
      >
        All accounts
      </Button>
      {accounts.map((account) => (
        <Button
          key={account.accountId}
          onClick={() => onChange(account.accountId)}
          size="sm"
          variant={value === account.accountId ? "primary" : "secondary"}
        >
          {accountName(accounts, account.accountId)}
        </Button>
      ))}
    </div>
  );
}
