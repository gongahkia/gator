import { BrowserWindow, screen, type Rectangle } from "electron";
import type { NativeMenuBarSnapshot, NativeTrayActions } from "../types";
import { menuBarPanelDataUrl } from "./menuBarPanelHtml";
import { parseMenuBarPanelUrl } from "./menuBarPanelNavigation";

const menuBarPanelGap = 2;

export class MenuBarPanelController {
  private menuBarPanelWindow: BrowserWindow | undefined;

  async toggle(
    actions: NativeTrayActions,
    snapshot: NativeMenuBarSnapshot,
    trayBounds: Rectangle
  ): Promise<void> {
    if (this.menuBarPanelWindow?.isVisible()) {
      this.menuBarPanelWindow.hide();
      return;
    }

    const panel = this.ensure(actions);
    panel.setBounds(menuBarPanelBounds(trayBounds, snapshot));
    await panel.loadURL(menuBarPanelDataUrl(snapshot));
    panel.show();
    panel.focus();
  }

  destroy(): void {
    if (!this.menuBarPanelWindow || this.menuBarPanelWindow.isDestroyed()) {
      this.menuBarPanelWindow = undefined;
      return;
    }

    this.menuBarPanelWindow.destroy();
    this.menuBarPanelWindow = undefined;
  }

  private ensure(actions: NativeTrayActions): BrowserWindow {
    if (this.menuBarPanelWindow && !this.menuBarPanelWindow.isDestroyed()) {
      return this.menuBarPanelWindow;
    }

    const panel = new BrowserWindow({
      width: 340,
      height: 640,
      show: false,
      frame: false,
      transparent: true,
      resizable: false,
      minimizable: false,
      maximizable: false,
      fullscreenable: false,
      movable: false,
      skipTaskbar: true,
      alwaysOnTop: true,
      title: "Hot Cross Buns menu bar panel",
      backgroundColor: "#00000000",
      webPreferences: {
        contextIsolation: true,
        javascript: false,
        nodeIntegration: false,
        sandbox: true,
        webSecurity: true
      }
    });

    panel.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true });
    panel.setAlwaysOnTop(true, "pop-up-menu");
    panel.on("blur", () => panel.hide());
    panel.on("closed", () => {
      if (this.menuBarPanelWindow === panel) {
        this.menuBarPanelWindow = undefined;
      }
    });
    panel.webContents.setWindowOpenHandler(() => ({ action: "deny" }));
    panel.webContents.on("will-navigate", (event, url) => {
      if (!url.startsWith("hcb-panel://")) {
        event.preventDefault();
        return;
      }

      event.preventDefault();
      this.handleNavigation(url, actions);
    });

    this.menuBarPanelWindow = panel;
    return panel;
  }

  private handleNavigation(url: string, actions: NativeTrayActions): void {
    this.menuBarPanelWindow?.hide();

    const parsed = parseMenuBarPanelUrl(url);

    if (!parsed) {
      return;
    }

    if (parsed.kind === "route") {
      actions.openRoute(parsed.route);
    } else if (parsed.action === "refresh") {
      actions.refresh();
    } else if (parsed.action === "openSettings") {
      actions.openSettings();
    } else if (parsed.action === "showWindow") {
      actions.openMainWindow();
    } else if (parsed.action === "quit") {
      actions.quit();
    }
  }
}

function menuBarPanelBounds(trayBounds: Rectangle, snapshot: NativeMenuBarSnapshot): Rectangle {
  const display = screen.getDisplayMatching(trayBounds);
  const workArea = display.workArea;
  const size = menuBarPanelSize(snapshot, Math.max(240, workArea.height - 48));
  const x = clamp(
    Math.round(trayBounds.x + trayBounds.width / 2 - size.width / 2),
    workArea.x + menuBarPanelGap,
    workArea.x + workArea.width - size.width - menuBarPanelGap
  );
  const menuBarIsAboveWorkArea = trayBounds.y < workArea.y + workArea.height / 2;
  const y = menuBarIsAboveWorkArea
    ? Math.min(
        trayBounds.y + trayBounds.height + menuBarPanelGap,
        workArea.y + workArea.height - size.height - menuBarPanelGap
      )
    : Math.max(
        workArea.y + menuBarPanelGap,
        trayBounds.y - size.height - menuBarPanelGap
      );

  return {
    x,
    y: Math.round(y),
    width: size.width,
    height: size.height
  };
}

function menuBarPanelSize(
  snapshot: NativeMenuBarSnapshot,
  maxHeight: number
): { width: number; height: number } {
  if (snapshot.panelStyle === "calendar") {
    const selectedRows = snapshot.calendar?.selectedItems.length ?? 0;
    return {
      width: 320,
      height: Math.min(maxHeight, Math.max(620, 474 + Math.min(selectedRows, 7) * 42))
    };
  }

  const rowCount = snapshot.sections.reduce((total, section) => total + section.items.length, 0);
  const accountHeight = snapshot.account ? 78 : 0;

  return {
    width: 340,
    height: Math.min(maxHeight, Math.max(300, 132 + rowCount * 44 + accountHeight + 112))
  };
}

function clamp(value: number, min: number, max: number): number {
  if (max < min) {
    return min;
  }

  return Math.min(max, Math.max(min, value));
}
