import SwiftUI
import GatorCore

struct TaskInspectorView: View {
    @Environment(ManagedSessionCoordinator.self) private var sessions
    let inspection: NativeTaskInspection
    let owner: GatorApplicationState?
    var compact: Bool
    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text("Task details").font(.headline).accessibilityAddTraits(.isHeader)
                Spacer()
                Button("Back to Conversation") { sessions.closeTaskInspection() }.keyboardShortcut(.escape, modifiers: [])
            }
            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
                    if let owner, let snapshot = owner.snapshot {
                        let task = NativeTaskPresentation(snapshot: snapshot, connection: owner.connectionStatus, issue: owner.issue)
                        Text(verbatim: task.title).font(.title3.weight(.medium)).textSelection(.enabled)
                        Label(task.state, systemImage: snapshot.hasApproval ? "hand.raised" : snapshot.running ? "clock" : "text.bubble")
                            .accessibilityLabel("Task status: " + task.state)
                        if !sessions.isInspectionCurrent { Text("This owner changed. Close these details and inspect the task again before requesting any controls.").foregroundStyle(.orange) }
                        if !task.summary.isEmpty { Text(verbatim: task.summary).textSelection(.enabled) }
                        if !task.issue.isEmpty { Label("Task needs attention", systemImage: "exclamationmark.triangle"); Text(verbatim: task.issue).textSelection(.enabled) }
                        if owner !== sessions.current {
                            Text("Work stays in its original Go owner while you inspect it.").font(.caption).foregroundStyle(.secondary)
                            Button("Open Owning Chat") { Task { await sessions.openConversation(task.ownerID) } }.disabled(sessions.switching)
                        }
                        DisclosureGroup("Task identity and ownership") {
                            Text(verbatim: "Conversation: " + task.ownerID)
                            if !task.parentID.isEmpty { Text(verbatim: "Requesting conversation: " + task.parentID) }
                            if !task.turnID.isEmpty { Text(verbatim: "Original turn: " + task.turnID) }
                            Text("Owner connection: " + owner.connectionStatus.rawValue)
                        }.font(.caption).textSelection(.enabled)
                        if !task.publicActivity.isEmpty {
                            VStack(alignment: .leading, spacing: 6) {
                                Text("Public work activity").font(.headline)
                                ForEach(Array(task.publicActivity.enumerated()), id: \.offset) { _, entry in Text(verbatim: entry) }
                            }.textSelection(.enabled)
                        }
                        let capability = snapshot.value["dialogue_task"]
                        if capability["can_guide"].bool || capability["can_stop"].bool || capability["can_correct_action"].bool || capability["can_revise_plan"].bool {
                            DisclosureGroup("Controls currently reported by Gator") {
                                if capability["can_guide"].bool { Text("Accepts guidance") }
                                if capability["can_stop"].bool { Text("Can be stopped in its owning chat") }
                                if capability["can_correct_action"].bool { Text("Pending action can receive a correction request") }
                                if capability["can_revise_plan"].bool { Text("Plan can receive a revision request") }
                            }.font(.caption)
                        }
                        if let exact = owner.currentApprovalReview {
                            Text("Pending proposed action · not executed").font(.headline)
                            ExactReviewContent(value: exact.result)
                            if owner === sessions.current {
                                Button("Review Exact Action") { sessions.closeTaskInspection(); owner.openApprovalReview() }
                                    .disabled(!sessions.isInspectionCurrent && sessions.inspection != nil)
                            } else { Text("Open the owning chat to make an exact decision.").font(.caption) }
                        } else if let retained = owner.messages.last(where: { $0.result["kind"].string == "approval" }) {
                            DisclosureGroup("Retained proposal · no approval authority") { ExactReviewContent(value: retained.result) }
                        }
                        ForEach(snapshot.value["task_plans"].array, id: \.self) { summary in
                            if let identity = NativePlanIdentity(conversationID: task.ownerID, summary: summary) {
                                plan(summary, identity: identity, owner: owner)
                            }
                        }
                        if snapshot.value["task_plans"].array.isEmpty && !snapshot.running && !snapshot.hasApproval && task.summary.isEmpty {
                            Text("Open the conversation to read its retained messages and results.").foregroundStyle(.secondary)
                        }
                    } else {
                        Text("The original owner is unavailable. Open its retained conversation explicitly; no replacement process was started by this inspector.")
                    }
                }.frame(maxWidth: .infinity, alignment: .leading).padding(12)
            }.frame(maxHeight: .infinity)
        }.padding(14).background(.regularMaterial, in: RoundedRectangle(cornerRadius: 14))
            .accessibilityIdentifier("gator.task.inspector." + inspection.ownerID)
    }
    @ViewBuilder private func plan(_ summary: JSONValue, identity: NativePlanIdentity, owner: GatorApplicationState) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            Label("Proposed plan", systemImage: "list.number").font(.headline)
            Text(verbatim: summary["task"].string).textSelection(.enabled)
            Text("Phase: " + summary["phase"].string + " · revision " + String(identity.revision)).font(.caption)
            if !summary["issue"].string.isEmpty { Text(verbatim: summary["issue"].string).textSelection(.enabled) }
            if let review = owner.planReviews[identity.taskID] {
                let fresh = owner.isCurrentPlanReview(review)
                Text(fresh ? review.status : "This read belongs to an older plan. Inspect the current proposal again.").font(.callout).foregroundStyle(fresh ? Color.primary : .orange)
                PlanReviewContent(review: review)
                if fresh, owner === sessions.current, summary["phase"].string == "review" {
                    Button("Request Plan Revision") { Task { await sessions.requestPlanRevision(owner: owner, review: review) } }
                        .disabled(sessions.switching || !sessions.isInspectionCurrent)
                        .help("Open an editable correction draft. This does not approve or execute the plan.")
                }
            } else {
                Text(verbatim: summary["outcome"].string).textSelection(.enabled)
                ForEach(Array(summary["steps"].array.enumerated()), id: \.offset) { i, step in Text(verbatim: "\(i + 1). " + step["text"].string).textSelection(.enabled) }
            }
            if owner === sessions.current {
                Button("Inspect Current Plan") { Task { await sessions.loadPlan(identity, owner: owner) } }
                    .disabled(!sessions.isInspectionCurrent || !owner.isConnected || !owner.operation.isEmpty)
            } else { Text("Open the owning chat to load its complete revision-bound review.").font(.caption) }
            Text("This is proposed work, not verified completion. Plan approval and execution controls remain in the terminal app.").font(.caption).foregroundStyle(.secondary)
        }.padding(12).background(.background, in: RoundedRectangle(cornerRadius: 10))
    }
}

private struct PlanReviewContent: View {
    let review: NativePlanReview
    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Objective").font(.caption.weight(.semibold)); Text(verbatim: review.value["content"]["outcome"].string)
            ForEach(Array(review.value["content"]["steps"].array.enumerated()), id: \.offset) { i, step in Text(verbatim: "\(i + 1). " + step["text"].string) }
            strings("Inputs", review.value["input_names"])
            strings("Selected sources", review.value["sources"])
            strings("Plan sources", review.value["content"]["sources"])
            strings("Proposed external effects", review.value["content"]["effects"])
            if !review.value["correction"].string.isEmpty { Text("Requested change").font(.caption.weight(.semibold)); Text(verbatim: review.value["correction"].string) }
            if !review.value["issue"].string.isEmpty { Text(verbatim: review.value["issue"].string) }
            DisclosureGroup("Review identity, limits and access context") {
                Text(verbatim: "Task: " + review.identity.taskID + "\nRevision: \(review.identity.revision)\nDigest: " + review.identity.digest).font(.caption)
                // Read-only Go review: captured budgets, sources and access changes,
                // not a tool payload or execution grant.
                Text(verbatim: review.value.pretty).font(.system(.caption, design: .monospaced))
            }
        }.textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
    }
    @ViewBuilder private func strings(_ label: String, _ value: JSONValue) -> some View {
        if !value.array.isEmpty {
            Text(label).font(.caption.weight(.semibold))
            ForEach(Array(value.array.enumerated()), id: \.offset) { _, item in Text(verbatim: item.string) }
        }
    }
}
