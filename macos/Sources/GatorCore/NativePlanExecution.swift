import Foundation

/// Finite presentation intent. Go remains the approval and dispatch authority.
public enum NativePlanAction: String, Codable, Sendable {
    case approve, start
    public var title: String { self == .approve ? "Approve & Start" : "Start Approved Plan" }
}
public enum NativePlanOperationPhase: String, Codable, Sendable {
    case submitted, acknowledged, accepted, working, waitingReview, completed, failed, stopped, rejected, uncertain, notSubmitted, reconciled
    public var awaitingResult: Bool { self == .submitted || self == .acknowledged }
}
public struct NativePlanOperation: Codable, Sendable, Equatable {
    public let id: String
    public let nonce: String
    public let identity: NativePlanIdentity
    public let generation: UUID
    public let action: NativePlanAction
    public var phase: NativePlanOperationPhase = .submitted
    public var resultConfirmed = false
    public var workID = ""
    public var error = ""
    public var blocksAuthorization: Bool { phase.awaitingResult || phase == .uncertain }
    public var reservesCapacity: Bool { blocksAuthorization || phase == .accepted }
    public var status: String {
        switch phase {
        case .reconciled: return "Retained authorization inspected after reconnect · no request replayed; inspect the canonical task state"
        case .notSubmitted: return "Not submitted · " + error
        case .submitted: return "Awaiting confirmation · authorization submitted"
        case .acknowledged: return "Awaiting confirmation · transport acknowledged, Go result pending"
        case .accepted: return "Go accepted the operation · execution not yet observed"
        case .working: return "Working · reviewed task execution observed"
        case .waitingReview: return "Waiting for exact action review · plan approval does not approve tool actions"
        case .completed: return "Completed · inspect the retained task result"
        case .failed: return "Task failed · inspect its retained error and effects"
        case .stopped: return "Task stopped · retained results may include partial effects"
        case .rejected: return "Go rejected the operation: " + error
        case .uncertain: return "Authorization outcome uncertain · no automatic retry. " + error
        }
    }
}
public enum NativePlanPresentation {
    static func scope(_ snapshot: BridgeSnapshot) -> JSONValue {
        let value = snapshot.value
        return .object(Dictionary(uniqueKeysWithValues: ["source", "sources", "provider", "model", "composer", "preferences_revision", "request_recoveries"].map { ($0, value[$0]) }))
    }
    public static func phaseLabel(_ value: JSONValue) -> String {
        switch value["state"].string {
        case "completed": return "Completed work · steps below are the reviewed plan, not individual step receipts"
        case "failed", "interrupted": return "Failed or interrupted · inspect retained effects before recovery"
        case "stopped", "cancelled": return "Stopped or cancelled · no new execution authorization"
        default: break
        }
        switch value["phase"].string {
        case "review": return "Proposed plan · not executed"
        case "approved": return "Approved but undispatched · explicit Start required"
        case "dispatching": return "Dispatched work · inspect live activity and retained results"
        case "preparing", "revising": return "Preparing a proposal · not approval-ready"
        default: return "Plan phase: " + value["phase"].string + " · task state: " + value["state"].string
        }
    }
    public static func complete(_ review: NativePlanReview) -> Bool {
        let value = review.value, content = value["content"]
        return !value["task"].string.isEmpty && !content["outcome"].string.isEmpty && !content["steps"].array.isEmpty && content["steps"].array.allSatisfy { !$0["id"].string.isEmpty && !$0["text"].string.isEmpty } && !content["effects"].array.isEmpty && value["budget"]["max_steps"] != .null && value["budget"]["code_max_steps"] != .null && value["sources"] != .null && review.identity.digest.count == 64
    }
}
