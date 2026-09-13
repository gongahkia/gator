import SwiftUI
import XCTest
@testable import HotCrossBunsMac

final class AccessibilityFoundationsTests: XCTestCase {
    func testDynamicTypeScaleIsMonotonicAndKeepsLargeAtBaseline() {
        XCTAssertEqual(HCBTextSize.dynamicTypeScale(for: .large), 1.0, accuracy: 0.001)

        let sizes: [DynamicTypeSize] = [
            .xSmall,
            .small,
            .medium,
            .large,
            .xLarge,
            .xxLarge,
            .xxxLarge,
            .accessibility1,
            .accessibility2,
            .accessibility3,
            .accessibility4,
            .accessibility5
        ]

        let scales = sizes.map(HCBTextSize.dynamicTypeScale(for:))
        XCTAssertEqual(scales, scales.sorted(), "Dynamic Type scale should never shrink as the system size increases.")
        XCTAssertGreaterThan(HCBTextSize.dynamicTypeScale(for: .accessibility5), HCBTextSize.dynamicTypeScale(for: .large))
    }

    func testReduceMotionIsUsedByAnimatedSurfaces() throws {
        let files = [
            "apps/apple/HotCrossBuns/Design/HCBAppearance.swift",
            "apps/apple/HotCrossBuns/Design/LoadingView.swift",
            "apps/apple/HotCrossBuns/Design/UndoToast.swift",
            "apps/apple/HotCrossBuns/Design/BulkResultToast.swift",
            "apps/apple/HotCrossBuns/Design/DeepLinkErrorToast.swift",
            "apps/apple/HotCrossBuns/App/MacSidebarShell.swift",
            "apps/apple/HotCrossBuns/Features/Calendar/CalendarHomeView.swift",
            "apps/apple/HotCrossBuns/Features/Calendar/DayGridView.swift",
            "apps/apple/HotCrossBuns/Features/Calendar/WeekGridView.swift",
            "apps/apple/HotCrossBuns/Features/Calendar/MonthGridView.swift",
            "apps/apple/HotCrossBuns/Features/Store/StoreView.swift",
            "apps/apple/HotCrossBuns/Features/Store/KanbanView.swift",
            "apps/apple/HotCrossBuns/Features/QuickAdd/QuickAddView.swift",
            "apps/apple/HotCrossBuns/Features/QuickAdd/QuickAddEventView.swift"
        ]

        for file in files {
            let source = try String(contentsOf: repoRoot.appending(path: file))
            XCTAssertTrue(
                source.contains("accessibilityReduceMotion") || source.contains("HCBMotion."),
                "\(file) should branch animation behavior through reduce-motion."
            )
        }
    }

    func testCoreInteractiveSurfacesHaveVoiceOverLabels() throws {
        let monthGrid = try String(contentsOf: repoRoot.appending(path: "apps/apple/HotCrossBuns/Features/Calendar/MonthGridView.swift"))
        XCTAssertTrue(monthGrid.contains("monthCellAccessibilityLabel"))
        XCTAssertTrue(monthGrid.contains(".accessibilityElement(children: .combine)"))
        XCTAssertTrue(monthGrid.contains(".accessibilityLabel(monthCellAccessibilityLabel"))

        let kanban = try String(contentsOf: repoRoot.appending(path: "apps/apple/HotCrossBuns/Features/Store/KanbanView.swift"))
        XCTAssertTrue(kanban.contains("taskAccessibilityLabel"))
        XCTAssertTrue(kanban.contains("completedTaskAccessibilityLabel"))
        XCTAssertTrue(kanban.contains(".accessibilityLabel(\"Tag \\(tag)\")"))

        let taskBoardBuilder = try String(contentsOf: repoRoot.appending(path: "apps/apple/HotCrossBuns/Features/Store/TaskBoardDisplaySnapshotBuilder.swift"))
        XCTAssertTrue(taskBoardBuilder.contains(#"input.surface == .notes ? "Note" : "Task""#))
        XCTAssertTrue(taskBoardBuilder.contains("accessibilityLabel: parts.joined(separator: \", \")"))
    }

    func testQuickCreateAndMapPreviewAvoidGestureOnlyControls() throws {
        let quickCreate = try String(contentsOf: repoRoot.appending(path: "apps/apple/HotCrossBuns/Features/Calendar/QuickCreatePopover.swift"))
        XCTAssertFalse(quickCreate.contains(".onTapGesture { /* no-op */ }"))
        XCTAssertFalse(quickCreate.contains(".onTapGesture { /* swallow */ }"))
        XCTAssertFalse(quickCreate.contains("isTaskListCardExpanded"))
        XCTAssertTrue(quickCreate.contains("Collapse date options"))
        XCTAssertTrue(quickCreate.contains("Expand task date"))

        let mapPreview = try String(contentsOf: repoRoot.appending(path: "apps/apple/HotCrossBuns/Features/Calendar/LocationMapPreview.swift"))
        XCTAssertFalse(mapPreview.contains(".onTapGesture { isPresentingFullView = true }"))
        XCTAssertTrue(mapPreview.contains(".accessibilityLabel(\"Open full map\")"))
        XCTAssertTrue(mapPreview.contains("fullMapAccessibilityHint"))

        let dayGrid = try String(contentsOf: repoRoot.appending(path: "apps/apple/HotCrossBuns/Features/Calendar/DayGridView.swift"))
        XCTAssertTrue(dayGrid.contains("dayTimedSlotButtons"))
        XCTAssertTrue(dayGrid.contains("timedSlotAccessibilityLabel"))

        let weekGrid = try String(contentsOf: repoRoot.appending(path: "apps/apple/HotCrossBuns/Features/Calendar/WeekGridView.swift"))
        XCTAssertTrue(weekGrid.contains("weekAllDayCreateMenu"))
        XCTAssertTrue(weekGrid.contains("weekTimedSlotMenu"))

        let monthGrid = try String(contentsOf: repoRoot.appending(path: "apps/apple/HotCrossBuns/Features/Calendar/MonthGridView.swift"))
        XCTAssertFalse(monthGrid.contains("monthCellCreateMenu"))
        XCTAssertFalse(monthGrid.contains("New event..."))
        XCTAssertFalse(monthGrid.contains("New all-day event..."))

        let calendarHome = try String(contentsOf: repoRoot.appending(path: "apps/apple/HotCrossBuns/Features/Calendar/CalendarHomeView.swift"))
        XCTAssertFalse(calendarHome.contains("New Event or Task"))
        XCTAssertFalse(calendarHome.contains("Open quick create"))

        let actionCenter = try String(contentsOf: repoRoot.appending(path: "apps/apple/HotCrossBuns/Features/Status/ActionCenter.swift"))
        XCTAssertFalse(actionCenter.contains("onTapGesture"))
    }

    func testCalendarMacControlPolishHasDiscoverableCommandsAndCancelActions() throws {
        let appCommands = try String(contentsOf: repoRoot.appending(path: "apps/apple/HotCrossBuns/App/AppCommands.swift"))
        XCTAssertTrue(appCommands.contains("Button(\"Share Availability…\")"))
        XCTAssertTrue(appCommands.contains("Button(\"Multi-Day View\")"))
        XCTAssertTrue(appCommands.contains("Button(\"Year View\")"))
        XCTAssertTrue(appCommands.contains(".calendarShareAvailability"))
        XCTAssertTrue(appCommands.contains(".calendarViewMultiDay"))
        XCTAssertTrue(appCommands.contains(".calendarViewYear"))
        XCTAssertTrue(appCommands.contains("calendarActions.canShareAvailability == false"))
        XCTAssertTrue(appCommands.contains("calendarActions.canShowMultiDay == false"))
        XCTAssertTrue(appCommands.contains("calendarActions.canShowYear == false"))

        let calendarHome = try String(contentsOf: repoRoot.appending(path: "apps/apple/HotCrossBuns/Features/Calendar/CalendarHomeView.swift"))
        XCTAssertTrue(calendarHome.contains("ViewThatFits(in: .horizontal)"))
        XCTAssertTrue(calendarHome.contains("calendarViewModeControl"))
        XCTAssertTrue(calendarHome.contains("Label(\"Share Availability\", systemImage: \"calendar.badge.clock\")"))
        XCTAssertTrue(calendarHome.contains(".disabled(canOpenShareAvailability == false)"))
        XCTAssertTrue(calendarHome.contains("canShareAvailability: canNavigate && canOpenShareAvailability"))
        XCTAssertTrue(calendarHome.contains("canShowMultiDay: canNavigate && visibleCalendarModes.contains(.multiDay)"))
        XCTAssertTrue(calendarHome.contains("canShowYear: canNavigate && visibleCalendarModes.contains(.year)"))
        XCTAssertTrue(calendarHome.contains(".keyboardShortcut(.cancelAction)"))
        XCTAssertTrue(calendarHome.contains(".help(\"Close Share Availability\")"))

        let actionCenter = try String(contentsOf: repoRoot.appending(path: "apps/apple/HotCrossBuns/Features/Status/ActionCenter.swift"))
        XCTAssertTrue(actionCenter.contains(".keyboardShortcut(.cancelAction)"))
        XCTAssertTrue(actionCenter.contains(".help(\"Close notifications\")"))
    }

    private var repoRoot: URL {
        URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
    }
}
