import Foundation

/// Lossless structured display data. This is never executable client authority.
public enum JSONValue: Codable, Sendable, Hashable {
    case object([String: JSONValue]), array([JSONValue]), string(String), number(Decimal), bool(Bool), null

    public init(from decoder: any Decoder) throws {
        let c = try decoder.singleValueContainer()
        if c.decodeNil() { self = .null }
        else if let v = try? c.decode(Bool.self) { self = .bool(v) }
        else if let v = try? c.decode(String.self) { self = .string(v) }
        else if let v = try? c.decode([String: JSONValue].self) { self = .object(v) }
        else if let v = try? c.decode([JSONValue].self) { self = .array(v) }
        else { self = .number(try c.decode(Decimal.self)) }
    }
    public func encode(to encoder: any Encoder) throws {
        var c = encoder.singleValueContainer()
        switch self {
        case .object(let v): try c.encode(v)
        case .array(let v): try c.encode(v)
        case .string(let v): try c.encode(v)
        case .number(let v): try c.encode(v)
        case .bool(let v): try c.encode(v)
        case .null: try c.encodeNil()
        }
    }
    public subscript(_ key: String) -> JSONValue { if case .object(let v) = self { return v[key] ?? .null }; return .null }
    public var string: String { if case .string(let v) = self { return v }; return "" }
    public var bool: Bool { if case .bool(let v) = self { return v }; return false }
    public var int: Int { if case .number(let v) = self { return NSDecimalNumber(decimal: v).intValue }; return 0 }
    public var array: [JSONValue] { if case .array(let v) = self { return v }; return [] }
    public var pretty: String {
        let encoder = JSONEncoder(); encoder.outputFormatting = [.prettyPrinted, .sortedKeys, .withoutEscapingSlashes]
        return (try? String(decoding: encoder.encode(self), as: UTF8.self)) ?? "Unavailable"
    }
}

public struct BridgeMessage: Identifiable, Sendable, Equatable {
    public let id: String
    public let value: JSONValue
    public var role: String { value["role"].string }
    public var text: String { value["text"].string }
    public var result: JSONValue { value["result"] }
    public var isUser: Bool { role == "You" || role.lowercased() == "user" }
    public var isStructured: Bool { result != .null }
    public var hasVisibleContent: Bool { !text.isEmpty || isStructured || value["bundle"] != .null || value["activity"] != .null }
}

public struct BridgeSnapshot: Sendable, Equatable {
    public let value: JSONValue
    public init(_ value: JSONValue) { self.value = value }
    public var conversationID: String { value["conversation_id"].string }
    public var context: String {
        if !conversationID.isEmpty { return "conversation:" + conversationID }
        let source = value["source"].string
        return source.isEmpty ? "" : "workspace:" + (value["primary"].bool ? "" : "topic:") + source
    }
    public var title: String { value["title"].string.isEmpty ? "Gator" : value["title"].string }
    public var running: Bool { value["running"].bool }
    public var cancelling: Bool { value["cancelling"].bool }
    public var composer: JSONValue { value["composer"] }
    public var messages: [BridgeMessage] {
        value["messages"].array.enumerated().map { index, message in
            // Appended history has a stable owner + index; content changes never change identity.
            BridgeMessage(id: "\(context):\(index)", value: message)
        }
    }
    /// Read-only dialogue history joins Work history without changing Go's raw
    /// message indexes (which exact approvals use).
    public var transcript: [BridgeMessage] {
        let turns = value["dialogue"].array.sorted { $0["sequence"].int < $1["sequence"].int }
        var remaining = ArraySlice(turns), result: [BridgeMessage] = []
        func appendTurn(_ turn: JSONValue) {
            let prefix = "\(context):dialogue:\(turn["id"].string)"
            result.append(BridgeMessage(id: prefix + ":user", value: .object(["role": .string("You"), "text": turn["text"]])))
            let response = turn["response"].string
            if !response.isEmpty { result.append(BridgeMessage(id: prefix + ":response", value: .object(["role": .string("Gator"), "text": .string(response)]))) }
            let peer = value["dialogue_peers"].array.first { $0["conversation_id"] == turn["task_chat"] && !turn["task_chat"].string.isEmpty }
            if let peer, !peer["summary"].string.isEmpty, peer["summary"].string != response {
                result.append(BridgeMessage(id: prefix + ":summary", value: .object(["role": .string("Gator"), "text": peer["summary"]])))
            }
            if !turn["issue"].string.isEmpty || !turn["task_chat"].string.isEmpty {
                let summary = [turn["issue"].string, turn["task_chat"].string.isEmpty ? "" : "Original task: \(turn["task_chat"].string)."].filter { !$0.isEmpty }.joined(separator: "\n")
                result.append(BridgeMessage(id: prefix + ":details", value: .object(["role": .string("Gator"), "result": .object(["kind": .string("notice"), "title": .string("Retained conversation context"), "summary": .string(summary), "preview": turn])])))
            }
        }
        for message in messages {
            let at = max(message.value["created_at"].string, message.value["activity"]["finished_at"].string, message.value["activity"]["started_at"].string)
            while let turn = remaining.first, turn["created_at"].string <= at { appendTurn(turn); remaining.removeFirst() }
            result.append(message)
        }
        for turn in remaining { appendTurn(turn) }
        return result
    }
    public var busy: Bool {
        running || value["dialogue_busy"].bool || value["catalog_busy"].bool || value["receipt_busy"].bool || value["goal_busy"].bool || !value["pending_operation"].string.isEmpty
    }
    public var hasExternal: Bool { value["external"] != .null || !value["pending_connection"].string.isEmpty }
    public var hasApproval: Bool { value["pending_approval"] != .null }
    public var unsafeToNavigate: Bool { busy || hasExternal || hasApproval || value["queued"].int > 0 }
    public var unsupportedNotice: String? {
        if hasExternal { return "This step requires a terminal handoff. Native handoffs are not available yet; no external process has been launched." }
        if value["dialogue_dispatch"] != .null || value["session_request"] != .null { return "This workflow requires managed sessions, which are not available in this native version." }
        if value["task_plans"].array.contains(where: { ["review", "interrupted", "approved", "preparing", "revising"].contains($0["phase"].string) }) { return "Inspect the proposed plan before any execution. Inspect its latest full review to authorize execution. Manual plan editing remains available in the terminal app." }
        if value["queued"].int > 0 { return "Retained queued work is Go-owned. Native queue editing and resuming are not available yet." }
        return nil
    }
}

public struct ApprovalReview: Sendable, Equatable, Identifiable {
    public let conversationID: String
    public let context: String
    public let generation: UUID
    public let interactionID: Int
    public let token: String
    public let messageIndex: Int
    public let result: JSONValue
    public var id: String { "\(generation):\(context):\(interactionID):\(token)" }
    public var inspectable: Bool { result["kind"].string == "approval" }
}

enum BridgeEvent: Sendable, Equatable {
    case state(BridgeSnapshot)
    case acknowledged(String)
    case updateReady(JSONValue)
    case disconnected(String)
}

struct BridgeFailure: Error, LocalizedError, Sendable {
    let message: String
    init(_ message: String) { self.message = message }
    var errorDescription: String? { message }
}

/// Framing works on bytes; UTF-8 is decoded only after the entire JSON line arrives.
struct BridgeFramer {
    private var buffer = Data()
    let limit: Int
    init(limit: Int = 16 * 1024 * 1024) { self.limit = limit }
    mutating func append(_ bytes: Data) throws -> [BridgeEvent] {
        guard buffer.count + bytes.count <= limit else { throw BridgeFailure("Gator sent an oversized state frame.") }
        buffer.append(bytes)
        var events: [BridgeEvent] = []
        while let newline = buffer.firstIndex(of: 10) {
            let line = Data(buffer[..<newline]); buffer.removeSubrange(...newline)
            let frame: JSONValue
            do { frame = try JSONDecoder().decode(JSONValue.self, from: line) }
            catch { throw BridgeFailure("Gator sent malformed JSON. Reconnect to review retained state.") }
            guard frame["version"] == .number(1) else { throw BridgeFailure("Unsupported Gator bridge version.") }
            switch frame["type"].string {
            case "state":
                guard case .object = frame["state"] else { throw BridgeFailure("Missing Gator state.") }
                events.append(.state(BridgeSnapshot(frame["state"])))
            case "action_applied":
                guard !frame["nonce"].string.isEmpty else { throw BridgeFailure("Missing acknowledgement identity.") }
                events.append(.acknowledged(frame["nonce"].string))
            case "update_ready": events.append(.updateReady(frame))
            default: throw BridgeFailure("Unsupported Gator bridge event.")
            }
        }
        return events
    }
    func finish() throws {
        if !buffer.isEmpty { throw BridgeFailure("Gator disconnected during a state frame. Its outcome is unconfirmed.") }
    }
}

/// Host-only finite commands. Native views never receive this type or a raw send method.
enum BridgeCommand: Sendable {
    case host(HostCommand)
    case submission(id: String, context: String, text: String, planFirst: Bool = false)
    case recovery(fields: [String: JSONValue])
    case receipt(NativeReceiptIdentity, action: NativeReceiptAction, request: String)
    case inspectPlan(NativePlanIdentity, request: String)
    case authorizePlan(NativePlanIdentity, action: NativePlanAction, request: String)
    case workMode(NativeWorkMode)
    case conversation(id: String, listID: String)
    case newConversation
    case model(id: String, request: String, provider: String, model: String)
    case effort(id: String, request: String, effort: String)
    case stop
    case approval(id: Int, token: String, approved: Bool)
    case exit

    func data(nonce: String) throws -> Data {
        var v: [String: JSONValue] = ["version": .number(1), "nonce": .string(nonce)]
        switch self {
        case .host(let command):
            for (key, value) in command.fields { v[key] = value }
        case let .submission(id, context, text, planFirst):
            guard !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty, text.utf8.count <= 128 * 1024 else { throw BridgeFailure("Messages must contain text and fit within 128 KiB.") }
            v["type"] = .string("upload_submit")
            v["upload"] = .object(["request_id": .string(id), "context": .string(context), "text": .string(text), "files": .array([]), "plan_first": .bool(planFirst)])
        case .recovery(let fields):
            v["type"] = .string("request_recovery"); v["request_recovery"] = .object(fields)
        case let .receipt(identity, action, request):
            var fields = identity.fields; fields["action"] = .string(action.rawValue); fields["request_id"] = .string(request)
            v["type"] = .string("receipt"); v["receipt"] = .object(fields)
        case let .inspectPlan(identity, request):
            v["type"] = .string("task_plan")
            v["task_plan"] = .object(["action": .string("inspect"), "conversation_id": .string(identity.conversationID), "task_id": .string(identity.taskID), "revision": .number(Decimal(identity.revision)), "digest": .string(identity.digest), "request_id": .string(request)])
        case let .authorizePlan(identity, action, request):
            v["type"] = .string("task_plan")
            v["task_plan"] = .object(["action": .string(action.rawValue), "conversation_id": .string(identity.conversationID), "task_id": .string(identity.taskID), "revision": .number(Decimal(identity.revision)), "digest": .string(identity.digest), "request_id": .string(request)])
        case .workMode(let mode): v["type"] = .string("submit"); v["text"] = .string("/mode " + mode.rawValue)
        case let .conversation(id, listID):
            v["type"] = .string("conversation"); v["conversation"] = .object(["id": .string(id), "list_id": .string(listID)])
        case .newConversation: v["type"] = .string("navigate"); v["text"] = .string("/new")
        case let .model(id, request, provider, model):
            v["type"] = .string("composer"); v["composer"] = .object(["id": .string(id), "request_id": .string(request), "action": .string("model"), "provider": .string(provider), "model": .string(model)])
        case let .effort(id, request, effort):
            v["type"] = .string("composer"); v["composer"] = .object(["id": .string(id), "request_id": .string(request), "action": .string("effort"), "effort": .string(effort)])
        case .stop: v["type"] = .string("submit"); v["text"] = .string("/cancel")
        case let .approval(id, token, approved):
            v["type"] = .string("approval"); v["interaction_id"] = .number(Decimal(id)); v["approval_token"] = .string(token); v["approved"] = .bool(approved)
        case .exit: v["type"] = .string("exit")
        }
        var data = try JSONEncoder().encode(JSONValue.object(v))
        guard data.count <= 256 * 1024 else { throw BridgeFailure("Native action exceeds the application limit.") }
        data.append(10); return data
    }
}
