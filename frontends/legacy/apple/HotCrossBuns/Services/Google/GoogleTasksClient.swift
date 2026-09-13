import Foundation

struct GoogleTasksClient: Sendable {
    private let transport: GoogleAPITransport

    init(transport: GoogleAPITransport) {
        self.transport = transport
    }

    func listTaskLists() async throws -> [TaskListMirror] {
        do {
            let response: GoogleTaskListsResponse = try await transport.get(
                path: "/tasks/v1/users/@me/lists",
                queryItems: [URLQueryItem(name: "fields", value: Self.taskListsFields)]
            )
            let lists = response.items.map(\.mirror)
            AppLogger.info("google task lists listed", category: .google, metadata: ["count": String(lists.count)])
            return lists
        } catch {
            AppLogger.warn("google task lists list failed", category: .google, metadata: GoogleDiagnostics.errorMetadata(error))
            throw error
        }
    }

    func insertTaskList(title: String) async throws -> TaskListMirror {
        let response: GoogleTaskListDTO = try await transport.request(
            method: "POST",
            path: "/tasks/v1/users/@me/lists",
            body: GoogleTaskListMutationDTO(title: title)
        )
        let mirror = response.mirror
        AppLogger.info("google task list write accepted", category: .google, metadata: [
            "action": "create",
            "taskListID": GoogleDiagnostics.redactedIdentifier(mirror.id)
        ])
        return mirror
    }

    func updateTaskList(taskListID: String, title: String) async throws -> TaskListMirror {
        let encodedTaskListID = taskListID.googlePathComponentEncoded
        let response: GoogleTaskListDTO = try await transport.request(
            method: "PATCH",
            path: "/tasks/v1/users/@me/lists/\(encodedTaskListID)",
            body: GoogleTaskListMutationDTO(title: title)
        )
        let mirror = response.mirror
        AppLogger.info("google task list write accepted", category: .google, metadata: [
            "action": "update",
            "taskListID": GoogleDiagnostics.redactedIdentifier(mirror.id)
        ])
        return mirror
    }

    func deleteTaskList(taskListID: String) async throws {
        let encodedTaskListID = taskListID.googlePathComponentEncoded
        try await transport.send(
            method: "DELETE",
            path: "/tasks/v1/users/@me/lists/\(encodedTaskListID)"
        )
        AppLogger.info("google task list write accepted", category: .google, metadata: [
            "action": "delete",
            "taskListID": GoogleDiagnostics.redactedIdentifier(taskListID)
        ])
    }

    // §14 — Returns fetched tasks plus the `Date` header from the FIRST
    // response page. Using the first page's server timestamp as the next
    // watermark is correct: subsequent pages share the same query snapshot,
    // and any task updated on Google after that first Date will be outside
    // this page set and therefore picked up by the next incremental sync.
    // `serverDate == nil` is only expected when the header is missing or
    // unparseable; SyncScheduler falls back to a local-clock watermark in
    // that case.
    func listTasks(taskListID: String, updatedMin: Date?, completedMin: Date? = nil) async throws -> GoogleTasksPage {
        let encodedTaskListID = taskListID.googlePathComponentEncoded
        let mode = updatedMin == nil ? "full" : "incremental"
        AppLogger.info("google tasks list start", category: .google, metadata: [
            "taskListID": GoogleDiagnostics.redactedIdentifier(taskListID),
            "mode": mode,
            "hasUpdatedMin": String(updatedMin != nil),
            "hasCompletedMin": String(completedMin != nil)
        ])
        let baseQueryItems = [
            URLQueryItem(name: "showCompleted", value: "true"),
            URLQueryItem(name: "showDeleted", value: "true"),
            URLQueryItem(name: "showHidden", value: "true"),
            URLQueryItem(name: "maxResults", value: "100"),
            URLQueryItem(name: "fields", value: Self.tasksFields)
        ]
        var pageToken: String?
        var tasks: [TaskMirror] = []
        var firstPageServerDate: Date?
        var isFirstPage = true
        var pageCount = 0

        do {
            repeat {
                pageCount += 1
                var queryItems = baseQueryItems

                if let updatedMin {
                    queryItems.append(URLQueryItem(name: "updatedMin", value: ISO8601DateFormatter.google.string(from: updatedMin)))
                } else if let completedMin {
                    queryItems.append(URLQueryItem(name: "completedMin", value: ISO8601DateFormatter.google.string(from: completedMin)))
                }

                if let pageToken {
                    queryItems.append(URLQueryItem(name: "pageToken", value: pageToken))
                }

                let (response, serverDate): (GoogleTasksResponse, Date?) = try await transport.getWithServerDate(
                    path: "/tasks/v1/lists/\(encodedTaskListID)/tasks",
                    queryItems: queryItems
                )

                if isFirstPage {
                    firstPageServerDate = serverDate
                    isFirstPage = false
                }

                tasks.append(contentsOf: response.items.map { $0.mirror(taskListID: taskListID) })
                pageToken = response.nextPageToken
            } while pageToken != nil
        } catch {
            AppLogger.warn("google tasks list failed", category: .google, metadata: [
                "taskListID": GoogleDiagnostics.redactedIdentifier(taskListID),
                "mode": mode,
                "pages": String(pageCount)
            ].merging(GoogleDiagnostics.errorMetadata(error)) { _, new in new })
            throw error
        }

        AppLogger.info("google tasks list succeeded", category: .google, metadata: [
            "taskListID": GoogleDiagnostics.redactedIdentifier(taskListID),
            "mode": mode,
            "pages": String(pageCount),
            "count": String(tasks.count),
            "hasServerDate": String(firstPageServerDate != nil)
        ])

        return GoogleTasksPage(tasks: tasks, serverDate: firstPageServerDate)
    }

    func insertTask(
        taskListID: String,
        title: String,
        notes: String,
        dueDate: Date?,
        parent: String? = nil,
        previous: String? = nil
    ) async throws -> TaskMirror {
        let encodedTaskListID = taskListID.googlePathComponentEncoded
        let requestBody = GoogleTaskMutationDTO(
            title: title,
            notes: notes.isEmpty ? nil : notes,
            due: dueDate.map { GoogleTaskDueDateFormatter.string(from: $0) }
        )
        var queryItems: [URLQueryItem] = []
        if let parent { queryItems.append(URLQueryItem(name: "parent", value: parent)) }
        if let previous { queryItems.append(URLQueryItem(name: "previous", value: previous)) }
        let response: GoogleTaskDTO = try await transport.request(
            method: "POST",
            path: "/tasks/v1/lists/\(encodedTaskListID)/tasks",
            queryItems: queryItems,
            body: requestBody
        )
        let mirror = response.mirror(taskListID: taskListID)
        AppLogger.info("google task write accepted", category: .google, metadata: [
            "action": "create",
            "taskListID": GoogleDiagnostics.redactedIdentifier(taskListID),
            "taskID": GoogleDiagnostics.redactedIdentifier(mirror.id),
            "hasNotes": String(notes.isEmpty == false),
            "hasDueDate": String(dueDate != nil),
            "hasParent": String(parent != nil)
        ])
        return mirror
    }

    func moveTask(
        taskListID: String,
        taskID: String,
        parent: String?,
        previous: String?
    ) async throws -> TaskMirror {
        let encodedTaskListID = taskListID.googlePathComponentEncoded
        let encodedTaskID = taskID.googlePathComponentEncoded
        var queryItems: [URLQueryItem] = []
        if let parent { queryItems.append(URLQueryItem(name: "parent", value: parent)) }
        if let previous { queryItems.append(URLQueryItem(name: "previous", value: previous)) }
        let response: GoogleTaskDTO = try await transport.request(
            method: "POST",
            path: "/tasks/v1/lists/\(encodedTaskListID)/tasks/\(encodedTaskID)/move",
            queryItems: queryItems
        )
        let mirror = response.mirror(taskListID: taskListID)
        AppLogger.info("google task write accepted", category: .google, metadata: [
            "action": "move",
            "taskListID": GoogleDiagnostics.redactedIdentifier(taskListID),
            "taskID": GoogleDiagnostics.redactedIdentifier(taskID),
            "hasParent": String(parent != nil),
            "hasPrevious": String(previous != nil)
        ])
        return mirror
    }

    func updateTask(
        taskListID: String,
        taskID: String,
        title: String,
        notes: String,
        dueDate: Date?,
        ifMatch: String? = nil
    ) async throws -> TaskMirror {
        let dueString = dueDate.map { GoogleTaskDueDateFormatter.string(from: $0) }
        return try await patchTask(
            taskListID: taskListID,
            taskID: taskID,
            body: GoogleTaskPatchDTO(
                title: title,
                notes: .value(notes),
                status: nil,
                due: dueString.map(NullableField.value) ?? .null,
                completed: .omitted
            ),
            ifMatch: ifMatch
        )
    }

    func setTaskCompleted(_ isCompleted: Bool, task: TaskMirror) async throws -> TaskMirror {
        try await patchTask(
            taskListID: task.taskListID,
            taskID: task.id,
            body: GoogleTaskPatchDTO(
                title: nil,
                notes: .omitted,
                status: isCompleted ? .completed : .needsAction,
                due: .omitted,
                completed: isCompleted ? .value(Date()) : .null
            ),
            ifMatch: task.etag
        )
    }

    func deleteTask(taskListID: String, taskID: String, ifMatch: String? = nil) async throws {
        let encodedTaskListID = taskListID.googlePathComponentEncoded
        let encodedTaskID = taskID.googlePathComponentEncoded
        try await transport.send(
            method: "DELETE",
            path: "/tasks/v1/lists/\(encodedTaskListID)/tasks/\(encodedTaskID)",
            ifMatch: ifMatch
        )
        AppLogger.info("google task write accepted", category: .google, metadata: [
            "action": "delete",
            "taskListID": GoogleDiagnostics.redactedIdentifier(taskListID),
            "taskID": GoogleDiagnostics.redactedIdentifier(taskID),
            "hasIfMatch": String(ifMatch?.isEmpty == false)
        ])
    }

    // Hides every completed task in the list from the Tasks web/mobile UI.
    // Hidden tasks remain accessible via `showHidden=true` on list() but are
    // not returned by default.
    func clearCompletedTasks(taskListID: String) async throws {
        let encodedTaskListID = taskListID.googlePathComponentEncoded
        try await transport.send(
            method: "POST",
            path: "/tasks/v1/lists/\(encodedTaskListID)/clear"
        )
        AppLogger.info("google task write accepted", category: .google, metadata: [
            "action": "clearCompleted",
            "taskListID": GoogleDiagnostics.redactedIdentifier(taskListID)
        ])
    }

    private func patchTask(
        taskListID: String,
        taskID: String,
        body: GoogleTaskPatchDTO,
        ifMatch: String? = nil
    ) async throws -> TaskMirror {
        let encodedTaskListID = taskListID.googlePathComponentEncoded
        let encodedTaskID = taskID.googlePathComponentEncoded
        let response: GoogleTaskDTO = try await transport.request(
            method: "PATCH",
            path: "/tasks/v1/lists/\(encodedTaskListID)/tasks/\(encodedTaskID)",
            body: body,
            ifMatch: ifMatch
        )
        let mirror = response.mirror(taskListID: taskListID)
        AppLogger.info("google task write accepted", category: .google, metadata: [
            "action": body.status == nil ? "update" : "completion",
            "taskListID": GoogleDiagnostics.redactedIdentifier(taskListID),
            "taskID": GoogleDiagnostics.redactedIdentifier(taskID),
            "hasIfMatch": String(ifMatch?.isEmpty == false)
        ])
        return mirror
    }

    private static let taskListsFields = [
        "items(id,title,updated,etag)"
    ].joined(separator: ",")

    private static let tasksFields = [
        "nextPageToken",
        "items(id,title,notes,status,due,completed,deleted,hidden,parent,position,etag,updated)"
    ].joined(separator: ",")
}

// §14 — Paginated Tasks fetch result. `serverDate` carries the Date header
// from the first page so SyncScheduler can set the next `updatedMin` from
// Google's clock rather than the local one.
struct GoogleTasksPage: Sendable {
    let tasks: [TaskMirror]
    let serverDate: Date?
}

private struct GoogleTaskListsResponse: Decodable, Sendable {
    var items: [GoogleTaskListDTO]
}

private struct GoogleTaskListDTO: Decodable, Sendable {
    var id: String
    var title: String
    var updated: Date?
    var etag: String?

    var mirror: TaskListMirror {
        TaskListMirror(id: id, title: title, updatedAt: updated, etag: etag)
    }
}

private struct GoogleTasksResponse: Decodable, Sendable {
    var items: [GoogleTaskDTO]
    var nextPageToken: String?
}

private struct GoogleTaskDTO: Decodable, Sendable {
    var id: String
    var title: String
    var notes: String?
    var status: String
    var due: String?
    var completed: Date?
    var deleted: Bool?
    var hidden: Bool?
    var parent: String?
    var position: String?
    var etag: String?
    var updated: Date?

    func mirror(taskListID: String) -> TaskMirror {
        TaskMirror(
            id: id,
            taskListID: taskListID,
            parentID: parent,
            title: title,
            notes: notes ?? "",
            status: TaskStatus(rawValue: status) ?? .needsAction,
            dueDate: due.flatMap { GoogleTaskDueDateFormatter.localMidnight(from: $0) },
            completedAt: completed,
            isDeleted: deleted ?? false,
            isHidden: hidden ?? false,
            position: position,
            etag: etag,
            updatedAt: updated
        )
    }
}

private struct GoogleTaskMutationDTO: Encodable, Sendable {
    var title: String
    var notes: String?
    var due: String?
}

private struct GoogleTaskListMutationDTO: Encodable, Sendable {
    var title: String
}

private struct GoogleTaskPatchDTO: Encodable, Sendable {
    var title: String?
    var notes: NullableField<String>
    var status: TaskStatus?
    var due: NullableField<String>
    var completed: NullableField<Date>

    enum CodingKeys: String, CodingKey {
        case title
        case notes
        case status
        case due
        case completed
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encodeIfPresent(title, forKey: .title)
        try container.encodeIfPresent(status, forKey: .status)
        try container.encode(notes, forKey: .notes)
        try container.encode(due, forKey: .due)
        try container.encode(completed, forKey: .completed)
    }
}

private enum NullableField<Value: Encodable & Sendable>: Encodable, Sendable {
    case omitted
    case null
    case value(Value)

    func encode(to encoder: Encoder) throws {
        switch self {
        case .omitted:
            break
        case .null:
            var container = encoder.singleValueContainer()
            try container.encodeNil()
        case .value(let value):
            try value.encode(to: encoder)
        }
    }
}

private extension KeyedEncodingContainer {
    mutating func encode<Value: Encodable & Sendable>(
        _ value: NullableField<Value>,
        forKey key: Key
    ) throws {
        switch value {
        case .omitted:
            break
        case .null:
            try encodeNil(forKey: key)
        case .value(let wrappedValue):
            try encode(wrappedValue, forKey: key)
        }
    }
}

private extension String {
    var googlePathComponentEncoded: String {
        var allowedCharacters = CharacterSet.urlPathAllowed
        allowedCharacters.remove(charactersIn: "/?#")
        return addingPercentEncoding(withAllowedCharacters: allowedCharacters) ?? self
    }
}

// Google Tasks' `due` field is RFC3339 but the API documents it as date-only —
// "time info is ignored" and stored as the date portion in UTC. Encoding a
// Swift Date via the default .iso8601 strategy emits the UTC wall-clock time,
// so local-midnight April 19 in UTC+8 becomes "2026-04-18T16:00:00Z" and
// Google stores the task as due April 18. We instead serialise the user's
// local Y/M/D directly into the timestamp so Google's date portion matches
// what the user picked, regardless of timezone. Reads reverse the mapping:
// Google returns UTC midnight of the stored date, which we re-anchor to
// local midnight of that same Y/M/D so subsequent local-calendar comparisons
// behave predictably.
enum GoogleTaskDueDateFormatter {
    static func string(from date: Date, calendar: Calendar = .current) -> String {
        let components = calendar.dateComponents([.year, .month, .day], from: date)
        guard
            let year = components.year,
            let month = components.month,
            let day = components.day
        else {
            return "1970-01-01T00:00:00.000Z"
        }
        return String(format: "%04d-%02d-%02dT00:00:00.000Z", year, month, day)
    }

    static func localMidnight(from rfc3339: String, calendar: Calendar = .current) -> Date? {
        // Accept either full RFC3339 ("2026-04-19T00:00:00.000Z") or date-only
        // ("2026-04-19") — both occur in responses.
        let prefix = String(rfc3339.prefix(10))
        let digits = prefix.split(separator: "-")
        guard digits.count == 3,
              let year = Int(digits[0]),
              let month = Int(digits[1]),
              let day = Int(digits[2])
        else {
            return nil
        }
        var components = DateComponents()
        components.year = year
        components.month = month
        components.day = day
        components.hour = 0
        components.minute = 0
        components.second = 0
        return calendar.date(from: components)
    }
}
