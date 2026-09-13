import CoreGraphics
import XCTest
@testable import HotCrossBunsMac

final class WindowRestorationStoreTests: XCTestCase {
    private var suiteName: String!
    private var defaults: UserDefaults!

    override func setUp() {
        super.setUp()
        suiteName = "WindowRestorationStoreTests-\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suiteName)!
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suiteName)
        defaults = nil
        suiteName = nil
        super.tearDown()
    }

    func testFrameRoundTripsThroughUserDefaults() {
        let store = WindowRestorationStore(defaults: defaults)
        let frame = CGRect(x: 120, y: 240, width: 900, height: 640)

        store.saveFrame(frame, for: .main)

        XCTAssertEqual(store.frame(for: .main), frame)
    }

    func testTinyFramesAreIgnored() {
        let store = WindowRestorationStore(defaults: defaults)

        store.saveFrame(CGRect(x: 1, y: 2, width: 100, height: 100), for: .history)

        XCTAssertNil(store.frame(for: .history))
    }

    func testOpenWindowSetOnlyTracksRestorableAuxiliaryWindows() {
        let store = WindowRestorationStore(defaults: defaults)

        store.markOpen(.main)
        store.markOpen(.history)
        store.markOpen(.help)
        store.markOpen(HCBWindowSceneID(rawValue: "update-available"))

        XCTAssertEqual(store.openWindowIDs(), [.history, .help])
    }

    func testClosingWindowRemovesItFromSession() {
        let store = WindowRestorationStore(defaults: defaults)
        store.markOpen(.history)
        store.markOpen(.syncIssues)

        store.markClosed(.history)

        XCTAssertEqual(store.openWindowIDs(), [.syncIssues])
    }

    func testClearOpenWindowsResetsSessionWithoutDeletingFrames() {
        let store = WindowRestorationStore(defaults: defaults)
        let frame = CGRect(x: 40, y: 50, width: 760, height: 560)
        store.saveFrame(frame, for: .history)
        store.markOpen(.history)

        store.clearOpenWindows()

        XCTAssertTrue(store.openWindowIDs().isEmpty)
        XCTAssertEqual(store.frame(for: .history), frame)
    }

    func testMainWindowSelectionAcceptsMainSceneIdentifier() {
        XCTAssertTrue(HCBMainWindowPresenter.isMainWindow(
            identifierRawValue: HCBWindowSceneID.main.rawValue,
            title: "History"
        ))
    }

    func testMainWindowSelectionUsesExactTitleFallback() {
        XCTAssertTrue(HCBMainWindowPresenter.isMainWindow(
            identifierRawValue: nil,
            title: "Hot Cross Buns"
        ))
        XCTAssertFalse(HCBMainWindowPresenter.isMainWindow(
            identifierRawValue: nil,
            title: "Hot Cross Buns Help"
        ))
    }
}
