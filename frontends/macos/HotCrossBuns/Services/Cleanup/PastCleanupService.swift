import Foundation

// Past-cleanup engine. Pure functions that inspect the current mirrors
// and the user's settings, decide what to delete, and issue the deletes
// through AppModel's existing mutation paths (so optimistic writes,
// offline queue, and audit logging all flow the same way). Visibility-
// only modes (dim / hide) are handled in views — this service is only
// about deletion.
//
// Operates in two phases per invocation:
//   1. computePreview() — dry run; returns the list of candidates.
//   2. execute(preview:) — fires deletes; returns counts + errors.
// Callers can show the preview to the user before execute, or skip the
// preview and go straight to execute for silent recurring sweeps after
// the user has acknowledged once.
//
// Safety rules baked in:
//   - Recurring master events (event.recurrence non-empty AND id has no
//     instance suffix) are NEVER deleted. The series stays alive.
//   - Past instances of recurring events (event.id has the _YYYYMMDD…
//     suffix) are fair game.
//   - Events with attendees are skipped unless the user has explicitly
//     opted in (allowDeletingAttendeeEvents). Deleting an attendee event
//     may notify/cancel for other people, so this requires consent.
//   - Completed tasks only — overdue-but-open tasks are never deleted.
//     Hiding overdue tasks is allowed (view-layer), never deletion.
struct PastCleanupPreview: Equatable, Sendable {
    var events: [CalendarEventMirror]
    var attendeeEventsSkipped: [CalendarEventMirror] // excluded because attendees + opt-out
    var recurringMastersSkipped: [CalendarEventMirror]
    var completedTasks: [TaskMirror]

    var totalDeletableCount: Int { events.count + completedTasks.count }
    var isEmpty: Bool { totalDeletableCount == 0 }
}

struct PastCleanupResult: Equatable, Sendable {
    var eventsDeleted: Int
    var tasksDeleted: Int
    var eventErrors: Int
    var taskErrors: Int

    var didAnything: Bool { eventsDeleted + tasksDeleted > 0 }
}

enum PastCleanupService {
    // Pure: which items in the current mirrors would be deleted under
    // the given settings at the given wall-clock. No mutations.
    static func computePreview(
        events: [CalendarEventMirror],
        tasks: [TaskMirror],
        settings: AppSettings,
        now: Date,
        calendar: Calendar = .current
    ) -> PastCleanupPreview {
        var eventCandidates: [CalendarEventMirror] = []
        var attendeeSkipped: [CalendarEventMirror] = []
        var masterSkipped: [CalendarEventMirror] = []

        if settings.pastEventBehavior.isDeletion {
            let eventCutoff = calendar.date(byAdding: .day, value: -settings.pastEventDeleteThresholdDays, to: now) ?? now
            for event in events {
                guard event.endDate < eventCutoff else { continue }
                // Skip the master event of a recurring series — deleting
                // that nukes the series and future occurrences. Only the
                // instance IDs (past occurrences) are eligible.
                let isMaster = event.recurrence.isEmpty == false && CalendarEventInstance.isInstanceID(event.id) == false
                if isMaster {
                    masterSkipped.append(event)
                    continue
                }
                if event.attendeeEmails.isEmpty == false, settings.allowDeletingAttendeeEvents == false {
                    attendeeSkipped.append(event)
                    continue
                }
                eventCandidates.append(event)
            }
        }

        var taskCandidates: [TaskMirror] = []
        if settings.completedTaskBehavior.isDeletion {
            let taskCutoff = calendar.date(byAdding: .day, value: -settings.completedTaskDeleteThresholdDays, to: now) ?? now
            for task in tasks where task.isDeleted == false && task.isCompleted {
                guard let completed = task.completedAt, completed < taskCutoff else { continue }
                taskCandidates.append(task)
            }
        }

        return PastCleanupPreview(
            events: eventCandidates,
            attendeeEventsSkipped: attendeeSkipped,
            recurringMastersSkipped: masterSkipped,
            completedTasks: taskCandidates
        )
    }
}

// AppModel-facing coordinator. Keeps the service pure by putting the
// mutation calls here; tests exercise the pure preview builder without
// needing a mock AppModel.
@MainActor
final class PastCleanupCoordinator {
    private unowned let model: AppModel
    private var dailyTickTask: Task<Void, Never>?

    init(model: AppModel) {
        self.model = model
    }

    // Build a preview from current in-memory state + settings.
    func currentPreview(now: Date = Date()) -> PastCleanupPreview {
        PastCleanupService.computePreview(
            events: model.events,
            tasks: model.tasks,
            settings: model.settings,
            now: now
        )
    }

    // Execute a preview. Fires deletes in sequence so we don't flood
    // the quota and so conflict resolution against the optimistic writer
    // doesn't get tangled. Errors accumulate into counts but don't stop
    // the run — best-effort cleanup.
    func execute(_ preview: PastCleanupPreview) async -> PastCleanupResult {
        var eventsDeleted = 0
        var tasksDeleted = 0
        var eventErrors = 0
        var taskErrors = 0

        for event in preview.events {
            let scope: AppModel.RecurringEventScope = CalendarEventInstance.isInstanceID(event.id) ? .thisOccurrence : .thisOccurrence
            let ok = await model.deleteEvent(event, scope: scope)
            if ok { eventsDeleted += 1 } else { eventErrors += 1 }
        }
        for task in preview.completedTasks {
            let ok = await model.deleteTask(task)
            if ok { tasksDeleted += 1 } else { taskErrors += 1 }
        }

        return PastCleanupResult(
            eventsDeleted: eventsDeleted,
            tasksDeleted: tasksDeleted,
            eventErrors: eventErrors,
            taskErrors: taskErrors
        )
    }

    // Compute + execute. Only issues deletes if the user has already
    // acknowledged the blast-radius modal for each enabled delete mode.
    // Callers without acknowledgement should route through the Settings
    // modal path instead.
    func runSilentSweepIfAcknowledged() async -> PastCleanupResult? {
        var preview = currentPreview()
        if model.settings.pastEventBehavior.isDeletion, model.settings.hasAckedEventDeletion == false {
            preview.events = []
            preview.attendeeEventsSkipped = []
        }
        if model.settings.completedTaskBehavior.isDeletion, model.settings.hasAckedTaskDeletion == false {
            preview.completedTasks = []
        }
        if preview.isEmpty { return nil }
        return await execute(preview)
    }

    // Daily tick — rescheduled on every settings change so toggling a
    // delete mode on kicks off the next sweep at midnight (local).
    func scheduleDailyTick() {
        dailyTickTask?.cancel()
        dailyTickTask = Task { [weak self] in
            while Task.isCancelled == false {
                // Sleep until the next local midnight. Falls back to 24h
                // if the calendar date math hits an edge case.
                let interval = self?.secondsUntilNextMidnight() ?? 60 * 60 * 24
                try? await Task.sleep(for: .seconds(interval))
                if Task.isCancelled { break }
                _ = await self?.runSilentSweepIfAcknowledged()
            }
        }
    }

    func cancelDailyTick() {
        dailyTickTask?.cancel()
        dailyTickTask = nil
    }

    private func secondsUntilNextMidnight() -> TimeInterval {
        let cal = Calendar.current
        let tomorrow = cal.startOfDay(for: Date().addingTimeInterval(86_400))
        return max(60, tomorrow.timeIntervalSince(Date()))
    }
}
