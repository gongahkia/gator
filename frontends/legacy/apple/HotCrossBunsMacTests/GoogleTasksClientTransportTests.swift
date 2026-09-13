import XCTest
@testable import HotCrossBunsMac

// Verifies that GoogleTasksClient assembles the exact HTTP request that
// Google expects: method, path, query items, JSON body shape, and
// If-Match headers for conditional writes. MockURLProtocol captures the
// real URLRequest that would be sent to the network.
final class GoogleTasksClientTransportTests: XCTestCase {
    private var client: GoogleTasksClient!

    override func setUp() {
        super.setUp()
        MockURLProtocol.reset()
        GoogleDiagnostics.setRawPayloadLoggingEnabled(false)
        AppLogger.shared.clearInMemoryEntries()
        let transport = GoogleAPITransport(
            baseURL: URL(string: "https://www.googleapis.com")!,
            tokenProvider: StaticAccessTokenProvider(token: "test-token"),
            urlSession: MockURLProtocol.testSession()
        )
        client = GoogleTasksClient(transport: transport)
    }

    override func tearDown() {
        AppLogger.shared.flush()
        AppLogger.shared.clearInMemoryEntries()
        GoogleDiagnostics.setRawPayloadLoggingEnabled(false)
        MockURLProtocol.reset()
        super.tearDown()
    }

    func testListTaskListsRequestsPartialResponseFields() async throws {
        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: "HTTP/1.1",
                headerFields: ["Content-Type": "application/json"]
            )!
            let queryItems = URLComponents(url: request.url!, resolvingAgainstBaseURL: false)?.queryItems ?? []
            let query = Dictionary(uniqueKeysWithValues: queryItems.map { ($0.name, $0.value ?? "") })
            XCTAssertEqual(request.url?.path, "/tasks/v1/users/@me/lists")
            XCTAssertEqual(query["fields"], "items(id,title,updated,etag)")
            return (response, #"{"items":[]}"#.data(using: .utf8)!)
        }

        let lists = try await client.listTaskLists()
        XCTAssertTrue(lists.isEmpty)
    }

    func testInsertTaskPostsJSONBodyWithAuthHeader() async throws {
        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: "HTTP/1.1",
                headerFields: ["Content-Type": "application/json"]
            )!
            let body = #"{"id":"srv-1","title":"Task","status":"needsAction","etag":"etag-1"}"#
            return (response, body.data(using: .utf8)!)
        }

        let task = try await client.insertTask(
            taskListID: "list-xyz",
            title: "Task",
            notes: "Body",
            dueDate: nil
        )
        XCTAssertEqual(task.id, "srv-1")
        XCTAssertEqual(task.etag, "etag-1")

        let captured = try XCTUnwrap(MockURLProtocol.capturedRequests.first)
        XCTAssertEqual(captured.httpMethod, "POST")
        XCTAssertEqual(captured.url?.path, "/tasks/v1/lists/list-xyz/tasks")
        XCTAssertEqual(captured.value(forHTTPHeaderField: "Authorization"), "Bearer test-token")
        XCTAssertEqual(captured.value(forHTTPHeaderField: "Content-Type"), "application/json")
        XCTAssertNil(captured.value(forHTTPHeaderField: "If-Match"))
    }

    func testUpdateTaskSendsIfMatchHeader() async throws {
        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: nil
            )!
            let body = #"{"id":"task-1","title":"Renamed","status":"needsAction","etag":"etag-2"}"#
            return (response, body.data(using: .utf8)!)
        }

        _ = try await client.updateTask(
            taskListID: "list-1",
            taskID: "task-1",
            title: "Renamed",
            notes: "New",
            dueDate: nil,
            ifMatch: "etag-prior"
        )

        let captured = try XCTUnwrap(MockURLProtocol.capturedRequests.first)
        XCTAssertEqual(captured.httpMethod, "PATCH")
        XCTAssertEqual(captured.url?.path, "/tasks/v1/lists/list-1/tasks/task-1")
        XCTAssertEqual(captured.value(forHTTPHeaderField: "If-Match"), "etag-prior")
    }

    func testDeleteTaskSendsIfMatchHeader() async throws {
        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(url: request.url!, statusCode: 204, httpVersion: nil, headerFields: nil)!
            return (response, Data())
        }

        try await client.deleteTask(taskListID: "list-1", taskID: "task-1", ifMatch: "etag-delete")

        let captured = try XCTUnwrap(MockURLProtocol.capturedRequests.first)
        XCTAssertEqual(captured.httpMethod, "DELETE")
        XCTAssertEqual(captured.url?.path, "/tasks/v1/lists/list-1/tasks/task-1")
        XCTAssertEqual(captured.value(forHTTPHeaderField: "If-Match"), "etag-delete")
    }

    func testPreconditionFailedBubblesAsSpecificError() async throws {
        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(url: request.url!, statusCode: 412, httpVersion: nil, headerFields: nil)!
            return (response, Data())
        }

        do {
            _ = try await client.updateTask(
                taskListID: "l", taskID: "t",
                title: "x", notes: "", dueDate: nil, ifMatch: "stale-etag"
            )
            XCTFail("Expected preconditionFailed")
        } catch let error as GoogleAPIError {
            XCTAssertEqual(error, .preconditionFailed)
        } catch {
            XCTFail("Unexpected error: \(error)")
        }
    }

    func testClearCompletedTasksPostsToClearEndpoint() async throws {
        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(url: request.url!, statusCode: 204, httpVersion: nil, headerFields: nil)!
            return (response, Data())
        }

        try await client.clearCompletedTasks(taskListID: "list-1")

        let captured = try XCTUnwrap(MockURLProtocol.capturedRequests.first)
        XCTAssertEqual(captured.httpMethod, "POST")
        XCTAssertEqual(captured.url?.path, "/tasks/v1/lists/list-1/clear")
    }

    // §14 — listTasks should carry the first-page Date header back to
    // SyncScheduler so the incremental-sync watermark is derived from
    // Google's clock rather than the local one. Missing header → nil.
    func testListTasksCapturesServerDateHeader() async throws {
        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: "HTTP/1.1",
                headerFields: [
                    "Content-Type": "application/json",
                    "Date": "Sun, 19 Apr 2026 23:59:12 GMT"
                ]
            )!
            let body = #"{"items":[]}"#
            return (response, body.data(using: .utf8)!)
        }

        let page = try await client.listTasks(taskListID: "list-1", updatedMin: nil)
        XCTAssertEqual(page.tasks.count, 0)
        let serverDate = try XCTUnwrap(page.serverDate)
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = TimeZone(secondsFromGMT: 0)!
        let comps = cal.dateComponents([.year, .month, .day, .hour, .minute, .second], from: serverDate)
        XCTAssertEqual(comps.year, 2026)
        XCTAssertEqual(comps.month, 4)
        XCTAssertEqual(comps.day, 19)
        XCTAssertEqual(comps.hour, 23)
        XCTAssertEqual(comps.minute, 59)
        XCTAssertEqual(comps.second, 12)
    }

    func testListTasksReturnsNilServerDateWhenHeaderMissing() async throws {
        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: "HTTP/1.1",
                headerFields: ["Content-Type": "application/json"]
            )!
            return (response, #"{"items":[]}"#.data(using: .utf8)!)
        }

        let page = try await client.listTasks(taskListID: "list-1", updatedMin: nil)
        XCTAssertNil(page.serverDate)
    }

    func testListTasksUsesCompletedMinForFullSync() async throws {
        let completedMin = Date(timeIntervalSince1970: 1_713_900_000)
        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: "HTTP/1.1",
                headerFields: ["Content-Type": "application/json"]
            )!
            let queryItems = URLComponents(url: request.url!, resolvingAgainstBaseURL: false)?.queryItems ?? []
            let query = Dictionary(uniqueKeysWithValues: queryItems.map { ($0.name, $0.value ?? "") })
            XCTAssertNil(query["updatedMin"])
            XCTAssertEqual(query["completedMin"], ISO8601DateFormatter.google.string(from: completedMin))
            XCTAssertTrue(query["fields"]?.contains("nextPageToken") == true)
            XCTAssertTrue(query["fields"]?.contains("items(id,title,notes,status,due,completed,deleted,hidden,parent,position,etag,updated)") == true)
            return (response, #"{"items":[]}"#.data(using: .utf8)!)
        }

        let page = try await client.listTasks(taskListID: "list-1", updatedMin: nil, completedMin: completedMin)
        XCTAssertEqual(page.tasks.count, 0)
    }

    func testListTasksUpdatedMinWinsOverCompletedMinForIncrementalSync() async throws {
        let updatedMin = Date(timeIntervalSince1970: 1_713_900_000)
        let completedMin = Date(timeIntervalSince1970: 1_700_000_000)
        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: "HTTP/1.1",
                headerFields: ["Content-Type": "application/json"]
            )!
            let queryItems = URLComponents(url: request.url!, resolvingAgainstBaseURL: false)?.queryItems ?? []
            let query = Dictionary(uniqueKeysWithValues: queryItems.map { ($0.name, $0.value ?? "") })
            XCTAssertEqual(query["updatedMin"], ISO8601DateFormatter.google.string(from: updatedMin))
            XCTAssertNil(query["completedMin"])
            return (response, #"{"items":[]}"#.data(using: .utf8)!)
        }

        let page = try await client.listTasks(taskListID: "list-1", updatedMin: updatedMin, completedMin: completedMin)
        XCTAssertEqual(page.tasks.count, 0)
    }

    func testTaskDueDateSerializedAsLocalDateString() async throws {
        var captured: Data?
        MockURLProtocol.requestHandler = { request in
            if let stream = request.httpBodyStream {
                var data = Data()
                stream.open()
                let bufferSize = 1024
                let buffer = UnsafeMutablePointer<UInt8>.allocate(capacity: bufferSize)
                while stream.hasBytesAvailable {
                    let read = stream.read(buffer, maxLength: bufferSize)
                    if read <= 0 { break }
                    data.append(buffer, count: read)
                }
                buffer.deallocate()
                stream.close()
                captured = data
            } else {
                captured = request.httpBody
            }
            let response = HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            let body = #"{"id":"srv-1","title":"T","status":"needsAction"}"#
            return (response, body.data(using: .utf8)!)
        }

        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = TimeZone(secondsFromGMT: 8 * 3600)!
        var comps = DateComponents()
        comps.year = 2026; comps.month = 4; comps.day = 19
        let localMidnight = cal.date(from: comps)!

        _ = try await client.insertTask(
            taskListID: "l",
            title: "T",
            notes: "",
            dueDate: localMidnight
        )

        let data = try XCTUnwrap(captured)
        let string = String(data: data, encoding: .utf8) ?? ""
        XCTAssertTrue(
            string.contains("\"due\":\"2026-04-"),
            "Body should encode due as a date-only yyyy-MM-dd string; got \(string)"
        )
    }

    func testGoogleTransportLogsSanitizedRequestAndResponseSummariesByDefault() async throws {
        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            let body = #"{"id":"srv-1","title":"Sensitive Task","notes":"Secret Body","status":"needsAction"}"#
            return (response, Data(body.utf8))
        }

        _ = try await client.insertTask(
            taskListID: "list-xyz",
            title: "Sensitive Task",
            notes: "Secret Body",
            dueDate: nil
        )

        let log = googleLogText()
        XCTAssertTrue(log.contains("google request start"))
        XCTAssertTrue(log.contains("google request succeeded"))
        XCTAssertTrue(log.contains("method=POST"))
        XCTAssertTrue(log.contains("endpoint=tasks.tasks"))
        XCTAssertTrue(log.contains("/tasks/v1/lists/<list:"))
        XCTAssertTrue(log.contains("queryNames=none"))
        XCTAssertTrue(log.contains("status=200"))
        XCTAssertTrue(log.contains("requestFields=notes,title"))
        XCTAssertTrue(log.contains("responseFields=id,notes,status,title"))
        XCTAssertFalse(log.contains("Bearer"))
        XCTAssertFalse(log.contains("test-token"))
        XCTAssertFalse(log.contains("Sensitive Task"))
        XCTAssertFalse(log.contains("Secret Body"))
        XCTAssertFalse(log.contains("requestBodySnippet"))
        XCTAssertFalse(log.contains("responseBodySnippet"))
    }

    func testGoogleTransportRawModeAddsFieldRedactedLocalPayloadSnippets() async throws {
        GoogleDiagnostics.setRawPayloadLoggingEnabled(true)
        XCTAssertTrue(GoogleDiagnostics.isRawPayloadLoggingEnabled)
        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            let body = #"{"id":"srv-raw","title":"Raw Task","status":"needsAction"}"#
            return (response, Data(body.utf8))
        }

        _ = try await client.insertTask(
            taskListID: "list-raw",
            title: "Raw Task",
            notes: "Raw Notes",
            dueDate: nil
        )

        let log = googleLogText()
        XCTAssertTrue(log.contains("requestBodySnippet="), log)
        XCTAssertTrue(log.contains("responseBodySnippet="), log)
        XCTAssertTrue(log.contains(#""title":"<redacted>""#), log)
        XCTAssertTrue(log.contains(#""notes":"<redacted>""#), log)
        XCTAssertTrue(log.contains(#""status":"needsAction""#), log)
        XCTAssertTrue(log.contains("requestBodySnippetTruncated=false"), log)
        XCTAssertTrue(log.contains("responseBodySnippetTruncated=false"), log)
        XCTAssertFalse(log.contains("Raw Task"), log)
        XCTAssertFalse(log.contains("Raw Notes"), log)
        XCTAssertFalse(log.contains("Bearer test-token"))
        XCTAssertFalse(log.contains("test-token"))
    }

    func testGoogleTransportFailureLogsStatusWithoutRawResponseBodyByDefault() async throws {
        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 500,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            let body = #"{"error":"Server says Secret Body"}"#
            return (response, Data(body.utf8))
        }

        do {
            try await client.deleteTask(taskListID: "list-1", taskID: "task-1")
            XCTFail("Expected HTTP failure")
        } catch let error as GoogleAPIError {
            XCTAssertEqual(error, .httpStatus(500, #"{"error":"Server says Secret Body"}"#))
        }

        let log = googleLogText()
        XCTAssertTrue(log.contains("google request failed"))
        XCTAssertTrue(log.contains("status=500"))
        XCTAssertTrue(log.contains("responseBytes="))
        XCTAssertFalse(log.contains("Server says Secret Body"))
        XCTAssertFalse(log.contains("responseBodySnippet"))
    }

    private func googleLogText() -> String {
        AppLogger.shared.flush()
        return AppLogger.shared
            .recentEntries(limit: 100, minimumLevel: .debug)
            .filter { $0.category == .google }
            .map { $0.formattedLine() }
            .joined(separator: "\n")
    }
}
