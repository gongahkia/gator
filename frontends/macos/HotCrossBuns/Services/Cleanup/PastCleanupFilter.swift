import Foundation

// View-side helpers for the dim/hide branches of PastEventBehavior and
// OverdueTaskBehavior / CompletedTaskBehavior. Pure functions so they
// can be threaded through snapshots without touching the AppModel.
//
// Semantics:
//   - Past event: endDate < now
//   - Overdue task: not completed AND due date before start-of-today
//   - Completed task: status == completed
extension AppSettings {
    // Filtering (hide mode).
    func shouldHidePastEvent(_ event: CalendarEventMirror, now: Date) -> Bool {
        pastEventBehavior == .hide && event.endDate < now
    }

    func shouldHideOverdueTask(_ task: TaskMirror, now: Date, calendar: Calendar = .current) -> Bool {
        guard overdueTaskBehavior == .hide else { return false }
        guard task.isCompleted == false, let due = task.dueDate else { return false }
        return due < calendar.startOfDay(for: now)
    }

    func shouldHideCompletedTask(_ task: TaskMirror) -> Bool {
        completedTaskBehavior == .hide && task.isCompleted
    }

    // Dimming (dim mode). Returns an opacity multiplier; 1.0 if no dim
    // applies. Callers multiply this into their existing opacity.
    func opacityForPastEvent(_ event: CalendarEventMirror, now: Date) -> Double {
        pastEventBehavior == .dim && event.endDate < now ? 0.45 : 1.0
    }

    func opacityForOverdueTask(_ task: TaskMirror, now: Date, calendar: Calendar = .current) -> Double {
        guard overdueTaskBehavior == .dim else { return 1.0 }
        guard task.isCompleted == false, let due = task.dueDate else { return 1.0 }
        return due < calendar.startOfDay(for: now) ? 0.45 : 1.0
    }
}
