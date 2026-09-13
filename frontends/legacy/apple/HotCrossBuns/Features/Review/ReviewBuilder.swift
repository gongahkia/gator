import Foundation

struct ReviewListSummary: Identifiable, Equatable, Sendable {
    let taskList: TaskListMirror
    let openTasks: [TaskMirror]
    let lastActivity: Date?
    let daysSinceActivity: Int?

    var id: TaskListMirror.ID { taskList.id }
}

enum ReviewBuilder {
    static let staleAfterDays = 7

    static func build(
        taskLists: [TaskListMirror],
        tasks: [TaskMirror],
        visibleTaskListIDs: Set<TaskListMirror.ID>,
        referenceDate: Date = Date(),
        calendar: Calendar = .current
    ) -> [ReviewListSummary] {
        let today = calendar.startOfDay(for: referenceDate)
        let filteredLists = taskLists.filter { visibleTaskListIDs.contains($0.id) }

        return filteredLists.map { list in
            let listTasks = tasks.filter { $0.taskListID == list.id && $0.isDeleted == false }
            let openTasks = listTasks
                .filter { $0.isCompleted == false }
                .sorted { lhs, rhs in
                    switch (lhs.dueDate, rhs.dueDate) {
                    case let (l?, r?): return l < r
                    case (nil, _?): return false
                    case (_?, nil): return true
                    case (nil, nil): return lhs.title.localizedCaseInsensitiveCompare(rhs.title) == .orderedAscending
                    }
                }
            let lastActivity = (listTasks.compactMap(\.updatedAt) + [list.updatedAt].compactMap { $0 }).max()
            let daysSince = lastActivity.map { date in
                calendar.dateComponents([.day], from: calendar.startOfDay(for: date), to: today).day ?? 0
            }
            return ReviewListSummary(
                taskList: list,
                openTasks: openTasks,
                lastActivity: lastActivity,
                daysSinceActivity: daysSince
            )
        }
        .sorted { lhs, rhs in
            (lhs.daysSinceActivity ?? .max) > (rhs.daysSinceActivity ?? .max)
        }
    }
}
