import XCTest
@testable import HotCrossBunsMac

final class ICSImporterTests: XCTestCase {
    func testParsesSingleTimedEvent() {
        let ics = """
        BEGIN:VCALENDAR
        VERSION:2.0
        BEGIN:VEVENT
        SUMMARY:Project sync
        DESCRIPTION:Weekly sync\\nAgenda attached
        LOCATION:Room 3
        DTSTART:20260419T140000Z
        DTEND:20260419T150000Z
        END:VEVENT
        END:VCALENDAR
        """
        let drafts = ICSImporter.parse(ics)
        XCTAssertEqual(drafts.count, 1)
        let draft = drafts[0]
        XCTAssertEqual(draft.summary, "Project sync")
        XCTAssertEqual(draft.description, "Weekly sync\nAgenda attached")
        XCTAssertEqual(draft.location, "Room 3")
        XCTAssertFalse(draft.isAllDay)
        XCTAssertEqual(draft.endDate.timeIntervalSince(draft.startDate), 3600)
        XCTAssertEqual(draft.startTimeZoneID, "UTC")
        XCTAssertEqual(draft.endTimeZoneID, "UTC")
    }

    func testParsesTZIDWithOnulNormalization() {
        let ics = """
        BEGIN:VEVENT
        SUMMARY:Legacy zone
        DTSTART;TZID=Asia/Calcutta:20260419T090000
        DTEND;TZID=Asia/Calcutta:20260419T100000
        END:VEVENT
        """
        let drafts = ICSImporter.parse(ics)
        XCTAssertEqual(drafts.count, 1)
        XCTAssertEqual(drafts[0].startTimeZoneID, "Asia/Kolkata")
        XCTAssertEqual(drafts[0].endTimeZoneID, "Asia/Kolkata")
    }

    func testParsesAllDayEvent() {
        let ics = """
        BEGIN:VEVENT
        SUMMARY:Conference
        DTSTART;VALUE=DATE:20260419
        DTEND;VALUE=DATE:20260421
        END:VEVENT
        """
        let drafts = ICSImporter.parse(ics)
        XCTAssertEqual(drafts.count, 1)
        XCTAssertTrue(drafts[0].isAllDay)
        // ICS end is exclusive (20260421); inclusive end should be 20260420.
        let cal = Calendar.current
        let endComponents = cal.dateComponents([.year, .month, .day], from: drafts[0].endDate)
        XCTAssertEqual(endComponents.year, 2026)
        XCTAssertEqual(endComponents.month, 4)
        XCTAssertEqual(endComponents.day, 20)
    }

    func testParsesMultipleEventsAndRRule() {
        let ics = """
        BEGIN:VEVENT
        SUMMARY:First
        DTSTART:20260101T100000Z
        DTEND:20260101T110000Z
        RRULE:FREQ=WEEKLY;INTERVAL=1
        END:VEVENT
        BEGIN:VEVENT
        SUMMARY:Second
        DTSTART:20260102T100000Z
        DTEND:20260102T110000Z
        END:VEVENT
        """
        let drafts = ICSImporter.parse(ics)
        XCTAssertEqual(drafts.count, 2)
        XCTAssertEqual(drafts[0].recurrence, ["RRULE:FREQ=WEEKLY;INTERVAL=1"])
        XCTAssertEqual(drafts[1].recurrence, [])
    }

    func testUnfoldsContinuationLines() {
        let ics = """
        BEGIN:VEVENT
        SUMMARY:Line
         continuation
        DTSTART:20260419T090000Z
        DTEND:20260419T100000Z
        END:VEVENT
        """
        let drafts = ICSImporter.parse(ics)
        XCTAssertEqual(drafts.count, 1)
        XCTAssertEqual(drafts[0].summary, "Linecontinuation")
    }

    func testIgnoresMalformedLinesOutsideVEvent() {
        let ics = """
        BEGIN:VCALENDAR
        foo:bar
        PRODID:some
        BEGIN:VEVENT
        SUMMARY:Keep me
        DTSTART:20260419T140000Z
        DTEND:20260419T150000Z
        END:VEVENT
        END:VCALENDAR
        """
        let drafts = ICSImporter.parse(ics)
        XCTAssertEqual(drafts.count, 1)
        XCTAssertEqual(drafts[0].summary, "Keep me")
    }

    func testIgnoresEventMissingStartDate() {
        let ics = """
        BEGIN:VEVENT
        SUMMARY:Broken
        DTEND:20260419T150000Z
        END:VEVENT
        """
        XCTAssertTrue(ICSImporter.parse(ics).isEmpty)
    }
}
