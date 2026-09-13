import Foundation

struct TodaySnapshotBuildProfile: Sendable {
    var dueTaskMilliseconds: Double
    var overdueTaskMilliseconds: Double
    var eventFilterMilliseconds: Double
    var eventSortMilliseconds: Double
    var totalMilliseconds: Double
    var inputEventCount: Int
    var scheduledEventCount: Int

    static func milliseconds(from start: UInt64, to end: UInt64) -> Double {
        Double(end - start) / 1_000_000
    }
}

struct CalendarSnapshotBuildProfile: Sendable {
    var setupMilliseconds: Double
    var eventScanMilliseconds: Double
    var eventVisibilityMilliseconds: Double
    var calendarCountMilliseconds: Double
    var colorAggregationMilliseconds: Double
    var literalTagExtractionMilliseconds: Double
    var literalSummaryScanMilliseconds: Double
    var literalDetailsScanMilliseconds: Double
    var literalLocationScanMilliseconds: Double
    var literalRegexMatchingMilliseconds: Double
    var literalDeduplicationMilliseconds: Double
    var boundTagAggregationMilliseconds: Double
    var tagCountMilliseconds: Double
    var colorMapMilliseconds: Double
    var totalMilliseconds: Double
    var inputEventCount: Int
    var selectedEventCount: Int
    var visibleEventCount: Int
    var literalTaggedEventCount: Int
    var boundTaggedEventCount: Int

    static func milliseconds(from start: UInt64, to end: UInt64) -> Double {
        Double(end - start) / 1_000_000
    }
}

struct TodaySnapshot: Equatable, Sendable {
    var date: Date
    var dueTasks: [TaskMirror]
    var scheduledEvents: [CalendarEventMirror]
    var overdueCount: Int

    static let empty = TodaySnapshot(date: Date(), dueTasks: [], scheduledEvents: [], overdueCount: 0)

    static func build(
        tasks: [TaskMirror],
        events: [CalendarEventMirror],
        referenceDate: Date,
        calendar: Calendar = .current
    ) -> TodaySnapshot {
        buildSnapshot(
            tasks: tasks,
            events: events,
            scheduledEvents: nil,
            referenceDate: referenceDate,
            calendar: calendar
        )
    }

    static func build(
        tasks: [TaskMirror],
        scheduledEvents: [CalendarEventMirror],
        referenceDate: Date,
        calendar: Calendar = .current
    ) -> TodaySnapshot {
        buildSnapshot(
            tasks: tasks,
            events: nil,
            scheduledEvents: scheduledEvents,
            referenceDate: referenceDate,
            calendar: calendar
        )
    }

    #if DEBUG
    static func buildProfiled(
        tasks: [TaskMirror],
        events: [CalendarEventMirror],
        referenceDate: Date,
        calendar: Calendar = .current
    ) -> (TodaySnapshot, TodaySnapshotBuildProfile) {
        buildSnapshotProfiled(
            tasks: tasks,
            events: events,
            scheduledEvents: nil,
            referenceDate: referenceDate,
            calendar: calendar
        )
    }

    static func buildProfiled(
        tasks: [TaskMirror],
        scheduledEvents: [CalendarEventMirror],
        referenceDate: Date,
        calendar: Calendar = .current
    ) -> (TodaySnapshot, TodaySnapshotBuildProfile) {
        buildSnapshotProfiled(
            tasks: tasks,
            events: nil,
            scheduledEvents: scheduledEvents,
            referenceDate: referenceDate,
            calendar: calendar
        )
    }
    #endif

    private static func buildSnapshot(
        tasks: [TaskMirror],
        events: [CalendarEventMirror]?,
        scheduledEvents: [CalendarEventMirror]?,
        referenceDate: Date,
        calendar: Calendar
    ) -> TodaySnapshot {
        let dueTasks = tasks.filter { task in
            guard !task.isCompleted, !task.isDeleted, let dueDate = task.dueDate else {
                return false
            }
            return calendar.isDate(dueDate, inSameDayAs: referenceDate)
        }

        let overdueCount = tasks.filter { task in
            guard !task.isCompleted, !task.isDeleted, let dueDate = task.dueDate else {
                return false
            }
            return dueDate < calendar.startOfDay(for: referenceDate)
        }.count

        let resolvedEvents = scheduledEvents ?? events?.filter { event in
            event.status != .cancelled && calendar.isDate(event.startDate, inSameDayAs: referenceDate)
        } ?? []
        let sortedEvents = resolvedEvents.sorted { lhs, rhs in
            lhs.startDate < rhs.startDate
        }

        return TodaySnapshot(
            date: referenceDate,
            dueTasks: dueTasks,
            scheduledEvents: sortedEvents,
            overdueCount: overdueCount
        )
    }

    #if DEBUG
    private static func buildSnapshotProfiled(
        tasks: [TaskMirror],
        events: [CalendarEventMirror]?,
        scheduledEvents: [CalendarEventMirror]?,
        referenceDate: Date,
        calendar: Calendar
    ) -> (TodaySnapshot, TodaySnapshotBuildProfile) {
        let totalStart = DispatchTime.now().uptimeNanoseconds
        let dueStart = totalStart
        let dueTasks = tasks.filter { task in
            guard !task.isCompleted, !task.isDeleted, let dueDate = task.dueDate else {
                return false
            }
            return calendar.isDate(dueDate, inSameDayAs: referenceDate)
        }

        let dueEnd = DispatchTime.now().uptimeNanoseconds
        let overdueStart = dueEnd
        let referenceStart = calendar.startOfDay(for: referenceDate)
        let overdueCount = tasks.filter { task in
            guard !task.isCompleted, !task.isDeleted, let dueDate = task.dueDate else {
                return false
            }
            return dueDate < referenceStart
        }.count

        let overdueEnd = DispatchTime.now().uptimeNanoseconds
        let eventFilterStart = overdueEnd
        let resolvedEvents = scheduledEvents ?? events?.filter { event in
            event.status != .cancelled && calendar.isDate(event.startDate, inSameDayAs: referenceDate)
        } ?? []

        let eventFilterEnd = DispatchTime.now().uptimeNanoseconds
        let eventSortStart = eventFilterEnd
        let sortedEvents = resolvedEvents.sorted { lhs, rhs in
            lhs.startDate < rhs.startDate
        }

        let eventSortEnd = DispatchTime.now().uptimeNanoseconds
        let snapshot = TodaySnapshot(
            date: referenceDate,
            dueTasks: dueTasks,
            scheduledEvents: sortedEvents,
            overdueCount: overdueCount
        )
        let profile = TodaySnapshotBuildProfile(
            dueTaskMilliseconds: TodaySnapshotBuildProfile.milliseconds(from: dueStart, to: dueEnd),
            overdueTaskMilliseconds: TodaySnapshotBuildProfile.milliseconds(from: overdueStart, to: overdueEnd),
            eventFilterMilliseconds: TodaySnapshotBuildProfile.milliseconds(from: eventFilterStart, to: eventFilterEnd),
            eventSortMilliseconds: TodaySnapshotBuildProfile.milliseconds(from: eventSortStart, to: eventSortEnd),
            totalMilliseconds: TodaySnapshotBuildProfile.milliseconds(from: totalStart, to: eventSortEnd),
            inputEventCount: events?.count ?? scheduledEvents?.count ?? 0,
            scheduledEventCount: sortedEvents.count
        )
        return (snapshot, profile)
    }
    #endif
}

struct CalendarSnapshot: Equatable, Sendable {
    var selectedCalendars: [CalendarListMirror]
    var selectedCalendarIDs: Set<CalendarListMirror.ID>
    var calendarColorHexByID: [CalendarListMirror.ID: String]
    var eventCountsByCalendarID: [CalendarListMirror.ID: Int]
    var eventCountsByColorID: [String: Int]
    var eventCountsByTagName: [String: Int]

    static let empty = CalendarSnapshot(
        selectedCalendars: [],
        selectedCalendarIDs: [],
        calendarColorHexByID: [:],
        eventCountsByCalendarID: [:],
        eventCountsByColorID: [:],
        eventCountsByTagName: [:]
    )

    static func build(
        calendars: [CalendarListMirror],
        events: [CalendarEventMirror],
        settings: AppSettings
    ) -> CalendarSnapshot {
        buildSnapshot(calendars: calendars, events: events, settings: settings).snapshot
    }

    #if DEBUG
    static func buildProfiled(
        calendars: [CalendarListMirror],
        events: [CalendarEventMirror],
        settings: AppSettings
    ) -> (CalendarSnapshot, CalendarSnapshotBuildProfile) {
        let result = buildSnapshot(calendars: calendars, events: events, settings: settings)
        return (result.snapshot, result.profile)
    }
    #endif

    private static func buildSnapshot(
        calendars: [CalendarListMirror],
        events: [CalendarEventMirror],
        settings: AppSettings
    ) -> (snapshot: CalendarSnapshot, profile: CalendarSnapshotBuildProfile) {
        #if DEBUG
        let totalStart = DispatchTime.now().uptimeNanoseconds
        let setupStart = totalStart
        #endif
        let selectedCalendars = calendars.filter(\.isSelected)
        let selectedIDs = Set(selectedCalendars.map(\.id))
        var eventCountsByCalendarID: [CalendarListMirror.ID: Int] = [:]
        var eventCountsByColorID: [String: Int] = [:]
        var eventCountsByTagName: [String: Int] = [:]
        let colorTagIndex = colorTagIndex(from: settings.colorTagBindings)
        let boundTagsByColorID = boundTagsByColorID(from: colorTagIndex)
        #if DEBUG
        let setupEnd = DispatchTime.now().uptimeNanoseconds
        let eventScanStart = setupEnd
        var selectedEventCount = 0
        var visibleEventCount = 0
        var literalTaggedEventCount = 0
        var boundTaggedEventCount = 0
        var eventVisibilityNanoseconds: UInt64 = 0
        var calendarCountNanoseconds: UInt64 = 0
        var colorAggregationNanoseconds: UInt64 = 0
        var literalTagExtractionNanoseconds: UInt64 = 0
        var literalTagExtractionProfile = LiteralTagExtractionProfile()
        var boundTagAggregationNanoseconds: UInt64 = 0
        var tagCountNanoseconds: UInt64 = 0
        #endif

        for event in events where selectedIDs.contains(event.calendarID) {
            #if DEBUG
            selectedEventCount += 1
            let visibilityStart = DispatchTime.now().uptimeNanoseconds
            #endif
            let isVisibleEvent = settings.showCompletedItemsInCalendar || event.status != .cancelled
            #if DEBUG
            eventVisibilityNanoseconds += DispatchTime.now().uptimeNanoseconds - visibilityStart
            #endif
            guard isVisibleEvent else {
                continue
            }
            #if DEBUG
            visibleEventCount += 1
            let calendarCountStart = DispatchTime.now().uptimeNanoseconds
            #endif
            eventCountsByCalendarID[event.calendarID, default: 0] += 1
            #if DEBUG
            calendarCountNanoseconds += DispatchTime.now().uptimeNanoseconds - calendarCountStart
            let colorAggregationStart = DispatchTime.now().uptimeNanoseconds
            #endif
            let colorID = eventColorID(for: event)
            eventCountsByColorID[colorID, default: 0] += 1
            #if DEBUG
            colorAggregationNanoseconds += DispatchTime.now().uptimeNanoseconds - colorAggregationStart
            let boundTags = boundTagsByColorID[colorID] ?? []
            if boundTags.isEmpty == false { boundTaggedEventCount += 1 }
            let hasLiteralTagMarker = hasPotentialLiteralTag(in: event)
            guard hasLiteralTagMarker || boundTags.isEmpty == false else { continue }
            let literalTagExtractionStart = DispatchTime.now().uptimeNanoseconds
            #else
            let boundTags = boundTagsByColorID[colorID] ?? []
            guard hasPotentialLiteralTag(in: event) || boundTags.isEmpty == false else { continue }
            #endif

            #if DEBUG
            var eventTagNames = literalTagNames(in: event, profile: &literalTagExtractionProfile)
            #else
            var eventTagNames = literalTagNames(in: event)
            #endif
            #if DEBUG
            literalTagExtractionNanoseconds += DispatchTime.now().uptimeNanoseconds - literalTagExtractionStart
            if eventTagNames.isEmpty == false { literalTaggedEventCount += 1 }
            let boundTagAggregationStart = DispatchTime.now().uptimeNanoseconds
            #endif
            eventTagNames.formUnion(boundTags)
            #if DEBUG
            boundTagAggregationNanoseconds += DispatchTime.now().uptimeNanoseconds - boundTagAggregationStart
            let tagCountStart = DispatchTime.now().uptimeNanoseconds
            #endif
            for tag in eventTagNames {
                eventCountsByTagName[tag, default: 0] += 1
            }
            #if DEBUG
            tagCountNanoseconds += DispatchTime.now().uptimeNanoseconds - tagCountStart
            #endif
        }
        #if DEBUG
        let eventScanEnd = DispatchTime.now().uptimeNanoseconds
        let colorMapStart = eventScanEnd
        #endif

        let calendarColorHexByID = Dictionary(uniqueKeysWithValues: calendars.map { ($0.id, $0.colorHex) })
        #if DEBUG
        let colorMapEnd = DispatchTime.now().uptimeNanoseconds
        #endif
        let snapshot = CalendarSnapshot(
            selectedCalendars: selectedCalendars,
            selectedCalendarIDs: selectedIDs,
            calendarColorHexByID: calendarColorHexByID,
            eventCountsByCalendarID: eventCountsByCalendarID,
            eventCountsByColorID: eventCountsByColorID,
            eventCountsByTagName: eventCountsByTagName
        )
        #if DEBUG
        let profile = CalendarSnapshotBuildProfile(
            setupMilliseconds: CalendarSnapshotBuildProfile.milliseconds(from: setupStart, to: setupEnd),
            eventScanMilliseconds: CalendarSnapshotBuildProfile.milliseconds(from: eventScanStart, to: eventScanEnd),
            eventVisibilityMilliseconds: Double(eventVisibilityNanoseconds) / 1_000_000,
            calendarCountMilliseconds: Double(calendarCountNanoseconds) / 1_000_000,
            colorAggregationMilliseconds: Double(colorAggregationNanoseconds) / 1_000_000,
            literalTagExtractionMilliseconds: Double(literalTagExtractionNanoseconds) / 1_000_000,
            literalSummaryScanMilliseconds: Double(literalTagExtractionProfile.summaryScanNanoseconds) / 1_000_000,
            literalDetailsScanMilliseconds: Double(literalTagExtractionProfile.detailsScanNanoseconds) / 1_000_000,
            literalLocationScanMilliseconds: Double(literalTagExtractionProfile.locationScanNanoseconds) / 1_000_000,
            literalRegexMatchingMilliseconds: Double(literalTagExtractionProfile.regexMatchingNanoseconds) / 1_000_000,
            literalDeduplicationMilliseconds: Double(literalTagExtractionProfile.deduplicationNanoseconds) / 1_000_000,
            boundTagAggregationMilliseconds: Double(boundTagAggregationNanoseconds) / 1_000_000,
            tagCountMilliseconds: Double(tagCountNanoseconds) / 1_000_000,
            colorMapMilliseconds: CalendarSnapshotBuildProfile.milliseconds(from: colorMapStart, to: colorMapEnd),
            totalMilliseconds: CalendarSnapshotBuildProfile.milliseconds(from: totalStart, to: DispatchTime.now().uptimeNanoseconds),
            inputEventCount: events.count,
            selectedEventCount: selectedEventCount,
            visibleEventCount: visibleEventCount,
            literalTaggedEventCount: literalTaggedEventCount,
            boundTaggedEventCount: boundTaggedEventCount
        )
        #else
        let profile = CalendarSnapshotBuildProfile(
            setupMilliseconds: 0,
            eventScanMilliseconds: 0,
            eventVisibilityMilliseconds: 0,
            calendarCountMilliseconds: 0,
            colorAggregationMilliseconds: 0,
            literalTagExtractionMilliseconds: 0,
            literalSummaryScanMilliseconds: 0,
            literalDetailsScanMilliseconds: 0,
            literalLocationScanMilliseconds: 0,
            literalRegexMatchingMilliseconds: 0,
            literalDeduplicationMilliseconds: 0,
            boundTagAggregationMilliseconds: 0,
            tagCountMilliseconds: 0,
            colorMapMilliseconds: 0,
            totalMilliseconds: 0,
            inputEventCount: 0,
            selectedEventCount: 0,
            visibleEventCount: 0,
            literalTaggedEventCount: 0,
            boundTaggedEventCount: 0
        )
        #endif
        return (snapshot, profile)
    }

    private static func eventColorID(for event: CalendarEventMirror) -> String {
        CalendarEventColor.from(colorId: event.colorId).rawValue
    }

    private static func hasPotentialLiteralTag(in event: CalendarEventMirror) -> Bool {
        event.summary.contains("#") || event.details.contains("#") || event.location.contains("#")
    }

    private struct LiteralTagExtractionProfile {
        var summaryScanNanoseconds: UInt64 = 0
        var detailsScanNanoseconds: UInt64 = 0
        var locationScanNanoseconds: UInt64 = 0
        var regexMatchingNanoseconds: UInt64 = 0
        var deduplicationNanoseconds: UInt64 = 0
    }

    private enum LiteralTagField {
        case summary
        case details
        case location
    }

    private static func literalTagNames(
        in event: CalendarEventMirror,
        profile: UnsafeMutablePointer<LiteralTagExtractionProfile>? = nil
    ) -> Set<String> {
        guard hasPotentialLiteralTag(in: event) else { return [] }
        let setStart = profile.map { _ in DispatchTime.now().uptimeNanoseconds } ?? 0
        var tags: Set<String> = []
        tags.reserveCapacity(2)
        if let profile {
            profile.pointee.deduplicationNanoseconds += DispatchTime.now().uptimeNanoseconds - setStart
        }
        insertLiteralTagNames(in: event.summary, field: .summary, into: &tags, profile: profile)
        insertLiteralTagNames(in: event.details, field: .details, into: &tags, profile: profile)
        insertLiteralTagNames(in: event.location, field: .location, into: &tags, profile: profile)
        return tags
    }

    private static func insertLiteralTagNames(
        in text: String,
        field: LiteralTagField,
        into tags: inout Set<String>,
        profile: UnsafeMutablePointer<LiteralTagExtractionProfile>? = nil
    ) {
        guard text.contains("#") else { return }
        let scanStart = profile.map { _ in DispatchTime.now().uptimeNanoseconds } ?? 0
        let extractedTags: [String]
        if let profile {
            let result = TagExtractor.tagsProfiled(in: text)
            extractedTags = result.tags
            profile.pointee.regexMatchingNanoseconds += result.profile.regexNanoseconds
        } else {
            extractedTags = TagExtractor.tags(in: text)
        }
        let scanEnd = profile.map { _ in DispatchTime.now().uptimeNanoseconds } ?? 0
        if let profile {
            switch field {
            case .summary:
                profile.pointee.summaryScanNanoseconds += scanEnd - scanStart
            case .details:
                profile.pointee.detailsScanNanoseconds += scanEnd - scanStart
            case .location:
                profile.pointee.locationScanNanoseconds += scanEnd - scanStart
            }
        }
        for tag in extractedTags {
            let deduplicationStart = profile.map { _ in DispatchTime.now().uptimeNanoseconds } ?? 0
            let normalized = normalizedTagName(tag)
            guard normalized.isEmpty == false else { continue }
            tags.insert(normalized)
            if let profile {
                profile.pointee.deduplicationNanoseconds += DispatchTime.now().uptimeNanoseconds - deduplicationStart
            }
        }
    }

    private static func colorTagIndex(from bindings: [String: String]) -> [String: String] {
        bindings.reduce(into: [String: String]()) { result, entry in
            let colorID = normalizedColorID(entry.key)
            let tagName = normalizedTagName(entry.value)
            guard colorID.isEmpty == false, tagName.isEmpty == false else { return }
            result[tagName] = colorID
        }
    }

    private static func boundTagsByColorID(from colorTagIndex: [String: String]) -> [String: [String]] {
        colorTagIndex.reduce(into: [String: [String]]()) { result, entry in
            result[entry.value, default: []].append(entry.key)
        }
    }

    private static func normalizedTagName(_ value: String) -> String {
        var trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        while trimmed.first == "#" {
            trimmed.removeFirst()
        }
        return trimmed
            .folding(options: [.caseInsensitive, .diacriticInsensitive], locale: .current)
            .lowercased()
    }

    private static func normalizedColorID(_ value: String) -> String {
        CalendarEventColor(rawValue: value)?.rawValue ?? CalendarEventColor.defaultColor.rawValue
    }
}

struct TaskListSectionSnapshot: Identifiable, Equatable, Sendable {
    var id: TaskListMirror.ID { taskList.id }
    var taskList: TaskListMirror
    var tasks: [TaskMirror]

    static func build(taskLists: [TaskListMirror], tasks: [TaskMirror]) -> [TaskListSectionSnapshot] {
        let visibleTasksByList = Dictionary(grouping: tasks.filter { !$0.isDeleted }) { task in
            task.taskListID
        }

        return taskLists.map { taskList in
            TaskListSectionSnapshot(
                taskList: taskList,
                tasks: visibleTasksByList[taskList.id, default: []]
            )
        }
    }
}

struct TaskBoardSnapshot: Equatable, Sendable {
    var datedTasks: [TaskMirror]
    var undatedTasks: [TaskMirror]

    static let empty = TaskBoardSnapshot(datedTasks: [], undatedTasks: [])

    static func build(
        tasks: [TaskMirror],
        tasksTabVisibleListIDs: Set<TaskListMirror.ID>,
        notesTabVisibleListIDs: Set<TaskListMirror.ID>,
        settings: AppSettings,
        referenceDate: Date,
        calendar: Calendar = .current
    ) -> TaskBoardSnapshot {
        var datedTasks: [TaskMirror] = []
        var undatedTasks: [TaskMirror] = []

        for task in tasks where task.isDeleted == false {
            if task.dueDate != nil {
                guard tasksTabVisibleListIDs.contains(task.taskListID) else { continue }
                guard settings.shouldHideOverdueTask(task, now: referenceDate, calendar: calendar) == false else { continue }
                datedTasks.append(task)
            }
            if task.appearsInNotesSurface {
                guard notesTabVisibleListIDs.contains(task.taskListID) else { continue }
                undatedTasks.append(task)
            }
        }

        return TaskBoardSnapshot(datedTasks: datedTasks, undatedTasks: undatedTasks)
    }
}

struct AccountScopedSettings: Hashable, Codable, Sendable {
    var syncMode: SyncMode
    var cloudSyncTargets: Set<CloudSyncTarget>
    var selectedCalendarIDs: Set<CalendarListMirror.ID>
    var selectedTaskListIDs: Set<TaskListMirror.ID>
    var hasConfiguredCalendarSelection: Bool
    var hasConfiguredTaskListSelection: Bool
    var eventRetentionDaysBack: Int
    var completedTaskRetentionDaysBack: Int
    var collapsedTaskListIDs: Set<TaskListMirror.ID>
    var tasksTabSelectedListIDs: Set<TaskListMirror.ID>
    var hasConfiguredTasksTabSelection: Bool
    var notesTabSelectedListIDs: Set<TaskListMirror.ID>
    var hasConfiguredNotesTabSelection: Bool
    var colorTagAutoApplyEnabled: Bool
    var colorTagBindings: [String: String]
    var colorTagMatchPolicy: ColorTagMatchPolicy
    var pastEventBehavior: PastEventBehavior
    var pastEventDeleteThresholdDays: Int
    var allowDeletingAttendeeEvents: Bool
    var showCompletedItemsInCalendar: Bool
    var overdueTaskBehavior: OverdueTaskBehavior
    var completedTaskBehavior: CompletedTaskBehavior
    var completedTaskDeleteThresholdDays: Int
    var hasAckedEventDeletion: Bool
    var hasAckedAttendeeDeletion: Bool
    var hasAckedTaskDeletion: Bool
    var dismissedDuplicateGroups: Set<String>

    init(settings: AppSettings) {
        syncMode = settings.syncMode
        cloudSyncTargets = settings.cloudSyncTargets
        selectedCalendarIDs = settings.selectedCalendarIDs
        selectedTaskListIDs = settings.selectedTaskListIDs
        hasConfiguredCalendarSelection = settings.hasConfiguredCalendarSelection
        hasConfiguredTaskListSelection = settings.hasConfiguredTaskListSelection
        eventRetentionDaysBack = settings.eventRetentionDaysBack
        completedTaskRetentionDaysBack = settings.completedTaskRetentionDaysBack
        collapsedTaskListIDs = settings.collapsedTaskListIDs
        tasksTabSelectedListIDs = settings.tasksTabSelectedListIDs
        hasConfiguredTasksTabSelection = settings.hasConfiguredTasksTabSelection
        notesTabSelectedListIDs = settings.notesTabSelectedListIDs
        hasConfiguredNotesTabSelection = settings.hasConfiguredNotesTabSelection
        colorTagAutoApplyEnabled = settings.colorTagAutoApplyEnabled
        colorTagBindings = settings.colorTagBindings
        colorTagMatchPolicy = settings.colorTagMatchPolicy
        pastEventBehavior = settings.pastEventBehavior
        pastEventDeleteThresholdDays = settings.pastEventDeleteThresholdDays
        allowDeletingAttendeeEvents = settings.allowDeletingAttendeeEvents
        showCompletedItemsInCalendar = settings.showCompletedItemsInCalendar
        overdueTaskBehavior = settings.overdueTaskBehavior
        completedTaskBehavior = settings.completedTaskBehavior
        completedTaskDeleteThresholdDays = settings.completedTaskDeleteThresholdDays
        hasAckedEventDeletion = settings.hasAckedEventDeletion
        hasAckedAttendeeDeletion = settings.hasAckedAttendeeDeletion
        hasAckedTaskDeletion = settings.hasAckedTaskDeletion
        dismissedDuplicateGroups = settings.dismissedDuplicateGroups
    }

    func applying(to settings: AppSettings) -> AppSettings {
        var settings = settings
        settings.syncMode = syncMode
        settings.cloudSyncTargets = cloudSyncTargets
        settings.selectedCalendarIDs = selectedCalendarIDs
        settings.selectedTaskListIDs = selectedTaskListIDs
        settings.hasConfiguredCalendarSelection = hasConfiguredCalendarSelection
        settings.hasConfiguredTaskListSelection = hasConfiguredTaskListSelection
        settings.eventRetentionDaysBack = eventRetentionDaysBack
        settings.completedTaskRetentionDaysBack = completedTaskRetentionDaysBack
        settings.collapsedTaskListIDs = collapsedTaskListIDs
        settings.tasksTabSelectedListIDs = tasksTabSelectedListIDs
        settings.hasConfiguredTasksTabSelection = hasConfiguredTasksTabSelection
        settings.notesTabSelectedListIDs = notesTabSelectedListIDs
        settings.hasConfiguredNotesTabSelection = hasConfiguredNotesTabSelection
        settings.colorTagAutoApplyEnabled = colorTagAutoApplyEnabled
        settings.colorTagBindings = colorTagBindings
        settings.colorTagMatchPolicy = colorTagMatchPolicy
        settings.pastEventBehavior = pastEventBehavior
        settings.pastEventDeleteThresholdDays = pastEventDeleteThresholdDays
        settings.allowDeletingAttendeeEvents = allowDeletingAttendeeEvents
        settings.showCompletedItemsInCalendar = showCompletedItemsInCalendar
        settings.overdueTaskBehavior = overdueTaskBehavior
        settings.completedTaskBehavior = completedTaskBehavior
        settings.completedTaskDeleteThresholdDays = completedTaskDeleteThresholdDays
        settings.hasAckedEventDeletion = hasAckedEventDeletion
        settings.hasAckedAttendeeDeletion = hasAckedAttendeeDeletion
        settings.hasAckedTaskDeletion = hasAckedTaskDeletion
        settings.dismissedDuplicateGroups = dismissedDuplicateGroups
        return settings
    }
}

struct AccountWorkspaceSnapshot: Identifiable, Equatable, Codable, Sendable {
    var id: GoogleAccount.ID { accountID }
    var accountID: GoogleAccount.ID
    var taskLists: [TaskListMirror]
    var tasks: [TaskMirror]
    var calendars: [CalendarListMirror]
    var events: [CalendarEventMirror]
    var settings: AccountScopedSettings
    var syncCheckpoints: [SyncCheckpoint]
    var pendingMutations: [PendingMutation]

    init(
        accountID: GoogleAccount.ID,
        taskLists: [TaskListMirror],
        tasks: [TaskMirror],
        calendars: [CalendarListMirror],
        events: [CalendarEventMirror],
        settings: AppSettings,
        syncCheckpoints: [SyncCheckpoint],
        pendingMutations: [PendingMutation]
    ) {
        self.accountID = accountID
        self.taskLists = taskLists
        self.tasks = tasks
        self.calendars = calendars
        self.events = events
        self.settings = AccountScopedSettings(settings: settings)
        self.syncCheckpoints = syncCheckpoints.map { $0.accountStamped(accountID: accountID) }
        self.pendingMutations = pendingMutations.map { $0.accountStamped(accountID: accountID) }
    }

    func effectiveSettings(mergedWith globalSettings: AppSettings) -> AppSettings {
        settings.applying(to: globalSettings)
    }

    func withoutEvents() -> AccountWorkspaceSnapshot {
        var copy = self
        copy.events = []
        return copy
    }

    func accountStamped() -> AccountWorkspaceSnapshot {
        var copy = self
        copy.syncCheckpoints = syncCheckpoints.map { $0.accountStamped(accountID: accountID) }
        copy.pendingMutations = pendingMutations.map { $0.accountStamped(accountID: accountID) }
        return copy
    }
}

extension SyncCheckpoint {
    func accountStamped(accountID: GoogleAccount.ID) -> SyncCheckpoint {
        guard self.accountID != accountID else { return self }
        var copy = self
        copy.accountID = accountID
        return copy
    }
}

extension PendingMutation {
    func accountStamped(accountID: GoogleAccount.ID) -> PendingMutation {
        guard self.accountID != accountID else { return self }
        var copy = self
        copy.accountID = accountID
        return copy
    }
}

struct CachedAppState: Codable, Sendable {
    // Bumped whenever the cache layout changes in a way that can't be
    // decoded field-by-field. CacheSchemaMigrator routes older decoded
    // payloads through migration shims before the decoder returns.
    static let currentSchemaVersion: Int = 3

    var schemaVersion: Int
    // Legacy active-account field. Kept so older caches decode cleanly and
    // existing AppModel call sites can continue to read the current account
    // while the multi-account backend is introduced in slices.
    var account: GoogleAccount?
    var accounts: [GoogleAccount]
    var activeAccountID: GoogleAccount.ID?
    var accountWorkspaces: [AccountWorkspaceSnapshot]
    var taskLists: [TaskListMirror]
    var tasks: [TaskMirror]
    var calendars: [CalendarListMirror]
    var events: [CalendarEventMirror]
    var settings: AppSettings
    var syncCheckpoints: [SyncCheckpoint]
    var pendingMutations: [PendingMutation]

    init(
        account: GoogleAccount?,
        accounts: [GoogleAccount] = [],
        activeAccountID: GoogleAccount.ID? = nil,
        accountWorkspaces: [AccountWorkspaceSnapshot] = [],
        taskLists: [TaskListMirror],
        tasks: [TaskMirror],
        calendars: [CalendarListMirror],
        events: [CalendarEventMirror],
        settings: AppSettings,
        syncCheckpoints: [SyncCheckpoint] = [],
        pendingMutations: [PendingMutation] = [],
        schemaVersion: Int = CachedAppState.currentSchemaVersion
    ) {
        self.schemaVersion = schemaVersion
        let normalizedAccounts = Self.normalizedAccounts(primary: account, accounts: accounts)
        let resolvedActiveAccountID = Self.resolvedActiveAccountID(
            primary: account,
            accounts: normalizedAccounts,
            requestedActiveAccountID: activeAccountID
        )
        self.account = Self.account(id: resolvedActiveAccountID, in: normalizedAccounts) ?? account
        self.accounts = normalizedAccounts
        self.activeAccountID = resolvedActiveAccountID
        let activeSyncCheckpoints = Self.activeScopedSyncCheckpoints(
            syncCheckpoints,
            activeAccountID: resolvedActiveAccountID
        )
        let activePendingMutations = Self.activeScopedPendingMutations(
            pendingMutations,
            activeAccountID: resolvedActiveAccountID
        )
        let metadataWorkspaces = Self.metadataOnlyWorkspaces(
            from: accountWorkspaces,
            accountIDs: normalizedAccounts.map(\.id),
            activeAccountID: resolvedActiveAccountID,
            settings: settings,
            syncCheckpoints: syncCheckpoints,
            pendingMutations: pendingMutations
        )
        let activeWorkspace = resolvedActiveAccountID.map {
            AccountWorkspaceSnapshot(
                accountID: $0,
                taskLists: taskLists,
                tasks: tasks,
                calendars: calendars,
                events: events,
                settings: settings,
                syncCheckpoints: activeSyncCheckpoints,
                pendingMutations: activePendingMutations
            )
        }
        self.accountWorkspaces = Self.normalizedWorkspaces(
            accountWorkspaces + metadataWorkspaces,
            activeWorkspace: activeWorkspace,
            validAccountIDs: Set(normalizedAccounts.map(\.id))
        )
        let resolvedWorkspace = Self.workspace(id: resolvedActiveAccountID, in: self.accountWorkspaces)
        self.taskLists = resolvedWorkspace?.taskLists ?? taskLists
        self.tasks = resolvedWorkspace?.tasks ?? tasks
        self.calendars = resolvedWorkspace?.calendars ?? calendars
        self.events = resolvedWorkspace?.events ?? events
        self.settings = resolvedWorkspace?.effectiveSettings(mergedWith: settings) ?? settings
        self.syncCheckpoints = resolvedWorkspace?.syncCheckpoints ?? activeSyncCheckpoints.map { mutationCheckpoint in
            guard let resolvedActiveAccountID else { return mutationCheckpoint }
            return mutationCheckpoint.accountStamped(accountID: resolvedActiveAccountID)
        }
        self.pendingMutations = resolvedWorkspace?.pendingMutations ?? activePendingMutations.map { mutation in
            guard let resolvedActiveAccountID else { return mutation }
            return mutation.accountStamped(accountID: resolvedActiveAccountID)
        }
    }

    enum CodingKeys: String, CodingKey {
        case schemaVersion
        case account
        case accounts
        case activeAccountID
        case accountWorkspaces
        case taskLists
        case tasks
        case calendars
        case events
        case settings
        case syncCheckpoints
        case pendingMutations
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        // Absent schemaVersion means the cache was written before versioning
        // was introduced — treat as v0 so future migrations can tell.
        schemaVersion = try container.decodeIfPresent(Int.self, forKey: .schemaVersion) ?? 0
        let decodedAccount = try container.decodeIfPresent(GoogleAccount.self, forKey: .account)
        let decodedAccounts = try container.decodeIfPresent([GoogleAccount].self, forKey: .accounts) ?? []
        let decodedActiveAccountID = try container.decodeIfPresent(GoogleAccount.ID.self, forKey: .activeAccountID)
        accounts = Self.normalizedAccounts(primary: decodedAccount, accounts: decodedAccounts)
        activeAccountID = Self.resolvedActiveAccountID(
            primary: decodedAccount,
            accounts: accounts,
            requestedActiveAccountID: decodedActiveAccountID
        )
        account = Self.account(id: activeAccountID, in: accounts) ?? decodedAccount
        let decodedTaskLists = try container.decodeIfPresent([TaskListMirror].self, forKey: .taskLists) ?? []
        let decodedTasks = try container.decodeIfPresent([TaskMirror].self, forKey: .tasks) ?? []
        let decodedCalendars = try container.decodeIfPresent([CalendarListMirror].self, forKey: .calendars) ?? []
        let decodedEvents = try container.decodeIfPresent([CalendarEventMirror].self, forKey: .events) ?? []
        let decodedSettings = try container.decodeIfPresent(AppSettings.self, forKey: .settings) ?? .default
        let decodedSyncCheckpoints = try container.decodeIfPresent([SyncCheckpoint].self, forKey: .syncCheckpoints) ?? []
        let decodedPendingMutations = try container.decodeIfPresent([PendingMutation].self, forKey: .pendingMutations) ?? []
        let decodedWorkspaces = try container.decodeIfPresent([AccountWorkspaceSnapshot].self, forKey: .accountWorkspaces) ?? []
        let activeSyncCheckpoints = Self.activeScopedSyncCheckpoints(
            decodedSyncCheckpoints,
            activeAccountID: activeAccountID
        )
        let activePendingMutations = Self.activeScopedPendingMutations(
            decodedPendingMutations,
            activeAccountID: activeAccountID
        )
        let metadataWorkspaces = Self.metadataOnlyWorkspaces(
            from: decodedWorkspaces,
            accountIDs: accounts.map(\.id),
            activeAccountID: activeAccountID,
            settings: decodedSettings,
            syncCheckpoints: decodedSyncCheckpoints,
            pendingMutations: decodedPendingMutations
        )
        let activeWorkspace = activeAccountID.map {
            AccountWorkspaceSnapshot(
                accountID: $0,
                taskLists: decodedTaskLists,
                tasks: decodedTasks,
                calendars: decodedCalendars,
                events: decodedEvents,
                settings: decodedSettings,
                syncCheckpoints: activeSyncCheckpoints,
                pendingMutations: activePendingMutations
            )
        }
        accountWorkspaces = Self.normalizedWorkspaces(
            decodedWorkspaces + metadataWorkspaces,
            activeWorkspace: activeWorkspace,
            validAccountIDs: Set(accounts.map(\.id))
        )
        let activeWorkspaceState = Self.workspace(id: activeAccountID, in: accountWorkspaces)
        taskLists = activeWorkspaceState?.taskLists ?? decodedTaskLists
        tasks = activeWorkspaceState?.tasks ?? decodedTasks
        calendars = activeWorkspaceState?.calendars ?? decodedCalendars
        events = activeWorkspaceState?.events ?? decodedEvents
        settings = activeWorkspaceState?.effectiveSettings(mergedWith: decodedSettings) ?? decodedSettings
        syncCheckpoints = activeWorkspaceState?.syncCheckpoints ?? activeSyncCheckpoints
        pendingMutations = activeWorkspaceState?.pendingMutations ?? activePendingMutations
        // Stamp-forward so the in-memory model always carries the current
        // version; LocalCacheStore writes this back on the next save.
        if schemaVersion < CachedAppState.currentSchemaVersion {
            schemaVersion = CacheSchemaMigrator.migrateInPlace(
                from: schemaVersion,
                to: CachedAppState.currentSchemaVersion
            )
        }
    }

    private static func normalizedAccounts(primary: GoogleAccount?, accounts: [GoogleAccount]) -> [GoogleAccount] {
        var seen: Set<GoogleAccount.ID> = []
        var normalized: [GoogleAccount] = []

        if let primary {
            normalized.append(primary)
            seen.insert(primary.id)
        }

        for account in accounts where seen.insert(account.id).inserted {
            normalized.append(account)
        }

        return normalized
    }

    private static func resolvedActiveAccountID(
        primary: GoogleAccount?,
        accounts: [GoogleAccount],
        requestedActiveAccountID: GoogleAccount.ID?
    ) -> GoogleAccount.ID? {
        if let requestedActiveAccountID,
           accounts.contains(where: { $0.id == requestedActiveAccountID }) {
            return requestedActiveAccountID
        }
        if let primary {
            return primary.id
        }
        return nil
    }

    private static func account(id: GoogleAccount.ID?, in accounts: [GoogleAccount]) -> GoogleAccount? {
        guard let id else { return nil }
        return accounts.first { $0.id == id }
    }

    private static func workspace(
        id: GoogleAccount.ID?,
        in workspaces: [AccountWorkspaceSnapshot]
    ) -> AccountWorkspaceSnapshot? {
        guard let id else { return nil }
        return workspaces.first { $0.accountID == id }
    }

    private static func activeScopedSyncCheckpoints(
        _ syncCheckpoints: [SyncCheckpoint],
        activeAccountID: GoogleAccount.ID?
    ) -> [SyncCheckpoint] {
        guard let activeAccountID else { return syncCheckpoints }
        return syncCheckpoints.filter { $0.accountID == activeAccountID }
    }

    private static func activeScopedPendingMutations(
        _ pendingMutations: [PendingMutation],
        activeAccountID: GoogleAccount.ID?
    ) -> [PendingMutation] {
        guard let activeAccountID else { return pendingMutations }
        return pendingMutations.filter { $0.accountID == nil || $0.accountID == activeAccountID }
    }

    private static func metadataOnlyWorkspaces(
        from existingWorkspaces: [AccountWorkspaceSnapshot],
        accountIDs: [GoogleAccount.ID],
        activeAccountID: GoogleAccount.ID?,
        settings: AppSettings,
        syncCheckpoints: [SyncCheckpoint],
        pendingMutations: [PendingMutation]
    ) -> [AccountWorkspaceSnapshot] {
        let existingWorkspaceIDs = Set(existingWorkspaces.map(\.accountID))
        let checkpointsByAccountID = Dictionary(grouping: syncCheckpoints, by: \.accountID)
        var mutationsByAccountID: [GoogleAccount.ID: [PendingMutation]] = [:]
        for mutation in pendingMutations {
            guard let accountID = mutation.accountID else { continue }
            mutationsByAccountID[accountID, default: []].append(mutation)
        }

        return accountIDs.compactMap { accountID in
            guard accountID != activeAccountID,
                  existingWorkspaceIDs.contains(accountID) == false else {
                return nil
            }
            let checkpoints = checkpointsByAccountID[accountID] ?? []
            let mutations = mutationsByAccountID[accountID] ?? []
            guard checkpoints.isEmpty == false || mutations.isEmpty == false else {
                return nil
            }
            return AccountWorkspaceSnapshot(
                accountID: accountID,
                taskLists: [],
                tasks: [],
                calendars: [],
                events: [],
                settings: settings,
                syncCheckpoints: checkpoints,
                pendingMutations: mutations
            )
        }
    }

    private static func normalizedWorkspaces(
        _ workspaces: [AccountWorkspaceSnapshot],
        activeWorkspace: AccountWorkspaceSnapshot?,
        validAccountIDs: Set<GoogleAccount.ID>
    ) -> [AccountWorkspaceSnapshot] {
        var byID: [GoogleAccount.ID: AccountWorkspaceSnapshot] = [:]
        var order: [GoogleAccount.ID] = []
        for workspace in workspaces where validAccountIDs.isEmpty || validAccountIDs.contains(workspace.accountID) {
            if byID[workspace.accountID] == nil {
                order.append(workspace.accountID)
            }
            byID[workspace.accountID] = workspace.accountStamped()
        }
        if let activeWorkspace {
            if byID[activeWorkspace.accountID] == nil {
                order.insert(activeWorkspace.accountID, at: 0)
            }
            byID[activeWorkspace.accountID] = activeWorkspace.accountStamped()
        }
        return order.compactMap { byID[$0] }
    }

    func switchingActiveAccount(to accountID: GoogleAccount.ID) -> CachedAppState? {
        guard let targetAccount = accounts.first(where: { $0.id == accountID }),
              let targetWorkspace = accountWorkspaces.first(where: { $0.accountID == accountID }) else {
            return nil
        }
        return CachedAppState(
            account: targetAccount,
            accounts: accounts,
            activeAccountID: accountID,
            accountWorkspaces: accountWorkspaces,
            taskLists: targetWorkspace.taskLists,
            tasks: targetWorkspace.tasks,
            calendars: targetWorkspace.calendars,
            events: targetWorkspace.events,
            settings: targetWorkspace.effectiveSettings(mergedWith: settings),
            syncCheckpoints: targetWorkspace.syncCheckpoints,
            pendingMutations: targetWorkspace.pendingMutations,
            schemaVersion: schemaVersion
        )
    }

    func activating(account targetAccount: GoogleAccount) -> CachedAppState {
        var nextAccounts = accounts
        if let index = nextAccounts.firstIndex(where: { $0.id == targetAccount.id }) {
            nextAccounts[index] = targetAccount
        } else {
            nextAccounts.append(targetAccount)
        }

        let targetWorkspace = accountWorkspaces.first { $0.accountID == targetAccount.id }
            ?? AccountWorkspaceSnapshot(
                accountID: targetAccount.id,
                taskLists: [],
                tasks: [],
                calendars: [],
                events: [],
                settings: settings,
                syncCheckpoints: [],
                pendingMutations: []
            )
        let nextWorkspaces = accountWorkspaces.contains(where: { $0.accountID == targetAccount.id })
            ? accountWorkspaces
            : accountWorkspaces + [targetWorkspace]

        return CachedAppState(
            account: targetAccount,
            accounts: nextAccounts,
            activeAccountID: targetAccount.id,
            accountWorkspaces: nextWorkspaces,
            taskLists: targetWorkspace.taskLists,
            tasks: targetWorkspace.tasks,
            calendars: targetWorkspace.calendars,
            events: targetWorkspace.events,
            settings: targetWorkspace.effectiveSettings(mergedWith: settings),
            syncCheckpoints: targetWorkspace.syncCheckpoints,
            pendingMutations: targetWorkspace.pendingMutations,
            schemaVersion: schemaVersion
        )
    }

    func removingAccountWorkspace(
        accountID removedAccountID: GoogleAccount.ID,
        fallbackAccountID: GoogleAccount.ID?
    ) -> CachedAppState {
        let remainingAccounts = accounts.filter { $0.id != removedAccountID }
        let remainingWorkspaces = accountWorkspaces.filter { $0.accountID != removedAccountID }
        let fallbackAccount = fallbackAccountID.flatMap { id in remainingAccounts.first { $0.id == id } }
        let fallbackWorkspace = fallbackAccountID.flatMap { id in remainingWorkspaces.first { $0.accountID == id } }
        return CachedAppState(
            account: fallbackAccount,
            accounts: remainingAccounts,
            activeAccountID: fallbackAccount?.id,
            accountWorkspaces: remainingWorkspaces,
            taskLists: fallbackWorkspace?.taskLists ?? [],
            tasks: fallbackWorkspace?.tasks ?? [],
            calendars: fallbackWorkspace?.calendars ?? [],
            events: fallbackWorkspace?.events ?? [],
            settings: fallbackWorkspace?.effectiveSettings(mergedWith: settings) ?? settings,
            syncCheckpoints: fallbackWorkspace?.syncCheckpoints ?? [],
            pendingMutations: fallbackWorkspace?.pendingMutations ?? [],
            schemaVersion: schemaVersion
        )
    }

    func withoutEventPayloads() -> CachedAppState {
        var copy = self
        copy.events = []
        copy.accountWorkspaces = accountWorkspaces.map { $0.withoutEvents() }
        return copy
    }

    static let empty = CachedAppState(
        account: nil,
        taskLists: [],
        tasks: [],
        calendars: [],
        events: [],
        settings: .default
    )

    static var preview: CachedAppState {
        let now = Date()
        let calendar = Calendar.current
        let inbox = TaskListMirror(id: "tasks-inbox", title: "Inbox", updatedAt: now, etag: "preview-1")
        let focus = TaskListMirror(id: "tasks-focus", title: "Focused Work", updatedAt: now, etag: "preview-2")
        let primary = CalendarListMirror(
            id: "primary",
            summary: "Personal Calendar",
            colorHex: "#F66B3D",
            isSelected: true,
            accessRole: "owner",
            etag: "calendar-1"
        )
        let planning = CalendarListMirror(
            id: "planning",
            summary: "Deep Work",
            colorHex: "#1677FF",
            isSelected: true,
            accessRole: "owner",
            etag: "calendar-2"
        )

        return CachedAppState(
            account: .preview,
            taskLists: [inbox, focus],
            tasks: [
                TaskMirror(
                    id: "task-1",
                    taskListID: inbox.id,
                    parentID: nil,
                    title: "Draft Google Tasks sync contract",
                    notes: "Keep the app model close to Google Tasks fields.",
                    status: .needsAction,
                    dueDate: now,
                    completedAt: nil,
                    isDeleted: false,
                    isHidden: false,
                    position: "0001",
                    etag: "task-1-etag",
                    updatedAt: now
                ),
                TaskMirror(
                    id: "task-2",
                    taskListID: focus.id,
                    parentID: nil,
                    title: "Map Calendar time blocks",
                    notes: "Time-specific work belongs in Calendar, not Tasks.",
                    status: .needsAction,
                    dueDate: calendar.date(byAdding: .day, value: 1, to: now),
                    completedAt: nil,
                    isDeleted: false,
                    isHidden: false,
                    position: "0002",
                    etag: "task-2-etag",
                    updatedAt: now
                )
            ],
            calendars: [primary, planning],
            events: [
                CalendarEventMirror(
                    id: "event-1",
                    calendarID: planning.id,
                    summary: "Calendar adapter design",
                    details: "Store nextSyncToken per selected calendar.",
                    startDate: calendar.date(bySettingHour: 10, minute: 0, second: 0, of: now) ?? now,
                    endDate: calendar.date(bySettingHour: 11, minute: 0, second: 0, of: now) ?? now,
                    isAllDay: false,
                    status: .confirmed,
                    recurrence: [],
                    etag: "event-1-etag",
                    updatedAt: now
                ),
                CalendarEventMirror(
                    id: "event-2",
                    calendarID: primary.id,
                    summary: "Review DMG distribution path",
                    details: "Developer ID signing and notarization before public downloads.",
                    startDate: calendar.date(bySettingHour: 14, minute: 30, second: 0, of: now) ?? now,
                    endDate: calendar.date(bySettingHour: 15, minute: 0, second: 0, of: now) ?? now,
                    isAllDay: false,
                    status: .confirmed,
                    recurrence: [],
                    etag: "event-2-etag",
                    updatedAt: now
                )
            ],
            settings: AppSettings(
                syncMode: .balanced,
                selectedCalendarIDs: [primary.id, planning.id],
                selectedTaskListIDs: [inbox.id, focus.id],
                enableLocalNotifications: true,
                hasCompletedOnboarding: true
            )
        )
    }
}
