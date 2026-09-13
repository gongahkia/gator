import XCTest
@testable import HotCrossBunsMac

final class GoogleTaskDueDateFormatterTests: XCTestCase {
    private func utcPlus(_ hours: Int) -> Calendar {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(secondsFromGMT: hours * 3600) ?? .current
        return calendar
    }

    func testEncodesLocalDateNotUTCDateInUTCPlus8() {
        let cal = utcPlus(8)
        // Local midnight 2026-04-19 in UTC+8 == 2026-04-18T16:00:00Z
        var comps = DateComponents()
        comps.year = 2026; comps.month = 4; comps.day = 19
        let localMidnight = cal.date(from: comps)!
        let encoded = GoogleTaskDueDateFormatter.string(from: localMidnight, calendar: cal)
        XCTAssertEqual(encoded, "2026-04-19T00:00:00.000Z")
    }

    func testEncodesLocalDateNotUTCDateInUTCMinus5() {
        let cal = utcPlus(-5)
        var comps = DateComponents()
        comps.year = 2026; comps.month = 4; comps.day = 19
        let localMidnight = cal.date(from: comps)!
        let encoded = GoogleTaskDueDateFormatter.string(from: localMidnight, calendar: cal)
        XCTAssertEqual(encoded, "2026-04-19T00:00:00.000Z")
    }

    func testDecodesRFC3339ToLocalMidnight() {
        let cal = utcPlus(8)
        let decoded = GoogleTaskDueDateFormatter.localMidnight(from: "2026-04-19T00:00:00.000Z", calendar: cal)!
        let comps = cal.dateComponents([.year, .month, .day, .hour, .minute], from: decoded)
        XCTAssertEqual(comps.year, 2026)
        XCTAssertEqual(comps.month, 4)
        XCTAssertEqual(comps.day, 19)
        XCTAssertEqual(comps.hour, 0)
        XCTAssertEqual(comps.minute, 0)
    }

    func testDecodesDateOnlyToLocalMidnight() {
        let cal = utcPlus(-5)
        let decoded = GoogleTaskDueDateFormatter.localMidnight(from: "2026-04-19", calendar: cal)!
        let comps = cal.dateComponents([.year, .month, .day, .hour], from: decoded)
        XCTAssertEqual(comps.year, 2026)
        XCTAssertEqual(comps.month, 4)
        XCTAssertEqual(comps.day, 19)
        XCTAssertEqual(comps.hour, 0)
    }

    func testRoundTripPreservesLocalDate() {
        let cal = utcPlus(8)
        var comps = DateComponents()
        comps.year = 2026; comps.month = 4; comps.day = 19
        let localMidnight = cal.date(from: comps)!
        let encoded = GoogleTaskDueDateFormatter.string(from: localMidnight, calendar: cal)
        let decoded = GoogleTaskDueDateFormatter.localMidnight(from: encoded, calendar: cal)!
        XCTAssertEqual(decoded, localMidnight)
    }

    func testReturnsNilForGarbage() {
        XCTAssertNil(GoogleTaskDueDateFormatter.localMidnight(from: "not-a-date"))
        XCTAssertNil(GoogleTaskDueDateFormatter.localMidnight(from: ""))
    }
}
