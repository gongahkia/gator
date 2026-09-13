import Foundation
import Network

enum MCPServerStatus: Equatable {
    case stopped
    case running(port: Int)
    case failed(String)

    var title: String {
        switch self {
        case .stopped:
            "Stopped"
        case .running(let port):
            "Running on 127.0.0.1:\(port)"
        case .failed:
            "Failed"
        }
    }

    var detail: String {
        switch self {
        case .stopped:
            "The local MCP server is disabled."
        case .running(let port):
            "MCP endpoint: http://127.0.0.1:\(port)/mcp"
        case .failed(let message):
            message
        }
    }
}

enum MCPActivityOutcome: String, Sendable {
    case succeeded
    case dryRun
    case applied
    case denied
    case confirmationRequired
    case invalid
    case failed
    case rateLimited

    var title: String {
        switch self {
        case .succeeded: "Succeeded"
        case .dryRun: "Dry-run"
        case .applied: "Applied"
        case .denied: "Denied"
        case .confirmationRequired: "Confirm"
        case .invalid: "Invalid"
        case .failed: "Failed"
        case .rateLimited: "Limited"
        }
    }

    var symbolName: String {
        switch self {
        case .succeeded: "checkmark.circle"
        case .dryRun: "eye"
        case .applied: "checkmark.seal"
        case .denied: "hand.raised"
        case .confirmationRequired: "key"
        case .invalid: "exclamationmark.triangle"
        case .failed: "xmark.octagon"
        case .rateLimited: "speedometer"
        }
    }
}

struct MCPActivityEntry: Identifiable, Equatable, Sendable {
    let id: UUID
    let timestamp: Date
    let client: String
    let method: String
    let toolName: String?
    let outcome: MCPActivityOutcome
    let detail: String
    let isWrite: Bool

    init(
        id: UUID = UUID(),
        timestamp: Date = Date(),
        client: String,
        method: String,
        toolName: String?,
        outcome: MCPActivityOutcome,
        detail: String,
        isWrite: Bool
    ) {
        self.id = id
        self.timestamp = timestamp
        self.client = client
        self.method = method
        self.toolName = toolName
        self.outcome = outcome
        self.detail = detail
        self.isWrite = isWrite
    }

    var title: String {
        if let toolName {
            return toolName
        }
        return method
    }
}

struct MCPRateLimitConfiguration: Equatable, Sendable {
    var maxRequests: Int
    var windowSeconds: TimeInterval

    static let `default` = MCPRateLimitConfiguration(maxRequests: 120, windowSeconds: 60)
}

private final class MCPRateLimiter: @unchecked Sendable {
    private let configuration: MCPRateLimitConfiguration
    private let lock = NSLock()
    private var timestampsByClient: [String: [Date]] = [:]

    init(configuration: MCPRateLimitConfiguration) {
        self.configuration = configuration
    }

    func allows(clientKey: String, now: Date) -> Bool {
        lock.lock()
        defer { lock.unlock() }

        let cutoff = now.addingTimeInterval(-configuration.windowSeconds)
        var timestamps = (timestampsByClient[clientKey] ?? []).filter { $0 >= cutoff }
        guard timestamps.count < configuration.maxRequests else {
            timestampsByClient[clientKey] = timestamps
            return false
        }
        timestamps.append(now)
        timestampsByClient[clientKey] = timestamps
        return true
    }
}

final class MCPServerController {
    static let maxHTTPHeaderBytes = 16 * 1024
    static let maxHTTPBodyBytes = 1 * 1024 * 1024
    static let maxHTTPRequestBytes = maxHTTPHeaderBytes + maxHTTPBodyBytes + 4

    private let toolService: HCBToolService
    private let tokenProvider: () throws -> String
    private let onStatus: @MainActor (MCPServerStatus) -> Void
    private let onActivity: @MainActor (MCPActivityEntry) -> Void
    private let auditRecorder: @Sendable (MCPActivityEntry, [String: String]) async -> Void
    private let rateLimitConfiguration: MCPRateLimitConfiguration
    private let rateLimiter: MCPRateLimiter
    private let queue = DispatchQueue(label: "com.gongahkia.hotcrossbuns.mcp-server")
    private var listener: NWListener?
    private var activePort: Int?

    init(
        toolService: HCBToolService,
        tokenProvider: @escaping () throws -> String = HCBMCPTokenStore.loadOrCreateToken,
        rateLimitConfiguration: MCPRateLimitConfiguration = .default,
        onStatus: @escaping @MainActor (MCPServerStatus) -> Void,
        onActivity: @escaping @MainActor (MCPActivityEntry) -> Void = { _ in },
        auditRecorder: @escaping @Sendable (MCPActivityEntry, [String: String]) async -> Void = { entry, metadata in
            MutationAuditLog.shared.record(
                kind: "mcp.write.\(entry.outcome.rawValue)",
                resourceID: entry.toolName ?? entry.method,
                summary: "MCP \(entry.title) \(entry.outcome.title.lowercased())",
                metadata: metadata
            )
        }
    ) {
        self.toolService = toolService
        self.tokenProvider = tokenProvider
        self.rateLimitConfiguration = rateLimitConfiguration
        self.rateLimiter = MCPRateLimiter(configuration: rateLimitConfiguration)
        self.onStatus = onStatus
        self.onActivity = onActivity
        self.auditRecorder = auditRecorder
    }

    func start(port: Int) {
        let port = max(1, min(65535, port))
        if activePort == port, listener != nil {
            publish(.running(port: port))
            return
        }
        stop()

        do {
            _ = try tokenProvider()
            let parameters = NWParameters.tcp
            parameters.allowLocalEndpointReuse = true
            guard let nwPort = NWEndpoint.Port(rawValue: UInt16(port)) else {
                publish(.failed("Invalid MCP port \(port)."))
                return
            }
            parameters.requiredLocalEndpoint = .hostPort(host: .ipv4(.loopback), port: nwPort)
            let listener = try NWListener(using: parameters)
            listener.service = nil
            listener.stateUpdateHandler = { [weak self] state in
                guard let self else { return }
                switch state {
                case .ready:
                    self.publish(.running(port: port))
                case .failed(let error):
                    self.publish(.failed("MCP server failed: \(error.localizedDescription)"))
                    self.stop()
                case .cancelled:
                    self.publish(.stopped)
                default:
                    break
                }
            }
            listener.newConnectionHandler = { [weak self] connection in
                self?.handle(connection)
            }
            self.listener = listener
            self.activePort = port
            listener.start(queue: queue)
        } catch {
            publish(.failed("Could not start MCP server: \(error.localizedDescription)"))
        }
    }

    func stop() {
        listener?.cancel()
        listener = nil
        activePort = nil
        publish(.stopped)
    }

    func handleHTTPRequest(
        data: Data,
        remoteIsLocal: Bool,
        now: Date = Date(),
        clientDescription: String = "Local client",
        clientKey: String = "loopback"
    ) async -> HTTPResponse {
        guard remoteIsLocal else {
            return .plain(status: 403, body: "Forbidden")
        }
        let request: HTTPRequest
        switch HTTPRequest.parse(data: data) {
        case .complete(let parsed):
            request = parsed
        case .tooLarge:
            return .plain(status: 413, body: "Payload Too Large")
        case .incomplete, .malformed:
            return .plain(status: 400, body: "Bad Request")
        }
        guard request.path == "/mcp" else {
            return .plain(status: 404, body: "Not Found")
        }
        let displayClient = Self.clientDescription(for: request.headers, fallback: clientDescription)
        guard rateLimiter.allows(clientKey: clientKey, now: now) else {
            let activity = MCPActivityEntry(
                timestamp: now,
                client: displayClient,
                method: "HTTP",
                toolName: nil,
                outcome: .rateLimited,
                detail: "Request limit reached",
                isWrite: false
            )
            publish(activity)
            AppLogger.warn("mcp request rate limited", category: .mcp, metadata: ["client": displayClient])
            return .plain(
                status: 429,
                body: "Too Many Requests",
                headers: ["Retry-After": String(Int(rateLimitConfiguration.windowSeconds.rounded(.up)))]
            )
        }
        guard request.method == "POST" else {
            return .plain(status: 405, body: "MCP Streamable HTTP GET/SSE is not implemented in Hot Cross Buns v1.")
        }
        guard originIsAllowed(request.headers["origin"]) else {
            return .plain(status: 403, body: "Forbidden origin")
        }
        do {
            let token = try tokenProvider()
            guard Self.authorizationMatches(request.headers["authorization"], token: token) else {
                return .plain(status: 401, body: "Unauthorized", headers: ["WWW-Authenticate": "Bearer"])
            }
        } catch {
            return .plain(status: 500, body: "MCP token unavailable")
        }
        return await handleJSONRPCBody(request.body, now: now, clientDescription: displayClient)
    }

    func handleJSONRPCBody(
        _ body: Data,
        now: Date = Date(),
        clientDescription: String = "Local client"
    ) async -> HTTPResponse {
        guard body.count <= Self.maxHTTPBodyBytes else {
            return .plain(status: 413, body: "Payload Too Large")
        }
        guard
            let object = try? JSONSerialization.jsonObject(with: body),
            let request = object as? [String: Any],
            request["jsonrpc"] as? String == "2.0",
            let method = request["method"] as? String
        else {
            return jsonRPCError(id: nil, code: -32700, message: "Parse error", status: 400)
        }

        let id = request["id"]
        let params = request["params"] as? [String: Any] ?? [:]
        let toolName = Self.toolName(method: method, params: params)
        let isWrite = toolName.map(HCBToolService.isWriteTool) ?? false
        if id == nil {
            if method == "notifications/initialized" {
                return .empty(status: 202)
            }
            return .empty(status: 202)
        }

        do {
            let result = try await handle(method: method, params: params)
            let activity = Self.activityEntry(
                timestamp: now,
                client: clientDescription,
                method: method,
                toolName: toolName,
                isWrite: isWrite,
                result: result
            )
            publish(activity)
            await recordWriteAuditIfNeeded(activity, params: params, result: result, error: nil)
            return jsonRPCResult(id: id, result: result)
        } catch let error as HCBToolError {
            let activity = Self.activityEntry(
                timestamp: now,
                client: clientDescription,
                method: method,
                toolName: toolName,
                isWrite: isWrite,
                error: error
            )
            publish(activity)
            await recordWriteAuditIfNeeded(activity, params: params, result: nil, error: error)
            return jsonRPCError(
                id: id,
                code: errorCode(for: error),
                message: error.localizedDescription,
                data: error.confirmationId.map { ["confirmationId": $0] },
                status: 200
            )
        } catch {
            let activity = MCPActivityEntry(
                timestamp: now,
                client: clientDescription,
                method: method,
                toolName: toolName,
                outcome: .failed,
                detail: "Internal error",
                isWrite: isWrite
            )
            publish(activity)
            await recordWriteAuditIfNeeded(activity, params: params, result: nil, error: nil)
            return jsonRPCError(id: id, code: -32603, message: "Internal error", status: 200)
        }
    }

    private func handle(method: String, params: [String: Any]) async throws -> [String: Any] {
        switch method {
        case "initialize":
            return [
                "protocolVersion": "2025-06-18",
                "capabilities": [
                    "tools": [
                        "listChanged": false
                    ]
                ],
                "serverInfo": [
                    "name": "Hot Cross Buns",
                    "version": Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "0"
                ],
                "instructions": "Hot Cross Buns exposes local tasks, notes, and calendar events. Writes obey the user's MCP permission mode."
            ]

        case "tools/list":
            return [
                "tools": HCBToolService.toolDefinitions.map { tool in
                    [
                        "name": tool.name,
                        "description": tool.description,
                        "inputSchema": tool.inputSchema
                    ]
                }
            ]

        case "tools/call":
            guard let name = params["name"] as? String else {
                throw HCBToolError.invalidArguments("tools/call requires a tool name.")
            }
            let arguments = params["arguments"] as? [String: Any] ?? [:]
            let structured = try await toolService.callTool(name: name, arguments: arguments)
            let text = Self.jsonString(structured)
            return [
                "content": [
                    [
                        "type": "text",
                        "text": text
                    ]
                ],
                "structuredContent": structured,
                "isError": false
            ]

        default:
            throw HCBToolError.unknownTool(method)
        }
    }

    private func handle(_ connection: NWConnection) {
        connection.start(queue: queue)
        receive(on: connection, buffer: Data())
    }

    private func receive(on connection: NWConnection, buffer: Data) {
        connection.receive(minimumIncompleteLength: 1, maximumLength: 64 * 1024) { [weak self] data, _, isComplete, error in
            guard let self else { return }
            var next = buffer
            if let data {
                next.append(data)
            }
            if let error {
                AppLogger.warn("mcp connection receive failed", category: .misc, metadata: ["error": error.localizedDescription])
                connection.cancel()
                return
            }
            switch HTTPRequest.parse(data: next) {
            case .complete:
                let remoteIsLocal = Self.endpointIsLocal(connection.endpoint)
                let endpointDescription = Self.endpointDescription(connection.endpoint)
                Task {
                    let response = await self.handleHTTPRequest(
                        data: next,
                        remoteIsLocal: remoteIsLocal,
                        clientDescription: endpointDescription,
                        clientKey: endpointDescription
                    )
                    connection.send(content: response.data, completion: .contentProcessed { _ in
                        connection.cancel()
                    })
                }
                return
            case .tooLarge:
                let response = HTTPResponse.plain(status: 413, body: "Payload Too Large")
                connection.send(content: response.data, completion: .contentProcessed { _ in
                    connection.cancel()
                })
                return
            case .malformed:
                let response = HTTPResponse.plain(status: 400, body: "Bad Request")
                connection.send(content: response.data, completion: .contentProcessed { _ in
                    connection.cancel()
                })
                return
            case .incomplete:
                break
            }
            if isComplete {
                connection.cancel()
                return
            }
            self.receive(on: connection, buffer: next)
        }
    }

    private func publish(_ status: MCPServerStatus) {
        Task { @MainActor in
            onStatus(status)
        }
    }

    private func publish(_ activity: MCPActivityEntry) {
        Task { @MainActor in
            onActivity(activity)
        }
    }

    private func originIsAllowed(_ origin: String?) -> Bool {
        guard let origin, origin.isEmpty == false else { return true }
        guard let url = URL(string: origin), let host = url.host?.lowercased() else { return false }
        return host == "127.0.0.1" || host == "localhost" || host == "::1"
    }

    private func recordWriteAuditIfNeeded(
        _ activity: MCPActivityEntry,
        params: [String: Any],
        result: [String: Any]?,
        error: HCBToolError?
    ) async {
        guard activity.isWrite else { return }
        var metadata: [String: String] = [
            "client": activity.client,
            "method": activity.method,
            "outcome": activity.outcome.rawValue
        ]
        if let toolName = activity.toolName {
            metadata["tool"] = toolName
        }
        let arguments = params["arguments"] as? [String: Any] ?? [:]
        metadata["argumentKeys"] = Self.argumentKeysDescription(arguments)
        metadata["dryRunRequested"] = String((arguments["dryRun"] as? Bool) == true)
        metadata["confirmationSupplied"] = String(arguments["confirmationId"] != nil)
        if let structured = result?["structuredContent"] as? [String: Any] {
            metadata["applied"] = String((structured["applied"] as? Bool) == true)
            metadata["dryRun"] = String((structured["dryRun"] as? Bool) == true)
            metadata["requiresConfirmation"] = String((structured["requiresConfirmation"] as? Bool) == true)
            metadata["confirmationIssued"] = String(structured["confirmationId"] != nil)
        }
        if let error {
            metadata["error"] = Self.auditErrorDescription(error)
        }
        await auditRecorder(activity, metadata)
    }

    private static func toolName(method: String, params: [String: Any]) -> String? {
        guard method == "tools/call" else { return nil }
        return params["name"] as? String
    }

    private static func activityEntry(
        timestamp: Date,
        client: String,
        method: String,
        toolName: String?,
        isWrite: Bool,
        result: [String: Any]
    ) -> MCPActivityEntry {
        let structured = result["structuredContent"] as? [String: Any]
        let dryRun = structured?["dryRun"] as? Bool == true
        let applied = structured?["applied"] as? Bool == true
        let requiresConfirmation = structured?["requiresConfirmation"] as? Bool == true
        let outcome: MCPActivityOutcome
        let detail: String
        if dryRun {
            outcome = .dryRun
            detail = requiresConfirmation ? "Dry-run confirmation issued" : "Dry-run preview returned"
        } else if applied {
            outcome = .applied
            detail = "Write applied"
        } else if requiresConfirmation {
            outcome = .confirmationRequired
            detail = "Confirmation required"
        } else {
            outcome = .succeeded
            detail = isWrite ? "Write request completed" : "Read request completed"
        }
        return MCPActivityEntry(
            timestamp: timestamp,
            client: client,
            method: method,
            toolName: toolName,
            outcome: outcome,
            detail: detail,
            isWrite: isWrite
        )
    }

    private static func activityEntry(
        timestamp: Date,
        client: String,
        method: String,
        toolName: String?,
        isWrite: Bool,
        error: HCBToolError
    ) -> MCPActivityEntry {
        let outcome: MCPActivityOutcome
        switch error {
        case .permissionDenied:
            outcome = .denied
        case .confirmationRequired:
            outcome = .confirmationRequired
        case .mutationFailed:
            outcome = .failed
        case .unknownTool, .invalidArguments, .confirmationMismatch, .notFound:
            outcome = .invalid
        }
        return MCPActivityEntry(
            timestamp: timestamp,
            client: client,
            method: method,
            toolName: toolName,
            outcome: outcome,
            detail: auditErrorDescription(error),
            isWrite: isWrite
        )
    }

    private static func argumentKeysDescription(_ arguments: [String: Any]) -> String {
        let keys = arguments.keys.sorted()
        return keys.isEmpty ? "none" : keys.joined(separator: ",")
    }

    private static func auditErrorDescription(_ error: HCBToolError) -> String {
        switch error {
        case .unknownTool:
            return "unknownTool"
        case .invalidArguments:
            return "invalidArguments"
        case .permissionDenied:
            return "permissionDenied"
        case .confirmationRequired:
            return "confirmationRequired"
        case .confirmationMismatch:
            return "confirmationMismatch"
        case .notFound:
            return "notFound"
        case .mutationFailed:
            return "mutationFailed"
        }
    }

    private static func clientDescription(for headers: [String: String], fallback: String) -> String {
        let userAgent = sanitizedHeader(headers["user-agent"])
        let origin = sanitizedHeader(headers["origin"].flatMap { URL(string: $0)?.host })
        switch (userAgent, origin) {
        case let (.some(userAgent), .some(origin)):
            return "\(userAgent) @ \(origin)"
        case let (.some(userAgent), nil):
            return userAgent
        default:
            return fallback
        }
    }

    private static func sanitizedHeader(_ value: String?) -> String? {
        guard let value else { return nil }
        let cleaned = value
            .replacingOccurrences(of: "\r", with: "")
            .replacingOccurrences(of: "\n", with: "")
            .trimmingCharacters(in: .whitespacesAndNewlines)
        guard cleaned.isEmpty == false else { return nil }
        return String(cleaned.prefix(80))
    }

    private func jsonRPCResult(id: Any?, result: [String: Any]) -> HTTPResponse {
        .json(status: 200, object: [
            "jsonrpc": "2.0",
            "id": id ?? NSNull(),
            "result": result
        ])
    }

    private func jsonRPCError(
        id: Any?,
        code: Int,
        message: String,
        data: [String: Any]? = nil,
        status: Int
    ) -> HTTPResponse {
        var error: [String: Any] = [
            "code": code,
            "message": message
        ]
        if let data {
            error["data"] = data
        }
        return .json(status: status, object: [
            "jsonrpc": "2.0",
            "id": id ?? NSNull(),
            "error": error
        ])
    }

    private func errorCode(for error: HCBToolError) -> Int {
        switch error {
        case .unknownTool:
            -32601
        case .invalidArguments, .notFound, .confirmationMismatch:
            -32602
        case .permissionDenied, .confirmationRequired:
            -32001
        case .mutationFailed:
            -32002
        }
    }

    private static func endpointIsLocal(_ endpoint: NWEndpoint) -> Bool {
        guard case .hostPort(let host, _) = endpoint else { return true }
        switch host {
        case .ipv4(let address):
            return address == IPv4Address.loopback
        case .ipv6(let address):
            return address == IPv6Address.loopback
        case .name(let name, _):
            let lowered = name.lowercased()
            return lowered == "localhost" || lowered == "127.0.0.1" || lowered == "::1"
        default:
            return false
        }
    }

    private static func endpointDescription(_ endpoint: NWEndpoint) -> String {
        guard case .hostPort(let host, let port) = endpoint else { return "Local client" }
        return "\(host):\(port.rawValue)"
    }

    private static func authorizationMatches(_ value: String?, token: String) -> Bool {
        guard let value, value.hasPrefix("Bearer ") else { return false }
        let candidate = String(value.dropFirst("Bearer ".count))
        return constantTimeEquals(candidate, token)
    }

    private static func constantTimeEquals(_ lhs: String, _ rhs: String) -> Bool {
        let left = Array(lhs.utf8)
        let right = Array(rhs.utf8)
        let count = max(left.count, right.count)
        var difference = left.count ^ right.count
        for index in 0..<count {
            let leftByte = index < left.count ? left[index] : 0
            let rightByte = index < right.count ? right[index] : 0
            difference |= Int(leftByte ^ rightByte)
        }
        return difference == 0
    }

    static func jsonString(_ object: Any) -> String {
        let compatible = HCBMCPJSONCompatibility.convert(object)
        guard JSONSerialization.isValidJSONObject(compatible),
              let data = try? JSONSerialization.data(withJSONObject: compatible, options: [.sortedKeys]),
              let string = String(data: data, encoding: .utf8) else {
            return "{}"
        }
        return string
    }
}

struct HTTPResponse {
    var status: Int
    var headers: [String: String]
    var body: Data

    var data: Data {
        var lines = [
            "HTTP/1.1 \(status) \(Self.reason(status))",
            "Content-Length: \(body.count)",
            "Connection: close"
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

    static func json(status: Int, object: [String: Any]) -> HTTPResponse {
        let compatible = HCBMCPJSONCompatibility.convert(object)
        let data = (try? JSONSerialization.data(withJSONObject: compatible, options: [.sortedKeys])) ?? Data("{}".utf8)
        return HTTPResponse(status: status, headers: ["Content-Type": "application/json"], body: data)
    }

    static func plain(status: Int, body: String, headers: [String: String] = [:]) -> HTTPResponse {
        var headers = headers
        headers["Content-Type"] = "text/plain; charset=utf-8"
        return HTTPResponse(status: status, headers: headers, body: Data(body.utf8))
    }

    static func empty(status: Int) -> HTTPResponse {
        HTTPResponse(status: status, headers: [:], body: Data())
    }

    private static func reason(_ status: Int) -> String {
        switch status {
        case 200: "OK"
        case 202: "Accepted"
        case 400: "Bad Request"
        case 401: "Unauthorized"
        case 403: "Forbidden"
        case 404: "Not Found"
        case 405: "Method Not Allowed"
        case 413: "Payload Too Large"
        case 429: "Too Many Requests"
        case 500: "Internal Server Error"
        default: "HTTP"
        }
    }
}

private struct HTTPRequest {
    enum ParseResult {
        case complete(HTTPRequest)
        case incomplete
        case malformed
        case tooLarge
    }

    var method: String
    var path: String
    var headers: [String: String]
    var body: Data
    var totalLength: Int

    static func parse(data: Data) -> ParseResult {
        guard data.count <= MCPServerController.maxHTTPRequestBytes else {
            return .tooLarge
        }
        guard let headerEnd = data.range(of: Data("\r\n\r\n".utf8)) else {
            return data.count > MCPServerController.maxHTTPHeaderBytes ? .tooLarge : .incomplete
        }
        guard headerEnd.lowerBound <= MCPServerController.maxHTTPHeaderBytes else {
            return .tooLarge
        }
        let headerData = data[..<headerEnd.lowerBound]
        guard let headerText = String(data: headerData, encoding: .utf8) else { return .malformed }
        let lines = headerText.components(separatedBy: "\r\n")
        guard let requestLine = lines.first, requestLine.isEmpty == false else { return .malformed }
        let parts = requestLine.split(separator: " ", maxSplits: 2).map(String.init)
        guard parts.count >= 2 else { return .malformed }
        var parsedHeaders: [String: String] = [:]
        for line in lines.dropFirst() {
            guard line.isEmpty == false else { continue }
            guard let colon = line.firstIndex(of: ":") else { return .malformed }
            let name = line[..<colon].trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
            let value = line[line.index(after: colon)...].trimmingCharacters(in: .whitespacesAndNewlines)
            guard name.isEmpty == false else { return .malformed }
            if parsedHeaders[name] != nil {
                return .malformed
            }
            parsedHeaders[name] = value
        }
        let bodyStart = headerEnd.upperBound
        guard let length = parseContentLength(parsedHeaders["content-length"]) else {
            return .malformed
        }
        guard length <= MCPServerController.maxHTTPBodyBytes else {
            return .tooLarge
        }
        guard bodyStart <= MCPServerController.maxHTTPRequestBytes,
              length <= MCPServerController.maxHTTPRequestBytes - bodyStart else {
            return .tooLarge
        }
        let totalLength = bodyStart + length
        guard data.count >= totalLength else {
            return .incomplete
        }
        guard data.count == totalLength else {
            return .malformed
        }
        return .complete(HTTPRequest(
            method: parts[0].uppercased(),
            path: parts[1],
            headers: parsedHeaders,
            body: Data(data[bodyStart..<totalLength]),
            totalLength: totalLength
        ))
    }

    private static func parseContentLength(_ raw: String?) -> Int? {
        guard let raw, raw.isEmpty == false else { return 0 }
        guard raw.allSatisfy(\.isNumber), let length = Int(raw) else { return nil }
        return length
    }
}

enum HCBMCPJSONCompatibility {
    static func convert(_ value: Any) -> Any {
        switch value {
        case let dict as [String: Any]:
            var out: [String: Any] = [:]
            for (key, value) in dict {
                out[key] = convert(value)
            }
            return out
        case let array as [Any]:
            return array.map(convert)
        case let date as Date:
            return ISO8601DateFormatter().string(from: date)
        case let url as URL:
            return url.absoluteString
        case Optional<Any>.none:
            return NSNull()
        default:
            return value
        }
    }
}
