import XCTest
@testable import HotCrossBunsMac

final class HCBChordTests: XCTestCase {
    private let bindings: [HCBChordBinding] = [
        HCBChordBinding(sequence: ["n", "t"], command: .newTask, hint: "New Task"),
        HCBChordBinding(sequence: ["n", "e"], command: .newEvent, hint: "New Event"),
        HCBChordBinding(sequence: ["g", "s"], command: .goToStore, hint: "Go to Store"),
        HCBChordBinding(sequence: ["g", "c"], command: .goToCalendar, hint: "Go to Calendar"),
        HCBChordBinding(sequence: ["p"], command: .commandPalette, hint: "Command Palette"),
        HCBChordBinding(sequence: ["r"], command: .refresh, hint: "Refresh")
    ]

    // MARK: - matches

    func testEmptyPrefixMatchesEverything() {
        let m = HCBChordMatcher.matches(current: [], in: bindings)
        XCTAssertEqual(m.count, bindings.count)
    }

    func testSingleKeyPrefixNarrows() {
        let m = HCBChordMatcher.matches(current: ["n"], in: bindings)
        XCTAssertEqual(Set(m.map(\.command)), [.newTask, .newEvent])
    }

    func testFullSequenceReturnsExact() {
        let m = HCBChordMatcher.matches(current: ["n", "t"], in: bindings)
        XCTAssertEqual(m.map(\.command), [.newTask])
    }

    func testUnknownPrefixReturnsEmpty() {
        let m = HCBChordMatcher.matches(current: ["x"], in: bindings)
        XCTAssertTrue(m.isEmpty)
    }

    func testCaseInsensitive() {
        let m = HCBChordMatcher.matches(current: ["N", "T"], in: bindings)
        XCTAssertEqual(m.map(\.command), [.newTask])
    }

    // MARK: - isExactTerminal

    func testExactTerminalOnSingleKeyBinding() {
        XCTAssertEqual(HCBChordMatcher.isExactTerminal(current: ["p"], in: bindings)?.command, .commandPalette)
    }

    func testExactTerminalOnTwoKeyBinding() {
        XCTAssertEqual(HCBChordMatcher.isExactTerminal(current: ["n", "t"], in: bindings)?.command, .newTask)
    }

    func testNoTerminalWhenPrefixOnly() {
        // "n" alone is a prefix of ["n","t"] and ["n","e"] — not terminal.
        XCTAssertNil(HCBChordMatcher.isExactTerminal(current: ["n"], in: bindings))
    }

    func testNoTerminalWhenSequenceDoesntMatch() {
        XCTAssertNil(HCBChordMatcher.isExactTerminal(current: ["x"], in: bindings))
    }

    func testExactPrefixOnlyTerminalWhenNoExtensions() {
        // Custom bindings where ["n"] alone AND ["n","t"] both exist — the
        // shorter sequence isn't terminal because the longer one extends it.
        let mixed: [HCBChordBinding] = [
            HCBChordBinding(sequence: ["n"], command: .newTask, hint: "New Task"),
            HCBChordBinding(sequence: ["n", "t"], command: .newEvent, hint: "extension")
        ]
        XCTAssertNil(HCBChordMatcher.isExactTerminal(current: ["n"], in: mixed))
    }

    // MARK: - hudHints

    func testHudHintsListNextKeys() {
        let hints = HCBChordMatcher.hudHints(current: ["n"], in: bindings)
        XCTAssertEqual(hints.map(\.key).sorted(), ["e", "t"])
    }

    func testHudHintsAtRootShowsAllRoots() {
        let hints = HCBChordMatcher.hudHints(current: [], in: bindings)
        XCTAssertEqual(hints.map(\.key).sorted(), ["g", "n", "p", "r"])
    }

    func testHudHintsSingleBindingShowsDirectLabel() {
        let hints = HCBChordMatcher.hudHints(current: ["n"], in: bindings)
        let t = hints.first(where: { $0.key == "t" })
        XCTAssertEqual(t?.label, "New Task")
    }

    func testHudHintsMultipleBindingsShowEllipsis() throws {
        let deep: [HCBChordBinding] = [
            HCBChordBinding(sequence: ["g", "s"], command: .goToStore, hint: "Go to Store"),
            HCBChordBinding(sequence: ["g", "c"], command: .goToCalendar, hint: "Go to Calendar"),
            HCBChordBinding(sequence: ["g", "x", "y"], command: .goToSettings, hint: "deep")
        ]
        // At top level, "g" surfaces 3 underlying bindings → ellipsis label.
        let hints = HCBChordMatcher.hudHints(current: [], in: deep)
        let first = try XCTUnwrap(hints.first, "no hints surfaced for top-level chord prefix")
        XCTAssertEqual(first.key, "g")
        XCTAssertTrue(first.label.contains("actions"))
    }
}
