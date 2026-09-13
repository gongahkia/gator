import { Notification, type NotificationConstructorOptions } from "electron";
import type {
  NativeNotificationRequest,
  ScheduledNativeNotification
} from "./types";

const maxNotificationDelayMs = 2_147_483_647;

type ElectronNotificationPlatform = "darwin" | "linux" | "win32";

export class ElectronNotificationScheduler {
  private readonly notificationTimers = new Map<string, NodeJS.Timeout>();
  private readonly activeNotifications = new Map<string, Notification>();

  constructor(
    private readonly platform: ElectronNotificationPlatform,
    private readonly currentPlatform: NodeJS.Platform | string = process.platform
  ) {}

  schedule(
    request: NativeNotificationRequest,
    onClick: () => void,
    onFailure?: (message: string) => void
  ): ScheduledNativeNotification | undefined {
    if (this.currentPlatform !== this.platform || !Notification.isSupported()) {
      return undefined;
    }

    const delayMs = Math.max(0, request.deliveryDate.getTime() - Date.now());

    if (delayMs > maxNotificationDelayMs) {
      return undefined;
    }

    const timer = setTimeout(() => {
      this.notificationTimers.delete(request.id);
      this.show(request, onClick, onFailure);
    }, delayMs);

    timer.unref?.();
    this.notificationTimers.set(request.id, timer);

    return {
      id: request.id,
      cancel: () => {
        clearTimeout(timer);
        this.notificationTimers.delete(request.id);
        this.closeActiveNotification(request.id);
      }
    };
  }

  clear(): void {
    for (const timer of this.notificationTimers.values()) {
      clearTimeout(timer);
    }

    this.notificationTimers.clear();

    for (const notification of this.activeNotifications.values()) {
      notification.close();
    }

    this.activeNotifications.clear();
  }

  private show(
    request: NativeNotificationRequest,
    onClick: () => void,
    onFailure?: (message: string) => void
  ): void {
    try {
      const notification = new Notification(notificationOptions(this.platform, request));
      const cleanup = () => {
        this.activeNotifications.delete(request.id);
      };

      this.activeNotifications.set(request.id, notification);
      notification.on("click", () => {
        cleanup();
        onClick();
      });
      notification.on("close", cleanup);
      notification.on("failed", (_event, error) => {
        cleanup();
        onFailure?.(notificationFailureMessage(error));
      });
      notification.show();
    } catch (error) {
      onFailure?.(notificationFailureMessage(error));
    }
  }

  private closeActiveNotification(id: string): void {
    const notification = this.activeNotifications.get(id);

    if (!notification) {
      return;
    }

    this.activeNotifications.delete(id);
    notification.close();
  }
}

function notificationOptions(
  platform: ElectronNotificationPlatform,
  request: NativeNotificationRequest
): NotificationConstructorOptions {
  const options: NotificationConstructorOptions = {
    title: request.title,
    body: request.body
  };

  if (platform === "linux") {
    options.timeoutType = "default";
    options.urgency = "normal";
  }

  return options;
}

function notificationFailureMessage(error: unknown): string {
  const detail =
    typeof error === "string"
      ? error
      : error instanceof Error
        ? error.message
        : "";

  return detail.trim()
    ? `Native notification failed: ${detail}`
    : "Native notification failed.";
}
