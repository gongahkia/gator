import { Menu, Tray, type MenuItemConstructorOptions } from "electron";
import { brandImage } from "../brandAssets";
import type { NativeOperationResult, NativeTrayActions } from "../types";

export class WindowsTrayController {
  private tray: Tray | undefined;
  private refreshTimer: NodeJS.Timeout | undefined;

  create(actions: NativeTrayActions): NativeOperationResult {
    try {
      this.destroy();
      this.tray = new Tray(brandImage("app-icon.png"));
      this.refreshPresentation(actions);
      this.tray.on("click", actions.showOrHideMainWindow);
      this.tray.on("double-click", actions.openMainWindow);
      this.tray.on("right-click", () => {
        this.refreshPresentation(actions);
        this.tray?.popUpContextMenu(windowsTrayMenu(actions));
      });
      this.refreshTimer = setInterval(() => {
        this.refreshPresentation(actions);
      }, 60_000);
      this.refreshTimer.unref?.();

      return {
        ok: true,
        state: "ready",
        message: "Windows notification-area tray icon is installed."
      };
    } catch (error) {
      this.destroy();
      return {
        ok: false,
        state: "error",
        message:
          error instanceof Error && error.message.trim()
            ? `Could not create the Windows tray icon: ${error.message}`
            : "Could not create the Windows tray icon."
      };
    }
  }

  destroy(): void {
    if (this.refreshTimer) {
      clearInterval(this.refreshTimer);
      this.refreshTimer = undefined;
    }

    this.tray?.destroy();
    this.tray = undefined;
  }

  private refreshPresentation(actions: NativeTrayActions): void {
    const snapshot = actions.snapshot();

    this.tray?.setToolTip(snapshot.tooltip);
    this.tray?.setContextMenu(windowsTrayMenu(actions));
  }
}

function windowsTrayMenu(actions: NativeTrayActions): Menu {
  const snapshot = actions.snapshot();
  const template: MenuItemConstructorOptions[] = [
    {
      label: "Open Hot Cross Buns",
      click: actions.openMainWindow
    },
    {
      label: "Show or Hide Window",
      click: actions.showOrHideMainWindow
    },
    {
      label: "Quick Capture",
      click: actions.openQuickAdd
    },
    { type: "separator" },
    {
      label: "Refresh Tasks and Calendar",
      click: actions.refresh
    },
    {
      label: "Settings",
      click: actions.openSettings
    }
  ];

  if (snapshot.statusLabel || snapshot.syncLabel) {
    template.push(
      { type: "separator" },
      {
        label: snapshot.statusLabel ?? snapshot.syncLabel,
        enabled: false
      }
    );
  }

  template.push(
    { type: "separator" },
    {
      label: "Quit",
      click: actions.quit
    }
  );

  return Menu.buildFromTemplate(template);
}
