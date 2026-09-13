import Foundation

struct HCBToolDefinition {
    var name: String
    var description: String
    var inputSchema: [String: Any]
}

enum HCBToolError: LocalizedError, Equatable {
    case unknownTool(String)
    case invalidArguments(String)
    case permissionDenied(String)
    case confirmationRequired(String, String?)
    case confirmationMismatch
    case notFound(String)
    case mutationFailed(String)

    var errorDescription: String? {
        switch self {
        case .unknownTool(let name):
            "Unknown MCP tool '\(name)'."
        case .invalidArguments(let message):
            message
        case .permissionDenied(let message):
            message
        case .confirmationRequired(let message, _):
            message
        case .confirmationMismatch:
            "Confirmation id is missing, expired, or does not match these arguments."
        case .notFound(let message):
            message
        case .mutationFailed(let message):
            message
        }
    }

    var confirmationId: String? {
        if case .confirmationRequired(_, let id) = self { return id }
        return nil
    }
}

@MainActor
final class HCBToolService {
    private struct PendingConfirmation {
        var toolName: String
        var canonicalArguments: String
        var expiresAt: Date
    }

    private let model: AppModel
    private var confirmations: [String: PendingConfirmation] = [:]
    private let confirmationTTL: TimeInterval = 5 * 60

    init(model: AppModel) {
        self.model = model
    }

    nonisolated static var toolDefinitions: [HCBToolDefinition] {
        [
            readTool("hcb_search", "Search tasks, notes, events, lists, and calendars.", properties: [
                "query": stringSchema("Advanced search or fuzzy query."),
                "scope": enumSchema(["all", "tasks", "notes", "events", "lists", "calendars"]),
                "limit": integerSchema("Maximum result count.")
            ], required: ["query"]),
            readTool("hcb_today", "Read today's due tasks and scheduled events.", properties: [:]),
            readTool("hcb_week", "Read the agenda for a seven-day window.", properties: [
                "startDate": stringSchema("Optional ISO-8601 date or date-time. Defaults to today.")
            ]),
            readTool("hcb_get_task", "Read one task or note by id.", properties: [
                "id": stringSchema("Task id.")
            ], required: ["id"]),
            readTool("hcb_get_event", "Read one event by id.", properties: [
                "id": stringSchema("Event id.")
            ], required: ["id"]),
            readTool("hcb_list_task_lists", "List available Google Tasks lists.", properties: [:]),
            readTool("hcb_list_calendars", "List available Google calendars.", properties: [:]),
            writeTool("hcb_create_task", "Create a dated task.", properties: [
                "title": stringSchema("Task title."),
                "notes": stringSchema("Optional task notes."),
                "dueDate": stringSchema("Optional ISO-8601 due date."),
                "taskListID": stringSchema("Optional task list id or title."),
                "dryRun": booleanSchema("Preview without applying."),
                "confirmationId": stringSchema("Confirmation id returned by a dry-run.")
            ], required: ["title"]),
            writeTool("hcb_create_note", "Create an undated note.", properties: [
                "title": stringSchema("Note title."),
                "notes": stringSchema("Optional note body."),
                "taskListID": stringSchema("Optional task list id or title."),
                "dryRun": booleanSchema("Preview without applying."),
                "confirmationId": stringSchema("Confirmation id returned by a dry-run.")
            ], required: ["title"]),
            writeTool("hcb_create_event", "Create a calendar event.", properties: [
                "title": stringSchema("Event title."),
                "details": stringSchema("Optional event details."),
                "startDate": stringSchema("ISO-8601 start date or date-time."),
                "endDate": stringSchema("Optional ISO-8601 end date or date-time."),
                "isAllDay": booleanSchema("Whether this is an all-day event."),
                "location": stringSchema("Optional location."),
                "calendarID": stringSchema("Optional calendar id or summary."),
                "dryRun": booleanSchema("Preview without applying."),
                "confirmationId": stringSchema("Confirmation id returned by a dry-run.")
            ], required: ["title", "startDate"]),
            writeTool("hcb_update_task", "Update task or note fields.", properties: [
                "id": stringSchema("Task id."),
                "patch": objectSchema("Fields: title, notes, dueDate, taskListID."),
                "dryRun": booleanSchema("Preview without applying."),
                "confirmationId": stringSchema("Confirmation id returned by a dry-run.")
            ], required: ["id", "patch"]),
            writeTool("hcb_update_event", "Update event fields.", properties: [
                "id": stringSchema("Event id."),
                "patch": objectSchema("Fields: title, details, startDate, endDate, isAllDay, location, calendarID."),
                "dryRun": booleanSchema("Preview without applying."),
                "confirmationId": stringSchema("Confirmation id returned by a dry-run.")
            ], required: ["id", "patch"]),
            writeTool("hcb_complete_task", "Mark a task complete.", properties: [
                "id": stringSchema("Task id."),
                "dryRun": booleanSchema("Preview without applying."),
                "confirmationId": stringSchema("Confirmation id returned by a dry-run.")
            ], required: ["id"]),
            writeTool("hcb_reopen_task", "Reopen a completed task.", properties: [
                "id": stringSchema("Task id."),
                "dryRun": booleanSchema("Preview without applying."),
                "confirmationId": stringSchema("Confirmation id returned by a dry-run.")
            ], required: ["id"]),
            writeTool("hcb_move_task", "Move a task or note to another list.", properties: [
                "id": stringSchema("Task id."),
                "taskListID": stringSchema("Destination task list id or title."),
                "dryRun": booleanSchema("Preview without applying."),
                "confirmationId": stringSchema("Confirmation id returned by a dry-run.")
            ], required: ["id", "taskListID"]),
            writeTool("hcb_delete_task", "Delete a task or note. Always requires confirmation.", properties: [
                "id": stringSchema("Task id."),
                "dryRun": booleanSchema("Preview without applying."),
                "confirmationId": stringSchema("Confirmation id returned by a dry-run.")
            ], required: ["id"]),
            writeTool("hcb_delete_event", "Delete an event. Always requires confirmation.", properties: [
                "id": stringSchema("Event id."),
                "dryRun": booleanSchema("Preview without applying."),
                "confirmationId": stringSchema("Confirmation id returned by a dry-run.")
            ], required: ["id"])
        ]
    }

    nonisolated static let writeToolNames: Set<String> = [
        "hcb_create_task",
        "hcb_create_note",
        "hcb_create_event",
        "hcb_update_task",
        "hcb_update_event",
        "hcb_complete_task",
        "hcb_reopen_task",
        "hcb_move_task",
        "hcb_delete_task",
        "hcb_delete_event"
    ]

    nonisolated static func isWriteTool(_ name: String) -> Bool {
        writeToolNames.contains(name)
    }

    func callTool(name: String, arguments: [String: Any]) async throws -> [String: Any] {
        switch name {
        case "hcb_search":
            return try await search(arguments)
        case "hcb_today":
            return today()
        case "hcb_week":
            return try week(arguments)
        case "hcb_get_task":
            return try getTask(arguments)
        case "hcb_get_event":
            return try getEvent(arguments)
        case "hcb_list_task_lists":
            return listTaskLists()
        case "hcb_list_calendars":
            return listCalendars()
        case "hcb_create_task":
            return try await createTask(arguments, isNote: false)
        case "hcb_create_note":
            return try await createTask(arguments, isNote: true)
        case "hcb_create_event":
            return try await createEvent(arguments)
        case "hcb_update_task":
            return try await updateTask(arguments)
        case "hcb_update_event":
            return try await updateEvent(arguments)
        case "hcb_complete_task":
            return try await setTaskCompleted(arguments, isCompleted: true)
        case "hcb_reopen_task":
            return try await setTaskCompleted(arguments, isCompleted: false)
        case "hcb_move_task":
            return try await moveTask(arguments)
        case "hcb_delete_task":
            return try await deleteTask(arguments)
        case "hcb_delete_event":
            return try await deleteEvent(arguments)
        default:
            throw HCBToolError.unknownTool(name)
        }
    }

    // MARK: - Reads

    private func search(_ arguments: [String: Any]) async throws -> [String: Any] {
        let query = try requiredString("query", in: arguments)
        let scope = string("scope", in: arguments) ?? "all"
        let limit = normalizedLimit(arguments["limit"])
        let cacheScope: LocalCacheEntitySearchScope
        switch scope {
        case "tasks":
            cacheScope = .tasks
        case "notes":
            cacheScope = .notes
        case "events":
            cacheScope = .events
        default:
            cacheScope = .all
        }
        if scope != "lists", scope != "calendars",
           let databaseEntities = try await model.searchEntities(query: query, scope: cacheScope, limit: limit) {
            let scopedEntities = databaseEntities.filter { entity in
                switch (scope, entity) {
                case ("tasks", .task(let task)):
                    return task.dueDate != nil
                case ("notes", .task(let task)):
                    return task.appearsInNotesSurface
                case ("events", .event):
                    return true
                case ("all", _):
                    return true
                default:
                    return scope == "all"
                }
            }
            return success(
                message: "Found \(scopedEntities.count) result\(scopedEntities.count == 1 ? "" : "s").",
                items: scopedEntities.map(sanitize)
            )
        }

        let parsed = AdvancedSearchParser.parse(query)
        let freeText = parsed.freeText.trimmingCharacters(in: .whitespacesAndNewlines)

        var entities: [QuickSwitcherEntity] = []
        if scope == "all" || scope == "tasks" || scope == "notes" {
            entities += model.tasks
                .filter { $0.isDeleted == false }
                .filter { scope != "tasks" || $0.dueDate != nil }
                .filter { scope != "notes" || $0.appearsInNotesSurface }
                .map(QuickSwitcherEntity.task)
        }
        if scope == "all" || scope == "events" {
            entities += model.events
                .filter { $0.status != .cancelled }
                .map(QuickSwitcherEntity.event)
        }
        if scope == "all" || scope == "lists" {
            entities += model.taskLists.map(QuickSwitcherEntity.taskList)
        }
        if scope == "all" || scope == "calendars" {
            entities += model.calendars.map(QuickSwitcherEntity.calendar)
        }

        let filtered = entities.filter {
            AdvancedSearchMatcher.matches(
                $0,
                query: parsed,
                calendars: model.calendars,
                taskLists: model.taskLists
            )
        }
        let ranked: [QuickSwitcherEntity]
        if freeText.isEmpty {
            ranked = Array(filtered.prefix(limit))
        } else {
            ranked = FuzzySearcher.rank(
                filtered,
                query: freeText,
                labelForItem: entityLabel,
                keywordsForItem: entityKeywords,
                limit: limit
            ).map(\.item)
        }

        return success(
            message: "Found \(ranked.count) result\(ranked.count == 1 ? "" : "s").",
            items: ranked.map(sanitize)
        )
    }

    private func today() -> [String: Any] {
        let snapshot = model.todaySnapshot
        return success(message: "Read today's agenda.", item: [
            "date": isoDate(snapshot.date),
            "overdueCount": snapshot.overdueCount,
            "tasks": snapshot.dueTasks.map(sanitize(task:)),
            "events": snapshot.scheduledEvents.map(sanitize(event:))
        ])
    }

    private func week(_ arguments: [String: Any]) throws -> [String: Any] {
        let calendar = Calendar.current
        let start = try string("startDate", in: arguments).flatMap(parseDate) ?? calendar.startOfDay(for: Date())
        guard let end = calendar.date(byAdding: .day, value: 7, to: start) else {
            throw HCBToolError.invalidArguments("Could not compute week end date.")
        }
        let tasks = model.tasks
            .filter { task in
                guard task.isDeleted == false, task.isCompleted == false, let due = task.dueDate else { return false }
                return due >= start && due < end
            }
            .map(sanitize(task:))
        let events = model.events
            .filter { $0.status != .cancelled && $0.endDate >= start && $0.startDate < end }
            .sorted { $0.startDate < $1.startDate }
            .map(sanitize(event:))

        return success(message: "Read week agenda.", item: [
            "startDate": isoDate(start),
            "endDate": isoDate(end),
            "tasks": tasks,
            "events": events
        ])
    }

    private func getTask(_ arguments: [String: Any]) throws -> [String: Any] {
        let id = try requiredString("id", in: arguments)
        guard let task = model.task(id: id), task.isDeleted == false else {
            throw HCBToolError.notFound("Task '\(id)' was not found.")
        }
        return success(message: "Read task.", item: sanitize(task: task))
    }

    private func getEvent(_ arguments: [String: Any]) throws -> [String: Any] {
        let id = try requiredString("id", in: arguments)
        guard let event = model.event(id: id), event.status != .cancelled else {
            throw HCBToolError.notFound("Event '\(id)' was not found.")
        }
        return success(message: "Read event.", item: sanitize(event: event))
    }

    private func listTaskLists() -> [String: Any] {
        success(message: "Read task lists.", items: model.taskLists.map(sanitize(taskList:)))
    }

    private func listCalendars() -> [String: Any] {
        success(message: "Read calendars.", items: model.calendars.map(sanitize(calendar:)))
    }

    // MARK: - Writes

    private func createTask(_ arguments: [String: Any], isNote: Bool) async throws -> [String: Any] {
        let title = try requiredString("title", in: arguments)
        let notes = string("notes", in: arguments) ?? ""
        let list = try resolveTaskList(string("taskListID", in: arguments) ?? string("list", in: arguments))
        let dueDate = isNote ? nil : try string("dueDate", in: arguments).flatMap(parseDate)
        let preview = [
            "kind": isNote ? "note" : "task",
            "title": title,
            "notes": notes,
            "dueDate": dueDate.map(isoDate) as Any,
            "taskList": sanitize(taskList: list)
        ].compactNulls()

        if let dryRun = try authorizeWrite(toolName: isNote ? "hcb_create_note" : "hcb_create_task", arguments: arguments, destructive: false, preview: preview) {
            return dryRun
        }

        let ok = await model.createTask(title: title, notes: notes, dueDate: dueDate, taskListID: list.id)
        guard ok else { throw HCBToolError.mutationFailed(model.lastMutationError ?? "Could not create \(isNote ? "note" : "task").") }
        return success(applied: true, message: isNote ? "Created note." : "Created task.", item: preview)
    }

    private func createEvent(_ arguments: [String: Any]) async throws -> [String: Any] {
        let title = try requiredString("title", in: arguments, aliases: ["summary"])
        let details = string("details", in: arguments) ?? string("notes", in: arguments) ?? ""
        let start = try parseDate(try requiredString("startDate", in: arguments, aliases: ["start"]))
        let isAllDay = bool("isAllDay", in: arguments) ?? bool("allDay", in: arguments) ?? false
        let end = try string("endDate", in: arguments).flatMap(parseDate)
            ?? string("end", in: arguments).flatMap(parseDate)
            ?? Calendar.current.date(byAdding: isAllDay ? .day : .hour, value: 1, to: start)
            ?? start
        let calendar = try resolveCalendar(string("calendarID", in: arguments) ?? string("calendar", in: arguments))
        let location = string("location", in: arguments) ?? ""
        let preview = [
            "kind": "event",
            "title": title,
            "details": details,
            "startDate": isoDate(start),
            "endDate": isoDate(end),
            "isAllDay": isAllDay,
            "location": location,
            "calendar": sanitize(calendar: calendar)
        ] as [String: Any]

        if let dryRun = try authorizeWrite(toolName: "hcb_create_event", arguments: arguments, destructive: false, preview: preview) {
            return dryRun
        }

        let ok = await model.createEvent(
            summary: title,
            details: details,
            startDate: start,
            endDate: end,
            isAllDay: isAllDay,
            reminderMinutes: nil,
            calendarID: calendar.id,
            location: location
        )
        guard ok else { throw HCBToolError.mutationFailed(model.lastMutationError ?? "Could not create event.") }
        return success(applied: true, message: "Created event.", item: preview)
    }

    private func updateTask(_ arguments: [String: Any]) async throws -> [String: Any] {
        let id = try requiredString("id", in: arguments)
        guard let task = model.task(id: id), task.isDeleted == false else {
            throw HCBToolError.notFound("Task '\(id)' was not found.")
        }
        let patch = try dictionary("patch", in: arguments)
        let title = string("title", in: patch) ?? task.title
        let notes = string("notes", in: patch) ?? task.notes
        let dueDate = try patch.keys.contains("dueDate") ? nullableDate("dueDate", in: patch) : task.dueDate
        let targetList = try (string("taskListID", in: patch) ?? string("list", in: patch)).map { try resolveTaskList($0) }
        var preview = sanitize(task: task)
        preview["patch"] = patch.redactedForMCP()
        if let targetList {
            preview["targetTaskList"] = sanitize(taskList: targetList)
        }

        if let dryRun = try authorizeWrite(toolName: "hcb_update_task", arguments: arguments, destructive: false, preview: preview) {
            return dryRun
        }

        let ok = await model.updateTask(task, title: title, notes: notes, dueDate: dueDate)
        guard ok else { throw HCBToolError.mutationFailed(model.lastMutationError ?? "Could not update task.") }
        if let targetList, let refreshed = model.task(id: id), refreshed.taskListID != targetList.id {
            let moved = await model.moveTaskToList(refreshed, toTaskListID: targetList.id)
            guard moved else { throw HCBToolError.mutationFailed(model.lastMutationError ?? "Could not move task.") }
        }
        return success(applied: true, message: "Updated task.", item: model.task(id: id).map(sanitize(task:)) ?? preview)
    }

    private func updateEvent(_ arguments: [String: Any]) async throws -> [String: Any] {
        let id = try requiredString("id", in: arguments)
        guard let event = model.event(id: id), event.status != .cancelled else {
            throw HCBToolError.notFound("Event '\(id)' was not found.")
        }
        let patch = try dictionary("patch", in: arguments)
        let title = string("title", in: patch) ?? string("summary", in: patch) ?? event.summary
        let details = string("details", in: patch) ?? string("notes", in: patch) ?? event.details
        let start = try patch.keys.contains("startDate") ? parseDate(try requiredString("startDate", in: patch)) : event.startDate
        let end = try patch.keys.contains("endDate") ? parseDate(try requiredString("endDate", in: patch)) : event.endDate
        let isAllDay = bool("isAllDay", in: patch) ?? event.isAllDay
        let location = string("location", in: patch) ?? event.location
        let calendar = try (string("calendarID", in: patch) ?? string("calendar", in: patch)).map { try resolveCalendar($0) }
        var preview = sanitize(event: event)
        preview["patch"] = patch.redactedForMCP()
        if let calendar {
            preview["targetCalendar"] = sanitize(calendar: calendar)
        }

        if let dryRun = try authorizeWrite(toolName: "hcb_update_event", arguments: arguments, destructive: false, preview: preview) {
            return dryRun
        }

        let ok = await model.updateEvent(
            event,
            summary: title,
            details: details,
            startDate: start,
            endDate: end,
            isAllDay: isAllDay,
            reminderMinutes: event.reminderMinutes.first,
            calendarID: calendar?.id ?? event.calendarID,
            location: location
        )
        guard ok else { throw HCBToolError.mutationFailed(model.lastMutationError ?? "Could not update event.") }
        return success(applied: true, message: "Updated event.", item: model.event(id: id).map(sanitize(event:)) ?? preview)
    }

    private func setTaskCompleted(_ arguments: [String: Any], isCompleted: Bool) async throws -> [String: Any] {
        let id = try requiredString("id", in: arguments)
        guard let task = model.task(id: id), task.isDeleted == false else {
            throw HCBToolError.notFound("Task '\(id)' was not found.")
        }
        let preview = sanitize(task: task).merging(["targetStatus": isCompleted ? "completed" : "needsAction"]) { _, new in new }
        if let dryRun = try authorizeWrite(toolName: isCompleted ? "hcb_complete_task" : "hcb_reopen_task", arguments: arguments, destructive: false, preview: preview) {
            return dryRun
        }
        let ok = await model.setTaskCompleted(isCompleted, task: task)
        guard ok else { throw HCBToolError.mutationFailed(model.lastMutationError ?? "Could not update task completion.") }
        return success(applied: true, message: isCompleted ? "Completed task." : "Reopened task.", item: model.task(id: id).map(sanitize(task:)) ?? preview)
    }

    private func moveTask(_ arguments: [String: Any]) async throws -> [String: Any] {
        let id = try requiredString("id", in: arguments)
        guard let task = model.task(id: id), task.isDeleted == false else {
            throw HCBToolError.notFound("Task '\(id)' was not found.")
        }
        let list = try resolveTaskList(try requiredString("taskListID", in: arguments, aliases: ["list"]))
        let preview = sanitize(task: task).merging(["targetTaskList": sanitize(taskList: list)]) { _, new in new }
        if let dryRun = try authorizeWrite(toolName: "hcb_move_task", arguments: arguments, destructive: false, preview: preview) {
            return dryRun
        }
        let ok = await model.moveTaskToList(task, toTaskListID: list.id)
        guard ok else { throw HCBToolError.mutationFailed(model.lastMutationError ?? "Could not move task.") }
        return success(applied: true, message: "Moved task.", item: model.task(id: id).map(sanitize(task:)) ?? preview)
    }

    private func deleteTask(_ arguments: [String: Any]) async throws -> [String: Any] {
        let id = try requiredString("id", in: arguments)
        guard let task = model.task(id: id), task.isDeleted == false else {
            throw HCBToolError.notFound("Task '\(id)' was not found.")
        }
        let preview = sanitize(task: task)
        if let dryRun = try authorizeWrite(toolName: "hcb_delete_task", arguments: arguments, destructive: true, preview: preview) {
            return dryRun
        }
        let ok = await model.deleteTask(task)
        guard ok else { throw HCBToolError.mutationFailed(model.lastMutationError ?? "Could not delete task.") }
        return success(applied: true, message: "Deleted task.", item: preview)
    }

    private func deleteEvent(_ arguments: [String: Any]) async throws -> [String: Any] {
        let id = try requiredString("id", in: arguments)
        guard let event = model.event(id: id), event.status != .cancelled else {
            throw HCBToolError.notFound("Event '\(id)' was not found.")
        }
        let preview = sanitize(event: event)
        if let dryRun = try authorizeWrite(toolName: "hcb_delete_event", arguments: arguments, destructive: true, preview: preview) {
            return dryRun
        }
        let ok = await model.deleteEvent(event)
        guard ok else { throw HCBToolError.mutationFailed(model.lastMutationError ?? "Could not delete event.") }
        return success(applied: true, message: "Deleted event.", item: preview)
    }

    // MARK: - Authorization

    private func authorizeWrite(
        toolName: String,
        arguments: [String: Any],
        destructive: Bool,
        preview: [String: Any]
    ) throws -> [String: Any]? {
        if model.settings.mcpPermissionMode == .readOnly {
            throw HCBToolError.permissionDenied("MCP is in read-only mode.")
        }

        let dryRun = bool("dryRun", in: arguments) ?? false
        let requiresConfirmation = destructive || model.settings.mcpPermissionMode == .confirmWrites
        if dryRun {
            let confirmationId = requiresConfirmation ? storeConfirmation(toolName: toolName, arguments: arguments) : nil
            return success(
                applied: false,
                dryRun: true,
                requiresConfirmation: requiresConfirmation,
                confirmationId: confirmationId,
                message: requiresConfirmation ? "Dry-run ready. Pass confirmationId to apply." : "Dry-run preview.",
                item: preview
            )
        }

        guard requiresConfirmation else { return nil }
        if let id = string("confirmationId", in: arguments) {
            guard consumeConfirmation(id: id, toolName: toolName, arguments: arguments) else {
                throw HCBToolError.confirmationMismatch
            }
            return nil
        } else {
            throw HCBToolError.confirmationRequired("Dry-run confirmation is required before this write can apply.", nil)
        }
    }

    private func storeConfirmation(toolName: String, arguments: [String: Any]) -> String {
        pruneConfirmations()
        let id = UUID().uuidString
        confirmations[id] = PendingConfirmation(
            toolName: toolName,
            canonicalArguments: canonicalArguments(arguments),
            expiresAt: Date().addingTimeInterval(confirmationTTL)
        )
        return id
    }

    private func consumeConfirmation(id: String, toolName: String, arguments: [String: Any]) -> Bool {
        pruneConfirmations()
        guard let confirmation = confirmations.removeValue(forKey: id),
              confirmation.toolName == toolName,
              confirmation.expiresAt >= Date(),
              confirmation.canonicalArguments == canonicalArguments(arguments) else {
            return false
        }
        return true
    }

    private func pruneConfirmations() {
        let now = Date()
        confirmations = confirmations.filter { $0.value.expiresAt >= now }
    }

    private func canonicalArguments(_ arguments: [String: Any]) -> String {
        var normalized = arguments
        normalized.removeValue(forKey: "dryRun")
        normalized.removeValue(forKey: "confirmationId")
        let object = normalized.jsonCompatible()
        guard JSONSerialization.isValidJSONObject(object),
              let data = try? JSONSerialization.data(withJSONObject: object, options: [.sortedKeys]),
              let string = String(data: data, encoding: .utf8) else {
            return String(describing: object)
        }
        return string
    }

    // MARK: - Sanitizers

    private func sanitize(_ entity: QuickSwitcherEntity) -> [String: Any] {
        switch entity {
        case .task(let task):
            return sanitize(task: task)
        case .event(let event):
            return sanitize(event: event)
        case .taskList(let list):
            return sanitize(taskList: list)
        case .calendar(let calendar):
            return sanitize(calendar: calendar)
        case .customFilter(let filter):
            return [
                "kind": "filter",
                "id": filter.id.uuidString,
                "name": filter.name,
                "queryExpression": filter.queryExpression as Any
            ].compactNulls()
        }
    }

    private func sanitize(task: TaskMirror) -> [String: Any] {
        [
            "kind": task.appearsInNotesSurface ? "note" : "task",
            "id": task.id,
            "title": TagExtractor.stripped(from: task.title),
            "rawTitle": task.title,
            "notes": task.notes,
            "status": task.status.rawValue,
            "isCompleted": task.isCompleted,
            "dueDate": task.dueDate.map(isoDate) as Any,
            "completedAt": task.completedAt.map(isoDate) as Any,
            "taskListID": task.taskListID,
            "taskListTitle": model.taskListTitle(for: task.taskListID),
            "deepLink": HCBDeepLinkBuilder.taskURL(for: task).absoluteString
        ].compactNulls()
    }

    private func sanitize(event: CalendarEventMirror) -> [String: Any] {
        [
            "kind": "event",
            "id": event.id,
            "title": event.summary,
            "details": event.details,
            "startDate": isoDate(event.startDate),
            "endDate": isoDate(event.endDate),
            "isAllDay": event.isAllDay,
            "status": event.status.rawValue,
            "calendarID": event.calendarID,
            "calendarTitle": model.calendarTitle(for: event.calendarID),
            "location": event.location,
            "attendeeEmails": event.attendeeEmails,
            "meetLink": event.meetLink,
            "htmlLink": event.htmlLink as Any,
            "deepLink": HCBDeepLinkBuilder.eventURL(for: event).absoluteString
        ].compactNulls()
    }

    private func sanitize(taskList: TaskListMirror) -> [String: Any] {
        [
            "kind": "taskList",
            "id": taskList.id,
            "title": taskList.title,
            "updatedAt": taskList.updatedAt.map(isoDate) as Any
        ].compactNulls()
    }

    private func sanitize(calendar: CalendarListMirror) -> [String: Any] {
        [
            "kind": "calendar",
            "id": calendar.id,
            "summary": calendar.summary,
            "colorHex": calendar.colorHex,
            "isSelected": calendar.isSelected,
            "accessRole": calendar.accessRole,
            "timeZoneID": calendar.timeZoneID as Any
        ].compactNulls()
    }

    private func success(
        applied: Bool = false,
        dryRun: Bool = false,
        requiresConfirmation: Bool = false,
        confirmationId: String? = nil,
        message: String,
        item: [String: Any]? = nil,
        items: [[String: Any]]? = nil
    ) -> [String: Any] {
        [
            "applied": applied,
            "dryRun": dryRun,
            "requiresConfirmation": requiresConfirmation,
            "confirmationId": confirmationId as Any,
            "message": message,
            "item": item as Any,
            "items": items as Any
        ].compactNulls()
    }

    // MARK: - Resolvers and parsing

    private func resolveTaskList(_ ref: String?) throws -> TaskListMirror {
        if let ref, let exact = model.taskLists.first(where: { $0.id == ref || $0.title.localizedCaseInsensitiveCompare(ref) == .orderedSame }) {
            return exact
        }
        guard let first = model.taskLists.first else {
            throw HCBToolError.invalidArguments("No task list is available.")
        }
        return first
    }

    private func resolveCalendar(_ ref: String?) throws -> CalendarListMirror {
        if let ref, let exact = model.calendars.first(where: { $0.id == ref || $0.summary.localizedCaseInsensitiveCompare(ref) == .orderedSame }) {
            return exact
        }
        if let selected = model.calendars.first(where: \.isSelected) {
            return selected
        }
        guard let first = model.calendars.first else {
            throw HCBToolError.invalidArguments("No calendar is available.")
        }
        return first
    }

    private func parseDate(_ raw: String) throws -> Date {
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        if let date = Self.isoDateTime.date(from: trimmed)
            ?? Self.isoDateTimeWithFractionalSeconds.date(from: trimmed)
            ?? Self.isoDateOnly.date(from: trimmed) {
            return date
        }
        switch HCBDeepLinkRouter.parseDateParam(trimmed) {
        case .success(let date):
            return date
        case .failure:
            throw HCBToolError.invalidArguments("Invalid date '\(raw)'. Use ISO-8601, today, tomorrow, or a relative date like +3d.")
        }
    }

    private func nullableDate(_ key: String, in arguments: [String: Any]) throws -> Date? {
        guard let value = arguments[key], !(value is NSNull) else { return nil }
        guard let raw = value as? String else {
            throw HCBToolError.invalidArguments("'\(key)' must be an ISO-8601 string or null.")
        }
        return try parseDate(raw)
    }

    private func requiredString(_ key: String, in arguments: [String: Any], aliases: [String] = []) throws -> String {
        for candidate in [key] + aliases {
            if let value = string(candidate, in: arguments), value.isEmpty == false {
                return value
            }
        }
        throw HCBToolError.invalidArguments("Missing required string argument '\(key)'.")
    }

    private func string(_ key: String, in arguments: [String: Any]) -> String? {
        guard let raw = arguments[key] as? String else { return nil }
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? nil : trimmed
    }

    private func bool(_ key: String, in arguments: [String: Any]) -> Bool? {
        arguments[key] as? Bool
    }

    private func dictionary(_ key: String, in arguments: [String: Any]) throws -> [String: Any] {
        guard let dict = arguments[key] as? [String: Any] else {
            throw HCBToolError.invalidArguments("'\(key)' must be an object.")
        }
        return dict
    }

    private func normalizedLimit(_ value: Any?) -> Int {
        let raw = (value as? Int) ?? (value as? NSNumber)?.intValue ?? 20
        return max(1, min(100, raw))
    }

    private func entityLabel(_ entity: QuickSwitcherEntity) -> String {
        switch entity {
        case .task(let task):
            return TagExtractor.stripped(from: task.title)
        case .event(let event):
            return event.summary
        case .taskList(let list):
            return list.title
        case .calendar(let calendar):
            return calendar.summary
        case .customFilter(let filter):
            return filter.name
        }
    }

    private func entityKeywords(_ entity: QuickSwitcherEntity) -> [String] {
        switch entity {
        case .task(let task):
            return TagExtractor.tags(in: task.title) + [task.notes, model.taskListTitle(for: task.taskListID)]
        case .event(let event):
            return [event.details, event.location, model.calendarTitle(for: event.calendarID)]
        case .taskList, .calendar:
            return []
        case .customFilter(let filter):
            return filter.queryExpression.map { [$0] } ?? []
        }
    }

    private func isoDate(_ date: Date) -> String {
        Self.isoDateTime.string(from: date)
    }

    private static let isoDateTime: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime]
        return formatter
    }()

    private static let isoDateTimeWithFractionalSeconds: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter
    }()

    private static let isoDateOnly: DateFormatter = {
        let formatter = DateFormatter()
        formatter.calendar = Calendar(identifier: .gregorian)
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.timeZone = TimeZone(secondsFromGMT: 0)
        formatter.dateFormat = "yyyy-MM-dd"
        return formatter
    }()

    nonisolated private static func readTool(_ name: String, _ description: String, properties: [String: Any], required: [String] = []) -> HCBToolDefinition {
        HCBToolDefinition(name: name, description: description, inputSchema: schema(properties: properties, required: required))
    }

    nonisolated private static func writeTool(_ name: String, _ description: String, properties: [String: Any], required: [String] = []) -> HCBToolDefinition {
        HCBToolDefinition(name: name, description: description, inputSchema: schema(properties: properties, required: required))
    }

    nonisolated private static func schema(properties: [String: Any], required: [String]) -> [String: Any] {
        [
            "type": "object",
            "properties": properties,
            "required": required,
            "additionalProperties": false
        ]
    }

    nonisolated private static func stringSchema(_ description: String) -> [String: Any] {
        ["type": "string", "description": description]
    }

    nonisolated private static func integerSchema(_ description: String) -> [String: Any] {
        ["type": "integer", "description": description]
    }

    nonisolated private static func booleanSchema(_ description: String) -> [String: Any] {
        ["type": "boolean", "description": description]
    }

    nonisolated private static func objectSchema(_ description: String) -> [String: Any] {
        ["type": "object", "description": description]
    }

    nonisolated private static func enumSchema(_ values: [String]) -> [String: Any] {
        ["type": "string", "enum": values]
    }
}

private extension Dictionary where Key == String, Value == Any {
    func compactNulls() -> [String: Any] {
        filter { !($0.value is NSNull) }.compactMapValues { value in
            if case Optional<Any>.none = value as Any? {
                return nil
            }
            return value
        }
    }

    func redactedForMCP() -> [String: Any] {
        var out: [String: Any] = [:]
        for (key, value) in self {
            if Self.isSensitiveMCPKey(key) {
                out[key] = "[redacted]"
            } else {
                out[key] = Self.redactedMCPValue(value)
            }
        }
        return out
    }

    private static func redactedMCPValue(_ value: Any) -> Any {
        switch value {
        case let dict as [String: Any]:
            return dict.redactedForMCP()
        case let array as [Any]:
            return array.map(redactedMCPValue)
        default:
            return value
        }
    }

    private static func isSensitiveMCPKey(_ key: String) -> Bool {
        let lower = key.lowercased()
        return lower.contains("token")
            || lower.contains("secret")
            || lower.contains("credential")
            || lower.contains("key")
            || lower.contains("authorization")
            || lower.contains("password")
    }

    func jsonCompatible() -> [String: Any] {
        var out: [String: Any] = [:]
        for (key, value) in self {
            out[key] = HCBJSONCompatibility.convert(value)
        }
        return out
    }
}

private enum HCBJSONCompatibility {
    static func convert(_ value: Any) -> Any {
        switch value {
        case let dict as [String: Any]:
            return dict.jsonCompatible()
        case let array as [Any]:
            return array.map(convert)
        case let date as Date:
            return ISO8601DateFormatter().string(from: date)
        case Optional<Any>.none:
            return NSNull()
        default:
            return value
        }
    }
}
