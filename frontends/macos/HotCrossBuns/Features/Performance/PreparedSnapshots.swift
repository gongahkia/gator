import Foundation

struct PreparedSnapshotKey: Hashable, Sendable, CustomStringConvertible {
    var rawValue: String

    init(_ parts: [String]) {
        rawValue = parts.joined(separator: "|")
    }

    init(_ rawValue: String) {
        self.rawValue = rawValue
    }

    var description: String { rawValue }
}

enum PreparedSnapshotKeys {
    static func calendar(
        mode: CalendarGridMode,
        dataRevision: UInt64,
        selectedCalendarIDs: Set<CalendarListMirror.ID>,
        visibleTaskListIDs: Set<TaskListMirror.ID>,
        filterKey: String,
        searchQuery: String,
        rangeKey: String,
        settings: AppSettings
    ) -> PreparedSnapshotKey {
        PreparedSnapshotKey([
            "surface=calendar",
            "mode=\(mode.rawValue)",
            "rev=\(dataRevision)",
            "cal=\(selectedCalendarIDs.sorted().joined(separator: ","))",
            "tasks=\(visibleTaskListIDs.sorted().joined(separator: ","))",
            "filter=\(filterKey)",
            "search=\(normalizedSearch(searchQuery))",
            "range=\(rangeKey)",
            "past=\(settings.pastEventBehavior.rawValue)",
            "showDone=\(settings.showCompletedItemsInCalendar)",
            "overdue=\(settings.overdueTaskBehavior.rawValue)"
        ])
    }

    static func calendar(
        mode: CalendarGridMode,
        dataRevision: String,
        selectedCalendarIDs: Set<CalendarListMirror.ID>,
        visibleTaskListIDs: Set<TaskListMirror.ID>,
        filterKey: String,
        searchQuery: String,
        rangeKey: String,
        settings: AppSettings
    ) -> PreparedSnapshotKey {
        PreparedSnapshotKey([
            "surface=calendar",
            "mode=\(mode.rawValue)",
            "rev=\(dataRevision)",
            "cal=\(selectedCalendarIDs.sorted().joined(separator: ","))",
            "tasks=\(visibleTaskListIDs.sorted().joined(separator: ","))",
            "filter=\(filterKey)",
            "search=\(normalizedSearch(searchQuery))",
            "range=\(rangeKey)",
            "past=\(settings.pastEventBehavior.rawValue)",
            "showDone=\(settings.showCompletedItemsInCalendar)",
            "overdue=\(settings.overdueTaskBehavior.rawValue)"
        ])
    }

    static func taskBoard(
        surface: TaskBoardSurface,
        dataRevision: UInt64,
        groupMode: KanbanColumnMode,
        visibleListIDs: Set<TaskListMirror.ID>,
        localOrder: [TaskMirror.ID] = []
    ) -> PreparedSnapshotKey {
        PreparedSnapshotKey([
            "surface=\(surface.rawValue)",
            "rev=\(dataRevision)",
            "group=\(groupMode.rawValue)",
            "lists=\(visibleListIDs.sorted().joined(separator: ","))",
            "order=\(localOrder.joined(separator: ","))"
        ])
    }

    static func dateRangeKey(_ dates: [Date]) -> String {
        guard let first = dates.first, let last = dates.last else { return "empty" }
        return "\(Int(first.timeIntervalSinceReferenceDate))-\(Int(last.timeIntervalSinceReferenceDate))-\(dates.count)"
    }

    static func dateKey(_ date: Date, calendar: Calendar = .current) -> String {
        "\(Int(calendar.startOfDay(for: date).timeIntervalSinceReferenceDate))"
    }

    static func yearKey(_ date: Date, calendar: Calendar = .current) -> String {
        "\(calendar.component(.year, from: date))"
    }

    private static func normalizedSearch(_ query: String) -> String {
        query.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    }
}

enum TaskBoardSurface: String, Sendable {
    case tasks
    case notes
}
