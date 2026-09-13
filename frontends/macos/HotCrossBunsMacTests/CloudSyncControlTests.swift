import XCTest
@testable import HotCrossBunsMac

final class CloudSyncControlTests: XCTestCase {
    override func tearDown() {
        MockURLProtocol.reset()
        super.tearDown()
    }

    func testLegacySettingsDefaultToSyncingTasksAndEvents() throws {
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

        XCTAssertEqual(settings.cloudSyncTargets, CloudSyncTarget.all)
        XCTAssertEqual(settings.completedTaskRetentionDaysBack, 365)
    }

    func testCloudSyncTargetsRoundTripTaskOnlyMode() throws {
        var settings = AppSettings.default
        settings.cloudSyncTargets = [.tasks]

        let data = try JSONEncoder().encode(settings)
        let decoded = try JSONDecoder().decode(AppSettings.self, from: data)

        XCTAssertEqual(decoded.cloudSyncTargets, [.tasks])
        XCTAssertTrue(decoded.cloudSyncTargets.syncsTasks)
        XCTAssertFalse(decoded.cloudSyncTargets.syncsEvents)
    }

    func testCloudSyncTargetsMapToResourceTypes() {
        let tasksOnly: Set<CloudSyncTarget> = [.tasks]
        let eventsOnly: Set<CloudSyncTarget> = [.events]

        XCTAssertTrue(tasksOnly.allows(.task))
        XCTAssertTrue(tasksOnly.allows(.taskList))
        XCTAssertFalse(tasksOnly.allows(.event))
        XCTAssertFalse(tasksOnly.allows(.calendar))

        XCTAssertFalse(eventsOnly.allows(.task))
        XCTAssertFalse(eventsOnly.allows(.taskList))
        XCTAssertTrue(eventsOnly.allows(.event))
        XCTAssertTrue(eventsOnly.allows(.calendar))
    }

    func testSchedulerDoesNotTouchNetworkWhenAllCloudTargetsAreDisabled() async throws {
        var settings = AppSettings.default
        settings.cloudSyncTargets = []
        let state = baseState(settings: settings)
        MockURLProtocol.requestHandler = { request in
            XCTFail("No request expected, got \(request.url?.absoluteString ?? "<nil>")")
            return Self.jsonResponse(for: request, body: #"{}"#)
        }

        let synced = try await makeScheduler().syncNow(mode: .balanced, baseState: state)

        XCTAssertTrue(MockURLProtocol.capturedRequests.isEmpty)
        XCTAssertEqual(synced.taskLists, state.taskLists)
        XCTAssertEqual(synced.tasks, state.tasks)
        XCTAssertEqual(synced.calendars, state.calendars)
        XCTAssertEqual(synced.events, state.events)
        XCTAssertEqual(synced.settings.syncMode, .balanced)
    }

    func testSchedulerPreservesNonActiveAccountMetadata() async throws {
        var settings = AppSettings.default
        settings.cloudSyncTargets = []
        let workAccount = GoogleAccount(
            id: "work-account",
            email: "work@example.com",
            displayName: "Work",
            grantedScopes: [GoogleScope.tasks],
            authProvider: .customDesktopOAuth
        )
        var state = baseState(settings: settings)
        state.accounts = [GoogleAccount.preview, workAccount]
        state.activeAccountID = GoogleAccount.preview.id
        state.syncCheckpoints = [
            SyncCheckpoint(
                id: SyncCheckpoint.stableID(accountID: GoogleAccount.preview.id, resourceType: .taskList, resourceID: "list"),
                accountID: GoogleAccount.preview.id,
                resourceType: .taskList,
                resourceID: "list",
                calendarSyncToken: nil,
                tasksUpdatedMin: Date(),
                lastSuccessfulSyncAt: Date()
            ),
            SyncCheckpoint(
                id: SyncCheckpoint.stableID(accountID: workAccount.id, resourceType: .taskList, resourceID: "work-list"),
                accountID: workAccount.id,
                resourceType: .taskList,
                resourceID: "work-list",
                calendarSyncToken: nil,
                tasksUpdatedMin: Date(),
                lastSuccessfulSyncAt: Date()
            )
        ]

        let synced = try await makeScheduler().syncNow(mode: .balanced, baseState: state)

        XCTAssertEqual(synced.accounts.map(\.id), [GoogleAccount.preview.id, workAccount.id])
        XCTAssertEqual(synced.activeAccountID, GoogleAccount.preview.id)
        XCTAssertEqual(Set(synced.syncCheckpoints.map(\.accountID)), [GoogleAccount.preview.id])
        let workWorkspace = try XCTUnwrap(synced.accountWorkspaces.first { $0.accountID == workAccount.id })
        XCTAssertEqual(Set(workWorkspace.syncCheckpoints.map(\.accountID)), [workAccount.id])
    }

    func testSchedulerUpdatesOnlyActiveWorkspaceAndPreservesInactiveWorkspaceData() async throws {
        var settings = AppSettings.default
        settings.cloudSyncTargets = CloudSyncTarget.all
        settings.selectedTaskListIDs = ["list"]
        settings.hasConfiguredTaskListSelection = true
        settings.selectedCalendarIDs = ["cal"]
        settings.hasConfiguredCalendarSelection = true
        let workAccount = GoogleAccount(
            id: "work-account",
            email: "work@example.com",
            displayName: "Work",
            grantedScopes: [GoogleScope.tasks, GoogleScope.calendar],
            authProvider: .customDesktopOAuth
        )
        let workMutation = PendingMutation(
            id: UUID(uuidString: "00000000-0000-0000-0000-00000000AA01")!,
            accountID: workAccount.id,
            createdAt: Date(timeIntervalSince1970: 100),
            resourceType: .task,
            resourceID: "work-task",
            action: .update,
            payload: Data()
        )
        let workWorkspace = AccountWorkspaceSnapshot(
            accountID: workAccount.id,
            taskLists: [TaskListMirror(id: "work-list", title: "Work", updatedAt: nil, etag: nil)],
            tasks: [Self.task(id: "work-task", taskListID: "work-list", completedAt: nil, completed: false)],
            calendars: [
                CalendarListMirror(id: "work-cal", summary: "Work", colorHex: "#111111", isSelected: true, accessRole: "owner")
            ],
            events: [
                CalendarEventMirror(
                    id: "work-event",
                    calendarID: "work-cal",
                    summary: "Work event",
                    details: "",
                    startDate: Date(timeIntervalSince1970: 200),
                    endDate: Date(timeIntervalSince1970: 300),
                    isAllDay: false,
                    status: .confirmed,
                    recurrence: [],
                    etag: nil,
                    updatedAt: nil
                )
            ],
            settings: .default,
            syncCheckpoints: [
                SyncCheckpoint(
                    id: SyncCheckpoint.stableID(accountID: workAccount.id, resourceType: .taskList, resourceID: "work-list"),
                    accountID: workAccount.id,
                    resourceType: .taskList,
                    resourceID: "work-list",
                    calendarSyncToken: nil,
                    tasksUpdatedMin: Date(timeIntervalSince1970: 50),
                    lastSuccessfulSyncAt: Date(timeIntervalSince1970: 50)
                )
            ],
            pendingMutations: [workMutation]
        )
        var state = baseState(settings: settings)
        state.accounts = [GoogleAccount.preview, workAccount]
        state.accountWorkspaces = [workWorkspace]
        MockURLProtocol.requestHandler = { request in
            switch request.url?.path {
            case "/tasks/v1/users/@me/lists":
                return Self.jsonResponse(for: request, body: Self.taskListsJSON)
            case "/tasks/v1/lists/list/tasks":
                return Self.jsonResponse(for: request, body: Self.tasksJSON)
            case "/calendar/v3/users/me/calendarList":
                return Self.jsonResponse(for: request, body: Self.calendarListJSON)
            case "/calendar/v3/calendars/cal/events":
                return Self.jsonResponse(for: request, body: Self.eventsJSON)
            default:
                XCTFail("Unexpected path \(request.url?.path ?? "<nil>")")
                return Self.jsonResponse(for: request, body: #"{}"#, statusCode: 404)
            }
        }

        let synced = try await makeScheduler().syncNow(mode: .balanced, baseState: state)

        XCTAssertEqual(synced.taskLists.map(\.id), ["list"])
        XCTAssertEqual(synced.tasks.map(\.id), ["remote-task"])
        XCTAssertEqual(synced.events.map(\.id), ["remote-event"])
        XCTAssertEqual(synced.accountWorkspaces.first { $0.accountID == workAccount.id }, workWorkspace)
    }

    func testSchedulerLeavesTasksLocalWhenOnlyEventsSync() async throws {
        var settings = AppSettings.default
        settings.cloudSyncTargets = [.events]
        settings.selectedCalendarIDs = ["cal"]
        settings.hasConfiguredCalendarSelection = true
        let state = baseState(settings: settings)
        MockURLProtocol.requestHandler = { request in
            let path = try XCTUnwrap(request.url?.path)
            XCTAssertFalse(path.hasPrefix("/tasks/"), "Tasks endpoints must not be called when Tasks sync is disabled")
            if path == "/calendar/v3/users/me/calendarList" {
                return Self.jsonResponse(for: request, body: Self.calendarListJSON)
            }
            if path == "/calendar/v3/calendars/cal/events" {
                return Self.jsonResponse(for: request, body: Self.eventsJSON)
            }
            XCTFail("Unexpected path \(path)")
            return Self.jsonResponse(for: request, body: #"{}"#, statusCode: 404)
        }

        let synced = try await makeScheduler().syncNow(mode: .balanced, baseState: state)

        XCTAssertEqual(synced.taskLists, state.taskLists)
        XCTAssertEqual(synced.tasks, state.tasks)
        XCTAssertEqual(synced.events.map(\.id), ["remote-event"])
        XCTAssertTrue(MockURLProtocol.capturedRequests.allSatisfy { $0.url?.path.hasPrefix("/tasks/") == false })
    }

    func testSchedulerEventFullSyncMergePreservesPendingDropsCancelledSortsAndUpdatesCheckpoint() async throws {
        var settings = AppSettings.default
        settings.cloudSyncTargets = [.events]
        settings.selectedCalendarIDs = ["cal"]
        settings.hasConfiguredCalendarSelection = true
        var state = baseState(settings: settings)
        let formatter = ISO8601DateFormatter()
        let pendingStart = formatter.date(from: "2026-05-01T12:00:00Z")!
        let staleStart = formatter.date(from: "2026-05-01T08:00:00Z")!
        state.events = [
            CalendarEventMirror(
                id: "stale-event",
                calendarID: "cal",
                summary: "Stale event",
                details: "",
                startDate: staleStart,
                endDate: staleStart.addingTimeInterval(1800),
                isAllDay: false,
                status: .confirmed,
                recurrence: [],
                etag: "stale-etag",
                updatedAt: staleStart
            ),
            CalendarEventMirror(
                id: "local-pending-event",
                calendarID: "cal",
                summary: "Pending event",
                details: "",
                startDate: pendingStart,
                endDate: pendingStart.addingTimeInterval(3600),
                isAllDay: false,
                status: .confirmed,
                recurrence: [],
                etag: nil,
                updatedAt: pendingStart
            )
        ]

        MockURLProtocol.requestHandler = { request in
            let path = try XCTUnwrap(request.url?.path)
            XCTAssertFalse(path.hasPrefix("/tasks/"), "Tasks endpoints must not be called when Events sync is disabled")
            if path == "/calendar/v3/users/me/calendarList" {
                return Self.jsonResponse(for: request, body: Self.calendarListJSON)
            }
            if path == "/calendar/v3/calendars/cal/events" {
                return Self.jsonResponse(for: request, body: Self.unsortedEventsWithCancelledJSON)
            }
            XCTFail("Unexpected path \(path)")
            return Self.jsonResponse(for: request, body: #"{}"#, statusCode: 404)
        }

        let synced = try await makeScheduler().syncNow(mode: .balanced, baseState: state)

        XCTAssertEqual(synced.events.map(\.id), ["early-all-day", "local-pending-event", "late-recurring"])
        XCTAssertFalse(synced.events.contains { $0.id == "stale-event" })
        XCTAssertFalse(synced.events.contains { $0.id == "cancelled-event" })
        XCTAssertTrue(synced.events.allSatisfy { $0.status != .cancelled })
        let lateEvent = try XCTUnwrap(synced.events.first { $0.id == "late-recurring" })
        XCTAssertEqual(lateEvent.recurrence, ["RRULE:FREQ=WEEKLY;COUNT=2"])
        XCTAssertEqual(lateEvent.endDate, formatter.date(from: "2026-05-03T12:30:00Z"))
        var eventCheckpoint: SyncCheckpoint?
        for checkpoint in synced.syncCheckpoints {
            guard checkpoint.resourceType == .calendar,
                  checkpoint.resourceID == "cal"
            else { continue }
            eventCheckpoint = checkpoint
            break
        }
        let checkpoint = try XCTUnwrap(eventCheckpoint)
        XCTAssertEqual(checkpoint.calendarSyncToken, "edge-token")
    }

    func testSchedulerSkipsSingleMissingCalendarAndKeepsSyncingEvents() async throws {
        var settings = AppSettings.default
        settings.cloudSyncTargets = [.events]
        settings.selectedCalendarIDs = ["cal", "missing-cal"]
        settings.hasConfiguredCalendarSelection = true
        let state = baseState(settings: settings)
        MockURLProtocol.requestHandler = { request in
            let path = try XCTUnwrap(request.url?.path)
            XCTAssertFalse(path.hasPrefix("/tasks/"), "Tasks endpoints must not be called when Events sync is disabled")
            if path == "/calendar/v3/users/me/calendarList" {
                return Self.jsonResponse(for: request, body: Self.calendarListWithMissingJSON)
            }
            if path == "/calendar/v3/calendars/cal/events" {
                return Self.jsonResponse(for: request, body: Self.eventsJSON)
            }
            if path == "/calendar/v3/calendars/missing-cal/events" {
                return Self.jsonResponse(for: request, body: Self.notFoundJSON, statusCode: 404)
            }
            XCTFail("Unexpected path \(path)")
            return Self.jsonResponse(for: request, body: #"{}"#, statusCode: 404)
        }

        let synced = try await makeScheduler().syncNow(mode: .balanced, baseState: state)

        XCTAssertEqual(synced.events.map(\.id), ["remote-event"])
        XCTAssertEqual(Set(synced.calendars.map(\.id)), ["cal", "missing-cal"])
    }

    func testSchedulerPrunesSelectedCalendarsMissingFromCalendarList() async throws {
        var settings = AppSettings.default
        settings.cloudSyncTargets = [.events]
        settings.selectedCalendarIDs = ["cal", "stale-cal"]
        settings.hasConfiguredCalendarSelection = true
        let state = baseState(settings: settings)
        MockURLProtocol.requestHandler = { request in
            let path = try XCTUnwrap(request.url?.path)
            XCTAssertFalse(path.hasPrefix("/tasks/"), "Tasks endpoints must not be called when Events sync is disabled")
            if path == "/calendar/v3/users/me/calendarList" {
                return Self.jsonResponse(for: request, body: Self.calendarListJSON)
            }
            if path == "/calendar/v3/calendars/cal/events" {
                return Self.jsonResponse(for: request, body: Self.eventsJSON)
            }
            XCTFail("Unexpected path \(path)")
            return Self.jsonResponse(for: request, body: #"{}"#, statusCode: 404)
        }

        let synced = try await makeScheduler().syncNow(mode: .balanced, baseState: state)

        XCTAssertEqual(synced.events.map(\.id), ["remote-event"])
        XCTAssertEqual(synced.settings.selectedCalendarIDs, ["cal"])
        XCTAssertFalse(MockURLProtocol.capturedRequests.contains { $0.url?.path == "/calendar/v3/calendars/stale-cal/events" })
    }

    func testSchedulerLeavesEventsLocalWhenOnlyTasksSync() async throws {
        var settings = AppSettings.default
        settings.cloudSyncTargets = [.tasks]
        settings.selectedTaskListIDs = ["list"]
        settings.hasConfiguredTaskListSelection = true
        let state = baseState(settings: settings)
        MockURLProtocol.requestHandler = { request in
            let path = try XCTUnwrap(request.url?.path)
            XCTAssertFalse(path.hasPrefix("/calendar/"), "Calendar endpoints must not be called when Events sync is disabled")
            if path == "/tasks/v1/users/@me/lists" {
                return Self.jsonResponse(for: request, body: Self.taskListsJSON)
            }
            if path == "/tasks/v1/lists/list/tasks" {
                return Self.jsonResponse(for: request, body: Self.tasksJSON)
            }
            XCTFail("Unexpected path \(path)")
            return Self.jsonResponse(for: request, body: #"{}"#, statusCode: 404)
        }

        let synced = try await makeScheduler().syncNow(mode: .balanced, baseState: state)

        XCTAssertEqual(synced.calendars, state.calendars)
        XCTAssertEqual(synced.events, state.events)
        XCTAssertEqual(synced.tasks.map(\.id), ["remote-task"])
        XCTAssertTrue(MockURLProtocol.capturedRequests.allSatisfy { $0.url?.path.hasPrefix("/calendar/") == false })
    }

    func testSchedulerFollowsGoogleSelectedCalendarsUntilUserConfiguresSelection() async throws {
        var settings = AppSettings.default
        settings.cloudSyncTargets = [.events]
        settings.hasConfiguredCalendarSelection = false
        let state = baseState(settings: settings)
        MockURLProtocol.requestHandler = { request in
            let path = try XCTUnwrap(request.url?.path)
            if path == "/calendar/v3/users/me/calendarList" {
                return Self.jsonResponse(for: request, body: Self.calendarListWithHolidayJSON)
            }
            if path == "/calendar/v3/calendars/cal/events" || path == "/calendar/v3/calendars/sg-holidays/events" {
                return Self.jsonResponse(for: request, body: Self.eventsJSON)
            }
            XCTFail("Unexpected path \(path)")
            return Self.jsonResponse(for: request, body: #"{}"#, statusCode: 404)
        }

        let synced = try await makeScheduler().syncNow(mode: .balanced, baseState: state)

        XCTAssertFalse(synced.settings.hasConfiguredCalendarSelection)
        XCTAssertEqual(synced.settings.selectedCalendarIDs, ["cal", "sg-holidays"])
        XCTAssertTrue(synced.calendars.first(where: { $0.id == "sg-holidays" })?.isSelected == true)
        XCTAssertFalse(MockURLProtocol.capturedRequests.contains { $0.url?.path == "/calendar/v3/calendars/hidden/events" })
    }

    func testSchedulerAutoSelectsNewGoogleSelectedCalendarsWhenSelectionWasConfigured() async throws {
        var settings = AppSettings.default
        settings.cloudSyncTargets = [.events]
        settings.selectedCalendarIDs = ["cal"]
        settings.hasConfiguredCalendarSelection = true
        let state = baseState(settings: settings)
        MockURLProtocol.requestHandler = { request in
            let path = try XCTUnwrap(request.url?.path)
            if path == "/calendar/v3/users/me/calendarList" {
                return Self.jsonResponse(for: request, body: Self.calendarListWithHolidayJSON)
            }
            if path == "/calendar/v3/calendars/cal/events" || path == "/calendar/v3/calendars/sg-holidays/events" {
                return Self.jsonResponse(for: request, body: Self.eventsJSON)
            }
            XCTFail("Unexpected path \(path)")
            return Self.jsonResponse(for: request, body: #"{}"#, statusCode: 404)
        }

        let synced = try await makeScheduler().syncNow(mode: .balanced, baseState: state)

        XCTAssertTrue(synced.settings.hasConfiguredCalendarSelection)
        XCTAssertEqual(synced.settings.selectedCalendarIDs, ["cal", "sg-holidays"])
        XCTAssertTrue(synced.calendars.first(where: { $0.id == "sg-holidays" })?.isSelected == true)
        XCTAssertFalse(MockURLProtocol.capturedRequests.contains { $0.url?.path == "/calendar/v3/calendars/hidden/events" })
    }

    func testSchedulerKeepsPreviouslyHiddenCalendarHiddenWhenGoogleSelectsIt() async throws {
        var settings = AppSettings.default
        settings.cloudSyncTargets = [.events]
        settings.selectedCalendarIDs = ["cal"]
        settings.hasConfiguredCalendarSelection = true
        var state = baseState(settings: settings)
        state.calendars.append(CalendarListMirror(
            id: "sg-holidays",
            summary: "Singapore Holidays",
            colorHex: "#0b8043",
            isSelected: false,
            accessRole: "reader",
            etag: "holiday-etag"
        ))
        MockURLProtocol.requestHandler = { request in
            let path = try XCTUnwrap(request.url?.path)
            if path == "/calendar/v3/users/me/calendarList" {
                return Self.jsonResponse(for: request, body: Self.calendarListWithHolidayJSON)
            }
            if path == "/calendar/v3/calendars/cal/events" {
                return Self.jsonResponse(for: request, body: Self.eventsJSON)
            }
            XCTFail("Unexpected path \(path)")
            return Self.jsonResponse(for: request, body: #"{}"#, statusCode: 404)
        }

        let synced = try await makeScheduler().syncNow(mode: .balanced, baseState: state)

        XCTAssertTrue(synced.settings.hasConfiguredCalendarSelection)
        XCTAssertEqual(synced.settings.selectedCalendarIDs, ["cal"])
        XCTAssertFalse(synced.calendars.first(where: { $0.id == "sg-holidays" })?.isSelected == true)
        XCTAssertFalse(MockURLProtocol.capturedRequests.contains { $0.url?.path == "/calendar/v3/calendars/sg-holidays/events" })
    }

    func testSchedulerUsesRetentionWindowsForFullSync() async throws {
        var settings = AppSettings.default
        settings.cloudSyncTargets = CloudSyncTarget.all
        settings.eventRetentionDaysBack = 365
        settings.completedTaskRetentionDaysBack = 180
        settings.selectedTaskListIDs = ["list"]
        settings.hasConfiguredTaskListSelection = true
        settings.selectedCalendarIDs = ["cal"]
        settings.hasConfiguredCalendarSelection = true
        let state = baseState(settings: settings)
        MockURLProtocol.requestHandler = { request in
            let path = try XCTUnwrap(request.url?.path)
            switch path {
            case "/tasks/v1/users/@me/lists":
                return Self.jsonResponse(for: request, body: Self.taskListsJSON)
            case "/tasks/v1/lists/list/tasks":
                let query = Self.query(for: request)
                XCTAssertNil(query["updatedMin"])
                XCTAssertNotNil(query["completedMin"])
                return Self.jsonResponse(for: request, body: Self.tasksJSON)
            case "/calendar/v3/users/me/calendarList":
                return Self.jsonResponse(for: request, body: Self.calendarListJSON)
            case "/calendar/v3/calendars/cal/events":
                let query = Self.query(for: request)
                XCTAssertNil(query["syncToken"])
                XCTAssertNotNil(query["timeMin"])
                return Self.jsonResponse(for: request, body: Self.eventsJSON)
            default:
                XCTFail("Unexpected path \(path)")
                return Self.jsonResponse(for: request, body: #"{}"#, statusCode: 404)
            }
        }

        _ = try await makeScheduler().syncNow(mode: .balanced, baseState: state)
    }

    func testSchedulerOmitsRetentionWindowsForForeverFullSync() async throws {
        var settings = AppSettings.default
        settings.cloudSyncTargets = CloudSyncTarget.all
        settings.eventRetentionDaysBack = 0
        settings.completedTaskRetentionDaysBack = 0
        settings.selectedTaskListIDs = ["list"]
        settings.hasConfiguredTaskListSelection = true
        settings.selectedCalendarIDs = ["cal"]
        settings.hasConfiguredCalendarSelection = true
        let state = baseState(settings: settings)
        MockURLProtocol.requestHandler = { request in
            let path = try XCTUnwrap(request.url?.path)
            switch path {
            case "/tasks/v1/users/@me/lists":
                return Self.jsonResponse(for: request, body: Self.taskListsJSON)
            case "/tasks/v1/lists/list/tasks":
                let query = Self.query(for: request)
                XCTAssertNil(query["updatedMin"])
                XCTAssertNil(query["completedMin"])
                return Self.jsonResponse(for: request, body: Self.tasksJSON)
            case "/calendar/v3/users/me/calendarList":
                return Self.jsonResponse(for: request, body: Self.calendarListJSON)
            case "/calendar/v3/calendars/cal/events":
                let query = Self.query(for: request)
                XCTAssertNil(query["syncToken"])
                XCTAssertNil(query["timeMin"])
                return Self.jsonResponse(for: request, body: Self.eventsJSON)
            default:
                XCTFail("Unexpected path \(path)")
                return Self.jsonResponse(for: request, body: #"{}"#, statusCode: 404)
            }
        }

        _ = try await makeScheduler().syncNow(mode: .balanced, baseState: state)
    }

    func testSchedulerPrunesOldCompletedTasksDuringIncrementalSync() async throws {
        var settings = AppSettings.default
        settings.cloudSyncTargets = [.tasks]
        settings.completedTaskRetentionDaysBack = 30
        settings.selectedTaskListIDs = ["list"]
        settings.hasConfiguredTaskListSelection = true
        var state = baseState(settings: settings)
        state.tasks = [
            Self.task(id: "old-completed", completedAt: Date().addingTimeInterval(-60 * 24 * 60 * 60), completed: true),
            Self.task(id: "recent-completed", completedAt: Date().addingTimeInterval(-5 * 24 * 60 * 60), completed: true),
            Self.task(id: "old-open", completedAt: nil, completed: false)
        ]
        state.syncCheckpoints = [
            SyncCheckpoint(
                id: SyncCheckpoint.stableID(accountID: GoogleAccount.preview.id, resourceType: .taskList, resourceID: "list"),
                accountID: GoogleAccount.preview.id,
                resourceType: .taskList,
                resourceID: "list",
                calendarSyncToken: nil,
                tasksUpdatedMin: Date().addingTimeInterval(-60),
                lastSuccessfulSyncAt: Date().addingTimeInterval(-60)
            )
        ]
        MockURLProtocol.requestHandler = { request in
            let path = try XCTUnwrap(request.url?.path)
            if path == "/tasks/v1/users/@me/lists" {
                return Self.jsonResponse(for: request, body: Self.taskListsJSON)
            }
            if path == "/tasks/v1/lists/list/tasks" {
                let query = Self.query(for: request)
                XCTAssertNotNil(query["updatedMin"])
                XCTAssertNil(query["completedMin"])
                return Self.jsonResponse(for: request, body: #"{"items":[]}"#)
            }
            XCTFail("Unexpected path \(path)")
            return Self.jsonResponse(for: request, body: #"{}"#, statusCode: 404)
        }

        let synced = try await makeScheduler().syncNow(mode: .balanced, baseState: state)

        XCTAssertEqual(Set(synced.tasks.map(\.id)), ["recent-completed", "old-open"])
    }

    func testSyncChangeSetReportsNarrowChangedEventAndTask() async throws {
        var settings = AppSettings.default
        settings.cloudSyncTargets = CloudSyncTarget.all
        settings.selectedTaskListIDs = ["list"]
        settings.hasConfiguredTaskListSelection = true
        settings.selectedCalendarIDs = ["cal"]
        settings.hasConfiguredCalendarSelection = true
        var state = baseState(settings: settings)
        state.syncCheckpoints = [
            SyncCheckpoint(
                id: SyncCheckpoint.stableID(accountID: GoogleAccount.preview.id, resourceType: .taskList, resourceID: "list"),
                accountID: GoogleAccount.preview.id,
                resourceType: .taskList,
                resourceID: "list",
                calendarSyncToken: nil,
                tasksUpdatedMin: Date(timeIntervalSince1970: 1_714_000_000),
                lastSuccessfulSyncAt: Date(timeIntervalSince1970: 1_714_000_000)
            ),
            SyncCheckpoint(
                id: SyncCheckpoint.stableID(accountID: GoogleAccount.preview.id, resourceType: .calendar, resourceID: "cal"),
                accountID: GoogleAccount.preview.id,
                resourceType: .calendar,
                resourceID: "cal",
                calendarSyncToken: "prev-token",
                tasksUpdatedMin: nil,
                lastSuccessfulSyncAt: Date(timeIntervalSince1970: 1_714_000_000)
            )
        ]

        MockURLProtocol.requestHandler = { request in
            switch request.url?.path {
            case "/tasks/v1/users/@me/lists":
                return Self.jsonResponse(for: request, body: Self.taskListsJSON)
            case "/tasks/v1/lists/list/tasks":
                let query = Self.query(for: request)
                XCTAssertNotNil(query["updatedMin"])
                return Self.jsonResponse(for: request, body: Self.changedCachedTaskJSON)
            case "/calendar/v3/users/me/calendarList":
                return Self.jsonResponse(for: request, body: Self.calendarListJSON)
            case "/calendar/v3/calendars/cal/events":
                let query = Self.query(for: request)
                XCTAssertEqual(query["syncToken"], "prev-token")
                return Self.jsonResponse(for: request, body: Self.changedCachedEventJSON)
            default:
                XCTFail("Unexpected path \(request.url?.path ?? "<nil>")")
                return Self.jsonResponse(for: request, body: #"{}"#, statusCode: 404)
            }
        }

        let result = try await makeScheduler().syncNowWithChangeSet(mode: .balanced, baseState: state)
        let changeSet = result.changeSet

        XCTAssertEqual(changeSet.taskLists.unchanged, ["list"])
        XCTAssertEqual(changeSet.calendars.unchanged, ["cal"])
        XCTAssertEqual(changeSet.tasks.updated, ["cached-task"])
        XCTAssertEqual(changeSet.events.updated, ["cached-event"])
        XCTAssertTrue(changeSet.tasks.inserted.isEmpty)
        XCTAssertTrue(changeSet.events.deleted.isEmpty)
        XCTAssertEqual(changeSet.affectedTaskListIDs, ["list"])
        XCTAssertEqual(changeSet.affectedCalendarIDs, ["cal"])
        XCTAssertTrue(changeSet.checkpointChanged)
        XCTAssertEqual(result.state.tasks.first { $0.id == "cached-task" }?.title, "Changed task")
        XCTAssertEqual(result.state.events.first { $0.id == "cached-event" }?.summary, "Changed event")

        let eventDay = Self.dayKey(state.events[0].startDate)
        XCTAssertTrue(changeSet.affectedDayKeys.contains(eventDay))
    }

    func testSyncChangeSetReportsDeletedMovedAndMultiDayEventDays() async throws {
        var settings = AppSettings.default
        settings.cloudSyncTargets = [.events]
        settings.selectedCalendarIDs = ["cal"]
        settings.hasConfiguredCalendarSelection = true
        var state = baseState(settings: settings)
        state.events = [
            CalendarEventMirror(
                id: "moved-event",
                calendarID: "cal",
                summary: "Moved event",
                details: "",
                startDate: Self.date("2026-05-01T10:00:00Z"),
                endDate: Self.date("2026-05-01T11:00:00Z"),
                isAllDay: false,
                status: .confirmed,
                recurrence: [],
                etag: "move-v1",
                updatedAt: Self.date("2026-05-01T00:00:00Z")
            ),
            CalendarEventMirror(
                id: "multi-day",
                calendarID: "cal",
                summary: "Multi day",
                details: "",
                startDate: Self.date("2026-05-10T00:00:00Z"),
                endDate: Self.date("2026-05-13T00:00:00Z"),
                isAllDay: true,
                status: .confirmed,
                recurrence: [],
                etag: "multi-v1",
                updatedAt: Self.date("2026-05-09T00:00:00Z")
            )
        ]
        state.syncCheckpoints = [
            SyncCheckpoint(
                id: SyncCheckpoint.stableID(accountID: GoogleAccount.preview.id, resourceType: .calendar, resourceID: "cal"),
                accountID: GoogleAccount.preview.id,
                resourceType: .calendar,
                resourceID: "cal",
                calendarSyncToken: "prev-token",
                tasksUpdatedMin: nil,
                lastSuccessfulSyncAt: Date(timeIntervalSince1970: 1_714_000_000)
            )
        ]

        MockURLProtocol.requestHandler = { request in
            switch request.url?.path {
            case "/calendar/v3/users/me/calendarList":
                return Self.jsonResponse(for: request, body: Self.calendarListJSON)
            case "/calendar/v3/calendars/cal/events":
                return Self.jsonResponse(for: request, body: Self.movedAndDeletedEventsJSON)
            default:
                XCTFail("Unexpected path \(request.url?.path ?? "<nil>")")
                return Self.jsonResponse(for: request, body: #"{}"#, statusCode: 404)
            }
        }

        let result = try await makeScheduler().syncNowWithChangeSet(mode: .balanced, baseState: state)
        let changeSet = result.changeSet

        XCTAssertEqual(changeSet.events.updated, ["moved-event"])
        XCTAssertEqual(changeSet.events.deleted, ["multi-day"])
        XCTAssertEqual(result.state.events.map(\.id), ["moved-event"])
        XCTAssertTrue(changeSet.affectedDayKeys.contains(Self.dayKey(Self.date("2026-05-01T12:00:00Z"))))
        XCTAssertTrue(changeSet.affectedDayKeys.contains(Self.dayKey(Self.date("2026-05-03T12:00:00Z"))))
        XCTAssertTrue(changeSet.affectedDayKeys.contains(Self.dayKey(Self.date("2026-05-10T12:00:00Z"))))
        XCTAssertTrue(changeSet.affectedDayKeys.contains(Self.dayKey(Self.date("2026-05-11T12:00:00Z"))))
        XCTAssertTrue(changeSet.affectedDayKeys.contains(Self.dayKey(Self.date("2026-05-12T12:00:00Z"))))
        XCTAssertFalse(changeSet.affectedDayKeys.contains(Self.dayKey(Self.date("2026-05-13T12:00:00Z"))))
    }

    private func makeScheduler() -> SyncScheduler {
        let transport = GoogleAPITransport(
            baseURL: URL(string: "https://example.test")!,
            tokenProvider: StaticAccessTokenProvider(token: "test-token"),
            urlSession: MockURLProtocol.testSession()
        )
        let tasksClient = GoogleTasksClient(transport: transport)
        let calendarClient = GoogleCalendarClient(transport: transport)
        return SyncScheduler(tasksClient: tasksClient, calendarClient: calendarClient)
    }

    private func baseState(settings: AppSettings) -> CachedAppState {
        let now = ISO8601DateFormatter().date(from: "2026-04-30T02:00:00Z")!
        return CachedAppState(
            account: .preview,
            taskLists: [TaskListMirror(id: "list", title: "Inbox", updatedAt: now, etag: "list-etag")],
            tasks: [
                TaskMirror(
                    id: "cached-task",
                    taskListID: "list",
                    parentID: nil,
                    title: "Local task",
                    notes: "",
                    status: .needsAction,
                    dueDate: now,
                    completedAt: nil,
                    isDeleted: false,
                    isHidden: false,
                    position: nil,
                    etag: "local-task-etag",
                    updatedAt: now
                )
            ],
            calendars: [
                CalendarListMirror(
                    id: "cal",
                    summary: "Work",
                    colorHex: "#000000",
                    isSelected: true,
                    accessRole: "owner",
                    etag: "cal-etag"
                )
            ],
            events: [
                CalendarEventMirror(
                    id: "cached-event",
                    calendarID: "cal",
                    summary: "Local event",
                    details: "",
                    startDate: now,
                    endDate: now.addingTimeInterval(3600),
                    isAllDay: false,
                    status: .confirmed,
                    recurrence: [],
                    etag: "local-event-etag",
                    updatedAt: now
                )
            ],
            settings: settings
        )
    }

    private static func jsonResponse(
        for request: URLRequest,
        body: String,
        statusCode: Int = 200
    ) -> (HTTPURLResponse, Data) {
        let response = HTTPURLResponse(
            url: request.url!,
            statusCode: statusCode,
            httpVersion: nil,
            headerFields: ["Content-Type": "application/json", "Date": "Thu, 30 Apr 2026 02:00:00 GMT"]
        )!
        return (response, Data(body.utf8))
    }

    private static func query(for request: URLRequest) -> [String: String] {
        let queryItems = URLComponents(url: request.url!, resolvingAgainstBaseURL: false)?.queryItems ?? []
        return Dictionary(uniqueKeysWithValues: queryItems.map { ($0.name, $0.value ?? "") })
    }

    private static func task(
        id: String,
        taskListID: String = "list",
        completedAt: Date?,
        completed: Bool
    ) -> TaskMirror {
        TaskMirror(
            id: id,
            taskListID: taskListID,
            parentID: nil,
            title: id,
            notes: "",
            status: completed ? .completed : .needsAction,
            dueDate: nil,
            completedAt: completedAt,
            isDeleted: false,
            isHidden: false,
            position: nil,
            etag: "\(id)-etag",
            updatedAt: completedAt
        )
    }

    private static func date(_ value: String) -> Date {
        ISO8601DateFormatter().date(from: value)!
    }

    private static func dayKey(_ value: Date) -> TimeInterval {
        Calendar.current.startOfDay(for: value).timeIntervalSinceReferenceDate
    }

    private static let taskListsJSON = """
    {
      "items": [
        {"id": "list", "title": "Inbox", "updated": "2026-04-30T01:00:00Z", "etag": "list-etag"}
      ]
    }
    """

    private static let tasksJSON = """
    {
      "items": [
        {"id": "remote-task", "title": "Remote task", "status": "needsAction", "updated": "2026-04-30T01:00:00Z", "etag": "remote-task-etag"}
      ]
    }
    """

    private static let changedCachedTaskJSON = """
    {
      "items": [
        {"id": "cached-task", "title": "Changed task", "status": "needsAction", "due": "2026-04-30T00:00:00.000Z", "updated": "2026-04-30T02:30:00Z", "etag": "changed-task-etag"}
      ]
    }
    """

    private static let calendarListJSON = """
    {
      "items": [
        {"id": "cal", "summary": "Work", "backgroundColor": "#000000", "selected": true, "accessRole": "owner", "etag": "cal-etag"}
      ]
    }
    """

    private static let calendarListWithMissingJSON = """
    {
      "items": [
        {"id": "cal", "summary": "Work", "backgroundColor": "#000000", "selected": true, "accessRole": "owner", "etag": "cal-etag"},
        {"id": "missing-cal", "summary": "Missing", "backgroundColor": "#666666", "selected": true, "accessRole": "reader", "etag": "missing-etag"}
      ]
    }
    """

    private static let calendarListWithHolidayJSON = """
    {
      "items": [
        {"id": "cal", "summary": "Work", "backgroundColor": "#000000", "selected": true, "accessRole": "owner", "etag": "cal-etag"},
        {"id": "sg-holidays", "summary": "Singapore Holidays", "backgroundColor": "#0b8043", "selected": true, "accessRole": "reader", "etag": "holiday-etag"},
        {"id": "hidden", "summary": "Hidden", "backgroundColor": "#666666", "selected": false, "accessRole": "reader", "etag": "hidden-etag"}
      ]
    }
    """

    private static let eventsJSON = """
    {
      "items": [
        {
          "id": "remote-event",
          "summary": "Remote event",
          "status": "confirmed",
          "start": {"dateTime": "2026-04-30T02:00:00Z"},
          "end": {"dateTime": "2026-04-30T03:00:00Z"},
          "updated": "2026-04-30T01:00:00Z",
          "etag": "remote-event-etag"
        }
      ],
      "nextSyncToken": "next-token"
    }
    """

    private static let changedCachedEventJSON = """
    {
      "items": [
        {
          "id": "cached-event",
          "summary": "Changed event",
          "status": "confirmed",
          "start": {"dateTime": "2026-04-30T02:00:00Z"},
          "end": {"dateTime": "2026-04-30T03:00:00Z"},
          "updated": "2026-04-30T02:30:00Z",
          "etag": "changed-event-etag"
        }
      ],
      "nextSyncToken": "next-token"
    }
    """

    private static let movedAndDeletedEventsJSON = """
    {
      "items": [
        {
          "id": "moved-event",
          "summary": "Moved event",
          "status": "confirmed",
          "start": {"dateTime": "2026-05-03T10:00:00Z"},
          "end": {"dateTime": "2026-05-03T11:00:00Z"},
          "updated": "2026-05-02T00:00:00Z",
          "etag": "move-v2"
        },
        {
          "id": "multi-day",
          "summary": "Multi day",
          "status": "cancelled",
          "start": {"date": "2026-05-10"},
          "end": {"date": "2026-05-13"},
          "updated": "2026-05-12T00:00:00Z",
          "etag": "multi-v2"
        }
      ],
      "nextSyncToken": "move-token"
    }
    """

    private static let unsortedEventsWithCancelledJSON = """
    {
      "items": [
        {
          "id": "late-recurring",
          "summary": "Late recurring event",
          "status": "confirmed",
          "start": {"dateTime": "2026-05-02T12:00:00Z"},
          "end": {"dateTime": "2026-05-03T12:30:00Z"},
          "recurrence": ["RRULE:FREQ=WEEKLY;COUNT=2"],
          "updated": "2026-04-30T01:10:00Z",
          "etag": "late-etag"
        },
        {
          "id": "cancelled-event",
          "summary": "Cancelled event",
          "status": "cancelled",
          "start": {"dateTime": "2026-05-01T10:00:00Z"},
          "end": {"dateTime": "2026-05-01T11:00:00Z"},
          "updated": "2026-04-30T01:11:00Z",
          "etag": "cancelled-etag"
        },
        {
          "id": "early-all-day",
          "summary": "Early all-day event",
          "status": "confirmed",
          "start": {"date": "2026-05-01"},
          "end": {"date": "2026-05-02"},
          "updated": "2026-04-30T01:12:00Z",
          "etag": "early-etag"
        }
      ],
      "nextSyncToken": "edge-token"
    }
    """

    private static let notFoundJSON = """
    {
      "error": {
        "code": 404,
        "message": "Not Found",
        "status": "NOT_FOUND"
      }
    }
    """
}
