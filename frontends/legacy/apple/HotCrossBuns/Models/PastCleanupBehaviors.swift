import Foundation

// Visibility + deletion behaviors for past events, overdue tasks, and
// completed tasks. All three ladders share the showAll default so the
// feature is opt-in on every axis. Deletion-capable ladders have a
// separate threshold integer (days) persisted in AppSettings.
//
// Notes tab is out of scope because it is a content lens over tasks with
// note bodies or no due date. Cleanup still acts through the underlying
// task/event behaviors.
enum PastEventBehavior: String, CaseIterable, Hashable, Codable, Sendable {
    case showAll
    case dim
    case hide
    case delete

    var title: String {
        switch self {
        case .showAll: "Show all"
        case .dim: "Dim past events"
        case .hide: "Hide past events"
        case .delete: "Delete past events on Google"
        }
    }

    var subtitle: String {
        switch self {
        case .showAll: "Default Google Calendar behavior — past events remain fully visible."
        case .dim: "Past events render at reduced opacity in every view."
        case .hide: "Past events are excluded from every view. Still in the local cache and on Google."
        case .delete: "Hot Cross Buns issues events.delete on Google for past events older than the threshold."
        }
    }

    var isDeletion: Bool { self == .delete }
}

enum OverdueTaskBehavior: String, CaseIterable, Hashable, Codable, Sendable {
    case showAll
    case dim
    case hide

    var title: String {
        switch self {
        case .showAll: "Show all"
        case .dim: "Dim overdue"
        case .hide: "Hide overdue"
        }
    }

    var subtitle: String {
        switch self {
        case .showAll: "Overdue tasks (due date passed, still open) stay fully visible — the reminder surface."
        case .dim: "Overdue open tasks render at reduced opacity."
        case .hide: "Overdue open tasks are hidden. No deletion — auto-deleting reminders is too risky."
        }
    }
}

enum CompletedTaskBehavior: String, CaseIterable, Hashable, Codable, Sendable {
    case showAll
    case hide
    case delete

    var title: String {
        switch self {
        case .showAll: "Show all"
        case .hide: "Hide completed"
        case .delete: "Delete completed on Google"
        }
    }

    var subtitle: String {
        switch self {
        case .showAll: "Completed tasks stay in every list."
        case .hide: "Completed tasks are hidden. Still on Google."
        case .delete: "Hot Cross Buns issues tasks.delete on Google for completed tasks whose completion is older than the threshold."
        }
    }

    var isDeletion: Bool { self == .delete }
}
