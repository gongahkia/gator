import Foundation
import Testing
@testable import GatorCore

func changedJSON(_ value: JSONValue, _ changes: [String: JSONValue]) -> JSONValue {
    guard case .object(var result) = value else { return value }; result.merge(changes) { _, new in new }; return .object(result)
}
func executableReview(_ summary: JSONValue) -> JSONValue {
    changedJSON(planValue(summary), ["budget": .object(["max_steps": .number(24), "code_max_steps": .number(8)]), "sources": .array([])])
}
@MainActor func inspectFixturePlan(_ app: ManagedSessionCoordinator, _ fixture: SessionFixture, summary: JSONValue = planSummary()) async throws -> NativePlanReview {
    let owner = app.current
    await fixture.patch(["task_plans": .array([summary])]); try await waitUntil { owner.snapshot?.value["task_plans"].array.first == summary }
    let identity = try #require(NativePlanIdentity(conversationID: owner.conversationID, summary: summary))
    await app.loadPlan(identity, owner: owner)
    let request = try #require(await fixture.sent("task_plan").last?["task_plan"]["request_id"].string)
    await fixture.patch(["task_plan_result": .object(["request_id": .string(request), "review": executableReview(summary)])])
    try await waitUntil { owner.planReviews[identity.taskID]?.identity == identity && owner.operation.isEmpty }
    return try #require(owner.planReviews[identity.taskID])
}

@Test @MainActor func exactPlanAuthorizationRequiresFullCurrentInspectionAndCorrectOwner() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = SessionFixture(), app = await managedFixture(root, [fixture], admissionTimeout: .seconds(2)), owner = app.current
    let review = try await inspectFixturePlan(app, fixture)
    #expect(app.planAuthorizationUnavailable(owner: owner, review: review, action: .approve) == nil)
    #expect(app.planAuthorizationUnavailable(owner: owner, review: review, action: .start) != nil)
    let uninspected = NativePlanReview(identity: review.identity, generation: owner.generation, value: review.value)
    #expect(app.planAuthorizationUnavailable(owner: owner, review: uninspected, action: .approve) != nil)
    let incomplete = NativePlanReview(identity: review.identity, generation: owner.generation, value: .null)
    #expect(!NativePlanPresentation.complete(incomplete))
    let wrong = try #require(NativePlanIdentity(conversationID: "other", summary: planSummary()))
    let spoof = NativePlanReview(identity: wrong, generation: owner.generation, value: review.value)
    await app.authorizePlan(owner: owner, review: spoof, action: .approve)
    #expect(await fixture.sent("task_plan").filter { $0["task_plan"]["action"].string == "approve" }.isEmpty)
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func planAckIsNotResultAndOnlyMatchingResultCanProjectExecution() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = SessionFixture(), app = await managedFixture(root, [fixture], admissionTimeout: .seconds(2)), owner = app.current
    let review = try await inspectFixturePlan(app, fixture)
    let pending = Task { await app.authorizePlan(owner: owner, review: review, action: .approve) }
    try await waitUntil { owner.planOperation?.phase == .acknowledged }
    let operation = try #require(owner.planOperation)
    let frame = try #require(await fixture.sent("task_plan").last?["task_plan"])
    #expect(frame["conversation_id"].string == owner.conversationID); #expect(frame["task_id"].string == review.identity.taskID)
    #expect(frame["revision"].int == review.identity.revision); #expect(frame["digest"].string == review.identity.digest)
    await fixture.patch(["task_plan_result": .object(["request_id": .string("wrong")])])
    try await Task.sleep(for: .milliseconds(20)); #expect(owner.planOperation?.resultConfirmed == false)
    await fixture.patch(["task_plan_result": .object(["request_id": .string(operation.id)])])
    await pending.value
    #expect(owner.planOperation?.phase == .accepted); #expect(owner.planOperation?.resultConfirmed == true)
    #expect(app.planAuthorizationUnavailable(owner: owner, review: review, action: .approve) != nil)
    let running = changedJSON(planSummary(), ["state": .string("running"), "phase": .string("dispatching"), "work_id": .string("original-run")])
    await fixture.patch(["task_plans": .array([running]), "running": .bool(true)])
    try await waitUntil { owner.planOperation?.phase == .working }
    await fixture.patch(["task_plans": .array([changedJSON(running, ["state": .string("completed")])]), "running": .bool(false)])
    try await waitUntil { owner.planOperation?.phase == .completed }
    await app.authorizePlan(owner: owner, review: review, action: .approve)
    #expect(await fixture.sent("task_plan").filter { $0["task_plan"]["action"].string == "approve" }.count == 1)
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func revisionDigestStateScopeAndPreparationChangesInvalidatePlanReview() async throws {
    let mutations: [[String: JSONValue]] = [
        ["task_plans": .array([planSummary(revision: 2)])],
        ["task_plans": .array([planSummary(digest: String(repeating: "d", count: 64))])],
        ["task_plans": .array([changedJSON(planSummary(), ["state": .string("cancelled")])])],
        ["task_plans": .array([planSummary(phase: "revising")])],
        ["task_plans": .array([planSummary(phase: "preparing")])],
        ["sources": .object(["selected": .array([.string("changed-account")])])],
        ["request_recoveries": .array([.object(["scope_digest": .string("changed")])])]
    ]
    for mutation in mutations {
        let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
        let fixture = SessionFixture(), app = await managedFixture(root, [fixture], admissionTimeout: .seconds(2)), owner = app.current
        let review = try await inspectFixturePlan(app, fixture)
        owner.editDraft("Preserve this correction or ordinary draft")
        await fixture.patch(mutation); try await waitUntil { !owner.isCurrentPlanReview(review) }
        await app.authorizePlan(owner: owner, review: review, action: .approve)
        #expect(await fixture.sent("task_plan").filter { $0["task_plan"]["action"].string == "approve" }.isEmpty)
        #expect(owner.draftText == "Preserve this correction or ordinary draft")
        #expect(await app.shutdown(confirmed: true))
    }
}

@Test @MainActor func duplicatePlanDecisionIsSerializedAndGoRejectionKeepsExactReview() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = SessionFixture(), app = await managedFixture(root, [fixture], admissionTimeout: .seconds(2)), owner = app.current
    let review = try await inspectFixturePlan(app, fixture)
    let first = Task { await app.authorizePlan(owner: owner, review: review, action: .approve) }
    let duplicate = Task { await app.authorizePlan(owner: owner, review: review, action: .approve) }
    try await waitUntil { owner.planOperation?.phase == .acknowledged }
    let id = try #require(owner.planOperation?.id)
    await fixture.patch(["task_plan_result": .object(["request_id": .string(id), "error": .string("original recovery scope changed")])])
    await first.value; await duplicate.value
    #expect(owner.planOperation?.phase == .rejected)
    #expect(owner.planOperation?.error == "original recovery scope changed")
    #expect(owner.planReviews[review.identity.taskID] == review)
    #expect(!owner.isCurrentPlanReview(review))
    #expect(await fixture.sent("task_plan").filter { $0["task_plan"]["action"].string == "approve" }.count == 1)
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func planUncertaintyPersistsAndRestartNeverReplaysAuthorization() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = SessionFixture(), app = await managedFixture(root, [fixture], admissionTimeout: .seconds(2)), owner = app.current
    let review = try await inspectFixturePlan(app, fixture)
    owner.editDraft("Retain normal text")
    let admission = Task { await app.authorizePlan(owner: owner, review: review, action: .approve) }
    try await waitUntil { owner.planOperation?.phase == .acknowledged }
    await fixture.disconnect(); await admission.value
    #expect(owner.planOperation?.phase == .uncertain)
    let original = try #require(owner.planOperation)
    #expect(app.quitGuard != nil); #expect(app.workingCount == 1)
    await app.authorizePlan(owner: owner, review: review, action: .approve)
    #expect(await fixture.sent("task_plan").filter { $0["task_plan"]["action"].string == "approve" }.count == 1)
    #expect(await app.shutdown(confirmed: true))
    let replacement = SessionFixture(), reopened = await managedFixture(root, [replacement], admissionTimeout: .seconds(2))
    #expect(reopened.current.planOperation?.id == original.id)
    #expect(reopened.current.planOperation?.phase == .uncertain)
    #expect(reopened.current.draftText == "Retain normal text")
    #expect(await replacement.sent("task_plan").isEmpty)
    let fresh = try await inspectFixturePlan(reopened, replacement, summary: planSummary(phase: "approved"))
    #expect(reopened.current.generation != original.generation)
    #expect(reopened.planAuthorizationUnavailable(owner: reopened.current, review: fresh, action: .start) == nil)
    #expect(await replacement.sent("task_plan").filter { $0["task_plan"]["action"].string == "start" }.isEmpty)
    #expect(try await NativeDraftStore(directory: root.appendingPathComponent("native"), profile: "managed").load().version == 4)
    #expect(await reopened.shutdown(confirmed: true))
}

@Test @MainActor func selectedOwnerAndFocusEpochGuardPlanAuthorizationAcrossConversations() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let first = SessionFixture(), other = SessionFixture(id: "other"), app = await managedFixture(root, [first, other], admissionTimeout: .seconds(2)), original = app.current
    let review = try await inspectFixturePlan(app, first)
    await app.newConversation(); #expect(app.current !== original)
    await app.authorizePlan(owner: original, review: review, action: .approve)
    #expect(await first.sent("task_plan").filter { $0["task_plan"]["action"].string == "approve" }.isEmpty)
    #expect(await other.sent("task_plan").isEmpty)
    await app.openConversation("parent"); #expect(!original.isCurrentPlanReview(review))
    let fresh = try await inspectFixturePlan(app, first)
    #expect(app.planAuthorizationUnavailable(owner: original, review: fresh, action: .approve) == nil)
    await other.disconnect(); try await waitUntil { !app.owners["other"]!.isConnected }
    #expect(original.isConnected); #expect(original.isCurrentPlanReview(fresh))
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func nativePlanAdmissionCannotBypassThreeWorkingOwnersOrEvictReview() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixtures = [SessionFixture(), SessionFixture(id: "a"), SessionFixture(id: "b"), SessionFixture(id: "c")]
    let app = await managedFixture(root, fixtures, admissionTimeout: .seconds(2)), parent = app.current
    _ = try await inspectFixturePlan(app, fixtures[0])
    for fixture in fixtures.dropFirst() { await app.newConversation(); await fixture.patch(["running": .bool(true)]); try await waitUntil { app.current.snapshot?.running == true } }
    #expect(app.workingCount == 3); #expect(app.ownerCount == 4)
    await app.openConversation("parent")
    let review = try await inspectFixturePlan(app, fixtures[0])
    #expect(app.planAuthorizationUnavailable(owner: parent, review: review, action: .approve)?.contains("Three") == true)
    await app.authorizePlan(owner: parent, review: review, action: .approve)
    #expect(await fixtures[0].sent("task_plan").filter { $0["task_plan"]["action"].string == "approve" }.isEmpty)
    #expect(await app.shutdown(confirmed: true))
}

@Test func planContentAndDecisionSemanticsDoNotImplyCompletedStepsOrReturnConsent() {
    #expect(NativePlanAction.approve.title == "Approve & Start")
    #expect(NativePlanAction.start.title != NativePlanAction.approve.title)
    #expect(NativePlanPresentation.phaseLabel(planSummary()).contains("not executed"))
    #expect(NativePlanPresentation.phaseLabel(planSummary(phase: "approved")).contains("undispatched"))
    #expect(!NativeInteractionSemantics.decisionKeyAllowed(36)); #expect(!NativeInteractionSemantics.decisionKeyAllowed(76))
    #expect(NativeInteractionSemantics.activateDecisionOnSpace(code: 49, repeating: false, modified: false))
}

@Test @MainActor func latePlanResultFromAnotherOwnerCannotClearSelectedOwnersPendingControl() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let first = SessionFixture(), other = SessionFixture(id: "other")
    let app = await managedFixture(root, [first, other], admissionTimeout: .seconds(1)), original = app.current
    let reviewA = try await inspectFixturePlan(app, first)
    await app.authorizePlan(owner: original, review: reviewA, action: .approve)
    let idA = try #require(original.planOperation?.id)
    await app.newConversation(); let second = app.current
    let reviewB = try await inspectFixturePlan(app, other)
    let attempt = Task { await app.authorizePlan(owner: second, review: reviewB, action: .approve) }
    try await waitUntil { second.planOperation?.phase == .acknowledged }
    let idB = try #require(second.planOperation?.id)
    await first.patch(["task_plan_result": .object(["request_id": .string(idA)])])
    try await waitUntil { original.planOperation?.resultConfirmed == true }
    #expect(second.planOperation?.id == idB); #expect(second.planOperation?.resultConfirmed == false)
    await other.patch(["task_plan_result": .object(["request_id": .string(idB), "error": .string("budget exhausted")])])
    await attempt.value
    #expect(app.current === second); #expect(second.planOperation?.error == "budget exhausted")
    #expect(original.planOperation?.id == idA)
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func finalPlanAdmissionRechecksCapacityAndStateAfterDurableRecording() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = SessionFixture(), app = await managedFixture(root, [fixture], admissionTimeout: .seconds(2)), owner = app.current
    let review = try await inspectFixturePlan(app, fixture)
    // The coordinator's final gate runs after the storage await. A task which
    // acquired the last slot meanwhile must prevent this process's frame send.
    await owner.authorizePlan(review, action: .approve, finalAdmissionAllowed: { false })
    #expect(owner.planOperation?.phase == .notSubmitted)
    #expect(await fixture.sent("task_plan").filter { $0["task_plan"]["action"].string == "approve" }.isEmpty)
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func approvedStartCannotAuthorizeUnapprovedDispatchedOrCancelledPlans() async throws {
    for summary in [planSummary(), changedJSON(planSummary(phase: "dispatching"), ["state": .string("completed")]), changedJSON(planSummary(phase: "approved"), ["state": .string("discarded")])] {
        let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
        let fixture = SessionFixture(), app = await managedFixture(root, [fixture], admissionTimeout: .seconds(2)), owner = app.current
        let review = try await inspectFixturePlan(app, fixture, summary: summary)
        #expect(app.planAuthorizationUnavailable(owner: owner, review: review, action: .start) != nil)
        await app.authorizePlan(owner: owner, review: review, action: .start)
        #expect(await fixture.sent("task_plan").filter { $0["task_plan"]["action"].string == "start" }.isEmpty)
        #expect(await app.shutdown(confirmed: true))
    }
}

@Test @MainActor func planPreparationRunningFlagDoesNotClaimReviewedExecution() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = SessionFixture(), app = await managedFixture(root, [fixture], admissionTimeout: .seconds(2)), owner = app.current
    owner.setPlanFirst(true); owner.editDraft("Prepare a proposal")
    await app.submit(from: owner)
    #expect(owner.submission?.phase == .accepted)
    await fixture.patch(["running": .bool(true), "task_plans": .array([planSummary(phase: "preparing")])])
    try await waitUntil { owner.snapshot?.running == true }
    #expect(owner.submission?.phase == .accepted); #expect(owner.planOperation == nil)
    await fixture.patch(["running": .bool(false), "task_plans": .array([planSummary()])])
    try await waitUntil { owner.snapshot?.running == false }
    #expect(owner.submission?.phase == .accepted)
    #expect(await fixture.sent("task_plan").isEmpty)
    #expect(await app.shutdown(confirmed: true))
}

@Test func nativePlanReceiptRetentionBoundsCompletedHistoryWithoutDroppingUncertainty() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let store = NativeDraftStore(directory: root, profile: "bounded-plan-receipts")
    var archive = NativeArchive(); archive.selectedConversationID = "chat-010"
    var records: [String: NativePlanOperation] = [:]
    for index in 0..<140 {
        let owner = String(format: "chat-%03d", index)
        let identity = try #require(NativePlanIdentity(conversationID: owner, summary: planSummary()))
        var record = NativePlanOperation(id: UUID().uuidString, nonce: UUID().uuidString, identity: identity, generation: UUID(), action: .approve)
        record.phase = index < 3 ? .uncertain : .completed
        records["conversation:" + owner] = record
    }
    archive.planOperationsByContext = records
    try await store.save(archive)
    let loaded = try await store.load()
    #expect(loaded.version == 4); #expect(loaded.planOperationsByContext?.count == 35)
    for index in 0..<3 {
        let context = "conversation:" + String(format: "chat-%03d", index)
        #expect(loaded.planOperationsByContext?[context]?.id == records[context]?.id)
    }
    #expect(loaded.planOperationsByContext?["conversation:chat-010"]?.id == records["conversation:chat-010"]?.id)
}
