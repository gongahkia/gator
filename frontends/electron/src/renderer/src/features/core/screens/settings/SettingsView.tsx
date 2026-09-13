import { useEffect, useMemo, useRef, useState } from "react";
import type {
  SettingsRecoveryActionRequest,
  SettingsSnapshot,
  SettingsUpdateRequest
} from "@shared/ipc/contracts";
import { previewAutoTagRules, validateAutoTagRule, type AutoTagTargetKind } from "@shared/ipc/autoTags";
import {
  appColorThemes,
  defaultAppColorTheme,
  resolveAppColorTheme,
  resolveEffectiveColorTheme,
  resolveEffectiveThemeMode,
  resolveAppThemeMode
} from "@shared/ipc/themeCatalog";
import { Bell, Brush, Copy, Info, Keyboard, Search, Settings2, SlidersHorizontal, Users } from "lucide-react";
import { useInspector } from "../../../../components/Inspector";
import { Button, Input, Panel, StatusBanner } from "../../../../components/primitives";
import { useCoreViewModelSource } from "../../coreViewModelSource";
import {
  currentSystemPrefersDark,
  fontFamilyOptions,
  sanitizedJson
} from "../../coreScreenShared";
import { hotkeyDefinitions } from "../../hotkeys";
import { AppearanceSettingsTab } from "./AppearanceSettingsTab";
import { AboutSettingsTab } from "./AboutSettingsTab";
import { AdvancedSettingsTab } from "./AdvancedSettingsTab";
import { AlertsSettingsTab } from "./AlertsSettingsTab";
import { GeneralSettingsTab } from "./GeneralSettingsTab";
import { HotkeysSettingsTab } from "./HotkeysSettingsTab";
import { ProfileSettingsTab } from "./ProfileSettingsTab";
import {
  SettingsSearchProvider,
  SettingsTabButton,
  settingsSearchMatches
} from "./SettingsPrimitives";
import { recoveryPhrase } from "./settingsUtils";

type SettingsTabId = "general" | "profile" | "appearance" | "hotkeys" | "alerts" | "advanced" | "about";
type AutoTagBackgroundNotice = {
  title: string;
  description: string;
  tone: "info" | "success" | "warning";
  changedKinds?: Array<{ kind: AutoTagTargetKind; changed: number }>;
};

const autoTagTargetKinds: AutoTagTargetKind[] = ["task", "event", "note"];

const settingsSearchTextByTab: Record<SettingsTabId, string> = {
  about: [
    "About",
    "Updates",
    "Automatically check for updates",
    "Update channel",
    "Last checked",
    "App",
    "Version",
    "Build",
    "Bundle ID",
    "Copy version info"
  ].join(" "),
  advanced: [
    "Advanced",
    "Calendars",
    "Read calendars",
    "Show hidden calendars",
    "Task lists",
    "Read tasks",
    "Per-tab list filters",
    "Tasks tab",
    "Notes tab",
    "Calendar tab",
    "Data control",
    "Mutation history",
    "Sync queue",
    "Portable export",
    "Portable archive",
    "Include attachments metadata",
    "Include diagnostics summary",
    "Include local settings",
    "Import portable archive",
    "Local backups",
    "Automatic local backups",
    "Keep backups",
    "Backup folder",
    "History",
    "Visible entries",
    "Storage cap",
    "History categories",
    "Duplicate detection",
    "Title similarity threshold",
    "Custom filters",
    "Pinned filters",
    "Boolean filters",
    "Tags",
    "Tag catalog",
    "Tag colors",
    "Merge tags",
    "Auto tags",
    "Background reapply",
    "Rules",
    "Prefix",
    "Contains",
    "Regex",
    "Task templates",
    "Event templates",
    "Note templates",
    "Customization",
    "CSS snippets",
    "settings.json",
    "keymap.json",
    "Sandboxed extensions",
    "ICS import subscriptions",
    "Local reports"
  ].join(" "),
  alerts: [
    "Alerts",
    "Notifications",
    "Local reminders",
    "Reminder lead time",
    "Completion sounds",
    "Task completion",
    "Task sound",
    "Event completion",
    "Event sound",
    "Glass Pop Chime Click Ding Pluck Tick Sparkle Success Soft bell Arcade Wood Coin Rise Pulse",
    "Imported sounds",
    "Menu bar",
    "Menu bar extra",
    "Menu bar panel",
    "Menu bar icon",
    "Pin Calendar Bun Checklist Target Bell Clock Star Bolt Spark Circle Diamond",
    "Menu bar badge for overdue tasks",
    "Dock",
    "Dock badge for overdue tasks",
    "Global hotkey",
    "Global quick-add hotkey",
    "Shortcut"
  ].join(" "),
  appearance: [
    "Appearance",
    "Theme",
    "Color theme",
    "Custom background",
    "Infer theme from background",
    "Inferred palette",
    "Color preview",
    "Reduce motion",
    "Layout scale",
    "Text size",
    "UI font",
    "Per-surface fonts",
    "Tasks font",
    "Calendar font",
    "Notes font",
    "Menu bar font",
    "Settings font",
    "Layout",
    "Default section",
    "Sidebar labels",
    "Menu bar compact mode",
    "Calendar default view",
    "Calendar day start",
    "Calendar day end"
  ].join(" "),
  general: [
    "General",
    "Language",
    "App language",
    "System Default follows your macOS language order",
    "Startup",
    "Open Hot Cross Buns at login",
    "Diagnostics",
    "Inspect logs mutation history sync queues support bundles",
    "Include performance diagnostics",
    "Include field-redacted Google payloads in local logs",
    "Agent access",
    "Local MCP server",
    "Permission mode",
    "MCP clients must follow this write policy before changes apply",
    "Port",
    "Running",
    "Stopped",
    "Reset token",
    "Sync",
    "Mode",
    "Refresh cadence for launch foreground periodic app activity",
    "Keep past events",
    "Keep completed tasks",
    "Custom",
    "Refresh",
    "Force full resync",
    "Setup",
    "Setup assistant",
    "Run setup again"
  ].join(" "),
  hotkeys: [
    "Hotkeys",
    "Shortcuts",
    "Keyboard",
    "Command palette",
    ...hotkeyDefinitions.map((definition) => `${definition.group} ${definition.label} ${definition.id}`)
  ].join(" "),
  profile: [
    "Profile",
    "Google OAuth client",
    "Google Cloud OAuth client",
    "Desktop OAuth client ID",
    "Client secret optional",
    "Google accounts",
    "Add Google Account",
    "Disconnect",
    "Task lists",
    "Calendars"
  ].join(" ")
};

export function SettingsView({
  onOpenDiagnostics
}: {
  onOpenDiagnostics?: () => void;
} = {}): JSX.Element {
  const source = useCoreViewModelSource();
  const { open: openInspector } = useInspector();
  const [confirmation, setConfirmation] = useState<{
    action: SettingsRecoveryActionRequest["action"];
    phrase: string;
  } | null>(null);
  const [confirmationInput, setConfirmationInput] = useState("");
  const [recoveryMessage, setRecoveryMessage] = useState<string | null>(null);
  const [autoTagBackgroundNotice, setAutoTagBackgroundNotice] = useState<AutoTagBackgroundNotice | null>(null);
  const [selectedSettingsTab, setSelectedSettingsTab] = useState<SettingsTabId>("general");
  const [settingsQuery, setSettingsQuery] = useState("");
  const [customRetentionAmount, setCustomRetentionAmount] = useState("60");
  const [customRetentionUnit, setCustomRetentionUnit] = useState<"days" | "months" | "years">("days");
  const settings = source.settings;
  const diagnostics = source.diagnosticsSummary;
  const googleStatus = source.googleStatus;
  const baseThemeMode = resolveAppThemeMode(settings.theme, currentSystemPrefersDark());
  const effectiveThemeMode = resolveEffectiveThemeMode(settings, currentSystemPrefersDark());
  const matchingColorThemes = appColorThemes.filter(
    (theme) => theme.isDark === (effectiveThemeMode === "dark")
  );
  const activeColorTheme = resolveEffectiveColorTheme(settings, effectiveThemeMode);
  const autoTagBulkCounts = useMemo(() => ({
    task: source.largeTaskWindow.filter((task) => {
      const preview = previewAutoTagRules(settings.autoTagRules, {
        kind: "task",
        title: task.title,
        body: task.detail,
        existingTags: task.tags ?? []
      });
      return previewDiffers(preview.title, task.title) ||
        previewDiffers(preview.body, task.detail) ||
        !sameStringSet(preview.tags, task.tags ?? []);
    }).length,
    event: source.calendarAgendaEvents.filter((event) => {
      if (event.sourceKind === "task" || event.hcbKind === "birthday") {
        return false;
      }

      const preview = previewAutoTagRules(settings.autoTagRules, {
        kind: "event",
        title: event.title,
        body: event.notes,
        existingTags: event.tags ?? [],
        existingEventColorId: event.colorId ?? null
      });
      return previewDiffers(preview.title, event.title) ||
        previewDiffers(preview.body, event.notes) ||
        !sameStringSet(preview.tags, event.tags ?? []) ||
        (preview.eventColorId ?? event.colorId ?? null) !== (event.colorId ?? null);
    }).length,
    note: source.initialNotes.filter((note) => {
      const preview = previewAutoTagRules(settings.autoTagRules, {
        kind: "note",
        title: note.title,
        body: note.body,
        existingTags: note.tags ?? []
      });
      return previewDiffers(preview.title, note.title) ||
        previewDiffers(preview.body, note.body) ||
        !sameStringSet(preview.tags, note.tags ?? []);
    }).length
  }), [settings.autoTagRules, source.calendarAgendaEvents, source.initialNotes, source.largeTaskWindow]);
  const [googleClientId, setGoogleClientId] = useState(googleStatus.clientId ?? "");
  const [googleClientSecret, setGoogleClientSecret] = useState("");
  const [systemFontFamilies, setSystemFontFamilies] = useState<string[]>([]);
  const systemFontFamiliesRequested = useRef(false);
  const availableFontFamilies = useMemo(
    () => fontFamilyOptions(systemFontFamilies, settings.uiFontName),
    [settings.uiFontName, systemFontFamilies]
  );
  const settingsSearchTexts = useMemo<Record<SettingsTabId, string>>(
    () => ({
      ...settingsSearchTextByTab,
      advanced: [
        settingsSearchTextByTab.advanced,
        source.taskLists.map((taskList) => taskList.title).join(" "),
        source.calendarSources.map((calendar) => `${calendar.title} ${calendar.timeZone ?? ""}`).join(" ")
      ].join(" "),
      profile: [
        settingsSearchTextByTab.profile,
        googleStatus.account?.displayName ?? "",
        googleStatus.account?.email ?? "",
        googleStatus.account?.timeZone ?? "",
        source.taskLists.map((taskList) => taskList.title).join(" "),
        source.calendarSources.map((calendar) => `${calendar.title} ${calendar.timeZone ?? ""}`).join(" ")
      ].join(" ")
    }),
    [googleStatus.account, source.calendarSources, source.taskLists]
  );
  const normalizedSettingsQuery = settingsQuery.trim();
  const matchingSettingsTabs = useMemo<SettingsTabId[]>(() => {
    if (!normalizedSettingsQuery) {
      return [];
    }

    return (Object.entries(settingsSearchTexts) as Array<[SettingsTabId, string]>)
      .filter(([, text]) => settingsSearchMatches(text, normalizedSettingsQuery))
      .map(([id]) => id);
  }, [normalizedSettingsQuery, settingsSearchTexts]);
  const autoTagRuleErrorCount = useMemo(
    () =>
      settings.autoTagRules.filter((rule) =>
        validateAutoTagRule(rule).some((issue) => issue.severity === "error")
      ).length,
    [settings.autoTagRules]
  );
  const autoTagRulesSignature = useMemo(() => JSON.stringify(settings.autoTagRules), [settings.autoTagRules]);
  const autoTagRulesSignatureRef = useRef<string | null>(null);
  const autoTagBackgroundTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    setGoogleClientId(googleStatus.clientId ?? "");
  }, [googleStatus.clientId]);

  useEffect(() => {
    if (normalizedSettingsQuery.length < 2 || matchingSettingsTabs.length === 0) {
      return;
    }

    if (selectedSettingsTab !== matchingSettingsTabs[0]) {
      setSelectedSettingsTab(matchingSettingsTabs[0]);
    }
  }, [matchingSettingsTabs, normalizedSettingsQuery, selectedSettingsTab]);

  useEffect(() => {
    if (selectedSettingsTab !== "appearance" || systemFontFamiliesRequested.current || !window.hcb) {
      return;
    }

    systemFontFamiliesRequested.current = true;
    void window.hcb.native.listFontFamilies().then((result) => {
      if (result.ok) {
        setSystemFontFamilies(result.data.families);
      }
    });
  }, [selectedSettingsTab]);

  useEffect(() => {
    const previousSignature = autoTagRulesSignatureRef.current;
    autoTagRulesSignatureRef.current = autoTagRulesSignature;

    if (previousSignature === null || previousSignature === autoTagRulesSignature) {
      return;
    }

    if (autoTagBackgroundTimerRef.current) {
      clearTimeout(autoTagBackgroundTimerRef.current);
    }

    setAutoTagBackgroundNotice(null);

    if (settings.autoTagBackgroundReapplyMode === "manual") {
      return;
    }

    let cancelled = false;
    autoTagBackgroundTimerRef.current = setTimeout(() => {
      void runAutoTagBackgroundReapply(settings.autoTagBackgroundReapplyMode, () => cancelled);
    }, 900);

    return () => {
      cancelled = true;
      if (autoTagBackgroundTimerRef.current) {
        clearTimeout(autoTagBackgroundTimerRef.current);
      }
    };
  }, [autoTagRuleErrorCount, autoTagRulesSignature, settings.autoTagBackgroundReapplyMode]);

  function updateSettings(request: SettingsUpdateRequest): void {
    setRecoveryMessage(null);
    void source.updateSettings(request);
  }

  async function reapplyAutoTags(kind: AutoTagTargetKind): Promise<void> {
    const preview = await source.previewAutoTagReapply({ kind, scope: "all" });

    if (!preview) {
      setRecoveryMessage(`Auto-tag reapply ${kind}: preview failed.`);
      return;
    }

    if (preview.blocked || preview.changed === 0) {
      setRecoveryMessage(`Auto-tag reapply ${kind}: ${preview.message}`);
      return;
    }

    const applied = await source.applyAutoTagReapply({ kind, scope: "all", confirm: true });

    source.refreshUndoStatus();
    source.refresh();
    setRecoveryMessage(
      applied
        ? `Auto-tag reapply ${kind}: ${applied.changed} updated, ${applied.failed} failed.`
        : `Auto-tag reapply ${kind}: apply failed.`
    );
  }

  async function runAutoTagBackgroundReapply(
    mode: SettingsSnapshot["autoTagBackgroundReapplyMode"],
    cancelled: () => boolean
  ): Promise<void> {
    if (autoTagRuleErrorCount > 0) {
      if (!cancelled()) {
        setAutoTagBackgroundNotice({
          title: "Auto-tag reapply blocked",
          description: "Fix auto-tag rule errors before background reapply can run.",
          tone: "warning"
        });
      }
      return;
    }

    const previews: Array<{ kind: AutoTagTargetKind; changed: number }> = [];

    for (const kind of autoTagTargetKinds) {
      const preview = await source.previewAutoTagReapply({ kind, scope: "all" });

      if (cancelled()) {
        return;
      }

      if (!preview || preview.blocked) {
        setAutoTagBackgroundNotice({
          title: "Auto-tag reapply blocked",
          description: preview?.message ?? `Auto-tag ${kind} preview failed.`,
          tone: "warning"
        });
        return;
      }

      if (preview.changed > 0) {
        previews.push({ kind, changed: preview.changed });
      }
    }

    if (previews.length === 0) {
      setAutoTagBackgroundNotice({
        title: "Auto-tag reapply preview",
        description: "No cached tasks, events, or notes need reapply.",
        tone: "info"
      });
      return;
    }

    if (mode === "preview") {
      setAutoTagBackgroundNotice({
        title: "Auto-tag reapply preview",
        description: autoTagBackgroundSummary(previews),
        tone: "info",
        changedKinds: previews
      });
      return;
    }

    await applyAutoTagBackgroundChanges(previews);
  }

  async function applyAutoTagBackgroundChanges(
    changedKinds: Array<{ kind: AutoTagTargetKind; changed: number }>
  ): Promise<void> {
    const applied: Array<{ kind: AutoTagTargetKind; changed: number; failed: number }> = [];

    for (const item of changedKinds) {
      const result = await source.applyAutoTagReapply({ kind: item.kind, scope: "all", confirm: true });

      if (result) {
        applied.push({ kind: item.kind, changed: result.changed, failed: result.failed });
      }
    }

    source.refreshUndoStatus();
    source.refresh();
    setAutoTagBackgroundNotice({
      title: "Auto-tag reapply applied",
      description: autoTagBackgroundAppliedSummary(applied),
      tone: "success"
    });
  }

  function updateBaseTheme(theme: SettingsSnapshot["theme"]): void {
    const nextMode = resolveAppThemeMode(theme, currentSystemPrefersDark());
    const currentColorTheme = resolveAppColorTheme(settings.colorTheme, baseThemeMode);
    const nextColorTheme = currentColorTheme.isDark === (nextMode === "dark")
      ? currentColorTheme
      : defaultAppColorTheme(nextMode);

    updateSettings({
      theme,
      colorTheme: nextColorTheme.id
    });
  }

  function updateSelectedTaskList(taskListId: string, selected: boolean): void {
    const current = new Set(settings.selectedTaskListIds.length > 0
      ? settings.selectedTaskListIds
      : source.taskLists.map((taskList) => taskList.id));

    if (selected) {
      current.add(taskListId);
    } else {
      current.delete(taskListId);
    }

    updateSettings({ selectedTaskListIds: [...current] });
  }

  function updateSelectedCalendar(calendarId: string, selected: boolean): void {
    const current = new Set(settings.selectedCalendarIds.length > 0
      ? settings.selectedCalendarIds
      : source.calendarSources.filter((calendar) => calendar.selected).map((calendar) => calendar.id));

    if (selected) {
      current.add(calendarId);
    } else {
      current.delete(calendarId);
    }

    updateSettings({ selectedCalendarIds: [...current] });
  }

  function beginRecoveryAction(action: SettingsRecoveryActionRequest["action"]): void {
    if (action === "refresh" || action === "resetOnboarding" || action === "checkForUpdates") {
      void runRecovery({ action });
      return;
    }

    setConfirmation({ action, phrase: recoveryPhrase(action) });
    setConfirmationInput("");
  }

  async function runRecovery(request: SettingsRecoveryActionRequest): Promise<void> {
    const result = await source.runRecoveryAction(request);

    if (result) {
      setRecoveryMessage(result.message);
      setConfirmation(null);
      setConfirmationInput("");
    }
  }

  function confirmRecoveryAction(): void {
    if (!confirmation || confirmationInput !== confirmation.phrase) {
      return;
    }

    void runRecovery({
      action: confirmation.action,
      confirmation: {
        accepted: true,
        phrase: confirmationInput
      }
    });
  }

  function copyDiagnosticsPayload(payload: string): void {
    void navigator.clipboard?.writeText(payload);
    setRecoveryMessage("Diagnostics summary copied without credentials, raw Google payloads, MCP bearer tokens, or sensitive bodies.");
  }

  async function openDiagnosticsDetails(): Promise<void> {
    if (onOpenDiagnostics) {
      onOpenDiagnostics();
      return;
    }

    const summaryResult = diagnostics ? null : await window.hcb?.diagnostics.summary();
    const freshDiagnostics = diagnostics ?? (summaryResult?.ok ? summaryResult.data : null);
    const payload = sanitizedJson(freshDiagnostics ?? { rows: source.settingsSections[0]?.rows ?? [] });

    openInspector({
      actions: (
        <Button onClick={() => copyDiagnosticsPayload(payload)} size="sm" variant="primary">
          <Copy aria-hidden="true" size={14} />
          Copy
        </Button>
      ),
      body: (
        <pre
          aria-label="Sanitized diagnostics JSON"
          className="max-h-[70vh] overflow-auto whitespace-pre-wrap rounded-hcbMd border border-border bg-surface-0 p-3 font-mono text-[var(--text-xs)] text-text-primary"
        >
          {payload}
        </pre>
      ),
      id: "diagnostics-summary",
      kind: "diagnostics",
      subtitle: "Sanitized JSON",
      title: "Diagnostics details"
    });
  }

  async function saveGoogleOAuthClient(): Promise<void> {
    setRecoveryMessage(null);

    if (!window.hcb) {
      return;
    }

    const request =
      googleClientSecret.trim().length > 0
        ? { clientId: googleClientId, clientSecret: googleClientSecret.trim() }
        : { clientId: googleClientId };
    const result = await window.hcb.google.saveOAuthClient(request);

    if (result.ok) {
      setRecoveryMessage("Google OAuth client configuration saved.");
      source.setGoogleStatus(result.data);
      return;
    }

    setRecoveryMessage(result.error.message);
  }

  async function beginGoogleOAuth(): Promise<void> {
    setRecoveryMessage(null);

    const result = await window.hcb?.google.beginOAuth();

    if (result?.ok) {
      setRecoveryMessage(result.data.message);
      source.refreshGoogleStatus();
      for (const delayMs of [2_000, 5_000, 10_000]) {
        window.setTimeout(() => source.refreshGoogleStatus(), delayMs);
      }
      return;
    }

    if (result && !result.ok) {
      setRecoveryMessage(result.error.message);
    }
  }

  async function disconnectGoogle(accountId?: string): Promise<void> {
    setRecoveryMessage(null);

    const result = await window.hcb?.google.disconnect(accountId ? { accountId } : {});

    if (result?.ok) {
      setRecoveryMessage("Google account disconnected.");
      source.setGoogleStatus(result.data);
      return;
    }

    if (result && !result.ok) {
      setRecoveryMessage(result.error.message);
    }
  }

  return (
    <div className="grid min-h-0 gap-3">
      <div className="flex min-w-0 items-center gap-1 overflow-x-auto border-b border-border pb-2">
        <SettingsTabButton
          active={selectedSettingsTab === "general"}
          icon={Settings2}
          label="General"
          onClick={() => setSelectedSettingsTab("general")}
        />
        <SettingsTabButton
          active={selectedSettingsTab === "profile"}
          icon={Users}
          label="Profile"
          onClick={() => setSelectedSettingsTab("profile")}
        />
        <SettingsTabButton
          active={selectedSettingsTab === "appearance"}
          icon={Brush}
          label="Appearance"
          onClick={() => setSelectedSettingsTab("appearance")}
        />
        <SettingsTabButton
          active={selectedSettingsTab === "hotkeys"}
          icon={Keyboard}
          label="Hotkeys"
          onClick={() => setSelectedSettingsTab("hotkeys")}
        />
        <SettingsTabButton
          active={selectedSettingsTab === "alerts"}
          icon={Bell}
          label="Alerts"
          onClick={() => setSelectedSettingsTab("alerts")}
        />
        <SettingsTabButton
          active={selectedSettingsTab === "advanced"}
          alertCount={autoTagRuleErrorCount}
          icon={SlidersHorizontal}
          label="Advanced"
          onClick={() => setSelectedSettingsTab("advanced")}
        />
        <SettingsTabButton
          active={selectedSettingsTab === "about"}
          icon={Info}
          label="About"
          onClick={() => setSelectedSettingsTab("about")}
        />
      </div>

      <div className="relative">
        <Search
          aria-hidden="true"
          className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-text-muted"
          size={15}
        />
        <Input
          aria-label="Search settings"
          className="pl-9"
          onChange={(event) => setSettingsQuery(event.currentTarget.value)}
          placeholder="Search settings"
          value={settingsQuery}
        />
      </div>

      {source.settingsMutationError ? (
        <StatusBanner
          description={source.settingsMutationError}
          title="Settings action not applied"
          tone="warning"
        />
      ) : null}
      {autoTagBackgroundNotice ? (
        <StatusBanner
          action={autoTagBackgroundNotice.changedKinds ? (
            <div className="flex shrink-0 items-center gap-2">
              <Button
                onClick={() => void applyAutoTagBackgroundChanges(autoTagBackgroundNotice.changedKinds ?? [])}
                size="sm"
                variant="secondary"
              >
                Apply
              </Button>
              <Button onClick={() => setAutoTagBackgroundNotice(null)} size="sm" variant="ghost">
                Dismiss
              </Button>
            </div>
          ) : undefined}
          description={autoTagBackgroundNotice.description}
          title={autoTagBackgroundNotice.title}
          tone={autoTagBackgroundNotice.tone}
        />
      ) : null}
      {recoveryMessage ? (
        <StatusBanner description={recoveryMessage} title="Settings action applied" tone="success" />
      ) : null}
      {confirmation ? (
        <Panel title="Confirm destructive action" description={confirmation.action}>
          <div className="grid gap-3 p-3">
            <Input
              aria-label="Confirmation phrase"
              onChange={(event) => setConfirmationInput(event.target.value)}
              placeholder={confirmation.phrase}
              value={confirmationInput}
            />
            <div className="flex items-center gap-2">
              <Button
                disabled={confirmationInput !== confirmation.phrase || source.settingsMutationPending}
                onClick={confirmRecoveryAction}
                variant="danger"
              >
                Confirm
              </Button>
              <Button onClick={() => setConfirmation(null)} variant="ghost">
                Cancel
              </Button>
            </div>
          </div>
        </Panel>
      ) : null}

      <SettingsSearchProvider query={settingsQuery}>
        {selectedSettingsTab === "general" ? (
          <GeneralSettingsTab
            beginRecoveryAction={beginRecoveryAction}
            customRetentionAmount={customRetentionAmount}
            customRetentionUnit={customRetentionUnit}
            openDiagnosticsDetails={openDiagnosticsDetails}
            setCustomRetentionAmount={setCustomRetentionAmount}
            setCustomRetentionUnit={setCustomRetentionUnit}
            settings={settings}
            settingsMutationPending={source.settingsMutationPending}
            updateSettings={updateSettings}
          />
        ) : null}

        {selectedSettingsTab === "profile" ? (
          <ProfileSettingsTab
            beginGoogleOAuth={beginGoogleOAuth}
            calendarSources={source.calendarSources}
            disconnectGoogle={disconnectGoogle}
            googleClientId={googleClientId}
            googleClientSecret={googleClientSecret}
            googleStatus={googleStatus}
            saveGoogleOAuthClient={saveGoogleOAuthClient}
            setGoogleClientId={setGoogleClientId}
            setGoogleClientSecret={setGoogleClientSecret}
            settings={settings}
            settingsMutationPending={source.settingsMutationPending}
            taskLists={source.taskLists}
            updateSelectedCalendar={updateSelectedCalendar}
            updateSelectedTaskList={updateSelectedTaskList}
          />
        ) : null}

        {selectedSettingsTab === "appearance" ? (
          <AppearanceSettingsTab
            activeColorTheme={activeColorTheme}
            availableFontFamilies={availableFontFamilies}
            matchingColorThemes={matchingColorThemes}
            settings={settings}
            updateBaseTheme={updateBaseTheme}
            updateSettings={updateSettings}
          />
        ) : null}

        {selectedSettingsTab === "hotkeys" ? (
          <HotkeysSettingsTab
            query={settingsQuery}
            settings={settings}
            updateSettings={updateSettings}
          />
        ) : null}

        {selectedSettingsTab === "alerts" ? (
          <AlertsSettingsTab settings={settings} updateSettings={updateSettings} />
        ) : null}

        {selectedSettingsTab === "advanced" ? (
          <AdvancedSettingsTab
            autoTagBulkCounts={autoTagBulkCounts}
            beginRecoveryAction={beginRecoveryAction}
            calendarSources={source.calendarSources}
            createTag={source.createTag}
            defaultTagColor={activeColorTheme.ember}
            deleteTag={source.deleteTag}
            mergeTags={source.mergeTags}
            onReapplyAutoTags={(kind) => void reapplyAutoTags(kind)}
            settings={settings}
            tags={source.tags}
            taskLists={source.taskLists}
            updateTag={source.updateTag}
            updateSelectedCalendar={updateSelectedCalendar}
            updateSelectedTaskList={updateSelectedTaskList}
            updateSettings={updateSettings}
          />
        ) : null}

        {selectedSettingsTab === "about" ? (
          <AboutSettingsTab
            beginRecoveryAction={beginRecoveryAction}
            diagnostics={diagnostics}
            nativeUpdaterStatus={source.native.updaterStatus}
            settings={settings}
            updateSettings={updateSettings}
          />
        ) : null}
      </SettingsSearchProvider>
    </div>
  );
}

function previewDiffers(left: string, right: string): boolean {
  return left.trim() !== right.trim();
}

function sameStringSet(left: readonly string[], right: readonly string[]): boolean {
  const normalizedLeft = [...new Set(left.map((value) => value.trim()).filter(Boolean))].sort();
  const normalizedRight = [...new Set(right.map((value) => value.trim()).filter(Boolean))].sort();

  return normalizedLeft.length === normalizedRight.length &&
    normalizedLeft.every((value, index) => value === normalizedRight[index]);
}

function autoTagBackgroundSummary(items: Array<{ kind: AutoTagTargetKind; changed: number }>): string {
  const total = items.reduce((sum, item) => sum + item.changed, 0);

  return `Rules changed. ${items.map((item) => autoTagKindLabel(item.kind, item.changed)).join(", ")} ${total === 1 ? "needs" : "need"} reapply.`;
}

function autoTagBackgroundAppliedSummary(items: Array<{ kind: AutoTagTargetKind; changed: number; failed: number }>): string {
  const changed = items.reduce((total, item) => total + item.changed, 0);
  const failed = items.reduce((total, item) => total + item.failed, 0);

  return `${changed} updated${failed > 0 ? `, ${failed} failed` : ""}.`;
}

function autoTagKindLabel(kind: AutoTagTargetKind, count: number): string {
  const noun = kind === "task" ? "task" : kind === "event" ? "event" : "note";

  return `${count} ${noun}${count === 1 ? "" : "s"}`;
}
