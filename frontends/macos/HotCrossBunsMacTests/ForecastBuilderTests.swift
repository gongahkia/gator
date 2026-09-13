import XCTest
@testable import HotCrossBunsMac

final class ForecastBuilderTests: XCTestCase {
    private let calendar: Calendar = {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = TimeZone(identifier: "UTC")!
        return cal
    }()

    private var now: Date {
        calendar.date(from: DateComponents(year: 2026, month: 4, day: 18, hour: 9))!
    }

    private func day(_ offset: Int) -> Date {
        calendar.date(byAdding: .day, value: offset, to: calendar.startOfDay(for: now))!
    }

    private func task(id: String, due: Date?, list: String = "L1", completed: Bool = false) -> TaskMirror {
        TaskMirror(
            id: id,
            taskListID: list,
            parentID: nil,
            title: id,
            notes: "",
            status: completed ? .completed : .needsAction,
            dueDate: due,
            completedAt: completed ? now : nil,
            isDeleted: false,
            isHidden: false,
            position: nil,
            etag: nil,
            updatedAt: nil
        )
    }

    private func event(id: String, start: Date, end: Date, cal: String = "primary") -> CalendarEventMirror {
        CalendarEventMirror(
            id: id,
            calendarID: cal,
            summary: id,
            details: "",
            startDate: start,
            endDate: end,
            isAllDay: false,
            status: .confirmed,
            recurrence: [],
            etag: nil,
            updatedAt: nil,
            reminderMinutes: []
        )
    }

    func testOverduePinsTasksBeforeToday() {
        let tasks = [
            task(id: "late", due: day(-5)),
            task(id: "older", due: day(-12)),
            task(id: "today", due: day(0))
        ]
        let forecast = ForecastBuilder.build(
            tasks: tasks,
            events: [],
            selectedTaskListIDs: ["L1"],
            selectedCalendarIDs: [],
            referenceDate: now,
            calendar: calendar
        )
        XCTAssertEqual(forecast.overdueTasks.map(\.id), ["older", "late"])
        XCTAssertEqual(forecast.days[0].tasks.map(\.id), ["today"])
    }

    func testCompletedTasksExcluded() {
        let tasks = [
            task(id: "open", due: day(-1)),
            task(id: "done", due: day(-1), completed: true)
        ]
        let forecast = ForecastBuilder.build(
            tasks: tasks,
            events: [],
            selectedTaskListIDs: ["L1"],
            selectedCalendarIDs: [],
            referenceDate: now,
            calendar: calendar
        )
        XCTAssertEqual(forecast.overdueTasks.map(\.id), ["open"])
    }

    func testHorizonIs14DaysByDefault() {
        let forecast = ForecastBuilder.build(
            tasks: [],
            events: [],
            selectedTaskListIDs: [],
            selectedCalendarIDs: [],
            referenceDate: now,
            calendar: calendar
        )
        XCTAssertEqual(forecast.days.count, ForecastBuilder.horizonDays)
        XCTAssertEqual(forecast.days.first?.date, calendar.startOfDay(for: now))
        XCTAssertEqual(forecast.days.last?.date, day(13))
    }

    func testTasksPlacedOnRightDay() {
        let tasks = [
            task(id: "d3", due: day(3)),
            task(id: "d7", due: day(7)),
            task(id: "past-horizon", due: day(20))
        ]
        let forecast = ForecastBuilder.build(
            tasks: tasks,
            events: [],
            selectedTaskListIDs: ["L1"],
            selectedCalendarIDs: [],
            referenceDate: now,
            calendar: calendar
        )
        XCTAssertEqual(forecast.days[3].tasks.map(\.id), ["d3"])
        XCTAssertEqual(forecast.days[7].tasks.map(\.id), ["d7"])
        XCTAssertFalse(forecast.days.contains { $0.tasks.contains { $0.id == "past-horizon" } }, "past-horizon task should not appear on any day within the forecast")
    }

    func testEventsBucketedByStartDate() {
        let events = [
            event(id: "morning", start: day(1).addingTimeInterval(9 * 3600), end: day(1).addingTimeInterval(10 * 3600)),
            event(id: "off-calendar", start: day(1).addingTimeInterval(11 * 3600), end: day(1).addingTimeInterval(12 * 3600), cal: "other")
        ]
        let forecast = ForecastBuilder.build(
            tasks: [],
            events: events,
            selectedTaskListIDs: [],
            selectedCalendarIDs: ["primary"],
            referenceDate: now,
            calendar: calendar
        )
        XCTAssertEqual(forecast.days[1].events.map(\.id), ["morning"])
    }

    func testTaskListFilterExcludesOtherLists() {
        let tasks = [
            task(id: "mine", due: day(1), list: "L1"),
            task(id: "theirs", due: day(1), list: "L2")
        ]
        let forecast = ForecastBuilder.build(
            tasks: tasks,
            events: [],
            selectedTaskListIDs: ["L1"],
            selectedCalendarIDs: [],
            referenceDate: now,
            calendar: calendar
        )
        XCTAssertEqual(forecast.days[1].tasks.map(\.id), ["mine"])
    }
}
