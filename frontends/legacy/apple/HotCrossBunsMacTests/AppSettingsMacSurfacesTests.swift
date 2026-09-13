import Carbon.HIToolbox
import XCTest
@testable import HotCrossBunsMac

final class AppSettingsMacSurfacesTests: XCTestCase {
    func testNewFlagsDefaultToTrueOnLegacyCache() throws {
        let data = Data(
            """
            {
              "syncMode": "balanced",
              "selectedCalendarIDs": [],
              "selectedTaskListIDs": [],
              "enableLocalNotifications": false
            }
            """.utf8
        )

        let settings = try JSONDecoder().decode(AppSettings.self, from: data)

        XCTAssertTrue(settings.showMenuBarExtra, "legacy caches should opt into the menu bar extra by default")
        XCTAssertFalse(settings.showDetailedMenuBar, "legacy caches should default to compact menu bar panel")
        XCTAssertEqual(settings.menuBarIcon, .buns, "legacy caches should keep the original menu bar icon")
        XCTAssertEqual(settings.menuBarAdaptiveStatusSource, .events)
        XCTAssertEqual(settings.menuBarAdaptiveEmptyBehavior, .iconOnly)
        XCTAssertEqual(settings.menuBarAdaptivePanelContent, .events)
        XCTAssertTrue(settings.showDockBadge, "legacy caches should opt into the dock badge by default")
        XCTAssertTrue(settings.restoreWindowStateEnabled, "legacy caches should restore prior window sessions by default")
        XCTAssertEqual(settings.globalHotkeyBinding, .defaultQuickAdd)
        XCTAssertEqual(settings.sidebarPlacement, .left)
        XCTAssertFalse(settings.disableAnimations, "legacy caches should keep animations enabled by default")
        XCTAssertEqual(settings.performanceMode, .snappy, "legacy caches should favor snappy rendering by default")
        XCTAssertFalse(settings.mcpServerEnabled, "legacy caches must not expose a local MCP server by default")
        XCTAssertEqual(settings.mcpPermissionMode, .confirmWrites)
        XCTAssertEqual(settings.mcpPort, 8765)
    }

    func testEncodedSettingsRoundTripPreservesMacSurfaceFlags() throws {
        let settings = AppSettings(
            syncMode: .manual,
            selectedCalendarIDs: ["primary"],
            selectedTaskListIDs: ["tasks"],
            enableLocalNotifications: true,
            showMenuBarExtra: false,
            showDetailedMenuBar: true,
            menuBarIcon: .sparkles,
            showDockBadge: false,
            restoreWindowStateEnabled: false,
            globalHotkeyBinding: GlobalHotkeyBinding(
                keyCode: UInt32(kVK_ANSI_K),
                key: .char("k"),
                modifiers: [.command, .shift]
            ),
            menuBarStyle: .adaptive,
            menuBarAdaptiveStatusSource: .eventsAndTasks,
            menuBarAdaptiveEmptyBehavior: .clear,
            menuBarAdaptivePanelContent: .tasks,
            disableAnimations: true,
            performanceMode: .rich,
            sidebarPlacement: .bottom,
            mcpServerEnabled: true,
            mcpPermissionMode: .allowWrites,
            mcpPort: 9876
        )

        let data = try JSONEncoder().encode(settings)
        let decoded = try JSONDecoder().decode(AppSettings.self, from: data)

        XCTAssertFalse(decoded.showMenuBarExtra)
        XCTAssertTrue(decoded.showDetailedMenuBar)
        XCTAssertEqual(decoded.menuBarStyle, .adaptive)
        XCTAssertEqual(decoded.menuBarAdaptiveStatusSource, .eventsAndTasks)
        XCTAssertEqual(decoded.menuBarAdaptiveEmptyBehavior, .clear)
        XCTAssertEqual(decoded.menuBarAdaptivePanelContent, .tasks)
        XCTAssertEqual(decoded.menuBarIcon, .sparkles)
        XCTAssertFalse(decoded.showDockBadge)
        XCTAssertFalse(decoded.restoreWindowStateEnabled)
        XCTAssertTrue(decoded.disableAnimations)
        XCTAssertEqual(decoded.performanceMode, .rich)
        XCTAssertEqual(decoded.globalHotkeyBinding.displayLabel, "⇧⌘K")
        XCTAssertEqual(decoded.sidebarPlacement, .bottom)
        XCTAssertTrue(decoded.mcpServerEnabled)
        XCTAssertEqual(decoded.mcpPermissionMode, .allowWrites)
        XCTAssertEqual(decoded.mcpPort, 9876)
    }

    func testMenuBarIconCatalogExposesUpToFiftyChoices() {
        XCTAssertEqual(AppSettings.MenuBarIcon.allCases.count, 50)
    }

    func testAdaptiveMenuBarSettingsMetadata() {
        XCTAssertTrue(AppSettings.MenuBarStyle.allCases.contains(.adaptive))
        XCTAssertEqual(AppSettings.MenuBarStyle.adaptive.title, "Adaptive")
        XCTAssertEqual(AppSettings.MenuBarAdaptiveStatusSource.events.title, "Events")
        XCTAssertEqual(AppSettings.MenuBarAdaptiveStatusSource.tasks.title, "Tasks")
        XCTAssertEqual(AppSettings.MenuBarAdaptiveStatusSource.eventsAndTasks.title, "Events + Tasks")
        XCTAssertEqual(AppSettings.MenuBarAdaptiveEmptyBehavior.iconOnly.title, "Icon only")
        XCTAssertEqual(AppSettings.MenuBarAdaptiveEmptyBehavior.clear.title, "Clear")
        XCTAssertEqual(AppSettings.MenuBarAdaptiveEmptyBehavior.nextCommitment.title, "Next commitment")
        XCTAssertEqual(AppSettings.MenuBarAdaptivePanelContent.events.title, "Events only")
        XCTAssertEqual(AppSettings.MenuBarAdaptivePanelContent.tasks.title, "Tasks only")
        XCTAssertEqual(AppSettings.MenuBarAdaptivePanelContent.eventsAndTasks.title, "Events + Tasks")
    }

    func testAdaptiveStatusUsesCurrentEventTimeRemaining() {
        let calendar = adaptiveCalendar()
        let now = adaptiveDate(hour: 10, minute: 10, calendar: calendar)
        let current = adaptiveEvent(
            id: "current-event",
            summary: "Deep Work",
            start: adaptiveDate(hour: 10, calendar: calendar),
            end: adaptiveDate(hour: 11, calendar: calendar)
        )
        let next = adaptiveEvent(
            id: "next-event",
            summary: "Design Review",
            start: adaptiveDate(hour: 10, minute: 30, calendar: calendar),
            end: adaptiveDate(hour: 11, minute: 30, calendar: calendar)
        )

        let status = MenuBarAdaptiveStatusResolver.status(
            now: now,
            events: [next, current],
            tasks: [],
            source: .events,
            emptyBehavior: .iconOnly,
            calendar: calendar
        )

        XCTAssertEqual(status.label, "Deep Work - 50m left")
        XCTAssertEqual(status.kind, .currentEvent("current-event"))
    }

    func testAdaptiveStatusUsesNextEventCountdownWhenNoCurrentEvent() {
        let calendar = adaptiveCalendar()
        let now = adaptiveDate(hour: 10, minute: 10, calendar: calendar)
        let cancelled = adaptiveEvent(
            id: "cancelled",
            summary: "Cancelled",
            start: adaptiveDate(hour: 10, minute: 20, calendar: calendar),
            end: adaptiveDate(hour: 11, calendar: calendar),
            status: .cancelled
        )
        let next = adaptiveEvent(
            id: "next-event",
            summary: "Design Review",
            start: adaptiveDate(hour: 10, minute: 30, calendar: calendar),
            end: adaptiveDate(hour: 11, calendar: calendar)
        )

        let status = MenuBarAdaptiveStatusResolver.status(
            now: now,
            events: [cancelled, next],
            tasks: [],
            source: .events,
            emptyBehavior: .iconOnly,
            calendar: calendar
        )

        XCTAssertEqual(status.label, "Design Review - in 20m")
        XCTAssertEqual(status.kind, .nextEvent("next-event"))
    }

    func testAdaptiveStatusUsesParenthesizedDetailForTruncatedEventTitle() {
        let calendar = adaptiveCalendar()
        let now = adaptiveDate(hour: 10, calendar: calendar)
        let next = adaptiveEvent(
            id: "next-event",
            summary: "ABCDEFGHIJKLMNOPQRSTUVWXYZ12345",
            start: adaptiveDate(hour: 11, minute: 53, calendar: calendar),
            end: adaptiveDate(hour: 12, calendar: calendar)
        )

        let status = MenuBarAdaptiveStatusResolver.status(
            now: now,
            events: [next],
            tasks: [],
            source: .events,
            emptyBehavior: .iconOnly,
            calendar: calendar
        )

        // Truncated titles move timing into parentheses instead of using the normal dash separator.
        XCTAssertEqual(status.label, "ABCDEFGHIJKLMNOPQRSTUVWXY... (in 1h 53m)")
        XCTAssertEqual(status.kind, .nextEvent("next-event"))
    }

    func testAdaptiveStatusSupportsIconOnlyAndClearEmptyStates() {
        let calendar = adaptiveCalendar()
        let now = adaptiveDate(hour: 10, calendar: calendar)

        let iconOnly = MenuBarAdaptiveStatusResolver.status(
            now: now,
            events: [],
            tasks: [],
            source: .events,
            emptyBehavior: .iconOnly,
            calendar: calendar
        )
        let clear = MenuBarAdaptiveStatusResolver.status(
            now: now,
            events: [],
            tasks: [],
            source: .events,
            emptyBehavior: .clear,
            calendar: calendar
        )

        XCTAssertNil(iconOnly.label)
        XCTAssertEqual(iconOnly.kind, .iconOnly)
        XCTAssertEqual(clear.label, "Clear")
        XCTAssertEqual(clear.kind, .clear)
    }

    func testAdaptiveStatusUsesOnlyDatedOpenTasks() {
        let calendar = adaptiveCalendar()
        let now = adaptiveDate(hour: 10, calendar: calendar)
        let dueToday = adaptiveTask(id: "due", title: "Submit invoice", due: adaptiveDate(hour: 0, calendar: calendar))
        let completed = adaptiveTask(
            id: "completed",
            title: "Completed",
            due: adaptiveDate(day: 9, hour: 0, calendar: calendar),
            status: .completed
        )
        let hidden = adaptiveTask(
            id: "hidden",
            title: "Hidden",
            due: adaptiveDate(day: 9, hour: 0, calendar: calendar),
            isHidden: true
        )
        let deleted = adaptiveTask(
            id: "deleted",
            title: "Deleted",
            due: adaptiveDate(day: 9, hour: 0, calendar: calendar),
            isDeleted: true
        )
        let undated = adaptiveTask(id: "undated", title: "Undated", due: nil)

        let status = MenuBarAdaptiveStatusResolver.status(
            now: now,
            events: [],
            tasks: [completed, hidden, deleted, undated, dueToday],
            source: .tasks,
            emptyBehavior: .iconOnly,
            calendar: calendar
        )

        XCTAssertEqual(status.label, "Submit invoice - due today")
        XCTAssertEqual(status.kind, .task("due"))
    }

    func testAdaptiveStatusUsesParenthesizedDetailForTruncatedTaskTitle() {
        let calendar = adaptiveCalendar()
        let now = adaptiveDate(hour: 10, calendar: calendar)
        let dueToday = adaptiveTask(
            id: "due",
            title: "ABCDEFGHIJKLMNOPQRSTUVWXYZ12345",
            due: adaptiveDate(hour: 0, calendar: calendar)
        )

        let status = MenuBarAdaptiveStatusResolver.status(
            now: now,
            events: [],
            tasks: [dueToday],
            source: .tasks,
            emptyBehavior: .iconOnly,
            calendar: calendar
        )

        // Truncated titles move timing into parentheses instead of using the normal dash separator.
        XCTAssertEqual(status.label, "ABCDEFGHIJKLMNOPQRSTUVWXY... (due today)")
        XCTAssertEqual(status.kind, .task("due"))
    }

    func testAdaptiveStatusCanFallbackToNextCommitmentAcrossSources() {
        let calendar = adaptiveCalendar()
        let now = adaptiveDate(hour: 10, calendar: calendar)
        let tomorrowTask = adaptiveTask(id: "tomorrow", title: "Prepare notes", due: adaptiveDate(day: 11, hour: 0, calendar: calendar))

        let status = MenuBarAdaptiveStatusResolver.status(
            now: now,
            events: [],
            tasks: [tomorrowTask],
            source: .events,
            emptyBehavior: .nextCommitment,
            calendar: calendar
        )

        XCTAssertEqual(status.label, "Prepare notes - due tomorrow")
        XCTAssertEqual(status.kind, .task("tomorrow"))
    }

    func testAdaptiveDurationTextRoundsMinuteBoundariesUp() {
        let start = Date(timeIntervalSince1970: 0)

        XCTAssertEqual(MenuBarAdaptiveStatusResolver.durationText(from: start, to: start.addingTimeInterval(1)), "1m")
        XCTAssertEqual(MenuBarAdaptiveStatusResolver.durationText(from: start, to: start.addingTimeInterval(60)), "1m")
        XCTAssertEqual(MenuBarAdaptiveStatusResolver.durationText(from: start, to: start.addingTimeInterval(61)), "2m")
        XCTAssertEqual(MenuBarAdaptiveStatusResolver.durationText(from: start, to: start.addingTimeInterval(60 * 60)), "1h")
        XCTAssertEqual(MenuBarAdaptiveStatusResolver.durationText(from: start, to: start.addingTimeInterval(60 * 60 + 1)), "1h 1m")
    }

    func testNavigationSurfacePlacementMetadataAndUnknownDecodeFallback() throws {
        XCTAssertEqual(NavigationSurfacePlacement.allCases.map(\.rawValue), ["left", "right", "top", "bottom"])
        XCTAssertEqual(NavigationSurfacePlacement.right.title, "Right")
        XCTAssertEqual(NavigationSurfacePlacement.top.systemImage, "rectangle.topthird.inset.filled")
        XCTAssertTrue(NavigationSurfacePlacement.bottom.isHorizontal)

        let data = Data(#""floating""#.utf8)
        let decoded = try JSONDecoder().decode(NavigationSurfacePlacement.self, from: data)

        XCTAssertEqual(decoded, .left)
    }

    @MainActor
    func testSidebarPlacementSetterUpdatesSettings() {
        let model = AppModel.testHostBootstrap()

        XCTAssertEqual(model.settings.sidebarPlacement, .left)

        model.setSidebarPlacement(.right)

        XCTAssertEqual(model.settings.sidebarPlacement, .right)
    }

    @MainActor
    func testSidebarItemHidingKeepsAtLeastOneVisibleTab() {
        let model = AppModel.testHostBootstrap()

        model.setSidebarItemHidden(.calendar, hidden: true)
        model.setSidebarItemHidden(.store, hidden: true)
        model.setSidebarItemHidden(.notes, hidden: true)

        XCTAssertEqual(model.settings.hiddenSidebarItems, Set(["calendar", "store"]))
    }

    @MainActor
    func testShortcutConflictStateNamesConflictingCommands() {
        let proposed = HCBShortcutCommand.refresh.defaultBinding
        let conflicts = hcbConflictingCommands(
            proposed: proposed,
            for: .newTask,
            overrides: [:]
        )
        let state = HCBShortcutConflictState(
            proposedBinding: proposed,
            targetCommand: .newTask,
            conflictingCommands: conflicts
        )

        XCTAssertEqual(conflicts, [.refresh])
        XCTAssertTrue(state.message.contains("Refresh Sync"))
        XCTAssertTrue(state.message.contains(proposed.displayLabel))
    }

    @MainActor
    func testCalendarPolishShortcutsExposeMacDefaultsWithoutConflicts() {
        XCTAssertEqual(HCBShortcutCommand.calendarShareAvailability.group, .calendar)
        XCTAssertEqual(HCBShortcutCommand.calendarShareAvailability.title, "Share Availability…")
        XCTAssertEqual(HCBShortcutCommand.calendarShareAvailability.defaultBinding.displayLabel, "⇧⌘A")

        XCTAssertEqual(HCBShortcutCommand.calendarViewMultiDay.group, .calendar)
        XCTAssertEqual(HCBShortcutCommand.calendarViewMultiDay.title, "Switch to Multi-Day View")
        XCTAssertEqual(HCBShortcutCommand.calendarViewMultiDay.defaultBinding.displayLabel, "⌥⌘5")

        XCTAssertEqual(HCBShortcutCommand.calendarViewYear.group, .calendar)
        XCTAssertEqual(HCBShortcutCommand.calendarViewYear.title, "Switch to Year View")
        XCTAssertEqual(HCBShortcutCommand.calendarViewYear.defaultBinding.displayLabel, "⌥⌘6")

        for command in [
            HCBShortcutCommand.calendarShareAvailability,
            .calendarViewMultiDay,
            .calendarViewYear
        ] {
            XCTAssertTrue(
                hcbConflictingCommands(
                    proposed: command.defaultBinding,
                    for: command,
                    overrides: [:]
                ).isEmpty,
                "\(command.title) should not reuse an existing default shortcut."
            )
        }
    }

    func testDisconnectImpactWarnsWithoutClaimingGoogleDeletion() {
        let summary = DisconnectImpactSummary(
            accountName: "Ada Lovelace",
            cacheFootprint: "42 KB",
            pendingMutationCount: 2,
            conflictedMutationCount: 1,
            quarantinedMutationCount: 1,
            invalidPayloadMutationCount: 1
        )

        let message = summary.confirmationMessage

        XCTAssertTrue(message.contains("Ada Lovelace"))
        XCTAssertTrue(message.contains("removes the saved Google session from Keychain"))
        XCTAssertTrue(message.contains("Google Tasks and Calendar data in your Google account will not be deleted"))
        XCTAssertTrue(message.contains("Local cache retained on this Mac: 42 KB"))
        XCTAssertTrue(message.contains("Pending sync work: 2 queued local writes"))
        XCTAssertTrue(message.contains("Needs attention: 1 conflict, 1 quarantined, 1 invalid"))
        XCTAssertTrue(message.contains("cannot reach Google while disconnected"))
    }

    func testDisconnectImpactUsesSingularQueuedWriteCopy() {
        let summary = DisconnectImpactSummary(
            accountName: "this Google account",
            cacheFootprint: "No local cache file found",
            pendingMutationCount: 1,
            conflictedMutationCount: 0,
            quarantinedMutationCount: 0,
            invalidPayloadMutationCount: 0
        )

        XCTAssertTrue(summary.confirmationMessage.contains("Pending sync work: 1 queued local write"))
        XCTAssertFalse(summary.confirmationMessage.contains("Needs attention:"))
    }

    func testAccountDisconnectImpactResolverUsesInactiveWorkspaceCounts() {
        let active = GoogleAccount.preview
        let work = GoogleAccount(
            id: "work-account",
            email: "work@example.com",
            displayName: "Work",
            grantedScopes: [GoogleScope.tasks],
            authProvider: .customDesktopOAuth
        )
        let activeMutation = pendingMutation(id: "00000000-0000-0000-0000-000000000001", accountID: active.id)
        let workConflict = pendingMutation(
            id: "00000000-0000-0000-0000-000000000002",
            accountID: work.id,
            quarantinedAt: Date(timeIntervalSince1970: 10),
            conflictedAt: Date(timeIntervalSince1970: 10)
        )
        let workInvalid = pendingMutation(
            id: "00000000-0000-0000-0000-000000000003",
            accountID: work.id,
            lastErrorSummary: "Invalid payload - rejected by Google",
            quarantinedAt: Date(timeIntervalSince1970: 20)
        )
        let resolver = AccountDisconnectImpactResolver(
            activeAccountID: active.id,
            activePendingMutations: [activeMutation],
            accountWorkspaces: [
                AccountWorkspaceSnapshot(
                    accountID: work.id,
                    taskLists: [],
                    tasks: [],
                    calendars: [],
                    events: [],
                    settings: .default,
                    syncCheckpoints: [],
                    pendingMutations: [workConflict, workInvalid]
                )
            ]
        )

        let activeSummary = resolver.summary(for: active.id, accountName: "Personal", cacheFootprint: "1 KB")
        let inactiveSummary = resolver.summary(for: work.id, accountName: "Work", cacheFootprint: "1 KB")

        XCTAssertEqual(activeSummary.pendingMutationCount, 1)
        XCTAssertEqual(activeSummary.conflictedMutationCount, 0)
        XCTAssertEqual(inactiveSummary.pendingMutationCount, 2)
        XCTAssertEqual(inactiveSummary.conflictedMutationCount, 1)
        XCTAssertEqual(inactiveSummary.quarantinedMutationCount, 2)
        XCTAssertEqual(inactiveSummary.invalidPayloadMutationCount, 1)
        XCTAssertTrue(resolver.cacheInvalidationKey.contains("work-account:2:1:2:1"))
    }

    private func pendingMutation(
        id: String,
        accountID: GoogleAccount.ID,
        lastErrorSummary: String? = nil,
        quarantinedAt: Date? = nil,
        conflictedAt: Date? = nil
    ) -> PendingMutation {
        PendingMutation(
            id: UUID(uuidString: id)!,
            accountID: accountID,
            createdAt: Date(timeIntervalSince1970: 1),
            resourceType: .task,
            resourceID: id,
            action: .update,
            payload: Data(),
            lastErrorSummary: lastErrorSummary,
            quarantinedAt: quarantinedAt,
            conflictedAt: conflictedAt
        )
    }

    private func adaptiveCalendar() -> Calendar {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(secondsFromGMT: 0)!
        return calendar
    }

    private func adaptiveDate(
        day: Int = 10,
        hour: Int,
        minute: Int = 0,
        calendar: Calendar
    ) -> Date {
        var components = DateComponents()
        components.calendar = calendar
        components.timeZone = calendar.timeZone
        components.year = 2026
        components.month = 5
        components.day = day
        components.hour = hour
        components.minute = minute
        return calendar.date(from: components)!
    }

    private func adaptiveEvent(
        id: String,
        summary: String,
        start: Date,
        end: Date,
        isAllDay: Bool = false,
        status: CalendarEventStatus = .confirmed
    ) -> CalendarEventMirror {
        CalendarEventMirror(
            id: id,
            calendarID: "primary",
            summary: summary,
            details: "",
            startDate: start,
            endDate: end,
            isAllDay: isAllDay,
            status: status,
            recurrence: [],
            etag: nil,
            updatedAt: nil
        )
    }

    private func adaptiveTask(
        id: String,
        title: String,
        due: Date?,
        status: TaskStatus = .needsAction,
        isDeleted: Bool = false,
        isHidden: Bool = false
    ) -> TaskMirror {
        TaskMirror(
            id: id,
            taskListID: "tasks",
            parentID: nil,
            title: title,
            notes: "",
            status: status,
            dueDate: due,
            completedAt: nil,
            isDeleted: isDeleted,
            isHidden: isHidden,
            position: nil,
            etag: nil,
            updatedAt: nil
        )
    }
}
