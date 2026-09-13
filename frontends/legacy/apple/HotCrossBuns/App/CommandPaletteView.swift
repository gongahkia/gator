import SwiftUI

struct CommandPaletteCommand: Identifiable {
    let id: String
    let title: String
    let subtitle: String
    let symbol: String
    let shortcut: String
    let keywords: [String]
    let action: () -> Void
}

// Merged palette — runs commands *and* jumps to tasks / notes / events /
// lists / calendars / saved filters in a single surface. Previously split
// into CommandPaletteView (do) and QuickSwitcherView (go) under ⌘P / ⌘O;
// unified here behind ⌘P so there's one muscle-memory entry point. Free-
// text matches rank commands and entities together by fuzzy score. Field
// operators (title:, tag:, list:, kind:, has:, attendee:, calendar:,
// /regex/) suppress commands — when the user is clearly searching
// entities, don't clutter the result set with command hits.
//
// Notes vs tasks: a note is a TaskMirror with dueDate == nil. Kind labels
// and icons discriminate in the row, and `kind:note` / `kind:task` in the
// DSL filters to one or the other.

enum QuickSwitcherEntity: Hashable, Identifiable, Sendable {
    case task(TaskMirror)
    case event(CalendarEventMirror)
    case taskList(TaskListMirror)
    case calendar(CalendarListMirror)
    case customFilter(CustomFilterDefinition)

    var id: String {
        switch self {
        case .task(let t): return "task-\(t.id)"
        case .event(let e): return "event-\(e.id)"
        case .taskList(let l): return "list-\(l.id)"
        case .calendar(let c): return "cal-\(c.id)"
        case .customFilter(let f): return "cf-\(f.id.uuidString)"
        }
    }

    var label: String {
        switch self {
        case .task(let t): return TagExtractor.stripped(from: t.title)
        case .event(let e): return e.summary
        case .taskList(let l): return l.title
        case .calendar(let c): return c.summary
        case .customFilter(let f): return f.name
        }
    }

    var keywords: [String] {
        switch self {
        case .task(let t):
            return TagExtractor.tags(in: t.title) + [t.notes]
        case .event(let e):
            return [e.details, e.location]
        case .taskList, .calendar: return []
        case .customFilter(let f): return f.queryExpression.map { [$0] } ?? []
        }
    }

    // Task vs Note is a display-only distinction (both are TaskMirror on
    // Google's side). Standalone undated tasks read as notes; dated tasks
    // with note bodies stay labelled as tasks while remaining searchable.
    fileprivate var kindLabel: String {
        switch self {
        case .task(let t): return t.dueDate == nil ? "Note" : "Task"
        case .event: return "Event"
        case .taskList: return "List"
        case .calendar: return "Calendar"
        case .customFilter: return "Filter"
        }
    }

    fileprivate var symbol: String {
        switch self {
        case .task(let t):
            if t.dueDate == nil { return "note.text" }
            return t.isCompleted ? "checkmark.circle.fill" : "circle"
        case .event: return "calendar"
        case .taskList: return "checklist"
        case .calendar: return "calendar.badge.clock"
        case .customFilter(let f): return f.systemImage
        }
    }
}

struct CommandPaletteEntitySnapshot: Sendable {
    let key: String
    let tasks: [TaskMirror]
    let events: [CalendarEventMirror]
    let taskLists: [TaskListMirror]
    let calendars: [CalendarListMirror]
    let customFilters: [CustomFilterDefinition]

    init(
        key: String,
        tasks: [TaskMirror],
        events: [CalendarEventMirror],
        taskLists: [TaskListMirror],
        calendars: [CalendarListMirror],
        customFilters: [CustomFilterDefinition]
    ) {
        self.key = key
        self.tasks = tasks
        self.events = events
        self.taskLists = taskLists
        self.calendars = calendars
        self.customFilters = customFilters
    }

    @MainActor
    init(
        taskStore: TaskStore,
        calendarStore: CalendarStore,
        commandPaletteStore: CommandPaletteStore,
        key: String,
        includeEntities: Bool
    ) {
        self.init(
            key: key,
            tasks: includeEntities ? taskStore.tasks : [],
            events: includeEntities ? calendarStore.events : [],
            taskLists: commandPaletteStore.taskLists,
            calendars: commandPaletteStore.calendars,
            customFilters: commandPaletteStore.customFilters
        )
    }
}

struct CommandPaletteEntitySearchResult: Sendable {
    let snapshotKey: String
    let query: String
    let entities: [QuickSwitcherEntity]
}

private struct CommandPaletteEntityIndex: Sendable {
    let snapshotKey: String
    let entities: [QuickSwitcherEntity]
    let lowercaseSearchBlobs: [String]
    let calendars: [CalendarListMirror]
    let taskLists: [TaskListMirror]
}

actor CommandPaletteEntityCache {
    private var index: CommandPaletteEntityIndex?

    func prepare(snapshot: CommandPaletteEntitySnapshot) async {
        _ = try? await index(for: snapshot)
    }

    func search(snapshot: CommandPaletteEntitySnapshot, query: String) async throws -> CommandPaletteEntitySearchResult {
        let index = try await index(for: snapshot)
        let entities = try Self.searchEntities(in: index, query: query)
        return CommandPaletteEntitySearchResult(snapshotKey: index.snapshotKey, query: query, entities: entities)
    }

    private func index(for snapshot: CommandPaletteEntitySnapshot) async throws -> CommandPaletteEntityIndex {
        if let index, index.snapshotKey == snapshot.key {
            return index
        }

        let next = try Self.buildIndex(snapshot: snapshot)
        index = next
        return next
    }

    private static func buildIndex(snapshot: CommandPaletteEntitySnapshot) throws -> CommandPaletteEntityIndex {
        var entities: [QuickSwitcherEntity] = []
        entities.reserveCapacity(
            snapshot.tasks.count
                + snapshot.events.count
                + snapshot.taskLists.count
                + snapshot.calendars.count
                + snapshot.customFilters.count
        )

        for (offset, task) in snapshot.tasks.enumerated() {
            if offset.isMultiple(of: 256) { try Task.checkCancellation() }
            if task.isDeleted == false { entities.append(.task(task)) }
        }
        for (offset, event) in snapshot.events.enumerated() {
            if offset.isMultiple(of: 256) { try Task.checkCancellation() }
            if event.status != .cancelled { entities.append(.event(event)) }
        }
        for taskList in snapshot.taskLists { entities.append(.taskList(taskList)) }
        for calendar in snapshot.calendars { entities.append(.calendar(calendar)) }
        for filter in snapshot.customFilters { entities.append(.customFilter(filter)) }

        var lowercaseSearchBlobs: [String] = []
        lowercaseSearchBlobs.reserveCapacity(entities.count)
        for (offset, entity) in entities.enumerated() {
            if offset.isMultiple(of: 256) { try Task.checkCancellation() }
            lowercaseSearchBlobs.append(
                ([entity.label] + entity.keywords)
                    .joined(separator: "\n")
                    .lowercased()
            )
        }

        return CommandPaletteEntityIndex(
            snapshotKey: snapshot.key,
            entities: entities,
            lowercaseSearchBlobs: lowercaseSearchBlobs,
            calendars: snapshot.calendars,
            taskLists: snapshot.taskLists
        )
    }

    private static func searchEntities(in index: CommandPaletteEntityIndex, query: String) throws -> [QuickSwitcherEntity] {
        let q = query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard q.isEmpty == false else { return [] }
        let parsed = AdvancedSearchParser.parse(q)

        if let pattern = parsed.regex {
            guard let compiled = try? NSRegularExpression(pattern: pattern, options: [.caseInsensitive]) else {
                return []
            }
            var matches: [QuickSwitcherEntity] = []
            matches.reserveCapacity(30)
            for entity in index.entities {
                if matches.count >= 30 { break }
                if matches.count.isMultiple(of: 16) { try Task.checkCancellation() }
                if AdvancedSearchMatcher.regexMatches(entity, compiled: compiled) {
                    matches.append(entity)
                }
            }
            return matches
        }

        let freeText = parsed.freeText.trimmingCharacters(in: .whitespacesAndNewlines)
        let hasStructuredFilters = CommandPaletteSearchRules.hasStructuredFilters(parsed)

        let entitiesFiltered: [QuickSwitcherEntity]
        if hasStructuredFilters {
            var hits: [QuickSwitcherEntity] = []
            hits.reserveCapacity(min(index.entities.count, 500))
            for (offset, entity) in index.entities.enumerated() {
                if offset.isMultiple(of: 256) { try Task.checkCancellation() }
                if AdvancedSearchMatcher.matches(
                    entity,
                    query: parsed,
                    calendars: index.calendars,
                    taskLists: index.taskLists
                ) {
                    hits.append(entity)
                }
            }
            entitiesFiltered = hits
        } else if freeText.count >= 2 {
            let lowered = freeText.lowercased()
            var hits: [QuickSwitcherEntity] = []
            hits.reserveCapacity(min(index.entities.count, 500))
            for (idx, lowerBlob) in index.lowercaseSearchBlobs.enumerated() {
                if idx.isMultiple(of: 256) { try Task.checkCancellation() }
                if lowerBlob.contains(lowered) {
                    hits.append(index.entities[idx])
                    if hits.count >= 500 { break }
                }
            }
            entitiesFiltered = hits
        } else {
            entitiesFiltered = index.entities.filter {
                switch $0 {
                case .taskList, .calendar, .customFilter: return true
                case .task, .event: return false
                }
            }
        }

        if freeText.isEmpty {
            return Array(
                entitiesFiltered
                    .sorted { $0.label.localizedCaseInsensitiveCompare($1.label) == .orderedAscending }
                    .prefix(30)
            )
        }

        let ranked = FuzzySearcher.rank(
            entitiesFiltered,
            query: freeText,
            labelForItem: { $0.label },
            keywordsForItem: { $0.keywords },
            limit: 38
        )
        return ranked.map { $0.item }
    }
}

// Union type backing the merged ranked list. Commands and entities share
// a row renderer + numeric-shortcut handling so the UX is uniform.
fileprivate enum PaletteItem: Identifiable {
    case command(CommandPaletteCommand)
    case entity(QuickSwitcherEntity)

    var id: String {
        switch self {
        case .command(let c): return "cmd-\(c.id)"
        case .entity(let e): return "ent-\(e.id)"
        }
    }

}

private enum CommandPaletteSearchRules {
    static func hasStructuredFilters(_ q: AdvancedSearchQuery) -> Bool {
        q != .empty && (
            q.regex != nil
            || q.titleContains.isEmpty == false
            || q.tagsAll.isEmpty == false
            || q.listMatch != nil
            || q.calendarMatch != nil
            || q.attendeeMatch != nil
            || q.kind != nil
            || q.requireNotes
            || q.requireLocation
            || q.requireDue
            || q.requireCompleted
            || q.requireOverdue
        )
    }

    // True when the parsed query carries only free text (no field operators
    // / bare keywords / regex). Drives the "should I include commands?"
    // decision — entity-filter-heavy queries suppress commands so the list
    // isn't noisy.
    static func hasFreeTextOnly(_ q: AdvancedSearchQuery) -> Bool {
        q.regex == nil
            && q.titleContains.isEmpty
            && q.tagsAll.isEmpty
            && q.listMatch == nil
            && q.calendarMatch == nil
            && q.attendeeMatch == nil
            && q.kind == nil
            && q.requireNotes == false
            && q.requireLocation == false
            && q.requireDue == false
            && q.requireCompleted == false
            && q.requireOverdue == false
    }
}

struct CommandPaletteView: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(AppModel.self) private var model
    private var taskStore: TaskStore { model.taskStore }
    private var calendarStore: CalendarStore { model.calendarStore }
    private var commandPaletteStore: CommandPaletteStore { model.commandPaletteStore }
    @AppStorage("hcb.commandPalette.recentCommandIDs") private var recentCommandIDsJSON: String = "[]"
    @State private var query = ""
    // Heavy entity results (tasks/events/notes) intentionally lag behind the
    // live query. Command results are cheap and update immediately so local
    // actions such as "New Task" feel instant.
    @State private var entityResults: [PaletteItem] = []
    @State private var entityResultsQuery = ""
    @State private var entityResultsSnapshotKey = ""
    @FocusState private var isSearchFocused: Bool

    let commands: [CommandPaletteCommand]
    let entityCache: CommandPaletteEntityCache
    let onSelectEntity: (QuickSwitcherEntity) -> Void
    var onClose: (() -> Void)?

    var body: some View {
        VStack(spacing: 0) {
            searchHeader

            if trimmedQuery.isEmpty == false || recentCommands.isEmpty == false {
                Divider().overlay(.secondary.opacity(0.25))

                ScrollView {
                    LazyVStack(spacing: 0) {
                        if trimmedQuery.isEmpty {
                            recentCommandsSection
                        } else if rankedItems.isEmpty, isEntitySearchPending {
                            entitySearchPendingState
                        } else if rankedItems.isEmpty {
                            emptyState
                        } else {
                            ForEach(Array(rankedItems.enumerated()), id: \.element.id) { index, item in
                                rowButton(item: item, index: index)
                            }
                        }
                    }
                    .hcbScaledPadding(.vertical, 12)
                }
            }
        }
        .onAppear {
            // Honour any deep-link-staged query: hotcrossbuns://search?q=… sets
            // model.pendingPaletteQuery before toggling the palette open. Clear
            // after consuming so a subsequent manual open starts blank.
            if let staged = commandPaletteStore.pendingPaletteQuery, staged.isEmpty == false {
                query = staged
                model.pendingPaletteQuery = nil
            }
            isSearchFocused = true
            Task { @MainActor in
                await Task.yield()
                isSearchFocused = true
            }
        }
        .task(id: fallbackPrewarmTaskID) {
            guard commandPaletteStore.usesDatabaseEntitySearch == false else { return }
            await entityCache.prepare(snapshot: CommandPaletteEntitySnapshot(
                taskStore: taskStore,
                calendarStore: calendarStore,
                commandPaletteStore: commandPaletteStore,
                key: containerSnapshotKey,
                includeEntities: false
            ))
        }
        // Entity-search debounce: each keystroke restarts this task; commands
        // do not wait for it because they are ranked directly from `query`.
        .task(id: entitySearchTaskID) {
            let liveQuery = trimmedQuery
            let snapshotKeyAtStart = snapshotKey
            guard liveQuery.isEmpty == false else {
                entityResultsQuery = ""
                entityResultsSnapshotKey = snapshotKeyAtStart
                entityResults = []
                return
            }
            if AdvancedSearchParser.parse(liveQuery).regex != nil {
                entityResultsQuery = liveQuery
                entityResultsSnapshotKey = snapshotKeyAtStart
                entityResults = []
                return
            }
            do {
                try await Task.sleep(for: .milliseconds(90))
                let entities: [QuickSwitcherEntity]
                let resultSnapshotKey: String
                if let databaseEntities = try await model.searchEntities(query: liveQuery, limit: 38) {
                    entities = databaseEntities
                    resultSnapshotKey = snapshotKeyAtStart
                } else {
                    let snapshot = CommandPaletteEntitySnapshot(
                        taskStore: taskStore,
                        calendarStore: calendarStore,
                        commandPaletteStore: commandPaletteStore,
                        key: containerSnapshotKey(for: snapshotKeyAtStart),
                        includeEntities: false
                    )
                    let result = try await entityCache.search(snapshot: snapshot, query: liveQuery)
                    entities = result.entities
                    resultSnapshotKey = snapshotKeyAtStart
                }
                try Task.checkCancellation()
                guard resultSnapshotKey == snapshotKey, liveQuery == trimmedQuery else { return }
                entityResultsQuery = liveQuery
                entityResultsSnapshotKey = resultSnapshotKey
                entityResults = entities.map { PaletteItem.entity($0) }
            } catch {
                // task cancelled by next keystroke — drop, the next one runs.
            }
        }
        .onSubmit(of: .text, executeFirstMatch)
        .hcbScaledFrame(
            minWidth: 520,
            idealWidth: 560,
            maxWidth: 620,
            minHeight: paletteHeight.min,
            idealHeight: paletteHeight.ideal
        )
        .background {
            RoundedRectangle(cornerRadius: 12)
                .fill(paletteBackgroundStyle)
        }
        .clipShape(RoundedRectangle(cornerRadius: 12))
        .presentationBackground(.clear)
        .hcbScaledPadding(14)
        .onExitCommand(perform: closePalette)
    }

    private var trimmedQuery: String {
        query.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private var recentCommandIDs: [String] {
        guard let data = recentCommandIDsJSON.data(using: .utf8),
              let ids = try? JSONDecoder().decode([String].self, from: data) else {
            return []
        }
        return ids
    }

    private var recentCommands: [CommandPaletteCommand] {
        let byID = Dictionary(uniqueKeysWithValues: commands.map { ($0.id, $0) })
        return recentCommandIDs.compactMap { byID[$0] }
    }

    private var paletteHeight: (min: CGFloat, ideal: CGFloat) {
        if trimmedQuery.isEmpty == false {
            return (360, 420)
        }
        if recentCommands.isEmpty == false {
            return (220, 300)
        }
        return (54, 54)
    }

    // Cheap fingerprint for the entity universe. The palette store revision
    // advances with task/event/list/calendar/filter changes without tying
    // palette cache churn to unrelated global app state.
    private var snapshotKey: String {
        "\(commandPaletteStore.entityRevision)|\(commandPaletteStore.customFilters.count)"
    }

    private var fallbackPrewarmTaskID: String {
        "\(containerSnapshotKey)|\(commandPaletteStore.usesDatabaseEntitySearch)"
    }

    private var entitySearchTaskID: String {
        "\(snapshotKey)|\(commandPaletteStore.usesDatabaseEntitySearch)|\(trimmedQuery)"
    }

    private var containerSnapshotKey: String {
        containerSnapshotKey(for: snapshotKey)
    }

    private func containerSnapshotKey(for key: String) -> String {
        "containers|\(key)"
    }

    private var paletteBackgroundStyle: AnyShapeStyle {
        if commandPaletteStore.performanceMode == .rich {
            AnyShapeStyle(.ultraThinMaterial)
        } else {
            AnyShapeStyle(AppColor.cardSurface)
        }
    }

    // Merged ranked list. Behaviour:
    //   - Commands rank synchronously from the live query.
    //   - Entities rank from the debounced query and are only shown while
    //     still current for the visible query.
    //   - Regex mode (/…/): entity-only regex filter, no commands.
    //   - Field-operator query with no free-text: entity-only structured
    //     filter, alphabetic, no commands. Keeps power-user DSL output stable.
    //   - Free-text (with or without operators): rank commands + entities
    //     together by fuzzy score; commands bubble up when the title matches.
    private var rankedItems: [PaletteItem] {
        let q = trimmedQuery
        guard q.isEmpty == false else { return [] }
        let commands = commandItems(forQuery: q)
        guard entityResultsQuery == q, entityResultsSnapshotKey == snapshotKey else { return commands }
        return commands + entityResults
    }

    private var isEntitySearchPending: Bool {
        let q = trimmedQuery
        return q.isEmpty == false && (entityResultsQuery != q || entityResultsSnapshotKey != snapshotKey)
    }

    private func commandItems(forQuery q: String) -> [PaletteItem] {
        guard q.isEmpty == false else { return [] }
        let parsed = AdvancedSearchParser.parse(q)
        let freeText = parsed.freeText.trimmingCharacters(in: .whitespacesAndNewlines)

        guard parsed.regex == nil,
              (CommandPaletteSearchRules.hasStructuredFilters(parsed) == false || CommandPaletteSearchRules.hasFreeTextOnly(parsed)) else {
            return []
        }

        guard freeText.isEmpty == false else { return [] }
        return FuzzySearcher.rank(
            commands,
            query: freeText,
            labelForItem: { $0.title },
            keywordsForItem: { [$0.subtitle] + $0.keywords },
            limit: 12
        )
        .map { PaletteItem.command($0.item) }
    }

    private func executeFirstMatch() {
        if trimmedQuery.isEmpty, let first = recentCommands.first {
            select(.command(first))
            return
        }
        // Commands are local and should win immediately when they match the
        // live query. If entity results are still warming, finish that search
        // asynchronously instead of rebuilding the index on the main actor.
        if let first = commandItems(forQuery: trimmedQuery).first {
            select(first)
            return
        }
        if entityResultsQuery == trimmedQuery,
           entityResultsSnapshotKey == snapshotKey,
           let first = entityResults.first {
            select(first)
            return
        }

        let liveQuery = trimmedQuery
        let snapshotKeyAtStart = snapshotKey
        let isRegexQuery = AdvancedSearchParser.parse(liveQuery).regex != nil
        Task {
            do {
                let entities: [QuickSwitcherEntity]
                let resultSnapshotKey: String
                if isRegexQuery {
                    let snapshot = await MainActor.run {
                        CommandPaletteEntitySnapshot(
                            taskStore: taskStore,
                            calendarStore: calendarStore,
                            commandPaletteStore: commandPaletteStore,
                            key: snapshotKeyAtStart,
                            includeEntities: true
                        )
                    }
                    let result = try await entityCache.search(snapshot: snapshot, query: liveQuery)
                    entities = result.entities
                    resultSnapshotKey = snapshotKeyAtStart
                } else if let databaseEntities = try await model.searchEntities(query: liveQuery, limit: 38) {
                    entities = databaseEntities
                    resultSnapshotKey = snapshotKeyAtStart
                } else {
                    let snapshot = await MainActor.run {
                        CommandPaletteEntitySnapshot(
                            taskStore: taskStore,
                            calendarStore: calendarStore,
                            commandPaletteStore: commandPaletteStore,
                            key: containerSnapshotKey(for: snapshotKeyAtStart),
                            includeEntities: false
                        )
                    }
                    let result = try await entityCache.search(snapshot: snapshot, query: liveQuery)
                    entities = result.entities
                    resultSnapshotKey = snapshotKeyAtStart
                }
                try Task.checkCancellation()
                guard let first = entities.first else { return }
                await MainActor.run {
                    guard trimmedQuery == liveQuery, snapshotKey == resultSnapshotKey else { return }
                    select(.entity(first))
                }
            } catch {
                // Search was cancelled or superseded.
            }
        }
    }

    private func select(_ item: PaletteItem) {
        closePalette()
        // Defer execution so panel/sheet teardown doesn't fight downstream
        // navigation / sheet presentation.
        Task { @MainActor in
            switch item {
            case .command(let command):
                recordRecentCommand(id: command.id)
                command.action()
            case .entity(let entity): onSelectEntity(entity)
            }
        }
    }

    private func closePalette() {
        if let onClose {
            onClose()
        } else {
            dismiss()
        }
    }

    private func recordRecentCommand(id: String) {
        var ids = recentCommandIDs.filter { $0 != id }
        ids.insert(id, at: 0)
        ids = Array(ids.prefix(10))
        guard let data = try? JSONEncoder().encode(ids),
              let json = String(data: data, encoding: .utf8) else {
            return
        }
        recentCommandIDsJSON = json
    }

    private var searchHeader: some View {
        HStack(spacing: 8) {
            Image(systemName: "command")
                .hcbFont(.subheadline, weight: .medium)
                .foregroundStyle(.secondary)

            TextField("Run a command or search — New Task, refresh, tag:deep, kind:note …", text: $query)
                .textFieldStyle(.plain)
                .focused($isSearchFocused)
                .hcbFont(.body)
                .onSubmit(executeFirstMatch)

            if query.isEmpty {
                Text("⌘P")
                    .hcbFont(.caption, weight: .semibold)
                    .foregroundStyle(.secondary)
                    .hcbScaledPadding(.horizontal, 6)
                    .hcbScaledPadding(.vertical, 2)
                    .background(
                        RoundedRectangle(cornerRadius: 5)
                            .fill(.thinMaterial)
                    )
            }
        }
        .hcbScaledPadding(.horizontal, 12)
        .hcbScaledPadding(.vertical, 9)
    }

    @ViewBuilder
    private func rowButton(item: PaletteItem, index: Int) -> some View {
        let button = Button { select(item) } label: {
            HStack(spacing: 6) {
                switch item {
                case .command(let c): CommandPaletteRow(command: c)
                case .entity(let e): EntityRow(entity: e, subtitle: entitySubtitle(e))
                }
                if let label = numericShortcutLabel(for: index) {
                    Text(label)
                        .font(.system(.caption2, design: .monospaced, weight: .medium))
                        .foregroundStyle(.secondary)
                        .hcbScaledPadding(.horizontal, 5)
                        .hcbScaledPadding(.vertical, 1)
                        .background(
                            RoundedRectangle(cornerRadius: 4)
                                .fill(.quaternary.opacity(0.5))
                        )
                }
            }
        }
        .buttonStyle(.plain)
        .hcbScaledPadding(.horizontal, 20)
        .hcbScaledPadding(.vertical, 8)

        if index < 10 {
            button.keyboardShortcut(numericKeyEquivalent(for: index), modifiers: [.command])
        } else {
            button
        }
    }

    private func numericKeyEquivalent(for index: Int) -> KeyEquivalent {
        let digit: Character = index == 9 ? "0" : Character(String(index + 1))
        return KeyEquivalent(digit)
    }

    private func numericShortcutLabel(for index: Int) -> String? {
        guard index < 10 else { return nil }
        let digit = index == 9 ? "0" : "\(index + 1)"
        return "⌘\(digit)"
    }

    private func entitySubtitle(_ entity: QuickSwitcherEntity) -> String {
        switch entity {
        case .task(let t):
            let list = model.taskListTitle(for: t.taskListID, fallback: "—")
            if let due = t.dueDate {
                return "\(list) · due \(due.formatted(.dateTime.month(.abbreviated).day()))"
            }
            return list
        case .event(let e):
            let cal = model.calendarTitle(for: e.calendarID, fallback: "—")
            if e.isAllDay {
                return "\(cal) · \(e.startDate.formatted(.dateTime.month(.abbreviated).day()))"
            }
            return "\(cal) · \(e.startDate.formatted(.dateTime.month(.abbreviated).day().hour().minute()))"
        case .taskList(let l):
            let count = model.openTaskCount(forTaskListID: l.id)
            return "\(count) open"
        case .calendar(let c):
            return c.colorHex
        case .customFilter(let f):
            return f.isUsingQueryDSL ? "DSL · \(f.queryExpression ?? "")" : f.dueWindow.title
        }
    }

    private var emptyState: some View {
        VStack(spacing: 10) {
            Image(systemName: "magnifyingglass")
                .hcbFont(.title2, weight: .semibold)
                .foregroundStyle(.secondary)
            Text(AdvancedSearchParser.parse(trimmedQuery).regex == nil ? "Nothing matches \"\(query)\"" : "Press Return to run regex search")
                .hcbFont(.headline)
                .foregroundStyle(.secondary)
            Text(AdvancedSearchParser.parse(trimmedQuery).regex == nil ? "Try a bare word, or `kind:note`, `tag:deep`, `/regex/`." : "Live results stay off for regex to keep the palette responsive.")
                .hcbFont(.footnote)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, minHeight: 180)
    }

    private var entitySearchPendingState: some View {
        VStack(spacing: 10) {
            ProgressView()
                .controlSize(.small)
            Text("Searching tasks, events, and notes…")
                .hcbFont(.headline)
                .foregroundStyle(.secondary)
            Text("Commands are available immediately.")
                .hcbFont(.footnote)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, minHeight: 180)
    }

    @ViewBuilder
    private var recentCommandsSection: some View {
        if recentCommands.isEmpty {
            VStack(spacing: 10) {
                Image(systemName: "command")
                    .hcbFont(.title2, weight: .semibold)
                    .foregroundStyle(.secondary)
                Text("No recent commands yet")
                    .hcbFont(.headline)
                    .foregroundStyle(.secondary)
                Text("Run a command once and it will stay here for quick repeat access.")
                    .hcbFont(.footnote)
                    .foregroundStyle(.secondary)
            }
            .frame(maxWidth: .infinity, minHeight: 180)
        } else {
            VStack(alignment: .leading, spacing: 8) {
                Text("Recent Commands")
                    .hcbFont(.caption, weight: .semibold)
                    .foregroundStyle(.secondary)
                    .textCase(.uppercase)
                    .hcbScaledPadding(.horizontal, 20)
                    .hcbScaledPadding(.top, 4)
                ForEach(Array(recentCommands.enumerated()), id: \.element.id) { index, command in
                    rowButton(item: .command(command), index: index)
                }
            }
        }
    }
}

private struct CommandPaletteRow: View {
    let command: CommandPaletteCommand

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: command.symbol)
                .hcbFont(.headline)
                .foregroundStyle(.secondary)
                .hcbScaledFrame(width: 24)

            VStack(alignment: .leading, spacing: 2) {
                Text(command.title)
                    .hcbFont(.body, weight: .semibold)
                Text(command.subtitle)
                    .hcbFont(.subheadline)
                    .foregroundStyle(.secondary)
            }

            Spacer(minLength: 12)

            Text("Command")
                .hcbFont(.caption2, weight: .medium)
                .foregroundStyle(.secondary)
                .hcbScaledPadding(.horizontal, 6)
                .hcbScaledPadding(.vertical, 2)
                .background(
                    Capsule().fill(.quaternary.opacity(0.4))
                )

            if command.shortcut.isEmpty == false {
                Text(formattedShortcut)
                    .font(.system(.subheadline, design: .monospaced, weight: .medium))
                    .foregroundStyle(.secondary)
            }
        }
        .contentShape(Rectangle())
        .hcbScaledPadding(.vertical, 2)
    }

    private var formattedShortcut: String {
        command.shortcut
            .replacingOccurrences(of: "Cmd+", with: "⌘")
            .replacingOccurrences(of: "Shift+", with: "⇧")
            .replacingOccurrences(of: "Option+", with: "⌥")
            .replacingOccurrences(of: "+", with: "")
    }
}

private struct EntityRow: View {
    let entity: QuickSwitcherEntity
    let subtitle: String

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: entity.symbol)
                .hcbFont(.headline)
                .foregroundStyle(tint)
                .hcbScaledFrame(width: 24)

            VStack(alignment: .leading, spacing: 2) {
                Text(entity.label.isEmpty ? "(untitled)" : entity.label)
                    .hcbFont(.body, weight: .semibold)
                    .lineLimit(1)
                Text(subtitle)
                    .hcbFont(.subheadline)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }

            Spacer(minLength: 12)

            Text(entity.kindLabel)
                .hcbFont(.caption2, weight: .medium)
                .foregroundStyle(.secondary)
                .hcbScaledPadding(.horizontal, 6)
                .hcbScaledPadding(.vertical, 2)
                .background(
                    Capsule().fill(.quaternary.opacity(0.4))
                )
        }
        .contentShape(Rectangle())
        .hcbScaledPadding(.vertical, 2)
    }

    private var tint: Color {
        switch entity {
        case .task(let t):
            if t.dueDate == nil { return AppColor.ink }
            return t.isCompleted ? AppColor.moss : AppColor.ember
        case .event: return AppColor.blue
        case .taskList: return AppColor.ink
        case .calendar: return AppColor.blue
        case .customFilter: return AppColor.ember
        }
    }
}
