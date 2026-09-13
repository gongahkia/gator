import AppIntents
import XCTest
@testable import HotCrossBunsMac

final class AppIntentRoutingTests: XCTestCase {
    override func setUp() {
        super.setUp()
        _ = AppIntentHandoff.consumeAll()
    }

    override func tearDown() {
        _ = AppIntentHandoff.consumeAll()
        super.tearDown()
    }

    func testHandoffQueuesRoutesInOrder() {
        AppIntentHandoff.save(.addTask)
        AppIntentHandoff.save(.calendar)
        AppIntentHandoff.save(.store)

        XCTAssertEqual(AppIntentHandoff.consumeAll(), [.addTask, .calendar, .store])
    }

    func testConsumePendingRoutePreservesRemainder() {
        AppIntentHandoff.save(.addEvent)
        AppIntentHandoff.save(.calendar)

        XCTAssertEqual(AppIntentHandoff.consumePendingRoute(), .addEvent)
        XCTAssertEqual(AppIntentHandoff.consumeAll(), [.calendar])
    }

    func testAppIntentsEnqueueExpectedRoutes() async throws {
        _ = try await AddGoogleTaskIntent().perform()
        XCTAssertEqual(AppIntentHandoff.consumeAll(), [.addTask])

        _ = try await AddGoogleCalendarEventIntent().perform()
        XCTAssertEqual(AppIntentHandoff.consumeAll(), [.addEvent])

        _ = try await OpenHotCrossBunsCalendarIntent().perform()
        XCTAssertEqual(AppIntentHandoff.consumeAll(), [.calendar])

        _ = try await OpenHotCrossBunsStoreIntent().perform()
        XCTAssertEqual(AppIntentHandoff.consumeAll(), [.store])
    }

    func testSharedInboxSanitizesOversizedTextForWrite() throws {
        let item = SharedInboxItem(
            text: String(repeating: "a", count: SharedInboxDefaults.maxTextBytes + 128),
            createdAt: Date(),
            source: "com.gongahkia.hotcrossbuns.mac.share"
        )

        let sanitized = try XCTUnwrap(SharedInboxDefaults.sanitizedForWrite(item))
        XCTAssertLessThanOrEqual(sanitized.text.utf8.count, SharedInboxDefaults.maxTextBytes)
    }

    func testSharedInboxRejectsUntrustedSourceForWrite() {
        let item = SharedInboxItem(
            text: "hello",
            createdAt: Date(),
            source: "com.example.other"
        )

        XCTAssertNil(SharedInboxDefaults.sanitizedForWrite(item))
    }
}
