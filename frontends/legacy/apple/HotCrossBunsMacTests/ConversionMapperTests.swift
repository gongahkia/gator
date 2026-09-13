import XCTest
@testable import HotCrossBunsMac

final class ConversionMapperTests: XCTestCase {
    private let calendar: Calendar = {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = TimeZone(identifier: "UTC")!
        return cal
    }()

    private func date(_ y: Int, _ m: Int, _ d: Int, h: Int = 0) -> Date {
        calendar.date(from: DateComponents(year: y, month: m, day: d, hour: h))!
    }

    private func event(
        summary: String = "Standup",
        details: String = "Weekly catch-up",
        location: String = "",
        start: Date = Date(),
        end: Date = Date(),
        attendees: [String] = [],
        recurrence: [String] = [],
        colorId: String? = nil,
        reminderMinutes: [Int] = [],
        meetLink: String = ""
    ) -> CalendarEventMirror {
        CalendarEventMirror(
            id: "evt",
            calendarID: "cal",
            summary: summary,
            details: details,
            startDate: start,
            endDate: end,
            isAllDay: false,
            status: .confirmed,
            recurrence: recurrence,
            etag: nil,
            updatedAt: nil,
            reminderMinutes: reminderMinutes,
            location: location,
            attendeeEmails: attendees,
            meetLink: meetLink,
            colorId: colorId
        )
    }

    private func task(
        title: String = "Ship release",
        notes: String = "",
        due: Date? = nil,
        parentID: String? = nil
    ) -> TaskMirror {
        TaskMirror(
            id: "tsk",
            taskListID: "list",
            parentID: parentID,
            title: title,
            notes: notes,
            status: .needsAction,
            dueDate: due,
            completedAt: nil,
            isDeleted: false,
            isHidden: false
        )
    }

    // MARK: - Event → Task/Note notes + dueDate

    func testEventNotesAppendLocation() {
        let e = event(details: "Join via laptop", location: "Studio B")
        let notes = ConversionMapper.taskNotes(fromEvent: e)
        XCTAssertTrue(notes.contains("Join via laptop"))
        XCTAssertTrue(notes.contains("Location: Studio B"))
    }

    func testEventNotesNoLocationNoLocationLine() {
        let e = event(details: "Join via laptop", location: "")
        let notes = ConversionMapper.taskNotes(fromEvent: e)
        XCTAssertEqual(notes, "Join via laptop")
    }

    func testEventNotesEmptyDetailsOnlyLocation() {
        let e = event(details: "", location: "Studio B")
        let notes = ConversionMapper.taskNotes(fromEvent: e)
        XCTAssertEqual(notes, "Location: Studio B")
    }

    func testEventDueDateIsStartOfDay() {
        let e = event(start: date(2026, 4, 22, h: 14), end: date(2026, 4, 22, h: 15))
        let due = ConversionMapper.taskDueDate(fromEvent: e, calendar: calendar)
        XCTAssertEqual(due, date(2026, 4, 22, h: 0))
    }

    // MARK: - Task → Event end-date default

    func testEventEndAllDayIsNextMidnight() {
        let start = date(2026, 4, 22)
        let end = ConversionMapper.eventEnd(fromTaskStart: start, isAllDay: true, calendar: calendar)
        XCTAssertEqual(end, date(2026, 4, 23))
    }

    func testEventEndTimedIsStartPlusHour() {
        let start = date(2026, 4, 22, h: 9)
        let end = ConversionMapper.eventEnd(fromTaskStart: start, isAllDay: false, calendar: calendar)
        XCTAssertEqual(end, date(2026, 4, 22, h: 10))
    }

    // MARK: - Lost fields

    func testEventToTaskListsTimingAndAttendeesWhenPresent() {
        let e = event(attendees: ["a@x"], recurrence: ["RRULE:FREQ=WEEKLY"], colorId: "7", reminderMinutes: [10], meetLink: "https://meet.example")
        let lost = ConversionMapper.lostFieldsForEventToTask(e, preserveDue: true)
        XCTAssertTrue(lost.contains(where: { $0.contains("Start/end times") }))
        XCTAssertTrue(lost.contains(where: { $0.contains("Attendees") }))
        XCTAssertTrue(lost.contains(where: { $0.contains("Recurrence") }))
        XCTAssertTrue(lost.contains(where: { $0.contains("Color") }))
        XCTAssertTrue(lost.contains(where: { $0.contains("Reminders") }))
        XCTAssertTrue(lost.contains(where: { $0.contains("Meet") }))
        XCTAssertFalse(lost.contains("Due date"))
    }

    func testEventToNoteIncludesDueInLostWhenNotPreserved() {
        let e = event()
        let lost = ConversionMapper.lostFieldsForEventToTask(e, preserveDue: false)
        XCTAssertTrue(lost.contains("Due date"))
    }

    func testTaskToEventListsSubtaskParentWhenPresent() {
        let t = task(parentID: "parent-123")
        let lost = ConversionMapper.lostFieldsForTaskToEvent(t, hasDueDate: true)
        XCTAssertTrue(lost.contains(where: { $0.contains("Subtask parent") }))
    }

    func testTaskToEventFlagsWhenNoDue() {
        let t = task(due: nil)
        let lost = ConversionMapper.lostFieldsForTaskToEvent(t, hasDueDate: false)
        XCTAssertTrue(lost.contains(where: { $0.contains("start time") }))
    }
}
