import Foundation

public struct NativeTaskInspection: Equatable, Sendable, Identifiable {
    public let ownerID: String
    public let generation: UUID
    public var id: String { ownerID } // Completion updates this task, not a replacement row.
}

public struct NativePlanIdentity: Codable, Equatable, Sendable, Identifiable {
    public let conversationID: String
    public let taskID: String
    public let revision: Int
    public let digest: String
    public var id: String { taskID }
    public init?(conversationID: String, summary: JSONValue) {
        let task = summary["id"].string, digest = summary["digest"].string
        guard validConversationID(conversationID), validConversationID(task), task.count == 32, summary["revision"].int >= 0, digest.isEmpty || digest.count == 64 else { return nil }
        self.conversationID = conversationID; self.taskID = task; self.revision = summary["revision"].int; self.digest = digest
    }
    public func matches(_ summary: JSONValue) -> Bool { summary["id"].string == taskID && summary["revision"].int == revision && summary["digest"].string == digest }
}

public struct NativePlanReview: Sendable, Equatable {
    public let identity: NativePlanIdentity
    public let generation: UUID
    public let value: JSONValue
    public var status: String {
        if value["edit_state"].string == "preparing" || value["phase"].string == "revising" { return "Revision requested · replacement is being prepared" }
        if value["phase"].string == "review" { return "Proposed plan · new review required before execution" }
        return "Plan state: " + value["phase"].string + " · " + value["state"].string
    }
}

public struct NativeCorrectionTarget: Codable, Equatable, Sendable {
    public enum Kind: String, Codable, Sendable { case action, plan }
    public let kind: Kind
    public let ownerID: String
    public let generation: UUID
    public let reviewToken: String?
    public let plan: NativePlanIdentity?
    public let taskTurnID: String
    public var label: String { kind == .action ? "Request an action correction · no approval" : "Request a plan revision · no execution" }
    func message(_ instruction: String) -> String {
        let subject = kind == .action ? "the pending action" : "plan task \(plan?.taskID ?? "") revision \(plan?.revision ?? 0)"
        return "Regarding \(subject) in conversation \(ownerID):\n\(instruction)\nThis is a correction request for review, not approval or permission to start or send anything."
    }
}

struct NativeCorrectionDraft: Codable, Equatable, Sendable {
    let target: NativeCorrectionTarget
    var text = ""
}

public enum NativeInteractionSemantics {
    public static func sendOnReturn(hasMarkedText: Bool, shift: Bool) -> Bool { !hasMarkedText && !shift }
    public static func decisionKeyAllowed(_ code: UInt16) -> Bool { code != 36 && code != 76 }
    public static func workingLabel(reduceMotion: Bool) -> String { reduceMotion ? "Working · progress updates continue" : "Working" }
}

public struct NativeReviewField: Identifiable, Equatable, Sendable {
    public let id: String
    public let label: String
    public let text: String
}

public enum NativeReviewPresentation {
    public static func fields(_ result: JSONValue) -> [NativeReviewField] {
        var fields: [NativeReviewField] = []
        func add(_ id: String, _ label: String, _ text: String) { fields.append(NativeReviewField(id: id, label: label, text: text)) }
        for (index, field) in result["fields"].array.enumerated() { add("field:\(index)", field["label"].string, field["value"].string) }
        var complete = true
        func flatten(_ value: JSONValue, _ path: String, _ label: String, _ depth: Int) {
            guard fields.count < 256, depth < 12 else { complete = false; return }
            switch value {
            case .object(let values):
                for key in values.keys.sorted() { flatten(values[key]!, path + "." + key, label.isEmpty ? key : label + " / " + key, depth + 1) }
            case .array(let values):
                for (index, item) in values.enumerated() { flatten(item, path + ".\(index)", label + " [\(index + 1)]", depth + 1) }
            case .string(let text): add(path, label.replacingOccurrences(of: "_", with: " "), text)
            default: add(path, label.replacingOccurrences(of: "_", with: " "), value.pretty)
            }
        }
        if result["preview"] != .null { flatten(result["preview"], "preview", "", 0) }
        // Bounded flattening must never hide exact proposal content.
        if !complete { fields.removeAll { $0.id.hasPrefix("preview") }; add("complete", "Complete exact preview", result["preview"].pretty) }
        for (index, section) in result["sections"].array.enumerated() {
            for (j, field) in section["fields"].array.enumerated() { add("section:\(index):\(j)", section["title"].string + " / " + field["label"].string, field["value"].string) }
            for (j, line) in section["lines"].array.enumerated() { add("line:\(index):\(j)", section["title"].string, line.string) }
        }
        return fields
    }
}

/// Public, bounded task prose only. No raw activity payloads or model reasoning.
public struct NativeTaskPresentation: Sendable {
    public let ownerID: String
    public let title: String
    public let state: String
    public let parentID: String
    public let turnID: String
    public let summary: String
    public let publicActivity: [String]
    public let issue: String
    public init(snapshot: BridgeSnapshot, connection: NativeConnectionStatus, issue: String) {
        ownerID = snapshot.conversationID; title = snapshot.title
        parentID = snapshot.value["dialogue_task"]["parent_id"].string
        turnID = snapshot.value["dialogue_task"]["turn_id"].string
        state = connection == .disconnected ? "Disconnected · retained state" : snapshot.hasApproval ? "Waiting for exact action review" : snapshot.running ? snapshot.cancelling ? "Stopping" : snapshot.value["status"].string.isEmpty ? "Working" : snapshot.value["status"].string : snapshot.value["dialogue_task"]["state"].string.isEmpty ? snapshot.value["task_plans"].array.isEmpty ? "Idle · inspect retained results" : "Proposed plan · not evidence of completion" : snapshot.value["dialogue_task"]["state"].string.capitalized
        summary = String(snapshot.value["dialogue_task"]["summary"].string.prefix(2000))
        publicActivity = snapshot.value["activity"]["entries"].array.filter { $0["kind"].string == "tool" }.suffix(8).map { $0["tool"].string + " · " + $0["state"].string }
        self.issue = [issue, snapshot.value["requests_issue"].string, snapshot.value["dialogue_issue"].string].filter { !$0.isEmpty }.joined(separator: "\n")
    }
}
