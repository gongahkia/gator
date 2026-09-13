import XCTest
@testable import HotCrossBunsMac

final class HCBDeepLinkRouterTests: XCTestCase {
    private let calendar: Calendar = {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = TimeZone(identifier: "UTC")!
        return cal
    }()

    private var now: Date {
        calendar.date(from: DateComponents(year: 2026, month: 4, day: 18, hour: 10))!
    }

    private func url(_ s: String) -> URL {
        URL(string: s)!
    }

    private func route(_ s: String) -> Result<HCBDeepLinkAction, HCBDeepLinkError> {
        HCBDeepLinkRouter.route(url(s), now: now, calendar: calendar)
    }

    // MARK: - Scheme safety

    func testRejectsWrongScheme() {
        // Google OAuth redirect / anything else must never be parsed as a deep link.
        XCTAssertTrue(route("https://example.com/task/abc").isFailure)
        XCTAssertTrue(route("com.googleusercontent.apps.123://oauth2redirect/").isFailure)
    }

    func testAcceptsSchemeCaseInsensitive() {
        XCTAssertNoThrow(try route("HotCrossBuns://task/abc").get())
    }

    func testRejectsEmptyHost() {
        if case .success = route("hotcrossbuns://") { XCTFail("should have failed") }
    }

    func testOpenAppRoute() throws {
        let action = try route("hotcrossbuns://open").get()
        XCTAssertEqual(action, .openApp)
    }

    // MARK: - task

    func testOpenTask() throws {
        let action = try route("hotcrossbuns://task/abc123").get()
        XCTAssertEqual(action, .openTask(id: "abc123"))
    }

    func testBuiltTaskLinkRoundTrips() throws {
        let url = HCBDeepLinkBuilder.taskURL(id: "task_abc-123")
        let action = try HCBDeepLinkRouter.route(url, now: now, calendar: calendar).get()
        XCTAssertEqual(url.absoluteString, "hotcrossbuns://task/task_abc-123")
        XCTAssertEqual(action, .openTask(id: "task_abc-123"))
    }

    func testOpenTaskMissingId() {
        XCTAssertTrue(route("hotcrossbuns://task").isFailure)
        XCTAssertTrue(route("hotcrossbuns://task/").isFailure)
    }

    func testOpenTaskIdLengthCap() {
        let longId = String(repeating: "a", count: HCBDeepLinkRouter.maxIdLength + 1)
        XCTAssertTrue(route("hotcrossbuns://task/\(longId)").isFailure)
    }

    // MARK: - event

    func testOpenEvent() throws {
        let action = try route("hotcrossbuns://event/evt_xyz").get()
        XCTAssertEqual(action, .openEvent(id: "evt_xyz"))
    }

    func testBuiltEventLinkRoundTrips() throws {
        let url = HCBDeepLinkBuilder.eventURL(id: "evt_xyz-123")
        let action = try HCBDeepLinkRouter.route(url, now: now, calendar: calendar).get()
        XCTAssertEqual(url.absoluteString, "hotcrossbuns://event/evt_xyz-123")
        XCTAssertEqual(action, .openEvent(id: "evt_xyz-123"))
    }

    func testOpenEventMissingId() {
        XCTAssertTrue(route("hotcrossbuns://event").isFailure)
    }

    // MARK: - new/task

    func testNewTaskTitleOnly() throws {
        let action = try route("hotcrossbuns://new/task?title=Buy%20milk").get()
        guard case .newTask(let p) = action else { return XCTFail() }
        XCTAssertEqual(p.title, "Buy milk")
        XCTAssertNil(p.dueDate)
        XCTAssertTrue(p.tags.isEmpty)
    }

    func testNewTaskAllFields() throws {
        let action = try route("hotcrossbuns://new/task?title=Email%20receipt&notes=call%20the%20bank&due=tomorrow&list=Work&tags=errand,urgent").get()
        guard case .newTask(let p) = action else { return XCTFail() }
        XCTAssertEqual(p.title, "Email receipt")
        XCTAssertEqual(p.notes, "call the bank")
        XCTAssertEqual(p.listIdOrTitle, "Work")
        XCTAssertEqual(p.tags, ["errand", "urgent"])
        let startOfTomorrow = calendar.date(byAdding: .day, value: 1, to: calendar.startOfDay(for: now))
        XCTAssertEqual(p.dueDate, startOfTomorrow)
    }

    func testNewTaskTagsWhitespaceAndHashStripping() throws {
        let action = try route("hotcrossbuns://new/task?title=x&tags=%23work%20%23urgent%20%23%20").get()
        guard case .newTask(let p) = action else { return XCTFail() }
        XCTAssertEqual(p.tags, ["work", "urgent"])
    }

    func testNewTaskRelativeDue() throws {
        let action = try route("hotcrossbuns://new/task?title=x&due=%2B7d").get()
        guard case .newTask(let p) = action else { return XCTFail() }
        let expected = calendar.date(byAdding: .day, value: 7, to: calendar.startOfDay(for: now))
        XCTAssertEqual(p.dueDate, expected)
    }

    func testNewTaskAbsoluteDue() throws {
        let action = try route("hotcrossbuns://new/task?title=x&due=2026-06-15").get()
        guard case .newTask(let p) = action else { return XCTFail() }
        let expected = calendar.date(from: DateComponents(year: 2026, month: 6, day: 15))
        XCTAssertEqual(p.dueDate, expected)
    }

    func testNewTaskInvalidDue() {
        XCTAssertTrue(route("hotcrossbuns://new/task?title=x&due=notadate").isFailure)
    }

    // MARK: - new/event

    func testNewEventDateOnlyImpliesAllDay() throws {
        let action = try route("hotcrossbuns://new/event?title=Off&start=2026-07-04").get()
        guard case .newEvent(let p) = action else { return XCTFail() }
        XCTAssertTrue(p.isAllDay)
        XCTAssertEqual(p.startDate, calendar.date(from: DateComponents(year: 2026, month: 7, day: 4)))
    }

    func testNewEventISO8601() throws {
        let action = try route("hotcrossbuns://new/event?title=Standup&start=2026-04-22T10:00:00Z&end=2026-04-22T10:30:00Z&location=Zoom").get()
        guard case .newEvent(let p) = action else { return XCTFail() }
        XCTAssertEqual(p.title, "Standup")
        XCTAssertEqual(p.location, "Zoom")
        XCTAssertFalse(p.isAllDay)
        XCTAssertNotNil(p.startDate)
        XCTAssertNotNil(p.endDate)
    }

    func testNewEventCalendarRef() throws {
        let action = try route("hotcrossbuns://new/event?title=x&start=2026-04-22T10:00:00Z&calendar=Work%20Calendar").get()
        guard case .newEvent(let p) = action else { return XCTFail() }
        XCTAssertEqual(p.calendarIdOrSummary, "Work Calendar")
    }

    func testNewEventAllDayOverride() throws {
        // allday=1 forces isAllDay even for an ISO timestamp start.
        let action = try route("hotcrossbuns://new/event?title=x&start=2026-04-22T10:00:00Z&allday=1").get()
        guard case .newEvent(let p) = action else { return XCTFail() }
        XCTAssertTrue(p.isAllDay)
    }

    func testNewEventInvalidStart() {
        XCTAssertTrue(route("hotcrossbuns://new/event?title=x&start=notatime").isFailure)
    }

    func testNewResourceUnknown() {
        XCTAssertTrue(route("hotcrossbuns://new/project?name=x").isFailure)
    }

    // MARK: - search

    func testSearchRequiresQuery() {
        XCTAssertTrue(route("hotcrossbuns://search").isFailure)
        XCTAssertTrue(route("hotcrossbuns://search?q=").isFailure)
    }

    func testSearchPrefillsQuery() throws {
        let action = try route("hotcrossbuns://search?q=tag%3Adeep%20AND%20-completed").get()
        XCTAssertEqual(action, .search("tag:deep AND -completed"))
    }

    // MARK: - Unknown host

    func testUnknownHost() {
        XCTAssertTrue(route("hotcrossbuns://refresh").isFailure)
        XCTAssertTrue(route("hotcrossbuns://settings").isFailure)
    }

    // MARK: - Size caps

    func testOversizedParamRejected() {
        let big = String(repeating: "x", count: HCBDeepLinkRouter.maxParamLength + 1)
        let encoded = big.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed)!
        XCTAssertTrue(route("hotcrossbuns://new/task?title=\(encoded)").isFailure)
    }

    // MARK: - Safety invariant: no mutation implied

    func testNewTaskIsAlwaysPrefillNeverAutoSubmitFlag() throws {
        // URL-scheme spec explicitly excluded auto-submit. Even if a user adds
        // a stray &submit=1, the router doesn't honour it — it's ignored and
        // the action is still newTask(prefill), which opens a sheet the user
        // must confirm.
        let action = try route("hotcrossbuns://new/task?title=x&submit=1").get()
        guard case .newTask(let p) = action else { return XCTFail() }
        XCTAssertEqual(p.title, "x")
    }
}

// Test helpers --------------------------------------------------------------

private extension Result where Failure == HCBDeepLinkError {
    var isFailure: Bool {
        if case .failure = self { return true }
        return false
    }
}
