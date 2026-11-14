import SwiftUI
import GatorCore

/// All material execution context is expanded before the exact decision.
struct PlanReviewContent: View {
    @Environment(ManagedSessionCoordinator.self) private var sessions
    let review: NativePlanReview
    let owner: GatorApplicationState
    private var value: JSONValue { review.value }
    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            field("Task objective", value["task"].string)
            field("Proposed outcome", value["content"]["outcome"].string)
            Text("Ordered proposed steps").font(.headline).accessibilityAddTraits(.isHeader)
            ForEach(Array(value["content"]["steps"].array.enumerated()), id: \.offset) { i, step in
                Text(verbatim: "\(i + 1). " + step["text"].string).accessibilityLabel("Proposed step \(i + 1): " + step["text"].string)
            }
            Text("These are proposed instructions. Completion is reported by the task's retained result, not checked off from this list.").font(.caption).foregroundStyle(.secondary)
            strings("Declared effects", value["content"]["effects"], empty: "No declared effects were supplied; an incomplete review cannot authorize execution.")
            strings("Selected connections and sources", value["sources"], empty: "None retained in this plan")
            strings("Sources used by this proposal", value["content"]["sources"], empty: "None declared")
            strings("Retained inputs", value["input_names"], empty: "No retained files or source inputs")
            strings("Inputs added", value["input_additions"])
            strings("Inputs removed", value["input_removals"])
            if !value["access_changes"].array.isEmpty {
                Text("Access and connection changes").font(.headline).accessibilityAddTraits(.isHeader)
                ForEach(Array(value["access_changes"].array.enumerated()), id: \.offset) { _, change in
                    VStack(alignment: .leading, spacing: 6) {
                        strings("Previously selected connections", change["before"], empty: "None")
                        strings("New selected connections", change["after"], empty: "None")
                        ForEach(Array(change["requirements"].array.enumerated()), id: \.offset) { _, requirement in
                            field("Required access", requirement.pretty)
                        }
                        // Access context is Go's public review projection, never a grant.
                        if change["prior_plan"] != .null { field("Prior plan affected by access change", change["prior_plan"].pretty) }
                    }.padding(10).background(.background, in: RoundedRectangle(cornerRadius: 8))
                }
            }
            Text(value["attempts"].array.isEmpty ? "Execution budget" : "Remaining execution budget").font(.headline).accessibilityAddTraits(.isHeader)
            budget(value["budget"])
            if value["original_budget"] != .null { Text("Original budget").font(.caption.weight(.semibold)); budget(value["original_budget"]) }
            ForEach(Array(value["attempts"].array.enumerated()), id: \.offset) { i, attempt in
                field("Prior attempt \(i + 1) · " + (attempt["uncertain"].bool ? "effects uncertain" : "retained measured usage"), attempt.pretty)
            }
            field(owner.isCurrentPlanReview(review) ? "Current task state and plan phase" : "State when this stale review was loaded", value["state"].string + " · " + value["phase"].string)
            field("Exact revision", String(review.identity.revision))
            if !value["correction"].string.isEmpty { field("Requested correction", value["correction"].string + "\n" + value["edit_state"].string) }
            if !value["issue"].string.isEmpty { field("Task error or recovery information", value["issue"].string) }
            ForEach(Array((owner.snapshot?.value["request_recoveries"].array ?? []).filter { $0["task_id"].string == review.identity.taskID }.enumerated()), id: \.offset) { _, recovery in
                field("Retained recovery requirements", recovery["recovery"].pretty)
            }
            if let pending = owner.planOperation, pending.identity.taskID == review.identity.taskID {
                Label(pending.status, systemImage: pending.phase == .uncertain ? "exclamationmark.triangle" : "clock")
                    .accessibilityLabel("Plan operation: " + pending.status)
                    .background(NativeAccessibilityStatus(status: pending.status).frame(width: 0, height: 0))
                if !pending.error.isEmpty { field("Operation details", pending.error) }
            }
            if value["phase"].string == "review" || value["phase"].string == "approved" {
                let action: NativePlanAction = value["phase"].string == "approved" ? .start : .approve
                let unavailable = sessions.planAuthorizationUnavailable(owner: owner, review: review, action: action)
                Text(action == .approve ? "Approve & Start authorizes this exact revision and may start execution immediately. Tool actions still require their own exact review where Go requires it." : "Start uses the retained approval for this exact plan. It does not approve a new or changed proposal.")
                    .font(.callout)
                ExactDecisionButton(title: action.title, enabled: unavailable == nil) {
                    Task { await sessions.authorizePlan(owner: owner, review: review, action: action) }
                }.fixedSize().accessibilityIdentifier("gator.plan." + action.rawValue + "." + review.identity.taskID)
                Text("Explicit Space or click authorizes · Return never authorizes").font(.caption).foregroundStyle(.secondary)
                if let unavailable { Label(unavailable, systemImage: "exclamationmark.triangle").font(.caption) }
            }
            DisclosureGroup("Exact review identity and data") {
                field("Task", review.identity.taskID)
                field("Digest", review.identity.digest)
                Text(verbatim: value.pretty).font(.system(.caption, design: .monospaced))
            }
        }.textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
    }
    private func field(_ label: String, _ text: String) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(verbatim: label).font(.caption.weight(.semibold)).foregroundStyle(.secondary)
            Text(verbatim: text).fixedSize(horizontal: false, vertical: true)
        }.accessibilityRepresentation { Text(verbatim: label + ": " + text) }
    }
    private func budget(_ value: JSONValue) -> some View {
        Text("Work steps: \(value["max_steps"].int) · code steps: \(value["code_max_steps"].int)").monospacedDigit()
    }
    @ViewBuilder private func strings(_ label: String, _ value: JSONValue, empty: String? = nil) -> some View {
        if !value.array.isEmpty || empty != nil {
            field(label, value.array.isEmpty ? (empty ?? "") : value.array.map(\.string).joined(separator: "\n"))
        }
    }
}
