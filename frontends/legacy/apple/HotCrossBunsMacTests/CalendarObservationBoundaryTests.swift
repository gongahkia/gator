import XCTest
@testable import HotCrossBunsMac

final class CalendarObservationBoundaryTests: XCTestCase {
    func testCalendarSurfacesReadSurfaceStoresInsteadOfBroadAppModelData() throws {
        let files = [
            "apps/apple/HotCrossBuns/Features/Calendar/CalendarHomeView.swift",
            "apps/apple/HotCrossBuns/Features/Calendar/CalendarSidebarFilters.swift",
            "apps/apple/HotCrossBuns/Features/Calendar/DayGridView.swift",
            "apps/apple/HotCrossBuns/Features/Calendar/WeekGridView.swift",
            "apps/apple/HotCrossBuns/Features/Calendar/MonthGridView.swift",
            "apps/apple/HotCrossBuns/Features/Calendar/YearGridView.swift",
            "apps/apple/HotCrossBuns/Features/Calendar/EventContextMenu.swift",
            "apps/apple/HotCrossBuns/Features/Calendar/EventHoverPreview.swift",
            "apps/apple/HotCrossBuns/Features/Calendar/QuickCreatePopover.swift"
        ]
        let forbiddenReads = [
            "model.calendars",
            "model.events",
            "model.calendarSnapshot",
            "model.visibleTaskListIDs",
            "model.settings",
            "model.syncState",
            "model.authState",
            "model.account",
            "model.lastMutationError",
            "model.event(",
            "model.task(",
            "model.eventsByDay",
            "model.tasksByDueDate",
            "model.cachedCalendar",
            "model.storeCalendar",
            "model.taskListTitle(",
            "model.taskLists"
        ]

        for file in files {
            let source = try String(contentsOf: repoRoot.appending(path: file))
            let uncommentedLines = source
                .components(separatedBy: .newlines)
                .map { line in
                    if let commentStart = line.range(of: "//")?.lowerBound {
                        return String(line[..<commentStart])
                    }
                    return line
                }
            let uncommentedSource = uncommentedLines.joined(separator: "\n")
            for forbiddenRead in forbiddenReads {
                XCTAssertFalse(
                    uncommentedSource.contains(forbiddenRead),
                    "\(file) should read \(forbiddenRead) through CalendarStore, TaskStore, or SettingsStore instead of AppModel."
                )
            }
        }
    }

    private var repoRoot: URL {
        URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
    }
}

final class TaskObservationBoundaryTests: XCTestCase {
    func testTaskSurfacesReadTaskStoreInsteadOfBroadAppModelData() throws {
        let files = [
            "apps/apple/HotCrossBuns/Features/Store/StoreView.swift",
            "apps/apple/HotCrossBuns/Features/Store/KanbanView.swift",
            "apps/apple/HotCrossBuns/Features/Tasks/TaskContextMenu.swift",
            "apps/apple/HotCrossBuns/Features/Tasks/TaskBulkActionBar.swift",
            "apps/apple/HotCrossBuns/Features/Tasks/TaskInspectorView.swift"
        ]
        let forbiddenReads = [
            "model.dataRevision",
            "model.taskBoardSnapshot",
            "model.taskLists",
            "model.tasks",
            "model.visibleTaskListIDs",
            "model.settings",
            "model.duplicateIndex",
            "model.task(",
            "model.taskList(",
            "model.taskListTitle(",
            "model.isRebuildingDerivedSnapshots"
        ]

        for file in files {
            let source = try String(contentsOf: repoRoot.appending(path: file))
            let uncommentedLines = source
                .components(separatedBy: .newlines)
                .map { line in
                    if let commentStart = line.range(of: "//")?.lowerBound {
                        return String(line[..<commentStart])
                    }
                    return line
                }
            let uncommentedSource = uncommentedLines.joined(separator: "\n")
            for forbiddenRead in forbiddenReads {
                XCTAssertFalse(
                    uncommentedSource.contains(forbiddenRead),
                    "\(file) should read \(forbiddenRead) through TaskStore instead of AppModel."
                )
            }
        }
    }

    private var repoRoot: URL {
        URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
    }
}

final class ShellMenuObservationBoundaryTests: XCTestCase {
    func testMacSidebarShellReadsNarrowStoresForDisplayState() throws {
        let file = "apps/apple/HotCrossBuns/App/MacSidebarShell.swift"
        let source = try uncommentedSource(file)
        let forbiddenReads = [
            "model.settings.",
            "model.syncState",
            "model.tasks",
            "model.events",
            "model.dataRevision",
            "model.calendars",
            "model.visibleTaskListIDs"
        ]

        for forbiddenRead in forbiddenReads {
            XCTAssertFalse(
                source.contains(forbiddenRead),
                "MacSidebarShell should read \(forbiddenRead) through SettingsStore, SyncStatusStore, CalendarStore, or TaskStore."
            )
        }
    }

    func testLeftSidebarUsesOnlyNativeSplitViewToggle() throws {
        let file = "apps/apple/HotCrossBuns/App/MacSidebarShell.swift"
        let source = try uncommentedSource(file)
        let modifierStart = try XCTUnwrap(source.range(of: "private struct NavigationSurfaceToggleToolbarModifier")?.lowerBound)
        let modifierEnd = try XCTUnwrap(source.range(of: "private struct ActionCenterDynamicContent")?.lowerBound)
        let modifierSource = String(source[modifierStart..<modifierEnd])
        XCTAssertTrue(
            modifierSource.contains("case .left:\n            content"),
            "Left sidebar placement should let NavigationSplitView render its native sidebar toggle."
        )

        let shellStart = try XCTUnwrap(source.range(of: "private var nativeLeftNavigationShell")?.lowerBound)
        let shellEnd = try XCTUnwrap(source.range(of: "private var trailingNavigationShell")?.lowerBound)
        let shellSource = String(source[shellStart..<shellEnd])
        XCTAssertFalse(
            shellSource.contains(".toolbar(removing: .sidebarToggle)"),
            "Native left NavigationSplitView should keep its system sidebar toggle instead of removing it and adding a custom duplicate."
        )
    }

    func testMenuBarSceneReadsMenuBarProjectionInsteadOfRawModelScans() throws {
        let file = "apps/apple/HotCrossBuns/App/MenuBarExtraScene.swift"
        let source = try uncommentedSource(file)
        let forbiddenReads = [
            "model.settings.",
            "model.dataRevision",
            "model.events",
            "model.tasks",
            "model.taskLists",
            "model.calendars",
            "model.menuBarAdaptiveStatus(",
            "model.menuBarEvents(",
            "model.menuBarTasks(",
            "model.menuBarDatedOpenTasks("
        ]

        for forbiddenRead in forbiddenReads {
            XCTAssertFalse(
                source.contains(forbiddenRead),
                "MenuBarExtraScene should read \(forbiddenRead) through MenuBarStore.projection."
            )
        }
    }

    func testCommandPaletteDoesNotKeyEntityCachesFromGlobalDataRevision() throws {
        let file = "apps/apple/HotCrossBuns/App/CommandPaletteView.swift"
        let source = try uncommentedSource(file)
        XCTAssertFalse(
            source.contains("model.dataRevision"),
            "CommandPaletteView should key entity caches from CommandPaletteStore.entityRevision."
        )
    }

    private func uncommentedSource(_ file: String) throws -> String {
        let source = try String(contentsOf: repoRoot.appending(path: file))
        return source
            .components(separatedBy: .newlines)
            .map { line in
                if let commentStart = line.range(of: "//")?.lowerBound {
                    return String(line[..<commentStart])
                }
                return line
            }
            .joined(separator: "\n")
    }

    private var repoRoot: URL {
        URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
    }
}
