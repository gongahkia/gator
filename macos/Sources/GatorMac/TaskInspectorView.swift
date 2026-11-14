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
                Button("Back to Conversation") { sessions.closeTaskInspection() }.focusable().keyboardShortcut(.escape, modifiers: [])
            }
            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
                    if let owner, let snapshot = owner.snapshot {
                        let task = NativeTaskPresentation(snapshot: snapshot, connection: owner.connectionStatus, issue: owner.issue)
                        Text(verbatim: task.title).font(.title3.weight(.medium)).textSelection(.enabled)
                        Label(task.state, systemImage: snapshot.hasApproval ? "hand.raised" : snapshot.running ? "clock" : "text.bubble")
                            .accessibilityLabel("Task status: " + task.state)
                            .background(NativeAccessibilityStatus(status: "Task: " + task.title + ". " + task.state).frame(width: 0, height: 0))
                        if !sessions.isInspectionCurrent { Text("This owner changed. Close these details and inspect the task again before requesting any controls.").foregroundStyle(.orange) }
                        if !task.summary.isEmpty { Text("Public retained summary · open the conversation for full output").font(.caption).foregroundStyle(.secondary); Text(verbatim: task.summary).textSelection(.enabled) }
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
                        ForEach(snapshot.value["task_plans"].array.compactMap(NativePlanSummary.init)) { card in
                            let summary = card.value
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
            Label("Task plan", systemImage: "list.number").font(.headline)
            Text(verbatim: summary["task"].string).textSelection(.enabled)
            Text("Phase: " + summary["phase"].string + " · revision " + String(identity.revision)).font(.caption)
            if !summary["issue"].string.isEmpty { Text(verbatim: summary["issue"].string).textSelection(.enabled) }
            if let review = owner.planReviews[identity.taskID] {
                let fresh = owner.isCurrentPlanReview(review)
                Text(fresh ? review.status : "This read belongs to an older plan, scope or task state. Inspect the current review again.").font(.callout).foregroundStyle(fresh ? Color.primary : .orange)
                PlanReviewContent(review: review, owner: owner)
                if fresh, owner === sessions.current, summary["phase"].string == "review" {
                    Button("Request Plan Revision") { Task { await sessions.requestPlanRevision(owner: owner, review: review) } }.focusable()
                        .disabled(sessions.switching || !sessions.isInspectionCurrent)
                        .help("Open an editable correction draft. This does not approve or execute the plan.")
                }
            } else {
                Text(verbatim: summary["outcome"].string).textSelection(.enabled)
                ForEach(Array(summary["steps"].array.enumerated()), id: \.offset) { i, step in Text(verbatim: "\(i + 1). " + step["text"].string).textSelection(.enabled) }
            }
            if owner === sessions.current {
                Button("Inspect Current Plan") { Task { await sessions.loadPlan(identity, owner: owner) } }.focusable()
                    .disabled(!sessions.isInspectionCurrent || !owner.isConnected || !owner.operation.isEmpty)
            } else { Text("Open the owning chat to load its complete revision-bound review.").font(.caption) }
            Text(NativePlanPresentation.phaseLabel(summary)).font(.caption).foregroundStyle(.secondary)
        }.padding(12).background(.background, in: RoundedRectangle(cornerRadius: 10))
    }
}
