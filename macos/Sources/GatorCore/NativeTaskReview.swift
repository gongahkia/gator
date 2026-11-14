import Foundation

public enum NativeWorkMode: String, CaseIterable, Sendable {
    case auto, inspect, draft, act
    public var label: String {
        switch self { case .auto: return "Automatic"; case .inspect: return "Read and Inspect"; case .draft: return "Prepare Drafts"; case .act: return "Propose Actions" }
    }
}

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

public struct NativePlanSummary: Identifiable, Sendable, Equatable {
    public let id: String
    public let value: JSONValue
    public init?(_ value: JSONValue) {
        guard validConversationID(value["id"].string) else { return nil }
        id = value["id"].string; self.value = value
    }
}

public struct NativePlanReview: Sendable, Equatable {
    public let identity: NativePlanIdentity
    public let generation: UUID
    public let value: JSONValue
    public var summaryAtInspection: JSONValue? = nil
    public var scopeAtInspection: JSONValue? = nil
    public var focusEpoch: UUID? = nil
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
    public static func activateDecisionOnSpace(code: UInt16, repeating: Bool, modified: Bool) -> Bool { code == 49 && !repeating && !modified }
    public static func workingLabel(reduceMotion: Bool) -> String { reduceMotion ? "Working · progress updates continue" : "Working" }
}

public struct NativeReviewField: Identifiable, Equatable, Sendable {
    public let id: String
    public let label: String
    public let text: String
    public var isTechnical: Bool { [".payload_sha256", ".correlation_id", ".id"].contains(where: id.hasSuffix) }
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
                let priority = ["connector_id", "account", "from", "to", "cc", "bcc", "destination", "target", "operation", "subject", "title", "capability", "preview", "payload", "body", "content"]
                let keys = values.keys.sorted { left, right in
                    let a = priority.firstIndex(of: left) ?? 100, b = priority.firstIndex(of: right) ?? 100
                    return a == b ? left < right : a < b
                }
                for key in keys { flatten(values[key]!, path + "." + key, label.isEmpty ? key : label + " / " + key, depth + 1) }
            case .array(let values):
                for (index, item) in values.enumerated() { flatten(item, path + ".\(index)", label + " [\(index + 1)]", depth + 1) }
            case .string(let text):
                // Go's action agreement carries its public preview as encoded
                // JSON. Decode only that known envelope for literal display;
                // user body strings remain untouched. Authority is the original
                // review/token, never this presentation projection.
                if path == "preview.preview", let decoded = try? JSONDecoder().decode(JSONValue.self, from: Data(text.utf8)), case .object = decoded {
                    flatten(decoded, path + ".content", label, depth + 1)
                } else { add(path, label.replacingOccurrences(of: "_", with: " "), text) }
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
        let terminal = snapshot.messages.last(where: { $0.value["activity"] != .null })?.value["activity"]["state"].string ?? ""
        let peer = snapshot.value["dialogue_task"]["state"].string
        if connection == .disconnected { state = "Disconnected · retained state" }
        else if snapshot.hasApproval { state = "Waiting for exact action review" }
        else if snapshot.running { state = snapshot.cancelling ? "Stopping" : snapshot.value["status"].string.isEmpty ? "Working" : snapshot.value["status"].string }
        else if snapshot.value["task_plans"].array.contains(where: { ["review", "approved", "preparing", "revising"].contains($0["phase"].string) }) { state = "Proposed plan · not evidence of completion" }
        else if !peer.isEmpty { state = peer.capitalized }
        else if ["completed", "failed", "stopped"].contains(terminal) { state = terminal.capitalized }
        else { state = "Idle · inspect retained results" }
        let peerSummary = snapshot.value["dialogue_task"]["summary"].string
        let prose = snapshot.messages.last(where: { ["gator", "assistant"].contains($0.role.lowercased()) && !$0.text.isEmpty })?.text ?? ""
        summary = String((peerSummary.isEmpty ? prose : peerSummary).prefix(2000))
        publicActivity = snapshot.value["activity"]["entries"].array.filter { $0["kind"].string == "tool" }.suffix(8).map { $0["tool"].string + " · " + $0["state"].string }
        self.issue = [issue, snapshot.value["requests_issue"].string, snapshot.value["dialogue_issue"].string].filter { !$0.isEmpty }.joined(separator: "\n")
    }
}
