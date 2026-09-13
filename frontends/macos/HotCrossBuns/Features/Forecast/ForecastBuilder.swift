import Foundation

enum ForecastEntry: Identifiable, Equatable, Sendable {
    case task(TaskMirror)
    case event(CalendarEventMirror)

    var id: String {
        switch self {
        case .task(let t): return "task-\(t.id)"
        case .event(let e): return "event-\(e.id)"
        }
    }

    var sortKey: Date {
        switch self {
        case .task: return .distantFuture
        case .event(let e): return e.startDate
        }
    }
}

struct ForecastDay: Identifiable, Equatable, Sendable {
    let id: Date
    let date: Date
    let tasks: [TaskMirror]
    let events: [CalendarEventMirror]

    var isEmpty: Bool { tasks.isEmpty && events.isEmpty }

    var entries: [ForecastEntry] {
        let eventEntries = events.map(ForecastEntry.event)
        let taskEntries = tasks.map(ForecastEntry.task)
        return eventEntries.sorted { $0.sortKey < $1.sortKey } + taskEntries
    }
}

struct Forecast: Equatable, Sendable {
    var overdueTasks: [TaskMirror]
    var days: [ForecastDay]

    var hasContent: Bool { overdueTasks.isEmpty == false || days.contains(where: { !$0.isEmpty }) }
}

enum ForecastBuilder {
    static let horizonDays = 14

    static func build(
        tasks: [TaskMirror],
        events: [CalendarEventMirror],
        selectedTaskListIDs: Set<TaskListMirror.ID>,
        selectedCalendarIDs: Set<CalendarListMirror.ID>,
        referenceDate: Date = Date(),
        calendar: Calendar = .current,
        horizon: Int = horizonDays
    ) -> Forecast {
        let filteredTasks = tasks.filter { task in
            task.isDeleted == false
                && task.isCompleted == false
                && selectedTaskListIDs.contains(task.taskListID)
        }

        let filteredEvents = events.filter { event in
            event.status != .cancelled && selectedCalendarIDs.contains(event.calendarID)
        }

        let today = calendar.startOfDay(for: referenceDate)

        let overdue = filteredTasks
            .filter { task in
                guard let due = task.dueDate else { return false }
                return calendar.startOfDay(for: due) < today
            }
            .sorted { (lhs: TaskMirror, rhs: TaskMirror) in
                let l = lhs.dueDate ?? referenceDate
                let r = rhs.dueDate ?? referenceDate
                return l < r
            }

        var days: [ForecastDay] = []
        for offset in 0..<horizon {
            guard let dayStart = calendar.date(byAdding: .day, value: offset, to: today) else { continue }
            let dayEnd = calendar.date(byAdding: .day, value: 1, to: dayStart) ?? dayStart

            let tasksForDay = filteredTasks
                .filter { task in
                    guard let due = task.dueDate else { return false }
                    return calendar.startOfDay(for: due) == dayStart
                }
                .sorted { $0.title.localizedCaseInsensitiveCompare($1.title) == .orderedAscending }

            let eventsForDay = filteredEvents
                .filter { event in
                    event.startDate < dayEnd && event.endDate > dayStart
                }
                .sorted { lhs, rhs in
                    if lhs.isAllDay != rhs.isAllDay { return lhs.isAllDay && rhs.isAllDay == false }
                    return lhs.startDate < rhs.startDate
                }

            days.append(ForecastDay(id: dayStart, date: dayStart, tasks: tasksForDay, events: eventsForDay))
        }

        return Forecast(overdueTasks: overdue, days: days)
    }
}
