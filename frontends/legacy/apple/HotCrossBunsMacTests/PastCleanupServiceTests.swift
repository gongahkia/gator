import XCTest
@testable import HotCrossBunsMac

final class PastCleanupServiceTests: XCTestCase {
    private let calendar: Calendar = {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = TimeZone(identifier: "UTC")!
        return cal
    }()

    private var now: Date {
        // 2026-04-22 10:00 UTC
        calendar.date(from: DateComponents(year: 2026, month: 4, day: 22, hour: 10))!
    }

    private func date(_ year: Int, _ month: Int, _ day: Int, hour: Int = 10) -> Date {
        calendar.date(from: DateComponents(year: year, month: month, day: day, hour: hour))!
    }

    private func baseSettings() -> AppSettings {
        var settings = AppSettings.default
        settings.pastEventBehavior = .delete
        settings.pastEventDeleteThresholdDays = 30
        settings.allowDeletingAttendeeEvents = false
        settings.completedTaskBehavior = .delete
        settings.completedTaskDeleteThresholdDays = 30
        return settings
    }

    private func event(
        id: String,
        end: Date,
        attendees: [String] = [],
        recurrence: [String] = []
    ) -> CalendarEventMirror {
        CalendarEventMirror(
            id: id,
            calendarID: "cal",
            summary: "e",
            details: "",
            startDate: end.addingTimeInterval(-3600),
            endDate: end,
            isAllDay: false,
            status: .confirmed,
            recurrence: recurrence,
            etag: nil,
            updatedAt: nil,
            attendeeEmails: attendees
        )
    }

    private func task(
        id: String,
        completedAt: Date?,
        isCompleted: Bool = true
    ) -> TaskMirror {
        TaskMirror(
            id: id,
            taskListID: "list",
            parentID: nil,
            title: "t",
            notes: "",
            status: isCompleted ? .completed : .needsAction,
            dueDate: nil,
            completedAt: completedAt,
            isDeleted: false,
            isHidden: false
        )
    }

    // MARK: - Events

    func testSingleOccurrencePastEventIncluded() {
        let old = event(id: "evt1", end: date(2026, 3, 1))
        let preview = PastCleanupService.computePreview(
            events: [old], tasks: [], settings: baseSettings(), now: now, calendar: calendar
        )
        XCTAssertEqual(preview.events.map(\.id), ["evt1"])
    }

    func testFutureEventExcluded() {
        let future = event(id: "evt1", end: date(2026, 5, 1))
        let preview = PastCleanupService.computePreview(
            events: [future], tasks: [], settings: baseSettings(), now: now, calendar: calendar
        )
        XCTAssertTrue(preview.events.isEmpty)
    }

    func testRecentPastEventUnderThresholdExcluded() {
        // 5 days ago with a 30-day threshold → not eligible yet.
        let recent = event(id: "evt1", end: calendar.date(byAdding: .day, value: -5, to: now)!)
        let preview = PastCleanupService.computePreview(
            events: [recent], tasks: [], settings: baseSettings(), now: now, calendar: calendar
        )
        XCTAssertTrue(preview.events.isEmpty)
    }

    func testRecurringMasterSkipped() {
        let master = event(id: "series1", end: date(2026, 3, 1), recurrence: ["RRULE:FREQ=WEEKLY"])
        let preview = PastCleanupService.computePreview(
            events: [master], tasks: [], settings: baseSettings(), now: now, calendar: calendar
        )
        XCTAssertTrue(preview.events.isEmpty)
        XCTAssertEqual(preview.recurringMastersSkipped.map(\.id), ["series1"])
    }

    func testRecurringInstancePastIncluded() {
        // Instance IDs have the _YYYYMMDD suffix.
        let instance = event(id: "series1_20260301T100000Z", end: date(2026, 3, 1))
        let preview = PastCleanupService.computePreview(
            events: [instance], tasks: [], settings: baseSettings(), now: now, calendar: calendar
        )
        XCTAssertEqual(preview.events.map(\.id), ["series1_20260301T100000Z"])
        XCTAssertTrue(preview.recurringMastersSkipped.isEmpty)
    }

    func testAttendeeEventSkippedWithoutOptIn() {
        let meeting = event(id: "evt1", end: date(2026, 3, 1), attendees: ["alice@example.com"])
        let preview = PastCleanupService.computePreview(
            events: [meeting], tasks: [], settings: baseSettings(), now: now, calendar: calendar
        )
        XCTAssertTrue(preview.events.isEmpty)
        XCTAssertEqual(preview.attendeeEventsSkipped.map(\.id), ["evt1"])
    }

    func testAttendeeEventIncludedWithOptIn() {
        var settings = baseSettings()
        settings.allowDeletingAttendeeEvents = true
        let meeting = event(id: "evt1", end: date(2026, 3, 1), attendees: ["alice@example.com"])
        let preview = PastCleanupService.computePreview(
            events: [meeting], tasks: [], settings: settings, now: now, calendar: calendar
        )
        XCTAssertEqual(preview.events.map(\.id), ["evt1"])
    }

    func testEventsNotEligibleWhenBehaviorIsNotDelete() {
        var settings = baseSettings()
        settings.pastEventBehavior = .hide
        let old = event(id: "evt1", end: date(2026, 3, 1))
        let preview = PastCleanupService.computePreview(
            events: [old], tasks: [], settings: settings, now: now, calendar: calendar
        )
        XCTAssertTrue(preview.events.isEmpty)
    }

    // MARK: - Tasks

    func testCompletedTaskAgedOutIncluded() {
        let t = task(id: "tsk1", completedAt: date(2026, 3, 1))
        let preview = PastCleanupService.computePreview(
            events: [], tasks: [t], settings: baseSettings(), now: now, calendar: calendar
        )
        XCTAssertEqual(preview.completedTasks.map(\.id), ["tsk1"])
    }

    func testOpenTaskNotIncludedEvenIfOld() {
        let t = task(id: "tsk1", completedAt: nil, isCompleted: false)
        let preview = PastCleanupService.computePreview(
            events: [], tasks: [t], settings: baseSettings(), now: now, calendar: calendar
        )
        XCTAssertTrue(preview.completedTasks.isEmpty)
    }

    func testRecentlyCompletedTaskNotIncluded() {
        // Completed 5 days ago; threshold is 30.
        let t = task(id: "tsk1", completedAt: calendar.date(byAdding: .day, value: -5, to: now))
        let preview = PastCleanupService.computePreview(
            events: [], tasks: [t], settings: baseSettings(), now: now, calendar: calendar
        )
        XCTAssertTrue(preview.completedTasks.isEmpty)
    }

    func testTasksNotEligibleWhenBehaviorIsNotDelete() {
        var settings = baseSettings()
        settings.completedTaskBehavior = .hide
        let t = task(id: "tsk1", completedAt: date(2026, 3, 1))
        let preview = PastCleanupService.computePreview(
            events: [], tasks: [t], settings: settings, now: now, calendar: calendar
        )
        XCTAssertTrue(preview.completedTasks.isEmpty)
    }

    func testEmptyPreviewReportsEmpty() {
        let preview = PastCleanupService.computePreview(
            events: [], tasks: [], settings: baseSettings(), now: now, calendar: calendar
        )
        XCTAssertTrue(preview.isEmpty)
        XCTAssertEqual(preview.totalDeletableCount, 0)
    }
}
