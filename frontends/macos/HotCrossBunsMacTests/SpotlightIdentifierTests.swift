import XCTest
@testable import HotCrossBunsMac

final class SpotlightIdentifierTests: XCTestCase {
    func testParsesTaskIdentifier() {
        let id = SpotlightIdentifier(uniqueIdentifier: SpotlightIndexer.taskURLScheme + "abc123")
        switch id {
        case .task(let value):
            XCTAssertEqual(value, "abc123")
        default:
            XCTFail("Expected task identifier")
        }
    }

    func testParsesEventIdentifier() {
        let id = SpotlightIdentifier(uniqueIdentifier: SpotlightIndexer.eventURLScheme + "evt-9")
        switch id {
        case .event(let value):
            XCTAssertEqual(value, "evt-9")
        default:
            XCTFail("Expected event identifier")
        }
    }

    func testRejectsUnknownPrefix() {
        XCTAssertNil(SpotlightIdentifier(uniqueIdentifier: "other://foo/123"))
    }
}
