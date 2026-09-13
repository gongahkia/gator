import { globalShortcut } from "electron";
import type { NativeOperationResult } from "../types";
import { unsupported } from "./operationResults";

export class GlobalShortcutRegistry {
  private readonly shortcuts = new Set<string>();

  register(accelerator: string, action: () => void): NativeOperationResult {
    if (process.platform !== "darwin") {
      return unsupported("Global shortcuts are not registered by this platform adapter.");
    }

    try {
      const registered = globalShortcut.register(accelerator, action);

      if (!registered) {
        return {
          ok: false,
          state: "conflict",
          message: `${accelerator} is already in use or blocked by macOS. Choose another quick capture shortcut in Settings.`
        };
      }

      this.shortcuts.add(accelerator);

      return {
        ok: true,
        state: "ready",
        message: `${accelerator} is registered for quick capture.`
      };
    } catch (error) {
      return {
        ok: false,
        state: "error",
        message:
          error instanceof Error
            ? error.message
            : `${accelerator} could not be registered as a global shortcut.`
      };
    }
  }

  unregister(accelerator?: string): void {
    if (accelerator) {
      globalShortcut.unregister(accelerator);
      this.shortcuts.delete(accelerator);
      return;
    }

    for (const shortcut of this.shortcuts) {
      globalShortcut.unregister(shortcut);
    }

    this.shortcuts.clear();
  }
}
