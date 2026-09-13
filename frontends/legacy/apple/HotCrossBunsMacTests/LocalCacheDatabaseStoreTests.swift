import XCTest
import CryptoKit
@testable import HotCrossBunsMac

final class LocalCacheDatabaseStoreTests: XCTestCase {
    private var tempDir: URL!
    private var jsonURL: URL!
    private var dbURL: URL!

    override func setUpWithError() throws {
        tempDir = URL(fileURLWithPath: NSTemporaryDirectory())
            .appending(path: "hcb-cache-db-tests-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: tempDir, withIntermediateDirectories: true)
        jsonURL = tempDir.appending(path: "cache-state.json")
        dbURL = tempDir.appending(path: "cache-state.sqlite")
    }

    override func tearDownWithError() throws {
        try? FileManager.default.removeItem(at: tempDir)
    }

    func testJSONSidecarMigrationPreservesStateAndWritesDatabase() async throws {
        let original = makeState(eventSuffix: "migrated", checkpointToken: "token-migrated")
        let legacyStore = LocalCacheStore(fileURL: jsonURL, storageBackend: .jsonSidecar)
        await legacyStore.save(original)

        let migratingStore = LocalCacheStore(fileURL: jsonURL)
        let loaded = await migratingStore.loadCachedState()

        XCTAssertTrue(FileManager.default.fileExists(atPath: jsonURL.path))
        XCTAssertTrue(FileManager.default.fileExists(atPath: tempDir.appending(path: "cache-events.json").path))
        XCTAssertTrue(FileManager.default.fileExists(atPath: dbURL.path))
        XCTAssertEqual(loaded.account, original.account)
        XCTAssertEqual(loaded.settings.syncMode, .nearRealtime)
        XCTAssertEqual(loaded.taskLists.map(\.id), original.taskLists.map(\.id))
        XCTAssertEqual(loaded.tasks.map(\.id), original.tasks.map(\.id))
        XCTAssertEqual(loaded.calendars.map(\.id), original.calendars.map(\.id))
        XCTAssertEqual(loaded.events.map(\.id), original.events.map(\.id))
        XCTAssertEqual(loaded.syncCheckpoints.first?.calendarSyncToken, "token-migrated")
        XCTAssertEqual(loaded.pendingMutations.first?.resourceID, "task-migrated")

        let reloader = LocalCacheStore(fileURL: jsonURL)
        let reloaded = await reloader.loadCachedState()
        XCTAssertEqual(reloaded.events.map(\.id), original.events.map(\.id))
        XCTAssertEqual(reloaded.syncCheckpoints.first?.calendarSyncToken, "token-migrated")
    }

    func testEmptySQLiteDatabaseMigratesLegacyJSONWithoutLoadWarning() async throws {
        let original = makeState(eventSuffix: "empty-db", checkpointToken: "token-empty-db")
        let legacyStore = LocalCacheStore(fileURL: jsonURL, storageBackend: .jsonSidecar)
        await legacyStore.save(original)
        _ = try LocalCacheDatabaseStore(fileURL: dbURL)

        let migratingStore = LocalCacheStore(fileURL: jsonURL)
        let loaded = await migratingStore.loadCachedState()

        XCTAssertEqual(loaded.events.map(\.id), ["event-empty-db"])
        XCTAssertEqual(loaded.syncCheckpoints.first?.calendarSyncToken, "token-empty-db")
        let warning = await migratingStore.lastLoadWarning
        XCTAssertNil(warning)

        let reloader = LocalCacheStore(fileURL: jsonURL)
        let reloaded = await reloader.loadCachedState()
        XCTAssertEqual(reloaded.events.map(\.id), ["event-empty-db"])
        XCTAssertEqual(reloaded.syncCheckpoints.first?.calendarSyncToken, "token-empty-db")
    }

    func testTransactionRollbackKeepsPriorEntitiesAndCheckpoints() throws {
        let store = try LocalCacheDatabaseStore(fileURL: dbURL)
        let original = makeState(eventSuffix: "before", checkpointToken: "token-before")
        let attempted = makeState(eventSuffix: "after", checkpointToken: "token-after")
        try store.save(original)

        XCTAssertThrowsError(
            try store.save(attempted, failAfterWritingEntitiesForTesting: true)
        ) { error in
            XCTAssertEqual(error as? LocalCacheDatabaseStore.CacheDatabaseError, .injectedRollback)
        }

        let loaded = try LocalCacheDatabaseStore(fileURL: dbURL).load()
        XCTAssertEqual(loaded.events.map(\.id), ["event-before"])
        XCTAssertEqual(loaded.syncCheckpoints.first?.calendarSyncToken, "token-before")
        XCTAssertNil(loaded.events.first { $0.id == "event-after" })
    }

    func testCheckpointDurabilitySharesEntityTransaction() throws {
        let state = makeState(eventSuffix: "durable", checkpointToken: "token-durable")
        let store = try LocalCacheDatabaseStore(fileURL: dbURL)

        try store.save(state)

        let reloaded = try LocalCacheDatabaseStore(fileURL: dbURL).load()
        XCTAssertEqual(reloaded.events.map(\.id), ["event-durable"])
        XCTAssertEqual(reloaded.syncCheckpoints.map(\.id), state.syncCheckpoints.map(\.id))
        XCTAssertEqual(reloaded.syncCheckpoints.first?.calendarSyncToken, "token-durable")
        XCTAssertEqual(try store.rowCountForTesting(table: "cache_events"), 1)
        XCTAssertEqual(try store.rowCountForTesting(table: "cache_sync_checkpoints"), 1)
    }

    func testIncrementalSyncApplySkipsUnchangedRowsAndUpdatesCheckpoint() throws {
        let unchanged = makeEvent(id: "event-keep")
        var changed = makeEvent(id: "event-change")
        let originalChanged = changed
        changed.summary = "Changed summary"
        changed.etag = "event-change-v2"
        changed.updatedAt = Date(timeIntervalSince1970: 1_700_010_000)

        let original = makeState(
            eventSuffix: "incremental",
            checkpointToken: "token-before",
            events: [unchanged, originalChanged]
        )
        let applied = makeState(
            eventSuffix: "incremental",
            checkpointToken: "token-after",
            events: [unchanged, changed]
        )
        let store = try LocalCacheDatabaseStore(fileURL: dbURL)
        try store.save(original)

        let unchangedHashBefore = try XCTUnwrap(store.contentHashForTesting(table: "cache_events", accountID: "account-1", id: "event-keep"))
        let changedHashBefore = try XCTUnwrap(store.contentHashForTesting(table: "cache_events", accountID: "account-1", id: "event-change"))
        let result = SyncApplyResult(
            state: applied,
            changeSet: incrementalEventChangeSet(
                updated: ["event-change"],
                unchanged: ["event-keep"],
                checkpointID: original.syncCheckpoints[0].id
            )
        )

        let profile = try store.applySyncResult(result)
        let unchangedHashAfter = try XCTUnwrap(store.contentHashForTesting(table: "cache_events", accountID: "account-1", id: "event-keep"))
        let changedHashAfter = try XCTUnwrap(store.contentHashForTesting(table: "cache_events", accountID: "account-1", id: "event-change"))
        let loaded = try LocalCacheDatabaseStore(fileURL: dbURL).load()

        XCTAssertEqual(unchangedHashAfter, unchangedHashBefore)
        XCTAssertNotEqual(changedHashAfter, changedHashBefore)
        XCTAssertGreaterThanOrEqual(profile.incrementalUnchangedSkippedRows, 1)
        XCTAssertEqual(profile.incrementalDeletedRows, 0)
        XCTAssertEqual(loaded.events.first { $0.id == "event-change" }?.summary, "Changed summary")
        XCTAssertEqual(loaded.syncCheckpoints.first?.calendarSyncToken, "token-after")
    }

    func testIncrementalSyncApplyRollsBackCheckpointWhenTransactionFails() throws {
        var changed = makeEvent(id: "event-rollback")
        let originalChanged = changed
        changed.summary = "Changed rollback"
        changed.etag = "event-rollback-v2"
        let original = makeState(
            eventSuffix: "rollback",
            checkpointToken: "token-before",
            events: [originalChanged]
        )
        let applied = makeState(
            eventSuffix: "rollback",
            checkpointToken: "token-after",
            events: [changed]
        )
        let store = try LocalCacheDatabaseStore(fileURL: dbURL)
        try store.save(original)

        let result = SyncApplyResult(
            state: applied,
            changeSet: incrementalEventChangeSet(
                updated: ["event-rollback"],
                unchanged: [],
                checkpointID: original.syncCheckpoints[0].id
            )
        )

        XCTAssertThrowsError(
            try store.applySyncResult(result, failAfterWritingEntitiesForTesting: true)
        ) { error in
            XCTAssertEqual(error as? LocalCacheDatabaseStore.CacheDatabaseError, .injectedRollback)
        }

        let loaded = try LocalCacheDatabaseStore(fileURL: dbURL).load()
        XCTAssertEqual(loaded.events.first { $0.id == "event-rollback" }?.summary, "Event event-rollback")
        XCTAssertEqual(loaded.syncCheckpoints.first?.calendarSyncToken, "token-before")
    }

    func testExplicitLocalEventEditCommitsOnlyTouchedEventAndPendingMutationRows() throws {
        let untouchedEvent = makeEvent(id: "event-keep")
        var changedEvent = makeEvent(id: "event-edit")
        let originalChangedEvent = changedEvent
        let task = makeTask(id: "task-keep")
        let original = makeState(
            eventSuffix: "explicit-event",
            checkpointToken: "token-before",
            events: [untouchedEvent, originalChangedEvent],
            tasks: [task],
            pendingMutations: []
        )
        let store = try LocalCacheDatabaseStore(fileURL: dbURL)
        try store.save(original)

        let untouchedEventHashBefore = try XCTUnwrap(store.contentHashForTesting(table: "cache_events", accountID: "account-1", id: untouchedEvent.id))
        let changedEventHashBefore = try XCTUnwrap(store.contentHashForTesting(table: "cache_events", accountID: "account-1", id: changedEvent.id))
        let taskHashBefore = try XCTUnwrap(store.contentHashForTesting(table: "cache_tasks", accountID: "account-1", id: task.id))

        changedEvent.summary = "Edited locally"
        changedEvent.etag = "event-edit-v2"
        let mutation = PendingMutation(
            id: UUID(uuidString: "22222222-2222-2222-2222-222222222222")!,
            accountID: "account-1",
            createdAt: Date(timeIntervalSince1970: 1_700_010_000),
            resourceType: .event,
            resourceID: changedEvent.id,
            action: .update,
            payload: Data(#"{"summary":"Edited locally"}"#.utf8)
        )
        let updated = makeState(
            eventSuffix: "explicit-event",
            checkpointToken: "token-before",
            events: [untouchedEvent, changedEvent],
            tasks: [task],
            pendingMutations: [mutation]
        )
        let changeSet = LocalCacheChangeSetBuilder.combined(
            LocalCacheChangeSetBuilder.eventUpdated(old: originalChangedEvent, new: changedEvent),
            LocalCacheChangeSetBuilder.pendingMutationInserted(mutation)
        )

        let profile = try store.commit(state: updated, changeSet: changeSet)

        let untouchedEventHashAfter = try XCTUnwrap(store.contentHashForTesting(table: "cache_events", accountID: "account-1", id: untouchedEvent.id))
        let changedEventHashAfter = try XCTUnwrap(store.contentHashForTesting(table: "cache_events", accountID: "account-1", id: changedEvent.id))
        let taskHashAfter = try XCTUnwrap(store.contentHashForTesting(table: "cache_tasks", accountID: "account-1", id: task.id))
        let pendingHash = try XCTUnwrap(store.contentHashForTesting(table: "cache_pending_mutations", accountID: "account-1", id: mutation.id.uuidString))

        XCTAssertEqual(untouchedEventHashAfter, untouchedEventHashBefore)
        XCTAssertNotEqual(changedEventHashAfter, changedEventHashBefore)
        XCTAssertEqual(taskHashAfter, taskHashBefore)
        XCTAssertFalse(pendingHash.isEmpty)
        XCTAssertEqual(try store.rowCountForTesting(table: "cache_pending_mutations"), 1)
        XCTAssertEqual(profile.incrementalDeletedRows, 0)
    }

    func testLocalEventChangeSetIncludesOldAndNewMovedMultiDayKeys() throws {
        let originalTimeZone = NSTimeZone.default
        NSTimeZone.default = TimeZone(identifier: "UTC")!
        defer { NSTimeZone.default = originalTimeZone }

        let movedOld = makeEvent(
            id: "event-moved",
            startDate: date(2026, 5, 1, hour: 10),
            endDate: date(2026, 5, 1, hour: 11)
        )
        let movedNew = makeEvent(
            id: "event-moved",
            startDate: date(2026, 5, 3, hour: 10),
            endDate: date(2026, 5, 3, hour: 11)
        )
        let movedChangeSet = LocalCacheChangeSetBuilder.eventUpdated(old: movedOld, new: movedNew)
        XCTAssertTrue(movedChangeSet.affectedDayKeys.contains(dayKey(2026, 5, 1)))
        XCTAssertTrue(movedChangeSet.affectedDayKeys.contains(dayKey(2026, 5, 3)))

        let multiOld = makeEvent(
            id: "event-multi",
            startDate: date(2026, 5, 10),
            endDate: date(2026, 5, 13),
            isAllDay: true
        )
        let multiNew = makeEvent(
            id: "event-multi",
            startDate: date(2026, 5, 12),
            endDate: date(2026, 5, 15),
            isAllDay: true
        )
        let multiChangeSet = LocalCacheChangeSetBuilder.eventUpdated(old: multiOld, new: multiNew)
        XCTAssertTrue(multiChangeSet.affectedDayKeys.contains(dayKey(2026, 5, 10)))
        XCTAssertTrue(multiChangeSet.affectedDayKeys.contains(dayKey(2026, 5, 11)))
        XCTAssertTrue(multiChangeSet.affectedDayKeys.contains(dayKey(2026, 5, 12)))
        XCTAssertTrue(multiChangeSet.affectedDayKeys.contains(dayKey(2026, 5, 13)))
        XCTAssertTrue(multiChangeSet.affectedDayKeys.contains(dayKey(2026, 5, 14)))
        XCTAssertFalse(multiChangeSet.affectedDayKeys.contains(dayKey(2026, 5, 15)))
    }

    func testExplicitTaskSchedulingChangeIncludesAffectedCalendarDaysAndBumpsRevisions() throws {
        let originalTimeZone = NSTimeZone.default
        NSTimeZone.default = TimeZone(identifier: "UTC")!
        defer { NSTimeZone.default = originalTimeZone }

        let oldDue = date(2026, 4, 18, hour: 9)
        let newDue = date(2026, 4, 20, hour: 9)
        var task = makeTask(id: "task-scheduled", dueDate: oldDue)
        let originalTask = task
        let original = makeState(
            eventSuffix: "task-schedule",
            checkpointToken: "token-before",
            events: [],
            tasks: [originalTask],
            pendingMutations: []
        )
        let store = try LocalCacheDatabaseStore(fileURL: dbURL)
        try store.save(original)

        let oldKey = dayKey(2026, 4, 18)
        let newKey = dayKey(2026, 4, 20)
        let oldRevisionBefore = try store.calendarDayRevisionForTesting(dayKey: oldKey)
        let newRevisionBefore = try store.calendarDayRevisionForTesting(dayKey: newKey)

        task.dueDate = newDue
        task.updatedAt = Date(timeIntervalSince1970: 1_700_011_000)
        let updated = makeState(
            eventSuffix: "task-schedule",
            checkpointToken: "token-before",
            events: [],
            tasks: [task],
            pendingMutations: []
        )
        let changeSet = LocalCacheChangeSetBuilder.taskUpdated(old: originalTask, new: task)

        XCTAssertTrue(changeSet.affectedDayKeys.contains(oldKey))
        XCTAssertTrue(changeSet.affectedDayKeys.contains(newKey))

        try store.commit(state: updated, changeSet: changeSet)

        XCTAssertGreaterThan(try store.calendarDayRevisionForTesting(dayKey: oldKey), oldRevisionBefore)
        XCTAssertGreaterThan(try store.calendarDayRevisionForTesting(dayKey: newKey), newRevisionBefore)
    }

    func testDerivedDiffFallbackSaveStillHandlesBroadAccountReplacement() async throws {
        let original = makeState(eventSuffix: "fallback-before", checkpointToken: "token-before")
        let replacementAccount = GoogleAccount(
            id: "account-2",
            email: "other@example.com",
            displayName: "Other",
            grantedScopes: [GoogleScope.tasks, GoogleScope.calendar],
            authProvider: .customDesktopOAuth
        )
        let replacement = CachedAppState(
            account: replacementAccount,
            accounts: [replacementAccount],
            activeAccountID: replacementAccount.id,
            taskLists: [TaskListMirror(id: "tasks-other", title: "Other", updatedAt: Date(timeIntervalSince1970: 1_700_020_000), etag: nil)],
            tasks: [makeTask(id: "task-other", dueDate: nil)],
            calendars: [CalendarListMirror(id: "calendar-other", summary: "Other", colorHex: "#00ACC1", isSelected: true, accessRole: "owner")],
            events: [makeEvent(id: "event-other")],
            settings: original.settings,
            syncCheckpoints: [],
            pendingMutations: []
        )
        let store = LocalCacheStore(fileURL: jsonURL)

        await store.save(original)
        await store.save(replacement)

        let reloader = LocalCacheStore(fileURL: jsonURL)
        let loaded = await reloader.loadCachedState()
        XCTAssertEqual(loaded.activeAccountID, replacementAccount.id)
        XCTAssertEqual(loaded.events.map(\.id), ["event-other"])
        XCTAssertEqual(loaded.tasks.map(\.id), ["task-other"])
    }

    func testStableRowHashUsesDeterministicSHA256AndIsStored() throws {
        let event = makeEvent(id: "event-hash")
        let state = makeState(eventSuffix: "hash", checkpointToken: "token-hash", events: [event])
        let persistedEvent = try XCTUnwrap(state.events.first { $0.id == event.id })
        let payload = try JSONEncoder.cacheDatabaseTestCanonical.encode(persistedEvent)
        let expected = SHA256.hash(data: payload).map { String(format: "%02x", $0) }.joined()

        let hash1 = try LocalCacheRowHasher.hash(persistedEvent, kind: "event")
        let hash2 = try LocalCacheRowHasher.hash(persistedEvent, kind: "event")
        XCTAssertEqual(hash1, expected)
        XCTAssertEqual(hash1, hash2)
        XCTAssertEqual(hash1.count, 64)

        let store = try LocalCacheDatabaseStore(fileURL: dbURL)
        try store.save(state)
        let stored = try store.contentHashForTesting(table: "cache_events", accountID: "account-1", id: "event-hash")
        XCTAssertEqual(stored, hash1)
    }

    func testCalendarAggregateCountsAndTagsUpdateIncrementally() throws {
        var literal = makeEvent(
            id: "event-literal",
            summary: "Planning #ops",
            colorId: CalendarEventColor.basil.rawValue
        )
        let bound = makeEvent(
            id: "event-bound",
            summary: "Roadmap",
            colorId: CalendarEventColor.sage.rawValue
        )
        let cancelled = makeEvent(
            id: "event-cancelled",
            summary: "Cancelled #ops",
            status: .cancelled,
            colorId: CalendarEventColor.tomato.rawValue
        )
        let store = try LocalCacheDatabaseStore(fileURL: dbURL)
        let original = makeState(
            eventSuffix: "aggregates",
            checkpointToken: "token-before",
            events: [literal, bound, cancelled]
        )
        try store.save(original)

        let initial = try store.calendarAggregateCounts(
            accountID: "account-1",
            selectedCalendarIDs: ["calendar-main"],
            colorTagBindings: [CalendarEventColor.sage.rawValue: "marketing"]
        )
        XCTAssertEqual(initial.eventCountsByCalendarID["calendar-main"], 2)
        XCTAssertEqual(initial.eventCountsByColorID[CalendarEventColor.basil.rawValue], 1)
        XCTAssertEqual(initial.eventCountsByColorID[CalendarEventColor.sage.rawValue], 1)
        XCTAssertEqual(initial.eventCountsByTagName["ops"], 1)
        XCTAssertEqual(initial.eventCountsByTagName["marketing"], 1)
        XCTAssertNil(initial.eventCountsByColorID[CalendarEventColor.tomato.rawValue])

        let includingCancelled = try store.calendarAggregateCounts(
            accountID: "account-1",
            selectedCalendarIDs: ["calendar-main"],
            includeCancelled: true
        )
        XCTAssertEqual(includingCancelled.eventCountsByCalendarID["calendar-main"], 3)
        XCTAssertEqual(includingCancelled.eventCountsByTagName["ops"], 2)
        XCTAssertEqual(includingCancelled.eventCountsByColorID[CalendarEventColor.tomato.rawValue], 1)

        literal.summary = "Planning #focus"
        literal.colorId = CalendarEventColor.sage.rawValue
        literal.etag = "event-literal-v2"
        let applied = makeState(
            eventSuffix: "aggregates",
            checkpointToken: "token-after",
            events: [literal, cancelled]
        )
        var changeSet = incrementalEventChangeSet(
            updated: ["event-literal"],
            unchanged: ["event-cancelled"],
            checkpointID: original.syncCheckpoints[0].id
        )
        changeSet.events.deleted = ["event-bound"]

        try store.applySyncResult(SyncApplyResult(state: applied, changeSet: changeSet))

        let updated = try store.calendarAggregateCounts(
            accountID: "account-1",
            selectedCalendarIDs: ["calendar-main"],
            colorTagBindings: [CalendarEventColor.sage.rawValue: "marketing"]
        )
        XCTAssertEqual(updated.eventCountsByCalendarID["calendar-main"], 1)
        XCTAssertEqual(updated.eventCountsByColorID[CalendarEventColor.sage.rawValue], 1)
        XCTAssertEqual(updated.eventCountsByTagName["focus"], 1)
        XCTAssertEqual(updated.eventCountsByTagName["marketing"], 1)
        XCTAssertNil(updated.eventCountsByTagName["ops"])
        XCTAssertEqual(try store.rowCountForTesting(table: "cache_calendar_event_index"), 2)
        XCTAssertEqual(try store.rowCountForTesting(table: "cache_calendar_event_days"), 2)

        try store.repairCalendarDerivedTables()
        let repaired = try store.calendarAggregateCounts(
            accountID: "account-1",
            selectedCalendarIDs: ["calendar-main"],
            colorTagBindings: [CalendarEventColor.sage.rawValue: "marketing"]
        )
        XCTAssertEqual(repaired, updated)
    }

    func testCalendarDayRevisionsAdvanceForMovedAndMultiDayEvents() throws {
        let originalTimeZone = NSTimeZone.default
        NSTimeZone.default = TimeZone(identifier: "UTC")!
        defer { NSTimeZone.default = originalTimeZone }

        let movedOriginal = makeEvent(
            id: "event-moved",
            startDate: date(2026, 4, 18, hour: 9),
            endDate: date(2026, 4, 18, hour: 10)
        )
        var movedUpdated = makeEvent(
            id: "event-moved",
            startDate: date(2026, 4, 19, hour: 9),
            endDate: date(2026, 4, 19, hour: 10)
        )
        movedUpdated.etag = "event-moved-v2"

        let multiOriginal = makeEvent(
            id: "event-multi",
            startDate: date(2026, 4, 20, hour: 9),
            endDate: date(2026, 4, 22, hour: 10)
        )
        var multiUpdated = makeEvent(
            id: "event-multi",
            startDate: date(2026, 4, 20, hour: 9),
            endDate: date(2026, 4, 23, hour: 10)
        )
        multiUpdated.etag = "event-multi-v2"

        let original = makeState(
            eventSuffix: "revision",
            checkpointToken: "token-before",
            events: [movedOriginal, multiOriginal]
        )
        let applied = makeState(
            eventSuffix: "revision",
            checkpointToken: "token-after",
            events: [movedUpdated, multiUpdated]
        )
        let store = try LocalCacheDatabaseStore(fileURL: dbURL)
        try store.save(original)

        let changeSet = incrementalEventChangeSet(
            updated: ["event-moved", "event-multi"],
            unchanged: [],
            checkpointID: original.syncCheckpoints[0].id
        )
        try store.applySyncResult(SyncApplyResult(state: applied, changeSet: changeSet))

        for day in [18, 19, 20, 21, 22, 23] {
            XCTAssertEqual(
                try store.calendarDayRevisionForTesting(dayKey: dayKey(2026, 4, day)),
                1,
                "expected day \(day) to be invalidated"
            )
        }
        XCTAssertGreaterThan(
            try store.calendarRangeRevisionForTesting(accountID: "account-1", kind: "month", key: "2026-4"),
            0
        )
    }

    func testFTSSearchIndexesInsertUpdateDeleteAndRanksTitleMatchesFirst() throws {
        let titleTask = makeTask(id: "task-title", title: "Launch review", notes: "")
        let notesTask = makeTask(id: "task-notes", title: "Routine admin", notes: "Launch review notes")
        let datedNoteTask = makeTask(id: "task-dated-note", title: "Follow up", notes: "Launch follow-up context")
        let titleEvent = makeEvent(id: "event-title", summary: "Launch briefing", details: "Agenda", location: "Room A")
        let detailsEvent = makeEvent(id: "event-details", summary: "Weekly sync", details: "Launch details")
        let store = try LocalCacheDatabaseStore(fileURL: dbURL)
        let original = makeState(
            eventSuffix: "fts",
            checkpointToken: "token-before",
            events: [titleEvent, detailsEvent],
            tasks: [titleTask, notesTask, datedNoteTask]
        )
        try store.save(original)

        let searchStarted = Date()
        let initial = try store.searchEntities(accountID: "account-1", query: "launch", limit: 10)
        XCTContext.runActivity(named: "FTS search latency: \(Date().timeIntervalSince(searchStarted) * 1000)ms") { _ in }
        XCTAssertEqual(initial.tasks.first?.id, "task-title")
        XCTAssertTrue(initial.tasks.contains { $0.id == "task-notes" })
        XCTAssertTrue(initial.tasks.contains { $0.id == "task-dated-note" })
        XCTAssertEqual(initial.events.first?.id, "event-title")
        XCTAssertTrue(initial.events.contains { $0.id == "event-details" })
        XCTAssertEqual(
            try store.searchEntities(accountID: "account-1", query: "kind:note", scope: .notes, limit: 10).tasks.map(\.id),
            ["task-dated-note", "task-notes"]
        )

        var renamedTask = notesTask
        renamedTask.title = "Budget review"
        renamedTask.notes = "Finance"
        var changeSet = SyncChangeSet.empty
        changeSet.tasks.updated = ["task-notes"]
        changeSet.tasks.unchanged = ["task-title", "task-dated-note"]
        changeSet.checkpoints.updated = [original.syncCheckpoints[0].id]
        changeSet.checkpointChanged = true
        let updatedState = makeState(
            eventSuffix: "fts",
            checkpointToken: "token-after",
            events: [titleEvent, detailsEvent],
            tasks: [titleTask, renamedTask, datedNoteTask]
        )
        try store.applySyncResult(SyncApplyResult(state: updatedState, changeSet: changeSet))

        let afterUpdate = try store.searchEntities(accountID: "account-1", query: "finance", limit: 10)
        XCTAssertEqual(afterUpdate.tasks.map(\.id), ["task-notes"])
        XCTAssertTrue(try store.searchEntities(accountID: "account-1", query: "launch", limit: 10).tasks.contains { $0.id == "task-notes" } == false)

        changeSet = SyncChangeSet.empty
        changeSet.tasks.deleted = ["task-notes"]
        changeSet.tasks.unchanged = ["task-title", "task-dated-note"]
        changeSet.events.deleted = ["event-details"]
        changeSet.events.unchanged = ["event-title"]
        changeSet.checkpoints.updated = [original.syncCheckpoints[0].id]
        changeSet.checkpointChanged = true
        let deletedState = makeState(
            eventSuffix: "fts",
            checkpointToken: "token-delete",
            events: [titleEvent],
            tasks: [titleTask, datedNoteTask]
        )
        try store.applySyncResult(SyncApplyResult(state: deletedState, changeSet: changeSet))

        let afterDelete = try store.searchEntities(accountID: "account-1", query: "finance", limit: 10)
        XCTAssertTrue(afterDelete.tasks.isEmpty)
        let detailsSearch = try store.searchEntities(accountID: "account-1", query: "details", limit: 10)
        XCTAssertTrue(detailsSearch.events.isEmpty)
    }

    func testOverdueTaskProjectionFiltersOrdersLimitsAndTaskListSelection() throws {
        let cutoff = date(2026, 5, 21)
        var otherListTask = makeTask(id: "task-other-list", title: "Other list", dueDate: date(2026, 5, 18))
        otherListTask.taskListID = "tasks-other"
        var hidden = makeTask(id: "task-hidden", title: "Hidden", dueDate: date(2026, 5, 19))
        hidden.isHidden = true

        let tasks = [
            makeTask(id: "task-future", title: "Future", dueDate: date(2026, 5, 22)),
            makeTask(id: "task-recent", title: "Recent", dueDate: date(2026, 5, 20)),
            makeTask(id: "task-beta", title: "Beta", dueDate: date(2026, 5, 19)),
            makeTask(id: "task-alpha", title: "Alpha", dueDate: date(2026, 5, 19)),
            makeTask(id: "task-completed", title: "Completed", dueDate: date(2026, 5, 19), completed: true),
            makeTask(id: "task-deleted", title: "Deleted", dueDate: date(2026, 5, 19), deleted: true),
            makeTask(id: "task-no-due", title: "No due", dueDate: nil),
            hidden,
            otherListTask
        ]
        let store = try LocalCacheDatabaseStore(fileURL: dbURL)
        try store.save(makeState(eventSuffix: "overdue", checkpointToken: "token-overdue", tasks: tasks))

        let allProjection = try store.overdueTasks(
            accountID: "account-1",
            before: cutoff,
            visibleTaskListIDs: nil,
            limit: 3
        )
        XCTAssertEqual(allProjection.totalCount, 4)
        XCTAssertEqual(allProjection.tasks.map(\.id), ["task-other-list", "task-alpha", "task-beta"])

        let selectedProjection = try store.overdueTasks(
            accountID: "account-1",
            before: cutoff,
            visibleTaskListIDs: ["tasks-main"],
            limit: -1
        )
        XCTAssertEqual(selectedProjection.totalCount, 3)
        XCTAssertEqual(selectedProjection.tasks.map(\.id), ["task-alpha", "task-beta", "task-recent"])

        let emptySelection = try store.overdueTasks(
            accountID: "account-1",
            before: cutoff,
            visibleTaskListIDs: [],
            limit: -1
        )
        XCTAssertEqual(emptySelection.totalCount, 0)
        XCTAssertTrue(emptySelection.tasks.isEmpty)
    }

    func testActionCenterSQLiteProjectionMatchesArrayBuilder() throws {
        let referenceDate = date(2026, 4, 18, hour: 9)
        let calendar = calendarUTC()
        let cutoff = calendar.startOfDay(for: referenceDate)
        let tasks = [
            makeTask(id: "task-older", title: "Older", dueDate: date(2026, 4, 14)),
            makeTask(id: "task-late", title: "Late", dueDate: date(2026, 4, 17)),
            makeTask(id: "task-today", title: "Today", dueDate: date(2026, 4, 18)),
            makeTask(id: "task-done", title: "Done", dueDate: date(2026, 4, 16), completed: true)
        ]
        var holdA = makeEvent(
            id: "hold-a",
            summary: "Planning",
            startDate: date(2026, 4, 19, hour: 9),
            endDate: date(2026, 4, 19, hour: 10)
        )
        holdA.availabilityHold = AvailabilityHoldMetadata(
            groupID: "group-a",
            title: "Planning",
            durationMinutes: 60,
            createdAt: referenceDate
        )
        var holdB = makeEvent(
            id: "hold-b",
            summary: "Design review",
            startDate: date(2026, 4, 20, hour: 9),
            endDate: date(2026, 4, 20, hour: 10)
        )
        holdB.availabilityHold = AvailabilityHoldMetadata(
            groupID: "group-b",
            title: "Design review",
            durationMinutes: 60,
            createdAt: referenceDate
        )
        var cancelledHold = makeEvent(
            id: "hold-cancelled",
            summary: "Ignore",
            startDate: date(2026, 4, 19, hour: 12),
            endDate: date(2026, 4, 19, hour: 13),
            status: .cancelled
        )
        cancelledHold.availabilityHold = AvailabilityHoldMetadata(
            groupID: "group-c",
            title: "Ignore",
            durationMinutes: 60,
            createdAt: referenceDate
        )
        let events = [holdB, holdA, cancelledHold, makeEvent(id: "normal")]
        let store = try LocalCacheDatabaseStore(fileURL: dbURL)
        try store.save(makeState(
            eventSuffix: "action-center",
            checkpointToken: "token-action-center",
            events: events,
            tasks: tasks
        ))

        let overdue = try store.overdueTasks(
            accountID: "account-1",
            before: cutoff,
            visibleTaskListIDs: ["tasks-main"],
            limit: 1
        )
        let holdGroups = try store.availabilityHoldGroups(accountID: "account-1")
            .map { group in
                ActionCenterHoldGroup(id: group.id, metadata: group.metadata, events: group.events)
            }
        let querySnapshot = ActionCenterBuilder.build(
            holdGroups: holdGroups,
            overdueTasks: overdue.tasks,
            overdueTaskCount: overdue.totalCount,
            pendingMutations: [],
            notificationSummary: nil,
            authState: .signedOut,
            syncState: .idle,
            isSyncPaused: false,
            mutationError: nil,
            syncFailureKind: nil,
            networkReachability: .online
        )
        let arraySnapshot = ActionCenterBuilder.build(
            tasks: tasks,
            events: events,
            pendingMutations: [],
            notificationSummary: nil,
            authState: .signedOut,
            syncState: .idle,
            isSyncPaused: false,
            mutationError: nil,
            syncFailureKind: nil,
            networkReachability: .online,
            referenceDate: referenceDate,
            calendar: calendar,
            overdueTaskDisplayLimit: 1
        )

        XCTAssertEqual(querySnapshot.holdGroups, arraySnapshot.holdGroups)
        XCTAssertEqual(querySnapshot.overdueTasks.map(\.id), arraySnapshot.overdueTasks.map(\.id))
        XCTAssertEqual(querySnapshot.overdueTaskCount, arraySnapshot.overdueTaskCount)
        XCTAssertEqual(querySnapshot.actionableCount, arraySnapshot.actionableCount)
    }

    func testCalendarProjectionDoesNotDecodeFullEventPayloads() throws {
        let originalTimeZone = NSTimeZone.default
        NSTimeZone.default = TimeZone(identifier: "UTC")!
        defer { NSTimeZone.default = originalTimeZone }

        let event = makeEvent(
            id: "event-projection",
            summary: "Projection review",
            startDate: date(2026, 4, 18, hour: 9),
            endDate: date(2026, 4, 18, hour: 10)
        )
        let task = makeTask(
            id: "task-projection",
            title: "Projection task",
            notes: "large notes not needed by grid",
            dueDate: date(2026, 4, 18)
        )
        let store = try LocalCacheDatabaseStore(fileURL: dbURL)
        try store.save(makeState(
            eventSuffix: "projection",
            checkpointToken: "token-projection",
            events: [event],
            tasks: [task]
        ))
        try store.corruptPayloadForTesting(table: "cache_events", accountID: "account-1", id: event.id)
        try store.corruptPayloadForTesting(table: "cache_tasks", accountID: "account-1", id: task.id)

        let projection = try store.calendarVisibleRangeProjection(
            accountID: "account-1",
            kind: .day,
            anchorDate: date(2026, 4, 18),
            selectedCalendarIDs: ["calendar-main"],
            calendar: calendarUTC()
        )

        XCTAssertEqual(projection.eventsByDay[dayKey(2026, 4, 18)], [event.id])
        XCTAssertEqual(projection.eventByID[event.id]?.summary, "Projection review")
        XCTAssertEqual(projection.eventByID[event.id]?.details, "")
        XCTAssertTrue(projection.eventSearchTextByID[event.id]?.contains("Details event-projection") == true)
        XCTAssertEqual(projection.tasksByDueDate[dayKey(2026, 4, 18)], [task.id])
        XCTAssertEqual(projection.taskByID[task.id]?.title, "Projection task")
        XCTAssertEqual(projection.taskByID[task.id]?.notes, "")
        XCTAssertTrue(projection.taskSearchTextByID[task.id]?.contains("large notes") == true)
    }

    func testSideEffectDirtyQueueEnqueueProcessDeleteAndBenchmark() throws {
        let task = makeTask(id: "task-dirty", title: "Dirty queue task")
        let event = makeEvent(id: "event-dirty", summary: "Dirty queue event")
        let store = try LocalCacheDatabaseStore(fileURL: dbURL)
        let original = makeState(
            eventSuffix: "dirty",
            checkpointToken: "token-before",
            events: [event],
            tasks: [task]
        )
        try store.save(original)
        let initialSpotlightItems = try store.sideEffectDirtyItems(target: .spotlight)
        try store.markSideEffectDirtyItemsProcessed(initialSpotlightItems)
        let initialNotificationItems = try store.sideEffectDirtyItems(target: .notification)
        try store.markSideEffectDirtyItemsProcessed(initialNotificationItems)

        var updatedTask = task
        updatedTask.title = "Dirty queue task updated"
        var changeSet = SyncChangeSet.empty
        changeSet.tasks.updated = [task.id]
        changeSet.events.deleted = [event.id]
        changeSet.checkpoints.updated = [original.syncCheckpoints[0].id]
        changeSet.checkpointChanged = true
        let updatedState = makeState(
            eventSuffix: "dirty",
            checkpointToken: "token-after",
            events: [],
            tasks: [updatedTask]
        )

        let started = Date()
        try store.applySyncResult(SyncApplyResult(state: updatedState, changeSet: changeSet))
        let elapsed = Date().timeIntervalSince(started) * 1000
        XCTContext.runActivity(named: "Post-sync side-effect dirty enqueue elapsed: \(elapsed)ms") { _ in }

        let spotlightItems = try store.sideEffectDirtyItems(target: .spotlight)
        XCTAssertEqual(Set(spotlightItems.compactMap(\.resourceID)), [task.id, event.id])
        XCTAssertTrue(spotlightItems.contains { $0.resourceType == .task && $0.operation == .upsert })
        XCTAssertTrue(spotlightItems.contains { $0.resourceType == .event && $0.operation == .delete })

        try store.markSideEffectDirtyItemsProcessed(spotlightItems.filter { $0.resourceType == .task })
        let remaining = try store.sideEffectDirtyItems(target: .spotlight)
        XCTAssertEqual(remaining.map(\.resourceID), [event.id])

        try store.markSideEffectDirtyItemsProcessed(remaining)
        XCTAssertTrue(try store.sideEffectDirtyItems(target: .spotlight).isEmpty)
        XCTAssertEqual(try store.rowCountForTesting(table: "cache_side_effect_dirty_queue"), 2)
    }

    private func makeState(
        eventSuffix: String,
        checkpointToken: String,
        events explicitEvents: [CalendarEventMirror]? = nil,
        tasks explicitTasks: [TaskMirror]? = nil,
        pendingMutations explicitPendingMutations: [PendingMutation]? = nil
    ) -> CachedAppState {
        let account = GoogleAccount(
            id: "account-1",
            email: "person@example.com",
            displayName: "Person",
            grantedScopes: [GoogleScope.tasks, GoogleScope.calendar],
            authProvider: .customDesktopOAuth
        )
        let taskList = TaskListMirror(
            id: "tasks-main",
            title: "Inbox",
            updatedAt: Date(timeIntervalSince1970: 1_700_000_000),
            etag: "task-list-etag"
        )
        let task = TaskMirror(
            id: "task-\(eventSuffix)",
            taskListID: taskList.id,
            parentID: nil,
            title: "Task \(eventSuffix)",
            notes: "Queued locally",
            status: .needsAction,
            dueDate: Date(timeIntervalSince1970: 1_700_086_400),
            completedAt: nil,
            isDeleted: false,
            isHidden: false,
            position: "0001",
            etag: "task-etag-\(eventSuffix)",
            updatedAt: Date(timeIntervalSince1970: 1_700_000_100)
        )
        let calendar = CalendarListMirror(
            id: "calendar-main",
            summary: "Calendar",
            colorHex: "#448AFF",
            isSelected: true,
            accessRole: "owner",
            etag: "calendar-etag",
            defaultReminderMinutes: [10],
            timeZoneID: "UTC"
        )
        let events = explicitEvents ?? [makeEvent(id: "event-\(eventSuffix)")]
        let tasks = explicitTasks ?? [task]
        var settings = AppSettings.default
        settings.syncMode = .nearRealtime
        settings.selectedCalendarIDs = [calendar.id]
        settings.hasConfiguredCalendarSelection = true
        settings.selectedTaskListIDs = [taskList.id]
        settings.hasConfiguredTaskListSelection = true
        let checkpoint = SyncCheckpoint(
            id: SyncCheckpoint.stableID(accountID: account.id, resourceType: .calendar, resourceID: calendar.id),
            accountID: account.id,
            resourceType: .calendar,
            resourceID: calendar.id,
            calendarSyncToken: checkpointToken,
            tasksUpdatedMin: nil,
            lastSuccessfulSyncAt: Date(timeIntervalSince1970: 1_700_001_000)
        )
        let mutation = PendingMutation(
            id: UUID(uuidString: "11111111-1111-1111-1111-111111111111")!,
            accountID: account.id,
            createdAt: Date(timeIntervalSince1970: 1_700_002_000),
            resourceType: .task,
            resourceID: task.id,
            action: .update,
            payload: Data(#"{"title":"Local"}"#.utf8)
        )

        return CachedAppState(
            account: account,
            accounts: [account],
            activeAccountID: account.id,
            taskLists: [taskList],
            tasks: tasks,
            calendars: [calendar],
            events: events,
            settings: settings,
            syncCheckpoints: [checkpoint],
            pendingMutations: explicitPendingMutations ?? [mutation]
        )
    }

    private func makeEvent(
        id: String,
        summary: String? = nil,
        details: String? = nil,
        startDate: Date = Date(timeIntervalSince1970: 1_700_003_000),
        endDate: Date = Date(timeIntervalSince1970: 1_700_006_600),
        isAllDay: Bool = false,
        status: CalendarEventStatus = .confirmed,
        location: String = "Room 1",
        colorId: String? = CalendarEventColor.basil.rawValue
    ) -> CalendarEventMirror {
        CalendarEventMirror(
            id: id,
            calendarID: "calendar-main",
            summary: summary ?? "Event \(id)",
            details: details ?? "Details \(id)",
            startDate: startDate,
            endDate: endDate,
            isAllDay: isAllDay,
            status: status,
            recurrence: [],
            etag: "event-etag-\(id)",
            updatedAt: Date(timeIntervalSince1970: 1_700_004_000),
            reminderMinutes: [10],
            location: location,
            attendeeEmails: ["person@example.com"],
            colorId: colorId,
            startTimeZoneID: "UTC",
            endTimeZoneID: "UTC"
        )
    }

    private func makeTask(
        id: String,
        title: String? = nil,
        notes: String = "",
        dueDate: Date? = Date(timeIntervalSince1970: 1_700_086_400),
        completed: Bool = false,
        deleted: Bool = false
    ) -> TaskMirror {
        TaskMirror(
            id: id,
            taskListID: "tasks-main",
            parentID: nil,
            title: title ?? "Task \(id)",
            notes: notes,
            status: completed ? .completed : .needsAction,
            dueDate: dueDate,
            completedAt: completed ? Date(timeIntervalSince1970: 1_700_090_000) : nil,
            isDeleted: deleted,
            isHidden: false,
            position: "0001",
            etag: "task-etag-\(id)",
            updatedAt: Date(timeIntervalSince1970: 1_700_000_100)
        )
    }

    private func date(_ year: Int, _ month: Int, _ day: Int, hour: Int = 0) -> Date {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC")!
        return calendar.date(from: DateComponents(year: year, month: month, day: day, hour: hour))!
    }

    private func dayKey(_ year: Int, _ month: Int, _ day: Int) -> TimeInterval {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC")!
        return calendar.startOfDay(for: date(year, month, day)).timeIntervalSinceReferenceDate
    }

    private func calendarUTC() -> Calendar {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC")!
        return calendar
    }

    private func incrementalEventChangeSet(
        updated: Set<String>,
        unchanged: Set<String>,
        checkpointID: String
    ) -> SyncChangeSet {
        var changeSet = SyncChangeSet.empty
        changeSet.events.updated = updated
        changeSet.events.unchanged = unchanged
        changeSet.checkpoints.updated = [checkpointID]
        changeSet.checkpointChanged = true
        changeSet.affectedCalendarIDs = ["calendar-main"]
        changeSet.affectedDayKeys = [
            Calendar.current.startOfDay(for: Date(timeIntervalSince1970: 1_700_003_000)).timeIntervalSinceReferenceDate
        ]
        return changeSet
    }
}

private extension JSONEncoder {
    static var cacheDatabaseTestCanonical: JSONEncoder {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .secondsSince1970
        encoder.outputFormatting = [.sortedKeys]
        return encoder
    }
}
