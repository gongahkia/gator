import Foundation
import Testing
@testable import GatorCore

@Test @MainActor func realGoPresentationNavigationAndShutdownDoNotDecidePendingApproval() async throws {
    let fixture = try await NativeReviewFixture(action: false, providerFixture: "dev-acceptance-provider")
    let sessions = fixture.app
    do {
        let parent = sessions.current
        try await fixture.send("Hold unrelated work")
        try await managedEventually("independent original work held") {
            parent.snapshot?.running == true && parent.snapshot?.value["dialogue_active"].bool == true
        }
        try await fixture.send("Begin inert pending approval")
        try await managedEventually("inert review available") {
            sessions.owners.values.contains { $0 !== parent && $0.currentApprovalReview != nil }
        }
        let owner = try #require(sessions.owners.values.first { $0 !== parent && $0.currentApprovalReview != nil })
        let review = try #require(owner.currentApprovalReview)
        let directory = fixture.root.appendingPathComponent("state/gator/interactions")
        let files = try #require(FileManager.default.enumerator(at: directory, includingPropertiesForKeys: nil))
            .allObjects.compactMap { $0 as? URL }.filter { $0.pathExtension == "json" }
        #expect(files.count == 1)
        let recordURL = try #require(files.first)
        let original = try Data(contentsOf: recordURL)
        let record = try JSONDecoder().decode(JSONValue.self, from: original)
        #expect(record["status"].string == "pending")
        #expect(record["approved"] == .null)
        parent.editDraft("Keep the requesting chat draft")
        owner.editDraft("Keep the pending owner's draft")
        for _ in 0..<20 {
            await sessions.openConversation(owner.conversationID)
            owner.openApprovalReview()
            #expect(owner.review == review)
            sessions.focusComposer()
            #expect(owner.review == nil)
            #expect(owner.isCurrentReview(review))
            await sessions.openConversation(parent.conversationID)
            #expect(parent.draftText == "Keep the requesting chat draft")
            #expect(owner.currentApprovalReview == review)
            #expect(owner.generation == review.generation)
            #expect(!owner.approvalSending)
            #expect(sessions.ownerCount == 2)
            #expect(try Data(contentsOf: recordURL) == original)
        }
        #expect(owner.draftText == "Keep the pending owner's draft")
        #expect(!(await sessions.shutdown()))
        #expect(owner.isCurrentReview(review))
        #expect(try Data(contentsOf: recordURL) == original)
        #expect(await sessions.shutdown(confirmed: true))
        // stopping an owner must not manufacture either an approval or a decline.
        #expect(try Data(contentsOf: recordURL) == original)
        #expect(owner.connectionStatus == .closed)
        await fixture.close()
    } catch {
        await fixture.close()
        throw error
    }
}
