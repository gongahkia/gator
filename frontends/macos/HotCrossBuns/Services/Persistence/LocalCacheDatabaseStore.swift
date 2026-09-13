import Foundation
import CryptoKit
import GRDB

final class LocalCacheDatabaseStore: @unchecked Sendable {
    enum CacheDatabaseError: Error, Equatable {
        case encryptedStoreLocked
        case missingEncryptionSalt
        case injectedRollback
        case missingStateRow
    }

    let fileURL: URL
    private let dbQueue: DatabaseQueue

    init(fileURL: URL) throws {
        self.fileURL = fileURL
        try FileManager.default.createDirectory(
            at: fileURL.deletingLastPathComponent(),
            withIntermediateDirectories: true
        )

        var configuration = Configuration()
        configuration.prepareDatabase { db in
            try db.execute(sql: "PRAGMA foreign_keys = ON")
            try db.execute(sql: "PRAGMA journal_mode = WAL")
            try db.execute(sql: "PRAGMA synchronous = NORMAL")
            try db.execute(sql: "PRAGMA temp_store = MEMORY")
            try db.execute(sql: "PRAGMA cache_size = -65536")
            try db.execute(sql: "PRAGMA mmap_size = 268435456")
        }
        dbQueue = try DatabaseQueue(path: fileURL.path, configuration: configuration)
        try Self.migrator.migrate(dbQueue)
    }

    var existsOnDisk: Bool {
        FileManager.default.fileExists(atPath: fileURL.path)
    }

    @discardableResult
    func save(
        _ state: CachedAppState,
        encryptionKey: SymmetricKey? = nil,
        salt: Data? = nil,
        failAfterWritingEntitiesForTesting: Bool = false
    ) throws -> LocalCacheDatabaseSaveProfile {
        let totalStart = Self.timestamp()
        let encodeStart = Self.timestamp()
        let encoded = try EncodedState(
            state: state,
            encryptionKey: encryptionKey,
            salt: salt
        )
        let encodeEnd = Self.timestamp()

        let transactionStart = Self.timestamp()
        try dbQueue.write { db in
            try Self.replaceDatabaseContents(
                db,
                encoded: encoded,
                failAfterWritingEntitiesForTesting: failAfterWritingEntitiesForTesting
            )
        }
        let transactionEnd = Self.timestamp()

        return LocalCacheDatabaseSaveProfile(
            accountRows: encoded.accounts.count,
            workspaceRows: encoded.workspaces.count,
            taskListRows: encoded.taskLists.count,
            taskRows: encoded.tasks.count,
            calendarRows: encoded.calendars.count,
            eventRows: encoded.events.count,
            checkpointRows: encoded.syncCheckpoints.count,
            pendingMutationRows: encoded.pendingMutations.count,
            encodeAndHashMilliseconds: Self.milliseconds(from: encodeStart, to: encodeEnd),
            transactionMilliseconds: Self.milliseconds(from: transactionStart, to: transactionEnd),
            totalMilliseconds: Self.milliseconds(from: totalStart, to: transactionEnd)
        )
    }

    @discardableResult
    func applySyncResult(
        _ result: SyncApplyResult,
        encryptionKey: SymmetricKey? = nil,
        salt: Data? = nil,
        failAfterWritingEntitiesForTesting: Bool = false
    ) throws -> LocalCacheDatabaseSaveProfile {
        let totalStart = Self.timestamp()
        let hasStateBeforeEncode = try dbQueue.read { db in
            try Int.fetchOne(db, sql: "SELECT COUNT(*) FROM cache_state WHERE id = 1") == 1
        }
        let encodeStart = Self.timestamp()
        let encoded = try EncodedState(
            state: result.state,
            changeSet: hasStateBeforeEncode ? result.changeSet : nil,
            encryptionKey: encryptionKey,
            salt: salt
        )
        let encodeEnd = Self.timestamp()

        let transactionStart = Self.timestamp()
        let writeCounts = try dbQueue.write { db in
            let hasState = try Int.fetchOne(db, sql: "SELECT COUNT(*) FROM cache_state WHERE id = 1") == 1
            guard hasState else {
                let fullEncoded = hasStateBeforeEncode
                    ? try EncodedState(state: result.state, encryptionKey: encryptionKey, salt: salt)
                    : encoded
                try Self.replaceDatabaseContents(
                    db,
                    encoded: fullEncoded,
                    failAfterWritingEntitiesForTesting: failAfterWritingEntitiesForTesting
                )
                return IncrementalWriteCounts(
                    upserted: fullEncoded.totalEntityRows,
                    deleted: 0,
                    unchangedSkipped: 0
                )
            }
            return try Self.applyIncrementalSyncContents(
                db,
                encoded: encoded,
                changeSet: result.changeSet,
                failAfterWritingEntitiesForTesting: failAfterWritingEntitiesForTesting
            )
        }
        let transactionEnd = Self.timestamp()

        return LocalCacheDatabaseSaveProfile(
            accountRows: encoded.accounts.count,
            workspaceRows: encoded.workspaces.count,
            taskListRows: encoded.taskLists.count,
            taskRows: encoded.tasks.count,
            calendarRows: encoded.calendars.count,
            eventRows: encoded.events.count,
            checkpointRows: encoded.syncCheckpoints.count,
            pendingMutationRows: encoded.pendingMutations.count,
            encodeAndHashMilliseconds: Self.milliseconds(from: encodeStart, to: encodeEnd),
            transactionMilliseconds: Self.milliseconds(from: transactionStart, to: transactionEnd),
            totalMilliseconds: Self.milliseconds(from: totalStart, to: transactionEnd),
            incrementalUpsertedRows: writeCounts.upserted,
            incrementalDeletedRows: writeCounts.deleted,
            incrementalUnchangedSkippedRows: writeCounts.unchangedSkipped
        )
    }

    @discardableResult
    func commit(
        state: CachedAppState,
        changeSet: CachePersistenceChangeSet,
        encryptionKey: SymmetricKey? = nil,
        salt: Data? = nil,
        failAfterWritingEntitiesForTesting: Bool = false
    ) throws -> LocalCacheDatabaseSaveProfile {
        try applySyncResult(
            SyncApplyResult(state: state, changeSet: changeSet),
            encryptionKey: encryptionKey,
            salt: salt,
            failAfterWritingEntitiesForTesting: failAfterWritingEntitiesForTesting
        )
    }

    func load(encryptionKey: SymmetricKey? = nil) throws -> CachedAppState {
        try loadProfiled(encryptionKey: encryptionKey).state
    }

    func loadProfiled(encryptionKey: SymmetricKey? = nil) throws -> (state: CachedAppState, profile: LocalCacheDatabaseLoadProfile) {
        let totalStart = Self.timestamp()
        let fetchStart = Self.timestamp()
        let decoded = try dbQueue.read { db in
            try DecodedState.fetch(db, encryptionKey: encryptionKey)
        }
        let fetchEnd = Self.timestamp()

        let rebuildStart = Self.timestamp()
        let state = decoded.cachedState()
        let rebuildEnd = Self.timestamp()

        return (
            state,
            LocalCacheDatabaseLoadProfile(
                accountRows: decoded.accounts.count,
                workspaceRows: decoded.workspaces.count,
                taskListRows: decoded.taskLists.totalValueCount,
                taskRows: decoded.tasks.totalValueCount,
                calendarRows: decoded.calendars.totalValueCount,
                eventRows: decoded.events.totalValueCount,
                checkpointRows: decoded.syncCheckpoints.totalValueCount,
                pendingMutationRows: decoded.pendingMutations.totalValueCount,
                fetchAndDecodeMilliseconds: Self.milliseconds(from: fetchStart, to: fetchEnd),
                rebuildStateMilliseconds: Self.milliseconds(from: rebuildStart, to: rebuildEnd),
                totalMilliseconds: Self.milliseconds(from: totalStart, to: rebuildEnd)
            )
        )
    }

    func repairCalendarDerivedTables(encryptionKey: SymmetricKey? = nil) throws {
        try dbQueue.write { db in
            let rows = try Self.fetchStoredEventRows(db, encryptionKey: encryptionKey)
            try Self.rebuildCalendarDerivedTables(db, rows: rows)
        }
    }

    func repairSearchAndRenderTables(encryptionKey: SymmetricKey? = nil) throws {
        try dbQueue.write { db in
            let taskRows = try Self.fetchStoredTaskRows(db, encryptionKey: encryptionKey)
            let eventRows = try Self.fetchStoredEventRows(db, encryptionKey: encryptionKey)
            try Self.rebuildSearchAndRenderTables(db, taskRows: taskRows, eventRows: eventRows)
        }
    }

    func repairSearchAndRenderTablesIfEmpty(encryptionKey: SymmetricKey? = nil) throws {
        try dbQueue.write { db in
            let derivedCount = (try Int.fetchOne(db, sql: "SELECT COUNT(*) FROM cache_task_render_index") ?? 0)
                + (try Int.fetchOne(db, sql: "SELECT COUNT(*) FROM cache_event_render_index") ?? 0)
            let sourceCount = (try Int.fetchOne(db, sql: "SELECT COUNT(*) FROM cache_tasks") ?? 0)
                + (try Int.fetchOne(db, sql: "SELECT COUNT(*) FROM cache_events") ?? 0)
            guard derivedCount == 0, sourceCount > 0 else { return }
            let taskRows = try Self.fetchStoredTaskRows(db, encryptionKey: encryptionKey)
            let eventRows = try Self.fetchStoredEventRows(db, encryptionKey: encryptionKey)
            try Self.rebuildSearchAndRenderTables(db, taskRows: taskRows, eventRows: eventRows)
        }
    }

    func searchEntities(
        accountID: String? = nil,
        query: String,
        scope: LocalCacheEntitySearchScope = .all,
        limit: Int = 40,
        encryptionKey: SymmetricKey? = nil
    ) throws -> LocalCacheEntitySearchResults {
        let storageAccountID = accountID ?? Self.unscopedAccountID
        let boundedLimit = max(1, min(limit, 100))
        return try dbQueue.read { db in
            try Self.searchEntities(
                db,
                accountID: storageAccountID,
                query: query,
                scope: scope,
                limit: boundedLimit,
                encryptionKey: encryptionKey
            )
        }
    }

    func overdueTasks(
        accountID: String? = nil,
        before cutoff: Date,
        visibleTaskListIDs: Set<TaskListMirror.ID>?,
        limit: Int,
        encryptionKey: SymmetricKey? = nil
    ) throws -> OverdueTaskProjection {
        let storageAccountID = accountID ?? Self.unscopedAccountID
        return try dbQueue.read { db in
            try Self.fetchOverdueTasks(
                db,
                accountID: storageAccountID,
                before: cutoff,
                visibleTaskListIDs: visibleTaskListIDs,
                limit: limit,
                encryptionKey: encryptionKey
            )
        }
    }

    func enqueueSideEffectRebuild(
        accountID: String? = nil,
        targets: Set<LocalIntegrationDirtyTarget> = Set(LocalIntegrationDirtyTarget.allCases)
    ) throws {
        let storageAccountID = accountID ?? Self.unscopedAccountID
        try dbQueue.write { db in
            try Self.enqueueSideEffectRebuild(db, accountID: storageAccountID, targets: targets)
        }
    }

    func sideEffectDirtyItems(
        target: LocalIntegrationDirtyTarget,
        limit: Int = 200
    ) throws -> [LocalIntegrationDirtyItem] {
        let boundedLimit = max(1, min(limit, 1_000))
        return try dbQueue.read { db in
            try Self.fetchSideEffectDirtyItems(db, target: target, limit: boundedLimit)
        }
    }

    func markSideEffectDirtyItemsProcessed(_ items: [LocalIntegrationDirtyItem]) throws {
        guard items.isEmpty == false else { return }
        try dbQueue.write { db in
            try Self.deleteSideEffectDirtyItems(db, items: items)
        }
    }

    func calendarAggregateCounts(
        accountID: String? = nil,
        selectedCalendarIDs: Set<CalendarListMirror.ID>? = nil,
        includeCancelled: Bool = false,
        colorTagBindings: [String: String] = [:]
    ) throws -> CalendarAggregateCounts {
        try dbQueue.read { db in
            try Self.fetchCalendarAggregateCounts(
                db,
                accountID: accountID ?? Self.unscopedAccountID,
                selectedCalendarIDs: selectedCalendarIDs,
                includeCancelled: includeCancelled,
                colorTagBindings: colorTagBindings
            )
        }
    }

    func calendarVisibleRangeProjection(
        accountID: String? = nil,
        kind: CalendarVisibleRangeKind,
        anchorDate: Date,
        dayCount: Int? = nil,
        selectedCalendarIDs: Set<CalendarListMirror.ID>,
        eventViewFilter: CalendarEventViewFilter = CalendarEventViewFilter(),
        includeCancelled: Bool = false,
        requiresEventDetails: Bool = false,
        encryptionKey: SymmetricKey? = nil,
        calendar: Calendar = .current
    ) throws -> CalendarVisibleRangeProjection {
        let range = Self.calendarVisibleRange(kind: kind, anchorDate: anchorDate, dayCount: dayCount, calendar: calendar)
        return try calendarVisibleRangeProjection(
            accountID: accountID,
            range: range,
            selectedCalendarIDs: selectedCalendarIDs,
            eventViewFilter: eventViewFilter,
            includeCancelled: includeCancelled,
            requiresEventDetails: requiresEventDetails,
            encryptionKey: encryptionKey,
            calendar: calendar
        )
    }

    func calendarVisibleRangeProjection(
        accountID: String? = nil,
        range: CalendarVisibleRange,
        selectedCalendarIDs: Set<CalendarListMirror.ID>,
        eventViewFilter: CalendarEventViewFilter = CalendarEventViewFilter(),
        includeCancelled: Bool = false,
        requiresEventDetails: Bool = false,
        encryptionKey: SymmetricKey? = nil,
        calendar: Calendar = .current
    ) throws -> CalendarVisibleRangeProjection {
        let storageAccountID = accountID ?? Self.unscopedAccountID
        return try dbQueue.read { db in
            let effectiveCalendarIDs = selectedCalendarIDs.intersection(eventViewFilter.visibleCalendarIDs ?? selectedCalendarIDs)
            var eventPredicates = [
                "d.account_id = ?",
                "d.day_key >= ?",
                "d.day_key <= ?"
            ]
            var eventArguments: StatementArguments = [
                storageAccountID,
                range.start.timeIntervalSinceReferenceDate,
                range.end.timeIntervalSinceReferenceDate
            ]
            Self.appendActiveEventPredicate(columnPrefix: "d.", includeCancelled: includeCancelled, predicates: &eventPredicates, arguments: &eventArguments)
            Self.appendSelectedCalendarPredicate(
                column: "d.calendar_id",
                selectedCalendarIDs: effectiveCalendarIDs,
                predicates: &eventPredicates,
                arguments: &eventArguments
            )
            if let visibleColorIDs = eventViewFilter.visibleColorIDs {
                if visibleColorIDs.isEmpty {
                    eventPredicates.append("0 = 1")
                } else {
                    let sortedIDs = visibleColorIDs.sorted()
                    eventPredicates.append("d.color_id IN (\(Self.placeholders(count: sortedIDs.count)))")
                    for id in sortedIDs {
                        eventArguments += [id]
                    }
                }
            }
            if range.kind == .year,
               requiresEventDetails == false,
               eventViewFilter.visibleTagNames == nil {
                let countRows = try Row.fetchAll(
                    db,
                    sql: """
                        SELECT d.day_key, COUNT(d.event_id) AS event_count
                        FROM cache_calendar_event_days d
                        WHERE \(eventPredicates.joined(separator: " AND "))
                        GROUP BY d.day_key
                        ORDER BY d.day_key ASC
                        """,
                    arguments: eventArguments
                )
                var eventCountsByDay: [TimeInterval: Int] = [:]
                eventCountsByDay.reserveCapacity(countRows.count)
                for row in countRows {
                    eventCountsByDay[row["day_key"]] = row["event_count"]
                }
                return CalendarVisibleRangeProjection(
                    range: range,
                    revisionKey: try Self.calendarRevisionKey(db, accountID: storageAccountID, dayKeys: range.dayKeys),
                    eventsByDay: [:],
                    eventCountsByDay: eventCountsByDay,
                    tasksByDueDate: [:],
                    eventByID: [:],
                    taskByID: [:],
                    eventSearchTextByID: [:],
                    taskSearchTextByID: [:]
                )
            }
            let searchTextSelection = range.kind == .year ? "'' AS search_text" : "r.search_text"
            let rows = try Row.fetchAll(
                db,
                sql: """
                    SELECT
                        d.day_key,
                        d.event_id,
                        d.calendar_id,
                        d.status,
                        d.color_id,
                        r.summary,
                        r.start_date,
                        r.end_date,
                        r.is_all_day,
                        \(searchTextSelection)
                    FROM cache_calendar_event_days d
                    JOIN cache_event_render_index r
                        ON r.account_id = d.account_id AND r.id = d.event_id
                    WHERE \(eventPredicates.joined(separator: " AND "))
                    ORDER BY d.day_key ASC, d.event_id ASC
                    """,
                arguments: eventArguments
            )

            var eventByID: [CalendarEventMirror.ID: CalendarEventMirror] = [:]
            var eventSearchTextByID: [CalendarEventMirror.ID: String] = [:]
            var eventsByDay: [TimeInterval: [CalendarEventMirror.ID]] = [:]
            var literalTagsByEventID: [CalendarEventMirror.ID: Set<String>] = [:]
            if eventViewFilter.visibleTagNames != nil {
                literalTagsByEventID = try Self.fetchEventTags(
                    db,
                    accountID: storageAccountID,
                    eventIDs: Set(rows.map { row -> String in row["event_id"] })
                )
            }
            for row in rows {
                let dayKey: TimeInterval = row["day_key"]
                let eventID: String = row["event_id"]
                let calendarID: String = row["calendar_id"]
                let status: String = row["status"]
                let colorID: String = row["color_id"]
                if let visibleTagNames = eventViewFilter.visibleTagNames {
                    let eventTagNames = Self.eventTagNames(
                        literalTags: literalTagsByEventID[eventID] ?? [],
                        colorID: colorID,
                        colorTagIndex: eventViewFilter.colorTagIndex
                    )
                    guard eventTagNames.isEmpty || eventTagNames.isDisjoint(with: visibleTagNames) == false else {
                        continue
                    }
                }
                let event: CalendarEventMirror
                if let cached = eventByID[eventID] {
                    event = cached
                } else {
                    let isAllDay: Int = row["is_all_day"]
                    let rendered = CalendarEventMirror(
                        id: eventID,
                        calendarID: calendarID,
                        summary: row["summary"],
                        details: "",
                        startDate: Date(timeIntervalSince1970: row["start_date"] as Double),
                        endDate: Date(timeIntervalSince1970: row["end_date"] as Double),
                        isAllDay: isAllDay != 0,
                        status: CalendarEventStatus(rawValue: status) ?? .confirmed,
                        recurrence: [],
                        etag: nil,
                        updatedAt: nil,
                        location: "",
                        colorId: colorID
                    )
                    eventByID[eventID] = rendered
                    eventSearchTextByID[eventID] = row["search_text"]
                    event = rendered
                }
                eventsByDay[dayKey, default: []].append(event.id)
            }

            let taskRows = try Row.fetchAll(
                db,
                sql: """
                    SELECT id, task_list_id, title, status, due_date, completed_at, is_deleted, is_hidden, position, updated_at, search_text
                    FROM cache_task_render_index
                    WHERE account_id = ?
                        AND due_date IS NOT NULL
                        AND due_date >= ?
                        AND due_date < ?
                    ORDER BY due_date ASC, title COLLATE NOCASE ASC, id ASC
                    """,
                arguments: [
                    storageAccountID,
                    range.start.timeIntervalSince1970,
                    (calendar.date(byAdding: .day, value: 1, to: range.end) ?? range.end).timeIntervalSince1970
                ]
            )
            var tasksByDueDate: [TimeInterval: [TaskMirror.ID]] = [:]
            var taskByID: [TaskMirror.ID: TaskMirror] = [:]
            var taskSearchTextByID: [TaskMirror.ID: String] = [:]
            for row in taskRows {
                let isDeleted: Int = row["is_deleted"]
                let isHidden: Int = row["is_hidden"]
                guard isDeleted == 0, isHidden == 0 else { continue }
                let taskListID: String = row["task_list_id"]
                let dueInterval: Double = row["due_date"]
                let dueDate = Date(timeIntervalSince1970: dueInterval)
                let dayKey = calendar.startOfDay(for: dueDate).timeIntervalSinceReferenceDate
                guard range.dayKeys.contains(dayKey) else { continue }
                let task = TaskMirror(
                    id: row["id"],
                    taskListID: taskListID,
                    parentID: nil,
                    title: row["title"],
                    notes: "",
                    status: TaskStatus(rawValue: row["status"] as String) ?? .needsAction,
                    dueDate: dueDate,
                    completedAt: (row["completed_at"] as Double?).map(Date.init(timeIntervalSince1970:)),
                    isDeleted: isDeleted != 0,
                    isHidden: isHidden != 0,
                    position: row["position"] as String?,
                    etag: nil,
                    updatedAt: (row["updated_at"] as Double?).map(Date.init(timeIntervalSince1970:))
                )
                tasksByDueDate[dayKey, default: []].append(task.id)
                taskByID[task.id] = task
                taskSearchTextByID[task.id] = row["search_text"]
            }

            return CalendarVisibleRangeProjection(
                range: range,
                revisionKey: try Self.calendarRevisionKey(db, accountID: storageAccountID, dayKeys: range.dayKeys),
                eventsByDay: eventsByDay,
                tasksByDueDate: tasksByDueDate,
                eventByID: eventByID,
                taskByID: taskByID,
                eventSearchTextByID: eventSearchTextByID,
                taskSearchTextByID: taskSearchTextByID
            )
        }
    }

    func events(
        ids: [CalendarEventMirror.ID],
        accountID: String? = nil,
        encryptionKey: SymmetricKey? = nil
    ) throws -> [CalendarEventMirror] {
        guard ids.isEmpty == false else { return [] }
        return try dbQueue.read { db in
            try Self.fetchEvents(
                db,
                accountID: accountID ?? Self.unscopedAccountID,
                ids: ids,
                encryptionKey: encryptionKey
            )
        }
    }

    func events(
        in interval: DateInterval,
        accountID: String? = nil,
        calendarIDs: Set<CalendarListMirror.ID>? = nil,
        eventViewFilter: CalendarEventViewFilter = CalendarEventViewFilter(),
        includeCancelled: Bool = false,
        encryptionKey: SymmetricKey? = nil
    ) throws -> [CalendarEventMirror] {
        guard interval.end > interval.start else { return [] }
        let storageAccountID = accountID ?? Self.unscopedAccountID
        return try dbQueue.read { db in
            let effectiveCalendarIDs = calendarIDs.map { ids in
                ids.intersection(eventViewFilter.visibleCalendarIDs ?? ids)
            } ?? eventViewFilter.visibleCalendarIDs
            var predicates = [
                "account_id = ?",
                "start_date < ?",
                "end_date > ?"
            ]
            var arguments: StatementArguments = [
                storageAccountID,
                interval.end.timeIntervalSince1970,
                interval.start.timeIntervalSince1970
            ]
            Self.appendActiveEventPredicate(includeCancelled: includeCancelled, predicates: &predicates, arguments: &arguments)
            Self.appendSelectedCalendarPredicate(
                column: "calendar_id",
                selectedCalendarIDs: effectiveCalendarIDs,
                predicates: &predicates,
                arguments: &arguments
            )
            if let visibleColorIDs = eventViewFilter.visibleColorIDs {
                if visibleColorIDs.isEmpty {
                    predicates.append("0 = 1")
                } else {
                    let sortedIDs = visibleColorIDs.sorted()
                    predicates.append("color_id IN (\(Self.placeholders(count: sortedIDs.count)))")
                    for id in sortedIDs {
                        arguments += [id]
                    }
                }
            }
            let ids = try String.fetchAll(
                db,
                sql: """
                    SELECT id
                    FROM cache_event_render_index
                    WHERE \(predicates.joined(separator: " AND "))
                    ORDER BY start_date ASC, summary COLLATE NOCASE ASC, id ASC
                    """,
                arguments: arguments
            )
            return try Self.fetchEvents(
                db,
                accountID: storageAccountID,
                ids: ids,
                encryptionKey: encryptionKey
            )
            .filter { event in
                eventViewFilter.allows(event)
                    && (includeCancelled || event.status != .cancelled)
                    && (calendarIDs?.contains(event.calendarID) ?? true)
                    && event.startDate < interval.end
                    && event.endDate > interval.start
            }
        }
    }

    func hasMatchingEvent(
        accountID: String? = nil,
        calendarID: CalendarListMirror.ID,
        summary: String,
        startDate: Date,
        isAllDay: Bool,
        tolerance: TimeInterval
    ) throws -> Bool {
        let storageAccountID = accountID ?? Self.unscopedAccountID
        let lowerBound = startDate.addingTimeInterval(-tolerance).timeIntervalSince1970
        let upperBound = startDate.addingTimeInterval(tolerance).timeIntervalSince1970
        return try dbQueue.read { db in
            let match = try Int.fetchOne(
                db,
                sql: """
                    SELECT 1
                    FROM cache_event_render_index
                    WHERE account_id = ?
                        AND calendar_id = ?
                        AND status <> ?
                        AND is_all_day = ?
                        AND summary = ? COLLATE NOCASE
                        AND start_date > ?
                        AND start_date < ?
                    LIMIT 1
                    """,
                arguments: [
                    storageAccountID,
                    calendarID,
                    CalendarEventStatus.cancelled.rawValue,
                    isAllDay ? 1 : 0,
                    summary,
                    lowerBound,
                    upperBound
                ]
            )
            return match != nil
        }
    }

    func availabilityHoldGroups(
        accountID: String? = nil,
        encryptionKey: SymmetricKey? = nil
    ) throws -> [AvailabilityHoldGroup] {
        let storageAccountID = accountID ?? Self.unscopedAccountID
        return try dbQueue.read { db in
            let ids = try String.fetchAll(
                db,
                sql: """
                    SELECT id
                    FROM cache_event_render_index
                    WHERE account_id = ?
                        AND availability_hold_group_id IS NOT NULL
                        AND status <> ?
                    ORDER BY start_date ASC, id ASC
                    """,
                arguments: [storageAccountID, CalendarEventStatus.cancelled.rawValue]
            )
            if ids.isEmpty == false {
                let events = try Self.fetchEvents(
                    db,
                    accountID: storageAccountID,
                    ids: ids,
                    encryptionKey: encryptionKey
                )
                return Self.availabilityHoldGroups(from: events)
            }

            let fallbackRows = try Self.fetchStoredEventRows(db, encryptionKey: encryptionKey)
            let fallbackEvents = try fallbackRows
                .filter { $0.accountID == storageAccountID }
                .map { row in
                    try PayloadCoder.decode(CalendarEventMirror.self, from: row.payload, encryptionKey: encryptionKey)
                }
            return Self.availabilityHoldGroups(from: fallbackEvents)
        }
    }

    func blockingEvents(
        for slot: AvailabilitySlot,
        accountID: String? = nil,
        calendarIDs: Set<CalendarListMirror.ID>,
        encryptionKey: SymmetricKey? = nil
    ) throws -> [CalendarEventMirror] {
        let interval = DateInterval(start: slot.startDate, end: slot.endDate)
        let candidates = try events(
            in: interval,
            accountID: accountID,
            calendarIDs: calendarIDs,
            eventViewFilter: CalendarEventViewFilter(),
            includeCancelled: false,
            encryptionKey: encryptionKey
        )
        return AvailabilitySlotResolver.blockingEvents(
            for: slot,
            events: candidates,
            calendarIDs: calendarIDs
        )
    }

#if DEBUG
    func contentHashForTesting(table: String, accountID: String? = nil, id: String? = nil) throws -> String? {
        let allowedTables = [
            "cache_accounts",
            "cache_workspaces",
            "cache_task_lists",
            "cache_tasks",
            "cache_calendars",
            "cache_events",
            "cache_sync_checkpoints",
            "cache_pending_mutations",
            "cache_state"
        ]
        guard allowedTables.contains(table) else { return nil }
        return try dbQueue.read { db in
            switch table {
            case "cache_state":
                return try String.fetchOne(db, sql: "SELECT content_hash FROM cache_state WHERE id = 1")
            case "cache_accounts":
                guard let id else { return nil }
                return try String.fetchOne(db, sql: "SELECT content_hash FROM cache_accounts WHERE id = ?", arguments: [id])
            case "cache_workspaces":
                guard let accountID else { return nil }
                return try String.fetchOne(db, sql: "SELECT content_hash FROM cache_workspaces WHERE account_id = ?", arguments: [accountID])
            default:
                guard let accountID, let id else { return nil }
                let idColumn = table == "cache_sync_checkpoints" ? "checkpoint_id" : "id"
                return try String.fetchOne(
                    db,
                    sql: "SELECT content_hash FROM \(table) WHERE account_id = ? AND \(idColumn) = ?",
                    arguments: [accountID, id]
                )
            }
        }
    }

    func rowCountForTesting(table: String) throws -> Int {
        let allowedTables = [
            "cache_accounts",
            "cache_workspaces",
            "cache_task_lists",
            "cache_tasks",
            "cache_calendars",
            "cache_events",
            "cache_sync_checkpoints",
            "cache_pending_mutations",
            "cache_state",
            "cache_calendar_event_index",
            "cache_calendar_event_days",
            "cache_calendar_event_tags",
            "cache_calendar_calendar_counts",
            "cache_calendar_color_counts",
            "cache_calendar_tag_counts",
            "cache_calendar_day_revisions",
            "cache_calendar_range_revisions",
            "cache_task_render_index",
            "cache_event_render_index",
            "cache_task_search_fts",
            "cache_event_search_fts",
            "cache_side_effect_dirty_queue"
        ]
        guard allowedTables.contains(table) else { return 0 }
        return try dbQueue.read { db in
            try Int.fetchOne(db, sql: "SELECT COUNT(*) FROM \(table)") ?? 0
        }
    }

    func corruptPayloadForTesting(table: String, accountID: String, id: String) throws {
        let allowedTables = ["cache_tasks", "cache_events"]
        guard allowedTables.contains(table) else { return }
        try dbQueue.write { db in
            try db.execute(
                sql: "UPDATE \(table) SET payload = ? WHERE account_id = ? AND id = ?",
                arguments: [Data("not-json".utf8), accountID, id]
            )
        }
    }

    func calendarDayRevisionForTesting(accountID: String = "account-1", dayKey: TimeInterval) throws -> Int {
        try dbQueue.read { db in
            try Int.fetchOne(
                db,
                sql: "SELECT revision FROM cache_calendar_day_revisions WHERE account_id = ? AND day_key = ?",
                arguments: [accountID, dayKey]
            ) ?? 0
        }
    }

    func calendarRangeRevisionForTesting(accountID: String = "account-1", kind: String, key: String) throws -> Int {
        try dbQueue.read { db in
            try Int.fetchOne(
                db,
                sql: """
                    SELECT revision
                    FROM cache_calendar_range_revisions
                    WHERE account_id = ? AND range_kind = ? AND range_key = ?
                    """,
                arguments: [accountID, kind, key]
            ) ?? 0
        }
    }
#endif
}

struct LocalCacheDatabaseSaveProfile: Sendable {
    var accountRows: Int
    var workspaceRows: Int
    var taskListRows: Int
    var taskRows: Int
    var calendarRows: Int
    var eventRows: Int
    var checkpointRows: Int
    var pendingMutationRows: Int
    var encodeAndHashMilliseconds: Double
    var transactionMilliseconds: Double
    var totalMilliseconds: Double
    var incrementalUpsertedRows: Int = 0
    var incrementalDeletedRows: Int = 0
    var incrementalUnchangedSkippedRows: Int = 0
}

struct LocalCacheDatabaseLoadProfile: Sendable {
    var accountRows: Int
    var workspaceRows: Int
    var taskListRows: Int
    var taskRows: Int
    var calendarRows: Int
    var eventRows: Int
    var checkpointRows: Int
    var pendingMutationRows: Int
    var fetchAndDecodeMilliseconds: Double
    var rebuildStateMilliseconds: Double
    var totalMilliseconds: Double
}

enum LocalCacheEntitySearchScope: Equatable, Sendable {
    case all
    case tasks
    case notes
    case events

    var includesTasks: Bool {
        switch self {
        case .all, .tasks, .notes: true
        case .events: false
        }
    }

    var includesEvents: Bool {
        switch self {
        case .all, .events: true
        case .tasks, .notes: false
        }
    }
}

struct LocalCacheEntitySearchResults: Sendable {
    var tasks: [TaskMirror]
    var events: [CalendarEventMirror]

    static let empty = LocalCacheEntitySearchResults(tasks: [], events: [])
}

enum LocalIntegrationDirtyTarget: String, CaseIterable, Sendable {
    case spotlight
    case notification
}

enum LocalIntegrationDirtyOperation: String, Sendable {
    case upsert
    case delete
    case rebuild
}

struct LocalIntegrationDirtyItem: Equatable, Sendable {
    var accountID: String
    var target: LocalIntegrationDirtyTarget
    var resourceType: SyncResourceType?
    var resourceID: String?
    var operation: LocalIntegrationDirtyOperation
    var enqueuedAt: Date

    var isFullRebuild: Bool {
        operation == .rebuild
    }
}

struct CalendarAggregateCounts: Equatable, Sendable {
    var eventCountsByCalendarID: [CalendarListMirror.ID: Int]
    var eventCountsByColorID: [String: Int]
    var eventCountsByTagName: [String: Int]

    static let empty = CalendarAggregateCounts(
        eventCountsByCalendarID: [:],
        eventCountsByColorID: [:],
        eventCountsByTagName: [:]
    )
}

enum LocalCacheRowHasher {
    static func hash<T: Encodable>(_ value: T, kind: String) throws -> String {
        try hash(canonicalPayload: JSONEncoder.cacheDatabaseCanonical.encode(value), kind: kind)
    }

    static func hash(canonicalPayload: Data, kind _: String) -> String {
        var hasher = SHA256()
        hasher.update(data: canonicalPayload)
        return hexString(for: hasher.finalize())
    }

    private static func hexString<D: Sequence>(for digest: D) -> String where D.Element == UInt8 {
        let table = Array("0123456789abcdef".utf8)
        var bytes: [UInt8] = []
        bytes.reserveCapacity(64)
        for byte in digest {
            bytes.append(table[Int(byte >> 4)])
            bytes.append(table[Int(byte & 0x0F)])
        }
        return String(decoding: bytes, as: UTF8.self)
    }
}

private extension LocalCacheDatabaseStore {
    static let unscopedAccountID = "__hcb_unscoped__"

    static func availabilityHoldGroups(from events: [CalendarEventMirror]) -> [AvailabilityHoldGroup] {
        let holds = events.filter { $0.status != .cancelled && $0.availabilityHold != nil }
        let grouped = Dictionary(grouping: holds) { $0.availabilityHold?.groupID ?? "" }
        return grouped.compactMap { groupID, events in
            guard groupID.isEmpty == false,
                  let metadata = events.compactMap(\.availabilityHold).first
            else { return nil }
            return AvailabilityHoldGroup(
                id: groupID,
                metadata: metadata,
                events: events.sorted { $0.startDate < $1.startDate }
            )
        }
        .sorted { lhs, rhs in
            let left = lhs.events.first?.startDate ?? lhs.metadata.createdAt
            let right = rhs.events.first?.startDate ?? rhs.metadata.createdAt
            return left < right
        }
    }

    struct IncrementalWriteCounts {
        var upserted: Int = 0
        var deleted: Int = 0
        var unchangedSkipped: Int = 0

        mutating func add(_ other: IncrementalWriteCounts) {
            upserted += other.upserted
            deleted += other.deleted
            unchangedSkipped += other.unchangedSkipped
        }
    }

    static var migrator: DatabaseMigrator {
        var migrator = DatabaseMigrator()
        migrator.registerMigration("cache-db-v1") { db in
            try db.execute(sql: """
                CREATE TABLE cache_state (
                    id INTEGER PRIMARY KEY CHECK (id = 1),
                    schema_version INTEGER NOT NULL,
                    active_account_id TEXT,
                    primary_account_id TEXT,
                    payload BLOB NOT NULL,
                    content_hash TEXT NOT NULL,
                    updated_at TEXT NOT NULL
                );

                CREATE TABLE cache_accounts (
                    id TEXT PRIMARY KEY,
                    position INTEGER NOT NULL,
                    payload BLOB NOT NULL,
                    content_hash TEXT NOT NULL
                );

                CREATE TABLE cache_workspaces (
                    account_id TEXT PRIMARY KEY,
                    position INTEGER NOT NULL,
                    payload BLOB NOT NULL,
                    content_hash TEXT NOT NULL
                );

                CREATE TABLE cache_task_lists (
                    account_id TEXT NOT NULL,
                    id TEXT NOT NULL,
                    position INTEGER NOT NULL,
                    payload BLOB NOT NULL,
                    content_hash TEXT NOT NULL,
                    PRIMARY KEY (account_id, id)
                );

                CREATE TABLE cache_tasks (
                    account_id TEXT NOT NULL,
                    id TEXT NOT NULL,
                    position INTEGER NOT NULL,
                    task_list_id TEXT NOT NULL,
                    updated_at REAL,
                    payload BLOB NOT NULL,
                    content_hash TEXT NOT NULL,
                    PRIMARY KEY (account_id, id)
                );

                CREATE TABLE cache_calendars (
                    account_id TEXT NOT NULL,
                    id TEXT NOT NULL,
                    position INTEGER NOT NULL,
                    payload BLOB NOT NULL,
                    content_hash TEXT NOT NULL,
                    PRIMARY KEY (account_id, id)
                );

                CREATE TABLE cache_events (
                    account_id TEXT NOT NULL,
                    id TEXT NOT NULL,
                    position INTEGER NOT NULL,
                    calendar_id TEXT NOT NULL,
                    start_date REAL NOT NULL,
                    end_date REAL NOT NULL,
                    updated_at REAL,
                    etag TEXT,
                    status TEXT NOT NULL,
                    payload BLOB NOT NULL,
                    content_hash TEXT NOT NULL,
                    PRIMARY KEY (account_id, id)
                );

                CREATE TABLE cache_sync_checkpoints (
                    account_id TEXT NOT NULL,
                    checkpoint_id TEXT NOT NULL,
                    position INTEGER NOT NULL,
                    resource_type TEXT NOT NULL,
                    resource_id TEXT NOT NULL,
                    last_successful_sync_at REAL,
                    payload BLOB NOT NULL,
                    content_hash TEXT NOT NULL,
                    PRIMARY KEY (account_id, checkpoint_id)
                );

                CREATE TABLE cache_pending_mutations (
                    account_id TEXT NOT NULL,
                    id TEXT NOT NULL,
                    position INTEGER NOT NULL,
                    resource_type TEXT NOT NULL,
                    resource_id TEXT NOT NULL,
                    created_at REAL NOT NULL,
                    payload BLOB NOT NULL,
                    content_hash TEXT NOT NULL,
                    PRIMARY KEY (account_id, id)
                );

                CREATE INDEX idx_cache_events_calendar_start ON cache_events(account_id, calendar_id, start_date);
                CREATE INDEX idx_cache_events_content_hash ON cache_events(content_hash);
                CREATE INDEX idx_cache_tasks_list_updated ON cache_tasks(account_id, task_list_id, updated_at);
                CREATE INDEX idx_cache_checkpoints_resource ON cache_sync_checkpoints(account_id, resource_type, resource_id);
                CREATE INDEX idx_cache_pending_resource ON cache_pending_mutations(account_id, resource_type, resource_id);
                """)
        }
        migrator.registerMigration("cache-db-v2-calendar-derived") { db in
            try db.execute(sql: """
                CREATE TABLE IF NOT EXISTS cache_calendar_event_index (
                    account_id TEXT NOT NULL,
                    event_id TEXT NOT NULL,
                    calendar_id TEXT NOT NULL,
                    start_date REAL NOT NULL,
                    end_date REAL NOT NULL,
                    is_all_day INTEGER NOT NULL,
                    status TEXT NOT NULL,
                    color_id TEXT NOT NULL,
                    content_hash TEXT NOT NULL,
                    PRIMARY KEY (account_id, event_id)
                );

                CREATE TABLE IF NOT EXISTS cache_calendar_event_days (
                    account_id TEXT NOT NULL,
                    day_key REAL NOT NULL,
                    event_id TEXT NOT NULL,
                    calendar_id TEXT NOT NULL,
                    status TEXT NOT NULL,
                    color_id TEXT NOT NULL,
                    PRIMARY KEY (account_id, day_key, event_id)
                );

                CREATE TABLE IF NOT EXISTS cache_calendar_event_tags (
                    account_id TEXT NOT NULL,
                    event_id TEXT NOT NULL,
                    tag_name TEXT NOT NULL,
                    PRIMARY KEY (account_id, event_id, tag_name)
                );

                CREATE TABLE IF NOT EXISTS cache_calendar_calendar_counts (
                    account_id TEXT NOT NULL,
                    calendar_id TEXT NOT NULL,
                    active_count INTEGER NOT NULL,
                    all_count INTEGER NOT NULL,
                    PRIMARY KEY (account_id, calendar_id)
                );

                CREATE TABLE IF NOT EXISTS cache_calendar_color_counts (
                    account_id TEXT NOT NULL,
                    color_id TEXT NOT NULL,
                    active_count INTEGER NOT NULL,
                    all_count INTEGER NOT NULL,
                    PRIMARY KEY (account_id, color_id)
                );

                CREATE TABLE IF NOT EXISTS cache_calendar_tag_counts (
                    account_id TEXT NOT NULL,
                    tag_name TEXT NOT NULL,
                    active_count INTEGER NOT NULL,
                    all_count INTEGER NOT NULL,
                    PRIMARY KEY (account_id, tag_name)
                );

                CREATE TABLE IF NOT EXISTS cache_calendar_day_revisions (
                    account_id TEXT NOT NULL,
                    day_key REAL NOT NULL,
                    revision INTEGER NOT NULL,
                    updated_at REAL NOT NULL,
                    PRIMARY KEY (account_id, day_key)
                );

                CREATE TABLE IF NOT EXISTS cache_calendar_range_revisions (
                    account_id TEXT NOT NULL,
                    range_kind TEXT NOT NULL,
                    range_key TEXT NOT NULL,
                    revision INTEGER NOT NULL,
                    updated_at REAL NOT NULL,
                    PRIMARY KEY (account_id, range_kind, range_key)
                );

                CREATE INDEX IF NOT EXISTS idx_cache_calendar_event_days_event
                    ON cache_calendar_event_days(account_id, event_id);
                CREATE INDEX IF NOT EXISTS idx_cache_calendar_event_days_range
                    ON cache_calendar_event_days(account_id, day_key, calendar_id, status);
                CREATE INDEX IF NOT EXISTS idx_cache_calendar_event_index_calendar
                    ON cache_calendar_event_index(account_id, calendar_id, status, color_id);
                CREATE INDEX IF NOT EXISTS idx_cache_calendar_event_tags_tag
                    ON cache_calendar_event_tags(account_id, tag_name, event_id);
                """)
        }
        migrator.registerMigration("cache-db-v3-search-render-dirty") { db in
            try db.execute(sql: """
                CREATE TABLE IF NOT EXISTS cache_task_render_index (
                    account_id TEXT NOT NULL,
                    id TEXT NOT NULL,
                    task_list_id TEXT NOT NULL,
                    title TEXT NOT NULL,
                    status TEXT NOT NULL,
                    due_date REAL,
                    completed_at REAL,
                    is_deleted INTEGER NOT NULL,
                    is_hidden INTEGER NOT NULL,
                    position TEXT,
                    updated_at REAL,
                    search_text TEXT NOT NULL,
                    content_hash TEXT NOT NULL,
                    PRIMARY KEY (account_id, id)
                );

                CREATE TABLE IF NOT EXISTS cache_event_render_index (
                    account_id TEXT NOT NULL,
                    id TEXT NOT NULL,
                    calendar_id TEXT NOT NULL,
                    summary TEXT NOT NULL,
                    start_date REAL NOT NULL,
                    end_date REAL NOT NULL,
                    is_all_day INTEGER NOT NULL,
                    status TEXT NOT NULL,
                    color_id TEXT NOT NULL,
                    search_text TEXT NOT NULL,
                    content_hash TEXT NOT NULL,
                    PRIMARY KEY (account_id, id)
                );

                CREATE VIRTUAL TABLE IF NOT EXISTS cache_task_search_fts USING fts5(
                    account_id UNINDEXED,
                    id UNINDEXED,
                    title,
                    notes,
                    tag_text,
                    task_list_id UNINDEXED,
                    status UNINDEXED,
                    is_deleted UNINDEXED,
                    is_hidden UNINDEXED,
                    due_date UNINDEXED,
                    completed_at UNINDEXED,
                    updated_at UNINDEXED,
                    content_hash UNINDEXED,
                    tokenize = 'unicode61 remove_diacritics 2'
                );

                CREATE VIRTUAL TABLE IF NOT EXISTS cache_event_search_fts USING fts5(
                    account_id UNINDEXED,
                    id UNINDEXED,
                    summary,
                    details,
                    location,
                    attendee_text,
                    meet_link,
                    calendar_id UNINDEXED,
                    status UNINDEXED,
                    start_date UNINDEXED,
                    end_date UNINDEXED,
                    is_all_day UNINDEXED,
                    updated_at UNINDEXED,
                    content_hash UNINDEXED,
                    tokenize = 'unicode61 remove_diacritics 2'
                );

                CREATE TABLE IF NOT EXISTS cache_side_effect_dirty_queue (
                    account_id TEXT NOT NULL,
                    target TEXT NOT NULL,
                    resource_type TEXT NOT NULL,
                    resource_id TEXT NOT NULL,
                    operation TEXT NOT NULL,
                    enqueued_at REAL NOT NULL,
                    PRIMARY KEY (account_id, target, resource_type, resource_id)
                );

                CREATE INDEX IF NOT EXISTS idx_cache_task_render_due
                    ON cache_task_render_index(account_id, due_date, task_list_id, is_deleted, is_hidden);
                CREATE INDEX IF NOT EXISTS idx_cache_event_render_start
                    ON cache_event_render_index(account_id, start_date, calendar_id, status);
                CREATE INDEX IF NOT EXISTS idx_cache_dirty_target
                    ON cache_side_effect_dirty_queue(target, enqueued_at);
                """)
        }
        migrator.registerMigration("cache-db-v4-calendar-query-indexes") { db in
            try db.execute(sql: """
                CREATE INDEX IF NOT EXISTS idx_cache_calendar_event_index_color
                    ON cache_calendar_event_index(account_id, color_id, status, calendar_id, event_id);
                CREATE INDEX IF NOT EXISTS idx_cache_calendar_event_days_range_event
                    ON cache_calendar_event_days(account_id, day_key, event_id, calendar_id, status, color_id);
                """)
        }
        migrator.registerMigration("cache-db-v5-calendar-query-event-metadata") { db in
            try db.execute(sql: """
                ALTER TABLE cache_event_render_index
                    ADD COLUMN transparency TEXT NOT NULL DEFAULT 'opaque';
                ALTER TABLE cache_event_render_index
                    ADD COLUMN availability_hold_group_id TEXT;
                CREATE INDEX IF NOT EXISTS idx_cache_event_render_overlap
                    ON cache_event_render_index(account_id, calendar_id, status, start_date, end_date, transparency);
                CREATE INDEX IF NOT EXISTS idx_cache_event_render_hold_group
                    ON cache_event_render_index(account_id, availability_hold_group_id, status, start_date);
                """)
        }
        migrator.registerMigration("cache-db-v6-drop-unused-base-cache-indexes") { db in
            for index in Self.obsoleteBulkWriteIndexes {
                try db.execute(sql: "DROP INDEX IF EXISTS \(index)")
            }
        }
        return migrator
    }

    static func replaceDatabaseContents(
        _ db: Database,
        encoded: EncodedState,
        failAfterWritingEntitiesForTesting: Bool
    ) throws {
        try dropBulkWriteIndexes(db)
        for table in [
            "cache_side_effect_dirty_queue",
            "cache_event_render_index",
            "cache_task_render_index",
            "cache_event_search_fts",
            "cache_task_search_fts",
            "cache_calendar_range_revisions",
            "cache_calendar_day_revisions",
            "cache_calendar_tag_counts",
            "cache_calendar_color_counts",
            "cache_calendar_calendar_counts",
            "cache_calendar_event_tags",
            "cache_calendar_event_days",
            "cache_calendar_event_index",
            "cache_pending_mutations",
            "cache_sync_checkpoints",
            "cache_events",
            "cache_calendars",
            "cache_tasks",
            "cache_task_lists",
            "cache_workspaces",
            "cache_accounts",
            "cache_state"
        ] {
            try db.execute(sql: "DELETE FROM \(table)")
        }

        try insertState(db, encoded.state)
        try insertAccounts(db, encoded.accounts)
        try insertWorkspaces(db, encoded.workspaces)
        try insertTaskLists(db, encoded.taskLists)
        try insertTasks(db, encoded.tasks)
        try insertCalendars(db, encoded.calendars)
        try insertEvents(db, encoded.events)
        try insertSyncCheckpoints(db, encoded.syncCheckpoints)
        try insertPendingMutations(db, encoded.pendingMutations)
        try rebuildCalendarDerivedTables(db, rows: encoded.events)
        try rebuildSearchAndRenderTables(db, taskRows: encoded.tasks, eventRows: encoded.events)
        try enqueueSideEffectRebuild(
            db,
            accountID: encoded.activeStorageAccountID,
            targets: Set(LocalIntegrationDirtyTarget.allCases)
        )

        if failAfterWritingEntitiesForTesting {
            throw CacheDatabaseError.injectedRollback
        }

        try createBulkWriteIndexes(db)
    }

    static let bulkWriteIndexes: [(name: String, sql: String)] = [
        (
            "idx_cache_calendar_event_days_event",
            "CREATE INDEX IF NOT EXISTS idx_cache_calendar_event_days_event ON cache_calendar_event_days(account_id, event_id)"
        ),
        (
            "idx_cache_calendar_event_days_range",
            "CREATE INDEX IF NOT EXISTS idx_cache_calendar_event_days_range ON cache_calendar_event_days(account_id, day_key, calendar_id, status)"
        ),
        (
            "idx_cache_calendar_event_index_calendar",
            "CREATE INDEX IF NOT EXISTS idx_cache_calendar_event_index_calendar ON cache_calendar_event_index(account_id, calendar_id, status, color_id)"
        ),
        (
            "idx_cache_calendar_event_tags_tag",
            "CREATE INDEX IF NOT EXISTS idx_cache_calendar_event_tags_tag ON cache_calendar_event_tags(account_id, tag_name, event_id)"
        ),
        (
            "idx_cache_task_render_due",
            "CREATE INDEX IF NOT EXISTS idx_cache_task_render_due ON cache_task_render_index(account_id, due_date, task_list_id, is_deleted, is_hidden)"
        ),
        (
            "idx_cache_event_render_start",
            "CREATE INDEX IF NOT EXISTS idx_cache_event_render_start ON cache_event_render_index(account_id, start_date, calendar_id, status)"
        ),
        (
            "idx_cache_dirty_target",
            "CREATE INDEX IF NOT EXISTS idx_cache_dirty_target ON cache_side_effect_dirty_queue(target, enqueued_at)"
        ),
        (
            "idx_cache_calendar_event_index_color",
            "CREATE INDEX IF NOT EXISTS idx_cache_calendar_event_index_color ON cache_calendar_event_index(account_id, color_id, status, calendar_id, event_id)"
        ),
        (
            "idx_cache_calendar_event_days_range_event",
            "CREATE INDEX IF NOT EXISTS idx_cache_calendar_event_days_range_event ON cache_calendar_event_days(account_id, day_key, event_id, calendar_id, status, color_id)"
        ),
        (
            "idx_cache_event_render_overlap",
            "CREATE INDEX IF NOT EXISTS idx_cache_event_render_overlap ON cache_event_render_index(account_id, calendar_id, status, start_date, end_date, transparency)"
        ),
        (
            "idx_cache_event_render_hold_group",
            "CREATE INDEX IF NOT EXISTS idx_cache_event_render_hold_group ON cache_event_render_index(account_id, availability_hold_group_id, status, start_date)"
        )
    ]

    static let obsoleteBulkWriteIndexes = [
        "idx_cache_events_calendar_start",
        "idx_cache_events_content_hash",
        "idx_cache_tasks_list_updated",
        "idx_cache_checkpoints_resource",
        "idx_cache_pending_resource"
    ]

    static func dropBulkWriteIndexes(_ db: Database) throws {
        for index in obsoleteBulkWriteIndexes {
            try db.execute(sql: "DROP INDEX IF EXISTS \(index)")
        }
        for index in bulkWriteIndexes {
            try db.execute(sql: "DROP INDEX IF EXISTS \(index.name)")
        }
    }

    static func createBulkWriteIndexes(_ db: Database) throws {
        for index in bulkWriteIndexes {
            try db.execute(sql: index.sql)
        }
    }

    static func applyIncrementalSyncContents(
        _ db: Database,
        encoded: EncodedState,
        changeSet: SyncChangeSet,
        failAfterWritingEntitiesForTesting: Bool
    ) throws -> IncrementalWriteCounts {
        var counts = IncrementalWriteCounts()
        counts.add(try upsertStateIfChanged(db, encoded.state))
        counts.add(try upsertAccountsIfChanged(db, encoded.accounts))
        counts.add(try upsertWorkspacesIfChanged(db, encoded.workspaces))

        let accountID = encoded.activeStorageAccountID

        let changedTaskIDs = changeSet.tasks.deleted
            .union(changeSet.tasks.inserted)
            .union(changeSet.tasks.updated)
        let oldAffectedTaskDayKeys = try taskDueDayKeysForTasks(
            db,
            accountID: accountID,
            ids: changeSet.tasks.deleted.union(changeSet.tasks.updated)
        )
        counts.deleted += try deleteRows(
            db,
            table: "cache_task_lists",
            idColumn: "id",
            accountID: accountID,
            ids: changeSet.taskLists.deleted
        )
        counts.add(try upsertEntityRows(
            db,
            table: "cache_task_lists",
            accountID: accountID,
            rows: encoded.taskLists,
            ids: changeSet.taskLists.inserted.union(changeSet.taskLists.updated)
        ))

        counts.deleted += try deleteRows(
            db,
            table: "cache_tasks",
            idColumn: "id",
            accountID: accountID,
            ids: changeSet.tasks.deleted
        )
        try deleteTaskSearchAndRenderRows(db, accountID: accountID, ids: changeSet.tasks.deleted)
        counts.add(try upsertTaskRows(
            db,
            accountID: accountID,
            rows: encoded.tasks,
            ids: changeSet.tasks.inserted.union(changeSet.tasks.updated)
        ))
        let changedTaskRows = encoded.tasks.filter {
            $0.accountID == accountID && changedTaskIDs.contains($0.id)
        }
        try upsertTaskSearchAndRenderRows(db, rows: changedTaskRows)
        try bumpCalendarRevisions(
            db,
            accountID: accountID,
            dayKeys: oldAffectedTaskDayKeys.union(taskDueDayKeys(for: changedTaskRows))
        )

        counts.deleted += try deleteRows(
            db,
            table: "cache_calendars",
            idColumn: "id",
            accountID: accountID,
            ids: changeSet.calendars.deleted
        )
        counts.add(try upsertEntityRows(
            db,
            table: "cache_calendars",
            accountID: accountID,
            rows: encoded.calendars,
            ids: changeSet.calendars.inserted.union(changeSet.calendars.updated)
        ))

        let changedEventIDs = changeSet.events.deleted
            .union(changeSet.events.inserted)
            .union(changeSet.events.updated)
        let oldAffectedDayKeys = try calendarDayKeysForEvents(
            db,
            accountID: accountID,
            ids: changeSet.events.deleted.union(changeSet.events.updated)
        )
        try removeCalendarDerivedRows(
            db,
            accountID: accountID,
            ids: changeSet.events.deleted.union(changeSet.events.updated)
        )

        counts.deleted += try deleteRows(
            db,
            table: "cache_events",
            idColumn: "id",
            accountID: accountID,
            ids: changeSet.events.deleted
        )
        try deleteEventSearchAndRenderRows(db, accountID: accountID, ids: changeSet.events.deleted)
        counts.add(try upsertEventRows(
            db,
            accountID: accountID,
            rows: encoded.events,
            ids: changeSet.events.inserted.union(changeSet.events.updated)
        ))
        let newEventRows = encoded.events.filter { $0.accountID == accountID && changedEventIDs.contains($0.id) }
        try insertCalendarDerivedRows(db, rows: newEventRows)
        try upsertEventSearchAndRenderRows(db, rows: newEventRows)
        let newAffectedDayKeys = Set(newEventRows.flatMap(\.dayKeys))
        try bumpCalendarRevisions(
            db,
            accountID: accountID,
            dayKeys: oldAffectedDayKeys.union(newAffectedDayKeys)
        )
        try enqueueSideEffectDirtyItems(db, accountID: accountID, changeSet: changeSet)

        counts.deleted += try deleteRows(
            db,
            table: "cache_sync_checkpoints",
            idColumn: "checkpoint_id",
            accountID: accountID,
            ids: changeSet.checkpoints.deleted
        )
        counts.add(try upsertCheckpointRows(
            db,
            accountID: accountID,
            rows: encoded.syncCheckpoints,
            ids: changeSet.checkpoints.inserted.union(changeSet.checkpoints.updated)
        ))

        counts.deleted += try deleteRows(
            db,
            table: "cache_pending_mutations",
            idColumn: "id",
            accountID: accountID,
            ids: changeSet.pendingMutations.deleted
        )
        counts.add(try upsertPendingMutationRows(
            db,
            accountID: accountID,
            rows: encoded.pendingMutations,
            ids: changeSet.pendingMutations.inserted.union(changeSet.pendingMutations.updated)
        ))

        counts.unchangedSkipped += changeSet.taskLists.unchanged.count
            + changeSet.tasks.unchanged.count
            + changeSet.calendars.unchanged.count
            + changeSet.events.unchanged.count
            + changeSet.checkpoints.unchanged.count
            + changeSet.pendingMutations.unchanged.count

        if failAfterWritingEntitiesForTesting {
            throw CacheDatabaseError.injectedRollback
        }
        return counts
    }

    static func upsertStateIfChanged(_ db: Database, _ row: EncodedState.StateRow) throws -> IncrementalWriteCounts {
        if try existingHash(db, table: "cache_state", idColumn: "id", id: "1") == row.contentHash {
            return IncrementalWriteCounts(unchangedSkipped: 1)
        }
        try db.execute(
            sql: """
                INSERT INTO cache_state
                    (id, schema_version, active_account_id, primary_account_id, payload, content_hash, updated_at)
                VALUES
                    (1, ?, ?, ?, ?, ?, ?)
                ON CONFLICT(id) DO UPDATE SET
                    schema_version = excluded.schema_version,
                    active_account_id = excluded.active_account_id,
                    primary_account_id = excluded.primary_account_id,
                    payload = excluded.payload,
                    content_hash = excluded.content_hash,
                    updated_at = excluded.updated_at
                """,
            arguments: [
                row.schemaVersion,
                row.activeAccountID,
                row.primaryAccountID,
                row.payload,
                row.contentHash,
                row.updatedAt
            ]
        )
        return IncrementalWriteCounts(upserted: 1)
    }

    static func upsertAccountsIfChanged(_ db: Database, _ rows: [EncodedState.AccountRow]) throws -> IncrementalWriteCounts {
        var counts = IncrementalWriteCounts()
        let statement = try db.makeStatement(sql: """
            INSERT INTO cache_accounts (id, position, payload, content_hash)
            VALUES (?, ?, ?, ?)
            ON CONFLICT(id) DO UPDATE SET
                position = excluded.position,
                payload = excluded.payload,
                content_hash = excluded.content_hash
            """)
        for row in rows {
            if try existingHash(db, table: "cache_accounts", idColumn: "id", id: row.id) == row.contentHash {
                counts.unchangedSkipped += 1
                continue
            }
            try statement.execute(arguments: [row.id, row.position, row.payload, row.contentHash])
            counts.upserted += 1
        }
        return counts
    }

    static func upsertWorkspacesIfChanged(_ db: Database, _ rows: [EncodedState.WorkspaceRow]) throws -> IncrementalWriteCounts {
        var counts = IncrementalWriteCounts()
        let statement = try db.makeStatement(sql: """
            INSERT INTO cache_workspaces (account_id, position, payload, content_hash)
            VALUES (?, ?, ?, ?)
            ON CONFLICT(account_id) DO UPDATE SET
                position = excluded.position,
                payload = excluded.payload,
                content_hash = excluded.content_hash
            """)
        for row in rows {
            if try existingScopedHash(db, table: "cache_workspaces", accountID: row.accountID) == row.contentHash {
                counts.unchangedSkipped += 1
                continue
            }
            try statement.execute(arguments: [row.accountID, row.position, row.payload, row.contentHash])
            counts.upserted += 1
        }
        return counts
    }

    static func upsertEntityRows(
        _ db: Database,
        table: String,
        accountID: String,
        rows: [EncodedState.EntityRow],
        ids: Set<String>
    ) throws -> IncrementalWriteCounts {
        guard ids.isEmpty == false else { return IncrementalWriteCounts() }
        var counts = IncrementalWriteCounts()
        let statement = try db.makeStatement(sql: """
            INSERT INTO \(table) (account_id, id, position, payload, content_hash)
            VALUES (?, ?, ?, ?, ?)
            ON CONFLICT(account_id, id) DO UPDATE SET
                position = excluded.position,
                payload = excluded.payload,
                content_hash = excluded.content_hash
            """)
        for row in rows where row.accountID == accountID && ids.contains(row.id) {
            if try existingScopedHash(db, table: table, accountID: row.accountID, idColumn: "id", id: row.id) == row.contentHash {
                counts.unchangedSkipped += 1
                continue
            }
            try statement.execute(arguments: [row.accountID, row.id, row.position, row.payload, row.contentHash])
            counts.upserted += 1
        }
        return counts
    }

    static func upsertTaskRows(
        _ db: Database,
        accountID: String,
        rows: [EncodedState.TaskRow],
        ids: Set<String>
    ) throws -> IncrementalWriteCounts {
        guard ids.isEmpty == false else { return IncrementalWriteCounts() }
        var counts = IncrementalWriteCounts()
        let statement = try db.makeStatement(sql: """
            INSERT INTO cache_tasks (account_id, id, position, task_list_id, updated_at, payload, content_hash)
            VALUES (?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT(account_id, id) DO UPDATE SET
                position = excluded.position,
                task_list_id = excluded.task_list_id,
                updated_at = excluded.updated_at,
                payload = excluded.payload,
                content_hash = excluded.content_hash
            """)
        for row in rows where row.accountID == accountID && ids.contains(row.id) {
            if try existingScopedHash(db, table: "cache_tasks", accountID: row.accountID, idColumn: "id", id: row.id) == row.contentHash {
                counts.unchangedSkipped += 1
                continue
            }
            try statement.execute(arguments: [row.accountID, row.id, row.position, row.taskListID, row.updatedAt, row.payload, row.contentHash])
            counts.upserted += 1
        }
        return counts
    }

    static func upsertEventRows(
        _ db: Database,
        accountID: String,
        rows: [EncodedState.EventRow],
        ids: Set<String>
    ) throws -> IncrementalWriteCounts {
        guard ids.isEmpty == false else { return IncrementalWriteCounts() }
        var counts = IncrementalWriteCounts()
        let statement = try db.makeStatement(sql: """
            INSERT INTO cache_events
                (account_id, id, position, calendar_id, start_date, end_date, updated_at, etag, status, payload, content_hash)
            VALUES
                (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT(account_id, id) DO UPDATE SET
                position = excluded.position,
                calendar_id = excluded.calendar_id,
                start_date = excluded.start_date,
                end_date = excluded.end_date,
                updated_at = excluded.updated_at,
                etag = excluded.etag,
                status = excluded.status,
                payload = excluded.payload,
                content_hash = excluded.content_hash
            """)
        for row in rows where row.accountID == accountID && ids.contains(row.id) {
            if try existingScopedHash(db, table: "cache_events", accountID: row.accountID, idColumn: "id", id: row.id) == row.contentHash {
                counts.unchangedSkipped += 1
                continue
            }
            try statement.execute(arguments: [
                row.accountID,
                row.id,
                row.position,
                row.calendarID,
                row.startDate,
                row.endDate,
                row.updatedAt,
                row.etag,
                row.status,
                row.payload,
                row.contentHash
            ])
            counts.upserted += 1
        }
        return counts
    }

    static func upsertCheckpointRows(
        _ db: Database,
        accountID: String,
        rows: [EncodedState.CheckpointRow],
        ids: Set<String>
    ) throws -> IncrementalWriteCounts {
        guard ids.isEmpty == false else { return IncrementalWriteCounts() }
        var counts = IncrementalWriteCounts()
        let statement = try db.makeStatement(sql: """
            INSERT INTO cache_sync_checkpoints
                (account_id, checkpoint_id, position, resource_type, resource_id, last_successful_sync_at, payload, content_hash)
            VALUES
                (?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT(account_id, checkpoint_id) DO UPDATE SET
                position = excluded.position,
                resource_type = excluded.resource_type,
                resource_id = excluded.resource_id,
                last_successful_sync_at = excluded.last_successful_sync_at,
                payload = excluded.payload,
                content_hash = excluded.content_hash
            """)
        for row in rows where row.accountID == accountID && ids.contains(row.id) {
            if try existingScopedHash(db, table: "cache_sync_checkpoints", accountID: row.accountID, idColumn: "checkpoint_id", id: row.id) == row.contentHash {
                counts.unchangedSkipped += 1
                continue
            }
            try statement.execute(arguments: [
                row.accountID,
                row.id,
                row.position,
                row.resourceType,
                row.resourceID,
                row.lastSuccessfulSyncAt,
                row.payload,
                row.contentHash
            ])
            counts.upserted += 1
        }
        return counts
    }

    static func upsertPendingMutationRows(
        _ db: Database,
        accountID: String,
        rows: [EncodedState.PendingMutationRow],
        ids: Set<String>
    ) throws -> IncrementalWriteCounts {
        guard ids.isEmpty == false else { return IncrementalWriteCounts() }
        var counts = IncrementalWriteCounts()
        let statement = try db.makeStatement(sql: """
            INSERT INTO cache_pending_mutations
                (account_id, id, position, resource_type, resource_id, created_at, payload, content_hash)
            VALUES
                (?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT(account_id, id) DO UPDATE SET
                position = excluded.position,
                resource_type = excluded.resource_type,
                resource_id = excluded.resource_id,
                created_at = excluded.created_at,
                payload = excluded.payload,
                content_hash = excluded.content_hash
            """)
        for row in rows where row.accountID == accountID && ids.contains(row.id) {
            if try existingScopedHash(db, table: "cache_pending_mutations", accountID: row.accountID, idColumn: "id", id: row.id) == row.contentHash {
                counts.unchangedSkipped += 1
                continue
            }
            try statement.execute(arguments: [
                row.accountID,
                row.id,
                row.position,
                row.resourceType,
                row.resourceID,
                row.createdAt,
                row.payload,
                row.contentHash
            ])
            counts.upserted += 1
        }
        return counts
    }

    static func deleteRows(
        _ db: Database,
        table: String,
        idColumn: String,
        accountID: String,
        ids: Set<String>
    ) throws -> Int {
        guard ids.isEmpty == false else { return 0 }
        let statement = try db.makeStatement(sql: "DELETE FROM \(table) WHERE account_id = ? AND \(idColumn) = ?")
        var count = 0
        for id in ids {
            try statement.execute(arguments: [accountID, id])
            count += db.changesCount
        }
        return count
    }

    static func rebuildSearchAndRenderTables(
        _ db: Database,
        taskRows: [EncodedState.TaskRow],
        eventRows: [EncodedState.EventRow]
    ) throws {
        for table in [
            "cache_task_render_index",
            "cache_event_render_index",
            "cache_task_search_fts",
            "cache_event_search_fts"
        ] {
            try db.execute(sql: "DELETE FROM \(table)")
        }
        try upsertTaskSearchAndRenderRows(db, rows: taskRows, deleteExistingRows: false)
        try upsertEventSearchAndRenderRows(db, rows: eventRows, deleteExistingRows: false)
    }

    static func upsertTaskSearchAndRenderRows(
        _ db: Database,
        rows: [EncodedState.TaskRow],
        deleteExistingRows: Bool = true
    ) throws {
        guard rows.isEmpty == false else { return }
        let renderStatement = try db.makeStatement(sql: """
            INSERT INTO cache_task_render_index
                (account_id, id, task_list_id, title, status, due_date, completed_at, is_deleted, is_hidden, position, updated_at, search_text, content_hash)
            VALUES
                (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT(account_id, id) DO UPDATE SET
                task_list_id = excluded.task_list_id,
                title = excluded.title,
                status = excluded.status,
                due_date = excluded.due_date,
                completed_at = excluded.completed_at,
                is_deleted = excluded.is_deleted,
                is_hidden = excluded.is_hidden,
                position = excluded.position,
                updated_at = excluded.updated_at,
                search_text = excluded.search_text,
                content_hash = excluded.content_hash
            """)
        let searchStatement = try db.makeStatement(sql: """
            INSERT INTO cache_task_search_fts
                (account_id, id, title, notes, tag_text, task_list_id, status, is_deleted, is_hidden, due_date, completed_at, updated_at, content_hash)
            VALUES
                (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            """)
        for row in rows {
            if deleteExistingRows {
                try deleteTaskSearchAndRenderRows(db, accountID: row.accountID, ids: [row.id])
            }
            try renderStatement.execute(arguments: [
                row.accountID,
                row.id,
                row.taskListID,
                row.title,
                row.status,
                row.dueDate,
                row.completedAt,
                row.isDeleted ? 1 : 0,
                row.isHidden ? 1 : 0,
                row.positionValue,
                row.updatedAt,
                row.searchText,
                row.contentHash
            ])
            try searchStatement.execute(arguments: [
                row.accountID,
                row.id,
                row.title,
                row.notes,
                row.tagText,
                row.taskListID,
                row.status,
                row.isDeleted ? 1 : 0,
                row.isHidden ? 1 : 0,
                row.dueDate,
                row.completedAt,
                row.updatedAt,
                row.contentHash
            ])
        }
    }

    static func deleteTaskSearchAndRenderRows(
        _ db: Database,
        accountID: String,
        ids: Set<String>
    ) throws {
        guard ids.isEmpty == false else { return }
        let renderStatement = try db.makeStatement(sql: "DELETE FROM cache_task_render_index WHERE account_id = ? AND id = ?")
        let searchStatement = try db.makeStatement(sql: "DELETE FROM cache_task_search_fts WHERE account_id = ? AND id = ?")
        for id in ids {
            try renderStatement.execute(arguments: [accountID, id])
            try searchStatement.execute(arguments: [accountID, id])
        }
    }

    static func upsertEventSearchAndRenderRows(
        _ db: Database,
        rows: [EncodedState.EventRow],
        deleteExistingRows: Bool = true
    ) throws {
        guard rows.isEmpty == false else { return }
        let renderStatement = try db.makeStatement(sql: """
            INSERT INTO cache_event_render_index
                (account_id, id, calendar_id, summary, start_date, end_date, is_all_day, status, color_id, search_text, content_hash, transparency, availability_hold_group_id)
            VALUES
                (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT(account_id, id) DO UPDATE SET
                calendar_id = excluded.calendar_id,
                summary = excluded.summary,
                start_date = excluded.start_date,
                end_date = excluded.end_date,
                is_all_day = excluded.is_all_day,
                status = excluded.status,
                color_id = excluded.color_id,
                search_text = excluded.search_text,
                content_hash = excluded.content_hash,
                transparency = excluded.transparency,
                availability_hold_group_id = excluded.availability_hold_group_id
            """)
        let searchStatement = try db.makeStatement(sql: """
            INSERT INTO cache_event_search_fts
                (account_id, id, summary, details, location, attendee_text, meet_link, calendar_id, status, start_date, end_date, is_all_day, updated_at, content_hash)
            VALUES
                (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            """)
        for row in rows {
            if deleteExistingRows {
                try deleteEventSearchAndRenderRows(db, accountID: row.accountID, ids: [row.id])
            }
            try renderStatement.execute(arguments: [
                row.accountID,
                row.id,
                row.calendarID,
                row.summary,
                row.startDate,
                row.endDate,
                row.isAllDay ? 1 : 0,
                row.status,
                row.colorID,
                row.searchText,
                row.contentHash,
                row.transparency,
                row.availabilityHoldGroupID
            ])
            try searchStatement.execute(arguments: [
                row.accountID,
                row.id,
                row.summary,
                row.details,
                row.location,
                row.attendeeText,
                row.meetLink,
                row.calendarID,
                row.status,
                row.startDate,
                row.endDate,
                row.isAllDay ? 1 : 0,
                row.updatedAt,
                row.contentHash
            ])
        }
    }

    static func deleteEventSearchAndRenderRows(
        _ db: Database,
        accountID: String,
        ids: Set<String>
    ) throws {
        guard ids.isEmpty == false else { return }
        let renderStatement = try db.makeStatement(sql: "DELETE FROM cache_event_render_index WHERE account_id = ? AND id = ?")
        let searchStatement = try db.makeStatement(sql: "DELETE FROM cache_event_search_fts WHERE account_id = ? AND id = ?")
        for id in ids {
            try renderStatement.execute(arguments: [accountID, id])
            try searchStatement.execute(arguments: [accountID, id])
        }
    }

    static func rebuildCalendarDerivedTables(
        _ db: Database,
        rows: [EncodedState.EventRow]
    ) throws {
        for table in [
            "cache_calendar_tag_counts",
            "cache_calendar_color_counts",
            "cache_calendar_calendar_counts",
            "cache_calendar_event_tags",
            "cache_calendar_event_days",
            "cache_calendar_event_index"
        ] {
            try db.execute(sql: "DELETE FROM \(table)")
        }
        try insertCalendarDerivedRows(db, rows: rows, updateCounts: false)
        try rebuildCalendarCountTables(db)
    }

    static func insertCalendarDerivedRows(
        _ db: Database,
        rows: [EncodedState.EventRow],
        updateCounts: Bool = true
    ) throws {
        guard rows.isEmpty == false else { return }

        let indexStatement = try db.makeStatement(sql: """
            INSERT INTO cache_calendar_event_index
                (account_id, event_id, calendar_id, start_date, end_date, is_all_day, status, color_id, content_hash)
            VALUES
                (?, ?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT(account_id, event_id) DO UPDATE SET
                calendar_id = excluded.calendar_id,
                start_date = excluded.start_date,
                end_date = excluded.end_date,
                is_all_day = excluded.is_all_day,
                status = excluded.status,
                color_id = excluded.color_id,
                content_hash = excluded.content_hash
            """)
        let dayStatement = try db.makeStatement(sql: """
            INSERT OR REPLACE INTO cache_calendar_event_days
                (account_id, day_key, event_id, calendar_id, status, color_id)
            VALUES
                (?, ?, ?, ?, ?, ?)
            """)
        let tagStatement = try db.makeStatement(sql: """
            INSERT OR REPLACE INTO cache_calendar_event_tags
                (account_id, event_id, tag_name)
            VALUES
                (?, ?, ?)
            """)

        for row in rows {
            try indexStatement.execute(arguments: [
                row.accountID,
                row.id,
                row.calendarID,
                row.startDate,
                row.endDate,
                row.isAllDay ? 1 : 0,
                row.status,
                row.colorID,
                row.contentHash
            ])
            for dayKey in row.dayKeys {
                try dayStatement.execute(arguments: [
                    row.accountID,
                    dayKey,
                    row.id,
                    row.calendarID,
                    row.status,
                    row.colorID
                ])
            }
            for tagName in row.tagNames.sorted() {
                try tagStatement.execute(arguments: [row.accountID, row.id, tagName])
            }
            if updateCounts {
                try applyCalendarCountDelta(db, row: row, delta: 1)
            }
        }
    }

    static func rebuildCalendarCountTables(_ db: Database) throws {
        for table in [
            "cache_calendar_tag_counts",
            "cache_calendar_color_counts",
            "cache_calendar_calendar_counts"
        ] {
            try db.execute(sql: "DELETE FROM \(table)")
        }

        let cancelled = CalendarEventStatus.cancelled.rawValue
        try db.execute(
            sql: """
                INSERT INTO cache_calendar_calendar_counts
                    (account_id, calendar_id, active_count, all_count)
                SELECT
                    account_id,
                    calendar_id,
                    SUM(CASE WHEN status <> ? THEN 1 ELSE 0 END),
                    COUNT(*)
                FROM cache_calendar_event_index
                GROUP BY account_id, calendar_id
                """,
            arguments: [cancelled]
        )
        try db.execute(
            sql: """
                INSERT INTO cache_calendar_color_counts
                    (account_id, color_id, active_count, all_count)
                SELECT
                    account_id,
                    color_id,
                    SUM(CASE WHEN status <> ? THEN 1 ELSE 0 END),
                    COUNT(*)
                FROM cache_calendar_event_index
                GROUP BY account_id, color_id
                """,
            arguments: [cancelled]
        )
        try db.execute(
            sql: """
                INSERT INTO cache_calendar_tag_counts
                    (account_id, tag_name, active_count, all_count)
                SELECT
                    t.account_id,
                    t.tag_name,
                    SUM(CASE WHEN i.status <> ? THEN 1 ELSE 0 END),
                    COUNT(*)
                FROM cache_calendar_event_tags t
                JOIN cache_calendar_event_index i
                    ON i.account_id = t.account_id AND i.event_id = t.event_id
                GROUP BY t.account_id, t.tag_name
                """,
            arguments: [cancelled]
        )
    }

    static func removeCalendarDerivedRows(
        _ db: Database,
        accountID: String,
        ids: Set<String>
    ) throws {
        guard ids.isEmpty == false else { return }
        for id in ids {
            guard let index = try Row.fetchOne(
                db,
                sql: """
                    SELECT event_id, calendar_id, start_date, end_date, is_all_day, status, color_id, content_hash
                    FROM cache_calendar_event_index
                    WHERE account_id = ? AND event_id = ?
                    """,
                arguments: [accountID, id]
            ) else {
                continue
            }
            let tagRows = try Row.fetchAll(
                db,
                sql: "SELECT tag_name FROM cache_calendar_event_tags WHERE account_id = ? AND event_id = ?",
                arguments: [accountID, id]
            )
            let isAllDayValue: Int = index["is_all_day"]
            let row = EncodedState.EventRow(
                accountID: accountID,
                id: id,
                position: 0,
                calendarID: index["calendar_id"],
                summary: "",
                details: "",
                startDate: index["start_date"],
                endDate: index["end_date"],
                isAllDay: isAllDayValue != 0,
                updatedAt: nil,
                etag: nil,
                status: index["status"],
                colorID: index["color_id"],
                location: "",
                attendeeText: "",
                meetLink: "",
                transparency: CalendarEventTransparency.opaque.rawValue,
                availabilityHoldGroupID: nil,
                tagNames: Set(tagRows.map { row -> String in row["tag_name"] }),
                dayKeys: [],
                searchText: "",
                payload: Data(),
                contentHash: index["content_hash"]
            )
            try applyCalendarCountDelta(db, row: row, delta: -1)
            try db.execute(
                sql: "DELETE FROM cache_calendar_event_days WHERE account_id = ? AND event_id = ?",
                arguments: [accountID, id]
            )
            try db.execute(
                sql: "DELETE FROM cache_calendar_event_tags WHERE account_id = ? AND event_id = ?",
                arguments: [accountID, id]
            )
            try db.execute(
                sql: "DELETE FROM cache_calendar_event_index WHERE account_id = ? AND event_id = ?",
                arguments: [accountID, id]
            )
        }
    }

    static func applyCalendarCountDelta(
        _ db: Database,
        row: EncodedState.EventRow,
        delta: Int
    ) throws {
        let activeDelta = row.status == CalendarEventStatus.cancelled.rawValue ? 0 : delta
        try applyCountDelta(
            db,
            table: "cache_calendar_calendar_counts",
            keyColumn: "calendar_id",
            accountID: row.accountID,
            key: row.calendarID,
            activeDelta: activeDelta,
            allDelta: delta
        )
        try applyCountDelta(
            db,
            table: "cache_calendar_color_counts",
            keyColumn: "color_id",
            accountID: row.accountID,
            key: row.colorID,
            activeDelta: activeDelta,
            allDelta: delta
        )
        for tagName in row.tagNames {
            try applyCountDelta(
                db,
                table: "cache_calendar_tag_counts",
                keyColumn: "tag_name",
                accountID: row.accountID,
                key: tagName,
                activeDelta: activeDelta,
                allDelta: delta
            )
        }
    }

    static func applyCountDelta(
        _ db: Database,
        table: String,
        keyColumn: String,
        accountID: String,
        key: String,
        activeDelta: Int,
        allDelta: Int
    ) throws {
        try db.execute(
            sql: """
                INSERT INTO \(table) (account_id, \(keyColumn), active_count, all_count)
                VALUES (?, ?, ?, ?)
                ON CONFLICT(account_id, \(keyColumn)) DO UPDATE SET
                    active_count = active_count + excluded.active_count,
                    all_count = all_count + excluded.all_count
                """,
            arguments: [accountID, key, activeDelta, allDelta]
        )
        try db.execute(
            sql: "DELETE FROM \(table) WHERE account_id = ? AND \(keyColumn) = ? AND active_count <= 0 AND all_count <= 0",
            arguments: [accountID, key]
        )
    }

    static func calendarDayKeysForEvents(
        _ db: Database,
        accountID: String,
        ids: Set<String>
    ) throws -> Set<TimeInterval> {
        guard ids.isEmpty == false else { return [] }
        var keys: Set<TimeInterval> = []
        for id in ids {
            let rows = try Row.fetchAll(
                db,
                sql: "SELECT day_key FROM cache_calendar_event_days WHERE account_id = ? AND event_id = ?",
                arguments: [accountID, id]
            )
            for row in rows {
                keys.insert(row["day_key"] as TimeInterval)
            }
        }
        return keys
    }

    static func taskDueDayKeysForTasks(
        _ db: Database,
        accountID: String,
        ids: Set<String>,
        calendar: Calendar = .current
    ) throws -> Set<TimeInterval> {
        guard ids.isEmpty == false else { return [] }
        var keys: Set<TimeInterval> = []
        for id in ids {
            let rows = try Row.fetchAll(
                db,
                sql: "SELECT due_date FROM cache_task_render_index WHERE account_id = ? AND id = ? AND due_date IS NOT NULL",
                arguments: [accountID, id]
            )
            for row in rows {
                let dueInterval: Double = row["due_date"]
                keys.insert(calendar.startOfDay(for: Date(timeIntervalSince1970: dueInterval)).timeIntervalSinceReferenceDate)
            }
        }
        return keys
    }

    static func taskDueDayKeys(
        for rows: [EncodedState.TaskRow],
        calendar: Calendar = .current
    ) -> Set<TimeInterval> {
        Set(rows.compactMap { row in
            row.dueDate.map { dueInterval in
                calendar.startOfDay(for: Date(timeIntervalSince1970: dueInterval)).timeIntervalSinceReferenceDate
            }
        })
    }

    static func bumpCalendarRevisions(
        _ db: Database,
        accountID: String,
        dayKeys: Set<TimeInterval>,
        calendar: Calendar = .current
    ) throws {
        guard dayKeys.isEmpty == false else { return }
        let now = Date().timeIntervalSince1970
        let dayStatement = try db.makeStatement(sql: """
            INSERT INTO cache_calendar_day_revisions (account_id, day_key, revision, updated_at)
            VALUES (?, ?, 1, ?)
            ON CONFLICT(account_id, day_key) DO UPDATE SET
                revision = revision + 1,
                updated_at = excluded.updated_at
            """)
        for dayKey in dayKeys {
            try dayStatement.execute(arguments: [accountID, dayKey, now])
        }

        var rangeKeys: Set<String> = []
        for dayKey in dayKeys {
            let date = Date(timeIntervalSinceReferenceDate: dayKey)
            let day = calendar.startOfDay(for: date)
            let weekStart = CalendarGridLayout.startOfWeek(containing: day, calendar: calendar)
            let components = calendar.dateComponents([.year, .month], from: day)
            if let year = components.year {
                rangeKeys.insert("year:\(year)")
                if let month = components.month {
                    rangeKeys.insert("month:\(year)-\(month)")
                }
            }
            rangeKeys.insert("week:\(Int(weekStart.timeIntervalSinceReferenceDate))")
        }

        let rangeStatement = try db.makeStatement(sql: """
            INSERT INTO cache_calendar_range_revisions (account_id, range_kind, range_key, revision, updated_at)
            VALUES (?, ?, ?, 1, ?)
            ON CONFLICT(account_id, range_kind, range_key) DO UPDATE SET
                revision = revision + 1,
                updated_at = excluded.updated_at
            """)
        for rangeKey in rangeKeys {
            guard let separator = rangeKey.firstIndex(of: ":") else { continue }
            let kind = String(rangeKey[..<separator])
            let key = String(rangeKey[rangeKey.index(after: separator)...])
            try rangeStatement.execute(arguments: [accountID, kind, key, now])
        }
    }

    static func calendarRevisionKey(
        _ db: Database,
        accountID: String,
        dayKeys: [TimeInterval]
    ) throws -> String {
        guard dayKeys.isEmpty == false else { return "0" }
        var revisions: [TimeInterval: Int] = [:]
        revisions.reserveCapacity(dayKeys.count)
        var arguments: StatementArguments = [accountID]
        for dayKey in dayKeys {
            arguments += [dayKey]
        }
        let rows = try Row.fetchAll(
            db,
            sql: """
                SELECT day_key, revision
                FROM cache_calendar_day_revisions
                WHERE account_id = ?
                    AND day_key IN (\(placeholders(count: dayKeys.count)))
                """,
            arguments: arguments
        )
        for row in rows {
            revisions[row["day_key"]] = row["revision"]
        }
        return dayKeys
            .map { key in "\(Int(key)):\(revisions[key] ?? 0)" }
            .joined(separator: ",")
    }

    static func calendarVisibleRange(
        kind: CalendarVisibleRangeKind,
        anchorDate: Date,
        dayCount: Int?,
        calendar: Calendar
    ) -> CalendarVisibleRange {
        let anchorStart = calendar.startOfDay(for: anchorDate)
        switch kind {
        case .day:
            return CalendarVisibleRange(kind: kind, start: anchorStart, end: anchorStart, calendar: calendar)
        case .week:
            let days = dayCount.map { count in
                (0..<max(1, count)).compactMap { calendar.date(byAdding: .day, value: $0, to: anchorStart) }
            } ?? CalendarGridLayout.weekDays(containing: anchorDate, calendar: calendar)
            return CalendarVisibleRange(kind: kind, start: days.first ?? anchorStart, end: days.last ?? anchorStart, calendar: calendar)
        case .month:
            let cells = CalendarGridLayout.monthCells(for: anchorDate, calendar: calendar)
            return CalendarVisibleRange(kind: kind, start: cells.first ?? anchorStart, end: cells.last ?? anchorStart, calendar: calendar)
        case .year:
            guard let yearStart = calendar.date(from: DateComponents(year: calendar.component(.year, from: anchorDate), month: 1, day: 1)),
                  let nextYear = calendar.date(from: DateComponents(year: calendar.component(.year, from: anchorDate) + 1, month: 1, day: 1)),
                  let yearEnd = calendar.date(byAdding: .day, value: -1, to: nextYear) else {
                return CalendarVisibleRange(kind: kind, start: anchorStart, end: anchorStart, calendar: calendar)
            }
            return CalendarVisibleRange(kind: kind, start: yearStart, end: yearEnd, calendar: calendar)
        case .agenda:
            let count = max(1, dayCount ?? 14)
            let end = calendar.date(byAdding: .day, value: count - 1, to: anchorStart) ?? anchorStart
            return CalendarVisibleRange(kind: kind, start: anchorStart, end: end, calendar: calendar)
        }
    }

    static func calendarEventDayKeys(
        for event: CalendarEventMirror,
        calendar: Calendar
    ) -> [TimeInterval] {
        calendarEventDayKeys(
            startDate: event.startDate,
            endDate: event.endDate,
            isAllDay: event.isAllDay,
            calendar: calendar
        )
    }

    static func calendarEventDayKeys(
        startDate: Date,
        endDate: Date,
        isAllDay: Bool,
        calendar: Calendar
    ) -> [TimeInterval] {
        let startDay = calendar.startOfDay(for: startDate)
        let endDay: Date
        if isAllDay {
            endDay = calendar.startOfDay(for: endDate)
        } else {
            let nextStartDay = calendar.date(byAdding: .day, value: 1, to: startDay)
            let isSameDayTimedEvent = endDate >= startDay
                && (nextStartDay.map { endDate < $0 } ?? false)
            endDay = isSameDayTimedEvent ? startDay : calendar.startOfDay(for: endDate)
        }

        var keys: [TimeInterval] = []
        var cursor = startDay
        var steps = 0
        while cursor <= endDay && steps < 366 {
            keys.append(cursor.timeIntervalSinceReferenceDate)
            guard let next = calendar.date(byAdding: .day, value: 1, to: cursor) else { break }
            cursor = next
            steps += 1
        }
        return keys
    }

    static func fetchCalendarAggregateCounts(
        _ db: Database,
        accountID: String,
        selectedCalendarIDs: Set<CalendarListMirror.ID>?,
        includeCancelled: Bool,
        colorTagBindings: [String: String]
    ) throws -> CalendarAggregateCounts {
        if let selectedCalendarIDs, selectedCalendarIDs.isEmpty {
            return .empty
        }

        let eventCountsByCalendarID = try fetchCalendarCounts(
            db,
            accountID: accountID,
            selectedCalendarIDs: selectedCalendarIDs,
            includeCancelled: includeCancelled
        )
        let eventCountsByColorID = try fetchColorCounts(
            db,
            accountID: accountID,
            selectedCalendarIDs: selectedCalendarIDs,
            includeCancelled: includeCancelled
        )
        var eventCountsByTagName = try fetchLiteralTagCounts(
            db,
            accountID: accountID,
            selectedCalendarIDs: selectedCalendarIDs,
            includeCancelled: includeCancelled
        )
        let colorTagIndex = CalendarEventViewFilter.colorTagIndex(from: colorTagBindings)
        for (tagName, colorID) in colorTagIndex {
            let boundCount = try fetchColorBoundTagCount(
                db,
                accountID: accountID,
                tagName: tagName,
                colorID: colorID,
                selectedCalendarIDs: selectedCalendarIDs,
                includeCancelled: includeCancelled
            )
            if boundCount > 0 {
                eventCountsByTagName[tagName, default: 0] += boundCount
            }
        }

        return CalendarAggregateCounts(
            eventCountsByCalendarID: eventCountsByCalendarID,
            eventCountsByColorID: eventCountsByColorID,
            eventCountsByTagName: eventCountsByTagName
        )
    }

    static func fetchCalendarCounts(
        _ db: Database,
        accountID: String,
        selectedCalendarIDs: Set<CalendarListMirror.ID>?,
        includeCancelled: Bool
    ) throws -> [CalendarListMirror.ID: Int] {
        let countColumn = includeCancelled ? "all_count" : "active_count"
        var predicates = ["account_id = ?", "\(countColumn) > 0"]
        var arguments: StatementArguments = [accountID]
        appendSelectedCalendarPredicate(
            column: "calendar_id",
            selectedCalendarIDs: selectedCalendarIDs,
            predicates: &predicates,
            arguments: &arguments
        )
        return try fetchStringIntMap(
            db,
            sql: """
                SELECT calendar_id AS key, \(countColumn) AS value
                FROM cache_calendar_calendar_counts
                WHERE \(predicates.joined(separator: " AND "))
                """,
            arguments: arguments
        )
    }

    static func fetchColorCounts(
        _ db: Database,
        accountID: String,
        selectedCalendarIDs: Set<CalendarListMirror.ID>?,
        includeCancelled: Bool
    ) throws -> [String: Int] {
        if selectedCalendarIDs == nil {
            let countColumn = includeCancelled ? "all_count" : "active_count"
            return try fetchStringIntMap(
                db,
                sql: """
                    SELECT color_id AS key, \(countColumn) AS value
                    FROM cache_calendar_color_counts
                    WHERE account_id = ? AND \(countColumn) > 0
                    """,
                arguments: [accountID]
            )
        }

        var predicates = ["account_id = ?"]
        var arguments: StatementArguments = [accountID]
        appendActiveEventPredicate(includeCancelled: includeCancelled, predicates: &predicates, arguments: &arguments)
        appendSelectedCalendarPredicate(
            column: "calendar_id",
            selectedCalendarIDs: selectedCalendarIDs,
            predicates: &predicates,
            arguments: &arguments
        )
        return try fetchStringIntMap(
            db,
            sql: """
                SELECT color_id AS key, COUNT(*) AS value
                FROM cache_calendar_event_index
                WHERE \(predicates.joined(separator: " AND "))
                GROUP BY color_id
                """,
            arguments: arguments
        )
    }

    static func fetchLiteralTagCounts(
        _ db: Database,
        accountID: String,
        selectedCalendarIDs: Set<CalendarListMirror.ID>?,
        includeCancelled: Bool
    ) throws -> [String: Int] {
        if selectedCalendarIDs == nil {
            let countColumn = includeCancelled ? "all_count" : "active_count"
            return try fetchStringIntMap(
                db,
                sql: """
                    SELECT tag_name AS key, \(countColumn) AS value
                    FROM cache_calendar_tag_counts
                    WHERE account_id = ? AND \(countColumn) > 0
                    """,
                arguments: [accountID]
            )
        }

        var predicates = ["i.account_id = ?"]
        var arguments: StatementArguments = [accountID]
        appendActiveEventPredicate(columnPrefix: "i.", includeCancelled: includeCancelled, predicates: &predicates, arguments: &arguments)
        appendSelectedCalendarPredicate(
            column: "i.calendar_id",
            selectedCalendarIDs: selectedCalendarIDs,
            predicates: &predicates,
            arguments: &arguments
        )
        return try fetchStringIntMap(
            db,
            sql: """
                SELECT t.tag_name AS key, COUNT(*) AS value
                FROM cache_calendar_event_tags t
                JOIN cache_calendar_event_index i
                    ON i.account_id = t.account_id AND i.event_id = t.event_id
                WHERE \(predicates.joined(separator: " AND "))
                GROUP BY t.tag_name
                """,
            arguments: arguments
        )
    }

    static func fetchColorBoundTagCount(
        _ db: Database,
        accountID: String,
        tagName: String,
        colorID: String,
        selectedCalendarIDs: Set<CalendarListMirror.ID>?,
        includeCancelled: Bool
    ) throws -> Int {
        var predicates = ["i.account_id = ?"]
        var arguments: StatementArguments = [accountID]
        appendActiveEventPredicate(columnPrefix: "i.", includeCancelled: includeCancelled, predicates: &predicates, arguments: &arguments)
        appendSelectedCalendarPredicate(
            column: "i.calendar_id",
            selectedCalendarIDs: selectedCalendarIDs,
            predicates: &predicates,
            arguments: &arguments
        )
        predicates.append("i.color_id = ?")
        arguments += [colorID]
        predicates.append("""
            NOT EXISTS (
                SELECT 1
                FROM cache_calendar_event_tags t
                WHERE t.account_id = i.account_id
                    AND t.event_id = i.event_id
                    AND t.tag_name = ?
            )
            """)
        arguments += [tagName]

        return try Int.fetchOne(
            db,
            sql: """
                SELECT COUNT(*)
                FROM cache_calendar_event_index i
                WHERE \(predicates.joined(separator: " AND "))
                """,
            arguments: arguments
        ) ?? 0
    }

    static func fetchStringIntMap(
        _ db: Database,
        sql: String,
        arguments: StatementArguments
    ) throws -> [String: Int] {
        let rows = try Row.fetchAll(db, sql: sql, arguments: arguments)
        var values: [String: Int] = [:]
        values.reserveCapacity(rows.count)
        for row in rows {
            let value: Int = row["value"]
            guard value > 0 else { continue }
            values[row["key"] as String] = value
        }
        return values
    }

    static func appendActiveEventPredicate(
        columnPrefix: String = "",
        includeCancelled: Bool,
        predicates: inout [String],
        arguments: inout StatementArguments
    ) {
        guard includeCancelled == false else { return }
        predicates.append("\(columnPrefix)status <> ?")
        arguments += [CalendarEventStatus.cancelled.rawValue]
    }

    static func appendSelectedCalendarPredicate(
        column: String,
        selectedCalendarIDs: Set<CalendarListMirror.ID>?,
        predicates: inout [String],
        arguments: inout StatementArguments
    ) {
        guard let selectedCalendarIDs else { return }
        guard selectedCalendarIDs.isEmpty == false else {
            predicates.append("0 = 1")
            return
        }
        let sortedIDs = selectedCalendarIDs.sorted()
        predicates.append("\(column) IN (\(placeholders(count: sortedIDs.count)))")
        for id in sortedIDs {
            arguments += [id]
        }
    }

    static func placeholders(count: Int) -> String {
        Array(repeating: "?", count: count).joined(separator: ", ")
    }

    static func searchEntities(
        _ db: Database,
        accountID: String,
        query rawQuery: String,
        scope: LocalCacheEntitySearchScope,
        limit: Int,
        encryptionKey: SymmetricKey?
    ) throws -> LocalCacheEntitySearchResults {
        let query = rawQuery.trimmingCharacters(in: .whitespacesAndNewlines)
        guard query.isEmpty == false else { return .empty }
        let parsed = AdvancedSearchParser.parse(query)
        guard parsed.regex == nil else { return .empty }

        let freeText = parsed.freeText.trimmingCharacters(in: .whitespacesAndNewlines)
        let ftsQuery = Self.ftsQuery(for: freeText)
        var remaining = limit
        var tasks: [TaskMirror] = []
        var events: [CalendarEventMirror] = []

        if scope.includesTasks, remaining > 0 {
            let ids = try searchTaskIDs(
                db,
                accountID: accountID,
                parsed: parsed,
                scope: scope,
                ftsQuery: ftsQuery,
                limit: remaining
            )
            tasks = try fetchTasks(db, accountID: accountID, ids: ids, encryptionKey: encryptionKey)
            remaining -= tasks.count
        }

        if scope.includesEvents, remaining > 0 {
            let ids = try searchEventIDs(
                db,
                accountID: accountID,
                parsed: parsed,
                ftsQuery: ftsQuery,
                limit: remaining
            )
            events = try fetchEvents(db, accountID: accountID, ids: ids, encryptionKey: encryptionKey)
        }

        return LocalCacheEntitySearchResults(tasks: tasks, events: events)
    }

    static func fetchOverdueTasks(
        _ db: Database,
        accountID: String,
        before cutoff: Date,
        visibleTaskListIDs: Set<TaskListMirror.ID>?,
        limit: Int,
        encryptionKey: SymmetricKey?
    ) throws -> OverdueTaskProjection {
        var predicates = [
            "account_id = ?",
            "due_date IS NOT NULL",
            "due_date < ?",
            "status <> ?",
            "is_deleted = 0",
            "is_hidden = 0"
        ]
        var arguments: StatementArguments = [
            accountID,
            cutoff.timeIntervalSince1970,
            TaskStatus.completed.rawValue
        ]
        if let visibleTaskListIDs {
            if visibleTaskListIDs.isEmpty {
                predicates.append("0 = 1")
            } else {
                let sortedIDs = visibleTaskListIDs.sorted()
                predicates.append("task_list_id IN (\(placeholders(count: sortedIDs.count)))")
                for id in sortedIDs {
                    arguments += [id]
                }
            }
        }

        let whereSQL = predicates.joined(separator: " AND ")
        let totalCount = try Int.fetchOne(
            db,
            sql: "SELECT COUNT(*) FROM cache_task_render_index WHERE \(whereSQL)",
            arguments: arguments
        ) ?? 0
        guard totalCount > 0, limit != 0 else {
            return OverdueTaskProjection(tasks: [], totalCount: totalCount)
        }

        var idArguments = arguments
        var sql = """
            SELECT id
            FROM cache_task_render_index
            WHERE \(whereSQL)
            ORDER BY due_date ASC, title COLLATE NOCASE ASC, id ASC
            """
        if limit > 0 {
            sql += "\nLIMIT ?"
            idArguments += [limit]
        }
        let ids = try String.fetchAll(db, sql: sql, arguments: idArguments)
        let tasks = try fetchTasks(db, accountID: accountID, ids: ids, encryptionKey: encryptionKey)
        return OverdueTaskProjection(tasks: tasks, totalCount: totalCount)
    }

    static func searchTaskIDs(
        _ db: Database,
        accountID: String,
        parsed: AdvancedSearchQuery,
        scope: LocalCacheEntitySearchScope,
        ftsQuery: String?,
        limit: Int
    ) throws -> [String] {
        if parsed.calendarMatch != nil || parsed.attendeeMatch != nil || parsed.requireLocation {
            return []
        }
        if let kind = parsed.kind, kind != .task && kind != .note {
            return []
        }

        let notesSurfacePredicate = "(due_date IS NULL OR TRIM(notes) <> '')"
        var predicates = ["account_id = ?", "is_deleted = 0"]
        var arguments: StatementArguments = [accountID]
        switch scope {
        case .tasks:
            predicates.append("due_date IS NOT NULL")
        case .notes:
            predicates.append(notesSurfacePredicate)
        case .all, .events:
            break
        }
        if parsed.kind == .task {
            predicates.append("due_date IS NOT NULL")
        } else if parsed.kind == .note {
            predicates.append(notesSurfacePredicate)
        }
        if let listMatch = parsed.listMatch {
            predicates.append("task_list_id = ?")
            arguments += [listMatch]
        }
        for title in parsed.titleContains {
            predicates.append("title LIKE ? ESCAPE '\\'")
            arguments += [Self.likePattern(for: title)]
        }
        for tag in parsed.tagsAll {
            predicates.append("tag_text LIKE ? ESCAPE '\\'")
            arguments += [Self.likePattern(for: tag)]
        }
        if parsed.requireNotes {
            predicates.append("notes <> ''")
        }
        if parsed.requireDue {
            predicates.append("due_date IS NOT NULL")
        }
        if parsed.requireCompleted {
            predicates.append("status = ?")
            arguments += [TaskStatus.completed.rawValue]
        }
        if parsed.requireOverdue {
            predicates.append("due_date IS NOT NULL AND due_date < ?")
            arguments += [Calendar.current.startOfDay(for: Date()).timeIntervalSince1970]
        }

        let usesFTSTable = ftsQuery != nil
            || parsed.tagsAll.isEmpty == false
            || parsed.requireNotes
            || scope == .notes
            || parsed.kind == .note
        let sql: String
        if let ftsQuery {
            arguments = [ftsQuery] + arguments + [limit]
            sql = """
                SELECT id
                FROM cache_task_search_fts
                WHERE cache_task_search_fts MATCH ?
                    AND \(predicates.joined(separator: " AND "))
                ORDER BY bm25(cache_task_search_fts, 0.1, 0.1, 10.0, 2.0, 4.0) ASC, updated_at DESC, id ASC
                LIMIT ?
                """
        } else if usesFTSTable {
            arguments += [limit]
            sql = """
                SELECT id
                FROM cache_task_search_fts
                WHERE \(predicates.joined(separator: " AND "))
                ORDER BY updated_at DESC, title COLLATE NOCASE ASC, id ASC
                LIMIT ?
                """
        } else {
            arguments += [limit]
            sql = """
                SELECT id
                FROM cache_task_render_index
                WHERE \(predicates.joined(separator: " AND "))
                ORDER BY COALESCE(updated_at, 0) DESC, title COLLATE NOCASE ASC, id ASC
                LIMIT ?
                """
        }
        return try String.fetchAll(db, sql: sql, arguments: arguments)
    }

    static func searchEventIDs(
        _ db: Database,
        accountID: String,
        parsed: AdvancedSearchQuery,
        ftsQuery: String?,
        limit: Int
    ) throws -> [String] {
        if parsed.listMatch != nil || parsed.requireOverdue || parsed.requireCompleted || parsed.requireDue || parsed.tagsAll.isEmpty == false {
            return []
        }
        if let kind = parsed.kind, kind != .event {
            return []
        }

        var predicates = ["account_id = ?", "status <> ?"]
        var arguments: StatementArguments = [accountID, CalendarEventStatus.cancelled.rawValue]
        if let calendarMatch = parsed.calendarMatch {
            predicates.append("calendar_id = ?")
            arguments += [calendarMatch]
        }
        for title in parsed.titleContains {
            predicates.append("summary LIKE ? ESCAPE '\\'")
            arguments += [Self.likePattern(for: title)]
        }
        if let attendee = parsed.attendeeMatch {
            predicates.append("attendee_text LIKE ? ESCAPE '\\'")
            arguments += [Self.likePattern(for: attendee)]
        }
        if parsed.requireNotes {
            predicates.append("details <> ''")
        }
        if parsed.requireLocation {
            predicates.append("location <> ''")
        }

        let usesFTSTable = ftsQuery != nil || parsed.attendeeMatch != nil || parsed.requireNotes || parsed.requireLocation
        let sql: String
        if let ftsQuery {
            arguments = [ftsQuery] + arguments + [limit]
            sql = """
                SELECT id
                FROM cache_event_search_fts
                WHERE cache_event_search_fts MATCH ?
                    AND \(predicates.joined(separator: " AND "))
                ORDER BY bm25(cache_event_search_fts, 0.1, 0.1, 10.0, 2.0, 3.0, 2.0, 1.0) ASC, start_date ASC, id ASC
                LIMIT ?
                """
        } else if usesFTSTable {
            arguments += [limit]
            sql = """
                SELECT id
                FROM cache_event_search_fts
                WHERE \(predicates.joined(separator: " AND "))
                ORDER BY start_date ASC, summary COLLATE NOCASE ASC, id ASC
                LIMIT ?
                """
        } else {
            arguments += [limit]
            sql = """
                SELECT id
                FROM cache_event_render_index
                WHERE \(predicates.joined(separator: " AND "))
                ORDER BY start_date ASC, summary COLLATE NOCASE ASC, id ASC
                LIMIT ?
                """
        }
        return try String.fetchAll(db, sql: sql, arguments: arguments)
    }

    static func fetchTasks(
        _ db: Database,
        accountID: String,
        ids: [String],
        encryptionKey: SymmetricKey?
    ) throws -> [TaskMirror] {
        guard ids.isEmpty == false else { return [] }
        var byID: [String: TaskMirror] = [:]
        let statement = try db.makeStatement(sql: "SELECT id, payload FROM cache_tasks WHERE account_id = ? AND id = ?")
        for id in ids {
            guard let row = try Row.fetchOne(statement, arguments: [accountID, id]) else { continue }
            let payload: Data = row["payload"]
            byID[id] = try PayloadCoder.decode(TaskMirror.self, from: payload, encryptionKey: encryptionKey)
        }
        return ids.compactMap { byID[$0] }
    }

    static func fetchEvents(
        _ db: Database,
        accountID: String,
        ids: [String],
        encryptionKey: SymmetricKey?
    ) throws -> [CalendarEventMirror] {
        guard ids.isEmpty == false else { return [] }
        var byID: [String: CalendarEventMirror] = [:]
        let statement = try db.makeStatement(sql: "SELECT id, payload FROM cache_events WHERE account_id = ? AND id = ?")
        for id in ids {
            guard let row = try Row.fetchOne(statement, arguments: [accountID, id]) else { continue }
            let payload: Data = row["payload"]
            byID[id] = try PayloadCoder.decode(CalendarEventMirror.self, from: payload, encryptionKey: encryptionKey)
        }
        return ids.compactMap { byID[$0] }
    }

    static func ftsQuery(for raw: String) -> String? {
        let tokens = raw
            .folding(options: [.caseInsensitive, .diacriticInsensitive], locale: .current)
            .lowercased()
            .split { character in
                character.isLetter == false && character.isNumber == false
            }
            .map(String.init)
            .filter { $0.isEmpty == false }
            .prefix(8)
        guard tokens.isEmpty == false else { return nil }
        return tokens.map { "\($0)*" }.joined(separator: " ")
    }

    static func likePattern(for raw: String) -> String {
        let escaped = raw
            .replacingOccurrences(of: "\\", with: "\\\\")
            .replacingOccurrences(of: "%", with: "\\%")
            .replacingOccurrences(of: "_", with: "\\_")
        return "%\(escaped)%"
    }

    static func attendeeSearchText(for event: CalendarEventMirror) -> String {
        (
            event.attendeeEmails
                + event.attendeeResponses.map(\.email)
                + event.attendeeResponses.compactMap(\.displayName)
        )
        .joined(separator: " ")
    }

    static func fetchEventTags(
        _ db: Database,
        accountID: String,
        eventIDs: Set<String>
    ) throws -> [String: Set<String>] {
        guard eventIDs.isEmpty == false else { return [:] }
        var tagsByEventID: [String: Set<String>] = [:]
        let statement = try db.makeStatement(sql: "SELECT tag_name FROM cache_calendar_event_tags WHERE account_id = ? AND event_id = ?")
        for id in eventIDs {
            let rows = try Row.fetchAll(statement, arguments: [accountID, id])
            tagsByEventID[id] = Set(rows.map { row -> String in row["tag_name"] })
        }
        return tagsByEventID
    }

    static func eventTagNames(
        literalTags: Set<String>,
        colorID: String,
        colorTagIndex: [String: String]
    ) -> Set<String> {
        var tags = literalTags
        for (tagName, boundColorID) in colorTagIndex where CalendarEventViewFilter.normalizedColorID(boundColorID) == colorID {
            tags.insert(tagName)
        }
        return tags
    }

    static let sideEffectRebuildResourceType = "__all__"
    static let sideEffectRebuildResourceID = "__all__"

    static func enqueueSideEffectDirtyItems(
        _ db: Database,
        accountID: String,
        changeSet: SyncChangeSet
    ) throws {
        for id in changeSet.tasks.inserted.union(changeSet.tasks.updated) {
            try enqueueSideEffectDirtyItem(db, accountID: accountID, resourceType: .task, resourceID: id, operation: .upsert)
        }
        for id in changeSet.tasks.deleted {
            try enqueueSideEffectDirtyItem(db, accountID: accountID, resourceType: .task, resourceID: id, operation: .delete)
        }
        for id in changeSet.events.inserted.union(changeSet.events.updated) {
            try enqueueSideEffectDirtyItem(db, accountID: accountID, resourceType: .event, resourceID: id, operation: .upsert)
        }
        for id in changeSet.events.deleted {
            try enqueueSideEffectDirtyItem(db, accountID: accountID, resourceType: .event, resourceID: id, operation: .delete)
        }
        if changeSet.settingsChanged || changeSet.calendars.hasChanges {
            try enqueueSideEffectRebuild(db, accountID: accountID, targets: [.notification])
        }
    }

    static func enqueueSideEffectDirtyItem(
        _ db: Database,
        accountID: String,
        resourceType: SyncResourceType,
        resourceID: String,
        operation: LocalIntegrationDirtyOperation,
        targets: Set<LocalIntegrationDirtyTarget> = Set(LocalIntegrationDirtyTarget.allCases)
    ) throws {
        let now = Date().timeIntervalSince1970
        let statement = try db.makeStatement(sql: """
            INSERT INTO cache_side_effect_dirty_queue
                (account_id, target, resource_type, resource_id, operation, enqueued_at)
            VALUES
                (?, ?, ?, ?, ?, ?)
            ON CONFLICT(account_id, target, resource_type, resource_id) DO UPDATE SET
                operation = CASE
                    WHEN cache_side_effect_dirty_queue.operation = ? THEN cache_side_effect_dirty_queue.operation
                    ELSE excluded.operation
                END,
                enqueued_at = excluded.enqueued_at
            """)
        for target in targets {
            try statement.execute(arguments: [
                accountID,
                target.rawValue,
                resourceType.rawValue,
                resourceID,
                operation.rawValue,
                now,
                LocalIntegrationDirtyOperation.rebuild.rawValue
            ])
        }
    }

    static func enqueueSideEffectRebuild(
        _ db: Database,
        accountID: String,
        targets: Set<LocalIntegrationDirtyTarget>
    ) throws {
        guard targets.isEmpty == false else { return }
        let now = Date().timeIntervalSince1970
        let statement = try db.makeStatement(sql: """
            INSERT INTO cache_side_effect_dirty_queue
                (account_id, target, resource_type, resource_id, operation, enqueued_at)
            VALUES
                (?, ?, ?, ?, ?, ?)
            ON CONFLICT(account_id, target, resource_type, resource_id) DO UPDATE SET
                operation = excluded.operation,
                enqueued_at = excluded.enqueued_at
            """)
        for target in targets {
            try statement.execute(arguments: [
                accountID,
                target.rawValue,
                sideEffectRebuildResourceType,
                sideEffectRebuildResourceID,
                LocalIntegrationDirtyOperation.rebuild.rawValue,
                now
            ])
        }
    }

    static func fetchSideEffectDirtyItems(
        _ db: Database,
        target: LocalIntegrationDirtyTarget,
        limit: Int
    ) throws -> [LocalIntegrationDirtyItem] {
        let rows = try Row.fetchAll(
            db,
            sql: """
                SELECT account_id, target, resource_type, resource_id, operation, enqueued_at
                FROM cache_side_effect_dirty_queue
                WHERE target = ?
                ORDER BY enqueued_at ASC
                LIMIT ?
                """,
            arguments: [target.rawValue, limit]
        )
        return rows.compactMap { row in
            let operationRaw: String = row["operation"]
            guard let operation = LocalIntegrationDirtyOperation(rawValue: operationRaw) else { return nil }
            let resourceTypeRaw: String = row["resource_type"]
            let resourceIDRaw: String = row["resource_id"]
            return LocalIntegrationDirtyItem(
                accountID: row["account_id"],
                target: target,
                resourceType: SyncResourceType(rawValue: resourceTypeRaw),
                resourceID: resourceIDRaw == sideEffectRebuildResourceID ? nil : resourceIDRaw,
                operation: operation,
                enqueuedAt: Date(timeIntervalSince1970: row["enqueued_at"] as Double)
            )
        }
    }

    static func deleteSideEffectDirtyItems(
        _ db: Database,
        items: [LocalIntegrationDirtyItem]
    ) throws {
        let statement = try db.makeStatement(sql: """
            DELETE FROM cache_side_effect_dirty_queue
            WHERE account_id = ? AND target = ? AND resource_type = ? AND resource_id = ?
            """)
        for item in items {
            try statement.execute(arguments: [
                item.accountID,
                item.target.rawValue,
                item.resourceType?.rawValue ?? sideEffectRebuildResourceType,
                item.resourceID ?? sideEffectRebuildResourceID
            ])
        }
    }

    static func fetchStoredTaskRows(
        _ db: Database,
        encryptionKey: SymmetricKey?
    ) throws -> [EncodedState.TaskRow] {
        let rows = try Row.fetchAll(
            db,
            sql: """
                SELECT account_id, id, position, payload, content_hash
                FROM cache_tasks
                ORDER BY account_id ASC, position ASC
                """
        )
        return try rows.map { row in
            let payload: Data = row["payload"]
            let task = try PayloadCoder.decode(
                TaskMirror.self,
                from: payload,
                encryptionKey: encryptionKey
            )
            return EncodedState.TaskRow(
                accountID: row["account_id"],
                id: row["id"],
                position: row["position"],
                taskListID: task.taskListID,
                title: task.title,
                notes: task.notes,
                status: task.status.rawValue,
                dueDate: task.dueDate?.timeIntervalSince1970,
                completedAt: task.completedAt?.timeIntervalSince1970,
                isDeleted: task.isDeleted,
                isHidden: task.isHidden,
                positionValue: task.position,
                updatedAt: task.updatedAt?.timeIntervalSince1970,
                tagText: TagExtractor.tags(in: task.title).joined(separator: " "),
                searchText: [task.title, task.notes].joined(separator: "\n"),
                payload: payload,
                contentHash: row["content_hash"]
            )
        }
    }

    static func fetchStoredEventRows(
        _ db: Database,
        encryptionKey: SymmetricKey?
    ) throws -> [EncodedState.EventRow] {
        let rows = try Row.fetchAll(
            db,
            sql: """
                SELECT account_id, id, position, payload, content_hash
                FROM cache_events
                ORDER BY account_id ASC, position ASC
                """
        )
        return try rows.map { row in
            let payload: Data = row["payload"]
            let event = try PayloadCoder.decode(
                CalendarEventMirror.self,
                from: payload,
                encryptionKey: encryptionKey
            )
            return EncodedState.EventRow(
                accountID: row["account_id"],
                id: row["id"],
                position: row["position"],
                calendarID: event.calendarID,
                summary: event.summary,
                details: event.details,
                startDate: event.startDate.timeIntervalSince1970,
                endDate: event.endDate.timeIntervalSince1970,
                isAllDay: event.isAllDay,
                updatedAt: event.updatedAt?.timeIntervalSince1970,
                etag: event.etag,
                status: event.status.rawValue,
                colorID: CalendarEventColor.from(colorId: event.colorId).rawValue,
                location: event.location,
                attendeeText: Self.attendeeSearchText(for: event),
                meetLink: event.meetLink,
                transparency: event.transparency.rawValue,
                availabilityHoldGroupID: event.availabilityHold?.groupID,
                tagNames: CalendarEventViewFilter.literalTagNames(in: event),
                dayKeys: calendarEventDayKeys(for: event, calendar: .current),
                searchText: [event.summary, event.details, event.location, Self.attendeeSearchText(for: event), event.meetLink].joined(separator: "\n"),
                payload: payload,
                contentHash: row["content_hash"]
            )
        }
    }

    static func existingHash(
        _ db: Database,
        table: String,
        idColumn: String,
        id: String
    ) throws -> String? {
        try String.fetchOne(
            db,
            sql: "SELECT content_hash FROM \(table) WHERE \(idColumn) = ?",
            arguments: [id]
        )
    }

    static func existingScopedHash(
        _ db: Database,
        table: String,
        accountID: String,
        idColumn: String? = nil,
        id: String? = nil
    ) throws -> String? {
        if let idColumn, let id {
            return try String.fetchOne(
                db,
                sql: "SELECT content_hash FROM \(table) WHERE account_id = ? AND \(idColumn) = ?",
                arguments: [accountID, id]
            )
        }
        return try String.fetchOne(
            db,
            sql: "SELECT content_hash FROM \(table) WHERE account_id = ?",
            arguments: [accountID]
        )
    }

    static func insertState(_ db: Database, _ row: EncodedState.StateRow) throws {
        try db.execute(
            sql: """
                INSERT INTO cache_state
                    (id, schema_version, active_account_id, primary_account_id, payload, content_hash, updated_at)
                VALUES
                    (1, ?, ?, ?, ?, ?, ?)
                """,
            arguments: [
                row.schemaVersion,
                row.activeAccountID,
                row.primaryAccountID,
                row.payload,
                row.contentHash,
                row.updatedAt
            ]
        )
    }

    static func insertAccounts(_ db: Database, _ rows: [EncodedState.AccountRow]) throws {
        let statement = try db.makeStatement(sql: """
            INSERT INTO cache_accounts (id, position, payload, content_hash)
            VALUES (?, ?, ?, ?)
            """)
        for row in rows {
            try statement.execute(arguments: [row.id, row.position, row.payload, row.contentHash])
        }
    }

    static func insertWorkspaces(_ db: Database, _ rows: [EncodedState.WorkspaceRow]) throws {
        let statement = try db.makeStatement(sql: """
            INSERT INTO cache_workspaces (account_id, position, payload, content_hash)
            VALUES (?, ?, ?, ?)
            """)
        for row in rows {
            try statement.execute(arguments: [row.accountID, row.position, row.payload, row.contentHash])
        }
    }

    static func insertTaskLists(_ db: Database, _ rows: [EncodedState.EntityRow]) throws {
        let statement = try db.makeStatement(sql: """
            INSERT INTO cache_task_lists (account_id, id, position, payload, content_hash)
            VALUES (?, ?, ?, ?, ?)
            """)
        for row in rows {
            try statement.execute(arguments: [row.accountID, row.id, row.position, row.payload, row.contentHash])
        }
    }

    static func insertTasks(_ db: Database, _ rows: [EncodedState.TaskRow]) throws {
        let statement = try db.makeStatement(sql: """
            INSERT INTO cache_tasks (account_id, id, position, task_list_id, updated_at, payload, content_hash)
            VALUES (?, ?, ?, ?, ?, ?, ?)
            """)
        for row in rows {
            try statement.execute(arguments: [
                row.accountID,
                row.id,
                row.position,
                row.taskListID,
                row.updatedAt,
                row.payload,
                row.contentHash
            ])
        }
    }

    static func insertCalendars(_ db: Database, _ rows: [EncodedState.EntityRow]) throws {
        let statement = try db.makeStatement(sql: """
            INSERT INTO cache_calendars (account_id, id, position, payload, content_hash)
            VALUES (?, ?, ?, ?, ?)
            """)
        for row in rows {
            try statement.execute(arguments: [row.accountID, row.id, row.position, row.payload, row.contentHash])
        }
    }

    static func insertEvents(_ db: Database, _ rows: [EncodedState.EventRow]) throws {
        let statement = try db.makeStatement(sql: """
            INSERT INTO cache_events
                (account_id, id, position, calendar_id, start_date, end_date, updated_at, etag, status, payload, content_hash)
            VALUES
                (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            """)
        for row in rows {
            try statement.execute(arguments: [
                row.accountID,
                row.id,
                row.position,
                row.calendarID,
                row.startDate,
                row.endDate,
                row.updatedAt,
                row.etag,
                row.status,
                row.payload,
                row.contentHash
            ])
        }
    }

    static func insertSyncCheckpoints(_ db: Database, _ rows: [EncodedState.CheckpointRow]) throws {
        let statement = try db.makeStatement(sql: """
            INSERT INTO cache_sync_checkpoints
                (account_id, checkpoint_id, position, resource_type, resource_id, last_successful_sync_at, payload, content_hash)
            VALUES
                (?, ?, ?, ?, ?, ?, ?, ?)
            """)
        for row in rows {
            try statement.execute(arguments: [
                row.accountID,
                row.id,
                row.position,
                row.resourceType,
                row.resourceID,
                row.lastSuccessfulSyncAt,
                row.payload,
                row.contentHash
            ])
        }
    }

    static func insertPendingMutations(_ db: Database, _ rows: [EncodedState.PendingMutationRow]) throws {
        let statement = try db.makeStatement(sql: """
            INSERT INTO cache_pending_mutations
                (account_id, id, position, resource_type, resource_id, created_at, payload, content_hash)
            VALUES
                (?, ?, ?, ?, ?, ?, ?, ?)
            """)
        for row in rows {
            try statement.execute(arguments: [
                row.accountID,
                row.id,
                row.position,
                row.resourceType,
                row.resourceID,
                row.createdAt,
                row.payload,
                row.contentHash
            ])
        }
    }

    static func timestamp() -> UInt64 {
        DispatchTime.now().uptimeNanoseconds
    }

    static func milliseconds(from start: UInt64, to end: UInt64) -> Double {
        Double(end - start) / 1_000_000
    }
}

private struct EncodedState {
    struct StateRow {
        var schemaVersion: Int
        var activeAccountID: String?
        var primaryAccountID: String?
        var payload: Data
        var contentHash: String
        var updatedAt: String
    }

    struct AccountRow {
        var id: String
        var position: Int
        var payload: Data
        var contentHash: String
    }

    struct WorkspaceRow {
        var accountID: String
        var position: Int
        var payload: Data
        var contentHash: String
    }

    struct EntityRow {
        var accountID: String
        var id: String
        var position: Int
        var payload: Data
        var contentHash: String
    }

    struct TaskRow {
        var accountID: String
        var id: String
        var position: Int
        var taskListID: String
        var title: String
        var notes: String
        var status: String
        var dueDate: Double?
        var completedAt: Double?
        var isDeleted: Bool
        var isHidden: Bool
        var positionValue: String?
        var updatedAt: Double?
        var tagText: String
        var searchText: String
        var payload: Data
        var contentHash: String
    }

    struct EventRow {
        var accountID: String
        var id: String
        var position: Int
        var calendarID: String
        var summary: String
        var details: String
        var startDate: Double
        var endDate: Double
        var isAllDay: Bool
        var updatedAt: Double?
        var etag: String?
        var status: String
        var colorID: String
        var location: String
        var attendeeText: String
        var meetLink: String
        var transparency: String
        var availabilityHoldGroupID: String?
        var tagNames: Set<String>
        var dayKeys: [TimeInterval]
        var searchText: String
        var payload: Data
        var contentHash: String
    }

    struct CheckpointRow {
        var accountID: String
        var id: String
        var position: Int
        var resourceType: String
        var resourceID: String
        var lastSuccessfulSyncAt: Double?
        var payload: Data
        var contentHash: String
    }

    struct PendingMutationRow {
        var accountID: String
        var id: String
        var position: Int
        var resourceType: String
        var resourceID: String
        var createdAt: Double
        var payload: Data
        var contentHash: String
    }

    var state: StateRow
    var accounts: [AccountRow] = []
    var workspaces: [WorkspaceRow] = []
    var taskLists: [EntityRow] = []
    var tasks: [TaskRow] = []
    var calendars: [EntityRow] = []
    var events: [EventRow] = []
    var syncCheckpoints: [CheckpointRow] = []
    var pendingMutations: [PendingMutationRow] = []

    var activeStorageAccountID: String {
        state.activeAccountID ?? LocalCacheDatabaseStore.unscopedAccountID
    }

    var totalEntityRows: Int {
        1
            + accounts.count
            + workspaces.count
            + taskLists.count
            + tasks.count
            + calendars.count
            + events.count
            + syncCheckpoints.count
            + pendingMutations.count
    }

    init(state cachedState: CachedAppState, encryptionKey: SymmetricKey?, salt: Data?) throws {
        try self.init(state: cachedState, changeSet: nil, encryptionKey: encryptionKey, salt: salt)
    }

    init(state cachedState: CachedAppState, changeSet: SyncChangeSet?, encryptionKey: SymmetricKey?, salt: Data?) throws {
        if encryptionKey != nil, salt == nil {
            throw LocalCacheDatabaseStore.CacheDatabaseError.missingEncryptionSalt
        }
        let rowFilter = RowFilter(changeSet: changeSet)
        let metadataState = cachedState.databaseMetadataSnapshot()
        let statePayload = try PayloadCoder.encode(
            metadataState,
            kind: "cachedAppState.metadata",
            encryptionKey: encryptionKey,
            salt: salt
        )
        state = StateRow(
            schemaVersion: cachedState.schemaVersion,
            activeAccountID: cachedState.activeAccountID,
            primaryAccountID: cachedState.account?.id,
            payload: statePayload.payload,
            contentHash: statePayload.contentHash,
            updatedAt: ISO8601DateFormatter().string(from: Date())
        )

        accounts = try cachedState.accounts.enumerated().map { index, account in
            let encoded = try PayloadCoder.encode(account, kind: "account", encryptionKey: encryptionKey, salt: salt)
            return AccountRow(id: account.id, position: index, payload: encoded.payload, contentHash: encoded.contentHash)
        }

        workspaces = try cachedState.accountWorkspaces.enumerated().map { index, workspace in
            let metadata = workspace.databaseMetadataSnapshot()
            let encoded = try PayloadCoder.encode(metadata, kind: "workspace", encryptionKey: encryptionKey, salt: salt)
            return WorkspaceRow(accountID: workspace.accountID, position: index, payload: encoded.payload, contentHash: encoded.contentHash)
        }

        for workspace in cachedState.accountWorkspaces {
            try appendRows(
                accountID: workspace.accountID,
                taskLists: workspace.taskLists,
                tasks: workspace.tasks,
                calendars: workspace.calendars,
                events: workspace.events,
                syncCheckpoints: workspace.syncCheckpoints,
                pendingMutations: workspace.pendingMutations,
                rowFilter: rowFilter,
                encryptionKey: encryptionKey,
                salt: salt
            )
        }

        if cachedState.activeAccountID == nil,
           cachedState.taskLists.isEmpty == false
            || cachedState.tasks.isEmpty == false
            || cachedState.calendars.isEmpty == false
            || cachedState.events.isEmpty == false
            || cachedState.syncCheckpoints.isEmpty == false
            || cachedState.pendingMutations.isEmpty == false {
            try appendRows(
                accountID: LocalCacheDatabaseStore.unscopedAccountID,
                taskLists: cachedState.taskLists,
                tasks: cachedState.tasks,
                calendars: cachedState.calendars,
                events: cachedState.events,
                syncCheckpoints: cachedState.syncCheckpoints,
                pendingMutations: cachedState.pendingMutations,
                rowFilter: rowFilter,
                encryptionKey: encryptionKey,
                salt: salt
            )
        }
    }

    struct RowFilter {
        var taskListIDs: Set<String>?
        var taskIDs: Set<String>?
        var calendarIDs: Set<String>?
        var eventIDs: Set<String>?
        var checkpointIDs: Set<String>?
        var pendingMutationIDs: Set<String>?

        init(changeSet: SyncChangeSet?) {
            guard let changeSet else {
                taskListIDs = nil
                taskIDs = nil
                calendarIDs = nil
                eventIDs = nil
                checkpointIDs = nil
                pendingMutationIDs = nil
                return
            }
            taskListIDs = Self.encodedIDs(changeSet.taskLists)
            taskIDs = Self.encodedIDs(changeSet.tasks)
            calendarIDs = Self.encodedIDs(changeSet.calendars)
            eventIDs = Self.encodedIDs(changeSet.events)
            checkpointIDs = Self.encodedIDs(changeSet.checkpoints)
            pendingMutationIDs = Self.encodedIDs(changeSet.pendingMutations)
        }

        private static func encodedIDs(_ changes: SyncChangeSet.RowChanges) -> Set<String> {
            changes.inserted.union(changes.updated)
        }

        func includesTaskList(_ id: String) -> Bool {
            taskListIDs?.contains(id) ?? true
        }

        func includesTask(_ id: String) -> Bool {
            taskIDs?.contains(id) ?? true
        }

        func includesCalendar(_ id: String) -> Bool {
            calendarIDs?.contains(id) ?? true
        }

        func includesEvent(_ id: String) -> Bool {
            eventIDs?.contains(id) ?? true
        }

        func includesCheckpoint(_ id: String) -> Bool {
            checkpointIDs?.contains(id) ?? true
        }

        func includesPendingMutation(_ id: String) -> Bool {
            pendingMutationIDs?.contains(id) ?? true
        }
    }

    mutating func appendRows(
        accountID: String,
        taskLists: [TaskListMirror],
        tasks: [TaskMirror],
        calendars: [CalendarListMirror],
        events: [CalendarEventMirror],
        syncCheckpoints: [SyncCheckpoint],
        pendingMutations: [PendingMutation],
        rowFilter: RowFilter,
        encryptionKey: SymmetricKey?,
        salt: Data?
    ) throws {
        self.taskLists += try taskLists.enumerated().compactMap { index, taskList in
            guard rowFilter.includesTaskList(taskList.id) else { return nil }
            let encoded = try PayloadCoder.encode(taskList, kind: "taskList", encryptionKey: encryptionKey, salt: salt)
            return EntityRow(accountID: accountID, id: taskList.id, position: index, payload: encoded.payload, contentHash: encoded.contentHash)
        }
        self.tasks += try tasks.enumerated().compactMap { index, task in
            guard rowFilter.includesTask(task.id) else { return nil }
            let encoded = try PayloadCoder.encode(task, kind: "task", encryptionKey: encryptionKey, salt: salt)
            return TaskRow(
                accountID: accountID,
                id: task.id,
                position: index,
                taskListID: task.taskListID,
                title: task.title,
                notes: task.notes,
                status: task.status.rawValue,
                dueDate: task.dueDate?.timeIntervalSince1970,
                completedAt: task.completedAt?.timeIntervalSince1970,
                isDeleted: task.isDeleted,
                isHidden: task.isHidden,
                positionValue: task.position,
                updatedAt: task.updatedAt?.timeIntervalSince1970,
                tagText: TagExtractor.tags(in: task.title).joined(separator: " "),
                searchText: [task.title, task.notes].joined(separator: "\n"),
                payload: encoded.payload,
                contentHash: encoded.contentHash
            )
        }
        self.calendars += try calendars.enumerated().compactMap { index, calendar in
            guard rowFilter.includesCalendar(calendar.id) else { return nil }
            let encoded = try PayloadCoder.encode(calendar, kind: "calendar", encryptionKey: encryptionKey, salt: salt)
            return EntityRow(accountID: accountID, id: calendar.id, position: index, payload: encoded.payload, contentHash: encoded.contentHash)
        }
        self.events += try events.enumerated().compactMap { index, event in
            guard rowFilter.includesEvent(event.id) else { return nil }
            let encoded = try PayloadCoder.encode(event, kind: "event", encryptionKey: encryptionKey, salt: salt)
            return EventRow(
                accountID: accountID,
                id: event.id,
                position: index,
                calendarID: event.calendarID,
                summary: event.summary,
                details: event.details,
                startDate: event.startDate.timeIntervalSince1970,
                endDate: event.endDate.timeIntervalSince1970,
                isAllDay: event.isAllDay,
                updatedAt: event.updatedAt?.timeIntervalSince1970,
                etag: event.etag,
                status: event.status.rawValue,
                colorID: CalendarEventColor.from(colorId: event.colorId).rawValue,
                location: event.location,
                attendeeText: LocalCacheDatabaseStore.attendeeSearchText(for: event),
                meetLink: event.meetLink,
                transparency: event.transparency.rawValue,
                availabilityHoldGroupID: event.availabilityHold?.groupID,
                tagNames: CalendarEventViewFilter.literalTagNames(in: event),
                dayKeys: LocalCacheDatabaseStore.calendarEventDayKeys(for: event, calendar: .current),
                searchText: [
                    event.summary,
                    event.details,
                    event.location,
                    LocalCacheDatabaseStore.attendeeSearchText(for: event),
                    event.meetLink
                ].joined(separator: "\n"),
                payload: encoded.payload,
                contentHash: encoded.contentHash
            )
        }
        self.syncCheckpoints += try syncCheckpoints.enumerated().compactMap { index, checkpoint in
            guard rowFilter.includesCheckpoint(checkpoint.id) else { return nil }
            let encoded = try PayloadCoder.encode(checkpoint, kind: "syncCheckpoint", encryptionKey: encryptionKey, salt: salt)
            return CheckpointRow(
                accountID: accountID,
                id: checkpoint.id,
                position: index,
                resourceType: checkpoint.resourceType.rawValue,
                resourceID: checkpoint.resourceID,
                lastSuccessfulSyncAt: checkpoint.lastSuccessfulSyncAt?.timeIntervalSince1970,
                payload: encoded.payload,
                contentHash: encoded.contentHash
            )
        }
        self.pendingMutations += try pendingMutations.enumerated().compactMap { index, mutation in
            let mutationID = mutation.id.uuidString
            guard rowFilter.includesPendingMutation(mutationID) else { return nil }
            let encoded = try PayloadCoder.encode(mutation, kind: "pendingMutation", encryptionKey: encryptionKey, salt: salt)
            return PendingMutationRow(
                accountID: accountID,
                id: mutationID,
                position: index,
                resourceType: mutation.resourceType.rawValue,
                resourceID: mutation.resourceID,
                createdAt: mutation.createdAt.timeIntervalSince1970,
                payload: encoded.payload,
                contentHash: encoded.contentHash
            )
        }
    }
}

private struct DecodedState {
    var baseState: CachedAppState
    var accounts: [GoogleAccount]
    var workspaces: [AccountWorkspaceSnapshot]
    var taskLists: [String: [TaskListMirror]]
    var tasks: [String: [TaskMirror]]
    var calendars: [String: [CalendarListMirror]]
    var events: [String: [CalendarEventMirror]]
    var syncCheckpoints: [String: [SyncCheckpoint]]
    var pendingMutations: [String: [PendingMutation]]

    static func fetch(_ db: Database, encryptionKey: SymmetricKey?) throws -> DecodedState {
        guard let stateRow = try Row.fetchOne(db, sql: "SELECT payload FROM cache_state WHERE id = 1"),
              let statePayload: Data = stateRow["payload"] else {
            throw LocalCacheDatabaseStore.CacheDatabaseError.missingStateRow
        }

        let baseState = try PayloadCoder.decode(
            CachedAppState.self,
            from: statePayload,
            encryptionKey: encryptionKey
        )
        let accounts = try fetchPayloads(
            db,
            sql: "SELECT payload FROM cache_accounts ORDER BY position ASC",
            type: GoogleAccount.self,
            encryptionKey: encryptionKey
        )
        let workspaces = try fetchPayloads(
            db,
            sql: "SELECT payload FROM cache_workspaces ORDER BY position ASC",
            type: AccountWorkspaceSnapshot.self,
            encryptionKey: encryptionKey
        )

        return DecodedState(
            baseState: baseState,
            accounts: accounts,
            workspaces: workspaces,
            taskLists: try fetchScopedPayloads(db, sql: "SELECT account_id, payload FROM cache_task_lists ORDER BY account_id ASC, position ASC", type: TaskListMirror.self, encryptionKey: encryptionKey),
            tasks: try fetchScopedPayloads(db, sql: "SELECT account_id, payload FROM cache_tasks ORDER BY account_id ASC, position ASC", type: TaskMirror.self, encryptionKey: encryptionKey),
            calendars: try fetchScopedPayloads(db, sql: "SELECT account_id, payload FROM cache_calendars ORDER BY account_id ASC, position ASC", type: CalendarListMirror.self, encryptionKey: encryptionKey),
            events: try fetchScopedPayloads(db, sql: "SELECT account_id, payload FROM cache_events ORDER BY account_id ASC, position ASC", type: CalendarEventMirror.self, encryptionKey: encryptionKey),
            syncCheckpoints: try fetchScopedPayloads(db, sql: "SELECT account_id, payload FROM cache_sync_checkpoints ORDER BY account_id ASC, position ASC", type: SyncCheckpoint.self, encryptionKey: encryptionKey),
            pendingMutations: try fetchScopedPayloads(db, sql: "SELECT account_id, payload FROM cache_pending_mutations ORDER BY account_id ASC, position ASC", type: PendingMutation.self, encryptionKey: encryptionKey)
        )
    }

    func cachedState() -> CachedAppState {
        let loadedAccounts = accounts.isEmpty ? baseState.accounts : accounts
        let workspaceIDs = Set(workspaces.map(\.accountID))
        var mergedWorkspaces = workspaces.map { workspace in
            filled(workspace)
        }

        for baseWorkspace in baseState.accountWorkspaces where workspaceIDs.contains(baseWorkspace.accountID) == false {
            mergedWorkspaces.append(filled(baseWorkspace))
        }

        let activeWorkspace = baseState.activeAccountID.flatMap { activeID in
            mergedWorkspaces.first { $0.accountID == activeID }
        }
        let unscopedTaskLists = taskLists[LocalCacheDatabaseStore.unscopedAccountID] ?? baseState.taskLists
        let unscopedTasks = tasks[LocalCacheDatabaseStore.unscopedAccountID] ?? baseState.tasks
        let unscopedCalendars = calendars[LocalCacheDatabaseStore.unscopedAccountID] ?? baseState.calendars
        let unscopedEvents = events[LocalCacheDatabaseStore.unscopedAccountID] ?? baseState.events
        let unscopedCheckpoints = syncCheckpoints[LocalCacheDatabaseStore.unscopedAccountID] ?? baseState.syncCheckpoints
        let unscopedMutations = pendingMutations[LocalCacheDatabaseStore.unscopedAccountID] ?? baseState.pendingMutations
        let rootTaskLists = activeWorkspace?.taskLists ?? unscopedTaskLists
        let rootTasks = activeWorkspace?.tasks ?? unscopedTasks
        let rootCalendars = activeWorkspace?.calendars ?? unscopedCalendars
        let rootEvents = activeWorkspace?.events ?? unscopedEvents
        let rootCheckpoints = activeWorkspace?.syncCheckpoints ?? unscopedCheckpoints
        let rootMutations = activeWorkspace?.pendingMutations ?? unscopedMutations
        let activeAccount = baseState.activeAccountID.flatMap { activeID in
            loadedAccounts.first { $0.id == activeID }
        } ?? baseState.account

        return CachedAppState(
            account: activeAccount,
            accounts: loadedAccounts,
            activeAccountID: baseState.activeAccountID,
            accountWorkspaces: mergedWorkspaces,
            taskLists: rootTaskLists,
            tasks: rootTasks,
            calendars: rootCalendars,
            events: rootEvents,
            settings: activeWorkspace?.effectiveSettings(mergedWith: baseState.settings) ?? baseState.settings,
            syncCheckpoints: rootCheckpoints,
            pendingMutations: rootMutations,
            schemaVersion: baseState.schemaVersion
        )
    }

    private func filled(_ workspace: AccountWorkspaceSnapshot) -> AccountWorkspaceSnapshot {
        var workspace = workspace
        workspace.taskLists = taskLists[workspace.accountID] ?? workspace.taskLists
        workspace.tasks = tasks[workspace.accountID] ?? workspace.tasks
        workspace.calendars = calendars[workspace.accountID] ?? workspace.calendars
        workspace.events = events[workspace.accountID] ?? workspace.events
        workspace.syncCheckpoints = syncCheckpoints[workspace.accountID] ?? workspace.syncCheckpoints
        workspace.pendingMutations = pendingMutations[workspace.accountID] ?? workspace.pendingMutations
        return workspace.accountStamped()
    }

    private static func fetchPayloads<T: Decodable>(
        _ db: Database,
        sql: String,
        type: T.Type,
        encryptionKey: SymmetricKey?
    ) throws -> [T] {
        try Row.fetchAll(db, sql: sql).map { row in
            let payload: Data = row["payload"]
            return try PayloadCoder.decode(T.self, from: payload, encryptionKey: encryptionKey)
        }
    }

    private static func fetchScopedPayloads<T: Decodable>(
        _ db: Database,
        sql: String,
        type: T.Type,
        encryptionKey: SymmetricKey?
    ) throws -> [String: [T]] {
        var values: [String: [T]] = [:]
        for row in try Row.fetchAll(db, sql: sql) {
            let accountID: String = row["account_id"]
            let payload: Data = row["payload"]
            let value = try PayloadCoder.decode(T.self, from: payload, encryptionKey: encryptionKey)
            values[accountID, default: []].append(value)
        }
        return values
    }
}

private enum PayloadCoder {
    struct EncryptedEnvelope: Codable {
        let encryptedV1: HCBCacheCrypto.EncryptedBlob
    }

    struct EncodedPayload {
        var payload: Data
        var contentHash: String
    }

    static func encode<T: Encodable>(
        _ value: T,
        kind: String,
        encryptionKey: SymmetricKey?,
        salt: Data?
    ) throws -> EncodedPayload {
        let canonical = try JSONEncoder.cacheDatabaseCanonical.encode(value)
        let contentHash = LocalCacheRowHasher.hash(canonicalPayload: canonical, kind: kind)
        guard let encryptionKey else {
            return EncodedPayload(payload: canonical, contentHash: contentHash)
        }
        guard let salt else {
            throw LocalCacheDatabaseStore.CacheDatabaseError.missingEncryptionSalt
        }
        let blob = try HCBCacheCrypto.encrypt(canonical, key: encryptionKey, salt: salt)
        let encrypted = try JSONEncoder.cacheDatabaseCanonical.encode(EncryptedEnvelope(encryptedV1: blob))
        return EncodedPayload(payload: encrypted, contentHash: contentHash)
    }

    static func decode<T: Decodable>(
        _ type: T.Type,
        from data: Data,
        encryptionKey: SymmetricKey?
    ) throws -> T {
        if let envelope = try? JSONDecoder.cacheDatabase.decode(EncryptedEnvelope.self, from: data) {
            guard let encryptionKey else {
                throw LocalCacheDatabaseStore.CacheDatabaseError.encryptedStoreLocked
            }
            let plaintext = try HCBCacheCrypto.decrypt(envelope.encryptedV1, key: encryptionKey)
            return try decodePayload(T.self, from: plaintext)
        }
        return try decodePayload(T.self, from: data)
    }

    private static func decodePayload<T: Decodable>(_ type: T.Type, from data: Data) throws -> T {
        do {
            return try JSONDecoder.cacheDatabase.decode(T.self, from: data)
        } catch {
            return try JSONDecoder.cacheDatabaseLegacyISO8601.decode(T.self, from: data)
        }
    }
}

private extension CachedAppState {
    func databaseMetadataSnapshot() -> CachedAppState {
        var copy = self
        copy.taskLists = []
        copy.tasks = []
        copy.calendars = []
        copy.events = []
        copy.syncCheckpoints = []
        copy.pendingMutations = []
        copy.accountWorkspaces = accountWorkspaces.map { $0.databaseMetadataSnapshot() }
        return copy
    }
}

private extension AccountWorkspaceSnapshot {
    func databaseMetadataSnapshot() -> AccountWorkspaceSnapshot {
        var copy = self
        copy.taskLists = []
        copy.tasks = []
        copy.calendars = []
        copy.events = []
        copy.syncCheckpoints = []
        copy.pendingMutations = []
        return copy
    }
}

private extension Dictionary where Value: Collection {
    var totalValueCount: Int {
        values.reduce(0) { $0 + $1.count }
    }
}

private extension JSONEncoder {
    static var cacheDatabaseCanonical: JSONEncoder {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .secondsSince1970
        encoder.outputFormatting = [.sortedKeys]
        return encoder
    }
}

private extension JSONDecoder {
    static var cacheDatabase: JSONDecoder {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .secondsSince1970
        return decoder
    }

    static var cacheDatabaseLegacyISO8601: JSONDecoder {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return decoder
    }
}
