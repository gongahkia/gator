import Foundation

/// Presentation only. Records from different journals remain distinct; no textual matching.
public struct NativeRecoveryRecord: Identifiable, Equatable, Sendable {
    public let id: String
    public let title: String
    public let status: String
    public let detail: String
    public let fields: [NativeRecoveryField]
}
public struct NativeRecoveryField: Identifiable, Equatable, Sendable {
    public let id: String
    public let text: String
}
public struct NativeReceiptIdentity: Codable, Equatable, Hashable, Sendable {
    public let conversationID, runID, actionID, correlationID, manifestSHA256: String
    var valid: Bool {
        validConversationID(conversationID) && validConversationID(runID) && actionID.range(of: "^action-[a-f0-9]{16}$", options: .regularExpression) != nil && correlationID.range(of: "^[a-f0-9]{32}$", options: .regularExpression) != nil && manifestSHA256.range(of: "^[a-f0-9]{64}$", options: .regularExpression) != nil
    }
    var fields: [String: JSONValue] {
        ["conversation_id": .string(conversationID), "run_id": .string(runID), "action_id": .string(actionID), "correlation_id": .string(correlationID), "manifest_sha256": .string(manifestSHA256)]
    }
}
public struct NativeRecoveryReceipt: Identifiable, Equatable, Sendable {
    public let id: String
    public let runID: String
    public let at: String
    public let live: Bool
    public let value: JSONValue
    public let identity: NativeReceiptIdentity?
    public var title: String { value["Operation"].string + " · " + NativeRecoveryPresentation.status(value["Status"].string) }
    public var fields: [NativeRecoveryField] {
        NativeRecoveryPresentation.fields(value, keys: ["ConnectorID", "Operation", "Status", "Recipient", "Subject", "ReceiptID", "ProviderResourceID", "Error", "ID", "CorrelationID"]) + value["Steps"].array.enumerated().map { i, step in
            NativeRecoveryField(id: "step:\(i)", text: "Batch step \(i + 1): " + [step["Operation"].string, step["Status"].string, step["ProviderResourceID"].string].filter { !$0.isEmpty }.joined(separator: " · "))
        }
    }
}
public enum NativeReceiptAction: String, Codable, Sendable { case inspect; case checkSent = "check_sent" }
public struct NativeReceiptOperation: Codable, Equatable, Sendable {
    public let id, nonce: String
    public let identity: NativeReceiptIdentity
    public let generation: UUID
    public let action: NativeReceiptAction
    public var phase = "submitted"
    public var result: JSONValue = .null
    public var unresolved: Bool { ["submitted", "acknowledged", "uncertain"].contains(phase) }
    public var summary: String {
        if phase == "uncertain" { return "Check outcome uncertain. No check, send or action will be replayed. Inspect retained evidence after reconnecting." }
        if phase == "submitted" || phase == "acknowledged" { return "Awaiting receipt result · acknowledgement is not confirmation" }
        if !result["error"].string.isEmpty { return "Check failed: " + result["error"].string }
        switch result["check"]["status"].string {
        case "found": return "Matching Sent record found. This does not establish delivery, reading or every external effect."
        case "ambiguous": return "Multiple possible records. The outcome remains uncertain; do not resend."
        case "unconfirmed": return "No matching outcome confirmed. This does not prove the action did not happen; do not resend."
        case "not_checked": return "No retained check. No Sent lookup has been performed by this inspection."
        default: return "Receipt result unavailable. The action outcome remains uncertain."
        }
    }
}

public struct NativeRecoveryProjection: Equatable, Sendable {
    public let conversationID: String
    public let records: [NativeRecoveryRecord]
    public let receipts: [NativeRecoveryReceipt]
    public let notices: [String]
    public var relevant: Bool { !records.isEmpty || !receipts.isEmpty || !notices.isEmpty }
    public var unresolved: Bool { receipts.contains { ["unknown", "pending"].contains($0.value["Status"].string) } || records.contains { !["Completed · retained evidence", "Stopped · retained state"].contains($0.status) } || !notices.isEmpty }
    public init(_ snapshot: BridgeSnapshot) {
        conversationID = snapshot.conversationID
        var rows: [NativeRecoveryRecord] = [], receipts: [NativeRecoveryReceipt] = [], notices: [String] = []
        var seen: Set<String> = []
        func add(_ source: String, _ id: String, _ title: String, _ status: String, _ detail: String, _ fields: [NativeRecoveryField] = []) {
            guard !id.isEmpty, seen.insert(source + ":" + id).inserted else { return }
            rows.append(.init(id: source + ":" + id, title: title, status: status, detail: detail, fields: fields))
        }
        for value in snapshot.value["request_recoveries"].array {
            let r = value["recovery"]
            let fields = NativeRecoveryPresentation.fields(value, keys: ["task_id", "provider", "model", "state", "plan_phase"]) + NativeRecoveryPresentation.fields(r, keys: ["kind", "summary", "repair", "scope_digest", "processing_digest", "needs", "preparation"])
            add("request", value["task_id"].string, value["text"].string, NativeRecoveryPresentation.status(value["state"].string), "Recovery guidance only. " + (r["retry_safe"].bool ? "Go reports conditional retry eligibility; this is not permission to replay. " : "No safe retry is established. ") + "Repair, retry preparation and discard are unavailable here.", fields)
        }
        for q in snapshot.value["queue"].array {
            add("queue", q["id"].string, q["text"].string, q["interrupted"].bool ? "Interrupted · not resumed" : "Queued · not executing", "Retained queue identity. Opening these details does not run or resume it.", NativeRecoveryPresentation.fields(q, keys: ["id", "inputs"]))
        }
        for plan in snapshot.value["task_plans"].array where ["interrupted", "failed", "stopped", "cancelled"].contains(plan["state"].string) || plan["phase"].string == "interrupted" {
            let fields = NativeRecoveryPresentation.fields(plan, keys: ["id", "work_id", "state", "phase", "revision", "digest", "issue", "outcome"])
            if let index = rows.firstIndex(where: { $0.id == "request:" + plan["id"].string }) {
                let retained = rows[index]
                rows[index] = .init(id: retained.id, title: retained.title, status: retained.status, detail: retained.detail + " Proposed plan steps do not establish completed execution.", fields: retained.fields + fields.map { .init(id: "plan." + $0.id, text: $0.text) })
            } else {
                add("plan", plan["id"].string, plan["task"].string, NativeRecoveryPresentation.status(plan["phase"].string == "interrupted" ? "interrupted" : plan["state"].string), "Retained plan; proposed steps are not proof of completed execution.", fields)
            }
        }
        for message in snapshot.messages {
            let activity = message.value["activity"], state = activity["state"].string
            if ["interrupted", "failed", "stopped", "review"].contains(state) {
                // No run identity? Keep the exact durable conversation message index.
                add("message", message.id, "Retained work activity", NativeRecoveryPresentation.status(state), "Retained text may be partial. It does not establish completed execution.", NativeRecoveryPresentation.fields(message.value, keys: ["run_id", "revision_id", "text"]) + NativeRecoveryPresentation.fields(activity, keys: ["state", "started_at", "finished_at", "error"]))
            }
        }
        let task = snapshot.value["dialogue_task"]
        if ["interrupted", "failed", "stopped", "review", "unconfirmed"].contains(task["state"].string), task["conversation_id"].string == snapshot.conversationID {
            add("task", task["turn_id"].string, task["title"].string, NativeRecoveryPresentation.status(task["state"].string), "Original delegated task; inspect retained summary without restarting it.", NativeRecoveryPresentation.fields(task, keys: ["conversation_id", "parent_id", "turn_id", "state", "summary", "issue"]))
        }
        var bundleSeen: Set<String> = []
        let bundles = snapshot.messages.map { $0.value["bundle"] } + [snapshot.value["work_details"]["latest_bundle"]]
        for bundle in bundles where bundle != .null {
            let run = bundle["RunID"].string, digest = bundle["ManifestSHA256"].string
            guard !run.isEmpty, bundleSeen.insert(run + ":" + digest).inserted else { continue }
            let consistent = bundles.filter { $0["RunID"].string == run && $0["ManifestSHA256"].string == digest }.allSatisfy { $0["Actions"] == bundle["Actions"] }
            if !consistent { notices.append("Conflicting retained receipt projections. Exact checks are unavailable; reopen the owner’s canonical state.") }
            if bundle["ActionsTruncated"].bool { notices.append("Some retained bundle action receipts were omitted. Evidence is incomplete.") }
            if !run.isEmpty {
                add("bundle", run + ":" + digest, "Retained result bundle", NativeRecoveryPresentation.status(bundle["Status"].string), "Sealed output is retained evidence, not proof that every external effect completed.", NativeRecoveryPresentation.fields(bundle, keys: ["RunID", "Status", "ManifestSHA256"]) + bundle["Artifacts"].array.enumerated().map { i, artifact in
                    NativeRecoveryField(id: "artifact:\(i)", text: [artifact["Path"].string, artifact["Preview"].string, artifact["Truncated"].bool ? "Preview truncated; full output remains in the original conversation bundle." : ""].filter { !$0.isEmpty }.joined(separator: "\n"))
                })
            }
            for (index, value) in bundle["Actions"].array.enumerated() {
                let action = value["ID"].string, correlation = value["CorrelationID"].string
                let matches = bundle["Actions"].array.filter { $0["ID"].string == action && $0["CorrelationID"].string == correlation }.count
                let candidate = NativeReceiptIdentity(conversationID: snapshot.conversationID, runID: run, actionID: action, correlationID: correlation, manifestSHA256: digest)
                let eligible = consistent && candidate.valid && matches == 1 && value["Status"].string == "unknown" && value["Operation"].string == "gmail_send"
                let identity = eligible ? candidate : nil
                receipts.append(.init(id: "bundle:\(run):\(digest):\(index)", runID: run, at: "", live: false, value: value, identity: identity))
            }
        }
        // Go already removes sealed receipts from this projection. Do not reconstruct or merge attempts.
        var recoveredSeen: Set<String> = []
        for value in snapshot.value["action_recoveries"].array {
            let id = value["id"].string
            guard !id.isEmpty, recoveredSeen.insert(id).inserted else { continue }
            receipts.append(.init(id: "journal:" + id, runID: value["run_id"].string, at: value["at"].string, live: value["live"].bool, value: value["receipt"], identity: nil))
        }
        if snapshot.value["action_recovery_truncated"].bool { notices.append("Saved action evidence is truncated to Go’s bounded projection; older attempts may be unavailable here.") }
        for key in ["action_recovery_issue", "requests_issue", "dialogue_issue"] where !snapshot.value[key].string.isEmpty { notices.append(snapshot.value[key].string) }
        records = rows; self.receipts = receipts; self.notices = notices
    }
}
public enum NativeRecoveryPresentation {
    static func fields(_ value: JSONValue, keys: [String]) -> [NativeRecoveryField] {
        keys.compactMap { key in
            let item = value[key]
            guard item != .null, item != .string(""), item != .array([]) else { return nil }
            return .init(id: key, text: key + ": " + (item.string.isEmpty ? item.pretty : item.string))
        }
    }
    public static func status(_ value: String) -> String {
        switch value {
        case "unknown", "unconfirmed": return "External outcome uncertain"
        case "interrupted": return "Interrupted · not resumed"
        case "failed": return "Failed · recorded error"
        case "stopped", "cancelled": return "Stopped · retained state"
        case "review", "pending_approval": return "Waiting for human review"
        case "completed", "success": return "Completed · retained evidence"
        case "executed": return "Provider accepted · delivery not established"
        case "pending": return "Proposed or pending · execution not confirmed"
        default: return value.isEmpty ? "State unavailable" : "Recorded state: " + value
        }
    }
}
