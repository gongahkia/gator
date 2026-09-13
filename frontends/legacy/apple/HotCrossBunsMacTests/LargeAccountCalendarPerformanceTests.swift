import XCTest
@testable import HotCrossBunsMac

final class LargeAccountCalendarPerformanceTests: XCTestCase {
    private let calendar = LargeAccountCalendarFixture.calendar

    override func tearDown() {
        MockURLProtocol.reset()
        super.tearDown()
    }

    func testLargeAccountFixtureContainsRegressionCases() {
        let state = LargeAccountCalendarFixture.makeState(eventCount: 15_000)
        let denseDay = LargeAccountCalendarFixture.denseDay
        let denseDayEnd = calendar.date(byAdding: .day, value: 1, to: denseDay)!
        let denseEvents = state.events.filter { $0.startDate < denseDayEnd && $0.endDate > denseDay }

        XCTAssertEqual(state.events.count, 15_000)
        XCTAssertGreaterThanOrEqual(denseEvents.count, 400)
        XCTAssertTrue(state.events.contains { $0.recurrence.isEmpty == false })
        XCTAssertTrue(state.events.contains { $0.isAllDay && $0.endDate.timeIntervalSince($0.startDate) > 86_400 })
        XCTAssertTrue(state.events.contains { $0.status == .cancelled })
        XCTAssertTrue(state.events.contains { $0.details.isEmpty == false && $0.location.isEmpty == false })
    }

    func testLocalCacheSidecarRoundTripsLargeAccountBenchmark() async throws {
        let tempDir = URL(fileURLWithPath: NSTemporaryDirectory())
            .appending(path: "hcb-large-cache-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: tempDir, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: tempDir) }

        let mainFile = tempDir.appending(path: "cache-state.json")
        let eventsFile = tempDir.appending(path: "cache-events.json")
        let state = LargeAccountCalendarFixture.makeState(eventCount: 15_000)
        let store = LocalCacheStore(fileURL: mainFile, storageBackend: .jsonSidecar)

        let (saveProfile, saveMs) = try await timedAsync("cache.save.15k") {
            try await store.profileSaveForBenchmark(state)
        }

        let mainData = try Data(contentsOf: mainFile)
        let mainDecoded = try JSONDecoder.largeAccountCache.decode(CachedAppState.self, from: mainData)
        XCTAssertTrue(mainDecoded.events.isEmpty, "large event payloads must stay in the split sidecar")
        XCTAssertTrue(FileManager.default.fileExists(atPath: eventsFile.path))

        let reloader = LocalCacheStore(fileURL: mainFile, storageBackend: .jsonSidecar)
        let (loadResult, loadMs) = try await timedAsync("cache.load.15k") {
            try await reloader.profileLoadCachedStateForBenchmark()
        }
        let loaded = loadResult.state
        let loadProfile = loadResult.profile

        let sidecarBytes = try Data(contentsOf: eventsFile).count
        print("HCBLargeAccountBenchmark cache.sidecarBytes=\(sidecarBytes) saveMs=\(format(saveMs)) loadMs=\(format(loadMs))")
        print("HCBLargeAccountBenchmark cache.saveBreakdown hashMs=\(format(saveProfile.eventsHashMilliseconds)) stripMs=\(format(saveProfile.stripEventsMilliseconds)) snapshotMs=\(format(saveProfile.snapshotRotationMilliseconds)) mainEncodeMs=\(format(saveProfile.mainEncodeMilliseconds)) mainEncryptMs=\(format(saveProfile.mainEncryptMilliseconds)) mainWriteMs=\(format(saveProfile.mainWriteMilliseconds)) sidecarWrite=\(saveProfile.sidecarShouldWrite) sidecarEncodeMs=\(format(saveProfile.sidecarEncodeMilliseconds)) sidecarEncryptMs=\(format(saveProfile.sidecarEncryptMilliseconds)) sidecarWriteMs=\(format(saveProfile.sidecarWriteMilliseconds)) totalMs=\(format(saveProfile.totalMilliseconds))")
        print("HCBLargeAccountBenchmark cache.loadBreakdown mainReadMs=\(format(loadProfile.mainReadMilliseconds)) mainEnvelopeMs=\(format(loadProfile.mainEnvelopeDecodeMilliseconds)) mainDecryptMs=\(format(loadProfile.mainDecryptMilliseconds)) mainDecodeMs=\(format(loadProfile.mainDecodeMilliseconds)) sidecarReadMs=\(format(loadProfile.sidecarReadMilliseconds)) sidecarEnvelopeMs=\(format(loadProfile.sidecarEnvelopeDecodeMilliseconds)) sidecarDecryptMs=\(format(loadProfile.sidecarDecryptMilliseconds)) sidecarPayloadDecodeMs=\(format(loadProfile.sidecarPayloadDecodeMilliseconds)) sidecarLegacyDecodeMs=\(format(loadProfile.sidecarLegacyDecodeMilliseconds)) sidecarApplyMs=\(format(loadProfile.sidecarApplyMilliseconds)) sidecarFormat=\(loadProfile.sidecarFormat.rawValue) fallbackMs=\(format(loadProfile.fallbackRecoveryMilliseconds)) totalMs=\(format(loadProfile.totalMilliseconds))")
        XCTAssertEqual(loaded.events.count, state.events.count)
        XCTAssertEqual(loaded.events.first?.id, state.events.first?.id)
        XCTAssertEqual(loaded.events.last?.id, state.events.last?.id)
    }

    func testLocalCacheDatabaseRoundTripsLargeAccountBenchmark() throws {
        let tempDir = URL(fileURLWithPath: NSTemporaryDirectory())
            .appending(path: "hcb-large-db-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: tempDir, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: tempDir) }

        let dbFile = tempDir.appending(path: "cache-state.sqlite")
        let state = LargeAccountCalendarFixture.makeState(eventCount: 15_000)
        let store = try LocalCacheDatabaseStore(fileURL: dbFile)

        let (saveProfile, saveMs) = try timed("cache.db.write.15k") {
            try store.save(state)
        }

        let reloader = try LocalCacheDatabaseStore(fileURL: dbFile)
        let (loadResult, loadMs) = try timed("cache.db.load.15k") {
            try reloader.loadProfiled()
        }
        let loaded = loadResult.state
        let loadProfile = loadResult.profile
        let dbBytes = (try? dbFile.resourceValues(forKeys: [.fileSizeKey]))?.fileSize ?? 0

        print("HCBLargeAccountBenchmark cache.dbBytes=\(dbBytes) saveMs=\(format(saveMs)) loadMs=\(format(loadMs))")
        print("HCBLargeAccountBenchmark cache.dbWriteBreakdown accounts=\(saveProfile.accountRows) workspaces=\(saveProfile.workspaceRows) taskLists=\(saveProfile.taskListRows) tasks=\(saveProfile.taskRows) calendars=\(saveProfile.calendarRows) events=\(saveProfile.eventRows) checkpoints=\(saveProfile.checkpointRows) pending=\(saveProfile.pendingMutationRows) encodeHashMs=\(format(saveProfile.encodeAndHashMilliseconds)) txMs=\(format(saveProfile.transactionMilliseconds)) totalMs=\(format(saveProfile.totalMilliseconds))")
        print("HCBLargeAccountBenchmark cache.dbLoadBreakdown accounts=\(loadProfile.accountRows) workspaces=\(loadProfile.workspaceRows) taskLists=\(loadProfile.taskListRows) tasks=\(loadProfile.taskRows) calendars=\(loadProfile.calendarRows) events=\(loadProfile.eventRows) checkpoints=\(loadProfile.checkpointRows) pending=\(loadProfile.pendingMutationRows) fetchDecodeMs=\(format(loadProfile.fetchAndDecodeMilliseconds)) rebuildMs=\(format(loadProfile.rebuildStateMilliseconds)) totalMs=\(format(loadProfile.totalMilliseconds))")
        XCTAssertEqual(loaded.events.count, state.events.count)
        XCTAssertEqual(loaded.events.first?.id, state.events.first?.id)
        XCTAssertEqual(loaded.events.last?.id, state.events.last?.id)
        XCTAssertEqual(try store.rowCountForTesting(table: "cache_events"), 15_000)
        XCTAssertLessThan(saveMs, 3_000, "15k-event SQLite full rebuild should stay under 3s")
    }

    func testLocalCacheDatabaseOneEventCommitLargeAccountBenchmark() async throws {
        let tempDir = URL(fileURLWithPath: NSTemporaryDirectory())
            .appending(path: "hcb-large-db-commit-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: tempDir, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: tempDir) }

        let cacheFile = tempDir.appending(path: "cache-state.json")
        let dbFile = tempDir.appending(path: "cache-state.sqlite")
        let state = LargeAccountCalendarFixture.makeState(eventCount: 15_000)
        let store = LocalCacheStore(fileURL: cacheFile)
        await store.save(state)

        let databaseBefore = try LocalCacheDatabaseStore(fileURL: dbFile)
        let untouchedID = state.events.first { $0.id != state.events[state.events.count / 2].id }!.id
        let changedID = state.events[state.events.count / 2].id
        let untouchedHashBefore = try XCTUnwrap(databaseBefore.contentHashForTesting(table: "cache_events", accountID: "large-account", id: untouchedID))
        let changedHashBefore = try XCTUnwrap(databaseBefore.contentHashForTesting(table: "cache_events", accountID: "large-account", id: changedID))

        var updatedEvents = state.events
        let originalChangedEvent = updatedEvents[state.events.count / 2]
        updatedEvents[state.events.count / 2].summary += " local edit"
        updatedEvents[state.events.count / 2].etag = "local-edit-v2"
        let updatedChangedEvent = updatedEvents[state.events.count / 2]
        let updated = CachedAppState(
            account: state.account,
            accounts: state.accounts,
            activeAccountID: state.activeAccountID,
            taskLists: state.taskLists,
            tasks: state.tasks,
            calendars: state.calendars,
            events: updatedEvents,
            settings: state.settings,
            syncCheckpoints: state.syncCheckpoints,
            pendingMutations: state.pendingMutations,
            schemaVersion: state.schemaVersion
        )

        let (_, commitMs) = try await timedAsync("cache.db.commitOneEvent.15k") {
            try await store.commit(
                state: updated,
                changeSet: LocalCacheChangeSetBuilder.eventUpdated(old: originalChangedEvent, new: updatedChangedEvent)
            )
        }

        let databaseAfter = try LocalCacheDatabaseStore(fileURL: dbFile)
        let untouchedHashAfter = try XCTUnwrap(databaseAfter.contentHashForTesting(table: "cache_events", accountID: "large-account", id: untouchedID))
        let changedHashAfter = try XCTUnwrap(databaseAfter.contentHashForTesting(table: "cache_events", accountID: "large-account", id: changedID))
        XCTAssertEqual(untouchedHashAfter, untouchedHashBefore)
        XCTAssertNotEqual(changedHashAfter, changedHashBefore)
        XCTAssertEqual(try databaseAfter.rowCountForTesting(table: "cache_events"), 15_000)
        XCTAssertLessThan(commitMs, 200, "one-event SQLite local commit should stay under 200ms")
    }

    func testCalendarVisibleRangeDatabaseQueriesLargeAccountBenchmark() throws {
        let originalTimeZone = NSTimeZone.default
        NSTimeZone.default = TimeZone(identifier: "UTC")!
        defer { NSTimeZone.default = originalTimeZone }

        let tempDir = URL(fileURLWithPath: NSTemporaryDirectory())
            .appending(path: "hcb-visible-range-db-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: tempDir, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: tempDir) }

        let state = LargeAccountCalendarFixture.makeState(eventCount: 15_000)
        let store = try LocalCacheDatabaseStore(fileURL: tempDir.appending(path: "cache-state.sqlite"))
        try store.save(state)
        let selectedCalendarIDs = Set(state.calendars.map(\.id))

        let (day, dayMs) = try timed("cache.db.visibleRange.day.15k") {
            try store.calendarVisibleRangeProjection(
                accountID: "large-account",
                kind: .day,
                anchorDate: LargeAccountCalendarFixture.denseDay,
                selectedCalendarIDs: selectedCalendarIDs,
                calendar: calendar
            )
        }
        let (week, weekMs) = try timed("cache.db.visibleRange.week.15k") {
            try store.calendarVisibleRangeProjection(
                accountID: "large-account",
                kind: .week,
                anchorDate: LargeAccountCalendarFixture.denseDay,
                selectedCalendarIDs: selectedCalendarIDs,
                calendar: calendar
            )
        }
        let (month, monthMs) = try timed("cache.db.visibleRange.month.15k") {
            try store.calendarVisibleRangeProjection(
                accountID: "large-account",
                kind: .month,
                anchorDate: LargeAccountCalendarFixture.denseDay,
                selectedCalendarIDs: selectedCalendarIDs,
                calendar: calendar
            )
        }
        let (agenda, agendaMs) = try timed("cache.db.visibleRange.agenda.15k") {
            try store.calendarVisibleRangeProjection(
                accountID: "large-account",
                kind: .agenda,
                anchorDate: LargeAccountCalendarFixture.denseDay,
                dayCount: 14,
                selectedCalendarIDs: selectedCalendarIDs,
                calendar: calendar
            )
        }
        let (year, yearMs) = try timed("cache.db.visibleRange.year.15k") {
            try store.calendarVisibleRangeProjection(
                accountID: "large-account",
                kind: .year,
                anchorDate: LargeAccountCalendarFixture.denseDay,
                selectedCalendarIDs: selectedCalendarIDs,
                calendar: calendar
            )
        }

        let yearEventCount = year.eventByID.isEmpty
            ? year.eventCountsByDay.values.reduce(0, +)
            : year.eventByID.count
        print("HCBLargeAccountBenchmark cache.dbVisibleRangeCounts dayEvents=\(day.eventByID.count) weekEvents=\(week.eventByID.count) monthEvents=\(month.eventByID.count) agendaEvents=\(agenda.eventByID.count) yearEvents=\(yearEventCount) dayMs=\(format(dayMs)) weekMs=\(format(weekMs)) monthMs=\(format(monthMs)) agendaMs=\(format(agendaMs)) yearMs=\(format(yearMs))")
        XCTAssertGreaterThan(day.eventByID.count, 300)
        XCTAssertGreaterThan(week.eventByID.count, day.eventByID.count)
        XCTAssertGreaterThan(month.eventByID.count, week.eventByID.count)
        XCTAssertGreaterThan(agenda.eventByID.count, week.eventByID.count)
        XCTAssertGreaterThan(yearEventCount, month.eventByID.count)
        XCTAssertLessThan(yearMs, 150, "year visible-range DB query should stay under 150ms")
    }

    func testCalendarRenderingSourcesDoNotReintroduceFullEventScans() throws {
        let appleDir = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
        let calendarDir = appleDir.appending(path: "HotCrossBuns/Features/Calendar")
        let files = [
            "CalendarHomeView.swift",
            "DayGridView.swift",
            "WeekGridView.swift",
            "MonthGridView.swift",
            "YearGridView.swift"
        ]
        let forbidden = [
            "calendarStore.events.filter",
            "calendarStore.events.contains",
            "events: calendarStore.events",
            "return calendarStore.events"
        ]

        for file in files {
            let source = try String(contentsOf: calendarDir.appending(path: file), encoding: .utf8)
            for pattern in forbidden {
                XCTAssertFalse(
                    source.contains(pattern),
                    "\(file) reintroduced a full calendarStore.events scan via \(pattern)"
                )
            }
        }
    }

    func testCalendarSnapshotPrewarmStoresAdjacentSnapshotsWithoutChangingVisibleSnapshot() throws {
        let appleDir = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
        let calendarDir = appleDir.appending(path: "HotCrossBuns/Features/Calendar")
        let expectations = [
            ("DayGridView.swift", "prewarmAdjacentDaySnapshots", "storeCalendarDaySnapshot", "preparedDaySnapshot = snapshot"),
            ("WeekGridView.swift", "prewarmAdjacentWeekSnapshots", "storeCalendarWeekSnapshot", "preparedWeekSnapshot = snapshot"),
            ("CalendarHomeView.swift", "prewarmAdjacentAgendaSnapshots", "storeCalendarAgendaSnapshot", "preparedAgendaSnapshot = snapshot"),
            ("YearGridView.swift", "prewarmAdjacentYearSnapshots", "storeCalendarYearSnapshot", "preparedYearSnapshot = snapshot")
        ]

        for (file, functionName, storeCall, visibleAssignment) in expectations {
            let source = try String(contentsOf: calendarDir.appending(path: file), encoding: .utf8)
            let body = try XCTUnwrap(functionBody(named: functionName, in: source), "\(file) missing \(functionName)")
            XCTAssertTrue(body.contains("performanceMode == .snappy"), "\(file) should only prewarm in snappy mode")
            XCTAssertTrue(body.contains(storeCall), "\(file) should store adjacent snapshots in the LRU cache")
            XCTAssertFalse(body.contains(visibleAssignment), "\(file) prewarm must not mutate the visible snapshot")
        }
    }

    func testWeekHoverPreviewIsRichModeOnly() throws {
        let appleDir = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
        let source = try String(
            contentsOf: appleDir.appending(path: "HotCrossBuns/Features/Calendar/WeekGridView.swift"),
            encoding: .utf8
        )

        XCTAssertTrue(source.contains("isWeekHoverPreviewEnabled"))
        XCTAssertTrue(source.contains("performanceMode == .rich"))
        XCTAssertTrue(source.contains("EventHoverPreviewModifier(event: placed.event, isEnabled: isWeekHoverPreviewEnabled)"))
    }

    @MainActor
    func testAppModelLargeCacheLoadBuildsDerivedIndexesBenchmark() async throws {
        let state = LargeAccountCalendarFixture.makeState(eventCount: 15_000)
        let model = makeModel(cachedState: state)

        let (_, loadMs) = await timedAsync("appModel.loadInitialState.15k") {
            await model.loadInitialState()
        }
        XCTAssertTrue(
            (model.lastApplyProfileForBenchmark?.rebuildSnapshotsMilliseconds ?? .greatestFiniteMagnitude) < 25,
            "cold cache load should assign cached state without doing the heavy derived rebuild inline"
        )
        try await waitForDerivedSnapshotRebuild(model)

        let activeEventCount = state.events.filter { $0.status != .cancelled }.count
        let eventDayReferences = model.eventsByDay.values.reduce(0) { $0 + $1.count }
        print("HCBLargeAccountBenchmark appModel.events=\(model.events.count) activeEvents=\(activeEventCount) eventDayRefs=\(eventDayReferences) loadMs=\(format(loadMs))")
        if let profile = model.lastApplyProfileForBenchmark {
            print("HCBLargeAccountBenchmark appModel.applyBreakdown diffMs=\(format(profile.diffSetupMilliseconds)) assignmentMs=\(format(profile.assignmentMilliseconds)) autoIncludeMs=\(format(profile.autoIncludeMilliseconds)) rebuildMs=\(format(profile.rebuildSnapshotsMilliseconds)) postApplyMs=\(format(profile.postApplyMilliseconds)) totalMs=\(format(profile.totalMilliseconds))")
        }
        if let profile = model.lastDerivedSnapshotBuildProfileForBenchmark {
            print("HCBLargeAccountBenchmark appModel.derivedBreakdown visibilityMs=\(format(profile.visibilityMilliseconds)) coreSnapshotsMs=\(format(profile.coreSnapshotMilliseconds)) eventBucketsMs=\(format(profile.eventBucketMilliseconds)) taskBucketsStatsMs=\(format(profile.taskBucketAndStatsMilliseconds)) idIndexesMs=\(format(profile.idIndexMilliseconds)) auxiliaryMs=\(format(profile.auxiliaryLookupMilliseconds)) fingerprintMs=\(format(profile.fingerprintMilliseconds)) totalMs=\(format(profile.totalMilliseconds))")
            print("HCBLargeAccountBenchmark appModel.derivedDetailed taskSectionsMs=\(format(profile.taskSectionsMilliseconds)) taskBoardMs=\(format(profile.taskBoardMilliseconds)) todayMs=\(format(profile.todaySnapshotMilliseconds)) calendarMs=\(format(profile.calendarSnapshotMilliseconds)) eventsByCalendarMs=\(format(profile.eventsByCalendarMilliseconds)) eventsByDayMs=\(format(profile.eventsByDayMilliseconds)) taskDueBucketsMs=\(format(profile.taskDueBucketsMilliseconds)) taskSidebarCountsMs=\(format(profile.taskSidebarCountsMilliseconds)) taskCompletionStatsMs=\(format(profile.taskCompletionStatsMilliseconds)) taskIndexMs=\(format(profile.taskIndexMilliseconds)) eventIndexMs=\(format(profile.eventIndexMilliseconds)) titleLookupMs=\(format(profile.titleLookupMilliseconds)) taskChildrenMs=\(format(profile.taskChildrenMilliseconds)) duplicateIndexMs=\(format(profile.duplicateIndexMilliseconds)) taskListIndexMs=\(format(profile.taskListIndexMilliseconds))")
            print("HCBLargeAccountBenchmark appModel.snapshotBreakdown todayDueMs=\(format(profile.todayTaskDueMilliseconds)) todayOverdueMs=\(format(profile.todayTaskOverdueMilliseconds)) todayEventFilterMs=\(format(profile.todayEventFilterMilliseconds)) todayEventSortMs=\(format(profile.todayEventSortMilliseconds)) todayScheduled=\(profile.todayScheduledEventCount) calendarSetupMs=\(format(profile.calendarSetupMilliseconds)) calendarEventScanMs=\(format(profile.calendarEventScanMilliseconds)) calendarSelected=\(profile.calendarSelectedEventCount) calendarVisible=\(profile.calendarVisibleEventCount)")
            print("HCBLargeAccountBenchmark appModel.calendarBreakdown visibilityMs=\(format(profile.calendarEventVisibilityMilliseconds)) calendarCountMs=\(format(profile.calendarCountMilliseconds)) colorMs=\(format(profile.calendarColorAggregationMilliseconds)) literalTagMs=\(format(profile.calendarLiteralTagExtractionMilliseconds)) literalSummaryScanMs=\(format(profile.calendarLiteralSummaryScanMilliseconds)) literalDetailsScanMs=\(format(profile.calendarLiteralDetailsScanMilliseconds)) literalLocationScanMs=\(format(profile.calendarLiteralLocationScanMilliseconds)) literalRegexMs=\(format(profile.calendarLiteralRegexMatchingMilliseconds)) literalDedupMs=\(format(profile.calendarLiteralDeduplicationMilliseconds)) boundTagMs=\(format(profile.calendarBoundTagAggregationMilliseconds)) tagCountMs=\(format(profile.calendarTagCountMilliseconds)) colorMapMs=\(format(profile.calendarColorMapMilliseconds)) literalTagged=\(profile.calendarLiteralTaggedEventCount) boundTagged=\(profile.calendarBoundTaggedEventCount)")
            print("HCBLargeAccountBenchmark appModel.eventsByDayBreakdown hidden=\(profile.hiddenEventCount) singleDayTimed=\(profile.singleDayTimedEventCount) allDay=\(profile.allDayEventCount) multiDay=\(profile.multiDayEventCount) hiddenFilterMs=\(format(profile.eventHiddenFilteringMilliseconds)) startOfDayMs=\(format(profile.eventStartOfDayMilliseconds)) startDayMs=\(format(profile.eventStartDayNormalizationMilliseconds)) endDayMs=\(format(profile.eventEndDayNormalizationMilliseconds)) sameDayProbeMs=\(format(profile.eventSameDayProbeMilliseconds)) singleDayTimedMs=\(format(profile.eventSingleDayTimedMilliseconds)) allDayMs=\(format(profile.eventAllDayMilliseconds)) multiDayMs=\(format(profile.eventMultiDayMilliseconds)) daySteppingMs=\(format(profile.eventDaySteppingMilliseconds)) allDaySteppingMs=\(format(profile.eventAllDaySteppingMilliseconds)) multiDaySteppingMs=\(format(profile.eventMultiDaySteppingMilliseconds)) bucketInsertMs=\(format(profile.eventBucketInsertionMilliseconds))")
        }
        XCTAssertEqual(model.events.count, state.events.count)
        XCTAssertEqual(model.eventByIDSnapshot.count, state.events.count)
        XCTAssertGreaterThanOrEqual(eventDayReferences, activeEventCount)
        XCTAssertFalse(model.isRebuildingDerivedSnapshots)
    }

    @MainActor
    func testAppModelEventOnlySyncApplySkipsFullDerivedRebuild() async throws {
        let eventCount = 8_000
        var state = LargeAccountCalendarFixture.makeState(eventCount: eventCount, calendarCount: 1)
        state.settings.cloudSyncTargets = [.events]
        state.settings.selectedCalendarIDs = ["cal-0"]
        state.settings.hasConfiguredCalendarSelection = true
        state.syncCheckpoints = [
            SyncCheckpoint(
                id: SyncCheckpoint.stableID(accountID: "large-account", resourceType: .calendar, resourceID: "cal-0"),
                accountID: "large-account",
                resourceType: .calendar,
                resourceID: "cal-0",
                calendarSyncToken: "previous-token",
                tasksUpdatedMin: nil,
                lastSuccessfulSyncAt: LargeAccountCalendarFixture.denseDay
            )
        ]
        let changedIndex = try XCTUnwrap(state.events.firstIndex { event in
            event.calendarID == "cal-0" && event.status != .cancelled
        })
        var changedEvent = state.events[changedIndex]
        changedEvent.summary += " #narrow-sync"
        changedEvent.etag = "\(changedEvent.etag ?? "event")-narrow"
        changedEvent.updatedAt = LargeAccountCalendarFixture.denseDay.addingTimeInterval(3_600)

        let model = makeModel(cachedState: state)
        await model.loadInitialState()
        try await waitForDerivedSnapshotRebuild(model)
        let beforeTaskRevision = model.taskStore.taskDisplayRevision
        let beforeNotesRevision = model.taskStore.notesDisplayRevision
        let beforeRevision = model.calendarDisplayRevisionKey(for: [changedEvent.startDate])
        let beforeTagCount = model.calendarSnapshot.eventCountsByTagName["narrow-sync"] ?? 0

        let calendarListData = try LargeAccountCalendarFixture.calendarListResponseData(calendars: state.calendars)
        let incrementalEventsData = try LargeAccountCalendarFixture.eventsResponseData(events: [changedEvent])
        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json", "Date": "Thu, 14 May 2026 02:00:00 GMT"]
            )!
            switch request.url?.path {
            case "/calendar/v3/users/me/calendarList":
                return (response, calendarListData)
            case "/calendar/v3/calendars/cal-0/events":
                return (response, incrementalEventsData)
            default:
                XCTFail("Unexpected request \(request.url?.absoluteString ?? "<nil>")")
                return (response, Data(#"{"items":[]}"#.utf8))
            }
        }

        let outcome = await model.refreshNow()
        guard case .succeeded = outcome else {
            XCTFail("Expected event-only refresh to succeed")
            return
        }

        let afterRevision = model.calendarDisplayRevisionKey(for: [changedEvent.startDate])
        XCTAssertEqual(model.event(id: changedEvent.id)?.summary, changedEvent.summary)
        XCTAssertEqual(model.calendarSnapshot.eventCountsByTagName["narrow-sync"], beforeTagCount + 1)
        XCTAssertNotEqual(afterRevision, beforeRevision)
        XCTAssertEqual(model.taskStore.taskDisplayRevision, beforeTaskRevision)
        XCTAssertEqual(model.taskStore.notesDisplayRevision, beforeNotesRevision)
        XCTAssertTrue(model.lastApplyUsedEventOnlyDerivedUpdateForBenchmark)
        if let eventOnlyProfile = model.lastEventOnlyApplyProfileForBenchmark {
            print("HCBLargeAccountBenchmark appModel.eventOnlyApply lookupMs=\(format(eventOnlyProfile.lookupMilliseconds)) indexMs=\(format(eventOnlyProfile.indexMilliseconds)) snapshotMapMs=\(format(eventOnlyProfile.snapshotMapMilliseconds)) bucketMs=\(format(eventOnlyProfile.bucketMilliseconds)) todayMs=\(format(eventOnlyProfile.todayMilliseconds)) totalMs=\(format(eventOnlyProfile.totalMilliseconds))")
        }
        XCTAssertLessThan(model.lastApplyProfileForBenchmark?.rebuildSnapshotsMilliseconds ?? .greatestFiniteMagnitude, 20)
    }

    @MainActor
    func testAppModelTaskOnlySyncApplyDefersFullDerivedRebuildAndAdvancesTaskStoreRevisions() async throws {
        var state = LargeAccountCalendarFixture.makeState(eventCount: 8_000, calendarCount: 1)
        state.settings.cloudSyncTargets = [.tasks]

        let changedIndex = try XCTUnwrap(state.tasks.firstIndex { $0.isDeleted == false })
        var remoteTasks = state.tasks
        remoteTasks[changedIndex].title += " #task-sync"
        remoteTasks[changedIndex].etag = "\(remoteTasks[changedIndex].etag ?? "task")-task-sync"
        remoteTasks[changedIndex].updatedAt = LargeAccountCalendarFixture.denseDay.addingTimeInterval(3_600)

        let model = makeModel(cachedState: state)
        await model.loadInitialState()
        try await waitForDerivedSnapshotRebuild(model)
        let beforeTaskRevision = model.taskStore.taskDisplayRevision
        let beforeNotesRevision = model.taskStore.notesDisplayRevision

        let taskListsData = try LargeAccountCalendarFixture.taskListsResponseData(taskLists: state.taskLists)
        let tasksData = try LargeAccountCalendarFixture.tasksResponseData(tasks: remoteTasks)
        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json", "Date": "Thu, 14 May 2026 02:00:00 GMT"]
            )!
            switch request.url?.path {
            case "/tasks/v1/users/@me/lists":
                return (response, taskListsData)
            case "/tasks/v1/lists/tasks-main/tasks":
                return (response, tasksData)
            default:
                XCTFail("Unexpected request \(request.url?.absoluteString ?? "<nil>")")
                return (response, Data(#"{"items":[]}"#.utf8))
            }
        }

        let outcome = await model.refreshNow()
        guard case .succeeded = outcome else {
            XCTFail("Expected task-only refresh to succeed")
            return
        }
        XCTAssertLessThan(model.lastApplyProfileForBenchmark?.rebuildSnapshotsMilliseconds ?? .greatestFiniteMagnitude, 20)

        try await waitForDerivedSnapshotRebuild(model)
        XCTAssertGreaterThan(model.taskStore.taskDisplayRevision, beforeTaskRevision)
        XCTAssertGreaterThan(model.taskStore.notesDisplayRevision, beforeNotesRevision)
        XCTAssertEqual(model.taskStore.task(id: remoteTasks[changedIndex].id)?.title, remoteTasks[changedIndex].title)
    }

    func testPreparedCalendarSnapshotsLargeAccountBenchmark() {
        let state = LargeAccountCalendarFixture.makeState(eventCount: 15_000)
        let input = LargeAccountCalendarFixture.displayInput(
            state: state,
            key: PreparedSnapshotKey("large-account"),
            anchorDate: LargeAccountCalendarFixture.denseDay
        )

        let (day, dayMs) = timed("snapshot.day.15k") {
            CalendarDisplaySnapshotBuilder.daySnapshot(input)
        }
        let (week, weekMs) = timed("snapshot.week.15k") {
            CalendarDisplaySnapshotBuilder.weekSnapshot(input)
        }
        let (agenda, agendaMs) = timed("snapshot.agenda.15k") {
            CalendarDisplaySnapshotBuilder.agendaSnapshot(input, dayCount: 14)
        }
        let (year, yearMs) = timed("snapshot.year.15k") {
            CalendarDisplaySnapshotBuilder.yearSnapshot(input)
        }

        print("HCBLargeAccountBenchmark snapshotCounts day=\(day.allDayEvents.count + day.timedEvents.count) dayLaidOut=\(day.laidOutTimedEvents.count) dayHidden=\(day.density.hiddenTimedEventCount) weekEvents=\(week.eventMetadataByID.count) weekDenseHidden=\(week.densityByDay.values.map(\.hiddenTimedEventCount).max() ?? 0) agendaEvents=\(agenda.eventMetadataByID.count) yearDays=\(year.countsByDay.count) dayMs=\(format(dayMs)) weekMs=\(format(weekMs)) agendaMs=\(format(agendaMs)) yearMs=\(format(yearMs))")
        XCTAssertGreaterThan(day.timedEvents.count, 300)
        XCTAssertEqual(day.laidOutTimedEvents.count, CalendarDensityRendering.dayTimedEventLimit)
        XCTAssertGreaterThan(day.density.hiddenTimedEventCount, 300)
        XCTAssertGreaterThan(week.eventMetadataByID.count, day.timedEvents.count)
        XCTAssertLessThanOrEqual(week.laidOutTimedEventsByDay.values.map(\.count).max() ?? 0, CalendarDensityRendering.weekTimedEventLimit)
        XCTAssertGreaterThan(agenda.days.flatMap(\.events).count, week.eventMetadataByID.count)
        XCTAssertGreaterThan(year.countsByDay.count, 300)
        XCTAssertTrue(day.timedEvents.allSatisfy { $0.status != .cancelled })
    }

    func testGoogleEventDecodeAndMapLargeAccountBreakdownBenchmark() throws {
        let state = LargeAccountCalendarFixture.makeState(eventCount: 15_000, calendarCount: 1)
        let eventsData = try LargeAccountCalendarFixture.eventsResponseData(events: state.events)

        let (rawResponse, rawDecodeMs) = try timed("sync.breakdown.rawStringDecode.15k") {
            try JSONDecoder().decode(LargeAccountRawGoogleEventsResponse.self, from: eventsData)
        }
        let profile = try GoogleCalendarClient.decodeAndMapEventsForBenchmark(
            data: eventsData,
            calendarID: "cal-0",
            defaultTimeZoneID: "UTC"
        )

        print("HCBLargeAccountBenchmark sync.breakdown.rawStringDecodeMs=\(format(rawDecodeMs)) decodeMs=\(format(profile.decodeMilliseconds)) mirrorMs=\(format(profile.mirrorMilliseconds)) decodeAndMirrorMs=\(format(profile.totalMilliseconds)) decoded=\(profile.decodedItemCount) mapped=\(profile.mappedEventCount)")
        XCTAssertEqual(rawResponse.items.count, state.events.count)
        XCTAssertEqual(profile.decodedItemCount, state.events.count)
        XCTAssertEqual(profile.mappedEventCount, state.events.count)
        XCTAssertEqual(profile.nextSyncToken, "large-sync-token")
    }

    func testGoogleCalendarClientListEventsLargeAccountBenchmark() async throws {
        let state = LargeAccountCalendarFixture.makeState(eventCount: 15_000, calendarCount: 1)
        let eventsData = try LargeAccountCalendarFixture.eventsResponseData(events: state.events)

        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json", "Date": "Thu, 14 May 2026 02:00:00 GMT"]
            )!
            XCTAssertEqual(request.url?.path, "/calendar/v3/calendars/cal-0/events")
            return (response, eventsData)
        }

        let transport = GoogleAPITransport(
            baseURL: URL(string: "https://example.test")!,
            tokenProvider: StaticAccessTokenProvider(token: "large-account-token"),
            urlSession: MockURLProtocol.testSession()
        )
        let client = GoogleCalendarClient(transport: transport)
        let (page, listMs) = try await timedAsync("sync.breakdown.clientListEvents.15k") {
            try await client.listEvents(calendarID: "cal-0", syncToken: nil, timeMin: nil, defaultTimeZoneID: "UTC")
        }

        print("HCBLargeAccountBenchmark sync.breakdown.clientEvents=\(page.events.count) clientListMs=\(format(listMs))")
        XCTAssertEqual(page.events.count, state.events.count)
        XCTAssertEqual(page.nextSyncToken, "large-sync-token")
    }

    func testSyncSchedulerFullEventApplyLargeAccountBenchmark() async throws {
        var state = LargeAccountCalendarFixture.makeState(eventCount: 15_000, calendarCount: 1)
        state.settings.cloudSyncTargets = [.events]
        state.settings.selectedCalendarIDs = ["cal-0"]
        state.settings.hasConfiguredCalendarSelection = true
        let expectedActiveEvents = state.events.filter { $0.status != .cancelled }.count
        let calendarListData = try LargeAccountCalendarFixture.calendarListResponseData(calendars: state.calendars)
        let eventsData = try LargeAccountCalendarFixture.eventsResponseData(events: state.events)

        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json", "Date": "Thu, 14 May 2026 02:00:00 GMT"]
            )!
            switch request.url?.path {
            case "/calendar/v3/users/me/calendarList":
                return (response, calendarListData)
            case "/calendar/v3/calendars/cal-0/events":
                return (response, eventsData)
            default:
                XCTFail("Unexpected request \(request.url?.absoluteString ?? "<nil>")")
                return (response, Data(#"{"items":[]}"#.utf8))
            }
        }

        let scheduler = makeScheduler()
        let (synced, syncMs) = try await timedAsync("sync.fullEvents.15k") {
            try await scheduler.syncNow(mode: .balanced, baseState: state)
        }

        print("HCBLargeAccountBenchmark sync.events=\(synced.events.count) syncMs=\(format(syncMs))")
        XCTAssertEqual(synced.events.count, expectedActiveEvents)
        XCTAssertEqual(
            synced.events.first { $0.id == "event-3" }?.details,
            "Detailed notes for event 3 #project"
        )
        XCTAssertEqual(synced.syncCheckpoints.count, 1)
        XCTAssertEqual(synced.syncCheckpoints.first?.calendarSyncToken, "large-sync-token")
    }

    func testSyncSchedulerSmallIncrementalVsFullEventApplyBenchmark() async throws {
        let eventCount = 8_000
        var fullBase = LargeAccountCalendarFixture.makeState(eventCount: eventCount, calendarCount: 1)
        fullBase.settings.cloudSyncTargets = [.events]
        fullBase.settings.selectedCalendarIDs = ["cal-0"]
        fullBase.settings.hasConfiguredCalendarSelection = true
        fullBase.events = []

        var incrementalBase = LargeAccountCalendarFixture.makeState(eventCount: eventCount, calendarCount: 1)
        incrementalBase.settings.cloudSyncTargets = [.events]
        incrementalBase.settings.selectedCalendarIDs = ["cal-0"]
        incrementalBase.settings.hasConfiguredCalendarSelection = true
        incrementalBase.syncCheckpoints = [
            SyncCheckpoint(
                id: SyncCheckpoint.stableID(accountID: "large-account", resourceType: .calendar, resourceID: "cal-0"),
                accountID: "large-account",
                resourceType: .calendar,
                resourceID: "cal-0",
                calendarSyncToken: "previous-token",
                tasksUpdatedMin: nil,
                lastSuccessfulSyncAt: LargeAccountCalendarFixture.denseDay
            )
        ]
        var changedEvent = incrementalBase.events[eventCount / 2]
        changedEvent.summary += " updated"
        changedEvent.etag = "\(changedEvent.etag ?? "event")-v2"
        changedEvent.updatedAt = LargeAccountCalendarFixture.denseDay.addingTimeInterval(3600)

        let calendarListData = try LargeAccountCalendarFixture.calendarListResponseData(calendars: incrementalBase.calendars)
        let fullEventsData = try LargeAccountCalendarFixture.eventsResponseData(events: incrementalBase.events)
        let incrementalEventsData = try LargeAccountCalendarFixture.eventsResponseData(events: [changedEvent])

        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json", "Date": "Thu, 14 May 2026 02:00:00 GMT"]
            )!
            switch request.url?.path {
            case "/calendar/v3/users/me/calendarList":
                return (response, calendarListData)
            case "/calendar/v3/calendars/cal-0/events":
                let query = Dictionary(uniqueKeysWithValues: (URLComponents(url: request.url!, resolvingAgainstBaseURL: false)?.queryItems ?? []).map { ($0.name, $0.value ?? "") })
                return (response, query["syncToken"] == nil ? fullEventsData : incrementalEventsData)
            default:
                XCTFail("Unexpected request \(request.url?.absoluteString ?? "<nil>")")
                return (response, Data(#"{"items":[]}"#.utf8))
            }
        }

        let scheduler = makeScheduler()
        let (fullResult, fullMs) = try await timedAsync("sync.fullEvents.\(eventCount).changeSet") {
            try await scheduler.syncNowWithChangeSet(mode: .balanced, baseState: fullBase)
        }
        let (incrementalResult, incrementalMs) = try await timedAsync("sync.incrementalOneEvent.\(eventCount).changeSet") {
            try await scheduler.syncNowWithChangeSet(mode: .balanced, baseState: incrementalBase)
        }

        print("HCBLargeAccountBenchmark sync.compare fullMs=\(format(fullMs)) incrementalMs=\(format(incrementalMs)) fullInserted=\(fullResult.changeSet.events.inserted.count) incrementalUpdated=\(incrementalResult.changeSet.events.updated.count) incrementalUnchanged=\(incrementalResult.changeSet.events.unchanged.count)")
        XCTAssertEqual(fullResult.changeSet.events.inserted.count, fullResult.state.events.count)
        XCTAssertEqual(incrementalResult.changeSet.events.updated, [changedEvent.id])
        XCTAssertEqual(incrementalResult.state.events.count, fullResult.state.events.count)
        XCTAssertLessThan(incrementalResult.changeSet.events.updated.count, fullResult.changeSet.events.inserted.count)
    }

    func testSyncSchedulerFullEventApplyBreakdownBenchmark() async throws {
        var state = LargeAccountCalendarFixture.makeState(eventCount: 15_000, calendarCount: 1)
        state.settings.cloudSyncTargets = [.events]
        state.settings.selectedCalendarIDs = ["cal-0"]
        state.settings.hasConfiguredCalendarSelection = true
        let expectedActiveEvents = state.events.filter { $0.status != .cancelled }.count
        let calendarListData = try LargeAccountCalendarFixture.calendarListResponseData(calendars: state.calendars)
        let eventsData = try LargeAccountCalendarFixture.eventsResponseData(events: state.events)

        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json", "Date": "Thu, 14 May 2026 02:00:00 GMT"]
            )!
            switch request.url?.path {
            case "/calendar/v3/users/me/calendarList":
                return (response, calendarListData)
            case "/calendar/v3/calendars/cal-0/events":
                return (response, eventsData)
            default:
                XCTFail("Unexpected request \(request.url?.absoluteString ?? "<nil>")")
                return (response, Data(#"{"items":[]}"#.utf8))
            }
        }

        let scheduler = makeScheduler()
        let profile = try await scheduler.profileEventSyncForBenchmark(mode: .balanced, baseState: state)

        print("HCBLargeAccountBenchmark sync.applyBreakdown calendars=\(profile.calendarCount) selected=\(profile.selectedCalendarCount) remoteEvents=\(profile.resultEventCount) mergedEvents=\(profile.mergedEventCount) checkpoints=\(profile.checkpointCount) checkpointIndexMs=\(format(profile.checkpointIndexMilliseconds)) calendarListMs=\(format(profile.calendarListMilliseconds)) filteringMs=\(format(profile.calendarFilteringMilliseconds)) pageCollectionMs=\(format(profile.eventPageCollectionMilliseconds)) checkpointCollectMs=\(format(profile.checkpointCollectMilliseconds)) calendarMapMs=\(format(profile.calendarMapMilliseconds)) eventMergeMs=\(format(profile.eventMergeMilliseconds)) checkpointMergeMs=\(format(profile.checkpointMergeMilliseconds)) stateBuildMs=\(format(profile.stateBuildMilliseconds)) totalMs=\(format(profile.totalMilliseconds))")
        print("HCBLargeAccountBenchmark sync.mergeBreakdown existing=\(profile.merge.existingCount) remote=\(profile.merge.remoteEventCount) dictionary=\(profile.merge.dictionaryCount) output=\(profile.merge.outputCount) fullSyncIDsMs=\(format(profile.merge.fullSyncIDMilliseconds)) resultCountMs=\(format(profile.merge.resultCountMilliseconds)) dictionarySetupMs=\(format(profile.merge.dictionarySetupMilliseconds)) preserveExistingMs=\(format(profile.merge.preserveExistingMilliseconds)) upsertRemoteMs=\(format(profile.merge.upsertRemoteMilliseconds)) cutoffMs=\(format(profile.merge.cutoffMilliseconds)) filterMs=\(format(profile.merge.filterMilliseconds)) sortCheckMs=\(format(profile.merge.sortCheckMilliseconds)) sortMs=\(format(profile.merge.sortMilliseconds)) didSort=\(profile.merge.didSort) totalMs=\(format(profile.merge.totalMilliseconds))")
        XCTAssertEqual(profile.resultEventCount, state.events.count)
        XCTAssertEqual(profile.mergedEventCount, expectedActiveEvents)
        XCTAssertEqual(profile.checkpointCount, 1)
        XCTAssertEqual(profile.merge.outputCount, expectedActiveEvents)
    }

    @MainActor
    func testMenuBarProjectionAdaptiveStatusLargeAccountBenchmark() async throws {
        var state = LargeAccountCalendarFixture.makeState(eventCount: 15_000)
        state.settings.menuBarAdaptiveStatusSource = .eventsAndTasks
        state.settings.menuBarAdaptiveEmptyBehavior = .nextCommitment
        let now = LargeAccountCalendarFixture.denseDay.addingTimeInterval(11 * 3_600)
        let model = makeModel(cachedState: state)
        await model.loadInitialState()
        try await waitForDerivedSnapshotRebuild(model)

        let projection = model.menuBarStore.projection
        XCTAssertLessThanOrEqual(projection.adaptiveEvents.count, 64)
        XCTAssertLessThanOrEqual(projection.adaptiveTasks.count, 64)
        XCTAssertFalse(projection.dayBuckets.isEmpty)

        let (status, statusMs) = timed("menu.status.15k") {
            projection.adaptiveStatus(now: now, calendar: calendar)
        }

        print("HCBLargeAccountBenchmark menu.status.kind=\(status.kind) statusMs=\(format(statusMs))")
        XCTAssertLessThan(statusMs, 5, "menu-bar timer refresh should resolve from capped projection candidates")
        switch status.kind {
        case .currentEvent, .nextEvent, .task:
            break
        case .clear, .iconOnly:
            XCTFail("large fixture should produce an actionable menu bar status")
        }
    }

    func testActionCenterProjectionLargeAccountBenchmark() async throws {
        let state = LargeAccountCalendarFixture.makeState(eventCount: 15_000)
        let fileURL = FileManager.default.temporaryDirectory
            .appending(path: "HotCrossBunsLargeAccountBenchmarks", directoryHint: .isDirectory)
            .appending(path: UUID().uuidString)
            .appendingPathExtension("json")
        defer { try? FileManager.default.removeItem(at: fileURL.deletingLastPathComponent()) }
        let store = LocalCacheStore(fileURL: fileURL)
        await store.save(state)

        let (projection, projectionMs) = try await timedAsync("actionCenter.overdueProjection.15k") {
            try await store.overdueTasks(
                before: LargeAccountCalendarFixture.denseDay,
                visibleTaskListIDs: ["tasks-main"],
                limit: ActionCenterBuilder.defaultOverdueTaskDisplayLimit
            )
        }
        print("HCBLargeAccountBenchmark actionCenter.overdueCount=\(projection.totalCount) shown=\(projection.tasks.count) projectionMs=\(format(projectionMs))")
        XCTAssertGreaterThan(projection.totalCount, 0)
        XCTAssertLessThan(projectionMs, 120)
    }

    @MainActor
    private func makeModel(cachedState: CachedAppState) -> AppModel {
        let customOAuthService = CustomGoogleOAuthService(tokenStore: InMemoryGoogleOAuthTokenStore(
            clientConfiguration: GoogleOAuthClientConfiguration(clientID: "large-account.apps.googleusercontent.com", clientSecret: nil)
        ))
        let authService = GoogleAuthService(customOAuthService: customOAuthService)
        let transport = GoogleAPITransport(
            baseURL: URL(string: "https://example.test")!,
            tokenProvider: StaticAccessTokenProvider(token: "large-account-token"),
            urlSession: MockURLProtocol.testSession()
        )
        let tasksClient = GoogleTasksClient(transport: transport)
        let calendarClient = GoogleCalendarClient(transport: transport)
        return AppModel(
            authService: authService,
            tasksClient: tasksClient,
            calendarClient: calendarClient,
            syncScheduler: SyncScheduler(tasksClient: tasksClient, calendarClient: calendarClient),
            cacheStore: LocalCacheStore(fileURL: nil, cachedState: cachedState)
        )
    }

    private func makeScheduler() -> SyncScheduler {
        let transport = GoogleAPITransport(
            baseURL: URL(string: "https://example.test")!,
            tokenProvider: StaticAccessTokenProvider(token: "large-account-token"),
            urlSession: MockURLProtocol.testSession()
        )
        let tasksClient = GoogleTasksClient(transport: transport)
        let calendarClient = GoogleCalendarClient(transport: transport)
        return SyncScheduler(tasksClient: tasksClient, calendarClient: calendarClient)
    }

    @MainActor
    private func waitForDerivedSnapshotRebuild(
        _ model: AppModel,
        timeout: TimeInterval = 8
    ) async throws {
        let deadline = Date().addingTimeInterval(timeout)
        while model.isRebuildingDerivedSnapshots || model.eventByIDSnapshot.count != model.events.count {
            if Date() >= deadline {
                XCTFail("Timed out waiting for deferred derived snapshot rebuild")
                return
            }
            try await Task.sleep(for: .milliseconds(25))
        }
        XCTAssertFalse(model.isRebuildingDerivedSnapshots)
    }

    private func functionBody(named name: String, in source: String) -> String? {
        guard let nameRange = source.range(of: "func \(name)") else { return nil }
        guard let openingBrace = source[nameRange.lowerBound...].firstIndex(of: "{") else { return nil }
        var depth = 0
        var index = openingBrace
        while index < source.endIndex {
            let character = source[index]
            if character == "{" {
                depth += 1
            } else if character == "}" {
                depth -= 1
                if depth == 0 {
                    return String(source[openingBrace...index])
                }
            }
            index = source.index(after: index)
        }
        return nil
    }

    @discardableResult
    private func timed<T>(_ label: String, _ block: () throws -> T) rethrows -> (T, Double) {
        let start = ContinuousClock.now
        let value = try block()
        let milliseconds = elapsedMilliseconds(since: start)
        print("HCBLargeAccountBenchmark \(label)=\(format(milliseconds))ms")
        return (value, milliseconds)
    }

    @discardableResult
    private func timedAsync<T>(_ label: String, _ block: () async throws -> T) async rethrows -> (T, Double) {
        let start = ContinuousClock.now
        let value = try await block()
        let milliseconds = elapsedMilliseconds(since: start)
        print("HCBLargeAccountBenchmark \(label)=\(format(milliseconds))ms")
        return (value, milliseconds)
    }

    private func elapsedMilliseconds(since start: ContinuousClock.Instant) -> Double {
        let elapsed = start.duration(to: .now).components
        return Double(elapsed.seconds) * 1_000
            + Double(elapsed.attoseconds) / 1_000_000_000_000_000
    }

    private func format(_ value: Double) -> String {
        String(format: "%.2f", value)
    }
}

private enum LargeAccountCalendarFixture {
    static let calendar: Calendar = {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = TimeZone(identifier: "UTC")!
        cal.firstWeekday = 1
        return cal
    }()

    static let denseDay = date(2026, 5, 14)
    private static let yearStart = date(2026, 1, 1)

    static func makeState(eventCount: Int, calendarCount: Int = 4) -> CachedAppState {
        let account = GoogleAccount(
            id: "large-account",
            email: "large@example.com",
            displayName: "Large Account",
            grantedScopes: [GoogleScope.tasks, GoogleScope.calendar],
            authProvider: .customDesktopOAuth
        )
        let calendars = (0..<calendarCount).map { index in
            CalendarListMirror(
                id: "cal-\(index)",
                summary: "Calendar \(index)",
                colorHex: calendarColor(index),
                isSelected: true,
                accessRole: index == 0 ? "owner" : "reader",
                etag: "cal-etag-\(index)",
                defaultReminderMinutes: [10],
                timeZoneID: "UTC"
            )
        }
        let tasks = makeTasks()
        var settings = AppSettings.default
        settings.cloudSyncTargets = CloudSyncTarget.all
        settings.selectedCalendarIDs = Set(calendars.map(\.id))
        settings.hasConfiguredCalendarSelection = true
        settings.selectedTaskListIDs = ["tasks-main"]
        settings.hasConfiguredTaskListSelection = true

        return CachedAppState(
            account: account,
            accounts: [account],
            activeAccountID: account.id,
            taskLists: [TaskListMirror(id: "tasks-main", title: "Inbox", updatedAt: yearStart, etag: "tasks-main-etag")],
            tasks: tasks,
            calendars: calendars,
            events: makeEvents(count: eventCount, calendars: calendars),
            settings: settings
        )
    }

    static func displayInput(
        state: CachedAppState,
        key: PreparedSnapshotKey,
        anchorDate: Date,
        searchQuery: String = ""
    ) -> CalendarDisplayInput {
        let indexes = buildIndexes(events: state.events, tasks: state.tasks)
        let eventByID = Dictionary(uniqueKeysWithValues: state.events.map { ($0.id, $0) })
        let taskByID = Dictionary(uniqueKeysWithValues: state.tasks.map { ($0.id, $0) })
        let range = CalendarVisibleRange(kind: .year, start: yearStart, end: calendar.date(byAdding: .day, value: 420, to: yearStart)!, calendar: calendar)
        let visibleRange = CalendarVisibleRangeProjection(
            range: range,
            revisionKey: "large-account",
            eventsByDay: indexes.eventsByDay,
            tasksByDueDate: indexes.tasksByDueDate,
            eventByID: eventByID,
            taskByID: taskByID
        )

        return CalendarDisplayInput(
            key: key,
            anchorDate: anchorDate,
            selectedCalendarIDs: state.settings.selectedCalendarIDs,
            eventViewFilter: CalendarEventViewFilter(),
            visibleTaskListIDs: state.settings.selectedTaskListIDs,
            searchQuery: searchQuery,
            visibleRange: visibleRange,
            calendarColorHexByID: Dictionary(uniqueKeysWithValues: state.calendars.map { ($0.id, $0.colorHex) }),
            taskListTitleByID: Dictionary(uniqueKeysWithValues: state.taskLists.map { ($0.id, $0.title) }),
            settings: state.settings,
            referenceDate: denseDay.addingTimeInterval(8 * 3_600),
            calendar: calendar
        )
    }

    static func taskListsResponseData(taskLists: [TaskListMirror]) throws -> Data {
        let items = taskLists.map { taskList in
            [
                "id": taskList.id,
                "title": taskList.title,
                "updated": taskList.updatedAt.map { ISO8601DateFormatter.google.string(from: $0) } ?? "2026-01-01T00:00:00Z",
                "etag": taskList.etag ?? ""
            ] as [String: Any]
        }
        return try JSONSerialization.data(withJSONObject: ["items": items], options: [])
    }

    static func tasksResponseData(tasks: [TaskMirror]) throws -> Data {
        let items = tasks.map { task in
            var object: [String: Any] = [
                "id": task.id,
                "title": task.title,
                "status": task.status.rawValue,
                "deleted": task.isDeleted,
                "hidden": task.isHidden,
                "etag": task.etag ?? "",
                "updated": task.updatedAt.map { ISO8601DateFormatter.google.string(from: $0) } ?? "2026-01-01T00:00:00Z"
            ]
            if task.notes.isEmpty == false {
                object["notes"] = task.notes
            }
            if let dueDate = task.dueDate {
                object["due"] = GoogleTaskDueDateFormatter.string(from: dueDate, calendar: calendar)
            }
            if let completedAt = task.completedAt {
                object["completed"] = ISO8601DateFormatter.google.string(from: completedAt)
            }
            if let parentID = task.parentID {
                object["parent"] = parentID
            }
            if let position = task.position {
                object["position"] = position
            }
            return object
        }
        return try JSONSerialization.data(withJSONObject: ["items": items], options: [])
    }

    static func calendarListResponseData(calendars: [CalendarListMirror]) throws -> Data {
        let items = calendars.map { calendar in
            [
                "id": calendar.id,
                "summary": calendar.summary,
                "backgroundColor": calendar.colorHex,
                "selected": calendar.isSelected,
                "accessRole": calendar.accessRole,
                "etag": calendar.etag ?? "",
                "timeZone": "UTC"
            ] as [String: Any]
        }
        return try JSONSerialization.data(withJSONObject: ["items": items], options: [])
    }

    static func eventsResponseData(events: [CalendarEventMirror]) throws -> Data {
        let items = events.map(googleEventJSONObject)
        return try JSONSerialization.data(
            withJSONObject: ["items": items, "nextSyncToken": "large-sync-token"],
            options: []
        )
    }

    private static func makeEvents(count: Int, calendars: [CalendarListMirror]) -> [CalendarEventMirror] {
        precondition(calendars.isEmpty == false)
        var events: [CalendarEventMirror] = []
        events.reserveCapacity(count)

        for index in 0..<count {
            let calendarID = calendars[index % calendars.count].id
            let isDense = index < min(600, count)
            let isAllDay = index % 23 == 0
            let isCancelled = index % 41 == 0
            let hasRecurrence = index % 29 == 0
            let start: Date
            let end: Date
            let allDay: Bool

            if isDense {
                let minute = (index % 96) * 15
                start = denseDay.addingTimeInterval(TimeInterval((8 * 60 + minute) * 60))
                end = start.addingTimeInterval(TimeInterval(45 * 60 + (index % 4) * 15 * 60))
                allDay = false
            } else if isAllDay {
                let dayOffset = index % 365
                start = calendar.date(byAdding: .day, value: dayOffset, to: yearStart)!
                let span = 2 + (index % 5)
                end = calendar.date(byAdding: .day, value: span, to: start)!
                allDay = true
            } else {
                let dayOffset = index % 420
                let base = calendar.date(byAdding: .day, value: dayOffset, to: yearStart)!
                let minute = ((index * 17) % (14 * 60))
                start = base.addingTimeInterval(TimeInterval((7 * 60 + minute) * 60))
                let duration = 30 + (index % 6) * 15
                end = start.addingTimeInterval(TimeInterval(duration * 60))
                allDay = false
            }

            events.append(CalendarEventMirror(
                id: "event-\(index)",
                calendarID: calendarID,
                summary: isDense ? "Dense review \(index) #focus" : "Planning event \(index)",
                details: index % 3 == 0 ? "Detailed notes for event \(index) #project" : "",
                startDate: start,
                endDate: end,
                isAllDay: allDay,
                status: isCancelled ? .cancelled : .confirmed,
                recurrence: hasRecurrence ? ["RRULE:FREQ=WEEKLY;COUNT=8"] : [],
                etag: "etag-\(index)",
                updatedAt: yearStart.addingTimeInterval(TimeInterval(index)),
                reminderMinutes: index % 7 == 0 ? [10, 30] : [],
                location: index % 4 == 0 ? "Room \(index % 20)" : "",
                attendeeEmails: index % 11 == 0 ? ["person\(index)@example.com"] : [],
                colorId: index % 9 == 0 ? CalendarEventColor.basil.rawValue : nil,
                startTimeZoneID: "UTC",
                endTimeZoneID: "UTC"
            ))
        }

        return events.sorted { lhs, rhs in
            if lhs.startDate != rhs.startDate { return lhs.startDate < rhs.startDate }
            return lhs.id < rhs.id
        }
    }

    private static func makeTasks() -> [TaskMirror] {
        (0..<240).map { index in
            let due = calendar.date(byAdding: .day, value: index % 60, to: yearStart)!
            return TaskMirror(
                id: "task-\(index)",
                taskListID: "tasks-main",
                parentID: nil,
                title: "Task \(index) #focus",
                notes: "",
                status: index % 17 == 0 ? .completed : .needsAction,
                dueDate: due,
                completedAt: index % 17 == 0 ? due : nil,
                isDeleted: false,
                isHidden: false,
                position: String(format: "%06d", index),
                etag: "task-etag-\(index)",
                updatedAt: due
            )
        }
    }

    private static func buildIndexes(
        events: [CalendarEventMirror],
        tasks: [TaskMirror]
    ) -> (eventsByDay: [TimeInterval: [CalendarEventMirror.ID]], tasksByDueDate: [TimeInterval: [TaskMirror.ID]]) {
        var eventsByDay: [TimeInterval: [CalendarEventMirror.ID]] = [:]
        eventsByDay.reserveCapacity(events.count)
        for event in events where event.status != .cancelled {
            let start = calendar.startOfDay(for: event.startDate)
            let end = CalendarGridLayout.eventEndDay(event: event, calendar: calendar)
            var cursor = start
            var steps = 0
            while cursor <= end && steps < 366 {
                eventsByDay[cursor.timeIntervalSinceReferenceDate, default: []].append(event.id)
                cursor = calendar.date(byAdding: .day, value: 1, to: cursor) ?? cursor.addingTimeInterval(86_400)
                steps += 1
            }
        }

        var tasksByDueDate: [TimeInterval: [TaskMirror.ID]] = [:]
        for task in tasks where task.isDeleted == false && task.isCompleted == false {
            guard let due = task.dueDate else { continue }
            tasksByDueDate[calendar.startOfDay(for: due).timeIntervalSinceReferenceDate, default: []].append(task.id)
        }
        return (eventsByDay, tasksByDueDate)
    }

    private static func googleEventJSONObject(_ event: CalendarEventMirror) -> [String: Any] {
        var object: [String: Any] = [
            "id": event.id,
            "summary": event.summary,
            "description": event.details,
            "status": event.status.rawValue,
            "updated": googleDateTime(event.updatedAt ?? event.startDate),
            "etag": event.etag ?? ""
        ]
        if event.location.isEmpty == false {
            object["location"] = event.location
        }
        if event.recurrence.isEmpty == false {
            object["recurrence"] = event.recurrence
        }
        if let colorId = event.colorId {
            object["colorId"] = colorId
        }
        if event.isAllDay {
            object["start"] = ["date": googleDate(event.startDate)]
            object["end"] = ["date": googleDate(event.endDate)]
        } else {
            object["start"] = ["dateTime": googleDateTime(event.startDate), "timeZone": "UTC"]
            object["end"] = ["dateTime": googleDateTime(event.endDate), "timeZone": "UTC"]
        }
        if event.attendeeEmails.isEmpty == false {
            object["attendees"] = event.attendeeEmails.map { ["email": $0] }
        }
        return object
    }

    private static func calendarColor(_ index: Int) -> String {
        ["#448AFF", "#FF7043", "#66BB6A", "#AB47BC"][index % 4]
    }

    private static func date(_ year: Int, _ month: Int, _ day: Int) -> Date {
        calendar.date(from: DateComponents(year: year, month: month, day: day))!
    }

    private static func googleDate(_ date: Date) -> String {
        let formatter = DateFormatter()
        formatter.calendar = calendar
        formatter.timeZone = calendar.timeZone
        formatter.dateFormat = "yyyy-MM-dd"
        return formatter.string(from: date)
    }

    private static func googleDateTime(_ date: Date) -> String {
        ISO8601DateFormatter().string(from: date)
    }
}

private extension JSONDecoder {
    static var largeAccountCache: JSONDecoder {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return decoder
    }
}

private struct LargeAccountRawGoogleEventsResponse: Decodable {
    var items: [LargeAccountRawGoogleEvent]
    var nextSyncToken: String?
}

private struct LargeAccountRawGoogleEvent: Decodable {
    var id: String
    var summary: String?
    var description: String?
    var location: String?
    var status: String?
    var start: LargeAccountRawGoogleEventDate?
    var end: LargeAccountRawGoogleEventDate?
    var recurrence: [String]?
    var etag: String?
    var updated: String?
    var attendees: [LargeAccountRawGoogleAttendee]?
    var colorId: String?
}

private struct LargeAccountRawGoogleEventDate: Decodable {
    var date: String?
    var dateTime: String?
    var timeZone: String?
}

private struct LargeAccountRawGoogleAttendee: Decodable {
    var email: String?
}
