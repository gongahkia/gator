import XCTest
@testable import HotCrossBunsMac

final class RecurrenceUntilRewriterTests: XCTestCase {
    func testAppendsUntilWhenAbsent() {
        let input = "RRULE:FREQ=WEEKLY;INTERVAL=1"
        let output = RecurrenceUntilRewriter.rewrite(rrule: input, until: "20260419T225959Z")
        XCTAssertEqual(output, "RRULE:FREQ=WEEKLY;INTERVAL=1;UNTIL=20260419T225959Z")
    }

    func testReplacesExistingUntil() {
        let input = "RRULE:FREQ=WEEKLY;UNTIL=20270101T000000Z;INTERVAL=2"
        let output = RecurrenceUntilRewriter.rewrite(rrule: input, until: "20260419T225959Z")
        XCTAssertEqual(output, "RRULE:FREQ=WEEKLY;UNTIL=20260419T225959Z;INTERVAL=2")
    }

    func testDropsCountInFavourOfUntil() {
        let input = "RRULE:FREQ=DAILY;COUNT=10"
        let output = RecurrenceUntilRewriter.rewrite(rrule: input, until: "20260419T225959Z")
        XCTAssertEqual(output, "RRULE:FREQ=DAILY;UNTIL=20260419T225959Z")
    }

    func testHandlesMissingRRULEPrefix() {
        let input = "FREQ=MONTHLY"
        let output = RecurrenceUntilRewriter.rewrite(rrule: input, until: "20260501")
        XCTAssertEqual(output, "FREQ=MONTHLY;UNTIL=20260501")
    }

    func testUntilStringForTimedUsesUTC() {
        var comps = DateComponents()
        comps.year = 2026; comps.month = 4; comps.day = 19
        comps.hour = 22; comps.minute = 59; comps.second = 59
        comps.timeZone = TimeZone(secondsFromGMT: 0)
        let date = Calendar(identifier: .gregorian).date(from: comps)!
        XCTAssertEqual(RecurrenceUntilRewriter.untilString(fromCutoff: date, isAllDay: false), "20260419T225959Z")
    }

    func testUntilStringForAllDayIsDateOnly() {
        var comps = DateComponents()
        comps.year = 2026; comps.month = 4; comps.day = 19
        comps.timeZone = TimeZone(secondsFromGMT: 0)
        let date = Calendar(identifier: .gregorian).date(from: comps)!
        XCTAssertEqual(RecurrenceUntilRewriter.untilString(fromCutoff: date, isAllDay: true), "20260419")
    }
}
