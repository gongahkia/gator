import Foundation

enum PreparedDueDateTone: Sendable, Equatable {
    case overdue
    case today
    case future
}

struct PreparedTaskCard: Identifiable, Equatable, Sendable {
    var id: TaskMirror.ID
    var taskListID: TaskListMirror.ID
    var title: String
    var isCompleted: Bool
    var strippedTitle: String
    var tags: [String]
    var listTitle: String
    var dueDateBadge: String?
    var dueDateTone: PreparedDueDateTone?
    var completedText: String?
    var notePreview: String
    var isDuplicate: Bool
    var accessibilityLabel: String

}

struct PreparedKanbanColumn: Identifiable, Equatable, Sendable {
    var id: String
    var title: String
    var subtitle: String?
    var openTasks: [PreparedTaskCard]
    var completedTasks: [PreparedTaskCard]
    var dropIntent: KanbanDropIntent?

    var taskCount: Int { openTasks.count + completedTasks.count }
}

struct TaskBoardDisplaySnapshot: Equatable, Sendable {
    var key: PreparedSnapshotKey
    var surface: TaskBoardSurface
    var columns: [PreparedKanbanColumn]
    var taskCount: Int
}

struct TaskBoardDisplayInput: Sendable {
    var key: PreparedSnapshotKey
    var surface: TaskBoardSurface
    var tasks: [TaskMirror]
    var columnMode: KanbanColumnMode
    var taskLists: [TaskListMirror]
    var taskListTitleByID: [TaskListMirror.ID: String]
    var duplicateTaskIDs: Set<TaskMirror.ID>
    var localOrder: [TaskMirror.ID]
    var referenceDate: Date
    var calendar: Calendar
}

enum TaskBoardDisplaySnapshotBuilder {
    static func snapshot(_ input: TaskBoardDisplayInput) -> TaskBoardDisplaySnapshot {
        let orderedTasks = orderedTasks(input.tasks, localOrder: input.localOrder)
        let columns = KanbanGrouping.columns(
            for: orderedTasks,
            mode: input.columnMode,
            taskLists: input.taskLists,
            now: input.referenceDate,
            calendar: input.calendar
        )

        let localOrderIndex = Dictionary(uniqueKeysWithValues: input.localOrder.enumerated().map { ($0.element, $0.offset) })
        let preparedColumns = columns.map { column in
            let columnTasks = orderedColumnTasks(column.tasks, localOrderIndex: localOrderIndex)
            let cards = columnTasks.map { card(for: $0, input: input) }
            return PreparedKanbanColumn(
                id: column.id,
                title: column.title,
                subtitle: subtitle(for: column, surface: input.surface),
                openTasks: cards.filter { $0.isCompleted == false },
                completedTasks: cards.filter(\.isCompleted),
                dropIntent: column.dropIntent
            )
        }
        // List-grouped boards should only show lists that contain at least one open or completed item.
        let visibleColumns = input.columnMode == .byList
            ? preparedColumns.filter { $0.taskCount > 0 }
            : preparedColumns

        return TaskBoardDisplaySnapshot(
            key: input.key,
            surface: input.surface,
            columns: visibleColumns,
            taskCount: orderedTasks.filter { $0.isDeleted == false }.count
        )
    }

    static func orderedTasks(_ tasks: [TaskMirror], localOrder: [TaskMirror.ID]) -> [TaskMirror] {
        guard localOrder.isEmpty == false else { return tasks }
        let pool = Dictionary(uniqueKeysWithValues: tasks.map { ($0.id, $0) })
        let orderedSet = Set(localOrder)
        let ordered = localOrder.compactMap { pool[$0] }
        let missing = tasks.filter { orderedSet.contains($0.id) == false }
        return ordered + missing
    }

    private static func orderedColumnTasks(
        _ tasks: [TaskMirror],
        localOrderIndex: [TaskMirror.ID: Int]
    ) -> [TaskMirror] {
        guard localOrderIndex.isEmpty == false else { return tasks }
        return tasks.sorted { lhs, rhs in
            switch (localOrderIndex[lhs.id], localOrderIndex[rhs.id]) {
            case let (l?, r?):
                return l < r
            case (_?, nil):
                return true
            case (nil, _?):
                return false
            case (nil, nil):
                return false
            }
        }
    }

    private static func card(for task: TaskMirror, input: TaskBoardDisplayInput) -> PreparedTaskCard {
        let strippedTitle = TagExtractor.stripped(from: task.title)
        let tags = TagExtractor.tags(in: task.title)
        let listTitle = input.taskListTitleByID[task.taskListID] ?? ""
        let dueBadge = task.dueDate.map { dueDateBadge($0, now: input.referenceDate, calendar: input.calendar) }
        let dueTone = task.dueDate.map { dueDateTone($0, now: input.referenceDate, calendar: input.calendar) }
        let completedText = task.completedAt.map {
            "Completed: \($0.formatted(.dateTime.weekday(.abbreviated).day().month(.abbreviated)))"
        } ?? (task.isCompleted ? "Completed" : nil)
        let notePreview = HCBTextMarkup.markdownSource(from: task.notes)
        var parts = [task.isCompleted ? "Completed task" : (input.surface == .notes ? "Note" : "Task"), strippedTitle]
        if let dueBadge {
            parts.append("due \(dueBadge)")
        }
        if listTitle.isEmpty == false {
            parts.append("list \(listTitle)")
        }
        if tags.isEmpty == false {
            parts.append("tags \(tags.joined(separator: ", "))")
        }
        if input.duplicateTaskIDs.contains(task.id) {
            parts.append("possible duplicate")
        }

        return PreparedTaskCard(
            id: task.id,
            taskListID: task.taskListID,
            title: task.title,
            isCompleted: task.isCompleted,
            strippedTitle: strippedTitle,
            tags: tags,
            listTitle: listTitle,
            dueDateBadge: dueBadge,
            dueDateTone: dueTone,
            completedText: completedText,
            notePreview: notePreview,
            isDuplicate: input.duplicateTaskIDs.contains(task.id),
            accessibilityLabel: parts.joined(separator: ", ")
        )
    }

    private static func subtitle(for column: KanbanColumn, surface: TaskBoardSurface) -> String? {
        let label = surface == .notes ? "note" : "task"
        return "\(column.tasks.count) \(label)\(column.tasks.count == 1 ? "" : "s")"
    }

    private static func dueDateBadge(_ due: Date, now: Date, calendar: Calendar) -> String {
        let startOfToday = calendar.startOfDay(for: now)
        let startOfDue = calendar.startOfDay(for: due)
        let days = calendar.dateComponents([.day], from: startOfToday, to: startOfDue).day ?? 0
        if days < 0 { return "Overdue \(-days)d" }
        if days == 0 { return "Today" }
        if days == 1 { return "Tomorrow" }
        if days < 7 { return due.formatted(.dateTime.weekday(.wide)) }
        return due.formatted(.dateTime.month(.abbreviated).day())
    }

    private static func dueDateTone(_ due: Date, now: Date, calendar: Calendar) -> PreparedDueDateTone {
        let startOfToday = calendar.startOfDay(for: now)
        let startOfDue = calendar.startOfDay(for: due)
        if startOfDue < startOfToday { return .overdue }
        if startOfDue == startOfToday { return .today }
        return .future
    }
}
