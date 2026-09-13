import { execFileSync } from "node:child_process";
import { globalShortcut } from "electron";
import { redactDiagnosticText } from "@shared/redaction";
import type { NativeFeatureState } from "@shared/ipc/contracts";
import type { NativeOperationResult } from "../types";
import { isLinuxUnvalidatedNativeShellEnabled } from "./previewGates";

export interface LinuxGlobalShortcutSupport {
  hasPortalShortcutSupport: boolean;
  hasWaylandSession: boolean;
  message: string;
  sessionType: "wayland" | "x11" | "unknown";
  state: NativeFeatureState;
  supported: boolean;
}

type PortalProbe = () => boolean;

export class LinuxGlobalShortcutRegistry {
  private readonly shortcuts = new Set<string>();

  register(accelerator: string, action: () => void): NativeOperationResult {
    const support = detectLinuxGlobalShortcutSupport();

    if (!support.supported) {
      return {
        ok: false,
        state: support.state,
        message: support.message
      };
    }

    try {
      const registered = globalShortcut.register(accelerator, action);

      if (!registered) {
        return {
          ok: false,
          state: "conflict",
          message: shortcutConflictMessage(accelerator, support)
        };
      }

      this.shortcuts.add(accelerator);

      return {
        ok: true,
        state: "ready",
        message: `${accelerator} is registered for quick capture on ${support.sessionTypeLabel}.`
      };
    } catch (error) {
      return {
        ok: false,
        state: "error",
        message: redactDiagnosticText(
          error instanceof Error && error.message.trim()
            ? error.message
            : `${accelerator} could not be registered as a Linux global shortcut.`
        ).slice(0, 500)
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

export function detectLinuxGlobalShortcutSupport(
  env: NodeJS.ProcessEnv = process.env,
  portalProbe: PortalProbe = probeGlobalShortcutsPortal
): LinuxGlobalShortcutSupport & { sessionTypeLabel: string } {
  const sessionType = normalizeSessionType(env.XDG_SESSION_TYPE);
  const hasWaylandSession = sessionType === "wayland";
  const hasX11Display = Boolean(env.DISPLAY?.trim());
  const hasPortalShortcutSupport = hasWaylandSession
    ? resolvePortalShortcutSupport(env, portalProbe)
    : false;

  if (!isLinuxUnvalidatedNativeShellEnabled(env)) {
    return {
      hasPortalShortcutSupport,
      hasWaylandSession,
      message: "Linux global shortcuts are explicitly unsupported in this technical preview until X11, Wayland portal, denial, conflict, and packaged AppImage behavior are manually validated. In-app quick add remains available.",
      sessionType,
      sessionTypeLabel: sessionType === "unknown" ? "unknown session" : sessionType,
      state: "unsupported",
      supported: false
    };
  }

  if (hasWaylandSession) {
    return {
      hasPortalShortcutSupport,
      hasWaylandSession,
      message: hasPortalShortcutSupport
        ? "Wayland session detected and XDG Desktop Portal GlobalShortcuts is available; registration can still be denied by the user or compositor."
        : "Wayland session detected but XDG Desktop Portal GlobalShortcuts is unavailable. In-app quick add remains available.",
      sessionType,
      sessionTypeLabel: "Wayland",
      state: hasPortalShortcutSupport ? "pending" : "unsupported",
      supported: hasPortalShortcutSupport
    };
  }

  if (sessionType === "x11" || hasX11Display) {
    return {
      hasPortalShortcutSupport: false,
      hasWaylandSession,
      message: "X11 session detected; Electron globalShortcut registration can be attempted but may still conflict with desktop shortcuts.",
      sessionType: "x11",
      sessionTypeLabel: "X11",
      state: "pending",
      supported: true
    };
  }

  return {
    hasPortalShortcutSupport: false,
    hasWaylandSession,
    message: "No X11 display or Wayland global-shortcut portal was detected. In-app quick add remains available.",
    sessionType: "unknown",
    sessionTypeLabel: "unknown session",
    state: "unsupported",
    supported: false
  };
}

function normalizeSessionType(value: string | undefined): LinuxGlobalShortcutSupport["sessionType"] {
  const normalized = value?.trim().toLowerCase();

  return normalized === "wayland" || normalized === "x11" ? normalized : "unknown";
}

function resolvePortalShortcutSupport(env: NodeJS.ProcessEnv, portalProbe: PortalProbe): boolean {
  const override = env.HCB_LINUX_GLOBAL_SHORTCUTS_PORTAL?.trim().toLowerCase();

  if (override === "1" || override === "true" || override === "yes") {
    return true;
  }

  if (override === "0" || override === "false" || override === "no") {
    return false;
  }

  return portalProbe();
}

function probeGlobalShortcutsPortal(): boolean {
  return probeWithBusctl() || probeWithGdbus();
}

function probeWithBusctl(): boolean {
  try {
    const stdout = execFileSync(
      "busctl",
      [
        "--user",
        "--timeout=1",
        "get-property",
        "org.freedesktop.portal.Desktop",
        "/org/freedesktop/portal/desktop",
        "org.freedesktop.portal.GlobalShortcuts",
        "version"
      ],
      {
        encoding: "utf8",
        stdio: ["ignore", "pipe", "ignore"],
        timeout: 1_000
      }
    );

    return /\b[0-9]+\b/.test(stdout);
  } catch {
    return false;
  }
}

function probeWithGdbus(): boolean {
  try {
    const stdout = execFileSync(
      "gdbus",
      [
        "introspect",
        "--session",
        "--dest",
        "org.freedesktop.portal.Desktop",
        "--object-path",
        "/org/freedesktop/portal/desktop"
      ],
      {
        encoding: "utf8",
        stdio: ["ignore", "pipe", "ignore"],
        timeout: 1_000
      }
    );

    return stdout.includes("org.freedesktop.portal.GlobalShortcuts");
  } catch {
    return false;
  }
}

function shortcutConflictMessage(
  accelerator: string,
  support: LinuxGlobalShortcutSupport & { sessionTypeLabel: string }
): string {
  const reason = support.hasWaylandSession
    ? "it may have been denied by the portal, reserved by the compositor, or already in use"
    : "it may already be in use or blocked by the desktop environment";

  return `${accelerator} could not be registered for quick capture on ${support.sessionTypeLabel}; ${reason}. Use in-app quick add or choose another shortcut.`;
}
