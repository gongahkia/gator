import Foundation

struct TaskListMirror: Identifiable, Hashable, Codable, Sendable {
    let id: String
    var title: String
    var updatedAt: Date?
    var etag: String?
}

struct TaskMirror: Identifiable, Hashable, Codable, Sendable {
    let id: String
    var taskListID: TaskListMirror.ID
    var parentID: String?
    var title: String
    var notes: String
    var status: TaskStatus
    var dueDate: Date?
    var completedAt: Date?
    var isDeleted: Bool
    var isHidden: Bool
    var position: String?
    var etag: String?
    var updatedAt: Date?

    var isCompleted: Bool {
        status == .completed
    }

    var hasNoteBody: Bool {
        notes.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false
    }

    var appearsInNotesSurface: Bool {
        dueDate == nil || hasNoteBody
    }
}

enum TaskStatus: String, Codable, Hashable, Sendable {
    case needsAction
    case completed
}
