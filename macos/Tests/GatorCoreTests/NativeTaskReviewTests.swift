import Foundation
import CryptoKit
import Testing
@testable import GatorCore

func proposedAction(_ body: String = "Exact body") -> JSONValue {
    .object(["kind": .string("approval"), "title": .string("Review requested action"), "fields": .array([.object(["label": .string("Action"), "value": .string("connector")])]), "preview": .object(["account": .string("fixture@example.com"), "to": .string("recipient@example.com"), "subject": .string("Original subject"), "body": .string(body)])])
}
func pendingAction(_ token: String = String(repeating: "a", count: 64), index: Int = 0) -> JSONValue {
    .object(["id": .number(7), "token": .string(token), "message_index": .number(Decimal(index))])
}
func planSummary(revision: Int = 1, digest: String = String(repeating: "c", count: 64), phase: String = "review") -> JSONValue {
    .object(["id": .string(String(repeating: "b", count: 32)), "revision": .number(Decimal(revision)), "digest": .string(digest), "task": .string("Compare venues"), "state": .string("queued"), "phase": .string(phase)])
}
func planValue(_ summary: JSONValue) -> JSONValue {
    guard case .object(var value) = summary else { return .null }
    value["content"] = .object(["outcome": .string("Exact proposed outcome"), "steps": .array([.object(["id": .string("one"), "text": .string("Read the selected scope")])]), "sources": .array([.string("original-source")]), "effects": .array([.string("read")])])
    value["input_names"] = .array([.string("brief.txt")]); return .object(value)
}

@Test @MainActor func taskInspectorUsesExactBackgroundOwnerAndRetainsIdentityAndNavigation() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let parent = SessionFixture(), child = SessionFixture(id: "child"), app = await managedFixture(root, [parent, child])
    try await startChild(app, parent: parent)
    app.current.editDraft("Original parent draft"); app.inspectTask("child")
    let selected = try #require(app.inspection), owner = try #require(app.inspectedOwner)
    #expect(app.current.conversationID == "parent"); #expect(owner.conversationID == selected.ownerID)
    #expect(NativeTaskPresentation(snapshot: try #require(owner.snapshot), connection: owner.connectionStatus, issue: "").ownerID == "child")
    await child.patch(["running": .bool(false), "dialogue_task": .object(["parent_id": .string("parent"), "turn_id": .string("original-turn"), "state": .string("completed"), "summary": .string("Actual retained result")])])
    try await waitUntil { owner.snapshot?.running == false }
    #expect(app.inspection == selected)
    #expect(NativeTaskPresentation(snapshot: try #require(owner.snapshot), connection: owner.connectionStatus, issue: "").summary == "Actual retained result")
    await app.openConversation("child"); #expect(app.inspection == nil)
    await app.openConversation("parent"); #expect(app.inspection == selected); #expect(app.current.draftText == "Original parent draft")
    #expect(await app.shutdown())
}

@Test @MainActor func correctionAffordanceOnlyOpensDraftAndAdmissionRestoresUnrelatedDraft() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = SessionFixture(), app = await managedFixture(root, [fixture]), owner = app.current
    await fixture.patch(["pending_approval": pendingAction(), "messages": .array([.object(["result": proposedAction()])])])
    try await waitUntil { owner.hasReviewableApproval }
    owner.openApprovalReview(); let exact = try #require(owner.review)
    owner.editDraft("My unrelated question"); owner.setPlanFirst(true)
    await app.requestActionCorrection(owner: owner, review: exact)
    #expect(owner.draftText.isEmpty); #expect(owner.correctionTarget?.kind == .action); #expect(!owner.planFirst)
    #expect(app.issue.isEmpty)
    #expect(await fixture.sent("upload_submit").isEmpty); #expect(await fixture.sent("dialogue_control").isEmpty); #expect(await fixture.sent("approval").isEmpty)
    owner.editDraft("Change only the subject"); await app.submit(from: owner)
    let request = try #require(await fixture.sent("upload_submit").last)
    #expect(request["upload"]["context"].string == "conversation:parent")
    #expect(request["upload"]["text"].string.contains("Change only the subject"))
    #expect(!request.pretty.contains(exact.token)); #expect(request["upload"]["plan_first"].bool == false)
    #expect(owner.draftText == "My unrelated question"); #expect(owner.planFirst)
    #expect(owner.correctionTarget == nil); #expect(await fixture.sent("approval").isEmpty)
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func staleOrDisconnectedCorrectionPreservesTextAndNeverReroutes() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = SessionFixture(), app = await managedFixture(root, [fixture]), owner = app.current
    await fixture.patch(["pending_approval": pendingAction(), "messages": .array([.object(["result": proposedAction()])])])
    try await waitUntil { owner.hasReviewableApproval }; owner.openApprovalReview()
    await app.requestActionCorrection(owner: owner, review: try #require(owner.review))
    owner.editDraft("Keep this correction")
    await fixture.patch(["pending_approval": pendingAction(String(repeating: "d", count: 64))])
    try await waitUntil { app.correctionUnavailableReason != nil }
    #expect(!app.canSubmit); await app.submit(from: owner)
    #expect(await fixture.sent("upload_submit").isEmpty); #expect(owner.draftText == "Keep this correction")
    await fixture.disconnect(); try await waitUntil { !owner.isConnected }
    await app.submit(from: owner); #expect(await fixture.sent("upload_submit").isEmpty)
    #expect(app.ownerCount == 1); #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func correctionDraftPersistsSeparatelyAndRequiresFreshInspectionAfterRestart() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = SessionFixture(), app = await managedFixture(root, [fixture]), owner = app.current
    await fixture.patch(["pending_approval": pendingAction(), "messages": .array([.object(["result": proposedAction()])])])
    try await waitUntil { owner.hasReviewableApproval }; owner.openApprovalReview()
    owner.editDraft("Retained normal draft")
    await app.requestActionCorrection(owner: owner, review: try #require(owner.review)); owner.editDraft("Retained correction")
    #expect(await app.shutdown(confirmed: true))
    let replacement = SessionFixture(), reopened = await managedFixture(root, [replacement])
    #expect(reopened.current.draftText == "Retained correction"); #expect(reopened.correctionUnavailableReason != nil)
    #expect(await replacement.sent("upload_submit").isEmpty)
    reopened.current.cancelCorrection(); #expect(reopened.current.draftText == "Retained normal draft")
    #expect(try await NativeDraftStore(directory: root.appendingPathComponent("native"), profile: "managed").load().version == 3)
    #expect(await reopened.shutdown())
}

@Test @MainActor func approvalCannotReappearFromRetainedOlderSnapshotOrBeRepeatedAfterSubmission() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = SessionFixture(), app = await managedFixture(root, [fixture]), owner = app.current
    let first = pendingAction(), next = pendingAction(String(repeating: "f", count: 64))
    await fixture.patch(["pending_approval": first, "messages": .array([.object(["result": proposedAction()])])])
    try await waitUntil { owner.hasReviewableApproval }; owner.openApprovalReview(); let old = try #require(owner.review)
    await fixture.patch(["pending_approval": next, "messages": .array([.object(["result": proposedAction("Replacement body")])])])
    try await waitUntil { !owner.isCurrentReview(old) }
    await app.respondToApproval(owner: owner, review: old, approved: true); #expect(await fixture.sent("approval").isEmpty)
    owner.openApprovalReview(); let fresh = try #require(owner.review)
    await app.respondToApproval(owner: owner, review: fresh, approved: false)
    await app.respondToApproval(owner: owner, review: fresh, approved: false)
    #expect(await fixture.sent("approval").count == 1)
    #expect(!owner.hasReviewableApproval) // Ack is not permission to repeat the same decision.
    await fixture.patch(["pending_approval": first]); try await Task.sleep(for: .milliseconds(20))
    owner.closeApprovalReview(); owner.openApprovalReview(); #expect(owner.review == nil)
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func missingReviewedMessageFailsClosedForBothApprovalAndDecline() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = SessionFixture(), app = await managedFixture(root, [fixture]), owner = app.current
    await fixture.patch(["pending_approval": pendingAction(index: 8)])
    try await waitUntil { owner.hasReviewableApproval }; owner.openApprovalReview()
    let missing = try #require(owner.review)
    #expect(!owner.isCurrentReview(missing))
    await app.respondToApproval(owner: owner, review: missing, approved: true)
    await app.respondToApproval(owner: owner, review: missing, approved: false)
    #expect(await fixture.sent("approval").isEmpty); #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func planInspectionMatchesExactReadResultAndCannotReuseChangedDigest() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = SessionFixture(), app = await managedFixture(root, [fixture], admissionTimeout: .seconds(2)), owner = app.current
    let summary = planSummary(), identity = try #require(NativePlanIdentity(conversationID: owner.conversationID, summary: summary))
    await fixture.patch(["task_plans": .array([summary])]); try await waitUntil { owner.isCurrentPlan(identity) }
    await app.loadPlan(identity, owner: owner)
    let sent = try #require(await fixture.sent("task_plan").last), request = sent["task_plan"]["request_id"].string
    #expect(sent["task_plan"]["action"].string == "inspect"); #expect(sent["task_plan"]["digest"].string == identity.digest)
    await fixture.patch(["task_plan_result": .object(["request_id": .string("unrelated"), "review": planValue(summary)])])
    try await Task.sleep(for: .milliseconds(20)); #expect(owner.planReviews.isEmpty)
    await fixture.patch(["task_plan_result": .object(["request_id": .string(request), "review": planValue(summary)])])
    try await waitUntil { !owner.planReviews.isEmpty }
    let review = try #require(owner.planReviews[identity.taskID]); #expect(owner.isCurrentPlanReview(review))
    #expect(review.value["content"]["steps"].array.first?["text"].string == "Read the selected scope")
    await fixture.patch(["task_plans": .array([planSummary(revision: 2, digest: String(repeating: "d", count: 64))])])
    try await waitUntil { !owner.isCurrentPlanReview(review) }
    await app.requestPlanRevision(owner: owner, review: review)
    #expect(owner.correctionTarget == nil); #expect(await fixture.sent("upload_submit").isEmpty)
    #expect(await app.shutdown())
}

@Test func exactContentAndNativeSemanticsAreAccessibleAndDoNotTreatReturnAsConsent() {
    let body = String(repeating: "Long paragraph\n", count: 150) + "FINAL REVIEW LINE"
    let fields = NativeReviewPresentation.fields(proposedAction(body))
    #expect(fields.contains { $0.label == "body" && $0.text == body })
    #expect(fields.contains { $0.label == "account" && $0.text == "fixture@example.com" })
    #expect(fields.contains { $0.label == "Action" && $0.text == "connector" })
    #expect(!NativeInteractionSemantics.decisionKeyAllowed(36)); #expect(!NativeInteractionSemantics.decisionKeyAllowed(76))
    #expect(NativeInteractionSemantics.decisionKeyAllowed(49)) // Space remains explicit keyboard access.
    #expect(NativeInteractionSemantics.activateDecisionOnSpace(code: 49, repeating: false, modified: false))
    #expect(!NativeInteractionSemantics.activateDecisionOnSpace(code: 49, repeating: true, modified: false))
    #expect(!NativeInteractionSemantics.activateDecisionOnSpace(code: 49, repeating: false, modified: true))
    #expect(!NativeInteractionSemantics.sendOnReturn(hasMarkedText: true, shift: false))
    #expect(!NativeInteractionSemantics.sendOnReturn(hasMarkedText: false, shift: true))
    #expect(NativeInteractionSemantics.sendOnReturn(hasMarkedText: false, shift: false))
    #expect(NativeInteractionSemantics.workingLabel(reduceMotion: true).contains("updates continue"))
}

@Test func taskInspectionDoesNotExposeRawToolPayloadOrPrivateReasoning() {
    let value = sampleState(["activity": .object(["entries": .array([.object(["kind": .string("text"), "text": .string("PRIVATE_REASONING")]), .object(["kind": .string("tool"), "tool": .string("read_file"), "state": .string("completed"), "payload": .string("RAW_SECRET")])])])])
    let presentation = NativeTaskPresentation(snapshot: value, connection: .connected, issue: "")
    #expect(presentation.publicActivity == ["read_file · completed"])
    #expect(!presentation.publicActivity.joined().contains("RAW_SECRET")); #expect(!presentation.publicActivity.joined().contains("PRIVATE_REASONING"))
}

@Test func realGoActionEnvelopeRendersExactPublicContentWithoutInterpretingUserBody() throws {
    let body = "Keep this literal: {\"preview\":\"not executable\"}\nFINAL REVIEW LINE"
    let payload: JSONValue = .object(["from": .string("me@example.com"), "to": .string("venue@example.com"), "subject": .string("Project Update"), "body": .string(body)])
    let encoded = String(decoding: try JSONEncoder().encode(payload), as: UTF8.self)
    let result: JSONValue = .object(["preview": .object(["connector_id": .string("google"), "operation": .string("gmail_send"), "preview": .string(encoded), "payload_sha256": .string("authoritative-digest")])])
    let fields = NativeReviewPresentation.fields(result)
    #expect(fields.contains { $0.label == "preview / to" && $0.text == "venue@example.com" })
    #expect(fields.contains { $0.label == "preview / body" && $0.text == body })
    #expect(fields.firstIndex(where: { $0.label == "preview / subject" })! < fields.firstIndex(where: { $0.label == "preview / body" })!)
    #expect(fields.contains { $0.isTechnical && $0.text == "authoritative-digest" })
    #expect(result["preview"]["preview"].string == encoded) // Exact source data is never rewritten.
}

@Test @MainActor func uncertainCorrectionRetainsOriginalAdmissionAndIsNotReplayedAfterRestart() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = SessionFixture(), app = await managedFixture(root, [fixture]), owner = app.current
    await fixture.patch(["pending_approval": pendingAction(), "messages": .array([.object(["result": proposedAction()])])])
    try await waitUntil { owner.hasReviewableApproval }; owner.openApprovalReview()
    owner.editDraft("Normal draft stays separate")
    await app.requestActionCorrection(owner: owner, review: try #require(owner.review))
    await fixture.setAutoAdmit(false); owner.editDraft("Correction with uncertain admission"); await app.submit(from: owner)
    let request = try #require(owner.pendingSubmission)
    #expect(request.phase == .uncertain); #expect(app.correctionStatus?.contains("no automatic retry") == true)
    #expect(await app.shutdown(confirmed: true))
    let next = SessionFixture(), reopened = await managedFixture(root, [next])
    #expect(reopened.current.pendingSubmission?.id == request.id)
    #expect(reopened.current.draftText == "Correction with uncertain admission")
    await reopened.retryAdmission(from: reopened.current) // Target epoch changed: do not re-admit stale instructions.
    #expect(await next.sent("upload_submit").isEmpty); #expect(await next.sent("approval").isEmpty)
    #expect(await reopened.shutdown(confirmed: true))
}

@Test @MainActor func inspectionBecomesStaleWhenOriginalOwnerReconnectsAndDoesNotStealFocus() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let parent = SessionFixture(), child = SessionFixture(id: "child"), replacement = SessionFixture(id: "child")
    let app = await managedFixture(root, [parent, child, replacement])
    try await startChild(app, parent: parent)
    app.inspectTask("child"); let retained = try #require(app.inspection)
    let originalFocus = app.current.focusRequest
    await child.disconnect(); try await waitUntil { app.owners["child"]?.connectionStatus == .disconnected }
    #expect(app.current.focusRequest == originalFocus); #expect(app.current.conversationID == "parent")
    await app.openConversation("child"); await app.reconnect(); await app.openConversation("parent")
    #expect(app.inspection == retained); #expect(!app.isInspectionCurrent)
    app.closeTaskInspection(); #expect(app.current.focusRequest != originalFocus)
    #expect(await replacement.sent("upload_submit").isEmpty); #expect(await app.shutdown())
}

@Test @MainActor func staleCorrectionAffordanceCannotRedirectANewerConversationSelection() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let first = SessionFixture(), second = SessionFixture(id: "second"), app = await managedFixture(root, [first, second])
    let originalOwner = app.current
    await first.patch(["pending_approval": pendingAction(), "messages": .array([.object(["result": proposedAction()])])])
    try await waitUntil { originalOwner.hasReviewableApproval }; originalOwner.openApprovalReview()
    let review = try #require(originalOwner.review)
    await app.newConversation(); app.current.editDraft("New conversation draft")
    await app.requestActionCorrection(owner: originalOwner, review: review)
    #expect(app.current.conversationID == "second"); #expect(app.current.draftText == "New conversation draft")
    #expect(originalOwner.correctionTarget == nil); #expect(await first.sent("approval").isEmpty)
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func workModeSelectionRequiresActualGoProjectionNotJustAcknowledgement() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = SessionFixture(), app = await managedFixture(root, [fixture]), owner = app.current
    let selecting = Task { await owner.selectWorkMode(.act) }
    try await waitForFrames(fixture, "submit", 1)
    #expect(!owner.operation.isEmpty)
    #expect(owner.snapshot?.value["work_details"]["mode"].string != "act")
    await fixture.patch(["work_details": .object(["mode": .string("act")])])
    await selecting.value
    #expect(owner.operation.isEmpty); #expect(owner.snapshot?.value["work_details"]["mode"].string == "act")
    #expect(await app.shutdown())
}
