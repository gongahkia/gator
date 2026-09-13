import Foundation

actor SyncScheduler {
    // Slack subtracted from the Tasks `updatedMin` watermark when we have to
    // fall back to the local clock (Date header missing or unparseable from
    // Google's response). The preferred path — §14 fix — reads Google's
    // server Date header on each listTasks call and uses that directly, so
    // clock drift between the user's device and Google is irrelevant. The
    // fallback slack handles the rare case where the header isn't available
    // (network quirks, mocks in tests without a Date header set).
    static let tasksWatermarkSlackSeconds: TimeInterval = 300

    private let tasksClient: GoogleTasksClient
    private let calendarClient: GoogleCalendarClient

    init(tasksClient: GoogleTasksClient, calendarClient: GoogleCalendarClient) {
        self.tasksClient = tasksClient
        self.calendarClient = calendarClient
    }

    func syncNow(mode: SyncMode, baseState: CachedAppState) async throws -> CachedAppState {
        try await syncNowWithChangeSet(mode: mode, baseState: baseState).state
    }

    func syncNowWithChangeSet(mode: SyncMode, baseState: CachedAppState) async throws -> SyncApplyResult {
        guard let accountID = baseState.account?.id else {
            return SyncApplyResult(state: baseState, changeSet: .empty)
        }

        let syncStartedAt = Date()
        let checkpointIndex = indexCheckpoints(baseState.syncCheckpoints)
        let syncTargets = baseState.settings.cloudSyncTargets
        let shouldSyncTasks = syncTargets.syncsTasks
        let shouldSyncEvents = syncTargets.syncsEvents
        let taskLists: [TaskListMirror]
        if shouldSyncTasks {
            AppLogger.info("sync stage start", category: .sync, metadata: ["stage": "tasks.taskLists.list"])
            do {
                taskLists = try await tasksClient.listTaskLists()
                AppLogger.info("sync stage succeeded", category: .sync, metadata: [
                    "stage": "tasks.taskLists.list",
                    "count": String(taskLists.count)
                ])
            } catch {
                AppLogger.error("sync stage failed", category: .sync, metadata: Self.syncErrorMetadata(error, stage: "tasks.taskLists.list"))
                throw error
            }
        } else {
            taskLists = baseState.taskLists
        }

        let calendars: [CalendarListMirror]
        if shouldSyncEvents {
            AppLogger.info("sync stage start", category: .sync, metadata: ["stage": "calendar.calendarList.list"])
            do {
                calendars = try await calendarClient.listCalendars()
                AppLogger.info("sync stage succeeded", category: .sync, metadata: [
                    "stage": "calendar.calendarList.list",
                    "count": String(calendars.count)
                ])
            } catch {
                AppLogger.error("sync stage failed", category: .sync, metadata: Self.syncErrorMetadata(error, stage: "calendar.calendarList.list"))
                throw error
            }
        } else {
            calendars = baseState.calendars
        }
        let selectedCalendarIDs = shouldSyncEvents
            ? resolvedSelectedCalendarIDs(
                calendars: calendars,
                previouslyKnownCalendars: baseState.calendars,
                settings: baseState.settings
            )
            : baseState.settings.selectedCalendarIDs
        let selectedTaskListIDs = shouldSyncTasks
            ? resolvedSelectedTaskListIDs(taskLists: taskLists, settings: baseState.settings)
            : baseState.settings.selectedTaskListIDs
        let selectedTaskLists = shouldSyncTasks ? taskLists.filter { selectedTaskListIDs.contains($0.id) } : []

        async let loadedTasks = shouldSyncTasks
            ? listTasks(
                for: selectedTaskLists,
                accountID: accountID,
                checkpointIndex: checkpointIndex,
                syncStartedAt: syncStartedAt,
                completedTaskRetentionDaysBack: baseState.settings.completedTaskRetentionDaysBack
            )
            : []
        async let loadedEvents = shouldSyncEvents
            ? listEvents(
                for: calendars,
                selectedCalendarIDs: selectedCalendarIDs,
                accountID: accountID,
                checkpointIndex: checkpointIndex,
                syncStartedAt: syncStartedAt,
                eventRetentionDaysBack: baseState.settings.eventRetentionDaysBack
            )
            : []

        let taskResults = try await loadedTasks
        let eventResults = try await loadedEvents
        let updatedCheckpoints = taskResults.map(\.checkpoint) + eventResults.map(\.checkpoint)

        var settings = baseState.settings
        settings.syncMode = mode
        if shouldSyncEvents {
            settings.selectedCalendarIDs = selectedCalendarIDs
            settings.hasConfiguredCalendarSelection = baseState.settings.hasConfiguredCalendarSelection
        }
        if shouldSyncTasks {
            settings.selectedTaskListIDs = selectedTaskListIDs
            settings.hasConfiguredTaskListSelection = baseState.settings.hasConfiguredTaskListSelection
                || baseState.settings.selectedTaskListIDs.isEmpty == false
                || selectedTaskListIDs.isEmpty == false
        }

        let syncedState = CachedAppState(
            account: baseState.account,
            accounts: baseState.accounts,
            activeAccountID: baseState.activeAccountID,
            accountWorkspaces: baseState.accountWorkspaces,
            taskLists: taskLists,
            tasks: shouldSyncTasks
                ? mergeTasks(
                    existing: baseState.tasks,
                    results: taskResults,
                    completedTaskRetentionDaysBack: settings.completedTaskRetentionDaysBack,
                    now: syncStartedAt
                )
                : baseState.tasks,
            calendars: calendars.map { calendar in
                var calendar = calendar
                if shouldSyncEvents {
                    calendar.isSelected = selectedCalendarIDs.contains(calendar.id)
                }
                return calendar
            },
            events: shouldSyncEvents
                ? mergeEvents(existing: baseState.events, results: eventResults, retentionDaysBack: settings.eventRetentionDaysBack, now: syncStartedAt)
                : baseState.events,
            settings: settings,
            syncCheckpoints: mergeCheckpoints(
                existing: baseState.syncCheckpoints,
                updated: updatedCheckpoints,
                accountID: accountID
            ),
            pendingMutations: baseState.pendingMutations
        )

        let changeSet = makeChangeSet(
            baseState: baseState,
            syncedState: syncedState,
            syncedTaskLists: shouldSyncTasks ? taskLists : nil,
            taskResults: taskResults,
            syncedCalendars: shouldSyncEvents ? calendars : nil,
            eventResults: eventResults,
            updatedCheckpoints: updatedCheckpoints
        )

        return SyncApplyResult(state: syncedState, changeSet: changeSet)
    }

#if DEBUG
    func profileEventSyncForBenchmark(mode: SyncMode, baseState: CachedAppState) async throws -> SyncSchedulerEventSyncProfile {
        guard let accountID = baseState.account?.id else {
            return SyncSchedulerEventSyncProfile.empty
        }

        let totalStart = DispatchTime.now().uptimeNanoseconds
        let syncStartedAt = Date()
        let checkpointStart = DispatchTime.now().uptimeNanoseconds
        let checkpointIndex = indexCheckpoints(baseState.syncCheckpoints)
        let checkpointIndexEnd = DispatchTime.now().uptimeNanoseconds

        let calendarListStart = DispatchTime.now().uptimeNanoseconds
        let calendars = try await calendarClient.listCalendars()
        let calendarListEnd = DispatchTime.now().uptimeNanoseconds

        let selectionStart = DispatchTime.now().uptimeNanoseconds
        let selectedCalendarIDs = resolvedSelectedCalendarIDs(
            calendars: calendars,
            previouslyKnownCalendars: baseState.calendars,
            settings: baseState.settings
        )
        let resolvedIDs = selectedCalendarIDs.isEmpty
            ? Set(calendars.filter(\.isSelected).map(\.id))
            : selectedCalendarIDs
        let selectedCalendars = calendars.filter { resolvedIDs.contains($0.id) }
        let selectionEnd = DispatchTime.now().uptimeNanoseconds

        let pageCollectionStart = DispatchTime.now().uptimeNanoseconds
        let eventResults = try await listEventsForBenchmark(
            selectedCalendars: selectedCalendars,
            accountID: accountID,
            checkpointIndex: checkpointIndex,
            syncStartedAt: syncStartedAt,
            eventRetentionDaysBack: baseState.settings.eventRetentionDaysBack
        )
        let pageCollectionEnd = DispatchTime.now().uptimeNanoseconds

        let checkpointCollectStart = DispatchTime.now().uptimeNanoseconds
        let updatedCheckpoints = eventResults.map(\.checkpoint)
        let checkpointCollectEnd = DispatchTime.now().uptimeNanoseconds

        var settings = baseState.settings
        settings.syncMode = mode
        settings.selectedCalendarIDs = selectedCalendarIDs
        settings.hasConfiguredCalendarSelection = baseState.settings.hasConfiguredCalendarSelection

        let calendarMapStart = DispatchTime.now().uptimeNanoseconds
        let mappedCalendars = calendars.map { calendar in
            var calendar = calendar
            calendar.isSelected = selectedCalendarIDs.contains(calendar.id)
            return calendar
        }
        let calendarMapEnd = DispatchTime.now().uptimeNanoseconds

        let mergeStart = DispatchTime.now().uptimeNanoseconds
        let (mergedEvents, mergeProfile) = mergeEventsProfiledForBenchmark(
            existing: baseState.events,
            results: eventResults,
            retentionDaysBack: settings.eventRetentionDaysBack,
            now: syncStartedAt
        )
        let mergeEnd = DispatchTime.now().uptimeNanoseconds

        let checkpointMergeStart = DispatchTime.now().uptimeNanoseconds
        let mergedCheckpoints = mergeCheckpoints(
            existing: baseState.syncCheckpoints,
            updated: updatedCheckpoints,
            accountID: accountID
        )
        let checkpointMergeEnd = DispatchTime.now().uptimeNanoseconds

        let stateBuildStart = DispatchTime.now().uptimeNanoseconds
        let syncedState = CachedAppState(
            account: baseState.account,
            accounts: baseState.accounts,
            activeAccountID: baseState.activeAccountID,
            accountWorkspaces: baseState.accountWorkspaces,
            taskLists: baseState.taskLists,
            tasks: baseState.tasks,
            calendars: mappedCalendars,
            events: mergedEvents,
            settings: settings,
            syncCheckpoints: mergedCheckpoints,
            pendingMutations: baseState.pendingMutations
        )
        _ = syncedState.events.count
        let stateBuildEnd = DispatchTime.now().uptimeNanoseconds

        return SyncSchedulerEventSyncProfile(
            calendarCount: calendars.count,
            selectedCalendarCount: selectedCalendars.count,
            resultEventCount: eventResults.reduce(0) { $0 + $1.events.count },
            mergedEventCount: mergedEvents.count,
            checkpointCount: mergedCheckpoints.count,
            checkpointIndexMilliseconds: Self.milliseconds(from: checkpointStart, to: checkpointIndexEnd),
            calendarListMilliseconds: Self.milliseconds(from: calendarListStart, to: calendarListEnd),
            calendarFilteringMilliseconds: Self.milliseconds(from: selectionStart, to: selectionEnd),
            eventPageCollectionMilliseconds: Self.milliseconds(from: pageCollectionStart, to: pageCollectionEnd),
            checkpointCollectMilliseconds: Self.milliseconds(from: checkpointCollectStart, to: checkpointCollectEnd),
            calendarMapMilliseconds: Self.milliseconds(from: calendarMapStart, to: calendarMapEnd),
            eventMergeMilliseconds: Self.milliseconds(from: mergeStart, to: mergeEnd),
            checkpointMergeMilliseconds: Self.milliseconds(from: checkpointMergeStart, to: checkpointMergeEnd),
            stateBuildMilliseconds: Self.milliseconds(from: stateBuildStart, to: stateBuildEnd),
            totalMilliseconds: Self.milliseconds(from: totalStart, to: stateBuildEnd),
            merge: mergeProfile
        )
    }

    private func listEventsForBenchmark(
        selectedCalendars: [CalendarListMirror],
        accountID: GoogleAccount.ID,
        checkpointIndex: [String: SyncCheckpoint],
        syncStartedAt: Date,
        eventRetentionDaysBack: Int
    ) async throws -> [CalendarSyncResult] {
        let fullSyncTimeMin = Self.retentionLowerBound(daysBack: eventRetentionDaysBack, now: syncStartedAt)
        let calendarClient = calendarClient
        return try await withThrowingTaskGroup(of: CalendarSyncResult?.self) { group in
            var iter = selectedCalendars.makeIterator()
            func scheduleNext() {
                guard let calendar = iter.next() else { return }
                let calendarID = calendar.id
                let cp = checkpoint(
                    accountID: accountID,
                    resourceType: .calendar,
                    resourceID: calendarID,
                    checkpointIndex: checkpointIndex
                )
                group.addTask {
                    let hadSyncToken = cp?.calendarSyncToken?.isEmpty == false
                    let page: GoogleCalendarEventsPage
                    let didFullSync: Bool
                    do {
                        page = try await calendarClient.listEvents(
                            calendarID: calendarID,
                            syncToken: cp?.calendarSyncToken,
                            timeMin: cp?.calendarSyncToken == nil ? fullSyncTimeMin : nil,
                            defaultTimeZoneID: calendar.timeZoneID
                        )
                        didFullSync = hadSyncToken == false
                    } catch GoogleAPIError.httpStatus(410, _) {
                        do {
                            page = try await calendarClient.listEvents(
                                calendarID: calendarID,
                                syncToken: nil,
                                timeMin: fullSyncTimeMin,
                                defaultTimeZoneID: calendar.timeZoneID
                            )
                        } catch GoogleAPIError.httpStatus(404, _) {
                            return nil
                        }
                        didFullSync = true
                    } catch GoogleAPIError.httpStatus(404, _) {
                        return nil
                    }

                    let nextCheckpoint = SyncCheckpoint(
                        id: SyncCheckpoint.stableID(
                            accountID: accountID,
                            resourceType: .calendar,
                            resourceID: calendarID
                        ),
                        accountID: accountID,
                        resourceType: .calendar,
                        resourceID: calendarID,
                        calendarSyncToken: page.nextSyncToken ?? cp?.calendarSyncToken,
                        tasksUpdatedMin: nil,
                        lastSuccessfulSyncAt: syncStartedAt
                    )
                    return CalendarSyncResult(
                        calendarID: calendarID,
                        events: page.events,
                        didFullSync: didFullSync,
                        checkpoint: nextCheckpoint
                    )
                }
            }

            for _ in 0..<Self.maxConcurrentSyncRequests {
                scheduleNext()
            }

            var results: [CalendarSyncResult] = []
            for try await result in group {
                if let result {
                    results.append(result)
                }
                scheduleNext()
            }
            return results
        }
    }

    private func mergeEventsProfiledForBenchmark(
        existing: [CalendarEventMirror],
        results: [CalendarSyncResult],
        retentionDaysBack: Int,
        now: Date
    ) -> ([CalendarEventMirror], SyncSchedulerEventMergeProfile) {
        let totalStart = DispatchTime.now().uptimeNanoseconds
        let fullSyncStart = DispatchTime.now().uptimeNanoseconds
        let fullSyncCalendarIDs = Set(results.filter(\.didFullSync).map(\.calendarID))
        let fullSyncEnd = DispatchTime.now().uptimeNanoseconds

        let resultCountStart = DispatchTime.now().uptimeNanoseconds
        let remoteEventCount = results.reduce(0) { $0 + $1.events.count }
        let resultCountEnd = DispatchTime.now().uptimeNanoseconds

        let dictionaryStart = DispatchTime.now().uptimeNanoseconds
        var eventsByID: [CalendarEventMirror.ID: CalendarEventMirror] = [:]
        eventsByID.reserveCapacity(existing.count + remoteEventCount)
        var eventOrder: [CalendarEventMirror.ID] = []
        eventOrder.reserveCapacity(existing.count + remoteEventCount)
        let dictionaryEnd = DispatchTime.now().uptimeNanoseconds

        func upsert(_ event: CalendarEventMirror) {
            if eventsByID[event.id] == nil {
                eventOrder.append(event.id)
            }
            eventsByID[event.id] = event
        }

        let preserveStart = DispatchTime.now().uptimeNanoseconds
        for event in existing {
            let isPending = OptimisticID.isPending(event.id)
            let preservedByIncrementalSync = fullSyncCalendarIDs.contains(event.calendarID) == false
            if isPending || preservedByIncrementalSync {
                upsert(event)
            }
        }
        let preserveEnd = DispatchTime.now().uptimeNanoseconds

        let upsertStart = DispatchTime.now().uptimeNanoseconds
        for result in results {
            for event in result.events {
                upsert(event)
            }
        }
        let upsertEnd = DispatchTime.now().uptimeNanoseconds

        let cutoffStart = DispatchTime.now().uptimeNanoseconds
        let cutoff: Date? = {
            guard retentionDaysBack > 0 else { return nil }
            return Calendar.current.date(byAdding: .day, value: -retentionDaysBack, to: now)
        }()
        let cutoffEnd = DispatchTime.now().uptimeNanoseconds

        let filterStart = DispatchTime.now().uptimeNanoseconds
        var filtered: [CalendarEventMirror] = []
        filtered.reserveCapacity(eventsByID.count)
        for id in eventOrder {
            guard let event = eventsByID[id],
                  event.status != .cancelled
            else { continue }
            if let cutoff,
               OptimisticID.isPending(event.id) == false,
               event.endDate < cutoff,
               (event.updatedAt ?? .distantPast) < cutoff {
                continue
            }
            filtered.append(event)
        }
        let filterEnd = DispatchTime.now().uptimeNanoseconds

        let sortCheckStart = DispatchTime.now().uptimeNanoseconds
        let needsSort = eventsAreSortedByStartDate(filtered) == false
        let sortCheckEnd = DispatchTime.now().uptimeNanoseconds

        let sortStart = DispatchTime.now().uptimeNanoseconds
        if needsSort {
            filtered.sort { lhs, rhs in
                lhs.startDate < rhs.startDate
            }
        }
        let sortEnd = DispatchTime.now().uptimeNanoseconds

        return (
            filtered,
            SyncSchedulerEventMergeProfile(
                existingCount: existing.count,
                remoteEventCount: remoteEventCount,
                dictionaryCount: eventsByID.count,
                outputCount: filtered.count,
                fullSyncIDMilliseconds: Self.milliseconds(from: fullSyncStart, to: fullSyncEnd),
                resultCountMilliseconds: Self.milliseconds(from: resultCountStart, to: resultCountEnd),
                dictionarySetupMilliseconds: Self.milliseconds(from: dictionaryStart, to: dictionaryEnd),
                preserveExistingMilliseconds: Self.milliseconds(from: preserveStart, to: preserveEnd),
                upsertRemoteMilliseconds: Self.milliseconds(from: upsertStart, to: upsertEnd),
                cutoffMilliseconds: Self.milliseconds(from: cutoffStart, to: cutoffEnd),
                filterMilliseconds: Self.milliseconds(from: filterStart, to: filterEnd),
                sortCheckMilliseconds: Self.milliseconds(from: sortCheckStart, to: sortCheckEnd),
                sortMilliseconds: Self.milliseconds(from: sortStart, to: sortEnd),
                didSort: needsSort,
                totalMilliseconds: Self.milliseconds(from: totalStart, to: sortEnd)
            )
        )
    }

    private static func milliseconds(from start: UInt64, to end: UInt64) -> Double {
        Double(end - start) / 1_000_000
    }
#endif

    // Concurrency cap for per-resource fan-out (task lists, calendars).
    // Before: one child task per resource, no ceiling. Users with many
    // shared calendars or task lists triggered 15+ parallel HTTP requests,
    // competing for CPU/network and occasionally hitting Google rate
    // limits. 5 is enough to hide latency behind one another without
    // flooding.
    private static let maxConcurrentSyncRequests = 5

    private func listTasks(
        for taskLists: [TaskListMirror],
        accountID: GoogleAccount.ID,
        checkpointIndex: [String: SyncCheckpoint],
        syncStartedAt: Date,
        completedTaskRetentionDaysBack: Int
    ) async throws -> [TaskListSyncResult] {
        let tasksClient = tasksClient
        let completedMin = Self.retentionLowerBound(daysBack: completedTaskRetentionDaysBack, now: syncStartedAt)
        return try await withThrowingTaskGroup(of: TaskListSyncResult.self) { group in
            var iter = taskLists.makeIterator()
            // Seed the window with up to `maxConcurrentSyncRequests` children.
            // As each finishes we add one more from the iterator until all are
            // dispatched — keeps in-flight work bounded without losing any
            // resource from the batch.
            var enqueued = 0
            while enqueued < Self.maxConcurrentSyncRequests, let taskList = iter.next() {
                let checkpoint = checkpoint(
                    accountID: accountID,
                    resourceType: .taskList,
                    resourceID: taskList.id,
                    checkpointIndex: checkpointIndex
                )

                group.addTask {
                    let page = try await tasksClient.listTasks(
                        taskListID: taskList.id,
                        updatedMin: checkpoint?.tasksUpdatedMin,
                        completedMin: checkpoint?.tasksUpdatedMin == nil ? completedMin : nil
                    )
                    // Prefer Google's server Date — no clock-drift exposure.
                    // Fallback path uses local clock minus the slack for the
                    // rare case where the header is absent.
                    let nextWatermark: Date = page.serverDate
                        ?? syncStartedAt.addingTimeInterval(-Self.tasksWatermarkSlackSeconds)
                    let nextCheckpoint = SyncCheckpoint(
                        id: SyncCheckpoint.stableID(
                            accountID: accountID,
                            resourceType: .taskList,
                            resourceID: taskList.id
                        ),
                        accountID: accountID,
                        resourceType: .taskList,
                        resourceID: taskList.id,
                        calendarSyncToken: nil,
                        tasksUpdatedMin: nextWatermark,
                        lastSuccessfulSyncAt: Date()
                    )
                    return TaskListSyncResult(
                        taskListID: taskList.id,
                        tasks: page.tasks,
                        didFullSync: checkpoint?.tasksUpdatedMin == nil,
                        checkpoint: nextCheckpoint
                    )
                }
                enqueued += 1
            }

            var results: [TaskListSyncResult] = []
            for try await result in group {
                results.append(result)
                // Maintain the sliding window: every time a child finishes
                // and we collect its result, add the next pending task list.
                if let nextList = iter.next() {
                    let checkpoint = checkpoint(
                        accountID: accountID,
                        resourceType: .taskList,
                        resourceID: nextList.id,
                        checkpointIndex: checkpointIndex
                    )
                    group.addTask {
                        let page = try await tasksClient.listTasks(
                            taskListID: nextList.id,
                            updatedMin: checkpoint?.tasksUpdatedMin,
                            completedMin: checkpoint?.tasksUpdatedMin == nil ? completedMin : nil
                        )
                        let nextWatermark: Date = page.serverDate
                            ?? syncStartedAt.addingTimeInterval(-Self.tasksWatermarkSlackSeconds)
                        let nextCheckpoint = SyncCheckpoint(
                            id: SyncCheckpoint.stableID(
                                accountID: accountID,
                                resourceType: .taskList,
                                resourceID: nextList.id
                            ),
                            accountID: accountID,
                            resourceType: .taskList,
                            resourceID: nextList.id,
                            calendarSyncToken: nil,
                            tasksUpdatedMin: nextWatermark,
                            lastSuccessfulSyncAt: Date()
                        )
                        return TaskListSyncResult(
                            taskListID: nextList.id,
                            tasks: page.tasks,
                            didFullSync: checkpoint?.tasksUpdatedMin == nil,
                            checkpoint: nextCheckpoint
                        )
                    }
                }
            }
            return results
        }
    }

    private func listEvents(
        for calendars: [CalendarListMirror],
        selectedCalendarIDs: Set<CalendarListMirror.ID>,
        accountID: GoogleAccount.ID,
        checkpointIndex: [String: SyncCheckpoint],
        syncStartedAt: Date,
        eventRetentionDaysBack: Int
    ) async throws -> [CalendarSyncResult] {
        let resolvedIDs = selectedCalendarIDs.isEmpty
            ? Set(calendars.filter(\.isSelected).map(\.id))
            : selectedCalendarIDs
        let selectedCalendars = calendars.filter { resolvedIDs.contains($0.id) }
        let fullSyncTimeMin = Self.retentionLowerBound(daysBack: eventRetentionDaysBack, now: syncStartedAt)
        let calendarClient = calendarClient

        return try await withThrowingTaskGroup(of: CalendarSyncResult?.self) { group in
            // Same sliding-window concurrency cap as listTasks.
            var iter = selectedCalendars.makeIterator()
            func scheduleNext() {
                guard let calendar = iter.next() else { return }
                let calendarID = calendar.id
                let baseMetadata = Self.calendarSyncMetadata(calendar)
                let cp = checkpoint(
                    accountID: accountID,
                    resourceType: .calendar,
                    resourceID: calendarID,
                    checkpointIndex: checkpointIndex
                )
                group.addTask {
                    let hadSyncToken = cp?.calendarSyncToken?.isEmpty == false
                    let page: GoogleCalendarEventsPage
                    let didFullSync: Bool

                    AppLogger.info("calendar events list start", category: .sync, metadata: baseMetadata)
                    do {
                        page = try await calendarClient.listEvents(
                            calendarID: calendarID,
                            syncToken: cp?.calendarSyncToken,
                            timeMin: cp?.calendarSyncToken == nil ? fullSyncTimeMin : nil,
                            defaultTimeZoneID: calendar.timeZoneID
                        )
                        didFullSync = hadSyncToken == false
                    } catch GoogleAPIError.httpStatus(410, _) {
                        do {
                            page = try await calendarClient.listEvents(
                                calendarID: calendarID,
                                syncToken: nil,
                                timeMin: fullSyncTimeMin,
                                defaultTimeZoneID: calendar.timeZoneID
                            )
                        } catch GoogleAPIError.httpStatus(404, let body) {
                            AppLogger.warn(
                                "calendar events list skipped",
                                category: .sync,
                                metadata: baseMetadata.merging(Self.syncErrorMetadata(GoogleAPIError.httpStatus(404, body), stage: "calendar.events.list")) { lhs, _ in lhs }
                            )
                            return nil
                        }
                        didFullSync = true
                    } catch GoogleAPIError.httpStatus(404, let body) {
                        AppLogger.warn(
                            "calendar events list skipped",
                            category: .sync,
                            metadata: baseMetadata.merging(Self.syncErrorMetadata(GoogleAPIError.httpStatus(404, body), stage: "calendar.events.list")) { lhs, _ in lhs }
                        )
                        return nil
                    } catch {
                        AppLogger.error(
                            "calendar events list failed",
                            category: .sync,
                            metadata: baseMetadata.merging(Self.syncErrorMetadata(error, stage: "calendar.events.list")) { lhs, _ in lhs }
                        )
                        throw error
                    }

                    let nextCheckpoint = SyncCheckpoint(
                        id: SyncCheckpoint.stableID(
                            accountID: accountID,
                            resourceType: .calendar,
                            resourceID: calendarID
                        ),
                        accountID: accountID,
                        resourceType: .calendar,
                        resourceID: calendarID,
                        calendarSyncToken: page.nextSyncToken ?? cp?.calendarSyncToken,
                        tasksUpdatedMin: nil,
                        lastSuccessfulSyncAt: syncStartedAt
                    )
                    AppLogger.info("calendar events list succeeded", category: .sync, metadata: baseMetadata.merging([
                        "events": String(page.events.count),
                        "fullSync": String(didFullSync)
                    ]) { lhs, _ in lhs })
                    return CalendarSyncResult(
                        calendarID: calendarID,
                        events: page.events,
                        didFullSync: didFullSync,
                        checkpoint: nextCheckpoint
                    )
                }
            }

            for _ in 0..<Self.maxConcurrentSyncRequests {
                scheduleNext()
            }

            var results: [CalendarSyncResult] = []
            for try await result in group {
                if let result {
                    results.append(result)
                }
                scheduleNext()
            }
            return results
        }
    }

    private static func calendarSyncMetadata(_ calendar: CalendarListMirror) -> [String: String] {
        [
            "stage": "calendar.events.list",
            "calendar": calendar.summary,
            "calendarID": GoogleDiagnostics.redactedIdentifier(calendar.id),
            "accessRole": calendar.accessRole,
            "googleSelected": String(calendar.isSelected)
        ]
    }

    private static func syncErrorMetadata(_ error: Error, stage: String) -> [String: String] {
        GoogleDiagnostics.errorMetadata(error, stage: stage)
    }

    private func resolvedSelectedCalendarIDs(
        calendars: [CalendarListMirror],
        previouslyKnownCalendars: [CalendarListMirror],
        settings: AppSettings
    ) -> Set<CalendarListMirror.ID> {
        let availableIDs = Set(calendars.map(\.id))
        if settings.hasConfiguredCalendarSelection {
            let requestedIDs = settings.selectedCalendarIDs.intersection(availableIDs)
            let knownIDs = Set(previouslyKnownCalendars.map(\.id))
            guard knownIDs.isEmpty == false else {
                return requestedIDs
            }

            // Preserve explicit HCB exclusions, but do not strand a calendar
            // that was just created in Google and already marked visible there.
            let newlySelectedIDs = Set(
                calendars
                    .filter { calendar in
                        calendar.isSelected && knownIDs.contains(calendar.id) == false
                    }
                    .map(\.id)
            )
            return requestedIDs.union(newlySelectedIDs)
        }

        return Set(calendars.filter(\.isSelected).map(\.id))
    }

    private static func retentionLowerBound(daysBack: Int, now: Date) -> Date? {
        guard daysBack > 0 else { return nil }
        return Calendar.current.date(byAdding: .day, value: -daysBack, to: now)
    }

    private func resolvedSelectedTaskListIDs(
        taskLists: [TaskListMirror],
        settings: AppSettings
    ) -> Set<TaskListMirror.ID> {
        let availableIDs = Set(taskLists.map(\.id))
        let requestedIDs = settings.selectedTaskListIDs.intersection(availableIDs)

        if settings.hasConfiguredTaskListSelection {
            return requestedIDs
        }

        return requestedIDs.isEmpty ? availableIDs : requestedIDs
    }

    private func checkpoint(
        accountID: GoogleAccount.ID,
        resourceType: SyncResourceType,
        resourceID: String,
        checkpointIndex: [String: SyncCheckpoint]
    ) -> SyncCheckpoint? {
        checkpointIndex[SyncCheckpoint.stableID(
            accountID: accountID,
            resourceType: resourceType,
            resourceID: resourceID
        )]
    }

    private func indexCheckpoints(_ checkpoints: [SyncCheckpoint]) -> [SyncCheckpoint.ID: SyncCheckpoint] {
        var checkpointsByID: [SyncCheckpoint.ID: SyncCheckpoint] = [:]

        for checkpoint in checkpoints {
            checkpointsByID[checkpoint.id] = checkpoint
        }

        return checkpointsByID
    }

    private func makeChangeSet(
        baseState: CachedAppState,
        syncedState: CachedAppState,
        syncedTaskLists: [TaskListMirror]?,
        taskResults: [TaskListSyncResult],
        syncedCalendars: [CalendarListMirror]?,
        eventResults: [CalendarSyncResult],
        updatedCheckpoints: [SyncCheckpoint]
    ) -> SyncChangeSet {
        var changeSet = SyncChangeSet.empty

        if let syncedTaskLists {
            let touchedIDs = Set(syncedTaskLists.map(\.id))
            let finalIDs = Set(syncedState.taskLists.map(\.id))
            let deletedIDs = Set(baseState.taskLists.map(\.id)).subtracting(finalIDs)
            changeSet.taskLists = Self.rowChanges(
                existing: baseState.taskLists,
                final: syncedState.taskLists,
                touchedIDs: touchedIDs,
                deletedIDs: deletedIDs,
                kind: "taskList",
                etag: \.etag
            )
        }

        let syncedTaskListIDs = Set(taskResults.map(\.taskListID))
        if syncedTaskListIDs.isEmpty == false {
            let touchedIDs = Set(taskResults.flatMap(\.tasks).map(\.id))
            let finalIDs = Set(syncedState.tasks.map(\.id))
            let deletedIDs = Set<String>(baseState.tasks.compactMap { task in
                guard syncedTaskListIDs.contains(task.taskListID),
                      OptimisticID.isPending(task.id) == false,
                      finalIDs.contains(task.id) == false
                else { return nil }
                return task.id
            })
            changeSet.tasks = Self.rowChanges(
                existing: baseState.tasks,
                final: syncedState.tasks,
                touchedIDs: touchedIDs,
                deletedIDs: deletedIDs,
                kind: "task",
                etag: \.etag
            )
        }

        if let syncedCalendars {
            let touchedIDs = Set(syncedCalendars.map(\.id))
            let finalIDs = Set(syncedState.calendars.map(\.id))
            let deletedIDs = Set(baseState.calendars.map(\.id)).subtracting(finalIDs)
            changeSet.calendars = Self.rowChanges(
                existing: baseState.calendars,
                final: syncedState.calendars,
                touchedIDs: touchedIDs,
                deletedIDs: deletedIDs,
                kind: "calendar",
                etag: \.etag
            )
        }

        let syncedCalendarIDs = Set(eventResults.map(\.calendarID))
        if syncedCalendarIDs.isEmpty == false {
            let touchedIDs = Set(eventResults.flatMap(\.events).map(\.id))
            let finalIDs = Set(syncedState.events.map(\.id))
            let deletedIDs = Set<String>(baseState.events.compactMap { event in
                guard syncedCalendarIDs.contains(event.calendarID),
                      OptimisticID.isPending(event.id) == false,
                      finalIDs.contains(event.id) == false
                else { return nil }
                return event.id
            })
            changeSet.events = Self.rowChanges(
                existing: baseState.events,
                final: syncedState.events,
                touchedIDs: touchedIDs,
                deletedIDs: deletedIDs,
                kind: "event",
                etag: \.etag
            )
        }

        let touchedCheckpointIDs = Set(updatedCheckpoints.map(\.id))
        changeSet.checkpoints = Self.rowChanges(
            existing: baseState.syncCheckpoints,
            final: syncedState.syncCheckpoints,
            touchedIDs: touchedCheckpointIDs,
            deletedIDs: [],
            kind: "syncCheckpoint",
            etag: { _ in nil }
        )
        changeSet.checkpointChanged = changeSet.checkpoints.hasChanges
        changeSet.settingsChanged = baseState.settings != syncedState.settings

        changeSet.affectedTaskListIDs.formUnion(changeSet.taskLists.inserted)
        changeSet.affectedTaskListIDs.formUnion(changeSet.taskLists.updated)
        changeSet.affectedTaskListIDs.formUnion(changeSet.taskLists.deleted)
        changeSet.affectedCalendarIDs.formUnion(changeSet.calendars.inserted)
        changeSet.affectedCalendarIDs.formUnion(changeSet.calendars.updated)
        changeSet.affectedCalendarIDs.formUnion(changeSet.calendars.deleted)

        Self.addAffectedTaskState(
            to: &changeSet,
            existing: baseState.tasks,
            final: syncedState.tasks
        )
        Self.addAffectedEventState(
            to: &changeSet,
            existing: baseState.events,
            final: syncedState.events
        )

        return changeSet
    }

    private static func rowChanges<T: Identifiable & Encodable>(
        existing: [T],
        final: [T],
        touchedIDs: Set<String>,
        deletedIDs: Set<String>,
        kind: String,
        etag: (T) -> String?
    ) -> SyncChangeSet.RowChanges where T.ID == String {
        let neededIDs = touchedIDs.union(deletedIDs)
        var existingByID: [String: T] = [:]
        var finalByID: [String: T] = [:]
        existingByID.reserveCapacity(neededIDs.count)
        finalByID.reserveCapacity(neededIDs.count)
        for row in existing where neededIDs.contains(row.id) {
            existingByID[row.id] = row
        }
        for row in final where neededIDs.contains(row.id) {
            finalByID[row.id] = row
        }
        var changes = SyncChangeSet.RowChanges()

        for id in neededIDs {
            let old = existingByID[id]
            let new = finalByID[id]
            switch (old, new) {
            case (nil, nil):
                continue
            case (nil, .some):
                changes.inserted.insert(id)
            case (.some, nil):
                changes.deleted.insert(id)
            case let (.some(oldValue), .some(newValue)):
                if rowsMatch(oldValue, newValue, kind: kind, etag: etag) {
                    changes.unchanged.insert(id)
                } else {
                    changes.updated.insert(id)
                }
            }
        }

        return changes
    }

    private static func rowsMatch<T: Encodable>(
        _ oldValue: T,
        _ newValue: T,
        kind: String,
        etag: (T) -> String?
    ) -> Bool {
        if let oldEtag = etag(oldValue), oldEtag.isEmpty == false,
           let newEtag = etag(newValue), newEtag.isEmpty == false {
            return oldEtag == newEtag
        }
        return rowHash(oldValue, kind: kind) == rowHash(newValue, kind: kind)
    }

    private static func rowHash<T: Encodable>(_ value: T, kind: String) -> String {
        (try? LocalCacheRowHasher.hash(value, kind: kind)) ?? String(describing: value)
    }

    private static func addAffectedTaskState(
        to changeSet: inout SyncChangeSet,
        existing: [TaskMirror],
        final: [TaskMirror]
    ) {
        let changedIDs = changeSet.tasks.inserted.union(changeSet.tasks.updated).union(changeSet.tasks.deleted)
        var existingByID: [TaskMirror.ID: TaskMirror] = [:]
        var finalByID: [TaskMirror.ID: TaskMirror] = [:]
        existingByID.reserveCapacity(changedIDs.count)
        finalByID.reserveCapacity(changedIDs.count)
        for task in existing where changedIDs.contains(task.id) {
            existingByID[task.id] = task
        }
        for task in final where changedIDs.contains(task.id) {
            finalByID[task.id] = task
        }
        for id in changedIDs {
            let old = existingByID[id]
            let new = finalByID[id]
            if let old {
                changeSet.affectedTaskListIDs.insert(old.taskListID)
                changeSet.affectedDayKeys.formUnion(dayKeys(forTask: old))
            }
            if let new {
                changeSet.affectedTaskListIDs.insert(new.taskListID)
                changeSet.affectedDayKeys.formUnion(dayKeys(forTask: new))
            }
        }
    }

    private static func addAffectedEventState(
        to changeSet: inout SyncChangeSet,
        existing: [CalendarEventMirror],
        final: [CalendarEventMirror]
    ) {
        let changedIDs = changeSet.events.inserted.union(changeSet.events.updated).union(changeSet.events.deleted)
        var existingByID: [CalendarEventMirror.ID: CalendarEventMirror] = [:]
        var finalByID: [CalendarEventMirror.ID: CalendarEventMirror] = [:]
        existingByID.reserveCapacity(changedIDs.count)
        finalByID.reserveCapacity(changedIDs.count)
        for event in existing where changedIDs.contains(event.id) {
            existingByID[event.id] = event
        }
        for event in final where changedIDs.contains(event.id) {
            finalByID[event.id] = event
        }
        for id in changedIDs {
            let old = existingByID[id]
            let new = finalByID[id]
            if let old {
                changeSet.affectedCalendarIDs.insert(old.calendarID)
                changeSet.affectedDayKeys.formUnion(dayKeys(forEvent: old))
            }
            if let new {
                changeSet.affectedCalendarIDs.insert(new.calendarID)
                changeSet.affectedDayKeys.formUnion(dayKeys(forEvent: new))
            }
        }
    }

    private static func dayKeys(forTask task: TaskMirror, calendar: Calendar = .current) -> Set<TimeInterval> {
        guard let dueDate = task.dueDate else { return [] }
        return [calendar.startOfDay(for: dueDate).timeIntervalSinceReferenceDate]
    }

    private static func dayKeys(forEvent event: CalendarEventMirror, calendar: Calendar = .current) -> Set<TimeInterval> {
        guard event.status != .cancelled else {
            return []
        }
        let startDay = calendar.startOfDay(for: event.startDate)
        let endDay = CalendarGridLayout.eventEndDay(event: event, calendar: calendar)
        guard startDay <= endDay else { return [] }

        var keys: Set<TimeInterval> = []
        var cursor = startDay
        var steps = 0
        while cursor <= endDay && steps < 366 {
            keys.insert(cursor.timeIntervalSinceReferenceDate)
            guard let next = calendar.date(byAdding: .day, value: 1, to: cursor) else { break }
            cursor = next
            steps += 1
        }
        return keys
    }

    private func mergeTasks(
        existing: [TaskMirror],
        results: [TaskListSyncResult],
        completedTaskRetentionDaysBack: Int,
        now: Date
    ) -> [TaskMirror] {
        let fullSyncTaskListIDs = Set(results.filter(\.didFullSync).map(\.taskListID))
        var tasksByID: [TaskMirror.ID: TaskMirror] = [:]

        for task in existing {
            let isPending = OptimisticID.isPending(task.id)
            let preservedByIncrementalSync = fullSyncTaskListIDs.contains(task.taskListID) == false
            // Optimistic local-ID tasks must survive full syncs — they will never be in the
            // server response until their create mutation lands, and dropping them here leaves
            // the row invisible until the next replay attempt.
            if isPending || preservedByIncrementalSync {
                tasksByID[task.id] = task
            }
        }

        for task in results.flatMap(\.tasks) {
            tasksByID[task.id] = task
        }

        let cutoff = Self.retentionLowerBound(daysBack: completedTaskRetentionDaysBack, now: now)

        return tasksByID.values
            .filter { $0.isDeleted == false } // purge tombstones post-merge
            .filter { task in
                guard let cutoff else { return true }
                if OptimisticID.isPending(task.id) { return true }
                guard task.isCompleted else { return true }
                if let completedAt = task.completedAt, completedAt >= cutoff { return true }
                if let updatedAt = task.updatedAt, updatedAt >= cutoff { return true }
                return false
            }
            .sorted { lhs, rhs in
                (lhs.dueDate ?? lhs.updatedAt ?? .distantFuture) < (rhs.dueDate ?? rhs.updatedAt ?? .distantFuture)
            }
    }

    private func mergeEvents(
        existing: [CalendarEventMirror],
        results: [CalendarSyncResult],
        retentionDaysBack: Int,
        now: Date
    ) -> [CalendarEventMirror] {
        let fullSyncCalendarIDs = Set(results.filter(\.didFullSync).map(\.calendarID))
        var eventsByID: [CalendarEventMirror.ID: CalendarEventMirror] = [:]
        let remoteEventCount = results.reduce(0) { $0 + $1.events.count }
        eventsByID.reserveCapacity(existing.count + remoteEventCount)
        var eventOrder: [CalendarEventMirror.ID] = []
        eventOrder.reserveCapacity(existing.count + remoteEventCount)

        func upsert(_ event: CalendarEventMirror) {
            if eventsByID[event.id] == nil {
                eventOrder.append(event.id)
            }
            eventsByID[event.id] = event
        }

        for event in existing {
            let isPending = OptimisticID.isPending(event.id)
            let preservedByIncrementalSync = fullSyncCalendarIDs.contains(event.calendarID) == false
            if isPending || preservedByIncrementalSync {
                upsert(event)
            }
        }

        for result in results {
            for event in result.events {
                upsert(event)
            }
        }

        // §7.02: drop events whose endDate is older than the retention window.
        // `retentionDaysBack <= 0` disables pruning (keep-forever). Optimistic
        // writes (pending-id) are ALWAYS kept so a local mutation from earlier
        // today isn't torched before it's confirmed by Google. Same reasoning
        // applies to recently-updated events — ignore the end-in-past rule if
        // the event was mutated on Google within the retention window (via
        // `updatedAt`), because users frequently reopen past meetings to edit
        // notes / action items.
        let cutoff: Date? = {
            guard retentionDaysBack > 0 else { return nil }
            return Calendar.current.date(byAdding: .day, value: -retentionDaysBack, to: now)
        }()

        var merged: [CalendarEventMirror] = []
        merged.reserveCapacity(eventsByID.count)
        for id in eventOrder {
            guard let event = eventsByID[id],
                  event.status != .cancelled
            else { continue }
            if let cutoff,
               OptimisticID.isPending(event.id) == false,
               event.endDate < cutoff,
               (event.updatedAt ?? .distantPast) < cutoff {
                continue
            }
            merged.append(event)
        }

        if eventsAreSortedByStartDate(merged) == false {
            merged.sort { lhs, rhs in
                lhs.startDate < rhs.startDate
            }
        }
        return merged
    }

    private func eventsAreSortedByStartDate(_ events: [CalendarEventMirror]) -> Bool {
        guard events.count > 1 else { return true }
        for index in 1..<events.count where events[index].startDate < events[index - 1].startDate {
            return false
        }
        return true
    }

    private func mergeCheckpoints(
        existing: [SyncCheckpoint],
        updated: [SyncCheckpoint],
        accountID: GoogleAccount.ID
    ) -> [SyncCheckpoint] {
        var checkpointsByID: [SyncCheckpoint.ID: SyncCheckpoint] = [:]

        for checkpoint in existing {
            checkpointsByID[checkpoint.id] = checkpoint
        }

        for checkpoint in updated where checkpoint.accountID == accountID {
            checkpointsByID[checkpoint.id] = checkpoint
        }

        return checkpointsByID.values.sorted { $0.id < $1.id }
    }
}

private struct TaskListSyncResult: Sendable {
    var taskListID: TaskListMirror.ID
    var tasks: [TaskMirror]
    var didFullSync: Bool
    var checkpoint: SyncCheckpoint
}

private struct CalendarSyncResult: Sendable {
    var calendarID: CalendarListMirror.ID
    var events: [CalendarEventMirror]
    var didFullSync: Bool
    var checkpoint: SyncCheckpoint
}

#if DEBUG
struct SyncSchedulerEventSyncProfile: Sendable {
    var calendarCount: Int
    var selectedCalendarCount: Int
    var resultEventCount: Int
    var mergedEventCount: Int
    var checkpointCount: Int
    var checkpointIndexMilliseconds: Double
    var calendarListMilliseconds: Double
    var calendarFilteringMilliseconds: Double
    var eventPageCollectionMilliseconds: Double
    var checkpointCollectMilliseconds: Double
    var calendarMapMilliseconds: Double
    var eventMergeMilliseconds: Double
    var checkpointMergeMilliseconds: Double
    var stateBuildMilliseconds: Double
    var totalMilliseconds: Double
    var merge: SyncSchedulerEventMergeProfile

    static let empty = SyncSchedulerEventSyncProfile(
        calendarCount: 0,
        selectedCalendarCount: 0,
        resultEventCount: 0,
        mergedEventCount: 0,
        checkpointCount: 0,
        checkpointIndexMilliseconds: 0,
        calendarListMilliseconds: 0,
        calendarFilteringMilliseconds: 0,
        eventPageCollectionMilliseconds: 0,
        checkpointCollectMilliseconds: 0,
        calendarMapMilliseconds: 0,
        eventMergeMilliseconds: 0,
        checkpointMergeMilliseconds: 0,
        stateBuildMilliseconds: 0,
        totalMilliseconds: 0,
        merge: .empty
    )
}

struct SyncSchedulerEventMergeProfile: Sendable {
    var existingCount: Int
    var remoteEventCount: Int
    var dictionaryCount: Int
    var outputCount: Int
    var fullSyncIDMilliseconds: Double
    var resultCountMilliseconds: Double
    var dictionarySetupMilliseconds: Double
    var preserveExistingMilliseconds: Double
    var upsertRemoteMilliseconds: Double
    var cutoffMilliseconds: Double
    var filterMilliseconds: Double
    var sortCheckMilliseconds: Double
    var sortMilliseconds: Double
    var didSort: Bool
    var totalMilliseconds: Double

    static let empty = SyncSchedulerEventMergeProfile(
        existingCount: 0,
        remoteEventCount: 0,
        dictionaryCount: 0,
        outputCount: 0,
        fullSyncIDMilliseconds: 0,
        resultCountMilliseconds: 0,
        dictionarySetupMilliseconds: 0,
        preserveExistingMilliseconds: 0,
        upsertRemoteMilliseconds: 0,
        cutoffMilliseconds: 0,
        filterMilliseconds: 0,
        sortCheckMilliseconds: 0,
        sortMilliseconds: 0,
        didSort: false,
        totalMilliseconds: 0
    )
}
#endif
