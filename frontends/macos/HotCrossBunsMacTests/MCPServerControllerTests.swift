import XCTest
import Darwin
@testable import HotCrossBunsMac

@MainActor
final class MCPServerControllerTests: XCTestCase {
    func testInitializeAndToolsListJSONRPC() async throws {
        let controller = controller()

        let initialize = try await postJSON(controller, object: [
            "jsonrpc": "2.0",
            "id": 1,
            "method": "initialize",
            "params": [:]
        ])
        XCTAssertEqual(initialize.status, 200)
        let initializeBody = try decode(initialize.body)
        XCTAssertEqual((initializeBody["result"] as? [String: Any])?["protocolVersion"] as? String, "2025-06-18")

        let tools = try await postJSON(controller, object: [
            "jsonrpc": "2.0",
            "id": "tools",
            "method": "tools/list",
            "params": [:]
        ])
        let toolsBody = try decode(tools.body)
        let result = try XCTUnwrap(toolsBody["result"] as? [String: Any])
        let list = try XCTUnwrap(result["tools"] as? [[String: Any]])
        XCTAssertTrue(list.contains { $0["name"] as? String == "hcb_search" })
        XCTAssertTrue(list.contains { $0["name"] as? String == "hcb_delete_event" })
    }

    func testToolCallAuthOriginAndMalformedRequests() async throws {
        let controller = controller()

        let unauthorized = await controller.handleHTTPRequest(
            data: httpRequest(headers: ["Authorization": "Bearer wrong"], body: rpc("tools/list")),
            remoteIsLocal: true
        )
        XCTAssertEqual(unauthorized.status, 401)

        let badOrigin = await controller.handleHTTPRequest(
            data: httpRequest(headers: ["Authorization": "Bearer test-token", "Origin": "https://example.com"], body: rpc("tools/list")),
            remoteIsLocal: true
        )
        XCTAssertEqual(badOrigin.status, 403)

        let remote = await controller.handleHTTPRequest(
            data: httpRequest(headers: ["Authorization": "Bearer test-token"], body: rpc("tools/list")),
            remoteIsLocal: false
        )
        XCTAssertEqual(remote.status, 403)

        let malformed = await controller.handleJSONRPCBody(Data("{".utf8))
        XCTAssertEqual(malformed.status, 400)
    }

    func testToolsCallWrapsStructuredContentAndInvalidMethodErrors() async throws {
        let controller = controller()
        let response = try await postJSON(controller, object: [
            "jsonrpc": "2.0",
            "id": "call",
            "method": "tools/call",
            "params": [
                "name": "hcb_get_task",
                "arguments": ["id": "task-1"]
            ]
        ])
        XCTAssertEqual(response.status, 200)
        let body = try decode(response.body)
        let result = try XCTUnwrap(body["result"] as? [String: Any])
        XCTAssertEqual(result["isError"] as? Bool, false)
        XCTAssertNotNil(result["structuredContent"])

        let errorResponse = try await postJSON(controller, object: [
            "jsonrpc": "2.0",
            "id": "bad",
            "method": "missing/method",
            "params": [:]
        ])
        let errorBody = try decode(errorResponse.body)
        XCTAssertNotNil(errorBody["error"])
    }

    func testRequestSizeLimitsRejectBeforeAuthAndJSONParsing() async throws {
        let controller = controller()

        let oversizedHeader = await controller.handleHTTPRequest(
            data: rawHTTPRequest(lines: [
                "POST /mcp HTTP/1.1",
                "Host: \(String(repeating: "a", count: MCPServerController.maxHTTPHeaderBytes))",
                "Content-Length: 2"
            ], body: Data("{}".utf8)),
            remoteIsLocal: true
        )
        XCTAssertEqual(oversizedHeader.status, 413)

        let oversizedDeclaredBody = await controller.handleHTTPRequest(
            data: rawHTTPRequest(lines: [
                "POST /mcp HTTP/1.1",
                "Host: 127.0.0.1",
                "Content-Length: \(MCPServerController.maxHTTPBodyBytes + 1)"
            ], body: Data()),
            remoteIsLocal: true
        )
        XCTAssertEqual(oversizedDeclaredBody.status, 413)

        let oversizedActualBody = await controller.handleHTTPRequest(
            data: httpRequest(headers: [:], body: Data(repeating: 65, count: MCPServerController.maxHTTPBodyBytes + 1)),
            remoteIsLocal: true
        )
        XCTAssertEqual(oversizedActualBody.status, 413)

        let oversizedJSONRPCBody = await controller.handleJSONRPCBody(Data(repeating: 65, count: MCPServerController.maxHTTPBodyBytes + 1))
        XCTAssertEqual(oversizedJSONRPCBody.status, 413)
    }

    func testMalformedContentLengthRequestsAreRejected() async throws {
        let controller = controller()

        let badLength = await controller.handleHTTPRequest(
            data: rawHTTPRequest(lines: [
                "POST /mcp HTTP/1.1",
                "Host: 127.0.0.1",
                "Content-Length: abc",
                "Authorization: Bearer test-token"
            ], body: Data()),
            remoteIsLocal: true
        )
        XCTAssertEqual(badLength.status, 400)

        let duplicateLength = await controller.handleHTTPRequest(
            data: rawHTTPRequest(lines: [
                "POST /mcp HTTP/1.1",
                "Host: 127.0.0.1",
                "Content-Length: 0",
                "Content-Length: 0",
                "Authorization: Bearer test-token"
            ], body: Data()),
            remoteIsLocal: true
        )
        XCTAssertEqual(duplicateLength.status, 400)

        let duplicateAuthorization = await controller.handleHTTPRequest(
            data: rawHTTPRequest(lines: [
                "POST /mcp HTTP/1.1",
                "Host: 127.0.0.1",
                "Content-Length: 0",
                "Authorization: Bearer test-token",
                "Authorization: Bearer test-token"
            ], body: Data()),
            remoteIsLocal: true
        )
        XCTAssertEqual(duplicateAuthorization.status, 400)

        var requestWithExtraBytes = httpRequest(headers: ["Authorization": "Bearer test-token"], body: rpc("tools/list"))
        requestWithExtraBytes.append(Data("GET /mcp HTTP/1.1\r\n\r\n".utf8))
        let extraBytes = await controller.handleHTTPRequest(
            data: requestWithExtraBytes,
            remoteIsLocal: true
        )
        XCTAssertEqual(extraBytes.status, 400)
    }

    func testRateLimitAppliesPerClientAndPublishesActivity() async throws {
        var activities: [MCPActivityEntry] = []
        let controller = MCPServerController(
            toolService: HCBToolService(model: AppModel.preview),
            tokenProvider: { "test-token" },
            rateLimitConfiguration: MCPRateLimitConfiguration(maxRequests: 2, windowSeconds: 60),
            onStatus: { _ in },
            onActivity: { activity in
                activities.append(activity)
            }
        )

        let request = httpRequest(headers: ["Authorization": "Bearer test-token"], body: rpc("tools/list"))
        let first = await controller.handleHTTPRequest(data: request, remoteIsLocal: true, now: Date(timeIntervalSince1970: 10), clientKey: "client-a")
        let second = await controller.handleHTTPRequest(data: request, remoteIsLocal: true, now: Date(timeIntervalSince1970: 11), clientKey: "client-a")
        let third = await controller.handleHTTPRequest(data: request, remoteIsLocal: true, now: Date(timeIntervalSince1970: 12), clientKey: "client-a")
        let otherClient = await controller.handleHTTPRequest(data: request, remoteIsLocal: true, now: Date(timeIntervalSince1970: 12), clientKey: "client-b")

        XCTAssertEqual(first.status, 200)
        XCTAssertEqual(second.status, 200)
        XCTAssertEqual(third.status, 429)
        XCTAssertEqual(otherClient.status, 200)
        await Task.yield()
        XCTAssertTrue(activities.contains { $0.outcome == .rateLimited && $0.detail == "Request limit reached" })
    }

    func testWriteCallsEmitAuditMetadataWithoutArgumentValues() async throws {
        let audit = MCPAuditCapture()
        var activities: [MCPActivityEntry] = []
        let model = AppModel.preview
        var settings = model.settings
        settings.mcpPermissionMode = .confirmWrites
        settings.cloudSyncTargets = []
        model.updateSettings(settings)
        let controller = MCPServerController(
            toolService: HCBToolService(model: model),
            tokenProvider: { "test-token" },
            onStatus: { _ in },
            onActivity: { activity in
                activities.append(activity)
            },
            auditRecorder: { entry, metadata in
                await audit.record(entry: entry, metadata: metadata)
            }
        )

        let response = try await postJSON(controller, object: [
            "jsonrpc": "2.0",
            "id": "dry-run",
            "method": "tools/call",
            "params": [
                "name": "hcb_create_note",
                "arguments": [
                    "title": "Private launch note",
                    "notes": "Do not write this literal to audit metadata.",
                    "dryRun": true
                ]
            ]
        ], headers: [
            "Authorization": "Bearer test-token",
            "User-Agent": "MCPTest/1.0"
        ])

        XCTAssertEqual(response.status, 200)
        await Task.yield()
        XCTAssertTrue(activities.contains { $0.toolName == "hcb_create_note" && $0.outcome == .dryRun })

        let records = await audit.records()
        let record = try XCTUnwrap(records.first)
        XCTAssertEqual(record.entry.toolName, "hcb_create_note")
        XCTAssertEqual(record.metadata["client"], "MCPTest/1.0")
        XCTAssertEqual(record.metadata["dryRunRequested"], "true")
        XCTAssertEqual(record.metadata["confirmationIssued"], "true")
        XCTAssertEqual(record.metadata["argumentKeys"], "dryRun,notes,title")
        XCTAssertFalse(record.metadata.values.contains { $0.contains("Private launch note") })
        XCTAssertFalse(record.metadata.values.contains { $0.contains("Do not write this literal") })
    }

    func testLoopbackListenerAcceptsAuthorizedJSONRPC() async throws {
        let port = try freeLocalPort()
        let running = expectation(description: "MCP server starts")
        let controller = MCPServerController(
            toolService: HCBToolService(model: AppModel.preview),
            tokenProvider: { "test-token" },
            onStatus: { status in
                if status == .running(port: port) {
                    running.fulfill()
                }
            }
        )
        controller.start(port: port)
        defer { controller.stop() }
        await fulfillment(of: [running], timeout: 5)

        var request = URLRequest(url: try XCTUnwrap(URL(string: "http://127.0.0.1:\(port)/mcp")))
        request.httpMethod = "POST"
        request.setValue("Bearer test-token", forHTTPHeaderField: "Authorization")
        request.httpBody = try JSONSerialization.data(withJSONObject: [
            "jsonrpc": "2.0",
            "id": "tools",
            "method": "tools/list",
            "params": [:]
        ])

        let (data, response) = try await URLSession.shared.data(for: request)
        XCTAssertEqual((response as? HTTPURLResponse)?.statusCode, 200)
        let body = try decode(data)
        let result = try XCTUnwrap(body["result"] as? [String: Any])
        let tools = try XCTUnwrap(result["tools"] as? [[String: Any]])
        XCTAssertTrue(tools.contains { $0["name"] as? String == "hcb_create_event" })
    }

    private func controller() -> MCPServerController {
        let service = HCBToolService(model: AppModel.preview)
        return MCPServerController(
            toolService: service,
            tokenProvider: { "test-token" },
            onStatus: { _ in }
        )
    }

    private func postJSON(
        _ controller: MCPServerController,
        object: [String: Any],
        headers: [String: String] = ["Authorization": "Bearer test-token"]
    ) async throws -> HTTPResponse {
        let body = try JSONSerialization.data(withJSONObject: object)
        return await controller.handleHTTPRequest(
            data: httpRequest(headers: headers, body: body),
            remoteIsLocal: true
        )
    }

    private func rpc(_ method: String) -> Data {
        (try? JSONSerialization.data(withJSONObject: [
            "jsonrpc": "2.0",
            "id": "id",
            "method": method,
            "params": [:]
        ])) ?? Data()
    }

    private func httpRequest(headers: [String: String], body: Data) -> Data {
        var lines = [
            "POST /mcp HTTP/1.1",
            "Host: 127.0.0.1",
            "Content-Length: \(body.count)"
        ]
        for (key, value) in headers {
            lines.append("\(key): \(value)")
        }
        lines.append("")
        lines.append("")
        var data = Data(lines.joined(separator: "\r\n").utf8)
        data.append(body)
        return data
    }

    private func rawHTTPRequest(lines: [String], body: Data) -> Data {
        var lines = lines
        lines.append("")
        lines.append("")
        var data = Data(lines.joined(separator: "\r\n").utf8)
        data.append(body)
        return data
    }

    private func decode(_ data: Data) throws -> [String: Any] {
        try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
    }

    private func freeLocalPort() throws -> Int {
        let descriptor = Darwin.socket(AF_INET, SOCK_STREAM, 0)
        XCTAssertGreaterThanOrEqual(descriptor, 0)
        defer { Darwin.close(descriptor) }

        var address = sockaddr_in()
        address.sin_len = UInt8(MemoryLayout<sockaddr_in>.size)
        address.sin_family = sa_family_t(AF_INET)
        address.sin_port = 0
        address.sin_addr = in_addr(s_addr: in_addr_t(INADDR_LOOPBACK).bigEndian)

        let bindStatus = withUnsafePointer(to: &address) { pointer in
            pointer.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                Darwin.bind(descriptor, $0, socklen_t(MemoryLayout<sockaddr_in>.size))
            }
        }
        XCTAssertEqual(bindStatus, 0)

        var boundAddress = sockaddr_in()
        var length = socklen_t(MemoryLayout<sockaddr_in>.size)
        let nameStatus = withUnsafeMutablePointer(to: &boundAddress) { pointer in
            pointer.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                Darwin.getsockname(descriptor, $0, &length)
            }
        }
        XCTAssertEqual(nameStatus, 0)
        return Int(UInt16(bigEndian: boundAddress.sin_port))
    }
}

private actor MCPAuditCapture {
    private var stored: [(entry: MCPActivityEntry, metadata: [String: String])] = []

    func record(entry: MCPActivityEntry, metadata: [String: String]) {
        stored.append((entry, metadata))
    }

    func records() -> [(entry: MCPActivityEntry, metadata: [String: String])] {
        stored
    }
}
