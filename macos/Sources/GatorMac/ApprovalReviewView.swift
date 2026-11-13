import SwiftUI
import GatorCore

struct ApprovalReviewView: View {
    @Environment(GatorApplicationState.self) private var application
    @Environment(ManagedSessionCoordinator.self) private var sessions
    let review: ApprovalReview
    var compact: Bool
    private var current: Bool { sessions.current === application && application.isCurrentReview(review) }
    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Label("Review this exact action", systemImage: "hand.raised").font(.headline).accessibilityAddTraits(.isHeader)
            Text("Owner: " + application.title).font(.caption).textSelection(.enabled)
            ScrollView {
                VStack(alignment: .leading, spacing: 12) {
                    if review.inspectable { ExactReviewContent(value: review.result) }
                    else { Text("The exact reviewed message is missing. Decisions are disabled. Inspect retained state or reconnect explicitly.") }
                    Text("Approving permits exactly this proposed operation. Only Gator's retained outcome can confirm execution or delivery.").font(.caption).foregroundStyle(.secondary)
                }.frame(maxWidth: .infinity, alignment: .leading).padding(10)
            }.frame(minHeight: 80, maxHeight: compact ? 290 : 500)
                .background(.background, in: RoundedRectangle(cornerRadius: 10))
            if application.approvalSending {
                Label("Decision submitted · awaiting the original owner's outcome", systemImage: "clock").font(.caption)
            } else if !current {
                Label("This review is stale, unavailable or already submitted. Inspect the current proposal before deciding.", systemImage: "exclamationmark.triangle").font(.caption)
            }
            HStack {
                Button("Back to Conversation") { application.closeApprovalReview() }.keyboardShortcut(.escape, modifiers: []).disabled(application.approvalSending)
                Spacer()
                Button("Request Correction") { let owner = application; Task { await sessions.requestActionCorrection(owner: owner, review: review) } }
                    .disabled(!current || application.approvalSending || sessions.switching)
                    .help("Open an editable correction draft. Nothing is approved or sent.")
            }
            HStack {
                Text("Space or click chooses · Return never approves").font(.caption2).foregroundStyle(.secondary)
                Spacer(minLength: 0)
                ExactDecisionButton(title: "Decline", enabled: current && !application.approvalSending && !sessions.switching) {
                    let owner = application; Task { await sessions.respondToApproval(owner: owner, review: review, approved: false) }
                }.fixedSize()
                ExactDecisionButton(title: "Approve Exact Action", enabled: current && review.inspectable && !application.approvalSending && !sessions.switching) {
                    let owner = application; Task { await sessions.respondToApproval(owner: owner, review: review, approved: true) }
                }.fixedSize()
            }
        }.padding(14).background(.regularMaterial, in: RoundedRectangle(cornerRadius: 16))
            .accessibilityIdentifier("gator.exact.review." + review.conversationID)
    }
}
