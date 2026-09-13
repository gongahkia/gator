import XCTest
@testable import HotCrossBunsMac

final class TagExtractorTests: XCTestCase {
    func testExtractsSingleTag() {
        XCTAssertEqual(TagExtractor.tags(in: "Pay rent #personal"), ["personal"])
    }

    func testExtractsMultipleTags() {
        XCTAssertEqual(TagExtractor.tags(in: "Review PR #work #urgent"), ["work", "urgent"])
    }

    func testIgnoresUnmatchedHashes() {
        XCTAssertEqual(TagExtractor.tags(in: "Use # alone"), [])
        XCTAssertEqual(TagExtractor.tags(in: "C# tutorial"), [])
    }

    func testIgnoresHashUnitNumbers() {
        XCTAssertEqual(TagExtractor.tags(in: "Meet at #11-07"), [])
        XCTAssertEqual(TagExtractor.tags(in: "Visit #11-07 #home"), ["home"])
        XCTAssertEqual(TagExtractor.tags(in: "Date-like room #2026-05-11"), [])
    }

    func testRequiresHashtagBoundary() {
        XCTAssertEqual(TagExtractor.tags(in: "Read C#dev notes"), [])
        XCTAssertEqual(TagExtractor.tags(in: "email foo#bar"), [])
        XCTAssertEqual(TagExtractor.tags(in: "email foo #bar"), ["bar"])
    }

    func testSupportsHyphenAndUnderscore() {
        XCTAssertEqual(TagExtractor.tags(in: "task #side_project-v2"), ["side_project-v2"])
    }

    func testSingleHashBoundarySemantics() {
        XCTAssertEqual(TagExtractor.tags(in: "ship-#launch."), ["launch"])
        XCTAssertEqual(TagExtractor.tags(in: "ship_#launch"), [])
    }

    func testTagLengthLimit() {
        let forty = String(repeating: "a", count: 40)
        let fortyOne = String(repeating: "a", count: 41)

        XCTAssertEqual(TagExtractor.tags(in: "#\(forty)"), [forty])
        XCTAssertEqual(TagExtractor.tags(in: "#\(fortyOne)"), [])
    }

    func testStrippedRemovesTagsAndCollapsesSpaces() {
        XCTAssertEqual(TagExtractor.stripped(from: "Pay rent #personal #urgent"), "Pay rent")
        XCTAssertEqual(TagExtractor.stripped(from: "#work Review PR"), "Review PR")
        XCTAssertEqual(TagExtractor.stripped(from: "Meet at #11-07 #home"), "Meet at #11-07")
    }

    func testStrippedPreservesTextWithoutTags() {
        XCTAssertEqual(TagExtractor.stripped(from: "Write blog post"), "Write blog post")
    }
}
