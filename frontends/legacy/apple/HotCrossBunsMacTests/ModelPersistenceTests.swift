import XCTest
@testable import HotCrossBunsMac

final class ModelPersistenceTests: XCTestCase {
    func testAppSettingsDecodeDefaultsNewFlags() throws {
        let data = Data(
            """
            {
              "syncMode": "manual",
              "selectedCalendarIDs": ["primary"],
              "selectedTaskListIDs": ["tasks"],
              "enableLocalNotifications": true
            }
            """.utf8
        )

        let settings = try JSONDecoder().decode(AppSettings.self, from: data)

        XCTAssertEqual(settings.syncMode, .manual)
        XCTAssertEqual(settings.selectedCalendarIDs, ["primary"])
        XCTAssertEqual(settings.selectedTaskListIDs, ["tasks"])
        XCTAssertTrue(settings.enableLocalNotifications)
        XCTAssertFalse(settings.hasConfiguredCalendarSelection)
        XCTAssertFalse(settings.hasConfiguredTaskListSelection)
        XCTAssertEqual(settings.appLanguage, .system)
        XCTAssertEqual(settings.sidebarPlacement, .left)
        XCTAssertFalse(settings.hasCompletedOnboarding)
    }

    func testAppLanguageRoundTripsSupportedOverrides() throws {
        for language in AppLanguage.allCases {
            let settings = AppSettings(
                syncMode: .manual,
                selectedCalendarIDs: [],
                selectedTaskListIDs: [],
                enableLocalNotifications: false,
                appLanguage: language
            )

            let data = try JSONEncoder().encode(settings)
            let decoded = try JSONDecoder().decode(AppSettings.self, from: data)

            XCTAssertEqual(decoded.appLanguage, language)
        }
    }

    func testAppLanguageFallsBackToSystemForUnknownRawValue() throws {
        let data = Data(
            """
            {
              "syncMode": "manual",
              "selectedCalendarIDs": [],
              "selectedTaskListIDs": [],
              "enableLocalNotifications": false,
              "appLanguage": "xx"
            }
            """.utf8
        )

        let settings = try JSONDecoder().decode(AppSettings.self, from: data)

        XCTAssertEqual(settings.appLanguage, .system)
    }

    func testAppLanguageLocaleMetadata() {
        XCTAssertNil(AppLanguage.system.localeIdentifier)

        let expectedLocaleIdentifiers: [AppLanguage: String] = [
            .en: "en",
            .ms: "ms",
            .ta: "ta",
            .zhHans: "zh-Hans",
            .id: "id",
            .vi: "vi",
            .th: "th",
            .ja: "ja",
            .ko: "ko",
            .zhHant: "zh-Hant",
            .hi: "hi"
        ]

        for (language, identifier) in expectedLocaleIdentifiers {
            XCTAssertEqual(language.localeIdentifier, identifier)
        }

        XCTAssertEqual(AppLanguage(localeIdentifier: "zh-Hans"), .zhHans)
        XCTAssertEqual(AppLanguage(localeIdentifier: "zh-CN"), .zhHans)
        XCTAssertEqual(AppLanguage(localeIdentifier: "zh-Hant"), .zhHant)
        XCTAssertEqual(AppLanguage(localeIdentifier: "zh-TW"), .zhHant)
        XCTAssertEqual(AppLanguage(localeIdentifier: "zh-HK"), .zhHant)
        XCTAssertEqual(AppLanguage(localeIdentifier: "ta-IN"), .ta)
        XCTAssertEqual(AppLanguage(localeIdentifier: "id-ID"), .id)
        XCTAssertEqual(AppLanguage(localeIdentifier: "vi-VN"), .vi)
        XCTAssertEqual(AppLanguage(localeIdentifier: "th-TH"), .th)
        XCTAssertEqual(AppLanguage(localeIdentifier: "ja-JP"), .ja)
        XCTAssertEqual(AppLanguage(localeIdentifier: "ko-KR"), .ko)
        XCTAssertEqual(AppLanguage(localeIdentifier: "hi-IN"), .hi)
    }

    func testAppLanguageCasesMatchSupportedLocalizationBatch() {
        let supportedLocaleIdentifiers = Set(AppLanguage.allCases.compactMap { $0.localeIdentifier })

        XCTAssertEqual(
            supportedLocaleIdentifiers,
            ["en", "ms", "ta", "zh-Hans", "id", "vi", "th", "ja", "ko", "zh-Hant", "hi"]
        )
    }

    func testCalendarEventDecodeDefaultsMissingReminders() throws {
        let data = Data(
            """
            {
              "id": "event-1",
              "calendarID": "primary",
              "summary": "Planning",
              "details": "",
              "startDate": "2026-04-18T01:00:00Z",
              "endDate": "2026-04-18T02:00:00Z",
              "isAllDay": false,
              "status": "confirmed",
              "recurrence": [],
              "etag": "etag-1",
              "updatedAt": "2026-04-18T00:00:00Z"
            }
            """.utf8
        )
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601

        let event = try decoder.decode(CalendarEventMirror.self, from: data)

        XCTAssertEqual(event.id, "event-1")
        XCTAssertEqual(event.reminderMinutes, [])
    }

    func testCachedAppStateDecodeDefaultsMissingCollections() throws {
        let data = Data("{}".utf8)

        let state = try JSONDecoder().decode(CachedAppState.self, from: data)

        XCTAssertNil(state.account)
        XCTAssertTrue(state.accounts.isEmpty)
        XCTAssertNil(state.activeAccountID)
        XCTAssertTrue(state.accountWorkspaces.isEmpty)
        XCTAssertTrue(state.taskLists.isEmpty)
        XCTAssertTrue(state.tasks.isEmpty)
        XCTAssertTrue(state.calendars.isEmpty)
        XCTAssertTrue(state.events.isEmpty)
        XCTAssertEqual(state.settings.syncMode, .balanced)
        XCTAssertTrue(state.syncCheckpoints.isEmpty)
        XCTAssertTrue(state.pendingMutations.isEmpty)
    }

    func testLocalCacheStoreRoundTripsState() async throws {
        let fileURL = FileManager.default.temporaryDirectory
            .appending(path: "HotCrossBunsTests", directoryHint: .isDirectory)
            .appending(path: UUID().uuidString)
            .appendingPathExtension("json")
        defer { try? FileManager.default.removeItem(at: fileURL.deletingLastPathComponent()) }
        let store = LocalCacheStore(fileURL: fileURL)

        await store.save(.preview)
        let loadedState = await store.loadCachedState()

        XCTAssertEqual(loadedState.account, .preview)
        XCTAssertEqual(loadedState.accounts, [.preview])
        XCTAssertEqual(loadedState.activeAccountID, GoogleAccount.preview.id)
        XCTAssertEqual(loadedState.taskLists.count, CachedAppState.preview.taskLists.count)
        XCTAssertEqual(loadedState.tasks.count, CachedAppState.preview.tasks.count)
        XCTAssertEqual(loadedState.calendars.count, CachedAppState.preview.calendars.count)
        XCTAssertEqual(loadedState.events.count, CachedAppState.preview.events.count)
        let cacheFilePath = await store.cacheFilePath()
        XCTAssertEqual(cacheFilePath, fileURL.deletingPathExtension().appendingPathExtension("sqlite").path)
    }

    @MainActor
    func testDatabaseEntitySearchSignalAndRegexFallback() async throws {
        let fileURL = FileManager.default.temporaryDirectory
            .appending(path: "HotCrossBunsTests", directoryHint: .isDirectory)
            .appending(path: UUID().uuidString)
            .appendingPathExtension("json")
        defer { try? FileManager.default.removeItem(at: fileURL.deletingLastPathComponent()) }

        let store = LocalCacheStore(fileURL: fileURL)
        await store.save(.preview)
        let model = makeModel(cachedState: .empty, cacheStore: store)
        await model.loadInitialState()

        XCTAssertTrue(model.usesDatabaseEntitySearch)

        let textResults = try await model.searchEntities(query: "Google Tasks", limit: 10)
        XCTAssertTrue(try XCTUnwrap(textResults).contains { entity in
            if case .task(let task) = entity {
                return task.id == "task-1"
            }
            return false
        })

        let structuredResults = try await model.searchEntities(query: "kind:event title:adapter", limit: 10)
        XCTAssertTrue(try XCTUnwrap(structuredResults).contains { entity in
            if case .event(let event) = entity {
                return event.id == "event-1"
            }
            return false
        })

        let regexResults = try await model.searchEntities(query: "/Calendar adapter/", limit: 10)
        XCTAssertNil(regexResults)
    }

    @MainActor
    func testTodayPrintPayloadFromQueryMatchesArrayFilteredBehavior() async throws {
        let fileURL = FileManager.default.temporaryDirectory
            .appending(path: "HotCrossBunsTests", directoryHint: .isDirectory)
            .appending(path: UUID().uuidString)
            .appendingPathExtension("json")
        defer { try? FileManager.default.removeItem(at: fileURL.deletingLastPathComponent()) }

        let now = Date(timeIntervalSince1970: 1_800_000_000)
        let calendar = Calendar.current
        let startOfToday = calendar.startOfDay(for: now)
        var state = CachedAppState.preview
        let taskListID = try XCTUnwrap(state.taskLists.first?.id)
        let calendarID = try XCTUnwrap(state.calendars.first?.id)
        state.settings.selectedCalendarIDs = [calendarID]
        state.settings.hasConfiguredCalendarSelection = true
        state.tasks = [
            task(
                id: "overdue",
                taskListID: taskListID,
                dueDate: calendar.date(byAdding: .day, value: -2, to: startOfToday),
                status: .needsAction,
                isDeleted: false
            ),
            task(
                id: "due-today",
                taskListID: taskListID,
                dueDate: startOfToday,
                status: .needsAction,
                isDeleted: false
            ),
            task(
                id: "completed-overdue",
                taskListID: taskListID,
                dueDate: calendar.date(byAdding: .day, value: -1, to: startOfToday),
                status: .completed,
                isDeleted: false
            ),
            task(
                id: "no-due",
                taskListID: taskListID,
                dueDate: nil,
                status: .needsAction,
                isDeleted: false
            )
        ]
        state.events = [
            event(id: "upcoming-1", calendarID: calendarID, startDate: now.addingTimeInterval(3_600), status: .confirmed),
            event(id: "upcoming-2", calendarID: calendarID, startDate: now.addingTimeInterval(7_200), status: .confirmed),
            event(id: "ongoing", calendarID: calendarID, startDate: now.addingTimeInterval(-600), status: .confirmed),
            event(id: "after-horizon", calendarID: calendarID, startDate: now.addingTimeInterval(90_000), status: .confirmed),
            event(id: "cancelled", calendarID: calendarID, startDate: now.addingTimeInterval(1_800), status: .cancelled)
        ]

        let store = LocalCacheStore(fileURL: fileURL)
        await store.save(state)
        let model = makeModel(cachedState: .empty, cacheStore: store)
        await model.loadInitialState()

        let payload = await model.todayPrintPayloadFromQuery(now: now)
        let horizon = calendar.date(byAdding: .day, value: 1, to: now) ?? now
        let expectedOverdue = model.tasks.filter { task in
            task.isDeleted == false
                && task.isCompleted == false
                && task.isHidden == false
                && (task.dueDate.map { $0 < startOfToday } ?? false)
        }
        let expectedUpcoming = model.events
            .filter { event in
                event.status != .cancelled
                    && event.calendarID == calendarID
                    && event.startDate > now
                    && event.startDate <= horizon
            }
            .sorted { lhs, rhs in
                if lhs.startDate != rhs.startDate { return lhs.startDate < rhs.startDate }
                return lhs.id < rhs.id
            }

        XCTAssertEqual(payload.todaySnapshot, model.todaySnapshot)
        XCTAssertEqual(payload.overdueTasks.map(\.id), expectedOverdue.map(\.id))
        XCTAssertEqual(payload.upcomingEvents.map(\.id), expectedUpcoming.map(\.id))
        XCTAssertEqual(payload.generatedAt, now)
    }

    @MainActor
    func testCacheLoadWarningDoesNotBecomeMutationError() async throws {
        let fileURL = FileManager.default.temporaryDirectory
            .appending(path: "HotCrossBunsTests", directoryHint: .isDirectory)
            .appending(path: UUID().uuidString)
            .appendingPathExtension("json")
        defer { try? FileManager.default.removeItem(at: fileURL.deletingLastPathComponent()) }

        let legacyStore = LocalCacheStore(fileURL: fileURL, storageBackend: .jsonSidecar)
        await legacyStore.save(.preview)
        try Data("not a sqlite database".utf8)
            .write(to: fileURL.deletingPathExtension().appendingPathExtension("sqlite"))

        let model = makeModel(cachedState: .empty, cacheStore: LocalCacheStore(fileURL: fileURL))
        await model.loadInitialState()

        XCTAssertNil(model.lastMutationError)
        XCTAssertEqual(model.lastCacheLoadWarning, "SQLite cache could not be read; restored from legacy JSON cache.")
    }

    func testCachedAppStateMigratesLegacyAccountIntoAccountCatalog() throws {
        let data = Data(
            """
            {
              "schemaVersion": 1,
              "account": {
                "id": "google-1",
                "email": "person@example.com",
                "displayName": "Personal",
                "grantedScopes": ["https://www.googleapis.com/auth/tasks"],
                "authProvider": "customDesktopOAuth"
              }
            }
            """.utf8
        )

        let state = try JSONDecoder().decode(CachedAppState.self, from: data)

        XCTAssertEqual(state.schemaVersion, CachedAppState.currentSchemaVersion)
        XCTAssertEqual(state.account?.id, "google-1")
        XCTAssertEqual(state.accounts.map(\.id), ["google-1"])
        XCTAssertEqual(state.activeAccountID, "google-1")
        XCTAssertEqual(state.accountWorkspaces.map(\.accountID), ["google-1"])
    }

    func testCachedAppStateMigratesLegacyRootPayloadIntoActiveWorkspace() throws {
        let data = Data(
            """
            {
              "schemaVersion": 1,
              "account": {
                "id": "google-1",
                "email": "person@example.com",
                "displayName": "Personal",
                "grantedScopes": ["https://www.googleapis.com/auth/tasks"],
                "authProvider": "customDesktopOAuth"
              },
              "taskLists": [
                { "id": "list-1", "title": "Inbox", "updatedAt": "2026-04-18T00:00:00Z", "etag": "tl" }
              ],
              "tasks": [
                {
                  "id": "task-1",
                  "taskListID": "list-1",
                  "title": "Legacy task",
                  "notes": "",
                  "status": "needsAction",
                  "isDeleted": false,
                  "isHidden": false
                }
              ],
              "calendars": [],
              "events": [],
              "settings": {
                "syncMode": "manual",
                "selectedCalendarIDs": [],
                "selectedTaskListIDs": ["list-1"],
                "enableLocalNotifications": false
              },
              "pendingMutations": [
                {
                  "id": "00000000-0000-0000-0000-000000000001",
                  "createdAt": "2026-04-18T00:00:00Z",
                  "resourceType": "task",
                  "resourceID": "task-1",
                  "action": "update",
                  "payload": ""
                }
              ]
            }
            """.utf8
        )
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601

        let state = try decoder.decode(CachedAppState.self, from: data)
        let workspace = try XCTUnwrap(state.accountWorkspaces.first)

        XCTAssertEqual(state.schemaVersion, CachedAppState.currentSchemaVersion)
        XCTAssertEqual(workspace.accountID, "google-1")
        XCTAssertEqual(workspace.taskLists.map(\.id), ["list-1"])
        XCTAssertEqual(workspace.tasks.map(\.id), ["task-1"])
        XCTAssertEqual(workspace.pendingMutations.first?.accountID, "google-1")
        XCTAssertEqual(workspace.settings.selectedTaskListIDs, ["list-1"])
    }

    func testCachedAppStateSplitsMixedRootSyncMetadataIntoAccountWorkspaces() throws {
        let personal = GoogleAccount.preview
        let work = GoogleAccount(
            id: "work-account",
            email: "work@example.com",
            displayName: "Work",
            grantedScopes: [GoogleScope.tasks],
            authProvider: .customDesktopOAuth
        )
        let personalCheckpoint = SyncCheckpoint(
            id: SyncCheckpoint.stableID(accountID: personal.id, resourceType: .taskList, resourceID: "personal-list"),
            accountID: personal.id,
            resourceType: .taskList,
            resourceID: "personal-list",
            calendarSyncToken: nil,
            tasksUpdatedMin: Date(timeIntervalSince1970: 10),
            lastSuccessfulSyncAt: nil
        )
        let workCheckpoint = SyncCheckpoint(
            id: SyncCheckpoint.stableID(accountID: work.id, resourceType: .taskList, resourceID: "work-list"),
            accountID: work.id,
            resourceType: .taskList,
            resourceID: "work-list",
            calendarSyncToken: nil,
            tasksUpdatedMin: Date(timeIntervalSince1970: 20),
            lastSuccessfulSyncAt: nil
        )
        let legacyActiveMutation = PendingMutation(
            id: UUID(uuidString: "00000000-0000-0000-0000-000000000010")!,
            createdAt: Date(timeIntervalSince1970: 30),
            resourceType: .task,
            resourceID: "personal-task",
            action: .update,
            payload: Data()
        )
        let workMutation = PendingMutation(
            id: UUID(uuidString: "00000000-0000-0000-0000-000000000020")!,
            accountID: work.id,
            createdAt: Date(timeIntervalSince1970: 40),
            resourceType: .task,
            resourceID: "work-task",
            action: .update,
            payload: Data()
        )

        let state = CachedAppState(
            account: personal,
            accounts: [personal, work],
            activeAccountID: personal.id,
            taskLists: [],
            tasks: [],
            calendars: [],
            events: [],
            settings: .default,
            syncCheckpoints: [personalCheckpoint, workCheckpoint],
            pendingMutations: [legacyActiveMutation, workMutation]
        )

        XCTAssertEqual(Set(state.syncCheckpoints.map(\.accountID)), [personal.id])
        XCTAssertEqual(Set(state.pendingMutations.compactMap(\.accountID)), [personal.id])
        let workWorkspace = try XCTUnwrap(state.accountWorkspaces.first { $0.accountID == work.id })
        XCTAssertEqual(workWorkspace.syncCheckpoints.map(\.resourceID), ["work-list"])
        XCTAssertEqual(workWorkspace.pendingMutations.map(\.accountID), [work.id])
    }

    func testSwitchingActiveAccountRestoresWorkspaceAndAccountScopedSettings() throws {
        let personal = GoogleAccount.preview
        let work = GoogleAccount(
            id: "work-account",
            email: "work@example.com",
            displayName: "Work",
            grantedScopes: [GoogleScope.tasks, GoogleScope.calendar],
            authProvider: .customDesktopOAuth
        )
        var workSettings = AppSettings.default
        workSettings.syncMode = .manual
        workSettings.selectedTaskListIDs = ["work-list"]
        workSettings.hasConfiguredTaskListSelection = true
        let workWorkspace = AccountWorkspaceSnapshot(
            accountID: work.id,
            taskLists: [
                TaskListMirror(id: "work-list", title: "Work", updatedAt: nil, etag: nil)
            ],
            tasks: [],
            calendars: [],
            events: [],
            settings: workSettings,
            syncCheckpoints: [],
            pendingMutations: []
        )
        let state = CachedAppState(
            account: personal,
            accounts: [personal, work],
            activeAccountID: personal.id,
            accountWorkspaces: [workWorkspace],
            taskLists: [TaskListMirror(id: "personal-list", title: "Personal", updatedAt: nil, etag: nil)],
            tasks: [],
            calendars: [],
            events: [],
            settings: .default,
            syncCheckpoints: [],
            pendingMutations: []
        )

        let switched = try XCTUnwrap(state.switchingActiveAccount(to: work.id))

        XCTAssertEqual(switched.account, work)
        XCTAssertEqual(switched.activeAccountID, work.id)
        XCTAssertEqual(switched.taskLists.map(\.id), ["work-list"])
        XCTAssertEqual(switched.settings.syncMode, .manual)
        XCTAssertEqual(switched.settings.selectedTaskListIDs, ["work-list"])
    }

    @MainActor
    func testAppModelSwitchesBetweenAccountWorkspaces() async throws {
        let personal = GoogleAccount.preview
        let work = GoogleAccount(
            id: "work-account",
            email: "work@example.com",
            displayName: "Work",
            grantedScopes: [GoogleScope.tasks, GoogleScope.calendar],
            authProvider: .customDesktopOAuth
        )
        let workWorkspace = AccountWorkspaceSnapshot(
            accountID: work.id,
            taskLists: [TaskListMirror(id: "work-list", title: "Work", updatedAt: nil, etag: nil)],
            tasks: [],
            calendars: [],
            events: [],
            settings: .default,
            syncCheckpoints: [],
            pendingMutations: []
        )
        let state = CachedAppState(
            account: personal,
            accounts: [personal, work],
            activeAccountID: personal.id,
            accountWorkspaces: [workWorkspace],
            taskLists: [TaskListMirror(id: "personal-list", title: "Personal", updatedAt: nil, etag: nil)],
            tasks: [],
            calendars: [],
            events: [],
            settings: .default,
            syncCheckpoints: [],
            pendingMutations: []
        )
        let model = makeModel(cachedState: state)
        await model.loadInitialState()

        XCTAssertEqual(model.activeAccountID, personal.id)
        XCTAssertEqual(model.taskLists.map(\.id), ["personal-list"])

        let didSwitchToWork = await model.switchGoogleAccount(to: work.id)
        XCTAssertTrue(didSwitchToWork)
        XCTAssertEqual(model.activeAccountID, work.id)
        XCTAssertEqual(model.taskLists.map(\.id), ["work-list"])

        let didSwitchToPersonal = await model.switchGoogleAccount(to: personal.id)
        XCTAssertTrue(didSwitchToPersonal)
        XCTAssertEqual(model.activeAccountID, personal.id)
        XCTAssertEqual(model.taskLists.map(\.id), ["personal-list"])
    }

    @MainActor
    func testSwitchingPersistsDirtyActiveSettingsBeforeLoadingTargetWorkspace() async throws {
        let personal = GoogleAccount.preview
        let work = GoogleAccount(
            id: "work-account",
            email: "work@example.com",
            displayName: "Work",
            grantedScopes: [GoogleScope.tasks, GoogleScope.calendar],
            authProvider: .customDesktopOAuth
        )
        var workSettings = AppSettings.default
        workSettings.tasksTabSelectedListIDs = ["work-list"]
        workSettings.hasConfiguredTasksTabSelection = true
        let workWorkspace = AccountWorkspaceSnapshot(
            accountID: work.id,
            taskLists: [TaskListMirror(id: "work-list", title: "Work", updatedAt: nil, etag: nil)],
            tasks: [],
            calendars: [],
            events: [],
            settings: workSettings,
            syncCheckpoints: [],
            pendingMutations: []
        )
        let state = CachedAppState(
            account: personal,
            accounts: [personal, work],
            activeAccountID: personal.id,
            accountWorkspaces: [workWorkspace],
            taskLists: [TaskListMirror(id: "personal-list", title: "Personal", updatedAt: nil, etag: nil)],
            tasks: [],
            calendars: [],
            events: [],
            settings: .default,
            syncCheckpoints: [],
            pendingMutations: []
        )
        let model = makeModel(cachedState: state)
        await model.loadInitialState()

        model.setTasksTabListFilter(["personal-list"])
        let didSwitchToWork = await model.switchGoogleAccount(to: work.id)
        XCTAssertTrue(didSwitchToWork)
        XCTAssertEqual(model.settings.tasksTabSelectedListIDs, ["work-list"])
        let didSwitchToPersonal = await model.switchGoogleAccount(to: personal.id)
        XCTAssertTrue(didSwitchToPersonal)

        XCTAssertEqual(model.settings.tasksTabSelectedListIDs, ["personal-list"])
        XCTAssertTrue(model.settings.hasConfiguredTasksTabSelection)
    }

    @MainActor
    func testSwitchingToMissingAccountLeavesLiveWorkspaceUntouched() async throws {
        let personal = GoogleAccount.preview
        let state = CachedAppState(
            account: personal,
            accounts: [personal],
            activeAccountID: personal.id,
            taskLists: [TaskListMirror(id: "personal-list", title: "Personal", updatedAt: nil, etag: nil)],
            tasks: [],
            calendars: [],
            events: [],
            settings: .default,
            syncCheckpoints: [],
            pendingMutations: []
        )
        let model = makeModel(cachedState: state)
        await model.loadInitialState()

        let didSwitch = await model.switchGoogleAccount(to: "missing-account")

        XCTAssertFalse(didSwitch)
        XCTAssertEqual(model.activeAccountID, personal.id)
        XCTAssertEqual(model.taskLists.map(\.id), ["personal-list"])
        XCTAssertEqual(model.lastMutationError, "That Google account is not connected on this Mac.")
    }

    @MainActor
    func testReplaySkipsInactiveAccountMutationsEvenIfTheyLeakIntoActiveQueue() async throws {
        MockURLProtocol.reset()
        defer { MockURLProtocol.reset() }

        let personal = GoogleAccount.preview
        let work = GoogleAccount(
            id: "work-account",
            email: "work@example.com",
            displayName: "Work",
            grantedScopes: [GoogleScope.tasks],
            authProvider: .customDesktopOAuth
        )
        var activeMutation = try PendingMutation.taskCreate(payload: PendingTaskCreatePayload(
            localID: "local-active",
            taskListID: "personal-list",
            title: "Active write",
            notes: "",
            dueDate: nil,
            parentID: nil
        ))
        activeMutation.accountID = personal.id
        var inactiveMutation = try PendingMutation.taskCreate(payload: PendingTaskCreatePayload(
            localID: "local-inactive",
            taskListID: "work-list",
            title: "Inactive write",
            notes: "",
            dueDate: nil,
            parentID: nil
        ))
        inactiveMutation.accountID = work.id
        var state = CachedAppState(
            account: personal,
            accounts: [personal, work],
            activeAccountID: personal.id,
            taskLists: [TaskListMirror(id: "personal-list", title: "Personal", updatedAt: nil, etag: nil)],
            tasks: [],
            calendars: [],
            events: [],
            settings: .default,
            syncCheckpoints: [],
            pendingMutations: []
        )
        state.pendingMutations = [activeMutation, inactiveMutation]
        MockURLProtocol.requestHandler = { request in
            XCTAssertEqual(request.url?.path, "/tasks/v1/lists/personal-list/tasks")
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            return (response, Data(#"{"id":"server-active","title":"Active write","status":"needsAction","etag":"srv"}"#.utf8))
        }
        let model = makeModel(cachedState: state, urlSession: MockURLProtocol.testSession())
        await model.loadInitialState()

        await model.replayPendingMutations()

        XCTAssertEqual(MockURLProtocol.capturedRequests.map { $0.url?.path }, ["/tasks/v1/lists/personal-list/tasks"])
        XCTAssertEqual(model.pendingMutations.map(\.id), [inactiveMutation.id])
        XCTAssertEqual(model.pendingMutations.first?.accountID, work.id)
    }

    @MainActor
    func testAppModelDisconnectsInactiveAccountWithoutTouchingActiveWorkspace() async throws {
        let personal = GoogleAccount.preview
        let work = GoogleAccount(
            id: "work-account",
            email: "work@example.com",
            displayName: "Work",
            grantedScopes: [GoogleScope.tasks, GoogleScope.calendar],
            authProvider: .customDesktopOAuth
        )
        let workWorkspace = AccountWorkspaceSnapshot(
            accountID: work.id,
            taskLists: [TaskListMirror(id: "work-list", title: "Work", updatedAt: nil, etag: nil)],
            tasks: [],
            calendars: [],
            events: [],
            settings: .default,
            syncCheckpoints: [],
            pendingMutations: []
        )
        let state = CachedAppState(
            account: personal,
            accounts: [personal, work],
            activeAccountID: personal.id,
            accountWorkspaces: [workWorkspace],
            taskLists: [TaskListMirror(id: "personal-list", title: "Personal", updatedAt: nil, etag: nil)],
            tasks: [],
            calendars: [],
            events: [],
            settings: .default,
            syncCheckpoints: [],
            pendingMutations: []
        )
        let store = InMemoryGoogleOAuthTokenStore(
            clientConfiguration: GoogleOAuthClientConfiguration(clientID: "abc.apps.googleusercontent.com", clientSecret: nil)
        )
        try store.saveTokenSet(testTokenSet(account: personal), accountID: personal.id)
        try store.saveTokenSet(testTokenSet(account: work), accountID: work.id)
        let model = makeModel(cachedState: state, tokenStore: store)
        await model.loadInitialState()

        await model.disconnectGoogleAccount(id: work.id)

        XCTAssertEqual(model.activeAccountID, personal.id)
        XCTAssertEqual(model.connectedAccounts.map(\.id), [personal.id])
        XCTAssertEqual(model.taskLists.map(\.id), ["personal-list"])
        XCTAssertNil(store.loadTokenSet(accountID: work.id))
        XCTAssertNotNil(store.loadTokenSet(accountID: personal.id))
    }

    @MainActor
    func testAppModelDisconnectsActiveAccountSwitchesToFallbackWorkspace() async throws {
        let personal = GoogleAccount.preview
        let work = GoogleAccount(
            id: "work-account",
            email: "work@example.com",
            displayName: "Work",
            grantedScopes: [GoogleScope.tasks, GoogleScope.calendar],
            authProvider: .customDesktopOAuth
        )
        let workWorkspace = AccountWorkspaceSnapshot(
            accountID: work.id,
            taskLists: [TaskListMirror(id: "work-list", title: "Work", updatedAt: nil, etag: nil)],
            tasks: [],
            calendars: [],
            events: [],
            settings: .default,
            syncCheckpoints: [],
            pendingMutations: []
        )
        let state = CachedAppState(
            account: personal,
            accounts: [personal, work],
            activeAccountID: personal.id,
            accountWorkspaces: [workWorkspace],
            taskLists: [TaskListMirror(id: "personal-list", title: "Personal", updatedAt: nil, etag: nil)],
            tasks: [],
            calendars: [],
            events: [],
            settings: .default,
            syncCheckpoints: [],
            pendingMutations: []
        )
        let store = InMemoryGoogleOAuthTokenStore(
            clientConfiguration: GoogleOAuthClientConfiguration(clientID: "abc.apps.googleusercontent.com", clientSecret: nil)
        )
        try store.saveTokenSet(testTokenSet(account: personal), accountID: personal.id)
        try store.saveTokenSet(testTokenSet(account: work), accountID: work.id)
        let model = makeModel(cachedState: state, tokenStore: store)
        await model.loadInitialState()

        await model.disconnectGoogleAccount(id: personal.id)

        XCTAssertEqual(model.activeAccountID, work.id)
        XCTAssertEqual(model.connectedAccounts.map(\.id), [work.id])
        XCTAssertEqual(model.taskLists.map(\.id), ["work-list"])
        XCTAssertNil(store.loadTokenSet(accountID: personal.id))
        XCTAssertNotNil(store.loadTokenSet(accountID: work.id))
    }

    @MainActor
    func testAppModelDisconnectsLastAccountLeavesSignedOutLocalState() async throws {
        let personal = GoogleAccount.preview
        let state = CachedAppState(
            account: personal,
            accounts: [personal],
            activeAccountID: personal.id,
            taskLists: [TaskListMirror(id: "personal-list", title: "Personal", updatedAt: nil, etag: nil)],
            tasks: [],
            calendars: [],
            events: [],
            settings: .default,
            syncCheckpoints: [],
            pendingMutations: []
        )
        let store = InMemoryGoogleOAuthTokenStore(
            clientConfiguration: GoogleOAuthClientConfiguration(clientID: "abc.apps.googleusercontent.com", clientSecret: nil)
        )
        try store.saveTokenSet(testTokenSet(account: personal), accountID: personal.id)
        let model = makeModel(cachedState: state, tokenStore: store)
        await model.loadInitialState()

        await model.disconnectGoogleAccount(id: personal.id)

        XCTAssertNil(model.activeAccountID)
        XCTAssertTrue(model.connectedAccounts.isEmpty)
        XCTAssertTrue(model.taskLists.isEmpty)
        XCTAssertNil(store.loadTokenSet(accountID: personal.id))
    }

    @MainActor
    func testChangingOAuthClientDropsInactiveCustomAccountsWithoutSigningOutEmbeddedActive() async throws {
        let personal = GoogleAccount.preview
        let work = GoogleAccount(
            id: "work-account",
            email: "work@example.com",
            displayName: "Work",
            grantedScopes: [GoogleScope.tasks, GoogleScope.calendar],
            authProvider: .customDesktopOAuth
        )
        let workWorkspace = AccountWorkspaceSnapshot(
            accountID: work.id,
            taskLists: [TaskListMirror(id: "work-list", title: "Work", updatedAt: nil, etag: nil)],
            tasks: [],
            calendars: [],
            events: [],
            settings: .default,
            syncCheckpoints: [],
            pendingMutations: []
        )
        let state = CachedAppState(
            account: personal,
            accounts: [personal, work],
            activeAccountID: personal.id,
            accountWorkspaces: [workWorkspace],
            taskLists: [TaskListMirror(id: "personal-list", title: "Personal", updatedAt: nil, etag: nil)],
            tasks: [],
            calendars: [],
            events: [],
            settings: .default,
            syncCheckpoints: [],
            pendingMutations: []
        )
        let store = InMemoryGoogleOAuthTokenStore(
            clientConfiguration: GoogleOAuthClientConfiguration(clientID: "abc.apps.googleusercontent.com", clientSecret: nil)
        )
        try store.saveTokenSet(testTokenSet(account: work), accountID: work.id)
        let model = makeModel(cachedState: state, tokenStore: store)
        await model.loadInitialState()

        model.saveCustomOAuthClientConfiguration(clientID: "new-client.apps.googleusercontent.com", clientSecret: nil)

        XCTAssertEqual(model.activeAccountID, personal.id)
        XCTAssertEqual(model.connectedAccounts.map(\.id), [personal.id])
        XCTAssertEqual(model.taskLists.map(\.id), ["personal-list"])
        XCTAssertFalse(model.accountWorkspaces.contains { $0.accountID == work.id })
        XCTAssertNil(store.loadTokenSet(accountID: work.id))
    }

    @MainActor
    func testClearingOAuthClientDropsActiveCustomAccountAndFallsBackToEmbeddedAccount() async throws {
        let personal = GoogleAccount.preview
        let work = GoogleAccount(
            id: "work-account",
            email: "work@example.com",
            displayName: "Work",
            grantedScopes: [GoogleScope.tasks, GoogleScope.calendar],
            authProvider: .customDesktopOAuth
        )
        let personalWorkspace = AccountWorkspaceSnapshot(
            accountID: personal.id,
            taskLists: [TaskListMirror(id: "personal-list", title: "Personal", updatedAt: nil, etag: nil)],
            tasks: [],
            calendars: [],
            events: [],
            settings: .default,
            syncCheckpoints: [],
            pendingMutations: []
        )
        let state = CachedAppState(
            account: work,
            accounts: [work, personal],
            activeAccountID: work.id,
            accountWorkspaces: [personalWorkspace],
            taskLists: [TaskListMirror(id: "work-list", title: "Work", updatedAt: nil, etag: nil)],
            tasks: [],
            calendars: [],
            events: [],
            settings: .default,
            syncCheckpoints: [],
            pendingMutations: []
        )
        let store = InMemoryGoogleOAuthTokenStore(
            clientConfiguration: GoogleOAuthClientConfiguration(clientID: "abc.apps.googleusercontent.com", clientSecret: nil)
        )
        try store.saveTokenSet(testTokenSet(account: work), accountID: work.id)
        let model = makeModel(cachedState: state, tokenStore: store)
        await model.loadInitialState()

        model.clearCustomOAuthClientConfiguration()

        XCTAssertEqual(model.activeAccountID, personal.id)
        XCTAssertEqual(model.connectedAccounts.map(\.id), [personal.id])
        XCTAssertEqual(model.taskLists.map(\.id), ["personal-list"])
        XCTAssertFalse(model.accountWorkspaces.contains { $0.accountID == work.id })
        XCTAssertNil(store.loadClientConfiguration())
        XCTAssertNil(store.loadTokenSet(accountID: work.id))
    }

    func testCachedAppStateAllowsSignedOutAccountCatalog() throws {
        let savedAccount = GoogleAccount(
            id: "google-2",
            email: "work@example.com",
            displayName: "Work",
            grantedScopes: [GoogleScope.tasks, GoogleScope.calendar],
            authProvider: .customDesktopOAuth
        )

        let state = CachedAppState(
            account: nil,
            accounts: [savedAccount],
            activeAccountID: nil,
            taskLists: [],
            tasks: [],
            calendars: [],
            events: [],
            settings: .default
        )

        XCTAssertNil(state.account)
        XCTAssertEqual(state.accounts, [savedAccount])
        XCTAssertNil(state.activeAccountID)
    }

    func testTodaySnapshotExcludesCompletedDeletedAndCancelledItems() {
        let calendar = Calendar(identifier: .gregorian)
        let referenceDate = calendar.date(from: DateComponents(year: 2026, month: 4, day: 18, hour: 12))!
        let yesterday = calendar.date(byAdding: .day, value: -1, to: referenceDate)!
        let todayMorning = calendar.date(from: DateComponents(year: 2026, month: 4, day: 18, hour: 9))!
        let todayAfternoon = calendar.date(from: DateComponents(year: 2026, month: 4, day: 18, hour: 14))!
        let taskListID = "tasks"
        let calendarID = "primary"
        let tasks = [
            task(id: "due-today", taskListID: taskListID, dueDate: todayMorning, status: .needsAction, isDeleted: false),
            task(id: "overdue", taskListID: taskListID, dueDate: yesterday, status: .needsAction, isDeleted: false),
            task(id: "completed", taskListID: taskListID, dueDate: todayMorning, status: .completed, isDeleted: false),
            task(id: "deleted", taskListID: taskListID, dueDate: todayMorning, status: .needsAction, isDeleted: true)
        ]
        let events = [
            event(id: "later", calendarID: calendarID, startDate: todayAfternoon, status: .confirmed),
            event(id: "cancelled", calendarID: calendarID, startDate: todayMorning, status: .cancelled),
            event(id: "earlier", calendarID: calendarID, startDate: todayMorning, status: .confirmed)
        ]

        let snapshot = TodaySnapshot.build(
            tasks: tasks,
            events: events,
            referenceDate: referenceDate,
            calendar: calendar
        )

        XCTAssertEqual(snapshot.dueTasks.map(\.id), ["due-today"])
        XCTAssertEqual(snapshot.overdueCount, 1)
        XCTAssertEqual(snapshot.scheduledEvents.map(\.id), ["earlier", "later"])
    }

    @MainActor
    private func makeModel(
        cachedState: CachedAppState,
        cacheStore: LocalCacheStore? = nil,
        tokenStore: InMemoryGoogleOAuthTokenStore = InMemoryGoogleOAuthTokenStore(
            clientConfiguration: GoogleOAuthClientConfiguration(clientID: "abc.apps.googleusercontent.com", clientSecret: nil)
        ),
        urlSession: URLSession = .shared
    ) -> AppModel {
        let customOAuthService = CustomGoogleOAuthService(tokenStore: tokenStore)
        let authService = GoogleAuthService(customOAuthService: customOAuthService)
        let transport = GoogleAPITransport(
            baseURL: URL(string: "https://www.googleapis.com")!,
            tokenProvider: StaticAccessTokenProvider(token: "test-token"),
            urlSession: urlSession
        )
        let tasksClient = GoogleTasksClient(transport: transport)
        let calendarClient = GoogleCalendarClient(transport: transport)
        return AppModel(
            authService: authService,
            tasksClient: tasksClient,
            calendarClient: calendarClient,
            syncScheduler: SyncScheduler(tasksClient: tasksClient, calendarClient: calendarClient),
            cacheStore: cacheStore ?? LocalCacheStore(fileURL: nil, cachedState: cachedState)
        )
    }

    private func testTokenSet(account: GoogleAccount) -> CustomGoogleOAuthTokenSet {
        CustomGoogleOAuthTokenSet(
            accessToken: "\(account.id)-access",
            refreshToken: "\(account.id)-refresh",
            expiresAt: Date().addingTimeInterval(3_600),
            grantedScopes: account.grantedScopes,
            account: account,
            idToken: nil
        )
    }

    private func task(
        id: String,
        taskListID: String,
        dueDate: Date?,
        status: TaskStatus,
        isDeleted: Bool
    ) -> TaskMirror {
        TaskMirror(
            id: id,
            taskListID: taskListID,
            parentID: nil,
            title: id,
            notes: "",
            status: status,
            dueDate: dueDate,
            completedAt: status == .completed ? Date() : nil,
            isDeleted: isDeleted,
            isHidden: false,
            position: nil,
            etag: nil,
            updatedAt: nil
        )
    }

    private func event(
        id: String,
        calendarID: String,
        startDate: Date,
        status: CalendarEventStatus
    ) -> CalendarEventMirror {
        CalendarEventMirror(
            id: id,
            calendarID: calendarID,
            summary: id,
            details: "",
            startDate: startDate,
            endDate: startDate.addingTimeInterval(1800),
            isAllDay: false,
            status: status,
            recurrence: [],
            etag: nil,
            updatedAt: nil
        )
    }
}
