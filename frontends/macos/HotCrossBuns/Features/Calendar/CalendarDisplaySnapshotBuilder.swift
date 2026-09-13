import Foundation

enum CalendarVisibleRangeKind: String, Hashable, Sendable {
    case day
    case week
    case month
    case year
    case agenda
}

struct CalendarVisibleRange: Equatable, Sendable {
    var kind: CalendarVisibleRangeKind
    var start: Date
    var end: Date
    var dayKeys: [TimeInterval]

    init(
        kind: CalendarVisibleRangeKind,
        start: Date,
        end: Date,
        calendar: Calendar
    ) {
        let normalizedStart = calendar.startOfDay(for: start)
        let normalizedEnd = calendar.startOfDay(for: end)
        self.kind = kind
        self.start = normalizedStart
        self.end = normalizedEnd

        var keys: [TimeInterval] = []
        var cursor = normalizedStart
        while cursor <= normalizedEnd {
            keys.append(cursor.timeIntervalSinceReferenceDate)
            guard let next = calendar.date(byAdding: .day, value: 1, to: cursor) else { break }
            cursor = next
        }
        dayKeys = keys
    }
}

struct CalendarVisibleRangeProjection: Equatable, Sendable {
    var range: CalendarVisibleRange
    var revisionKey: String
    var eventsByDay: [TimeInterval: [CalendarEventMirror.ID]]
    var eventCountsByDay: [TimeInterval: Int] = [:]
    var tasksByDueDate: [TimeInterval: [TaskMirror.ID]]
    var eventByID: [CalendarEventMirror.ID: CalendarEventMirror]
    var taskByID: [TaskMirror.ID: TaskMirror]
    var eventSearchTextByID: [CalendarEventMirror.ID: String] = [:]
    var taskSearchTextByID: [TaskMirror.ID: String] = [:]

    static func empty(kind: CalendarVisibleRangeKind, anchorDate: Date, calendar: Calendar) -> CalendarVisibleRangeProjection {
        let day = calendar.startOfDay(for: anchorDate)
        return CalendarVisibleRangeProjection(
            range: CalendarVisibleRange(kind: kind, start: day, end: day, calendar: calendar),
            revisionKey: "0",
            eventsByDay: [:],
            tasksByDueDate: [:],
            eventByID: [:],
            taskByID: [:],
            eventSearchTextByID: [:],
            taskSearchTextByID: [:]
        )
    }
}

struct CalendarDisplayInput: Sendable {
    var key: PreparedSnapshotKey
    var anchorDate: Date
    var selectedCalendarIDs: Set<CalendarListMirror.ID>
    var eventViewFilter: CalendarEventViewFilter
    var visibleTaskListIDs: Set<TaskListMirror.ID>
    var searchQuery: String
    var visibleRange: CalendarVisibleRangeProjection
    var calendarColorHexByID: [CalendarListMirror.ID: String]
    var taskListTitleByID: [TaskListMirror.ID: String]
    var settings: AppSettings
    var referenceDate: Date
    var calendar: Calendar
}

struct CalendarEventDisplayMetadata: Equatable, Sendable {
    var colorHex: String?
    var opacity: Double
    var timeRangeLabel: String
    var accessibilityLabel: String
}

struct CalendarTaskDisplayMetadata: Equatable, Sendable {
    var title: String
    var strippedTitle: String
    var listTitle: String
    var opacity: Double
    var accessibilityLabel: String
}

enum CalendarDensityRendering {
    static let denseDayEventThreshold = 120
    static let denseMonthItemThreshold = 80
    static let dayTimedEventLimit = 72
    static let weekTimedEventLimit = 48
    static let allDayEventLimit = 24
}

struct CalendarDayDensitySummary: Equatable, Sendable {
    var allDayEventCount: Int
    var timedEventCount: Int
    var taskCount: Int
    var visibleAllDayEventLimit: Int
    var visibleTimedEventLimit: Int

    var totalItemCount: Int {
        allDayEventCount + timedEventCount + taskCount
    }

    var visibleAllDayEventCount: Int {
        min(allDayEventCount, visibleAllDayEventLimit)
    }

    var visibleTimedEventCount: Int {
        min(timedEventCount, visibleTimedEventLimit)
    }

    var hiddenAllDayEventCount: Int {
        max(0, allDayEventCount - visibleAllDayEventCount)
    }

    var hiddenTimedEventCount: Int {
        max(0, timedEventCount - visibleTimedEventCount)
    }

    var usesDenseTimedRendering: Bool {
        timedEventCount >= CalendarDensityRendering.denseDayEventThreshold
    }

    var usesDenseAllDayRendering: Bool {
        allDayEventCount > visibleAllDayEventLimit
    }

    var usesDenseMonthRendering: Bool {
        totalItemCount >= CalendarDensityRendering.denseMonthItemThreshold
    }
}

struct CalendarDayDisplaySnapshot: Equatable, Sendable {
    var key: PreparedSnapshotKey
    var dayStart: Date
    var dayEnd: Date
    var allDayEvents: [CalendarEventMirror]
    var timedEvents: [CalendarEventMirror]
    var laidOutTimedEvents: [CalendarGridLayout.LaidOutEvent]
    var density: CalendarDayDensitySummary
    var eventMetadataByID: [CalendarEventMirror.ID: CalendarEventDisplayMetadata]
}

struct CalendarWeekDisplaySnapshot: Equatable, Sendable {
    struct DayLabel: Identifiable, Equatable, Sendable {
        var day: Date
        var weekday: String
        var dayNumber: String
        var isToday: Bool

        var id: Date { day }
    }

    struct AllDaySpan: Identifiable, Equatable, Sendable {
        var event: CalendarEventMirror
        var startColumn: Int
        var endColumn: Int
        var laneIndex: Int

        var id: String { event.id }
        var columnCount: Int { endColumn - startColumn + 1 }
    }

    var key: PreparedSnapshotKey
    var days: [Date]
    var dayLabels: [DayLabel]
    var timedEventsByDay: [TimeInterval: [CalendarEventMirror]]
    var laidOutTimedEventsByDay: [TimeInterval: [CalendarGridLayout.LaidOutEvent]]
    var allDaySpans: [AllDaySpan]
    var allDayEventsByDay: [TimeInterval: [CalendarEventMirror]]
    var tasksByDay: [TimeInterval: [TaskMirror]]
    var densityByDay: [TimeInterval: CalendarDayDensitySummary]
    var eventMetadataByID: [CalendarEventMirror.ID: CalendarEventDisplayMetadata]
    var taskMetadataByID: [TaskMirror.ID: CalendarTaskDisplayMetadata]
}

struct CalendarAgendaDisplaySnapshot: Equatable, Sendable {
    struct Day: Identifiable, Equatable, Sendable {
        var date: Date
        var events: [CalendarEventMirror]
        var tasks: [TaskMirror]

        var id: Date { date }
        var isEmpty: Bool { events.isEmpty && tasks.isEmpty }
    }

    var key: PreparedSnapshotKey
    var days: [Day]
    var eventMetadataByID: [CalendarEventMirror.ID: CalendarEventDisplayMetadata]
    var taskMetadataByID: [TaskMirror.ID: CalendarTaskDisplayMetadata]

    var isEmpty: Bool { days.allSatisfy(\.isEmpty) }
}

struct CalendarYearDisplaySnapshot: Equatable, Sendable {
    struct Month: Identifiable, Equatable, Sendable {
        var monthStart: Date
        var monthNumber: Int
        var monthName: String
        var cells: [Date]

        var id: Date { monthStart }
    }

    var key: PreparedSnapshotKey
    var year: Int
    var months: [Month]
    var countsByDay: [TimeInterval: Int]
    var maxCount: Int
}

enum CalendarDisplaySnapshotBuilder {
    static func daySnapshot(_ input: CalendarDisplayInput) -> CalendarDayDisplaySnapshot {
        let dayStart = input.calendar.startOfDay(for: input.anchorDate)
        let dayEnd = input.calendar.date(byAdding: .day, value: 1, to: dayStart) ?? dayStart
        let visible = visibleEvents(input, from: dayStart, to: dayStart)
        let allDayEvents = visible
            .filter(\.isAllDay)
            .sorted { $0.summary.localizedCaseInsensitiveCompare($1.summary) == .orderedAscending }
        let timedEvents = visible
            .filter { $0.isAllDay == false }
            .sorted { $0.startDate < $1.startDate }
        let density = CalendarDayDensitySummary(
            allDayEventCount: allDayEvents.count,
            timedEventCount: timedEvents.count,
            taskCount: 0,
            visibleAllDayEventLimit: CalendarDensityRendering.allDayEventLimit,
            visibleTimedEventLimit: CalendarDensityRendering.dayTimedEventLimit
        )
        let timedEventsForLayout = density.usesDenseTimedRendering
            ? Array(timedEvents.prefix(density.visibleTimedEventLimit))
            : timedEvents
        let metadata = metadataByEventID(for: visible, input: input)

        return CalendarDayDisplaySnapshot(
            key: input.key,
            dayStart: dayStart,
            dayEnd: dayEnd,
            allDayEvents: allDayEvents,
            timedEvents: timedEvents,
            laidOutTimedEvents: CalendarGridLayout.layout(eventsInDay: timedEventsForLayout, calendar: input.calendar),
            density: density,
            eventMetadataByID: metadata
        )
    }

    static func weekSnapshot(_ input: CalendarDisplayInput, multiDayCount: Int? = nil) -> CalendarWeekDisplaySnapshot {
        let days: [Date]
        if let multiDayCount, multiDayCount > 0 {
            let start = input.calendar.startOfDay(for: input.anchorDate)
            days = (0..<multiDayCount).compactMap { input.calendar.date(byAdding: .day, value: $0, to: start) }
        } else {
            days = CalendarGridLayout.weekDays(containing: input.anchorDate, calendar: input.calendar)
        }

        guard let first = days.first, let last = days.last else {
            return CalendarWeekDisplaySnapshot(
                key: input.key,
                days: [],
                dayLabels: [],
                timedEventsByDay: [:],
                laidOutTimedEventsByDay: [:],
                allDaySpans: [],
                allDayEventsByDay: [:],
                tasksByDay: [:],
                densityByDay: [:],
                eventMetadataByID: [:],
                taskMetadataByID: [:]
            )
        }

        let visible = visibleEvents(input, from: first, to: last)
        let allDaySpans = layoutAllDaySpans(visible, days: days, calendar: input.calendar)
        var timedEventsByDay: [TimeInterval: [CalendarEventMirror]] = [:]
        var allDayEventsByDay: [TimeInterval: [CalendarEventMirror]] = [:]
        var densityByDay: [TimeInterval: CalendarDayDensitySummary] = [:]
        let tasksByDay = tasksByDay(input, days: days)

        for day in days {
            let dayStart = input.calendar.startOfDay(for: day)
            let dayEnd = input.calendar.date(byAdding: .day, value: 1, to: dayStart) ?? dayStart
            let key = dayKey(dayStart, calendar: input.calendar)
            let dayEvents = visible.filter { event in
                event.startDate < dayEnd && event.endDate > dayStart
            }
            timedEventsByDay[key] = dayEvents
                .filter { $0.isAllDay == false }
                .sorted { $0.startDate < $1.startDate }
            allDayEventsByDay[key] = dayEvents
                .filter(\.isAllDay)
                .sorted { lhs, rhs in
                    if lhs.startDate != rhs.startDate { return lhs.startDate < rhs.startDate }
                    return lhs.summary.localizedCaseInsensitiveCompare(rhs.summary) == .orderedAscending
                }
            densityByDay[key] = CalendarDayDensitySummary(
                allDayEventCount: allDayEventsByDay[key]?.count ?? 0,
                timedEventCount: timedEventsByDay[key]?.count ?? 0,
                taskCount: tasksByDay[key]?.count ?? 0,
                visibleAllDayEventLimit: CalendarDensityRendering.allDayEventLimit,
                visibleTimedEventLimit: CalendarDensityRendering.weekTimedEventLimit
            )
        }

        let laidOutTimedEventsByDay = Dictionary(uniqueKeysWithValues: timedEventsByDay.map { key, dayEvents in
            let density = densityByDay[key]
            let layoutEvents = density?.usesDenseTimedRendering == true
                ? Array(dayEvents.prefix(density?.visibleTimedEventLimit ?? CalendarDensityRendering.weekTimedEventLimit))
                : dayEvents
            return (key, CalendarGridLayout.layout(eventsInDay: layoutEvents, calendar: input.calendar))
        })
        let dayLabels = days.map { day in
            CalendarWeekDisplaySnapshot.DayLabel(
                day: day,
                weekday: day.formatted(.dateTime.weekday(.abbreviated)),
                dayNumber: day.formatted(.dateTime.day()),
                isToday: input.calendar.isDateInToday(day)
            )
        }

        return CalendarWeekDisplaySnapshot(
            key: input.key,
            days: days,
            dayLabels: dayLabels,
            timedEventsByDay: timedEventsByDay,
            laidOutTimedEventsByDay: laidOutTimedEventsByDay,
            allDaySpans: allDaySpans,
            allDayEventsByDay: allDayEventsByDay,
            tasksByDay: tasksByDay,
            densityByDay: densityByDay,
            eventMetadataByID: metadataByEventID(for: visible, input: input),
            taskMetadataByID: metadataByTaskID(for: Array(tasksByDay.values.joined()), input: input)
        )
    }

    static func agendaSnapshot(_ input: CalendarDisplayInput, dayCount: Int = 14) -> CalendarAgendaDisplaySnapshot {
        let start = input.calendar.startOfDay(for: input.anchorDate)
        let days = (0..<dayCount).compactMap { input.calendar.date(byAdding: .day, value: $0, to: start) }
        guard let last = days.last else {
            return CalendarAgendaDisplaySnapshot(key: input.key, days: [], eventMetadataByID: [:], taskMetadataByID: [:])
        }

        let visible = visibleEvents(input, from: start, to: last)
        var eventsByDay: [TimeInterval: [CalendarEventMirror]] = [:]
        for event in visible {
            let eventStart = max(input.calendar.startOfDay(for: event.startDate), start)
            let eventEnd = min(CalendarGridLayout.eventEndDay(event: event, calendar: input.calendar), last)
            guard eventStart <= eventEnd else { continue }
            var cursor = eventStart
            while cursor <= eventEnd {
                let key = dayKey(cursor, calendar: input.calendar)
                eventsByDay[key, default: []].append(event)
                guard let next = input.calendar.date(byAdding: .day, value: 1, to: cursor) else { break }
                cursor = next
            }
        }

        for key in eventsByDay.keys {
            eventsByDay[key]?.sort { lhs, rhs in
                if lhs.isAllDay != rhs.isAllDay { return lhs.isAllDay && rhs.isAllDay == false }
                return lhs.startDate < rhs.startDate
            }
        }

        let tasks = tasksByDay(input, days: days)
        let sections = days.map { day in
            let key = dayKey(day, calendar: input.calendar)
            return CalendarAgendaDisplaySnapshot.Day(
                date: input.calendar.startOfDay(for: day),
                events: eventsByDay[key] ?? [],
                tasks: tasks[key] ?? []
            )
        }

        return CalendarAgendaDisplaySnapshot(
            key: input.key,
            days: sections,
            eventMetadataByID: metadataByEventID(for: visible, input: input),
            taskMetadataByID: metadataByTaskID(for: Array(tasks.values.joined()), input: input)
        )
    }

    static func yearSnapshot(_ input: CalendarDisplayInput) -> CalendarYearDisplaySnapshot {
        let year = input.calendar.component(.year, from: input.anchorDate)
        guard let yearStart = input.calendar.date(from: DateComponents(year: year, month: 1, day: 1)),
              let yearEnd = input.calendar.date(from: DateComponents(year: year + 1, month: 1, day: 1)) else {
            return CalendarYearDisplaySnapshot(key: input.key, year: year, months: [], countsByDay: [:], maxCount: 0)
        }

        let endInclusive = input.calendar.date(byAdding: .day, value: -1, to: yearEnd) ?? yearStart
        let query = input.searchQuery.trimmingCharacters(in: .whitespacesAndNewlines)
        if query.isEmpty,
           input.settings.pastEventBehavior != .hide,
           input.visibleRange.eventCountsByDay.isEmpty == false {
            let months = (1...12).compactMap { month -> CalendarYearDisplaySnapshot.Month? in
                guard let monthStart = input.calendar.date(from: DateComponents(year: year, month: month, day: 1)) else { return nil }
                return CalendarYearDisplaySnapshot.Month(
                    monthStart: monthStart,
                    monthNumber: month,
                    monthName: monthStart.formatted(.dateTime.month(.wide)),
                    cells: CalendarGridLayout.monthCells(for: monthStart, calendar: input.calendar)
                )
            }
            return CalendarYearDisplaySnapshot(
                key: input.key,
                year: year,
                months: months,
                countsByDay: input.visibleRange.eventCountsByDay,
                maxCount: input.visibleRange.eventCountsByDay.values.max() ?? 0
            )
        }

        let visible = visibleEvents(input, from: yearStart, to: endInclusive)
        var countsByDay: [TimeInterval: Int] = [:]
        for event in visible {
            let eventStart = max(input.calendar.startOfDay(for: event.startDate), yearStart)
            let eventEnd = min(CalendarGridLayout.eventEndDay(event: event, calendar: input.calendar), endInclusive)
            guard eventStart <= eventEnd else { continue }
            var cursor = eventStart
            var steps = 0
            while cursor <= eventEnd && steps < 366 {
                countsByDay[dayKey(cursor, calendar: input.calendar), default: 0] += 1
                guard let next = input.calendar.date(byAdding: .day, value: 1, to: cursor) else { break }
                cursor = next
                steps += 1
            }
        }

        let months = (1...12).compactMap { month -> CalendarYearDisplaySnapshot.Month? in
            guard let monthStart = input.calendar.date(from: DateComponents(year: year, month: month, day: 1)) else { return nil }
            return CalendarYearDisplaySnapshot.Month(
                monthStart: monthStart,
                monthNumber: month,
                monthName: monthStart.formatted(.dateTime.month(.wide)),
                cells: CalendarGridLayout.monthCells(for: monthStart, calendar: input.calendar)
            )
        }

        return CalendarYearDisplaySnapshot(
            key: input.key,
            year: year,
            months: months,
            countsByDay: countsByDay,
            maxCount: countsByDay.values.max() ?? 0
        )
    }

    static func dayKey(_ day: Date, calendar: Calendar) -> TimeInterval {
        calendar.startOfDay(for: day).timeIntervalSinceReferenceDate
    }

    private static func visibleEvents(_ input: CalendarDisplayInput, from rangeStart: Date, to rangeEnd: Date) -> [CalendarEventMirror] {
        let first = input.calendar.startOfDay(for: rangeStart)
        let last = input.calendar.startOfDay(for: rangeEnd)
        let query = input.searchQuery.trimmingCharacters(in: .whitespacesAndNewlines)
        var seen: Set<CalendarEventMirror.ID> = []
        var events: [CalendarEventMirror] = []
        var cursor = first
        while cursor <= last {
            let key = dayKey(cursor, calendar: input.calendar)
            for eventID in input.visibleRange.eventsByDay[key] ?? [] where seen.insert(eventID).inserted {
                guard let event = input.visibleRange.eventByID[eventID] else { continue }
                guard input.selectedCalendarIDs.contains(event.calendarID) else { continue }
                guard input.eventViewFilter.allows(event) else { continue }
                guard input.settings.shouldHidePastEvent(event, now: input.referenceDate) == false else { continue }
                guard query.isEmpty || matchesSearch(event, query: query, projection: input.visibleRange) else { continue }
                events.append(event)
            }
            guard let next = input.calendar.date(byAdding: .day, value: 1, to: cursor) else { break }
            cursor = next
        }
        return events
    }

    private static func matchesSearch(
        _ event: CalendarEventMirror,
        query: String,
        projection: CalendarVisibleRangeProjection
    ) -> Bool {
        if let searchText = projection.eventSearchTextByID[event.id] {
            return searchText.localizedCaseInsensitiveContains(query)
        }
        return event.summary.localizedCaseInsensitiveContains(query)
            || event.details.localizedCaseInsensitiveContains(query)
            || event.location.localizedCaseInsensitiveContains(query)
    }

    private static func tasksByDay(_ input: CalendarDisplayInput, days: [Date]) -> [TimeInterval: [TaskMirror]] {
        var result: [TimeInterval: [TaskMirror]] = [:]
        for day in days {
            let key = dayKey(day, calendar: input.calendar)
            let tasks = (input.visibleRange.tasksByDueDate[key] ?? [])
                .compactMap { input.visibleRange.taskByID[$0] }
                .filter { input.visibleTaskListIDs.contains($0.taskListID) }
                .sorted { $0.title.localizedCaseInsensitiveCompare($1.title) == .orderedAscending }
            if tasks.isEmpty == false {
                result[key] = tasks
            }
        }
        return result
    }

    private static func layoutAllDaySpans(
        _ events: [CalendarEventMirror],
        days: [Date],
        calendar: Calendar
    ) -> [CalendarWeekDisplaySnapshot.AllDaySpan] {
        guard let weekStart = days.first, let weekEnd = days.last else { return [] }
        let weekStartDay = calendar.startOfDay(for: weekStart)
        let weekEndDay = calendar.startOfDay(for: weekEnd)
        let allDay = events.filter(\.isAllDay)

        let spans: [(event: CalendarEventMirror, start: Int, end: Int)] = allDay.compactMap { event in
            let eventStart = calendar.startOfDay(for: event.startDate)
            let eventEnd = CalendarGridLayout.eventEndDay(event: event, calendar: calendar)
            guard eventStart <= weekEndDay, eventEnd >= weekStartDay else { return nil }
            let clampedStart = max(eventStart, weekStartDay)
            let clampedEnd = min(eventEnd, weekEndDay)
            let startIndex = calendar.dateComponents([.day], from: weekStartDay, to: clampedStart).day ?? 0
            let endIndex = calendar.dateComponents([.day], from: weekStartDay, to: clampedEnd).day ?? 0
            let lastColumn = max(days.count - 1, 0)
            return (event, max(0, min(lastColumn, startIndex)), max(0, min(lastColumn, endIndex)))
        }

        let sorted = spans.sorted { lhs, rhs in
            if lhs.start != rhs.start { return lhs.start < rhs.start }
            return lhs.end > rhs.end
        }
        var lanes: [[Int]] = []
        var assigned: [CalendarWeekDisplaySnapshot.AllDaySpan] = []
        for span in sorted {
            var placedLane: Int?
            for (index, lane) in lanes.enumerated() {
                if let last = lane.last, last >= span.start { continue }
                lanes[index].append(span.end)
                placedLane = index
                break
            }
            if placedLane == nil {
                lanes.append([span.end])
                placedLane = lanes.count - 1
            }
            assigned.append(CalendarWeekDisplaySnapshot.AllDaySpan(
                event: span.event,
                startColumn: span.start,
                endColumn: span.end,
                laneIndex: placedLane ?? 0
            ))
        }
        return assigned
    }

    private static func metadataByEventID(
        for events: [CalendarEventMirror],
        input: CalendarDisplayInput
    ) -> [CalendarEventMirror.ID: CalendarEventDisplayMetadata] {
        Dictionary(uniqueKeysWithValues: events.map { event in
            (
                event.id,
                CalendarEventDisplayMetadata(
                    colorHex: CalendarEventColor.from(colorId: event.colorId).hex ?? input.calendarColorHexByID[event.calendarID],
                    opacity: input.settings.opacityForPastEvent(event, now: input.referenceDate),
                    timeRangeLabel: timeRangeLabel(for: event),
                    accessibilityLabel: eventAccessibilityLabel(event)
                )
            )
        })
    }

    private static func metadataByTaskID(
        for tasks: [TaskMirror],
        input: CalendarDisplayInput
    ) -> [TaskMirror.ID: CalendarTaskDisplayMetadata] {
        Dictionary(uniqueKeysWithValues: tasks.map { task in
            let strippedTitle = TagExtractor.stripped(from: task.title)
            let listTitle = input.taskListTitleByID[task.taskListID] ?? ""
            var parts = [task.isCompleted ? "completed task" : "task", strippedTitle]
            if listTitle.isEmpty == false {
                parts.append("list \(listTitle)")
            }
            return (
                task.id,
                CalendarTaskDisplayMetadata(
                    title: task.title,
                    strippedTitle: strippedTitle,
                    listTitle: listTitle,
                    opacity: task.isCompleted ? 0.6 : input.settings.opacityForOverdueTask(task, now: input.referenceDate, calendar: input.calendar),
                    accessibilityLabel: parts.joined(separator: ", ")
                )
            )
        })
    }

    private static func timeRangeLabel(for event: CalendarEventMirror) -> String {
        if event.isAllDay {
            return "All day"
        }
        return "\(event.startDate.formatted(.dateTime.hour().minute())) - \(event.endDate.formatted(.dateTime.hour().minute()))"
    }

    private static func eventAccessibilityLabel(_ event: CalendarEventMirror) -> String {
        if event.isAllDay {
            return "\(event.summary), all day \(event.startDate.formatted(.dateTime.weekday(.wide).month(.abbreviated).day()))"
        }
        let start = event.startDate.formatted(.dateTime.weekday(.wide).hour().minute())
        let end = event.endDate.formatted(.dateTime.hour().minute())
        return "\(event.summary), \(start) to \(end)"
    }
}
