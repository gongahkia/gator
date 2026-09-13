import type { NativeAction } from "@shared/ipc/contracts";
import type { NativeNotificationRequest, NativePlannerSnapshotSource } from "./types";

const notificationHorizonMs = 24 * 60 * 60 * 1000;
const defaultEventReminderMs = 10 * 60 * 1000;
const immediateNotificationDelayMs = 1_000;
const maxScheduledNotifications = 40;

interface BuildNativeNotificationRequestsOptions {
  planner: NativePlannerSnapshotSource;
  now: Date;
  leadMinutes: number;
}

export function buildNativeNotificationRequests(
  options: BuildNativeNotificationRequestsOptions
): NativeNotificationRequest[] {
  const horizon = new Date(options.now.getTime() + notificationHorizonMs);

  return [
    ...taskNotificationRequests(options.planner, options.now, horizon),
    ...eventNotificationRequests(options.planner, options.now, horizon, options.leadMinutes)
  ].slice(0, maxScheduledNotifications);
}

function taskNotificationRequests(
  planner: NativePlannerSnapshotSource,
  now: Date,
  horizon: Date
): NativeNotificationRequest[] {
  try {
    return planner
      .listTasks({ status: "active", limit: 100 })
      .items.flatMap((task) => {
        if (!task.dueAt) {
          return [];
        }

        const dueAt = dateFromIso(task.dueAt);

        if (!dueAt || dueAt.getTime() > horizon.getTime()) {
          return [];
        }

        const deliveryDate =
          dueAt.getTime() <= now.getTime()
            ? new Date(now.getTime() + immediateNotificationDelayMs)
            : dueAt;

        return [
          {
            id: `task:${task.id}`,
            title: "Task due",
            body: task.title,
            deliveryDate,
            action: {
              type: "openRoute",
              route: {
                kind: "task",
                id: task.id
              }
            } satisfies NativeAction
          }
        ];
      });
  } catch {
    return [];
  }
}

function eventNotificationRequests(
  planner: NativePlannerSnapshotSource,
  now: Date,
  horizon: Date,
  leadMinutes: number
): NativeNotificationRequest[] {
  try {
    return planner
      .listCalendarEvents({
        start: now.toISOString(),
        end: horizon.toISOString(),
        limit: 100
      })
      .items.flatMap((event) => {
        const startsAt = dateFromIso(event.startsAt);

        if (!startsAt || startsAt.getTime() < now.getTime()) {
          return [];
        }

        const reminderMs = leadMinutes > 0 ? leadMinutes * 60_000 : defaultEventReminderMs;
        const reminderAt = new Date(startsAt.getTime() - reminderMs);
        const deliveryDate =
          reminderAt.getTime() <= now.getTime()
            ? new Date(now.getTime() + immediateNotificationDelayMs)
            : reminderAt;

        if (deliveryDate.getTime() > horizon.getTime()) {
          return [];
        }

        return [
          {
            id: `event:${event.id}`,
            title: "Upcoming event",
            body: event.title,
            deliveryDate,
            action: {
              type: "openRoute",
              route: {
                kind: "event",
                id: event.id
              }
            } satisfies NativeAction
          }
        ];
      });
  } catch {
    return [];
  }
}

function dateFromIso(value: string): Date | null {
  const date = new Date(value);

  return Number.isFinite(date.getTime()) ? date : null;
}
