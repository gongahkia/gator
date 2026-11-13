import SwiftUI
import GatorCore

struct ApprovalReviewView: View {
    @Environment(GatorApplicationState.self) private var application
    let review: ApprovalReview
    var compact: Bool
    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Label("Review this exact action", systemImage: "hand.raised").font(.headline)
            Text("Conversation: \(review.conversationID)").font(.caption).foregroundStyle(.secondary).textSelection(.enabled)
            ScrollView {
                VStack(alignment: .leading, spacing: 10) {
                    if review.inspectable {
                        Text(verbatim: review.result["title"].string).font(.headline)
                        Text(verbatim: review.result["summary"].string)
                        // All structured data remains literal and inspectable, including unknown fields.
                        Text(verbatim: review.result.pretty).font(.system(.caption, design: .monospaced)).textSelection(.enabled)
                    } else {
                        Text("Exact action details are missing. Approval is disabled; you can decline this pending request.")
                    }
                }.frame(maxWidth: .infinity, alignment: .leading).padding(10)
            }
            .frame(maxHeight: compact ? 280 : 420)
            .background(.background, in: RoundedRectangle(cornerRadius: 10))
            if !application.isCurrentReview(review) {
                Text("This review changed or disconnected. Close it and review the current action.").foregroundStyle(.orange).font(.callout)
            }
            HStack {
                Button("Close") { application.closeApprovalReview() }.disabled(application.approvalSending)
                Spacer()
                Button("Decline", role: .destructive) { Task { await application.respondToApproval(review, approved: false) } }
                    .disabled(!application.isCurrentReview(review) || application.approvalSending)
                Button("Approve Exact Action") { Task { await application.respondToApproval(review, approved: true) } }
                    .buttonStyle(.borderedProminent)
                    .disabled(!review.inspectable || !application.isCurrentReview(review) || application.approvalSending)
            }
            if application.approvalSending { Text("Waiting for Go to confirm the outcome…").font(.caption).foregroundStyle(.secondary) }
        }
        .padding(14)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 16))
    }
}
