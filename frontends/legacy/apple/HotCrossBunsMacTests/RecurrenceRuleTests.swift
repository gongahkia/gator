import XCTest
@testable import HotCrossBunsMac

final class RecurrenceRuleTests: XCTestCase {
    private let calendar: Calendar = {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = TimeZone(identifier: "UTC")!
        return cal
    }()

    func testRoundTripDaily() {
        let rule = RecurrenceRule(frequency: .daily, interval: 3)
        let round = RecurrenceRule.parse(rrule: rule.rruleString())
        XCTAssertEqual(round, rule)
    }

    func testRoundTripWeekly() {
        let rule = RecurrenceRule(frequency: .weekly, interval: 2)
        XCTAssertEqual(RecurrenceRule.parse(rrule: rule.rruleString()), rule)
    }

    func testRoundTripMonthlyWithCountEnd() {
        let rule = RecurrenceRule(frequency: .monthly, interval: 1, end: .after(5))
        XCTAssertEqual(rule.rruleString(), "RRULE:FREQ=MONTHLY;INTERVAL=1;COUNT=5")
        XCTAssertEqual(RecurrenceRule.parse(rrule: rule.rruleString()), rule)
    }

    func testRoundTripYearlyWithUntilEnd() {
        let until = calendar.date(from: DateComponents(year: 2026, month: 12, day: 31, hour: 23, minute: 59, second: 0))!
        let rule = RecurrenceRule(frequency: .yearly, interval: 1, end: .until(until))
        XCTAssertEqual(rule.rruleString(), "RRULE:FREQ=YEARLY;INTERVAL=1;UNTIL=20261231T235900Z")
        XCTAssertEqual(RecurrenceRule.parse(rrule: rule.rruleString()), rule)
    }

    func testParseWithoutRRULEPrefix() {
        XCTAssertEqual(
            RecurrenceRule.parse(rrule: "FREQ=MONTHLY;INTERVAL=1"),
            RecurrenceRule(frequency: .monthly, interval: 1)
        )
    }

    func testParseInvalidReturnsNil() {
        XCTAssertNil(RecurrenceRule.parse(rrule: "junk"))
    }

    func testAdvanceDaily() {
        let rule = RecurrenceRule(frequency: .daily, interval: 1)
        let start = calendar.date(from: DateComponents(year: 2026, month: 4, day: 18))!
        let expected = calendar.date(from: DateComponents(year: 2026, month: 4, day: 19))!
        XCTAssertEqual(rule.advance(start, calendar: calendar), expected)
    }

    func testAdvanceWeeklyInterval2() {
        let rule = RecurrenceRule(frequency: .weekly, interval: 2)
        let start = calendar.date(from: DateComponents(year: 2026, month: 4, day: 18))!
        let expected = calendar.date(from: DateComponents(year: 2026, month: 5, day: 2))!
        XCTAssertEqual(rule.advance(start, calendar: calendar), expected)
    }

    func testEventDurationFormatterLabelsCommonDurations() {
        XCTAssertEqual(EventDurationFormatter.label(minutes: 15), "15 min")
        XCTAssertEqual(EventDurationFormatter.label(minutes: 60), "1h")
        XCTAssertEqual(EventDurationFormatter.label(minutes: 90), "1h 30m")
        XCTAssertEqual(EventDurationFormatter.label(minutes: 24 * 60), "1d")
    }

    func testEventEndTimeOptionsUseFifteenMinuteDurationsAndKeepCurrentDuration() {
        let start = calendar.date(from: DateComponents(year: 2026, month: 4, day: 18, hour: 9, minute: 0))!
        let customEnd = calendar.date(from: DateComponents(year: 2026, month: 4, day: 18, hour: 10, minute: 7))!

        let options = EventEndTimeOption.options(
            startDate: start,
            currentEndDate: customEnd,
            timeZoneID: "UTC",
            calendar: calendar
        )

        XCTAssertEqual(options.prefix(4).map(\.durationMinutes), [15, 30, 45, 60])
        XCTAssertTrue(options.contains { $0.durationMinutes == 67 })
        XCTAssertTrue(options.contains { $0.durationMinutes == 90 })
        XCTAssertEqual(
            options.first(where: { $0.durationMinutes == 67 })?.durationTitle,
            "1h 7m"
        )
    }

}
