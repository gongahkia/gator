import type { CoreViewModelSource } from "./coreViewModelSource";

export type AppNotificationTone = "info" | "success" | "warning" | "danger" | "offline";

export interface AppNotification {
  id: string;
  title: string;
  description: string;
  status: string;
  tone: AppNotificationTone;
}

export function getAppNotifications(source: CoreViewModelSource): AppNotification[] {
  const notifications: AppNotification[] = [];
  const googleConnectionState = source.googleStatus.account?.connectionState;

  if (source.dataState === "loading") {
    notifications.push({
      id: "cache.loading",
      title: "Loading planner data",
      description: "Opening the synced planner workspace.",
      status: "Loading",
      tone: "info"
    });
  } else if (source.dataState === "error") {
    notifications.push({
      id: "cache.error",
      title: "Planner data unavailable",
      description: source.errorMessage ?? "The planner data request failed.",
      status: "Error",
      tone: "danger"
    });
  } else if (source.dataState === "offline") {
    notifications.push({
      id: "cache.offline",
      title: "Offline",
      description: source.errorMessage ?? "The preload bridge is unavailable in this renderer context.",
      status: "Offline",
      tone: "offline"
    });
  } else if (source.isStale || source.dataState === "stale") {
    notifications.push({
      id: "cache.stale",
      title: "Refreshing planner data",
      description: "Rendering current rows while a newer read is pending.",
      status: "Stale",
      tone: "info"
    });
  } else if (source.dataState === "empty") {
    notifications.push({
      id: "cache.empty",
      title: "No planner data",
      description: "No tasks, events, or notes are available yet.",
      status: "Empty",
      tone: "warning"
    });
  } else {
    notifications.push({
      id: "cache.ready",
      title: "Planner data ready",
      description: "Tasks, events, notes, and settings are loaded.",
      status: "Ready",
      tone: "success"
    });
  }

  if (
    source.dataState !== "loading" &&
    source.dataState !== "offline" &&
    googleConnectionState !== "connected"
  ) {
    notifications.push({
      id: "google.disconnected",
      title: "Google account not connected",
      description: "Showing local planner data. Connect Google to sync changes.",
      status: "Google",
      tone: "warning"
    });
  }

  if (hasFailedGoogleWrite(source)) {
    notifications.push({
      id: "google.write.failed",
      title: "Some changes need attention",
      description: "One or more Google updates did not finish. Open Diagnostics to review them.",
      status: "Action needed",
      tone: "danger"
    });
  }

  if (source.hydrationState === "failed") {
    notifications.push({
      id: "cache.hydration.failed",
      title: "Some counts could not refresh",
      description: "Tasks and notes are still usable, but some sidebar counts could not be updated. Use Reload to retry.",
      status: "Counts",
      tone: "warning"
    });
  }

  if (source.settingsMutationError) {
    notifications.push({
      id: "settings.error",
      title: "Settings action not applied",
      description: source.settingsMutationError,
      status: "Settings",
      tone: "warning"
    });
  }

  if (source.taskMutationError) {
    notifications.push({
      id: "tasks.error",
      title: "Task action not applied",
      description: source.taskMutationError,
      status: "Tasks",
      tone: "warning"
    });
  }

  return notifications;
}

function hasFailedGoogleWrite(source: CoreViewModelSource): boolean {
  if ((source.diagnosticsSummary?.pendingMutations.failedCount ?? 0) > 0) {
    return true;
  }

  return (
    source.largeTaskWindow.some((task) => task.mutationState === "failed") ||
    source.calendarAgendaEvents.some((event) => event.mutationState === "failed") ||
    Object.values(source.calendarEventsById).some((event) => event.mutationState === "failed") ||
    source.scheduledTaskBlocks.some((block) => block.mutationState === "failed")
  );
}
