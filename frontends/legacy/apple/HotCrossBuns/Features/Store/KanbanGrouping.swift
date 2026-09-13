import Foundation

// Pure grouping logic for the Kanban board. Testable without SwiftUI.
//
// Column-mode picks a dimension to split tasks by. Every mode is either a
// native Google Tasks attribute (list) or derived from already-present
// state (due date, tag tokens in title). No hidden local-only fields — the
// kanban is a lens over the mirror, never a separate data store.

enum KanbanColumnMode: String, CaseIterable, Hashable, Codable, Sendable {
    case byList
    case byDueBucket
    case byTag

    var title: String {
        switch self {
        case .byList: "List"
        case .byDueBucket: "Due date"
        case .byTag: "Tag"
        }
    }

    var systemImage: String {
        switch self {
        case .byList: "checklist"
        case .byDueBucket: "calendar.badge.clock"
        case .byTag: "number"
        }
    }
}

// A single kanban column. `dropIntent` describes what BulkTaskOperation to
// fire when a card is dropped onto this column. nil ⇒ not a drop target
// (derived columns where dropping has no clear semantics, e.g. "Untagged"
// under byTag).
struct KanbanColumn: Identifiable, Equatable, Sendable {
    let id: String
    let title: String
    let subtitle: String?
    let tasks: [TaskMirror]
    let dropIntent: KanbanDropIntent?
}

enum KanbanDropIntent: Equatable, Sendable {
    case moveToList(listId: String)
    case setDue(date: Date?)
    case setTag(add: String?, remove: String?)

    // Converts the drop intent + a dragged task into a concrete operation
    // for BulkTaskOptimizer / AppModel.performBulkTaskOperations.
    func operation(for taskId: String) -> BulkTaskOperation? {
        switch self {
        case .moveToList(let listId):
            return .moveToList(taskId: taskId, targetListId: listId)
        case .setDue(let date):
            return .setDue(taskId: taskId, dueDate: date)
        case .setTag(let add, let remove):
            if let add { return .addTag(taskId: taskId, tag: add) }
            if let remove { return .removeTag(taskId: taskId, tag: remove) }
            return nil
        }
    }
}

enum KanbanGrouping {
    // Returns deterministically-ordered columns for the given tasks. Empty
    // columns are still included when they carry a clear drop target (e.g.
    // an empty list column in byList is a valid drop destination). Columns
    // without a native destination (derived buckets that already have every
    // task) are still shown — a board with no empty columns hides structure.
    static func columns(
        for tasks: [TaskMirror],
        mode: KanbanColumnMode,
        taskLists: [TaskListMirror],
        now: Date = Date(),
        calendar: Calendar = .current
    ) -> [KanbanColumn] {
        let visible = tasks.filter { $0.isDeleted == false }
        switch mode {
        case .byList:
            return byList(tasks: visible, taskLists: taskLists)
        case .byDueBucket:
            return byDueBucket(tasks: visible, now: now, calendar: calendar)
        case .byTag:
            return byTag(tasks: visible)
        }
    }

    // MARK: - by list

    private static func byList(tasks: [TaskMirror], taskLists: [TaskListMirror]) -> [KanbanColumn] {
        var bucket: [String: [TaskMirror]] = [:]
        for t in tasks { bucket[t.taskListID, default: []].append(t) }
        return taskLists.map { list in
            let items = bucket[list.id] ?? []
            return KanbanColumn(
                id: "list-\(list.id)",
                title: list.title,
                subtitle: "\(items.count) task\(items.count == 1 ? "" : "s")",
                tasks: items.sorted(by: sortKey),
                dropIntent: .moveToList(listId: list.id)
            )
        }
    }

    // MARK: - by due bucket

    fileprivate enum DueBucket: CaseIterable {
        case overdue, today, thisWeek, later, noDate
    }

    private static func byDueBucket(tasks: [TaskMirror], now: Date, calendar: Calendar) -> [KanbanColumn] {
        let startOfToday = calendar.startOfDay(for: now)
        let startOfTomorrow = calendar.date(byAdding: .day, value: 1, to: startOfToday) ?? startOfToday
        let startOfWeekEnd = calendar.date(byAdding: .day, value: 7, to: startOfToday) ?? startOfToday

        func bucket(for task: TaskMirror) -> DueBucket {
            guard let due = task.dueDate else { return .noDate }
            let day = calendar.startOfDay(for: due)
            if day < startOfToday { return .overdue }
            if day < startOfTomorrow { return .today }
            if day < startOfWeekEnd { return .thisWeek }
            return .later
        }

        var bucketed: [DueBucket: [TaskMirror]] = [:]
        for t in tasks { bucketed[bucket(for: t), default: []].append(t) }

        return DueBucket.allCases.map { b in
            let items = (bucketed[b] ?? []).sorted(by: sortKey)
            let (title, dropDate): (String, Date?) = {
                switch b {
                case .overdue:
                    // Drop → set due to yesterday so the task *stays* overdue.
                    let yesterday = calendar.date(byAdding: .day, value: -1, to: startOfToday) ?? startOfToday
                    return ("Overdue", yesterday)
                case .today: return ("Today", startOfToday)
                case .thisWeek:
                    // Drop → set due to +3 days (mid-week), a reasonable "this week" pick.
                    let midWeek = calendar.date(byAdding: .day, value: 3, to: startOfToday) ?? startOfToday
                    return ("This week", midWeek)
                case .later:
                    let later = calendar.date(byAdding: .day, value: 14, to: startOfToday) ?? startOfToday
                    return ("Later", later)
                case .noDate: return ("No date", nil)
                }
            }()
            // "No date" drops clear the due date (setDue nil).
            let intent: KanbanDropIntent = .setDue(date: dropDate)
            return KanbanColumn(
                id: "bucket-\(b)",
                title: title,
                subtitle: "\(items.count) task\(items.count == 1 ? "" : "s")",
                tasks: items,
                dropIntent: intent
            )
        }
    }

    // MARK: - by tag

    // Groups tasks by their #tag tokens. A task appears in every column
    // whose tag it carries. Tasks without tags land in an "Untagged" column
    // which has no drop semantics (dropping there is ambiguous: which tag
    // would we remove?).
    private static func byTag(tasks: [TaskMirror]) -> [KanbanColumn] {
        var byTag: [String: [TaskMirror]] = [:]
        var untagged: [TaskMirror] = []

        for t in tasks {
            let tags = TagExtractor.tags(in: t.title)
            if tags.isEmpty {
                untagged.append(t)
            } else {
                for tag in tags {
                    byTag[tag.lowercased(), default: []].append(t)
                }
            }
        }

        let tagColumns = byTag.keys.sorted().map { key -> KanbanColumn in
            let items = byTag[key] ?? []
            return KanbanColumn(
                id: "tag-\(key)",
                title: "#\(key)",
                subtitle: "\(items.count) task\(items.count == 1 ? "" : "s")",
                tasks: items.sorted(by: sortKey),
                dropIntent: .setTag(add: key, remove: nil)
            )
        }

        let untaggedColumn = KanbanColumn(
            id: "tag-untagged",
            title: "Untagged",
            subtitle: "\(untagged.count) task\(untagged.count == 1 ? "" : "s")",
            tasks: untagged.sorted(by: sortKey),
            dropIntent: nil
        )

        return tagColumns + [untaggedColumn]
    }

    // MARK: - sort helper

    // Sort ordering inside a column: uncompleted first, then by due date
    // (ascending, nil last), then by title. Same sort used across smart
    // lists for consistency.
    private static func sortKey(_ a: TaskMirror, _ b: TaskMirror) -> Bool {
        if a.isCompleted != b.isCompleted { return a.isCompleted == false }
        switch (a.dueDate, b.dueDate) {
        case let (ad?, bd?):
            if ad != bd { return ad < bd }
        case (_?, nil): return true
        case (nil, _?): return false
        case (nil, nil): break
        }
        return a.title.localizedCaseInsensitiveCompare(b.title) == .orderedAscending
    }
}
