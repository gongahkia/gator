import Foundation

enum SyncMode: String, CaseIterable, Identifiable, Codable, Sendable {
    case manual
    case balanced
    case nearRealtime

    var id: String { rawValue }

    var title: String {
        switch self {
        case .manual:
            "Manual"
        case .balanced:
            "Balanced"
        case .nearRealtime:
            "Near real-time"
        }
    }

    var detail: String {
        switch self {
        case .manual:
            "Only refresh when requested."
        case .balanced:
            "Refresh on launch, foreground, and periodic app activity."
        case .nearRealtime:
            "Poll more aggressively while foregrounded with backoff."
        }
    }

    var guidance: String {
        switch self {
        case .manual:
            "Only syncs when you tap Refresh. Best for low-bandwidth or API-quota-sensitive setups."
        case .balanced:
            "Syncs on launch and when you return to the app. Recommended."
        case .nearRealtime:
            "Polls every 90 seconds while the app is open. Highest Google API usage."
        }
    }
}

enum CloudSyncTarget: String, CaseIterable, Identifiable, Codable, Hashable, Sendable {
    case tasks
    case events

    var id: String { rawValue }

    var title: String {
        switch self {
        case .tasks:
            "Tasks and notes"
        case .events:
            "Calendar events"
        }
    }

    var systemImage: String {
        switch self {
        case .tasks:
            "checklist"
        case .events:
            "calendar"
        }
    }

    var detail: String {
        switch self {
        case .tasks:
            "Google Tasks lists, tasks, notes, and queued task writes."
        case .events:
            "Google Calendar lists, events, and queued event writes."
        }
    }

    static let all: Set<CloudSyncTarget> = Set(allCases)
}

extension Set where Element == CloudSyncTarget {
    var syncsTasks: Bool { contains(.tasks) }
    var syncsEvents: Bool { contains(.events) }

    func allows(_ resourceType: SyncResourceType) -> Bool {
        switch resourceType {
        case .task, .taskList:
            syncsTasks
        case .event, .calendar:
            syncsEvents
        }
    }
}

enum SyncState: Equatable, Sendable {
    case idle
    case syncing(startedAt: Date)
    case synced(at: Date)
    case failed(message: String)

    var title: String {
        switch self {
        case .idle:
            "Ready"
        case .syncing:
            "Syncing"
        case .synced:
            "Synced"
        case .failed:
            "Sync failed"
        }
    }
}

enum MCPPermissionMode: String, CaseIterable, Identifiable, Codable, Hashable, Sendable {
    case readOnly
    case confirmWrites
    case allowWrites

    var id: String { rawValue }

    var title: String {
        switch self {
        case .readOnly:
            "Read-only"
        case .confirmWrites:
            "Confirm writes"
        case .allowWrites:
            "Allow writes"
        }
    }

    var detail: String {
        switch self {
        case .readOnly:
            "MCP clients can search and read Hot Cross Buns data but cannot change it."
        case .confirmWrites:
            "MCP clients must dry-run writes and pass the returned confirmation id before changes apply."
        case .allowWrites:
            "MCP clients can apply non-destructive writes directly. Deletes still require confirmation."
        }
    }
}

struct SyncCheckpoint: Identifiable, Hashable, Codable, Sendable {
    let id: String
    var accountID: GoogleAccount.ID
    var resourceType: SyncResourceType
    var resourceID: String
    var calendarSyncToken: String?
    var tasksUpdatedMin: Date?
    var lastSuccessfulSyncAt: Date?
}

extension SyncCheckpoint {
    static func stableID(
        accountID: GoogleAccount.ID,
        resourceType: SyncResourceType,
        resourceID: String
    ) -> String {
        "\(accountID)::\(resourceType.rawValue)::\(resourceID)"
    }
}

struct SyncApplyResult: Sendable {
    var state: CachedAppState
    var changeSet: SyncChangeSet
}

struct SyncChangeSet: Equatable, Sendable {
    struct RowChanges: Equatable, Sendable {
        var inserted: Set<String> = []
        var updated: Set<String> = []
        var deleted: Set<String> = []
        var unchanged: Set<String> = []

        var hasChanges: Bool {
            inserted.isEmpty == false || updated.isEmpty == false || deleted.isEmpty == false
        }

        var touched: Set<String> {
            inserted.union(updated).union(deleted).union(unchanged)
        }

        mutating func formUnion(_ other: RowChanges) {
            inserted.formUnion(other.inserted)
            updated.formUnion(other.updated)
            deleted.formUnion(other.deleted)
            unchanged.formUnion(other.unchanged)
        }
    }

    var taskLists = RowChanges()
    var tasks = RowChanges()
    var calendars = RowChanges()
    var events = RowChanges()
    var checkpoints = RowChanges()
    var pendingMutations = RowChanges()
    var affectedDayKeys: Set<TimeInterval> = []
    var affectedCalendarIDs: Set<CalendarListMirror.ID> = []
    var affectedTaskListIDs: Set<TaskListMirror.ID> = []
    var settingsChanged = false
    var checkpointChanged = false

    static let empty = SyncChangeSet()

    var hasRowChanges: Bool {
        taskLists.hasChanges
            || tasks.hasChanges
            || calendars.hasChanges
            || events.hasChanges
            || checkpoints.hasChanges
            || pendingMutations.hasChanges
    }

    var requiresDerivedSnapshotRebuild: Bool {
        taskLists.hasChanges
            || tasks.hasChanges
            || calendars.hasChanges
            || events.hasChanges
            || settingsChanged
    }

    var canApplyEventOnlyDerivedUpdate: Bool {
        events.hasChanges
            && taskLists.hasChanges == false
            && tasks.hasChanges == false
            && calendars.hasChanges == false
            && settingsChanged == false
    }

    var canUseNarrowCalendarInvalidation: Bool {
        affectedDayKeys.isEmpty == false
            && calendars.hasChanges == false
            && settingsChanged == false
    }
}

typealias CachePersistenceChangeSet = SyncChangeSet

extension SyncChangeSet {
    mutating func formUnion(_ other: SyncChangeSet) {
        taskLists.formUnion(other.taskLists)
        tasks.formUnion(other.tasks)
        calendars.formUnion(other.calendars)
        events.formUnion(other.events)
        checkpoints.formUnion(other.checkpoints)
        pendingMutations.formUnion(other.pendingMutations)
        affectedDayKeys.formUnion(other.affectedDayKeys)
        affectedCalendarIDs.formUnion(other.affectedCalendarIDs)
        affectedTaskListIDs.formUnion(other.affectedTaskListIDs)
        settingsChanged = settingsChanged || other.settingsChanged
        checkpointChanged = checkpointChanged || other.checkpointChanged
    }

    func unioning(_ other: SyncChangeSet) -> SyncChangeSet {
        var copy = self
        copy.formUnion(other)
        return copy
    }
}

enum LocalCacheChangeSetBuilder {
    static func eventInserted(_ event: CalendarEventMirror) -> CachePersistenceChangeSet {
        var changeSet = CachePersistenceChangeSet.empty
        changeSet.events.inserted.insert(event.id)
        addAffectedEvent(event, to: &changeSet)
        return changeSet
    }

    static func eventUpdated(old: CalendarEventMirror?, new: CalendarEventMirror) -> CachePersistenceChangeSet {
        var changeSet = CachePersistenceChangeSet.empty
        if let old, old.id != new.id {
            changeSet.events.deleted.insert(old.id)
            changeSet.events.inserted.insert(new.id)
        } else {
            changeSet.events.updated.insert(new.id)
        }
        if let old {
            addAffectedEvent(old, to: &changeSet)
        }
        addAffectedEvent(new, to: &changeSet)
        return changeSet
    }

    static func eventDeleted(_ event: CalendarEventMirror) -> CachePersistenceChangeSet {
        var changeSet = CachePersistenceChangeSet.empty
        changeSet.events.deleted.insert(event.id)
        addAffectedEvent(event, to: &changeSet)
        return changeSet
    }

    static func taskInserted(_ task: TaskMirror) -> CachePersistenceChangeSet {
        var changeSet = CachePersistenceChangeSet.empty
        changeSet.tasks.inserted.insert(task.id)
        addAffectedTask(task, to: &changeSet)
        return changeSet
    }

    static func taskUpdated(old: TaskMirror?, new: TaskMirror) -> CachePersistenceChangeSet {
        var changeSet = CachePersistenceChangeSet.empty
        if let old, old.id != new.id {
            changeSet.tasks.deleted.insert(old.id)
            changeSet.tasks.inserted.insert(new.id)
        } else {
            changeSet.tasks.updated.insert(new.id)
        }
        if let old {
            addAffectedTask(old, to: &changeSet)
        }
        addAffectedTask(new, to: &changeSet)
        return changeSet
    }

    static func taskDeleted(_ task: TaskMirror) -> CachePersistenceChangeSet {
        var changeSet = CachePersistenceChangeSet.empty
        changeSet.tasks.deleted.insert(task.id)
        addAffectedTask(task, to: &changeSet)
        return changeSet
    }

    static func pendingMutationInserted(_ mutation: PendingMutation) -> CachePersistenceChangeSet {
        pendingMutationInserted(id: mutation.id)
    }

    static func pendingMutationInserted(id: PendingMutation.ID) -> CachePersistenceChangeSet {
        var changeSet = CachePersistenceChangeSet.empty
        changeSet.pendingMutations.inserted.insert(id.uuidString)
        return changeSet
    }

    static func pendingMutationUpdated(_ mutation: PendingMutation) -> CachePersistenceChangeSet {
        pendingMutationUpdated(id: mutation.id)
    }

    static func pendingMutationUpdated(id: PendingMutation.ID) -> CachePersistenceChangeSet {
        var changeSet = CachePersistenceChangeSet.empty
        changeSet.pendingMutations.updated.insert(id.uuidString)
        return changeSet
    }

    static func pendingMutationDeleted(_ mutation: PendingMutation) -> CachePersistenceChangeSet {
        pendingMutationDeleted(id: mutation.id)
    }

    static func pendingMutationDeleted(id: PendingMutation.ID) -> CachePersistenceChangeSet {
        var changeSet = CachePersistenceChangeSet.empty
        changeSet.pendingMutations.deleted.insert(id.uuidString)
        return changeSet
    }

    static func combined(_ changeSets: CachePersistenceChangeSet...) -> CachePersistenceChangeSet {
        combined(changeSets)
    }

    static func combined(_ changeSets: [CachePersistenceChangeSet]) -> CachePersistenceChangeSet {
        var combined = CachePersistenceChangeSet.empty
        for changeSet in changeSets {
            combined.formUnion(changeSet)
        }
        return combined
    }

    static func dayKeys(forTask task: TaskMirror, calendar: Calendar = .current) -> Set<TimeInterval> {
        guard let dueDate = task.dueDate else { return [] }
        return [calendar.startOfDay(for: dueDate).timeIntervalSinceReferenceDate]
    }

    static func dayKeys(forEvent event: CalendarEventMirror, calendar: Calendar = .current) -> Set<TimeInterval> {
        guard event.status != .cancelled else { return [] }
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

    private static func addAffectedEvent(_ event: CalendarEventMirror, to changeSet: inout CachePersistenceChangeSet) {
        changeSet.affectedCalendarIDs.insert(event.calendarID)
        changeSet.affectedDayKeys.formUnion(dayKeys(forEvent: event))
    }

    private static func addAffectedTask(_ task: TaskMirror, to changeSet: inout CachePersistenceChangeSet) {
        changeSet.affectedTaskListIDs.insert(task.taskListID)
        changeSet.affectedDayKeys.formUnion(dayKeys(forTask: task))
    }
}

enum SyncResourceType: String, Hashable, Codable, Sendable {
    case taskList
    case calendar
    case task
    case event
}

struct PendingMutation: Identifiable, Hashable, Codable, Sendable {
    let id: UUID
    var accountID: GoogleAccount.ID?
    var createdAt: Date
    var resourceType: SyncResourceType
    var resourceID: String
    var action: PendingMutationAction
    var payload: Data
    // Poison-pill tracking (added in B). attemptCount increments on every
    // transient failure; lastAttemptAt gates replay until the backoff window
    // passes; quarantinedAt moves the mutation out of the automatic replay
    // loop after BackoffPolicy.maxAttempts failures. Codable decodeIfPresent
    // back-compat: pre-B queued mutations decode with zeroes and are treated
    // as fresh.
    var attemptCount: Int = 0
    var lastAttemptAt: Date? = nil
    var lastErrorSummary: String? = nil
    var quarantinedAt: Date? = nil
    // Set when the mutation was quarantined specifically because the server
    // returned 412 Precondition Failed (the canonical "someone else edited
    // this resource" signal). Drives the conflict-resolution UI — the user
    // can either force-overwrite (re-issue without etag) or discard.
    var conflictedAt: Date? = nil

    enum CodingKeys: String, CodingKey {
        case id, accountID, createdAt, resourceType, resourceID, action, payload
        case attemptCount, lastAttemptAt, lastErrorSummary, quarantinedAt, conflictedAt
    }

    init(
        id: UUID,
        accountID: GoogleAccount.ID? = nil,
        createdAt: Date,
        resourceType: SyncResourceType,
        resourceID: String,
        action: PendingMutationAction,
        payload: Data,
        attemptCount: Int = 0,
        lastAttemptAt: Date? = nil,
        lastErrorSummary: String? = nil,
        quarantinedAt: Date? = nil,
        conflictedAt: Date? = nil
    ) {
        self.id = id
        self.accountID = accountID
        self.createdAt = createdAt
        self.resourceType = resourceType
        self.resourceID = resourceID
        self.action = action
        self.payload = payload
        self.attemptCount = attemptCount
        self.lastAttemptAt = lastAttemptAt
        self.lastErrorSummary = lastErrorSummary
        self.quarantinedAt = quarantinedAt
        self.conflictedAt = conflictedAt
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(UUID.self, forKey: .id)
        accountID = try c.decodeIfPresent(GoogleAccount.ID.self, forKey: .accountID)
        createdAt = try c.decode(Date.self, forKey: .createdAt)
        resourceType = try c.decode(SyncResourceType.self, forKey: .resourceType)
        resourceID = try c.decode(String.self, forKey: .resourceID)
        action = try c.decode(PendingMutationAction.self, forKey: .action)
        payload = try c.decode(Data.self, forKey: .payload)
        attemptCount = try c.decodeIfPresent(Int.self, forKey: .attemptCount) ?? 0
        lastAttemptAt = try c.decodeIfPresent(Date.self, forKey: .lastAttemptAt)
        lastErrorSummary = try c.decodeIfPresent(String.self, forKey: .lastErrorSummary)
        quarantinedAt = try c.decodeIfPresent(Date.self, forKey: .quarantinedAt)
        conflictedAt = try c.decodeIfPresent(Date.self, forKey: .conflictedAt)
    }

    var isQuarantined: Bool { quarantinedAt != nil }
    var isConflict: Bool { conflictedAt != nil }

    // Returns true when the backoff window has elapsed (or no prior attempt).
    // A mutation becomes eligible for retry `policy.delay(forAttempt:)` after
    // its `lastAttemptAt`. Quarantined mutations never auto-replay.
    func isReadyToReplay(now: Date, policy: BackoffPolicy) -> Bool {
        guard isQuarantined == false else { return false }
        guard let last = lastAttemptAt else { return true }
        let delay = policy.delay(forAttempt: max(0, attemptCount - 1))
        let secs = Double(delay.components.seconds) + Double(delay.components.attoseconds) / 1e18
        return now.timeIntervalSince(last) >= secs
    }
}

enum PendingMutationAction: String, Hashable, Codable, Sendable {
    case create
    case update
    case completion
    case delete
}

enum AppLanguage: String, CaseIterable, Identifiable, Hashable, Sendable, Codable {
    case system
    case en
    case ms
    case ta
    case zhHans = "zh-Hans"
    case id
    case vi
    case th
    case ja
    case ko
    case zhHant = "zh-Hant"
    case hi

    var id: String { rawValue }

    var locale: Locale {
        guard let identifier = localeIdentifier else { return .autoupdatingCurrent }
        return Locale(identifier: identifier)
    }

    var localeIdentifier: String? {
        switch self {
        case .system: nil
        case .en: "en"
        case .ms: "ms"
        case .ta: "ta"
        case .zhHans: "zh-Hans"
        case .id: "id"
        case .vi: "vi"
        case .th: "th"
        case .ja: "ja"
        case .ko: "ko"
        case .zhHant: "zh-Hant"
        case .hi: "hi"
        }
    }

    var title: String {
        switch self {
        case .system: "System Default"
        case .en: "English"
        case .ms: "Bahasa Melayu"
        case .ta: "தமிழ்"
        case .zhHans: "简体中文"
        case .id: "Bahasa Indonesia"
        case .vi: "Tiếng Việt"
        case .th: "ไทย"
        case .ja: "日本語"
        case .ko: "한국어"
        case .zhHant: "繁體中文"
        case .hi: "हिन्दी"
        }
    }

    var helloWordmark: String {
        switch self {
        case .system: AppLanguage.preferredSupportedLanguage.helloWordmark
        case .en: "Hello"
        case .ms: "Helo"
        case .ta: "வணக்கம்"
        case .zhHans: "你好"
        case .id: "Halo"
        case .vi: "Xin chào"
        case .th: "สวัสดี"
        case .ja: "こんにちは"
        case .ko: "안녕하세요"
        case .zhHant: "你好"
        case .hi: "नमस्ते"
        }
    }

    static var preferredSupportedLanguage: AppLanguage {
        Locale.preferredLanguages
            .lazy
            .compactMap(AppLanguage.init(localeIdentifier:))
            .first ?? .en
    }

    init?(localeIdentifier: String) {
        let components = Locale.Components(identifier: localeIdentifier).languageComponents
        let languageCode = components.languageCode?.identifier.lowercased()
        let scriptCode = components.script?.identifier.lowercased()
        let regionCode = components.region?.identifier.lowercased()
        switch languageCode {
        case "en": self = .en
        case "ms": self = .ms
        case "ta": self = .ta
        case "id": self = .id
        case "vi": self = .vi
        case "th": self = .th
        case "ja": self = .ja
        case "ko": self = .ko
        case "hi": self = .hi
        case "zh" where scriptCode == "hant" || regionCode == "tw" || regionCode == "hk" || regionCode == "mo":
            self = .zhHant
        case "zh" where scriptCode == nil || scriptCode == "hans": self = .zhHans
        default: return nil
        }
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        let rawValue = try container.decode(String.self)
        self = AppLanguage(rawValue: rawValue) ?? .system
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.singleValueContainer()
        try container.encode(rawValue)
    }
}

enum NavigationSurfacePlacement: String, CaseIterable, Identifiable, Hashable, Sendable, Codable {
    case left
    case right
    case top
    case bottom

    var id: String { rawValue }

    var title: String {
        switch self {
        case .left: "Left"
        case .right: "Right"
        case .top: "Top"
        case .bottom: "Bottom"
        }
    }

    var systemImage: String {
        switch self {
        case .left: "sidebar.left"
        case .right: "sidebar.right"
        case .top: "rectangle.topthird.inset.filled"
        case .bottom: "rectangle.bottomthird.inset.filled"
        }
    }

    var isHorizontal: Bool {
        switch self {
        case .top, .bottom: true
        case .left, .right: false
        }
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        let rawValue = try container.decode(String.self)
        self = NavigationSurfacePlacement(rawValue: rawValue) ?? .left
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.singleValueContainer()
        try container.encode(rawValue)
    }
}

enum AppPerformanceMode: String, Codable, Hashable, Sendable, CaseIterable, Identifiable {
    case snappy
    case rich

    var id: String { rawValue }

    var title: String {
        switch self {
        case .snappy: "Snappy"
        case .rich: "Rich"
        }
    }

    var summary: String {
        switch self {
        case .snappy: "Prioritizes fast calendar transitions, immediate search, and opaque surfaces."
        case .rich: "Restores hover previews, translucent backgrounds, and custom image effects."
        }
    }
}

struct AppSettings: Hashable, Codable, Sendable {
    var syncMode: SyncMode
    var cloudSyncTargets: Set<CloudSyncTarget>
    var selectedCalendarIDs: Set<CalendarListMirror.ID>
    var selectedTaskListIDs: Set<TaskListMirror.ID>
    var hasConfiguredCalendarSelection: Bool
    var hasConfiguredTaskListSelection: Bool
    var enableLocalNotifications: Bool
    var enableTaskCompletionSound: Bool
    var enableEventCompletionSound: Bool
    var taskCompletionSoundChoice: CompletionSoundChoice
    var eventCompletionSoundChoice: CompletionSoundChoice
    var appLanguage: AppLanguage
    var hasCompletedOnboarding: Bool
    var hasSeenFeatureTour: Bool
    var showMenuBarExtra: Bool
    var showDetailedMenuBar: Bool
    var showMenuBarBadge: Bool
    var menuBarIcon: MenuBarIcon
    var showDockBadge: Bool
    var restoreWindowStateEnabled: Bool
    var enableGlobalHotkey: Bool
    var globalHotkeyBinding: GlobalHotkeyBinding
    var customFilters: [CustomFilterDefinition]
    var eventTemplates: [EventTemplate]
    var customCompletionSounds: [CompletionSoundAsset]
    var menuBarStyle: MenuBarStyle
    var menuBarAdaptiveStatusSource: MenuBarAdaptiveStatusSource
    var menuBarAdaptiveEmptyBehavior: MenuBarAdaptiveEmptyBehavior
    var menuBarAdaptivePanelContent: MenuBarAdaptivePanelContent
    var uiLayoutScale: Double // 0.80–1.50, geometric scale of UI chrome only (not text)
    var uiTextSizePoints: Double // literal body-text point size (9–24), drives every semantic style
    var uiFontName: String? // PostScript name, nil for system
    var colorSchemeID: String // identifier into HCBColorScheme.all
    var customColorSchemes: [HCBCustomColorScheme] // user-authored palettes, exportable with settings
    var appBackgroundTranslucencyEnabled: Bool // true = clear NSWindow + translucent app fill
    var appBackgroundOpacity: Double // 0.35-1.0, applied to the app fill over desktop/custom image
    var customBackgroundImagePath: String? // copied image in Application Support, nil = theme/window background
    var disableAnimations: Bool // app-level Reduce Motion override for users who want no UI motion
    var performanceMode: AppPerformanceMode // snappy by default; rich restores expensive presentation affordances
    var shortcutOverrides: [String: HCBKeyBinding] // HCBShortcutCommand.rawValue → binding
    var sidebarPlacement: NavigationSurfacePlacement // where the app navigation surface is rendered
    var hiddenSidebarItems: Set<String> // SidebarItem.rawValues user has hidden
    var hiddenCalendarViewModes: Set<String> // CalendarGridMode.rawValues user has hidden from Calendar picker
    var perSurfaceFontOverrides: [String: HCBSurfaceFontOverride] // HCBSurface.rawValue → override
    var cacheEncryptionEnabled: Bool // §6.12 — whether LocalCacheStore should encrypt at rest
    var auditLogEncryptionEnabled: Bool // whether MutationAuditLog should encrypt history at rest
    var rawGoogleDiagnosticsEnabled: Bool // local-only field-redacted Google payload snippets in Diagnostics logs
    var taskTemplates: [TaskTemplate] // §6.13 — local-only task templates with variable expansion
    var eventRetentionDaysBack: Int // §7.02 — drop events with endDate older than (now - N days) during sync merge; 0 = keep forever
    var completedTaskRetentionDaysBack: Int // drop completed tasks older than (now - N days) during sync merge; 0 = keep forever
    var collapsedTaskListIDs: Set<TaskListMirror.ID> // §7.01 — task-list section IDs the user has folded shut in StoreView
    var multiDayCount: Int // §7.01 Phase D2 — day count for Multi-Day view (2-7, default 3)
    var quickCreateExpandedByDefault: Bool // §7.01 follow-up — show all QuickCreate fields up-front instead of behind [+ More]
    // App-wide task reminder policy. Per-task offsets were removed in F-reminders
    // because Google Tasks API has no reminder field and writing offsets into
    // notes violated the "no new schema Google has to read" invariant. Now every
    // open task with a due date fires a single local notification at
    // `due - thresholdDays` on this Mac at taskReminderHour:taskReminderMinute.
    // 0 = disabled. Defaults: 7 days before, 09:00.
    var taskReminderThresholdDays: Int
    var taskReminderHour: Int
    var taskReminderMinute: Int

    // Per-tab list visibility overrides. When `hasConfigured…` is true the
    // corresponding set replaces the global selectedTaskListIDs for that
    // tab only. Purely local — Google never sees this.
    var tasksTabSelectedListIDs: Set<TaskListMirror.ID>
    var hasConfiguredTasksTabSelection: Bool
    var notesTabSelectedListIDs: Set<TaskListMirror.ID>
    var hasConfiguredNotesTabSelection: Bool

    // Notes tab layout. Persisted so the choice survives relaunch.
    var notesViewMode: NotesViewMode
    var notesKanbanColumnMode: KanbanColumnMode

    // Color-tag auto-apply (events only). bindings = colorId ("1".."11") → tag spelling.
    var colorTagAutoApplyEnabled: Bool
    var colorTagBindings: [String: String]
    var colorTagMatchPolicy: ColorTagMatchPolicy

    // Past-cleanup behaviors. Every mode defaults to showAll so the
    // feature is opt-in on every axis. Deletion modes require the user
    // to acknowledge the blast-radius modal; the ack flags reset when
    // they flip the mode off, so toggling back on re-prompts.
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

    // Duplicate-detection dismissals. Key = stable hash of the sorted member
    // IDs in a duplicate group. Group composition changes (edits, deletes)
    // invalidate the dismissal automatically because the hash no longer matches.
    var dismissedDuplicateGroups: Set<String>

    // History-log (audit log surfacing) preferences.
    var historyVisibleLimit: Int           // N most recent entries shown in window (default 50)
    var historyStorageCap: Int             // total entries retained on disk (default 5000, ceiling 50000)
    var historyCategoryFilters: Set<String> // "create"|"edit"|"delete"|"complete"|"duplicate"|"move"|"clipboard"|"restore"|"bulk"|"sync"|"other"
    var dailyLocalBackupEnabled: Bool
    var dailyLocalBackupRetentionCount: Int
    var lastDailyLocalBackupAt: Date?
    var mcpServerEnabled: Bool
    var mcpPermissionMode: MCPPermissionMode
    var mcpPort: Int

    init(
        syncMode: SyncMode,
        cloudSyncTargets: Set<CloudSyncTarget> = CloudSyncTarget.all,
        selectedCalendarIDs: Set<CalendarListMirror.ID>,
        selectedTaskListIDs: Set<TaskListMirror.ID>,
        hasConfiguredCalendarSelection: Bool = false,
        hasConfiguredTaskListSelection: Bool = false,
        enableLocalNotifications: Bool,
        enableTaskCompletionSound: Bool = true,
        enableEventCompletionSound: Bool = true,
        taskCompletionSoundChoice: CompletionSoundChoice = .defaultTask,
        eventCompletionSoundChoice: CompletionSoundChoice = .defaultEvent,
        appLanguage: AppLanguage = .system,
        hasCompletedOnboarding: Bool = false,
        hasSeenFeatureTour: Bool = false,
        showMenuBarExtra: Bool = true,
        showDetailedMenuBar: Bool = false,
        showMenuBarBadge: Bool = true,
        menuBarIcon: MenuBarIcon = .buns,
        showDockBadge: Bool = true,
        restoreWindowStateEnabled: Bool = true,
        enableGlobalHotkey: Bool = true,
        globalHotkeyBinding: GlobalHotkeyBinding = .defaultQuickAdd,
        customFilters: [CustomFilterDefinition] = [],
        eventTemplates: [EventTemplate] = [],
        customCompletionSounds: [CompletionSoundAsset] = [],
        menuBarStyle: MenuBarStyle = .compact,
        menuBarAdaptiveStatusSource: MenuBarAdaptiveStatusSource = .events,
        menuBarAdaptiveEmptyBehavior: MenuBarAdaptiveEmptyBehavior = .iconOnly,
        menuBarAdaptivePanelContent: MenuBarAdaptivePanelContent = .events,
        uiLayoutScale: Double = 1.0,
        uiTextSizePoints: Double = 13.0,
        uiFontName: String? = nil,
        colorSchemeID: String = "notion",
        customColorSchemes: [HCBCustomColorScheme] = [],
        appBackgroundTranslucencyEnabled: Bool = false,
        appBackgroundOpacity: Double = 1.0,
        customBackgroundImagePath: String? = nil,
        disableAnimations: Bool = false,
        performanceMode: AppPerformanceMode = .snappy,
        shortcutOverrides: [String: HCBKeyBinding] = [:],
        sidebarPlacement: NavigationSurfacePlacement = .left,
        hiddenSidebarItems: Set<String> = [],
        hiddenCalendarViewModes: Set<String> = [],
        perSurfaceFontOverrides: [String: HCBSurfaceFontOverride] = [:],
        cacheEncryptionEnabled: Bool = false,
        auditLogEncryptionEnabled: Bool = false,
        rawGoogleDiagnosticsEnabled: Bool = false,
        taskTemplates: [TaskTemplate] = [],
        eventRetentionDaysBack: Int = 365,
        completedTaskRetentionDaysBack: Int = 365,
        collapsedTaskListIDs: Set<TaskListMirror.ID> = [],
        multiDayCount: Int = 3,
        quickCreateExpandedByDefault: Bool = true,
        taskReminderThresholdDays: Int = 7,
        taskReminderHour: Int = 9,
        taskReminderMinute: Int = 0,
        tasksTabSelectedListIDs: Set<TaskListMirror.ID> = [],
        hasConfiguredTasksTabSelection: Bool = false,
        notesTabSelectedListIDs: Set<TaskListMirror.ID> = [],
        hasConfiguredNotesTabSelection: Bool = false,
        notesViewMode: NotesViewMode = .grid,
        notesKanbanColumnMode: KanbanColumnMode = .byList,
        colorTagAutoApplyEnabled: Bool = false,
        colorTagBindings: [String: String] = [:],
        colorTagMatchPolicy: ColorTagMatchPolicy = .firstMatch,
        pastEventBehavior: PastEventBehavior = .showAll,
        pastEventDeleteThresholdDays: Int = 30,
        allowDeletingAttendeeEvents: Bool = false,
        showCompletedItemsInCalendar: Bool = false,
        overdueTaskBehavior: OverdueTaskBehavior = .showAll,
        completedTaskBehavior: CompletedTaskBehavior = .showAll,
        completedTaskDeleteThresholdDays: Int = 30,
        hasAckedEventDeletion: Bool = false,
        hasAckedAttendeeDeletion: Bool = false,
        hasAckedTaskDeletion: Bool = false,
        dismissedDuplicateGroups: Set<String> = [],
        historyVisibleLimit: Int = 50,
        historyStorageCap: Int = 5000,
        historyCategoryFilters: Set<String> = ["create", "edit", "delete", "complete", "duplicate", "move", "clipboard", "restore", "bulk", "other"],
        dailyLocalBackupEnabled: Bool = false,
        dailyLocalBackupRetentionCount: Int = 14,
        lastDailyLocalBackupAt: Date? = nil,
        mcpServerEnabled: Bool = false,
        mcpPermissionMode: MCPPermissionMode = .confirmWrites,
        mcpPort: Int = 8765
    ) {
        self.syncMode = syncMode
        self.cloudSyncTargets = cloudSyncTargets
        self.selectedCalendarIDs = selectedCalendarIDs
        self.selectedTaskListIDs = selectedTaskListIDs
        self.hasConfiguredCalendarSelection = hasConfiguredCalendarSelection
        self.hasConfiguredTaskListSelection = hasConfiguredTaskListSelection
        self.enableLocalNotifications = enableLocalNotifications
        self.enableTaskCompletionSound = enableTaskCompletionSound
        self.enableEventCompletionSound = enableEventCompletionSound
        self.taskCompletionSoundChoice = taskCompletionSoundChoice
        self.eventCompletionSoundChoice = eventCompletionSoundChoice
        self.appLanguage = appLanguage
        self.hasCompletedOnboarding = hasCompletedOnboarding
        self.hasSeenFeatureTour = hasSeenFeatureTour
        self.showMenuBarExtra = showMenuBarExtra
        self.showDetailedMenuBar = showDetailedMenuBar
        self.showMenuBarBadge = showMenuBarBadge
        self.menuBarIcon = menuBarIcon
        self.showDockBadge = showDockBadge
        self.restoreWindowStateEnabled = restoreWindowStateEnabled
        self.enableGlobalHotkey = enableGlobalHotkey
        self.globalHotkeyBinding = globalHotkeyBinding
        self.customFilters = customFilters
        self.eventTemplates = eventTemplates
        self.customCompletionSounds = customCompletionSounds
        self.menuBarStyle = menuBarStyle
        self.menuBarAdaptiveStatusSource = menuBarAdaptiveStatusSource
        self.menuBarAdaptiveEmptyBehavior = menuBarAdaptiveEmptyBehavior
        self.menuBarAdaptivePanelContent = menuBarAdaptivePanelContent
        self.uiLayoutScale = uiLayoutScale
        self.uiTextSizePoints = uiTextSizePoints
        self.uiFontName = uiFontName
        self.colorSchemeID = colorSchemeID
        self.customColorSchemes = customColorSchemes
        self.appBackgroundTranslucencyEnabled = appBackgroundTranslucencyEnabled
        self.appBackgroundOpacity = max(0.35, min(1.0, appBackgroundOpacity))
        let normalizedBackgroundPath = customBackgroundImagePath?.trimmingCharacters(in: .whitespacesAndNewlines)
        self.customBackgroundImagePath = (normalizedBackgroundPath?.isEmpty ?? true) ? nil : normalizedBackgroundPath
        self.disableAnimations = disableAnimations
        self.performanceMode = performanceMode
        self.shortcutOverrides = shortcutOverrides
        self.sidebarPlacement = sidebarPlacement
        self.hiddenSidebarItems = hiddenSidebarItems
        self.hiddenCalendarViewModes = hiddenCalendarViewModes
        self.perSurfaceFontOverrides = perSurfaceFontOverrides
        self.cacheEncryptionEnabled = cacheEncryptionEnabled
        self.auditLogEncryptionEnabled = auditLogEncryptionEnabled
        self.rawGoogleDiagnosticsEnabled = rawGoogleDiagnosticsEnabled
        self.taskTemplates = taskTemplates
        self.eventRetentionDaysBack = max(0, min(eventRetentionDaysBack, 3650))
        self.completedTaskRetentionDaysBack = max(0, min(completedTaskRetentionDaysBack, 3650))
        self.collapsedTaskListIDs = collapsedTaskListIDs
        self.multiDayCount = max(2, min(7, multiDayCount))
        self.quickCreateExpandedByDefault = quickCreateExpandedByDefault
        self.taskReminderThresholdDays = max(0, min(365, taskReminderThresholdDays))
        self.taskReminderHour = max(0, min(23, taskReminderHour))
        self.taskReminderMinute = max(0, min(59, taskReminderMinute))
        self.tasksTabSelectedListIDs = tasksTabSelectedListIDs
        self.hasConfiguredTasksTabSelection = hasConfiguredTasksTabSelection
        self.notesTabSelectedListIDs = notesTabSelectedListIDs
        self.hasConfiguredNotesTabSelection = hasConfiguredNotesTabSelection
        self.notesViewMode = notesViewMode
        self.notesKanbanColumnMode = notesKanbanColumnMode
        self.colorTagAutoApplyEnabled = colorTagAutoApplyEnabled
        self.colorTagBindings = colorTagBindings
        self.colorTagMatchPolicy = colorTagMatchPolicy
        self.pastEventBehavior = pastEventBehavior
        self.pastEventDeleteThresholdDays = max(1, min(365, pastEventDeleteThresholdDays))
        self.allowDeletingAttendeeEvents = allowDeletingAttendeeEvents
        self.showCompletedItemsInCalendar = showCompletedItemsInCalendar
        self.overdueTaskBehavior = overdueTaskBehavior
        self.completedTaskBehavior = completedTaskBehavior
        self.completedTaskDeleteThresholdDays = max(1, min(365, completedTaskDeleteThresholdDays))
        self.hasAckedEventDeletion = hasAckedEventDeletion
        self.hasAckedAttendeeDeletion = hasAckedAttendeeDeletion
        self.hasAckedTaskDeletion = hasAckedTaskDeletion
        self.dismissedDuplicateGroups = dismissedDuplicateGroups
        self.historyVisibleLimit = max(1, min(MutationAuditLog.absoluteCeiling, historyVisibleLimit))
        self.historyStorageCap = max(1, min(MutationAuditLog.absoluteCeiling, historyStorageCap))
        self.historyCategoryFilters = historyCategoryFilters
        self.dailyLocalBackupEnabled = dailyLocalBackupEnabled
        self.dailyLocalBackupRetentionCount = max(1, min(90, dailyLocalBackupRetentionCount))
        self.lastDailyLocalBackupAt = lastDailyLocalBackupAt
        self.mcpServerEnabled = mcpServerEnabled
        self.mcpPermissionMode = mcpPermissionMode
        self.mcpPort = max(1, min(65535, mcpPort))
    }

    enum CodingKeys: String, CodingKey {
        case syncMode
        case cloudSyncTargets
        case selectedCalendarIDs
        case selectedTaskListIDs
        case hasConfiguredCalendarSelection
        case hasConfiguredTaskListSelection
        case enableLocalNotifications
        case enableTaskCompletionSound
        case enableEventCompletionSound
        case taskCompletionSoundChoice
        case eventCompletionSoundChoice
        case appLanguage
        case hasCompletedOnboarding
        case hasSeenFeatureTour
        case showMenuBarExtra
        case showDetailedMenuBar
        case showMenuBarBadge
        case menuBarIcon
        case showDockBadge
        case restoreWindowStateEnabled
        case enableGlobalHotkey
        case globalHotkeyBinding
        case customFilters
        case eventTemplates
        case customCompletionSounds
        case menuBarStyle
        case menuBarAdaptiveStatusSource
        case menuBarAdaptiveEmptyBehavior
        case menuBarAdaptivePanelContent
        case uiLayoutScale
        case uiTextSizePoints
        case uiFontName
        case colorSchemeID
        case customColorSchemes
        case appBackgroundTranslucencyEnabled
        case appBackgroundOpacity
        case customBackgroundImagePath
        case disableAnimations
        case performanceMode
        case shortcutOverrides
        case sidebarPlacement
        case hiddenSidebarItems
        case hiddenCalendarViewModes
        case perSurfaceFontOverrides
        case cacheEncryptionEnabled
        case auditLogEncryptionEnabled
        case rawGoogleDiagnosticsEnabled
        case taskTemplates
        case eventRetentionDaysBack
        case completedTaskRetentionDaysBack
        case collapsedTaskListIDs
        case multiDayCount
        case quickCreateExpandedByDefault
        case taskReminderThresholdDays
        case taskReminderHour
        case taskReminderMinute
        case tasksTabSelectedListIDs
        case hasConfiguredTasksTabSelection
        case notesTabSelectedListIDs
        case hasConfiguredNotesTabSelection
        case notesViewMode
        case notesKanbanColumnMode
        case colorTagAutoApplyEnabled
        case colorTagBindings
        case colorTagMatchPolicy
        case pastEventBehavior
        case pastEventDeleteThresholdDays
        case allowDeletingAttendeeEvents
        case showCompletedItemsInCalendar
        case overdueTaskBehavior
        case completedTaskBehavior
        case completedTaskDeleteThresholdDays
        case hasAckedEventDeletion
        case hasAckedAttendeeDeletion
        case hasAckedTaskDeletion
        case dismissedDuplicateGroups
        case historyVisibleLimit
        case historyStorageCap
        case historyCategoryFilters
        case dailyLocalBackupEnabled
        case dailyLocalBackupRetentionCount
        case lastDailyLocalBackupAt
        case mcpServerEnabled
        case mcpPermissionMode
        case mcpPort
    }

    // Legacy key (0-6 ladder) read via dynamic CodingKey so it stays out of
    // synthesized encode(to:) while still being readable by init(from:).
    private struct LegacyKey: CodingKey {
        var stringValue: String
        var intValue: Int? { nil }
        init?(stringValue: String) { self.stringValue = stringValue }
        init?(intValue: Int) { return nil }
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        syncMode = try container.decodeIfPresent(SyncMode.self, forKey: .syncMode) ?? .balanced
        cloudSyncTargets = try container.decodeIfPresent(Set<CloudSyncTarget>.self, forKey: .cloudSyncTargets) ?? CloudSyncTarget.all
        selectedCalendarIDs = try container.decodeIfPresent(Set<CalendarListMirror.ID>.self, forKey: .selectedCalendarIDs) ?? []
        selectedTaskListIDs = try container.decodeIfPresent(Set<TaskListMirror.ID>.self, forKey: .selectedTaskListIDs) ?? []
        hasConfiguredCalendarSelection = try container.decodeIfPresent(Bool.self, forKey: .hasConfiguredCalendarSelection) ?? false
        hasConfiguredTaskListSelection = try container.decodeIfPresent(Bool.self, forKey: .hasConfiguredTaskListSelection) ?? false
        enableLocalNotifications = try container.decodeIfPresent(Bool.self, forKey: .enableLocalNotifications) ?? false
        enableTaskCompletionSound = try container.decodeIfPresent(Bool.self, forKey: .enableTaskCompletionSound) ?? true
        enableEventCompletionSound = try container.decodeIfPresent(Bool.self, forKey: .enableEventCompletionSound) ?? true
        taskCompletionSoundChoice = try container.decodeIfPresent(CompletionSoundChoice.self, forKey: .taskCompletionSoundChoice) ?? .defaultTask
        eventCompletionSoundChoice = try container.decodeIfPresent(CompletionSoundChoice.self, forKey: .eventCompletionSoundChoice) ?? .defaultEvent
        appLanguage = try container.decodeIfPresent(AppLanguage.self, forKey: .appLanguage) ?? .system
        hasCompletedOnboarding = try container.decodeIfPresent(Bool.self, forKey: .hasCompletedOnboarding) ?? false
        hasSeenFeatureTour = try container.decodeIfPresent(Bool.self, forKey: .hasSeenFeatureTour) ?? false
        showMenuBarExtra = try container.decodeIfPresent(Bool.self, forKey: .showMenuBarExtra) ?? true
        showDetailedMenuBar = try container.decodeIfPresent(Bool.self, forKey: .showDetailedMenuBar) ?? false
        showMenuBarBadge = try container.decodeIfPresent(Bool.self, forKey: .showMenuBarBadge) ?? true
        menuBarIcon = try container.decodeIfPresent(MenuBarIcon.self, forKey: .menuBarIcon) ?? .buns
        showDockBadge = try container.decodeIfPresent(Bool.self, forKey: .showDockBadge) ?? true
        restoreWindowStateEnabled = try container.decodeIfPresent(Bool.self, forKey: .restoreWindowStateEnabled) ?? true
        enableGlobalHotkey = try container.decodeIfPresent(Bool.self, forKey: .enableGlobalHotkey) ?? true
        globalHotkeyBinding = try container.decodeIfPresent(GlobalHotkeyBinding.self, forKey: .globalHotkeyBinding) ?? .defaultQuickAdd
        customFilters = try container.decodeIfPresent([CustomFilterDefinition].self, forKey: .customFilters) ?? []
        eventTemplates = try container.decodeIfPresent([EventTemplate].self, forKey: .eventTemplates) ?? []
        customCompletionSounds = try container.decodeIfPresent([CompletionSoundAsset].self, forKey: .customCompletionSounds) ?? []
        // Legacy raw values may include removed cases. Decode via try? + raw-string
        // fallback so an unknown / removed case doesn't fail the whole load.
        if let explicit = try? container.decodeIfPresent(MenuBarStyle.self, forKey: .menuBarStyle) {
            menuBarStyle = explicit
        } else if let raw = try? container.decodeIfPresent(String.self, forKey: .menuBarStyle) {
            // "focusStrip" is now the new Compact panel; older compact-like
            // variants also collapse into the single compact choice.
            menuBarStyle = MenuBarStyle.legacy(rawValue: raw) ?? .compact
        } else {
            // Migrate legacy showDetailedMenuBar bool into the new style enum
            menuBarStyle = showDetailedMenuBar ? .detailed : .compact
        }
        menuBarAdaptiveStatusSource = try container.decodeIfPresent(MenuBarAdaptiveStatusSource.self, forKey: .menuBarAdaptiveStatusSource) ?? .events
        menuBarAdaptiveEmptyBehavior = try container.decodeIfPresent(MenuBarAdaptiveEmptyBehavior.self, forKey: .menuBarAdaptiveEmptyBehavior) ?? .iconOnly
        menuBarAdaptivePanelContent = try container.decodeIfPresent(MenuBarAdaptivePanelContent.self, forKey: .menuBarAdaptivePanelContent) ?? .events
        uiLayoutScale = try container.decodeIfPresent(Double.self, forKey: .uiLayoutScale) ?? 1.0
        if let points = try container.decodeIfPresent(Double.self, forKey: .uiTextSizePoints) {
            uiTextSizePoints = points
        } else if
            let legacyContainer = try? decoder.container(keyedBy: LegacyKey.self),
            let legacyKey = LegacyKey(stringValue: "uiTextSizeStep"),
            let legacyStep = try legacyContainer.decodeIfPresent(Int.self, forKey: legacyKey)
        {
            // Migrate 0-6 ladder to literal points. Prior mapping:
            // xSmall=11, small=12, medium=12, large=13, xLarge=15, xxLarge=17, xxxLarge=19.
            let ladder: [Double] = [11, 12, 12, 13, 15, 17, 19]
            uiTextSizePoints = ladder[max(0, min(legacyStep, ladder.count - 1))]
        } else {
            uiTextSizePoints = 13.0
        }
        uiFontName = try container.decodeIfPresent(String.self, forKey: .uiFontName)
        colorSchemeID = try container.decodeIfPresent(String.self, forKey: .colorSchemeID) ?? "notion"
        customColorSchemes = try container.decodeIfPresent([HCBCustomColorScheme].self, forKey: .customColorSchemes) ?? []
        appBackgroundTranslucencyEnabled = try container.decodeIfPresent(Bool.self, forKey: .appBackgroundTranslucencyEnabled) ?? false
        let rawBackgroundOpacity = try container.decodeIfPresent(Double.self, forKey: .appBackgroundOpacity) ?? 1.0
        appBackgroundOpacity = max(0.35, min(1.0, rawBackgroundOpacity))
        if let rawBackgroundPath = try container.decodeIfPresent(String.self, forKey: .customBackgroundImagePath) {
            let trimmed = rawBackgroundPath.trimmingCharacters(in: .whitespacesAndNewlines)
            customBackgroundImagePath = trimmed.isEmpty ? nil : trimmed
        } else {
            customBackgroundImagePath = nil
        }
        disableAnimations = try container.decodeIfPresent(Bool.self, forKey: .disableAnimations) ?? false
        performanceMode = try container.decodeIfPresent(AppPerformanceMode.self, forKey: .performanceMode) ?? .snappy
        shortcutOverrides = try container.decodeIfPresent([String: HCBKeyBinding].self, forKey: .shortcutOverrides) ?? [:]
        sidebarPlacement = try container.decodeIfPresent(NavigationSurfacePlacement.self, forKey: .sidebarPlacement) ?? .left
        hiddenSidebarItems = try container.decodeIfPresent(Set<String>.self, forKey: .hiddenSidebarItems) ?? []
        hiddenCalendarViewModes = try container.decodeIfPresent(Set<String>.self, forKey: .hiddenCalendarViewModes) ?? []
        perSurfaceFontOverrides = try container.decodeIfPresent([String: HCBSurfaceFontOverride].self, forKey: .perSurfaceFontOverrides) ?? [:]
        cacheEncryptionEnabled = try container.decodeIfPresent(Bool.self, forKey: .cacheEncryptionEnabled) ?? false
        auditLogEncryptionEnabled = try container.decodeIfPresent(Bool.self, forKey: .auditLogEncryptionEnabled) ?? false
        rawGoogleDiagnosticsEnabled = try container.decodeIfPresent(Bool.self, forKey: .rawGoogleDiagnosticsEnabled) ?? false
        taskTemplates = try container.decodeIfPresent([TaskTemplate].self, forKey: .taskTemplates) ?? []
        let rawEventRetention = try container.decodeIfPresent(Int.self, forKey: .eventRetentionDaysBack) ?? 365
        eventRetentionDaysBack = max(0, min(rawEventRetention, 3650))
        let rawCompletedTaskRetention = try container.decodeIfPresent(Int.self, forKey: .completedTaskRetentionDaysBack) ?? 365
        completedTaskRetentionDaysBack = max(0, min(rawCompletedTaskRetention, 3650))
        collapsedTaskListIDs = try container.decodeIfPresent(Set<TaskListMirror.ID>.self, forKey: .collapsedTaskListIDs) ?? []
        let rawMultiDay = try container.decodeIfPresent(Int.self, forKey: .multiDayCount) ?? 3
        multiDayCount = max(2, min(7, rawMultiDay))
        quickCreateExpandedByDefault = try container.decodeIfPresent(Bool.self, forKey: .quickCreateExpandedByDefault) ?? true
        let rawThreshold = try container.decodeIfPresent(Int.self, forKey: .taskReminderThresholdDays) ?? 7
        taskReminderThresholdDays = max(0, min(365, rawThreshold))
        let rawHour = try container.decodeIfPresent(Int.self, forKey: .taskReminderHour) ?? 9
        taskReminderHour = max(0, min(23, rawHour))
        let rawMinute = try container.decodeIfPresent(Int.self, forKey: .taskReminderMinute) ?? 0
        taskReminderMinute = max(0, min(59, rawMinute))
        tasksTabSelectedListIDs = try container.decodeIfPresent(Set<TaskListMirror.ID>.self, forKey: .tasksTabSelectedListIDs) ?? []
        hasConfiguredTasksTabSelection = try container.decodeIfPresent(Bool.self, forKey: .hasConfiguredTasksTabSelection) ?? false
        notesTabSelectedListIDs = try container.decodeIfPresent(Set<TaskListMirror.ID>.self, forKey: .notesTabSelectedListIDs) ?? []
        hasConfiguredNotesTabSelection = try container.decodeIfPresent(Bool.self, forKey: .hasConfiguredNotesTabSelection) ?? false
        notesViewMode = try container.decodeIfPresent(NotesViewMode.self, forKey: .notesViewMode) ?? .grid
        notesKanbanColumnMode = try container.decodeIfPresent(KanbanColumnMode.self, forKey: .notesKanbanColumnMode) ?? .byList
        colorTagAutoApplyEnabled = try container.decodeIfPresent(Bool.self, forKey: .colorTagAutoApplyEnabled) ?? false
        colorTagBindings = try container.decodeIfPresent([String: String].self, forKey: .colorTagBindings) ?? [:]
        colorTagMatchPolicy = try container.decodeIfPresent(ColorTagMatchPolicy.self, forKey: .colorTagMatchPolicy) ?? .firstMatch
        pastEventBehavior = try container.decodeIfPresent(PastEventBehavior.self, forKey: .pastEventBehavior) ?? .showAll
        let rawEventThreshold = try container.decodeIfPresent(Int.self, forKey: .pastEventDeleteThresholdDays) ?? 30
        pastEventDeleteThresholdDays = max(1, min(365, rawEventThreshold))
        allowDeletingAttendeeEvents = try container.decodeIfPresent(Bool.self, forKey: .allowDeletingAttendeeEvents) ?? false
        showCompletedItemsInCalendar = try container.decodeIfPresent(Bool.self, forKey: .showCompletedItemsInCalendar) ?? false
        overdueTaskBehavior = try container.decodeIfPresent(OverdueTaskBehavior.self, forKey: .overdueTaskBehavior) ?? .showAll
        completedTaskBehavior = try container.decodeIfPresent(CompletedTaskBehavior.self, forKey: .completedTaskBehavior) ?? .showAll
        let rawTaskThreshold = try container.decodeIfPresent(Int.self, forKey: .completedTaskDeleteThresholdDays) ?? 30
        completedTaskDeleteThresholdDays = max(1, min(365, rawTaskThreshold))
        hasAckedEventDeletion = try container.decodeIfPresent(Bool.self, forKey: .hasAckedEventDeletion) ?? false
        hasAckedAttendeeDeletion = try container.decodeIfPresent(Bool.self, forKey: .hasAckedAttendeeDeletion) ?? false
        hasAckedTaskDeletion = try container.decodeIfPresent(Bool.self, forKey: .hasAckedTaskDeletion) ?? false
        dismissedDuplicateGroups = try container.decodeIfPresent(Set<String>.self, forKey: .dismissedDuplicateGroups) ?? []
        let rawVisible = try container.decodeIfPresent(Int.self, forKey: .historyVisibleLimit) ?? 50
        historyVisibleLimit = max(1, min(MutationAuditLog.absoluteCeiling, rawVisible))
        let rawCap = try container.decodeIfPresent(Int.self, forKey: .historyStorageCap) ?? 5000
        historyStorageCap = max(1, min(MutationAuditLog.absoluteCeiling, rawCap))
        historyCategoryFilters = try container.decodeIfPresent(Set<String>.self, forKey: .historyCategoryFilters)
            ?? ["create", "edit", "delete", "complete", "duplicate", "move", "clipboard", "restore", "bulk", "other"]
        dailyLocalBackupEnabled = try container.decodeIfPresent(Bool.self, forKey: .dailyLocalBackupEnabled) ?? false
        let rawBackupRetention = try container.decodeIfPresent(Int.self, forKey: .dailyLocalBackupRetentionCount) ?? 14
        dailyLocalBackupRetentionCount = max(1, min(90, rawBackupRetention))
        lastDailyLocalBackupAt = try container.decodeIfPresent(Date.self, forKey: .lastDailyLocalBackupAt)
        mcpServerEnabled = try container.decodeIfPresent(Bool.self, forKey: .mcpServerEnabled) ?? false
        mcpPermissionMode = try container.decodeIfPresent(MCPPermissionMode.self, forKey: .mcpPermissionMode) ?? .confirmWrites
        let rawMCPPort = try container.decodeIfPresent(Int.self, forKey: .mcpPort) ?? 8765
        mcpPort = max(1, min(65535, rawMCPPort))
    }

    enum MenuBarStyle: String, Codable, Hashable, Sendable, CaseIterable {
        case adaptive
        case compact
        case detailed
        case weekly

        var title: String {
            switch self {
            case .adaptive: "Adaptive"
            case .compact: "Compact"
            case .detailed: "Calendar"
            case .weekly: "Week-at-a-glance"
            }
        }

        static func legacy(rawValue: String) -> MenuBarStyle? {
            switch rawValue {
            case "focusStrip", "minimalBadge":
                .compact
            default:
                MenuBarStyle(rawValue: rawValue)
            }
        }
    }

    enum MenuBarAdaptiveStatusSource: String, Codable, Hashable, Sendable, CaseIterable {
        case events
        case tasks
        case eventsAndTasks

        var title: String {
            switch self {
            case .events: "Events"
            case .tasks: "Tasks"
            case .eventsAndTasks: "Events + Tasks"
            }
        }
    }

    enum MenuBarAdaptiveEmptyBehavior: String, Codable, Hashable, Sendable, CaseIterable {
        case iconOnly
        case clear
        case nextCommitment

        var title: String {
            switch self {
            case .iconOnly: "Icon only"
            case .clear: "Clear"
            case .nextCommitment: "Next commitment"
            }
        }
    }

    enum MenuBarAdaptivePanelContent: String, Codable, Hashable, Sendable, CaseIterable {
        case events
        case tasks
        case eventsAndTasks

        var title: String {
            switch self {
            case .events: "Events only"
            case .tasks: "Tasks only"
            case .eventsAndTasks: "Events + Tasks"
            }
        }
    }

    enum MenuBarIcon: String, Codable, Hashable, Sendable, CaseIterable, Identifiable {
        case buns
        case calendar
        case calendarPlus
        case calendarMinus
        case calendarCircle
        case checkCircle
        case checkSquare
        case checklist
        case list
        case listRectangle
        case textCheck
        case note
        case document
        case clipboard
        case bookmark
        case flag
        case star
        case bell
        case clock
        case timer
        case hourglass
        case alarm
        case stopwatch
        case sunrise
        case sun
        case moon
        case cloud
        case cloudSun
        case umbrella
        case sparkles
        case flame
        case bolt
        case leaf
        case drop
        case heart
        case crown
        case rosette
        case seal
        case paperplane
        case tray
        case archiveBox
        case shippingBox
        case bag
        case cart
        case gift
        case tag
        case pin
        case mapPin
        case location
        case house

        var id: String { rawValue }

        var title: String {
            switch self {
            case .buns: "Buns"
            case .calendar: "Calendar"
            case .calendarPlus: "Calendar Plus"
            case .calendarMinus: "Calendar Minus"
            case .calendarCircle: "Calendar Circle"
            case .checkCircle: "Check Circle"
            case .checkSquare: "Check Square"
            case .checklist: "Checklist"
            case .list: "List"
            case .listRectangle: "List Panel"
            case .textCheck: "Text Check"
            case .note: "Note"
            case .document: "Document"
            case .clipboard: "Clipboard"
            case .bookmark: "Bookmark"
            case .flag: "Flag"
            case .star: "Star"
            case .bell: "Bell"
            case .clock: "Clock"
            case .timer: "Timer"
            case .hourglass: "Hourglass"
            case .alarm: "Alarm"
            case .stopwatch: "Stopwatch"
            case .sunrise: "Sunrise"
            case .sun: "Sun"
            case .moon: "Moon"
            case .cloud: "Cloud"
            case .cloudSun: "Cloud Sun"
            case .umbrella: "Umbrella"
            case .sparkles: "Sparkles"
            case .flame: "Flame"
            case .bolt: "Bolt"
            case .leaf: "Leaf"
            case .drop: "Drop"
            case .heart: "Heart"
            case .crown: "Crown"
            case .rosette: "Rosette"
            case .seal: "Seal"
            case .paperplane: "Paper Plane"
            case .tray: "Tray"
            case .archiveBox: "Archive Box"
            case .shippingBox: "Shipping Box"
            case .bag: "Bag"
            case .cart: "Cart"
            case .gift: "Gift"
            case .tag: "Tag"
            case .pin: "Pin"
            case .mapPin: "Map Pin"
            case .location: "Location"
            case .house: "House"
            }
        }

        var systemImageName: String? {
            switch self {
            case .buns: nil
            case .calendar: "calendar"
            case .calendarPlus: "calendar.badge.plus"
            case .calendarMinus: "calendar.badge.minus"
            case .calendarCircle: "calendar.circle"
            case .checkCircle: "checkmark.circle"
            case .checkSquare: "checkmark.square"
            case .checklist: "checklist"
            case .list: "list.bullet"
            case .listRectangle: "list.bullet.rectangle"
            case .textCheck: "text.badge.checkmark"
            case .note: "note.text"
            case .document: "doc.text"
            case .clipboard: "clipboard"
            case .bookmark: "bookmark"
            case .flag: "flag"
            case .star: "star"
            case .bell: "bell"
            case .clock: "clock"
            case .timer: "timer"
            case .hourglass: "hourglass"
            case .alarm: "alarm"
            case .stopwatch: "stopwatch"
            case .sunrise: "sunrise"
            case .sun: "sun.max"
            case .moon: "moon"
            case .cloud: "cloud"
            case .cloudSun: "cloud.sun"
            case .umbrella: "umbrella"
            case .sparkles: "sparkles"
            case .flame: "flame"
            case .bolt: "bolt"
            case .leaf: "leaf"
            case .drop: "drop"
            case .heart: "heart"
            case .crown: "crown"
            case .rosette: "rosette"
            case .seal: "seal"
            case .paperplane: "paperplane"
            case .tray: "tray"
            case .archiveBox: "archivebox"
            case .shippingBox: "shippingbox"
            case .bag: "bag"
            case .cart: "cart"
            case .gift: "gift"
            case .tag: "tag"
            case .pin: "pin"
            case .mapPin: "mappin"
            case .location: "location"
            case .house: "house"
            }
        }
    }

    static let `default` = AppSettings(
        syncMode: .balanced,
        selectedCalendarIDs: [],
        selectedTaskListIDs: [],
        enableLocalNotifications: false,
        hasCompletedOnboarding: false
    )
}
