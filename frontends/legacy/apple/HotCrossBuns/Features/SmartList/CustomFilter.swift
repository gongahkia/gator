import Foundation

enum DueWindow: String, CaseIterable, Codable, Hashable, Sendable {
    case any
    case none
    case overdue
    case today
    case next7Days

    var title: String {
        switch self {
        case .any: "Any due"
        case .none: "No date"
        case .overdue: "Overdue"
        case .today: "Due today"
        case .next7Days: "Due in 7 days"
        }
    }
}

struct CustomFilterDefinition: Codable, Hashable, Identifiable, Sendable {
    let id: UUID
    var name: String
    var systemImage: String
    var dueWindow: DueWindow
    var includeCompleted: Bool
    var taskListIDs: Set<String>
    var tagsAny: [String]
    // Optional DSL expression. When non-empty, overrides the structured fields above.
    // Parsed by QueryCompiler. Invalid expressions match nothing (never match everything).
    var queryExpression: String?
    // Whether this filter shows up in the menu-bar extra popover (§6.10).
    // Local-only UI affordance — never written back to Google.
    var pinnedToMenuBar: Bool
    var lastUsedAt: Date?
    var useCount: Int

    init(
        id: UUID = UUID(),
        name: String,
        systemImage: String = "line.3.horizontal.decrease.circle",
        dueWindow: DueWindow = .any,
        includeCompleted: Bool = false,
        taskListIDs: Set<String> = [],
        tagsAny: [String] = [],
        queryExpression: String? = nil,
        pinnedToMenuBar: Bool = false,
        lastUsedAt: Date? = nil,
        useCount: Int = 0
    ) {
        self.id = id
        self.name = name
        self.systemImage = systemImage
        self.dueWindow = dueWindow
        self.includeCompleted = includeCompleted
        self.taskListIDs = taskListIDs
        self.tagsAny = tagsAny
        self.queryExpression = queryExpression
        self.pinnedToMenuBar = pinnedToMenuBar
        self.lastUsedAt = lastUsedAt
        self.useCount = max(0, useCount)
    }

    enum CodingKeys: String, CodingKey {
        case id, name, systemImage, dueWindow, includeCompleted, taskListIDs, tagsAny, queryExpression, pinnedToMenuBar
        case lastUsedAt, useCount
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(UUID.self, forKey: .id)
        name = try c.decode(String.self, forKey: .name)
        systemImage = try c.decodeIfPresent(String.self, forKey: .systemImage) ?? "line.3.horizontal.decrease.circle"
        dueWindow = try c.decodeIfPresent(DueWindow.self, forKey: .dueWindow) ?? .any
        includeCompleted = try c.decodeIfPresent(Bool.self, forKey: .includeCompleted) ?? false
        taskListIDs = try c.decodeIfPresent(Set<String>.self, forKey: .taskListIDs) ?? []
        tagsAny = try c.decodeIfPresent([String].self, forKey: .tagsAny) ?? []
        queryExpression = try c.decodeIfPresent(String.self, forKey: .queryExpression)
        pinnedToMenuBar = try c.decodeIfPresent(Bool.self, forKey: .pinnedToMenuBar) ?? false
        lastUsedAt = try c.decodeIfPresent(Date.self, forKey: .lastUsedAt)
        useCount = max(0, try c.decodeIfPresent(Int.self, forKey: .useCount) ?? 0)
    }

    var isUsingQueryDSL: Bool {
        guard let expr = queryExpression?.trimmingCharacters(in: .whitespacesAndNewlines) else { return false }
        return expr.isEmpty == false
    }

    func duplicated(id: UUID = UUID()) -> CustomFilterDefinition {
        CustomFilterDefinition(
            id: id,
            name: hcbCopyName(name),
            systemImage: systemImage,
            dueWindow: dueWindow,
            includeCompleted: includeCompleted,
            taskListIDs: taskListIDs,
            tagsAny: tagsAny,
            queryExpression: queryExpression,
            pinnedToMenuBar: pinnedToMenuBar,
            lastUsedAt: nil,
            useCount: 0
        )
    }

    // Per-task match. Compiles DSL per call; callers on hot paths should use
    // `filter(_:now:calendar:taskLists:)` which compiles once.
    func matches(
        _ task: TaskMirror,
        now: Date = Date(),
        calendar: Calendar = .current,
        taskLists: [TaskListMirror] = []
    ) -> Bool {
        guard task.isDeleted == false else { return false }
        if isUsingQueryDSL, let expr = queryExpression {
            switch QueryCompiler.compile(expr) {
            case .success(let q):
                return q.matches(task, context: QueryContext(now: now, calendar: calendar, taskLists: taskLists))
            case .failure:
                return false
            }
        }
        return matchesStructured(task, now: now, calendar: calendar)
    }

    // Hot path: compile DSL once, evaluate across all tasks.
    // On compile failure the filter yields an empty result so users see a clear
    // empty state rather than an accidentally-unbounded list.
    func filter(
        _ tasks: [TaskMirror],
        now: Date = Date(),
        calendar: Calendar = .current,
        taskLists: [TaskListMirror] = []
    ) -> [TaskMirror] {
        if isUsingQueryDSL, let expr = queryExpression {
            switch QueryCompiler.compile(expr) {
            case .success(let q):
                let ctx = QueryContext(now: now, calendar: calendar, taskLists: taskLists)
                return tasks.filter { $0.isDeleted == false && q.matches($0, context: ctx) }
            case .failure:
                return []
            }
        }
        return tasks.filter { matchesStructured($0, now: now, calendar: calendar) }
    }

    func compiledQuery() -> Result<CompiledQuery, QueryCompileError>? {
        guard isUsingQueryDSL, let expr = queryExpression else { return nil }
        return QueryCompiler.compile(expr)
    }

    private func matchesStructured(
        _ task: TaskMirror,
        now: Date,
        calendar: Calendar
    ) -> Bool {
        guard task.isDeleted == false else { return false }
        if includeCompleted == false, task.isCompleted { return false }

        if taskListIDs.isEmpty == false, taskListIDs.contains(task.taskListID) == false {
            return false
        }

        if tagsAny.isEmpty == false {
            let taskTags = Set(TagExtractor.tags(in: task.title).map { $0.lowercased() })
            let targetTags = Set(tagsAny.map { $0.lowercased() })
            if taskTags.isDisjoint(with: targetTags) { return false }
        }

        let startOfToday = calendar.startOfDay(for: now)
        switch dueWindow {
        case .any: return true
        case .none: return task.dueDate == nil
        case .overdue:
            guard let due = task.dueDate else { return false }
            return calendar.startOfDay(for: due) < startOfToday
        case .today:
            guard let due = task.dueDate else { return false }
            return calendar.startOfDay(for: due) == startOfToday
        case .next7Days:
            guard let due = task.dueDate else { return false }
            guard let horizon = calendar.date(byAdding: .day, value: 7, to: startOfToday) else { return false }
            let startOfDue = calendar.startOfDay(for: due)
            return startOfDue >= startOfToday && startOfDue < horizon
        }
    }
}

func hcbCopyName(_ name: String) -> String {
    let trimmed = name.trimmingCharacters(in: .whitespacesAndNewlines)
    return trimmed.isEmpty ? "Copy" : "\(trimmed) Copy"
}
