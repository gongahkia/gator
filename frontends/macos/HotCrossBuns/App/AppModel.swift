import Foundation
import Observation
import AppKit
import ImageIO
import UniformTypeIdentifiers

struct LoadingOverlayState: Equatable, Sendable {
    var message: String
}

struct LocalAttachmentDiagnostic: Identifiable, Equatable {
    var id: String
    var sourceKind: String
    var sourceTitle: String
    var sourceID: String
    var attachmentKind: LocalFileAttachment.Kind
    var displayName: String
    var url: URL
    var health: LocalAttachmentHealth
}

private enum LocalBackupEncryptionError: LocalizedError {
    case missingKeyOrSalt

    var errorDescription: String? {
        switch self {
        case .missingKeyOrSalt:
            "Cache encryption is enabled, but the local backup encryption key is unavailable."
        }
    }
}

private enum DerivedSnapshotRebuildStrategy {
    case synchronous
    case deferred
}

enum LocalAttachmentRepairError: LocalizedError {
    case replacementUnavailable(LocalAttachmentHealth)

    var errorDescription: String? {
        switch self {
        case .replacementUnavailable(let health):
            return "The selected replacement is \(health.repairLabel.lowercased()). Choose a readable file."
        }
    }
}

struct NearRealtimePollDiagnostics: Equatable, Sendable {
    var recordedAt: Date
    var delaySeconds: Double
    var attempt: Int
    var isNetworkConstrained: Bool
    var isLowPowerModeEnabled: Bool
    var isMainWindowFocused: Bool
}

struct LocalIntegrationDiagnostics: Equatable, Sendable {
    var notificationUpdatedAt: Date?
    var notificationDurationMilliseconds: Double?
    var spotlight: SpotlightIndexingSummary
}

struct PerformanceDiagnosticsSnapshot: Equatable, Sendable {
    var nearRealtime: NearRealtimePollDiagnostics?
    var integrations: LocalIntegrationDiagnostics
}

@MainActor
@Observable
final class CalendarStore {
    private(set) var account: GoogleAccount?
    private(set) var authState: AuthState = .authenticating
    private(set) var syncState: SyncState = .idle
    private(set) var isSyncPaused: Bool = false
    private(set) var lastMutationError: String?
    private(set) var calendars: [CalendarListMirror] = []
    private(set) var events: [CalendarEventMirror] = []
    private(set) var calendarSnapshot: CalendarSnapshot = .empty
    private(set) var visibleTaskListIDs: Set<TaskListMirror.ID> = []
    private(set) var settings: AppSettings = .default
    private(set) var calendarDisplayRevision: UInt64 = 0
    private(set) var isRebuildingDerivedSnapshots: Bool = false
    @ObservationIgnored private var eventsByDay: [TimeInterval: [CalendarEventMirror.ID]] = [:]
    @ObservationIgnored private var tasksByDueDate: [TimeInterval: [TaskMirror.ID]] = [:]
    @ObservationIgnored private var eventByIDSnapshot: [CalendarEventMirror.ID: CalendarEventMirror] = [:]
    @ObservationIgnored private var taskByIDSnapshot: [TaskMirror.ID: TaskMirror] = [:]
    @ObservationIgnored private var taskListTitleByID: [TaskListMirror.ID: String] = [:]
    @ObservationIgnored private var calendarDayDisplayRevisions: [TimeInterval: UInt64] = [:]
    @ObservationIgnored private var calendarPreparedSnapshotCache = CalendarPreparedSnapshotCache(limit: 12)
    @ObservationIgnored private var calendarRevisionKeyCache: [String: String] = [:]

    func publishSession(
        account: GoogleAccount?,
        authState: AuthState,
        syncState: SyncState,
        isSyncPaused: Bool,
        lastMutationError: String?
    ) {
        self.account = account
        self.authState = authState
        self.syncState = syncState
        self.isSyncPaused = isSyncPaused
        self.lastMutationError = lastMutationError
    }

    func publishSnapshots(
        calendars: [CalendarListMirror],
        events: [CalendarEventMirror],
        calendarSnapshot: CalendarSnapshot,
        visibleTaskListIDs: Set<TaskListMirror.ID>,
        settings: AppSettings,
        eventsByDay: [TimeInterval: [CalendarEventMirror.ID]],
        tasksByDueDate: [TimeInterval: [TaskMirror.ID]],
        eventByIDSnapshot: [CalendarEventMirror.ID: CalendarEventMirror],
        taskByIDSnapshot: [TaskMirror.ID: TaskMirror],
        taskListTitleByID: [TaskListMirror.ID: String],
        calendarDisplayRevision: UInt64,
        calendarDayDisplayRevisions: [TimeInterval: UInt64],
        isRebuildingDerivedSnapshots: Bool
    ) {
        self.calendars = calendars
        self.events = events
        self.calendarSnapshot = calendarSnapshot
        self.visibleTaskListIDs = visibleTaskListIDs
        self.settings = settings
        self.eventsByDay = eventsByDay
        self.tasksByDueDate = tasksByDueDate
        self.eventByIDSnapshot = eventByIDSnapshot
        self.taskByIDSnapshot = taskByIDSnapshot
        self.taskListTitleByID = taskListTitleByID
        self.calendarDisplayRevision = calendarDisplayRevision
        self.calendarDayDisplayRevisions = calendarDayDisplayRevisions
        calendarRevisionKeyCache.removeAll()
        self.isRebuildingDerivedSnapshots = isRebuildingDerivedSnapshots
    }

    func setRebuildingDerivedSnapshots(_ value: Bool) {
        isRebuildingDerivedSnapshots = value
    }

    func removePreparedSnapshots() {
        calendarPreparedSnapshotCache.removeAll()
    }

    func event(id: CalendarEventMirror.ID) -> CalendarEventMirror? {
        eventByIDSnapshot[id]
    }

    func task(id: TaskMirror.ID) -> TaskMirror? {
        taskByIDSnapshot[id]
    }

    func taskListTitle(for id: TaskListMirror.ID, fallback: String = "Unknown list") -> String {
        taskListTitleByID[id] ?? fallback
    }

    func eventIDs(on day: Date, calendar: Calendar = .current) -> [CalendarEventMirror.ID] {
        eventsByDay[calendar.startOfDay(for: day).timeIntervalSinceReferenceDate] ?? []
    }

    func taskIDs(dueOn day: Date, calendar: Calendar = .current) -> [TaskMirror.ID] {
        tasksByDueDate[calendar.startOfDay(for: day).timeIntervalSinceReferenceDate] ?? []
    }

    func calendarDisplayRevisionKey(for dates: [Date], calendar: Calendar = .current) -> String {
        let dayKeys = Set(dates.map { calendar.startOfDay(for: $0).timeIntervalSinceReferenceDate })
        return calendarDisplayRevisionKey(forDayKeys: dayKeys)
    }

    func calendarDisplayRevisionKey(from start: Date, to end: Date, calendar: Calendar = .current) -> String {
        let normalizedStart = calendar.startOfDay(for: start)
        let normalizedEnd = calendar.startOfDay(for: end)
        guard normalizedStart <= normalizedEnd else {
            return calendarDisplayRevisionKey(for: [start], calendar: calendar)
        }
        var dayKeys: [TimeInterval] = []
        var cursor = normalizedStart
        while cursor <= normalizedEnd {
            dayKeys.append(cursor.timeIntervalSinceReferenceDate)
            guard let next = calendar.date(byAdding: .day, value: 1, to: cursor) else { break }
            cursor = next
        }
        return calendarDisplayRevisionKey(forSortedDayKeys: dayKeys)
    }

    func calendarDisplayRevisionKey(forYearContaining date: Date, calendar: Calendar = .current) -> String {
        guard let yearStart = calendar.date(from: DateComponents(year: calendar.component(.year, from: date), month: 1, day: 1)),
              let nextYear = calendar.date(from: DateComponents(year: calendar.component(.year, from: date) + 1, month: 1, day: 1))
        else {
            return calendarDisplayRevisionKey(for: [date], calendar: calendar)
        }
        let yearEnd = calendar.date(byAdding: .day, value: -1, to: nextYear) ?? yearStart
        return calendarDisplayRevisionKey(from: yearStart, to: yearEnd, calendar: calendar)
    }

    func calendarVisibleRangeProjection(
        kind: CalendarVisibleRangeKind,
        anchorDate: Date,
        dayCount: Int? = nil,
        calendar: Calendar = .current
    ) -> CalendarVisibleRangeProjection {
        let range = calendarVisibleRange(kind: kind, anchorDate: anchorDate, dayCount: dayCount, calendar: calendar)
        return calendarVisibleRangeProjection(range: range)
    }

    func calendarVisibleRangeProjection(
        kind: CalendarVisibleRangeKind,
        start: Date,
        end: Date,
        calendar: Calendar = .current
    ) -> CalendarVisibleRangeProjection {
        calendarVisibleRangeProjection(
            range: CalendarVisibleRange(kind: kind, start: start, end: end, calendar: calendar)
        )
    }

    func calendarDisplayInput(
        key: PreparedSnapshotKey,
        kind: CalendarVisibleRangeKind,
        anchorDate: Date,
        dayCount: Int? = nil,
        eventViewFilter: CalendarEventViewFilter,
        searchQuery: String,
        referenceDate: Date = Date(),
        calendar: Calendar = .current
    ) -> CalendarDisplayInput {
        CalendarDisplayInput(
            key: key,
            anchorDate: anchorDate,
            selectedCalendarIDs: calendarSnapshot.selectedCalendarIDs,
            eventViewFilter: eventViewFilter,
            visibleTaskListIDs: visibleTaskListIDs,
            searchQuery: searchQuery,
            visibleRange: calendarVisibleRangeProjection(
                kind: kind,
                anchorDate: anchorDate,
                dayCount: dayCount,
                calendar: calendar
            ),
            calendarColorHexByID: calendarSnapshot.calendarColorHexByID,
            taskListTitleByID: taskListTitleByID,
            settings: settings,
            referenceDate: referenceDate,
            calendar: calendar
        )
    }

    func cachedCalendarDaySnapshot(for key: PreparedSnapshotKey) -> CalendarDayDisplaySnapshot? {
        calendarPreparedSnapshotCache.daySnapshot(for: key)
    }

    func storeCalendarDaySnapshot(_ snapshot: CalendarDayDisplaySnapshot) {
        calendarPreparedSnapshotCache.store(snapshot)
    }

    func cachedCalendarWeekSnapshot(for key: PreparedSnapshotKey) -> CalendarWeekDisplaySnapshot? {
        calendarPreparedSnapshotCache.weekSnapshot(for: key)
    }

    func storeCalendarWeekSnapshot(_ snapshot: CalendarWeekDisplaySnapshot) {
        calendarPreparedSnapshotCache.store(snapshot)
    }

    func cachedCalendarAgendaSnapshot(for key: PreparedSnapshotKey) -> CalendarAgendaDisplaySnapshot? {
        calendarPreparedSnapshotCache.agendaSnapshot(for: key)
    }

    func storeCalendarAgendaSnapshot(_ snapshot: CalendarAgendaDisplaySnapshot) {
        calendarPreparedSnapshotCache.store(snapshot)
    }

    func cachedCalendarYearSnapshot(for key: PreparedSnapshotKey) -> CalendarYearDisplaySnapshot? {
        calendarPreparedSnapshotCache.yearSnapshot(for: key)
    }

    func storeCalendarYearSnapshot(_ snapshot: CalendarYearDisplaySnapshot) {
        calendarPreparedSnapshotCache.store(snapshot)
    }

    private func calendarVisibleRangeProjection(range: CalendarVisibleRange) -> CalendarVisibleRangeProjection {
        let dayKeySet = Set(range.dayKeys)
        var visibleEventsByDay: [TimeInterval: [CalendarEventMirror.ID]] = [:]
        var visibleEventByID: [CalendarEventMirror.ID: CalendarEventMirror] = [:]
        var visibleTasksByDueDate: [TimeInterval: [TaskMirror.ID]] = [:]
        var visibleTaskByID: [TaskMirror.ID: TaskMirror] = [:]

        for key in range.dayKeys {
            let eventIDs = eventsByDay[key] ?? []
            if eventIDs.isEmpty == false {
                visibleEventsByDay[key] = eventIDs
            }
            for eventID in eventIDs {
                if let event = eventByIDSnapshot[eventID] {
                    visibleEventByID[eventID] = event
                }
            }

            let taskIDs = tasksByDueDate[key] ?? []
            if taskIDs.isEmpty == false {
                visibleTasksByDueDate[key] = taskIDs
            }
            for taskID in taskIDs {
                if let task = taskByIDSnapshot[taskID] {
                    visibleTaskByID[taskID] = task
                }
            }
        }

        return CalendarVisibleRangeProjection(
            range: range,
            revisionKey: calendarDisplayRevisionKey(forDayKeys: dayKeySet),
            eventsByDay: visibleEventsByDay,
            tasksByDueDate: visibleTasksByDueDate,
            eventByID: visibleEventByID,
            taskByID: visibleTaskByID,
            eventSearchTextByID: Dictionary(uniqueKeysWithValues: visibleEventByID.values.map { event in
                (event.id, "\(event.summary)\n\(event.details)\n\(event.location)")
            }),
            taskSearchTextByID: Dictionary(uniqueKeysWithValues: visibleTaskByID.values.map { task in
                (task.id, "\(task.title)\n\(task.notes)")
            })
        )
    }

    private func calendarVisibleRange(
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
            guard let first = days.first, let last = days.last else {
                return CalendarVisibleRange(kind: kind, start: anchorStart, end: anchorStart, calendar: calendar)
            }
            return CalendarVisibleRange(kind: kind, start: first, end: last, calendar: calendar)
        case .month:
            let cells = CalendarGridLayout.monthCells(for: anchorDate, calendar: calendar)
            guard let first = cells.first, let last = cells.last else {
                return CalendarVisibleRange(kind: kind, start: anchorStart, end: anchorStart, calendar: calendar)
            }
            return CalendarVisibleRange(kind: kind, start: first, end: last, calendar: calendar)
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

    private func calendarDisplayRevisionKey(forDayKeys dayKeys: Set<TimeInterval>) -> String {
        calendarDisplayRevisionKey(forSortedDayKeys: dayKeys.sorted())
    }

    private func calendarDisplayRevisionKey(forSortedDayKeys sortedDayKeys: [TimeInterval]) -> String {
        guard sortedDayKeys.isEmpty == false else {
            return "\(calendarDisplayRevision)"
        }
        let cacheKey = "\(calendarDisplayRevision)|\(sortedDayKeys.map { String(Int($0)) }.joined(separator: ","))"
        if let cached = calendarRevisionKeyCache[cacheKey] {
            return cached
        }
        let dayRevisionPart = sortedDayKeys
            .map { key in "\(Int(key)):\(calendarDayDisplayRevisions[key] ?? 0)" }
            .joined(separator: ",")
        let revisionKey = "\(calendarDisplayRevision)[\(dayRevisionPart)]"
        calendarRevisionKeyCache[cacheKey] = revisionKey
        return revisionKey
    }
}

struct TaskSurfaceSettings: Equatable, Sendable {
    var hasConfiguredTasksTabSelection: Bool
    var tasksTabSelectedListIDs: Set<TaskListMirror.ID>
    var hasConfiguredNotesTabSelection: Bool
    var notesTabSelectedListIDs: Set<TaskListMirror.ID>
    var notesKanbanColumnMode: KanbanColumnMode

    static let `default` = TaskSurfaceSettings(settings: .default)

    init(settings: AppSettings) {
        self.hasConfiguredTasksTabSelection = settings.hasConfiguredTasksTabSelection
        self.tasksTabSelectedListIDs = settings.tasksTabSelectedListIDs
        self.hasConfiguredNotesTabSelection = settings.hasConfiguredNotesTabSelection
        self.notesTabSelectedListIDs = settings.notesTabSelectedListIDs
        self.notesKanbanColumnMode = settings.notesKanbanColumnMode
    }
}

struct ShellChromeSettings: Equatable, Sendable {
    var syncMode: SyncMode
    var appLanguage: AppLanguage
    var hasCompletedOnboarding: Bool
    var hasSeenFeatureTour: Bool
    var enableGlobalHotkey: Bool
    var globalHotkeyBinding: GlobalHotkeyBinding
    var uiLayoutScale: Double
    var uiTextSizePoints: Double
    var uiFontName: String?
    var colorSchemeID: String
    var customColorSchemes: [HCBCustomColorScheme]
    var appBackgroundTranslucencyEnabled: Bool
    var appBackgroundOpacity: Double
    var customBackgroundImagePath: String?
    var disableAnimations: Bool
    var performanceMode: AppPerformanceMode
    var shortcutOverrides: [String: HCBKeyBinding]
    var sidebarPlacement: NavigationSurfacePlacement
    var hiddenSidebarItems: Set<String>
    var cacheEncryptionEnabled: Bool
    var colorTagBindings: [String: String]

    static let `default` = ShellChromeSettings(settings: .default)

    init(settings: AppSettings) {
        syncMode = settings.syncMode
        appLanguage = settings.appLanguage
        hasCompletedOnboarding = settings.hasCompletedOnboarding
        hasSeenFeatureTour = settings.hasSeenFeatureTour
        enableGlobalHotkey = settings.enableGlobalHotkey
        globalHotkeyBinding = settings.globalHotkeyBinding
        uiLayoutScale = settings.uiLayoutScale
        uiTextSizePoints = settings.uiTextSizePoints
        uiFontName = settings.uiFontName
        colorSchemeID = settings.colorSchemeID
        customColorSchemes = settings.customColorSchemes
        appBackgroundTranslucencyEnabled = settings.appBackgroundTranslucencyEnabled
        appBackgroundOpacity = settings.appBackgroundOpacity
        customBackgroundImagePath = settings.customBackgroundImagePath
        disableAnimations = settings.disableAnimations
        performanceMode = settings.performanceMode
        shortcutOverrides = settings.shortcutOverrides
        sidebarPlacement = settings.sidebarPlacement
        hiddenSidebarItems = settings.hiddenSidebarItems
        cacheEncryptionEnabled = settings.cacheEncryptionEnabled
        colorTagBindings = settings.colorTagBindings
    }
}

@MainActor
@Observable
final class TaskStore {
    private(set) var account: GoogleAccount?
    private(set) var authState: AuthState = .authenticating
    private(set) var syncState: SyncState = .idle
    private(set) var isMutating: Bool = false
    private(set) var lastMutationError: String?
    private(set) var taskLists: [TaskListMirror] = []
    private(set) var tasks: [TaskMirror] = []
    private(set) var visibleTaskListIDs: Set<TaskListMirror.ID> = []
    private(set) var taskSections: [TaskListSectionSnapshot] = []
    private(set) var taskBoardSnapshot: TaskBoardSnapshot = .empty
    private(set) var openTaskCountForSidebar: Int = 0
    private(set) var datedOpenTaskCount: Int = 0
    private(set) var undatedOpenTaskCount: Int = 0
    private(set) var recentlyCompletedTaskID: TaskMirror.ID?
    private(set) var taskDisplayRevision: UInt64 = 0
    private(set) var notesDisplayRevision: UInt64 = 0
    private(set) var isRebuildingDerivedSnapshots: Bool = false
    private(set) var surfaceSettings: TaskSurfaceSettings = .default
    @ObservationIgnored private var taskByIDSnapshot: [TaskMirror.ID: TaskMirror] = [:]
    @ObservationIgnored private var taskListByIDSnapshot: [TaskListMirror.ID: TaskListMirror] = [:]
    @ObservationIgnored private var taskListTitleByID: [TaskListMirror.ID: String] = [:]
    @ObservationIgnored private var taskChildrenByParentID: [TaskMirror.ID: [TaskMirror.ID]] = [:]
    @ObservationIgnored private var duplicateIndex: DuplicateIndex = .empty

    var tasksTabVisibleListIDs: Set<TaskListMirror.ID> {
        surfaceSettings.hasConfiguredTasksTabSelection
            ? surfaceSettings.tasksTabSelectedListIDs
            : visibleTaskListIDs
    }

    var notesTabVisibleListIDs: Set<TaskListMirror.ID> {
        surfaceSettings.hasConfiguredNotesTabSelection
            ? surfaceSettings.notesTabSelectedListIDs
            : visibleTaskListIDs
    }

    var duplicateTaskIDs: Set<TaskMirror.ID> {
        Set(duplicateIndex.memberToGroup.keys)
    }

    func publishSession(
        account: GoogleAccount?,
        authState: AuthState,
        syncState: SyncState,
        isMutating: Bool,
        lastMutationError: String?
    ) {
        guard self.account != account
            || self.authState != authState
            || self.syncState != syncState
            || self.isMutating != isMutating
            || self.lastMutationError != lastMutationError
        else { return }

        self.account = account
        self.authState = authState
        self.syncState = syncState
        self.isMutating = isMutating
        self.lastMutationError = lastMutationError
    }

    func publish(
        taskLists: [TaskListMirror],
        tasks: [TaskMirror],
        visibleTaskListIDs: Set<TaskListMirror.ID>,
        taskSections: [TaskListSectionSnapshot],
        taskBoardSnapshot: TaskBoardSnapshot,
        openTaskCountForSidebar: Int,
        datedOpenTaskCount: Int,
        undatedOpenTaskCount: Int,
        recentlyCompletedTaskID: TaskMirror.ID?,
        taskDisplayRevision: UInt64,
        notesDisplayRevision: UInt64,
        surfaceSettings: TaskSurfaceSettings,
        taskByIDSnapshot: [TaskMirror.ID: TaskMirror],
        taskListTitleByID: [TaskListMirror.ID: String],
        taskChildrenByParentID: [TaskMirror.ID: [TaskMirror.ID]],
        duplicateIndex: DuplicateIndex,
        isRebuildingDerivedSnapshots: Bool
    ) {
        guard self.taskDisplayRevision != taskDisplayRevision
            || self.notesDisplayRevision != notesDisplayRevision
            || self.surfaceSettings != surfaceSettings
            || self.isRebuildingDerivedSnapshots != isRebuildingDerivedSnapshots
            || self.recentlyCompletedTaskID != recentlyCompletedTaskID
        else { return }

        self.taskLists = taskLists
        self.tasks = tasks
        self.visibleTaskListIDs = visibleTaskListIDs
        self.taskSections = taskSections
        self.taskBoardSnapshot = taskBoardSnapshot
        self.openTaskCountForSidebar = openTaskCountForSidebar
        self.datedOpenTaskCount = datedOpenTaskCount
        self.undatedOpenTaskCount = undatedOpenTaskCount
        self.recentlyCompletedTaskID = recentlyCompletedTaskID
        self.taskDisplayRevision = taskDisplayRevision
        self.notesDisplayRevision = notesDisplayRevision
        self.surfaceSettings = surfaceSettings
        self.taskByIDSnapshot = taskByIDSnapshot
        self.taskListByIDSnapshot = Dictionary(uniqueKeysWithValues: taskLists.map { ($0.id, $0) })
        self.taskListTitleByID = taskListTitleByID
        self.taskChildrenByParentID = taskChildrenByParentID
        self.duplicateIndex = duplicateIndex
        self.isRebuildingDerivedSnapshots = isRebuildingDerivedSnapshots
    }

    func setRebuildingDerivedSnapshots(_ value: Bool) {
        guard isRebuildingDerivedSnapshots != value else { return }
        isRebuildingDerivedSnapshots = value
    }

    func task(id: TaskMirror.ID) -> TaskMirror? {
        taskByIDSnapshot[id] ?? tasks.first { $0.id == id }
    }

    func taskList(id: TaskListMirror.ID) -> TaskListMirror? {
        taskListByIDSnapshot[id] ?? taskLists.first { $0.id == id }
    }

    func taskListTitle(for id: TaskListMirror.ID, fallback: String = "Unknown list") -> String {
        taskListTitleByID[id] ?? fallback
    }

    func taskListTitlesByID() -> [TaskListMirror.ID: String] {
        taskListTitleByID
    }

    func children(of taskID: TaskMirror.ID) -> [TaskMirror] {
        (taskChildrenByParentID[taskID] ?? []).compactMap { task(id: $0) }
    }

    func duplicateGroupKey(for taskID: TaskMirror.ID) -> String? {
        duplicateIndex.groupKey(for: taskID)
    }

    func duplicateSiblings(of taskID: TaskMirror.ID) -> [TaskMirror] {
        duplicateIndex.siblings(of: taskID).compactMap { task(id: $0) }
    }

    func canIndent(_ task: TaskMirror) -> Bool {
        TaskHierarchy.canIndent(task, within: tasks)
    }

    func canOutdent(_ task: TaskMirror) -> Bool {
        TaskHierarchy.canOutdent(task)
    }
}

struct StatusBannerSnapshot: Equatable, Sendable {
    var authState: AuthState
    var syncState: SyncState
    var mutationError: String?
    var cacheWarning: String?
    var isSyncPaused: Bool
    var pendingMutationCount: Int
    var quarantinedCount: Int
    var invalidPayloadCount: Int
    var conflictCount: Int
    var deferredReminderSummary: NotificationScheduleSummary?
    var syncFailureKind: SyncFailureKind?
    var daysSinceLastLaunch: Int?
    var syncScope: SyncScopeSummary

    static let empty = StatusBannerSnapshot(
        authState: .authenticating,
        syncState: .idle,
        mutationError: nil,
        cacheWarning: nil,
        isSyncPaused: false,
        pendingMutationCount: 0,
        quarantinedCount: 0,
        invalidPayloadCount: 0,
        conflictCount: 0,
        deferredReminderSummary: nil,
        syncFailureKind: nil,
        daysSinceLastLaunch: nil,
        syncScope: SyncScopeSummary(tasks: 0, events: 0)
    )
}

struct MenuBarSettingsSnapshot: Equatable, Sendable {
    var showMenuBarExtra: Bool
    var showMenuBarBadge: Bool
    var menuBarIcon: AppSettings.MenuBarIcon
    var menuBarStyle: AppSettings.MenuBarStyle
    var menuBarAdaptiveStatusSource: AppSettings.MenuBarAdaptiveStatusSource
    var menuBarAdaptiveEmptyBehavior: AppSettings.MenuBarAdaptiveEmptyBehavior
    var menuBarAdaptivePanelContent: AppSettings.MenuBarAdaptivePanelContent
    var appLanguage: AppLanguage
    var uiLayoutScale: Double
    var uiTextSizePoints: Double
    var uiFontName: String?
    var colorSchemeID: String
    var customColorSchemes: [HCBCustomColorScheme]
    var appBackgroundTranslucencyEnabled: Bool
    var appBackgroundOpacity: Double
    var customBackgroundImagePath: String?
    var disableAnimations: Bool
    var performanceMode: AppPerformanceMode

    static let `default` = MenuBarSettingsSnapshot(settings: .default)

    init(settings: AppSettings) {
        showMenuBarExtra = settings.showMenuBarExtra
        showMenuBarBadge = settings.showMenuBarBadge
        menuBarIcon = settings.menuBarIcon
        menuBarStyle = settings.menuBarStyle
        menuBarAdaptiveStatusSource = settings.menuBarAdaptiveStatusSource
        menuBarAdaptiveEmptyBehavior = settings.menuBarAdaptiveEmptyBehavior
        menuBarAdaptivePanelContent = settings.menuBarAdaptivePanelContent
        appLanguage = settings.appLanguage
        uiLayoutScale = settings.uiLayoutScale
        uiTextSizePoints = settings.uiTextSizePoints
        uiFontName = settings.uiFontName
        colorSchemeID = settings.colorSchemeID
        customColorSchemes = settings.customColorSchemes
        appBackgroundTranslucencyEnabled = settings.appBackgroundTranslucencyEnabled
        appBackgroundOpacity = settings.appBackgroundOpacity
        customBackgroundImagePath = settings.customBackgroundImagePath
        disableAnimations = settings.disableAnimations
        performanceMode = settings.performanceMode
    }
}

struct MenuBarDayProjection: Equatable, Sendable {
    var events: [CalendarEventMirror]
    var tasks: [TaskMirror]

    static let empty = MenuBarDayProjection(events: [], tasks: [])
}

struct MenuBarPinnedFilterProjection: Identifiable, Equatable, Sendable {
    var definition: CustomFilterDefinition
    var matchingTasks: [TaskMirror]

    var id: CustomFilterDefinition.ID { definition.id }
}

struct MenuBarProjection: Equatable, Sendable {
    var settings: MenuBarSettingsSnapshot
    var syncState: SyncState
    var account: GoogleAccount?
    var connectedAccounts: [GoogleAccount]
    var activeAccountID: GoogleAccount.ID?
    var overdueCount: Int
    var displayRevision: UInt64
    var adaptiveEvents: [CalendarEventMirror]
    var adaptiveTasks: [TaskMirror]
    var dayBuckets: [TimeInterval: MenuBarDayProjection]
    var pinnedFilters: [MenuBarPinnedFilterProjection]
    var calendarTitleByID: [CalendarListMirror.ID: String]
    var taskListTitleByID: [TaskListMirror.ID: String]

    static let empty = MenuBarProjection(
        settings: .default,
        syncState: .idle,
        account: nil,
        connectedAccounts: [],
        activeAccountID: nil,
        overdueCount: 0,
        displayRevision: 0,
        adaptiveEvents: [],
        adaptiveTasks: [],
        dayBuckets: [:],
        pinnedFilters: [],
        calendarTitleByID: [:],
        taskListTitleByID: [:]
    )

    var displayAccounts: [GoogleAccount] {
        var seen = Set<GoogleAccount.ID>()
        var ordered: [GoogleAccount] = []
        if let account, seen.insert(account.id).inserted {
            ordered.append(account)
        }
        for connectedAccount in connectedAccounts where seen.insert(connectedAccount.id).inserted {
            ordered.append(connectedAccount)
        }
        return ordered
    }

    var activeAccount: GoogleAccount? {
        let accounts = displayAccounts
        if let activeAccountID, let account = accounts.first(where: { $0.id == activeAccountID }) {
            return account
        }
        return account ?? accounts.first
    }

    func adaptiveStatus(now: Date = Date(), calendar: Calendar = .current) -> MenuBarAdaptiveStatus {
        MenuBarAdaptiveStatusResolver.status(
            now: now,
            events: adaptiveEvents,
            tasks: adaptiveTasks,
            source: settings.menuBarAdaptiveStatusSource,
            emptyBehavior: settings.menuBarAdaptiveEmptyBehavior,
            calendar: calendar
        )
    }

    func dayProjection(on day: Date, calendar: Calendar = .current) -> MenuBarDayProjection {
        dayBuckets[calendar.startOfDay(for: day).timeIntervalSinceReferenceDate] ?? .empty
    }

    func events(on day: Date, calendar: Calendar = .current) -> [CalendarEventMirror] {
        dayProjection(on: day, calendar: calendar).events
    }

    func tasks(on day: Date, calendar: Calendar = .current) -> [TaskMirror] {
        dayProjection(on: day, calendar: calendar).tasks
    }

    func calendarTitle(for id: CalendarListMirror.ID, fallback: String = "Calendar") -> String {
        calendarTitleByID[id] ?? fallback
    }

    func taskListTitle(for id: TaskListMirror.ID, fallback: String = "Tasks") -> String {
        taskListTitleByID[id] ?? fallback
    }
}

@MainActor
@Observable
final class SettingsStore {
    private(set) var settings: AppSettings = .default
    private(set) var shellChrome: ShellChromeSettings = .default

    func publish(settings: AppSettings) {
        let shellChrome = ShellChromeSettings(settings: settings)
        if self.settings != settings {
            self.settings = settings
        }
        if self.shellChrome != shellChrome {
            self.shellChrome = shellChrome
        }
    }
}

@MainActor
@Observable
final class SyncStatusStore {
    private(set) var authState: AuthState = .authenticating
    private(set) var syncState: SyncState = .idle
    private(set) var isSyncPaused: Bool = false
    private(set) var pendingMutationCount: Int = 0
    private(set) var daysSinceLastLaunch: Int?
    private(set) var lastSuccessfulSyncAt: Date?
    private(set) var syncFailureKind: SyncFailureKind?
    private(set) var banner: StatusBannerSnapshot = .empty

    func publish(
        authState: AuthState,
        syncState: SyncState,
        isSyncPaused: Bool,
        pendingMutationCount: Int,
        daysSinceLastLaunch: Int?,
        lastSuccessfulSyncAt: Date?,
        syncFailureKind: SyncFailureKind?,
        banner: StatusBannerSnapshot
    ) {
        if self.authState != authState { self.authState = authState }
        if self.syncState != syncState { self.syncState = syncState }
        if self.isSyncPaused != isSyncPaused { self.isSyncPaused = isSyncPaused }
        if self.pendingMutationCount != pendingMutationCount { self.pendingMutationCount = pendingMutationCount }
        if self.daysSinceLastLaunch != daysSinceLastLaunch { self.daysSinceLastLaunch = daysSinceLastLaunch }
        if self.lastSuccessfulSyncAt != lastSuccessfulSyncAt { self.lastSuccessfulSyncAt = lastSuccessfulSyncAt }
        if self.syncFailureKind != syncFailureKind { self.syncFailureKind = syncFailureKind }
        if self.banner != banner { self.banner = banner }
    }
}

@MainActor
@Observable
final class CommandPaletteStore {
    private(set) var pendingPaletteQuery: String?
    private(set) var entityRevision: UInt64 = 0
    private(set) var taskLists: [TaskListMirror] = []
    private(set) var calendars: [CalendarListMirror] = []
    private(set) var customFilters: [CustomFilterDefinition] = []
    private(set) var usesDatabaseEntitySearch: Bool = true
    private(set) var performanceMode: AppPerformanceMode = .snappy

    func publish(
        pendingPaletteQuery: String?,
        entityRevision: UInt64,
        taskLists: [TaskListMirror],
        calendars: [CalendarListMirror],
        customFilters: [CustomFilterDefinition],
        usesDatabaseEntitySearch: Bool,
        performanceMode: AppPerformanceMode
    ) {
        if self.pendingPaletteQuery != pendingPaletteQuery { self.pendingPaletteQuery = pendingPaletteQuery }
        if self.entityRevision != entityRevision { self.entityRevision = entityRevision }
        if self.taskLists != taskLists { self.taskLists = taskLists }
        if self.calendars != calendars { self.calendars = calendars }
        if self.customFilters != customFilters { self.customFilters = customFilters }
        if self.usesDatabaseEntitySearch != usesDatabaseEntitySearch {
            self.usesDatabaseEntitySearch = usesDatabaseEntitySearch
        }
        if self.performanceMode != performanceMode { self.performanceMode = performanceMode }
    }
}

@MainActor
@Observable
final class MenuBarStore {
    private(set) var projection: MenuBarProjection = .empty

    func publish(projection: MenuBarProjection) {
        guard self.projection != projection else { return }
        self.projection = projection
    }
}

#if DEBUG
struct AppModelApplyProfile: Sendable {
    var diffSetupMilliseconds: Double
    var assignmentMilliseconds: Double
    var autoIncludeMilliseconds: Double
    var rebuildSnapshotsMilliseconds: Double
    var postApplyMilliseconds: Double
    var totalMilliseconds: Double

    static func milliseconds(from start: UInt64, to end: UInt64) -> Double {
        Double(end - start) / 1_000_000
    }
}

struct AppModelEventOnlyApplyProfile: Sendable {
    var lookupMilliseconds: Double
    var indexMilliseconds: Double
    var snapshotMapMilliseconds: Double
    var bucketMilliseconds: Double
    var todayMilliseconds: Double
    var totalMilliseconds: Double
}

#endif

struct DerivedSnapshotBuildProfile: Sendable {
    var visibilityMilliseconds: Double
    var taskSectionsMilliseconds: Double
    var taskBoardMilliseconds: Double
    var todaySnapshotMilliseconds: Double
    var todayTaskDueMilliseconds: Double
    var todayTaskOverdueMilliseconds: Double
    var todayEventFilterMilliseconds: Double
    var todayEventSortMilliseconds: Double
    var todayScheduledEventCount: Int
    var calendarSnapshotMilliseconds: Double
    var calendarSetupMilliseconds: Double
    var calendarEventScanMilliseconds: Double
    var calendarEventVisibilityMilliseconds: Double
    var calendarCountMilliseconds: Double
    var calendarColorAggregationMilliseconds: Double
    var calendarLiteralTagExtractionMilliseconds: Double
    var calendarLiteralSummaryScanMilliseconds: Double
    var calendarLiteralDetailsScanMilliseconds: Double
    var calendarLiteralLocationScanMilliseconds: Double
    var calendarLiteralRegexMatchingMilliseconds: Double
    var calendarLiteralDeduplicationMilliseconds: Double
    var calendarBoundTagAggregationMilliseconds: Double
    var calendarTagCountMilliseconds: Double
    var calendarColorMapMilliseconds: Double
    var calendarSelectedEventCount: Int
    var calendarVisibleEventCount: Int
    var calendarLiteralTaggedEventCount: Int
    var calendarBoundTaggedEventCount: Int
    var coreSnapshotMilliseconds: Double
    var eventsByCalendarMilliseconds: Double
    var eventsByDayMilliseconds: Double
    var eventHiddenFilteringMilliseconds: Double
    var eventStartOfDayMilliseconds: Double
    var eventStartDayNormalizationMilliseconds: Double
    var eventEndDayNormalizationMilliseconds: Double
    var eventSameDayProbeMilliseconds: Double
    var eventSingleDayTimedMilliseconds: Double
    var eventAllDayMilliseconds: Double
    var eventMultiDayMilliseconds: Double
    var eventDaySteppingMilliseconds: Double
    var eventAllDaySteppingMilliseconds: Double
    var eventMultiDaySteppingMilliseconds: Double
    var eventBucketInsertionMilliseconds: Double
    var eventBucketMilliseconds: Double
    var hiddenEventCount: Int
    var singleDayTimedEventCount: Int
    var allDayEventCount: Int
    var multiDayEventCount: Int
    var taskDueBucketsMilliseconds: Double
    var taskSidebarCountsMilliseconds: Double
    var taskCompletionStatsMilliseconds: Double
    var taskBucketAndStatsMilliseconds: Double
    var taskIndexMilliseconds: Double
    var eventIndexMilliseconds: Double
    var idIndexMilliseconds: Double
    var titleLookupMilliseconds: Double
    var taskChildrenMilliseconds: Double
    var duplicateIndexMilliseconds: Double
    var taskListIndexMilliseconds: Double
    var auxiliaryLookupMilliseconds: Double
    var fingerprintMilliseconds: Double
    var totalMilliseconds: Double
}

@MainActor
@Observable
final class AppModel {
    private let authService: GoogleAuthService
    private let tasksClient: GoogleTasksClient
    private let calendarClient: GoogleCalendarClient
    private let syncScheduler: SyncScheduler
    private let cacheStore: LocalCacheStore
    private let calendarQueryService: any CalendarQuerying
    private let notificationScheduler: LocalNotificationScheduler
    private let spotlightIndexer: SpotlightIndexer
    private let loginItemController: LoginItemController
    private let localBackupService: LocalBackupService
    @ObservationIgnored let calendarStore = CalendarStore()
    @ObservationIgnored let taskStore = TaskStore()
    @ObservationIgnored let settingsStore = SettingsStore()
    @ObservationIgnored let syncStatusStore = SyncStatusStore()
    @ObservationIgnored let commandPaletteStore = CommandPaletteStore()
    @ObservationIgnored let menuBarStore = MenuBarStore()
    @ObservationIgnored private var mcpServerController: MCPServerController?
    @ObservationIgnored private var calendarPreparedSnapshotCache = CalendarPreparedSnapshotCache(limit: 12)
    @ObservationIgnored private var calendarRevisionKeyCache: [String: String] = [:]
    @ObservationIgnored private var calendarDisplayFingerprint: Int?
    @ObservationIgnored private var nearRealtimePollDiagnostics: NearRealtimePollDiagnostics?
    @ObservationIgnored private var notificationDiagnosticUpdatedAt: Date?
    @ObservationIgnored private var notificationDiagnosticDurationMilliseconds: Double?
    #if DEBUG
    @ObservationIgnored private(set) var lastApplyProfileForBenchmark: AppModelApplyProfile?
    @ObservationIgnored private(set) var lastDerivedSnapshotBuildProfileForBenchmark: DerivedSnapshotBuildProfile?
    @ObservationIgnored private(set) var lastEventOnlyApplyProfileForBenchmark: AppModelEventOnlyApplyProfile?
    @ObservationIgnored private(set) var lastApplyUsedEventOnlyDerivedUpdateForBenchmark = false
#endif

    private(set) var account: GoogleAccount?
    private(set) var connectedAccounts: [GoogleAccount] = []
    private(set) var activeAccountID: GoogleAccount.ID?
    private(set) var accountWorkspaces: [AccountWorkspaceSnapshot] = []
    // Default to .authenticating (not .signedOut) so the pre-loadInitialState
    // window shows a "connecting" state instead of a false-positive
    // "reconnect Google" banner. loadInitialState flips this to the real
    // state once the cached account + Keychain restore have run.
    private(set) var authState: AuthState = .authenticating
    private(set) var syncState: SyncState = .idle
    // Reference-counted so overlapping mutations (e.g. TaskInspectorView's
    // detached commit-on-close race with a simultaneous completion toggle
    // elsewhere) don't prematurely flip isMutating to false and let
    // refreshNow start syncing while another mutation's still in flight.
    private var mutationCount: Int = 0
    var isMutating: Bool { mutationCount > 0 }
    private var isReplayingMutations: Bool = false
    // Set by the near-real-time loop when it has exhausted its retry
    // budget against transient failures; cleared on a successful refresh or
    // a manual resume (scene activation, user tap).
    private(set) var isSyncPaused: Bool = false
    // Set when the Share Extension (or a future Services-menu / drag
    // handler) queues text for the main app. QuickAddView reads this
    // on appear, prefills its title field, then clears it.
    var pendingSharedPrefill: String?
    // Populated by HCBDeepLinkRouter when a hotcrossbuns:// URL routes to
    // new/task. The AddTaskSheet reads these on appear, populates its fields,
    // then clears to nil. Deep links never auto-submit — the user always
    // confirms the prefilled sheet before any Google write.
    var pendingTaskPrefill: DeepLinkTaskPrefill?
    var pendingEventPrefill: DeepLinkEventPrefill?
    // Query pre-populated into the command palette when a search deep link fires.
    var pendingPaletteQuery: String?
    // Populated on first launch-time check; DiagnosticsView surfaces.
    private(set) var keychainHealth: KeychainHealth = .unknown
    private(set) var customOAuthClientConfiguration: GoogleOAuthClientConfiguration?
    private(set) var opensAtLogin: Bool = false
    private(set) var loginItemError: String?
    // Days between this launch and the previous launch's wall-clock,
    // computed once in loadInitialState. Surfaced in AppStatusBanner
    // alongside the .syncing state so users see "5 days since last
    // open — fetching from Google" rather than a silent freeze on
    // first launch after a gap. Nil on the very first launch (no
    // prior timestamp on disk).
    private(set) var daysSinceLastLaunch: Int?
    private(set) var lastMutationError: String?
    private(set) var lastCacheLoadWarning: String?
    private(set) var usesDatabaseEntitySearch = false
    private(set) var globalHotkeyRegistrationState: GlobalHotkeyRegistrationState = .disabled
    private(set) var syncFailureKind: SyncFailureKind?
    private(set) var loadingOverlay: LoadingOverlayState?
    private(set) var localBackupSummary: LocalBackupService.BackupSummary?
    private(set) var mcpServerStatus: MCPServerStatus = .stopped
    private(set) var mcpRecentActivity: [MCPActivityEntry] = []
    private(set) var taskLists: [TaskListMirror] = []
    private(set) var tasks: [TaskMirror] = []
    private(set) var calendars: [CalendarListMirror] = []
    private(set) var events: [CalendarEventMirror] = []
    // Pre-bucketed event IDs by calendar id. Recomputed in rebuildSnapshots so
    // each grid view can avoid the per-render `model.events.filter` over the
    // full event corpus (~17k+ at scale → ~3k once bucketed by typical
    // calendar selections, and lookup is O(1) per calendar). Cancelled
    // events are omitted to match the existing visibleEvents semantics. IDs
    // keep these indexes compact instead of retaining duplicate event mirrors.
    private(set) var eventsByCalendar: [CalendarListMirror.ID: [CalendarEventMirror.ID]] = [:]
    // Pre-bucketed event IDs keyed on startOfDay. Multi-day events appear in
    // every day they overlap. Cancelled events excluded. Grid views look up
    // O(1) per cell rather than re-filtering the full event corpus each
    // render. Stored as [TimeInterval] keys so Dictionary hashing stays
    // fast — comparing Date instances allocates formatter strings in some
    // Swift runtimes. Callers round to startOfDay before lookup.
    private(set) var eventsByDay: [TimeInterval: [CalendarEventMirror.ID]] = [:]
    // Pre-bucketed task IDs keyed on startOfDay(dueDate). Open, non-deleted
    // tasks only. Mirrors eventsByDay so grids skip the tasks.filter per
    // cell render without retaining duplicate task mirrors.
    private(set) var tasksByDueDate: [TimeInterval: [TaskMirror.ID]] = [:]
    private(set) var visibleTaskListIDs: Set<TaskListMirror.ID> = []
    private(set) var taskSections: [TaskListSectionSnapshot] = []
    private(set) var taskBoardSnapshot: TaskBoardSnapshot = .empty
    private(set) var todaySnapshot: TodaySnapshot = .empty
    private(set) var calendarSnapshot: CalendarSnapshot = .empty
    // Cached during rebuildSnapshots so the sidebar badge doesn't have to
    // re-filter the full tasks array on every sidebar render.
    private(set) var openTaskCountForSidebar: Int = 0
    // Split badges for the post-refactor sidebar: dated open tasks land in
    // Tasks; standalone undated notes plus tasks with note bodies land in
    // Notes. Recomputed in rebuildSnapshots alongside the combined count so
    // the sidebar can render without re-filtering.
    private(set) var datedOpenTaskCount: Int = 0
    private(set) var undatedOpenTaskCount: Int = 0
    // Per-list completion stats for Store section headers — avoids O(n)
    // filtering per list on every header render.
    private(set) var taskListCompletionStats: [TaskListMirror.ID: TaskListCompletionStats] = [:]
    // Small title lookup maps shared by row/menu/search surfaces. These avoid
    // repeated first(where:) scans while keeping the large task/event mirrors
    // as the single source of truth.
    private(set) var taskListTitleByID: [TaskListMirror.ID: String] = [:]
    private(set) var calendarTitleByID: [CalendarListMirror.ID: String] = [:]
    private(set) var taskChildrenByParentID: [TaskMirror.ID: [TaskMirror.ID]] = [:]
    // Monotonic counter bumped on every rebuildSnapshots pass — i.e. every
    // time the observable task/event/list/calendar state changes in a way a
    // downstream view might care about. Consumer views (MonthGrid, WeekGrid,
    // CommandPalette) compose this into their cache keys instead of the
    // previous count-only key; renames / reschedules / recolors that kept
    // the total count the same used to not bust those caches, producing
    // stale UI. Bumping here is the one-place invariant: if snapshots
    // rebuilt, the revision advances.
    private(set) var dataRevision: UInt64 = 0
    // Bumped only when data that can affect calendar display snapshots
    // changes. Calendar views key prepared snapshots from this narrower
    // revision so unrelated task/list/settings churn does not flush cached
    // day/week/month/year work.
    private(set) var calendarDisplayRevision: UInt64 = 0
    // Bumped only when task-list/task/settings data that can affect the
    // Tasks tab changes. Calendar-only syncs must not invalidate task boards.
    private(set) var taskDisplayRevision: UInt64 = 0
    // Same as taskDisplayRevision, scoped to the Notes tab so local ordering
    // and prepared snapshot keys do not churn on unrelated data changes.
    private(set) var notesDisplayRevision: UInt64 = 0
    private(set) var commandPaletteEntityRevision: UInt64 = 0
    private(set) var menuBarDisplayRevision: UInt64 = 0
    @ObservationIgnored private var calendarDayDisplayRevisions: [TimeInterval: UInt64] = [:]
    // True while a scheduled derived snapshot/index rebuild is in flight.
    // Prepared views use this to block interactions rather than rendering
    // stale immutable snapshots against freshly mutated source arrays.
    private(set) var isRebuildingDerivedSnapshots: Bool = false
    // O(1) ID-to-index lookups maintained by rebuildSnapshots. task(id:) / event(id:)
    // previously scanned the full arrays on every call, including inside
    // menus and body evaluations — observable lag for users with thousands
    // of tasks/events. The values are indexes rather than full mirrors so
    // lookup tables do not double-retain the largest data collections.
    private(set) var taskByIDSnapshot: [TaskMirror.ID: TaskMirror] = [:]
    private(set) var eventByIDSnapshot: [CalendarEventMirror.ID: CalendarEventMirror] = [:]
    private var taskIndexByID: [TaskMirror.ID: Int] = [:]
    private var eventIndexByID: [CalendarEventMirror.ID: Int] = [:]
    private var taskListIndexByID: [TaskListMirror.ID: Int] = [:]
    private(set) var syncCheckpoints: [SyncCheckpoint] = []
    private(set) var pendingMutations: [PendingMutation] = []
    private(set) var lastNotificationScheduleSummary: NotificationScheduleSummary?
    private(set) var recentlyCompletedTaskID: TaskMirror.ID?
    private(set) var undoable: UndoableAction?
    private var undoActionID = UUID()
    var undoActionToken: UUID { undoActionID }
    var settings: AppSettings
    // Rebuilt by rebuildDuplicateIndex whenever tasks or dismissedDuplicateGroups change. Cards and the task inspector read .isMember / .siblings to show the !! badge and duplicate banner.
    private(set) var duplicateIndex: DuplicateIndex = .empty

    // Past-cleanup coordinator. Assigned at the tail of init — @Observable
    // blocks lazy storage, so this is IUO wired up in the initializer once
    // `self` is complete enough to unown safely.
    private(set) var pastCleanupCoordinator: PastCleanupCoordinator!

    var lastSuccessfulSyncAt: Date? {
        syncCheckpoints.compactMap(\.lastSuccessfulSyncAt).max()
    }

    var isGoogleAuthConfigured: Bool {
        authService.isConfigured
    }

    init(
        authService: GoogleAuthService,
        tasksClient: GoogleTasksClient,
        calendarClient: GoogleCalendarClient,
        syncScheduler: SyncScheduler,
        cacheStore: LocalCacheStore,
        calendarQueryService: (any CalendarQuerying)? = nil,
        notificationScheduler: LocalNotificationScheduler = LocalNotificationScheduler(),
        spotlightIndexer: SpotlightIndexer = SpotlightIndexer(),
        loginItemController: LoginItemController = LoginItemController(),
        localBackupService: LocalBackupService = LocalBackupService(),
        settings: AppSettings = .default
    ) {
        self.authService = authService
        self.tasksClient = tasksClient
        self.calendarClient = calendarClient
        self.syncScheduler = syncScheduler
        self.cacheStore = cacheStore
        self.calendarQueryService = calendarQueryService ?? LocalCacheCalendarQueryService(cacheStore: cacheStore)
        self.notificationScheduler = notificationScheduler
        self.spotlightIndexer = spotlightIndexer
        self.loginItemController = loginItemController
        self.localBackupService = localBackupService
        self.settings = settings
        GoogleDiagnostics.setRawPayloadLoggingEnabled(settings.rawGoogleDiagnosticsEnabled)
        self.customOAuthClientConfiguration = authService.customOAuthClientConfiguration
        self.opensAtLogin = loginItemController.isEnabled
        self.pastCleanupCoordinator = PastCleanupCoordinator(model: self)
        publishSnapshotSurfaceStores()
    }

    static func bootstrap() -> AppModel {
        let customOAuthService = CustomGoogleOAuthService()
        let tokenProvider = GoogleSignInAccessTokenProvider(customOAuthService: customOAuthService)
        let transport = GoogleAPITransport(
            baseURL: URL(string: "https://www.googleapis.com")!,
            tokenProvider: tokenProvider
        )

        let tasksClient = GoogleTasksClient(transport: transport)
        let calendarClient = GoogleCalendarClient(transport: transport)
        return AppModel(
            authService: GoogleAuthService(customOAuthService: customOAuthService),
            tasksClient: tasksClient,
            calendarClient: calendarClient,
            syncScheduler: SyncScheduler(
                tasksClient: tasksClient,
                calendarClient: calendarClient
            ),
            cacheStore: LocalCacheStore(),
            notificationScheduler: LocalNotificationScheduler()
        )
    }

    static func testHostBootstrap() -> AppModel {
        let customOAuthService = CustomGoogleOAuthService(tokenStore: EmptyGoogleOAuthTokenStore())
        let transport = GoogleAPITransport(
            baseURL: URL(string: "https://www.googleapis.com")!,
            tokenProvider: StaticAccessTokenProvider(token: "unit-test-token")
        )
        let tasksClient = GoogleTasksClient(transport: transport)
        let calendarClient = GoogleCalendarClient(transport: transport)
        return AppModel(
            authService: GoogleAuthService(customOAuthService: customOAuthService),
            tasksClient: tasksClient,
            calendarClient: calendarClient,
            syncScheduler: SyncScheduler(tasksClient: tasksClient, calendarClient: calendarClient),
            cacheStore: LocalCacheStore(fileURL: nil),
            notificationScheduler: LocalNotificationScheduler()
        )
    }

    static var preview: AppModel {
        let model = AppModel.testHostBootstrap()
        model.installPreviewData()
        return model
    }

    func loadInitialState() async {
        loadingOverlay = LoadingOverlayState(message: "Loading your workspace…")
        defer { loadingOverlay = nil }
        recordLaunchAndComputeDaysSinceLast()
        // Probe Keychain before anything that would touch GIDSignIn. If
        // the Keychain is locked/denied the restore will fail generically;
        // the probe lets us surface a specific reason in Diagnostics.
        keychainHealth = KeychainProbe.run()
        customOAuthClientConfiguration = authService.customOAuthClientConfiguration
        if keychainHealth == .denied {
            AppLogger.warn("keychain inaccessible at launch", category: .auth)
        }
        // §6.12 — if the cache is encrypted and we have a cached key in
        // Keychain, install it on the store before the first read.
        let cachedCacheKey = HCBCacheKeychain.load()
        if let key = cachedCacheKey {
            await cacheStore.setEncryptionKey(key)
        }
        usesDatabaseEntitySearch = await cacheStore.supportsEntitySearch()
        let cachedState = await cacheStore.loadCachedState()
        apply(cachedState, rebuildStrategy: .deferred)
        await dismissLoadingOverlayBeforeFollowUpWork()
        localBackupSummary = await localBackupService.summary()
        await runDailyLocalBackupIfNeeded()
        if settings.auditLogEncryptionEnabled,
           let key = cachedCacheKey,
           let salt = await cacheStore.currentSalt() {
            await MutationAuditLog.shared.configureEncryption(enabled: true, key: key, salt: salt)
        } else {
            await MutationAuditLog.shared.configureEncryption(enabled: false, key: nil, salt: nil)
        }
        // If we have a cached account, optimistically show signed-in while
        // restoreGoogleSession runs. If not, stay in .authenticating — don't
        // flip to .signedOut prematurely; restoreGoogleSession will do that
        // only if the Keychain actually has no prior session.
        if let account = cachedState.account {
            authState = .signedIn(account)
        }
        lastCacheLoadWarning = await cacheStore.lastLoadWarning
        publishSnapshotSurfaceStores()
        await synchronizeLocalNotifications()
        reconcileMCPServer()
        // Arm the daily midnight tick for past-cleanup. No-op until the
        // user has enabled a deletion behavior AND acknowledged the
        // blast-radius modal; the coordinator filters on read.
        pastCleanupCoordinator.scheduleDailyTick()
    }

    private func dismissLoadingOverlayBeforeFollowUpWork() async {
        guard loadingOverlay != nil else { return }
        loadingOverlay = nil
        await Task.yield()
    }

    // Reads the prior wall-clock launch time from UserDefaults, computes the
    // gap in whole days, and stamps "now". Nil result on first launch (no
    // prior timestamp). Surfaced by AppStatusBanner alongside .syncing so a
    // long-absence cold launch reads as "5 days since last open — fetching"
    // rather than a silent multi-second freeze.
    private func recordLaunchAndComputeDaysSinceLast() {
        let key = "hcb.lastSeenAt"
        let defaults = UserDefaults.standard
        let now = Date()
        if let stored = defaults.object(forKey: key) as? Date {
            let days = Calendar.current.dateComponents([.day], from: stored, to: now).day ?? 0
            daysSinceLastLaunch = max(0, days)
        }
        defaults.set(now, forKey: key)
    }

    func restoreGoogleSession() async {
        defer { publishSessionSurfaceStores() }
        do {
            guard let restoredAccount = try await authService.restorePreviousSignIn() else {
                account = nil
                activeAccountID = nil
                authState = .signedOut
                return
            }

            setActiveAccount(restoredAccount)
            authState = .signedIn(restoredAccount)
        } catch {
            authState = .failed(error.localizedDescription)
        }
    }

    func connectGoogleAccount() async {
        authState = .authenticating
        publishSessionSurfaceStores()
        let priorState = currentCachedState()
        do {
            let account = try await authService.signIn()
            apply(priorState.activating(account: account))
            authState = .signedIn(account)
            syncState = .idle
            publishSessionSurfaceStores()
            // Force-flush: account state must hit disk before any sync run.
            await saveCurrentState()
        } catch {
            if GoogleAuthService.isUserCancellation(error) {
                authState = .cancelled("Sign-in was cancelled. Tap Connect Google to try again.")
            } else {
                authState = .failed(error.localizedDescription)
            }
            publishSessionSurfaceStores()
        }
    }

    func saveCustomOAuthClientConfiguration(clientID: String, clientSecret: String?) {
        do {
            let saved = try authService.saveCustomOAuthClientConfiguration(clientID: clientID, clientSecret: clientSecret)
            customOAuthClientConfiguration = saved
            if clearCustomDesktopOAuthAccounts() {
                Task { await saveCurrentState() }
            }
        } catch {
            authState = .failed(error.localizedDescription)
        }
    }

    func clearCustomOAuthClientConfiguration() {
        authService.clearCustomOAuthClientConfiguration()
        customOAuthClientConfiguration = nil
        if clearCustomDesktopOAuthAccounts() {
            Task { await saveCurrentState() }
        }
    }

    func disconnectGoogleAccount() async {
        await disconnectGoogleAccount(id: activeAccountID ?? account?.id)
    }

    func disconnectGoogleAccount(id targetAccountID: GoogleAccount.ID?) async {
        guard let targetAccountID else {
            activeAccountID = nil
            account = nil
            authState = .signedOut
            syncState = .idle
            publishSessionSurfaceStores()
            await saveCurrentState()
            return
        }

        let priorState = currentCachedState()
        do {
            try await authService.disconnect(accountID: targetAccountID)
        } catch {
            authService.signOut(accountID: targetAccountID)
        }

        let wasActive = targetAccountID == priorState.activeAccountID
        let fallbackAccountID = wasActive
            ? priorState.accounts.first(where: { $0.id != targetAccountID })?.id
            : priorState.activeAccountID
        let nextState = priorState.removingAccountWorkspace(
            accountID: targetAccountID,
            fallbackAccountID: fallbackAccountID
        )
        apply(nextState)
        authState = nextState.account.map(AuthState.signedIn) ?? .signedOut
        syncState = .idle
        publishSessionSurfaceStores()
        await saveCurrentState()
        if wasActive {
            await spotlightIndexer.removeAll()
            await synchronizeLocalNotifications()
        }
    }

    @discardableResult
    func switchGoogleAccount(to accountID: GoogleAccount.ID) async -> Bool {
        guard accountID != activeAccountID else { return true }
        guard let switched = currentCachedState().switchingActiveAccount(to: accountID) else {
            lastMutationError = "That Google account is not connected on this Mac."
            return false
        }
        apply(switched)
        authState = switched.account.map(AuthState.signedIn) ?? .signedOut
        syncState = .idle
        publishSessionSurfaceStores()
        await saveCurrentState()
        await spotlightIndexer.removeAll()
        await synchronizeLocalNotifications()
        return true
    }

    func handleAuthRedirect(_ url: URL) {
        _ = authService.handleRedirectURL(url)
    }

    func refreshForCurrentSyncMode() async {
        guard settings.syncMode != .manual else {
            return
        }

        await refreshNow()
    }

    enum RefreshOutcome: Sendable {
        case skipped
        case succeeded
        case failed(Error)
    }

    @discardableResult
    func refreshNow() async -> RefreshOutcome {
        if case .syncing = syncState {
            return .skipped
        }

        if isMutating {
            AppLogger.debug("refresh skipped: mutation in flight", category: .sync)
            return .skipped // defer refresh until mutation settles to avoid state races
        }

        guard account != nil else {
            AppLogger.warn("refresh skipped: no account", category: .sync)
            // Don't surface this as a sync .failed state — on cold launch
            // refreshNow runs before auth restore completes, which would
            // flash a "Couldn't reach Google" banner even when the user
            // is correctly signed in. authState .signedOut already surfaces
            // the legitimate "not connected" case via AppStatusBanner.
            return .skipped
        }

        let started = Date()
        AppLogger.info("refresh start", category: .sync, metadata: ["mode": settings.syncMode.rawValue])
        syncState = .syncing(startedAt: started)
        let shouldBlockForInitialRemoteData = tasks.isEmpty && events.isEmpty && calendars.isEmpty
        if shouldBlockForInitialRemoteData {
            loadingOverlay = LoadingOverlayState(message: "Refreshing Google data…")
        }
        publishSessionSurfaceStores()
        defer {
            loadingOverlay = nil
            publishSessionSurfaceStores()
        }
        syncFailureKind = nil
        await replayPendingMutations()
        do {
            let syncResult = try await syncScheduler.syncNowWithChangeSet(
                mode: settings.syncMode,
                baseState: currentCachedState()
            )
            try await cacheStore.commitSyncResult(syncResult)
            let syncedState = syncResult.state
            apply(syncResult)
            authState = syncedState.account.map(AuthState.signedIn) ?? .signedOut
            syncState = .synced(at: Date())
            isSyncPaused = false
            syncFailureKind = nil
            publishSessionSurfaceStores()
            await dismissLoadingOverlayBeforeFollowUpWork()
            await synchronizeLocalNotifications()
            await runDailyLocalBackupIfNeeded()
            let duration = Int(Date().timeIntervalSince(started) * 1000)
            AppLogger.info("refresh succeeded", category: .sync, metadata: [
                "ms": String(duration),
                "tasks": String(tasks.count),
                "events": String(events.count)
            ])
            // Past-cleanup runs silently after each successful sync once
            // the user has acknowledged the blast-radius modal. Fires
            // delete calls serially so quota + audit log stay orderly.
            _ = await pastCleanupCoordinator.runSilentSweepIfAcknowledged()
            return .succeeded
        } catch {
            syncFailureKind = SyncFailureKind.classify(error)
            var httpStatus: String? = nil
            if case let GoogleAPIError.httpStatus(status, _) = error {
                httpStatus = String(status)
                if status == 401 || status == 403 {
                    authState = .failed(error.localizedDescription)
                }
            }
            if let tokenError = error as? GoogleTokenRefreshError, tokenError.requiresReconnect {
                authState = .failed(tokenError.localizedDescription)
            }
            var meta = GoogleDiagnostics.errorMetadata(error)
            if let httpStatus { meta["status"] = httpStatus }
            AppLogger.error("refresh failed", category: .sync, metadata: meta)
            syncState = .failed(message: error.localizedDescription)
            publishSessionSurfaceStores()
            return .failed(error)
        }
    }

    func createTask(
        title: String,
        notes: String,
        dueDate: Date?,
        taskListID: TaskListMirror.ID,
        parentID: TaskMirror.ID? = nil
    ) async -> Bool {
        await createTaskInternal(
            title: title,
            notes: notes,
            dueDate: dueDate,
            taskListID: taskListID,
            parentID: parentID,
            recordAs: nil // default to .taskCreate below
        )
    }

    @discardableResult
    private func createTaskInternal(
        title: String,
        notes: String,
        dueDate: Date?,
        taskListID: TaskListMirror.ID,
        parentID: TaskMirror.ID?,
        recordAs override: ((TaskMirror) -> UndoableAction)?
    ) async -> Bool {
        guard requireAccount(mutationDescription: "creating tasks") else {
            return false
        }

        let trimmedTitle = title.trimmingCharacters(in: .whitespacesAndNewlines)
        let trimmedNotes = notes.trimmingCharacters(in: .whitespacesAndNewlines)
        let localID = OptimisticID.generate()

        let optimisticTask = TaskMirror(
            id: localID,
            taskListID: taskListID,
            parentID: parentID,
            title: trimmedTitle,
            notes: trimmedNotes,
            status: .needsAction,
            dueDate: dueDate,
            completedAt: nil,
            isDeleted: false,
            isHidden: false,
            position: nil,
            etag: nil,
            updatedAt: Date()
        )
        upsert(optimisticTask)

        let undoAction = override?(optimisticTask) ?? .taskCreate(snapshot: optimisticTask)
        guard settings.cloudSyncTargets.syncsTasks else {
            recordUndo(undoAction)
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.taskInserted(optimisticTask))
            await synchronizeLocalNotifications()
            return true
        }

        do {
            let payload = PendingTaskCreatePayload(
                localID: localID,
                taskListID: taskListID,
                title: trimmedTitle,
                notes: trimmedNotes,
                dueDate: dueDate,
                parentID: parentID.flatMap { OptimisticID.isPending($0) ? nil : $0 }
            )
            let mutation = try PendingMutation.taskCreate(payload: payload)
            let queuedMutation = enqueuePendingMutation(mutation)
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.combined(
                LocalCacheChangeSetBuilder.taskInserted(optimisticTask),
                LocalCacheChangeSetBuilder.pendingMutationInserted(queuedMutation)
            ))
        } catch {
            removeTask(id: localID)
            lastMutationError = "Could not queue task for sync: \(error.localizedDescription)"
            return false
        }

        recordUndo(undoAction)
        Task { await replayPendingMutations() }
        return true
    }

    func duplicateTask(_ task: TaskMirror) async -> Bool {
        await duplicateTask(task, dueDate: task.dueDate)
    }

    func duplicateTask(_ task: TaskMirror, dueDate: Date?) async -> Bool {
        let sourceTitle = task.title
        return await createTaskInternal(
            title: task.title,
            notes: task.notes,
            dueDate: dueDate,
            taskListID: task.taskListID,
            parentID: task.parentID,
            recordAs: { optimistic in .taskDuplicate(newSnapshot: optimistic, sourceTitle: sourceTitle) }
        )
    }

    func duplicateEvent(_ event: CalendarEventMirror) async -> Bool {
        await duplicateEvent(event, offsetDays: 0)
    }

    // Shift start/end by offsetDays when duplicating so repeating monthly-ish
    // events that don't fit an RRULE can be cloned forward quickly.
    func duplicateEvent(_ event: CalendarEventMirror, offsetDays: Int) async -> Bool {
        let calendar = Calendar.current
        let shiftedStart = calendar.date(byAdding: .day, value: offsetDays, to: event.startDate) ?? event.startDate
        let shiftedEnd = calendar.date(byAdding: .day, value: offsetDays, to: event.endDate) ?? event.endDate
        return await createEvent(
            summary: event.summary,
            details: event.details,
            startDate: shiftedStart,
            endDate: shiftedEnd,
            isAllDay: event.isAllDay,
            reminderMinutes: event.reminderMinutes.first,
            calendarID: event.calendarID,
            location: event.location,
            startTimeZoneID: event.startTimeZoneID,
            endTimeZoneID: event.endTimeZoneID
        )
    }

    func moveTaskToList(_ task: TaskMirror, toTaskListID: TaskListMirror.ID) async -> Bool {
        guard task.taskListID != toTaskListID else { return true }
        guard requireAccount(mutationDescription: "moving tasks") else { return false }
        if settings.cloudSyncTargets.syncsTasks {
            guard requirePersisted(task.id) else { return false }
        }

        guard settings.cloudSyncTargets.syncsTasks else {
            let moved = applyLocalTaskMove(task, toTaskListID: toTaskListID, parentID: nil, previousSiblingID: nil)
            let fromListTitle = taskLists.first(where: { $0.id == task.taskListID })?.title ?? task.taskListID
            let toListTitle = taskLists.first(where: { $0.id == toTaskListID })?.title ?? toTaskListID
            recordUndo(.taskMove(
                taskID: moved.id,
                fromListID: task.taskListID,
                toListID: toTaskListID,
                title: task.title,
                fromListTitle: fromListTitle,
                toListTitle: toListTitle
            ))
            scheduleCacheSave()
            await synchronizeLocalNotifications()
            return true
        }

        beginMutation()
        // Track the inserted destination task separately so we can compensate
        // if a later step fails. Without this, a failure after insertTask
        // leaves the task duplicated on Google — present in both source and
        // destination lists until the user notices and deletes one manually.
        var insertedForCompensation: TaskMirror?
        do {
            let inserted = try await tasksClient.insertTask(
                taskListID: toTaskListID,
                title: task.title,
                notes: task.notes,
                dueDate: task.dueDate,
                parent: nil
            )
            insertedForCompensation = inserted
            let wasCompleted = task.isCompleted
            var finalTask = inserted
            if wasCompleted {
                finalTask = try await tasksClient.setTaskCompleted(true, task: inserted)
                insertedForCompensation = finalTask
            }
            try await tasksClient.deleteTask(
                taskListID: task.taskListID,
                taskID: task.id,
                ifMatch: task.etag
            )
            removeTask(id: task.id)
            upsert(finalTask)
            let fromListTitle = taskLists.first(where: { $0.id == task.taskListID })?.title ?? task.taskListID
            let toListTitle = taskLists.first(where: { $0.id == toTaskListID })?.title ?? toTaskListID
            recordUndo(.taskMove(
                taskID: finalTask.id,
                fromListID: task.taskListID,
                toListID: toTaskListID,
                title: task.title,
                fromListTitle: fromListTitle,
                toListTitle: toListTitle
            ))
            endMutation(error: nil)
            scheduleCacheSave()
            await synchronizeLocalNotifications()
            return true
        } catch {
            // Best-effort rollback of the destination task. If the
            // compensating delete itself fails, log it via lastMutationError
            // but keep the original failure as the primary message so the
            // user knows what actually went wrong first.
            if let orphan = insertedForCompensation {
                do {
                    try await tasksClient.deleteTask(
                        taskListID: orphan.taskListID,
                        taskID: orphan.id,
                        ifMatch: orphan.etag
                    )
                } catch {
                    lastMutationError = "Move failed and the duplicate on the destination list could not be cleaned up. Please remove it manually."
                    endMutation(error: nil)
                    return false
                }
            }
            endMutation(error: error)
            return false
        }
    }

    func indentTask(_ task: TaskMirror) async -> Bool {
        guard requireAccount(mutationDescription: "indenting tasks") else { return false }
        if settings.cloudSyncTargets.syncsTasks {
            guard requirePersisted(task.id) else { return false }
        }
        guard TaskHierarchy.canIndent(task, within: tasks) else {
            lastMutationError = "This task can't be indented."
            return false
        }
        guard let sibling = TaskHierarchy.precedingSibling(of: task, in: tasks) else {
            lastMutationError = "Indent needs a task above it in the same list."
            return false
        }

        guard settings.cloudSyncTargets.syncsTasks else {
            _ = applyLocalTaskMove(task, toTaskListID: task.taskListID, parentID: sibling.id, previousSiblingID: nil)
            scheduleCacheSave()
            return true
        }

        beginMutation()
        do {
            let moved = try await tasksClient.moveTask(
                taskListID: task.taskListID,
                taskID: task.id,
                parent: sibling.id,
                previous: nil
            )
            upsert(moved)
            endMutation(error: nil)
            scheduleCacheSave()
            return true
        } catch {
            endMutation(error: error)
            return false
        }
    }

    // Reorders `task` so that it sits immediately after `previousSiblingID`
    // (or at the top of its parent when previousSiblingID is nil). Used by
    // the subtask drag-reorder UI.
    func reorderTask(_ task: TaskMirror, afterSiblingID previousSiblingID: TaskMirror.ID?) async -> Bool {
        guard requireAccount(mutationDescription: "reordering tasks") else { return false }
        if settings.cloudSyncTargets.syncsTasks {
            guard requirePersisted(task.id) else { return false }
        }
        if let previousSiblingID, previousSiblingID == task.id { return false }

        guard settings.cloudSyncTargets.syncsTasks else {
            _ = applyLocalTaskMove(
                task,
                toTaskListID: task.taskListID,
                parentID: task.parentID,
                previousSiblingID: previousSiblingID
            )
            scheduleCacheSave()
            return true
        }

        beginMutation()
        do {
            let moved = try await tasksClient.moveTask(
                taskListID: task.taskListID,
                taskID: task.id,
                parent: task.parentID,
                previous: previousSiblingID
            )
            upsert(moved)
            endMutation(error: nil)
            scheduleCacheSave()
            return true
        } catch {
            endMutation(error: error)
            return false
        }
    }

    func outdentTask(_ task: TaskMirror) async -> Bool {
        guard requireAccount(mutationDescription: "outdenting tasks") else { return false }
        if settings.cloudSyncTargets.syncsTasks {
            guard requirePersisted(task.id) else { return false }
        }
        guard TaskHierarchy.canOutdent(task) else {
            lastMutationError = "This task isn't nested."
            return false
        }

        guard settings.cloudSyncTargets.syncsTasks else {
            _ = applyLocalTaskMove(task, toTaskListID: task.taskListID, parentID: nil, previousSiblingID: task.parentID)
            scheduleCacheSave()
            return true
        }

        beginMutation()
        do {
            let moved = try await tasksClient.moveTask(
                taskListID: task.taskListID,
                taskID: task.id,
                parent: nil,
                previous: task.parentID
            )
            upsert(moved)
            endMutation(error: nil)
            scheduleCacheSave()
            return true
        } catch {
            endMutation(error: error)
            return false
        }
    }

    func createTaskList(title: String) async -> Bool {
        guard requireAccount(mutationDescription: "creating task lists") else {
            return false
        }

        let trimmedTitle = title.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmedTitle.isEmpty == false else {
            lastMutationError = "Give the task list a name before saving."
            return false
        }

        guard settings.cloudSyncTargets.syncsTasks else {
            let taskList = TaskListMirror(
                id: "local-task-list-\(UUID().uuidString)",
                title: trimmedTitle,
                updatedAt: Date(),
                etag: nil
            )
            upsert(taskList)

            if settings.hasConfiguredTaskListSelection {
                settings.selectedTaskListIDs.insert(taskList.id)
            }

            scheduleCacheSave()
            return true
        }

        beginMutation()
        do {
            let taskList = try await tasksClient.insertTaskList(title: trimmedTitle)
            upsert(taskList)

            if settings.hasConfiguredTaskListSelection {
                settings.selectedTaskListIDs.insert(taskList.id)
            }

            endMutation(error: nil)
            scheduleCacheSave()
            return true
        } catch {
            endMutation(error: error)
            return false
        }
    }

    func updateTaskList(_ taskList: TaskListMirror, title: String) async -> Bool {
        guard requireAccount(mutationDescription: "updating task lists") else {
            return false
        }

        let trimmedTitle = title.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmedTitle.isEmpty == false else {
            lastMutationError = "Give the task list a name before saving."
            return false
        }

        guard settings.cloudSyncTargets.syncsTasks else {
            var updatedTaskList = taskList
            updatedTaskList.title = trimmedTitle
            updatedTaskList.updatedAt = Date()
            updatedTaskList.etag = nil
            upsert(updatedTaskList)
            scheduleCacheSave()
            return true
        }

        beginMutation()
        do {
            let updatedTaskList = try await tasksClient.updateTaskList(
                taskListID: taskList.id,
                title: trimmedTitle
            )
            upsert(updatedTaskList)
            endMutation(error: nil)
            scheduleCacheSave()
            return true
        } catch {
            endMutation(error: error)
            return false
        }
    }

    @discardableResult
    func clearCompletedTasks(in taskListID: TaskListMirror.ID) async -> Int {
        guard requireAccount(mutationDescription: "clearing completed tasks") else { return 0 }

        guard settings.cloudSyncTargets.syncsTasks else {
            let affected = tasks.filter { $0.taskListID == taskListID && $0.isCompleted }
            for task in affected {
                removeTask(id: task.id)
            }
            scheduleCacheSave()
            await synchronizeLocalNotifications()
            return affected.count
        }

        // Drain any pending delete/completion mutations targeting this list
        // first so the batch clear doesn't race with them — otherwise a
        // queued delete can replay against a task Google has already
        // hidden via clear and 404 with a confusing lastMutationError.
        await replayPendingMutations()
        beginMutation()
        do {
            try await tasksClient.clearCompletedTasks(taskListID: taskListID)
            // Google's clear hides completed tasks server-side. Mirror locally
            // so the UI reflects it immediately — server sync will confirm on
            // the next refresh.
            let affected = tasks.filter { $0.taskListID == taskListID && $0.isCompleted }
            for task in affected {
                removeTask(id: task.id)
            }
            endMutation(error: nil)
            scheduleCacheSave()
            await synchronizeLocalNotifications()
            return affected.count
        } catch {
            endMutation(error: error)
            return 0
        }
    }

    func deleteTaskList(_ taskList: TaskListMirror) async -> Bool {
        guard requireAccount(mutationDescription: "deleting task lists") else {
            return false
        }

        guard settings.cloudSyncTargets.syncsTasks else {
            removeTaskList(id: taskList.id)
            scheduleCacheSave()
            await synchronizeLocalNotifications()
            return true
        }

        beginMutation()
        do {
            try await tasksClient.deleteTaskList(taskListID: taskList.id)
            removeTaskList(id: taskList.id)
            endMutation(error: nil)
            scheduleCacheSave()
            await synchronizeLocalNotifications()
            return true
        } catch {
            endMutation(error: error)
            return false
        }
    }

    func updateTask(
        _ task: TaskMirror,
        title: String,
        notes: String,
        dueDate: Date?
    ) async -> Bool {
        guard requireAccount(mutationDescription: "updating tasks") else {
            return false
        }
        if settings.cloudSyncTargets.syncsTasks {
            guard requirePersisted(task.id) else { return false }
        }

        let trimmedTitle = title.trimmingCharacters(in: .whitespacesAndNewlines)
        let trimmedNotes = notes.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmedTitle.isEmpty == false else {
            lastMutationError = "Give the task a title before saving."
            return false
        }

        let originalTask = task
        var optimistic = task
        optimistic.title = trimmedTitle
        optimistic.notes = trimmedNotes
        optimistic.dueDate = dueDate
        upsert(optimistic)

        guard settings.cloudSyncTargets.syncsTasks else {
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.taskUpdated(old: originalTask, new: optimistic))
            await synchronizeLocalNotifications()
            recordUndo(.taskEdit(priorSnapshot: originalTask))
            return true
        }

        beginMutation()
        do {
            let updatedTask = try await tasksClient.updateTask(
                taskListID: task.taskListID,
                taskID: task.id,
                title: trimmedTitle,
                notes: trimmedNotes,
                dueDate: dueDate,
                ifMatch: task.etag
            )
            upsert(updatedTask)
            endMutation(error: nil)
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.taskUpdated(old: originalTask, new: updatedTask))
            await synchronizeLocalNotifications()
            recordUndo(.taskEdit(priorSnapshot: originalTask))
            return true
        } catch let error as GoogleAPIError where error.isTransient {
            let payload = PendingTaskUpdatePayload(
                taskListID: task.taskListID,
                taskID: task.id,
                title: trimmedTitle,
                notes: trimmedNotes,
                dueDate: dueDate,
                etagSnapshot: task.etag
            )
            if let mutation = try? PendingMutation.taskUpdate(payload: payload) {
                let queuedMutation = enqueuePendingMutation(mutation)
                scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.combined(
                    LocalCacheChangeSetBuilder.taskUpdated(old: originalTask, new: optimistic),
                    LocalCacheChangeSetBuilder.pendingMutationInserted(queuedMutation)
                ))
                await synchronizeLocalNotifications()
                endMutation(error: nil)
                Task { await replayPendingMutations() }
                return true
            }
            upsert(originalTask)
            endMutation(error: error)
            return false
        } catch {
            upsert(originalTask)
            if let apiError = error as? GoogleAPIError, apiError == .preconditionFailed {
                await refreshNow()
            }
            endMutation(error: error)
            return false
        }
    }

    func setTaskCompleted(_ isCompleted: Bool, task: TaskMirror) async -> Bool {
        guard requireAccount(mutationDescription: "updating tasks") else {
            return false
        }
        if settings.cloudSyncTargets.syncsTasks {
            guard requirePersisted(task.id) else { return false }
        }

        // Optimistic local update so the UI flips immediately. Snapshot the
        // prior state so we can revert on terminal failure.
        let originalTask = task
        var optimistic = task
        optimistic.status = isCompleted ? .completed : .needsAction
        optimistic.completedAt = isCompleted ? Date() : nil
        upsert(optimistic)
        // Record the undoable toast NOW, not after the API + notification-sync
        // round-trip. Previously users saw a 15-20s delay between clicking
        // Complete and the toast appearing because recordUndo sat after
        // `await synchronizeLocalNotifications()`. The snapshot (originalTask)
        // is captured pre-await so rollback on terminal failure still works;
        // a stale undo entry from a reverted failure is harmless (the undo
        // path idempotently re-sets the status).
        if isCompleted {
            recentlyCompletedTaskID = optimistic.id
        } else if recentlyCompletedTaskID == optimistic.id {
            recentlyCompletedTaskID = nil
        }
        recordUndo(.taskCompletion(
            taskID: optimistic.id,
            priorCompleted: originalTask.isCompleted,
            title: optimistic.title
        ))
        if isCompleted {
            CompletionSoundPlayer.play(.taskCompleted, settings: settings)
        }

        guard settings.cloudSyncTargets.syncsTasks else {
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.taskUpdated(old: originalTask, new: optimistic))
            await synchronizeLocalNotifications()
            return true
        }

        beginMutation()
        do {
            let updatedTask = try await tasksClient.setTaskCompleted(isCompleted, task: task)
            upsert(updatedTask)
            endMutation(error: nil)
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.taskUpdated(old: originalTask, new: updatedTask))
            await synchronizeLocalNotifications()
            return true
        } catch let error as GoogleAPIError where error.isTransient {
            // Network blip / rate limit / server error — keep the optimistic
            // state and let the replay loop retry this against Google.
            let payload = PendingTaskCompletionPayload(
                taskListID: task.taskListID,
                taskID: task.id,
                isCompleted: isCompleted,
                etagSnapshot: task.etag
            )
            if let mutation = try? PendingMutation.taskCompletion(payload: payload) {
                let queuedMutation = enqueuePendingMutation(mutation)
                scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.combined(
                    LocalCacheChangeSetBuilder.taskUpdated(old: originalTask, new: optimistic),
                    LocalCacheChangeSetBuilder.pendingMutationInserted(queuedMutation)
                ))
                await synchronizeLocalNotifications()
                endMutation(error: nil)
                Task { await replayPendingMutations() }
                return true
            }
            upsert(originalTask)
            endMutation(error: error)
            return false
        } catch {
            upsert(originalTask)
            if let apiError = error as? GoogleAPIError, apiError == .preconditionFailed {
                await refreshNow()
            }
            endMutation(error: error)
            return false
        }
    }

    func clearRecentCompletion() {
        recentlyCompletedTaskID = nil
    }

    func undoRecentCompletion() async {
        guard let id = recentlyCompletedTaskID, let task = task(id: id) else { return }
        recentlyCompletedTaskID = nil
        _ = await setTaskCompleted(false, task: task)
    }

    // MARK: Generic undo

    private func recordUndo(_ action: UndoableAction) {
        let (kind, resourceID, summary, metadata) = Self.auditTuple(for: action)
        let priorJSON = Self.priorSnapshotJSON(for: action)
        let postJSON = Self.postSnapshotJSON(for: action)
        // Informational-only actions (sync diffs, bulk summaries) should not
        // hijack the 6-second undo toast — their performUndo is a no-op, so
        // users would see a dead "Undo" button after every 30s poll tick.
        // Write them straight to the audit log instead.
        switch action {
        case .syncPulled, .bulkAction, .clipboardOp:
            Task { await MutationAuditLog.shared.record(kind: kind, resourceID: resourceID, summary: summary, metadata: metadata, priorSnapshotJSON: priorJSON, postSnapshotJSON: postJSON) }
            return
        default:
            break
        }
        undoable = action
        undoActionID = UUID()
        // Record a permanent audit entry in parallel — the undo window
        // is short (~6s) but the audit log is a forever record of what
        // the user did, useful when reconstructing "when did I mark
        // that task done?" months later.
        Task { await MutationAuditLog.shared.record(kind: kind, resourceID: resourceID, summary: summary, metadata: metadata, priorSnapshotJSON: priorJSON, postSnapshotJSON: postJSON) }
    }

    // JSON-encode the pre-state so the history window can offer "Copy
    // snapshot" on ops that Google's API cannot truly reverse (a hard
    // delete past the undo-stack TTL, a move that reassigned IDs, etc.).
    // Returns nil when there's no meaningful prior state (creates).
    private static func priorSnapshotJSON(for action: UndoableAction) -> String? {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        encoder.dateEncodingStrategy = .iso8601
        switch action {
        case .taskDelete(let snap), .taskEdit(let snap), .taskRestore(let snap):
            return Self.encode(snap, using: encoder)
        case .eventDelete(let snap), .eventEdit(let snap), .eventRestore(let snap), .eventDismissed(let snap):
            return Self.encode(snap, using: encoder)
        default:
            return nil
        }
    }

    // JSON-encode the post-state for ops where the "new" resource is the
    // interesting thing to keep (creates, duplicates). Other cases are
    // either unchanged-from-prior (edits reuse prior) or reversible without
    // a snapshot (completions).
    private static func postSnapshotJSON(for action: UndoableAction) -> String? {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        encoder.dateEncodingStrategy = .iso8601
        switch action {
        case .taskCreate(let snap):
            return Self.encode(snap, using: encoder)
        case .taskDuplicate(let snap, _):
            return Self.encode(snap, using: encoder)
        case .eventCreate(let snap):
            return Self.encode(snap, using: encoder)
        default:
            return nil
        }
    }

    private static func encode<T: Encodable>(_ value: T, using encoder: JSONEncoder) -> String? {
        guard let data = try? encoder.encode(value) else { return nil }
        return String(data: data, encoding: .utf8)
    }

    private static func auditTuple(for action: UndoableAction) -> (String, String, String, [String: String]) {
        switch action {
        case .taskCompletion(let id, let prior, let title):
            return (
                prior ? "task.reopen" : "task.complete",
                id,
                title,
                ["priorCompleted": String(prior)]
            )
        case .taskDelete(let snap):
            return ("task.delete", snap.id, snap.title, ["list": snap.taskListID])
        case .taskEdit(let prior):
            return ("task.edit", prior.id, prior.title, ["list": prior.taskListID])
        case .eventDelete(let snap):
            return ("event.delete", snap.id, snap.summary, ["calendar": snap.calendarID])
        case .eventEdit(let prior):
            return ("event.edit", prior.id, prior.summary, ["calendar": prior.calendarID])
        case .taskCreate(let snap):
            return ("task.create", snap.id, snap.title, ["list": snap.taskListID])
        case .taskDuplicate(let snap, let sourceTitle):
            return ("task.duplicate", snap.id, snap.title, ["list": snap.taskListID, "sourceTitle": sourceTitle])
        case .taskMove(let id, let fromID, let toID, let title, let fromTitle, let toTitle):
            return ("task.move", id, title, [
                "fromListID": fromID,
                "toListID": toID,
                "fromListTitle": fromTitle,
                "toListTitle": toTitle
            ])
        case .eventCreate(let snap):
            return ("event.create", snap.id, snap.summary, ["calendar": snap.calendarID])
        case .eventDismissed(let snap):
            return ("event.dismiss", snap.id, snap.summary, ["calendar": snap.calendarID])
        case .clipboardOp(let kind, let resourceID, let title):
            return ("clipboard.\(kind)", resourceID, title, [:])
        case .taskRestore(let snap):
            return ("task.restore", snap.id, snap.title, ["list": snap.taskListID])
        case .eventRestore(let snap):
            return ("event.restore", snap.id, snap.summary, ["calendar": snap.calendarID])
        case .bulkAction(let kind, let count, let firstTitle):
            return ("bulk.\(kind)", "", firstTitle, ["count": String(count)])
        case .syncPulled(let kind, let count):
            return ("sync.\(kind)", "", "sync", ["count": String(count)])
        }
    }

    func clearUndo() {
        undoable = nil
    }

    func markSyncPaused() {
        isSyncPaused = true
        publishSessionSurfaceStores()
    }

    func resumeSync() {
        isSyncPaused = false
        publishSessionSurfaceStores()
    }

    // Drains the App Group's shared inbox (populated by the Share
    // Extension) and stashes the concatenated text for QuickAdd to
    // prefill. Returns true if anything was picked up so the caller
    // can decide whether to present the QuickAdd sheet.
    @discardableResult
    func consumePendingSharedItems() -> Bool {
        let items = SharedInboxDefaults.consumeAll()
        guard items.isEmpty == false else { return false }
        pendingSharedPrefill = items
            .map(\.text)
            .joined(separator: "\n")
        return true
    }

    // Drops a single PendingMutation by id so the user can unstick a
    // replay loop that keeps 412'ing (e.g. Google rejected a queued edit
    // because the item was changed elsewhere). Called from DiagnosticsView.
    @discardableResult
    func clearPendingMutation(id: PendingMutation.ID) -> Bool {
        guard let mutation = pendingMutations.first(where: { $0.id == id }) else { return false }
        pendingMutations.removeAll { $0.id == id }
        scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.pendingMutationDeleted(mutation))
        return true
    }

    func clearAllPendingMutations() {
        guard pendingMutations.isEmpty == false else { return }
        pendingMutations = []
        scheduleCacheSave()
    }

    func performUndo() async {
        guard let action = undoable else { return }
        undoable = nil
        switch action {
        case .taskCompletion(let id, let prior, _):
            guard let task = task(id: id) else { return }
            _ = await setTaskCompleted(prior, task: task)
        case .taskDelete(let snap):
            _ = await createTask(
                title: snap.title,
                notes: snap.notes,
                dueDate: snap.dueDate,
                taskListID: snap.taskListID,
                parentID: snap.parentID
            )
        case .taskEdit(let priorSnap):
            guard let task = task(id: priorSnap.id) else { return }
            _ = await updateTask(
                task,
                title: priorSnap.title,
                notes: priorSnap.notes,
                dueDate: priorSnap.dueDate
            )
        case .eventDelete(let snap):
            _ = await createEvent(
                summary: snap.summary,
                details: snap.details,
                startDate: snap.startDate,
                endDate: snap.endDate,
                isAllDay: snap.isAllDay,
                reminderMinutes: snap.reminderMinutes.first,
                calendarID: snap.calendarID,
                location: snap.location,
                recurrence: snap.recurrence,
                attendeeEmails: snap.attendeeEmails,
                notifyGuests: false,
                addGoogleMeet: false,
                colorId: snap.colorId,
                startTimeZoneID: snap.startTimeZoneID,
                endTimeZoneID: snap.endTimeZoneID,
                transparency: snap.transparency,
                visibility: snap.visibility,
                hcbTaskID: snap.hcbTaskID,
                availabilityHold: snap.availabilityHold
            )
        case .eventEdit(let priorSnap):
            guard let event = event(id: priorSnap.id) else { return }
            let inclusiveEnd = priorSnap.isAllDay
                ? (Calendar.current.date(byAdding: .day, value: -1, to: priorSnap.endDate) ?? priorSnap.endDate)
                : priorSnap.endDate
            _ = await updateEvent(
                event,
                summary: priorSnap.summary,
                details: priorSnap.details,
                startDate: priorSnap.startDate,
                endDate: inclusiveEnd,
                isAllDay: priorSnap.isAllDay,
                reminderMinutes: priorSnap.reminderMinutes.first,
                calendarID: priorSnap.calendarID,
                location: priorSnap.location,
                recurrence: priorSnap.recurrence,
                attendeeEmails: priorSnap.attendeeEmails,
                notifyGuests: false,
                addGoogleMeet: false,
                colorId: priorSnap.colorId,
                startTimeZoneID: priorSnap.startTimeZoneID,
                endTimeZoneID: priorSnap.endTimeZoneID,
                transparency: priorSnap.transparency,
                visibility: priorSnap.visibility,
                hcbTaskID: priorSnap.hcbTaskID,
                availabilityHold: priorSnap.availabilityHold
            )
        case .taskCreate(let snap):
            // undo create = delete the created task
            guard let task = task(id: snap.id) else { return }
            _ = await deleteTask(task)
        case .taskDuplicate(let snap, _):
            // undo duplicate = delete the new copy
            guard let task = task(id: snap.id) else { return }
            _ = await deleteTask(task)
        case .taskMove(let id, let fromID, _, _, _, _):
            // undo move = move back to fromListID. id is the finalTask.id from
            // moveTaskToList (the new Google-assigned ID on the destination
            // list). If the mirror no longer contains it (user deleted it,
            // sync dropped it), silently bail — better a no-op than moving
            // an unrelated task.
            guard let task = task(id: id) else { return }
            _ = await moveTaskToList(task, toTaskListID: fromID)
        case .eventCreate(let snap):
            guard let event = event(id: snap.id) else { return }
            _ = await deleteEvent(event)
        case .eventDismissed(let snap):
            // same inverse as eventDelete — Google has no "uncomplete" so the
            // event is recreated from the snapshot.
            _ = await createEvent(
                summary: snap.summary,
                details: snap.details,
                startDate: snap.startDate,
                endDate: snap.endDate,
                isAllDay: snap.isAllDay,
                reminderMinutes: snap.reminderMinutes.first,
                calendarID: snap.calendarID,
                location: snap.location,
                recurrence: snap.recurrence,
                attendeeEmails: snap.attendeeEmails,
                notifyGuests: false,
                addGoogleMeet: false,
                colorId: snap.colorId,
                startTimeZoneID: snap.startTimeZoneID,
                endTimeZoneID: snap.endTimeZoneID,
                transparency: snap.transparency,
                visibility: snap.visibility,
                hcbTaskID: snap.hcbTaskID,
                availabilityHold: snap.availabilityHold
            )
        case .clipboardOp, .bulkAction, .syncPulled, .taskRestore, .eventRestore:
            // not invertible from the short-TTL undo toast; history window handles these via snapshot copy
            return
        }
    }

    func deleteTask(_ task: TaskMirror) async -> Bool {
        guard requireAccount(mutationDescription: "deleting tasks") else {
            return false
        }
        // If the create is still pending, we can retract it locally: drop the
        // queued mutation and remove the optimistic row. No Google call needed.
        if OptimisticID.isPending(task.id) {
            let removedMutations = pendingMutations.filter { mutation in
                mutation.resourceType == .task
                    && mutation.action == .create
                    && mutation.resourceID == task.id
            }
            pendingMutations.removeAll { removedMutations.map(\.id).contains($0.id) }
            removeTask(id: task.id)
            lastMutationError = nil
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.combined(
                [LocalCacheChangeSetBuilder.taskDeleted(task)]
                    + removedMutations.map(LocalCacheChangeSetBuilder.pendingMutationDeleted)
            ))
            await synchronizeLocalNotifications()
            return true
        }

        let originalTask = task
        removeTask(id: task.id)

        guard settings.cloudSyncTargets.syncsTasks else {
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.taskDeleted(originalTask))
            await synchronizeLocalNotifications()
            recordUndo(.taskDelete(snapshot: originalTask))
            return true
        }

        beginMutation()
        do {
            try await tasksClient.deleteTask(taskListID: task.taskListID, taskID: task.id, ifMatch: task.etag)
            endMutation(error: nil)
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.taskDeleted(originalTask))
            await synchronizeLocalNotifications()
            recordUndo(.taskDelete(snapshot: originalTask))
            return true
        } catch let error as GoogleAPIError where error.isTransient {
            let payload = PendingTaskDeletePayload(
                taskListID: task.taskListID,
                taskID: task.id,
                etagSnapshot: task.etag
            )
            if let mutation = try? PendingMutation.taskDelete(payload: payload) {
                let queuedMutation = enqueuePendingMutation(mutation)
                scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.combined(
                    LocalCacheChangeSetBuilder.taskDeleted(originalTask),
                    LocalCacheChangeSetBuilder.pendingMutationInserted(queuedMutation)
                ))
                await synchronizeLocalNotifications()
                endMutation(error: nil)
                Task { await replayPendingMutations() }
                return true
            }
            upsert(originalTask)
            endMutation(error: error)
            return false
        } catch {
            upsert(originalTask)
            if let apiError = error as? GoogleAPIError, apiError == .preconditionFailed {
                await refreshNow()
            }
            endMutation(error: error)
            return false
        }
    }

    func createEvent(
        summary: String,
        details: String,
        startDate: Date,
        endDate: Date,
        isAllDay: Bool,
        reminderMinutes: Int?,
        calendarID: CalendarListMirror.ID,
        location: String = "",
        recurrence: [String] = [],
        attendeeEmails: [String] = [],
        notifyGuests: Bool = false,
        addGoogleMeet: Bool = false,
        colorId: String? = nil,
        startTimeZoneID: String? = nil,
        endTimeZoneID: String? = nil,
        transparency: CalendarEventTransparency = .opaque,
        visibility: CalendarEventVisibility = .defaultVisibility,
        hcbTaskID: String? = nil,
        availabilityHold: AvailabilityHoldMetadata? = nil,
        replayImmediately: Bool = true,
        recordUndoAction: Bool = true
    ) async -> Bool {
        guard requireAccount(mutationDescription: "creating events") else {
            return false
        }

        guard isValidEventRange(startDate: startDate, endDate: endDate, isAllDay: isAllDay) else {
            lastMutationError = isAllDay ? "All-day event end date cannot be before the start date." : "Event end time must be after the start time."
            return false
        }

        let trimmedSummary = summary.trimmingCharacters(in: .whitespacesAndNewlines)
        let trimmedDetails = details.trimmingCharacters(in: .whitespacesAndNewlines)
        let localID = OptimisticID.generate()
        let trimmedLocation = location.trimmingCharacters(in: .whitespacesAndNewlines)
        let cleanedEmails = attendeeEmails
            .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
            .filter { $0.isEmpty == false }
        let resolvedTimeZones = resolvedEventTimeZones(
            startTimeZoneID: startTimeZoneID,
            endTimeZoneID: endTimeZoneID,
            calendarID: calendarID,
            existingEvent: nil
        )
        let optimisticEvent = CalendarEventMirror(
            id: localID,
            calendarID: calendarID,
            summary: trimmedSummary,
            details: trimmedDetails,
            startDate: startDate,
            endDate: endDate,
            isAllDay: isAllDay,
            status: .confirmed,
            recurrence: recurrence,
            etag: nil,
            updatedAt: Date(),
            reminderMinutes: reminderMinutes.map { [$0] } ?? [],
            location: trimmedLocation,
            attendeeEmails: cleanedEmails,
            attendeeResponses: cleanedEmails.map {
                CalendarEventAttendee(email: $0, displayName: nil, responseStatus: .needsAction)
            },
            meetLink: "",
            colorId: colorId,
            startTimeZoneID: resolvedTimeZones.start,
            endTimeZoneID: resolvedTimeZones.end,
            transparency: transparency,
            visibility: visibility,
            hcbTaskID: hcbTaskID,
            availabilityHold: availabilityHold
        )
        upsert(optimisticEvent)

        guard settings.cloudSyncTargets.syncsEvents else {
            if recordUndoAction {
                recordUndo(.eventCreate(snapshot: optimisticEvent))
            }
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.eventInserted(optimisticEvent))
            await synchronizeLocalNotifications()
            return true
        }

        do {
            let payload = PendingEventCreatePayload(
                localID: localID,
                calendarID: calendarID,
                summary: trimmedSummary,
                details: trimmedDetails,
                startDate: startDate,
                endDate: endDate,
                isAllDay: isAllDay,
                reminderMinutes: reminderMinutes,
                location: trimmedLocation,
                recurrence: recurrence,
                attendeeEmails: cleanedEmails,
                notifyGuests: notifyGuests,
                addGoogleMeet: addGoogleMeet,
                colorId: colorId,
                startTimeZoneID: resolvedTimeZones.start,
                endTimeZoneID: resolvedTimeZones.end,
                transparency: transparency,
                visibility: visibility,
                hcbTaskID: hcbTaskID,
                availabilityHold: availabilityHold
            )
            let mutation = try PendingMutation.eventCreate(payload: payload)
            let queuedMutation = enqueuePendingMutation(mutation)
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.combined(
                LocalCacheChangeSetBuilder.eventInserted(optimisticEvent),
                LocalCacheChangeSetBuilder.pendingMutationInserted(queuedMutation)
            ))
        } catch {
            removeEvent(id: localID)
            lastMutationError = "Could not queue event for sync: \(error.localizedDescription)"
            return false
        }

        if recordUndoAction {
            recordUndo(.eventCreate(snapshot: optimisticEvent))
        }
        if replayImmediately {
            Task { await replayPendingMutations() }
        }
        return true
    }

    enum RecurringEventScope: Sendable {
        case thisOccurrence
        case thisAndFollowing
        case allInSeries
    }

    func updateEvent(
        _ event: CalendarEventMirror,
        summary: String,
        details: String,
        startDate: Date,
        endDate: Date,
        isAllDay: Bool,
        reminderMinutes: Int?,
        calendarID: CalendarListMirror.ID,
        location: String = "",
        recurrence: [String]? = nil,
        attendeeEmails: [String]? = nil,
        notifyGuests: Bool = false,
        scope: RecurringEventScope = .thisOccurrence,
        addGoogleMeet: Bool = false,
        colorId: String? = nil,
        startTimeZoneID: String? = nil,
        endTimeZoneID: String? = nil,
        transparency: CalendarEventTransparency? = nil,
        visibility: CalendarEventVisibility? = nil,
        hcbTaskID: String? = nil,
        availabilityHold: AvailabilityHoldMetadata? = nil,
        clearAvailabilityHoldMetadata: Bool = false
    ) async -> Bool {
        guard requireAccount(mutationDescription: "updating events") else {
            return false
        }
        if settings.cloudSyncTargets.syncsEvents {
            guard requirePersisted(event.id) else { return false }
        }

        let trimmedSummary = summary.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmedSummary.isEmpty == false else {
            lastMutationError = "Give the event a title before saving."
            return false
        }

        guard isValidEventRange(startDate: startDate, endDate: endDate, isAllDay: isAllDay) else {
            lastMutationError = isAllDay ? "All-day event end date cannot be before the start date." : "Event end time must be after the start time."
            return false
        }

        let trimmedDetails = details.trimmingCharacters(in: .whitespacesAndNewlines)
        let trimmedLocation = location.trimmingCharacters(in: .whitespacesAndNewlines)
        let cleanedEmails = (attendeeEmails ?? event.attendeeEmails)
            .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
            .filter { $0.isEmpty == false }
        let effectiveRecurrence = recurrence ?? event.recurrence
        let resolvedTimeZones = resolvedEventTimeZones(
            startTimeZoneID: startTimeZoneID,
            endTimeZoneID: endTimeZoneID,
            calendarID: calendarID,
            existingEvent: event
        )
        let effectiveTransparency = transparency ?? event.transparency
        let effectiveVisibility = visibility ?? event.visibility
        let effectiveAvailabilityHold = clearAvailabilityHoldMetadata ? nil : (availabilityHold ?? event.availabilityHold)

        // Optimistic update for the in-place (single-occurrence, same-calendar)
        // case. Cross-calendar moves and series-wide edits need server round-
        // trip semantics, so we keep the old state visible until confirmed.
        let originalEvent = event
        guard settings.cloudSyncTargets.syncsEvents else {
            var localEvent = event
            localEvent.calendarID = calendarID
            localEvent.summary = trimmedSummary
            localEvent.details = trimmedDetails
            localEvent.startDate = startDate
            localEvent.endDate = endDate
            localEvent.isAllDay = isAllDay
            localEvent.location = trimmedLocation
            localEvent.recurrence = effectiveRecurrence
            localEvent.attendeeEmails = cleanedEmails
            localEvent.reminderMinutes = reminderMinutes.map { [$0] } ?? []
            localEvent.colorId = colorId
            localEvent.startTimeZoneID = resolvedTimeZones.start
            localEvent.endTimeZoneID = resolvedTimeZones.end
            localEvent.transparency = effectiveTransparency
            localEvent.visibility = effectiveVisibility
            localEvent.hcbTaskID = hcbTaskID ?? event.hcbTaskID
            localEvent.availabilityHold = effectiveAvailabilityHold
            upsert(localEvent)
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.eventUpdated(old: originalEvent, new: localEvent))
            await synchronizeLocalNotifications()
            recordUndo(.eventEdit(priorSnapshot: originalEvent))
            return true
        }

        let canQueue = scope == .thisOccurrence && calendarID == event.calendarID
        var queuedOptimisticEvent: CalendarEventMirror?
        if canQueue {
            var optimistic = event
            optimistic.summary = trimmedSummary
            optimistic.details = trimmedDetails
            optimistic.startDate = startDate
            optimistic.endDate = endDate
            optimistic.isAllDay = isAllDay
            optimistic.location = trimmedLocation
            optimistic.recurrence = effectiveRecurrence
            optimistic.attendeeEmails = cleanedEmails
            optimistic.reminderMinutes = reminderMinutes.map { [$0] } ?? []
            optimistic.startTimeZoneID = resolvedTimeZones.start
            optimistic.endTimeZoneID = resolvedTimeZones.end
            optimistic.colorId = colorId
            optimistic.transparency = effectiveTransparency
            optimistic.visibility = effectiveVisibility
            optimistic.hcbTaskID = hcbTaskID ?? event.hcbTaskID
            optimistic.availabilityHold = effectiveAvailabilityHold
            upsert(optimistic)
            queuedOptimisticEvent = optimistic
        }

        beginMutation()
        do {
            let eventToUpdate: CalendarEventMirror
            if calendarID != event.calendarID {
                // Cross-calendar moves on a recurring event need to use the
                // series ID for .allInSeries scope — Google treats a move of
                // the instance ID as a single-occurrence override with a
                // different parent calendar, which leaves the rest of the
                // series orphaned on the source calendar.
                let sourceID = scope == .allInSeries
                    ? CalendarEventInstance.seriesID(from: event.id)
                    : event.id
                eventToUpdate = try await calendarClient.moveEvent(
                    calendarID: event.calendarID,
                    eventID: sourceID,
                    destinationCalendarID: calendarID
                )
            } else {
                eventToUpdate = event
            }

            let targetEventID = scope == .allInSeries
                ? CalendarEventInstance.seriesID(from: eventToUpdate.id)
                : eventToUpdate.id

            // Preserve any existing hcb backlink on the event unless the caller
            // explicitly supplied a new one. Without this, every update would
            // clear extendedProperties.private.hcbTaskID (Google Calendar
            // treats an omitted field on PATCH as "leave as-is" for the outer
            // bag — but we pass a dict, so we need to echo it back ourselves).
            let effectiveHCBTaskID = hcbTaskID ?? event.hcbTaskID
            let updateAvailabilityHold = clearAvailabilityHoldMetadata ? nil : (availabilityHold ?? event.availabilityHold)

            let updatedEvent = try await calendarClient.updateEvent(
                calendarID: eventToUpdate.calendarID,
                eventID: targetEventID,
                summary: trimmedSummary,
                details: trimmedDetails,
                startDate: startDate,
                endDate: endDate,
                isAllDay: isAllDay,
                reminderMinutes: reminderMinutes,
                location: trimmedLocation,
                recurrence: effectiveRecurrence,
                attendeeEmails: cleanedEmails,
                sendUpdates: notifyGuests ? "all" : "none",
                addGoogleMeet: addGoogleMeet,
                colorId: colorId,
                startTimeZoneID: resolvedTimeZones.start,
                endTimeZoneID: resolvedTimeZones.end,
                hcbTaskID: effectiveHCBTaskID,
                transparency: effectiveTransparency,
                visibility: effectiveVisibility,
                availabilityHold: updateAvailabilityHold,
                clearAvailabilityHoldMetadata: clearAvailabilityHoldMetadata,
                ifMatch: scope == .allInSeries ? nil : eventToUpdate.etag
            )
            if calendarID != event.calendarID {
                removeEvent(id: event.id)
            }
            upsert(updatedEvent)
            endMutation(error: nil)
            if scope == .thisOccurrence {
                scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.eventUpdated(old: originalEvent, new: updatedEvent))
            } else {
                scheduleCacheSave()
            }
            await synchronizeLocalNotifications()
            if canQueue { recordUndo(.eventEdit(priorSnapshot: originalEvent)) }
            return true
        } catch let error as GoogleAPIError where error.isTransient && canQueue {
            let payload = PendingEventUpdatePayload(
                calendarID: event.calendarID,
                eventID: event.id,
                summary: trimmedSummary,
                details: trimmedDetails,
                startDate: startDate,
                endDate: endDate,
                isAllDay: isAllDay,
                reminderMinutes: reminderMinutes,
                location: trimmedLocation,
                recurrence: effectiveRecurrence,
                attendeeEmails: cleanedEmails,
                notifyGuests: notifyGuests,
                etagSnapshot: event.etag,
                addGoogleMeet: addGoogleMeet,
                colorId: colorId,
                startTimeZoneID: resolvedTimeZones.start,
                endTimeZoneID: resolvedTimeZones.end,
                hcbTaskID: hcbTaskID ?? event.hcbTaskID,
                transparency: effectiveTransparency,
                visibility: effectiveVisibility,
                availabilityHold: effectiveAvailabilityHold,
                clearAvailabilityHoldMetadata: clearAvailabilityHoldMetadata
            )
            if let mutation = try? PendingMutation.eventUpdate(payload: payload) {
                let queuedMutation = enqueuePendingMutation(mutation)
                scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.combined(
                    LocalCacheChangeSetBuilder.eventUpdated(old: originalEvent, new: queuedOptimisticEvent ?? originalEvent),
                    LocalCacheChangeSetBuilder.pendingMutationInserted(queuedMutation)
                ))
                await synchronizeLocalNotifications()
                endMutation(error: nil)
                Task { await replayPendingMutations() }
                return true
            }
            upsert(originalEvent)
            endMutation(error: error)
            return false
        } catch {
            if canQueue { upsert(originalEvent) }
            if let apiError = error as? GoogleAPIError, apiError == .preconditionFailed {
                await refreshNow()
            }
            endMutation(error: error)
            return false
        }
    }

    // Deletes the event from Google (since Calendar has no completion state)
    // but records the undoable as .eventDismissed so the user can distinguish
    // "marked done" from "deleted" in the history log + undo toast. Always
    // .thisOccurrence scope — dismissing a recurring master doesn't make
    // sense (you finish individual occurrences, not the whole series).
    func dismissEvent(_ event: CalendarEventMirror) async -> Bool {
        guard requireAccount(mutationDescription: "marking events done") else {
            return false
        }
        // Pending creates still in the outbox — retract locally, no Google call.
        if OptimisticID.isPending(event.id) {
            let removedMutations = pendingMutations.filter { mutation in
                mutation.resourceType == .event
                    && mutation.action == .create
                    && mutation.resourceID == event.id
            }
            let removedMutationIDs = Set(removedMutations.map(\.id))
            pendingMutations.removeAll { removedMutationIDs.contains($0.id) }
            removeEvent(id: event.id)
            lastMutationError = nil
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.combined(
                [LocalCacheChangeSetBuilder.eventDeleted(event)]
                    + removedMutations.map(LocalCacheChangeSetBuilder.pendingMutationDeleted)
            ))
            await synchronizeLocalNotifications()
            recordUndo(.eventDismissed(snapshot: event))
            return true
        }

        let originalEvent = event
        removeEvent(id: event.id)
        // Record the undoable immediately (optimistic) so the toast fires
        // without waiting on Google's 200.
        recordUndo(.eventDismissed(snapshot: originalEvent))
        CompletionSoundPlayer.play(.eventDismissed, settings: settings)
        guard settings.cloudSyncTargets.syncsEvents else {
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.eventDeleted(originalEvent))
            await synchronizeLocalNotifications()
            return true
        }

        beginMutation()
        do {
            let targetID = event.id
            try await calendarClient.deleteEvent(calendarID: event.calendarID, eventID: targetID, ifMatch: event.etag)
            endMutation(error: nil)
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.eventDeleted(originalEvent))
            await synchronizeLocalNotifications()
            return true
        } catch let error as GoogleAPIError where error.isTransient {
            // Queue for retry — same pattern as deleteEvent.
            let payload = PendingEventDeletePayload(
                calendarID: event.calendarID,
                eventID: event.id,
                etagSnapshot: event.etag
            )
            if let mutation = try? PendingMutation.eventDelete(payload: payload) {
                let queuedMutation = enqueuePendingMutation(mutation)
                scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.combined(
                    LocalCacheChangeSetBuilder.eventDeleted(originalEvent),
                    LocalCacheChangeSetBuilder.pendingMutationInserted(queuedMutation)
                ))
                await synchronizeLocalNotifications()
                endMutation(error: nil)
                Task { await replayPendingMutations() }
                return true
            }
            upsert(originalEvent)
            endMutation(error: error)
            return false
        } catch {
            upsert(originalEvent)
            endMutation(error: error)
            return false
        }
    }

    func deleteEvent(_ event: CalendarEventMirror, scope: RecurringEventScope = .thisOccurrence) async -> Bool {
        guard requireAccount(mutationDescription: "deleting events") else {
            return false
        }
        // Retract a still-pending event create locally without calling Google.
        if OptimisticID.isPending(event.id) {
            let removedMutations = pendingMutations.filter { mutation in
                mutation.resourceType == .event
                    && mutation.action == .create
                    && mutation.resourceID == event.id
            }
            let removedMutationIDs = Set(removedMutations.map(\.id))
            pendingMutations.removeAll { removedMutationIDs.contains($0.id) }
            removeEvent(id: event.id)
            lastMutationError = nil
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.combined(
                [LocalCacheChangeSetBuilder.eventDeleted(event)]
                    + removedMutations.map(LocalCacheChangeSetBuilder.pendingMutationDeleted)
            ))
            await synchronizeLocalNotifications()
            return true
        }

        let originalEvent = event
        let canQueue = scope == .thisOccurrence
        if canQueue {
            removeEvent(id: event.id)
        }

        guard settings.cloudSyncTargets.syncsEvents else {
            switch scope {
            case .thisOccurrence:
                if canQueue == false {
                    removeEvent(id: event.id)
                }
            case .thisAndFollowing:
                let seriesID = CalendarEventInstance.seriesID(from: event.id)
                events.removeAll {
                    CalendarEventInstance.seriesID(from: $0.id) == seriesID
                        && $0.startDate >= event.startDate
                }
                scheduleRebuildSnapshots()
            case .allInSeries:
                let seriesID = CalendarEventInstance.seriesID(from: event.id)
                events.removeAll { CalendarEventInstance.seriesID(from: $0.id) == seriesID }
                scheduleRebuildSnapshots()
            }
            if scope == .thisOccurrence {
                scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.eventDeleted(originalEvent))
            } else {
                scheduleCacheSave()
            }
            await synchronizeLocalNotifications()
            if canQueue { recordUndo(.eventDelete(snapshot: originalEvent)) }
            return true
        }

        beginMutation()
        do {
            if scope == .thisAndFollowing {
                // Fetch the master so we have its current RRULE(s), then rewrite
                // each with an UNTIL clause that falls before this instance —
                // the instance itself and every future one disappear from the
                // series on the next sync. No delete verb is issued: truncation
                // alone removes future occurrences.
                let seriesID = CalendarEventInstance.seriesID(from: event.id)
                let master = try await calendarClient.getEvent(calendarID: event.calendarID, eventID: seriesID)
                let cutoff = Calendar.current.date(byAdding: .second, value: -1, to: event.startDate) ?? event.startDate
                let untilString = RecurrenceUntilRewriter.untilString(fromCutoff: cutoff, isAllDay: master.isAllDay)
                let rewritten = master.recurrence.map { RecurrenceUntilRewriter.rewrite(rrule: $0, until: untilString) }
                _ = try await calendarClient.patchEventRecurrence(
                    calendarID: event.calendarID,
                    eventID: seriesID,
                    recurrence: rewritten,
                    ifMatch: master.etag
                )
                // Drop the instance and every later instance of the same series
                // from the local mirror so the grid updates before the next
                // sync reconciles.
                let dropID = seriesID
                events.removeAll { existing in
                    guard CalendarEventInstance.seriesID(from: existing.id) == dropID else { return false }
                    return existing.startDate >= event.startDate
                }
                scheduleRebuildSnapshots()
                endMutation(error: nil)
                scheduleCacheSave()
                await synchronizeLocalNotifications()
                recordUndo(.eventDelete(snapshot: originalEvent))
                return true
            }
            let targetEventID = scope == .allInSeries
                ? CalendarEventInstance.seriesID(from: event.id)
                : event.id
            let ifMatch: String? = scope == .allInSeries ? nil : event.etag
            try await calendarClient.deleteEvent(
                calendarID: event.calendarID,
                eventID: targetEventID,
                ifMatch: ifMatch
            )
            if scope == .allInSeries {
                let seriesID = CalendarEventInstance.seriesID(from: event.id)
                events.removeAll { CalendarEventInstance.seriesID(from: $0.id) == seriesID }
                scheduleRebuildSnapshots()
            }
            endMutation(error: nil)
            if scope == .thisOccurrence {
                scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.eventDeleted(originalEvent))
            } else {
                scheduleCacheSave()
            }
            await synchronizeLocalNotifications()
            if canQueue { recordUndo(.eventDelete(snapshot: originalEvent)) }
            return true
        } catch let error as GoogleAPIError where error.isTransient && canQueue {
            let payload = PendingEventDeletePayload(
                calendarID: event.calendarID,
                eventID: event.id,
                etagSnapshot: event.etag
            )
            if let mutation = try? PendingMutation.eventDelete(payload: payload) {
                let queuedMutation = enqueuePendingMutation(mutation)
                scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.combined(
                    LocalCacheChangeSetBuilder.eventDeleted(originalEvent),
                    LocalCacheChangeSetBuilder.pendingMutationInserted(queuedMutation)
                ))
                await synchronizeLocalNotifications()
                endMutation(error: nil)
                Task { await replayPendingMutations() }
                return true
            }
            upsert(originalEvent)
            endMutation(error: error)
            return false
        } catch {
            if canQueue { upsert(originalEvent) }
            if let apiError = error as? GoogleAPIError, apiError == .preconditionFailed {
                await refreshNow()
            }
            endMutation(error: error)
            return false
        }
    }

    func createAvailabilityHoldGroup(
        title: String,
        durationMinutes: Int,
        slots: [AvailabilitySlot],
        calendarID: CalendarListMirror.ID,
        timeZoneID: String
    ) async -> Bool {
        let trimmedTitle = title.trimmingCharacters(in: .whitespacesAndNewlines)
        let resolvedTitle = trimmedTitle.isEmpty ? "Meeting" : trimmedTitle
        let sortedSlots = AvailabilitySlotResolver.normalized(slots)
        guard sortedSlots.isEmpty == false else {
            lastMutationError = "Select at least one available slot before creating holds."
            return false
        }
        guard sortedSlots.count <= AvailabilityHoldLimits.maxSlotsPerGroup else {
            lastMutationError = "Create up to \(AvailabilityHoldLimits.maxSlotsPerGroup) availability holds at a time."
            return false
        }

        let groupID = UUID().uuidString
        let createdAt = Date()
        var createdCount = 0
        for slot in sortedSlots {
            let metadata = AvailabilityHoldMetadata(
                groupID: groupID,
                title: resolvedTitle,
                durationMinutes: durationMinutes,
                createdAt: createdAt
            )
            let didCreate = await createEvent(
                summary: "Hold: \(resolvedTitle)",
                details: AvailabilitySnippetFormatter.holdDetails(
                    title: resolvedTitle,
                    durationMinutes: durationMinutes,
                    timeZoneID: timeZoneID,
                    slot: slot
                ),
                startDate: slot.startDate,
                endDate: slot.endDate,
                isAllDay: false,
                reminderMinutes: nil,
                calendarID: calendarID,
                notifyGuests: false,
                startTimeZoneID: timeZoneID,
                endTimeZoneID: timeZoneID,
                transparency: .opaque,
                visibility: .privateVisibility,
                availabilityHold: metadata,
                replayImmediately: false,
                recordUndoAction: false
            )
            guard didCreate else {
                if createdCount > 0 {
                    lastMutationError = "Created \(createdCount) of \(sortedSlots.count) holds. Review pending holds before sharing."
                    if settings.cloudSyncTargets.syncsEvents {
                        Task { await replayPendingMutations() }
                    }
                }
                return false
            }
            createdCount += 1
        }

        if settings.cloudSyncTargets.syncsEvents {
            Task { await replayPendingMutations() }
        }

        return true
    }

    func confirmAvailabilityHold(_ hold: CalendarEventMirror) async -> Bool {
        guard let metadata = hold.availabilityHold else {
            lastMutationError = "This calendar event is not an availability hold."
            return false
        }

        let didConfirm = await updateEvent(
            hold,
            summary: metadata.title,
            details: "",
            startDate: hold.startDate,
            endDate: hold.endDate,
            isAllDay: false,
            reminderMinutes: hold.reminderMinutes.first,
            calendarID: hold.calendarID,
            location: hold.location,
            recurrence: [],
            attendeeEmails: [],
            notifyGuests: false,
            addGoogleMeet: false,
            colorId: hold.colorId,
            startTimeZoneID: hold.startTimeZoneID,
            endTimeZoneID: hold.endTimeZoneID,
            transparency: .opaque,
            visibility: .defaultVisibility,
            hcbTaskID: hold.hcbTaskID,
            clearAvailabilityHoldMetadata: true
        )
        guard didConfirm else { return false }

        let siblings = availabilityHolds(groupID: metadata.groupID).filter { $0.id != hold.id }
        var allSiblingsDeleted = true
        for sibling in siblings {
            let didDelete = await deleteEvent(sibling)
            allSiblingsDeleted = allSiblingsDeleted && didDelete
        }
        if allSiblingsDeleted == false {
            lastMutationError = "Confirmed the selected hold, but one or more alternate holds could not be removed."
            return false
        }
        return true
    }

    @discardableResult
    func cancelAvailabilityHoldGroup(groupID: String) async -> Bool {
        let holds = availabilityHolds(groupID: groupID)
        guard holds.isEmpty == false else {
            lastMutationError = "No availability holds were found for this group."
            return false
        }
        var allDeleted = true
        for hold in holds {
            let didDelete = await deleteEvent(hold)
            allDeleted = allDeleted && didDelete
        }
        if allDeleted == false {
            lastMutationError = "One or more availability holds could not be removed."
        }
        return allDeleted
    }

    func availabilityHolds(groupID: String) -> [CalendarEventMirror] {
        events
            .filter { event in
                event.status != .cancelled
                    && event.availabilityHold?.groupID == groupID
            }
            .sorted { lhs, rhs in lhs.startDate < rhs.startDate }
    }

    func availabilityHoldGroupsFromQuery() async -> [AvailabilityHoldGroup] {
        do {
            return try await calendarQueryService.availabilityHoldGroups(accountID: activeAccountID)
        } catch {
            HCBPerformanceTelemetry.debug(
                "availability hold query fallback",
                metadata: ["error": String(describing: error)]
            )
            return Self.availabilityHoldGroups(from: events)
        }
    }

    private static func availabilityHoldGroups(from events: [CalendarEventMirror]) -> [AvailabilityHoldGroup] {
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

    func overdueTasksFromQuery(
        before cutoff: Date,
        visibleTaskListIDs requestedVisibleTaskListIDs: Set<TaskListMirror.ID>? = nil,
        limit: Int = ActionCenterBuilder.defaultOverdueTaskDisplayLimit
    ) async -> OverdueTaskProjection {
        do {
            return try await cacheStore.overdueTasks(
                before: cutoff,
                visibleTaskListIDs: requestedVisibleTaskListIDs,
                limit: limit
            )
        } catch {
            HCBPerformanceTelemetry.debug(
                "overdue task query fallback",
                metadata: ["error": String(describing: error)]
            )
            return Self.overdueTaskProjection(
                from: tasks,
                before: cutoff,
                visibleTaskListIDs: requestedVisibleTaskListIDs,
                limit: limit
            )
        }
    }

    func todayPrintPayloadFromQuery(now: Date = Date()) async -> TodayPrintPayload {
        let calendar = Calendar.current
        let horizon = calendar.date(byAdding: .day, value: 1, to: now) ?? now
        let selectedCalendarIDs = Set(calendarSnapshot.selectedCalendars.map(\.id))
        let overdue = await overdueTasksFromQuery(
            before: calendar.startOfDay(for: now),
            visibleTaskListIDs: nil,
            limit: -1
        ).tasks

        let upcoming: [CalendarEventMirror]
        do {
            upcoming = try await calendarQueryService.events(
                in: DateInterval(start: now, end: horizon),
                accountID: activeAccountID,
                calendarIDs: selectedCalendarIDs,
                eventViewFilter: CalendarEventViewFilter(),
                includeCancelled: false
            )
            .filter { event in
                event.startDate > now
                    && event.startDate <= horizon
            }
            .sorted { lhs, rhs in
                if lhs.startDate != rhs.startDate { return lhs.startDate < rhs.startDate }
                return lhs.id < rhs.id
            }
        } catch {
            HCBPerformanceTelemetry.debug(
                "today print event query fallback",
                metadata: ["error": String(describing: error)]
            )
            upcoming = events
                .filter { event in
                    event.status != .cancelled
                        && selectedCalendarIDs.contains(event.calendarID)
                        && event.startDate > now
                        && event.startDate <= horizon
                }
                .sorted { lhs, rhs in
                    if lhs.startDate != rhs.startDate { return lhs.startDate < rhs.startDate }
                    return lhs.id < rhs.id
                }
        }

        return TodayPrintPayload(
            todaySnapshot: todaySnapshot,
            overdueTasks: overdue,
            upcomingEvents: upcoming,
            generatedAt: now
        )
    }

    private static func overdueTaskProjection(
        from tasks: [TaskMirror],
        before cutoff: Date,
        visibleTaskListIDs: Set<TaskListMirror.ID>?,
        limit: Int
    ) -> OverdueTaskProjection {
        var overdue = tasks.filter { task in
            task.isCompleted == false
                && task.isDeleted == false
                && task.isHidden == false
                && (visibleTaskListIDs?.contains(task.taskListID) ?? true)
                && (task.dueDate.map { $0 < cutoff } ?? false)
        }
        overdue.sort { lhs, rhs in
            let left = lhs.dueDate ?? .distantPast
            let right = rhs.dueDate ?? .distantPast
            if left != right { return left < right }
            let titleOrder = lhs.title.localizedCaseInsensitiveCompare(rhs.title)
            if titleOrder != .orderedSame { return titleOrder == .orderedAscending }
            return lhs.id < rhs.id
        }
        let limited = limit < 0 ? overdue : Array(overdue.prefix(limit))
        return OverdueTaskProjection(tasks: limited, totalCount: overdue.count)
    }

    func updateSyncMode(_ mode: SyncMode) {
        settings.syncMode = mode
        Task {
            scheduleCacheSave()
        }
    }

    func setAppLanguage(_ language: AppLanguage) {
        guard settings.appLanguage != language else { return }
        settings.appLanguage = language
        scheduleCacheSave()
    }

    func setCloudSyncTarget(_ target: CloudSyncTarget, enabled: Bool) {
        var targets = settings.cloudSyncTargets
        if enabled {
            targets.insert(target)
        } else {
            targets.remove(target)
        }
        guard targets != settings.cloudSyncTargets else { return }
        settings.cloudSyncTargets = targets
        scheduleCacheSave()

        if enabled {
            Task {
                await replayPendingMutations()
                await refreshNow()
            }
        }
    }

    func setShowMenuBarExtra(_ isEnabled: Bool) {
        guard settings.showMenuBarExtra != isEnabled else {
            return
        }
        settings.showMenuBarExtra = isEnabled
        scheduleCacheSave()
    }

    func setShowDockBadge(_ isEnabled: Bool) {
        guard settings.showDockBadge != isEnabled else {
            return
        }
        settings.showDockBadge = isEnabled
        scheduleCacheSave()
    }

    func setShowMenuBarBadge(_ isEnabled: Bool) {
        guard settings.showMenuBarBadge != isEnabled else {
            return
        }
        settings.showMenuBarBadge = isEnabled
        scheduleCacheSave()
    }

    func setMenuBarIcon(_ icon: AppSettings.MenuBarIcon) {
        guard settings.menuBarIcon != icon else {
            return
        }
        settings.menuBarIcon = icon
        scheduleCacheSave()
    }

    func setEnableGlobalHotkey(_ isEnabled: Bool) {
        guard settings.enableGlobalHotkey != isEnabled else { return }
        settings.enableGlobalHotkey = isEnabled
        scheduleCacheSave()
    }

    func setGlobalHotkeyBinding(_ binding: GlobalHotkeyBinding) {
        guard settings.globalHotkeyBinding != binding else { return }
        settings.globalHotkeyBinding = binding
        scheduleCacheSave()
    }

    func setGlobalHotkeyRegistrationState(_ state: GlobalHotkeyRegistrationState) {
        globalHotkeyRegistrationState = state
    }

    // History-log + duplicate-detection setters. Kept clustered for discoverability.

    func setHistoryVisibleLimit(_ limit: Int) {
        let clamped = max(1, min(MutationAuditLog.absoluteCeiling, limit))
        guard settings.historyVisibleLimit != clamped else { return }
        settings.historyVisibleLimit = clamped
        scheduleCacheSave()
    }

    func setHistoryStorageCap(_ cap: Int) {
        let clamped = max(1, min(MutationAuditLog.absoluteCeiling, cap))
        guard settings.historyStorageCap != clamped else { return }
        settings.historyStorageCap = clamped
        scheduleCacheSave()
        Task { await MutationAuditLog.shared.setRetentionLimit(clamped) }
    }

    func setHistoryCategoryEnabled(_ category: String, enabled: Bool) {
        var set = settings.historyCategoryFilters
        if enabled { set.insert(category) } else { set.remove(category) }
        guard settings.historyCategoryFilters != set else { return }
        settings.historyCategoryFilters = set
        scheduleCacheSave()
    }

    func dismissDuplicateGroup(_ groupKey: String) {
        var set = settings.dismissedDuplicateGroups
        set.insert(groupKey)
        guard settings.dismissedDuplicateGroups != set else { return }
        settings.dismissedDuplicateGroups = set
        scheduleCacheSave()
        rebuildDuplicateIndex()
    }

    func dismissDuplicateGroupContaining(_ taskID: TaskMirror.ID) {
        guard let key = duplicateIndex.groupKey(for: taskID) else { return }
        dismissDuplicateGroup(key)
    }

    func restoreDuplicateGroup(_ groupKey: String) {
        var set = settings.dismissedDuplicateGroups
        set.remove(groupKey)
        guard settings.dismissedDuplicateGroups != set else { return }
        settings.dismissedDuplicateGroups = set
        scheduleCacheSave()
        rebuildDuplicateIndex()
    }

    func clearAllDuplicateDismissals() {
        guard settings.dismissedDuplicateGroups.isEmpty == false else { return }
        settings.dismissedDuplicateGroups = []
        scheduleCacheSave()
        rebuildDuplicateIndex()
    }

    // §7.02 — event retention window. Clamped to a sane [0, 3650] range so a
    // corrupt cache can't request a 100-year cutoff. 0 = keep-forever.
    func setEventRetentionDaysBack(_ days: Int) {
        let clamped = max(0, min(days, 3650))
        guard settings.eventRetentionDaysBack != clamped else { return }
        settings.eventRetentionDaysBack = clamped
        // Apply in-memory immediately so users see the cache shrink without
        // waiting for the next sync tick. Pruning mirrors the SyncScheduler
        // logic — same cutoff semantics, same preserve-recent-edits carveout.
        if clamped > 0 {
            let cutoff = Calendar.current.date(byAdding: .day, value: -clamped, to: Date())
            if let cutoff {
                events = events.filter { event in
                    if OptimisticID.isPending(event.id) { return true }
                    if event.endDate >= cutoff { return true }
                    if let updatedAt = event.updatedAt, updatedAt >= cutoff { return true }
                    return false
                }
            }
        }
        scheduleRebuildSnapshots()
        scheduleCacheSave()
    }

    func setCompletedTaskRetentionDaysBack(_ days: Int) {
        let clamped = max(0, min(days, 3650))
        guard settings.completedTaskRetentionDaysBack != clamped else { return }
        settings.completedTaskRetentionDaysBack = clamped
        if clamped > 0,
           let cutoff = Calendar.current.date(byAdding: .day, value: -clamped, to: Date()) {
            tasks = tasks.filter { task in
                if OptimisticID.isPending(task.id) { return true }
                guard task.isCompleted else { return true }
                if let completedAt = task.completedAt, completedAt >= cutoff { return true }
                if let updatedAt = task.updatedAt, updatedAt >= cutoff { return true }
                return false
            }
        }
        scheduleRebuildSnapshots()
        scheduleCacheSave()
    }

    func setSidebarItemHidden(_ item: SidebarItem, hidden: Bool) {
        guard item.isHideable else { return }
        var next = settings.hiddenSidebarItems
        if hidden {
            let wouldRemainVisible = SidebarItem.allCases.contains { other in
                other != item && next.contains(other.rawValue) == false
            }
            guard wouldRemainVisible else { return }
            next.insert(item.rawValue)
        } else {
            next.remove(item.rawValue)
        }
        guard next != settings.hiddenSidebarItems else { return }
        settings.hiddenSidebarItems = next
        scheduleCacheSave()
    }

    func setSidebarPlacement(_ placement: NavigationSurfacePlacement) {
        guard settings.sidebarPlacement != placement else { return }
        settings.sidebarPlacement = placement
        scheduleCacheSave()
    }

    func setCalendarViewModeHidden(_ mode: CalendarGridMode, hidden: Bool) {
        var next = settings.hiddenCalendarViewModes
        if hidden {
            // never hide every mode — Calendar tab needs at least one visible
            let wouldRemainVisible = CalendarGridMode.allCases.contains { other in
                other != mode && next.contains(other.rawValue) == false
            }
            guard wouldRemainVisible else { return }
            next.insert(mode.rawValue)
        } else {
            next.remove(mode.rawValue)
        }
        guard next != settings.hiddenCalendarViewModes else { return }
        settings.hiddenCalendarViewModes = next
        scheduleCacheSave()
    }

    // §6.13 — Task templates. Stored locally only; never written to Google.
    func upsertTaskTemplate(_ template: TaskTemplate) {
        if let index = settings.taskTemplates.firstIndex(where: { $0.id == template.id }) {
            settings.taskTemplates[index] = template
        } else {
            settings.taskTemplates.append(template)
        }
        scheduleCacheSave()
    }

    func deleteTaskTemplate(_ id: UUID) {
        settings.taskTemplates.removeAll { $0.id == id }
        scheduleCacheSave()
    }

    @discardableResult
    func duplicateTaskTemplate(_ template: TaskTemplate) -> TaskTemplate {
        let copy = template.duplicated()
        upsertTaskTemplate(copy)
        return copy
    }

    func markTaskTemplateUsed(_ id: TaskTemplate.ID) {
        guard let index = settings.taskTemplates.firstIndex(where: { $0.id == id }) else { return }
        settings.taskTemplates[index].lastUsedAt = Date()
        settings.taskTemplates[index].useCount += 1
        scheduleCacheSave()
    }

    // Instantiates a task template: expands every field with the given
    // variable context, then creates a real Google Task via the existing
    // createTask path. Returns true on success. `prompts` maps the label of
    // each {{prompt:Label}} placeholder to the user's typed answer.
    @discardableResult
    func instantiateTaskTemplate(_ template: TaskTemplate, prompts: [String: String] = [:]) async -> Bool {
        let ctx = HCBTemplateContext(
            now: Date(),
            calendar: .current,
            clipboard: NSPasteboard.general.string(forType: .string),
            prompts: prompts
        )
        let title = HCBTemplateExpander.expand(template.title, context: ctx)
            .replacingOccurrences(of: HCBTemplateExpander.cursorSentinel, with: "")
        let notes = HCBTemplateExpander.expand(template.notes, context: ctx)
            .replacingOccurrences(of: HCBTemplateExpander.cursorSentinel, with: "")
        let dueString = HCBTemplateExpander.expand(template.due, context: ctx)
            .replacingOccurrences(of: HCBTemplateExpander.cursorSentinel, with: "")
            .trimmingCharacters(in: .whitespacesAndNewlines)
        let listRef = HCBTemplateExpander.expand(template.listIdOrTitle, context: ctx)
            .trimmingCharacters(in: .whitespacesAndNewlines)

        let dueDate: Date? = {
            guard dueString.isEmpty == false else { return nil }
            let f = DateFormatter()
            f.locale = Locale(identifier: "en_US_POSIX")
            f.dateFormat = "yyyy-MM-dd"
            return f.date(from: dueString).map { Calendar.current.startOfDay(for: $0) }
        }()

        let resolvedListID: String? = {
            if listRef.isEmpty { return taskLists.first?.id }
            if let exact = taskLists.first(where: { $0.id == listRef }) { return exact.id }
            return taskLists.first(where: { $0.title.localizedCaseInsensitiveCompare(listRef) == .orderedSame })?.id
        }()

        guard let listID = resolvedListID else {
            lastMutationError = "No task list available for template '\(template.name)'."
            return false
        }

        let created = await createTask(
            title: title,
            notes: notes,
            dueDate: dueDate,
            taskListID: listID
        )
        if created {
            markTaskTemplateUsed(template.id)
        }
        return created
    }

    // §6.12 — Cache encryption lifecycle. Enabling derives a key from the
    // user's passphrase + a fresh salt, encrypts the current cache, and
    // persists the key in Keychain so future launches don't re-prompt.
    // Returns true on success; false + sets lastMutationError on failure.
    @discardableResult
    func enableCacheEncryption(passphrase: String) async -> Bool {
        let trimmed = passphrase.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false else {
            lastMutationError = "Passphrase is empty."
            return false
        }
        let salt = await cacheStore.ensureSalt()
        do {
            let key = try HCBCacheCrypto.deriveKey(passphrase: trimmed, salt: salt)
            try HCBCacheKeychain.save(key)
            await cacheStore.setEncryptionKey(key)
            settings.cacheEncryptionEnabled = true
            settings.auditLogEncryptionEnabled = true
            await MutationAuditLog.shared.configureEncryption(enabled: true, key: key, salt: salt)
            scheduleCacheSave() // triggers a save that will now encrypt
            return true
        } catch {
            lastMutationError = "Could not enable encryption: \(error)"
            HCBCacheKeychain.clear()
            await cacheStore.setEncryptionKey(nil)
            return false
        }
    }

    // Clears encryption, rewriting the cache as plaintext and removing the
    // salt sidecar so no half-encrypted state lingers. Requires the current
    // passphrase so a stolen laptop can't disable encryption by just flipping
    // the Settings toggle — the verify step confirms the caller knows the key.
    @discardableResult
    func disableCacheEncryption(currentPassphrase: String) async -> Bool {
        guard await verifyCachePassphrase(currentPassphrase) else {
            lastMutationError = "Passphrase does not match."
            return false
        }
        do {
            try await cacheStore.dropEncryption()
        } catch {
            lastMutationError = "Could not rewrite cache as plaintext: \(error)"
            return false
        }
        await MutationAuditLog.shared.configureEncryption(enabled: false, key: HCBCacheKeychain.load(), salt: nil)
        HCBCacheKeychain.clear()
        settings.cacheEncryptionEnabled = false
        settings.auditLogEncryptionEnabled = false
        await saveCurrentState()
        return true
    }

    // Verifies a passphrase without mutating anything on disk. Used by the
    // disable-encryption flow and the change-passphrase flow.
    func verifyCachePassphrase(_ passphrase: String) async -> Bool {
        guard let salt = await cacheStore.currentSalt() else { return false }
        guard let derived = try? HCBCacheCrypto.deriveKey(passphrase: passphrase, salt: salt) else { return false }
        guard let cached = HCBCacheKeychain.load() else {
            // Keychain was cleared but cache is still encrypted — accept a
            // correct re-derivation by attempting a decrypt via the loaded
            // envelope. The caller should treat true here as "this passphrase
            // would unlock the cache if set."
            return true // best-effort
        }
        return derived.withUnsafeBytes { d in
            cached.withUnsafeBytes { c in
                d.count == c.count && memcmp(d.baseAddress, c.baseAddress, d.count) == 0
            }
        }
    }

    // Re-keys the cache: verifies the current passphrase, derives a fresh
    // key from the new one with a fresh salt, re-encrypts. Fails closed:
    // on any error we leave the old key + cache intact.
    @discardableResult
    func changeCachePassphrase(from current: String, to next: String) async -> Bool {
        guard await verifyCachePassphrase(current) else {
            lastMutationError = "Current passphrase does not match."
            return false
        }
        let trimmed = next.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false else {
            lastMutationError = "New passphrase is empty."
            return false
        }
        // Fresh salt on re-key — rotates the KDF input alongside the passphrase.
        let newSalt = HCBCacheCrypto.randomSalt()
        do {
            let newKey = try HCBCacheCrypto.deriveKey(passphrase: trimmed, salt: newSalt)
            // Write the new salt first so the next encrypt picks it up. The
            // existing salt is overwritten atomically below via ensureSalt.
            if let saltURL = await cacheStore.saltFileURL() {
                try? FileManager.default.createDirectory(at: saltURL.deletingLastPathComponent(), withIntermediateDirectories: true)
                try newSalt.write(to: saltURL, options: [.atomic])
            }
            try HCBCacheKeychain.save(newKey)
            await cacheStore.setEncryptionKey(newKey)
            if settings.auditLogEncryptionEnabled {
                await MutationAuditLog.shared.configureEncryption(enabled: true, key: newKey, salt: newSalt)
            }
            scheduleCacheSave() // re-encrypt with the new key
            return true
        } catch {
            lastMutationError = "Could not change passphrase: \(error)"
            return false
        }
    }

    // Updates a per-surface font override (§6.11). Passing `.empty` clears
    // the override so the surface falls back to the global Appearance font.
    func setPerSurfaceFont(_ surface: HCBSurface, override: HCBSurfaceFontOverride) {
        var next = settings.perSurfaceFontOverrides
        if override.isEmpty {
            next.removeValue(forKey: surface.rawValue)
        } else {
            next[surface.rawValue] = override
        }
        guard next != settings.perSurfaceFontOverrides else { return }
        settings.perSurfaceFontOverrides = next
        scheduleCacheSave()
    }

    func updateSettings(_ next: AppSettings) {
        guard settings != next else { return }
        settings = next
        GoogleDiagnostics.setRawPayloadLoggingEnabled(settings.rawGoogleDiagnosticsEnabled)
        scheduleCacheSave()
        reconcileMCPServer()
    }

    func setMCPServerEnabled(_ isEnabled: Bool) {
        guard settings.mcpServerEnabled != isEnabled else { return }
        settings.mcpServerEnabled = isEnabled
        scheduleCacheSave()
        reconcileMCPServer()
    }

    func setMCPPermissionMode(_ mode: MCPPermissionMode) {
        guard settings.mcpPermissionMode != mode else { return }
        settings.mcpPermissionMode = mode
        scheduleCacheSave()
    }

    func setMCPPort(_ port: Int) {
        let normalized = max(1, min(65535, port))
        guard settings.mcpPort != normalized else { return }
        settings.mcpPort = normalized
        scheduleCacheSave()
        reconcileMCPServer()
    }

    func resetMCPBearerToken() -> String? {
        do {
            let token = try HCBMCPTokenStore.resetToken()
            if settings.mcpServerEnabled {
                reconcileMCPServer()
            }
            return token
        } catch {
            lastMutationError = "Could not reset MCP token: \(error.localizedDescription)"
            return nil
        }
    }

    func mcpClientConfigurationJSON() -> String {
        let token = (try? HCBMCPTokenStore.loadOrCreateToken()) ?? "<token unavailable>"
        let object: [String: Any] = [
            "mcpServers": [
                "hot-cross-buns": [
                    "url": "http://127.0.0.1:\(settings.mcpPort)/mcp",
                    "headers": [
                        "Authorization": "Bearer \(token)"
                    ]
                ]
            ]
        ]
        guard let data = try? JSONSerialization.data(withJSONObject: object, options: [.prettyPrinted, .sortedKeys]),
              let json = String(data: data, encoding: .utf8) else {
            return "{}"
        }
        return json
    }

    func clearMCPRecentActivity() {
        mcpRecentActivity = []
    }

    private func reconcileMCPServer() {
        guard settings.mcpServerEnabled else {
            mcpServerController?.stop()
            mcpServerStatus = .stopped
            return
        }
        if mcpServerController == nil {
            let toolService = HCBToolService(model: self)
            mcpServerController = MCPServerController(
                toolService: toolService,
                onStatus: { [weak self] status in
                    self?.mcpServerStatus = status
                },
                onActivity: { [weak self] activity in
                    self?.recordMCPActivity(activity)
                }
            )
        }
        mcpServerController?.start(port: settings.mcpPort)
    }

    private func recordMCPActivity(_ activity: MCPActivityEntry) {
        mcpRecentActivity.insert(activity, at: 0)
        if mcpRecentActivity.count > 20 {
            mcpRecentActivity.removeLast(mcpRecentActivity.count - 20)
        }
    }

    func setRawGoogleDiagnosticsEnabled(_ isEnabled: Bool) {
        guard settings.rawGoogleDiagnosticsEnabled != isEnabled else { return }
        settings.rawGoogleDiagnosticsEnabled = isEnabled
        GoogleDiagnostics.setRawPayloadLoggingEnabled(isEnabled)
        AppLogger.info("raw google diagnostics changed", category: .google, metadata: ["enabled": String(isEnabled)])
        scheduleCacheSave()
    }

    func settingsExportBundle() -> SettingsTransferBundle {
        SettingsTransferBundle(settings: settings)
    }

    func exportPortableArchive(to directoryURL: URL, options: PortableExportOptions = .all) throws -> PortableExportSummary {
        try PortableExportArchive.write(state: currentCachedState(), to: directoryURL, options: options)
    }

    func previewPortableImport(from archiveURL: URL) throws -> PortableImportPreview {
        try PortableExportArchive.previewImport(from: archiveURL, comparingTo: currentCachedState())
    }

    @discardableResult
    func importPortableArchive(
        from archiveURL: URL,
        options: PortableImportOptions = .all
    ) async throws -> PortableImportSummary {
        let backupURL = try await localBackupService.writeBackup(
            state: currentCachedState(),
            now: Date(),
            retentionCount: max(settings.dailyLocalBackupRetentionCount, 14),
            encryptionContext: try await localBackupEncryptionContext()
        )
        let result = try PortableExportArchive.importState(from: archiveURL)
        applyImportedState(result.state, options: options)
        localBackupSummary = await localBackupService.summary()
        var summary = result.summary
        summary.preImportBackupURL = backupURL
        return summary
    }

    func localAttachmentDiagnostics(includeHealthy: Bool = false) -> [LocalAttachmentDiagnostic] {
        var diagnostics: [LocalAttachmentDiagnostic] = []
        for task in tasks {
            for attachment in LocalFileAttachment.parseAll(in: task.notes) {
                appendAttachmentDiagnostic(
                    attachment,
                    sourceKind: "Task",
                    sourceTitle: task.title,
                    sourceID: task.id,
                    includeHealthy: includeHealthy,
                    into: &diagnostics
                )
            }
        }
        for event in events {
            for attachment in LocalFileAttachment.parseAll(in: event.details) {
                appendAttachmentDiagnostic(
                    attachment,
                    sourceKind: "Event",
                    sourceTitle: event.summary,
                    sourceID: event.id,
                    includeHealthy: includeHealthy,
                    into: &diagnostics
                )
            }
        }
        return diagnostics
    }

    @discardableResult
    func relinkLocalAttachment(originalURL: URL, replacementURL: URL, kind: LocalFileAttachment.Kind) throws -> Int {
        let replacementHealth = LocalFileAttachment(kind: kind, displayName: replacementURL.lastPathComponent, url: replacementURL).health
        guard replacementHealth.isAvailable else {
            throw LocalAttachmentRepairError.replacementUnavailable(replacementHealth)
        }

        let replacements = [originalURL.absoluteString: replacementURL]
        var updatedFieldCount = 0
        for index in tasks.indices {
            let next = LocalFileAttachment.rewritePointers(in: tasks[index].notes, replacing: replacements)
            if next != tasks[index].notes {
                tasks[index].notes = next
                updatedFieldCount += 1
            }
        }
        for index in events.indices {
            let next = LocalFileAttachment.rewritePointers(in: events[index].details, replacing: replacements)
            if next != events[index].details {
                events[index].details = next
                updatedFieldCount += 1
            }
        }

        if updatedFieldCount > 0 {
            rebuildSnapshots()
            scheduleCacheSave()
        }
        return updatedFieldCount
    }

    func previewSettingsImport(_ bundle: SettingsTransferBundle) -> SettingsImportPreview {
        let next = settingsForImport(bundle.settings)
        return SettingsImportPreview(
            changeCount: settingsImportChangeCount(from: settings, to: next),
            summaries: settingsImportSummaries(from: settings, to: next),
            excludedFields: bundle.excludedFields
        )
    }

    func applySettingsImport(_ bundle: SettingsTransferBundle) {
        let next = settingsForImport(bundle.settings)
        guard settings != next else { return }
        settings = next
        rebuildSnapshots()
        scheduleCacheSave()
    }

    private func applyImportedState(_ imported: CachedAppState, options: PortableImportOptions = .all) {
        if options.includeSyncMetadata {
            account = imported.account
            connectedAccounts = imported.accounts
            activeAccountID = imported.activeAccountID
            accountWorkspaces = imported.accountWorkspaces
            syncCheckpoints = imported.syncCheckpoints
            pendingMutations = imported.pendingMutations
        }
        if options.includeTasks {
            taskLists = imported.taskLists
            tasks = imported.tasks
        }
        if options.includeEvents {
            calendars = imported.calendars
            events = imported.events
        }
        if options.includeSettings {
            settings = settingsForImport(imported.settings)
        }
        rebuildSnapshots()
        scheduleCacheSave()
    }

    private func appendAttachmentDiagnostic(
        _ attachment: LocalFileAttachment,
        sourceKind: String,
        sourceTitle: String,
        sourceID: String,
        includeHealthy: Bool,
        into diagnostics: inout [LocalAttachmentDiagnostic]
    ) {
        let health = attachment.health
        guard includeHealthy || health.isAvailable == false else { return }
        diagnostics.append(LocalAttachmentDiagnostic(
            id: "\(sourceKind)-\(sourceID)-\(attachment.url.absoluteString)",
            sourceKind: sourceKind,
            sourceTitle: sourceTitle,
            sourceID: sourceID,
            attachmentKind: attachment.kind,
            displayName: attachment.displayName,
            url: attachment.url,
            health: health
        ))
    }

    private func settingsForImport(_ imported: AppSettings) -> AppSettings {
        var next = imported
        // Encryption enablement is tied to this Mac's Keychain key. Importing
        // the flag without the key would leave the UI claiming encryption is
        // active while the cache store cannot encrypt/decrypt with that key.
        next.cacheEncryptionEnabled = settings.cacheEncryptionEnabled
        next.auditLogEncryptionEnabled = settings.auditLogEncryptionEnabled
        return next
    }

    private func settingsImportChangeCount(from current: AppSettings, to next: AppSettings) -> Int {
        [
            current.syncMode != next.syncMode,
            current.selectedCalendarIDs != next.selectedCalendarIDs,
            current.selectedTaskListIDs != next.selectedTaskListIDs,
            current.shortcutOverrides != next.shortcutOverrides,
            current.hiddenSidebarItems != next.hiddenSidebarItems,
            current.hiddenCalendarViewModes != next.hiddenCalendarViewModes,
            current.customFilters != next.customFilters,
            current.taskTemplates != next.taskTemplates,
            current.eventTemplates != next.eventTemplates,
            current.colorSchemeID != next.colorSchemeID,
            current.uiLayoutScale != next.uiLayoutScale,
            current.uiTextSizePoints != next.uiTextSizePoints,
            current.uiFontName != next.uiFontName,
            current.disableAnimations != next.disableAnimations,
            current.performanceMode != next.performanceMode,
            current.perSurfaceFontOverrides != next.perSurfaceFontOverrides,
            current.sidebarPlacement != next.sidebarPlacement,
            current.menuBarStyle != next.menuBarStyle,
            current.menuBarAdaptiveStatusSource != next.menuBarAdaptiveStatusSource,
            current.menuBarAdaptiveEmptyBehavior != next.menuBarAdaptiveEmptyBehavior,
            current.menuBarAdaptivePanelContent != next.menuBarAdaptivePanelContent,
            current.menuBarIcon != next.menuBarIcon,
            current.showMenuBarExtra != next.showMenuBarExtra,
            current.showMenuBarBadge != next.showMenuBarBadge,
            current.showDockBadge != next.showDockBadge,
            current.enableGlobalHotkey != next.enableGlobalHotkey,
            current.globalHotkeyBinding != next.globalHotkeyBinding,
            current.enableLocalNotifications != next.enableLocalNotifications,
            current.pastEventBehavior != next.pastEventBehavior,
            current.overdueTaskBehavior != next.overdueTaskBehavior,
            current.completedTaskBehavior != next.completedTaskBehavior,
            current.eventRetentionDaysBack != next.eventRetentionDaysBack,
            current.completedTaskRetentionDaysBack != next.completedTaskRetentionDaysBack,
            current.showCompletedItemsInCalendar != next.showCompletedItemsInCalendar,
            current.historyVisibleLimit != next.historyVisibleLimit,
            current.historyStorageCap != next.historyStorageCap
        ].filter { $0 }.count
    }

    private func settingsImportSummaries(from current: AppSettings, to next: AppSettings) -> [String] {
        var summaries: [String] = []

        if current.colorSchemeID != next.colorSchemeID
            || current.uiLayoutScale != next.uiLayoutScale
            || current.uiTextSizePoints != next.uiTextSizePoints
            || current.uiFontName != next.uiFontName
            || current.disableAnimations != next.disableAnimations
            || current.performanceMode != next.performanceMode
            || current.perSurfaceFontOverrides != next.perSurfaceFontOverrides
            || current.sidebarPlacement != next.sidebarPlacement {
            summaries.append("Appearance, font, layout, or theme preferences will change.")
        }
        if current.shortcutOverrides != next.shortcutOverrides
            || current.enableGlobalHotkey != next.enableGlobalHotkey
            || current.globalHotkeyBinding != next.globalHotkeyBinding {
            summaries.append("Keyboard shortcut and global hotkey settings will change.")
        }
        if current.syncMode != next.syncMode
            || current.selectedCalendarIDs != next.selectedCalendarIDs
            || current.selectedTaskListIDs != next.selectedTaskListIDs
            || current.hasConfiguredCalendarSelection != next.hasConfiguredCalendarSelection
            || current.hasConfiguredTaskListSelection != next.hasConfiguredTaskListSelection
            || current.eventRetentionDaysBack != next.eventRetentionDaysBack
            || current.completedTaskRetentionDaysBack != next.completedTaskRetentionDaysBack {
            summaries.append("Sync mode, retention, calendar visibility, or task-list visibility will change.")
        }
        if current.customFilters != next.customFilters {
            summaries.append("Custom filters will change from \(current.customFilters.count) to \(next.customFilters.count).")
        }
        if current.taskTemplates != next.taskTemplates || current.eventTemplates != next.eventTemplates {
            summaries.append("Templates will change from \(current.taskTemplates.count + current.eventTemplates.count) to \(next.taskTemplates.count + next.eventTemplates.count).")
        }
        if current.menuBarStyle != next.menuBarStyle
            || current.menuBarAdaptiveStatusSource != next.menuBarAdaptiveStatusSource
            || current.menuBarAdaptiveEmptyBehavior != next.menuBarAdaptiveEmptyBehavior
            || current.menuBarAdaptivePanelContent != next.menuBarAdaptivePanelContent
            || current.menuBarIcon != next.menuBarIcon
            || current.showMenuBarExtra != next.showMenuBarExtra
            || current.showMenuBarBadge != next.showMenuBarBadge
            || current.showDockBadge != next.showDockBadge
            || current.hiddenSidebarItems != next.hiddenSidebarItems
            || current.hiddenCalendarViewModes != next.hiddenCalendarViewModes {
            summaries.append("Menu bar, Dock badge, sidebar, or calendar view visibility will change.")
        }
        if current.enableLocalNotifications != next.enableLocalNotifications
            || current.enableTaskCompletionSound != next.enableTaskCompletionSound
            || current.enableEventCompletionSound != next.enableEventCompletionSound
            || current.taskCompletionSoundChoice != next.taskCompletionSoundChoice
            || current.eventCompletionSoundChoice != next.eventCompletionSoundChoice {
            summaries.append("Reminder and completion sound preferences will change.")
        }
        if current.pastEventBehavior != next.pastEventBehavior
            || current.overdueTaskBehavior != next.overdueTaskBehavior
            || current.completedTaskBehavior != next.completedTaskBehavior
            || current.eventRetentionDaysBack != next.eventRetentionDaysBack
            || current.completedTaskRetentionDaysBack != next.completedTaskRetentionDaysBack
            || current.showCompletedItemsInCalendar != next.showCompletedItemsInCalendar {
            summaries.append("Calendar cleanup and completed-item visibility preferences will change.")
        }
        if current.historyVisibleLimit != next.historyVisibleLimit
            || current.historyStorageCap != next.historyStorageCap
            || current.historyCategoryFilters != next.historyCategoryFilters {
            summaries.append("History window preferences will change.")
        }

        return summaries
    }

    func refreshOpenAtLoginStatus() {
        opensAtLogin = loginItemController.isEnabled
    }

    func setOpenAtLogin(_ isEnabled: Bool) {
        loginItemError = nil
        do {
            try loginItemController.setEnabled(isEnabled)
            opensAtLogin = loginItemController.isEnabled
        } catch {
            opensAtLogin = loginItemController.isEnabled
            loginItemError = error.localizedDescription
        }
    }

    // Per-tab list filters. Nil = inherit the global selectedTaskListIDs.
    func setTasksTabListFilter(_ ids: Set<TaskListMirror.ID>?) {
        if let ids {
            settings.tasksTabSelectedListIDs = ids
            settings.hasConfiguredTasksTabSelection = true
        } else {
            settings.tasksTabSelectedListIDs = []
            settings.hasConfiguredTasksTabSelection = false
        }
        scheduleRebuildSnapshots()
        scheduleCacheSave()
    }

    func setNotesTabListFilter(_ ids: Set<TaskListMirror.ID>?) {
        if let ids {
            settings.notesTabSelectedListIDs = ids
            settings.hasConfiguredNotesTabSelection = true
        } else {
            settings.notesTabSelectedListIDs = []
            settings.hasConfiguredNotesTabSelection = false
        }
        scheduleRebuildSnapshots()
        scheduleCacheSave()
    }

    func setNotesViewMode(_ mode: NotesViewMode) {
        guard settings.notesViewMode != mode else { return }
        settings.notesViewMode = mode
        scheduleCacheSave()
    }

    func setNotesKanbanColumnMode(_ mode: KanbanColumnMode) {
        guard settings.notesKanbanColumnMode != mode else { return }
        settings.notesKanbanColumnMode = mode
        publishSnapshotSurfaceStores()
        scheduleCacheSave()
    }

    func setColorTagAutoApplyEnabled(_ enabled: Bool) {
        guard settings.colorTagAutoApplyEnabled != enabled else { return }
        settings.colorTagAutoApplyEnabled = enabled
        scheduleCacheSave()
    }

    func setColorTagBinding(colorId: String, tag: String?) {
        let normalized = tag?.trimmingCharacters(in: .whitespacesAndNewlines)
        if let normalized, normalized.isEmpty == false {
            settings.colorTagBindings[colorId] = normalized
        } else {
            settings.colorTagBindings.removeValue(forKey: colorId)
        }
        scheduleCacheSave()
    }

    func setColorTagMatchPolicy(_ policy: ColorTagMatchPolicy) {
        guard settings.colorTagMatchPolicy != policy else { return }
        settings.colorTagMatchPolicy = policy
        scheduleCacheSave()
    }

    // MARK: - Past cleanup setters

    // Switching into/out of .delete resets the ack flag so the user
    // re-consents if they flip it back on after disabling.
    func setPastEventBehavior(_ behavior: PastEventBehavior) {
        guard settings.pastEventBehavior != behavior else { return }
        let wasDeletion = settings.pastEventBehavior.isDeletion
        settings.pastEventBehavior = behavior
        if wasDeletion, behavior.isDeletion == false {
            settings.hasAckedEventDeletion = false
        }
        scheduleCacheSave()
    }

    func setPastEventDeleteThresholdDays(_ days: Int) {
        let clamped = max(1, min(365, days))
        guard settings.pastEventDeleteThresholdDays != clamped else { return }
        settings.pastEventDeleteThresholdDays = clamped
        scheduleCacheSave()
    }

    func setAllowDeletingAttendeeEvents(_ allowed: Bool) {
        guard settings.allowDeletingAttendeeEvents != allowed else { return }
        settings.allowDeletingAttendeeEvents = allowed
        if allowed == false {
            settings.hasAckedAttendeeDeletion = false
        }
        scheduleCacheSave()
    }

    func setShowCompletedItemsInCalendar(_ isVisible: Bool) {
        guard settings.showCompletedItemsInCalendar != isVisible else { return }
        settings.showCompletedItemsInCalendar = isVisible
        rebuildSnapshots()
        scheduleCacheSave()
    }

    func setOverdueTaskBehavior(_ behavior: OverdueTaskBehavior) {
        guard settings.overdueTaskBehavior != behavior else { return }
        settings.overdueTaskBehavior = behavior
        scheduleCacheSave()
    }

    func setCompletedTaskBehavior(_ behavior: CompletedTaskBehavior) {
        guard settings.completedTaskBehavior != behavior else { return }
        let wasDeletion = settings.completedTaskBehavior.isDeletion
        settings.completedTaskBehavior = behavior
        if wasDeletion, behavior.isDeletion == false {
            settings.hasAckedTaskDeletion = false
        }
        scheduleCacheSave()
    }

    func setCompletedTaskDeleteThresholdDays(_ days: Int) {
        let clamped = max(1, min(365, days))
        guard settings.completedTaskDeleteThresholdDays != clamped else { return }
        settings.completedTaskDeleteThresholdDays = clamped
        scheduleCacheSave()
    }

    func acknowledgeEventDeletion() {
        guard settings.hasAckedEventDeletion == false else { return }
        settings.hasAckedEventDeletion = true
        scheduleCacheSave()
    }

    func acknowledgeAttendeeDeletion() {
        guard settings.hasAckedAttendeeDeletion == false else { return }
        settings.hasAckedAttendeeDeletion = true
        scheduleCacheSave()
    }

    func acknowledgeTaskDeletion() {
        guard settings.hasAckedTaskDeletion == false else { return }
        settings.hasAckedTaskDeletion = true
        scheduleCacheSave()
    }

    func setUILayoutScale(_ scale: Double) {
        guard settings.uiLayoutScale != scale else { return }
        settings.uiLayoutScale = scale
        scheduleCacheSave()
    }

    func setUITextSizePoints(_ points: Double) {
        let clamped = HCBTextSize.clamp(points)
        guard settings.uiTextSizePoints != clamped else { return }
        settings.uiTextSizePoints = clamped
        scheduleCacheSave()
    }

    func setUIFontName(_ name: String?) {
        let normalized = name?.trimmingCharacters(in: .whitespacesAndNewlines)
        let resolved = (normalized?.isEmpty ?? true) ? nil : normalized
        guard settings.uiFontName != resolved else { return }
        settings.uiFontName = resolved
        scheduleCacheSave()
    }

    func setColorSchemeID(_ id: String) {
        guard settings.colorSchemeID != id else { return }
        guard HCBColorScheme.scheme(id: id, customSchemes: settings.customColorSchemes) != nil else { return }
        settings.colorSchemeID = id
        HCBColorSchemeStore.current = HCBColorScheme.scheme(id: id, customSchemes: settings.customColorSchemes) ?? .notion
        scheduleCacheSave()
    }

    func setAppBackgroundTranslucencyEnabled(_ isEnabled: Bool) {
        guard settings.appBackgroundTranslucencyEnabled != isEnabled else { return }
        settings.appBackgroundTranslucencyEnabled = isEnabled
        scheduleCacheSave()
    }

    func setAppBackgroundOpacity(_ opacity: Double) {
        let clamped = max(0.35, min(1.0, opacity))
        guard settings.appBackgroundOpacity != clamped else { return }
        settings.appBackgroundOpacity = clamped
        scheduleCacheSave()
    }

    func importCustomBackground(from sourceURL: URL) throws {
        let didStartAccessing = sourceURL.startAccessingSecurityScopedResource()
        defer {
            if didStartAccessing {
                sourceURL.stopAccessingSecurityScopedResource()
            }
        }

        let directory = try Self.customBackgroundDirectory()
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let destinationURL = directory.appending(path: "custom-background.jpg")

        if let currentPath = settings.customBackgroundImagePath {
            try? FileManager.default.removeItem(at: URL(filePath: currentPath))
        }
        if let oldBackgrounds = try? FileManager.default.contentsOfDirectory(at: directory, includingPropertiesForKeys: nil) {
            for url in oldBackgrounds where url.lastPathComponent.hasPrefix("custom-background.") {
                try? FileManager.default.removeItem(at: url)
            }
        }
        try? FileManager.default.removeItem(at: destinationURL)
        try Self.writeDownsampledCustomBackground(from: sourceURL, to: destinationURL)

        settings.customBackgroundImagePath = destinationURL.path
        settings.appBackgroundTranslucencyEnabled = true
        settings.appBackgroundOpacity = min(settings.appBackgroundOpacity, 0.55)
        scheduleCacheSave()
    }

    func clearCustomBackground() {
        if let currentPath = settings.customBackgroundImagePath {
            try? FileManager.default.removeItem(at: URL(filePath: currentPath))
        }
        guard settings.customBackgroundImagePath != nil else { return }
        settings.customBackgroundImagePath = nil
        scheduleCacheSave()
    }

    func upsertCustomColorScheme(_ scheme: HCBCustomColorScheme, select: Bool = true) {
        var normalized = scheme
        normalized.title = normalized.title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? "Custom theme" : normalized.title
        if let index = settings.customColorSchemes.firstIndex(where: { $0.id == normalized.id }) {
            settings.customColorSchemes[index] = normalized
        } else {
            settings.customColorSchemes.append(normalized)
        }
        if select {
            settings.colorSchemeID = normalized.id
            HCBColorSchemeStore.current = normalized.colorScheme()
        }
        scheduleCacheSave()
    }

    func deleteCustomColorScheme(id: String) {
        guard settings.customColorSchemes.contains(where: { $0.id == id }) else { return }
        settings.customColorSchemes.removeAll { $0.id == id }
        if settings.colorSchemeID == id {
            let replacement = HCBColorScheme.all.first { $0.isDark == HCBColorSchemeStore.current.isDark } ?? .notion
            settings.colorSchemeID = replacement.id
            HCBColorSchemeStore.current = replacement
        }
        scheduleCacheSave()
    }

    func duplicateCurrentColorSchemeForEditing() -> HCBCustomColorScheme {
        let base = HCBColorScheme.scheme(id: settings.colorSchemeID, customSchemes: settings.customColorSchemes) ?? .notion
        return HCBCustomColorScheme(copying: base)
    }

    func setDailyLocalBackupEnabled(_ enabled: Bool) {
        guard settings.dailyLocalBackupEnabled != enabled else { return }
        settings.dailyLocalBackupEnabled = enabled
        scheduleCacheSave()
        if enabled {
            Task { await runDailyLocalBackupNow() }
        }
    }

    func setDailyLocalBackupRetentionCount(_ count: Int) {
        let clamped = max(1, min(90, count))
        guard settings.dailyLocalBackupRetentionCount != clamped else { return }
        settings.dailyLocalBackupRetentionCount = clamped
        scheduleCacheSave()
    }

    @discardableResult
    func runDailyLocalBackupNow() async -> Bool {
        await runDailyLocalBackupIfNeeded(force: true)
    }

    func refreshLocalBackupSummary() async {
        localBackupSummary = await localBackupService.summary()
    }

    func openLocalBackupFolder() async {
        do {
            try await localBackupService.openBackupDirectoryInFinder()
            localBackupSummary = await localBackupService.summary()
        } catch {
            AppLogger.warn("backup folder open failed", category: .cache, metadata: ["error": String(describing: error)])
        }
    }

    func setShortcutBinding(_ command: HCBShortcutCommand, binding: HCBKeyBinding?) {
        if let binding {
            settings.shortcutOverrides[command.rawValue] = binding
        } else {
            settings.shortcutOverrides.removeValue(forKey: command.rawValue)
        }
        HCBShortcutStorage.persist(settings.shortcutOverrides)
        scheduleCacheSave()
    }

    func resetAllShortcutBindings() {
        guard settings.shortcutOverrides.isEmpty == false else { return }
        settings.shortcutOverrides.removeAll()
        HCBShortcutStorage.persist(settings.shortcutOverrides)
        scheduleCacheSave()
    }

    func upsertCustomFilter(_ filter: CustomFilterDefinition) {
        if let index = settings.customFilters.firstIndex(where: { $0.id == filter.id }) {
            settings.customFilters[index] = filter
        } else {
            settings.customFilters.append(filter)
        }
        scheduleCacheSave()
    }

    func deleteCustomFilter(_ id: CustomFilterDefinition.ID) {
        settings.customFilters.removeAll { $0.id == id }
        scheduleCacheSave()
    }

    @discardableResult
    func duplicateCustomFilter(_ filter: CustomFilterDefinition) -> CustomFilterDefinition {
        let copy = filter.duplicated()
        upsertCustomFilter(copy)
        return copy
    }

    func markCustomFilterUsed(_ id: CustomFilterDefinition.ID) {
        guard let index = settings.customFilters.firstIndex(where: { $0.id == id }) else { return }
        settings.customFilters[index].lastUsedAt = Date()
        settings.customFilters[index].useCount += 1
        scheduleCacheSave()
    }

    func upsertEventTemplate(_ template: EventTemplate) {
        if let index = settings.eventTemplates.firstIndex(where: { $0.id == template.id }) {
            settings.eventTemplates[index] = template
        } else {
            settings.eventTemplates.append(template)
        }
        scheduleCacheSave()
    }

    func deleteEventTemplate(_ id: EventTemplate.ID) {
        settings.eventTemplates.removeAll { $0.id == id }
        scheduleCacheSave()
    }

    @discardableResult
    func duplicateEventTemplate(_ template: EventTemplate) -> EventTemplate {
        let copy = template.duplicated()
        upsertEventTemplate(copy)
        return copy
    }

    func markEventTemplateUsed(_ id: EventTemplate.ID) {
        guard let index = settings.eventTemplates.firstIndex(where: { $0.id == id }) else { return }
        settings.eventTemplates[index].lastUsedAt = Date()
        settings.eventTemplates[index].useCount += 1
        scheduleCacheSave()
    }

    // §6.13b — Instantiates an event template: expands every templated field
    // with the supplied prompt answers + clipboard + date vars, resolves the
    // date + time anchors into a real start Date, and creates a Google event
    // via the existing createEvent path. Returns true on success.
    //
    // Date/time composition:
    //   dateAnchor expands to "YYYY-MM-DD"; empty → today (start-of-day).
    //   timeAnchor is literal "HH:mm" in 24h; empty → now rounded up to the
    //   next 15-minute boundary. When isAllDay, timeAnchor is ignored and
    //   endDate = startDate + max(durationMinutes/1440 * 1, 1) days (treated
    //   as inclusive-end by createEvent).
    //
    // Like task templates, NO template metadata ever lands in the Google
    // event — the resulting event is indistinguishable on google.com from a
    // manually-created one.
    @discardableResult
    func instantiateEventTemplate(_ template: EventTemplate, prompts: [String: String] = [:]) async -> Bool {
        let ctx = HCBTemplateContext(
            now: Date(),
            calendar: .current,
            clipboard: NSPasteboard.general.string(forType: .string),
            prompts: prompts
        )
        func expand(_ s: String) -> String {
            HCBTemplateExpander.expand(s, context: ctx)
                .replacingOccurrences(of: HCBTemplateExpander.cursorSentinel, with: "")
        }

        let summary = expand(template.summary)
        let details = expand(template.details)
        let location = expand(template.location)
        let dateString = expand(template.dateAnchor).trimmingCharacters(in: .whitespacesAndNewlines)
        let calendarRef = expand(template.calendarIdOrTitle).trimmingCharacters(in: .whitespacesAndNewlines)
        let attendees = template.attendees
            .map { expand($0).trimmingCharacters(in: .whitespacesAndNewlines) }
            .filter { $0.isEmpty == false }

        // Resolve date.
        let baseCalendar = Calendar.current
        let now = Date()
        let startOfDay: Date
        if dateString.isEmpty {
            startOfDay = baseCalendar.startOfDay(for: now)
        } else {
            let f = DateFormatter()
            f.locale = Locale(identifier: "en_US_POSIX")
            f.dateFormat = "yyyy-MM-dd"
            guard let parsed = f.date(from: dateString) else {
                lastMutationError = "Event template '\(template.name)' has an invalid dateAnchor: \(dateString)."
                return false
            }
            startOfDay = baseCalendar.startOfDay(for: parsed)
        }

        // Compose start Date.
        let startDate: Date
        let endDate: Date
        if template.isAllDay {
            startDate = startOfDay
            let days = max(Int(ceil(Double(template.durationMinutes) / (24 * 60))), 1)
            endDate = baseCalendar.date(byAdding: .day, value: days, to: startOfDay) ?? startOfDay
        } else {
            let timeString = template.timeAnchor.trimmingCharacters(in: .whitespacesAndNewlines)
            let (hour, minute): (Int, Int) = {
                if timeString.isEmpty {
                    // Round up to the next 15-minute boundary.
                    let comps = baseCalendar.dateComponents([.hour, .minute], from: now)
                    let rawMin = (comps.minute ?? 0)
                    let bumped = ((rawMin / 15) + 1) * 15
                    if bumped >= 60 {
                        return ((comps.hour ?? 0) + 1, 0)
                    }
                    return (comps.hour ?? 0, bumped)
                }
                let parts = timeString.split(separator: ":").compactMap { Int($0) }
                if parts.count == 2, (0...23).contains(parts[0]), (0...59).contains(parts[1]) {
                    return (parts[0], parts[1])
                }
                return (9, 0) // fallback default
            }()
            let resolvedStart = baseCalendar.date(bySettingHour: hour, minute: minute, second: 0, of: startOfDay) ?? startOfDay
            startDate = resolvedStart
            endDate = resolvedStart.addingTimeInterval(TimeInterval(max(template.durationMinutes, 5) * 60))
        }

        // Resolve calendar — "" / unknown falls back to the first writable.
        let writable = calendars.filter { $0.accessRole == "owner" || $0.accessRole == "writer" }
        let resolvedCalendarID: CalendarListMirror.ID? = {
            if calendarRef.isEmpty { return writable.first?.id ?? calendars.first?.id }
            if let exact = calendars.first(where: { $0.id == calendarRef }) { return exact.id }
            return calendars.first(where: { $0.summary.localizedCaseInsensitiveCompare(calendarRef) == .orderedSame })?.id
        }()
        guard let calendarID = resolvedCalendarID else {
            lastMutationError = "No calendar available for template '\(template.name)'."
            return false
        }

        // Recurrence: Google expects an array of "RRULE:..." strings.
        let recurrence: [String] = {
            let trimmed = template.recurrenceRule.trimmingCharacters(in: .whitespacesAndNewlines)
            guard trimmed.isEmpty == false else { return [] }
            if trimmed.uppercased().hasPrefix("RRULE:") { return [trimmed] }
            return ["RRULE:\(trimmed)"]
        }()

        let created = await createEvent(
            summary: summary,
            details: details,
            startDate: startDate,
            endDate: endDate,
            isAllDay: template.isAllDay,
            reminderMinutes: template.reminderMinutes,
            calendarID: calendarID,
            location: location,
            recurrence: recurrence,
            attendeeEmails: attendees,
            notifyGuests: false,
            addGoogleMeet: template.addGoogleMeet,
            colorId: template.colorId
        )
        if created {
            markEventTemplateUsed(template.id)
        }
        return created
    }

    func bulkDeleteEvents(_ events: [CalendarEventMirror]) async -> Int {
        let firstTitle = events.first?.summary ?? ""
        var deleted = 0
        for event in events {
            if await deleteEvent(event, scope: .thisOccurrence) {
                deleted += 1
            }
        }
        if deleted > 1 {
            // suppress N individual eventDelete undo entries behind one bulk summary. individual entries were already recorded in deleteEvent so this is additive for history; performUndo on .bulkAction is a no-op.
            recordUndo(.bulkAction(kind: "delete", count: deleted, firstTitle: firstTitle))
        }
        return deleted
    }

    // Adds a #tag token to the task title. Idempotent: if the tag is already
    // present (case-insensitive), no API call is issued.
    func addTag(_ tag: String, to task: TaskMirror) async -> Bool {
        let normalized = tag.trimmingCharacters(in: CharacterSet(charactersIn: "# ")).trimmingCharacters(in: .whitespacesAndNewlines)
        guard normalized.isEmpty == false else {
            lastMutationError = "Tag is empty."
            return false
        }
        let existing = Set(TagExtractor.tags(in: task.title).map { $0.lowercased() })
        if existing.contains(normalized.lowercased()) { return true }
        let newTitle = task.title.trimmingCharacters(in: .whitespaces).isEmpty
            ? "#\(normalized)"
            : "\(task.title.trimmingCharacters(in: .whitespaces)) #\(normalized)"
        return await updateTask(task, title: newTitle, notes: task.notes, dueDate: task.dueDate)
    }

    // Removes a specific #tag token (case-insensitive) from the task title.
    // Idempotent: if the tag isn't present, no API call is issued.
    func removeTag(_ tag: String, from task: TaskMirror) async -> Bool {
        let normalized = tag.trimmingCharacters(in: CharacterSet(charactersIn: "# ")).trimmingCharacters(in: .whitespacesAndNewlines)
        guard normalized.isEmpty == false else {
            lastMutationError = "Tag is empty."
            return false
        }
        let existing = Set(TagExtractor.tags(in: task.title).map { $0.lowercased() })
        guard existing.contains(normalized.lowercased()) else { return true }
        // Match `#tag` preceded by whitespace or start, ending on a non-[A-Za-z0-9_-]
        // boundary — so `#work` doesn't clobber `#workout`. TagExtractor allows
        // letters/digits/underscore/hyphen as tag chars, so the boundary is any
        // char outside that set or end-of-string.
        let escaped = NSRegularExpression.escapedPattern(for: normalized)
        let pattern = "(^|\\s)#\(escaped)(?![A-Za-z0-9_\\-])"
        let cleaned = (try? NSRegularExpression(pattern: pattern, options: [.caseInsensitive]))
            .map { $0.stringByReplacingMatches(in: task.title, range: NSRange(task.title.startIndex..., in: task.title), withTemplate: "$1") }
            ?? task.title
        let collapsed = cleaned
            .replacingOccurrences(of: "\\s+", with: " ", options: .regularExpression)
            .trimmingCharacters(in: .whitespacesAndNewlines)
        if collapsed == task.title { return true }
        return await updateTask(task, title: collapsed, notes: task.notes, dueDate: task.dueDate)
    }

    // Bulk entry point: runs the optimizer, throttles dispatch, aggregates
    // per-op success/failure. Each op still routes through the optimistic-write
    // helpers so offline queue + etag conflict paths apply.
    //
    // throttleInterval caps request rate against Google's per-user-per-100s
    // quota. 40 ms ≈ 25 req/s, safely under the documented 100 req/100s bucket.
    func performBulkTaskOperations(
        _ ops: [BulkTaskOperation],
        throttleInterval: Duration = .milliseconds(40)
    ) async -> BulkTaskExecutionResult {
        let optimized = BulkTaskOptimizer.optimize(ops, currentTasks: tasks)
        guard optimized.operations.isEmpty == false else {
            return BulkTaskExecutionResult(
                submitted: 0,
                succeeded: 0,
                failures: [],
                droppedAsNoOp: optimized.droppedCount
            )
        }

        var succeeded = 0
        var failures: [BulkTaskFailure] = []
        var lastDispatchedAt: Date?
        for op in optimized.operations {
            if let last = lastDispatchedAt {
                // Token-bucket substitute: sleep until `throttleInterval` has
                // elapsed since the previous dispatch. Simple, deterministic,
                // no background task needed.
                let elapsed = Duration.seconds(max(0, Date().timeIntervalSince(last)))
                let remaining = throttleInterval - elapsed
                if remaining > .zero {
                    try? await Task.sleep(for: remaining)
                }
            }
            lastDispatchedAt = Date()

            let ok = await dispatchBulkOperation(op)
            if ok {
                succeeded += 1
            } else {
                failures.append(
                    BulkTaskFailure(
                        operation: op,
                        message: lastMutationError ?? "Operation failed."
                    )
                )
            }
        }

        return BulkTaskExecutionResult(
            submitted: optimized.operations.count,
            succeeded: succeeded,
            failures: failures,
            droppedAsNoOp: optimized.droppedCount
        )
    }

    private func dispatchBulkOperation(_ op: BulkTaskOperation) async -> Bool {
        // Re-resolve from current mirror — a previous op in the same batch may
        // have mutated this task (e.g., moveToList changes taskListID + etag).
        guard let task = self.task(id: op.taskId) else { return false }
        switch op {
        case .complete: return await setTaskCompleted(true, task: task)
        case .reopen: return await setTaskCompleted(false, task: task)
        case .delete: return await deleteTask(task)
        case .setDue(_, let dueDate): return await updateTask(task, title: task.title, notes: task.notes, dueDate: dueDate)
        case .moveToList(_, let targetListId): return await moveTaskToList(task, toTaskListID: targetListId)
        case .addTag(_, let tag): return await addTag(tag, to: task)
        case .removeTag(_, let tag): return await removeTag(tag, from: task)
        }
    }

    func bulkShiftEvents(_ events: [CalendarEventMirror], byMinutes minutes: Int) async -> Int {
        guard minutes != 0 else { return 0 }
        var shifted = 0
        for event in events {
            guard event.isAllDay == false else { continue }
            let newStart = event.startDate.addingTimeInterval(TimeInterval(minutes) * 60)
            let newEnd = event.endDate.addingTimeInterval(TimeInterval(minutes) * 60)
            let didUpdate = await updateEvent(
                event,
                summary: event.summary,
                details: event.details,
                startDate: newStart,
                endDate: newEnd,
                isAllDay: event.isAllDay,
                reminderMinutes: event.reminderMinutes.first,
                calendarID: event.calendarID,
                location: event.location,
                scope: .thisOccurrence
            )
            if didUpdate { shifted += 1 }
        }
        return shifted
    }

    func saveAsEventTemplate(_ event: CalendarEventMirror, name: String) {
        let duration = max(Int(event.endDate.timeIntervalSince(event.startDate) / 60), 15)
        let template = EventTemplate(
            name: name,
            summary: event.summary,
            details: event.details,
            location: event.location,
            durationMinutes: duration,
            isAllDay: event.isAllDay,
            reminderMinutes: event.reminderMinutes.first,
            colorId: event.colorId,
            attendees: event.attendeeEmails,
            addGoogleMeet: event.meetLink.isEmpty == false
        )
        upsertEventTemplate(template)
    }

    func setShowDetailedMenuBar(_ isEnabled: Bool) {
        guard settings.showDetailedMenuBar != isEnabled else {
            return
        }
        settings.showDetailedMenuBar = isEnabled
        scheduleCacheSave()
    }

    func setMenuBarStyle(_ style: AppSettings.MenuBarStyle) {
        guard settings.menuBarStyle != style else { return }
        settings.menuBarStyle = style
        // Keep legacy bool in sync so older reads don't misreport
        settings.showDetailedMenuBar = (style == .detailed)
        scheduleCacheSave()
    }

    func setMenuBarAdaptiveStatusSource(_ source: AppSettings.MenuBarAdaptiveStatusSource) {
        guard settings.menuBarAdaptiveStatusSource != source else { return }
        settings.menuBarAdaptiveStatusSource = source
        scheduleCacheSave()
    }

    func setMenuBarAdaptiveEmptyBehavior(_ behavior: AppSettings.MenuBarAdaptiveEmptyBehavior) {
        guard settings.menuBarAdaptiveEmptyBehavior != behavior else { return }
        settings.menuBarAdaptiveEmptyBehavior = behavior
        scheduleCacheSave()
    }

    func setMenuBarAdaptivePanelContent(_ content: AppSettings.MenuBarAdaptivePanelContent) {
        guard settings.menuBarAdaptivePanelContent != content else { return }
        settings.menuBarAdaptivePanelContent = content
        scheduleCacheSave()
    }

    func setTaskCompletionSoundEnabled(_ isEnabled: Bool) {
        guard settings.enableTaskCompletionSound != isEnabled else { return }
        settings.enableTaskCompletionSound = isEnabled
        scheduleCacheSave()
    }

    func setEventCompletionSoundEnabled(_ isEnabled: Bool) {
        guard settings.enableEventCompletionSound != isEnabled else { return }
        settings.enableEventCompletionSound = isEnabled
        scheduleCacheSave()
    }

    func setTaskCompletionSoundChoice(_ choice: CompletionSoundChoice) {
        guard settings.taskCompletionSoundChoice != choice else { return }
        settings.taskCompletionSoundChoice = normalizedCompletionSoundChoice(choice, fallback: .defaultTask)
        scheduleCacheSave()
    }

    func setEventCompletionSoundChoice(_ choice: CompletionSoundChoice) {
        guard settings.eventCompletionSoundChoice != choice else { return }
        settings.eventCompletionSoundChoice = normalizedCompletionSoundChoice(choice, fallback: .defaultEvent)
        scheduleCacheSave()
    }

    @discardableResult
    func importCustomCompletionSound(from sourceURL: URL) throws -> CompletionSoundAsset {
        let asset = try CompletionSoundLibrary.importSound(from: sourceURL)
        settings.customCompletionSounds.insert(asset, at: 0)
        scheduleCacheSave()
        return asset
    }

    func deleteCustomCompletionSound(_ assetID: CompletionSoundAsset.ID) {
        guard let asset = settings.customCompletionSounds.first(where: { $0.id == assetID }) else { return }
        settings.customCompletionSounds.removeAll { $0.id == assetID }
        CompletionSoundLibrary.delete(asset)

        if settings.taskCompletionSoundChoice.customAssetID == assetID {
            settings.taskCompletionSoundChoice = .defaultTask
        }
        if settings.eventCompletionSoundChoice.customAssetID == assetID {
            settings.eventCompletionSoundChoice = .defaultEvent
        }
        scheduleCacheSave()
    }

    func updateLocalNotificationsEnabled(_ isEnabled: Bool) {
        settings.enableLocalNotifications = isEnabled
        Task {
            scheduleCacheSave()
            await runNotificationSync(requestAuthorization: isEnabled)
        }
    }

    func requestEnableLocalNotifications() async -> NotificationAuthorizationOutcome {
        let outcome = await notificationScheduler.authorizationOutcome(requestAuthorization: true)
        switch outcome {
        case .authorized:
            settings.enableLocalNotifications = true
            scheduleCacheSave()
            await runNotificationSync(requestAuthorization: false)
            return .authorized
        case .denied, .notDetermined:
            settings.enableLocalNotifications = false
            scheduleCacheSave()
            await runNotificationSync(requestAuthorization: false)
            return .denied
        }
    }

    func setTaskReminderThresholdDays(_ days: Int) {
        let clamped = max(0, min(365, days))
        guard settings.taskReminderThresholdDays != clamped else { return }
        settings.taskReminderThresholdDays = clamped
        Task {
            scheduleCacheSave()
            await synchronizeLocalNotifications()
        }
    }

    func setTaskReminderTime(hour: Int, minute: Int) {
        let h = max(0, min(23, hour))
        let m = max(0, min(59, minute))
        guard settings.taskReminderHour != h || settings.taskReminderMinute != m else { return }
        settings.taskReminderHour = h
        settings.taskReminderMinute = m
        Task {
            scheduleCacheSave()
            await synchronizeLocalNotifications()
        }
    }

    func completeOnboarding() {
        settings.hasCompletedOnboarding = true
        Task {
            scheduleCacheSave()
        }
    }

    func resetOnboarding() {
        settings.hasCompletedOnboarding = false
        settings.hasSeenFeatureTour = false
        Task {
            scheduleCacheSave()
        }
    }

    func markFeatureTourSeen() {
        guard settings.hasSeenFeatureTour == false else { return }
        settings.hasSeenFeatureTour = true
        Task {
            scheduleCacheSave()
        }
    }

    func clearFailureState() {
        if case .failed = syncState {
            syncState = .idle
        }

        if case .failed = authState {
            authState = account.map(AuthState.signedIn) ?? .signedOut
        }

        lastMutationError = nil
        lastCacheLoadWarning = nil
        syncFailureKind = nil
    }

    func replayPendingMutations() async {
        guard let activeAccountID = account?.id else { return }
        guard pendingMutations.isEmpty == false else { return }
        AppLogger.info("replay start", category: .replay, metadata: ["queued": String(pendingMutations.count)])
        // Serialise replay loops: `createTask` / `createEvent` spawn detached
        // `Task { replayPendingMutations() }` calls and `refreshNow` also
        // calls us. Without this flag two loops can pass each mutation's
        // "still queued" guard simultaneously and both fire insertTask,
        // creating a duplicate on Google.
        guard isReplayingMutations == false else { return }
        isReplayingMutations = true
        defer { isReplayingMutations = false }

        // Cap how many times we re-enter the loop to avoid spinning on a
        // mutation that keeps getting retried (transient error that never
        // resolves within this activation). Each pass we only keep going
        // if we made progress — which guarantees termination either by
        // draining or by stabilising on a non-shrinking queue.
        let maxPasses = 4
        let policy = BackoffPolicy.nearRealtime
        var processedIDs: Set<PendingMutation.ID> = []
        for _ in 0..<maxPasses {
            let now = Date()
            // Skip quarantined mutations (they require explicit user action
            // via Diagnostics) and mutations whose backoff window hasn't
            // elapsed yet (we'll pick them up on a later call).
            let snapshot = pendingMutations.filter {
                processedIDs.contains($0.id) == false
                    && ($0.accountID ?? activeAccountID) == activeAccountID
                    && settings.cloudSyncTargets.allows($0.resourceType)
                    && $0.isReadyToReplay(now: now, policy: policy)
            }
            guard snapshot.isEmpty == false else { break }
            for mutation in snapshot {
                guard pendingMutations.contains(where: { $0.id == mutation.id }) else {
                    processedIDs.insert(mutation.id)
                    continue
                }
                logReplayAttempt(mutation)
                await replay(mutation)
                processedIDs.insert(mutation.id)
            }
            // If nothing new was added during this pass, we're done.
            let remaining = pendingMutations.filter { processedIDs.contains($0.id) == false }
            if remaining.isEmpty { break }
        }
    }

    // On a transient failure, bump attempt count + stamp the backoff timer on
    // the mutation. When we hit BackoffPolicy.maxAttempts, the mutation moves
    // into quarantine and stops auto-replaying. The user can retry or discard
    // from DiagnosticsView.
    @discardableResult
    private func markMutationTransientFailure(_ mutationID: PendingMutation.ID, error: Error) -> PendingMutation? {
        syncFailureKind = SyncFailureKind.classify(error)
        guard let idx = pendingMutations.firstIndex(where: { $0.id == mutationID }) else { return nil }
        pendingMutations[idx].attemptCount += 1
        pendingMutations[idx].lastAttemptAt = Date()
        pendingMutations[idx].lastErrorSummary = error.localizedDescription
        if pendingMutations[idx].attemptCount >= BackoffPolicy.nearRealtime.maxAttempts {
            pendingMutations[idx].quarantinedAt = Date()
            AppLogger.warn("mutation quarantined after \(pendingMutations[idx].attemptCount) attempts", category: .replay, metadata: [
                "id": mutationID.uuidString,
                "resourceType": pendingMutations[idx].resourceType.rawValue,
                "action": pendingMutations[idx].action.rawValue,
                "error": GoogleDiagnostics.sanitizedErrorDescription(error)
            ])
        }
        return pendingMutations[idx]
    }

    // Count of mutations that have exceeded the retry ceiling. Drives the
    // AppStatusBanner warning + the DiagnosticsView quarantine section.
    var quarantinedMutationCount: Int {
        pendingMutations.filter(\.isQuarantined).count
    }

    var invalidPayloadMutationCount: Int {
        pendingMutations.filter {
            $0.isQuarantined
                && $0.isConflict == false
                && (($0.lastErrorSummary ?? "").hasPrefix("Invalid payload"))
        }.count
    }

    // Releases a quarantined mutation back into the automatic replay loop.
    // Called from the Diagnostics "Retry" button.
    @discardableResult
    func requeueQuarantinedMutation(id: PendingMutation.ID) -> Bool {
        guard let idx = pendingMutations.firstIndex(where: { $0.id == id }) else { return false }
        pendingMutations[idx].attemptCount = 0
        pendingMutations[idx].lastAttemptAt = nil
        pendingMutations[idx].lastErrorSummary = nil
        pendingMutations[idx].quarantinedAt = nil
        pendingMutations[idx].conflictedAt = nil
        let updatedMutation = pendingMutations[idx]
        Task {
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.pendingMutationUpdated(updatedMutation))
            await replayPendingMutations()
        }
        return true
    }

    // On a 412 Precondition Failed from the replay path, instead of dropping
    // the queued write + silently refreshing (which lost the user's edit),
    // flag the mutation as a conflict. The user resolves it from Diagnostics
    // via "Keep my change" (forceOverwriteConflictedMutation — re-issues the
    // write without etag) or "Discard" (clearPendingMutation).
    @discardableResult
    private func markMutationConflict(_ mutationID: PendingMutation.ID, error: Error) -> PendingMutation? {
        syncFailureKind = SyncFailureKind.classify(error)
        guard let idx = pendingMutations.firstIndex(where: { $0.id == mutationID }) else { return nil }
        pendingMutations[idx].lastAttemptAt = Date()
        pendingMutations[idx].lastErrorSummary = "Server changed underneath — choose whose change wins."
        pendingMutations[idx].quarantinedAt = Date()
        pendingMutations[idx].conflictedAt = Date()
        AppLogger.warn("mutation conflict — 412 on replay", category: .replay, metadata: [
            "id": mutationID.uuidString,
            "resourceType": pendingMutations[idx].resourceType.rawValue,
            "action": pendingMutations[idx].action.rawValue,
            "error": GoogleDiagnostics.sanitizedErrorDescription(error)
        ])
        return pendingMutations[idx]
    }

    @discardableResult
    private func markMutationInvalidPayload(_ mutationID: PendingMutation.ID, error: GoogleAPIError) -> PendingMutation? {
        syncFailureKind = SyncFailureKind.classify(error)
        guard let idx = pendingMutations.firstIndex(where: { $0.id == mutationID }) else { return nil }
        pendingMutations[idx].lastAttemptAt = Date()
        pendingMutations[idx].lastErrorSummary = "Invalid payload — Google rejected this queued write. Copy the payload, fix the source data, then retry."
        pendingMutations[idx].quarantinedAt = Date()
        AppLogger.warn("mutation quarantined — invalid payload", category: .replay, metadata: [
            "id": mutationID.uuidString,
            "resourceType": pendingMutations[idx].resourceType.rawValue,
            "action": pendingMutations[idx].action.rawValue,
            "error": GoogleDiagnostics.sanitizedErrorDescription(error)
        ])
        return pendingMutations[idx]
    }

    // Force-reissue a conflicted mutation without If-Match so Google accepts
    // it as an unconditional overwrite of the current server state. Called
    // from the Diagnostics "Keep my change" button.
    @discardableResult
    func forceOverwriteConflictedMutation(id: PendingMutation.ID) async -> Bool {
        guard let mutation = pendingMutations.first(where: { $0.id == id }),
              mutation.isConflict
        else { return false }
        guard settings.cloudSyncTargets.allows(mutation.resourceType) else {
            lastMutationError = "This queued write is being kept local because its cloud sync surface is disabled in Settings."
            return false
        }
        do {
            var changeSet = LocalCacheChangeSetBuilder.pendingMutationDeleted(mutation)
            switch (mutation.resourceType, mutation.action) {
            case (.task, .update):
                let payload = try PendingMutationEncoder.decodeTaskUpdate(mutation.payload)
                let originalTask = task(id: payload.taskID)
                let updated = try await tasksClient.updateTask(
                    taskListID: payload.taskListID,
                    taskID: payload.taskID,
                    title: payload.title,
                    notes: payload.notes,
                    dueDate: payload.dueDate,
                    ifMatch: nil
                )
                upsert(updated)
                changeSet.formUnion(LocalCacheChangeSetBuilder.taskUpdated(old: originalTask, new: updated))
            case (.task, .completion):
                let payload = try PendingMutationEncoder.decodeTaskCompletion(mutation.payload)
                let originalTask = task(id: payload.taskID)
                let stub = TaskMirror(
                    id: payload.taskID,
                    taskListID: payload.taskListID,
                    parentID: nil,
                    title: "",
                    notes: "",
                    status: payload.isCompleted ? .completed : .needsAction,
                    dueDate: nil,
                    completedAt: nil,
                    isDeleted: false,
                    isHidden: false,
                    position: nil,
                    etag: nil,
                    updatedAt: nil
                )
                let updated = try await tasksClient.setTaskCompleted(payload.isCompleted, task: stub)
                upsert(updated)
                changeSet.formUnion(LocalCacheChangeSetBuilder.taskUpdated(old: originalTask, new: updated))
            case (.task, .delete):
                let payload = try PendingMutationEncoder.decodeTaskDelete(mutation.payload)
                let deletedTask = task(id: payload.taskID)
                try await tasksClient.deleteTask(taskListID: payload.taskListID, taskID: payload.taskID, ifMatch: nil)
                removeTask(id: payload.taskID)
                if let deletedTask {
                    changeSet.formUnion(LocalCacheChangeSetBuilder.taskDeleted(deletedTask))
                }
            case (.event, .update):
                let payload = try PendingMutationEncoder.decodeEventUpdate(mutation.payload)
                let originalEvent = event(id: payload.eventID)
                let updated = try await calendarClient.updateEvent(
                    calendarID: payload.calendarID,
                    eventID: payload.eventID,
                    summary: payload.summary,
                    details: payload.details,
                    startDate: payload.startDate,
                    endDate: payload.endDate,
                    isAllDay: payload.isAllDay,
                    reminderMinutes: payload.reminderMinutes,
                    location: payload.location,
                    recurrence: payload.recurrence,
                    attendeeEmails: payload.attendeeEmails,
                    sendUpdates: payload.notifyGuests ? "all" : "none",
                    addGoogleMeet: payload.addGoogleMeet,
                    colorId: payload.colorId,
                    startTimeZoneID: payload.startTimeZoneID,
                    endTimeZoneID: payload.endTimeZoneID,
                    hcbTaskID: payload.hcbTaskID,
                    transparency: payload.transparency,
                    visibility: payload.visibility,
                    availabilityHold: payload.availabilityHold,
                    clearAvailabilityHoldMetadata: payload.clearAvailabilityHoldMetadata,
                    ifMatch: nil
                )
                upsert(updated)
                changeSet.formUnion(LocalCacheChangeSetBuilder.eventUpdated(old: originalEvent, new: updated))
            case (.event, .delete):
                let payload = try PendingMutationEncoder.decodeEventDelete(mutation.payload)
                let deletedEvent = event(id: payload.eventID)
                try await calendarClient.deleteEvent(calendarID: payload.calendarID, eventID: payload.eventID, ifMatch: nil)
                removeEvent(id: payload.eventID)
                if let deletedEvent {
                    changeSet.formUnion(LocalCacheChangeSetBuilder.eventDeleted(deletedEvent))
                }
            default:
                // Create mutations never 412 (no etag). Only updates/deletes
                // reach this state.
                return false
            }
            pendingMutations.removeAll { $0.id == id }
            scheduleCacheSave(changeSet: changeSet)
            await synchronizeLocalNotifications()
            return true
        } catch {
            lastMutationError = "Overwrite failed: \(error.localizedDescription)"
            return false
        }
    }

    var conflictedMutationCount: Int {
        pendingMutations.filter(\.isConflict).count
    }

    private func replay(_ mutation: PendingMutation) async {
        switch (mutation.resourceType, mutation.action) {
        case (.task, .create):
            await replayTaskCreate(mutation)
        case (.event, .create):
            await replayEventCreate(mutation)
        case (.task, .update):
            await replayTaskUpdate(mutation)
        case (.task, .completion):
            await replayTaskCompletion(mutation)
        case (.task, .delete):
            await replayTaskDelete(mutation)
        case (.event, .update):
            await replayEventUpdate(mutation)
        case (.event, .delete):
            await replayEventDelete(mutation)
        default:
            pendingMutations.removeAll { $0.id == mutation.id }
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.pendingMutationDeleted(mutation))
        }
    }

    private func replayTaskCreate(_ mutation: PendingMutation) async {
        do {
            let payload = try PendingMutationEncoder.decodeTaskCreate(mutation.payload)
            let parent = payload.parentID.flatMap { id -> String? in
                OptimisticID.isPending(id) ? nil : id
            }
            let created = try await tasksClient.insertTask(
                taskListID: payload.taskListID,
                title: payload.title,
                notes: payload.notes,
                dueDate: payload.dueDate,
                parent: parent
            )
            let localTask = task(id: payload.localID)
            removeTask(id: payload.localID)
            upsert(created)
            logReplayAccepted(mutation, acceptedID: created.id)
            pendingMutations.removeAll { $0.id == mutation.id }
            var changeSets = [
                LocalCacheChangeSetBuilder.taskInserted(created),
                LocalCacheChangeSetBuilder.pendingMutationDeleted(mutation)
            ]
            if let localTask {
                changeSets.append(LocalCacheChangeSetBuilder.taskDeleted(localTask))
            }
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.combined(changeSets))
            await synchronizeLocalNotifications()
        } catch let error as GoogleAPIError where error.isTransient {
            let updatedMutation = markMutationTransientFailure(mutation.id, error: error)
            lastMutationError = "Can't reach Google right now — queued for automatic retry."
            if let updatedMutation {
                scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.pendingMutationUpdated(updatedMutation))
            }
        } catch let error as GoogleAPIError where error.isInvalidPayload {
            let updatedMutation = markMutationInvalidPayload(mutation.id, error: error)
            lastMutationError = "Task couldn't be created on Google. The queued payload was preserved in Sync Issues."
            if let updatedMutation {
                scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.pendingMutationUpdated(updatedMutation))
            }
        } catch {
            var changeSets = [LocalCacheChangeSetBuilder.pendingMutationDeleted(mutation)]
            if let payload = try? PendingMutationEncoder.decodeTaskCreate(mutation.payload) {
                if let localTask = task(id: payload.localID) {
                    changeSets.append(LocalCacheChangeSetBuilder.taskDeleted(localTask))
                }
                removeTask(id: payload.localID)
            }
            pendingMutations.removeAll { $0.id == mutation.id }
            lastMutationError = "Task couldn't be created: \(error.localizedDescription)"
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.combined(changeSets))
        }
    }

    private func replayTaskUpdate(_ mutation: PendingMutation) async {
        do {
            let payload = try PendingMutationEncoder.decodeTaskUpdate(mutation.payload)
            let originalTask = task(id: payload.taskID)
            let updated = try await tasksClient.updateTask(
                taskListID: payload.taskListID,
                taskID: payload.taskID,
                title: payload.title,
                notes: payload.notes,
                dueDate: payload.dueDate,
                ifMatch: payload.etagSnapshot
            )
            upsert(updated)
            logReplayAccepted(mutation, acceptedID: updated.id)
            pendingMutations.removeAll { $0.id == mutation.id }
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.combined(
                LocalCacheChangeSetBuilder.taskUpdated(old: originalTask, new: updated),
                LocalCacheChangeSetBuilder.pendingMutationDeleted(mutation)
            ))
            await synchronizeLocalNotifications()
        } catch let error as GoogleAPIError where error.isTransient {
            let updatedMutation = markMutationTransientFailure(mutation.id, error: error)
            lastMutationError = "Can't reach Google right now — queued for automatic retry."
            if let updatedMutation {
                scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.pendingMutationUpdated(updatedMutation))
            }
        } catch let error as GoogleAPIError where error.isInvalidPayload {
            let updatedMutation = markMutationInvalidPayload(mutation.id, error: error)
            lastMutationError = "Queued task update was preserved in Sync Issues because Google rejected its payload."
            if let updatedMutation {
                scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.pendingMutationUpdated(updatedMutation))
            }
        } catch let error as GoogleAPIError where error == .preconditionFailed {
            // Queued edit raced a server-side change. Don't silently drop —
            // surface as a conflict the user resolves from Diagnostics.
            if let updatedMutation = markMutationConflict(mutation.id, error: error) {
                scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.pendingMutationUpdated(updatedMutation))
            }
            await refreshNow()
        } catch {
            pendingMutations.removeAll { $0.id == mutation.id }
            lastMutationError = "Queued task update failed: \(error.localizedDescription)"
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.pendingMutationDeleted(mutation))
        }
    }

    private func replayTaskCompletion(_ mutation: PendingMutation) async {
        do {
            let payload = try PendingMutationEncoder.decodeTaskCompletion(mutation.payload)
            let originalTask = task(id: payload.taskID)
            // Reconstruct a minimal TaskMirror so tasksClient.setTaskCompleted
            // can pass the snapshot etag via If-Match.
            let stub = TaskMirror(
                id: payload.taskID,
                taskListID: payload.taskListID,
                parentID: nil,
                title: "",
                notes: "",
                status: payload.isCompleted ? .completed : .needsAction,
                dueDate: nil,
                completedAt: nil,
                isDeleted: false,
                isHidden: false,
                position: nil,
                etag: payload.etagSnapshot,
                updatedAt: nil
            )
            let updated = try await tasksClient.setTaskCompleted(payload.isCompleted, task: stub)
            upsert(updated)
            logReplayAccepted(mutation, acceptedID: updated.id)
            pendingMutations.removeAll { $0.id == mutation.id }
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.combined(
                LocalCacheChangeSetBuilder.taskUpdated(old: originalTask, new: updated),
                LocalCacheChangeSetBuilder.pendingMutationDeleted(mutation)
            ))
            await synchronizeLocalNotifications()
        } catch let error as GoogleAPIError where error.isTransient {
            let updatedMutation = markMutationTransientFailure(mutation.id, error: error)
            lastMutationError = "Can't reach Google right now — queued for automatic retry."
            if let updatedMutation {
                scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.pendingMutationUpdated(updatedMutation))
            }
        } catch let error as GoogleAPIError where error.isInvalidPayload {
            let updatedMutation = markMutationInvalidPayload(mutation.id, error: error)
            lastMutationError = "Queued task completion was preserved in Sync Issues because Google rejected its payload."
            if let updatedMutation {
                scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.pendingMutationUpdated(updatedMutation))
            }
        } catch let error as GoogleAPIError where error == .preconditionFailed {
            if let updatedMutation = markMutationConflict(mutation.id, error: error) {
                scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.pendingMutationUpdated(updatedMutation))
            }
            await refreshNow()
        } catch {
            pendingMutations.removeAll { $0.id == mutation.id }
            lastMutationError = "Queued completion failed: \(error.localizedDescription)"
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.pendingMutationDeleted(mutation))
        }
    }

    private func replayTaskDelete(_ mutation: PendingMutation) async {
        do {
            let payload = try PendingMutationEncoder.decodeTaskDelete(mutation.payload)
            let deletedTask = task(id: payload.taskID)
            try await tasksClient.deleteTask(
                taskListID: payload.taskListID,
                taskID: payload.taskID,
                ifMatch: payload.etagSnapshot
            )
            removeTask(id: payload.taskID)
            logReplayAccepted(mutation, acceptedID: payload.taskID)
            pendingMutations.removeAll { $0.id == mutation.id }
            var changeSets = [LocalCacheChangeSetBuilder.pendingMutationDeleted(mutation)]
            if let deletedTask {
                changeSets.append(LocalCacheChangeSetBuilder.taskDeleted(deletedTask))
            }
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.combined(changeSets))
            await synchronizeLocalNotifications()
        } catch let error as GoogleAPIError where error.isTransient {
            let updatedMutation = markMutationTransientFailure(mutation.id, error: error)
            lastMutationError = "Can't reach Google right now — queued for automatic retry."
            if let updatedMutation {
                scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.pendingMutationUpdated(updatedMutation))
            }
        } catch let error as GoogleAPIError where error.isInvalidPayload {
            let updatedMutation = markMutationInvalidPayload(mutation.id, error: error)
            lastMutationError = "Queued task delete was preserved in Sync Issues because Google rejected its payload."
            if let updatedMutation {
                scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.pendingMutationUpdated(updatedMutation))
            }
        } catch let error as GoogleAPIError where error == .preconditionFailed {
            if let updatedMutation = markMutationConflict(mutation.id, error: error) {
                scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.pendingMutationUpdated(updatedMutation))
            }
            await refreshNow()
        } catch {
            pendingMutations.removeAll { $0.id == mutation.id }
            lastMutationError = "Queued delete failed: \(error.localizedDescription)"
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.pendingMutationDeleted(mutation))
        }
    }

    private func replayEventUpdate(_ mutation: PendingMutation) async {
        do {
            let payload = try PendingMutationEncoder.decodeEventUpdate(mutation.payload)
            let originalEvent = event(id: payload.eventID)
            let updated = try await calendarClient.updateEvent(
                calendarID: payload.calendarID,
                eventID: payload.eventID,
                summary: payload.summary,
                details: payload.details,
                startDate: payload.startDate,
                endDate: payload.endDate,
                isAllDay: payload.isAllDay,
                reminderMinutes: payload.reminderMinutes,
                location: payload.location,
                recurrence: payload.recurrence,
                attendeeEmails: payload.attendeeEmails,
                sendUpdates: payload.notifyGuests ? "all" : "none",
                addGoogleMeet: payload.addGoogleMeet,
                colorId: payload.colorId,
                startTimeZoneID: payload.startTimeZoneID,
                endTimeZoneID: payload.endTimeZoneID,
                hcbTaskID: payload.hcbTaskID,
                transparency: payload.transparency,
                visibility: payload.visibility,
                availabilityHold: payload.availabilityHold,
                clearAvailabilityHoldMetadata: payload.clearAvailabilityHoldMetadata,
                ifMatch: payload.etagSnapshot
            )
            upsert(updated)
            logReplayAccepted(mutation, acceptedID: updated.id)
            pendingMutations.removeAll { $0.id == mutation.id }
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.combined(
                LocalCacheChangeSetBuilder.eventUpdated(old: originalEvent, new: updated),
                LocalCacheChangeSetBuilder.pendingMutationDeleted(mutation)
            ))
            await synchronizeLocalNotifications()
        } catch let error as GoogleAPIError where error.isTransient {
            let updatedMutation = markMutationTransientFailure(mutation.id, error: error)
            lastMutationError = "Can't reach Google right now — queued for automatic retry."
            if let updatedMutation {
                scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.pendingMutationUpdated(updatedMutation))
            }
        } catch let error as GoogleAPIError where error.isInvalidPayload {
            let updatedMutation = markMutationInvalidPayload(mutation.id, error: error)
            lastMutationError = "Queued event update was preserved in Sync Issues because Google rejected its payload."
            if let updatedMutation {
                scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.pendingMutationUpdated(updatedMutation))
            }
        } catch let error as GoogleAPIError where error == .preconditionFailed {
            if let updatedMutation = markMutationConflict(mutation.id, error: error) {
                scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.pendingMutationUpdated(updatedMutation))
            }
            await refreshNow()
        } catch {
            pendingMutations.removeAll { $0.id == mutation.id }
            lastMutationError = "Queued event update failed: \(error.localizedDescription)"
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.pendingMutationDeleted(mutation))
        }
    }

    private func replayEventDelete(_ mutation: PendingMutation) async {
        do {
            let payload = try PendingMutationEncoder.decodeEventDelete(mutation.payload)
            let deletedEvent = event(id: payload.eventID)
            try await calendarClient.deleteEvent(
                calendarID: payload.calendarID,
                eventID: payload.eventID,
                ifMatch: payload.etagSnapshot
            )
            removeEvent(id: payload.eventID)
            logReplayAccepted(mutation, acceptedID: payload.eventID)
            pendingMutations.removeAll { $0.id == mutation.id }
            var changeSets = [LocalCacheChangeSetBuilder.pendingMutationDeleted(mutation)]
            if let deletedEvent {
                changeSets.append(LocalCacheChangeSetBuilder.eventDeleted(deletedEvent))
            }
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.combined(changeSets))
            await synchronizeLocalNotifications()
        } catch let error as GoogleAPIError where error.isTransient {
            let updatedMutation = markMutationTransientFailure(mutation.id, error: error)
            lastMutationError = "Can't reach Google right now — queued for automatic retry."
            if let updatedMutation {
                scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.pendingMutationUpdated(updatedMutation))
            }
        } catch let error as GoogleAPIError where error.isInvalidPayload {
            let updatedMutation = markMutationInvalidPayload(mutation.id, error: error)
            lastMutationError = "Queued event delete was preserved in Sync Issues because Google rejected its payload."
            if let updatedMutation {
                scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.pendingMutationUpdated(updatedMutation))
            }
        } catch let error as GoogleAPIError where error == .preconditionFailed {
            if let updatedMutation = markMutationConflict(mutation.id, error: error) {
                scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.pendingMutationUpdated(updatedMutation))
            }
            await refreshNow()
        } catch {
            pendingMutations.removeAll { $0.id == mutation.id }
            lastMutationError = "Queued event delete failed: \(error.localizedDescription)"
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.pendingMutationDeleted(mutation))
        }
    }

    private func replayEventCreate(_ mutation: PendingMutation) async {
        do {
            let payload = try PendingMutationEncoder.decodeEventCreate(mutation.payload)
            let created = try await calendarClient.insertEvent(
                calendarID: payload.calendarID,
                summary: payload.summary,
                details: payload.details,
                startDate: payload.startDate,
                endDate: payload.endDate,
                isAllDay: payload.isAllDay,
                reminderMinutes: payload.reminderMinutes,
                location: payload.location,
                recurrence: payload.recurrence,
                attendeeEmails: payload.attendeeEmails,
                sendUpdates: payload.notifyGuests ? "all" : "none",
                addGoogleMeet: payload.addGoogleMeet,
                colorId: payload.colorId,
                startTimeZoneID: payload.startTimeZoneID,
                endTimeZoneID: payload.endTimeZoneID,
                transparency: payload.transparency,
                visibility: payload.visibility,
                hcbTaskID: payload.hcbTaskID,
                availabilityHold: payload.availabilityHold
            )
            let localEvent = event(id: payload.localID)
            removeEvent(id: payload.localID)
            upsert(created)
            logReplayAccepted(mutation, acceptedID: created.id)
            pendingMutations.removeAll { $0.id == mutation.id }
            var changeSets = [
                LocalCacheChangeSetBuilder.eventInserted(created),
                LocalCacheChangeSetBuilder.pendingMutationDeleted(mutation)
            ]
            if let localEvent {
                changeSets.append(LocalCacheChangeSetBuilder.eventDeleted(localEvent))
            }
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.combined(changeSets))
            await synchronizeLocalNotifications()
        } catch let error as GoogleAPIError where error.isTransient {
            let updatedMutation = markMutationTransientFailure(mutation.id, error: error)
            lastMutationError = "Can't reach Google right now — queued for automatic retry."
            if let updatedMutation {
                scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.pendingMutationUpdated(updatedMutation))
            }
        } catch let error as GoogleAPIError where error.isInvalidPayload {
            let updatedMutation = markMutationInvalidPayload(mutation.id, error: error)
            lastMutationError = "Event couldn't be created on Google. The queued payload was preserved in Sync Issues."
            if let updatedMutation {
                scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.pendingMutationUpdated(updatedMutation))
            }
        } catch {
            var changeSets = [LocalCacheChangeSetBuilder.pendingMutationDeleted(mutation)]
            if let payload = try? PendingMutationEncoder.decodeEventCreate(mutation.payload) {
                if let localEvent = event(id: payload.localID) {
                    changeSets.append(LocalCacheChangeSetBuilder.eventDeleted(localEvent))
                }
                removeEvent(id: payload.localID)
            }
            pendingMutations.removeAll { $0.id == mutation.id }
            lastMutationError = "Event couldn't be created: \(error.localizedDescription)"
            scheduleCacheSave(changeSet: LocalCacheChangeSetBuilder.combined(changeSets))
        }
    }

    private func requireAccount(mutationDescription: String) -> Bool {
        guard account != nil else {
            lastMutationError = "Sign in to Google in Settings to \(mutationDescription)."
            return false
        }
        return true
    }

    // Reject mutations that target an item whose create is still waiting to
    // hit Google. The server-side ID doesn't exist yet, so PATCH/DELETE would
    // 404 and leave the UI in an indeterminate state.
    private func requirePersisted(_ id: String) -> Bool {
        guard OptimisticID.isPending(id) else { return true }
        lastMutationError = "This item is still syncing to Google. Try again once the pending sync indicator clears."
        return false
    }

    @discardableResult
    private func enqueuePendingMutation(_ mutation: PendingMutation) -> PendingMutation {
        let mutation = account.map { mutation.accountStamped(accountID: $0.id) } ?? mutation
        pendingMutations.append(mutation)
        AppLogger.info(
            "mutation queued",
            category: .mutation,
            metadata: Self.mutationLogMetadata(mutation).merging([
                "queuedCount": String(pendingMutations.count)
            ]) { _, new in new }
        )
        return mutation
    }

    private func logReplayAttempt(_ mutation: PendingMutation) {
        AppLogger.info(
            "mutation replay attempt",
            category: .replay,
            metadata: Self.mutationLogMetadata(mutation)
        )
    }

    private func logReplayAccepted(_ mutation: PendingMutation, acceptedID: String? = nil) {
        var metadata = Self.mutationLogMetadata(mutation)
        if let acceptedID {
            metadata["acceptedID"] = GoogleDiagnostics.redactedIdentifier(acceptedID)
        }
        AppLogger.info("mutation replay accepted", category: .replay, metadata: metadata)
    }

    private static func mutationLogMetadata(_ mutation: PendingMutation) -> [String: String] {
        var metadata: [String: String] = [
            "id": mutation.id.uuidString,
            "resourceType": mutation.resourceType.rawValue,
            "resourceID": GoogleDiagnostics.redactedIdentifier(mutation.resourceID),
            "action": mutation.action.rawValue,
            "attempt": String(mutation.attemptCount),
            "accountID": mutation.accountID.map(GoogleDiagnostics.redactedIdentifier) ?? "legacy"
        ]
        metadata.merge(mutationPayloadMetadata(mutation)) { _, new in new }
        return metadata
    }

    private static func mutationPayloadMetadata(_ mutation: PendingMutation) -> [String: String] {
        switch (mutation.resourceType, mutation.action) {
        case (.task, .create):
            guard let payload = try? PendingMutationEncoder.decodeTaskCreate(mutation.payload) else { return ["payload": "decodeFailed"] }
            return [
                "taskListID": GoogleDiagnostics.redactedIdentifier(payload.taskListID),
                "localID": GoogleDiagnostics.redactedIdentifier(payload.localID),
                "hasNotes": String(payload.notes.isEmpty == false),
                "hasDueDate": String(payload.dueDate != nil),
                "hasParent": String(payload.parentID?.isEmpty == false)
            ]
        case (.task, .update):
            guard let payload = try? PendingMutationEncoder.decodeTaskUpdate(mutation.payload) else { return ["payload": "decodeFailed"] }
            return [
                "taskListID": GoogleDiagnostics.redactedIdentifier(payload.taskListID),
                "taskID": GoogleDiagnostics.redactedIdentifier(payload.taskID),
                "hasNotes": String(payload.notes.isEmpty == false),
                "hasDueDate": String(payload.dueDate != nil),
                "hasIfMatch": String(payload.etagSnapshot?.isEmpty == false)
            ]
        case (.task, .completion):
            guard let payload = try? PendingMutationEncoder.decodeTaskCompletion(mutation.payload) else { return ["payload": "decodeFailed"] }
            return [
                "taskListID": GoogleDiagnostics.redactedIdentifier(payload.taskListID),
                "taskID": GoogleDiagnostics.redactedIdentifier(payload.taskID),
                "isCompleted": String(payload.isCompleted),
                "hasIfMatch": String(payload.etagSnapshot?.isEmpty == false)
            ]
        case (.task, .delete):
            guard let payload = try? PendingMutationEncoder.decodeTaskDelete(mutation.payload) else { return ["payload": "decodeFailed"] }
            return [
                "taskListID": GoogleDiagnostics.redactedIdentifier(payload.taskListID),
                "taskID": GoogleDiagnostics.redactedIdentifier(payload.taskID),
                "hasIfMatch": String(payload.etagSnapshot?.isEmpty == false)
            ]
        case (.event, .create):
            guard let payload = try? PendingMutationEncoder.decodeEventCreate(mutation.payload) else { return ["payload": "decodeFailed"] }
            return eventPayloadMetadata(
                calendarID: payload.calendarID,
                eventID: payload.localID,
                details: payload.details,
                isAllDay: payload.isAllDay,
                reminderMinutes: payload.reminderMinutes,
                location: payload.location,
                recurrence: payload.recurrence,
                attendeeEmails: payload.attendeeEmails,
                notifyGuests: payload.notifyGuests,
                addGoogleMeet: payload.addGoogleMeet,
                colorId: payload.colorId,
                hcbTaskID: payload.hcbTaskID,
                etag: nil
            )
        case (.event, .update):
            guard let payload = try? PendingMutationEncoder.decodeEventUpdate(mutation.payload) else { return ["payload": "decodeFailed"] }
            return eventPayloadMetadata(
                calendarID: payload.calendarID,
                eventID: payload.eventID,
                details: payload.details,
                isAllDay: payload.isAllDay,
                reminderMinutes: payload.reminderMinutes,
                location: payload.location,
                recurrence: payload.recurrence,
                attendeeEmails: payload.attendeeEmails,
                notifyGuests: payload.notifyGuests,
                addGoogleMeet: payload.addGoogleMeet,
                colorId: payload.colorId,
                hcbTaskID: payload.hcbTaskID,
                etag: payload.etagSnapshot
            )
        case (.event, .delete):
            guard let payload = try? PendingMutationEncoder.decodeEventDelete(mutation.payload) else { return ["payload": "decodeFailed"] }
            return [
                "calendarID": GoogleDiagnostics.redactedIdentifier(payload.calendarID),
                "eventID": GoogleDiagnostics.redactedIdentifier(payload.eventID),
                "hasIfMatch": String(payload.etagSnapshot?.isEmpty == false)
            ]
        default:
            return [:]
        }
    }

    private static func eventPayloadMetadata(
        calendarID: String,
        eventID: String,
        details: String,
        isAllDay: Bool,
        reminderMinutes: Int?,
        location: String,
        recurrence: [String],
        attendeeEmails: [String],
        notifyGuests: Bool,
        addGoogleMeet: Bool,
        colorId: String?,
        hcbTaskID: String?,
        etag: String?
    ) -> [String: String] {
        [
            "calendarID": GoogleDiagnostics.redactedIdentifier(calendarID),
            "eventID": GoogleDiagnostics.redactedIdentifier(eventID),
            "hasDetails": String(details.isEmpty == false),
            "isAllDay": String(isAllDay),
            "hasReminder": String(reminderMinutes != nil),
            "hasLocation": String(location.isEmpty == false),
            "recurrenceCount": String(recurrence.count),
            "attendeeCount": String(attendeeEmails.count),
            "notifyGuests": String(notifyGuests),
            "addGoogleMeet": String(addGoogleMeet),
            "hasColor": String(colorId?.isEmpty == false),
            "hasHCBTask": String(hcbTaskID?.isEmpty == false),
            "hasIfMatch": String(etag?.isEmpty == false)
        ]
    }

    private func beginMutation() {
        mutationCount += 1
        lastMutationError = nil
        publishSessionSurfaceStores()
    }

    private func endMutation(error: Error?) {
        mutationCount = max(0, mutationCount - 1)
        if let error {
            lastMutationError = error.localizedDescription
            syncFailureKind = SyncFailureKind.classify(error)
            if syncFailureKind == .authRequired {
                authState = .failed(error.localizedDescription)
            }
            let meta = GoogleDiagnostics.errorMetadata(error)
            AppLogger.warn("mutation failed", category: .mutation, metadata: meta)
        } else if mutationCount == 0 {
            syncFailureKind = nil
        }
        publishSessionSurfaceStores()
    }

    func forceFullResync() async {
        syncCheckpoints = []
        await saveCurrentState()
        await refreshNow()
    }

    func clearCachedGoogleDataAndRefresh() async {
        taskLists = []
        tasks = []
        calendars = []
        events = []
        syncCheckpoints = []
        pendingMutations = []
        scheduleRebuildSnapshots()
        syncState = .idle
        await saveCurrentState()
        await spotlightIndexer.removeAll()
        await synchronizeLocalNotifications()

        if account != nil {
            await refreshNow()
        }
    }

    func cacheFilePath() async -> String {
        await cacheStore.cacheFilePath() ?? "In-memory cache only"
    }

    func openLocalCacheFolder() async {
        guard let path = await cacheStore.cacheFilePath() else { return }
        let url = URL(fileURLWithPath: path)
        await MainActor.run {
            NSWorkspace.shared.activateFileViewerSelecting([url])
        }
    }

    func cacheFootprintDescription() async -> String {
        let bytes = await cacheStore.cacheFootprintBytes()
        guard bytes > 0 else { return "No local cache file found" }
        return ByteCountFormatter.string(fromByteCount: bytes, countStyle: .file)
    }

    func diagnosticSummary(cachePath: String) -> String {
        let accountLabel = account?.displayName ?? authState.title
        let lastSync = lastSuccessfulSyncAt?.formatted(date: .abbreviated, time: .standard) ?? "Never"
        let selectedTaskLists = settings.hasConfiguredTaskListSelection ? settings.selectedTaskListIDs.count : taskLists.count
        let selectedCalendars = settings.hasConfiguredCalendarSelection ? settings.selectedCalendarIDs.count : calendars.filter(\.isSelected).count

        return """
        Hot Cross Buns Diagnostics
        Account: \(accountLabel)
        Auth state: \(authState.title)
        Sync state: \(syncState.title)
        Sync mode: \(settings.syncMode.title)
        Last successful sync: \(lastSync)
        Task lists: \(taskLists.count)
        Selected task lists: \(selectedTaskLists)
        Tasks: \(tasks.count)
        Calendars: \(calendars.count)
        Selected calendars: \(selectedCalendars)
        Events: \(events.count)
        Sync checkpoints: \(syncCheckpoints.count)
        Pending writes: \(pendingMutations.count)
        Local reminders: \(settings.enableLocalNotifications ? "enabled" : "disabled")
        Onboarding: \(settings.hasCompletedOnboarding ? "completed" : "not completed")
        Cache path: \(cachePath)
        """
    }

    func toggleCalendar(_ calendarID: CalendarListMirror.ID) {
        guard let index = calendars.firstIndex(where: { $0.id == calendarID }) else {
            return
        }
        calendars[index].isSelected.toggle()
        settings.hasConfiguredCalendarSelection = true
        settings.selectedCalendarIDs = Set(calendars.filter(\.isSelected).map(\.id))
        scheduleRebuildSnapshots()
        scheduleCacheSave()
    }

    func isTaskListSelected(_ taskListID: TaskListMirror.ID) -> Bool {
        if settings.hasConfiguredTaskListSelection {
            return settings.selectedTaskListIDs.contains(taskListID)
        }

        return true
    }

    func toggleTaskList(_ taskListID: TaskListMirror.ID) {
        guard taskLists.contains(where: { $0.id == taskListID }) else {
            return
        }

        var selectedIDs = settings.hasConfiguredTaskListSelection
            ? settings.selectedTaskListIDs
            : Set(taskLists.map(\.id))

        if selectedIDs.contains(taskListID) {
            selectedIDs.remove(taskListID)
        } else {
            selectedIDs.insert(taskListID)
        }

        settings.hasConfiguredTaskListSelection = true
        settings.selectedTaskListIDs = selectedIDs
        scheduleRebuildSnapshots()
        scheduleCacheSave()
    }

    func task(id: TaskMirror.ID) -> TaskMirror? {
        guard let index = taskIndexByID[id],
              tasks.indices.contains(index),
              tasks[index].id == id else {
            // Snapshot rebuilds are intentionally coalesced to the next tick.
            // During that tiny stale-index window, fall back to a scan instead
            // of returning the wrong mirror after an insertion/removal shifts.
            return tasks.first { $0.id == id }
        }
        return tasks[index]
    }

    func searchEntities(
        query: String,
        scope: LocalCacheEntitySearchScope = .all,
        limit: Int = 40
    ) async throws -> [QuickSwitcherEntity]? {
        let parsed = AdvancedSearchParser.parse(query)
        guard parsed.regex == nil else { return nil }
        guard let results = try await cacheStore.searchEntities(query: query, scope: scope, limit: limit) else {
            return nil
        }
        var entities: [QuickSwitcherEntity] = []
        entities.reserveCapacity(results.tasks.count + results.events.count)
        entities += results.tasks.map(QuickSwitcherEntity.task)
        entities += results.events.map(QuickSwitcherEntity.event)

        if scope == .all {
            entities += containerSearchEntities(query: query, limit: max(0, limit - entities.count))
        }
        return Array(entities.prefix(limit))
    }

    private func containerSearchEntities(query: String, limit: Int) -> [QuickSwitcherEntity] {
        guard limit > 0 else { return [] }
        let parsed = AdvancedSearchParser.parse(query)
        guard parsed.regex == nil else { return [] }
        let freeText = parsed.freeText.trimmingCharacters(in: .whitespacesAndNewlines)
        let containers: [QuickSwitcherEntity] =
            taskLists.map(QuickSwitcherEntity.taskList)
            + calendars.map(QuickSwitcherEntity.calendar)
            + settings.customFilters.map(QuickSwitcherEntity.customFilter)
        let structurallyFiltered = containers.filter {
            AdvancedSearchMatcher.matches(
                $0,
                query: parsed,
                calendars: calendars,
                taskLists: taskLists
            )
        }
        if freeText.isEmpty {
            return Array(
                structurallyFiltered
                    .sorted { $0.label.localizedCaseInsensitiveCompare($1.label) == .orderedAscending }
                    .prefix(limit)
            )
        }
        return FuzzySearcher.rank(
            structurallyFiltered,
            query: freeText,
            labelForItem: { $0.label },
            keywordsForItem: { $0.keywords },
            limit: limit
        ).map(\.item)
    }

    func event(id: CalendarEventMirror.ID) -> CalendarEventMirror? {
        guard let index = eventIndexByID[id],
              events.indices.contains(index),
              events[index].id == id else {
            return events.first { $0.id == id }
        }
        return events[index]
    }

    func taskList(id: TaskListMirror.ID) -> TaskListMirror? {
        guard let index = taskListIndexByID[id],
              taskLists.indices.contains(index),
              taskLists[index].id == id else {
            return taskLists.first { $0.id == id }
        }
        return taskLists[index]
    }

    func taskListTitle(for id: TaskListMirror.ID, fallback: String = "Unknown list") -> String {
        taskListTitleByID[id] ?? fallback
    }

    func calendarTitle(for id: CalendarListMirror.ID, fallback: String = "Calendar") -> String {
        calendarTitleByID[id] ?? fallback
    }

    private func calendarTimeZoneID(for id: CalendarListMirror.ID) -> String? {
        calendars.first(where: { $0.id == id })?.timeZoneID
    }

    private func resolvedEventTimeZones(
        startTimeZoneID: String?,
        endTimeZoneID: String?,
        calendarID: CalendarListMirror.ID,
        existingEvent: CalendarEventMirror?
    ) -> (start: String, end: String) {
        let start = TimezoneSupport.validatedIdentifier(startTimeZoneID)
            ?? TimezoneSupport.validatedIdentifier(existingEvent?.startTimeZoneID)
            ?? TimezoneSupport.validatedIdentifier(calendarTimeZoneID(for: calendarID))
            ?? TimezoneSupport.currentIdentifier
        let end = TimezoneSupport.validatedIdentifier(endTimeZoneID)
            ?? TimezoneSupport.validatedIdentifier(existingEvent?.endTimeZoneID)
            ?? start
        return (start, end)
    }

    func openTaskCount(forTaskListID id: TaskListMirror.ID) -> Int {
        taskListCompletionStats[id]?.openCount ?? 0
    }

    func children(of taskID: TaskMirror.ID) -> [TaskMirror] {
        (taskChildrenByParentID[taskID] ?? []).compactMap { task(id: $0) }
    }

    private func publishSessionSurfaceStores() {
        let bannerSnapshot = StatusBannerSnapshot(
            authState: authState,
            syncState: syncState,
            mutationError: lastMutationError,
            cacheWarning: lastCacheLoadWarning,
            isSyncPaused: isSyncPaused,
            pendingMutationCount: pendingMutations.count,
            quarantinedCount: quarantinedMutationCount,
            invalidPayloadCount: invalidPayloadMutationCount,
            conflictCount: conflictedMutationCount,
            deferredReminderSummary: lastNotificationScheduleSummary,
            syncFailureKind: syncFailureKind,
            daysSinceLastLaunch: daysSinceLastLaunch,
            syncScope: SyncScopeSummary(tasks: tasks.count, events: events.count)
        )
        calendarStore.publishSession(
            account: account,
            authState: authState,
            syncState: syncState,
            isSyncPaused: isSyncPaused,
            lastMutationError: lastMutationError
        )
        taskStore.publishSession(
            account: account,
            authState: authState,
            syncState: syncState,
            isMutating: isMutating,
            lastMutationError: lastMutationError
        )
        syncStatusStore.publish(
            authState: authState,
            syncState: syncState,
            isSyncPaused: isSyncPaused,
            pendingMutationCount: pendingMutations.count,
            daysSinceLastLaunch: daysSinceLastLaunch,
            lastSuccessfulSyncAt: lastSuccessfulSyncAt,
            syncFailureKind: syncFailureKind,
            banner: bannerSnapshot
        )
        publishMenuBarProjectionSurfaceStore()
    }

    private func publishSnapshotSurfaceStores() {
        calendarStore.publishSnapshots(
            calendars: calendars,
            events: events,
            calendarSnapshot: calendarSnapshot,
            visibleTaskListIDs: visibleTaskListIDs,
            settings: settings,
            eventsByDay: eventsByDay,
            tasksByDueDate: tasksByDueDate,
            eventByIDSnapshot: eventByIDSnapshot,
            taskByIDSnapshot: taskByIDSnapshot,
            taskListTitleByID: taskListTitleByID,
            calendarDisplayRevision: calendarDisplayRevision,
            calendarDayDisplayRevisions: calendarDayDisplayRevisions,
            isRebuildingDerivedSnapshots: isRebuildingDerivedSnapshots
        )
        taskStore.publish(
            taskLists: taskLists,
            tasks: tasks,
            visibleTaskListIDs: visibleTaskListIDs,
            taskSections: taskSections,
            taskBoardSnapshot: taskBoardSnapshot,
            openTaskCountForSidebar: openTaskCountForSidebar,
            datedOpenTaskCount: datedOpenTaskCount,
            undatedOpenTaskCount: undatedOpenTaskCount,
            recentlyCompletedTaskID: recentlyCompletedTaskID,
            taskDisplayRevision: taskDisplayRevision,
            notesDisplayRevision: notesDisplayRevision,
            surfaceSettings: TaskSurfaceSettings(settings: settings),
            taskByIDSnapshot: taskByIDSnapshot,
            taskListTitleByID: taskListTitleByID,
            taskChildrenByParentID: taskChildrenByParentID,
            duplicateIndex: duplicateIndex,
            isRebuildingDerivedSnapshots: isRebuildingDerivedSnapshots
        )
        settingsStore.publish(settings: settings)
        commandPaletteStore.publish(
            pendingPaletteQuery: pendingPaletteQuery,
            entityRevision: commandPaletteEntityRevision,
            taskLists: taskLists,
            calendars: calendars,
            customFilters: settings.customFilters,
            usesDatabaseEntitySearch: usesDatabaseEntitySearch,
            performanceMode: settings.performanceMode
        )
        publishSessionSurfaceStores()
    }

    private func publishMenuBarProjectionSurfaceStore() {
        menuBarStore.publish(projection: makeMenuBarProjection())
    }

    private func makeMenuBarProjection(
        now: Date = Date(),
        calendar: Calendar = .current
    ) -> MenuBarProjection {
        let selectedCalendarIDs = calendarSnapshot.selectedCalendarIDs.isEmpty
            ? Set(calendars.map(\.id))
            : calendarSnapshot.selectedCalendarIDs
        let visibleTaskListIDSet = visibleTaskListIDs
        let today = calendar.startOfDay(for: now)
        let dayRange = (-7...35).compactMap { calendar.date(byAdding: .day, value: $0, to: today) }
        let dayKeys = dayRange.map { calendar.startOfDay(for: $0).timeIntervalSinceReferenceDate }

        func projectedEvents(forDayKey key: TimeInterval) -> [CalendarEventMirror] {
            (eventsByDay[key] ?? [])
                .compactMap { eventByIDSnapshot[$0] }
                .filter { event in
                    selectedCalendarIDs.contains(event.calendarID)
                        && event.status != .cancelled
                }
                .sorted { lhs, rhs in
                    if lhs.isAllDay != rhs.isAllDay { return lhs.isAllDay && rhs.isAllDay == false }
                    if lhs.startDate != rhs.startDate { return lhs.startDate < rhs.startDate }
                    return lhs.summary.localizedCaseInsensitiveCompare(rhs.summary) == .orderedAscending
                }
        }

        func projectedTasks(forDayKey key: TimeInterval) -> [TaskMirror] {
            (tasksByDueDate[key] ?? [])
                .compactMap { taskByIDSnapshot[$0] }
                .filter { task in
                    visibleTaskListIDSet.contains(task.taskListID)
                        && task.dueDate != nil
                        && task.isDeleted == false
                        && task.isCompleted == false
                        && task.isHidden == false
                }
                .sorted { lhs, rhs in
                    let lhsDue = lhs.dueDate ?? .distantFuture
                    let rhsDue = rhs.dueDate ?? .distantFuture
                    if lhsDue == rhsDue {
                        return lhs.title.localizedCaseInsensitiveCompare(rhs.title) == .orderedAscending
                    }
                    return lhsDue < rhsDue
                }
        }

        var dayBuckets: [TimeInterval: MenuBarDayProjection] = [:]
        for key in dayKeys {
            let events = projectedEvents(forDayKey: key)
            let tasks = projectedTasks(forDayKey: key)
            if events.isEmpty == false || tasks.isEmpty == false {
                dayBuckets[key] = MenuBarDayProjection(events: events, tasks: tasks)
            }
        }

        let futureEventKeys = eventsByDay.keys
            .filter { key in
                let day = Date(timeIntervalSinceReferenceDate: key)
                let lowerBound = calendar.date(byAdding: .day, value: -1, to: today) ?? today
                return day >= lowerBound
            }
            .sorted()
        var adaptiveEvents: [CalendarEventMirror] = []
        var seenEventIDs: Set<CalendarEventMirror.ID> = []
        for key in futureEventKeys {
            for event in projectedEvents(forDayKey: key)
                where event.endDate > now && seenEventIDs.insert(event.id).inserted {
                adaptiveEvents.append(event)
                if adaptiveEvents.count >= 64 { break }
            }
            if adaptiveEvents.count >= 64 { break }
        }

        var adaptiveTasks: [TaskMirror] = []
        var seenTaskIDs: Set<TaskMirror.ID> = []
        for key in tasksByDueDate.keys.sorted() {
            for task in projectedTasks(forDayKey: key)
                where seenTaskIDs.insert(task.id).inserted {
                adaptiveTasks.append(task)
                if adaptiveTasks.count >= 64 { break }
            }
            if adaptiveTasks.count >= 64 { break }
        }

        let pinnedFilters = settings.customFilters
            .filter(\.pinnedToMenuBar)
            .prefix(4)
            .map { filter in
                MenuBarPinnedFilterProjection(
                    definition: filter,
                    matchingTasks: filter.filter(
                        tasks,
                        now: now,
                        calendar: calendar,
                        taskLists: taskLists
                    )
                )
            }

        return MenuBarProjection(
            settings: MenuBarSettingsSnapshot(settings: settings),
            syncState: syncState,
            account: account,
            connectedAccounts: connectedAccounts,
            activeAccountID: activeAccountID,
            overdueCount: todaySnapshot.overdueCount,
            displayRevision: menuBarDisplayRevision,
            adaptiveEvents: adaptiveEvents.sorted { lhs, rhs in
                if lhs.startDate == rhs.startDate { return lhs.id < rhs.id }
                return lhs.startDate < rhs.startDate
            },
            adaptiveTasks: adaptiveTasks.sorted { lhs, rhs in
                let leftDue = lhs.dueDate ?? .distantFuture
                let rightDue = rhs.dueDate ?? .distantFuture
                if leftDue == rightDue { return lhs.id < rhs.id }
                return leftDue < rightDue
            },
            dayBuckets: dayBuckets,
            pinnedFilters: Array(pinnedFilters),
            calendarTitleByID: calendarTitleByID,
            taskListTitleByID: taskListTitleByID
        )
    }

    private func setDerivedSnapshotRebuildInFlight(_ value: Bool) {
        isRebuildingDerivedSnapshots = value
        calendarStore.setRebuildingDerivedSnapshots(value)
        taskStore.setRebuildingDerivedSnapshots(value)
    }

    private func apply(_ state: CachedAppState) {
        apply(state, changeSet: nil, rebuildStrategy: .synchronous)
    }

    private func apply(_ state: CachedAppState, rebuildStrategy: DerivedSnapshotRebuildStrategy) {
        apply(state, changeSet: nil, rebuildStrategy: rebuildStrategy)
    }

    private func apply(_ result: SyncApplyResult) {
        apply(result.state, changeSet: result.changeSet, rebuildStrategy: .deferred)
    }

    private func apply(
        _ state: CachedAppState,
        changeSet: SyncChangeSet?,
        rebuildStrategy: DerivedSnapshotRebuildStrategy
    ) {
        #if DEBUG
        let totalStart = DispatchTime.now().uptimeNanoseconds
        #endif
        // Sync-diff detection: count net changes vs current in-memory state BEFORE
        // we overwrite. Skipped on cold launch (empty local) and when nothing
        // differed (steady-state poll). Only the net diff is recorded so the
        // history log isn't spammed every 30s refresh tick.
        #if DEBUG
        let diffStart = DispatchTime.now().uptimeNanoseconds
        #endif
        let priorTasks = tasks
        let priorEvents = events
        let shouldEmitSyncDiff = priorTasks.isEmpty == false || priorEvents.isEmpty == false
        #if DEBUG
        let diffEnd = DispatchTime.now().uptimeNanoseconds
        let assignmentStart = DispatchTime.now().uptimeNanoseconds
        #endif
        let shouldAssignAll = changeSet == nil
        let shouldAssignTaskLists = shouldAssignAll || changeSet?.taskLists.hasChanges == true
        let shouldAssignTasks = shouldAssignAll || changeSet?.tasks.hasChanges == true
        let shouldAssignCalendars = shouldAssignAll || changeSet?.calendars.hasChanges == true
        let shouldAssignEvents = shouldAssignAll || changeSet?.events.hasChanges == true
        let shouldAssignSettings = shouldAssignAll || changeSet?.settingsChanged == true
        let shouldAssignCheckpoints = shouldAssignAll || changeSet?.checkpointChanged == true
        let shouldAssignPendingMutations = shouldAssignAll || state.pendingMutations != pendingMutations
        let shouldApplyEventOnlyDerivedUpdate = shouldAssignAll == false
            && changeSet?.canApplyEventOnlyDerivedUpdate == true
        let shouldRebuildDerivedSnapshots = shouldAssignAll
            || (changeSet?.requiresDerivedSnapshotRebuild == true && shouldApplyEventOnlyDerivedUpdate == false)
        #if DEBUG
        lastApplyUsedEventOnlyDerivedUpdateForBenchmark = false
        lastEventOnlyApplyProfileForBenchmark = nil
        #endif

        account = state.account
        connectedAccounts = state.accounts
        activeAccountID = state.activeAccountID
        if shouldAssignAll || state.accountWorkspaces != accountWorkspaces {
            accountWorkspaces = state.accountWorkspaces
        }
        authService.setActiveAccountID(state.activeAccountID)
        if shouldAssignTaskLists { taskLists = state.taskLists }
        if shouldAssignTasks { tasks = state.tasks }
        if shouldAssignCalendars { calendars = state.calendars }
        if shouldAssignEvents { events = state.events }
        if shouldAssignSettings { settings = state.settings }
        GoogleDiagnostics.setRawPayloadLoggingEnabled(settings.rawGoogleDiagnosticsEnabled)
        if shouldAssignCheckpoints { syncCheckpoints = state.syncCheckpoints }
        if shouldAssignPendingMutations { pendingMutations = state.pendingMutations }
        HCBColorSchemeStore.current = HCBColorScheme.scheme(id: settings.colorSchemeID, customSchemes: settings.customColorSchemes) ?? .notion
        HCBShortcutStorage.persist(settings.shortcutOverrides)
        #if DEBUG
        let assignmentEnd = DispatchTime.now().uptimeNanoseconds
        let autoIncludeStart = DispatchTime.now().uptimeNanoseconds
        #endif
        if shouldAssignAll || shouldAssignTaskLists {
            autoIncludeNewTaskLists()
        }
        #if DEBUG
        let autoIncludeEnd = DispatchTime.now().uptimeNanoseconds
        let rebuildStart = DispatchTime.now().uptimeNanoseconds
        #endif
        if shouldRebuildDerivedSnapshots {
            switch rebuildStrategy {
            case .synchronous:
                rebuildSnapshots(syncChangeSet: changeSet)
            case .deferred:
                rebuildSnapshotsDeferred(referenceDate: Date(), syncChangeSet: changeSet)
            }
        } else if shouldApplyEventOnlyDerivedUpdate, let changeSet {
            #if DEBUG
            lastApplyUsedEventOnlyDerivedUpdateForBenchmark = true
            #endif
            applyEventOnlyDerivedUpdate(
                priorEvents: priorEvents,
                nextEvents: events,
                changeSet: changeSet,
                referenceDate: Date()
            )
        } else {
            setDerivedSnapshotRebuildInFlight(false)
        }
        #if DEBUG
        let rebuildEnd = DispatchTime.now().uptimeNanoseconds
        #endif
        // apply user's configured retention cap to the audit log actor on boot / after any apply (settings could have been sync-pulled remotely too).
        let cap = settings.historyStorageCap
        Task { await MutationAuditLog.shared.setRetentionLimit(cap) }
        #if DEBUG
        let postApplyStart = DispatchTime.now().uptimeNanoseconds
        #endif

        let shouldRecordSyncDiff = changeSet.map {
            $0.tasks.hasChanges || $0.events.hasChanges
        } ?? true
        if shouldEmitSyncDiff && shouldRecordSyncDiff {
            Self.recordSyncDiffAsync(
                priorTasks: priorTasks,
                priorEvents: priorEvents,
                nextTasks: state.tasks,
                nextEvents: state.events
            )
        }
        publishSnapshotSurfaceStores()
        #if DEBUG
        let totalEnd = DispatchTime.now().uptimeNanoseconds
        lastApplyProfileForBenchmark = AppModelApplyProfile(
            diffSetupMilliseconds: AppModelApplyProfile.milliseconds(from: diffStart, to: diffEnd),
            assignmentMilliseconds: AppModelApplyProfile.milliseconds(from: assignmentStart, to: assignmentEnd),
            autoIncludeMilliseconds: AppModelApplyProfile.milliseconds(from: autoIncludeStart, to: autoIncludeEnd),
            rebuildSnapshotsMilliseconds: AppModelApplyProfile.milliseconds(from: rebuildStart, to: rebuildEnd),
            postApplyMilliseconds: AppModelApplyProfile.milliseconds(from: postApplyStart, to: totalEnd),
            totalMilliseconds: AppModelApplyProfile.milliseconds(from: totalStart, to: totalEnd)
        )
        #endif
    }

    // Counts sync diffs away from the main apply path. These history records
    // are informational only, so they should not delay freshly synced data
    // becoming visible.
    nonisolated private static func recordSyncDiffAsync(
        priorTasks: [TaskMirror],
        priorEvents: [CalendarEventMirror],
        nextTasks: [TaskMirror],
        nextEvents: [CalendarEventMirror]
    ) {
        Task.detached(priority: .utility) {
            let priorTaskMap = Dictionary(uniqueKeysWithValues: priorTasks.map { ($0.id, $0) })
            let priorEventMap = Dictionary(uniqueKeysWithValues: priorEvents.map { ($0.id, $0) })

            let taskDiff = syncDiffCount(prior: priorTaskMap, next: nextTasks)
            let eventDiff = syncDiffCount(prior: priorEventMap, next: nextEvents)

            if taskDiff > 0 {
                await MutationAuditLog.shared.record(
                    kind: "sync.task",
                    resourceID: "",
                    summary: "sync",
                    metadata: ["count": String(taskDiff)],
                    priorSnapshotJSON: nil,
                    postSnapshotJSON: nil
                )
            }
            if eventDiff > 0 {
                await MutationAuditLog.shared.record(
                    kind: "sync.event",
                    resourceID: "",
                    summary: "sync",
                    metadata: ["count": String(eventDiff)],
                    priorSnapshotJSON: nil,
                    postSnapshotJSON: nil
                )
            }
        }
    }

    nonisolated private static func syncDiffCount<V: Identifiable & Equatable>(
        prior: [String: V],
        next: [V]
    ) -> Int where V.ID == String {
        var count = 0
        var seenIDs = Set<String>()
        seenIDs.reserveCapacity(next.count)

        for newValue in next {
            let id = newValue.id
            seenIDs.insert(id)
            if let oldValue = prior[id] {
                if oldValue != newValue { count += 1 }
            } else {
                count += 1
            }
        }
        for id in prior.keys where seenIDs.contains(id) == false { count += 1 }
        return count
    }

    // When sync brings a new TaskListMirror that the user hasn't seen yet,
    // append its id to any *configured* per-tab/global selection sets so
    // the new list is visible by default. Without this, a newly-created
    // Google Tasks list silently disappears from HCB the next time the
    // user opens it because the configured-selection check filters it out.
    // Unconfigured selections (hasConfigured == false) already show every
    // list; nothing to do for those.
    private func autoIncludeNewTaskLists() {
        let allIDs = Set(taskLists.map(\.id))
        var changed = false
        if settings.hasConfiguredTaskListSelection {
            let missing = allIDs.subtracting(settings.selectedTaskListIDs)
            if missing.isEmpty == false {
                settings.selectedTaskListIDs.formUnion(missing)
                changed = true
            }
        }
        if settings.hasConfiguredTasksTabSelection {
            let missing = allIDs.subtracting(settings.tasksTabSelectedListIDs)
            if missing.isEmpty == false {
                settings.tasksTabSelectedListIDs.formUnion(missing)
                changed = true
            }
        }
        if settings.hasConfiguredNotesTabSelection {
            let missing = allIDs.subtracting(settings.notesTabSelectedListIDs)
            if missing.isEmpty == false {
                settings.notesTabSelectedListIDs.formUnion(missing)
                changed = true
            }
        }
        if changed {
            AppLogger.info("auto-included new task lists in selection", category: .sync)
        }
    }

    private func installPreviewData() {
        apply(.preview)
        authState = .signedIn(.preview)
        syncState = .synced(at: Date())
    }

    private func setActiveAccount(_ account: GoogleAccount) {
        upsertConnectedAccount(account)
        self.account = account
        activeAccountID = account.id
    }

    private func upsertConnectedAccount(_ account: GoogleAccount) {
        if let index = connectedAccounts.firstIndex(where: { $0.id == account.id }) {
            connectedAccounts[index] = account
        } else {
            connectedAccounts.append(account)
        }
    }

    private func removeConnectedAccount(id: GoogleAccount.ID) {
        connectedAccounts.removeAll { $0.id == id }
        if activeAccountID == id {
            activeAccountID = nil
        }
        if account?.id == id {
            account = nil
        }
    }

    private func clearAccounts(provider: GoogleAuthProvider) {
        var removedIDs = Set(connectedAccounts.filter { $0.authProvider == provider }.map(\.id))
        if let account, account.authProvider == provider {
            removedIDs.insert(account.id)
        }
        connectedAccounts.removeAll { $0.authProvider == provider }
        accountWorkspaces.removeAll { removedIDs.contains($0.accountID) }
        if let activeAccountID, removedIDs.contains(activeAccountID) {
            self.activeAccountID = nil
        }
        if let account, removedIDs.contains(account.id) {
            self.account = nil
        }
    }

    @discardableResult
    private func clearCustomDesktopOAuthAccounts() -> Bool {
        let priorState = currentCachedState()
        var removedIDs = Set(priorState.accounts
            .filter { $0.authProvider == .customDesktopOAuth }
            .map(\.id))
        if let account = priorState.account, account.authProvider == .customDesktopOAuth {
            removedIDs.insert(account.id)
        }
        guard removedIDs.isEmpty == false else { return false }

        let remainingAccounts = priorState.accounts.filter { removedIDs.contains($0.id) == false }
        let remainingWorkspaces = priorState.accountWorkspaces.filter { removedIDs.contains($0.accountID) == false }
        let activeWasRemoved = priorState.activeAccountID.map { removedIDs.contains($0) } ?? false
        let nextActiveAccountID = activeWasRemoved
            ? remainingAccounts.first?.id
            : priorState.activeAccountID
        let nextAccount = nextActiveAccountID.flatMap { id in remainingAccounts.first { $0.id == id } }
        let nextWorkspace = nextActiveAccountID.flatMap { id in remainingWorkspaces.first { $0.accountID == id } }
        let preserveActiveRoot = activeWasRemoved == false

        let nextState = CachedAppState(
            account: nextAccount,
            accounts: remainingAccounts,
            activeAccountID: nextAccount?.id,
            accountWorkspaces: remainingWorkspaces,
            taskLists: nextWorkspace?.taskLists ?? (preserveActiveRoot ? priorState.taskLists : []),
            tasks: nextWorkspace?.tasks ?? (preserveActiveRoot ? priorState.tasks : []),
            calendars: nextWorkspace?.calendars ?? (preserveActiveRoot ? priorState.calendars : []),
            events: nextWorkspace?.events ?? (preserveActiveRoot ? priorState.events : []),
            settings: nextWorkspace?.effectiveSettings(mergedWith: priorState.settings) ?? priorState.settings,
            syncCheckpoints: nextWorkspace?.syncCheckpoints ?? (preserveActiveRoot ? priorState.syncCheckpoints : []),
            pendingMutations: nextWorkspace?.pendingMutations ?? (preserveActiveRoot ? priorState.pendingMutations : []),
            schemaVersion: priorState.schemaVersion
        )
        apply(nextState)
        authState = nextState.account.map(AuthState.signedIn) ?? .signedOut
        syncState = .idle
        return true
    }

    private func applyLocalTaskMove(
        _ task: TaskMirror,
        toTaskListID: TaskListMirror.ID,
        parentID: TaskMirror.ID?,
        previousSiblingID: TaskMirror.ID?
    ) -> TaskMirror {
        var moved = task
        moved.taskListID = toTaskListID
        moved.parentID = parentID
        moved.updatedAt = Date()

        var siblings = TaskHierarchy.sortByPosition(tasks.filter {
            $0.id != task.id
                && $0.taskListID == toTaskListID
                && $0.parentID == parentID
                && $0.isDeleted == false
        })

        let insertionIndex: Int
        if let previousSiblingID,
           let previousIndex = siblings.firstIndex(where: { $0.id == previousSiblingID }) {
            insertionIndex = siblings.index(after: previousIndex)
        } else {
            insertionIndex = 0
        }

        siblings.insert(moved, at: min(insertionIndex, siblings.count))
        for (index, sibling) in siblings.enumerated() {
            var ranked = sibling
            ranked.position = String(format: "%06d", index + 1)
            ranked.updatedAt = ranked.id == moved.id ? moved.updatedAt : ranked.updatedAt
            upsert(ranked)
            if ranked.id == moved.id {
                moved = ranked
            }
        }

        return moved
    }

    private func upsert(_ task: TaskMirror) {
        if let index = taskArrayIndex(for: task.id) {
            tasks[index] = task
        } else {
            taskIndexByID[task.id] = tasks.count
            tasks.append(task)
        }
        scheduleRebuildSnapshots()
    }

    private func upsert(_ taskList: TaskListMirror) {
        if let index = taskListArrayIndex(for: taskList.id) {
            taskLists[index] = taskList
        } else {
            taskListIndexByID[taskList.id] = taskLists.count
            taskLists.append(taskList)
        }
        scheduleRebuildSnapshots()
    }

    private func removeTaskList(id: TaskListMirror.ID) {
        if let index = taskListArrayIndex(for: id) {
            taskLists.remove(at: index)
            rebuildTaskListIndex()
        } else {
            taskLists.removeAll { $0.id == id }
            rebuildTaskListIndex()
        }
        tasks.removeAll { $0.taskListID == id }
        rebuildTaskIndex()
        settings.selectedTaskListIDs.remove(id)
        syncCheckpoints.removeAll { checkpoint in
            checkpoint.resourceType == .taskList && checkpoint.resourceID == id
        }
        scheduleRebuildSnapshots()
    }

    private func removeTask(id: TaskMirror.ID) {
        if let index = taskArrayIndex(for: id) {
            tasks.remove(at: index)
            rebuildTaskIndex()
        } else {
            let originalCount = tasks.count
            tasks.removeAll { $0.id == id }
            if tasks.count != originalCount {
                rebuildTaskIndex()
            }
        }
        scheduleRebuildSnapshots()
    }

    private func upsert(_ event: CalendarEventMirror) {
        var resolved = event
        // When Google reports useDefault=true and no explicit overrides,
        // surface the calendar's defaultReminders so the local-notification
        // scheduler fires at the user's actual configured offset instead
        // of a hard-coded fallback. Only applies to timed events — all-day
        // Google events don't honour the default in Calendar either.
        if resolved.usedDefaultReminders,
           resolved.reminderMinutes.isEmpty,
           resolved.isAllDay == false,
           let cal = calendars.first(where: { $0.id == resolved.calendarID }),
           cal.defaultReminderMinutes.isEmpty == false {
            resolved.reminderMinutes = cal.defaultReminderMinutes
        }
        if let index = eventArrayIndex(for: resolved.id) {
            events[index] = resolved
        } else {
            eventIndexByID[resolved.id] = events.count
            events.append(resolved)
        }
        scheduleRebuildSnapshots()
    }

    private func removeEvent(id: CalendarEventMirror.ID) {
        if let index = eventArrayIndex(for: id) {
            events.remove(at: index)
            rebuildEventIndex()
        } else {
            let originalCount = events.count
            events.removeAll { $0.id == id }
            if events.count != originalCount {
                rebuildEventIndex()
            }
        }
        scheduleRebuildSnapshots()
    }

    private func taskArrayIndex(for id: TaskMirror.ID) -> Int? {
        if let index = taskIndexByID[id],
           tasks.indices.contains(index),
           tasks[index].id == id {
            return index
        }
        return tasks.firstIndex { $0.id == id }
    }

    private func eventArrayIndex(for id: CalendarEventMirror.ID) -> Int? {
        if let index = eventIndexByID[id],
           events.indices.contains(index),
           events[index].id == id {
            return index
        }
        guard let index = events.firstIndex(where: { $0.id == id }) else {
            return nil
        }
        eventIndexByID[id] = index
        return index
    }

    private func taskListArrayIndex(for id: TaskListMirror.ID) -> Int? {
        if let index = taskListIndexByID[id],
           taskLists.indices.contains(index),
           taskLists[index].id == id {
            return index
        }
        return taskLists.firstIndex { $0.id == id }
    }

    private func rebuildTaskIndex() {
        var indexByID: [TaskMirror.ID: Int] = [:]
        var taskByID: [TaskMirror.ID: TaskMirror] = [:]
        indexByID.reserveCapacity(tasks.count)
        taskByID.reserveCapacity(tasks.count)
        for (index, task) in tasks.enumerated() {
            indexByID[task.id] = index
            taskByID[task.id] = task
        }
        taskIndexByID = indexByID
        taskByIDSnapshot = taskByID
    }

    private func rebuildEventIndex() {
        var indexByID: [CalendarEventMirror.ID: Int] = [:]
        var eventByID: [CalendarEventMirror.ID: CalendarEventMirror] = [:]
        indexByID.reserveCapacity(events.count)
        eventByID.reserveCapacity(events.count)
        for (index, event) in events.enumerated() {
            indexByID[event.id] = index
            eventByID[event.id] = event
        }
        eventIndexByID = indexByID
        eventByIDSnapshot = eventByID
    }

    private func rebuildTaskListIndex() {
        taskListIndexByID = Dictionary(uniqueKeysWithValues: taskLists.enumerated().map { ($0.element.id, $0.offset) })
    }

    private func currentCachedState() -> CachedAppState {
        CachedAppState(
            account: account,
            accounts: connectedAccounts,
            activeAccountID: activeAccountID,
            accountWorkspaces: accountWorkspaces,
            taskLists: taskLists,
            tasks: tasks,
            calendars: calendars,
            events: events,
            settings: settings,
            syncCheckpoints: syncCheckpoints,
            pendingMutations: pendingMutations
        )
    }

    private static func customBackgroundDirectory() throws -> URL {
        guard let appSupportURL = FileManager.default.urls(
            for: .applicationSupportDirectory,
            in: .userDomainMask
        ).first else {
            throw CocoaError(.fileNoSuchFile)
        }
        let appDirectoryName = Bundle.main.bundleIdentifier ?? "HotCrossBuns"
        return appSupportURL
            .appending(path: appDirectoryName, directoryHint: .isDirectory)
            .appending(path: "Backgrounds", directoryHint: .isDirectory)
    }

    private static func writeDownsampledCustomBackground(from sourceURL: URL, to destinationURL: URL) throws {
        guard let source = CGImageSourceCreateWithURL(sourceURL as CFURL, nil) else {
            throw CocoaError(.fileReadCorruptFile)
        }
        let thumbnailOptions: [CFString: Any] = [
            kCGImageSourceCreateThumbnailFromImageAlways: true,
            kCGImageSourceCreateThumbnailWithTransform: true,
            kCGImageSourceThumbnailMaxPixelSize: 2_400
        ]
        guard let image = CGImageSourceCreateThumbnailAtIndex(source, 0, thumbnailOptions as CFDictionary) else {
            throw CocoaError(.fileReadCorruptFile)
        }
        guard let destination = CGImageDestinationCreateWithURL(
            destinationURL as CFURL,
            UTType.jpeg.identifier as CFString,
            1,
            nil
        ) else {
            throw CocoaError(.fileWriteUnknown)
        }
        let properties: [CFString: Any] = [
            kCGImageDestinationLossyCompressionQuality: 0.86
        ]
        CGImageDestinationAddImage(destination, image, properties as CFDictionary)
        guard CGImageDestinationFinalize(destination) else {
            throw CocoaError(.fileWriteUnknown)
        }
    }

    @discardableResult
    private func runDailyLocalBackupIfNeeded(force: Bool = false, now: Date = Date()) async -> Bool {
        guard force || settings.dailyLocalBackupEnabled else { return false }
        if force == false,
           let last = settings.lastDailyLocalBackupAt,
           Calendar.current.isDate(last, inSameDayAs: now) {
            return false
        }

        do {
            _ = try await localBackupService.writeBackup(
                state: currentCachedState(),
                now: now,
                retentionCount: settings.dailyLocalBackupRetentionCount,
                encryptionContext: try await localBackupEncryptionContext()
            )
            settings.lastDailyLocalBackupAt = now
            localBackupSummary = await localBackupService.summary()
            scheduleCacheSave()
            return true
        } catch {
            AppLogger.warn("daily local backup failed", category: .cache, metadata: [
                "error": String(describing: error)
            ])
            localBackupSummary = await localBackupService.summary()
            return false
        }
    }

    private func localBackupEncryptionContext() async throws -> LocalBackupService.EncryptionContext? {
        guard settings.cacheEncryptionEnabled else { return nil }
        guard let key = HCBCacheKeychain.load(),
              let salt = await cacheStore.currentSalt() else {
            throw LocalBackupEncryptionError.missingKeyOrSalt
        }
        return LocalBackupService.EncryptionContext(key: key, salt: salt)
    }

    // Synchronous flush. Use only from explicit-flush sites (logout,
    // scenePhase background, tests). Routine mutations should call
    // scheduleCacheSave() so dozens of rapid writes during a sync flush
    // coalesce into a single disk hit.
    private func saveCurrentState() async {
        pendingSaveTask?.cancel()
        pendingSaveTask = nil
        pendingCacheSaveChangeSet = nil
        pendingCacheSaveRequiresDerivedDiff = false
        await cacheStore.save(currentCachedState())
    }

    // Debounced save. Multiple calls within a 500ms window collapse into
    // one cacheStore.save invocation. Snapshots the current state at
    // schedule time so the actual write reflects the last call's state
    // even if more mutations land before the timer fires. The 500ms
    // window is long enough to absorb a sync flush burst (dozens of
    // upserts in <100ms) yet short enough that user-perceived latency
    // for "did my edit persist?" stays imperceptible.
    private var pendingSaveTask: Task<Void, Never>?
    private var pendingCacheSaveChangeSet: CachePersistenceChangeSet?
    private var pendingCacheSaveRequiresDerivedDiff = false

    func scheduleCacheSave() {
        pendingCacheSaveChangeSet = nil
        pendingCacheSaveRequiresDerivedDiff = true
        schedulePendingCacheSaveTask()
    }

    func scheduleCacheSave(changeSet: CachePersistenceChangeSet) {
        guard changeSet.hasRowChanges || changeSet.settingsChanged || changeSet.checkpointChanged else {
            return
        }
        if pendingCacheSaveRequiresDerivedDiff == false {
            if var pending = pendingCacheSaveChangeSet {
                pending.formUnion(changeSet)
                pendingCacheSaveChangeSet = pending
            } else {
                pendingCacheSaveChangeSet = changeSet
            }
        }
        schedulePendingCacheSaveTask()
    }

    private func schedulePendingCacheSaveTask() {
        pendingSaveTask?.cancel()
        let store = cacheStore
        pendingSaveTask = Task { @MainActor [weak self] in
            try? await Task.sleep(for: .milliseconds(500))
            if Task.isCancelled { return }
            guard let self else { return }
            // Re-snapshot at fire time, not at schedule time, so any mutations
            // between schedule and fire are also persisted.
            let save = self.pendingCacheSaveRequest()
            await Self.writeCacheSnapshot(save.snapshot, changeSet: save.changeSet, to: store)
        }
    }

    private func pendingCacheSaveRequest() -> (snapshot: CachedAppState, changeSet: CachePersistenceChangeSet?) {
        let snapshot = currentCachedState()
        let changeSet = pendingCacheSaveRequiresDerivedDiff ? nil : pendingCacheSaveChangeSet
        pendingSaveTask = nil
        pendingCacheSaveChangeSet = nil
        pendingCacheSaveRequiresDerivedDiff = false
        return (snapshot, changeSet)
    }

    private static func writeCacheSnapshot(
        _ snapshot: CachedAppState,
        changeSet: CachePersistenceChangeSet?,
        to store: LocalCacheStore
    ) async {
        guard let changeSet else {
            await store.save(snapshot)
            return
        }
        do {
            try await store.commit(state: snapshot, changeSet: changeSet)
        } catch {
            AppLogger.warn("explicit cache commit failed, falling back to derived diff save", category: .cache, metadata: [
                "error": String(describing: error)
            ])
            await store.save(snapshot)
        }
    }

    // Force-flush any pending debounced save. Bind to scenePhase
    // .background and to logout/disconnect flows so a backgrounded app
    // can't lose the last 500ms of mutations.
    func flushPendingCacheSave() async {
        guard let task = pendingSaveTask else { return }
        task.cancel()
        let save = pendingCacheSaveRequest()
        await Self.writeCacheSnapshot(save.snapshot, changeSet: save.changeSet, to: cacheStore)
    }

    private func isValidEventRange(startDate: Date, endDate: Date, isAllDay: Bool) -> Bool {
        if isAllDay {
            return Calendar.current.startOfDay(for: endDate) >= Calendar.current.startOfDay(for: startDate)
        }

        return endDate > startDate
    }

    func notificationScheduleSummary() async -> NotificationScheduleSummary? {
        await notificationScheduler.lastSummary
    }

    func rebuildLocalIntegrationIndexesForDiagnostics() async {
        try? await cacheStore.enqueueSideEffectRebuild(targets: Set(LocalIntegrationDirtyTarget.allCases))
        await runNotificationSync(requestAuthorization: false)
        await runSpotlightSync()
    }

    func recordNearRealtimePollDiagnostic(
        delaySeconds: Double,
        attempt: Int,
        isNetworkConstrained: Bool,
        isLowPowerModeEnabled: Bool,
        isMainWindowFocused: Bool
    ) {
        nearRealtimePollDiagnostics = NearRealtimePollDiagnostics(
            recordedAt: Date(),
            delaySeconds: delaySeconds,
            attempt: attempt,
            isNetworkConstrained: isNetworkConstrained,
            isLowPowerModeEnabled: isLowPowerModeEnabled,
            isMainWindowFocused: isMainWindowFocused
        )
    }

    func performanceDiagnosticsSnapshot() async -> PerformanceDiagnosticsSnapshot {
        PerformanceDiagnosticsSnapshot(
            nearRealtime: nearRealtimePollDiagnostics,
            integrations: LocalIntegrationDiagnostics(
                notificationUpdatedAt: notificationDiagnosticUpdatedAt,
                notificationDurationMilliseconds: notificationDiagnosticDurationMilliseconds,
                spotlight: await spotlightIndexer.summary()
            )
        )
    }

    // Notification + Spotlight debounce handles. Kept @ObservationIgnored
    // so cancel/re-assign doesn't invalidate observing views.
    @ObservationIgnored private var notificationDebounce: Task<Void, Never>?
    @ObservationIgnored private var spotlightDebounce: Task<Void, Never>?

    // Every task/event/list mutation path calls this. Previously it ran BOTH
    // the full notification scan AND the full Spotlight domain rebuild in
    // sequence, on the user-facing command path — a checkbox click could
    // trigger a Spotlight domain-delete + re-index of every task and event.
    //
    // Now: the actual work is debounced (notifications 500 ms, Spotlight
    // 2 s) and runs in independent Tasks so a sync flush of dozens of
    // upserts collapses to a single deferred scan per integration. The
    // user's click returns immediately; integrations catch up shortly
    // after with the latest state (Task body reads `self.tasks` / `self.events`
    // at fire time, not at schedule time).
    private func synchronizeLocalNotifications(requestAuthorization: Bool = false) async {
        notificationDebounce?.cancel()
        spotlightDebounce?.cancel()
        notificationDebounce = Task { [weak self] in
            try? await Task.sleep(for: .milliseconds(500))
            guard Task.isCancelled == false, let self else { return }
            await self.runNotificationSync(requestAuthorization: requestAuthorization)
        }
        spotlightDebounce = Task { [weak self] in
            try? await Task.sleep(for: .seconds(2))
            guard Task.isCancelled == false, let self else { return }
            await self.runSpotlightSync()
        }
    }

    private func runNotificationSync(requestAuthorization: Bool) async {
        // Snapshot on main actor at fire time — reflects the latest state
        // regardless of how many mutations piled up during the debounce.
        let started = Date()
        let settingsNow = settings
        let dirtyItems = (try? await cacheStore.sideEffectDirtyItems(target: .notification, limit: 500)) ?? []
        let lastNotificationSummary = await notificationScheduler.lastSummary
        let requiresFullRebuild = dirtyItems.contains(where: \.isFullRebuild)
            || requestAuthorization
            || settingsNow.enableLocalNotifications == false
            || lastNotificationSummary == nil

        if dirtyItems.isEmpty == false, requiresFullRebuild == false {
            let dirty = dirtyResourceIDs(from: dirtyItems)
            await notificationScheduler.synchronizeDirty(
                changedTasks: dirty.upsertedTaskIDs.compactMap { task(id: $0) },
                deletedTaskIDs: dirty.deletedTaskIDs,
                changedEvents: dirty.upsertedEventIDs.compactMap { event(id: $0) },
                deletedEventIDs: dirty.deletedEventIDs,
                settings: settingsNow,
                requestAuthorization: false
            )
            try? await cacheStore.markSideEffectDirtyItemsProcessed(dirtyItems)
        } else {
            let tasksNow = tasks
            let eventsNow = events
            await notificationScheduler.synchronize(
                tasks: tasksNow,
                events: eventsNow,
                settings: settingsNow,
                requestAuthorization: requestAuthorization
            )
            if dirtyItems.isEmpty == false {
                try? await cacheStore.markSideEffectDirtyItemsProcessed(dirtyItems)
            }
        }
        lastNotificationScheduleSummary = await notificationScheduler.lastSummary
        notificationDiagnosticUpdatedAt = Date()
        notificationDiagnosticDurationMilliseconds = Date().timeIntervalSince(started) * 1000
    }

    private func normalizedCompletionSoundChoice(
        _ choice: CompletionSoundChoice,
        fallback: CompletionSoundChoice
    ) -> CompletionSoundChoice {
        switch choice.source {
        case .system:
            return CompletionSoundLibrary.builtInSoundNames.contains(choice.identifier) ? choice : fallback
        case .custom:
            guard
                let assetID = choice.customAssetID,
                settings.customCompletionSounds.contains(where: { $0.id == assetID })
            else {
                return fallback
            }
            return choice
        }
    }

    private func runSpotlightSync() async {
        let dirtyItems = (try? await cacheStore.sideEffectDirtyItems(target: .spotlight, limit: 500)) ?? []
        let isSpotlightPrimed = await spotlightIndexer.isPrimed()
        let requiresFullRebuild = dirtyItems.contains(where: \.isFullRebuild) || isSpotlightPrimed == false
        if dirtyItems.isEmpty == false, requiresFullRebuild == false {
            let dirty = dirtyResourceIDs(from: dirtyItems)
            await spotlightIndexer.applyDirty(
                changedTasks: dirty.upsertedTaskIDs.compactMap { task(id: $0) },
                deletedTaskIDs: dirty.deletedTaskIDs,
                changedEvents: dirty.upsertedEventIDs.compactMap { event(id: $0) },
                deletedEventIDs: dirty.deletedEventIDs
            )
            try? await cacheStore.markSideEffectDirtyItemsProcessed(dirtyItems)
        } else {
            let tasksNow = tasks
            let eventsNow = events
            await spotlightIndexer.update(tasks: tasksNow, events: eventsNow)
            if dirtyItems.isEmpty == false {
                try? await cacheStore.markSideEffectDirtyItemsProcessed(dirtyItems)
            }
        }
    }

    private func dirtyResourceIDs(
        from items: [LocalIntegrationDirtyItem]
    ) -> (
        upsertedTaskIDs: Set<TaskMirror.ID>,
        deletedTaskIDs: Set<TaskMirror.ID>,
        upsertedEventIDs: Set<CalendarEventMirror.ID>,
        deletedEventIDs: Set<CalendarEventMirror.ID>
    ) {
        var upsertedTaskIDs: Set<TaskMirror.ID> = []
        var deletedTaskIDs: Set<TaskMirror.ID> = []
        var upsertedEventIDs: Set<CalendarEventMirror.ID> = []
        var deletedEventIDs: Set<CalendarEventMirror.ID> = []
        for item in items {
            guard let resourceType = item.resourceType, let resourceID = item.resourceID else { continue }
            switch (resourceType, item.operation) {
            case (.task, .upsert):
                upsertedTaskIDs.insert(resourceID)
            case (.task, .delete):
                deletedTaskIDs.insert(resourceID)
            case (.event, .upsert):
                upsertedEventIDs.insert(resourceID)
            case (.event, .delete):
                deletedEventIDs.insert(resourceID)
            default:
                break
            }
        }
        return (upsertedTaskIDs, deletedTaskIDs, upsertedEventIDs, deletedEventIDs)
    }

    func calendarDisplayRevisionKey(for dates: [Date], calendar: Calendar = .current) -> String {
        let dayKeys = Set(dates.map { calendar.startOfDay(for: $0).timeIntervalSinceReferenceDate })
        return calendarDisplayRevisionKey(forDayKeys: dayKeys)
    }

    func calendarDisplayRevisionKey(from start: Date, to end: Date, calendar: Calendar = .current) -> String {
        let normalizedStart = calendar.startOfDay(for: start)
        let normalizedEnd = calendar.startOfDay(for: end)
        guard normalizedStart <= normalizedEnd else {
            return calendarDisplayRevisionKey(for: [start], calendar: calendar)
        }
        var dayKeys: [TimeInterval] = []
        var cursor = normalizedStart
        while cursor <= normalizedEnd {
            dayKeys.append(cursor.timeIntervalSinceReferenceDate)
            guard let next = calendar.date(byAdding: .day, value: 1, to: cursor) else { break }
            cursor = next
        }
        return calendarDisplayRevisionKey(forSortedDayKeys: dayKeys)
    }

    func calendarDisplayRevisionKey(forYearContaining date: Date, calendar: Calendar = .current) -> String {
        guard let yearStart = calendar.date(from: DateComponents(year: calendar.component(.year, from: date), month: 1, day: 1)),
              let nextYear = calendar.date(from: DateComponents(year: calendar.component(.year, from: date) + 1, month: 1, day: 1))
        else {
            return calendarDisplayRevisionKey(for: [date], calendar: calendar)
        }
        let yearEnd = calendar.date(byAdding: .day, value: -1, to: nextYear) ?? yearStart
        return calendarDisplayRevisionKey(from: yearStart, to: yearEnd, calendar: calendar)
    }

    func calendarVisibleRangeProjection(
        kind: CalendarVisibleRangeKind,
        anchorDate: Date,
        dayCount: Int? = nil,
        calendar: Calendar = .current
    ) -> CalendarVisibleRangeProjection {
        let range = calendarVisibleRange(kind: kind, anchorDate: anchorDate, dayCount: dayCount, calendar: calendar)
        return calendarVisibleRangeProjection(range: range)
    }

    func calendarVisibleRangeProjection(
        kind: CalendarVisibleRangeKind,
        start: Date,
        end: Date,
        calendar: Calendar = .current
    ) -> CalendarVisibleRangeProjection {
        calendarVisibleRangeProjection(
            range: CalendarVisibleRange(kind: kind, start: start, end: end, calendar: calendar)
        )
    }

    private func calendarVisibleRangeProjection(range: CalendarVisibleRange) -> CalendarVisibleRangeProjection {
        let dayKeySet = Set(range.dayKeys)
        var visibleEventsByDay: [TimeInterval: [CalendarEventMirror.ID]] = [:]
        var visibleEventByID: [CalendarEventMirror.ID: CalendarEventMirror] = [:]
        var visibleTasksByDueDate: [TimeInterval: [TaskMirror.ID]] = [:]
        var visibleTaskByID: [TaskMirror.ID: TaskMirror] = [:]

        for key in range.dayKeys {
            let eventIDs = eventsByDay[key] ?? []
            if eventIDs.isEmpty == false {
                visibleEventsByDay[key] = eventIDs
            }
            for eventID in eventIDs {
                if let event = eventByIDSnapshot[eventID] {
                    visibleEventByID[eventID] = event
                }
            }

            let taskIDs = tasksByDueDate[key] ?? []
            if taskIDs.isEmpty == false {
                visibleTasksByDueDate[key] = taskIDs
            }
            for taskID in taskIDs {
                if let task = taskByIDSnapshot[taskID] {
                    visibleTaskByID[taskID] = task
                }
            }
        }

        return CalendarVisibleRangeProjection(
            range: range,
            revisionKey: calendarDisplayRevisionKey(forDayKeys: dayKeySet),
            eventsByDay: visibleEventsByDay,
            tasksByDueDate: visibleTasksByDueDate,
            eventByID: visibleEventByID,
            taskByID: visibleTaskByID,
            eventSearchTextByID: Dictionary(uniqueKeysWithValues: visibleEventByID.values.map { event in
                (event.id, "\(event.summary)\n\(event.details)\n\(event.location)")
            }),
            taskSearchTextByID: Dictionary(uniqueKeysWithValues: visibleTaskByID.values.map { task in
                (task.id, "\(task.title)\n\(task.notes)")
            })
        )
    }

    func calendarDisplayInput(
        key: PreparedSnapshotKey,
        kind: CalendarVisibleRangeKind,
        anchorDate: Date,
        dayCount: Int? = nil,
        eventViewFilter: CalendarEventViewFilter,
        searchQuery: String,
        referenceDate: Date = Date(),
        calendar: Calendar = .current
    ) -> CalendarDisplayInput {
        CalendarDisplayInput(
            key: key,
            anchorDate: anchorDate,
            selectedCalendarIDs: calendarSnapshot.selectedCalendarIDs,
            eventViewFilter: eventViewFilter,
            visibleTaskListIDs: visibleTaskListIDs,
            searchQuery: searchQuery,
            visibleRange: calendarVisibleRangeProjection(
                kind: kind,
                anchorDate: anchorDate,
                dayCount: dayCount,
                calendar: calendar
            ),
            calendarColorHexByID: calendarSnapshot.calendarColorHexByID,
            taskListTitleByID: taskListTitleByID,
            settings: settings,
            referenceDate: referenceDate,
            calendar: calendar
        )
    }

    func calendarDisplayInputFromQuery(
        key: PreparedSnapshotKey,
        kind: CalendarVisibleRangeKind,
        anchorDate: Date,
        dayCount: Int? = nil,
        eventViewFilter: CalendarEventViewFilter,
        searchQuery: String,
        referenceDate: Date = Date(),
        calendar: Calendar = .current
    ) async -> CalendarDisplayInput {
        let visibleRange = await calendarVisibleRangeProjectionFromQuery(
            kind: kind,
            anchorDate: anchorDate,
            dayCount: dayCount,
            eventViewFilter: eventViewFilter,
            searchQuery: searchQuery,
            calendar: calendar
        )
        return CalendarDisplayInput(
            key: key,
            anchorDate: anchorDate,
            selectedCalendarIDs: calendarSnapshot.selectedCalendarIDs,
            eventViewFilter: eventViewFilter,
            visibleTaskListIDs: visibleTaskListIDs,
            searchQuery: searchQuery,
            visibleRange: visibleRange,
            calendarColorHexByID: calendarSnapshot.calendarColorHexByID,
            taskListTitleByID: taskListTitleByID,
            settings: settings,
            referenceDate: referenceDate,
            calendar: calendar
        )
    }

    func calendarVisibleRangeProjectionFromQuery(
        kind: CalendarVisibleRangeKind,
        anchorDate: Date,
        dayCount: Int? = nil,
        eventViewFilter: CalendarEventViewFilter,
        searchQuery: String,
        calendar: Calendar = .current
    ) async -> CalendarVisibleRangeProjection {
        let range = calendarVisibleRange(kind: kind, anchorDate: anchorDate, dayCount: dayCount, calendar: calendar)
        return await calendarVisibleRangeProjectionFromQuery(
            range: range,
            eventViewFilter: eventViewFilter,
            searchQuery: searchQuery,
            calendar: calendar
        )
    }

    func calendarVisibleRangeProjectionFromQuery(
        kind: CalendarVisibleRangeKind,
        start: Date,
        end: Date,
        eventViewFilter: CalendarEventViewFilter,
        searchQuery: String,
        calendar: Calendar = .current
    ) async -> CalendarVisibleRangeProjection {
        await calendarVisibleRangeProjectionFromQuery(
            range: CalendarVisibleRange(kind: kind, start: start, end: end, calendar: calendar),
            eventViewFilter: eventViewFilter,
            searchQuery: searchQuery,
            calendar: calendar
        )
    }

    private func calendarVisibleRangeProjectionFromQuery(
        range: CalendarVisibleRange,
        eventViewFilter: CalendarEventViewFilter,
        searchQuery: String,
        calendar: Calendar
    ) async -> CalendarVisibleRangeProjection {
        let fallback = calendarVisibleRangeProjection(range: range)
        do {
            var projection = try await calendarQueryService.visibleRangeProjection(
                accountID: activeAccountID,
                range: range,
                selectedCalendarIDs: calendarSnapshot.selectedCalendarIDs,
                eventViewFilter: eventViewFilter,
                includeCancelled: settings.showCompletedItemsInCalendar,
                requiresEventDetails: range.kind != .year
                    || searchQuery.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false
                    || settings.pastEventBehavior == .hide
                    || eventViewFilter.visibleTagNames != nil,
                calendar: calendar
            )
            if projection.revisionKey == "0"
                && projection.eventByID.isEmpty
                && projection.taskByID.isEmpty
                && (fallback.eventByID.isEmpty == false || fallback.taskByID.isEmpty == false) {
                return fallback
            }
            enrichCalendarProjection(&projection, includeSearchText: searchQuery.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false)
            return projection
        } catch {
            HCBPerformanceTelemetry.debug(
                "calendar db projection fallback",
                metadata: ["error": String(describing: error)]
            )
            return fallback
        }
    }

    private func enrichCalendarProjection(
        _ projection: inout CalendarVisibleRangeProjection,
        includeSearchText: Bool
    ) {
        for eventID in projection.eventByID.keys {
            guard let fullEvent = eventByIDSnapshot[eventID] else { continue }
            projection.eventByID[eventID] = fullEvent
            if includeSearchText {
                projection.eventSearchTextByID[eventID] = "\(fullEvent.summary)\n\(fullEvent.details)\n\(fullEvent.location)"
            }
        }
        for taskID in projection.taskByID.keys {
            guard let fullTask = taskByIDSnapshot[taskID] else { continue }
            projection.taskByID[taskID] = fullTask
            if includeSearchText {
                projection.taskSearchTextByID[taskID] = "\(fullTask.title)\n\(fullTask.notes)"
            }
        }
    }

    private func calendarVisibleRange(
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
            guard let first = days.first, let last = days.last else {
                return CalendarVisibleRange(kind: kind, start: anchorStart, end: anchorStart, calendar: calendar)
            }
            return CalendarVisibleRange(kind: kind, start: first, end: last, calendar: calendar)
        case .month:
            let cells = CalendarGridLayout.monthCells(for: anchorDate, calendar: calendar)
            guard let first = cells.first, let last = cells.last else {
                return CalendarVisibleRange(kind: kind, start: anchorStart, end: anchorStart, calendar: calendar)
            }
            return CalendarVisibleRange(kind: kind, start: first, end: last, calendar: calendar)
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

    private func calendarDisplayRevisionKey(forDayKeys dayKeys: Set<TimeInterval>) -> String {
        calendarDisplayRevisionKey(forSortedDayKeys: dayKeys.sorted())
    }

    private func calendarDisplayRevisionKey(forSortedDayKeys sortedDayKeys: [TimeInterval]) -> String {
        guard sortedDayKeys.isEmpty == false else {
            return "\(calendarDisplayRevision)"
        }
        let cacheKey = "\(calendarDisplayRevision)|\(sortedDayKeys.map { String(Int($0)) }.joined(separator: ","))"
        if let cached = calendarRevisionKeyCache[cacheKey] {
            return cached
        }
        let dayRevisionPart = sortedDayKeys
            .map { key in "\(Int(key)):\(calendarDayDisplayRevisions[key] ?? 0)" }
            .joined(separator: ",")
        let revisionKey = "\(calendarDisplayRevision)[\(dayRevisionPart)]"
        calendarRevisionKeyCache[cacheKey] = revisionKey
        return revisionKey
    }

    func cachedCalendarDaySnapshot(for key: PreparedSnapshotKey) -> CalendarDayDisplaySnapshot? {
        let snapshot = calendarPreparedSnapshotCache.daySnapshot(for: key)
        HCBPerformanceTelemetry.debug(
            "calendar day snapshot cache \(snapshot == nil ? "miss" : "hit")",
            metadata: ["key": key.rawValue]
        )
        return snapshot
    }

    func storeCalendarDaySnapshot(_ snapshot: CalendarDayDisplaySnapshot) {
        calendarPreparedSnapshotCache.store(snapshot)
    }

    func cachedCalendarWeekSnapshot(for key: PreparedSnapshotKey) -> CalendarWeekDisplaySnapshot? {
        let snapshot = calendarPreparedSnapshotCache.weekSnapshot(for: key)
        HCBPerformanceTelemetry.debug(
            "calendar week snapshot cache \(snapshot == nil ? "miss" : "hit")",
            metadata: ["key": key.rawValue]
        )
        return snapshot
    }

    func storeCalendarWeekSnapshot(_ snapshot: CalendarWeekDisplaySnapshot) {
        calendarPreparedSnapshotCache.store(snapshot)
    }

    func cachedCalendarAgendaSnapshot(for key: PreparedSnapshotKey) -> CalendarAgendaDisplaySnapshot? {
        let snapshot = calendarPreparedSnapshotCache.agendaSnapshot(for: key)
        HCBPerformanceTelemetry.debug(
            "calendar agenda snapshot cache \(snapshot == nil ? "miss" : "hit")",
            metadata: ["key": key.rawValue]
        )
        return snapshot
    }

    func storeCalendarAgendaSnapshot(_ snapshot: CalendarAgendaDisplaySnapshot) {
        calendarPreparedSnapshotCache.store(snapshot)
    }

    func cachedCalendarYearSnapshot(for key: PreparedSnapshotKey) -> CalendarYearDisplaySnapshot? {
        let snapshot = calendarPreparedSnapshotCache.yearSnapshot(for: key)
        HCBPerformanceTelemetry.debug(
            "calendar year snapshot cache \(snapshot == nil ? "miss" : "hit")",
            metadata: ["key": key.rawValue]
        )
        return snapshot
    }

    func storeCalendarYearSnapshot(_ snapshot: CalendarYearDisplaySnapshot) {
        calendarPreparedSnapshotCache.store(snapshot)
    }

    // Schedules a coalesced rebuild on the next runloop tick. Multiple
    // calls within the same tick collapse to one. Used by mutation paths
    // (createTask, updateEvent, etc.) so a sync flush of dozens of upserts
    // produces a single snapshot rebuild instead of one per upsert.
    private var snapshotRebuildPending = false
    private var snapshotRebuildTask: Task<Void, Never>?
    private var snapshotRebuildGeneration: UInt64 = 0

    private func scheduleRebuildSnapshots() {
        if snapshotRebuildPending { return }
        snapshotRebuildPending = true
        snapshotRebuildGeneration &+= 1
        let generation = snapshotRebuildGeneration
        setDerivedSnapshotRebuildInFlight(true)
        Task { @MainActor [weak self] in
            guard let self else { return }
            guard generation == self.snapshotRebuildGeneration else { return }
            self.snapshotRebuildPending = false
            self.startDerivedSnapshotRebuild(referenceDate: Date(), syncChangeSet: nil)
        }
    }

    private func rebuildSnapshots(referenceDate: Date = Date(), syncChangeSet: SyncChangeSet? = nil) {
        // Synchronous-flush variant. Prefer scheduleRebuildSnapshots() for
        // mutation hot paths so multiple rapid mutations coalesce. Use
        // this only when a downstream caller needs snapshots populated
        // before its next line (apply() after sync, init bootstrap).
        snapshotRebuildPending = false
        snapshotRebuildGeneration &+= 1
        snapshotRebuildTask?.cancel()
        snapshotRebuildTask = nil
        setDerivedSnapshotRebuildInFlight(true)
        let started = ContinuousClock.now
        let snapshots = Self.buildDerivedSnapshots(derivedSnapshotInput(referenceDate: referenceDate))
        applyDerivedSnapshots(snapshots, syncChangeSet: syncChangeSet)
        setDerivedSnapshotRebuildInFlight(false)
        logDerivedSnapshotRebuild(started: started, snapshots: snapshots)
    }

    private func rebuildSnapshotsDeferred(referenceDate: Date = Date(), syncChangeSet: SyncChangeSet? = nil) {
        snapshotRebuildPending = false
        snapshotRebuildGeneration &+= 1
        setDerivedSnapshotRebuildInFlight(true)
        startDerivedSnapshotRebuild(referenceDate: referenceDate, syncChangeSet: syncChangeSet)
    }

    private func startDerivedSnapshotRebuild(referenceDate: Date, syncChangeSet: SyncChangeSet?) {
        let input = derivedSnapshotInput(referenceDate: referenceDate)
        snapshotRebuildTask?.cancel()
        snapshotRebuildTask = Task { @MainActor [weak self] in
            let started = ContinuousClock.now
            let snapshots = await Task.detached(priority: .utility) {
                Self.buildDerivedSnapshots(input)
            }.value
            guard let self, Task.isCancelled == false else { return }
            self.applyDerivedSnapshots(snapshots, syncChangeSet: syncChangeSet)
            self.setDerivedSnapshotRebuildInFlight(false)
            self.snapshotRebuildTask = nil
            self.logDerivedSnapshotRebuild(started: started, snapshots: snapshots)
        }
    }

    private func derivedSnapshotInput(referenceDate: Date) -> DerivedAppSnapshotInput {
        DerivedAppSnapshotInput(
            taskLists: taskLists,
            tasks: tasks,
            calendars: calendars,
            events: events,
            settings: settings,
            referenceDate: referenceDate
        )
    }

    private func applyDerivedSnapshots(_ snapshots: DerivedAppSnapshots, syncChangeSet: SyncChangeSet?) {
        visibleTaskListIDs = snapshots.visibleTaskListIDs
        taskSections = snapshots.taskSections
        taskBoardSnapshot = snapshots.taskBoardSnapshot
        todaySnapshot = snapshots.todaySnapshot
        calendarSnapshot = snapshots.calendarSnapshot
        eventsByCalendar = snapshots.eventsByCalendar
        eventsByDay = snapshots.eventsByDay
        tasksByDueDate = snapshots.tasksByDueDate
        datedOpenTaskCount = snapshots.datedOpenTaskCount
        undatedOpenTaskCount = snapshots.undatedOpenTaskCount
        openTaskCountForSidebar = snapshots.openTaskCountForSidebar
        taskListCompletionStats = snapshots.taskListCompletionStats
        taskListTitleByID = snapshots.taskListTitleByID
        calendarTitleByID = snapshots.calendarTitleByID
        taskChildrenByParentID = snapshots.taskChildrenByParentID
        duplicateIndex = snapshots.duplicateIndex
        taskByIDSnapshot = snapshots.taskByIDSnapshot
        eventByIDSnapshot = snapshots.eventByIDSnapshot
        taskIndexByID = snapshots.taskIndexByID
        eventIndexByID = snapshots.eventIndexByID
        taskListIndexByID = snapshots.taskListIndexByID
        #if DEBUG
        lastDerivedSnapshotBuildProfileForBenchmark = snapshots.buildProfile
        #endif
        if calendarDisplayFingerprint != snapshots.calendarDisplayFingerprint {
            calendarDisplayFingerprint = snapshots.calendarDisplayFingerprint
            if let syncChangeSet,
               syncChangeSet.canUseNarrowCalendarInvalidation {
                for dayKey in syncChangeSet.affectedDayKeys {
                    calendarDayDisplayRevisions[dayKey, default: 0] &+= 1
                }
                calendarRevisionKeyCache.removeAll()
                HCBPerformanceTelemetry.debug(
                    "calendar day revisions advanced",
                    metadata: [
                        "days": "\(syncChangeSet.affectedDayKeys.count)",
                        "globalRevision": "\(calendarDisplayRevision)"
                    ]
                )
            } else {
                calendarDisplayRevision &+= 1
                calendarDayDisplayRevisions.removeAll()
                calendarRevisionKeyCache.removeAll()
                calendarPreparedSnapshotCache.removeAll()
                calendarStore.removePreparedSnapshots()
                HCBPerformanceTelemetry.debug(
                    "calendar display revision advanced",
                    metadata: ["revision": "\(calendarDisplayRevision)"]
                )
            }
        }

        // Advance the content revision only after every derived snapshot and
        // lookup map has been applied together. Prepared views key off this
        // revision, so they never rebuild against half-refreshed indexes.
        dataRevision &+= 1
        commandPaletteEntityRevision &+= 1
        menuBarDisplayRevision &+= 1
        if shouldAdvanceTaskSurfaceRevisions(syncChangeSet: syncChangeSet) {
            taskDisplayRevision &+= 1
            notesDisplayRevision &+= 1
        }
        publishSnapshotSurfaceStores()
    }

    private func shouldAdvanceTaskSurfaceRevisions(syncChangeSet: SyncChangeSet?) -> Bool {
        guard let syncChangeSet else {
            return true
        }
        return syncChangeSet.taskLists.hasChanges
            || syncChangeSet.tasks.hasChanges
            || syncChangeSet.settingsChanged
    }

    private func applyEventOnlyDerivedUpdate(
        priorEvents: [CalendarEventMirror],
        nextEvents: [CalendarEventMirror],
        changeSet: SyncChangeSet,
        referenceDate: Date
    ) {
        #if DEBUG
        let totalStart = DispatchTime.now().uptimeNanoseconds
        let lookupStart = totalStart
        #endif
        let changedIDs = changeSet.events.inserted
            .union(changeSet.events.updated)
            .union(changeSet.events.deleted)
        guard changedIDs.isEmpty == false else {
            setDerivedSnapshotRebuildInFlight(false)
            return
        }

        let priorChanged = Dictionary(uniqueKeysWithValues: changedIDs.compactMap { id in
            eventByIDSnapshot[id].map { (id, $0) }
        })
        let upsertedIDs = changeSet.events.inserted.union(changeSet.events.updated)
        var nextChanged: [CalendarEventMirror.ID: CalendarEventMirror] = [:]
        nextChanged.reserveCapacity(upsertedIDs.count)
        #if DEBUG
        let lookupEnd = DispatchTime.now().uptimeNanoseconds
        let indexStart = lookupEnd
        #endif
        if upsertedIDs.isEmpty == false {
            var remaining = upsertedIDs
            for (index, event) in nextEvents.enumerated() where remaining.contains(event.id) {
                nextChanged[event.id] = event
                eventIndexByID[event.id] = index
                remaining.remove(event.id)
                if remaining.isEmpty { break }
            }
        }
        if changeSet.events.deleted.isEmpty == false {
            for id in changeSet.events.deleted {
                eventIndexByID.removeValue(forKey: id)
            }
        }
        #if DEBUG
        let indexEnd = DispatchTime.now().uptimeNanoseconds
        let snapshotMapStart = indexEnd
        #endif
        for id in changeSet.events.deleted {
            eventByIDSnapshot.removeValue(forKey: id)
        }
        for event in nextChanged.values {
            eventByIDSnapshot[event.id] = event
        }

        #if DEBUG
        let snapshotMapEnd = DispatchTime.now().uptimeNanoseconds
        let bucketStart = snapshotMapEnd
        #endif
        var affectedCalendarIDs = changeSet.affectedCalendarIDs
        var affectedDayKeys = changeSet.affectedDayKeys
        var calendarBucketsToSort: Set<CalendarListMirror.ID> = []
        var dayBucketsToSort: Set<TimeInterval> = []
        for id in changedIDs {
            let old = priorChanged[id]
            let new = nextChanged[id]
            let oldKeys = old.map { Self.calendarDisplayDayKeys(for: $0, settings: settings) } ?? []
            let newKeys = new.map { Self.calendarDisplayDayKeys(for: $0, settings: settings) } ?? []
            affectedDayKeys.formUnion(oldKeys)
            affectedDayKeys.formUnion(newKeys)
            if let old {
                affectedCalendarIDs.insert(old.calendarID)
                applyCalendarSnapshotCountDelta(for: old, delta: -1)
            }
            if let new {
                affectedCalendarIDs.insert(new.calendarID)
                applyCalendarSnapshotCountDelta(for: new, delta: 1)
            }

            guard Self.calendarDisplayPlacementChanged(old: old, new: new, settings: settings) else {
                continue
            }
            if let old, Self.includesEventInCalendarDisplay(old, settings: settings) {
                removeEventFromCalendarBucket(old)
                calendarBucketsToSort.insert(old.calendarID)
                for key in oldKeys {
                    removeEventID(old.id, fromDayKey: key)
                    dayBucketsToSort.insert(key)
                }
            }
            if let new, Self.includesEventInCalendarDisplay(new, settings: settings) {
                eventsByCalendar[new.calendarID, default: []].append(new.id)
                calendarBucketsToSort.insert(new.calendarID)
                for key in newKeys {
                    eventsByDay[key, default: []].append(new.id)
                    dayBucketsToSort.insert(key)
                }
            }
        }
        for calendarID in calendarBucketsToSort {
            sortCalendarBucket(calendarID)
        }
        for dayKey in affectedDayKeys {
            if dayBucketsToSort.contains(dayKey) {
                sortDayBucket(dayKey)
            }
            calendarDayDisplayRevisions[dayKey, default: 0] &+= 1
        }
        calendarRevisionKeyCache.removeAll()

        #if DEBUG
        let bucketEnd = DispatchTime.now().uptimeNanoseconds
        let todayStart = bucketEnd
        #endif
        var scheduledEvents = todaySnapshot.scheduledEvents
        scheduledEvents.removeAll { changedIDs.contains($0.id) }
        let calendar = Calendar.current
        for event in nextChanged.values
            where event.status != .cancelled && calendar.isDate(event.startDate, inSameDayAs: referenceDate) {
            scheduledEvents.append(event)
        }
        todaySnapshot = TodaySnapshot.build(
            tasks: tasks,
            scheduledEvents: scheduledEvents,
            referenceDate: referenceDate,
            calendar: calendar
        )

        dataRevision &+= 1
        commandPaletteEntityRevision &+= 1
        menuBarDisplayRevision &+= 1
        setDerivedSnapshotRebuildInFlight(false)
        #if DEBUG
        let todayEnd = DispatchTime.now().uptimeNanoseconds
        lastEventOnlyApplyProfileForBenchmark = AppModelEventOnlyApplyProfile(
            lookupMilliseconds: AppModelApplyProfile.milliseconds(from: lookupStart, to: lookupEnd),
            indexMilliseconds: AppModelApplyProfile.milliseconds(from: indexStart, to: indexEnd),
            snapshotMapMilliseconds: AppModelApplyProfile.milliseconds(from: snapshotMapStart, to: snapshotMapEnd),
            bucketMilliseconds: AppModelApplyProfile.milliseconds(from: bucketStart, to: bucketEnd),
            todayMilliseconds: AppModelApplyProfile.milliseconds(from: todayStart, to: todayEnd),
            totalMilliseconds: AppModelApplyProfile.milliseconds(from: totalStart, to: todayEnd)
        )
        #endif
        HCBPerformanceTelemetry.debug(
            "event-only derived update applied",
            metadata: [
                "events": "\(changedIDs.count)",
                "days": "\(affectedDayKeys.count)",
                "calendars": "\(affectedCalendarIDs.count)"
            ]
        )
    }

    private static func calendarDisplayPlacementChanged(
        old: CalendarEventMirror?,
        new: CalendarEventMirror?,
        settings: AppSettings
    ) -> Bool {
        guard let old, let new else { return true }
        return old.calendarID != new.calendarID
            || old.startDate != new.startDate
            || old.endDate != new.endDate
            || old.isAllDay != new.isAllDay
            || includesEventInCalendarDisplay(old, settings: settings) != includesEventInCalendarDisplay(new, settings: settings)
    }

    private func removeEventFromCalendarBucket(_ event: CalendarEventMirror) {
        guard var bucket = eventsByCalendar[event.calendarID] else { return }
        bucket.removeAll { $0 == event.id }
        if bucket.isEmpty {
            eventsByCalendar.removeValue(forKey: event.calendarID)
        } else {
            eventsByCalendar[event.calendarID] = bucket
        }
    }

    private func removeEventID(_ eventID: CalendarEventMirror.ID, fromDayKey dayKey: TimeInterval) {
        guard var bucket = eventsByDay[dayKey] else { return }
        bucket.removeAll { $0 == eventID }
        if bucket.isEmpty {
            eventsByDay.removeValue(forKey: dayKey)
        } else {
            eventsByDay[dayKey] = bucket
        }
    }

    private func sortCalendarBucket(_ calendarID: CalendarListMirror.ID) {
        guard var bucket = eventsByCalendar[calendarID] else { return }
        bucket = Array(Set(bucket)).sorted { lhs, rhs in
            guard let left = eventByIDSnapshot[lhs], let right = eventByIDSnapshot[rhs] else {
                return lhs < rhs
            }
            if left.startDate != right.startDate { return left.startDate < right.startDate }
            return lhs < rhs
        }
        eventsByCalendar[calendarID] = bucket
    }

    private func sortDayBucket(_ dayKey: TimeInterval) {
        guard var bucket = eventsByDay[dayKey] else { return }
        bucket = Array(Set(bucket)).sorted { lhs, rhs in
            guard let left = eventByIDSnapshot[lhs], let right = eventByIDSnapshot[rhs] else {
                return lhs < rhs
            }
            if left.isAllDay != right.isAllDay { return left.isAllDay && right.isAllDay == false }
            if left.startDate != right.startDate { return left.startDate < right.startDate }
            return lhs < rhs
        }
        eventsByDay[dayKey] = bucket
    }

    private func applyCalendarSnapshotCountDelta(for event: CalendarEventMirror, delta: Int) {
        guard calendarSnapshot.selectedCalendarIDs.contains(event.calendarID),
              Self.includesEventInCalendarDisplay(event, settings: settings)
        else { return }
        Self.applyCountDelta(delta, key: event.calendarID, to: &calendarSnapshot.eventCountsByCalendarID)
        let colorID = CalendarEventViewFilter.eventColorID(for: event)
        Self.applyCountDelta(delta, key: colorID, to: &calendarSnapshot.eventCountsByColorID)
        let colorTagIndex = CalendarEventViewFilter.colorTagIndex(from: settings.colorTagBindings)
        for tagName in CalendarEventViewFilter.tagNames(in: event, eventColorID: colorID, colorTagIndex: colorTagIndex) {
            Self.applyCountDelta(delta, key: tagName, to: &calendarSnapshot.eventCountsByTagName)
        }
    }

    private static func applyCountDelta<Key: Hashable>(_ delta: Int, key: Key, to counts: inout [Key: Int]) {
        let next = (counts[key] ?? 0) + delta
        if next > 0 {
            counts[key] = next
        } else {
            counts.removeValue(forKey: key)
        }
    }

    private static func includesEventInCalendarDisplay(_ event: CalendarEventMirror, settings: AppSettings) -> Bool {
        settings.showCompletedItemsInCalendar || event.status != .cancelled
    }

    private static func calendarDisplayDayKeys(
        for event: CalendarEventMirror,
        settings: AppSettings,
        calendar: Calendar = .current
    ) -> Set<TimeInterval> {
        guard includesEventInCalendarDisplay(event, settings: settings) else { return [] }
        let startDay = calendar.startOfDay(for: event.startDate)
        let endDay: Date
        if event.isAllDay {
            endDay = calendar.startOfDay(for: event.endDate)
        } else {
            let nextStartDay = calendar.date(byAdding: .day, value: 1, to: startDay)
            let isSameDayTimedEvent = event.endDate >= startDay
                && (nextStartDay.map { event.endDate < $0 } ?? false)
            endDay = isSameDayTimedEvent ? startDay : calendar.startOfDay(for: event.endDate)
        }
        var keys: Set<TimeInterval> = []
        var day = startDay
        var steps = 0
        while day <= endDay && steps < 366 {
            keys.insert(day.timeIntervalSinceReferenceDate)
            guard let next = calendar.date(byAdding: .day, value: 1, to: day) else { break }
            day = next
            steps += 1
        }
        return keys
    }

    private func logDerivedSnapshotRebuild(started: ContinuousClock.Instant, snapshots: DerivedAppSnapshots) {
        let elapsed = started.duration(to: .now)
        let micros = (elapsed.components.seconds * 1_000_000)
            + (elapsed.components.attoseconds / 1_000_000_000_000)
        AppLogger.debug("rebuildSnapshots", category: .perf, metadata: [
            "duration_us": String(micros),
            "tasks": String(snapshots.taskCount),
            "events": String(snapshots.eventCount),
            "visible_tasks": String(snapshots.visibleTaskCount)
        ])
    }

    private nonisolated static func buildDerivedSnapshots(_ input: DerivedAppSnapshotInput) -> DerivedAppSnapshots {
        #if DEBUG
        let totalStart = DispatchTime.now().uptimeNanoseconds
        let visibilityStart = DispatchTime.now().uptimeNanoseconds
        #endif
        let visibleTaskListIDs = input.settings.hasConfiguredTaskListSelection
            ? input.settings.selectedTaskListIDs
            : Set(input.taskLists.map(\.id))
        let visibleTaskLists = input.taskLists.filter { visibleTaskListIDs.contains($0.id) }
        let visibleTasks = input.tasks.filter { visibleTaskListIDs.contains($0.taskListID) }
        let tasksTabVisibleListIDs = input.settings.hasConfiguredTasksTabSelection
            ? input.settings.tasksTabSelectedListIDs
            : visibleTaskListIDs
        let notesTabVisibleListIDs = input.settings.hasConfiguredNotesTabSelection
            ? input.settings.notesTabSelectedListIDs
            : visibleTaskListIDs

        #if DEBUG
        let visibilityEnd = DispatchTime.now().uptimeNanoseconds
        let taskSectionsStart = DispatchTime.now().uptimeNanoseconds
        #endif
        let taskSections = TaskListSectionSnapshot.build(taskLists: visibleTaskLists, tasks: visibleTasks)
        #if DEBUG
        let taskSectionsEnd = DispatchTime.now().uptimeNanoseconds
        let taskBoardStart = taskSectionsEnd
        #endif
        let taskBoardSnapshot = TaskBoardSnapshot.build(
            tasks: input.tasks,
            tasksTabVisibleListIDs: tasksTabVisibleListIDs,
            notesTabVisibleListIDs: notesTabVisibleListIDs,
            settings: input.settings,
            referenceDate: input.referenceDate
        )
        #if DEBUG
        let taskBoardEnd = DispatchTime.now().uptimeNanoseconds
        let eventBucketStart = DispatchTime.now().uptimeNanoseconds
        var eventsByCalendarNanoseconds: UInt64 = 0
        var eventHiddenFilteringNanoseconds: UInt64 = 0
        var eventStartDayNormalizationNanoseconds: UInt64 = 0
        var eventEndDayNormalizationNanoseconds: UInt64 = 0
        var eventSameDayProbeNanoseconds: UInt64 = 0
        var eventSingleDayTimedNanoseconds: UInt64 = 0
        var eventAllDayNanoseconds: UInt64 = 0
        var eventMultiDayNanoseconds: UInt64 = 0
        var eventDaySteppingNanoseconds: UInt64 = 0
        var eventAllDaySteppingNanoseconds: UInt64 = 0
        var eventMultiDaySteppingNanoseconds: UInt64 = 0
        var eventBucketInsertionNanoseconds: UInt64 = 0
        var hiddenEventCount = 0
        var singleDayTimedEventCount = 0
        var allDayEventCount = 0
        var multiDayEventCount = 0
        #endif

        // Bucket events by calendar id so grid views can skip full-corpus
        // filters while rendering.
        struct EventDayBounds {
            var id: CalendarEventMirror.ID
            var isAllDay: Bool
            var startDay: Date
            var endDay: Date
            var startKey: TimeInterval
        }

        let cal = Calendar.current
        let referenceDay = cal.startOfDay(for: input.referenceDate)
        let shouldIncludeCancelledEvents = input.settings.showCompletedItemsInCalendar
        var eventsByCalendar: [CalendarListMirror.ID: [CalendarEventMirror.ID]] = [:]
        eventsByCalendar.reserveCapacity(input.calendars.count)
        var eventDayBounds: [EventDayBounds] = []
        eventDayBounds.reserveCapacity(input.events.count)
        var todayScheduledEvents: [CalendarEventMirror] = []
        todayScheduledEvents.reserveCapacity(min(input.events.count, 512))
        for event in input.events {
            #if DEBUG
            let eventFilterStart = DispatchTime.now().uptimeNanoseconds
            #endif
            let isVisibleEvent = shouldIncludeCancelledEvents || event.status != .cancelled
            #if DEBUG
            eventHiddenFilteringNanoseconds += DispatchTime.now().uptimeNanoseconds - eventFilterStart
            if isVisibleEvent == false { hiddenEventCount += 1 }
            #endif
            guard isVisibleEvent else { continue }

            #if DEBUG
            let eventsByCalendarAppendStart = DispatchTime.now().uptimeNanoseconds
            #endif
            eventsByCalendar[event.calendarID, default: []].append(event.id)
            #if DEBUG
            eventsByCalendarNanoseconds += DispatchTime.now().uptimeNanoseconds - eventsByCalendarAppendStart
            let startOfDayStart = DispatchTime.now().uptimeNanoseconds
            #endif

            let startDay = cal.startOfDay(for: event.startDate)
            #if DEBUG
            eventStartDayNormalizationNanoseconds += DispatchTime.now().uptimeNanoseconds - startOfDayStart
            #endif
            let endDay: Date
            if event.isAllDay {
                #if DEBUG
                let endOfDayStart = DispatchTime.now().uptimeNanoseconds
                #endif
                endDay = cal.startOfDay(for: event.endDate)
                #if DEBUG
                eventEndDayNormalizationNanoseconds += DispatchTime.now().uptimeNanoseconds - endOfDayStart
                allDayEventCount += 1
                #endif
            } else {
                #if DEBUG
                let sameDayProbeStart = DispatchTime.now().uptimeNanoseconds
                #endif
                let nextStartDay = cal.date(byAdding: .day, value: 1, to: startDay)
                let isSameDayTimedEvent = event.endDate >= startDay
                    && (nextStartDay.map { event.endDate < $0 } ?? false)
                #if DEBUG
                eventSameDayProbeNanoseconds += DispatchTime.now().uptimeNanoseconds - sameDayProbeStart
                #endif

                if isSameDayTimedEvent {
                    endDay = startDay
                    #if DEBUG
                    singleDayTimedEventCount += 1
                    #endif
                } else {
                    #if DEBUG
                    let endOfDayStart = DispatchTime.now().uptimeNanoseconds
                    #endif
                    endDay = cal.startOfDay(for: event.endDate)
                    #if DEBUG
                    eventEndDayNormalizationNanoseconds += DispatchTime.now().uptimeNanoseconds - endOfDayStart
                    if startDay == endDay {
                        singleDayTimedEventCount += 1
                    } else {
                        multiDayEventCount += 1
                    }
                    #endif
                }
            }
            if event.status != .cancelled && startDay == referenceDay {
                todayScheduledEvents.append(event)
            }

            eventDayBounds.append(EventDayBounds(
                id: event.id,
                isAllDay: event.isAllDay,
                startDay: startDay,
                endDay: endDay,
                startKey: startDay.timeIntervalSinceReferenceDate
            ))
        }

        #if DEBUG
        let eventsByDayStart = DispatchTime.now().uptimeNanoseconds
        #endif
        var eventsByDay: [TimeInterval: [CalendarEventMirror.ID]] = [:]
        eventsByDay.reserveCapacity(eventDayBounds.count)
        for eventDayBound in eventDayBounds {
            #if DEBUG
            let categoryStart = DispatchTime.now().uptimeNanoseconds
            #endif

            if eventDayBound.isAllDay == false && eventDayBound.startDay == eventDayBound.endDay {
                #if DEBUG
                let bucketInsertionStart = DispatchTime.now().uptimeNanoseconds
                #endif
                eventsByDay[eventDayBound.startKey, default: []].append(eventDayBound.id)
                #if DEBUG
                eventBucketInsertionNanoseconds += DispatchTime.now().uptimeNanoseconds - bucketInsertionStart
                #endif
                #if DEBUG
                eventSingleDayTimedNanoseconds += DispatchTime.now().uptimeNanoseconds - categoryStart
                #endif
                continue
            }

            var day = eventDayBound.startDay
            var steps = 0
            while day <= eventDayBound.endDay && steps < 366 {
                #if DEBUG
                let bucketInsertionStart = DispatchTime.now().uptimeNanoseconds
                #endif
                eventsByDay[day.timeIntervalSinceReferenceDate, default: []].append(eventDayBound.id)
                #if DEBUG
                eventBucketInsertionNanoseconds += DispatchTime.now().uptimeNanoseconds - bucketInsertionStart
                #endif
                #if DEBUG
                let dayStepStart = DispatchTime.now().uptimeNanoseconds
                #endif
                guard let next = cal.date(byAdding: .day, value: 1, to: day) else { break }
                #if DEBUG
                let dayStepElapsed = DispatchTime.now().uptimeNanoseconds - dayStepStart
                eventDaySteppingNanoseconds += dayStepElapsed
                if eventDayBound.isAllDay {
                    eventAllDaySteppingNanoseconds += dayStepElapsed
                } else {
                    eventMultiDaySteppingNanoseconds += dayStepElapsed
                }
                #endif
                day = next
                steps += 1
            }
            #if DEBUG
            let categoryElapsed = DispatchTime.now().uptimeNanoseconds - categoryStart
            if eventDayBound.isAllDay {
                eventAllDayNanoseconds += categoryElapsed
            } else {
                eventMultiDayNanoseconds += categoryElapsed
            }
            #endif
        }
        #if DEBUG
        let eventBucketEnd = DispatchTime.now().uptimeNanoseconds
        let todaySnapshotStart = eventBucketEnd
        #endif
        #if DEBUG
        let todaySnapshotResult = TodaySnapshot.buildProfiled(
            tasks: visibleTasks,
            scheduledEvents: todayScheduledEvents,
            referenceDate: input.referenceDate
        )
        let todaySnapshot = todaySnapshotResult.0
        let todaySnapshotProfile = todaySnapshotResult.1
        #else
        let todaySnapshot = TodaySnapshot.build(
            tasks: visibleTasks,
            scheduledEvents: todayScheduledEvents,
            referenceDate: input.referenceDate
        )
        #endif
        #if DEBUG
        let todaySnapshotEnd = DispatchTime.now().uptimeNanoseconds
        let calendarSnapshotStart = todaySnapshotEnd
        #endif
        #if DEBUG
        let calendarSnapshotResult = CalendarSnapshot.buildProfiled(
            calendars: input.calendars,
            events: input.events,
            settings: input.settings
        )
        let calendarSnapshot = calendarSnapshotResult.0
        let calendarSnapshotProfile = calendarSnapshotResult.1
        #else
        let calendarSnapshot = CalendarSnapshot.build(
            calendars: input.calendars,
            events: input.events,
            settings: input.settings
        )
        #endif
        #if DEBUG
        let calendarSnapshotEnd = DispatchTime.now().uptimeNanoseconds
        let taskBucketStart = DispatchTime.now().uptimeNanoseconds
        let taskDueBucketsStart = taskBucketStart
        #endif

        var tasksByDueDate: [TimeInterval: [TaskMirror.ID]] = [:]
        for task in input.tasks where task.isDeleted == false && (input.settings.showCompletedItemsInCalendar || task.isCompleted == false) {
            guard let due = task.dueDate else { continue }
            let key = cal.startOfDay(for: due).timeIntervalSinceReferenceDate
            tasksByDueDate[key, default: []].append(task.id)
        }

        #if DEBUG
        let taskDueBucketsEnd = DispatchTime.now().uptimeNanoseconds
        let taskSidebarCountsStart = taskDueBucketsEnd
        #endif
        var dated = 0
        var undated = 0
        for task in input.tasks where task.isCompleted == false && task.isDeleted == false {
            if task.appearsInNotesSurface {
                if notesTabVisibleListIDs.contains(task.taskListID) { undated += 1 }
            }
            if task.dueDate != nil, tasksTabVisibleListIDs.contains(task.taskListID) {
                dated += 1
            }
        }

        #if DEBUG
        let taskSidebarCountsEnd = DispatchTime.now().uptimeNanoseconds
        let taskCompletionStatsStart = taskSidebarCountsEnd
        #endif
        var stats: [TaskListMirror.ID: TaskListCompletionStats] = [:]
        for task in input.tasks where task.isDeleted == false {
            var entry = stats[task.taskListID] ?? TaskListCompletionStats(total: 0, completed: 0)
            entry.total += 1
            if task.isCompleted { entry.completed += 1 }
            stats[task.taskListID] = entry
        }
        #if DEBUG
        let taskBucketEnd = DispatchTime.now().uptimeNanoseconds
        let idIndexStart = DispatchTime.now().uptimeNanoseconds
        let taskIndexStart = idIndexStart
        #endif

        var taskIndexByID: [TaskMirror.ID: Int] = [:]
        var taskByIDSnapshot: [TaskMirror.ID: TaskMirror] = [:]
        taskIndexByID.reserveCapacity(input.tasks.count)
        taskByIDSnapshot.reserveCapacity(input.tasks.count)
        for (index, task) in input.tasks.enumerated() {
            taskIndexByID[task.id] = index
            taskByIDSnapshot[task.id] = task
        }

        #if DEBUG
        let taskIndexEnd = DispatchTime.now().uptimeNanoseconds
        let eventIndexStart = taskIndexEnd
        #endif
        var eventIndexByID: [CalendarEventMirror.ID: Int] = [:]
        var eventByIDSnapshot: [CalendarEventMirror.ID: CalendarEventMirror] = [:]
        eventIndexByID.reserveCapacity(input.events.count)
        eventByIDSnapshot.reserveCapacity(input.events.count)
        for (index, event) in input.events.enumerated() {
            eventIndexByID[event.id] = index
            eventByIDSnapshot[event.id] = event
        }
        #if DEBUG
        let idIndexEnd = DispatchTime.now().uptimeNanoseconds
        let auxiliaryStart = DispatchTime.now().uptimeNanoseconds
        let titleLookupStart = auxiliaryStart
        #endif

        let taskListTitleByID = Dictionary(uniqueKeysWithValues: input.taskLists.map { ($0.id, $0.title) })
        let calendarTitleByID = Dictionary(uniqueKeysWithValues: input.calendars.map { ($0.id, $0.summary) })
        #if DEBUG
        let titleLookupEnd = DispatchTime.now().uptimeNanoseconds
        let taskChildrenStart = titleLookupEnd
        #endif
        let taskChildrenByParentID = buildTaskChildrenByParentID(tasks: input.tasks)
        #if DEBUG
        let taskChildrenEnd = DispatchTime.now().uptimeNanoseconds
        let duplicateIndexStart = taskChildrenEnd
        #endif
        let duplicateIndex = DuplicateIndex.build(
            tasks: input.tasks,
            dismissedGroupKeys: input.settings.dismissedDuplicateGroups
        )
        #if DEBUG
        let duplicateIndexEnd = DispatchTime.now().uptimeNanoseconds
        let taskListIndexStart = duplicateIndexEnd
        #endif
        let taskListIndexByID = Dictionary(uniqueKeysWithValues: input.taskLists.enumerated().map { ($0.element.id, $0.offset) })
        #if DEBUG
        let auxiliaryEnd = DispatchTime.now().uptimeNanoseconds
        let taskListIndexEnd = auxiliaryEnd
        let fingerprintStart = DispatchTime.now().uptimeNanoseconds
        #endif

        let displayFingerprint = calendarDisplayFingerprint(
            input: input,
            selectedCalendarIDs: calendarSnapshot.selectedCalendarIDs,
            visibleTaskListIDs: visibleTaskListIDs
        )
        #if DEBUG
        let fingerprintEnd = DispatchTime.now().uptimeNanoseconds
        let coreSnapshotMilliseconds =
            AppModelApplyProfile.milliseconds(from: taskSectionsStart, to: taskSectionsEnd)
            + AppModelApplyProfile.milliseconds(from: taskBoardStart, to: taskBoardEnd)
            + AppModelApplyProfile.milliseconds(from: todaySnapshotStart, to: todaySnapshotEnd)
            + AppModelApplyProfile.milliseconds(from: calendarSnapshotStart, to: calendarSnapshotEnd)
        let buildProfile = DerivedSnapshotBuildProfile(
            visibilityMilliseconds: AppModelApplyProfile.milliseconds(from: visibilityStart, to: visibilityEnd),
            taskSectionsMilliseconds: AppModelApplyProfile.milliseconds(from: taskSectionsStart, to: taskSectionsEnd),
            taskBoardMilliseconds: AppModelApplyProfile.milliseconds(from: taskBoardStart, to: taskBoardEnd),
            todaySnapshotMilliseconds: AppModelApplyProfile.milliseconds(from: todaySnapshotStart, to: todaySnapshotEnd),
            todayTaskDueMilliseconds: todaySnapshotProfile.dueTaskMilliseconds,
            todayTaskOverdueMilliseconds: todaySnapshotProfile.overdueTaskMilliseconds,
            todayEventFilterMilliseconds: todaySnapshotProfile.eventFilterMilliseconds,
            todayEventSortMilliseconds: todaySnapshotProfile.eventSortMilliseconds,
            todayScheduledEventCount: todaySnapshotProfile.scheduledEventCount,
            calendarSnapshotMilliseconds: AppModelApplyProfile.milliseconds(from: calendarSnapshotStart, to: calendarSnapshotEnd),
            calendarSetupMilliseconds: calendarSnapshotProfile.setupMilliseconds,
            calendarEventScanMilliseconds: calendarSnapshotProfile.eventScanMilliseconds,
            calendarEventVisibilityMilliseconds: calendarSnapshotProfile.eventVisibilityMilliseconds,
            calendarCountMilliseconds: calendarSnapshotProfile.calendarCountMilliseconds,
            calendarColorAggregationMilliseconds: calendarSnapshotProfile.colorAggregationMilliseconds,
            calendarLiteralTagExtractionMilliseconds: calendarSnapshotProfile.literalTagExtractionMilliseconds,
            calendarLiteralSummaryScanMilliseconds: calendarSnapshotProfile.literalSummaryScanMilliseconds,
            calendarLiteralDetailsScanMilliseconds: calendarSnapshotProfile.literalDetailsScanMilliseconds,
            calendarLiteralLocationScanMilliseconds: calendarSnapshotProfile.literalLocationScanMilliseconds,
            calendarLiteralRegexMatchingMilliseconds: calendarSnapshotProfile.literalRegexMatchingMilliseconds,
            calendarLiteralDeduplicationMilliseconds: calendarSnapshotProfile.literalDeduplicationMilliseconds,
            calendarBoundTagAggregationMilliseconds: calendarSnapshotProfile.boundTagAggregationMilliseconds,
            calendarTagCountMilliseconds: calendarSnapshotProfile.tagCountMilliseconds,
            calendarColorMapMilliseconds: calendarSnapshotProfile.colorMapMilliseconds,
            calendarSelectedEventCount: calendarSnapshotProfile.selectedEventCount,
            calendarVisibleEventCount: calendarSnapshotProfile.visibleEventCount,
            calendarLiteralTaggedEventCount: calendarSnapshotProfile.literalTaggedEventCount,
            calendarBoundTaggedEventCount: calendarSnapshotProfile.boundTaggedEventCount,
            coreSnapshotMilliseconds: coreSnapshotMilliseconds,
            eventsByCalendarMilliseconds: Double(eventsByCalendarNanoseconds) / 1_000_000,
            eventsByDayMilliseconds: AppModelApplyProfile.milliseconds(from: eventsByDayStart, to: eventBucketEnd),
            eventHiddenFilteringMilliseconds: Double(eventHiddenFilteringNanoseconds) / 1_000_000,
            eventStartOfDayMilliseconds: Double(eventStartDayNormalizationNanoseconds + eventEndDayNormalizationNanoseconds) / 1_000_000,
            eventStartDayNormalizationMilliseconds: Double(eventStartDayNormalizationNanoseconds) / 1_000_000,
            eventEndDayNormalizationMilliseconds: Double(eventEndDayNormalizationNanoseconds) / 1_000_000,
            eventSameDayProbeMilliseconds: Double(eventSameDayProbeNanoseconds) / 1_000_000,
            eventSingleDayTimedMilliseconds: Double(eventSingleDayTimedNanoseconds) / 1_000_000,
            eventAllDayMilliseconds: Double(eventAllDayNanoseconds) / 1_000_000,
            eventMultiDayMilliseconds: Double(eventMultiDayNanoseconds) / 1_000_000,
            eventDaySteppingMilliseconds: Double(eventDaySteppingNanoseconds) / 1_000_000,
            eventAllDaySteppingMilliseconds: Double(eventAllDaySteppingNanoseconds) / 1_000_000,
            eventMultiDaySteppingMilliseconds: Double(eventMultiDaySteppingNanoseconds) / 1_000_000,
            eventBucketInsertionMilliseconds: Double(eventBucketInsertionNanoseconds) / 1_000_000,
            eventBucketMilliseconds: AppModelApplyProfile.milliseconds(from: eventBucketStart, to: eventBucketEnd),
            hiddenEventCount: hiddenEventCount,
            singleDayTimedEventCount: singleDayTimedEventCount,
            allDayEventCount: allDayEventCount,
            multiDayEventCount: multiDayEventCount,
            taskDueBucketsMilliseconds: AppModelApplyProfile.milliseconds(from: taskDueBucketsStart, to: taskDueBucketsEnd),
            taskSidebarCountsMilliseconds: AppModelApplyProfile.milliseconds(from: taskSidebarCountsStart, to: taskSidebarCountsEnd),
            taskCompletionStatsMilliseconds: AppModelApplyProfile.milliseconds(from: taskCompletionStatsStart, to: taskBucketEnd),
            taskBucketAndStatsMilliseconds: AppModelApplyProfile.milliseconds(from: taskBucketStart, to: taskBucketEnd),
            taskIndexMilliseconds: AppModelApplyProfile.milliseconds(from: taskIndexStart, to: taskIndexEnd),
            eventIndexMilliseconds: AppModelApplyProfile.milliseconds(from: eventIndexStart, to: idIndexEnd),
            idIndexMilliseconds: AppModelApplyProfile.milliseconds(from: idIndexStart, to: idIndexEnd),
            titleLookupMilliseconds: AppModelApplyProfile.milliseconds(from: titleLookupStart, to: titleLookupEnd),
            taskChildrenMilliseconds: AppModelApplyProfile.milliseconds(from: taskChildrenStart, to: taskChildrenEnd),
            duplicateIndexMilliseconds: AppModelApplyProfile.milliseconds(from: duplicateIndexStart, to: duplicateIndexEnd),
            taskListIndexMilliseconds: AppModelApplyProfile.milliseconds(from: taskListIndexStart, to: taskListIndexEnd),
            auxiliaryLookupMilliseconds: AppModelApplyProfile.milliseconds(from: auxiliaryStart, to: auxiliaryEnd),
            fingerprintMilliseconds: AppModelApplyProfile.milliseconds(from: fingerprintStart, to: fingerprintEnd),
            totalMilliseconds: AppModelApplyProfile.milliseconds(from: totalStart, to: fingerprintEnd)
        )
        let optionalBuildProfile: DerivedSnapshotBuildProfile? = buildProfile
        #else
        let optionalBuildProfile: DerivedSnapshotBuildProfile? = nil
        #endif

        return DerivedAppSnapshots(
            visibleTaskListIDs: visibleTaskListIDs,
            taskSections: taskSections,
            taskBoardSnapshot: taskBoardSnapshot,
            todaySnapshot: todaySnapshot,
            calendarSnapshot: calendarSnapshot,
            eventsByCalendar: eventsByCalendar,
            eventsByDay: eventsByDay,
            tasksByDueDate: tasksByDueDate,
            datedOpenTaskCount: dated,
            undatedOpenTaskCount: undated,
            openTaskCountForSidebar: dated + undated,
            taskListCompletionStats: stats,
            taskListTitleByID: taskListTitleByID,
            calendarTitleByID: calendarTitleByID,
            taskChildrenByParentID: taskChildrenByParentID,
            duplicateIndex: duplicateIndex,
            taskByIDSnapshot: taskByIDSnapshot,
            eventByIDSnapshot: eventByIDSnapshot,
            taskIndexByID: taskIndexByID,
            eventIndexByID: eventIndexByID,
            taskListIndexByID: taskListIndexByID,
            calendarDisplayFingerprint: displayFingerprint,
            taskCount: input.tasks.count,
            eventCount: input.events.count,
            visibleTaskCount: visibleTasks.count,
            buildProfile: optionalBuildProfile
        )
    }

    private nonisolated static func calendarDisplayFingerprint(
        input: DerivedAppSnapshotInput,
        selectedCalendarIDs: Set<CalendarListMirror.ID>,
        visibleTaskListIDs: Set<TaskListMirror.ID>
    ) -> Int {
        var hasher = Hasher()
        hasher.combine(input.settings.showCompletedItemsInCalendar)
        hasher.combine(input.settings.pastEventBehavior.rawValue)
        hasher.combine(input.settings.overdueTaskBehavior.rawValue)
        for entry in input.settings.colorTagBindings.sorted(by: { $0.key < $1.key }) {
            hasher.combine(entry.key)
            hasher.combine(entry.value)
        }
        for calendarID in selectedCalendarIDs.sorted() {
            hasher.combine(calendarID)
        }
        for taskListID in visibleTaskListIDs.sorted() {
            hasher.combine(taskListID)
        }
        for calendar in input.calendars {
            hasher.combine(calendar.id)
            hasher.combine(calendar.summary)
            hasher.combine(calendar.colorHex)
            hasher.combine(calendar.isSelected)
        }
        for event in input.events where selectedCalendarIDs.contains(event.calendarID) {
            hasher.combine(event)
        }
        for task in input.tasks where task.dueDate != nil && task.isDeleted == false {
            hasher.combine(task)
        }
        return hasher.finalize()
    }

    private nonisolated static func buildTaskChildrenByParentID(tasks: [TaskMirror]) -> [TaskMirror.ID: [TaskMirror.ID]] {
        var grouped: [TaskMirror.ID: [TaskMirror]] = [:]
        for task in tasks where task.isDeleted == false {
            guard let parentID = task.parentID else { continue }
            grouped[parentID, default: []].append(task)
        }
        return grouped.mapValues { children in
            TaskHierarchy.sortByPosition(children).map(\.id)
        }
    }

    // Public re-index helper for call sites that change dismissedDuplicateGroups but don't otherwise mutate tasks (avoids paying the full rebuildSnapshots cost).
    func rebuildDuplicateIndex() {
        duplicateIndex = DuplicateIndex.build(
            tasks: tasks,
            dismissedGroupKeys: settings.dismissedDuplicateGroups
        )
    }
}

private struct DerivedAppSnapshotInput: Sendable {
    var taskLists: [TaskListMirror]
    var tasks: [TaskMirror]
    var calendars: [CalendarListMirror]
    var events: [CalendarEventMirror]
    var settings: AppSettings
    var referenceDate: Date
}

private struct DerivedAppSnapshots: Sendable {
    var visibleTaskListIDs: Set<TaskListMirror.ID>
    var taskSections: [TaskListSectionSnapshot]
    var taskBoardSnapshot: TaskBoardSnapshot
    var todaySnapshot: TodaySnapshot
    var calendarSnapshot: CalendarSnapshot
    var eventsByCalendar: [CalendarListMirror.ID: [CalendarEventMirror.ID]]
    var eventsByDay: [TimeInterval: [CalendarEventMirror.ID]]
    var tasksByDueDate: [TimeInterval: [TaskMirror.ID]]
    var datedOpenTaskCount: Int
    var undatedOpenTaskCount: Int
    var openTaskCountForSidebar: Int
    var taskListCompletionStats: [TaskListMirror.ID: TaskListCompletionStats]
    var taskListTitleByID: [TaskListMirror.ID: String]
    var calendarTitleByID: [CalendarListMirror.ID: String]
    var taskChildrenByParentID: [TaskMirror.ID: [TaskMirror.ID]]
    var duplicateIndex: DuplicateIndex
    var taskByIDSnapshot: [TaskMirror.ID: TaskMirror]
    var eventByIDSnapshot: [CalendarEventMirror.ID: CalendarEventMirror]
    var taskIndexByID: [TaskMirror.ID: Int]
    var eventIndexByID: [CalendarEventMirror.ID: Int]
    var taskListIndexByID: [TaskListMirror.ID: Int]
    var calendarDisplayFingerprint: Int
    var taskCount: Int
    var eventCount: Int
    var visibleTaskCount: Int
    var buildProfile: DerivedSnapshotBuildProfile?
}

@MainActor
private final class CalendarPreparedSnapshotCache {
    private let daySnapshots: PreparedSnapshotLRU<CalendarDayDisplaySnapshot>
    private let weekSnapshots: PreparedSnapshotLRU<CalendarWeekDisplaySnapshot>
    private let agendaSnapshots: PreparedSnapshotLRU<CalendarAgendaDisplaySnapshot>
    private let yearSnapshots: PreparedSnapshotLRU<CalendarYearDisplaySnapshot>

    init(limit: Int) {
        daySnapshots = PreparedSnapshotLRU(limit: limit)
        weekSnapshots = PreparedSnapshotLRU(limit: limit)
        agendaSnapshots = PreparedSnapshotLRU(limit: limit)
        yearSnapshots = PreparedSnapshotLRU(limit: limit)
    }

    func daySnapshot(for key: PreparedSnapshotKey) -> CalendarDayDisplaySnapshot? {
        daySnapshots.value(for: key)
    }

    func store(_ snapshot: CalendarDayDisplaySnapshot) {
        daySnapshots.insert(snapshot, for: snapshot.key)
    }

    func weekSnapshot(for key: PreparedSnapshotKey) -> CalendarWeekDisplaySnapshot? {
        weekSnapshots.value(for: key)
    }

    func store(_ snapshot: CalendarWeekDisplaySnapshot) {
        weekSnapshots.insert(snapshot, for: snapshot.key)
    }

    func agendaSnapshot(for key: PreparedSnapshotKey) -> CalendarAgendaDisplaySnapshot? {
        agendaSnapshots.value(for: key)
    }

    func store(_ snapshot: CalendarAgendaDisplaySnapshot) {
        agendaSnapshots.insert(snapshot, for: snapshot.key)
    }

    func yearSnapshot(for key: PreparedSnapshotKey) -> CalendarYearDisplaySnapshot? {
        yearSnapshots.value(for: key)
    }

    func store(_ snapshot: CalendarYearDisplaySnapshot) {
        yearSnapshots.insert(snapshot, for: snapshot.key)
    }

    func removeAll() {
        daySnapshots.removeAll()
        weekSnapshots.removeAll()
        agendaSnapshots.removeAll()
        yearSnapshots.removeAll()
    }
}

private final class PreparedSnapshotLRU<Value> {
    private let limit: Int
    private var values: [PreparedSnapshotKey: Value] = [:]
    private var recency: [PreparedSnapshotKey] = []

    init(limit: Int) {
        self.limit = max(1, limit)
    }

    func value(for key: PreparedSnapshotKey) -> Value? {
        guard let value = values[key] else { return nil }
        touch(key)
        return value
    }

    func insert(_ value: Value, for key: PreparedSnapshotKey) {
        values[key] = value
        touch(key)
        while recency.count > limit, let evicted = recency.first {
            recency.removeFirst()
            values.removeValue(forKey: evicted)
        }
    }

    func removeAll() {
        values.removeAll()
        recency.removeAll()
    }

    private func touch(_ key: PreparedSnapshotKey) {
        recency.removeAll { $0 == key }
        recency.append(key)
    }
}

private final class EmptyGoogleOAuthTokenStore: GoogleOAuthTokenStoring, @unchecked Sendable {
    func loadClientConfiguration() -> GoogleOAuthClientConfiguration? { nil }
    func saveClientConfiguration(_ configuration: GoogleOAuthClientConfiguration) throws {}
    func clearClientConfiguration() {}
    func loadTokenSet(accountID: GoogleAccount.ID) -> CustomGoogleOAuthTokenSet? { nil }
    func loadFirstTokenSet() -> CustomGoogleOAuthTokenSet? { nil }
    func saveTokenSet(_ tokenSet: CustomGoogleOAuthTokenSet, accountID: GoogleAccount.ID) throws {}
    func deleteTokenSet(accountID: GoogleAccount.ID) {}
    func deleteAllTokenSets() {}
}

struct TaskListCompletionStats: Equatable, Sendable {
    var total: Int
    var completed: Int
    var openCount: Int { total - completed }
    var fraction: Double { total == 0 ? 0 : Double(completed) / Double(total) }
}
