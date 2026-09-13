import type { SettingsSnapshot } from "@shared/ipc/contracts";
import {
  Bell,
  Command,
  Gauge,
  PanelLeftClose,
  PanelLeftOpen,
  PanelRightOpen,
  RefreshCw,
  Settings2
} from "lucide-react";
import { Badge, Button } from "../../components/primitives";
import { useI18n } from "../../i18n";
import { ariaKeyShortcuts } from "../core/hotkeys";

type ToolbarActionId = SettingsSnapshot["toolbarActionOrder"][number];

export function AppHeader({
  activeSectionTitle,
  appNotificationsCount,
  commandPaletteOpen,
  diagnosticsOpen,
  keybindings,
  notificationsOpen,
  onOpenCommandPalette,
  onOpenSplitPane,
  onRefresh,
  onToggleDiagnostics,
  onToggleNotifications,
  onToggleSettings,
  onToggleSidebar,
  settingsOpen,
  sidebarOpen,
  toolbarActionOrder
}: {
  activeSectionTitle: string;
  appNotificationsCount: number;
  commandPaletteOpen: boolean;
  diagnosticsOpen: boolean;
  keybindings: SettingsSnapshot["keybindings"];
  notificationsOpen: boolean;
  onOpenCommandPalette: () => void;
  onOpenSplitPane: () => void;
  onRefresh: () => void;
  onToggleDiagnostics: () => void;
  onToggleNotifications: () => void;
  onToggleSettings: () => void;
  onToggleSidebar: () => void;
  settingsOpen: boolean;
  sidebarOpen: boolean;
  toolbarActionOrder: SettingsSnapshot["toolbarActionOrder"];
}): JSX.Element {
  const { t } = useI18n();
  const SidebarToggleIcon = sidebarOpen ? PanelLeftClose : PanelLeftOpen;
  const toolbarButtons: Record<ToolbarActionId, JSX.Element> = {
    commandPalette: (
      <Button
        aria-expanded={commandPaletteOpen}
        aria-keyshortcuts={ariaKeyShortcuts(keybindings["commandPalette.open"])}
        aria-label={t("action.commandPalette")}
        className="min-w-8"
        key="commandPalette"
        onClick={() => onOpenCommandPalette()}
        title={t("action.commandPalette")}
        variant={commandPaletteOpen ? "secondary" : "ghost"}
      >
        <Command aria-hidden="true" size={15} />
      </Button>
    ),
    notifications: (
      <Button
        aria-expanded={notificationsOpen}
        aria-keyshortcuts={ariaKeyShortcuts(keybindings["navigation.notifications.toggle"])}
        aria-label={`${t("action.notifications")}, ${appNotificationsCount} active`}
        className="min-w-8"
        key="notifications"
        onClick={onToggleNotifications}
        title={t("action.notifications")}
        variant={notificationsOpen ? "secondary" : "ghost"}
      >
        <Bell aria-hidden="true" size={15} />
        <Badge tone={appNotificationsCount > 1 ? "warning" : "neutral"}>
          {appNotificationsCount}
        </Badge>
      </Button>
    ),
    diagnostics: (
      <Button
        aria-expanded={diagnosticsOpen}
        aria-keyshortcuts={ariaKeyShortcuts(keybindings["navigation.diagnostics.toggle"])}
        aria-label={t("action.diagnostics")}
        className="min-w-8"
        key="diagnostics"
        onClick={onToggleDiagnostics}
        title={t("action.diagnostics")}
        variant={diagnosticsOpen ? "secondary" : "ghost"}
      >
        <Gauge aria-hidden="true" size={15} />
      </Button>
    ),
    splitPane: (
      <Button
        aria-keyshortcuts={ariaKeyShortcuts(keybindings["pane.split.horizontal"])}
        aria-label={t("action.splitView")}
        className="min-w-8"
        key="splitPane"
        onClick={onOpenSplitPane}
        title={t("action.splitView")}
        variant="ghost"
      >
        <PanelRightOpen aria-hidden="true" size={15} />
      </Button>
    ),
    refresh: (
      <Button
        aria-keyshortcuts={ariaKeyShortcuts(keybindings["sync.refresh"])}
        aria-label={t("action.refresh")}
        className="min-w-8"
        data-action-id="sync.refresh"
        key="refresh"
        onClick={onRefresh}
        title={t("action.refresh")}
        variant="ghost"
      >
        <RefreshCw aria-hidden="true" size={15} />
      </Button>
    ),
    settings: (
      <Button
        aria-expanded={settingsOpen}
        aria-keyshortcuts={ariaKeyShortcuts(keybindings["navigation.settings"])}
        aria-label={t("action.settings")}
        className="min-w-8"
        key="settings"
        onClick={onToggleSettings}
        title={t("action.settings")}
        variant={settingsOpen ? "secondary" : "ghost"}
      >
        <Settings2 aria-hidden="true" size={15} />
      </Button>
    )
  };

  return (
    <header className="flex min-h-14 shrink-0 flex-wrap items-center justify-between gap-3 border-b border-border bg-bg-primary px-3 py-2 sm:flex-nowrap md:px-5">
      <div className="flex min-w-0 items-center gap-3">
        <Button
          aria-controls="app-sidebar"
          aria-expanded={sidebarOpen}
          aria-keyshortcuts={ariaKeyShortcuts(keybindings["navigation.sidebar.toggle"])}
          aria-label={sidebarOpen ? t("nav.hideSidebar") : t("nav.showSidebar")}
          className="min-w-8"
          onClick={onToggleSidebar}
          title={sidebarOpen ? t("nav.hideSidebar") : t("nav.showSidebar")}
          variant="ghost"
        >
          <SidebarToggleIcon aria-hidden="true" size={15} />
        </Button>
        <h1 className="sr-only" id="planner-title">{activeSectionTitle}</h1>
      </div>

      <div className="flex min-w-0 shrink-0 items-center gap-2 overflow-x-auto" role="toolbar" aria-label="Planner actions">
        {toolbarActionOrder.map((actionId) => toolbarButtons[actionId])}
      </div>
    </header>
  );
}
