import Foundation

enum CalendarGridMode: String, CaseIterable, Hashable, Sendable {
    case agenda
    case day
    case multiDay // §7.01 Phase D2 — N-day configurable window
    case week
    case month
    case year // §7.01 Phase D3 — 4x3 mini-months overview

    var title: String {
        switch self {
        case .agenda: "Agenda"
        case .day: "Day"
        case .multiDay: "Multi-Day"
        case .week: "Week"
        case .month: "Month"
        case .year: "Year"
        }
    }

    var systemImage: String {
        switch self {
        case .agenda: "list.bullet.rectangle"
        case .day: "calendar.day.timeline.leading"
        case .multiDay: "calendar.day.timeline.trailing"
        case .week: "calendar.day.timeline.left"
        case .month: "calendar"
        case .year: "square.grid.3x3"
        }
    }
}

enum CalendarGridLayout {
    static func weekDays(containing date: Date, calendar: Calendar = .current) -> [Date] {
        let startOfWeek = startOfWeek(containing: date, calendar: calendar)
        return (0..<7).compactMap { offset in
            calendar.date(byAdding: .day, value: offset, to: startOfWeek)
        }
    }

    static func startOfWeek(containing date: Date, calendar: Calendar = .current) -> Date {
        let startOfDay = calendar.startOfDay(for: date)
        let weekday = calendar.component(.weekday, from: startOfDay)
        let delta = (weekday - calendar.firstWeekday + 7) % 7
        return calendar.date(byAdding: .day, value: -delta, to: startOfDay) ?? startOfDay
    }

    static func monthCells(for date: Date, calendar: Calendar = .current) -> [Date] {
        let startOfMonth = calendar.date(from: calendar.dateComponents([.year, .month], from: date)) ?? date
        let leadingOffset: Int = {
            let weekday = calendar.component(.weekday, from: startOfMonth)
            return (weekday - calendar.firstWeekday + 7) % 7
        }()
        let gridStart = calendar.date(byAdding: .day, value: -leadingOffset, to: startOfMonth) ?? startOfMonth
        return (0..<42).compactMap { offset in
            calendar.date(byAdding: .day, value: offset, to: gridStart)
        }
    }

    static func eventsByDay(
        _ events: [CalendarEventMirror],
        from rangeStart: Date,
        to rangeEnd: Date,
        calendar: Calendar = .current
    ) -> [Date: [CalendarEventMirror]] {
        let startOfRange = calendar.startOfDay(for: rangeStart)
        let endOfRange = calendar.startOfDay(for: rangeEnd)
        var bucket: [Date: [CalendarEventMirror]] = [:]

        for event in events where event.status != .cancelled {
            let eventStartDay = calendar.startOfDay(for: event.startDate)
            let inclusiveEndDay = eventEndDay(event: event, calendar: calendar)

            let firstDay = max(eventStartDay, startOfRange)
            let lastDay = min(inclusiveEndDay, endOfRange)
            guard firstDay <= lastDay else { continue }

            var cursor = firstDay
            while cursor <= lastDay {
                bucket[cursor, default: []].append(event)
                guard let next = calendar.date(byAdding: .day, value: 1, to: cursor) else { break }
                cursor = next
            }
        }

        for day in bucket.keys {
            bucket[day]?.sort { lhs, rhs in
                if lhs.isAllDay != rhs.isAllDay { return lhs.isAllDay && rhs.isAllDay == false }
                return lhs.startDate < rhs.startDate
            }
        }
        return bucket
    }

    static func eventEndDay(event: CalendarEventMirror, calendar: Calendar = .current) -> Date {
        if event.isAllDay {
            let startOfEndDay = calendar.startOfDay(for: event.endDate)
            if startOfEndDay > calendar.startOfDay(for: event.startDate) {
                return calendar.date(byAdding: .day, value: -1, to: startOfEndDay) ?? startOfEndDay
            }
            return startOfEndDay
        }
        return calendar.startOfDay(for: event.endDate)
    }

    struct MonthBand: Identifiable, Equatable, Sendable {
        let event: CalendarEventMirror
        let startColumn: Int // inclusive 0..6 within week
        let endColumn: Int   // inclusive 0..6 within week
        let lane: Int
        var id: String { "\(event.id)-\(startColumn)-\(endColumn)" }
    }

    // Events eligible for month-grid bands: all-day events (single or multi-day)
    // and timed events that span more than one day. Single-day timed events
    // still render as per-cell chips.
    static func isBandEvent(_ event: CalendarEventMirror, calendar: Calendar = .current) -> Bool {
        if event.isAllDay { return true }
        let startDay = calendar.startOfDay(for: event.startDate)
        let endDay = eventEndDay(event: event, calendar: calendar)
        return endDay > startDay
    }

    // For a given week (array of 7 days, leading = column 0), return band rects
    // per event that intersects the week, with greedy lane assignment so bars
    // don't overlap. Events spanning multiple weeks produce one band per week.
    static func monthBands(for week: [Date], events: [CalendarEventMirror], calendar: Calendar = .current) -> [MonthBand] {
        guard week.isEmpty == false else { return [] }
        let weekStart = calendar.startOfDay(for: week[0])
        let weekEnd = calendar.startOfDay(for: week[week.count - 1])
        let bandEvents = events
            .filter { isBandEvent($0, calendar: calendar) }
            .sorted { lhs, rhs in
                let lhsStart = calendar.startOfDay(for: lhs.startDate)
                let rhsStart = calendar.startOfDay(for: rhs.startDate)
                if lhsStart != rhsStart { return lhsStart < rhsStart }
                let lhsEnd = eventEndDay(event: lhs, calendar: calendar)
                let rhsEnd = eventEndDay(event: rhs, calendar: calendar)
                return lhsEnd > rhsEnd // longer spans first
            }

        var laneOccupancy: [[Bool]] = [] // lane × column
        var result: [MonthBand] = []

        for event in bandEvents {
            let startDay = calendar.startOfDay(for: event.startDate)
            let endDay = eventEndDay(event: event, calendar: calendar)
            let clampedStart = max(startDay, weekStart)
            let clampedEnd = min(endDay, weekEnd)
            guard clampedStart <= clampedEnd else { continue }
            let startCol = calendar.dateComponents([.day], from: weekStart, to: clampedStart).day ?? 0
            let endCol = calendar.dateComponents([.day], from: weekStart, to: clampedEnd).day ?? 0
            guard startCol >= 0, endCol <= week.count - 1, startCol <= endCol else { continue }

            var laneIndex = 0
            while true {
                if laneIndex >= laneOccupancy.count {
                    laneOccupancy.append(Array(repeating: false, count: week.count))
                }
                let row = laneOccupancy[laneIndex]
                let available = (startCol...endCol).allSatisfy { row[$0] == false }
                if available {
                    for c in startCol...endCol { laneOccupancy[laneIndex][c] = true }
                    result.append(MonthBand(event: event, startColumn: startCol, endColumn: endCol, lane: laneIndex))
                    break
                }
                laneIndex += 1
            }
        }
        return result
    }

    struct LaidOutEvent: Equatable, Sendable {
        let event: CalendarEventMirror
        let columnIndex: Int
        let columnCount: Int
    }

    static func layout(eventsInDay events: [CalendarEventMirror], calendar: Calendar = .current) -> [LaidOutEvent] {
        let sorted = events.sorted { $0.startDate < $1.startDate }
        var groups: [[CalendarEventMirror]] = []
        var currentGroup: [CalendarEventMirror] = []
        var currentGroupEnd: Date?

        for event in sorted {
            guard let groupEnd = currentGroupEnd else {
                currentGroup = [event]
                currentGroupEnd = event.endDate
                continue
            }

            if event.startDate < groupEnd {
                currentGroup.append(event)
                if event.endDate > groupEnd {
                    currentGroupEnd = event.endDate
                }
            } else {
                groups.append(currentGroup)
                currentGroup = [event]
                currentGroupEnd = event.endDate
            }
        }

        if currentGroup.isEmpty == false {
            groups.append(currentGroup)
        }

        return groups.flatMap { layoutCollisionGroup($0) }
    }

    private static func layoutCollisionGroup(_ events: [CalendarEventMirror]) -> [LaidOutEvent] {
        let sorted = events.sorted { $0.startDate < $1.startDate }
        var columns: [[CalendarEventMirror]] = []

        for event in sorted {
            var placed = false
            for columnIndex in columns.indices {
                if let last = columns[columnIndex].last, last.endDate <= event.startDate {
                    columns[columnIndex].append(event)
                    placed = true
                    break
                }
            }
            if placed == false {
                columns.append([event])
            }
        }

        var result: [LaidOutEvent] = []
        for (columnIndex, bucket) in columns.enumerated() {
            for event in bucket {
                result.append(LaidOutEvent(event: event, columnIndex: columnIndex, columnCount: columns.count))
            }
        }
        return result
    }
}
