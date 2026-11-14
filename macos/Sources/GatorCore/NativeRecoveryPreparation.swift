import Foundation

public struct NativeRequestRecovery: Equatable, Identifiable, Sendable {
    public let conversationID: String
    public let generation: UUID
    public let value: JSONValue
    public var id: String { value["task_id"].string }
    var token: String { value["token"].string }
    public var needs: [JSONValue] { value["recovery"]["needs"].array }
    public var valid: Bool { validConversationID(conversationID) && id.range(of: "^[a-f0-9]{32}$", options: .regularExpression) != nil && token.range(of: "^[a-f0-9]{64}$", options: .regularExpression) != nil }
    public var retrySafe: Bool { value["recovery"]["retry_safe"].bool }
    public var summary: String {
        if !valid { return "Recovery identity unavailable · inspection only" }
        if !retrySafe { return "Retry preparation is unsafe or unsupported. Inspect retained effects; no replay is available." }
        if !needs.isEmpty { return "This retained task requires a review of configured account access before preparing another plan." }
        if value["state"].string != "failed" { return "This work is no longer an eligible failed request. Inspect its current plan and retained outcome." }
        return "Go reports conditional recovery preparation eligibility. Repair the original provider/model if required, then explicitly prepare a plan for fresh review."
    }
}
public struct NativeRecoveryAccess: Equatable, Sendable {
    public let recovery: NativeRequestRecovery
    public let scope: JSONValue
    public var requirements: [JSONValue]
    // Positional: do not replace with a set or the backend's union of connections.
    public var selections: [String]
    public var review: JSONValue?
    public var issue = ""
    public var ready: Bool { review?["ready"].bool == true && !(review?["token"].string ?? "").isEmpty }
}
public enum NativeRecoveryAction: String, Codable, Sendable {
    case reviewAccess = "review_access", prepareAccess = "prepare_access", prepareRetry = "prepare_retry"
}
public struct NativeRecoveryPreparation: Codable, Equatable, Sendable {
    public let id, nonce, conversationID, taskID, token: String
    public let generation: UUID
    public let action: NativeRecoveryAction
    public let connections: [String]?
    public let accessToken: String?
    public let previousRevision: Int
    public var resultConfirmed: Bool?
    public var accepted: Bool?
    public var phase = "submitted"
    public var error = ""
    public var blocks: Bool { ["submitted", "acknowledged", "accepted", "preparing", "uncertain"].contains(phase) }
    var awaitsResult: Bool { ["submitted", "acknowledged", "uncertain"].contains(phase) }
    public var status: String {
        switch phase {
        case "submitted", "acknowledged": return "Recovery preparation pending · transport acknowledgement is not admission"
        case "accepted": return "Go admitted preparation · a revised plan is not yet confirmed; nothing approved"
        case "preparing": return "Model preparing revised plan · no execution authorized"
        case "plan_ready": return "Revised plan ready · inspect its latest revision and approve separately"
        case "reconciled": return "A newer proposal is retained after reconnect. Prior admission remains unconfirmed; no request was replayed. Inspect the current plan."
        case "uncertain": return accepted == true ? "Preparation outcome uncertain · Go had admitted preparation; revised plan not confirmed; no automatic retry" : "Preparation outcome uncertain · admission unconfirmed; retained request identity; no automatic retry"
        case "rejected": return "Go rejected preparation: " + error
        case "not_submitted": return "Preparation not submitted: " + error
        default: return "Preparation failed: " + error
        }
    }
    var fields: [String: JSONValue] {
        var result: [String: JSONValue] = ["conversation_id": .string(conversationID), "task_id": .string(taskID), "token": .string(token), "action": .string(action.rawValue), "request_id": .string(id)]
        if let connections { result["connections"] = .array(connections.map(JSONValue.string)) }
        if let accessToken { result["access_token"] = .string(accessToken) }
        return result
    }
}
