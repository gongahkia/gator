import XCTest
@testable import HotCrossBunsMac

final class CalendarEventInstanceTests: XCTestCase {
    func testDetectsTimedInstanceID() {
        XCTAssertTrue(CalendarEventInstance.isInstanceID("abc123_20260420T090000Z"))
    }

    func testDetectsAllDayInstanceID() {
        XCTAssertTrue(CalendarEventInstance.isInstanceID("abc123_20260420"))
    }

    func testRejectsSeriesRootID() {
        XCTAssertFalse(CalendarEventInstance.isInstanceID("abc123"))
    }

    func testSeriesIDStripsTimedSuffix() {
        XCTAssertEqual(
            CalendarEventInstance.seriesID(from: "abc123_20260420T090000Z"),
            "abc123"
        )
    }

    func testSeriesIDStripsAllDaySuffix() {
        XCTAssertEqual(
            CalendarEventInstance.seriesID(from: "abc123_20260420"),
            "abc123"
        )
    }

    func testSeriesIDIsIdentityForRoot() {
        XCTAssertEqual(CalendarEventInstance.seriesID(from: "abc123"), "abc123")
    }
}
