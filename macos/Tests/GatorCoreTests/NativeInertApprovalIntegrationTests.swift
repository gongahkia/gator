import Foundation
import Testing
@testable import GatorCore

@Test @MainActor func realGoInertApprovalNavigationPreservesExactOwnerDraftAndUnrelatedWork() async throws {
    let fixture = try await NativeReviewFixture(action: false, providerFixture: "dev-acceptance-provider")
    let app = fixture.app
    func events() -> [JSONValue] {
        ((try? String(contentsOf: fixture.root.appendingPathComponent("provider-trace.jsonl"), encoding: .utf8)) ?? "")
            .split(separator: "\n").compactMap { try? JSONDecoder().decode(JSONValue.self, from: Data($0.utf8)) }
    }
    func release() { try? Data().write(to: fixture.root.appendingPathComponent("release-work")) }
    do {
        let parent = app.current
        try await fixture.send("Hold unrelated work")
        try await managedEventually("unrelated fixture work held") {
            parent.snapshot?.running == true && parent.snapshot?.value["dialogue_active"].bool == true && events().contains { $0["kind"].string == "held-work" }
        }
        try await fixture.send("Begin inert pending approval")
        try await managedEventually("inert approval owned by delegated child") {
            app.owners.values.contains { $0 !== parent && $0.currentApprovalReview != nil }
        }
        let child = try #require(app.owners.values.first { $0 !== parent && $0.currentApprovalReview != nil })
        let original = try #require(child.currentApprovalReview)
        #expect(original.inspectable)
        #expect(original.result.pretty.contains("acceptance.example.invalid"))
        parent.editDraft("Keep my foreground draft")
        await app.openConversation(child.conversationID)
        child.editDraft("Keep the task owner's draft")
        child.openApprovalReview()
        #expect(child.review == original)
        app.focusComposer()
        #expect(child.review == nil)
        #expect(child.currentApprovalReview == original)
        await app.openConversation(parent.conversationID)
        #expect(parent.draftText == "Keep my foreground draft")
        #expect(parent.snapshot?.running == true)
        await app.openConversation(child.conversationID)
        child.openApprovalReview()
        #expect(child.review == original)
        #expect(child.draftText == "Keep the task owner's draft")
        #expect(app.ownerCount == 2)
        await app.respondToApproval(owner: child, review: original, approved: false)
        try await managedEventually("explicit inert decline retained") { child.currentApprovalReview == nil && child.snapshot?.running == false }
        #expect(parent.snapshot?.running == true)
        #expect(events().contains { $0["kind"].string == "terminal" && $0["hasToolResult"].bool })
        #expect(!events().contains { ["unexpected-effect", "fixture-error"].contains($0["kind"].string) })
        await app.openConversation(parent.conversationID)
        #expect(parent.draftText == "Keep my foreground draft")
        release()
        try await managedEventually("held work completes only after release") { parent.snapshot?.running == false }
        await fixture.close()
    } catch {
        release(); await fixture.close(); throw error
    }
}
