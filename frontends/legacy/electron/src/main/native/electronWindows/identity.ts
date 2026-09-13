import { app } from "electron";
import type { NativeOperationResult } from "../types";

export const windowsAppUserModelId = "dev.hotcrossbuns.hotcrossbuns";

export function applyWindowsAppIdentity(
  platform: NodeJS.Platform | string = process.platform
): NativeOperationResult {
  if (platform !== "win32") {
    return {
      ok: false,
      state: "unsupported",
      message: "Windows AppUserModelID is only applied on Windows."
    };
  }

  try {
    app.setAppUserModelId?.(windowsAppUserModelId);

    return {
      ok: true,
      state: "ready",
      message: `Windows AppUserModelID is set to ${windowsAppUserModelId}.`
    };
  } catch {
    return {
      ok: false,
      state: "error",
      message: "Windows AppUserModelID could not be set."
    };
  }
}
