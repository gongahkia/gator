import { Menu, app, type MenuItemConstructorOptions } from "electron";
import { brandImage } from "../brandAssets";
import type { NativeOperationResult, NativeTrayActions } from "../types";
import { unsupported } from "./operationResults";

export function installAppMenu(actions: NativeTrayActions): NativeOperationResult {
  if (process.platform !== "darwin") {
    return unsupported("macOS application menu is unavailable on this platform.");
  }

  app.setName(macAppDisplayName());

  const dockIcon = brandImage("app-icon.png");
  if (!dockIcon.isEmpty()) {
    app.dock?.setIcon(dockIcon);
  }

  Menu.setApplicationMenu(Menu.buildFromTemplate(appMenuTemplate(actions)));

  return {
    ok: true,
    state: "ready",
    message: "macOS application menu is installed."
  };
}

function macAppDisplayName(): string {
  const currentName = app.name?.trim();
  return currentName && currentName !== "Electron" ? currentName : "Hot Cross Buns";
}

function appMenuTemplate(actions: NativeTrayActions): MenuItemConstructorOptions[] {
  const appName = macAppDisplayName();

  return [
    {
      label: appName,
      submenu: [
        { label: `About ${appName}`, role: "about" },
        { type: "separator" },
        {
          label: "Settings...",
          accelerator: "Command+,",
          click: actions.openSettings
        },
        { type: "separator" },
        { role: "services" },
        { type: "separator" },
        { label: `Hide ${appName}`, role: "hide" },
        { role: "hideOthers" },
        { role: "unhide" },
        { type: "separator" },
        {
          label: `Quit ${appName}`,
          accelerator: "Command+Q",
          click: actions.quit
        }
      ]
    },
    {
      label: "File",
      submenu: [
        {
          label: "Open Today",
          accelerator: "Command+1",
          click: () => actions.openRoute({ kind: "today" })
        },
        { type: "separator" },
        { role: "close" }
      ]
    },
    {
      label: "Edit",
      submenu: [
        { role: "undo" },
        { role: "redo" },
        { type: "separator" },
        { role: "cut" },
        { role: "copy" },
        { role: "paste" },
        { role: "pasteAndMatchStyle" },
        { role: "delete" },
        { role: "selectAll" }
      ]
    },
    {
      label: "View",
      submenu: [
        {
          label: "Today",
          accelerator: "Command+1",
          click: () => actions.openRoute({ kind: "today" })
        },
        {
          label: "Tasks",
          accelerator: "Command+2",
          click: () => actions.openRoute({ kind: "tasks" })
        },
        {
          label: "Calendar",
          accelerator: "Command+3",
          click: () => actions.openRoute({ kind: "calendar" })
        },
        {
          label: "Notes",
          accelerator: "Command+4",
          click: () => actions.openRoute({ kind: "notes" })
        },
        {
          label: "Search",
          accelerator: "Command+5",
          click: () => actions.openRoute({ kind: "search" })
        },
        { type: "separator" },
        { role: "resetZoom" },
        { role: "zoomIn" },
        { role: "zoomOut" },
        { type: "separator" },
        { role: "toggleDevTools" },
        { type: "separator" },
        { role: "togglefullscreen" }
      ]
    },
    {
      label: "Sync",
      submenu: [
        {
          label: "Sync Now",
          accelerator: "Command+R",
          click: actions.refresh
        },
        {
          label: "Sync Settings",
          click: actions.openSettings
        }
      ]
    },
    {
      label: "Tasks",
      submenu: [
        {
          label: "Open Tasks",
          accelerator: "Command+2",
          click: () => actions.openRoute({ kind: "tasks" })
        },
        {
          label: "Search Tasks",
          click: () => actions.openRoute({ kind: "search", query: "tasks" })
        }
      ]
    },
    {
      label: "Calendar",
      submenu: [
        {
          label: "Open Calendar",
          accelerator: "Command+3",
          click: () => actions.openRoute({ kind: "calendar" })
        },
        {
          label: "Open Today",
          accelerator: "Command+1",
          click: () => actions.openRoute({ kind: "today" })
        },
        {
          label: "Refresh Calendar",
          click: actions.refresh
        }
      ]
    },
    {
      label: "Window",
      submenu: [
        {
          label: `Show ${appName}`,
          click: actions.openMainWindow
        },
        {
          label: "Hide Window",
          click: actions.showOrHideMainWindow
        },
        { type: "separator" },
        { role: "minimize" },
        { role: "zoom" },
        { type: "separator" },
        { role: "front" }
      ]
    },
    {
      role: "help",
      submenu: [
        {
          label: "Search",
          accelerator: "Command+5",
          click: () => actions.openRoute({ kind: "search" })
        },
        {
          label: "Settings",
          click: actions.openSettings
        }
      ]
    }
  ];
}
