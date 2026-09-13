import Foundation

// A reversible mutation captured at the moment it fired. AppModel records
// one on every undoable action; UndoToast shows the most recent for ~6s
// with a button that asks AppModel to apply the inverse.
enum UndoableAction: Sendable, Equatable {
    // Completion toggle — undo flips the flag back.
    case taskCompletion(taskID: TaskMirror.ID, priorCompleted: Bool, title: String)
    // Delete on a persisted task — undo recreates it (new server ID, same
    // content). There is no Google Tasks "undelete" API, so a recreate is
    // the best we can do; position/subtask parent is preserved where possible.
    case taskDelete(snapshot: TaskMirror)
    // Edit that changed title / notes / due date — undo writes the snapshot
    // back through updateTask.
    case taskEdit(priorSnapshot: TaskMirror)
    // Delete on a persisted event — undo recreates (no "undelete" API).
    case eventDelete(snapshot: CalendarEventMirror)
    // Edit that changed summary / details / time / etc. — undo writes the
    // snapshot back through updateEvent.
    case eventEdit(priorSnapshot: CalendarEventMirror)
    // Net-new task creation — undo deletes the created task.
    case taskCreate(snapshot: TaskMirror)
    // Duplicate — creates a second task from an existing one. Undo deletes
    // the new copy. sourceTitle retained for history summary readability.
    case taskDuplicate(newSnapshot: TaskMirror, sourceTitle: String)
    // Moving a task between lists (Google rebuilds as a new task in target;
    // old ID is gone). Undo is a second move back. fromList/toList titles
    // captured for history readability when the list may later be renamed.
    case taskMove(taskID: TaskMirror.ID, fromListID: TaskListMirror.ID, toListID: TaskListMirror.ID, title: String, fromListTitle: String, toListTitle: String)
    // Net-new event creation — undo deletes.
    case eventCreate(snapshot: CalendarEventMirror)
    // User marked an event "done" via the checkmark affordance on a calendar
    // tile. Google Calendar has no completion concept, so this is a delete
    // under the hood — but the history log treats it as a completion so the
    // user can distinguish "I finished this meeting" from "I deleted this
    // by accident". Undo recreates via createEvent exactly like eventDelete.
    case eventDismissed(snapshot: CalendarEventMirror)
    // Copy / paste / cut clipboard operations. kind = "copy" | "paste" | "cut".
    // Not reversible server-side; recorded purely for history.
    case clipboardOp(kind: String, resourceID: String, title: String)
    // Restore after undo or after a remote re-sync brought a deleted item back.
    case taskRestore(snapshot: TaskMirror)
    case eventRestore(snapshot: CalendarEventMirror)
    // Bulk mutation (multi-select delete/move/complete). One entry per user
    // action; firstTitle shown + "(+N more)" in history.
    case bulkAction(kind: String, count: Int, firstTitle: String)
    // Remote sync pulled a real diff from Google (delta > 0). Off by default
    // in history filter because chatty otherwise, but still persisted.
    case syncPulled(kind: String, count: Int)

    var summary: String {
        switch self {
        case .taskCompletion(_, let prior, let title):
            return prior ? "Reopened \"\(title)\"" : "Completed \"\(title)\""
        case .taskDelete(let snap):
            return "Deleted \"\(snap.title)\""
        case .taskEdit(let snap):
            return "Edited \"\(snap.title)\""
        case .eventDelete(let snap):
            return "Deleted event \"\(snap.summary)\""
        case .eventEdit(let snap):
            return "Edited event \"\(snap.summary)\""
        case .taskCreate(let snap):
            return "Created \"\(snap.title)\""
        case .taskDuplicate(let snap, _):
            return "Duplicated \"\(snap.title)\""
        case .taskMove(_, _, _, let title, let fromTitle, let toTitle):
            return "Moved \"\(title)\" from \(fromTitle) to \(toTitle)"
        case .eventCreate(let snap):
            return "Created event \"\(snap.summary)\""
        case .eventDismissed(let snap):
            return "Marked \"\(snap.summary)\" as done"
        case .clipboardOp(let kind, _, let title):
            let verb = kind.capitalized
            return "\(verb) \"\(title)\""
        case .taskRestore(let snap):
            return "Restored \"\(snap.title)\""
        case .eventRestore(let snap):
            return "Restored event \"\(snap.summary)\""
        case .bulkAction(let kind, let count, let firstTitle):
            return count > 1 ? "\(kind.capitalized) \"\(firstTitle)\" (+\(count - 1) more)" : "\(kind.capitalized) \"\(firstTitle)\""
        case .syncPulled(let kind, let count):
            return "Sync pulled \(count) \(kind) change\(count == 1 ? "" : "s")"
        }
    }

    var sfSymbol: String {
        switch self {
        case .taskCompletion(_, let prior, _): prior ? "arrow.uturn.backward.circle" : "checkmark.circle.fill"
        case .taskDelete, .eventDelete: "trash"
        case .taskEdit, .eventEdit: "square.and.pencil"
        case .taskCreate, .eventCreate: "plus.circle"
        case .eventDismissed: "checkmark.circle"
        case .taskDuplicate: "plus.square.on.square"
        case .taskMove: "arrow.right.square"
        case .clipboardOp: "doc.on.clipboard"
        case .taskRestore, .eventRestore: "arrow.uturn.backward.circle"
        case .bulkAction: "square.grid.2x2"
        case .syncPulled: "arrow.triangle.2.circlepath"
        }
    }
}
