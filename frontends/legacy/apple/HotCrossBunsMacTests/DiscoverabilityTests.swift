import XCTest
@testable import HotCrossBunsMac

@MainActor
final class DiscoverabilityTests: XCTestCase {
    func testSidebarItemsExposeShortcutAwareTooltips() {
        for item in SidebarItem.allCases {
            let help = item.navigationHelp()

            XCTAssertTrue(help.contains("Jump to \(item.title)"))
            XCTAssertTrue(help.contains(item.shortcutCommand.defaultBinding.displayLabel))
        }
    }

    func testFeatureTourIsFirstRunAndMarksSeen() throws {
        let settings = try JSONDecoder().decode(AppSettings.self, from: Data("{}".utf8))

        XCTAssertFalse(settings.hasSeenFeatureTour)

        var next = settings
        next.hasSeenFeatureTour = true
        let data = try JSONEncoder().encode(next)
        let decoded = try JSONDecoder().decode(AppSettings.self, from: data)

        XCTAssertTrue(decoded.hasSeenFeatureTour)

        let source = try String(contentsOf: repoRoot.appending(path: "apps/apple/HotCrossBuns/App/MacSidebarShell.swift"))
        XCTAssertTrue(source.contains("presentFeatureTourIfNeeded()"))
        XCTAssertTrue(source.contains("shellChrome.hasSeenFeatureTour == false"))
        XCTAssertTrue(source.contains("model.markFeatureTourSeen()"))
    }

    func testQuickAddSheetsExposeInlineGrammarReferences() throws {
        let taskSource = try String(contentsOf: repoRoot.appending(path: "apps/apple/HotCrossBuns/Features/QuickAdd/QuickAddView.swift"))
        let eventSource = try String(contentsOf: repoRoot.appending(path: "apps/apple/HotCrossBuns/Features/QuickAdd/QuickAddEventView.swift"))

        XCTAssertTrue(taskSource.contains("NaturalLanguageTaskParser.helpEntries"))
        XCTAssertTrue(eventSource.contains("NaturalLanguageEventParser.helpEntries"))
        XCTAssertTrue(taskSource.contains("isGrammarExpanded"))
        XCTAssertTrue(eventSource.contains("isGrammarExpanded"))
        XCTAssertTrue(taskSource.contains("Show quick-add grammar"))
        XCTAssertTrue(eventSource.contains("Show quick-add grammar"))
    }

    private var repoRoot: URL {
        URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
    }
}
